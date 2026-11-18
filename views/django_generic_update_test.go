package views_test

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/forms"
	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/views"
)

// eachUpdateDB runs fn once per database dialect with the author that
// setUpTestData of Django's UpdateViewTests creates.
func eachUpdateDB(t *testing.T, fn func(t *testing.T, db *orm.DB, author Author)) {
	t.Helper()
	djangotest.EachDB(t, genericModels(), func(t *testing.T, db *orm.DB) {
		author := Author{Name: "Randall Munroe", Slug: "randall-munroe"}
		create(t, db, &author)
		fn(t, db, author)
	})
}

// authorUpdate is AuthorUpdate of Django's tests/generic_views/views.py.
// Django counts the calls of get_form in get_form_called_count; the Go view
// counts the calls of its Form function in *calls when calls is not nil.
// Form takes the place of fields = "__all__", as a view has one or the other.
func authorUpdate(t *testing.T, db *orm.DB, calls *int) views.UpdateView[Author] {
	t.Helper()
	return views.UpdateView[Author]{
		DB:         db,
		Engine:     genericEngine(t),
		SuccessURL: "/list/authors/",
		Form: func(*http.Request) (*forms.ModelForm[Author], error) {
			if calls != nil {
				*calls++
			}
			return forms.NewModelForm[Author](db, forms.WithFields(forms.AllFields)), nil
		},
	}
}

// naiveAuthorUpdate is NaiveAuthorUpdate of Django's
// tests/generic_views/views.py.
func naiveAuthorUpdate(t *testing.T, db *orm.DB, successURL string) views.UpdateView[Author] {
	t.Helper()
	return views.UpdateView[Author]{
		DB: db, Engine: genericEngine(t), SuccessURL: successURL, Fields: []string{forms.AllFields},
	}
}

// Ported from Django: tests/generic_views/test_edit.py::UpdateViewTests.test_update_post
func TestUpdateViewPost(t *testing.T) {
	eachUpdateDB(t, func(t *testing.T, db *orm.DB, author Author) {
		calls := 0
		c := mountEdit(t, db, "/edit/author/{pk}/update/{$}", authorUpdate(t, db, &calls))
		path := fmt.Sprintf("/edit/author/%d/update/", author.ID)
		resp := c.Get(path)
		djangotest.AssertStatus(t, resp, http.StatusOK)
		djangotest.AssertSequenceEqual(t, contextForm(t, resp).FieldNames(), []string{"name", "slug"})
		assertContextObject(t, db, resp, "object", author)
		assertContextObject(t, db, resp, "author", author)
		djangotest.AssertTemplateUsed(t, resp, "generic_views/author_form.html")
		djangotest.AssertEqual(t, calls, 1)

		// Modification with both POST and PUT (browser compatible).
		resp = c.Post(path, map[string]string{"name": "Randall Munroe (xkcd)", "slug": "randall-munroe"})
		djangotest.AssertStatus(t, resp, http.StatusFound)
		djangotest.AssertRedirects(t, resp, "/list/authors/")
		djangotest.AssertSequenceEqual(t, authorNames(t, db), []string{"Randall Munroe (xkcd)"})
	})
}

// Ported from Django: tests/generic_views/test_edit.py::UpdateViewTests.test_update_invalid
func TestUpdateViewInvalid(t *testing.T) {
	eachUpdateDB(t, func(t *testing.T, db *orm.DB, author Author) {
		calls := 0
		c := mountEdit(t, db, "/edit/author/{pk}/update/{$}", authorUpdate(t, db, &calls))
		resp := c.Post(fmt.Sprintf("/edit/author/%d/update/", author.ID),
			map[string]string{"name": strings.Repeat("A", 101), "slug": "randall-munroe"})
		djangotest.AssertStatus(t, resp, http.StatusOK)
		djangotest.AssertTemplateUsed(t, resp, "generic_views/author_form.html")
		djangotest.AssertEqual(t, contextForm(t, resp).Errors().Len(), 1)
		djangotest.AssertSequenceEqual(t, allOf[Author](t, db, "name"), []Author{author})
		djangotest.AssertEqual(t, calls, 1)
	})
}

// Ported from Django: tests/generic_views/test_edit.py::UpdateViewTests.test_update_with_object_url
func TestUpdateViewWithObjectURL(t *testing.T) {
	eachUpdateDB(t, func(t *testing.T, db *orm.DB, _ Author) {
		a := Artist{Name: "Rene Magritte"}
		create(t, db, &a)
		view := views.UpdateView[Artist]{DB: db, Engine: genericEngine(t), Fields: []string{forms.AllFields}}
		c := mountEdit(t, db, "/edit/artists/{pk}/update/{$}", view)
		resp := c.Post(fmt.Sprintf("/edit/artists/%d/update/", a.ID), map[string]string{"name": "Rene Magritte"})
		djangotest.AssertStatus(t, resp, http.StatusFound)
		djangotest.AssertRedirects(t, resp, fmt.Sprintf("/detail/artist/%d/", a.ID))
		djangotest.AssertSequenceEqual(t, allOf[Artist](t, db, "id"), []Artist{a})
	})
}

