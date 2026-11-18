package migrations

import (
	"context"
	"errors"

	"github.com/oliverhaas/djangogo/orm"
)

// RunSQL runs raw SQL statements (Django's RunSQL).
//
// ReverseSQL holds the statements that undo SQL. With a nil ReverseSQL the
// operation is irreversible, and unapplying its migration fails. A non-nil
// empty ReverseSQL means that there is nothing to undo.
//
// The statements are executed as written. They are part of the migration
// source, so never build them from user input.
type RunSQL struct {
	SQL        []string
	ReverseSQL []string
	// StateOperations describe the effect of the SQL on the models, so later
	// migrations see the right state. They change the state only.
	StateOperations []Operation
}

// StateForwards applies the state operations.
func (o RunSQL) StateForwards(app string, state *ProjectState) error {
	for _, op := range o.StateOperations {
		if err := op.StateForwards(app, state); err != nil {
			return err
		}
	}
	return nil
}

// DatabaseForwards executes SQL.
func (o RunSQL) DatabaseForwards(ctx context.Context, _ string, e *SchemaEditor, _, _ *ProjectState) error {
	return o.run(ctx, e, o.SQL)
}

// DatabaseBackwards executes ReverseSQL.
func (o RunSQL) DatabaseBackwards(ctx context.Context, _ string, e *SchemaEditor, _, _ *ProjectState) error {
	if o.ReverseSQL == nil {
		return errors.New("migrations: you cannot reverse this operation")
	}
	return o.run(ctx, e, o.ReverseSQL)
}

// run executes the statements in order.
func (RunSQL) run(ctx context.Context, e *SchemaEditor, stmts []string) error {
	for _, s := range stmts {
		if s == "" {
			continue
		}
		if err := e.Execute(ctx, s); err != nil {
			return err
		}
	}
	return nil
}

// Describe returns Django's description.
func (RunSQL) Describe() string { return "Raw SQL operation" }

// Reversible reports whether ReverseSQL is set.
func (o RunSQL) Reversible() bool { return o.ReverseSQL != nil }

// Category marks the operation as raw SQL.
func (RunSQL) Category() Category { return CategorySQL }

// GoFunc is the signature of a data migration function. db is bound to the
// transaction of the running migration: ORM queries through it and raw SQL
// through db.SQL() are committed or rolled back together with the migration.
// state is the project state at this point of the history. Work with the
// tables and columns state describes, not with the current Go models, which
// may be ahead of it.
type GoFunc func(ctx context.Context, db *orm.DB, state *ProjectState) error

// RunGo runs Go code inside a migration, the counterpart of Django's
// RunPython. Use it for data migrations.
//
// With a nil Reverse the operation is irreversible, and unapplying its
// migration fails. Set Reverse to Noop when there is nothing to undo.
type RunGo struct {
	Forward GoFunc
	Reverse GoFunc
}

// Noop is a GoFunc that does nothing (Django's RunPython.noop).
func Noop(context.Context, *orm.DB, *ProjectState) error { return nil }

// StateForwards changes nothing: RunGo does not alter the schema.
func (RunGo) StateForwards(string, *ProjectState) error { return nil }

// DatabaseForwards calls Forward with the state before the operation.
func (o RunGo) DatabaseForwards(ctx context.Context, _ string, e *SchemaEditor, from, _ *ProjectState) error {
	if o.Forward == nil {
		return errors.New("migrations: RunGo requires a Forward function")
	}
	db, err := e.DB()
	if err != nil {
		return err
	}
	return o.Forward(ctx, db, from)
}

// DatabaseBackwards calls Reverse with the state after the operation.
func (o RunGo) DatabaseBackwards(ctx context.Context, _ string, e *SchemaEditor, from, _ *ProjectState) error {
	if o.Reverse == nil {
		return errors.New("migrations: you cannot reverse this operation")
	}
	db, err := e.DB()
	if err != nil {
		return err
	}
	return o.Reverse(ctx, db, from)
}

// Describe returns the description. Django says "Raw Python operation".
func (RunGo) Describe() string { return "Raw Go operation" }

// Reversible reports whether Reverse is set.
func (o RunGo) Reversible() bool { return o.Reverse != nil }

// ReducesToSQL reports false: Go code cannot be shown as SQL.
func (RunGo) ReducesToSQL() bool { return false }

// Category marks the operation as Go code.
func (RunGo) Category() Category { return CategoryGo }
