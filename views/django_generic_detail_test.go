package views_test

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/templates"
	"github.com/oliverhaas/djangogo/views"
)

// authorDetail is AuthorDetail of Django's tests/generic_views/views.py.
func authorDetail(t *testing.T, db *orm.DB) views.DetailView[Author] {
	t.Helper()
	return views.DetailView[Author]{DB: db, Engine: genericEngine(t)}
}

// assertAuthorDetail asserts the response of a detail view that shows the
// author under "object" and under the given second key.
func assertAuthorDetail(t *testing.T, db *orm.DB, resp *djangotest.Response, key string, want Author, template string) {
	t.Helper()
	djangotest.AssertStatus(t, resp, http.StatusOK)
	assertContextObject(t, db, resp, "object", want)
	assertContextObject(t, db, resp, key, want)
	djangotest.AssertTemplateUsed(t, resp, template)
}

// Ported from Django: tests/generic_views/test_detail.py::DetailViewTest.test_simple_object
func TestDetailViewSimpleObject(t *testing.T) {
	view := views.DetailView[map[string]string]{
		Engine:       genericEngine(t),
		TemplateName: "generic_views/detail.html",
		GetObject: func(*http.Request) (map[string]string, error) {
			return map[string]string{"foo": "bar"}, nil
		},
	}
	resp := mount(t, "/detail/obj/{$}", view).Get("/detail/obj/")
	djangotest.AssertStatus(t, resp, http.StatusOK)
	djangotest.AssertEqual(t, contextValue(t, resp, "object"), any(map[string]string{"foo": "bar"}))
	if v, ok := contextValue(t, resp, "view").(views.DetailView[map[string]string]); !ok {
		t.Errorf("context[\"view\"] is %T, want the detail view", v)
	}
	djangotest.AssertTemplateUsed(t, resp, "generic_views/detail.html")
}

// Ported from Django: tests/generic_views/test_detail.py::DetailViewTest.test_detail_by_pk
func TestDetailViewByPK(t *testing.T) {
	eachGenericDB(t, func(t *testing.T, db *orm.DB, d genericData) {
		c := mount(t, "/detail/author/{pk}/{$}", authorDetail(t, db))
		resp := c.Get(fmt.Sprintf("/detail/author/%d/", d.author1.ID))
		assertAuthorDetail(t, db, resp, "author", d.author1, "generic_views/author_detail.html")
	})
}

// Ported from Django: tests/generic_views/test_detail.py::DetailViewTest.test_detail_missing_object
func TestDetailViewMissingObject(t *testing.T) {
	eachGenericDB(t, func(t *testing.T, db *orm.DB, _ genericData) {
		resp := mount(t, "/detail/author/{pk}/{$}", authorDetail(t, db)).Get("/detail/author/500/")
		djangotest.AssertStatus(t, resp, http.StatusNotFound)
		djangotest.AssertEqual(t, contextValue(t, resp, "exception"), any("No author found matching the query"))
	})
}

// Ported from Django: tests/generic_views/test_detail.py::DetailViewTest.test_detail_object_does_not_exist
//
// In Django a queryset that raises the base ObjectDoesNotExist, not the
// DoesNotExist of the model, is not turned into a 404. Go has the one sentinel
// orm.ErrDoesNotExist, which the central error mapping answers with a 404 also
// when a hook returns it.
func TestDetailViewObjectDoesNotExist(t *testing.T) {
	view := views.DetailView[Book]{
		Engine:       genericEngine(t),
		TemplateName: "generic_views/detail.html",
		GetObject:    func(*http.Request) (Book, error) { return Book{}, orm.ErrDoesNotExist },
	}
	resp := mount(t, "/detail/doesnotexist/{pk}/{$}", view).Get("/detail/doesnotexist/1/")
	djangotest.AssertStatus(t, resp, http.StatusNotFound)
}

