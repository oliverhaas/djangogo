package migrations

import (
	"context"
	"database/sql"
	"testing"

	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/orm/backends/sqlite"
)

// newTestDB opens an isolated in-memory SQLite database named after the test and wraps
// it in an *orm.DB. The pool is pinned to a single connection so the in-memory database
// is shared across queries. Tests that use this MUST NOT call t.Parallel(), since they
// share a database name within a process.
func newTestDB(t *testing.T) *orm.DB {
	t.Helper()
	dsn := "file:" + t.Name() + "?mode=memory&cache=shared"
	sdb, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	sdb.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sdb.Close() })
	return orm.NewDB(sdb, sqlite.New(), orm.NewRegistry())
}

// bioField returns a nullable Bio text field state.
func bioField() FieldState {
	return FieldState{Name: "Bio", Column: "bio", Kind: orm.KindText, Null: true}
}

// createPersonMigration returns a 0001 migration that creates the Person model.
func createPersonMigration() Migration {
	return Migration{
		App:        "app",
		Name:       "0001",
		Operations: []Operation{CreateModel{Name: "Person", Table: "person", Fields: personFields()}},
	}
}

// ---------------------------------------------------------------------------
// StateFromMigrations
// ---------------------------------------------------------------------------

func TestStateFromMigrations(t *testing.T) {
	t.Parallel()
	migs := []Migration{
		{App: "app", Name: "0001", Operations: []Operation{
			CreateModel{Name: "Person", Table: "person", Fields: personFields()},
		}},
		{App: "app", Name: "0002", Operations: []Operation{
			AddField{Model: "Person", Field: bioField()},
		}},
	}
	ps := StateFromMigrations(migs)
	ms, ok := ps.Models["Person"]
	if !ok {
		t.Fatal("Person model missing from replayed state")
	}
	if _, ok := ms.FieldByName("Bio"); !ok {
		t.Errorf("Person.Bio field missing after replay; fields: %#v", ms.Fields)
	}
}

// ---------------------------------------------------------------------------
// Registry
// ---------------------------------------------------------------------------

func TestRegistryOrdering(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	a1 := Migration{App: "blog", Name: "0001"}
	a2 := Migration{App: "blog", Name: "0002"}
	b1 := Migration{App: "auth", Name: "0001"}
	// Interleave insertion order to prove app order tracks first-seen, not Add call order.
	r.Add(a1)
	r.Add(b1)
	r.Add(a2)

	blog := r.ForApp("blog")
	if len(blog) != 2 || blog[0].Name != "0001" || blog[1].Name != "0002" {
		t.Errorf("ForApp(blog): got %#v, want [0001 0002]", blog)
	}
	if got := r.ForApp("missing"); got != nil {
		t.Errorf("ForApp(missing): got %#v, want nil", got)
	}

	all := r.All()
	wantKeys := []string{"blog/0001", "blog/0002", "auth/0001"}
	if len(all) != len(wantKeys) {
		t.Fatalf("All(): got %d migrations, want %d (%#v)", len(all), len(wantKeys), all)
	}
	for i, want := range wantKeys {
		got := all[i].App + "/" + all[i].Name
		if got != want {
			t.Errorf("All()[%d]: got %s, want %s", i, got, want)
		}
	}
}

// ---------------------------------------------------------------------------
// EnsureTable + AppliedSet
// ---------------------------------------------------------------------------

