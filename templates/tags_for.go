package templates

import (
	"reflect"
	"slices"

	"github.com/flosch/pongo2/v6"
)

// forTagNode implements Django's {% for %}:
//
//	{% for post in posts %}...{% empty %}...{% endfor %}
//	{% for key, value in mapping.items %}   keys in sorted order
//	{% for key, value in mapping %}         the same
//	{% for x, y in points %}                each element a pair
//	{% for post in posts reversed %}
//
// Inside the loop forloop has Django's attributes: counter, counter0,
// revcounter, revcounter0, first, last, length and parentloop. It replaces
// pongo2's tag, whose forloop has capitalized Go names.
type forTagNode struct {
	vars     []string
	iterable pongo2.IEvaluator
	reversed bool
	sorted   bool
	body     *pongo2.NodeWrapper
	empty    *pongo2.NodeWrapper
}

// Execute runs the body once per element, or the empty branch when there is
// nothing to iterate over.
func (n *forTagNode) Execute(ctx *pongo2.ExecutionContext, w pongo2.TemplateWriter) (forErr *pongo2.Error) {
	forCtx := pongo2.NewChildExecutionContext(ctx)
	loop := make(map[string]any, 8)
	if parent, ok := forCtx.Private["forloop"]; ok {
		loop["parentloop"] = parent
	}
	forCtx.Private["forloop"] = loop

	obj, err := n.iterable.Evaluate(forCtx)
	if err != nil {
		return err
	}
	obj.IterateOrder(func(idx, count int, key, value *pongo2.Value) bool {
		n.assign(forCtx, key, value)
		loop["counter"] = idx + 1
		loop["counter0"] = idx
		loop["revcounter"] = count - idx
		loop["revcounter0"] = count - idx - 1
		loop["first"] = idx == 0
		loop["last"] = idx == count-1
		loop["length"] = count
		if err := n.body.Execute(forCtx, w); err != nil {
			forErr = err
			return false
		}
		return true
	}, func() {
		if n.empty != nil {
			forErr = n.empty.Execute(forCtx, w)
		}
	}, n.reversed, n.sorted)
	return forErr
}

// assign binds the loop variables for one element. With one variable it is the
// element (the key, for a map). With several, a map gives key and value and
// any other element is unpacked like a Python tuple; variables the element
// has no value for are nil.
func (n *forTagNode) assign(ctx *pongo2.ExecutionContext, key, value *pongo2.Value) {
	if len(n.vars) == 1 {
		ctx.Private[n.vars[0]] = key
		return
	}
	if value != nil {
		ctx.Private[n.vars[0]] = key
		ctx.Private[n.vars[1]] = value
		for _, name := range n.vars[2:] {
			ctx.Private[name] = nil
		}
		return
	}
	rv := reflect.ValueOf(key.Interface())
	unpack := rv.IsValid() && (rv.Kind() == reflect.Slice || rv.Kind() == reflect.Array)
	for i, name := range n.vars {
		if unpack && i < rv.Len() {
			ctx.Private[name] = rv.Index(i).Interface()
		} else {
			ctx.Private[name] = nil
		}
	}
}

// dictViewFilter returns the filter behind a loop over mapping.items,
// mapping.keys or mapping.values, the methods of a Python dict a Go map does
// not have. djangoSyntax rewrites such a loop to iterate over
// "mapping.items|_dict_items:mapping". When the map has no entry of that
// name, the filter returns what the method would: the keys, the values or
// the key and value pairs, in key order.
func dictViewFilter(view string) pongo2.FilterFunction {
	return func(in, param *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
		if _, isModel := param.Interface().(modelView); isModel || !in.IsNil() {
			return in, nil
		}
		rv := reflect.ValueOf(param.Interface())
		if !rv.IsValid() || rv.Kind() != reflect.Map {
			return in, nil
		}
		keys := rv.MapKeys()
		slices.SortFunc(keys, func(a, b reflect.Value) int {
			return compareValues(a.Interface(), b.Interface())
		})
		out := make([]any, len(keys))
		for i, key := range keys {
			switch view {
			case "keys":
				out[i] = key.Interface()
			case "values":
				out[i] = rv.MapIndex(key).Interface()
			default:
				out[i] = []any{key.Interface(), rv.MapIndex(key).Interface()}
			}
		}
		return pongo2.AsValue(out), nil
	}
}

// forTagParser parses {% for a[, b...] in iterable [reversed] %} up to its
// {% endfor %}, with an optional {% empty %} branch.
func forTagParser(doc *pongo2.Parser, _ *pongo2.Token, args *pongo2.Parser) (pongo2.INodeTag, *pongo2.Error) {
	node := &forTagNode{}
	for {
		name := args.MatchType(pongo2.TokenIdentifier)
		if name == nil {
			return nil, args.Error("'for' statements should use the format 'for x in y'", nil)
		}
		node.vars = append(node.vars, name.Val)
		if args.Match(pongo2.TokenSymbol, ",") == nil {
			break
		}
	}
	if args.Match(pongo2.TokenKeyword, "in") == nil {
		return nil, args.Error("'for' statements should use the format 'for x in y'", nil)
	}
	iterable, err := args.ParseExpression()
	if err != nil {
		return nil, err
	}
	node.iterable = iterable
	for args.Remaining() > 0 {
		switch {
		case args.Match(pongo2.TokenIdentifier, "reversed") != nil:
			node.reversed = true
		case args.Match(pongo2.TokenIdentifier, "sorted") != nil:
			node.sorted = true
		default:
			return nil, args.Error("'for' statements should use the format 'for x in y'", nil)
		}
	}

	body, endArgs, err := doc.WrapUntilTag("empty", "endfor")
	if err != nil {
		return nil, err
	}
	node.body = body
	if endArgs.Count() > 0 {
		return nil, endArgs.Error("Arguments not allowed here.", nil)
	}
	if body.Endtag == "empty" {
		empty, endArgs, err := doc.WrapUntilTag("endfor")
		if err != nil {
			return nil, err
		}
		if endArgs.Count() > 0 {
			return nil, endArgs.Error("Arguments not allowed here.", nil)
		}
		node.empty = empty
	}
	return node, nil
}
