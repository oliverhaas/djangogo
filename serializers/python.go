package serializers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"iter"
	"math"
	"net/netip"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/oliverhaas/djangogo/internal/duration"
	"github.com/oliverhaas/djangogo/internal/pyjson"
	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/validators"
)

// pythonFormat is the registry entry for the internal "python" serializer.
type pythonFormat struct{}

// InternalUseOnly marks the python serializer as internal so PublicFormats
// excludes it.
func (pythonFormat) InternalUseOnly() bool { return true }

// Serialize drains objs to validate that serialization succeeds. Call ToObjects
// to obtain the []*Object slice directly.
func (pythonFormat) Serialize(_ io.Writer, objs iter.Seq2[*Object, error], _ *Options) error {
	for _, err := range objs {
		if err != nil {
			return err
		}
	}
	return nil
}

// Deserialize returns an error because the python format operates on *Object
// values via FromObjects rather than an io.Reader stream.
func (pythonFormat) Deserialize(_ io.Reader) iter.Seq2[*Object, error] {
	return func(yield func(*Object, error) bool) {
		yield(nil, NewDeserializationError("Error deserializing object: python format requires FromObjects"))
	}
}

// ToObjects converts a slice of model instances or pointers into their
// Python-serializer Object representations, porting Django's
// django.core.serializers.python.Serializer.
func ToObjects(ctx context.Context, db *orm.DB, objs any, opts ...Option) ([]*Object, error) {
	o := NewOptions(opts...)
	var out []*Object
	for obj, err := range iterObjects(ctx, db, objs, o) {
		if err != nil {
			return nil, err
		}
		out = append(out, obj)
	}
	return out, nil
}

// iterObjects yields one *Object per element of objs and updates o's
// ProgressBar after each element.
func iterObjects(ctx context.Context, db *orm.DB, objs any, o *Options) iter.Seq2[*Object, error] {
	return func(yield func(*Object, error) bool) {
		if objs == nil {
			return
		}
		rv := reflect.ValueOf(objs)
		if rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array {
			yield(nil, &SerializationError{Message: fmt.Sprintf("serializers: expected slice of model instances, got %T", objs)})
			return
		}
		var tempReg *orm.Registry
		pb := NewProgressBar(o.ProgressOutput, o.ObjectCount)
		n := rv.Len()
		for i := range n {
			elem := rv.Index(i).Interface()
			m, sv, ptr, err := resolveInstance(db, &tempReg, elem)
			if err != nil {
				yield(nil, err)
				return
			}
			obj, err := serializeOneObject(ctx, db, m, sv, ptr, o)
			if err != nil {
				yield(nil, err)
				return
			}
			pb.Update(i + 1)
			if !yield(obj, nil) {
				return
			}
		}
	}
}

// resolveInstance extracts the *orm.Model, struct reflect.Value, and pointer
// any for instance.
func resolveInstance(db *orm.DB, tempReg **orm.Registry, instance any) (*orm.Model, reflect.Value, any, error) {
	if instance == nil {
		return nil, reflect.Value{}, nil, &SerializationError{Message: "serializers: cannot serialize nil model instance"}
	}
	rv := reflect.ValueOf(instance)
	var ptr any
	var sv reflect.Value
	switch rv.Kind() {
	case reflect.Pointer:
		if rv.IsNil() {
			return nil, reflect.Value{}, nil, &SerializationError{Message: "serializers: cannot serialize nil model pointer"}
		}
		sv = rv.Elem()
		ptr = instance
	case reflect.Struct:
		sv = rv
		p := reflect.New(rv.Type())
		p.Elem().Set(rv)
		ptr = p.Interface()
	default:
		return nil, reflect.Value{}, nil, &SerializationError{Message: fmt.Sprintf("serializers: expected model struct or pointer, got %T", instance)}
	}
	if sv.Kind() != reflect.Struct {
		return nil, reflect.Value{}, nil, &SerializationError{Message: fmt.Sprintf("serializers: expected model struct, got %T", instance)}
	}

	if db != nil && db.Registry() != nil {
		if m, ok := db.Registry().ModelOf(ptr); ok {
			return m, sv, ptr, nil
		}
	}
	if *tempReg == nil {
		*tempReg = orm.NewRegistry()
	}
	if m, ok := (*tempReg).ModelOf(ptr); ok {
		return m, sv, ptr, nil
	}
	m, err := (*tempReg).Register(reflect.New(sv.Type()).Interface())
	if err != nil {
		return nil, reflect.Value{}, nil, &SerializationError{Message: err.Error(), Err: err}
	}
	return m, sv, ptr, nil
}

// serializeOneObject builds the *Object for a single model instance.
func serializeOneObject(
	ctx context.Context,
	db *orm.DB,
	m *orm.Model,
	sv reflect.Value,
	ptr any,
	o *Options,
) (*Object, error) {
	out := &Object{
		Model: m.LabelLower(),
	}
	pkField := m.PrimaryKey()
	if !o.UseNaturalPrimaryKeys || !modelHasNaturalKey(m) {
		out.HasPK = true
		if pkField != nil {
			val, err := valueFromField(m, sv, pkField)
			if err != nil {
				return nil, err
			}
			out.PK = val
		}
	}

	fields := m.Fields()
	m2mFields := m.ManyToMany()
	out.Fields = make([]FieldValue, 0, len(fields)+len(m2mFields))

	for _, f := range fields {
		if f == pkField {
			continue
		}
		if o.SelectedFields != nil && !slices.Contains(o.SelectedFields, f.Attr) {
			continue
		}
		if f.Rel == nil {
			val, err := valueFromField(m, sv, f)
			if err != nil {
				return nil, err
			}
			out.Fields = append(out.Fields, FieldValue{Name: f.Attr, Value: val})
		} else {
			val, err := handleFKField(ctx, db, sv, f, o)
			if err != nil {
				return nil, err
			}
			out.Fields = append(out.Fields, FieldValue{Name: f.Attr, Value: val})
		}
	}

	for _, f := range m2mFields {
		if f.Rel == nil || !f.Rel.AutoCreated {
			continue
		}
		if o.SelectedFields != nil && !slices.Contains(o.SelectedFields, f.Attr) {
			continue
		}
		val, err := handleM2MField(ctx, db, m, sv, ptr, f, o)
		if err != nil {
			return nil, err
		}
		out.Fields = append(out.Fields, FieldValue{Name: f.Attr, Value: val})
	}
	return out, nil
}

