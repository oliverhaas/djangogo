// Package views provides request/response helpers and ORM-backed generic views
// (DetailView, ListView) that fetch model instances and render them through a
// templates.Engine.
package views

import (
	"encoding/json"
	"net/http"

	"github.com/oliverhaas/djangogo/templates"
)

// Render renders the named template with ctx to w as text/html (200), or writes a
// 500 and returns the error on failure. It buffers via the engine so a render error
// does not emit a partially written 200 body.
func Render(w http.ResponseWriter, e *templates.Engine, name string, ctx map[string]any) error {
	out, err := e.Render(name, ctx)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return err
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, werr := w.Write([]byte(out))
	return werr
}

// Redirect issues an HTTP redirect to url with the given status (e.g. 302).
func Redirect(w http.ResponseWriter, r *http.Request, url string, status int) {
	http.Redirect(w, r, url, status)
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
