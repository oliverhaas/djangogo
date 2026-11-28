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
// outcomes. A node without children restricts nothing, negated or not. It
// ports the counting of Django's WhereNode.as_sql.
func (n *whereNode) simplify() matchKind {
	if len(n.children) == 0 {
		return matchAll
	}
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

// operands returns the expressions that the predicates of the tree compare.
func (n *whereNode) operands() []sqlNode {
	if n == nil {
		return nil
	}
	var out []sqlNode
	for _, ch := range n.children {
		if ch.node != nil {
			out = append(out, ch.node.operands()...)
			continue
		}
		if ch.pred.lhs != nil {
			out = append(out, ch.pred.lhs)
		}
		out = append(out, ch.pred.rhs...)
	}
	return out
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

// containsOver reports whether any predicate of the tree references a
// window, which forces it into the QUALIFY part.
func (n *whereNode) containsOver() bool {
	for _, ch := range n.children {
		if ch.node != nil {
			if ch.node.containsOver() {
				return true
			}
		} else if ch.pred.containsOver() {
			return true
		}
	}
	return false
}

// errHeterogeneousQualify refuses a disjunction of a window predicate and a
// row predicate in a query with aggregates, as Django does.
var errHeterogeneousQualify = errors.New("orm: Heterogeneous disjunctive predicates against window functions are " + //nolint:revive,staticcheck // Django's message
	"not implemented when performing conditional aggregation.")

// split divides the tree into the part that can filter rows before grouping
// (WHERE), the part that needs the aggregates (HAVING) and the part that
// needs the windows (QUALIFY), which filters the rows of the query after
// they are computed. Children joined by AND are split one by one; children
// joined by OR, or by AND under a negation, must stay together, so the
// whole node goes to HAVING, or to QUALIFY when it holds a window. A
// disjunction of a window predicate and a row predicate cannot stay
// together in a query with aggregates, mustGroupBy, because the QUALIFY
// part would filter the grouped rows. It ports Django's
// WhereNode.split_having_qualify.
func (n *whereNode) split(negated, mustGroupBy bool) (where, having, qualify *whereNode, err error) {
	if n.empty() {
		return nil, nil, nil, nil
	}
	agg, over := n.containsAggregate(), n.containsOver()
	if !agg && !over {
		return n, nil, nil, nil
	}
	inNegated := negated != n.negated
	connected := (inNegated && n.conn == connAnd) || (!inNegated && n.conn == connOr)
	if connected && !over {
		return nil, n, nil, nil
	}
	var whereParts, havingParts, qualifyParts []whereChild
	for _, ch := range n.children {
		switch {
		case ch.node != nil:
			wn, hn, qn, err := ch.node.split(inNegated, mustGroupBy)
			if err != nil {
				return nil, nil, nil, err
			}
			if wn != nil {
				whereParts = append(whereParts, whereChild{node: wn})
			}
			if hn != nil {
				havingParts = append(havingParts, whereChild{node: hn})
			}
			if qn != nil {
				qualifyParts = append(qualifyParts, whereChild{node: qn})
			}
		case ch.pred.containsOver():
			qualifyParts = append(qualifyParts, ch)
		case ch.pred.containsAggregate():
			havingParts = append(havingParts, ch)
		default:
			whereParts = append(whereParts, ch)
		}
	}
	if connected && len(qualifyParts) > 0 {
		if len(whereParts) > 0 && mustGroupBy {
			return nil, nil, nil, errHeterogeneousQualify
		}
		return nil, nil, n, nil
	}
	part := func(children []whereChild) *whereNode {
		if len(children) == 0 {
			return nil
		}
		return &whereNode{conn: n.conn, negated: n.negated, children: children}
	}
	return part(whereParts), part(havingParts), part(qualifyParts), nil
}

// addFilters resolves every Filter and Exclude condition of the query, in
// call order, into c.where, c.having and c.qualify, and before each one the
// annotations added before it. A join that was INNER before a call is INNER
// after it: a later call may only relax the joins it introduced itself, as
// in Django's Query.add_q.
func (c *compiler) addFilters() error {
	root := &whereNode{conn: connAnd}
	if mf := c.q.m2mFilter; mf != nil && c.q.model != nil && c.q.model.PrimaryKey() != nil {
		h := hop{
			fromColumn: c.q.model.PrimaryKey().Column,
			target:     mf.through,
			toColumn:   mf.targetFKCol,
			nullable:   false,
			many:       true,
		}
		joins := c.joins.setup([]hop{h}, nil)
		arg, err := bindField(c.d, mf.sourceField, mf.ownerPK)
		if err != nil {
			return err
		}
		pred := &predicate{
			lookup: lookupExact,
			lhs:    colNode{join: joins[0], field: mf.sourceField},
			rhs:    []sqlNode{paramNode{value: arg}},
		}
		root.children = append(root.children, whereChild{pred: pred})
	}
	if mp := c.q.m2mPrefetch; mp != nil && c.q.model != nil && c.q.model.PrimaryKey() != nil {
		h := hop{
			fromColumn: c.q.model.PrimaryKey().Column,
			target:     mp.through,
			toColumn:   mp.targetFK.Column,
			nullable:   false,
			many:       true,
		}
		joins := c.joins.setup([]hop{h}, nil)
		c.joins.demote(joins)
		c.m2mPrefetchJoin = joins[0]
		pred := &predicate{
			lookup: lookupIn,
			lhs:    colNode{join: joins[0], field: mp.sourceFK},
			rhs:    make([]sqlNode, 0, len(mp.parentPKs)),
		}
		for _, pk := range mp.parentPKs {
			arg, err := bindField(c.d, mp.sourceFK, pk)
			if err != nil {
				return err
			}
			pred.rhs = append(pred.rhs, paramNode{value: arg})
		}
		pred.empty = len(pred.rhs) == 0
		root.children = append(root.children, whereChild{pred: pred})
	}
	for i, cond := range c.q.filters {
		if err := c.resolveAnnotations(i); err != nil {
			return err
		}
		if cond.err != nil {
			return cond.err
		}
		existing := c.joins.innerJoins()
		c.reuse = make(map[*join]bool)
		node, _, err := c.addQ(cond, false)
		c.reuse = nil
		if err != nil {
			return err
		}
		root.add(node)
		c.joins.demote(existing)
	}
	if c.q.none {
		// Resolve the filters first, so that None reports an invalid one, as
		// Django's set_empty adds its NothingNode next to them.
		root.children = append(root.children, whereChild{pred: &predicate{empty: true}})
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
	var err error
	c.where, c.having, c.qualify, err = root.split(false, c.q.hasAggregates())
	return err
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
		switch {
		case ch.sub != nil:
			child, inner, err = c.addQ(*ch.sub, currentNegated)
		case ch.expr != nil:
			child, err = c.buildExprFilter(*ch.expr)
		default:
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

// buildExprFilter resolves a boolean expression used as a condition, such as
// Exists. It casts no vote on the joins, as Django's build_filter returns no
// used joins for one.
func (c *compiler) buildExprFilter(e Expr) (*whereNode, error) {
	n, err := c.resolveExpr(e)
	if err != nil {
		return nil, err
	}
	if n.vtype() != typeBool {
		return nil, errors.New("orm: cannot filter against a non-conditional expression")
	}
	pred := &predicate{lookup: lookupExpr, lhs: n}
	return &whereNode{conn: connAnd, children: []whereChild{{pred: pred}}}, nil
}

// annotationField returns the field that a value compared with the
// annotation n binds as: a date, a datetime, a time of day or a duration binds
// as the dialect stores that type, such as a duration in microseconds on
// SQLite, and a JSON value as JSON text. It is nil for other types, whose
// values bind as they are.
func annotationField(n sqlNode) *Field {
	switch t := n.vtype(); t {
	case typeDate, typeTime, typeTimeOfDay, typeDuration, typeJSON:
		return &Field{Kind: valueKind(t)}
	}
	return nil
}

// buildFilter resolves one key/value condition. It returns the condition and
// the joins it needs to be INNER. It ports Django's Query.build_filter.
//
// Two cases need rows that an INNER join would drop, and so cast no vote: an
// "is null" test, which is true for a row whose relation is missing, and any
// test under a negation, since NOT must also keep the rows without a related
// row. A negated test on a column that can be NULL also gets "AND col IS NOT
// NULL", so that NOT turns the SQL unknown of a NULL comparison into a match,
// as Django's exclude() does. A negated "in" whose values hold a nil gets "OR
// col IS NULL" instead, so that the nil excludes the rows that hold NULL.
func (c *compiler) buildFilter(key string, value any, currentNegated bool) (*whereNode, []*join, error) {
	parts := strings.Split(key, lookupSep)
	c.used = c.used[:0]

	// An annotation name can hold the separator, such as "links__sum", so try
	// each prefix of the key, as Django's refs_expression does.
	for i := 1; i <= len(parts); i++ {
		n, ok := c.annots[strings.Join(parts[:i], lookupSep)]
		if !ok {
			continue
		}
		kind := c.annotKind(strings.Join(parts[:i], lookupSep), n)
		lookup, transforms, err := c.kindLookupOf(parts[i:], kind)
		if err != nil {
			return nil, nil, err
		}
		var pred *predicate
		if len(transforms) == 0 {
			pred, err = c.buildPredicate(lookup, n, annotationField(n), value, nil)
		} else {
			pred, err = c.transformPredicate(lookup, transforms, n, kind, value)
		}
		if err != nil {
			return nil, nil, err
		}
		return &whereNode{conn: connAnd, children: []whereChild{{pred: pred}}}, nil, nil
	}

	path, err := resolvePath(c.q.model, parts, c.annotNames)
	if err != nil {
		return nil, nil, err
	}
	lookup, transforms, err := c.lookupOf(path.rest, &path)
	if err != nil {
		return nil, nil, err
	}
	// A negated condition across a relation that can match several rows has to
	// be a correlated EXISTS subquery (Django's split_exclude): as a join it
	// would only drop the related rows that match, not the parent row itself.
	if currentNegated && !c.noSplit {
		for _, h := range path.hops {
			if h.many {
				node, err := c.splitExclude(path, transforms, lookup, value)
				if err != nil {
					return nil, nil, err
				}
				return node, nil, nil
			}
		}
	}
	col := c.column(path)
	pred, err := c.fieldPredicate(lookup, transforms, col, path.field, value, path.expectedModel())
	if err != nil {
		return nil, nil, err
	}
	used := append([]*join(nil), c.used...)
	node := &whereNode{conn: connAnd, children: []whereChild{{pred: pred}}}

	isNull := pred.lookup == lookupIsNull
	requireOuter := isNull && pred.flag && !currentNegated
	// A nil value that is not an isnull test, which compares a JSON value
	// with JSON null, keeps the joins and adds no test of the column, as
	// Django skips both for a lookup whose rhs is None.
	if currentNegated && (!isNull || !pred.flag) && value != nil {
		requireOuter = true
		if !isNull {
			nullable := path.field.Null || col.join.outer
			if nullable && pred.null {
				orNull := &predicate{lookup: lookupIsNull, lhs: col, flag: true}
				return &whereNode{conn: connOr, children: []whereChild{{pred: pred}, {pred: orNull}}}, nil, nil
			}
			if nullable {
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

// lookupOf returns the lookup named by the trailing parts of a key that
// ends on the field path, "exact" when there is none, and the transforms
// before it, as Django's Query.build_lookup splits them. A relation takes
// one lookup and no transforms; a plain field is read by kindLookupOf.
func (c *compiler) lookupOf(rest []string, path *fieldPath) (string, []string, error) {
	if len(rest) == 0 {
		return lookupExact, nil, nil
	}
	if path.relation {
		name := rest[0]
		if len(rest) == 1 && slices.Contains(relationLookupNames, name) {
			return name, nil, nil
		}
		return "", nil, c.unsupportedLookup(strings.Join(rest, lookupSep), name, relationFieldName(path), relationLookupNames)
	}
	return c.kindLookupOf(rest, path.field.Kind)
}

// kindLookupOf returns the lookup named by rest, the trailing parts of a key
// that ends on a value of kind kind, "exact" when there is none, and the
// transforms before it: every part but the last must be a transform of the
// value before it, and the last is a lookup or, failing that, a transform
// compared with "exact". Anything else is a FieldError worded as Django's
// Query.try_transform words it, with the same "perhaps you meant"
// suggestions.
func (c *compiler) kindLookupOf(rest []string, kind Kind) (string, []string, error) {
	if len(rest) == 0 {
		return lookupExact, nil, nil
	}
	fieldName := kind.FieldClassName()
	last := len(rest) - 1
	for _, name := range rest[:last] {
		if !hasTransform(kind, name) {
			unsupported := strings.Join(rest[slices.Index(rest, name):], lookupSep)
			return "", nil, c.unsupportedLookup(unsupported, name, fieldName, lookupCandidates(kind))
		}
		kind = outputOf(kind, name)
		fieldName = kind.FieldClassName()
	}
	switch name := rest[last]; {
	case slices.Contains(lookupsOf(kind), name):
		return name, rest[:last], nil
	case hasTransform(kind, name):
		return lookupExact, rest, nil
	default:
		return "", nil, c.unsupportedLookup(name, name, fieldName, lookupCandidates(kind))
	}
}

// relationFieldName returns the name of the Django class of the field a path
// ends on, for error messages. Django names the field of the last join: a
// many-to-many path in either direction ends on a foreign key of the through
// table, and a reverse path on the ManyToOneRel or OneToOneRel of the foreign
// key.
func relationFieldName(path *fieldPath) string {
	switch {
	case path.m2m || (path.reverse != nil && path.reverse.Kind == RelM2M):
		return "ForeignKey"
	case path.reverse != nil && path.reverse.Kind == RelO2O:
		return "OneToOneRel"
	case path.reverse != nil:
		return "ManyToOneRel"
	case path.field != nil && path.field.Rel != nil && path.field.Rel.Kind == RelO2O:
		return "OneToOneField"
	default:
		return "ForeignKey"
	}
}

// unsupportedLookup returns the FieldError of a key part that is neither a
// lookup nor a transform of the value it follows. unsupported is the text
// the message quotes, name the part that failed, kind the Django field class
// of the value, and candidates the names the suggestions come from.
func (c *compiler) unsupportedLookup(unsupported, name, kind string, candidates []string) error {
	suggestion := "."
	if similar := closeMatches(name, candidates); len(similar) > 0 {
		suggestion = ", perhaps you meant " + strings.Join(similar, " or ") + "?"
	}
	return &FieldError{
		Model: c.q.model.Name(),
		Message: "unsupported lookup '" + unsupported + "' for " + kind +
			" or join on the field not permitted" + suggestion,
	}
}

// fieldPredicate returns the predicate of a key that ends on a field: col,
// the field's column, wrapped in transforms and compared with value under
// lookup.
func (c *compiler) fieldPredicate(lookup string, transforms []string, col colNode, field *Field, value any, expectedModel *Model) (*predicate, error) {
	if len(transforms) == 0 {
		return c.buildPredicate(lookup, col, field, value, expectedModel)
	}
	return c.transformPredicate(lookup, transforms, col, field.Kind, value)
}

// transformPredicate returns the predicate of lhs, a value of kind kind,
// wrapped in transforms and compared with value under lookup.
func (c *compiler) transformPredicate(lookup string, transforms []string, lhs sqlNode, kind Kind, value any) (*predicate, error) {
	for _, name := range transforms {
		n, err := c.transform(lhs, kind, name)
		if err != nil {
			return nil, err
		}
		lhs, kind = n, outputOf(kind, name)
	}
	if last, ok := lhs.(transformNode); ok {
		if p, err := c.yearPredicate(lookup, last, value); p != nil || err != nil {
			return p, err
		}
	}
	return c.buildPredicate(lookup, lhs, &Field{Kind: kind}, value, nil)
}
