package orm

import (
	"context"
	"fmt"
	"reflect"
	"strings"
)

// LabeledRows returns the (primary key, label) pairs for every row of model m,
// ordered by primary key. Each label is orm.LabelContext of the scanned row, so
// a model with a String() or a ContextLabeler method drives the displayed text. It is the data source for a
// foreign-key <select> in forms and the admin changelist's FK column.
func LabeledRows(ctx context.Context, db *DB, m *Model) ([][2]string, error) {
	pk := m.PrimaryKey()
	if pk == nil {
		return nil, fmt.Errorf("orm: model %s has no primary key", m.Name())
	}
	d := db.Dialect()
	fields := m.Fields()
	cols := make([]string, len(fields))
	for i, f := range fields {
		cols[i] = d.Quote(f.Column)
	}
	query := "SELECT " + strings.Join(cols, ", ") + " FROM " + d.Quote(m.Table()) +
		" ORDER BY " + d.Quote(pk.Column)

	rows, err := db.conn(ctx).QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("orm: labeled rows %s: %w", m.Name(), err)
	}
	defer func() { _ = rows.Close() }()

	var objs []reflect.Value
	for rows.Next() {
		dest := reflect.New(m.GoType())
		elem := dest.Elem()
		targets := make([]any, len(fields))
		for i, f := range fields {
			targets[i] = elem.Field(f.Index).Addr().Interface()
		}
		if err := rows.Scan(targets...); err != nil {
			return nil, fmt.Errorf("orm: labeled rows %s: %w", m.Name(), err)
		}
		objs = append(objs, dest)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("orm: labeled rows %s: %w", m.Name(), err)
	}
	_ = rows.Close()
	return labelRows(ctx, db, m, objs)
}

// labelRows returns the (primary key, label) pairs of objs, pointers to
// instances of m. Call it after the rows are closed: a ContextLabeler may
// query the database on the same connection.
func labelRows(ctx context.Context, db *DB, m *Model, objs []reflect.Value) ([][2]string, error) {
	pk := m.PrimaryKey()
	out := make([][2]string, 0, len(objs))
	for _, obj := range objs {
		label, err := LabelContext(ctx, db, m, obj.Interface())
		if err != nil {
			return nil, fmt.Errorf("orm: labeled rows %s: %w", m.Name(), err)
		}
		out = append(out, [2]string{fmt.Sprint(obj.Elem().Field(pk.Index).Interface()), label})
	}
	return out, nil
}

// LabeledRowsOrdered returns (primary key, label) pairs for rows of m ordered
// by ordering, which holds terms as QuerySet.OrderBy takes them (or
// m.Ordering() when ordering is empty, falling back to primary key). When pks is non-nil, only rows whose primary key is in pks are
// returned.
func LabeledRowsOrdered(ctx context.Context, db *DB, m *Model, ordering []any, pks []any) ([][2]string, error) {
	return LabeledRowsWhere(ctx, db, m, Cond{}, ordering, pks)
}

// LabeledRowsWhere is LabeledRowsOrdered for the rows of m that match where,
// such as the LimitChoicesTo of a relation to m. A row that matches through
// several related rows is returned once.
func LabeledRowsWhere(ctx context.Context, db *DB, m *Model, where Cond, ordering []any, pks []any) ([][2]string, error) {
	q := newQuery(db, m)
	if len(ordering) > 0 {
		q = q.orderByArgs(ordering)
	}
	return labeledRows(ctx, q, where, pks)
}

// LabeledQuerySetRows is LabeledRowsWhere for the rows of qs, a queryset of
// the rows of m, such as the queryset of a ModelChoiceField. The rows come in
// the ordering of qs, else in m.Ordering(), else by primary key. It fails
// when qs is a queryset of another model or of Values.
func LabeledQuerySetRows(ctx context.Context, m *Model, qs AnyQuerySet, where Cond, pks []any) ([][2]string, error) {
	var q *query
	if qs != nil {
		q = qs.subquery()
	}
	if q == nil || q.model != m || q.values != nil {
		return nil, fmt.Errorf("orm: labeled rows %s: the queryset does not select rows of %s", m.Name(), m.Name())
	}
	return labeledRows(ctx, q, where, pks)
}

