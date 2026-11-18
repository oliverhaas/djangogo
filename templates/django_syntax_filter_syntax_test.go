package templates_test

import (
	"testing"
)

// TestFilterSyntax ports Django's FilterSyntaxTests.
//
// Ported from Django: tests/template_tests/syntax_tests/test_filter_syntax.py::FilterSyntaxTests.test_filter_syntax01
// Ported from Django: tests/template_tests/syntax_tests/test_filter_syntax.py::FilterSyntaxTests.test_filter_syntax02
// Ported from Django: tests/template_tests/syntax_tests/test_filter_syntax.py::FilterSyntaxTests.test_filter_syntax03
// Ported from Django: tests/template_tests/syntax_tests/test_filter_syntax.py::FilterSyntaxTests.test_filter_syntax04
// Ported from Django: tests/template_tests/syntax_tests/test_filter_syntax.py::FilterSyntaxTests.test_filter_syntax05
// Ported from Django: tests/template_tests/syntax_tests/test_filter_syntax.py::FilterSyntaxTests.test_filter_syntax06
// Ported from Django: tests/template_tests/syntax_tests/test_filter_syntax.py::FilterSyntaxTests.test_filter_syntax07
// Ported from Django: tests/template_tests/syntax_tests/test_filter_syntax.py::FilterSyntaxTests.test_filter_syntax08
// Ported from Django: tests/template_tests/syntax_tests/test_filter_syntax.py::FilterSyntaxTests.test_filter_syntax08_multi_line
// Ported from Django: tests/template_tests/syntax_tests/test_filter_syntax.py::FilterSyntaxTests.test_filter_syntax09
// Ported from Django: tests/template_tests/syntax_tests/test_filter_syntax.py::FilterSyntaxTests.test_filter_syntax10
// Ported from Django: tests/template_tests/syntax_tests/test_filter_syntax.py::FilterSyntaxTests.test_filter_syntax11
// Ported from Django: tests/template_tests/syntax_tests/test_filter_syntax.py::FilterSyntaxTests.test_filter_syntax15
// Ported from Django: tests/template_tests/syntax_tests/test_filter_syntax.py::FilterSyntaxTests.test_filter_syntax16
// Ported from Django: tests/template_tests/syntax_tests/test_filter_syntax.py::FilterSyntaxTests.test_filter_syntax17
// Ported from Django: tests/template_tests/syntax_tests/test_filter_syntax.py::FilterSyntaxTests.test_filter_syntax19
// Ported from Django: tests/template_tests/syntax_tests/test_filter_syntax.py::FilterSyntaxTests.test_filter_syntax20
func TestFilterSyntax(t *testing.T) {
	runRenderCases(t, []renderCase{
		{name: "test_filter_syntax01", src: "{{ var|upper }}", ctx: map[string]any{"var": "Django is the greatest!"}, want: "DJANGO IS THE GREATEST!"},
		{name: "test_filter_syntax02", src: "{{ var|upper|lower }}", ctx: map[string]any{"var": "Django is the greatest!"}, want: "django is the greatest!"},
		{name: "test_filter_syntax03", src: "{{ var |upper }}", ctx: map[string]any{"var": "Django is the greatest!"}, want: "DJANGO IS THE GREATEST!"},
		{name: "test_filter_syntax04", src: "{{ var| upper }}", ctx: map[string]any{"var": "Django is the greatest!"}, want: "DJANGO IS THE GREATEST!"},
		{name: "test_filter_syntax05", src: "{{ var|does_not_exist }}", wantErr: true},
		{name: "test_filter_syntax06", src: "{{ var|fil(ter) }}", wantErr: true},
		{name: "test_filter_syntax07", src: "{% nothing_to_see_here %}", wantErr: true},
		{name: "test_filter_syntax08", src: "{% %}", wantErr: true},
		{name: "test_filter_syntax08_multi_line", src: "line 1\nline 2\nline 3{% %}\nline 4\nline 5", wantErr: true},
		{name: "test_filter_syntax09", src: `{{ var|cut:"o"|upper|lower }}`, ctx: map[string]any{"var": "Foo"}, want: "f"},
		{name: "test_filter_syntax10", src: `{{ var|default_if_none:" endquote\" hah" }}`, ctx: map[string]any{"var": nil}, want: ` endquote" hah`},
		{name: "test_filter_syntax11", src: "{{ var|default_if_none:var2 }}", ctx: map[string]any{"var": nil, "var2": "happy"}, want: "happy"},
		{name: "test_filter_syntax15", src: `{{ var|default_if_none:"foo\bar" }}`, ctx: map[string]any{"var": nil}, want: `foo\bar`},
		{name: "test_filter_syntax16", src: `{{ var|default_if_none:"foo\now" }}`, ctx: map[string]any{"var": nil}, want: `foo\now`},
		{name: "test_filter_syntax17", src: `{{ var|join:"" }}`, ctx: map[string]any{"var": []any{"a", "b", "c"}}, want: "abc"},
		{name: "test_filter_syntax19", src: "{{ var|truncatewords:1 }}", ctx: map[string]any{"var": "hello world"}, want: "hello …"},
		{name: "test_filter_syntax20", src: `{{ ""|default_if_none:"was none" }}`, want: ""},
	})
}
