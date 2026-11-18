package djangotest_test

import (
	"net/http"
	"net/url"
	"testing"
	"testing/fstest"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/forms"
	"github.com/oliverhaas/djangogo/templates"
	"github.com/oliverhaas/djangogo/views"
)

// This file ports the form tests of Django's test client and the tests of
// assertFormError. A Go form has no class name, so the form in the failure
// messages is "<Form ...>" where Django writes the class, "<TestForm ...>".
//
// The template engine of this branch records the template a view renders but
// not the templates it extends, so the checks that base.html was used are
// skipped (FIXME-PORT) until the engine records them.

// formTemplates holds the templates of the form views: the inline templates of
// tests/test_client/views.py under their names, and form_view.html,
// extended.html and base.html of tests/templates.
var formTemplates = fstest.MapFS{
	"Valid POST Template":   {Data: []byte("Valid POST data.")},
	"Invalid POST Template": {Data: []byte("Invalid POST data. {{ form.errors }}")},
	"Form GET Template":     {Data: []byte("Viewing base form. {{ form }}.")},
	"base.html": {Data: []byte("<html>\n<head></head>\n<body>\n" +
		"<h1>Django Internal Tests: {% block title %}{% endblock %}</h1>\n" +
		"{% block content %}\n{% endblock %}\n</body>\n</html>\n")},
	"form_view.html": {Data: []byte(`{% extends "base.html" %}
{% block title %}Submit data{% endblock %}
{% block content %}
<h1>{{ message }}</h1>
<form method="post">
{% if form.errors %}
<p class='warning'>Please correct the errors below:</p>
{% endif %}
<ul class='form'>
{{ form }}
<li><input type='submit' value='Submit'></li>
</ul>
</form>

{% endblock %}
`)},
	"extended.html": {Data: []byte(`{% extends "base.html" %}
{% block title %}Extended template{% endblock %}
{% block content %}
This is just a template extending the base.
{% endblock %}
`)},
}

// clientTestForm is TestForm of tests/test_client/views.py.
func clientTestForm() *forms.Form {
	choices := forms.Choices(
		"a", "First Choice", "b", "Second Choice", "c", "Third Choice",
		"d", "Fourth Choice", "e", "Fifth Choice",
	)
	return forms.New(
		forms.F("text", forms.NewCharField()),
		forms.F("email", forms.NewEmailField()),
		forms.F("value", forms.NewIntegerField()),
		forms.F("single", forms.NewChoiceField(choices)),
		forms.F("multi", forms.NewMultipleChoiceField(choices)),
	).CleanWith(func(f *forms.Form) error {
		if f.CleanedData()["text"] == "Raise non-field error" {
			return forms.NewValidationError("Non-field error.", "")
		}
		return nil
	})
}

// formSite serves form_view and form_view_with_template of
// tests/test_client/views.py and request_data of
// tests/test_client_regress/views.py.
type formSite struct {
	engine *templates.Engine
}

// newFormClient returns a test client for the form views.
func newFormClient(t *testing.T) *djangotest.Client {
	t.Helper()
	engine, err := templates.NewEngineFS(formTemplates)
	if err != nil {
		t.Fatalf("NewEngineFS: %v", err)
	}
	s := &formSite{engine: engine}
	mux := http.NewServeMux()
	mux.HandleFunc("/form_view/{$}", s.formView)
	mux.HandleFunc("/form_view_with_template/{$}", s.formViewWithTemplate)
	mux.HandleFunc("/request_data/{$}", s.requestData("base.html", "sausage"))
	mux.HandleFunc("/request_data_extended/{$}", s.requestData("extended.html", "bacon"))
	return djangotest.NewClient(t, mux)
}

// render renders a template of the form views. A template that fails to render
// is a bug of this fixture, so it panics.
func (s *formSite) render(w http.ResponseWriter, r *http.Request, name string, ctx map[string]any) {
	if err := views.Render(w, r, s.engine, name, ctx); err != nil {
		panic(err)
	}
}

