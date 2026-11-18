package templates_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/templates"
)

// ctxAuthor is the target of the relation in the conversion tests.
type ctxAuthor struct {
	ID   int64
	Name string
}

// String is the label of an author.
func (a ctxAuthor) String() string { return a.Name }

// ctxPost has every kind of attribute a template can ask a model for.
type ctxPost struct {
	ID        int64
	Title     string
	Status    string
	CreatedAt int64
	Author    orm.FK[ctxAuthor]
}

// String is the label of a post; the markup in it must not reach the page raw.
func (p ctxPost) String() string { return "<b>" + p.Title + "</b>" }

// Choices declares the status options.
func (ctxPost) Choices() map[string][]orm.Choice {
	return map[string][]orm.Choice{"Status": {{Value: "d", Label: "Draft"}, {Value: "p", Label: "Published"}}}
}

// GetAbsoluteURL is the canonical URL, reached as get_absolute_url.
func (p *ctxPost) GetAbsoluteURL() string { return "/posts/" + p.Title + "/" }

// Related returns other posts, to check that method results are converted.
func (p ctxPost) Related() []ctxPost { return []ctxPost{{ID: 9, Title: "Other"}} }

// Broken fails, to check that a method error fails the render.
func (ctxPost) Broken() (string, error) { return "", errors.New("broken method") }

// WithArg takes a parameter and is therefore not callable from a template.
func (ctxPost) WithArg(int) string { return "never" }

// ctxRegistry registers the test models.
func ctxRegistry(t *testing.T) *orm.Registry {
	t.Helper()
	r := orm.NewRegistry()
	for _, m := range []any{&ctxAuthor{}, &ctxPost{}} {
		if _, err := r.Register(m); err != nil {
			t.Fatalf("register: %v", err)
		}
	}
	if err := r.Resolve(); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	return r
}

// modelEngine returns a string engine that recognizes the test models.
func modelEngine(t *testing.T) *templates.Engine {
	t.Helper()
	engine := templates.New()
	engine.SetModels(ctxRegistry(t))
	return engine
}

// renderStr renders a source string and fails the test on an error.
func renderStr(t *testing.T, engine *templates.Engine, src string, ctx map[string]any) string {
	t.Helper()
	out, err := engine.RenderString(src, ctx)
	if err != nil {
		t.Fatalf("render %q: %v", src, err)
	}
	return out
}

// TestModelInContext checks the Django names a template uses on a model
// instance that sits in the context as a plain struct or pointer.
func TestModelInContext(t *testing.T) {
	engine := modelEngine(t)
	post := ctxPost{ID: 7, Title: "Hi", Status: "p", CreatedAt: 99}
	post.Author.SetObject(&ctxAuthor{ID: 3, Name: "Ann & Bo"}, 3)

	tests := []struct {
		src  string
		want string
	}{
		{`{{ post.title }}`, "Hi"},
		{`{{ post.created_at }}`, "99"},
		{`{{ post.pk }}|{{ post.id }}`, "7|7"},
		{`{{ post.author_id }}`, "3"},
		{`{{ post.author.name }}`, "Ann &amp; Bo"},
		{`{{ post.author }}`, "Ann &amp; Bo"},
		{`{{ post.author.pk }}`, "3"},
		{`{{ post.status }}|{{ post.get_status_display }}`, "p|Published"},
		{`{{ post.get_absolute_url }}`, "/posts/Hi/"},
		{`{{ post }}`, "&lt;b&gt;Hi&lt;/b&gt;"},
		{`{% for p in post.related %}{{ p.title }}:{{ p.pk }}{% endfor %}`, "Other:9"},
		{`[{{ post.with_arg }}]`, "[]"},
		{`[{{ post.no_such_attribute }}]`, "[]"},
	}
	for _, form := range []struct {
		name  string
		value any
	}{{"struct", post}, {"pointer", &post}} {
		for _, tt := range tests {
			if got := renderStr(t, engine, tt.src, map[string]any{"post": form.value}); got != tt.want {
				t.Errorf("%s: %s = %q, want %q", form.name, tt.src, got, tt.want)
			}
		}
	}
}

