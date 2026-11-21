package forms

import (
	"context"
	"fmt"
	"net/url"
	"slices"
	"strings"
)

// NamedField pairs a field with its name in a form.
type NamedField struct {
	// Name is the key of the field in submitted data and in CleanedData.
	Name string
	// Field is the field definition.
	Field Field
}

// F names a field for New: forms.New(forms.F("title", forms.NewCharField())).
func F(name string, field Field) NamedField {
	return NamedField{Name: name, Field: field}
}

// Form is a set of named fields that validates submitted data and renders
// itself as HTML, the counterpart of Django's Form. A Form is built per
// request, usually by a constructor function of the application:
//
//	func newContactForm() *forms.Form {
//		return forms.New(
//			forms.F("subject", forms.NewCharField(forms.MaxLength(100))),
//			forms.F("sender", forms.NewEmailField()),
//		)
//	}
//
//	form := newContactForm().Bind(r.PostForm)
//	if form.IsValid() {
//		subject := form.CleanedData()["subject"].(string)
//	}
//
// The exported fields are the constructor arguments of a Django form; set
// them before the form is rendered or validated.
type Form struct {
	// Prefix is put before every field name in the HTML, as "prefix-name", so
	// that several forms share one <form> tag.
	Prefix string
	// AutoID is the pattern of the HTML id of a field, with %s for the field
	// name. "id_%s" is the default, "%s" uses the name itself (Django's
	// auto_id=True) and "" leaves ids and <label> tags out.
	AutoID string
	// Initial holds initial values by field name. An entry overrides the
	// Initial option of the field.
	Initial map[string]any
	// LabelSuffix is appended to a label that does not end in punctuation; it
	// defaults to ":".
	LabelSuffix string
	// EmptyPermitted lets a bound form with no changed data pass validation
	// unvalidated; formsets use it for their extra forms.
	EmptyPermitted bool
	// UseRequiredAttribute renders the HTML required attribute on the widgets
	// of required fields; it defaults to true.
	UseRequiredAttribute bool
	// RequiredCSSClass and ErrorCSSClass are added to the rows of required
	// fields and of fields with errors.
	RequiredCSSClass, ErrorCSSClass string

	names  []string
	fields map[string]Field
	// boundFields caches the bound fields, as Django's _bound_fields_cache.
	boundFields map[string]*BoundField

	bound   bool
	data    url.Values
	errors  *ErrorDict
	cleaned map[string]any

	fieldCleaners map[string]func(*Form, any) (any, error)
	cleaners      []func(*Form) error
	postCleaners  []func(*Form)

	ctx      context.Context
	resolved bool
	err      error
}

// New returns an unbound form with the given fields, in order.
func New(fields ...NamedField) *Form {
	f := &Form{
		AutoID:               "id_%s",
		LabelSuffix:          ":",
		UseRequiredAttribute: true,
		fields:               make(map[string]Field, len(fields)),
		fieldCleaners:        map[string]func(*Form, any) (any, error){},
	}
	for _, nf := range fields {
		f.AddField(nf.Name, nf.Field)
	}
	return f
}

// AddField appends a field, or replaces the field of the same name in place.
func (f *Form) AddField(name string, field Field) {
	if _, ok := f.fields[name]; !ok {
		f.names = append(f.names, name)
	}
	f.fields[name] = field
	delete(f.boundFields, name)
}

// RemoveField drops a field from the form.
func (f *Form) RemoveField(name string) {
	if _, ok := f.fields[name]; !ok {
		return
	}
	delete(f.fields, name)
	delete(f.boundFields, name)
	for i, n := range f.names {
		if n == name {
			f.names = append(f.names[:i:i], f.names[i+1:]...)
			break
		}
	}
}

// OrderFields rearranges the fields: the named ones come first in the given
// order, the others keep their relative order, as Django's order_fields does.
func (f *Form) OrderFields(names ...string) {
	seen := map[string]bool{}
	ordered := make([]string, 0, len(f.names))
	for _, n := range names {
		if _, ok := f.fields[n]; ok && !seen[n] {
			ordered = append(ordered, n)
			seen[n] = true
		}
	}
	for _, n := range f.names {
		if !seen[n] {
			ordered = append(ordered, n)
		}
	}
	f.names = ordered
}

// FieldNames returns the names of the fields in order.
func (f *Form) FieldNames() []string { return append([]string(nil), f.names...) }

// FieldDef returns the definition of a field, Django's form.fields[name].
func (f *Form) FieldDef(name string) (Field, bool) {
	field, ok := f.fields[name]
	return field, ok
}

// clone returns a copy that shares the field definitions but not the state of
// a validation run.
func (f *Form) clone() *Form {
	c := *f
	c.names = append([]string(nil), f.names...)
	c.fields = make(map[string]Field, len(f.fields))
	for k, v := range f.fields {
		c.fields[k] = v
	}
	c.fieldCleaners = make(map[string]func(*Form, any) (any, error), len(f.fieldCleaners))
	for k, v := range f.fieldCleaners {
		c.fieldCleaners[k] = v
	}
	c.cleaners = slices.Clone(f.cleaners)
	c.postCleaners = slices.Clone(f.postCleaners)
	c.errors, c.cleaned, c.boundFields = nil, nil, nil
	return &c
}

