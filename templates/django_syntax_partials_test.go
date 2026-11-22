package templates_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/templates"
)

// partialTemplates are the templates the partial tests of Django's
// syntax_tests extend and include.
var partialTemplates = map[string]string{
	"partial_base.html": "<main>{% block main %}Default main content.{% endblock main %}</main>",
	"partial_included.html": "INCLUDED TEMPLATE START\n" +
		"{% partialdef included-partial %}\n" +
		"THIS IS CONTENT FROM THE INCLUDED PARTIAL\n" +
		"{% endpartialdef %}\n\n" +
		"Now using the partial: {% partial included-partial %}\n" +
		"INCLUDED TEMPLATE END\n",
}

// partialFiles are Django's tests/template_tests/templates/partial_*.html.
var partialFiles = map[string]string{
	"partial_base.html":  "{% block main %}\n{% endblock main %}\n",
	"partial_child.html": "{% extends 'partial_base.html' %}\n\n{% partialdef extra-content %}\nExtra Content\n{% endpartialdef %}\n\n{% block main %}\nMain Content\n{% endblock %}\n",
	"partial_examples.html": "{% partialdef test-partial %}\nTEST-PARTIAL-CONTENT\n{% endpartialdef %}\n\n" +
		"{% block main %}\nBEGINNING\n{% partial test-partial %}\nMIDDLE\n{% partial test-partial %}\nEND\n{% endblock main %}\n\n" +
		"{% partialdef inline-partial inline %}\nINLINE-CONTENT\n{% endpartialdef %}\n",
}

// partialEngine returns an engine with the given templates and the ones of
// partialTemplates.
func partialEngine(named map[string]string) *templates.Engine {
	all := map[string]string{}
	for k, v := range partialTemplates {
		all[k] = v
	}
	for k, v := range named {
		all[k] = v
	}
	return templates.New(templates.MapSource("locmem", all))
}

// renderPartial renders name with an engine holding the given templates.
func renderPartial(t *testing.T, named map[string]string, name string) string {
	t.Helper()
	out, err := partialEngine(named).Render(name, nil)
	if err != nil {
		t.Fatalf("render %s: %v", name, err)
	}
	return out
}

// genPartialTemplate is Django's gen_partial_template without arguments.
func genPartialTemplate(name string) string {
	return "{% partialdef " + name + " %}TEST with " + name + "!{% endpartialdef %}{% partial " + name + " %}"
}

// validPartialdefNames are Django's valid_partialdef_names.
var validPartialdefNames = []string{
	"dot.in.name",
	"'space in name'",
	"exclamation!",
	"@at",
	"slash/something",
	"inline",
	"inline-inline",
	"INLINEwith+plus",
	"with&amp",
	"with%percent",
	"with,comma",
	"with:colon",
	"with;semicolon",
	"[brackets]",
	"(parens)",
	"{curly}",
}

// TestPartialValidNames ports Django's test of the names a partial can have.
//
// Ported from Django: tests/template_tests/syntax_tests/test_partials.py::PartialTagTests.test_valid_partialdef_names
func TestPartialValidNames(t *testing.T) {
	for _, name := range validPartialdefNames {
		t.Run(name, func(t *testing.T) {
			got := renderPartial(t, map[string]string{"t": genPartialTemplate(name)}, "t")
			if want := "TEST with " + name + "!"; got != want {
				t.Errorf("%s = %q, want %q", genPartialTemplate(name), got, want)
			}
		})
	}
}

