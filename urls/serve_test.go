package urls_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/oliverhaas/djangogo/urls"
)

// TestHandlerSeesTypedParams checks the three ways a handler reads its
// arguments: the converted value, the typed helper, and the captured text.
func TestHandlerSeesTypedParams(t *testing.T) {
	const id = "0fa3c1e2-5b7d-4c8e-9f10-112233445566"
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		year, okYear := urls.IntParam(r, "year")
		slug, okSlug := urls.StringParam(r, "slug")
		uid, okUID := urls.UUIDParam(r, "id")
		_, wrongType := urls.IntParam(r, "slug")
		_, missing := urls.StringParam(r, "nope")
		fmt.Fprintf(w, "%d %v %s %v %s %v %v %v|%v|%s|%s",
			year, okYear, slug, okSlug, uid, okUID, wrongType, missing,
			urls.Param(r, "year"), r.PathValue("year"), urls.Match(r).URLName)
	})
	router := urls.NewRouter(urls.Path("articles/<int:year>/<slug:slug>/<uuid:id>/", h, "article"))
	rec := serve(router, http.MethodGet, "/articles/2024/hello-world/"+id+"/")
	want := "2024 true hello-world true " + id + " true false false|2024|2024|article"
	if rec.Code != http.StatusOK || rec.Body.String() != want {
		t.Fatalf("got %d %q, want 200 %q", rec.Code, rec.Body.String(), want)
	}
}

// TestConverterMismatchIs404 checks that a value the converter does not accept
// makes the route not match.
func TestConverterMismatchIs404(t *testing.T) {
	router := urls.NewRouter(urls.Path("articles/<int:year>/", emptyView, "year"))
	for _, target := range []string{"/articles/abc/", "/articles/20x4/", "/articles/99999999999999999999/"} {
		if rec := serve(router, http.MethodGet, target); rec.Code != http.StatusNotFound {
			t.Errorf("%s: status = %d, want 404", target, rec.Code)
		}
	}
}

// TestRoutesMatchInDeclarationOrder checks that Path and RePath routes share
// one ordered list and the first match wins.
func TestRoutesMatchInDeclarationOrder(t *testing.T) {
	say := func(s string) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintf(w, "%s %v %v", s, urls.Args(r), urls.Match(r).Kwargs)
		})
	}
	router := urls.NewRouter(
		urls.RePath(`^items/(?P<id>[0-9]+)/$`, say("regex"), "by-id"),
		urls.Path("items/<slug:name>/", say("path"), "by-name"),
		urls.RePath(`^items/([a-z]+)/([0-9]+)/$`, say("unnamed"), "unnamed"),
	)
	for target, want := range map[string]string{
		"/items/42/":    "regex [] map[id:42]",
		"/items/chair/": "path [] map[name:chair]",
		"/items/a/7/":   "unnamed [a 7] map[]",
	} {
		if got := serve(router, http.MethodGet, target).Body.String(); got != want {
			t.Errorf("%s: got %q, want %q", target, got, want)
		}
	}
}

// TestNotFoundHandler checks the configurable 404 handler.
func TestNotFoundHandler(t *testing.T) {
	router := urls.NewRouter(urls.Path("a/", emptyView, "a"))
	router.NotFoundHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("custom 404 for " + r.URL.Path))
	})
	rec := serve(router, http.MethodGet, "/missing/")
	if rec.Code != http.StatusNotFound || rec.Body.String() != "custom 404 for /missing/" {
		t.Fatalf("got %d %q", rec.Code, rec.Body.String())
	}
}

