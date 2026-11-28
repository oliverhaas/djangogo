package orm

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
	"time"
)

// valueType is the Go type a selected SQL value is converted to.
type valueType uint8

const (
	// typeUnknown leaves the driver's value as it is, except that bytes become
	// a string.
	typeUnknown valueType = iota
	// typeInt converts to int64.
	typeInt
	// typeFloat converts to float64.
	typeFloat
	// typeString converts to string.
	typeString
	// typeBool converts to bool.
	typeBool
	// typeTime converts to time.Time.
	typeTime
	// typeDecimal converts to Decimal.
	typeDecimal
	// typeDate converts to Date.
	typeDate
	// typeTimeOfDay converts to Time.
	typeTimeOfDay
	// typeDuration converts to time.Duration.
	typeDuration
	// typeUUID converts to UUID.
	typeUUID
	// typeJSON converts to json.RawMessage.
	typeJSON
	// typeBinary converts to []byte.
	typeBinary
)

// fieldValueType returns the type the values of field f are read as.
func fieldValueType(f *Field) valueType {
	switch f.Kind {
	case KindAuto, KindInt, KindSmallInt, KindBigInt,
		KindPositiveSmallInt, KindPositiveInt, KindPositiveBigInt:
		return typeInt
	case KindFloat:
		return typeFloat
	case KindDecimal:
		return typeDecimal
	case KindChar, KindText, KindSlug, KindEmail, KindURL, KindGenericIPAddress:
		return typeString
	case KindBool:
		return typeBool
	case KindDateTime:
		return typeTime
	case KindDate:
		return typeDate
	case KindTime:
		return typeTimeOfDay
	case KindDuration:
		return typeDuration
	case KindUUID:
		return typeUUID
	case KindJSON:
		return typeJSON
	case KindBinary:
		return typeBinary
	default:
		return typeUnknown
	}
}

// goValueType returns the type a Go value bound as a parameter reads back as.
func goValueType(v any) valueType {
	if v == nil {
		return typeUnknown
	}
	t := reflect.TypeOf(v)
	switch t {
	case timeType:
		return typeTime
	case decimalType:
		return typeDecimal
	case dateType:
		return typeDate
	case timeOfDayType:
		return typeTimeOfDay
	case durationType:
		return typeDuration
	case uuidType:
		return typeUUID
	case jsonRawMessageType:
		return typeJSON
	case byteSliceType:
		return typeBinary
	case netipAddrType:
		return typeString
	}
	switch t.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return typeInt
	case reflect.Float32, reflect.Float64:
		return typeFloat
	case reflect.String:
		return typeString
	case reflect.Bool:
		return typeBool
	default:
		return typeUnknown
	}
}

// sqlWriter accumulates the text of a statement and its bound arguments.
// Placeholders are numbered as they are written, so the arguments are always
// in the textual order that positional placeholders need.
type sqlWriter struct {
	d    Dialect
	b    *strings.Builder
	args []any
	// ddl writes for a schema statement: columns without their table alias,
	// and values as SQL literals instead of bound parameters, which DDL does
	// not take. err records the first value that has no literal form.
	ddl bool
	err error
}

// newSQLWriter returns an empty writer for dialect d.
func newSQLWriter(d Dialect) *sqlWriter {
	return &sqlWriter{d: d, b: &strings.Builder{}}
}

// write appends raw SQL text.
func (w *sqlWriter) write(s string) { w.b.WriteString(s) }

// param binds v and appends its placeholder.
func (w *sqlWriter) param(v any) {
	if w.ddl {
		w.b.WriteString(w.quoteValue(v))
		return
	}
	w.args = append(w.args, v)
	w.b.WriteString(w.d.Placeholder(len(w.args)))
}

// literal binds v, the literal of an expression, and appends its placeholder,
// typed by a dialect that needs that.
func (w *sqlWriter) literal(v any) {
	var placeholder string
	if w.ddl {
		placeholder = w.quoteValue(v)
	} else {
		w.args = append(w.args, literalArg(w.d, v))
		placeholder = w.d.Placeholder(len(w.args))
	}
	if ld, ok := w.d.(LiteralDialect); ok {
		placeholder = ld.LiteralSQL(placeholder, v)
	}
	w.b.WriteString(placeholder)
}

