package templates

import (
	"errors"
	"fmt"

	"github.com/flosch/pongo2/v6"

	"github.com/oliverhaas/djangogo/urls"
)

// urlKwarg is one name=value argument of {% url %}.
type urlKwarg struct {
	name string
	expr pongo2.IEvaluator
}

// urlTagNode implements Django's {% url %}:
//
//	{% url "name" arg1 arg2 %}
//	{% url "name" key1=value1 key2=value2 %}
//	{% url "ns:name" pk as detail_url %}
type urlTagNode struct {
	start    *pongo2.Token
	nameExpr pongo2.IEvaluator
	argExprs []pongo2.IEvaluator
	kwargs   []urlKwarg
	asVar    string
}

// Execute reverses the route and writes the URL, escaped under autoescape, or
// stores it in the "as" variable. A route that does not reverse is a render
// error; with "as" the variable is empty instead, so a template can test it.
// The resolver is the one injected by Engine.SetResolver, else the
// process-global one.
func (n *urlTagNode) Execute(ctx *pongo2.ExecutionContext, w pongo2.TemplateWriter) *pongo2.Error {
	nameVal, err := n.nameExpr.Evaluate(ctx)
	if err != nil {
		return err
	}
	args := make([]any, 0, len(n.argExprs)+2)
	for _, expr := range n.argExprs {
		val, verr := expr.Evaluate(ctx)
		if verr != nil {
			return verr
		}
		args = append(args, val.Interface())
	}
	if len(n.kwargs) > 0 {
		kwargs := make(urls.Kwargs, len(n.kwargs))
		for _, kw := range n.kwargs {
			val, verr := kw.expr.Evaluate(ctx)
			if verr != nil {
				return verr
			}
			kwargs[kw.name] = val.Interface()
		}
		args = append(args, kwargs)
	}
	if app := currentApp(stateFrom(ctx)); app != "" {
		args = append(args, urls.CurrentApp(app))
	}
	resolved, rerr := resolverFrom(ctx)(nameVal.String(), args...)
	if rerr != nil {
		if n.asVar == "" || !errors.Is(rerr, urls.ErrNoReverseMatch) {
			return ctx.OrigError(fmt.Errorf("templates: url %q: %w", nameVal.String(), rerr), n.start)
		}
		resolved = ""
	}
	if n.asVar != "" {
		ctx.Private[n.asVar] = resolved
		return nil
	}
	if ctx.Autoescape {
		resolved = escapeHTML(resolved)
	}
	if _, werr := w.WriteString(resolved); werr != nil {
		return ctx.OrigError(werr, n.start)
	}
	return nil
}

// currentApp returns the current application of the render: the one set with
// WithCurrentApp, else the namespace the request resolved to.
func currentApp(st *renderState) string {
	if st == nil || st.request == nil {
		return ""
	}
	if app, ok := st.request.Context().Value(currentAppKey{}).(string); ok {
		return app
	}
	if m := urls.Match(st.request); m != nil {
		return m.Namespace()
	}
	return ""
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

// urlTagParser parses {% url %}. The name and every argument are expressions:
// literals, variables, variables with filters.
func urlTagParser(_ *pongo2.Parser, start *pongo2.Token, args *pongo2.Parser) (pongo2.INodeTag, *pongo2.Error) {
	if args.Remaining() == 0 {
		return nil, args.Error("'url' takes at least one argument, a URL pattern name.", start)
	}
	nameExpr, err := args.ParseExpression()
	if err != nil {
		return nil, err
	}
	node := &urlTagNode{start: start, nameExpr: nameExpr}
	for args.Remaining() > 0 {
		if args.Match(pongo2.TokenKeyword, "as") != nil {
			name := args.MatchType(pongo2.TokenIdentifier)
			if name == nil || args.Remaining() > 0 {
				return nil, args.Error("Malformed arguments to url tag", start)
			}
			node.asVar = name.Val
			break
		}
		if key := args.PeekType(pongo2.TokenIdentifier); key != nil && args.PeekN(1, pongo2.TokenSymbol, "=") != nil {
			args.ConsumeN(2)
			if args.Remaining() == 0 {
				return nil, args.Error("Malformed arguments to url tag", start)
			}
			expr, kerr := args.ParseExpression()
			if kerr != nil {
				return nil, kerr
			}
			node.kwargs = append(node.kwargs, urlKwarg{name: key.Val, expr: expr})
			continue
		}
		expr, aerr := args.ParseExpression()
		if aerr != nil {
			return nil, aerr
		}
		node.argExprs = append(node.argExprs, expr)
	}
	return node, nil
}