// TestModelMethodError checks that an error returned by a model method fails
// the render.
func TestModelMethodError(t *testing.T) {
	_, err := modelEngine(t).RenderString(`{{ post.broken }}`, map[string]any{"post": ctxPost{}})
	if err == nil {
		t.Fatal("render succeeded, want the method's error")
	}
}

// TestUnloadedRelation checks a relation the query did not load: the key is
// there, the object is not, and nothing is fetched behind the template's back.
func TestUnloadedRelation(t *testing.T) {
	engine := modelEngine(t)
	var post ctxPost
	post.Author.SetPK(3)
	got := renderStr(t, engine, `{{ post.author_id }}[{{ post.author }}][{{ post.author.name }}]`, map[string]any{"post": post})
	if want := "3[][]"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	got = renderStr(t, engine, `[{{ post.author_id }}]{% if post.author %}has{% else %}none{% endif %}`,
		map[string]any{"post": ctxPost{}})
	if want := "[]none"; got != want {
		t.Errorf("unset relation: got %q, want %q", got, want)
	}
}

// ctxPage shows how a type of another package presents itself to templates.
type ctxPage struct {
	Items []ctxPost
	Num   int
}

// TemplateValue gives the page Django's attribute names.
func (p ctxPage) TemplateValue() any {
	return map[string]any{"object_list": p.Items, "number": p.Num}
}

// TestModelsInContainers checks that instances are found in slices, in nested
// maps and behind a ContextValuer.
func TestModelsInContainers(t *testing.T) {
	engine := modelEngine(t)
	posts := []ctxPost{{ID: 1, Title: "a"}, {ID: 2, Title: "b"}}
	const loop = `{% for p in posts %}{{ p.title }}{{ p.pk }} {% endfor %}`
	tests := []struct {
		name string
		src  string
		ctx  map[string]any
		want string
	}{
		{"slice of structs", loop, map[string]any{"posts": posts}, "a1 b2 "},
		{"slice of pointers", loop, map[string]any{"posts": []*ctxPost{&posts[0], &posts[1]}}, "a1 b2 "},
		{"slice of any", loop, map[string]any{"posts": []any{posts[0], &posts[1]}}, "a1 b2 "},
		{
			"nested map", `{{ page.first.title }}/{% for p in page.rest %}{{ p.title }}{% endfor %}`,
			map[string]any{"page": map[string]any{"first": posts[0], "rest": posts[1:]}},
			"a/b",
		},
		{
			"context valuer", `{{ page.number }}:{% for p in page.object_list %}{{ p.title }}{% endfor %}`,
			map[string]any{"page": ctxPage{Items: posts, Num: 4}},
			"4:ab",
		},
		{"plain values stay", `{{ names|join:"," }}{{ n }}`, map[string]any{"names": []string{"x", "y"}, "n": 5}, "x,y5"},
	}
	for _, tt := range tests {
		if got := renderStr(t, engine, tt.src, tt.ctx); got != tt.want {
			t.Errorf("%s: got %q, want %q", tt.name, got, tt.want)
		}
	}
	if len(posts) != 2 || posts[0].Title != "a" {
		t.Errorf("the caller's slice was changed: %v", posts)
	}
}

// TestModelContextPublicForm checks the map ModelContext hands to callers:
// Django's names, nested objects as ModelMap, and a raw label.
func TestModelContextPublicForm(t *testing.T) {
	reg := ctxRegistry(t)
	m, _ := reg.ModelOf(&ctxPost{})
	post := ctxPost{ID: 7, Title: "Hi", Status: "d"}
	post.Author.SetObject(&ctxAuthor{ID: 3, Name: "Ann"}, 3)
	mm := templates.ModelContext(m, &post)

	if mm["title"] != "Hi" || mm["pk"] != int64(7) || mm["author_id"] != int64(3) || mm["get_status_display"] != "Draft" {
		t.Errorf("attributes = %v", mm)
	}
	author, ok := mm["author"].(templates.ModelMap)
	if !ok || author["name"] != "Ann" {
		t.Errorf("author = %#v, want a ModelMap", mm["author"])
	}
	if got, want := mm.String(), "<b>Hi</b>"; got != want {
		t.Errorf("String() = %q, want the raw label %q", got, want)
	}
	// In a template the same map prints its label escaped.
	if got := renderStr(t, templates.New(), `{{ post }}|{{ post.author.name }}`, map[string]any{"post": mm}); got != "&lt;b&gt;Hi&lt;/b&gt;|Ann" {
		t.Errorf("rendered %q", got)
	}
}