// handleFKField serializes a ForeignKey or OneToOneField value as its target
// primary key or, when NaturalForeignKeys is active and the target implements
// NaturalKeyer or ContextNaturalKeyer, the target's natural key slice.
func handleFKField(ctx context.Context, db *orm.DB, sv reflect.Value, f *orm.Field, o *Options) (any, error) {
	fv := sv.Field(f.Index)
	pk, loadedObj := extractRelationState(f.Rel.Target, fv)
	if pk == 0 && loadedObj == nil {
		return nil, nil
	}
	if o.UseNaturalForeignKeys && modelHasNaturalKey(f.Rel.Target) {
		target := loadedObj
		if target == nil {
			if db == nil {
				return nil, &SerializationError{Message: fmt.Sprintf("serializers: cannot fetch natural foreign key %s.%s without a database", f.Rel.Target.Name(), f.Attr)}
			}
			fetched, err := orm.GetByPK(ctx, db, f.Rel.Target, pk)
			if err != nil {
				return nil, err
			}
			target = fetched
		}
		nk, _, err := naturalKeyOfContext(ctx, db, target)
		if err != nil {
			return nil, err
		}
		return nk, nil
	}
	if pk == 0 {
		return nil, nil
	}
	return pk, nil
}

// handleM2MField serializes a forward auto-created many-to-many field from the
// prefetch cache when loaded, or via orm.RelatedMany otherwise.
func handleM2MField(
	ctx context.Context,
	db *orm.DB,
	m *orm.Model,
	sv reflect.Value,
	ptr any,
	f *orm.Field,
	o *Options,
) (any, error) {
	targetModel := f.Rel.Target
	useNat := o.UseNaturalForeignKeys && modelHasNaturalKey(targetModel)

	m2mValue := func(related any) (any, error) {
		if useNat {
			nk, _, err := naturalKeyOfContext(ctx, db, related)
			return nk, err
		}
		relSV := reflect.Indirect(reflect.ValueOf(related))
		return valueFromField(targetModel, relSV, targetModel.PrimaryKey())
	}

	if items, _, loaded := orm.CachedList(m, ptr, f.Attr); loaded && items != nil {
		rv := reflect.ValueOf(items)
		out := make([]any, 0, rv.Len())
		for i := range rv.Len() {
			val, err := m2mValue(rv.Index(i).Interface())
			if err != nil {
				return nil, err
			}
			out = append(out, val)
		}
		return out, nil
	}

	if db == nil || instancePKForDisplay(m, sv) == nil {
		return []any{}, nil
	}
	qs, err := orm.RelatedMany(db, ptr, f)
	if err != nil {
		return nil, err
	}
	if !useNat && embedsRelCache(targetModel) {
		qs = qs.Only("pk")
	}
	rows, err := qs.All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]any, 0, len(rows))
	for _, row := range rows {
		val, err := m2mValue(row)
		if err != nil {
			return nil, err
		}
		out = append(out, val)
	}
	return out, nil
}

// embedsRelCache reports whether the struct of m embeds orm.RelCache, which
// QuerySet.Only needs to record the deferred fields.
func embedsRelCache(m *orm.Model) bool {
	t := m.GoType()
	if t == nil || t.Kind() != reflect.Struct {
		return false
	}
	for i := range t.NumField() {
		if t.Field(i).Type == reflect.TypeFor[orm.RelCache]() {
			return true
		}
	}
	return false
}

// extractRelationState reads the integer primary key and optional loaded object
// from an orm.FK[T] or orm.O2O[T] struct field.
func extractRelationState(targetModel *orm.Model, fv reflect.Value) (int64, any) {
	var pk int64
	if keyed, ok := fv.Interface().(interface{ PK() int64 }); ok {
		pk = keyed.PK()
	}
	var loadedObj any
	if objMeth := fv.MethodByName("Object"); objMeth.IsValid() {
		res := objMeth.Call(nil)
		if len(res) == 2 && res[1].Bool() && !res[0].IsNil() {
			loadedObj = res[0].Interface()
			if pk == 0 && targetModel != nil {
				if p, ok := instancePKForDisplay(targetModel, res[0].Elem()).(int64); ok {
					pk = p
				}
			}
		}
	}
	return pk, loadedObj
}

// instancePKForDisplay returns the primary key value of sv, or nil when an
// auto, relation, or zero UUID primary key is unset.
func instancePKForDisplay(m *orm.Model, sv reflect.Value) any {
	if m == nil || !sv.IsValid() {
		return nil
	}
	pk := m.PrimaryKey()
	if pk == nil {
		return nil
	}
	val, err := valueFromField(m, sv, pk)
	if err != nil {
		return nil
	}
	return val
}

// valueFromField extracts the Python-layer value for field f from struct value
// sv: protected values pass through, and other types convert via Django's
// value_to_string rules.
func valueFromField(_ *orm.Model, sv reflect.Value, f *orm.Field) (any, error) {
	fv := sv.Field(f.Index)
	if f.Rel != nil {
		pk, _ := extractRelationState(f.Rel.Target, fv)
		if pk == 0 {
			return nil, nil
		}
		return pk, nil
	}
	if fv.Kind() == reflect.Pointer {
		if fv.IsNil() {
			return nil, nil
		}
		fv = fv.Elem()
	}

	switch f.Kind {
	case orm.KindAuto:
		n := readIntField(fv)
		if n == 0 {
			return nil, nil
		}
		return n, nil
	case orm.KindInt, orm.KindSmallInt, orm.KindBigInt,
		orm.KindPositiveSmallInt, orm.KindPositiveInt, orm.KindPositiveBigInt:
		return readIntField(fv), nil
	case orm.KindFloat:
		if fv.Kind() == reflect.Float32 {
			fl, _ := strconv.ParseFloat(strconv.FormatFloat(fv.Float(), 'g', -1, 32), 64)
			return fl, nil
		}
		return fv.Float(), nil
	case orm.KindBool:
		return fv.Bool(), nil
	case orm.KindChar, orm.KindText, orm.KindSlug, orm.KindEmail, orm.KindURL:
		return fv.String(), nil
	case orm.KindDecimal:
		if dec, ok := fv.Interface().(orm.Decimal); ok {
			return dec, nil
		}
	case orm.KindDateTime:
		if t, ok := fv.Interface().(time.Time); ok {
			if t.IsZero() && f.Null {
				return nil, nil
			}
			return t.UTC(), nil
		}
	case orm.KindDate:
		switch v := fv.Interface().(type) {
		case orm.Date:
			if v.IsZero() && f.Null {
				return nil, nil
			}
			return v, nil
		case time.Time:
			if v.IsZero() && f.Null {
				return nil, nil
			}
			return orm.DateFromTime(v), nil
		}
	case orm.KindTime:
		switch v := fv.Interface().(type) {
		case orm.Time:
			return v, nil
		case time.Time:
			return orm.TimeFromTime(v), nil
		}
	case orm.KindDuration:
		if d, ok := fv.Interface().(time.Duration); ok {
			return duration.String(d), nil
		}
	case orm.KindUUID:
		switch v := fv.Interface().(type) {
		case orm.UUID:
			if f.PrimaryKey && v.IsZero() {
				return nil, nil
			}
			return v.String(), nil
		case string:
			if f.PrimaryKey && v == "" {
				return nil, nil
			}
			return v, nil
		}
	case orm.KindJSON:
		return jsonFieldToValue(f, fv)
	case orm.KindBinary:
		if b, ok := fv.Interface().([]byte); ok {
			if b == nil && f.Null {
				return nil, nil
			}
			return base64.StdEncoding.EncodeToString(b), nil
		}
	case orm.KindGenericIPAddress:
		switch v := fv.Interface().(type) {
		case netip.Addr:
			if !v.IsValid() {
				if f.Null {
					return nil, nil
				}
				return "", nil
			}
			return v.String(), nil
		case string:
			return v, nil
		}
	}
	return fv.Interface(), nil
}

