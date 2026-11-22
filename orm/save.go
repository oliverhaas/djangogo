package orm

import (
	"context"
	"fmt"
	"reflect"
	"slices"
	"time"
)

// createdKey is the context key under which a save reports whether it
// inserted a new row.
type createdKey struct{}

// Created reports, inside a PostSave handler, whether the save inserted a new
// row, like the created argument of Django's post_save signal. It is false
// when the row was updated, and outside a PostSave handler.
func Created(ctx context.Context) bool {
	created, _ := ctx.Value(createdKey{}).(bool)
	return created
}

// Create inserts obj as a new row. Any filters on the queryset are ignored.
// Fields with a default= that hold their zero value get the default, and
// auto_now and auto_now_add fields are stamped. An auto primary key that is
// zero is assigned by the database and written back into obj; a non-zero one
// is inserted as given. PreSave and PostSave handlers are fired.
func (qs *QuerySet[T]) Create(ctx context.Context, obj *T) error {
	if qs.q.err != nil {
		return qs.q.err
	}
	// PreSave runs first, so that a handler can still change the object.
	if err := firePreSave(ctx, qs.q.db, obj); err != nil {
		return err
	}
	return qs.insertOne(ctx, obj)
}

// insertOne inserts obj and fires PostSave with Created set.
func (qs *QuerySet[T]) insertOne(ctx context.Context, obj *T) error {
	db, m := qs.q.db, qs.q.model
	v := reflect.ValueOf(obj).Elem()
	if err := syncLoadedRelations(db.Registry(), m, v, "save"); err != nil {
		return err
	}
	if err := checkInsertDeferred(m, v); err != nil {
		return err
	}
	applyDefaults(v, m.fields)
	applyAutoNowOnCreate(v, m.fields, db.now())
	if err := db.insert(ctx, m, []reflect.Value{v}, false); err != nil {
		markForRollbackOnError(ctx, err)
		return err
	}
	// The row holds every field of obj now.
	setDeferred(m, v, nil)
	return firePostSave(context.WithValue(ctx, createdKey{}, true), db, obj)
}

// saveConfig collects the options of Save.
type saveConfig struct {
	updateFields []string
}

// SaveOption is an optional argument of Save.
type SaveOption func(*saveConfig)

// UpdateFields restricts Save to an UPDATE of the named fields, like Django's
// save(update_fields=[...]). An auto_now field is only stamped when it is
// named. The object must already exist: saving nothing is an error that wraps
// ErrDoesNotExist.
func UpdateFields(keys ...string) SaveOption {
	return func(c *saveConfig) { c.updateFields = append(c.updateFields, keys...) }
}

// Save writes obj to the database, like Django's Model.save(). An object
// whose primary key is zero is inserted, exactly as by Create. Otherwise its
// row is updated: auto_now fields are stamped and every field but the primary
// key is written. When no row has that primary key, the object is inserted
// with it. PreSave and PostSave handlers are fired either way; Created tells a
// PostSave handler which of the two happened.
//
// An object loaded with Defer or Only is saved as with UpdateFields of its
// loaded fields, as Django does. A deferred field is written as well when it
// no longer holds its zero value, which marks it as assigned; a deferred field
// assigned its zero value keeps the stored value.
//
// Save is the per-object path. Update on a queryset writes only the fields it
// is given, to every matching row, without stamps or signals.
func (qs *QuerySet[T]) Save(ctx context.Context, obj *T, opts ...SaveOption) error {
	if qs.q.err != nil {
		return qs.q.err
	}
	cfg := &saveConfig{}
	for _, opt := range opts {
		opt(cfg)
	}
	db, m := qs.q.db, qs.q.model
	pk := m.PrimaryKey()
	v := reflect.ValueOf(obj).Elem()
	if err := syncLoadedRelations(db.Registry(), m, v, "save"); err != nil {
		return err
	}
	if pk == nil || pkFieldIsZero(pk, v.Field(pk.Index)) {
		if len(cfg.updateFields) > 0 {
			return fmt.Errorf("orm: save %s: cannot force an update in save() with no primary key", m.Name())
		}
		return qs.Create(ctx, obj)
	}
	fields, err := saveFields(m, cfg.updateFields)
	if err != nil {
		return err
	}
	forced := len(cfg.updateFields) > 0
	if deferred := deferredOf(m, v); !forced && len(deferred) > 0 {
		fields = slices.DeleteFunc(fields, func(f *Field) bool {
			return slices.Contains(deferred, attname(f)) && v.Field(f.Index).IsZero()
		})
		forced = true
	}
	if err := firePreSave(ctx, db, obj); err != nil {
		return err
	}
	now := db.now()
	for _, f := range fields {
		if f.AutoNow && isTemporalAutoKind(f.Kind) {
			stampTemporalAuto(v.Field(f.Index), f.Kind, now)
		}
	}
	var updated bool
	if len(fields) > 0 {
		updated, err = db.updateRow(ctx, m, v, fields)
	} else {
		updated, err = db.rowExists(ctx, m, v)
	}
	if err != nil {
		markForRollbackOnError(ctx, err)
		return err
	}
	if !updated {
		if forced {
			err := fmt.Errorf("orm: save %s: save with update_fields did not affect any rows: %w",
				m.Name(), ErrDoesNotExist)
			markForRollbackOnError(ctx, err)
			return err
		}
		return qs.insertOne(ctx, obj)
	}
	markLoaded(m, v, fields)
	return firePostSave(context.WithValue(ctx, createdKey{}, false), db, obj)
}

