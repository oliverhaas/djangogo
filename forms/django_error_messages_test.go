package forms_test

import (
	"regexp"
	"testing"

	"github.com/oliverhaas/djangogo/forms"
)

// lengthMessages returns the custom required, min_length and max_length
// messages shared by the char-like field tests.
func lengthMessages() map[string]string {
	return map[string]string{
		"required":   "REQUIRED",
		"min_length": "LENGTH %(show_value)s, MIN LENGTH %(limit_value)s",
		"max_length": "LENGTH %(show_value)s, MAX LENGTH %(limit_value)s",
	}
}

// numberMessages returns the custom messages shared by the numeric field
// tests.
func numberMessages() map[string]string {
	return map[string]string{
		"required":  "REQUIRED",
		"invalid":   "INVALID",
		"min_value": "MIN VALUE IS %(limit_value)s",
		"max_value": "MAX VALUE IS %(limit_value)s",
	}
}

// Ported from Django: tests/forms_tests/tests/test_error_messages.py::FormsErrorMessagesTestCase.test_charfield
func TestErrorMessagesCharfield(t *testing.T) {
	f := forms.NewCharField(forms.MinLength(5), forms.MaxLength(10), forms.ErrorMessages(lengthMessages()))
	rejects(t, f, "", "REQUIRED")
	rejects(t, f, "1234", "LENGTH 4, MIN LENGTH 5")
	rejects(t, f, "12345678901", "LENGTH 11, MAX LENGTH 10")
}

// Ported from Django: tests/forms_tests/tests/test_error_messages.py::FormsErrorMessagesTestCase.test_integerfield
func TestErrorMessagesIntegerfield(t *testing.T) {
	f := forms.NewIntegerField(forms.MinValue(5), forms.MaxValue(10), forms.ErrorMessages(numberMessages()))
	rejects(t, f, "", "REQUIRED")
	rejects(t, f, "abc", "INVALID")
	rejects(t, f, "4", "MIN VALUE IS 5")
	rejects(t, f, "11", "MAX VALUE IS 10")
}

// Ported from Django: tests/forms_tests/tests/test_error_messages.py::FormsErrorMessagesTestCase.test_floatfield
func TestErrorMessagesFloatfield(t *testing.T) {
	f := forms.NewFloatField(forms.MinValue(5), forms.MaxValue(10), forms.ErrorMessages(numberMessages()))
	rejects(t, f, "", "REQUIRED")
	rejects(t, f, "abc", "INVALID")
	rejects(t, f, "4", "MIN VALUE IS 5")
	rejects(t, f, "11", "MAX VALUE IS 10")
}

// Ported from Django: tests/forms_tests/tests/test_error_messages.py::FormsErrorMessagesTestCase.test_decimalfield
func TestErrorMessagesDecimalfield(t *testing.T) {
	e := numberMessages()
	e["max_digits"] = "MAX DIGITS IS %(max)s"
	e["max_decimal_places"] = "MAX DP IS %(max)s"
	e["max_whole_digits"] = "MAX DIGITS BEFORE DP IS %(max)s"
	f := forms.NewDecimalField(forms.MinValue(5), forms.MaxValue(10), forms.ErrorMessages(e))
	rejects(t, f, "", "REQUIRED")
	rejects(t, f, "abc", "INVALID")
	rejects(t, f, "4", "MIN VALUE IS 5")
	rejects(t, f, "11", "MAX VALUE IS 10")

	f2 := forms.NewDecimalField(forms.MaxDigits(4), forms.DecimalPlaces(2), forms.ErrorMessages(e))
	rejects(t, f2, "123.45", "MAX DIGITS IS 4")
	rejects(t, f2, "1.234", "MAX DP IS 2")
	rejects(t, f2, "123.4", "MAX DIGITS BEFORE DP IS 2")
}

// Ported from Django: tests/forms_tests/tests/test_error_messages.py::FormsErrorMessagesTestCase.test_datefield
func TestErrorMessagesDatefield(t *testing.T) {
	f := forms.NewDateField(forms.ErrorMessages(map[string]string{"required": "REQUIRED", "invalid": "INVALID"}))
	rejects(t, f, "", "REQUIRED")
	rejects(t, f, "abc", "INVALID")
}

// Ported from Django: tests/forms_tests/tests/test_error_messages.py::FormsErrorMessagesTestCase.test_timefield
func TestErrorMessagesTimefield(t *testing.T) {
	f := forms.NewTimeField(forms.ErrorMessages(map[string]string{"required": "REQUIRED", "invalid": "INVALID"}))
	rejects(t, f, "", "REQUIRED")
	rejects(t, f, "abc", "INVALID")
}

// Ported from Django: tests/forms_tests/tests/test_error_messages.py::FormsErrorMessagesTestCase.test_datetimefield
func TestErrorMessagesDatetimefield(t *testing.T) {
	f := forms.NewDateTimeField(forms.ErrorMessages(map[string]string{"required": "REQUIRED", "invalid": "INVALID"}))
	rejects(t, f, "", "REQUIRED")
	rejects(t, f, "abc", "INVALID")
}

