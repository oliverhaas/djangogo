package admin

import (
	"fmt"
	"html"
	"net/url"
	"reflect"
	"strings"

	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/templates"
)

// resolvedColumn describes one column in a changelist's ListDisplay after
// resolving field names, relation paths, "__str__", or custom DisplayColumns.
type resolvedColumn struct {
	// rawName is the entry as written in ModelAdmin.ListDisplay.
	rawName string
	// cssName is the normalized CSS identifier used in column-<name> / field-<name>.
	cssName string
	// label is the column header text.
	label string
	// orderField is the ORM OrderBy key for sorting this column, or "" if unsortable.
	orderField string
	// field is the direct model field when this column is a local field.
	field *orm.Field
	// pathSteps holds the sequence of concrete fields along a "__"-spanning FK/O2O path.
	pathSteps []*orm.Field
	// pathModels holds the owning *orm.Model of each step in pathSteps.
	pathModels []*orm.Model
	// isStr reports whether this column renders the object's string representation.
	isStr bool
	// isActionCheckbox reports whether this column is the leading selection checkbox.
	isActionCheckbox bool
	// custom holds the custom DisplayColumn definition when matched in ModelAdmin.DisplayColumns.
	custom *DisplayColumn
}

// resolveModelField looks up a concrete field on m by Go struct name, attribute
// name, column name, or "pk".
func resolveModelField(m *orm.Model, name string) (*orm.Field, bool) {
	if name == "pk" {
		pk := m.PrimaryKey()
		return pk, pk != nil
	}
	if f, ok := m.FieldByName(name); ok {
		return f, true
	}
	if f, ok := m.FieldByAttr(name); ok {
		return f, true
	}
	if f, ok := m.FieldByColumn(name); ok {
		return f, true
	}
	if attr, ok := strings.CutSuffix(name, "_id"); ok {
		if f, found := m.FieldByAttr(attr); found && f.Rel != nil {
			return f, true
		}
	}
	return nil, false
}

// normalizeFieldPath translates a field name or "__"-separated path whose
// segments may use Go struct field names (e.g. "Author__Name") into ORM
// snake_case attribute paths ("author__name").
func normalizeFieldPath(m *orm.Model, raw string) string {
	if m == nil || raw == "" {
		return raw
	}
	desc := strings.HasPrefix(raw, "-")
	bare := strings.TrimPrefix(raw, "-")
	parts := strings.Split(bare, "__")
	cur := m
	out := make([]string, len(parts))
	for i, part := range parts {
		if cur == nil {
			out[i] = part
			continue
		}
		if part == "pk" {
			out[i] = "pk"
			continue
		}
		if f, ok := cur.FieldByName(part); ok {
			out[i] = f.Attr
			if f.Rel != nil && f.Rel.Target != nil {
				cur = f.Rel.Target
			} else {
				cur = nil
			}
			continue
		}
		if f, ok := cur.FieldByAttr(part); ok {
			out[i] = f.Attr
			if f.Rel != nil && f.Rel.Target != nil {
				cur = f.Rel.Target
			} else {
				cur = nil
			}
			continue
		}
		if f, ok := cur.ManyToManyField(part); ok {
			out[i] = f.Attr
			if f.Rel != nil && f.Rel.Target != nil {
				cur = f.Rel.Target
			} else {
				cur = nil
			}
			continue
		}
		if info, ok := orm.ResolveLookupPath(cur, part); ok && info.Reverse != nil {
			out[i] = info.Reverse.QueryName
			cur = info.TargetModel
			continue
		}
		out[i] = part
		cur = nil
	}
	joined := strings.Join(out, "__")
	if desc {
		return "-" + joined
	}
	return joined
}