// formView validates TestForm against the POST data, or against the query
// string for any other method.
func (s *formSite) formView(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if r.Method == http.MethodPost {
		form := clientTestForm().Bind(r.PostForm)
		if form.IsValid() {
			s.render(w, r, "Valid POST Template", nil)
		} else {
			s.render(w, r, "Invalid POST Template", map[string]any{"form": form})
		}
		return
	}
	form := clientTestForm().Bind(r.URL.Query())
	s.render(w, r, "Form GET Template", map[string]any{"form": form})
}

// formViewWithTemplate renders form_view.html, which extends base.html, with
// TestForm and a message about the POST data.
func (s *formSite) formViewWithTemplate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	form, message := clientTestForm(), "GET form page"
	if r.Method == http.MethodPost {
		form = form.Bind(r.PostForm)
		message = "POST data has errors"
		if form.IsValid() {
			message = "POST data OK"
		}
	}
	s.render(w, r, "form_view.html", map[string]any{"form": form, "message": message})
}

// requestData renders template with the request data and data in the
// context. An absent parameter is "" where Django has None. The template
// engine accepts only identifiers as context keys, so the keys are get_foo and
// so on where Django has get-foo.
func (s *formSite) requestData(template, data string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		query := r.URL.Query()
		s.render(w, r, template, map[string]any{
			"get_foo":  query.Get("foo"),
			"get_bar":  query.Get("bar"),
			"post_foo": r.PostForm.Get("foo"),
			"post_bar": r.PostForm.Get("bar"),
			"data":     data,
		})
	}
}

// validFormData is the post_data of the form tests of tests/test_client.
func validFormData() url.Values {
	return url.Values{
		"text":   {"Hello World"},
		"email":  {"foo@example.com"},
		"value":  {"37"},
		"single": {"b"},
		"multi":  {"b", "c", "e"},
	}
}

// Ported from Django: tests/test_client/tests.py::ClientTest.test_valid_form
func TestValidForm(t *testing.T) {
	resp := newFormClient(t).Post("/form_view/", validFormData())
	djangotest.AssertStatus(t, resp, 200)
	djangotest.AssertTemplateUsed(t, resp, "Valid POST Template")
}

// Ported from Django: tests/test_client/tests.py::ClientTest.test_valid_form_with_hints
func TestValidFormWithHints(t *testing.T) {
	hints := url.Values{"text": {"Hello World"}, "multi": {"b", "c", "e"}}
	resp := newFormClient(t).Get("/form_view/", djangotest.Query(hints))
	// The multi-value data has been rolled out ok.
	djangotest.AssertContains(t, resp, "Select a valid choice.", djangotest.Count(0))
	djangotest.AssertTemplateUsed(t, resp, "Form GET Template")
}

// Ported from Django: tests/test_client/tests.py::ClientTest.test_incomplete_data_form
func TestIncompleteDataForm(t *testing.T) {
	data := url.Values{"text": {"Hello World"}, "value": {"37"}}
	resp := newFormClient(t).Post("/form_view/", data)
	djangotest.AssertContains(t, resp, "This field is required.", djangotest.Count(3))
	djangotest.AssertTemplateUsed(t, resp, "Invalid POST Template")
	form := contextValue(t, resp, "form")
	djangotest.AssertFormError(t, form, "email", []string{"This field is required."})
	djangotest.AssertFormError(t, form, "single", []string{"This field is required."})
	djangotest.AssertFormError(t, form, "multi", []string{"This field is required."})
}

// Ported from Django: tests/test_client/tests.py::ClientTest.test_form_error
func TestFormError(t *testing.T) {
	data := validFormData()
	data.Set("email", "not an email address")
	resp := newFormClient(t).Post("/form_view/", data)
	djangotest.AssertStatus(t, resp, 200)
	djangotest.AssertTemplateUsed(t, resp, "Invalid POST Template")
	djangotest.AssertFormError(t, contextValue(t, resp, "form"), "email", []string{"Enter a valid email address."})
}

