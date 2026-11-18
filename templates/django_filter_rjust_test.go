package templates_test

import (
	"testing"

	"github.com/oliverhaas/djangogo/templates"
)

// TestRjust ports Django's RjustTests.
//
// Ported from Django: tests/template_tests/filter_tests/test_rjust.py::RjustTests.test_rjust01
// Ported from Django: tests/template_tests/filter_tests/test_rjust.py::RjustTests.test_rjust02
func TestRjust(t *testing.T) {
	runRenderCases(t, []renderCase{
		{name: "test_rjust01", src: `{% autoescape off %}.{{ a|rjust:"5" }}. .{{ b|rjust:"5" }}.{% endautoescape %}`, ctx: map[string]any{"a": "a&b", "b": templates.MarkSafe("a&b")}, want: ".  a&b. .  a&b."},
		{name: "test_rjust02", src: `.{{ a|rjust:"5" }}. .{{ b|rjust:"5" }}.`, ctx: map[string]any{"a": "a&b", "b": templates.MarkSafe("a&b")}, want: ".  a&amp;b. .  a&b."},
	})
}

// TestRjustFunction ports Django's FunctionTests.
//
// Ported from Django: tests/template_tests/filter_tests/test_rjust.py::FunctionTests.test_rjust
// Ported from Django: tests/template_tests/filter_tests/test_rjust.py::FunctionTests.test_less_than_string_length
// Ported from Django: tests/template_tests/filter_tests/test_rjust.py::FunctionTests.test_non_string_input
func TestRjustFunction(t *testing.T) {
	runRenderCases(t, []renderCase{
		{name: "test_rjust", src: "{{ a|rjust:b }}", ctx: map[string]any{"a": "test", "b": 10}, want: "      test"},
		{name: "test_less_than_string_length", src: "{{ a|rjust:b }}", ctx: map[string]any{"a": "test", "b": 3}, want: "test"},
		{name: "test_non_string_input", src: "{{ a|rjust:b }}", ctx: map[string]any{"a": 123, "b": 4}, want: " 123"},
	})
}
