package migrations

import (
	"context"
	"time"

	"github.com/oliverhaas/djangogo/orm"
)

// PlanStep is one step of a migration plan: a migration and the direction it
// runs in.
type PlanStep struct {
	Migration *Migration
	// Backwards is true when the migration is unapplied.
	Backwards bool
}

// Progress events reported by the Executor.
const (
	ApplyStart     = "apply_start"
	ApplySuccess   = "apply_success"
	UnapplyStart   = "unapply_start"
	UnapplySuccess = "unapply_success"
)

// Executor plans and runs migrations against a database, the counterpart of
// Django's MigrationExecutor. Every migration is applied or unapplied in its
// own transaction together with its record in the tracking table.
type Executor struct {
	// Loader holds the graph and the applied migrations. The executor keeps
	// its Applied map up to date while it runs.
	Loader *Loader
	// Recorder reads and writes the tracking table.
	Recorder *Recorder
	// Progress, when set, is called at the start and end of every step with
	// one of the progress events.
	Progress func(event string, m *Migration, fake bool)

	db *orm.DB
}

// NewExecutor loads the migrations of reg and the applied migrations of db.
// installedApps has the meaning it has for NewLoader.
func NewExecutor(ctx context.Context, db *orm.DB, reg *Registry, installedApps []string) (*Executor, error) {
	loader, err := NewLoader(ctx, reg, db, installedApps)
	if err != nil {
		return nil, err
	}
	return &Executor{Loader: loader, Recorder: NewRecorder(db), db: db}, nil
}

// MigrationPlan returns the steps that bring the database to the targets.
//
// A target with an empty Name means "unapply every migration of that app". A
// target that is applied is a backward target: the migrations after it in its
// app are unapplied, together with everything that depends on them. Any other
// target is applied with all its unapplied dependencies. With cleanStart the
// plan is made as if nothing was applied.
func (x *Executor) MigrationPlan(targets []Key, cleanStart bool) ([]PlanStep, error) {
	applied := map[Key]bool{}
	if !cleanStart {
		for key := range x.Loader.Applied {
			applied[key] = true
		}
	}
	graph := x.Loader.Graph
	var plan []PlanStep
	unapply := func(from Key) error {
		steps, err := graph.BackwardsPlan(from)
		if err != nil {
			return err
		}
		for _, key := range steps {
			if applied[key] {
				plan = append(plan, PlanStep{Migration: graph.Migration(key), Backwards: true})
				delete(applied, key)
			}
		}
		return nil
	}
	for _, target := range targets {
		switch {
		case target.Name == "":
			for _, root := range graph.RootNodes(target.App) {
				if err := unapply(root); err != nil {
					return nil, err
				}
			}
		case applied[target]:
			// Roll back through the next migrations of the same app only.
			// Going back to the target node itself could undo migrations of
			// other apps that do not need it.
			for _, child := range graph.Children(target) {
				if child.App != target.App {
					continue
				}
				if err := unapply(child); err != nil {
					return nil, err
				}
			}
		default:
			steps, err := graph.ForwardsPlan(target)
			if err != nil {
				return nil, err
			}
			for _, key := range steps {
				if !applied[key] {
					plan = append(plan, PlanStep{Migration: graph.Migration(key)})
					applied[key] = true
				}
			}
		}
	}
	return plan, nil
}

// fullPlan is the forward plan for an empty database: every migration, in
// dependency order.
func (x *Executor) fullPlan() ([]PlanStep, error) {
	return x.MigrationPlan(x.Loader.Graph.LeafNodes(), true)
}

// appliedState returns the state that the applied migrations produce.
func (x *Executor) appliedState(full []PlanStep) (*ProjectState, error) {
	state := NewProjectState()
	for _, step := range full {
		if _, ok := x.Loader.Applied[step.Migration.Key()]; ok {
			if err := step.Migration.MutateState(state); err != nil {
				return nil, err
			}
		}
	}
	return state, nil
}

// Migrate brings the database to the targets and returns the resulting
// project state. With a nil plan it computes the plan from the targets;
// passing a plan lets the caller show it first. A plan that mixes forward and
// backward steps is rejected with an InvalidMigrationPlanError. With fake the
// migrations are only recorded, not run.
func (x *Executor) Migrate(ctx context.Context, targets []Key, plan []PlanStep, fake bool) (*ProjectState, error) {
	if plan == nil {
		var err error
		if plan, err = x.MigrationPlan(targets, false); err != nil {
			return nil, err
		}
	}
	full, err := x.fullPlan()
	if err != nil {
		return nil, err
	}
	if len(plan) == 0 {
		return x.appliedState(full)
	}
	// The tracking table is created only when there is something to record.
	if err := x.Recorder.EnsureSchema(ctx); err != nil {
		return nil, err
	}
	forwards, backwards := 0, 0
	for _, step := range plan {
		if step.Backwards {
			backwards++
		} else {
			forwards++
		}
	}
	switch {
	case forwards > 0 && backwards > 0:
		return nil, &InvalidMigrationPlanError{Plan: plan}
	case forwards > 0:
		return x.migrateForwards(ctx, plan, full, fake)
	default:
		return x.migrateBackwards(ctx, plan, full, fake)
	}
}

