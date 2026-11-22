package validators_test

import (
	"errors"
	"net/netip"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/forms"
	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/validators"
)

// assertInvalid runs v on val and verifies that it returns a *ValidationError
// with the expected code and formatted message (when non-empty).
func assertInvalid(t *testing.T, v validators.Validator, val any, wantCode, wantMsg string) {
	t.Helper()
	err := v(val)
	var ve *validators.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("validator(%#v) = %v, want *ValidationError", val, err)
	}
	if wantCode != "" {
		djangotest.AssertEqual(t, ve.Code, wantCode)
	}
	if wantMsg != "" {
		djangotest.AssertEqual(t, ve.Messages(), []string{wantMsg})
	}
}

// Ported from Django: tests/validators/tests.py::TestValidators.test_single_message
func TestValidationErrorSingleMessage(t *testing.T) {
	v := validators.NewValidationError("Not Valid", "")
	djangotest.AssertEqual(t, v.Messages(), []string{"Not Valid"})
	djangotest.AssertEqual(t, v.Error(), "Not Valid")
}

// Ported from Django: tests/validators/tests.py::TestValidators.test_message_list
func TestValidationErrorMessageList(t *testing.T) {
	v := validators.ValidationErrors("First Problem", "Second Problem")
	djangotest.AssertEqual(t, v.Messages(), []string{"First Problem", "Second Problem"})
}

// Ported from Django: tests/validators/tests.py::TestValidators.test_message_dict
func TestValidationErrorMessageDict(t *testing.T) {
	v := validators.ValidationErrorDict(map[string]any{"first": []string{"First Problem"}})
	djangotest.AssertEqual(t, v.MessageDict(), map[string][]string{"first": {"First Problem"}})
}

// Ported from Django: tests/validators/tests.py::TestValidators.test_max_length_validator_message
func TestMaxLengthValidatorMessage(t *testing.T) {
	v := validators.MaxLengthValidator(16, `"%(value)s" has more than %(limit_value)d characters.`)
	assertInvalid(t, v, "djangoproject.com", "max_length", `"djangoproject.com" has more than 16 characters.`)
}

// Ported from Django: tests/validators/tests.py::EmailValidatorTests.test_invalid_domain_error
func TestEmailValidatorInvalidDomainError(t *testing.T) {
	err := validators.EmailValidator()("local@invalid_domain")
	var ve *validators.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("got %v, want *ValidationError", err)
	}
	djangotest.AssertEqual(t, ve.Code, "invalid")
	djangotest.AssertEqual(t, ve.Params, map[string]any{"value": "local@invalid_domain", "domain": "invalid_domain"})
	djangotest.AssertEqual(t, ve.Messages(), []string{"Enter a valid email address."})
}

// Ported from Django: tests/validators/tests.py::EmailValidatorTests.test_invalid_username_error
func TestEmailValidatorInvalidUsernameError(t *testing.T) {
	err := validators.EmailValidator()("not valid@example.com")
	var ve *validators.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("got %v, want *ValidationError", err)
	}
	djangotest.AssertEqual(t, ve.Code, "invalid")
	djangotest.AssertEqual(t, ve.Params, map[string]any{"value": "not valid@example.com", "username": "not valid"})
}

// No Django test exists: the local part of an address takes the letters that
// Python's re.IGNORECASE matches with [A-Z] beyond ASCII, "İ", "ı", "ſ" and
// the Kelvin sign, and no other letter, as Django's EmailValidator does.
// Django's PasswordResetFormTest sends to "mıke@example.org".
func TestEmailValidatorIgnoreCaseLetters(t *testing.T) {
	for _, email := range []string{"mıke@example.org", "İa@x.org", "ſam@x.org", "\u212ax@x.org", `"ı"@x.org`, `"\ı"@x.org`} {
		djangotest.AssertEqual(t, validators.ValidateEmail(email), nil)
	}
	assertInvalid(t, validators.ValidateEmail, "é@x.org", "invalid", "Enter a valid email address.")
}

