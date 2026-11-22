package templates_test

import (
	"errors"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/oliverhaas/djangogo/templates"
	"github.com/oliverhaas/djangogo/urls"
)

// basicTemplates are the templates of Django's test_basic the include tests
// pull in.
var basicTemplates = map[string]string{
	"basic-syntax01": "something cool",
	"basic-syntax02": "{{ headline }}",
	"basic-syntax03": "{{ first }} --- {{ second }}",
}

// memEngine returns an engine over in-memory templates: basicTemplates plus
// the given ones.
func memEngine(named map[string]string) *templates.Engine {
	all := make(map[string]string, len(basicTemplates)+len(named))
	for k, v := range basicTemplates {
		all[k] = v
	}
	for k, v := range named {
		all[k] = v
	}
	return templates.New(templates.MapSource("locmem", all))
}

// TestIncludeTag ports the rendering cases of Django's IncludeTagTests.
//
// include13 and include14 differ from Django on purpose: pongo2 starts every
// included template with autoescaping on, so an include inside
// {% autoescape off %} still escapes ("&amp;" where Django prints "&").
//
// Ported from Django: tests/template_tests/syntax_tests/test_include.py::IncludeTagTests.test_include01
// Ported from Django: tests/template_tests/syntax_tests/test_include.py::IncludeTagTests.test_include02
// Ported from Django: tests/template_tests/syntax_tests/test_include.py::IncludeTagTests.test_include03
// Ported from Django: tests/template_tests/syntax_tests/test_include.py::IncludeTagTests.test_include06
// Ported from Django: tests/template_tests/syntax_tests/test_include.py::IncludeTagTests.test_include07
// Ported from Django: tests/template_tests/syntax_tests/test_include.py::IncludeTagTests.test_include08
// Ported from Django: tests/template_tests/syntax_tests/test_include.py::IncludeTagTests.test_include09
// Ported from Django: tests/template_tests/syntax_tests/test_include.py::IncludeTagTests.test_include10
// Ported from Django: tests/template_tests/syntax_tests/test_include.py::IncludeTagTests.test_include11
// Ported from Django: tests/template_tests/syntax_tests/test_include.py::IncludeTagTests.test_include12
// Ported from Django: tests/template_tests/syntax_tests/test_include.py::IncludeTagTests.test_include13
// Ported from Django: tests/template_tests/syntax_tests/test_include.py::IncludeTagTests.test_include14
func TestIncludeTag(t *testing.T) {
	tests := []struct {
		name string
		src  string
		ctx  map[string]any
		want string
	}{
		{"include01", `{% include "basic-syntax01" %}`, nil, "something cool"},
		{"include02", `{% include "basic-syntax02" %}`, map[string]any{"headline": "Included"}, "Included"},
		{
			"include03", `{% include template_name %}`,
			map[string]any{"template_name": "basic-syntax02", "headline": "Included"},
			"Included",
		},
		{"include06", `{% include "include 05"%}`, nil, "template with a space"},
		{
			"include07", `{% include "basic-syntax02" with headline="Inline" %}`,
			map[string]any{"headline": "Included"},
			"Inline",
		},
		{
			"include08", `{% include headline with headline="Dynamic" %}`,
			map[string]any{"headline": "basic-syntax02"},
			"Dynamic",
		},
		{
			"include09",
			`{{ first }}--{% include "basic-syntax03" with first=second|lower|upper second=first|upper %}--{{ second }}`,
			map[string]any{"first": "Ul", "second": "lU"},
			"Ul--LU --- UL--lU",
		},
		{"include10", `{% include "basic-syntax03" only %}`, map[string]any{"first": "1"}, " --- "},
		{"include11", `{% include "basic-syntax03" only with second=2 %}`, map[string]any{"first": "1"}, " --- 2"},
		{"include12", `{% include "basic-syntax03" with first=1 only %}`, map[string]any{"second": "2"}, "1 --- "},
		{
			"include13", `{% autoescape off %}{% include "basic-syntax03" %}{% endautoescape %}`,
			map[string]any{"first": "&"},
			"&amp; --- ",
		},
		{
			"include14", `{% autoescape off %}{% include "basic-syntax03" with first=var1 only %}{% endautoescape %}`,
			map[string]any{"var1": "&"},
			"&amp; --- ",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			engine := memEngine(map[string]string{tt.name: tt.src, "include 05": "template with a space"})
			got, err := engine.Render(tt.name, tt.ctx)
			if err != nil {
				t.Fatalf("render: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

// TestIncludeTagMissing checks that a missing include fails the render, not
// the load, and names the missing template.
//
// Ported from Django: tests/template_tests/syntax_tests/test_include.py::IncludeTagTests.test_include04
// Ported from Django: tests/template_tests/syntax_tests/test_include.py::IncludeTests.test_include_missing_template
// Ported from Django: tests/template_tests/syntax_tests/test_include.py::IncludeTests.test_include_immediate_missing
func TestIncludeTagMissing(t *testing.T) {
	engine := memEngine(map[string]string{
		"include04":               `a{% include "nonexistent" %}b`,
		"test_include_error.html": `{% include "missing.html" %}`,
	})
	if _, err := engine.SelectTemplate("include04"); err != nil {
		t.Fatalf("a template with a missing include must load: %v", err)
	}
	_, err := engine.Render("include04", nil)
	if miss := wantMissing(t, err); miss.Name != "nonexistent" {
		t.Errorf("Name = %q, want the included template", miss.Name)
	}
	_, err = engine.Render("test_include_error.html", nil)
	if miss := wantMissing(t, err); miss.Name != "missing.html" || !strings.Contains(err.Error(), "missing.html") {
		t.Errorf("error = %v, want it to name missing.html", err)
	}
	if _, err := engine.RenderString(`{% if false %}{% include "this_does_not_exist.html" %}{% endif %}`, nil); err != nil {
		t.Errorf("an include that is not reached must not fail: %v", err)
	}
}

// TestIncludeTagSyntaxErrors ports the malformed include tags. They fail when
// the template is loaded.
//
// Ported from Django: tests/template_tests/syntax_tests/test_include.py::IncludeTagTests.test_include_error01
// Ported from Django: tests/template_tests/syntax_tests/test_include.py::IncludeTagTests.test_include_error02
// Ported from Django: tests/template_tests/syntax_tests/test_include.py::IncludeTagTests.test_include_error03
// Ported from Django: tests/template_tests/syntax_tests/test_include.py::IncludeTagTests.test_include_error04
// Ported from Django: tests/template_tests/syntax_tests/test_include.py::IncludeTagTests.test_include_error05
// Ported from Django: tests/template_tests/syntax_tests/test_include.py::IncludeTagTests.test_include_error06
// Ported from Django: tests/template_tests/syntax_tests/test_include.py::IncludeTagTests.test_include_empty
func TestIncludeTagSyntaxErrors(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{"include-error01", `{% include "basic-syntax01" with %}`, `"with" in 'include' tag needs at least one keyword argument.`},
		{"include-error02", `{% include "basic-syntax01" with "no key" %}`, `"with" in 'include' tag needs at least one keyword argument.`},
		{"include-error03", `{% include "basic-syntax01" with dotted.arg="error" %}`, `"with" in 'include' tag needs at least one keyword argument.`},
		{"include-error04", `{% include "basic-syntax01" something_random %}`, `Unknown argument for 'include' tag: 'something_random'.`},
		{"include-error05", `{% include "basic-syntax01" foo="duplicate" foo="key" %}`, `Unknown argument for 'include' tag: 'foo'.`},
		{"include-error06", `{% include "basic-syntax01" only only %}`, `The 'only' option was specified more than once.`},
		{"include_empty", `{% include %}`, `'include' tag takes at least one argument: the name of the template to be included.`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := memEngine(map[string]string{tt.name: tt.src}).SelectTemplate(tt.name)
			if err == nil {
				t.Fatal("the template loaded, want a syntax error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error = %q, want it to contain %q", err.Error(), tt.want)
			}
		})
	}
}

// TestIncludeErrorInIncluded checks that a broken template fails the render
// of the template that includes it, by a literal name or by a variable.
// Django's cases use a broken tag library; the broken template here has a tag
// that does not exist.
//
// Ported from Django: tests/template_tests/syntax_tests/test_include.py::IncludeTagTests.test_include_error08
// Ported from Django: tests/template_tests/syntax_tests/test_include.py::IncludeTagTests.test_include_error10
func TestIncludeErrorInIncluded(t *testing.T) {
	engine := memEngine(map[string]string{
		"include-fail2":   `{% no_such_tag %}`,
		"include-error08": `{% include "include-fail2" %}`,
		"include-error10": `{% include failed_include %}`,
	})
	for _, name := range []string{"include-error08", "include-error10"} {
		_, err := engine.Render(name, map[string]any{"failed_include": "include-fail2"})
		if err == nil || !strings.Contains(err.Error(), "no_such_tag") {
			t.Errorf("%s: error = %v, want the syntax error of the included template", name, err)
		}
	}
}

// TestIncludeTemplateIterable checks a variable holding several names: the
// first that exists is taken. Django also accepts a tuple; a Go slice or array
// stands for both.
//
// Ported from Django: tests/template_tests/syntax_tests/test_include.py::IncludeTests.test_include_template_iterable
func TestIncludeTemplateIterable(t *testing.T) {
	engine := memEngine(map[string]string{"index.html": "index\n"})
	for _, names := range []any{
		[]string{"admin/fail.html", "index.html"},
		[2]string{"admin/fail.html", "index.html"},
		[]any{"admin/fail.html", "index.html"},
	} {
		got, err := engine.RenderString(`{% include var %}`, map[string]any{"var": names})
		if err != nil {
			t.Fatalf("render with %v: %v", names, err)
		}
		if got != "index\n" {
			t.Errorf("render with %v = %q, want %q", names, got, "index\n")
		}
	}
}

// TestIncludeTemplateNone checks an include whose variable is missing.
//
// Ported from Django: tests/template_tests/syntax_tests/test_include.py::IncludeTests.test_include_template_none
func TestIncludeTemplateNone(t *testing.T) {
	engine := memEngine(nil)
	for _, ctx := range []map[string]any{{"var": nil}, {}} {
		_, err := engine.RenderString(`{% include var %}`, ctx)
		wantMissing(t, err)
		if !strings.Contains(err.Error(), "No template names provided") {
			t.Errorf("error = %q, want Django's message", err.Error())
		}
	}
}

// TestIncludeRecursive checks a template that includes itself for each level
// of a tree. Django's variants with a relative name and with a template object
// in the context have no counterpart here.
//
// Ported from Django: tests/template_tests/syntax_tests/test_include.py::IncludeTests.test_include_recursive
func TestIncludeRecursive(t *testing.T) {
	leaf := func(name string, children ...map[string]any) map[string]any {
		list := make([]map[string]any, 0, len(children))
		list = append(list, children...)
		return map[string]any{"comment": name, "children": list}
	}
	comments := []map[string]any{
		leaf("A1", leaf("B1"), leaf("B2"), leaf("B3", leaf("C1"))),
	}
	engine := memEngine(map[string]string{
		"recursive_include.html": "Recursion!\n" +
			"{% for comment in comments %}\n" +
			"    {{ comment.comment }}\n" +
			"    {% if comment.children %}\n" +
			"        {% include \"recursive_include.html\" with comments=comment.children %}\n" +
			"    {% endif %}\n" +
			"{% endfor %}\n",
	})
	out, err := engine.Render("recursive_include.html", map[string]any{"comments": comments})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	got := strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(out, " ", ""), "\n", " "))
	if want := "Recursion!  A1  Recursion!  B1   B2   B3  Recursion!  C1"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestIncludeWithoutEnd checks that a template including itself without a
// stop condition is a render error and not a stack overflow.
func TestIncludeWithoutEnd(t *testing.T) {
	engine := memEngine(map[string]string{"loop.html": `x{% include "loop.html" %}`})
	_, err := engine.Render("loop.html", nil)
	if err == nil || !strings.Contains(err.Error(), "maximum include depth exceeded") {
		t.Fatalf("error = %v, want the include depth error", err)
	}
}

// TestIncludeCache checks that an included template is compiled once and
// shared by every render of the include.
//
// Ported from Django: tests/template_tests/syntax_tests/test_include.py::IncludeTests.test_include_cache
func TestIncludeCache(t *testing.T) {
	counting := &countingFS{FS: fstest.MapFS{
		"template": {Data: []byte(`{% for x in vars %}{% include "include" %}{% endfor %}`)},
		"include":  {Data: []byte(`{% include "next" %}`)},
		"next":     {Data: []byte(`{{ x }}`)},
	}}
	engine := templates.New(templates.FSSource("locmem", counting))
	got := mustRender(t, engine, "template", map[string]any{"vars": []int{0, 1, 2, 3, 4, 5, 6, 7, 8}})
	if got != "012345678" {
		t.Errorf("got %q, want %q", got, "012345678")
	}
	if n := counting.opens.Load(); n != 3 {
		t.Errorf("the three templates were read %d times, want once each", n)
	}
}

// TestNormalExtend checks inheritance over three levels through the engine's
// sources.
//
// Ported from Django: tests/template_tests/test_extends.py::ExtendsBehaviorTests.test_normal_extend
func TestNormalExtend(t *testing.T) {
	dir := writeTemplates(t, map[string]string{
		"one.html":   "{% extends \"two.html\" %}\n\n{% block content %}{{ block.super }} one{% endblock %}\n",
		"two.html":   "{% extends \"three.html\" %}\n\n{% block content %}{{ block.super }} two{% endblock %}\n",
		"three.html": "{% block content %}three{% endblock %}\n",
	})
	engine := templates.New(templates.DirSource(dir))
	if got := strings.TrimSpace(mustRender(t, engine, "one.html", nil)); got != "three two one" {
		t.Errorf("got %q, want %q", got, "three two one")
	}
}

// TestExtendAcrossSources checks that a child in one source extends a parent
// that only a later source has, the way a project template extends the base
// template of an app.
func TestExtendAcrossSources(t *testing.T) {
	project := templates.MapSource("project", map[string]string{
		"blog/post.html": `{% extends "base.html" %}{% block content %}post{% endblock %}`,
	})
	app := templates.MapSource("app", map[string]string{
		"base.html":      `<main>{% block content %}{% endblock %}</main>`,
		"blog/post.html": `shadowed`,
	})
	engine := templates.New(project, app)
	if got := mustRender(t, engine, "blog/post.html", nil); got != "<main>post</main>" {
		t.Errorf("got %q", got)
	}
}

// TestExtendMissing checks that a missing parent is reported as the template
// that does not exist, with the candidates tried for it.
//
// Ported from Django: tests/template_tests/test_extends.py::ExtendsBehaviorTests.test_extend_missing
func TestExtendMissing(t *testing.T) {
	engine := templates.New(templates.MapSource("fs", map[string]string{
		"extend-missing.html": `{% extends "missing.html" %}`,
	}))
	_, err := engine.Render("extend-missing.html", nil)
	miss := wantMissing(t, err)
	if miss.Name != "missing.html" {
		t.Errorf("Name = %q, want missing.html", miss.Name)
	}
	if len(miss.Tried) != 1 || miss.Tried[0].Origin != "fs:missing.html" {
		t.Errorf("Tried = %v, want the one candidate fs:missing.html", miss.Tried)
	}
}

// TestExtendsNotFirstTag checks that {% extends %} after another tag is an
// error that names the template.
//
// Ported from Django: tests/template_tests/test_extends.py::ExtendsBehaviorTests.test_extends_not_first_tag_in_extended_template
// Ported from Django: tests/template_tests/test_extends.py::ExtendsBehaviorTests.test_extends_not_first_tag_in_extended_template_from_string
func TestExtendsNotFirstTag(t *testing.T) {
	const src = "{% block content %}B{% endblock %}{% extends 'base.html' %}"
	engine := templates.New(templates.MapSource("locmem", map[string]string{"index.html": src}))
	_, err := engine.Render("index.html", nil)
	want := "{% extends 'base.html' %} must be the first tag in 'index.html'."
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Errorf("index.html: error = %v, want one with %q", err, want)
	}
	_, err = engine.RenderString(src, nil)
	want = "{% extends 'base.html' %} must be the first tag in the template."
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Errorf("string: error = %v, want one with %q", err, want)
	}
}