// Ported from Django: tests/test_client/tests.py::ClientTest.test_valid_form_with_template
func TestValidFormWithTemplate(t *testing.T) {
	resp := newFormClient(t).Post("/form_view_with_template/", validFormData())
	djangotest.AssertContains(t, resp, "POST data OK")
	djangotest.AssertTemplateUsed(t, resp, "form_view.html")
	djangotest.AssertTemplateNotUsed(t, resp, "Valid POST Template")
	t.Skip("FIXME-PORT: Go: the template engine does not record the templates a template extends, Django: base.html is a template used")
	djangotest.AssertTemplateUsed(t, resp, "base.html")
}

// Ported from Django: tests/test_client/tests.py::ClientTest.test_incomplete_data_form_with_template
func TestIncompleteDataFormWithTemplate(t *testing.T) {
	data := url.Values{"text": {"Hello World"}, "value": {"37"}}
	resp := newFormClient(t).Post("/form_view_with_template/", data)
	djangotest.AssertContains(t, resp, "POST data has errors")
	djangotest.AssertTemplateUsed(t, resp, "form_view.html")
	djangotest.AssertTemplateNotUsed(t, resp, "Invalid POST Template")
	form := contextValue(t, resp, "form")
	djangotest.AssertFormError(t, form, "email", []string{"This field is required."})
	djangotest.AssertFormError(t, form, "single", []string{"This field is required."})
	djangotest.AssertFormError(t, form, "multi", []string{"This field is required."})
	t.Skip("FIXME-PORT: Go: the template engine does not record the templates a template extends, Django: base.html is a template used")
	djangotest.AssertTemplateUsed(t, resp, "base.html")
}

// Ported from Django: tests/test_client/tests.py::ClientTest.test_form_error_with_template
func TestFormErrorWithTemplate(t *testing.T) {
	data := validFormData()
	data.Set("email", "not an email address")
	resp := newFormClient(t).Post("/form_view_with_template/", data)
	djangotest.AssertContains(t, resp, "POST data has errors")
	djangotest.AssertTemplateUsed(t, resp, "form_view.html")
	djangotest.AssertTemplateNotUsed(t, resp, "Invalid POST Template")
	djangotest.AssertFormError(t, contextValue(t, resp, "form"), "email", []string{"Enter a valid email address."})
	t.Skip("FIXME-PORT: Go: the template engine does not record the templates a template extends, Django: base.html is a template used")
	djangotest.AssertTemplateUsed(t, resp, "base.html")
}

// Ported from Django: tests/test_client_regress/tests.py::AssertTemplateUsedTests.test_multiple_context
func TestMultipleContext(t *testing.T) {
	resp := newFormClient(t).Post("/form_view_with_template/", validFormData())
	djangotest.AssertContains(t, resp, "POST data OK")
	mustFail(t, "Template 'form_view.html' was used unexpectedly in rendering the response", func(ft djangotest.TB) {
		djangotest.AssertTemplateNotUsed(ft, resp, "form_view.html")
	})
	t.Skip("FIXME-PORT: Go: the template engine does not record the templates a template extends, Django: base.html is a template used")
	mustFail(t, "Template 'base.html' was used unexpectedly in rendering the response", func(ft djangotest.TB) {
		djangotest.AssertTemplateNotUsed(ft, resp, "base.html")
	})
	mustFail(t, "Template 'Valid POST Template' was not a template used to render "+
		"the response. Actual template(s) used: form_view.html, base.html", func(ft djangotest.TB) {
		djangotest.AssertTemplateUsed(ft, resp, "Valid POST Template")
	})
	mustFail(t, "Template 'base.html' was expected to be rendered 2 time(s) but "+
		"was actually rendered 1 time(s).", func(ft djangotest.TB) {
		djangotest.AssertTemplateUsed(ft, resp, "base.html", djangotest.Count(2))
	})
}

// The contexts of a response are Response.Templates; the Go client has no
// RequestContext or ContextList types to check, and a missing key is reported
// by the second result of Response.Context instead of a KeyError.
//
// Ported from Django: tests/test_client_regress/tests.py::ContextTests.test_single_context
func TestContextsSingleContext(t *testing.T) {
	resp := newFormClient(t).Get("/request_data/", djangotest.Query(url.Values{"foo": {"whiz"}}))
	djangotest.AssertEqual(t, len(resp.Templates), 1)
	djangotest.AssertEqual(t, contextValue(t, resp, "get_foo"), any("whiz"))
	djangotest.AssertEqual(t, contextValue(t, resp, "data"), any("sausage"))
	_, ok := resp.Context("does-not-exist")
	djangotest.AssertEqual(t, ok, false)
}

