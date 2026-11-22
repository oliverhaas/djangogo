package views

import (
	"errors"
	"maps"
	"net/http"
	"net/url"

	"github.com/oliverhaas/djangogo/forms"
	"github.com/oliverhaas/djangogo/messages"
	"github.com/oliverhaas/djangogo/templates"
	"github.com/oliverhaas/djangogo/validators"
)

// maxFormMemory is the part of a multipart body that is held in memory while
// it is parsed, the default of net/http.
const maxFormMemory = 32 << 20

// formData returns the submitted data of r, the data that Django's
// FormMixin.get_form_kwargs passes to the form: the url-encoded or multipart
// body of a POST or PUT request. It reports false for any other method. A body
// that cannot be parsed is a bad request. Unlike Django, which reads no form
// data from the body of a PUT, the body of a PUT is read like that of a POST.
func formData(r *http.Request) (url.Values, bool, error) {
	if r.Method != http.MethodPost && r.Method != http.MethodPut {
		return nil, false, nil
	}
	err := r.ParseMultipartForm(maxFormMemory)
	if err != nil && !errors.Is(err, http.ErrNotMultipart) {
		return nil, false, BadRequest("the form data cannot be parsed: " + err.Error())
	}
	return r.PostForm, true, nil
}

// prepareForm applies the initial values and the prefix of a view to form and
// gives it the context of the request, Django's FormMixin.get_form_kwargs.
// initial is merged over the initial values of the form into a new map, so the
// map of the view is never shared with a form (Django's get_initial returns a
// copy for the same reason).
func prepareForm(form *forms.Form, r *http.Request, initial map[string]any, prefix string) {
	if len(initial) > 0 {
		merged := make(map[string]any, len(form.Initial)+len(initial))
		maps.Copy(merged, form.Initial)
		maps.Copy(merged, initial)
		form.Initial = merged
	}
	if prefix != "" {
		form.Prefix = prefix
	}
	form.SetContext(r.Context())
}

// formValid validates a bound form and runs the FormValid hook of a view on
// valid data. It is Django's split into form_valid and form_invalid, where
// form_valid can still turn to form_invalid: it reports false when the form
// has errors before or after the hook. ready runs between the validation and
// the hook, so that a configuration mistake shows before the hook has side
// effects. An error of the form (Form.Err), of ready or of the hook is
// returned.
func formValid(form *forms.Form, ready, hook func() error) (bool, error) {
	if !form.IsValid() {
		return false, form.Err()
	}
	if err := ready(); err != nil {
		return false, err
	}
	if hook == nil {
		return true, nil
	}
	if err := hook(); err != nil {
		return false, err
	}
	return form.IsValid(), form.Err()
}

// redirectFound answers with a 302 redirect to target after Django's redirect
// checks. addMessage runs between the checks and the redirect, while the
// message middleware can still store a message.
func redirectFound(w http.ResponseWriter, target string, addMessage func() error) error {
	location, err := checkedLocation(target, 0)
	if err != nil {
		return err
	}
	if err := addMessage(); err != nil {
		return err
	}
	writeRedirect(w, http.StatusFound, location)
	return nil
}

// addSuccessMessage adds the success message of a view for r, Django's
// SuccessMessageMixin.form_valid: the result of get when it is not nil, else
// message with its %(name)s placeholders filled from cleanedData. An empty
// message adds nothing. Without messages.Middleware it returns
// messages.ErrMiddlewareMissing, as messages.success raises MessageFailure.
func addSuccessMessage(r *http.Request, message string, get func() (string, error), cleanedData map[string]any) error {
	if get != nil {
		var err error
		if message, err = get(); err != nil {
			return err
		}
	} else {
		message = validators.FormatMessage(message, cleanedData)
	}
	if message == "" {
		return nil
	}
	return messages.Success(r, message)
}

// errNoSuccessURL is the configuration error of a form view without a
// success URL.
func errNoSuccessURL() error {
	return improperlyConfigured("No URL to redirect to. Provide a SuccessURL.")
}

