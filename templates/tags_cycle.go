package templates

import (
	"bytes"
	"fmt"
	"reflect"
	"runtime"
	"sync"
	"weak"

	"github.com/flosch/pongo2/v6"
)

// parseScope is what the tags of one template learn while it is compiled:
// Django keeps it on its parser, so that a cycle name is known within its
// template only.
type parseScope struct {
	// namedCycles holds the cycles defined with "as name".
	namedCycles map[string]*cycleTagNode
	// lastCycle is the cycle {% resetcycle %} without a name resets.
	lastCycle *cycleTagNode
}

// parseScopes holds the parseScope of every template being compiled, keyed
// by its pongo2 parser, which pongo2 hands to every tag of the template. The
// key is weak and a cleanup drops the entry once the parser is gone, so a
// compiled template leaves nothing behind.
var (
	parseScopesMu sync.Mutex
	parseScopes   = map[weak.Pointer[pongo2.Parser]]*parseScope{}
)

// scopeOf returns the parseScope of the template doc parses.
func scopeOf(doc *pongo2.Parser) *parseScope {
	key := weak.Make(doc)
	parseScopesMu.Lock()
	defer parseScopesMu.Unlock()
	if s, ok := parseScopes[key]; ok {
		return s
	}
	s := &parseScope{namedCycles: map[string]*cycleTagNode{}}
	parseScopes[key] = s
	runtime.AddCleanup(doc, func(k weak.Pointer[pongo2.Parser]) {
		parseScopesMu.Lock()
		delete(parseScopes, k)
		parseScopesMu.Unlock()
	}, key)
	return s
}

// tagState returns the state a tag keeps for the render of the current
// template, created by init on first use. An included template starts with
// its own, as in Django, where each template render has a fresh render
// context.
func tagState[T any](ctx *pongo2.ExecutionContext, key any, init func() T) T {
	st := stateFrom(ctx)
	if st == nil {
		return init()
	}
	if st.tagState == nil {
		st.tagState = make(map[any]any)
	}
	if v, ok := st.tagState[key].(T); ok {
		return v
	}
	v := init()
	st.tagState[key] = v
	return v
}

// cycleTagNode implements Django's {% cycle %}:
//
//	{% cycle 'odd' 'even' %}
//	{% cycle a b c as color %}           also stores the value
//	{% cycle a b c as color silent %}    only stores it
//	{% cycle color %}                    advances the cycle named color
//
// It replaces pongo2's tag, which prints variables unescaped.
type cycleTagNode struct {
	args   []tagArg
	asVar  string
	silent bool
}

// cyclePosition is the position of a cycle within a render.
type cyclePosition struct{ next int }

// Execute prints the next value of the cycle.
func (n *cycleTagNode) Execute(ctx *pongo2.ExecutionContext, w pongo2.TemplateWriter) *pongo2.Error {
	pos := tagState(ctx, n, func() *cyclePosition { return &cyclePosition{} })
	idx := pos.next % len(n.args)
	pos.next = idx + 1
	val, err := n.args[idx].evaluate(ctx)
	if err != nil {
		return err
	}
	if n.asVar != "" {
		ctx.Private[n.asVar] = val
	}
	if n.silent {
		return nil
	}
	return writeOutput(ctx, w, val)
}

// cycleTagParser parses {% cycle v1 v2 ... [as name [silent]] %} and
// {% cycle name %}.
func cycleTagParser(doc *pongo2.Parser, _ *pongo2.Token, args *pongo2.Parser) (pongo2.INodeTag, *pongo2.Error) {
	scope := scopeOf(doc)
	if args.Count() == 1 {
		name := args.Current()
		if name.Typ != pongo2.TokenIdentifier {
			return nil, args.Error(fmt.Sprintf("No named cycles in template. '%s' is not defined", name.Val), nil)
		}
		node, ok := scope.namedCycles[name.Val]
		if !ok {
			if len(scope.namedCycles) == 0 {
				return nil, args.Error(fmt.Sprintf("No named cycles in template. '%s' is not defined", name.Val), nil)
			}
			return nil, args.Error(fmt.Sprintf("Named cycle '%s' does not exist", name.Val), nil)
		}
		return node, nil
	}
	tagArgs, err := parseTagArgs(args)
	if err != nil {
		return nil, err
	}
	node := &cycleTagNode{args: tagArgs}
	if len(node.args) == 0 {
		return nil, args.Error("'cycle' tag requires at least two arguments", nil)
	}
	asVar, err := parseAsVar(args)
	if err != nil {
		return nil, err
	}
	node.asVar = asVar
	if asVar != "" {
		if args.Remaining() > 0 {
			flag := args.Current()
			if flag.Val != "silent" {
				return nil, args.Error(fmt.Sprintf("Only 'silent' flag is allowed after cycle's name, not '%s'.", flag.Val), nil)
			}
			args.Consume()
			node.silent = true
		}
		scope.namedCycles[asVar] = node
	}
	if args.Remaining() > 0 {
		return nil, args.Error("Malformed arguments to 'cycle' tag.", nil)
	}
	scope.lastCycle = node
	return node, nil
}

