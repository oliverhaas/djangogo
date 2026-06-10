package postgres_test

import (
	"strings"
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/orm/backends/postgres"
)

// presetRow declares default= tags; the DDL must NOT emit a DEFAULT
// clause because defaults are applied Go-side at INSERT, mirroring Django.
type presetRow struct {
	ID    int64
	Count int    `orm:"default=7"`
	Label string `orm:"default=draft;max_length=20"`
}

func TestCreateTableSQL_NoDefaultClause(t *testing.T) {
	r := orm.NewRegistry()
	m := mustRegister(t, r, &presetRow{})
	got := postgres.New().CreateTableSQL(m)
	if strings.Contains(strings.ToUpper(got), "DEFAULT") {
		t.Errorf("CreateTableSQL emitted a DEFAULT clause (defaults are Go-side, not DDL):\n%s", got)
	}
}

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
	d := postgres.New()

	if got := d.Name(); got != "postgres" {
		t.Errorf("Name() = %q, want %q", got, "postgres")
	}

	if got := d.Placeholder(1); got != "$1" {
		t.Errorf("Placeholder(1) = %q, want %q", got, "$1")
	}

	if got := d.Placeholder(3); got != "$3" {
		t.Errorf("Placeholder(3) = %q, want %q", got, "$3")
	}

	if got := d.Quote("title"); got != `"title"` {
		t.Errorf("Quote(%q) = %q, want %q", "title", got, `"title"`)
	}

	if got := d.Quote(`we"ird`); got != `"we""ird"` {
		t.Errorf("Quote(%q) = %q, want %q", `we"ird`, got, `"we""ird"`)
	}

	if !d.SupportsReturning() {
		t.Error("SupportsReturning() must be true for PostgreSQL")
	}
}

func TestCreateTableSQL_Author(t *testing.T) {
	r := orm.NewRegistry()
	authorModel := mustRegister(t, r, &Author{})
	d := postgres.New()

	want := `CREATE TABLE "author" ("id" BIGSERIAL PRIMARY KEY, "name" VARCHAR(100) NOT NULL, "email" VARCHAR(255) NOT NULL UNIQUE)`
	got := d.CreateTableSQL(authorModel)

	if got != want {
		t.Errorf("CreateTableSQL(Author):\ngot:  %s\nwant: %s", got, want)
	}
}

func TestCreateTableSQL_Tag(t *testing.T) {
	r := orm.NewRegistry()
	tagModel := mustRegister(t, r, &Tag{})
	d := postgres.New()

	got := d.CreateTableSQL(tagModel)

	const wantSlug = `"slug" VARCHAR(50) NOT NULL PRIMARY KEY`
	const wantLabel = `"label" VARCHAR(255) NOT NULL`

	if got != `CREATE TABLE "tag" (`+wantSlug+`, `+wantLabel+`)` {
		t.Errorf("CreateTableSQL(Tag):\ngot:  %s", got)
	}
}

func TestColumnType_Kinds(t *testing.T) {
	r := orm.NewRegistry()
	eventModel := mustRegister(t, r, &Event{})
	d := postgres.New()

	tests := []struct {
		field string
		want  string
	}{
		{"ID", "BIGSERIAL PRIMARY KEY"},
		{"Happened", "TIMESTAMPTZ"},
		{"Published", "BOOLEAN"},
		{"Notes", "TEXT"},
	}

	for _, tc := range tests {
		f, ok := eventModel.FieldByName(tc.field)
		if !ok {
			t.Fatalf("field %q not found in Event model", tc.field)
		}

		got := d.ColumnType(f)
		if got != tc.want {
			t.Errorf("ColumnType(%q) = %q, want %q", tc.field, got, tc.want)
		}
	}
}
