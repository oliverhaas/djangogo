package orm

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// compiler turns one query into SQL. Compiling has two phases. The first
// resolves keys into columns, which creates the joins and settles their types;
// the second renders the statement in textual order, binding arguments as they
// are written. The split is needed because a join created late (by an ORDER BY
// key, for example) appears early in the text, in the FROM clause.
type compiler struct {
	q     *query
	d     Dialect
	joins *joinSet
	// annots holds the resolved annotations by name; annotNames keeps their
	// order. resolved is the number of the query's annotations resolved so
	// far.
	annots     map[string]sqlNode
	annotNames []string
	resolved   int
	// used collects the joins touched while one filter condition is resolved.
	used []*join
	// reuse tracks multi-valued joins reusable within the current Filter call;
	// nil outside Filter/Exclude (OrderBy, Values, Annotate, SelectRelated).
	reuse map[*join]bool
	// noSplit keeps a negated condition across a multi-valued relation a
	// join, as Django's split_subq=False does for the condition of an
	// aggregate's Filter.
	noSplit bool
	// where and having are the resolved filter conditions.
	where, having *whereNode
	// empty reports that the filters can match no row. The statement builders
	// then return errEmptyResultSet, after they validated the rest of the query.
	empty bool
	// m2mPrefetchJoin holds the through-table join used by an M2M prefetch
	// query so rowsStmt selects its parent FK column.
	m2mPrefetchJoin *join
	// parent is the compiler of the query that this one is a subquery of, nil
	// for a query of its own. outerRefs are the references to it, which a
	// GROUP BY of the parent needs.
	parent    *compiler
	outerRefs []outerRef
	// groupKeys are the resolved valuesSpec.groupKeys.
	groupKeys []sqlNode
}

// compile runs the first phase for q: it resolves the annotations and the
// filters in call order.
func (q *query) compile() (*compiler, error) { return q.compileIn(nil) }

// compileIn runs the first phase for q as a subquery of the query of parent,
// whose fields OuterRef reads, or for a query of its own when parent is nil.
// The tables of a subquery take aliases with the prefix after the one of
// parent.
func (q *query) compileIn(parent *compiler) (*compiler, error) {
	if q.err != nil {
		return nil, q.err
	}
	js := newJoinSet(q.model)
	if parent != nil {
		js = newSubqueryJoinSet(q.model, nextAliasPrefix(parent.joins.aliasPrefix), 0)
	}
	c := &compiler{
		q:      q,
		d:      q.db.Dialect(),
		joins:  js,
		annots: make(map[string]sqlNode, len(q.annotations)),
		parent: parent,
	}
	if err := c.addFilters(); err != nil {
		return nil, err
	}
	if err := c.resolveAnnotations(len(q.filters)); err != nil {
		return nil, err
	}
	if q.values != nil && q.values.groups {
		for _, key := range q.values.groupKeys {
			n, err := c.resolveExpr(F(key))
			if err != nil {
				return nil, err
			}
			c.groupKeys = append(c.groupKeys, n)
		}
	}
	// A limit of zero rows matches nothing, whatever the filters say.
	c.empty = c.empty || q.limit == 0
	return c, nil
}

// resolveAnnotations resolves the annotations that were added before the
// given number of Filter and Exclude calls and are not resolved yet. An
// annotation of a name that is annotated already replaces the earlier one in
// its place, and reads it by that name, as Django's annotate does.
func (c *compiler) resolveAnnotations(filters int) error {
	for _, a := range c.q.annotations[c.resolved:] {
		if a.filters > filters {
			return nil
		}
		n, err := c.resolveExpr(a.expr)
		if err != nil {
			return err
		}
		if _, ok := c.annots[a.name]; !ok {
			c.annotNames = append(c.annotNames, a.name)
		}
		c.annots[a.name] = n
		c.resolved++
	}
	return nil
}

// column returns the column a resolved path ends on, joining the tables on
// the way.
func (c *compiler) column(p fieldPath) colNode {
	joins := c.joins.setup(p.hops, c.reuse)
	c.used = append(c.used, joins...)
	j := c.joins.base
	if len(joins) > 0 {
		j = joins[len(joins)-1]
	}
	c.used = append(c.used, c.joins.reserve(j, p.trimmed, c.reuse)...)
	col := colNode{join: j, field: p.field}
	if j == c.joins.base {
		col.value = c.q.against[p.field]
	}
	return col
}

// baseColumn returns field f of the query's own table.
func (c *compiler) baseColumn(f *Field) colNode {
	return colNode{join: c.joins.base, field: f}
}

