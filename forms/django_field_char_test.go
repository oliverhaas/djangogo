package forms_test

import (
	"testing"

	"github.com/oliverhaas/djangogo/forms"
)

// rendersTo asserts the HTML of field rendered as the field "f" of an unbound
// form, Django's FormFieldAssertionsMixin.assertWidgetRendersTo.
func rendersTo(t *testing.T, field forms.Field, want string) {
	t.Helper()
	assertHTMLEqual(t, forms.New(forms.F("f", field)).Field("f").String(), want)
}

// Ported from Django: tests/forms_tests/field_tests/test_charfield.py::CharFieldTest.test_charfield_1
func TestCharfield1(t *testing.T) {
	f := forms.NewCharField()
	cleans(t, f, 1, "1")
	cleans(t, f, "hello", "hello")
	rejects(t, f, nil, "This field is required.")
	rejects(t, f, "", "This field is required.")
	cleans(t, f, []any{1, 2, 3}, "[1, 2, 3]")
	equal(t, f.MaxLength, 0)
	equal(t, f.MinLength, 0)
}

// Ported from Django: tests/forms_tests/field_tests/test_charfield.py::CharFieldTest.test_charfield_2
func TestCharfield2(t *testing.T) {
	f := forms.NewCharField(forms.Required(false))
	cleans(t, f, 1, "1")
	cleans(t, f, "hello", "hello")
	cleans(t, f, nil, "")
	cleans(t, f, "", "")
	cleans(t, f, []any{1, 2, 3}, "[1, 2, 3]")
}

// Ported from Django: tests/forms_tests/field_tests/test_charfield.py::CharFieldTest.test_charfield_3
func TestCharfield3(t *testing.T) {
	f := forms.NewCharField(forms.MaxLength(10), forms.Required(false))
	cleans(t, f, "12345", "12345")
	cleans(t, f, "1234567890", "1234567890")
	rejects(t, f, "1234567890a", "Ensure this value has at most 10 characters (it has 11).")
	equal(t, f.MaxLength, 10)
	equal(t, f.MinLength, 0)
}

// Ported from Django: tests/forms_tests/field_tests/test_charfield.py::CharFieldTest.test_charfield_4
func TestCharfield4(t *testing.T) {
	f := forms.NewCharField(forms.MinLength(10), forms.Required(false))
	cleans(t, f, "", "")
	rejects(t, f, "12345", "Ensure this value has at least 10 characters (it has 5).")
	cleans(t, f, "1234567890", "1234567890")
	cleans(t, f, "1234567890a", "1234567890a")
	equal(t, f.MinLength, 10)
}

// Ported from Django: tests/forms_tests/field_tests/test_charfield.py::CharFieldTest.test_charfield_5
func TestCharfield5(t *testing.T) {
	f := forms.NewCharField(forms.MinLength(10), forms.Required(true))
	rejects(t, f, "", "This field is required.")
	rejects(t, f, "12345", "Ensure this value has at least 10 characters (it has 5).")
	cleans(t, f, "1234567890", "1234567890")
	cleans(t, f, "1234567890a", "1234567890a")
}

// Ported from Django: tests/forms_tests/field_tests/test_charfield.py::CharFieldTest.test_charfield_widget_attrs
func TestCharfieldWidgetAttrs(t *testing.T) {
	f := forms.NewCharField()
	equal(t, f.WidgetAttrs(forms.TextInput{}), forms.Attrs{})
	equal(t, f.WidgetAttrs(forms.Textarea{}), forms.Attrs{})

	f = forms.NewCharField(forms.MaxLength(10))
	for _, w := range []forms.Widget{forms.TextInput{}, forms.PasswordInput{}, forms.Textarea{}} {
		equal(t, f.WidgetAttrs(w), forms.Attrs{"maxlength": "10"})
	}
	f = forms.NewCharField(forms.MinLength(5))
	for _, w := range []forms.Widget{forms.TextInput{}, forms.PasswordInput{}, forms.Textarea{}} {
		equal(t, f.WidgetAttrs(w), forms.Attrs{"minlength": "5"})
	}
	f = forms.NewCharField(forms.MaxLength(10), forms.MinLength(5))
	for _, w := range []forms.Widget{forms.TextInput{}, forms.PasswordInput{}, forms.Textarea{}} {
		equal(t, f.WidgetAttrs(w), forms.Attrs{"maxlength": "10", "minlength": "5"})
	}
	equal(t, f.WidgetAttrs(forms.HiddenInput{}), forms.Attrs{})
}

// Ported from Django: tests/forms_tests/field_tests/test_charfield.py::CharFieldTest.test_charfield_strip
func TestCharfieldStrip(t *testing.T) {
	f := forms.NewCharField()
	cleans(t, f, " 1", "1")
	cleans(t, f, "1 ", "1")

	f = forms.NewCharField(forms.Strip(false))
	cleans(t, f, " 1", " 1")
	cleans(t, f, "1 ", "1 ")
}

// Ported from Django: tests/forms_tests/field_tests/test_charfield.py::CharFieldTest.test_strip_before_checking_empty
func TestStripBeforeCheckingEmpty(t *testing.T) {
	f := forms.NewCharField(forms.Required(false), forms.EmptyValue(nil))
	cleans(t, f, " ", nil)
}

// stringWrapper is a value whose text is its String method, the StringWrapper
// of test_clean_non_string.
type stringWrapper struct{ v string }

// String returns the wrapped text.
func (s stringWrapper) String() string { return s.v }

// Ported from Django: tests/forms_tests/field_tests/test_charfield.py::CharFieldTest.test_clean_non_string
func TestCleanNonString(t *testing.T) {
	value := stringWrapper{" "}
	cleans(t, forms.NewCharField(forms.Required(false), forms.EmptyValue(nil)), value, nil)
	cleans(t, forms.NewCharField(forms.Strip(false)), value, " ")
}

// Ported from Django: tests/forms_tests/field_tests/test_charfield.py::CharFieldTest.test_charfield_disabled
func TestCharfieldDisabled(t *testing.T) {
	rendersTo(t, forms.NewCharField(forms.Disabled()), `<input type="text" name="f" id="id_f" disabled required>`)
}

// Ported from Django: tests/forms_tests/field_tests/test_charfield.py::CharFieldTest.test_null_characters_prohibited
func TestNullCharactersProhibited(t *testing.T) {
	rejects(t, forms.NewCharField(), "\x00something", "Null characters are not allowed.")
}
