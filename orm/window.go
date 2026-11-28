package orm

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// Window evaluates expr over a window of rows, Django's Window: an aggregate
// such as Sum or Avg, or a window function such as Rank or Lag, with an OVER
// clause. PartitionBy splits the rows into partitions, OrderBy orders the
// rows of each partition and Frame limits the rows the value is computed
// over:
//
//	Annotate("rank", orm.Window(orm.Rank()).PartitionBy("department").OrderBy("-salary"))
//
// A window has no GROUP BY of its own: in a query with aggregates, the
// partition and order terms are grouped by, not the source. A query that
// filters against a window, or an Update that assigns one, fails with an
// error, and so does a source that is neither an aggregate nor a window
// function.
func Window(expr Expr) Expr { return Expr{kind: exprWindow, arg: &expr} }

// PartitionBy splits the rows of a Window into partitions, Django's
// partition_by: the value of each row is computed over the rows of its
// partition. Each term is a field key or an Expr. PartitionBy is an error on
// an expression that is not a Window.
func (e Expr) PartitionBy(terms ...any) Expr {
	e.partition = make([]Expr, len(terms))
	for i, t := range terms {
		e.partition[i] = fieldOrExpr(t)
	}
	return e
}

// Frame limits the rows of the partition that the value of a Window is
// computed over, Django's frame: a RowRange or a ValueRange. Frame is an
// error on an expression that is not a Window.
func (e Expr) Frame(f WindowFrame) Expr {
	e.frame = &f
	return e
}

// WindowFrame is the frame clause of a Window, Django's RowRange and
// ValueRange. A bound is an int: a negative one counts the rows or values
// before the current row (PRECEDING), a positive one those after it
// (FOLLOWING) and zero is the current row. A nil start is UNBOUNDED
// PRECEDING and a nil end UNBOUNDED FOLLOWING. A query with a bound of
// another type fails with an error, as Django raises a ValueError.
type WindowFrame struct {
	frameType  string
	start, end any
	exclusion  WindowFrameExclusion
}

// RowRange is a frame of rows, Django's RowRange: ROWS BETWEEN start AND end.
// RowRange(-1, 1) is the row before the current row, the current row and the
// row after it.
func RowRange(start, end any) WindowFrame {
	return WindowFrame{frameType: "ROWS", start: start, end: end}
}

// ValueRange is a frame of the rows whose ordering value is in a range of
// the current row's, Django's ValueRange: RANGE BETWEEN start AND end. The
// start is negative, zero or nil and the end positive, zero or nil. Django
// refuses an offset other than zero on PostgreSQL; the ORM allows it there,
// as PostgreSQL supports it since version 11.
func ValueRange(start, end any) WindowFrame {
	return WindowFrame{frameType: "RANGE", start: start, end: end}
}

// Exclusion leaves rows out of the frame, Django's exclusion argument:
// ExcludeCurrentRow, ExcludeGroup, ExcludeTies or ExcludeNoOthers.
func (f WindowFrame) Exclusion(x WindowFrameExclusion) WindowFrame {
	f.exclusion = x
	return f
}

// String returns the frame clause, such as "ROWS BETWEEN 1 PRECEDING AND
// UNBOUNDED FOLLOWING", Django's str() of a frame. It does not check the
// bounds.
func (f WindowFrame) String() string {
	start, end := "UNBOUNDED PRECEDING", "UNBOUNDED FOLLOWING"
	if n, ok := f.start.(int); ok {
		start = frameBound(n)
	}
	if n, ok := f.end.(int); ok {
		end = frameBound(n)
	}
	return f.clause(start, end)
}

// clause returns the frame clause with the given bounds.
func (f WindowFrame) clause(start, end string) string {
	s := f.frameType + " BETWEEN " + start + " AND " + end
	if f.exclusion != 0 {
		s += " EXCLUDE " + f.exclusion.String()
	}
	return s
}

