package orm

import (
	"errors"
	"fmt"
	"strings"
)

// AnyQuerySet is a queryset of any row type: a *QuerySet[T], a
// *ValuesQuerySet or a *ValuesListQuerySet. Subquery and Exists take one.
// Only the querysets of this package implement it.
type AnyQuerySet interface {
	subquery() *query
}

// Subquery is the value of the query qs inside another query, Django's
// Subquery: "(SELECT ...)". qs selects one column, the single key of Values
// or ValuesList, or the primary key of a queryset of model rows. OuterRef
// references a field of the query that the subquery is part of:
//
//	newest := orm.Query[Comment](db).Filter("post", orm.OuterRef("pk")).OrderBy("-created")
//	orm.Query[Post](db).Annotate("newest_email", orm.Subquery(newest.Values("email").Limit(1)))
//
// The ordering of qs applies only when it is sliced. The value is NULL when
// qs matches no row, and a database error when it matches several rows where
// one value is expected. A Subquery is also the value of an "in" lookup, as a
// queryset is.
func Subquery(qs AnyQuerySet) Expr {
	e := Expr{kind: exprSubquery}
	if qs != nil {
		e.sub = qs.subquery()
	}
	return e
}

// Exists reports whether the query qs matches any row, Django's Exists:
// "EXISTS(SELECT ...)". It is a boolean expression, so it can be an
// annotation, a term of OrderBy, or a condition of Filter, Exclude and Q:
//
//	orm.Query[Post](db).Filter(orm.Exists(orm.Query[Comment](db).Filter("post", orm.OuterRef("pk"))))
//
// The statement selects a constant and at most one row, and drops the
// ordering of qs. Exists of a queryset known to match no row, such as one of
// None, is false.
func Exists(qs AnyQuerySet) Expr {
	e := Subquery(qs)
	e.exists = true
	return e
}

// OuterRef references a field of the query that a subquery is part of,
// Django's OuterRef. ref is a field key of that query, such as "pk" or
// "author__name", or an annotation name, or an OuterRef, which references the
// query one level further out:
//
//	orm.Query[Comment](db).Filter("post", orm.OuterRef("pk"))
//	orm.Query[Tag](db).Filter("name", orm.OuterRef(orm.OuterRef("name")))
//
// A query that holds an OuterRef can only run as a Subquery or Exists, or as
// the value of a lookup of another query; on its own it fails with an error.
func OuterRef(ref any) Expr {
	switch r := ref.(type) {
	case string:
		return Expr{kind: exprOuterRef, key: r}
	case Expr:
		if r.kind == exprOuterRef {
			return Expr{kind: exprOuterRef, arg: &r}
		}
	}
	return Expr{kind: exprOuterRef, value: ref}
}

// errOuterRef refuses to run a query that references an outer query on its
// own, in the words of Django's ResolvedOuterRef.
var errOuterRef = errors.New(
	"orm: this queryset contains a reference to an outer query and may only be used in a subquery")

// outerRef is a reference of a subquery to its outer query: the node the
// outer query resolved it to, and whether its key crosses a relation, which
// Django calls possibly multivalued.
type outerRef struct {
	node  sqlNode
	multi bool
}

// resolveOuterRef binds an OuterRef in the outer query of c and records it.
func (c *compiler) resolveOuterRef(e Expr) (sqlNode, error) {
	if c.parent == nil {
		return nil, errOuterRef
	}
	var ref Expr
	switch {
	case e.arg != nil:
		ref = *e.arg
	case e.key != "":
		ref = F(e.key)
	default:
		return nil, fmt.Errorf("orm: OuterRef takes a field key or an OuterRef, got %T", e.value)
	}
	node, err := c.parent.resolveExpr(ref)
	if err != nil {
		return nil, err
	}
	c.outerRefs = append(c.outerRefs, outerRef{node: node, multi: strings.Contains(e.key, lookupSep)})
	return outerRefNode{inner: node}, nil
}

