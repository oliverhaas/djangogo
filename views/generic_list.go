package views

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/templates"
)

// All returns the whole slice.
func (l sliceList[T]) All(context.Context) ([]T, error) { return l.items, nil }

// All loads every row of the queryset with one query.
func (l querySetList[T]) All(ctx context.Context) ([]T, error) { return l.qs.All(ctx) }

// Exists reports whether the queryset has a row, without loading or counting
// the rows.
func (l querySetList[T]) Exists(ctx context.Context) (bool, error) { return l.qs.Exists(ctx) }

// allObjects returns every object of list. A list with an All method (the
// lists of SliceList and QuerySetList) is read with one call, any other list
// is counted and then sliced.
func allObjects[T any](ctx context.Context, list ObjectList[T]) ([]T, error) {
	if all, ok := list.(interface {
		All(ctx context.Context) ([]T, error)
	}); ok {
		return all.All(ctx)
	}
	n, err := list.Count(ctx)
	if err != nil {
		return nil, err
	}
	return list.Slice(ctx, 0, n)
}

// isEmpty reports whether list has no objects. A list with an Exists method
// (the list of QuerySetList) answers with that cheaper query, any other list
// is counted.
func isEmpty[T any](ctx context.Context, list ObjectList[T]) (bool, error) {
	if e, ok := list.(interface {
		Exists(ctx context.Context) (bool, error)
	}); ok {
		exists, err := e.Exists(ctx)
		return !exists, err
	}
	n, err := list.Count(ctx)
	return n == 0, err
}

// emptyListMessage is Django's 404 message for an empty list that is not
// allowed to be empty.
const emptyListMessage = "Empty list and “ListView.allow_empty” is False."

// ListView renders a list of objects, Django's ListView. It answers GET and
// HEAD. For a model the list is every object of T in DB:
//
//	mux.Handle("/posts/", views.ListView[Post]{DB: db, Engine: engine, PaginateBy: 20})
//
// The template is "<app>/<model>_list.html". Its context holds the objects
// under "object_list" and under "<model>_list" ("post_list"), and "paginator",
// "page_obj" and "is_paginated", which are nil, nil and false without
// PaginateBy. With PaginateBy the page number comes from the path value or the
// query parameter "page", "last" selects the last page, and a page that does
// not exist is answered with the 404 page. A template loops over
// page_obj.object_list where a Django template loops over page_obj.
//
// T is the element type: a model struct, or any other type together with
// ObjectList. Set DB also when Queryset is set: the model name, the default
// template name and the template form of the objects come from the registry
// of DB.
//
// An unpaginated list costs one query, a page costs a COUNT and a
// LIMIT/OFFSET query, and AllowEmpty set to false adds an EXISTS query to a
// paginated list.
type ListView[T any] struct {
	// DB is the database of the model (model).
	DB *orm.DB
	// Engine renders the template and the error templates.
	Engine *templates.Engine
	// Queryset is the list to show (queryset). Nil shows all objects of T.
	Queryset *orm.QuerySet[T]
	// GetQueryset replaces overriding get_queryset.
	GetQueryset func(r *http.Request) (*orm.QuerySet[T], error)
	// ObjectList is a list that is not a queryset, for example SliceList. It
	// is used when Queryset and GetQueryset are unset.
	ObjectList ObjectList[T]
	// Ordering orders the queryset by these columns, "-" first for descending
	// (ordering).
	Ordering []string
	// AllowEmpty set to false answers an empty list with a 404 (allow_empty).
	// Nil means true.
	AllowEmpty *bool
	// PaginateBy is the page size (paginate_by). Zero shows the whole list.
	PaginateBy int
	// PaginateOrphans is the number of items the last page may hold on top of
	// PaginateBy (paginate_orphans).
	PaginateOrphans int
	// PageKwarg names the path value or query parameter with the page number
	// (page_kwarg, default "page").
	PageKwarg string
	// ContextObjectName is the context key of the list next to "object_list"
	// (context_object_name, default "<model>_list").
	ContextObjectName string
	// TemplateName is the template to render (template_name). The default
	// name stays a second candidate.
	TemplateName string
	// TemplateNameSuffix is the suffix of the default template name
	// (template_name_suffix, default "_list").
	TemplateNameSuffix string
	// ContentType overrides the Content-Type header (content_type).
	ContentType string
	// ExtraContext is added to the context (extra_context).
	ExtraContext map[string]any
	// GetContextData replaces overriding get_context_data. The context it
	// receives holds the list as []T, the page as *Page[T] and the paginator
	// as *Paginator[T].
	GetContextData ContextFunc
	// GetTemplateNames replaces overriding get_template_names.
	GetTemplateNames func(r *http.Request) ([]string, error)
	// GetPaginator replaces overriding get_paginator and setting
	// paginator_class. Nil builds the paginator with NewPaginator.
	GetPaginator func(list ObjectList[T], perPage, orphans int, allowEmptyFirstPage bool) *Paginator[T]
}

// ServeHTTP renders the list for GET and HEAD.
func (v ListView[T]) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	dispatch(w, r, v.Engine, methodHandlers{get: v.get})
}

