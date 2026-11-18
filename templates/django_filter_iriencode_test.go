package templates_test

import (
	"testing"

	"github.com/oliverhaas/djangogo/templates"
)

// TestIriencode ports Django's IriencodeTests.
//
// Ported from Django: tests/template_tests/filter_tests/test_iriencode.py::IriencodeTests.test_iriencode01
// Ported from Django: tests/template_tests/filter_tests/test_iriencode.py::IriencodeTests.test_iriencode02
// Ported from Django: tests/template_tests/filter_tests/test_iriencode.py::IriencodeTests.test_iriencode03
// Ported from Django: tests/template_tests/filter_tests/test_iriencode.py::IriencodeTests.test_iriencode04
func TestIriencode(t *testing.T) {
	runRenderCases(t, []renderCase{
		{name: "test_iriencode01", src: "{{ url|iriencode }}", ctx: map[string]any{"url": "?test=1&me=2"}, want: "?test=1&amp;me=2"},
		{name: "test_iriencode02", src: "{% autoescape off %}{{ url|iriencode }}{% endautoescape %}", ctx: map[string]any{"url": "?test=1&me=2"}, want: "?test=1&me=2"},
		{name: "test_iriencode03", src: "{{ url|iriencode }}", ctx: map[string]any{"url": templates.MarkSafe("?test=1&me=2")}, want: "?test=1&me=2"},
		{name: "test_iriencode04", src: "{% autoescape off %}{{ url|iriencode }}{% endautoescape %}", ctx: map[string]any{"url": templates.MarkSafe("?test=1&me=2")}, want: "?test=1&me=2"},
	})
}

// TestIriencodeFunction ports Django's FunctionTests.
//
// Ported from Django: tests/template_tests/filter_tests/test_iriencode.py::FunctionTests.test_unicode
func TestIriencodeFunction(t *testing.T) {
	runRenderCases(t, []renderCase{
		{name: "test_unicode", src: "{{ a|iriencode }}", ctx: map[string]any{"a": "Sør-Trøndelag"}, want: "S%C3%B8r-Tr%C3%B8ndelag"},
	})
}
