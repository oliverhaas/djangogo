package templates_test

import (
	"testing"
)

// TestSafeseq ports Django's SafeseqTests.
//
// Ported from Django: tests/template_tests/filter_tests/test_safeseq.py::SafeseqTests.test_safeseq01
// Ported from Django: tests/template_tests/filter_tests/test_safeseq.py::SafeseqTests.test_safeseq02
func TestSafeseq(t *testing.T) {
	runRenderCases(t, []renderCase{
		{name: "test_safeseq01", src: `{{ a|join:", " }} -- {{ a|safeseq|join:", " }}`, ctx: map[string]any{"a": []any{"&", "<"}}, want: "&amp;, &lt; -- &, <"},
		{name: "test_safeseq02", src: `{% autoescape off %}{{ a|join:", " }} -- {{ a|safeseq|join:", " }}{% endautoescape %}`, ctx: map[string]any{"a": []any{"&", "<"}}, want: "&, < -- &, <"},
	})
}
