package templates_test

import (
	"testing"
)

// TestDictsortreversedFunction ports Django's FunctionTests.
//
// Ported from Django: tests/template_tests/filter_tests/test_dictsortreversed.py::FunctionTests.test_invalid_values
// Ported from Django: tests/template_tests/filter_tests/test_dictsortreversed.py::FunctionTests.test_invalid_args
func TestDictsortreversedFunction(t *testing.T) {
	runRenderCases(t, []renderCase{
		{name: "test_invalid_values_1", src: "{{ a|dictsortreversed:b }}", ctx: map[string]any{"a": []any{1, 2, 3}, "b": "age"}, want: ""},
		{name: "test_invalid_values_2", src: "{{ a|dictsortreversed:b }}", ctx: map[string]any{"a": "Hello!", "b": "age"}, want: ""},
		{name: "test_invalid_values_3", src: "{{ a|dictsortreversed:b }}", ctx: map[string]any{"a": map[string]any{"a": 1}, "b": "age"}, want: ""},
		{name: "test_invalid_values_4", src: "{{ a|dictsortreversed:b }}", ctx: map[string]any{"a": 1, "b": "age"}, want: ""},
		{name: "test_invalid_args_1", src: "{{ a|dictsortreversed:b }}", ctx: map[string]any{"a": []any{map[string]any{}}, "b": "._private"}, want: ""},
		{name: "test_invalid_args_2", src: "{{ a|dictsortreversed:b }}", ctx: map[string]any{"a": []any{map[string]any{"_private": "test"}}, "b": "_private"}, want: ""},
		{name: "test_invalid_args_3", src: "{{ a|dictsortreversed:b }}", ctx: map[string]any{"a": []any{map[string]any{"nested": map[string]any{"_private": "test"}}}, "b": "nested._private"}, want: ""},
	})
}
