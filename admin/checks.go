package admin

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/oliverhaas/djangogo/checks"
	"github.com/oliverhaas/djangogo/orm"
)

// adminObject is the object of a ModelAdmin or InlineModelAdmin check
// message. Django names the admin class; a Go admin is a value without a
// name, so the message names its kind, "ModelAdmin" or "InlineModelAdmin",
// and the model it administers.
type adminObject struct {
	kind  string
	model *orm.Model
}

// Label returns "<kind> of <app>.<Model>".
func (o adminObject) Label() string { return o.kind + " of " + o.model.Label() }

// Check runs the checks of Django's ModelAdminChecks that apply to the Go
// ModelAdmin over every registered model and returns the problems, in the
// order Django checks them: AutocompleteFields (admin.E037, admin.E038,
// admin.E039, admin.E040), RawIDFields (admin.E002, admin.E003), Fields
// (admin.E004, admin.E005, admin.E006, admin.E013), Fieldsets (admin.E008,
// admin.E011, admin.E012, admin.E013), ExcludeFields (admin.E015),
// FilterVertical and FilterHorizontal (admin.E013, admin.E019, admin.E020),
// RadioFields (admin.E022, admin.E023, admin.E024), PrepopulatedFields
// (admin.E027, admin.E028, admin.E030), Ordering (admin.E032, admin.E033),
// ReadonlyFields (admin.E035), the inlines (see checkInlines), ListDisplay
// (admin.E108, admin.E109), ListDisplayLinks (admin.E111), ListFilter
// (admin.E113, admin.E116), ListEditable (admin.E121, admin.E122,
// admin.E123, admin.E124, admin.E125), DateHierarchy (admin.E127,
// admin.E128), then Actions (admin.E130).
func (s *AdminSite) Check() []checks.Message {
	var out []checks.Message
	for _, e := range s.entries {
		obj := adminObject{kind: "ModelAdmin", model: e.model}
		out = append(out, s.checkBase(e, obj)...)
		out = append(out, s.checkInlines(e, obj)...)
		for i, name := range e.admin.ListDisplay {
			out = append(out, checkListDisplayItem(e, obj, name, fmt.Sprintf("list_display[%d]", i))...)
		}
		out = append(out, checkListDisplayLinks(e, obj)...)
		out = append(out, checkListFilter(e, obj)...)
		for i, name := range e.admin.ListEditable {
			out = append(out, checkListEditableItem(e, obj, name, fmt.Sprintf("list_editable[%d]", i))...)
		}
		out = append(out, checkDateHierarchy(e, obj)...)
		out = append(out, checkActions(e, obj)...)
	}
	return out
}

// checkBase runs the checks of Django's BaseModelAdminChecks, which a
// ModelAdmin and an InlineModelAdmin share, over the admin of e.
func (s *AdminSite) checkBase(e *entry, obj adminObject) []checks.Message {
	var out []checks.Message
	for i, name := range e.admin.AutocompleteFields {
		out = append(out, s.checkAutocompleteItem(e, obj, name, fmt.Sprintf("autocomplete_fields[%d]", i))...)
	}
	for i, name := range e.admin.RawIDFields {
		out = append(out, checkRawIDItem(e, obj, name, fmt.Sprintf("raw_id_fields[%d]", i))...)
	}
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
	out = append(out, checkRadioFields(e, obj)...)
	out = append(out, checkPrepopulated(e, obj)...)
	out = append(out, checkOrdering(e, obj)...)
	for i, name := range e.admin.ReadonlyFields {
		out = append(out, checkReadonlyItem(e, obj, name, fmt.Sprintf("readonly_fields[%d]", i))...)
	}
	return out
}

