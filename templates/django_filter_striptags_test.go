package templates_test

import (
	"testing"

	"github.com/oliverhaas/djangogo/templates"
)

// TestStriptags ports Django's StriptagsTests.
//
// Ported from Django: tests/template_tests/filter_tests/test_striptags.py::StriptagsTests.test_striptags01
// Ported from Django: tests/template_tests/filter_tests/test_striptags.py::StriptagsTests.test_striptags02
func TestStriptags(t *testing.T) {
	runRenderCases(t, []renderCase{
		{name: "test_striptags01", src: "{{ a|striptags }} {{ b|striptags }}", ctx: map[string]any{"a": "<a>x</a> <p><b>y</b></p>", "b": templates.MarkSafe("<a>x</a> <p><b>y</b></p>")}, want: "x y x y"},
		{name: "test_striptags02", src: "{% autoescape off %}{{ a|striptags }} {{ b|striptags }}{% endautoescape %}", ctx: map[string]any{"a": "<a>x</a> <p><b>y</b></p>", "b": templates.MarkSafe("<a>x</a> <p><b>y</b></p>")}, want: "x y x y"},
	})
}

// TestStriptagsFunction ports Django's FunctionTests.
//
// Ported from Django: tests/template_tests/filter_tests/test_striptags.py::FunctionTests.test_non_string_input
func TestStriptagsFunction(t *testing.T) {
	runRenderCases(t, []renderCase{
		{name: "test_non_string_input", src: "{{ a|striptags }}", ctx: map[string]any{"a": 123}, want: "123"},
	})
}
