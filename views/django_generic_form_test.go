package views_test

import (
	"context"
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/auth"
	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/forms"
	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/views"
)

// contactForm is ContactForm of Django's tests/generic_views/forms.py.
func contactForm(*http.Request) (*forms.Form, error) {
	return forms.New(
		forms.F("name", forms.NewCharField()),
		forms.F("message", forms.NewCharField(forms.UseWidget(forms.Textarea{}))),
	), nil
}

// contactView is ContactView of Django's tests/generic_views/views.py.
func contactView(t *testing.T) views.FormView {
	t.Helper()
	return views.FormView{
		Engine:       genericEngine(t),
		Form:         contactForm,
		SuccessURL:   "/list/authors/",
		TemplateName: "generic_views/form.html",
	}
}

// contextForm returns the form of the template context.
func contextForm(t *testing.T, resp *djangotest.Response) *forms.Form {
	t.Helper()
	form, ok := forms.FormOf(contextValue(t, resp, "form"))
	if !ok {
		t.Fatalf("context[\"form\"] is %T, want a form", contextValue(t, resp, "form"))
	}
	return form
}

// Ported from Django: tests/generic_views/test_edit.py::FormMixinTests.test_initial_data
//
// Django changes the dictionary that get_initial returns and calls it again.
// The Go view has no get_initial: the test changes the initial values of the
// form of one request and asserts that the next request does not see the
// change.
func TestFormMixinInitialData(t *testing.T) {
	var seen []map[string]any
	view := contactView(t)
	view.Initial = map[string]any{"name": "Me"}
	view.GetContextData = func(_ *http.Request, ctx map[string]any) (map[string]any, error) {
		initial := ctx["form"].(*forms.Form).Initial
		seen = append(seen, maps.Clone(initial))
		initial["foo"] = "bar"
		return ctx, nil
	}
	c := mount(t, "/contact/{$}", view)
	c.Get("/contact/")
	c.Get("/contact/")
	djangotest.AssertEqual(t, len(seen), 2)
	djangotest.AssertEqual(t, seen[1], map[string]any{"name": "Me"})
	djangotest.AssertEqual(t, view.Initial, map[string]any{"name": "Me"})
}

// Ported from Django: tests/generic_views/test_edit.py::FormMixinTests.test_get_prefix
func TestFormMixinGetPrefix(t *testing.T) {
	resp := mount(t, "/contact/{$}", contactView(t)).Get("/contact/")
	djangotest.AssertEqual(t, contextForm(t, resp).Prefix, "")

	view := contactView(t)
	view.Prefix = "test"
	resp = mount(t, "/contact/{$}", view).Get("/contact/")
	djangotest.AssertEqual(t, contextForm(t, resp).Prefix, "test")
	djangotest.AssertContains(t, resp, `name="test-name"`)
}

// Ported from Django: tests/generic_views/test_edit.py::FormMixinTests.test_get_form
//
// Django passes a form class to get_form or sets form_class. The Go view has
// the Form function in place of both: the form in the context is the form it
// returns.
func TestFormMixinGetForm(t *testing.T) {
	var returned *forms.Form
	view := contactView(t)
	view.Form = func(r *http.Request) (*forms.Form, error) {
		returned, _ = contactForm(r)
		return returned, nil
	}
	resp := mount(t, "/contact/{$}", view).Get("/contact/")
	if contextForm(t, resp) != returned {
		t.Error("the form in the context is not the form Form returned")
	}
}

// Ported from Django: tests/generic_views/test_edit.py::FormMixinTests.test_get_context_data
func TestFormMixinGetContextData(t *testing.T) {
	resp := mount(t, "/contact/{$}", contactView(t)).Get("/contact/")
	djangotest.AssertStatus(t, resp, http.StatusOK)
	djangotest.AssertSequenceEqual(t, contextForm(t, resp).FieldNames(), []string{"name", "message"})
	djangotest.AssertEqual(t, contextForm(t, resp).IsBound(), false)
	djangotest.AssertTemplateUsed(t, resp, "generic_views/form.html")
}

