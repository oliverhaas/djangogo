package templates_test

import (
	"testing"

	"github.com/oliverhaas/djangogo/templates"
)

// TestSlice ports Django's SliceTests.
//
// Ported from Django: tests/template_tests/filter_tests/test_slice.py::SliceTests.test_slice01
// Ported from Django: tests/template_tests/filter_tests/test_slice.py::SliceTests.test_slice02
func TestSlice(t *testing.T) {
	runRenderCases(t, []renderCase{
		{name: "test_slice01", src: `{{ a|slice:"1:3" }} {{ b|slice:"1:3" }}`, ctx: map[string]any{"a": "a&b", "b": templates.MarkSafe("a&b")}, want: "&amp;b &b"},
		{name: "test_slice02", src: `{% autoescape off %}{{ a|slice:"1:3" }} {{ b|slice:"1:3" }}{% endautoescape %}`, ctx: map[string]any{"a": "a&b", "b": templates.MarkSafe("a&b")}, want: "&b &b"},
	})
}

// TestSliceFunction ports Django's FunctionTests.
//
// Ported from Django: tests/template_tests/filter_tests/test_slice.py::FunctionTests.test_zero_length
// Ported from Django: tests/template_tests/filter_tests/test_slice.py::FunctionTests.test_index
// Ported from Django: tests/template_tests/filter_tests/test_slice.py::FunctionTests.test_index_integer
// Ported from Django: tests/template_tests/filter_tests/test_slice.py::FunctionTests.test_negative_index
// Ported from Django: tests/template_tests/filter_tests/test_slice.py::FunctionTests.test_range
// Ported from Django: tests/template_tests/filter_tests/test_slice.py::FunctionTests.test_range_multiple
// Ported from Django: tests/template_tests/filter_tests/test_slice.py::FunctionTests.test_range_step
func TestSliceFunction(t *testing.T) {
	runRenderCases(t, []renderCase{
		{name: "test_zero_length", src: "{{ a|slice:b }}", ctx: map[string]any{"a": "abcdefg", "b": "0"}, want: ""},
		{name: "test_index", src: "{{ a|slice:b }}", ctx: map[string]any{"a": "abcdefg", "b": "1"}, want: "a"},
		{name: "test_index_integer", src: "{{ a|slice:b }}", ctx: map[string]any{"a": "abcdefg", "b": 1}, want: "a"},
		{name: "test_negative_index", src: "{{ a|slice:b }}", ctx: map[string]any{"a": "abcdefg", "b": "-1"}, want: "abcdef"},
		{name: "test_range", src: "{{ a|slice:b }}", ctx: map[string]any{"a": "abcdefg", "b": "1:2"}, want: "b"},
		{name: "test_range_multiple", src: "{{ a|slice:b }}", ctx: map[string]any{"a": "abcdefg", "b": "1:3"}, want: "bc"},
		{name: "test_range_step", src: "{{ a|slice:b }}", ctx: map[string]any{"a": "abcdefg", "b": "0::2"}, want: "aceg"},
	})
}
