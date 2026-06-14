// Package migrations provides types and utilities for capturing schema state
// snapshots from the ORM registry, comparing states across migrations, and
// building the auto-migration engine that generates SQL DDL operations.
package migrations

import (
	"github.com/oliverhaas/djangogo/orm"
)

// FieldState is a serializable snapshot of one model field's schema-relevant metadata.
type FieldState struct {
	Name       string
	Column     string
	Kind       orm.Kind
	PrimaryKey bool
	Null       bool
	Unique     bool
	MaxLength  int
	// RelKind classifies a relation field; orm.RelNone for scalar fields.
	RelKind orm.RelKind
	// RelTargetTable is the referenced table for a FK, e.g. "author".
	RelTargetTable string
	// RelTargetColumn is the referenced primary-key column for a FK, e.g. "id".
	RelTargetColumn string
	// RelOnDelete is the FK's ON DELETE action; orm.OnDeleteDoNothing for scalars
	// and for FKs that do not set on_delete.
	RelOnDelete orm.OnDelete
}

// Equal reports whether two field states are schema-equivalent.
func (f FieldState) Equal(other FieldState) bool {
	return f.Name == other.Name &&
		f.Column == other.Column &&
		f.Kind == other.Kind &&
		f.PrimaryKey == other.PrimaryKey &&
		f.Null == other.Null &&
		f.Unique == other.Unique &&
		f.MaxLength == other.MaxLength &&
		f.RelKind == other.RelKind &&
		f.RelTargetTable == other.RelTargetTable &&
		f.RelTargetColumn == other.RelTargetColumn &&
		f.RelOnDelete == other.RelOnDelete
}

// fieldStateFromField builds a FieldState from an orm.Field pointer. Relation
// metadata is captured only when f carries a relation; the target table and
// column require the registry to have been resolved (Rel.Target set).
func fieldStateFromField(f *orm.Field) FieldState {
	fs := FieldState{
		Name:       f.Name,
		Column:     f.Column,
		Kind:       f.Kind,
		PrimaryKey: f.PrimaryKey,
		Null:       f.Null,
		Unique:     f.Unique,
		MaxLength:  f.MaxLength,
	}
	if f.Rel != nil {
		fs.RelKind = f.Rel.Kind
		fs.RelOnDelete = f.Rel.OnDelete
		if f.Rel.Target != nil {
			fs.RelTargetTable = f.Rel.Target.Table()
			fs.RelTargetColumn = f.Rel.Target.PrimaryKey().Column
		}
	}
	return fs
}

// ModelState is a snapshot of one model's table and ordered fields.
type ModelState struct {
	Name   string
	Table  string
	Fields []FieldState
}

// FieldByName returns the FieldState with the given Go struct name, or false if
// no such field exists.
func (m *ModelState) FieldByName(name string) (*FieldState, bool) {
	for i := range m.Fields {
		if m.Fields[i].Name == name {
			return &m.Fields[i], true
		}
	}
	return nil, false
}

// ProjectState is the full set of model states, with a deterministic creation order.
type ProjectState struct {
	// Models maps model name to its ModelState snapshot.
	Models map[string]*ModelState
	// Order holds model names in registration order.
	Order []string
}

// NewProjectState returns an empty ProjectState ready for population.
func NewProjectState() *ProjectState {
	return &ProjectState{Models: map[string]*ModelState{}}
}

// Clone returns a deep copy of the ProjectState. Mutating the clone does not
// affect the original.
func (ps *ProjectState) Clone() *ProjectState {
	out := &ProjectState{
		Models: make(map[string]*ModelState, len(ps.Models)),
		Order:  make([]string, len(ps.Order)),
	}
	copy(out.Order, ps.Order)
	for k, ms := range ps.Models {
		cloned := &ModelState{
			Name:   ms.Name,
			Table:  ms.Table,
			Fields: make([]FieldState, len(ms.Fields)),
		}
		copy(cloned.Fields, ms.Fields)
		out.Models[k] = cloned
	}
	return out
}

// StateFromRegistry snapshots every model in r into a ProjectState. Order
// follows the registry's registration order.
func StateFromRegistry(r *orm.Registry) *ProjectState {
	ps := NewProjectState()
	for _, m := range r.Models() {
		rawFields := m.Fields()
		fields := make([]FieldState, len(rawFields))
		for i, f := range rawFields {
			fields[i] = fieldStateFromField(f)
		}
		ms := &ModelState{
			Name:   m.Name(),
			Table:  m.Table(),
			Fields: fields,
		}
		ps.Models[m.Name()] = ms
		ps.Order = append(ps.Order, m.Name())
	}
	return ps
}