func TestEnsureTableAndAppliedSet(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	if err := EnsureTable(ctx, db); err != nil {
		t.Fatalf("EnsureTable: %v", err)
	}
	applied, err := AppliedSet(ctx, db)
	if err != nil {
		t.Fatalf("AppliedSet: %v", err)
	}
	if len(applied) != 0 {
		t.Errorf("AppliedSet on empty table: got %#v, want empty", applied)
	}

	// Idempotent re-create must not error.
	if err := EnsureTable(ctx, db); err != nil {
		t.Fatalf("EnsureTable (second call): %v", err)
	}

	if _, err := Apply(ctx, db, []Migration{createPersonMigration()}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	applied, err = AppliedSet(ctx, db)
	if err != nil {
		t.Fatalf("AppliedSet after Apply: %v", err)
	}
	if !applied["app/0001"] {
		t.Errorf("AppliedSet after Apply: got %#v, want app/0001 present", applied)
	}
}

// ---------------------------------------------------------------------------
// Apply: single migration
// ---------------------------------------------------------------------------

func TestApplyCreateModel(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	done, err := Apply(ctx, db, []Migration{createPersonMigration()})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(done) != 1 || done[0] != "app/0001" {
		t.Errorf("done: got %#v, want [app/0001]", done)
	}

	// The person table exists: insert + select round-trips.
	if _, err := db.SQL().ExecContext(ctx, `INSERT INTO "person" ("name", "age") VALUES (?, ?)`, "Ada", 36); err != nil {
		t.Fatalf("insert into person: %v", err)
	}
	var name string
	var age int
	if err := db.SQL().QueryRowContext(ctx, `SELECT "name", "age" FROM "person"`).Scan(&name, &age); err != nil {
		t.Fatalf("select from person: %v", err)
	}
	if name != "Ada" || age != 36 {
		t.Errorf("round-trip: got (%q, %d), want (Ada, 36)", name, age)
	}
}

// ---------------------------------------------------------------------------
// Idempotency
// ---------------------------------------------------------------------------

func TestApplyIdempotent(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	migs := []Migration{createPersonMigration()}

	if _, err := Apply(ctx, db, migs); err != nil {
		t.Fatalf("Apply (first): %v", err)
	}
	// A second Apply must be a no-op: empty done, no error (table not recreated).
	done, err := Apply(ctx, db, migs)
	if err != nil {
		t.Fatalf("Apply (second): %v", err)
	}
	if len(done) != 0 {
		t.Errorf("Apply (second) done: got %#v, want empty", done)
	}
}

// ---------------------------------------------------------------------------
// Incremental: state threading across an already-applied migration
// ---------------------------------------------------------------------------

func TestApplyIncremental(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	mig1 := createPersonMigration()
	mig2 := Migration{
		App:          "app",
		Name:         "0002",
		Dependencies: []string{"0001"},
		Operations:   []Operation{AddField{Model: "Person", Field: bioField()}},
	}

	if _, err := Apply(ctx, db, []Migration{mig1}); err != nil {
		t.Fatalf("Apply 0001: %v", err)
	}

	// Now apply [0001, 0002]: 0001 is skipped but advances state so 0002's AddField
	// sees the post-0001 schema and renders ALTER TABLE against the existing table.
	done, err := Apply(ctx, db, []Migration{mig1, mig2})
	if err != nil {
		t.Fatalf("Apply 0001+0002: %v", err)
	}
	if len(done) != 1 || done[0] != "app/0002" {
		t.Errorf("done: got %#v, want [app/0002]", done)
	}

	// The bio column now exists.
	if _, err := db.SQL().ExecContext(ctx, `INSERT INTO "person" ("name", "age", "bio") VALUES (?, ?, ?)`, "Grace", 40, "hopper"); err != nil {
		t.Fatalf("insert with bio: %v", err)
	}
	var bio string
	if err := db.SQL().QueryRowContext(ctx, `SELECT "bio" FROM "person"`).Scan(&bio); err != nil {
		t.Fatalf("select bio: %v", err)
	}
	if bio != "hopper" {
		t.Errorf("bio: got %q, want hopper", bio)
	}
}

// ---------------------------------------------------------------------------
// Rebuild op: RemoveField via temp-table rebuild preserves rows
// ---------------------------------------------------------------------------

func TestApplyRebuildPreservesRows(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	if _, err := Apply(ctx, db, []Migration{createPersonMigration()}); err != nil {
		t.Fatalf("Apply 0001: %v", err)
	}
	if _, err := db.SQL().ExecContext(ctx, `INSERT INTO "person" ("name", "age") VALUES (?, ?)`, "Alan", 41); err != nil {
		t.Fatalf("insert person: %v", err)
	}

	mig2 := Migration{
		App:        "app",
		Name:       "0002",
		Operations: []Operation{RemoveField{Model: "Person", Field: "Age"}},
	}
	done, err := Apply(ctx, db, []Migration{createPersonMigration(), mig2})
	if err != nil {
		t.Fatalf("Apply remove field: %v", err)
	}
	if len(done) != 1 || done[0] != "app/0002" {
		t.Errorf("done: got %#v, want [app/0002]", done)
	}

	// The age column is gone, but the existing row (its name) is preserved.
	var name string
	if err := db.SQL().QueryRowContext(ctx, `SELECT "name" FROM "person"`).Scan(&name); err != nil {
		t.Fatalf("select name after rebuild: %v", err)
	}
	if name != "Alan" {
		t.Errorf("preserved name: got %q, want Alan", name)
	}
	// The age column is gone. (SQLite treats SELECT "age" on a missing column as a
	// string literal, so probe the schema via pragma_table_info instead.)
	if hasColumn(ctx, t, db, "person", "age") {
		t.Error("age column still present after RemoveField rebuild")
	}
	if !hasColumn(ctx, t, db, "person", "name") {
		t.Error("name column missing after RemoveField rebuild")
	}
}

// hasColumn reports whether table has a column named col.
func hasColumn(ctx context.Context, t *testing.T, db *orm.DB, table, col string) bool {
	t.Helper()
	rows, err := db.SQL().QueryContext(ctx, `SELECT name FROM pragma_table_info(?)`, table)
	if err != nil {
		t.Fatalf("pragma_table_info(%q): %v", table, err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan column name: %v", err)
		}
		if name == col {
			return true
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("pragma rows err: %v", err)
	}
	return false
}

// ---------------------------------------------------------------------------
// Failure rollback
// ---------------------------------------------------------------------------

func TestApplyFailureRollsBack(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	if _, err := Apply(ctx, db, []Migration{createPersonMigration()}); err != nil {
		t.Fatalf("Apply 0001: %v", err)
	}

	// 0002 adds a column that already exists -> duplicate-column error at exec time.
	dupAge := FieldState{Name: "Age", Column: "age", Kind: orm.KindInt, Null: true}
	mig2 := Migration{
		App:        "app",
		Name:       "0002",
		Operations: []Operation{AddField{Model: "Person", Field: dupAge}},
	}

	done, err := Apply(ctx, db, []Migration{createPersonMigration(), mig2})
	if err == nil {
		t.Fatal("Apply: expected error from duplicate column, got nil")
	}
	if len(done) != 0 {
		t.Errorf("done on failure: got %#v, want empty", done)
	}

	// The failed migration must NOT be recorded (transaction rolled back).
	applied, err := AppliedSet(ctx, db)
	if err != nil {
		t.Fatalf("AppliedSet: %v", err)
	}
	if applied["app/0002"] {
		t.Error("app/0002 recorded as applied despite rollback")
	}
	if !applied["app/0001"] {
		t.Error("app/0001 missing; earlier migration should remain applied")
	}
}
