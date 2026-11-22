package orm

import (
	"fmt"
	"slices"
	"strings"
	"sync"
)

// fieldOrExpr returns x as an expression, with a string naming a field: the
// arguments of Django's functions are read so. Any other value that is not an
// Expr is a literal.
func fieldOrExpr(x any) Expr {
	if key, ok := x.(string); ok {
		return F(key)
	}
	return toExpr(x)
}

// function builds a call of the SQL function fn of one argument. arg is a
// field key or an Expr: like Django's functions, a string names a field. Pass
// Value("text") for a literal. output is the kind of the result when typed
// is set; otherwise the result has the kind of the argument.
func function(fn string, arg any, output Kind, typed bool) Expr {
	a := fieldOrExpr(arg)
	return Expr{kind: exprFunc, fn: fn, arg: &a, output: output, typed: typed}
}

// Lower returns arg in lower case, Django's Lower. arg is a field key such as
// "name", or an Expr.
func Lower(arg any) Expr { return function("LOWER", arg, 0, false) }

// Upper returns arg in upper case, Django's Upper.
func Upper(arg any) Expr { return function("UPPER", arg, 0, false) }

// Length returns the number of characters of arg, Django's Length. The value
// is an int64, and nil when arg is NULL.
func Length(arg any) Expr { return function("LENGTH", arg, KindInt, true) }

// Trim returns arg without leading and trailing spaces, Django's Trim.
func Trim(arg any) Expr { return function("TRIM", arg, 0, false) }

// LTrim returns arg without leading spaces, Django's LTrim.
func LTrim(arg any) Expr { return function("LTRIM", arg, 0, false) }

// RTrim returns arg without trailing spaces, Django's RTrim.
func RTrim(arg any) Expr { return function("RTRIM", arg, 0, false) }

// Abs returns the absolute value of arg, Django's Abs. The value has the kind
// of arg, and is nil when arg is NULL.
func Abs(arg any) Expr { return function("ABS", arg, 0, false) }

// fnConcat marks the exprCall of Concat, which is written with || and not as
// a call.
const fnConcat = "CONCAT"

// call builds a call of the SQL function fn of several arguments, each a
// field key or an Expr.
func call(fn string, args []any) Expr {
	exprs := make([]Expr, len(args))
	for i, a := range args {
		exprs[i] = fieldOrExpr(a)
	}
	return Expr{kind: exprCall, fn: fn, args: exprs}
}

// Coalesce returns the first of its arguments that is not NULL, Django's
// Coalesce. Each argument is a field key or an Expr; pass Value("text") for a
// literal. It takes at least two arguments:
//
//	Annotate("display_name", orm.Coalesce("alias", "name"))
//	Annotate("total", orm.Coalesce(orm.Sum("views"), orm.Value(0)))
func Coalesce(args ...any) Expr { return call("COALESCE", args) }

// Concat joins the text of its arguments, Django's Concat. Each argument is a
// field key or an Expr; pass Value(" - ") for a literal. A NULL argument
// counts as the empty text, and one that is not text is cast to text. It
// takes at least two arguments, and the value is a string:
//
//	Annotate("label", orm.Concat("name", orm.Value(" ("), "goes_by", orm.Value(")")))
func Concat(args ...any) Expr { return call(fnConcat, args) }

// resolveCall binds Coalesce or Concat. Concat casts each argument that is not
// text to text on every dialect, as Cast does. Django's ConcatPair casts on
// PostgreSQL only and leaves the conversion to the || of SQLite, which gives
// the same text.
func (c *compiler) resolveCall(e Expr) (sqlNode, error) {
	if len(e.args) < 2 {
		name := "Coalesce"
		if e.fn == fnConcat {
			name = "Concat"
		}
		return nil, fmt.Errorf("orm: %s must take at least two expressions", name)
	}
	args := make([]sqlNode, len(e.args))
	for i, a := range e.args {
		n, err := c.resolveExpr(a)
		if err != nil {
			return nil, err
		}
		if e.fn == fnConcat && n.vtype() != typeString {
			n = convertNode{inner: n, to: &Field{Kind: KindText}, input: valueKind(n.vtype())}
		}
		args[i] = n
	}
	if e.fn == fnConcat {
		return concatNode{args: args}, nil
	}
	return callNode{fn: e.fn, args: args}, nil
}

