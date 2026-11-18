package forms

import (
	"slices"

	"github.com/oliverhaas/djangogo/orm"
)

// ModelFormOption configures how a form is derived from a model, the
// counterpart of the Meta options of a Django ModelForm:
//
//	Meta.fields          WithFields
//	Meta.exclude         WithExclude
//	Meta.labels          WithLabel, once per field
//	Meta.widgets         WithWidget, once per field
//	Meta.help_texts      WithHelpText, once per field
//	Meta.error_messages  WithErrorMessages, once per field
//	Meta.field_classes   WithFormField, once per field
//
// A field is named by its attribute name ("created_at"); the Go field name
// ("CreatedAt") is accepted as well.
type ModelFormOption func(*modelFormConfig)

// fieldFactory builds the form field of a model field from the options that
// its model field implies.
type fieldFactory func(db *orm.DB, opts []FieldOption) Field

// modelFormConfig accumulates the options of a model form.
type modelFormConfig struct {
	only          []string
	fieldsSet     bool // WithFields was given, maybe without names
	declared      []string
	exclude       map[string]bool
	labels        map[string]string
	widgets       map[string]Widget
	helpTexts     map[string]string
	required      map[string]bool
	errorMessages map[string]map[string]string
	formFields    map[string]Field
	factories     map[string]fieldFactory
}

// AllFields is the name list entry that selects every editable field of the
// model, Django's ALL_FIELDS ("__all__"): WithFields(AllFields).
const AllFields = "__all__"

// WithFields restricts the form to the named fields, in the given order
// (Meta.fields); without names the form has no model fields. AllFields selects
// every editable field, in the order of the model. A name that is neither a
// model field nor declared with WithFormField makes the form fail (see
// Form.Err) with Django's FieldError message "Unknown field(s) (name)
// specified for Model", and so does a field that is not editable. The auto
// primary key is ignored.
func WithFields(names ...string) ModelFormOption {
	return func(c *modelFormConfig) {
		c.fieldsSet = true
		c.only = append(c.only, names...)
	}
}

// WithExclude drops the named fields from the form (Meta.exclude). An
// exclusion wins over an inclusion.
func WithExclude(names ...string) ModelFormOption {
	return func(c *modelFormConfig) {
		for _, n := range names {
			c.exclude[n] = true
		}
	}
}

// WithLabel overrides the label of a field (Meta.labels).
func WithLabel(field, label string) ModelFormOption {
	return func(c *modelFormConfig) { c.labels[field] = label }
}

// WithWidget overrides the widget of a field (Meta.widgets).
func WithWidget(field string, w Widget) ModelFormOption {
	return func(c *modelFormConfig) { c.widgets[field] = w }
}

// WithHelpText overrides the help text of a field (Meta.help_texts).
func WithHelpText(field, text string) ModelFormOption {
	return func(c *modelFormConfig) { c.helpTexts[field] = text }
}

// WithRequired overrides whether a field is required, which otherwise follows
// the blank option of the model field.
func WithRequired(field string, required bool) ModelFormOption {
	return func(c *modelFormConfig) { c.required[field] = required }
}

// WithErrorMessages overrides error messages of a field by code
// (Meta.error_messages).
func WithErrorMessages(field string, messages map[string]string) ModelFormOption {
	return func(c *modelFormConfig) {
		if c.errorMessages[field] == nil {
			c.errorMessages[field] = map[string]string{}
		}
		for code, msg := range messages {
			c.errorMessages[field][code] = msg
		}
	}
}

// WithFormField declares a form field, as a field declared on a Django
// ModelForm: the form always has it, whatever WithFields and WithExclude say.
// For a model field it takes the place of the derived field (also the
// counterpart of Meta.field_classes); WithFields gives its position, and
// without one it comes after the fields of the form, in the order of the
// options. WithLabel, WithWidget and the other overrides of the same name
// apply to it as well.
func WithFormField(field string, f Field) ModelFormOption {
	return func(c *modelFormConfig) {
		if _, ok := c.formFields[field]; !ok {
			c.declared = append(c.declared, field)
		}
		c.formFields[field] = f
	}
}

// WithQuerySet restricts the options of a foreign key field to the rows of qs,
// the counterpart of setting the queryset of a ModelChoiceField. The cleaned
// value of the field becomes the selected *R. It needs a database, so it
// applies to NewModelForm and not to FromModel.
func WithQuerySet[R any](field string, qs *orm.QuerySet[R]) ModelFormOption {
	return func(c *modelFormConfig) {
		c.factories[field] = func(db *orm.DB, opts []FieldOption) Field {
			if db == nil {
				return nil
			}
			return NewModelChoiceField(db, qs, opts...)
		}
	}
}

// newModelFormConfig applies opts to an empty configuration.
func newModelFormConfig(opts []ModelFormOption) *modelFormConfig {
	c := &modelFormConfig{
		exclude:       map[string]bool{},
		labels:        map[string]string{},
		widgets:       map[string]Widget{},
		helpTexts:     map[string]string{},
		required:      map[string]bool{},
		errorMessages: map[string]map[string]string{},
		formFields:    map[string]Field{},
		factories:     map[string]fieldFactory{},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// attrName returns the attribute name of the model field that name refers to,
// by attribute name or by Go field name, and name itself when there is none.
func attrName(m *orm.Model, name string) string {
	if _, ok := m.FieldByAttr(name); ok {
		return name
	}
	if mf, ok := m.FieldByName(name); ok {
		return mf.Attr
	}
	return name
}

// normalize rewrites every field name of the configuration to its attribute
// name, so that the rest of the code looks names up one way.
func (c *modelFormConfig) normalize(m *orm.Model) {
	for i, n := range c.only {
		c.only[i] = attrName(m, n)
	}
	for i, n := range c.declared {
		c.declared[i] = attrName(m, n)
	}
	c.exclude = renamed(m, c.exclude)
	c.labels = renamed(m, c.labels)
	c.widgets = renamed(m, c.widgets)
	c.helpTexts = renamed(m, c.helpTexts)
	c.required = renamed(m, c.required)
	c.errorMessages = renamed(m, c.errorMessages)
	c.formFields = renamed(m, c.formFields)
	c.factories = renamed(m, c.factories)
}

// renamed returns in with its keys rewritten to attribute names.
func renamed[V any](m *orm.Model, in map[string]V) map[string]V {
	out := make(map[string]V, len(in))
	for k, v := range in {
		out[attrName(m, k)] = v
	}
	return out
}

// includedNames returns the names of the fields to include, in order: the
// WithFields list when given, the model's declaration order otherwise, without
// the excluded names.
func (c *modelFormConfig) includedNames(all []string) []string {
	source := all
	if c.fieldsSet && !slices.Contains(c.only, AllFields) {
		source = c.only
	}
	out := make([]string, 0, len(source))
	for _, name := range source {
		if !c.exclude[name] {
			out = append(out, name)
		}
	}
	return out
}

// apply sets the per-field overrides on a built field.
func (c *modelFormConfig) apply(name string, f Field) {
	base := f.Base()
	if label, ok := c.labels[name]; ok {
		base.SetLabel(label)
	}
	if w, ok := c.widgets[name]; ok {
		base.Widget = w
	}
	if text, ok := c.helpTexts[name]; ok {
		base.HelpText = text
	}
	if req, ok := c.required[name]; ok {
		base.Required = req
	}
	for code, msg := range c.errorMessages[name] {
		base.ErrorMessages[code] = msg
	}
}
