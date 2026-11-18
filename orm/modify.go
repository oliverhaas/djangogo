package orm

import (
	"errors"
	"fmt"
)

// errDeleteSliced is returned by Delete on a queryset with a limit or offset.
var errDeleteSliced = errors.New("orm: cannot use 'limit' or 'offset' with delete()")

// errUpdateSliced is returned by Update on a queryset with a limit or offset.
var errUpdateSliced = errors.New("orm: cannot update a query once a slice has been taken")

// deleteSQL returns the DELETE statement for the rows q matches.
func (q *query) deleteSQL() (string, []any, error) {
	c, err := q.compile()
	if err != nil {
		return "", nil, err
	}
	if q.sliced() {
		return "", nil, errDeleteSliced
	}
	if c.empty {
		return "", nil, errEmptyResultSet
	}
	w := newSQLWriter(c.d)
	w.write("DELETE FROM ")
	w.ident(q.model.Table())
	if err := c.renderRestriction(w); err != nil {
		return "", nil, err
	}
	return w.String(), w.args, nil
}

// renderRestriction writes the WHERE clause of an UPDATE or DELETE. Those
// statements cannot join, so a query whose filters span relations selects the
// matching primary keys in a subquery instead.
func (c *compiler) renderRestriction(w *sqlWriter) error {
	if c.joins.used() == 0 && !c.q.hasAggregates() {
		if !c.where.empty() {
			w.write(" WHERE ")
			c.where.render(w)
		}
		return nil
	}
	pk := c.q.model.PrimaryKey()
	if pk == nil {
		return fmt.Errorf("orm: model %s has no primary key", c.q.model.Name())
	}
	items := []selectItem{{node: c.baseColumn(pk)}}
	sub := &selectStmt{c: c, items: items, limit: -1, groupBy: c.groupBy(items, nil)}
	w.write(" WHERE ")
	w.column(c.q.model.Table(), pk.Column)
	w.write(" IN (")
	sub.render(w)
	w.write(")")
	return nil
}

// errUpdateArgs is returned when the arguments of Update are not alternating
// field keys and values.
var errUpdateArgs = errors.New("orm: Update requires alternating string columns and values")

// updateSQL returns the UPDATE statement that applies the key/value pairs to
// the rows q matches. A value is a plain value or an Expr over the fields of
// the row being updated.
func (q *query) updateSQL(pairs []any) (string, []any, error) {
	c, err := q.compile()
	if err != nil {
		return "", nil, err
	}
	if len(pairs) == 0 || len(pairs)%2 != 0 {
		return "", nil, errUpdateArgs
	}
	if q.sliced() {
		return "", nil, errUpdateSliced
	}
	type assignment struct {
		field *Field
		value sqlNode
	}
	sets := make([]assignment, 0, len(pairs)/2)
	for i := 0; i < len(pairs); i += 2 {
		key, ok := pairs[i].(string)
		if !ok {
			return "", nil, errUpdateArgs
		}
		f, ok := lookupField(q.model, key)
		if !ok {
			return "", nil, &FieldError{
				Model:   q.model.Name(),
				Message: "cannot resolve keyword '" + key + "' into field. Choices are: " + fieldChoices(q.model, nil),
			}
		}
		value, err := c.assignedValue(f, pairs[i+1])
		if err != nil {
			return "", nil, err
		}
		sets = append(sets, assignment{field: f, value: value})
	}
	if c.empty {
		return "", nil, errEmptyResultSet
	}

	w := newSQLWriter(c.d)
	w.write("UPDATE ")
	w.ident(q.model.Table())
	w.write(" SET ")
	for i, a := range sets {
		if i > 0 {
			w.write(", ")
		}
		w.ident(a.field.Column)
		w.write(" = ")
		a.value.render(w)
	}
	if err := c.renderRestriction(w); err != nil {
		return "", nil, err
	}
	return w.String(), w.args, nil
}

// assignedValue resolves the value an UPDATE assigns to field f. An
// expression may only read the row that is updated: the statement has no
// joins, and an aggregate has no place in it.
func (c *compiler) assignedValue(f *Field, value any) (sqlNode, error) {
	var expr *Expr
	switch e := value.(type) {
	case Expr:
		expr = &e
	case *Expr:
		expr = e
	}
	if expr == nil {
		v, err := plainValue(c.q.db.Registry(), f, value)
		if err != nil {
			return nil, err
		}
		return paramNode{value: v}, nil
	}
	n, err := c.resolveExpr(*expr)
	if err != nil {
		return nil, err
	}
	if readsJoin(n) {
		return nil, &FieldError{Model: c.q.model.Name(), Message: "joined field references are not permitted in this query"}
	}
	if n.aggregate() {
		return nil, &FieldError{Model: c.q.model.Name(), Message: "aggregate functions are not allowed in this query"}
	}
	return n, nil
}
