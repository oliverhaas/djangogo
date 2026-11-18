package forms_test

import (
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/forms"
)

// Ported from Django: tests/forms_tests/field_tests/test_durationfield.py::DurationFieldTest.test_durationfield_clean
func TestDurationfieldClean(t *testing.T) {
	f := forms.NewDurationField()
	cleans(t, f, "30", 30*time.Second)
	cleans(t, f, "15:30", 15*time.Minute+30*time.Second)
	cleans(t, f, "1:15:30", time.Hour+15*time.Minute+30*time.Second)
	cleans(t, f, "1 1:15:30.3",
		24*time.Hour+time.Hour+15*time.Minute+30*time.Second+300*time.Millisecond)
	cleans(t, f, 10800*time.Second, 10800*time.Second)
	rejects(t, f, "", "This field is required.")
	rejects(t, f, "not_a_time", "Enter a valid duration.")
	rejects(t, forms.NewDurationField(), "P3(3D", "Enter a valid duration.")
}

// Ported from Django: tests/forms_tests/field_tests/test_durationfield.py::DurationFieldTest.test_durationfield_clean_not_required
func TestDurationfieldCleanNotRequired(t *testing.T) {
	f := forms.NewDurationField(forms.Required(false))
	cleans(t, f, "", nil)
}

// Ported from Django: tests/forms_tests/field_tests/test_durationfield.py::DurationFieldTest.test_overflow
func TestDurationfieldOverflow(t *testing.T) {
	// Django names the limits of timedelta (999999999 days); the limits here are
	// those of time.Duration, about 292 years.
	msg := "The number of days must be between -106751 and 106751."
	f := forms.NewDurationField()
	rejects(t, f, "1000000000 00:00:00", msg)
	rejects(t, f, "-1000000000 00:00:00", msg)
	rejects(t, f, "106752 00:00:00", msg)
}

// Ported from Django: tests/forms_tests/field_tests/test_durationfield.py::DurationFieldTest.test_durationfield_render
func TestDurationfieldRender(t *testing.T) {
	rendersTo(t, forms.NewDurationField(forms.Initial(time.Hour)),
		`<input id="id_f" type="text" name="f" value="01:00:00" required>`)
}

// Ported from Django: tests/forms_tests/field_tests/test_durationfield.py::DurationFieldTest.test_durationfield_integer_value
func TestDurationfieldIntegerValue(t *testing.T) {
	// Django reads an integer as seconds; Go reads a time.Duration, so the
	// integer 10800 is given as that many seconds.
	f := forms.NewDurationField()
	cleans(t, f, 10800*time.Second, 10800*time.Second)
}

// Ported from Django: tests/forms_tests/field_tests/test_durationfield.py::DurationFieldTest.test_durationfield_prepare_value
func TestDurationfieldPrepareValue(t *testing.T) {
	field := forms.NewDurationField()
	td := 15*time.Minute + 30*time.Second
	// duration_string(td) is "00:15:30".
	equal(t, field.PrepareValue(td), "00:15:30")
	equal(t, field.PrepareValue("arbitrary"), "arbitrary")
	equal(t, field.PrepareValue(nil), nil)
}
