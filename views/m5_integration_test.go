package views_test

import (
	"context"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/auth"
	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/orm/backends/sqlite"
	"github.com/oliverhaas/djangogo/sessions"
	"github.com/oliverhaas/djangogo/templates"
	"github.com/oliverhaas/djangogo/urls"
	"github.com/oliverhaas/djangogo/views"
)

// Article is the demo model the protected DetailView renders.
type Article struct {
	ID    int64
	Title string `orm:"max_length=200"`
}

// newM5DB registers the auth models plus Article, resolves, freezes, creates all
// tables, and returns the handle.
func newM5DB(t *testing.T) *orm.DB {
	t.Helper()
	reg := orm.NewRegistry()
	for _, m := range auth.AppModels() {
		if _, err := reg.Register(m); err != nil {
			t.Fatalf("Register(%T): %v", m, err)
		}
	}
	if _, err := reg.Register(&Article{}); err != nil {
		t.Fatalf("Register(Article): %v", err)
	}
	if err := reg.Resolve(); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	reg.Freeze()

	dsn := "file:" + t.Name() + "?mode=memory&cache=shared"
	sdb, err := sqlite.Open(dsn)
	if err != nil {
		t.Fatalf("sqlite.Open: %v", err)
	}
	t.Cleanup(func() { _ = sdb.Close() })
	db := orm.NewDB(sdb, sqlite.New(), reg)

	ctx := context.Background()
	for _, m := range reg.Models() {
		if err := db.CreateTable(ctx, m); err != nil {
			t.Fatalf("CreateTable(%s): %v", m.Name(), err)
		}
	}
	return db
}

// newM5Engine writes login.html and article.html into a temp dir and returns the
// Engine.
func newM5Engine(t *testing.T) *templates.Engine {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"login.html": `<form method="post" action="/login/">` +
			`{% csrf_token %}` +
			`<input name="username"><input name="password" type="password">` +
			`<button type="submit">Log in</button>` +
			`{% if error %}<p class="error">{{ error }}</p>{% endif %}` +
			`</form>`,
		"article.html": `<h1>{{ object.Title }}</h1>`,
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	eng, err := templates.NewEngine(dir)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	return eng
}

