package templates_test

import (
	"testing"

	"github.com/oliverhaas/djangogo/templates"
)

// TestCut ports Django's CutTests.
//
// Ported from Django: tests/template_tests/filter_tests/test_cut.py::CutTests.test_cut01
// Ported from Django: tests/template_tests/filter_tests/test_cut.py::CutTests.test_cut02
// Ported from Django: tests/template_tests/filter_tests/test_cut.py::CutTests.test_cut03
// Ported from Django: tests/template_tests/filter_tests/test_cut.py::CutTests.test_cut04
// Ported from Django: tests/template_tests/filter_tests/test_cut.py::CutTests.test_cut05
// Ported from Django: tests/template_tests/filter_tests/test_cut.py::CutTests.test_cut06
func TestCut(t *testing.T) {
	runRenderCases(t, []renderCase{
		{name: "test_cut01", src: `{% autoescape off %}{{ a|cut:"x" }} {{ b|cut:"x" }}{% endautoescape %}`, ctx: map[string]any{"a": "x&y", "b": templates.MarkSafe("x&amp;y")}, want: "&y &amp;y"},
		{name: "test_cut02", src: `{{ a|cut:"x" }} {{ b|cut:"x" }}`, ctx: map[string]any{"a": "x&y", "b": templates.MarkSafe("x&amp;y")}, want: "&amp;y &amp;y"},
		{name: "test_cut03", src: `{% autoescape off %}{{ a|cut:"&" }} {{ b|cut:"&" }}{% endautoescape %}`, ctx: map[string]any{"a": "x&y", "b": templates.MarkSafe("x&amp;y")}, want: "xy xamp;y"},
		{name: "test_cut04", src: `{{ a|cut:"&" }} {{ b|cut:"&" }}`, ctx: map[string]any{"a": "x&y", "b": templates.MarkSafe("x&amp;y")}, want: "xy xamp;y"},
		{name: "test_cut05", src: `{% autoescape off %}{{ a|cut:";" }} {{ b|cut:";" }}{% endautoescape %}`, ctx: map[string]any{"a": "x&y", "b": templates.MarkSafe("x&amp;y")}, want: "x&y x&ampy"},
		{name: "test_cut06", src: `{{ a|cut:";" }} {{ b|cut:";" }}`, ctx: map[string]any{"a": "x&y", "b": templates.MarkSafe("x&amp;y")}, want: "x&amp;y x&amp;ampy"},
	})
}

// TestCutFunction ports Django's FunctionTests.
//
// Ported from Django: tests/template_tests/filter_tests/test_cut.py::FunctionTests.test_character
// Ported from Django: tests/template_tests/filter_tests/test_cut.py::FunctionTests.test_characters
// Ported from Django: tests/template_tests/filter_tests/test_cut.py::FunctionTests.test_non_matching_string
// Ported from Django: tests/template_tests/filter_tests/test_cut.py::FunctionTests.test_non_string_input
func TestCutFunction(t *testing.T) {
	runRenderCases(t, []renderCase{
		{name: "test_character", src: "{{ a|cut:b }}", ctx: map[string]any{"a": "a string to be mangled", "b": "a"}, want: " string to be mngled"},
		{name: "test_characters", src: "{{ a|cut:b }}", ctx: map[string]any{"a": "a string to be mangled", "b": "ng"}, want: "a stri to be maled"},
		{name: "test_non_matching_string", src: "{{ a|cut:b }}", ctx: map[string]any{"a": "a string to be mangled", "b": "strings"}, want: "a string to be mangled"},
		{name: "test_non_string_input", src: "{{ a|cut:b }}", ctx: map[string]any{"a": 123, "b": "2"}, want: "13"},
	})
}