// resolveExpr binds an expression to the query: field references become
// columns (or the annotation of that name) and literals become parameters.
func (c *compiler) resolveExpr(e Expr) (sqlNode, error) {
	if e.distinct && e.kind != exprAggregate {
		return nil, errors.New("orm: Distinct applies to aggregates only")
	}
	if e.filter != nil && e.kind != exprAggregate {
		return nil, errors.New("orm: Filter applies to aggregates only")
	}
	if e.def != nil && e.kind != exprCase && e.kind != exprAggregate {
		return nil, errors.New("orm: Default applies to Case and aggregates only")
	}
	if e.order != nil && e.kind != exprAggregate {
		return nil, errors.New("orm: OrderBy applies to aggregates only")
	}
	switch e.kind {
	case exprField:
		if n, ok := c.annots[e.key]; ok {
			return n, nil
		}
		p, err := resolveFieldPath(c.q.model, e.key, c.annotNames)
		if err != nil {
			return nil, err
		}
		return c.keyNode(p)
	case exprValue:
		switch e.value.(type) {
		case Decimal, Date, Time, UUID:
			// Keep the Go type, which gives the type of the value. The
			// driver binds it by its Value method.
			return paramNode{value: e.value, literal: true}, nil
		}
		v, err := plainValue(c.q.db.Registry(), nil, e.value)
		if err != nil {
			return nil, err
		}
		return paramNode{value: v, literal: v != nil}, nil
	case exprBinary:
		left, err := c.resolveExpr(*e.left)
		if err != nil {
			return nil, err
		}
		right, err := c.resolveExpr(*e.right)
		if err != nil {
			return nil, err
		}
		if err := c.checkCombinedTypes(e.op, left.vtype(), right.vtype()); err != nil {
			return nil, err
		}
		n := binaryNode{op: e.op, left: left, right: right}
		if err := c.setTemporalTemplate(&n); err != nil {
			return nil, err
		}
		return n, nil
	case exprAggregate:
		return c.resolveAggregate(e)
	case exprFunc:
		arg, err := c.resolveExpr(*e.arg)
		if err != nil {
			return nil, err
		}
		typ := arg.vtype()
		if e.typed {
			typ = fieldValueType(&Field{Kind: e.output})
		}
		return funcNode{fn: e.fn, arg: arg, typ: typ}, nil
	case exprNode:
		return e.node, nil
	case exprCall:
		return c.resolveCall(e)
	case exprCast:
		arg, err := c.resolveExpr(*e.arg)
		if err != nil {
			return nil, err
		}
		return convertNode{inner: arg, to: e.castTo, input: valueKind(arg.vtype())}, nil
	case exprWrap:
		arg, err := c.resolveExpr(*e.arg)
		if err != nil {
			return nil, err
		}
		return typedNode{node: arg, typ: fieldValueType(e.castTo)}, nil
	case exprCase:
		return c.resolveCase(e)
	case exprNot:
		arg, err := c.resolveExpr(*e.arg)
		if err != nil {
			return nil, err
		}
		if arg.vtype() != typeBool {
			return nil, errNegateNonConditional
		}
		return notNode{inner: arg}, nil
	case exprSubquery:
		return c.resolveSubquery(e)
	case exprOuterRef:
		return c.resolveOuterRef(e)
	case exprStar:
		return nil, errors.New(`orm: "*" is only valid as the argument of Count`)
	default:
		return nil, errors.New("orm: empty expression")
	}
}

// errNegateNonConditional refuses the negation of an expression that is not
// boolean, as Django's NegatedExpression does.
var errNegateNonConditional = errors.New("orm: cannot negate non-conditional expressions")

// setTemporalTemplate gives n the SQL of the dialect for the combination of a
// duration with a value of another type, or for the difference of two dates,
// two datetimes or two times of day, as Django's CombinedExpression becomes a
// DurationExpression or a TemporalSubtraction. A side of unknown type counts
// as another type than a duration. On a dialect whose duration arithmetic is
// a function, a product or a quotient of a duration with a value that is not
// a number is an error, as in Django's DurationExpression.as_sqlite.
func (c *compiler) setTemporalTemplate(n *binaryNode) error {
	td, ok := c.d.(TemporalDialect)
	if !ok {
		return nil
	}
	left, right := n.left.vtype(), n.right.vtype()
	switch {
	case (left == typeDuration || right == typeDuration) && left != right:
		tmpl, err := td.DurationSQL(n.op)
		if err != nil || tmpl == "" {
			return err
		}
		numeric := func(t valueType) bool {
			return t == typeInt || t == typeFloat || t == typeDecimal || t == typeDuration
		}
		if (n.op == "*" || n.op == "/") && left != typeUnknown && right != typeUnknown &&
			(!numeric(left) || !numeric(right)) {
			return fmt.Errorf("orm: invalid arguments for operator %s", n.op)
		}
		n.tmpl = tmpl
	case n.op == "-" && left == right && (left == typeDate || left == typeTime || left == typeTimeOfDay):
		n.tmpl = td.SubtractTemporalSQL(valueKind(left))
	}
	return nil
}

