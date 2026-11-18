package views

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/templates"
)

// kindError is a sentinel error with a fixed message and an optional parent
// kind, so that errors.Is(ErrEmptyPage, ErrInvalidPage) holds the way
// issubclass(EmptyPage, InvalidPage) does in Django.
type kindError struct {
	msg    string
	parent error
}

// Error returns the fixed message of the sentinel.
func (e *kindError) Error() string { return e.msg }

// Unwrap returns the parent kind, or nil for a root kind.
func (e *kindError) Unwrap() error { return e.parent }

// Sentinel errors a view returns to select the error response. Match them with
// errors.Is; ServeError documents the status each one maps to.
var (
	// ErrNotFound is Django's Http404: the requested resource does not exist.
	ErrNotFound error = &kindError{msg: "views: not found"}
	// ErrPermissionDenied is Django's PermissionDenied: the user may not do this.
	ErrPermissionDenied error = &kindError{msg: "views: permission denied"}
	// ErrBadRequest is Django's BadRequest: the request is malformed.
	ErrBadRequest error = &kindError{msg: "views: bad request"}
	// ErrSuspiciousOperation is Django's SuspiciousOperation: the request looks
	// like an attack. It is a kind of ErrBadRequest.
	ErrSuspiciousOperation error = &kindError{msg: "views: suspicious operation", parent: ErrBadRequest}
	// ErrDisallowedRedirect is Django's DisallowedRedirect: a redirect target
	// was rejected. It is a kind of ErrSuspiciousOperation.
	ErrDisallowedRedirect error = &kindError{msg: "views: disallowed redirect", parent: ErrSuspiciousOperation}
	// ErrImproperlyConfigured is Django's ImproperlyConfigured: a view lacks a
	// setting it needs. It is a programming error and maps to a 500.
	ErrImproperlyConfigured error = &kindError{msg: "views: improperly configured"}
	// ErrTemplateDoesNotExist is Django's TemplateDoesNotExist: none of the
	// candidate templates could be loaded.
	ErrTemplateDoesNotExist error = &kindError{msg: "views: template does not exist"}
)

// messageError is an error of a given kind that carries a message meant for
// the response (Django's exception argument, as in Http404("No Poll matches")).
type messageError struct {
	kind error
	msg  string
}

// Error returns the message alone, without a package prefix, because the
// message is what reaches the 404 and 403 templates.
func (e *messageError) Error() string { return e.msg }

// Unwrap returns the kind, so errors.Is matches the sentinel.
func (e *messageError) Unwrap() error { return e.kind }

// NotFound returns an ErrNotFound with a message, Django's Http404(msg). The
// message is passed to the 404 template as "exception".
func NotFound(msg string) error { return &messageError{kind: ErrNotFound, msg: msg} }

// PermissionDenied returns an ErrPermissionDenied with a message, Django's
// PermissionDenied(msg). The message is passed to the 403 template as
// "exception".
func PermissionDenied(msg string) error { return &messageError{kind: ErrPermissionDenied, msg: msg} }

// BadRequest returns an ErrBadRequest with a message, Django's BadRequest(msg).
// The message is not shown to the client.
func BadRequest(msg string) error { return &messageError{kind: ErrBadRequest, msg: msg} }

// SuspiciousOperation returns an ErrSuspiciousOperation with a message,
// Django's SuspiciousOperation(msg). The message is not shown to the client.
func SuspiciousOperation(msg string) error {
	return &messageError{kind: ErrSuspiciousOperation, msg: msg}
}

// improperlyConfigured returns an ErrImproperlyConfigured with a formatted
// message. Generic views use it for a missing template name, queryset or
// success URL.
func improperlyConfigured(format string, args ...any) error {
	return &messageError{kind: ErrImproperlyConfigured, msg: "views: " + fmt.Sprintf(format, args...)}
}

// exceptionMessage returns the text the error templates receive as
// "exception": the message of a NotFound or PermissionDenied error, and
// fallback for an error without one (Django falls back to the exception class
// name, which keeps internal error text out of the page).
func exceptionMessage(err error, fallback string) string {
	var me *messageError
	if errors.As(err, &me) {
		return me.msg
	}
	return fallback
}

// writtenError marks an error whose response has already been written, so the
// central error mapping does not write a second response for it.
type writtenError struct{ err error }

