package templates_test

import (
	"testing"

	"github.com/oliverhaas/djangogo/templates"
)

// TestLinebreaksbr ports Django's LinebreaksbrTests.
//
// Ported from Django: tests/template_tests/filter_tests/test_linebreaksbr.py::LinebreaksbrTests.test_linebreaksbr01
// Ported from Django: tests/template_tests/filter_tests/test_linebreaksbr.py::LinebreaksbrTests.test_linebreaksbr02
func TestLinebreaksbr(t *testing.T) {
	runRenderCases(t, []renderCase{
		{name: "test_linebreaksbr01", src: "{{ a|linebreaksbr }} {{ b|linebreaksbr }}", ctx: map[string]any{"a": "x&\ny", "b": templates.MarkSafe("x&\ny")}, want: "x&amp;<br>y x&<br>y"},
		{name: "test_linebreaksbr02", src: "{% autoescape off %}{{ a|linebreaksbr }} {{ b|linebreaksbr }}{% endautoescape %}", ctx: map[string]any{"a": "x&\ny", "b": templates.MarkSafe("x&\ny")}, want: "x&<br>y x&<br>y"},
	})
}

// TestLinebreaksbrFunction ports Django's FunctionTests.
//
// Ported from Django: tests/template_tests/filter_tests/test_linebreaksbr.py::FunctionTests.test_non_string_input
func TestLinebreaksbrFunction(t *testing.T) {
	runRenderCases(t, []renderCase{
		{name: "test_non_string_input", src: "{{ a|linebreaksbr }}", ctx: map[string]any{"a": 123}, want: "123"},
	})
}
