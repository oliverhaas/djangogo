package views

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"

	"github.com/oliverhaas/djangogo/orm"
)

// modelTypeName returns the Go type name of T (without pointer), which is the
// model's object name in Django's "No %s matches the given query." message.
func modelTypeName[T any]() string {
	t := reflect.TypeFor[T]()
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t.Name()
}

// noMatch returns Django's Http404 of get_object_or_404 for model T.
func noMatch[T any]() error {
	return NotFound(fmt.Sprintf("No %s matches the given query.", modelTypeName[T]()))
}

// GetObjectOr404 returns the single object of qs that matches the filter
// pairs, Django's get_object_or_404. When no object matches it returns an
// ErrNotFound with the message "No <Model> matches the given query.", which
// ServeError turns into a 404. Every other error, orm.ErrMultipleObjectsReturned
// included, is returned unchanged.
//
// Django accepts a model class, a manager or a queryset as first argument. In
// Go that is always a queryset: pass orm.Query[T](db) for the whole model.
func GetObjectOr404[T any](ctx context.Context, qs *orm.QuerySet[T], pairs ...any) (T, error) {
	obj, err := qs.Get(ctx, pairs...)
	if errors.Is(err, orm.ErrDoesNotExist) {
		var zero T
		return zero, noMatch[T]()
	}
	return obj, err
}

// GetListOr404 returns the objects of qs that match the filter pairs, Django's
// get_list_or_404. An empty result is an ErrNotFound with the message
// "No <Model> matches the given query.".
func GetListOr404[T any](ctx context.Context, qs *orm.QuerySet[T], pairs ...any) ([]T, error) {
	objs, err := qs.Filter(pairs...).All(ctx)
	if err != nil {
		return nil, err
	}
	if len(objs) == 0 {
		return nil, noMatch[T]()
	}
	return objs, nil
}

// NotAllowed answers with 405 Method Not Allowed and an Allow header listing
// the permitted methods, Django's HttpResponseNotAllowed. The rejected request
// is logged as a warning, as Django does.
func NotAllowed(w http.ResponseWriter, r *http.Request, permitted ...string) {
	logResponse(r, http.StatusMethodNotAllowed,
		fmt.Sprintf("Method Not Allowed (%s): %s", escapeLogValue(r.Method), escapeLogValue(r.URL.Path)))
	w.Header().Set("Allow", strings.Join(permitted, ", "))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusMethodNotAllowed)
}

// RequireMethods returns middleware that lets a request through only when its
// method is one of methods (uppercase, for example "GET", "POST") and answers
// 405 with an Allow header otherwise, Django's require_http_methods decorator.
func RequireMethods(methods ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			for _, m := range methods {
				if r.Method == m {
					next.ServeHTTP(w, r)
					return
				}
			}
			NotAllowed(w, r, methods...)
		})
	}
}

// RequireGET lets only GET requests through, Django's require_GET decorator.
func RequireGET(next http.Handler) http.Handler {
	return RequireMethods(http.MethodGet)(next)
}

// RequirePOST lets only POST requests through, Django's require_POST decorator.
func RequirePOST(next http.Handler) http.Handler {
	return RequireMethods(http.MethodPost)(next)
}

// RequireSafe lets only GET and HEAD requests through, Django's require_safe
// decorator.
func RequireSafe(next http.Handler) http.Handler {
	return RequireMethods(http.MethodGet, http.MethodHead)(next)
}
