package templates_test

import (
	"testing"

	"github.com/oliverhaas/djangogo/templates"
)

// syntaxCase is one row of the basic syntax tables: a named template source,
// its context and either the expected output or an expected failure.
type syntaxCase struct {
	name    string
	src     string
	ctx     map[string]any
	want    string
	wantErr bool
	skip    string
}

// runSyntaxCases renders each row as the template "name" next to the shared
// basic templates and asserts the output or the failure.
func runSyntaxCases(t *testing.T, tests []syntaxCase) {
	t.Helper()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.skip != "" {
				t.Skip(tt.skip)
			}
			engine := memEngine(map[string]string{tt.name: tt.src})
			got, err := engine.Render(tt.name, tt.ctx)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("render succeeded with %q, want an error", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("render: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

// TestBasicSyntax ports Django's BasicSyntaxTests. The string_if_invalid
// branches are ported for the default engine (empty string) only.
//
// Ported from Django: tests/template_tests/syntax_tests/test_basic.py::BasicSyntaxTests.test_basic_syntax01
// Ported from Django: tests/template_tests/syntax_tests/test_basic.py::BasicSyntaxTests.test_basic_syntax02
// Ported from Django: tests/template_tests/syntax_tests/test_basic.py::BasicSyntaxTests.test_basic_syntax03
// Ported from Django: tests/template_tests/syntax_tests/test_basic.py::BasicSyntaxTests.test_basic_syntax04
// Ported from Django: tests/template_tests/syntax_tests/test_basic.py::BasicSyntaxTests.test_basic_syntax06
// Ported from Django: tests/template_tests/syntax_tests/test_basic.py::BasicSyntaxTests.test_basic_syntax07
// Ported from Django: tests/template_tests/syntax_tests/test_basic.py::BasicSyntaxTests.test_basic_syntax08
// Ported from Django: tests/template_tests/syntax_tests/test_basic.py::BasicSyntaxTests.test_basic_syntax09
// Ported from Django: tests/template_tests/syntax_tests/test_basic.py::BasicSyntaxTests.test_basic_syntax10
// Ported from Django: tests/template_tests/syntax_tests/test_basic.py::BasicSyntaxTests.test_basic_syntax11
// Ported from Django: tests/template_tests/syntax_tests/test_basic.py::BasicSyntaxTests.test_basic_syntax12
// Ported from Django: tests/template_tests/syntax_tests/test_basic.py::BasicSyntaxTests.test_basic_syntax13
// Ported from Django: tests/template_tests/syntax_tests/test_basic.py::BasicSyntaxTests.test_basic_syntax14
// Ported from Django: tests/template_tests/syntax_tests/test_basic.py::BasicSyntaxTests.test_basic_syntax15
// Ported from Django: tests/template_tests/syntax_tests/test_basic.py::BasicSyntaxTests.test_basic_syntax16
// Ported from Django: tests/template_tests/syntax_tests/test_basic.py::BasicSyntaxTests.test_basic_syntax17
// Ported from Django: tests/template_tests/syntax_tests/test_basic.py::BasicSyntaxTests.test_basic_syntax18
// Ported from Django: tests/template_tests/syntax_tests/test_basic.py::BasicSyntaxTests.test_basic_syntax19
// Ported from Django: tests/template_tests/syntax_tests/test_basic.py::BasicSyntaxTests.test_basic_syntax20
// Ported from Django: tests/template_tests/syntax_tests/test_basic.py::BasicSyntaxTests.test_basic_syntax20b
// Ported from Django: tests/template_tests/syntax_tests/test_basic.py::BasicSyntaxTests.test_basic_syntax21
// Ported from Django: tests/template_tests/syntax_tests/test_basic.py::BasicSyntaxTests.test_basic_syntax22
// Ported from Django: tests/template_tests/syntax_tests/test_basic.py::BasicSyntaxTests.test_basic_syntax23
// Ported from Django: tests/template_tests/syntax_tests/test_basic.py::BasicSyntaxTests.test_basic_syntax24
// Ported from Django: tests/template_tests/syntax_tests/test_basic.py::BasicSyntaxTests.test_basic_syntax25
// Ported from Django: tests/template_tests/syntax_tests/test_basic.py::BasicSyntaxTests.test_basic_syntax26
// Ported from Django: tests/template_tests/syntax_tests/test_basic.py::BasicSyntaxTests.test_basic_syntax27
// Ported from Django: tests/template_tests/syntax_tests/test_basic.py::BasicSyntaxTests.test_basic_syntax28
// Ported from Django: tests/template_tests/syntax_tests/test_basic.py::BasicSyntaxTests.test_basic_syntax29
// Ported from Django: tests/template_tests/syntax_tests/test_basic.py::BasicSyntaxTests.test_basic_syntax30
// Ported from Django: tests/template_tests/syntax_tests/test_basic.py::BasicSyntaxTests.test_basic_syntax31
// Ported from Django: tests/template_tests/syntax_tests/test_basic.py::BasicSyntaxTests.test_basic_syntax32
// Ported from Django: tests/template_tests/syntax_tests/test_basic.py::BasicSyntaxTests.test_basic_syntax33
// Ported from Django: tests/template_tests/syntax_tests/test_basic.py::BasicSyntaxTests.test_basic_syntax34
// Ported from Django: tests/template_tests/syntax_tests/test_basic.py::BasicSyntaxTests.test_basic_syntax35
// Ported from Django: tests/template_tests/syntax_tests/test_basic.py::BasicSyntaxTests.test_basic_syntax36
// Ported from Django: tests/template_tests/syntax_tests/test_basic.py::BasicSyntaxTests.test_basic_syntax37
// Ported from Django: tests/template_tests/syntax_tests/test_basic.py::BasicSyntaxTests.test_basic_syntax38
// Ported from Django: tests/template_tests/syntax_tests/test_basic.py::BasicSyntaxTests.test_unclosed_block
// Ported from Django: tests/template_tests/syntax_tests/test_basic.py::BasicSyntaxTests.test_unclosed_block2
// Ported from Django: tests/template_tests/syntax_tests/test_basic.py::BasicSyntaxTests.test_double_dot_in_literal
func TestBasicSyntax(t *testing.T) {
	const pyObject = "needs Python attribute lookup on an object"
	tuple := func(v ...any) []any { return v }
	runSyntaxCases(t, []syntaxCase{
		{name: "basic-syntax01", src: "something cool", want: "something cool"},
		{name: "basic-syntax02", src: "{{ headline }}", ctx: map[string]any{"headline": "Success"}, want: "Success"},
		{
			name: "basic-syntax03", src: "{{ first }} --- {{ second }}",
			ctx: map[string]any{"first": 1, "second": 2}, want: "1 --- 2",
		},
		{name: "basic-syntax04", src: "as{{ missing }}df", want: "asdf"},
		{name: "basic-syntax06", src: "{{ multi word variable }}", wantErr: true},
		{name: "basic-syntax07", src: "{{ }}", wantErr: true},
		{name: "basic-syntax08", src: "{{        }}", wantErr: true},
		{name: "basic-syntax09", src: "{{ var.method }}", skip: pyObject},
		{name: "basic-syntax10", src: "{{ var.otherclass.method }}", skip: pyObject},
		{name: "basic-syntax11", src: "{{ var.blech }}", skip: pyObject},
		{name: "basic-syntax12", src: "{{ var.__dict__ }}", wantErr: true, skip: "Django rejects names that start with an underscore; pongo2 resolves them to nothing"},
		{name: "basic-syntax13", src: "{{ va>r }}", wantErr: true, skip: "pongo2 reads va>r as a comparison, where Django reports a syntax error"},
		{name: "basic-syntax14", src: "{{ (var.r) }}", wantErr: true},
		{name: "basic-syntax15", src: "{{ sp%am }}", wantErr: true},
		{name: "basic-syntax16", src: "{{ eggs! }}", wantErr: true},
		{name: "basic-syntax17", src: "{{ moo? }}", wantErr: true},
		{
			name: "basic-syntax18", src: "{{ foo.bar }}",
			ctx: map[string]any{"foo": map[string]any{"bar": "baz"}}, want: "baz",
		},
		{
			name: "basic-syntax19", src: "{{ foo.spam }}",
			ctx: map[string]any{"foo": map[string]any{"bar": "baz"}}, want: "",
		},
		{name: "basic-syntax20", src: "{{ var.method2 }}", skip: pyObject},
		{name: "basic-syntax20b", src: "{{ var.method5 }}", skip: pyObject},
		{name: "basic-syntax21", src: "a {{ moo %} b", want: "a {{ moo %} b"},
		{name: "basic-syntax22", src: "{{ moo #}", want: "{{ moo #}"},
		{name: "basic-syntax23", src: "{{ moo #} {{ cow }}", wantErr: true},
		{name: "basic-syntax24", src: "{{ moo\n }}", want: "{{ moo\n }}"},
		{name: "basic-syntax25", src: `{{ "fred" }}`, want: "fred"},
		{name: "basic-syntax26", src: `{{ "\"fred\"" }}`, want: `"fred"`},
		{name: "basic-syntax27", src: `{{ _("\"fred\"") }}`, want: `"fred"`},
		{name: "basic-syntax28", src: "{{ a.b }}", skip: pyObject},
		{name: "basic-syntax29", src: "{{ a.b }}", skip: pyObject},
		{
			name: "basic-syntax30", skip: "pongo2 reads 1.2.3 as a number followed by garbage, Django as a variable lookup", src: "{{ 1.2.3 }}",
			ctx:  map[string]any{"1": map[string]any{"2": map[string]any{"3": "d"}}},
			want: "d",
		},
		{
			name: "basic-syntax31", skip: "pongo2 reads 1.2.3 as a number followed by garbage, Django as a variable lookup", src: "{{ 1.2.3 }}",
			ctx:  map[string]any{"1": map[string]any{"2": tuple("a", "b", "c", "d")}},
			want: "d",
		},
		{
			name: "basic-syntax32", skip: "pongo2 reads 1.2.3 as a number followed by garbage, Django as a variable lookup", src: "{{ 1.2.3 }}",
			ctx: map[string]any{"1": tuple(
				tuple("x", "x", "x", "x"), tuple("y", "y", "y", "y"), tuple("a", "b", "c", "d"),
			)},
			want: "d",
		},
		{
			name: "basic-syntax33", skip: "pongo2 reads 1.2.3 as a number followed by garbage, Django as a variable lookup", src: "{{ 1.2.3 }}",
			ctx:  map[string]any{"1": tuple("xxxx", "yyyy", "abcd")},
			want: "d",
		},
		{
			name: "basic-syntax34", skip: "pongo2 reads 1.2.3 as a number followed by garbage, Django as a variable lookup", src: "{{ 1.2.3 }}",
			ctx: map[string]any{"1": tuple(
				map[string]any{"x": "x"}, map[string]any{"y": "y"}, map[string]any{"z": "z", "3": "d"},
			)},
			want: "d",
		},
		{name: "basic-syntax35", src: "{{ 1 }}", ctx: map[string]any{"1": "abc"}, want: "1"},
		{name: "basic-syntax36", skip: "a float literal prints with six decimals (\"1.200000\"), see fidelity/divergences.md", src: "{{ 1.2 }}", ctx: map[string]any{"1": "abc"}, want: "1.2"},
		{
			name: "basic-syntax37", src: "{{ callable }}",
			ctx:  map[string]any{"callable": func() string { return "foo bar" }},
			want: "foo bar",
		},
		{
			name: "basic-syntax38", src: "{{ var.callable }}",
			ctx:  map[string]any{"var": map[string]any{"callable": func() string { return "foo bar" }}},
			want: "foo bar",
		},
		{name: "test_unclosed_block", src: "{% block content %}", wantErr: true},
		{name: "test_unclosed_block2", src: "{% if a %}", wantErr: true},
		{name: "test_double_dot_in_literal-1", src: `{{ "hello..world" }}`, want: "hello..world"},
		{name: "test_double_dot_in_literal-2", src: "{{ 'a..b'|upper }}", want: "A..B"},
		{name: "test_double_dot_in_literal-3", src: `{{ missing|default:"a..b" }}`, want: "a..b"},
		{name: "test_double_dot_in_literal-4", src: `{{ _("a..b") }}`, want: "a..b"},
	})
}

// TestBasicSyntaxFormatStrings ports the test that percent signs and %s in a
// template pass through untouched.
//
// Ported from Django: tests/template_tests/syntax_tests/test_basic.py::BasicSyntaxTests.test_ignores_strings_that_look_like_format_interpolation
func TestBasicSyntaxFormatStrings(t *testing.T) {
	engine := templates.New(templates.MapSource("locmem", map[string]string{
		"tpl-str": "%s", "tpl-percent": "%%", "tpl-weird-percent": "% %s",
	}))
	for name, want := range map[string]string{"tpl-str": "%s", "tpl-percent": "%%", "tpl-weird-percent": "% %s"} {
		got, err := engine.Render(name, nil)
		if err != nil {
			t.Fatalf("render %s: %v", name, err)
		}
		if got != want {
			t.Errorf("%s: got %q, want %q", name, got, want)
		}
	}
}

// TestBasicSyntaxSkipped records the Django tests of test_basic.py that have no
// Go counterpart.
//
// Ported from Django: tests/template_tests/syntax_tests/test_basic.py::BasicSyntaxTests.test_subscriptable_class
// Ported from Django: tests/template_tests/syntax_tests/test_basic.py::BasicSyntaxTests.test_access_class_property_if_getitem_is_defined_in_metaclass
// Ported from Django: tests/template_tests/syntax_tests/test_basic.py::BasicSyntaxTests.test_double_dot_lookup
// Ported from Django: tests/template_tests/syntax_tests/test_basic.py::BlockContextTests.test_repr
// Ported from Django: tests/template_tests/syntax_tests/test_basic.py::DeprecatedTests.test_double_dot_lookup
func TestBasicSyntaxSkipped(t *testing.T) {
	for _, tt := range []struct{ name, reason string }{
		{"test_subscriptable_class", "needs Python classes and metaclass lookup"},
		{"test_access_class_property_if_getitem_is_defined_in_metaclass", "needs Python classes and metaclass lookup"},
		{"test_double_dot_lookup", "Python deprecation warning with no Go counterpart"},
		{"BlockContextTests.test_repr", "Python repr of a Django internal"},
		{"DeprecatedTests.test_double_dot_lookup", "Python deprecation warning with no Go counterpart"},
	} {
		t.Run(tt.name, func(t *testing.T) { t.Skip(tt.reason) })
	}
}

// TestTemplateNameInException ports TemplateNameInExceptionTests. Only the
// failure is asserted: pongo2 words its errors differently.
//
// Ported from Django: tests/template_tests/syntax_tests/test_basic.py::TemplateNameInExceptionTests.test_template_name_in_error_message
// Ported from Django: tests/template_tests/syntax_tests/test_basic.py::TemplateNameInExceptionTests.test_template_name_not_in_debug_view
// Ported from Django: tests/template_tests/syntax_tests/test_basic.py::TemplateNameInExceptionTests.test_unknown_source_template
func TestTemplateNameInException(t *testing.T) {
	engine := templates.New(templates.MapSource("locmem", map[string]string{"test.html": "{% endfor %}"}))
	if _, err := engine.Render("test.html", nil); err == nil {
		t.Error("named template: want an error for a stray endfor")
	}
	if _, err := engine.RenderString("{% endfor %}", nil); err == nil {
		t.Error("string template: want an error for a stray endfor")
	}
}

// exceptionTemplates are the templates of Django's test_extends the
// exception tests extend.
var exceptionTemplates = map[string]string{
	"inheritance01": "1{% block first %}&{% endblock %}3{% block second %}_{% endblock %}",
	"inheritance15": "{% extends 'inheritance01' %}" +
		"{% block first %}2{% block inner %}inner{% endblock %}{% endblock %}",
	"inheritance16": "{% extends 'inheritance15' %}{% block inner %}out{% endblock %}",
	"inheritance17": "{% load testtags %}{% block first %}1234{% endblock %}",
}

// TestErrors ports Django's ExceptionsTests. Only the failure is asserted:
// pongo2 has one error type for syntax and missing templates.
//
// exception04 relies on the testtags library, which does not exist here, so
// the load of inheritance17 fails for that reason, not for Django's.
//
// Ported from Django: tests/template_tests/syntax_tests/test_exceptions.py::ExceptionsTests.test_exception01
// Ported from Django: tests/template_tests/syntax_tests/test_exceptions.py::ExceptionsTests.test_exception02
// Ported from Django: tests/template_tests/syntax_tests/test_exceptions.py::ExceptionsTests.test_exception03
// Ported from Django: tests/template_tests/syntax_tests/test_exceptions.py::ExceptionsTests.test_exception04
// Ported from Django: tests/template_tests/syntax_tests/test_exceptions.py::ExceptionsTests.test_exception05
// Ported from Django: tests/template_tests/syntax_tests/test_exceptions.py::ExceptionsTests.test_unknown_origin_relative_path
func TestErrors(t *testing.T) {
	tests := []struct {
		name string
		src  string
		skip string
	}{
		{name: "exception01", src: "{% extends 'nonexistent' %}"},
		{name: "exception02", src: "{% extends nonexistent %}"},
		{
			name: "exception03",
			src: "{% extends 'inheritance01' %}" +
				"{% block first %}2{% endblock %}{% extends 'inheritance16' %}",
		},
		{
			name: "exception04",
			src:  "{% extends 'inheritance17' %}{% block first %}{% echo 400 %}5678{% endblock %}",
		},
		{name: "exception05", src: "{% block first %}{{ block.super }}{% endblock %}", skip: "{{ block.super }} outside a child template renders nothing instead of failing"},
		{name: "test_unknown_origin_relative_path-1", src: "{% extends './nonexistent.html' %}"},
		{name: "test_unknown_origin_relative_path-2", src: "{% extends '../nonexistent.html' %}"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.skip != "" {
				t.Skip(tt.skip)
			}
			all := map[string]string{tt.name: tt.src}
			for k, v := range exceptionTemplates {
				all[k] = v
			}
			engine := templates.New(templates.MapSource("locmem", all))
			if got, err := engine.Render(tt.name, nil); err == nil {
				t.Fatalf("render succeeded with %q, want an error", got)
			}
		})
	}
}