// checkAutocompleteItem checks one entry of AutocompleteFields, Django's
// _check_autocomplete_fields_item: the field exists, is a foreign key or a
// many-to-many field, and its related model has an admin on s with search
// fields. Django's messages name the ModelAdmin classes; a Go ModelAdmin has
// no class name, so they name the type and the model.
func (s *AdminSite) checkAutocompleteItem(e *entry, obj adminObject, name, label string) []checks.Message {
	f, exists := relationOrField(e.model, name)
	if !exists {
		return []checks.Message{checks.Error(fmt.Sprintf(
			"The value of '%s' refers to '%s', which is not a field of '%s'.", label, name, e.model.Label()),
			checks.Obj(obj), checks.ID("admin.E037"))}
	}
	if f == nil || f.Rel == nil || f.Rel.Target == nil {
		return []checks.Message{checks.Error(fmt.Sprintf(
			"The value of '%s' must be a foreign key or a many-to-many field.", label),
			checks.Obj(obj), checks.ID("admin.E038"))}
	}
	related := s.entryByModel(f.Rel.Target)
	if related == nil {
		return []checks.Message{checks.Error(fmt.Sprintf(
			"An admin for model %q has to be registered to be referenced by ModelAdmin.autocomplete_fields.",
			f.Rel.Target.Name()), checks.Obj(obj), checks.ID("admin.E039"))}
	}
	if len(related.admin.SearchFields) == 0 && related.admin.GetSearchFields == nil {
		return []checks.Message{checks.Error(fmt.Sprintf(
			"ModelAdmin of %s must define \"search_fields\", because it's referenced by ModelAdmin.autocomplete_fields.",
			f.Rel.Target.Label()), checks.Obj(obj), checks.ID("admin.E040"))}
	}
	return nil
}

// checkRadioFields checks RadioFields, Django's _check_radio_fields, in the
// order of the sorted keys: each key is a foreign key or a field with
// choices, and each value is Horizontal or Vertical.
func checkRadioFields(e *entry, obj adminObject) []checks.Message {
	var out []checks.Message
	for _, name := range slices.Sorted(maps.Keys(e.admin.RadioFields)) {
		f, exists := relationOrField(e.model, name)
		switch {
		case !exists:
			out = append(out, checks.Error(fmt.Sprintf(
				"The value of 'radio_fields' refers to '%s', which is not a field of '%s'.",
				name, e.model.Label()), checks.Obj(obj), checks.ID("admin.E022")))
		case f == nil || !f.HasChoices && (f.Rel == nil || f.Rel.Kind == orm.RelM2M):
			out = append(out, checks.Error(fmt.Sprintf(
				"The value of 'radio_fields' refers to '%s', which is not an instance of ForeignKey, "+
					"and does not have a 'choices' definition.", name),
				checks.Obj(obj), checks.ID("admin.E023")))
		}
		if style := e.admin.RadioFields[name]; style != Horizontal && style != Vertical {
			out = append(out, checks.Error(fmt.Sprintf(
				"The value of 'radio_fields[\"%s\"]' must be either admin.HORIZONTAL or admin.VERTICAL.", name),
				checks.Obj(obj), checks.ID("admin.E024")))
		}
	}
	return out
}

// checkPrepopulated checks PrepopulatedFields, Django's
// _check_prepopulated_fields, in the order of the sorted keys: each key is a
// field that is not a date and time, a foreign key or a many-to-many field,
// and each value names fields.
func checkPrepopulated(e *entry, obj adminObject) []checks.Message {
	var out []checks.Message
	fields := e.admin.PrepopulatedFields
	for _, name := range slices.Sorted(maps.Keys(fields)) {
		f, exists := relationOrField(e.model, name)
		switch {
		case !exists:
			out = append(out, checks.Error(fmt.Sprintf(
				"The value of 'prepopulated_fields' refers to '%s', which is not a field of '%s'.",
				name, e.model.Label()), checks.Obj(obj), checks.ID("admin.E027")))
		case f != nil && (f.Kind == orm.KindDateTime || f.Rel != nil):
			out = append(out, checks.Error(fmt.Sprintf(
				"The value of 'prepopulated_fields' refers to '%s', which must not be a DateTimeField, "+
					"a ForeignKey, a OneToOneField, or a ManyToManyField.", name),
				checks.Obj(obj), checks.ID("admin.E028")))
		}
		for i, dep := range fields[name] {
			if _, exists := relationOrField(e.model, dep); !exists {
				out = append(out, checks.Error(fmt.Sprintf(
					"The value of 'prepopulated_fields[\"%s\"][%d]' refers to '%s', which is not a field of '%s'.",
					name, i, dep, e.model.Label()), checks.Obj(obj), checks.ID("admin.E030")))
			}
		}
	}
	return out
}