// sql returns the frame clause, or the error of a bound that Django's
// window_frame_rows_start_end or window_frame_range_start_end refuses.
func (f WindowFrame) sql() (string, error) {
	start, startInt := f.start.(int)
	end, endInt := f.end.(int)
	if f.frameType == "RANGE" {
		if (f.start != nil && !startInt) || (startInt && start > 0) {
			return "", fmt.Errorf("orm: start argument must be a negative integer, zero, or nil, but got '%v'.", f.start) //nolint:revive,staticcheck // Django's message
		}
		if (f.end != nil && !endInt) || (endInt && end < 0) {
			return "", fmt.Errorf("orm: end argument must be a positive integer, zero, or nil, but got '%v'.", f.end) //nolint:revive,staticcheck // Django's message
		}
	} else {
		if startInt && endInt && start > end {
			return "", errors.New("orm: start cannot be greater than end.") //nolint:revive,staticcheck // Django's message
		}
		if f.start != nil && !startInt {
			return "", fmt.Errorf("orm: start argument must be an integer, zero, or nil, but got '%v'.", f.start) //nolint:revive,staticcheck // Django's message
		}
		if f.end != nil && !endInt {
			return "", fmt.Errorf("orm: end argument must be an integer, zero, or nil, but got '%v'.", f.end) //nolint:revive,staticcheck // Django's message
		}
	}
	return f.String(), nil
}

// frameBound returns the SQL of the frame bound n: "n PRECEDING" for a
// negative n, "CURRENT ROW" for zero and "n FOLLOWING" for a positive n.
func frameBound(n int) string {
	switch {
	case n < 0:
		return strconv.Itoa(-n) + " PRECEDING"
	case n == 0:
		return "CURRENT ROW"
	default:
		return strconv.Itoa(n) + " FOLLOWING"
	}
}

// WindowFrameExclusion names the rows that a frame leaves out, Django's
// WindowFrameExclusion.
type WindowFrameExclusion uint8

const (
	// ExcludeCurrentRow leaves out the current row.
	ExcludeCurrentRow WindowFrameExclusion = iota + 1
	// ExcludeGroup leaves out the current row and its peers, the rows with
	// the same ordering value.
	ExcludeGroup
	// ExcludeTies leaves out the peers of the current row, but not the row
	// itself.
	ExcludeTies
	// ExcludeNoOthers leaves out no row, the default.
	ExcludeNoOthers
)

// String returns the SQL of the exclusion after EXCLUDE, such as "CURRENT
// ROW".
func (x WindowFrameExclusion) String() string {
	switch x {
	case ExcludeCurrentRow:
		return "CURRENT ROW"
	case ExcludeGroup:
		return "GROUP"
	case ExcludeTies:
		return "TIES"
	case ExcludeNoOthers:
		return "NO OTHERS"
	default:
		return "WindowFrameExclusion(" + strconv.Itoa(int(x)) + ")"
	}
}

// The SQL functions of the window functions.
const (
	fnRowNumber   = "ROW_NUMBER"
	fnRank        = "RANK"
	fnDenseRank   = "DENSE_RANK"
	fnCumeDist    = "CUME_DIST"
	fnPercentRank = "PERCENT_RANK"
	fnNtile       = "NTILE"
	fnFirstValue  = "FIRST_VALUE"
	fnLastValue   = "LAST_VALUE"
	fnNthValue    = "NTH_VALUE"
	fnLag         = "LAG"
	fnLead        = "LEAD"
)

// windowFunc builds a call of the window function fn. A typed call has a
// result of kind output; the others have the kind of their first argument.
func windowFunc(fn string, output Kind, typed bool, args ...Expr) Expr {
	return Expr{kind: exprWindowFunc, fn: fn, output: output, typed: typed, args: args}
}

// intLiteral is the integer n written into the SQL, as the offsets and
// counts of the window functions are, rather than bound as a parameter.
func intLiteral(n int) Expr {
	return Expr{kind: exprNode, node: rawNode{sql: strconv.Itoa(n), typ: typeInt}}
}

// windowSource returns expr as the first argument of a window function, or
// the error of a nil expr, in the wording of Django's message for name.
func windowSource(name string, expr any) (Expr, error) {
	if expr == nil {
		return Expr{}, fmt.Errorf("orm: %s requires a non-null source expression.", name) //nolint:revive,staticcheck // Django's message
	}
	return fieldOrExpr(expr), nil
}

