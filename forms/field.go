package forms

import (
	"reflect"
	"time"
)

// Field is one field of a form, the counterpart of Django's forms.Field. It
// converts submitted data to a Go value, validates it, and knows the widget
// that renders it. The concrete fields of this package embed BaseField, which
// implements every method with Django's default behavior.
//
// Clean runs the three steps of Django's Field.clean: ToGo (to_python),
// Validate and RunValidators.
type Field interface {
	// Base returns the options shared by all fields.
	Base() *BaseField
	// ToGo converts the widget's value to the field's Go type, Django's
	// to_python.
	ToGo(value any) (any, error)
	// Validate checks the converted value against the field's own rules.
	Validate(value any) error
	// RunValidators runs the field's validators over a non-empty value and
	// collects all their errors.
	RunValidators(value any) error
	// Clean converts and validates a value and returns the cleaned value.
	Clean(value any) (any, error)
	// PrepareValue converts a Go value to the form a widget displays.
	PrepareValue(value any) any
	// BoundData returns the value to display for a bound form: the submitted
	// data, or the initial value for a disabled field.
	BoundData(data, initial any) any
	// HasChanged reports whether the submitted data differs from the initial
	// value.
	HasChanged(initial, data any) bool
	// WidgetAttrs returns HTML attributes the field adds to its widget, such
	// as maxlength.
	WidgetAttrs(w Widget) Attrs
}

// BaseField holds the options every field has and implements Field with
// Django's default behavior. A custom field embeds it, overrides the methods
// that differ, and calls Init from its constructor so that the defaults
// dispatch to the overrides.
type BaseField struct {
	// Required rejects an empty value. It defaults to true.
	Required bool
	// Label is the human-readable name; nil derives it from the field name and
	// a pointer to "" suppresses the label.
	Label *string
	// LabelSuffix overrides the form's label suffix when not nil.
	LabelSuffix *string
	// Initial is the value shown in an unbound form. A func() any is called
	// when the value is needed.
	Initial any
	// Widget renders the field.
	Widget Widget
	// HelpText is descriptive text shown beside the widget. It is rendered
	// without escaping by the form layouts, as in Django.
	HelpText string
	// ErrorMessages maps an error code to its message template.
	ErrorMessages map[string]string
	// Validators run over a non-empty converted value.
	Validators []Validator
	// Disabled renders the widget disabled and ignores submitted data in favor
	// of the initial value.
	Disabled bool

	self Field
}

// Init prepares the embedded BaseField of a custom field: self is the field
// that embeds it, widget its default widget and messages its default error
// messages. The options are applied last.
func (b *BaseField) Init(self Field, widget Widget, messages map[string]string, opts ...FieldOption) {
	b.init(self, widget, messages, collectOptions(opts))
}

// init is Init with the options already collected.
func (b *BaseField) init(self Field, widget Widget, messages map[string]string, o *fieldOptions) {
	b.self = self
	b.Required = true
	if o.required != nil {
		b.Required = *o.required
	}
	b.Label = o.label
	b.LabelSuffix = o.labelSuffix
	b.Initial = o.initial
	b.Widget = widget
	if o.widget != nil {
		b.Widget = o.widget
	}
	b.HelpText = o.helpText
	b.Disabled = o.disabled
	b.ErrorMessages = map[string]string{"required": "This field is required."}
	for code, msg := range messages {
		b.ErrorMessages[code] = msg
	}
	for code, msg := range o.errorMessages {
		b.ErrorMessages[code] = msg
	}
	b.Validators = append(b.Validators, o.validators...)
}

// Base implements Field.
func (b *BaseField) Base() *BaseField { return b }

// SetLabel sets the label.
func (b *BaseField) SetLabel(label string) { b.Label = &label }

// Error returns a ValidationError with the field's message for code. At most
// one params map is used.
func (b *BaseField) Error(code string, params ...map[string]any) *ValidationError {
	return NewValidationError(b.ErrorMessages[code], code, params...)
}

