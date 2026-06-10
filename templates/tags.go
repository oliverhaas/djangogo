package templates

import (
	"fmt"
	"html"
	"strings"
	"sync"

	"github.com/flosch/pongo2/v6"
)

// StaticBase is the URL prefix for {% static %} (default "/static/").
var StaticBase = "/static/"

// Resolver reverses a named route to a URL path, the signature behind {% url %}.
// It matches urls.Router.Reverse so a router can be wired in directly.
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

func init() { registerOnce.Do(registerTags) }

// registerTags registers the Django-style {% static %}, {% url %} and
// {% csrf_token %} tags with pongo2. pongo2 deliberately omits these (they are
// web-framework specific), so they are implemented as real tags here.
func registerTags() {
	mustRegister("static", staticTagParser)
	mustRegister("url", urlTagParser)
	mustRegister("csrf_token", csrfTokenTagParser)
}

// mustRegister registers a tag and panics if pongo2 rejects it. A rejection means
// a programming error (duplicate name), not a runtime condition.
func mustRegister(name string, fn pongo2.TagParser) {
	if err := pongo2.RegisterTag(name, fn); err != nil {
		panic(fmt.Sprintf("templates: register tag %q: %v", name, err))
	}
}

// staticTagNode implements {% static <expr> %}.
type staticTagNode struct {
	pathExpr pongo2.IEvaluator
}

// Execute writes StaticBase joined with the evaluated path expression.
func (n *staticTagNode) Execute(ctx *pongo2.ExecutionContext, w pongo2.TemplateWriter) *pongo2.Error {
	val, err := n.pathExpr.Evaluate(ctx)
	if err != nil {
		return err
	}
	_, werr := w.WriteString(joinStatic(StaticBase, val.String()))
	if werr != nil {
		return ctx.OrigError(werr, nil)
	}
	return nil
}

// joinStatic joins a static URL base and a path with exactly one slash between them.
func joinStatic(base, path string) string {
	return strings.TrimRight(base, "/") + "/" + strings.TrimLeft(path, "/")
}

// staticTagParser parses {% static "css/app.css" %}. The argument may be a string
// literal or any expression resolving to a string.
func staticTagParser(_ *pongo2.Parser, _ *pongo2.Token, args *pongo2.Parser) (pongo2.INodeTag, *pongo2.Error) {
	pathExpr, err := args.ParseExpression()
	if err != nil {
		return nil, err
	}
	if args.Remaining() > 0 {
		return nil, args.Error("templates: 'static' takes exactly one argument", nil)
	}
	return &staticTagNode{pathExpr: pathExpr}, nil
}

// urlTagNode implements {% url <name-expr> [arg-expr ...] %}.
type urlTagNode struct {
	nameExpr pongo2.IEvaluator
	argExprs []pongo2.IEvaluator
}

// Execute resolves the route name and arguments against the context and writes the
// reversed URL. It prefers a per-render Resolver injected into the context (by
// Engine.SetResolver), falling back to the process-global resolver. A resolver
// error is surfaced as a template error.
func (n *urlTagNode) Execute(ctx *pongo2.ExecutionContext, w pongo2.TemplateWriter) *pongo2.Error {
	nameVal, err := n.nameExpr.Evaluate(ctx)
	if err != nil {
		return err
	}
	args := make([]any, 0, len(n.argExprs))
	for _, expr := range n.argExprs {
		val, verr := expr.Evaluate(ctx)
		if verr != nil {
			return verr
		}
		args = append(args, val.Interface())
	}
	resolve := resolverFrom(ctx)
	resolved, rerr := resolve(nameVal.String(), args...)
	if rerr != nil {
		return ctx.OrigError(fmt.Errorf("templates: url %q: %w", nameVal.String(), rerr), nil)
	}
	if _, werr := w.WriteString(resolved); werr != nil {
		return ctx.OrigError(werr, nil)
	}
	return nil
}

// resolverFrom returns the per-render Resolver carried in the context, or the
// process-global resolver (URLResolverFunc) when none is present.
func resolverFrom(ctx *pongo2.ExecutionContext) Resolver {
	if v, ok := ctx.Public[resolverContextKey]; ok {
		if r, ok := v.(Resolver); ok && r != nil {
			return r
		}
	}
	return URLResolverFunc()
}

// urlTagParser parses {% url "route-name" arg1 arg2 %}. The name and each argument
// are pongo2 expressions evaluated against the context.
func urlTagParser(_ *pongo2.Parser, _ *pongo2.Token, args *pongo2.Parser) (pongo2.INodeTag, *pongo2.Error) {
	nameExpr, err := args.ParseExpression()
	if err != nil {
		return nil, err
	}
	node := &urlTagNode{nameExpr: nameExpr}
	for args.Remaining() > 0 {
		argExpr, aerr := args.ParseExpression()
		if aerr != nil {
			return nil, aerr
		}
		node.argExprs = append(node.argExprs, argExpr)
	}
	return node, nil
}

// csrfTokenTagNode implements {% csrf_token %}.
type csrfTokenTagNode struct{}

// Execute writes a hidden input carrying the HTML-escaped context value csrf_token
// (empty string when absent).
func (csrfTokenTagNode) Execute(ctx *pongo2.ExecutionContext, w pongo2.TemplateWriter) *pongo2.Error {
	token := ""
	if v, ok := ctx.Public["csrf_token"]; ok && v != nil {
		token = pongo2.AsValue(v).String()
	}
	field := fmt.Sprintf(
		`<input type="hidden" name="csrfmiddlewaretoken" value="%s">`,
		html.EscapeString(token),
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
