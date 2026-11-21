package migrations

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
)

// CreateModel creates a model's table, with its indexes and unique-together
// sets.
type CreateModel struct {
	// Name is the Go type name of the model.
	Name string
	// Table is the database table; empty means "<app>_<name>" in lowercase.
	Table          string
	Fields         []FieldState
	Indexes        []Index
	UniqueTogether [][]string
}

// StateForwards adds the model to the state.
func (o CreateModel) StateForwards(app string, state *ProjectState) error {
	if _, ok := state.Model(app, o.Name); ok {
		return fmt.Errorf("migrations: model %s already exists in the migration state", ModelLabel(app, o.Name))
	}
	seen := map[string]bool{}
	for _, f := range o.Fields {
		if seen[f.Name] {
			return fmt.Errorf("migrations: found duplicate value %s in CreateModel fields argument", f.Name)
		}
		seen[f.Name] = true
	}
	for _, idx := range o.Indexes {
		if idx.Name == "" {
			return errors.New("migrations: indexes passed to CreateModel operations require a name argument")
		}
	}
	table := o.Table
	if table == "" {
		table = defaultTable(app, o.Name)
	}
	ms := &ModelState{App: app, Name: o.Name, Table: table, Fields: o.Fields, Indexes: o.Indexes, UniqueTogether: o.UniqueTogether}
	state.AddModel(ms.clone())
	return nil
}

// DatabaseForwards creates the table.
func (o CreateModel) DatabaseForwards(ctx context.Context, app string, e *SchemaEditor, _, to *ProjectState) error {
	ms, err := to.mustModel(app, o.Name)
	if err != nil {
		return err
	}
	return e.createModel(ctx, to, ms)
}

// DatabaseBackwards drops the table.
func (o CreateModel) DatabaseBackwards(ctx context.Context, app string, e *SchemaEditor, from, _ *ProjectState) error {
	ms, err := from.mustModel(app, o.Name)
	if err != nil {
		return err
	}
	return e.deleteModel(ctx, from, ms)
}

// Describe returns Django's description.
func (o CreateModel) Describe() string { return "Create model " + o.Name }

// Category marks the operation as an addition.
func (CreateModel) Category() Category { return CategoryAddition }

// NameFragment returns the lowercase model name.
func (o CreateModel) NameFragment() string { return lowerName(o.Name) }

// DeleteModel drops a model's table.
type DeleteModel struct {
	Name string
}

// StateForwards removes the model from the state.
func (o DeleteModel) StateForwards(app string, state *ProjectState) error {
	if _, err := state.mustModel(app, o.Name); err != nil {
		return err
	}
	state.RemoveModel(app, o.Name)
	return nil
}

// DatabaseForwards drops the table.
func (o DeleteModel) DatabaseForwards(ctx context.Context, app string, e *SchemaEditor, from, _ *ProjectState) error {
	ms, err := from.mustModel(app, o.Name)
	if err != nil {
		return err
	}
	return e.deleteModel(ctx, from, ms)
}

// DatabaseBackwards creates the table again. The rows are not restored.
func (o DeleteModel) DatabaseBackwards(ctx context.Context, app string, e *SchemaEditor, _, to *ProjectState) error {
	ms, err := to.mustModel(app, o.Name)
	if err != nil {
		return err
	}
	return e.createModel(ctx, to, ms)
}

// Describe returns Django's description.
func (o DeleteModel) Describe() string { return "Delete model " + o.Name }

// Category marks the operation as a removal.
func (DeleteModel) Category() Category { return CategoryRemoval }

// NameFragment returns "delete_<model>".
func (o DeleteModel) NameFragment() string { return "delete_" + lowerName(o.Name) }

// RenameModel renames a model. A table with the default name is renamed with
// it; a table set through Meta keeps its name. Foreign keys of other models
// follow the model.
type RenameModel struct {
	OldName string
	NewName string
}

// StateForwards renames the model and repoints the relations to it.
func (o RenameModel) StateForwards(app string, state *ProjectState) error {
	ms, err := state.mustModel(app, o.OldName)
	if err != nil {
		return err
	}
	if _, exists := state.Model(app, o.NewName); exists {
		return fmt.Errorf("migrations: model %s already exists in the migration state", ModelLabel(app, o.NewName))
	}
	oldLabel, newLabel := ms.Label(), ModelLabel(app, o.NewName)
	renamed := ms.clone()
	renamed.Name = o.NewName
	if ms.Table == defaultTable(app, o.OldName) {
		renamed.Table = defaultTable(app, o.NewName)
	}
	// Keep the model at its position, so the creation order stays stable.
	state.Models[newLabel] = renamed
	delete(state.Models, oldLabel)
	if i := slices.Index(state.Order, oldLabel); i >= 0 {
		state.Order[i] = newLabel
	}
	for _, ref := range state.referencing(oldLabel) {
		if ref.field.RelTo == oldLabel {
			ref.field.RelTo = newLabel
		}
		if ref.field.RelThrough == oldLabel {
			ref.field.RelThrough = newLabel
		}
	}
	return nil
}

