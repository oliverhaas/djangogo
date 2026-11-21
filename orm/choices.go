package orm

import (
	"fmt"
	"maps"
	"math"
	"net/netip"
	"reflect"
	"slices"
	"strconv"
	"time"
)

// BlankChoiceDash is Django's BLANK_CHOICE_DASH label ("---------") prepended
// by Field.GetChoices(true) when no blank choice is already declared.
const BlankChoiceDash = "---------"

// ChoiceOf constructs a Choice with a typed value and display label, the Go
// analog of one (value, label) pair in Django's choices=.
func ChoiceOf(value any, label string) Choice {
	return Choice{
		Value: formatChoiceString(value),
		Label: label,
		Raw:   value,
	}
}

// ChoiceGroup constructs a named optgroup of choices, the Go analog of
// ("Group Name", [(value, label), ...]) in Django's choices=.
func ChoiceGroup(label string, choices ...Choice) Choice {
	return Choice{
		Label: label,
		Group: slices.Clone(choices),
	}
}

// EffectiveChoices returns the field's declared choices, invoking ChoicesFunc
// when set, or nil when choices were not declared.
func (f *Field) EffectiveChoices() []Choice {
	if f.ChoicesFunc != nil {
		return f.ChoicesFunc()
	}
	return f.Choices
}

// FlatChoices returns all leaf (value, label) choices in declaration order,
// expanding any optgroups. It is the Go analog of Django's Field.flatchoices.
func (f *Field) FlatChoices() []Choice {
	raw := f.EffectiveChoices()
	if len(raw) == 0 {
		return nil
	}
	return flattenChoices(raw)
}

// flattenChoices recursively expands optgroups in choices into flat entries.
func flattenChoices(choices []Choice) []Choice {
	out := make([]Choice, 0, len(choices))
	for _, ch := range choices {
		if len(ch.Group) > 0 {
			out = append(out, flattenChoices(ch.Group)...)
		} else {
			out = append(out, ch)
		}
	}
	return out
}

// GetChoices returns the field's choices for forms and widgets, optionally
// prepending BlankChoiceDash when includeBlank is true and no blank choice
// (empty string or nil) is already present. It is the Go analog of Django's
// Field.get_choices(include_blank=...).
func (f *Field) GetChoices(includeBlank bool) []Choice {
	choices := f.EffectiveChoices()
	if !includeBlank {
		return slices.Clone(choices)
	}
	for _, ch := range flattenChoices(choices) {
		if ch.Value == "" && (ch.Raw == nil || ch.Raw == "") {
			return slices.Clone(choices)
		}
	}
	out := make([]Choice, 0, len(choices)+1)
	out = append(out, Choice{Value: "", Label: BlankChoiceDash})
	out = append(out, choices...)
	return out
}

// DisplayValue returns the human-readable choice label matching value, or the
// unwrapped value itself when no choice matches (and nil for a nil value or nil
// pointer). It mirrors Django's Model._get_FIELD_display.
func (f *Field) DisplayValue(value any) any {
	unwrapped, isNull := unwrapChoicePointer(value)
	if isNull {
		return nil
	}
	for _, ch := range f.FlatChoices() {
		if choiceMatches(f, ch, unwrapped) {
			return ch.Label
		}
	}
	return unwrapped
}

// FieldDisplay returns the display value for fieldName on instance, matching
// Django's instance.get_<field>_display(). If instance defines an explicit
// Get<FieldName>Display() method, that override is called; otherwise the
// field's choices are consulted.
func (m *Model) FieldDisplay(instance any, fieldName string) any {
	return GetDisplay(m, instance, fieldName)
}

// GetDisplay returns the display value for fieldName on instance, the Go
// equivalent of Django's get_FOO_display(). fieldName may be the Go struct
// field name, attribute name, or column name. If instance defines a custom
// Get<StructFieldName>Display() method, that method is called instead.
func GetDisplay(m *Model, instance any, fieldName string) any {
	if m == nil || instance == nil {
		return nil
	}
	f, ok := resolveMetaField(fieldName, m.byName, m.byAttr, m.byColumn)
	if !ok {
		return nil
	}
	// Honor an explicit Get<FieldName>Display() override on the struct type.
	rv := reflect.ValueOf(instance)
	if method := rv.MethodByName("Get" + f.Name + "Display"); method.IsValid() {
		mt := method.Type()
		if mt.NumIn() == 0 && mt.NumOut() == 1 {
			return method.Call(nil)[0].Interface()
		}
	}
	elem := rv
	for elem.Kind() == reflect.Pointer {
		if elem.IsNil() {
			return nil
		}
		elem = elem.Elem()
	}
	if elem.Kind() != reflect.Struct || elem.Type() != m.goType {
		return nil
	}
	return f.DisplayValue(elem.Field(f.Index).Interface())
}

