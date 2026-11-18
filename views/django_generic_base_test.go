package views_test

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/views"
)

// writeSimple is the get method of SimpleView in Django's test_base.py.
func writeSimple(w http.ResponseWriter, _ *http.Request) error {
	_, err := w.Write([]byte("This is a simple view"))
	return err
}

// The views of Django's test_base.py: SimpleView implements GET,
// SimplePostView GET and POST with one handler, PostOnlyView POST.
var (
	simpleView     = views.View{Get: writeSimple}
	simplePostView = views.View{Get: writeSimple, Post: writeSimple}
	postOnlyView   = views.View{Post: func(w http.ResponseWriter, _ *http.Request) error {
		_, err := w.Write([]byte("This view only accepts POST"))
		return err
	}}
)

// assertSimple asserts the answer of writeSimple, Django's _assert_simple.
func assertSimple(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	djangotest.AssertEqual(t, rec.Code, http.StatusOK)
	djangotest.AssertEqual(t, rec.Body.String(), "This is a simple view")
}

// Ported from Django: tests/generic_views/test_base.py::ViewTest.test_pathological_http_method
func TestViewPathologicalHTTPMethod(t *testing.T) {
	djangotest.AssertEqual(t, serveMethod(simpleView, "DISPATCH").Code, http.StatusMethodNotAllowed)
}

// Ported from Django: tests/generic_views/test_base.py::ViewTest.test_get_only
func TestViewGetOnly(t *testing.T) {
	assertSimple(t, serveMethod(simpleView, "GET"))
	djangotest.AssertEqual(t, serveMethod(simpleView, "POST").Code, http.StatusMethodNotAllowed)
	djangotest.AssertEqual(t, serveMethod(simpleView, "FAKE").Code, http.StatusMethodNotAllowed)
}

// Ported from Django: tests/generic_views/test_base.py::ViewTest.test_get_and_head
func TestViewGetAndHead(t *testing.T) {
	assertSimple(t, serveMethod(simpleView, "GET"))
	djangotest.AssertEqual(t, serveMethod(simpleView, "HEAD").Code, http.StatusOK)
}

// Ported from Django: tests/generic_views/test_base.py::ViewTest.test_head_no_get
// Ported from Django: tests/generic_views/test_base.py::ViewTest.test_direct_instantiation
//
// A Go view is a value that serves requests, so there is no as_view() step
// that test_direct_instantiation could go around.
func TestViewHeadNoGet(t *testing.T) {
	djangotest.AssertEqual(t, serveMethod(postOnlyView, "HEAD").Code, http.StatusMethodNotAllowed)
}

// Ported from Django: tests/generic_views/test_base.py::ViewTest.test_get_and_post
func TestViewGetAndPost(t *testing.T) {
	assertSimple(t, serveMethod(simplePostView, "GET"))
	assertSimple(t, serveMethod(simplePostView, "POST"))
	djangotest.AssertEqual(t, serveMethod(simplePostView, "FAKE").Code, http.StatusMethodNotAllowed)
}

// Ported from Django: tests/generic_views/test_base.py::ViewTest.test_options
func TestViewOptions(t *testing.T) {
	rec := serveMethod(simpleView, "OPTIONS")
	djangotest.AssertEqual(t, rec.Code, http.StatusOK)
	djangotest.AssertNotEqual(t, rec.Header().Get("Allow"), "")
}

// Ported from Django: tests/generic_views/test_base.py::ViewTest.test_options_for_get_view
// Ported from Django: tests/generic_views/test_base.py::ViewTest.test_options_for_get_and_post_view
// Ported from Django: tests/generic_views/test_base.py::ViewTest.test_options_for_post_view
func TestViewOptionsAllow(t *testing.T) {
	for _, tc := range []struct {
		name string
		view views.View
		want []string
	}{
		{"get", simpleView, []string{"GET", "HEAD", "OPTIONS"}},
		{"get and post", simplePostView, []string{"GET", "HEAD", "OPTIONS", "POST"}},
		{"post", postOnlyView, []string{"OPTIONS", "POST"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			allowed := strings.Split(serveMethod(tc.view, "OPTIONS").Header().Get("Allow"), ", ")
			slices.Sort(allowed)
			djangotest.AssertSequenceEqual(t, allowed, tc.want)
		})
	}
}

