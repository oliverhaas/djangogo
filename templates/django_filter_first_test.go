package templates_test

import (
	"testing"

	"github.com/oliverhaas/djangogo/templates"
)

// TestFirst ports Django's FirstTests.
//
// Ported from Django: tests/template_tests/filter_tests/test_first.py::FirstTests.test_first01
// Ported from Django: tests/template_tests/filter_tests/test_first.py::FirstTests.test_first02
func TestFirst(t *testing.T) {
	runRenderCases(t, []renderCase{
		{name: "test_first01", src: "{{ a|first }} {{ b|first }}", ctx: map[string]any{"a": []any{"a&b", "x"}, "b": []any{templates.MarkSafe("a&b"), "x"}}, want: "a&amp;b a&b"},
		{name: "test_first02", src: "{% autoescape off %}{{ a|first }} {{ b|first }}{% endautoescape %}", ctx: map[string]any{"a": []any{"a&b", "x"}, "b": []any{templates.MarkSafe("a&b"), "x"}}, want: "a&b a&b"},
	})
}

// TestFirstFunction ports Django's FunctionTests.
//
// Ported from Django: tests/template_tests/filter_tests/test_first.py::FunctionTests.test_list
// Ported from Django: tests/template_tests/filter_tests/test_first.py::FunctionTests.test_empty_string
// Ported from Django: tests/template_tests/filter_tests/test_first.py::FunctionTests.test_string
func TestFirstFunction(t *testing.T) {
	runRenderCases(t, []renderCase{
		{name: "test_list", src: "{{ a|first }}", ctx: map[string]any{"a": []any{0, 1, 2}}, want: "0"},
		{name: "test_empty_string", src: "{{ a|first }}", ctx: map[string]any{"a": ""}, want: ""},
		{name: "test_string", src: "{{ a|first }}", ctx: map[string]any{"a": "test"}, want: "t"},
	})
}