// FormView shows a form on GET and HEAD and validates the submitted data on
// POST and PUT, Django's FormView. Valid data leads to a redirect to
// SuccessURL with status 302. Data with errors renders the template again,
// with the bound form and its errors. The context holds the form under
// "form".
//
//	mux.Handle("/contact/", views.FormView{
//		Engine:       engine,
//		TemplateName: "contact.html",
//		Form:         func(*http.Request) (*forms.Form, error) { return newContactForm(), nil },
//		FormValid: func(r *http.Request, form *forms.Form) error {
//			return sendMessage(r.Context(), form.CleanedData())
//		},
//		SuccessURL: "/thanks/",
//	})
//
// Keep csrf.Middleware in front of a view that accepts a form.
type FormView struct {
	// Engine renders the template and the error templates.
	Engine *templates.Engine
	// Form returns a new unbound form for the request, the counterpart of
	// form_class and get_form_class. The view applies Initial and Prefix and
	// binds the form to the submitted data, so Form must return a new form on
	// every call. It is required.
	Form func(r *http.Request) (*forms.Form, error)
	// Initial holds initial values by field name (initial). They are merged
	// over the Initial of the form.
	Initial map[string]any
	// Prefix replaces the Prefix of the form when it is not empty (prefix).
	Prefix string
	// SuccessURL is the URL to redirect to after valid data (success_url). It
	// is required unless GetSuccessURL is set.
	SuccessURL string
	// GetSuccessURL replaces overriding get_success_url. It receives the valid
	// form.
	GetSuccessURL func(r *http.Request, form *forms.Form) (string, error)
	// FormValid runs for valid data before the redirect, the Go form of
	// overriding form_valid and calling super().form_valid(). An error it
	// returns is answered by ServeError. When it adds an error to the form
	// (form.AddError), the view renders the form again instead of
	// redirecting, as a form_valid that returns form_invalid() does.
	FormValid func(r *http.Request, form *forms.Form) error
	// SuccessMessage is added as a success message after valid data, with its
	// %(name)s placeholders filled from the cleaned data of the form
	// (success_message of SuccessMessageMixin). It needs messages.Middleware.
	SuccessMessage string
	// GetSuccessMessage replaces overriding get_success_message. It receives
	// the cleaned data of the form. An empty result adds no message.
	GetSuccessMessage func(r *http.Request, cleanedData map[string]any) (string, error)
	// TemplateName is the template to render (template_name). It is required
	// unless GetTemplateNames is set.
	TemplateName string
	// ContentType overrides the Content-Type header (content_type).
	ContentType string
	// ExtraContext is added to the context (extra_context).
	ExtraContext map[string]any
	// GetContextData replaces overriding get_context_data. The form in the
	// context it receives is the *forms.Form.
	GetContextData ContextFunc
	// GetTemplateNames replaces overriding get_template_names.
	GetTemplateNames func(r *http.Request) ([]string, error)
}

// ServeHTTP renders the form for GET and HEAD and processes the submitted data
// for POST and PUT.
func (v FormView) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	dispatch(w, r, v.Engine, methodHandlers{get: v.process, post: v.process, put: v.process})
}

// form returns the form for the request, bound to the submitted data of a POST
// or PUT, Django's FormMixin.get_form.
func (v FormView) form(r *http.Request) (*forms.Form, error) {
	if v.Form == nil {
		return nil, improperlyConfigured("FormView requires Form, the function that returns its form")
	}
	form, err := v.Form(r)
	if err != nil {
		return nil, err
	}
	if form == nil {
		return nil, improperlyConfigured("FormView.Form returned no form")
	}
	prepareForm(form, r, v.Initial, v.Prefix)
	data, bound, err := formData(r)
	if err != nil {
		return nil, err
	}
	if bound {
		form = form.Bind(data)
	}
	return form, nil
}

// process renders an unbound form, and validates a bound one, Django's
// ProcessFormView: valid data is answered with the redirect to the success
// URL, data with errors with the form again.
func (v FormView) process(w http.ResponseWriter, r *http.Request) error {
	form, err := v.form(r)
	if err != nil {
		return err
	}
	if !form.IsBound() {
		return v.render(w, r, form)
	}
	var hook func() error
	if v.FormValid != nil {
		hook = func() error { return v.FormValid(r, form) }
	}
	valid, err := formValid(form, v.checkSuccessURL, hook)
	if err != nil {
		return err
	}
	if !valid {
		return v.render(w, r, form)
	}
	target := v.SuccessURL
	if v.GetSuccessURL != nil {
		if target, err = v.GetSuccessURL(r, form); err != nil {
			return err
		}
	}
	cleaned := form.CleanedData()
	var get func() (string, error)
	if v.GetSuccessMessage != nil {
		get = func() (string, error) { return v.GetSuccessMessage(r, cleaned) }
	}
	return redirectFound(w, target, func() error { return addSuccessMessage(r, v.SuccessMessage, get, cleaned) })
}

// checkSuccessURL reports a view without a success URL, Django's
// FormMixin.get_success_url.
func (v FormView) checkSuccessURL() error {
	if v.SuccessURL == "" && v.GetSuccessURL == nil {
		return errNoSuccessURL()
	}
	return nil
}

// render writes the page with the form, Django's
// render_to_response(get_context_data(form=form)).
func (v FormView) render(w http.ResponseWriter, r *http.Request, form *forms.Form) error {
	ctx, err := viewContext(r, v, map[string]any{"form": form}, v.ExtraContext, v.GetContextData)
	if err != nil {
		return err
	}
	var names []string
	if v.GetTemplateNames != nil {
		names, err = v.GetTemplateNames(r)
	} else {
		names, err = baseTemplateNames("FormView", v.TemplateName)
	}
	if err != nil {
		return err
	}
	return renderResponse(w, r, v.Engine, names, ctx, contentTypeOption(v.ContentType))
}
