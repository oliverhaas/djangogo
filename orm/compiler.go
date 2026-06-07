package orm

import (
	"fmt"
	"strconv"
	"strings"
)

// compileWhere appends predicates, threading placeholders via next, and returns the
// " WHERE ..." fragment (empty if no clauses) and the collected args.
func (q *QuerySet[T]) compileWhere(next func() string) (string, []any, error) {
	if len(q.wheres) == 0 {
		return "", nil, nil
	}
	d := q.db.Dialect()
	preds := make([]string, 0, len(q.wheres))
	var args []any
	for _, w := range q.wheres {
		pred, pArgs, err := buildPredicate(d, q.model, w.key, w.value, next)
		if err != nil {
			return "", nil, err
		}
		if w.negate {
			pred = "NOT (" + pred + ")"
		}
		preds = append(preds, pred)
		args = append(args, pArgs...)
	}
	return " WHERE " + strings.Join(preds, " AND "), args, nil
}

// compileSelect renders the SELECT statement and its args for the queryset.
func (q *QuerySet[T]) compileSelect() (string, []any, error) {
	if q.err != nil {
		return "", nil, q.err
	}
	d := q.db.Dialect()

	var b strings.Builder
	b.WriteString("SELECT ")
	cols := q.model.Columns()
	quoted := make([]string, len(cols))
	for i, c := range cols {
		quoted[i] = d.Quote(c)
	}
	b.WriteString(strings.Join(quoted, ", "))
	b.WriteString(" FROM ")
	b.WriteString(d.Quote(q.model.Table()))

	n := 0
	next := func() string {
		n++
		return d.Placeholder(n)
	}
	where, args, err := q.compileWhere(next)
	if err != nil {
		return "", nil, err
	}
	b.WriteString(where)

	order, err := q.compileOrderBy()
	if err != nil {
		return "", nil, err
	}
	b.WriteString(order)

	b.WriteString(q.compileLimitOffset())

	return b.String(), args, nil
}

// compileOrderBy renders the " ORDER BY ..." fragment, validating each column
// against the model. It returns an empty string when there are no order terms.
func (q *QuerySet[T]) compileOrderBy() (string, error) {
	if len(q.orders) == 0 {
		return "", nil
	}
	d := q.db.Dialect()
	terms := make([]string, len(q.orders))
	for i, o := range q.orders {
		if _, ok := q.model.byColumn[o.column]; !ok {
			return "", fmt.Errorf("orm: unknown order field %q on model %s", o.column, q.model.Name())
		}
		term := d.Quote(o.column)
		if o.desc {
			term += " DESC"
		}
		terms[i] = term
	}
	return " ORDER BY " + strings.Join(terms, ", "), nil
}

// compileLimitOffset renders inline LIMIT/OFFSET clauses. SQLite requires a LIMIT
// before an OFFSET, so an offset-only query emits "LIMIT -1 OFFSET <n>".
func (q *QuerySet[T]) compileLimitOffset() string {
	var b strings.Builder
	switch {
	case q.offset > 0 && q.limit < 0:
		b.WriteString(" LIMIT -1 OFFSET ")
		b.WriteString(strconv.Itoa(q.offset))
	default:
		if q.limit >= 0 {
			b.WriteString(" LIMIT ")
			b.WriteString(strconv.Itoa(q.limit))
		}
		if q.offset > 0 {
			b.WriteString(" OFFSET ")
			b.WriteString(strconv.Itoa(q.offset))
		}
	}
	return b.String()
}

// compileCount renders the COUNT(*) statement and its args for the queryset.
func (q *QuerySet[T]) compileCount() (string, []any, error) {
	if q.err != nil {
		return "", nil, q.err
	}
	d := q.db.Dialect()

	n := 0
	next := func() string {
		n++
		return d.Placeholder(n)
	}
	where, args, err := q.compileWhere(next)
	if err != nil {
		return "", nil, err
	}
	return "SELECT COUNT(*) FROM " + d.Quote(q.model.Table()) + where, args, nil
}

// compileDelete renders the DELETE statement and its args for the queryset.
func (q *QuerySet[T]) compileDelete() (string, []any, error) {
	if q.err != nil {
		return "", nil, q.err
	}
	d := q.db.Dialect()

	n := 0
	next := func() string {
		n++
		return d.Placeholder(n)
	}
	where, args, err := q.compileWhere(next)
	if err != nil {
		return "", nil, err
	}
	return "DELETE FROM " + d.Quote(q.model.Table()) + where, args, nil
}