// TestPartialUsage ports Django's tests of {% partialdef %} and
// {% partial %} in one template, with {% extends %} and {% include %}.
//
// Ported from Django: tests/template_tests/syntax_tests/test_partials.py::PartialTagTests.test_basic_usage
// Ported from Django: tests/template_tests/syntax_tests/test_partials.py::PartialTagTests.test_partial_inline_only_with_before_and_after_content
// Ported from Django: tests/template_tests/syntax_tests/test_partials.py::PartialTagTests.test_partial_inline_and_used_once
// Ported from Django: tests/template_tests/syntax_tests/test_partials.py::PartialTagTests.test_partial_inline_and_used_once_with_before_and_after_content
// Ported from Django: tests/template_tests/syntax_tests/test_partials.py::PartialTagTests.test_partial_used_before_definition
// Ported from Django: tests/template_tests/syntax_tests/test_partials.py::PartialTagTests.test_partial_defined_outside_main_block
// Ported from Django: tests/template_tests/syntax_tests/test_partials.py::PartialTagTests.test_partial_used_with_block_super
// Ported from Django: tests/template_tests/syntax_tests/test_partials.py::PartialTagTests.test_partial_in_included_template
// Ported from Django: tests/template_tests/syntax_tests/test_partials.py::PartialTagTests.test_partial_as_include_in_template
// Ported from Django: tests/template_tests/syntax_tests/test_partials.py::PartialTagTests.test_nested_partial_error_exception_info
func TestPartialUsage(t *testing.T) {
	const content = "HERE IS THE TEST CONTENT"
	for _, tc := range []struct {
		name, src string
		want      string
		contains  bool
		trim      bool
	}{
		{name: "test_basic_usage/basic", src: "{% partialdef testing-name %}" + content + "{% endpartialdef %}{% partial testing-name %}", want: content, trim: true},
		{name: "test_basic_usage/basic_inline", src: "{% partialdef testing-name inline %}" + content + "{% endpartialdef %}", want: content, trim: true},
		{name: "test_basic_usage/inline_inline", src: "{% partialdef inline inline %}" + content + "{% endpartialdef %}", want: content, trim: true},
		{
			name: "test_basic_usage/with_newlines",
			src:  "{% partialdef testing-name %}\n" + content + "\n{% endpartialdef testing-name %}\n{% partial testing-name %}",
			want: content, trim: true,
		},
		{
			name: "test_partial_inline_only_with_before_and_after_content",
			src:  "BEFORE\n{% partialdef testing-name inline %}" + content + "{% endpartialdef %}\nAFTER",
			want: "BEFORE\n" + content + "\nAFTER", trim: true,
		},
		{
			name: "test_partial_inline_and_used_once",
			src:  "{% partialdef testing-name inline %}" + content + "{% endpartialdef testing-name %}\n{% partial testing-name %}",
			want: content + "\n" + content,
		},
		{
			name: "test_partial_inline_and_used_once_with_before_and_after_content",
			src:  "BEFORE\n{% partialdef content_snippet inline %}" + content + "{% endpartialdef %}\nAFTER\n{% partial content_snippet %}",
			want: "BEFORE\n" + content + "\nAFTER\n" + content, trim: true,
		},
		{
			name: "test_partial_used_before_definition",
			src: "TEMPLATE START\n{% partial testing-name %}\nMIDDLE CONTENT\n{% partialdef testing-name %}\n" +
				"THIS IS THE PARTIAL CONTENT\n{% endpartialdef %}\nTEMPLATE END",
			want: "TEMPLATE START\n\nTHIS IS THE PARTIAL CONTENT\n\nMIDDLE CONTENT\n\nTEMPLATE END",
		},
		{
			name: "test_partial_defined_outside_main_block",
			src: "{% extends 'partial_base.html' %}{% partialdef testing-name %}Inside Content{% endpartialdef %}" +
				"{% block main %}Main content with {% partial testing-name %}{% endblock %}",
			want: "<main>Main content with Inside Content</main>", contains: true,
		},
		{
			name: "test_partial_used_with_block_super",
			src: "{% extends 'partial_base.html' %}{% partialdef testing-name %}Inside Content{% endpartialdef %}" +
				"{% block main %}{{ block.super }} Main content with {% partial testing-name %}{% endblock %}",
			want: "<main>Default main content. Main content with Inside Content</main>", contains: true,
		},
		{
			name: "test_partial_in_included_template",
			src:  "MAIN TEMPLATE START\n{% include 'partial_included.html' %}\nMAIN TEMPLATE END",
			want: "MAIN TEMPLATE START\nINCLUDED TEMPLATE START\n\n\nNow using the partial: \n" +
				"THIS IS CONTENT FROM THE INCLUDED PARTIAL\n\nINCLUDED TEMPLATE END\n\nMAIN TEMPLATE END",
		},
		{
			name: "test_partial_as_include_in_template",
			src:  "MAIN TEMPLATE START\n{% include 'partial_included.html#included-partial' %}\nMAIN TEMPLATE END",
			want: "MAIN TEMPLATE START\n\nTHIS IS CONTENT FROM THE INCLUDED PARTIAL\n\nMAIN TEMPLATE END",
		},
		{
			name: "test_nested_partial_error_exception_info",
			src: "<h1>Title</h1>\n{% partialdef outer_partial %}\n    <div class=\"outer\">\n" +
				"        {% partialdef inner_partial %}\n            <p>{{ undefined_var }}</p>\n        {% endpartialdef %}\n" +
				"        {% partial inner_partial %}\n    </div>\n{% endpartialdef %}\n{% partial outer_partial %}\n",
			want: "<p></p>", contains: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := renderPartial(t, map[string]string{"t": tc.src}, "t")
			switch {
			case tc.contains:
				if !strings.Contains(got, tc.want) {
					t.Errorf("%s = %q, want it to contain %q", tc.src, got, tc.want)
				}
			case tc.trim:
				if strings.TrimSpace(got) != tc.want {
					t.Errorf("%s = %q, want %q", tc.src, got, tc.want)
				}
			case got != tc.want:
				t.Errorf("%s = %q, want %q", tc.src, got, tc.want)
			}
		})
	}
}

