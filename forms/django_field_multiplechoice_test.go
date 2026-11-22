package forms_test

import (
	"testing"

	"github.com/oliverhaas/djangogo/forms"
)

// Python tuples and lists both become []any in Go.

// Ported from Django: tests/forms_tests/field_tests/test_multiplechoicefield.py::MultipleChoiceFieldTest.test_multiplechoicefield_1
func TestMultiplechoicefield1(t *testing.T) {
	f := forms.NewMultipleChoiceField(forms.Choices("1", "One", "2", "Two"))
	rejects(t, f, "", "This field is required.")
	rejects(t, f, nil, "This field is required.")
	cleans(t, f, []any{1}, []string{"1"})
	cleans(t, f, []any{"1"}, []string{"1"})
	cleans(t, f, []any{"1", "2"}, []string{"1", "2"})
	cleans(t, f, []any{1, "2"}, []string{"1", "2"})
	cleans(t, f, []any{1, "2"}, []string{"1", "2"})
	rejects(t, f, "hello", "Enter a list of values.")
	rejects(t, f, []any{}, "This field is required.")
	rejects(t, f, []any{}, "This field is required.")
	rejects(t, f, []any{"3"}, "Select a valid choice. 3 is not one of the available choices.")
}

// Ported from Django: tests/forms_tests/field_tests/test_multiplechoicefield.py::MultipleChoiceFieldTest.test_multiplechoicefield_2
func TestMultiplechoicefield2(t *testing.T) {
	f := forms.NewMultipleChoiceField(forms.Choices("1", "One", "2", "Two"), forms.Required(false))
	cleans(t, f, "", []string{})
	cleans(t, f, nil, []string{})
	cleans(t, f, []any{1}, []string{"1"})
	cleans(t, f, []any{"1"}, []string{"1"})
	cleans(t, f, []any{"1", "2"}, []string{"1", "2"})
	cleans(t, f, []any{1, "2"}, []string{"1", "2"})
	cleans(t, f, []any{1, "2"}, []string{"1", "2"})
	rejects(t, f, "hello", "Enter a list of values.")
	cleans(t, f, []any{}, []string{})
	cleans(t, f, []any{}, []string{})
	rejects(t, f, []any{"3"}, "Select a valid choice. 3 is not one of the available choices.")
}

// Ported from Django: tests/forms_tests/field_tests/test_multiplechoicefield.py::MultipleChoiceFieldTest.test_multiplechoicefield_changed
func TestMultiplechoicefieldChanged(t *testing.T) {
	f := forms.NewMultipleChoiceField(forms.Choices("1", "One", "2", "Two", "3", "Three"))
	equal(t, f.HasChanged(nil, nil), false)
	equal(t, f.HasChanged([]any{}, nil), false)
	equal(t, f.HasChanged(nil, []string{"1"}), true)
	equal(t, f.HasChanged([]any{1, 2}, []string{"1", "2"}), false)
	equal(t, f.HasChanged([]any{2, 1}, []string{"1", "2"}), false)
	equal(t, f.HasChanged([]any{1, 2}, []string{"1"}), true)
	equal(t, f.HasChanged([]any{1, 2}, []string{"1", "3"}), true)
}

// Ported from Django: tests/forms_tests/field_tests/test_multiplechoicefield.py::MultipleChoiceFieldTest.test_disabled_has_changed
func TestMultiplechoicefieldDisabledHasChanged(t *testing.T) {
	f := forms.NewMultipleChoiceField(forms.Choices("1", "One", "2", "Two"), forms.Disabled())
	equal(t, f.HasChanged("x", "y"), false)
}

// Ported from Django: tests/forms_tests/field_tests/test_multiplechoicefield.py::MultipleChoiceFieldTest.test_validate_duplicated_values
func TestValidateDuplicatedValues(t *testing.T) {
	f := forms.NewMultipleChoiceField(forms.Choices("1", "One", "2", "Two", "3", "Three", "4", "Four", "5", "Five"))
	equal(t, f.Validate([]string{"4", "4", "5", "5"}), nil)
}

// Ported from Django: tests/forms_tests/field_tests/test_multiplechoicefield.py::MultipleChoiceFieldTest.test_validate_duplicated_invalid_values
func TestValidateDuplicatedInvalidValues(t *testing.T) {
	f := forms.NewMultipleChoiceField(forms.Choices("1", "one", "2", "Two"))
	err := f.Validate([]string{"invalid-one", "invalid-one", "invalid-two"})
	equal(t, forms.ValidationErrors(err).Messages(),
		[]string{"Select a valid choice. invalid-one is not one of the available choices."})
}

// Ported from Django: tests/forms_tests/field_tests/test_multiplechoicefield.py::MultipleChoiceFieldTest.test_multiplechoicefield_3
func TestMultiplechoicefield3(t *testing.T) {
	f := forms.NewMultipleChoiceField(forms.ChoiceList(choiceNumbersLetters))
	cleans(t, f, []any{1}, []string{"1"})
	cleans(t, f, []string{"1"}, []string{"1"})
	cleans(t, f, []any{1, 5}, []string{"1", "5"})
	cleans(t, f, []any{1, "5"}, []string{"1", "5"})
	cleans(t, f, []any{"1", 5}, []string{"1", "5"})
	cleans(t, f, []string{"1", "5"}, []string{"1", "5"})
	rejects(t, f, []string{"6"}, "Select a valid choice. 6 is not one of the available choices.")
	rejects(t, f, []string{"1", "6"}, "Select a valid choice. 6 is not one of the available choices.")
}