// literalArg returns the argument bound for the literal v. A duration is
// bound as the dialect stores one, a count of microseconds on SQLite, and
// not as the nanoseconds of time.Duration. An unsigned integer and a
// netip.Addr are bound as database/sql takes them, as an int64 and as text.
func literalArg(d Dialect, v any) any {
	var f *Field
	if _, ok := v.(time.Duration); ok {
		f = &Field{Kind: KindDuration}
	}
	if bound, err := bindField(d, f, v); err == nil {
		return bound
	}
	return v
}

// quoteValue returns v as an SQL literal and records the error of a value
// that has none.
func (w *sqlWriter) quoteValue(v any) string {
	s, err := QuoteValue(w.d, v)
	if err != nil && w.err == nil {
		w.err = err
	}
	return s
}

// ident appends a quoted identifier.
func (w *sqlWriter) ident(name string) { w.b.WriteString(w.d.Quote(name)) }

// column appends a column qualified by its table alias.
func (w *sqlWriter) column(alias, column string) {
	if w.ddl {
		w.b.WriteString(w.d.Quote(column))
		return
	}
	w.b.WriteString(w.d.Quote(alias))
	w.b.WriteByte('.')
	w.b.WriteString(w.d.Quote(column))
}

// capture runs fn and returns the text it wrote instead of appending it. The
// arguments fn binds are kept, so the captured text must be written back in
// the same order.
func (w *sqlWriter) capture(fn func()) string {
	saved := w.b
	w.b = &strings.Builder{}
	fn()
	s := w.b.String()
	w.b = saved
	return s
}

// String returns the statement text written so far.
func (w *sqlWriter) String() string { return w.b.String() }

// sqlNode is a resolved value expression: its fields are bound to the joins of
// one query, and it renders itself into a statement.
type sqlNode interface {
	// render writes the expression.
	render(w *sqlWriter)
	// vtype is the Go type the expression's value is read as.
	vtype() valueType
	// aggregate reports whether the expression holds an aggregate call.
	aggregate() bool
}

// colNode is a column of a joined table. value is set when a constraint is
// checked against an instance: the instance's value of the field then takes
// the place of the column.
type colNode struct {
	join  *join
	field *Field
	value *instanceValue
}

// render writes the qualified column, or the instance value that replaces it.
func (n colNode) render(w *sqlWriter) {
	if n.value != nil {
		n.value.render(w)
		return
	}
	w.column(n.join.alias, n.field.Column)
}

// vtype is the type of the column's field.
func (n colNode) vtype() valueType { return fieldValueType(n.field) }

// aggregate is false: a column is not an aggregate.
func (colNode) aggregate() bool { return false }

// castNode casts node to the column type typ, e.g. the text object id of a
// generic relation to the integer type of the primary key it holds.
type castNode struct {
	node sqlNode
	typ  string
}

// render writes CAST(node AS typ).
func (n castNode) render(w *sqlWriter) {
	w.write("CAST(")
	n.node.render(w)
	w.write(" AS " + n.typ + ")")
}

// vtype is the type of node: the cast keeps the value comparable with the
// column it is cast for.
func (n castNode) vtype() valueType { return n.node.vtype() }

// aggregate reports whether node holds an aggregate call.
func (n castNode) aggregate() bool { return n.node.aggregate() }

// paramNode is a value bound as a query parameter. literal marks the literal
// of an expression, which has no column next to it that gives it a type.
type paramNode struct {
	value   any
	literal bool
}

// render binds the value.
func (n paramNode) render(w *sqlWriter) {
	if n.literal {
		w.literal(n.value)
		return
	}
	w.param(n.value)
}

// vtype follows the Go type of the value.
func (n paramNode) vtype() valueType { return goValueType(n.value) }

// aggregate is false: a literal is not an aggregate.
func (paramNode) aggregate() bool { return false }

// rawNode is fixed SQL text that needs no quoting or binding, such as "1".
type rawNode struct {
	sql string
	typ valueType
}

// render writes the text.
func (n rawNode) render(w *sqlWriter) { w.write(n.sql) }

// vtype is the declared type of the text.
func (n rawNode) vtype() valueType { return n.typ }

// aggregate is false: fixed text is never an aggregate here.
func (rawNode) aggregate() bool { return false }

// binaryNode combines two expressions with an arithmetic operator. tmpl, when
// set, is the SQL of the dialect for a combination of dates, times and
// durations (TemporalDialect).
type binaryNode struct {
	op          string
	left, right sqlNode
	tmpl        string
}