// Ported from Django: tests/generic_views/test_detail.py::DetailViewTest.test_detail_by_custom_pk
func TestDetailViewByCustomPK(t *testing.T) {
	eachGenericDB(t, func(t *testing.T, db *orm.DB, d genericData) {
		view := authorDetail(t, db)
		view.PKURLKwarg = "foo"
		c := mount(t, "/detail/author/bycustompk/{foo}/{$}", view)
		resp := c.Get(fmt.Sprintf("/detail/author/bycustompk/%d/", d.author1.ID))
		assertAuthorDetail(t, db, resp, "author", d.author1, "generic_views/author_detail.html")
	})
}

// Ported from Django: tests/generic_views/test_detail.py::DetailViewTest.test_detail_by_slug
func TestDetailViewBySlug(t *testing.T) {
	eachGenericDB(t, func(t *testing.T, db *orm.DB, d genericData) {
		c := mount(t, "/detail/author/byslug/{slug}/{$}", authorDetail(t, db))
		resp := c.Get("/detail/author/byslug/scott-rosenberg/")
		assertAuthorDetail(t, db, resp, "author", d.author2, "generic_views/author_detail.html")
	})
}

// Ported from Django: tests/generic_views/test_detail.py::DetailViewTest.test_detail_by_custom_slug
func TestDetailViewByCustomSlug(t *testing.T) {
	eachGenericDB(t, func(t *testing.T, db *orm.DB, d genericData) {
		view := authorDetail(t, db)
		view.SlugURLKwarg = "foo"
		c := mount(t, "/detail/author/bycustomslug/{foo}/{$}", view)
		resp := c.Get("/detail/author/bycustomslug/scott-rosenberg/")
		assertAuthorDetail(t, db, resp, "author", d.author2, "generic_views/author_detail.html")
	})
}

// Ported from Django: tests/generic_views/test_detail.py::DetailViewTest.test_detail_by_pk_ignore_slug
// Ported from Django: tests/generic_views/test_detail.py::DetailViewTest.test_detail_by_pk_ignore_slug_mismatch
//
// Django's route is "<int:pk>-<slug>/". A wildcard of a Go route pattern spans
// a whole path segment, so the route is "{pk}/{slug}/".
func TestDetailViewByPKIgnoreSlug(t *testing.T) {
	eachGenericDB(t, func(t *testing.T, db *orm.DB, d genericData) {
		c := mount(t, "/detail/author/bypkignoreslug/{pk}/{slug}/{$}", authorDetail(t, db))
		for _, slug := range []string{"roberto-bolano", "scott-rosenberg"} {
			resp := c.Get(fmt.Sprintf("/detail/author/bypkignoreslug/%d/%s/", d.author1.ID, slug))
			assertAuthorDetail(t, db, resp, "author", d.author1, "generic_views/author_detail.html")
		}
	})
}

// Ported from Django: tests/generic_views/test_detail.py::DetailViewTest.test_detail_by_pk_and_slug
// Ported from Django: tests/generic_views/test_detail.py::DetailViewTest.test_detail_by_pk_and_slug_mismatch_404
func TestDetailViewByPKAndSlug(t *testing.T) {
	eachGenericDB(t, func(t *testing.T, db *orm.DB, d genericData) {
		view := authorDetail(t, db)
		view.QueryPKAndSlug = true
		c := mount(t, "/detail/author/bypkandslug/{pk}/{slug}/{$}", view)
		resp := c.Get(fmt.Sprintf("/detail/author/bypkandslug/%d/roberto-bolano/", d.author1.ID))
		assertAuthorDetail(t, db, resp, "author", d.author1, "generic_views/author_detail.html")
		resp = c.Get(fmt.Sprintf("/detail/author/bypkandslug/%d/scott-rosenberg/", d.author1.ID))
		djangotest.AssertStatus(t, resp, http.StatusNotFound)
	})
}