// RowNumber is the number of the row in its partition, starting at 1,
// Django's RowNumber, ROW_NUMBER() in SQL. It is valid only as the source of
// a Window. The result is an int64.
func RowNumber() Expr { return windowFunc(fnRowNumber, KindInt, true) }

// Rank is the rank of the row in its partition, with gaps after ties,
// Django's Rank, RANK() in SQL. It is valid only as the source of a Window.
// The result is an int64.
func Rank() Expr { return windowFunc(fnRank, KindInt, true) }

// DenseRank is the rank of the row in its partition, without gaps after
// ties, Django's DenseRank, DENSE_RANK() in SQL. It is valid only as the
// source of a Window. The result is an int64.
func DenseRank() Expr { return windowFunc(fnDenseRank, KindInt, true) }

// CumeDist is the share of the rows of the partition that come before the
// row or tie with it, Django's CumeDist, CUME_DIST() in SQL. It is valid only
// as the source of a Window. The result is a float64.
func CumeDist() Expr { return windowFunc(fnCumeDist, KindFloat, true) }

// PercentRank is the relative rank of the row in its partition, (rank - 1) /
// (rows - 1), Django's PercentRank, PERCENT_RANK() in SQL. It is valid only
// as the source of a Window. The result is a float64.
func PercentRank() Expr { return windowFunc(fnPercentRank, KindFloat, true) }

// Ntile is the number of the bucket of the row, when the rows of the
// partition are split into numBuckets buckets of about the same size,
// Django's Ntile, NTILE(n) in SQL. It is valid only as the source of a
// Window. The result is an int64. A query with numBuckets below 1 fails with
// an error.
func Ntile(numBuckets int) Expr {
	e := windowFunc(fnNtile, KindInt, true, intLiteral(numBuckets))
	if numBuckets <= 0 {
		e.err = errors.New("orm: num_buckets must be greater than 0.") //nolint:revive,staticcheck // Django's message
	}
	return e
}

// FirstValue is the value of expr in the first row of the frame, Django's
// FirstValue, FIRST_VALUE in SQL. expr is a field key or an Expr. It is valid
// only as the source of a Window, and the result has the Go type of expr.
func FirstValue(expr any) Expr { return windowFunc(fnFirstValue, 0, false, fieldOrExpr(expr)) }

// LastValue is the value of expr in the last row of the frame, Django's
// LastValue, LAST_VALUE in SQL. With an ordering and no Frame, the frame
// ends at the current row and its peers. expr is a field key or an Expr. It
// is valid only as the source of a Window, and the result has the Go type of
// expr.
func LastValue(expr any) Expr { return windowFunc(fnLastValue, 0, false, fieldOrExpr(expr)) }

// NthValue is the value of expr in row nth of the frame, counted from 1, and
// nil when the frame has fewer rows, Django's NthValue, NTH_VALUE in SQL.
// expr is a field key or an Expr. It is valid only as the source of a
// Window, and the result has the Go type of expr. A query with a nil expr or
// with nth below 1 fails with an error.
func NthValue(expr any, nth int) Expr {
	source, err := windowSource("NthValue", expr)
	e := windowFunc(fnNthValue, 0, false, source, intLiteral(nth))
	if err == nil && nth <= 0 {
		err = errors.New("orm: NthValue requires a positive integer as for nth.") //nolint:revive,staticcheck // Django's message
	}
	e.err = err
	return e
}

// Lag is the value of expr in the row offset rows before the current row of
// the partition, Django's Lag, LAG in SQL. Default sets the value for a row
// that has no such row; it is nil without one. expr is a field key or an
// Expr. It is valid only as the source of a Window, and the result has the
// Go type of expr. A query with a nil expr or with an offset below 1 fails
// with an error.
func Lag(expr any, offset int) Expr { return lagLead(fnLag, "Lag", expr, offset) }

// Lead is the value of expr in the row offset rows after the current row of
// the partition, Django's Lead, LEAD in SQL. It is otherwise like Lag:
//
//	orm.Window(orm.Lead("salary", 1).Default(0)).OrderBy("hire_date")
func Lead(expr any, offset int) Expr { return lagLead(fnLead, "Lead", expr, offset) }

