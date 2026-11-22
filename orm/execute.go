package orm

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
)

// CreateTable creates the table for m using the dialect's DDL, along with its
// field indexes, Meta.Indexes, Meta.UniqueTogether, Meta.Constraints, and any
// auto-created M2M join tables declared on m. The tables that m references
// must exist on PostgreSQL; CreateTables creates several tables in an order
// that works.
func (db *DB) CreateTable(ctx context.Context, m *Model) error {
	return db.createTable(ctx, m, nil, nil)
}

// CreateTables creates the tables of models as CreateTable does, a table
// after the tables it references. On a ForeignKeyDialect, a table that refers
// to a table created later, as in a cycle of foreign keys, is created without
// its foreign keys, which are added once every table exists, as Django's
// schema editor does with its deferred SQL. Tables that exist already are an
// error.
func (db *DB) CreateTables(ctx context.Context, models ...*Model) error {
	created := make(map[*Model]bool, len(models))
	var later []deferredFK
	for _, m := range creationOrder(models) {
		if err := db.createTable(ctx, m, created, &later); err != nil {
			return err
		}
	}
	// later holds fields only on a ForeignKeyDialect.
	fk, _ := db.dialect.(ForeignKeyDialect)
	for _, l := range later {
		if _, err := db.conn(ctx).ExecContext(ctx, fk.AddForeignKeySQL(l.model, l.field)); err != nil {
			return fmt.Errorf("orm: add foreign key %s.%s: %w", l.model.Table(), l.field.Column, err)
		}
	}
	return nil
}

// deferredFK is a foreign key field together with its model.
type deferredFK struct {
	model *Model
	field *Field
}

// createTable creates the table of m. With created, which holds the models
// whose tables CreateTables has created, a ForeignKeyDialect creates the
// table without its foreign keys when one refers to a table that does not
// exist yet, and the foreign keys go to later.
func (db *DB) createTable(ctx context.Context, m *Model, created map[*Model]bool, later *[]deferredFK) error {
	defs, stmts, err := db.constraintsDDL(m)
	if err != nil {
		return err
	}
	ddl := db.dialect.CreateTableSQL(m)
	if fk, ok := db.dialect.(ForeignKeyDialect); ok && created != nil && refersAhead(m, created) {
		ddl = fk.CreateTableNoForeignKeysSQL(m)
		for _, f := range m.Fields() {
			if f.Rel != nil && f.Rel.Target != nil {
				*later = append(*later, deferredFK{model: m, field: f})
			}
		}
	}
	if len(defs) > 0 {
		// Add the table constraints to the column definitions.
		ddl = strings.TrimSuffix(ddl, ")") + ", " + strings.Join(defs, ", ") + ")"
	}
	if _, err := db.conn(ctx).ExecContext(ctx, ddl); err != nil {
		return fmt.Errorf("orm: create table %s: %w", m.Table(), err)
	}
	if err := db.createTableExtras(ctx, m); err != nil {
		return err
	}
	for _, stmt := range stmts {
		if _, err := db.conn(ctx).ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("orm: create table %s: %w", m.Table(), err)
		}
	}
	if created != nil {
		created[m] = true
	}
	for _, f := range m.m2mFields {
		if f.Rel != nil && f.Rel.AutoCreated && f.Rel.Through != nil {
			if err := db.createTable(ctx, f.Rel.Through, created, later); err != nil {
				return err
			}
		}
	}
	return nil
}

// refersAhead reports whether a foreign key of m refers to another model
// whose table is not in created.
func refersAhead(m *Model, created map[*Model]bool) bool {
	for _, f := range m.Fields() {
		if f.Rel != nil && f.Rel.Target != nil && f.Rel.Target != m && !created[f.Rel.Target] {
			return true
		}
	}
	return false
}

// creationOrder sorts models so that a model follows every model its foreign
// keys and auto-created many-to-many tables reference, keeping the given
// order otherwise. The members of a reference cycle, and a model that refers
// to itself, keep the given order among themselves.
func creationOrder(models []*Model) []*Model {
	out := make([]*Model, 0, len(models))
	state := make(map[*Model]int, len(models)) // 0 new, 1 visiting, 2 done
	var visit func(m *Model)
	visit = func(m *Model) {
		if state[m] != 0 {
			return
		}
		state[m] = 1
		for _, f := range m.Fields() {
			if f.Rel != nil && f.Rel.Target != nil {
				visit(f.Rel.Target)
			}
		}
		for _, f := range m.ManyToMany() {
			if f.Rel != nil && f.Rel.AutoCreated && f.Rel.Target != nil {
				visit(f.Rel.Target)
			}
		}
		state[m] = 2
		out = append(out, m)
	}
	for _, m := range models {
		visit(m)
	}
	return out
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
	return q.fetchWithParent(ctx, func(row reflect.Value, _ any, plan *rowPlan, annots []any) {
		each(row, plan, annots)
	})
}

