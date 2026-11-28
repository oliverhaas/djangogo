package djangotest

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/orm"
)

// shelfAuthor and shelfBook are a referenced model and the model that
// references it, for the tests of the database helpers.
type shelfAuthor struct {
	ID   int64
	Name string `orm:"max_length=100"`
}

type shelfBook struct {
	ID     int64
	Title  string `orm:"max_length=100"`
	Author orm.FK[shelfAuthor]
}

// shelfModels lists the referencing model first, so that creating the tables
// in registration order would fail on PostgreSQL.
var shelfModels = []any{App("shelf", &shelfBook{}, &shelfAuthor{})}

func TestAppRegistersModelsUnderLabel(t *testing.T) {
	reg := NewRegistry(t, shelfModels...)
	models := reg.AppModels("shelf")
	if len(models) != 2 {
		t.Fatalf("app shelf has %d models, want 2", len(models))
	}
	for _, m := range models {
		if m.App() != "shelf" {
			t.Errorf("model %s is in app %q, want shelf", m.Name(), m.App())
		}
		if !strings.HasPrefix(m.Table(), "shelf_") {
			t.Errorf("model %s has table %q, want the prefix shelf_", m.Name(), m.Table())
		}
	}
}

func TestNewRegistryBareModelHasNoAppLabel(t *testing.T) {
	reg := NewRegistry(t, &shelfAuthor{})
	models := reg.Models()
	if len(models) != 1 {
		t.Fatalf("registry has %d models, want 1", len(models))
	}
	if models[0].App() != "" {
		t.Errorf("a bare model is in app %q, want none", models[0].App())
	}
}

// roundTrip creates an author and a book and reads the book back through the
// foreign key, which needs both tables and the reference between them.
func roundTrip(t *testing.T, db *orm.DB) {
	t.Helper()
	ctx := context.Background()
	author := shelfAuthor{Name: "Ursula"}
	if err := orm.Query[shelfAuthor](db).Create(ctx, &author); err != nil {
		t.Fatalf("create author: %v", err)
	}
	book := shelfBook{Title: "The Dispossessed"}
	book.Author.SetPK(author.ID)
	if err := orm.Query[shelfBook](db).Create(ctx, &book); err != nil {
		t.Fatalf("create book: %v", err)
	}
	got, err := orm.Query[shelfBook](db).Get(ctx, "author_id", author.ID)
	if err != nil {
		t.Fatalf("get book: %v", err)
	}
	AssertEqual(t, got.Title, "The Dispossessed")
}

func TestNewDBCreatesTables(t *testing.T) {
	roundTrip(t, NewDB(t, shelfModels...))
}

func TestNewDBIsPrivate(t *testing.T) {
	first, second := NewDB(t, shelfModels...), NewDB(t, shelfModels...)
	roundTrip(t, first)

	n, err := orm.Query[shelfAuthor](second).Count(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("a second database sees %d rows of the first, want 0", n)
	}
}

func TestNewPostgresDBCreatesTablesInPrivateSchema(t *testing.T) {
	first, second := NewPostgresDB(t, shelfModels...), NewPostgresDB(t, shelfModels...)
	roundTrip(t, first)

	n, err := orm.Query[shelfAuthor](second).Count(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("a second schema sees %d rows of the first, want 0", n)
	}
}

func TestEachDBRunsEveryDialect(t *testing.T) {
	var dialects []string
	EachDB(t, shelfModels, func(t *testing.T, db *orm.DB) {
		dialects = append(dialects, db.Dialect().Name())
		roundTrip(t, db)
	})
	want := []string{"sqlite"}
	if os.Getenv(PostgresDSNEnv) != "" {
		want = append(want, "postgres")
	}
	AssertSequenceEqual(t, dialects, want)
}

func TestSchemaDSN(t *testing.T) {
	cases := []struct{ dsn, want string }{
		{"postgres://u:p@localhost:5432/db?sslmode=disable", "postgres://u:p@localhost:5432/db?search_path=dt_x%2Cpublic&sslmode=disable"},
		{"postgresql://localhost/db", "postgresql://localhost/db?search_path=dt_x%2Cpublic"},
		{"host=localhost dbname=db", "host=localhost dbname=db search_path=dt_x,public"},
	}
	for _, tc := range cases {
		got, err := schemaDSN(tc.dsn, "dt_x")
		if err != nil {
			t.Errorf("schemaDSN(%q): %v", tc.dsn, err)
			continue
		}
		AssertEqual(t, got, tc.want)
	}
}