// TestPartialNested ports Django's test of nested inline partials, loaded
// one at a time with "#name".
//
// Ported from Django: tests/template_tests/syntax_tests/test_partials.py::PartialTagTests.test_nested_partials
func TestPartialNested(t *testing.T) {
	named := map[string]string{
		"nested_simple": "{% extends 'base.html' %}{% block content %}This is my main page." +
			"{% partialdef outer inline %}    It hosts a couple of partials.\n" +
			"    {% partialdef inner inline %}        And an inner one.    {% endpartialdef inner %}" +
			"{% endpartialdef outer %}{% endblock content %}",
		"use_outer": "{% include 'nested_simple#outer' %}",
		"use_inner": "{% include 'nested_simple#inner' %}",
	}
	out := renderPartial(t, named, "use_outer")
	var lines []string
	for line := range strings.SplitSeq(out, "\n") {
		lines = append(lines, strings.TrimSpace(line))
	}
	if want := []string{"It hosts a couple of partials.", "And an inner one."}; strings.Join(lines, "|") != strings.Join(want, "|") {
		t.Errorf("use_outer = %q, want the lines %q", out, want)
	}
	if out := renderPartial(t, named, "use_inner"); strings.TrimSpace(out) != "And an inner one." {
		t.Errorf("use_inner = %q, want %q", out, "And an inner one.")
	}
}

