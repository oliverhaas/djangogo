package migrations

import (
	"fmt"
	"slices"
	"strings"

	"github.com/oliverhaas/djangogo/orm"
)

// FieldState is the snapshot of one model field as a migration records it. It
// carries what decides the database column and the default used for existing
// rows. Options that only affect forms and the admin (editable, verbose_name,
// help_text, choices) are left out, so changing them needs no migration.
type FieldState struct {
	// Name is the Go struct field name, e.g. "Title".
	Name string
	// Column is the database column, e.g. "title" or "author_id".
	Column string
	// Kind is the logical column type.
	Kind       orm.Kind
	PrimaryKey bool
	Null       bool
	Unique     bool
	// MaxLength is the VARCHAR length of a Char field.
	MaxLength int
	// MaxDigits and DecimalPlaces hold precision and scale for KindDecimal.
	MaxDigits     int
	DecimalPlaces int
	// DBIndex requests a single-column index (the field option "index").
	DBIndex bool
	// Blank mirrors Django's blank=True. A non-null Char or Text field that may
	// be blank gets the empty string for existing rows when it is added.
	Blank bool
	// Default is the field default (int64, bool or string), or nil for none.
	Default any
	// AutoNow and AutoNowAdd mirror the DateTime options of the same name.
	AutoNow    bool
	AutoNowAdd bool
	// RelKind classifies a relation field; orm.RelNone for a scalar field.
	RelKind orm.RelKind
	// RelTo is the label of the referenced model, "app.Model", for a relation.
	RelTo string
	// RelOnDelete is the ON DELETE action of a foreign key.
	RelOnDelete orm.OnDelete
}

// Equal reports whether two field states are identical.
func (f FieldState) Equal(other FieldState) bool {
	if !defaultsEqual(f.Default, other.Default) {
		return false
	}
	f.Default, other.Default = nil, nil
	return f == other
}

// sameDefinition reports whether two fields differ at most in name and
// column. A renamed field keeps its definition, which is what rename detection
// looks for.
func (f FieldState) sameDefinition(other FieldState) bool {
	if !defaultsEqual(f.Default, other.Default) {
		return false
	}
	f.Name, f.Column, f.Default = "", "", nil
	other.Name, other.Column, other.Default = "", "", nil
	return f == other
}

// IsRelation reports whether the field is a foreign key.
func (f FieldState) IsRelation() bool { return f.RelKind != orm.RelNone && f.RelTo != "" }

// Attr returns the snake_case attribute name, the counterpart of Django's
// field name: "created_at" for CreatedAt.
func (f FieldState) Attr() string { return snakeCase(f.Name) }

// fieldStateFromField snapshots an orm field.
func fieldStateFromField(f *orm.Field) FieldState {
	fs := FieldState{
		Name:          f.Name,
		Column:        f.Column,
		Kind:          f.Kind,
		PrimaryKey:    f.PrimaryKey,
		Null:          f.Null,
		Unique:        f.Unique,
		MaxLength:     f.MaxLength,
		MaxDigits:     f.MaxDigits,
		DecimalPlaces: f.DecimalPlaces,
		DBIndex:       f.DBIndex,
		Blank:         f.Blank,
		AutoNow:       f.AutoNow,
		AutoNowAdd:    f.AutoNowAdd,
	}
	if f.HasDefault {
		fs.Default = f.Default
	}
	if f.Rel != nil {
		fs.RelKind = f.Rel.Kind
		fs.RelOnDelete = f.Rel.OnDelete
		if f.Rel.Target != nil {
			fs.RelTo = ModelLabel(f.Rel.Target.App(), f.Rel.Target.Name())
		}
	}
	return fs
}

// Index is a named index over one or more fields of a model.
type Index struct {
	// Name is the index name in the database. It is required.
	Name string
	// Fields lists the Go field names the index covers, in order.
	Fields []string
}

// ModelState is the snapshot of one model: its app, table, fields, indexes and
// unique-together sets.
type ModelState struct {
	// App is the app label the model belongs to.
	App string
	// Name is the Go type name, e.g. "Post".
	Name string
	// Table is the database table.
	Table string
	// Fields holds the fields in declaration order.
	Fields []FieldState
	// Indexes holds the model-level indexes.
	Indexes []Index
	// UniqueTogether holds sets of Go field names that must be unique together.
	UniqueTogether [][]string
}

// Label returns "app.Name".
func (m *ModelState) Label() string { return ModelLabel(m.App, m.Name) }

// FieldByName returns the field with the given Go name.
func (m *ModelState) FieldByName(name string) (*FieldState, bool) {
	for i := range m.Fields {
		if m.Fields[i].Name == name {
			return &m.Fields[i], true
		}
	}
	return nil, false
}

// PrimaryKey returns the primary-key field, or nil when the model has none.
func (m *ModelState) PrimaryKey() *FieldState {
	for i := range m.Fields {
		if m.Fields[i].PrimaryKey {
			return &m.Fields[i]
		}
	}
	return nil
}

// IndexByName returns the model-level index with the given name.
func (m *ModelState) IndexByName(name string) (*Index, bool) {
	for i := range m.Indexes {
		if m.Indexes[i].Name == name {
			return &m.Indexes[i], true
		}
	}
	return nil, false
}