// Bind returns a copy of the form bound to data, the counterpart of passing
// data to a Django form's constructor. The receiver stays unbound. Nil data
// binds an empty submission.
func (f *Form) Bind(data url.Values) *Form {
	c := f.clone()
	if data == nil {
		data = url.Values{}
	}
	c.bound, c.data = true, data
	return c
}

// IsBound reports whether the form holds submitted data.
func (f *Form) IsBound() bool { return f.bound }

// Data returns the submitted data, nil for an unbound form.
func (f *Form) Data() url.Values { return f.data }

// SetContext sets the context for the database queries of validation and
// rendering (model choice options, uniqueness checks). Without it the queries
// run with context.Background.
func (f *Form) SetContext(ctx context.Context) { f.ctx = ctx }

// context returns the form's context.
func (f *Form) context() context.Context {
	if f.ctx == nil {
		return context.Background()
	}
	return f.ctx
}

// Err returns the first failure that is not a validation error, such as a
// database error while loading choices. A form with such a failure is never
// valid; the caller answers it as a server error.
func (f *Form) Err() error { return f.err }

// fail records an infrastructure failure.
func (f *Form) fail(err error) {
	if f.err == nil && err != nil {
		f.err = err
	}
}

// resolve loads the data of the fields that need a database, once.
func (f *Form) resolve() {
	if f.resolved {
		return
	}
	f.resolved = true
	for _, name := range f.names {
		cf, ok := f.fields[name].(ContextField)
		if !ok {
			continue
		}
		field, err := cf.WithContext(f.context())
		if err != nil {
			f.fail(fmt.Errorf("forms: field %q: %w", name, err))
			continue
		}
		f.fields[name] = field
		delete(f.boundFields, name)
	}
}

// CleanFieldWith registers a function that runs after the named field has
// been cleaned, the counterpart of a clean_<name>() method. It receives the
// cleaned value and returns the value to store, or an error for the field.
func (f *Form) CleanFieldWith(name string, fn func(form *Form, value any) (any, error)) *Form {
	f.fieldCleaners[name] = fn
	return f
}

// CleanWith registers a function that runs after all fields have been
// cleaned, the counterpart of the clean() method. It reads CleanedData,
// reports field errors with AddError, and returns an error for the form as a
// whole.
func (f *Form) CleanWith(fn func(form *Form) error) *Form {
	f.cleaners = append(f.cleaners, fn)
	return f
}

// AddPrefix returns the HTML name of a field: "prefix-name" with a prefix.
func (f *Form) AddPrefix(name string) string {
	if f.Prefix == "" {
		return name
	}
	return f.Prefix + "-" + name
}

// Errors returns the errors of the form, validating it first when needed. An
// unbound form has none.
func (f *Form) Errors() *ErrorDict {
	if f.errors == nil {
		f.fullClean()
	}
	return f.errors
}

// IsValid reports whether the form is bound and has no errors.
func (f *Form) IsValid() bool {
	return f.bound && f.Errors().Len() == 0 && f.err == nil
}

// NonFieldErrors returns the errors that belong to the form as a whole.
func (f *Form) NonFieldErrors() ErrorList {
	if l, ok := f.Errors().lists[NonFieldErrors]; ok {
		return *l
	}
	return *newErrorList("nonfield", "")
}

// CleanedData returns the validated values by field name. It is nil until the
// form has been validated; a field with an error has no entry.
func (f *Form) CleanedData() map[string]any { return f.cleaned }

// Cleaned returns CleanedData after validating the form when needed.
func (f *Form) Cleaned() map[string]any {
	f.Errors()
	return f.cleaned
}

// AddError adds an error to a field, or to the form as a whole when field is
// "" or NonFieldErrors. err is a message string, an error, or a
// *ValidationError; a dictionary error with field "" spreads over its fields.
// The field loses its entry in CleanedData.
func (f *Form) AddError(field string, err any) {
	f.Errors()
	ve := toValidationError(err)
	byField := map[string][]*ValidationError{}
	order := []string{}
	if ve.IsDict() {
		if field != "" && field != NonFieldErrors {
			f.fail(fmt.Errorf("forms: AddError: a dictionary error cannot be added to the field %q", field))
			return
		}
		order = sortedKeys(ve.Fields)
		byField = ve.Fields
	} else {
		if field == "" {
			field = NonFieldErrors
		}
		order = append(order, field)
		byField[field] = ve.ErrorList()
	}
	for _, name := range order {
		switch {
		case name == NonFieldErrors:
			f.errors.add(name, "nonfield", "", byField[name])
		case f.fields[name] == nil:
			f.fail(fmt.Errorf("forms: AddError: the form has no field named %q", name))
			f.errors.add(NonFieldErrors, "nonfield", "", byField[name])
		default:
			f.errors.add(name, "", f.Field(name).AutoID(), byField[name])
		}
		delete(f.cleaned, name)
	}
}

