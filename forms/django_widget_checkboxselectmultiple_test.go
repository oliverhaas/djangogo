package forms_test

import (
	"testing"

	"github.com/oliverhaas/djangogo/forms"
)

// widgetBeatlesChecks is the expected checkbox group of the beatles with the
// selected values checked and an optional leading empty choice.
func widgetBeatlesChecks(blank bool, selected ...string) string {
	out := "<div>"
	row := func(value, label string) {
		mark := ""
		for _, s := range selected {
			if s == value {
				mark = " checked"
			}
		}
		out += `<div><label><input` + mark + ` type="checkbox" name="beatles" value="` + value + `"> ` + label + "</label></div>"
	}
	if blank {
		row("", "Unknown")
	}
	for _, c := range widgetBeatles {
		row(c.Value, c.Label)
	}
	return out + "</div>"
}

// Ported from Django: tests/forms_tests/widget_tests/test_checkboxselectmultiple.py::CheckboxSelectMultipleTest.test_render_value
func TestCheckboxSelectMultipleRenderValue(t *testing.T) {
	widgetCheckHTML(t, forms.CheckboxSelectMultiple{Choices: widgetBeatles}, "beatles", []string{"J"}, nil,
		widgetBeatlesChecks(false, "J"))
}

// Ported from Django: tests/forms_tests/widget_tests/test_checkboxselectmultiple.py::CheckboxSelectMultipleTest.test_render_value_multiple
func TestCheckboxSelectMultipleRenderValueMultiple(t *testing.T) {
	widgetCheckHTML(t, forms.CheckboxSelectMultiple{Choices: widgetBeatles}, "beatles", []string{"J", "P"}, nil,
		widgetBeatlesChecks(false, "J", "P"))
}

// Ported from Django: tests/forms_tests/widget_tests/test_checkboxselectmultiple.py::CheckboxSelectMultipleTest.test_render_none
func TestCheckboxSelectMultipleRenderNone(t *testing.T) {
	// With a None value no option is selected, even the empty one.
	choices := append([]forms.Choice{{Value: "", Label: "Unknown"}}, widgetBeatles...)
	widgetCheckHTML(t, forms.CheckboxSelectMultiple{Choices: choices}, "beatles", nil, nil, widgetBeatlesChecks(true))
}

// widgetLettersChecks is the expected checkbox group of the letters a, b and c
// with a and c checked, under the id abc.
const widgetLettersChecks = `
        <div id="abc">
        <div>
        <label for="abc_0">
        <input checked type="checkbox" name="letters" value="a" id="abc_0"> A</label>
        </div>
        <div><label for="abc_1">
        <input type="checkbox" name="letters" value="b" id="abc_1"> B</label></div>
        <div>
        <label for="abc_2">
        <input checked type="checkbox" name="letters" value="c" id="abc_2"> C</label>
        </div>
        </div>`

// Ported from Django: tests/forms_tests/widget_tests/test_checkboxselectmultiple.py::CheckboxSelectMultipleTest.test_separate_ids
func TestCheckboxSelectMultipleSeparateIDs(t *testing.T) {
	// Each input gets a separate ID.
	w := forms.CheckboxSelectMultiple{Choices: []forms.Choice{{Value: "a", Label: "A"}, {Value: "b", Label: "B"}, {Value: "c", Label: "C"}}}
	widgetCheckHTML(t, w, "letters", []string{"a", "c"}, forms.Attrs{"id": "abc"}, widgetLettersChecks)
}

// Ported from Django: tests/forms_tests/widget_tests/test_checkboxselectmultiple.py::CheckboxSelectMultipleTest.test_separate_ids_constructor
func TestCheckboxSelectMultipleSeparateIDsConstructor(t *testing.T) {
	// Each input gets a separate ID when the ID is passed to the constructor.
	w := forms.CheckboxSelectMultiple{
		Attrs:   forms.Attrs{"id": "abc"},
		Choices: []forms.Choice{{Value: "a", Label: "A"}, {Value: "b", Label: "B"}, {Value: "c", Label: "C"}},
	}
	widgetCheckHTML(t, w, "letters", []string{"a", "c"}, nil, widgetLettersChecks)
}

// Ported from Django: tests/forms_tests/widget_tests/test_checkboxselectmultiple.py::CheckboxSelectMultipleTest.test_use_required_attribute
func TestCheckboxSelectMultipleUseRequiredAttribute(t *testing.T) {
	w := forms.CheckboxSelectMultiple{Choices: widgetBeatles}
	// Always false because browser validation would require all checkboxes to
	// be checked instead of at least one.
	equal(t, w.UseRequiredAttribute(nil), false)
	equal(t, w.UseRequiredAttribute([]string{}), false)
	equal(t, w.UseRequiredAttribute([]string{"J", "P"}), false)
}

// Ported from Django: tests/forms_tests/widget_tests/test_checkboxselectmultiple.py::CheckboxSelectMultipleTest.test_value_omitted_from_data
func TestCheckboxSelectMultipleValueOmittedFromData(t *testing.T) {
	w := forms.CheckboxSelectMultiple{Choices: widgetBeatles}
	equal(t, w.ValueOmittedFromData(vals(), "field"), false)
	equal(t, w.ValueOmittedFromData(vals("field", "value"), "field"), false)
}

// Ported from Django: tests/forms_tests/widget_tests/test_checkboxselectmultiple.py::CheckboxSelectMultipleTest.test_label
func TestCheckboxSelectMultipleLabel(t *testing.T) {
	// The label has no for attribute: clicking it would toggle the first checkbox.
	field := forms.NewMultipleChoiceField(forms.UseWidget(forms.CheckboxSelectMultiple{}))
	boundField := forms.New(forms.F("f", field)).Field("f")
	equal(t, field.Widget.IDForLabel("id"), "")
	equal(t, boundField.LabelTag(), "<label>F:</label>")
	equal(t, boundField.LegendTag(), "<legend>F:</legend>")
}

// The Django template wrapping the field (label and fieldset) is not ported;
// the widget HTML is what the form renders.
// Ported from Django: tests/forms_tests/widget_tests/test_checkboxselectmultiple.py::CheckboxSelectMultipleTest.test_fieldset
func TestCheckboxSelectMultipleFieldset(t *testing.T) {
	w := forms.CheckboxSelectMultiple{}
	equal(t, w.UseFieldset(), true)
	field := forms.NewChoiceField(forms.UseWidget(w), forms.Choices("J", "John", "P", "Paul", "G", "George", "R", "Ringo"))
	assertHTMLEqual(t, widgetFieldHTML(field),
		`<div id="id_field">
		<div><label for="id_field_0"><input type="checkbox" name="field" value="J" id="id_field_0"> John</label></div>
		<div><label for="id_field_1"><input type="checkbox" name="field" value="P" id="id_field_1">Paul</label></div>
		<div><label for="id_field_2"><input type="checkbox" name="field" value="G" id="id_field_2"> George</label></div>
		<div><label for="id_field_3"><input type="checkbox" name="field" value="R" id="id_field_3">Ringo</label></div></div>`)
}
