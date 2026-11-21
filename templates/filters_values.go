package templates

import (
	"math"
	"math/big"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"github.com/flosch/pongo2/v6"

	"github.com/oliverhaas/djangogo/orm"
)

// asInt converts a template value like Python's int(): an integer, a float
// (truncated), a boolean or a string that holds an integer.
func asInt(v *pongo2.Value) (int, bool) {
	switch {
	case v.IsInteger():
		return v.Integer(), true
	case v.IsFloat():
		f := v.Float()
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return 0, false
		}
		return int(f), true
	case v.IsBool():
		if v.Bool() {
			return 1, true
		}
		return 0, true
	case v.IsString():
		n, err := strconv.Atoi(strings.TrimSpace(v.String()))
		return n, err == nil
	}
	return 0, false
}

// filterAdd implements Django's add: the sum when both sides are integers
// (or strings holding integers), otherwise two strings or two lists joined.
// Sides that do not fit together give an empty string. pongo2's filter
// appends the argument as text in that case.
func filterAdd(in, param *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	if a, ok := asInt(in); ok {
		if b, ok := asInt(param); ok {
			return pongo2.AsValue(a + b), nil
		}
	}
	if in.IsString() && param.IsString() {
		return str(in.String() + param.String())
	}
	a, b := reflect.ValueOf(in.Interface()), reflect.ValueOf(param.Interface())
	if isList(a) && isList(b) {
		out := make([]any, 0, a.Len()+b.Len())
		for _, list := range []reflect.Value{a, b} {
			for i := range list.Len() {
				out = append(out, list.Index(i).Interface())
			}
		}
		return pongo2.AsValue(out), nil
	}
	return str("")
}

// filterGetDigit implements Django's get_digit: the digit of a whole number
// at the given position counted from the right, starting at 1. A position
// past the first digit gives 0; input that is not a whole number, or a
// position below 1, gives the value back.
func filterGetDigit(in, param *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	pos, okPos := asInt(param)
	value, okValue := asInt(in)
	if !okPos || !okValue || pos < 1 {
		return in, nil
	}
	digits := strconv.Itoa(value)
	if pos > len(digits) || digits[len(digits)-pos] == '-' {
		return pongo2.AsValue(0), nil
	}
	return pongo2.AsValue(int(digits[len(digits)-pos] - '0')), nil
}

// filterYesno implements Django's yesno: one of the comma-separated words of
// the argument ("yes,no,maybe" by default) for a true, a false and a nil
// value. With two words nil takes the word for false.
func filterYesno(in, param *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	yes, no, maybe := "yes", "no", "maybe"
	if !param.IsNil() {
		bits := strings.Split(param.String(), ",")
		if len(bits) < 2 {
			return in, nil
		}
		yes, no, maybe = bits[0], bits[1], bits[1]
		if len(bits) == 3 {
			maybe = bits[2]
		}
	}
	switch {
	case in.IsNil():
		return str(maybe)
	case in.IsTrue():
		return str(yes)
	}
	return str(no)
}

// decimalRe matches a number written with plain decimal digits.
var decimalRe = regexp.MustCompile(`^[+-]?(\d+\.?\d*|\.\d+)$`)

// decimalText returns a number in plain decimal digits: an integer, a float
// in its shortest exact form (Python's repr) or a numeric string.
func decimalText(v *pongo2.Value) (string, bool) {
	switch d := v.Interface().(type) {
	case orm.Decimal:
		return d.FixedString(), true
	case *orm.Decimal:
		if d != nil {
			return d.FixedString(), true
		}
		return "", false
	}
	switch {
	case v.IsInteger():
		return strconv.Itoa(v.Integer()), true
	case v.IsString():
		s := strings.TrimSpace(v.String())
		if decimalRe.MatchString(s) {
			return s, true
		}
		f, err := strconv.ParseFloat(s, 64)
		if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
			return "", false
		}
		return strconv.FormatFloat(f, 'f', -1, 64), true
	case v.IsFloat():
		f := v.Float()
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return "", false
		}
		return strconv.FormatFloat(f, 'f', -1, 64), true
	}
	return "", false
}

// nonFiniteRe matches the infinities and NaNs Python's Decimal reads.
var nonFiniteRe = regexp.MustCompile(`(?i)^\s*[+-]?(inf|infinity|s?nan[0-9]*)\s*$`)

// nonFinite returns the text of v when it is an infinity or NaN, a float or
// a string Python's Decimal reads as one.
func nonFinite(v *pongo2.Value) (string, bool) {
	switch {
	case v.IsFloat():
		switch f := v.Float(); {
		case math.IsNaN(f):
			return "nan", true
		case math.IsInf(f, 1):
			return "inf", true
		case math.IsInf(f, -1):
			return "-inf", true
		}
	case v.IsString() && nonFiniteRe.MatchString(v.String()):
		return v.String(), true
	}
	return "", false
}

// groupThousands inserts a comma between every three digits of a whole
// number given in decimal digits.
func groupThousands(whole string) string {
	var b strings.Builder
	for i, d := range whole {
		if i > 0 && (len(whole)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(d)
	}
	return b.String()
}

// filterFloatformat implements Django's floatformat. Without an argument a
// number is rounded to one decimal place, which is dropped when it is zero.
// A positive argument is the number of places to show, a negative one the
// number to show unless they are all zero. A trailing "g" groups the
// thousands ("2g").
//
// The number is rounded half up on its decimal digits, as Django does with
// Decimal: 2.675 gives 2.68, where pongo2's filter, working on the binary
// float, gives 2.67.
func filterFloatformat(in, param *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	if s, ok := nonFinite(in); ok {
		// Django gives an infinity or NaN back as Python prints it.
		return str(s)
	}
	digits, ok := decimalText(in)
	if !ok {
		return str("")
	}
	places, group := -1, false
	if !param.IsNil() {
		arg := strings.TrimSpace(param.String())
		for strings.HasSuffix(arg, "g") || strings.HasSuffix(arg, "u") {
			group = group || strings.HasSuffix(arg, "g")
			arg = arg[:len(arg)-1]
		}
		if arg != "" {
			n, err := strconv.Atoi(arg)
			if err != nil {
				// Django gives the number back for an argument it cannot read.
				return str(digits)
			}
			places = n
		}
	}
	negative := strings.HasPrefix(digits, "-")
	whole, frac, _ := strings.Cut(strings.TrimLeft(digits, "+-"), ".")
	shown := max(places, -places)
	if strings.Trim(frac, "0") == "" && places <= 0 {
		// A whole number is printed without decimal places.
		shown, frac = 0, ""
	}
	for len(frac) <= shown {
		frac += "0"
	}
	num, ok := new(big.Int).SetString("0"+whole+frac[:shown], 10)
	if !ok {
		return str("")
	}
	if frac[shown] >= '5' {
		num.Add(num, big.NewInt(1))
	}
	out := num.String()
	for len(out) <= shown {
		out = "0" + out
	}
	whole, frac = out[:len(out)-shown], out[len(out)-shown:]
	if group {
		whole = groupThousands(whole)
	}
	if frac != "" {
		whole += "." + frac
	}
	if negative && num.Sign() != 0 {
		whole = "-" + whole
	}
	return str(whole)
}
