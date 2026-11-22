package contenttypes

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/oliverhaas/djangogo/orm"
)

// GenericForeignKey is a field that refers to an instance of any model,
// Django's GenericForeignKey. It has no column: the model stores the
// reference in two other fields, a foreign key to ContentType and an object
// id field that holds the primary key. The tag names them, and defaults to
// content_type and object_id:
//
//	type TaggedItem struct {
//		ID            int64
//		ContentType   orm.FK[contenttypes.ContentType]
//		ObjectID      int64
//		ContentObject contenttypes.GenericForeignKey `orm:"ct_field=content_type;fk_field=object_id"`
//	}
//
// Read the object with GetObject and assign it with SetObject. GetOrCreate
// and UpdateOrCreate accept the field name in their defaults.
type GenericForeignKey struct {
	obj    any
	cached bool
}

// PrivateField marks GenericForeignKey as a field without a column.
func (GenericForeignKey) PrivateField() {}

// HeldObject returns the object SetObject assigned or GetObject loaded, or
// nil, so that Save refuses an unsaved one.
func (g GenericForeignKey) HeldObject() any {
	if !g.cached {
		return nil
	}
	return g.obj
}

// AssignPrivateField assigns value to the generic foreign key pf of instance,
// for the defaults of GetOrCreate and UpdateOrCreate.
func (GenericForeignKey) AssignPrivateField(ctx context.Context, db *orm.DB, _ *orm.Model, pf *orm.PrivateField, instance, value any) error {
	return SetObject(ctx, db, instance, pf.Attr, value)
}

// ResolvePrivateField checks that the content type field of the generic
// foreign key pf of m is a foreign key to ContentType and that its object id
// field exists, Django's GenericForeignKey checks.
func (GenericForeignKey) ResolvePrivateField(m *orm.Model, pf *orm.PrivateField) error {
	_, _, err := GenericForeignKeyFields(m, pf)
	return err
}

// PrefetchTarget returns the model and the primary key of the object that the
// generic foreign key pf of instance refers to, so that PrefetchRelated loads
// the objects of many instances with one query per content type. A stale
// content type refers to no object.
func (GenericForeignKey) PrefetchTarget(ctx context.Context, db *orm.DB, pf *orm.PrivateField, instance any) (*orm.Model, any, error) {
	t, err := lookupGFK(db, instance, pf.Attr)
	if err != nil {
		return nil, nil, err
	}
	ctID := t.elem.Field(t.ct.Index).Interface().(interface{ PK() int64 }).PK()
	objectID := plainKey(t.elem.Field(t.fk.Index).Interface())
	if ctID == 0 || objectID == nil {
		return nil, nil, nil
	}
	ct, err := GetForID(ctx, db, ctID)
	if errors.Is(err, orm.ErrDoesNotExist) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	m, ok := ModelClass(db, ct)
	if !ok {
		return nil, nil, nil
	}
	return m, objectID, nil
}

// CachedObject returns the object cached on the generic foreign key pf of
// instance and reports whether one is cached.
func (GenericForeignKey) CachedObject(pf *orm.PrivateField, instance any) (any, bool) {
	g := reflect.ValueOf(instance).Elem().Field(pf.Index).Interface().(GenericForeignKey)
	return g.obj, g.cached
}

// CacheObject caches obj, which PrefetchRelated loaded, on the generic foreign
// key pf of instance.
func (GenericForeignKey) CacheObject(pf *orm.PrivateField, instance, obj any) {
	reflect.ValueOf(instance).Elem().Field(pf.Index).Set(reflect.ValueOf(GenericForeignKey{obj: obj, cached: true}))
}

