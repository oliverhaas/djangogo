package djangogo_test

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/oliverhaas/djangogo"
	"github.com/oliverhaas/djangogo/conf"
	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/manage"
)

// TestNewOpensPostgreSQL opens the PostgreSQL database of the test DSN under
// every driver name Djan-Go-Go accepts for it, and Close releases it.
func TestNewOpensPostgreSQL(t *testing.T) {
	dsn := os.Getenv(djangotest.PostgresDSNEnv)
	if dsn == "" {
		t.Skipf("%s is not set", djangotest.PostgresDSNEnv)
	}
	for _, driver := range []string{"postgres", "postgresql", "pgx"} {
		t.Run(driver, func(t *testing.T) {
			app, err := djangogo.New(conf.Settings{SecretKey: "k", Database: conf.Database{Driver: driver, DSN: dsn}})
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			djangotest.AssertEqual(t, app.DB.Dialect().Name(), "postgres")
			var one int
			if err := app.DB.SQL().QueryRowContext(context.Background(), "SELECT 1").Scan(&one); err != nil {
				t.Fatalf("query: %v", err)
			}
			if err := app.Close(); err != nil {
				t.Fatalf("Close: %v", err)
			}
			if err := app.DB.SQL().PingContext(context.Background()); err == nil {
				t.Error("the database still answers after Close")
			}
		})
	}
}

// TestNewOpensDatabaseURL opens the database a DATABASE_URL-style value
// names, as a project reads it from the environment.
func TestNewOpensDatabaseURL(t *testing.T) {
	db, err := conf.ParseDatabaseURL("sqlite://:memory:")
	if err != nil {
		t.Fatalf("ParseDatabaseURL: %v", err)
	}
	app, err := djangogo.New(conf.Settings{SecretKey: "k", Database: db})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	djangotest.AssertEqual(t, app.DB.Dialect().Name(), "sqlite")
}

// TestNewRejectsUnknownDriverByName names the driver and the supported ones.
func TestNewRejectsUnknownDriverByName(t *testing.T) {
	_, err := djangogo.New(conf.Settings{SecretKey: "k", Database: conf.Database{Driver: "oracle", DSN: "x"}})
	if err == nil {
		t.Fatal("New accepted the driver oracle")
	}
	djangotest.AssertEqual(t, err.Error(), `djangogo: unsupported database driver "oracle" (want "sqlite" or "postgres")`)
}

// TestCloseReleasesDatabase closes the database once; a second Close and the
// Close of an application without a database do nothing.
func TestCloseReleasesDatabase(t *testing.T) {
	app, err := djangogo.New(conf.Settings{SecretKey: "k", Database: conf.Database{Driver: "sqlite", DSN: "file:closetest?mode=memory&cache=shared"}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := app.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := app.DB.SQL().PingContext(context.Background()); err == nil {
		t.Error("the database still answers after Close")
	}
	if err := app.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}

	bare, err := djangogo.New(conf.Settings{SecretKey: "k"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := bare.Close(); err != nil {
		t.Errorf("Close without a database: %v", err)
	}
}

// tag returns middleware that appends name to the X-Order header of the
// response before calling next, so the header lists the middleware in the
// order they ran.
func tag(name string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Add("X-Order", name)
			next.ServeHTTP(w, r)
		})
	}
}

// TestUseAddsProjectMiddleware runs the middleware of Use inside the default
// stack, in the order given, across several calls.
func TestUseAddsProjectMiddleware(t *testing.T) {
	app, err := djangogo.New(conf.Settings{SecretKey: "k", Debug: true})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	app.Use(tag("first"), tag("second"))
	app.Use(tag("third"))

	resp := djangotest.NewClient(t, app.Handler, djangotest.WithHost("localhost")).Get("/")
	djangotest.AssertEqual(t, resp.StatusCode, http.StatusOK)
	djangotest.AssertEqual(t, resp.Header.Values("X-Order"), []string{"first", "second", "third"})
	// The default stack ran around them: clickjacking protection is in it.
	djangotest.AssertEqual(t, resp.Header.Get("X-Frame-Options"), "DENY")
}

// TestRecoverWrapsEverything turns a panic in project middleware into a 500
// and logs it to Err; with Debug on the page shows the panic.
func TestRecoverWrapsEverything(t *testing.T) {
	for _, debug := range []bool{false, true} {
		t.Run(fmt.Sprintf("debug=%v", debug), func(t *testing.T) {
			app, err := djangogo.New(conf.Settings{SecretKey: "k", Debug: debug, AllowedHosts: []string{"testserver"}})
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			var log bytes.Buffer
			app.Err = &log
			app.Use(func(http.Handler) http.Handler {
				return http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("boom in middleware") })
			})

			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Host = "testserver"
			app.Handler.ServeHTTP(rec, req)

			djangotest.AssertEqual(t, rec.Code, http.StatusInternalServerError)
			checkContains(t, log.String(), "boom in middleware")
			if got := strings.Contains(rec.Body.String(), "boom in middleware"); got != debug {
				t.Errorf("the page shows the panic: %v, want %v", got, debug)
			}
		})
	}
}

