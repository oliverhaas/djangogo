package views

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/templates"
)

// DetailView renders a single T fetched by its primary key from the URL path.
type DetailView[T any] struct {
	DB          *orm.DB
	Engine      *templates.Engine
	Template    string                             // template name to render
	PKParam     string                             // path value name carrying the pk (default "pk")
	ContextName string                             // template var for the object (default "object")
	Extra       func(*http.Request) map[string]any // optional extra context
}

// ServeHTTP fetches the T whose primary key matches the URL path value and renders
// it through the template. A missing or non-integer pk and a no-row result both
// yield 404; any other lookup or render error yields 500.
func (v DetailView[T]) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	pkParam := v.PKParam
	if pkParam == "" {
		pkParam = "pk"
	}
	// Parse to int64 so the pk filter is type-correct on Postgres (and others).
	pk, err := strconv.ParseInt(r.PathValue(pkParam), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	obj, err := orm.Query[T](v.DB).Get(r.Context(), "id", pk)
	if err != nil {
		if errors.Is(err, orm.ErrDoesNotExist) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	contextName := v.ContextName
	if contextName == "" {
		contextName = "object"
	}
	ctx := mergeExtra(r, v.Extra)
	ctx[contextName] = obj

	_ = Render(w, v.Engine, v.Template, ctx)
}

// ListView renders all T (optionally filtered) into a template.
type ListView[T any] struct {
	DB          *orm.DB
	Engine      *templates.Engine
	Template    string
	ContextName string                         // template var for the slice (default "objects")
	Query       func(*orm.DB) *orm.QuerySet[T] // optional; default Query[T](db)
	Extra       func(*http.Request) map[string]any
}

// ServeHTTP runs the (custom or default) queryset, fetches every row, and renders
// the slice into the template. A query or render error yields 500.
func (v ListView[T]) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	qs := orm.Query[T](v.DB)
	if v.Query != nil {
		qs = v.Query(v.DB)
	}

	objects, err := qs.All(r.Context())
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	contextName := v.ContextName
	if contextName == "" {
		contextName = "objects"
	}
	ctx := mergeExtra(r, v.Extra)
	ctx[contextName] = objects

	_ = Render(w, v.Engine, v.Template, ctx)
}

// mergeExtra returns a fresh context map seeded with the result of extra (when
// non-nil), ready for the view to add its object(s).
func mergeExtra(r *http.Request, extra func(*http.Request) map[string]any) map[string]any {
	ctx := make(map[string]any)
	if extra != nil {
		for k, val := range extra(r) {
			ctx[k] = val
		}
	}
	return ctx
}
