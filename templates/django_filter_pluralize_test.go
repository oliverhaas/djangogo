package templates_test

import (
	"testing"
)

// TestPluralizeFunction ports Django's FunctionTests.
//
// Ported from Django: tests/template_tests/filter_tests/test_pluralize.py::FunctionTests.test_integers
// Ported from Django: tests/template_tests/filter_tests/test_pluralize.py::FunctionTests.test_floats
// Ported from Django: tests/template_tests/filter_tests/test_pluralize.py::FunctionTests.test_lists
// Ported from Django: tests/template_tests/filter_tests/test_pluralize.py::FunctionTests.test_suffixes
// Ported from Django: tests/template_tests/filter_tests/test_pluralize.py::FunctionTests.test_value_error
func TestPluralizeFunction(t *testing.T) {
	runRenderCases(t, []renderCase{
		{name: "test_integers_1", src: "{{ a|pluralize }}", ctx: map[string]any{"a": 1}, want: ""},
		{name: "test_integers_2", src: "{{ a|pluralize }}", ctx: map[string]any{"a": 0}, want: "s"},
		{name: "test_integers_3", src: "{{ a|pluralize }}", ctx: map[string]any{"a": 2}, want: "s"},
		{name: "test_floats_1", src: "{{ a|pluralize }}", ctx: map[string]any{"a": 0.5}, want: "s"},
		{name: "test_floats_2", src: "{{ a|pluralize }}", ctx: map[string]any{"a": 1.5}, want: "s"},
		{name: "test_lists_1", src: "{{ a|pluralize }}", ctx: map[string]any{"a": []any{1}}, want: ""},
		{name: "test_lists_2", src: "{{ a|pluralize }}", ctx: map[string]any{"a": []any{}}, want: "s"},
		{name: "test_lists_3", src: "{{ a|pluralize }}", ctx: map[string]any{"a": []any{1, 2, 3}}, want: "s"},
		{name: "test_suffixes_1", src: "{{ a|pluralize:b }}", ctx: map[string]any{"a": 1, "b": "es"}, want: ""},
		{name: "test_suffixes_2", src: "{{ a|pluralize:b }}", ctx: map[string]any{"a": 0, "b": "es"}, want: "es"},
		{name: "test_suffixes_3", src: "{{ a|pluralize:b }}", ctx: map[string]any{"a": 2, "b": "es"}, want: "es"},
		{name: "test_suffixes_4", src: "{{ a|pluralize:b }}", ctx: map[string]any{"a": 1, "b": "y,ies"}, want: "y"},
		{name: "test_suffixes_5", src: "{{ a|pluralize:b }}", ctx: map[string]any{"a": 0, "b": "y,ies"}, want: "ies"},
		{name: "test_suffixes_6", src: "{{ a|pluralize:b }}", ctx: map[string]any{"a": 2, "b": "y,ies"}, want: "ies"},
		{name: "test_suffixes_7", src: "{{ a|pluralize:b }}", ctx: map[string]any{"a": 0, "b": "y,ies,error"}, want: ""},
		{name: "test_value_error_1", src: "{{ a|pluralize:b }}", ctx: map[string]any{"a": "", "b": "y,es"}, want: ""},
		{name: "test_value_error_2", src: "{{ a|pluralize:b }}", ctx: map[string]any{"a": "", "b": "es"}, want: ""},
	})
}
