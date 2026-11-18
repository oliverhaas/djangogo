package middleware_test

import (
	"net/http"
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/middleware"
)

// The tests in this file are ports of Django's tests/middleware/test_security.py.
// A Django setting becomes a field of SecurityOptions, and the zero value of a
// field stands for the setting being off or None.

// securityView is the port of SecurityMiddlewareTest.response: a view that
// answers 200 with the given headers already set.
func securityView(headers map[string]string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		for k, v := range headers {
			w.Header().Set(k, v)
		}
		w.WriteHeader(http.StatusOK)
	})
}

// securityHandler wraps securityView in the Security middleware built from opts.
func securityHandler(t *testing.T, opts middleware.SecurityOptions, headers map[string]string) http.Handler {
	t.Helper()
	mw, err := middleware.Security(opts)
	if err != nil {
		t.Fatalf("Security(%+v): %v", opts, err)
	}
	return mw(securityView(headers))
}

// processResponse is the port of SecurityMiddlewareTest.process_response: it
// sends GET /some/url through the middleware, over HTTPS when secure is set,
// and returns the response.
func processResponse(t *testing.T, opts middleware.SecurityOptions, secure bool, headers map[string]string) *djangotest.Response {
	t.Helper()
	c := djangotest.NewClient(t, securityHandler(t, opts, headers))
	if secure {
		return c.Get("/some/url", djangotest.Secure())
	}
	return c.Get("/some/url")
}

// processRequest is the port of SecurityMiddlewareTest.process_request. Django
// calls the hook alone and gets a redirect or None. Here the request passes the
// whole middleware: a redirect is the same response, and None shows as the 200
// of the view.
func processRequest(t *testing.T, opts middleware.SecurityOptions, path string, secure bool) *djangotest.Response {
	t.Helper()
	c := djangotest.NewClient(t, securityHandler(t, opts, nil))
	if secure {
		return c.Get(path, djangotest.Secure())
	}
	return c.Get(path)
}

// assertNotRedirected is the counterpart of assertIsNone(ret) for
// process_request: the request reached the view.
func assertNotRedirected(t *testing.T, resp *djangotest.Response) {
	t.Helper()
	djangotest.AssertStatus(t, resp, http.StatusOK)
	assertNoHeader(t, resp, "Location")
}

// assertNoHeader is the counterpart of assertNotIn(name, response.headers).
func assertNoHeader(t *testing.T, resp *djangotest.Response, name string) {
	t.Helper()
	if values, ok := resp.Header[http.CanonicalHeaderKey(name)]; ok {
		t.Errorf("response has an unexpected %s header: %q", name, values)
	}
}

// TestSTSOn checks that with HSTSSeconds 3600 the middleware adds
// "Strict-Transport-Security: max-age=3600" to the response.
//
// Ported from Django: tests/middleware/test_security.py::SecurityMiddlewareTest.test_sts_on
func TestSTSOn(t *testing.T) {
	resp := processResponse(t, middleware.SecurityOptions{HSTSSeconds: 3600}, true, nil)
	djangotest.AssertEqual(t, resp.Header.Get("Strict-Transport-Security"), "max-age=3600")
}

// TestSTSAlreadyPresent checks that the middleware does not override a
// "Strict-Transport-Security" header already present in the response.
//
// Ported from Django: tests/middleware/test_security.py::SecurityMiddlewareTest.test_sts_already_present
func TestSTSAlreadyPresent(t *testing.T) {
	resp := processResponse(t, middleware.SecurityOptions{HSTSSeconds: 3600}, true,
		map[string]string{"Strict-Transport-Security": "max-age=7200"})
	djangotest.AssertEqual(t, resp.Header.Values("Strict-Transport-Security"), []string{"max-age=7200"})
}

// TestSTSOnlyIfSecure checks that the "Strict-Transport-Security" header is not
// added to responses going over an insecure connection.
//
// Ported from Django: tests/middleware/test_security.py::SecurityMiddlewareTest.test_sts_only_if_secure
func TestSTSOnlyIfSecure(t *testing.T) {
	resp := processResponse(t, middleware.SecurityOptions{HSTSSeconds: 3600}, false, nil)
	assertNoHeader(t, resp, "Strict-Transport-Security")
}

