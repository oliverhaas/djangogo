package orm

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
)

// Defaults holds the field values GetOrCreate and UpdateOrCreate use beyond
// the lookup: a map from field key to value, like Django's defaults argument.
// A value of type func() any is called when it is needed.
type Defaults map[string]any

// GetOrCreate returns the row matching the conditions, or creates it when
// there is none, like Django's get_or_create(). It reports whether the row
// was created. The conditions take the form of Filter; a new row is built
// from the plain key/value conditions (the keys without a lookup or relation
// hop) and from defaults.
//
//	author, created, err := orm.Query[Author](db).GetOrCreate(ctx,
//		orm.Defaults{"age": 30}, "name", "Ada")
//
// Two callers may both find no row and both try to create it. The create
// therefore runs in its own Atomic block, and when it fails on a constraint
// the lookup is repeated: the loser of the race returns the winner's row.
// This needs a unique constraint on the lookup fields, as in Django. Any
// other failure, and a constraint violation that the lookup does not explain,
// is returned as it is.
func (qs *QuerySet[T]) GetOrCreate(ctx context.Context, defaults Defaults, args ...any) (T, bool, error) {
	var zero T
	obj, err := qs.Get(ctx, args...)
	if err == nil {
		return obj, false, nil
	}
	if !errors.Is(err, ErrDoesNotExist) {
		return zero, false, err
	}
	var fresh T
	if err := qs.populate(ctx, &fresh, args, defaults); err != nil {
		return zero, false, err
	}
	createErr := qs.q.db.Atomic(ctx, func(ctx context.Context) error {
		return qs.Create(ctx, &fresh)
	})
	if createErr == nil {
		return fresh, true, nil
	}
	if errors.Is(createErr, ErrIntegrity) {
		if existing, err := qs.Get(ctx, args...); err == nil {
			return existing, false, nil
		}
	}
	return zero, false, createErr
}

// UpdateOrCreate updates the row matching the conditions with defaults, or
// creates it from the conditions and defaults when there is none, like
// Django's update_or_create(). It reports whether the row was created. The
// whole operation runs in one Atomic block, with the row locked where the
// dialect supports it. An update goes through Save, restricted to the fields
// in defaults and the auto_now fields, and fires the save signals. When
// defaults name a private field, such as a generic foreign key, the update
// writes every field, as in Django.
func (qs *QuerySet[T]) UpdateOrCreate(ctx context.Context, defaults Defaults, args ...any) (T, bool, error) {
	var (
		out     T
		created bool
	)
	err := qs.q.db.Atomic(ctx, func(ctx context.Context) error {
		obj, isNew, err := qs.SelectForUpdate().GetOrCreate(ctx, defaults, args...)
		if err != nil {
			return err
		}
		out, created = obj, isNew
		if created {
			return nil
		}
		opts, ok, err := applyUpdateDefaults(ctx, qs.q.db, qs.q.model, &out, defaults)
		if err != nil || !ok {
			return err
		}
		return qs.Save(ctx, &out, opts...)
	})
	if err != nil {
		var zero T
		return zero, false, err
	}
	return out, created, nil
}

// applyUpdateDefaults sets the fields of obj, an instance of m, named in
// defaults, and returns the options of the Save that writes them: the fields
// in defaults and the auto_now fields, or every field when defaults name a
// private field. It reports false when there is nothing to save.
func applyUpdateDefaults(ctx context.Context, db *DB, m *Model, obj any, defaults Defaults) ([]SaveOption, bool, error) {
	keys := sortedKeys(defaults)
	elem := reflect.ValueOf(obj).Elem()
	full := false
	for _, key := range keys {
		if err := setAttr(ctx, db, m, elem, key, defaults[key]); err != nil {
			return nil, false, err
		}
		if _, ok := m.PrivateField(key); ok {
			full = true
		}
	}
	if full {
		return nil, true, nil
	}
	for _, f := range m.fields {
		if f.AutoNow {
			keys = append(keys, f.Attr)
		}
	}
	if len(keys) == 0 {
		return nil, false, nil
	}
	return []SaveOption{UpdateFields(keys...)}, true, nil
}