// inPlan returns the set of migrations of a plan.
func inPlan(plan []PlanStep) map[Key]bool {
	set := make(map[Key]bool, len(plan))
	for _, step := range plan {
		set[step.Migration.Key()] = true
	}
	return set
}

// migrateForwards applies the planned migrations in the order of the full plan.
func (x *Executor) migrateForwards(ctx context.Context, plan, full []PlanStep, fake bool) (*ProjectState, error) {
	state, err := x.appliedState(full)
	if err != nil {
		return nil, err
	}
	todo := inPlan(plan)
	for _, step := range full {
		if len(todo) == 0 {
			break
		}
		if !todo[step.Migration.Key()] {
			continue
		}
		if err := x.applyMigration(ctx, state, step.Migration, fake); err != nil {
			return nil, err
		}
		delete(todo, step.Migration.Key())
	}
	return state, nil
}

// migrateBackwards unapplies the planned migrations in plan order. The state
// before each of them is computed first, from the applied migrations only.
func (x *Executor) migrateBackwards(ctx context.Context, plan, full []PlanStep, fake bool) (*ProjectState, error) {
	todo := inPlan(plan)
	before := map[Key]*ProjectState{}
	state := NewProjectState()
	for _, step := range full {
		if len(todo) == 0 {
			break
		}
		key := step.Migration.Key()
		_, isApplied := x.Loader.Applied[key]
		switch {
		case todo[key]:
			before[key] = state
			state = state.Clone()
			if err := step.Migration.MutateState(state); err != nil {
				return nil, err
			}
			delete(todo, key)
		case isApplied:
			if err := step.Migration.MutateState(state); err != nil {
				return nil, err
			}
		}
	}
	for _, step := range plan {
		if err := x.unapplyMigration(ctx, before[step.Migration.Key()], step.Migration, fake); err != nil {
			return nil, err
		}
	}
	return x.appliedState(full)
}

// report calls the progress callback when one is set.
func (x *Executor) report(event string, m *Migration, fake bool) {
	if x.Progress != nil {
		x.Progress(event, m, fake)
	}
}

// applyMigration runs one migration forwards and records it, in one
// transaction. state is advanced in place.
func (x *Executor) applyMigration(ctx context.Context, state *ProjectState, m *Migration, fake bool) error {
	x.report(ApplyStart, m, fake)
	if fake {
		if err := m.MutateState(state); err != nil {
			return err
		}
		if err := recordApplied(ctx, x.db.SQL(), x.db.Dialect(), m.Key()); err != nil {
			return err
		}
	} else {
		// Work on a copy, so a failed migration leaves the caller's state as
		// the database is.
		next := state.Clone()
		err := WithSchemaEditor(ctx, x.db, func(e *SchemaEditor) error {
			if err := m.apply(ctx, next, e); err != nil {
				return err
			}
			return recordApplied(ctx, e.run, e.dialect, m.Key())
		})
		if err != nil {
			return err
		}
		*state = *next
	}
	x.Loader.Applied[m.Key()] = time.Now().UTC()
	x.report(ApplySuccess, m, fake)
	return nil
}

// unapplyMigration runs one migration backwards and removes its record, in
// one transaction. state is the state before the migration.
func (x *Executor) unapplyMigration(ctx context.Context, state *ProjectState, m *Migration, fake bool) error {
	x.report(UnapplyStart, m, fake)
	if fake {
		if err := recordUnapplied(ctx, x.db.SQL(), x.db.Dialect(), m.Key()); err != nil {
			return err
		}
	} else {
		err := WithSchemaEditor(ctx, x.db, func(e *SchemaEditor) error {
			if err := m.unapply(ctx, state, e); err != nil {
				return err
			}
			return recordUnapplied(ctx, e.run, e.dialect, m.Key())
		})
		if err != nil {
			return err
		}
	}
	delete(x.Loader.Applied, m.Key())
	x.report(UnapplySuccess, m, fake)
	return nil
}

// Apply registers migs in a fresh registry and applies every unapplied one,
// in dependency order. It returns the applied migrations as "app.name". It is
// a shortcut for tests and scripts; the migrate command uses an Executor.
func Apply(ctx context.Context, db *orm.DB, migs []Migration) ([]string, error) {
	reg := NewRegistry()
	for _, m := range migs {
		reg.Add(m)
	}
	x, err := NewExecutor(ctx, db, reg, nil)
	if err != nil {
		return nil, err
	}
	if err := x.Loader.CheckConsistentHistory(); err != nil {
		return nil, err
	}
	var done []string
	x.Progress = func(event string, m *Migration, _ bool) {
		if event == ApplySuccess {
			done = append(done, m.String())
		}
	}
	if _, err := x.Migrate(ctx, x.Loader.Graph.LeafNodes(), nil, false); err != nil {
		return done, err
	}
	return done, nil
}