// applyWithChoices applies choices from the legacy Choices() hook during model
// registration. Any non-relation field kind is allowed.
func applyWithChoices(modelName string, byName, byAttr, byColumn map[string]*Field, model any) error {
	wc, ok := model.(withChoices)
	if !ok {
		return nil
	}
	for fieldName, choices := range wc.Choices() {
		f, found := resolveMetaField(fieldName, byName, byAttr, byColumn)
		if !found {
			return fmt.Errorf("orm: model %s: Choices() names unknown field %q", modelName, fieldName)
		}
		if f.Rel != nil {
			return fmt.Errorf("orm: model %s: choices on relation field %q are not supported", modelName, fieldName)
		}
		if choices != nil {
			f.HasChoices = true
			f.Choices = cloneChoices(choices)
		}
	}
	return nil
}

// applyMetaFieldHooks applies Meta.Choices, Meta.ChoicesFunc, Meta.Validators,
// and Meta.ErrorMessages to the model's fields during registration.
func applyMetaFieldHooks(m *Model, meta Meta) error {
	for fieldName, choices := range meta.Choices {
		f, found := resolveMetaField(fieldName, m.byName, m.byAttr, m.byColumn)
		if !found {
			return fmt.Errorf("orm: model %s: Meta.Choices names unknown field %q", m.name, fieldName)
		}
		if f.Rel != nil {
			return fmt.Errorf("orm: model %s: Meta.Choices on relation field %q is not supported", m.name, fieldName)
		}
		if choices != nil {
			f.HasChoices = true
			f.Choices = cloneChoices(choices)
		}
	}
	for fieldName, fn := range meta.ChoicesFunc {
		f, found := resolveMetaField(fieldName, m.byName, m.byAttr, m.byColumn)
		if !found {
			return fmt.Errorf("orm: model %s: Meta.ChoicesFunc names unknown field %q", m.name, fieldName)
		}
		if f.Rel != nil {
			return fmt.Errorf("orm: model %s: Meta.ChoicesFunc on relation field %q is not supported", m.name, fieldName)
		}
		if fn != nil {
			if f.HasChoices {
				return fmt.Errorf("orm: model %s: field %q cannot set both Choices and ChoicesFunc", m.name, f.Name)
			}
			f.HasChoices = true
			f.ChoicesFunc = fn
		}
	}
	for fieldName, vs := range meta.Validators {
		f, found := resolveMetaField(fieldName, m.byName, m.byAttr, m.byColumn)
		if !found {
			return fmt.Errorf("orm: model %s: Meta.Validators names unknown field %q", m.name, fieldName)
		}
		f.Validators = append(f.Validators, vs...)
	}
	for fieldName, msgs := range meta.ErrorMessages {
		f, found := resolveMetaField(fieldName, m.byName, m.byAttr, m.byColumn)
		if !found {
			return fmt.Errorf("orm: model %s: Meta.ErrorMessages names unknown field %q", m.name, fieldName)
		}
		if len(msgs) > 0 {
			if f.ErrorMessages == nil {
				f.ErrorMessages = make(map[string]string, len(msgs))
			}
			maps.Copy(f.ErrorMessages, msgs)
		}
	}
	return nil
}

// cloneChoices returns a deep copy of choices so mutating caller slices cannot
// alter the frozen Model descriptor.
func cloneChoices(choices []Choice) []Choice {
	if choices == nil {
		return nil
	}
	out := make([]Choice, len(choices))
	for i, ch := range choices {
		out[i] = ch
		if ch.Group != nil {
			out[i].Group = cloneChoices(ch.Group)
		}
	}
	return out
}

// unwrapChoicePointer dereferences a single pointer layer for choice matching
// and display, reporting whether the value is nil.
func unwrapChoicePointer(v any) (any, bool) {
	if v == nil {
		return nil, true
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return nil, true
		}
		return rv.Elem().Interface(), false
	}
	return v, false
}

