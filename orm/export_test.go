package orm

// CompileSelectForTest exposes the compiled SELECT to external tests so they
// can assert on the generated SQL, such as the joins of select_related.
func CompileSelectForTest[T any](qs *QuerySet[T]) (string, []any, error) {
	return qs.compileSelect()
}

// compileSelect returns the SELECT statement of the queryset.
func (qs *QuerySet[T]) compileSelect() (string, []any, error) {
	sql, args, _, err := qs.q.selectSQL()
	return sql, args, err
}

// compileCount returns the statement Count runs.
func (qs *QuerySet[T]) compileCount() (string, []any, error) {
	if qs.q.err != nil {
		return "", nil, qs.q.err
	}
	sql, args, _, err := qs.q.aggregateSQL([]annotation{{name: countAlias, expr: Count("*")}})
	return sql, args, err
}

// compileDelete returns the statement Delete runs.
func (qs *QuerySet[T]) compileDelete() (string, []any, error) {
	return qs.q.deleteSQL()
}

// compileUpdate returns the statement Update runs for the assignments.
func (qs *QuerySet[T]) compileUpdate(assignments ...any) (string, []any, error) {
	return qs.q.updateSQL(assignments)
}

// WhereForTest describes a condition tree to an external test: a leaf when
// Leaf is set, and otherwise a node that joins Children with AND, or with OR
// when Or is set.
type WhereForTest struct {
	Leaf     string
	Or       bool
	Negated  bool
	Children []WhereForTest
}

// The leaves of a WhereForTest tree.
const (
	// WhereDummy is a condition the database has to evaluate. It renders as
	// "dummy IS NULL".
	WhereDummy = "dummy"
	// WhereNothing is a condition that no row meets.
	WhereNothing = "nothing"
)

// The outcomes CompileWhereForTest reports instead of SQL.
const (
	// WhereMatchesNothing is Django's EmptyResultSet.
	WhereMatchesNothing = "EmptyResultSet"
	// WhereMatchesAll is Django's FullResultSet.
	WhereMatchesAll = "FullResultSet"
)

// node converts the description into a condition tree.
func (n WhereForTest) node() *whereNode {
	out := &whereNode{conn: connAnd, negated: n.Negated}
	if n.Or {
		out.conn = connOr
	}
	for _, ch := range n.Children {
		switch ch.Leaf {
		case WhereDummy:
			lhs := rawNode{sql: WhereDummy}
			out.children = append(out.children, whereChild{pred: &predicate{lookup: lookupIsNull, lhs: lhs, flag: true}})
		case WhereNothing:
			out.children = append(out.children, whereChild{pred: &predicate{empty: true}})
		default:
			out.children = append(out.children, whereChild{node: ch.node()})
		}
	}
	return out
}

// CompileWhereForTest returns what Django's WhereNode.as_sql gives for the
// tree: its SQL, or WhereMatchesNothing or WhereMatchesAll when the outcome is
// known without the database.
func CompileWhereForTest(d Dialect, n WhereForTest) string {
	node := n.node()
	switch node.simplify() {
	case matchNone:
		return WhereMatchesNothing
	case matchAll:
		return WhereMatchesAll
	}
	w := newSQLWriter(d)
	node.render(w)
	return w.String()
}