// TestSTSOff checks that with HSTSSeconds 0 the middleware does not add a
// "Strict-Transport-Security" header to the response.
//
// Ported from Django: tests/middleware/test_security.py::SecurityMiddlewareTest.test_sts_off
func TestSTSOff(t *testing.T) {
	resp := processResponse(t, middleware.SecurityOptions{HSTSSeconds: 0}, true, nil)
	assertNoHeader(t, resp, "Strict-Transport-Security")
}

// TestSTSIncludeSubdomains checks that with HSTSSeconds non-zero and
// HSTSIncludeSubdomains true the header carries the "includeSubDomains"
// directive.
//
// Ported from Django: tests/middleware/test_security.py::SecurityMiddlewareTest.test_sts_include_subdomains
func TestSTSIncludeSubdomains(t *testing.T) {
	resp := processResponse(t, middleware.SecurityOptions{HSTSSeconds: 600, HSTSIncludeSubdomains: true}, true, nil)
	djangotest.AssertEqual(t, resp.Header.Get("Strict-Transport-Security"), "max-age=600; includeSubDomains")
}

// TestSTSNoIncludeSubdomains checks that with HSTSSeconds non-zero and
// HSTSIncludeSubdomains false the header has no "includeSubDomains" directive.
//
// Ported from Django: tests/middleware/test_security.py::SecurityMiddlewareTest.test_sts_no_include_subdomains
func TestSTSNoIncludeSubdomains(t *testing.T) {
	resp := processResponse(t, middleware.SecurityOptions{HSTSSeconds: 600, HSTSIncludeSubdomains: false}, true, nil)
	djangotest.AssertEqual(t, resp.Header.Get("Strict-Transport-Security"), "max-age=600")
}

// TestSTSPreload checks that with HSTSSeconds non-zero and HSTSPreload true the
// header carries the "preload" directive.
//
// Ported from Django: tests/middleware/test_security.py::SecurityMiddlewareTest.test_sts_preload
func TestSTSPreload(t *testing.T) {
	resp := processResponse(t, middleware.SecurityOptions{HSTSSeconds: 10886400, HSTSPreload: true}, true, nil)
	djangotest.AssertEqual(t, resp.Header.Get("Strict-Transport-Security"), "max-age=10886400; preload")
}

// TestSTSSubdomainsAndPreload checks that with HSTSSeconds non-zero,
// HSTSIncludeSubdomains and HSTSPreload true the header carries both the
// "includeSubDomains" and the "preload" directive.
//
// Ported from Django: tests/middleware/test_security.py::SecurityMiddlewareTest.test_sts_subdomains_and_preload
func TestSTSSubdomainsAndPreload(t *testing.T) {
	resp := processResponse(t, middleware.SecurityOptions{
		HSTSSeconds:           10886400,
		HSTSIncludeSubdomains: true,
		HSTSPreload:           true,
	}, true, nil)
	djangotest.AssertEqual(t, resp.Header.Get("Strict-Transport-Security"), "max-age=10886400; includeSubDomains; preload")
}

// TestSTSNoPreload checks that with HSTSSeconds non-zero and HSTSPreload false
// the header has no "preload" directive.
//
// Ported from Django: tests/middleware/test_security.py::SecurityMiddlewareTest.test_sts_no_preload
func TestSTSNoPreload(t *testing.T) {
	resp := processResponse(t, middleware.SecurityOptions{HSTSSeconds: 10886400, HSTSPreload: false}, true, nil)
	djangotest.AssertEqual(t, resp.Header.Get("Strict-Transport-Security"), "max-age=10886400")
}

// TestContentTypeOn checks that with ContentTypeNosniff true the middleware
// adds the "X-Content-Type-Options: nosniff" header to the response.
//
// Ported from Django: tests/middleware/test_security.py::SecurityMiddlewareTest.test_content_type_on
func TestContentTypeOn(t *testing.T) {
	resp := processResponse(t, middleware.SecurityOptions{ContentTypeNosniff: true}, false, nil)
	djangotest.AssertEqual(t, resp.Header.Get("X-Content-Type-Options"), "nosniff")
}