// Ported from Django: tests/generic_views/test_edit.py::BasicFormTests.test_post_data
func TestBasicFormPostData(t *testing.T) {
	djangotest.EachDB(t, genericModels(), func(t *testing.T, db *orm.DB) {
		c := mount(t,
			"/contact/{$}", contactView(t),
			"/list/authors/{$}", authorList(t, db),
		)
		resp := c.Post("/contact/", map[string]string{"name": "Me", "message": "Hello"})
		djangotest.AssertRedirects(t, resp, "/list/authors/")
	})
}

// Ported from Django: tests/generic_views/test_edit.py::BasicFormTests.test_late_form_validation
//
// LateValidationView overrides form_valid to add an error and return
// form_invalid. The Go view gets the same from a FormValid hook that adds the
// error.
func TestBasicFormLateValidation(t *testing.T) {
	view := contactView(t)
	view.FormValid = func(_ *http.Request, form *forms.Form) error {
		form.AddError("", "There is an error")
		return nil
	}
	resp := mount(t, "/late-validation/{$}", view).
		Post("/late-validation/", map[string]string{"name": "Me", "message": "Hello"})
	djangotest.AssertStatus(t, resp, http.StatusOK)
	djangotest.AssertEqual(t, contextForm(t, resp).IsValid(), false)
	djangotest.AssertSequenceEqual(t, contextForm(t, resp).NonFieldErrors().Messages(), []string{"There is an error"})
}

// TestFormViewInvalidData asserts that data with errors renders the form again
// with status 200 and its errors, without the FormValid hook.
func TestFormViewInvalidData(t *testing.T) {
	called := false
	view := contactView(t)
	view.FormValid = func(*http.Request, *forms.Form) error {
		called = true
		return nil
	}
	resp := mount(t, "/contact/{$}", view).Post("/contact/", map[string]string{"name": "Me"})
	djangotest.AssertStatus(t, resp, http.StatusOK)
	djangotest.AssertEqual(t, called, false)
	form := contextForm(t, resp)
	djangotest.AssertEqual(t, form.IsBound(), true)
	djangotest.AssertEqual(t, form.Errors().Messages(), map[string][]string{"message": {"This field is required."}})
	djangotest.AssertContains(t, resp, "This field is required.")
}

// TestFormViewPut asserts that a PUT is processed like a POST. Django names it
// the browser compatible form of a POST, but reads no form data from the body
// of a PUT; the Go view reads the body.
func TestFormViewPut(t *testing.T) {
	body := url.Values{"name": {"Me"}, "message": {"Hello"}}
	resp := mount(t, "/contact/{$}", contactView(t)).
		Put("/contact/", body, djangotest.ContentType(djangotest.FormContentType))
	djangotest.AssertStatus(t, resp, http.StatusFound)
	djangotest.AssertEqual(t, resp.URL(), "/list/authors/")
}

// TestFormViewMethods asserts the methods a form view answers: GET, POST, PUT,
// HEAD and OPTIONS, as Django's ProcessFormView.
func TestFormViewMethods(t *testing.T) {
	resp := mount(t, "/contact/{$}", contactView(t)).Delete("/contact/", nil)
	djangotest.AssertStatus(t, resp, http.StatusMethodNotAllowed)
	djangotest.AssertEqual(t, resp.Header.Get("Allow"), "GET, POST, PUT, HEAD, OPTIONS")
}

