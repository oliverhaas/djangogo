package views

import (
	"net/http"
	"reflect"

	"github.com/oliverhaas/djangogo/forms"
	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/templates"
)

// modelFormView is the machinery CreateView and UpdateView share, Django's
// ModelFormMixin with ProcessFormView: the form of the model, its validation,
// the saving and the redirect to the success URL.
type modelFormView[T any] struct {
	objectView[T]

	fields        []string
	form          func(r *http.Request) (*forms.ModelForm[T], error)
	initial       map[string]any
	prefix        string
	successURL    string
	getSuccessURL func(r *http.Request, obj T) (string, error)
	formValid     func(r *http.Request, form *forms.ModelForm[T]) error
}

// modelForm returns the form for the request, Django's
// ModelFormMixin.get_form_class with get_form: it edits instance when that is
// not nil and is bound to the submitted data of a POST or PUT.
func (m modelFormView[T]) modelForm(r *http.Request, instance *T) (*forms.ModelForm[T], error) {
	var form *forms.ModelForm[T]
	switch {
	case m.form != nil && len(m.fields) > 0:
		return nil, improperlyConfigured("Specifying both Fields and Form is not permitted (%s).", m.name)
	case m.form != nil:
		f, err := m.form(r)
		if err != nil {
			return nil, err
		}
		if f == nil {
			return nil, improperlyConfigured("%s.Form returned no form", m.name)
		}
		form = f
	case len(m.fields) == 0:
		return nil, improperlyConfigured("Using %s without the Fields attribute is prohibited.", m.name)
	case modelFor[T](m.db) == nil:
		return nil, improperlyConfigured(
			"%s needs DB and a registered model %s to build its form, or an implementation of Form",
			m.name, modelTypeName[T]())
	default:
		form = forms.NewModelForm[T](m.db, forms.WithFields(m.fields...))
	}
	if instance != nil {
		form = form.WithInstance(instance)
	}
	if err := form.Err(); err != nil {
		return nil, err
	}
	prepareForm(form.Form, r, m.initial, m.prefix)
	data, bound, err := formData(r)
	if err != nil {
		return nil, err
	}
	if bound {
		form = form.Bind(data)
	}
	return form, nil
}

// process renders an unbound form with render, and validates a bound one,
// Django's ProcessFormView with ModelFormMixin.form_valid: valid data is saved
// and answered with the redirect to the success URL of the saved object, data
// with errors with the form again.
func (m modelFormView[T]) process(
	w http.ResponseWriter, r *http.Request, form *forms.ModelForm[T], render func(*forms.ModelForm[T]) error,
) error {
	if !form.IsBound() {
		return render(form)
	}
	var hook func() error
	if m.formValid != nil {
		hook = func() error { return m.formValid(r, form) }
	}
	valid, err := formValid(form.Form, m.checkSuccessURL, hook)
	if err != nil {
		return err
	}
	if !valid {
		return render(form)
	}
	obj, err := form.Save(r.Context())
	if err != nil {
		return err
	}
	var target string
	if m.getSuccessURL != nil {
		target, err = m.getSuccessURL(r, *obj)
	} else {
		target, err = successURL(m.db, m.successURL, *obj)
	}
	if err != nil {
		return err
	}
	return redirectFound(w, target)
}

// checkSuccessURL reports a view that cannot name a success URL, before the
// form is saved. Django finds the mistake only after saving.
func (m modelFormView[T]) checkSuccessURL() error {
	if m.getSuccessURL != nil || m.successURL != "" || hasAbsoluteURL[T]() {
		return nil
	}
	return errNoAbsoluteURL()
}

// absoluteURLerType is the type of the AbsoluteURLer interface.
var absoluteURLerType = reflect.TypeFor[AbsoluteURLer]()

// hasAbsoluteURL reports whether a value of T, or a pointer to one, has a
// GetAbsoluteURL method.
func hasAbsoluteURL[T any]() bool {
	t := reflect.TypeFor[T]()
	return t.Implements(absoluteURLerType) || reflect.PointerTo(t).Implements(absoluteURLerType)
}

