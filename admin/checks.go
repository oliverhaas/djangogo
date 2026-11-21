package admin

import (
	"fmt"

	"github.com/oliverhaas/djangogo/checks"
	"github.com/oliverhaas/djangogo/orm"
)

// adminObject is the object of a ModelAdmin check message. Django names the
// ModelAdmin class; a Go ModelAdmin is a value without a name, so the message
// names the model it administers.
type adminObject struct{ model *orm.Model }

// Label returns "ModelAdmin of <app>.<Model>".
func (o adminObject) Label() string { return "ModelAdmin of " + o.model.Label() }

// Check runs the checks of Django's ModelAdminChecks that apply to the Go
// ModelAdmin over every registered model and returns the problems, in the
// order Django checks them: FilterVertical and FilterHorizontal
// (admin.E013, admin.E019, admin.E020), then ListDisplay (admin.E108,
// admin.E109).
func (s *AdminSite) Check() []checks.Message {
	var out []checks.Message
	for _, e := range s.entries {
		obj := adminObject{model: e.model}
		for i, name := range e.admin.FilterVertical {
			out = append(out, checkFilterItem(e, obj, name, fmt.Sprintf("filter_vertical[%d]", i))...)
		}
		for i, name := range e.admin.FilterHorizontal {
			out = append(out, checkFilterItem(e, obj, name, fmt.Sprintf("filter_horizontal[%d]", i))...)
		}
		for i, name := range e.admin.ListDisplay {
			out = append(out, checkListDisplayItem(e, obj, name, fmt.Sprintf("list_display[%d]", i))...)
		}
	}
	return out
}

// checkFilterItem checks one entry of FilterVertical or FilterHorizontal,
// Django's _check_filter_item: the field exists and is a many-to-many field
// without an explicit through model.
func checkFilterItem(e *entry, obj adminObject, name, label string) []checks.Message {
	if f := manyToManyField(e.model, name); f != nil {
		if f.Rel != nil && !f.Rel.AutoCreated {
			return []checks.Message{checks.Error(fmt.Sprintf(
				"The value of '%s' cannot include the ManyToManyField '%s', because that field manually specifies a relationship model.",
				label, name), checks.Obj(obj), checks.ID("admin.E013"))}
		}
		return nil
	}
	_, isField := resolveModelField(e.model, name)
	info, isRelation := orm.ResolveLookupPath(e.model, name)
	// Django's get_field finds a reverse relation too, so a reverse or
	// many-to-many path fails the type check below, not this one.
	related := isRelation && info.Lookup == "" && (info.Reverse != nil || info.IsM2M)
	if !isField && !related {
		return []checks.Message{checks.Error(fmt.Sprintf(
			"The value of '%s' refers to '%s', which is not a field of '%s'.", label, name, e.model.Label()),
			checks.Obj(obj), checks.ID("admin.E019"))}
	}
	return []checks.Message{checks.Error(fmt.Sprintf("The value of '%s' must be a many-to-many field.", label),
		checks.Obj(obj), checks.ID("admin.E020"))}
}

// checkListDisplayItem checks one entry of ListDisplay, Django's
// _check_list_display_item: a DisplayColumns key or a column the changelist
// can show, and not a many-valued relation.
func checkListDisplayItem(e *entry, obj adminObject, name, label string) []checks.Message {
	if _, ok := e.admin.DisplayColumns[name]; ok {
		return nil
	}
	if manyValued(e.model, name) {
		return []checks.Message{checks.Error(fmt.Sprintf(
			"The value of '%s' must not be a many-to-many field or a reverse foreign key.", label),
			checks.Obj(obj), checks.ID("admin.E109"))}
	}
	if _, ok := resolveDisplayColumn(e, name); ok {
		return nil
	}
	return []checks.Message{checks.Error(fmt.Sprintf(
		"The value of '%s' refers to '%s', which is not a callable or attribute of 'ModelAdmin', or an attribute, method, or field on '%s'.",
		label, name, e.model.Label()), checks.Obj(obj), checks.ID("admin.E108"))}
}

// manyToManyField returns the forward many-to-many field of m with the Go
// name or attribute name name, or nil.
func manyToManyField(m *orm.Model, name string) *orm.Field {
	for _, f := range m.ManyToMany() {
		if f.Name == name || f.Attr == name {
			return f
		}
	}
	return nil
}

// manyValued reports whether name addresses a many-to-many relation or a
// reverse foreign key of m: a field, a reverse accessor such as "song_set",
// or a relation path.
func manyValued(m *orm.Model, name string) bool {
	if manyToManyField(m, name) != nil {
		return true
	}
	if rev, ok := m.ReverseRelation(name); ok && rev.Kind == orm.RelFK {
		return true
	}
	info, ok := orm.ResolveLookupPath(m, name)
	if !ok || info.Lookup != "" {
		return false
	}
	return info.IsM2M || info.Reverse != nil && info.Reverse.Kind == orm.RelFK
}
