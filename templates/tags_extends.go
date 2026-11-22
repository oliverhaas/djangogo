package templates

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/flosch/pongo2/v6"
)

// extendsVarTag stands for a Django {% extends %} tag whose parent is not a
// string literal, such as {% extends layout %}. pongo2 links a child to its
// parent when it compiles the child, before any variable has a value, so
// rewrite changes
//
//	{% extends layout %}rest
//
// to
//
//	{% _extends_var "layout" layout %}rest{% end_extends_var %}
//
// When it renders, the tag renders a variant of the template in its place:
// the same source, with the {% extends %} tag naming the parent the variable
// holds, see served.
const extendsVarTag = "_extends_var"

// stringName is pongo2's name of a template compiled from a string.
const stringName = "<string>"

// stringOrigin is the origin of a template given as a string, which no
// source has.
var stringOrigin = origin{name: stringName, source: -1}

// errExtendsNoEngine is the render error for a {% extends %} with a variable
// outside an Engine.
var errExtendsNoEngine = errors.New("templates: extends: the template is not rendered by an Engine")

// origin is a template of a source: its name and the index of the source.
// Two sources with the same name hold two templates.
type origin struct {
	name   string
	source int
}

// extendsOverride is a {% extends %} with a variable, resolved: the depth of
// its template in the chain (0 for the template rendered) and the parent the
// variable names.
type extendsOverride struct {
	depth  int
	parent string
}

// served is what a name the loader serves to pongo2 stands for. pongo2 links
// a child to its parent by name, so the name of a parent carries what Django
// keeps while it renders a chain:
//
//   - history, the templates below in the chain, the leaf first. As in
//     Django, a parent is the first template of its name that is not in the
//     history, so a template can extend its namesake in a later source.
//   - overrides, the parents a render gave the {% extends %} tags with a
//     variable, see extendsVarTag.
//
// A served name is the template name, followed by the history and the
// overrides between NUL bytes, which no template name has (see cleanName).
// displayName removes them from messages.
type served struct {
	// name is the template name, with "#partial" for a partial.
	name      string
	history   []origin
	overrides []extendsOverride
}

// String returns the served name.
func (s served) String() string {
	if len(s.history) == 0 && len(s.overrides) == 0 {
		return s.name
	}
	v := url.Values{}
	for _, o := range s.history {
		v.Add("h", strconv.Itoa(o.source)+":"+o.name)
	}
	for _, o := range s.overrides {
		v.Add("o", strconv.Itoa(o.depth)+":"+o.parent)
	}
	return s.name + "\x00" + v.Encode() + "\x00"
}

// parseServed reads a served name. A plain template name has no history and
// no overrides.
func parseServed(name string) served {
	base, rest, ok := strings.Cut(name, "\x00")
	if !ok {
		return served{name: name}
	}
	s := served{name: base}
	v, _ := url.ParseQuery(strings.TrimSuffix(rest, "\x00"))
	for _, h := range v["h"] {
		i, n, _ := strings.Cut(h, ":")
		src, _ := strconv.Atoi(i)
		s.history = append(s.history, origin{name: n, source: src})
	}
	for _, o := range v["o"] {
		i, parent, _ := strings.Cut(o, ":")
		depth, _ := strconv.Atoi(i)
		s.overrides = append(s.overrides, extendsOverride{depth: depth, parent: parent})
	}
	return s
}

// override returns the parent the overrides give the template at depth.
func (s served) override(depth int) (string, bool) {
	for _, o := range s.overrides {
		if o.depth == depth {
			return o.parent, true
		}
	}
	return "", false
}

// servedRe finds the history and the overrides in a served name.
var servedRe = regexp.MustCompile("\x00[^\x00]*\x00")

// displayName returns text with each served name in it cut back to the
// template name.
func displayName(text string) string {
	if !strings.Contains(text, "\x00") {
		return text
	}
	return servedRe.ReplaceAllString(text, "")
}

// variant compiles the template name with the parents overrides give and
// returns it with its served name. An empty name stands for the template
// given as a string, whose source is src.
func (e *Engine) variant(name string, overrides []extendsOverride, src []byte) (*pongo2.Template, string, error) {
	current := served{name: name, overrides: overrides}.String()
	if name != "" {
		tpl, err := e.load(current)
		return tpl, current, err
	}
	var tpl *pongo2.Template
	err := guard("parse string", func() (err error) {
		tpl, err = e.set.FromBytes(e.loader.serve(current, src, "", served{overrides: overrides}, stringOrigin))
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
		// No template name has a NUL byte, and a served name uses it.
		return ctx.OrigError(&TemplateDoesNotExist{Name: parent}, n.start)
	}
	// The template being rendered is st.current: the template of this tag
	// or one that extends it. It renders again, with this tag naming parent.
	leaf := parseServed(st.current)
	depth := len(parseServed(n.start.Filename).history)
	overrides := append(slices.Clip(leaf.overrides), extendsOverride{depth: depth, parent: parent})
	tpl, current, err := st.engine.variant(leaf.name, overrides, st.source)
	if err != nil {
		return ctx.OrigError(err, n.start)
	}
	inc := make(pongo2.Context, len(ctx.Public)+len(ctx.Private))
	inc.Update(ctx.Public)
	inc.Update(ctx.Private)
	if st.recording {
		// The templates below this one in the chain are recorded already.
		if chain := st.engine.loader.chain(current); len(chain) > depth {
			for _, name := range chain[depth:] {
				st.included = append(st.included, Rendered{Name: name, Context: visibleContext(inc)})
			}
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
