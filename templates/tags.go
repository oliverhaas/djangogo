package templates

import (
	"fmt"
	"strings"
	"sync"

	"github.com/flosch/pongo2/v6"
)

// StaticBase is the URL prefix for {% static %} (default "/static/").
var StaticBase = "/static/"

// Resolver reverses a named route to a URL path, the signature behind {% url %}.
// It matches urls.Router.Reverse so a router can be wired in directly. The
// positional arguments of the tag arrive as args; keyword arguments follow as
// one urls.Kwargs value, and the current application, when the request has
// one, as a urls.CurrentApp value.
type Resolver func(name string, args ...any) (string, error)

// resolverContextKey is the context key under which an Engine injects its
// per-render Resolver (see Engine.SetResolver). {% url %} prefers it over the
// process-global resolver, letting different engines reverse against different
// route tables. The name is a valid identifier so pongo2 accepts it as a key.
const resolverContextKey = "__djangogo_url_resolver__"

// urlResolverMu guards the process-global fallback resolver. The global is read
// during template rendering ({% url %}) and written at boot by the app, possibly
// from different goroutines (a second Application, concurrent tests), so access is
// synchronized to avoid a data race on the function value.
var (
	urlResolverMu sync.RWMutex
	urlResolverFn Resolver = func(name string, _ ...any) (string, error) {
		return "", fmt.Errorf("templates: no URLResolver configured (cannot reverse %q)", name)
	}
)

// SetURLResolver sets the process-global fallback resolver called by {% url %}
// when no per-render resolver is present in the context. It is wired by the app at
// boot and is safe for concurrent use.
func SetURLResolver(r Resolver) {
	urlResolverMu.Lock()
	defer urlResolverMu.Unlock()
	urlResolverFn = r
}

// URLResolverFunc returns the current process-global fallback resolver. It is safe
// for concurrent use, e.g. to save and restore the resolver in a test.
func URLResolverFunc() Resolver {
	urlResolverMu.RLock()
	defer urlResolverMu.RUnlock()
	return urlResolverFn
}

// registerOnce guards tag registration. pongo2's tag registry is process-global
// and RegisterTag panics-equivalent (returns an error) on a duplicate name, so the
// tags are registered exactly once for the whole process regardless of how many
// engines are created.
var registerOnce sync.Once

func init() {
	registerOnce.Do(func() {
		registerTags()
		registerFilters()
	})
}

// registerTags registers the Django-style {% static %}, {% url %} and
// {% csrf_token %} tags with pongo2. pongo2 deliberately omits these (they are
// web-framework specific), so they are implemented as real tags here. The
// pongo2 tags that behave differently from Django's are replaced.
func registerTags() {
	mustRegister("static", staticTagParser)
	mustRegister("url", urlTagParser)
	mustRegister("csrf_token", csrfTokenTagParser)
	mustRegister("load", loadTagParser)
	mustReplace("include", includeTagParser)
	mustReplace("for", forTagParser)
	mustReplace("cycle", cycleTagParser)
	mustRegister("resetcycle", resetcycleTagParser)
	mustReplace("ifchanged", ifchangedTagParser)
	mustRegister(syntaxErrorTag, syntaxErrorTagParser)
	mustReplace("widthratio", widthratioTagParser)
	mustReplace("now", nowTagParser)
	mustReplace("firstof", firstofTagParser)
	mustReplace("spaceless", spacelessTagParser)
	mustRegister("regroup", regroupTagParser)
	mustRegister("querystring", querystringTagParser)
	mustRegister("debug", debugTagParser)
	mustRegister("partialdef", partialDefTagParser)
	mustRegister("partial", partialTagParser)
	mustRegister(partialTemplateTag, partialTemplateTagParser)
	mustRegister(extendsVarTag, extendsVarTagParser)
}

// mustRegister registers a tag and panics if pongo2 rejects it. A rejection means
// a programming error (duplicate name), not a runtime condition.
func mustRegister(name string, fn pongo2.TagParser) {
	if err := pongo2.RegisterTag(name, fn); err != nil {
		panic(fmt.Sprintf("templates: register tag %q: %v", name, err))
	}
}

// mustReplace replaces a pongo2 tag and panics if pongo2 does not have it, which
// means the pongo2 version changed under this package.
func mustReplace(name string, fn pongo2.TagParser) {
	if err := pongo2.ReplaceTag(name, fn); err != nil {
		panic(fmt.Sprintf("templates: replace tag %q: %v", name, err))
	}
}

// staticTagNode implements Django's {% static %}:
//
//	{% static "css/app.css" %}
//	{% static path as url %}
type staticTagNode struct {
	pathExpr pongo2.IEvaluator
	asVar    string
}

// Execute writes StaticBase joined with the percent-encoded path, or stores
// it under the "as" name.
func (n *staticTagNode) Execute(ctx *pongo2.ExecutionContext, w pongo2.TemplateWriter) *pongo2.Error {
	val, err := n.pathExpr.Evaluate(ctx)
	if err != nil {
		return err
	}
	url := Static(val.String())
	if n.asVar != "" {
		ctx.Private[n.asVar] = url
		return nil
	}
	return writeOutput(ctx, w, pongo2.AsValue(url))
}

// Static returns the URL of the static file at path, what {% static path %}
// writes: StaticBase joined with the percent-encoded path, Django's
// django.templatetags.static.static.
func Static(path string) string {
	return joinStatic(StaticBase, percentEncode(path, "/"))
}

// joinStatic joins a static URL base and a path with exactly one slash between them.
func joinStatic(base, path string) string {
	return strings.TrimRight(base, "/") + "/" + strings.TrimLeft(path, "/")
}

// staticTagParser parses {% static "css/app.css" [as name] %}. The argument
// may be a string literal or any expression resolving to a string.
func staticTagParser(_ *pongo2.Parser, _ *pongo2.Token, args *pongo2.Parser) (pongo2.INodeTag, *pongo2.Error) {
	pathExpr, err := args.ParseExpression()
	if err != nil {
		return nil, err
	}
	asVar, err := parseAsVar(args)
	if err != nil {
		return nil, err
	}
	if args.Remaining() > 0 {
		return nil, args.Error("templates: 'static' takes exactly one argument", nil)
	}
	return &staticTagNode{pathExpr: pathExpr, asVar: asVar}, nil
}

// csrfTokenTagNode implements {% csrf_token %}.
type csrfTokenTagNode struct{}

// Execute writes a hidden input carrying the HTML-escaped context value
// csrf_token. Like Django, it writes nothing when the token is missing, empty or
// "NOTPROVIDED".
func (csrfTokenTagNode) Execute(ctx *pongo2.ExecutionContext, w pongo2.TemplateWriter) *pongo2.Error {
	token := ""
	if v, ok := ctx.Public["csrf_token"]; ok && v != nil {
		token = pongo2.AsValue(v).String()
	}
	if token == "" || token == "NOTPROVIDED" {
		return nil
	}
	field := fmt.Sprintf(
		`<input type="hidden" name="csrfmiddlewaretoken" value="%s">`,
		escapeHTML(token),
	)
	if _, werr := w.WriteString(field); werr != nil {
		return ctx.OrigError(werr, nil)
	}
	return nil
}

// csrfTokenTagParser parses {% csrf_token %} (no arguments).
func csrfTokenTagParser(_ *pongo2.Parser, _ *pongo2.Token, args *pongo2.Parser) (pongo2.INodeTag, *pongo2.Error) {
	if args.Remaining() > 0 {
		return nil, args.Error("templates: 'csrf_token' takes no arguments", nil)
	}
	return csrfTokenTagNode{}, nil
}