// TestFormViewConfiguration asserts the configuration errors of a form view:
// no Form, and no success URL. A view without success URL fails before its
// FormValid hook runs.
func TestFormViewConfiguration(t *testing.T) {
	logs := captureLogs(t)
	view := contactView(t)
	view.Form = nil
	resp := mount(t, "/contact/{$}", view).Get("/contact/")
	assertImproperlyConfigured(t, logs, resp, "FormView requires Form")

	logs = captureLogs(t)
	called := false
	view = contactView(t)
	view.SuccessURL = ""
	view.FormValid = func(*http.Request, *forms.Form) error {
		called = true
		return nil
	}
	resp = mount(t, "/contact/{$}", view).Post("/contact/", map[string]string{"name": "Me", "message": "Hello"})
	assertImproperlyConfigured(t, logs, resp, "No URL to redirect to. Provide a SuccessURL.")
	djangotest.AssertEqual(t, called, false)
}

// TestFormViewHooks asserts that GetSuccessURL names the target and that an
// error of FormValid is answered by ServeError, here with a 403.
func TestFormViewHooks(t *testing.T) {
	view := contactView(t)
	view.GetSuccessURL = func(_ *http.Request, form *forms.Form) (string, error) {
		return "/thanks/" + form.CleanedData()["name"].(string) + "/", nil
	}
	data := map[string]string{"name": "Me", "message": "Hello"}
	resp := mount(t, "/contact/{$}", view).Post("/contact/", data)
	djangotest.AssertStatus(t, resp, http.StatusFound)
	djangotest.AssertEqual(t, resp.URL(), "/thanks/Me/")

	view.FormValid = func(*http.Request, *forms.Form) error { return views.PermissionDenied("closed") }
	resp = mount(t, "/contact/{$}", view).Post("/contact/", data)
	djangotest.AssertStatus(t, resp, http.StatusForbidden)
}

// TestFormViewRefusesDisallowedRedirect asserts that a success URL with a
// scheme a redirect may not use is answered with 400.
func TestFormViewRefusesDisallowedRedirect(t *testing.T) {
	view := contactView(t)
	view.SuccessURL = "javascript:alert(1)"
	resp := mount(t, "/contact/{$}", view).Post("/contact/", map[string]string{"name": "Me", "message": "Hello"})
	djangotest.AssertStatus(t, resp, http.StatusBadRequest)
}

// authorNames returns the names of all authors in the order of Django's
// Author.Meta.ordering, Author.objects.values_list("name", flat=True).
func authorNames(t *testing.T, db *orm.DB) []string {
	t.Helper()
	authors := allOf[Author](t, db, "name")
	names := make([]string, len(authors))
	for i, a := range authors {
		names[i] = a.Name
	}
	return names
}

// authorForm is AuthorForm of Django's tests/generic_views/forms.py: a model
// form of name and slug that declares both fields itself.
func authorForm(db *orm.DB) func(*http.Request) (*forms.ModelForm[Author], error) {
	return func(*http.Request) (*forms.ModelForm[Author], error) {
		return forms.NewModelForm[Author](db,
			forms.WithFields("name", "slug"),
			forms.WithFormField("name", forms.NewCharField()),
			forms.WithFormField("slug", forms.NewSlugField()),
		), nil
	}
}

// assertAuthorForm asserts that the form of the context is an AuthorForm,
// Django's assertIsInstance(res.context["form"], views.AuthorForm): its slug
// field is the declared SlugField.
func assertAuthorForm(t *testing.T, resp *djangotest.Response) {
	t.Helper()
	field, _ := contextForm(t, resp).FieldDef("slug")
	if _, ok := field.(*forms.SlugField); !ok {
		t.Errorf("the slug field of the form is %T, want *forms.SlugField", field)
	}
}

// authorCreate is AuthorCreate of Django's tests/generic_views/views.py.
func authorCreate(t *testing.T, db *orm.DB) views.CreateView[Author] {
	t.Helper()
	return views.CreateView[Author]{
		DB: db, Engine: genericEngine(t), SuccessURL: "/list/authors/", Fields: []string{forms.AllFields},
	}
}