// Error returns the message of the underlying error.
func (e *writtenError) Error() string { return e.err.Error() }

// Unwrap returns the underlying error.
func (e *writtenError) Unwrap() error { return e.err }

// HandlerFunc is a view that reports failure by returning an error instead of
// writing an error response itself, the Go shape of a Django view that raises
// Http404 or PermissionDenied. Adapt it to an http.Handler with Handler.
type HandlerFunc func(w http.ResponseWriter, r *http.Request) error

// Handler adapts fn to an http.Handler. An error returned by fn is turned into
// a response by ServeError, the single place where errors map to statuses. The
// engine renders the error templates and may be nil.
func Handler(e *templates.Engine, fn HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := fn(w, r); err != nil {
			ServeError(w, r, e, err)
		}
	})
}

// NotFoundHandler returns a handler that answers every request with the 404
// page. Mount it as the fallback route so unknown URLs get the same page as a
// view returning ErrNotFound. The 404 template sees "Resolver404" as its
// exception, as it does in Django for an unmatched URL.
func NotFoundHandler(e *templates.Engine) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ServeError(w, r, e, NotFound("Resolver404"))
	})
}

// StatusCode returns the HTTP status an error maps to: 404 for ErrNotFound and
// orm.ErrDoesNotExist, 403 for ErrPermissionDenied, 400 for ErrBadRequest and
// its kinds (ErrSuspiciousOperation, ErrDisallowedRedirect), and 500 for
// everything else.
func StatusCode(err error) int {
	switch {
	case errors.Is(err, ErrNotFound), errors.Is(err, orm.ErrDoesNotExist):
		return http.StatusNotFound
	case errors.Is(err, ErrPermissionDenied):
		return http.StatusForbidden
	case errors.Is(err, ErrBadRequest):
		return http.StatusBadRequest
	default:
		return http.StatusInternalServerError
	}
}

// ServeError writes the error response for err. It is the one mapping from
// errors to responses, used by Handler and by every generic view, and the
// counterpart of Django's exception handling in the request handler: the
// status comes from StatusCode and the body from the matching default error
// view (NotFoundPage, PermissionDeniedPage, BadRequestPage, ServerErrorPage),
// which renders 404.html, 403.html, 400.html or 500.html when the engine has
// it and Django's default body otherwise. A 500 is logged with its cause,
// which is never shown to the client. The engine may be nil.
func ServeError(w http.ResponseWriter, r *http.Request, e *templates.Engine, err error) {
	var written *writtenError
	if errors.As(err, &written) {
		// Render already answered with the 500 page and logged the cause.
		return
	}
	var pageErr error
	switch StatusCode(err) {
	case http.StatusNotFound:
		pageErr = NotFoundPage(w, r, e, err, "")
	case http.StatusForbidden:
		pageErr = PermissionDeniedPage(w, r, e, err, "")
	case http.StatusBadRequest:
		pageErr = BadRequestPage(w, r, e, err, "")
	default:
		logServerError(r, err)
		pageErr = ServerErrorPage(w, r, e, "")
	}
	if pageErr != nil {
		// The error template itself is broken. Answer with the plain 500 body
		// instead of leaving the response empty.
		logServerError(r, pageErr)
		writeHTML(w, http.StatusInternalServerError, defaultErrorPage("Server Error (500)", ""))
	}
}

// logServerError logs the cause of a 500 response. The client only sees the
// generic page, so the log is the one place the cause is recorded.
func logServerError(r *http.Request, err error) {
	slog.ErrorContext(r.Context(), "Internal Server Error: "+escapeLogValue(r.URL.Path),
		"status", http.StatusInternalServerError, "method", r.Method, "error", err)
}

// logResponse logs a client error response the way Django's log_response does
// for 405 and 410 answers: a warning with the status code attached.
func logResponse(r *http.Request, status int, msg string) {
	slog.WarnContext(r.Context(), msg, "status", status, "method", r.Method)
}

// escapeLogValue escapes control and non-ASCII characters of a request value
// before it goes into a log message, so a crafted URL cannot forge log lines
// or inject terminal escape sequences.
func escapeLogValue(s string) string {
	q := strconv.QuoteToASCII(s)
	q = q[1 : len(q)-1]
	return strings.ReplaceAll(q, `\"`, `"`)
}
