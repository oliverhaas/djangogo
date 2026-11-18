package forms

import (
	"math"
	"regexp"
	"strconv"
	"strings"
)

// CharField is a text field; its cleaned value is a string.
type CharField struct {
	BaseField
	// MaxLength and MinLength bound the length in characters; 0 means no bound.
	MaxLength, MinLength int
	// Strip removes surrounding whitespace before validation.
	Strip bool
	// EmptyValue is the cleaned value of an empty submission, "" by default.
	EmptyValue any
}

// NewCharField returns a CharField. It reads the options MaxLength, MinLength,
// Strip and EmptyValue beside the common ones.
func NewCharField(opts ...FieldOption) *CharField {
	f := &CharField{}
	f.setup(f, TextInput{}, nil, collectOptions(opts))
	return f
}

// setup initializes a CharField that is embedded in self.
func (f *CharField) setup(self Field, widget Widget, messages map[string]string, o *fieldOptions) {
	f.init(self, widget, messages, o)
	f.MaxLength, f.MinLength = o.maxLength, o.minLength
	f.Strip = o.strip == nil || *o.strip
	f.EmptyValue = ""
	if o.hasEmptyValue {
		f.EmptyValue = o.emptyValue
	}
	var own []Validator
	if f.MinLength > 0 {
		own = append(own, MinLengthValidator(f.MinLength))
	}
	if f.MaxLength > 0 {
		own = append(own, MaxLengthValidator(f.MaxLength))
	}
	own = append(own, ProhibitNullCharactersValidator)
	f.Validators = append(own, f.Validators...)
}

// ToGo returns the value as a string, stripped when Strip is set.
func (f *CharField) ToGo(value any) (any, error) {
	if isEmpty(value) {
		return f.EmptyValue, nil
	}
	s := pyString(value)
	if f.Strip {
		s = strings.TrimSpace(s)
	}
	if s == "" {
		return f.EmptyValue, nil
	}
	return s, nil
}

// WidgetAttrs adds maxlength and minlength to a visible widget.
func (f *CharField) WidgetAttrs(w Widget) Attrs {
	attrs := Attrs{}
	if w.IsHidden() {
		return attrs
	}
	if f.MaxLength > 0 {
		attrs["maxlength"] = strconv.Itoa(f.MaxLength)
	}
	if f.MinLength > 0 {
		attrs["minlength"] = strconv.Itoa(f.MinLength)
	}
	return attrs
}

// rangeValidators returns the validators for the min, max and step options.
func rangeValidators(o *fieldOptions) []Validator {
	var out []Validator
	if o.maxValue != nil {
		out = append(out, MaxValueValidator(o.maxValue))
	}
	if o.minValue != nil {
		out = append(out, MinValueValidator(o.minValue))
	}
	if o.stepSize != nil {
		out = append(out, StepValueValidator(o.stepSize, o.minValue))
	}
	return out
}

// numberAttrs returns the min, max and step attributes of a number input.
func numberAttrs(w Widget, minValue, maxValue, step any) Attrs {
	attrs := Attrs{}
	if _, ok := w.(NumberInput); !ok {
		return attrs
	}
	if minValue != nil {
		attrs["min"] = minValue
	}
	if maxValue != nil {
		attrs["max"] = maxValue
	}
	if step != nil {
		attrs["step"] = step
	}
	return attrs
}

// IntegerField is a whole-number field; its cleaned value is an int64, or nil
// when empty.
type IntegerField struct {
	BaseField
	// MinValue, MaxValue and StepSize are the limits given at construction,
	// kept for the widget attributes.
	MinValue, MaxValue, StepSize any
}

// trailingZerosRE matches a decimal point followed only by zeros, which an
// integer field tolerates ("3.0").
var trailingZerosRE = regexp.MustCompile(`\.0*\s*$`)

