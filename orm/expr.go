package orm

import "strings"

// exprKind identifies the shape of an Expr.
type exprKind uint8

const (
	// The zero kind marks the zero Expr, which is not a valid expression.
	_ exprKind = iota
	// exprField references a field by key.
	exprField
	// exprValue is a literal bound as a query parameter.
	exprValue
	// exprBinary combines two expressions with an arithmetic operator.
	exprBinary
	// exprAggregate is an aggregate function call.
	exprAggregate
	// exprStar is the "*" argument of COUNT(*).
	exprStar
)

// Expr is an SQL value expression: a field reference (F), a literal (Value),
// an arithmetic combination of expressions, or an aggregate (Count, Sum, Avg,
// Min, Max). It is immutable; every method returns a new Expr.
//
// An Expr can be the value of a filter, so a column is compared with another
// column or with a computed value:
//
//	Filter("views__gt", orm.F("likes"))
//	Filter("views__gt", orm.F("likes").Mul(2))
//
// the value of an Update assignment, computed by the database:
//
//	Update(ctx, "views", orm.F("views").Add(1))
//
// or an annotation or aggregate:
//
//	Annotate("n", orm.Count("id"))
//	Aggregate(ctx, "avg_age", orm.Avg("age"))
type Expr struct {
	kind     exprKind
	key      string
	value    any
	op       string
	left     *Expr
	right    *Expr
	fn       string
	arg      *Expr
	distinct bool
}

// F references a model field by key, like Django's F(): F("views") or, across
// a relation, F("author__age"). "pk" names the primary key.
func F(key string) Expr { return Expr{kind: exprField, key: key} }

// Value wraps a Go value as a literal expression, bound as a query parameter
// (Django's Value()). Plain values passed to the arithmetic methods of Expr are
// wrapped automatically.
func Value(v any) Expr { return Expr{kind: exprValue, value: v} }

// toExpr returns x as an expression: an Expr unchanged, any other value as a
// literal.
func toExpr(x any) Expr {
	switch e := x.(type) {
	case Expr:
		return e
	case *Expr:
		if e != nil {
			return *e
		}
	}
	return Value(x)
}

// binary builds the arithmetic combination "e op x".
func (e Expr) binary(op string, x any) Expr {
	r := toExpr(x)
	return Expr{kind: exprBinary, op: op, left: &e, right: &r}
}

// Add returns e + x. x is an Expr or a plain value.
func (e Expr) Add(x any) Expr { return e.binary("+", x) }

// Sub returns e - x. x is an Expr or a plain value.
func (e Expr) Sub(x any) Expr { return e.binary("-", x) }

// Mul returns e * x. x is an Expr or a plain value.
func (e Expr) Mul(x any) Expr { return e.binary("*", x) }

// Div returns e / x. x is an Expr or a plain value. Dividing two integer
// columns is an integer division on both SQLite and PostgreSQL.
func (e Expr) Div(x any) Expr { return e.binary("/", x) }

// Mod returns e % x. x is an Expr or a plain value.
func (e Expr) Mod(x any) Expr { return e.binary("%", x) }

// Distinct makes an aggregate consider each distinct value once, like Django's
// distinct=True: orm.Count("author").Distinct() is COUNT(DISTINCT author_id).
// It is an error on an expression that is not an aggregate.
func (e Expr) Distinct() Expr {
	e.distinct = true
	return e
}

// aggregate builds a call of the aggregate function fn. arg is a field key or
// an Expr.
func aggregate(fn string, arg any) Expr {
	var a Expr
	switch v := arg.(type) {
	case string:
		if v == "*" {
			a = Expr{kind: exprStar}
		} else {
			a = F(v)
		}
	default:
		a = toExpr(arg)
	}
	return Expr{kind: exprAggregate, fn: fn, arg: &a}
}

// Count is the COUNT aggregate (Django's Count). arg is a field key such as
// "id" or "author__name", an Expr, or "*" for COUNT(*). The result is an
// int64. Use Distinct for COUNT(DISTINCT ...).
func Count(arg any) Expr { return aggregate("COUNT", arg) }

// Sum is the SUM aggregate (Django's Sum). arg is a field key or an Expr. The
// result is an int64 for an integer field and nil when no row matches.
func Sum(arg any) Expr { return aggregate("SUM", arg) }

// Avg is the AVG aggregate (Django's Avg). arg is a field key or an Expr. The
// result is a float64 on every dialect, and nil when no row matches.
func Avg(arg any) Expr { return aggregate("AVG", arg) }

// Min is the MIN aggregate (Django's Min). arg is a field key or an Expr. The
// result has the Go type of the field, and is nil when no row matches.
func Min(arg any) Expr { return aggregate("MIN", arg) }

// Max is the MAX aggregate (Django's Max). arg is a field key or an Expr. The
// result has the Go type of the field, and is nil when no row matches.
func Max(arg any) Expr { return aggregate("MAX", arg) }

// defaultAlias returns the name Django gives an aggregate that is passed
// without one, "<key>__<function>" (for example "age__avg"). Only an aggregate
// over a plain field reference has a default alias.
func (e Expr) defaultAlias() (string, bool) {
	if e.kind != exprAggregate || e.arg == nil || e.arg.kind != exprField {
		return "", false
	}
	return e.arg.key + "__" + strings.ToLower(e.fn), true
}

// containsAggregate reports whether the expression holds an aggregate call.
func (e Expr) containsAggregate() bool {
	switch e.kind {
	case exprAggregate:
		return true
	case exprBinary:
		return e.left.containsAggregate() || e.right.containsAggregate()
	default:
		return false
	}
}
