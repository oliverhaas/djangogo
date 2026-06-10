package forms

// ModelFormOption configures how FromModel and FromStruct derive a form from a
// model, mirroring the Meta options of a Django ModelForm (fields, exclude,
// labels, widgets, help_texts).
type ModelFormOption func(*modelFormConfig)

// modelFormConfig accumulates the options applied to a model-derived form.
type modelFormConfig struct {
	only      []string          // WithFields: include only these, in this order.
	exclude   map[string]bool   // WithExclude: drop these names.
	labels    map[string]string // WithLabel: override a field's label.
	widgets   map[string]Widget // WithWidget: override a field's widget.
	helpTexts map[string]string // WithHelpText: set a field's help text.
	required  map[string]bool   // WithRequired: override a field's required flag.
}

// WithFields restricts the form to the named fields, in the given order,
// mirroring a ModelForm Meta.fields list. Names that are unknown or skipped (the
// auto primary key) are ignored.
func WithFields(names ...string) ModelFormOption {
	return func(c *modelFormConfig) { c.only = append(c.only, names...) }
}

// WithExclude drops the named fields from the form, mirroring Meta.exclude. An
// exclusion wins over an inclusion when a name appears in both.
func WithExclude(names ...string) ModelFormOption {
	return func(c *modelFormConfig) {
		for _, n := range names {
			c.exclude[n] = true
		}
	}
}

// WithLabel overrides the human-readable label of a field.
func WithLabel(field, label string) ModelFormOption {
	return func(c *modelFormConfig) { c.labels[field] = label }
}

// WithWidget overrides the widget used to render a field.
func WithWidget(field string, w Widget) ModelFormOption {
	return func(c *modelFormConfig) { c.widgets[field] = w }
}

// WithHelpText sets the help text of a field.
func WithHelpText(field, text string) ModelFormOption {
	return func(c *modelFormConfig) { c.helpTexts[field] = text }
}

// WithRequired overrides whether a field is required.
func WithRequired(field string, required bool) ModelFormOption {
	return func(c *modelFormConfig) { c.required[field] = required }
}

// newModelFormConfig builds a config with initialized maps and applies opts.
func newModelFormConfig(opts []ModelFormOption) *modelFormConfig {
	c := &modelFormConfig{
		exclude:   map[string]bool{},
		labels:    map[string]string{},
		widgets:   map[string]Widget{},
		helpTexts: map[string]string{},
		required:  map[string]bool{},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// includedNames returns the field names to include, in order: cfg.only when set,
// otherwise the model's declaration order in allNames, with excluded names
// removed in both cases.
func (c *modelFormConfig) includedNames(allNames []string) []string {
	source := allNames
	if len(c.only) > 0 {
		source = c.only
	}
	out := make([]string, 0, len(source))
	for _, name := range source {
		if c.exclude[name] {
			continue
		}
		out = append(out, name)
	}
	return out
}

// apply mutates a field with the per-field overrides keyed by the field's name.
func (c *modelFormConfig) apply(f *Field) {
	if label, ok := c.labels[f.Name]; ok {
		f.Label = label
	}
	if w, ok := c.widgets[f.Name]; ok {
		f.Widget = w
	}
	if text, ok := c.helpTexts[f.Name]; ok {
		f.HelpText = text
	}
	if req, ok := c.required[f.Name]; ok {
		f.Required = req
	}
}