// readIntField reads a signed or unsigned integer reflect.Value as int64.
func readIntField(fv reflect.Value) int64 {
	switch fv.Kind() {
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return int64(fv.Uint())
	default:
		return fv.Int()
	}
}

// jsonFieldToValue decodes a KindJSON struct field into its unmarshaled value
// for Object.Fields.
func jsonFieldToValue(f *orm.Field, fv reflect.Value) (any, error) {
	if raw, ok := fv.Interface().(json.RawMessage); ok {
		if len(raw) == 0 {
			return nil, nil
		}
		if decoded, err := pyjson.Decode(raw); err == nil {
			return decoded, nil
		}
		return string(raw), nil
	}
	if (fv.Kind() == reflect.Slice || fv.Kind() == reflect.Map || fv.Kind() == reflect.Interface) && fv.IsNil() && f.Null {
		return nil, nil
	}
	b, err := json.Marshal(fv.Interface())
	if err != nil {
		return nil, &SerializationError{Message: err.Error(), Err: err}
	}
	return pyjson.Decode(b)
}

// FromObjects deserializes a sequence of Objects into unsaved
// DeserializedObject values, porting Django's
// django.core.serializers.python.Deserializer.
func FromObjects(
	ctx context.Context,
	db *orm.DB,
	objs iter.Seq2[*Object, error],
	opts ...Option,
) iter.Seq2[*DeserializedObject, error] {
	o := NewOptions(opts...)
	return func(yield func(*DeserializedObject, error) bool) {
		for obj, err := range objs {
			if err != nil {
				var de *DeserializationError
				if errors.As(err, &de) {
					yield(nil, err)
				} else {
					yield(nil, NewDeserializationError("Error deserializing object: "+err.Error()))
				}
				return
			}
			if obj == nil {
				yield(nil, NewDeserializationError("Error deserializing object: nil object"))
				return
			}
			dObj, skip, err := handleObject(ctx, db, obj, o)
			if err != nil {
				var de *DeserializationError
				if !errors.As(err, &de) {
					err = NewDeserializationError("Error deserializing object: " + err.Error())
				}
				yield(nil, err)
				return
			}
			if skip {
				continue
			}
			if !yield(dObj, nil) {
				return
			}
		}
	}
}

// handleObject converts a single *Object into a *DeserializedObject.
func handleObject(
	ctx context.Context,
	db *orm.DB,
	obj *Object,
	o *Options,
) (dObj *DeserializedObject, skip bool, err error) {
	if db == nil || db.Registry() == nil {
		return nil, false, NewDeserializationError(fmt.Sprintf("Invalid model identifier: '%s'", obj.Model))
	}
	m, err := lookupModel(db.Registry(), obj.Model)
	if err != nil {
		if o.IgnoreNonexistent {
			return nil, true, nil
		}
		return nil, false, err
	}

	pkField := m.PrimaryKey()
	var pkVal any
	hasExplicitPK := false
	if obj.HasPK && pkField != nil {
		convertedPK, err := toPythonPK(pkField, obj.PK)
		if err != nil {
			return nil, false, DeserializationErrorWithData(err, obj.Model, obj.PK, nil)
		}
		if convertedPK != nil {
			pkVal = convertedPK
			hasExplicitPK = true
		}
	}

	fieldValues := make(map[*orm.Field]any, len(obj.Fields))
	m2mData := make(map[string][]any)
	var deferredFields map[*orm.Field]any
	var nullFields []*orm.Field

	for _, fv := range obj.Fields {
		if m2mField, ok := m.ManyToManyField(fv.Name); ok {
			values, deferred, badVal, err := deserializeM2MValues(ctx, db, m2mField, fv.Value, o.HandleForwardReferences)
			if err != nil {
				return nil, false, DeserializationErrorWithData(err, obj.Model, obj.PK, badVal)
			}
			if deferred {
				if deferredFields == nil {
					deferredFields = make(map[*orm.Field]any)
				}
				deferredFields[m2mField] = fv.Value
			} else {
				m2mData[m2mField.Attr] = values
			}
			continue
		}

		field, ok := m.FieldByAttr(fv.Name)
		if !ok {
			if o.IgnoreNonexistent {
				continue
			}
			return nil, false, NewDeserializationError(fmt.Sprintf("Error deserializing object: %s has no field named '%s'", m.Name(), fv.Name))
		}

		if field.Rel != nil && (field.Rel.Kind == orm.RelFK || field.Rel.Kind == orm.RelO2O) {
			val, deferred, err := deserializeFKValue(ctx, db, field, fv.Value, o.HandleForwardReferences)
			if err != nil {
				return nil, false, DeserializationErrorWithData(err, obj.Model, obj.PK, fv.Value)
			}
			if deferred {
				if deferredFields == nil {
					deferredFields = make(map[*orm.Field]any)
				}
				deferredFields[field] = fv.Value
			} else {
				if val == nil && !field.Null {
					nullFields = append(nullFields, field)
				}
				fieldValues[field] = val
			}
			continue
		}

		val, err := toPython(field, fv.Value)
		if err != nil {
			return nil, false, DeserializationErrorWithData(err, obj.Model, obj.PK, fv.Value)
		}
		if val == nil && !field.Null {
			nullFields = append(nullFields, field)
		}
		fieldValues[field] = val
	}

	instance, err := buildInstance(ctx, db, m, pkField, pkVal, hasExplicitPK, fieldValues)
	if err != nil {
		return nil, false, err
	}
	return &DeserializedObject{
		Object:         instance,
		Model:          m,
		M2MData:        m2mData,
		DeferredFields: deferredFields,
		nullFields:     nullFields,
	}, false, nil
}

