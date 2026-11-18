package forms_test

import (
	"testing"

	"github.com/oliverhaas/djangogo/forms"
)

// plusMinusOne are the choices (1, "+1"), (-1, "-1") of the Django tests.
var plusMinusOne = forms.Choices("1", "+1", "-1", "-1")

// Ported from Django: tests/forms_tests/field_tests/test_typedmultiplechoicefield.py::TypedMultipleChoiceFieldTest.test_typedmultiplechoicefield_1
func TestTypedmultiplechoicefield1(t *testing.T) {
	f := forms.NewTypedMultipleChoiceField(plusMinusOne, forms.Coerce(coerceInt))
	cleans(t, f, []string{"1"}, []any{1})
	rejects(t, f, []string{"2"}, "Select a valid choice. 2 is not one of the available choices.")
}

// Ported from Django: tests/forms_tests/field_tests/test_typedmultiplechoicefield.py::TypedMultipleChoiceFieldTest.test_typedmultiplechoicefield_2
func TestTypedmultiplechoicefield2(t *testing.T) {
	// Different coercion, same validation.
	f := forms.NewTypedMultipleChoiceField(plusMinusOne, forms.Coerce(coerceFloat))
	cleans(t, f, []string{"1"}, []any{1.0})
}

// Ported from Django: tests/forms_tests/field_tests/test_typedmultiplechoicefield.py::TypedMultipleChoiceFieldTest.test_typedmultiplechoicefield_3
func TestTypedmultiplechoicefield3(t *testing.T) {
	// This can also cause weirdness: be careful (bool(-1) == True,
	// remember)
	f := forms.NewTypedMultipleChoiceField(plusMinusOne, forms.Coerce(coerceBool))
	cleans(t, f, []string{"-1"}, []any{true})
}

// Ported from Django: tests/forms_tests/field_tests/test_typedmultiplechoicefield.py::TypedMultipleChoiceFieldTest.test_typedmultiplechoicefield_4
func TestTypedmultiplechoicefield4(t *testing.T) {
	f := forms.NewTypedMultipleChoiceField(plusMinusOne, forms.Coerce(coerceInt))
	cleans(t, f, []string{"1", "-1"}, []any{1, -1})
	rejects(t, f, []string{"1", "2"}, "Select a valid choice. 2 is not one of the available choices.")
}

// Ported from Django: tests/forms_tests/field_tests/test_typedmultiplechoicefield.py::TypedMultipleChoiceFieldTest.test_typedmultiplechoicefield_5
func TestTypedmultiplechoicefield5(t *testing.T) {
	// Even more weirdness: if you have a valid choice but your coercion
	// function can't coerce, you'll still get a validation error. Don't do
	// this!
	f := forms.NewTypedMultipleChoiceField(forms.Choices("A", "A", "B", "B"), forms.Coerce(coerceInt))
	rejects(t, f, []string{"B"}, "Select a valid choice. B is not one of the available choices.")
	// Required fields require values
	rejects(t, f, []string{}, "This field is required.")
}

// Ported from Django: tests/forms_tests/field_tests/test_typedmultiplechoicefield.py::TypedMultipleChoiceFieldTest.test_typedmultiplechoicefield_6
func TestTypedmultiplechoicefield6(t *testing.T) {
	// Non-required fields aren't required
	f := forms.NewTypedMultipleChoiceField(plusMinusOne, forms.Coerce(coerceInt), forms.Required(false))
	cleans(t, f, []string{}, []any{})
}

// Ported from Django: tests/forms_tests/field_tests/test_typedmultiplechoicefield.py::TypedMultipleChoiceFieldTest.test_typedmultiplechoicefield_7
func TestTypedmultiplechoicefield7(t *testing.T) {
	// If you want cleaning an empty value to return a different type, tell
	// the field
	f := forms.NewTypedMultipleChoiceField(plusMinusOne, forms.Coerce(coerceInt),
		forms.Required(false), forms.EmptyValue(nil))
	cleans(t, f, []string{}, nil)
}

// Ported from Django: tests/forms_tests/field_tests/test_typedmultiplechoicefield.py::TypedMultipleChoiceFieldTest.test_typedmultiplechoicefield_has_changed
func TestTypedmultiplechoicefieldHasChanged(t *testing.T) {
	// has_changed should not trigger required validation
	f := forms.NewTypedMultipleChoiceField(plusMinusOne, forms.Coerce(coerceInt), forms.Required(true))
	equal(t, f.HasChanged(nil, ""), false)
}

// Ported from Django: tests/forms_tests/field_tests/test_typedmultiplechoicefield.py::TypedMultipleChoiceFieldTest.test_typedmultiplechoicefield_special_coerce
func TestTypedmultiplechoicefieldSpecialCoerce(t *testing.T) {
	// A coerce function which results in a value not present in choices
	// should raise an appropriate error (#21397).
	coerceFunc := func(val string) (any, error) { return forms.ParseDecimal("1." + val) }
	f := forms.NewTypedMultipleChoiceField(forms.Choices("1", "1", "2", "2"),
		forms.Coerce(coerceFunc), forms.Required(true))
	cleans(t, f, []string{"2"}, []any{forms.MustDecimal("1.2")})
	rejects(t, f, []string{}, "This field is required.")
	rejects(t, f, []string{"3"}, "Select a valid choice. 3 is not one of the available choices.")
}
