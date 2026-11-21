package orm

import (
	"context"
	"database/sql/driver"
	"errors"
	"fmt"
	"reflect"
	"strconv"
)

// O2O is a forward one-to-one relation to T (a unique foreign key, Django's
// OneToOneField). It stores the related primary key and an optional loaded
// object, and implements sql.Scanner and driver.Valuer so the underlying
// integer column is read and written by database/sql directly.
//
// An O2O field is automatically marked Unique. It may also be tagged with
// orm:"pk" to serve as the model's primary key.
type O2O[T any] struct {
	pk     int64
	obj    *T
	loaded bool
}

// PK returns the related primary key.
func (o O2O[T]) PK() int64 { return o.pk }

// SetPK sets the related primary key and clears any loaded object.
func (o *O2O[T]) SetPK(pk int64) { o.pk = pk; o.obj = nil; o.loaded = false }

// SetObject records both a loaded object and its primary key.
func (o *O2O[T]) SetObject(obj *T, pk int64) { o.obj = obj; o.pk = pk; o.loaded = true }

// Object returns the loaded object and whether it has been loaded.
func (o O2O[T]) Object() (*T, bool) { return o.obj, o.loaded }

// Scan implements sql.Scanner. src is the integer foreign-key column, or nil.
func (o *O2O[T]) Scan(src any) error {
	switch v := src.(type) {
	case nil:
		o.pk = 0
	case int64:
		o.pk = v
	case int:
		o.pk = int64(v)
	case int32:
		o.pk = int64(v)
	case []byte:
		n, err := strconv.ParseInt(string(v), 10, 64)
		if err != nil {
			return fmt.Errorf("orm: O2O.Scan: cannot parse %q as int64: %w", v, err)
		}
		o.pk = n
	case string:
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return fmt.Errorf("orm: O2O.Scan: cannot parse %q as int64: %w", v, err)
		}
		o.pk = n
	default:
		return fmt.Errorf("orm: O2O.Scan: unsupported source type %T", src)
	}
	o.obj = nil
	o.loaded = false
	return nil
}

// Value implements driver.Valuer. It returns nil when the relation is unset
// (pk == 0), otherwise the primary key as an int64.
func (o O2O[T]) Value() (driver.Value, error) {
	if o.pk == 0 {
		return nil, nil
	}
	return o.pk, nil
}

// relKind reports RelO2O, allowing reflection to detect one-to-one fields.
func (O2O[T]) relKind() RelKind { return RelO2O }

// relTarget reports the related struct type.
func (O2O[T]) relTarget() reflect.Type { var z T; return reflect.TypeOf(z) }

// Fetch loads the related object. If it is already loaded it is returned
// directly without querying the database; otherwise it is fetched via
// Query[T](db).Get on the stored primary key. An unset O2O (pk == 0) returns
// ErrDoesNotExist without querying the database.
func (o *O2O[T]) Fetch(ctx context.Context, db *DB) (*T, error) {
	if o.loaded {
		if o.obj == nil {
			var zero T
			if m, ok := db.Registry().ModelOf(zero); ok {
				return nil, doesNotExist(m)
			}
			return nil, ErrDoesNotExist
		}
		return o.obj, nil
	}
	if o.pk == 0 {
		var zero T
		if m, ok := db.Registry().ModelOf(zero); ok {
			return nil, doesNotExist(m)
		}
		return nil, ErrDoesNotExist
	}
	v, err := Query[T](db).Get(ctx, "pk", o.pk)
	if err != nil {
		return nil, err
	}
	o.obj = &v
	o.loaded = true
	return &v, nil
}

// RelatedOne returns the single Child object pointing to owner through the
// reverse one-to-one relation named name (the related_name tag, or the
// lowercased Child model name by default).
//
// When owner embeds orm.RelCache and the relation was loaded via SelectRelated
// or a previous RelatedOne call, the cached object (or cached ErrDoesNotExist)
// is returned without querying the database. An unsaved owner (zero primary
// key) returns an error matching ErrDoesNotExist without querying the database.
// On a missing reverse row, the returned error matches ErrDoesNotExist and
// carries Django's "<Owner> has no <name>." message.
func RelatedOne[Child any](ctx context.Context, db *DB, owner any, name string) (*Child, error) {
	ownerModel, ownerVal, ownerPtr, err := inspectOwner(db.Registry(), owner)
	if err != nil {
		return nil, err
	}
	rev, err := lookupReverseForChild[Child](ownerModel, name, RelO2O)
	if err != nil {
		return nil, err
	}
	ownerPK, err := extractModelPKInt64(ownerModel, ownerVal)
	if err != nil {
		return nil, err
	}
	if ownerPK == 0 {
		return nil, relatedDoesNotExist(ownerModel, rev.Name)
	}
	if cached, missing, ok := getReverseOneCache(ownerModel, ownerVal, rev.Name); ok {
		if missing {
			return nil, relatedDoesNotExist(ownerModel, rev.Name)
		}
		if ptr, ok := cached.(*Child); ok {
			return ptr, nil
		}
	}
	child, err := Query[Child](db).Get(ctx, rev.Field.Column, ownerPK)
	if err != nil {
		if errors.Is(err, ErrDoesNotExist) {
			setReverseOneCacheModel(ownerModel, ownerVal, rev.Name, nil, rev.Source, true)
			return nil, relatedDoesNotExist(ownerModel, rev.Name)
		}
		return nil, err
	}
	childPtr := &child
	setReverseOneCacheModel(ownerModel, ownerVal, rev.Name, childPtr, rev.Source, false)
	// Cache the back-reference on child's forward O2O field so
	// child.<field>.Fetch requires no query.
	if !ownerPtr.IsValid() {
		cp := reflect.New(ownerModel.goType)
		cp.Elem().Set(ownerVal)
		ownerPtr = cp
	}
	setForwardObject(reflect.ValueOf(childPtr).Elem(), rev.Field, ownerPtr, ownerPK)
	return childPtr, nil
}

// setForwardObject calls SetObject(targetPtr, targetPK) on the relation field f
// of struct value elem.
func setForwardObject(elem reflect.Value, f *Field, targetPtr reflect.Value, targetPK int64) {
	set := elem.Field(f.Index).Addr().MethodByName("SetObject")
	if !set.IsValid() {
		return
	}
	set.Call([]reflect.Value{targetPtr, reflect.ValueOf(targetPK)})
}

// setForwardPK calls SetPK(pk) on the relation field f of struct value elem.
func setForwardPK(elem reflect.Value, f *Field, pk int64) {
	set := elem.Field(f.Index).Addr().MethodByName("SetPK")
	if !set.IsValid() {
		return
	}
	set.Call([]reflect.Value{reflect.ValueOf(pk)})
}

// extractModelPKInt64 reads the integer primary key of struct value sv,
// supporting both plain integer PKs and O2O primary keys.
func extractModelPKInt64(m *Model, sv reflect.Value) (int64, error) {
	pk := m.PrimaryKey()
	if pk == nil {
		return 0, fmt.Errorf("orm: model %s has no primary key", m.Name())
	}
	fv := sv.Field(pk.Index)
	if pk.Rel != nil {
		if keyed, ok := fv.Interface().(interface{ PK() int64 }); ok {
			return keyed.PK(), nil
		}
		return 0, fmt.Errorf("orm: model %s primary key %s has no PK() method", m.Name(), pk.Name)
	}
	switch fv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return fv.Int(), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return int64(fv.Uint()), nil
	default:
		return 0, fmt.Errorf("orm: model %s primary key %s is not an integer", m.Name(), pk.Name)
	}
}
