package views

import (
	"maps"
	"net/http"
	"strings"

	"github.com/oliverhaas/djangogo/templates"
)

// methodHandlers holds the handler of each HTTP method a view implements. A
// nil entry means the view does not implement the method.
type methodHandlers struct {
	get, post, put, patch, delete, head, options, trace HandlerFunc
}

// methodOrder lists the methods a view can implement in the order of Django's
// View.http_method_names, which is also the order of the Allow header.
var methodOrder = []string{
	http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch,
	http.MethodDelete, http.MethodHead, http.MethodOptions, http.MethodTrace,
}

// handler returns the handler for an HTTP method, or nil when the view does
// not implement it. HEAD falls back to the GET handler and OPTIONS to the
// default answer, as in Django's View.
func (h methodHandlers) handler(method string) HandlerFunc {
	switch method {
	case http.MethodGet:
		return h.get
	case http.MethodPost:
		return h.post
	case http.MethodPut:
		return h.put
	case http.MethodPatch:
		return h.patch
	case http.MethodDelete:
		return h.delete
	case http.MethodHead:
		if h.head != nil {
			return h.head
		}
		return h.get
	case http.MethodOptions:
		if h.options != nil {
			return h.options
		}
		return h.defaultOptions
	case http.MethodTrace:
		return h.trace
	default:
		return nil
	}
}

// allowed returns the methods the view implements, for the Allow header.
func (h methodHandlers) allowed() []string {
	var out []string
	for _, method := range methodOrder {
		if h.handler(method) != nil {
			out = append(out, method)
		}
	}
	return out
}

// defaultOptions answers an OPTIONS request with the allowed methods, Django's
// View.options.
func (h methodHandlers) defaultOptions(w http.ResponseWriter, _ *http.Request) error {
	w.Header().Set("Allow", strings.Join(h.allowed(), ", "))
	w.Header().Set("Content-Length", "0")
	w.WriteHeader(http.StatusOK)
	return nil
}

// dispatch runs the handler for the request method, Django's View.dispatch. A
// method without handler gets a 405 with an Allow header. An error returned by
// the handler is answered by ServeError, with e rendering the error templates.
// Every generic view serves its requests through dispatch.
func dispatch(w http.ResponseWriter, r *http.Request, e *templates.Engine, h methodHandlers) {
	fn := h.handler(r.Method)
	if fn == nil {
		NotAllowed(w, r, h.allowed()...)
		return
	}
	if err := fn(w, r); err != nil {
		ServeError(w, r, e, err)
	}
}

// View dispatches a request to the handler of its HTTP method, Django's View
// class. Set a field for each method the view implements. A request with any
// other method gets 405 Method Not Allowed with an Allow header. HEAD is served
// by Get unless Head is set, and OPTIONS answers with the allowed methods
// unless Options is set.
//
//	mux.Handle("/contact/", views.View{Engine: engine, Get: showForm, Post: sendMessage})
//
// Mount the view on a route pattern without a method ("/contact/", not
// "GET /contact/"), otherwise the router answers the other methods itself.
type View struct {
	// Engine renders the error templates for errors the handlers return. It
	// may be nil.
	Engine *templates.Engine
	// Get handles GET, and HEAD when Head is nil.
	Get HandlerFunc
	// Post handles POST.
	Post HandlerFunc
	// Put handles PUT.
	Put HandlerFunc
	// Patch handles PATCH.
	Patch HandlerFunc
	// Delete handles DELETE.
	Delete HandlerFunc
	// Head handles HEAD.
	Head HandlerFunc
	// Options handles OPTIONS.
	Options HandlerFunc
	// Trace handles TRACE.
	Trace HandlerFunc
}

// ServeHTTP dispatches the request by its method.
func (v View) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	dispatch(w, r, v.Engine, methodHandlers{
		get: v.Get, post: v.Post, put: v.Put, patch: v.Patch,
		delete: v.Delete, head: v.Head, options: v.Options, trace: v.Trace,
	})
}

// ContextFunc is the GetContextData hook of the generic views, the Go form of
// overriding get_context_data: it receives the context the view has built
// (object, object_list, view, the extra context ...) and returns the context
// to render. Returning the map it was given after changing it is fine.
type ContextFunc func(r *http.Request, ctx map[string]any) (map[string]any, error)

// viewContext finishes the context of a generic view, Django's
// ContextMixin.get_context_data: it adds the view under "view" unless base
// has that key, lets extra override entries, and hands the result to the
// GetContextData hook, which has the last word. base is not modified.
func viewContext(r *http.Request, view any, base, extra map[string]any, hook ContextFunc) (map[string]any, error) {
	ctx := make(map[string]any, len(base)+len(extra)+1)
	maps.Copy(ctx, base)
	if _, ok := ctx["view"]; !ok {
		ctx["view"] = view
	}
	maps.Copy(ctx, extra)
	if hook == nil {
		return ctx, nil
	}
	return hook(r, ctx)
}

// pathValue is one wildcard of a route pattern with its value in the request.
type pathValue struct {
	name, value string
}

// pathValues returns the wildcards of the route pattern that matched r with
// their values, in pattern order. They are the counterpart of the keyword
// arguments Django's URL resolver passes to a view. A request that was not
// routed by a pattern has none.
func pathValues(r *http.Request) []pathValue {
	var out []pathValue
	rest := r.Pattern
	for {
		start := strings.IndexByte(rest, '{')
		if start < 0 {
			return out
		}
		end := strings.IndexByte(rest[start:], '}')
		if end < 0 {
			return out
		}
		name := strings.TrimSuffix(rest[start+1:start+end], "...")
		rest = rest[start+end+1:]
		if name == "" || name == "$" {
			continue
		}
		out = append(out, pathValue{name: name, value: r.PathValue(name)})
	}
}

// contentTypeOption returns the response options for the ContentType field of
// a generic view: none for "", which keeps text/html.
func contentTypeOption(contentType string) []ResponseOption {
	if contentType == "" {
		return nil
	}
	return []ResponseOption{WithContentType(contentType)}
}

// baseTemplateNames returns the template candidates of a view that renders one
// named template, Django's TemplateResponseMixin.get_template_names. view is
// the type name used in the configuration error.
func baseTemplateNames(view, templateName string) ([]string, error) {
	if templateName == "" {
		return nil, improperlyConfigured(
			"%s requires either a definition of TemplateName or an implementation of GetTemplateNames", view)
	}
	return []string{templateName}, nil
}
