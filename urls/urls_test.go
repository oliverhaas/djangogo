package urls_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/oliverhaas/djangogo/urls"
)

// handlerFunc returns a handler that writes the given status code.
func handlerFunc(code int) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(code)
	}
}

// idCapture returns a handler that writes the value of the "id" path param.
func idCapture() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(r.PathValue("id")))
	}
}

// ---- Path / PathFunc -------------------------------------------------------

func TestPathBuildsRoute(t *testing.T) {
	h := handlerFunc(http.StatusOK)
	r := urls.Path("GET /articles/{id}/", h, "article-detail")
	if r.Pattern != "GET /articles/{id}/" {
		t.Fatalf("Pattern: got %q", r.Pattern)
	}
	if r.Name != "article-detail" {
		t.Fatalf("Name: got %q", r.Name)
	}
	if r.Handler == nil {
		t.Fatal("Handler must not be nil")
	}
}

func TestPathFuncBuildsRoute(t *testing.T) {
	fn := handlerFunc(http.StatusOK)
	r := urls.PathFunc("GET /articles/{id}/", fn, "article-detail")
	if r.Pattern != "GET /articles/{id}/" {
		t.Fatalf("Pattern: got %q", r.Pattern)
	}
	if r.Handler == nil {
		t.Fatal("Handler must not be nil")
	}
}

func TestPathAnonymousRoute(t *testing.T) {
	r := urls.Path("/about/", handlerFunc(http.StatusOK), "")
	if r.Name != "" {
		t.Fatalf("expected empty name, got %q", r.Name)
	}
}

// ---- Router: serving -------------------------------------------------------

func TestRouterServes200(t *testing.T) {
	router := urls.NewRouter(
		urls.Path("GET /articles/{id}/", idCapture(), "article-detail"),
	)
	req := httptest.NewRequest(http.MethodGet, "/articles/99/", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", rec.Code)
	}
	if got := rec.Body.String(); got != "99" {
		t.Fatalf("body: got %q, want %q", got, "99")
	}
}

func TestRouterServeMuxReturnsUnderlyingMux(t *testing.T) {
	router := urls.NewRouter(
		urls.Path("GET /ping/", handlerFunc(http.StatusOK), ""),
	)
	mux := router.ServeMux()
	if mux == nil {
		t.Fatal("ServeMux() must not return nil")
	}
	req := httptest.NewRequest(http.MethodGet, "/ping/", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status via mux: got %d, want 200", rec.Code)
	}
}

func TestRouterUnmatchedPath404(t *testing.T) {
	router := urls.NewRouter(
		urls.Path("GET /articles/{id}/", idCapture(), "article-detail"),
	)
	req := httptest.NewRequest(http.MethodGet, "/nope/", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status: got %d, want 404", rec.Code)
	}
}

func TestRouterMethodMismatch405(t *testing.T) {
	// POST to a GET-only route must be rejected by the mux (405 Method Not Allowed).
	router := urls.NewRouter(
		urls.Path("GET /articles/{id}/", idCapture(), "article-detail"),
	)
	req := httptest.NewRequest(http.MethodPost, "/articles/1/", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status: got %d, want 405", rec.Code)
	}
}

// ---- Reverse ---------------------------------------------------------------

