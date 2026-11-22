package templates

import (
	"bytes"
	"errors"
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
	val, _ = filterOut(val, nil)
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

// widthratioTagNode implements Django's {% widthratio value max width %}: the
// ratio of value to max, scaled to width and rounded like Python rounds (half
// to even). pongo2's tag rounds differently.
type widthratioTagNode struct {
	value, max, width pongo2.IEvaluator
	asVar             string
}

// errWidthratioWidth is Django's error for a width that is not an integer.
var errWidthratioWidth = errors.New("widthratio final argument must be a number")

// pyFloat converts a template value like Python's float(): a number, a
// boolean or a string that holds a number ("1.5", "inf", "nan").
func pyFloat(v *pongo2.Value) (float64, bool) {
	switch {
	case v.IsNumber():
		return v.Float(), true
	case v.IsBool():
		if v.Bool() {
			return 1, true
		}
		return 0, true
	case v.IsString():
		f, err := strconv.ParseFloat(strings.TrimSpace(v.String()), 64)
		return f, err == nil || errors.Is(err, strconv.ErrRange)
	}
	return 0, false
}

// Execute prints or stores the scaled ratio: "0" when max is zero, "" when
// value or max is not a number.
func (n *widthratioTagNode) Execute(ctx *pongo2.ExecutionContext, w pongo2.TemplateWriter) *pongo2.Error {
	vals := make([]*pongo2.Value, 3)
	for i, expr := range []pongo2.IEvaluator{n.value, n.max, n.width} {
		val, err := expr.Evaluate(ctx)
		if err != nil {
			return err
		}
		vals[i] = val
	}
	width, ok := asInt(vals[2])
	if !ok {
		return ctx.OrigError(errWidthratioWidth, nil)
	}
	value, okValue := pyFloat(vals[0])
	limit, okLimit := pyFloat(vals[1])
	result := ""
	switch {
	case !okValue || !okLimit:
	case limit == 0:
		result = "0"
	default:
		ratio := math.RoundToEven(value / limit * float64(width))
		if !math.IsNaN(ratio) && !math.IsInf(ratio, 0) {
			result = strconv.FormatFloat(ratio, 'f', 0, 64)
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

// TimeZone is the current time zone of the templates, Django's TIME_ZONE: the
// {% now %} tag prints the time in it. It is UTC unless the application sets
// it from Settings.TimeZone.
var TimeZone = time.UTC

// Execute prints or stores the current time in the given format.
func (n *nowTagNode) Execute(ctx *pongo2.ExecutionContext, w pongo2.TemplateWriter) *pongo2.Error {
	format, err := n.format.Evaluate(ctx)
	if err != nil {
		return err
	}
	out := formatDate(timeNow().In(TimeZone), format.String())
	if n.asVar != "" {
		ctx.Private[n.asVar] = out
		return nil
	}
	// Django writes the output of a tag as it is; only {{ }} escapes.
	if _, err := w.WriteString(out); err != nil {
		return ctx.OrigError(err, nil)
	}
	return nil
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

// syntaxErrorTag is the tag rewrite puts in place of a construct that
// Django rejects and pongo2 would accept, see syntaxError.
const syntaxErrorTag = "_syntax_error"

// syntaxErrorTagParser fails with the message the tag carries.
func syntaxErrorTagParser(_ *pongo2.Parser, start *pongo2.Token, args *pongo2.Parser) (pongo2.INodeTag, *pongo2.Error) {
	msg := args.MatchType(pongo2.TokenString)
	if msg == nil {
		return nil, args.Error("invalid template syntax", start)
	}
	return nil, args.Error(msg.Val, start)
}