// TestContentTypeAlreadyPresent checks that the middleware does not override an
// "X-Content-Type-Options" header already present in the response.
//
// Ported from Django: tests/middleware/test_security.py::SecurityMiddlewareTest.test_content_type_already_present
func TestContentTypeAlreadyPresent(t *testing.T) {
	resp := processResponse(t, middleware.SecurityOptions{ContentTypeNosniff: true}, true,
		map[string]string{"X-Content-Type-Options": "foo"})
	djangotest.AssertEqual(t, resp.Header.Values("X-Content-Type-Options"), []string{"foo"})
}

// TestContentTypeOff checks that with ContentTypeNosniff false the middleware
// does not add an "X-Content-Type-Options" header to the response.
//
// Ported from Django: tests/middleware/test_security.py::SecurityMiddlewareTest.test_content_type_off
func TestContentTypeOff(t *testing.T) {
	resp := processResponse(t, middleware.SecurityOptions{ContentTypeNosniff: false}, false, nil)
	assertNoHeader(t, resp, "X-Content-Type-Options")
}

// TestSSLRedirectOn checks that with SSLRedirect true the middleware redirects
// any non-secure request to the https:// version of the same URL.
//
// Ported from Django: tests/middleware/test_security.py::SecurityMiddlewareTest.test_ssl_redirect_on
func TestSSLRedirectOn(t *testing.T) {
	resp := processRequest(t, middleware.SecurityOptions{SSLRedirect: true}, "/some/url?query=string", false)
	djangotest.AssertStatus(t, resp, http.StatusMovedPermanently)
	djangotest.AssertEqual(t, resp.Header.Get("Location"), "https://testserver/some/url?query=string")
}

// TestNoRedirectSSL checks that the middleware does not redirect secure
// requests.
//
// Ported from Django: tests/middleware/test_security.py::SecurityMiddlewareTest.test_no_redirect_ssl
func TestNoRedirectSSL(t *testing.T) {
	resp := processRequest(t, middleware.SecurityOptions{SSLRedirect: true}, "/some/url", true)
	assertNotRedirected(t, resp)
}

// TestRedirectExempt checks that the middleware does not redirect requests with
// a URL path matching an exempt pattern.
//
// Ported from Django: tests/middleware/test_security.py::SecurityMiddlewareTest.test_redirect_exempt
func TestRedirectExempt(t *testing.T) {
	opts := middleware.SecurityOptions{SSLRedirect: true, RedirectExempt: []string{"^insecure/"}}
	resp := processRequest(t, opts, "/insecure/page", false)
	assertNotRedirected(t, resp)
}

// TestRedirectSSLHost checks that the middleware redirects to SSLHost if given.
//
// Ported from Django: tests/middleware/test_security.py::SecurityMiddlewareTest.test_redirect_ssl_host
func TestRedirectSSLHost(t *testing.T) {
	opts := middleware.SecurityOptions{SSLRedirect: true, SSLHost: "secure.example.com"}
	resp := processRequest(t, opts, "/some/url", false)
	djangotest.AssertStatus(t, resp, http.StatusMovedPermanently)
	djangotest.AssertEqual(t, resp.Header.Get("Location"), "https://secure.example.com/some/url")
}

// TestSSLRedirectOff checks that with SSLRedirect false the middleware does not
// redirect.
//
// Ported from Django: tests/middleware/test_security.py::SecurityMiddlewareTest.test_ssl_redirect_off
func TestSSLRedirectOff(t *testing.T) {
	resp := processRequest(t, middleware.SecurityOptions{SSLRedirect: false}, "/some/url", false)
	assertNotRedirected(t, resp)
}

// TestReferrerPolicyOff checks that with an empty ReferrerPolicy (None in
// Django) the middleware does not add a "Referrer-Policy" header.
//
// Ported from Django: tests/middleware/test_security.py::SecurityMiddlewareTest.test_referrer_policy_off
func TestReferrerPolicyOff(t *testing.T) {
	resp := processResponse(t, middleware.SecurityOptions{ReferrerPolicy: ""}, false, nil)
	assertNoHeader(t, resp, "Referrer-Policy")
}