// fetchWithParent runs the SELECT of model rows and calls each with every row,
// its optional M2M prefetch parent key, the row plan, and annotation values.
func (q *query) fetchWithParent(ctx context.Context, each func(row reflect.Value, parentKey any, plan *rowPlan, annots []any)) error {
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
		row, parentKey, annots, err := plan.scanWithParent(rows)
		if err != nil {
			return fmt.Errorf("orm: scan %s: %w", q.model.Name(), err)
		}
		each(row, parentKey, plan, annots)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("orm: scan %s: %w", q.model.Name(), err)
	}
	return nil
}

// All runs the query and returns every matching row. The objects named by
// SelectRelated are loaded in the same query, and those named by
// PrefetchRelated are loaded with one batched IN query per level.
func (qs *QuerySet[T]) All(ctx context.Context) ([]T, error) {
	if qs.q.prefetchedList != nil {
		if cached, ok := qs.q.prefetchedList.([]T); ok {
			out := make([]T, len(cached))
			copy(out, cached)
			return out, nil
		}
	}
	var out []T
	err := qs.q.fetch(ctx, func(row reflect.Value, _ *rowPlan, _ []any) {
		out = append(out, row.Interface().(T))
	})
	if err != nil {
		return nil, err
	}
	if len(out) > 0 && len(qs.q.prefetchRelated) > 0 {
		elems := make([]reflect.Value, len(out))
		for i := range out {
			elems[i] = reflect.ValueOf(&out[i]).Elem()
		}
		if err := prefetchRelatedObjects(ctx, qs.q.db, qs.q.model, elems, qs.q.prefetchRelated); err != nil {
			return nil, err
		}
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
	if len(out) > 0 && len(qs.q.prefetchRelated) > 0 {
		elems := make([]reflect.Value, len(out))
		for i := range out {
			elems[i] = reflect.ValueOf(&out[i].Object).Elem()
		}
		if err := prefetchRelatedObjects(ctx, qs.q.db, qs.q.model, elems, qs.q.prefetchRelated); err != nil {
			return nil, err
		}
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
	if qs.q.prefetchedList != nil {
		if cached, ok := qs.q.prefetchedList.([]T); ok {
			var zero T
			if len(cached) == 0 {
				return zero, doesNotExist(qs.q.model)
			}
			return cached[0], nil
		}
	}
	q := qs.q
	if len(q.effectiveOrders()) == 0 {
		q = q.orderBy([]string{"pk"})
	}
	return first[T](ctx, q)
}

// Last returns the last row of the queryset: the first row of the reversed
// ordering, with a queryset without an ordering ordered by primary key. It
// returns ErrDoesNotExist when the queryset is empty.
func (qs *QuerySet[T]) Last(ctx context.Context) (T, error) {
	if qs.q.prefetchedList != nil {
		if cached, ok := qs.q.prefetchedList.([]T); ok {
			var zero T
			if len(cached) == 0 {
				return zero, doesNotExist(qs.q.model)
			}
			return cached[len(cached)-1], nil
		}
	}
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
	if qs.q.prefetchedList != nil {
		if cached, ok := qs.q.prefetchedList.([]T); ok {
			return int64(len(cached)), nil
		}
	}
	return qs.q.count(ctx)
}

// Exists reports whether any row matches the queryset, with a SELECT
// EXISTS(...) that stops at the first match.
func (qs *QuerySet[T]) Exists(ctx context.Context) (bool, error) {
	if qs.q.prefetchedList != nil {
		if cached, ok := qs.q.prefetchedList.([]T); ok {
			return len(cached) > 0, nil
		}
	}
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
		err = fmt.Errorf("orm: update %s: %w", qs.q.model.Name(), err)
		markForRollbackOnError(ctx, err)
		return 0, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("orm: update %s: rows affected: %w", qs.q.model.Name(), err)
	}
	return affected, nil
}

// Delete removes every row matching the queryset and returns the total number
// of rows deleted (including cascaded dependents). Delete on a queryset with a
// Limit or Offset is an error.
//
// When incoming foreign keys or PreDelete/PostDelete handlers exist, Delete
// uses a collector that enforces on_delete=protect (ErrProtected),
// on_delete=restrict (ErrRestricted), on_delete=set_null, and
// on_delete=cascade in dependency order, firing PreDelete and PostDelete
// handlers on the affected models. Otherwise it runs a single DELETE statement.
func (qs *QuerySet[T]) Delete(ctx context.Context) (int64, error) {
	return qs.deleteWithCollector(ctx)
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
		err = fmt.Errorf("orm: delete %s: %w", qs.q.model.Name(), err)
		markForRollbackOnError(ctx, err)
		return 0, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("orm: delete %s: rows affected: %w", qs.q.model.Name(), err)
	}
	return affected, nil
}
