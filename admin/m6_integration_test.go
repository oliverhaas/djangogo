package admin_test

import (
	"context"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/admin"
	"github.com/oliverhaas/djangogo/auth"
	"github.com/oliverhaas/djangogo/csrf"
	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/orm/backends/sqlite"
	"github.com/oliverhaas/djangogo/sessions"
	"github.com/oliverhaas/djangogo/templates"
	"github.com/oliverhaas/djangogo/urls"
)

// Product is the demo model the milestone-6 exit integration drives end to end.
type Product struct {
	ID     int64
	Name   string `orm:"max_length=100"`
	Price  int64
	Active bool
}

// csrfInputRe extracts the value of the hidden csrfmiddlewaretoken input from a
// rendered form so the test can echo it back on the matching POST.
var csrfInputRe = regexp.MustCompile(
	`name="csrfmiddlewaretoken"\s+value="([^"]*)"`,
)

// extractCSRF pulls the csrfmiddlewaretoken value out of an HTML body, failing
// the test when no token is present.
func extractCSRF(t *testing.T, body string) string {
	t.Helper()
	m := csrfInputRe.FindStringSubmatch(body)
	if m == nil {
		t.Fatalf("no csrfmiddlewaretoken hidden input in body:\n%s", body)
	}
	return m[1]
}