// HasError reports whether the field has an error, with the given code when
// one is named.
func (f *Form) HasError(field string, code ...string) bool {
	if field == "" {
		field = NonFieldErrors
	}
	list, ok := f.Errors().lists[field]
	if !ok {
		return false
	}
	if len(code) == 0 {
		return true
	}
	for _, e := range list.data {
		if e.Code == code[0] {
			return true
		}
	}
	return false
}

// fullClean validates the form, Django's full_clean.
func (f *Form) fullClean() {
	f.errors = newErrorDict()
	if !f.bound {
		return
	}
	f.resolve()
	f.cleaned = map[string]any{}
	if f.EmptyPermitted && !f.HasChanged() {
		return
	}
	for _, name := range f.names {
		bf := f.Field(name)
		field := bf.field
		raw := bf.Data()
		if field.Base().Disabled {
			raw = bf.Initial()
		}
		value, err := field.Clean(raw)
		if err == nil {
			f.cleaned[name] = value
			if hook, ok := f.fieldCleaners[name]; ok {
				value, err = hook(f, value)
				if err == nil {
					f.cleaned[name] = value
				}
			}
		}
		if err != nil {
			f.AddError(name, err)
		}
	}
	for _, clean := range f.cleaners {
		if err := clean(f); err != nil {
			f.AddError("", err)
		}
	}
	for _, post := range f.postCleaners {
		post(f)
	}
}

// ChangedData returns the names of the fields whose submitted value differs
// from the initial one.
func (f *Form) ChangedData() []string {
	f.resolve()
	changed := []string{}
	for _, name := range f.names {
		bf := f.Field(name)
		if bf.field.HasChanged(bf.Initial(), bf.Data()) {
			changed = append(changed, name)
		}
	}
	return changed
}

// HasChanged reports whether any field differs from its initial value.
func (f *Form) HasChanged() bool { return len(f.ChangedData()) > 0 }

// Field returns the bound field of the given name, Django's form[name]. It
// returns nil for an unknown name.
func (f *Form) Field(name string) *BoundField {
	f.resolve()
	if bf, ok := f.boundFields[name]; ok {
		return bf
	}
	field, ok := f.fields[name]
	if !ok {
		return nil
	}
	bf := &BoundField{Name: name, form: f, field: field}
	if f.boundFields == nil {
		f.boundFields = map[string]*BoundField{}
	}
	f.boundFields[name] = bf
	return bf
}

// Fields returns the bound fields in order, what iterating a Django form
// yields.
func (f *Form) Fields() []*BoundField {
	out := make([]*BoundField, 0, len(f.names))
	for _, name := range f.names {
		out = append(out, f.Field(name))
	}
	return out
}

// HiddenFields returns the bound fields whose widget is hidden.
func (f *Form) HiddenFields() []*BoundField {
	var out []*BoundField
	for _, bf := range f.Fields() {
		if bf.IsHidden() {
			out = append(out, bf)
		}
	}
	return out
}

// VisibleFields returns the bound fields whose widget is not hidden.
func (f *Form) VisibleFields() []*BoundField {
	var out []*BoundField
	for _, bf := range f.Fields() {
		if !bf.IsHidden() {
			out = append(out, bf)
		}
	}
	return out
}

// SetChoices replaces the options of a choice field from (value, label)
// pairs. A model choice field keeps its empty first option. An unknown name or
// a field without choices is ignored.
func (f *Form) SetChoices(name string, choices [][2]string) {
	cf, ok := f.fields[name].(choiceField)
	if !ok {
		return
	}
	list := make([]Choice, len(choices))
	for i, c := range choices {
		list[i] = Choice{Value: c[0], Label: c[1]}
	}
	cf.setChoices(list)
}

// IsMultipart reports whether the form needs multipart encoding. No widget of
// this package uploads files, so it is false.
func (f *Form) IsMultipart() bool { return false }

// String returns AsDiv, as str() of a Django form does.
func (f *Form) String() string { return f.AsDiv() }

// HTML returns the rendered markup; templates print it without escaping.
func (f *Form) HTML() string { return f.String() }

// Render returns the form in its default layout, AsDiv.
func (f *Form) Render() string { return f.AsDiv() }

// Repr returns a short description of the form for messages, Django's
// repr(form): "<Form bound=True, valid=False, fields=(name;email)>". valid is
// Unknown until the form has been validated; Repr does not validate it.
func (f *Form) Repr() string {
	valid := "Unknown"
	if f.errors != nil {
		valid = pyString(f.bound && f.errors.Len() == 0)
	}
	return fmt.Sprintf("<Form bound=%s, valid=%s, fields=(%s)>", pyString(f.bound), valid, strings.Join(f.names, ";"))
}

// prettyName converts a field name to a label, Django's pretty_name:
// "first_name" becomes "First name".
func prettyName(name string) string {
	if name == "" {
		return ""
	}
	s := strings.ToLower(strings.ReplaceAll(name, "_", " "))
	return capfirst(s)
}

// capfirst uppercases the first character of s.
func capfirst(s string) string {
	for i, r := range s {
		return strings.ToUpper(string(r)) + s[i+len(string(r)):]
	}
	return s
}
