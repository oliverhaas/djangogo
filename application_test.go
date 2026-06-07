// application_test.go
package djangogo

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/oliverhaas/djangogo/conf"
	"github.com/oliverhaas/djangogo/orm"
)

// TestModel is a minimal model used to exercise the model registry, migration,
// and migrate wiring end to end.
type TestModel struct {
	ID    int64
	Title string
}

// testApp implements both apps.Config and apps.ModelProvider.
type testApp struct{ name string }

func (a testApp) Name() string  { return a.name }
func (a testApp) Models() []any { return []any{&TestModel{}} }

// blogAuthor and blogArticle exercise a forward foreign key between two models
// declared by the same app, so New must Resolve relations before Freeze.
type blogAuthor struct {
	ID   int64
	Name string
}

type blogArticle struct {
	ID     int64
	Title  string `orm:"max_length=200"`
	Author orm.FK[blogAuthor]
}

// relApp declares Author and Article(FK[Author]); both targets are registered.
type relApp struct{ name string }

func (a relApp) Name() string  { return a.name }
func (a relApp) Models() []any { return []any{&blogAuthor{}, &blogArticle{}} }

// danglingFK has an FK to a model the app never registers, so Resolve must fail.
type danglingFK struct {
	ID     int64
	Author orm.FK[blogAuthor]
}

// danglingApp declares only danglingFK, whose FK target blogAuthor is unregistered.
type danglingApp struct{ name string }

func (a danglingApp) Name() string  { return a.name }
func (a danglingApp) Models() []any { return []any{&danglingFK{}} }