// loggedPaths are the request paths of Django's response logging tests with
// the form they have in the log message: control characters are escaped.
var loggedPaths = []struct{ name, path, escaped string }{
	{"plain", "/foo/", "/foo/"},
	{"escape sequence", "/\x1b[1;31mNOW IN RED!!!1B[0m/", `/\x1b[1;31mNOW IN RED!!!1B[0m/`},
}

// Ported from Django: tests/generic_views/test_base.py::ViewTest.test_method_not_allowed_response_logged
func TestViewMethodNotAllowedResponseLogged(t *testing.T) {
	for _, tc := range loggedPaths {
		t.Run(tc.name, func(t *testing.T) {
			logs := captureLogs(t)
			req := httptest.NewRequest("BOGUS", "/", nil)
			req.URL.Path = tc.path
			rec := httptest.NewRecorder()
			simpleView.ServeHTTP(rec, req)
			djangotest.AssertEqual(t, rec.Code, http.StatusMethodNotAllowed)
			djangotest.AssertSequenceEqual(t, logs.messages(slog.LevelWarn),
				[]string{"Method Not Allowed (BOGUS): " + tc.escaped})
			djangotest.AssertSequenceEqual(t, logs.statuses(slog.LevelWarn), []int64{http.StatusMethodNotAllowed})
		})
	}
}

// aboutNames is get_template_names of AboutTemplateView in Django's
// test_base.py.
func aboutNames(*http.Request) ([]string, error) {
	return []string{"generic_views/about.html"}, nil
}

// Ported from Django: tests/generic_views/test_base.py::TemplateViewTest.test_get
func TestTemplateViewGet(t *testing.T) {
	c := mount(t, "/about/{$}", views.TemplateView{Engine: genericEngine(t), GetTemplateNames: aboutNames})
	djangotest.AssertContains(t, c.Get("/about/"), "<h1>About</h1>")
}

// Ported from Django: tests/generic_views/test_base.py::TemplateViewTest.test_head
func TestTemplateViewHead(t *testing.T) {
	c := mount(t, "/about/{$}", views.TemplateView{Engine: genericEngine(t), GetTemplateNames: aboutNames})
	djangotest.AssertStatus(t, c.Head("/about/"), http.StatusOK)
}

// Ported from Django: tests/generic_views/test_base.py::TemplateViewTest.test_get_template_attribute
// Ported from Django: tests/generic_views/test_base.py::TemplateViewTest.test_get_generic_template
//
// A class attribute and an as_view() argument are the same struct field in Go.
func TestTemplateViewTemplateName(t *testing.T) {
	c := mount(t, "/about/{$}", views.TemplateView{
		Engine: genericEngine(t), TemplateName: "generic_views/about.html",
	})
	djangotest.AssertContains(t, c.Get("/about/"), "<h1>About</h1>")
}

// Ported from Django: tests/generic_views/test_base.py::TemplateViewTest.test_template_name_required
func TestTemplateViewTemplateNameRequired(t *testing.T) {
	logs := captureLogs(t)
	c := mount(t, "/template/no_template/{$}", views.TemplateView{Engine: genericEngine(t)})
	assertImproperlyConfigured(t, logs, c.Get("/template/no_template/"),
		"TemplateView requires either a definition of TemplateName or an implementation of GetTemplateNames")
}