// NewIntegerField returns an IntegerField. It reads the options MinValue,
// MaxValue and StepSize beside the common ones.
func NewIntegerField(opts ...FieldOption) *IntegerField {
	o := collectOptions(opts)
	f := &IntegerField{MinValue: o.minValue, MaxValue: o.maxValue, StepSize: o.stepSize}
	f.init(f, NumberInput{}, map[string]string{"invalid": "Enter a whole number."}, o)
	f.Validators = append(rangeValidators(o), f.Validators...)
	return f
}

// ToGo parses a whole number.
func (f *IntegerField) ToGo(value any) (any, error) {
	if isEmpty(value) {
		return nil, nil
	}
	if i, ok := toInt(value); ok {
		return i, nil
	}
	i, ok := pyInt(trailingZerosRE.ReplaceAllString(pyString(value), ""))
	if !ok {
		return nil, f.Error("invalid")
	}
	return i, nil
}

// WidgetAttrs adds min, max and step to a number input.
func (f *IntegerField) WidgetAttrs(w Widget) Attrs {
	return numberAttrs(w, f.MinValue, f.MaxValue, f.StepSize)
}

// FloatField is a floating-point field; its cleaned value is a float64, or nil
// when empty.
type FloatField struct {
	BaseField
	// MinValue, MaxValue and StepSize are the limits given at construction.
	MinValue, MaxValue, StepSize any
}

// NewFloatField returns a FloatField. It reads the options MinValue, MaxValue
// and StepSize beside the common ones.
func NewFloatField(opts ...FieldOption) *FloatField {
	o := collectOptions(opts)
	f := &FloatField{MinValue: o.minValue, MaxValue: o.maxValue, StepSize: o.stepSize}
	f.init(f, NumberInput{}, map[string]string{"invalid": "Enter a number."}, o)
	f.Validators = append(rangeValidators(o), f.Validators...)
	return f
}

// ToGo parses a decimal floating-point number.
func (f *FloatField) ToGo(value any) (any, error) {
	if isEmpty(value) {
		return nil, nil
	}
	if n, ok := toFloat(value); ok {
		return n, nil
	}
	s := strings.TrimSpace(pyString(value))
	// ParseFloat also reads hexadecimal floats and digit separators, which a
	// form must not accept.
	if strings.ContainsAny(s, "xX_pP") {
		return nil, f.Error("invalid")
	}
	n, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil, f.Error("invalid")
	}
	return n, nil
}

// Validate rejects the infinities and not-a-number.
func (f *FloatField) Validate(value any) error {
	if err := f.BaseField.Validate(value); err != nil {
		return err
	}
	if n, ok := value.(float64); ok && (math.IsNaN(n) || math.IsInf(n, 0)) {
		return f.Error("invalid")
	}
	return nil
}

// WidgetAttrs adds min, max and step to a number input; without a step size
// the step is "any", so that the browser accepts fractions.
func (f *FloatField) WidgetAttrs(w Widget) Attrs {
	attrs := numberAttrs(w, f.MinValue, f.MaxValue, f.StepSize)
	if _, ok := w.(NumberInput); ok && f.StepSize == nil {
		if _, set := w.Attributes()["step"]; !set {
			attrs["step"] = "any"
		}
	}
	return attrs
}

// DecimalField is a fixed-point number field; its cleaned value is a Decimal,
// or nil when empty.
type DecimalField struct {
	BaseField
	// MinValue, MaxValue and StepSize are the limits given at construction.
	MinValue, MaxValue, StepSize any
	// MaxDigits and DecimalPlaces bound the digits; -1 means no bound.
	MaxDigits, DecimalPlaces int
}

// NewDecimalField returns a DecimalField. It reads the options MaxDigits,
// DecimalPlaces, MinValue, MaxValue and StepSize beside the common ones.
func NewDecimalField(opts ...FieldOption) *DecimalField {
	o := collectOptions(opts)
	f := &DecimalField{
		MinValue: o.minValue, MaxValue: o.maxValue, StepSize: o.stepSize,
		MaxDigits: o.maxDigits, DecimalPlaces: o.decimalPlaces,
	}
	f.init(f, NumberInput{}, map[string]string{"invalid": "Enter a number."}, o)
	own := rangeValidators(o)
	own = append(own, DecimalValidator(f.MaxDigits, f.DecimalPlaces))
	f.Validators = append(own, f.Validators...)
	return f
}

