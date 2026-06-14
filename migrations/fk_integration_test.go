package migrations

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"

	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/orm/backends/postgres"
	"github.com/oliverhaas/djangogo/orm/backends/sqlite"
)

// fkAuthor and fkArticle model a forward FK relation used by the
// makemigrations -> migrate FK integration tests.
type fkAuthor struct {
	ID   int64
	Name string `orm:"max_length=100"`
}

type fkArticle struct {
	ID     int64
	Title  string `orm:"max_length=200"`
	Author orm.FK[fkAuthor]
}

// fkRegistry registers Author and Article(FK[Author]) and resolves the relation.
func fkRegistry(t *testing.T) *orm.Registry {
	t.Helper()
	r := orm.NewRegistry()
	if _, err := r.Register(&fkAuthor{}); err != nil {
		t.Fatalf("Register(fkAuthor): %v", err)
	}
	if _, err := r.Register(&fkArticle{}); err != nil {
		t.Fatalf("Register(fkArticle): %v", err)
	}
	if err := r.Resolve(); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	r.Freeze()
	return r
}

// fkMigrations diffs an empty prior state against the resolved registry and
// returns the resulting migration (a CreateModel for each model).
func fkMigrations(t *testing.T, r *orm.Registry) []Migration {
	t.Helper()
	current := StateFromRegistry(r)
	ops := Diff(NewProjectState(), current)
	if len(ops) != 2 {
		t.Fatalf("Diff produced %d ops, want 2 CreateModel; ops=%#v", len(ops), ops)
	}
	for _, op := range ops {
		if _, ok := op.(CreateModel); !ok {
			t.Fatalf("expected CreateModel, got %T", op)
		}
	}
	return []Migration{{App: "blog", Name: "0001_initial", Operations: ops}}
}

// TestFKIntegration_SQLite runs makemigrations -> migrate for a relational model
// and confirms the article table carries a foreign key pointing at author(id).
func TestFKIntegration_SQLite(t *testing.T) {
	dsn := "file:" + t.Name() + "?mode=memory&cache=shared"
	sdb, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	sdb.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sdb.Close() })

	ctx := context.Background()
	// SQLite enforces foreign keys only when the pragma is on.
	if _, err := sdb.ExecContext(ctx, "PRAGMA foreign_keys = ON"); err != nil {
		t.Fatalf("enable foreign_keys: %v", err)
	}

	r := fkRegistry(t)
	db := orm.NewDB(sdb, sqlite.New(), r)
	migs := fkMigrations(t, r)

	if _, err := Apply(ctx, db, migs); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	// The fkarticle table declares a foreign key targeting fkauthor(id).
	rows, err := sdb.QueryContext(ctx, `PRAGMA foreign_key_list("fkarticle")`)
	if err != nil {
		t.Fatalf("PRAGMA foreign_key_list: %v", err)
	}
	defer func() { _ = rows.Close() }()

	var found bool
	cols, err := rows.Columns()
	if err != nil {
		t.Fatalf("rows.Columns: %v", err)
	}
	for rows.Next() {
		cells := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range cells {
			ptrs[i] = &cells[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			t.Fatalf("scan foreign_key_list row: %v", err)
		}
		row := map[string]any{}
		for i, c := range cols {
			row[c] = cells[i]
		}
		if asString(row["table"]) == "fkauthor" && asString(row["from"]) == "author_id" && asString(row["to"]) == "id" {
			found = true
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("foreign_key_list rows err: %v", err)
	}
	if !found {
		t.Fatal("fkarticle table has no foreign key to fkauthor(id)")
	}

	// With FK enforcement on, an insert referencing a missing author must fail.
	if _, err := sdb.ExecContext(ctx,
		`INSERT INTO "fkarticle" ("title", "author_id") VALUES (?, ?)`, "Orphan", 9999); err == nil {
		t.Fatal("insert with a dangling author_id should violate the foreign key")
	}
}

// asString coerces a PRAGMA cell (string or []byte) to string for comparison.
func asString(v any) string {
	switch s := v.(type) {
	case string:
		return s
	case []byte:
		return string(s)
	default:
		return ""
	}
}

// TestFKIntegration_Postgres runs the same makemigrations -> migrate flow against
// PostgreSQL and confirms the foreign key is enforced. Skipped without a DSN.
func TestFKIntegration_Postgres(t *testing.T) {
	dsn := os.Getenv("DJANGOGO_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("DJANGOGO_TEST_POSTGRES_DSN not set; skipping PostgreSQL FK integration test")
	}

	sdb, err := postgres.Open(dsn)
	if err != nil {
		t.Fatalf("postgres.Open: %v", err)
	}
	t.Cleanup(func() { _ = sdb.Close() })

	ctx := context.Background()
	if err := sdb.PingContext(ctx); err != nil {
		t.Fatalf("ping: %v", err)
	}

	// Clean slate: drop dependent table first, then base table and the migration
	// tracking table so Apply re-runs from scratch.
	for _, stmt := range []string{
		`DROP TABLE IF EXISTS "fkarticle"`,
		`DROP TABLE IF EXISTS "fkauthor"`,
	} {
		if _, err := sdb.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("cleanup %q: %v", stmt, err)
		}
	}
	t.Cleanup(func() {
		for _, stmt := range []string{
			`DROP TABLE IF EXISTS "fkarticle"`,
			`DROP TABLE IF EXISTS "fkauthor"`,
		} {
			_, _ = sdb.ExecContext(context.Background(), stmt)
		}
	})

	r := fkRegistry(t)
	migs := fkMigrations(t, r)
	d := postgres.New()

	// Execute each CreateModel's rendered DDL directly. The migration runner's
	// tracking-table DDL is SQLite-flavored (AUTOINCREMENT) and not portable to
	// PostgreSQL, so this test exercises the FK-capturing CreateModel SQL path
	// (createTableSQL) against a live PostgreSQL backend without it.
	ps := NewProjectState()
	for _, mig := range migs {
		for _, op := range mig.Operations {
			stmts, err := op.SQL(d, ps)
			if err != nil {
				t.Fatalf("%s SQL: %v", op.Describe(), err)
			}
			for _, s := range stmts {
				if _, err := sdb.ExecContext(ctx, s); err != nil {
					t.Fatalf("exec %q: %v", s, err)
				}
			}
			op.Apply(ps)
		}
	}

	// The foreign key is recorded in the catalog.
	var n int
	const q = `SELECT count(*) FROM information_schema.table_constraints
		WHERE table_name = 'fkarticle' AND constraint_type = 'FOREIGN KEY'`
	if err := sdb.QueryRowContext(ctx, q).Scan(&n); err != nil {
		t.Fatalf("query FK constraint: %v", err)
	}
	if n == 0 {
		t.Fatal("fkarticle table has no FOREIGN KEY constraint in information_schema")
	}

	// PostgreSQL always enforces FKs: a dangling author_id must fail.
	_, err = sdb.ExecContext(ctx,
		`INSERT INTO "fkarticle" ("title", "author_id") VALUES ($1, $2)`, "Orphan", 9999)
	if err == nil {
		t.Fatal("insert with a dangling author_id should violate the foreign key")
	}
	var pgErr interface{ SQLState() string }
	if errors.As(err, &pgErr) && pgErr.SQLState() != "23503" {
		t.Logf("foreign key violation SQLState = %s (want 23503)", pgErr.SQLState())
	}
}