// ToGo implements Field; the default returns the value unchanged.
func (b *BaseField) ToGo(value any) (any, error) { return value, nil }

// Validate implements Field; the default rejects an empty value of a required
// field.
func (b *BaseField) Validate(value any) error {
	if b.Required && isEmpty(value) {
		return b.Error("required")
	}
	return nil
}

// RunValidators implements Field. An error whose code has a message in
// ErrorMessages takes that message.
func (b *BaseField) RunValidators(value any) error {
	if isEmpty(value) {
		return nil
	}
	var errs []*ValidationError
	for _, validate := range b.Validators {
		err := validate(value)
		if err == nil {
			continue
		}
		ve := toValidationError(err)
		if msg, ok := b.ErrorMessages[ve.Code]; ok && ve.Code != "" && !ve.IsList() && !ve.IsDict() {
			clone := *ve
			clone.Message = msg
			ve = &clone
		}
		errs = append(errs, ve.ErrorList()...)
	}
	if len(errs) > 0 {
		return &ValidationError{Errors: errs}
	}
	return nil
}

// Clean implements Field.
func (b *BaseField) Clean(value any) (any, error) {
	f := b.self
	v, err := f.ToGo(value)
	if err != nil {
		return nil, err
	}
	if err := f.Validate(v); err != nil {
		return nil, err
	}
	if err := f.RunValidators(v); err != nil {
		return nil, err
	}
	return v, nil
}

// PrepareValue implements Field; the default returns the value unchanged.
func (b *BaseField) PrepareValue(value any) any { return value }

// BoundData implements Field.
func (b *BaseField) BoundData(data, initial any) any {
	if b.Disabled {
		return initial
	}
	return data
}

// HasChanged implements Field. Data that does not convert counts as changed.
func (b *BaseField) HasChanged(initial, data any) bool {
	if b.Disabled {
		return false
	}
	v, err := b.self.ToGo(data)
	if err != nil {
		return true
	}
	return !valuesEqual(orEmptyString(initial), orEmptyString(v))
}

// WidgetAttrs implements Field; the default adds nothing.
func (b *BaseField) WidgetAttrs(Widget) Attrs { return nil }

// isEmpty reports whether v is one of the values a field treats as "nothing
// submitted", Django's EMPTY_VALUES: nil, "" and an empty list.
func isEmpty(v any) bool {
	switch t := v.(type) {
	case nil:
		return true
	case string:
		return t == ""
	case []string:
		return len(t) == 0
	case []any:
		return len(t) == 0
	}
	return false
}

// orEmptyString maps nil to "", as Django's has_changed does before comparing.
func orEmptyString(v any) any {
	if v == nil {
		return ""
	}
	return v
}

// valuesEqual compares two field values: numbers by value across Go types,
// times by instant, everything else structurally.
func valuesEqual(a, b any) bool {
	if _, isString := a.(string); !isString {
		if c, ok := compareValues(a, b); ok {
			return c == 0
		}
	}
	if ta, ok := a.(time.Time); ok {
		if tb, ok := b.(time.Time); ok {
			return ta.Equal(tb)
		}
	}
	return reflect.DeepEqual(a, b)
}

// fieldOptions collects everything a FieldOption can set. Each constructor
// reads the options that apply to its field and ignores the rest.
type fieldOptions struct {
	required      *bool
	label         *string
	labelSuffix   *string
	initial       any
	widget        Widget
	helpText      string
	errorMessages map[string]string
	validators    []Validator
	disabled      bool

	maxLength     int
	minLength     int
	strip         *bool
	emptyValue    any
	hasEmptyValue bool
	minValue      any
	maxValue      any
	stepSize      any
	choices       []Choice
	coerce        func(string) (any, error)
	inputFormats  []string
	dateFormats   []string
	timeFormats   []string
	requireAll    *bool
	maxDigits     int
	decimalPlaces int
	emptyLabel    *string
	allowUnicode  bool
	assumeScheme  string
	protocol      string
	unpackIPv4    bool
	jsonEncoder   func(any) ([]byte, error)
	jsonDecoder   func([]byte) (any, error)
}

