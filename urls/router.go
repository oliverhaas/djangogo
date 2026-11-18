package urls

import (
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"
)

// Router matches request paths against routes in declaration order and
// resolves route names back to URLs. It is the counterpart of Django's root
// URLconf: an http.Handler that dispatches to the first matching route.
//
// The exported fields configure what happens when no route matches. Set them
// before the router serves requests.
type Router struct {
	// NotFoundHandler answers requests no route matches (Django's handler404).
	// The default writes a plain 404.
	NotFoundHandler http.Handler
	// MethodNotAllowedHandler answers requests whose path matches routes that
	// are declared for other methods. The Allow header is set before it runs.
	// The default writes a plain 405.
	MethodNotAllowedHandler http.Handler
	// AppendSlash redirects a path that matches no route to the same path with
	// a trailing slash when that one matches, Django's APPEND_SLASH. NewRouter
	// turns it on, as Django does by default.
	AppendSlash bool
	// Debug makes the slash redirect refuse requests that would lose their
	// body (POST, PUT, PATCH, DELETE) with Django's explanation instead of
	// redirecting them, as Django does when DEBUG is on.
	Debug bool

	// root is the top of the URLconf tree, Django's resolver for "^/".
	root *node

	// mu guards revCache.
	mu sync.RWMutex
	// revCache holds the prepared reverse candidates per namespace path and
	// route name.
	revCache map[string][]*possibility

	// muxOnce guards mux.
	muxOnce sync.Once
	// mux is the ServeMux handed out by ServeMux.
	mux *http.ServeMux
}

// NewRouter builds a Router from routes. It panics when a route is declared
// wrongly (an unknown converter, an expression that does not compile), which
// is a programming error found at start-up. Use NewRouterChecked to get the
// error instead.
//
// Routes may share a name, as in Django: Reverse picks the one that fits its
// arguments.
func NewRouter(routes ...Route) *Router {
	r, err := NewRouterChecked(routes...)
	if err != nil {
		panic(fmt.Sprintf("urls: NewRouter: %v", err))
	}
	return r
}

// NewRouterChecked is NewRouter returning an *ImproperlyConfigured error
// instead of panicking.
func NewRouterChecked(routes ...Route) (*Router, error) {
	rootPat, err := compileRoute("/", false, nil)
	if err != nil {
		return nil, err
	}
	specs := make([]*routeSpec, len(routes))
	for i, r := range routes {
		specs[i] = r.specOrDefault()
	}
	root := &node{pat: rootPat}
	root.children, err = buildNodes(routes, specs, 0, buildContext{converters: allConverters()})
	if err != nil {
		return nil, err
	}
	root.populate()
	return &Router{AppendSlash: true, root: root, revCache: map[string][]*possibility{}}, nil
}

// ServeMux returns an http.ServeMux that serves the router.
//
// Deprecated: the router no longer wraps a ServeMux. Use the Router itself,
// which is an http.Handler.
func (rt *Router) ServeMux() *http.ServeMux {
	rt.muxOnce.Do(func() {
		rt.mux = http.NewServeMux()
		rt.mux.Handle("/", rt)
	})
	return rt.mux
}

// Resolve returns the route path resolves to, Django's resolve(). path is a
// decoded URL path with its leading slash. The request method plays no part.
// The error is a *Resolver404 when no route matches.
func (rt *Router) Resolve(path string) (*ResolverMatch, error) {
	st := resolveState{collect: true}
	m, tried, _ := rt.root.resolve(&st, path)
	if m == nil {
		st.partial = true
		var partialTried [][]TriedPattern
		if m, partialTried, _ = rt.root.resolve(&st, path); m != nil {
			tried = partialTried
		}
	}
	if m == nil {
		return nil, &Resolver404{Path: path, Tried: tried}
	}
	m.Tried = tried
	return m, nil
}

// IsValidPath reports whether path resolves to a route, Django's
// is_valid_path().
func (rt *Router) IsValidPath(path string) bool {
	return rt.lookup("", path) != nil
}

// lookup finds the route for a path without recording the tried patterns.
// An empty method skips the method check.
func (rt *Router) lookup(method, path string) *ResolverMatch {
	m, _ := rt.lookupAllowed(method, path)
	return m
}