// checkRawIDItem checks one entry of RawIDFields, Django's
// _check_raw_id_fields_item: the field exists under its name, not the
// column name of a foreign key, and is a foreign key or a many-to-many field.
func checkRawIDItem(e *entry, obj adminObject, name, label string) []checks.Message {
	f, exists := relationOrField(e.model, name)
	if !exists {
		return []checks.Message{checks.Error(fmt.Sprintf(
			"The value of '%s' refers to '%s', which is not a field of '%s'.", label, name, e.model.Label()),
			checks.Obj(obj), checks.ID("admin.E002"))}
	}
	if f == nil || f.Rel == nil || f.Rel.Target == nil {
		return []checks.Message{checks.Error(fmt.Sprintf(
			"The value of '%s' must be a foreign key or a many-to-many field.", label),
			checks.Obj(obj), checks.ID("admin.E003"))}
	}
	return nil
}

// relationOrField looks up name on m as Django's _meta.get_field does for
// the raw-id and autocomplete checks: a field or many-to-many field by its Go
// name or attribute name, else a reverse relation, which exists but is no
// field (f is nil).
func relationOrField(m *orm.Model, name string) (f *orm.Field, exists bool) {
	if f := manyToManyField(m, name); f != nil {
		return f, true
	}
	if f, ok := m.FieldByName(name); ok {
		return f, true
	}
	if f, ok := m.FieldByAttr(name); ok {
		return f, true
	}
	if _, ok := m.ReverseRelation(name); ok {
		return nil, true
	}
	return nil, false
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
// _check_readonly_fields_item: a DisplayColumns key, a concrete or
// many-to-many field of the model, or a model method, the names a read-only
// field can show.
func checkReadonlyItem(e *entry, obj adminObject, name, label string) []checks.Message {
	if _, ok := e.admin.DisplayColumns[name]; ok || readonlyModelField(e.model, name) != nil {
		return nil
	}
	if _, ok := findModelMethod(e.model, name); ok {
		return nil
	}
	return []checks.Message{checks.Error(fmt.Sprintf(
		"The value of '%s' refers to '%s', which is not a callable, an attribute of '%s', or an attribute of '%s'.",
		label, name, obj.kind, e.model.Label()), checks.Obj(obj), checks.ID("admin.E035"))}
}

// checkOrdering checks Ordering, Django's _check_ordering: the random marker
// "?" stands alone, and each name is "pk" or a field of the model, with or
// without a leading "-". Django skips the names that span relations.
func checkOrdering(e *entry, obj adminObject) []checks.Message {
	ordering := e.admin.Ordering
	var out []checks.Message
	for i, name := range ordering {
		if name == "?" {
			if len(ordering) != 1 {
				out = append(out, checks.Error(
					"The value of 'ordering' has the random ordering marker '?', but contains other fields as well.",
					checks.Hint(`Either remove the "?", or remove the other fields.`),
					checks.Obj(obj), checks.ID("admin.E032")))
			}
			continue
		}
		field := strings.TrimPrefix(name, "-")
		if strings.Contains(field, "__") || field == "pk" {
			continue
		}
		if _, ok := relationOrField(e.model, field); !ok {
			out = append(out, checks.Error(fmt.Sprintf(
				"The value of 'ordering[%d]' refers to '%s', which is not a field of '%s'.", i, field, e.model.Label()),
				checks.Obj(obj), checks.ID("admin.E033")))
		}
	}
	return out
}

// checkListDisplayLinks checks ListDisplayLinks, Django's
// _check_list_display_links: each entry is a column of ListDisplay. Django
// skips the check when the admin overrides get_list_display, as when
// GetListDisplay is set.
func checkListDisplayLinks(e *entry, obj adminObject) []checks.Message {
	if e.admin.GetListDisplay != nil {
		return nil
	}
	display := e.admin.ListDisplay
	if len(display) == 0 {
		display = []string{"__str__"}
	}
	var out []checks.Message
	for i, name := range e.admin.ListDisplayLinks {
		link := normalizeFieldPath(e.model, name)
		if !slices.ContainsFunc(display, func(d string) bool { return normalizeFieldPath(e.model, d) == link }) {
			out = append(out, checks.Error(fmt.Sprintf(
				"The value of 'list_display_links[%d]' refers to '%s', which is not defined in 'list_display'.", i, name),
				checks.Obj(obj), checks.ID("admin.E111")))
		}
	}
	return out
}

// checkListFilter checks ListFilter, Django's _check_list_filter: each entry
// is a field name, a FieldFilter or a SimpleListFilter (admin.E113), and a
// field name names a field of the model or of a related model (admin.E116).
// Django checks the field of a (field, FieldListFilter) pair only on the
// changelist; the field of a FieldFilter is checked here.
func checkListFilter(e *entry, obj adminObject) []checks.Message {
	var out []checks.Message
	for i, item := range e.admin.ListFilter {
		var name string
		switch v := item.(type) {
		case SimpleListFilter, *SimpleListFilter:
			continue
		case string:
			name = v
		case FieldFilter:
			name = v.Field
		default:
			out = append(out, checks.Error(fmt.Sprintf(
				"The value of 'list_filter[%d]' must be a field name, a FieldFilter, or a SimpleListFilter.", i),
				checks.Obj(obj), checks.ID("admin.E113")))
			continue
		}
		info, ok := orm.ResolveLookupPath(e.model, normalizeFieldPath(e.model, name))
		if !ok || info.Lookup != "" {
			out = append(out, checks.Error(fmt.Sprintf(
				"The value of 'list_filter[%d]' refers to '%s', which does not refer to a Field.", i, name),
				checks.Obj(obj), checks.ID("admin.E116")))
		}
	}
	return out
}

// checkActions checks Actions, Django's _check_actions: no two actions of
// the ModelAdmin share a name. An action of the ModelAdmin replaces the site
// action of the same name, and AddAction keeps the site's names unique, so
// only the ModelAdmin's own actions can clash.
func checkActions(e *entry, obj adminObject) []checks.Message {
	counts := make(map[string]int, len(e.admin.Actions))
	var names []string
	for _, a := range e.admin.Actions {
		if counts[a.Name] == 0 {
			names = append(names, a.Name)
		}
		counts[a.Name]++
	}
	var out []checks.Message
	for _, name := range names {
		if counts[name] > 1 {
			out = append(out, checks.Error(fmt.Sprintf(
				"Name attributes of actions defined in ModelAdmin must be unique. Name '%s' is not unique.", name),
				checks.Obj(obj), checks.ID("admin.E130")))
		}
	}
	return out
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

// checkListEditableItem checks one entry of ListEditable, Django's
// _check_list_editable_item: a field of the model that ListDisplay shows,
// that is not a link, and that the admin can edit. The Go name and the
// attribute name of a field name the same field.
func checkListEditableItem(e *entry, obj adminObject, name, label string) []checks.Message {
	f, ok := e.model.FieldByName(name)
	if !ok {
		f, ok = e.model.FieldByAttr(name)
	}
	if !ok {
		f = manyToManyField(e.model, name)
	}
	if f == nil {
		return []checks.Message{checks.Error(fmt.Sprintf(
			"The value of '%s' refers to '%s', which is not a field of '%s'.", label, name, e.model.Label()),
			checks.Obj(obj), checks.ID("admin.E121"))}
	}
	display := formNames(e.model, e.admin.ListDisplay)
	if !slices.Contains(display, f.Attr) {
		return []checks.Message{checks.Error(fmt.Sprintf(
			"The value of '%s' refers to '%s', which is not contained in 'list_display'.", label, name),
			checks.Obj(obj), checks.ID("admin.E122"))}
	}
	links := e.admin.ListDisplayLinks
	if len(links) > 0 && slices.Contains(formNames(e.model, links), f.Attr) {
		return []checks.Message{checks.Error(fmt.Sprintf(
			"The value of '%s' cannot be in both 'list_editable' and 'list_display_links'.", name),
			checks.Obj(obj), checks.ID("admin.E123"))}
	}
	// A nil ListDisplayLinks links the first column, Django's empty
	// list_display_links; an empty one links none, Django's None.
	if display[0] == f.Attr && links == nil {
		return []checks.Message{checks.Error(fmt.Sprintf(
			"The value of '%s' refers to the first field in 'list_display' ('%s'), which cannot be used unless 'list_display_links' is set.",
			label, e.admin.ListDisplay[0]), checks.Obj(obj), checks.ID("admin.E124"))}
	}
	if !f.Editable || f.PrimaryKey {
		return []checks.Message{checks.Error(fmt.Sprintf(
			"The value of '%s' refers to '%s', which is not editable through the admin.", label, name),
			checks.Obj(obj), checks.ID("admin.E125"))}
	}
	return nil
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
