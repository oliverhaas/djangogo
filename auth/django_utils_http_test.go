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

// Ported from Django: tests/utils_tests/test_http.py::Base36IntTests.test_roundtrip
func TestDjangoBase36IntRoundtrip(t *testing.T) {
	for _, n := range []int64{0, 1, 1000, 1000000} {
		got, err := auth.Base36ToInt(auth.IntToBase36(uint64(n)))
		if err != nil || got != n {
			t.Errorf("Base36ToInt(IntToBase36(%d)) = %d, %v; want %d", n, got, err, n)
		}
	}
}

// No Go counterpart: tests/utils_tests/test_http.py::Base36IntTests.test_negative_input
// IntToBase36 takes a uint64, so a negative input does not compile.

// No Go counterpart: tests/utils_tests/test_http.py::Base36IntTests.test_to_base36_errors
// The argument types are checked at compile time.

// Ported from Django: tests/utils_tests/test_http.py::Base36IntTests.test_invalid_literal
func TestDjangoBase36IntInvalidLiteral(t *testing.T) {
	for _, n := range []string{"#", " "} {
		_, err := auth.Base36ToInt(n)
		want := "invalid literal for int() with base 36: '" + n + "'"
		if err == nil || err.Error() != want {
			t.Errorf("Base36ToInt(%q) error = %v, want %q", n, err, want)
		}
	}
}

// Ported from Django: tests/utils_tests/test_http.py::Base36IntTests.test_input_too_large
func TestDjangoBase36IntInputTooLarge(t *testing.T) {
	_, err := auth.Base36ToInt(strings.Repeat("1", 14))
	if err == nil || err.Error() != "Base36 input too large" {
		t.Errorf("Base36ToInt(14 digits) error = %v, want Base36 input too large", err)
	}
}

// No Go counterpart: tests/utils_tests/test_http.py::Base36IntTests.test_to_int_errors
// The argument types are checked at compile time.

// Ported from Django: tests/utils_tests/test_http.py::Base36IntTests.test_values
func TestDjangoBase36IntValues(t *testing.T) {
	for _, tc := range []struct {
		n   int64
		b36 string
	}{{0, "0"}, {1, "1"}, {42, "16"}, {818469960, "django"}} {
		if got := auth.IntToBase36(uint64(tc.n)); got != tc.b36 {
			t.Errorf("IntToBase36(%d) = %q, want %q", tc.n, got, tc.b36)
		}
		if got, err := auth.Base36ToInt(tc.b36); err != nil || got != tc.n {
			t.Errorf("Base36ToInt(%q) = %d, %v; want %d", tc.b36, got, err, tc.n)
		}
	}
}

// No Django test exists: Base36ToInt accepts what Python's int(s, 36) accepts
// (upper case, a sign, surrounding whitespace and single underscores between
// digits) and refuses a number beyond int64 with Django's message.
func TestBase36ToIntPythonSyntax(t *testing.T) {
	for in, want := range map[string]int64{"DJANGO": 818469960, " 16\n": 42, "+16": 42, "-16": -42, "1_6": 42} {
		if got, err := auth.Base36ToInt(in); err != nil || got != want {
			t.Errorf("Base36ToInt(%q) = %d, %v; want %d", in, got, err, want)
		}
	}
	for _, in := range []string{"", "_16", "16_", "1__6", "+", "1 6"} {
		if _, err := auth.Base36ToInt(in); err == nil || !strings.HasPrefix(err.Error(), "invalid literal") {
			t.Errorf("Base36ToInt(%q) error = %v, want an invalid literal error", in, err)
		}
	}
	if _, err := auth.Base36ToInt("zzzzzzzzzzzzz"); err == nil || err.Error() != "Base36 input too large" {
		t.Errorf("Base36ToInt(13 z) error = %v, want Base36 input too large", err)
	}
}

// Ported from Django: tests/utils_tests/test_http.py::URLSafeBase64Tests.test_roundtrip
func TestDjangoURLSafeBase64Roundtrip(t *testing.T) {
	encoded := auth.URLSafeBase64Encode([]byte("foo"))
	decoded, err := auth.URLSafeBase64Decode(encoded)
	if err != nil || string(decoded) != "foo" {
		t.Errorf("URLSafeBase64Decode(%q) = %q, %v; want foo", encoded, decoded, err)
	}
}

// No Django test exists: the encoding drops the padding and uses the URL
// alphabet, as urlsafe_base64_encode does, and the decoding accepts the
// padding back.
func TestURLSafeBase64Padding(t *testing.T) {
	if got := auth.URLSafeBase64Encode([]byte{0xfb, 0xff}); got != "-_8" {
		t.Errorf("URLSafeBase64Encode = %q, want -_8", got)
	}
	if got, err := auth.URLSafeBase64Decode("MQ=="); err != nil || string(got) != "1" {
		t.Errorf("URLSafeBase64Decode(MQ==) = %q, %v; want 1", got, err)
	}
	if _, err := auth.URLSafeBase64Decode("M!"); err == nil {
		t.Error("URLSafeBase64Decode(M!) succeeds, want an error")
	}
}
