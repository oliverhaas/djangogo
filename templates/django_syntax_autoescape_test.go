package templates_test

import (
	"testing"

	"github.com/oliverhaas/djangogo/templates"
)

// TestAutoescapeTag ports Django's AutoescapeTagTests.
//
// Ported from Django: tests/template_tests/syntax_tests/test_autoescape.py::AutoescapeTagTests.test_autoescape_tag01
// Ported from Django: tests/template_tests/syntax_tests/test_autoescape.py::AutoescapeTagTests.test_autoescape_tag02
// Ported from Django: tests/template_tests/syntax_tests/test_autoescape.py::AutoescapeTagTests.test_autoescape_tag03
// Ported from Django: tests/template_tests/syntax_tests/test_autoescape.py::AutoescapeTagTests.test_autoescape_tag04
// Ported from Django: tests/template_tests/syntax_tests/test_autoescape.py::AutoescapeTagTests.test_autoescape_tag05
// Ported from Django: tests/template_tests/syntax_tests/test_autoescape.py::AutoescapeTagTests.test_autoescape_tag06
// Ported from Django: tests/template_tests/syntax_tests/test_autoescape.py::AutoescapeTagTests.test_autoescape_tag07
// Ported from Django: tests/template_tests/syntax_tests/test_autoescape.py::AutoescapeTagTests.test_autoescape_tag08
// Ported from Django: tests/template_tests/syntax_tests/test_autoescape.py::AutoescapeTagTests.test_autoescape_filtertag01
// Ported from Django: tests/template_tests/syntax_tests/test_autoescape.py::AutoescapeTagTests.test_autoescape_filters01
// Ported from Django: tests/template_tests/syntax_tests/test_autoescape.py::AutoescapeTagTests.test_autoescape_filters02
// Ported from Django: tests/template_tests/syntax_tests/test_autoescape.py::AutoescapeTagTests.test_autoescape_literals01
// Ported from Django: tests/template_tests/syntax_tests/test_autoescape.py::AutoescapeTagTests.test_autoescape_stringiterations01
// Ported from Django: tests/template_tests/syntax_tests/test_autoescape.py::AutoescapeTagTests.test_autoescape_lookup01
// Ported from Django: tests/template_tests/syntax_tests/test_autoescape.py::AutoescapeTagTests.test_invalid_arg
// Ported from Django: tests/template_tests/syntax_tests/test_autoescape.py::AutoescapeTagTests.test_no_arg
func TestAutoescapeTag(t *testing.T) {
	runRenderCases(t, []renderCase{
		{name: "test_autoescape_tag01", src: "{% autoescape off %}hello{% endautoescape %}", want: "hello"},
		{name: "test_autoescape_tag02", src: "{% autoescape off %}{{ first }}{% endautoescape %}", ctx: map[string]any{"first": "<b>hello</b>"}, want: "<b>hello</b>"},
		{name: "test_autoescape_tag03", src: "{% autoescape on %}{{ first }}{% endautoescape %}", ctx: map[string]any{"first": "<b>hello</b>"}, want: "&lt;b&gt;hello&lt;/b&gt;"},
		{name: "test_autoescape_tag04", src: "{% autoescape off %}{{ first }} {% autoescape on %}{{ first }}{% endautoescape %}{% endautoescape %}", ctx: map[string]any{"first": "<a>"}, want: "<a> &lt;a&gt;"},
		{name: "test_autoescape_tag05", src: "{% autoescape on %}{{ first }}{% endautoescape %}", ctx: map[string]any{"first": "<b>first</b>"}, want: "&lt;b&gt;first&lt;/b&gt;"},
		{name: "test_autoescape_tag06", src: "{{ first }}", ctx: map[string]any{"first": templates.MarkSafe("<b>first</b>")}, want: "<b>first</b>"},
		{name: "test_autoescape_tag07", src: "{% autoescape on %}{{ first }}{% endautoescape %}", ctx: map[string]any{"first": templates.MarkSafe("<b>Apple</b>")}, want: "<b>Apple</b>"},
		{name: "test_autoescape_tag08", src: `{% autoescape on %}{{ var|default_if_none:" endquote\" hah" }}{% endautoescape %}`, ctx: map[string]any{"var": nil}, want: ` endquote" hah`},
		{name: "test_autoescape_filtertag01", src: "{{ first }}{% filter safe %}{{ first }} x<y{% endfilter %}", ctx: map[string]any{"first": "<a>"}, wantErr: true},
		{name: "test_autoescape_filters01", src: `{{ var|cut:"&" }}`, ctx: map[string]any{"var": "this & that"}, want: "this  that"},
		{name: "test_autoescape_filters02", src: `{{ var|join:" & " }}`, ctx: map[string]any{"var": []any{"Tom", "Dick", "Harry"}}, want: "Tom & Dick & Harry"},
		{name: "test_autoescape_literals01", src: `{{ "this & that" }}`, want: "this & that"},
		{name: "test_autoescape_stringiterations01", src: "{% for l in var %}{{ l }},{% endfor %}", ctx: map[string]any{"var": "K&R"}, want: "K,&amp;,R,"},
		{name: "test_autoescape_lookup01", src: "{{ var.key }}", ctx: map[string]any{"var": map[string]any{"key": "this & that"}}, want: "this &amp; that"},
		{name: "test_invalid_arg", src: "{% autoescape true %}{{ var.key }}{% endautoescape %}", ctx: map[string]any{"var": map[string]any{"key": "this & that"}}, wantErr: true},
		{name: "test_no_arg", src: "{% autoescape %}{{ var.key }}{% endautoescape %}", ctx: map[string]any{"var": map[string]any{"key": "this & that"}}, wantErr: true},
	})
}