// TemporalDialect is an optional capability of a Dialect whose arithmetic on
// dates, datetimes, times of day and durations is not that of the SQL
// operators, Django's combine_duration_expression and subtract_temporals.
// Each method returns a template in which the first "{}" stands for the left
// side and the second "{}" for the right side, or "" for "(left op right)".
//
// DurationSQL is the SQL of left op right where one side is a duration and
// the other is not. An error refuses the operator. SubtractTemporalSQL is
// the SQL of left - right where both sides have kind KindDate, KindDateTime
// or KindTime; the result is a duration.
type TemporalDialect interface {
	DurationSQL(op string) (template string, err error)
	SubtractTemporalSQL(kind Kind) (template string)
}

// render writes "(left op right)", or tmpl with the sides in its "{}". The
// parentheses keep the precedence of nested combinations as it was written in
// Go. The operators without a common infix form are written as functions or
// identities that both dialects run.
func (n binaryNode) render(w *sqlWriter) {
	if n.tmpl != "" {
		before, rest, _ := strings.Cut(n.tmpl, "{}")
		middle, after, _ := strings.Cut(rest, "{}")
		w.write(before)
		n.left.render(w)
		w.write(middle)
		n.right.render(w)
		w.write(after)
		return
	}
	switch n.op {
	case opPow:
		w.write("POWER(")
		n.left.render(w)
		w.write(", ")
		n.right.render(w)
		w.write(")")
		return
	case opBitXor:
		// a XOR b is (a | b) - (a & b): the bits of a & b are a subset of
		// those of a | b, so the subtraction clears them without a borrow.
		w.write("((")
		n.left.render(w)
		w.write(" | ")
		n.right.render(w)
		w.write(") - (")
		n.left.render(w)
		w.write(" & ")
		n.right.render(w)
		w.write("))")
		return
	}
	w.write("(")
	n.left.render(w)
	w.write(" " + n.op + " ")
	if n.op == opShiftLeft || n.op == opShiftRight {
		// PostgreSQL shifts a bigint by an integer only, and a literal is
		// bound as a bigint.
		w.write("CAST(")
		n.right.render(w)
		w.write(" AS INTEGER)")
	} else {
		n.right.render(w)
	}
	w.write(")")
}

// vtype is the type of Django's table of temporal combinations, and
// otherwise float when either side is float, decimal when either side is
// decimal, and the type of the first side that has a known type, as Django
// combines an integer with a float or a decimal.
func (n binaryNode) vtype() valueType {
	if t, ok := temporalResult(n.op, n.left.vtype(), n.right.vtype()); ok {
		return t
	}
	t := commonType([]sqlNode{n.left, n.right})
	if t != typeFloat && (n.left.vtype() == typeDecimal || n.right.vtype() == typeDecimal) {
		return typeDecimal
	}
	return t
}

// temporalResult returns the type of left op right by Django's table of
// combinations of dates, datetimes, times of day and durations: a date or a
// datetime shifted by a duration is a datetime, a time of day shifted by a
// duration is a time of day, and the difference of two dates or datetimes, or
// of two times of day, is a duration. ok is false for the other combinations.
func temporalResult(op string, left, right valueType) (valueType, bool) {
	dated := func(t valueType) bool { return t == typeDate || t == typeTime }
	switch op {
	case "+":
		switch {
		case dated(left) && right == typeDuration, left == typeDuration && dated(right):
			return typeTime, true
		case left == typeTimeOfDay && right == typeDuration, left == typeDuration && right == typeTimeOfDay:
			return typeTimeOfDay, true
		}
	case "-":
		switch {
		case dated(left) && right == typeDuration:
			return typeTime, true
		case left == typeTimeOfDay && right == typeDuration:
			return typeTimeOfDay, true
		case dated(left) && dated(right), left == typeTimeOfDay && right == typeTimeOfDay:
			return typeDuration, true
		}
	}
	return typeUnknown, false
}

// commonType is the type that the values of nodes are read as together: float
// when one of them is float, and otherwise the first type that is known.
func commonType(nodes []sqlNode) valueType {
	out := typeUnknown
	for _, n := range nodes {
		switch t := n.vtype(); {
		case t == typeFloat:
			return typeFloat
		case out == typeUnknown:
			out = t
		}
	}
	return out
}

