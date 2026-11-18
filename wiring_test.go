package djangogo

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/apps"
	"github.com/oliverhaas/djangogo/auth"
	"github.com/oliverhaas/djangogo/conf"
	"github.com/oliverhaas/djangogo/contenttypes"
	"github.com/oliverhaas/djangogo/csrf"
	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/messages"
	"github.com/oliverhaas/djangogo/migrations"
	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/sessions"
	"github.com/oliverhaas/djangogo/urls"
)

// routeApp mounts its routes at the site root.
type routeApp struct{ routes []urls.Route }

func (routeApp) Name() string         { return "routes" }
func (a routeApp) URLs() []urls.Route { return a.routes }

// newWiredApp builds an application for the settings, with the secret key and
// the allowed host of the tests filled in, and puts the auth configuration
// back after the test.
func newWiredApp(t *testing.T, s conf.Settings, configs ...apps.Config) *Application {
	t.Helper()
	app, err := newWiredAppErr(t, s, configs...)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	return app
}

// newWiredAppErr is newWiredApp for a test that expects New to fail.
func newWiredAppErr(t *testing.T, s conf.Settings, configs ...apps.Config) (*Application, error) {
	t.Helper()
	prev := auth.CurrentConfig()
	t.Cleanup(func() { auth.Configure(prev) })
	if s.SecretKey == "" {
		s.SecretKey = "k"
	}
	if s.AllowedHosts == nil {
		s.AllowedHosts = []string{"example.com"}
	}
	return New(s, configs...)
}

// memoryDB returns an in-memory SQLite database of its own for the test.
func memoryDB(t *testing.T) conf.Database {
	name := strings.NewReplacer("/", "_", " ", "_").Replace(t.Name())
	return conf.Database{Driver: "sqlite", DSN: "file:" + name + "?mode=memory&cache=shared"}
}

// request sends a request to the handler of the application.
func request(app *Application, method, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	app.Handler.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
	return rec
}

// cookie returns the cookie of that name the response sets, or nil.
func cookie(rec *httptest.ResponseRecorder, name string) *http.Cookie {
	for _, c := range rec.Result().Cookies() {
		if c.Name == name {
			return c
		}
	}
	return nil
}

// TestSessionEngineSelectsTheStore keeps sessions in the database when the
// engine is "db", a database is configured and the sessions app is installed,
// and in signed cookies otherwise.
func TestSessionEngineSelectsTheStore(t *testing.T) {
	cases := []struct {
		name     string
		engine   string
		database bool
		configs  []apps.Config
		want     string
	}{
		{"db engine with the sessions app", "", true, []apps.Config{sessions.App{}}, "*sessions.DBStore"},
		{"db engine without the sessions app", "", true, nil, "*sessions.SignedCookieStore"},
		{"db engine without a database", "", false, []apps.Config{sessions.App{}}, "*sessions.SignedCookieStore"},
		{"signed cookies", conf.SessionEngineSignedCookies, true, []apps.Config{sessions.App{}}, "*sessions.SignedCookieStore"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := conf.Settings{SessionEngine: tc.engine}
			if tc.database {
				s.Database = memoryDB(t)
			}
			app := newWiredApp(t, s, tc.configs...)
			djangotest.AssertEqual(t, fmt.Sprintf("%T", app.sessions), tc.want)
		})
	}
}

// TestSessionSettingsShapeTheCookie passes the SESSION_COOKIE_* settings on to
// the session cookie.
func TestSessionSettingsShapeTheCookie(t *testing.T) {
	view := func(_ http.ResponseWriter, r *http.Request) {
		sess, _ := sessions.FromContext(r.Context())
		sess.Set("k", "v")
	}
	app := newWiredApp(t, conf.Settings{
		SessionCookieName:     "sid",
		SessionCookieSameSite: "Strict",
		SessionCookieSecure:   true,
		SessionCookiePath:     "/app/",
	}, routeApp{[]urls.Route{urls.PathFunc("GET /{$}", view, "home")}})

	c := cookie(request(app, http.MethodGet, "/"), "sid")
	if c == nil {
		t.Fatal("no sid cookie")
	}
	djangotest.AssertEqual(t, c.SameSite, http.SameSiteStrictMode)
	djangotest.AssertEqual(t, c.Secure, true)
	djangotest.AssertEqual(t, c.HttpOnly, true)
	djangotest.AssertEqual(t, c.Path, "/app/")
}

// TestCSRFSettingsAndExemptViews names the CSRF cookie after CSRF_COOKIE_NAME,
// rejects an unsafe request without a token and lets a view marked with
// csrf.Exempt through, which the middleware finds through the router.
func TestCSRFSettingsAndExemptViews(t *testing.T) {
	ok := func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "ok") }
	token := func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, csrf.GetToken(r)) }
	app := newWiredApp(t, conf.Settings{CSRFCookieName: "ctok"}, routeApp{[]urls.Route{
		urls.PathFunc("POST /form/{$}", ok, "form"),
		urls.Path("POST /hook/{$}", csrf.Exempt(http.HandlerFunc(ok)), "hook"),
		urls.PathFunc("GET /token/{$}", token, "token"),
	}})

	djangotest.AssertEqual(t, request(app, http.MethodPost, "/form/").Code, http.StatusForbidden)
	djangotest.AssertEqual(t, request(app, http.MethodPost, "/hook/").Code, http.StatusOK)
	if cookie(request(app, http.MethodGet, "/token/"), "ctok") == nil {
		t.Error("no ctok cookie")
	}
}