// resolveDisplayColumn resolves one entry of ModelAdmin.ListDisplay into a
// resolvedColumn. Returns ok=false for unrecognized names.
func resolveDisplayColumn(e *entry, name string) (resolvedColumn, bool) {
	if name == "action_checkbox" {
		return resolvedColumn{
			rawName:          name,
			cssName:          "action-checkbox",
			isActionCheckbox: true,
		}, true
	}
	if name == "__str__" {
		singular, _ := modelVerboseNames(e.model)
		return resolvedColumn{
			rawName: name,
			cssName: "__str__",
			label:   capFirst(singular),
			isStr:   true,
		}, true
	}
	if col, ok := e.admin.DisplayColumns[name]; ok {
		c := col
		label := c.Description
		if label == "" {
			label = prettyName(name)
		}
		orderField := ""
		if c.OrderField != "" {
			orderField = normalizeFieldPath(e.model, c.OrderField)
		}
		return resolvedColumn{
			rawName:    name,
			cssName:    name,
			label:      label,
			orderField: orderField,
			custom:     &c,
		}, true
	}
	if f, ok := resolveModelField(e.model, name); ok {
		return resolvedColumn{
			rawName:    name,
			cssName:    f.Attr,
			label:      fieldLabel(f),
			orderField: f.Attr,
			field:      f,
		}, true
	}
	// "__"-spanning forward FK/O2O path such as "parent__name" or "parent__parent__name".
	if strings.Contains(name, "__") {
		parts := strings.Split(name, "__")
		cur := e.model
		steps := make([]*orm.Field, 0, len(parts))
		models := make([]*orm.Model, 0, len(parts))
		normParts := make([]string, 0, len(parts))
		for i, part := range parts {
			if cur == nil {
				return resolvedColumn{}, false
			}
			f, ok := resolveModelField(cur, part)
			if !ok {
				return resolvedColumn{}, false
			}
			steps = append(steps, f)
			models = append(models, cur)
			normParts = append(normParts, f.Attr)
			if i < len(parts)-1 {
				if f.Rel == nil || (f.Rel.Kind != orm.RelFK && f.Rel.Kind != orm.RelO2O) || f.Rel.Target == nil {
					return resolvedColumn{}, false
				}
				cur = f.Rel.Target
			}
		}
		last := steps[len(steps)-1]
		normPath := strings.Join(normParts, "__")
		return resolvedColumn{
			rawName:    name,
			cssName:    normPath,
			label:      fieldLabel(last),
			orderField: normPath,
			pathSteps:  steps,
			pathModels: models,
		}, true
	}
	return resolvedColumn{}, false
}

// selectRelatedForColumns collects the forward FK/O2O select_related paths
// needed to render cols without N+1 queries.
func selectRelatedForColumns(cols []resolvedColumn) []string {
	seen := make(map[string]bool)
	var out []string
	add := func(path string) {
		if path != "" && !seen[path] {
			seen[path] = true
			out = append(out, path)
		}
	}
	for _, col := range cols {
		if col.field != nil && col.field.Rel != nil && (col.field.Rel.Kind == orm.RelFK || col.field.Rel.Kind == orm.RelO2O) {
			add(col.field.Attr)
			continue
		}
		if len(col.pathSteps) > 0 {
			// Select every relation hop along the path; if the terminal field
			// itself is a forward FK/O2O, include it too.
			limit := len(col.pathSteps) - 1
			last := col.pathSteps[len(col.pathSteps)-1]
			if last.Rel != nil && (last.Rel.Kind == orm.RelFK || last.Rel.Kind == orm.RelO2O) {
				limit = len(col.pathSteps)
			}
			if limit > 0 {
				segs := make([]string, limit)
				for i := range limit {
					segs[i] = col.pathSteps[i].Attr
				}
				add(strings.Join(segs, "__"))
			}
		}
	}
	return out
}

