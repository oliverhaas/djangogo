package urls

import (
	"maps"
	"net/url"
	"regexp"
	"slices"
	"strings"
)

// CurrentApp is an argument for Router.Reverse that names the instance
// namespace of the request being served, Django's current_app. It only
// matters when one application is mounted under several instance namespaces:
// reversing "app:name" then picks the instance the request belongs to.
type CurrentApp string

// ReverseOptions are the arguments of Router.ReverseWith, the keyword
// arguments of Django's reverse().
type ReverseOptions struct {
	// Args are positional arguments. They cannot be combined with Kwargs.
	Args []any
	// Kwargs are keyword arguments.
	Kwargs Kwargs
	// CurrentApp is the instance namespace of the current request; see
	// CurrentApp.
	CurrentApp string
	// Query is appended as the query string when it is not empty. Keys are
	// written in sorted order.
	Query url.Values
	// Fragment is appended after a # when it is not empty. It is not escaped,
	// as in Django.
	Fragment string
}

// reverseEntry is one named route as seen from an include: its expression
// relative to the include, its extra arguments, and its converters. It is the
// tuple Django keeps in URLResolver.reverse_dict, without the normalized
// forms, which depend on the namespace prefix and are computed on demand.
type reverseEntry struct {
	pattern    string
	defaults   Kwargs
	converters map[string]Converter
}

// namespaceEntry is one instance namespace reachable from an include: the
// expression in front of it and the include that holds its routes.
type namespaceEntry struct {
	prefix     string
	node       *node
	converters map[string]Converter
}

// reverseIndex is what an include knows about the names below it, Django's
// reverse_dict, namespace_dict and app_dict.
type reverseIndex struct {
	// lookups maps a route name to its routes, the last declared first.
	lookups map[string][]reverseEntry
	// namespaces maps an instance namespace to its include; of two includes
	// with the same namespace the first declared wins.
	namespaces map[string]namespaceEntry
	// apps maps an application namespace to its instance namespaces, the last
	// declared first.
	apps map[string][]string
}

// populate builds the reverse index of an include and of the includes below
// it. It mirrors URLResolver._populate.
func (n *node) populate() {
	idx := &reverseIndex{
		lookups:    map[string][]reverseEntry{},
		namespaces: map[string]namespaceEntry{},
		apps:       map[string][]string{},
	}
	for i := len(n.children) - 1; i >= 0; i-- {
		c := n.children[i]
		p := strings.TrimPrefix(c.pat.pyRegex, "^")
		if c.leaf != nil {
			if c.leaf.name != "" {
				idx.lookups[c.leaf.name] = append(idx.lookups[c.leaf.name], reverseEntry{
					pattern: p, defaults: c.leaf.kwargs, converters: c.pat.converters,
				})
			}
			continue
		}
		c.populate()
		if c.appName != "" {
			idx.apps[c.appName] = append(idx.apps[c.appName], c.namespace)
			idx.namespaces[c.namespace] = namespaceEntry{prefix: p, node: c, converters: c.pat.converters}
			continue
		}
		for name, entries := range c.rev.lookups {
			for _, e := range entries {
				idx.lookups[name] = append(idx.lookups[name], reverseEntry{
					pattern:    p + e.pattern,
					defaults:   mergeMaps(e.defaults, c.kwargs),
					converters: mergeMaps(n.pat.converters, c.pat.converters, e.converters),
				})
			}
		}
		for ns, e := range c.rev.namespaces {
			idx.namespaces[ns] = namespaceEntry{
				prefix: p + e.prefix, node: e.node,
				converters: mergeMaps(e.converters, c.pat.converters),
			}
		}
		for app, list := range c.rev.apps {
			idx.apps[app] = append(idx.apps[app], list...)
		}
	}
	n.rev = idx
}

// mergeMaps returns the union of the maps, later ones winning. It returns one
// of its arguments unchanged when the others are empty.
func mergeMaps[M ~map[K]V, K comparable, V any](ms ...M) M {
	var out M
	owned := false
	for _, m := range ms {
		if len(m) == 0 {
			continue
		}
		if out == nil {
			out = m
			continue
		}
		if !owned {
			out = maps.Clone(out)
			owned = true
		}
		maps.Copy(out, m)
	}
	return out
}

// possibility is one route a name can reverse to, ready for use: the text
// forms, the full expression, and the compiled expression that candidates are
// checked against.
type possibility struct {
	forms      []revForm
	pattern    string
	defaults   Kwargs
	converters map[string]Converter
	check      *regexp.Regexp
}

