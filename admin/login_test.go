package admin_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/admin"
	"github.com/oliverhaas/djangogo/auth"
	"github.com/oliverhaas/djangogo/csrf"
	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/orm/backends/sqlite"
	"github.com/oliverhaas/djangogo/sessions"
	"github.com/oliverhaas/djangogo/urls"
)

// newUserDB builds a frozen registry holding only the auth User model, opens an
// in-memory SQLite database, and creates the users table.
func newUserDB(t *testing.T) *orm.DB {
	t.Helper()

	reg := orm.NewRegistry()
	if _, err := reg.Register(&auth.User{}); err != nil {
		t.Fatalf("Register(User): %v", err)
	}
	if err := reg.Resolve(); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	reg.Freeze()

	sdb, err := sqlite.Open("file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("sqlite.Open: %v", err)
	}
	t.Cleanup(func() { _ = sdb.Close() })
	db := orm.NewDB(sdb, sqlite.New(), reg)

	m, ok := reg.Get("User")
	if !ok {
		t.Fatal("User model not found in registry")
	}
	if err := db.CreateTable(context.Background(), m); err != nil {
		t.Fatalf("CreateTable(User): %v", err)
	}
	return db
}

// seedStaffUser creates a staff user with the given password and returns it.
func seedStaffUser(t *testing.T, db *orm.DB, username, password string) auth.User {
	t.Helper()
	u := auth.User{Username: username, IsStaff: true, IsActive: true}
	if err := u.SetPassword(password); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}
	if err := orm.Query[auth.User](db).Create(context.Background(), &u); err != nil {
		t.Fatalf("Create(%s): %v", username, err)
	}
	return u
}

// loginHandler wires the AdminSite router behind the sessions -> csrf -> auth
// chain so the login view can read the session and CSRF token exactly as it does
// in production.
func loginHandler(t *testing.T, db *orm.DB) (*admin.AdminSite, http.Handler) {
	t.Helper()
	site, err := admin.NewAdminSite(db)
	if err != nil {
		t.Fatalf("NewAdminSite: %v", err)
	}
	router := urls.NewRouter(site.Routes()...)
	store := sessions.NewSignedCookieStore([]byte("login-test-secret"))
	var h http.Handler = router
	h = auth.Middleware(db)(h)
	h = csrf.Middleware(h)
	h = sessions.Middleware(store, "sessionid")(h)
	return site, h
}

func TestLoginGETRendersForm(t *testing.T) {
	db := newUserDB(t)
	_, h := loginHandler(t, db)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin/login/?next=%2Fadmin%2Fproduct%2F", nil)
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("login GET status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `<form method="post"`) {
		t.Errorf("login GET body missing post form:\n%s", body)
	}
	if !strings.Contains(body, `name="username"`) || !strings.Contains(body, `name="password"`) {
		t.Errorf("login GET body missing credential inputs:\n%s", body)
	}
	if !strings.Contains(body, `name="csrfmiddlewaretoken"`) {
		t.Errorf("login GET body missing csrf hidden input:\n%s", body)
	}
	if !strings.Contains(body, `name="next" value="/admin/product/"`) {
		t.Errorf("login GET body missing safe next hidden input:\n%s", body)
	}
}

// loginPost runs a full GET (to capture the session cookie + csrf token) then a
// POST through the same handler so the CSRF middleware accepts the request. It
// returns the POST recorder.
func loginPost(t *testing.T, h http.Handler, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	getRec := httptest.NewRecorder()
	getReq := httptest.NewRequest(http.MethodGet, "/admin/login/", nil)
	h.ServeHTTP(getRec, getReq)
	token := extractCSRF(t, getRec.Body.String())
	cookie := getRec.Result().Cookies()

	form.Set("csrfmiddlewaretoken", token)
	postRec := httptest.NewRecorder()
	postReq := httptest.NewRequest(http.MethodPost, "/admin/login/", strings.NewReader(form.Encode()))
	postReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for _, c := range cookie {
		postReq.AddCookie(c)
	}
	h.ServeHTTP(postRec, postReq)
	return postRec
}

func TestLoginPOSTValidRedirectsToSafeNext(t *testing.T) {
	db := newUserDB(t)
	_, h := loginHandler(t, db)
	seedStaffUser(t, db, "admin", "secret")

	form := url.Values{}
	form.Set("username", "admin")
	form.Set("password", "secret")
	form.Set("next", "/admin/product/")
	rec := loginPost(t, h, form)

	if rec.Code != http.StatusFound {
		t.Fatalf("login POST status = %d, want 302\n%s", rec.Code, rec.Body.String())
	}
	if loc := rec.Header().Get("Location"); loc != "/admin/product/" {
		t.Errorf("login POST Location = %q, want /admin/product/", loc)
	}
}

func TestLoginPOSTRejectsOpenRedirectNext(t *testing.T) {
	db := newUserDB(t)
	_, h := loginHandler(t, db)
	seedStaffUser(t, db, "admin", "secret")

	for _, evil := range []string{"//evil.example.com/path", "https://evil.example.com", "javascript:alert(1)"} {
		form := url.Values{}
		form.Set("username", "admin")
		form.Set("password", "secret")
		form.Set("next", evil)
		rec := loginPost(t, h, form)

		if rec.Code != http.StatusFound {
			t.Fatalf("login POST (%q) status = %d, want 302", evil, rec.Code)
		}
		if loc := rec.Header().Get("Location"); loc != "/admin/" {
			t.Errorf("login POST next=%q Location = %q, want /admin/ (open redirect blocked)", evil, loc)
		}
	}
}

func TestLoginPOSTBadCredentialsReRenders(t *testing.T) {
	db := newUserDB(t)
	_, h := loginHandler(t, db)
	seedStaffUser(t, db, "admin", "secret")

	cases := []struct {
		name             string
		username, passwd string
	}{
		{"wrong password", "admin", "nope"},
		{"unknown user", "ghost", "secret"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			form := url.Values{}
			form.Set("username", c.username)
			form.Set("password", c.passwd)
			rec := loginPost(t, h, form)

			if rec.Code != http.StatusOK {
				t.Fatalf("login POST status = %d, want 200 (re-render)", rec.Code)
			}
			body := rec.Body.String()
			if !strings.Contains(body, "correct username and password") {
				t.Errorf("login POST body missing error message:\n%s", body)
			}
		})
	}
}

func TestLoginPOSTNonStaffRejected(t *testing.T) {
	db := newUserDB(t)
	_, h := loginHandler(t, db)
	// A valid, active, but non-staff user must not be allowed into the admin.
	u := auth.User{Username: "joe", IsStaff: false, IsActive: true}
	if err := u.SetPassword("secret"); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}
	if err := orm.Query[auth.User](db).Create(context.Background(), &u); err != nil {
		t.Fatalf("Create(joe): %v", err)
	}

	form := url.Values{}
	form.Set("username", "joe")
	form.Set("password", "secret")
	rec := loginPost(t, h, form)

	if rec.Code != http.StatusOK {
		t.Fatalf("non-staff login POST status = %d, want 200 (re-render)", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "not staff") {
		t.Errorf("non-staff login POST body missing staff error:\n%s", rec.Body.String())
	}
}
