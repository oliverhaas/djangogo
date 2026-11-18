package orm

import (
	"context"
	"errors"
	"fmt"
	"reflect"
)

// CreateTable creates the table for m using the dialect's DDL.
func (db *DB) CreateTable(ctx context.Context, m *Model) error {
	ddl := db.dialect.CreateTableSQL(m)
	if _, err := db.conn(ctx).ExecContext(ctx, ddl); err != nil {
		return fmt.Errorf("orm: create table %s: %w", m.Table(), err)
	}
	return nil
}

// Annotated is a model row together with the values of the queryset's
// annotations, as returned by AllAnnotated.
type Annotated[T any] struct {
	// Object is the model row.
	Object T
	// Annotations maps each annotation name to its value; see Aggregate for
	// the Go types.
	Annotations map[string]any
}

// fetch runs the SELECT of model rows and calls each with every row and its
// annotation values.
func (q *query) fetch(ctx context.Context, each func(row reflect.Value, plan *rowPlan, annots []any)) error {
	query, args, plan, err := q.selectSQL()
	if errors.Is(err, errEmptyResultSet) {
		return nil
	}
	if err != nil {
		return err
	}
	rows, err := q.db.conn(ctx).QueryContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("orm: query %s: %w", q.model.Name(), err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		row, annots, err := plan.scan(rows)
		if err != nil {
			return fmt.Errorf("orm: scan %s: %w", q.model.Name(), err)
		}
		each(row, plan, annots)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("orm: scan %s: %w", q.model.Name(), err)
	}
	return nil
}