func TestReverseSingleParam(t *testing.T) {
	router := urls.NewRouter(
		urls.Path("GET /articles/{id}/", handlerFunc(http.StatusOK), "article-detail"),
	)
	got, err := router.Reverse("article-detail", 42)
	if err != nil {
		t.Fatalf("Reverse: %v", err)
	}
	if want := "/articles/42/"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestReverseMultiParam(t *testing.T) {
	router := urls.NewRouter(
		urls.Path("GET /blog/{year}/{slug}/", handlerFunc(http.StatusOK), "post-detail"),
	)
	got, err := router.Reverse("post-detail", 2024, "hello-world")
	if err != nil {
		t.Fatalf("Reverse: %v", err)
	}
	if want := "/blog/2024/hello-world/"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestReverseStaticPath(t *testing.T) {
	router := urls.NewRouter(
		urls.Path("/about/", handlerFunc(http.StatusOK), "about"),
	)
	got, err := router.Reverse("about")
	if err != nil {
		t.Fatalf("Reverse: %v", err)
	}
	if want := "/about/"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestReverseUnknownNameErrors(t *testing.T) {
	router := urls.NewRouter()
	if _, err := router.Reverse("does-not-exist"); err == nil {
		t.Fatal("expected error for unknown name, got nil")
	}
}

func TestReverseArgCountMismatchErrors(t *testing.T) {
	router := urls.NewRouter(
		urls.Path("GET /articles/{id}/", handlerFunc(http.StatusOK), "article-detail"),
	)
	if _, err := router.Reverse("article-detail"); err == nil {
		t.Fatal("expected error for missing arg, got nil")
	}
	if _, err := router.Reverse("article-detail", 1, 2); err == nil {
		t.Fatal("expected error for extra arg, got nil")
	}
}

func TestReverseWildcardCatchAll(t *testing.T) {
	// {slug...} is a catch-all wildcard; treated as one placeholder.
	router := urls.NewRouter(
		urls.Path("GET /files/{slug...}", handlerFunc(http.StatusOK), "file"),
	)
	got, err := router.Reverse("file", "a/b/c.txt")
	if err != nil {
		t.Fatalf("Reverse: %v", err)
	}
	if want := "/files/a/b/c.txt"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// ---- Include ---------------------------------------------------------------

func TestIncludePrefixesPatternAndName(t *testing.T) {
	inner := urls.Path("GET /{id}/", handlerFunc(http.StatusOK), "detail")
	routes := urls.Include("blog/", "blog", inner)
	if len(routes) != 1 {
		t.Fatalf("Include returned %d routes, want 1", len(routes))
	}
	r := routes[0]
	if want := "GET /blog/{id}/"; r.Pattern != want {
		t.Fatalf("Pattern: got %q, want %q", r.Pattern, want)
	}
	if want := "blog:detail"; r.Name != want {
		t.Fatalf("Name: got %q, want %q", r.Name, want)
	}
}

func TestIncludeMethodlessPattern(t *testing.T) {
	inner := urls.Path("/about/", handlerFunc(http.StatusOK), "about")
	routes := urls.Include("site/", "site", inner)
	r := routes[0]
	if want := "/site/about/"; r.Pattern != want {
		t.Fatalf("Pattern: got %q, want %q", r.Pattern, want)
	}
	if want := "site:about"; r.Name != want {
		t.Fatalf("Name: got %q, want %q", r.Name, want)
	}
}

func TestIncludeEmptyNamespace(t *testing.T) {
	inner := urls.Path("/x/", handlerFunc(http.StatusOK), "x")
	routes := urls.Include("prefix/", "", inner)
	if routes[0].Name != "x" {
		t.Fatalf("Name: got %q, want %q", routes[0].Name, "x")
	}
}

func TestIncludeAnonymousRouteNameStaysEmpty(t *testing.T) {
	inner := urls.Path("/x/", handlerFunc(http.StatusOK), "")
	routes := urls.Include("prefix/", "ns", inner)
	if routes[0].Name != "" {
		t.Fatalf("expected empty name, got %q", routes[0].Name)
	}
}

func TestRouterWithIncludedRoutes(t *testing.T) {
	inner := urls.PathFunc("GET /{id}/", idCapture(), "detail")
	included := urls.Include("blog/", "blog", inner)
	router := urls.NewRouter(included...)

	// Serve
	req := httptest.NewRequest(http.MethodGet, "/blog/7/", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", rec.Code)
	}
	if got := rec.Body.String(); got != "7" {
		t.Fatalf("path value: got %q, want %q", got, "7")
	}

	// Reverse
	got, err := router.Reverse("blog:detail", 7)
	if err != nil {
		t.Fatalf("Reverse: %v", err)
	}
	if want := "/blog/7/"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// ---- NewRouter duplicate name ----------------------------------------------

func TestNewRouterDuplicateNamePanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on duplicate name, got none")
		}
	}()
	urls.NewRouter(
		urls.Path("/a/", handlerFunc(http.StatusOK), "same"),
		urls.Path("/b/", handlerFunc(http.StatusOK), "same"),
	)
}