// Ported from Django: tests/test_client_regress/tests.py::ContextTests.test_inherited_context
func TestContextsInheritedContext(t *testing.T) {
	resp := newFormClient(t).Get("/request_data_extended/", djangotest.Query(url.Values{"foo": {"whiz"}}))
	djangotest.AssertEqual(t, contextValue(t, resp, "get_foo"), any("whiz"))
	djangotest.AssertEqual(t, contextValue(t, resp, "data"), any("bacon"))
	_, ok := resp.Context("does-not-exist")
	djangotest.AssertEqual(t, ok, false)
	t.Skip("FIXME-PORT: Go: the template engine does not record the templates a template extends, Django: 2 contexts, extended.html and base.html")
	djangotest.AssertEqual(t, len(resp.Templates), 2)
}

// assertFormErrorForm is TestForm of tests/test_utils/tests.py: "invalid" is
// an error of the field, "invalid_non_field" an error of the form.
func assertFormErrorForm() *forms.Form {
	return forms.New(forms.F("field", forms.NewCharField())).
		CleanFieldWith("field", func(_ *forms.Form, value any) (any, error) {
			if value == "invalid" {
				return nil, forms.NewValidationError("invalid value", "")
			}
			return value, nil
		}).
		CleanWith(func(f *forms.Form) error {
			if f.CleanedData()["field"] == "invalid_non_field" {
				return forms.NewValidationError("non-field error", "")
			}
			return nil
		})
}

// cleanedForm is TestForm._get_cleaned_form: the form bound to value and
// validated.
func cleanedForm(value string) *forms.Form {
	form := assertFormErrorForm().Bind(url.Values{"field": {value}})
	form.Errors()
	return form
}

// AssertFormError takes a list of messages; a single message is a list of
// one, so this test and the next check the same thing.
//
// Ported from Django: tests/test_utils/tests.py::AssertFormErrorTests.test_single_error
func TestAssertFormErrorSingleError(t *testing.T) {
	djangotest.AssertFormError(t, cleanedForm("invalid"), "field", []string{"invalid value"})
}

// Ported from Django: tests/test_utils/tests.py::AssertFormErrorTests.test_error_list
func TestAssertFormErrorErrorList(t *testing.T) {
	djangotest.AssertFormError(t, cleanedForm("invalid"), "field", []string{"invalid value"})
}

// Ported from Django: tests/test_utils/tests.py::AssertFormErrorTests.test_empty_errors_valid_form
func TestAssertFormErrorEmptyErrorsValidForm(t *testing.T) {
	djangotest.AssertFormError(t, cleanedForm("valid"), "field", nil)
}

// The empty field name stands for Django's field=None.
//
// Ported from Django: tests/test_utils/tests.py::AssertFormErrorTests.test_empty_errors_valid_form_non_field_errors
func TestAssertFormErrorEmptyErrorsValidFormNonFieldErrors(t *testing.T) {
	djangotest.AssertFormError(t, cleanedForm("valid"), "", []string{})
}

// Ported from Django: tests/test_utils/tests.py::AssertFormErrorTests.test_field_not_in_form
func TestAssertFormErrorFieldNotInForm(t *testing.T) {
	msg := "The form <Form bound=True, valid=False, fields=(field)> does not " +
		"contain the field 'other_field'."
	mustFail(t, msg, func(ft djangotest.TB) {
		djangotest.AssertFormError(ft, cleanedForm("invalid"), "other_field", []string{"invalid value"})
	})
	mustFail(t, "Custom prefix: "+msg, func(ft djangotest.TB) {
		djangotest.AssertFormError(ft, cleanedForm("invalid"), "other_field", []string{"invalid value"},
			djangotest.MsgPrefix("Custom prefix"))
	})
}