// labeledRows returns the (primary key, label) pairs of the rows of q that
// match where, for LabeledRowsWhere and LabeledQuerySetRows. When pks is not
// nil, only the rows whose primary key is in pks are returned.
func labeledRows(ctx context.Context, q *query, where Cond, pks []any) ([][2]string, error) {
	db, m := q.db, q.model
	if m.PrimaryKey() == nil {
		return nil, fmt.Errorf("orm: model %s has no primary key", m.Name())
	}
	if pks != nil && len(pks) == 0 {
		return nil, nil
	}
	if pks != nil {
		q = q.filter([]any{"pk__in", pks}, false)
	}
	if !where.empty() || where.err != nil {
		q = q.filter([]any{"pk__in", newQuery(db, m).filter([]any{where}, false)}, false)
	}
	if !q.explicitOrder && len(m.Ordering()) == 0 {
		q = q.orderBy([]string{"pk"})
	}
	c, err := q.compile()
	if err != nil {
		return nil, fmt.Errorf("orm: labeled rows %s: %w", m.Name(), err)
	}
	if c.empty {
		return nil, nil
	}
	items := make([]selectItem, len(m.fields))
	for i, f := range m.fields {
		items[i] = selectItem{node: c.baseColumn(f)}
	}
	stmt, err := c.build(items, true)
	if err != nil {
		return nil, fmt.Errorf("orm: labeled rows %s: %w", m.Name(), err)
	}
	w := newSQLWriter(c.d)
	stmt.render(w)
	rows, err := db.conn(ctx).QueryContext(ctx, w.String(), w.args...)
	if err != nil {
		return nil, fmt.Errorf("orm: labeled rows %s: %w", m.Name(), err)
	}
	defer func() { _ = rows.Close() }()

	plan := &rowPlan{model: m, ncols: len(m.fields)}
	var objs []reflect.Value
	for rows.Next() {
		elem, _, err := plan.scan(rows)
		if err != nil {
			return nil, fmt.Errorf("orm: labeled rows %s: %w", m.Name(), err)
		}
		objs = append(objs, elem.Addr())
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("orm: labeled rows %s: %w", m.Name(), err)
	}
	_ = rows.Close()
	return labelRows(ctx, db, m, objs)
}

// DistinctValues returns the distinct values of the field key over every row
// of model m, ordered by key. It is
// Query[T](db).ValuesList(key).Distinct().OrderBy(key).Flat(ctx) for code
// that knows the model only at run time, such as the admin's
// AllValuesFieldListFilter on a field of a related model.
func DistinctValues(ctx context.Context, db *DB, m *Model, key string) ([]any, error) {
	vl := &ValuesListQuerySet{q: newQuery(db, m).withValues([]string{key})}
	return vl.Distinct().OrderBy(key).Flat(ctx)
}

// GetByPK returns the row of model m whose primary key is pk, as a pointer to
// a new instance of m, or ErrDoesNotExist. It is Query[T](db).Get(ctx, "pk",
// pk) for code that knows the model only at run time, such as Django's
// ContentType.get_object_for_this_type.
func GetByPK(ctx context.Context, db *DB, m *Model, pk any) (any, error) {
	pkf := m.PrimaryKey()
	if pkf == nil {
		return nil, fmt.Errorf("orm: model %s has no primary key", m.Name())
	}
	w := newSQLWriter(db.dialect)
	w.write("SELECT ")
	for i, f := range m.fields {
		if i > 0 {
			w.write(", ")
		}
		w.ident(f.Column)
	}
	w.write(" FROM ")
	w.ident(m.Table())
	w.write(" WHERE ")
	w.ident(pkf.Column)
	w.write(" = ")
	w.param(pk)
	rows, err := db.conn(ctx).QueryContext(ctx, w.String(), w.args...)
	if err != nil {
		return nil, fmt.Errorf("orm: get %s: %w", m.Name(), err)
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("orm: get %s: %w", m.Name(), err)
		}
		return nil, fmt.Errorf("%s: %w", m.Name(), ErrDoesNotExist)
	}
	plan := &rowPlan{model: m, ncols: len(m.fields)}
	elem, _, err := plan.scan(rows)
	if err != nil {
		return nil, fmt.Errorf("orm: get %s: %w", m.Name(), err)
	}
	return elem.Addr().Interface(), nil
}
