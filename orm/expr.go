package orm

import (
	"slices"
	"strings"
)

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
	// exprFunc is a call of a function of one argument, such as LOWER.
	exprFunc
	// exprNode is a value that is already bound to the query: the value a
	// transform of RegisterTransform transforms.
	exprNode
	// exprCall is a call of a function of several arguments: Coalesce,
	// Concat, Greatest or Least.
	exprCall
	// exprCast converts its argument to the type of a field.
	exprCast
	// exprCase is a conditional expression.
	exprCase
	// exprNot negates a boolean expression.
	exprNot
	// exprWrap reads its argument as a value of another type, Django's
	// ExpressionWrapper.
	exprWrap
	// exprSubquery is the value of a query, Subquery, or whether it matches
	// any row, Exists.
	exprSubquery
	// exprOuterRef references a field of the query that a subquery is part
	// of, OuterRef.
	exprOuterRef
)

// Expr is an SQL value expression: a field reference (F), a literal (Value),
// an arithmetic or bitwise combination of expressions, an aggregate (Count,
// Sum, Avg, Min, Max, StdDev, Variance, AnyValue, StringAgg), a text function
// (Lower, Upper, Length, Trim, LTrim, RTrim, Concat), Abs, Coalesce,
// Greatest, Least, Cast, a conditional expression (Case), the negation of a
// boolean expression (Not), an expression of a given type
// (ExpressionWrapper), the value of a query (Subquery, Exists) or a field of
// the query a subquery is part of (OuterRef). It is immutable; every method
// returns a new Expr.
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
// an annotation or aggregate:
//
//	Annotate("n", orm.Count("id"))
//	Aggregate(ctx, "avg_age", orm.Avg("age"))
//
// or, when it is boolean, a condition of Filter, Exclude or Q:
//
//	Filter(orm.Exists(orm.Query[Book](db).Filter("author", orm.OuterRef("pk"))))
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
	// filter is the condition of an aggregate's Filter, nil without one.
	filter *Cond
	// output is the kind of the result of a function when typed is set.
	// Without typed the result has the kind of the argument.
	output Kind
	typed  bool
	// node is the bound value of an exprNode.
	node sqlNode
	// args are the arguments of an exprCall, and of an exprAggregate the
	// arguments after the first, such as the delimiter of StringAgg.
	args []Expr
	// castTo is the field whose type an exprCast converts to, and the field
	// whose type an exprWrap reads its argument as.
	castTo *Field
	// whens are the branches of an exprCase, and def its Default, nil
	// without one. def is also the Default of an aggregate.
	whens []When
	def   *Expr
	// order holds the terms of an aggregate's OrderBy, in the forms
	// QuerySet.OrderBy takes.
	order []any
	// sub is the query of an exprSubquery, and exists reports an Exists.
	sub    *query
	exists bool
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

