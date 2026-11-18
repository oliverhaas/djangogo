package migrations

import (
	"context"
	"fmt"
	"regexp"
	"time"
)

// Special dependency names, as in Django: FirstMigration resolves to the first
// migration of the named app, LatestMigration to its last one.
const (
	FirstMigration  = "__first__"
	LatestMigration = "__latest__"
)

// Migration is one named step in the history of an app: a list of operations
// plus the migrations that must be applied before it. It is the counterpart of
// a Django Migration class.
type Migration struct {
	// App is the app label the migration belongs to.
	App string
	// Name is the migration name, e.g. "0001_initial".
	Name string
	// Dependencies lists the migrations that must be applied first. They may
	// belong to other apps.
	Dependencies []Key
	// RunBefore lists migrations that must be applied after this one. Use it
	// when the other migration cannot name this one as a dependency.
	RunBefore []Key
	// Initial marks the first migration of an app.
	Initial bool
	// Operations are applied in order, and unapplied in reverse order.
	Operations []Operation
}

// Key returns the (app, name) pair that identifies the migration.
func (m *Migration) Key() Key { return Key{App: m.App, Name: m.Name} }

// String returns "app.name".
func (m *Migration) String() string { return m.App + "." + m.Name }

// MutateState applies the state change of every operation to state, in place.
func (m *Migration) MutateState(state *ProjectState) error {
	for _, op := range m.Operations {
		if err := op.StateForwards(m.App, state); err != nil {
			return fmt.Errorf("migrations: %s: %s: %w", m, op.Describe(), err)
		}
	}
	return nil
}

// apply runs the migration forwards through the schema editor. state is the
// project state before the migration and is advanced in place. In collect
// mode each operation is preceded by a comment with its description, as
// Django's sqlmigrate prints it.
func (m *Migration) apply(ctx context.Context, state *ProjectState, e *SchemaEditor) error {
	for _, op := range m.Operations {
		before := 0
		if e.collecting {
			e.comment("--", "-- "+op.Describe(), "--")
			if !reducesToSQL(op) {
				e.comment("-- THIS OPERATION CANNOT BE WRITTEN AS SQL")
				if err := op.StateForwards(m.App, state); err != nil {
					return fmt.Errorf("migrations: %s: %s: %w", m, op.Describe(), err)
				}
				continue
			}
			before = len(e.collected)
		}
		old := state.Clone()
		if err := op.StateForwards(m.App, state); err != nil {
			return fmt.Errorf("migrations: %s: %s: %w", m, op.Describe(), err)
		}
		if err := op.DatabaseForwards(ctx, m.App, e, old, state); err != nil {
			return fmt.Errorf("migrations: %s: %s: %w", m, op.Describe(), err)
		}
		if e.collecting && before == len(e.collected) {
			e.comment("-- (no-op)")
		}
	}
	return nil
}

// unapply runs the migration backwards. state is the project state before the
// migration. Every operation needs the states on both of its sides, so they
// are computed first; the operations then run in reverse order.
func (m *Migration) unapply(ctx context.Context, state *ProjectState, e *SchemaEditor) error {
	type step struct {
		op       Operation
		old, new *ProjectState
	}
	steps := make([]step, 0, len(m.Operations))
	cur := state
	for _, op := range m.Operations {
		if !isReversible(op) {
			return &IrreversibleError{Operation: op.Describe(), Migration: m.Key()}
		}
		next := cur.Clone()
		if err := op.StateForwards(m.App, next); err != nil {
			return fmt.Errorf("migrations: %s: %s: %w", m, op.Describe(), err)
		}
		steps = append(steps, step{op: op, old: cur, new: next})
		cur = next
	}
	for i := len(steps) - 1; i >= 0; i-- {
		s := steps[i]
		before := 0
		if e.collecting {
			e.comment("--", "-- "+s.op.Describe(), "--")
			if !reducesToSQL(s.op) {
				e.comment("-- THIS OPERATION CANNOT BE WRITTEN AS SQL")
				continue
			}
			before = len(e.collected)
		}
		if err := s.op.DatabaseBackwards(ctx, m.App, e, s.new, s.old); err != nil {
			return fmt.Errorf("migrations: %s: undo %s: %w", m, s.op.Describe(), err)
		}
		if e.collecting && before == len(e.collected) {
			e.comment("-- (no-op)")
		}
	}
	return nil
}

// nonWord matches the characters Django replaces in a suggested name.
var nonWord = regexp.MustCompile(`\W+`)

// nameTimestamp returns the timestamp used in a fallback migration name. Tests
// replace it to get a fixed name.
var nameTimestamp = func() string { return time.Now().Format("20060102_1504") }

// SuggestName proposes the part of the migration name after the number,
// derived from the operations as Django does: "initial" for an initial
// migration, otherwise the joined name fragments of the operations, and
// "auto_<timestamp>" when an operation has no fragment.
func (m *Migration) SuggestName() string {
	if m.Initial {
		return "initial"
	}
	var fragments []string
	for _, op := range m.Operations {
		if f := nameFragment(op); f != "" {
			fragments = append(fragments, nonWord.ReplaceAllString(f, "_"))
		}
	}
	if len(fragments) == 0 || len(fragments) != len(m.Operations) {
		return "auto_" + nameTimestamp()
	}
	name := fragments[0]
	for _, f := range fragments[1:] {
		next := name + "_" + f
		if len(next) > 52 {
			name += "_and_more"
			break
		}
		name = next
	}
	return name
}