// GenericForeignKeyFields returns the content type field and the object id
// field of the generic foreign key pf of m, the fields that the ct_field and
// fk_field of Django's GenericForeignKey name.
func GenericForeignKeyFields(m *orm.Model, pf *orm.PrivateField) (ct, fk *orm.Field, err error) {
	ctName, fkName := "content_type", "object_id"
	for token := range strings.SplitSeq(pf.Tag, ";") {
		token = strings.TrimSpace(token)
		key, value, _ := strings.Cut(token, "=")
		switch key {
		case "":
		case "ct_field":
			ctName = value
		case "fk_field":
			fkName = value
		default:
			return nil, nil, fmt.Errorf("unknown generic foreign key option %q", token)
		}
	}
	fk, ok := m.FieldByAttr(fkName)
	if !ok {
		return nil, nil, fmt.Errorf("the generic foreign key object id references the nonexistent field '%s'", fkName)
	}
	ct, ok = m.FieldByAttr(ctName)
	if !ok {
		return nil, nil, fmt.Errorf("the generic foreign key content type references the nonexistent field '%s.%s'",
			m.Name(), ctName)
	}
	if ct.Rel == nil || ct.Rel.Kind != orm.RelFK || ct.Rel.Target == nil ||
		ct.Rel.Target.GoType() != reflect.TypeFor[ContentType]() {
		return nil, nil, fmt.Errorf("'%s.%s' is not a foreign key to 'contenttypes.ContentType'", m.Name(), ctName)
	}
	return ct, fk, nil
}

// GenericRelation is the reverse side of the generic foreign key of T,
// Django's GenericRelation: the rows of T that refer to an instance of the
// model that declares the field. It has no column. The tag sets
// related_query_name, the name that leads from T back to the model in
// filters, and object_id_field and content_type_field, the fields of T that
// the generic foreign key uses (default object_id and content_type):
//
//	type Animal struct {
//		ID   int64
//		Tags contenttypes.GenericRelation[TaggedItem] `orm:"related_query_name=animal"`
//	}
//
// Filters follow the field like a reverse foreign key ("tags__tag"),
// PrefetchRelated loads it, deleting an instance deletes its rows, and
// orm.GenericSet manages them.
type GenericRelation[T any] struct{}

// PrivateField marks GenericRelation as a field without a column.
func (GenericRelation[T]) PrivateField() {}

// GenericRelationTarget returns the struct type T.
func (GenericRelation[T]) GenericRelationTarget() reflect.Type { return reflect.TypeFor[T]() }

// ContentTypeFilter returns the filter that selects the content type of m.
func (GenericRelation[T]) ContentTypeFilter(m *orm.Model) []any {
	key := keyOf(m)
	return []any{"app_label", key.appLabel, "model", key.model}
}

// ContentTypeID returns the primary key of the content type of m, creating
// the content type when it does not exist yet.
func (GenericRelation[T]) ContentTypeID(ctx context.Context, db *orm.DB, m *orm.Model) (any, error) {
	ct, err := GetForModel(ctx, db, m)
	if err != nil {
		return nil, err
	}
	return ct.ID, nil
}

// Check that the generic fields implement the interfaces the orm looks for.
var (
	_ orm.ObjectHolder         = GenericForeignKey{}
	_ orm.PrivateFieldAssigner = GenericForeignKey{}
	_ orm.PrivateFieldResolver = GenericForeignKey{}
	_ orm.ObjectPrefetcher     = GenericForeignKey{}
	_ orm.GenericRelationType  = GenericRelation[struct{}]{}
)

// gfkTarget is a generic foreign key of one instance.
type gfkTarget struct {
	elem   reflect.Value
	gfk    reflect.Value
	ct, fk *orm.Field
}