// checkCombinedTypes refuses arithmetic between a float and a decimal, whose
// result type Django cannot infer either.
func (c *compiler) checkCombinedTypes(op string, left, right valueType) error {
	if !slices.Contains([]string{"+", "-", "*", "/", "%"}, op) {
		return nil
	}
	names := map[valueType]string{typeFloat: "FloatField", typeDecimal: "DecimalField"}
	if left == right || names[left] == "" || names[right] == "" {
		return nil
	}
	return &FieldError{
		Model: c.q.model.Name(),
		Message: fmt.Sprintf("cannot infer type of '%s' expression involving these types: %s, %s; "+
			"Cast one of them", op, names[left], names[right]),
	}
}

// errStarFilter refuses Count("*") with a filter, as Django does.
var errStarFilter = errors.New("orm: Star cannot be used with filter. Please specify a field.") //nolint:revive,staticcheck // Django's message

// resolveAggregate binds an aggregate call. An aggregate of an aggregate is
// refused here; Aggregate computes one over an annotated queryset through a
// subquery instead. An aggregate with a Default is COALESCE(aggregate,
// default) with the type of the aggregate, as Django wraps it in Coalesce.
func (c *compiler) resolveAggregate(e Expr) (sqlNode, error) {
	if err := checkAggregate(e); err != nil {
		return nil, err
	}
	if e.arg.kind == exprStar {
		if err := checkStar(e); err != nil {
			return nil, err
		}
		return c.aggregateDefault(e, aggNode{fn: e.fn})
	}
	arg, err := c.resolveExpr(*e.arg)
	if err != nil {
		return nil, err
	}
	if arg.aggregate() {
		return nil, c.aggregateOfAggregate(e)
	}
	agg := aggNode{fn: e.fn, arg: arg, distinct: e.distinct}
	if agg.extra, err = c.resolveAggregateArgs(e); err != nil {
		return nil, err
	}
	if agg.order, err = c.aggregateOrder(e.order); err != nil {
		return nil, err
	}
	var n sqlNode = agg
	where, match, err := c.condition(e.filter)
	switch {
	case err != nil:
		return nil, err
	case match == matchNone:
		n = emptyAggNode{agg: agg}
	case match == matchSome:
		agg.filter = condNode{where: where}
		n = agg
	}
	return c.aggregateDefault(e, n)
}

// checkStar refuses an aggregate of "*" other than a plain Count.
func checkStar(e Expr) error {
	if e.filter != nil {
		return errStarFilter
	}
	if e.fn != "COUNT" || e.distinct {
		return errors.New(`orm: "*" is only valid as the argument of a plain Count`)
	}
	return nil
}

// checkAggregate refuses what Django's Aggregate.__init__ refuses: a Default
// of Count, Distinct on an aggregate other than Count, Sum, Avg and
// StringAgg, and an OrderBy of an aggregate other than StringAgg.
func checkAggregate(e Expr) error {
	name := aggregateName(e.fn)
	switch {
	case e.def != nil && e.fn == "COUNT":
		return fmt.Errorf("orm: %s does not allow default", name)
	case e.distinct && !slices.Contains([]string{"COUNT", "SUM", "AVG", fnStringAgg}, e.fn):
		return fmt.Errorf("orm: %s does not allow distinct", name)
	case len(e.order) > 0 && e.fn != fnStringAgg:
		return fmt.Errorf("orm: %s does not allow order_by", name)
	}
	return nil
}

// aggregateOfAggregate returns the error of an aggregate whose argument holds
// an aggregate, in Django's wording: Cannot compute Avg('n'): 'n' is an
// aggregate.
func (c *compiler) aggregateOfAggregate(e Expr) error {
	name := "its argument"
	if e.arg.kind == exprField {
		name = "'" + e.arg.key + "'"
	}
	return &FieldError{
		Model:   c.q.model.Name(),
		Message: fmt.Sprintf("cannot compute %s(%s): %s is an aggregate", aggregateName(e.fn), name, name),
	}
}