// resetcycleTagNode implements Django's {% resetcycle [name] %}: the cycle
// starts again from its first value.
type resetcycleTagNode struct {
	cycle *cycleTagNode
}

// Execute resets the cycle.
func (n *resetcycleTagNode) Execute(ctx *pongo2.ExecutionContext, _ pongo2.TemplateWriter) *pongo2.Error {
	tagState(ctx, n.cycle, func() *cyclePosition { return &cyclePosition{} }).next = 0
	return nil
}

// resetcycleTagParser parses {% resetcycle [name] %}. Without a name it
// resets the last {% cycle %} before it.
func resetcycleTagParser(doc *pongo2.Parser, _ *pongo2.Token, args *pongo2.Parser) (pongo2.INodeTag, *pongo2.Error) {
	scope := scopeOf(doc)
	switch args.Count() {
	case 0:
		if scope.lastCycle == nil {
			return nil, args.Error("No cycles in template.", nil)
		}
		return &resetcycleTagNode{cycle: scope.lastCycle}, nil
	case 1:
		name := args.Current().Val
		node, ok := scope.namedCycles[name]
		if !ok {
			return nil, args.Error(fmt.Sprintf("Named cycle '%s' does not exist.", name), nil)
		}
		return &resetcycleTagNode{cycle: node}, nil
	}
	return nil, args.Error("'resetcycle' tag accepts at most one argument.", nil)
}

// ifchangedTagNode implements Django's {% ifchanged %}: the body is printed
// when its output (or, with arguments, the value of one of them) differs from
// the last time. Inside a loop the last value belongs to that loop, so an
// inner loop starts over with every pass of the outer one. pongo2's tag keeps
// it for the whole render.
type ifchangedTagNode struct {
	watched  []pongo2.IEvaluator
	body     *pongo2.NodeWrapper
	elseBody *pongo2.NodeWrapper
}

// ifchangedLast holds the last value an {% ifchanged %} saw.
type ifchangedLast struct {
	set   bool
	value any
}

// last returns where the tag keeps its last value: in the innermost forloop,
// or in the render of the template outside a loop.
func (n *ifchangedTagNode) last(ctx *pongo2.ExecutionContext) *ifchangedLast {
	loop, ok := ctx.Private["forloop"].(map[string]any)
	if !ok {
		loop, ok = ctx.Public["forloop"].(map[string]any)
	}
	if !ok {
		return tagState(ctx, n, func() *ifchangedLast { return &ifchangedLast{} })
	}
	// The key cannot be written in a template, so it cannot clash with
	// Django's names in forloop.
	key := fmt.Sprintf("\x00ifchanged %p", n)
	last, ok := loop[key].(*ifchangedLast)
	if !ok {
		last = &ifchangedLast{}
		loop[key] = last
	}
	return last
}

// Execute prints the body when the watched values changed, else the
// {% else %} branch.
func (n *ifchangedTagNode) Execute(ctx *pongo2.ExecutionContext, w pongo2.TemplateWriter) *pongo2.Error {
	last := n.last(ctx)
	var compare any
	var rendered *bytes.Buffer
	if len(n.watched) == 0 {
		rendered = &bytes.Buffer{}
		if err := n.body.Execute(ctx, rendered); err != nil {
			return err
		}
		compare = rendered.String()
	} else {
		values := make([]any, len(n.watched))
		for i, expr := range n.watched {
			val, err := expr.Evaluate(ctx)
			if err != nil {
				// Django resolves the arguments with ignore_failures.
				values[i] = nil
				continue
			}
			values[i] = val.Interface()
		}
		compare = values
	}
	if !last.set || !reflect.DeepEqual(compare, last.value) {
		last.set, last.value = true, compare
		if rendered != nil {
			if _, err := w.WriteString(rendered.String()); err != nil {
				return ctx.OrigError(err, nil)
			}
			return nil
		}
		return n.body.Execute(ctx, w)
	}
	if n.elseBody != nil {
		return n.elseBody.Execute(ctx, w)
	}
	return nil
}

// ifchangedTagParser parses {% ifchanged [values...] %} up to
// {% endifchanged %}, with an optional {% else %} branch.
func ifchangedTagParser(doc *pongo2.Parser, _ *pongo2.Token, args *pongo2.Parser) (pongo2.INodeTag, *pongo2.Error) {
	node := &ifchangedTagNode{}
	for args.Remaining() > 0 {
		expr, err := args.ParseExpression()
		if err != nil {
			return nil, err
		}
		node.watched = append(node.watched, expr)
	}
	body, endArgs, err := doc.WrapUntilTag("else", "endifchanged")
	if err != nil {
		return nil, err
	}
	if endArgs.Count() > 0 {
		return nil, endArgs.Error("Arguments not allowed here.", nil)
	}
	node.body = body
	if body.Endtag == "else" {
		elseBody, endArgs, err := doc.WrapUntilTag("endifchanged")
		if err != nil {
			return nil, err
		}
		if endArgs.Count() > 0 {
			return nil, endArgs.Error("Arguments not allowed here.", nil)
		}
		node.elseBody = elseBody
	}
	return node, nil
}
