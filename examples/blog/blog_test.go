package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/orm/backends/sqlite"
)

// csrfTokenRe extracts the token rendered by the {% csrf_token %} tag's hidden
// input, so a test can replay it on a POST the CSRF middleware will accept.
var csrfTokenRe = regexp.MustCompile(`name="csrfmiddlewaretoken" value="([^"]+)"`)

// seedDB opens a second handle on the shared in-memory database and inserts the
// fixture posts. newServer has already created the tables, so this registry only
// needs Post to run inserts against the same underlying database.
func seedDB(t *testing.T, dsn string) {
	t.Helper()

	reg := orm.NewRegistry()
	if _, err := reg.Register(&Post{}); err != nil {
		t.Fatalf("Register(Post): %v", err)
	}
	if err := reg.Resolve(); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	reg.Freeze()

	sdb, err := sqlite.Open(dsn)
	if err != nil {
		t.Fatalf("sqlite.Open: %v", err)
	}
	t.Cleanup(func() { _ = sdb.Close() })
	db := orm.NewDB(sdb, sqlite.New(), reg)

	ctx := context.Background()
	posts := []Post{
		{Title: "First Post", Body: "Hello, world.", Published: true, CreatedAt: time.Unix(1, 0)},
		{Title: "Second Post", Body: "Another one.", Published: true, CreatedAt: time.Unix(2, 0)},
		{Title: "Draft Post", Body: "Not ready.", Published: false, CreatedAt: time.Unix(3, 0)},
	}
	for i := range posts {
		if err := orm.Query[Post](db).Create(ctx, &posts[i]); err != nil {
			t.Fatalf("Create(%q): %v", posts[i].Title, err)
		}
	}
}

// newTestServer wires the app against a shared in-memory SQLite database and
// seeds it with fixture posts, returning the live handler. The shared-cache DSN
// lets the seeding handle and the server's handle see the same database, and the
// per-test database name keeps tests isolated from one another.
func newTestServer(t *testing.T) http.Handler {
	t.Helper()

	dsn := "file:" + t.Name() + "?mode=memory&cache=shared"
	handler, err := newServer(config{DSN: dsn, SecretKey: "test-secret"})
	if err != nil {
		t.Fatalf("newServer: %v", err)
	}
	seedDB(t, dsn)
	return handler
}

// get issues a GET against the in-process handler and returns the recorder.
func get(t *testing.T, handler http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

// TestPostListRendersPublishedPosts checks that the public "/" list renders the
// seeded published posts and hides the unpublished draft.
func TestPostListRendersPublishedPosts(t *testing.T) {
	handler := newTestServer(t)

	rec := get(t, handler, "/")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET / status = %d, want 200\n%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{"First Post", "Second Post"} {
		if !strings.Contains(body, want) {
			t.Errorf("post list missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "Draft Post") {
		t.Errorf("post list leaked the unpublished draft:\n%s", body)
	}
}

// TestPostDetailRendersOnePost checks that "/posts/{pk}/" renders one post's body
// and that an unknown pk yields 404.
func TestPostDetailRendersOnePost(t *testing.T) {
	handler := newTestServer(t)

	rec := get(t, handler, "/posts/1/")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /posts/1/ status = %d, want 200\n%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "First Post") || !strings.Contains(body, "Hello, world.") {
		t.Errorf("detail page missing post content:\n%s", body)
	}

	rec = get(t, handler, "/posts/999/")
	if rec.Code != http.StatusNotFound {
		t.Errorf("GET /posts/999/ status = %d, want 404", rec.Code)
	}
}

// TestPostDetailShowsCommentForm checks that the detail page renders the comment
// ModelForm (Name and Body fields plus the CSRF hidden input) and the empty
// comments state before any comment exists.
func TestPostDetailShowsCommentForm(t *testing.T) {
	handler := newTestServer(t)

	rec := get(t, handler, "/posts/1/")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /posts/1/ status = %d, want 200\n%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		`name="csrfmiddlewaretoken"`,
		`name="Name"`,
		`name="Body"`,
		"No comments yet",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("detail page missing %q:\n%s", want, body)
		}
	}
}

// TestCommentSubmissionCreatesComment drives the full comment round-trip: GET to
// obtain the session cookie and CSRF token, POST the comment with both, expect
// the Post/Redirect/Get 302, then confirm the comment shows on a fresh GET.
func TestCommentSubmissionCreatesComment(t *testing.T) {
	handler := newTestServer(t)

	rec := get(t, handler, "/posts/1/")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /posts/1/ status = %d, want 200\n%s", rec.Code, rec.Body.String())
	}
	cookies := rec.Result().Cookies()
	m := csrfTokenRe.FindStringSubmatch(rec.Body.String())
	if m == nil {
		t.Fatalf("no CSRF token in detail page:\n%s", rec.Body.String())
	}

	form := url.Values{
		"Name":                {"Alice"},
		"Body":                {"Great write-up!"},
		"csrfmiddlewaretoken": {m[1]},
	}
	req := httptest.NewRequest(http.MethodPost, "/posts/1/", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("POST comment status = %d, want 302 (PRG)\n%s", rec.Code, rec.Body.String())
	}
	if loc := rec.Header().Get("Location"); loc != "/posts/1/" {
		t.Errorf("PRG redirect Location = %q, want /posts/1/", loc)
	}

	rec = get(t, handler, "/posts/1/")
	body := rec.Body.String()
	if !strings.Contains(body, "Alice") || !strings.Contains(body, "Great write-up!") {
		t.Errorf("submitted comment not shown on detail page:\n%s", body)
	}
	if strings.Contains(body, "No comments yet") {
		t.Errorf("empty-comments state shown despite a saved comment:\n%s", body)
	}
}

// TestAdminRedirectsAnonymous checks that the staff-gated admin redirects an
// anonymous user to the admin login with a next parameter.
func TestAdminRedirectsAnonymous(t *testing.T) {
	handler := newTestServer(t)

	rec := get(t, handler, "/admin/")
	if rec.Code != http.StatusFound {
		t.Fatalf("GET /admin/ status = %d, want 302", rec.Code)
	}
	loc := rec.Header().Get("Location")
	if !strings.HasPrefix(loc, "/admin/login/") {
		t.Errorf("admin redirect Location = %q, want /admin/login/ prefix", loc)
	}
}
