package templates_test

import (
	"testing"

	"github.com/oliverhaas/djangogo/templates"
)

// TestWordwrap ports Django's WordwrapTests.
//
// Ported from Django: tests/template_tests/filter_tests/test_wordwrap.py::WordwrapTests.test_wordwrap01
// Ported from Django: tests/template_tests/filter_tests/test_wordwrap.py::WordwrapTests.test_wordwrap02
func TestWordwrap(t *testing.T) {
	runRenderCases(t, []renderCase{
		{name: "test_wordwrap01", src: `{% autoescape off %}{{ a|wordwrap:"3" }} {{ b|wordwrap:"3" }}{% endautoescape %}`, ctx: map[string]any{"a": "a & b", "b": templates.MarkSafe("a & b")}, want: "a &\nb a &\nb"},
		{name: "test_wordwrap02", src: `{{ a|wordwrap:"3" }} {{ b|wordwrap:"3" }}`, ctx: map[string]any{"a": "a & b", "b": templates.MarkSafe("a & b")}, want: "a &amp;\nb a &\nb"},
	})
}

// TestWordwrapFunction ports Django's FunctionTests.
//
// Ported from Django: tests/template_tests/filter_tests/test_wordwrap.py::FunctionTests.test_indent
// Ported from Django: tests/template_tests/filter_tests/test_wordwrap.py::FunctionTests.test_indent2
// Ported from Django: tests/template_tests/filter_tests/test_wordwrap.py::FunctionTests.test_non_string_input
func TestWordwrapFunction(t *testing.T) {
	runRenderCases(t, []renderCase{
		{name: "test_indent", src: "{{ a|wordwrap:b }}", ctx: map[string]any{"a": "this is a short paragraph of text.\n  But this line should be indented", "b": 14}, want: "this is a\nshort\nparagraph of\ntext.\n  But this\nline should be\nindented"},
		{name: "test_indent2", src: "{{ a|wordwrap:b }}", ctx: map[string]any{"a": "this is a short paragraph of text.\n  But this line should be indented", "b": 15}, want: "this is a short\nparagraph of\ntext.\n  But this line\nshould be\nindented"},
		{name: "test_non_string_input", src: "{{ a|wordwrap:b }}", ctx: map[string]any{"a": 123, "b": 2}, want: "123"},
	})
}