// objectList returns the list of the request, Django's
// MultipleObjectMixin.get_queryset.
func (v ListView[T]) objectList(r *http.Request) (ObjectList[T], error) {
	var qs *orm.QuerySet[T]
	switch {
	case v.GetQueryset != nil:
		var err error
		if qs, err = v.GetQueryset(r); err != nil {
			return nil, err
		}
		if qs == nil {
			return nil, improperlyConfigured("ListView.GetQueryset returned no queryset")
		}
	case v.Queryset != nil:
		qs = v.Queryset
	case v.ObjectList != nil:
		return v.ObjectList, nil
	case modelFor[T](v.DB) != nil:
		qs = orm.Query[T](v.DB)
	default:
		return nil, improperlyConfigured(
			"ListView is missing a QuerySet. Define ListView.DB with a registered model %s, "+
				"ListView.Queryset or ListView.ObjectList, or implement ListView.GetQueryset",
			modelTypeName[T]())
	}
	if len(v.Ordering) > 0 {
		qs = qs.OrderBy(v.Ordering...)
	}
	return QuerySetList(qs), nil
}

// get loads the list, or one page of it, and renders it.
func (v ListView[T]) get(w http.ResponseWriter, r *http.Request) error {
	list, err := v.objectList(r)
	if err != nil {
		return err
	}
	base, err := v.listContext(r, list)
	if err != nil {
		return err
	}
	data, err := viewContext(r, v, base, v.ExtraContext, v.GetContextData)
	if err != nil {
		return err
	}
	names, err := v.templateNames(r)
	if err != nil {
		return err
	}
	return renderResponse(w, r, v.Engine, names, modelContext[T](v.DB, data), contentTypeOption(v.ContentType))
}

// listContext builds the context of the list, Django's
// MultipleObjectMixin.get_context_data, after the allow_empty check of
// BaseListView.get.
func (v ListView[T]) listContext(r *http.Request, list ObjectList[T]) (map[string]any, error) {
	ctx := r.Context()
	allowEmpty := v.AllowEmpty == nil || *v.AllowEmpty
	out := map[string]any{"paginator": nil, "page_obj": nil, "is_paginated": false}
	var objects []T
	if v.PaginateBy > 0 {
		if !allowEmpty {
			// EXISTS is cheaper than loading the unpaginated list.
			empty, err := isEmpty(ctx, list)
			if err != nil {
				return nil, err
			}
			if empty {
				return nil, NotFound(emptyListMessage)
			}
		}
		paginator, page, err := v.paginate(r, list, allowEmpty)
		if err != nil {
			return nil, err
		}
		out["paginator"], out["page_obj"], out["is_paginated"] = paginator, page, page.HasOtherPages()
		objects = page.ObjectList
	} else {
		var err error
		if objects, err = allObjects(ctx, list); err != nil {
			return nil, err
		}
		if !allowEmpty && len(objects) == 0 {
			return nil, NotFound(emptyListMessage)
		}
	}
	out["object_list"] = objects
	name := v.ContextObjectName
	if name == "" {
		if m := modelFor[T](v.DB); m != nil {
			name = modelName(m) + "_list"
		}
	}
	if name != "" {
		out[name] = objects
	}
	return out, nil
}

// paginate returns the paginator and the requested page, Django's
// MultipleObjectMixin.paginate_queryset.
func (v ListView[T]) paginate(r *http.Request, list ObjectList[T], allowEmpty bool) (*Paginator[T], *Page[T], error) {
	ctx := r.Context()
	var paginator *Paginator[T]
	if v.GetPaginator != nil {
		paginator = v.GetPaginator(list, v.PaginateBy, v.PaginateOrphans, allowEmpty)
	} else {
		paginator = NewPaginator(list, v.PaginateBy,
			WithOrphans(v.PaginateOrphans), WithAllowEmptyFirstPage(allowEmpty))
	}
	if paginator == nil {
		return nil, nil, improperlyConfigured("ListView.GetPaginator returned no paginator")
	}
	if m := modelFor[T](v.DB); m != nil && paginator.TemplateObject == nil {
		paginator.TemplateObject = func(obj T) any { return templates.ModelContext(m, obj) }
	}
	kwarg := orDefault(v.PageKwarg, "page")
	raw := r.PathValue(kwarg)
	if raw == "" {
		raw = r.URL.Query().Get(kwarg)
	}
	number := 1
	if raw != "" {
		n, err := strconv.Atoi(strings.TrimSpace(raw))
		switch {
		case err == nil:
			number = n
		case raw == "last":
			if number, err = paginator.NumPages(ctx); err != nil {
				return nil, nil, err
			}
		default:
			return nil, nil, NotFound("Page is not “last”, nor can it be converted to an int.")
		}
	}
	page, err := paginator.Page(ctx, number)
	if errors.Is(err, ErrInvalidPage) {
		return nil, nil, NotFound(fmt.Sprintf("Invalid page (%d): %s", number, err))
	}
	if err != nil {
		return nil, nil, err
	}
	return paginator, page, nil
}

// templateNames returns the template candidates, Django's
// MultipleObjectTemplateResponseMixin.get_template_names: TemplateName, then
// "<app>/<model><suffix>.html" for a model.
func (v ListView[T]) templateNames(r *http.Request) ([]string, error) {
	if v.GetTemplateNames != nil {
		return v.GetTemplateNames(r)
	}
	var names []string
	if v.TemplateName != "" {
		names = append(names, v.TemplateName)
	}
	if m := modelFor[T](v.DB); m != nil {
		names = append(names, modelTemplateName(m, orDefault(v.TemplateNameSuffix, "_list")))
	}
	if len(names) == 0 {
		return nil, improperlyConfigured(
			"ListView requires either a definition of TemplateName or DB with a registered model %s",
			modelTypeName[T]())
	}
	return names, nil
}
