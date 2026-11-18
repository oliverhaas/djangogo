package conf_test

import (
	"testing"

	"github.com/oliverhaas/djangogo/conf"
	"github.com/oliverhaas/djangogo/djangotest"
)

// The tests in this file are ports of the Django tests of the rules that
// Settings.Validate applies. Django spreads these rules over system checks
// and over the code that reads a setting; here Validate returns them as
// problems, and the checks package gives each one its message id. MEDIA_URL has
// no field, so CheckURLSettingsTests.test_media_url_no_slash is not ported.

// staticURLProblems returns the problems of settings with the given STATIC_URL.
func staticURLProblems(staticURL string) []conf.Problem {
	s := conf.Defaults()
	s.StaticURL = staticURL
	return s.Validate()
}

// Ported from Django: tests/check_framework/test_urls.py::CheckURLSettingsTests.test_slash_no_errors
func TestSlashNoErrors(t *testing.T) {
	djangotest.AssertEqual(t, staticURLProblems("a/"), nil)
}

// Ported from Django: tests/check_framework/test_urls.py::CheckURLSettingsTests.test_empty_string_no_errors
func TestEmptyStringNoErrors(t *testing.T) {
	djangotest.AssertEqual(t, staticURLProblems(""), nil)
}

// Ported from Django: tests/check_framework/test_urls.py::CheckURLSettingsTests.test_static_url_no_slash
func TestStaticURLNoSlash(t *testing.T) {
	djangotest.AssertEqual(t, staticURLProblems("noslash"), []conf.Problem{
		{Setting: "STATIC_URL", Msg: "The STATIC_URL setting must end with a slash."},
	})
}

// TestStaticURLDoubleSlashAllowed allows a double slash, presuming the user
// knows what they are doing.
//
// Ported from Django: tests/check_framework/test_urls.py::CheckURLSettingsTests.test_static_url_double_slash_allowed
func TestStaticURLDoubleSlashAllowed(t *testing.T) {
	djangotest.AssertEqual(t, staticURLProblems("slashes//"), nil)
}

// TestInvalidURL reports a trusted origin without a scheme. The message is
// Django's without its reference to Django 4.0 and its release notes.
//
// Ported from Django: tests/check_framework/test_4_0_compatibility.py::CheckCSRFTrustedOrigins.test_invalid_url
func TestInvalidURL(t *testing.T) {
	s := conf.Defaults()
	s.CSRFTrustedOrigins = []string{"example.com"}

	djangotest.AssertEqual(t, s.Validate(), []conf.Problem{{
		Setting: "CSRF_TRUSTED_ORIGINS",
		Msg: "The values in the CSRF_TRUSTED_ORIGINS setting must start with a scheme " +
			"(usually http:// or https://) but found example.com.",
	}})
}

// Ported from Django: tests/check_framework/test_4_0_compatibility.py::CheckCSRFTrustedOrigins.test_valid_urls
func TestValidURLs(t *testing.T) {
	s := conf.Defaults()
	s.CSRFTrustedOrigins = []string{"http://example.com", "https://example.com"}

	djangotest.AssertEqual(t, s.Validate(), nil)
}

// TestSamesite accepts the SameSite values in any letter case. Django checks
// the value when a cookie is set; here Validate checks the two settings that
// feed the session cookie and the CSRF cookie.
//
// Ported from Django: tests/responses/test_cookie.py::SetCookieTests.test_samesite
func TestSamesite(t *testing.T) {
	for _, value := range []string{"None", "Lax", "strict"} {
		s := conf.Defaults()
		s.SessionCookieSameSite = value
		s.CSRFCookieSameSite = value
		djangotest.AssertEqual(t, s.Validate(), nil)
	}
}

// TestInvalidSamesite reports another SameSite value. Django raises
// ValueError('samesite must be "lax", "none", or "strict".') from set_cookie,
// which does not name a setting; here the problem names it.
//
// Ported from Django: tests/responses/test_cookie.py::SetCookieTests.test_invalid_samesite
func TestInvalidSamesite(t *testing.T) {
	s := conf.Defaults()
	s.SessionCookieSameSite = "invalid"
	s.CSRFCookieSameSite = "invalid"

	djangotest.AssertEqual(t, s.Validate(), []conf.Problem{
		{
			Setting: "SESSION_COOKIE_SAMESITE",
			Msg:     `The SESSION_COOKIE_SAMESITE setting must be "Lax", "Strict" or "None", but is "invalid".`,
		},
		{
			Setting: "CSRF_COOKIE_SAMESITE",
			Msg:     `The CSRF_COOKIE_SAMESITE setting must be "Lax", "Strict" or "None", but is "invalid".`,
		},
	})
}
