package auth_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/auth"
	"github.com/oliverhaas/djangogo/djangotest"
)

// usePasswordValidators configures the password validators for the test.
func usePasswordValidators(t *testing.T, validators ...auth.PasswordValidator) {
	t.Helper()
	cfg := auth.CurrentConfig()
	cfg.PasswordValidators = append([]auth.PasswordValidator{}, validators...)
	t.Cleanup(auth.Configure(cfg))
}

// useTestValidators installs the validators of Django's PasswordValidationTest:
// the common password list and a minimum length of 12.
func useTestValidators(t *testing.T) {
	t.Helper()
	usePasswordValidators(t, auth.CommonPasswordValidator{}, auth.MinimumLengthValidator{MinLength: 12})
}

// passwordError returns err as the error of a single validator.
func passwordError(t *testing.T, err error) *auth.PasswordError {
	t.Helper()
	var pe *auth.PasswordError
	if !errors.As(err, &pe) {
		t.Fatalf("error = %v (%T), want a *auth.PasswordError", err, err)
	}
	return pe
}

// passwordErrors returns err as the errors of ValidatePassword.
func passwordErrors(t *testing.T, err error) auth.PasswordErrors {
	t.Helper()
	var errs auth.PasswordErrors
	if !errors.As(err, &errs) {
		t.Fatalf("error = %v (%T), want auth.PasswordErrors", err, err)
	}
	return errs
}

// testClientUser is the user of Django's UserAttributeSimilarityValidatorTest.
func testClientUser() *auth.User {
	return &auth.User{
		Username:  "testclient",
		Email:     "testclient@example.com",
		FirstName: "Test",
		LastName:  "Client",
	}
}

// Ported from Django: tests/auth_tests/test_validators.py::PasswordValidationTest.test_get_default_password_validators
func TestDjangoGetDefaultPasswordValidators(t *testing.T) {
	useTestValidators(t)
	validators := auth.ConfiguredPasswordValidators()
	djangotest.AssertEqual(t, len(validators), 2)
	if _, ok := validators[0].(auth.CommonPasswordValidator); !ok {
		t.Errorf("first validator is a %T, want auth.CommonPasswordValidator", validators[0])
	}
	minLength, ok := validators[1].(auth.MinimumLengthValidator)
	if !ok {
		t.Fatalf("second validator is a %T, want auth.MinimumLengthValidator", validators[1])
	}
	djangotest.AssertEqual(t, minLength.MinLength, 12)
}

// Ported from Django: tests/auth_tests/test_validators.py::PasswordValidationTest.test_validate_password
func TestDjangoValidatePassword(t *testing.T) {
	useTestValidators(t)
	must(t, auth.ValidatePassword("sufficiently-long", nil, nil))
	const msgTooShort = "This password is too short. It must contain at least 12 characters."

	errs := passwordErrors(t, auth.ValidatePassword("django4242", nil, nil))
	djangotest.AssertSequenceEqual(t, errs.Messages(), []string{msgTooShort})
	djangotest.AssertEqual(t, errs[0].Code, "password_too_short")

	errs = passwordErrors(t, auth.ValidatePassword("password", nil, nil))
	djangotest.AssertSequenceEqual(t, errs.Messages(), []string{"This password is too common.", msgTooShort})
	djangotest.AssertEqual(t, errs[0].Code, "password_too_common")

	must(t, auth.ValidatePassword("password", nil, []auth.PasswordValidator{}))
}

// Ported from Django: tests/auth_tests/test_validators.py::PasswordValidationTest.test_password_validators_help_texts
func TestDjangoPasswordValidatorsHelpTexts(t *testing.T) {
	useTestValidators(t)
	helpTexts := auth.PasswordValidatorsHelpTexts(nil)
	djangotest.AssertEqual(t, len(helpTexts), 2)
	if !strings.Contains(helpTexts[1], "12 characters") {
		t.Errorf("help text %q does not mention 12 characters", helpTexts[1])
	}
	djangotest.AssertEqual(t, len(auth.PasswordValidatorsHelpTexts([]auth.PasswordValidator{})), 0)
}