// Ported from Django: tests/generic_views/test_base.py::TemplateViewTest.test_template_engine
//
// Django selects one of the configured engines by name. A Go view renders with
// the engine in its Engine field.
func TestTemplateViewEngine(t *testing.T) {
	for _, body := range []string{"DTL\n", "Jinja2\n"} {
		eng := memEngine(t, map[string]string{"generic_views/using.html": body})
		c := mount(t, "/using/{$}", views.TemplateView{Engine: eng, TemplateName: "generic_views/using.html"})
		djangotest.AssertEqual(t, c.Get("/using/").Content(), body)
	}
}

// assertTemplateView asserts that the context entry "view" is the view.
func assertTemplateView(t *testing.T, resp *djangotest.Response) {
	t.Helper()
	if view, ok := contextValue(t, resp, "view").(views.TemplateView); !ok {
		t.Errorf("context[\"view\"] is %T, want views.TemplateView", view)
	}
}

// Ported from Django: tests/generic_views/test_base.py::TemplateViewTest.test_template_params
func TestTemplateViewParams(t *testing.T) {
	c := mount(t, "/template/simple/{foo}/{$}", views.TemplateView{
		Engine: genericEngine(t), TemplateName: "generic_views/about.html",
	})
	resp := c.Get("/template/simple/bar/")
	djangotest.AssertStatus(t, resp, http.StatusOK)
	djangotest.AssertEqual(t, contextValue(t, resp, "foo"), any("bar"))
	assertTemplateView(t, resp)
}

// Ported from Django: tests/generic_views/test_base.py::TemplateViewTest.test_extra_template_params
func TestTemplateViewExtraParams(t *testing.T) {
	c := mount(t, "/template/custom/{foo}/{$}", views.TemplateView{
		Engine:       genericEngine(t),
		TemplateName: "generic_views/about.html",
		GetContextData: func(_ *http.Request, ctx map[string]any) (map[string]any, error) {
			ctx["key"] = "value"
			return ctx, nil
		},
	})
	resp := c.Get("/template/custom/bar/")
	djangotest.AssertStatus(t, resp, http.StatusOK)
	djangotest.AssertEqual(t, contextValue(t, resp, "foo"), any("bar"))
	djangotest.AssertEqual(t, contextValue(t, resp, "key"), any("value"))
	assertTemplateView(t, resp)
}

// Ported from Django: tests/generic_views/test_base.py::TemplateViewTest.test_content_type
func TestTemplateViewContentType(t *testing.T) {
	c := mount(t, "/template/content_type/{$}", views.TemplateView{
		Engine: genericEngine(t), TemplateName: "generic_views/robots.txt", ContentType: "text/plain",
	})
	djangotest.AssertEqual(t, c.Get("/template/content_type/").Header.Get("Content-Type"), "text/plain")
}

// Ported from Django: tests/generic_views/test_base.py::TemplateViewTest.test_extra_context
func TestTemplateViewExtraContext(t *testing.T) {
	c := mount(t, "/template/extra_context/{$}", views.TemplateView{
		Engine:       genericEngine(t),
		TemplateName: "generic_views/about.html",
		ExtraContext: map[string]any{"title": "Title"},
	})
	djangotest.AssertEqual(t, contextValue(t, c.Get("/template/extra_context/"), "title"), any("Title"))
}

// dummyBook is the object of CustomContextView and CustomSingleObjectView in
// Django's tests/generic_views/views.py.
func dummyBook(*http.Request) (Book, error) { return Book{Name: "dummy"}, nil }

// contextOf serves one request to view and returns the response.
func contextOf(t *testing.T, view http.Handler) *djangotest.Response {
	t.Helper()
	resp := mount(t, "/{$}", view).Get("/")
	djangotest.AssertStatus(t, resp, http.StatusOK)
	return resp
}