// possibilities resolves the namespaces of a view name and returns the routes
// its last part can reverse to. It mirrors the namespace walk of
// django.urls.reverse and get_ns_resolver.
func (rt *Router) possibilities(viewname, currentApp string) (view string, poss []*possibility, err error) {
	path := strings.Split(viewname, ":")
	view = path[len(path)-1]
	path = path[:len(path)-1]

	var currentPath []string
	if currentApp != "" {
		currentPath = strings.Split(currentApp, ":")
		slices.Reverse(currentPath)
	}
	n := rt.root
	nsPattern := ""
	var nsConverters map[string]Converter
	var resolved []string
	for _, ns := range path {
		currentNS := ""
		if len(currentPath) > 0 {
			currentNS = currentPath[len(currentPath)-1]
			currentPath = currentPath[:len(currentPath)-1]
		}
		if appList, ok := n.rev.apps[ns]; ok {
			switch {
			case currentNS != "" && slices.Contains(appList, currentNS):
				// Reversing for a particular instance of the application.
				ns = currentNS
			case !slices.Contains(appList, ns):
				// No instance carries the application's name, so the first
				// instance is the default.
				ns = appList[0]
			}
		}
		if ns != currentNS {
			currentPath = nil
		}
		e, ok := n.rev.namespaces[ns]
		if !ok {
			if len(resolved) > 0 {
				return view, nil, noReverseMatchf("%s is not a registered namespace inside '%s'",
					pyRepr(ns), strings.Join(resolved, ":"))
			}
			return view, nil, noReverseMatchf("%s is not a registered namespace", pyRepr(ns))
		}
		resolved = append(resolved, ns)
		nsPattern += e.prefix
		nsConverters = mergeMaps(nsConverters, e.converters)
		n = e.node
	}

	key := strings.Join(resolved, ":") + "\x00" + view
	rt.mu.RLock()
	poss, ok := rt.revCache[key]
	rt.mu.RUnlock()
	if ok {
		return view, poss, nil
	}
	entries := n.rev.lookups[view]
	if len(entries) == 0 {
		return view, nil, nil
	}
	poss = make([]*possibility, len(entries))
	for i, e := range entries {
		p := &possibility{
			pattern:    nsPattern + e.pattern,
			defaults:   e.defaults,
			converters: mergeMaps(nsConverters, e.converters),
		}
		p.forms = normalize(p.pattern)
		// A pattern that does not compile here can never confirm a candidate,
		// so the route stays listed in messages but never reverses.
		p.check, _ = regexp.Compile("^/" + pyToGoRegex(p.pattern))
		poss[i] = p
	}
	rt.mu.Lock()
	rt.revCache[key] = poss
	rt.mu.Unlock()
	return view, poss, nil
}

// Reverse builds the URL of a named route, Django's reverse(). name may carry
// namespaces, as in "blog:detail" or "outer:inner:detail".
//
// The arguments are positional values for the parameters of the route, in
// order, or one Kwargs (or map[string]any) with named values; the two cannot
// be mixed. A CurrentApp argument sets the current application. The values are
// written with the route's converters and percent-encoded the way Django
// encodes them.
//
// Several routes may share a name. The one that fits the arguments is used:
// the number of positional arguments, or the names of the keyword arguments,
// must equal the route's parameters, and the resulting URL must match the
// route. Later routes are tried first, as in Django.
//
// The error is a *NoReverseMatch with Django's message when nothing fits.
func (rt *Router) Reverse(name string, args ...any) (string, error) {
	var opts ReverseOptions
	for _, a := range args {
		switch v := a.(type) {
		case Kwargs:
			opts.Kwargs = mergeMaps(opts.Kwargs, v)
		case map[string]any:
			opts.Kwargs = mergeMaps(opts.Kwargs, Kwargs(v))
		case CurrentApp:
			opts.CurrentApp = string(v)
		default:
			opts.Args = append(opts.Args, a)
		}
	}
	return rt.ReverseWith(name, opts)
}