// Ported from Django: tests/validators/tests.py::TestValidators.test_validators
func TestSharedValidatorsCore(t *testing.T) {
	// EmailValidator with custom allowlist.
	djangotest.AssertEqual(t, validators.EmailValidator("example")("user@example"), nil)
	assertInvalid(t, validators.ValidateEmail, "user@example", "invalid", "Enter a valid email address.")
	djangotest.AssertEqual(t, validators.ValidateEmail("user@localhost"), nil)
	djangotest.AssertEqual(t, validators.ValidateEmail("user@example.com"), nil)

	// URLValidator with default and extended schemes.
	djangotest.AssertEqual(t, validators.ValidateURL("https://www.djangoproject.com/"), nil)
	assertInvalid(t, validators.ValidateURL, "not-a-url", "invalid", "Enter a valid URL.")
	djangotest.AssertEqual(t, validators.URLValidator("git", "git+ssh")("git+ssh://git@github.com/django/django.git"), nil)

	// Slug and Unicode slug validators.
	djangotest.AssertEqual(t, validators.ValidateSlug("valid-slug_123"), nil)
	assertInvalid(t, validators.ValidateSlug, "invalid slug", "invalid",
		"Enter a valid “slug” consisting of letters, numbers, underscores or hyphens.")
	djangotest.AssertEqual(t, validators.ValidateUnicodeSlug("объект-1"), nil)
	assertInvalid(t, validators.ValidateUnicodeSlug, "invalid slug!", "invalid",
		"Enter a valid “slug” consisting of Unicode letters, numbers, underscores, or hyphens.")

	// Integer and IntListValidator.
	djangotest.AssertEqual(t, validators.ValidateInteger("-42"), nil)
	assertInvalid(t, validators.ValidateInteger, "3.14", "invalid", "Enter a valid integer.")
	djangotest.AssertEqual(t, validators.ValidateCommaSeparatedIntegerList("1,2,3"), nil)
	assertInvalid(t, validators.ValidateCommaSeparatedIntegerList, "1,-2,3", "invalid",
		"Enter only digits separated by commas.")
	negList := validators.IntListValidator(":", "", "", true)
	djangotest.AssertEqual(t, negList("-1:2:-3"), nil)
	assertInvalid(t, negList, "-1,2", "invalid", "Enter a valid value.")

	// MinLength / MaxLength singular and plural messages.
	assertInvalid(t, validators.MaxLengthValidator(1), "ab", "max_length",
		"Ensure this value has at most 1 character (it has 2).")
	assertInvalid(t, validators.MaxLengthValidator(3), "abcd", "max_length",
		"Ensure this value has at most 3 characters (it has 4).")
	assertInvalid(t, validators.MinLengthValidator(1), "", "min_length",
		"Ensure this value has at least 1 character (it has 0).")
	assertInvalid(t, validators.MinLengthValidator(5), "abc", "min_length",
		"Ensure this value has at least 5 characters (it has 3).")
	// Binary []byte length.
	djangotest.AssertEqual(t, validators.MaxLengthValidator(3)([]byte{1, 2, 3}), nil)
	assertInvalid(t, validators.MaxLengthValidator(2), []byte{1, 2, 3}, "max_length",
		"Ensure this value has at most 2 characters (it has 3).")

	// MinValue / MaxValue on ints, floats, time.Time, time.Duration.
	assertInvalid(t, validators.MinValueValidator(10), 5, "min_value",
		"Ensure this value is greater than or equal to 10.")
	assertInvalid(t, validators.MaxValueValidator(10), 15, "max_value",
		"Ensure this value is less than or equal to 10.")
	djangotest.AssertEqual(t, validators.MinValueValidator(time.Second)(2*time.Second), nil)
	assertInvalid(t, validators.MaxValueValidator(time.Second), 2*time.Second, "max_value",
		"Ensure this value is less than or equal to 00:00:01.")

	// RegexValidator and InverseRegexValidator.
	re := regexp.MustCompile(`^[a-z]+$`)
	djangotest.AssertEqual(t, validators.RegexValidator(re, "", "")("abc"), nil)
	assertInvalid(t, validators.RegexValidator(re, "", ""), "123", "invalid", "Enter a valid value.")
	djangotest.AssertEqual(t, validators.InverseRegexValidator(re, "", "")("123"), nil)
	assertInvalid(t, validators.InverseRegexValidator(re, "", ""), "abc", "invalid", "Enter a valid value.")

	// ProhibitNullCharactersValidator.
	djangotest.AssertEqual(t, validators.ProhibitNullCharactersValidator("clean"), nil)
	assertInvalid(t, validators.ProhibitNullCharactersValidator, "bad\x00str", "null_characters_not_allowed",
		"Null characters are not allowed.")

	// FileExtensionValidator.
	fev := validators.FileExtensionValidator([]string{"pdf", ".TXT"}, "", "")
	djangotest.AssertEqual(t, fev("report.PDF"), nil)
	djangotest.AssertEqual(t, fev("notes.txt"), nil)
	assertInvalid(t, fev, "image.png", "invalid_extension",
		"File extension “png” is not allowed. Allowed extensions are: pdf, txt.")
}

