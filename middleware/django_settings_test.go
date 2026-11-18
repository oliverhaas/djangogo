package middleware_test

import (
	"crypto/tls"
	"net/http"
	"net/url"
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/middleware"
)

// The tests in this file are ports of SecureProxySslHeaderTest in Django's
// tests/settings_tests/tests.py. The setting SECURE_PROXY_SSL_HEADER is the
// second argument of IsSecure, and a META key such as HTTP_X_FORWARDED_PROTO is
// the request header X-Forwarded-Proto.

// forwardedProto is the SECURE_PROXY_SSL_HEADER value all of these tests use,
// in Django's META spelling.
var forwardedProto = []string{"HTTP_X_FORWARDED_PROTO", "https"}

// plainRequest builds a request without TLS, the counterpart of HttpRequest().
// Each name and value pair of headers sets one request header.
func plainRequest(headers ...string) *http.Request {
	r := &http.Request{Method: http.MethodGet, URL: &url.URL{Path: "/"}, Header: make(http.Header)}
	for i := 0; i+1 < len(headers); i += 2 {
		r.Header.Set(headers[i], headers[i+1])
	}
	return r
}

// Ported from Django: tests/settings_tests/tests.py::SecureProxySslHeaderTest.test_none
func TestSecureProxySSLHeaderNone(t *testing.T) {
	djangotest.AssertEqual(t, middleware.IsSecure(plainRequest(), nil), false)
}

// Ported from Django: tests/settings_tests/tests.py::SecureProxySslHeaderTest.test_set_without_xheader
func TestSecureProxySSLHeaderSetWithoutXHeader(t *testing.T) {
	djangotest.AssertEqual(t, middleware.IsSecure(plainRequest(), forwardedProto), false)
}

// Ported from Django: tests/settings_tests/tests.py::SecureProxySslHeaderTest.test_set_with_xheader_wrong
func TestSecureProxySSLHeaderSetWithXHeaderWrong(t *testing.T) {
	r := plainRequest("X-Forwarded-Proto", "wrongvalue")
	djangotest.AssertEqual(t, middleware.IsSecure(r, forwardedProto), false)
}

// Ported from Django: tests/settings_tests/tests.py::SecureProxySslHeaderTest.test_set_with_xheader_right
func TestSecureProxySSLHeaderSetWithXHeaderRight(t *testing.T) {
	r := plainRequest("X-Forwarded-Proto", "https")
	djangotest.AssertEqual(t, middleware.IsSecure(r, forwardedProto), true)
}

// Ported from Django: tests/settings_tests/tests.py::SecureProxySslHeaderTest.test_set_with_xheader_leftmost_right
func TestSecureProxySSLHeaderSetWithXHeaderLeftmostRight(t *testing.T) {
	r := plainRequest("X-Forwarded-Proto", "https, http")
	djangotest.AssertEqual(t, middleware.IsSecure(r, forwardedProto), true)
	r = plainRequest("X-Forwarded-Proto", "https  , http")
	djangotest.AssertEqual(t, middleware.IsSecure(r, forwardedProto), true)
}

// Ported from Django: tests/settings_tests/tests.py::SecureProxySslHeaderTest.test_set_with_xheader_leftmost_not_secure
func TestSecureProxySSLHeaderSetWithXHeaderLeftmostNotSecure(t *testing.T) {
	r := plainRequest("X-Forwarded-Proto", "http, https")
	djangotest.AssertEqual(t, middleware.IsSecure(r, forwardedProto), false)
}

// Ported from Django: tests/settings_tests/tests.py::SecureProxySslHeaderTest.test_set_with_xheader_multiple_not_secure
func TestSecureProxySSLHeaderSetWithXHeaderMultipleNotSecure(t *testing.T) {
	r := plainRequest("X-Forwarded-Proto", "http ,wrongvalue,http,http")
	djangotest.AssertEqual(t, middleware.IsSecure(r, forwardedProto), false)
}

// TestSecureProxySSLHeaderXHeaderPreferredToUnderlyingRequest ports the
// ProxyRequest subclass, whose _get_scheme always answers "https", as a request
// that arrived over TLS: the proxy connects via HTTPS, the client via HTTP.
//
// Ported from Django: tests/settings_tests/tests.py::SecureProxySslHeaderTest.test_xheader_preferred_to_underlying_request
func TestSecureProxySSLHeaderXHeaderPreferredToUnderlyingRequest(t *testing.T) {
	r := plainRequest("X-Forwarded-Proto", "http")
	r.TLS = &tls.ConnectionState{}
	djangotest.AssertEqual(t, middleware.IsSecure(r, forwardedProto), false)
}
