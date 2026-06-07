package migrations

import (
	"database/sql"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/orm/backends/sqlite"
)

// ---------------------------------------------------------------------------
// Shared fixtures: a Person model state with id (auto pk), name (char 100), age (int).
// ---------------------------------------------------------------------------

func personFields() []FieldState {
	return []FieldState{
		{Name: "ID", Column: "id", Kind: orm.KindAuto, PrimaryKey: true},
		{Name: "Name", Column: "name", Kind: orm.KindChar, MaxLength: 100},
		{Name: "Age", Column: "age", Kind: orm.KindInt},
	}
}

// personState returns a ProjectState containing only the Person model.
func personState() *ProjectState {
	ps := NewProjectState()
	ps.Models["Person"] = &ModelState{
		Name:   "Person",
		Table:  "person",
		Fields: personFields(),
	}
	ps.Order = []string{"Person"}
	return ps
}

// assertSQL compares a statement slice against the expected statements.
func assertSQL(t *testing.T, got []string, err error, want []string) {
	t.Helper()
	if err != nil {
		t.Fatalf("SQL returned error: %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("statement count: got %d, want %d\ngot:  %#v\nwant: %#v", len(got), len(want), got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("statement[%d]:\n got: %s\nwant: %s", i, got[i], want[i])
		}
	}
}

// ---------------------------------------------------------------------------
// CreateModel
// ---------------------------------------------------------------------------

func TestCreateModel_SQL(t *testing.T) {
	t.Parallel()
	d := sqlite.New()
	op := CreateModel{Name: "Person", Table: "person", Fields: personFields()}
	got, err := op.SQL(d, NewProjectState())
	assertSQL(t, got, err, []string{
		`CREATE TABLE "person" ("id" INTEGER PRIMARY KEY AUTOINCREMENT, "name" VARCHAR(100) NOT NULL, "age" INTEGER NOT NULL)`,
	})
}

func TestCreateModel_Apply(t *testing.T) {
	t.Parallel()
	ps := NewProjectState()
	op := CreateModel{Name: "Person", Table: "person", Fields: personFields()}
	op.Apply(ps)

	ms, ok := ps.Models["Person"]
	if !ok {
		t.Fatal("Person not added to Models")
	}
	if ms.Table != "person" {
		t.Errorf("Table = %q, want %q", ms.Table, "person")
	}
	if len(ms.Fields) != 3 {
		t.Fatalf("Fields len = %d, want 3", len(ms.Fields))
	}
	if len(ps.Order) != 1 || ps.Order[0] != "Person" {
		t.Errorf("Order = %v, want [Person]", ps.Order)
	}

	// Mutating the op's fields must not affect the stored state (copy semantics).
	op.Fields[0].Column = "mutated"
	if ps.Models["Person"].Fields[0].Column == "mutated" {
		t.Error("Apply did not copy Fields; mutation leaked into state")
	}

	// Applying again must not duplicate the Order entry.
	op.Apply(ps)
	if len(ps.Order) != 1 {
		t.Errorf("re-Apply duplicated Order: %v", ps.Order)
	}
}

func TestCreateModel_Describe(t *testing.T) {
	t.Parallel()
	op := CreateModel{Name: "Person", Table: "person"}
	if got := op.Describe(); got != "CreateModel Person" {
		t.Errorf("Describe = %q, want %q", got, "CreateModel Person")
	}
}

// ---------------------------------------------------------------------------
// DeleteModel
// ---------------------------------------------------------------------------

func TestDeleteModel_SQL(t *testing.T) {
	t.Parallel()
	d := sqlite.New()
	op := DeleteModel{Name: "Person"}
	got, err := op.SQL(d, personState())
	assertSQL(t, got, err, []string{`DROP TABLE "person"`})
}

func TestDeleteModel_SQL_MissingModel(t *testing.T) {
	t.Parallel()
	d := sqlite.New()
	op := DeleteModel{Name: "Ghost"}
	if _, err := op.SQL(d, NewProjectState()); err == nil {
		t.Fatal("expected error for missing model, got nil")
	}
}

func TestDeleteModel_Apply(t *testing.T) {
	t.Parallel()
	ps := personState()
	op := DeleteModel{Name: "Person"}
	op.Apply(ps)

	if _, ok := ps.Models["Person"]; ok {
		t.Error("Person still present in Models after Apply")
	}
	if len(ps.Order) != 0 {
		t.Errorf("Order = %v, want empty", ps.Order)
	}
}

