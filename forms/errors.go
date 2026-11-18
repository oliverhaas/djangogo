package forms

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// NonFieldErrors is the key under which an ErrorDict stores the errors that
// belong to the form as a whole, Django's NON_FIELD_ERRORS.
const NonFieldErrors = "__all__"

// ValidationError is a validation failure, the counterpart of Django's
// ValidationError. It has one of three shapes:
//
//   - a single error: Message, with an optional Code and Params,
//   - a list of errors: Errors,
//   - a dictionary of errors by field name: Fields.
//
// Message is a message template in Django's notation: a placeholder
// "%(name)s" or "%(name)d" is replaced by Params["name"]. The formatted text is
// shown to the user verbatim, so it is a full sentence, unlike a conventional
// lowercase Go error string.
type ValidationError struct {
	// Message is the message template of a single error.
	Message string
	// Code identifies the kind of failure ("required", "invalid", "max_length").
	Code string
	// Params are the values substituted into Message.
	Params map[string]any
	// Errors are the members of a list error.
	Errors []*ValidationError
	// Fields maps a field name (or NonFieldErrors) to its errors.
	Fields map[string][]*ValidationError
}

// NewValidationError returns a single error with the given message template
// and code, Django's ValidationError(message, code=code, params=params). At
// most one params map is used.
func NewValidationError(message, code string, params ...map[string]any) *ValidationError {
	e := &ValidationError{Message: message, Code: code}
	if len(params) > 0 {
		e.Params = params[0]
	}
	return e
}

// ValidationErrors returns a list error, Django's ValidationError([...]). Each
// argument is a message string, an error, or a *ValidationError; nested lists
// and dictionaries are flattened into the list.
func ValidationErrors(errs ...any) *ValidationError {
	out := &ValidationError{Errors: []*ValidationError{}}
	for _, e := range errs {
		out.Errors = append(out.Errors, toValidationError(e).ErrorList()...)
	}
	return out
}

// ValidationErrorDict returns a dictionary error, Django's
// ValidationError({...}). Each value is a message string, an error, a
// *ValidationError, or a slice of those.
func ValidationErrorDict(fields map[string]any) *ValidationError {
	out := &ValidationError{Fields: make(map[string][]*ValidationError, len(fields))}
	for name, v := range fields {
		out.Fields[name] = toValidationError(v).ErrorList()
	}
	return out
}

// toValidationError converts a loosely typed error value into a
// *ValidationError: nil is an empty list, a string a single message, a slice a
// list, a map a dictionary, and any other error a single message.
func toValidationError(v any) *ValidationError {
	switch e := v.(type) {
	case nil:
		return &ValidationError{Errors: []*ValidationError{}}
	case *ValidationError:
		if e == nil {
			return &ValidationError{Errors: []*ValidationError{}}
		}
		return e
	case string:
		return &ValidationError{Message: e}
	case []string:
		items := make([]any, len(e))
		for i, s := range e {
			items[i] = s
		}
		return ValidationErrors(items...)
	case []any:
		return ValidationErrors(e...)
	case []error:
		items := make([]any, len(e))
		for i, s := range e {
			items[i] = s
		}
		return ValidationErrors(items...)
	case []*ValidationError:
		items := make([]any, len(e))
		for i, s := range e {
			items[i] = s
		}
		return ValidationErrors(items...)
	case map[string]any:
		return ValidationErrorDict(e)
	case map[string][]string:
		fields := make(map[string]any, len(e))
		for k, msgs := range e {
			fields[k] = msgs
		}
		return ValidationErrorDict(fields)
	case map[string]string:
		fields := make(map[string]any, len(e))
		for k, msg := range e {
			fields[k] = msg
		}
		return ValidationErrorDict(fields)
	case error:
		var ve *ValidationError
		if errors.As(e, &ve) {
			return ve
		}
		return &ValidationError{Message: e.Error()}
	default:
		return &ValidationError{Message: fmt.Sprint(v)}
	}
}

// isDict reports whether e is a dictionary error.
func (e *ValidationError) isDict() bool { return e.Fields != nil }

// isList reports whether e is a list error.
func (e *ValidationError) isList() bool { return e.Fields == nil && e.Errors != nil }

// ErrorList returns the single errors e consists of: e itself for a single
// error, the members of a list, and all errors of a dictionary (in field name
// order). It is Django's error_list.
func (e *ValidationError) ErrorList() []*ValidationError {
	switch {
	case e.isDict():
		var out []*ValidationError
		for _, name := range sortedKeys(e.Fields) {
			out = append(out, e.Fields[name]...)
		}
		return out
	case e.isList():
		return e.Errors
	default:
		return []*ValidationError{e}
	}
}

// Messages returns the formatted messages of every single error in e, Django's
// ValidationError.messages.
func (e *ValidationError) Messages() []string {
	list := e.ErrorList()
	out := make([]string, len(list))
	for i, item := range list {
		out[i] = item.format()
	}
	return out
}

// MessageDict returns the formatted messages of a dictionary error by field
// name, Django's message_dict. For a list or single error the messages are
// filed under NonFieldErrors.
func (e *ValidationError) MessageDict() map[string][]string {
	out := map[string][]string{}
	if !e.isDict() {
		out[NonFieldErrors] = e.Messages()
		return out
	}
	for name, list := range e.Fields {
		msgs := make([]string, len(list))
		for i, item := range list {
			msgs[i] = item.format()
		}
		out[name] = msgs
	}
	return out
}

// format returns the message of a single error with its params substituted.
func (e *ValidationError) format() string {
	return formatMessage(e.Message, e.Params)
}

// Error implements the error interface. A single error is its formatted
// message; a list joins its messages with "; "; a dictionary prefixes each
// field's messages with the field name.
func (e *ValidationError) Error() string {
	if e.isDict() {
		parts := make([]string, 0, len(e.Fields))
		for _, name := range sortedKeys(e.Fields) {
			msgs := make([]string, len(e.Fields[name]))
			for i, item := range e.Fields[name] {
				msgs[i] = item.format()
			}
			parts = append(parts, name+": "+strings.Join(msgs, "; "))
		}
		return strings.Join(parts, "; ")
	}
	return strings.Join(e.Messages(), "; ")
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

// formatMessage substitutes Django-style named placeholders in msg:
// "%(name)s" and "%(name)d" become the value of params["name"], and "%%"
// becomes "%". Without params the message is returned unchanged, as Python
// only applies the % operator when params are given.
func formatMessage(msg string, params map[string]any) string {
	if len(params) == 0 || !strings.Contains(msg, "%") {
		return msg
	}
	var b strings.Builder
	for i := 0; i < len(msg); i++ {
		c := msg[i]
		if c != '%' || i+1 >= len(msg) {
			b.WriteByte(c)
			continue
		}
		if msg[i+1] == '%' {
			b.WriteByte('%')
			i++
			continue
		}
		if msg[i+1] != '(' {
			b.WriteByte(c)
			continue
		}
		end := strings.IndexByte(msg[i:], ')')
		if end < 0 || i+end+1 >= len(msg) {
			b.WriteByte(c)
			continue
		}
		name := msg[i+2 : i+end]
		val, ok := params[name]
		if !ok {
			b.WriteByte(c)
			continue
		}
		b.WriteString(pyString(val))
		i += end + 1 // skip the conversion character after ")"
	}
	return b.String()
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