// Ported from Django: tests/forms_tests/tests/test_error_messages.py::FormsErrorMessagesTestCase.test_regexfield
func TestErrorMessagesRegexfield(t *testing.T) {
	e := lengthMessages()
	e["invalid"] = "INVALID"
	f := forms.NewRegexField(regexp.MustCompile(`^[0-9]+$`), forms.MinLength(5), forms.MaxLength(10), forms.ErrorMessages(e))
	rejects(t, f, "", "REQUIRED")
	rejects(t, f, "abcde", "INVALID")
	rejects(t, f, "1234", "LENGTH 4, MIN LENGTH 5")
	rejects(t, f, "12345678901", "LENGTH 11, MAX LENGTH 10")
}

// Ported from Django: tests/forms_tests/tests/test_error_messages.py::FormsErrorMessagesTestCase.test_emailfield
func TestErrorMessagesEmailfield(t *testing.T) {
	e := lengthMessages()
	e["invalid"] = "INVALID"
	f := forms.NewEmailField(forms.MinLength(8), forms.MaxLength(10), forms.ErrorMessages(e))
	rejects(t, f, "", "REQUIRED")
	rejects(t, f, "abcdefgh", "INVALID")
	rejects(t, f, "a@b.com", "LENGTH 7, MIN LENGTH 8")
	rejects(t, f, "aye@bee.com", "LENGTH 11, MAX LENGTH 10")
}

// Ported from Django: tests/forms_tests/tests/test_error_messages.py::FormsErrorMessagesTestCase.test_urlfield
func TestErrorMessagesURLfield(t *testing.T) {
	e := map[string]string{
		"required":   "REQUIRED",
		"invalid":    "INVALID",
		"max_length": `"%(value)s" has more than %(limit_value)d characters.`,
	}
	f := forms.NewURLField(forms.ErrorMessages(e), forms.MaxLength(17))
	rejects(t, f, "", "REQUIRED")
	rejects(t, f, "abc.c", "INVALID")
	rejects(t, f, "djangoproject.com", `"https://djangoproject.com" has more than 17 characters.`)
}

// Ported from Django: tests/forms_tests/tests/test_error_messages.py::FormsErrorMessagesTestCase.test_booleanfield
func TestErrorMessagesBooleanfield(t *testing.T) {
	f := forms.NewBooleanField(forms.ErrorMessages(map[string]string{"required": "REQUIRED"}))
	rejects(t, f, "", "REQUIRED")
}

// Ported from Django: tests/forms_tests/tests/test_error_messages.py::FormsErrorMessagesTestCase.test_choicefield
func TestErrorMessagesChoicefield(t *testing.T) {
	e := map[string]string{"required": "REQUIRED", "invalid_choice": "%(value)s IS INVALID CHOICE"}
	f := forms.NewChoiceField(forms.Choices("a", "aye"), forms.ErrorMessages(e))
	rejects(t, f, "", "REQUIRED")
	rejects(t, f, "b", "b IS INVALID CHOICE")
}

// Ported from Django: tests/forms_tests/tests/test_error_messages.py::FormsErrorMessagesTestCase.test_multiplechoicefield
func TestErrorMessagesMultiplechoicefield(t *testing.T) {
	e := map[string]string{
		"required":       "REQUIRED",
		"invalid_choice": "%(value)s IS INVALID CHOICE",
		"invalid_list":   "NOT A LIST",
	}
	f := forms.NewMultipleChoiceField(forms.Choices("a", "aye"), forms.ErrorMessages(e))
	rejects(t, f, "", "REQUIRED")
	rejects(t, f, "b", "NOT A LIST")
	rejects(t, f, []string{"b"}, "b IS INVALID CHOICE")
}

// Ported from Django: tests/forms_tests/tests/test_error_messages.py::FormsErrorMessagesTestCase.test_error_messages_escaping
//
// Django renders form.errors through a template; Go renders it with AsUL,
// which escapes the same way. The ModelMultipleChoiceField part needs a
// database and is not ported.
func TestErrorMessagesEscaping(t *testing.T) {
	want := `<ul class="errorlist"><li>field<ul class="errorlist" id="id_field_error">` +
		`<li>Select a valid choice. &lt;script&gt; is not one of the ` +
		`available choices.</li></ul></li></ul>`

	f := forms.New(forms.F("field", forms.NewChoiceField(forms.Choices("one", "One"))))
	f = f.Bind(vals("field", "<script>"))
	assertHTMLEqual(t, f.Errors().AsUL(), want)

	f = forms.New(forms.F("field", forms.NewMultipleChoiceField(forms.Choices("one", "One"))))
	f = f.Bind(vals("field", "<script>"))
	assertHTMLEqual(t, f.Errors().AsUL(), want)
}
