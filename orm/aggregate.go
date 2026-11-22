package orm

import (
	"context"
	"database/sql/driver"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"time"
)

// subqueryAlias names the derived table of an aggregate computed over a
// subquery, as Django names it.
const subqueryAlias = "subquery"

// aggregateSQL returns the statement that computes aggs over the rows of q,
// and the result type of each aggregate.
//
// The aggregates are normally selected straight from the query's tables. A
// query that is already grouped, sliced, or distinct cannot be aggregated that
// way: the rows to aggregate are the result of the query, so it becomes a
// subquery, as in Django's Query.get_aggregation. So does a query whose
// aggregate reads an annotation that holds a subquery.
func (q *query) aggregateSQL(aggs []annotation) (string, []any, []valueType, error) {
	c, err := q.compile()
	if err != nil {
		return "", nil, nil, err
	}
	w := newSQLWriter(c.d)
	types := make([]valueType, len(aggs))
	// The values over no rows are known when every expression has one.
	skip := c.empty && emptyValuesKnown(aggs)

	if !q.hasAggregates() && !q.sliced() && !q.distinct && !slices.ContainsFunc(aggs, func(a annotation) bool {
		return q.refsSubquery(a.expr)
	}) {
		items := make([]selectItem, len(aggs))
		for i, a := range aggs {
			n, err := c.resolveExpr(a.expr)
			if err != nil {
				return "", nil, nil, err
			}
			if !n.aggregate() {
				return "", nil, nil, fmt.Errorf("orm: %s is not an aggregate expression", a.name)
			}
			items[i] = selectItem{node: n, alias: a.name}
			types[i] = n.vtype()
			// A later aggregate of the call may refer to this one by name.
			c.annots[a.name] = n
			c.annotNames = append(c.annotNames, a.name)
		}
		if skip {
			return "", nil, types, errEmptyResultSet
		}
		stmt := &selectStmt{c: c, items: items, limit: -1}
		stmt.render(w)
		return w.String(), w.args, types, nil
	}

	inner, prunable, err := c.aggregateInner()
	if err != nil {
		return "", nil, nil, err
	}
	// innerColumn returns the column of the subquery that selects n: an item
	// that selects it already, or a new one named "__col1", "__col2" and so
	// on, as Django names them.
	aliases := make(map[string]string, len(inner))
	for _, it := range inner {
		if key := nodeKey(c.d, it.node); it.alias != "" && aliases[key] == "" {
			aliases[key] = it.alias
		}
	}
	used := make(map[string]bool)
	cols := 0
	innerColumn := func(n sqlNode) subColNode {
		key := nodeKey(c.d, n)
		col, ok := aliases[key]
		if !ok {
			cols++
			col = "__col" + strconv.Itoa(cols)
			inner = append(inner, selectItem{node: n, alias: col})
			aliases[key] = col
		}
		used[col] = true
		return subColNode{table: subqueryAlias, column: col, typ: n.vtype()}
	}
	// outer binds the expressions of the outer statement: each aggregate is
	// computed over columns of the subquery, and a key outside an aggregate
	// names an aggregate of the call before it.
	outer := make([]selectItem, len(aggs))
	named := make(map[string]sqlNode, len(aggs))
	agg := func(e Expr) (sqlNode, error) {
		return c.outerAggregate(e, named, innerColumn)
	}
	for i, a := range aggs {
		e, err := outerExpr(a.expr, named, agg)
		if err != nil {
			return "", nil, nil, err
		}
		n, err := c.resolveExpr(e)
		if err != nil {
			return "", nil, nil, err
		}
		if !n.aggregate() {
			return "", nil, nil, fmt.Errorf("orm: %s is not an aggregate expression", a.name)
		}
		outer[i] = selectItem{node: n, alias: a.name}
		types[i] = n.vtype()
		named[a.name] = n
	}
	inner = c.pruneInner(inner, prunable, used)
	// The order of the inner rows only matters when the query is sliced.
	sub, err := c.build(inner, q.sliced())
	if err != nil {
		return "", nil, nil, err
	}
	if skip {
		return "", nil, types, errEmptyResultSet
	}
	sub.forUpdate = false
	// The outer columns are read by position, so they carry no alias, as in
	// Django's SQLAggregateCompiler.
	w.write("SELECT ")
	for i, it := range outer {
		if i > 0 {
			w.write(", ")
		}
		it.node.render(w)
	}
	w.write(" FROM (")
	sub.render(w)
	w.write(") ")
	w.ident(subqueryAlias)
	return w.String(), w.args, types, nil
}

