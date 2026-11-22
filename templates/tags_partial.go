package templates

import (
	"errors"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"weak"

	"github.com/flosch/pongo2/v6"
)

// partialTemplateTag wraps the source of a template loaded as
// "name#partial", see partialSource:
//
//	{% _partial_template "partial" %}source{% end_partial_template %}
//
// It parses the source, so that its {% partialdef %} tags define their
// partials, and renders only the named partial.
const partialTemplateTag = "_partial_template"

// errPartialDepth is the render error for partials nested deeper than
// maxIncludeDepth, where Django runs into Python's recursion limit.
var errPartialDepth = errors.New("templates: partial: maximum partial depth exceeded (a partial that renders itself?)")

// partialRegistry holds the partials one template defines, by name.
type partialRegistry struct {
	partials map[string]*pongo2.NodeWrapper
}

// partialRegistries maps the parser of a template to its partialRegistry.
// pongo2 hands every tag of a template the same parser and offers no place
// for data of its own. The key is weak and its entry goes when the template
// does; the nodes of the template hold the registry itself.
var partialRegistries sync.Map // weak.Pointer[pongo2.Parser] -> *partialRegistry

// registryOf returns the partials of the template that doc parses.
func registryOf(doc *pongo2.Parser) *partialRegistry {
	key := weak.Make(doc)
	if reg, ok := partialRegistries.Load(key); ok {
		return reg.(*partialRegistry)
	}
	reg, loaded := partialRegistries.LoadOrStore(key, &partialRegistry{partials: map[string]*pongo2.NodeWrapper{}})
	if !loaded {
		runtime.AddCleanup(doc, func(k weak.Pointer[pongo2.Parser]) { partialRegistries.Delete(k) }, key)
	}
	return reg.(*partialRegistry)
}

// renderPartial renders the body of a partial with the context of the tag
// that asks for it. Like Django's PartialTemplate it starts with a tag state
// of its own, so a {% cycle %} in it starts over.
func renderPartial(ctx *pongo2.ExecutionContext, w pongo2.TemplateWriter, body *pongo2.NodeWrapper, start *pongo2.Token) *pongo2.Error {
	st := stateFrom(ctx)
	if st == nil {
		return body.Execute(ctx, w)
	}
	if st.depth >= maxIncludeDepth {
		return ctx.OrigError(errPartialDepth, start)
	}
	st.depth++
	saved := st.tagState
	st.tagState = nil
	err := body.Execute(ctx, w)
	st.tagState = saved
	st.depth--
	return err
}

// partialDefNode implements Django's {% partialdef %}:
//
//	{% partialdef card inline %}<div>{{ item }}</div>{% endpartialdef %}
//
// It names a piece of the template, which {% partial card %} renders and
// which loads on its own as "page.html#card". With "inline" it also renders
// where it is defined.
type partialDefNode struct {
	body   *pongo2.NodeWrapper
	inline bool
}

// Execute renders the partial in place when it is inline.
func (n *partialDefNode) Execute(ctx *pongo2.ExecutionContext, w pongo2.TemplateWriter) *pongo2.Error {
	if !n.inline {
		return nil
	}
	return n.body.Execute(ctx, w)
}

// partialNode implements Django's {% partial name %}, which renders a
// partial of the same template. The partial can be defined after the tag.
type partialNode struct {
	start    *pongo2.Token
	name     string
	registry *partialRegistry
}

// Execute renders the partial.
func (n *partialNode) Execute(ctx *pongo2.ExecutionContext, w pongo2.TemplateWriter) *pongo2.Error {
	body, ok := n.registry.partials[n.name]
	if !ok {
		msg := fmt.Sprintf("Partial '%s' is not defined in the current template.", n.name)
		return ctx.OrigError(errors.New(msg), n.start)
	}
	return renderPartial(ctx, w, body, n.start)
}

// partialTemplateNode renders the partial a "name#partial" template stands
// for, see partialTemplateTag.
type partialTemplateNode struct {
	start *pongo2.Token
	body  *pongo2.NodeWrapper
}

