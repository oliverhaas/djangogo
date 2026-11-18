package templates_test

import (
	"testing"

	"github.com/oliverhaas/djangogo/templates"
)

// TestUrlize ports Django's UrlizeTests.
//
// Ported from Django: tests/template_tests/filter_tests/test_urlize.py::UrlizeTests.test_urlize01
// Ported from Django: tests/template_tests/filter_tests/test_urlize.py::UrlizeTests.test_urlize02
// Ported from Django: tests/template_tests/filter_tests/test_urlize.py::UrlizeTests.test_urlize03
// Ported from Django: tests/template_tests/filter_tests/test_urlize.py::UrlizeTests.test_urlize04
// Ported from Django: tests/template_tests/filter_tests/test_urlize.py::UrlizeTests.test_urlize05
// Ported from Django: tests/template_tests/filter_tests/test_urlize.py::UrlizeTests.test_urlize06
// Ported from Django: tests/template_tests/filter_tests/test_urlize.py::UrlizeTests.test_urlize07
// Ported from Django: tests/template_tests/filter_tests/test_urlize.py::UrlizeTests.test_urlize08
// Ported from Django: tests/template_tests/filter_tests/test_urlize.py::UrlizeTests.test_urlize09
func TestUrlize(t *testing.T) {
	runRenderCases(t, []renderCase{
		{name: "test_urlize01", src: "{% autoescape off %}{{ a|urlize }} {{ b|urlize }}{% endautoescape %}", ctx: map[string]any{"a": "http://example.com/?x=&y=", "b": templates.MarkSafe("http://example.com?x=&amp;y=&lt;2&gt;")}, want: `<a href="http://example.com/?x=&amp;y=" rel="nofollow">http://example.com/?x=&y=</a> <a href="http://example.com?x=&amp;y=%3C2%3E" rel="nofollow">http://example.com?x=&amp;y=&lt;2&gt;</a>`},
		{name: "test_urlize02", src: "{{ a|urlize }} {{ b|urlize }}", ctx: map[string]any{"a": "http://example.com/?x=&y=", "b": templates.MarkSafe("http://example.com?x=&amp;y=")}, want: `<a href="http://example.com/?x=&amp;y=" rel="nofollow">http://example.com/?x=&amp;y=</a> <a href="http://example.com?x=&amp;y=" rel="nofollow">http://example.com?x=&amp;y=</a>`},
		{name: "test_urlize03", src: "{% autoescape off %}{{ a|urlize }}{% endautoescape %}", ctx: map[string]any{"a": templates.MarkSafe("a &amp; b")}, want: "a &amp; b"},
		{name: "test_urlize04", src: "{{ a|urlize }}", ctx: map[string]any{"a": templates.MarkSafe("a &amp; b")}, want: "a &amp; b"},
		{name: "test_urlize05", src: "{% autoescape off %}{{ a|urlize }}{% endautoescape %}", ctx: map[string]any{"a": "<script>alert('foo')</script>"}, want: "<script>alert('foo')</script>"},
		{name: "test_urlize06", src: "{{ a|urlize }}", ctx: map[string]any{"a": "<script>alert('foo')</script>"}, want: "&lt;script&gt;alert(&#x27;foo&#x27;)&lt;/script&gt;"},
		{name: "test_urlize07", src: "{{ a|urlize }}", ctx: map[string]any{"a": "Email me at me@example.com"}, want: `Email me at <a href="mailto:me@example.com">me@example.com</a>`},
		{name: "test_urlize08", src: "{{ a|urlize }}", ctx: map[string]any{"a": "Email me at <me@example.com>"}, want: `Email me at &lt;<a href="mailto:me@example.com">me@example.com</a>&gt;`},
		{name: "test_urlize09", src: "{% autoescape off %}{{ a|urlize }}{% endautoescape %}", ctx: map[string]any{"a": "http://example.com/?x=&amp;y=&lt;2&gt;"}, want: `<a href="http://example.com/?x=&amp;y=%3C2%3E" rel="nofollow">http://example.com/?x=&amp;y=&lt;2&gt;</a>`},
	})
}

// TestUrlizeFunction ports Django's FunctionTests.
//
// Ported from Django: tests/template_tests/filter_tests/test_urlize.py::FunctionTests.test_word_with_dot
// Ported from Django: tests/template_tests/filter_tests/test_urlize.py::FunctionTests.test_malformed
// Ported from Django: tests/template_tests/filter_tests/test_urlize.py::FunctionTests.test_invalid_email
// Ported from Django: tests/template_tests/filter_tests/test_urlize.py::FunctionTests.test_brackets
// Ported from Django: tests/template_tests/filter_tests/test_urlize.py::FunctionTests.test_non_string_input
func TestUrlizeFunction(t *testing.T) {
	runRenderCases(t, []renderCase{
		{name: "test_word_with_dot", src: "{{ a|urlize }}", ctx: map[string]any{"a": "some.organization"}, want: "some.organization"},
		{name: "test_malformed_1", src: "{{ a|urlize }}", ctx: map[string]any{"a": "http:///www.google.com"}, want: "http:///www.google.com"},
		{name: "test_malformed_2", src: "{{ a|urlize }}", ctx: map[string]any{"a": "http://.google.com"}, want: "http://.google.com"},
		{name: "test_malformed_3", src: "{{ a|urlize }}", ctx: map[string]any{"a": "http://@foo.com"}, want: "http://@foo.com"},
		{name: "test_invalid_email", src: "{{ a|urlize }}", ctx: map[string]any{"a": "email@.stream.ru"}, want: "email@.stream.ru"},
		{name: "test_brackets", src: "{{ a|urlize }}", ctx: map[string]any{"a": "see test[at[example.com"}, want: "see test[at[example.com"},
		{name: "test_non_string_input", src: "{{ a|urlize }}", ctx: map[string]any{"a": 123}, want: "123"},
	})
}