// naiveAuthorCreate is NaiveAuthorCreate of Django's
// tests/generic_views/views.py.
func naiveAuthorCreate(t *testing.T, db *orm.DB, successURL string) views.CreateView[Author] {
	t.Helper()
	return views.CreateView[Author]{
		DB: db, Engine: genericEngine(t), SuccessURL: successURL, Fields: []string{forms.AllFields},
	}
}

// mountEdit mounts routes next to the views the redirects of the edit views
// point to, so that AssertRedirects can load the target. A route of routes
// replaces the default view of the same pattern. The patterns with "édit"
// stand for the patterns "^[eé]dit/..." of Django's urls.py.
func mountEdit(t *testing.T, db *orm.DB, routes ...any) *djangotest.Client {
	t.Helper()
	artistDetail := views.DetailView[Artist]{DB: db, Engine: genericEngine(t)}
	defaults := []any{
		"/list/authors/{$}", authorList(t, db),
		"/edit/authors/create/{$}", authorCreate(t, db),
		"/édit/authors/create/{$}", authorCreate(t, db),
		"/edit/author/{pk}/update/{$}", authorUpdate(t, db, nil),
		"/édit/author/{pk}/update/{$}", authorUpdate(t, db, nil),
		"/detail/author/{pk}/{$}", authorDetail(t, db),
		"/detail/artist/{pk}/{$}", artistDetail,
	}
	taken := map[any]bool{}
	for i := 0; i < len(routes); i += 2 {
		taken[routes[i]] = true
	}
	for i := 0; i+1 < len(defaults); i += 2 {
		if !taken[defaults[i]] {
			routes = append(routes, defaults[i], defaults[i+1])
		}
	}
	return mount(t, routes...)
}

// Ported from Django: tests/generic_views/test_edit.py::CreateViewTests.test_create
func TestCreateView(t *testing.T) {
	djangotest.EachDB(t, genericModels(), func(t *testing.T, db *orm.DB) {
		c := mountEdit(t, db)
		resp := c.Get("/edit/authors/create/")
		djangotest.AssertStatus(t, resp, http.StatusOK)
		djangotest.AssertSequenceEqual(t, contextForm(t, resp).FieldNames(), []string{"name", "slug"})
		if _, ok := contextValue(t, resp, "view").(views.CreateView[Author]); !ok {
			t.Errorf("context[\"view\"] is %T, want views.CreateView[Author]", contextValue(t, resp, "view"))
		}
		assertNotInContext(t, resp, "object")
		assertNotInContext(t, resp, "author")
		djangotest.AssertTemplateUsed(t, resp, "generic_views/author_form.html")

		resp = c.Post("/edit/authors/create/", map[string]string{"name": "Randall Munroe", "slug": "randall-munroe"})
		djangotest.AssertStatus(t, resp, http.StatusFound)
		djangotest.AssertRedirects(t, resp, "/list/authors/")
		djangotest.AssertSequenceEqual(t, authorNames(t, db), []string{"Randall Munroe"})
	})
}

// Ported from Django: tests/generic_views/test_edit.py::CreateViewTests.test_create_invalid
func TestCreateViewInvalid(t *testing.T) {
	djangotest.EachDB(t, genericModels(), func(t *testing.T, db *orm.DB) {
		resp := mountEdit(t, db).Post("/edit/authors/create/",
			map[string]string{"name": strings.Repeat("A", 101), "slug": "randall-munroe"})
		djangotest.AssertStatus(t, resp, http.StatusOK)
		djangotest.AssertTemplateUsed(t, resp, "generic_views/author_form.html")
		djangotest.AssertEqual(t, contextForm(t, resp).Errors().Len(), 1)
		assertAuthorCount(t, db, 0)
	})
}

