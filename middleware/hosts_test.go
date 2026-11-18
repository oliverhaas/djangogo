package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/middleware"
)

// badRequestPage is Django's plain 400 page (ERROR_PAGE_TEMPLATE rendered by
// django.views.defaults.bad_request), byte for byte.
const badRequestPage = `
<!doctype html>
<html lang="en">
<head>
  <title>Bad Request (400)</title>
</head>
<body>
  <h1>Bad Request (400)</h1><p></p>
</body>
</html>
`

// serveHost sends a hand-built GET with the given Host header through handler.
// A client refuses to send most of the malformed hosts.
func serveHost(handler http.Handler, host string, headers ...string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, hostRequest(host, headers...))
	return rec
}

// TestAllowedHostsRejectsUnknownHost checks that a request for a host that is
// not allowed gets Django's plain 400 page and never reaches the handler.
func TestAllowedHostsRejectsUnknownHost(t *testing.T) {
	reached := false
	view := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { reached = true })
	handler := middleware.AllowedHosts(middleware.AllowedHostsOptions{AllowedHosts: []string{"example.com"}})(view)

	rec := serveHost(handler, "evil.com")
	djangotest.AssertEqual(t, rec.Code, http.StatusBadRequest)
	djangotest.AssertEqual(t, rec.Header().Get("Content-Type"), "text/html; charset=utf-8")
	djangotest.AssertEqual(t, rec.Body.String(), badRequestPage)
	djangotest.AssertEqual(t, reached, false)
}

// TestAllowedHostsPassesAllowedHost checks that an allowed host reaches the
// handler, with a port, in another case, and as a subdomain of a dotted entry.
func TestAllowedHostsPassesAllowedHost(t *testing.T) {
	handler := middleware.AllowedHosts(middleware.AllowedHostsOptions{
		AllowedHosts: []string{".example.com", "[2001:db8::1]"},
	})(emptyView)

	for _, host := range []string{"example.com", "WWW.Example.COM:8000", "example.com.", "[2001:db8::1]:443"} {
		t.Run(host, func(t *testing.T) {
			djangotest.AssertEqual(t, serveHost(handler, host).Code, http.StatusOK)
		})
	}
}

// TestAllowedHostsDebugMessage checks the 400 page with Debug on: it carries
// the message of the DisallowedHost error, HTML-escaped. The hosts are quoted
// with Python's repr, and the expected messages are what Django 6.0 answers
// for the same hosts.
func TestAllowedHostsDebugMessage(t *testing.T) {
	const invalid = " The domain name provided is not valid according to RFC 1034/1035."
	handler := middleware.AllowedHosts(middleware.AllowedHostsOptions{
		AllowedHosts: []string{"example.com"},
		Debug:        true,
	})(emptyView)

	for _, tc := range []struct{ host, details string }{
		{"evil.com", "Invalid HTTP_HOST header: &#x27;evil.com&#x27;. You may need to add &#x27;evil.com&#x27; to ALLOWED_HOSTS."},
		{"<b>", "Invalid HTTP_HOST header: &#x27;&lt;b&gt;&#x27;." + invalid},
		{"ex'ample.com", "Invalid HTTP_HOST header: &quot;ex&#x27;ample.com&quot;." + invalid},
		{`a'b"c`, `Invalid HTTP_HOST header: &#x27;a\&#x27;b&quot;c&#x27;.` + invalid},
		{"tab\there", `Invalid HTTP_HOST header: &#x27;tab\there&#x27;.` + invalid},
		{`back\slash`, `Invalid HTTP_HOST header: &#x27;back\\slash&#x27;.` + invalid},
		{"del\x7f", `Invalid HTTP_HOST header: &#x27;del\x7f&#x27;.` + invalid},
		{"nbsp\u00a0", `Invalid HTTP_HOST header: &#x27;nbsp\xa0&#x27;.` + invalid},
		{"zero\u200bwidth", `Invalid HTTP_HOST header: &#x27;zero\u200bwidth&#x27;.` + invalid},
		{"café", "Invalid HTTP_HOST header: &#x27;café&#x27;." + invalid},
		{"\U0001f642", "Invalid HTTP_HOST header: &#x27;\U0001f642&#x27;." + invalid},
	} {
		t.Run(tc.host, func(t *testing.T) {
			rec := serveHost(handler, tc.host)
			djangotest.AssertEqual(t, rec.Code, http.StatusBadRequest)
			djangotest.AssertEqual(t, rec.Body.String(),
				"\n<!doctype html>\n<html lang=\"en\">\n<head>\n  <title>Bad Request (400)</title>\n</head>\n<body>\n"+
					"  <h1>Bad Request (400)</h1><p>"+tc.details+"</p>\n</body>\n</html>\n")
		})
	}
}

