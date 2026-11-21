package admin

import (
	"context"
	"fmt"
	"html"
	"html/template"
	"net/http"
	"reflect"
	"slices"
	"strings"

	"github.com/oliverhaas/djangogo/forms"
	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/templates"
)

// formName returns the attribute name of the model field that name refers to
// by Go name or attribute name, the name of its form field, or name itself
// when it names no field.
func formName(m *orm.Model, name string) string {
	if f, ok := m.FieldByName(name); ok {
		return f.Attr
	}
	if f, ok := m.FieldByAttr(name); ok {
		return f.Attr
	}
	if f := manyToManyField(m, name); f != nil {
		return f.Attr
	}
	return name
}

// formNames returns formName of each of names.
func formNames(m *orm.Model, names []string) []string {
	out := make([]string, len(names))
	for i, n := range names {
		out[i] = formName(m, n)
	}
	return out
}

// readonlyFields returns the read-only field names for r and obj as form
// field names, Django's get_readonly_fields.
func (e *entry) readonlyFields(r *http.Request, obj any) []string {
	names := e.admin.ReadonlyFields
	if e.admin.GetReadonlyFields != nil {
		names = e.admin.GetReadonlyFields(r, obj)
	}
	return formNames(e.model, names)
}

// prepopulatedFields returns the prepopulated fields for r and obj, Django's
// get_prepopulated_fields.
func (e *entry) prepopulatedFields(r *http.Request, obj any) map[string][]string {
	if e.admin.GetPrepopulatedFields != nil {
		return e.admin.GetPrepopulatedFields(r, obj)
	}
	return e.admin.PrepopulatedFields
}

// lineNames returns the field names of one entry of a layout: a name, or a
// []string of names on one line. ok is false for any other type.
func lineNames(entry any) (names []string, ok bool) {
	switch v := entry.(type) {
	case string:
		return []string{v}, true
	case []string:
		return v, true
	}
	return nil, false
}

// flattenFields returns the names of a layout in order, Django's flatten.
func flattenFields(layout []any) []string {
	var out []string
	for _, entry := range layout {
		names, _ := lineNames(entry)
		out = append(out, names...)
	}
	return out
}

// flattenFieldsets returns the names of every fieldset in order, Django's
// flatten_fieldsets.
func flattenFieldsets(sets []Fieldset) []string {
	out := make([]string, 0, len(sets))
	for _, fs := range sets {
		out = append(out, flattenFields(fs.Fields)...)
	}
	return out
}

// layoutFields returns the layout of the form for r and obj, Django's
// get_fields: GetFields, else Fields, else the fields of the model form in
// model order followed by the read-only fields.
func (s *AdminSite) layoutFields(r *http.Request, e *entry, obj any) []any {
	if e.admin.GetFields != nil {
		return e.admin.GetFields(r, obj)
	}
	if len(e.admin.Fields) > 0 {
		return e.admin.Fields
	}
	readonly := e.readonlyFields(r, obj)
	opts := append([]forms.ModelFormOption{forms.WithExclude(slices.Concat(s.excluded(r, e, obj), readonly)...)},
		e.formOptions(r, obj)...)
	base := forms.FromModel(e.model, opts...)
	out := make([]any, 0, len(base.FieldNames())+len(readonly))
	for _, name := range base.FieldNames() {
		out = append(out, name)
	}
	for _, name := range readonly {
		out = append(out, name)
	}
	return out
}

// formOptions returns the options of the model form for r and obj, nil on
// the add page: GetFormOptions, else FormOptions.
func (e *entry) formOptions(r *http.Request, obj any) []forms.ModelFormOption {
	if e.admin.GetFormOptions != nil {
		return e.admin.GetFormOptions(r, obj)
	}
	return e.admin.FormOptions
}

// fieldsets returns the fieldsets of the form for r and obj, Django's
// get_fieldsets: GetFieldsets, else the fieldsets of the add page of the
// admin of users, else Fieldsets, else one fieldset without a name that
// holds layoutFields.
func (s *AdminSite) fieldsets(r *http.Request, e *entry, obj any) []Fieldset {
	if e.admin.GetFieldsets != nil {
		return e.admin.GetFieldsets(r, obj)
	}
	if obj == nil && e.admin.hooks != nil && e.admin.hooks.addFieldsets != nil {
		return e.admin.hooks.addFieldsets
	}
	if len(e.admin.Fieldsets) > 0 {
		return e.admin.Fieldsets
	}
	return []Fieldset{{Fields: s.layoutFields(r, e, obj)}}
}

// adminForm is the form of the add and change views with what the template
// needs to lay it out, Django's helpers.AdminForm.
type adminForm struct {
	form     *forms.Form
	sets     []Fieldset
	readonly []string
	// obj is the object the read-only fields show: the edited one, or a new
	// one with the field defaults on the add page.
	obj any
	// opts are the options of the model form from the admin, whose labels
	// and help texts the read-only fields show.
	opts []forms.ModelFormOption
	// skip names a field whose own line the fieldsets leave out: the
	// foreign key to the parent in the form of an inline, Django's
	// InlineFieldset.
	skip string
}

