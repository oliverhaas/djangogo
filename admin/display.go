package admin

import (
	"fmt"
	"reflect"

	"github.com/oliverhaas/djangogo/orm"
)

// displayColumns returns the fields shown on the changelist for e. When the
// ModelAdmin.ListDisplay names specific fields they are returned in that order
// (unknown names are skipped); otherwise every model field is returned.
func displayColumns(e *entry) []*orm.Field {
	if len(e.admin.ListDisplay) == 0 {
		return e.model.Fields()
	}
	cols := make([]*orm.Field, 0, len(e.admin.ListDisplay))
	for _, name := range e.admin.ListDisplay {
		if f, ok := e.model.FieldByName(name); ok {
			cols = append(cols, f)
		}
	}
	return cols
}

// formatCell renders a single struct field value as a display string. A relation
// field is an orm.FK[..] whose PK method yields the related primary key; scalars
// are formatted with fmt.Sprint.
func formatCell(v reflect.Value) string {
	if pk, ok := fkPK(v); ok {
		return fmt.Sprint(pk)
	}
	return fmt.Sprint(v.Interface())
}

// pkOf returns the primary-key value of the struct addressed by elem for model
// e, as an int64 (0 when the model has no primary key).
func pkOf(elem reflect.Value, e *entry) int64 {
	pk := e.model.PrimaryKey()
	if pk == nil {
		return 0
	}
	return elem.Field(pk.Index).Int()
}

// fkPK reports whether v is an orm.FK[..] value and, if so, returns its related
// primary key by calling its PK method via reflection.
func fkPK(v reflect.Value) (int64, bool) {
	m := v.MethodByName("PK")
	if !m.IsValid() {
		return 0, false
	}
	mt := m.Type()
	if mt.NumIn() != 0 || mt.NumOut() != 1 || mt.Out(0).Kind() != reflect.Int64 {
		return 0, false
	}
	return m.Call(nil)[0].Int(), true
}
