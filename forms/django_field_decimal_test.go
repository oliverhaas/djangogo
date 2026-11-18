package forms_test

import (
	"testing"

	"github.com/oliverhaas/djangogo/forms"
)

// cleansDecimal asserts that the field cleans value to the decimal number
// want. Python's Decimal("0.50") == Decimal("0.5"), so the numbers compare by
// value, not by the digits they were written with.
func cleansDecimal(t *testing.T, f forms.Field, value any, want string) {
	t.Helper()
	got, err := f.Clean(value)
	if err != nil {
		t.Errorf("Clean(%#v) failed: %v", value, err)
		return
	}
	d, ok := got.(forms.Decimal)
	if !ok || d.Cmp(forms.MustDecimal(want)) != 0 {
		t.Errorf("Clean(%#v) = %#v, want Decimal(%q)", value, got, want)
	}
}

// Ported from Django: tests/forms_tests/field_tests/test_decimalfield.py::DecimalFieldTest.test_decimalfield_1
func TestDecimalfield1(t *testing.T) {
	f := forms.NewDecimalField(forms.MaxDigits(4), forms.DecimalPlaces(2))
	rendersTo(t, f, `<input id="id_f" step="0.01" type="number" name="f" required>`)
	rejects(t, f, "", "This field is required.")
	rejects(t, f, nil, "This field is required.")
	cleansDecimal(t, f, "1", "1")
	cleans(t, f, "1", forms.MustDecimal("1"))
	cleansDecimal(t, f, "23", "23")
	cleansDecimal(t, f, "3.14", "3.14")
	cleansDecimal(t, f, 3.14, "3.14")
	cleansDecimal(t, f, forms.MustDecimal("3.14"), "3.14")
	cleansDecimal(t, f, "1.0 ", "1.0")
	cleansDecimal(t, f, " 1.0", "1.0")
	cleansDecimal(t, f, " 1.0 ", "1.0")
	rejects(t, f, "123.45", "Ensure that there are no more than 4 digits in total.")
	rejects(t, f, "1.234", "Ensure that there are no more than 2 decimal places.")
	rejects(t, f, "123.4", "Ensure that there are no more than 2 digits before the decimal point.")
	cleansDecimal(t, f, "-12.34", "-12.34")
	rejects(t, f, "-123.45", "Ensure that there are no more than 4 digits in total.")
	cleansDecimal(t, f, "-.12", "-0.12")
	cleansDecimal(t, f, "-00.12", "-0.12")
	cleansDecimal(t, f, "-000.12", "-0.12")
	rejects(t, f, "-000.123", "Ensure that there are no more than 2 decimal places.")
	rejects(t, f, "-000.12345", "Ensure that there are no more than 4 digits in total.")
	equal(t, f.MaxDigits, 4)
	equal(t, f.DecimalPlaces, 2)
	equal(t, f.MaxValue, nil)
	equal(t, f.MinValue, nil)
}

// Ported from Django: tests/forms_tests/field_tests/test_decimalfield.py::DecimalFieldTest.test_enter_a_number_error
func TestEnterANumberError(t *testing.T) {
	f := forms.NewDecimalField(forms.MaxValue(1), forms.MaxDigits(4), forms.DecimalPlaces(2))
	values := []string{
		"-NaN", "NaN", "+NaN", "-sNaN", "sNaN", "+sNaN",
		"-Inf", "Inf", "+Inf", "-Infinity", "Infinity", "+Infinity",
		"a", "łąść", "1.0a", "--0.12",
	}
	for _, value := range values {
		t.Run(value, func(t *testing.T) {
			rejects(t, f, value, "Enter a number.")
		})
	}
}

// Ported from Django: tests/forms_tests/field_tests/test_decimalfield.py::DecimalFieldTest.test_decimalfield_2
func TestDecimalfield2(t *testing.T) {
	f := forms.NewDecimalField(forms.MaxDigits(4), forms.DecimalPlaces(2), forms.Required(false))
	cleans(t, f, "", nil)
	cleans(t, f, nil, nil)
	cleansDecimal(t, f, "1", "1")
	equal(t, f.MaxDigits, 4)
	equal(t, f.DecimalPlaces, 2)
	equal(t, f.MaxValue, nil)
	equal(t, f.MinValue, nil)
}

// Ported from Django: tests/forms_tests/field_tests/test_decimalfield.py::DecimalFieldTest.test_decimalfield_3
func TestDecimalfield3(t *testing.T) {
	f := forms.NewDecimalField(
		forms.MaxDigits(4), forms.DecimalPlaces(2),
		forms.MaxValue(forms.MustDecimal("1.5")), forms.MinValue(forms.MustDecimal("0.5")),
	)
	rendersTo(t, f, `<input step="0.01" name="f" min="0.5" max="1.5" type="number" id="id_f" required>`)
	rejects(t, f, "1.6", "Ensure this value is less than or equal to 1.5.")
	rejects(t, f, "0.4", "Ensure this value is greater than or equal to 0.5.")
	cleansDecimal(t, f, "1.5", "1.5")
	cleansDecimal(t, f, "0.5", "0.5")
	cleansDecimal(t, f, ".5", "0.5")
	cleansDecimal(t, f, "00.50", "0.50")
	equal(t, f.MaxDigits, 4)
	equal(t, f.DecimalPlaces, 2)
	equal(t, f.MaxValue, forms.MustDecimal("1.5"))
	equal(t, f.MinValue, forms.MustDecimal("0.5"))
}

