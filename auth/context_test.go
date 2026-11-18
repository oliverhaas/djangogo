package auth_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/oliverhaas/djangogo/auth"
	"github.com/oliverhaas/djangogo/templates"
)

// userTemplate prints what a template sees of the user and its permissions.
const userTemplate = `{{ user }}|{{ user.username }}|{{ user.pk }}|` +
	`{% if user.is_authenticated %}in{% else %}out{% endif %}|` +
	`{% if perms.auth %}auth{% endif %}{% if perms.auth.add_group %}+add_group{% endif %}`

// renderUser renders userTemplate for r with the auth context processor.
func renderUser(t *testing.T, r *http.Request) string {
	t.Helper()
	engine, err := templates.NewEngineFS(fstest.MapFS{"user.html": {Data: []byte(userTemplate)}})
	must(t, err)
	engine.AddContextProcessor(auth.ContextProcessor)
	out, err := engine.RenderRequest(r, "user.html", nil)
	must(t, err)
	return out
}

// TestContextProcessorAnonymous checks the template view of a request
// without a logged-in user, with and without Middleware.
func TestContextProcessorAnonymous(t *testing.T) {
	db := newTestDB(t)
	want := "AnonymousUser|||out|"
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	if got := renderUser(t, r); got != want {
		t.Errorf("without Middleware: %q, want %q", got, want)
	}
	auth.Middleware(db)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		if got := renderUser(t, r); got != want {
			t.Errorf("with Middleware: %q, want %q", got, want)
		}
	})).ServeHTTP(httptest.NewRecorder(), r)
}

// TestContextProcessorEscapesUser checks that {{ user }} escapes the
// username once, and that the permissions come from the database of
// Middleware.
func TestContextProcessorEscapesUser(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	u, err := auth.CreateUser(ctx, db, `<b>"o'k"</b>`, "", "pw", false, false, true)
	must(t, err)
	must(t, u.AddPermissions(ctx, db, groupPerm(t, db, "Can add group", "add_group")))
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r = r.WithContext(auth.WithUser(r.Context(), reload(t, db, u.ID)))

	escaped := "&lt;b&gt;&quot;o&#x27;k&quot;&lt;/b&gt;"
	auth.Middleware(db)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got := renderUser(t, r)
		if !strings.HasPrefix(got, escaped+"|") || !strings.HasSuffix(got, "|in|auth+add_group") {
			t.Errorf("rendered %q, want %s|...|in|auth+add_group", got, escaped)
		}
	})).ServeHTTP(httptest.NewRecorder(), r)

	// Without Middleware there is no database to read permissions from.
	if got := renderUser(t, r); !strings.HasSuffix(got, "|in|") {
		t.Errorf("without Middleware: %q, want no permissions", got)
	}
}

// TestContextProcessorSuperuser checks that an active superuser sees every
// permission in the database.
func TestContextProcessorSuperuser(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	groupPerm(t, db, "Can add group", "add_group")
	u, err := auth.CreateSuperuser(ctx, db, "root", "", "pw")
	must(t, err)
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r = r.WithContext(auth.WithUser(r.Context(), u))
	var got string
	auth.Middleware(db)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got = renderUser(t, r)
	})).ServeHTTP(httptest.NewRecorder(), r)
	if !strings.HasPrefix(got, "root|root|") || !strings.HasSuffix(got, "|in|auth+add_group") {
		t.Errorf("rendered %q, want root|root|...|in|auth+add_group", got)
	}
}

// TestTemplatePermsShape checks the map behind perms.
func TestTemplatePermsShape(t *testing.T) {
	p := auth.TemplatePermsOf([]string{"blog.add_post", "blog.change_post", "shop.view_item", "nodot"})
	if len(p) != 5 {
		t.Errorf("perms has %d keys, want 5: %v", len(p), p)
	}
	blog, _ := p["blog"].(map[string]bool)
	if len(blog) != 2 || !blog["add_post"] || !blog["change_post"] {
		t.Errorf(`perms["blog"] = %v`, p["blog"])
	}
	if p["blog.add_post"] != true || p["shop.view_item"] != true {
		t.Errorf("full permission keys missing: %v", p)
	}
}
