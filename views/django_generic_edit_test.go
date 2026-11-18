package views_test

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/forms"
	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/views"
)

// eachDeleteDB runs fn once per database dialect with the author that
// setUpTestData of Django's DeleteViewTests creates.
func eachDeleteDB(t *testing.T, fn func(t *testing.T, db *orm.DB, author Author)) {
	t.Helper()
	djangotest.EachDB(t, genericModels(), func(t *testing.T, db *orm.DB) {
		author := Author{Name: "Randall Munroe", Slug: "randall-munroe"}
		create(t, db, &author)
		fn(t, db, author)
	})
}

// authorDelete is AuthorDelete of Django's tests/generic_views/views.py.
func authorDelete(t *testing.T, db *orm.DB) views.DeleteView[Author] {
	t.Helper()
	return views.DeleteView[Author]{DB: db, Engine: genericEngine(t), SuccessURL: "/list/authors/"}
}

// mountDelete mounts the delete view next to the author list its success URL
// points to, so that AssertRedirects can load the target.
func mountDelete(t *testing.T, db *orm.DB, view views.DeleteView[Author]) *djangotest.Client {
	t.Helper()
	return mount(t,
		"/edit/author/{pk}/delete/{$}", view,
		"/list/authors/{$}", authorList(t, db),
	)
}

// assertAuthorCount asserts the number of authors in the database.
func assertAuthorCount(t *testing.T, db *orm.DB, want int) {
	t.Helper()
	djangotest.AssertEqual(t, len(allOf[Author](t, db)), want)
}

// Ported from Django: tests/generic_views/test_edit.py::DeleteViewTests.test_delete_by_post
func TestDeleteViewByPost(t *testing.T) {
	eachDeleteDB(t, func(t *testing.T, db *orm.DB, author Author) {
		c := mountDelete(t, db, authorDelete(t, db))
		path := fmt.Sprintf("/edit/author/%d/delete/", author.ID)

		resp := c.Get(path)
		djangotest.AssertStatus(t, resp, http.StatusOK)
		assertContextObject(t, db, resp, "object", author)
		assertContextObject(t, db, resp, "author", author)
		djangotest.AssertTemplateUsed(t, resp, "generic_views/author_confirm_delete.html")
		assertAuthorCount(t, db, 1)

		resp = c.Post(path, nil)
		djangotest.AssertRedirects(t, resp, "/list/authors/")
		assertAuthorCount(t, db, 0)
	})
}

// Ported from Django: tests/generic_views/test_edit.py::DeleteViewTests.test_delete_by_delete
func TestDeleteViewByDelete(t *testing.T) {
	eachDeleteDB(t, func(t *testing.T, db *orm.DB, author Author) {
		c := mountDelete(t, db, authorDelete(t, db))
		resp := c.Delete(fmt.Sprintf("/edit/author/%d/delete/", author.ID), nil)
		djangotest.AssertRedirects(t, resp, "/list/authors/")
		assertAuthorCount(t, db, 0)
	})
}

// Ported from Django: tests/generic_views/test_edit.py::DeleteViewTests.test_delete_with_redirect
func TestDeleteViewWithRedirect(t *testing.T) {
	eachDeleteDB(t, func(t *testing.T, db *orm.DB, author Author) {
		view := authorDelete(t, db)
		view.SuccessURL = "/edit/authors/create/"
		c := mountEdit(t, db, "/edit/author/{pk}/delete/redirect/{$}", view)
		resp := c.Post(fmt.Sprintf("/edit/author/%d/delete/redirect/", author.ID), nil)
		djangotest.AssertStatus(t, resp, http.StatusFound)
		djangotest.AssertRedirects(t, resp, "/edit/authors/create/")
		assertAuthorCount(t, db, 0)
	})
}