// Cast converts arg to the type of the field to, Django's Cast. arg is a
// field key or an Expr, or another Go value as a literal. Of to only Kind,
// MaxLength, MaxDigits and DecimalPlaces count: they give the SQL type, the
// one a column of the field has, and the Go type the value is read as.
//
//	Annotate("age_text", orm.Cast("age", orm.Field{Kind: orm.KindChar, MaxLength: 255}))
//	Annotate("day", orm.Cast("start_datetime", orm.Field{Kind: orm.KindDate}))
//
// A KindChar without MaxLength casts to TEXT. A datetime is cast to a date or
// a time of day in TransformTimeZone, and a date is midnight in that zone.
// SQLite has no date and time types: there a cast to a datetime, a date or a
// time of day converts the text, and a cast to a VARCHAR or a DECIMAL keeps
// the length and the digits of the value, which PostgreSQL cuts and rounds.
func Cast(arg any, to Field) Expr {
	a := fieldOrExpr(arg)
	f := Field{Kind: to.Kind, MaxLength: to.MaxLength, MaxDigits: to.MaxDigits, DecimalPlaces: to.DecimalPlaces}
	return Expr{kind: exprCast, arg: &a, castTo: &f}
}

// CastDialect is an optional capability of a Dialect whose CAST does not
// convert every value as Django's Cast does, such as SQLite's, which has no
// date and time types. CastSQL returns the SQL that converts a value of kind
// input to typ, the column type of kind output, as a template like the one of
// TransformDialect, or ok false for CAST(value AS typ). tzname is the time
// zone a datetime is read in (TransformTimeZone).
type CastDialect interface {
	CastSQL(input, output Kind, typ, tzname string) (template string, params []any, ok bool)
}

// outputKind returns the kind of the value of e when the expression it
// transforms has kind input: the result kind of a function such as Length or
// Cast, and input for the others.
func (e Expr) outputKind(input Kind) Kind {
	switch e.kind {
	case exprFunc:
		if e.typed {
			return e.output
		}
		return e.arg.outputKind(input)
	case exprCast:
		return e.castTo.Kind
	case exprNot:
		return KindBool
	case exprCall:
		if e.fn == fnConcat {
			return KindText
		}
	}
	return input
}

// registeredTransform is a transform of RegisterTransform.
type registeredTransform struct {
	name string
	fn   func(arg any) Expr
}

// transformRegistry holds the transforms of RegisterTransform by field kind.
var transformRegistry struct {
	sync.RWMutex
	byKind map[Kind][]*registeredTransform
}

// RegisterTransform makes fn the transform name of the fields of kind k,
// Django's Field.register_lookup with a Transform such as Lower. After
//
//	orm.RegisterTransform(orm.KindChar, "lower", orm.Lower)
//
// the key "name__lower" is the lowercased name in Filter ("name__lower",
// "john smith"), Values, OrderBy and F, and the lookups that follow it
// compare the lowercased value ("name__lower__startswith"). fn gets the
// transformed value and returns the expression of the transform. The value
// of the transform has the kind of the result of fn: an integer for Length,
// and kind k for the other text functions.
//
// The transform applies to kind k only, where Django's registration on
// CharField also covers its subclasses: register it for KindSlug, KindEmail
// and KindURL as well where they need it. A later registration of a name
// takes the place of an earlier one. RegisterTransform returns the function
// that removes the transform again. It panics on an empty name, a name with
// "__" or a nil fn.
func RegisterTransform(k Kind, name string, fn func(arg any) Expr) (unregister func()) {
	if name == "" || strings.Contains(name, lookupSep) || fn == nil {
		panic(fmt.Sprintf("orm: RegisterTransform needs a name without %q and a function", lookupSep))
	}
	rt := &registeredTransform{name: name, fn: fn}
	transformRegistry.Lock()
	defer transformRegistry.Unlock()
	if transformRegistry.byKind == nil {
		transformRegistry.byKind = map[Kind][]*registeredTransform{}
	}
	transformRegistry.byKind[k] = append(transformRegistry.byKind[k], rt)
	return func() {
		transformRegistry.Lock()
		defer transformRegistry.Unlock()
		transformRegistry.byKind[k] = slices.DeleteFunc(transformRegistry.byKind[k], func(x *registeredTransform) bool { return x == rt })
	}
}

// registered returns the function of the transform name of kind k that
// RegisterTransform registered last.
func registered(k Kind, name string) (func(arg any) Expr, bool) {
	transformRegistry.RLock()
	defer transformRegistry.RUnlock()
	list := transformRegistry.byKind[k]
	for i := len(list) - 1; i >= 0; i-- {
		if list[i].name == name {
			return list[i].fn, true
		}
	}
	return nil, false
}

// registeredNames returns the names of the transforms of kind k that
// RegisterTransform registered, each once.
func registeredNames(k Kind) []string {
	transformRegistry.RLock()
	defer transformRegistry.RUnlock()
	var names []string
	for _, rt := range transformRegistry.byKind[k] {
		if !slices.Contains(names, rt.name) {
			names = append(names, rt.name)
		}
	}
	return names
}
