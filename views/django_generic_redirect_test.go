package views_test

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/urls"
	"github.com/oliverhaas/djangogo/views"
)

// redirectOf serves one request to the view, as Django's tests call the view
// with a RequestFactory request.
func redirectOf(view views.RedirectView, method, target string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	view.ServeHTTP(rec, httptest.NewRequest(method, target, nil))
	return rec
}

// assertRedirect asserts the status and the Location header of a redirect.
func assertRedirect(t *testing.T, rec *httptest.ResponseRecorder, status int, location string) {
	t.Helper()
	djangotest.AssertEqual(t, rec.Code, status)
	djangotest.AssertEqual(t, rec.Header().Get("Location"), location)
}

// Ported from Django: tests/generic_views/test_base.py::RedirectViewTest.test_no_url
// Ported from Django: tests/generic_views/test_base.py::RedirectViewTest.test_direct_instantiation
//
// test_direct_instantiation sends HEAD to a view that was built without
// as_view(). A Go view is always built that way.
func TestRedirectViewNoURL(t *testing.T) {
	djangotest.AssertEqual(t, redirectOf(views.RedirectView{}, "GET", "/foo/").Code, http.StatusGone)
	djangotest.AssertEqual(t, redirectOf(views.RedirectView{}, "HEAD", "/foo/").Code, http.StatusGone)
}

// Ported from Django: tests/generic_views/test_base.py::RedirectViewTest.test_default_redirect
// Ported from Django: tests/generic_views/test_base.py::RedirectViewTest.test_temporary_redirect
// Ported from Django: tests/generic_views/test_base.py::RedirectViewTest.test_redirect_when_meta_contains_no_query_string
//
// Permanent set to false is the zero value, and a Go request without a query
// string has an empty one, so the three Django tests are one case here.
func TestRedirectViewDefaultRedirect(t *testing.T) {
	assertRedirect(t, redirectOf(views.RedirectView{URL: "/bar/"}, "GET", "/foo/"), http.StatusFound, "/bar/")
}

// Ported from Django: tests/generic_views/test_base.py::RedirectViewTest.test_permanent_redirect
func TestRedirectViewPermanentRedirect(t *testing.T) {
	view := views.RedirectView{URL: "/bar/", Permanent: true}
	assertRedirect(t, redirectOf(view, "GET", "/foo/"), http.StatusMovedPermanently, "/bar/")
}

// Ported from Django: tests/generic_views/test_base.py::RedirectViewTest.test_preserve_request_temporary_redirect
func TestRedirectViewPreserveRequestTemporaryRedirect(t *testing.T) {
	view := views.RedirectView{URL: "/bar/", PreserveRequest: true}
	assertRedirect(t, redirectOf(view, "GET", "/foo/"), http.StatusTemporaryRedirect, "/bar/")
}

// Ported from Django: tests/generic_views/test_base.py::RedirectViewTest.test_preserve_request_permanent_redirect
func TestRedirectViewPreserveRequestPermanentRedirect(t *testing.T) {
	view := views.RedirectView{URL: "/bar/", PreserveRequest: true, Permanent: true}
	assertRedirect(t, redirectOf(view, "GET", "/foo/"), http.StatusPermanentRedirect, "/bar/")
}

// Ported from Django: tests/generic_views/test_base.py::RedirectViewTest.test_include_args
func TestRedirectViewIncludeArgs(t *testing.T) {
	assertRedirect(t, redirectOf(views.RedirectView{URL: "/bar/"}, "GET", "/foo/?pork=spam"),
		http.StatusFound, "/bar/")
	view := views.RedirectView{URL: "/bar/", QueryString: true}
	assertRedirect(t, redirectOf(view, "GET", "/foo/?pork=spam"), http.StatusFound, "/bar/?pork=spam")
}

// Ported from Django: tests/generic_views/test_base.py::RedirectViewTest.test_include_urlencoded_args
func TestRedirectViewIncludeURLEncodedArgs(t *testing.T) {
	view := views.RedirectView{URL: "/bar/", QueryString: true}
	assertRedirect(t, redirectOf(view, "GET", "/foo/?unicode=%E2%9C%93"),
		http.StatusFound, "/bar/?unicode=%E2%9C%93")
}

// Ported from Django: tests/generic_views/test_base.py::RedirectViewTest.test_parameter_substitution
//
// Django writes the placeholder as %(object_id)d and fills it from the keyword
// arguments of the view. The Go view writes {object_id} and fills it from the
// path values of the route.
func TestRedirectViewParameterSubstitution(t *testing.T) {
	c := mount(t, "/foo/{object_id}/{$}", views.RedirectView{URL: "/bar/{object_id}/"})
	resp := c.Get("/foo/42/")
	djangotest.AssertStatus(t, resp, http.StatusFound)
	djangotest.AssertEqual(t, resp.URL(), "/bar/42/")
}

