package urls

import (
	"regexp"
	"strings"
	"unicode"
)

// httpMethods are the tokens accepted as the method prefix of a route.
var httpMethods = map[string]bool{
	"GET": true, "HEAD": true, "POST": true, "PUT": true, "PATCH": true,
	"DELETE": true, "CONNECT": true, "OPTIONS": true, "TRACE": true,
}

// splitMethodPath splits a route into its optional METHOD prefix and the path
// part. Without a method prefix the method is empty.
//
//	"GET /articles/{id}/" -> ("GET", "/articles/{id}/")
//	"POST articles/<int:id>/" -> ("POST", "articles/<int:id>/")
//	"/about/" -> ("", "/about/")
func splitMethodPath(pattern string) (method, path string) {
	tok, rest, found := strings.Cut(pattern, " ")
	if !found {
		return "", pattern
	}
	if httpMethods[tok] || strings.HasPrefix(rest, "/") {
		return tok, strings.TrimLeft(rest, " ")
	}
	return "", pattern
}

// compiledPattern is one compiled route, include prefix, or regular expression,
// the counterpart of Django's RoutePattern and RegexPattern.
type compiledPattern struct {
	// display is what Django's str(pattern) gives: the route or the expression
	// as written.
	display string
	// pyRegex is the expression in Python's dialect, as Django's
	// pattern.regex.pattern. Reverse builds its candidates and its messages from
	// it.
	pyRegex string
	// re is the compiled expression. It is nil for a route without parameters,
	// which is compared as plain text.
	re *regexp.Regexp
	// literal is the text of a route without parameters.
	literal string
	// prefix is the text before the first parameter of a route. A path that
	// does not start with it cannot match, which saves running re.
	prefix string
	// endpoint marks a pattern that must consume the whole remaining path.
	endpoint bool
	// isRegex marks a RePath or regex-include pattern.
	isRegex bool
	// names holds the group name per capture index; index 0 and unnamed groups
	// are empty.
	names []string
	// hasNamed reports whether any group is named.
	hasNamed bool
	// converters maps a parameter name to its converter (routes only).
	converters map[string]Converter
}

// rawValue is the text one named group captured, kept for
// http.Request.PathValue.
type rawValue struct {
	name, value string
}

// patternMatch is the result of matching a path against a compiledPattern.
type patternMatch struct {
	// rest is the part of the path the pattern left over.
	rest string
	// args are the unnamed groups of an expression without named groups.
	args []any
	// kwargs are the named groups after conversion.
	kwargs Kwargs
	// raw are the named groups as captured.
	raw []rawValue
}

// match matches path, which never has its leading slash, against the pattern.
// It mirrors RoutePattern.match and RegexPattern.match.
func (p *compiledPattern) match(path string) (patternMatch, bool) {
	if p.re == nil {
		switch {
		case p.endpoint:
			if path == p.literal {
				return patternMatch{}, true
			}
		case strings.HasPrefix(path, p.literal):
			return patternMatch{rest: path[len(p.literal):]}, true
		}
		return patternMatch{}, false
	}
	if !strings.HasPrefix(path, p.prefix) {
		return patternMatch{}, false
	}
	loc := p.re.FindStringSubmatchIndex(path)
	if loc == nil {
		return patternMatch{}, false
	}
	m := patternMatch{rest: path[loc[1]:]}
	if !p.hasNamed {
		if len(p.names) > 1 {
			m.args = make([]any, 0, len(p.names)-1)
			for i := 1; i < len(p.names); i++ {
				if loc[2*i] < 0 {
					m.args = append(m.args, nil)
					continue
				}
				m.args = append(m.args, path[loc[2*i]:loc[2*i+1]])
			}
		}
		return m, true
	}
	m.kwargs = make(Kwargs, len(p.names)-1)
	m.raw = make([]rawValue, 0, len(p.names)-1)
	for i := 1; i < len(p.names); i++ {
		name := p.names[i]
		if name == "" || loc[2*i] < 0 {
			continue
		}
		text := path[loc[2*i]:loc[2*i+1]]
		m.raw = append(m.raw, rawValue{name: name, value: text})
		conv, ok := p.converters[name]
		if !ok {
			m.kwargs[name] = text
			continue
		}
		v, err := conv.ToGo(text)
		if err != nil {
			return patternMatch{}, false
		}
		m.kwargs[name] = v
	}
	return m, true
}

// parameterRe finds the parameters of a route: Django's <converter:name> and
// <name>, and ServeMux's {name}, {name...} and {$}.
var parameterRe = regexp.MustCompile(`<(?:([^>:]+):)?([^>]+)>|\{([^{}]*)\}`)

// pyWhitespace is Python's string.whitespace, which Django forbids inside the
// angle brackets of a route.
const pyWhitespace = " \t\n\r\x0b\x0c"

