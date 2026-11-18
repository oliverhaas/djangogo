package templates_test

import (
	"testing"

	"github.com/oliverhaas/djangogo/templates"
)

// TestStringformat ports Django's StringformatTests.
//
// Ported from Django: tests/template_tests/filter_tests/test_stringformat.py::StringformatTests.test_stringformat01
// Ported from Django: tests/template_tests/filter_tests/test_stringformat.py::StringformatTests.test_stringformat02
func TestStringformat(t *testing.T) {
	runRenderCases(t, []renderCase{
		{name: "test_stringformat01", src: `{% autoescape off %}.{{ a|stringformat:"5s" }}. .{{ b|stringformat:"5s" }}.{% endautoescape %}`, ctx: map[string]any{"a": "a<b", "b": templates.MarkSafe("a<b")}, want: ".  a<b. .  a<b."},
		{name: "test_stringformat02", src: `.{{ a|stringformat:"5s" }}. .{{ b|stringformat:"5s" }}.`, ctx: map[string]any{"a": "a<b", "b": templates.MarkSafe("a<b")}, want: ".  a&lt;b. .  a<b."},
	})
}

// TestStringformatFunction ports Django's FunctionTests.
//
// Ported from Django: tests/template_tests/filter_tests/test_stringformat.py::FunctionTests.test_format
// Ported from Django: tests/template_tests/filter_tests/test_stringformat.py::FunctionTests.test_invalid
func TestStringformatFunction(t *testing.T) {
	runRenderCases(t, []renderCase{
		{name: "test_format_1", src: "{{ a|stringformat:b }}", ctx: map[string]any{"a": 1, "b": "03d"}, want: "001"},
		{name: "test_format_2", src: "{{ a|stringformat:b }}", ctx: map[string]any{"a": []any{1, nil}, "b": "s"}, want: "[1, None]", skip: "a list prints in Go syntax, not as Python's repr, see fidelity/divergences.md"},
		{name: "test_format_3", src: "{{ a|stringformat:b }}", ctx: map[string]any{"a": []any{1, 2, 3}, "b": "s"}, want: "(1, 2, 3)", skip: "a list prints in Go syntax, not as Python's repr, see fidelity/divergences.md"},
		{name: "test_format_4", src: "{{ a|stringformat:b }}", ctx: map[string]any{"a": []any{1}, "b": "s"}, want: "(1,)", skip: "a list prints in Go syntax, not as Python's repr, see fidelity/divergences.md"},
		{name: "test_invalid_1", src: "{{ a|stringformat:b }}", ctx: map[string]any{"a": 1, "b": "z"}, want: ""},
		{name: "test_invalid_2", src: "{{ a|stringformat:b }}", ctx: map[string]any{"a": nil, "b": "d"}, want: ""},
		{name: "test_invalid_3", src: "{{ a|stringformat:b }}", ctx: map[string]any{"a": []any{1, 2, 3}, "b": "d"}, want: ""},
	})
}
