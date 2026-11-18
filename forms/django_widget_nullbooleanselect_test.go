package forms_test

import (
	"testing"

	"github.com/oliverhaas/djangogo/forms"
)

// widgetNullBooleanHTML is the select of a NullBooleanSelect with the option
// selected, the expected HTML of its render tests.
func widgetNullBooleanHTML(selected string) string {
	out := `<select name="is_cool">`
	for _, o := range [][2]string{{"unknown", "Unknown"}, {"true", "Yes"}, {"false", "No"}} {
		mark := ""
		if o[0] == selected {
			mark = " selected"
		}
		out += `<option value="` + o[0] + `"` + mark + ">" + o[1] + "</option>"
	}
	return out + "</select>"
}

// widgetNullBooleanCases are the value to selected-option pairs of the render
// tests of NullBooleanSelectTest, keyed by their Django test names.
var widgetNullBooleanCases = []struct {
	test     string
	value    any
	selected string
}{
	{"test_render_true", true, "true"},
	{"test_render_false", false, "false"},
	{"test_render_none", nil, "unknown"},
	{"test_render_value_unknown", "unknown", "unknown"},
	{"test_render_value_true", "true", "true"},
	{"test_render_value_false", "false", "false"},
	{"test_render_value_1", "1", "unknown"},
	{"test_render_value_2", "2", "true"},
	{"test_render_value_3", "3", "false"},
}

func widgetCheckNullBoolean(t *testing.T, test string) {
	t.Helper()
	for _, c := range widgetNullBooleanCases {
		if c.test == test {
			widgetCheckHTML(t, forms.NullBooleanSelect{}, "is_cool", c.value, nil, widgetNullBooleanHTML(c.selected))
			return
		}
	}
	t.Fatalf("unknown case %s", test)
}

// Ported from Django: tests/forms_tests/widget_tests/test_nullbooleanselect.py::NullBooleanSelectTest.test_render_true
func TestNullBooleanSelectRenderTrue(t *testing.T) { widgetCheckNullBoolean(t, "test_render_true") }

// Ported from Django: tests/forms_tests/widget_tests/test_nullbooleanselect.py::NullBooleanSelectTest.test_render_false
func TestNullBooleanSelectRenderFalse(t *testing.T) { widgetCheckNullBoolean(t, "test_render_false") }

// Ported from Django: tests/forms_tests/widget_tests/test_nullbooleanselect.py::NullBooleanSelectTest.test_render_none
func TestNullBooleanSelectRenderNone(t *testing.T) { widgetCheckNullBoolean(t, "test_render_none") }

// Ported from Django: tests/forms_tests/widget_tests/test_nullbooleanselect.py::NullBooleanSelectTest.test_render_value_unknown
func TestNullBooleanSelectRenderValueUnknown(t *testing.T) {
	widgetCheckNullBoolean(t, "test_render_value_unknown")
}

// Ported from Django: tests/forms_tests/widget_tests/test_nullbooleanselect.py::NullBooleanSelectTest.test_render_value_true
func TestNullBooleanSelectRenderValueTrue(t *testing.T) {
	widgetCheckNullBoolean(t, "test_render_value_true")
}

// Ported from Django: tests/forms_tests/widget_tests/test_nullbooleanselect.py::NullBooleanSelectTest.test_render_value_false
func TestNullBooleanSelectRenderValueFalse(t *testing.T) {
	widgetCheckNullBoolean(t, "test_render_value_false")
}

// Ported from Django: tests/forms_tests/widget_tests/test_nullbooleanselect.py::NullBooleanSelectTest.test_render_value_1
func TestNullBooleanSelectRenderValue1(t *testing.T) {
	widgetCheckNullBoolean(t, "test_render_value_1")
}

// Ported from Django: tests/forms_tests/widget_tests/test_nullbooleanselect.py::NullBooleanSelectTest.test_render_value_2
func TestNullBooleanSelectRenderValue2(t *testing.T) {
	widgetCheckNullBoolean(t, "test_render_value_2")
}

// Ported from Django: tests/forms_tests/widget_tests/test_nullbooleanselect.py::NullBooleanSelectTest.test_render_value_3
func TestNullBooleanSelectRenderValue3(t *testing.T) {
	widgetCheckNullBoolean(t, "test_render_value_3")
}

// The Django template wrapping the field (label and div) is not ported; the
// widget HTML is what the form renders.
// Ported from Django: tests/forms_tests/widget_tests/test_nullbooleanselect.py::NullBooleanSelectTest.test_fieldset
func TestNullBooleanSelectFieldset(t *testing.T) {
	w := forms.NullBooleanSelect{}
	equal(t, w.UseFieldset(), false)
	assertHTMLEqual(t, widgetFieldHTML(forms.NewNullBooleanField(forms.UseWidget(w))),
		`<select name="field" id="id_field">
		<option value="unknown" selected>Unknown</option>
		<option value="true">Yes</option>
		<option value="false">No</option></select>`)
}