// saveFields returns the fields Save writes: every field but the primary key,
// or the ones named by UpdateFields.
func saveFields(m *Model, keys []string) ([]*Field, error) {
	if len(keys) == 0 {
		fields := make([]*Field, 0, len(m.fields))
		for _, f := range m.fields {
			if f != m.pk {
				fields = append(fields, f)
			}
		}
		return fields, nil
	}
	fields := make([]*Field, 0, len(keys))
	for _, key := range keys {
		f, ok := lookupField(m, key)
		if !ok || f == m.pk {
			return nil, &FieldError{
				Model: m.Name(),
				Message: "the following fields do not exist in this model, are m2m fields, " +
					"primary keys, or are non-concrete fields: " + key,
			}
		}
		fields = append(fields, f)
	}
	return fields, nil
}

// updateRow writes the given fields of the struct v to the row with v's
// primary key. It reports whether such a row exists.
func (db *DB) updateRow(ctx context.Context, m *Model, v reflect.Value, fields []*Field) (bool, error) {
	w := newSQLWriter(db.dialect)
	w.write("UPDATE ")
	w.ident(m.Table())
	w.write(" SET ")
	for i, f := range fields {
		if i > 0 {
			w.write(", ")
		}
		w.ident(f.Column)
		w.write(" = ")
		arg, err := bindField(db.dialect, f, v.Field(f.Index).Interface())
		if err != nil {
			return false, fmt.Errorf("orm: update %s: %w", m.Name(), err)
		}
		w.param(arg)
	}
	w.write(" WHERE ")
	w.ident(m.pk.Column)
	w.write(" = ")
	pkArg, err := bindField(db.dialect, m.pk, v.Field(m.pk.Index).Interface())
	if err != nil {
		return false, fmt.Errorf("orm: update %s: %w", m.Name(), err)
	}
	w.param(pkArg)
	result, err := db.conn(ctx).ExecContext(ctx, w.String(), w.args...)
	if err != nil {
		return false, fmt.Errorf("orm: update %s: %w", m.Name(), err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("orm: update %s: rows affected: %w", m.Name(), err)
	}
	return affected > 0, nil
}

// insert writes the structs in objs as one INSERT statement. All objects must
// agree on whether their auto primary key is set: when it is zero the column
// is left to the database and the generated keys are written back into the
// structs, where the dialect can return them. With ignoreConflicts a row that
// violates a constraint is skipped and no key is written back.
func (db *DB) insert(ctx context.Context, m *Model, objs []reflect.Value, ignoreConflicts bool) error {
	if len(objs) == 0 {
		return nil
	}
	d := db.dialect
	pk := m.PrimaryKey()
	generated := pk != nil && pk.Kind == KindAuto && objs[0].Field(pk.Index).IsZero()

	fields := make([]*Field, 0, len(m.fields))
	for _, f := range m.fields {
		if generated && f == pk {
			continue
		}
		fields = append(fields, f)
	}

	w := newSQLWriter(d)
	w.write("INSERT INTO ")
	w.ident(m.Table())
	if len(fields) == 0 {
		if len(objs) != 1 {
			return fmt.Errorf("orm: insert %s: a model without columns to insert is created one row at a time", m.Name())
		}
		w.write(" DEFAULT VALUES")
	} else {
		w.write(" (")
		for i, f := range fields {
			if i > 0 {
				w.write(", ")
			}
			w.ident(f.Column)
		}
		w.write(") VALUES ")
		for r, obj := range objs {
			if r > 0 {
				w.write(", ")
			}
			w.write("(")
			for i, f := range fields {
				if i > 0 {
					w.write(", ")
				}
				arg, err := bindField(d, f, obj.Field(f.Index).Interface())
				if err != nil {
					return fmt.Errorf("orm: insert %s: %w", m.Name(), err)
				}
				w.param(arg)
			}
			w.write(")")
		}
	}
	// A row of defaults has no value that could conflict.
	if ignoreConflicts && len(fields) > 0 {
		w.write(" ON CONFLICT DO NOTHING")
	}

	if generated && !ignoreConflicts && d.SupportsReturning() {
		w.write(" RETURNING ")
		w.ident(pk.Column)
		return db.insertReturning(ctx, m, objs, w)
	}
	result, err := db.conn(ctx).ExecContext(ctx, w.String(), w.args...)
	if err != nil {
		return fmt.Errorf("orm: insert %s: %w", m.Name(), err)
	}
	// Without RETURNING only the key of a single row is known.
	if generated && !ignoreConflicts && len(objs) == 1 {
		id, err := result.LastInsertId()
		if err != nil {
			return fmt.Errorf("orm: insert %s: last insert id: %w", m.Name(), err)
		}
		return writeBackAutoPK(objs[0].Field(pk.Index), id, m.Name())
	}
	return nil
}

// insertReturning runs an INSERT ... RETURNING pk and writes the keys back
// into objs, which the database returns in insertion order.
func (db *DB) insertReturning(ctx context.Context, m *Model, objs []reflect.Value, w *sqlWriter) error {
	rows, err := db.conn(ctx).QueryContext(ctx, w.String(), w.args...)
	if err != nil {
		return fmt.Errorf("orm: insert %s: %w", m.Name(), err)
	}
	defer func() { _ = rows.Close() }()
	i := 0
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return fmt.Errorf("orm: insert %s: %w", m.Name(), err)
		}
		if i < len(objs) {
			if err := writeBackAutoPK(objs[i].Field(m.pk.Index), id, m.Name()); err != nil {
				return err
			}
		}
		i++
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("orm: insert %s: %w", m.Name(), db.classify(err))
	}
	if i != len(objs) {
		return fmt.Errorf("orm: insert %s: %d rows inserted, %d expected", m.Name(), i, len(objs))
	}
	return nil
}