// Ported from Django: tests/auth_tests/test_validators.py::PasswordValidationTest.test_password_validators_help_text_html
func TestDjangoPasswordValidatorsHelpTextHTML(t *testing.T) {
	useTestValidators(t)
	helpText := auth.PasswordValidatorsHelpTextHTML(nil)
	djangotest.AssertEqual(t, strings.Count(helpText, "<li>"), 2)
	if !strings.Contains(helpText, "12 characters") {
		t.Errorf("help text %q does not mention 12 characters", helpText)
	}
}

// ampersandValidator has a help text that needs escaping.
type ampersandValidator struct{}

func (ampersandValidator) Validate(string, *auth.User) error { return nil }
func (ampersandValidator) HelpText() string                  { return "Must contain &" }

// Ported from Django: tests/auth_tests/test_validators.py::PasswordValidationTest.test_password_validators_help_text_html_escaping
func TestDjangoPasswordValidatorsHelpTextHTMLEscaping(t *testing.T) {
	helpText := auth.PasswordValidatorsHelpTextHTML([]auth.PasswordValidator{ampersandValidator{}})
	djangotest.AssertEqual(t, helpText, "<ul><li>Must contain &amp;</li></ul>")
}

// Ported from Django: tests/auth_tests/test_validators.py::PasswordValidationTest.test_empty_password_validator_help_text_html
func TestDjangoEmptyPasswordValidatorHelpTextHTML(t *testing.T) {
	usePasswordValidators(t)
	djangotest.AssertEqual(t, auth.PasswordValidatorsHelpTextHTML(nil), "")
}

// Ported from Django: tests/auth_tests/test_validators.py::MinimumLengthValidatorTest.test_validate
func TestDjangoMinimumLengthValidatorValidate(t *testing.T) {
	must(t, auth.MinimumLengthValidator{}.Validate("12345678", nil))
	must(t, auth.MinimumLengthValidator{MinLength: 3}.Validate("123", nil))

	pe := passwordError(t, auth.MinimumLengthValidator{}.Validate("1234567", nil))
	djangotest.AssertEqual(t, pe.Message, "This password is too short. It must contain at least 8 characters.")
	djangotest.AssertEqual(t, pe.Code, "password_too_short")
	djangotest.AssertEqual(t, pe.Params["min_length"], any(8))

	pe = passwordError(t, auth.MinimumLengthValidator{MinLength: 3}.Validate("12", nil))
	djangotest.AssertEqual(t, pe.Message, "This password is too short. It must contain at least 3 characters.")
	djangotest.AssertEqual(t, pe.Code, "password_too_short")
	djangotest.AssertEqual(t, pe.Params["min_length"], any(3))
}

// Ported from Django: tests/auth_tests/test_validators.py::MinimumLengthValidatorTest.test_help_text
func TestDjangoMinimumLengthValidatorHelpText(t *testing.T) {
	djangotest.AssertEqual(t, auth.MinimumLengthValidator{}.HelpText(),
		"Your password must contain at least 8 characters.")
}

// Ported from Django: tests/auth_tests/test_validators.py::MinimumLengthValidatorTest.test_l10n
//
// Django checks the two forms it hands to ngettext. There is no translation
// catalog here, so the test checks the singular form of both texts.
func TestDjangoMinimumLengthValidatorL10n(t *testing.T) {
	v := auth.MinimumLengthValidator{MinLength: 1}
	pe := passwordError(t, v.Validate("", nil))
	djangotest.AssertEqual(t, pe.Message, "This password is too short. It must contain at least 1 character.")
	djangotest.AssertEqual(t, v.HelpText(), "Your password must contain at least 1 character.")
}