// Not returns the negation of the boolean expression e, Django's ~F() and
// ~Case(): F("is_active").Not() is true where is_active is false. A query
// with the negation of an expression that is not boolean fails with an
// error, as Django raises a TypeError:
//
//	Update(ctx, "is_active", orm.F("is_active").Not())
//
// The negation of a negation is the expression itself.
func (e Expr) Not() Expr {
	if e.kind == exprNot {
		return *e.arg
	}
	return Expr{kind: exprNot, arg: &e}
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

// The operators of Expr that are not written infix in every dialect.
const (
	// opPow is the power operator, rendered as POWER(e, x).
	opPow = "^"
	// opBitXor is the bitwise exclusive or, which SQLite has no operator for.
	opBitXor = "#"
	// opShiftLeft and opShiftRight shift an integer by a number of bits.
	opShiftLeft  = "<<"
	opShiftRight = ">>"
)

// Pow returns e raised to the power x, like Django's F("n") ** x. x is an
// Expr or a plain value. The SQL is POWER(e, x), a floating point number on
// both dialects. With integer operands the result is read as an integer,
// truncated toward zero; a fractional result written to an integer column is
// rounded by PostgreSQL and kept as a real number by SQLite.
func (e Expr) Pow(x any) Expr { return e.binary(opPow, x) }

// BitAnd returns the bitwise and of e and x, like Django's bitand().
func (e Expr) BitAnd(x any) Expr { return e.binary("&", x) }

// BitOr returns the bitwise or of e and x, like Django's bitor().
func (e Expr) BitOr(x any) Expr { return e.binary("|", x) }

// BitXor returns the bitwise exclusive or of e and x, like Django's
// bitxor(). It is written as (e | x) - (e & x), which is the same for
// integers and runs on every dialect.
func (e Expr) BitXor(x any) Expr { return e.binary(opBitXor, x) }

// BitLeftShift returns e shifted left by x bits, like Django's
// bitleftshift().
func (e Expr) BitLeftShift(x any) Expr { return e.binary(opShiftLeft, x) }

// BitRightShift returns e shifted right by x bits, like Django's
// bitrightshift().
func (e Expr) BitRightShift(x any) Expr { return e.binary(opShiftRight, x) }

// Distinct makes an aggregate consider each distinct value once, like Django's
// distinct=True: orm.Count("author").Distinct() is COUNT(DISTINCT author_id).
// It is an error on an expression that is not an aggregate.
func (e Expr) Distinct() Expr {
	e.distinct = true
	return e
}

// Filter makes an aggregate consider only the rows that match the condition,
// like Django's filter= argument: orm.Count("pk").Filter("rating__gt", 3)
// counts the rows rated above 3. The arguments are those of Q. An aggregate
// with a filter has no default alias. Filter is an error on an expression
// that is not an aggregate, and on Count("*").
func (e Expr) Filter(args ...any) Expr {
	c := Q(args...)
	e.filter = &c
	return e
}

// OrderBy orders the values that a StringAgg joins, Django's order_by
// argument of an aggregate. Each term is a field key, "-" and a key for
// descending order, an Expr or an OrderExpr:
//
//	orm.StringAgg("name", orm.Value(";")).OrderBy("-original_opening")
//
// An aggregate with an ordering has no default alias. OrderBy is an error on
// an expression that is not a StringAgg, as Django allows order_by only
// where the order changes the value.
func (e Expr) OrderBy(terms ...any) Expr {
	e.order = slices.Clone(terms)
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
// result is an orm.Decimal for a decimal field, a time.Duration for a
// duration field and a float64 for the others, on every dialect, and nil when
// no row matches.
func Avg(arg any) Expr { return aggregate("AVG", arg) }

// Min is the MIN aggregate (Django's Min). arg is a field key or an Expr. The
// result has the Go type of the field, and is nil when no row matches.
func Min(arg any) Expr { return aggregate("MIN", arg) }

// Max is the MAX aggregate (Django's Max). arg is a field key or an Expr. The
// result has the Go type of the field, and is nil when no row matches.
func Max(arg any) Expr { return aggregate("MAX", arg) }

// StdDev is the standard deviation aggregate (Django's StdDev): of the
// population, STDDEV_POP, or of a sample when sample is set, STDDEV_SAMP. arg
// is a field key or an Expr. The result is an orm.Decimal for a decimal field
// and a float64 for the others, and nil when no row matches or, for a sample,
// when one row matches. SQLite has no such function; the SQLite backend
// registers one.
func StdDev(arg any, sample bool) Expr {
	if sample {
		return aggregate("STDDEV_SAMP", arg)
	}
	return aggregate("STDDEV_POP", arg)
}

// Variance is the variance aggregate (Django's Variance): of the population,
// VAR_POP, or of a sample when sample is set, VAR_SAMP. arg is a field key or
// an Expr. The result is that of StdDev.
func Variance(arg any, sample bool) Expr {
	if sample {
		return aggregate("VAR_SAMP", arg)
	}
	return aggregate("VAR_POP", arg)
}

// AnyValue is the ANY_VALUE aggregate (Django's AnyValue): one of the values
// of the group that is not NULL, so that a column which is the same in each
// row of a group can be selected without grouping by it. arg is a field key
// or an Expr, and the result has its Go type. PostgreSQL has ANY_VALUE from
// version 16 on; the SQLite backend registers one.
func AnyValue(arg any) Expr { return aggregate("ANY_VALUE", arg) }

// StringAgg joins the text of the values of arg with delimiter between them,
// Django's StringAgg, STRING_AGG in SQL. arg and delimiter are field keys or
// Exprs; pass Value(";") for a literal delimiter. The result is a string, nil
// when no row matches. OrderBy orders the values. SQLite refuses Distinct on
// it, since its STRING_AGG takes the delimiter as a second argument.
//
//	Aggregate(ctx, "names", orm.StringAgg("name", orm.Value(", ")).OrderBy("name"))
func StringAgg(arg any, delimiter any) Expr {
	e := aggregate(fnStringAgg, arg)
	e.args = []Expr{fieldOrExpr(delimiter)}
	return e
}

// fnStringAgg is the SQL function of StringAgg.
const fnStringAgg = "STRING_AGG"

// aggregateName returns the name of the Django aggregate class of the SQL
// function fn, as Django's messages and default aliases name it.
func aggregateName(fn string) string {
	switch fn {
	case "STDDEV_POP", "STDDEV_SAMP":
		return "StdDev"
	case "VAR_POP", "VAR_SAMP":
		return "Variance"
	case "ANY_VALUE":
		return "AnyValue"
	case fnStringAgg:
		return "StringAgg"
	default:
		return fn[:1] + strings.ToLower(fn[1:])
	}
}

// defaultAlias returns the name Django gives an aggregate that is passed
// without one, "<key>__<function>" (for example "age__avg"). Only an aggregate
// of one plain field reference, without a filter or an ordering, has a default
// alias.
func (e Expr) defaultAlias() (string, bool) {
	if e.kind != exprAggregate || e.arg == nil || e.arg.kind != exprField || e.filter != nil ||
		len(e.args) > 0 || len(e.order) > 0 {
		return "", false
	}
	return e.arg.key + "__" + strings.ToLower(aggregateName(e.fn)), true
}

// containsAggregate reports whether the expression holds an aggregate call.
func (e Expr) containsAggregate() bool {
	switch e.kind {
	case exprAggregate:
		return true
	case exprBinary:
		return e.left.containsAggregate() || e.right.containsAggregate()
	case exprFunc, exprCast, exprNot, exprWrap:
		return e.arg.containsAggregate()
	case exprNode:
		return e.node.aggregate()
	case exprCall:
		return slices.ContainsFunc(e.args, Expr.containsAggregate)
	case exprCase:
		for _, w := range e.whens {
			if fieldOrExpr(w.Then).containsAggregate() {
				return true
			}
		}
		return e.def != nil && e.def.containsAggregate()
	default:
		return false
	}
}
