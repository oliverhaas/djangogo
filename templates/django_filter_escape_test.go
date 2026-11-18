package templates_test

import (
	"testing"

	"github.com/oliverhaas/djangogo/templates"
)

// TestEscape ports Django's EscapeTests.
//
// Ported from Django: tests/template_tests/filter_tests/test_escape.py::EscapeTests.test_escape01
// Ported from Django: tests/template_tests/filter_tests/test_escape.py::EscapeTests.test_escape02
// Ported from Django: tests/template_tests/filter_tests/test_escape.py::EscapeTests.test_escape03
// Ported from Django: tests/template_tests/filter_tests/test_escape.py::EscapeTests.test_escape04
func TestEscape(t *testing.T) {
	runRenderCases(t, []renderCase{
		{name: "test_escape01", src: "{{ a|escape }} {{ b|escape }}", ctx: map[string]any{"a": "x&y", "b": templates.MarkSafe("x&y")}, want: "x&amp;y x&y"},
		{name: "test_escape02", src: "{% autoescape off %}{{ a|escape }} {{ b|escape }}{% endautoescape %}", ctx: map[string]any{"a": "x&y", "b": templates.MarkSafe("x&y")}, want: "x&amp;y x&y"},
		{name: "test_escape03", src: "{% autoescape off %}{{ a|escape|escape }}{% endautoescape %}", ctx: map[string]any{"a": "x&y"}, want: "x&amp;y"},
		{name: "test_escape04", src: "{{ a|escape|escape }}", ctx: map[string]any{"a": "x&y"}, want: "x&amp;y"},
	})
}

// TestEscapeFunction ports Django's FunctionTests.
//
// Ported from Django: tests/template_tests/filter_tests/test_escape.py::FunctionTests.test_non_string_input
func TestEscapeFunction(t *testing.T) {
	runRenderCases(t, []renderCase{
		{name: "test_non_string_input", src: "{{ a|escape }}", ctx: map[string]any{"a": 123}, want: "123"},
	})
}
