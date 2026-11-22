// Package templates provides a pongo2-based template engine that behaves like
// Django's: templates are found by name in an ordered list of sources (project
// directories first, then the templates each app ships), {% extends %} and
// {% include %} go through the same lookup, and the Django tags and filters
// pongo2 lacks or renders differently ({% url %}, {% static %}, {% csrf_token %},
// {% for %} with a lowercase forloop, the date formats, ...) are provided here.
//
// A context is a map[string]any. Model instances in it, however deep, are
// shown to the template the way Django shows them: attributes by their
// snake_case name, pk, <relation>_id, get_<field>_display and the instance
// label for a bare {{ object }} (see ModelContext).
package templates

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"strings"

	"github.com/flosch/pongo2/v6"
)

// Engine renders templates looked up by name in its sources. Configure it at
// startup (sources, resolver, context processors, models); once it serves
// requests it is safe for concurrent use and compiles each template once.
type Engine struct {
	set        *pongo2.TemplateSet
	loader     *sourceLoader
	resolver   Resolver
	processors []ContextProcessor
	models     ModelLookup
}

// stateContextKey is the context key under which a render carries its
// renderState. The name is a valid identifier so pongo2 accepts it as a key.
const stateContextKey = "__djangogo_render__"

// renderState is what the tags of this package need to know about the render
// they run in.
type renderState struct {
	// engine loads the templates {% include %} names.
	engine *Engine
	// request is the request of a RenderRequest call, nil otherwise.
	request *http.Request
	// recording is set when a Recorder listens, so that includes keep their
	// context.
	recording bool
	// included lists the templates pulled in by {% include %}, in render order.
	included []Rendered
	// depth counts the includes being rendered, one inside the other.
	depth int
	// tagState holds what tags such as {% cycle %} remember during the
	// render of one template, keyed by the tag, see tagState.
	tagState map[any]any
}

// stateFrom returns the renderState of the render ctx belongs to, or nil for a
// template executed outside an Engine.
func stateFrom(ctx *pongo2.ExecutionContext) *renderState {
	st, _ := ctx.Public[stateContextKey].(*renderState)
	return st
}

// SetResolver sets the URL resolver injected into every render context, so this
// engine's {% url %} tags reverse against the given route table rather than the
// process-global resolver. Passing nil clears it (falling back to the global).
func (e *Engine) SetResolver(r Resolver) { e.resolver = r }

// SetDebug switches template caching off (true) or on (false, the default).
// With debug on every render reads and compiles the template again, so an
// edit shows without a restart, as with Django's loaders under DEBUG, and
// {% debug %} prints the context.
func (e *Engine) SetDebug(debug bool) { e.set.Debug = debug }

// AddSource appends sources to the search order, after the ones the engine
// already has. An application adds the templates of each installed app this
// way, so that the project's own directories, given first, override them.
func (e *Engine) AddSource(sources ...Source) { e.loader.add(sources...) }

// Sources returns the label of every source in search order.
func (e *Engine) Sources() []string { return e.loader.labels() }

// context builds the pongo2 context of one render: the caller's values with
// the model instances among them converted for the template, plus the render
// state and this engine's resolver. The caller's map is copied, never mutated.
func (e *Engine) context(ctx map[string]any, st *renderState) pongo2.Context {
	pc := make(pongo2.Context, len(ctx)+2)
	conv := converter{lookup: e.modelLookup()}
	for k, v := range ctx {
		pc[k] = conv.value(v, 0)
	}
	if e.resolver != nil {
		pc[resolverContextKey] = e.resolver
	}
	if st == nil {
		st = &renderState{}
	}
	st.engine = e
	pc[stateContextKey] = st
	return pc
}

// New returns an Engine that looks templates up in the given sources, in
// order. With no source it renders strings only until AddSource is called.
//
//	engine := templates.New(
//		templates.DirSource("templates"),       // the project's own, searched first
//		templates.FSSource("blog", blogFS),     // then what each app ships
//	)
func New(sources ...Source) *Engine {
	loader := &sourceLoader{}
	loader.add(sources...)
	set := pongo2.NewSet("djangogo", loader)
	// Django's literals. pongo2 itself knows true and false only, and reads
	// the capitalized words as the names of variables.
	set.Globals["True"], set.Globals["False"], set.Globals["None"] = true, false, nil
	return &Engine{set: set, loader: loader}
}

// NewEngine returns an Engine loading templates from the given directories (searched
// in order). Pass zero dirs to render only from strings.
func NewEngine(dirs ...string) (*Engine, error) {
	sources := make([]Source, 0, len(dirs))
	for _, dir := range dirs {
		info, err := os.Stat(dir)
		if err != nil {
			return nil, fmt.Errorf("templates: load dir %q: %w", dir, err)
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("templates: load dir %q: not a directory", dir)
		}
		sources = append(sources, DirSource(dir))
	}
	return New(sources...), nil
}

// NewEngineFS returns an Engine loading templates from an fs.FS (e.g. embed.FS).
func NewEngineFS(fsys fs.FS) (*Engine, error) {
	if fsys == nil {
		return nil, errors.New("templates: fsys must not be nil")
	}
	return New(FSSource("fs", fsys)), nil
}

// newEngine builds an Engine without sources, for rendering strings.
func newEngine() *Engine { return New() }

// pongoError exposes the cause of a pongo2 error to errors.Is and errors.As,
// which pongo2's own error type hides. The message stays pongo2's.
type pongoError struct {
	err *pongo2.Error
}

// Error returns pongo2's message, with the template position.
func (e *pongoError) Error() string { return e.err.Error() }

