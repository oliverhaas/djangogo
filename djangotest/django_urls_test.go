package djangotest_test

import (
	"net/http"
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/urls"
)

// resolverMatchRouter is the part of tests/urlpatterns_reverse/namespace_urls.py
// the resolver_match tests request.
func resolverMatchRouter() *urls.Router {
	ok := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})
	return urls.NewRouter(
		urls.Path("resolver_match/", ok, "test-resolver-match"),
		urls.Path("articles/<int:year>/", ok, "article-year"),
	)
}

// TestResolverMatchOnRequest checks the accessor for Django's
// response.resolver_match.
//
// Ported from Django: tests/urlpatterns_reverse/tests.py::ResolverMatchTests.test_resolver_match_on_request
func TestResolverMatchOnRequest(t *testing.T) {
	c := djangotest.NewClient(t, resolverMatchRouter())
	m := djangotest.ResolverMatch(t, c.Get("/resolver_match/"))
	if m == nil || m.URLName != "test-resolver-match" {
		t.Fatalf("resolver match = %+v, want url name test-resolver-match", m)
	}
}

// TestResolverMatchWithExplicitResolver checks the form for a client whose
// handler wraps the router, where the router has to be passed.
func TestResolverMatchWithExplicitResolver(t *testing.T) {
	router := resolverMatchRouter()
	wrapped := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { router.ServeHTTP(w, r) })
	c := djangotest.NewClient(t, wrapped)
	resp := c.Get("/articles/2024/")
	m := djangotest.ResolverMatch(t, resp, router)
	if m == nil || m.URLName != "article-year" || m.Kwargs["year"] != 2024 {
		t.Fatalf("resolver match = %+v, want article-year with year 2024", m)
	}
	mustFail(t, `ResolverMatch has no URL configuration to resolve "/articles/2024/" with`, func(ft djangotest.TB) {
		djangotest.ResolverMatch(ft, resp)
	})
}

// TestResolverMatchFailsOnUnresolvedPath checks the failure for a path no
// route matches.
func TestResolverMatchFailsOnUnresolvedPath(t *testing.T) {
	c := djangotest.NewClient(t, resolverMatchRouter())
	resp := c.Get("/nope/")
	mustFail(t, `The path "/nope/" of the request did not resolve to a route`, func(ft djangotest.TB) {
		djangotest.ResolverMatch(ft, resp)
	})
	mustFail(t, "ResolverMatch needs a response that came from a Client request.", func(ft djangotest.TB) {
		djangotest.ResolverMatch(ft, nil)
	})
}
