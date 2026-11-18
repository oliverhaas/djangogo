package auth_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"testing/fstest"

	"github.com/oliverhaas/djangogo/auth"
	"github.com/oliverhaas/djangogo/contenttypes"
	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/messages"
	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/sessions"
	"github.com/oliverhaas/djangogo/templates"
)

// The templates of tests/auth_tests/templates/context_processors, verbatim.
const (
	// tplAccess is auth_attrs_access.html.
	tplAccess = "{{ user }}\n"
	// tplMessages is auth_attrs_messages.html.
	tplMessages = "{% for m in messages %}{{ m }}{% endfor %}\n"
	// tplNoAccess is auth_attrs_no_access.html.
	tplNoAccess = "\n"
	// tplPermInPerms is auth_attrs_perm_in_perms.html.
	tplPermInPerms = `{% if 'auth' in perms %}Has auth permissions{% endif %}
{% if 'auth.add_permission' in perms %}Has auth.add_permission permissions{% endif %}
{% if 'nonexistent' in perms %}nonexistent perm found{% endif %}
{% if 'auth.nonexistent' in perms %}auth.nonexistent perm found{% endif %}
`
	// tplPerms is auth_attrs_perms.html.
	tplPerms = `{% if perms.auth %}Has auth permissions{% endif %}
{% if perms.auth.add_permission %}Has auth.add_permission permissions{% endif %}
{% if perms.nonexistent %}nonexistent perm found{% endif %}
{% if perms.auth.nonexistent in perms %}auth.nonexistent perm found{% endif %}
`
	// tplTestAccess is auth_attrs_test_access.html.
	tplTestAccess = "{% if session_accessed %}Session accessed{% else %}Session not accessed{% endif %}\n"
	// tplUser is auth_attrs_user.html.
	tplUser = `unicode: {{ user }}
id: {{ user.pk }}
username: {{ user.username }}
url: {% url 'userpage' user %}
`
)

