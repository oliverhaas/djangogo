package views

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/oliverhaas/djangogo/templates"
)

// Default template names of the error views, as in django/views/defaults.py.
const (
	error404TemplateName = "404.html"
	error403TemplateName = "403.html"
	error400TemplateName = "400.html"
	error500TemplateName = "500.html"
)

// defaultErrorPage returns Django's ERROR_PAGE_TEMPLATE filled with title and
// details. Both arguments are fixed strings, never request data, so the page
// needs no escaping.
func defaultErrorPage(title, details string) string {
	return fmt.Sprintf(`
<!doctype html>
<html lang="en">
<head>
  <title>%[1]s</title>
</head>
<body>
  <h1>%[1]s</h1><p>%[2]s</p>
</body>
</html>
`, title, details)
}

// writeHTML writes body as an HTML response with the given status.
func writeHTML(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}

// isTemplateMissing reports whether err, returned by the engine for the
// template name, says that this very template could not be found. A template
// that exists but extends or includes a missing one does not count: that is a
// defect of the template and must surface as an error.
//
// The engine has no existence check and no sentinel for a missing template,
// so this is the one place that inspects its error text.
func isTemplateMissing(err error, name string) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.HasPrefix(msg, fmt.Sprintf("templates: load %q:", name)) &&
		strings.Contains(msg, "in "+name+"]") &&
		strings.Contains(msg, "unable to resolve template")
}

// errorPage renders an error template for the request, or falls back to
// Django's default body when the default template of that status is not
// provided by the project. A missing custom template (templateName set by the
// caller) is an error, as it is in Django. Nothing is written when an error is
// returned. With withRequest unset the template is rendered without context
// processors, which keeps the 500 page independent of request state.
func errorPage(
	w http.ResponseWriter, r *http.Request, e *templates.Engine,
	status int, templateName, defaultName string, ctx map[string]any,
	withRequest bool, fallback string,
) error {
	name := templateName
	if name == "" {
		name = defaultName
	}
	if e == nil {
		if templateName != "" {
			return fmt.Errorf("%w: %s (no template engine)", ErrTemplateDoesNotExist, name)
		}
		writeHTML(w, status, fallback)
		return nil
	}
	var (
		body string
		err  error
	)
	if withRequest {
		body, err = e.RenderRequest(r, name, ctx)
	} else {
		body, err = e.Render(name, ctx)
	}
	switch {
	case err == nil:
		writeHTML(w, status, body)
		return nil
	case isTemplateMissing(err, name) && templateName == "":
		writeHTML(w, status, fallback)
		return nil
	case isTemplateMissing(err, name):
		return fmt.Errorf("%w: %s", ErrTemplateDoesNotExist, name)
	default:
		return err
	}
}

// NotFoundPage writes the 404 response, Django's page_not_found view. It
// renders templateName ("" means 404.html) with the context
//
//	request_path  the request path, percent-quoted against content injection
//	exception     the message of the NotFound error, or "Http404" without one
//
// and falls back to Django's default "Not Found" body when the project has no
// 404.html. A missing custom template is an error (ErrTemplateDoesNotExist).
// Nothing is written when an error is returned.
func NotFoundPage(w http.ResponseWriter, r *http.Request, e *templates.Engine, err error, templateName string) error {
	ctx := map[string]any{
		"request_path": quotePath(r.URL.Path),
		"exception":    exceptionMessage(err, "Http404"),
	}
	return errorPage(w, r, e, http.StatusNotFound, templateName, error404TemplateName, ctx, true,
		defaultErrorPage("Not Found", "The requested resource was not found on this server."))
}

// PermissionDeniedPage writes the 403 response, Django's permission_denied
// view. It renders templateName ("" means 403.html) with the context
//
//	exception  the message of the PermissionDenied error, or "" without one
//
// and falls back to Django's default "403 Forbidden" body when the project has
// no 403.html. A missing custom template is an error.
func PermissionDeniedPage(w http.ResponseWriter, r *http.Request, e *templates.Engine, err error, templateName string) error {
	ctx := map[string]any{"exception": exceptionMessage(err, "")}
	return errorPage(w, r, e, http.StatusForbidden, templateName, error403TemplateName, ctx, true,
		defaultErrorPage("403 Forbidden", ""))
}

// BadRequestPage writes the 400 response, Django's bad_request view. It
// renders templateName ("" means 400.html) and falls back to Django's default
// "Bad Request (400)" body. The error is deliberately not passed to the
// template, so no detail of a suspicious request is disclosed. A missing
// custom template is an error.
func BadRequestPage(w http.ResponseWriter, r *http.Request, e *templates.Engine, _ error, templateName string) error {
	return errorPage(w, r, e, http.StatusBadRequest, templateName, error400TemplateName, nil, true,
		defaultErrorPage("Bad Request (400)", ""))
}

// ServerErrorPage writes the 500 response, Django's server_error view. It
// renders templateName ("" means 500.html) with an empty context and without
// context processors, as Django does, so the page still renders when request
// state is what broke. It falls back to Django's default "Server Error (500)"
// body. A missing custom template is an error.
func ServerErrorPage(w http.ResponseWriter, r *http.Request, e *templates.Engine, templateName string) error {
	return errorPage(w, r, e, http.StatusInternalServerError, templateName, error500TemplateName, nil, false,
		defaultErrorPage("Server Error (500)", ""))
}

// quotePath percent-encodes a URL path like Python's urllib.parse.quote with
// its default safe set: ASCII letters, digits and "_.-~/" stay, every other
// byte becomes %XX.
func quotePath(p string) string {
	const upperHex = "0123456789ABCDEF"
	var b strings.Builder
	for i := 0; i < len(p); i++ {
		c := p[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9',
			c == '_', c == '.', c == '-', c == '~', c == '/':
			b.WriteByte(c)
		default:
			b.WriteByte('%')
			b.WriteByte(upperHex[c>>4])
			b.WriteByte(upperHex[c&0x0f])
		}
	}
	return b.String()
}