// Ported from Django: tests/generic_views/test_detail.py::DetailViewTest.test_verbose_name
func TestDetailViewVerboseName(t *testing.T) {
	eachGenericDB(t, func(t *testing.T, db *orm.DB, d genericData) {
		view := views.DetailView[Artist]{DB: db, Engine: genericEngine(t)}
		resp := mount(t, "/detail/artist/{pk}/{$}", view).Get(fmt.Sprintf("/detail/artist/%d/", d.artist1.ID))
		djangotest.AssertStatus(t, resp, http.StatusOK)
		assertContextObject(t, db, resp, "object", d.artist1)
		assertContextObject(t, db, resp, "artist", d.artist1)
		djangotest.AssertTemplateUsed(t, resp, "generic_views/artist_detail.html")
	})
}

// Ported from Django: tests/generic_views/test_detail.py::DetailViewTest.test_template_name
func TestDetailViewTemplateName(t *testing.T) {
	eachGenericDB(t, func(t *testing.T, db *orm.DB, d genericData) {
		view := authorDetail(t, db)
		view.TemplateName = "generic_views/about.html"
		c := mount(t, "/detail/author/{pk}/template_name/{$}", view)
		resp := c.Get(fmt.Sprintf("/detail/author/%d/template_name/", d.author1.ID))
		assertAuthorDetail(t, db, resp, "author", d.author1, "generic_views/about.html")
	})
}

// Ported from Django: tests/generic_views/test_detail.py::DetailViewTest.test_template_name_suffix
func TestDetailViewTemplateNameSuffix(t *testing.T) {
	eachGenericDB(t, func(t *testing.T, db *orm.DB, d genericData) {
		view := authorDetail(t, db)
		view.TemplateNameSuffix = "_view"
		c := mount(t, "/detail/author/{pk}/template_name_suffix/{$}", view)
		resp := c.Get(fmt.Sprintf("/detail/author/%d/template_name_suffix/", d.author1.ID))
		assertAuthorDetail(t, db, resp, "author", d.author1, "generic_views/author_view.html")
	})
}

// Ported from Django: tests/generic_views/test_detail.py::DetailViewTest.test_template_name_field
func TestDetailViewTemplateNameField(t *testing.T) {
	eachGenericDB(t, func(t *testing.T, db *orm.DB, d genericData) {
		view := views.DetailView[Page]{DB: db, Engine: genericEngine(t), TemplateNameField: "template"}
		c := mount(t, "/detail/page/{pk}/field/{$}", view)
		resp := c.Get(fmt.Sprintf("/detail/page/%d/field/", d.page1.ID))
		djangotest.AssertStatus(t, resp, http.StatusOK)
		assertContextObject(t, db, resp, "object", d.page1)
		assertContextObject(t, db, resp, "page", d.page1)
		djangotest.AssertTemplateUsed(t, resp, "generic_views/page_template.html")
	})
}

// TestDetailViewTemplateNameFieldStaysInTemplateDir asserts that a template
// name stored in the database cannot select a file outside the template
// directory: the name is skipped and the default template is rendered.
func TestDetailViewTemplateNameFieldStaysInTemplateDir(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "templates", "generic_views")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	for path, body := range map[string]string{
		filepath.Join(root, "outside.html"):    "OUTSIDE",
		filepath.Join(dir, "page_detail.html"): "default template",
	} {
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	eng, err := templates.NewEngine(filepath.Join(root, "templates"))
	if err != nil {
		t.Fatal(err)
	}
	db := djangotest.NewDB(t, genericModels()...)
	for _, name := range []string{"../outside.html", filepath.Join(root, "outside.html")} {
		page := Page{Content: "content", Template: name}
		create(t, db, &page)
		view := views.DetailView[Page]{DB: db, Engine: eng, TemplateNameField: "template"}
		resp := mount(t, "/detail/page/{pk}/field/{$}", view).Get(fmt.Sprintf("/detail/page/%d/field/", page.ID))
		djangotest.AssertStatus(t, resp, http.StatusOK)
		djangotest.AssertEqual(t, resp.Content(), "default template")
	}
}

