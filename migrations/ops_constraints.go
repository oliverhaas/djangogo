package migrations

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/oliverhaas/djangogo/orm"
)

// AddConstraint adds a check or unique constraint to a model.
type AddConstraint struct {
	Model      string
	Constraint orm.Constraint
}

// StateForwards adds the constraint to the model state.
func (o AddConstraint) StateForwards(app string, state *ProjectState) error {
	if o.Constraint == nil {
		return errors.New("migrations: AddConstraint requires a constraint")
	}
	ms, err := state.mustModel(app, o.Model)
	if err != nil {
		return err
	}
	if _, exists := ms.ConstraintByName(o.Constraint.ConstraintName()); exists {
		return fmt.Errorf("migrations: %s already has a constraint named %q", ms.Label(), o.Constraint.ConstraintName())
	}
	ms.Constraints = append(ms.Constraints, o.Constraint)
	return nil
}

// DatabaseForwards creates the constraint.
func (o AddConstraint) DatabaseForwards(ctx context.Context, app string, e *SchemaEditor, _, to *ProjectState) error {
	ms, err := to.mustModel(app, o.Model)
	if err != nil {
		return err
	}
	return e.addConstraint(ctx, to, ms, o.Constraint)
}

// DatabaseBackwards drops the constraint.
func (o AddConstraint) DatabaseBackwards(ctx context.Context, app string, e *SchemaEditor, from, to *ProjectState) error {
	oldMS, err := from.mustModel(app, o.Model)
	if err != nil {
		return err
	}
	newMS, err := to.mustModel(app, o.Model)
	if err != nil {
		return err
	}
	return e.removeConstraint(ctx, to, oldMS, newMS, o.Constraint)
}

// Describe returns Django's description.
func (o AddConstraint) Describe() string {
	return fmt.Sprintf("Create constraint %s on model %s", constraintName(o.Constraint), lowerName(o.Model))
}

// Category marks the operation as an addition.
func (AddConstraint) Category() Category { return CategoryAddition }

// NameFragment returns "<model>_<constraint>".
func (o AddConstraint) NameFragment() string {
	return lowerName(o.Model) + "_" + strings.ToLower(constraintName(o.Constraint))
}

// RemoveConstraint drops a named constraint of a model.
type RemoveConstraint struct {
	Model string
	Name  string
}

// StateForwards removes the constraint from the model state.
func (o RemoveConstraint) StateForwards(app string, state *ProjectState) error {
	ms, err := state.mustModel(app, o.Model)
	if err != nil {
		return err
	}
	i := slices.IndexFunc(ms.Constraints, func(c orm.Constraint) bool { return c.ConstraintName() == o.Name })
	if i < 0 {
		return fmt.Errorf("migrations: %s has no constraint named %q", ms.Label(), o.Name)
	}
	ms.Constraints = slices.Delete(slices.Clone(ms.Constraints), i, i+1)
	return nil
}

// DatabaseForwards drops the constraint as the from state defines it.
func (o RemoveConstraint) DatabaseForwards(ctx context.Context, app string, e *SchemaEditor, from, to *ProjectState) error {
	oldMS, err := from.mustModel(app, o.Model)
	if err != nil {
		return err
	}
	newMS, err := to.mustModel(app, o.Model)
	if err != nil {
		return err
	}
	c, ok := oldMS.ConstraintByName(o.Name)
	if !ok {
		return fmt.Errorf("migrations: %s has no constraint named %q", oldMS.Label(), o.Name)
	}
	return e.removeConstraint(ctx, to, oldMS, newMS, c)
}

// DatabaseBackwards creates the constraint again from its definition in the
// state.
func (o RemoveConstraint) DatabaseBackwards(ctx context.Context, app string, e *SchemaEditor, _, to *ProjectState) error {
	ms, err := to.mustModel(app, o.Model)
	if err != nil {
		return err
	}
	c, ok := ms.ConstraintByName(o.Name)
	if !ok {
		return fmt.Errorf("migrations: %s has no constraint named %q", ms.Label(), o.Name)
	}
	return e.addConstraint(ctx, to, ms, c)
}

// Describe returns Django's description.
func (o RemoveConstraint) Describe() string {
	return fmt.Sprintf("Remove constraint %s from model %s", o.Name, lowerName(o.Model))
}

// Category marks the operation as a removal.
func (RemoveConstraint) Category() Category { return CategoryRemoval }

// NameFragment returns "remove_<model>_<constraint>".
func (o RemoveConstraint) NameFragment() string {
	return "remove_" + lowerName(o.Model) + "_" + strings.ToLower(o.Name)
}

// AlterConstraint replaces a constraint of a model with one that differs only
// in attributes the database does not hold, such as the violation error
// message. It changes nothing in the database.
type AlterConstraint struct {
	Model      string
	Name       string
	Constraint orm.Constraint
}

// StateForwards replaces the constraint in the model state.
func (o AlterConstraint) StateForwards(app string, state *ProjectState) error {
	if o.Constraint == nil {
		return errors.New("migrations: AlterConstraint requires a constraint")
	}
	ms, err := state.mustModel(app, o.Model)
	if err != nil {
		return err
	}
	i := slices.IndexFunc(ms.Constraints, func(c orm.Constraint) bool { return c.ConstraintName() == o.Name })
	if i < 0 {
		return fmt.Errorf("migrations: %s has no constraint named %q", ms.Label(), o.Name)
	}
	ms.Constraints = slices.Clone(ms.Constraints)
	ms.Constraints[i] = o.Constraint
	return nil
}

// DatabaseForwards does nothing.
func (AlterConstraint) DatabaseForwards(context.Context, string, *SchemaEditor, *ProjectState, *ProjectState) error {
	return nil
}

// DatabaseBackwards does nothing.
func (AlterConstraint) DatabaseBackwards(context.Context, string, *SchemaEditor, *ProjectState, *ProjectState) error {
	return nil
}

// Describe returns Django's description.
func (o AlterConstraint) Describe() string {
	return fmt.Sprintf("Alter constraint %s on %s", o.Name, lowerName(o.Model))
}

// Category marks the operation as an alteration.
func (AlterConstraint) Category() Category { return CategoryAlteration }

// NameFragment returns "alter_<model>_<constraint>".
func (o AlterConstraint) NameFragment() string {
	return "alter_" + lowerName(o.Model) + "_" + strings.ToLower(constraintName(o.Constraint))
}

// constraintName returns the name of c, or "" for a nil constraint.
func constraintName(c orm.Constraint) string {
	if c == nil {
		return ""
	}
	return c.ConstraintName()
}
