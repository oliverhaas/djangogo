package templates_test

import (
	"testing"
)

// TestDefaultIfNoneFunction ports Django's FunctionTests.
//
// Ported from Django: tests/template_tests/filter_tests/test_default_if_none.py::FunctionTests.test_value
// Ported from Django: tests/template_tests/filter_tests/test_default_if_none.py::FunctionTests.test_none
// Ported from Django: tests/template_tests/filter_tests/test_default_if_none.py::FunctionTests.test_empty_string
func TestDefaultIfNoneFunction(t *testing.T) {
	runRenderCases(t, []renderCase{
		{name: "test_value", src: "{{ a|default_if_none:b }}", ctx: map[string]any{"a": "val", "b": "default"}, want: "val"},
		{name: "test_none", src: "{{ a|default_if_none:b }}", ctx: map[string]any{"a": nil, "b": "default"}, want: "default"},
		{name: "test_empty_string", src: "{{ a|default_if_none:b }}", ctx: map[string]any{"a": "", "b": "default"}, want: ""},
	})
}
