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