// applyDefaults sets every field that carries a default= and holds its zero
// value to the default, as Django applies a default when the object is
// created. A relation or an auto primary key never carries a default.
//
// A Go zero value cannot be told apart from "not set": an explicit zero, such
// as Active=false against default=true, is replaced by the default.
func applyDefaults(v reflect.Value, fields []*Field) {
	for _, f := range fields {
		if !f.HasDefault {
			continue
		}
		fv := v.Field(f.Index)
		if fv.IsZero() {
			setDefaultValue(fv, f.Default)
		}
	}
}

// ApplyDefaults sets every field of obj, a *T of m, that carries a default=
// and holds its zero value to the default, the values Django's Model() gives
// a new instance. Create does this itself; the admin calls it to show the
// read-only fields of an unsaved object.
func (m *Model) ApplyDefaults(obj any) {
	applyDefaults(reflect.ValueOf(obj).Elem(), m.fields)
}

// setDefaultValue assigns a parsed default value onto a struct field, matching
// the field's Go kind. Unhandled kinds leave the field untouched.
func setDefaultValue(fv reflect.Value, def any) {
	if fv.Kind() == reflect.Pointer {
		setKindDefaultValue(fv, def)
		return
	}
	switch d := def.(type) {
	case int64:
		switch fv.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			fv.SetInt(d)
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			fv.SetUint(uint64(d))
		}
	case bool:
		fv.SetBool(d)
	case string:
		if fv.Kind() == reflect.String {
			fv.SetString(d)
		} else {
			setKindDefaultValue(fv, def)
		}
	default:
		setKindDefaultValue(fv, def)
	}
}