// ReverseWith is Reverse with every argument of Django's reverse() spelled
// out: positional or keyword arguments, the current application, a query
// string and a fragment.
func (rt *Router) ReverseWith(name string, opts ReverseOptions) (string, error) {
	view, poss, err := rt.possibilities(name, opts.CurrentApp)
	if err != nil {
		return "", err
	}
	if len(opts.Args) > 0 && len(opts.Kwargs) > 0 {
		return "", ErrMixedArgs
	}
	for _, p := range poss {
		for i := range p.forms {
			if u, ok := p.build(&p.forms[i], opts.Args, opts.Kwargs); ok {
				if q := opts.Query.Encode(); q != "" {
					u += "?" + q
				}
				if opts.Fragment != "" {
					u += "#" + opts.Fragment
				}
				return u, nil
			}
		}
	}
	if len(poss) == 0 {
		return "", noReverseMatchf(
			"Reverse for '%s' not found. '%s' is not a valid view function or pattern name.", view, view)
	}
	argMsg := "no arguments"
	switch {
	case len(opts.Args) > 0:
		argMsg = "arguments '" + pyTuple(opts.Args) + "'"
	case len(opts.Kwargs) > 0:
		argMsg = "keyword arguments '" + pyDict(opts.Kwargs) + "'"
	}
	patterns := make([]any, len(poss))
	for i, p := range poss {
		patterns[i] = p.pattern
	}
	return "", noReverseMatchf("Reverse for '%s' with %s not found. %d pattern(s) tried: %s",
		view, argMsg, len(patterns), pyList(patterns))
}

// build fills one reverse form with the arguments and returns the encoded URL
// when the arguments fit the route. It is the body of the loop in Django's
// URLResolver._reverse_with_prefix.
func (p *possibility) build(form *revForm, args []any, kwargs Kwargs) (string, bool) {
	var subs map[string]any
	if len(args) > 0 {
		if len(args) != len(form.params) {
			return "", false
		}
		subs = make(map[string]any, len(args))
		for i, name := range form.params {
			subs[name] = args[i]
		}
	} else {
		// The keyword arguments must name exactly the parameters, apart from
		// names that are extra arguments of the route.
		for k := range kwargs {
			if _, isDefault := p.defaults[k]; !isDefault && !slices.Contains(form.params, k) {
				return "", false
			}
		}
		for _, name := range form.params {
			if _, given := kwargs[name]; !given {
				if _, isDefault := p.defaults[name]; !isDefault {
					return "", false
				}
			}
		}
		// An extra argument that is passed must have the value the route
		// declares.
		for k, v := range p.defaults {
			if slices.Contains(form.params, k) {
				continue
			}
			if given, ok := kwargs[k]; ok && !looseEqual(given, v) {
				return "", false
			}
		}
		subs = kwargs
	}

	var b strings.Builder
	b.WriteByte('/')
	for _, part := range form.parts {
		if part.ref == "" {
			b.WriteString(part.lit)
			continue
		}
		v, ok := subs[part.ref]
		if !ok {
			return "", false
		}
		if conv, ok := p.converters[part.ref]; ok {
			text, err := conv.ToURL(v)
			if err != nil {
				return "", false
			}
			b.WriteString(text)
			continue
		}
		b.WriteString(pyStr(v))
	}
	// The resolver works on decoded paths, so the candidate is checked before
	// it is encoded.
	candidate := b.String()
	if p.check == nil || !p.check.MatchString(candidate) {
		return "", false
	}
	return escapeLeadingSlashes(quotePath(candidate)), true
}

// quotePath percent-encodes a path as Django's reverse does: Python's
// urllib.parse.quote with RFC 3986 sub-delimiters and "/~:@" left alone.
func quotePath(s string) string {
	const upperHex = "0123456789ABCDEF"
	safe := func(c byte) bool {
		switch {
		case 'a' <= c && c <= 'z', 'A' <= c && c <= 'Z', '0' <= c && c <= '9':
			return true
		}
		return strings.IndexByte("_.-~!$&'()*+,;=/:@", c) >= 0
	}
	n := 0
	for i := 0; i < len(s); i++ {
		if !safe(s[i]) {
			n++
		}
	}
	if n == 0 {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + 2*n)
	for i := 0; i < len(s); i++ {
		c := s[i]
		if safe(c) {
			b.WriteByte(c)
			continue
		}
		b.WriteByte('%')
		b.WriteByte(upperHex[c>>4])
		b.WriteByte(upperHex[c&15])
	}
	return b.String()
}

// escapeLeadingSlashes keeps a URL that starts with two slashes from being
// read as a scheme-relative URL (//host/path) by escaping the second slash.
// It is django.utils.http.escape_leading_slashes.
func escapeLeadingSlashes(u string) string {
	if strings.HasPrefix(u, "//") {
		return "/%2F" + strings.TrimPrefix(u, "//")
	}
	return u
}
