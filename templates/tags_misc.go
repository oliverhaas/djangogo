package templates

import (
	"bytes"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/flosch/pongo2/v6"
)

// writeOutput writes a tag's result the way a variable is printed: escaped
// under autoescape when it is a string that is not marked safe.
func writeOutput(ctx *pongo2.ExecutionContext, w pongo2.TemplateWriter, val *pongo2.Value) *pongo2.Error {
	out := val.String()
	if ctx.Autoescape && val.IsString() && !isSafe(val) {
		out = escapeHTML(out)
	}
	if _, err := w.WriteString(out); err != nil {
		return ctx.OrigError(err, nil)
	}
	return nil
}

// parseAsVar parses the optional trailing "as name" of a tag and returns the
// name, "" when there is none.
func parseAsVar(args *pongo2.Parser) (string, *pongo2.Error) {
	if args.Match(pongo2.TokenKeyword, "as") == nil {
		return "", nil
	}
	name := args.MatchType(pongo2.TokenIdentifier)
	if name == nil {
		return "", args.Error("Expected a variable name after 'as'.", nil)
	}
	return name.Val, nil
}

// loadTagNode implements {% load %} as a tag without effect. The tags and
// filters of this package are always available, so a template written for
// Django keeps its {% load static %} line.
type loadTagNode struct{}

// Execute writes nothing.
func (loadTagNode) Execute(*pongo2.ExecutionContext, pongo2.TemplateWriter) *pongo2.Error { return nil }

// loadTagParser accepts any arguments.
func loadTagParser(_ *pongo2.Parser, _ *pongo2.Token, args *pongo2.Parser) (pongo2.INodeTag, *pongo2.Error) {
	args.ConsumeN(args.Remaining())
	return loadTagNode{}, nil
}

// spacelessTagNode implements Django's {% spaceless %}: the white space
// between the HTML tags of its content is removed, and so is the white space
// around the content, which pongo2's tag keeps.
type spacelessTagNode struct {
	body *pongo2.NodeWrapper
}

// spacesBetweenTags finds the white space between two HTML tags.
var spacesBetweenTags = regexp.MustCompile(`>\s+<`)

// Execute renders the content and writes it without the white space.
func (n *spacelessTagNode) Execute(ctx *pongo2.ExecutionContext, w pongo2.TemplateWriter) *pongo2.Error {
	var buf bytes.Buffer
	if err := n.body.Execute(ctx, &buf); err != nil {
		return err
	}
	out := spacesBetweenTags.ReplaceAllString(strings.TrimSpace(buf.String()), "><")
	if _, err := w.WriteString(out); err != nil {
		return ctx.OrigError(err, nil)
	}
	return nil
}

// spacelessTagParser parses {% spaceless %} up to {% endspaceless %}.
func spacelessTagParser(doc *pongo2.Parser, _ *pongo2.Token, args *pongo2.Parser) (pongo2.INodeTag, *pongo2.Error) {
	if args.Remaining() > 0 {
		return nil, args.Error("'spaceless' takes no arguments", nil)
	}
	body, endArgs, err := doc.WrapUntilTag("endspaceless")
	if err != nil {
		return nil, err
	}
	if endArgs.Count() > 0 {
		return nil, endArgs.Error("Arguments not allowed here.", nil)
	}
	return &spacelessTagNode{body: body}, nil
}

// tagArg is an argument of a tag that prints one of its arguments.
type tagArg struct {
	expr pongo2.IEvaluator
	// literal is set for a string literal of the template. Django prints
	// such a string as written, without escaping it.
	literal bool
}

// evaluate returns the value of the argument, marked safe for a literal.
func (a tagArg) evaluate(ctx *pongo2.ExecutionContext) (*pongo2.Value, *pongo2.Error) {
	val, err := a.expr.Evaluate(ctx)
	if err == nil && a.literal {
		val = pongo2.AsSafeValue(val.String())
	}
	return val, err
}

// parseTagArgs parses the arguments of such a tag, up to an "as" or the end.
func parseTagArgs(args *pongo2.Parser) ([]tagArg, *pongo2.Error) {
	var out []tagArg
	for args.Remaining() > 0 && args.Peek(pongo2.TokenKeyword, "as") == nil {
		// A string followed by a filter or an operator is an expression.
		literal := args.PeekType(pongo2.TokenString) != nil && args.PeekTypeN(1, pongo2.TokenSymbol) == nil
		expr, err := args.ParseExpression()
		if err != nil {
			return nil, err
		}
		out = append(out, tagArg{expr: expr, literal: literal})
	}
	return out, nil
}

// firstofTagNode implements Django's {% firstof %}: the first of its
// arguments that is true, printed or, with "as name", stored. pongo2's tag
// has no "as".
type firstofTagNode struct {
	args  []tagArg
	asVar string
}

// Execute prints or stores the first true argument, or nothing (an empty
// string for "as") when there is none.
func (n *firstofTagNode) Execute(ctx *pongo2.ExecutionContext, w pongo2.TemplateWriter) *pongo2.Error {
	first := pongo2.AsValue("")
	for _, arg := range n.args {
		val, err := arg.evaluate(ctx)
		if err != nil {
			return err
		}
		if val.IsTrue() {
			first = val
			break
		}
	}
	if n.asVar != "" {
		ctx.Private[n.asVar] = first
		return nil
	}
	return writeOutput(ctx, w, first)
}