// TestAllowedHostsDebugAllowsLocalhost checks that Debug with an empty list
// allows the variants of localhost and nothing else, and that a list switches
// that off.
func TestAllowedHostsDebugAllowsLocalhost(t *testing.T) {
	debug := middleware.AllowedHosts(middleware.AllowedHostsOptions{Debug: true})(emptyView)
	for host, want := range map[string]int{
		"localhost:8000":      http.StatusOK,
		"app.localhost":       http.StatusOK,
		"127.0.0.1:8000":      http.StatusOK,
		"[::1]:8000":          http.StatusOK,
		"example.com":         http.StatusBadRequest,
		"localhost.evil.com":  http.StatusBadRequest,
		"127.0.0.1.evil.com":  http.StatusBadRequest,
		"notlocalhost":        http.StatusBadRequest,
		"evil.com@localhost":  http.StatusBadRequest,
		"localhost:8000/path": http.StatusBadRequest,
	} {
		t.Run(host, func(t *testing.T) {
			djangotest.AssertEqual(t, serveHost(debug, host).Code, want)
		})
	}

	listed := middleware.AllowedHosts(middleware.AllowedHostsOptions{
		Debug:        true,
		AllowedHosts: []string{"example.com"},
	})(emptyView)
	djangotest.AssertEqual(t, serveHost(listed, "localhost").Code, http.StatusBadRequest)
}

// TestAllowedHostsForwardedHost checks that X-Forwarded-Host decides only when
// UseXForwardedHost is set: without the option a client could pass any host
// through the header.
func TestAllowedHostsForwardedHost(t *testing.T) {
	for _, tc := range []struct {
		name      string
		forwarded bool
		host      string
		header    string
		want      int
	}{
		{"header ignored", false, "example.com", "evil.com", http.StatusOK},
		{"header ignored for a bad host", false, "evil.com", "example.com", http.StatusBadRequest},
		{"header decides", true, "internal:8000", "example.com", http.StatusOK},
		{"header rejected", true, "example.com", "evil.com", http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			handler := middleware.AllowedHosts(middleware.AllowedHostsOptions{
				AllowedHosts:      []string{"example.com"},
				UseXForwardedHost: tc.forwarded,
			})(emptyView)
			rec := serveHost(handler, tc.host, "X-Forwarded-Host", tc.header)
			djangotest.AssertEqual(t, rec.Code, tc.want)
		})
	}
}

// TestValidateHost checks the three kinds of patterns of an allowed hosts
// list. The host is the lower-cased domain SplitDomainPort returns.
func TestValidateHost(t *testing.T) {
	for _, tc := range []struct {
		host    string
		allowed []string
		want    bool
	}{
		{"example.com", []string{"example.com"}, true},
		{"example.com", []string{"EXAMPLE.com"}, true},
		{"www.example.com", []string{"example.com"}, false},
		{"www.example.com", []string{".example.com"}, true},
		{"example.com", []string{".example.com"}, true},
		{"badexample.com", []string{".example.com"}, false},
		{"anything.at.all", []string{"*"}, true},
		{"example.com", []string{"other.com", "example.com"}, true},
		{"example.com", nil, false},
		{"example.com", []string{""}, false},
	} {
		got := middleware.ValidateHost(tc.host, tc.allowed)
		if got != tc.want {
			t.Errorf("ValidateHost(%q, %q) = %v, want %v", tc.host, tc.allowed, got, tc.want)
		}
	}
}
