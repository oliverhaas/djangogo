package templates_test

import (
	"testing"
)

// TestTime ports Django's TimeTests.
//
// Ported from Django: tests/template_tests/filter_tests/test_time.py::TimeTests.test_time06
func TestTime(t *testing.T) {
	runRenderCases(t, []renderCase{
		{name: "test_time06", src: `{{ obj|time:"P:e:O:T:Z" }}`, ctx: map[string]any{"obj": "non-datetime-value"}, want: ""},
	})
}

// TestTimeFunction ports Django's FunctionTests.
//
// Ported from Django: tests/template_tests/filter_tests/test_time.py::FunctionTests.test_no_args
func TestTimeFunction(t *testing.T) {
	runRenderCases(t, []renderCase{
		{name: "test_no_args_1", src: "{{ a|time }}", ctx: map[string]any{"a": ""}, want: ""},
		{name: "test_no_args_2", src: "{{ a|time }}", ctx: map[string]any{"a": nil}, want: ""},
	})
}
