package templates_test

import (
	"testing"
)

// TestAdd ports Django's AddTests.
//
// Ported from Django: tests/template_tests/filter_tests/test_add.py::AddTests.test_add01
// Ported from Django: tests/template_tests/filter_tests/test_add.py::AddTests.test_add02
// Ported from Django: tests/template_tests/filter_tests/test_add.py::AddTests.test_add03
// Ported from Django: tests/template_tests/filter_tests/test_add.py::AddTests.test_add04
// Ported from Django: tests/template_tests/filter_tests/test_add.py::AddTests.test_add05
// Ported from Django: tests/template_tests/filter_tests/test_add.py::AddTests.test_add06
func TestAdd(t *testing.T) {
	runRenderCases(t, []renderCase{
		{name: "test_add01", src: `{{ i|add:"5" }}`, ctx: map[string]any{"i": 2000}, want: "2005"},
		{name: "test_add02", src: `{{ i|add:"napis" }}`, ctx: map[string]any{"i": 2000}, want: ""},
		{name: "test_add03", src: "{{ i|add:16 }}", ctx: map[string]any{"i": "not_an_int"}, want: ""},
		{name: "test_add04", src: `{{ i|add:"16" }}`, ctx: map[string]any{"i": "not_an_int"}, want: "not_an_int16"},
		{name: "test_add05", src: "{{ l1|add:l2 }}", ctx: map[string]any{"l1": []any{1, 2}, "l2": []any{3, 4}}, want: "[1, 2, 3, 4]", skip: "add does not concatenate lists, see fidelity/divergences.md"},
		{name: "test_add06", src: "{{ t1|add:t2 }}", ctx: map[string]any{"t1": []any{3, 4}, "t2": []any{1, 2}}, want: "(3, 4, 1, 2)", skip: "add does not concatenate tuples, see fidelity/divergences.md"},
	})
}

// TestAddFunction ports Django's FunctionTests.
//
// Ported from Django: tests/template_tests/filter_tests/test_add.py::FunctionTests.test_add
func TestAddFunction(t *testing.T) {
	runRenderCases(t, []renderCase{
		{name: "test_add", src: "{{ a|add:b }}", ctx: map[string]any{"a": "1", "b": "2"}, want: "3"},
	})
}