// CreateView shows a form for a new object of model T and saves it, Django's
// CreateView. GET and HEAD render "<app>/<model>_form.html" with the form under
// "form". POST and PUT validate the submitted data. Valid data is saved as a
// new row and answered with a 302 redirect to SuccessURL or to the
// GetAbsoluteURL of the new object. Data with errors renders the template
// again, with the bound form and its errors.
//
//	mux.Handle("/posts/new/", views.CreateView[Post]{
//		DB: db, Engine: engine, Fields: []string{"title", "body"}, SuccessURL: "/posts/{id}/",
//	})
//
// The view does not check who may create. Wrap it in auth.LoginRequired or
// auth.PermissionRequired, and keep csrf.Middleware in front of it.
type CreateView[T any] struct {
	// DB is the database of the model (model).
	DB *orm.DB
	// Engine renders the template and the error templates.
	Engine *templates.Engine
	// Fields names the model fields of the form by attribute name (fields).
	// []string{forms.AllFields} selects every editable field. Either Fields or
	// Form is required, not both.
	Fields []string
	// Form returns a new unbound form for the request, the counterpart of
	// form_class and get_form_class, for a form with more options than Fields
	// (widgets, labels, form fields). The view applies Initial and Prefix and
	// binds the form, so Form must return a new form on every call.
	Form func(r *http.Request) (*forms.ModelForm[T], error)
	// Initial holds initial values by field name (initial). They are merged
	// over the Initial of the form.
	Initial map[string]any
	// Prefix replaces the Prefix of the form when it is not empty (prefix).
	Prefix string
	// SuccessURL is the URL to redirect to after saving (success_url). A
	// {column} placeholder is replaced by that field of the saved object,
	// percent-encoded, for example "/posts/{id}/". Without SuccessURL and
	// GetSuccessURL the view redirects to the GetAbsoluteURL of the object.
	SuccessURL string
	// GetSuccessURL replaces overriding get_success_url. It receives the saved
	// object.
	GetSuccessURL func(r *http.Request, obj T) (string, error)
	// FormValid runs for valid data before the object is saved, the Go form of
	// overriding form_valid and calling super().form_valid(). It may change
	// the object through form.Instance(), for example to set its owner. An
	// error it returns is answered by ServeError. When it adds an error to the
	// form (form.AddError), nothing is saved and the view renders the form
	// again.
	FormValid func(r *http.Request, form *forms.ModelForm[T]) error
	// TemplateName is the template to render (template_name). It replaces the
	// default name.
	TemplateName string
	// TemplateNameSuffix is the suffix of the default template name
	// (template_name_suffix, default "_form").
	TemplateNameSuffix string
	// ContentType overrides the Content-Type header (content_type).
	ContentType string
	// ExtraContext is added to the context (extra_context).
	ExtraContext map[string]any
	// GetContextData replaces overriding get_context_data. The form in the
	// context it receives is the *forms.ModelForm[T].
	GetContextData ContextFunc
	// GetTemplateNames replaces overriding get_template_names.
	GetTemplateNames func(r *http.Request) ([]string, error)
}

// ServeHTTP renders the form for GET and HEAD and processes the submitted data
// for POST and PUT.
func (v CreateView[T]) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	dispatch(w, r, v.Engine, methodHandlers{get: v.process, post: v.process, put: v.process})
}

// core maps the exported fields to the shared machinery.
func (v CreateView[T]) core() modelFormView[T] {
	var getTemplateNames func(r *http.Request, obj T) ([]string, error)
	if v.GetTemplateNames != nil {
		getTemplateNames = func(r *http.Request, _ T) ([]string, error) { return v.GetTemplateNames(r) }
	}
	return modelFormView[T]{
		objectView: objectView[T]{
			name:               "CreateView",
			self:               v,
			db:                 v.DB,
			engine:             v.Engine,
			templateName:       v.TemplateName,
			templateNameSuffix: orDefault(v.TemplateNameSuffix, "_form"),
			contentType:        v.ContentType,
			extraContext:       v.ExtraContext,
			getContextData:     v.GetContextData,
			getTemplateNames:   getTemplateNames,
		},
		fields:        v.Fields,
		form:          v.Form,
		initial:       v.Initial,
		prefix:        v.Prefix,
		successURL:    v.SuccessURL,
		getSuccessURL: v.GetSuccessURL,
		formValid:     v.FormValid,
	}
}

// process renders or processes the form of a new object. The context has no
// object, as the object of Django's CreateView is None until it is saved.
func (v CreateView[T]) process(w http.ResponseWriter, r *http.Request) error {
	core := v.core()
	form, err := core.modelForm(r, nil)
	if err != nil {
		return err
	}
	return core.process(w, r, form, func(form *forms.ModelForm[T]) error {
		var zero T
		return core.render(w, r, zero, map[string]any{"form": form})
	})
}

