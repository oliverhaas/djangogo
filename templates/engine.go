// Package templates provides a pongo2-based template engine with a small set of
// Django-style tags ({% static %}, {% url %}, {% csrf_token %}). Engines load
// templates from one or more directories or from an fs.FS and render them with a
// map[string]any context.
package templates

import (
	"fmt"
	"io"
	"io/fs"

	"github.com/flosch/pongo2/v6"
)

// Engine renders pongo2 templates loaded from one or more directories or an fs.FS.
type Engine struct {
	set      *pongo2.TemplateSet
	resolver Resolver
}

// SetResolver sets the URL resolver injected into every render context, so this
// engine's {% url %} tags reverse against the given route table rather than the
// process-global resolver. Passing nil clears it (falling back to the global).
func (e *Engine) SetResolver(r Resolver) { e.resolver = r }

// context builds a pongo2 context from ctx, injecting this engine's resolver
// (when set) under resolverContextKey. The caller's map is copied, never mutated.
func (e *Engine) context(ctx map[string]any) pongo2.Context {
	if e.resolver == nil {
		return pongo2.Context(ctx)
	}
	pc := make(pongo2.Context, len(ctx)+1)
	for k, v := range ctx {
		pc[k] = v
	}
	pc[resolverContextKey] = e.resolver
	return pc
}

// NewEngine returns an Engine loading templates from the given directories (searched
// in order). Pass zero dirs to render only from strings.
func NewEngine(dirs ...string) (*Engine, error) {
	loaders := make([]pongo2.TemplateLoader, 0, len(dirs))
	for _, dir := range dirs {
		loader, err := pongo2.NewLocalFileSystemLoader(dir)
		if err != nil {
			return nil, fmt.Errorf("templates: load dir %q: %w", dir, err)
		}
		loaders = append(loaders, loader)
	}
	return newEngine(loaders...), nil
}

// NewEngineFS returns an Engine loading templates from an fs.FS (e.g. embed.FS).
func NewEngineFS(fsys fs.FS) (*Engine, error) {
	if fsys == nil {
		return nil, fmt.Errorf("templates: fsys must not be nil")
	}
	return newEngine(pongo2.NewFSLoader(fsys)), nil
}

// newEngine builds an Engine from the given loaders. A string-only loader is added
// when none are supplied so RenderString always works (FromString never consults a
// loader, but NewSet requires at least one).
func newEngine(loaders ...pongo2.TemplateLoader) *Engine {
	if len(loaders) == 0 {
		loaders = append(loaders, stringOnlyLoader{})
	}
	return &Engine{set: pongo2.NewSet("djangogo", loaders...)}
}

// Render renders the named template with ctx and returns the output.
func (e *Engine) Render(name string, ctx map[string]any) (string, error) {
	tpl, err := e.set.FromCache(name)
	if err != nil {
		return "", fmt.Errorf("templates: load %q: %w", name, err)
	}
	out, err := tpl.Execute(e.context(ctx))
	if err != nil {
		return "", fmt.Errorf("templates: render %q: %w", name, err)
	}
	return out, nil
}

// RenderTo renders the named template with ctx to w.
func (e *Engine) RenderTo(w io.Writer, name string, ctx map[string]any) error {
	tpl, err := e.set.FromCache(name)
	if err != nil {
		return fmt.Errorf("templates: load %q: %w", name, err)
	}
	if err := tpl.ExecuteWriter(e.context(ctx), w); err != nil {
		return fmt.Errorf("templates: render %q: %w", name, err)
	}
	return nil
}

// RenderString renders a template given as a source string (useful for tests).
func (e *Engine) RenderString(src string, ctx map[string]any) (string, error) {
	tpl, err := e.set.FromString(src)
	if err != nil {
		return "", fmt.Errorf("templates: parse string: %w", err)
	}
	out, err := tpl.Execute(e.context(ctx))
	if err != nil {
		return "", fmt.Errorf("templates: render string: %w", err)
	}
	return out, nil
}

// stringOnlyLoader is a no-op TemplateLoader used by string-only engines so that
// pongo2.NewSet (which requires at least one loader) is satisfied. It never
// resolves a file; FromString and FromBytes bypass loaders entirely.
type stringOnlyLoader struct{}

// Abs returns name unchanged; the loader never resolves files.
func (stringOnlyLoader) Abs(_, name string) string { return name }

// Get always fails because this loader serves no files.
func (stringOnlyLoader) Get(path string) (io.Reader, error) {
	return nil, fmt.Errorf("templates: no template directory configured (cannot load %q)", path)
}
