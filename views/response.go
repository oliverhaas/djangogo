package views

import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strings"

	"github.com/oliverhaas/djangogo/templates"
)

// TemplateValuer is implemented by a value that must be shown to a template
// in another shape than its Go type. Render and every generic view replace
// each top-level context value that implements it by the result of
// TemplateValue before rendering.
//
// The template engine resolves {{ x.name }} only against map keys and exported
// Go names, and it cannot both iterate a value and read attributes from it.
// A Page therefore returns a map with Django's lowercase names, and a form
// returns its template view the same way. A TemplateValue must not hide
// user-controlled text behind a String method: the engine escapes only
// string values and writes any other value with a String method raw.
type TemplateValuer interface {
	TemplateValue() any
}

// templateContext returns a copy of ctx in which every top-level value that
// implements TemplateValuer is replaced by its template value. The caller's
// map is not modified. A nil pointer stays nil, so a view can put an unset
// *Page or form into the context and the template sees an empty value.
func templateContext(ctx map[string]any) map[string]any {
	out := make(map[string]any, len(ctx))
	for k, v := range ctx {
		tv, ok := v.(TemplateValuer)
		switch {
		case !ok:
			out[k] = v
		case isNilPointer(v):
			out[k] = nil
		default:
			out[k] = tv.TemplateValue()
		}
	}
	return out
}

// isNilPointer reports whether v holds a nil pointer.
func isNilPointer(v any) bool {
	rv := reflect.ValueOf(v)
	return rv.Kind() == reflect.Pointer && rv.IsNil()
}

// ResponseOption adjusts the response written by Render, RenderFirst and
// JSONResponse, the counterpart of the status and content_type arguments of
// Django's shortcuts.
type ResponseOption func(*responseConfig)

// responseConfig holds the adjustable parts of a response.
type responseConfig struct {
	status      int
	contentType string
}

// WithStatus sets the HTTP status of the response (default 200).
func WithStatus(code int) ResponseOption {
	return func(c *responseConfig) { c.status = code }
}

// WithContentType sets the Content-Type header of the response.
func WithContentType(contentType string) ResponseOption {
	return func(c *responseConfig) { c.contentType = contentType }
}

// newResponseConfig applies opts on top of the given defaults.
func newResponseConfig(contentType string, opts []ResponseOption) responseConfig {
	cfg := responseConfig{status: http.StatusOK, contentType: contentType}
	for _, opt := range opts {
		opt(&cfg)
	}
	return cfg
}

// renderCandidates renders the first template of names that exists, Django's
// select_template, for the request r. It returns an ErrTemplateDoesNotExist
// naming all candidates when none exists. An error inside an existing template
// is returned as it is and ends the search.
func renderCandidates(r *http.Request, e *templates.Engine, names []string, ctx map[string]any) (string, error) {
	if e == nil {
		return "", improperlyConfigured("no template engine to render %s", strings.Join(names, ", "))
	}
	data := templateContext(ctx)
	for _, name := range names {
		out, err := e.RenderRequest(r, name, data)
		if err == nil {
			return out, nil
		}
		if !isTemplateMissing(err, name) {
			return "", err
		}
	}
	return "", fmt.Errorf("%w: %s", ErrTemplateDoesNotExist, strings.Join(names, ", "))
}

// renderResponse renders the first existing template of names and writes it.
// It writes nothing when it returns an error, so the caller can still answer
// with an error page.
func renderResponse(
	w http.ResponseWriter, r *http.Request, e *templates.Engine,
	names []string, ctx map[string]any, opts []ResponseOption,
) error {
	out, err := renderCandidates(r, e, names, ctx)
	if err != nil {
		return err
	}
	cfg := newResponseConfig("text/html; charset=utf-8", opts)
	w.Header().Set("Content-Type", cfg.contentType)
	w.WriteHeader(cfg.status)
	_, err = w.Write([]byte(out))
	if err != nil {
		// The status line is out already, so no error page can follow.
		return &writtenError{err: err}
	}
	return nil
}

// Render renders the named template with ctx for the request r and writes it
// to w as text/html with status 200, the counterpart of Django's
// render(request, template_name, context, content_type, status). Pass
// WithStatus and WithContentType for the last two arguments. The engine's
// context processors are applied, context values implementing TemplateValuer
// are converted, and under the test client the template name and context are
// recorded on the response.
//
// Output is buffered, so a failed render never emits a partial page. On
// failure Render answers with the 500 page (see ServeError) and returns the
// error; a view of type HandlerFunc may return that error as it is, the
// response is not written twice.
func Render(
	w http.ResponseWriter, r *http.Request, e *templates.Engine,
	name string, ctx map[string]any, opts ...ResponseOption,
) error {
	return RenderFirst(w, r, e, []string{name}, ctx, opts...)
}

// RenderFirst is Render for a list of candidate template names: the first one
// that exists is rendered, as Django's render does for a list of names.
func RenderFirst(
	w http.ResponseWriter, r *http.Request, e *templates.Engine,
	names []string, ctx map[string]any, opts ...ResponseOption,
) error {
	err := renderResponse(w, r, e, names, ctx, opts)
	if err == nil {
		return nil
	}
	ServeError(w, r, e, err)
	return &writtenError{err: err}
}

// RenderToString renders the named template with ctx for the request r and
// returns the output, the counterpart of Django's render_to_string. Context
// processors and TemplateValuer conversion apply as in Render.
func RenderToString(r *http.Request, e *templates.Engine, name string, ctx map[string]any) (string, error) {
	return renderCandidates(r, e, []string{name}, ctx)
}

// JSON writes v as application/json with the given status. A marshal failure writes
// a 500 and returns the error.
func JSON(w http.ResponseWriter, status int, v any) error {
	body, err := json.Marshal(v)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return err
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, werr := w.Write(body)
	return werr
}

// JSONResponse writes v as JSON with status 200 and the content type
// application/json, the counterpart of Django's JsonResponse. Pass WithStatus
// or WithContentType to change either. Unlike JsonResponse it has no "safe"
// switch: any value encoding/json can marshal is accepted. Nothing is written
// when marshaling fails, so a HandlerFunc can return the error.
func JSONResponse(w http.ResponseWriter, v any, opts ...ResponseOption) error {
	body, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("views: marshal JSON response: %w", err)
	}
	cfg := newResponseConfig("application/json", opts)
	w.Header().Set("Content-Type", cfg.contentType)
	w.WriteHeader(cfg.status)
	if _, err = w.Write(body); err != nil {
		return &writtenError{err: err}
	}
	return nil
}