// formatChoiceString converts a typed choice value to its string form for
// Choice.Value (used by forms, templates, and system checks).
func formatChoiceString(v any) string {
	v, isNull := unwrapChoicePointer(v)
	if isNull {
		return ""
	}
	switch x := v.(type) {
	case string:
		return x
	case bool:
		return strconv.FormatBool(x)
	case int:
		return strconv.Itoa(x)
	case int8:
		return strconv.FormatInt(int64(x), 10)
	case int16:
		return strconv.FormatInt(int64(x), 10)
	case int32:
		return strconv.FormatInt(int64(x), 10)
	case int64:
		return strconv.FormatInt(x, 10)
	case uint:
		return strconv.FormatUint(uint64(x), 10)
	case uint8:
		return strconv.FormatUint(uint64(x), 10)
	case uint16:
		return strconv.FormatUint(uint64(x), 10)
	case uint32:
		return strconv.FormatUint(uint64(x), 10)
	case uint64:
		return strconv.FormatUint(x, 10)
	case float32:
		return strconv.FormatFloat(float64(x), 'g', -1, 32)
	case float64:
		return strconv.FormatFloat(x, 'g', -1, 64)
	case fmt.Stringer:
		return x.String()
	default:
		return fmt.Sprint(x)
	}
}

// choiceMatches reports whether val equals choice ch under the semantics of
// field f (numeric equality for ints/floats/Decimals, instant equality for
// time.Time, and canonical string fallback when ch.Raw is nil).
func choiceMatches(f *Field, ch Choice, val any) bool {
	if ch.Raw != nil {
		return valuesEqualForChoice(ch.Raw, val)
	}
	// Plain Choice{Value: ..., Label: ...} without Raw: compare against Value.
	switch x := val.(type) {
	case string:
		return x == ch.Value
	case Decimal:
		if parsed, err := ParseDecimal(ch.Value); err == nil {
			return x.Equal(parsed)
		}
		return x.String() == ch.Value
	case UUID:
		if parsed, err := ParseUUID(ch.Value); err == nil {
			return x == parsed
		}
		return x.String() == ch.Value
	case Date:
		if parsed, err := ParseDate(ch.Value); err == nil {
			return x == parsed
		}
		return x.String() == ch.Value
	case Time:
		if parsed, err := ParseTime(ch.Value); err == nil {
			return x == parsed
		}
		return x.String() == ch.Value
	case netip.Addr:
		if parsed, err := netip.ParseAddr(ch.Value); err == nil {
			return x == parsed
		}
		return x.String() == ch.Value
	}
	if f != nil && isIntegerKind(f.Kind) {
		if n, ok := asInt64(val); ok {
			if parsed, err := strconv.ParseInt(ch.Value, 10, 64); err == nil {
				return n == parsed
			}
		}
	}
	return formatChoiceString(val) == ch.Value
}

// valuesEqualForChoice compares two typed choice values for equality.
func valuesEqualForChoice(a, b any) bool {
	a, aNull := unwrapChoicePointer(a)
	b, bNull := unwrapChoicePointer(b)
	if aNull || bNull {
		return aNull && bNull
	}
	if ai, ok1 := asInt64(a); ok1 {
		if bi, ok2 := asInt64(b); ok2 {
			return ai == bi
		}
	}
	switch x := a.(type) {
	case string:
		y, ok := b.(string)
		return ok && x == y
	case bool:
		y, ok := b.(bool)
		return ok && x == y
	case float64:
		if y, ok := asFloat64(b); ok {
			return x == y
		}
		return false
	case float32:
		if y, ok := asFloat64(b); ok {
			return float64(x) == y
		}
		return false
	case Decimal:
		y, ok := b.(Decimal)
		return ok && x.Equal(y)
	case Date:
		y, ok := b.(Date)
		return ok && x == y
	case Time:
		y, ok := b.(Time)
		return ok && x == y
	case time.Time:
		y, ok := b.(time.Time)
		return ok && x.Equal(y)
	case time.Duration:
		y, ok := b.(time.Duration)
		return ok && x == y
	case UUID:
		y, ok := b.(UUID)
		return ok && x == y
	case netip.Addr:
		y, ok := b.(netip.Addr)
		return ok && x == y
	default:
		return reflect.DeepEqual(a, b)
	}
}

// asInt64 converts any signed or unsigned integer (within int64 range) to int64.
func asInt64(v any) (int64, bool) {
	switch x := v.(type) {
	case int:
		return int64(x), true
	case int8:
		return int64(x), true
	case int16:
		return int64(x), true
	case int32:
		return int64(x), true
	case int64:
		return x, true
	case uint:
		if uint64(x) > math.MaxInt64 {
			return 0, false
		}
		return int64(x), true
	case uint8:
		return int64(x), true
	case uint16:
		return int64(x), true
	case uint32:
		return int64(x), true
	case uint64:
		if x > math.MaxInt64 {
			return 0, false
		}
		return int64(x), true
	default:
		return 0, false
	}
}

// asFloat64 converts a float32 or float64 to float64.
func asFloat64(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case float32:
		return float64(x), true
	default:
		return 0, false
	}
}
