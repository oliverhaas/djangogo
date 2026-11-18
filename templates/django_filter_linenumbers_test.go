package templates_test

import (
	"testing"

	"github.com/oliverhaas/djangogo/templates"
)

// TestLinenumbers ports Django's LinenumbersTests.
//
// Ported from Django: tests/template_tests/filter_tests/test_linenumbers.py::LinenumbersTests.test_linenumbers01
// Ported from Django: tests/template_tests/filter_tests/test_linenumbers.py::LinenumbersTests.test_linenumbers02
func TestLinenumbers(t *testing.T) {
	runRenderCases(t, []renderCase{
		{name: "test_linenumbers01", src: "{{ a|linenumbers }} {{ b|linenumbers }}", ctx: map[string]any{"a": "one\n<two>\nthree", "b": templates.MarkSafe("one\n&lt;two&gt;\nthree")}, want: "1. one\n2. &lt;two&gt;\n3. three 1. one\n2. &lt;two&gt;\n3. three"},
		{name: "test_linenumbers02", src: "{% autoescape off %}{{ a|linenumbers }} {{ b|linenumbers }}{% endautoescape %}", ctx: map[string]any{"a": "one\n<two>\nthree", "b": templates.MarkSafe("one\n&lt;two&gt;\nthree")}, want: "1. one\n2. <two>\n3. three 1. one\n2. &lt;two&gt;\n3. three"},
	})
}

// TestLinenumbersFunction ports Django's FunctionTests.
//
// Ported from Django: tests/template_tests/filter_tests/test_linenumbers.py::FunctionTests.test_linenumbers
// Ported from Django: tests/template_tests/filter_tests/test_linenumbers.py::FunctionTests.test_linenumbers2
// Ported from Django: tests/template_tests/filter_tests/test_linenumbers.py::FunctionTests.test_non_string_input
func TestLinenumbersFunction(t *testing.T) {
	runRenderCases(t, []renderCase{
		{name: "test_linenumbers", src: "{{ a|linenumbers }}", ctx: map[string]any{"a": "line 1\nline 2"}, want: "1. line 1\n2. line 2"},
		{name: "test_linenumbers2", src: "{{ a|linenumbers }}", ctx: map[string]any{"a": "x\nx\nx\nx\nx\nx\nx\nx\nx\nx"}, want: "01. x\n02. x\n03. x\n04. x\n05. x\n06. x\n07. x\n08. x\n09. x\n10. x"},
		{name: "test_non_string_input", src: "{{ a|linenumbers }}", ctx: map[string]any{"a": 123}, want: "1. 123"},
	})
}
