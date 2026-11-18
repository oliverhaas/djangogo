package forms_test

import (
	"testing"

	"github.com/oliverhaas/djangogo/forms"
)

// widgetNumericChoices is SelectMultipleTest.numeric_choices.
var widgetNumericChoices = []forms.Choice{{Value: "0", Label: "0"}, {Value: "1", Label: "1"}, {Value: "2", Label: "2"}, {Value: "3", Label: "3"}, {Value: "0", Label: "extra"}}

// widgetBeatlesSelect is the expected select of the beatles render tests, with
// the selected option values marked.
func widgetBeatlesSelect(prefix []forms.Choice, selected ...string) string {
	out := `<select multiple name="beatles">`
	for _, c := range append(append([]forms.Choice{}, prefix...), widgetBeatles...) {
		mark := ""
		for _, s := range selected {
			if s == c.Value {
				mark = " selected"
			}
		}
		out += `<option value="` + c.Value + `"` + mark + ">" + c.Label + "</option>"
	}
	return out + "</select>"
}

// Ported from Django: tests/forms_tests/widget_tests/test_selectmultiple.py::SelectMultipleTest.test_render_selected
func TestSelectMultipleRenderSelected(t *testing.T) {
	widgetCheckHTML(t, forms.SelectMultiple{Choices: widgetBeatles}, "beatles", []string{"J"}, nil,
		widgetBeatlesSelect(nil, "J"))
}

// Ported from Django: tests/forms_tests/widget_tests/test_selectmultiple.py::SelectMultipleTest.test_render_multiple_selected
func TestSelectMultipleRenderMultipleSelected(t *testing.T) {
	widgetCheckHTML(t, forms.SelectMultiple{Choices: widgetBeatles}, "beatles", []string{"J", "P"}, nil,
		widgetBeatlesSelect(nil, "J", "P"))
}

// Ported from Django: tests/forms_tests/widget_tests/test_selectmultiple.py::SelectMultipleTest.test_render_none
func TestSelectMultipleRenderNone(t *testing.T) {
	// With a None value no option is selected, even the empty one.
	unknown := []forms.Choice{{Value: "", Label: "Unknown"}}
	w := forms.SelectMultiple{Choices: append(append([]forms.Choice{}, unknown...), widgetBeatles...)}
	widgetCheckHTML(t, w, "beatles", nil, nil, widgetBeatlesSelect(unknown))
}

// Ported from Django: tests/forms_tests/widget_tests/test_selectmultiple.py::SelectMultipleTest.test_render_value_label
func TestSelectMultipleRenderValueLabel(t *testing.T) {
	// A value that matches a label but no option value selects nothing.
	widgetCheckHTML(t, forms.SelectMultiple{Choices: widgetBeatles}, "beatles", []string{"John"}, nil,
		widgetBeatlesSelect(nil))
}

// Ported from Django: tests/forms_tests/widget_tests/test_selectmultiple.py::SelectMultipleTest.test_multiple_options_same_value
func TestSelectMultipleMultipleOptionsSameValue(t *testing.T) {
	// Multiple options with the same value can be selected (#8103).
	widgetCheckHTML(t, forms.SelectMultiple{Choices: widgetNumericChoices}, "choices", []string{"0"}, nil,
		`<select multiple name="choices">
            <option value="0" selected>0</option>
            <option value="1">1</option>
            <option value="2">2</option>
            <option value="3">3</option>
            <option value="0" selected>extra</option>
            </select>`)
}

// Ported from Django: tests/forms_tests/widget_tests/test_selectmultiple.py::SelectMultipleTest.test_multiple_values_invalid
func TestSelectMultipleMultipleValuesInvalid(t *testing.T) {
	// The valid values are selected when some of the given ones are not valid.
	widgetCheckHTML(t, forms.SelectMultiple{Choices: widgetBeatles}, "beatles", []string{"J", "G", "foo"}, nil,
		widgetBeatlesSelect(nil, "J", "G"))
}

// Ported from Django: tests/forms_tests/widget_tests/test_selectmultiple.py::SelectMultipleTest.test_compare_string
func TestSelectMultipleCompareString(t *testing.T) {
	html := `<select multiple name="nums">
            <option value="1">1</option>
            <option value="2" selected>2</option>
            <option value="3">3</option>
            </select>`
	w := forms.SelectMultiple{Choices: widgetNumChoices}
	widgetCheckHTML(t, w, "nums", []any{2}, nil, html)
	widgetCheckHTML(t, w, "nums", []string{"2"}, nil, html)
}

// Ported from Django: tests/forms_tests/widget_tests/test_selectmultiple.py::SelectMultipleTest.test_value_omitted_from_data
func TestSelectMultipleValueOmittedFromData(t *testing.T) {
	w := forms.SelectMultiple{Choices: widgetBeatles}
	equal(t, w.ValueOmittedFromData(vals(), "field"), false)
	equal(t, w.ValueOmittedFromData(vals("field", "value"), "field"), false)
}

// The Django template wrapping the field (label and div) is not ported; the
// widget HTML is what the form renders.
// Ported from Django: tests/forms_tests/widget_tests/test_selectmultiple.py::SelectMultipleTest.test_fieldset
func TestSelectMultipleFieldset(t *testing.T) {
	w := forms.SelectMultiple{}
	equal(t, w.UseFieldset(), false)
	field := forms.NewChoiceField(forms.UseWidget(w), forms.Required(false), forms.Choices("J", "John", "P", "Paul", "G", "George", "R", "Ringo"))
	assertHTMLEqual(t, widgetFieldHTML(field),
		`<select multiple name="field" id="id_field">
		<option value="J">John</option>  <option value="P">Paul</option>
		<option value="G">George</option><option value="R">Ringo</option></select>`)
}