func TestNewWiresRegistries(t *testing.T) {
	app, err := New(conf.Settings{SecretKey: "k"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if app.Settings.Host != "127.0.0.1" {
		t.Errorf("defaults not applied; Host = %q", app.Settings.Host)
	}
	if app.Handler == nil {
		t.Error("Handler should be set")
	}
	// built-in commands are registered
	names := app.Commands.Names()
	found := map[string]bool{}
	for _, n := range names {
		found[n] = true
	}
	if !found["runserver"] || !found["version"] {
		t.Errorf("built-in commands missing: %v", names)
	}
}

func TestNewRejectsBadSettings(t *testing.T) {
	if _, err := New(conf.Settings{}); err == nil {
		t.Error("New should reject settings without a SecretKey")
	}
}

func TestExecuteRunsCommand(t *testing.T) {
	app, err := New(conf.Settings{SecretKey: "k"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := app.Execute([]string{"version"}); err != nil {
		t.Errorf("Execute(version): %v", err)
	}
}

func hasCommand(app *Application, name string) bool {
	for _, n := range app.Commands.Names() {
		if n == name {
			return true
		}
	}
	return false
}

func TestNewWiresModelsAndDB(t *testing.T) {
	app, err := New(conf.Settings{
		SecretKey: "k",
		Database:  conf.Database{Driver: "sqlite", DSN: "file:apptest?mode=memory&cache=shared"},
	}, testApp{name: "blog"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if app.Models == nil {
		t.Fatal("Models registry should be set")
	}
	if _, ok := app.Models.Get("TestModel"); !ok {
		t.Error("Models registry should contain TestModel")
	}
	if app.DB == nil {
		t.Error("DB should be set when a DSN is configured")
	}
	if app.MigrationsApp != "blog" {
		t.Errorf("MigrationsApp = %q, want blog", app.MigrationsApp)
	}
	if !hasCommand(app, "makemigrations") || !hasCommand(app, "migrate") {
		t.Errorf("migration commands missing: %v", app.Commands.Names())
	}
}

func TestNewWithoutDSNLeavesDBNil(t *testing.T) {
	app, err := New(conf.Settings{SecretKey: "k"}, testApp{name: "blog"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if app.DB != nil {
		t.Error("DB should be nil when no DSN is configured")
	}
	if !hasCommand(app, "makemigrations") || !hasCommand(app, "migrate") {
		t.Errorf("migration commands missing: %v", app.Commands.Names())
	}
}

func TestNewRejectsUnknownDriver(t *testing.T) {
	_, err := New(conf.Settings{
		SecretKey: "k",
		Database:  conf.Database{Driver: "oracle", DSN: "whatever"},
	})
	if err == nil {
		t.Error("New should reject an unknown database driver with a DSN")
	}
}

func TestMakemigrationsAndMigrate(t *testing.T) {
	app, err := New(conf.Settings{
		SecretKey: "k",
		Database:  conf.Database{Driver: "sqlite", DSN: "file:m3exit?mode=memory&cache=shared"},
	}, testApp{name: "blog"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Point migration output at a temp dir and capture command output.
	app.MigrationsDir = t.TempDir()
	var buf bytes.Buffer
	app.Out = &buf

	before := len(app.Migrations.All())

	// makemigrations creates the 0001_initial migration.
	if err := app.Execute([]string{"makemigrations"}); err != nil {
		t.Fatalf("makemigrations: %v", err)
	}
	migs := app.Migrations.All()
	if len(migs) != before+1 {
		t.Fatalf("registry grew by %d, want 1", len(migs)-before)
	}
	created := migs[len(migs)-1]
	if created.Name != "0001_initial" {
		t.Errorf("migration name = %q, want 0001_initial", created.Name)
	}
	wantPath := filepath.Join(app.MigrationsDir, "blog", "0001_initial.go")
	if _, statErr := os.Stat(wantPath); statErr != nil {
		t.Errorf("migration file not written at %q: %v", wantPath, statErr)
	}

	// migrate applies it; the table should now exist.
	if err := app.Execute([]string{"migrate"}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	rows, err := orm.Query[TestModel](app.DB).All(context.Background())
	if err != nil {
		t.Fatalf("query after migrate: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("expected empty table, got %d rows", len(rows))
	}

	// Re-running makemigrations detects no changes and does not grow the registry.
	buf.Reset()
	if err := app.Execute([]string{"makemigrations"}); err != nil {
		t.Fatalf("makemigrations (rerun): %v", err)
	}
	if got := len(app.Migrations.All()); got != len(migs) {
		t.Errorf("registry grew on rerun: %d -> %d", len(migs), got)
	}
	if out := buf.String(); out != "No changes detected\n" {
		t.Errorf("rerun output = %q, want %q", out, "No changes detected\n")
	}
}

// TestNewResolvesRelations verifies that New resolves model relations before
// freezing, so an FK field's Rel.Target is set and the relational model's
// CREATE TABLE emits a FOREIGN KEY constraint.
func TestNewResolvesRelations(t *testing.T) {
	app, err := New(conf.Settings{
		SecretKey: "k",
		Database:  conf.Database{Driver: "sqlite", DSN: "file:relapp?mode=memory&cache=shared"},
	}, relApp{name: "blog"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	article, ok := app.Models.Get("blogArticle")
	if !ok {
		t.Fatal("Models registry should contain blogArticle")
	}
	author, ok := app.Models.Get("blogAuthor")
	if !ok {
		t.Fatal("Models registry should contain blogAuthor")
	}

	fk, ok := article.FieldByName("Author")
	if !ok {
		t.Fatal("blogArticle should have an Author field")
	}
	if fk.Rel == nil {
		t.Fatal("Author field should carry a relation")
	}
	if fk.Rel.Target != author {
		t.Fatalf("Author Rel.Target = %v, want blogAuthor model (relations not resolved)", fk.Rel.Target)
	}

	// The resolved relation must surface as a FOREIGN KEY in the table DDL.
	if err := app.DB.CreateTable(context.Background(), author); err != nil {
		t.Fatalf("CreateTable(blogAuthor): %v", err)
	}
	if err := app.DB.CreateTable(context.Background(), article); err != nil {
		t.Fatalf("CreateTable(blogArticle): %v", err)
	}
	rows, err := app.DB.SQL().QueryContext(context.Background(), `PRAGMA foreign_key_list("blogarticle")`)
	if err != nil {
		t.Fatalf("PRAGMA foreign_key_list: %v", err)
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		t.Fatal("blogarticle table has no foreign key after CreateTable")
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("foreign_key_list rows err: %v", err)
	}
}

// TestNewRejectsUnresolvableRelation verifies that New returns an error when a
// model's FK targets a model that is never registered.
func TestNewRejectsUnresolvableRelation(t *testing.T) {
	_, err := New(conf.Settings{SecretKey: "k"}, danglingApp{name: "blog"})
	if err == nil {
		t.Fatal("New should reject an app whose FK target is unregistered")
	}
}
