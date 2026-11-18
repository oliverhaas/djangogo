package urls

import (
	"maps"
	"net/http"
	"slices"
	"strings"
)

// node is one entry of the URLconf tree: a route (Django's URLPattern) when
// leaf is set, otherwise an include (Django's URLResolver).
type node struct {
	// pat matches the part of the path this node is responsible for.
	pat *compiledPattern
	// leaf holds the route data; nil for an include.
	leaf *leafInfo

	// children are the routes and includes below an include, in declaration
	// order.
	children []*node
	// appName and namespace are the namespaces of an include; both are empty
	// for a plain include.
	appName, namespace string
	// kwargs are the extra keyword arguments of an include.
	kwargs Kwargs
	// rev is the reverse index of an include.
	rev *reverseIndex
}

// leafInfo is what a route contributes to a match. Everything that does not
// depend on the request path is computed once, when the router is built.
type leafInfo struct {
	// handler serves the route.
	handler http.Handler
	// name is the route's own name.
	name string
	// kwargs are the extra keyword arguments of the route itself.
	kwargs Kwargs
	// methods lists the methods the route answers; nil means all.
	methods []string
	// subtree marks a ServeMux pattern with a trailing slash, which also
	// matches longer paths when nothing matches exactly.
	subtree bool
	// mustCopy says that the keyword arguments of a match get entries beyond
	// the captured ones, so they need a map of their own.
	mustCopy bool

	// route, appNames, namespaces, viewName and extraKwargs are the fields of
	// the same name of the ResolverMatch.
	route       string
	appNames    []string
	namespaces  []string
	viewName    string
	extraKwargs Kwargs
}

// allows reports whether the route answers method.
func (l *leafInfo) allows(method string) bool {
	if l.methods == nil {
		return true
	}
	for _, m := range l.methods {
		if m == method || (m == http.MethodGet && method == http.MethodHead) {
			return true
		}
	}
	return false
}

// buildContext is what an include passes down to the routes below it while
// the tree is built.
type buildContext struct {
	// converters is the converter set the router was built with.
	converters map[string]Converter
	// chain lists the include nodes above, outermost first.
	chain []*node
}

// buildNodes turns the flat route list back into a tree. Routes that came
// through the same Include call and follow each other share one include node.
func buildNodes(routes []Route, specs []*routeSpec, depth int, bc buildContext) ([]*node, error) {
	var out []*node
	for i := 0; i < len(routes); {
		spec := specs[i]
		if len(spec.chain) <= depth {
			n, err := buildLeaf(routes[i], spec, bc)
			if err != nil {
				return nil, err
			}
			out = append(out, n)
			i++
			continue
		}
		inc := spec.chain[depth]
		j := i
		for j < len(routes) && len(specs[j].chain) > depth && specs[j].chain[depth] == inc {
			j++
		}
		n, err := buildInclude(inc, bc)
		if err != nil {
			return nil, err
		}
		sub := bc
		sub.chain = append(slices.Clone(bc.chain), n)
		n.children, err = buildNodes(routes[i:j], specs[i:j], depth+1, sub)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
		i = j
	}
	return out, nil
}

// buildInclude compiles the prefix of one Include call.
func buildInclude(inc *includeSpec, bc buildContext) (*node, error) {
	var pat *compiledPattern
	var err error
	if inc.regex {
		pat, err = compileRegex(inc.prefix)
	} else {
		pat, err = compileRoute(inc.prefix, false, bc.converters)
	}
	if err != nil {
		return nil, err
	}
	return &node{pat: pat, appName: inc.appName, namespace: inc.namespace, kwargs: inc.kwargs}, nil
}

// buildLeaf compiles one route and precomputes the request-independent part
// of its matches.
func buildLeaf(r Route, spec *routeSpec, bc buildContext) (*node, error) {
	if r.Handler == nil {
		return nil, configErrorf("urls: route %q has no handler", r.Pattern)
	}
	lf := &leafInfo{handler: r.Handler, name: spec.name, kwargs: spec.kwargs}
	var pat *compiledPattern
	var err error
	if spec.regex {
		pat, err = compileRegex(spec.raw)
	} else {
		method, path := splitMethodPath(spec.raw)
		if method != "" {
			lf.methods = []string{method}
		}
		body := strings.TrimLeft(path, "/")
		mux := strings.HasPrefix(path, "/")
		lf.subtree = mux && (body == "" || strings.HasSuffix(body, "/"))
		pat, err = compileRoute(body, !lf.subtree, bc.converters)
		if err == nil && pat.endpoint {
			lf.subtree = false
		}
	}
	if err != nil {
		return nil, err
	}

	lf.route = pat.display
	lf.extraKwargs = lf.kwargs
	lf.mustCopy = len(lf.kwargs) > 0
	for i := len(bc.chain) - 1; i >= 0; i-- {
		inc := bc.chain[i]
		lf.route = joinRoute(inc.pat.display, lf.route)
		if len(inc.kwargs) > 0 {
			merged := maps.Clone(inc.kwargs)
			maps.Copy(merged, lf.extraKwargs)
			lf.extraKwargs = merged
		}
		if len(inc.kwargs) > 0 || inc.pat.hasNamed {
			lf.mustCopy = true
		}
	}
	for _, inc := range bc.chain {
		if inc.appName != "" {
			lf.appNames = append(lf.appNames, inc.appName)
		}
		if inc.namespace != "" {
			lf.namespaces = append(lf.namespaces, inc.namespace)
		}
	}
	if lf.name != "" {
		lf.viewName = strings.Join(append(slices.Clone(lf.namespaces), lf.name), ":")
	}
	return &node{pat: pat, leaf: lf}, nil
}