// lagLead builds a call of LAG or LEAD, Django's LagLeadFunction.
func lagLead(fn, name string, expr any, offset int) Expr {
	source, err := windowSource(name, expr)
	e := windowFunc(fn, 0, false, source, intLiteral(offset))
	if err == nil && offset <= 0 {
		err = fmt.Errorf("orm: %s requires a positive integer for the offset.", name) //nolint:revive,staticcheck // Django's message
	}
	e.err = err
	return e
}

// windowFuncName returns the name of the Django class of the window
// function fn.
func windowFuncName(fn string) string {
	var b strings.Builder
	for part := range strings.SplitSeq(strings.ToLower(fn), "_") {
		b.WriteString(strings.ToUpper(part[:1]) + part[1:])
	}
	return b.String()
}

// exprClassName returns the name of the Django class of e, as Django's
// messages name an expression.
func exprClassName(e Expr) string {
	switch e.kind {
	case exprField:
		return "F"
	case exprValue:
		return "Value"
	case exprBinary:
		return "CombinedExpression"
	case exprAggregate:
		return aggregateName(e.fn)
	case exprFunc, exprCall, exprWindowFunc:
		return windowFuncName(e.fn)
	case exprCast:
		return "Cast"
	case exprCase:
		return "Case"
	case exprNot:
		return "NegatedExpression"
	case exprWrap:
		return "ExpressionWrapper"
	case exprSubquery:
		if e.exists {
			return "Exists"
		}
		return "Subquery"
	case exprOuterRef:
		return "OuterRef"
	case exprWindow:
		return "Window"
	default:
		return "Expression"
	}
}

// resolveWindow binds a Window: its source, which must be an aggregate or a
// window function, and its partition, order and frame. The window is not an
// aggregate, so a query that holds only windows has no GROUP BY.
func (c *compiler) resolveWindow(e Expr) (sqlNode, error) {
	var source sqlNode
	var err error
	switch e.arg.kind {
	case exprAggregate:
		source, err = c.resolveExpr(*e.arg)
	case exprWindowFunc:
		source, err = c.resolveWindowFunc(*e.arg)
	default:
		if e.arg.err != nil {
			return nil, e.arg.err
		}
		return nil, fmt.Errorf("orm: Expression '%s' isn't compatible with OVER clauses.", exprClassName(*e.arg)) //nolint:revive,staticcheck // Django's message
	}
	if err != nil {
		return nil, err
	}
	n := windowNode{source: source}
	for _, p := range e.partition {
		pn, err := c.resolveExpr(p)
		if err != nil {
			return nil, err
		}
		n.partition = append(n.partition, pn)
	}
	for _, t := range e.order {
		switch t.(type) {
		case string, Expr, OrderExpr:
		default:
			return nil, fmt.Errorf("orm: Window.OrderBy must be either a string reference to a field or an expression, not %v.", t) //nolint:revive,staticcheck // Django's message
		}
	}
	if n.order, err = c.windowOrder(e.order); err != nil {
		return nil, err
	}
	if e.frame != nil {
		if n.frame, err = e.frame.sql(); err != nil {
			return nil, err
		}
	}
	return n, nil
}

// windowOrder binds the terms of a Window's OrderBy, Django's OrderByList: a
// key, "-" and a key, an Expr or an OrderExpr. Unlike the ordering of an
// aggregate, a term can hold an aggregate, which the GROUP BY of the query
// computes.
func (c *compiler) windowOrder(terms []any) ([]orderItem, error) {
	out := make([]orderItem, 0, len(terms))
	for _, t := range terms {
		var item orderItem
		var x Expr
		switch v := t.(type) {
		case string:
			key := v
			if strings.HasPrefix(key, "-") {
				key, item.desc = key[1:], true
			}
			x = F(key)
		case Expr:
			x = v
		case OrderExpr:
			if v.err != nil {
				return nil, v.err
			}
			x = v.expr
			item.desc, item.nullsFirst, item.nullsLast = v.desc, v.nullsFirst, v.nullsLast
		}
		n, err := c.resolveExpr(x)
		if err != nil {
			return nil, err
		}
		item.node = n
		out = append(out, item)
	}
	return out, nil
}

