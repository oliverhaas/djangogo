package templates_test

import (
	"testing"

	"github.com/oliverhaas/djangogo/templates"
)

// TestAddslashes ports Django's AddslashesTests.
//
// Ported from Django: tests/template_tests/filter_tests/test_addslashes.py::AddslashesTests.test_addslashes01
// Ported from Django: tests/template_tests/filter_tests/test_addslashes.py::AddslashesTests.test_addslashes02
func TestAddslashes(t *testing.T) {
	runRenderCases(t, []renderCase{
		{name: "test_addslashes01", src: "{% autoescape off %}{{ a|addslashes }} {{ b|addslashes }}{% endautoescape %}", ctx: map[string]any{"a": "<a>'", "b": templates.MarkSafe("<a>'")}, want: `<a>\' <a>\'`},
		{name: "test_addslashes02", src: "{{ a|addslashes }} {{ b|addslashes }}", ctx: map[string]any{"a": "<a>'", "b": templates.MarkSafe("<a>'")}, want: `&lt;a&gt;\&#x27; <a>\'`},
	})
}

// TestAddslashesFunction ports Django's FunctionTests.
//
// Ported from Django: tests/template_tests/filter_tests/test_addslashes.py::FunctionTests.test_backslashes
// Ported from Django: tests/template_tests/filter_tests/test_addslashes.py::FunctionTests.test_non_string_input
func TestAddslashesFunction(t *testing.T) {
	runRenderCases(t, []renderCase{
		{name: "test_backslashes", src: "{{ a|addslashes }}", ctx: map[string]any{"a": `\ : backslashes, too`}, want: `\\ : backslashes, too`},
		{name: "test_non_string_input", src: "{{ a|addslashes }}", ctx: map[string]any{"a": 123}, want: "123"},
	})
}
