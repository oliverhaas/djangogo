package migrations

import (
	"reflect"
	"slices"

	"github.com/oliverhaas/djangogo/orm"
)

// constraintChange lists the constraints a kept model gains, loses and
// alters.
type constraintChange struct {
	key                     modelKey
	added, removed, altered []orm.Constraint
}

// sameConstraint reports whether a and b define the same constraint.
func sameConstraint(a, b orm.Constraint) bool {
	srcA, okA := constraintSource(a)
	srcB, okB := constraintSource(b)
	if okA && okB {
		return srcA == srcB
	}
	return reflect.DeepEqual(a, b)
}

// withoutViolation returns c without the attributes that the database does
// not hold (Django's non_db_attrs).
func withoutViolation(c orm.Constraint) orm.Constraint {
	switch x := c.(type) {
	case orm.CheckConstraint:
		x.ViolationErrorCode, x.ViolationErrorMessage = "", ""
		return x
	case *orm.CheckConstraint:
		if x != nil {
			return withoutViolation(*x)
		}
	case orm.UniqueConstraint:
		x.ViolationErrorCode, x.ViolationErrorMessage = "", ""
		return x
	case *orm.UniqueConstraint:
		if x != nil {
			return withoutViolation(*x)
		}
	}
	return c
}

// recreatesConstraint reports whether the change from oldC to newC needs the
// database constraint dropped and created again: anything but the violation
// attributes differs. A field of a unique constraint renamed with its column
// kept counts under its old name (Django's
// _constraint_should_be_dropped_and_recreated).
func (a *Autodetector) recreatesConstraint(key modelKey, oldC, newC orm.Constraint) bool {
	oldC, newC = withoutViolation(oldC), withoutViolation(newC)
	if u, ok := newC.(orm.UniqueConstraint); ok && len(u.Fields) > 0 {
		fields := slices.Clone(u.Fields)
		for _, r := range a.renames {
			if r.key.app != key.app || r.key.model != key.name || r.oldField.Column != r.newField.Column {
				continue
			}
			if i := slices.Index(fields, r.key.field); i >= 0 {
				fields[i] = r.oldName
			}
		}
		u.Fields = fields
		newC = u
	}
	return !sameConstraint(oldC, newC)
}

// createAlteredConstraints compares the constraints of the kept models. A
// constraint that keeps its name and changes only in what the database does
// not hold is altered; any other difference removes the old constraint and
// adds the new one.
func (a *Autodetector) createAlteredConstraints() {
	for _, key := range a.keptModels {
		oldConstraints := a.oldModel(key).Constraints
		newConstraints := a.to.Models[key.label()].Constraints
		change := constraintChange{key: key}
		alteredNames := map[string]bool{}
		for _, oldC := range oldConstraints {
			for _, newC := range newConstraints {
				if oldC.ConstraintName() == newC.ConstraintName() && !sameConstraint(oldC, newC) &&
					!a.recreatesConstraint(key, oldC, newC) {
					change.altered = append(change.altered, newC)
					alteredNames[newC.ConstraintName()] = true
				}
			}
		}
		contains := func(list []orm.Constraint, c orm.Constraint) bool {
			return slices.ContainsFunc(list, func(o orm.Constraint) bool { return sameConstraint(o, c) })
		}
		for _, c := range newConstraints {
			if !contains(oldConstraints, c) && !alteredNames[c.ConstraintName()] {
				change.added = append(change.added, c)
			}
		}
		for _, c := range oldConstraints {
			if !contains(newConstraints, c) && !alteredNames[c.ConstraintName()] {
				change.removed = append(change.removed, c)
			}
		}
		if len(change.added) > 0 || len(change.removed) > 0 || len(change.altered) > 0 {
			a.constraintChanges = append(a.constraintChanges, change)
		}
	}
}

// generateRemovedConstraints generates RemoveConstraint. It runs before the
// fields are removed.
func (a *Autodetector) generateRemovedConstraints() {
	for _, change := range a.constraintChanges {
		for _, c := range change.removed {
			a.addOperation(change.key.app, RemoveConstraint{Model: change.key.name, Name: c.ConstraintName()}, nil, false)
		}
	}
}

// generateAddedConstraints generates AddConstraint. It runs after the fields
// are added.
func (a *Autodetector) generateAddedConstraints() {
	for _, change := range a.constraintChanges {
		deps := modelDeps(a.to.Models[change.key.label()])
		for _, c := range change.added {
			a.addOperation(change.key.app, AddConstraint{Model: change.key.name, Constraint: c}, deps, false)
		}
	}
}

// generateAlteredConstraints generates AlterConstraint.
func (a *Autodetector) generateAlteredConstraints() {
	for _, change := range a.constraintChanges {
		deps := modelDeps(a.to.Models[change.key.label()])
		for _, c := range change.altered {
			a.addOperation(change.key.app,
				AlterConstraint{Model: change.key.name, Name: c.ConstraintName(), Constraint: c}, deps, false)
		}
	}
}
