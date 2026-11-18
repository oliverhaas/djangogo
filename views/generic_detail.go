package views

import (
	"net/http"

	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/templates"
)

// DetailView renders one object of model T, Django's DetailView. It answers
// GET and HEAD. The object is looked up by the path value "pk" or "slug" of
// the route:
//
//	mux.Handle("/posts/{pk}/", views.DetailView[Post]{DB: db, Engine: engine})
//
// The template is "<app>/<model>_detail.html" and its context holds the object
// under "object" and under the lowercased model name ("post"), plus the view
// under "view". A missing object is answered with the 404 page.
//
// T is the model struct type. Set DB also when Queryset is set: the model name,
// the default template name and the template form of the object come from the
// registry of DB.
type DetailView[T any] struct {
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
	// (template_name_suffix, default "_detail").
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
}

// ServeHTTP renders the object for GET and HEAD.
func (v DetailView[T]) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	dispatch(w, r, v.Engine, methodHandlers{get: v.get})
}

// core maps the exported fields to the shared single-object machinery.
func (v DetailView[T]) core() objectView[T] {
	return objectView[T]{
		name:               "DetailView",
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
		templateNameSuffix: orDefault(v.TemplateNameSuffix, "_detail"),
		contentType:        v.ContentType,
		extraContext:       v.ExtraContext,
		getContextData:     v.GetContextData,
		getTemplateNames:   v.GetTemplateNames,
	}
}

// get looks the object up and renders it.
func (v DetailView[T]) get(w http.ResponseWriter, r *http.Request) error {
	core := v.core()
	obj, err := core.object(r)
	if err != nil {
		return err
	}
	return core.render(w, r, obj, core.context(obj))
}