// aggregateInner returns the items that the subquery of aggregateSQL selects
// before the columns of the aggregates are added, and the aliases of the
// annotations among them that pruneInner may leave out. A Values query
// selects its keys, a distinct query every field and annotation, since
// DISTINCT compares them all, and any other query its annotations only, as
// Django's get_aggregation drops the default columns.
func (c *compiler) aggregateInner() ([]selectItem, map[string]bool, error) {
	q := c.q
	prunable := make(map[string]bool)
	if q.values != nil {
		items, _, err := c.valuesItems()
		if err != nil || q.distinct {
			return items, prunable, err
		}
		for _, name := range c.annotNames[q.values.annotated:] {
			if !slices.Contains(q.values.keys, name) {
				prunable[name] = true
			}
		}
		return items, prunable, nil
	}
	var items []selectItem
	if q.distinct {
		for _, f := range q.model.fields {
			items = append(items, selectItem{node: c.baseColumn(f)})
		}
	}
	for _, name := range c.annotNames {
		items = append(items, selectItem{node: c.annots[name], alias: name})
		prunable[name] = !q.distinct
	}
	return items, prunable, nil
}

// pruneInner leaves out of items each prunable annotation that the
// aggregates do not use and that adds no column to the GROUP BY, as Django's
// get_aggregation masks the annotations of its inner query. The rows of the
// subquery stay the same. Without any item left it selects the primary key.
func (c *compiler) pruneInner(items []selectItem, prunable, used map[string]bool) []selectItem {
	kept := make([]selectItem, 0, len(items))
	for _, it := range items {
		if prunable[it.alias] && !used[it.alias] && len(columnsOf(it.node)) == 0 {
			continue
		}
		kept = append(kept, it)
	}
	if len(kept) == 0 {
		kept = append(kept, selectItem{node: c.baseColumn(c.q.model.pk)})
	}
	return kept
}

// outerExpr returns e with each aggregate replaced by the node that agg binds
// it to, and each key by the aggregate of named it names, so that resolving
// the result reads no column of the query. It refuses a key that names no
// aggregate of named, and an expression that holds an aggregate in a part
// other than an operand or a function argument.
func outerExpr(e Expr, named map[string]sqlNode, agg func(Expr) (sqlNode, error)) (Expr, error) {
	switch e.kind {
	case exprAggregate:
		n, err := agg(e)
		return Expr{kind: exprNode, node: n}, err
	case exprField:
		n, ok := named[e.key]
		if !ok {
			return Expr{}, fmt.Errorf("orm: %s is not an aggregate of the call", e.key)
		}
		return Expr{kind: exprNode, node: n}, nil
	case exprBinary:
		left, err := outerExpr(*e.left, named, agg)
		if err != nil {
			return Expr{}, err
		}
		right, err := outerExpr(*e.right, named, agg)
		if err != nil {
			return Expr{}, err
		}
		e.left, e.right = &left, &right
		return e, nil
	case exprFunc, exprCast, exprWrap:
		arg, err := outerExpr(*e.arg, named, agg)
		if err != nil {
			return Expr{}, err
		}
		e.arg = &arg
		return e, nil
	case exprCall:
		args := make([]Expr, len(e.args))
		for i, a := range e.args {
			var err error
			if args[i], err = outerExpr(a, named, agg); err != nil {
				return Expr{}, err
			}
		}
		e.args = args
		return e, nil
	case exprValue:
		return e, nil
	default:
		if e.containsAggregate() {
			return Expr{}, errors.New(
				"orm: this expression cannot be computed over an annotated, sliced, or distinct queryset")
		}
		return e, nil
	}
}

