package orm

import (
	"errors"
	"strconv"
)

// The SQL names of the math functions of several or no arguments, which
// resolveMath binds.
const (
	fnATan2  = "ATAN2"
	fnLog    = "LOG"
	fnMod    = "MOD"
	fnPi     = "PI"
	fnPower  = "POWER"
	fnRandom = "RANDOM"
	fnRound  = "ROUND"
)

// numericFunction builds a call of the SQL function fn of one argument whose
// value is a Decimal for a decimal argument and a float64 otherwise, Django's
// NumericOutputFieldMixin.
func numericFunction(fn string, arg any) Expr {
	e := function(fn, arg, 0, false)
	e.numeric = true
	return e
}

// numericType is the type of the value of a function of Django's
// NumericOutputFieldMixin whose argument has type t: a decimal stays a
// decimal, and any other type gives a float.
func numericType(t valueType) valueType {
	if t == typeDecimal {
		return typeDecimal
	}
	return typeFloat
}

// ACos returns the arccosine of arg, Django's ACos. arg is a field key or an
// Expr. The value is a Decimal for a decimal argument and a float64
// otherwise, as for the other trigonometric functions, Degrees, Exp, Ln,
// Radians and Sqrt, and nil when arg is NULL.
func ACos(arg any) Expr { return numericFunction("ACOS", arg) }

// ASin returns the arcsine of arg, Django's ASin.
func ASin(arg any) Expr { return numericFunction("ASIN", arg) }

// ATan returns the arctangent of arg, Django's ATan.
func ATan(arg any) Expr { return numericFunction("ATAN", arg) }

// ATan2 returns the arctangent of y / x, Django's ATan2. Each argument is a
// field key or an Expr. The value is a Decimal when an argument is a decimal
// and a float64 otherwise, as for Log, Mod and Power.
func ATan2(y, x any) Expr { return call(fnATan2, []any{y, x}) }

// Ceil returns the smallest integer that is not less than arg, Django's
// Ceil. The value has the kind of arg, as for Floor and Sign.
func Ceil(arg any) Expr { return function("CEILING", arg, 0, false) }

// Cos returns the cosine of arg, Django's Cos.
func Cos(arg any) Expr { return numericFunction("COS", arg) }

// Cot returns the cotangent of arg, Django's Cot. SQLite has no COT; there
// it is a function that this package's SQLite backend registers, as Django
// does.
func Cot(arg any) Expr { return numericFunction("COT", arg) }

// Degrees converts arg from radians to degrees, Django's Degrees.
func Degrees(arg any) Expr { return numericFunction("DEGREES", arg) }

// Exp returns e to the power of arg, Django's Exp.
func Exp(arg any) Expr { return numericFunction("EXP", arg) }

// Floor returns the largest integer that is not greater than arg, Django's
// Floor.
func Floor(arg any) Expr { return function("FLOOR", arg, 0, false) }

// Ln returns the natural logarithm of arg, Django's Ln.
func Ln(arg any) Expr { return numericFunction("LN", arg) }

// Log returns the logarithm of x to the base base, Django's Log.
func Log(base, x any) Expr { return call(fnLog, []any{base, x}) }

// Mod returns the remainder of x divided by y, Django's Mod.
func Mod(x, y any) Expr { return call(fnMod, []any{x, y}) }

// Pi returns the number pi as a float64, Django's Pi.
func Pi() Expr { return Expr{kind: exprCall, fn: fnPi} }

// Power returns x to the power of y, Django's Power.
func Power(x, y any) Expr { return call(fnPower, []any{x, y}) }

// Radians converts arg from degrees to radians, Django's Radians.
func Radians(arg any) Expr { return numericFunction("RADIANS", arg) }

// Random returns a random float64 in [0, 1), Django's Random. SQLite's
// RANDOM returns an integer, so there it is RAND, a function that this
// package's SQLite backend registers, as Django does.
func Random() Expr { return Expr{kind: exprCall, fn: fnRandom} }

// Round rounds arg to precision decimal places, Django's Round. The value has
// the kind of arg. A negative precision rounds to tens, hundreds and so on;
// SQLite does not support it, and the query fails with Django's message.
// Use 0 for Django's default precision:
//
//	Annotate("rounded", orm.Round("price", 2))
func Round(arg any, precision int) Expr {
	return Expr{kind: exprCall, fn: fnRound, args: []Expr{fieldOrExpr(arg), Value(precision)}}
}

// Sign returns -1, 0 or 1 for a negative, zero or positive arg, Django's
// Sign.
func Sign(arg any) Expr { return function("SIGN", arg, 0, false) }

// Sin returns the sine of arg, Django's Sin.
func Sin(arg any) Expr { return numericFunction("SIN", arg) }

// Sqrt returns the square root of arg, Django's Sqrt.
func Sqrt(arg any) Expr { return numericFunction("SQRT", arg) }

// Tan returns the tangent of arg, Django's Tan.
func Tan(arg any) Expr { return numericFunction("TAN", arg) }

// resolveMath binds ATan2, Log, Mod, Pi, Power, Random or Round. The value of
// Round has the type of its argument, and that of the others is a decimal
// when an argument is a decimal and a float otherwise. PostgreSQL has no LOG,
// MOD and ROUND of a double precision, so there a float argument of Log, Mod
// and Round is cast to NUMERIC(1000, 15), as Django's FixDecimalInputMixin
// does. The precision of Round is written into the SQL.
func (c *compiler) resolveMath(e Expr) (sqlNode, error) {
	sqlite := c.d.Name() == "sqlite"
	fixDecimal := c.d.Name() == "postgres" && (e.fn == fnLog || e.fn == fnMod || e.fn == fnRound)
	args := make([]sqlNode, len(e.args))
	typ := typeFloat
	for i, a := range e.args {
		if e.fn == fnRound && i == 1 {
			precision, _ := a.value.(int)
			if sqlite && precision < 0 {
				return nil, errors.New("orm: SQLite does not support negative precision.") //nolint:revive,staticcheck // Django's message
			}
			args[i] = rawNode{sql: strconv.Itoa(precision), typ: typeInt}
			continue
		}
		n, err := c.resolveExpr(a)
		if err != nil {
			return nil, err
		}
		if n.vtype() == typeDecimal {
			typ = typeDecimal
		}
		if e.fn == fnRound {
			typ = n.vtype()
		}
		if fixDecimal && n.vtype() == typeFloat {
			n = convertNode{inner: n, to: &Field{Kind: KindDecimal, MaxDigits: 1000, DecimalPlaces: 15}, input: KindFloat}
		}
		args[i] = n
	}
	fn := e.fn
	if fn == fnRandom && sqlite {
		fn = "RAND"
	}
	return callNode{fn: fn, args: args, typ: typ}, nil
}