func TestDeleteModel_Describe(t *testing.T) {
	t.Parallel()
	op := DeleteModel{Name: "Person"}
	if got := op.Describe(); got != "DeleteModel Person" {
		t.Errorf("Describe = %q, want %q", got, "DeleteModel Person")
	}
}

// ---------------------------------------------------------------------------
// AddField
// ---------------------------------------------------------------------------

func TestAddField_SQL(t *testing.T) {
	t.Parallel()
	d := sqlite.New()
	op := AddField{
		Model: "Person",
		Field: FieldState{Name: "Bio", Column: "bio", Kind: orm.KindText, Null: true},
	}
	got, err := op.SQL(d, personState())
	assertSQL(t, got, err, []string{`ALTER TABLE "person" ADD COLUMN "bio" TEXT`})
}

func TestAddField_SQL_NotNullableErrors(t *testing.T) {
	t.Parallel()
	d := sqlite.New()
	op := AddField{
		Model: "Person",
		Field: FieldState{Name: "Bio", Column: "bio", Kind: orm.KindText},
	}
	_, err := op.SQL(d, personState())
	if err == nil {
		t.Fatal("expected error for non-nullable AddField, got nil")
	}
	if !strings.Contains(err.Error(), "must be nullable") {
		t.Errorf("error = %q, want it to mention nullability", err.Error())
	}
}

func TestAddField_SQL_MissingModel(t *testing.T) {
	t.Parallel()
	d := sqlite.New()
	op := AddField{
		Model: "Ghost",
		Field: FieldState{Name: "Bio", Column: "bio", Kind: orm.KindText, Null: true},
	}
	if _, err := op.SQL(d, NewProjectState()); err == nil {
		t.Fatal("expected error for missing model, got nil")
	}
}

func TestAddField_Apply(t *testing.T) {
	t.Parallel()
	ps := personState()
	op := AddField{
		Model: "Person",
		Field: FieldState{Name: "Bio", Column: "bio", Kind: orm.KindText, Null: true},
	}
	op.Apply(ps)

	fields := ps.Models["Person"].Fields
	if len(fields) != 4 {
		t.Fatalf("Fields len = %d, want 4", len(fields))
	}
	if fields[3].Name != "Bio" {
		t.Errorf("appended field name = %q, want %q", fields[3].Name, "Bio")
	}
}

func TestAddField_Describe(t *testing.T) {
	t.Parallel()
	op := AddField{Model: "Person", Field: FieldState{Name: "Bio"}}
	if got := op.Describe(); got != "AddField Person.Bio" {
		t.Errorf("Describe = %q, want %q", got, "AddField Person.Bio")
	}
}

// ---------------------------------------------------------------------------
// RemoveField
// ---------------------------------------------------------------------------

func TestRemoveField_SQL(t *testing.T) {
	t.Parallel()
	d := sqlite.New()
	op := RemoveField{Model: "Person", Field: "Age"}
	got, err := op.SQL(d, personState())
	assertSQL(t, got, err, []string{
		`CREATE TABLE "person__new" ("id" INTEGER PRIMARY KEY AUTOINCREMENT, "name" VARCHAR(100) NOT NULL)`,
		`INSERT INTO "person__new" ("id", "name") SELECT "id", "name" FROM "person"`,
		`DROP TABLE "person"`,
		`ALTER TABLE "person__new" RENAME TO "person"`,
	})
}

func TestRemoveField_SQL_MissingField(t *testing.T) {
	t.Parallel()
	d := sqlite.New()
	op := RemoveField{Model: "Person", Field: "Nope"}
	if _, err := op.SQL(d, personState()); err == nil {
		t.Fatal("expected error for missing field, got nil")
	}
}

func TestRemoveField_SQL_MissingModel(t *testing.T) {
	t.Parallel()
	d := sqlite.New()
	op := RemoveField{Model: "Ghost", Field: "Age"}
	if _, err := op.SQL(d, NewProjectState()); err == nil {
		t.Fatal("expected error for missing model, got nil")
	}
}

