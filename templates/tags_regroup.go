package templates

import (
	"reflect"

	"github.com/flosch/pongo2/v6"
)

// groupedResult is one group of {% regroup %}, Django's GroupedResult named
// tuple: group.grouper and group.list read it, and
// {% for grouper, list in grouped %} unpacks it.
type groupedResult map[string]any

// regroupTagNode implements Django's {% regroup %}:
//
//	{% regroup musicians by instrument as grouped %}
//
// It stores in grouped one groupedResult per run of consecutive elements
// with the same key, in the order of the list, so the list must be sorted
// by the key. rewrite changes the key expression to start with the
// variable name ("grouped.instrument"), as Django does: the tag binds each
// element to that name while it evaluates the key.
type regroupTagNode struct {
	target  pongo2.IEvaluator
	key     pongo2.IEvaluator
	varName string
}

// Execute groups the target and stores the groups. A target that does not
// resolve gives an empty list, as Django resolves it with ignore_failures.
func (n *regroupTagNode) Execute(ctx *pongo2.ExecutionContext, _ pongo2.TemplateWriter) *pongo2.Error {
	groups := []any{}
	if target, err := n.target.Evaluate(ctx); err == nil && !target.IsNil() {
		groups = n.group(ctx, target)
	}
	ctx.Private[n.varName] = groups
	return nil
}

// group splits the items of target into groups of consecutive items with an
// equal key.
func (n *regroupTagNode) group(ctx *pongo2.ExecutionContext, target *pongo2.Value) []any {
	groups := []any{}
	var current groupedResult
	var last any
	target.Iterate(func(_, _ int, item, _ *pongo2.Value) bool {
		ctx.Private[n.varName] = item.Interface()
		// Django resolves the key with ignore_failures, which gives None.
		var key, grouper any
		if val, err := n.key.Evaluate(ctx); err == nil {
			key, grouper = val.Interface(), val.Interface()
			if isSafe(val) {
				// A key such as bar|join:"" stays marked safe.
				grouper = val
			}
		}
		if current == nil || !groupKeyEqual(last, key) {
			current = groupedResult{"grouper": grouper, "list": []any{}}
			groups = append(groups, current)
			last = key
		}
		current["list"] = append(current["list"].([]any), item.Interface())
		return true
	}, func() {})
	return groups
}

// groupKeyEqual reports whether two keys of {% regroup %} are equal, as
// Python's == compares them: numbers by value, model instances by model and
// primary key, everything else deeply.
func groupKeyEqual(a, b any) bool {
	if ma, ok := a.(modelView); ok {
		if mb, ok := b.(modelView); ok && ma["pk"] != nil && mb["pk"] != nil {
			return ma.className() == mb.className() && groupKeyEqual(ma["pk"], mb["pk"])
		}
	}
	va, vb := pongo2.AsValue(a), pongo2.AsValue(b)
	if va.IsNumber() && vb.IsNumber() {
		return va.Float() == vb.Float()
	}
	if ta, ok := asTime(va); ok {
		if tb, ok := asTime(vb); ok {
			return ta.Equal(tb)
		}
	}
	return reflect.DeepEqual(a, b)
}

// regroupTagParser parses {% regroup target by key as name %}, after
// rewrite checked the arguments and prefixed the key with the name.
func regroupTagParser(_ *pongo2.Parser, _ *pongo2.Token, args *pongo2.Parser) (pongo2.INodeTag, *pongo2.Error) {
	target, err := args.ParseExpression()
	if err != nil {
		return nil, err
	}
	if args.Match(pongo2.TokenIdentifier, "by") == nil {
		return nil, args.Error("second argument to 'regroup' tag must be 'by'", nil)
	}
	key, err := args.ParseExpression()
	if err != nil {
		return nil, err
	}
	if args.Match(pongo2.TokenKeyword, "as") == nil {
		return nil, args.Error("next-to-last argument to 'regroup' tag must be 'as'", nil)
	}
	name := args.MatchType(pongo2.TokenIdentifier)
	if name == nil || args.Remaining() > 0 {
		return nil, args.Error("'regroup' tag takes five arguments", nil)
	}
	return &regroupTagNode{target: target, key: key, varName: name.Val}, nil
}
