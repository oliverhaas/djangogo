package views_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/templates"
	"github.com/oliverhaas/djangogo/urls"
	"github.com/oliverhaas/djangogo/views"
)

// shortcutsClient serves the views of Django's tests/shortcuts/views.py.
func shortcutsClient(t *testing.T) *djangotest.Client {
	t.Helper()
	eng := memEngine(t, map[string]string{
		"shortcuts/render_test.html": "{{ foo }}.{{ bar }}.{{ baz }}.{{ request.path }}\n",
	})
	eng.AddContextProcessor(requestProcessor)
	ctx := map[string]any{"foo": "FOO", "bar": "BAR"}
	mux := http.NewServeMux()
	mux.Handle("/render/{$}", views.Handler(eng, func(w http.ResponseWriter, r *http.Request) error {
		return views.Render(w, r, eng, "shortcuts/render_test.html", ctx)
	}))
	mux.Handle("/render/multiple_templates/{$}", views.Handler(eng, func(w http.ResponseWriter, r *http.Request) error {
		return views.RenderFirst(w, r, eng,
			[]string{"shortcuts/no_such_template.html", "shortcuts/render_test.html"}, ctx)
	}))
	mux.Handle("/render/content_type/{$}", views.Handler(eng, func(w http.ResponseWriter, r *http.Request) error {
		return views.Render(w, r, eng, "shortcuts/render_test.html", ctx,
			views.WithContentType("application/x-rendertest"))
	}))
	mux.Handle("/render/status/{$}", views.Handler(eng, func(w http.ResponseWriter, r *http.Request) error {
		return views.Render(w, r, eng, "shortcuts/render_test.html", ctx, views.WithStatus(http.StatusForbidden))
	}))
	return djangotest.NewClient(t, mux)
}

// Ported from Django: tests/shortcuts/tests.py::RenderTests.test_render
//
// The current_app assertion is dropped: requests have no such attribute here.
func TestRenderShortcut(t *testing.T) {
	resp := shortcutsClient(t).Get("/render/")
	djangotest.AssertStatus(t, resp, http.StatusOK)
	djangotest.AssertEqual(t, resp.Content(), "FOO.BAR../render/\n")
	djangotest.AssertEqual(t, resp.Header.Get("Content-Type"), "text/html; charset=utf-8")
}

// Ported from Django: tests/shortcuts/tests.py::RenderTests.test_render_with_multiple_templates
func TestRenderWithMultipleTemplates(t *testing.T) {
	resp := shortcutsClient(t).Get("/render/multiple_templates/")
	djangotest.AssertStatus(t, resp, http.StatusOK)
	djangotest.AssertEqual(t, resp.Content(), "FOO.BAR../render/multiple_templates/\n")
	djangotest.AssertTemplateUsed(t, resp, "shortcuts/render_test.html")
}

// Ported from Django: tests/shortcuts/tests.py::RenderTests.test_render_with_content_type
func TestRenderWithContentType(t *testing.T) {
	resp := shortcutsClient(t).Get("/render/content_type/")
	djangotest.AssertStatus(t, resp, http.StatusOK)
	djangotest.AssertEqual(t, resp.Content(), "FOO.BAR../render/content_type/\n")
	djangotest.AssertEqual(t, resp.Header.Get("Content-Type"), "application/x-rendertest")
}

// Ported from Django: tests/shortcuts/tests.py::RenderTests.test_render_with_status
func TestRenderWithStatus(t *testing.T) {
	resp := shortcutsClient(t).Get("/render/status/")
	djangotest.AssertStatus(t, resp, http.StatusForbidden)
	djangotest.AssertEqual(t, resp.Content(), "FOO.BAR../render/status/\n")
}

// Ported from Django: tests/shortcuts/tests.py::RedirectTests.test_redirect_response_status_code
func TestRedirectResponseStatusCode(t *testing.T) {
	tests := []struct {
		permanent, preserveRequest bool
		want                       int
	}{
		{true, false, 301},
		{false, false, 302},
		{false, true, 307},
		{true, true, 308},
	}
	for _, tc := range tests {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rd := views.Redirector{Permanent: tc.permanent, PreserveRequest: tc.preserveRequest}
		djangotest.AssertEqual(t, rd.To(rec, req, "/path/is/irrelevant/"), nil)
		djangotest.AssertEqual(t, rec.Code, tc.want)
		djangotest.AssertEqual(t, rec.Header().Get("Location"), "/path/is/irrelevant/")
	}
}

