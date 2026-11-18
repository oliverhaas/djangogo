package orm

import (
	"errors"
	"slices"
	"strings"
)

// whereNode is a resolved condition tree: the WHERE or HAVING clause of a
// statement. A child is a predicate or a nested node.
type whereNode struct {
	conn     connector
	negated  bool
	children []whereChild
}

// whereChild is one child of a whereNode: a predicate, or a nested node when
// node is set.
type whereChild struct {
	pred *predicate
	node *whereNode
}

// errEmptyResultSet reports that a query can match no row, which is known
// without running it. The execution methods turn it into an empty result; it
// never reaches a caller. It is Django's EmptyResultSet.
var errEmptyResultSet = errors.New("orm: empty result set")

// matchKind tells what a condition is known to match before the query runs.
type matchKind int

const (
	// matchSome is a condition that the database has to evaluate.
	matchSome matchKind = iota
	// matchNone is a condition that no row can meet.
	matchNone
	// matchAll is a condition that every row meets.
	matchAll
)

// empty reports whether the node holds no condition.
func (n *whereNode) empty() bool { return n == nil || len(n.children) == 0 }

// simplify removes the children whose outcome is known without running the
// query and reports what is then known about the node itself. A conjunction
// matches nothing as soon as one child does and everything when all children
// do; a disjunction is the mirror image, and a negation swaps the two
// outcomes. It ports the counting of Django's WhereNode.as_sql.
func (n *whereNode) simplify() matchKind {
	fullNeeded, emptyNeeded := len(n.children), 1
	if n.conn == connOr {
		fullNeeded, emptyNeeded = 1, len(n.children)
	}
	kept := make([]whereChild, 0, len(n.children))
	for _, ch := range n.children {
		kind := matchSome
		switch {
		case ch.node != nil:
			kind = ch.node.simplify()
		case ch.pred.empty:
			kind = matchNone
		}
		switch kind {
		case matchNone:
			emptyNeeded--
		case matchAll:
			fullNeeded--
		default:
			kept = append(kept, ch)
		}
		if emptyNeeded == 0 {
			return n.outcome(matchNone)
		}
		if fullNeeded == 0 {
			return n.outcome(matchAll)
		}
	}
	n.children = kept
	return matchSome
}

// outcome returns kind as seen from outside the node: swapped when the node
// is negated.
func (n *whereNode) outcome(kind matchKind) matchKind {
	if !n.negated {
		return kind
	}
	if kind == matchNone {
		return matchAll
	}
	return matchNone
}

// add appends child to n. A child that is not negated and either uses n's
// connector or has a single child is merged into n, as Django's Node.add
// squashes it, so that chained filters render as one flat conjunction.
func (n *whereNode) add(child *whereNode) {
	if child.empty() {
		return
	}
	if !child.negated && (child.conn == n.conn || len(child.children) == 1) {
		n.children = append(n.children, child.children...)
		return
	}
	n.children = append(n.children, whereChild{node: child})
}

// render writes the condition. A negated node renders as "NOT (...)" and a
// node with several children is parenthesized, as Django's WhereNode.as_sql
// renders them. The node must not be empty.
func (n *whereNode) render(w *sqlWriter) {
	wrap := n.negated || len(n.children) > 1
	switch {
	case n.negated:
		w.write("NOT (")
	case wrap:
		w.write("(")
	}
	for i, ch := range n.children {
		if i > 0 {
			w.write(" " + n.conn.String() + " ")
		}
		if ch.node != nil {
			ch.node.render(w)
		} else {
			ch.pred.render(w)
		}
	}
	if wrap {
		w.write(")")
	}
}

// containsAggregate reports whether any predicate of the tree references an
// aggregate, which forces it into the HAVING clause.
func (n *whereNode) containsAggregate() bool {
	for _, ch := range n.children {
		if ch.node != nil {
			if ch.node.containsAggregate() {
				return true
			}
		} else if ch.pred.containsAggregate() {
			return true
		}
	}
	return false
}