// ctxProcSite returns a client for the views of Django's auth_processor_*
// URLs. With withMessages the messages middleware and context processor are
// on, otherwise only the session and auth middleware are (AUTH_MIDDLEWARE).
func ctxProcSite(t *testing.T, db *orm.DB, withMessages bool) *djangotest.Client {
	t.Helper()
	engine, err := templates.NewEngineFS(fstest.MapFS{
		"auth_attrs_access.html":        {Data: []byte(tplAccess)},
		"auth_attrs_messages.html":      {Data: []byte(tplMessages)},
		"auth_attrs_no_access.html":     {Data: []byte(tplNoAccess)},
		"auth_attrs_perm_in_perms.html": {Data: []byte(tplPermInPerms)},
		"auth_attrs_perms.html":         {Data: []byte(tplPerms)},
		"auth_attrs_test_access.html":   {Data: []byte(tplTestAccess)},
		"auth_attrs_user.html":          {Data: []byte(tplUser)},
	})
	must(t, err)
	if withMessages {
		engine.AddContextProcessor(auth.ContextProcessor, messages.ContextProcessor)
	} else {
		engine.AddContextProcessor(auth.ContextProcessor)
	}
	engine.SetResolver(templates.Resolver(func(name string, args ...any) (string, error) {
		if name != "userpage" {
			return "", fmt.Errorf("no URL named %q", name)
		}
		return "/userpage/" + fmt.Sprint(args[0]) + "/", nil
	}))
	render := func(w http.ResponseWriter, r *http.Request, name string, data map[string]any) {
		out, err := engine.RenderRequest(r, name, data)
		if err != nil {
			t.Errorf("render %s: %v", name, err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_, _ = fmt.Fprint(w, out)
	}
	simple := func(name string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) { render(w, r, name, nil) }
	}
	// accessView renders first, then reports whether the session was read.
	accessView := func(first string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if _, err := engine.RenderRequest(r, first, nil); err != nil {
				t.Errorf("render %s: %v", first, err)
			}
			sess, _ := sessions.FromContext(r.Context())
			render(w, r, "auth_attrs_test_access.html", map[string]any{"session_accessed": sess.Accessed()})
		}
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /auth_processor_no_attr_access/", accessView("auth_attrs_no_access.html"))
	mux.HandleFunc("GET /auth_processor_attr_access/", accessView("auth_attrs_access.html"))
	mux.HandleFunc("GET /auth_processor_user/", simple("auth_attrs_user.html"))
	mux.HandleFunc("GET /auth_processor_perms/", simple("auth_attrs_perms.html"))
	mux.HandleFunc("GET /auth_processor_perm_in_perms/", simple("auth_attrs_perm_in_perms.html"))
	mux.HandleFunc("GET /auth_processor_messages/", func(w http.ResponseWriter, r *http.Request) {
		_ = messages.Info(r, "Message 1")
		render(w, r, "auth_attrs_messages.html", nil)
	})

	store := sessions.NewDBStore(db)
	var h http.Handler = mux
	if withMessages {
		h = messages.Middleware(h)
	}
	h = sessions.Middleware(store, sessionCookieName)(auth.Middleware(db)(h))
	return djangotest.NewClient(t, h,
		djangotest.WithSessions(store, sessionCookieName), djangotest.WithDB(db))
}

// normalUserWithAddPermission creates the user "normal" holding
// auth.add_permission, as the setup of the perms tests.
func normalUserWithAddPermission(t *testing.T, db *orm.DB) *auth.User {
	t.Helper()
	ctx := context.Background()
	_, err := auth.SyncPermissions(ctx, db)
	must(t, err)
	u := createUser(t, db, "normal", "secret")
	ct, err := contenttypes.GetForInstance(ctx, db, &auth.Permission{})
	must(t, err)
	perm, err := orm.Query[auth.Permission](db).
		Filter("content_type_id", ct.ID).Filter("codename", "add_permission").Get(ctx)
	must(t, err)
	must(t, u.AddPermissions(ctx, db, &perm))
	return u
}

// Ported from Django: tests/auth_tests/test_context_processors.py::PermWrapperTests.test_repr
func TestPermWrapperRepr(t *testing.T) {
	t.Skip("tests the Python repr of PermWrapper")
}

// permsIn renders an in-operator check of key against perms.
func permsIn(t *testing.T, engine *templates.Engine, perms auth.TemplatePerms, key string) bool {
	t.Helper()
	out, err := engine.RenderString("{% if '"+key+"' in perms %}yes{% endif %}", map[string]any{"perms": perms})
	must(t, err)
	return out == "yes"
}

// 'something' in PermWrapper works as expected for modules and full
// permissions.
//
// Ported from Django: tests/auth_tests/test_context_processors.py::PermWrapperTests.test_permwrapper_in
func TestPermWrapperIn(t *testing.T) {
	engine, err := templates.NewEngineFS(fstest.MapFS{})
	must(t, err)
	perms := auth.TemplatePermsOf([]string{"mockapp.someperm"})
	for _, tc := range []struct {
		key  string
		want bool
	}{
		{"mockapp", true},
		{"nonexistent", false},
		{"mockapp.someperm", true},
		{"mockapp.nonexistent", false},
	} {
		if got := permsIn(t, engine, perms, tc.key); got != tc.want {
			t.Errorf("%q in perms = %v, want %v", tc.key, got, tc.want)
		}
	}
}

// No endless loops if accessed with 'in' (refs #18979).
//
// Ported from Django: tests/auth_tests/test_context_processors.py::PermWrapperTests.test_permlookupdict_in
func TestPermLookupDictIn(t *testing.T) {
	t.Skip("tests the Python __eq__ protocol of an arbitrary object; the perms view is a plain map")
}

// Ported from Django: tests/auth_tests/test_context_processors.py::PermWrapperTests.test_iter
func TestPermWrapperIter(t *testing.T) {
	t.Skip("pongo2 iterates a map, so there is no iteration error")
}

// The session is not accessed simply by including the auth context
// processor.
//
// Ported from Django: tests/auth_tests/test_context_processors.py::AuthContextProcessorTests.test_session_not_accessed
func TestContextProcessorSessionNotAccessed(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		c := ctxProcSite(t, db, false)
		djangotest.AssertContains(t, c.Get("/auth_processor_no_attr_access/"), "Session not accessed")
	})
}