// TestMessagesMiddlewareIsInstalled lets a view add a message.
func TestMessagesMiddlewareIsInstalled(t *testing.T) {
	view := func(w http.ResponseWriter, r *http.Request) {
		if err := messages.Info(r, "saved"); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
	app := newWiredApp(t, conf.Settings{}, routeApp{[]urls.Route{urls.PathFunc("GET /{$}", view, "home")}})
	rec := request(app, http.MethodGet, "/")
	djangotest.AssertEqual(t, rec.Code, http.StatusOK)
}

// TestPasswordHashersSetting configures the hashers of PASSWORD_HASHERS, named
// by Django's class or by the algorithm, the first one preferred.
func TestPasswordHashersSetting(t *testing.T) {
	newWiredApp(t, conf.Settings{PasswordHashers: []string{
		"django.contrib.auth.hashers.MD5PasswordHasher",
		"pbkdf2_sha256",
	}})
	h, err := auth.GetHasher("default")
	if err != nil {
		t.Fatalf("GetHasher(default): %v", err)
	}
	djangotest.AssertEqual(t, h.Algorithm(), "md5")
	if _, err := auth.GetHasher("pbkdf2_sha256"); err != nil {
		t.Errorf("GetHasher(pbkdf2_sha256): %v", err)
	}
	if _, err := auth.GetHasher("argon2"); !errors.Is(err, auth.ErrUnknownHasher) {
		t.Errorf("GetHasher(argon2) = %v, want ErrUnknownHasher", err)
	}
}

// TestAuthPasswordValidatorsSetting configures the validators of
// AUTH_PASSWORD_VALIDATORS with their options.
func TestAuthPasswordValidatorsSetting(t *testing.T) {
	newWiredApp(t, conf.Settings{AuthPasswordValidators: []conf.PasswordValidator{
		{Name: "django.contrib.auth.password_validation.MinimumLengthValidator", Options: map[string]any{"min_length": 12}},
		{Name: "UserAttributeSimilarityValidator", Options: map[string]any{
			"user_attributes": []any{"username"}, "max_similarity": 0.5,
		}},
		{Name: "NumericPasswordValidator"},
	}})
	djangotest.AssertEqual(t, auth.ConfiguredPasswordValidators(), []auth.PasswordValidator{
		auth.MinimumLengthValidator{MinLength: 12},
		auth.UserAttributeSimilarityValidator{UserAttributes: []string{"username"}, MaxSimilarity: 0.5},
		auth.NumericPasswordValidator{},
	})
}

// TestCommonPasswordValidatorList reads password_list_path, plain or gzip
// compressed.
func TestCommonPasswordValidatorList(t *testing.T) {
	const list = "Hunter2\n  letmein \n"
	var gz bytes.Buffer
	zw := gzip.NewWriter(&gz)
	if _, err := io.WriteString(zw, list); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string][]byte{"passwords.txt": []byte(list), "passwords.txt.gz": gz.Bytes()} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), name)
			if err := os.WriteFile(path, content, 0o600); err != nil {
				t.Fatal(err)
			}
			newWiredApp(t, conf.Settings{AuthPasswordValidators: []conf.PasswordValidator{
				{Name: "CommonPasswordValidator", Options: map[string]any{"password_list_path": path}},
			}})
			v := auth.ConfiguredPasswordValidators()[0]
			for _, common := range []string{"hunter2", "LetMeIn"} {
				if err := v.Validate(common, nil); err == nil {
					t.Errorf("%s passed the list", common)
				}
			}
			if err := v.Validate("password", nil); err != nil {
				t.Errorf("password, not on the list, failed: %v", err)
			}
		})
	}
}

// TestUnknownAuthSettings refuses to start with a hasher, a validator or a
// validator option it does not know, or an option of the wrong type.
func TestUnknownAuthSettings(t *testing.T) {
	cases := []struct {
		name string
		s    conf.Settings
		want string
	}{
		{
			"hasher",
			conf.Settings{PasswordHashers: []string{"SHA1PasswordHasher"}},
			`The PASSWORD_HASHERS setting names an unknown hasher "SHA1PasswordHasher".`,
		},
		{
			"validator",
			conf.Settings{AuthPasswordValidators: []conf.PasswordValidator{{Name: "LengthValidator"}}},
			`The AUTH_PASSWORD_VALIDATORS setting names an unknown validator "LengthValidator".`,
		},
		{"option", conf.Settings{AuthPasswordValidators: []conf.PasswordValidator{
			{Name: "MinimumLengthValidator", Options: map[string]any{"max_length": 3}},
		}}, `AUTH_PASSWORD_VALIDATORS: MinimumLengthValidator has no option "max_length".`},
		{"option type", conf.Settings{AuthPasswordValidators: []conf.PasswordValidator{
			{Name: "MinimumLengthValidator", Options: map[string]any{"min_length": "12"}},
		}}, `AUTH_PASSWORD_VALIDATORS: the option "min_length" of MinimumLengthValidator must be an integer.`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := newWiredAppErr(t, tc.s)
			var ic *conf.ImproperlyConfigured
			if !errors.As(err, &ic) {
				t.Fatalf("New = %v, want ImproperlyConfigured", err)
			}
			djangotest.AssertEqual(t, ic.Msg, tc.want)
		})
	}
}