// DatabaseForwards renames the table when its name changes.
func (o RenameModel) DatabaseForwards(ctx context.Context, app string, e *SchemaEditor, from, to *ProjectState) error {
	return o.rename(ctx, app, e, from, to, o.OldName, o.NewName)
}

// DatabaseBackwards renames the table back.
func (o RenameModel) DatabaseBackwards(ctx context.Context, app string, e *SchemaEditor, from, to *ProjectState) error {
	return o.rename(ctx, app, e, from, to, o.NewName, o.OldName)
}

// rename moves the table of the model named oldName in from to the table of
// the model named newName in to.
func (RenameModel) rename(ctx context.Context, app string, e *SchemaEditor, from, to *ProjectState, oldName, newName string) error {
	oldMS, err := from.mustModel(app, oldName)
	if err != nil {
		return err
	}
	newMS, err := to.mustModel(app, newName)
	if err != nil {
		return err
	}
	return e.alterTable(ctx, from, to, oldMS, newMS)
}

// Describe returns Django's description.
func (o RenameModel) Describe() string { return "Rename model " + o.OldName + " to " + o.NewName }

// Category marks the operation as an alteration.
func (RenameModel) Category() Category { return CategoryAlteration }

// NameFragment returns "rename_<old>_<new>".
func (o RenameModel) NameFragment() string {
	return "rename_" + lowerName(o.OldName) + "_" + lowerName(o.NewName)
}

// AlterModelTable renames the table of a model (a changed Meta.Table).
type AlterModelTable struct {
	Name  string
	Table string
}

// StateForwards sets the table name.
func (o AlterModelTable) StateForwards(app string, state *ProjectState) error {
	ms, err := state.mustModel(app, o.Name)
	if err != nil {
		return err
	}
	ms.Table = o.Table
	if ms.Table == "" {
		ms.Table = defaultTable(app, o.Name)
	}
	return nil
}

// DatabaseForwards renames the table.
func (o AlterModelTable) DatabaseForwards(ctx context.Context, app string, e *SchemaEditor, from, to *ProjectState) error {
	oldMS, newMS, err := modelPair(app, o.Name, from, to)
	if err != nil {
		return err
	}
	return e.alterTable(ctx, from, to, oldMS, newMS)
}

// DatabaseBackwards renames the table back.
func (o AlterModelTable) DatabaseBackwards(ctx context.Context, app string, e *SchemaEditor, from, to *ProjectState) error {
	return o.DatabaseForwards(ctx, app, e, from, to)
}

// Describe returns Django's description.
func (o AlterModelTable) Describe() string {
	table := o.Table
	if table == "" {
		table = "(default)"
	}
	return "Rename table for " + lowerName(o.Name) + " to " + table
}

// Category marks the operation as an alteration.
func (AlterModelTable) Category() Category { return CategoryAlteration }

// NameFragment returns "alter_<model>_table".
func (o AlterModelTable) NameFragment() string { return "alter_" + lowerName(o.Name) + "_table" }

// AlterUniqueTogether replaces the unique-together sets of a model (Django's
// unique_together option). Each set lists Go field names. The operation was
// chosen over AddConstraint and RemoveConstraint because it maps directly to
// a set-valued model option: the autodetector diffs the sets and no
// constraint needs a name chosen by the developer.
type AlterUniqueTogether struct {
	Name           string
	UniqueTogether [][]string
}

// StateForwards replaces the sets.
func (o AlterUniqueTogether) StateForwards(app string, state *ProjectState) error {
	ms, err := state.mustModel(app, o.Name)
	if err != nil {
		return err
	}
	for _, set := range o.UniqueTogether {
		if _, err := ms.columns(set); err != nil {
			return err
		}
	}
	ms.UniqueTogether = cloneFieldSets(o.UniqueTogether)
	return nil
}

// DatabaseForwards drops the constraints of removed sets and adds those of new sets.
func (o AlterUniqueTogether) DatabaseForwards(ctx context.Context, app string, e *SchemaEditor, from, to *ProjectState) error {
	oldMS, newMS, err := modelPair(app, o.Name, from, to)
	if err != nil {
		return err
	}
	return e.alterUniqueTogether(ctx, oldMS, newMS)
}