// TestGlobalModelLookup checks that an engine without its own lookup uses the
// process-global one.
func TestGlobalModelLookup(t *testing.T) {
	templates.SetModelLookup(ctxRegistry(t))
	t.Cleanup(func() { templates.SetModelLookup(nil) })
	got := renderStr(t, templates.New(), `{{ post.title }}`, map[string]any{"post": ctxPost{Title: "global"}})
	if got != "global" {
		t.Errorf("got %q", got)
	}
}

// TestRecorderSeesExtendsAndIncludes checks the recorded renders of one
// request: the template asked for, the templates it extends, then the
// includes with the context each one saw.
func TestRecorderSeesExtendsAndIncludes(t *testing.T) {
	engine := templates.New(templates.MapSource("app", map[string]string{
		"base.html":   `<html>{% block content %}{% endblock %}{% include "footer.html" only %}</html>`,
		"layout.html": `{% extends "base.html" %}`,
		"page.html": `{% extends "layout.html" %}{% block content %}` +
			`{% for item in items %}{% include "item.html" with label=item %}{% endfor %}{% endblock %}`,
		"item.html":      `{% extends "item_base.html" %}{% block label %}{{ label }}{% endblock %}`,
		"item_base.html": `<li>{% block label %}{% endblock %}</li>`,
		"footer.html":    `<footer></footer>`,
	}))
	rec := &templates.Recorder{}
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r = r.WithContext(templates.WithRecorder(r.Context(), rec))
	out, err := engine.RenderRequest(r, "page.html", map[string]any{"items": []string{"a", "b"}, "title": "T"})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if want := "<html><li>a</li><li>b</li><footer></footer></html>"; out != want {
		t.Fatalf("got %q, want %q", out, want)
	}

	got := rec.Rendered()
	want := []string{
		"page.html", "layout.html", "base.html",
		"item.html", "item_base.html", "item.html", "item_base.html", "footer.html",
	}
	if len(got) != len(want) {
		t.Fatalf("recorded %d renders, want %d: %v", len(got), len(want), got)
	}
	for i, name := range want {
		if got[i].Name != name {
			t.Errorf("render %d is %q, want %q", i, got[i].Name, name)
		}
	}
	for i := range 3 {
		if got[i].Context["title"] != "T" {
			t.Errorf("%s: context = %v, want the context of the render", got[i].Name, got[i].Context)
		}
	}
	if got[3].Context["label"] != "a" || got[5].Context["label"] != "b" || got[3].Context["title"] != "T" {
		t.Errorf("include contexts = %v and %v", got[3].Context, got[5].Context)
	}
	if footer := got[7].Context; len(footer) != 0 {
		t.Errorf("an include with only saw %v, want nothing", footer)
	}
	for _, r := range got {
		for key := range r.Context {
			if len(key) > 1 && key[:2] == "__" {
				t.Errorf("%s: internal key %q leaked into the recorded context", r.Name, key)
			}
		}
	}
}

// TestRecorderKeepsCallerValues checks that the recorded context holds the
// values the view passed, not the converted template form, so a test can
// compare against its own objects.
func TestRecorderKeepsCallerValues(t *testing.T) {
	engine := templates.New(templates.MapSource("app", map[string]string{"p.html": `{{ post.title }}`}))
	engine.SetModels(ctxRegistry(t))
	rec := &templates.Recorder{}
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r = r.WithContext(templates.WithRecorder(r.Context(), rec))
	post := &ctxPost{ID: 1, Title: "kept"}
	if _, err := engine.RenderRequest(r, "p.html", map[string]any{"post": post}); err != nil {
		t.Fatalf("render: %v", err)
	}
	if got := rec.Rendered()[0].Context["post"]; got != post {
		t.Errorf("recorded post = %#v, want the pointer the view passed", got)
	}
}
