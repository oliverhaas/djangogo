package templates_test

import (
	"testing"

	"github.com/oliverhaas/djangogo/templates"
)

// TestUrlizetrunc ports Django's UrlizetruncTests.
//
// Ported from Django: tests/template_tests/filter_tests/test_urlizetrunc.py::UrlizetruncTests.test_urlizetrunc01
// Ported from Django: tests/template_tests/filter_tests/test_urlizetrunc.py::UrlizetruncTests.test_urlizetrunc02
func TestUrlizetrunc(t *testing.T) {
	runRenderCases(t, []renderCase{
		{name: "test_urlizetrunc01", src: `{% autoescape off %}{{ a|urlizetrunc:"8" }} {{ b|urlizetrunc:"8" }}{% endautoescape %}`, ctx: map[string]any{"a": `"Unsafe" http://example.com/x=&y=`, "b": templates.MarkSafe("&quot;Safe&quot; http://example.com?x=&amp;y=")}, want: `"Unsafe" <a href="http://example.com/x=&amp;y=" rel="nofollow">http://…</a> &quot;Safe&quot; <a href="http://example.com?x=&amp;y=" rel="nofollow">http://…</a>`},
		{name: "test_urlizetrunc02", src: `{{ a|urlizetrunc:"8" }} {{ b|urlizetrunc:"8" }}`, ctx: map[string]any{"a": `"Unsafe" http://example.com/x=&y=`, "b": templates.MarkSafe("&quot;Safe&quot; http://example.com?x=&amp;y=")}, want: `&quot;Unsafe&quot; <a href="http://example.com/x=&amp;y=" rel="nofollow">http://…</a> &quot;Safe&quot; <a href="http://example.com?x=&amp;y=" rel="nofollow">http://…</a>`},
	})
}

// TestUrlizetruncFunction ports Django's FunctionTests.
//
// Ported from Django: tests/template_tests/filter_tests/test_urlizetrunc.py::FunctionTests.test_non_string_input
func TestUrlizetruncFunction(t *testing.T) {
	runRenderCases(t, []renderCase{
		{name: "test_non_string_input", src: "{{ a|urlizetrunc:b }}", ctx: map[string]any{"a": 123, "b": 1}, want: "123"},
	})
}
