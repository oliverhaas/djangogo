package templates_test

import (
	"testing"

	"github.com/oliverhaas/djangogo/templates"
)

// TestJoin ports Django's JoinTests.
//
// Ported from Django: tests/template_tests/filter_tests/test_join.py::JoinTests.test_join01
// Ported from Django: tests/template_tests/filter_tests/test_join.py::JoinTests.test_join02
// Ported from Django: tests/template_tests/filter_tests/test_join.py::JoinTests.test_join03
// Ported from Django: tests/template_tests/filter_tests/test_join.py::JoinTests.test_join04
// Ported from Django: tests/template_tests/filter_tests/test_join.py::JoinTests.test_join05
// Ported from Django: tests/template_tests/filter_tests/test_join.py::JoinTests.test_join06
// Ported from Django: tests/template_tests/filter_tests/test_join.py::JoinTests.test_join07
// Ported from Django: tests/template_tests/filter_tests/test_join.py::JoinTests.test_join08
func TestJoin(t *testing.T) {
	runRenderCases(t, []renderCase{
		{name: "test_join01", src: `{{ a|join:", " }}`, ctx: map[string]any{"a": []any{"alpha", "beta & me"}}, want: "alpha, beta &amp; me"},
		{name: "test_join02", src: `{% autoescape off %}{{ a|join:", " }}{% endautoescape %}`, ctx: map[string]any{"a": []any{"alpha", "beta & me"}}, want: "alpha, beta & me"},
		{name: "test_join03", src: `{{ a|join:" &amp; " }}`, ctx: map[string]any{"a": []any{"alpha", "beta & me"}}, want: "alpha &amp; beta &amp; me"},
		{name: "test_join04", src: `{% autoescape off %}{{ a|join:" &amp; " }}{% endautoescape %}`, ctx: map[string]any{"a": []any{"alpha", "beta & me"}}, want: "alpha &amp; beta & me"},
		{name: "test_join05", src: "{{ a|join:var }}", ctx: map[string]any{"a": []any{"alpha", "beta & me"}, "var": " & "}, want: "alpha &amp; beta &amp; me"},
		{name: "test_join06", src: "{{ a|join:var }}", ctx: map[string]any{"a": []any{"alpha", "beta & me"}, "var": templates.MarkSafe(" & ")}, want: "alpha & beta &amp; me"},
		{name: "test_join07", src: "{{ a|join:var|lower }}", ctx: map[string]any{"a": []any{"Alpha", "Beta & me"}, "var": " & "}, want: "alpha &amp; beta &amp; me"},
		{name: "test_join08", src: "{{ a|join:var|lower }}", ctx: map[string]any{"a": []any{"Alpha", "Beta & me"}, "var": templates.MarkSafe(" & ")}, want: "alpha & beta &amp; me"},
	})
}

// TestJoinFunction ports Django's FunctionTests.
//
// Ported from Django: tests/template_tests/filter_tests/test_join.py::FunctionTests.test_list
func TestJoinFunction(t *testing.T) {
	runRenderCases(t, []renderCase{
		{name: "test_list", src: "{{ a|join:b }}", ctx: map[string]any{"a": []any{0, 1, 2}, "b": "glue"}, want: "0glue1glue2"},
	})
}