// lookupGFK returns the generic foreign key name of instance, a pointer to a
// model struct.
func lookupGFK(db *orm.DB, instance any, name string) (gfkTarget, error) {
	rv := reflect.ValueOf(instance)
	if rv.Kind() != reflect.Pointer || rv.IsNil() || rv.Elem().Kind() != reflect.Struct {
		return gfkTarget{}, fmt.Errorf("contenttypes: %T is not a pointer to a model struct", instance)
	}
	m, ok := db.Registry().ModelOf(instance)
	if !ok {
		return gfkTarget{}, fmt.Errorf("contenttypes: %T is not a registered model", instance)
	}
	pf, ok := m.PrivateField(name)
	if !ok || pf.Type != reflect.TypeFor[GenericForeignKey]() {
		return gfkTarget{}, &orm.FieldError{Model: m.Name(), Message: "no generic foreign key '" + name + "'"}
	}
	ct, fk, err := GenericForeignKeyFields(m, pf)
	if err != nil {
		return gfkTarget{}, fmt.Errorf("contenttypes: model %s field %s: %w", m.Name(), pf.Name, err)
	}
	elem := rv.Elem()
	return gfkTarget{elem: elem, gfk: elem.Field(pf.Index), ct: ct, fk: fk}, nil
}

// GetObject returns the object the generic foreign key name of instance
// refers to, Django's instance.<name>. instance is a pointer to a model
// struct. The result is a pointer to a model struct, or nil when the content
// type field is empty or the row does not exist. The object is cached on
// instance while its content type and primary key match the two fields.
func GetObject(ctx context.Context, db *orm.DB, instance any, name string) (any, error) {
	t, err := lookupGFK(db, instance, name)
	if err != nil {
		return nil, err
	}
	ctID := t.elem.Field(t.ct.Index).Interface().(interface{ PK() int64 }).PK()
	objectID := plainKey(t.elem.Field(t.fk.Index).Interface())
	cached := t.gfk.Interface().(GenericForeignKey)
	if cached.cached {
		if cached.obj == nil {
			return nil, nil
		}
		match, err := matches(ctx, db, cached.obj, ctID, objectID)
		if err != nil {
			return nil, err
		}
		if match {
			return cached.obj, nil
		}
	}
	var obj any
	if ctID != 0 {
		obj, err = objectForType(ctx, db, ctID, objectID)
		if err != nil {
			return nil, err
		}
	}
	t.gfk.Set(reflect.ValueOf(GenericForeignKey{obj: obj, cached: true}))
	return obj, nil
}

// objectForType loads the row with primary key pk of the model of content
// type ctID, Django's get_object_for_this_type. It returns nil for a stale
// content type or a missing row.
func objectForType(ctx context.Context, db *orm.DB, ctID int64, pk any) (any, error) {
	ct, err := GetForID(ctx, db, ctID)
	if errors.Is(err, orm.ErrDoesNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	m, ok := ModelClass(db, ct)
	if !ok || pk == nil {
		return nil, nil
	}
	obj, err := orm.GetByPK(ctx, db, m, pk)
	if errors.Is(err, orm.ErrDoesNotExist) {
		return nil, nil
	}
	return obj, err
}

// matches reports whether obj has the content type ctID and the primary key
// objectID.
func matches(ctx context.Context, db *orm.DB, obj any, ctID int64, objectID any) (bool, error) {
	m, ok := db.Registry().ModelOf(obj)
	if !ok {
		return false, fmt.Errorf("contenttypes: %T is not a registered model", obj)
	}
	ct, err := GetForModel(ctx, db, m)
	if err != nil {
		return false, err
	}
	if ct.ID != ctID {
		return false, nil
	}
	pk := plainKey(pkOf(m, obj))
	if pk == nil || objectID == nil {
		return pk == objectID, nil
	}
	// A text object id field can hold an integer primary key.
	return fmt.Sprint(pk) == fmt.Sprint(objectID), nil
}

// pkOf returns the primary key field value of obj, an instance of m.
func pkOf(m *orm.Model, obj any) any {
	return reflect.Indirect(reflect.ValueOf(obj)).Field(m.PrimaryKey().Index).Interface()
}

// plainKey returns v as int64 or string so that keys of different Go types
// compare equal, or nil for a zero or nil key.
func plainKey(v any) any {
	if keyed, ok := v.(interface{ PK() int64 }); ok {
		v = keyed.PK()
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return nil
		}
		rv = rv.Elem()
	}
	if !rv.IsValid() || rv.IsZero() {
		return nil
	}
	switch rv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return rv.Int()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return int64(rv.Uint())
	case reflect.String:
		return rv.String()
	default:
		return rv.Interface()
	}
}

