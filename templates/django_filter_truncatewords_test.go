package templates_test

import (
	"testing"

	"github.com/oliverhaas/djangogo/templates"
)

// TestTruncatewords ports Django's TruncatewordsTests.
//
// Ported from Django: tests/template_tests/filter_tests/test_truncatewords.py::TruncatewordsTests.test_truncatewords01
// Ported from Django: tests/template_tests/filter_tests/test_truncatewords.py::TruncatewordsTests.test_truncatewords02
func TestTruncatewords(t *testing.T) {
	runRenderCases(t, []renderCase{
		{name: "test_truncatewords01", src: `{% autoescape off %}{{ a|truncatewords:"2" }} {{ b|truncatewords:"2"}}{% endautoescape %}`, ctx: map[string]any{"a": "alpha & bravo", "b": templates.MarkSafe("alpha &amp; bravo")}, want: "alpha & … alpha &amp; …"},
		{name: "test_truncatewords02", src: `{{ a|truncatewords:"2" }} {{ b|truncatewords:"2"}}`, ctx: map[string]any{"a": "alpha & bravo", "b": templates.MarkSafe("alpha &amp; bravo")}, want: "alpha &amp; … alpha &amp; …"},
	})
}

// TestTruncatewordsFunction ports Django's FunctionTests.
//
// Ported from Django: tests/template_tests/filter_tests/test_truncatewords.py::FunctionTests.test_truncate
// Ported from Django: tests/template_tests/filter_tests/test_truncatewords.py::FunctionTests.test_truncate2
// Ported from Django: tests/template_tests/filter_tests/test_truncatewords.py::FunctionTests.test_overtruncate
// Ported from Django: tests/template_tests/filter_tests/test_truncatewords.py::FunctionTests.test_invalid_number
// Ported from Django: tests/template_tests/filter_tests/test_truncatewords.py::FunctionTests.test_non_string_input
func TestTruncatewordsFunction(t *testing.T) {
	runRenderCases(t, []renderCase{
		{name: "test_truncate", src: "{{ a|truncatewords:b }}", ctx: map[string]any{"a": "A sentence with a few words in it", "b": 1}, want: "A …"},
		{name: "test_truncate2", src: "{{ a|truncatewords:b }}", ctx: map[string]any{"a": "A sentence with a few words in it", "b": 5}, want: "A sentence with a few …"},
		{name: "test_overtruncate", src: "{{ a|truncatewords:b }}", ctx: map[string]any{"a": "A sentence with a few words in it", "b": 100}, want: "A sentence with a few words in it"},
		{name: "test_invalid_number", src: "{{ a|truncatewords:b }}", ctx: map[string]any{"a": "A sentence with a few words in it", "b": "not a number"}, want: "A sentence with a few words in it"},
		{name: "test_non_string_input", src: "{{ a|truncatewords:b }}", ctx: map[string]any{"a": 123, "b": 2}, want: "123"},
	})
}