// buildInstance allocates a new instance of m, applies defaults, sets parsed
// field values, and resolves the primary key via natural keys when omitted.
func buildInstance(
	ctx context.Context,
	db *orm.DB,
	m *orm.Model,
	pkField *orm.Field,
	pkVal any,
	hasExplicitPK bool,
	fieldValues map[*orm.Field]any,
) (any, error) {
	ptr := reflect.New(m.GoType())
	elem := ptr.Elem()
	m.ApplyDefaults(ptr.Interface())

	for _, f := range m.Fields() {
		val, present := fieldValues[f]
		if !present {
			continue
		}
		if err := setModelField(f, elem.Field(f.Index), val); err != nil {
			return nil, NewDeserializationError("Error deserializing object: " + err.Error())
		}
	}

	if hasExplicitPK && pkField != nil {
		if err := setModelField(pkField, elem.Field(pkField.Index), pkVal); err != nil {
			return nil, NewDeserializationError("Error deserializing object: " + err.Error())
		}
		return ptr.Interface(), nil
	}

	if pkField != nil && !hasExplicitPK {
		if modelHasNaturalKey(m) && modelHasNaturalKeyGetter(m) && db != nil {
			nk, _, err := naturalKeyOfContext(ctx, db, ptr.Interface())
			if err != nil {
				return nil, NewDeserializationError("Error deserializing object: " + err.Error())
			}
			found, err := callGetByNaturalKey(ctx, db, m, nk)
			switch {
			case err == nil && found != nil:
				foundSV := reflect.Indirect(reflect.ValueOf(found))
				foundPK := instancePKForDisplay(m, foundSV)
				if err := setModelField(pkField, elem.Field(pkField.Index), foundPK); err != nil {
					return nil, NewDeserializationError("Error deserializing object: " + err.Error())
				}
			case err != nil && !errors.Is(err, orm.ErrDoesNotExist):
				return nil, NewDeserializationError("Error deserializing object: " + err.Error())
			case errors.Is(err, orm.ErrDoesNotExist) && !pkField.HasDefault:
				elem.Field(pkField.Index).SetZero()
			}
		}
	}
	return ptr.Interface(), nil
}

// lookupModel resolves "app_label.model_name" in reg with a case-sensitive app
// label and case-insensitive model name, matching Django's apps.get_model.
func lookupModel(reg *orm.Registry, identifier string) (*orm.Model, error) {
	if identifier == "" {
		return nil, NewDeserializationError("Invalid model identifier: ''")
	}
	if strings.Contains(identifier, ".") {
		app, name, _ := strings.Cut(identifier, ".")
		if app == "" || name == "" || strings.Contains(name, ".") {
			return nil, NewDeserializationError(fmt.Sprintf("Invalid model identifier: '%s'", identifier))
		}
		for _, m := range reg.AppModels(app) {
			if strings.EqualFold(m.Name(), name) {
				return m, nil
			}
		}
		return nil, NewDeserializationError(fmt.Sprintf("Invalid model identifier: '%s'", identifier))
	}
	for _, m := range reg.Models() {
		if m.App() == "" && strings.EqualFold(m.Name(), identifier) {
			return m, nil
		}
	}
	return nil, NewDeserializationError(fmt.Sprintf("Invalid model identifier: '%s'", identifier))
}

// deserializeM2MValues converts a raw M2M list into target primary keys,
// porting Django's deserialize_m2m_values.
func deserializeM2MValues(
	ctx context.Context,
	db *orm.DB,
	field *orm.Field,
	fieldValue any,
	handleForwardReferences bool,
) (values []any, deferred bool, badVal any, err error) {
	items, ok := asIterableSlice(fieldValue)
	if !ok {
		return nil, false, fieldValue, fmt.Errorf("'%s' object is not iterable", pyTypeName(fieldValue))
	}
	targetModel := field.Rel.Target
	hasNatGetter := modelHasNaturalKeyGetter(targetModel)
	targetPKField := targetModel.PrimaryKey()

	values = make([]any, 0, len(items))
	for _, item := range items {
		if hasNatGetter {
			if natKey, isList := asIterableSlice(item); isList {
				targetObj, err := callGetByNaturalKey(ctx, db, targetModel, natKey)
				if err != nil {
					if errors.Is(err, orm.ErrDoesNotExist) && handleForwardReferences {
						return nil, true, nil, nil
					}
					return nil, false, item, err
				}
				targetSV := reflect.Indirect(reflect.ValueOf(targetObj))
				values = append(values, instancePKForDisplay(targetModel, targetSV))
				continue
			}
		}
		pk, err := toPythonPK(targetPKField, item)
		if err != nil {
			if errors.Is(err, orm.ErrDoesNotExist) && handleForwardReferences {
				return nil, true, nil, nil
			}
			return nil, false, item, err
		}
		values = append(values, pk)
	}
	return values, false, nil, nil
}

// deserializeFKValue converts a raw FK or O2O value into the target primary
// key, porting Django's deserialize_fk_value.
func deserializeFKValue(
	ctx context.Context,
	db *orm.DB,
	field *orm.Field,
	fieldValue any,
	handleForwardReferences bool,
) (val any, deferred bool, err error) {
	if fieldValue == nil {
		return nil, false, nil
	}
	targetModel := field.Rel.Target
	if modelHasNaturalKeyGetter(targetModel) {
		if natKey, isList := asIterableSlice(fieldValue); isList {
			targetObj, err := callGetByNaturalKey(ctx, db, targetModel, natKey)
			if err != nil {
				if errors.Is(err, orm.ErrDoesNotExist) && handleForwardReferences {
					return nil, true, nil
				}
				return nil, false, err
			}
			targetSV := reflect.Indirect(reflect.ValueOf(targetObj))
			return instancePKForDisplay(targetModel, targetSV), false, nil
		}
	}
	pk, err := toPythonPK(targetModel.PrimaryKey(), fieldValue)
	if err != nil {
		return nil, false, err
	}
	return pk, false, nil
}