// TestM5LoginGatedExit wires the whole web stack (templates, urls, sessions, auth,
// the generic DetailView) and asserts the login gate end to end.
func TestM5LoginGatedExit(t *testing.T) {
	db := newM5DB(t)
	eng := newM5Engine(t)
	ctx := context.Background()

	// Seed a user and an article.
	user := auth.User{Username: "alice"}
	if err := user.SetPassword("secret"); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}
	if err := orm.Query[auth.User](db).Create(ctx, &user); err != nil {
		t.Fatalf("Create(user): %v", err)
	}
	article := Article{Title: "Hello, World"}
	if err := orm.Query[Article](db).Create(ctx, &article); err != nil {
		t.Fatalf("Create(article): %v", err)
	}

	// Build the router. Reverse is wired into the template {% url %} resolver below.
	var router *urls.Router

	loginGet := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = views.Render(w, eng, "login.html", map[string]any{})
	})
	loginPost := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		username := r.PostFormValue("username")
		password := r.PostFormValue("password")
		u, err := orm.Query[auth.User](db).Get(r.Context(), "username", username)
		if err != nil || !u.CheckPassword(password) {
			w.WriteHeader(http.StatusUnauthorized)
			_ = views.Render(w, eng, "login.html", map[string]any{"error": "invalid credentials"})
			return
		}
		sess, ok := sessions.FromContext(r.Context())
		if !ok {
			http.Error(w, "no session", http.StatusInternalServerError)
			return
		}
		auth.Login(sess, &u)
		views.Redirect(w, r, "/articles/1/", http.StatusFound)
	})

	detail := views.DetailView[Article]{
		DB:       db,
		Engine:   eng,
		Template: "article.html",
		PKParam:  "pk",
	}
	protected := auth.LoginRequired("/login/")(detail)

	router = urls.NewRouter(
		urls.Path("GET /login/", loginGet, "login"),
		urls.Path("POST /login/", loginPost, "login-post"),
		urls.Path("GET /articles/{pk}/", protected, "article-detail"),
	)

	// Wire {% url %} to the router and restore the default resolver afterwards.
	prevResolver := templates.URLResolver
	templates.URLResolver = router.Reverse
	t.Cleanup(func() { templates.URLResolver = prevResolver })

	// Compose middleware: sessions outermost so the session is in context before
	// auth reads it.
	store := sessions.NewSignedCookieStore([]byte("test-secret"))
	handler := sessions.Middleware(store, "sessionid")(auth.Middleware(db)(router))

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookiejar.New: %v", err)
	}
	// Do not follow redirects: we assert the 302s directly.
	client := &http.Client{
		Jar: jar,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	// 1. Anonymous GET /articles/1/ -> 302 redirect to /login/ with ?next=.
	res := mustGet(t, client, srv.URL+"/articles/1/")
	if res.status != http.StatusFound {
		t.Fatalf("anon GET /articles/1/: status = %d, want 302", res.status)
	}
	if !strings.HasPrefix(res.location, "/login/") || !strings.Contains(res.location, "next=") {
		t.Errorf("anon redirect Location = %q, want /login/?next=...", res.location)
	}

	// 2. GET /login/ -> 200, body contains the form.
	res = mustGet(t, client, srv.URL+"/login/")
	if res.status != http.StatusOK {
		t.Fatalf("GET /login/: status = %d, want 200", res.status)
	}
	if !strings.Contains(res.body, `<form method="post"`) || !strings.Contains(res.body, `name="username"`) {
		t.Errorf("GET /login/ body missing form: %q", res.body)
	}

	// 3. POST /login/ valid creds -> 302 to the article, and a session cookie is set.
	form := url.Values{"username": {"alice"}, "password": {"secret"}}
	res = mustPostForm(t, client, srv.URL+"/login/", form)
	if res.status != http.StatusFound {
		t.Fatalf("POST /login/: status = %d, want 302; body=%q", res.status, res.body)
	}
	if res.location != "/articles/1/" {
		t.Errorf("POST /login/ Location = %q, want /articles/1/", res.location)
	}
	if !res.hasCookie("sessionid") {
		t.Errorf("POST /login/ did not set a sessionid cookie; cookies=%v", res.cookies)
	}

	// 4. GET /articles/1/ WITH the session cookie -> 200, body contains the Title.
	res = mustGet(t, client, srv.URL+"/articles/1/")
	if res.status != http.StatusOK {
		t.Fatalf("authed GET /articles/1/: status = %d, want 200; body=%q", res.status, res.body)
	}
	if !strings.Contains(res.body, "Hello, World") {
		t.Errorf("authed GET /articles/1/ body missing title: %q", res.body)
	}

	// 5. GET /articles/999/ with the cookie -> 404.
	res = mustGet(t, client, srv.URL+"/articles/999/")
	if res.status != http.StatusNotFound {
		t.Errorf("authed GET /articles/999/: status = %d, want 404", res.status)
	}
}

// respInfo is a value snapshot of an HTTP response: the helpers read and close the
// body before returning, so no open *http.Response (and its body) escapes.
type respInfo struct {
	status   int
	location string
	body     string
	cookies  []*http.Cookie
}

// hasCookie reports whether a non-empty cookie named name was set on the response.
func (r respInfo) hasCookie(name string) bool {
	for _, c := range r.cookies {
		if c.Name == name && c.Value != "" {
			return true
		}
	}
	return false
}

// mustGet performs a GET and returns a snapshot of the response, closing the body.
func mustGet(t *testing.T, c *http.Client, target string) respInfo {
	t.Helper()
	resp, err := c.Get(target)
	if err != nil {
		t.Fatalf("GET %s: %v", target, err)
	}
	defer func() { _ = resp.Body.Close() }()
	return readResp(t, resp)
}

// mustPostForm performs a form POST and returns a snapshot of the response, closing
// the body.
func mustPostForm(t *testing.T, c *http.Client, target string, form url.Values) respInfo {
	t.Helper()
	resp, err := c.PostForm(target, form)
	if err != nil {
		t.Fatalf("POST %s: %v", target, err)
	}
	defer func() { _ = resp.Body.Close() }()
	return readResp(t, resp)
}

// readResp reads resp's body fully and returns a value snapshot. The caller is
// responsible for closing resp.Body.
func readResp(t *testing.T, resp *http.Response) respInfo {
	t.Helper()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return respInfo{
		status:   resp.StatusCode,
		location: resp.Header.Get("Location"),
		body:     string(b),
		cookies:  resp.Cookies(),
	}
}
