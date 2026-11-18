package middleware_test

import (
	"net/http"
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/middleware"
)

// The tests in this file are ports of XFrameOptionsMiddlewareTest in Django's
// tests/middleware/tests.py. The setting X_FRAME_OPTIONS is the argument of
// XFrameOptions.

// emptyView is the port of get_response_empty: a view that answers with an
// empty 200.
var emptyView = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
})

// xframeHeaders sends a GET through handler and returns the X-Frame-Options
// values of the response.
func xframeHeaders(t *testing.T, handler http.Handler, opts ...djangotest.RequestOption) []string {
	t.Helper()
	resp := djangotest.NewClient(t, handler).Get("/", opts...)
	djangotest.AssertStatus(t, resp, http.StatusOK)
	return resp.Header.Values("X-Frame-Options")
}

// TestSameOrigin checks that the value can be set to SAMEORIGIN, in either
// case, to have the middleware use that value for the HTTP header.
//
// Ported from Django: tests/middleware/tests.py::XFrameOptionsMiddlewareTest.test_same_origin
func TestSameOrigin(t *testing.T) {
	djangotest.AssertEqual(t, xframeHeaders(t, middleware.XFrameOptions("SAMEORIGIN")(emptyView)), []string{"SAMEORIGIN"})
	djangotest.AssertEqual(t, xframeHeaders(t, middleware.XFrameOptions("sameorigin")(emptyView)), []string{"SAMEORIGIN"})
}

// TestDeny checks that the value can be set to DENY, in either case, to have
// the middleware use that value for the HTTP header.
//
// Ported from Django: tests/middleware/tests.py::XFrameOptionsMiddlewareTest.test_deny
func TestDeny(t *testing.T) {
	djangotest.AssertEqual(t, xframeHeaders(t, middleware.XFrameOptions("DENY")(emptyView)), []string{"DENY"})
	djangotest.AssertEqual(t, xframeHeaders(t, middleware.XFrameOptions("deny")(emptyView)), []string{"DENY"})
}

// TestDefaultsSameorigin checks that the header defaults to DENY if the value
// is not set (the name of the Django test is older than that default).
//
// Ported from Django: tests/middleware/tests.py::XFrameOptionsMiddlewareTest.test_defaults_sameorigin
func TestDefaultsSameorigin(t *testing.T) {
	djangotest.AssertEqual(t, xframeHeaders(t, middleware.XFrameOptions("")(emptyView)), []string{"DENY"})
}

// TestDontSetIfSet checks that the middleware does not override an
// X-Frame-Options header that is already set.
//
// Ported from Django: tests/middleware/tests.py::XFrameOptionsMiddlewareTest.test_dont_set_if_set
func TestDontSetIfSet(t *testing.T) {
	sameOriginResponse := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Frame-Options", "SAMEORIGIN")
		w.WriteHeader(http.StatusOK)
	})
	denyResponse := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Frame-Options", "DENY")
		w.WriteHeader(http.StatusOK)
	})

	djangotest.AssertEqual(t, xframeHeaders(t, middleware.XFrameOptions("DENY")(sameOriginResponse)), []string{"SAMEORIGIN"})
	djangotest.AssertEqual(t, xframeHeaders(t, middleware.XFrameOptions("SAMEORIGIN")(denyResponse)), []string{"DENY"})
}

// TestResponseExempt checks that the header is still set for a response that is
// not exempt, and left out for an exempt one. Django marks the response with
// the attribute xframe_options_exempt; here the view calls ExemptXFrameOptions.
//
// Ported from Django: tests/middleware/tests.py::XFrameOptionsMiddlewareTest.test_response_exempt
func TestResponseExempt(t *testing.T) {
	xframeExemptResponse := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		middleware.ExemptXFrameOptions(r)
		w.WriteHeader(http.StatusOK)
	})
	xframeNotExemptResponse := emptyView

	mw := middleware.XFrameOptions("SAMEORIGIN")
	djangotest.AssertEqual(t, xframeHeaders(t, mw(xframeNotExemptResponse)), []string{"SAMEORIGIN"})
	djangotest.AssertEqual(t, xframeHeaders(t, mw(xframeExemptResponse)), nil)
}

// TestIsExtendable asserts the Go behavior. Django subclasses the middleware
// and overrides get_xframe_options_value to choose the value from the request
// or the response. XFrameOptions has one fixed value, and it never replaces a
// header that is already set, so the value is chosen per request by a
// middleware placed inside it (from the request) or by the view (from the
// response).
//
// Ported from Django: tests/middleware/tests.py::XFrameOptionsMiddlewareTest.test_is_extendable
func TestIsExtendable(t *testing.T) {
	// otherXFrameOptions is the port of OtherXFrameOptionsMiddleware: it
	// answers SAMEORIGIN for a request marked as such and DENY otherwise.
	otherXFrameOptions := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			value := "DENY"
			if r.Header.Get("X-Test-Sameorigin") != "" {
				value = "SAMEORIGIN"
			}
			w.Header().Set("X-Frame-Options", value)
			next.ServeHTTP(w, r)
		})
	}
	sameOriginResponse := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Frame-Options", "SAMEORIGIN")
		w.WriteHeader(http.StatusOK)
	})

	deny := middleware.Chain(middleware.XFrameOptions("DENY"), otherXFrameOptions)
	djangotest.AssertEqual(t, xframeHeaders(t, deny(sameOriginResponse)), []string{"SAMEORIGIN"})
	djangotest.AssertEqual(t,
		xframeHeaders(t, deny(emptyView), djangotest.Header("X-Test-Sameorigin", "1")),
		[]string{"SAMEORIGIN"})

	sameOrigin := middleware.Chain(middleware.XFrameOptions("SAMEORIGIN"), otherXFrameOptions)
	djangotest.AssertEqual(t, xframeHeaders(t, sameOrigin(emptyView)), []string{"DENY"})
}