// Ported from Django: tests/auth_tests/test_validators.py::UserAttributeSimilarityValidatorTest.test_validate
func TestDjangoUserAttributeSimilarityValidatorValidate(t *testing.T) {
	user := testClientUser()
	const expected = "The password is too similar to the "

	must(t, auth.UserAttributeSimilarityValidator{}.Validate("testclient", nil))

	pe := passwordError(t, auth.UserAttributeSimilarityValidator{}.Validate("testclient", user))
	djangotest.AssertEqual(t, pe.Message, expected+"username.")
	djangotest.AssertEqual(t, pe.Code, "password_too_similar")

	pe = passwordError(t, auth.UserAttributeSimilarityValidator{}.Validate("example.com", user))
	djangotest.AssertEqual(t, pe.Message, expected+"email address.")

	firstName := auth.UserAttributeSimilarityValidator{UserAttributes: []string{"first_name"}, MaxSimilarity: 0.3}
	pe = passwordError(t, firstName.Validate("testclient", user))
	djangotest.AssertEqual(t, pe.Message, expected+"first name.")

	// A limit of 1 still refuses a password that equals the value.
	firstName.MaxSimilarity = 1
	pe = passwordError(t, firstName.Validate(user.FirstName, user))
	djangotest.AssertEqual(t, pe.Message, expected+"first name.")

	// Django raises ValueError("max_similarity must be at least 0.1") for a
	// lower limit. A struct literal cannot fail, so the limit counts as 0.1.
	firstName.MaxSimilarity = 0.09
	atLimit := auth.UserAttributeSimilarityValidator{UserAttributes: []string{"first_name"}, MaxSimilarity: 0.1}
	for _, password := range []string{"t", "zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzt", "zzzz"} {
		got, want := firstName.Validate(password, user), atLimit.Validate(password, user)
		if (got == nil) != (want == nil) {
			t.Errorf("limit 0.09 on %q = %v, limit 0.1 = %v", password, got, want)
		}
	}

	must(t, auth.UserAttributeSimilarityValidator{UserAttributes: []string{"first_name"}}.Validate("testclient", user))
}

// Ported from Django: tests/auth_tests/test_validators.py::UserAttributeSimilarityValidatorTest.test_help_text
func TestDjangoUserAttributeSimilarityValidatorHelpText(t *testing.T) {
	djangotest.AssertEqual(t, auth.UserAttributeSimilarityValidator{}.HelpText(),
		"Your password can’t be too similar to your other personal information.")
}

// Ported from Django: tests/auth_tests/test_validators.py::CommonPasswordValidatorTest.test_validate
func TestDjangoCommonPasswordValidatorValidate(t *testing.T) {
	must(t, auth.CommonPasswordValidator{}.Validate("a-safe-password", nil))
	pe := passwordError(t, auth.CommonPasswordValidator{}.Validate("godzilla", nil))
	djangotest.AssertEqual(t, pe.Message, "This password is too common.")
}

// Ported from Django: tests/auth_tests/test_validators.py::CommonPasswordValidatorTest.test_common_hexed_codes
func TestDjangoCommonPasswordValidatorCommonHexedCodes(t *testing.T) {
	for _, password := range []string{"asdfjkl:", "&#2336:"} {
		t.Run(password, func(t *testing.T) {
			pe := passwordError(t, auth.CommonPasswordValidator{}.Validate(password, nil))
			djangotest.AssertEqual(t, pe.Message, "This password is too common.")
		})
	}
}

// Ported from Django: tests/auth_tests/test_validators.py::CommonPasswordValidatorTest.test_validate_custom_list
//
// Django reads the custom list from a file; here it is a slice.
func TestDjangoCommonPasswordValidatorValidateCustomList(t *testing.T) {
	validator := auth.NewCommonPasswordValidator([]string{"from-my-custom-list"})
	must(t, validator.Validate("a-safe-password", nil))

	pe := passwordError(t, validator.Validate("from-my-custom-list", nil))
	djangotest.AssertEqual(t, pe.Message, "This password is too common.")
	djangotest.AssertEqual(t, pe.Code, "password_too_common")
}

