package templates_test

import (
	"testing"

	"github.com/oliverhaas/djangogo/templates"
)

// TestLjust ports Django's LjustTests.
//
// Ported from Django: tests/template_tests/filter_tests/test_ljust.py::LjustTests.test_ljust01
// Ported from Django: tests/template_tests/filter_tests/test_ljust.py::LjustTests.test_ljust02
func TestLjust(t *testing.T) {
	runRenderCases(t, []renderCase{
		{name: "test_ljust01", src: `{% autoescape off %}.{{ a|ljust:"5" }}. .{{ b|ljust:"5" }}.{% endautoescape %}`, ctx: map[string]any{"a": "a&b", "b": templates.MarkSafe("a&b")}, want: ".a&b  . .a&b  ."},
		{name: "test_ljust02", src: `.{{ a|ljust:"5" }}. .{{ b|ljust:"5" }}.`, ctx: map[string]any{"a": "a&b", "b": templates.MarkSafe("a&b")}, want: ".a&amp;b  . .a&b  ."},
	})
}

// TestLjustFunction ports Django's FunctionTests.
//
// Ported from Django: tests/template_tests/filter_tests/test_ljust.py::FunctionTests.test_ljust
// Ported from Django: tests/template_tests/filter_tests/test_ljust.py::FunctionTests.test_less_than_string_length
// Ported from Django: tests/template_tests/filter_tests/test_ljust.py::FunctionTests.test_non_string_input
func TestLjustFunction(t *testing.T) {
	runRenderCases(t, []renderCase{
		{name: "test_ljust_1", src: "{{ a|ljust:b }}", ctx: map[string]any{"a": "test", "b": 10}, want: "test      "},
		{name: "test_ljust_2", src: "{{ a|ljust:b }}", ctx: map[string]any{"a": "test", "b": 3}, want: "test"},
		{name: "test_less_than_string_length", src: "{{ a|ljust:b }}", ctx: map[string]any{"a": "test", "b": 3}, want: "test"},
		{name: "test_non_string_input", src: "{{ a|ljust:b }}", ctx: map[string]any{"a": 123, "b": 4}, want: "123 "},
	})
}
