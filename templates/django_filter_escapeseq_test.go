package templates_test

import (
	"testing"

	"github.com/oliverhaas/djangogo/templates"
)

// TestEscapeseq ports Django's EscapeseqTests.
//
// Ported from Django: tests/template_tests/filter_tests/test_escapeseq.py::EscapeseqTests.test_basic
// Ported from Django: tests/template_tests/filter_tests/test_escapeseq.py::EscapeseqTests.test_autoescape_off
// Ported from Django: tests/template_tests/filter_tests/test_escapeseq.py::EscapeseqTests.test_chain_join
// Ported from Django: tests/template_tests/filter_tests/test_escapeseq.py::EscapeseqTests.test_chain_join_autoescape_off
func TestEscapeseq(t *testing.T) {
	runRenderCases(t, []renderCase{
		{name: "test_basic", src: `{{ a|escapeseq|join:", " }} -- {{ b|escapeseq|join:", " }}`, ctx: map[string]any{"a": []any{"x&y", "<p>"}, "b": []any{templates.MarkSafe("x&y"), templates.MarkSafe("<p>")}}, want: "x&amp;y, &lt;p&gt; -- x&y, <p>"},
		{name: "test_autoescape_off", src: `{% autoescape off %}{{ a|escapeseq|join:", " }} -- {{ b|escapeseq|join:", "}}{% endautoescape %}`, ctx: map[string]any{"a": []any{"x&y", "<p>"}, "b": []any{templates.MarkSafe("x&y"), templates.MarkSafe("<p>")}}, want: "x&amp;y, &lt;p&gt; -- x&y, <p>"},
		{name: "test_chain_join", src: `{{ a|escapeseq|join:"<br/>" }}`, ctx: map[string]any{"a": []any{"x&y", "<p>"}}, want: "x&amp;y<br/>&lt;p&gt;"},
		{name: "test_chain_join_autoescape_off", src: `{% autoescape off %}{{ a|escapeseq|join:"<br/>" }}{% endautoescape %}`, ctx: map[string]any{"a": []any{"x&y", "<p>"}}, want: "x&amp;y<br/>&lt;p&gt;"},
	})
}
