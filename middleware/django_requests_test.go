package middleware_test

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/middleware"
)

// The tests in this file are ports of HostValidationTests in Django's
// tests/requests_tests/tests.py. Django sets request.META by hand; the ports
// build an *http.Request by hand for the same reason: net/http rejects some of
// the malformed Host headers before a handler runs, so they cannot be sent
// through a client.

// poisonedHosts are Host headers that try to smuggle another host past the
// validation.
var poisonedHosts = []string{
	"example.com@evil.tld",
	"example.com:dr.frankenstein@evil.tld",
	"example.com:dr.frankenstein@evil.tld:80",
	"example.com:80/badpath",
	"example.com: recovermypassword.com",
}

// hostRequest builds a GET request with the given Host header, the counterpart
// of request.META = {"HTTP_HOST": host}. Further headers are given as name and
// value pairs.
func hostRequest(host string, headers ...string) *http.Request {
	r := &http.Request{
		Method: http.MethodGet,
		URL:    &url.URL{Path: "/"},
		Host:   host,
		Header: make(http.Header),
	}
	for i := 0; i+1 < len(headers); i += 2 {
		r.Header.Set(headers[i], headers[i+1])
	}
	return r
}

// assertDisallowedHost fails the test unless err is a *DisallowedHost, the
// counterpart of assertRaises(DisallowedHost).
func assertDisallowedHost(t *testing.T, host string, err error) *middleware.DisallowedHost {
	t.Helper()
	var disallowed *middleware.DisallowedHost
	if !errors.As(err, &disallowed) {
		t.Fatalf("GetHost with host %q: got error %v, want a *DisallowedHost", host, err)
	}
	return disallowed
}

// assertNoHostHeaderRejected checks what GetHost does for a request without any
// host. Django rebuilds the host from SERVER_NAME and SERVER_PORT in that case.
// A Go request has no server name, so the request is rejected.
func assertNoHostHeaderRejected(t *testing.T, opts middleware.AllowedHostsOptions) {
	t.Helper()
	_, err := middleware.GetHost(hostRequest(""), opts)
	disallowed := assertDisallowedHost(t, "", err)
	djangotest.AssertEqual(t, disallowed.Error(),
		"Invalid HTTP_HOST header: ''. The domain name provided is not valid according to RFC 1034/1035.")
}

// Ported from Django: tests/requests_tests/tests.py::HostValidationTests.test_http_get_host
func TestHTTPGetHost(t *testing.T) {
	opts := middleware.AllowedHostsOptions{
		UseXForwardedHost: false,
		AllowedHosts: []string{
			"forward.com",
			"example.com",
			"internal.com",
			"12.34.56.78",
			"[2001:19f0:feee::dead:beef:cafe]",
			"xn--4ca9at.com",
			".multitenant.com",
			"INSENSITIVE.com",
			"[::ffff:169.254.169.254]",
		},
	}

	// Check if X_FORWARDED_HOST is provided: it is ignored.
	host, err := middleware.GetHost(hostRequest("example.com", "X-Forwarded-Host", "forward.com"), opts)
	djangotest.AssertEqual(t, err, nil)
	djangotest.AssertEqual(t, host, "example.com")

	// Check if X_FORWARDED_HOST isn't provided.
	host, err = middleware.GetHost(hostRequest("example.com"), opts)
	djangotest.AssertEqual(t, err, nil)
	djangotest.AssertEqual(t, host, "example.com")

	// Check if HTTP_HOST isn't provided. Go behavior: there is no SERVER_NAME
	// to fall back to (see assertNoHostHeaderRejected).
	assertNoHostHeaderRejected(t, opts)

	legitHosts := []string{
		"example.com",
		"example.com:80",
		"12.34.56.78",
		"12.34.56.78:443",
		"[2001:19f0:feee::dead:beef:cafe]",
		"[2001:19f0:feee::dead:beef:cafe]:8080",
		"xn--4ca9at.com", // Punycode for öäü.com
		"anything.multitenant.com",
		"multitenant.com",
		"insensitive.com",
		"example.com.",
		"example.com.:80",
		"[::ffff:169.254.169.254]",
	}
	for _, legit := range legitHosts {
		host, err := middleware.GetHost(hostRequest(legit), opts)
		if err != nil {
			t.Errorf("GetHost with host %q: unexpected error %v", legit, err)
		}
		djangotest.AssertEqual(t, host, legit)
	}

	// Poisoned host headers are rejected as suspicious.
	for _, poisoned := range append(append([]string{}, poisonedHosts...), "other.com", "example.com..") {
		_, err := middleware.GetHost(hostRequest(poisoned), opts)
		assertDisallowedHost(t, poisoned, err)
	}
}