// Ported from Django: tests/auth_tests/test_validators.py::CommonPasswordValidatorTest.test_validate_django_supplied_file
func TestDjangoCommonPasswordValidatorValidateDjangoSuppliedFile(t *testing.T) {
	passwords := auth.CommonPasswords()
	if len(passwords) < 19000 {
		t.Fatalf("the embedded list has %d passwords, want about 20000", len(passwords))
	}
	for password := range passwords {
		if password != strings.ToLower(password) {
			t.Errorf("password %q of the embedded list is not in lower case", password)
		}
	}
}

// Ported from Django: tests/auth_tests/test_validators.py::CommonPasswordValidatorTest.test_help_text
func TestDjangoCommonPasswordValidatorHelpText(t *testing.T) {
	djangotest.AssertEqual(t, auth.CommonPasswordValidator{}.HelpText(),
		"Your password can’t be a commonly used password.")
}

// Ported from Django: tests/auth_tests/test_validators.py::NumericPasswordValidatorTest.test_validate
func TestDjangoNumericPasswordValidatorValidate(t *testing.T) {
	must(t, auth.NumericPasswordValidator{}.Validate("a-safe-password", nil))

	pe := passwordError(t, auth.NumericPasswordValidator{}.Validate("42424242", nil))
	djangotest.AssertEqual(t, pe.Message, "This password is entirely numeric.")
	djangotest.AssertEqual(t, pe.Code, "password_entirely_numeric")
}

// Ported from Django: tests/auth_tests/test_validators.py::NumericPasswordValidatorTest.test_help_text
func TestDjangoNumericPasswordValidatorHelpText(t *testing.T) {
	djangotest.AssertEqual(t, auth.NumericPasswordValidator{}.HelpText(),
		"Your password can’t be entirely numeric.")
}

// Ported from Django: tests/auth_tests/test_validators.py::UsernameValidatorsTests.test_unicode_validator
func TestDjangoUnicodeUsernameValidator(t *testing.T) {
	valid := []string{"joe", "René", "ᴮᴵᴳᴮᴵᴿᴰ", "أحمد"}
	invalid := []string{
		"o'connell",
		"عبد ال",
		"zerowidth" + string(rune(0x200B)) + "space",
		"nonbreaking" + string(rune(0x00A0)) + "space",
		"en" + string(rune(0x2013)) + "dash",
		"trailingnewline\n",
	}
	for _, username := range valid {
		t.Run(username, func(t *testing.T) { must(t, auth.ValidateUsername(username)) })
	}
	for _, username := range invalid {
		t.Run(username, func(t *testing.T) {
			var ue *auth.UsernameError
			if err := auth.ValidateUsername(username); !errors.As(err, &ue) {
				t.Errorf("ValidateUsername(%q) = %v, want a *auth.UsernameError", username, err)
			}
		})
	}
}

// Ported from Django: tests/auth_tests/test_validators.py::UsernameValidatorsTests.test_ascii_validator
func TestDjangoASCIIUsernameValidator(t *testing.T) {
	valid := []string{"glenn", "GLEnN", "jean-marc"}
	invalid := []string{"o'connell", "Éric", "jean marc", "أحمد", "trailingnewline\n"}
	for _, username := range valid {
		t.Run(username, func(t *testing.T) { must(t, auth.ValidateASCIIUsername(username)) })
	}
	for _, username := range invalid {
		t.Run(username, func(t *testing.T) {
			var ue *auth.UsernameError
			if err := auth.ValidateASCIIUsername(username); !errors.As(err, &ue) {
				t.Errorf("ValidateASCIIUsername(%q) = %v, want a *auth.UsernameError", username, err)
			}
		})
	}
}
