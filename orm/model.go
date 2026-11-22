// Package orm provides metadata primitives and query primitives for the Djan-Go-Go ORM.
package orm

import "reflect"

// Model is an immutable descriptor for a registered Go struct type.
// All mutation happens during Register; after Freeze the registry and its
// models are read-only by convention (no exported setters exist).
type Model struct {
	app                   string
	name                  string
	table                 string
	fields                []*Field
	pk                    *Field
	byName                map[string]*Field
	byAttr                map[string]*Field
	byColumn              map[string]*Field
	goType                reflect.Type
	cacheIndex            int
	autoCreated           bool
	m2mFields             []*Field
	byM2MName             map[string]*Field
	byM2MAttr             map[string]*Field
	reverseRels           []*ReverseRelation
	byReverseName         map[string]*ReverseRelation
	byReverseQuery        map[string]*ReverseRelation
	ambiguousReverseName  map[string]bool
	ambiguousReverseQuery map[string]bool
	ordering              []string
	uniqueTogether        [][]string
	indexes               []Index
	constraints           []Constraint
	verboseName           string
	verboseNamePlural     string
}

// Name returns the Go struct type name (e.g. "Author").
func (m *Model) Name() string { return m.name }

// App returns the label of the app the model was registered under (Django's
// app_label, e.g. "blog"), or "" when it was registered without one.
func (m *Model) App() string { return m.app }

// Label returns the model's registry key: "<app>.<Name>" (e.g. "blog.Post") for
// a model registered under an app, and the bare Name otherwise. It is unique
// within a Registry.
func (m *Model) Label() string {
	if m.app == "" {
		return m.name
	}
	return m.app + "." + m.name
}

// Table returns the database table name.
func (m *Model) Table() string { return m.table }

// Fields returns a shallow copy of the model's field slice.
// Callers may reorder or append to the returned slice without affecting the Model.
// The *Field values themselves must be treated as read-only.
func (m *Model) Fields() []*Field {
	cp := make([]*Field, len(m.fields))
	copy(cp, m.fields)
	return cp
}

// PrimaryKey returns the primary-key field, or nil if none was resolved.
func (m *Model) PrimaryKey() *Field { return m.pk }

// Relations returns the model's relation fields (those with a non-nil Rel) in
// field-declaration order.
func (m *Model) Relations() []*Field {
	var rels []*Field
	for _, f := range m.fields {
		if f.Rel != nil {
			rels = append(rels, f)
		}
	}
	return rels
}

// FieldByName looks up a field by its Go struct name.
func (m *Model) FieldByName(name string) (*Field, bool) {
	f, ok := m.byName[name]
	return f, ok
}

// FieldByAttr looks up a field by its snake_case attribute name (Field.Attr),
// e.g. "created_at" or, for a foreign key, "author".
func (m *Model) FieldByAttr(attr string) (*Field, bool) {
	f, ok := m.byAttr[attr]
	return f, ok
}

// FieldByColumn looks up a field by its database column name, e.g. "author_id".
func (m *Model) FieldByColumn(column string) (*Field, bool) {
	f, ok := m.byColumn[column]
	return f, ok
}

// Columns returns the database column names in field-declaration order.
func (m *Model) Columns() []string {
	cols := make([]string, len(m.fields))
	for i, f := range m.fields {
		cols[i] = f.Column
	}
	return cols
}

// GoType returns the reflect.Type of the underlying Go struct (not a pointer).
// It is nil for synthetic auto-created M2M through models.
func (m *Model) GoType() reflect.Type { return m.goType }

// AutoCreated reports whether m is a synthetic intermediate model generated
// automatically for a many-to-many field without an explicit through= model.
func (m *Model) AutoCreated() bool { return m.autoCreated }

// ManyToMany returns a shallow copy of the forward many-to-many fields
// declared on m, in field-declaration order.
func (m *Model) ManyToMany() []*Field {
	if len(m.m2mFields) == 0 {
		return nil
	}
	cp := make([]*Field, len(m.m2mFields))
	copy(cp, m.m2mFields)
	return cp
}

// ManyToManyField looks up a declared forward many-to-many field on m by its
// snake_case attribute name (e.g. "publications") or Go struct field name
// (e.g. "Publications").
func (m *Model) ManyToManyField(name string) (*Field, bool) {
	if f, ok := m.byM2MAttr[name]; ok {
		return f, true
	}
	f, ok := m.byM2MName[name]
	return f, ok
}