// joinRoute joins the display forms of an include prefix and of what is below
// it, dropping the ^ an expression starts with. It is Django's
// URLResolver._join_route.
func joinRoute(route1, route2 string) string {
	if route1 == "" {
		return route2
	}
	return route1 + strings.TrimPrefix(route2, "^")
}

// resolveState carries the request-wide inputs and outputs of one walk over
// the tree.
type resolveState struct {
	// method is the request method; empty means methods are not checked.
	method string
	// partial lets ServeMux subtree patterns match longer paths.
	partial bool
	// collect asks for the list of tried patterns.
	collect bool
	// allowed gathers the methods of routes that matched the path but not the
	// method.
	allowed []string
}

// resolveLeaf matches path against a route. It mirrors URLPattern.resolve.
func (n *node) resolveLeaf(st *resolveState, path string) *ResolverMatch {
	pm, ok := n.pat.match(path)
	if !ok {
		return nil
	}
	lf := n.leaf
	if lf.subtree && pm.rest != "" && !st.partial {
		return nil
	}
	if st.method != "" && !lf.allows(st.method) {
		st.allowed = append(st.allowed, lf.methods...)
		return nil
	}
	m := &ResolverMatch{
		Handler:        lf.handler,
		Args:           pm.args,
		Kwargs:         pm.kwargs,
		URLName:        lf.name,
		AppNames:       lf.appNames,
		Namespaces:     lf.namespaces,
		Route:          lf.route,
		ViewName:       lf.viewName,
		CapturedKwargs: pm.kwargs,
		ExtraKwargs:    lf.extraKwargs,
		raw:            pm.raw,
	}
	if lf.mustCopy {
		m.Kwargs = make(Kwargs, len(pm.kwargs)+len(lf.extraKwargs))
		maps.Copy(m.Kwargs, pm.kwargs)
		maps.Copy(m.Kwargs, lf.kwargs)
	}
	return m
}

// resolve matches path against an include and the routes below it. entered
// reports whether the prefix matched, which decides how a failure shows up in
// the tried list. It mirrors URLResolver.resolve.
func (n *node) resolve(st *resolveState, path string) (m *ResolverMatch, tried [][]TriedPattern, entered bool) {
	pm, ok := n.pat.match(path)
	if !ok {
		return nil, nil, false
	}
	for _, c := range n.children {
		var sub *ResolverMatch
		var subTried [][]TriedPattern
		subEntered := false
		if c.leaf != nil {
			sub = c.resolveLeaf(st, pm.rest)
		} else {
			sub, subTried, subEntered = c.resolve(st, pm.rest)
		}
		if st.collect {
			if subEntered {
				for _, t := range subTried {
					tried = append(tried, append([]TriedPattern{c.triedPattern()}, t...))
				}
			} else {
				tried = append(tried, []TriedPattern{c.triedPattern()})
			}
		}
		if sub == nil {
			continue
		}
		n.mergeInto(sub, pm)
		return sub, tried, true
	}
	return nil, tried, true
}

// mergeInto adds what an include contributes to the match of a route below
// it. The route's own arguments win over the include's extra arguments, which
// win over the include's captures.
func (n *node) mergeInto(sub *ResolverMatch, pm patternMatch) {
	if len(n.kwargs) > 0 || len(pm.kwargs) > 0 {
		for k, v := range n.kwargs {
			if _, ok := sub.Kwargs[k]; !ok {
				sub.Kwargs[k] = v
			}
		}
		for k, v := range pm.kwargs {
			if _, ok := sub.Kwargs[k]; !ok {
				sub.Kwargs[k] = v
			}
		}
	}
	if len(sub.Kwargs) == 0 && len(pm.args) > 0 {
		sub.Args = append(slices.Clone(pm.args), sub.Args...)
	}
	if len(pm.raw) > 0 {
		sub.raw = append(slices.Clone(pm.raw), sub.raw...)
	}
}
