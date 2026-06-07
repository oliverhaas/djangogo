package orm

import (
	"context"
	"database/sql"
	"fmt"
	"reflect"
	"strings"
)

// CreateTable creates the table for m using the dialect's DDL.
func (db *DB) CreateTable(ctx context.Context, m *Model) error {
	ddl := db.dialect.CreateTableSQL(m)
	if _, err := db.conn(ctx).ExecContext(ctx, ddl); err != nil {
		return fmt.Errorf("orm: create table %s: %w", m.Table(), err)
	}
	return nil
}

// scanRows scans every row (whose columns are m.Columns() in field order) into a
// []T. Each scan target is the address of the corresponding struct field so the
// driver writes directly into a freshly allocated value of T.
func scanRows[T any](rows *sql.Rows, m *Model) ([]T, error) {
	fields := m.Fields()
	var out []T
	for rows.Next() {
		dest := reflect.New(m.GoType())
		elem := dest.Elem()
		targets := make([]any, len(fields))
		for i, f := range fields {
			targets[i] = elem.Field(f.Index).Addr().Interface()
		}
		if err := rows.Scan(targets...); err != nil {
			return nil, fmt.Errorf("orm: scan %s: %w", m.Name(), err)
		}
		out = append(out, elem.Interface().(T))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("orm: scan %s: %w", m.Name(), err)
	}
	return out, nil
}

// scanRowsJoined scans the result of a select_related LEFT JOIN. Each row holds
// the main model's columns followed, for each joined FK, by the target's
// columns. The main columns scan into a fresh T; each target's columns scan into
// NULL-tolerant holders. When the target primary key is non-null the holders are
// materialised into a target struct and attached to the FK field via SetObject;
// a null target (no match / null FK) leaves the FK unloaded but keeps the row.
func scanRowsJoined[T any](rows *sql.Rows, m *Model, rels []joinedRel) ([]T, error) {
	fields := m.Fields()
	var out []T
	for rows.Next() {
		dest := reflect.New(m.GoType())
		mainElem := dest.Elem()

		ncols := len(fields)
		for _, r := range rels {
			ncols += len(r.target.Fields())
		}
		targets := make([]any, 0, ncols)
		for _, f := range fields {
			targets = append(targets, mainElem.Field(f.Index).Addr().Interface())
		}
		// One *any holder per target column, NULL-tolerant for the LEFT JOIN.
		holders := make([][]any, len(rels))
		for ri, r := range rels {
			tf := r.target.Fields()
			holders[ri] = make([]any, len(tf))
			for ci := range tf {
				holders[ri][ci] = new(any)
				targets = append(targets, holders[ri][ci])
			}
		}

		if err := rows.Scan(targets...); err != nil {
			return nil, fmt.Errorf("orm: scan %s: %w", m.Name(), err)
		}

		for ri, r := range rels {
			if err := attachJoined(mainElem, r, holders[ri]); err != nil {
				return nil, fmt.Errorf("orm: scan %s: %w", m.Name(), err)
			}
		}
		out = append(out, mainElem.Interface().(T))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("orm: scan %s: %w", m.Name(), err)
	}
	return out, nil
}

// attachJoined materialises one joined FK's scanned holders into a target struct
// and attaches it to the FK field via SetObject. When the target primary key is
// null (no match or null FK) the FK is left unloaded.
func attachJoined(mainElem reflect.Value, r joinedRel, holders []any) error {
	tf := r.target.Fields()
	pk := r.target.PrimaryKey()
	pkColIdx := -1
	for i, f := range tf {
		if f == pk {
			pkColIdx = i
			break
		}
	}
	if pkColIdx < 0 {
		return fmt.Errorf("orm: target model %s has no primary key column", r.target.Name())
	}
	pkVal := *(holders[pkColIdx].(*any))
	if pkVal == nil {
		return nil // null FK / no match: leave the FK unloaded.
	}
	pkInt, err := asInt64(pkVal)
	if err != nil {
		return err
	}

	tgt := reflect.New(r.target.GoType())
	tgtElem := tgt.Elem()
	for i, f := range tf {
		v := *(holders[i].(*any))
		if err := assignScanned(tgtElem.Field(f.Index), v); err != nil {
			return err
		}
	}

	fkVal := mainElem.Field(r.field.Index).Addr()
	method := fkVal.MethodByName("SetObject")
	if !method.IsValid() {
		return fmt.Errorf("orm: field %s does not expose SetObject", r.field.Name)
	}
	method.Call([]reflect.Value{tgt, reflect.ValueOf(pkInt)})
	return nil
}

