package templates_test

import (
	"testing"

	"github.com/oliverhaas/djangogo/templates"
)

// TestLinebreaks ports Django's LinebreaksTests.
//
// Ported from Django: tests/template_tests/filter_tests/test_linebreaks.py::LinebreaksTests.test_linebreaks01
// Ported from Django: tests/template_tests/filter_tests/test_linebreaks.py::LinebreaksTests.test_linebreaks02
func TestLinebreaks(t *testing.T) {
	runRenderCases(t, []renderCase{
		{name: "test_linebreaks01", src: "{{ a|linebreaks }} {{ b|linebreaks }}", ctx: map[string]any{"a": "x&\ny", "b": templates.MarkSafe("x&\ny")}, want: "<p>x&amp;<br>y</p> <p>x&<br>y</p>"},
		{name: "test_linebreaks02", src: "{% autoescape off %}{{ a|linebreaks }} {{ b|linebreaks }}{% endautoescape %}", ctx: map[string]any{"a": "x&\ny", "b": templates.MarkSafe("x&\ny")}, want: "<p>x&<br>y</p> <p>x&<br>y</p>"},
	})
}