// fieldsetContext returns the template data of the fieldsets of af, one map
// per fieldset in the shape that admin/includes/fieldset.html reads.
func (s *AdminSite) fieldsetContext(ctx context.Context, e *entry, af *adminForm) []map[string]any {
	out := make([]map[string]any, 0, len(af.sets))
	for _, fs := range af.sets {
		lines := make([]map[string]any, 0, len(fs.Fields))
		// Django's Fieldset.is_collapsible looks the fields with errors up
		// in the entries of the fieldset, so a field on a shared line does
		// not keep the fieldset open.
		collapsible := slices.Contains(fs.Classes, "collapse")
		for _, entry := range fs.Fields {
			if name, single := entry.(string); single && af.form.IsBound() && af.form.Errors().Has(formName(e.model, name)) {
				collapsible = false
			}
			if name, single := entry.(string); single && af.skip != "" && formName(e.model, name) == af.skip {
				continue
			}
			names, _ := lineNames(entry)
			if line := s.lineContext(ctx, e, af, formNames(e.model, names)); line != nil {
				lines = append(lines, line)
			}
		}
		out = append(out, map[string]any{
			"name":           fs.Name,
			"classes":        strings.Join(fs.Classes, " "),
			"description":    templates.MarkSafe(fs.Description),
			"is_collapsible": collapsible,
			"lines":          lines,
		})
	}
	return out
}

// lineContext returns the template data of one line of a fieldset, Django's
// helpers.Fieldline, or nil when none of names is on the form.
func (s *AdminSite) lineContext(ctx context.Context, e *entry, af *adminForm, names []string) map[string]any {
	items := make([]map[string]any, 0, len(names))
	var errs []string
	hasVisible := false
	for i, name := range names {
		bf := af.form.Field(name)
		if slices.Contains(af.readonly, name) {
			item := s.readonlyFieldContext(ctx, e, af, name, i == 0)
			// A read-only field that is still on the form, as on a page
			// that the user may only view, keeps the hidden widget of the
			// form field, Django's AdminReadonlyField.is_hidden.
			hidden := bf != nil && bf.IsHidden()
			item["is_hidden"] = hidden
			if !hidden {
				hasVisible = true
			}
			items = append(items, item)
			continue
		}
		if bf == nil {
			continue
		}
		if !bf.IsHidden() {
			hasVisible = true
		}
		if ul := bf.Errors().AsUL(); ul != "" {
			errs = append(errs, ul)
		}
		items = append(items, adminFieldContext(bf, i == 0))
	}
	if len(items) == 0 {
		return nil
	}
	lineErrors := strings.Trim(strings.Join(errs, "\n"), "\n")
	multi := len(names) != 1
	for _, item := range items {
		item["errors_class"] = !item["is_readonly"].(bool) && item["has_errors"].(bool) || !multi && lineErrors != ""
	}
	return map[string]any{
		"multi":             multi,
		"has_visible_field": hasVisible,
		"errors":            templates.MarkSafe(lineErrors),
		"items":             items,
	}
}

// adminFieldContext returns the template data of an editable field, Django's
// helpers.AdminField.
func adminFieldContext(bf *forms.BoundField, first bool) map[string]any {
	_, checkbox := bf.Field().Base().Widget.(forms.CheckboxInput)
	var classes []string
	if checkbox {
		classes = append(classes, "vCheckboxLabel")
	}
	if bf.Field().Base().Required {
		classes = append(classes, "required")
	}
	if !first {
		classes = append(classes, "inline")
	}
	var attrs forms.Attrs
	if len(classes) > 0 {
		attrs = forms.Attrs{"class": strings.Join(classes, " ")}
	}
	var suffix *string
	if checkbox {
		empty := ""
		suffix = &empty
	}
	tag := "label"
	if bf.UseFieldset() {
		tag = "legend"
	}
	return map[string]any{
		"name":         bf.Name,
		"html":         templates.MarkSafe(bf.String()),
		"label_tag":    templates.MarkSafe(bf.LabelTagWith("", attrs, suffix, tag)),
		"help_text":    templates.MarkSafe(bf.HelpText()),
		"id_for_label": bf.IDForLabel(),
		"is_hidden":    bf.IsHidden(),
		"is_checkbox":  checkbox,
		"is_fieldset":  bf.UseFieldset(),
		"is_readonly":  false,
		"has_errors":   bf.Errors().Len() > 0,
		"errors":       templates.MarkSafe(bf.Errors().AsUL()),
	}
}

// readonlyFieldContext returns the template data of a read-only field,
// Django's helpers.AdminReadonlyField: a model field or a DisplayColumns key.
func (s *AdminSite) readonlyFieldContext(ctx context.Context, e *entry, af *adminForm, name string, first bool) map[string]any {
	obj := af.obj
	f := readonlyModelField(e.model, name)
	label, help := readonlyLabel(e, af.opts, f, name)
	class := ""
	if !first {
		class = ` class="inline"`
	}
	return map[string]any{
		"name":         name,
		"label_tag":    templates.MarkSafe("<label" + class + ">" + html.EscapeString(capFirst(label)) + ":</label>"),
		"help_text":    templates.MarkSafe(help),
		"id_for_label": "",
		"is_hidden":    false,
		"is_checkbox":  false,
		"is_fieldset":  false,
		"is_readonly":  true,
		"has_errors":   false,
		"errors":       "",
		"contents":     templates.MarkSafe(s.readonlyContents(ctx, e, obj, f, name)),
	}
}