// All runs the compiled SELECT and returns every matching row scanned into []T.
// When select_related fields are requested it uses the joined scan path so the
// related objects are loaded in the same query.
func (q *QuerySet[T]) All(ctx context.Context) ([]T, error) {
	if q.err != nil {
		return nil, q.err
	}
	var rels []joinedRel
	if len(q.selectRelated) > 0 {
		var err error
		rels, err = q.resolveSelectRelated()
		if err != nil {
			return nil, err
		}
	}
	query, args, err := q.compileSelect()
	if err != nil {
		return nil, err
	}
	rows, err := q.db.conn(ctx).QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("orm: query %s: %w", q.model.Name(), err)
	}
	defer func() { _ = rows.Close() }()
	if len(rels) > 0 {
		return scanRowsJoined[T](rows, q.model, rels)
	}
	return scanRows[T](rows, q.model)
}

// Get returns the single row matching the queryset (with any pairs applied as
// additional filters). It returns ErrDoesNotExist when no row matches and
// ErrMultipleObjectsReturned when more than one does.
func (q *QuerySet[T]) Get(ctx context.Context, pairs ...any) (T, error) {
	var zero T
	if q.err != nil {
		return zero, q.err
	}
	// Limit to 2 rows: enough to detect "multiple" without fetching everything.
	rows, err := q.Filter(pairs...).Limit(2).All(ctx)
	if err != nil {
		return zero, err
	}
	switch len(rows) {
	case 0:
		return zero, ErrDoesNotExist
	case 1:
		return rows[0], nil
	default:
		return zero, ErrMultipleObjectsReturned
	}
}

// Count runs COUNT(*) for the queryset and returns the number of matching rows.
func (q *QuerySet[T]) Count(ctx context.Context) (int64, error) {
	if q.err != nil {
		return 0, q.err
	}
	query, args, err := q.compileCount()
	if err != nil {
		return 0, err
	}
	var n int64
	if err := q.db.conn(ctx).QueryRowContext(ctx, query, args...).Scan(&n); err != nil {
		return 0, fmt.Errorf("orm: count %s: %w", q.model.Name(), err)
	}
	return n, nil
}

// Exists reports whether any row matches the queryset, using a cheap
// SELECT EXISTS(...) rather than a full count.
func (q *QuerySet[T]) Exists(ctx context.Context) (bool, error) {
	if q.err != nil {
		return false, q.err
	}
	d := q.db.Dialect()
	n := 0
	next := func() string {
		n++
		return d.Placeholder(n)
	}
	where, args, err := q.compileWhere(next, "")
	if err != nil {
		return false, err
	}
	query := "SELECT EXISTS(SELECT 1 FROM " + d.Quote(q.model.Table()) + where + ")"
	var exists bool
	if err := q.db.conn(ctx).QueryRowContext(ctx, query, args...).Scan(&exists); err != nil {
		return false, fmt.Errorf("orm: exists %s: %w", q.model.Name(), err)
	}
	return exists, nil
}

