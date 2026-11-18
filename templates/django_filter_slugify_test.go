package templates_test

import (
	"testing"

	"github.com/oliverhaas/djangogo/templates"
)

// TestSlugify ports Django's SlugifyTests.
//
// Ported from Django: tests/template_tests/filter_tests/test_slugify.py::SlugifyTests.test_slugify01
// Ported from Django: tests/template_tests/filter_tests/test_slugify.py::SlugifyTests.test_slugify02
func TestSlugify(t *testing.T) {
	runRenderCases(t, []renderCase{
		{name: "test_slugify01", src: "{% autoescape off %}{{ a|slugify }} {{ b|slugify }}{% endautoescape %}", ctx: map[string]any{"a": "a & b", "b": templates.MarkSafe("a &amp; b")}, want: "a-b a-amp-b"},
		{name: "test_slugify02", src: "{{ a|slugify }} {{ b|slugify }}", ctx: map[string]any{"a": "a & b", "b": templates.MarkSafe("a &amp; b")}, want: "a-b a-amp-b"},
	})
}

// TestSlugifyFunction ports Django's FunctionTests.
//
// Ported from Django: tests/template_tests/filter_tests/test_slugify.py::FunctionTests.test_non_string_input
func TestSlugifyFunction(t *testing.T) {
	runRenderCases(t, []renderCase{
		{name: "test_non_string_input", src: "{{ a|slugify }}", ctx: map[string]any{"a": 123}, want: "123"},
	})
}