// readonlyLabel returns the label and the help text of the read-only field
// name with the model field f, which may be nil. The WithLabel and
// WithHelpText options of the form come first, as Django reads
// ModelForm.Meta.labels and help_texts in AdminReadonlyField and
// InlineAdminFormSet.fields; then the verbose name and the help text of the
// model field, or the description of a DisplayColumn.
func readonlyLabel(e *entry, opts []forms.ModelFormOption, f *orm.Field, name string) (label, help string) {
	label = prettyName(name)
	if f != nil {
		label, help = fieldVerboseName(f), f.HelpText
	} else if col, ok := e.admin.DisplayColumns[name]; ok && col.Description != "" {
		label = col.Description
	}
	labels, helpTexts := forms.ModelFormLabels(opts...)
	keys := []string{name}
	if f != nil && f.Name != name {
		keys = append(keys, f.Name)
	}
	for _, key := range keys {
		if v := labels[key]; v != "" {
			label = v
			break
		}
	}
	for _, key := range keys {
		if v := helpTexts[key]; v != "" {
			help = v
			break
		}
	}
	return label, help
}

// readonlyModelField returns the concrete or many-to-many field of m that name
// refers to by Go name or attribute name, or nil.
func readonlyModelField(m *orm.Model, name string) *orm.Field {
	if f, ok := m.FieldByName(name); ok {
		return f
	}
	if f, ok := m.FieldByAttr(name); ok {
		return f
	}
	return manyToManyField(m, name)
}

// readonlyContents returns the HTML of the value of a read-only field,
// Django's AdminReadonlyField.contents: the related objects of a many-to-many
// field separated by commas, a foreign key as a link to the change page of
// the related object when its model has an admin on the site, the value of
// a DisplayColumn, and any other field as the changelist shows it. Line
// breaks become <br>.
func (s *AdminSite) readonlyContents(ctx context.Context, e *entry, obj any, f *orm.Field, name string) string {
	empty := e.emptyValue(s)
	switch {
	case f != nil && f.Rel != nil && f.Rel.Kind == orm.RelM2M:
		if !hasPK(e, obj) {
			// An unsaved object has no related rows; Django's manager
			// raises ValueError, which shows the empty value.
			return html.EscapeString(empty)
		}
		return linebreaksbr(html.EscapeString(s.manyLabels(ctx, obj, f)))
	case f != nil && f.Rel != nil:
		return s.readonlyRelated(ctx, reflect.ValueOf(obj).Elem().Field(f.Index), f, empty)
	case f != nil:
		out, _ := formatFieldHTML(e.model, obj, f, reflect.ValueOf(obj).Elem().Field(f.Index), empty, false)
		return linebreaksbr(out)
	}
	col, ok := e.admin.DisplayColumns[name]
	if !ok || col.Value == nil {
		return html.EscapeString(empty)
	}
	value := col.Value(obj)
	if col.Boolean {
		return booleanIconHTML(value)
	}
	if h, ok := value.(template.HTML); ok {
		return string(h)
	}
	if value == nil {
		// Django's linebreaksbr turns None into its string.
		return "None"
	}
	return linebreaksbr(html.EscapeString(fmt.Sprint(value)))
}

// readonlyRelated returns the HTML of a read-only foreign key: a link to the
// change page of the related object, its label when its model has no admin
// on the site, or the empty value when it is not set.
func (s *AdminSite) readonlyRelated(ctx context.Context, fv reflect.Value, f *orm.Field, empty string) string {
	pk, ok := fkPK(fv)
	if !ok || pk == 0 || f.Rel.Target == nil {
		return html.EscapeString(empty)
	}
	rows, err := orm.LabeledRowsOrdered(ctx, s.db, f.Rel.Target, nil, []any{pk})
	if err != nil || len(rows) == 0 {
		return html.EscapeString(empty)
	}
	label := html.EscapeString(rows[0][1])
	if rel := s.entryByModel(f.Rel.Target); rel != nil {
		return `<a href="` + html.EscapeString(s.changeURL(rel, pk)) + `">` + label + `</a>`
	}
	return linebreaksbr(label)
}

// hasPK reports whether obj, a *T of e, has a primary key, that is, has been
// saved.
func hasPK(e *entry, obj any) bool {
	return pkOf(reflect.ValueOf(obj).Elem(), e) != 0
}

// linebreaksbr replaces the line breaks of escaped text with <br>, Django's
// linebreaksbr filter.
func linebreaksbr(escaped string) string {
	escaped = strings.ReplaceAll(escaped, "\r\n", "\n")
	escaped = strings.ReplaceAll(escaped, "\r", "\n")
	return strings.ReplaceAll(escaped, "\n", "<br>")
}
