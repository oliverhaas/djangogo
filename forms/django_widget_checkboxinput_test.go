package forms_test

import (
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/forms"
)

// Ported from Django: tests/forms_tests/widget_tests/test_checkboxinput.py::CheckboxInputTest.test_render_empty
func TestCheckboxInputRenderEmpty(t *testing.T) {
	widgetCheckHTML(t, forms.CheckboxInput{}, "is_cool", "", nil, `<input type="checkbox" name="is_cool">`)
}

// Ported from Django: tests/forms_tests/widget_tests/test_checkboxinput.py::CheckboxInputTest.test_render_none
func TestCheckboxInputRenderNone(t *testing.T) {
	widgetCheckHTML(t, forms.CheckboxInput{}, "is_cool", nil, nil, `<input type="checkbox" name="is_cool">`)
}

// Ported from Django: tests/forms_tests/widget_tests/test_checkboxinput.py::CheckboxInputTest.test_render_false
func TestCheckboxInputRenderFalse(t *testing.T) {
	widgetCheckHTML(t, forms.CheckboxInput{}, "is_cool", false, nil, `<input type="checkbox" name="is_cool">`)
}

// Ported from Django: tests/forms_tests/widget_tests/test_checkboxinput.py::CheckboxInputTest.test_render_true
func TestCheckboxInputRenderTrue(t *testing.T) {
	widgetCheckHTML(t, forms.CheckboxInput{}, "is_cool", true, nil, `<input checked type="checkbox" name="is_cool">`)
}

// Ported from Django: tests/forms_tests/widget_tests/test_checkboxinput.py::CheckboxInputTest.test_render_value
func TestCheckboxInputRenderValue(t *testing.T) {
	// Any value that is not in ('', None, False, True) checks the box and sets
	// the value attribute.
	widgetCheckHTML(t, forms.CheckboxInput{}, "is_cool", "foo", nil,
		`<input checked type="checkbox" name="is_cool" value="foo">`)
}

// Ported from Django: tests/forms_tests/widget_tests/test_checkboxinput.py::CheckboxInputTest.test_render_int
func TestCheckboxInputRenderInt(t *testing.T) {
	// Integers are handled by value, not as booleans (#17114).
	widgetCheckHTML(t, forms.CheckboxInput{}, "is_cool", 0, nil, `<input checked type="checkbox" name="is_cool" value="0">`)
	widgetCheckHTML(t, forms.CheckboxInput{}, "is_cool", 1, nil, `<input checked type="checkbox" name="is_cool" value="1">`)
}

// Ported from Django: tests/forms_tests/widget_tests/test_checkboxinput.py::CheckboxInputTest.test_render_check_test
func TestCheckboxInputRenderCheckTest(t *testing.T) {
	w := forms.CheckboxInput{CheckTest: func(v any) bool { return strings.HasPrefix(v.(string), "hello") }}
	widgetCheckHTML(t, w, "greeting", "", nil, `<input type="checkbox" name="greeting">`)
	widgetCheckHTML(t, w, "greeting", "hello", nil, `<input checked type="checkbox" name="greeting" value="hello">`)
	widgetCheckHTML(t, w, "greeting", "hello there", nil,
		`<input checked type="checkbox" name="greeting" value="hello there">`)
	widgetCheckHTML(t, w, "greeting", "hello & goodbye", nil,
		`<input checked type="checkbox" name="greeting" value="hello &amp; goodbye">`)
}

// Ported from Django: tests/forms_tests/widget_tests/test_checkboxinput.py::CheckboxInputTest.test_render_check_exception
func TestCheckboxInputRenderCheckException(t *testing.T) {
	// A panic of the check test is not swallowed (#17888).
	w := forms.CheckboxInput{CheckTest: func(v any) bool { return strings.HasPrefix(v.(string), "hello") }}
	defer func() {
		if recover() == nil {
			t.Error("Render swallowed the panic of the check test")
		}
	}()
	w.Render("greeting", true, nil)
}

// Ported from Django: tests/forms_tests/widget_tests/test_checkboxinput.py::CheckboxInputTest.test_value_from_datadict
func TestCheckboxInputValueFromDatadict(t *testing.T) {
	// An unchecked box is not submitted, so a missing key is false.
	equal(t, forms.CheckboxInput{}.ValueFromData(vals(), "testing"), false)
}

// Ported from Django: tests/forms_tests/widget_tests/test_checkboxinput.py::CheckboxInputTest.test_value_from_datadict_string_int
func TestCheckboxInputValueFromDatadictStringInt(t *testing.T) {
	equal(t, forms.CheckboxInput{}.ValueFromData(vals("testing", "0"), "testing"), true)
}

// Ported from Django: tests/forms_tests/widget_tests/test_checkboxinput.py::CheckboxInputTest.test_value_omitted_from_data
func TestCheckboxInputValueOmittedFromData(t *testing.T) {
	w := forms.CheckboxInput{}
	equal(t, w.ValueOmittedFromData(vals("field", "value"), "field"), false)
	equal(t, w.ValueOmittedFromData(vals(), "field"), false)
}

// Ported from Django: tests/forms_tests/widget_tests/test_checkboxinput.py::CheckboxInputTest.test_fieldset
func TestCheckboxInputFieldset(t *testing.T) {
	w := forms.CheckboxInput{}
	equal(t, w.UseFieldset(), false)
	assertHTMLEqual(t, widgetFieldHTML(forms.NewBooleanField(forms.UseWidget(w))),
		`<input id="id_field" name="field" required type="checkbox">`)
}