// Ported from Django: tests/requests_tests/tests.py::HostValidationTests.test_http_get_host_with_x_forwarded_host
func TestHTTPGetHostWithXForwardedHost(t *testing.T) {
	opts := middleware.AllowedHostsOptions{UseXForwardedHost: true, AllowedHosts: []string{"*"}}

	// Check if X_FORWARDED_HOST is provided: it is obeyed.
	host, err := middleware.GetHost(hostRequest("example.com", "X-Forwarded-Host", "forward.com"), opts)
	djangotest.AssertEqual(t, err, nil)
	djangotest.AssertEqual(t, host, "forward.com")

	// Check if X_FORWARDED_HOST isn't provided.
	host, err = middleware.GetHost(hostRequest("example.com"), opts)
	djangotest.AssertEqual(t, err, nil)
	djangotest.AssertEqual(t, host, "example.com")

	// Check if HTTP_HOST isn't provided. Go behavior: there is no SERVER_NAME
	// to fall back to (see assertNoHostHeaderRejected).
	assertNoHostHeaderRejected(t, opts)

	legitHosts := []string{
		"example.com",
		"example.com:80",
		"12.34.56.78",
		"12.34.56.78:443",
		"[2001:19f0:feee::dead:beef:cafe]",
		"[2001:19f0:feee::dead:beef:cafe]:8080",
		"xn--4ca9at.com", // Punycode for öäü.com
	}
	for _, legit := range legitHosts {
		host, err := middleware.GetHost(hostRequest(legit), opts)
		if err != nil {
			t.Errorf("GetHost with host %q: unexpected error %v", legit, err)
		}
		djangotest.AssertEqual(t, host, legit)
	}

	// Poisoned host headers are rejected as suspicious.
	for _, poisoned := range poisonedHosts {
		_, err := middleware.GetHost(hostRequest(poisoned), opts)
		assertDisallowedHost(t, poisoned, err)
	}
}

// TestGetPort ports the test with the port of the Host header in the place of
// SERVER_PORT: a Go request carries no server port. Without a port in the Host
// header the default port of the scheme is returned, which the last two
// assertions add.
//
// Ported from Django: tests/requests_tests/tests.py::HostValidationTests.test_get_port
func TestGetPort(t *testing.T) {
	opts := middleware.AllowedHostsOptions{UseXForwardedPort: false}

	// Shouldn't use the X-Forwarded-Port header.
	r := hostRequest("testserver:8080", "X-Forwarded-Port", "80")
	djangotest.AssertEqual(t, middleware.GetPort(r, opts), "8080")

	r = hostRequest("testserver:8080")
	djangotest.AssertEqual(t, middleware.GetPort(r, opts), "8080")

	r = hostRequest("testserver")
	djangotest.AssertEqual(t, middleware.GetPort(r, opts), "80")

	r = hostRequest("testserver")
	r.TLS = &tls.ConnectionState{}
	djangotest.AssertEqual(t, middleware.GetPort(r, opts), "443")
}

// Ported from Django: tests/requests_tests/tests.py::HostValidationTests.test_get_port_with_x_forwarded_port
func TestGetPortWithXForwardedPort(t *testing.T) {
	opts := middleware.AllowedHostsOptions{UseXForwardedPort: true}

	// Should use the X-Forwarded-Port header.
	r := hostRequest("testserver:8080", "X-Forwarded-Port", "80")
	djangotest.AssertEqual(t, middleware.GetPort(r, opts), "80")

	r = hostRequest("testserver:8080")
	djangotest.AssertEqual(t, middleware.GetPort(r, opts), "8080")
}

// TestHostValidationInDebugMode checks that variants of localhost are allowed
// when the allowed hosts are empty and Debug is on.
//
// Ported from Django: tests/requests_tests/tests.py::HostValidationTests.test_host_validation_in_debug_mode
func TestHostValidationInDebugMode(t *testing.T) {
	opts := middleware.AllowedHostsOptions{Debug: true, AllowedHosts: nil}

	for _, valid := range []string{"localhost", "subdomain.localhost", "127.0.0.1", "[::1]"} {
		host, err := middleware.GetHost(hostRequest(valid), opts)
		if err != nil {
			t.Errorf("GetHost with host %q: unexpected error %v", valid, err)
		}
		djangotest.AssertEqual(t, host, valid)
	}

	// Other hostnames raise a DisallowedHost.
	_, err := middleware.GetHost(hostRequest("example.com"), opts)
	assertDisallowedHost(t, "example.com", err)
}

