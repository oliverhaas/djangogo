package templates_test

import (
	"testing"

	"github.com/oliverhaas/djangogo/templates"
)

// TestLast ports Django's LastTests.
//
// Ported from Django: tests/template_tests/filter_tests/test_last.py::LastTests.test_last01
// Ported from Django: tests/template_tests/filter_tests/test_last.py::LastTests.test_last02
// Ported from Django: tests/template_tests/filter_tests/test_last.py::LastTests.test_empty_list
func TestLast(t *testing.T) {
	runRenderCases(t, []renderCase{
		{name: "test_last01", src: "{{ a|last }} {{ b|last }}", ctx: map[string]any{"a": []any{"x", "a&b"}, "b": []any{"x", templates.MarkSafe("a&b")}}, want: "a&amp;b a&b"},
		{name: "test_last02", src: "{% autoescape off %}{{ a|last }} {{ b|last }}{% endautoescape %}", ctx: map[string]any{"a": []any{"x", "a&b"}, "b": []any{"x", templates.MarkSafe("a&b")}}, want: "a&b a&b"},
		{name: "test_empty_list", src: "{% autoescape off %}{{ a|last }}{% endautoescape %}", ctx: map[string]any{"a": []any{}}, want: ""},
	})
}