// Create inserts obj as a new row. Any filters on the queryset are ignored. For
// a model with a KindAuto primary key the column is omitted from the INSERT and
// the database-assigned id is written back into obj.
func (q *QuerySet[T]) Create(ctx context.Context, obj *T) error {
	if q.err != nil {
		return q.err
	}
	d := q.db.Dialect()
	v := reflect.ValueOf(obj).Elem()
	pk := q.model.PrimaryKey()
	autoPK := pk != nil && pk.Kind == KindAuto

	fields := q.model.Fields()
	cols := make([]string, 0, len(fields))
	placeholders := make([]string, 0, len(fields))
	args := make([]any, 0, len(fields))
	n := 0
	for _, f := range fields {
		if autoPK && f == pk {
			continue
		}
		n++
		cols = append(cols, d.Quote(f.Column))
		placeholders = append(placeholders, d.Placeholder(n))
		args = append(args, v.Field(f.Index).Interface())
	}

	query := "INSERT INTO " + d.Quote(q.model.Table()) +
		" (" + strings.Join(cols, ", ") + ") VALUES (" + strings.Join(placeholders, ", ") + ")"

	// Auto PK with a RETURNING-capable backend: read the assigned id straight back.
	if autoPK && d.SupportsReturning() {
		query += " RETURNING " + d.Quote(pk.Column)
		var id int64
		if err := q.db.conn(ctx).QueryRowContext(ctx, query, args...).Scan(&id); err != nil {
			return fmt.Errorf("orm: insert %s: %w", q.model.Name(), err)
		}
		return writeBackAutoPK(v.Field(pk.Index), id, q.model.Name())
	}

	result, err := q.db.conn(ctx).ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("orm: insert %s: %w", q.model.Name(), err)
	}

	// Auto PK without RETURNING (e.g. SQLite): fall back to LastInsertId.
	if autoPK {
		id, err := result.LastInsertId()
		if err != nil {
			return fmt.Errorf("orm: insert %s: last insert id: %w", q.model.Name(), err)
		}
		return writeBackAutoPK(v.Field(pk.Index), id, q.model.Name())
	}
	return nil
}

// writeBackAutoPK assigns the database-generated id into the auto primary-key
// struct field, handling both signed and unsigned integer kinds.
func writeBackAutoPK(pkField reflect.Value, id int64, modelName string) error {
	switch pkField.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		pkField.SetInt(id)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		pkField.SetUint(uint64(id))
	default:
		return fmt.Errorf("orm: insert %s: cannot assign auto pk to %s field", modelName, pkField.Kind())
	}
	return nil
}

// Update applies the given column/value assignments to every matching row and
// returns the number of rows affected. assignments must be even-arity pairs of a
// string column name followed by its value, e.g. Update(ctx, "age", 30).
func (q *QuerySet[T]) Update(ctx context.Context, assignments ...any) (int64, error) {
	if q.err != nil {
		return 0, q.err
	}
	if len(assignments)%2 != 0 {
		return 0, fmt.Errorf("orm: Update requires alternating string columns and values")
	}
	assigns := make([]assignment, 0, len(assignments)/2)
	for i := 0; i < len(assignments); i += 2 {
		col, ok := assignments[i].(string)
		if !ok {
			return 0, fmt.Errorf("orm: Update requires alternating string columns and values")
		}
		assigns = append(assigns, assignment{column: col, value: assignments[i+1]})
	}

	query, args, err := q.compileUpdate(assigns)
	if err != nil {
		return 0, err
	}
	result, err := q.db.conn(ctx).ExecContext(ctx, query, args...)
	if err != nil {
		return 0, fmt.Errorf("orm: update %s: %w", q.model.Name(), err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("orm: update %s: rows affected: %w", q.model.Name(), err)
	}
	return affected, nil
}

// Delete removes every matching row and returns the number of rows deleted.
func (q *QuerySet[T]) Delete(ctx context.Context) (int64, error) {
	if q.err != nil {
		return 0, q.err
	}
	query, args, err := q.compileDelete()
	if err != nil {
		return 0, err
	}
	result, err := q.db.conn(ctx).ExecContext(ctx, query, args...)
	if err != nil {
		return 0, fmt.Errorf("orm: delete %s: %w", q.model.Name(), err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("orm: delete %s: rows affected: %w", q.model.Name(), err)
	}
	return affected, nil
}
