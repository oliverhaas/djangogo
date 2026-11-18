package middleware_test

import (
	"crypto/tls"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp/syntax"
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/middleware"
)

// mustSecurity builds the Security middleware or fails the test.
func mustSecurity(t *testing.T, opts middleware.SecurityOptions) func(http.Handler) http.Handler {
	t.Helper()
	mw, err := middleware.Security(opts)
	if err != nil {
		t.Fatalf("Security(%+v): %v", opts, err)
	}
	return mw
}

// TestSecurityRejectsBadOptions checks the two errors of the constructor: a
// RedirectExempt pattern that does not compile, with the error of the regexp
// package wrapped, and a ProxySSLHeader that is not a pair.
func TestSecurityRejectsBadOptions(t *testing.T) {
	_, err := middleware.Security(middleware.SecurityOptions{RedirectExempt: []string{"^ok/", "("}})
	var syntaxErr *syntax.Error
	if !errors.As(err, &syntaxErr) {
		t.Errorf("RedirectExempt with a bad pattern: got error %v, want a wrapped *syntax.Error", err)
	}

	for _, header := range [][]string{
		{"X-Forwarded-Proto"},
		{"X-Forwarded-Proto", "https", "extra"},
	} {
		if _, err := middleware.Security(middleware.SecurityOptions{ProxySSLHeader: header}); err == nil {
			t.Errorf("ProxySSLHeader %q: got no error", header)
		}
	}
}

// TestSecurityRedirectUsesValidatedHost checks that the redirect goes to the
// host AllowedHosts validated, which is the forwarded host behind a proxy, and
// keeps the path and the query.
func TestSecurityRedirectUsesValidatedHost(t *testing.T) {
	handler := middleware.Chain(
		middleware.AllowedHosts(middleware.AllowedHostsOptions{
			AllowedHosts:      []string{"public.example"},
			UseXForwardedHost: true,
		}),
		mustSecurity(t, middleware.SecurityOptions{SSLRedirect: true}),
	)(emptyView)

	resp := djangotest.NewClient(t, handler).Get("/some/path/?page=2&q=a+b",
		djangotest.Header("Host", "internal:8000"),
		djangotest.Header("X-Forwarded-Host", "public.example"))
	djangotest.AssertStatus(t, resp, http.StatusMovedPermanently)
	djangotest.AssertEqual(t, resp.Header.Get("Location"), "https://public.example/some/path/?page=2&q=a+b")
}

// TestSecurityRedirectRejectsInvalidHost checks that, without AllowedHosts in
// front, a Host header that is not a host does not end up in a Location
// header.
func TestSecurityRedirectRejectsInvalidHost(t *testing.T) {
	handler := mustSecurity(t, middleware.SecurityOptions{SSLRedirect: true})(emptyView)

	for _, host := range []string{"evil.com/path", "evil.com\r\nSet-Cookie: a=b", "user@evil.com", ""} {
		t.Run(host, func(t *testing.T) {
			rec := serveHost(handler, host)
			djangotest.AssertEqual(t, rec.Code, http.StatusBadRequest)
			djangotest.AssertEqual(t, rec.Header().Values("Location"), nil)
		})
	}
}

// TestSecurityRedirectKeepsHost checks that a path cannot change the host of
// the redirect: a path that starts with two slashes stays a path.
func TestSecurityRedirectKeepsHost(t *testing.T) {
	handler := mustSecurity(t, middleware.SecurityOptions{SSLRedirect: true})(emptyView)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, &http.Request{
		Method: http.MethodGet,
		URL:    &url.URL{Path: "//evil.com/x"},
		Host:   "testserver",
		Header: make(http.Header),
	})
	djangotest.AssertEqual(t, rec.Code, http.StatusMovedPermanently)
	djangotest.AssertEqual(t, rec.Header().Get("Location"), "https://testserver//evil.com/x")
}