// resolveSubquery binds a Subquery or an Exists as part of the query of c.
func (c *compiler) resolveSubquery(e Expr) (sqlNode, error) {
	if e.sub == nil {
		return nil, errors.New("orm: Subquery and Exists require a queryset")
	}
	if e.sub.db != c.q.db {
		return nil, errors.New("orm: subqueries aren't allowed across different databases")
	}
	if e.exists {
		return c.existsNode(e.sub)
	}
	stmt, refs, err := e.sub.subselect(c, "")
	switch {
	case errors.Is(err, errEmptyResultSet):
		return subqueryNode{typ: stmt.items[0].node.vtype(), refs: refs}, nil
	case err != nil:
		return nil, err
	}
	return subqueryNode{stmt: stmt, typ: stmt.items[0].node.vtype(), refs: refs}, nil
}

// existsNode binds Exists of sub as part of the query of c. An Exists of a
// query known to match no row is false.
func (c *compiler) existsNode(sub *query) (sqlNode, error) {
	sc, stmt, err := sub.existsStmt(c)
	switch {
	case errors.Is(err, errEmptyResultSet):
		return paramNode{value: false, literal: true}, nil
	case err != nil:
		return nil, err
	}
	return subqueryNode{stmt: stmt, exists: true, typ: typeBool, refs: sc.outerRefs}, nil
}

// existsStmt builds the statement whose rows tell whether q matches any row,
// as part of the query of parent, or on its own when parent is nil. As
// Django's Query.exists does, it selects the constant 1, unless q is both
// distinct and sliced, where the selected columns decide which rows there
// are. A query that groups its rows by the keys of Values keeps that GROUP
// BY. The statement has no ordering and a limit of one row, and it keeps the
// offset of q. It reports errEmptyResultSet when q matches no row.
func (q *query) existsStmt(parent *compiler) (*compiler, *selectStmt, error) {
	c, err := q.compileIn(parent)
	if err != nil {
		return nil, nil, err
	}
	items := []selectItem{{node: rawNode{sql: "1", typ: typeInt}, alias: "a"}}
	keepSelect := q.distinct && q.sliced()
	var grouped []selectItem
	switch {
	case keepSelect:
		if items, err = c.selectedItems(); err != nil {
			return nil, nil, err
		}
	case q.values != nil && q.values.groups && q.hasAggregates():
		// The statement no longer selects what the rows are grouped by, so
		// the GROUP BY cannot refer to it by position.
		if grouped, err = c.selectedItems(); err != nil {
			return nil, nil, err
		}
		for i := range grouped {
			grouped[i].alias = ""
		}
	}
	stmt, err := c.build(items, false)
	if err != nil {
		return nil, nil, err
	}
	if c.empty {
		return nil, nil, errEmptyResultSet
	}
	if grouped != nil {
		stmt.groupBy = c.groupBy(grouped, nil)
	}
	if !keepSelect {
		stmt.distinct = false
	}
	stmt.forUpdate = false
	if stmt.limit < 0 || stmt.limit > 1 {
		stmt.limit = 1
	}
	return c, stmt, nil
}

// selectedItems returns what the query of c selects as a subquery: the items
// of Values, or the primary key of a queryset of model rows.
func (c *compiler) selectedItems() ([]selectItem, error) {
	if c.q.values != nil {
		items, _, err := c.valuesItems()
		return items, err
	}
	pk := c.q.model.PrimaryKey()
	if pk == nil {
		return nil, fmt.Errorf("orm: model %s has no primary key", c.q.model.Name())
	}
	return []selectItem{{node: c.baseColumn(pk)}}, nil
}

// subqueryNode is a Subquery, "(SELECT ...)", or an Exists, "EXISTS(SELECT
// ...)". A Subquery without a statement matches no row and is NULL. refs are
// the references of the statement to the outer query, its subqueries'
// included.
type subqueryNode struct {
	stmt   *selectStmt
	exists bool
	typ    valueType
	refs   []outerRef
}