// asIterableSlice converts v into a []any when v is a non-string, non-[]byte
// slice or array, matching Python's hasattr(v, "__iter__") and not
// isinstance(v, (str, bytes)).
func asIterableSlice(v any) ([]any, bool) {
	if v == nil {
		return nil, false
	}
	switch x := v.(type) {
	case string, []byte, json.RawMessage:
		return nil, false
	case []any:
		return x, true
	default:
		rv := reflect.ValueOf(v)
		if rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array {
			return nil, false
		}
		if rv.Type().Elem().Kind() == reflect.Uint8 {
			return nil, false
		}
		out := make([]any, rv.Len())
		for i := range rv.Len() {
			out[i] = rv.Index(i).Interface()
		}
		return out, true
	}
}

// pyTypeName returns the Python type name for v used in TypeError messages.
func pyTypeName(v any) string {
	switch v.(type) {
	case nil:
		return "NoneType"
	case bool:
		return "bool"
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return "int"
	case float32, float64:
		return "float"
	case string:
		return "str"
	default:
		return fmt.Sprintf("%T", v)
	}
}

// toPythonPK converts raw into the Go value for pkField, unwrapping relation
// primary keys (such as an O2O primary key) to their underlying integer kind.
func toPythonPK(pkField *orm.Field, raw any) (any, error) {
	if pkField == nil || raw == nil {
		return nil, nil
	}
	if pkField.Rel != nil && pkField.Rel.Target != nil && pkField.Rel.Target.PrimaryKey() != nil {
		return toPythonPK(pkField.Rel.Target.PrimaryKey(), raw)
	}
	return toPython(pkField, raw)
}

// toPython converts value into the Go representation expected by field f,
// raising an *orm.ValidationError with Django's field error messages on
// invalid input.
func toPython(f *orm.Field, value any) (any, error) {
	switch f.Kind {
	case orm.KindAuto, orm.KindInt, orm.KindSmallInt, orm.KindBigInt,
		orm.KindPositiveSmallInt, orm.KindPositiveInt, orm.KindPositiveBigInt:
		return toPythonInteger(f, value)
	case orm.KindFloat:
		return toPythonFloat(f, value)
	case orm.KindDecimal:
		return toPythonDecimal(f, value)
	case orm.KindBool:
		return toPythonBool(f, value)
	case orm.KindChar, orm.KindText, orm.KindSlug, orm.KindEmail, orm.KindURL:
		return toPythonChar(value)
	case orm.KindDate:
		return toPythonDate(f, value)
	case orm.KindDateTime:
		return toPythonDateTime(f, value)
	case orm.KindTime:
		return toPythonTime(f, value)
	case orm.KindDuration:
		return toPythonDuration(f, value)
	case orm.KindUUID:
		return toPythonUUID(f, value)
	case orm.KindBinary:
		return toPythonBinary(value)
	case orm.KindGenericIPAddress:
		return toPythonIP(f, value)
	case orm.KindJSON:
		return value, nil
	default:
		return value, nil
	}
}

// fieldInvalidError returns an *orm.ValidationError for code on f, falling back
// to fallbackMsg when f has no custom override for code.
func fieldInvalidError(f *orm.Field, code, fallbackMsg string, value any) error {
	msg := fallbackMsg
	if f != nil {
		if custom := f.ErrorMessage(code); custom != "" {
			msg = custom
		}
	}
	return orm.NewValidationError(msg, code, map[string]any{"value": value})
}

// toPythonInteger implements IntegerField.to_python.
func toPythonInteger(f *orm.Field, value any) (any, error) {
	if value == nil {
		return nil, nil
	}
	const invalidMsg = "“%(value)s” value must be an integer."
	switch v := value.(type) {
	case int64:
		return v, nil
	case int:
		return int64(v), nil
	case int32:
		return int64(v), nil
	case int16:
		return int64(v), nil
	case int8:
		return int64(v), nil
	case uint:
		if uint64(v) > math.MaxInt64 {
			return nil, fieldInvalidError(f, "invalid", invalidMsg, value)
		}
		return int64(v), nil
	case uint64:
		if v > math.MaxInt64 {
			return nil, fieldInvalidError(f, "invalid", invalidMsg, value)
		}
		return int64(v), nil
	case uint32:
		return int64(v), nil
	case uint16:
		return int64(v), nil
	case uint8:
		return int64(v), nil
	case bool:
		if v {
			return int64(1), nil
		}
		return int64(0), nil
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) || v < float64(math.MinInt64) || v >= float64(math.MaxInt64) {
			return nil, fieldInvalidError(f, "invalid", invalidMsg, value)
		}
		return int64(v), nil
	case json.Number:
		if n, err := strconv.ParseInt(string(v), 10, 64); err == nil {
			return n, nil
		}
		if fl, err := strconv.ParseFloat(string(v), 64); err == nil && !math.IsNaN(fl) && !math.IsInf(fl, 0) && fl >= float64(math.MinInt64) && fl < float64(math.MaxInt64) {
			return int64(fl), nil
		}
		return nil, fieldInvalidError(f, "invalid", invalidMsg, string(v))
	case string:
		if n, ok := parsePyInt(v); ok {
			return n, nil
		}
		return nil, fieldInvalidError(f, "invalid", invalidMsg, v)
	default:
		return nil, fieldInvalidError(f, "invalid", invalidMsg, value)
	}
}

// parsePyInt parses s according to Python's int(s) rules (optional whitespace,
// optional sign, Unicode decimal digits, single underscores between digits).
func parsePyInt(s string) (int64, bool) {
	s = strings.TrimSpace(s)
	var b strings.Builder
	prevDigit := false
	for i, r := range s {
		switch {
		case i == 0 && (r == '+' || r == '-'):
			b.WriteRune(r)
		case r == '_' && prevDigit:
			prevDigit = false
		case unicode.Is(unicode.Nd, r):
			b.WriteByte(byte('0' + unicodeDigitValue(r)))
			prevDigit = true
		default:
			return 0, false
		}
	}
	if !prevDigit {
		return 0, false
	}
	n, err := strconv.ParseInt(b.String(), 10, 64)
	return n, err == nil
}

// unicodeDigitValue returns the numeric value (0..9) of a Unicode decimal digit.
func unicodeDigitValue(r rune) int {
	for _, rng := range unicode.Nd.R16 {
		if lo, hi := rune(rng.Lo), rune(rng.Hi); r >= lo && r <= hi {
			return int(r-lo) % 10
		}
	}
	for _, rng := range unicode.Nd.R32 {
		if lo, hi := rune(rng.Lo), rune(rng.Hi); r >= lo && r <= hi {
			return int(r-lo) % 10
		}
	}
	return 0
}

