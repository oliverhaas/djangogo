package templates_test

import (
	"testing"
)

// TestUrlencode ports Django's UrlencodeTests.
//
// Ported from Django: tests/template_tests/filter_tests/test_urlencode.py::UrlencodeTests.test_urlencode01
// Ported from Django: tests/template_tests/filter_tests/test_urlencode.py::UrlencodeTests.test_urlencode02
func TestUrlencode(t *testing.T) {
	runRenderCases(t, []renderCase{
		{name: "test_urlencode01", src: "{{ url|urlencode }}", ctx: map[string]any{"url": `/test&"/me?/`}, want: "/test%26%22/me%3F/"},
		{name: "test_urlencode02", src: `/test/{{ urlbit|urlencode:"" }}/`, ctx: map[string]any{"urlbit": "escape/slash"}, want: "/test/escape%2Fslash/"},
	})
}

// TestUrlencodeFunction ports Django's FunctionTests.
//
// Ported from Django: tests/template_tests/filter_tests/test_urlencode.py::FunctionTests.test_non_string_input
func TestUrlencodeFunction(t *testing.T) {
	runRenderCases(t, []renderCase{
		{name: "test_non_string_input", src: "{{ a|urlencode }}", ctx: map[string]any{"a": 1}, want: "1"},
	})
}
