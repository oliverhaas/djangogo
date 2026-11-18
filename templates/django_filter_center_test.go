package templates_test

import (
	"testing"

	"github.com/oliverhaas/djangogo/templates"
)

// TestCenter ports Django's CenterTests.
//
// Ported from Django: tests/template_tests/filter_tests/test_center.py::CenterTests.test_center01
// Ported from Django: tests/template_tests/filter_tests/test_center.py::CenterTests.test_center02
func TestCenter(t *testing.T) {
	runRenderCases(t, []renderCase{
		{name: "test_center01", src: `{% autoescape off %}.{{ a|center:"5" }}. .{{ b|center:"5" }}.{% endautoescape %}`, ctx: map[string]any{"a": "a&b", "b": templates.MarkSafe("a&b")}, want: ". a&b . . a&b ."},
		{name: "test_center02", src: `.{{ a|center:"5" }}. .{{ b|center:"5" }}.`, ctx: map[string]any{"a": "a&b", "b": templates.MarkSafe("a&b")}, want: ". a&amp;b . . a&b ."},
	})
}

// TestCenterFunction ports Django's FunctionTests.
//
// Ported from Django: tests/template_tests/filter_tests/test_center.py::FunctionTests.test_center
// Ported from Django: tests/template_tests/filter_tests/test_center.py::FunctionTests.test_non_string_input
// Ported from Django: tests/template_tests/filter_tests/test_center.py::FunctionTests.test_odd_input
// Ported from Django: tests/template_tests/filter_tests/test_center.py::FunctionTests.test_even_input
func TestCenterFunction(t *testing.T) {
	runRenderCases(t, []renderCase{
		{name: "test_center", src: "{{ a|center:b }}", ctx: map[string]any{"a": "test", "b": 6}, want: " test "},
		{name: "test_non_string_input", src: "{{ a|center:b }}", ctx: map[string]any{"a": 123, "b": 5}, want: " 123 "},
		{name: "test_odd_input", src: "{{ a|center:b }}", ctx: map[string]any{"a": "odd", "b": 6}, want: " odd  "},
		{name: "test_even_input", src: "{{ a|center:b }}", ctx: map[string]any{"a": "even", "b": 7}, want: " even  "},
	})
}
