package templates_test

import (
	"testing"
)

// TestTruncatewordsHtmlFunction ports Django's FunctionTests.
//
// Ported from Django: tests/template_tests/filter_tests/test_truncatewords_html.py::FunctionTests.test_truncate_unicode
func TestTruncatewordsHtmlFunction(t *testing.T) {
	runRenderCases(t, []renderCase{
		{name: "test_truncate_unicode", src: "{{ a|truncatewords_html:b }}", ctx: map[string]any{"a": "Ångström was here", "b": 1}, want: "Ångström …"},
	})
}
