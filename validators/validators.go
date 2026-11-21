package validators

import (
	"math"
	"math/big"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

// Validator checks a value that a model or form field has already converted to
// its Go type. It returns nil for a valid value and a *ValidationError
// otherwise. The validators in this package carry the codes and messages of
// django.core.validators.
type Validator func(value any) error

// decimalLike is implemented by both orm.Decimal and forms.Decimal, allowing
// numeric and decimal validators to work on either type without an import
// cycle.
type decimalLike interface {
	DigitCounts() (digits, decimals int)
	Rat() *big.Rat
	Float64() float64
	String() string
}

// invalid returns the error most validators raise: the given message with the
// code "invalid" and the offending value as a parameter.
func invalid(message string, value any) *ValidationError {
	return NewValidationError(message, "invalid", map[string]any{"value": value})
}

// RegexValidator returns a validator that requires the string form of the
// value to match re, Django's RegexValidator. An empty message or code selects
// Django's defaults, "Enter a valid value." and "invalid".
func RegexValidator(re *regexp.Regexp, message, code string) Validator {
	return regexValidator(re, message, code, false)
}

// InverseRegexValidator returns a validator that rejects a value whose string
// form matches re, Django's RegexValidator(inverse_match=True).
func InverseRegexValidator(re *regexp.Regexp, message, code string) Validator {
	return regexValidator(re, message, code, true)
}

// regexValidator implements RegexValidator and its inverse.
func regexValidator(re *regexp.Regexp, message, code string, inverse bool) Validator {
	if message == "" {
		message = "Enter a valid value."
	}
	if code == "" {
		code = "invalid"
	}
	return func(value any) error {
		if re.MatchString(PyString(value)) == inverse {
			return NewValidationError(message, code, map[string]any{"value": value})
		}
		return nil
	}
}

// Patterns of the simple regex validators. Go's \d matches ASCII digits only,
// where Python's also matches other Unicode decimal digits.
var (
	slugRE        = regexp.MustCompile(`^[-a-zA-Z0-9_]+\z`)
	unicodeSlugRE = regexp.MustCompile(`^[-\p{L}\p{N}_]+\z`)
	integerRE     = regexp.MustCompile(`^-?\d+\z`)
)

// ValidateSlug accepts ASCII letters, numbers, underscores and hyphens,
// Django's validate_slug.
var ValidateSlug = RegexValidator(slugRE,
	"Enter a valid “slug” consisting of letters, numbers, underscores or hyphens.", "invalid")

// ValidateUnicodeSlug accepts Unicode letters, numbers, underscores and
// hyphens, Django's validate_unicode_slug.
var ValidateUnicodeSlug = RegexValidator(unicodeSlugRE,
	"Enter a valid “slug” consisting of Unicode letters, numbers, underscores, or hyphens.", "invalid")

// ValidateInteger accepts an optionally negative run of digits, Django's
// validate_integer.
var ValidateInteger = RegexValidator(integerRE, "Enter a valid integer.", "invalid")

// IntListValidator returns a validator that accepts integers separated by sep,
// Django's int_list_validator. When sep is empty, "," is used.
func IntListValidator(sep, message, code string, allowNegative bool) Validator {
	if sep == "" {
		sep = ","
	}
	neg := ""
	if allowNegative {
		neg = "(-)?"
	}
	escapedSep := regexp.QuoteMeta(sep)
	re := regexp.MustCompile(`^` + neg + `\d+(?:` + escapedSep + neg + `\d+)*\z`)
	return RegexValidator(re, message, code)
}

// ValidateCommaSeparatedIntegerList accepts digits separated by commas,
// Django's validate_comma_separated_integer_list.
var ValidateCommaSeparatedIntegerList = IntListValidator(",", "Enter only digits separated by commas.", "invalid", false)

// ProhibitNullCharactersValidator rejects a string that contains a NUL
// character, Django's ProhibitNullCharactersValidator.
func ProhibitNullCharactersValidator(value any) error {
	if strings.ContainsRune(PyString(value), 0) {
		return NewValidationError("Null characters are not allowed.", "null_characters_not_allowed",
			map[string]any{"value": value})
	}
	return nil
}

// valueLen returns the length a length validator measures: characters of a
// string, bytes of a []byte, elements of a slice.
func valueLen(v any) int {
	switch t := v.(type) {
	case string:
		return utf8.RuneCountInString(t)
	case []byte:
		return len(t)
	case []string:
		return len(t)
	case []any:
		return len(t)
	default:
		return utf8.RuneCountInString(PyString(v))
	}
}

// plural picks the singular or plural message as Django's ngettext does for
// English.
func plural(singular, pluralForm string, n int) string {
	if n == 1 {
		return singular
	}
	return pluralForm
}

// MaxLengthValidator rejects a value longer than limit, Django's
// MaxLengthValidator. An optional message overrides Django's default template.
func MaxLengthValidator(limit int, message ...string) Validator {
	custom := ""
	if len(message) > 0 {
		custom = message[0]
	}
	return func(value any) error {
		n := valueLen(value)
		if n <= limit {
			return nil
		}
		msg := custom
		if msg == "" {
			msg = plural(
				"Ensure this value has at most %(limit_value)d character (it has %(show_value)d).",
				"Ensure this value has at most %(limit_value)d characters (it has %(show_value)d).", limit)
		}
		return NewValidationError(msg, "max_length",
			map[string]any{"limit_value": limit, "show_value": n, "value": value})
	}
}

// MinLengthValidator rejects a value shorter than limit, Django's
// MinLengthValidator. An optional message overrides Django's default template.
func MinLengthValidator(limit int, message ...string) Validator {
	custom := ""
	if len(message) > 0 {
		custom = message[0]
	}
	return func(value any) error {
		n := valueLen(value)
		if n >= limit {
			return nil
		}
		msg := custom
		if msg == "" {
			msg = plural(
				"Ensure this value has at least %(limit_value)d character (it has %(show_value)d).",
				"Ensure this value has at least %(limit_value)d characters (it has %(show_value)d).", limit)
		}
		return NewValidationError(msg, "min_length",
			map[string]any{"limit_value": limit, "show_value": n, "value": value})
	}
}

// MaxValueValidator rejects a value greater than limit, Django's
// MaxValueValidator. Numbers, decimals, time.Time, time.Duration, strings, and
// types defining a Compare(T) int method (such as orm.Date and orm.Time) are
// comparable; a value that cannot be compared with limit passes. An optional
// message overrides Django's default template.
func MaxValueValidator(limit any, message ...string) Validator {
	msg := "Ensure this value is less than or equal to %(limit_value)s."
	if len(message) > 0 && message[0] != "" {
		msg = message[0]
	}
	return func(value any) error {
		if c, ok := CompareValues(value, limit); ok && c > 0 {
			return NewValidationError(msg, "max_value",
				map[string]any{"limit_value": limit, "show_value": value, "value": value})
		}
		return nil
	}
}

// MinValueValidator rejects a value less than limit, Django's
// MinValueValidator. An optional message overrides Django's default template.
func MinValueValidator(limit any, message ...string) Validator {
	msg := "Ensure this value is greater than or equal to %(limit_value)s."
	if len(message) > 0 && message[0] != "" {
		msg = message[0]
	}
	return func(value any) error {
		if c, ok := CompareValues(value, limit); ok && c < 0 {
			return NewValidationError(msg, "min_value",
				map[string]any{"limit_value": limit, "show_value": value, "value": value})
		}
		return nil
	}
}

// StepValueValidator rejects a number that is not a multiple of step, Django's
// StepValueValidator. An optional offset moves the origin of the multiples.
func StepValueValidator(step any, offset ...any) Validator {
	return func(value any) error {
		if isStepMultiple(value, step, offset...) {
			return nil
		}
		params := map[string]any{"limit_value": step, "show_value": value, "value": value}
		if len(offset) == 0 || offset[0] == nil {
			return NewValidationError("Ensure this value is a multiple of step size %(limit_value)s.",
				"step_size", params)
		}
		params["offset"] = offset[0]
		params["valid_value1"] = addNumbers(offset[0], step, 1)
		params["valid_value2"] = addNumbers(offset[0], step, 2)
		return NewValidationError("Ensure this value is a multiple of step size %(limit_value)s, "+
			"starting from %(offset)s, e.g. %(offset)s, %(valid_value1)s, %(valid_value2)s, and so on.",
			"step_size", params)
	}
}

// isStepMultiple reports whether value - offset is a multiple of step as
// Django's StepValueValidator decides it: the difference is computed in the
// value's type, then math.remainder of the floats must be within 1e-9 of 0. A
// value or step that is not a number passes.
func isStepMultiple(value, step any, offset ...any) bool {
	var off any = int64(0)
	if len(offset) > 0 && offset[0] != nil {
		off = offset[0]
	}
	var diff float64
	_, floatValue := value.(float64)
	_, floatOffset := off.(float64)
	if rv, ok := toRat(value); ok && !floatValue && !floatOffset {
		ro, ok := toRat(off)
		if !ok {
			return true
		}
		diff, _ = new(big.Rat).Sub(rv, ro).Float64()
	} else {
		v, ok1 := toFloat(value)
		o, ok2 := toFloat(off)
		if !ok1 || !ok2 {
			return true
		}
		diff = v - o
	}
	s, ok := toFloat(step)
	if !ok || s == 0 {
		return true
	}
	return math.Abs(math.Remainder(diff, s)) <= 1e-9
}

// addNumbers returns offset + n*step: an integer when both are integers, a
// decimal string with the larger number of decimal places when either is a
// decimalLike value (as Python's decimal arithmetic gives), and a float
// otherwise.
func addNumbers(offset, step any, n int64) any {
	oi, ok1 := toInt(offset)
	si, ok2 := toInt(step)
	if ok1 && ok2 {
		return oi + n*si
	}
	_, decOffset := offset.(decimalLike)
	_, decStep := step.(decimalLike)
	if decOffset || decStep {
		ro, okO := toRat(offset)
		rs, okS := toRat(step)
		if okO && okS {
			sum := new(big.Rat).Add(ro, new(big.Rat).Mul(rs, new(big.Rat).SetInt64(n)))
			return sum.FloatString(max(fractionDigits(offset), fractionDigits(step)))
		}
	}
	of, _ := toFloat(offset)
	sf, _ := toFloat(step)
	return of + float64(n)*sf
}

// fractionDigits returns the number of decimal places of a decimalLike value,
// 0 for any other value.
func fractionDigits(v any) int {
	if d, ok := v.(decimalLike); ok {
		_, dec := d.DigitCounts()
		return dec
	}
	return 0
}

// toInt converts a Go integer of any width to int64.
func toInt(v any) (int64, bool) {
	switch t := v.(type) {
	case int:
		return int64(t), true
	case int8:
		return int64(t), true
	case int16:
		return int64(t), true
	case int32:
		return int64(t), true
	case int64:
		return t, true
	case uint:
		if uint64(t) > math.MaxInt64 {
			return 0, false
		}
		return int64(t), true
	case uint8:
		return int64(t), true
	case uint16:
		return int64(t), true
	case uint32:
		return int64(t), true
	case uint64:
		if t > math.MaxInt64 {
			return 0, false
		}
		return int64(t), true
	}
	return 0, false
}

// toFloat converts a number (integer, float or decimalLike) to float64.
func toFloat(v any) (float64, bool) {
	if i, ok := toInt(v); ok {
		return float64(i), true
	}
	switch t := v.(type) {
	case float64:
		return t, true
	case float32:
		return float64(t), true
	case decimalLike:
		return t.Float64(), true
	}
	return 0, false
}

// toRat converts a number to an exact rational, so that a decimal compares
// without a detour through binary floating point.
func toRat(v any) (*big.Rat, bool) {
	if i, ok := toInt(v); ok {
		return new(big.Rat).SetInt64(i), true
	}
	switch t := v.(type) {
	case decimalLike:
		return t.Rat(), true
	case float64:
		if math.IsNaN(t) || math.IsInf(t, 0) {
			return nil, false
		}
		return new(big.Rat).SetFloat64(t), true
	case float32:
		return toRat(float64(t))
	}
	return nil, false
}

// CompareValues orders two values of comparable types: numbers of any mix,
// times, durations, strings, or equal concrete types implementing
// Compare(T) int. The second result is false when the pair has no order.
func CompareValues(a, b any) (int, bool) {
	if a == nil || b == nil {
		return 0, false
	}
	switch x := a.(type) {
	case time.Time:
		if y, ok := b.(time.Time); ok {
			return x.Compare(y), true
		}
		return 0, false
	case time.Duration:
		if y, ok := b.(time.Duration); ok {
			return compareOrdered(x, y), true
		}
		return 0, false
	case string:
		if y, ok := b.(string); ok {
			return strings.Compare(x, y), true
		}
		return 0, false
	}
	if x, ok := toInt(a); ok {
		if y, ok := toInt(b); ok {
			return compareOrdered(x, y), true
		}
	}
	xf, xIsFloat := a.(float64)
	yf, yIsFloat := b.(float64)
	if (xIsFloat && math.IsInf(xf, 0)) || (yIsFloat && math.IsInf(yf, 0)) {
		x, ok1 := toFloat(a)
		y, ok2 := toFloat(b)
		return compareOrdered(x, y), ok1 && ok2
	}
	x, ok1 := toRat(a)
	y, ok2 := toRat(b)
	if ok1 && ok2 {
		return x.Cmp(y), true
	}
	// Fall back to a Compare(T) int method on matching concrete types (e.g.
	// orm.Date, orm.Time, netip.Addr).
	ra, rb := reflect.ValueOf(a), reflect.ValueOf(b)
	if ra.Type() == rb.Type() {
		if m := ra.MethodByName("Compare"); m.IsValid() {
			mt := m.Type()
			if mt.NumIn() == 1 && mt.In(0) == ra.Type() && mt.NumOut() == 1 && mt.Out(0).Kind() == reflect.Int {
				return int(m.Call([]reflect.Value{rb})[0].Int()), true
			}
		}
	}
	return 0, false
}

// compareOrdered is cmp.Compare for the ordered types used here, without
// NaN rules.
func compareOrdered[T int64 | float64 | time.Duration](x, y T) int {
	switch {
	case x < y:
		return -1
	case x > y:
		return 1
	default:
		return 0
	}
}

// DecimalValidator limits the digits of a decimal value (orm.Decimal or
// forms.Decimal), Django's DecimalValidator. A negative maxDigits or
// decimalPlaces disables that limit.
func DecimalValidator(maxDigits, decimalPlaces int) Validator {
	return func(value any) error {
		d, ok := value.(decimalLike)
		if !ok {
			return nil
		}
		digits, decimals := d.DigitCounts()
		whole := digits - decimals
		if maxDigits >= 0 && digits > maxDigits {
			return NewValidationError(plural(
				"Ensure that there are no more than %(max)s digit in total.",
				"Ensure that there are no more than %(max)s digits in total.", maxDigits),
				"max_digits", map[string]any{"max": maxDigits, "value": value})
		}
		if decimalPlaces >= 0 && decimals > decimalPlaces {
			return NewValidationError(plural(
				"Ensure that there are no more than %(max)s decimal place.",
				"Ensure that there are no more than %(max)s decimal places.", decimalPlaces),
				"max_decimal_places", map[string]any{"max": decimalPlaces, "value": value})
		}
		if maxDigits >= 0 && decimalPlaces >= 0 && whole > maxDigits-decimalPlaces {
			limit := maxDigits - decimalPlaces
			return NewValidationError(plural(
				"Ensure that there are no more than %(max)s digit before the decimal point.",
				"Ensure that there are no more than %(max)s digits before the decimal point.", limit),
				"max_whole_digits", map[string]any{"max": limit, "value": value})
		}
		return nil
	}
}

// FileExtensionValidator checks that a file name has one of the allowed
// extensions (case-insensitive, without leading dots), Django's
// FileExtensionValidator.
func FileExtensionValidator(allowedExtensions []string, message, code string) Validator {
	if message == "" {
		message = "File extension “%(extension)s” is not allowed. Allowed extensions are: %(allowed_extensions)s."
	}
	if code == "" {
		code = "invalid_extension"
	}
	allowed := make([]string, len(allowedExtensions))
	allowedSet := make(map[string]bool, len(allowedExtensions))
	for i, ext := range allowedExtensions {
		lower := strings.ToLower(strings.TrimPrefix(ext, "."))
		allowed[i] = lower
		allowedSet[lower] = true
	}
	return func(value any) error {
		if allowedExtensions == nil {
			return nil
		}
		name := PyString(value)
		ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(name), "."))
		if !allowedSet[ext] {
			return NewValidationError(message, code, map[string]any{
				"extension":          ext,
				"allowed_extensions": strings.Join(allowed, ", "),
				"value":              value,
			})
		}
		return nil
	}
}
