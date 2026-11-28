package forms

import (
	"fmt"
	"math/big"
	"reflect"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/oliverhaas/djangogo/internal/duration"
)

// The functions below reproduce Python's conversions of values to text,
// integers and truth values, so that cleaned values and messages match
// Django's.

// pyString renders v the way Python's str() renders the corresponding value,
// so that messages and widget values match Django's output.
func pyString(v any) string {
	switch t := v.(type) {
	case nil:
		return "None"
	case string:
		return t
	case bool:
		if t {
			return "True"
		}
		return "False"
	case float64:
		return formatFloat(t)
	case float32:
		return formatFloat(float64(t))
	case time.Time:
		return formatDateTime(t)
	case time.Duration:
		return duration.String(t)
	case fmt.Stringer:
		return t.String()
	}
	if rv := reflect.ValueOf(v); rv.Kind() == reflect.Slice && rv.Type().Elem().Kind() != reflect.Uint8 {
		items := make([]string, rv.Len())
		for i := range items {
			items[i] = pyRepr(rv.Index(i).Interface())
		}
		return "[" + strings.Join(items, ", ") + "]"
	}
	return fmt.Sprint(v)
}

// pyRepr renders v the way Python's repr() renders the corresponding value: a
// string in single quotes, everything else as pyString.
func pyRepr(v any) string {
	if s, ok := v.(string); ok {
		return "'" + strings.ReplaceAll(strings.ReplaceAll(s, `\`, `\\`), "'", `\'`) + "'"
	}
	return pyString(v)
}

// formatFloat renders a float as Python's str(float) does: the shortest
// representation, always with a fractional part ("1.0").
func formatFloat(f float64) string {
	s := strconv.FormatFloat(f, 'g', -1, 64)
	if strings.ContainsAny(s, ".eEnN") { // "NaN" and "+Inf" stay as they are
		return s
	}
	return s + ".0"
}

// normalizePyInt validates Python int syntax (surrounding whitespace, optional
// sign, Unicode decimal digits, single underscores between digits) and returns
// its ASCII decimal digit string.
func normalizePyInt(s string) (string, bool) {
	s = strings.TrimSpace(s)
	var b strings.Builder
	prevDigit := false
	for i, r := range s {
		switch {
		case i == 0 && (r == '+' || r == '-'):
			b.WriteRune(r)
		case r == '_' && prevDigit:
			prevDigit = false
			continue
		case unicode.Is(unicode.Nd, r):
			b.WriteByte(byte('0' + digitValue(r)))
			prevDigit = true
			continue
		default:
			return "", false
		}
	}
	if !prevDigit {
		return "", false
	}
	return b.String(), true
}

// pyInt parses s as Python's int(s) does: surrounding whitespace, an optional
// sign, and decimal digits of any script (fullwidth "５０" is 50), single
// underscores allowed between digits.
func pyInt(s string) (int64, bool) {
	norm, ok := normalizePyInt(s)
	if !ok {
		return 0, false
	}
	i, err := strconv.ParseInt(norm, 10, 64)
	return i, err == nil
}

// pyBigInt parses s as an arbitrary-precision Python integer.
func pyBigInt(s string) (*big.Int, bool) {
	norm, ok := normalizePyInt(s)
	if !ok {
		return nil, false
	}
	n, ok := new(big.Int).SetString(norm, 10)
	return n, ok
}

// digitValue returns the value of a Unicode decimal digit. The digits of every
// script are encoded as runs of ten consecutive code points starting at zero.
func digitValue(r rune) int {
	for _, rng := range unicode.Nd.R16 {
		if lo, hi := rune(rng.Lo), rune(rng.Hi); r >= lo && r <= hi {
			return int(r-lo) % 10
		}
	}
	for _, rng := range unicode.Nd.R32 {
		if lo, hi := rune(rng.Lo), rune(rng.Hi); r >= lo && r <= hi {
			return int(r-lo) % 10
		}
	}
	return 0
}

// pyTruth reports the truth of v as Python's bool() does: zero numbers and
// empty slices and maps are false, everything else is true.
func pyTruth(v any) bool {
	if r, ok := toRat(v); ok {
		return r.Sign() != 0
	}
	if f, ok := toFloat(v); ok {
		return f != 0
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Slice, reflect.Map, reflect.String:
		return rv.Len() > 0
	case reflect.Pointer, reflect.Interface:
		return !rv.IsNil()
	}
	return true
}