// outerAggregate binds the aggregate e of the statement that aggregates over
// the subquery of an annotated, sliced, or distinct query: its argument, its
// ordering and the condition of its Filter are columns that innerColumn
// selects in the subquery, and its Default is bound as an outer expression.
// An argument that names an aggregate of named is refused, as in a plain
// aggregate.
func (c *compiler) outerAggregate(e Expr, named map[string]sqlNode, innerColumn func(sqlNode) subColNode) (sqlNode, error) {
	if err := checkAggregate(e); err != nil {
		return nil, err
	}
	agg := aggNode{fn: e.fn, distinct: e.distinct}
	if e.arg.kind == exprStar {
		if err := checkStar(e); err != nil {
			return nil, err
		}
	} else {
		if _, ok := named[e.arg.key]; ok && e.arg.kind == exprField {
			return nil, c.aggregateOfAggregate(e)
		}
		arg, err := c.resolveExpr(*e.arg)
		if err != nil {
			return nil, err
		}
		agg.arg = innerColumn(arg)
	}
	extra, err := c.resolveAggregateArgs(e)
	if err != nil {
		return nil, err
	}
	for _, x := range extra {
		agg.extra = append(agg.extra, innerColumn(x))
	}
	order, err := c.aggregateOrder(e.order)
	if err != nil {
		return nil, err
	}
	for _, o := range order {
		o.node = innerColumn(o.node)
		agg.order = append(agg.order, o)
	}
	where, match, err := c.condition(e.filter)
	if err != nil {
		return nil, err
	}
	var n sqlNode = agg
	switch match {
	case matchNone:
		n = emptyAggNode{agg: agg}
	case matchSome:
		agg.filter = condNode{where: outerWhere(where, innerColumn)}
		n = agg
	}
	if e.def == nil {
		return n, nil
	}
	def, err := outerExpr(*e.def, named, func(x Expr) (sqlNode, error) {
		return c.outerAggregate(x, named, innerColumn)
	})
	if err != nil {
		return nil, err
	}
	e.def = &def
	return c.aggregateDefault(e, n)
}

// outerWhere returns the filter of an aggregate that is computed over a
// subquery, as Django's get_aggregation repoints it: the subquery selects the
// operands of each condition, and the outer aggregate compares those columns,
// so that an operand the subquery selects already, such as a subquery
// annotation, is not computed twice. A condition with a subquery of its own
// is selected by the subquery as a whole.
func outerWhere(n *whereNode, innerColumn func(sqlNode) subColNode) *whereNode {
	out := &whereNode{conn: n.conn, negated: n.negated, children: make([]whereChild, len(n.children))}
	for i, ch := range n.children {
		switch {
		case ch.node != nil:
			out.children[i] = whereChild{node: outerWhere(ch.node, innerColumn)}
		case ch.pred.sub != nil:
			col := innerColumn(condNode{where: &whereNode{conn: connAnd, children: []whereChild{ch}}})
			out.children[i] = whereChild{pred: &predicate{lookup: lookupExpr, lhs: col}}
		default:
			p := *ch.pred
			if p.lhs != nil {
				p.lhs = innerColumn(p.lhs)
			}
			p.rhs = make([]sqlNode, len(ch.pred.rhs))
			for k, r := range ch.pred.rhs {
				if _, ok := r.(paramNode); ok {
					p.rhs[k] = r
				} else {
					p.rhs[k] = innerColumn(r)
				}
			}
			out.children[i] = whereChild{pred: &p}
		}
	}
	return out
}

// emptyValuesKnown reports whether the value of every expression over no rows
// is known, so that an aggregate over a query known to match nothing runs no
// statement.
func emptyValuesKnown(aggs []annotation) bool {
	for _, a := range aggs {
		if _, ok := emptyValue(a.expr); !ok {
			return false
		}
	}
	return true
}

// emptyValue returns the value of e over no rows, Django's
// empty_result_set_value: zero for Count, the value over no rows of the
// Default of another aggregate and NULL without one, a literal itself, and
// the first value of a Coalesce that is not NULL. It reports false for the
// other expressions, whose value only the database gives.
func emptyValue(e Expr) (any, bool) {
	switch e.kind {
	case exprAggregate:
		switch {
		case e.fn == "COUNT":
			return int64(0), true
		case e.def != nil:
			return emptyValue(*e.def)
		}
		return nil, true
	case exprValue:
		return e.value, true
	case exprCall:
		if e.fn != "COALESCE" {
			return nil, false
		}
		for _, a := range e.args {
			if v, ok := emptyValue(a); !ok || v != nil {
				return v, ok
			}
		}
		return nil, true
	}
	return nil, false
}

