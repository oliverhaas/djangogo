package templates

import (
	"runtime/debug"
	"strings"

	"github.com/flosch/pongo2/v6"
)

// debugTagNode implements Django's {% debug %}: with Debug set on the engine
// (see Engine.SetDebug), the variables of the context and the modules of
// the build, as Python's pprint writes them; nothing otherwise. The output
// is always escaped.
//
// Django prints one dict per context level, the innermost first. pongo2
// keeps two levels, so the tag prints the variables the template set (in a
// loop, {% with %} and so on) if there are any, then the variables of the
// render, then the builtins True, False and None. Go has no sys.modules:
// the tag prints the modules of the build with their versions in its place.
type debugTagNode struct{}

// Execute writes the context and the modules when Debug is set.
func (debugTagNode) Execute(ctx *pongo2.ExecutionContext, w pongo2.TemplateWriter) *pongo2.Error {
	st := stateFrom(ctx)
	if st == nil || st.engine == nil || !st.engine.set.Debug {
		return nil
	}
	globals := st.engine.set.Globals
	local := map[string]any{}
	for k, v := range ctx.Private {
		if k != "pongo2" {
			local[k] = v
		}
	}
	render := map[string]any{}
	for k, v := range ctx.Public {
		if _, global := globals[k]; global || k == stateContextKey || k == resolverContextKey {
			continue
		}
		render[k] = v
	}
	var b strings.Builder
	if len(local) > 0 {
		b.WriteString(pformat(local))
	}
	b.WriteString(pformat(render))
	b.WriteString(pformat(map[string]any(globals)))
	b.WriteString("\n\n")
	b.WriteString(pformat(buildModules()))
	if _, err := w.WriteString(escapeHTML(b.String())); err != nil {
		return ctx.OrigError(err, nil)
	}
	return nil
}

// buildModules returns the modules of the running binary, path to version:
// the main module and its dependencies.
func buildModules() map[string]any {
	modules := map[string]any{}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return modules
	}
	modules[info.Main.Path] = info.Main.Version
	for _, dep := range info.Deps {
		modules[dep.Path] = dep.Version
	}
	return modules
}

// debugTagParser parses {% debug %}, which takes no arguments.
func debugTagParser(_ *pongo2.Parser, start *pongo2.Token, args *pongo2.Parser) (pongo2.INodeTag, *pongo2.Error) {
	if args.Remaining() > 0 {
		return nil, args.Error("'debug' tag takes no arguments", start)
	}
	return debugTagNode{}, nil
}