// TestContextProcessorsSetting runs the context processors of the setting,
// those of a new Django project without it, and refuses an unknown one.
func TestContextProcessorsSetting(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "page.html"), []byte("{{ request.path }}|{{ STATIC_URL }}"), 0o600); err != nil {
		t.Fatal(err)
	}
	render := func(t *testing.T, names []string) string {
		t.Helper()
		app := newWiredApp(t, conf.Settings{Templates: conf.Templates{Dirs: []string{dir}, ContextProcessors: names}})
		out, err := app.Templates.RenderRequest(httptest.NewRequest(http.MethodGet, "/where/", nil), "page.html", nil)
		if err != nil {
			t.Fatalf("RenderRequest: %v", err)
		}
		return out
	}
	djangotest.AssertEqual(t, render(t, nil), "/where/|")
	djangotest.AssertEqual(t, render(t, []string{"django.template.context_processors.static"}), "|/static/")

	_, err := newWiredAppErr(t, conf.Settings{Templates: conf.Templates{ContextProcessors: []string{"i18n"}}})
	var ic *conf.ImproperlyConfigured
	if !errors.As(err, &ic) {
		t.Fatalf("New = %v, want ImproperlyConfigured", err)
	}
	djangotest.AssertEqual(t, ic.Msg, `The TEMPLATES_CONTEXT_PROCESSORS setting names an unknown context processor "i18n".`)
}

// TestPostMigrateCreatesPermissions creates the content types and the default
// permissions after migrate, once the migrations of contenttypes and auth are
// applied, as Django's post_migrate receivers do.
func TestPostMigrateCreatesPermissions(t *testing.T) {
	app := newWiredApp(t, conf.Settings{Database: memoryDB(t)}, contenttypes.App{}, auth.App{})
	ctx := context.Background()
	if len(app.postMigrate) != 1 {
		t.Fatalf("%d post-migrate hooks, want 1", len(app.postMigrate))
	}
	hook := app.postMigrate[0]

	// Before the migrations are applied the tables are missing and the hook
	// does nothing.
	if err := hook(ctx, app.DB); err != nil {
		t.Fatalf("hook before the migrations: %v", err)
	}

	for _, m := range app.Models.Models() {
		if err := app.DB.CreateTable(ctx, m); err != nil {
			t.Fatalf("CreateTable(%s): %v", m.Name(), err)
		}
	}
	rec := migrations.NewRecorder(app.DB)
	if err := rec.EnsureSchema(ctx); err != nil {
		t.Fatal(err)
	}
	for _, label := range []string{"contenttypes", "auth"} {
		if err := rec.RecordApplied(ctx, migrations.Key{App: label, Name: "0001_initial"}); err != nil {
			t.Fatal(err)
		}
	}
	if err := hook(ctx, app.DB); err != nil {
		t.Fatalf("hook: %v", err)
	}
	n, err := orm.Query[auth.Permission](app.DB).Filter("codename", "add_user").Count(ctx)
	if err != nil {
		t.Fatal(err)
	}
	djangotest.AssertEqual(t, n, int64(1))
}

// TestNotFoundRendersThe404Template answers an unknown path with the
// 404.html of the project when Debug is off.
func TestNotFoundRendersThe404Template(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "404.html"), []byte("Missing {{ request_path }}"), 0o600); err != nil {
		t.Fatal(err)
	}
	home := func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "home") }
	app := newWiredApp(t, conf.Settings{Templates: conf.Templates{Dirs: []string{dir}, AppDirs: true}},
		routeApp{[]urls.Route{urls.PathFunc("GET /{$}", home, "home")}})
	rec := request(app, http.MethodGet, "/nope/")
	djangotest.AssertEqual(t, rec.Code, http.StatusNotFound)
	djangotest.AssertEqual(t, rec.Body.String(), "Missing /nope/")
}

// TestAuthCommandsAreRegistered registers changepassword and clearsessions.
// clearsessions on signed cookies does nothing, as Django's clear_expired of
// that engine does.
func TestAuthCommandsAreRegistered(t *testing.T) {
	app := newWiredApp(t, conf.Settings{})
	for _, name := range []string{"createsuperuser", "changepassword", "clearsessions"} {
		if !hasCommand(app, name) {
			t.Errorf("command %s is not registered", name)
		}
	}
	if err := app.Execute([]string{"clearsessions"}); err != nil {
		t.Errorf("clearsessions: %v", err)
	}
}
