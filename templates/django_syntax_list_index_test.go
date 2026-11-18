package templates_test

import (
	"testing"
)

// TestListIndex ports Django's ListIndexTests.
//
// Ported from Django: tests/template_tests/syntax_tests/test_list_index.py::ListIndexTests.test_list_index01
// Ported from Django: tests/template_tests/syntax_tests/test_list_index.py::ListIndexTests.test_list_index02
// Ported from Django: tests/template_tests/syntax_tests/test_list_index.py::ListIndexTests.test_list_index03
// Ported from Django: tests/template_tests/syntax_tests/test_list_index.py::ListIndexTests.test_list_index04
// Ported from Django: tests/template_tests/syntax_tests/test_list_index.py::ListIndexTests.test_list_index05
func TestListIndex(t *testing.T) {
	runRenderCases(t, []renderCase{
		{name: "test_list_index01", src: "{{ var.1 }}", ctx: map[string]any{"var": []any{"first item", "second item"}}, want: "second item"},
		{name: "test_list_index02", src: "{{ var.5 }}", ctx: map[string]any{"var": []any{"first item", "second item"}}, want: ""},
		{name: "test_list_index03", src: "{{ var.1 }}", ctx: map[string]any{"var": nil}, want: ""},
		{name: "test_list_index04", src: "{{ var.1 }}", ctx: map[string]any{"var": map[string]any{}}, want: "", skip: `{{ var.1 }} does not look up the map key "1", see fidelity/divergences.md`},
		{name: "test_list_index05", src: "{{ var.1 }}", ctx: map[string]any{"var": map[string]any{"1": "hello"}}, want: "hello", skip: `{{ var.1 }} does not look up the map key "1", see fidelity/divergences.md`},
	})
}
