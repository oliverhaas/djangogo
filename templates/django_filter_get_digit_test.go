package templates_test

import (
	"testing"
)

// TestGetDigitFunction ports Django's FunctionTests.
//
// Ported from Django: tests/template_tests/filter_tests/test_get_digit.py::FunctionTests.test_values
// Ported from Django: tests/template_tests/filter_tests/test_get_digit.py::FunctionTests.test_string
func TestGetDigitFunction(t *testing.T) {
	runRenderCases(t, []renderCase{
		{name: "test_values_1", src: "{{ a|get_digit:b }}", ctx: map[string]any{"a": 123, "b": 1}, want: "3"},
		{name: "test_values_2", src: "{{ a|get_digit:b }}", ctx: map[string]any{"a": 123, "b": 2}, want: "2"},
		{name: "test_values_3", src: "{{ a|get_digit:b }}", ctx: map[string]any{"a": 123, "b": 3}, want: "1"},
		{name: "test_values_4", src: "{{ a|get_digit:b }}", ctx: map[string]any{"a": 123, "b": 4}, want: "0"},
		{name: "test_values_5", src: "{{ a|get_digit:b }}", ctx: map[string]any{"a": 123, "b": 0}, want: "123"},
		{name: "test_string", src: "{{ a|get_digit:b }}", ctx: map[string]any{"a": "xyz", "b": 0}, want: "xyz"},
	})
}
