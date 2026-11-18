package urls

import (
	"fmt"
	"math"
	"reflect"
	"sort"
	"strconv"
	"strings"
)

// The helpers in this file print Go values the way Python prints the matching
// values. Reverse needs them twice: URL text is built with str(), and Django's
// NoReverseMatch messages embed the repr() of the arguments, which ported
// tests and people who know Django expect to read unchanged.

// pyStr returns the text Python's str() gives for the matching value.
func pyStr(v any) string {
	switch x := v.(type) {
	case nil:
		return "None"
	case string:
		return x
	case fmt.Stringer:
		return x.String()
	case bool:
		if x {
			return "True"
		}
		return "False"
	case float64:
		return pyFloat(x)
	case float32:
		return pyFloat(float64(x))
	case []byte:
		return string(x)
	}
	return fmt.Sprint(v)
}

// pyFloat formats a float as Python's repr does: the shortest text that
// round-trips, always with a decimal point or an exponent.
func pyFloat(f float64) string {
	switch {
	case math.IsInf(f, 1):
		return "inf"
	case math.IsInf(f, -1):
		return "-inf"
	case math.IsNaN(f):
		return "nan"
	}
	if abs := math.Abs(f); abs != 0 && (abs >= 1e16 || abs < 1e-4) {
		return strconv.FormatFloat(f, 'e', -1, 64)
	}
	s := strconv.FormatFloat(f, 'f', -1, 64)
	if !strings.Contains(s, ".") {
		s += ".0"
	}
	return s
}

// pyRepr returns the text Python's repr() gives for the matching value.
func pyRepr(v any) string {
	switch x := v.(type) {
	case nil:
		return "None"
	case string:
		return pyQuote(x)
	case bool, float64, float32:
		return pyStr(x)
	case Kwargs:
		return pyDict(x)
	case map[string]any:
		return pyDict(x)
	case []any:
		return pyList(x)
	case []string:
		items := make([]any, len(x))
		for i, s := range x {
			items[i] = s
		}
		return pyList(items)
	case fmt.Stringer:
		return pyQuote(x.String())
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Slice || rv.Kind() == reflect.Array {
		items := make([]any, rv.Len())
		for i := range items {
			items[i] = rv.Index(i).Interface()
		}
		return pyList(items)
	}
	return fmt.Sprint(v)
}

// pyQuote returns a Python string literal for s: single quotes, unless s holds
// a single quote and no double quote.
func pyQuote(s string) string {
	quote := byte('\'')
	if strings.Contains(s, "'") && !strings.Contains(s, `"`) {
		quote = '"'
	}
	var b strings.Builder
	b.WriteByte(quote)
	for _, r := range s {
		switch {
		case r == rune(quote) || r == '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\r':
			b.WriteString(`\r`)
		case r == '\t':
			b.WriteString(`\t`)
		case r < 0x20 || r == 0x7f:
			fmt.Fprintf(&b, `\x%02x`, r)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte(quote)
	return b.String()
}

// pyList returns the repr of a Python list holding items.
func pyList(items []any) string {
	parts := make([]string, len(items))
	for i, it := range items {
		parts[i] = pyRepr(it)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

// pyTuple returns the repr of a Python tuple holding items, including the
// trailing comma of a one-element tuple.
func pyTuple(items []any) string {
	parts := make([]string, len(items))
	for i, it := range items {
		parts[i] = pyRepr(it)
	}
	if len(parts) == 1 {
		return "(" + parts[0] + ",)"
	}
	return "(" + strings.Join(parts, ", ") + ")"
}

// pyDict returns the repr of a Python dict. Go maps have no insertion order,
// so the keys are sorted to keep messages stable.
func pyDict(m map[string]any) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = pyQuote(k) + ": " + pyRepr(m[k])
	}
	return "{" + strings.Join(parts, ", ") + "}"
}

// looseEqual reports whether a value passed to Reverse equals a default
// keyword argument, the way Python's == does: numbers compare by value across
// integer and float types, everything else by deep equality.
func looseEqual(a, b any) bool {
	if fa, ok := toFloat(a); ok {
		if fb, ok := toFloat(b); ok {
			return fa == fb
		}
		return false
	}
	return reflect.DeepEqual(a, b)
}

// toFloat converts a numeric value to float64. Booleans count as numbers, as
// they do in Python (True == 1).
func toFloat(v any) (float64, bool) {
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Bool:
		if rv.Bool() {
			return 1, true
		}
		return 0, true
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return float64(rv.Int()), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return float64(rv.Uint()), true
	case reflect.Float32, reflect.Float64:
		return rv.Float(), true
	default:
		return 0, false
	}
}
