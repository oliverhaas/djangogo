package templates_test

import (
	"testing"

	"github.com/oliverhaas/djangogo/templates"
)

// TestDefault ports Django's DefaultTests.
//
// Ported from Django: tests/template_tests/filter_tests/test_default.py::DefaultTests.test_default01
// Ported from Django: tests/template_tests/filter_tests/test_default.py::DefaultTests.test_default02
// Ported from Django: tests/template_tests/filter_tests/test_default.py::DefaultTests.test_default03
// Ported from Django: tests/template_tests/filter_tests/test_default.py::DefaultTests.test_default04
func TestDefault(t *testing.T) {
	runRenderCases(t, []renderCase{
		{name: "test_default01", src: `{{ a|default:"x<" }}`, ctx: map[string]any{"a": ""}, want: "x<"},
		{name: "test_default02", src: `{% autoescape off %}{{ a|default:"x<" }}{% endautoescape %}`, ctx: map[string]any{"a": ""}, want: "x<"},
		{name: "test_default03", src: `{{ a|default:"x<" }}`, ctx: map[string]any{"a": templates.MarkSafe("x>")}, want: "x>"},
		{name: "test_default04", src: `{% autoescape off %}{{ a|default:"x<" }}{% endautoescape %}`, ctx: map[string]any{"a": templates.MarkSafe("x>")}, want: "x>"},
	})
}

// TestDefaultIfNone ports Django's DefaultIfNoneTests.
//
// Ported from Django: tests/template_tests/filter_tests/test_default.py::DefaultIfNoneTests.test_default_if_none01
// Ported from Django: tests/template_tests/filter_tests/test_default.py::DefaultIfNoneTests.test_default_if_none02
func TestDefaultIfNone(t *testing.T) {
	runRenderCases(t, []renderCase{
		{name: "test_default_if_none01", src: `{{ a|default:"x<" }}`, ctx: map[string]any{"a": nil}, want: "x<"},
		{name: "test_default_if_none02", src: `{% autoescape off %}{{ a|default:"x<" }}{% endautoescape %}`, ctx: map[string]any{"a": nil}, want: "x<"},
	})
}

// TestDefaultFunction ports Django's FunctionTests.
//
// Ported from Django: tests/template_tests/filter_tests/test_default.py::FunctionTests.test_value
// Ported from Django: tests/template_tests/filter_tests/test_default.py::FunctionTests.test_none
// Ported from Django: tests/template_tests/filter_tests/test_default.py::FunctionTests.test_empty_string
func TestDefaultFunction(t *testing.T) {
	runRenderCases(t, []renderCase{
		{name: "test_value", src: "{{ a|default:b }}", ctx: map[string]any{"a": "val", "b": "default"}, want: "val"},
		{name: "test_none", src: "{{ a|default:b }}", ctx: map[string]any{"a": nil, "b": "default"}, want: "default"},
		{name: "test_empty_string", src: "{{ a|default:b }}", ctx: map[string]any{"a": "", "b": "default"}, want: "default"},
	})
}