// FieldOption configures a field at construction, the counterpart of a keyword
// argument of a Django field: forms.NewCharField(forms.MaxLength(100),
// forms.Required(false)). An option that does not apply to a field is ignored.
type FieldOption func(*fieldOptions)

// collectOptions applies opts to an empty option set.
func collectOptions(opts []FieldOption) *fieldOptions {
	o := &fieldOptions{maxDigits: -1, decimalPlaces: -1}
	label := EmptyChoiceLabel
	o.emptyLabel = &label
	for _, opt := range opts {
		opt(o)
	}
	return o
}

// Required sets whether an empty value is rejected (required=).
func Required(required bool) FieldOption {
	return func(o *fieldOptions) { o.required = &required }
}

// Label sets the human-readable name of the field (label=).
func Label(label string) FieldOption {
	return func(o *fieldOptions) { o.label = &label }
}

// LabelSuffix overrides the form's label suffix for the field (label_suffix=).
func LabelSuffix(suffix string) FieldOption {
	return func(o *fieldOptions) { o.labelSuffix = &suffix }
}

// Initial sets the value shown in an unbound form (initial=). A func() any is
// called each time the value is needed.
func Initial(value any) FieldOption {
	return func(o *fieldOptions) { o.initial = value }
}

// UseWidget sets the widget that renders the field (widget=).
func UseWidget(w Widget) FieldOption {
	return func(o *fieldOptions) { o.widget = w }
}

// HelpText sets the descriptive text of the field (help_text=).
func HelpText(text string) FieldOption {
	return func(o *fieldOptions) { o.helpText = text }
}

// ErrorMessages overrides message templates by error code (error_messages=).
func ErrorMessages(messages map[string]string) FieldOption {
	return func(o *fieldOptions) {
		if o.errorMessages == nil {
			o.errorMessages = map[string]string{}
		}
		for code, msg := range messages {
			o.errorMessages[code] = msg
		}
	}
}

// Validators adds validators to the field (validators=).
func Validators(validators ...Validator) FieldOption {
	return func(o *fieldOptions) { o.validators = append(o.validators, validators...) }
}

// Disabled renders the field disabled and makes it ignore submitted data
// (disabled=True).
func Disabled() FieldOption {
	return func(o *fieldOptions) { o.disabled = true }
}

// MaxLength limits the length of a text value (max_length=).
func MaxLength(n int) FieldOption {
	return func(o *fieldOptions) { o.maxLength = n }
}

// MinLength sets the least length of a text value (min_length=).
func MinLength(n int) FieldOption {
	return func(o *fieldOptions) { o.minLength = n }
}

// Strip sets whether surrounding whitespace is removed from a text value
// (strip=); it is by default.
func Strip(strip bool) FieldOption {
	return func(o *fieldOptions) { o.strip = &strip }
}

// EmptyValue sets the cleaned value of an empty submission (empty_value=).
func EmptyValue(value any) FieldOption {
	return func(o *fieldOptions) { o.emptyValue, o.hasEmptyValue = value, true }
}

// MinValue sets the least accepted value of a numeric or temporal field
// (min_value=).
func MinValue(value any) FieldOption {
	return func(o *fieldOptions) { o.minValue = value }
}

// MaxValue sets the greatest accepted value of a numeric or temporal field
// (max_value=).
func MaxValue(value any) FieldOption {
	return func(o *fieldOptions) { o.maxValue = value }
}

// StepSize requires a number to be a multiple of step (step_size=).
func StepSize(step any) FieldOption {
	return func(o *fieldOptions) { o.stepSize = step }
}

// Choices sets the options of a choice field (choices=) from alternating
// values and labels: forms.Choices("s", "Small", "l", "Large").
func Choices(valuesAndLabels ...string) FieldOption {
	return func(o *fieldOptions) {
		o.choices = make([]Choice, 0, len(valuesAndLabels)/2)
		for i := 0; i+1 < len(valuesAndLabels); i += 2 {
			o.choices = append(o.choices, Choice{Value: valuesAndLabels[i], Label: valuesAndLabels[i+1]})
		}
	}
}