func TestRemoveField_Apply(t *testing.T) {
	t.Parallel()
	ps := personState()
	op := RemoveField{Model: "Person", Field: "Age"}
	op.Apply(ps)

	fields := ps.Models["Person"].Fields
	if len(fields) != 2 {
		t.Fatalf("Fields len = %d, want 2", len(fields))
	}
	for _, f := range fields {
		if f.Name == "Age" {
			t.Error("Age field still present after Apply")
		}
	}
}

func TestRemoveField_Describe(t *testing.T) {
	t.Parallel()
	op := RemoveField{Model: "Person", Field: "Age"}
	if got := op.Describe(); got != "RemoveField Person.Age" {
		t.Errorf("Describe = %q, want %q", got, "RemoveField Person.Age")
	}
}

// ---------------------------------------------------------------------------
// AlterField
// ---------------------------------------------------------------------------

func TestAlterField_SQL(t *testing.T) {
	t.Parallel()
	d := sqlite.New()
	op := AlterField{
		Model: "Person",
		Field: FieldState{Name: "Name", Column: "name", Kind: orm.KindChar, MaxLength: 200},
	}
	got, err := op.SQL(d, personState())
	assertSQL(t, got, err, []string{
		`CREATE TABLE "person__new" ("id" INTEGER PRIMARY KEY AUTOINCREMENT, "name" VARCHAR(200) NOT NULL, "age" INTEGER NOT NULL)`,
		`INSERT INTO "person__new" ("id", "name", "age") SELECT "id", "name", "age" FROM "person"`,
		`DROP TABLE "person"`,
		`ALTER TABLE "person__new" RENAME TO "person"`,
	})
}

func TestAlterField_SQL_MissingField(t *testing.T) {
	t.Parallel()
	d := sqlite.New()
	op := AlterField{
		Model: "Person",
		Field: FieldState{Name: "Nope", Column: "nope", Kind: orm.KindInt},
	}
	if _, err := op.SQL(d, personState()); err == nil {
		t.Fatal("expected error for missing field, got nil")
	}
}

func TestAlterField_SQL_MissingModel(t *testing.T) {
	t.Parallel()
	d := sqlite.New()
	op := AlterField{
		Model: "Ghost",
		Field: FieldState{Name: "Name", Column: "name", Kind: orm.KindChar, MaxLength: 200},
	}
	if _, err := op.SQL(d, NewProjectState()); err == nil {
		t.Fatal("expected error for missing model, got nil")
	}
}

func TestAlterField_Apply(t *testing.T) {
	t.Parallel()
	ps := personState()
	op := AlterField{
		Model: "Person",
		Field: FieldState{Name: "Name", Column: "name", Kind: orm.KindChar, MaxLength: 200},
	}
	op.Apply(ps)

	f, ok := ps.Models["Person"].FieldByName("Name")
	if !ok {
		t.Fatal("Name field missing after Apply")
	}
	if f.MaxLength != 200 {
		t.Errorf("MaxLength = %d, want 200", f.MaxLength)
	}
}

func TestAlterField_Describe(t *testing.T) {
	t.Parallel()
	op := AlterField{Model: "Person", Field: FieldState{Name: "Name"}}
	if got := op.Describe(); got != "AlterField Person.Name" {
		t.Errorf("Describe = %q, want %q", got, "AlterField Person.Name")
	}
}

// ---------------------------------------------------------------------------
// rebuildTableSQL: empty common columns omits the INSERT.
// ---------------------------------------------------------------------------

func TestRebuildTableSQL_NoCommonColumns(t *testing.T) {
	t.Parallel()
	d := sqlite.New()
	old := []FieldState{{Name: "Old", Column: "old", Kind: orm.KindInt}}
	newF := []FieldState{{Name: "New", Column: "new", Kind: orm.KindInt}}
	got := rebuildTableSQL(d, "t", old, newF)
	if len(got) != 3 {
		t.Fatalf("statement count = %d, want 3 (INSERT omitted)\n%#v", len(got), got)
	}
	for _, stmt := range got {
		if strings.HasPrefix(stmt, "INSERT") {
			t.Errorf("unexpected INSERT statement: %s", stmt)
		}
	}
}

// ---------------------------------------------------------------------------
// Integration: real in-memory SQLite rebuild preserves data.
// ---------------------------------------------------------------------------

