package migrations

import (
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/orm/backends/postgres"
	"github.com/oliverhaas/djangogo/orm/backends/sqlite"
)

// articleFields returns a Person-like Article model state with an FK to author.
func articleFields() []FieldState {
	return []FieldState{
		{Name: "ID", Column: "id", Kind: orm.KindAuto, PrimaryKey: true},
		{Name: "Title", Column: "title", Kind: orm.KindChar, MaxLength: 200},
		{
			Name: "Author", Column: "author_id", Kind: orm.KindInt,
			RelKind: orm.RelFK, RelTargetTable: "author", RelTargetColumn: "id",
		},
	}
}

func TestCreateTableSQL_SQLiteEmitsForeignKey(t *testing.T) {
	t.Parallel()
	got := createTableSQL(sqlite.New(), "article", articleFields())
	want := `CREATE TABLE "article" ("id" INTEGER PRIMARY KEY AUTOINCREMENT, ` +
		`"title" VARCHAR(200) NOT NULL, "author_id" INTEGER NOT NULL, ` +
		`FOREIGN KEY ("author_id") REFERENCES "author" ("id"))`
	if got != want {
		t.Errorf("sqlite createTableSQL:\n got %q\nwant %q", got, want)
	}
}

func TestCreateTableSQL_PostgresEmitsForeignKey(t *testing.T) {
	t.Parallel()
	got := createTableSQL(postgres.New(), "article", articleFields())
	want := `CREATE TABLE "article" ("id" BIGSERIAL PRIMARY KEY, ` +
		`"title" VARCHAR(200) NOT NULL, "author_id" BIGINT NOT NULL, ` +
		`FOREIGN KEY ("author_id") REFERENCES "author" ("id"))`
	if got != want {
		t.Errorf("postgres createTableSQL:\n got %q\nwant %q", got, want)
	}
}

// TestCreateTableSQL_EmitsOnDelete confirms a FK FieldState carrying an
// on_delete action renders the ON DELETE clause.
func TestCreateTableSQL_EmitsOnDelete(t *testing.T) {
	t.Parallel()
	fields := articleFields()
	fields[2].RelOnDelete = orm.OnDeleteCascade
	got := createTableSQL(sqlite.New(), "article", fields)
	if !strings.Contains(got, `REFERENCES "author" ("id") ON DELETE CASCADE`) {
		t.Errorf("createTableSQL missing ON DELETE CASCADE:\n%s", got)
	}
}

// TestCreateTableSQL_NoForeignKeyForScalars confirms a scalar-only model emits
// no FOREIGN KEY clause.
func TestCreateTableSQL_NoForeignKeyForScalars(t *testing.T) {
	t.Parallel()
	got := createTableSQL(sqlite.New(), "person", personFields())
	if strings.Contains(got, "FOREIGN KEY") {
		t.Errorf("scalar-only table should not contain FOREIGN KEY, got %q", got)
	}
}
