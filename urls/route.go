// Package urls is the URL dispatcher of Djan-Go-Go: route declaration, an
// ordered router, and named-URL reverse resolution, modeled on django.urls.
//
// Routes are declared with Path, PathFunc, RePath and RePathFunc and composed
// with Include, IncludeApp and IncludeWith. A Router matches them in the order
// they were declared (the first match wins), hands the handler its typed
// arguments, and turns a route name back into a URL with Reverse.
//
// # Route syntax
//
// Path accepts two spellings and tells them apart by the leading slash:
//
//   - Django routes: "articles/<int:year>/<slug:slug>/". No leading slash. The
//     route must match the whole path. <name> is short for <str:name>.
//   - ServeMux patterns: "GET /articles/{id}/". A leading slash, {name} for a
//     path segment, {name...} for the rest of the path and {$} for the end of
//     the path. As in net/http, a pattern that ends in a slash also covers the
//     paths below it, but only when no route matches the path exactly.
//
// Either spelling may use the parameters of the other, and either may start
// with an HTTP method ("POST articles/<int:pk>/delete/"). A route with a
// method answers other methods with 405 Method Not Allowed unless a later
// route takes the path; GET routes also serve HEAD.
//
// RePath takes a regular expression in the syntax of Go's regexp package.
// Django expressions carry over except for lookaround and backreferences,
// which RE2 does not have, and \w, \d and \s, which match ASCII only.
//
// # Converters
//
// The converters are Django's: str, int, slug, uuid and path, plus the ones
// added with RegisterConverter. A value the converter rejects makes the route
// not match, which ends in a 404 rather than a call to the handler.
//
// # Arguments
//
// Captured text stays available through http.Request.PathValue. The converted
// values, the route name and the namespaces are in the ResolverMatch that
// Match returns for the request; Param, IntParam and StringParam read single
// values from it.
package urls

import (
	"maps"
	"net/http"
	"strings"
)

// Kwargs are keyword arguments: the named values a route captures, the extra
// values a route passes to its handler, and the named arguments of Reverse.
type Kwargs map[string]any

// Route is a single URL pattern bound to a handler, with an optional name for
// reverse.
type Route struct {
	// Pattern is the route as declared, for example "articles/<int:year>/" or
	// "GET /articles/{id}/". For a route returned by Include it is the include
	// prefix joined with the route.
	Pattern string
	// Name is the optional reverse name, for example "article-detail". For a
	// route returned by a namespaced Include it carries the namespace, as in
	// "blog:detail".
	Name string
	// Handler serves the requests the route matches.
	Handler http.Handler

	// spec carries what the exported fields cannot: the syntax, the extra
	// keyword arguments and the includes the route passed through. It is nil
	// for a Route built as a struct literal, which is then read as a Path.
	spec *routeSpec
}

// routeSpec is the declaration behind a Route.
type routeSpec struct {
	// regex marks a RePath route.
	regex bool
	// raw is the pattern given to Path or RePath.
	raw string
	// name is the route's own name, without namespaces.
	name string
	// kwargs are the extra keyword arguments of the route.
	kwargs Kwargs
	// chain lists the includes the route passed through, outermost first.
	chain []*includeSpec
	// flat is the Pattern the constructor wrote, kept to notice a later edit
	// of the exported field.
	flat string
}

// includeSpec is one Include call. Routes that came through the same call
// share one includeSpec, which is how the router puts them back under one
// resolver.
type includeSpec struct {
	// prefix is the route or expression in front of the included routes.
	prefix string
	// regex marks prefix as a regular expression.
	regex bool
	// appName is the application namespace, empty for a plain include.
	appName string
	// namespace is the instance namespace, empty for a plain include.
	namespace string
	// kwargs are passed to every included handler.
	kwargs Kwargs
}

// Path declares a route, Django's path(). pattern is a Django route
// ("articles/<int:year>/") or a ServeMux pattern ("GET /articles/{id}/"); see
// the package documentation. name may be empty.
func Path(pattern string, h http.Handler, name string) Route {
	return Route{
		Pattern: pattern, Name: name, Handler: h,
		spec: &routeSpec{raw: pattern, name: name, flat: pattern},
	}
}

// PathFunc is Path for an http.HandlerFunc.
func PathFunc(pattern string, h http.HandlerFunc, name string) Route {
	return Path(pattern, h, name)
}

// RePath declares a route matched by a regular expression, Django's
// re_path(). Named groups reach the handler as keyword arguments; an
// expression without named groups passes its groups as positional arguments.
// The expression is searched for at the start of the remaining path, so it
// should begin with ^ and, unless it is meant as a prefix, end with $.
func RePath(expr string, h http.Handler, name string) Route {
	return Route{
		Pattern: expr, Name: name, Handler: h,
		spec: &routeSpec{regex: true, raw: expr, name: name, flat: expr},
	}
}