// Ported from Django: tests/shortcuts/tests.py::RedirectTests.test_redirect_max_length_default_raises
func TestRedirectMaxLengthDefaultRaises(t *testing.T) {
	longURL := "https://example.com/" + strings.Repeat("x", views.MaxURLRedirectLength)
	rec := httptest.NewRecorder()
	err := views.RedirectTo(rec, httptest.NewRequest(http.MethodGet, "/", nil), longURL)
	djangotest.AssertErrorIs(t, err, views.ErrDisallowedRedirect)
	djangotest.AssertErrorIs(t, err, views.ErrSuspiciousOperation)
	djangotest.AssertErrorIs(t, err, views.ErrBadRequest)
	if err != nil {
		djangotest.AssertEqual(t, err.Error(), "Unsafe redirect exceeding 16384 characters")
	}
	djangotest.AssertEqual(t, rec.Header().Get("Location"), "")
}

// Ported from Django: tests/shortcuts/tests.py::RedirectTests.test_redirect_max_length_override_passes
//
// Django disables the check with max_length=None, Go with a negative
// MaxLength.
func TestRedirectMaxLengthOverridePasses(t *testing.T) {
	longURL := "https://example.com/" + strings.Repeat("x", views.MaxURLRedirectLength)
	rec := httptest.NewRecorder()
	err := views.Redirector{MaxLength: -1}.To(rec, httptest.NewRequest(http.MethodGet, "/", nil), longURL)
	djangotest.AssertEqual(t, err, nil)
	djangotest.AssertEqual(t, rec.Header().Get("Location"), longURL)
}

// Ported from Django: tests/shortcuts/tests.py::RedirectTests.test_redirect_custom_strict_limit_raises
func TestRedirectCustomStrictLimitRaises(t *testing.T) {
	rec := httptest.NewRecorder()
	err := views.Redirector{MaxLength: 5}.To(rec, httptest.NewRequest(http.MethodGet, "/", nil), "https://example.com/")
	djangotest.AssertErrorIs(t, err, views.ErrDisallowedRedirect)
	if err != nil {
		djangotest.AssertEqual(t, err.Error(), "Unsafe redirect exceeding 5 characters")
	}
}

// shortcutsArtist is a model with a URL of its own.
type shortcutsArtist struct{ id int }

func (a shortcutsArtist) GetAbsoluteURL() string { return "/artists/" + string(rune('0'+a.id)) + "/" }

// TestResolveURL covers Django's resolve_url rules. Django tests them in
// tests/resolve_url, which needs its URL configuration; here an explicit
// resolver takes that role.
func TestResolveURL(t *testing.T) {
	router := urls.NewRouter(
		urls.PathFunc("GET /people/{pk}/", func(http.ResponseWriter, *http.Request) {}, "person-detail"),
		urls.PathFunc("GET /home/", func(http.ResponseWriter, *http.Request) {}, "home"),
	)
	rd := views.Redirector{Resolver: router.Reverse}
	tests := []struct {
		to   any
		args []any
		want string
	}{
		{"/something/", nil, "/something/"},
		{"../", nil, "../"},
		{"../relative/", nil, "../relative/"},
		{"./", nil, "./"},
		{"./relative/", nil, "./relative/"},
		{"https://example.com/", nil, "https://example.com/"},
		{shortcutsArtist{id: 7}, nil, "/artists/7/"},
		{"home", nil, "/home/"},
		{"person-detail", []any{3}, "/people/3/"},
	}
	for _, tc := range tests {
		got, err := rd.Resolve(tc.to, tc.args...)
		djangotest.AssertEqual(t, err, nil)
		djangotest.AssertEqual(t, got, tc.want)
	}
	// A name that cannot be reversed is an error, not a URL.
	if _, err := rd.Resolve("does-not-exist"); err == nil {
		t.Error("Resolve of an unknown name succeeded, want an error")
	}
	if _, err := rd.Resolve(42); err == nil {
		t.Error("Resolve(42) succeeded, want an error")
	}

	// Without an explicit resolver the process-wide one is used.
	previous := templates.URLResolverFunc()
	templates.SetURLResolver(router.Reverse)
	t.Cleanup(func() { templates.SetURLResolver(previous) })
	got, err := views.ResolveURL("person-detail", 9)
	djangotest.AssertEqual(t, err, nil)
	djangotest.AssertEqual(t, got, "/people/9/")
}