// renderColumnCell renders one changelist cell as safe HTML (pre-escaped) and
// returns the extra CSS classes (such as "nowrap") for the cell.
func renderColumnCell(col resolvedColumn, obj any, m *orm.Model, emptyVal string, avoidLink bool) (string, []string) {
	if col.isStr {
		label := orm.Label(m, obj)
		if strings.TrimSpace(label) == "" {
			label = emptyVal
		}
		return html.EscapeString(label), nil
	}
	if col.custom != nil {
		colEmpty := emptyVal
		if col.custom.EmptyValue != "" {
			colEmpty = col.custom.EmptyValue
		}
		val := any(nil)
		if col.custom.Value != nil {
			val = col.custom.Value(obj)
		}
		return formatDisplayValueHTML(val, colEmpty, col.custom.Boolean), nil
	}
	if col.field != nil {
		elem := reflect.ValueOf(obj).Elem()
		return formatFieldHTML(m, obj, col.field, elem.Field(col.field.Index), emptyVal, avoidLink)
	}
	if len(col.pathSteps) > 0 {
		curObj := obj
		for i, step := range col.pathSteps {
			rv := reflect.ValueOf(curObj)
			if !rv.IsValid() || (rv.Kind() == reflect.Pointer && rv.IsNil()) {
				return html.EscapeString(emptyVal), nil
			}
			if rv.Kind() == reflect.Pointer {
				rv = rv.Elem()
			}
			fv := rv.Field(step.Index)
			if i == len(col.pathSteps)-1 {
				return formatFieldHTML(col.pathModels[i], curObj, step, fv, emptyVal, avoidLink)
			}
			relObj, loaded := fkObject(fv)
			if !loaded || relObj == nil {
				return html.EscapeString(emptyVal), nil
			}
			curObj = relObj
		}
	}
	return html.EscapeString(emptyVal), nil
}

// formatFieldHTML formats a concrete model field value as HTML matching
// Django's display_for_field (choices via orm.GetDisplay, boolean icons, URL
// links, FK labels, empty value placeholder, and nowrap classes).
func formatFieldHTML(m *orm.Model, obj any, f *orm.Field, fv reflect.Value, emptyVal string, avoidLink bool) (string, []string) {
	var extraClasses []string
	if f.Kind == orm.KindDate || f.Kind == orm.KindTime || (f.Rel != nil && (f.Rel.Kind == orm.RelFK || f.Rel.Kind == orm.RelO2O)) {
		extraClasses = append(extraClasses, "nowrap")
	}

	// Forward FK or O2O relation field.
	if f.Rel != nil && (f.Rel.Kind == orm.RelFK || f.Rel.Kind == orm.RelO2O) {
		pk, ok := fkPK(fv)
		if !ok || pk == 0 {
			return html.EscapeString(emptyVal), extraClasses
		}
		if relObj, loaded := fkObject(fv); loaded && relObj != nil && f.Rel.Target != nil {
			lbl := orm.Label(f.Rel.Target, relObj)
			if strings.TrimSpace(lbl) == "" {
				lbl = emptyVal
			}
			return html.EscapeString(lbl), extraClasses
		}
		return html.EscapeString(fmt.Sprint(pk)), extraClasses
	}

	// Choices field (other than BooleanField without explicit match, wait:
	// Django checks flatchoices BEFORE BooleanField!).
	if f.HasChoices && len(f.FlatChoices()) > 0 {
		_, isNil := derefValue(fv)
		if isNil {
			// Check if choices declare a label for nil.
			for _, ch := range f.FlatChoices() {
				if ch.Raw == nil && ch.Value == "" {
					return html.EscapeString(ch.Label), extraClasses
				}
			}
			return html.EscapeString(emptyVal), extraClasses
		}
		disp := orm.GetDisplay(m, obj, f.Name)
		if disp == nil {
			return html.EscapeString(emptyVal), extraClasses
		}
		s := fmt.Sprint(disp)
		if strings.TrimSpace(s) == "" {
			return html.EscapeString(emptyVal), extraClasses
		}
		return html.EscapeString(s), extraClasses
	}

	// Boolean field renders Django's icon HTML.
	if f.Kind == orm.KindBool {
		val, isNil := derefValue(fv)
		if isNil {
			return booleanIconHTML(nil), extraClasses
		}
		return booleanIconHTML(val), extraClasses
	}

	val, isNil := derefValue(fv)
	if isNil {
		return html.EscapeString(emptyVal), extraClasses
	}

	// Safe URL link unless avoidLink is true (field is already in list_display_links).
	if f.Kind == orm.KindURL {
		s := strings.TrimSpace(fmt.Sprint(val))
		if s == "" {
			return html.EscapeString(emptyVal), extraClasses
		}
		if !avoidLink && isSafeHTTPURL(s) {
			esc := html.EscapeString(s)
			return fmt.Sprintf(`<a href="%s">%s</a>`, esc, esc), extraClasses
		}
		return html.EscapeString(s), extraClasses
	}

	return formatDisplayValueHTML(val, emptyVal, false), extraClasses
}