// Ported from Django: tests/forms_tests/field_tests/test_decimalfield.py::DecimalFieldTest.test_decimalfield_4
func TestDecimalfield4(t *testing.T) {
	f := forms.NewDecimalField(forms.DecimalPlaces(2))
	rejects(t, f, "0.00000001", "Ensure that there are no more than 2 decimal places.")
}

// Ported from Django: tests/forms_tests/field_tests/test_decimalfield.py::DecimalFieldTest.test_decimalfield_5
func TestDecimalfield5(t *testing.T) {
	f := forms.NewDecimalField(forms.MaxDigits(3))
	// Leading whole zeros "collapse" to one digit.
	cleansDecimal(t, f, "0000000.10", "0.1")
	// But a leading 0 before the . doesn't count toward max_digits
	cleansDecimal(t, f, "0000000.100", "0.100")
	// Only leading whole zeros "collapse" to one digit.
	cleansDecimal(t, f, "000000.02", "0.02")
	rejects(t, f, "000000.0002", "Ensure that there are no more than 3 digits in total.")
	cleansDecimal(t, f, ".002", "0.002")
}

// Ported from Django: tests/forms_tests/field_tests/test_decimalfield.py::DecimalFieldTest.test_decimalfield_6
func TestDecimalfield6(t *testing.T) {
	f := forms.NewDecimalField(forms.MaxDigits(2), forms.DecimalPlaces(2))
	cleansDecimal(t, f, ".01", ".01")
	rejects(t, f, "1.1", "Ensure that there are no more than 0 digits before the decimal point.")
}

// Ported from Django: tests/forms_tests/field_tests/test_decimalfield.py::DecimalFieldTest.test_decimalfield_step_size_min_value
func TestDecimalfieldStepSizeMinValue(t *testing.T) {
	f := forms.NewDecimalField(
		forms.StepSize(forms.MustDecimal("0.3")),
		forms.MinValue(forms.MustDecimal("-0.4")),
	)
	rendersTo(t, f, `<input name="f" min="-0.4" step="0.3" type="number" id="id_f" required>`)
	rejects(t, f, "1",
		"Ensure this value is a multiple of step size 0.3, starting from -0.4, e.g. -0.4, -0.1, 0.2, and so on.")
	cleansDecimal(t, f, "0.2", "0.2")
	cleansDecimal(t, f, 2, "2")
	equal(t, f.StepSize, forms.MustDecimal("0.3"))
}

// Ported from Django: tests/forms_tests/field_tests/test_decimalfield.py::DecimalFieldTest.test_decimalfield_scientific
func TestDecimalfieldScientific(t *testing.T) {
	f := forms.NewDecimalField(forms.MaxDigits(4), forms.DecimalPlaces(2))
	rejects(t, f, "1E+2", "Ensure that there are no more than 2 digits before the decimal point.")
	cleansDecimal(t, f, "1E+1", "10")
	cleansDecimal(t, f, "1E-1", "0.1")
	cleansDecimal(t, f, "0.546e+2", "54.6")
}

// Ported from Django: tests/forms_tests/field_tests/test_decimalfield.py::DecimalFieldTest.test_decimalfield_widget_attrs
func TestDecimalfieldWidgetAttrs(t *testing.T) {
	f := forms.NewDecimalField(forms.MaxDigits(6), forms.DecimalPlaces(2))
	equal(t, f.WidgetAttrs(forms.TextInput{}), forms.Attrs{})
	equal(t, f.WidgetAttrs(forms.NumberInput{}), forms.Attrs{"step": "0.01"})
	f = forms.NewDecimalField(forms.MaxDigits(10), forms.DecimalPlaces(0))
	equal(t, f.WidgetAttrs(forms.NumberInput{}), forms.Attrs{"step": "1"})
	f = forms.NewDecimalField(forms.MaxDigits(19), forms.DecimalPlaces(19))
	equal(t, f.WidgetAttrs(forms.NumberInput{}), forms.Attrs{"step": "1e-19"})
	f = forms.NewDecimalField(forms.MaxDigits(20))
	equal(t, f.WidgetAttrs(forms.NumberInput{}), forms.Attrs{"step": "any"})
	f = forms.NewDecimalField(forms.MaxDigits(6),
		forms.UseWidget(forms.NumberInput{Attrs: forms.Attrs{"step": "0.01"}}))
	rendersTo(t, f, `<input step="0.01" name="f" type="number" id="id_f" required>`)
}

// Ported from Django: tests/forms_tests/field_tests/test_decimalfield.py::DecimalFieldTest.test_decimalfield_changed
func TestDecimalfieldChanged(t *testing.T) {
	f := forms.NewDecimalField(forms.MaxDigits(2), forms.DecimalPlaces(2))
	d := forms.MustDecimal("0.1")
	equal(t, f.HasChanged(d, "0.10"), false)
	equal(t, f.HasChanged(d, "0.101"), true)
}