// TestRedirectToChecksTarget asserts the redirect safety checks Django's
// HttpResponseRedirectBase applies: a disallowed scheme is refused before
// anything is written, and the Location is percent-encoded.
func TestRedirectToChecksTarget(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	for _, target := range []string{"javascript:alert(1)//", "data:text/html,x/", " JavaScript:alert(1)/", "file:///etc/passwd"} {
		rec := httptest.NewRecorder()
		err := views.RedirectTo(rec, req, target)
		djangotest.AssertErrorIs(t, err, views.ErrDisallowedRedirect)
		djangotest.AssertEqual(t, rec.Header().Get("Location"), "")
	}
	rec := httptest.NewRecorder()
	err := views.RedirectTo(rec, req, "ssh://example.com/")
	if err == nil || err.Error() != "Unsafe redirect to URL with protocol 'ssh'" {
		t.Errorf("error = %v, want Django's message for a disallowed protocol", err)
	}

	rec = httptest.NewRecorder()
	djangotest.AssertEqual(t, views.RedirectPermanent(rec, req, "/café/a b/\r\nSet-Cookie: x=1"), nil)
	djangotest.AssertEqual(t, rec.Code, http.StatusMovedPermanently)
	djangotest.AssertEqual(t, rec.Header().Get("Location"), "/caf%C3%A9/a%20b/%0D%0ASet-Cookie:%20x=1")
}

// GetArticle and GetAuthor are the models of Django's
// tests/get_object_or_404/models.py, without the many-to-many relation.
type GetArticle struct {
	ID    int64
	Title string `orm:"max_length=50"`
}

// GetAuthor is the Author model of the get_object_or_404 tests.
type GetAuthor struct {
	ID   int64
	Name string `orm:"max_length=50"`
}

// Ported from Django: tests/get_object_or_404/tests.py::GetObjectOr404Tests.test_get_object_or_404
//
// The assertions through the related manager a1.article_set, the custom
// manager and Q objects are left out: the ORM has no many-to-many relations,
// managers or Q objects. The Go test also asserts Django's 404 message.
func TestGetObjectOr404(t *testing.T) {
	djangotest.EachDB(t, []any{&GetArticle{}, &GetAuthor{}}, func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		articles := orm.Query[GetArticle](db)
		for _, name := range []string{"Brave Sir Robin", "Patsy"} {
			if err := orm.Query[GetAuthor](db).Create(ctx, &GetAuthor{Name: name}); err != nil {
				t.Fatal(err)
			}
		}
		// No Articles yet, so we should get an Http404 error.
		_, err := views.GetObjectOr404(ctx, articles, "title", "Foo")
		djangotest.AssertErrorIs(t, err, views.ErrNotFound)
		if err != nil {
			djangotest.AssertEqual(t, err.Error(), "No GetArticle matches the given query.")
		}
		djangotest.AssertEqual(t, views.StatusCode(err), http.StatusNotFound)

		article := GetArticle{Title: "Run away!"}
		if err := articles.Create(ctx, &article); err != nil {
			t.Fatal(err)
		}
		// get_object_or_404 can be passed a Model to query.
		got, err := views.GetObjectOr404(ctx, articles, "title__contains", "Run")
		djangotest.AssertEqual(t, err, nil)
		djangotest.AssertEqual(t, got, article)
		// No articles containing "Camelot". This should raise an Http404 error.
		_, err = views.GetObjectOr404(ctx, articles, "title__contains", "Camelot")
		djangotest.AssertErrorIs(t, err, views.ErrNotFound)
		// QuerySets can be used too.
		got, err = views.GetObjectOr404(ctx, articles.Filter("title__startswith", "Run"), "title__contains", "Run")
		djangotest.AssertEqual(t, err, nil)
		djangotest.AssertEqual(t, got, article)
		// Just as when using a get() lookup, you will get an error if more than
		// one object is returned.
		_, err = views.GetObjectOr404(ctx, orm.Query[GetAuthor](db))
		djangotest.AssertErrorIs(t, err, orm.ErrMultipleObjectsReturned)
		// Using an empty QuerySet raises an Http404 error.
		_, err = views.GetObjectOr404(ctx, articles.None(), "title__contains", "Run")
		djangotest.AssertErrorIs(t, err, views.ErrNotFound)

		// get_list_or_404 can be used to get lists of objects
		list, err := views.GetListOr404(ctx, articles, "title__icontains", "Run")
		djangotest.AssertEqual(t, err, nil)
		djangotest.AssertEqual(t, list, []GetArticle{article})
		// Http404 is returned if the list is empty.
		_, err = views.GetListOr404(ctx, articles, "title__icontains", "Shrubbery")
		djangotest.AssertErrorIs(t, err, views.ErrNotFound)
		if err != nil {
			djangotest.AssertEqual(t, err.Error(), "No GetArticle matches the given query.")
		}
	})
}

