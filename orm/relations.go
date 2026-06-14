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
	// OnDelete is the referential action when the referenced row is deleted. It
	// defaults to OnDeleteDoNothing (SQL NO ACTION) when on_delete is unset.
	OnDelete OnDelete
	// targetType is the related struct type (e.g. Author).
	targetType reflect.Type
	// Target is the resolved related model, set by Registry.Resolve.
	Target *Model
}

// OnDelete is the referential action applied when the row a foreign key points
// to is deleted; it maps to the SQL ON DELETE clause. djangogo has no app-level
// cascade collector, so the action is enforced entirely by the database.
type OnDelete uint8

const (
	// OnDeleteDoNothing emits no ON DELETE clause (SQL NO ACTION): a delete of a
	// referenced row is left to the database, which rejects it when foreign-key
	// constraints are enforced. It is the default when on_delete is unset and
	// matches Django's DO_NOTHING.
	OnDeleteDoNothing OnDelete = iota
	// OnDeleteCascade deletes the rows that reference the deleted row
	// (ON DELETE CASCADE), matching Django's CASCADE.
	OnDeleteCascade
	// OnDeleteSetNull sets the foreign key to NULL (ON DELETE SET NULL); the
	// column must be nullable. It matches Django's SET_NULL.
	OnDeleteSetNull
	// OnDeleteRestrict rejects the delete while referencing rows exist
	// (ON DELETE RESTRICT), matching Django's RESTRICT.
	OnDeleteRestrict
)

// Clause returns the SQL ON DELETE fragment for od with a leading space, or ""
// for OnDeleteDoNothing (NO ACTION is the SQL default, so no clause is emitted).
func (od OnDelete) Clause() string {
	switch od {
	case OnDeleteCascade:
		return " ON DELETE CASCADE"
	case OnDeleteSetNull:
		return " ON DELETE SET NULL"
	case OnDeleteRestrict:
		return " ON DELETE RESTRICT"
	default:
		return ""
	}
}

// String returns the on_delete tag name for od (e.g. "cascade").
func (od OnDelete) String() string {
	switch od {
	case OnDeleteCascade:
		return "cascade"
	case OnDeleteSetNull:
		return "set_null"
	case OnDeleteRestrict:
		return "restrict"
	default:
		return "do_nothing"
	}
}

// ParseOnDelete maps an on_delete tag value to an OnDelete, erroring on an
// unknown value.
func ParseOnDelete(raw string) (OnDelete, error) {
	switch raw {
	case "cascade":
		return OnDeleteCascade, nil
	case "set_null":
		return OnDeleteSetNull, nil
	case "restrict":
		return OnDeleteRestrict, nil
	case "do_nothing":
		return OnDeleteDoNothing, nil
	default:
		return OnDeleteDoNothing, fmt.Errorf("invalid on_delete %q: must be cascade, set_null, restrict, or do_nothing", raw)
	}
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

// RelatedObjects returns a QuerySet of Child rows whose FK column equals
// parentPK. fkColumn is the FK column on Child (e.g. "author_id"). It documents
// the reverse-FK pattern; callers may equivalently write
// Query[Child](db).Filter(fkColumn, parentPK).
func RelatedObjects[Child any](db *DB, fkColumn string, parentPK int64) *QuerySet[Child] {
	return Query[Child](db).Filter(fkColumn, parentPK)
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

// Prefetch loads, in a single query, all Child rows whose fkColumn is one of
// parentPKs, and returns a map from each parent PK to its children. fkColumn is
// the FK column on Child (e.g. "author_id"). This is the batched-IN reverse
// equivalent of Django's prefetch_related: it avoids the N+1 of fetching
// children per parent. When parentPKs is empty it returns an empty map and runs
// no query.
func Prefetch[Child any](ctx context.Context, db *DB, fkColumn string, parentPKs []int64) (map[int64][]Child, error) {
	out := make(map[int64][]Child)
	if len(parentPKs) == 0 {
		return out, nil
	}

	var zero Child
	m, ok := db.Registry().ModelOf(zero)
	if !ok {
		return nil, fmt.Errorf("orm: Prefetch: no model registered for %T", zero)
	}
	var fkField *Field
	for _, f := range m.Fields() {
		if f.Column == fkColumn && f.Rel != nil {
			fkField = f
			break
		}
	}
	if fkField == nil {
		return nil, fmt.Errorf("orm: Prefetch: %s has no FK column %q", m.Name(), fkColumn)
	}

	children, err := Query[Child](db).Filter(fkColumn+"__in", parentPKs).All(ctx)
	if err != nil {
		return nil, err
	}
	for i := range children {
		// The FK field is an FK[Parent] value; its PK() method yields the parent pk.
		pk := reflect.ValueOf(children[i]).Field(fkField.Index).MethodByName("PK").Call(nil)[0].Int()
		out[pk] = append(out[pk], children[i])
	}
	return out, nil
}

// PKsOf returns the auto primary-key values of parents, read by reflection from
// each parent's primary-key field. It returns an error when no model is
// registered for T or the model has no primary key.
func PKsOf[T any](db *DB, parents []T) ([]int64, error) {
	if len(parents) == 0 {
		return nil, nil
	}
	var zero T
	m, ok := db.Registry().ModelOf(zero)
	if !ok {
		return nil, fmt.Errorf("orm: PKsOf: no model registered for %T", zero)
	}
	pk := m.PrimaryKey()
	if pk == nil {
		return nil, fmt.Errorf("orm: PKsOf: model %s has no primary key", m.Name())
	}
	pks := make([]int64, len(parents))
	for i := range parents {
		pks[i] = reflect.ValueOf(parents[i]).Field(pk.Index).Int()
	}
	return pks, nil
}