// Ported from Django: tests/generic_views/test_base.py::GetContextDataTest.test_get_context_data_super
//
// Django passes keyword arguments to get_context_data. The Go view takes them
// from ExtraContext, which overrides the entries the view builds, and runs the
// GetContextData hook last.
func TestGetContextDataSuper(t *testing.T) {
	view := views.DetailView[Book]{
		Engine:            genericEngine(t),
		TemplateName:      "generic_views/detail.html",
		GetObject:         dummyBook,
		ContextObjectName: "test_name",
		ExtraContext:      map[string]any{"kwarg_test": "kwarg_value"},
		GetContextData: func(_ *http.Request, ctx map[string]any) (map[string]any, error) {
			ctx["custom_key"] = "custom_value"
			return ctx, nil
		},
	}
	resp := contextOf(t, view)
	djangotest.AssertEqual(t, contextValue(t, resp, "test_name"), any(Book{Name: "dummy"}))
	djangotest.AssertEqual(t, contextValue(t, resp, "kwarg_test"), any("kwarg_value"))
	djangotest.AssertEqual(t, contextValue(t, resp, "custom_key"), any("custom_value"))

	view.ExtraContext = map[string]any{"test_name": "test_value"}
	djangotest.AssertEqual(t, contextValue(t, contextOf(t, view), "test_name"), any("test_value"))
}

// Ported from Django: tests/generic_views/test_base.py::GetContextDataTest.test_object_at_custom_name_in_context_data
func TestObjectAtCustomNameInContextData(t *testing.T) {
	resp := contextOf(t, views.DetailView[Book]{
		Engine:            genericEngine(t),
		TemplateName:      "generic_views/detail.html",
		GetObject:         dummyBook,
		ContextObjectName: "pony",
	})
	djangotest.AssertEqual(t, contextValue(t, resp, "pony"), any(Book{Name: "dummy"}))
}

// Ported from Django: tests/generic_views/test_base.py::GetContextDataTest.test_object_in_get_context_data
func TestObjectInGetContextData(t *testing.T) {
	resp := contextOf(t, views.DetailView[Book]{
		Engine: genericEngine(t), TemplateName: "generic_views/detail.html", GetObject: dummyBook,
	})
	djangotest.AssertEqual(t, contextValue(t, resp, "object"), any(Book{Name: "dummy"}))
}

// Ported from Django: tests/generic_views/test_base.py::UseMultipleObjectMixinTest.test_use_queryset_from_view
func TestUseQuerysetFromView(t *testing.T) {
	items := []map[string]string{{"name": "John"}, {"name": "Yoko"}}
	resp := contextOf(t, views.ListView[map[string]string]{
		Engine: genericEngine(t), TemplateName: "generic_views/list.html", ObjectList: views.SliceList(items),
	})
	djangotest.AssertEqual(t, contextValue(t, resp, "object_list"), any(items))
}

// Ported from Django: tests/generic_views/test_base.py::UseMultipleObjectMixinTest.test_overwrite_queryset
//
// Django passes object_list to get_context_data. The Go counterpart is the
// GetContextData hook, which replaces the entry.
func TestOverwriteQueryset(t *testing.T) {
	items := []map[string]string{{"name": "John"}, {"name": "Yoko"}}
	other := []map[string]string{{"name": "Lennon"}, {"name": "Ono"}}
	resp := contextOf(t, views.ListView[map[string]string]{
		Engine:       genericEngine(t),
		TemplateName: "generic_views/list.html",
		ObjectList:   views.SliceList(items),
		GetContextData: func(_ *http.Request, ctx map[string]any) (map[string]any, error) {
			ctx["object_list"] = other
			return ctx, nil
		},
	})
	djangotest.AssertEqual(t, contextValue(t, resp, "object_list"), any(other))
}

// Ported from Django: tests/generic_views/test_base.py::SingleObjectTemplateResponseMixinTest.test_template_mixin_without_template
func TestTemplateMixinWithoutTemplate(t *testing.T) {
	logs := captureLogs(t)
	c := mount(t, "/{$}", views.DetailView[Book]{Engine: genericEngine(t), GetObject: dummyBook})
	assertImproperlyConfigured(t, logs, c.Get("/"),
		"DetailView requires a definition of TemplateName, DB with a registered model")
}
