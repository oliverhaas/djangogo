package migrations

import (
	"context"
	"fmt"
	"slices"

	"github.com/oliverhaas/djangogo/orm"
)

// AddField adds a column to a model's table.
//
// A column that is NOT NULL needs a value for the rows that already exist. It
// is taken from the field default; Default supplies a one-off value when the
// field has none (Django's preserve_default=False). The database default is
// dropped again after the column is filled, as Django does.
type AddField struct {
	Model string
	Field FieldState
	// Default is a one-off value for existing rows. It is not part of the state.
	Default any
}

// StateForwards appends the field to the model.
func (o AddField) StateForwards(app string, state *ProjectState) error {
	ms, err := state.mustModel(app, o.Model)
	if err != nil {
		return err
	}
	if _, exists := ms.FieldByName(o.Field.Name); exists {
		return fmt.Errorf("migrations: %s already has a field named %q", ms.Label(), o.Field.Name)
	}
	ms.Fields = append(ms.Fields, o.Field)
	return nil
}

// DatabaseForwards adds the column.
func (o AddField) DatabaseForwards(ctx context.Context, app string, e *SchemaEditor, from, to *ProjectState) error {
	oldMS, newMS, err := modelPair(app, o.Model, from, to)
	if err != nil {
		return err
	}
	return e.addField(ctx, to, oldMS, newMS, o.Field, o.Default)
}

// DatabaseBackwards drops the column.
func (o AddField) DatabaseBackwards(ctx context.Context, app string, e *SchemaEditor, from, to *ProjectState) error {
	oldMS, newMS, err := modelPair(app, o.Model, from, to)
	if err != nil {
		return err
	}
	return e.removeField(ctx, from, to, oldMS, newMS, o.Field)
}

// Describe returns Django's description.
func (o AddField) Describe() string {
	return "Add field " + o.Field.Attr() + " to " + lowerName(o.Model)
}

// Category marks the operation as an addition.
func (AddField) Category() Category { return CategoryAddition }

// NameFragment returns "<model>_<field>".
func (o AddField) NameFragment() string { return lowerName(o.Model) + "_" + o.Field.Attr() }

// RemoveField drops a column from a model's table.
type RemoveField struct {
	Model string
	// Field is the Go name of the field.
	Field string
}

// StateForwards removes the field from the model.
func (o RemoveField) StateForwards(app string, state *ProjectState) error {
	ms, err := state.mustModel(app, o.Model)
	if err != nil {
		return err
	}
	i := slices.IndexFunc(ms.Fields, func(f FieldState) bool { return f.Name == o.Field })
	if i < 0 {
		return errNoField(ms, o.Field)
	}
	ms.Fields = slices.Delete(ms.Fields, i, i+1)
	return nil
}

// DatabaseForwards drops the column.
func (o RemoveField) DatabaseForwards(ctx context.Context, app string, e *SchemaEditor, from, to *ProjectState) error {
	oldMS, newMS, err := modelPair(app, o.Model, from, to)
	if err != nil {
		return err
	}
	f, ok := oldMS.FieldByName(o.Field)
	if !ok {
		return errNoField(oldMS, o.Field)
	}
	return e.removeField(ctx, from, to, oldMS, newMS, *f)
}

// DatabaseBackwards adds the column again. Its values are not restored; a NOT
// NULL column is filled from the field default.
func (o RemoveField) DatabaseBackwards(ctx context.Context, app string, e *SchemaEditor, from, to *ProjectState) error {
	oldMS, newMS, err := modelPair(app, o.Model, from, to)
	if err != nil {
		return err
	}
	f, ok := newMS.FieldByName(o.Field)
	if !ok {
		return errNoField(newMS, o.Field)
	}
	return e.addField(ctx, to, oldMS, newMS, *f, nil)
}

// Describe returns Django's description.
func (o RemoveField) Describe() string {
	return "Remove field " + snakeCase(o.Field) + " from " + lowerName(o.Model)
}

// Category marks the operation as a removal.
func (RemoveField) Category() Category { return CategoryRemoval }

// NameFragment returns "remove_<model>_<field>".
func (o RemoveField) NameFragment() string {
	return "remove_" + lowerName(o.Model) + "_" + snakeCase(o.Field)
}

// AlterField changes the definition of an existing field: type, null, unique,
// index, relation or default. On PostgreSQL the column is altered in place; on
// SQLite the table is rebuilt, keeping rows, indexes and foreign keys.
type AlterField struct {
	Model string
	// Field is the new definition. Its Name selects the field to change.
	Field FieldState
	// Default is a one-off value for rows that hold NULL when the field
	// becomes NOT NULL. It is not part of the state.
	Default any
}

// StateForwards replaces the field definition.
func (o AlterField) StateForwards(app string, state *ProjectState) error {
	ms, err := state.mustModel(app, o.Model)
	if err != nil {
		return err
	}
	f, ok := ms.FieldByName(o.Field.Name)
	if !ok {
		return errNoField(ms, o.Field.Name)
	}
	*f = o.Field
	return nil
}

// DatabaseForwards alters the column.
func (o AlterField) DatabaseForwards(ctx context.Context, app string, e *SchemaEditor, from, to *ProjectState) error {
	return o.alter(ctx, app, e, from, to, o.Default)
}

