package forms

import (
	"encoding/json"
	"strings"
)

// ErrorList is the list of errors of one field or of the form, the counterpart
// of Django's ErrorList. The zero value is an empty list.
type ErrorList struct {
	data       []*ValidationError
	errorClass string
	fieldID    string
}

// newErrorList returns an empty list. extraClass is appended to the CSS class
// "errorlist" (Django passes "nonfield" for form-level errors) and fieldID is
// the auto id of the field the list belongs to.
func newErrorList(extraClass, fieldID string) *ErrorList {
	l := &ErrorList{errorClass: "errorlist", fieldID: fieldID}
	if extraClass != "" {
		l.errorClass += " " + extraClass
	}
	return l
}

// NewErrorList returns a list of the given errors. Each is a message string,
// an error or a *ValidationError; a list error contributes all its errors.
func NewErrorList(errs ...any) ErrorList {
	l := newErrorList("", "")
	for _, e := range errs {
		l.data = append(l.data, toValidationError(e).ErrorList()...)
	}
	return *l
}

// Len returns the number of errors.
func (l ErrorList) Len() int { return len(l.data) }

// Messages returns the formatted messages, what iterating a Django ErrorList
// yields. It never returns nil, so it compares equal to an empty literal.
func (l ErrorList) Messages() []string {
	out := make([]string, len(l.data))
	for i, e := range l.data {
		out[i] = e.format()
	}
	return out
}

// AsData returns the errors as ValidationError values, Django's as_data().
func (l ErrorList) AsData() []*ValidationError {
	return append([]*ValidationError(nil), l.data...)
}

// Contains reports whether msg is one of the formatted messages.
func (l ErrorList) Contains(msg string) bool {
	for _, e := range l.data {
		if e.format() == msg {
			return true
		}
	}
	return false
}

// ErrorJSON is one entry of the JSON form of an error list.
type ErrorJSON struct {
	Message string `json:"message"`
	Code    string `json:"code"`
}

// JSONData returns the errors as message and code pairs, Django's
// get_json_data(). With escapeHTML the messages are HTML-escaped, for output
// that is inserted into a page without escaping.
func (l ErrorList) JSONData(escapeHTML bool) []ErrorJSON {
	out := make([]ErrorJSON, len(l.data))
	for i, e := range l.data {
		msg := e.format()
		if escapeHTML {
			msg = escape(msg)
		}
		out[i] = ErrorJSON{Message: msg, Code: e.Code}
	}
	return out
}

// AsJSON returns JSONData serialized as JSON, Django's as_json(). The JSON is
// compact; Django's has a space after each separator.
func (l ErrorList) AsJSON(escapeHTML bool) string {
	return marshalJSON(l.JSONData(escapeHTML))
}

// AsText returns the errors as a plain-text list, Django's as_text().
func (l ErrorList) AsText() string {
	var b strings.Builder
	for i, e := range l.data {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString("* " + e.format())
	}
	return b.String()
}

// AsUL returns the errors as an HTML list, Django's as_ul(). An empty list
// renders as the empty string.
func (l ErrorList) AsUL() string {
	if len(l.data) == 0 {
		return ""
	}
	var b strings.Builder
	class := l.errorClass
	if class == "" {
		class = "errorlist"
	}
	b.WriteString(`<ul class="` + escape(class) + `"`)
	if l.fieldID != "" {
		b.WriteString(` id="` + escape(l.fieldID) + `_error"`)
	}
	b.WriteString(">")
	for _, e := range l.data {
		b.WriteString("<li>" + escape(e.format()) + "</li>")
	}
	b.WriteString("</ul>")
	return b.String()
}

// String returns AsUL, as str() of a Django ErrorList does.
func (l ErrorList) String() string { return l.AsUL() }

// ErrorDict holds the errors of a form by field name, the counterpart of
// Django's ErrorDict. Form-level errors are stored under NonFieldErrors. The
// fields keep the order in which their first error was added.
type ErrorDict struct {
	order []string
	lists map[string]*ErrorList
}

