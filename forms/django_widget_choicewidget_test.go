package forms_test

import (
	"testing"

	"github.com/oliverhaas/djangogo/forms"
)

// widgetEmptyChoice is the choices [("", "select please"), ("P", "Paul")] of
// the required-attribute tests; Go has one spelling of the empty value.
var widgetEmptyChoice = []forms.Choice{{Value: "", Label: "select please"}, {Value: "P", Label: "Paul"}}

// Ported from Django: tests/forms_tests/widget_tests/test_choicewidget.py::ChoiceWidgetTest.test_renders_required_when_possible_to_select_empty_field_str
func TestChoiceWidgetRendersRequiredWhenPossibleToSelectEmptyFieldStr(t *testing.T) {
	equal(t, forms.Select{Choices: widgetEmptyChoice}.UseRequiredAttribute(nil), true)
	equal(t, forms.RadioSelect{Choices: widgetEmptyChoice}.UseRequiredAttribute(nil), true)
}
