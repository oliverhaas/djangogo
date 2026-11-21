package orm

import (
	"context"
	"fmt"
	"reflect"
	"strings"
)

// LabeledRows returns the (primary key, label) pairs for every row of model m,
// ordered by primary key. Each label is orm.Label of the scanned row, so a model
// with a String() method drives the displayed text. It is the data source for a
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

	var out [][2]string
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
		pkVal := elem.Field(pk.Index).Interface()
		out = append(out, [2]string{fmt.Sprint(pkVal), Label(m, dest.Interface())})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("orm: labeled rows %s: %w", m.Name(), err)
	}
	return out, nil
}

// LabeledRowsOrdered returns (primary key, label) pairs for rows of m ordered
// by ordering (or m.Ordering() when ordering is empty, falling back to primary
// key). When pks is non-nil, only rows whose primary key is in pks are
// returned.
func LabeledRowsOrdered(ctx context.Context, db *DB, m *Model, ordering []string, pks []any) ([][2]string, error) {
	pk := m.PrimaryKey()
	if pk == nil {
		return nil, fmt.Errorf("orm: model %s has no primary key", m.Name())
	}
	if pks != nil && len(pks) == 0 {
		return nil, nil
	}
	q := newQuery(db, m)
	if pks != nil {
		q = q.filter([]any{"pk__in", pks}, false)
	}
	switch {
	case len(ordering) > 0:
		q = q.orderBy(ordering)
	case len(m.Ordering()) == 0:
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
	var out [][2]string
	for rows.Next() {
		elem, _, err := plan.scan(rows)
		if err != nil {
			return nil, fmt.Errorf("orm: labeled rows %s: %w", m.Name(), err)
		}
		pkVal := elem.Field(pk.Index).Interface()
		out = append(out, [2]string{fmt.Sprint(pkVal), Label(m, elem.Addr().Interface())})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("orm: labeled rows %s: %w", m.Name(), err)
	}
	return out, nil
}