// DatabaseBackwards restores the previous column definition.
func (o AlterField) DatabaseBackwards(ctx context.Context, app string, e *SchemaEditor, from, to *ProjectState) error {
	return o.alter(ctx, app, e, from, to, nil)
}

// alter changes the column from its definition in from to the one in to.
func (o AlterField) alter(ctx context.Context, app string, e *SchemaEditor, from, to *ProjectState, oneOff any) error {
	oldMS, newMS, err := modelPair(app, o.Model, from, to)
	if err != nil {
		return err
	}
	oldF, ok := oldMS.FieldByName(o.Field.Name)
	if !ok {
		return errNoField(oldMS, o.Field.Name)
	}
	newF, ok := newMS.FieldByName(o.Field.Name)
	if !ok {
		return errNoField(newMS, o.Field.Name)
	}
	return e.alterField(ctx, from, to, oldMS, newMS, *oldF, *newF, oneOff)
}

// Describe returns Django's description.
func (o AlterField) Describe() string {
	return "Alter field " + o.Field.Attr() + " on " + lowerName(o.Model)
}

// Category marks the operation as an alteration.
func (AlterField) Category() Category { return CategoryAlteration }

// NameFragment returns "alter_<model>_<field>".
func (o AlterField) NameFragment() string {
	return "alter_" + lowerName(o.Model) + "_" + o.Field.Attr()
}

// RenameField renames a field and, with it, its column. The data is kept.
type RenameField struct {
	Model   string
	OldName string
	NewName string
	// Column is the new column name. When empty, a column that had the default
	// name of the old field gets the default name of the new field, and a
	// column set through a column= tag keeps its name.
	Column string
}

// StateForwards renames the field and the references to it in the indexes and
// unique-together sets of the model.
func (o RenameField) StateForwards(app string, state *ProjectState) error {
	ms, err := state.mustModel(app, o.Model)
	if err != nil {
		return err
	}
	f, ok := ms.FieldByName(o.OldName)
	if !ok {
		return errNoField(ms, o.OldName)
	}
	if _, exists := ms.FieldByName(o.NewName); exists {
		return fmt.Errorf("migrations: %s already has a field named %q", ms.Label(), o.NewName)
	}
	switch {
	case o.Column != "":
		f.Column = o.Column
	case f.Column == defaultColumn(o.OldName, f.RelKind):
		f.Column = defaultColumn(o.NewName, f.RelKind)
	}
	f.Name = o.NewName
	for i := range ms.Indexes {
		renameIn(ms.Indexes[i].Fields, o.OldName, o.NewName)
	}
	for _, set := range ms.UniqueTogether {
		renameIn(set, o.OldName, o.NewName)
	}
	return nil
}

// defaultColumn returns the column the ORM derives for a field name: the
// snake_case name, with "_id" appended for a foreign key, and "" for M2M.
func defaultColumn(name string, rel orm.RelKind) string {
	if rel == orm.RelM2M {
		return ""
	}
	if rel != orm.RelNone {
		return snakeCase(name) + "_id"
	}
	return snakeCase(name)
}

// renameIn replaces every occurrence of oldName in names, preserving a leading
// "-" prefix on descending index fields.
func renameIn(names []string, oldName, newName string) {
	for i, n := range names {
		switch n {
		case oldName:
			names[i] = newName
		case "-" + oldName:
			names[i] = "-" + newName
		}
	}
}

// DatabaseForwards renames the column.
func (o RenameField) DatabaseForwards(ctx context.Context, app string, e *SchemaEditor, from, to *ProjectState) error {
	return o.rename(ctx, app, e, from, to, o.OldName, o.NewName)
}

// DatabaseBackwards renames the column back.
func (o RenameField) DatabaseBackwards(ctx context.Context, app string, e *SchemaEditor, from, to *ProjectState) error {
	return o.rename(ctx, app, e, from, to, o.NewName, o.OldName)
}

// rename alters the field named oldName in from into the field named newName in to.
func (o RenameField) rename(ctx context.Context, app string, e *SchemaEditor, from, to *ProjectState, oldName, newName string) error {
	oldMS, newMS, err := modelPair(app, o.Model, from, to)
	if err != nil {
		return err
	}
	oldF, ok := oldMS.FieldByName(oldName)
	if !ok {
		return errNoField(oldMS, oldName)
	}
	newF, ok := newMS.FieldByName(newName)
	if !ok {
		return errNoField(newMS, newName)
	}
	return e.alterField(ctx, from, to, oldMS, newMS, *oldF, *newF, nil)
}

// Describe returns Django's description.
func (o RenameField) Describe() string {
	return "Rename field " + snakeCase(o.OldName) + " on " + lowerName(o.Model) + " to " + snakeCase(o.NewName)
}

// Category marks the operation as an alteration.
func (RenameField) Category() Category { return CategoryAlteration }

// NameFragment returns "rename_<old>_<model>_<new>".
func (o RenameField) NameFragment() string {
	return "rename_" + snakeCase(o.OldName) + "_" + lowerName(o.Model) + "_" + snakeCase(o.NewName)
}
