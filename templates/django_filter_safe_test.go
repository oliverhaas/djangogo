package templates_test

import (
	"testing"
)

// TestSafe ports Django's SafeTests.
//
// Ported from Django: tests/template_tests/filter_tests/test_safe.py::SafeTests.test_safe01
// Ported from Django: tests/template_tests/filter_tests/test_safe.py::SafeTests.test_safe02
func TestSafe(t *testing.T) {
	runRenderCases(t, []renderCase{
		{name: "test_safe01", src: "{{ a }} -- {{ a|safe }}", ctx: map[string]any{"a": "<b>hello</b>"}, want: "&lt;b&gt;hello&lt;/b&gt; -- <b>hello</b>"},
		{name: "test_safe02", src: "{% autoescape off %}{{ a }} -- {{ a|safe }}{% endautoescape %}", ctx: map[string]any{"a": "<b>hello</b>"}, want: "<b>hello</b> -- <b>hello</b>"},
	})
}