// Ported from Django: tests/get_object_or_404/tests.py::GetObjectOr404Tests.test_get_object_or_404_queryset_attribute_error
// Ported from Django: tests/get_object_or_404/tests.py::GetObjectOr404Tests.test_get_list_or_404_queryset_attribute_error
//
// Django checks that an AttributeError raised by the queryset is not hidden.
// The Go counterpart is a queryset error (here an unknown column), which must
// come back unchanged and not as a 404.
func TestGetOr404QuerysetError(t *testing.T) {
	db := djangotest.NewDB(t, &GetArticle{})
	ctx := context.Background()
	_, err := views.GetObjectOr404(ctx, orm.Query[GetArticle](db), "no_such_column", 42)
	if err == nil || errors.Is(err, views.ErrNotFound) {
		t.Errorf("GetObjectOr404 error = %v, want the queryset error", err)
	}
	djangotest.AssertEqual(t, views.StatusCode(err), http.StatusInternalServerError)
	_, err = views.GetListOr404(ctx, orm.Query[GetArticle](db), "no_such_column__icontains", "Run")
	if err == nil || errors.Is(err, views.ErrNotFound) {
		t.Errorf("GetListOr404 error = %v, want the queryset error", err)
	}
}

// TestJSONResponse covers the JsonResponse counterpart.
func TestJSONResponse(t *testing.T) {
	rec := httptest.NewRecorder()
	djangotest.AssertEqual(t, views.JSONResponse(rec, map[string]any{"foo": "bar"}), nil)
	djangotest.AssertEqual(t, rec.Code, http.StatusOK)
	djangotest.AssertEqual(t, rec.Header().Get("Content-Type"), "application/json")
	djangotest.AssertJSONEqual(t, rec.Body.String(), map[string]any{"foo": "bar"})

	rec = httptest.NewRecorder()
	djangotest.AssertEqual(t, views.JSONResponse(rec, []int{1, 2}, views.WithStatus(http.StatusCreated)), nil)
	djangotest.AssertEqual(t, rec.Code, http.StatusCreated)

	rec = httptest.NewRecorder()
	if err := views.JSONResponse(rec, make(chan int)); err == nil {
		t.Error("JSONResponse of a channel succeeded, want an error")
	}
	djangotest.AssertEqual(t, rec.Body.Len(), 0)
}

// shortcutsBox is a context value with a template shape of its own.
type shortcutsBox struct{ label string }

// TemplateValue returns the map the template reads.
func (b *shortcutsBox) TemplateValue() any { return map[string]any{"label": b.label} }

// TestRenderConvertsTemplateValuers asserts that Render and RenderToString
// replace top-level TemplateValuer values, leave the caller's map alone and
// keep a nil pointer empty.
func TestRenderConvertsTemplateValuers(t *testing.T) {
	eng := memEngine(t, map[string]string{
		"box.html": "{{ box.label }}|{% if missing %}set{% else %}unset{% endif %}",
	})
	var missing *shortcutsBox
	box := &shortcutsBox{label: "<b>crate</b>"}
	ctx := map[string]any{"box": box, "missing": missing}
	handler := views.Handler(eng, func(w http.ResponseWriter, r *http.Request) error {
		return views.Render(w, r, eng, "box.html", ctx)
	})
	resp := djangotest.NewClient(t, handler).Get("/")
	djangotest.AssertEqual(t, resp.Content(), "&lt;b&gt;crate&lt;/b&gt;|unset")
	if ctx["box"] != any(box) {
		t.Error("Render replaced the value in the caller's context map")
	}

	out, err := views.RenderToString(httptest.NewRequest(http.MethodGet, "/", nil), eng, "box.html", ctx)
	djangotest.AssertEqual(t, err, nil)
	djangotest.AssertEqual(t, out, "&lt;b&gt;crate&lt;/b&gt;|unset")
}
