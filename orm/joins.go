package orm

import "strconv"

// join is one table of a query's FROM clause: the base table, or a table
// joined to its parent on parent.fromColumn = alias.toColumn.
type join struct {
	alias      string
	model      *Model
	parent     *join // nil for the base table
	fromColumn string
	toColumn   string
	// nullable reports that the join may match no row.
	nullable bool
	// outer selects LEFT OUTER JOIN over INNER JOIN. It is always false for
	// the base table.
	outer bool
	// read reports that the statement reads a column of the table. A join that
	// is only reserved is left out of the FROM clause.
	read bool
}

// joinSet is the set of tables a query reads. A relation traversed by several
// keys (a filter, an ordering, a select_related) is joined once: joins are
// reused by parent and columns.
type joinSet struct {
	base  *join
	joins []*join // joined tables in creation order, without the base
	// aliases counts the aliases handed out per table name.
	aliases map[string]int
}

// newJoinSet returns the join set of a query on model m. The base table is
// aliased by its own name.
func newJoinSet(m *Model) *joinSet {
	return &joinSet{
		base:    &join{alias: m.Table(), model: m},
		aliases: map[string]int{m.Table(): 1},
	}
}

// setup follows hops from the base table and returns the join of each hop,
// creating the ones that do not exist yet. A new join is LEFT OUTER when its
// hop is nullable or its parent is LEFT OUTER, and INNER otherwise, as in
// Django's Query.join.
func (js *joinSet) setup(hops []hop) []*join {
	return js.follow(js.base, hops, true)
}

// reserve records the joins of hops from parent without reading from them. A
// condition on a foreign-key column is answered by the column itself, but it
// still says something about the relation: the join takes part in the votes
// on the join types, so that a later key which does read the table finds it
// with the right type. This is the join Django creates and then trims.
func (js *joinSet) reserve(parent *join, hops []hop) []*join {
	return js.follow(parent, hops, false)
}

// used returns the number of joins the statement reads from.
func (js *joinSet) used() int {
	n := 0
	for _, j := range js.joins {
		if j.read {
			n++
		}
	}
	return n
}

// follow returns the join of each hop from parent, creating the missing ones
// and marking them as read when read is set.
func (js *joinSet) follow(parent *join, hops []hop, read bool) []*join {
	out := make([]*join, 0, len(hops))
	for _, h := range hops {
		j := js.find(parent, h)
		if j == nil {
			j = &join{
				alias:      js.newAlias(h.target.Table()),
				model:      h.target,
				parent:     parent,
				fromColumn: h.fromColumn,
				toColumn:   h.toColumn,
				nullable:   h.nullable,
				outer:      h.nullable || parent.outer,
			}
			js.joins = append(js.joins, j)
		}
		j.read = j.read || read
		out = append(out, j)
		parent = j
	}
	return out
}

// find returns the existing join for hop h from parent, or nil.
func (js *joinSet) find(parent *join, h hop) *join {
	for _, j := range js.joins {
		if j.parent == parent && j.model == h.target && j.fromColumn == h.fromColumn && j.toColumn == h.toColumn {
			return j
		}
	}
	return nil
}

// newAlias returns the alias for a new use of table. The first use is the
// table name itself; a later one is "T<n>" with n the position of the alias
// among all aliases of the query, which is Django's naming.
func (js *joinSet) newAlias(table string) string {
	js.aliases[table]++
	if js.aliases[table] == 1 {
		return table
	}
	return "T" + strconv.Itoa(len(js.joins)+2)
}

// innerJoins returns the joins that are currently INNER.
func (js *joinSet) innerJoins() []*join {
	var out []*join
	for _, j := range js.joins {
		if !j.outer {
			out = append(out, j)
		}
	}
	return out
}

// promote turns the given joins into LEFT OUTER joins where that can change
// the result: a join is promoted when it is nullable or hangs off an outer
// join. Promoting a join re-examines the joins that hang off it. It ports
// Django's Query.promote_joins.
func (js *joinSet) promote(joins []*join) {
	queue := append([]*join(nil), joins...)
	for len(queue) > 0 {
		j := queue[0]
		queue = queue[1:]
		if j.parent == nil || j.outer {
			continue
		}
		if !j.nullable && !j.parent.outer {
			continue
		}
		j.outer = true
		for _, child := range js.joins {
			if child.parent == j {
				queue = append(queue, child)
			}
		}
	}
}

// demote turns the given joins back into INNER joins, and with each one the
// chain of outer joins leading to it: a row can only satisfy a condition on a
// joined table when every join on the way matched. It ports Django's
// Query.demote_joins.
func (js *joinSet) demote(joins []*join) {
	queue := append([]*join(nil), joins...)
	for len(queue) > 0 {
		j := queue[0]
		queue = queue[1:]
		if !j.outer {
			continue
		}
		j.outer = false
		if j.parent != nil && j.parent.parent != nil {
			queue = append(queue, j.parent)
		}
	}
}

// joinPromoter decides the type of the joins used by the children of one
// condition node, porting Django's JoinPromoter. Each child votes for the
// joins it needs to be INNER. Under AND a single vote is enough; under OR a
// join may only be INNER when every child needs it, because a row must still
// be found through the other branch when the join matches nothing.
type joinPromoter struct {
	effectiveOr bool
	children    int
	votes       map[*join]int
	order       []*join // voted joins in first-vote order, for determinism
}

// newJoinPromoter returns the promoter for a node with n children combined by
// conn. A negated node swaps the connector, by De Morgan's laws.
func newJoinPromoter(conn connector, n int, negated bool) *joinPromoter {
	or := conn == connOr
	if negated {
		or = !or
	}
	return &joinPromoter{effectiveOr: or, children: n, votes: make(map[*join]int)}
}

// addVotes records that one child needs each of joins to be INNER.
func (p *joinPromoter) addVotes(joins []*join) {
	seen := make(map[*join]bool, len(joins))
	for _, j := range joins {
		if j.parent == nil || seen[j] {
			continue
		}
		seen[j] = true
		if _, ok := p.votes[j]; !ok {
			p.order = append(p.order, j)
		}
		p.votes[j]++
	}
}

// update applies the votes to the join set and returns the joins that are
// required to be INNER, which the caller passes on as its own vote.
func (p *joinPromoter) update(js *joinSet) []*join {
	var toPromote, toDemote []*join
	for _, j := range p.order {
		votes := p.votes[j]
		if p.effectiveOr && votes < p.children {
			toPromote = append(toPromote, j)
		}
		if !p.effectiveOr || votes == p.children {
			toDemote = append(toDemote, j)
		}
	}
	js.promote(toPromote)
	js.demote(toDemote)
	return toDemote
}
