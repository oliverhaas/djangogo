package templates_test

import (
	"testing"

	"github.com/oliverhaas/djangogo/templates"
)

// TestCapfirst ports Django's CapfirstTests.
//
// Ported from Django: tests/template_tests/filter_tests/test_capfirst.py::CapfirstTests.test_capfirst01
// Ported from Django: tests/template_tests/filter_tests/test_capfirst.py::CapfirstTests.test_capfirst02
func TestCapfirst(t *testing.T) {
	runRenderCases(t, []renderCase{
		{name: "test_capfirst01", src: "{% autoescape off %}{{ a|capfirst }} {{ b|capfirst }}{% endautoescape %}", ctx: map[string]any{"a": "fred>", "b": templates.MarkSafe("fred&gt;")}, want: "Fred> Fred&gt;"},
		{name: "test_capfirst02", src: "{{ a|capfirst }} {{ b|capfirst }}", ctx: map[string]any{"a": "fred>", "b": templates.MarkSafe("fred&gt;")}, want: "Fred&gt; Fred&gt;"},
	})
}

// TestCapfirstFunction ports Django's FunctionTests.
//
// Ported from Django: tests/template_tests/filter_tests/test_capfirst.py::FunctionTests.test_capfirst
func TestCapfirstFunction(t *testing.T) {
	runRenderCases(t, []renderCase{
		{name: "test_capfirst", src: "{{ a|capfirst }}", ctx: map[string]any{"a": "hello world"}, want: "Hello world"},
	})
}
