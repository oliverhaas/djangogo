// Package orm provides metadata primitives and query primitives for the Djan-Go-Go ORM.
package orm

import "reflect"

// Model is an immutable descriptor for a registered Go struct type.
// All mutation happens during Register; after Freeze the registry and its
// models are read-only by convention (no exported setters exist).
type Model struct {
	name     string
	table    string
	fields   []*Field
	pk       *Field
	byName   map[string]*Field
	byColumn map[string]*Field
	goType   reflect.Type
}

// Name returns the Go struct type name (e.g. "Author").
func (m *Model) Name() string { return m.name }

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

// Columns returns the database column names in field-declaration order.
func (m *Model) Columns() []string {
	cols := make([]string, len(m.fields))
	for i, f := range m.fields {
		cols[i] = f.Column
	}
	return cols
}

// GoType returns the reflect.Type of the underlying Go struct (not a pointer).
func (m *Model) GoType() reflect.Type { return m.goType }