// TestReferrerPolicyOn checks that with ReferrerPolicy set to a valid value the
// middleware adds a "Referrer-Policy" header to the response. Django also takes
// a list or a tuple of values; ReferrerPolicy is a string, so only the string
// forms are ported.
//
// Ported from Django: tests/middleware/test_security.py::SecurityMiddlewareTest.test_referrer_policy_on
func TestReferrerPolicyOn(t *testing.T) {
	for _, tc := range []struct{ value, expected string }{
		{"strict-origin", "strict-origin"},
		{"strict-origin,origin", "strict-origin,origin"},
		{"strict-origin, origin", "strict-origin,origin"},
	} {
		t.Run(tc.value, func(t *testing.T) {
			resp := processResponse(t, middleware.SecurityOptions{ReferrerPolicy: tc.value}, false, nil)
			djangotest.AssertEqual(t, resp.Header.Get("Referrer-Policy"), tc.expected)
		})
	}
}

// TestReferrerPolicyAlreadyPresent checks that the middleware does not override
// a "Referrer-Policy" header already present in the response.
//
// Ported from Django: tests/middleware/test_security.py::SecurityMiddlewareTest.test_referrer_policy_already_present
func TestReferrerPolicyAlreadyPresent(t *testing.T) {
	resp := processResponse(t, middleware.SecurityOptions{ReferrerPolicy: "strict-origin"}, false,
		map[string]string{"Referrer-Policy": "unsafe-url"})
	djangotest.AssertEqual(t, resp.Header.Values("Referrer-Policy"), []string{"unsafe-url"})
}

// TestCOOPOff checks that with an empty CrossOriginOpenerPolicy (None in
// Django) the middleware does not add a "Cross-Origin-Opener-Policy" header.
//
// Ported from Django: tests/middleware/test_security.py::SecurityMiddlewareTest.test_coop_off
func TestCOOPOff(t *testing.T) {
	resp := processResponse(t, middleware.SecurityOptions{CrossOriginOpenerPolicy: ""}, false, nil)
	assertNoHeader(t, resp, "Cross-Origin-Opener-Policy")
}

// TestCOOPDefault asserts the Go behavior. In Django the setting
// SECURE_CROSS_ORIGIN_OPENER_POLICY defaults to "same-origin". SecurityOptions
// has no defaults of its own: its zero value adds no header, and the default
// belongs to the settings that fill the options in.
//
// Ported from Django: tests/middleware/test_security.py::SecurityMiddlewareTest.test_coop_default
func TestCOOPDefault(t *testing.T) {
	resp := processResponse(t, middleware.SecurityOptions{}, false, nil)
	assertNoHeader(t, resp, "Cross-Origin-Opener-Policy")
}

// TestCOOPOn checks that with CrossOriginOpenerPolicy set to a valid value the
// middleware adds a "Cross-Origin-Opener-Policy" header to the response.
//
// Ported from Django: tests/middleware/test_security.py::SecurityMiddlewareTest.test_coop_on
func TestCOOPOn(t *testing.T) {
	for _, value := range []string{"same-origin", "same-origin-allow-popups", "unsafe-none"} {
		t.Run(value, func(t *testing.T) {
			resp := processResponse(t, middleware.SecurityOptions{CrossOriginOpenerPolicy: value}, false, nil)
			djangotest.AssertEqual(t, resp.Header.Get("Cross-Origin-Opener-Policy"), value)
		})
	}
}

// TestCOOPAlreadyPresent checks that the middleware does not override a
// "Cross-Origin-Opener-Policy" header already present in the response.
//
// Ported from Django: tests/middleware/test_security.py::SecurityMiddlewareTest.test_coop_already_present
func TestCOOPAlreadyPresent(t *testing.T) {
	resp := processResponse(t, middleware.SecurityOptions{CrossOriginOpenerPolicy: "unsafe-none"}, false,
		map[string]string{"Cross-Origin-Opener-Policy": "same-origin"})
	djangotest.AssertEqual(t, resp.Header.Values("Cross-Origin-Opener-Policy"), []string{"same-origin"})
}
