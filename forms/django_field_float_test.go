package forms_test

import (
	"testing"

	"github.com/oliverhaas/djangogo/forms"
)

// Ported from Django: tests/forms_tests/field_tests/test_floatfield.py::FloatFieldTest.test_floatfield_1
func TestFloatfield1(t *testing.T) {
	f := forms.NewFloatField()
	rendersTo(t, f, `<input step="any" type="number" name="f" id="id_f" required>`)
	rejects(t, f, "", "This field is required.")
	rejects(t, f, nil, "This field is required.")
	cleans(t, f, "1", 1.0)
	cleans(t, f, "23", 23.0)
	cleans(t, f, "3.14", 3.1400000000000001)
	cleans(t, f, 3.14, 3.1400000000000001)
	cleans(t, f, 42, 42.0)
	rejects(t, f, "a", "Enter a number.")
	cleans(t, f, "1.0 ", 1.0)
	cleans(t, f, " 1.0", 1.0)
	cleans(t, f, " 1.0 ", 1.0)
	rejects(t, f, "1.0a", "Enter a number.")
	equal(t, f.MaxValue, nil)
	equal(t, f.MinValue, nil)
	rejects(t, f, "Infinity", "Enter a number.")
	rejects(t, f, "NaN", "Enter a number.")
	rejects(t, f, "-Inf", "Enter a number.")
}

// Ported from Django: tests/forms_tests/field_tests/test_floatfield.py::FloatFieldTest.test_floatfield_2
func TestFloatfield2(t *testing.T) {
	f := forms.NewFloatField(forms.Required(false))
	cleans(t, f, "", nil)
	cleans(t, f, nil, nil)
	cleans(t, f, "1", 1.0)
	equal(t, f.MaxValue, nil)
	equal(t, f.MinValue, nil)
}

// Ported from Django: tests/forms_tests/field_tests/test_floatfield.py::FloatFieldTest.test_floatfield_3
func TestFloatfield3(t *testing.T) {
	f := forms.NewFloatField(forms.MaxValue(1.5), forms.MinValue(0.5))
	rendersTo(t, f, `<input step="any" name="f" min="0.5" max="1.5" type="number" id="id_f" required>`)
	rejects(t, f, "1.6", "Ensure this value is less than or equal to 1.5.")
	rejects(t, f, "0.4", "Ensure this value is greater than or equal to 0.5.")
	cleans(t, f, "1.5", 1.5)
	cleans(t, f, "0.5", 0.5)
	equal(t, f.MaxValue, 1.5)
	equal(t, f.MinValue, 0.5)
}

// Ported from Django: tests/forms_tests/field_tests/test_floatfield.py::FloatFieldTest.test_floatfield_4
func TestFloatfield4(t *testing.T) {
	f := forms.NewFloatField(forms.StepSize(0.02))
	rendersTo(t, f, `<input name="f" step="0.02" type="number" id="id_f" required>`)
	rejects(t, f, "0.01", "Ensure this value is a multiple of step size 0.02.")
	cleans(t, f, "2.34", 2.34)
	cleans(t, f, "2.1", 2.1)
	cleans(t, f, "-.5", -0.50)
	cleans(t, f, "-1.26", -1.26)
	equal(t, f.StepSize, 0.02)
}

// Ported from Django: tests/forms_tests/field_tests/test_floatfield.py::FloatFieldTest.test_floatfield_step_size_min_value
func TestFloatfieldStepSizeMinValue(t *testing.T) {
	f := forms.NewFloatField(forms.StepSize(0.02), forms.MinValue(0.01))
	rejects(t, f, "0.02",
		"Ensure this value is a multiple of step size 0.02, starting from 0.01, e.g. 0.01, 0.03, 0.05, and so on.")
	cleans(t, f, "2.33", 2.33)
	cleans(t, f, "0.11", 0.11)
	equal(t, f.StepSize, 0.02)
}

// Ported from Django: tests/forms_tests/field_tests/test_floatfield.py::FloatFieldTest.test_floatfield_widget_attrs
func TestFloatfieldWidgetAttrs(t *testing.T) {
	f := forms.NewFloatField(forms.UseWidget(forms.NumberInput{
		Attrs: forms.Attrs{"step": 0.01, "max": 1.0, "min": 0.0},
	}))
	rendersTo(t, f, `<input step="0.01" name="f" min="0.0" max="1.0" type="number" id="id_f" required>`)
}

// Ported from Django: tests/forms_tests/field_tests/test_floatfield.py::FloatFieldTest.test_floatfield_changed
func TestFloatfieldChanged(t *testing.T) {
	f := forms.NewFloatField()
	n := 4.35
	equal(t, f.HasChanged(n, "4.3500"), false)
}