// Ported from Django: tests/generic_views/test_edit.py::DeleteViewTests.test_delete_with_interpolated_redirect
func TestDeleteViewWithInterpolatedRedirect(t *testing.T) {
	eachDeleteDB(t, func(t *testing.T, db *orm.DB, author Author) {
		view := authorDelete(t, db)
		view.SuccessURL = "/edit/authors/create/?deleted={id}"
		nonASCII := authorDelete(t, db)
		nonASCII.SuccessURL = "/%C3%A9dit/authors/create/?deleted={id}"
		c := mountEdit(t, db,
			"/edit/author/{pk}/delete/interpolate_redirect/{$}", view,
			"/edit/author/{pk}/delete/interpolate_redirect_nonascii/{$}", nonASCII,
		)

		resp := c.Post(fmt.Sprintf("/edit/author/%d/delete/interpolate_redirect/", author.ID), nil)
		djangotest.AssertStatus(t, resp, http.StatusFound)
		djangotest.AssertRedirects(t, resp, fmt.Sprintf("/edit/authors/create/?deleted=%d", author.ID))
		assertAuthorCount(t, db, 0)

		// Also test with escaped chars in URL.
		a := Author{Name: "Randall Munroe", Slug: "randall-munroe"}
		create(t, db, &a)
		resp = c.Post(fmt.Sprintf("/edit/author/%d/delete/interpolate_redirect_nonascii/", a.ID), nil)
		djangotest.AssertStatus(t, resp, http.StatusFound)
		djangotest.AssertRedirects(t, resp, fmt.Sprintf("/%%C3%%A9dit/authors/create/?deleted=%d", a.ID))
	})
}

// Ported from Django: tests/generic_views/test_edit.py::DeleteViewTests.test_delete_with_special_properties
func TestDeleteViewWithSpecialProperties(t *testing.T) {
	eachDeleteDB(t, func(t *testing.T, db *orm.DB, author Author) {
		view := views.DeleteView[Author]{
			DB:                db,
			Engine:            genericEngine(t),
			Queryset:          orm.Query[Author](db),
			TemplateName:      "generic_views/confirm_delete.html",
			ContextObjectName: "thingy",
			SuccessURL:        "/list/authors/",
		}
		c := mount(t,
			"/edit/author/{pk}/delete/special/{$}", view,
			"/list/authors/{$}", authorList(t, db),
		)
		path := fmt.Sprintf("/edit/author/%d/delete/special/", author.ID)

		resp := c.Get(path)
		djangotest.AssertStatus(t, resp, http.StatusOK)
		assertContextObject(t, db, resp, "object", author)
		assertContextObject(t, db, resp, "thingy", author)
		assertNotInContext(t, resp, "author")
		djangotest.AssertTemplateUsed(t, resp, "generic_views/confirm_delete.html")

		resp = c.Post(path, nil)
		djangotest.AssertRedirects(t, resp, "/list/authors/")
		assertAuthorCount(t, db, 0)
	})
}

// Ported from Django: tests/generic_views/test_edit.py::DeleteViewTests.test_delete_without_redirect
//
// Django's message names success_url, the Go message names the field
// SuccessURL.
func TestDeleteViewWithoutRedirect(t *testing.T) {
	eachDeleteDB(t, func(t *testing.T, db *orm.DB, author Author) {
		logs := captureLogs(t)
		view := views.DeleteView[Author]{DB: db, Engine: genericEngine(t)}
		c := mount(t, "/edit/author/{pk}/delete/naive/{$}", view)
		resp := c.Post(fmt.Sprintf("/edit/author/%d/delete/naive/", author.ID), nil)
		assertImproperlyConfigured(t, logs, resp, "No URL to redirect to. Provide a SuccessURL.")
		assertAuthorCount(t, db, 1)
	})
}

