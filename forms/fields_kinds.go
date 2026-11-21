package forms

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/netip"
	"reflect"
	"sort"
	"strings"

	"github.com/oliverhaas/djangogo/validators"
)

// MaxIPv6AddressLength is the default maximum character length of a
// GenericIPAddressField, matching Django's MAX_IPV6_ADDRESS_LENGTH.
const MaxIPv6AddressLength = 39

// GenericIPAddressField is a CharField that accepts an IPv4 address, an IPv6
// address, or both, normalizing IPv6 values to compressed lowercase form.
type GenericIPAddressField struct {
	CharField
	// Protocol is "both", "ipv4", or "ipv6" (lowercased).
	Protocol string
	// UnpackIPv4 converts IPv4-mapped IPv6 addresses to IPv4 strings.
	UnpackIPv4 bool
}

// NewGenericIPAddressField returns a GenericIPAddressField. It reads the
// options Protocol, UnpackIPv4, and the options of CharField (with MaxLength
// defaulting to MaxIPv6AddressLength). It panics if Protocol is unknown or
// UnpackIPv4 is combined with a protocol other than "both", matching Django's
// ValueError.
func NewGenericIPAddressField(opts ...FieldOption) *GenericIPAddressField {
	o := collectOptions(opts)
	protocol := o.protocol
	if protocol == "" {
		protocol = "both"
	}
	ipValidators, _, err := validators.IPAddressValidators(protocol, o.unpackIPv4)
	if err != nil {
		panic(err)
	}
	if o.maxLength == 0 {
		o.maxLength = MaxIPv6AddressLength
	}
	f := &GenericIPAddressField{
		Protocol:   strings.ToLower(protocol),
		UnpackIPv4: o.unpackIPv4,
	}
	f.setup(f, TextInput{}, nil, o)
	f.Validators = append(ipValidators, f.Validators...)
	return f
}

// PrepareValue formats a netip.Addr for display, turning an invalid zero Addr
// into an empty string.
func (f *GenericIPAddressField) PrepareValue(value any) any {
	if addr, ok := value.(netip.Addr); ok {
		if !addr.IsValid() {
			return ""
		}
		return addr.String()
	}
	return value
}

// ToGo strips whitespace and normalizes IPv6 addresses as Django's
// GenericIPAddressField.to_python does.
func (f *GenericIPAddressField) ToGo(value any) (any, error) {
	if addr, ok := value.(netip.Addr); ok {
		if !addr.IsValid() {
			return f.EmptyValue, nil
		}
		value = addr.String()
	}
	if isEmpty(value) {
		return f.EmptyValue, nil
	}
	s := strings.TrimSpace(pyString(value))
	if s == "" {
		return f.EmptyValue, nil
	}
	if strings.Contains(s, ":") {
		return cleanIPv6Address(s, f.UnpackIPv4, f.MaxLength)
	}
	return s, nil
}

// cleanIPv6Address validates and compresses an IPv6 address string, matching
// Django's django.utils.ipv6.clean_ipv6_address.
func cleanIPv6Address(ipStr string, unpackIPv4 bool, maxLength int) (string, error) {
	if maxLength > 0 && len(ipStr) > maxLength {
		return "", NewValidationError("This is not a valid IPv6 address.", "invalid",
			map[string]any{"protocol": "IPv6"})
	}
	addr, err := netip.ParseAddr(ipStr)
	if err != nil || addr.Zone() != "" || !addr.Is6() {
		return "", NewValidationError("This is not a valid IPv6 address.", "invalid",
			map[string]any{"protocol": "IPv6"})
	}
	if addr.Is4In6() {
		if unpackIPv4 {
			return addr.Unmap().String(), nil
		}
		return "::ffff:" + addr.Unmap().String(), nil
	}
	return addr.String(), nil
}

// InvalidJSONInput marks raw text from a bound form that failed JSON decoding
// so PrepareValue renders it back without re-quoting, matching Django's
// InvalidJSONInput.
type InvalidJSONInput string