// TestGetHostSuggestionOfAllowedHost checks that GetHost makes helpful
// suggestions if a valid-looking host is not in the allowed hosts.
//
// Ported from Django: tests/requests_tests/tests.py::HostValidationTests.test_get_host_suggestion_of_allowed_host
func TestGetHostSuggestionOfAllowedHost(t *testing.T) {
	opts := middleware.AllowedHostsOptions{AllowedHosts: []string{}}
	const (
		msgInvalidHost = "Invalid HTTP_HOST header: '%s'."
		msgSuggestion  = msgInvalidHost + " You may need to add '%s' to ALLOWED_HOSTS."
		msgSuggestion2 = msgInvalidHost + " The domain name provided is not valid according to RFC 1034/1035."
	)

	// Valid-looking hosts.
	for _, host := range []string{
		"example.com",
		"12.34.56.78",
		"[2001:19f0:feee::dead:beef:cafe]",
		"xn--4ca9at.com", // Punycode for öäü.com
	} {
		_, err := middleware.GetHost(hostRequest(host), opts)
		disallowed := assertDisallowedHost(t, host, err)
		djangotest.AssertEqual(t, disallowed.Error(), fmt.Sprintf(msgSuggestion, host, host))
	}

	// Valid-looking hosts with a port number.
	for _, tc := range []struct {
		domain string
		port   int
	}{
		{"example.com", 80},
		{"12.34.56.78", 443},
		{"[2001:19f0:feee::dead:beef:cafe]", 8080},
	} {
		host := fmt.Sprintf("%s:%d", tc.domain, tc.port)
		_, err := middleware.GetHost(hostRequest(host), opts)
		disallowed := assertDisallowedHost(t, host, err)
		djangotest.AssertEqual(t, disallowed.Error(), fmt.Sprintf(msgSuggestion, host, tc.domain))
	}

	// Django matches these with assertRaisesMessage, which looks for the text
	// inside the message; the full message is compared here.
	for _, host := range poisonedHosts {
		_, err := middleware.GetHost(hostRequest(host), opts)
		disallowed := assertDisallowedHost(t, host, err)
		djangotest.AssertEqual(t, disallowed.Error(), fmt.Sprintf(msgSuggestion2, host))
	}

	_, err := middleware.GetHost(hostRequest("invalid_hostname.com"), opts)
	disallowed := assertDisallowedHost(t, "invalid_hostname.com", err)
	djangotest.AssertEqual(t, disallowed.Error(), fmt.Sprintf(msgSuggestion2, "invalid_hostname.com"))
}

// Ported from Django: tests/requests_tests/tests.py::HostValidationTests.test_split_domain_port
func TestSplitDomainPort(t *testing.T) {
	for _, tc := range []struct {
		host, domain, port string
	}{
		{"<invalid>", "", ""},
		{"<invalid>:8080", "", ""},
		{"example.com 8080", "", ""},
		{"example.com:invalid", "", ""},
		{"[::1]", "[::1]", ""},
		{"[::1]:8080", "[::1]", "8080"},
		{"[::ffff:127.0.0.1]", "[::ffff:127.0.0.1]", ""},
		{"[::ffff:127.0.0.1]:8080", "[::ffff:127.0.0.1]", "8080"},
		{"[1851:0000:3238:DEF1:0177:0000:0000:0125]", "[1851:0000:3238:def1:0177:0000:0000:0125]", ""},
		{"[1851:0000:3238:DEF1:0177:0000:0000:0125]:8080", "[1851:0000:3238:def1:0177:0000:0000:0125]", "8080"},
		{"127.0.0.1", "127.0.0.1", ""},
		{"127.0.0.1:8080", "127.0.0.1", "8080"},
		{"example.com", "example.com", ""},
		{"example.com:8080", "example.com", "8080"},
		{"example.com.", "example.com", ""},
		{"example.com.:8080", "example.com", "8080"},
		{"xn--n28h.test", "xn--n28h.test", ""},
		{"xn--n28h.test:8080", "xn--n28h.test", "8080"},
		{"subdomain.example.com", "subdomain.example.com", ""},
		{"subdomain.example.com:8080", "subdomain.example.com", "8080"},
	} {
		t.Run(tc.host, func(t *testing.T) {
			domain, port := middleware.SplitDomainPort(tc.host)
			djangotest.AssertEqual(t, domain, tc.domain)
			djangotest.AssertEqual(t, port, tc.port)
		})
	}
}