// All runs the query and returns every matching row. The objects named by
// SelectRelated are loaded in the same query.
func (qs *QuerySet[T]) All(ctx context.Context) ([]T, error) {
	var out []T
	err := qs.q.fetch(ctx, func(row reflect.Value, _ *rowPlan, _ []any) {
		out = append(out, row.Interface().(T))
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// AllAnnotated runs the query and returns every matching row with the values
// of its annotations.
func (qs *QuerySet[T]) AllAnnotated(ctx context.Context) ([]Annotated[T], error) {
	var out []Annotated[T]
	err := qs.q.fetch(ctx, func(row reflect.Value, plan *rowPlan, annots []any) {
		values := make(map[string]any, len(annots))
		for i, name := range plan.annots {
			values[name] = annots[i]
		}
		out = append(out, Annotated[T]{Object: row.Interface().(T), Annotations: values})
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Get returns the single row matching the queryset, with any arguments
// applied as additional Filter conditions. Its error matches ErrDoesNotExist
// when no row matches and ErrMultipleObjectsReturned when more than one does,
// and carries Django's message for either.
//
// The ordering of the queryset is dropped, as Django's get() drops it: the
// order of at most one row does not matter. A queryset with a limit or an
// offset keeps it, because there it decides which rows are looked at.
func (qs *QuerySet[T]) Get(ctx context.Context, args ...any) (T, error) {
	var zero T
	matching := qs.Filter(args...)
	if !matching.q.sliced() {
		matching = matching.OrderBy()
	}
	// A few rows more than one are fetched to say how many matched. A
	// smaller limit of the caller stays: it hides no row from the count.
	limit := maxGetResults
	if own := matching.q.limit; own >= 0 && own <= maxGetResults {
		limit = own + 1
	} else {
		matching = matching.Limit(maxGetResults)
	}
	rows, err := matching.All(ctx)
	if err != nil {
		return zero, err
	}
	switch len(rows) {
	case 0:
		return zero, doesNotExist(qs.q.model)
	case 1:
		return rows[0], nil
	default:
		return zero, multipleObjects(qs.q.model, len(rows), limit)
	}
}

// First returns the first row of the queryset, ordered by primary key when
// the queryset has no ordering. It returns ErrDoesNotExist when the queryset
// is empty, where Django's first() returns None.
func (qs *QuerySet[T]) First(ctx context.Context) (T, error) {
	q := qs.q
	if len(q.orders) == 0 {
		q = q.orderBy([]string{"pk"})
	}
	return first[T](ctx, q)
}

// Last returns the last row of the queryset: the first row of the reversed
// ordering, with a queryset without an ordering ordered by primary key. It
// returns ErrDoesNotExist when the queryset is empty.
func (qs *QuerySet[T]) Last(ctx context.Context) (T, error) {
	return first[T](ctx, qs.q.reversed())
}

// first returns the first row of q.
func first[T any](ctx context.Context, q *query) (T, error) {
	var zero T
	cp := q.clone()
	cp.limit = 1
	rows, err := (&QuerySet[T]{q: cp}).All(ctx)
	if err != nil {
		return zero, err
	}
	if len(rows) == 0 {
		return zero, doesNotExist(q.model)
	}
	return rows[0], nil
}

// Count returns the number of rows matching the queryset, with one COUNT(*)
// query.
func (qs *QuerySet[T]) Count(ctx context.Context) (int64, error) {
	if qs.q.err != nil {
		return 0, qs.q.err
	}
	return qs.q.count(ctx)
}

// Exists reports whether any row matches the queryset, with a SELECT
// EXISTS(...) that stops at the first match.
func (qs *QuerySet[T]) Exists(ctx context.Context) (bool, error) {
	query, args, err := qs.q.existsSQL()
	if errors.Is(err, errEmptyResultSet) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	rows, err := qs.q.db.conn(ctx).QueryContext(ctx, query, args...)
	if err != nil {
		return false, fmt.Errorf("orm: exists %s: %w", qs.q.model.Name(), err)
	}
	defer func() { _ = rows.Close() }()
	var exists bool
	if rows.Next() {
		if err := rows.Scan(&exists); err != nil {
			return false, fmt.Errorf("orm: exists %s: %w", qs.q.model.Name(), err)
		}
	}
	if err := rows.Err(); err != nil {
		return false, fmt.Errorf("orm: exists %s: %w", qs.q.model.Name(), err)
	}
	return exists, nil
}

// Update sets the given fields on every row matching the queryset, in one
// UPDATE statement, and returns the number of rows matched. The arguments are
// alternating field keys and values:
//
//	Update(ctx, "age", 30)
//	Update(ctx, "views", orm.F("views").Add(1))
//	Update(ctx, "author", author)
//
// It is the counterpart of Django's QuerySet.update(): it writes exactly the
// fields it is given, so it does not stamp auto_now fields, and it fires no
// save signals. To save one object with those, use Save. A value may be an
// Expr over the fields of the row; an expression that spans a relation is an
// error. Update on a queryset with a Limit or Offset is an error.
func (qs *QuerySet[T]) Update(ctx context.Context, assignments ...any) (int64, error) {
	query, args, err := qs.q.updateSQL(assignments)
	if errors.Is(err, errEmptyResultSet) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	result, err := qs.q.db.conn(ctx).ExecContext(ctx, query, args...)
	if err != nil {
		return 0, fmt.Errorf("orm: update %s: %w", qs.q.model.Name(), err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("orm: update %s: rows affected: %w", qs.q.model.Name(), err)
	}
	return affected, nil
}

// Delete removes every row matching the queryset, in one DELETE statement,
// and returns the number of rows deleted. Delete on a queryset with a Limit
// or Offset is an error.
//
// When a PreDelete or PostDelete handler is registered for T, Delete first
// fetches the matching rows, so that it can fire PreDelete for each before
// the DELETE and PostDelete for each after it.
func (qs *QuerySet[T]) Delete(ctx context.Context) (int64, error) {
	if qs.q.err != nil {
		return 0, qs.q.err
	}
	if hasDeleteHandlers[T]() {
		return qs.deleteWithSignals(ctx)
	}
	return qs.deleteRows(ctx)
}

// deleteRows runs the DELETE and returns the number of rows affected.
func (qs *QuerySet[T]) deleteRows(ctx context.Context) (int64, error) {
	query, args, err := qs.q.deleteSQL()
	if errors.Is(err, errEmptyResultSet) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	result, err := qs.q.db.conn(ctx).ExecContext(ctx, query, args...)
	if err != nil {
		return 0, fmt.Errorf("orm: delete %s: %w", qs.q.model.Name(), err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("orm: delete %s: rows affected: %w", qs.q.model.Name(), err)
	}
	return affected, nil
}

// deleteWithSignals fetches the matching rows, fires PreDelete for each, runs
// the DELETE, then fires PostDelete for each.
func (qs *QuerySet[T]) deleteWithSignals(ctx context.Context) (int64, error) {
	if qs.q.sliced() {
		return 0, errDeleteSliced
	}
	rows, err := qs.All(ctx)
	if err != nil {
		return 0, err
	}
	for i := range rows {
		if err := firePreDelete(ctx, qs.q.db, &rows[i]); err != nil {
			return 0, err
		}
	}
	affected, err := qs.deleteRows(ctx)
	if err != nil {
		return 0, err
	}
	for i := range rows {
		if err := firePostDelete(ctx, qs.q.db, &rows[i]); err != nil {
			return 0, err
		}
	}
	return affected, nil
}