// UpdateView shows a form for an existing object of model T and saves the
// changes, Django's UpdateView. The object is looked up as by DetailView. GET
// and HEAD render "<app>/<model>_form.html" with the object under "object" and
// under the model name, and the form with the values of the object under
// "form". POST and PUT validate the submitted data. Valid data updates the row
// of the object and is answered with a 302 redirect to SuccessURL or to the
// GetAbsoluteURL of the object. Data with errors renders the template again.
//
//	mux.Handle("/posts/{pk}/edit/", views.UpdateView[Post]{
//		DB: db, Engine: engine, Fields: []string{"title", "body"},
//	})
//
// Django's context holds the object after the submitted data was applied to
// it. The object in the context of this view stays as it was loaded; the form
// edits a copy.
//
// The view does not check who may edit. Wrap it in auth.LoginRequired or
// auth.PermissionRequired, and keep csrf.Middleware in front of it.
type UpdateView[T any] struct {
	// DB is the database of the model (model).
	DB *orm.DB
	// Engine renders the template and the error templates.
	Engine *templates.Engine
	// Queryset restricts the lookup (queryset). Nil looks in all objects.
	Queryset *orm.QuerySet[T]
	// GetQueryset replaces overriding get_queryset.
	GetQueryset func(r *http.Request) (*orm.QuerySet[T], error)
	// GetObject replaces overriding get_object. It takes the place of the
	// whole lookup. Return an ErrNotFound for a missing object.
	GetObject func(r *http.Request) (T, error)
	// PKURLKwarg names the path value with the primary key (pk_url_kwarg,
	// default "pk").
	PKURLKwarg string
	// SlugURLKwarg names the path value with the slug (slug_url_kwarg, default
	// "slug").
	SlugURLKwarg string
	// SlugField is the column the slug is compared with (slug_field, default
	// "slug").
	SlugField string
	// QueryPKAndSlug filters by pk and slug when the route has both
	// (query_pk_and_slug). Otherwise the pk alone decides.
	QueryPKAndSlug bool
	// Fields names the model fields of the form by attribute name (fields).
	// []string{forms.AllFields} selects every editable field. Either Fields or
	// Form is required, not both.
	Fields []string
	// Form returns a new unbound form for the request, the counterpart of
	// form_class and get_form_class. The view sets the object as the instance
	// of the form, applies Initial and Prefix and binds the form, so Form must
	// return a new form on every call.
	Form func(r *http.Request) (*forms.ModelForm[T], error)
	// Initial holds initial values by field name (initial). They win over the
	// values of the object.
	Initial map[string]any
	// Prefix replaces the Prefix of the form when it is not empty (prefix).
	Prefix string
	// SuccessURL is the URL to redirect to after saving (success_url). A
	// {column} placeholder is replaced by that field of the saved object,
	// percent-encoded. Without SuccessURL and GetSuccessURL the view
	// redirects to the GetAbsoluteURL of the object.
	SuccessURL string
	// GetSuccessURL replaces overriding get_success_url. It receives the saved
	// object.
	GetSuccessURL func(r *http.Request, obj T) (string, error)
	// FormValid runs for valid data before the changes are saved, the Go form
	// of overriding form_valid and calling super().form_valid(). It may change
	// the object through form.Instance(). An error it returns is answered by
	// ServeError. When it adds an error to the form (form.AddError), nothing
	// is saved and the view renders the form again.
	FormValid func(r *http.Request, form *forms.ModelForm[T]) error
	// ContextObjectName is the context key of the object next to "object"
	// (context_object_name, default the lowercased model name).
	ContextObjectName string
	// TemplateName is the template to render (template_name). It replaces the
	// default name.
	TemplateName string
	// TemplateNameField is a column of the object that holds a template name,
	// tried before the default name (template_name_field).
	TemplateNameField string
	// TemplateNameSuffix is the suffix of the default template name
	// (template_name_suffix, default "_form").
	TemplateNameSuffix string
	// ContentType overrides the Content-Type header (content_type).
	ContentType string
	// ExtraContext is added to the context (extra_context).
	ExtraContext map[string]any
	// GetContextData replaces overriding get_context_data. The object in the
	// context it receives has the type T, the form *forms.ModelForm[T].
	GetContextData ContextFunc
	// GetTemplateNames replaces overriding get_template_names.
	GetTemplateNames func(r *http.Request, obj T) ([]string, error)
}

// ServeHTTP renders the form for GET and HEAD and processes the submitted data
// for POST and PUT.
func (v UpdateView[T]) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	dispatch(w, r, v.Engine, methodHandlers{get: v.process, post: v.process, put: v.process})
}

// core maps the exported fields to the shared machinery.
func (v UpdateView[T]) core() modelFormView[T] {
	return modelFormView[T]{
		objectView: objectView[T]{
			name:               "UpdateView",
			self:               v,
			db:                 v.DB,
			engine:             v.Engine,
			queryset:           v.Queryset,
			getQueryset:        v.GetQueryset,
			getObject:          v.GetObject,
			pkURLKwarg:         v.PKURLKwarg,
			slugURLKwarg:       v.SlugURLKwarg,
			slugField:          v.SlugField,
			queryPKAndSlug:     v.QueryPKAndSlug,
			contextObjectName:  v.ContextObjectName,
			templateName:       v.TemplateName,
			templateNameField:  v.TemplateNameField,
			templateNameSuffix: orDefault(v.TemplateNameSuffix, "_form"),
			contentType:        v.ContentType,
			extraContext:       v.ExtraContext,
			getContextData:     v.GetContextData,
			getTemplateNames:   v.GetTemplateNames,
		},
		fields:        v.Fields,
		form:          v.Form,
		initial:       v.Initial,
		prefix:        v.Prefix,
		successURL:    v.SuccessURL,
		getSuccessURL: v.GetSuccessURL,
		formValid:     v.FormValid,
	}
}

// process looks up the object and renders or processes its form.
func (v UpdateView[T]) process(w http.ResponseWriter, r *http.Request) error {
	core := v.core()
	obj, err := core.object(r)
	if err != nil {
		return err
	}
	edited := obj
	form, err := core.modelForm(r, &edited)
	if err != nil {
		return err
	}
	return core.process(w, r, form, func(form *forms.ModelForm[T]) error {
		ctx := core.context(obj)
		ctx["form"] = form
		return core.render(w, r, obj, ctx)
	})
}