// toPythonFloat implements FloatField.to_python.
func toPythonFloat(f *orm.Field, value any) (any, error) {
	if value == nil {
		return nil, nil
	}
	const invalidMsg = "“%(value)s” value must be a float."
	switch v := value.(type) {
	case float64:
		return v, nil
	case float32:
		fl, _ := strconv.ParseFloat(strconv.FormatFloat(float64(v), 'g', -1, 32), 64)
		return fl, nil
	case int64:
		return float64(v), nil
	case int:
		return float64(v), nil
	case int32:
		return float64(v), nil
	case int16:
		return float64(v), nil
	case int8:
		return float64(v), nil
	case uint, uint64, uint32, uint16, uint8:
		fl, _ := strconv.ParseFloat(fmt.Sprint(v), 64)
		return fl, nil
	case json.Number:
		fl, err := strconv.ParseFloat(string(v), 64)
		if err != nil {
			return nil, fieldInvalidError(f, "invalid", invalidMsg, string(v))
		}
		return fl, nil
	case string:
		fl, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		if err != nil {
			return nil, fieldInvalidError(f, "invalid", invalidMsg, v)
		}
		return fl, nil
	default:
		return nil, fieldInvalidError(f, "invalid", invalidMsg, value)
	}
}

// toPythonDecimal implements DecimalField.to_python.
func toPythonDecimal(f *orm.Field, value any) (any, error) {
	if value == nil {
		return nil, nil
	}
	const invalidMsg = "“%(value)s” value must be a decimal number."
	switch v := value.(type) {
	case orm.Decimal:
		return v, nil
	case string:
		d, err := orm.ParseDecimal(v)
		if err != nil {
			return nil, fieldInvalidError(f, "invalid", invalidMsg, v)
		}
		return d, nil
	case json.Number:
		d, err := orm.ParseDecimal(string(v))
		if err != nil {
			return nil, fieldInvalidError(f, "invalid", invalidMsg, string(v))
		}
		return d, nil
	case int64:
		return orm.DecimalFromInt(v), nil
	case int:
		return orm.DecimalFromInt(int64(v)), nil
	case int32:
		return orm.DecimalFromInt(int64(v)), nil
	case int16:
		return orm.DecimalFromInt(int64(v)), nil
	case int8:
		return orm.DecimalFromInt(int64(v)), nil
	case float64:
		d, err := orm.DecimalFromFloat(v)
		if err != nil {
			return nil, fieldInvalidError(f, "invalid", invalidMsg, v)
		}
		return d, nil
	case float32:
		d, err := orm.DecimalFromFloat(float64(v))
		if err != nil {
			return nil, fieldInvalidError(f, "invalid", invalidMsg, v)
		}
		return d, nil
	default:
		return nil, fieldInvalidError(f, "invalid", invalidMsg, value)
	}
}

// toPythonBool implements BooleanField.to_python.
func toPythonBool(f *orm.Field, value any) (any, error) {
	nullable := f != nil && f.Null
	if nullable && (value == nil || value == "") {
		return nil, nil
	}
	if !nullable && value == nil {
		return nil, nil
	}
	switch v := value.(type) {
	case bool:
		return v, nil
	case int, int64, int32, int16, int8:
		n := reflect.ValueOf(v).Int()
		if n == 1 {
			return true, nil
		}
		if n == 0 {
			return false, nil
		}
	case json.Number:
		switch string(v) {
		case "1":
			return true, nil
		case "0":
			return false, nil
		}
	case string:
		switch v {
		case "t", "True", "1":
			return true, nil
		case "f", "False", "0":
			return false, nil
		}
	}
	if nullable {
		return nil, fieldInvalidError(f, "invalid_nullable", "“%(value)s” value must be either True, False, or None.", value)
	}
	return nil, fieldInvalidError(f, "invalid", "“%(value)s” value must be either True or False.", value)
}

// toPythonChar implements CharField.to_python and TextField.to_python.
func toPythonChar(value any) (any, error) {
	if value == nil {
		return nil, nil
	}
	if s, ok := value.(string); ok {
		return s, nil
	}
	return validators.PyString(value), nil
}

var (
	dateRE     = regexp.MustCompile(`^(\d{4})-(\d{1,2})-(\d{1,2})$`)
	timeRE     = regexp.MustCompile(`^(\d{1,2}):(\d{1,2})(?::(\d{1,2})(?:[.,](\d{1,6})\d{0,6})?)?$`)
	dateTimeRE = regexp.MustCompile(`^(\d{4})-(\d{1,2})-(\d{1,2})[T ](\d{1,2}):(\d{1,2})(?::(\d{1,2})(?:[.,](\d{1,6})\d{0,6})?)?\s*(Z|[+-]\d{2}(?::?\d{2})?)?$`)
)

// toPythonDate implements DateField.to_python.
func toPythonDate(f *orm.Field, value any) (any, error) {
	if value == nil {
		return nil, nil
	}
	switch v := value.(type) {
	case orm.Date:
		return v, nil
	case time.Time:
		return orm.DateFromTime(v.UTC()), nil
	case string:
		d, wellFormatted, validDate := parseDateString(v)
		if validDate {
			return d, nil
		}
		if wellFormatted {
			return nil, fieldInvalidError(
				f,
				"invalid_date",
				"“%(value)s” value has the correct format (YYYY-MM-DD) but it is an invalid date.",
				v,
			)
		}
		return nil, fieldInvalidError(
			f,
			"invalid",
			"“%(value)s” value has an invalid date format. It must be in YYYY-MM-DD format.",
			v,
		)
	default:
		return nil, fieldInvalidError(
			f,
			"invalid",
			"“%(value)s” value has an invalid date format. It must be in YYYY-MM-DD format.",
			value,
		)
	}
}

// parseDateString parses s with Django's date_re, distinguishing format errors
// from out-of-range calendar dates.
func parseDateString(s string) (d orm.Date, wellFormatted, validDate bool) {
	m := dateRE.FindStringSubmatch(s)
	if m == nil {
		return orm.Date{}, false, false
	}
	year, _ := strconv.Atoi(m[1])
	month, _ := strconv.Atoi(m[2])
	day, _ := strconv.Atoi(m[3])
	if year < 1 || month < 1 || month > 12 || day < 1 || day > 31 {
		return orm.Date{}, true, false
	}
	t := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
	if t.Year() != year || int(t.Month()) != month || t.Day() != day {
		return orm.Date{}, true, false
	}
	return orm.NewDate(year, time.Month(month), day), true, true
}