// TestPartialErrors ports Django's tests of templates the partial tags
// reject. Django's messages for a tag it does not know and an unclosed tag
// are pongo2's here; those rows check for an error only.
//
// Ported from Django: tests/template_tests/syntax_tests/test_partials.py::PartialTagTests.test_basic_parse_errors
// Ported from Django: tests/template_tests/syntax_tests/test_partials.py::PartialTagTests.test_partialdef_invalid_inline
// Ported from Django: tests/template_tests/syntax_tests/test_partials.py::PartialTagTests.test_broken_partial_unclosed_exception_info
// Ported from Django: tests/template_tests/syntax_tests/test_partials.py::PartialTagTests.test_undefined_partial_exception_info
// Ported from Django: tests/template_tests/syntax_tests/test_partials.py::PartialTagTests.test_partial_with_syntax_error_exception_info
// Ported from Django: tests/template_tests/syntax_tests/test_partials.py::PartialTagTests.test_partial_in_extended_template_error
// Ported from Django: tests/template_tests/syntax_tests/test_partials.py::PartialTagTests.test_broken_partial_nesting
// Ported from Django: tests/template_tests/syntax_tests/test_partials.py::PartialTagTests.test_broken_partial_nesting_mixed
//
// FIXME-PORT: the template_debug details of these errors (Django's debug
// page) and PartialTagTests.test_partial_runtime_exception_has_debug_info,
// test_partial_template_get_exception_info_delegation and
// test_partial_runtime_error_exception_info, which need them.
func TestPartialErrors(t *testing.T) {
	for _, tc := range []struct {
		name, src, wantErr string
	}{
		{"partial_undefined_name", "{% partial undefined %}", "Partial 'undefined' is not defined in the current template."},
		{"partial_missing_name", "{% partial %}", "'partial' tag requires a single argument"},
		{"partial_closing_tag", "{% partialdef testing-name %}TEST{% endpartialdef %}{% partial testing-name %}{% endpartial %}", "endpartial"},
		{"partialdef_missing_name", "{% partialdef %}{% endpartialdef %}", "'partialdef' tag requires a name"},
		{"partialdef_missing_close_tag", "{% partialdef name %}TEST", "endpartialdef"},
		{
			"partialdef_opening_closing_name_mismatch", "{% partialdef testing-name %}TEST{% endpartialdef invalid %}",
			"expected 'endpartialdef' or 'endpartialdef testing-name'.",
		},
		{"partialdef_invalid_name", genPartialTemplate("with\nnewline"), "endpartialdef"},
		{"partialdef_extra_params", "{% partialdef testing-name inline extra %}TEST{% endpartialdef %}", "'partialdef' tag takes at most 2 arguments"},
		{
			"partialdef_duplicated_names",
			"{% partialdef testing-name %}TEST{% endpartialdef %}{% partialdef testing-name %}TEST{% endpartialdef %}{% partial testing-name %}",
			"Partial 'testing-name' is already defined in the 'partialdef_duplicated_names' template.",
		},
		{
			"partialdef_duplicated_nested_names",
			"{% partialdef testing-name %}TEST{% partialdef testing-name %}TEST{% endpartialdef %}{% endpartialdef %}{% partial testing-name %}",
			"Partial 'testing-name' is already defined in the 'partialdef_duplicated_nested_names' template.",
		},
		{"with_params", "{% partialdef testing-name inline=true %}TEST{% endpartialdef %}", "The 'inline' argument does not have any parameters"},
		{"uppercase", "{% partialdef testing-name INLINE %}TEST{% endpartialdef %}", "The 'inline' argument does not have any parameters"},
		{
			"partial_broken_unclosed",
			"<div>Before partial</div>{% partialdef unclosed_partial %}<p>This partial has no closing tag</p><div>After partial content</div>",
			"endpartialdef",
		},
		{
			"partial_with_undefined_reference", "<h1>Header</h1>\n{% partial undefined %}\n<p>After undefined partial</p>\n",
			"Partial 'undefined' is not defined in the current template.",
		},
		{
			"partial_with_syntax_error",
			"<h1>Title</h1>\n{% partialdef syntax_error_partial %}\n    {% if user %}\n        <p>User: {{ user.name }}</p>\n" +
				"    {% endif\n    <p>Missing closing tag above</p>\n{% endpartialdef %}\n{% partial syntax_error_partial %}\n",
			"endpartialdef",
		},
		{
			"child.html",
			"{% extends 'parent.html' %}\n{% block content %}\n    {% partialdef content_partial %}\n" +
				"        <p>{{ missing_variable|undefined_filter }}</p>\n    {% endpartialdef %}\n    {% partial content_partial %}\n{% endblock %}\n",
			"undefined_filter",
		},
		{
			"partial_broken_nesting",
			"<div>Before partial</div>\n{% partialdef outer %}\n{% partialdef inner %}...{% endpartialdef outer %}\n{% endpartialdef inner %}\n<div>After partial content</div>",
			"'endpartialdef inner'",
		},
		{
			"partial_broken_nesting_mixed",
			"<div>Before partial</div>\n{% partialdef outer %}\n{% partialdef inner %}...{% endpartialdef %}\n{% endpartialdef inner %}\n<div>After partial content</div>",
			"'endpartialdef outer'",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			engine := partialEngine(map[string]string{
				tc.name:       tc.src,
				"parent.html": "<!DOCTYPE html>\n<html>\n<head>{% block title %}Default Title{% endblock %}</head>\n<body>\n    {% block content %}{% endblock %}\n</body>\n</html>\n",
			})
			got, err := engine.Render(tc.name, nil)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("%s: got %q, %v; want an error with %q", tc.src, got, err, tc.wantErr)
			}
		})
	}
}

