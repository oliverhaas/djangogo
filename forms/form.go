package forms

import (
	"html"
	"net/url"
	"strings"
)

// Form is a collection of fields that validates submitted data and renders HTML.
// The zero Form is not usable; construct one with New.
type Form struct {
	fields   []*Field
	data     url.Values
	errors   map[string][]string
	cleaned  map[string]any
	nonField []string
	bound    bool
}

// New returns a Form with the given fields.
func New(fields ...*Field) *Form {
	return &Form{
		fields:  fields,
		errors:  make(map[string][]string),
		cleaned: make(map[string]any),
	}
}

// Bind attaches submitted data and marks the form as bound. It returns the form
// for chaining.
func (f *Form) Bind(data url.Values) *Form {
	f.data = data
	f.bound = true
	return f
}

// IsValid cleans every field, populating Errors and Cleaned, and reports whether
// no errors (field or non-field) were recorded.
func (f *Form) IsValid() bool {
	f.errors = make(map[string][]string)
	f.cleaned = make(map[string]any)
	if len(f.nonField) > 0 {
		f.errors[""] = append(f.errors[""], f.nonField...)
	}

	for _, field := range f.fields {
		raw := f.rawValue(field)
		val, err := field.Clean(raw)
		if err != nil {
			f.errors[field.Name] = append(f.errors[field.Name], err.Error())
			continue
		}
		f.cleaned[field.Name] = val
	}
	return len(f.errors) == 0
}

// rawValue returns the submitted raw value for a field. For a checkbox the mere
// presence of the key counts as truthy.
func (f *Form) rawValue(field *Field) string {
	if field.Kind == BoolField {
		if _, ok := f.data[field.Name]; ok {
			v := f.data.Get(field.Name)
			if v == "" {
				return "on"
			}
			return v
		}
		return ""
	}
	return f.data.Get(field.Name)
}

// Cleaned returns the cleaned values gathered by the last IsValid call.
func (f *Form) Cleaned() map[string]any { return f.cleaned }

// Errors returns the field and non-field errors gathered by the last IsValid
// call (and any added via AddError). The "" key holds non-field errors.
func (f *Form) Errors() map[string][]string { return f.errors }

// AddError records an error message. An empty field name records a non-field
// error (shown above the fields).
func (f *Form) AddError(field, msg string) {
	if field == "" {
		f.nonField = append(f.nonField, msg)
	}
	if f.errors == nil {
		f.errors = make(map[string][]string)
	}
	f.errors[field] = append(f.errors[field], msg)
}

// Fields returns the form's fields in declaration order.
func (f *Form) Fields() []*Field { return f.fields }

// EmptyChoiceLabel is the label of the blank option prepended to a foreign-key
// <select>, mirroring Django's ModelChoiceField.empty_label ("---------").
const EmptyChoiceLabel = "---------"

// SetChoices populates the named field's option set, used to fill a foreign-key
// <select> with the related model's rows after the form is built. It prepends an
// empty option (Django's empty_label) so the relation can be left blank: a
// non-required FK then submits empty and clears to NULL, and a required FK is not
// silently pre-set to the first row (Clean rejects the empty submission because
// the empty value is checked before choice membership). It updates the field's
// Choices (which ChoiceField validation reads) and, when the field renders as a
// Select, refreshes the widget's options. Unknown names are ignored.
func (f *Form) SetChoices(name string, choices [][2]string) {
	for _, field := range f.fields {
		if field.Name != name {
			continue
		}
		withEmpty := make([][2]string, 0, len(choices)+1)
		withEmpty = append(withEmpty, [2]string{"", EmptyChoiceLabel})
		withEmpty = append(withEmpty, choices...)
		field.Choices = withEmpty
		if _, ok := field.Widget.(Select); ok {
			field.Widget = Select{Choices: withEmpty}
		}
		return
	}
}

// BoundValue returns the currently submitted value for name, for re-rendering.
func (f *Form) BoundValue(name string) string {
	if f.data == nil {
		return ""
	}
	return f.data.Get(name)
}

// Render renders the form as a sequence of <p> blocks (Django's as_p style):
// non-field errors first, then for each field a label, its widget populated with
// the bound value, and that field's errors. All dynamic content is HTML-escaped.
func (f *Form) Render() string {
	var b strings.Builder
	for _, msg := range f.errors[""] {
		b.WriteString(`<p class="errornote">`)
		b.WriteString(html.EscapeString(msg))
		b.WriteString(`</p>`)
	}
	for _, field := range f.fields {
		b.WriteString(`<p>`)
		label := field.Label
		if label == "" {
			label = field.Name
		}
		b.WriteString(`<label>`)
		b.WriteString(html.EscapeString(label))
		b.WriteString(`</label>`)
		b.WriteString(field.defaultWidget().Render(field.Name, f.BoundValue(field.Name), nil))
		for _, msg := range f.errors[field.Name] {
			b.WriteString(`<span class="error">`)
			b.WriteString(html.EscapeString(msg))
			b.WriteString(`</span>`)
		}
		b.WriteString(`</p>`)
	}
	return b.String()
}
