package forms

import (
	"math"
	"math/big"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

// Validator checks a value that a field has already converted to its Go type.
// It returns nil for a valid value and a *ValidationError otherwise. The
// validators of this file carry the codes and messages of
// django.core.validators.
type Validator func(value any) error

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
		if re.MatchString(pyString(value)) == inverse {
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
	intListRE     = regexp.MustCompile(`^\d+(?:,\d+)*\z`)
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

// ValidateCommaSeparatedIntegerList accepts digits separated by commas,
// Django's validate_comma_separated_integer_list.
var ValidateCommaSeparatedIntegerList = RegexValidator(intListRE, "Enter only digits separated by commas.", "invalid")

// ProhibitNullCharactersValidator rejects a string that contains a NUL
// character, Django's ProhibitNullCharactersValidator.
func ProhibitNullCharactersValidator(value any) error {
	if strings.ContainsRune(pyString(value), 0) {
		return NewValidationError("Null characters are not allowed.", "null_characters_not_allowed",
			map[string]any{"value": value})
	}
	return nil
}

// valueLen returns the length a length validator measures: characters of a
// string, elements of a list.
func valueLen(v any) int {
	switch t := v.(type) {
	case string:
		return utf8.RuneCountInString(t)
	case []string:
		return len(t)
	case []any:
		return len(t)
	default:
		return utf8.RuneCountInString(pyString(v))
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
// MaxLengthValidator.
func MaxLengthValidator(limit int) Validator {
	return func(value any) error {
		n := valueLen(value)
		if n <= limit {
			return nil
		}
		return NewValidationError(plural(
			"Ensure this value has at most %(limit_value)d character (it has %(show_value)d).",
			"Ensure this value has at most %(limit_value)d characters (it has %(show_value)d).", limit),
			"max_length", map[string]any{"limit_value": limit, "show_value": n, "value": value})
	}
}

// MinLengthValidator rejects a value shorter than limit, Django's
// MinLengthValidator.
func MinLengthValidator(limit int) Validator {
	return func(value any) error {
		n := valueLen(value)
		if n >= limit {
			return nil
		}
		return NewValidationError(plural(
			"Ensure this value has at least %(limit_value)d character (it has %(show_value)d).",
			"Ensure this value has at least %(limit_value)d characters (it has %(show_value)d).", limit),
			"min_length", map[string]any{"limit_value": limit, "show_value": n, "value": value})
	}
}

// MaxValueValidator rejects a value greater than limit, Django's
// MaxValueValidator. Numbers, Decimal, time.Time, time.Duration and strings
// are comparable; a value that cannot be compared with limit passes.
func MaxValueValidator(limit any) Validator {
	return func(value any) error {
		if c, ok := compareValues(value, limit); ok && c > 0 {
			return NewValidationError("Ensure this value is less than or equal to %(limit_value)s.",
				"max_value", map[string]any{"limit_value": limit, "show_value": value, "value": value})
		}
		return nil
	}
}

// MinValueValidator rejects a value less than limit, Django's
// MinValueValidator.
func MinValueValidator(limit any) Validator {
	return func(value any) error {
		if c, ok := compareValues(value, limit); ok && c < 0 {
			return NewValidationError("Ensure this value is greater than or equal to %(limit_value)s.",
				"min_value", map[string]any{"limit_value": limit, "show_value": value, "value": value})
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
// Decimal with the larger number of decimal places when either is a Decimal,
// as Python's decimal arithmetic gives, and a float otherwise.
func addNumbers(offset, step any, n int64) any {
	oi, ok1 := toInt(offset)
	si, ok2 := toInt(step)
	if ok1 && ok2 {
		return oi + n*si
	}
	_, decOffset := offset.(Decimal)
	_, decStep := step.(Decimal)
	if decOffset || decStep {
		ro, okO := toRat(offset)
		rs, okS := toRat(step)
		if okO && okS {
			sum := new(big.Rat).Add(ro, new(big.Rat).Mul(rs, new(big.Rat).SetInt64(n)))
			if d, err := ParseDecimal(sum.FloatString(max(fractionDigits(offset), fractionDigits(step)))); err == nil {
				return d
			}
		}
	}
	of, _ := toFloat(offset)
	sf, _ := toFloat(step)
	return of + float64(n)*sf
}

// fractionDigits returns the number of decimal places of a Decimal, 0 for any
// other value.
func fractionDigits(v any) int {
	if d, ok := v.(Decimal); ok && d.exp < 0 {
		return -d.exp
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

// toFloat converts a number (integer, float or Decimal) to float64.
func toFloat(v any) (float64, bool) {
	if i, ok := toInt(v); ok {
		return float64(i), true
	}
	switch t := v.(type) {
	case float64:
		return t, true
	case float32:
		return float64(t), true
	case Decimal:
		return t.Float64(), true
	}
	return 0, false
}

// toRat converts a number to an exact rational, so that a Decimal compares
// without a detour through binary floating point.
func toRat(v any) (*big.Rat, bool) {
	if i, ok := toInt(v); ok {
		return new(big.Rat).SetInt64(i), true
	}
	switch t := v.(type) {
	case Decimal:
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

// compareValues orders two values of comparable types: numbers of any mix,
// times, durations and strings. The second result is false when the pair has
// no order.
func compareValues(a, b any) (int, bool) {
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
	if !ok1 || !ok2 {
		return 0, false
	}
	return x.Cmp(y), true
}

// compareOrdered is cmp.Compare for the few ordered types used here, without
// its NaN rules.
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

// DecimalValidator limits the digits of a Decimal, Django's DecimalValidator.
// A negative maxDigits or decimalPlaces disables that limit.
func DecimalValidator(maxDigits, decimalPlaces int) Validator {
	return func(value any) error {
		d, ok := value.(Decimal)
		if !ok {
			return nil
		}
		digits, decimals := d.digitCounts()
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