// TestExtendsIncludeMissing checks that the missing template is named when a
// parent that exists includes one that does not.
//
// Ported from Django: tests/template_tests/syntax_tests/test_include.py::IncludeTests.test_extends_include_missing_baseloader
// Ported from Django: tests/template_tests/syntax_tests/test_include.py::IncludeTests.test_extends_include_missing_cachedloader
func TestExtendsIncludeMissing(t *testing.T) {
	engine := templates.New(templates.MapSource("app", map[string]string{
		"test_extends_error.html": `{% extends "broken_base.html" %}`,
		"broken_base.html":        `{% include "missing.html" %}`,
	}))
	// The second render takes the templates from the cache.
	for range 2 {
		_, err := engine.Render("test_extends_error.html", nil)
		if miss := wantMissing(t, err); miss.Name != "missing.html" {
			t.Errorf("Name = %q, want missing.html", miss.Name)
		}
	}
}

// TestRenderErrorUnwraps checks that the cause of a render error is reachable
// through the error chain, here the NoReverseMatch of a {% url %} inside a
// block of an included template that extends another.
func TestRenderErrorUnwraps(t *testing.T) {
	engine := templates.New(templates.MapSource("app", map[string]string{
		"included_base.html":    `{% block content %}{% block error_here %}{% endblock %}{% endblock %}`,
		"included_content.html": `{% extends "included_base.html" %}{% block error_here %}{% url "nonexistent_url" %}{% endblock %}`,
		"page.html":             `{% include "included_content.html" %}`,
	}))
	router := urls.NewRouter()
	engine.SetResolver(router.Reverse)
	_, err := engine.Render("page.html", nil)
	if !errors.Is(err, urls.ErrNoReverseMatch) {
		t.Fatalf("error = %v, want a NoReverseMatch in the chain", err)
	}
}
