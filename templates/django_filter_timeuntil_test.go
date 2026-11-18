package templates_test

import (
	"testing"
	"time"
)

// TestTimeuntil ports Django's TimeuntilTests.
//
// Ported from Django: tests/template_tests/filter_tests/test_timeuntil.py::TimeuntilTests.test_timeuntil01
// Ported from Django: tests/template_tests/filter_tests/test_timeuntil.py::TimeuntilTests.test_timeuntil02
// Ported from Django: tests/template_tests/filter_tests/test_timeuntil.py::TimeuntilTests.test_timeuntil03
func TestTimeuntil(t *testing.T) {
	runRenderCases(t, []renderCase{
		{name: "test_timeuntil01", src: "{{ a|timeuntil }}", ctx: map[string]any{"a": time.Now().Add(2*time.Minute + 10*time.Second)}, want: "2\u00a0minutes"},
		{name: "test_timeuntil02", src: "{{ a|timeuntil }}", ctx: map[string]any{"a": time.Now().Add(24*time.Hour + 10*time.Second)}, want: "1\u00a0day"},
		{name: "test_timeuntil03", src: "{{ a|timeuntil }}", ctx: map[string]any{"a": time.Now().Add(8*time.Hour + 10*time.Minute + 10*time.Second)}, want: "8\u00a0hours, 10\u00a0minutes"},
	})
}

// TestTimeuntilFunction ports Django's FunctionTests.
//
// Ported from Django: tests/template_tests/filter_tests/test_timeuntil.py::FunctionTests.test_until_now
// Ported from Django: tests/template_tests/filter_tests/test_timeuntil.py::FunctionTests.test_no_args
// Ported from Django: tests/template_tests/filter_tests/test_timeuntil.py::FunctionTests.test_explicit_date
func TestTimeuntilFunction(t *testing.T) {
	runRenderCases(t, []renderCase{
		{name: "test_until_now", src: "{{ a|timeuntil }}", ctx: map[string]any{"a": time.Now().Add(24*time.Hour + time.Second)}, want: "1\u00a0day"},
		{name: "test_no_args", src: "{{ a|timeuntil }}", ctx: map[string]any{"a": nil}, want: ""},
		{name: "test_explicit_date", src: "{{ a|timeuntil:b }}", ctx: map[string]any{"a": time.Date(2005, 12, 30, 0, 0, 0, 0, time.UTC), "b": time.Date(2005, 12, 29, 0, 0, 0, 0, time.UTC)}, want: "1\u00a0day"},
	})
}