// lookupAllowed is lookup that also returns the methods of the routes that
// matched the path but were declared for other methods.
func (rt *Router) lookupAllowed(method, path string) (*ResolverMatch, []string) {
	st := resolveState{method: method}
	if m, _, _ := rt.root.resolve(&st, path); m != nil {
		return m, nil
	}
	if len(st.allowed) > 0 {
		return nil, st.allowed
	}
	st.partial = true
	m, _, _ := rt.root.resolve(&st, path)
	return m, st.allowed
}

// ServeHTTP dispatches the request to the first route that matches its path
// and method. The handler finds the ResolverMatch with Match and the captured
// text with http.Request.PathValue.
func (rt *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	path := req.URL.Path
	m, allowed := rt.lookupAllowed(req.Method, path)
	if m != nil {
		req = req.WithContext(WithMatch(req.Context(), m))
		for _, rv := range m.raw {
			req.SetPathValue(rv.name, rv.value)
		}
		m.Handler.ServeHTTP(w, req)
		return
	}
	if len(allowed) > 0 {
		rt.methodNotAllowed(w, req, allowed)
		return
	}
	if rt.AppendSlash && !strings.HasSuffix(path, "/") && rt.shouldAppendSlash(path) {
		rt.redirectWithSlash(w, req)
		return
	}
	if rt.NotFoundHandler != nil {
		rt.NotFoundHandler.ServeHTTP(w, req)
		return
	}
	http.NotFound(w, req)
}

// SlashOptOut is implemented by a handler that does not want requests for its
// URL without the trailing slash to be redirected, the counterpart of
// Django's no_append_slash decorator.
type SlashOptOut interface {
	// ShouldAppendSlash reports whether the slash redirect applies.
	ShouldAppendSlash() bool
}

// noAppendSlash is the handler NoAppendSlash returns.
type noAppendSlash struct{ http.Handler }

// ShouldAppendSlash opts the handler out of the slash redirect.
func (noAppendSlash) ShouldAppendSlash() bool { return false }

// NoAppendSlash wraps a handler so that a request for its URL without the
// trailing slash gets a 404 instead of the redirect, Django's
// @no_append_slash. Use it for endpoints where a redirect would hide a
// client mistake, such as API endpoints.
func NoAppendSlash(h http.Handler) http.Handler { return noAppendSlash{h} }

// shouldAppendSlash reports whether path, which matched no route, matches one
// once a slash is appended. It is the second half of
// CommonMiddleware.should_redirect_with_slash.
func (rt *Router) shouldAppendSlash(path string) bool {
	m := rt.lookup("", path+"/")
	if m == nil {
		return false
	}
	if opt, ok := m.Handler.(SlashOptOut); ok {
		return opt.ShouldAppendSlash()
	}
	return true
}

// redirectWithSlash answers with a permanent redirect to the request URL with
// a slash appended to its path, as CommonMiddleware does. The target is built
// from the path as the client sent it, and leading double slashes are escaped
// so that the redirect cannot leave the site.
func (rt *Router) redirectWithSlash(w http.ResponseWriter, req *http.Request) {
	target := escapeLeadingSlashes(req.URL.EscapedPath() + "/")
	if req.URL.RawQuery != "" {
		target += "?" + req.URL.RawQuery
	}
	if rt.Debug && slices.Contains([]string{"DELETE", "POST", "PUT", "PATCH"}, req.Method) {
		msg := fmt.Sprintf(
			"You called this URL via %[1]s, but the URL doesn't end in a slash and you have "+
				"APPEND_SLASH set. Django can't redirect to the slash URL while maintaining %[1]s data. "+
				"Change your form to point to %[2]s (note the trailing slash), or set "+
				"APPEND_SLASH=False in your Django settings.",
			req.Method, req.Host+target)
		http.Error(w, msg, http.StatusInternalServerError)
		return
	}
	// http.Redirect would resolve the target against the request path; the
	// target is already absolute, so the header is written as is.
	w.Header().Set("Location", target)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusMovedPermanently)
}

// methodNotAllowed answers a request whose path only matches routes for other
// methods.
func (rt *Router) methodNotAllowed(w http.ResponseWriter, req *http.Request, allowed []string) {
	methods := slices.Clone(allowed)
	if slices.Contains(methods, http.MethodGet) {
		methods = append(methods, http.MethodHead)
	}
	slices.Sort(methods)
	w.Header().Set("Allow", strings.Join(slices.Compact(methods), ", "))
	if rt.MethodNotAllowedHandler != nil {
		rt.MethodNotAllowedHandler.ServeHTTP(w, req)
		return
	}
	http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
}
