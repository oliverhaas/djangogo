package templates_test

import (
	"testing"

	"github.com/oliverhaas/djangogo/templates"
)

// TestMakeList ports Django's MakeListTests.
//
// Ported from Django: tests/template_tests/filter_tests/test_make_list.py::MakeListTests.test_make_list01
// Ported from Django: tests/template_tests/filter_tests/test_make_list.py::MakeListTests.test_make_list02
// Ported from Django: tests/template_tests/filter_tests/test_make_list.py::MakeListTests.test_make_list03
// Ported from Django: tests/template_tests/filter_tests/test_make_list.py::MakeListTests.test_make_list04
func TestMakeList(t *testing.T) {
	runRenderCases(t, []renderCase{
		{name: "test_make_list01", src: "{% autoescape off %}{{ a|make_list }}{% endautoescape %}", ctx: map[string]any{"a": templates.MarkSafe("&")}, want: "['&']"},
		{name: "test_make_list02", src: "{{ a|make_list }}", ctx: map[string]any{"a": templates.MarkSafe("&")}, want: "[&#x27;&amp;&#x27;]"},
		{name: "test_make_list03", src: `{% autoescape off %}{{ a|make_list|stringformat:"s"|safe }}{% endautoescape %}`, ctx: map[string]any{"a": templates.MarkSafe("&")}, want: "['&']"},
		{name: "test_make_list04", src: `{{ a|make_list|stringformat:"s"|safe }}`, ctx: map[string]any{"a": templates.MarkSafe("&")}, want: "['&']"},
	})
}
