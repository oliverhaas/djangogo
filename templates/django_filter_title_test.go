package templates_test

import (
	"testing"
)

// TestTitle ports Django's TitleTests.
//
// Ported from Django: tests/template_tests/filter_tests/test_title.py::TitleTests.test_title1
// Ported from Django: tests/template_tests/filter_tests/test_title.py::TitleTests.test_title2
func TestTitle(t *testing.T) {
	runRenderCases(t, []renderCase{
		{name: "test_title1", src: "{{ a|title }}", ctx: map[string]any{"a": "JOE'S CRAB SHACK"}, want: "Joe&#x27;s Crab Shack"},
		{name: "test_title2", src: "{{ a|title }}", ctx: map[string]any{"a": "555 WEST 53RD STREET"}, want: "555 West 53rd Street"},
	})
}

// TestTitleFunction ports Django's FunctionTests.
//
// Ported from Django: tests/template_tests/filter_tests/test_title.py::FunctionTests.test_unicode
// Ported from Django: tests/template_tests/filter_tests/test_title.py::FunctionTests.test_non_string_input
func TestTitleFunction(t *testing.T) {
	runRenderCases(t, []renderCase{
		{name: "test_unicode", src: "{{ a|title }}", ctx: map[string]any{"a": "discothèque"}, want: "Discothèque"},
		{name: "test_non_string_input", src: "{{ a|title }}", ctx: map[string]any{"a": 123}, want: "123"},
	})
}