// formatDisplayValueHTML formats an arbitrary value as safe HTML, matching
// Django's display_for_value.
func formatDisplayValueHTML(val any, emptyVal string, boolean bool) string {
	if boolean {
		return booleanIconHTML(val)
	}
	if val == nil {
		return html.EscapeString(emptyVal)
	}
	rv := reflect.ValueOf(val)
	if rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return html.EscapeString(emptyVal)
		}
		val = rv.Elem().Interface()
	}
	if s, ok := val.(string); ok {
		if strings.TrimSpace(s) == "" {
			return html.EscapeString(emptyVal)
		}
		return html.EscapeString(strings.TrimSpace(s))
	}
	if s, ok := templates.Localize(val); ok {
		if s == "" {
			return html.EscapeString(emptyVal)
		}
		return html.EscapeString(s)
	}
	return html.EscapeString(fmt.Sprint(val))
}

// booleanIconHTML returns Django's boolean indicator <img> tag for True, False,
// or None (nil).
func booleanIconHTML(val any) string {
	icon := "unknown"
	alt := "None"
	if val != nil {
		rv := reflect.ValueOf(val)
		if rv.Kind() == reflect.Pointer {
			if !rv.IsNil() {
				val = rv.Elem().Interface()
			} else {
				val = nil
			}
		}
	}
	if b, ok := val.(bool); ok {
		if b {
			icon = "yes"
			alt = "True"
		} else {
			icon = "no"
			alt = "False"
		}
	}
	return fmt.Sprintf(`<img src="/static/admin/img/icon-%s.svg" alt="%s">`, icon, alt)
}

// isSafeHTTPURL reports whether raw parses as an http, https, or ftp URL with
// a non-empty host, preventing javascript: or other unsafe links.
func isSafeHTTPURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https", "ftp":
		return u.Host != ""
	default:
		return false
	}
}

// derefValue unwraps a pointer reflect.Value, returning (nil, true) when nil.
func derefValue(v reflect.Value) (any, bool) {
	if !v.IsValid() {
		return nil, true
	}
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return nil, true
		}
		return v.Elem().Interface(), false
	}
	return v.Interface(), false
}

// pkOf returns the primary-key value of the struct addressed by elem for model
// e, as an int64 (0 when the model has no primary key).
func pkOf(elem reflect.Value, e *entry) int64 {
	pk := e.model.PrimaryKey()
	if pk == nil {
		return 0
	}
	fv := elem.Field(pk.Index)
	if fv.CanInt() {
		return fv.Int()
	}
	return 0
}

// fkPK reports whether v is an orm.FK[..] or orm.O2O[..] value and, if so,
// returns its related primary key by calling its PK method via reflection.
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

// fkObject reports whether v is a loaded orm.FK[..] or orm.O2O[..] value and
// returns its loaded related *T pointer.
func fkObject(v reflect.Value) (any, bool) {
	m := v.MethodByName("Object")
	if !m.IsValid() {
		return nil, false
	}
	mt := m.Type()
	if mt.NumIn() != 0 || mt.NumOut() != 2 || mt.Out(1).Kind() != reflect.Bool {
		return nil, false
	}
	res := m.Call(nil)
	if !res[1].Bool() || res[0].IsNil() {
		return nil, false
	}
	return res[0].Interface(), true
}
