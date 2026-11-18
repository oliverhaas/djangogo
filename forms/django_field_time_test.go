package forms_test

import (
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/forms"
)

// clock returns a time of day as a time.Time on January 1 of year 0 in UTC,
// the cleaned value of a TimeField, where Django returns a datetime.time.
func clock(hour, minute, sec, nsec int) time.Time {
	return time.Date(0, 1, 1, hour, minute, sec, nsec, time.UTC)
}

// Ported from Django: tests/forms_tests/field_tests/test_timefield.py::TimeFieldTest.test_timefield_1
func TestTimefield1(t *testing.T) {
	f := forms.NewTimeField()
	cleans(t, f, clock(14, 25, 0, 0), clock(14, 25, 0, 0))
	cleans(t, f, clock(14, 25, 59, 0), clock(14, 25, 59, 0))
	cleans(t, f, "14:25", clock(14, 25, 0, 0))
	cleans(t, f, "14:25:59", clock(14, 25, 59, 0))
	rejects(t, f, "hello", "Enter a valid time.")
	rejects(t, f, "1:24 p.m.", "Enter a valid time.")
}

// Ported from Django: tests/forms_tests/field_tests/test_timefield.py::TimeFieldTest.test_timefield_2
func TestTimefield2(t *testing.T) {
	f := forms.NewTimeField(forms.InputFormats("%I:%M %p"))
	cleans(t, f, clock(14, 25, 0, 0), clock(14, 25, 0, 0))
	cleans(t, f, clock(14, 25, 59, 0), clock(14, 25, 59, 0))
	cleans(t, f, "4:25 AM", clock(4, 25, 0, 0))
	cleans(t, f, "4:25 PM", clock(16, 25, 0, 0))
	rejects(t, f, "14:30:45", "Enter a valid time.")
}

// Ported from Django: tests/forms_tests/field_tests/test_timefield.py::TimeFieldTest.test_timefield_3
func TestTimefield3(t *testing.T) {
	f := forms.NewTimeField()
	// Test whitespace stripping behavior (#5714)
	cleans(t, f, " 14:25 ", clock(14, 25, 0, 0))
	cleans(t, f, " 14:25:59 ", clock(14, 25, 59, 0))
	rejects(t, f, "   ", "Enter a valid time.")
}

// Ported from Django: tests/forms_tests/field_tests/test_timefield.py::TimeFieldTest.test_timefield_changed
func TestTimefieldChanged(t *testing.T) {
	t1 := clock(12, 51, 34, 482548000)
	t2 := clock(12, 51, 0, 0)
	f := forms.NewTimeField(forms.InputFormats("%H:%M", "%H:%M %p"))
	equal(t, f.HasChanged(t1, "12:51"), true)
	equal(t, f.HasChanged(t2, "12:51"), false)
	equal(t, f.HasChanged(t2, "12:51 PM"), false)
}
