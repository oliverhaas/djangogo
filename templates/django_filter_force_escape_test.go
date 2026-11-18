package templates_test

import (
	"testing"
)

// TestForceEscape ports Django's ForceEscapeTests.
//
// Ported from Django: tests/template_tests/filter_tests/test_force_escape.py::ForceEscapeTests.test_force_escape01
// Ported from Django: tests/template_tests/filter_tests/test_force_escape.py::ForceEscapeTests.test_force_escape02
// Ported from Django: tests/template_tests/filter_tests/test_force_escape.py::ForceEscapeTests.test_force_escape03
// Ported from Django: tests/template_tests/filter_tests/test_force_escape.py::ForceEscapeTests.test_force_escape04
// Ported from Django: tests/template_tests/filter_tests/test_force_escape.py::ForceEscapeTests.test_force_escape05
// Ported from Django: tests/template_tests/filter_tests/test_force_escape.py::ForceEscapeTests.test_force_escape06
// Ported from Django: tests/template_tests/filter_tests/test_force_escape.py::ForceEscapeTests.test_force_escape07
// Ported from Django: tests/template_tests/filter_tests/test_force_escape.py::ForceEscapeTests.test_force_escape08
func TestForceEscape(t *testing.T) {
	runRenderCases(t, []renderCase{
		{name: "test_force_escape01", src: "{% autoescape off %}{{ a|force_escape }}{% endautoescape %}", ctx: map[string]any{"a": "x&y"}, want: "x&amp;y"},
		{name: "test_force_escape02", src: "{{ a|force_escape }}", ctx: map[string]any{"a": "x&y"}, want: "x&amp;y"},
		{name: "test_force_escape03", src: "{% autoescape off %}{{ a|force_escape|force_escape }}{% endautoescape %}", ctx: map[string]any{"a": "x&y"}, want: "x&amp;amp;y"},
		{name: "test_force_escape04", src: "{{ a|force_escape|force_escape }}", ctx: map[string]any{"a": "x&y"}, want: "x&amp;amp;y"},
		{name: "test_force_escape05", src: "{% autoescape off %}{{ a|force_escape|escape }}{% endautoescape %}", ctx: map[string]any{"a": "x&y"}, want: "x&amp;y"},
		{name: "test_force_escape06", src: "{{ a|force_escape|escape }}", ctx: map[string]any{"a": "x&y"}, want: "x&amp;y"},
		{name: "test_force_escape07", src: "{% autoescape off %}{{ a|escape|force_escape }}{% endautoescape %}", ctx: map[string]any{"a": "x&y"}, want: "x&amp;amp;y"},
		{name: "test_force_escape08", src: "{{ a|escape|force_escape }}", ctx: map[string]any{"a": "x&y"}, want: "x&amp;amp;y"},
	})
}