// TestMethodNotAllowed checks the 405 answer: the Allow header lists the
// methods of the routes that match the path, and the handler is configurable.
func TestMethodNotAllowed(t *testing.T) {
	router := urls.NewRouter(
		urls.Path("GET /login/", emptyView, "login"),
		urls.Path("POST /login/", emptyView, "login-post"),
		urls.Path("any/", emptyView, "any"),
	)
	rec := serve(router, http.MethodDelete, "/login/")
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rec.Code)
	}
	if got, want := rec.Header().Get("Allow"), "GET, HEAD, POST"; got != want {
		t.Errorf("Allow = %q, want %q", got, want)
	}
	if rec := serve(router, http.MethodHead, "/login/"); rec.Code != http.StatusOK {
		t.Errorf("HEAD on a GET route: status = %d, want 200", rec.Code)
	}
	if rec := serve(router, http.MethodDelete, "/any/"); rec.Code != http.StatusOK {
		t.Errorf("route without a method: status = %d, want 200", rec.Code)
	}
	router.MethodNotAllowedHandler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusMethodNotAllowed)
		_, _ = w.Write([]byte("custom 405"))
	})
	rec = serve(router, http.MethodPut, "/login/")
	if rec.Code != http.StatusMethodNotAllowed || rec.Body.String() != "custom 405" {
		t.Errorf("custom handler: got %d %q", rec.Code, rec.Body.String())
	}
}

// TestSubtreePatternIsAFallback checks that a ServeMux pattern with a
// trailing slash also serves the paths below it, but only when no route
// matches the path exactly.
func TestSubtreePatternIsAFallback(t *testing.T) {
	say := func(s string) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(s)) })
	}
	router := urls.NewRouter(
		urls.Path("/static/", say("tree"), "static"),
		urls.Path("/static/exact.css", say("exact"), "exact"),
		urls.Path("django/", say("django"), "django"),
	)
	for target, want := range map[string]string{
		"/static/":          "tree",
		"/static/a/b.css":   "tree",
		"/static/exact.css": "exact",
		"/django/":          "django",
	} {
		if got := serve(router, http.MethodGet, target).Body.String(); got != want {
			t.Errorf("%s: got %q, want %q", target, got, want)
		}
	}
	if rec := serve(router, http.MethodGet, "/django/below/"); rec.Code != http.StatusNotFound {
		t.Errorf("a Django route must not match below itself: status = %d", rec.Code)
	}
}

// TestIncludeKwargsAndCurrentApp checks extra arguments on an include and the
// instance choice by current application.
func TestIncludeKwargsAndCurrentApp(t *testing.T) {
	polls := []urls.Route{urls.Path("<int:pk>/", emptyView, "detail")}
	router := urls.NewRouter(slices.Concat(
		urls.IncludeWith(urls.IncludeOptions{
			Prefix: "author-polls/", AppName: "polls", Namespace: "author-polls", Kwargs: urls.Kwargs{"site": "a"},
		}, polls...),
		urls.IncludeApp("publisher-polls/", "polls", "publisher-polls", polls...),
	)...)

	m := mustResolve(t, router, "/author-polls/3/")
	assertKwargs(t, "Kwargs", m.Kwargs, urls.Kwargs{"pk": 3, "site": "a"})
	assertKwargs(t, "ExtraKwargs", m.ExtraKwargs, urls.Kwargs{"site": "a"})
	if m.ViewName != "author-polls:detail" || m.AppName() != "polls" {
		t.Errorf("ViewName %q, AppName %q", m.ViewName, m.AppName())
	}

	// Without a current application and without a default instance, the last
	// instance declared is used.
	assertReverse(t, router, "/publisher-polls/3/", "polls:detail", 3)
	assertReverse(t, router, "/author-polls/3/", "polls:detail", 3, urls.CurrentApp("author-polls"))
	assertReverse(t, router, "/author-polls/3/", "author-polls:detail", urls.Kwargs{"pk": 3})
}

// TestRouterIsSafeForConcurrentUse serves and reverses from several
// goroutines; the race detector checks the shared reverse cache.
func TestRouterIsSafeForConcurrentUse(t *testing.T) {
	router := urls.NewRouter(urls.Path("a/<int:id>/", emptyView, "a"))
	done := make(chan struct{})
	for range 8 {
		go func() {
			defer func() { done <- struct{}{} }()
			for i := range 50 {
				if _, err := router.Reverse("a", i); err != nil {
					t.Error(err)
				}
				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/a/1/", nil))
			}
		}()
	}
	for range 8 {
		<-done
	}
}
