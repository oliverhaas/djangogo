package templates_test

import (
	"testing"
)

// TestFirstOfTag ports Django's FirstOfTagTests.
//
// Ported from Django: tests/template_tests/syntax_tests/test_firstof.py::FirstOfTagTests.test_firstof01
// Ported from Django: tests/template_tests/syntax_tests/test_firstof.py::FirstOfTagTests.test_firstof02
// Ported from Django: tests/template_tests/syntax_tests/test_firstof.py::FirstOfTagTests.test_firstof03
// Ported from Django: tests/template_tests/syntax_tests/test_firstof.py::FirstOfTagTests.test_firstof04
// Ported from Django: tests/template_tests/syntax_tests/test_firstof.py::FirstOfTagTests.test_firstof05
// Ported from Django: tests/template_tests/syntax_tests/test_firstof.py::FirstOfTagTests.test_firstof06
// Ported from Django: tests/template_tests/syntax_tests/test_firstof.py::FirstOfTagTests.test_firstof07
// Ported from Django: tests/template_tests/syntax_tests/test_firstof.py::FirstOfTagTests.test_firstof08
// Ported from Django: tests/template_tests/syntax_tests/test_firstof.py::FirstOfTagTests.test_firstof09
// Ported from Django: tests/template_tests/syntax_tests/test_firstof.py::FirstOfTagTests.test_firstof10
// Ported from Django: tests/template_tests/syntax_tests/test_firstof.py::FirstOfTagTests.test_firstof11
// Ported from Django: tests/template_tests/syntax_tests/test_firstof.py::FirstOfTagTests.test_firstof12
// Ported from Django: tests/template_tests/syntax_tests/test_firstof.py::FirstOfTagTests.test_firstof13
// Ported from Django: tests/template_tests/syntax_tests/test_firstof.py::FirstOfTagTests.test_firstof14
func TestFirstOfTag(t *testing.T) {
	runRenderCases(t, []renderCase{
		{name: "test_firstof01", src: "{% firstof a b c %}", ctx: map[string]any{"a": 0, "c": 0, "b": 0}, want: ""},
		{name: "test_firstof02", src: "{% firstof a b c %}", ctx: map[string]any{"a": 1, "c": 0, "b": 0}, want: "1"},
		{name: "test_firstof03", src: "{% firstof a b c %}", ctx: map[string]any{"a": 0, "c": 0, "b": 2}, want: "2"},
		{name: "test_firstof04", src: "{% firstof a b c %}", ctx: map[string]any{"a": 0, "c": 3, "b": 0}, want: "3"},
		{name: "test_firstof05", src: "{% firstof a b c %}", ctx: map[string]any{"a": 1, "c": 3, "b": 2}, want: "1"},
		{name: "test_firstof06", src: "{% firstof a b c %}", ctx: map[string]any{"c": 3, "b": 0}, want: "3"},
		{name: "test_firstof07", src: `{% firstof a b "c" %}`, ctx: map[string]any{"a": 0}, want: "c"},
		{name: "test_firstof08", src: `{% firstof a b "c and d" %}`, ctx: map[string]any{"a": 0, "b": 0}, want: "c and d"},
		{name: "test_firstof09", src: "{% firstof %}", wantErr: true},
		{name: "test_firstof10", src: "{% firstof a %}", ctx: map[string]any{"a": "<"}, want: "&lt;"},
		{name: "test_firstof11", src: "{% firstof a b %}", ctx: map[string]any{"a": "<", "b": ">"}, want: "&lt;"},
		{name: "test_firstof12", src: "{% firstof a b %}", ctx: map[string]any{"a": "", "b": ">"}, want: "&gt;"},
		{name: "test_firstof13", src: "{% autoescape off %}{% firstof a %}{% endautoescape %}", ctx: map[string]any{"a": "<"}, want: "<"},
		{name: "test_firstof14", src: "{% firstof a|safe b %}", ctx: map[string]any{"a": "<"}, want: "<"},
	})
}