// SetObject assigns obj to the generic foreign key name of instance, Django's
// instance.<name> = obj: it sets the content type field to the content type
// of obj, creating it when needed, and the object id field to the primary
// key of obj. A nil obj clears both fields. instance is a pointer to a model
// struct and obj a model struct or a pointer to one.
func SetObject(ctx context.Context, db *orm.DB, instance any, name string, obj any) error {
	t, err := lookupGFK(db, instance, name)
	if err != nil {
		return err
	}
	ctField := t.elem.Field(t.ct.Index)
	fkField := t.elem.Field(t.fk.Index)
	if obj == nil || (reflect.ValueOf(obj).Kind() == reflect.Pointer && reflect.ValueOf(obj).IsNil()) {
		ctField.Addr().Interface().(interface{ SetPK(int64) }).SetPK(0)
		fkField.SetZero()
		t.gfk.Set(reflect.ValueOf(GenericForeignKey{cached: true}))
		return nil
	}
	m, ok := db.Registry().ModelOf(obj)
	if !ok {
		return fmt.Errorf("contenttypes: %T is not a registered model", obj)
	}
	ct, err := GetForModel(ctx, db, m)
	if err != nil {
		return err
	}
	if fk, ok := ctField.Addr().Interface().(*orm.FK[ContentType]); ok {
		fk.SetObject(&ct, ct.ID)
	} else {
		ctField.Addr().Interface().(interface{ SetPK(int64) }).SetPK(ct.ID)
	}
	if err := setObjectID(fkField, pkOf(m, obj)); err != nil {
		return fmt.Errorf("contenttypes: field %s: %w", t.fk.Name, err)
	}
	t.gfk.Set(reflect.ValueOf(GenericForeignKey{obj: obj, cached: true}))
	return nil
}

// setObjectID sets the object id field fv to the primary key pk.
func setObjectID(fv reflect.Value, pk any) error {
	key := plainKey(pk)
	if key == nil {
		fv.SetZero()
		return nil
	}
	target := fv
	if fv.Kind() == reflect.Pointer {
		target = reflect.New(fv.Type().Elem()).Elem()
	}
	kv := reflect.ValueOf(key)
	switch {
	case kv.Type().AssignableTo(target.Type()):
		target.Set(kv)
	case kv.Kind() == reflect.Int64 && target.CanInt():
		target.SetInt(kv.Int())
	case kv.Kind() == reflect.Int64 && target.CanUint():
		target.SetUint(uint64(kv.Int()))
	case kv.Kind() == reflect.Int64 && target.Kind() == reflect.String:
		target.SetString(fmt.Sprint(kv.Int()))
	case kv.Kind() == reflect.String && (target.CanInt() || target.CanUint()):
		// An integer object id takes a text primary key that holds a number,
		// as Django's IntegerField.get_prep_value does.
		n, err := strconv.ParseInt(kv.String(), 10, 64)
		if err != nil || (target.CanUint() && n < 0) {
			return fmt.Errorf("expected a number but got %q", kv.String())
		}
		if target.CanInt() {
			target.SetInt(n)
		} else {
			target.SetUint(uint64(n))
		}
	case kv.Type().ConvertibleTo(target.Type()) && kv.Kind() == target.Kind():
		target.Set(kv.Convert(target.Type()))
	default:
		// A text object id holds the text form of a primary key such as a UUID.
		s, ok := key.(fmt.Stringer)
		if !ok || target.Kind() != reflect.String {
			return fmt.Errorf("cannot store a %T primary key", pk)
		}
		target.SetString(s.String())
	}
	if fv.Kind() == reflect.Pointer {
		fv.Set(target.Addr())
	}
	return nil
}