// resolveWindowFunc binds a call of a window function. The arguments after
// the first are integer literals, and the Default of Lag and Lead is the
// last argument.
func (c *compiler) resolveWindowFunc(e Expr) (sqlNode, error) {
	if e.err != nil {
		return nil, e.err
	}
	args := e.args
	if e.def != nil {
		args = append(args[:len(args):len(args)], *e.def)
	}
	n := windowFuncNode{fn: e.fn, args: make([]sqlNode, len(args))}
	for i, a := range args {
		an, err := c.resolveExpr(a)
		if err != nil {
			return nil, err
		}
		n.args[i] = an
	}
	switch {
	case e.typed:
		n.typ = fieldValueType(&Field{Kind: e.output})
	case len(n.args) > 0:
		n.typ = n.args[0].vtype()
	}
	return n, nil
}

// windowNode is an expression evaluated over a window of rows: "source OVER
// (PARTITION BY partition ORDER BY order frame)".
type windowNode struct {
	source    sqlNode
	partition []sqlNode
	order     []orderItem
	frame     string
}

// render writes the window. A decimal window is cast to NUMERIC on SQLite
// outside the OVER clause, with its aggregate left uncast, as Django's
// Window.as_sqlite does.
func (n windowNode) render(w *sqlWriter) {
	cast := n.vtype() == typeDecimal && w.d.Name() == "sqlite"
	if cast {
		w.write("CAST(")
	}
	if a, ok := n.source.(aggNode); ok {
		a.renderCall(w)
	} else {
		n.source.render(w)
	}
	w.write(" OVER (")
	sep := ""
	for i, p := range n.partition {
		if i == 0 {
			w.write("PARTITION BY ")
		} else {
			w.write(", ")
		}
		p.render(w)
		sep = " "
	}
	if len(n.order) > 0 {
		w.write(sep)
		writeOrderTerms(w, n.order)
		sep = " "
	}
	if n.frame != "" {
		w.write(sep + n.frame)
	}
	w.write(")")
	if cast {
		w.write(" AS NUMERIC)")
	}
}

// vtype is the type of the source.
func (n windowNode) vtype() valueType { return n.source.vtype() }

// aggregate is false: a window is computed after the GROUP BY, over the
// grouped rows, as Django's Window has contains_aggregate False.
func (windowNode) aggregate() bool { return false }

// windowFuncNode is a call of a window function, such as RANK() or
// LAG(salary, 1).
type windowFuncNode struct {
	fn   string
	args []sqlNode
	typ  valueType
}

// render writes "FN(args)". The window casts a decimal value on SQLite, so
// the call is never cast.
func (n windowFuncNode) render(w *sqlWriter) {
	w.write(n.fn + "(")
	for i, a := range n.args {
		if i > 0 {
			w.write(", ")
		}
		a.render(w)
	}
	w.write(")")
}

// vtype is the type of the result.
func (n windowFuncNode) vtype() valueType { return n.typ }

// aggregate is false: a window function is computed over the window.
func (windowFuncNode) aggregate() bool { return false }

// containsOver reports whether n holds a window expression, Django's
// contains_over_clause.
func containsOver(n sqlNode) bool {
	switch n := n.(type) {
	case windowNode, windowFuncNode:
		return true
	case binaryNode:
		return containsOver(n.left) || containsOver(n.right)
	case aggNode:
		return n.arg != nil && containsOver(n.arg)
	case transformNode:
		return containsOver(n.inner)
	case funcNode:
		return containsOver(n.arg)
	case callNode, concatNode, convertNode, caseNode, truncNode:
		return slices.ContainsFunc(nodeParts(n), containsOver)
	case condNode:
		return slices.ContainsFunc(n.where.operands(), containsOver)
	case typedNode:
		return containsOver(n.node)
	case notNode:
		return containsOver(n.inner)
	default:
		return false
	}
}