// anyAggregate reports whether one of nodes holds an aggregate.
func anyAggregate(nodes []sqlNode) bool {
	for _, n := range nodes {
		if n.aggregate() {
			return true
		}
	}
	return false
}

// aggregate reports whether either side holds an aggregate.
func (n binaryNode) aggregate() bool { return n.left.aggregate() || n.right.aggregate() }

// funcNode is a call of a function of one argument, such as LOWER(name).
type funcNode struct {
	fn  string
	arg sqlNode
	typ valueType
}

// render writes "FN(arg)". A decimal result is cast to NUMERIC on SQLite, as
// for a caseNode.
func (n funcNode) render(w *sqlWriter) {
	if n.typ == typeDecimal && w.d.Name() == "sqlite" {
		w.write("CAST(")
		defer w.write(" AS NUMERIC)")
	}
	w.write(n.fn + "(")
	n.arg.render(w)
	w.write(")")
}

// vtype is the type of the function's result.
func (n funcNode) vtype() valueType { return n.typ }

// aggregate reports whether the argument holds an aggregate.
func (n funcNode) aggregate() bool { return n.arg.aggregate() }

// notNode is the negation of a boolean expression.
type notNode struct {
	inner sqlNode
}

// render writes "NOT (inner)".
func (n notNode) render(w *sqlWriter) {
	w.write("NOT (")
	n.inner.render(w)
	w.write(")")
}

// vtype is bool.
func (notNode) vtype() valueType { return typeBool }

// aggregate reports whether the negated expression holds an aggregate.
func (n notNode) aggregate() bool { return n.inner.aggregate() }

// typedNode is node read as type typ, such as the COALESCE of an aggregate
// with its Default, which has the type of the aggregate.
type typedNode struct {
	node sqlNode
	typ  valueType
}

// render writes node.
func (n typedNode) render(w *sqlWriter) { n.node.render(w) }

// vtype is typ.
func (n typedNode) vtype() valueType { return n.typ }

// aggregate reports whether node holds an aggregate.
func (n typedNode) aggregate() bool { return n.node.aggregate() }

// callNode is a call of a function of several arguments, such as
// COALESCE(a, b).
type callNode struct {
	fn   string
	args []sqlNode
}

// render writes "FN(a, b, ...)". A decimal result is cast to NUMERIC on
// SQLite, as for a funcNode.
func (n callNode) render(w *sqlWriter) {
	if n.vtype() == typeDecimal && w.d.Name() == "sqlite" {
		w.write("CAST(")
		defer w.write(" AS NUMERIC)")
	}
	w.write(n.fn + "(")
	for i, a := range n.args {
		if i > 0 {
			w.write(", ")
		}
		a.render(w)
	}
	w.write(")")
}

// vtype is the common type of the arguments.
func (n callNode) vtype() valueType { return commonType(n.args) }

// aggregate reports whether an argument holds an aggregate.
func (n callNode) aggregate() bool { return anyAggregate(n.args) }

// collateNode is an expression read with a collation, Django's Collate.
type collateNode struct {
	inner     sqlNode
	collation string
}

// render writes `inner COLLATE "collation"`.
func (n collateNode) render(w *sqlWriter) {
	n.inner.render(w)
	w.write(" COLLATE ")
	w.ident(n.collation)
}

// vtype is the type of the expression.
func (n collateNode) vtype() valueType { return n.inner.vtype() }

// aggregate reports whether the expression holds an aggregate.
func (n collateNode) aggregate() bool { return n.inner.aggregate() }

// concatNode joins the text of its arguments, each NULL taken as the empty
// text, as Django's Concat does with ConcatPair and COALESCE.
type concatNode struct {
	args []sqlNode
}

// render writes "(COALESCE(a, ”) || COALESCE(b, ”) ...)".
func (n concatNode) render(w *sqlWriter) {
	w.write("(")
	for i, a := range n.args {
		if i > 0 {
			w.write(" || ")
		}
		w.write("COALESCE(")
		a.render(w)
		w.write(", '')")
	}
	w.write(")")
}

// vtype is string.
func (concatNode) vtype() valueType { return typeString }

// aggregate reports whether an argument holds an aggregate.
func (n concatNode) aggregate() bool { return anyAggregate(n.args) }

// convertNode converts inner, a value of kind input, to the type of the field
// to: the node of Cast. A dialect that is a CastDialect writes the
// conversions that its CAST lacks.
type convertNode struct {
	inner sqlNode
	to    *Field
	input Kind
}