// The session is accessed if the auth context processor is used and relevant
// attributes accessed.
//
// Ported from Django: tests/auth_tests/test_context_processors.py::AuthContextProcessorTests.test_session_is_accessed
func TestContextProcessorSessionIsAccessed(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		c := ctxProcSite(t, db, false)
		djangotest.AssertContains(t, c.Get("/auth_processor_attr_access/"), "Session accessed")
	})
}

// Ported from Django: tests/auth_tests/test_context_processors.py::AuthContextProcessorTests.test_perms_attrs
func TestContextProcessorPermsAttrs(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		u := normalUserWithAddPermission(t, db)
		c := ctxProcSite(t, db, true)
		c.ForceLogin(u)
		resp := c.Get("/auth_processor_perms/")
		djangotest.AssertContains(t, resp, "Has auth permissions")
		djangotest.AssertContains(t, resp, "Has auth.add_permission permissions")
		djangotest.AssertNotContains(t, resp, "nonexistent")
	})
}

// Ported from Django: tests/auth_tests/test_context_processors.py::AuthContextProcessorTests.test_perm_in_perms_attrs
func TestContextProcessorPermInPermsAttrs(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		normalUserWithAddPermission(t, db)
		c := ctxProcSite(t, db, true)
		if !c.Login("normal", "secret") {
			t.Fatal("login failed")
		}
		resp := c.Get("/auth_processor_perm_in_perms/")
		djangotest.AssertContains(t, resp, "Has auth permissions")
		djangotest.AssertContains(t, resp, "Has auth.add_permission permissions")
		djangotest.AssertNotContains(t, resp, "nonexistent")
	})
}

// Ported from Django: tests/auth_tests/test_context_processors.py::AuthContextProcessorTests.test_message_attrs
func TestContextProcessorMessageAttrs(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		super, err := auth.CreateSuperuser(context.Background(), db, "super", "super@example.com", "secret")
		must(t, err)
		c := ctxProcSite(t, db, true)
		c.ForceLogin(super)
		djangotest.AssertContains(t, c.Get("/auth_processor_messages/"), "Message 1")
	})
}

// The lazy objects returned behave just like the wrapped objects.
//
// Ported from Django: tests/auth_tests/test_context_processors.py::AuthContextProcessorTests.test_user_attrs
func TestContextProcessorUserAttrs(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		super, err := auth.CreateSuperuser(context.Background(), db, "super", "super@example.com", "secret")
		must(t, err)
		c := ctxProcSite(t, db, true)
		if !c.Login("super", "secret") {
			t.Fatal("login failed")
		}
		resp := c.Get("/auth_processor_user/")
		djangotest.AssertContains(t, resp, "unicode: super")
		djangotest.AssertContains(t, resp, fmt.Sprintf("id: %d", super.ID))
		djangotest.AssertContains(t, resp, "username: super")
		// Bug #12037 is tested by the {% url %} in the template.
		djangotest.AssertContains(t, resp, "url: /userpage/super/")

		// Django builds a Q() with the user here; Go has no counterpart.

		// User equality: compare the context user with the real user.
		v, ok := resp.Context("user")
		if !ok {
			t.Fatal(`"user" is not in the context`)
		}
		fn, ok := v.(func() (auth.TemplateUser, error))
		if !ok {
			t.Fatalf(`"user" is a %T, want func() (auth.TemplateUser, error)`, v)
		}
		tu, err := fn()
		must(t, err)
		djangotest.AssertEqual(t, tu["pk"], any(super.ID))
		djangotest.AssertEqual(t, tu["username"], any("super"))
	})
}