// cascadeParent and cascadeChild model an ON DELETE CASCADE relation used to
// confirm the action is enforced end to end on a live database.
type cascadeParent struct {
	ID   int64
	Name string `orm:"max_length=50"`
}

type cascadeChild struct {
	ID     int64
	Parent orm.FK[cascadeParent] `orm:"on_delete=cascade"`
}

// TestFKOnDeleteCascade_Postgres confirms that deleting a parent row cascades to
// its children when the FK declares on_delete=cascade. Skipped without a DSN.
func TestFKOnDeleteCascade_Postgres(t *testing.T) {
	dsn := os.Getenv("DJANGOGO_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("DJANGOGO_TEST_POSTGRES_DSN not set; skipping PostgreSQL on_delete cascade test")
	}

	sdb, err := postgres.Open(dsn)
	if err != nil {
		t.Fatalf("postgres.Open: %v", err)
	}
	t.Cleanup(func() { _ = sdb.Close() })

	ctx := context.Background()
	drops := []string{`DROP TABLE IF EXISTS "cascadechild"`, `DROP TABLE IF EXISTS "cascadeparent"`}
	for _, s := range drops {
		if _, err := sdb.ExecContext(ctx, s); err != nil {
			t.Fatalf("cleanup %q: %v", s, err)
		}
	}
	t.Cleanup(func() {
		for _, s := range drops {
			_, _ = sdb.ExecContext(context.Background(), s)
		}
	})

	reg := orm.NewRegistry()
	if _, err := reg.Register(&cascadeParent{}); err != nil {
		t.Fatalf("Register(cascadeParent): %v", err)
	}
	if _, err := reg.Register(&cascadeChild{}); err != nil {
		t.Fatalf("Register(cascadeChild): %v", err)
	}
	if err := reg.Resolve(); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	reg.Freeze()

	d := postgres.New()
	parent, _ := reg.Get("cascadeParent")
	child, _ := reg.Get("cascadeChild")
	for _, m := range []*orm.Model{parent, child} {
		if _, err := sdb.ExecContext(ctx, d.CreateTableSQL(m)); err != nil {
			t.Fatalf("create table %s: %v", m.Name(), err)
		}
	}

	db := orm.NewDB(sdb, d, reg)
	p := &cascadeParent{Name: "p"}
	if err := orm.Query[cascadeParent](db).Create(ctx, p); err != nil {
		t.Fatalf("create parent: %v", err)
	}
	c := &cascadeChild{}
	c.Parent.SetPK(p.ID)
	if err := orm.Query[cascadeChild](db).Create(ctx, c); err != nil {
		t.Fatalf("create child: %v", err)
	}

	if _, err := orm.Query[cascadeParent](db).Filter("id", p.ID).Delete(ctx); err != nil {
		t.Fatalf("delete parent: %v", err)
	}

	n, err := orm.Query[cascadeChild](db).Count(ctx)
	if err != nil {
		t.Fatalf("count children: %v", err)
	}
	if n != 0 {
		t.Fatalf("after cascade delete, child count = %d, want 0", n)
	}
}
