package orm

import (
	"context"
	"errors"
	"fmt"
	"strconv"
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
// subquery, as in Django's Query.get_aggregation.
func (q *query) aggregateSQL(aggs []annotation) (string, []any, []valueType, error) {
	c, err := q.compile()
	if err != nil {
		return "", nil, nil, err
	}
	w := newSQLWriter(c.d)
	types := make([]valueType, len(aggs))
	// The values over no rows are known when every aggregate is a plain one.
	skip := c.empty && plainAggregates(aggs)

	if !q.hasAggregates() && !q.sliced() && !q.distinct {
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
			return "", nil, nil, errEmptyResultSet
		}
		stmt := &selectStmt{c: c, items: items, limit: -1}
		stmt.render(w)
		return w.String(), w.args, types, nil
	}

	var inner []selectItem
	if q.values != nil {
		inner, _, err = c.valuesItems()
		if err != nil {
			return "", nil, nil, err
		}
	} else {
		for _, f := range q.model.fields {
			inner = append(inner, selectItem{node: c.baseColumn(f)})
		}
	}
	outer := make([]selectItem, len(aggs))
	// innerColumn selects n in the subquery and returns its column there,
	// named "__col1", "__col2" and so on, as Django names them.
	cols := 0
	innerColumn := func(n sqlNode) subColNode {
		cols++
		col := "__col" + strconv.Itoa(cols)
		inner = append(inner, selectItem{node: n, alias: col})
		return subColNode{table: subqueryAlias, column: col, typ: n.vtype()}
	}
	for i, a := range aggs {
		if a.expr.kind != exprAggregate {
			return "", nil, nil, errors.New(
				"orm: only a plain aggregate can be computed over an annotated, sliced, or distinct queryset")
		}
		agg := aggNode{fn: a.expr.fn, distinct: a.expr.distinct}
		if a.expr.arg.kind == exprStar {
			if a.expr.filter != nil {
				return "", nil, nil, errStarFilter
			}
		} else {
			arg, err := c.resolveExpr(*a.expr.arg)
			if err != nil {
				return "", nil, nil, err
			}
			agg.arg = innerColumn(arg)
		}
		// The subquery selects the condition of a filter, and the outer
		// aggregate filters on that column.
		where, match, err := c.aggregateFilter(a.expr.filter)
		if err != nil {
			return "", nil, nil, err
		}
		var n sqlNode = agg
		switch match {
		case matchNone:
			n = emptyAggNode{agg: agg}
		case matchSome:
			agg.filter = innerColumn(condNode{where: where})
			n = agg
		}
		outer[i] = selectItem{node: n, alias: a.name}
		types[i] = n.vtype()
	}
	// The order of the inner rows only matters when the query is sliced.
	sub, err := c.build(inner, q.sliced())
	if err != nil {
		return "", nil, nil, err
	}
	if skip {
		return "", nil, nil, errEmptyResultSet
	}
	sub.forUpdate = false
	w.write("SELECT ")
	for i, it := range outer {
		if i > 0 {
			w.write(", ")
		}
		it.node.render(w)
		w.write(" AS ")
		w.ident(it.alias)
	}
	w.write(" FROM (")
	sub.render(w)
	w.write(") ")
	w.ident(subqueryAlias)
	return w.String(), w.args, types, nil
}

// plainAggregates reports whether every expression is a bare aggregate call,
// whose value over no rows is known: zero for Count and NULL for the others.
func plainAggregates(aggs []annotation) bool {
	for _, a := range aggs {
		if a.expr.kind != exprAggregate {
			return false
		}
	}
	return true
}

// aggregate runs the aggregates over the rows of q and returns their values
// by name.
func (q *query) aggregate(ctx context.Context, aggs []annotation) (map[string]any, error) {
	query, args, types, err := q.aggregateSQL(aggs)
	if errors.Is(err, errEmptyResultSet) {
		out := make(map[string]any, len(aggs))
		for _, a := range aggs {
			out[a.name] = nil
			if a.expr.fn == "COUNT" {
				out[a.name] = int64(0)
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
