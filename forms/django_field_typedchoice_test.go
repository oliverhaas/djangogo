package forms_test

import (
	"strconv"
	"testing"

	"github.com/oliverhaas/djangogo/forms"
)

// coerceInt is Python's int as a coerce function.
func coerceInt(s string) (any, error) {
	i, err := strconv.Atoi(s)
	return i, err
}

// coerceFloat is Python's float as a coerce function.
func coerceFloat(s string) (any, error) { return strconv.ParseFloat(s, 64) }

// coerceBool is Python's bool as a coerce function: any non-empty string is
// true.
func coerceBool(s string) (any, error) { return s != "", nil }

// Ported from Django: tests/forms_tests/field_tests/test_typedchoicefield.py::TypedChoiceFieldTest.test_typedchoicefield_1
func TestTypedchoicefield1(t *testing.T) {
	f := forms.NewTypedChoiceField(forms.Choices("1", "+1", "-1", "-1"), forms.Coerce(coerceInt))
	cleans(t, f, "1", 1)
	rejects(t, f, "2", "Select a valid choice. 2 is not one of the available choices.")
}

// Ported from Django: tests/forms_tests/field_tests/test_typedchoicefield.py::TypedChoiceFieldTest.test_typedchoicefield_2
func TestTypedchoicefield2(t *testing.T) {
	// Different coercion, same validation.
	f := forms.NewTypedChoiceField(forms.Choices("1", "+1", "-1", "-1"), forms.Coerce(coerceFloat))
	cleans(t, f, "1", 1.0)
}

// Ported from Django: tests/forms_tests/field_tests/test_typedchoicefield.py::TypedChoiceFieldTest.test_typedchoicefield_3
func TestTypedchoicefield3(t *testing.T) {
	// This can also cause weirdness: be careful (bool(-1) == True,
	// remember)
	f := forms.NewTypedChoiceField(forms.Choices("1", "+1", "-1", "-1"), forms.Coerce(coerceBool))
	cleans(t, f, "-1", true)
}

// Ported from Django: tests/forms_tests/field_tests/test_typedchoicefield.py::TypedChoiceFieldTest.test_typedchoicefield_4
func TestTypedchoicefield4(t *testing.T) {
	// Even more weirdness: if you have a valid choice but your coercion
	// function can't coerce, you'll still get a validation error. Don't do
	// this!
	f := forms.NewTypedChoiceField(forms.Choices("A", "A", "B", "B"), forms.Coerce(coerceInt))
	rejects(t, f, "B", "Select a valid choice. B is not one of the available choices.")
	// Required fields require values
	rejects(t, f, "", "This field is required.")
}

// Ported from Django: tests/forms_tests/field_tests/test_typedchoicefield.py::TypedChoiceFieldTest.test_typedchoicefield_5
func TestTypedchoicefield5(t *testing.T) {
	// Non-required fields aren't required
	f := forms.NewTypedChoiceField(forms.Choices("1", "+1", "-1", "-1"), forms.Coerce(coerceInt), forms.Required(false))
	cleans(t, f, "", "")
	// If you want cleaning an empty value to return a different type, tell
	// the field
}

// Ported from Django: tests/forms_tests/field_tests/test_typedchoicefield.py::TypedChoiceFieldTest.test_typedchoicefield_6
func TestTypedchoicefield6(t *testing.T) {
	f := forms.NewTypedChoiceField(
		forms.Choices("1", "+1", "-1", "-1"), forms.Coerce(coerceInt),
		forms.Required(false), forms.EmptyValue(nil),
	)
	cleans(t, f, "", nil)
}

// Ported from Django: tests/forms_tests/field_tests/test_typedchoicefield.py::TypedChoiceFieldTest.test_typedchoicefield_has_changed
func TestTypedchoicefieldHasChanged(t *testing.T) {
	// has_changed should not trigger required validation
	f := forms.NewTypedChoiceField(forms.Choices("1", "+1", "-1", "-1"), forms.Coerce(coerceInt), forms.Required(true))
	equal(t, f.HasChanged(nil, ""), false)
	equal(t, f.HasChanged(1, "1"), false)
	equal(t, f.HasChanged("1", "1"), false)

	f = forms.NewTypedChoiceField(
		forms.Choices("", forms.EmptyChoiceLabel, "a", "a", "b", "b"),
		forms.Coerce(func(s string) (any, error) { return s, nil }),
		forms.Required(false), forms.Initial(nil), forms.EmptyValue(nil),
	)
	equal(t, f.HasChanged(nil, ""), false)
	equal(t, f.HasChanged("", "a"), true)
	equal(t, f.HasChanged("a", "a"), false)
}

// Ported from Django: tests/forms_tests/field_tests/test_typedchoicefield.py::TypedChoiceFieldTest.test_typedchoicefield_special_coerce
func TestTypedchoicefieldSpecialCoerce(t *testing.T) {
	// A coerce function which results in a value not present in choices
	// should raise an appropriate error (#21397).
	coerceFunc := func(val string) (any, error) { return forms.MustDecimal("1." + val), nil }
	f := forms.NewTypedChoiceField(forms.Choices("1", "1", "2", "2"), forms.Coerce(coerceFunc), forms.Required(true))
	cleans(t, f, "2", forms.MustDecimal("1.2"))
	rejects(t, f, "", "This field is required.")
	rejects(t, f, "3", "Select a valid choice. 3 is not one of the available choices.")
}