// render writes the subquery.
func (n subqueryNode) render(w *sqlWriter) {
	switch {
	case n.stmt == nil:
		w.write("NULL")
		return
	case n.exists:
		w.write("EXISTS(")
	default:
		w.write("(")
	}
	n.stmt.render(w)
	w.write(")")
}

// vtype is the type of the selected column, or bool for Exists.
func (n subqueryNode) vtype() valueType { return n.typ }

// aggregate is false: an aggregate of the statement is computed inside it.
func (subqueryNode) aggregate() bool { return false }

// outerRefNode is an OuterRef inside a subquery: the node the outer query
// resolved it to.
type outerRefNode struct {
	inner sqlNode
}

// render writes the node of the outer query.
func (n outerRefNode) render(w *sqlWriter) { n.inner.render(w) }

// vtype is the type of the referenced value.
func (n outerRefNode) vtype() valueType { return n.inner.vtype() }

// aggregate is false: the value is fixed for each row of the outer query,
// whatever it is computed from.
func (outerRefNode) aggregate() bool { return false }

// refsSubquery reports whether e refers to an annotation of q that holds a
// subquery, directly or through another annotation. Aggregate then computes
// e over a subquery, as Django's get_aggregation does for refs_subquery,
// because a database cannot aggregate the value of a correlated subquery.
func (q *query) refsSubquery(e Expr) bool {
	seen := make(map[string]bool)
	var ref func(key string) bool
	ref = func(key string) bool {
		name, _, _ := strings.Cut(strings.TrimPrefix(key, "-"), lookupSep)
		if seen[name] {
			return false
		}
		seen[name] = true
		for i := len(q.annotations) - 1; i >= 0; i-- {
			if a := q.annotations[i]; a.name == name {
				return mentionsSubquery(a.expr, true, ref)
			}
		}
		return false
	}
	return mentionsSubquery(e, false, ref)
}

// mentionsSubquery reports whether e holds a subquery, when direct is set,
// or a key for which ref reports true.
func mentionsSubquery(e Expr, direct bool, ref func(key string) bool) bool {
	switch e.kind {
	case exprSubquery:
		return direct
	case exprField:
		return ref(e.key)
	}
	for _, x := range []*Expr{e.left, e.right, e.arg, e.def} {
		if x != nil && mentionsSubquery(*x, direct, ref) {
			return true
		}
	}
	for _, x := range e.args {
		if mentionsSubquery(x, direct, ref) {
			return true
		}
	}
	for _, w := range e.whens {
		if condMentionsSubquery(w.Cond, direct, ref) || mentionsSubquery(fieldOrExpr(w.Then), direct, ref) {
			return true
		}
	}
	if e.filter != nil && condMentionsSubquery(*e.filter, direct, ref) {
		return true
	}
	for _, o := range e.order {
		var found bool
		switch t := o.(type) {
		case string:
			found = ref(t)
		case OrderExpr:
			found = mentionsSubquery(t.expr, direct, ref)
		default:
			found = mentionsSubquery(toExpr(o), direct, ref)
		}
		if found {
			return true
		}
	}
	return false
}

// condMentionsSubquery is mentionsSubquery for the keys, values and
// expressions of a condition.
func condMentionsSubquery(c Cond, direct bool, ref func(key string) bool) bool {
	for _, ch := range c.children {
		switch {
		case ch.sub != nil:
			if condMentionsSubquery(*ch.sub, direct, ref) {
				return true
			}
		case ch.expr != nil:
			if mentionsSubquery(*ch.expr, direct, ref) {
				return true
			}
		default:
			if ref(ch.key) {
				return true
			}
			if v, ok := ch.value.(Expr); ok && mentionsSubquery(v, direct, ref) {
				return true
			}
			if v, ok := ch.value.(*Expr); ok && v != nil && mentionsSubquery(*v, direct, ref) {
				return true
			}
		}
	}
	return false
}
