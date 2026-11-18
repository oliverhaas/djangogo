package templates_test

import (
	"testing"

	"github.com/oliverhaas/djangogo/templates"
)

// TestUnorderedList ports Django's UnorderedListTests.
//
// Ported from Django: tests/template_tests/filter_tests/test_unordered_list.py::UnorderedListTests.test_unordered_list01
// Ported from Django: tests/template_tests/filter_tests/test_unordered_list.py::UnorderedListTests.test_unordered_list02
// Ported from Django: tests/template_tests/filter_tests/test_unordered_list.py::UnorderedListTests.test_unordered_list03
// Ported from Django: tests/template_tests/filter_tests/test_unordered_list.py::UnorderedListTests.test_unordered_list04
// Ported from Django: tests/template_tests/filter_tests/test_unordered_list.py::UnorderedListTests.test_unordered_list05
func TestUnorderedList(t *testing.T) {
	runRenderCases(t, []renderCase{
		{name: "test_unordered_list01", src: "{{ a|unordered_list }}", ctx: map[string]any{"a": []any{"x>", []any{"<y"}}}, want: "\t<li>x&gt;\n\t<ul>\n\t\t<li>&lt;y</li>\n\t</ul>\n\t</li>"},
		{name: "test_unordered_list02", src: "{% autoescape off %}{{ a|unordered_list }}{% endautoescape %}", ctx: map[string]any{"a": []any{"x>", []any{"<y"}}}, want: "\t<li>x>\n\t<ul>\n\t\t<li><y</li>\n\t</ul>\n\t</li>"},
		{name: "test_unordered_list03", src: "{{ a|unordered_list }}", ctx: map[string]any{"a": []any{"x>", []any{templates.MarkSafe("<y")}}}, want: "\t<li>x&gt;\n\t<ul>\n\t\t<li><y</li>\n\t</ul>\n\t</li>"},
		{name: "test_unordered_list04", src: "{% autoescape off %}{{ a|unordered_list }}{% endautoescape %}", ctx: map[string]any{"a": []any{"x>", []any{templates.MarkSafe("<y")}}}, want: "\t<li>x>\n\t<ul>\n\t\t<li><y</li>\n\t</ul>\n\t</li>"},
		{name: "test_unordered_list05", src: "{% autoescape off %}{{ a|unordered_list }}{% endautoescape %}", ctx: map[string]any{"a": []any{"x>", []any{"<y"}}}, want: "\t<li>x>\n\t<ul>\n\t\t<li><y</li>\n\t</ul>\n\t</li>"},
	})
}
