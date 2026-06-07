package orm

import (
	"fmt"
	"strings"
)

// whereClause is a single filter or exclude condition awaiting compilation.
type whereClause struct {
	key    string
	value  any
	negate bool // true for Exclude
}

// orderClause is a single ORDER BY term.
type orderClause struct {
	column string
	desc   bool
}

// QuerySet is a lazy, immutable, clone-per-method query builder for model T.
// Chain methods never mutate their receiver; each returns a fresh clone.
type QuerySet[T any] struct {
	db            *DB
	model         *Model
	wheres        []whereClause
	orders        []orderClause
	selectRelated []string // FK field NAMES to eager-load via LEFT JOIN
	limit         int      // -1 means no limit
	offset        int      // 0 means no offset
	err           error
}

// Query starts a new QuerySet for model T resolved from db's registry.
func Query[T any](db *DB) *QuerySet[T] {
	var zero T
	q := &QuerySet[T]{db: db, limit: -1}
	m, ok := db.Registry().ModelOf(zero)
	if !ok {
		q.err = fmt.Errorf("orm: no model registered for %T", zero)
		return q
	}
	q.model = m
	return q
}

// clone returns a deep copy of q with fresh backing slices so that mutating the
// clone can never affect the receiver.
func (q *QuerySet[T]) clone() *QuerySet[T] {
	cp := &QuerySet[T]{
		db:     q.db,
		model:  q.model,
		limit:  q.limit,
		offset: q.offset,
		err:    q.err,
	}
	if len(q.wheres) > 0 {
		cp.wheres = make([]whereClause, len(q.wheres))
		copy(cp.wheres, q.wheres)
	}
	if len(q.orders) > 0 {
		cp.orders = make([]orderClause, len(q.orders))
		copy(cp.orders, q.orders)
	}
	if len(q.selectRelated) > 0 {
		cp.selectRelated = make([]string, len(q.selectRelated))
		copy(cp.selectRelated, q.selectRelated)
	}
	return cp
}

// SelectRelated returns a clone that eager-loads the named forward-FK fields via
// a single LEFT JOIN, populating each FK's loaded object during the read. Each
// name must identify a forward-FK field on the model; an unknown or non-FK name
// is reported as an error when the queryset is compiled or executed.
func (q *QuerySet[T]) SelectRelated(fields ...string) *QuerySet[T] {
	cp := q.clone()
	cp.selectRelated = append(cp.selectRelated, fields...)
	return cp
}

// Filter returns a clone with additional equality/lookup conditions.
// pairs must be alternating string keys and values.
func (q *QuerySet[T]) Filter(pairs ...any) *QuerySet[T] {
	return q.addWheres(pairs, false)
}

// Exclude returns a clone with additional negated conditions.
// pairs must be alternating string keys and values.
func (q *QuerySet[T]) Exclude(pairs ...any) *QuerySet[T] {
	return q.addWheres(pairs, true)
}

// addWheres appends one whereClause per key/value pair to a clone, validating
// arity and key types. The first build error is preserved.
func (q *QuerySet[T]) addWheres(pairs []any, negate bool) *QuerySet[T] {
	cp := q.clone()
	if len(pairs)%2 != 0 {
		cp.setErr(fmt.Errorf("orm: Filter requires alternating string keys and values"))
		return cp
	}
	for i := 0; i < len(pairs); i += 2 {
		key, ok := pairs[i].(string)
		if !ok {
			cp.setErr(fmt.Errorf("orm: Filter requires alternating string keys and values"))
			return cp
		}
		cp.wheres = append(cp.wheres, whereClause{key: key, value: pairs[i+1], negate: negate})
	}
	return cp
}

// setErr records err only if no error has been recorded yet (first error wins).
func (q *QuerySet[T]) setErr(err error) {
	if q.err == nil {
		q.err = err
	}
}

// OrderBy returns a clone ordered by the given fields. A leading "-" requests
// descending order. Column validity is checked at compile time.
func (q *QuerySet[T]) OrderBy(fields ...string) *QuerySet[T] {
	cp := q.clone()
	for _, field := range fields {
		desc := false
		col := field
		if strings.HasPrefix(col, "-") {
			desc = true
			col = col[1:]
		}
		cp.orders = append(cp.orders, orderClause{column: col, desc: desc})
	}
	return cp
}

// Limit returns a clone capped to at most n rows.
func (q *QuerySet[T]) Limit(n int) *QuerySet[T] {
	cp := q.clone()
	cp.limit = n
	return cp
}

// Offset returns a clone that skips the first n rows.
func (q *QuerySet[T]) Offset(n int) *QuerySet[T] {
	cp := q.clone()
	cp.offset = n
	return cp
}