// confirmDeleteForm is ConfirmDeleteForm of Django's
// tests/generic_views/forms.py.
func confirmDeleteForm(*http.Request) (*forms.Form, error) {
	form := forms.New(forms.F("confirm", forms.NewBooleanField()))
	form.CleanWith(func(form *forms.Form) error {
		if _, ok := form.CleanedData()["confirm"]; !ok {
			return forms.NewValidationError("You must confirm the delete.", "")
		}
		return nil
	})
	return form, nil
}

// authorDeleteForm is AuthorDeleteFormView of Django's
// tests/generic_views/views.py.
func authorDeleteForm(t *testing.T, db *orm.DB) views.DeleteView[Author] {
	t.Helper()
	view := authorDelete(t, db)
	view.Form = confirmDeleteForm
	return view
}

// Ported from Django: tests/generic_views/test_edit.py::DeleteViewTests.test_delete_with_form_as_post
func TestDeleteViewWithFormAsPost(t *testing.T) {
	eachDeleteDB(t, func(t *testing.T, db *orm.DB, author Author) {
		c := mountEdit(t, db, "/edit/author/{pk}/delete/form/{$}", authorDeleteForm(t, db))
		path := fmt.Sprintf("/edit/author/%d/delete/form/", author.ID)
		resp := c.Get(path)
		djangotest.AssertStatus(t, resp, http.StatusOK)
		assertContextObject(t, db, resp, "object", author)
		assertContextObject(t, db, resp, "author", author)
		djangotest.AssertTemplateUsed(t, resp, "generic_views/author_confirm_delete.html")

		resp = c.Post(path, map[string]string{"confirm": "True"})
		djangotest.AssertStatus(t, resp, http.StatusFound)
		djangotest.AssertRedirects(t, resp, "/list/authors/")
		assertAuthorCount(t, db, 0)
	})
}

// Ported from Django: tests/generic_views/test_edit.py::DeleteViewTests.test_delete_with_form_as_post_with_validation_error
func TestDeleteViewWithFormAsPostWithValidationError(t *testing.T) {
	eachDeleteDB(t, func(t *testing.T, db *orm.DB, author Author) {
		c := mountEdit(t, db, "/edit/author/{pk}/delete/form/{$}", authorDeleteForm(t, db))
		path := fmt.Sprintf("/edit/author/%d/delete/form/", author.ID)
		resp := c.Get(path)
		djangotest.AssertStatus(t, resp, http.StatusOK)
		assertContextObject(t, db, resp, "object", author)
		assertContextObject(t, db, resp, "author", author)
		djangotest.AssertTemplateUsed(t, resp, "generic_views/author_confirm_delete.html")

		resp = c.Post(path, nil)
		djangotest.AssertStatus(t, resp, http.StatusOK)
		errs := contextForm(t, resp).Errors()
		djangotest.AssertEqual(t, errs.Len(), 2)
		djangotest.AssertEqual(t, errs.Messages(), map[string][]string{
			"__all__": {"You must confirm the delete."},
			"confirm": {"This field is required."},
		})
		assertAuthorCount(t, db, 1)
	})
}

// TestDeleteViewFormInContext asserts that the confirmation page of a view
// without Form has an unbound form without fields under "form", the Form of
// Django's BaseDeleteView, and that DELETE deletes without the form.
func TestDeleteViewFormInContext(t *testing.T) {
	eachDeleteDB(t, func(t *testing.T, db *orm.DB, author Author) {
		c := mountDelete(t, db, authorDelete(t, db))
		resp := c.Get(fmt.Sprintf("/edit/author/%d/delete/", author.ID))
		form := contextForm(t, resp)
		djangotest.AssertEqual(t, form.IsBound(), false)
		djangotest.AssertEqual(t, len(form.FieldNames()), 0)

		c = mount(t, "/edit/author/{pk}/delete/form/{$}", authorDeleteForm(t, db), "/list/authors/{$}", authorList(t, db))
		resp = c.Delete(fmt.Sprintf("/edit/author/%d/delete/form/", author.ID), nil)
		djangotest.AssertRedirects(t, resp, "/list/authors/")
		assertAuthorCount(t, db, 0)
	})
}

