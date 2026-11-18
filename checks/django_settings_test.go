package checks_test

import (
	"testing"

	"github.com/oliverhaas/djangogo/checks"
	"github.com/oliverhaas/djangogo/conf"
)

// The tests in this file are ports of the Django tests of the checks that read
// one setting: tests/check_framework/test_urls.py (CheckURLSettingsTests),
// tests/check_framework/test_4_0_compatibility.py and
// tests/staticfiles_tests/test_checks.py (FindersCheckTests). A Django test
// overrides the setting and calls the check function; a port sets the field of
// conf.Defaults() and runs the checks of Django's tag through the Default
// registry.

// taggedMessages runs the built-in checks that carry tag on s, without the
// deployment checks.
func taggedMessages(s conf.Settings, tag string) []checks.Message {
	return checks.Default.Run(&checks.Context{Settings: &s}, checks.RunOptions{Tags: []string{tag}})
}

// TestSlashNoErrors accepts a STATIC_URL that ends with a slash. There is no
// MEDIA_URL setting, so the port sets STATIC_URL alone.
//
// Ported from Django: tests/check_framework/test_urls.py::CheckURLSettingsTests.test_slash_no_errors
func TestSlashNoErrors(t *testing.T) {
	s := conf.Defaults()
	s.StaticURL = "a/"
	assertMessages(t, taggedMessages(s, checks.TagURLs))
}

// TestEmptyStringNoErrors accepts an empty STATIC_URL.
//
// Ported from Django: tests/check_framework/test_urls.py::CheckURLSettingsTests.test_empty_string_no_errors
func TestEmptyStringNoErrors(t *testing.T) {
	s := conf.Defaults()
	s.StaticURL = ""
	assertMessages(t, taggedMessages(s, checks.TagURLs))
}

// TestStaticURLNoSlash reports a STATIC_URL that does not end with a slash.
//
// Ported from Django: tests/check_framework/test_urls.py::CheckURLSettingsTests.test_static_url_no_slash
func TestStaticURLNoSlash(t *testing.T) {
	s := conf.Defaults()
	s.StaticURL = "noslash"
	assertMessages(t, taggedMessages(s, checks.TagURLs),
		checks.Error("The STATIC_URL setting must end with a slash.", checks.ID("urls.E006")))
}

// TestStaticURLDoubleSlashAllowed accepts a STATIC_URL that ends with two
// slashes.
//
// Ported from Django: tests/check_framework/test_urls.py::CheckURLSettingsTests.test_static_url_double_slash_allowed
func TestStaticURLDoubleSlashAllowed(t *testing.T) {
	s := conf.Defaults()
	s.StaticURL = "slashes//"
	assertMessages(t, taggedMessages(s, checks.TagURLs))
}

// TestInvalidURL reports a trusted origin without a scheme. The text is the
// one conf.Settings.Validate gives: Django's without "As of Django 4.0, " in
// front and "See the release notes for details." behind, which point at
// Django's own history.
//
// Ported from Django: tests/check_framework/test_4_0_compatibility.py::CheckCSRFTrustedOrigins.test_invalid_url
func TestInvalidURL(t *testing.T) {
	s := conf.Defaults()
	s.CSRFTrustedOrigins = []string{"example.com"}
	assertMessages(t, taggedMessages(s, checks.TagCompatibility), checks.Error(
		"The values in the CSRF_TRUSTED_ORIGINS setting must start with a scheme (usually http:// or https://) but found example.com.",
		checks.ID("4_0.E001"),
	))
}

// TestValidURLs accepts trusted origins with a scheme.
//
// Ported from Django: tests/check_framework/test_4_0_compatibility.py::CheckCSRFTrustedOrigins.test_valid_urls
func TestValidURLs(t *testing.T) {
	s := conf.Defaults()
	s.CSRFTrustedOrigins = []string{"http://example.com", "https://example.com"}
	assertMessages(t, taggedMessages(s, checks.TagCompatibility))
}

// TestNoErrorsWithTestSettings accepts static directories that exist and a
// static root that is none of them, which is what Django's test settings hold.
//
// Ported from Django: tests/staticfiles_tests/test_checks.py::FindersCheckTests.test_no_errors_with_test_settings
func TestNoErrorsWithTestSettings(t *testing.T) {
	s := conf.Defaults()
	s.StaticRoot = t.TempDir()
	s.StaticFilesDirs = []string{t.TempDir(), t.TempDir()}
	assertMessages(t, taggedMessages(s, checks.TagStaticfiles))
}

// TestDirsContainsStaticRoot reports the static root among the static
// directories.
//
// Ported from Django: tests/staticfiles_tests/test_checks.py::FindersCheckTests.test_dirs_contains_static_root
func TestDirsContainsStaticRoot(t *testing.T) {
	s := conf.Defaults()
	s.StaticRoot = t.TempDir()
	s.StaticFilesDirs = []string{s.StaticRoot}
	assertMessages(t, taggedMessages(s, checks.TagStaticfiles), checks.Error(
		"The STATICFILES_DIRS setting should not contain the STATIC_ROOT setting.",
		checks.ID("staticfiles.E002"),
	))
}

// TestNonexistentDirectories warns about every static directory that does not
// exist. Django's second entry is a (prefix, path) pair, which the setting
// does not have here; the port keeps its path as a plain entry. That the
// finder skips such a directory is a test of the staticfiles package.
//
// Ported from Django: tests/staticfiles_tests/test_checks.py::FindersCheckTests.test_nonexistent_directories
func TestNonexistentDirectories(t *testing.T) {
	s := conf.Defaults()
	s.StaticFilesDirs = []string{"/fake/path", "/fake/prefixed/path"}
	assertMessages(t, taggedMessages(s, checks.TagStaticfiles),
		checks.Warning(
			"The directory '/fake/path' in the STATICFILES_DIRS setting does not exist.",
			checks.ID("staticfiles.W004"),
		),
		checks.Warning(
			"The directory '/fake/prefixed/path' in the STATICFILES_DIRS setting does not exist.",
			checks.ID("staticfiles.W004"),
		),
	)
}
