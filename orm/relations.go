package orm

import (
	"context"
	"database/sql/driver"
	"fmt"
	"reflect"
	"strconv"
)

// RelKind classifies a relation field.
type RelKind uint8

const (
	// RelNone indicates the field is not a relation.
	RelNone RelKind = iota
	// RelFK indicates a forward foreign key.
	RelFK
)

// Relation describes a forward relation on a field.
type Relation struct {
	// Kind classifies the relation (currently only RelFK).
	Kind RelKind
	// Column is the FK column on this model (e.g. "author_id").
	Column string
	// targetType is the related struct type (e.g. Author).
	targetType reflect.Type
	// Target is the resolved related model, set by Registry.Resolve.
	Target *Model
}

// FK is a forward foreign key to T. It stores the related primary key and an
// optional loaded object. It implements sql.Scanner and driver.Valuer so the FK
// column (an integer) is read and written by database/sql directly.
type FK[T any] struct {
	pk     int64
	obj    *T
	loaded bool
}

// PK returns the related primary key.
func (f FK[T]) PK() int64 { return f.pk }

// SetPK sets the related primary key and clears any loaded object.
func (f *FK[T]) SetPK(pk int64) { f.pk = pk; f.obj = nil; f.loaded = false }

// SetObject records both a loaded object and its primary key.
func (f *FK[T]) SetObject(obj *T, pk int64) { f.obj = obj; f.pk = pk; f.loaded = true }

// Object returns the loaded object and whether it has been loaded.
func (f FK[T]) Object() (*T, bool) { return f.obj, f.loaded }

// Scan implements sql.Scanner. src is the integer FK column, or nil.
func (f *FK[T]) Scan(src any) error {
	switch v := src.(type) {
	case nil:
		f.pk = 0
	case int64:
		f.pk = v
	case int:
		f.pk = int64(v)
	case int32:
		f.pk = int64(v)
	case []byte:
		n, err := strconv.ParseInt(string(v), 10, 64)
		if err != nil {
			return fmt.Errorf("orm: FK.Scan: cannot parse %q as int64: %w", v, err)
		}
		f.pk = n
	case string:
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return fmt.Errorf("orm: FK.Scan: cannot parse %q as int64: %w", v, err)
		}
		f.pk = n
	default:
		return fmt.Errorf("orm: FK.Scan: unsupported source type %T", src)
	}
	f.obj = nil
	f.loaded = false
	return nil
}

// Value implements driver.Valuer. It returns nil when the FK is unset (pk == 0),
// otherwise the primary key as an int64.
func (f FK[T]) Value() (driver.Value, error) {
	if f.pk == 0 {
		return nil, nil
	}
	return f.pk, nil
}

// relKind reports the relation kind, allowing reflection to detect relation fields.
func (FK[T]) relKind() RelKind { return RelFK }

// relTarget reports the related struct type.
func (FK[T]) relTarget() reflect.Type { var z T; return reflect.TypeOf(z) }

// relationMarker is implemented by relation field types (e.g. FK[T]) so that
// reflection can detect them without knowing the concrete type parameter.
type relationMarker interface {
	relKind() RelKind
	relTarget() reflect.Type
}

// Fetch loads the related object. If it is already loaded it is returned
// directly; otherwise it is fetched via Query[T](db).Get on the stored pk. Any
// error (including ErrDoesNotExist) is propagated.
func (f *FK[T]) Fetch(ctx context.Context, db *DB) (*T, error) {
	if f.loaded {
		return f.obj, nil
	}
	v, err := Query[T](db).Get(ctx, "id", f.pk)
	if err != nil {
		return nil, err
	}
	f.obj = &v
	f.loaded = true
	return &v, nil
}
