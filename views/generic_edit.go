package views

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/oliverhaas/djangogo/forms"
	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/templates"
)

// escapeURLValue percent-encodes a value that is inserted into a URL. Only
// unreserved characters (letters, digits, "-", "_", ".", "~") stay, so the
// value cannot add a path segment, a query parameter or a fragment.
func escapeURLValue(value any) string {
	const upperHex = "0123456789ABCDEF"
	s := fmt.Sprint(value)
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9',
			c == '-', c == '_', c == '.', c == '~':
			b.WriteByte(c)
		default:
			b.WriteByte('%')
			b.WriteByte(upperHex[c>>4])
			b.WriteByte(upperHex[c&0x0f])
		}
	}
	return b.String()
}

// formatURL replaces each {name} placeholder of pattern with the value lookup
// returns for the name, percent-encoded. A placeholder without value is a
// configuration error. Text that is not a placeholder is copied unchanged.
func formatURL(pattern string, lookup func(name string) (any, bool)) (string, error) {
	var b strings.Builder
	rest := pattern
	for {
		start := strings.IndexByte(rest, '{')
		if start < 0 {
			b.WriteString(rest)
			return b.String(), nil
		}
		end := strings.IndexByte(rest[start:], '}')
		if end < 0 {
			b.WriteString(rest)
			return b.String(), nil
		}
		name := rest[start+1 : start+end]
		value, ok := lookup(name)
		if !ok {
			return "", improperlyConfigured("URL %q has the placeholder {%s}, which has no value", pattern, name)
		}
		b.WriteString(rest[:start])
		b.WriteString(escapeURLValue(value))
		rest = rest[start+end+1:]
	}
}

// objectURL fills the {column} placeholders of pattern with the field values
// of obj ("{pk}" is the primary key), the counterpart of Django's
// success_url.format(**self.object.__dict__). Unlike str.format it
// percent-encodes the values.
func objectURL[T any](db *orm.DB, pattern string, obj T) (string, error) {
	m := modelFor[T](db)
	return formatURL(pattern, func(name string) (any, bool) {
		return fieldValue(m, obj, name)
	})
}

// successURL returns the URL a view redirects to after it saved obj, Django's
// ModelFormMixin.get_success_url: explicit with its {column} placeholders
// filled from obj, or the GetAbsoluteURL of obj when explicit is empty. A
// model without that method and no explicit URL is a configuration error.
func successURL[T any](db *orm.DB, explicit string, obj T) (string, error) {
	if explicit != "" {
		return objectURL(db, explicit, obj)
	}
	if u, ok := any(obj).(AbsoluteURLer); ok {
		return u.GetAbsoluteURL(), nil
	}
	if u, ok := any(&obj).(AbsoluteURLer); ok {
		return u.GetAbsoluteURL(), nil
	}
	return "", errNoAbsoluteURL()
}

// errNoAbsoluteURL is the configuration error of a view that saves an object
// and has neither a success URL nor a model with a GetAbsoluteURL method.
func errNoAbsoluteURL() error {
	return improperlyConfigured(
		"No URL to redirect to. Either provide a SuccessURL or define a GetAbsoluteURL method on the model.")
}

// DeleteView deletes one object of model T, Django's DeleteView. GET and HEAD
// render a confirmation page from "<app>/<model>_confirm_delete.html", with
// the same context and the same lookup as DetailView, and the confirmation
// form under "form". POST validates the form and, when it is valid, deletes
// the object and redirects to SuccessURL with status 302. A form with errors
// renders the page again. DELETE deletes without a form, as in Django.
//
//	mux.Handle("/posts/{pk}/delete/", views.DeleteView[Post]{
//		DB: db, Engine: engine, SuccessURL: "/posts/",
//	})
//
// The view does not check who may delete. Wrap it in auth.LoginRequired or
// auth.PermissionRequired, and keep csrf.Middleware in front of it.
type DeleteView[T any] struct {
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
	// (template_name_suffix, default "_confirm_delete").
	TemplateNameSuffix string
	// ContentType overrides the Content-Type header (content_type).
	ContentType string
	// ExtraContext is added to the context (extra_context).
	ExtraContext map[string]any
	// GetContextData replaces overriding get_context_data. The object in the
	// context it receives has the type T.
	GetContextData ContextFunc
	// GetTemplateNames replaces overriding get_template_names.
	GetTemplateNames func(r *http.Request, obj T) ([]string, error)
	// SuccessURL is the URL to redirect to after the deletion (success_url).
	// A {column} placeholder is replaced by that field of the deleted object,
	// percent-encoded, for example "/posts/?deleted={id}". It is required
	// unless GetSuccessURL is set.
	SuccessURL string
	// GetSuccessURL replaces overriding get_success_url. It runs before the
	// object is deleted.
	GetSuccessURL func(r *http.Request, obj T) (string, error)
	// DeleteObject replaces the deletion itself, for example to mark the
	// object as deleted instead of removing its row. Nil deletes the row with
	// the primary key of the object.
	DeleteObject func(r *http.Request, obj T) error
	// Form returns a new unbound confirmation form for the request
	// (form_class), which a POST must pass before anything is deleted. Nil
	// uses a form without fields, which is always valid.
	Form func(r *http.Request) (*forms.Form, error)
}

