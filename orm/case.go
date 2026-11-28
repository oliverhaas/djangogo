package orm

import "errors"

// When is one branch of a Case: the value Then for the rows that match Cond,
// Django's When. Then is a field key or an Expr, as the arguments of the
// functions are; pass Value("text") for a literal. A Go value that is not a
// string or an Expr is a literal too:
//
//	orm.When{Cond: orm.Q("integer", 1), Then: orm.Value("one")}
//	orm.When{Cond: orm.Q("integer__gt", 2), Then: orm.F("integer").Add(1)}
//	orm.When{Cond: orm.Q("account_type", "G"), Then: 5}
type When struct {
	// Cond is the condition of the branch. It cannot be empty.
	Cond Cond
	// Then is the value of the branch.
	Then any
}

// Case returns the value of the first branch whose condition a row meets,
// Django's Case. A row that meets none gets the value of Default, or NULL
// without one:
//
//	Annotate("discount", orm.Case(
//		orm.When{Cond: orm.Q("account_type", "G"), Then: orm.Value("5%")},
//		orm.When{Cond: orm.Q("account_type", "P"), Then: orm.Value("10%")},
//	).Default(orm.Value("0%")))
//
// A Case is an Expr: it can be annotated, aggregated (Sum(Case(...))),
// compared in a filter, or assigned in an Update. The conditions take the
// keys and lookups of Filter. One that spans a relation joins it with a LEFT
// OUTER JOIN, so that a row without a related row still gets a value, and a
// negated one keeps the join, as the condition of an aggregate's Filter
// does. In an Update a condition can only read the updated row.
func Case(whens ...When) Expr {
	return Expr{kind: exprCase, whens: whens}
}

// Default sets the value of a Case for the rows that meet no branch, or the
// value of an aggregate over no rows, Django's default argument. Of a Case, x
// is a field key or an Expr, or another Go value as a literal. Of an
// aggregate, x is an Expr or a Go value, a string included, as a literal, and
// the aggregate becomes COALESCE(aggregate, x) with the type of the
// aggregate:
//
//	Aggregate(ctx, "total", orm.Sum("pages").Default(0))
//
// Of Lag and Lead, x is the value for a row that has no row at the offset,
// a field key or an Expr, or another Go value as a literal.
//
// Default is an error on Count, whose value over no rows is 0, and on an
// expression that is neither a Case, an aggregate, Lag nor Lead.
func (e Expr) Default(x any) Expr {
	var d Expr
	if e.kind == exprAggregate {
		d = toExpr(x)
	} else {
		d = fieldOrExpr(x)
	}
	e.def = &d
	return e
}

// errEmptyWhen refuses a When without a condition, as Django does.
var errEmptyWhen = errors.New("orm: an empty Q() can't be used as a When() condition")

// resolveCase binds a Case. A branch that no row can meet is left out, and
// one that every row meets ends the expression as its default, as Django's
// Case.as_sql skips the EmptyResultSet of a When and stops at its
// FullResultSet. A Case that keeps no branch is its default.
func (c *compiler) resolveCase(e Expr) (sqlNode, error) {
	n := caseNode{def: rawNode{sql: "NULL"}}
	if e.def != nil {
		def, err := c.resolveExpr(*e.def)
		if err != nil {
			return nil, err
		}
		n.def = def
	}
	full := false
	for _, w := range e.whens {
		if w.Cond.err == nil && len(w.Cond.children) == 0 {
			return nil, errEmptyWhen
		}
		where, match, err := c.condition(&w.Cond)
		if err != nil {
			return nil, err
		}
		then, err := c.resolveExpr(fieldOrExpr(w.Then))
		if err != nil {
			return nil, err
		}
		switch {
		case full || match == matchNone:
		case match == matchAll:
			n.def, full = then, true
		default:
			n.whens = append(n.whens, caseBranch{cond: condNode{where: where}, then: then})
		}
	}
	if len(n.whens) == 0 {
		return n.def, nil
	}
	return n, nil
}

// caseBranch is one resolved WHEN ... THEN ... of a caseNode.
type caseBranch struct {
	cond condNode
	then sqlNode
}

// caseNode is a searched CASE expression with at least one branch.
type caseNode struct {
	whens []caseBranch
	def   sqlNode
}

// render writes "CASE WHEN cond THEN value ... ELSE default END". A decimal
// CASE is cast to NUMERIC on SQLite, as Django's SQLiteNumericMixin does:
// SQLite compares the text of a decimal with a number only when one side has
// NUMERIC affinity, and an expression other than a column has none.
func (n caseNode) render(w *sqlWriter) {
	numeric := n.vtype() == typeDecimal && w.d.Name() == "sqlite"
	if numeric {
		w.write("CAST(")
		defer w.write(" AS NUMERIC)")
	}
	w.write("CASE")
	for _, b := range n.whens {
		w.write(" WHEN ")
		b.cond.render(w)
		w.write(" THEN ")
		b.then.render(w)
	}
	w.write(" ELSE ")
	n.def.render(w)
	w.write(" END")
}

// vtype is the common type of the values of the branches and the default,
// as Django resolves the output field of a Case from them.
func (n caseNode) vtype() valueType { return commonType(n.values()) }

// aggregate reports whether a condition or a value holds an aggregate.
func (n caseNode) aggregate() bool {
	for _, b := range n.whens {
		if b.cond.aggregate() || b.then.aggregate() {
			return true
		}
	}
	return n.def.aggregate()
}

// values returns the values of the branches and the default.
func (n caseNode) values() []sqlNode {
	out := make([]sqlNode, 0, len(n.whens)+1)
	for _, b := range n.whens {
		out = append(out, b.then)
	}
	return append(out, n.def)
}

// parts returns the expressions a caseNode is made of: the conditions, the
// values and the default.
func (n caseNode) parts() []sqlNode {
	out := make([]sqlNode, 0, 2*len(n.whens)+1)
	for _, b := range n.whens {
		out = append(out, b.cond, b.then)
	}
	return append(out, n.def)
}
