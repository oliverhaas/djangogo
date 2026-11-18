package auth_test

import (
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/auth"
)

// Ported from Django: tests/utils_tests/test_http.py::URLHasAllowedHostAndSchemeTests.test_bad_urls
func TestDjangoURLHasAllowedHostAndSchemeBadURLs(t *testing.T) {
	badURLs := []string{
		"http://example.com",
		"http:///example.com",
		"https://example.com",
		"ftp://example.com",
		`\\example.com`,
		`\\\example.com`,
		`/\\/example.com`,
		`\\\example.com`,
		`\\example.com`,
		`\\//example.com`,
		`/\/example.com`,
		`\/example.com`,
		`/\example.com`,
		"http:///example.com",
		`http:/\//example.com`,
		`http:\/example.com`,
		`http:/\example.com`,
		`javascript:alert("XSS")`,
		"\njavascript:alert(x)",
		"java\nscript:alert(x)",
		"\x08//example.com",
		`http://otherserver\@example.com`,
		`http:\\testserver\@example.com`,
		`http://testserver\me:pass@example.com`,
		`http://testserver\@example.com`,
		`http:\\testserver\confirm\me@example.com`,
		"http:999999999",
		"ftp:9999999999",
		"\n",
		"http://[2001:cdba:0000:0000:0000:0000:3257:9652/",
		"http://2001:cdba:0000:0000:0000:0000:3257:9652]/",
	}
	for _, badURL := range badURLs {
		t.Run(badURL, func(t *testing.T) {
			if auth.URLHasAllowedHostAndScheme(badURL, []string{"testserver", "testserver2"}, false) {
				t.Errorf("URLHasAllowedHostAndScheme(%q) = true, want false", badURL)
			}
		})
	}
}

// Ported from Django: tests/utils_tests/test_http.py::URLHasAllowedHostAndSchemeTests.test_good_urls
func TestDjangoURLHasAllowedHostAndSchemeGoodURLs(t *testing.T) {
	goodURLs := []string{
		"/view/?param=http://example.com",
		"/view/?param=https://example.com",
		"/view?param=ftp://example.com",
		"view/?param=//example.com",
		"https://testserver/",
		"HTTPS://testserver/",
		"//testserver/",
		"http://testserver/confirm?email=me@example.com",
		"/url%20with%20spaces/",
		"path/http:2222222222",
	}
	for _, goodURL := range goodURLs {
		t.Run(goodURL, func(t *testing.T) {
			if !auth.URLHasAllowedHostAndScheme(goodURL, []string{"otherserver", "testserver"}, false) {
				t.Errorf("URLHasAllowedHostAndScheme(%q) = false, want true", goodURL)
			}
		})
	}
}

// Ported from Django: tests/utils_tests/test_http.py::URLHasAllowedHostAndSchemeTests.test_basic_auth
func TestDjangoURLHasAllowedHostAndSchemeBasicAuth(t *testing.T) {
	if !auth.URLHasAllowedHostAndScheme("http://user:pass@testserver/", []string{"user:pass@testserver"}, false) {
		t.Error("valid basic auth credentials were refused")
	}
}

// Ported from Django: tests/utils_tests/test_http.py::URLHasAllowedHostAndSchemeTests.test_no_allowed_hosts
func TestDjangoURLHasAllowedHostAndSchemeNoAllowedHosts(t *testing.T) {
	if !auth.URLHasAllowedHostAndScheme("/confirm/me@example.com", nil, false) {
		t.Error("a path without host was refused")
	}
	if auth.URLHasAllowedHostAndScheme(`http://testserver\@example.com`, nil, false) {
		t.Error("basic auth without host was allowed")
	}
}

// Ported from Django: tests/utils_tests/test_http.py::URLHasAllowedHostAndSchemeTests.test_allowed_hosts_str
//
// Go has no string-or-set argument: one host is a slice with one element.
func TestDjangoURLHasAllowedHostAndSchemeAllowedHostsStr(t *testing.T) {
	if !auth.URLHasAllowedHostAndScheme("http://good.com/good", []string{"good.com"}, false) {
		t.Error("http://good.com/good was refused")
	}
	if auth.URLHasAllowedHostAndScheme("http://good.co/evil", []string{"good.com"}, false) {
		t.Error("http://good.co/evil was allowed")
	}
}

// Ported from Django: tests/utils_tests/test_http.py::URLHasAllowedHostAndSchemeTests.test_secure_param_https_urls
func TestDjangoURLHasAllowedHostAndSchemeSecureParamHTTPSURLs(t *testing.T) {
	for _, secureURL := range []string{
		"https://example.com/p",
		"HTTPS://example.com/p",
		"/view/?param=http://example.com",
	} {
		t.Run(secureURL, func(t *testing.T) {
			if !auth.URLHasAllowedHostAndScheme(secureURL, []string{"example.com"}, true) {
				t.Errorf("URLHasAllowedHostAndScheme(%q, requireHTTPS) = false, want true", secureURL)
			}
		})
	}
}

// Ported from Django: tests/utils_tests/test_http.py::URLHasAllowedHostAndSchemeTests.test_secure_param_non_https_urls
func TestDjangoURLHasAllowedHostAndSchemeSecureParamNonHTTPSURLs(t *testing.T) {
	for _, insecureURL := range []string{
		"http://example.com/p",
		"ftp://example.com/p",
		"//example.com/p",
	} {
		t.Run(insecureURL, func(t *testing.T) {
			if auth.URLHasAllowedHostAndScheme(insecureURL, []string{"example.com"}, true) {
				t.Errorf("URLHasAllowedHostAndScheme(%q, requireHTTPS) = true, want false", insecureURL)
			}
		})
	}
}

// Ported from Django: tests/utils_tests/test_http.py::URLHasAllowedHostAndSchemeTests.test_max_url_length
func TestDjangoURLHasAllowedHostAndSchemeMaxURLLength(t *testing.T) {
	const allowedHost, maxURLLength = "example.com", 2048
	boundary := allowedHost + "/" + strings.Repeat("é", maxURLLength-len(allowedHost)-1)
	cases := []struct {
		name string
		url  string
		want bool
	}{
		{"at the limit", boundary, true},
		{"one over", boundary + "ú", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := auth.URLHasAllowedHostAndScheme(tc.url, []string{allowedHost}, false); got != tc.want {
				t.Errorf("URLHasAllowedHostAndScheme(%d characters) = %v, want %v", len([]rune(tc.url)), got, tc.want)
			}
		})
	}
}