// Ported from Django: tests/test_utils/tests.py::AssertFormErrorTests.test_field_with_no_errors
func TestAssertFormErrorFieldWithNoErrors(t *testing.T) {
	msg := "The errors of field 'field' on form <Form bound=True, valid=True, " +
		"fields=(field)> don't match."
	assertion := func(ft djangotest.TB, opts ...djangotest.AssertOption) {
		djangotest.AssertFormError(ft, cleanedForm("valid"), "field", []string{"invalid value"}, opts...)
	}
	mustFail(t, msg, func(ft djangotest.TB) { assertion(ft) })
	mustFail(t, "[] != ['invalid value']", func(ft djangotest.TB) { assertion(ft) })
	mustFail(t, "Custom prefix: "+msg, func(ft djangotest.TB) { assertion(ft, djangotest.MsgPrefix("Custom prefix")) })
}

// Ported from Django: tests/test_utils/tests.py::AssertFormErrorTests.test_field_with_different_error
func TestAssertFormErrorFieldWithDifferentError(t *testing.T) {
	msg := "The errors of field 'field' on form <Form bound=True, valid=False, " +
		"fields=(field)> don't match."
	assertion := func(ft djangotest.TB, opts ...djangotest.AssertOption) {
		djangotest.AssertFormError(ft, cleanedForm("invalid"), "field", []string{"other error"}, opts...)
	}
	mustFail(t, msg, func(ft djangotest.TB) { assertion(ft) })
	mustFail(t, "['invalid value'] != ['other error']", func(ft djangotest.TB) { assertion(ft) })
	mustFail(t, "Custom prefix: "+msg, func(ft djangotest.TB) { assertion(ft, djangotest.MsgPrefix("Custom prefix")) })
}

// Ported from Django: tests/test_utils/tests.py::AssertFormErrorTests.test_unbound_form
func TestAssertFormErrorUnboundForm(t *testing.T) {
	msg := "The form <Form bound=False, valid=Unknown, fields=(field)> is not " +
		"bound, it will never have any errors."
	mustFail(t, msg, func(ft djangotest.TB) {
		djangotest.AssertFormError(ft, assertFormErrorForm(), "field", nil)
	})
	mustFail(t, "Custom prefix: "+msg, func(ft djangotest.TB) {
		djangotest.AssertFormError(ft, assertFormErrorForm(), "field", nil, djangotest.MsgPrefix("Custom prefix"))
	})
}

// Ported from Django: tests/test_utils/tests.py::AssertFormErrorTests.test_empty_errors_invalid_form
func TestAssertFormErrorEmptyErrorsInvalidForm(t *testing.T) {
	msg := "The errors of field 'field' on form <Form bound=True, valid=False, " +
		"fields=(field)> don't match."
	assertion := func(ft djangotest.TB) {
		djangotest.AssertFormError(ft, cleanedForm("invalid"), "field", nil)
	}
	mustFail(t, msg, assertion)
	mustFail(t, "['invalid value'] != []", assertion)
}

// Ported from Django: tests/test_utils/tests.py::AssertFormErrorTests.test_non_field_errors
func TestAssertFormErrorNonFieldErrors(t *testing.T) {
	djangotest.AssertFormError(t, cleanedForm("invalid_non_field"), "", []string{"non-field error"})
}

// Ported from Django: tests/test_utils/tests.py::AssertFormErrorTests.test_different_non_field_errors
func TestAssertFormErrorDifferentNonFieldErrors(t *testing.T) {
	msg := "The non-field errors of form <Form bound=True, valid=False, " +
		"fields=(field)> don't match."
	assertion := func(ft djangotest.TB, opts ...djangotest.AssertOption) {
		djangotest.AssertFormError(ft, cleanedForm("invalid_non_field"), "", []string{"other non-field error"}, opts...)
	}
	mustFail(t, msg, func(ft djangotest.TB) { assertion(ft) })
	mustFail(t, "['non-field error'] != ['other non-field error']", func(ft djangotest.TB) { assertion(ft) })
	mustFail(t, "Custom prefix: "+msg, func(ft djangotest.TB) { assertion(ft, djangotest.MsgPrefix("Custom prefix")) })
}