// ToGo parses a decimal number.
func (f *DecimalField) ToGo(value any) (any, error) {
	if isEmpty(value) {
		return nil, nil
	}
	if d, ok := value.(Decimal); ok {
		return d, nil
	}
	d, err := ParseDecimal(strings.TrimSpace(pyString(value)))
	if err != nil {
		return nil, f.Error("invalid")
	}
	return d, nil
}

// WidgetAttrs adds min, max and step to a number input; the step follows the
// number of decimal places.
func (f *DecimalField) WidgetAttrs(w Widget) Attrs {
	attrs := numberAttrs(w, f.MinValue, f.MaxValue, f.StepSize)
	if _, ok := w.(NumberInput); !ok || f.StepSize != nil {
		return attrs
	}
	if _, set := w.Attributes()["step"]; set {
		return attrs
	}
	attrs["step"] = "any"
	if f.DecimalPlaces >= 0 {
		attrs["step"] = strings.ToLower(Decimal{coef: "1", exp: -f.DecimalPlaces}.String())
	}
	return attrs
}

// BooleanField is a checkbox field; its cleaned value is a bool. A required
// BooleanField must be checked, as in Django.
type BooleanField struct {
	BaseField
}

// NewBooleanField returns a BooleanField.
func NewBooleanField(opts ...FieldOption) *BooleanField {
	f := &BooleanField{}
	f.init(f, CheckboxInput{}, nil, collectOptions(opts))
	return f
}

// ToGo returns false for nil, false, "", "false" and "0", and true otherwise.
// A hidden input or a JavaScript client may submit the false spellings.
func (f *BooleanField) ToGo(value any) (any, error) {
	switch v := value.(type) {
	case nil:
		return false, nil
	case bool:
		return v, nil
	case string:
		lower := strings.ToLower(v)
		return v != "" && lower != "false" && lower != "0", nil
	}
	return pyTruth(value), nil
}

// Validate rejects false for a required field.
func (f *BooleanField) Validate(value any) error {
	if b, _ := value.(bool); !b && f.Required {
		return f.Error("required")
	}
	return nil
}

// HasChanged compares the truth of both values.
func (f *BooleanField) HasChanged(initial, data any) bool {
	if f.Disabled {
		return false
	}
	a, _ := f.ToGo(initial)
	b, _ := f.ToGo(data)
	return a != b
}

// NullBooleanField is a three-state field; its cleaned value is true, false or
// nil. It never fails validation.
type NullBooleanField struct {
	BaseField
}

// NewNullBooleanField returns a NullBooleanField.
func NewNullBooleanField(opts ...FieldOption) *NullBooleanField {
	f := &NullBooleanField{}
	f.init(f, NullBooleanSelect{}, nil, collectOptions(opts))
	return f
}

// ToGo maps the true and false spellings to a bool and everything else to nil.
func (f *NullBooleanField) ToGo(value any) (any, error) {
	switch v := value.(type) {
	case bool:
		return v, nil
	case string:
		switch v {
		case "True", "true", "1":
			return true, nil
		case "False", "false", "0":
			return false, nil
		}
	}
	return nil, nil
}

// Validate accepts every value: nil is a legitimate answer.
func (f *NullBooleanField) Validate(any) error { return nil }

// EmailField is a CharField that requires an email address.
type EmailField struct {
	CharField
}

// NewEmailField returns an EmailField. The maximum length defaults to 320.
func NewEmailField(opts ...FieldOption) *EmailField {
	o := collectOptions(opts)
	if o.maxLength == 0 {
		o.maxLength = 320
	}
	strip := true
	o.strip = &strip
	f := &EmailField{}
	f.setup(f, EmailInput{}, nil, o)
	f.Validators = append([]Validator{ValidateEmail}, f.Validators...)
	return f
}