// split divides the tree into the part that can filter rows before grouping
// (WHERE) and the part that needs the aggregates (HAVING). Children joined by
// AND are split one by one; children joined by OR, or by AND under a negation,
// must stay together, so the whole node goes to HAVING. It ports Django's
// WhereNode.split_having_qualify.
func (n *whereNode) split(negated bool) (where, having *whereNode) {
	if n.empty() {
		return nil, nil
	}
	if !n.containsAggregate() {
		return n, nil
	}
	inNegated := negated != n.negated
	if (inNegated && n.conn == connAnd) || (!inNegated && n.conn == connOr) {
		return nil, n
	}
	var whereParts, havingParts []whereChild
	for _, ch := range n.children {
		switch {
		case ch.node != nil:
			wn, hn := ch.node.split(inNegated)
			if wn != nil {
				whereParts = append(whereParts, whereChild{node: wn})
			}
			if hn != nil {
				havingParts = append(havingParts, whereChild{node: hn})
			}
		case ch.pred.containsAggregate():
			havingParts = append(havingParts, ch)
		default:
			whereParts = append(whereParts, ch)
		}
	}
	if len(whereParts) > 0 {
		where = &whereNode{conn: n.conn, negated: n.negated, children: whereParts}
	}
	if len(havingParts) > 0 {
		having = &whereNode{conn: n.conn, negated: n.negated, children: havingParts}
	}
	return where, having
}

// addFilters resolves every Filter and Exclude condition of the query, in
// call order, into c.where and c.having. A join that was INNER before a call
// is INNER after it: a later call may only relax the joins it introduced
// itself, as in Django's Query.add_q.
func (c *compiler) addFilters() error {
	root := &whereNode{conn: connAnd}
	for _, cond := range c.q.filters {
		if cond.err != nil {
			return cond.err
		}
		existing := c.joins.innerJoins()
		node, _, err := c.addQ(cond, false)
		if err != nil {
			return err
		}
		root.add(node)
		c.joins.demote(existing)
	}
	switch root.simplify() {
	case matchNone:
		// A statement that has to run all the same (an aggregate whose value over
		// no rows only the database can tell) filters on a false condition.
		c.empty = true
		c.where = &whereNode{conn: connAnd, children: []whereChild{{pred: &predicate{empty: true}}}}
		return nil
	case matchAll:
		return nil
	}
	c.where, c.having = root.split(false)
	return nil
}

// addQ resolves one condition tree. It returns the resolved node and the joins
// the tree needs to be INNER, which the parent uses as this child's vote.
// currentNegated tells whether the tree sits under an odd number of negations.
// It ports Django's Query._add_q.
func (c *compiler) addQ(cond Cond, currentNegated bool) (*whereNode, []*join, error) {
	currentNegated = currentNegated != cond.negated
	node := &whereNode{conn: cond.conn, negated: cond.negated}
	promoter := newJoinPromoter(cond.conn, len(cond.children), currentNegated)
	for _, ch := range cond.children {
		var (
			child *whereNode
			inner []*join
			err   error
		)
		if ch.sub != nil {
			child, inner, err = c.addQ(*ch.sub, currentNegated)
		} else {
			child, inner, err = c.buildFilter(ch.key, ch.value, currentNegated)
		}
		if err != nil {
			return nil, nil, err
		}
		promoter.addVotes(inner)
		node.add(child)
	}
	return node, promoter.update(c.joins), nil
}