// toPythonDateTime implements DateTimeField.to_python.
func toPythonDateTime(f *orm.Field, value any) (any, error) {
	if value == nil {
		return nil, nil
	}
	switch v := value.(type) {
	case time.Time:
		return v.UTC(), nil
	case orm.Date:
		return v.Time(), nil
	case string:
		if t, wellFormatted, validDT := parseDateTimeString(v); wellFormatted {
			if validDT {
				return t.UTC(), nil
			}
			return nil, fieldInvalidError(
				f,
				"invalid_datetime",
				"“%(value)s” value has the correct format (YYYY-MM-DD HH:MM[:ss[.uuuuuu]][TZ]) but it is an invalid date/time.",
				v,
			)
		}
		if d, wellFormatted, validDate := parseDateString(v); wellFormatted {
			if validDate {
				return d.Time(), nil
			}
			return nil, fieldInvalidError(
				f,
				"invalid_date",
				"“%(value)s” value has the correct format (YYYY-MM-DD) but it is an invalid date.",
				v,
			)
		}
		return nil, fieldInvalidError(
			f,
			"invalid",
			"“%(value)s” value has an invalid format. It must be in YYYY-MM-DD HH:MM[:ss[.uuuuuu]][TZ] format.",
			v,
		)
	default:
		return nil, fieldInvalidError(
			f,
			"invalid",
			"“%(value)s” value has an invalid format. It must be in YYYY-MM-DD HH:MM[:ss[.uuuuuu]][TZ] format.",
			value,
		)
	}
}

// parseDateTimeString parses s with Django's datetime_re, distinguishing format
// errors from out-of-range calendar or clock components.
func parseDateTimeString(s string) (t time.Time, wellFormatted, validDT bool) {
	m := dateTimeRE.FindStringSubmatch(s)
	if m == nil {
		return time.Time{}, false, false
	}
	year, _ := strconv.Atoi(m[1])
	month, _ := strconv.Atoi(m[2])
	day, _ := strconv.Atoi(m[3])
	hour, _ := strconv.Atoi(m[4])
	minute, _ := strconv.Atoi(m[5])
	second := 0
	if m[6] != "" {
		second, _ = strconv.Atoi(m[6])
	}
	nanos := 0
	if m[7] != "" {
		padded := (m[7] + "000000")[:6]
		micros, _ := strconv.Atoi(padded)
		nanos = micros * 1000
	}
	if year < 1 || month < 1 || month > 12 || day < 1 || day > 31 || hour > 23 || minute > 59 || second > 59 {
		return time.Time{}, true, false
	}
	loc := time.UTC
	if tz := m[8]; tz != "" && tz != "Z" {
		digits := strings.ReplaceAll(tz[1:], ":", "")
		tzHours, _ := strconv.Atoi(digits[:2])
		tzMins := 0
		if len(digits) == 4 {
			tzMins, _ = strconv.Atoi(digits[2:])
		}
		if tzHours > 23 || tzMins > 59 {
			return time.Time{}, true, false
		}
		offset := tzHours*3600 + tzMins*60
		if tz[0] == '-' {
			offset = -offset
		}
		loc = time.FixedZone("", offset)
	}
	constructed := time.Date(year, time.Month(month), day, hour, minute, second, nanos, loc)
	if constructed.Year() != year || int(constructed.Month()) != month || constructed.Day() != day {
		return time.Time{}, true, false
	}
	return constructed, true, true
}

// toPythonTime implements TimeField.to_python.
func toPythonTime(f *orm.Field, value any) (any, error) {
	if value == nil {
		return nil, nil
	}
	switch v := value.(type) {
	case orm.Time:
		return v, nil
	case time.Time:
		return orm.TimeFromTime(v), nil
	case string:
		tm, wellFormatted, validTime := parseTimeString(v)
		if validTime {
			return tm, nil
		}
		if wellFormatted {
			return nil, fieldInvalidError(
				f,
				"invalid_time",
				"“%(value)s” value has the correct format (HH:MM[:ss[.uuuuuu]]) but it is an invalid time.",
				v,
			)
		}
		return nil, fieldInvalidError(
			f,
			"invalid",
			"“%(value)s” value has an invalid format. It must be in HH:MM[:ss[.uuuuuu]] format.",
			v,
		)
	default:
		return nil, fieldInvalidError(
			f,
			"invalid",
			"“%(value)s” value has an invalid format. It must be in HH:MM[:ss[.uuuuuu]] format.",
			value,
		)
	}
}

// parseTimeString parses s with Django's time_re, distinguishing format errors
// from out-of-range hour, minute, or second values.
func parseTimeString(s string) (tm orm.Time, wellFormatted, validTime bool) {
	m := timeRE.FindStringSubmatch(s)
	if m == nil {
		return orm.Time{}, false, false
	}
	hour, _ := strconv.Atoi(m[1])
	minute, _ := strconv.Atoi(m[2])
	second := 0
	if m[3] != "" {
		second, _ = strconv.Atoi(m[3])
	}
	nanos := 0
	if m[4] != "" {
		padded := (m[4] + "000000")[:6]
		micros, _ := strconv.Atoi(padded)
		nanos = micros * 1000
	}
	if hour > 23 || minute > 59 || second > 59 {
		return orm.Time{}, true, false
	}
	return orm.NewTime(hour, minute, second, nanos), true, true
}

// toPythonDuration implements DurationField.to_python.
func toPythonDuration(f *orm.Field, value any) (any, error) {
	if value == nil {
		return nil, nil
	}
	if d, ok := value.(time.Duration); ok {
		return d, nil
	}
	if s, ok := value.(string); ok {
		if d, ok, overflow := duration.Parse(s); ok && !overflow {
			return d, nil
		}
	}
	return nil, fieldInvalidError(
		f,
		"invalid",
		"“%(value)s” value has an invalid format. It must be in [DD] [[HH:]MM:]ss[.uuuuuu] format.",
		value,
	)
}

// toPythonUUID implements UUIDField.to_python.
func toPythonUUID(f *orm.Field, value any) (any, error) {
	if value == nil {
		return nil, nil
	}
	if u, ok := value.(orm.UUID); ok {
		return u, nil
	}
	if s, ok := value.(string); ok {
		u, err := orm.ParseUUID(s)
		if err != nil {
			return nil, fieldInvalidError(f, "invalid", "“%(value)s” is not a valid UUID.", s)
		}
		return u, nil
	}
	return nil, fieldInvalidError(f, "invalid", "“%(value)s” is not a valid UUID.", value)
}

// toPythonBinary implements BinaryField.to_python, decoding base64 strings into
// []byte.
func toPythonBinary(value any) (any, error) {
	if value == nil {
		return nil, nil
	}
	if b, ok := value.([]byte); ok {
		return b, nil
	}
	if s, ok := value.(string); ok {
		b, err := base64.StdEncoding.DecodeString(s)
		if err != nil {
			return nil, err
		}
		return b, nil
	}
	return nil, fmt.Errorf("cannot convert %T to []byte", value)
}