// RePathFunc is RePath for an http.HandlerFunc.
func RePathFunc(expr string, h http.HandlerFunc, name string) Route {
	return RePath(expr, h, name)
}

// WithKwargs returns a copy of the route that passes kwargs to its handler in
// addition to the captured arguments, the third argument of Django's path().
// Extra arguments win over captured ones of the same name. The handler reads
// them with Param or from the ResolverMatch.
func (r Route) WithKwargs(kwargs Kwargs) Route {
	spec := r.specOrDefault()
	clone := *spec
	clone.kwargs = make(Kwargs, len(spec.kwargs)+len(kwargs))
	maps.Copy(clone.kwargs, spec.kwargs)
	maps.Copy(clone.kwargs, kwargs)
	r.spec = &clone
	return r
}

// specOrDefault returns the declaration of the route. A Route built as a
// struct literal, or one whose Pattern or Name was edited after Path made it,
// is read from its exported fields.
func (r Route) specOrDefault() *routeSpec {
	if r.spec == nil {
		return &routeSpec{raw: r.Pattern, name: r.Name, flat: r.Pattern}
	}
	if len(r.spec.chain) == 0 && (r.spec.flat != r.Pattern || r.spec.name != r.Name) {
		clone := *r.spec
		clone.raw, clone.flat, clone.name = r.Pattern, r.Pattern, r.Name
		return &clone
	}
	return r.spec
}

// IncludeOptions configures IncludeWith.
type IncludeOptions struct {
	// Prefix is the route in front of the included routes, for example
	// "blog/" or "users/<int:user_id>/". It is used as written: unlike
	// Include, IncludeWith does not add a trailing slash.
	Prefix string
	// Regex makes Prefix a regular expression, as when Django's re_path()
	// wraps include().
	Regex bool
	// AppName is the application namespace (Django's app_name). It defaults
	// to Namespace.
	AppName string
	// Namespace is the instance namespace. It defaults to AppName. With both
	// empty the included names are not namespaced.
	Namespace string
	// Kwargs are passed to every included handler.
	Kwargs Kwargs
}

// Include returns routes mounted under prefix, Django's
// path(prefix, include(...)). prefix is a route like "blog/" or
// "users/<int:user_id>/"; a missing trailing slash is added. A non-empty
// namespace becomes both the application and the instance namespace, so the
// included names reverse as "<namespace>:<name>".
//
// The returned routes carry the joined pattern and the qualified name in
// their exported fields. Pass them to NewRouter or to another Include.
func Include(prefix, namespace string, routes ...Route) []Route {
	return IncludeApp(prefix, namespace, namespace, routes...)
}

// IncludeApp is Include with separate application and instance namespaces,
// Django's include((patterns, app_name), namespace=namespace). Mounting one
// application twice under different instance namespaces lets Reverse pick the
// instance through ReverseOptions.CurrentApp. An empty namespace defaults to
// appName and the other way around.
func IncludeApp(prefix, appName, namespace string, routes ...Route) []Route {
	p := strings.TrimLeft(prefix, "/")
	if p != "" && !strings.HasSuffix(p, "/") {
		p += "/"
	}
	return IncludeWith(IncludeOptions{Prefix: p, AppName: appName, Namespace: namespace}, routes...)
}

// IncludeWith is the general form of Include: it also takes a regular
// expression as the prefix and extra keyword arguments for the included
// handlers.
func IncludeWith(opts IncludeOptions, routes ...Route) []Route {
	inc := &includeSpec{
		prefix:    opts.Prefix,
		regex:     opts.Regex,
		appName:   opts.AppName,
		namespace: opts.Namespace,
		kwargs:    maps.Clone(opts.Kwargs),
	}
	if !inc.regex {
		inc.prefix = strings.TrimLeft(inc.prefix, "/")
	}
	if inc.namespace == "" {
		inc.namespace = inc.appName
	}
	if inc.appName == "" {
		inc.appName = inc.namespace
	}
	out := make([]Route, len(routes))
	for i, r := range routes {
		spec := *r.specOrDefault()
		spec.chain = append([]*includeSpec{inc}, spec.chain...)
		name := r.Name
		if name != "" && inc.namespace != "" {
			name = inc.namespace + ":" + name
		}
		spec.flat = joinPattern(inc, r.Pattern, r.spec != nil && r.spec.regex)
		out[i] = Route{Pattern: spec.flat, Name: name, Handler: r.Handler, spec: &spec}
	}
	return out
}

// joinPattern builds the Pattern shown for an included route: the include
// prefix in front of the route, in the spelling of the route.
func joinPattern(inc *includeSpec, pattern string, regex bool) string {
	if inc.regex || regex {
		return inc.prefix + strings.TrimPrefix(pattern, "^")
	}
	method, path := splitMethodPath(pattern)
	joined := inc.prefix + strings.TrimLeft(path, "/")
	if strings.HasPrefix(path, "/") {
		joined = "/" + joined
	}
	if method != "" {
		return method + " " + joined
	}
	return joined
}
