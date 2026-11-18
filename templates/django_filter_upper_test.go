package templates_test

import (
	"testing"

	"github.com/oliverhaas/djangogo/templates"
)

// TestUpper ports Django's UpperTests.
//
// Ported from Django: tests/template_tests/filter_tests/test_upper.py::UpperTests.test_upper01
// Ported from Django: tests/template_tests/filter_tests/test_upper.py::UpperTests.test_upper02
func TestUpper(t *testing.T) {
	runRenderCases(t, []renderCase{
		{name: "test_upper01", src: "{% autoescape off %}{{ a|upper }} {{ b|upper }}{% endautoescape %}", ctx: map[string]any{"a": "a & b", "b": templates.MarkSafe("a &amp; b")}, want: "A & B A &AMP; B"},
		{name: "test_upper02", src: "{{ a|upper }} {{ b|upper }}", ctx: map[string]any{"a": "a & b", "b": templates.MarkSafe("a &amp; b")}, want: "A &amp; B A &amp;AMP; B"},
	})
}

// TestUpperFunction ports Django's FunctionTests.
//
// Ported from Django: tests/template_tests/filter_tests/test_upper.py::FunctionTests.test_upper
// Ported from Django: tests/template_tests/filter_tests/test_upper.py::FunctionTests.test_unicode
// Ported from Django: tests/template_tests/filter_tests/test_upper.py::FunctionTests.test_non_string_input
func TestUpperFunction(t *testing.T) {
	runRenderCases(t, []renderCase{
		{name: "test_upper", src: "{{ a|upper }}", ctx: map[string]any{"a": "Mixed case input"}, want: "MIXED CASE INPUT"},
		{name: "test_unicode", src: "{{ a|upper }}", ctx: map[string]any{"a": "ë"}, want: "Ë"},
		{name: "test_non_string_input", src: "{{ a|upper }}", ctx: map[string]any{"a": 123}, want: "123"},
	})
}