// DatabaseBackwards restores the previous sets.
func (o AlterUniqueTogether) DatabaseBackwards(ctx context.Context, app string, e *SchemaEditor, from, to *ProjectState) error {
	return o.DatabaseForwards(ctx, app, e, from, to)
}

// Describe returns Django's description.
func (o AlterUniqueTogether) Describe() string {
	return fmt.Sprintf("Alter unique_together for %s (%d constraint(s))", lowerName(o.Name), len(o.UniqueTogether))
}

// Category marks the operation as an alteration.
func (AlterUniqueTogether) Category() Category { return CategoryAlteration }

// NameFragment returns "alter_<model>_unique_together".
func (o AlterUniqueTogether) NameFragment() string {
	return "alter_" + lowerName(o.Name) + "_unique_together"
}

// AddIndex adds a named index to a model.
type AddIndex struct {
	Model string
	Index Index
}

// StateForwards adds the index to the model state.
func (o AddIndex) StateForwards(app string, state *ProjectState) error {
	if o.Index.Name == "" {
		return errors.New("migrations: indexes passed to AddIndex operations require a name argument")
	}
	ms, err := state.mustModel(app, o.Model)
	if err != nil {
		return err
	}
	if _, err := ms.indexColumns(o.Index.Fields); err != nil {
		return err
	}
	if _, exists := ms.IndexByName(o.Index.Name); exists {
		return fmt.Errorf("migrations: %s already has an index named %q", ms.Label(), o.Index.Name)
	}
	ms.Indexes = append(ms.Indexes, Index{Name: o.Index.Name, Fields: slices.Clone(o.Index.Fields)})
	return nil
}

// DatabaseForwards creates the index.
func (o AddIndex) DatabaseForwards(ctx context.Context, app string, e *SchemaEditor, _, to *ProjectState) error {
	ms, err := to.mustModel(app, o.Model)
	if err != nil {
		return err
	}
	return e.addIndex(ctx, ms, o.Index)
}

// DatabaseBackwards drops the index.
func (o AddIndex) DatabaseBackwards(ctx context.Context, _ string, e *SchemaEditor, _, _ *ProjectState) error {
	return e.removeIndex(ctx, o.Index.Name)
}

// Describe returns Django's description.
func (o AddIndex) Describe() string {
	attrs := make([]string, len(o.Index.Fields))
	for i, f := range o.Index.Fields {
		attrs[i] = snakeCase(strings.TrimPrefix(f, "-"))
	}
	return fmt.Sprintf("Create index %s on field(s) %s of model %s",
		o.Index.Name, strings.Join(attrs, ", "), lowerName(o.Model))
}

// Category marks the operation as an addition.
func (AddIndex) Category() Category { return CategoryAddition }

// NameFragment returns "<model>_<index>".
func (o AddIndex) NameFragment() string {
	return lowerName(o.Model) + "_" + strings.ToLower(o.Index.Name)
}

// RemoveIndex drops a named index of a model.
type RemoveIndex struct {
	Model string
	Name  string
}

// StateForwards removes the index from the model state.
func (o RemoveIndex) StateForwards(app string, state *ProjectState) error {
	ms, err := state.mustModel(app, o.Model)
	if err != nil {
		return err
	}
	i := slices.IndexFunc(ms.Indexes, func(idx Index) bool { return idx.Name == o.Name })
	if i < 0 {
		return fmt.Errorf("migrations: %s has no index named %q", ms.Label(), o.Name)
	}
	ms.Indexes = slices.Delete(ms.Indexes, i, i+1)
	return nil
}

// DatabaseForwards drops the index.
func (o RemoveIndex) DatabaseForwards(ctx context.Context, _ string, e *SchemaEditor, _, _ *ProjectState) error {
	return e.removeIndex(ctx, o.Name)
}

// DatabaseBackwards creates the index again from its definition in the state.
func (o RemoveIndex) DatabaseBackwards(ctx context.Context, app string, e *SchemaEditor, _, to *ProjectState) error {
	ms, err := to.mustModel(app, o.Model)
	if err != nil {
		return err
	}
	idx, ok := ms.IndexByName(o.Name)
	if !ok {
		return fmt.Errorf("migrations: %s has no index named %q", ms.Label(), o.Name)
	}
	return e.addIndex(ctx, ms, *idx)
}

// Describe returns Django's description.
func (o RemoveIndex) Describe() string {
	return "Remove index " + o.Name + " from " + lowerName(o.Model)
}

// Category marks the operation as a removal.
func (RemoveIndex) Category() Category { return CategoryRemoval }

// NameFragment returns "remove_<model>_<index>".
func (o RemoveIndex) NameFragment() string {
	return "remove_" + lowerName(o.Model) + "_" + strings.ToLower(o.Name)
}
