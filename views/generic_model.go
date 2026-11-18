package views

import (
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"unicode"

	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/templates"
)

// modelFor returns the model registered for T in db, or nil when db is nil or
// T is not a registered model.
func modelFor[T any](db *orm.DB) *orm.Model {
	if db == nil {
		return nil
	}
	var zero T
	m, ok := db.Registry().ModelOf(zero)
	if !ok {
		return nil
	}
	return m
}

// modelName returns Django's model_name: the lowercased type name. It is the
// context key of a single object ("author"), the stem of the list key
// ("author_list") and the stem of the default template names.
func modelName(m *orm.Model) string {
	return strings.ToLower(m.Name())
}

// verboseName returns Django's default verbose_name of a model: the type name
// split at its capitals and lowercased ("BlogPost" gives "blog post").
func verboseName(m *orm.Model) string {
	var b strings.Builder
	runes := []rune(m.Name())
	for i, c := range runes {
		if i > 0 && unicode.IsUpper(c) &&
			(unicode.IsLower(runes[i-1]) || (i+1 < len(runes) && unicode.IsLower(runes[i+1]))) {
			b.WriteByte(' ')
		}
		b.WriteRune(unicode.ToLower(c))
	}
	return b.String()
}

// modelTemplateName returns the default template name of a model view,
// "<app>/<model><suffix>.html", or "<model><suffix>.html" for a model that was
// registered without an app label.
func modelTemplateName(m *orm.Model, suffix string) string {
	name := modelName(m) + suffix + ".html"
	if m.App() == "" {
		return name
	}
	return m.App() + "/" + name
}

// modelContext returns the context a model view hands to the template engine:
// every top-level value of the view's model type T (an instance, a pointer to
// one, or a slice of either) is replaced by its templates.ModelMap, so a
// template reads {{ object.title }} by column name. Hooks run before this step
// and see the typed values. Without a registered model ctx is returned as it
// is.
//
// A list view has its list under two keys ("object_list" and "<model>_list").
// The list is converted once, so both keys still hold one slice.
func modelContext[T any](db *orm.DB, ctx map[string]any) map[string]any {
	m := modelFor[T](db)
	if m == nil {
		return ctx
	}
	converted := make(map[*T][]any)
	out := make(map[string]any, len(ctx))
	for key, value := range ctx {
		switch v := value.(type) {
		case *T:
			out[key] = templates.ModelContext(m, v)
		case []T:
			if len(v) == 0 {
				out[key] = []any{}
				continue
			}
			maps, ok := converted[&v[0]]
			if !ok || len(maps) != len(v) {
				maps = modelMaps(m, v)
				converted[&v[0]] = maps
			}
			out[key] = maps
		case []*T:
			out[key] = modelMaps(m, v)
		case T:
			out[key] = templates.ModelContext(m, v)
		default:
			out[key] = value
		}
	}
	return out
}

// modelMaps converts each element with templates.ModelContext.
func modelMaps[E any](m *orm.Model, objects []E) []any {
	out := make([]any, len(objects))
	for i := range objects {
		out[i] = templates.ModelContext(m, objects[i])
	}
	return out
}

// fieldValue returns the value of the field of obj that is stored in column,
// with "pk" naming the primary key. It reports false for a nil model, an
// unknown column, a relation field and a nil instance.
func fieldValue[T any](m *orm.Model, obj T, column string) (any, bool) {
	if m == nil {
		return nil, false
	}
	f, ok := m.FieldByColumn(column)
	if column == "pk" {
		f, ok = m.PrimaryKey(), m.PrimaryKey() != nil
	}
	if !ok || f.Rel != nil {
		return nil, false
	}
	v := reflect.ValueOf(obj)
	for v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return nil, false
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return nil, false
	}
	return v.Field(f.Index).Interface(), true
}

// objectView is the machinery shared by the views that work on one model
// instance: the lookup by pk or slug, the context with the model-named key and
// the choice of template. It combines Django's SingleObjectMixin and
// SingleObjectTemplateResponseMixin. DetailView and DeleteView fill it from
// their exported fields, and so does every further view on a single object.
type objectView[T any] struct {
	// name is the exported type name of the view, for configuration errors.
	name string
	// self is the view value, the "view" entry of the context.
	self any

	db     *orm.DB
	engine *templates.Engine

	queryset       *orm.QuerySet[T]
	getQueryset    func(r *http.Request) (*orm.QuerySet[T], error)
	getObject      func(r *http.Request) (T, error)
	pkURLKwarg     string
	slugURLKwarg   string
	slugField      string
	queryPKAndSlug bool

	contextObjectName  string
	templateName       string
	templateNameField  string
	templateNameSuffix string
	contentType        string
	extraContext       map[string]any
	getContextData     ContextFunc
	getTemplateNames   func(r *http.Request, obj T) ([]string, error)
}