// columns resolves Go field names to their database columns.
func (m *ModelState) columns(fields []string) ([]string, error) {
	out := make([]string, len(fields))
	for i, name := range fields {
		f, ok := m.FieldByName(name)
		if !ok {
			return nil, fmt.Errorf("migrations: %s has no field named %q", m.Label(), name)
		}
		out[i] = f.Column
	}
	return out, nil
}

// clone returns a deep copy.
func (m *ModelState) clone() *ModelState {
	out := *m
	out.Fields = slices.Clone(m.Fields)
	out.Indexes = make([]Index, len(m.Indexes))
	for i, idx := range m.Indexes {
		out.Indexes[i] = Index{Name: idx.Name, Fields: slices.Clone(idx.Fields)}
	}
	if len(m.Indexes) == 0 {
		out.Indexes = nil
	}
	out.UniqueTogether = cloneFieldSets(m.UniqueTogether)
	return &out
}

// cloneFieldSets deep-copies a list of field-name sets.
func cloneFieldSets(sets [][]string) [][]string {
	if len(sets) == 0 {
		return nil
	}
	out := make([][]string, len(sets))
	for i, s := range sets {
		out[i] = slices.Clone(s)
	}
	return out
}

// ModelLabel joins an app label and a model name into "app.Model".
func ModelLabel(app, name string) string { return app + "." + name }

// splitLabel splits "app.Model" into its parts.
func splitLabel(label string) (app, name string) {
	i := strings.LastIndex(label, ".")
	if i < 0 {
		return "", label
	}
	return label[:i], label[i+1:]
}

// ProjectState is the set of all model states at one point of the migration
// history, the counterpart of Django's ProjectState.
type ProjectState struct {
	// Models maps the label "app.Model" to the model state.
	Models map[string]*ModelState
	// Order holds the labels in creation order.
	Order []string
}

// NewProjectState returns an empty state.
func NewProjectState() *ProjectState {
	return &ProjectState{Models: map[string]*ModelState{}}
}

// Model returns the state of the model name in app.
func (ps *ProjectState) Model(app, name string) (*ModelState, bool) {
	ms, ok := ps.Models[ModelLabel(app, name)]
	return ms, ok
}

// mustModel returns the model or an error that names it.
func (ps *ProjectState) mustModel(app, name string) (*ModelState, error) {
	ms, ok := ps.Model(app, name)
	if !ok {
		return nil, fmt.Errorf("migrations: model %s not found in the migration state", ModelLabel(app, name))
	}
	return ms, nil
}

// AddModel adds or replaces a model state.
func (ps *ProjectState) AddModel(ms *ModelState) {
	label := ms.Label()
	if _, ok := ps.Models[label]; !ok {
		ps.Order = append(ps.Order, label)
	}
	ps.Models[label] = ms
}

// RemoveModel removes the model name of app.
func (ps *ProjectState) RemoveModel(app, name string) {
	label := ModelLabel(app, name)
	delete(ps.Models, label)
	if i := slices.Index(ps.Order, label); i >= 0 {
		ps.Order = slices.Delete(ps.Order, i, i+1)
	}
}

// Clone returns a deep copy.
func (ps *ProjectState) Clone() *ProjectState {
	out := &ProjectState{
		Models: make(map[string]*ModelState, len(ps.Models)),
		Order:  slices.Clone(ps.Order),
	}
	for label, ms := range ps.Models {
		out.Models[label] = ms.clone()
	}
	return out
}

// Apps returns the sorted app labels that have at least one model.
func (ps *ProjectState) Apps() []string {
	var out []string
	for _, ms := range ps.Models {
		if !slices.Contains(out, ms.App) {
			out = append(out, ms.App)
		}
	}
	slices.Sort(out)
	return out
}

// relatedTo returns the model a relation field points to.
func (ps *ProjectState) relatedTo(f FieldState) (*ModelState, error) {
	ms, ok := ps.Models[f.RelTo]
	if !ok {
		return nil, fmt.Errorf("migrations: field %s refers to model %s, which is not in the migration state",
			f.Name, f.RelTo)
	}
	return ms, nil
}

// referencing returns every (model, field) whose foreign key points to label.
func (ps *ProjectState) referencing(label string) []fieldRef {
	var out []fieldRef
	for _, l := range ps.Order {
		ms := ps.Models[l]
		for i := range ms.Fields {
			if ms.Fields[i].IsRelation() && ms.Fields[i].RelTo == label {
				out = append(out, fieldRef{model: ms, field: &ms.Fields[i]})
			}
		}
	}
	return out
}

// fieldRef names one field of one model.
type fieldRef struct {
	model *ModelState
	field *FieldState
}

// StateFromRegistry snapshots every model of r. Models keep their registration
// order. Model-level indexes and unique-together sets are not read, because
// the registry does not expose them.
func StateFromRegistry(r *orm.Registry) *ProjectState {
	ps := NewProjectState()
	for _, m := range r.Models() {
		raw := m.Fields()
		fields := make([]FieldState, len(raw))
		for i, f := range raw {
			fields[i] = fieldStateFromField(f)
		}
		ps.AddModel(&ModelState{App: m.App(), Name: m.Name(), Table: m.Table(), Fields: fields})
	}
	return ps
}

// StateFromMigrations replays the operations of migs, in slice order, onto an
// empty state. Use Graph.MakeState when the migrations of several apps depend
// on each other.
func StateFromMigrations(migs []Migration) (*ProjectState, error) {
	ps := NewProjectState()
	for i := range migs {
		if err := migs[i].MutateState(ps); err != nil {
			return nil, err
		}
	}
	return ps, nil
}
