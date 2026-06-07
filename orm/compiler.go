package orm

import (
	"fmt"
	"strconv"
	"strings"
)

// compileWhere appends predicates, threading placeholders via next, and returns the
// " WHERE ..." fragment (empty if no clauses) and the collected args. When qualify
// is non-empty, each column reference is prefixed with that table name so the
// predicate is unambiguous in a JOIN query.
func (q *QuerySet[T]) compileWhere(next func() string, qualify string) (string, []any, error) {
	if len(q.wheres) == 0 {
		return "", nil, nil
	}
	d := q.db.Dialect()
	preds := make([]string, 0, len(q.wheres))
	var args []any
	for _, w := range q.wheres {
		pred, pArgs, err := buildPredicate(d, q.model, w.key, w.value, next, qualify)
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
// When select_related fields are requested it delegates to compileSelectRelated,
// which renders a LEFT JOIN query instead.
func (q *QuerySet[T]) compileSelect() (string, []any, error) {
	if q.err != nil {
		return "", nil, q.err
	}
	if len(q.selectRelated) > 0 {
		return q.compileSelectRelated()
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
	where, args, err := q.compileWhere(next, "")
	if err != nil {
		return "", nil, err
	}
	b.WriteString(where)

	order, err := q.compileOrderBy("")
	if err != nil {
		return "", nil, err
	}
	b.WriteString(order)

	b.WriteString(q.compileLimitOffset())

	return b.String(), args, nil
}

// compileOrderBy renders the " ORDER BY ..." fragment, validating each column
// against the model. It returns an empty string when there are no order terms.
// When qualify is non-empty, each column reference is prefixed with that table
// name so the ordering is unambiguous in a JOIN query.
func (q *QuerySet[T]) compileOrderBy(qualify string) (string, error) {
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
		if qualify != "" {
			term = d.Quote(qualify) + "." + term
		}
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
	where, args, err := q.compileWhere(next, "")
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
	where, args, err := q.compileWhere(next, "")
	if err != nil {
		return "", nil, err
	}
	return "DELETE FROM " + d.Quote(q.model.Table()) + where, args, nil
}

// assignment is a single column/value pair for an UPDATE ... SET clause.
type assignment struct {
	column string
	value  any
}

// compileUpdate renders the UPDATE statement and its args for the queryset.
// Placeholders thread the SET values first, then the WHERE args, sharing one
// counter. Each assignment column must be a known column on the model.
func (q *QuerySet[T]) compileUpdate(assigns []assignment) (string, []any, error) {
	if q.err != nil {
		return "", nil, q.err
	}
	d := q.db.Dialect()

	n := 0
	next := func() string {
		n++
		return d.Placeholder(n)
	}

	sets := make([]string, len(assigns))
	args := make([]any, 0, len(assigns)+len(q.wheres))
	for i, a := range assigns {
		if _, ok := q.model.byColumn[a.column]; !ok {
			return "", nil, fmt.Errorf("orm: unknown update column %q on model %s", a.column, q.model.Name())
		}
		sets[i] = d.Quote(a.column) + " = " + next()
		args = append(args, a.value)
	}

	where, wArgs, err := q.compileWhere(next, "")
	if err != nil {
		return "", nil, err
	}
	args = append(args, wArgs...)

	return "UPDATE " + d.Quote(q.model.Table()) + " SET " + strings.Join(sets, ", ") + where, args, nil
}
