package views_test

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/views"
)

// okView is the wrapped view of the decorator tests: it answers "OK".
var okView = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
	_, _ = w.Write([]byte("OK"))
})

// serveMethod sends a request with the given method to h and returns the
// recorded response.
func serveMethod(h http.Handler, method string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, "/view/", nil))
	return rec
}

// Ported from Django: tests/decorators/test_http.py::RequireHttpMethodsTest.test_require_http_methods_methods
//
// Django compares the response class. The Go test compares the status and, for
// a refused method, the Allow header and the untouched view.
func TestRequireHTTPMethodsMethods(t *testing.T) {
	view := views.RequireMethods("GET", "PUT")(okView)
	for _, method := range []string{"GET", "PUT"} {
		rec := serveMethod(view, method)
		djangotest.AssertEqual(t, rec.Code, http.StatusOK)
		djangotest.AssertEqual(t, rec.Body.String(), "OK")
	}
	for _, method := range []string{"HEAD", "POST", "DELETE"} {
		rec := serveMethod(view, method)
		djangotest.AssertEqual(t, rec.Code, http.StatusMethodNotAllowed)
		djangotest.AssertEqual(t, rec.Header().Get("Allow"), "GET, PUT")
		djangotest.AssertEqual(t, rec.Body.String(), "")
	}
}

// Ported from Django: tests/decorators/test_http.py::RequireSafeDecoratorTest.test_require_safe_accepts_only_safe_methods
func TestRequireSafeAcceptsOnlySafeMethods(t *testing.T) {
	view := views.RequireSafe(okView)
	for _, method := range []string{"GET", "HEAD"} {
		djangotest.AssertEqual(t, serveMethod(view, method).Code, http.StatusOK)
	}
	for _, method := range []string{"POST", "PUT", "DELETE"} {
		rec := serveMethod(view, method)
		djangotest.AssertEqual(t, rec.Code, http.StatusMethodNotAllowed)
		djangotest.AssertEqual(t, rec.Header().Get("Allow"), "GET, HEAD")
	}
}

// TestRequireGETAndPOST covers require_GET and require_POST, which Django
// defines as require_http_methods(["GET"]) and require_http_methods(["POST"]).
func TestRequireGETAndPOST(t *testing.T) {
	djangotest.AssertEqual(t, serveMethod(views.RequireGET(okView), "GET").Code, http.StatusOK)
	djangotest.AssertEqual(t, serveMethod(views.RequireGET(okView), "POST").Code, http.StatusMethodNotAllowed)
	djangotest.AssertEqual(t, serveMethod(views.RequirePOST(okView), "POST").Code, http.StatusOK)
	rec := serveMethod(views.RequirePOST(okView), "GET")
	djangotest.AssertEqual(t, rec.Code, http.StatusMethodNotAllowed)
	djangotest.AssertEqual(t, rec.Header().Get("Allow"), "POST")
}

// TestNotAllowedLogs asserts the warning Django's require_http_methods logs
// for a refused request, with control characters of the request escaped.
func TestNotAllowedLogs(t *testing.T) {
	logs := captureLogs(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/view/", nil)
	req.URL.Path = "/view/\nforged line"
	views.NotAllowed(rec, req, "GET", "HEAD")
	djangotest.AssertEqual(t, rec.Code, http.StatusMethodNotAllowed)
	djangotest.AssertEqual(t, rec.Header().Get("Allow"), "GET, HEAD")
	djangotest.AssertSequenceEqual(t, logs.messages(slog.LevelWarn),
		[]string{`Method Not Allowed (POST): /view/\nforged line`})
}
