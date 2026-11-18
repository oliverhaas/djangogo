package templates_test

import (
	"testing"
	"time"
)

// TestTimesince ports Django's TimesinceTests.
//
// Ported from Django: tests/template_tests/filter_tests/test_timesince.py::TimesinceTests.test_timesince01
// Ported from Django: tests/template_tests/filter_tests/test_timesince.py::TimesinceTests.test_timesince02
// Ported from Django: tests/template_tests/filter_tests/test_timesince.py::TimesinceTests.test_timesince03
func TestTimesince(t *testing.T) {
	runRenderCases(t, []renderCase{
		{name: "test_timesince01", src: "{{ a|timesince }}", ctx: map[string]any{"a": time.Now().Add(-time.Minute - 10*time.Second)}, want: "1\u00a0minute"},
		{name: "test_timesince02", src: "{{ a|timesince }}", ctx: map[string]any{"a": time.Now().Add(-24*time.Hour - time.Minute)}, want: "1\u00a0day"},
		{name: "test_timesince03", src: "{{ a|timesince }}", ctx: map[string]any{"a": time.Now().Add(-time.Hour - 25*time.Minute - 10*time.Second)}, want: "1\u00a0hour, 25\u00a0minutes"},
	})
}

// TestTimesinceFunction ports Django's FunctionTests.
//
// Ported from Django: tests/template_tests/filter_tests/test_timesince.py::FunctionTests.test_since_now
// Ported from Django: tests/template_tests/filter_tests/test_timesince.py::FunctionTests.test_no_args
// Ported from Django: tests/template_tests/filter_tests/test_timesince.py::FunctionTests.test_explicit_date
func TestTimesinceFunction(t *testing.T) {
	runRenderCases(t, []renderCase{
		{name: "test_since_now", src: "{{ a|timesince }}", ctx: map[string]any{"a": time.Now().Add(-24 * time.Hour)}, want: "1\u00a0day"},
		{name: "test_no_args", src: "{{ a|timesince }}", ctx: map[string]any{"a": nil}, want: ""},
		{name: "test_explicit_date", src: "{{ a|timesince:b }}", ctx: map[string]any{"a": time.Date(2005, 12, 29, 0, 0, 0, 0, time.UTC), "b": time.Date(2005, 12, 30, 0, 0, 0, 0, time.UTC)}, want: "1\u00a0day"},
	})
}