// applyAutoNowOnCreate stamps every auto_now and auto_now_add field with now,
// for a row that is being inserted.
func applyAutoNowOnCreate(v reflect.Value, fields []*Field, now time.Time) {
	for _, f := range fields {
		if !isTemporalAutoKind(f.Kind) || (!f.AutoNow && !f.AutoNowAdd) {
			continue
		}
		stampTemporalAuto(v.Field(f.Index), f.Kind, now)
	}
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

// pkFieldIsZero reports whether the primary key field value fv is unset. For
// an O2O primary key it inspects PK() rather than struct zero-ness, because an
// O2O with a loaded unsaved object has loaded=true yet PK()==0.
func pkFieldIsZero(pk *Field, fv reflect.Value) bool {
	if pk.Rel != nil {
		if keyed, ok := fv.Interface().(interface{ PK() int64 }); ok {
			return keyed.PK() == 0
		}
	}
	return fv.IsZero()
}

// rowExists reports whether a row with v's primary key exists in m's table.
// Used by Save on models whose only column is the primary key (such as a
// primary-key O2O model).
func (db *DB) rowExists(ctx context.Context, m *Model, v reflect.Value) (bool, error) {
	w := newSQLWriter(db.dialect)
	w.write("SELECT 1 FROM ")
	w.ident(m.Table())
	w.write(" WHERE ")
	w.ident(m.pk.Column)
	w.write(" = ")
	pkArg, err := bindField(db.dialect, m.pk, v.Field(m.pk.Index).Interface())
	if err != nil {
		return false, fmt.Errorf("orm: save %s: %w", m.Name(), err)
	}
	w.param(pkArg)
	w.write(" LIMIT 1")
	rows, err := db.conn(ctx).QueryContext(ctx, w.String(), w.args...)
	if err != nil {
		return false, fmt.Errorf("orm: save %s: %w", m.Name(), err)
	}
	defer func() { _ = rows.Close() }()
	exists := rows.Next()
	if err := rows.Err(); err != nil {
		return false, fmt.Errorf("orm: save %s: %w", m.Name(), err)
	}
	return exists, nil
}

// syncLoadedRelations checks every relation field of v, a struct of model m,
// that holds a loaded object with pk == 0. If the referenced object has since
// been saved (non-zero PK), its PK is copied back into the relation field; if
// it is still unsaved, the operation is rejected with Django's
// unsaved-related-object error. A private field that holds an unsaved object
// is rejected too. operation names the operation in the error, e.g. "save".
func syncLoadedRelations(reg *Registry, m *Model, v reflect.Value, operation string) error {
	for _, f := range m.fields {
		if f.Rel == nil || f.Rel.Target == nil {
			continue
		}
		fv := v.Field(f.Index)
		keyed, ok := fv.Interface().(interface{ PK() int64 })
		if !ok || keyed.PK() != 0 {
			continue
		}
		objMeth := fv.MethodByName("Object")
		if !objMeth.IsValid() {
			continue
		}
		res := objMeth.Call(nil)
		if len(res) != 2 || !res[1].Bool() || res[0].IsNil() {
			continue
		}
		targetPK, err := extractModelPKInt64(f.Rel.Target, res[0].Elem())
		if err != nil {
			return err
		}
		if targetPK == 0 {
			return unsavedRelatedError(operation, f.Attr)
		}
		setForwardObject(v, f, res[0], targetPK)
	}
	for _, pf := range m.privateFields {
		holder, ok := v.Field(pf.Index).Interface().(ObjectHolder)
		if !ok {
			continue
		}
		obj := holder.HeldObject()
		if obj == nil {
			continue
		}
		target, ok := reg.ModelOf(obj)
		if !ok {
			return fmt.Errorf("orm: field %s.%s holds %T, which is not a registered model", m.Name(), pf.Name, obj)
		}
		pk, zero, err := extractModelPK(target, reflect.Indirect(reflect.ValueOf(obj)))
		if err != nil {
			return err
		}
		// Django checks for a pk of None, and an empty text pk is a valid one.
		if _, text := pk.(string); zero && !text {
			return unsavedRelatedError(operation, pf.Attr)
		}
	}
	return nil
}

// unsavedRelatedError is Django's error for an operation on an object whose
// relation field attr holds an unsaved object.
func unsavedRelatedError(operation, attr string) error {
	return fmt.Errorf("orm: %s() prohibited to prevent data loss due to unsaved related object '%s'", operation, attr)
}