// TestPartialLoading ports Django's tests of loading "name#partial".
//
// Ported from Django: tests/template_tests/syntax_tests/test_partials.py::PartialTagTests.test_undefined_partial_exception_info_template_does_not_exist
// Ported from Django: tests/template_tests/test_partials.py::PartialTagsTests.test_invalid_template_name_raises_template_does_not_exist
// Ported from Django: tests/template_tests/test_partials.py::PartialTagsTests.test_full_template_from_loader
// Ported from Django: tests/template_tests/test_partials.py::PartialTagsTests.test_chained_exception_forwarded
// Ported from Django: tests/template_tests/test_partials.py::PartialTagsTests.test_context_available_in_response_for_partial_template
// Ported from Django: tests/template_tests/test_partials.py::PartialTagsTests.test_response_with_multiple_parts
// Ported from Django: tests/template_tests/test_partials.py::PartialTagsTests.test_partial_engine_assignment_with_real_template
//
// No Go counterpart: a template name that is not a string (123, None),
// PartialTemplate.source and its offsets (FindPartialSourceTests,
// test_template_source_warning), templates without extra_data
// (RobustPartialHandlingTests) and the cached loader settings
// (test_partials_use_cached_loader_when_configured).
func TestPartialLoading(t *testing.T) {
	engine := templates.New(templates.MapSource("locmem", map[string]string{
		"existing_template":     "<h1>Header</h1><p>This template has no partials defined</p>",
		"partial_base.html":     partialFiles["partial_base.html"],
		"partial_child.html":    partialFiles["partial_child.html"],
		"partial_examples.html": partialFiles["partial_examples.html"],
	}))
	missing := func(t *testing.T, name, want string) *templates.TemplateDoesNotExist {
		t.Helper()
		_, err := engine.Render(name, nil)
		var tdne *templates.TemplateDoesNotExist
		if !errors.As(err, &tdne) || !strings.Contains(err.Error(), want) {
			t.Fatalf("Render(%q) = %v, want a TemplateDoesNotExist with %q", name, err, want)
		}
		return tdne
	}
	t.Run("test_undefined_partial_exception_info_template_does_not_exist", func(t *testing.T) {
		missing(t, "existing_template#undefined", "undefined")
	})
	t.Run("test_invalid_template_name_raises_template_does_not_exist", func(t *testing.T) {
		for _, name := range []string{"", "#", "#name"} {
			missing(t, name, name)
		}
	})
	t.Run("test_full_template_from_loader", func(t *testing.T) {
		out, err := engine.Render("partial_examples.html", nil)
		if err != nil {
			t.Fatal(err)
		}
		if n := strings.Count(out, "TEST-PARTIAL-CONTENT"); n != 2 {
			t.Errorf("TEST-PARTIAL-CONTENT printed %d times, want 2: %q", n, out)
		}
		if n := strings.Count(out, "INLINE-CONTENT"); n != 1 {
			t.Errorf("INLINE-CONTENT printed %d times, want 1: %q", n, out)
		}
	})
	t.Run("test_chained_exception_forwarded", func(t *testing.T) {
		tdne := missing(t, "not_there.html#not-a-partial", "not_there.html")
		if len(tdne.Tried) == 0 || !strings.Contains(tdne.Tried[0].Origin, "not_there.html") {
			t.Errorf("Tried = %+v, want not_there.html first", tdne.Tried)
		}
	})
	t.Run("test_context_available_in_response_for_partial_template", func(t *testing.T) {
		out, err := engine.Render("partial_examples.html#test-partial", map[string]any{"foo": "bar"})
		if err != nil || !strings.Contains(out, "TEST-PARTIAL-CONTENT") {
			t.Errorf("Render = %q, %v; want TEST-PARTIAL-CONTENT", out, err)
		}
	})
	t.Run("test_response_with_multiple_parts", func(t *testing.T) {
		var all strings.Builder
		for _, name := range []string{"partial_child.html", "partial_child.html#extra-content"} {
			out, err := engine.Render(name, nil)
			if err != nil {
				t.Fatal(err)
			}
			all.WriteString(out)
		}
		for _, want := range []string{"Main Content", "Extra Content"} {
			if !strings.Contains(all.String(), want) {
				t.Errorf("output %q lacks %q", all.String(), want)
			}
		}
	})
	t.Run("test_partial_engine_assignment_with_real_template", func(t *testing.T) {
		out, err := engine.Render("partial_examples.html#test-partial", nil)
		if err != nil || strings.TrimSpace(out) != "TEST-PARTIAL-CONTENT" {
			t.Errorf("Render = %q, %v; want TEST-PARTIAL-CONTENT", out, err)
		}
	})
}

