// Package urls provides route declaration, a ServeMux-backed router, and
// named-URL reverse resolution for Djan-Go-Go applications.
//
// Routes are declared with Path or PathFunc and composed with Include. A Router
// registers them into a net/http.ServeMux and allows reversing named routes back
// to URL paths.
package urls

import (
	"net/http"
	"strings"
)

// Route is a single URL pattern bound to a handler, with an optional name for reverse.
type Route struct {
	// Pattern is a ServeMux pattern, e.g. "GET /articles/{id}/" or "/about/".
	Pattern string
	// Name is the optional reverse name, e.g. "article-detail".
	Name    string
	Handler http.Handler
}

// Path declares a route. pattern is a ServeMux pattern (may include a leading METHOD).
func Path(pattern string, h http.Handler, name string) Route {
	return Route{Pattern: pattern, Name: name, Handler: h}
}

// PathFunc is Path for an http.HandlerFunc.
func PathFunc(pattern string, h http.HandlerFunc, name string) Route {
	return Route{Pattern: pattern, Name: name, Handler: h}
}

// splitMethodPath splits a ServeMux pattern into its optional METHOD prefix and
// the path part. If the pattern has no method prefix the method is empty.
// Example: "GET /articles/{id}/" -> ("GET", "/articles/{id}/")
//
//	"/about/" -> ("", "/about/")
func splitMethodPath(pattern string) (method, path string) {
	// A method prefix is an all-uppercase token followed by a space before the path.
	idx := strings.IndexByte(pattern, ' ')
	if idx < 0 {
		return "", pattern
	}
	tok := pattern[:idx]
	rest := pattern[idx+1:]
	if strings.HasPrefix(rest, "/") {
		return tok, rest
	}
	// No leading slash after the space -- treat the whole thing as a path.
	return "", pattern
}

// joinPrefixPath joins prefix in front of path, normalising to a single leading
// slash and no doubled slashes at the join point.
//
// prefix examples: "blog/", "blog", "" -- path always starts with "/".
func joinPrefixPath(prefix, path string) string {
	// Ensure the prefix has no leading slash and a trailing slash.
	p := strings.TrimLeft(prefix, "/")
	if p != "" && !strings.HasSuffix(p, "/") {
		p += "/"
	}
	// Ensure the path has a leading slash.
	pt := strings.TrimLeft(path, "/")
	return "/" + p + pt
}

// Include returns copies of routes with their path prefixed by prefix and their
// names namespaced as "<namespace>:<name>" (when namespace is non-empty and the
// route already has a name). prefix is a path segment like "blog/" (no leading
// slash needed). Method prefixes on the inner routes are preserved.
func Include(prefix, namespace string, routes ...Route) []Route {
	out := make([]Route, len(routes))
	for i, r := range routes {
		method, path := splitMethodPath(r.Pattern)
		newPath := joinPrefixPath(prefix, path)
		var newPattern string
		if method != "" {
			newPattern = method + " " + newPath
		} else {
			newPattern = newPath
		}
		name := r.Name
		if r.Name != "" && namespace != "" {
			name = namespace + ":" + r.Name
		}
		out[i] = Route{Pattern: newPattern, Name: name, Handler: r.Handler}
	}
	return out
}
