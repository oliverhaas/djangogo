package forms_test

import (
	"regexp"
	"testing"

	"github.com/oliverhaas/djangogo/forms"
)

// Ported from Django: tests/forms_tests/field_tests/test_regexfield.py::RegexFieldTest.test_regexfield_1
func TestRegexfield1(t *testing.T) {
	f := forms.NewRegexField(regexp.MustCompile("^[0-9][A-F][0-9]$"))
	cleans(t, f, "2A2", "2A2")
	cleans(t, f, "3F3", "3F3")
	rejects(t, f, "3G3", "Enter a valid value.")
	rejects(t, f, " 2A2", "Enter a valid value.")
	rejects(t, f, "2A2 ", "Enter a valid value.")
	rejects(t, f, "", "This field is required.")
}

// Ported from Django: tests/forms_tests/field_tests/test_regexfield.py::RegexFieldTest.test_regexfield_2
func TestRegexfield2(t *testing.T) {
	f := forms.NewRegexField(regexp.MustCompile("^[0-9][A-F][0-9]$"), forms.Required(false))
	cleans(t, f, "2A2", "2A2")
	cleans(t, f, "3F3", "3F3")
	rejects(t, f, "3G3", "Enter a valid value.")
	cleans(t, f, "", "")
}

// Ported from Django: tests/forms_tests/field_tests/test_regexfield.py::RegexFieldTest.test_regexfield_3
func TestRegexfield3(t *testing.T) {
	f := forms.NewRegexField(regexp.MustCompile("^[0-9][A-F][0-9]$"))
	cleans(t, f, "2A2", "2A2")
	cleans(t, f, "3F3", "3F3")
	rejects(t, f, "3G3", "Enter a valid value.")
	rejects(t, f, " 2A2", "Enter a valid value.")
	rejects(t, f, "2A2 ", "Enter a valid value.")
}

// Ported from Django: tests/forms_tests/field_tests/test_regexfield.py::RegexFieldTest.test_regexfield_4
func TestRegexfield4(t *testing.T) {
	f := forms.NewRegexField(regexp.MustCompile("^[0-9]+$"), forms.MinLength(5), forms.MaxLength(10))
	rejects(t, f, "123", "Ensure this value has at least 5 characters (it has 3).")
	rejects(t, f, "abc", "Ensure this value has at least 5 characters (it has 3).", "Enter a valid value.")
	cleans(t, f, "12345", "12345")
	cleans(t, f, "1234567890", "1234567890")
	rejects(t, f, "12345678901", "Ensure this value has at most 10 characters (it has 11).")
	rejects(t, f, "12345a", "Enter a valid value.")
}

// Ported from Django: tests/forms_tests/field_tests/test_regexfield.py::RegexFieldTest.test_regexfield_unicode_characters
func TestRegexfieldUnicodeCharacters(t *testing.T) {
	// Go's \w matches ASCII word characters only; Python's matches Unicode
	// letters and digits, which Go spells [\p{L}\p{N}_].
	f := forms.NewRegexField(regexp.MustCompile(`^[\p{L}\p{N}_]+$`))
	cleans(t, f, "éèøçÎÎ你好", "éèøçÎÎ你好")
}

// Ported from Django: tests/forms_tests/field_tests/test_regexfield.py::RegexFieldTest.test_regexfield_strip
func TestRegexfieldStrip(t *testing.T) {
	f := forms.NewRegexField(regexp.MustCompile("^[a-z]+$"), forms.Strip(true))
	cleans(t, f, " a", "a")
	cleans(t, f, "a ", "a")
}

// Ported from Django: tests/forms_tests/field_tests/test_regexfield.py::RegexFieldTest.test_empty_value
func TestRegexfieldEmptyValue(t *testing.T) {
	f := forms.NewRegexField(regexp.MustCompile(""), forms.Required(false))
	cleans(t, f, "", "")
	cleans(t, f, nil, "")
	f = forms.NewRegexField(regexp.MustCompile(""), forms.EmptyValue(nil), forms.Required(false))
	cleans(t, f, "", nil)
	cleans(t, f, nil, nil)
}
