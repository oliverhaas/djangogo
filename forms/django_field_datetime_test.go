package forms_test

import (
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/forms"
)

// datetime returns a time.Time in UTC, Django's naive datetime(...) with the
// microseconds given as nanoseconds.
func datetime(year int, month time.Month, day, hour, minute, sec, nsec int) time.Time {
	return time.Date(year, month, day, hour, minute, sec, nsec, time.UTC)
}

// Ported from Django: tests/forms_tests/field_tests/test_datetimefield.py::DateTimeFieldTest.test_datetimefield_clean
func TestDatetimefieldClean(t *testing.T) {
	tests := []struct {
		value any
		want  time.Time
	}{
		{date(2006, 10, 25), datetime(2006, 10, 25, 0, 0, 0, 0)},
		{datetime(2006, 10, 25, 14, 30, 0, 0), datetime(2006, 10, 25, 14, 30, 0, 0)},
		{datetime(2006, 10, 25, 14, 30, 59, 0), datetime(2006, 10, 25, 14, 30, 59, 0)},
		{datetime(2006, 10, 25, 14, 30, 59, 200000), datetime(2006, 10, 25, 14, 30, 59, 200000)},
		{"2006-10-25 14:30:45.000200", datetime(2006, 10, 25, 14, 30, 45, 200000)},
		{"2006-10-25 14:30:45.0002", datetime(2006, 10, 25, 14, 30, 45, 200000)},
		{"2006-10-25 14:30:45", datetime(2006, 10, 25, 14, 30, 45, 0)},
		{"2006-10-25 14:30:00", datetime(2006, 10, 25, 14, 30, 0, 0)},
		{"2006-10-25 14:30", datetime(2006, 10, 25, 14, 30, 0, 0)},
		{"2006-10-25", datetime(2006, 10, 25, 0, 0, 0, 0)},
		{"10/25/2006 14:30:45.000200", datetime(2006, 10, 25, 14, 30, 45, 200000)},
		{"10/25/2006 14:30:45", datetime(2006, 10, 25, 14, 30, 45, 0)},
		{"10/25/2006 14:30:00", datetime(2006, 10, 25, 14, 30, 0, 0)},
		{"10/25/2006 14:30", datetime(2006, 10, 25, 14, 30, 0, 0)},
		{"10/25/2006", datetime(2006, 10, 25, 0, 0, 0, 0)},
		{"10/25/06 14:30:45.000200", datetime(2006, 10, 25, 14, 30, 45, 200000)},
		{"10/25/06 14:30:45", datetime(2006, 10, 25, 14, 30, 45, 0)},
		{"10/25/06 14:30:00", datetime(2006, 10, 25, 14, 30, 0, 0)},
		{"10/25/06 14:30", datetime(2006, 10, 25, 14, 30, 0, 0)},
		{"10/25/06", datetime(2006, 10, 25, 0, 0, 0, 0)},
		// ISO 8601 formats.
		{"2014-09-23T22:34:41.614804", datetime(2014, 9, 23, 22, 34, 41, 614804000)},
		{"2014-09-23T22:34:41", datetime(2014, 9, 23, 22, 34, 41, 0)},
		{"2014-09-23T22:34", datetime(2014, 9, 23, 22, 34, 0, 0)},
		{"2014-09-23", datetime(2014, 9, 23, 0, 0, 0, 0)},
		{"2014-09-23T22:34Z", datetime(2014, 9, 23, 22, 34, 0, 0)},
		{"2014-09-23T22:34+07:00", time.Date(2014, 9, 23, 22, 34, 0, 0, time.FixedZone("", 420*60))},
		// Whitespace stripping.
		{" 2006-10-25   14:30:45 ", datetime(2006, 10, 25, 14, 30, 45, 0)},
		{" 2006-10-25 ", datetime(2006, 10, 25, 0, 0, 0, 0)},
		{" 10/25/2006 14:30:45 ", datetime(2006, 10, 25, 14, 30, 45, 0)},
		{" 10/25/2006 14:30 ", datetime(2006, 10, 25, 14, 30, 0, 0)},
		{" 10/25/2006 ", datetime(2006, 10, 25, 0, 0, 0, 0)},
		{" 10/25/06 14:30:45 ", datetime(2006, 10, 25, 14, 30, 45, 0)},
		{" 10/25/06 ", datetime(2006, 10, 25, 0, 0, 0, 0)},
		{" 2014-09-23T22:34:41.614804 ", datetime(2014, 9, 23, 22, 34, 41, 614804000)},
		{" 2014-09-23T22:34Z ", datetime(2014, 9, 23, 22, 34, 0, 0)},
	}
	f := forms.NewDateTimeField()
	for _, tc := range tests {
		t.Run("", func(t *testing.T) {
			got, err := f.Clean(tc.value)
			if err != nil {
				t.Fatalf("Clean(%#v) failed: %v", tc.value, err)
			}
			if gt, ok := got.(time.Time); !ok || !gt.Equal(tc.want) {
				t.Errorf("Clean(%#v) = %v, want %v", tc.value, got, tc.want)
			}
		})
	}
}

