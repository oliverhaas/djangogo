package urls_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/urls"
)

// pageText is the body of the routes of middlewareURLs.
const pageText = "Here's the text of the web page."

// optOutView is a handler type that opts out of the slash redirect itself,
// the counterpart of Django's class-based view with no_append_slash.
type optOutView struct{}

// ServeHTTP answers with an empty 200.
func (optOutView) ServeHTTP(http.ResponseWriter, *http.Request) {}

// ShouldAppendSlash opts out of the redirect.
func (optOutView) ShouldAppendSlash() bool { return false }

// middlewareURLs is the part of tests/middleware/urls.py the slash tests use.
// Django's CommonMiddleware does this work; in Go the router does, so the
// settings APPEND_SLASH and DEBUG become the router's fields.
func middlewareURLs(appendSlash, debug bool) *urls.Router {
	page := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(pageText)) })
	router := urls.NewRouter(
		urls.Path("noslash", page, ""),
		urls.Path("slash/", page, ""),
		urls.Path("needsquoting#/", page, ""),
		urls.RePath(`^(.+)/security/$`, page, ""),
		urls.Path("sensitive_fbv/", urls.NoAppendSlash(page), ""),
		urls.Path("sensitive_cbv/", optOutView{}, ""),
	)
	router.AppendSlash = appendSlash
	router.Debug = debug
	return router
}

// serve runs one request through the router.
func serve(router http.Handler, method, target string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(method, target, nil))
	return rec
}

// TestAppendSlash runs the slash cases of Django's CommonMiddlewareTest.
//
// Ported from Django: tests/middleware/tests.py::CommonMiddlewareTest.test_append_slash_have_slash
// Ported from Django: tests/middleware/tests.py::CommonMiddlewareTest.test_append_slash_slashless_resource
// Ported from Django: tests/middleware/tests.py::CommonMiddlewareTest.test_append_slash_slashless_unknown
// Ported from Django: tests/middleware/tests.py::CommonMiddlewareTest.test_append_slash_redirect
// Ported from Django: tests/middleware/tests.py::CommonMiddlewareTest.test_append_slash_redirect_querystring
// Ported from Django: tests/middleware/tests.py::CommonMiddlewareTest.test_append_slash_redirect_querystring_have_slash
// Ported from Django: tests/middleware/tests.py::CommonMiddlewareTest.test_append_slash_disabled
// Ported from Django: tests/middleware/tests.py::CommonMiddlewareTest.test_append_slash_opt_out
// Ported from Django: tests/middleware/tests.py::CommonMiddlewareTest.test_append_slash_quoted
func TestAppendSlash(t *testing.T) {
	tests := []struct {
		django      string
		appendSlash bool
		target      string
		status      int
		location    string
		body        string
	}{
		{"test_append_slash_have_slash", true, "/slash/", 200, "", pageText},
		{"test_append_slash_slashless_resource", true, "/noslash", 200, "", pageText},
		{"test_append_slash_slashless_unknown", true, "/unknown", 404, "", ""},
		{"test_append_slash_redirect", true, "/slash", 301, "/slash/", ""},
		{"test_append_slash_redirect_querystring", true, "/slash?test=1", 301, "/slash/?test=1", ""},
		{"test_append_slash_redirect_querystring_have_slash", true, "/slash?test=slash/", 301, "/slash/?test=slash/", ""},
		{"test_append_slash_disabled", false, "/slash", 404, "", ""},
		{"test_append_slash_opt_out", true, "/sensitive_fbv", 404, "", ""},
		{"test_append_slash_opt_out", true, "/sensitive_cbv", 404, "", ""},
		{"test_append_slash_quoted", true, "/needsquoting%23", 301, "/needsquoting%23/", ""},
	}
	for _, tt := range tests {
		t.Run(tt.django+tt.target, func(t *testing.T) {
			rec := serve(middlewareURLs(tt.appendSlash, false), http.MethodGet, tt.target)
			if rec.Code != tt.status {
				t.Fatalf("status = %d, want %d", rec.Code, tt.status)
			}
			if got := rec.Header().Get("Location"); got != tt.location {
				t.Errorf("Location = %q, want %q", got, tt.location)
			}
			if tt.body != "" && rec.Body.String() != tt.body {
				t.Errorf("body = %q, want %q", rec.Body.String(), tt.body)
			}
		})
	}
}

// TestAppendSlashNoRedirectInDebug checks that, with Debug on, a request that
// carries a body is refused with Django's explanation instead of being
// redirected and losing the body. Django raises RuntimeError; the router
// answers 500 with the same text.
//
// Ported from Django: tests/middleware/tests.py::CommonMiddlewareTest.test_append_slash_no_redirect_in_DEBUG
func TestAppendSlashNoRedirectInDebug(t *testing.T) {
	router := middlewareURLs(true, true)
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		rec := serve(router, method, "/slash")
		if rec.Code != http.StatusInternalServerError {
			t.Errorf("%s: status = %d, want 500", method, rec.Code)
		}
		want := "maintaining " + method + " data. Change your form to point to example.com/slash/"
		if !strings.Contains(rec.Body.String(), want) {
			t.Errorf("%s: body = %q, want it to contain %q", method, rec.Body.String(), want)
		}
	}
	if rec := serve(router, http.MethodGet, "/slash"); rec.Code != http.StatusMovedPermanently {
		t.Errorf("GET: status = %d, want 301", rec.Code)
	}
	if rec := serve(middlewareURLs(true, false), http.MethodPost, "/slash"); rec.Code != http.StatusMovedPermanently {
		t.Errorf("POST without Debug: status = %d, want 301", rec.Code)
	}
}

// TestAppendSlashLeadingSlashes checks that a path starting with two slashes
// is not redirected to a scheme-relative URL, which a browser would follow to
// another host.
//
// Ported from Django: tests/middleware/tests.py::CommonMiddlewareTest.test_append_slash_leading_slashes
func TestAppendSlashLeadingSlashes(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.URL = &url.URL{Path: "//evil.com/security"}
	rec := httptest.NewRecorder()
	middlewareURLs(true, false).ServeHTTP(rec, req)
	if rec.Code != http.StatusMovedPermanently {
		t.Fatalf("status = %d, want 301", rec.Code)
	}
	if got, want := rec.Header().Get("Location"), "/%2Fevil.com/security/"; got != want {
		t.Errorf("Location = %q, want %q", got, want)
	}
}