// TestDeleteViewMissingObject asserts that POST for an object that does not
// exist is answered with 404 and deletes nothing.
func TestDeleteViewMissingObject(t *testing.T) {
	eachDeleteDB(t, func(t *testing.T, db *orm.DB, author Author) {
		c := mountDelete(t, db, authorDelete(t, db))
		resp := c.Post(fmt.Sprintf("/edit/author/%d/delete/", author.ID+1), nil)
		djangotest.AssertStatus(t, resp, http.StatusNotFound)
		assertAuthorCount(t, db, 1)
	})
}

// TestDeleteViewRefusesDisallowedRedirect asserts that a success URL with a
// scheme a redirect may not use is answered with 400 before anything is
// deleted.
func TestDeleteViewRefusesDisallowedRedirect(t *testing.T) {
	eachDeleteDB(t, func(t *testing.T, db *orm.DB, author Author) {
		view := authorDelete(t, db)
		view.SuccessURL = "javascript:alert(1)"
		c := mount(t, "/edit/author/{pk}/delete/{$}", view)
		resp := c.Post(fmt.Sprintf("/edit/author/%d/delete/", author.ID), nil)
		djangotest.AssertStatus(t, resp, http.StatusBadRequest)
		assertAuthorCount(t, db, 1)
	})
}

// TestDeleteViewEncodesInterpolatedValues asserts that a field value cannot
// add a query parameter to the success URL: it is percent-encoded, where
// Django's str.format inserts it as it is.
func TestDeleteViewEncodesInterpolatedValues(t *testing.T) {
	eachDeleteDB(t, func(t *testing.T, db *orm.DB, _ Author) {
		author := Author{Name: "a&next=//example.com/", Slug: "a"}
		create(t, db, &author)
		view := authorDelete(t, db)
		view.SuccessURL = "/list/authors/?deleted={name}"
		c := mount(t, "/edit/author/{pk}/delete/{$}", view)
		resp := c.Post(fmt.Sprintf("/edit/author/%d/delete/", author.ID), nil)
		djangotest.AssertStatus(t, resp, http.StatusFound)
		djangotest.AssertEqual(t, resp.URL(), "/list/authors/?deleted=a%26next%3D%2F%2Fexample.com%2F")
	})
}

// TestDeleteViewHooks asserts that GetSuccessURL and DeleteObject take the
// place of the success URL and of the deletion, and that a failed deletion is
// answered with the 500 page.
func TestDeleteViewHooks(t *testing.T) {
	eachDeleteDB(t, func(t *testing.T, db *orm.DB, author Author) {
		var deleted []int64
		view := views.DeleteView[Author]{
			DB:     db,
			Engine: genericEngine(t),
			GetSuccessURL: func(_ *http.Request, obj Author) (string, error) {
				return "/authors/gone/" + obj.Slug + "/", nil
			},
			DeleteObject: func(_ *http.Request, obj Author) error {
				deleted = append(deleted, obj.ID)
				return nil
			},
		}
		path := fmt.Sprintf("/edit/author/%d/delete/", author.ID)
		resp := mount(t, "/edit/author/{pk}/delete/{$}", view).Post(path, nil)
		djangotest.AssertStatus(t, resp, http.StatusFound)
		djangotest.AssertEqual(t, resp.URL(), "/authors/gone/randall-munroe/")
		djangotest.AssertSequenceEqual(t, deleted, []int64{author.ID})
		assertAuthorCount(t, db, 1)

		logs := captureLogs(t)
		view.DeleteObject = func(*http.Request, Author) error { return errors.New("row is locked") }
		resp = mount(t, "/edit/author/{pk}/delete/{$}", view).Post(path, nil)
		djangotest.AssertStatus(t, resp, http.StatusInternalServerError)
		djangotest.AssertEqual(t, len(logs.errs()), 1)
	})
}
