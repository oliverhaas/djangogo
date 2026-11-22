package orm

import (
	"context"
	"fmt"
	"reflect"
)

// rawKey is the context key under which SaveRaw marks its signals as raw.
type rawKey struct{}

// Raw reports, inside a PreSave or PostSave handler, whether the save stores
// the object exactly as given, like the raw argument of Django's pre_save and
// post_save signals. Loading a fixture saves raw: a handler that reads or
// changes other rows returns early when Raw is true, because those rows can
// still be missing.
func Raw(ctx context.Context) bool {
	raw, _ := ctx.Value(rawKey{}).(bool)
	return raw
}

// SaveRaw writes obj, a pointer to an instance of a model of db, exactly as it
// is, like Django's save_base(raw=True) that loaddata and the objects of the
// serializers use. Defaults are not applied, auto_now fields are not stamped
// and related objects are not checked. An object with a primary key updates
// the row with that key, or is inserted when no such row exists. An object
// without a primary key is inserted, and the key that the database assigns is
// written back. PreSave and PostSave handlers run with Raw set, and Created
// tells a PostSave handler whether the row was inserted.
func SaveRaw(ctx context.Context, db *DB, obj any) error {
	m, ok := db.Registry().ModelOf(obj)
	rv := reflect.ValueOf(obj)
	if !ok || rv.Kind() != reflect.Pointer || rv.IsNil() {
		return fmt.Errorf("orm: SaveRaw needs a pointer to a model instance, got %T", obj)
	}
	v := rv.Elem()
	ctx = context.WithValue(ctx, rawKey{}, true)
	if err := fireModelSignal(ctx, m.goType, preSave, []reflect.Value{v}); err != nil {
		return err
	}
	pk := m.PrimaryKey()
	updated := false
	if pk != nil && !pkFieldIsZero(pk, v.Field(pk.Index)) {
		fields, err := saveFields(m, nil)
		if err != nil {
			return err
		}
		if len(fields) > 0 {
			updated, err = db.updateRow(ctx, m, v, fields)
		} else {
			updated, err = db.rowExists(ctx, m, v)
		}
		if err != nil {
			markForRollbackOnError(ctx, err)
			return err
		}
	}
	if !updated {
		if err := db.insert(ctx, m, []reflect.Value{v}, false); err != nil {
			markForRollbackOnError(ctx, err)
			return err
		}
	}
	setDeferred(m, v, nil)
	return fireModelSignal(context.WithValue(ctx, createdKey{}, !updated), m.goType, postSave, []reflect.Value{v})
}