// render writes the conversion of the dialect, or CAST(inner AS type).
func (n convertNode) render(w *sqlWriter) {
	to := *n.to
	if to.MaxLength == 0 && fieldValueType(&to) == typeString && to.Kind != KindGenericIPAddress {
		to.Kind = KindText
	}
	typ := castType(w.d, &to)
	if cd, ok := w.d.(CastDialect); ok {
		if tmpl, params, ok := cd.CastSQL(n.input, to.Kind, typ, TransformTimeZone); ok {
			renderTemplate(w, tmpl, params, n.inner)
			return
		}
	}
	castNode{node: n.inner, typ: typ}.render(w)
}

// vtype is the type the values of the field are read as.
func (n convertNode) vtype() valueType { return fieldValueType(n.to) }

// aggregate reports whether the converted value holds an aggregate.
func (n convertNode) aggregate() bool { return n.inner.aggregate() }

// valueKind returns the field kind whose values are read as t: the kind of
// the value a Cast converts.
func valueKind(t valueType) Kind {
	switch t {
	case typeInt:
		return KindBigInt
	case typeFloat:
		return KindFloat
	case typeBool:
		return KindBool
	case typeTime:
		return KindDateTime
	case typeDecimal:
		return KindDecimal
	case typeDate:
		return KindDate
	case typeTimeOfDay:
		return KindTime
	case typeDuration:
		return KindDuration
	case typeUUID:
		return KindUUID
	case typeJSON:
		return KindJSON
	case typeBinary:
		return KindBinary
	default:
		return KindText
	}
}

// nodeParts returns the expressions that a function, a Cast or a Case is
// made of.
func nodeParts(n sqlNode) []sqlNode {
	switch v := n.(type) {
	case callNode:
		return v.args
	case concatNode:
		return v.args
	case convertNode:
		return []sqlNode{v.inner}
	case caseNode:
		return v.parts()
	default:
		return nil
	}
}

// likeExprNode is the LIKE pattern built from an expression: the value of
// inner with "\", "%", and "_" escaped by a backslash, between the "%"
// wildcards that prefix and suffix ask for. The wildcards are SQL literals,
// never bound values, so that the database sees the whole pattern.
type likeExprNode struct {
	inner          sqlNode
	prefix, suffix bool
}

// render writes the pattern, parenthesized so that the concatenation binds
// before the LIKE that uses it.
func (n likeExprNode) render(w *sqlWriter) {
	w.write("(")
	if n.prefix {
		w.write("'%' || ")
	}
	w.write("REPLACE(REPLACE(REPLACE(")
	n.inner.render(w)
	w.write(`, '\', '\\'), '%', '\%'), '_', '\_')`)
	if n.suffix {
		w.write(" || '%'")
	}
	w.write(")")
}

// vtype is string: a pattern is text.
func (likeExprNode) vtype() valueType { return typeString }

// aggregate reports whether the expression holds an aggregate.
func (n likeExprNode) aggregate() bool { return n.inner.aggregate() }

// aggNode is an aggregate function call. arg is nil for COUNT(*), extra holds
// the arguments after the first, such as the delimiter of STRING_AGG, order
// the terms of its ORDER BY, and filter the condition of a Filter, nil
// without one.
type aggNode struct {
	fn       string
	arg      sqlNode
	extra    []sqlNode
	distinct bool
	order    []orderItem
	filter   sqlNode
}

// render writes "FN(DISTINCT arg, extra ORDER BY order)" or "FN(*)",
// followed by "FILTER (WHERE condition)" for a filter. A decimal aggregate is
// cast to NUMERIC on SQLite, as for a funcNode.
func (n aggNode) render(w *sqlWriter) {
	if n.vtype() == typeDecimal && w.d.Name() == "sqlite" {
		w.write("CAST(")
		defer w.write(" AS NUMERIC)")
	}
	w.write(n.fn + "(")
	if n.distinct {
		w.write("DISTINCT ")
	}
	if n.arg == nil {
		w.write("*")
	} else {
		n.arg.render(w)
	}
	for _, x := range n.extra {
		w.write(", ")
		x.render(w)
	}
	for i, o := range n.order {
		if i == 0 {
			w.write(" ORDER BY ")
		} else {
			w.write(", ")
		}
		o.node.render(w)
		if o.desc {
			w.write(" DESC")
		} else {
			w.write(" ASC")
		}
		switch {
		case o.nullsLast:
			w.write(" NULLS LAST")
		case o.nullsFirst:
			w.write(" NULLS FIRST")
		}
	}
	w.write(")")
	if n.filter != nil {
		w.write(" FILTER (WHERE ")
		n.filter.render(w)
		w.write(")")
	}
}