// ChoiceList sets the options of a choice field from a slice.
func ChoiceList(choices []Choice) FieldOption {
	return func(o *fieldOptions) { o.choices = append([]Choice(nil), choices...) }
}

// Coerce sets the function a TypedChoiceField converts a valid choice with
// (coerce=).
func Coerce(fn func(string) (any, error)) FieldOption {
	return func(o *fieldOptions) { o.coerce = fn }
}

// InputFormats sets the layouts a date or time field accepts (input_formats=).
// Each is a Go layout ("02.01.2006") or a strftime string ("%d.%m.%Y").
func InputFormats(formats ...string) FieldOption {
	return func(o *fieldOptions) { o.inputFormats = formats }
}

// InputDateFormats sets the layouts the date of a SplitDateTimeField accepts
// (input_date_formats=), as InputFormats does for a DateField.
func InputDateFormats(formats ...string) FieldOption {
	return func(o *fieldOptions) { o.dateFormats = formats }
}

// InputTimeFormats sets the layouts the time of a SplitDateTimeField accepts
// (input_time_formats=), as InputFormats does for a TimeField.
func InputTimeFormats(formats ...string) FieldOption {
	return func(o *fieldOptions) { o.timeFormats = formats }
}

// RequireAllFields sets whether a required MultiValueField needs a value for
// every subfield (require_all_fields=); it does by default.
func RequireAllFields(all bool) FieldOption {
	return func(o *fieldOptions) { o.requireAll = &all }
}

// MaxDigits limits the total number of digits of a decimal (max_digits=).
func MaxDigits(n int) FieldOption {
	return func(o *fieldOptions) { o.maxDigits = n }
}

// DecimalPlaces limits the number of decimal places of a decimal
// (decimal_places=).
func DecimalPlaces(n int) FieldOption {
	return func(o *fieldOptions) { o.decimalPlaces = n }
}

// EmptyLabel sets the label of the empty option of a model choice field
// (empty_label=).
func EmptyLabel(label string) FieldOption {
	return func(o *fieldOptions) { o.emptyLabel = &label }
}

// NoEmptyLabel removes the empty option of a model choice field
// (empty_label=None).
func NoEmptyLabel() FieldOption {
	return func(o *fieldOptions) { o.emptyLabel = nil }
}

// AllowUnicode lets a slug field accept Unicode letters (allow_unicode=True).
func AllowUnicode() FieldOption {
	return func(o *fieldOptions) { o.allowUnicode = true }
}

// AssumeScheme sets the scheme a URL field adds to a value without one
// (assume_scheme=); the default is "https".
func AssumeScheme(scheme string) FieldOption {
	return func(o *fieldOptions) { o.assumeScheme = scheme }
}

// Protocol restricts a GenericIPAddressField to "both", "ipv4", or "ipv6"
// (case-insensitive, protocol=); the default is "both".
func Protocol(protocol string) FieldOption {
	return func(o *fieldOptions) { o.protocol = protocol }
}

// UnpackIPv4 unpacks IPv4-mapped IPv6 addresses (::ffff:a.b.c.d) to plain IPv4
// strings on a GenericIPAddressField (unpack_ipv4=True). Valid only with
// protocol "both".
func UnpackIPv4() FieldOption {
	return func(o *fieldOptions) { o.unpackIPv4 = true }
}

// JSONEncoder sets a custom encoder for a JSONField (encoder=).
func JSONEncoder(fn func(any) ([]byte, error)) FieldOption {
	return func(o *fieldOptions) { o.jsonEncoder = fn }
}

// JSONDecoder sets a custom decoder for a JSONField (decoder=).
func JSONDecoder(fn func([]byte) (any, error)) FieldOption {
	return func(o *fieldOptions) { o.jsonDecoder = fn }
}
