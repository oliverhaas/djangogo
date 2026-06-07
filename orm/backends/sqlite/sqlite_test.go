package sqlite_test

import (
	"context"
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/orm/backends/sqlite"
)

// -- test models --

type Author struct {
	ID    int64
	Name  string `orm:"max_length=100"`
	Email string `orm:"unique"`
}

type Tag struct {
	Slug  string `orm:"pk;max_length=50"`
	Label string
}

type Event struct {
	ID        int64
	Happened  time.Time `orm:"null"`
	Published bool      `orm:"null"`
	Notes     string    `orm:"type=text;null"`
}

func mustRegister(t *testing.T, r *orm.Registry, v any) *orm.Model {
	t.Helper()

	m, err := r.Register(v)
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	return m
}

func TestDialectBasics(t *testing.T) {
	d := sqlite.New()

	if got := d.Name(); got != "sqlite" {
		t.Errorf("Name() = %q, want %q", got, "sqlite")
	}

	for _, n := range []int{1, 5} {
		if got := d.Placeholder(n); got != "?" {
			t.Errorf("Placeholder(%d) = %q, want %q", n, got, "?")
		}
	}

	if got := d.Quote("title"); got != `"title"` {
		t.Errorf("Quote(%q) = %q, want %q", "title", got, `"title"`)
	}

	if d.SupportsReturning() {
		t.Error("SupportsReturning() must be false for SQLite")
	}
}

func TestCreateTableSQL_Author(t *testing.T) {
	r := orm.NewRegistry()
	authorModel := mustRegister(t, r, &Author{})
	d := sqlite.New()

	want := `CREATE TABLE "author" ("id" INTEGER PRIMARY KEY AUTOINCREMENT, "name" VARCHAR(100) NOT NULL, "email" VARCHAR(255) NOT NULL UNIQUE)`
	got := d.CreateTableSQL(authorModel)

	if got != want {
		t.Errorf("CreateTableSQL(Author):\ngot:  %s\nwant: %s", got, want)
	}
}

func TestCreateTableSQL_Tag(t *testing.T) {
	r := orm.NewRegistry()
	tagModel := mustRegister(t, r, &Tag{})
	d := sqlite.New()

	got := d.CreateTableSQL(tagModel)

	const wantSlug = `"slug" VARCHAR(50) NOT NULL PRIMARY KEY`
	const wantLabel = `"label" VARCHAR(255) NOT NULL`

	if got != `CREATE TABLE "tag" (`+wantSlug+`, `+wantLabel+`)` {
		t.Errorf("CreateTableSQL(Tag):\ngot:  %s", got)
	}
}

func TestColumnType_NullableFields(t *testing.T) {
	r := orm.NewRegistry()
	eventModel := mustRegister(t, r, &Event{})
	d := sqlite.New()

	tests := []struct {
		column string
		want   string
	}{
		{"happened", "DATETIME"},
		{"published", "BOOLEAN"},
		{"notes", "TEXT"},
	}

	for _, tc := range tests {
		f, ok := eventModel.FieldByName(columnToName(tc.column))
		if !ok {
			t.Fatalf("field %q not found in Event model", tc.column)
		}

		got := d.ColumnType(f)
		if got != tc.want {
			t.Errorf("ColumnType(%q) = %q, want %q", tc.column, got, tc.want)
		}
	}
}

// columnToName converts a lowercase column name back to the Go field name for
// Event (simple capitalise-first approach sufficient for this test).
func columnToName(col string) string {
	if len(col) == 0 {
		return col
	}

	return string(col[0]-32) + col[1:]
}

func TestIntegration(t *testing.T) {
	r := orm.NewRegistry()
	authorModel := mustRegister(t, r, &Author{})
	d := sqlite.New()

	db, err := sqlite.Open("file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	defer db.Close()

	ctx := context.Background()

	_, err = db.ExecContext(ctx, d.CreateTableSQL(authorModel))
	if err != nil {
		t.Fatalf("CREATE TABLE: %v", err)
	}

	_, err = db.ExecContext(ctx, `INSERT INTO "author"(name,email) VALUES('a','a@b.c')`)
	if err != nil {
		t.Fatalf("INSERT: %v", err)
	}

	var count int

	if err = db.QueryRowContext(ctx, `SELECT count(*) FROM "author"`).Scan(&count); err != nil {
		t.Fatalf("SELECT count: %v", err)
	}

	if count != 1 {
		t.Errorf("count = %d, want 1", count)
	}
}

// TestOpenSharedMemoryDSN confirms that the "file:name?mode=memory&cache=shared"
// form yields a working shared in-memory database whose state persists across
// separate queries on the returned *sql.DB (i.e. the pool is pinned to one conn).
func TestOpenSharedMemoryDSN(t *testing.T) {
	db, err := sqlite.Open("file:sharedmem?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	defer db.Close()

	ctx := context.Background()

	if _, err := db.ExecContext(ctx, `CREATE TABLE t (id INTEGER PRIMARY KEY)`); err != nil {
		t.Fatalf("CREATE TABLE: %v", err)
	}

	if _, err := db.ExecContext(ctx, `INSERT INTO t(id) VALUES (1)`); err != nil {
		t.Fatalf("INSERT: %v", err)
	}

	var count int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM t`).Scan(&count); err != nil {
		t.Fatalf("SELECT count: %v", err)
	}

	if count != 1 {
		t.Errorf("count = %d, want 1 (table did not persist across queries)", count)
	}
}