// TestSecurityRedirectCarriesHeaders checks that the redirect gets the
// response headers as well, as Django's process_response sees the redirect of
// process_request. It is not secure, so it gets no HSTS header.
func TestSecurityRedirectCarriesHeaders(t *testing.T) {
	handler := mustSecurity(t, middleware.SecurityOptions{
		SSLRedirect:        true,
		HSTSSeconds:        3600,
		ContentTypeNosniff: true,
		ReferrerPolicy:     "same-origin",
	})(emptyView)

	resp := djangotest.NewClient(t, handler).Get("/")
	djangotest.AssertStatus(t, resp, http.StatusMovedPermanently)
	djangotest.AssertEqual(t, resp.Header.Get("X-Content-Type-Options"), "nosniff")
	djangotest.AssertEqual(t, resp.Header.Get("Referrer-Policy"), "same-origin")
	assertNoHeader(t, resp, "Strict-Transport-Security")
}

// TestSecurityHeadersWithoutWrite checks that a handler that never touches the
// response writer still gets the headers: net/http sends its response after
// the handler returned.
func TestSecurityHeadersWithoutWrite(t *testing.T) {
	silent := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})
	handler := middleware.Chain(
		mustSecurity(t, middleware.SecurityOptions{HSTSSeconds: 60, ContentTypeNosniff: true}),
		middleware.XFrameOptions("SAMEORIGIN"),
	)(silent)

	resp := djangotest.NewClient(t, handler).Get("/", djangotest.Secure())
	djangotest.AssertStatus(t, resp, http.StatusOK)
	djangotest.AssertEqual(t, resp.Header.Get("Strict-Transport-Security"), "max-age=60")
	djangotest.AssertEqual(t, resp.Header.Get("X-Content-Type-Options"), "nosniff")
	djangotest.AssertEqual(t, resp.Header.Get("X-Frame-Options"), "SAMEORIGIN")
}

// TestSecurityHeadersOnErrorPage checks that the 500 page Recover sends for a
// panic below the middleware carries the headers too.
func TestSecurityHeadersOnErrorPage(t *testing.T) {
	handler := middleware.Chain(
		middleware.Recover(middleware.RecoverOptions{Log: io.Discard}),
		mustSecurity(t, middleware.SecurityOptions{ContentTypeNosniff: true, CrossOriginOpenerPolicy: "same-origin"}),
		middleware.XFrameOptions("DENY"),
	)(raises500("boom"))

	resp := djangotest.NewClient(t, handler).Get("/")
	djangotest.AssertStatus(t, resp, http.StatusInternalServerError)
	djangotest.AssertEqual(t, resp.Header.Get("X-Content-Type-Options"), "nosniff")
	djangotest.AssertEqual(t, resp.Header.Get("Cross-Origin-Opener-Policy"), "same-origin")
	djangotest.AssertEqual(t, resp.Header.Get("X-Frame-Options"), "DENY")
}

// TestSecurityHeadersBeforeFlush checks that the headers are in place when a
// handler starts its response with a flush.
func TestSecurityHeadersBeforeFlush(t *testing.T) {
	flushing := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if err := http.NewResponseController(w).Flush(); err != nil {
			t.Errorf("flush: %v", err)
		}
		_, _ = io.WriteString(w, "late")
	})
	handler := mustSecurity(t, middleware.SecurityOptions{ContentTypeNosniff: true})(flushing)

	resp := djangotest.NewClient(t, handler).Get("/")
	djangotest.AssertEqual(t, resp.Header.Get("X-Content-Type-Options"), "nosniff")
	djangotest.AssertEqual(t, resp.Content(), "late")
}

// TestIsSecureHeaderSpelling checks that the proxy header can be named as an
// HTTP header as well as in Django's META spelling, and that a pair that is
// not a pair counts as unset.
func TestIsSecureHeaderSpelling(t *testing.T) {
	forwarded := plainRequest("X-Forwarded-Proto", "https")
	djangotest.AssertEqual(t, middleware.IsSecure(forwarded, []string{"X-Forwarded-Proto", "https"}), true)
	djangotest.AssertEqual(t, middleware.IsSecure(forwarded, []string{"x-forwarded-proto", "https"}), true)
	djangotest.AssertEqual(t, middleware.IsSecure(forwarded, []string{"HTTP_X_FORWARDED_PROTO", "https"}), true)
	djangotest.AssertEqual(t, middleware.IsSecure(forwarded, []string{"X-Forwarded-Proto"}), false)

	overTLS := plainRequest()
	overTLS.TLS = &tls.ConnectionState{}
	djangotest.AssertEqual(t, middleware.IsSecure(overTLS, []string{"X-Forwarded-Proto"}), true)
}
