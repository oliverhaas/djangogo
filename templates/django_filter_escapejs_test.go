package templates_test

import (
	"testing"
)

// TestEscapejs ports Django's EscapejsTests.
//
// Ported from Django: tests/template_tests/filter_tests/test_escapejs.py::EscapejsTests.test_escapejs01
// Ported from Django: tests/template_tests/filter_tests/test_escapejs.py::EscapejsTests.test_escapejs02
func TestEscapejs(t *testing.T) {
	runRenderCases(t, []renderCase{
		{name: "test_escapejs01", src: "{{ a|escapejs }}", ctx: map[string]any{"a": "testing\r\njavascript 'string\" <b>escaping</b>"}, want: `testing\u000D\u000Ajavascript \u0027string\u0022 \u003Cb\u003Eescaping\u003C/b\u003E`},
		{name: "test_escapejs02", src: "{% autoescape off %}{{ a|escapejs }}{% endautoescape %}", ctx: map[string]any{"a": "testing\r\njavascript 'string\" <b>escaping</b>"}, want: `testing\u000D\u000Ajavascript \u0027string\u0022 \u003Cb\u003Eescaping\u003C/b\u003E`},
	})
}

// TestEscapejsFunction ports Django's FunctionTests.
//
// Ported from Django: tests/template_tests/filter_tests/test_escapejs.py::FunctionTests.test_backslashes
// Ported from Django: tests/template_tests/filter_tests/test_escapejs.py::FunctionTests.test_whitespace
// Ported from Django: tests/template_tests/filter_tests/test_escapejs.py::FunctionTests.test_paragraph_separator
func TestEscapejsFunction(t *testing.T) {
	runRenderCases(t, []renderCase{
		{name: "test_backslashes", src: "{{ a|escapejs }}", ctx: map[string]any{"a": `\ : backslashes, too`}, want: `\u005C : backslashes, too`},
		{name: "test_whitespace", src: "{{ a|escapejs }}", ctx: map[string]any{"a": "and lots of whitespace: \r\n\t\u000b\u000c\u0008"}, want: `and lots of whitespace: \u000D\u000A\u0009\u000B\u000C\u0008`},
		{name: "test_paragraph_separator", src: "{{ a|escapejs }}", ctx: map[string]any{"a": "paragraph separator:\u2029and line separator:\u2028"}, want: `paragraph separator:\u2029and line separator:\u2028`},
	})
}