// Ported from Django: tests/forms_tests/field_tests/test_datetimefield.py::DateTimeFieldTest.test_datetimefield_clean_invalid
func TestDatetimefieldCleanInvalid(t *testing.T) {
	f := forms.NewDateTimeField()
	msg := "Enter a valid date/time."
	rejects(t, f, "hello", msg)
	rejects(t, f, "2006-10-25 4:30 p.m.", msg)
	rejects(t, f, "   ", msg)
	rejects(t, f, "2014-09-23T28:23", msg)
	f = forms.NewDateTimeField(forms.InputFormats("%Y %m %d %I:%M %p"))
	rejects(t, f, "2006.10.25 14:30:45", msg)
}

// Ported from Django: tests/forms_tests/field_tests/test_datetimefield.py::DateTimeFieldTest.test_datetimefield_clean_input_formats
func TestDatetimefieldCleanInputFormats(t *testing.T) {
	f := forms.NewDateTimeField(forms.InputFormats("%Y %m %d %I:%M %p"))
	cleans(t, f, date(2006, 10, 25), datetime(2006, 10, 25, 0, 0, 0, 0))
	cleans(t, f, datetime(2006, 10, 25, 14, 30, 0, 0), datetime(2006, 10, 25, 14, 30, 0, 0))
	cleans(t, f, datetime(2006, 10, 25, 14, 30, 59, 0), datetime(2006, 10, 25, 14, 30, 59, 0))
	cleans(t, f, datetime(2006, 10, 25, 14, 30, 59, 200000), datetime(2006, 10, 25, 14, 30, 59, 200000))
	cleans(t, f, "2006 10 25 2:30 PM", datetime(2006, 10, 25, 14, 30, 0, 0))
	// ISO-like formats are always accepted.
	cleans(t, f, "2006-10-25 14:30:45", datetime(2006, 10, 25, 14, 30, 45, 0))

	f = forms.NewDateTimeField(forms.InputFormats("%Y.%m.%d %H:%M:%S.%f"))
	cleans(t, f, "2006.10.25 14:30:45.0002", datetime(2006, 10, 25, 14, 30, 45, 200000))
}

// Ported from Django: tests/forms_tests/field_tests/test_datetimefield.py::DateTimeFieldTest.test_datetimefield_not_required
func TestDatetimefieldNotRequired(t *testing.T) {
	f := forms.NewDateTimeField(forms.Required(false))
	cleans(t, f, nil, nil)
	cleans(t, f, "", nil)
}

// Ported from Django: tests/forms_tests/field_tests/test_datetimefield.py::DateTimeFieldTest.test_datetimefield_changed
func TestDatetimefieldChanged(t *testing.T) {
	f := forms.NewDateTimeField(forms.InputFormats("%Y %m %d %I:%M %p"))
	d := datetime(2006, 9, 17, 14, 30, 0, 0)
	equal(t, f.HasChanged(d, "2006 09 17 2:30 PM"), false)
}

// No Django test exists: Django's tests of to_current_timezone set a current
// time zone, which the Go package does not have; UTC is its current time
// zone. A database driver may return a time in the local zone.
func TestDatetimeShownInUTC(t *testing.T) {
	zoned := time.Date(1940, 7, 7, 14, 30, 0, 0, time.FixedZone("CEST", 2*60*60))
	form := forms.New(
		forms.F("at", forms.NewDateTimeField(forms.Initial(zoned))),
		forms.F("split", forms.NewSplitDateTimeField(forms.Initial(zoned))),
	)
	requireSubstring(t, form.Field("at").String(), `value="1940-07-07 12:30:00"`)
	requireSubstring(t, form.Field("split").String(), `value="12:30:00"`)
}