// TestRedirectViewEncodesPathValues asserts that a path value cannot add a
// path segment or a query parameter to the target: it is percent-encoded.
func TestRedirectViewEncodesPathValues(t *testing.T) {
	c := mount(t, "/foo/{name}/{$}", views.RedirectView{URL: "/bar/{name}/"})
	resp := c.Get("/foo/a%2Fb%3Fc=d/")
	djangotest.AssertStatus(t, resp, http.StatusFound)
	djangotest.AssertEqual(t, resp.URL(), "/bar/a%2Fb%3Fc%3Dd/")
}

// Ported from Django: tests/generic_views/test_base.py::RedirectViewTest.test_named_url_pattern
// Ported from Django: tests/generic_views/test_base.py::RedirectViewTest.test_named_url_pattern_using_args
//
// Django passes the captured arguments by keyword in the first test and by
// position in the second. A Go route has named path values only, and the view
// hands them to the resolver in pattern order.
func TestRedirectViewNamedURLPattern(t *testing.T) {
	router := urls.NewRouter(urls.Path("/detail/artist/{pk}/{$}", http.NotFoundHandler(), "artist_detail"))
	c := mount(t, "/foo/{pk}/{$}", views.RedirectView{PatternName: "artist_detail", Resolver: router.Reverse})
	resp := c.Get("/foo/1/")
	djangotest.AssertStatus(t, resp, http.StatusFound)
	djangotest.AssertEqual(t, resp.URL(), "/detail/artist/1/")
}

// TestRedirectViewUnknownPattern asserts that a pattern name that cannot be
// reversed is answered with the 500 page, where Django raises NoReverseMatch.
func TestRedirectViewUnknownPattern(t *testing.T) {
	logs := captureLogs(t)
	router := urls.NewRouter()
	view := views.RedirectView{PatternName: "missing", Resolver: router.Reverse}
	djangotest.AssertEqual(t, redirectOf(view, "GET", "/foo/").Code, http.StatusInternalServerError)
	djangotest.AssertEqual(t, len(logs.errs()), 1)
}

// Ported from Django: tests/generic_views/test_base.py::RedirectViewTest.test_redirect_POST
// Ported from Django: tests/generic_views/test_base.py::RedirectViewTest.test_redirect_HEAD
// Ported from Django: tests/generic_views/test_base.py::RedirectViewTest.test_redirect_OPTIONS
// Ported from Django: tests/generic_views/test_base.py::RedirectViewTest.test_redirect_PUT
// Ported from Django: tests/generic_views/test_base.py::RedirectViewTest.test_redirect_PATCH
// Ported from Django: tests/generic_views/test_base.py::RedirectViewTest.test_redirect_DELETE
func TestRedirectViewMethods(t *testing.T) {
	for _, method := range []string{"POST", "HEAD", "OPTIONS", "PUT", "PATCH", "DELETE"} {
		t.Run(method, func(t *testing.T) {
			assertRedirect(t, redirectOf(views.RedirectView{URL: "/bar/"}, method, "/foo/"), http.StatusFound, "/bar/")
		})
	}
}

// Ported from Django: tests/generic_views/test_base.py::RedirectViewTest.test_gone_response_logged
func TestRedirectViewGoneResponseLogged(t *testing.T) {
	for _, tc := range loggedPaths {
		t.Run(tc.name, func(t *testing.T) {
			logs := captureLogs(t)
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.URL.Path = tc.path
			rec := httptest.NewRecorder()
			views.RedirectView{}.ServeHTTP(rec, req)
			djangotest.AssertEqual(t, rec.Code, http.StatusGone)
			djangotest.AssertSequenceEqual(t, logs.messages(slog.LevelWarn), []string{"Gone: " + tc.escaped})
			djangotest.AssertSequenceEqual(t, logs.statuses(slog.LevelWarn), []int64{http.StatusGone})
		})
	}
}

// Ported from Django: tests/generic_views/test_base.py::RedirectViewTest.test_redirect_with_querry_string_in_destination
// Ported from Django: tests/generic_views/test_base.py::RedirectViewTest.test_redirect_with_query_string_in_destination_and_request
// Ported from Django: tests/generic_views/test_base.py::RedirectViewTest.test_redirect_with_same_query_string_param_will_append_not_replace
func TestRedirectViewQueryStringInDestination(t *testing.T) {
	view := views.RedirectView{URL: "/bar/?pork=spam", QueryString: true}
	for _, tc := range []struct{ request, want string }{
		{"/foo", "/bar/?pork=spam"},
		{"/foo/?utm_source=social", "/bar/?pork=spam&utm_source=social"},
		{"/foo/?utm_source=social&pork=ham", "/bar/?pork=spam&utm_source=social&pork=ham"},
	} {
		assertRedirect(t, redirectOf(view, "GET", tc.request), http.StatusFound, tc.want)
	}
}
