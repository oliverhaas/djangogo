package orm

import (
	"database/sql/driver"
	"errors"
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"
)

// errDecimalSyntax reports text that cannot be parsed as a finite decimal number.
var errDecimalSyntax = errors.New("orm: invalid decimal syntax")

// maxDecimalExponent bounds parsed exponents so rational arithmetic cannot
// exhaust memory on a short input such as "1E999999999".
const maxDecimalExponent = 2000

// Decimal is an exact base-10 fixed-point number that preserves both precision
// and trailing fractional zeros (scale), the Go counterpart of Python's
// decimal.Decimal for KindDecimal model fields.
//
// Decimal is a small comparable value type using only the standard library
// (no external dependency in go.mod). The zero value represents 0. Use Equal
// for numeric equality ("1.5" == "1.50") and ExactEqual for total equality
// including scale ("8.320" != "8.32").
type Decimal struct {
	neg  bool
	coef string // significant digits without leading zeros; "" represents 0
	exp  int    // base-10 exponent
}

// ParseDecimal parses a finite decimal string such as "-12.50", ".5", or "1E+3".
// Surrounding whitespace is trimmed. NaN and infinities are rejected.
func ParseDecimal(s string) (Decimal, error) {
	rest := strings.TrimSpace(s)
	if rest == "" {
		return Decimal{}, errDecimalSyntax
	}
	var d Decimal
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
		exp, err := parseDecimalExponent(rest[i+1:])
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
	for i := range len(digits) {
		if digits[i] < '0' || digits[i] > '9' {
			return Decimal{}, errDecimalSyntax
		}
	}
	d.exp -= len(fracPart)
	d.coef = strings.TrimLeft(digits, "0")
	return d, nil
}

// parseDecimalExponent parses an optional sign followed by bounded digits.
func parseDecimalExponent(s string) (int, error) {
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

// MustDecimal parses s with ParseDecimal and panics on error. It is intended
// for constants and tests.
func MustDecimal(s string) Decimal {
	d, err := ParseDecimal(s)
	if err != nil {
		panic(fmt.Sprintf("orm.MustDecimal(%q): %v", s, err))
	}
	return d
}

// DecimalFromInt returns the exact Decimal for n.
func DecimalFromInt(n int64) Decimal {
	if n == 0 {
		return Decimal{}
	}
	d, _ := ParseDecimal(strconv.FormatInt(n, 10))
	return d
}

// DecimalFromFloat converts a finite float64 to a Decimal using its shortest
// exact decimal representation. NaN and infinities return an error.
func DecimalFromFloat(f float64) (Decimal, error) {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return Decimal{}, fmt.Errorf("orm: %v value must be a decimal number", f)
	}
	return ParseDecimal(strconv.FormatFloat(f, 'g', -1, 64))
}

// digits returns the coefficient digits, or "0" when d is zero.
func (d Decimal) digits() string {
	if d.coef == "" {
		return "0"
	}
	return d.coef
}

// IsZero reports whether d is numerically zero.
func (d Decimal) IsZero() bool { return d.coef == "" }

// String formats d as Python's str(Decimal): plain notation where compact,
// scientific notation for positive exponents or very small values.
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

// FixedString formats d in plain fixed-point decimal notation with no exponent,
// suitable for SQL NUMERIC and DECIMAL literals and bind parameters.
func (d Decimal) FixedString() string {
	digits := d.digits()
	var b strings.Builder
	if d.neg {
		b.WriteByte('-')
	}
	if d.exp >= 0 {
		b.WriteString(digits)
		if !d.IsZero() && d.exp > 0 {
			b.WriteString(strings.Repeat("0", d.exp))
		}
		return b.String()
	}
	scale := -d.exp
	if len(digits) <= scale {
		b.WriteString("0.")
		b.WriteString(strings.Repeat("0", scale-len(digits)))
		b.WriteString(digits)
		return b.String()
	}
	split := len(digits) - scale
	b.WriteString(digits[:split])
	b.WriteByte('.')
	b.WriteString(digits[split:])
	return b.String()
}

