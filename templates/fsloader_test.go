package templates_test

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/oliverhaas/djangogo/templates"
)

// TestFSEngineResolvesNamesFromRoot checks that a template in a subdirectory
// extends and includes others by their full name, as in Django, and not by a
// path relative to its own directory.
func TestFSEngineResolvesNamesFromRoot(t *testing.T) {
	fsys := fstest.MapFS{
		"blog/base.html":      {Data: []byte("<main>{% block content %}{% endblock %}</main>")},
		"blog/item.html":      {Data: []byte("item")},
		"blog/post_list.html": {Data: []byte(`{% extends "blog/base.html" %}{% block content %}{% include "blog/item.html" %}{% endblock %}`)},
	}
	engine, err := templates.NewEngineFS(fsys)
	if err != nil {
		t.Fatalf("NewEngineFS: %v", err)
	}
	out, err := engine.Render("blog/post_list.html", nil)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if got, want := strings.TrimSpace(out), "<main>item</main>"; got != want {
		t.Errorf("Render = %q, want %q", got, want)
	}
}
