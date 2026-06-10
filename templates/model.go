package templates

import (
	"reflect"

	"github.com/oliverhaas/djangogo/orm"
)

// modelLabelKey is the ModelMap key under which the __str__ label is stored. It
// mirrors Python's dunder so it never collides with a real snake_case column.
const modelLabelKey = "__str__"

// ModelMap is the template-facing view of a model instance: a map of snake_case
// column names to values. pongo2 resolves map keys by exact string lookup, so
// {{ post.title }} works where a struct field (resolved case-sensitively as
// Title) would not. ModelMap also implements fmt.Stringer, so a bare {{ post }}
// renders the model's __str__ label.
type ModelMap map[string]any

// String returns the model's label (its __str__), satisfying fmt.Stringer so
// pongo2 renders {{ post }} as the label.
func (m ModelMap) String() string {
	if s, ok := m[modelLabelKey].(string); ok {
		return s
	}
	return ""
}

// ModelContext builds a ModelMap exposing instance's scalar fields under their
// snake_case column names, plus "id"/"pk" aliases for the primary key and the
// __str__ label. It mirrors how a Django template sees a model instance. The
// instance may be a value or a pointer. Relation fields are skipped in v1: only
// their integer FK column would be reachable, which is more confusing than
// useful in a template.
func ModelContext(m *orm.Model, instance any) ModelMap {
	out := make(ModelMap)
	v := reflect.ValueOf(instance)
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			out[modelLabelKey] = orm.Label(m, instance)
			return out
		}
		v = v.Elem()
	}
	if v.Kind() == reflect.Struct {
		for _, f := range m.Fields() {
			if f.Rel != nil {
				continue
			}
			val := v.Field(f.Index).Interface()
			out[f.Column] = val
			if f.PrimaryKey {
				out["pk"] = val
				out["id"] = val
			}
		}
	}
	out[modelLabelKey] = orm.Label(m, instance)
	return out
}