// compileRoute compiles a route in Django or ServeMux syntax. route has no
// leading slash and no method prefix. endpoint says whether the route must
// match the whole remaining path; a {$} in the route forces it. It mirrors
// Django's _route_to_regex.
func compileRoute(route string, endpoint bool, converters map[string]Converter) (*compiledPattern, error) {
	p := &compiledPattern{display: route, endpoint: endpoint, converters: map[string]Converter{}}
	var py strings.Builder
	py.WriteByte('^')
	var literal strings.Builder
	first := true
	prev := 0
	for _, loc := range parameterRe.FindAllStringSubmatchIndex(route, -1) {
		chunk := route[prev:loc[0]]
		whole := route[loc[0]:loc[1]]
		prev = loc[1]
		literal.WriteString(chunk)
		py.WriteString(pyReEscape(chunk))
		var convName, param string
		rest := false
		if loc[6] >= 0 {
			inner := route[loc[6]:loc[7]]
			if inner == "$" {
				if loc[1] != len(route) {
					return nil, configErrorf("URL route %s uses {$} before the end of the route.", pyRepr(route))
				}
				p.endpoint = true
				continue
			}
			convName, param = "str", inner
			if name, ok := strings.CutSuffix(inner, "..."); ok {
				param, rest = name, true
			}
		} else {
			if strings.ContainsAny(whole, pyWhitespace) {
				return nil, configErrorf(
					"URL route %s cannot contain whitespace in angle brackets <…>.", pyRepr(route))
			}
			convName = "str"
			if loc[2] >= 0 {
				convName = route[loc[2]:loc[3]]
			}
			param = route[loc[4]:loc[5]]
		}
		if !isIdentifier(param) {
			return nil, configErrorf(
				"URL route %s uses parameter name %s which isn't a valid Python identifier.",
				pyRepr(route), pyRepr(param))
		}
		conv, ok := converters[convName]
		if rest {
			conv, ok = muxRestConverter, true
		} else if convName == "str" && !ok {
			conv, ok = defaultConverters["str"], true
		}
		if !ok {
			return nil, configErrorf("URL route %s uses invalid converter %s.", pyRepr(route), pyRepr(convName))
		}
		if _, dup := p.converters[param]; dup {
			return nil, configErrorf("URL route %s uses parameter name %s more than once.", pyRepr(route), pyRepr(param))
		}
		p.converters[param] = conv
		if first {
			p.prefix = literal.String()
			first = false
		}
		py.WriteString("(?P<" + param + ">" + conv.Regex() + ")")
	}
	tail := route[prev:]
	literal.WriteString(tail)
	py.WriteString(pyReEscape(tail))
	if p.endpoint {
		py.WriteString(`\Z`)
	}
	p.pyRegex = py.String()
	if len(p.converters) == 0 {
		p.literal = literal.String()
		return p, nil
	}
	re, err := regexp.Compile(pyToGoRegex(p.pyRegex))
	if err != nil {
		return nil, configErrorf("URL route %s does not compile: %v", pyRepr(route), err)
	}
	p.re = re
	p.names = re.SubexpNames()
	p.hasNamed = true
	return p, nil
}

// muxRestConverter backs ServeMux's {name...}, which matches the rest of the
// path including the empty rest.
var muxRestConverter Converter = stringConverter{regex: ".*"}

// compileRegex compiles a regular expression written for Django's re_path. It
// mirrors RegexPattern.
func compileRegex(expr string) (*compiledPattern, error) {
	re, err := regexp.Compile(pyToGoRegex(expr))
	if err != nil {
		return nil, configErrorf("%q is not a valid regular expression: %v", expr, err)
	}
	p := &compiledPattern{display: expr, pyRegex: expr, re: re, isRegex: true, names: re.SubexpNames()}
	for _, n := range p.names {
		if n != "" {
			p.hasNamed = true
			break
		}
	}
	return p, nil
}

// isIdentifier reports whether s can name a parameter. Python accepts any
// Unicode identifier; a Go expression needs ASCII group names, so the check is
// the ASCII subset.
func isIdentifier(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		if r > unicode.MaxASCII {
			return false
		}
		if r != '_' && !unicode.IsLetter(r) && (i == 0 || !unicode.IsDigit(r)) {
			return false
		}
	}
	return true
}

// pyReEscape escapes text for use in a regular expression exactly as Python's
// re.escape does, so that the expressions shown in messages equal Django's.
func pyReEscape(s string) string {
	const special = "()[]{}?*+-|^$\\.&~# \t\n\r\v\f"
	if !strings.ContainsAny(s, special) {
		return s
	}
	var b strings.Builder
	for _, r := range s {
		if r < unicode.MaxASCII && strings.ContainsRune(special, r) {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}

// pyToGoRegex translates the parts of Python's dialect that Go's regexp
// package spells differently. Only \Z needs it (Go writes \z); constructs Go
// has no equivalent for, such as lookaround, are left alone and fail to
// compile.
func pyToGoRegex(expr string) string {
	if !strings.Contains(expr, `\Z`) {
		return expr
	}
	var b strings.Builder
	for i := 0; i < len(expr); i++ {
		c := expr[i]
		if c != '\\' || i+1 == len(expr) {
			b.WriteByte(c)
			continue
		}
		i++
		if expr[i] == 'Z' {
			b.WriteString(`\z`)
			continue
		}
		b.WriteByte('\\')
		b.WriteByte(expr[i])
	}
	return b.String()
}
