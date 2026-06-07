package forms

import (
	"fmt"
	"net/url"
	"reflect"
	"strconv"
	"time"

	"github.com/oliverhaas/djangogo/orm"
)

// FromModel derives a Form from an orm model. The auto primary key is skipped.
// Each model field maps to a form Field whose Kind and default Widget follow the
// orm Kind. A foreign-key column (Rel != nil) maps to an IntegerField for the
// related id, named after the model field.
func FromModel(m *orm.Model) *Form {
	var fields []*Field
	for _, mf := range m.Fields() {
		ff := formFieldFor(mf)
		if ff == nil {
			continue
		}
		fields = append(fields, ff)
	}
	return New(fields...)
}

// formFieldFor builds a form Field for a single orm field, or returns nil to
// skip it (the auto primary key).
func formFieldFor(mf *orm.Field) *Field {
	// Foreign keys map to an integer id input before the scalar Kind switch,
	// since an FK column has orm Kind KindInt but a non-nil Rel.
	if mf.Rel != nil {
		return &Field{
			Name:     mf.Name,
			Label:    humanize(mf.Name),
			Required: !mf.Null,
			Kind:     IntegerField,
			Widget:   NumberInput{},
		}
	}

	switch mf.Kind {
	case orm.KindAuto:
		return nil
	case orm.KindChar:
		return &Field{
			Name:      mf.Name,
			Label:     humanize(mf.Name),
			Required:  !mf.Null,
			Kind:      CharField,
			MaxLength: mf.MaxLength,
			Widget:    TextInput{},
		}
	case orm.KindText:
		return &Field{
			Name:     mf.Name,
			Label:    humanize(mf.Name),
			Required: !mf.Null,
			Kind:     TextField,
			Widget:   Textarea{},
		}
	case orm.KindInt:
		return &Field{
			Name:     mf.Name,
			Label:    humanize(mf.Name),
			Required: !mf.Null,
			Kind:     IntegerField,
			Widget:   NumberInput{},
		}
	case orm.KindBool:
		return &Field{
			Name:   mf.Name,
			Label:  humanize(mf.Name),
			Kind:   BoolField,
			Widget: CheckboxInput{},
		}
	case orm.KindDateTime:
		return &Field{
			Name:     mf.Name,
			Label:    humanize(mf.Name),
			Required: !mf.Null,
			Kind:     DateTimeField,
			Widget:   TextInput{},
		}
	default:
		return nil
	}
}

// FromStruct derives a Form from a model and pre-fills it with the current field
// values of obj (a pointer to an instance of the model's struct type).
func FromStruct(m *orm.Model, obj any) *Form {
	f := FromModel(m)
	rv := reflect.ValueOf(obj)
	if rv.Kind() == reflect.Pointer {
		rv = rv.Elem()
	}
	data := url.Values{}
	if rv.Kind() == reflect.Struct {
		for _, mf := range m.Fields() {
			ff := formFieldFor(mf)
			if ff == nil {
				continue
			}
			fv := rv.Field(mf.Index)
			if s, ok := stringifyFieldValue(mf, fv); ok {
				data.Set(mf.Name, s)
			}
		}
	}
	return f.Bind(data)
}

// stringifyFieldValue renders a struct field value as the string a widget would
// re-display. It returns false when the value should be omitted.
func stringifyFieldValue(mf *orm.Field, fv reflect.Value) (string, bool) {
	if mf.Rel != nil {
		pk := fv.MethodByName("PK").Call(nil)[0].Int()
		if pk == 0 {
			return "", false
		}
		return strconv.FormatInt(pk, 10), true
	}
	switch fv.Kind() {
	case reflect.String:
		return fv.String(), true
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(fv.Int(), 10), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return strconv.FormatUint(fv.Uint(), 10), true
	case reflect.Bool:
		if fv.Bool() {
			return "true", true
		}
		return "", false
	case reflect.Struct:
		if t, ok := fv.Interface().(time.Time); ok {
			if t.IsZero() {
				return "", false
			}
			return t.Format(time.RFC3339), true
		}
		return "", false
	default:
		return "", false
	}
}

// PopulateStruct sets the exported fields of dest (a pointer to a struct) from
// cleaned data, matching by orm Field.Name. It handles string, int64, bool, and
// time.Time values, and sets a foreign key's primary key via its SetPK method
// from the cleaned int64.
func PopulateStruct(m *orm.Model, cleaned map[string]any, dest any) error {
	rv := reflect.ValueOf(dest)
	if rv.Kind() != reflect.Pointer || rv.IsNil() {
		return fmt.Errorf("forms: PopulateStruct requires a non-nil pointer, got %T", dest)
	}
	elem := rv.Elem()
	if elem.Kind() != reflect.Struct {
		return fmt.Errorf("forms: PopulateStruct requires a pointer to a struct, got %T", dest)
	}

	for _, mf := range m.Fields() {
		val, ok := cleaned[mf.Name]
		if !ok {
			continue
		}
		fv := elem.Field(mf.Index)

		if mf.Rel != nil {
			pk, ok := val.(int64)
			if !ok {
				return fmt.Errorf("forms: field %s: FK value must be int64, got %T", mf.Name, val)
			}
			fv.Addr().MethodByName("SetPK").Call([]reflect.Value{reflect.ValueOf(pk)})
			continue
		}

		if err := setScalar(mf, fv, val); err != nil {
			return err
		}
	}
	return nil
}

// setScalar assigns a cleaned scalar value to a struct field, converting integer
// widths as needed.
func setScalar(mf *orm.Field, fv reflect.Value, val any) error {
	switch v := val.(type) {
	case string:
		fv.SetString(v)
	case int64:
		switch fv.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			fv.SetInt(v)
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			fv.SetUint(uint64(v))
		default:
			return fmt.Errorf("forms: field %s: cannot assign int64 to %s", mf.Name, fv.Kind())
		}
	case bool:
		fv.SetBool(v)
	case time.Time:
		fv.Set(reflect.ValueOf(v))
	default:
		return fmt.Errorf("forms: field %s: unsupported cleaned type %T", mf.Name, val)
	}
	return nil
}

// humanize turns a Go field name into a display label. For now it returns the
// name unchanged; a future revision may insert spaces at word boundaries.
func humanize(name string) string { return name }
