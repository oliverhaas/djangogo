package templates_test

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/templates"
)

// recursiveTemplates are the templates of Django's
// tests/template_tests/recursive_templates, by directory.
var recursiveTemplates = map[string]map[string]string{
	"fs": {
		"one.html":             "{% extends \"two.html\" %}\n\n{% block content %}{{ block.super }} one{% endblock %}\n",
		"two.html":             "{% extends \"three.html\" %}\n\n{% block content %}{{ block.super }} two{% endblock %}\n",
		"three.html":           "{% block content %}three{% endblock %}\n",
		"recursive.html":       "{% extends \"recursive.html\" %}\n\n{% block content %}{{ block.super }} fs/recursive{% endblock %}\n",
		"other-recursive.html": "{% extends \"recursive.html\" %}\n",
		"extend-missing.html":  "{% extends \"missing.html\" %}\n",
		"self.html":            "{% extends \"self.html\" %}\n",
	},
	"fs2": {
		"recursive.html": "{% extends \"recursive.html\" %}\n\n{% block content %}{{ block.super }} fs2/recursive{% endblock %}\n",
	},
	"fs3": {
		"recursive.html": "{% block content %}fs3/recursive{% endblock %}\n",
	},
}

// recursiveSources returns a directory source for each of dirs, with the
// templates recursiveTemplates gives that directory.
func recursiveSources(t *testing.T, dirs ...string) []templates.Source {
	t.Helper()
	sources := make([]templates.Source, len(dirs))
	for i, dir := range dirs {
		sources[i] = templates.DirSource(writeTemplates(t, recursiveTemplates[dir]))
	}
	return sources
}

// TestExtendRecursive checks a template that extends its namesake: the
// parent is the template of that name in a later source.
//
// Ported from Django: tests/template_tests/test_extends.py::ExtendsBehaviorTests.test_extend_recursive
// Ported from Django: tests/template_tests/test_extends.py::ExtendsBehaviorTests.test_extend_cached
func TestExtendRecursive(t *testing.T) {
	engine := templates.New(recursiveSources(t, "fs", "fs2", "fs3")...)
	const want = "fs3/recursive fs2/recursive fs/recursive"
	// The second render of recursive.html takes it from the cache.
	for _, name := range []string{"recursive.html", "recursive.html", "other-recursive.html"} {
		if got := strings.TrimSpace(mustRender(t, engine, name, nil)); got != want {
			t.Errorf("%s: got %q, want %q", name, got, want)
		}
	}
	// No Go counterpart: test_extend_cached also counts the entries of the
	// cached loader, which the engine does not expose.
}

