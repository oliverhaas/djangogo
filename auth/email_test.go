package auth_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/auth"
)

// Ported from Django 6.0.3 tests/validators/tests.py::TestValidators::test_validators
// (the validate_email rows of TEST_DATA; the None row and the custom allowlist
// row are left out because ValidateEmail takes a string and has a fixed
// allowlist).
func TestValidateEmailDjangoData(t *testing.T) {
	valid := []string{
		"email@here.com",
		"weirder-email@here.and.there.com",
		"email@[127.0.0.1]",
		"email@[2001:dB8::1]",
		"email@[2001:dB8:0:0:0:0:0:1]",
		"email@[::fffF:127.0.0.1]",
		"example@valid-----hyphens.com",
		"example@valid-with-hyphens.com",
		"test@domain.with.idn.tld.उदाहरण.परीक्षा",
		"email@localhost",
		`"test@test"@example.com`,
		"example@atm." + strings.Repeat("a", 63),
		"example@" + strings.Repeat("a", 63) + ".atm",
		"example@" + strings.Repeat("a", 63) + "." + strings.Repeat("b", 10) + ".atm",
		"email@xn--4ca9at.com",
		"email@öäü.com",
		"email@עִתוֹן.example.il",
		"email@މިހާރު.example.mv",
		"email@漢字.example.com",
		"editor@މިހާރު.example.mv",
		"\"\\\t\"@here.com",
		"a@" + strings.Repeat("a", 63) + ".us",
	}
	invalid := []string{
		"example@atm." + strings.Repeat("a", 64),
		"example@" + strings.Repeat("b", 64) + ".atm." + strings.Repeat("a", 63),
		"example@" + strings.Repeat(strings.Repeat("a", 63)+".", 100) + "com",
		"",
		"abc",
		"abc@",
		"abc@bar",
		"a @x.cz",
		"abc@.com",
		"something@@somewhere.com",
		"email@127.0.0.1",
		"email@[127.0.0.256]",
		"email@[2001:db8::12345]",
		"email@[2001:db8:0:0:0:0:1]",
		"email@[::ffff:127.0.0.256]",
		"email@[2001:dg8::1]",
		"email@[2001:dG8:0:0:0:0:0:1]",
		"email@[::fTzF:127.0.0.1]",
		"example@invalid-.com",
		"example@-invalid.com",
		"example@invalid.com-",
		"example@inv-.alid-.com",
		"example@inv-.-alid.com",
		"test@example.com\n\n<script src=\"x.js\">",
		"@domain.com",
		"email.domain.com",
		"email@domain@domain.com",
		"email@domain..com",
		"email@.domain.com",
		"email@-domain.com",
		"email@domain-.com",
		"email@domain.com-",
		"\"\\\n\"@here.com",
		"trailingdot@shouldfail.com.",
		"a@" + strings.Repeat("a", 64) + ".us",
		"a@b.com\n",
		"a\n@b.com",
		"\"test@test\"\n@example.com",
		"a@[127.0.0.1]\n",
	}
	for _, email := range valid {
		if err := auth.ValidateEmail(email); err != nil {
			t.Errorf("ValidateEmail(%q) = %v, want nil", email, err)
		}
	}
	for _, email := range invalid {
		assertInvalidEmail(t, email)
	}
}

// TestValidateEmailOracle checks addresses whose verdict was taken from
// Django 6.0.3's validate_email: the characters Python's case-insensitive
// patterns fold to ASCII letters, the IPv6 length limit, IPv4 leading zeros,
// and the top-level domain rules.
func TestValidateEmailOracle(t *testing.T) {
	valid := []string{
		"ſ@example.com",
		"K@example.com",
		"ı@example.com",
		"İ@example.com",
		"\"ſ\"@example.com",
		"\"K\"@example.com",
		"a@[::ffff:1.2.3.4]",
		"a@[1.2.3.4]",
		"a@ſſ.com",
		"a@b.KK",
		"a@b.xn--ab",
		"a@b.XN--ab1",
		"a@b.a-b",
		"a@1.com",
		"a@[1:2:3:4:5:6:7::]",
		"a@[fe80::1]",
		`""@example.com`,
		`"a\"b"@example.com`,
	}
	invalid := []string{
		"a@[0000:0000:0000:0000:0000:ffff:192.168.100.200]",
		"a@[01.1.1.1]",
		"a@b.xn--",
		"a@b.ab-",
		"a@b.c1",
		"a@LOCALHOST",
		"a@b",
		"a@b.com.",
		"a@-b.com",
		"a..b@example.com",
		".a@example.com",
		"a@",
		"a@b.c\U0001F600",
		"a@[fe80::1%eth0]",
		"\xff@example.com",
	}
	for _, email := range valid {
		if err := auth.ValidateEmail(email); err != nil {
			t.Errorf("ValidateEmail(%q) = %v, want nil", email, err)
		}
	}
	for _, email := range invalid {
		assertInvalidEmail(t, email)
	}
}

// assertInvalidEmail fails the test unless ValidateEmail rejects email with
// Django's message.
func assertInvalidEmail(t *testing.T, email string) {
	t.Helper()
	err := auth.ValidateEmail(email)
	var emailErr *auth.EmailError
	if !errors.As(err, &emailErr) {
		t.Errorf("ValidateEmail(%q) = %v, want an *EmailError", email, err)
		return
	}
	if emailErr.Message != "Enter a valid email address." {
		t.Errorf("ValidateEmail(%q) message = %q", email, emailErr.Message)
	}
}