// newErrorDict returns an empty dictionary.
func newErrorDict() *ErrorDict {
	return &ErrorDict{lists: map[string]*ErrorList{}}
}

// Len returns the number of fields that have errors.
func (d *ErrorDict) Len() int {
	if d == nil {
		return 0
	}
	return len(d.order)
}

// Has reports whether field has at least one error.
func (d *ErrorDict) Has(field string) bool {
	if d == nil {
		return false
	}
	_, ok := d.lists[field]
	return ok
}

// Get returns the errors of field, an empty list when it has none.
func (d *ErrorDict) Get(field string) ErrorList {
	if d == nil {
		return ErrorList{}
	}
	if l, ok := d.lists[field]; ok {
		return *l
	}
	return ErrorList{}
}

// Fields returns the names of the fields with errors, in the order their first
// error was added.
func (d *ErrorDict) Fields() []string {
	if d == nil {
		return nil
	}
	return append([]string(nil), d.order...)
}

// Messages returns the formatted messages by field name, the value a Django
// test compares form.errors against.
func (d *ErrorDict) Messages() map[string][]string {
	out := map[string][]string{}
	if d == nil {
		return out
	}
	for _, name := range d.order {
		out[name] = d.lists[name].Messages()
	}
	return out
}

// AsData returns the errors by field name as ValidationError values, Django's
// as_data().
func (d *ErrorDict) AsData() map[string][]*ValidationError {
	out := map[string][]*ValidationError{}
	if d == nil {
		return out
	}
	for _, name := range d.order {
		out[name] = d.lists[name].AsData()
	}
	return out
}

// JSONData returns the errors by field name as message and code pairs, Django's
// get_json_data().
func (d *ErrorDict) JSONData(escapeHTML bool) map[string][]ErrorJSON {
	out := map[string][]ErrorJSON{}
	if d == nil {
		return out
	}
	for _, name := range d.order {
		out[name] = d.lists[name].JSONData(escapeHTML)
	}
	return out
}

// AsJSON returns JSONData serialized as JSON, Django's as_json(). The keys are
// in alphabetical order.
func (d *ErrorDict) AsJSON(escapeHTML bool) string {
	return marshalJSON(d.JSONData(escapeHTML))
}

// AsText returns the errors as a plain-text outline, Django's as_text().
func (d *ErrorDict) AsText() string {
	if d == nil {
		return ""
	}
	var b strings.Builder
	for i, name := range d.order {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString("* " + name)
		for _, msg := range d.lists[name].Messages() {
			b.WriteString("\n  * " + msg)
		}
	}
	return b.String()
}

// AsUL returns the errors as a nested HTML list, Django's as_ul(). An empty
// dictionary renders as the empty string.
func (d *ErrorDict) AsUL() string {
	if d.Len() == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(`<ul class="errorlist">`)
	for _, name := range d.order {
		b.WriteString("<li>" + escape(name) + d.lists[name].AsUL() + "</li>")
	}
	b.WriteString("</ul>")
	return b.String()
}

// String returns AsUL, as str() of a Django ErrorDict does.
func (d *ErrorDict) String() string { return d.AsUL() }

// add appends errs to the list of field, creating the list with the given CSS
// class and field id when it is the first error of that field.
func (d *ErrorDict) add(field, extraClass, fieldID string, errs []*ValidationError) {
	l, ok := d.lists[field]
	if !ok {
		l = newErrorList(extraClass, fieldID)
		d.lists[field] = l
		d.order = append(d.order, field)
	}
	l.data = append(l.data, errs...)
}

// marshalJSON serializes v without escaping HTML characters, as Python's
// json.dumps does.
func marshalJSON(v any) string {
	var b strings.Builder
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return "null"
	}
	return strings.TrimSuffix(b.String(), "\n")
}
