package templates_test

import (
	"testing"

	"github.com/oliverhaas/djangogo/templates"
)

// TestChaining ports Django's ChainingTests.
//
// Ported from Django: tests/template_tests/filter_tests/test_chaining.py::ChainingTests.test_chaining01
// Ported from Django: tests/template_tests/filter_tests/test_chaining.py::ChainingTests.test_chaining02
// Ported from Django: tests/template_tests/filter_tests/test_chaining.py::ChainingTests.test_chaining03
// Ported from Django: tests/template_tests/filter_tests/test_chaining.py::ChainingTests.test_chaining04
// Ported from Django: tests/template_tests/filter_tests/test_chaining.py::ChainingTests.test_chaining05
// Ported from Django: tests/template_tests/filter_tests/test_chaining.py::ChainingTests.test_chaining06
// Ported from Django: tests/template_tests/filter_tests/test_chaining.py::ChainingTests.test_chaining07
// Ported from Django: tests/template_tests/filter_tests/test_chaining.py::ChainingTests.test_chaining08
// Ported from Django: tests/template_tests/filter_tests/test_chaining.py::ChainingTests.test_chaining09
// Ported from Django: tests/template_tests/filter_tests/test_chaining.py::ChainingTests.test_chaining10
// Ported from Django: tests/template_tests/filter_tests/test_chaining.py::ChainingTests.test_chaining11
// Ported from Django: tests/template_tests/filter_tests/test_chaining.py::ChainingTests.test_chaining12
// Ported from Django: tests/template_tests/filter_tests/test_chaining.py::ChainingTests.test_chaining13
// Ported from Django: tests/template_tests/filter_tests/test_chaining.py::ChainingTests.test_chaining14
func TestChaining(t *testing.T) {
	runRenderCases(t, []renderCase{
		{name: "test_chaining01", src: `{{ a|capfirst|center:"7" }}.{{ b|capfirst|center:"7" }}`, ctx: map[string]any{"a": "a < b", "b": templates.MarkSafe("a < b")}, want: " A &lt; b . A < b "},
		{name: "test_chaining02", src: `{% autoescape off %}{{ a|capfirst|center:"7" }}.{{ b|capfirst|center:"7" }}{% endautoescape %}`, ctx: map[string]any{"a": "a < b", "b": templates.MarkSafe("a < b")}, want: " A < b . A < b "},
		{name: "test_chaining03", src: `{{ a|cut:"b"|capfirst }}.{{ b|cut:"b"|capfirst }}`, ctx: map[string]any{"a": "a < b", "b": templates.MarkSafe("a < b")}, want: "A &lt; .A < "},
		{name: "test_chaining04", src: `{% autoescape off %}{{ a|cut:"b"|capfirst }}.{{ b|cut:"b"|capfirst }}{% endautoescape %}`, ctx: map[string]any{"a": "a < b", "b": templates.MarkSafe("a < b")}, want: "A < .A < "},
		{name: "test_chaining05", src: "{{ a|escape|capfirst }}", ctx: map[string]any{"a": "a < b"}, want: "A &lt; b"},
		{name: "test_chaining06", src: "{% autoescape off %}{{ a|escape|capfirst }}{% endautoescape %}", ctx: map[string]any{"a": "a < b"}, want: "A &lt; b"},
		{name: "test_chaining07", src: `{{ a|force_escape|cut:";" }}`, ctx: map[string]any{"a": "a < b"}, want: "a &amp;lt b"},
		{name: "test_chaining08", src: `{% autoescape off %}{{ a|force_escape|cut:";" }}{% endautoescape %}`, ctx: map[string]any{"a": "a < b"}, want: "a &lt b"},
		{name: "test_chaining09", src: `{{ a|cut:";"|force_escape }}`, ctx: map[string]any{"a": "a < b"}, want: "a &lt; b"},
		{name: "test_chaining10", src: `{% autoescape off %}{{ a|cut:";"|force_escape }}{% endautoescape %}`, ctx: map[string]any{"a": "a < b"}, want: "a &lt; b"},
		{name: "test_chaining11", src: `{{ a|cut:"b"|safe }}`, ctx: map[string]any{"a": "a < b"}, want: "a < "},
		{name: "test_chaining12", src: `{% autoescape off %}{{ a|cut:"b"|safe }}{% endautoescape %}`, ctx: map[string]any{"a": "a < b"}, want: "a < "},
		{name: "test_chaining13", src: "{{ a|safe|force_escape }}", ctx: map[string]any{"a": "a < b"}, want: "a &lt; b"},
		{name: "test_chaining14", src: "{% autoescape off %}{{ a|safe|force_escape }}{% endautoescape %}", ctx: map[string]any{"a": "a < b"}, want: "a &lt; b"},
	})
}
