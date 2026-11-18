package forms_test

import (
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/forms"
)

// Cleaned dates are time.Time at midnight UTC in Go, where Django returns a
// datetime.date.

// Ported from Django: tests/forms_tests/field_tests/test_datefield.py::DateFieldTest.test_datefield_1
func TestDatefield1(t *testing.T) {
	f := forms.NewDateField()
	want := date(2006, 10, 25)
	cleans(t, f, date(2006, 10, 25), want)
	cleans(t, f, time.Date(2006, 10, 25, 14, 30, 0, 0, time.UTC), want)
	cleans(t, f, time.Date(2006, 10, 25, 14, 30, 59, 0, time.UTC), want)
	cleans(t, f, time.Date(2006, 10, 25, 14, 30, 59, 200000, time.UTC), want)
	cleans(t, f, "2006-10-25", want)
	cleans(t, f, "10/25/2006", want)
	cleans(t, f, "10/25/06", want)
	cleans(t, f, "Oct 25 2006", want)
	cleans(t, f, "October 25 2006", want)
	cleans(t, f, "October 25, 2006", want)
	cleans(t, f, "25 October 2006", want)
	cleans(t, f, "25 October, 2006", want)
	rejects(t, f, "2006-4-31", "Enter a valid date.")
	rejects(t, f, "200a-10-25", "Enter a valid date.")
	rejects(t, f, "25/10/06", "Enter a valid date.")
	rejects(t, f, "0-0-0", "Enter a valid date.")
	rejects(t, f, nil, "This field is required.")
}

// Ported from Django: tests/forms_tests/field_tests/test_datefield.py::DateFieldTest.test_datefield_2
func TestDatefield2(t *testing.T) {
	f := forms.NewDateField(forms.Required(false))
	cleans(t, f, nil, nil)
	cleans(t, f, "", nil)
}

// Ported from Django: tests/forms_tests/field_tests/test_datefield.py::DateFieldTest.test_datefield_3
func TestDatefield3(t *testing.T) {
	f := forms.NewDateField(forms.InputFormats("%Y %m %d"))
	want := date(2006, 10, 25)
	cleans(t, f, date(2006, 10, 25), want)
	cleans(t, f, time.Date(2006, 10, 25, 14, 30, 0, 0, time.UTC), want)
	cleans(t, f, "2006 10 25", want)
	rejects(t, f, "2006-10-25", "Enter a valid date.")
	rejects(t, f, "10/25/2006", "Enter a valid date.")
	rejects(t, f, "10/25/06", "Enter a valid date.")
}

// Ported from Django: tests/forms_tests/field_tests/test_datefield.py::DateFieldTest.test_datefield_4
func TestDatefield4(t *testing.T) {
	// Test whitespace stripping behavior (#5714)
	f := forms.NewDateField()
	want := date(2006, 10, 25)
	cleans(t, f, " 10/25/2006 ", want)
	cleans(t, f, " 10/25/06 ", want)
	cleans(t, f, " Oct 25   2006 ", want)
	cleans(t, f, " October  25 2006 ", want)
	cleans(t, f, " October 25, 2006 ", want)
	cleans(t, f, " 25 October 2006 ", want)
	rejects(t, f, "   ", "Enter a valid date.")
}

// Ported from Django: tests/forms_tests/field_tests/test_datefield.py::DateFieldTest.test_datefield_5
func TestDatefield5(t *testing.T) {
	// Test null bytes (#18982)
	f := forms.NewDateField()
	rejects(t, f, "a\x00b", "Enter a valid date.")
}

// Ported from Django: tests/forms_tests/field_tests/test_datefield.py::DateFieldTest.test_datefield_changed
func TestDatefieldChanged(t *testing.T) {
	f := forms.NewDateField(forms.InputFormats("%d/%m/%Y"))
	d := date(2007, 9, 17)
	equal(t, f.HasChanged(d, "17/09/2007"), false)
}
