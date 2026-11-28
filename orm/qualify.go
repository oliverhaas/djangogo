package orm

import (
	"slices"
	"strconv"
)

// qualifyAlias and qualifyMaskAlias name the derived tables of a query that
// filters against windows, as Django names them.
const (
	qualifyAlias     = "qualify"
	qualifyMaskAlias = "qualify_mask"
)

// renderQualify writes a statement that filters against windows, which SQL
// computes after WHERE and HAVING, the way Django's get_qualify_sql emulates
// QUALIFY. The statement without the window conditions, its ordering and its
// slice selects every column under an alias, "col1", "col2" and so on for a
// column without one. An outer statement filters its rows on the
// conditions, then orders and slices them. An operand of a condition or a
// term of the ordering that the statement does not select is selected as
// "qual0", "qual1" and so on, and an outermost statement then selects the
// columns of the statement only.
func (s *selectStmt) renderQualify(w *sqlWriter) {
	c := s.c
	inner := *s
	inner.items = slices.Clone(s.items)
	inner.order, inner.limit, inner.offset, inner.forUpdate = nil, -1, 0, false
	aliases := make(map[string]string, len(inner.items))
	for i := range inner.items {
		it := &inner.items[i]
		if it.alias == "" {
			it.alias = "col" + strconv.Itoa(i+1)
		}
		if key := nodeKey(c.d, it.node); aliases[key] == "" {
			aliases[key] = it.alias
		}
	}
	selected := len(inner.items)
	ref := func(n sqlNode) sqlNode {
		key := nodeKey(c.d, n)
		alias, ok := aliases[key]
		if !ok {
			alias = "qual" + strconv.Itoa(len(inner.items)-selected)
			inner.items = append(inner.items, selectItem{node: n, alias: alias})
			aliases[key] = alias
		}
		return refNode{alias: alias, typ: n.vtype()}
	}
	qualify := c.qualify.withOperands(ref)
	outer := &selectStmt{c: c, items: s.items, order: slices.Clone(s.order), limit: -1}
	for i, o := range outer.order {
		// An item is ordered by its position, which is the same in the
		// derived tables.
		if o.random || (o.alias != "" && s.position(o.alias) > 0) {
			continue
		}
		outer.order[i].node, outer.order[i].alias = ref(o.node), ""
	}

	masked := len(inner.items) > selected
	if masked && len(s.groupBy) > 0 {
		inner.groupBy = groupAdded(c.d, s.groupBy, inner.items[selected:])
	}
	w.write("SELECT ")
	if masked {
		for i, it := range inner.items[:selected] {
			if i > 0 {
				w.write(", ")
			}
			w.ident(it.alias)
		}
		w.write(" FROM (SELECT ")
	}
	w.write("* FROM (")
	inner.renderSelect(w)
	w.write(") ")
	w.ident(qualifyAlias)
	w.write(" WHERE ")
	qualify.render(w)
	if masked {
		w.write(") ")
		w.ident(qualifyMaskAlias)
	}
	outer.renderOrder(w)
	renderLimit(w, s.limit, s.offset)
}

// groupAdded returns the GROUP BY columns with the columns of the items that
// a query filtering against windows selects in addition, such as the
// partition and ordering of a window that only a filter reads, which
// PostgreSQL requires.
func groupAdded(d Dialect, groupBy []sqlNode, added []selectItem) []sqlNode {
	out := slices.Clone(groupBy)
	seen := make(map[string]bool, len(out))
	for _, n := range out {
		seen[nodeKey(d, n)] = true
	}
	for _, it := range added {
		for _, col := range columnsOf(it.node) {
			if key := nodeKey(d, col); !seen[key] {
				seen[key] = true
				out = append(out, col)
			}
		}
	}
	return out
}

// withOperands returns a copy of the tree with each operand of its
// predicates replaced by ref of it. A bound value stays.
func (n *whereNode) withOperands(ref func(sqlNode) sqlNode) *whereNode {
	out := &whereNode{conn: n.conn, negated: n.negated, children: make([]whereChild, len(n.children))}
	for i, ch := range n.children {
		if ch.node != nil {
			out.children[i] = whereChild{node: ch.node.withOperands(ref)}
			continue
		}
		p := *ch.pred
		if p.lhs != nil {
			p.lhs = ref(p.lhs)
		}
		p.rhs = make([]sqlNode, len(ch.pred.rhs))
		for j, r := range ch.pred.rhs {
			if _, ok := r.(paramNode); ok {
				p.rhs[j] = r
			} else {
				p.rhs[j] = ref(r)
			}
		}
		out.children[i] = whereChild{pred: &p}
	}
	return out
}

// refNode is a column of the derived table of a query that filters against
// windows, which the outer statement reads by its alias, Django's Ref.
type refNode struct {
	alias string
	typ   valueType
}

// render writes the alias.
func (n refNode) render(w *sqlWriter) { w.ident(n.alias) }

// vtype is the type of the selected expression.
func (n refNode) vtype() valueType { return n.typ }

// aggregate is false: the derived table computed the value.
func (refNode) aggregate() bool { return false }