// resolveAggregateArgs binds the arguments of an aggregate after the first:
// the delimiter of a StringAgg. SQLite takes DISTINCT only in an aggregate of
// one argument, and Django refuses it there.
func (c *compiler) resolveAggregateArgs(e Expr) ([]sqlNode, error) {
	if len(e.args) == 0 {
		return nil, nil
	}
	if e.distinct && c.d.Name() == "sqlite" {
		return nil, fmt.Errorf("orm: %s does not support distinct with multiple expressions on this database backend",
			aggregateName(e.fn))
	}
	out := make([]sqlNode, len(e.args))
	for i, a := range e.args {
		n, err := c.resolveExpr(a)
		if err != nil {
			return nil, err
		}
		out[i] = n
	}
	return out, nil
}

// aggregateOrder binds the terms of an aggregate's OrderBy, Django's
// AggregateOrderBy: a key, "-" and a key, an Expr or an OrderExpr. A key is
// the column or annotation it names; the default ordering of a related model
// does not apply.
func (c *compiler) aggregateOrder(terms []any) ([]orderItem, error) {
	if len(terms) == 0 {
		return nil, nil
	}
	parsed, err := parseOrderArgs(terms)
	if err != nil {
		return nil, err
	}
	out := make([]orderItem, len(parsed))
	for i, t := range parsed {
		x := F(t.key)
		if t.expr != nil {
			x = *t.expr
		} else if t.random {
			return nil, errors.New(`orm: "?" cannot order the values of an aggregate`)
		}
		n, err := c.resolveExpr(x)
		if err != nil {
			return nil, err
		}
		out[i] = orderItem{node: n, desc: t.desc, nullsFirst: t.nullsFirst, nullsLast: t.nullsLast}
	}
	return out, nil
}

// aggregateDefault returns the aggregate n of e, wrapped in COALESCE with the
// Default of e when it has one.
func (c *compiler) aggregateDefault(e Expr, n sqlNode) (sqlNode, error) {
	if e.def == nil {
		return n, nil
	}
	d, err := c.resolveExpr(*e.def)
	if err != nil {
		return nil, err
	}
	return typedNode{node: callNode{fn: "COALESCE", args: []sqlNode{n, d}}, typ: n.vtype()}, nil
}

// condition resolves the condition of an aggregate's Filter or of a When and
// reports what it is known to match. A condition that every row meets leaves
// an aggregate plain, and one that no row meets makes it the value of the
// aggregate over no rows, as Django compiles AggregateFilter. The condition
// reuses any join of the query, and the joins it needs become LEFT OUTER, so
// that it removes no row of the query. A negated condition across a
// multi-valued relation stays a join. Django's Q.resolve_expression does both.
func (c *compiler) condition(cond *Cond) (*whereNode, matchKind, error) {
	if cond == nil {
		return nil, matchAll, nil
	}
	if cond.err != nil {
		return nil, matchSome, cond.err
	}
	reuse, used, noSplit := c.reuse, c.used, c.noSplit
	c.reuse, c.used, c.noSplit = nil, nil, true
	where, inner, err := c.addQ(*cond, false)
	c.reuse, c.used, c.noSplit = reuse, used, noSplit
	if err != nil {
		return nil, matchSome, err
	}
	c.joins.promote(inner)
	return where, where.simplify(), nil
}

// selectItem is one entry of a SELECT list, with an optional output alias.
type selectItem struct {
	node  sqlNode
	alias string
}

// orderItem is one resolved ORDER BY term. alias is set when the term is a
// selected expression, which is then ordered by its position in the SELECT
// list.
type orderItem struct {
	node       sqlNode
	alias      string
	desc       bool
	random     bool
	nullsFirst bool
	nullsLast  bool
}

// selectStmt is a resolved SELECT statement, ready to render.
type selectStmt struct {
	c         *compiler
	items     []selectItem
	distinct  bool
	groupBy   []sqlNode
	order     []orderItem
	limit     int // -1 means no limit
	offset    int
	forUpdate bool
}

// build assembles the SELECT of items with the query's DISTINCT, GROUP BY,
// slice, and locking. The ordering is included when ordered is set; a
// statement whose row order cannot matter leaves it out.
func (c *compiler) build(items []selectItem, ordered bool) (*selectStmt, error) {
	s := &selectStmt{
		c:         c,
		items:     items,
		distinct:  c.q.distinct,
		limit:     c.q.limit,
		offset:    c.q.offset,
		forUpdate: c.q.forUpdate,
	}
	if ordered {
		order, err := c.resolveOrder(items)
		if err != nil {
			return nil, err
		}
		s.order = order
		if s.distinct {
			s.items = c.withOrderColumns(s.items, order)
		}
	}
	s.groupBy = c.groupBy(s.items, s.order)
	return s, nil
}