// vtype is int64 for COUNT, string for STRING_AGG and the argument's type for
// SUM, MIN, MAX and ANY_VALUE. AVG keeps a decimal or a duration argument's
// type and the deviations and variances a decimal one, and they are float64
// for the others, as Django's NumericOutputFieldMixin resolves it.
func (n aggNode) vtype() valueType {
	switch n.fn {
	case "COUNT":
		return typeInt
	case fnStringAgg:
		return typeString
	case "AVG", "STDDEV_POP", "STDDEV_SAMP", "VAR_POP", "VAR_SAMP":
		if n.arg != nil {
			t := n.arg.vtype()
			if t == typeDecimal || (t == typeDuration && n.fn == "AVG") {
				return t
			}
		}
		return typeFloat
	default:
		if n.arg == nil {
			return typeUnknown
		}
		return n.arg.vtype()
	}
}

// aggregate is true.
func (aggNode) aggregate() bool { return true }

// emptyAggNode is an aggregate whose filter no row can meet. It is the value
// of the aggregate over no rows, 0 for COUNT and NULL for the others, as
// Django compiles it from empty_result_set_value.
type emptyAggNode struct {
	agg aggNode
}

// render writes 0 or NULL.
func (n emptyAggNode) render(w *sqlWriter) {
	if n.agg.fn == "COUNT" {
		w.write("0")
	} else {
		w.write("NULL")
	}
}

// vtype is the type of the aggregate.
func (n emptyAggNode) vtype() valueType { return n.agg.vtype() }

// aggregate is true: the query still groups as for the aggregate.
func (emptyAggNode) aggregate() bool { return true }

// condNode is a condition used as a value: the filter of an aggregate, or the
// column that an aggregate over a subquery filters on.
type condNode struct {
	where *whereNode
}

// render writes the condition.
func (n condNode) render(w *sqlWriter) { n.where.render(w) }

// vtype is bool.
func (condNode) vtype() valueType { return typeBool }

// aggregate reports whether the condition reads an aggregate.
func (n condNode) aggregate() bool { return n.where.containsAggregate() }

// subColNode is a column of a derived table, used by an aggregate computed
// over a subquery.
type subColNode struct {
	table, column string
	typ           valueType
}

// render writes the qualified column of the derived table.
func (n subColNode) render(w *sqlWriter) { w.column(n.table, n.column) }

// vtype is the type of the inner expression.
func (n subColNode) vtype() valueType { return n.typ }

// aggregate is false: the aggregate, if any, was computed inside the subquery.
func (subColNode) aggregate() bool { return false }

// readsJoin reports whether n reads a column of a joined table, as opposed to
// the base table of the query.
func readsJoin(n sqlNode) bool {
	switch n := n.(type) {
	case colNode:
		return n.join.parent != nil
	case binaryNode:
		return readsJoin(n.left) || readsJoin(n.right)
	case aggNode:
		return n.arg != nil && readsJoin(n.arg)
	case transformNode:
		return readsJoin(n.inner)
	case funcNode:
		return readsJoin(n.arg)
	case callNode, concatNode, convertNode, caseNode:
		return slices.ContainsFunc(nodeParts(n), readsJoin)
	case condNode:
		return slices.ContainsFunc(n.where.operands(), readsJoin)
	case typedNode:
		return readsJoin(n.node)
	case notNode:
		return readsJoin(n.inner)
	case subqueryNode:
		return slices.ContainsFunc(n.refs, func(r outerRef) bool { return readsJoin(r.node) })
	default:
		return false
	}
}

// nodeKey returns the SQL text of n and its bound values, which identify the
// expression when duplicates are removed from a GROUP BY, ORDER BY or SELECT
// list. The values take part because two expressions can differ in them
// only: on SQLite the year and the month of a datetime are the same function
// call with another first argument.
func nodeKey(d Dialect, n sqlNode) string {
	w := newSQLWriter(d)
	n.render(w)
	if len(w.args) == 0 {
		return w.String()
	}
	return fmt.Sprintf("%s %#v", w.String(), w.args)
}