// orDefault returns value, or fallback when value is empty.
func orDefault(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

// baseQueryset returns the queryset the object is looked up in, Django's
// SingleObjectMixin.get_queryset.
func (o objectView[T]) baseQueryset(r *http.Request) (*orm.QuerySet[T], error) {
	if o.getQueryset != nil {
		qs, err := o.getQueryset(r)
		if err != nil {
			return nil, err
		}
		if qs == nil {
			return nil, improperlyConfigured("%s.GetQueryset returned no queryset", o.name)
		}
		return qs, nil
	}
	if o.queryset != nil {
		return o.queryset, nil
	}
	return orm.Query[T](o.db), nil
}

// object returns the object the request addresses, Django's
// SingleObjectMixin.get_object. The GetObject hook replaces the lookup.
// Otherwise the path value named by pkURLKwarg (default "pk") filters the
// primary key, and the path value named by slugURLKwarg (default "slug")
// filters slugField (default "slug") when there is no pk or queryPKAndSlug is
// set. A request without a match gets a 404 with Django's message.
func (o objectView[T]) object(r *http.Request) (T, error) {
	var zero T
	if o.getObject != nil {
		return o.getObject(r)
	}
	m := modelFor[T](o.db)
	if m == nil {
		return zero, improperlyConfigured(
			"%s needs DB and a registered model %s to look up its object, or an implementation of GetObject",
			o.name, modelTypeName[T]())
	}
	qs, err := o.baseQueryset(r)
	if err != nil {
		return zero, err
	}
	pk := r.PathValue(orDefault(o.pkURLKwarg, "pk"))
	slug := r.PathValue(orDefault(o.slugURLKwarg, "slug"))
	if pk == "" && slug == "" {
		return zero, improperlyConfigured(
			"%s must be called with either an object pk or a slug in the route pattern", o.name)
	}
	notFound := NotFound(fmt.Sprintf("No %s found matching the query", verboseName(m)))
	if pk != "" {
		value, ok := pkValue(m.PrimaryKey(), pk)
		if !ok {
			return zero, notFound
		}
		qs = qs.Filter(m.PrimaryKey().Column, value)
	}
	if slug != "" && (pk == "" || o.queryPKAndSlug) {
		qs = qs.Filter(orDefault(o.slugField, "slug"), slug)
	}
	obj, err := qs.Get(r.Context())
	if errors.Is(err, orm.ErrDoesNotExist) {
		return zero, notFound
	}
	return obj, err
}

// pkValue converts the text of a path value to the Go value the primary key is
// filtered with. It reports false for text that cannot be a value of an
// integer key, which then matches no object.
func pkValue(pk *orm.Field, raw string) (any, bool) {
	if pk.Kind != orm.KindAuto && pk.Kind != orm.KindInt {
		return raw, true
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return nil, false
	}
	return n, true
}

// context returns the base context for obj, Django's
// SingleObjectMixin.get_context_data: the object under "object" and under
// contextObjectName, which defaults to the lowercased model name.
func (o objectView[T]) context(obj T) map[string]any {
	ctx := map[string]any{"object": obj}
	name := o.contextObjectName
	if name == "" {
		if m := modelFor[T](o.db); m != nil {
			name = modelName(m)
		}
	}
	if name != "" {
		ctx[name] = obj
	}
	return ctx
}

// templateNames returns the template candidates for obj, Django's
// SingleObjectTemplateResponseMixin.get_template_names: the explicit name
// alone, otherwise the name stored in the templateNameField column of obj
// followed by "<app>/<model><suffix>.html". A stored name that is not a clean
// relative path is skipped, so data cannot point the engine outside its
// template directories.
func (o objectView[T]) templateNames(r *http.Request, obj T) ([]string, error) {
	if o.getTemplateNames != nil {
		return o.getTemplateNames(r, obj)
	}
	if o.templateName != "" {
		return []string{o.templateName}, nil
	}
	m := modelFor[T](o.db)
	if m == nil {
		return nil, improperlyConfigured(
			"%s requires a definition of TemplateName, DB with a registered model %s, "+
				"or an implementation of GetTemplateNames", o.name, modelTypeName[T]())
	}
	var names []string
	if o.templateNameField != "" {
		value, _ := fieldValue(m, obj, o.templateNameField)
		if name, ok := value.(string); ok && fs.ValidPath(name) {
			names = append(names, name)
		}
	}
	return append(names, modelTemplateName(m, o.templateNameSuffix)), nil
}

// render writes the page for obj. base is the context before the "view" entry,
// the extra context and the GetContextData hook are applied: the result of
// context for a view that shows an object, with further entries such as a form
// added by the caller. Nothing is written when render returns an error.
func (o objectView[T]) render(
	w http.ResponseWriter, r *http.Request, obj T, base map[string]any, opts ...ResponseOption,
) error {
	data, err := viewContext(r, o.self, base, o.extraContext, o.getContextData)
	if err != nil {
		return err
	}
	names, err := o.templateNames(r, obj)
	if err != nil {
		return err
	}
	opts = append(contentTypeOption(o.contentType), opts...)
	return renderResponse(w, r, o.engine, names, modelContext[T](o.db, data), opts)
}