// Ported from Django: tests/generic_views/test_edit.py::UpdateViewTests.test_update_with_redirect
func TestUpdateViewWithRedirect(t *testing.T) {
	eachUpdateDB(t, func(t *testing.T, db *orm.DB, author Author) {
		c := mountEdit(t, db,
			"/edit/author/{pk}/update/redirect/{$}", naiveAuthorUpdate(t, db, "/edit/authors/create/"))
		resp := c.Post(fmt.Sprintf("/edit/author/%d/update/redirect/", author.ID),
			map[string]string{"name": "Randall Munroe (author of xkcd)", "slug": "randall-munroe"})
		djangotest.AssertStatus(t, resp, http.StatusFound)
		djangotest.AssertRedirects(t, resp, "/edit/authors/create/")
		djangotest.AssertSequenceEqual(t, authorNames(t, db), []string{"Randall Munroe (author of xkcd)"})
	})
}

// Ported from Django: tests/generic_views/test_edit.py::UpdateViewTests.test_update_with_interpolated_redirect
func TestUpdateViewWithInterpolatedRedirect(t *testing.T) {
	eachUpdateDB(t, func(t *testing.T, db *orm.DB, author Author) {
		c := mountEdit(t, db,
			"/edit/author/{pk}/update/interpolate_redirect/{$}",
			naiveAuthorUpdate(t, db, "/edit/author/{id}/update/"),
			"/edit/author/{pk}/update/interpolate_redirect_nonascii/{$}",
			naiveAuthorUpdate(t, db, "/%C3%A9dit/author/{id}/update/"),
		)
		resp := c.Post(fmt.Sprintf("/edit/author/%d/update/interpolate_redirect/", author.ID),
			map[string]string{"name": "Randall Munroe (author of xkcd)", "slug": "randall-munroe"})
		djangotest.AssertSequenceEqual(t, authorNames(t, db), []string{"Randall Munroe (author of xkcd)"})
		djangotest.AssertStatus(t, resp, http.StatusFound)
		pk := allOf[Author](t, db, "name")[0].ID
		djangotest.AssertRedirects(t, resp, fmt.Sprintf("/edit/author/%d/update/", pk))

		// Also test with escaped chars in URL.
		resp = c.Post(fmt.Sprintf("/edit/author/%d/update/interpolate_redirect_nonascii/", author.ID),
			map[string]string{"name": "John Doe", "slug": "john-doe"})
		djangotest.AssertStatus(t, resp, http.StatusFound)
		john, err := orm.Query[Author](db).Filter("name", "John Doe").Get(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		djangotest.AssertRedirects(t, resp, fmt.Sprintf("/%%C3%%A9dit/author/%d/update/", john.ID))
	})
}

// Ported from Django: tests/generic_views/test_edit.py::UpdateViewTests.test_update_with_special_properties
func TestUpdateViewWithSpecialProperties(t *testing.T) {
	eachUpdateDB(t, func(t *testing.T, db *orm.DB, author Author) {
		view := views.UpdateView[Author]{
			DB:                db,
			Engine:            genericEngine(t),
			Form:              authorForm(db),
			TemplateName:      "generic_views/form.html",
			ContextObjectName: "thingy",
			GetSuccessURL: func(_ *http.Request, obj Author) (string, error) {
				return fmt.Sprintf("/detail/author/%d/", obj.ID), nil
			},
		}
		c := mountEdit(t, db, "/edit/author/{pk}/update/special/{$}", view)
		path := fmt.Sprintf("/edit/author/%d/update/special/", author.ID)
		resp := c.Get(path)
		djangotest.AssertStatus(t, resp, http.StatusOK)
		assertAuthorForm(t, resp)
		assertContextObject(t, db, resp, "object", author)
		assertContextObject(t, db, resp, "thingy", author)
		assertNotInContext(t, resp, "author")
		djangotest.AssertTemplateUsed(t, resp, "generic_views/form.html")

		resp = c.Post(path, map[string]string{"name": "Randall Munroe (author of xkcd)", "slug": "randall-munroe"})
		djangotest.AssertStatus(t, resp, http.StatusFound)
		djangotest.AssertRedirects(t, resp, fmt.Sprintf("/detail/author/%d/", author.ID))
		djangotest.AssertSequenceEqual(t, authorNames(t, db), []string{"Randall Munroe (author of xkcd)"})
	})
}

// Ported from Django: tests/generic_views/test_edit.py::UpdateViewTests.test_update_without_redirect
//
// Django's message names success_url and get_absolute_url, the Go message
// names the field SuccessURL and the method GetAbsoluteURL. The Go view finds
// the mistake before it saves.
func TestUpdateViewWithoutRedirect(t *testing.T) {
	eachUpdateDB(t, func(t *testing.T, db *orm.DB, author Author) {
		logs := captureLogs(t)
		c := mount(t, "/edit/author/{pk}/update/naive/{$}", naiveAuthorUpdate(t, db, ""))
		resp := c.Post(fmt.Sprintf("/edit/author/%d/update/naive/", author.ID),
			map[string]string{"name": "Randall Munroe (author of xkcd)", "slug": "randall-munroe"})
		assertImproperlyConfigured(t, logs, resp,
			"No URL to redirect to. Either provide a SuccessURL or define a GetAbsoluteURL method on the model.")
		djangotest.AssertSequenceEqual(t, authorNames(t, db), []string{"Randall Munroe"})
	})
}

// Ported from Django: tests/generic_views/test_edit.py::UpdateViewTests.test_update_get_object
//
// OneAuthorUpdate looks up the author with pk 1, the pk setUpTestData gives
// it. The Go GetObject looks up the pk the fixture got.
func TestUpdateViewGetObject(t *testing.T) {
	eachUpdateDB(t, func(t *testing.T, db *orm.DB, author Author) {
		view := views.UpdateView[Author]{
			DB:         db,
			Engine:     genericEngine(t),
			SuccessURL: "/list/authors/",
			Fields:     []string{forms.AllFields},
			GetObject: func(r *http.Request) (Author, error) {
				return orm.Query[Author](db).Filter("id", author.ID).Get(r.Context())
			},
		}
		c := mountEdit(t, db, "/edit/author/update/{$}", view)
		resp := c.Get("/edit/author/update/")
		djangotest.AssertStatus(t, resp, http.StatusOK)
		djangotest.AssertSequenceEqual(t, contextForm(t, resp).FieldNames(), []string{"name", "slug"})
		if _, ok := contextValue(t, resp, "view").(views.UpdateView[Author]); !ok {
			t.Errorf("context[\"view\"] is %T, want views.UpdateView[Author]", contextValue(t, resp, "view"))
		}
		assertContextObject(t, db, resp, "object", author)
		assertContextObject(t, db, resp, "author", author)
		djangotest.AssertTemplateUsed(t, resp, "generic_views/author_form.html")

		// Modification with both POST and PUT (browser compatible).
		resp = c.Post("/edit/author/update/",
			map[string]string{"name": "Randall Munroe (xkcd)", "slug": "randall-munroe"})
		djangotest.AssertStatus(t, resp, http.StatusFound)
		djangotest.AssertRedirects(t, resp, "/list/authors/")
		djangotest.AssertSequenceEqual(t, authorNames(t, db), []string{"Randall Munroe (xkcd)"})
	})
}

// TestUpdateViewForm asserts that the form shows the values of the object,
// that Initial wins over them, and that the object in the context of an
// invalid submission is the object as it was loaded.
func TestUpdateViewForm(t *testing.T) {
	eachUpdateDB(t, func(t *testing.T, db *orm.DB, author Author) {
		view := naiveAuthorUpdate(t, db, "/list/authors/")
		path := fmt.Sprintf("/edit/author/%d/update/", author.ID)
		c := mountEdit(t, db, "/edit/author/{pk}/update/{$}", view)
		resp := c.Get(path)
		djangotest.AssertContains(t, resp, `name="name" value="Randall Munroe"`)

		view.Initial = map[string]any{"slug": "xkcd"}
		c = mountEdit(t, db, "/edit/author/{pk}/update/{$}", view)
		resp = c.Get(path)
		djangotest.AssertContains(t, resp, `name="name" value="Randall Munroe"`)
		djangotest.AssertContains(t, resp, `name="slug" value="xkcd"`)

		resp = c.Post(path, map[string]string{"name": strings.Repeat("A", 101), "slug": "x"})
		djangotest.AssertStatus(t, resp, http.StatusOK)
		assertContextObject(t, db, resp, "object", author)
	})
}

// TestUpdateViewMissingObject asserts that a request for an object that does
// not exist is answered with 404 and changes nothing.
func TestUpdateViewMissingObject(t *testing.T) {
	eachUpdateDB(t, func(t *testing.T, db *orm.DB, author Author) {
		c := mountEdit(t, db)
		path := fmt.Sprintf("/edit/author/%d/update/", author.ID+1)
		djangotest.AssertStatus(t, c.Get(path), http.StatusNotFound)
		resp := c.Post(path, map[string]string{"name": "Someone", "slug": "someone"})
		djangotest.AssertStatus(t, resp, http.StatusNotFound)
		djangotest.AssertSequenceEqual(t, authorNames(t, db), []string{"Randall Munroe"})
	})
}
