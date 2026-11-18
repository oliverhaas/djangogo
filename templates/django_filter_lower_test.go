package templates_test

import (
	"testing"

	"github.com/oliverhaas/djangogo/templates"
)

// TestLower ports Django's LowerTests.
//
// Ported from Django: tests/template_tests/filter_tests/test_lower.py::LowerTests.test_lower01
// Ported from Django: tests/template_tests/filter_tests/test_lower.py::LowerTests.test_lower02
func TestLower(t *testing.T) {
	runRenderCases(t, []renderCase{
		{name: "test_lower01", src: "{% autoescape off %}{{ a|lower }} {{ b|lower }}{% endautoescape %}", ctx: map[string]any{"a": "Apple & banana", "b": templates.MarkSafe("Apple &amp; banana")}, want: "apple & banana apple &amp; banana"},
		{name: "test_lower02", src: "{{ a|lower }} {{ b|lower }}", ctx: map[string]any{"a": "Apple & banana", "b": templates.MarkSafe("Apple &amp; banana")}, want: "apple &amp; banana apple &amp; banana"},
	})
}

// TestLowerFunction ports Django's FunctionTests.
//
// Ported from Django: tests/template_tests/filter_tests/test_lower.py::FunctionTests.test_lower
// Ported from Django: tests/template_tests/filter_tests/test_lower.py::FunctionTests.test_unicode
// Ported from Django: tests/template_tests/filter_tests/test_lower.py::FunctionTests.test_non_string_input
func TestLowerFunction(t *testing.T) {
	runRenderCases(t, []renderCase{
		{name: "test_lower", src: "{{ a|lower }}", ctx: map[string]any{"a": "TEST"}, want: "test"},
		{name: "test_unicode", src: "{{ a|lower }}", ctx: map[string]any{"a": "Ë"}, want: "ë"},
		{name: "test_non_string_input", src: "{{ a|lower }}", ctx: map[string]any{"a": 123}, want: "123"},
	})
}
