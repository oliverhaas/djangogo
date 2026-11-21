package orm

// splitExclude builds a correlated EXISTS subquery for a negated condition
// that traverses a multi-valued relation (a reverse foreign key), porting
// Django's Query.split_exclude.
//
// Joining a multi-valued relation directly under NOT would drop only the
// matching related rows rather than excluding the parent row whenever any
// related row matches. Evaluating the positive condition inside EXISTS and
// keeping the negation on the outer whereNode excludes every parent row for
// which any matching related row exists.
func (c *compiler) splitExclude(path fieldPath, lookup string, value any) (*whereNode, error) {
	firstMany := -1
	for i, h := range path.hops {
		if h.many {
			firstMany = i
			break
		}
	}

	// Build the predicate against the outer compiler so that F(...) expressions
	// on the right-hand side bind to the outer query's tables and aliases.
	placeholder := colNode{join: c.joins.base, field: path.field}
	pred, err := c.buildPredicate(lookup, placeholder, path.field, value, path.expectedModel())
	if err != nil {
		return nil, err
	}
	if pred.empty {
		return &whereNode{conn: connAnd, children: []whereChild{{pred: &predicate{empty: true}}}}, nil
	}

	// isnull=True (including exact=nil) must match outer rows that have NO
	// related row at all as well as those whose related column is NULL, so the
	// subquery starts at the outer base model U0 with LEFT OUTER JOINs along
	// the full path.
	if pred.lookup == lookupIsNull && pred.flag {
		basePK := c.q.model.PrimaryKey()
		subJS := newSubqueryJoinSet(c.q.model, "U0")
		subJoins := subJS.follow(subJS.base, path.hops, true, nil)
		// Force every hop in the isnull=True subquery to LEFT OUTER JOIN so
		// missing intermediate rows evaluate to NULL rather than being dropped.
		for _, j := range subJoins {
			j.outer = true
		}
		targetJoin := subJoins[len(subJoins)-1]
		pred.lhs = colNode{join: targetJoin, field: path.field}
		corrPred := &predicate{
			lookup: lookupExact,
			lhs:    colNode{join: subJS.base, field: basePK},
			rhs:    []sqlNode{colNode{join: c.joins.base, field: basePK}},
		}
		subC := &compiler{
			q:     c.q,
			d:     c.d,
			joins: subJS,
			where: &whereNode{
				conn:     connAnd,
				children: []whereChild{{pred: pred}, {pred: corrPred}},
			},
		}
		stmt := &selectStmt{
			c:     subC,
			items: []selectItem{{node: rawNode{sql: "1", typ: typeInt}}},
			limit: 1,
		}
		return &whereNode{conn: connAnd, children: []whereChild{{pred: &predicate{lookup: lookupExists, sub: stmt}}}}, nil
	}

	// Non-isnull=True: set up any single-valued prefix hops on the outer query
	// and start the correlated EXISTS subquery at the target of the first
	// multi-valued hop.
	outerJoin := c.joins.base
	if firstMany > 0 {
		prefixJoins := c.joins.setup(path.hops[:firstMany], c.reuse)
		outerJoin = prefixJoins[len(prefixJoins)-1]
	}
	firstHop := path.hops[firstMany]
	subJS := newSubqueryJoinSet(firstHop.target, "U1")
	targetJoin := subJS.base
	if len(path.hops) > firstMany+1 {
		restJoins := subJS.follow(subJS.base, path.hops[firstMany+1:], true, nil)
		subJS.demote(restJoins)
		targetJoin = restJoins[len(restJoins)-1]
	}
	pred.lhs = colNode{join: targetJoin, field: path.field}
	corrPred := &predicate{
		lookup: lookupExact,
		lhs:    colNode{join: subJS.base, field: &Field{Column: firstHop.toColumn}},
		rhs:    []sqlNode{colNode{join: outerJoin, field: &Field{Column: firstHop.fromColumn}}},
	}
	subC := &compiler{
		q:     c.q,
		d:     c.d,
		joins: subJS,
		where: &whereNode{
			conn:     connAnd,
			children: []whereChild{{pred: pred}, {pred: corrPred}},
		},
	}
	stmt := &selectStmt{
		c:     subC,
		items: []selectItem{{node: rawNode{sql: "1", typ: typeInt}}},
		limit: 1,
	}
	return &whereNode{conn: connAnd, children: []whereChild{{pred: &predicate{lookup: lookupExists, sub: stmt}}}}, nil
}