// openMemDB opens a shared-cache in-memory SQLite database isolated to the calling
// test. The database name is derived from the test name so parallel integration tests
// do not collide on a single process-wide in-memory database. The pool is pinned to a
// single connection so the shared in-memory database survives for the test's duration.
func openMemDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := "file:" + t.Name() + "?mode=memory&cache=shared"
	db, err := sqlite.Open(dsn)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	db.SetMaxOpenConns(1)
	return db
}

func execAll(t *testing.T, db *sql.DB, stmts []string) {
	t.Helper()
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("Exec(%q): %v", stmt, err)
		}
	}
}

func TestIntegration_RemoveFieldRebuild_PreservesData(t *testing.T) {
	t.Parallel()
	db := openMemDB(t)
	defer db.Close()

	d := sqlite.New()
	ps := personState()

	create, err := CreateModel{Name: "Person", Table: "person", Fields: personFields()}.SQL(d, NewProjectState())
	if err != nil {
		t.Fatalf("CreateModel SQL: %v", err)
	}
	execAll(t, db, create)

	if _, err := db.Exec(`INSERT INTO "person" (name, age) VALUES (?, ?)`, "Alice", 30); err != nil {
		t.Fatalf("insert Alice: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO "person" (name, age) VALUES (?, ?)`, "Bob", 25); err != nil {
		t.Fatalf("insert Bob: %v", err)
	}

	rebuild, err := RemoveField{Model: "Person", Field: "Age"}.SQL(d, ps)
	if err != nil {
		t.Fatalf("RemoveField SQL: %v", err)
	}

	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	for _, stmt := range rebuild {
		if _, err := tx.Exec(stmt); err != nil {
			_ = tx.Rollback()
			t.Fatalf("tx.Exec(%q): %v", stmt, err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	// The age column must be gone.
	if _, err := db.Exec(`SELECT age FROM "person"`); err == nil {
		t.Error("SELECT age succeeded; column was not dropped")
	}

	// The two rows must survive with id and name intact.
	rows, err := db.Query(`SELECT id, name FROM "person" ORDER BY id`)
	if err != nil {
		t.Fatalf("query survivors: %v", err)
	}
	defer rows.Close()

	type row struct {
		id   int64
		name string
	}
	var got []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.name); err != nil {
			t.Fatalf("scan: %v", err)
		}
		got = append(got, r)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows.Err: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("row count = %d, want 2", len(got))
	}
	if got[0].name != "Alice" || got[1].name != "Bob" {
		t.Errorf("names = [%q, %q], want [Alice, Bob]", got[0].name, got[1].name)
	}
	if got[0].id != 1 || got[1].id != 2 {
		t.Errorf("ids = [%d, %d], want [1, 2]", got[0].id, got[1].id)
	}
}

func TestIntegration_AlterFieldRebuild_PreservesData(t *testing.T) {
	t.Parallel()
	db := openMemDB(t)
	defer db.Close()

	d := sqlite.New()
	ps := personState()

	create, err := CreateModel{Name: "Person", Table: "person", Fields: personFields()}.SQL(d, NewProjectState())
	if err != nil {
		t.Fatalf("CreateModel SQL: %v", err)
	}
	execAll(t, db, create)

	if _, err := db.Exec(`INSERT INTO "person" (name, age) VALUES (?, ?)`, "Carol", 40); err != nil {
		t.Fatalf("insert Carol: %v", err)
	}

	rebuild, err := AlterField{
		Model: "Person",
		Field: FieldState{Name: "Name", Column: "name", Kind: orm.KindChar, MaxLength: 200},
	}.SQL(d, ps)
	if err != nil {
		t.Fatalf("AlterField SQL: %v", err)
	}

	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	for _, stmt := range rebuild {
		if _, err := tx.Exec(stmt); err != nil {
			_ = tx.Rollback()
			t.Fatalf("tx.Exec(%q): %v", stmt, err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	var (
		id   int64
		name string
		age  int64
	)
	if err := db.QueryRow(`SELECT id, name, age FROM "person"`).Scan(&id, &name, &age); err != nil {
		t.Fatalf("query survivor: %v", err)
	}
	if id != 1 || name != "Carol" || age != 40 {
		t.Errorf("row = (%d, %q, %d), want (1, Carol, 40)", id, name, age)
	}
}
