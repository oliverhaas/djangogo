package forms_test

import (
	"testing"

	"github.com/oliverhaas/djangogo/forms"
)

// Cleaned values of an IntegerField are int64 in Go where Django returns int.

// Ported from Django: tests/forms_tests/field_tests/test_integerfield.py::IntegerFieldTest.test_integerfield_1
func TestIntegerfield1(t *testing.T) {
	f := forms.NewIntegerField()
	rendersTo(t, f, `<input type="number" name="f" id="id_f" required>`)
	rejects(t, f, "", "This field is required.")
	rejects(t, f, nil, "This field is required.")
	cleans(t, f, "1", int64(1))
	cleans(t, f, "23", int64(23))
	rejects(t, f, "a", "Enter a whole number.")
	cleans(t, f, 42, int64(42))
	rejects(t, f, 3.14, "Enter a whole number.")
	cleans(t, f, "1 ", int64(1))
	cleans(t, f, " 1", int64(1))
	cleans(t, f, " 1 ", int64(1))
	rejects(t, f, "1a", "Enter a whole number.")
	equal(t, f.MaxValue, nil)
	equal(t, f.MinValue, nil)
}

// Ported from Django: tests/forms_tests/field_tests/test_integerfield.py::IntegerFieldTest.test_integerfield_2
func TestIntegerfield2(t *testing.T) {
	f := forms.NewIntegerField(forms.Required(false))
	cleans(t, f, "", nil)
	cleans(t, f, nil, nil)
	cleans(t, f, "1", int64(1))
	cleans(t, f, "23", int64(23))
	rejects(t, f, "a", "Enter a whole number.")
	cleans(t, f, "1 ", int64(1))
	cleans(t, f, " 1", int64(1))
	cleans(t, f, " 1 ", int64(1))
	rejects(t, f, "1a", "Enter a whole number.")
	equal(t, f.MaxValue, nil)
	equal(t, f.MinValue, nil)
}

// Ported from Django: tests/forms_tests/field_tests/test_integerfield.py::IntegerFieldTest.test_integerfield_3
func TestIntegerfield3(t *testing.T) {
	f := forms.NewIntegerField(forms.MaxValue(10))
	rendersTo(t, f, `<input max="10" type="number" name="f" id="id_f" required>`)
	rejects(t, f, nil, "This field is required.")
	cleans(t, f, 1, int64(1))
	cleans(t, f, 10, int64(10))
	rejects(t, f, 11, "Ensure this value is less than or equal to 10.")
	cleans(t, f, "10", int64(10))
	rejects(t, f, "11", "Ensure this value is less than or equal to 10.")
	equal(t, f.MaxValue, 10)
	equal(t, f.MinValue, nil)
}

// Ported from Django: tests/forms_tests/field_tests/test_integerfield.py::IntegerFieldTest.test_integerfield_4
func TestIntegerfield4(t *testing.T) {
	f := forms.NewIntegerField(forms.MinValue(10))
	rendersTo(t, f, `<input id="id_f" type="number" name="f" min="10" required>`)
	rejects(t, f, nil, "This field is required.")
	rejects(t, f, 1, "Ensure this value is greater than or equal to 10.")
	cleans(t, f, 10, int64(10))
	cleans(t, f, 11, int64(11))
	cleans(t, f, "10", int64(10))
	cleans(t, f, "11", int64(11))
	equal(t, f.MaxValue, nil)
	equal(t, f.MinValue, 10)
}

// Ported from Django: tests/forms_tests/field_tests/test_integerfield.py::IntegerFieldTest.test_integerfield_5
func TestIntegerfield5(t *testing.T) {
	f := forms.NewIntegerField(forms.MinValue(10), forms.MaxValue(20))
	rendersTo(t, f, `<input id="id_f" max="20" type="number" name="f" min="10" required>`)
	rejects(t, f, nil, "This field is required.")
	rejects(t, f, 1, "Ensure this value is greater than or equal to 10.")
	cleans(t, f, 10, int64(10))
	cleans(t, f, 11, int64(11))
	cleans(t, f, "10", int64(10))
	cleans(t, f, "11", int64(11))
	cleans(t, f, 20, int64(20))
	rejects(t, f, 21, "Ensure this value is less than or equal to 20.")
	equal(t, f.MaxValue, 20)
	equal(t, f.MinValue, 10)
}

// Ported from Django: tests/forms_tests/field_tests/test_integerfield.py::IntegerFieldTest.test_integerfield_6
func TestIntegerfield6(t *testing.T) {
	f := forms.NewIntegerField(forms.StepSize(3))
	rendersTo(t, f, `<input name="f" step="3" type="number" id="id_f" required>`)
	rejects(t, f, "10", "Ensure this value is a multiple of step size 3.")
	cleans(t, f, 12, int64(12))
	cleans(t, f, "12", int64(12))
	equal(t, f.StepSize, 3)
}

// Ported from Django: tests/forms_tests/field_tests/test_integerfield.py::IntegerFieldTest.test_integerfield_step_size_min_value
func TestIntegerfieldStepSizeMinValue(t *testing.T) {
	f := forms.NewIntegerField(forms.StepSize(3), forms.MinValue(-1))
	rendersTo(t, f, `<input name="f" min="-1" step="3" type="number" id="id_f" required>`)
	rejects(t, f, "9",
		"Ensure this value is a multiple of step size 3, starting from -1, e.g. -1, 2, 5, and so on.")
	cleans(t, f, "2", int64(2))
	cleans(t, f, "-1", int64(-1))
	equal(t, f.StepSize, 3)
}

// Ported from Django: tests/forms_tests/field_tests/test_integerfield.py::IntegerFieldTest.test_integerfield_float
func TestIntegerfieldFloat(t *testing.T) {
	f := forms.NewIntegerField()
	cleans(t, f, 1.0, int64(1))
	cleans(t, f, "1.0", int64(1))
	cleans(t, f, " 1.0 ", int64(1))
	cleans(t, f, "1.", int64(1))
	cleans(t, f, " 1. ", int64(1))
	rejects(t, f, "1.5", "Enter a whole number.")
	rejects(t, f, "…", "Enter a whole number.")
}

// Python's int is unbounded; the Go value is an int64, so the largest Django
// value, 9223372036854775808, is out of range.

// Ported from Django: tests/forms_tests/field_tests/test_integerfield.py::IntegerFieldTest.test_integerfield_big_num
func TestIntegerfieldBigNum(t *testing.T) {
	f := forms.NewIntegerField()
	cleans(t, f, "9223372036854775807", int64(9223372036854775807))
	rejects(t, f, "9223372036854775808", "Enter a whole number.")
}

// Ported from Django: tests/forms_tests/field_tests/test_integerfield.py::IntegerFieldTest.test_integerfield_unicode_number
func TestIntegerfieldUnicodeNumber(t *testing.T) {
	f := forms.NewIntegerField()
	cleans(t, f, "５０", int64(50))
}
