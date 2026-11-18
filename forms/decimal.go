package forms

import (
	"errors"
	"math/big"
	"strconv"
	"strings"
)

// errDecimalSyntax reports a string that is not a decimal number.
var errDecimalSyntax = errors.New("forms: invalid decimal syntax")

// Decimal is a decimal number that keeps the digits it was written with, the
// value a DecimalField produces. It mirrors the parts of Python's
// decimal.Decimal that a form needs: "1.50" stays "1.50" and compares equal to
// "1.5". The zero value is the number 0.
type Decimal struct {
	neg  bool
	coef string // digits without leading zeros, "" meaning "0"
	exp  int
}

// ParseDecimal parses a decimal number: an optional sign, digits with an
// optional fraction, and an optional exponent ("-12.50", ".5", "1E+3"). Not a
// number and the infinities are rejected.
func ParseDecimal(s string) (Decimal, error) {
	var d Decimal
	rest := s
	switch {
	case strings.HasPrefix(rest, "-"):
		d.neg = true
		rest = rest[1:]
	case strings.HasPrefix(rest, "+"):
		rest = rest[1:]
	}
	mantissa := rest
	if i := strings.IndexAny(rest, "eE"); i >= 0 {
		mantissa = rest[:i]
		exp, err := parseExponent(rest[i+1:])
		if err != nil {
			return Decimal{}, err
		}
		d.exp = exp
	}
	intPart, fracPart, _ := strings.Cut(mantissa, ".")
	if intPart == "" && fracPart == "" {
		return Decimal{}, errDecimalSyntax
	}
	digits := intPart + fracPart
	for i := 0; i < len(digits); i++ {
		if digits[i] < '0' || digits[i] > '9' {
			return Decimal{}, errDecimalSyntax
		}
	}
	d.exp -= len(fracPart)
	d.coef = strings.TrimLeft(digits, "0")
	return d, nil
}

// maxDecimalExponent bounds the exponent of a parsed decimal. The exact
// arithmetic of Rat costs memory in proportion to the exponent, so an
// unbounded one would let a short input such as "1E999999999" exhaust it.
const maxDecimalExponent = 2000

// parseExponent parses the exponent of a decimal: an optional sign and digits.
func parseExponent(s string) (int, error) {
	unsigned := s
	if unsigned != "" && (unsigned[0] == '+' || unsigned[0] == '-') {
		unsigned = unsigned[1:]
	}
	if unsigned == "" || strings.Trim(unsigned, "0123456789") != "" {
		return 0, errDecimalSyntax
	}
	exp, err := strconv.Atoi(s)
	if err != nil || exp > maxDecimalExponent || exp < -maxDecimalExponent {
		return 0, errDecimalSyntax
	}
	return exp, nil
}

// MustDecimal is ParseDecimal for a constant known to be valid, such as a
// limit in a field definition; it returns 0 for an invalid string.
func MustDecimal(s string) Decimal {
	d, err := ParseDecimal(s)
	if err != nil {
		return Decimal{}
	}
	return d
}

// digits returns the coefficient digits, "0" for zero.
func (d Decimal) digits() string {
	if d.coef == "" {
		return "0"
	}
	return d.coef
}

// IsZero reports whether d is 0.
func (d Decimal) IsZero() bool { return d.coef == "" }

// String formats d as Python's str(Decimal) does: plain notation where it is
// exact, scientific notation for a positive exponent or a very small number.
func (d Decimal) String() string {
	digits := d.digits()
	left := d.exp + len(digits)
	dot := 1
	if d.exp <= 0 && left > -6 {
		dot = left
	}
	var b strings.Builder
	if d.neg {
		b.WriteByte('-')
	}
	switch {
	case dot <= 0:
		b.WriteString("0." + strings.Repeat("0", -dot) + digits)
	case dot >= len(digits):
		b.WriteString(digits + strings.Repeat("0", dot-len(digits)))
	default:
		b.WriteString(digits[:dot] + "." + digits[dot:])
	}
	if left != dot {
		b.WriteByte('E')
		if left > dot {
			b.WriteByte('+')
		}
		b.WriteString(strconv.Itoa(left - dot))
	}
	return b.String()
}

// Rat returns the exact value of d as a rational number.
func (d Decimal) Rat() *big.Rat {
	coef, _ := new(big.Int).SetString(d.digits(), 10)
	if d.neg {
		coef.Neg(coef)
	}
	r := new(big.Rat).SetInt(coef)
	if d.exp == 0 {
		return r
	}
	exp := d.exp
	if exp < 0 {
		exp = -exp
	}
	pow := new(big.Rat).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(exp)), nil))
	if d.exp > 0 {
		return r.Mul(r, pow)
	}
	return r.Quo(r, pow)
}

// Cmp compares the numeric values of d and other: -1, 0 or +1.
func (d Decimal) Cmp(other Decimal) int { return d.Rat().Cmp(other.Rat()) }

// Float64 returns the nearest float64 to d.
func (d Decimal) Float64() float64 {
	f, _ := d.Rat().Float64()
	return f
}

// digitCounts returns the total number of digits and the number of decimal
// places, counted as Django's DecimalValidator counts them.
func (d Decimal) digitCounts() (digits, decimals int) {
	n := len(d.digits())
	if d.exp >= 0 {
		if !d.IsZero() {
			n += d.exp // a positive exponent adds that many trailing zeros
		}
		return n, 0
	}
	if -d.exp > n {
		// The fraction has leading zeros: every digit is a decimal place.
		return -d.exp, -d.exp
	}
	return n, -d.exp
}
