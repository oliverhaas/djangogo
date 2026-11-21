package forms

import (
	"sort"
	"strings"

	"github.com/oliverhaas/djangogo/validators"
)

// NonFieldErrors is the key under which an ErrorDict stores the errors that
// belong to the form as a whole, Django's NON_FIELD_ERRORS.
const NonFieldErrors = validators.NonFieldErrors

// ValidationError is a validation failure, aliased to validators.ValidationError
// so form and model validation share one error type.
type ValidationError = validators.ValidationError

// NewValidationError returns a single error with the given message template
// and code, Django's ValidationError(message, code=code, params=params). At
// most one params map is used.
func NewValidationError(message, code string, params ...map[string]any) *ValidationError {
	return validators.NewValidationError(message, code, params...)
}

// ValidationErrors returns a list error, Django's ValidationError([...]).
func ValidationErrors(errs ...any) *ValidationError {
	return validators.ValidationErrors(errs...)
}

// ValidationErrorDict returns a dictionary error, Django's
// ValidationError({...}).
func ValidationErrorDict(fields map[string]any) *ValidationError {
	return validators.ValidationErrorDict(fields)
}

// toValidationError converts a loosely typed error value into a *ValidationError.
func toValidationError(v any) *ValidationError {
	return validators.ToValidationError(v)
}

// sortedKeys returns the keys of m in ascending order.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// escape escapes s for HTML exactly as Django's escape() does (html.escape of
// the Python standard library), which differs from Go's html.EscapeString in
// the entities of the quote characters.
func escape(s string) string {
	if !strings.ContainsAny(s, `&<>"'`) {
		return s
	}
	return htmlReplacer.Replace(s)
}

// htmlReplacer holds the five replacements of Python's html.escape.
var htmlReplacer = strings.NewReplacer(
	"&", "&amp;",
	"<", "&lt;",
	">", "&gt;",
	`"`, "&quot;",
	"'", "&#x27;",
)
