package urls

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"
)

// Router builds an http.ServeMux from routes and resolves named routes back to URLs.
//
// Duplicate route names cause NewRouter to panic (it is a boot-time programming
// error). Use NewRouterChecked if you need a graceful error instead.
type Router struct {
	mux    *http.ServeMux
	byName map[string]string // name -> pattern (with optional METHOD prefix)
}

// NewRouter registers routes into a fresh ServeMux and indexes their names.
// It panics if two routes share the same non-empty name.
func NewRouter(routes ...Route) *Router {
	r, err := NewRouterChecked(routes...)
	if err != nil {
		panic(fmt.Sprintf("urls: NewRouter: %v", err))
	}
	return r
}

// NewRouterChecked is like NewRouter but returns an error instead of panicking
// on a duplicate route name.
func NewRouterChecked(routes ...Route) (*Router, error) {
	mux := http.NewServeMux()
	byName := make(map[string]string, len(routes))
	for _, route := range routes {
		mux.Handle(route.Pattern, route.Handler)
		if route.Name != "" {
			if _, exists := byName[route.Name]; exists {
				return nil, fmt.Errorf("urls: duplicate route name %q", route.Name)
			}
			byName[route.Name] = route.Pattern
		}
	}
	return &Router{mux: mux, byName: byName}, nil
}

// ServeMux returns the underlying *http.ServeMux.
func (r *Router) ServeMux() *http.ServeMux {
	return r.mux
}

// ServeHTTP delegates to the underlying ServeMux, allowing Router to be used
// directly as an http.Handler.
func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	r.mux.ServeHTTP(w, req)
}

// placeholderRe matches {name} and {name...} wildcards in a ServeMux pattern.
var placeholderRe = regexp.MustCompile(`\{[^{}]+\}`)

// Reverse builds the URL for a named route, substituting {param} placeholders
// with args in order. It returns an error for an unknown name or an arg-count
// mismatch.
func (r *Router) Reverse(name string, args ...any) (string, error) {
	pattern, ok := r.byName[name]
	if !ok {
		return "", fmt.Errorf("urls: reverse: unknown route name %q", name)
	}
	// Strip optional leading METHOD token to get the path only.
	_, path := splitMethodPath(pattern)

	// Collect placeholder positions.
	matches := placeholderRe.FindAllStringIndex(path, -1)
	if len(matches) != len(args) {
		return "", fmt.Errorf(
			"urls: reverse %q: expected %d arg(s), got %d",
			name, len(matches), len(args),
		)
	}

	// Replace placeholders left to right.
	var sb strings.Builder
	pos := 0
	for i, loc := range matches {
		sb.WriteString(path[pos:loc[0]])
		fmt.Fprint(&sb, args[i])
		pos = loc[1]
	}
	sb.WriteString(path[pos:])
	return sb.String(), nil
}