// Ported from Django: tests/generic_views/test_detail.py::DetailViewTest.test_context_object_name
func TestDetailViewContextObjectName(t *testing.T) {
	eachGenericDB(t, func(t *testing.T, db *orm.DB, d genericData) {
		view := authorDetail(t, db)
		view.ContextObjectName = "thingy"
		c := mount(t, "/detail/author/{pk}/context_object_name/{$}", view)
		resp := c.Get(fmt.Sprintf("/detail/author/%d/context_object_name/", d.author1.ID))
		assertAuthorDetail(t, db, resp, "thingy", d.author1, "generic_views/author_detail.html")
		assertNotInContext(t, resp, "author")
	})
}

// Ported from Django: tests/generic_views/test_detail.py::DetailViewTest.test_duplicated_context_object_name
func TestDetailViewDuplicatedContextObjectName(t *testing.T) {
	eachGenericDB(t, func(t *testing.T, db *orm.DB, d genericData) {
		view := authorDetail(t, db)
		view.ContextObjectName = "object"
		c := mount(t, "/detail/author/{pk}/dupe_context_object_name/{$}", view)
		resp := c.Get(fmt.Sprintf("/detail/author/%d/dupe_context_object_name/", d.author1.ID))
		assertAuthorDetail(t, db, resp, "object", d.author1, "generic_views/author_detail.html")
		assertNotInContext(t, resp, "author")
	})
}

// Ported from Django: tests/generic_views/test_detail.py::DetailViewTest.test_custom_detail
//
// Django's AuthorCustomDetail overrides get() and renders a context of its
// own. The Go view replaces the context in the GetContextData hook, which may
// return a new map.
func TestDetailViewCustomDetail(t *testing.T) {
	eachGenericDB(t, func(t *testing.T, db *orm.DB, d genericData) {
		view := authorDetail(t, db)
		view.TemplateName = "generic_views/author_detail.html"
		view.GetContextData = func(_ *http.Request, ctx map[string]any) (map[string]any, error) {
			return map[string]any{"custom_author": ctx["author"]}, nil
		}
		c := mount(t, "/detail/author/{pk}/custom_detail/{$}", view)
		resp := c.Get(fmt.Sprintf("/detail/author/%d/custom_detail/", d.author1.ID))
		djangotest.AssertStatus(t, resp, http.StatusOK)
		assertContextObject(t, db, resp, "custom_author", d.author1)
		assertNotInContext(t, resp, "author")
		assertNotInContext(t, resp, "object")
		djangotest.AssertTemplateUsed(t, resp, "generic_views/author_detail.html")
	})
}

// Ported from Django: tests/generic_views/test_detail.py::DetailViewTest.test_invalid_url
//
// Django raises AttributeError for a route without pk and slug. The Go view
// reports it as a configuration error.
func TestDetailViewInvalidURL(t *testing.T) {
	eachGenericDB(t, func(t *testing.T, db *orm.DB, _ genericData) {
		logs := captureLogs(t)
		c := mount(t, "/detail/author/invalid/url/{$}", authorDetail(t, db))
		assertImproperlyConfigured(t, logs, c.Get("/detail/author/invalid/url/"),
			"DetailView must be called with either an object pk or a slug in the route pattern")
	})
}

// Ported from Django: tests/generic_views/test_detail.py::DetailViewTest.test_invalid_queryset
//
// Django clears the queryset of a view without model. The Go counterpart is a
// view without DB.
func TestDetailViewInvalidQueryset(t *testing.T) {
	logs := captureLogs(t)
	c := mount(t, "/detail/author/invalid/qs/{$}", views.DetailView[Author]{Engine: genericEngine(t)})
	assertImproperlyConfigured(t, logs, c.Get("/detail/author/invalid/qs/"),
		"DetailView needs DB and a registered model")
}