// TestRecursiveMultipleSources checks templates that extend their namesakes
// in a later source, all the way up the chain.
//
// Ported from Django: tests/template_tests/test_extends.py::ExtendsBehaviorTests.test_recursive_multiple_loaders
func TestRecursiveMultipleSources(t *testing.T) {
	locmem := templates.MapSource("locmem", map[string]string{
		"one.html":   `{% extends "one.html" %}{% block content %}{{ block.super }} locmem-one{% endblock %}`,
		"two.html":   `{% extends "two.html" %}{% block content %}{{ block.super }} locmem-two{% endblock %}`,
		"three.html": `{% extends "three.html" %}{% block content %}{{ block.super }} locmem-three{% endblock %}`,
	})
	engine := templates.New(append([]templates.Source{locmem}, recursiveSources(t, "fs")...)...)
	want := "three locmem-three two locmem-two one locmem-one"
	if got := strings.TrimSpace(mustRender(t, engine, "one.html", nil)); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestUniqueHistoryPerSource checks that two sources with the same name are
// two templates: the first extends the second.
//
// Ported from Django: tests/template_tests/test_extends.py::ExtendsBehaviorTests.test_unique_history_per_loader
func TestUniqueHistoryPerSource(t *testing.T) {
	engine := templates.New(
		templates.MapSource("loader1", map[string]string{
			"base.html": `{% extends "base.html" %}{% block content %}{{ block.super }} loader1{% endblock %}`,
		}),
		templates.MapSource("loader2", map[string]string{
			"base.html": "{% block content %}loader2{% endblock %}",
		}),
	)
	if got := strings.TrimSpace(mustRender(t, engine, "base.html", nil)); got != "loader2 loader1" {
		t.Errorf("got %q, want %q", got, "loader2 loader1")
	}
}

// TestBlockOverrideInExtendedIncludedTemplate checks that an included
// template starts its own chain: it extends its namesake even though the
// template that includes it was reached by extending a namesake too.
//
// Ported from Django: tests/template_tests/test_extends.py::ExtendsBehaviorTests.test_block_override_in_extended_included_template
func TestBlockOverrideInExtendedIncludedTemplate(t *testing.T) {
	engine := templates.New(
		templates.MapSource("loader1", map[string]string{
			"base.html":     "{% extends 'base.html' %}{% block base %}{{ block.super }}2{% endblock %}",
			"included.html": "{% extends 'included.html' %}{% block included %}{{ block.super }}B{% endblock %}",
		}),
		templates.MapSource("loader2", map[string]string{
			"base.html":     "{% block base %}1{% endblock %}{% include 'included.html' %}",
			"included.html": "{% block included %}A{% endblock %}",
		}),
	)
	if got := mustRender(t, engine, "base.html", nil); got != "12AB" {
		t.Errorf("got %q, want %q", got, "12AB")
	}
}

// TestExtendSelfError checks a template that extends itself when no later
// source has a template of that name, directly and through another
// template.
//
// Ported from Django: tests/template_tests/test_extends.py::ExtendsBehaviorTests.test_extend_self_error
func TestExtendSelfError(t *testing.T) {
	engine := templates.New(recursiveSources(t, "fs")...)
	_, err := engine.Render("self.html", nil)
	miss := wantMissing(t, err)
	if miss.Name != "self.html" {
		t.Errorf("Name = %q, want self.html", miss.Name)
	}
	if len(miss.Tried) != 1 || !strings.HasSuffix(miss.Tried[0].Origin, "self.html") ||
		miss.Tried[0].Reason != "Skipped to avoid recursion" {
		t.Errorf("Tried = %v, want self.html skipped to avoid recursion", miss.Tried)
	}

	// No Django test exists for a cycle through another template.
	engine = templates.New(templates.MapSource("fs", map[string]string{
		"a.html": `{% extends "b.html" %}`,
		"b.html": `{% extends 'a.html' %}`,
	}))
	_, err = engine.Render("a.html", nil)
	miss = wantMissing(t, err)
	if miss.Name != "a.html" || len(miss.Tried) != 1 || miss.Tried[0] != (templates.TriedSource{Origin: "fs:a.html", Reason: "Skipped to avoid recursion"}) {
		t.Errorf("a.html: error = %v, want a.html skipped to avoid recursion", err)
	}
}

// TestExtendRecursiveRecorded checks that a Recorder lists each template of
// a chain through namesakes.
//
// No Django test exists for the templates the test client records here.
func TestExtendRecursiveRecorded(t *testing.T) {
	engine := templates.New(recursiveSources(t, "fs", "fs2", "fs3")...)
	rec := &templates.Recorder{}
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r = r.WithContext(templates.WithRecorder(r.Context(), rec))
	if _, err := engine.RenderRequest(r, "recursive.html", nil); err != nil {
		t.Fatalf("RenderRequest: %v", err)
	}
	rendered := rec.Rendered()
	names := make([]string, 0, len(rendered))
	for _, rd := range rendered {
		names = append(names, rd.Name)
	}
	if want := []string{"recursive.html", "recursive.html", "recursive.html"}; !slices.Equal(names, want) {
		t.Errorf("recorded %q, want %q", names, want)
	}
}