// sortedKeys returns the keys of d in sorted order, so that the statements
// built from it are the same on every run.
func sortedKeys(d Defaults) []string {
	keys := make([]string, 0, len(d))
	for k := range d {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// populate fills a new object from the plain key/value conditions in args and
// from defaults, which win over the conditions. Conditions that are Q objects,
// span a relation, or carry a lookup only take part in the lookup, as in
// Django.
func (qs *QuerySet[T]) populate(ctx context.Context, obj *T, args []any, defaults Defaults) error {
	m, db := qs.q.model, qs.q.db
	elem := reflect.ValueOf(obj).Elem()
	for i := 0; i < len(args); i++ {
		key, ok := args[i].(string)
		if !ok {
			continue
		}
		i++
		if i >= len(args) || strings.Contains(key, lookupSep) {
			continue
		}
		if err := setAttr(ctx, db, m, elem, key, args[i]); err != nil {
			return err
		}
	}
	for _, key := range sortedKeys(defaults) {
		if err := setAttr(ctx, db, m, elem, key, defaults[key]); err != nil {
			return err
		}
	}
	return nil
}

// setAttr sets the field of the struct elem named by key to value. A foreign
// key takes a primary key, an orm.FK[T], or a model instance. A private field
// takes what its PrivateFieldAssigner accepts, e.g. a model instance for a
// generic foreign key.
func setAttr(ctx context.Context, db *DB, m *Model, elem reflect.Value, key string, value any) error {
	if pf, ok := m.PrivateField(key); ok {
		assigner, ok := reflect.Zero(pf.Type).Interface().(PrivateFieldAssigner)
		if !ok {
			return fmt.Errorf("orm: field %s.%s cannot be assigned", m.Name(), pf.Name)
		}
		if fn, ok := value.(func() any); ok {
			value = fn()
		}
		return assigner.AssignPrivateField(ctx, db, m, pf, elem.Addr().Interface(), value)
	}
	reg := db.Registry()
	f, ok := lookupField(m, key)
	if !ok {
		return &FieldError{Model: m.Name(), Message: "invalid field name(s) for model " + m.Name() + ": '" + key + "'."}
	}
	if fn, ok := value.(func() any); ok {
		value = fn()
	}
	fv := elem.Field(f.Index)
	if value != nil && reflect.TypeOf(value) == fv.Type() {
		fv.Set(reflect.ValueOf(value))
		return nil
	}
	if f.Rel != nil {
		v, err := plainValue(reg, f, value)
		if err != nil {
			return err
		}
		sc, ok := fv.Addr().Interface().(sql.Scanner)
		if !ok {
			return fmt.Errorf("orm: field %s.%s cannot be assigned", m.Name(), f.Name)
		}
		return sc.Scan(v)
	}
	if value == nil {
		fv.SetZero()
		return nil
	}
	rv := reflect.ValueOf(value)
	if fv.Kind() == reflect.Pointer && rv.Kind() != reflect.Pointer {
		// A nullable field takes its plain value.
		ptr := reflect.New(fv.Type().Elem())
		if err := assignValue(ptr.Elem(), rv); err != nil {
			return fmt.Errorf("orm: cannot assign a %T value to field %s.%s", value, m.Name(), f.Name)
		}
		fv.Set(ptr)
		return nil
	}
	if err := assignValue(fv, rv); err != nil {
		return fmt.Errorf("orm: cannot assign a %T value to field %s.%s", value, m.Name(), f.Name)
	}
	return nil
}

// assignValue sets fv to rv, converting between numeric kinds and between
// types of the same kind.
func assignValue(fv, rv reflect.Value) error {
	switch {
	case rv.Type().AssignableTo(fv.Type()):
		fv.Set(rv)
	case isNumeric(rv.Kind()) && isNumeric(fv.Kind()),
		rv.Kind() == fv.Kind() && rv.Type().ConvertibleTo(fv.Type()):
		fv.Set(rv.Convert(fv.Type()))
	default:
		return errors.New("orm: incompatible value")
	}
	return nil
}
