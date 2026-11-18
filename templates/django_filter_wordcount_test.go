package templates_test

import (
	"testing"

	"github.com/oliverhaas/djangogo/templates"
)

// TestWordcount ports Django's WordcountTests.
//
// Ported from Django: tests/template_tests/filter_tests/test_wordcount.py::WordcountTests.test_wordcount01
// Ported from Django: tests/template_tests/filter_tests/test_wordcount.py::WordcountTests.test_wordcount02
func TestWordcount(t *testing.T) {
	runRenderCases(t, []renderCase{
		{name: "test_wordcount01", src: "{% autoescape off %}{{ a|wordcount }} {{ b|wordcount }}{% endautoescape %}", ctx: map[string]any{"a": "a & b", "b": templates.MarkSafe("a &amp; b")}, want: "3 3"},
		{name: "test_wordcount02", src: "{{ a|wordcount }} {{ b|wordcount }}", ctx: map[string]any{"a": "a & b", "b": templates.MarkSafe("a &amp; b")}, want: "3 3"},
	})
}

// TestWordcountFunction ports Django's FunctionTests.
//
// Ported from Django: tests/template_tests/filter_tests/test_wordcount.py::FunctionTests.test_empty_string
// Ported from Django: tests/template_tests/filter_tests/test_wordcount.py::FunctionTests.test_count_one
// Ported from Django: tests/template_tests/filter_tests/test_wordcount.py::FunctionTests.test_count_multiple
// Ported from Django: tests/template_tests/filter_tests/test_wordcount.py::FunctionTests.test_non_string_input
func TestWordcountFunction(t *testing.T) {
	runRenderCases(t, []renderCase{
		{name: "test_empty_string", src: "{{ a|wordcount }}", ctx: map[string]any{"a": ""}, want: "0"},
		{name: "test_count_one", src: "{{ a|wordcount }}", ctx: map[string]any{"a": "oneword"}, want: "1"},
		{name: "test_count_multiple", src: "{{ a|wordcount }}", ctx: map[string]any{"a": "lots of words"}, want: "3"},
		{name: "test_non_string_input", src: "{{ a|wordcount }}", ctx: map[string]any{"a": 123}, want: "1"},
	})
}