// Ported from Django: tests/validators/tests.py::TestValidators.test_validators
func TestSharedValidatorsORMAndFormsTypes(t *testing.T) {
	// Verify orm.ValidationError, forms.ValidationError, and validators.ValidationError
	// are the exact same type via alias.
	err := validators.ValidateEmail("bad")
	var ormVE *orm.ValidationError
	var formsVE *forms.ValidationError
	if !errors.As(err, &ormVE) || !errors.As(err, &formsVE) {
		t.Fatalf("ValidationError alias mismatch: err=%T", err)
	}

	// DecimalValidator works identically on orm.Decimal and forms.Decimal.
	dv := validators.DecimalValidator(4, 2)
	djangotest.AssertEqual(t, dv(orm.MustDecimal("12.34")), nil)
	djangotest.AssertEqual(t, dv(forms.MustDecimal("12.34")), nil)
	assertInvalid(t, dv, orm.MustDecimal("123.45"), "max_digits",
		"Ensure that there are no more than 4 digits in total.")
	assertInvalid(t, dv, orm.MustDecimal("1.234"), "max_decimal_places",
		"Ensure that there are no more than 2 decimal places.")
	assertInvalid(t, dv, orm.MustDecimal("123"), "max_whole_digits",
		"Ensure that there are no more than 2 digits before the decimal point.")
	assertInvalid(t, validators.DecimalValidator(1, 1), orm.MustDecimal("12"), "max_digits",
		"Ensure that there are no more than 1 digit in total.")
	assertInvalid(t, validators.DecimalValidator(5, 1), orm.MustDecimal("1.23"), "max_decimal_places",
		"Ensure that there are no more than 1 decimal place.")

	// MinValue / MaxValue on orm.Decimal, orm.Date, orm.Time.
	djangotest.AssertEqual(t, validators.MinValueValidator(orm.MustDecimal("1.5"))(orm.MustDecimal("1.50")), nil)
	assertInvalid(t, validators.MaxValueValidator(orm.MustDecimal("1.5")), orm.MustDecimal("2.0"), "max_value",
		"Ensure this value is less than or equal to 1.5.")

	d1 := orm.MustDate("2025-01-01")
	d2 := orm.MustDate("2025-06-15")
	djangotest.AssertEqual(t, validators.MinValueValidator(d1)(d2), nil)
	assertInvalid(t, validators.MaxValueValidator(d1), d2, "max_value",
		"Ensure this value is less than or equal to 2025-01-01.")

	t1 := orm.MustTime("09:00:00")
	t2 := orm.MustTime("17:30:00")
	djangotest.AssertEqual(t, validators.MinValueValidator(t1)(t2), nil)
	assertInvalid(t, validators.MaxValueValidator(t1), t2, "max_value",
		"Ensure this value is less than or equal to 09:00:00.")

	// StepValueValidator with orm.Decimal and offset message.
	sv := validators.StepValueValidator(orm.MustDecimal("0.2"), orm.MustDecimal("0.1"))
	djangotest.AssertEqual(t, sv(orm.MustDecimal("0.5")), nil)
	assertInvalid(t, sv, orm.MustDecimal("0.4"), "step_size",
		"Ensure this value is a multiple of step size 0.2, starting from 0.1, e.g. 0.1, 0.3, 0.5, and so on.")

	// IP validators on both string and netip.Addr.
	v4 := netip.MustParseAddr("192.0.2.1")
	v6 := netip.MustParseAddr("2001:db8::1")
	djangotest.AssertEqual(t, validators.ValidateIPv4Address(v4), nil)
	assertInvalid(t, validators.ValidateIPv4Address, v6, "invalid", "Enter a valid IPv4 address.")
	djangotest.AssertEqual(t, validators.ValidateIPv6Address(v6), nil)
	assertInvalid(t, validators.ValidateIPv6Address, v4, "invalid", "Enter a valid IPv6 address.")
	djangotest.AssertEqual(t, validators.ValidateIPv46Address(v4), nil)
	djangotest.AssertEqual(t, validators.ValidateIPv46Address(v6), nil)
	assertInvalid(t, validators.ValidateIPv46Address, netip.Addr{}, "invalid",
		"Enter a valid IPv4 or IPv6 address.")

	// IPAddressValidators helper.
	vs, msg, err := validators.IPAddressValidators("ipv4", false)
	djangotest.AssertEqual(t, err, nil)
	djangotest.AssertEqual(t, len(vs), 1)
	djangotest.AssertEqual(t, msg, "Enter a valid IPv4 address.")
	_, _, err = validators.IPAddressValidators("ipv4", true)
	if err == nil || !strings.Contains(err.Error(), "unpack_ipv4") {
		t.Errorf("IPAddressValidators(ipv4, true) err = %v, want unpack_ipv4 error", err)
	}
	_, _, err = validators.IPAddressValidators("bogus", false)
	if err == nil {
		t.Error("IPAddressValidators(bogus, false) = nil, want error")
	}

	// FormatMessage %()r quotes strings and leaves numbers unquoted.
	djangotest.AssertEqual(t,
		validators.FormatMessage("Value %(value)r is not a valid choice.", map[string]any{"value": "not a"}),
		"Value 'not a' is not a valid choice.")
	djangotest.AssertEqual(t,
		validators.FormatMessage("Value %(value)r is not a valid choice.", map[string]any{"value": 99}),
		"Value 99 is not a valid choice.")
}