// ServeHTTP renders the confirmation page for GET and HEAD and deletes the
// object for POST and DELETE.
func (v DeleteView[T]) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	dispatch(w, r, v.Engine, methodHandlers{get: v.get, post: v.post, delete: v.delete})
}

// core maps the exported fields to the shared single-object machinery.
func (v DeleteView[T]) core() objectView[T] {
	return objectView[T]{
		name:               "DeleteView",
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
		templateNameSuffix: orDefault(v.TemplateNameSuffix, "_confirm_delete"),
		contentType:        v.ContentType,
		extraContext:       v.ExtraContext,
		getContextData:     v.GetContextData,
		getTemplateNames:   v.GetTemplateNames,
	}
}

// form returns the confirmation form, bound to the submitted data of a POST,
// Django's FormMixin.get_form with the form_class Form of BaseDeleteView.
func (v DeleteView[T]) form(r *http.Request) (*forms.Form, error) {
	form := forms.New()
	if v.Form != nil {
		var err error
		if form, err = v.Form(r); err != nil {
			return nil, err
		}
		if form == nil {
			return nil, improperlyConfigured("DeleteView.Form returned no form")
		}
	}
	prepareForm(form, r, nil, "")
	data, bound, err := formData(r)
	if err != nil {
		return nil, err
	}
	if bound {
		form = form.Bind(data)
	}
	return form, nil
}

// get renders the confirmation page.
func (v DeleteView[T]) get(w http.ResponseWriter, r *http.Request) error {
	core := v.core()
	obj, err := core.object(r)
	if err != nil {
		return err
	}
	form, err := v.form(r)
	if err != nil {
		return err
	}
	return v.render(w, r, obj, form)
}

// render writes the confirmation page with the form.
func (v DeleteView[T]) render(w http.ResponseWriter, r *http.Request, obj T, form *forms.Form) error {
	core := v.core()
	ctx := core.context(obj)
	ctx["form"] = form
	return core.render(w, r, obj, ctx)
}

// post validates the confirmation form, Django's BaseDeleteView.post: valid
// data deletes the object, data with errors renders the page again.
func (v DeleteView[T]) post(w http.ResponseWriter, r *http.Request) error {
	obj, err := v.core().object(r)
	if err != nil {
		return err
	}
	form, err := v.form(r)
	if err != nil {
		return err
	}
	if !form.IsValid() {
		if err := form.Err(); err != nil {
			return err
		}
		return v.render(w, r, obj, form)
	}
	return v.deleteAndRedirect(w, r, obj)
}

// delete removes the object without a form, Django's DeletionMixin.delete.
func (v DeleteView[T]) delete(w http.ResponseWriter, r *http.Request) error {
	obj, err := v.core().object(r)
	if err != nil {
		return err
	}
	return v.deleteAndRedirect(w, r, obj)
}

// deleteAndRedirect removes obj and redirects to the success URL. The URL is
// resolved and checked first, so a view without a valid target deletes
// nothing.
func (v DeleteView[T]) deleteAndRedirect(w http.ResponseWriter, r *http.Request, obj T) error {
	target, err := v.successURL(r, obj)
	if err != nil {
		return err
	}
	location, err := checkedLocation(target, 0)
	if err != nil {
		return err
	}
	if v.DeleteObject != nil {
		err = v.DeleteObject(r, obj)
	} else {
		err = deleteObject(r.Context(), v.DB, obj)
	}
	if err != nil {
		return err
	}
	writeRedirect(w, http.StatusFound, location)
	return nil
}

// successURL returns the redirect target after the deletion, Django's
// DeletionMixin.get_success_url. Unlike the views that save an object it does
// not fall back to GetAbsoluteURL, because the object is gone.
func (v DeleteView[T]) successURL(r *http.Request, obj T) (string, error) {
	if v.GetSuccessURL != nil {
		return v.GetSuccessURL(r, obj)
	}
	if v.SuccessURL == "" {
		return "", errNoSuccessURL()
	}
	return objectURL(v.DB, v.SuccessURL, obj)
}

// deleteObject deletes the row of obj by its primary key.
func deleteObject[T any](ctx context.Context, db *orm.DB, obj T) error {
	m := modelFor[T](db)
	pk, ok := fieldValue(m, obj, "pk")
	if !ok {
		return improperlyConfigured(
			"DeleteView needs DB and a registered model %s to delete its object, or an implementation of DeleteObject",
			modelTypeName[T]())
	}
	_, err := orm.Query[T](db).Filter(m.PrimaryKey().Column, pk).Delete(ctx)
	return err
}
