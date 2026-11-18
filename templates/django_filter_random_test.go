package templates_test

import (
	"testing"

	"github.com/oliverhaas/djangogo/templates"
)

// TestRandom ports Django's RandomTests.
//
// Ported from Django: tests/template_tests/filter_tests/test_random.py::RandomTests.test_random01
// Ported from Django: tests/template_tests/filter_tests/test_random.py::RandomTests.test_random02
// Ported from Django: tests/template_tests/filter_tests/test_random.py::RandomTests.test_empty_list
func TestRandom(t *testing.T) {
	runRenderCases(t, []renderCase{
		{name: "test_random01", src: "{{ a|random }} {{ b|random }}", ctx: map[string]any{"a": []any{"a&b", "a&b"}, "b": []any{templates.MarkSafe("a&b"), templates.MarkSafe("a&b")}}, want: "a&amp;b a&b"},
		{name: "test_random02", src: "{% autoescape off %}{{ a|random }} {{ b|random }}{% endautoescape %}", ctx: map[string]any{"a": []any{"a&b", "a&b"}, "b": []any{templates.MarkSafe("a&b"), templates.MarkSafe("a&b")}}, want: "a&b a&b"},
		{name: "test_empty_list", src: "{{ list|random }}", ctx: map[string]any{"list": []any{}}, want: ""},
	})
}
