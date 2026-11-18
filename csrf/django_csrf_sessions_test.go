package csrf_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/csrf"
	"github.com/oliverhaas/djangogo/sessions"
)

// sessionMode returns a harness that keeps the secret in the session.
func sessionMode(t *testing.T) *harness {
	t.Helper()
	return &harness{t: t, useSessions: true, store: sessions.NewSignedCookieStore([]byte("csrf-test"))}
}

// Django raises ImproperlyConfigured. A Go middleware cannot raise, so the
// request fails with a 500 that names the cause.
//
// Ported from Django: tests/csrf_tests/tests.py::CsrfViewMiddlewareUseSessionsTests.test_no_session_on_request
func TestNoSessionOnRequest(t *testing.T) {
	called := false
	handler := csrf.MiddlewareWithOptions(csrf.Options{UseSessions: true})(
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", http.NoBody))

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}
	if called {
		t.Error("the view ran without a session")
	}
	if !strings.Contains(rec.Body.String(), "the sessions middleware must run first") {
		t.Errorf("body = %q, want the cause", rec.Body.String())
	}
}

// Ported from Django: tests/csrf_tests/tests.py::CsrfViewMiddlewareUseSessionsTests.test_masked_unmasked_combinations
func TestSessionsMaskedUnmaskedCombinations(t *testing.T) {
	assertCombinationsAccepted(t, sessionMode(t), []combination{
		{maskedTestSecret1, testSecret, nil},
		{maskedTestSecret1, maskedTestSecret2, nil},
		{maskedTestSecret1, "", str(testSecret)},
		{maskedTestSecret1, "", str(maskedTestSecret2)},
	})
}

// Go has no ensure_csrf_cookie decorator. A view that asks for a token under
// the middleware has the same effect: the secret lands in the session.
//
// Ported from Django: tests/csrf_tests/tests.py::CsrfViewMiddlewareUseSessionsTests.test_process_response_get_token_used
func TestSessionsProcessResponseGetTokenUsed(t *testing.T) {
	res := sessionMode(t).do(get(), tokenView)
	if !res.sent || len(res.secret) != csrf.SecretLength {
		t.Errorf("secret in the session = %q (saved = %v), want a new one", res.secret, res.sent)
	}
	if c := res.csrfCookie(csrf.DefaultCookieName); c != nil {
		t.Errorf("a CSRF cookie was sent in session mode: %s", c)
	}
}

// Ported from Django: tests/csrf_tests/tests.py::CsrfViewMiddlewareUseSessionsTests.test_session_modify
func TestSessionModify(t *testing.T) {
	h := sessionMode(t)
	first := h.do(get(), tokenView)
	if !first.sent || first.secret == "" {
		t.Fatal("the first request did not store a secret")
	}
	second := h.do(request{secret: &first.secret}, tokenView)
	if second.sent {
		t.Error("the session was saved although the secret is unchanged")
	}
	checkTokenPresent(t, second, first.secret)
}

// The domain comes from Options.CookieDomain in both modes; Django reads
// SESSION_COOKIE_DOMAIN here.
//
// Ported from Django: tests/csrf_tests/tests.py::CsrfViewMiddlewareUseSessionsTests.test_https_good_referer_matches_cookie_domain
func TestSessionsHTTPSGoodRefererMatchesCookieDomain(t *testing.T) {
	refererMatchesCookieDomain(t, sessionMode(t))
}

// Ported from Django: tests/csrf_tests/tests.py::CsrfViewMiddlewareUseSessionsTests.test_https_good_referer_matches_cookie_domain_with_different_port
func TestSessionsHTTPSGoodRefererMatchesCookieDomainWithDifferentPort(t *testing.T) {
	refererMatchesCookieDomainWithPort(t, sessionMode(t))
}

// Ported from Django: tests/csrf_tests/tests.py::CsrfViewMiddlewareUseSessionsTests.test_https_reject_insecure_referer
func TestSessionsHTTPSRejectInsecureReferer(t *testing.T) {
	rejectInsecureReferer(t, sessionMode(t))
}