// newM6DB builds a frozen registry holding the auth models plus Product, opens a
// shared in-memory SQLite database, and creates every table.
func newM6DB(t *testing.T) *orm.DB {
	t.Helper()

	reg := orm.NewRegistry()
	for _, m := range auth.AppModels() {
		if _, err := reg.Register(m); err != nil {
			t.Fatalf("Register(%T): %v", m, err)
		}
	}
	if _, err := reg.Register(&Product{}); err != nil {
		t.Fatalf("Register(Product): %v", err)
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

	ctx := context.Background()
	for _, name := range []string{
		"User", "Group", "Permission",
		"UserGroup", "UserPermission", "GroupPermission",
		"Product",
	} {
		m, ok := reg.Get(name)
		if !ok {
			t.Fatalf("model %s not in registry", name)
		}
		if err := db.CreateTable(ctx, m); err != nil {
			t.Fatalf("CreateTable(%s): %v", name, err)
		}
	}
	return db
}

// m6Client is the cookie-carrying HTTP client used to drive the live server. Its
// redirect policy returns the last response so 302s are observable.
func m6Client(t *testing.T) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookiejar.New: %v", err)
	}
	return &http.Client{
		Jar:           jar,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

// getBody issues a GET and returns the status and body, closing the response.
func getBody(t *testing.T, c *http.Client, rawurl string) (int, string) {
	t.Helper()
	resp, err := c.Get(rawurl)
	if err != nil {
		t.Fatalf("GET %s: %v", rawurl, err)
	}
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode, readAll(t, resp)
}

// postForm issues a form POST and returns the status, Location header, and body,
// closing the response.
func postForm(t *testing.T, c *http.Client, rawurl string, form url.Values) (int, string, string) {
	t.Helper()
	resp, err := c.PostForm(rawurl, form)
	if err != nil {
		t.Fatalf("POST %s: %v", rawurl, err)
	}
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode, resp.Header.Get("Location"), readAll(t, resp)
}

// readAll drains resp.Body into a string.
func readAll(t *testing.T, resp *http.Response) string {
	t.Helper()
	var sb strings.Builder
	buf := make([]byte, 4096)
	for {
		n, err := resp.Body.Read(buf)
		sb.Write(buf[:n])
		if n == 0 || err != nil {
			break
		}
	}
	return sb.String()
}

// TestM6EndToEndAdminCRUD wires the whole stack (sessions, csrf, auth, admin)
// behind a real httptest.Server and drives an anonymous redirect, a staff login,
// and a CSRF-protected add/change/delete cycle, asserting both the rendered HTML
// and the resulting ORM state at each step, plus that a POST without a valid CSRF
// token is rejected with 403.
func TestM6EndToEndAdminCRUD(t *testing.T) {
	db := newM6DB(t)
	ctx := context.Background()

	// Seed a staff user.
	u := auth.User{Username: "admin", IsStaff: true, IsActive: true}
	if err := u.SetPassword("adminpass"); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}
	if err := orm.Query[auth.User](db).Create(ctx, &u); err != nil {
		t.Fatalf("Create(admin user): %v", err)
	}

	site, err := admin.NewAdminSite(db)
	if err != nil {
		t.Fatalf("NewAdminSite: %v", err)
	}
	admin.Register[Product](site, admin.ModelAdmin{
		ListDisplay: []string{"ID", "Name", "Price"},
		Ordering:    []string{"id"},
	})

	router := urls.NewRouter(site.Routes()...)

	// The admin templates reverse no named URLs, but restore the resolver anyway
	// so the test leaves no global state behind.
	savedResolver := templates.URLResolverFunc()
	t.Cleanup(func() { templates.SetURLResolver(savedResolver) })

	// Middleware chain (outer to inner): sessions -> csrf -> auth -> router.
	store := sessions.NewSignedCookieStore([]byte("m6-integration-secret"))
	var handler http.Handler = router
	handler = auth.Middleware(db)(handler)
	handler = csrf.Middleware(handler)
	handler = sessions.Middleware(store, "sessionid")(handler)

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	c := m6Client(t)
	base := srv.URL

	// 1. Anonymous GET of a protected page redirects to the login URL.
	{
		resp, err := c.Get(base + "/admin/product/")
		if err != nil {
			t.Fatalf("anon GET: %v", err)
		}
		body := readAll(t, resp)
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusFound {
			t.Fatalf("(1) anon /admin/product/ status = %d, want 302\n%s", resp.StatusCode, body)
		}
		if loc := resp.Header.Get("Location"); !strings.HasPrefix(loc, "/admin/login/") {
			t.Fatalf("(1) anon redirect Location = %q, want /admin/login/ prefix", loc)
		}
	}

	// 2. GET the login page: form present, csrf token + session seeded.
	loginURL := base + "/admin/login/?next=%2Fadmin%2Fproduct%2F"
	status, body := getBody(t, c, loginURL)
	if status != http.StatusOK {
		t.Fatalf("(2) GET login status = %d, want 200\n%s", status, body)
	}
	if !strings.Contains(body, `<form method="post"`) {
		t.Fatalf("(2) login body missing post form:\n%s", body)
	}
	loginToken := extractCSRF(t, body)

	// 3. POST the login form with the captured token -> 302 to the next target.
	{
		form := url.Values{}
		form.Set("username", "admin")
		form.Set("password", "adminpass")
		form.Set("csrfmiddlewaretoken", loginToken)
		form.Set("next", "/admin/product/")
		st, loc, b := postForm(t, c, base+"/admin/login/", form)
		if st != http.StatusFound {
			t.Fatalf("(3) login POST status = %d, want 302\n%s", st, b)
		}
		if loc != "/admin/product/" {
			t.Fatalf("(3) login POST Location = %q, want /admin/product/", loc)
		}
	}

	// 4. The changelist is now reachable and empty-ish.
	status, body = getBody(t, c, base+"/admin/product/")
	if status != http.StatusOK {
		t.Fatalf("(4) authed changelist status = %d, want 200\n%s", status, body)
	}
	if strings.Contains(body, "Widget") {
		t.Fatalf("(4) changelist unexpectedly contains a product:\n%s", body)
	}

	// 5. Add a product through the add form, then verify HTML + DB.
	status, body = getBody(t, c, base+"/admin/product/add/")
	if status != http.StatusOK {
		t.Fatalf("(5) add GET status = %d, want 200\n%s", status, body)
	}
	addToken := extractCSRF(t, body)
	{
		form := url.Values{}
		form.Set("Name", "Widget")
		form.Set("Price", "999")
		form.Set("Active", "on")
		form.Set("csrfmiddlewaretoken", addToken)
		st, loc, b := postForm(t, c, base+"/admin/product/add/", form)
		if st != http.StatusFound {
			t.Fatalf("(5) add POST status = %d, want 302\n%s", st, b)
		}
		if loc != "/admin/product/" {
			t.Fatalf("(5) add POST Location = %q, want /admin/product/", loc)
		}
	}
	status, body = getBody(t, c, base+"/admin/product/")
	if status != http.StatusOK || !strings.Contains(body, "Widget") {
		t.Fatalf("(5) changelist after add status=%d missing Widget:\n%s", status, body)
	}
	prod, err := orm.Query[Product](db).Get(ctx, "name", "Widget")
	if err != nil {
		t.Fatalf("(5) DB Get(Widget): %v", err)
	}
	if prod.Price != 999 || !prod.Active {
		t.Fatalf("(5) created Product = %+v, want Price 999 Active true", prod)
	}

	// 6. Change the product's name.
	status, body = getBody(t, c, base+"/admin/product/1/change/")
	if status != http.StatusOK {
		t.Fatalf("(6) change GET status = %d, want 200\n%s", status, body)
	}
	changeToken := extractCSRF(t, body)
	{
		form := url.Values{}
		form.Set("Name", "Gadget")
		form.Set("Price", "999")
		form.Set("Active", "on")
		form.Set("csrfmiddlewaretoken", changeToken)
		st, loc, b := postForm(t, c, base+"/admin/product/1/change/", form)
		if st != http.StatusFound {
			t.Fatalf("(6) change POST status = %d, want 302\n%s", st, b)
		}
		if loc != "/admin/product/" {
			t.Fatalf("(6) change POST Location = %q, want /admin/product/", loc)
		}
	}
	status, body = getBody(t, c, base+"/admin/product/")
	if status != http.StatusOK || !strings.Contains(body, "Gadget") {
		t.Fatalf("(6) changelist after change status=%d missing Gadget:\n%s", status, body)
	}
	prod, err = orm.Query[Product](db).Get(ctx, "id", 1)
	if err != nil {
		t.Fatalf("(6) DB Get(id=1): %v", err)
	}
	if prod.Name != "Gadget" {
		t.Fatalf("(6) updated Product.Name = %q, want Gadget", prod.Name)
	}

	// 7. Delete the product.
	status, body = getBody(t, c, base+"/admin/product/1/delete/")
	if status != http.StatusOK {
		t.Fatalf("(7) delete GET status = %d, want 200\n%s", status, body)
	}
	deleteToken := extractCSRF(t, body)
	{
		form := url.Values{}
		form.Set("csrfmiddlewaretoken", deleteToken)
		st, loc, b := postForm(t, c, base+"/admin/product/1/delete/", form)
		if st != http.StatusFound {
			t.Fatalf("(7) delete POST status = %d, want 302\n%s", st, b)
		}
		if loc != "/admin/product/" {
			t.Fatalf("(7) delete POST Location = %q, want /admin/product/", loc)
		}
	}
	if _, err := orm.Query[Product](db).Get(ctx, "id", 1); err == nil {
		t.Fatalf("(7) Product id=1 still present after delete")
	}
	status, body = getBody(t, c, base+"/admin/product/")
	if status != http.StatusOK {
		t.Fatalf("(7) changelist after delete status = %d, want 200", status)
	}
	if strings.Contains(body, "Gadget") {
		t.Fatalf("(7) changelist still lists deleted product:\n%s", body)
	}

	// 8. A POST without a valid CSRF token is rejected with 403.
	{
		form := url.Values{}
		form.Set("Name", "NoCSRF")
		form.Set("Price", "1")
		// No csrfmiddlewaretoken on purpose.
		st, _, _ := postForm(t, c, base+"/admin/product/add/", form)
		if st != http.StatusForbidden {
			t.Fatalf("(8) add POST without csrf status = %d, want 403", st)
		}
		if _, err := orm.Query[Product](db).Get(ctx, "name", "NoCSRF"); err == nil {
			t.Fatalf("(8) product created despite missing csrf token")
		}
	}
}