// TestDefaultStackValidatesHost answers 400 for a host that is not allowed,
// Django's DisallowedHost, before any other middleware or view runs.
func TestDefaultStackValidatesHost(t *testing.T) {
	app, err := djangogo.New(conf.Settings{SecretKey: "k", AllowedHosts: []string{"example.com"}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	app.Err = &bytes.Buffer{}
	app.Use(tag("project"))

	resp := djangotest.NewClient(t, app.Handler, djangotest.WithHost("evil.example.org")).Get("/")
	djangotest.AssertEqual(t, resp.StatusCode, http.StatusBadRequest)
	djangotest.AssertEqual(t, len(resp.Header.Values("X-Order")), 0)

	resp = djangotest.NewClient(t, app.Handler, djangotest.WithHost("example.com")).Get("/")
	djangotest.AssertEqual(t, resp.StatusCode, http.StatusOK)
}

// echoCommand prints its name and arguments, so a test sees which command
// of several with the same name ran.
type echoCommand struct {
	name, owner string
	out         *bytes.Buffer
}

// Name returns the command name.
func (c *echoCommand) Name() string { return c.name }

// Help returns the help text.
func (c *echoCommand) Help() string { return "Echoes its owner." }

// Run prints the owner and the arguments.
func (c *echoCommand) Run(args []string) error {
	fmt.Fprintf(c.out, "%s %s\n", c.owner, strings.Join(args, " "))
	return nil
}

// commandApp is an app with commands and template and static trees.
type commandApp struct {
	name     string
	commands []manage.Command
	files    fstest.MapFS
}

// Name returns the app name.
func (a commandApp) Name() string { return a.name }

// Commands returns the commands of the app.
func (a commandApp) Commands() []manage.Command { return a.commands }

// Templates returns the template tree of the app.
func (a commandApp) Templates() fs.FS { return a.files }

// Static returns the static tree of the app.
func (a commandApp) Static() fs.FS { return a.files }

// TestAppCommandsOverride registers the commands of the apps under their
// label: the app listed first wins over a later one and over a built-in
// command of the same name, as in Django.
func TestAppCommandsOverride(t *testing.T) {
	var out bytes.Buffer
	first := commandApp{name: "first", commands: []manage.Command{
		&echoCommand{name: "greet", owner: "first", out: &out},
		&echoCommand{name: "shell", owner: "first", out: &out},
	}}
	second := commandApp{name: "second", commands: []manage.Command{
		&echoCommand{name: "greet", owner: "second", out: &out},
		&echoCommand{name: "farewell", owner: "second", out: &out},
	}}
	app, err := djangogo.New(conf.Settings{SecretKey: "k"}, first, second)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	app.Out, app.Err = &bytes.Buffer{}, &bytes.Buffer{}

	for _, args := range [][]string{{"greet", "a"}, {"shell"}, {"farewell", "b"}} {
		if code := app.Run(args); code != 0 {
			t.Fatalf("Run(%v) = %d; Err: %s", args, code, app.Err)
		}
	}
	djangotest.AssertEqual(t, out.String(), "first a\nfirst \nsecond b\n")
	djangotest.AssertEqual(t, app.Commands.App("greet"), "first")
	djangotest.AssertEqual(t, app.Commands.App("farewell"), "second")
}

// TestAppTemplatesAndStatic finds the templates and the static files of the
// apps, the templates only with Settings.Templates.AppDirs, which is on by
// default.
func TestAppTemplatesAndStatic(t *testing.T) {
	files := fstest.MapFS{"blog/hello.html": {Data: []byte("Hello {{ name }}!")}}
	blog := commandApp{name: "blog", files: files}

	app, err := djangogo.New(conf.Settings{SecretKey: "k"}, blog)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	got, err := app.Templates.Render("blog/hello.html", map[string]any{"name": "Go"})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	djangotest.AssertEqual(t, got, "Hello Go!")
	if _, err := fs.Stat(app.Static, "blog/hello.html"); err != nil {
		t.Errorf("the static file of the app is not found: %v", err)
	}

	// A literal cannot switch off a setting whose default is true; Defaults can.
	s := conf.Defaults()
	s.SecretKey = "k"
	s.Templates.AppDirs = false
	app, err = djangogo.New(s, blog)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := app.Templates.Render("blog/hello.html", nil); err == nil {
		t.Error("an app template is found without Templates.AppDirs")
	}
}

// TestRunPrintsErrorsAndExitCodes maps the errors of a command to Django's
// exit codes through Run, the entry point of a main function.
func TestRunPrintsErrorsAndExitCodes(t *testing.T) {
	app, err := djangogo.New(conf.Settings{SecretKey: "k"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	var errOut bytes.Buffer
	app.Out, app.Err = &bytes.Buffer{}, &errOut

	djangotest.AssertEqual(t, app.Run([]string{"check", "--no-such-flag"}), 2)
	checkContains(t, errOut.String(), "error: ")

	errOut.Reset()
	djangotest.AssertEqual(t, app.Run([]string{"check", "nosuchapp"}), 1)
	djangotest.AssertEqual(t, errOut.String(), "CommandError: No installed app with label 'nosuchapp'.\n")

	errOut.Reset()
	djangotest.AssertEqual(t, app.Run([]string{"nosuchcommand"}), 1)
	checkContains(t, errOut.String(), "Unknown command: 'nosuchcommand'")
}