// URLField is a CharField that requires a URL. A value without a scheme gets
// the assumed scheme, "https" by default.
type URLField struct {
	CharField
	// AssumeScheme is the scheme added to a value without one.
	AssumeScheme string
}

// NewURLField returns a URLField. It reads the option AssumeScheme beside
// those of a CharField.
func NewURLField(opts ...FieldOption) *URLField {
	o := collectOptions(opts)
	strip := true
	o.strip = &strip
	f := &URLField{AssumeScheme: o.assumeScheme}
	if f.AssumeScheme == "" {
		f.AssumeScheme = "https"
	}
	f.setup(f, URLInput{}, map[string]string{"invalid": "Enter a valid URL."}, o)
	f.Validators = append([]Validator{ValidateURL}, f.Validators...)
	return f
}

// schemeRE matches the scheme at the start of a URL.
var schemeRE = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9+.-]*:`)

// ToGo adds the assumed scheme to a value that has none.
func (f *URLField) ToGo(value any) (any, error) {
	v, err := f.CharField.ToGo(value)
	s, ok := v.(string)
	if err != nil || !ok || s == "" {
		return v, err
	}
	if strings.Contains(s, "[") != strings.Contains(s, "]") {
		return nil, f.Error("invalid")
	}
	if !schemeRE.MatchString(s) || !strings.Contains(s, "://") && strings.Contains(strings.SplitN(s, ":", 2)[0], ".") {
		return f.AssumeScheme + "://" + strings.TrimPrefix(s, "//"), nil
	}
	return s, nil
}

// SlugField is a CharField that requires a slug.
type SlugField struct {
	CharField
}

// NewSlugField returns a SlugField. With the option AllowUnicode it accepts
// Unicode letters.
func NewSlugField(opts ...FieldOption) *SlugField {
	o := collectOptions(opts)
	f := &SlugField{}
	f.setup(f, TextInput{}, nil, o)
	validate := ValidateSlug
	if o.allowUnicode {
		validate = ValidateUnicodeSlug
	}
	f.Validators = append([]Validator{validate}, f.Validators...)
	return f
}

// RegexField is a CharField whose value must match a pattern.
type RegexField struct {
	CharField
}

// NewRegexField returns a RegexField for re. Override the message of the code
// "invalid" with ErrorMessages.
func NewRegexField(re *regexp.Regexp, opts ...FieldOption) *RegexField {
	o := collectOptions(opts)
	// Unlike CharField, RegexField keeps surrounding whitespace by default.
	if o.strip == nil {
		strip := false
		o.strip = &strip
	}
	f := &RegexField{}
	f.setup(f, TextInput{}, nil, o)
	f.Validators = append(f.Validators, RegexValidator(re, "", ""))
	return f
}

// UUIDField is a CharField that requires a UUID; its cleaned value is the UUID
// in its canonical form, lowercase with hyphens.
type UUIDField struct {
	CharField
}

// NewUUIDField returns a UUIDField.
func NewUUIDField(opts ...FieldOption) *UUIDField {
	f := &UUIDField{}
	f.setup(f, TextInput{}, map[string]string{"invalid": "Enter a valid UUID."}, collectOptions(opts))
	return f
}

// ToGo accepts the spellings Python's uuid.UUID accepts: 32 hexadecimal
// digits, with or without hyphens, braces or a urn:uuid: prefix.
func (f *UUIDField) ToGo(value any) (any, error) {
	v, err := f.CharField.ToGo(value)
	s, ok := v.(string)
	if err != nil || !ok {
		return v, err
	}
	if s == "" {
		return nil, nil
	}
	hex := strings.NewReplacer("urn:", "", "uuid:", "", "{", "", "}", "", "-", "").Replace(s)
	if len(hex) != 32 {
		return nil, f.Error("invalid")
	}
	hex = strings.ToLower(hex)
	for i := 0; i < len(hex); i++ {
		if c := hex[i]; (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return nil, f.Error("invalid")
		}
	}
	return hex[:8] + "-" + hex[8:12] + "-" + hex[12:16] + "-" + hex[16:20] + "-" + hex[20:], nil
}