// Execute renders the partial.
func (n *partialTemplateNode) Execute(ctx *pongo2.ExecutionContext, w pongo2.TemplateWriter) *pongo2.Error {
	return renderPartial(ctx, w, n.body, n.start)
}

// partialDefTagParser parses {% partialdef "name" [inline] %} up to its
// {% endpartialdef %}, after rewrite checked the arguments and quoted
// the name.
func partialDefTagParser(doc *pongo2.Parser, start *pongo2.Token, args *pongo2.Parser) (pongo2.INodeTag, *pongo2.Error) {
	name := args.MatchType(pongo2.TokenString)
	if name == nil {
		return nil, args.Error("'partialdef' tag requires a name", start)
	}
	node := &partialDefNode{inline: args.Match(pongo2.TokenIdentifier, "inline") != nil}
	if args.Remaining() > 0 {
		return nil, args.Error("'partialdef' tag takes at most 2 arguments", start)
	}
	body, end, err := doc.WrapUntilTag("endpartialdef")
	if err != nil {
		return nil, err
	}
	if end.Remaining() > 0 {
		tok := end.Current()
		if end.Match(pongo2.TokenString, name.Val) == nil || end.Remaining() > 0 {
			msg := fmt.Sprintf("Invalid block tag on line %d: 'endpartialdef %s', expected 'endpartialdef' or "+
				"'endpartialdef %s'. Did you forget to register or load this tag?", tok.Line, tok.Val, name.Val)
			return nil, end.Error(msg, tok)
		}
	}
	reg := registryOf(doc)
	if _, ok := reg.partials[name.Val]; ok {
		tplName, _, _ := strings.Cut(displayName(start.Filename), "#")
		msg := fmt.Sprintf("Partial '%s' is already defined in the '%s' template.", name.Val, tplName)
		return nil, args.Error(msg, start)
	}
	reg.partials[name.Val] = body
	node.body = body
	return node, nil
}

// partialTagParser parses {% partial "name" %}, after rewrite checked
// the argument and quoted the name.
func partialTagParser(doc *pongo2.Parser, start *pongo2.Token, args *pongo2.Parser) (pongo2.INodeTag, *pongo2.Error) {
	name := args.MatchType(pongo2.TokenString)
	if name == nil || args.Remaining() > 0 {
		return nil, args.Error("'partial' tag requires a single argument", start)
	}
	return &partialNode{start: start, name: name.Val, registry: registryOf(doc)}, nil
}

// partialTemplateTagParser parses the wrapper of a "name#partial" template.
// A partial the template does not define is a TemplateDoesNotExist, as in
// Django.
func partialTemplateTagParser(doc *pongo2.Parser, start *pongo2.Token, args *pongo2.Parser) (pongo2.INodeTag, *pongo2.Error) {
	name := args.MatchType(pongo2.TokenString)
	if name == nil || args.Remaining() > 0 {
		return nil, args.Error("invalid template syntax", start)
	}
	if _, _, err := doc.WrapUntilTag("end" + partialTemplateTag); err != nil {
		return nil, err
	}
	body, ok := registryOf(doc).partials[name.Val]
	if !ok {
		perr := args.Error("partial not defined", start)
		perr.OrigError = &TemplateDoesNotExist{Name: name.Val}
		return nil, perr
	}
	return &partialTemplateNode{start: start, body: body}, nil
}

// partialSource returns the source of the template loaded as
// "name#partial": the source of name, without its {% extends %} tag,
// wrapped in the tag that renders the partial. The wrapper goes on the first
// line, so the lines in error messages stay right.
func partialSource(src []byte, name, partial string) []byte {
	out := []byte("{% " + partialTemplateTag + " " + quoteName(partial) + " %}")
	out = append(out, rewrite(src, name, func(string, bool) (string, bool) { return "", true })...)
	return append(out, "{% end"+partialTemplateTag+" %}"...)
}

// quoteName returns s as a pongo2 string literal, which knows the escapes
// \" and \\ only.
func quoteName(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}
