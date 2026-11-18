package templates

import (
	"errors"
	"reflect"

	"github.com/flosch/pongo2/v6"
)

// maxIncludeDepth bounds nested includes. A template that includes itself
// without end would otherwise exhaust the stack and take the process down.
const maxIncludeDepth = 100

// errIncludeDepth is the render error for includes nested deeper than
// maxIncludeDepth.
var errIncludeDepth = errors.New("templates: include: maximum include depth exceeded (a template that includes itself?)")

// errIncludeNoEngine is the render error for an include outside an Engine.
var errIncludeNoEngine = errors.New("templates: include: the template is not rendered by an Engine")

// includePair is one name=value pair of {% include ... with %}.
type includePair struct {
	name string
	expr pongo2.IEvaluator
}

// includeTagNode implements Django's {% include %}:
//
//	{% include "blog/_comment.html" %}
//	{% include template_name with comment=c only %}
//
// The name goes through the sources of the engine like any other template
// name. A variable may also hold a list of names, of which the first that
// exists is taken.
type includeTagNode struct {
	start    *pongo2.Token
	nameExpr pongo2.IEvaluator
	with     []includePair
	only     bool
}

// Execute renders the named template with the current context (or, with
// "only", just the given pairs) and tells the render state about it.
func (n *includeTagNode) Execute(ctx *pongo2.ExecutionContext, w pongo2.TemplateWriter) *pongo2.Error {
	st := stateFrom(ctx)
	if st == nil || st.engine == nil {
		return ctx.OrigError(errIncludeNoEngine, n.start)
	}
	nameVal, perr := n.nameExpr.Evaluate(ctx)
	if perr != nil {
		return perr
	}
	name, tpl, err := st.engine.loadFirst(includeNames(nameVal))
	if err != nil {
		return ctx.OrigError(err, n.start)
	}

	inc := make(pongo2.Context, len(ctx.Public)+len(ctx.Private)+len(n.with))
	if n.only {
		// The internal keys travel on, so that tags in the included template
		// still find the engine and the resolver.
		inc[stateContextKey] = st
		if r, ok := ctx.Public[resolverContextKey]; ok {
			inc[resolverContextKey] = r
		}
	} else {
		inc.Update(ctx.Public)
		inc.Update(ctx.Private)
	}
	for _, pair := range n.with {
		val, verr := pair.expr.Evaluate(ctx)
		if verr != nil {
			return verr
		}
		inc[pair.name] = val
	}

	if st.recording {
		st.included = append(st.included, Rendered{Name: name, Context: visibleContext(inc)})
		for _, parent := range st.engine.loader.chain(name) {
			st.included = append(st.included, Rendered{Name: parent, Context: visibleContext(inc)})
		}
	}
	if st.depth >= maxIncludeDepth {
		return ctx.OrigError(errIncludeDepth, n.start)
	}
	st.depth++
	err = tpl.ExecuteWriter(inc, w)
	st.depth--
	if err != nil {
		var inner *pongo2.Error
		if errors.As(err, &inner) {
			return inner
		}
		return ctx.OrigError(err, n.start)
	}
	return nil
}

// includeNames returns the template names a value of {% include %} stands
// for: the string itself, the strings of a list, none for a missing value.
func includeNames(v *pongo2.Value) []string {
	if v.IsNil() {
		return nil
	}
	rv := reflect.ValueOf(v.Interface())
	if rv.IsValid() && (rv.Kind() == reflect.Slice || rv.Kind() == reflect.Array) {
		names := make([]string, 0, rv.Len())
		for i := range rv.Len() {
			names = append(names, pongo2.AsValue(rv.Index(i).Interface()).String())
		}
		return names
	}
	return []string{v.String()}
}

// visibleContext copies an include context for the recorder, without the
// internal keys and with template values unwrapped.
func visibleContext(inc pongo2.Context) map[string]any {
	out := make(map[string]any, len(inc))
	for k, v := range inc {
		if k == stateContextKey || k == resolverContextKey || k == "pongo2" {
			continue
		}
		if pv, ok := v.(*pongo2.Value); ok {
			v = pv.Interface()
		}
		out[k] = v
	}
	return out
}

// includeTagParser parses {% include name [with a=b ...] [only] %}.
func includeTagParser(_ *pongo2.Parser, start *pongo2.Token, args *pongo2.Parser) (pongo2.INodeTag, *pongo2.Error) {
	if args.Remaining() == 0 {
		return nil, args.Error(
			"'include' tag takes at least one argument: the name of the template to be included.", start)
	}
	nameExpr, err := args.ParseExpression()
	if err != nil {
		return nil, err
	}
	node := &includeTagNode{start: start, nameExpr: nameExpr}
	seenWith := false
	for args.Remaining() > 0 {
		switch {
		case args.Match(pongo2.TokenIdentifier, "with") != nil:
			if seenWith {
				return nil, args.Error("The 'with' option was specified more than once.", start)
			}
			seenWith = true
			for {
				key := args.PeekType(pongo2.TokenIdentifier)
				if key == nil || args.PeekN(1, pongo2.TokenSymbol, "=") == nil {
					break
				}
				args.ConsumeN(2)
				expr, verr := args.ParseExpression()
				if verr != nil {
					return nil, verr
				}
				node.with = append(node.with, includePair{name: key.Val, expr: expr})
			}
			if len(node.with) == 0 {
				return nil, args.Error("\"with\" in 'include' tag needs at least one keyword argument.", start)
			}
		case args.Match(pongo2.TokenIdentifier, "only") != nil:
			if node.only {
				return nil, args.Error("The 'only' option was specified more than once.", start)
			}
			node.only = true
		default:
			return nil, args.Error("Unknown argument for 'include' tag: '"+args.Current().Val+"'.", start)
		}
	}
	return node, nil
}