// JSONString marks a JSON string value already decoded by JSONField.ToGo so
// re-cleaning the value returns it unchanged instead of parsing it again,
// matching Django's JSONString.
type JSONString string

// JSONField is a CharField that parses JSON text into Go values and formats
// them back for display in a Textarea, matching Django's forms.JSONField.
type JSONField struct {
	CharField
	// Encoder overrides default JSON serialization for PrepareValue and HasChanged.
	Encoder func(any) ([]byte, error)
	// Decoder overrides default JSON deserialization for ToGo and BoundData.
	Decoder func([]byte) (any, error)
}

// NewJSONField returns a JSONField with Textarea as its default widget. It
// reads JSONEncoder, JSONDecoder, and the options of CharField.
func NewJSONField(opts ...FieldOption) *JSONField {
	o := collectOptions(opts)
	f := &JSONField{
		Encoder: o.jsonEncoder,
		Decoder: o.jsonDecoder,
	}
	f.setup(f, Textarea{}, map[string]string{"invalid": "Enter a valid JSON."}, o)
	return f
}

// decodeJSON parses raw JSON bytes using the custom decoder or encoding/json.
func (f *JSONField) decodeJSON(raw []byte) (any, error) {
	if f.Decoder != nil {
		return f.Decoder(raw)
	}
	var out any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// encodeJSON serializes value for display or change comparison.
func (f *JSONField) encodeJSON(value any) (string, error) {
	if raw, ok := value.(json.RawMessage); ok {
		if len(raw) == 0 {
			value = nil
		} else if decoded, err := f.decodeJSON(raw); err == nil {
			value = decoded
		} else {
			return string(raw), nil
		}
	}
	if js, ok := value.(JSONString); ok {
		value = string(js)
	}
	if f.Encoder != nil {
		b, err := f.Encoder(value)
		if err != nil {
			return "", err
		}
		return string(b), nil
	}
	var b strings.Builder
	if err := writePythonJSON(&b, value); err != nil {
		return "", err
	}
	return b.String(), nil
}

// ToGo decodes a JSON string into its Go value, returning nil for empty input
// and wrapping decoded strings in JSONString so calling Clean on a cleaned
// value is idempotent.
func (f *JSONField) ToGo(value any) (any, error) {
	if f.Disabled {
		return value, nil
	}
	if value == nil {
		return nil, nil
	}
	switch v := value.(type) {
	case string:
		if v == "" {
			return nil, nil
		}
		converted, err := f.decodeJSON([]byte(v))
		if err != nil {
			return nil, f.Error("invalid", map[string]any{"value": value})
		}
		if s, ok := converted.(string); ok {
			return JSONString(s), nil
		}
		return converted, nil
	case json.RawMessage:
		if len(v) == 0 {
			return nil, nil
		}
		converted, err := f.decodeJSON(v)
		if err != nil {
			return nil, f.Error("invalid", map[string]any{"value": string(v)})
		}
		if s, ok := converted.(string); ok {
			return JSONString(s), nil
		}
		return converted, nil
	case JSONString, bool,
		int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64,
		float32, float64:
		return value, nil
	default:
		rv := reflect.ValueOf(value)
		switch rv.Kind() {
		case reflect.Slice, reflect.Array, reflect.Map, reflect.Struct:
			return value, nil
		}
	}
	s := pyString(value)
	converted, err := f.decodeJSON([]byte(s))
	if err != nil {
		return nil, f.Error("invalid", map[string]any{"value": value})
	}
	if str, ok := converted.(string); ok {
		return JSONString(str), nil
	}
	return converted, nil
}

// Validate rejects empty JSON values (nil, "", [], {}) when the field is
// required, matching Django's Field.empty_values on JSONField.
func (f *JSONField) Validate(value any) error {
	if f.Required && isEmptyJSON(value) {
		return f.Error("required")
	}
	return nil
}

// RunValidators runs validators only when value is not an empty JSON value.
func (f *JSONField) RunValidators(value any) error {
	if isEmptyJSON(value) {
		return nil
	}
	return f.BaseField.RunValidators(value)
}

// isEmptyJSON reports whether v is one of Django's EMPTY_VALUES for JSONField.
func isEmptyJSON(v any) bool {
	if isEmpty(v) {
		return true
	}
	switch t := v.(type) {
	case map[string]any:
		return len(t) == 0
	case JSONString:
		return t == ""
	}
	return false
}

// BoundData decodes valid JSON for redisplay and wraps invalid text in
// InvalidJSONInput so PrepareValue does not overquote it.
func (f *JSONField) BoundData(data, initial any) any {
	if f.Disabled {
		return initial
	}
	if data == nil {
		return nil
	}
	if s, ok := data.(string); ok {
		converted, err := f.decodeJSON([]byte(s))
		if err != nil {
			return InvalidJSONInput(s)
		}
		return converted
	}
	return data
}

// PrepareValue formats value as JSON with Python-compatible spacing and
// unescaped UTF-8, leaving InvalidJSONInput untouched.
func (f *JSONField) PrepareValue(value any) any {
	if inv, ok := value.(InvalidJSONInput); ok {
		return string(inv)
	}
	encoded, err := f.encodeJSON(value)
	if err != nil {
		return value
	}
	return encoded
}

// HasChanged compares the canonical sorted-key JSON representations of initial
// and data so key order is ignored while types (such as true vs 1) stay distinct.
func (f *JSONField) HasChanged(initial, data any) bool {
	if f.Disabled {
		return false
	}
	converted, err := f.ToGo(data)
	if err != nil {
		return true
	}
	a, errA := f.encodeJSON(initial)
	b, errB := f.encodeJSON(converted)
	if errA != nil || errB != nil {
		return true
	}
	return a != b
}

// writePythonJSON writes v as JSON with Python's default separators (", ", ": "),
// sorted object keys, and unescaped UTF-8 (ensure_ascii=False, no HTML escapes).
func writePythonJSON(b *strings.Builder, v any) error {
	switch x := v.(type) {
	case nil:
		b.WriteString("null")
		return nil
	case string:
		return writeJSONString(b, x)
	case JSONString:
		return writeJSONString(b, string(x))
	case json.RawMessage:
		if len(x) == 0 {
			b.WriteString("null")
			return nil
		}
		var decoded any
		if err := json.Unmarshal(x, &decoded); err != nil {
			return err
		}
		return writePythonJSON(b, decoded)
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Pointer, reflect.Interface:
		if rv.IsNil() {
			b.WriteString("null")
			return nil
		}
		return writePythonJSON(b, rv.Elem().Interface())
	case reflect.Slice, reflect.Array:
		if rv.Kind() == reflect.Slice && rv.IsNil() {
			b.WriteString("null")
			return nil
		}
		b.WriteByte('[')
		for i := range rv.Len() {
			if i > 0 {
				b.WriteString(", ")
			}
			if err := writePythonJSON(b, rv.Index(i).Interface()); err != nil {
				return err
			}
		}
		b.WriteByte(']')
		return nil
	case reflect.Map:
		if rv.IsNil() {
			b.WriteString("null")
			return nil
		}
		if rv.Type().Key().Kind() != reflect.String {
			return fmt.Errorf("forms: JSON map key of type %s", rv.Type().Key())
		}
		keys := make([]string, 0, rv.Len())
		for _, k := range rv.MapKeys() {
			keys = append(keys, k.String())
		}
		sort.Strings(keys)
		b.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				b.WriteString(", ")
			}
			if err := writeJSONString(b, k); err != nil {
				return err
			}
			b.WriteString(": ")
			if err := writePythonJSON(b, rv.MapIndex(reflect.ValueOf(k).Convert(rv.Type().Key())).Interface()); err != nil {
				return err
			}
		}
		b.WriteByte('}')
		return nil
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return err
	}
	b.WriteString(strings.TrimSuffix(buf.String(), "\n"))
	return nil
}

// writeJSONString encodes s as a JSON string with UTF-8 preserved and no HTML
// escaping.
func writeJSONString(b *strings.Builder, s string) error {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(s); err != nil {
		return err
	}
	b.WriteString(strings.TrimSuffix(buf.String(), "\n"))
	return nil
}
