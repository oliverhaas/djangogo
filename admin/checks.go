package admin

import (
	"fmt"
	"slices"
	"strings"

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
// order Django checks them: Fields (admin.E004, admin.E005, admin.E006,
// admin.E013), Fieldsets (admin.E008, admin.E011, admin.E012, admin.E013),
// ExcludeFields (admin.E015), FilterVertical and FilterHorizontal
// (admin.E013, admin.E019, admin.E020), ReadonlyFields (admin.E035), then
// ListDisplay (admin.E108, admin.E109).
func (s *AdminSite) Check() []checks.Message {
	var out []checks.Message
	for _, e := range s.entries {
		obj := adminObject{model: e.model}
		out = append(out, checkFields(e, obj)...)
		out = append(out, checkFieldsets(e, obj)...)
		if dups := duplicates(e.model, e.admin.ExcludeFields); len(dups) > 0 {
			out = append(out, checks.Error("The value of 'exclude' contains duplicate field(s).",
				checks.Hint(duplicatesHint(dups)), checks.Obj(obj), checks.ID("admin.E015")))
		}
		for i, name := range e.admin.FilterVertical {
			out = append(out, checkFilterItem(e, obj, name, fmt.Sprintf("filter_vertical[%d]", i))...)
		}
		for i, name := range e.admin.FilterHorizontal {
			out = append(out, checkFilterItem(e, obj, name, fmt.Sprintf("filter_horizontal[%d]", i))...)
		}
		for i, name := range e.admin.ReadonlyFields {
			out = append(out, checkReadonlyItem(e, obj, name, fmt.Sprintf("readonly_fields[%d]", i))...)
		}
		for i, name := range e.admin.ListDisplay {
			out = append(out, checkListDisplayItem(e, obj, name, fmt.Sprintf("list_display[%d]", i))...)
		}
	}
	return out
}

// checkFields checks Fields, Django's _check_fields: not set together with
// Fieldsets, no field twice, and each entry a field spec.
func checkFields(e *entry, obj adminObject) []checks.Message {
	if len(e.admin.Fields) == 0 {
		return nil
	}
	if len(e.admin.Fieldsets) > 0 {
		return []checks.Message{checks.Error("Both 'fieldsets' and 'fields' are specified.",
			checks.Obj(obj), checks.ID("admin.E005"))}
	}
	if dups := duplicates(e.model, flattenFields(e.admin.Fields)); len(dups) > 0 {
		return []checks.Message{checks.Error("The value of 'fields' contains duplicate field(s).",
			checks.Hint(duplicatesHint(dups)), checks.Obj(obj), checks.ID("admin.E006"))}
	}
	var out []checks.Message
	for i, entry := range e.admin.Fields {
		out = append(out, checkFieldSpec(e, obj, entry, "fields", fmt.Sprintf("fields[%d]", i), "admin.E004")...)
	}
	return out
}

// checkFieldsets checks Fieldsets, Django's _check_fieldsets: each fieldset
// has fields, no field is in two places, and each entry is a field spec. A
// Fieldset with nil Fields is Django's fieldset without the "fields" key.
func checkFieldsets(e *entry, obj adminObject) []checks.Message {
	var out []checks.Message
	var seen []string
	for i, fs := range e.admin.Fieldsets {
		label := fmt.Sprintf("fieldsets[%d]", i)
		if fs.Fields == nil {
			out = append(out, checks.Error(fmt.Sprintf("The value of '%s[1]' must contain the key 'fields'.", label),
				checks.Obj(obj), checks.ID("admin.E011")))
			continue
		}
		names := formNames(e.model, flattenFields(fs.Fields))
		seen = append(seen, names...)
		var dups []string
		for _, name := range duplicates(e.model, seen) {
			if slices.Contains(names, name) {
				dups = append(dups, name)
			}
		}
		if len(dups) > 0 {
			out = append(out, checks.Error(fmt.Sprintf("There are duplicate field(s) in '%s[1]'.", label),
				checks.Hint(duplicatesHint(dups)), checks.Obj(obj), checks.ID("admin.E012")))
			continue
		}
		for j, entry := range fs.Fields {
			out = append(out, checkFieldSpec(e, obj, entry, label+`[1]["fields"]`,
				fmt.Sprintf("%s[1]['fields'][%d]", label, j), "admin.E008")...)
		}
	}
	return out
}

// checkFieldSpec checks one entry of Fields or of the Fields of a Fieldset,
// Django's _check_field_spec: a name, or a []string of names on one line,
// each checked under label. Any other type is reported under entryLabel
// with id; Django's types leave no such entry to check.
func checkFieldSpec(e *entry, obj adminObject, entry any, label, entryLabel, id string) []checks.Message {
	switch v := entry.(type) {
	case string:
		return checkFieldSpecItem(e, obj, v, label)
	case []string:
		var out []checks.Message
		for i, name := range v {
			out = append(out, checkFieldSpecItem(e, obj, name, fmt.Sprintf("%s[%d]", label, i))...)
		}
		return out
	}
	return []checks.Message{checks.Error(fmt.Sprintf("The value of '%s' must be a field name or a []string.", entryLabel),
		checks.Obj(obj), checks.ID(id))}
}

// checkFieldSpecItem checks one field name of a layout, Django's
// _check_field_spec_item: a read-only field or a name that is no model field
// passes, and a many-to-many field must not have an explicit through model.
func checkFieldSpecItem(e *entry, obj adminObject, name, label string) []checks.Message {
	if slices.Contains(formNames(e.model, e.admin.ReadonlyFields), formName(e.model, name)) {
		return nil
	}
	if f := manyToManyField(e.model, name); f != nil && f.Rel != nil && !f.Rel.AutoCreated {
		return []checks.Message{checks.Error(fmt.Sprintf(
			"The value of '%s' cannot include the ManyToManyField '%s', because that field manually specifies a relationship model.",
			label, name), checks.Obj(obj), checks.ID("admin.E013"))}
	}
	return nil
}

// checkReadonlyItem checks one entry of ReadonlyFields, Django's
// _check_readonly_fields_item: a DisplayColumns key or a concrete or
// many-to-many field of the model, the names a read-only field can show.
func checkReadonlyItem(e *entry, obj adminObject, name, label string) []checks.Message {
	if _, ok := e.admin.DisplayColumns[name]; ok || readonlyModelField(e.model, name) != nil {
		return nil
	}
	return []checks.Message{checks.Error(fmt.Sprintf(
		"The value of '%s' refers to '%s', which is not a callable, an attribute of 'ModelAdmin', or an attribute of '%s'.",
		label, name, e.model.Label()), checks.Obj(obj), checks.ID("admin.E035"))}
}

// duplicates returns the form names that occur more than once in names, in
// the order of their first occurrence, as Django's Counter lists them. Two
// spellings of one field, its Go name and its attribute name, count as the
// same field.
func duplicates(m *orm.Model, names []string) []string {
	counts := make(map[string]int, len(names))
	var order []string
	for _, n := range formNames(m, names) {
		if counts[n] == 0 {
			order = append(order, n)
		}
		counts[n]++
	}
	var out []string
	for _, n := range order {
		if counts[n] > 1 {
			out = append(out, n)
		}
	}
	return out
}

// duplicatesHint returns Django's hint "Remove duplicates of 'a', 'b'.".
func duplicatesHint(names []string) string {
	quoted := make([]string, len(names))
	for i, n := range names {
		quoted[i] = "'" + n + "'"
	}
	return "Remove duplicates of " + strings.Join(quoted, ", ") + "."
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