// resolveOrder resolves the ORDER BY terms. A term is an annotation name, a
// field key, which may span relations, or an expression. A relation addressed
// by its attribute name (not "<attr>_id") inherits the target model's default
// ordering, as in Django. An expression is ordered by once: its first term
// wins, because a later one could never matter.
func (c *compiler) resolveOrder(items []selectItem) ([]orderItem, error) {
	selected := make(map[string]string, 2*len(items))
	for _, it := range items {
		if it.alias != "" {
			selected[it.alias] = it.alias
			if key := selectedExprKey(c.d, it.node); selected[key] == "" {
				selected[key] = it.alias
			}
		}
	}
	terms := c.q.effectiveOrders()
	out := make([]orderItem, 0, len(terms))
	seen := make(map[string]bool, len(terms))
	stack := map[*Model]bool{c.q.model: true}
	for _, t := range terms {
		var err error
		out, err = c.appendOrderTerm(out, t, selected, seen, stack)
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

// selectedExprKey returns the key under which resolveOrder records the
// expression of a selected item with an alias, apart from the alias itself.
func selectedExprKey(d Dialect, n sqlNode) string { return "expr " + nodeKey(d, n) }

// appendOrderTerm resolves one orderTerm, expanding a related model's default
// Meta.Ordering when the key ends on a relation attribute rather than its "_id"
// column. selected maps the alias of each selected item that has one, and the
// selectedExprKey of its expression, to the alias: a term on a selected
// expression is ordered by its position, as in Django. stack
// tracks models on the active expansion chain to break cycles while allowing
// sibling branches to reference the same model.
func (c *compiler) appendOrderTerm(out []orderItem, t orderTerm, selected map[string]string, seen map[string]bool, stack map[*Model]bool) ([]orderItem, error) {
	if t.random {
		return append(out, orderItem{random: true}), nil
	}
	if t.expr != nil {
		return c.appendOrderExpr(out, t, selected, seen)
	}
	item := orderItem{desc: t.desc}
	// An annotation is identified by its name: two of them may differ
	// only in a bound value, which the SQL text does not show.
	key := "annotation " + t.key
	if n, ok := c.annots[t.key]; ok {
		item.node = n
		if selected[t.key] != "" {
			item.alias = t.key
		}
	} else if n, ok, err := c.annotationTransform(t.key); ok || err != nil {
		if err != nil {
			return nil, err
		}
		item.node = n
		key = nodeKey(c.d, n)
	} else {
		p, err := resolveFieldPath(c.q.model, t.key, c.annotNames)
		if err != nil {
			return nil, err
		}
		if target := relatedOrderingTarget(t.key, p); target != nil && !stack[target] {
			stack[target] = true
			for _, sub := range target.ordering {
				out, err = c.appendOrderTerm(out, relatedOrderTerm(t, sub), selected, seen, stack)
				if err != nil {
					delete(stack, target)
					return nil, err
				}
			}
			delete(stack, target)
			return out, nil
		}
		item.node, err = c.keyNode(p)
		if err != nil {
			return nil, err
		}
		key = nodeKey(c.d, item.node)
		item.alias = selected[selectedExprKey(c.d, item.node)]
	}
	if seen[key] {
		return out, nil
	}
	seen[key] = true
	return append(out, item), nil
}

// annotationTransform resolves a key that names an annotation followed by
// transforms of its value, such as "sum_pages__mod100", as Django's
// names_to_path reads an annotation as a field. It reports false for a key
// that does not start with an annotation or goes on with a name that is not a
// transform, so that the key resolves as a field path.
func (c *compiler) annotationTransform(key string) (sqlNode, bool, error) {
	parts := strings.Split(key, lookupSep)
	for i := len(parts) - 1; i >= 1; i-- {
		n, ok := c.annots[strings.Join(parts[:i], lookupSep)]
		if !ok {
			continue
		}
		for _, name := range parts[i:] {
			kind := valueKind(n.vtype())
			if !slices.Contains(transformsOf(kind), name) {
				return nil, false, nil
			}
			var err error
			if n, err = c.transform(n, kind, name); err != nil {
				return nil, true, err
			}
		}
		return n, true, nil
	}
	return nil, false, nil
}

// relatedOrderTerm returns the term sub of the default ordering of the model
// that t orders by, as read from the model of t. A key is prefixed with the
// key of t, and its direction is flipped when t is descending. An expression
// reads its fields through the relation. An expression with a direction, an
// OrderExpr, keeps it, and any other takes the direction of t, as in Django.
func relatedOrderTerm(t, sub orderTerm) orderTerm {
	switch {
	case sub.random:
		return sub
	case sub.expr != nil:
		e := sub.expr.prefixed(t.key + lookupSep)
		if sub.directed {
			sub.expr = &e
			return sub
		}
		return orderTerm{expr: &e, desc: t.desc}
	default:
		return orderTerm{key: t.key + lookupSep + sub.key, desc: t.desc != sub.desc}
	}
}

// appendOrderExpr resolves an orderTerm of an expression. F of an annotation
// name orders by the annotation, and an expression that is selected under an
// alias is ordered by its position. F of a relation orders by its column, not by
// the default ordering of the related model, as in Django.
func (c *compiler) appendOrderExpr(out []orderItem, t orderTerm, selected map[string]string, seen map[string]bool) ([]orderItem, error) {
	item := orderItem{desc: t.desc, nullsFirst: t.nullsFirst, nullsLast: t.nullsLast}
	var key string
	if n, ok := c.annots[t.expr.key]; ok && t.expr.kind == exprField {
		item.node = n
		item.alias = selected[t.expr.key]
		key = "annotation " + t.expr.key
	} else {
		n, err := c.resolveExpr(*t.expr)
		if err != nil {
			return nil, err
		}
		item.node = n
		key = nodeKey(c.d, n)
		item.alias = selected[selectedExprKey(c.d, n)]
	}
	if seen[key] {
		return out, nil
	}
	seen[key] = true
	return append(out, item), nil
}

// relatedOrderingTarget returns the related target model whose Meta.Ordering
// should be inherited for ordering key, or nil when key addresses a plain
// field, a relation's "_id" attname/column, or a target without Meta.Ordering.
func relatedOrderingTarget(key string, p fieldPath) *Model {
	if !p.relation {
		return nil
	}
	if p.targetModel != nil {
		if len(p.targetModel.ordering) == 0 {
			return nil
		}
		return p.targetModel
	}
	if p.reverse != nil {
		if len(p.model.ordering) == 0 {
			return nil
		}
		return p.model
	}
	if p.field == nil || p.field.Rel == nil || p.field.Rel.Target == nil {
		return nil
	}
	if len(p.field.Rel.Target.ordering) == 0 {
		return nil
	}
	last := key
	if i := strings.LastIndex(key, lookupSep); i >= 0 {
		last = key[i+len(lookupSep):]
	}
	if last == attname(p.field) || last == p.field.Column {
		return nil
	}
	return p.field.Rel.Target
}

// withOrderColumns appends the ORDER BY expressions that are not selected.
// SELECT DISTINCT may only be ordered by selected expressions, so they are
// added to the list, as Django adds them; the callers ignore the extra
// columns when they scan.
func (c *compiler) withOrderColumns(items []selectItem, order []orderItem) []selectItem {
	seen := make(map[string]bool, len(items))
	for _, it := range items {
		seen[nodeKey(c.d, it.node)] = true
	}
	for _, o := range order {
		if o.random || o.alias != "" {
			continue
		}
		key := nodeKey(c.d, o.node)
		if !seen[key] {
			seen[key] = true
			items = append(items, selectItem{node: o.node})
		}
	}
	return items
}

// groupBy returns the GROUP BY columns of a statement, or nil when the query
// has no aggregate annotation. A query that called Values before annotating
// groups by the values columns (Django's values().annotate() idiom), and by
// the keys of an earlier Values that grouped (valuesSpec.groupKeys); any other
// query groups by every column of its own table, so that each row stays one
// group. The columns of the remaining selected and ordering expressions are
// added because PostgreSQL requires them.
//
// An expression that is selected with an alias is grouped by its position in
// the SELECT list, as Django does. PostgreSQL matches a grouped expression by
// its text, and a transform written twice binds its parameters twice, under
// different placeholders.
func (c *compiler) groupBy(items []selectItem, order []orderItem) []sqlNode {
	if !c.q.hasAggregates() {
		return nil
	}
	positions := make(map[string]int)
	for i, it := range items {
		if it.alias == "" {
			continue
		}
		if key := nodeKey(c.d, it.node); positions[key] == 0 {
			positions[key] = i + 1
		}
	}
	var out []sqlNode
	seen := make(map[string]bool)
	addCols := func(cols []sqlNode) {
		for _, col := range cols {
			key := nodeKey(c.d, col)
			if seen[key] {
				continue
			}
			seen[key] = true
			if pos := positions[key]; pos > 0 {
				out = append(out, rawNode{sql: strconv.Itoa(pos)})
				continue
			}
			out = append(out, col)
		}
	}
	grouped := c.q.values != nil && c.q.values.groups
	add := func(n sqlNode) {
		// Values groups by a subquery that reads the outer query as a
		// whole, as Django's set_group_by groups by a reference to it.
		if sq, ok := n.(subqueryNode); ok && grouped && len(sq.refs) > 0 {
			addCols([]sqlNode{sq})
			return
		}
		addCols(columnsOf(n))
	}
	if !grouped {
		for _, f := range c.q.model.fields {
			add(c.baseColumn(f))
		}
	}
	for _, n := range c.groupKeys {
		add(n)
	}
	for _, it := range items {
		add(it.node)
	}
	for _, o := range order {
		if !o.random {
			add(o.node)
		}
	}
	return out
}

// columnsOf returns the columns a non-aggregate expression reads, which are
// what a GROUP BY must list for it. The arguments of an aggregate are not
// grouped.
func columnsOf(n sqlNode) []sqlNode {
	switch v := n.(type) {
	case colNode:
		return []sqlNode{v}
	case binaryNode:
		// Django groups by the expression as a whole: grouping pages % 10 by
		// pages would split a group into one group per page count.
		if !v.aggregate() {
			return []sqlNode{v}
		}
		return append(columnsOf(v.left), columnsOf(v.right)...)
	case transformNode:
		// Django groups by the transform itself: grouping by its column would
		// split a group such as one year into one group per value.
		return []sqlNode{v}
	case funcNode:
		if v.aggregate() {
			return columnsOf(v.arg)
		}
		return []sqlNode{v}
	case callNode, concatNode, convertNode, caseNode:
		// Django groups by a function or a Case as a whole, and by the
		// columns of its parts when it holds an aggregate.
		if !n.aggregate() {
			return []sqlNode{n}
		}
		var out []sqlNode
		for _, p := range nodeParts(n) {
			out = append(out, columnsOf(p)...)
		}
		return out
	case condNode:
		var out []sqlNode
		for _, p := range v.where.operands() {
			out = append(out, columnsOf(p)...)
		}
		return out
	case typedNode:
		return columnsOf(v.node)
	case notNode:
		return columnsOf(v.inner)
	case subqueryNode:
		// A subquery is grouped by the columns of the outer query it reads,
		// or as a whole when one of them crosses a relation, as Django's
		// Query.get_group_by_cols does.
		var out []sqlNode
		for _, r := range v.refs {
			if r.multi {
				return []sqlNode{v}
			}
			out = append(out, columnsOf(r.node)...)
		}
		return out
	default:
		// An OuterRef is a value of the outer query, which this query does
		// not group by.
		return nil
	}
}

// render writes the statement.
func (s *selectStmt) render(w *sqlWriter) {
	c := s.c
	w.write("SELECT ")
	if s.distinct {
		w.write("DISTINCT ")
	}
	for i, it := range s.items {
		if i > 0 {
			w.write(", ")
		}
		it.node.render(w)
		if it.alias != "" {
			w.write(" AS ")
			w.ident(it.alias)
		}
	}
	w.write(" FROM ")
	c.renderFrom(w)
	if !c.where.empty() {
		w.write(" WHERE ")
		c.where.render(w)
	}
	if len(s.groupBy) > 0 {
		w.write(" GROUP BY ")
		for i, n := range s.groupBy {
			if i > 0 {
				w.write(", ")
			}
			n.render(w)
		}
	}
	if !c.having.empty() {
		w.write(" HAVING ")
		c.having.render(w)
	}
	s.renderOrder(w)
	renderLimit(w, s.limit, s.offset)
	if s.forUpdate && dialectFeatures(c.d).SelectForUpdate {
		w.write(" FOR UPDATE")
	}
}

// position returns the 1-based position of the item selected under alias, or
// 0 when no item has it.
func (s *selectStmt) position(alias string) int {
	for i, it := range s.items {
		if it.alias == alias {
			return i + 1
		}
	}
	return 0
}

// renderOrder writes the ORDER BY clause, if any.
func (s *selectStmt) renderOrder(w *sqlWriter) {
	for i, o := range s.order {
		if i == 0 {
			w.write(" ORDER BY ")
		} else {
			w.write(", ")
		}
		switch {
		case o.random:
			w.write("RANDOM()")
			continue
		case o.alias != "" && s.position(o.alias) > 0:
			// Django orders by the position (PositionRef): a column
			// of SelectRelated can share the alias.
			w.write(strconv.Itoa(s.position(o.alias)))
		case o.alias != "":
			w.ident(o.alias)
		default:
			o.node.render(w)
		}
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
}

// renderFrom writes the base table and the joins, in creation order, which
// always puts a table after the one it is joined to.
func (c *compiler) renderFrom(w *sqlWriter) {
	w.ident(c.joins.base.model.Table())
	if c.joins.base.alias != c.joins.base.model.Table() {
		w.write(" ")
		w.ident(c.joins.base.alias)
	}
	for _, j := range c.joins.joins {
		if !j.read {
			continue
		}
		if j.outer {
			w.write(" LEFT OUTER JOIN ")
		} else {
			w.write(" INNER JOIN ")
		}
		w.ident(j.model.Table())
		if j.alias != j.model.Table() {
			w.write(" ")
			w.ident(j.alias)
		}
		w.write(" ON (")
		w.column(j.parent.alias, j.fromColumn)
		w.write(" = ")
		if typ := genericCastType(w.d, j.generic, j.genericOnParent); typ != "" {
			w.write("CAST(")
			w.column(j.alias, j.toColumn)
			w.write(" AS " + typ + ")")
		} else {
			w.column(j.alias, j.toColumn)
		}
		if j.generic != nil {
			ctAlias := j.alias
			if j.genericOnParent {
				ctAlias = j.parent.alias
			}
			w.write(" AND ")
			renderContentTypeRestriction(w, ctAlias, j.generic)
		}
		w.write(")")
	}
}

// genericCastType returns the type that the far column of a hop of the
// generic relation gr is cast to when it is compared with the near one, or ""
// when it is compared as it is. The hop goes from the primary key to the
// object id, or back with onParent. Only a generic relation compares columns
// of different types, and only a dialect with CastJoinColumns needs the cast.
func genericCastType(d Dialect, gr *GenericRelation, onParent bool) string {
	if gr == nil || !dialectFeatures(d).CastJoinColumns {
		return ""
	}
	from, to := gr.Model.pk, gr.ObjectID
	if onParent {
		from, to = to, from
	}
	if typ := castType(d, from); typ != castType(d, to) {
		return typ
	}
	return ""
}

// castType returns the column type of f without the column constraints, the
// type that a CAST to the type of f names. An auto primary key casts to its
// integer type.
func castType(d Dialect, f *Field) string {
	bare := *f
	bare.Column, bare.Null, bare.Unique, bare.PrimaryKey = "", true, false, false
	if bare.Kind == KindAuto {
		bare.Kind = KindBigInt
	}
	return d.ColumnType(&bare)
}

// renderLimit writes the LIMIT and OFFSET clauses. An offset without a limit
// needs a stand-in limit on a dialect that cannot write OFFSET alone.
func renderLimit(w *sqlWriter, limit, offset int) {
	switch {
	case limit >= 0:
		w.write(" LIMIT " + strconv.Itoa(limit))
		if offset > 0 {
			w.write(" OFFSET " + strconv.Itoa(offset))
		}
	case offset > 0:
		if noLimit := dialectFeatures(w.d).NoLimitValue; noLimit != "" {
			w.write(" LIMIT " + noLimit)
		}
		w.write(" OFFSET " + strconv.Itoa(offset))
	}
}

// sliced reports whether the query has a limit or an offset.
func (q *query) sliced() bool { return q.limit >= 0 || q.offset > 0 }

// subselect returns the statement of q as a subquery of the query of parent,
// and the references of q to that query. It selects the single Values column,
// or the primary key of a queryset of model rows. lookup names the lookup
// whose value q is in the error for a queryset that selects several columns,
// and is empty for a Subquery. A query that can match no row returns its
// statement with errEmptyResultSet.
func (q *query) subselect(parent *compiler, lookup string) (*selectStmt, []outerRef, error) {
	c, err := q.compileIn(parent)
	if err != nil {
		return nil, nil, err
	}
	items, err := c.selectedItems()
	if err != nil {
		return nil, nil, err
	}
	if len(items) != 1 {
		if lookup == "" {
			return nil, nil, fmt.Errorf("orm: the QuerySet of a Subquery must have 1 selected field (received %d)",
				len(items))
		}
		return nil, nil, fmt.Errorf(
			"orm: the QuerySet value for the %s lookup must have 1 selected fields (received %d)",
			lookup, len(items))
	}
	// The order of a subquery only matters when it is sliced.
	s, err := c.build(items, q.sliced())
	if err != nil {
		return nil, nil, err
	}
	if c.empty {
		return s, c.outerRefs, errEmptyResultSet
	}
	return s, c.outerRefs, nil
}

// existsSQL returns the statement that reports whether q matches any row.
func (q *query) existsSQL() (string, []any, error) {
	c, s, err := q.existsStmt(nil)
	if err != nil {
		return "", nil, err
	}
	w := newSQLWriter(c.d)
	w.write("SELECT EXISTS(")
	s.render(w)
	w.write(")")
	return w.String(), w.args, nil
}
