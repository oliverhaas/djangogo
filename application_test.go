// application_test.go
package djangogo

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/admin"
	"github.com/oliverhaas/djangogo/auth"
	"github.com/oliverhaas/djangogo/conf"
	"github.com/oliverhaas/djangogo/migrations"
	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/urls"
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
	if !found["startproject"] || !found["startapp"] {
		t.Errorf("scaffold commands missing: %v", names)
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

// wiredPost is the model the wired test app registers with the admin.
type wiredPost struct {
	ID    int64
	Title string `orm:"max_length=100"`
}

func (p wiredPost) String() string { return p.Title }

// wiredApp implements Config, ModelProvider, URLProvider, and the admin
// RegisterAdmin hook, so New must mount its URLs and its admin.
type wiredApp struct{}

func (wiredApp) Name() string  { return "wired" }
func (wiredApp) Models() []any { return []any{&wiredPost{}} }

func (wiredApp) URLs() []urls.Route {
	home := func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("home-ok")) }
	return []urls.Route{urls.PathFunc("GET /{$}", home, "home")}
}

func (wiredApp) RegisterAdmin(site *admin.AdminSite) {
	admin.Register[wiredPost](site, admin.ModelAdmin{})
}

func TestNewBuildsWiredHandler(t *testing.T) {
	app, err := New(conf.Settings{
		SecretKey: "k",
		Database:  conf.Database{Driver: "sqlite", DSN: "file:wiredhandler?mode=memory&cache=shared"},
	}, wiredApp{}, auth.App{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// The URLProvider's home route is mounted at the site root.
	rec := httptest.NewRecorder()
	app.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET / status = %d, want 200", rec.Code)
	}
	if got := rec.Body.String(); got != "home-ok" {
		t.Errorf("GET / body = %q, want %q", got, "home-ok")
	}

	// The admin is mounted; an anonymous request is redirected to its login page
	// by the staff gate, proving the sessions -> csrf -> auth chain is wired.
	rec2 := httptest.NewRecorder()
	app.Handler.ServeHTTP(rec2, httptest.NewRequest(http.MethodGet, "/admin/", nil))
	if rec2.Code != http.StatusFound {
		t.Fatalf("GET /admin/ status = %d, want 302", rec2.Code)
	}
	if loc := rec2.Header().Get("Location"); !strings.Contains(loc, "/admin/login/") {
		t.Errorf("GET /admin/ Location = %q, want it to point at the admin login", loc)
	}
}

func TestNewWithoutURLProviderUsesLivenessHandler(t *testing.T) {
	// testApp provides models but neither URLs nor an admin registration, so the
	// handler falls back to the liveness stub.
	app, err := New(conf.Settings{SecretKey: "k"}, testApp{name: "blog"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	rec := httptest.NewRecorder()
	app.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET / status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "is running") {
		t.Errorf("GET / body = %q, want the liveness line", rec.Body.String())
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

// TestMakemigrationsWarnsOnLikelyRename verifies that when a field looks renamed
// (a removed field and an added field of the same type), makemigrations prints a
// data-loss warning rather than silently dropping the column. The prior state is
// seeded from the registry's own field inference -- Title renamed to Headline --
// so the test does not depend on the ORM's exact type mapping.
func TestMakemigrationsWarnsOnLikelyRename(t *testing.T) {
	app, err := New(conf.Settings{
		SecretKey: "k",
		Database:  conf.Database{Driver: "sqlite", DSN: "file:renamewarn?mode=memory&cache=shared"},
	}, testApp{name: "blog"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	app.MigrationsDir = t.TempDir()
	var buf bytes.Buffer
	app.Out = &buf

	// Find the registry model carrying the Title field and build a prior
	// migration identical to it except Title is named Headline.
	current := migrations.StateFromRegistry(app.Models)
	var modelName string
	var ms *migrations.ModelState
	for name, m := range current.Models {
		if _, ok := m.FieldByName("Title"); ok {
			modelName, ms = name, m
			break
		}
	}
	if ms == nil {
		t.Fatal("registry has no model with a Title field")
	}
	fields := make([]migrations.FieldState, len(ms.Fields))
	copy(fields, ms.Fields)
	for i := range fields {
		if fields[i].Name == "Title" {
			fields[i].Name = "Headline"
			fields[i].Column = "headline"
		}
	}
	app.Migrations.Add(migrations.Migration{
		App:  app.MigrationsApp,
		Name: "0001_initial",
		Operations: []migrations.Operation{
			migrations.CreateModel{Name: modelName, Table: ms.Table, Fields: fields},
		},
	})

	if err := app.Execute([]string{"makemigrations"}); err != nil {
		t.Fatalf("makemigrations: %v", err)
	}

	out := buf.String()
	wantRename := modelName + ".Headline -> " + modelName + ".Title"
	if !strings.Contains(out, "Warning:") || !strings.Contains(out, wantRename) {
		t.Errorf("expected rename warning for %q, got:\n%s", wantRename, out)
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