// Ported from Django: tests/generic_views/test_edit.py::CreateViewTests.test_create_with_object_url
func TestCreateViewWithObjectURL(t *testing.T) {
	djangotest.EachDB(t, genericModels(), func(t *testing.T, db *orm.DB) {
		view := views.CreateView[Artist]{DB: db, Engine: genericEngine(t), Fields: []string{forms.AllFields}}
		c := mountEdit(t, db, "/edit/artists/create/{$}", view)
		resp := c.Post("/edit/artists/create/", map[string]string{"name": "Rene Magritte"})
		djangotest.AssertStatus(t, resp, http.StatusFound)
		artist, err := orm.Query[Artist](db).Filter("name", "Rene Magritte").Get(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		djangotest.AssertRedirects(t, resp, artist.GetAbsoluteURL())
		djangotest.AssertSequenceEqual(t, allOf[Artist](t, db, "id"), []Artist{artist})
	})
}

// Ported from Django: tests/generic_views/test_edit.py::CreateViewTests.test_create_with_redirect
func TestCreateViewWithRedirect(t *testing.T) {
	djangotest.EachDB(t, genericModels(), func(t *testing.T, db *orm.DB) {
		c := mountEdit(t, db,
			"/edit/authors/create/redirect/{$}", naiveAuthorCreate(t, db, "/edit/authors/create/"))
		resp := c.Post("/edit/authors/create/redirect/",
			map[string]string{"name": "Randall Munroe", "slug": "randall-munroe"})
		djangotest.AssertStatus(t, resp, http.StatusFound)
		djangotest.AssertRedirects(t, resp, "/edit/authors/create/")
		djangotest.AssertSequenceEqual(t, authorNames(t, db), []string{"Randall Munroe"})
	})
}

// Ported from Django: tests/generic_views/test_edit.py::CreateViewTests.test_create_with_interpolated_redirect
func TestCreateViewWithInterpolatedRedirect(t *testing.T) {
	djangotest.EachDB(t, genericModels(), func(t *testing.T, db *orm.DB) {
		c := mountEdit(t, db,
			"/edit/authors/create/interpolate_redirect/{$}",
			naiveAuthorCreate(t, db, "/edit/author/{id}/update/"),
			"/edit/authors/create/interpolate_redirect_nonascii/{$}",
			naiveAuthorCreate(t, db, "/%C3%A9dit/author/{id}/update/"),
		)
		resp := c.Post("/edit/authors/create/interpolate_redirect/",
			map[string]string{"name": "Randall Munroe", "slug": "randall-munroe"})
		djangotest.AssertSequenceEqual(t, authorNames(t, db), []string{"Randall Munroe"})
		djangotest.AssertStatus(t, resp, http.StatusFound)
		pk := allOf[Author](t, db, "name")[0].ID
		djangotest.AssertRedirects(t, resp, fmt.Sprintf("/edit/author/%d/update/", pk))

		// Also test with escaped chars in URL.
		resp = c.Post("/edit/authors/create/interpolate_redirect_nonascii/",
			map[string]string{"name": "John Doe", "slug": "john-doe"})
		djangotest.AssertStatus(t, resp, http.StatusFound)
		john, err := orm.Query[Author](db).Filter("name", "John Doe").Get(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		djangotest.AssertRedirects(t, resp, fmt.Sprintf("/%%C3%%A9dit/author/%d/update/", john.ID))
	})
}

// Ported from Django: tests/generic_views/test_edit.py::CreateViewTests.test_create_with_special_properties
func TestCreateViewWithSpecialProperties(t *testing.T) {
	djangotest.EachDB(t, genericModels(), func(t *testing.T, db *orm.DB) {
		view := views.CreateView[Author]{
			DB:           db,
			Engine:       genericEngine(t),
			Form:         authorForm(db),
			TemplateName: "generic_views/form.html",
			GetSuccessURL: func(_ *http.Request, obj Author) (string, error) {
				return fmt.Sprintf("/detail/author/%d/", obj.ID), nil
			},
		}
		c := mountEdit(t, db, "/edit/authors/create/special/{$}", view)
		resp := c.Get("/edit/authors/create/special/")
		djangotest.AssertStatus(t, resp, http.StatusOK)
		assertAuthorForm(t, resp)
		assertNotInContext(t, resp, "object")
		assertNotInContext(t, resp, "author")
		djangotest.AssertTemplateUsed(t, resp, "generic_views/form.html")

		resp = c.Post("/edit/authors/create/special/",
			map[string]string{"name": "Randall Munroe", "slug": "randall-munroe"})
		djangotest.AssertStatus(t, resp, http.StatusFound)
		obj, err := orm.Query[Author](db).Filter("slug", "randall-munroe").Get(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		djangotest.AssertRedirects(t, resp, fmt.Sprintf("/detail/author/%d/", obj.ID))
		djangotest.AssertSequenceEqual(t, allOf[Author](t, db, "name"), []Author{obj})
	})
}

// Ported from Django: tests/generic_views/test_edit.py::CreateViewTests.test_create_without_redirect
//
// Django's message names success_url and get_absolute_url, the Go message
// names the field SuccessURL and the method GetAbsoluteURL. Django saves the
// object before it finds the mistake; the Go view finds it first and saves
// nothing.
func TestCreateViewWithoutRedirect(t *testing.T) {
	djangotest.EachDB(t, genericModels(), func(t *testing.T, db *orm.DB) {
		logs := captureLogs(t)
		c := mount(t, "/edit/authors/create/naive/{$}", naiveAuthorCreate(t, db, ""))
		resp := c.Post("/edit/authors/create/naive/",
			map[string]string{"name": "Randall Munroe", "slug": "randall-munroe"})
		assertImproperlyConfigured(t, logs, resp,
			"No URL to redirect to. Either provide a SuccessURL or define a GetAbsoluteURL method on the model.")
		assertAuthorCount(t, db, 0)
	})
}

// Ported from Django: tests/generic_views/test_edit.py::CreateViewTests.test_create_restricted
//
// Django decorates the post method with login_required. The Go view is
// wrapped in auth.LoginRequired as a whole. The login page is not part of
// the port, so the redirect is asserted without loading it.
func TestCreateViewRestricted(t *testing.T) {
	djangotest.EachDB(t, genericModels(), func(t *testing.T, db *orm.DB) {
		view := authorCreate(t, db)
		c := mount(t, "/edit/authors/create/restricted/{$}", auth.LoginRequired("/accounts/login/")(view))
		resp := c.Post("/edit/authors/create/restricted/",
			map[string]string{"name": "Randall Munroe", "slug": "randall-munroe"})
		djangotest.AssertStatus(t, resp, http.StatusFound)
		djangotest.AssertRedirects(t, resp, "/accounts/login/?next=/edit/authors/create/restricted/",
			djangotest.FetchRedirectResponse(false))
		assertAuthorCount(t, db, 0)
	})
}

// Ported from Django: tests/generic_views/test_edit.py::CreateViewTests.test_create_view_with_restricted_fields
//
// Django reads the fields of get_form_class(); the Go test reads the fields of
// the form in the context.
func TestCreateViewWithRestrictedFields(t *testing.T) {
	djangotest.EachDB(t, genericModels(), func(t *testing.T, db *orm.DB) {
		view := views.CreateView[Author]{DB: db, Engine: genericEngine(t), Fields: []string{"name"}}
		resp := mount(t, "/create/{$}", view).Get("/create/")
		djangotest.AssertSequenceEqual(t, contextForm(t, resp).FieldNames(), []string{"name"})
	})
}

// Ported from Django: tests/generic_views/test_edit.py::CreateViewTests.test_create_view_all_fields
func TestCreateViewAllFields(t *testing.T) {
	djangotest.EachDB(t, genericModels(), func(t *testing.T, db *orm.DB) {
		view := views.CreateView[Author]{DB: db, Engine: genericEngine(t), Fields: []string{forms.AllFields}}
		resp := mount(t, "/create/{$}", view).Get("/create/")
		djangotest.AssertSequenceEqual(t, contextForm(t, resp).FieldNames(), []string{"name", "slug"})
	})
}

// Ported from Django: tests/generic_views/test_edit.py::CreateViewTests.test_create_view_without_explicit_fields
//
// Django's message names ModelFormMixin and the subclass, the Go message the
// view type.
func TestCreateViewWithoutExplicitFields(t *testing.T) {
	djangotest.EachDB(t, genericModels(), func(t *testing.T, db *orm.DB) {
		logs := captureLogs(t)
		resp := mount(t, "/create/{$}", views.CreateView[Author]{DB: db, Engine: genericEngine(t)}).Get("/create/")
		assertImproperlyConfigured(t, logs, resp, "Using CreateView without the Fields attribute is prohibited.")
	})
}

// Ported from Django: tests/generic_views/test_edit.py::CreateViewTests.test_define_both_fields_and_form_class
func TestCreateViewDefineBothFieldsAndForm(t *testing.T) {
	djangotest.EachDB(t, genericModels(), func(t *testing.T, db *orm.DB) {
		logs := captureLogs(t)
		view := views.CreateView[Author]{
			DB: db, Engine: genericEngine(t), Form: authorForm(db), Fields: []string{"name"},
		}
		resp := mount(t, "/create/{$}", view).Get("/create/")
		assertImproperlyConfigured(t, logs, resp, "Specifying both Fields and Form is not permitted")
	})
}

// TestCreateViewFormValid asserts that the FormValid hook can set a field of
// the new object that is not in the form, and that an error it adds to the
// form renders the form again and saves nothing.
func TestCreateViewFormValid(t *testing.T) {
	djangotest.EachDB(t, genericModels(), func(t *testing.T, db *orm.DB) {
		view := authorCreate(t, db)
		view.Fields = []string{"name"}
		view.FormValid = func(_ *http.Request, form *forms.ModelForm[Author]) error {
			form.Instance().Slug = strings.ToLower(strings.ReplaceAll(form.Instance().Name, " ", "-"))
			return nil
		}
		c := mountEdit(t, db, "/create/{$}", view)
		resp := c.Post("/create/", map[string]string{"name": "Randall Munroe"})
		djangotest.AssertRedirects(t, resp, "/list/authors/")
		authors := allOf[Author](t, db, "name")
		djangotest.AssertEqual(t, len(authors), 1)
		djangotest.AssertEqual(t, authors[0].Slug, "randall-munroe")

		view.FormValid = func(_ *http.Request, form *forms.ModelForm[Author]) error {
			form.AddError("name", "Taken.")
			return nil
		}
		resp = mount(t, "/create/{$}", view).Post("/create/", map[string]string{"name": "Someone Else"})
		djangotest.AssertStatus(t, resp, http.StatusOK)
		djangotest.AssertEqual(t, contextForm(t, resp).Errors().Messages(), map[string][]string{"name": {"Taken."}})
		assertAuthorCount(t, db, 1)
	})
}

// TestCreateViewInitialAndPrefix asserts that Initial and Prefix reach the
// form.
func TestCreateViewInitialAndPrefix(t *testing.T) {
	djangotest.EachDB(t, genericModels(), func(t *testing.T, db *orm.DB) {
		view := authorCreate(t, db)
		view.Initial = map[string]any{"name": "Anonymous"}
		view.Prefix = "author"
		resp := mount(t, "/create/{$}", view).Get("/create/")
		djangotest.AssertContains(t, resp, `name="author-name" value="Anonymous"`)

		resp = mountEdit(t, db, "/create/{$}", view).Post("/create/",
			map[string]string{"author-name": "Randall Munroe", "author-slug": "randall-munroe"})
		djangotest.AssertRedirects(t, resp, "/list/authors/")
		djangotest.AssertSequenceEqual(t, authorNames(t, db), []string{"Randall Munroe"})
	})
}