// emptyResult converts the value over no rows v to the Go type of t, as a
// value the database returns for t is converted.
func emptyResult(v any, t valueType) (any, error) {
	if d, ok := v.(time.Duration); ok && t == typeDuration {
		return d, nil
	}
	v, err := driver.DefaultParameterConverter.ConvertValue(v)
	if err != nil {
		return nil, err
	}
	return convertValue(v, t)
}

// aggregate runs the aggregates over the rows of q and returns their values
// by name.
func (q *query) aggregate(ctx context.Context, aggs []annotation) (map[string]any, error) {
	query, args, types, err := q.aggregateSQL(aggs)
	if errors.Is(err, errEmptyResultSet) {
		out := make(map[string]any, len(aggs))
		for i, a := range aggs {
			v, _ := emptyValue(a.expr)
			if out[a.name], err = emptyResult(v, types[i]); err != nil {
				return nil, fmt.Errorf("orm: aggregate %s: %s: %w", q.model.Name(), a.name, err)
			}
		}
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	rows, err := q.db.conn(ctx).QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("orm: aggregate %s: %w", q.model.Name(), err)
	}
	defer func() { _ = rows.Close() }()
	raw := make([]any, len(aggs))
	targets := make([]any, len(aggs))
	for i := range raw {
		targets[i] = &raw[i]
	}
	if rows.Next() {
		if err := rows.Scan(targets...); err != nil {
			return nil, fmt.Errorf("orm: aggregate %s: %w", q.model.Name(), err)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("orm: aggregate %s: %w", q.model.Name(), err)
	}
	out := make(map[string]any, len(aggs))
	for i, a := range aggs {
		v, err := convertValue(raw[i], types[i])
		if err != nil {
			return nil, fmt.Errorf("orm: aggregate %s: %s: %w", q.model.Name(), a.name, err)
		}
		out[a.name] = v
	}
	return out, nil
}

// countAlias names the COUNT(*) column of a Count query, as Django names it.
const countAlias = "__count"

// count returns the number of rows q yields.
func (q *query) count(ctx context.Context) (int64, error) {
	res, err := q.aggregate(ctx, []annotation{{name: countAlias, expr: Count("*")}})
	if err != nil {
		return 0, err
	}
	n, _ := res[countAlias].(int64)
	return n, nil
}

// Aggregate computes aggregates over the rows of the queryset and returns
// their values by name, like Django's aggregate(). The arguments are
// alternating names and aggregate expressions; an aggregate over a plain
// field may be passed without a name and is then named "<key>__<function>":
//
//	res, err := qs.Aggregate(ctx, orm.Avg("age"), "oldest", orm.Max("age"))
//	// res["age__avg"], res["oldest"]
//
// The Go type of a value is the same on every dialect: Count is an int64, Avg
// a float64, Sum an int64 for an integer field (a float64 when its argument
// is a float expression), and Min and Max have the type of their field
// (int64, string, bool, or time.Time). Every aggregate but Count is nil when
// no row matches.
//
// On a queryset that is annotated with aggregates, sliced, or distinct, the
// aggregates are computed over the queryset's result, through a subquery, and
// each must then be a plain aggregate such as orm.Avg("n").
func (qs *QuerySet[T]) Aggregate(ctx context.Context, args ...any) (map[string]any, error) {
	return qs.q.runAggregate(ctx, args)
}

// Aggregate computes aggregates over the rows of the queryset; see
// QuerySet.Aggregate.
func (vs *ValuesQuerySet) Aggregate(ctx context.Context, args ...any) (map[string]any, error) {
	return vs.q.runAggregate(ctx, args)
}

// runAggregate parses the arguments of Aggregate and runs them.
func (q *query) runAggregate(ctx context.Context, args []any) (map[string]any, error) {
	if q.err != nil {
		return nil, q.err
	}
	aggs, err := parseAnnotations(args, "aggregates")
	if err != nil {
		return nil, err
	}
	if len(aggs) == 0 {
		return map[string]any{}, nil
	}
	return q.aggregate(ctx, aggs)
}