// TestPartialContextAndSource ports Django's tests of the context a nested
// partial sees, of duplicate names, and of partialdef tags inside verbatim
// and comment blocks. Django checks the source of the partial there; here
// the partial loads by "#name" and renders its own content.
//
// Ported from Django: tests/template_tests/test_partials.py::RobustPartialHandlingTests.test_nested_partials_rendering_with_context
// Ported from Django: tests/template_tests/test_partials.py::FindPartialSourceTests.test_partials_with_duplicate_names
// Ported from Django: tests/template_tests/test_partials.py::FindPartialSourceTests.test_partial_template_embedded_in_verbatim
// Ported from Django: tests/template_tests/test_partials.py::FindPartialSourceTests.test_partial_template_embedded_in_comment_block
// Ported from Django: tests/template_tests/test_partials.py::FindPartialSourceTests.test_partial_template_embedded_in_inline_comment
// Ported from Django: tests/template_tests/test_partials.py::FindPartialSourceTests.test_partial_template_contains_fake_end_inside_verbatim
func TestPartialContextAndSource(t *testing.T) {
	t.Run("test_nested_partials_rendering_with_context", func(t *testing.T) {
		src := "\n        {% partialdef outer inline %}\n            Hello {{ name }}!\n" +
			"            {% partialdef inner inline %}\n                Your age is {{ age }}.\n" +
			"            {% endpartialdef inner %}\n            Nice to meet you.\n        {% endpartialdef outer %}\n        "
		out, err := templates.New(templates.MapSource("locmem", map[string]string{"template.html": src})).
			Render("template.html", map[string]any{"name": "Alice", "age": 25})
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{"Hello Alice!", "Your age is 25.", "Nice to meet you."} {
			if !strings.Contains(out, want) {
				t.Errorf("output %q lacks %q", out, want)
			}
		}
	})
	t.Run("test_partials_with_duplicate_names", func(t *testing.T) {
		for _, src := range []string{
			"\n{% partialdef duplicate %}{% partialdef duplicate %}\nCONTENT\n{% endpartialdef %}{% endpartialdef %}\n",
			"\n{% if x %}\n  {% partialdef duplicate %}\n  CONTENT\n  {% endpartialdef %}\n{% else %}\n" +
				"  {% partialdef duplicate %}\n  OTHER-CONTENT\n  {% endpartialdef %}\n{% endif %}\n",
		} {
			_, err := templates.New(templates.MapSource("locmem", map[string]string{"template.html": src})).Render("template.html", nil)
			if want := "Partial 'duplicate' is already defined in the 'template.html' template."; err == nil || !strings.Contains(err.Error(), want) {
				t.Errorf("%s: %v, want an error with %q", src, err, want)
			}
		}
	})
	for _, tc := range []struct{ name, src, want, notWant string }{
		{
			"test_partial_template_embedded_in_verbatim",
			"{% verbatim %}\n{% partialdef testing-name %}\n<p>Should be ignored</p>{% endpartialdef testing-name %}\n{% endverbatim %}\n" +
				"{% partialdef testing-name %}\n<p>Content</p>\n{% endpartialdef %}\n",
			"\n<p>Content</p>\n", "",
		},
		{
			"test_partial_template_embedded_in_comment_block",
			"{% comment %}\n{% partialdef testing-name %}\n{% endcomment %}\n{% partialdef testing-name %}\n<p>Comment Content</p>\n{% endpartialdef %}\n",
			"\n<p>Comment Content</p>\n", "",
		},
		{
			"test_partial_template_embedded_in_inline_comment",
			"{# {% partialdef testing-name %} #}\n{% partialdef testing-name %}\n<p>Inline Comment Content</p>\n{% endpartialdef %}\n",
			"\n<p>Inline Comment Content</p>\n", "",
		},
		{
			"test_partial_template_contains_fake_end_inside_verbatim",
			"{% partialdef testing-name %}\n{% verbatim %}{% endpartialdef %}{% endverbatim %}\n<p>Body</p>\n{% endpartialdef %}\n",
			"\n{% endpartialdef %}\n<p>Body</p>\n", "",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, err := templates.New(templates.MapSource("locmem", map[string]string{"t": tc.src})).Render("t#testing-name", nil)
			if err != nil {
				t.Fatal(err)
			}
			if out != tc.want {
				t.Errorf("t#testing-name = %q, want %q", out, tc.want)
			}
		})
	}
}

// TestPartialRecursion stops a partial that renders itself with an error,
// where Django raises RecursionError.
//
// No Django test exists for a recursive partial.
func TestPartialRecursion(t *testing.T) {
	_, err := partialEngine(map[string]string{"t": "{% partialdef a %}{% partial a %}{% endpartialdef %}{% partial a %}"}).Render("t", nil)
	if err == nil || !strings.Contains(err.Error(), "maximum") {
		t.Fatalf("got %v, want a depth error", err)
	}
}
