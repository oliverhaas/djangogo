package templates_test

import (
	"testing"

	"github.com/oliverhaas/djangogo/templates"
)

// TestLength ports Django's LengthTests.
//
// Ported from Django: tests/template_tests/filter_tests/test_length.py::LengthTests.test_length01
// Ported from Django: tests/template_tests/filter_tests/test_length.py::LengthTests.test_length02
// Ported from Django: tests/template_tests/filter_tests/test_length.py::LengthTests.test_length03
// Ported from Django: tests/template_tests/filter_tests/test_length.py::LengthTests.test_length04
// Ported from Django: tests/template_tests/filter_tests/test_length.py::LengthTests.test_length05
// Ported from Django: tests/template_tests/filter_tests/test_length.py::LengthTests.test_length06
// Ported from Django: tests/template_tests/filter_tests/test_length.py::LengthTests.test_length07
func TestLength(t *testing.T) {
	runRenderCases(t, []renderCase{
		{name: "test_length01", src: "{{ list|length }}", ctx: map[string]any{"list": []any{"4", nil, true, map[string]any{}}}, want: "4"},
		{name: "test_length02", src: "{{ list|length }}", ctx: map[string]any{"list": []any{}}, want: "0"},
		{name: "test_length03", src: "{{ string|length }}", ctx: map[string]any{"string": ""}, want: "0"},
		{name: "test_length04", src: "{{ string|length }}", ctx: map[string]any{"string": "django"}, want: "6"},
		{name: "test_length05", src: "{% if string|length == 6 %}Pass{% endif %}", ctx: map[string]any{"string": templates.MarkSafe("django")}, want: "Pass"},
		{name: "test_length06", src: "{{ int|length }}", ctx: map[string]any{"int": 7}, want: "0"},
		{name: "test_length07", src: "{{ None|length }}", ctx: map[string]any{"None": nil}, want: "0"},
	})
}

// TestLengthFunction ports Django's FunctionTests.
//
// Ported from Django: tests/template_tests/filter_tests/test_length.py::FunctionTests.test_string
// Ported from Django: tests/template_tests/filter_tests/test_length.py::FunctionTests.test_safestring
// Ported from Django: tests/template_tests/filter_tests/test_length.py::FunctionTests.test_list
func TestLengthFunction(t *testing.T) {
	runRenderCases(t, []renderCase{
		{name: "test_string", src: "{{ a|length }}", ctx: map[string]any{"a": "1234"}, want: "4"},
		{name: "test_safestring", src: "{{ a|length }}", ctx: map[string]any{"a": templates.MarkSafe("1234")}, want: "4"},
		{name: "test_list", src: "{{ a|length }}", ctx: map[string]any{"a": []any{1, 2, 3, 4}}, want: "4"},
	})
}
