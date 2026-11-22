package forms_test

import (
	"testing"

	"github.com/oliverhaas/djangogo/forms"
)

// Ported from Django: tests/forms_tests/field_tests/test_choicefield.py::ChoiceFieldTest.test_choicefield_1
func TestChoicefield1(t *testing.T) {
	f := forms.NewChoiceField(forms.Choices("1", "One", "2", "Two"))
	rejects(t, f, "", "This field is required.")
	rejects(t, f, nil, "This field is required.")
	cleans(t, f, 1, "1")
	cleans(t, f, "1", "1")
	rejects(t, f, "3", "Select a valid choice. 3 is not one of the available choices.")
}

// Ported from Django: tests/forms_tests/field_tests/test_choicefield.py::ChoiceFieldTest.test_choicefield_2
func TestChoicefield2(t *testing.T) {
	f := forms.NewChoiceField(forms.Choices("1", "One", "2", "Two"), forms.Required(false))
	cleans(t, f, "", "")
	cleans(t, f, nil, "")
	cleans(t, f, 1, "1")
	cleans(t, f, "1", "1")
	rejects(t, f, "3", "Select a valid choice. 3 is not one of the available choices.")
}

// Ported from Django: tests/forms_tests/field_tests/test_choicefield.py::ChoiceFieldTest.test_choicefield_3
func TestChoicefield3(t *testing.T) {
	f := forms.NewChoiceField(forms.Choices("J", "John", "P", "Paul"))
	cleans(t, f, "J", "J")
	rejects(t, f, "John", "Select a valid choice. John is not one of the available choices.")
}

// Ported from Django: tests/forms_tests/field_tests/test_choicefield.py::ChoiceFieldTest.test_choicefield_choices_default
func TestChoicefieldChoicesDefault(t *testing.T) {
	f := forms.NewChoiceField()
	equal(t, len(f.Choices), 0)
}

// Ported from Django: tests/forms_tests/field_tests/test_choicefield.py::ChoiceFieldTest.test_choicefield_mapping
func TestChoicefieldMapping(t *testing.T) {
	// A Go choice list is ordered, so the dict becomes a list of Choice.
	f := forms.NewChoiceField(forms.ChoiceList([]forms.Choice{{Value: "J", Label: "John"}, {Value: "P", Label: "Paul"}}))
	cleans(t, f, "J", "J")
}

// Ported from Django: tests/forms_tests/field_tests/test_choicefield.py::ChoiceFieldTest.test_choicefield_disabled
func TestChoicefieldDisabled(t *testing.T) {
	f := forms.NewChoiceField(forms.Choices("J", "John", "P", "Paul"), forms.Disabled())
	rendersTo(t, f,
		`<select id="id_f" name="f" disabled><option value="J">John</option>`+
			`<option value="P">Paul</option></select>`)
}

// choiceNumbersLetters is the choices of Django's test_choicefield_4 and
// test_multiplechoicefield_3: two groups and an option.
var choiceNumbersLetters = []forms.Choice{
	{Label: "Numbers", Group: []forms.Choice{{Value: "1", Label: "One"}, {Value: "2", Label: "Two"}}},
	{Label: "Letters", Group: []forms.Choice{{Value: "3", Label: "A"}, {Value: "4", Label: "B"}}},
	{Value: "5", Label: "Other"},
}

// Ported from Django: tests/forms_tests/field_tests/test_choicefield.py::ChoiceFieldTest.test_choicefield_4
func TestChoicefield4(t *testing.T) {
	f := forms.NewChoiceField(forms.ChoiceList(choiceNumbersLetters))
	cleans(t, f, 1, "1")
	cleans(t, f, "1", "1")
	cleans(t, f, 3, "3")
	cleans(t, f, "3", "3")
	cleans(t, f, 5, "5")
	cleans(t, f, "5", "5")
	rejects(t, f, "6", "Select a valid choice. 6 is not one of the available choices.")
}

// A Go choice list is ordered, so the dict becomes a list of Choice.
//
// Ported from Django: tests/forms_tests/field_tests/test_choicefield.py::ChoiceFieldTest.test_choicefield_grouped_mapping
// Ported from Django: tests/forms_tests/field_tests/test_choicefield.py::ChoiceFieldTest.test_choicefield_grouped_mapping_inner_dict
func TestChoicefieldGroupedMapping(t *testing.T) {
	f := forms.NewChoiceField(forms.ChoiceList(choiceNumbersLetters[:2]))
	for _, v := range []string{"1", "2", "3", "4"} {
		cleans(t, f, v, v)
	}
	rejects(t, f, "Numbers", "Select a valid choice. Numbers is not one of the available choices.")
}