// Unwrap returns the pongo2 error and its cause.
func (e *pongoError) Unwrap() []error {
	if e.err.OrigError == nil {
		return []error{e.err}
	}
	return []error{e.err, e.err.OrigError}
}

// wrapPongo makes the cause of a pongo2 error reachable; other errors pass
// through.
func wrapPongo(err error) error {
	var perr *pongo2.Error
	if errors.As(err, &perr) {
		return &pongoError{err: perr}
	}
	return err
}

// loadError turns pongo2's "unable to resolve template" into the
// TemplateDoesNotExist of the template that is missing, which is name itself
// or one it extends.
func (e *Engine) loadError(err error) error {
	var perr *pongo2.Error
	if errors.As(err, &perr) && perr.Sender == "fromfile" && perr.Filename != "" {
		if miss := e.loader.missing(perr.Filename); miss != nil {
			return miss
		}
	}
	// A "name#partial" whose template does not define the partial.
	var miss *TemplateDoesNotExist
	if perr != nil && errors.As(perr.OrigError, &miss) {
		return miss
	}
	return wrapPongo(err)
}

// errPanic is the cause of the error a render returns when pongo2 panicked.
var errPanic = errors.New("pongo2 panicked")

// guard runs fn and turns a panic into an error. pongo2 panics on some
// malformed templates ({{ a%b }} with both sides missing divides by zero),
// and a template must not take down the request that renders it.
func guard(what string, fn func() error) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("templates: %s: %w: %v", what, errPanic, r)
		}
	}()
	return fn()
}

// load returns the compiled template of name, compiling it on first use.
func (e *Engine) load(name string) (*pongo2.Template, error) {
	if name == "" {
		// pongo2 reports an empty name without saying which template it was.
		return nil, fmt.Errorf("templates: load %q: %w", name, &TemplateDoesNotExist{Name: name})
	}
	var tpl *pongo2.Template
	err := guard(fmt.Sprintf("load %q", name), func() (err error) {
		tpl, err = e.set.FromCache(name)
		return err
	})
	if errors.Is(err, errPanic) {
		return nil, err
	}
	if err != nil {
		return nil, fmt.Errorf("templates: load %q: %w", name, e.loadError(err))
	}
	return tpl, nil
}

// SelectTemplate returns the first of names that the engine has, Django's
// select_template. Views use it to prefer a specific template over a generic
// one. The error is a *TemplateDoesNotExist listing every candidate when no
// name is found; a template that is found but does not compile is an error
// right away.
func (e *Engine) SelectTemplate(names ...string) (string, error) {
	name, _, err := e.loadFirst(names)
	return name, err
}

// loadFirst compiles the first of names that the engine has.
func (e *Engine) loadFirst(names []string) (string, *pongo2.Template, error) {
	if len(names) == 0 {
		return "", nil, &TemplateDoesNotExist{Name: "No template names provided"}
	}
	var tried []TriedSource
	for _, name := range names {
		tpl, err := e.load(name)
		if err == nil {
			return name, tpl, nil
		}
		var miss *TemplateDoesNotExist
		if len(names) == 1 || !errors.As(err, &miss) || miss.Name != name {
			return "", nil, err
		}
		tried = append(tried, miss.Tried...)
	}
	return "", nil, &TemplateDoesNotExist{Name: strings.Join(names, ", "), Tried: tried}
}

// Render renders the named template with ctx and returns the output.
func (e *Engine) Render(name string, ctx map[string]any) (string, error) {
	return e.render(name, ctx, nil)
}

// render renders the named template with the given render state.
func (e *Engine) render(name string, ctx map[string]any, st *renderState) (string, error) {
	tpl, err := e.load(name)
	if err != nil {
		return "", err
	}
	var out string
	err = guard(fmt.Sprintf("render %q", name), func() (err error) {
		out, err = tpl.Execute(e.context(ctx, st))
		return err
	})
	if err != nil && !errors.Is(err, errPanic) {
		err = fmt.Errorf("templates: render %q: %w", name, wrapPongo(err))
	}
	return out, err
}

// RenderTo renders the named template with ctx to w.
func (e *Engine) RenderTo(w io.Writer, name string, ctx map[string]any) error {
	tpl, err := e.load(name)
	if err != nil {
		return err
	}
	err = guard(fmt.Sprintf("render %q", name), func() error {
		return tpl.ExecuteWriter(e.context(ctx, nil), w)
	})
	if err != nil && !errors.Is(err, errPanic) {
		err = fmt.Errorf("templates: render %q: %w", name, wrapPongo(err))
	}
	return err
}

// RenderString renders a template given as a source string. The source is
// compiled on every call, so it suits tests and one-off snippets, not the
// request path.
func (e *Engine) RenderString(src string, ctx map[string]any) (string, error) {
	return e.renderString(src, ctx, nil)
}

// renderString renders a source string with the given render state.
func (e *Engine) renderString(src string, ctx map[string]any, st *renderState) (string, error) {
	var tpl *pongo2.Template
	err := guard("parse string", func() (err error) {
		tpl, err = e.set.FromBytes(djangoSyntax([]byte(src)))
		return err
	})
	if err != nil {
		if errors.Is(err, errPanic) {
			return "", err
		}
		return "", fmt.Errorf("templates: parse string: %w", e.loadError(err))
	}
	var out string
	err = guard("render string", func() (err error) {
		out, err = tpl.Execute(e.context(ctx, st))
		return err
	})
	if err != nil && !errors.Is(err, errPanic) {
		err = fmt.Errorf("templates: render string: %w", wrapPongo(err))
	}
	return out, err
}
