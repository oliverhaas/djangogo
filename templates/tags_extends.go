package templates

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/flosch/pongo2/v6"
)

// extendsVarTag stands for a Django {% extends %} tag whose parent is not a
// string literal, such as {% extends layout %}. pongo2 links a child to its
// parent when it compiles the child, before any variable has a value, so
// djangoSyntax rewrites
//
//	{% extends layout %}rest
//
// to
//
//	{% _extends_var "layout" layout %}rest{% end_extends_var %}
//
// When it renders, the tag renders a variant of the template in its place:
// the same source, with the {% extends %} tag naming the parent the variable
// holds, see variantName.
const extendsVarTag = "_extends_var"

// stringName is pongo2's name of a template compiled from a string.
const stringName = "<string>"

// errExtendsNoEngine is the render error for a {% extends %} with a variable
// outside an Engine.
var errExtendsNoEngine = errors.New("templates: extends: the template is not rendered by an Engine")

// extendsOverride is a {% extends %} with a variable, resolved: the template
// of the tag and the parent the variable names.
type extendsOverride struct {
	template, parent string
}

// variantName returns the name under which the loader serves the template
// name with the parents overrides give: each template of overrides extends
// the parent given there. Each override goes between NUL bytes, which no
// template name has (see cleanName), with the unit separator between
// template and parent. pongo2 reads the byte 1 as the end of its input, so
// it does not serve. displayName removes the overrides from messages.
func variantName(name string, overrides []extendsOverride) string {
	var b strings.Builder
	b.WriteString(name)
	for _, o := range overrides {
		b.WriteString("\x00" + o.template + "\x1f" + o.parent + "\x00")
	}
	return b.String()
}

// parseVariant splits a name of variantName into the template name and the
// overrides. A plain template name has none.
func parseVariant(name string) (string, []extendsOverride) {
	base, rest, ok := strings.Cut(name, "\x00")
	if !ok {
		return name, nil
	}
	var overrides []extendsOverride
	for pair := range strings.SplitSeq(strings.TrimSuffix(rest, "\x00"), "\x00\x00") {
		tpl, parent, _ := strings.Cut(pair, "\x1f")
		overrides = append(overrides, extendsOverride{template: tpl, parent: parent})
	}
	return base, overrides
}

// overrideOf returns the parent overrides give the template name.
func overrideOf(overrides []extendsOverride, name string) (string, bool) {
	for _, o := range overrides {
		if o.template == name {
			return o.parent, true
		}
	}
	return "", false
}

// overridesRe finds the overrides in a name of variantName.
var overridesRe = regexp.MustCompile("\x00[^\x00]*\x00")

// displayName returns text with each name of variantName in it cut back to
// the template name.
func displayName(text string) string {
	if !strings.Contains(text, "\x00") {
		return text
	}
	return overridesRe.ReplaceAllString(text, "")
}

// variantSource returns the source of the template name with the parents
// overrides give. Its {% extends %} names the parent that overrides give it
// or else the parent it names itself, as a variant with the same overrides,
// so that they hold all the way up the chain. An empty name stands for a
// template given as a string.
func variantSource(src []byte, name string, overrides []extendsOverride) []byte {
	self := name
	if self == "" {
		self = stringName
	}
	return rewrite(src, name, func(parent string, literal bool) (string, bool) {
		if to, ok := overrideOf(overrides, self); ok {
			return variantName(to, overrides), true
		}
		if literal {
			return variantName(parent, overrides), true
		}
		return "", false
	})
}

// variant compiles the template name with the parents overrides give and
// returns it with its name, see variantName. An empty name stands for the
// template given as a string, whose source is src.
func (e *Engine) variant(name string, overrides []extendsOverride, src []byte) (*pongo2.Template, string, error) {
	current := variantName(name, overrides)
	if name != "" {
		tpl, err := e.load(current)
		return tpl, current, err
	}
	var tpl *pongo2.Template
	err := guard("parse string", func() (err error) {
		tpl, err = e.set.FromBytes(variantSource(src, "", overrides))
		return err
	})
	if err != nil && !errors.Is(err, errPanic) {
		err = fmt.Errorf("templates: parse string: %w", e.loadError(err))
	}
	return tpl, current, err
}

// extendsVarNode implements {% extends %} with a variable, see
// extendsVarTag.
type extendsVarNode struct {
	start *pongo2.Token
	// source is the parent as written, for the error message.
	source string
	parent pongo2.IEvaluator
}

// Execute renders, in place of the template, its variant in which this tag
// extends the parent the expression names.
func (n *extendsVarNode) Execute(ctx *pongo2.ExecutionContext, w pongo2.TemplateWriter) *pongo2.Error {
	st := stateFrom(ctx)
	if st == nil || st.engine == nil {
		return ctx.OrigError(errExtendsNoEngine, n.start)
	}
	val, perr := n.parent.Evaluate(ctx)
	if perr != nil {
		return perr
	}
	if !val.IsTrue() {
		got := "''"
		if !val.IsNil() {
			got = pyRepr(val.Interface())
		}
		msg := fmt.Sprintf("Invalid template name in 'extends' tag: %s. Got this from the '%s' variable.", got, n.source)
		return ctx.OrigError(errors.New(msg), n.start)
	}
	parent := val.String()
	if strings.ContainsRune(parent, 0) {
		// No template name has a NUL byte, and variantName uses it.
		return ctx.OrigError(&TemplateDoesNotExist{Name: parent}, n.start)
	}
	// The template being rendered is st.current: the template of this tag
	// or one that extends it. It renders again, with this tag naming parent.
	leaf, overrides := parseVariant(st.current)
	self, _ := parseVariant(n.start.Filename)
	overrides = append(overrides, extendsOverride{template: self, parent: parent})
	tpl, current, err := st.engine.variant(leaf, overrides, st.source)
	if err != nil {
		return ctx.OrigError(err, n.start)
	}
	inc := make(pongo2.Context, len(ctx.Public)+len(ctx.Private))
	inc.Update(ctx.Public)
	inc.Update(ctx.Private)
	if st.recording {
		st.included = append(st.included, Rendered{Name: parent, Context: visibleContext(inc)})
		for _, grand := range st.engine.loader.chain(parent) {
			st.included = append(st.included, Rendered{Name: grand, Context: visibleContext(inc)})
		}
	}
	saved := st.current
	st.current = current
	err = tpl.ExecuteWriter(inc, w)
	st.current = saved
	if err != nil {
		var inner *pongo2.Error
		if errors.As(err, &inner) {
			return inner
		}
		return ctx.OrigError(err, n.start)
	}
	return nil
}

// extendsVarTagParser parses {% _extends_var "source" expression %} and the
// rest of the template, up to {% end_extends_var %}.
func extendsVarTagParser(doc *pongo2.Parser, start *pongo2.Token, args *pongo2.Parser) (pongo2.INodeTag, *pongo2.Error) {
	source := args.MatchType(pongo2.TokenString)
	if source == nil {
		return nil, args.Error("invalid template syntax", start)
	}
	parent, err := args.ParseExpression()
	if err != nil {
		return nil, err
	}
	if args.Remaining() > 0 {
		return nil, args.Error("'extends' takes one argument", start)
	}
	if _, _, err := doc.WrapUntilTag("end" + extendsVarTag); err != nil {
		return nil, err
	}
	return &extendsVarNode{start: start, source: source.Val, parent: parent}, nil
}
