package orm

import (
	"fmt"
	"reflect"
)

// Stringer is the interface a model implements to control its human-readable
// label, mirroring Django's __str__. It is an alias for fmt.Stringer so a model
// satisfies it with a plain String() string method and needs no djangogo import.
type Stringer = fmt.Stringer

// Label returns the human-readable label for a model instance, mirroring
// Django's str(instance). When the instance implements Stringer its String()
// result is used; otherwise Label falls back to Django's default
// "<Model> object (<pk>)" form. The instance may be a value or a pointer.
func Label(m *Model, instance any) string {
	if s, ok := asStringer(instance); ok {
		return s.String()
	}
	return defaultLabel(m, instance)
}

// asStringer returns instance as an fmt.Stringer when it (or a pointer to a copy
// of it) implements the interface. The copy path lets a String() declared on a
// pointer receiver be found even when instance is passed by value.
func asStringer(instance any) (fmt.Stringer, bool) {
	if s, ok := instance.(fmt.Stringer); ok {
		return s, true
	}
	v := reflect.ValueOf(instance)
	if !v.IsValid() || v.Kind() == reflect.Pointer {
		return nil, false
	}
	pv := reflect.New(v.Type())
	pv.Elem().Set(v)
	if s, ok := pv.Interface().(fmt.Stringer); ok {
		return s, true
	}
	return nil, false
}

// defaultLabel returns Django's default label "<Model> object (<pk>)" for an
// instance without a String() method.
func defaultLabel(m *Model, instance any) string {
	return fmt.Sprintf("%s object (%v)", m.Name(), pkValue(m, instance))
}

// pkValue returns the primary-key value of instance via reflection, or an empty
// string when the model has no primary key or instance is not a usable struct.
func pkValue(m *Model, instance any) any {
	pk := m.PrimaryKey()
	if pk == nil {
		return ""
	}
	v := reflect.ValueOf(instance)
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return ""
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return ""
	}
	return v.Field(pk.Index).Interface()
}