// toPythonIP implements GenericIPAddressField.to_python.
func toPythonIP(f *orm.Field, value any) (any, error) {
	if value == nil {
		return nil, nil
	}
	if addr, ok := value.(netip.Addr); ok {
		return addr.String(), nil
	}
	s := strings.TrimSpace(validators.PyString(value))
	if strings.Contains(s, ":") {
		addr, err := netip.ParseAddr(s)
		if err != nil || addr.Zone() != "" || !addr.Is6() {
			return nil, fieldInvalidError(f, "invalid", "This is not a valid IPv6 address.", s)
		}
		if addr.Is4In6() {
			if f != nil && f.UnpackIPv4 {
				return addr.Unmap().String(), nil
			}
			return "::ffff:" + addr.Unmap().String(), nil
		}
		return addr.String(), nil
	}
	return s, nil
}

var (
	ormDecimalType = reflect.TypeFor[orm.Decimal]()
	ormDateType    = reflect.TypeFor[orm.Date]()
	ormTimeType    = reflect.TypeFor[orm.Time]()
	ormUUIDType    = reflect.TypeFor[orm.UUID]()
	goTimeType     = reflect.TypeFor[time.Time]()
	netipAddrType  = reflect.TypeFor[netip.Addr]()
	rawMsgType     = reflect.TypeFor[json.RawMessage]()
	byteSliceType  = reflect.TypeFor[[]byte]()
)

// setModelField writes a converted toPython value val onto struct field fv for
// ORM field f.
func setModelField(f *orm.Field, fv reflect.Value, val any) error {
	if val == nil {
		fv.SetZero()
		return nil
	}
	if f != nil && f.Rel != nil && (f.Rel.Kind == orm.RelFK || f.Rel.Kind == orm.RelO2O) {
		pk, ok := val.(int64)
		if !ok {
			converted, err := toPythonInteger(f, val)
			if err != nil {
				return err
			}
			pk, _ = converted.(int64)
		}
		set := fv.Addr().MethodByName("SetPK")
		if !set.IsValid() {
			return fmt.Errorf("relation field %s has no SetPK method", f.Name)
		}
		set.Call([]reflect.Value{reflect.ValueOf(pk)})
		return nil
	}
	if fv.Kind() == reflect.Pointer {
		target := reflect.New(fv.Type().Elem())
		if err := setModelField(f, target.Elem(), val); err != nil {
			return err
		}
		fv.Set(target)
		return nil
	}
	if f != nil && f.Kind == orm.KindJSON {
		return setJSONField(fv, val)
	}

	rv := reflect.ValueOf(val)
	if rv.Type().AssignableTo(fv.Type()) {
		fv.Set(rv)
		return nil
	}

	switch fv.Type() {
	case ormDecimalType:
		if d, ok := val.(orm.Decimal); ok {
			fv.Set(reflect.ValueOf(d))
			return nil
		}
	case ormDateType:
		switch x := val.(type) {
		case orm.Date:
			fv.Set(reflect.ValueOf(x))
			return nil
		case time.Time:
			fv.Set(reflect.ValueOf(orm.DateFromTime(x)))
			return nil
		}
	case ormTimeType:
		switch x := val.(type) {
		case orm.Time:
			fv.Set(reflect.ValueOf(x))
			return nil
		case time.Time:
			fv.Set(reflect.ValueOf(orm.TimeFromTime(x)))
			return nil
		}
	case goTimeType:
		switch x := val.(type) {
		case time.Time:
			fv.Set(reflect.ValueOf(x))
			return nil
		case orm.Date:
			fv.Set(reflect.ValueOf(x.Time()))
			return nil
		case orm.Time:
			fv.Set(reflect.ValueOf(time.Date(0, 1, 1, x.Hour, x.Minute, x.Second, x.Nanosecond, time.UTC)))
			return nil
		}
	case ormUUIDType:
		switch x := val.(type) {
		case orm.UUID:
			fv.Set(reflect.ValueOf(x))
			return nil
		case string:
			u, err := orm.ParseUUID(x)
			if err != nil {
				return err
			}
			fv.Set(reflect.ValueOf(u))
			return nil
		}
	case netipAddrType:
		if s, ok := val.(string); ok {
			if s == "" {
				fv.SetZero()
				return nil
			}
			addr, err := netip.ParseAddr(s)
			if err != nil {
				return err
			}
			fv.Set(reflect.ValueOf(addr))
			return nil
		}
	case byteSliceType:
		if b, ok := val.([]byte); ok {
			fv.SetBytes(b)
			return nil
		}
	}

	switch fv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if n, ok := val.(int64); ok && !fv.OverflowInt(n) {
			fv.SetInt(n)
			return nil
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if n, ok := val.(int64); ok && n >= 0 && !fv.OverflowUint(uint64(n)) {
			fv.SetUint(uint64(n))
			return nil
		}
	case reflect.Float32, reflect.Float64:
		if fl, ok := val.(float64); ok {
			fv.SetFloat(fl)
			return nil
		}
	case reflect.String:
		switch x := val.(type) {
		case string:
			fv.SetString(x)
			return nil
		case orm.UUID:
			fv.SetString(x.String())
			return nil
		case netip.Addr:
			if x.IsValid() {
				fv.SetString(x.String())
			} else {
				fv.SetString("")
			}
			return nil
		default:
			fv.SetString(validators.PyString(val))
			return nil
		}
	}
	return fmt.Errorf("cannot assign %T to %s", val, fv.Type())
}

// setJSONField writes val into a KindJSON struct field (json.RawMessage or any
// Go type tagged orm:"type=json").
func setJSONField(fv reflect.Value, val any) error {
	if raw, ok := val.(json.RawMessage); ok {
		if fv.Type() == rawMsgType {
			cp := make(json.RawMessage, len(raw))
			copy(cp, raw)
			fv.Set(reflect.ValueOf(cp))
			return nil
		}
		return json.Unmarshal(raw, fv.Addr().Interface())
	}
	encoded, err := pyjson.Marshal(val, pyjson.Options{
		Indent:      -1,
		EnsureASCII: false,
		Default:     DjangoJSONDefault,
	})
	if err != nil {
		return err
	}
	if fv.Type() == rawMsgType {
		fv.Set(reflect.ValueOf(json.RawMessage(encoded)))
		return nil
	}
	return json.Unmarshal([]byte(encoded), fv.Addr().Interface())
}