// buildFilter resolves one key/value condition. It returns the condition and
// the joins it needs to be INNER. It ports Django's Query.build_filter.
//
// Two cases need rows that an INNER join would drop, and so cast no vote: an
// "is null" test, which is true for a row whose relation is missing, and any
// test under a negation, since NOT must also keep the rows without a related
// row. A negated test on a column that can be NULL also gets "AND col IS NOT
// NULL", so that NOT turns the SQL unknown of a NULL comparison into a match,
// as Django's exclude() does.
func (c *compiler) buildFilter(key string, value any, currentNegated bool) (*whereNode, []*join, error) {
	parts := strings.Split(key, lookupSep)
	c.used = c.used[:0]

	if n, ok := c.annots[parts[0]]; ok {
		lookup, err := c.lookupOf(parts[1:], nil)
		if err != nil {
			return nil, nil, err
		}
		pred, err := c.buildPredicate(lookup, n, nil, value)
		if err != nil {
			return nil, nil, err
		}
		return &whereNode{conn: connAnd, children: []whereChild{{pred: pred}}}, nil, nil
	}

	path, err := resolvePath(c.q.model, parts, c.annotNames)
	if err != nil {
		return nil, nil, err
	}
	// A negated condition across a relation that can match several rows has to
	// be a subquery (Django's split_exclude): as a join it would only drop the
	// related rows that match, not the row itself.
	if currentNegated {
		for _, h := range path.hops {
			if h.many {
				return nil, nil, errors.New("orm: excluding across a multi-valued relation is not supported")
			}
		}
	}
	lookup, err := c.lookupOf(path.rest, &path)
	if err != nil {
		return nil, nil, err
	}
	col := c.column(path)
	pred, err := c.buildPredicate(lookup, col, path.field, value)
	if err != nil {
		return nil, nil, err
	}
	used := append([]*join(nil), c.used...)
	node := &whereNode{conn: connAnd, children: []whereChild{{pred: pred}}}

	isNull := pred.lookup == lookupIsNull
	requireOuter := isNull && pred.flag && !currentNegated
	if currentNegated && (!isNull || !pred.flag) {
		requireOuter = true
		if !isNull {
			if path.field.Null || col.join.outer {
				node.children = append(node.children, whereChild{pred: &predicate{lookup: lookupIsNull, lhs: col}})
			}
			if len(pred.rhs) == 1 {
				if rhs, ok := pred.rhs[0].(colNode); ok && (rhs.field.Null || rhs.join.outer) {
					node.children = append(node.children, whereChild{pred: &predicate{lookup: lookupIsNull, lhs: rhs}})
				}
			}
		}
	}
	if requireOuter {
		return node, nil, nil
	}
	return node, used, nil
}

// lookupOf returns the lookup named by the trailing parts of a key: "exact"
// when there is none. path is the resolved field path, or nil when the key
// names an annotation. Anything that is not a single lookup the field
// supports is a FieldError worded as Django's Query.try_transform words it,
// with the same "perhaps you meant" suggestions.
func (c *compiler) lookupOf(rest []string, path *fieldPath) (string, error) {
	if len(rest) == 0 {
		return lookupExact, nil
	}
	name := rest[0]
	supported := lookupNames
	kind := "an annotation"
	if path != nil {
		kind = djangoFieldName(path.field)
		if path.relation {
			supported = relationLookupNames
			kind = "ForeignKey"
		}
	}
	if len(rest) == 1 && slices.Contains(supported, name) {
		return name, nil
	}
	suggestion := "."
	if similar := closeMatches(name, supported); len(similar) > 0 {
		suggestion = ", perhaps you meant " + strings.Join(similar, " or ") + "?"
	}
	return "", &FieldError{
		Model: c.q.model.Name(),
		Message: "unsupported lookup '" + strings.Join(rest, lookupSep) + "' for " + kind +
			" or join on the field not permitted" + suggestion,
	}
}

// djangoFieldName returns the name of the Django field class that corresponds
// to the kind of f, used in error messages that Django words with it.
func djangoFieldName(f *Field) string {
	switch f.Kind {
	case KindAuto:
		return "BigAutoField"
	case KindInt:
		return "IntegerField"
	case KindChar:
		return "CharField"
	case KindText:
		return "TextField"
	case KindBool:
		return "BooleanField"
	case KindDateTime:
		return "DateTimeField"
	default:
		return "Field"
	}
}
