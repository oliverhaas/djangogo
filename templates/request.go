package templates

import (
	"context"
	"fmt"
	"maps"
	"net/http"
	"sync"
)

// ContextProcessor returns template variables derived from the request, the
// equivalent of a Django context processor. Its result is merged into the
// context of every request-bound render (see Engine.RenderRequest).
type ContextProcessor func(r *http.Request) map[string]any

// AddContextProcessor appends processors to the engine. They run in the order
// added on every RenderRequest call. Configure processors at startup, before
// the engine serves requests; the list is not safe for concurrent mutation.
func (e *Engine) AddContextProcessor(processors ...ContextProcessor) {
	e.processors = append(e.processors, processors...)
}

// RenderRequest renders the named template for the request r. The context is
// built from the engine's context processors (in order, later ones overriding
// earlier ones) and then data, whose keys win over processor keys, exactly as
// the context passed to Django's render() overrides processor output. When the
// request context carries a Recorder (see WithRecorder), the render is
// recorded with its template name and final context.
func (e *Engine) RenderRequest(r *http.Request, name string, data map[string]any) (string, error) {
	if r == nil {
		return "", fmt.Errorf("templates: RenderRequest %q: request must not be nil", name)
	}
	merged := make(map[string]any, len(data))
	for _, p := range e.processors {
		maps.Copy(merged, p(r))
	}
	maps.Copy(merged, data)
	out, err := e.Render(name, merged)
	if err != nil {
		return "", err
	}
	if rec, ok := RecorderFromContext(r.Context()); ok {
		rec.record(Rendered{Name: name, Context: merged})
	}
	return out, nil
}

// Rendered describes one template render observed by a Recorder: the template
// name as requested and the final context it was rendered with.
type Rendered struct {
	Name    string
	Context map[string]any
}

// Recorder collects the request-bound template renders of one request. It is
// the hook behind the test client's Response.Templates and Response.Context,
// the counterpart of Django's template_rendered signal. The zero value is
// ready to use and safe for concurrent use.
type Recorder struct {
	mu       sync.Mutex
	rendered []Rendered
}

// record appends one observed render.
func (rec *Recorder) record(r Rendered) {
	rec.mu.Lock()
	defer rec.mu.Unlock()
	rec.rendered = append(rec.rendered, r)
}

// Rendered returns the renders observed so far, in render order.
func (rec *Recorder) Rendered() []Rendered {
	rec.mu.Lock()
	defer rec.mu.Unlock()
	out := make([]Rendered, len(rec.rendered))
	copy(out, rec.rendered)
	return out
}

// recorderKey is the context key under which a Recorder is stored.
type recorderKey struct{}

// WithRecorder returns a copy of ctx carrying rec, so that every RenderRequest
// call made with a request derived from ctx is recorded.
func WithRecorder(ctx context.Context, rec *Recorder) context.Context {
	return context.WithValue(ctx, recorderKey{}, rec)
}

// RecorderFromContext returns the Recorder bound to ctx, if any.
func RecorderFromContext(ctx context.Context) (*Recorder, bool) {
	rec, ok := ctx.Value(recorderKey{}).(*Recorder)
	return rec, ok && rec != nil
}