// firstofTagParser parses {% firstof v1 v2 ... [as name] %}.
func firstofTagParser(_ *pongo2.Parser, _ *pongo2.Token, args *pongo2.Parser) (pongo2.INodeTag, *pongo2.Error) {
	tagArgs, err := parseTagArgs(args)
	if err != nil {
		return nil, err
	}
	node := &firstofTagNode{args: tagArgs}
	if len(node.args) == 0 {
		return nil, args.Error("'firstof' statement requires at least one argument", nil)
	}
	asVar, err := parseAsVar(args)
	if err != nil {
		return nil, err
	}
	node.asVar = asVar
	if args.Remaining() > 0 {
		return nil, args.Error("Malformed arguments to 'firstof' tag.", nil)
	}
	return node, nil
}

// cycleTagNode implements Django's {% cycle %}:
//
//	{% cycle 'odd' 'even' %}
//	{% cycle a b c as color %}           also stores the value
//	{% cycle a b c as color silent %}    only stores it
//
// It replaces pongo2's tag, which prints variables unescaped.
type cycleTagNode struct {
	args   []tagArg
	asVar  string
	silent bool
}

// Execute prints the next value of the cycle. The position is kept per render.
func (n *cycleTagNode) Execute(ctx *pongo2.ExecutionContext, w pongo2.TemplateWriter) *pongo2.Error {
	idx := 0
	if st := stateFrom(ctx); st != nil {
		if st.cycles == nil {
			st.cycles = make(map[*cycleTagNode]int)
		}
		idx = st.cycles[n]
		st.cycles[n] = idx + 1
	}
	val, err := n.args[idx%len(n.args)].evaluate(ctx)
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

// cycleTagParser parses {% cycle v1 v2 ... [as name [silent]] %}.
func cycleTagParser(_ *pongo2.Parser, _ *pongo2.Token, args *pongo2.Parser) (pongo2.INodeTag, *pongo2.Error) {
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
	if asVar != "" && args.Match(pongo2.TokenIdentifier, "silent") != nil {
		node.silent = true
	}
	if args.Remaining() > 0 {
		return nil, args.Error("Malformed arguments to 'cycle' tag.", nil)
	}
	return node, nil
}

// widthratioTagNode implements Django's {% widthratio value max width %}: the
// ratio of value to max, scaled to width and rounded like Python rounds (half
// to even). pongo2's tag rounds differently.
type widthratioTagNode struct {
	value, max, width pongo2.IEvaluator
	asVar             string
}

// Execute prints or stores the scaled ratio; "0" when max is zero.
func (n *widthratioTagNode) Execute(ctx *pongo2.ExecutionContext, w pongo2.TemplateWriter) *pongo2.Error {
	var nums [3]float64
	for i, expr := range []pongo2.IEvaluator{n.value, n.max, n.width} {
		val, err := expr.Evaluate(ctx)
		if err != nil {
			return err
		}
		nums[i] = val.Float()
	}
	result := "0"
	if nums[1] != 0 {
		ratio := math.RoundToEven(nums[0] / nums[1] * nums[2])
		if math.IsNaN(ratio) || math.IsInf(ratio, 0) {
			result = ""
		} else {
			result = strconv.FormatInt(int64(ratio), 10)
		}
	}
	if n.asVar != "" {
		ctx.Private[n.asVar] = result
		return nil
	}
	if _, err := w.WriteString(result); err != nil {
		return ctx.OrigError(err, nil)
	}
	return nil
}

// widthratioTagParser parses {% widthratio value max width [as name] %}.
func widthratioTagParser(_ *pongo2.Parser, _ *pongo2.Token, args *pongo2.Parser) (pongo2.INodeTag, *pongo2.Error) {
	node := &widthratioTagNode{}
	for _, target := range []*pongo2.IEvaluator{&node.value, &node.max, &node.width} {
		if args.Remaining() == 0 {
			return nil, args.Error("widthratio takes at least three arguments", nil)
		}
		expr, err := args.ParseExpression()
		if err != nil {
			return nil, err
		}
		*target = expr
	}
	asVar, err := parseAsVar(args)
	if err != nil {
		return nil, err
	}
	node.asVar = asVar
	if args.Remaining() > 0 {
		return nil, args.Error("widthratio takes at least three arguments", nil)
	}
	return node, nil
}

// nowTagNode implements Django's {% now "format" %} with Django's date format
// characters (see the date filter). pongo2's tag takes a Go layout.
type nowTagNode struct {
	format pongo2.IEvaluator
	asVar  string
}

// timeNow returns the current time; a test replaces it.
var timeNow = time.Now

// Execute prints or stores the current time in the given format.
func (n *nowTagNode) Execute(ctx *pongo2.ExecutionContext, w pongo2.TemplateWriter) *pongo2.Error {
	format, err := n.format.Evaluate(ctx)
	if err != nil {
		return err
	}
	out := formatDate(timeNow(), format.String())
	if n.asVar != "" {
		ctx.Private[n.asVar] = out
		return nil
	}
	return writeOutput(ctx, w, pongo2.AsValue(out))
}

// nowTagParser parses {% now "format" [as name] %}.
func nowTagParser(_ *pongo2.Parser, _ *pongo2.Token, args *pongo2.Parser) (pongo2.INodeTag, *pongo2.Error) {
	if args.Remaining() == 0 {
		return nil, args.Error("'now' statement takes one argument", nil)
	}
	format, err := args.ParseExpression()
	if err != nil {
		return nil, err
	}
	asVar, err := parseAsVar(args)
	if err != nil {
		return nil, err
	}
	if args.Remaining() > 0 {
		return nil, args.Error("'now' statement takes one argument", nil)
	}
	return &nowTagNode{format: format, asVar: asVar}, nil
}