// Quantize returns d rounded to exactly places fractional digits using
// round-half-to-even (Python's default ROUND_HALF_EVEN). Negative places are
// treated as 0.
func (d Decimal) Quantize(places int) Decimal {
	if places < 0 {
		places = 0
	}
	targetExp := -places
	if d.exp == targetExp {
		return d
	}
	if d.IsZero() {
		return Decimal{neg: d.neg, coef: "", exp: targetExp}
	}
	if d.exp > targetExp {
		return Decimal{
			neg:  d.neg,
			coef: d.coef + strings.Repeat("0", d.exp-targetExp),
			exp:  targetExp,
		}
	}
	// Fewer digits remain after dropping targetExp - d.exp trailing digits.
	cut := targetExp - d.exp
	if cut > len(d.coef) {
		return Decimal{neg: d.neg, coef: "", exp: targetExp}
	}
	kept := d.coef[:len(d.coef)-cut]
	dropped := d.coef[len(d.coef)-cut:]
	roundUp := shouldRoundUpHalfEven(kept, dropped)
	if kept == "" {
		kept = "0"
	}
	if roundUp {
		n, _ := new(big.Int).SetString(kept, 10)
		n.Add(n, big.NewInt(1))
		kept = n.String()
	}
	kept = strings.TrimLeft(kept, "0")
	return Decimal{neg: d.neg, coef: kept, exp: targetExp}
}

// shouldRoundUpHalfEven reports whether dropping dropped from kept rounds away
// from zero under ROUND_HALF_EVEN.
func shouldRoundUpHalfEven(kept, dropped string) bool {
	if dropped == "" || dropped[0] < '5' {
		return false
	}
	if dropped[0] > '5' {
		return true
	}
	for i := 1; i < len(dropped); i++ {
		if dropped[i] != '0' {
			return true
		}
	}
	// Exact tie: round up only when the last kept digit is odd.
	if kept == "" {
		return false
	}
	return (kept[len(kept)-1]-'0')%2 == 1
}

// Rat returns the exact rational value of d.
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

// Cmp compares the numeric values of d and other, returning -1, 0, or +1.
func (d Decimal) Cmp(other Decimal) int { return d.Rat().Cmp(other.Rat()) }

// Equal reports whether d and other are numerically equal ("1.5" equals "1.50").
func (d Decimal) Equal(other Decimal) bool { return d.Cmp(other) == 0 }

// ExactEqual reports whether d and other have the same sign, digits, and scale
// (like Python's Decimal.compare_total == 0).
func (d Decimal) ExactEqual(other Decimal) bool { return d == other }

// Float64 returns the nearest float64 to d.
func (d Decimal) Float64() float64 {
	f, _ := d.Rat().Float64()
	return f
}

// DigitCounts returns total digits and decimal places as Django's
// DecimalValidator counts them.
func (d Decimal) DigitCounts() (digits, decimals int) {
	n := len(d.digits())
	if d.exp >= 0 {
		if !d.IsZero() {
			n += d.exp
		}
		return n, 0
	}
	if -d.exp > n {
		return -d.exp, -d.exp
	}
	return n, -d.exp
}

// Value implements driver.Valuer, returning the plain fixed-point string so
// precision is never lost to floating point.
func (d Decimal) Value() (driver.Value, error) {
	return d.FixedString(), nil
}

// Scan implements sql.Scanner. Float values coming from SQLite's NUMERIC
// affinity are formatted to 15 significant digits, matching Django's SQLite
// decimal converter.
func (d *Decimal) Scan(src any) error {
	switch v := src.(type) {
	case nil:
		*d = Decimal{}
		return nil
	case string:
		parsed, err := ParseDecimal(v)
		if err != nil {
			return fmt.Errorf("orm: scan Decimal %q: %w", v, err)
		}
		*d = parsed
		return nil
	case []byte:
		parsed, err := ParseDecimal(string(v))
		if err != nil {
			return fmt.Errorf("orm: scan Decimal %q: %w", string(v), err)
		}
		*d = parsed
		return nil
	case int64:
		*d = DecimalFromInt(v)
		return nil
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return fmt.Errorf("orm: scan Decimal: non-finite float %v", v)
		}
		// SQLite stores non-integer decimals as IEEE-754 doubles (15 significant
		// decimal digits); mirror Django's Context(prec=15).create_decimal_from_float.
		parsed, err := ParseDecimal(strconv.FormatFloat(v, 'g', 15, 64))
		if err != nil {
			return fmt.Errorf("orm: scan Decimal %v: %w", v, err)
		}
		*d = parsed
		return nil
	default:
		return fmt.Errorf("orm: cannot scan %T into Decimal", src)
	}
}

// MarshalText implements encoding.TextMarshaler.
func (d Decimal) MarshalText() ([]byte, error) {
	return []byte(d.String()), nil
}

// UnmarshalText implements encoding.TextUnmarshaler.
func (d *Decimal) UnmarshalText(text []byte) error {
	parsed, err := ParseDecimal(string(text))
	if err != nil {
		return err
	}
	*d = parsed
	return nil
}
