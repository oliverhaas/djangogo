package urls

import (
	"context"
	"net/http"
	"strings"
)

// ResolverMatch describes the route a path resolved to, the counterpart of
// django.urls.ResolverMatch. Router.Resolve returns one, and Match returns the
// one of the request being served.
//
// The maps and slices of a match that came from a request are shared with the
// router and must not be modified.
type ResolverMatch struct {
	// Handler is the handler of the matched route (Django's func).
	Handler http.Handler
	// Args are the positional arguments: the unnamed groups of a RePath
	// expression that has no named groups. A group that did not take part in
	// the match is nil.
	Args []any
	// Kwargs are the keyword arguments: the converted captures of the route
	// and of the includes above it, together with the extra arguments.
	Kwargs Kwargs
	// URLName is the name of the matched route, without namespaces.
	URLName string
	// AppNames are the application namespaces of the includes the route sits
	// in, outermost first.
	AppNames []string
	// Namespaces are the instance namespaces of those includes, outermost
	// first.
	Namespaces []string
	// Route is the pattern of the matched route with its include prefixes, for
	// example "blog/<int:year>/".
	Route string
	// ViewName is the name to pass to Reverse: the namespaces and URLName
	// joined with colons. It is empty for a route without a name, where Django
	// would give the dotted path of the view function.
	ViewName string
	// CapturedKwargs are the keyword arguments captured by the route itself.
	CapturedKwargs Kwargs
	// ExtraKwargs are the keyword arguments given to the route or to its
	// includes instead of being captured.
	ExtraKwargs Kwargs
	// Tried lists the patterns that were tried up to and including the match.
	// Only Router.Resolve fills it.
	Tried [][]TriedPattern

	// raw holds the captured text of the named groups for PathValue.
	raw []rawValue
}

// TriedPattern is one step on the way to a route: an include prefix or the
// route itself. A debug 404 page prints the steps of each tried chain.
type TriedPattern struct {
	// Pattern is the route or expression as it was declared.
	Pattern string
	// Name is the name of a route; empty for an unnamed route and for an
	// include.
	Name string
	// Include marks an include prefix (Django's URLResolver) as opposed to a
	// route (Django's URLPattern).
	Include bool
}

// String formats the step as Django's debug 404 page does: the pattern,
// followed by [name='...'] for a named route.
func (t TriedPattern) String() string {
	if t.Name == "" {
		return t.Pattern
	}
	return t.Pattern + " [name='" + t.Name + "']"
}

// triedPattern describes the node for a tried list.
func (n *node) triedPattern() TriedPattern {
	if n.leaf == nil {
		return TriedPattern{Pattern: n.pat.display, Include: true}
	}
	return TriedPattern{Pattern: n.pat.display, Name: n.leaf.name}
}

// AppName returns the application namespaces joined with colons (Django's
// app_name).
func (m *ResolverMatch) AppName() string { return strings.Join(m.AppNames, ":") }

// Namespace returns the instance namespaces joined with colons (Django's
// namespace).
func (m *ResolverMatch) Namespace() string { return strings.Join(m.Namespaces, ":") }

// matchKey is the context key the router stores the match under.
type matchKey struct{}

// Match returns the ResolverMatch of the request, Django's
// request.resolver_match. It is nil for a request that did not come through a
// Router.
func Match(r *http.Request) *ResolverMatch {
	m, _ := r.Context().Value(matchKey{}).(*ResolverMatch)
	return m
}

// WithMatch returns a context that carries m, for code that serves a handler
// outside a Router (a test, or a dispatcher of its own) and wants Match and
// Param to work.
func WithMatch(ctx context.Context, m *ResolverMatch) context.Context {
	return context.WithValue(ctx, matchKey{}, m)
}

// Param returns the converted value of a keyword argument of the request's
// route: an int for <int:year>, a UUID for <uuid:id>, a string for <str:name>
// and {name}, or the value given as an extra argument. It is nil when the
// route has no such argument.
func Param(r *http.Request, name string) any {
	m := Match(r)
	if m == nil {
		return nil
	}
	return m.Kwargs[name]
}

// ParamAs returns the keyword argument name of the request's route as a T. ok
// is false when the route has no such argument or it has another type.
func ParamAs[T any](r *http.Request, name string) (value T, ok bool) {
	value, ok = Param(r, name).(T)
	return value, ok
}

// IntParam returns an argument captured by the int converter.
func IntParam(r *http.Request, name string) (int, bool) {
	return ParamAs[int](r, name)
}

// StringParam returns an argument captured as text: by the str, slug or path
// converter, by {name}, or by a named group of a RePath expression.
func StringParam(r *http.Request, name string) (string, bool) {
	return ParamAs[string](r, name)
}

// UUIDParam returns an argument captured by the uuid converter.
func UUIDParam(r *http.Request, name string) (UUID, bool) {
	return ParamAs[UUID](r, name)
}

// Args returns the positional arguments of the request's route, the unnamed
// groups of a RePath expression.
func Args(r *http.Request) []any {
	m := Match(r)
	if m == nil {
		return nil
	}
	return m.Args
}
