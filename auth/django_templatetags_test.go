package auth_test

import (
	"testing"

	"github.com/oliverhaas/djangogo/auth"
	"github.com/oliverhaas/djangogo/djangotest"
)

// Ported from Django: tests/auth_tests/test_templatetags.py::RenderPasswordAsHashTests.test_valid_password
func TestRenderPasswordAsHashValidPassword(t *testing.T) {
	useHashers(t, auth.PBKDF2Hasher{})
	value := "pbkdf2_sha256$100000$a6Pucb1qSFcD$WmCkn9Hqidj48NVe5x0FEM6A9YiOqQcl/83m2Z5u" +
		"dm0="
	hashedHTML := "<p><strong>algorithm</strong>: <bdi>pbkdf2_sha256</bdi> " +
		"<strong>iterations</strong>: <bdi>100000</bdi> " +
		"<strong>salt</strong>: <bdi>a6Pucb******</bdi> " +
		"<strong>hash</strong>: <bdi>WmCkn9**************************************" +
		"</bdi></p>"
	djangotest.AssertEqual(t, auth.RenderPasswordAsHash(value), hashedHTML)
}

// Ported from Django: tests/auth_tests/test_templatetags.py::RenderPasswordAsHashTests.test_invalid_password
//
// Django's test settings use the MD5 hasher.
func TestRenderPasswordAsHashInvalidPassword(t *testing.T) {
	useHashers(t, auth.MD5Hasher{})
	expected := "<p><strong>Invalid password format or unknown hashing algorithm.</strong>" +
		"</p>"
	for _, value := range []string{"pbkdf2_sh", "md5$password", "invalid", "testhash$password"} {
		t.Run(value, func(t *testing.T) {
			djangotest.AssertEqual(t, auth.RenderPasswordAsHash(value), expected)
		})
	}
}

// Ported from Django: tests/auth_tests/test_templatetags.py::RenderPasswordAsHashTests.test_no_password
//
// No Go counterpart: None, a string has no such value.
func TestRenderPasswordAsHashNoPassword(t *testing.T) {
	expected := "<p><strong>No password set.</strong></p>"
	unusable, err := auth.MakeUnusablePassword()
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"", unusable} {
		t.Run(value, func(t *testing.T) {
			djangotest.AssertEqual(t, auth.RenderPasswordAsHash(value), expected)
		})
	}
}
