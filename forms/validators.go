package forms

import (
	"math"
	"math/big"
	"regexp"

	"github.com/oliverhaas/djangogo/validators"
)

// Validator checks a value that a field has already converted to its Go type.
// It is aliased to validators.Validator so form and model validation share one
// validator type.
type Validator = validators.Validator

// RegexValidator returns a validator that requires the string form of the
// value to match re, Django's RegexValidator.
func RegexValidator(re *regexp.Regexp, message, code string) Validator {
	return validators.RegexValidator(re, message, code)
}

// InverseRegexValidator returns a validator that rejects a value whose string
// form matches re, Django's RegexValidator(inverse_match=True).
func InverseRegexValidator(re *regexp.Regexp, message, code string) Validator {
	return validators.InverseRegexValidator(re, message, code)
}

// ValidateSlug accepts ASCII letters, numbers, underscores and hyphens,
// Django's validate_slug.
var ValidateSlug = validators.ValidateSlug

// ValidateUnicodeSlug accepts Unicode letters, numbers, underscores and
// hyphens, Django's validate_unicode_slug.
var ValidateUnicodeSlug = validators.ValidateUnicodeSlug

// ValidateInteger accepts an optionally negative run of digits, Django's
// validate_integer.
var ValidateInteger = validators.ValidateInteger

// ValidateCommaSeparatedIntegerList accepts digits separated by commas,
// Django's validate_comma_separated_integer_list.
var ValidateCommaSeparatedIntegerList = validators.ValidateCommaSeparatedIntegerList

// ProhibitNullCharactersValidator rejects a string that contains a NUL
// character, Django's ProhibitNullCharactersValidator.
func ProhibitNullCharactersValidator(value any) error {
	return validators.ProhibitNullCharactersValidator(value)
}

// MaxLengthValidator rejects a value longer than limit, Django's
// MaxLengthValidator.
func MaxLengthValidator(limit int) Validator {
	return validators.MaxLengthValidator(limit)
}

// MinLengthValidator rejects a value shorter than limit, Django's
// MinLengthValidator.
func MinLengthValidator(limit int) Validator {
	return validators.MinLengthValidator(limit)
}

// MaxValueValidator rejects a value greater than limit, Django's
// MaxValueValidator.
func MaxValueValidator(limit any) Validator {
	return validators.MaxValueValidator(limit)
}

// MinValueValidator rejects a value less than limit, Django's
// MinValueValidator.
func MinValueValidator(limit any) Validator {
	return validators.MinValueValidator(limit)
}

// StepValueValidator rejects a number that is not a multiple of step, Django's
// StepValueValidator.
func StepValueValidator(step any, offset ...any) Validator {
	return validators.StepValueValidator(step, offset...)
}

// DecimalValidator limits the digits of a Decimal, Django's DecimalValidator.
func DecimalValidator(maxDigits, decimalPlaces int) Validator {
	return validators.DecimalValidator(maxDigits, decimalPlaces)
}

// compareValues orders two values of comparable types.
func compareValues(a, b any) (int, bool) {
	return validators.CompareValues(a, b)
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

// toRat converts a number to an exact rational.
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
