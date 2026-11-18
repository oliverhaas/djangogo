package migrations_test

import (
	"context"
	"testing"

	"github.com/oliverhaas/djangogo/migrations"
	"github.com/oliverhaas/djangogo/orm"
)

// The fixtures below are the Go counterparts of the migration modules that
// Django's tests load through MIGRATION_MODULES. Models and fields carry Go
// names; tables and columns are the ones Django creates.

// otherAuthorFields are the columns of Author in migrations.test_migrations
// and of OtherAuthor in the migrations2 fixtures.
func otherAuthorFields() []migrations.FieldState {
	return []migrations.FieldState{
		idField,
		{Name: "Name", Column: "name", Kind: orm.KindChar, MaxLength: 255},
		{Name: "Slug", Column: "slug", Kind: orm.KindChar, MaxLength: 50, Null: true, DBIndex: true},
		{Name: "Age", Column: "age", Kind: orm.KindInt, Default: int64(0)},
		{Name: "SillyField", Column: "silly_field", Kind: orm.KindBool, Default: false},
	}
}

// testMigrations mirrors Django's migrations.test_migrations: 0001_initial
// creates Author and Tribble, 0002_second drops Tribble and adds Book.
func testMigrations() []migrations.Migration {
	author := fkField("Author", "author_id", "migrations.Author")
	author.Null = true
	author.RelOnDelete = orm.OnDeleteSetNull
	return []migrations.Migration{
		{
			App: "migrations", Name: "0001_initial", Initial: true,
			Operations: []migrations.Operation{
				migrations.CreateModel{Name: "Author", Fields: otherAuthorFields()},
				migrations.CreateModel{Name: "Tribble", Fields: []migrations.FieldState{
					idField,
					{Name: "Fluffy", Column: "fluffy", Kind: orm.KindBool, Default: true},
				}},
				migrations.AddField{Model: "Tribble", Field: migrations.FieldState{
					Name: "Bool", Column: "bool", Kind: orm.KindBool, Default: false,
				}},
				migrations.AlterUniqueTogether{Name: "Author", UniqueTogether: [][]string{{"Name", "Slug"}}},
			},
		},
		{
			App: "migrations", Name: "0002_second",
			Dependencies: []migrations.Key{k("migrations", "0001_initial")},
			Operations: []migrations.Operation{
				migrations.DeleteModel{Name: "Tribble"},
				migrations.RemoveField{Model: "Author", Field: "SillyField"},
				migrations.AddField{Model: "Author", Field: migrations.FieldState{
					Name: "Rating", Column: "rating", Kind: orm.KindInt, Default: int64(0),
				}},
				migrations.CreateModel{Name: "Book", Fields: []migrations.FieldState{idField, author}},
			},
		},
	}
}

// testMigrations2 mirrors Django's migrations2.test_migrations_2, whose only
// migration depends on the second migration of the "migrations" app.
func testMigrations2() []migrations.Migration {
	return []migrations.Migration{{
		App: "migrations2", Name: "0001_initial",
		Dependencies: []migrations.Key{k("migrations", "0002_second")},
		Operations:   []migrations.Operation{migrations.CreateModel{Name: "OtherAuthor", Fields: otherAuthorFields()}},
	}}
}

// testMigrations2NoDeps mirrors Django's migrations2.test_migrations_2_no_deps.
func testMigrations2NoDeps() []migrations.Migration {
	return []migrations.Migration{{
		App: "migrations2", Name: "0001_initial",
		Operations: []migrations.Operation{migrations.CreateModel{Name: "OtherAuthor", Fields: otherAuthorFields()}},
	}}
}

// createOnly returns a migration that creates one model with only a primary
// key, plus the given foreign keys.
func createOnly(app, name, model string, deps []migrations.Key, fks ...migrations.FieldState) migrations.Migration {
	fields := append([]migrations.FieldState{idField}, fks...)
	return migrations.Migration{
		App: app, Name: name, Dependencies: deps,
		Operations: []migrations.Operation{migrations.CreateModel{Name: model, Fields: fields}},
	}
}

// lookupErrorMigrations mirrors Django's migrations_test_apps lookuperror_a,
// lookuperror_b and lookuperror_c: three apps whose migrations depend on each
// other in both directions.
func lookupErrorMigrations() []migrations.Migration {
	a, b, c := "lookuperror_a", "lookuperror_b", "lookuperror_c"
	return []migrations.Migration{
		createOnly(a, "0001_initial", "A1", nil),
		createOnly(a, "0002_a2", "A2", []migrations.Key{k(a, "0001_initial")}),
		createOnly(a, "0003_a3", "A3",
			[]migrations.Key{k(c, "0002_c2"), k(b, "0002_b2"), k(a, "0002_a2")},
			fkField("B2", "b2_id", b+".B2"), fkField("C2", "c2_id", c+".C2")),
		createOnly(a, "0004_a4", "A4", []migrations.Key{k(a, "0003_a3")}),
		createOnly(b, "0001_initial", "B1", nil),
		createOnly(b, "0002_b2", "B2",
			[]migrations.Key{k(a, "0002_a2"), k(b, "0001_initial")},
			fkField("A1", "a1_id", a+".A1")),
		createOnly(b, "0003_b3", "B3", []migrations.Key{k(b, "0002_b2")}),
		createOnly(c, "0001_initial", "C1", nil),
		createOnly(c, "0002_c2", "C2",
			[]migrations.Key{k(a, "0002_a2"), k(c, "0001_initial")},
			fkField("A1", "a1_id", a+".A1")),
		createOnly(c, "0003_c3", "C3", []migrations.Key{k(c, "0002_c2")}),
	}
}

// stateOnly wraps operations so that they change the migration state but not
// the database, as Django's SeparateDatabaseAndState with only state
// operations does.
func stateOnly(ops ...migrations.Operation) migrations.Operation {
	return migrations.RunSQL{ReverseSQL: []string{}, StateOperations: ops}
}

// mutateStateMigrations mirrors Django's migrations_test_apps mutate_state_a
// and mutate_state_b, whose migrations only change the state.
func mutateStateMigrations() []migrations.Migration {
	return []migrations.Migration{
		{
			App: "mutate_state_a", Name: "0001_initial",
			Dependencies: []migrations.Key{k("mutate_state_b", "0001_initial")},
			Operations: []migrations.Operation{
				stateOnly(migrations.CreateModel{Name: "A", Fields: []migrations.FieldState{idField}}),
			},
		},
		{
			App: "mutate_state_b", Name: "0001_initial",
			Operations: []migrations.Operation{
				stateOnly(migrations.CreateModel{Name: "B", Fields: []migrations.FieldState{idField}}),
			},
		},
		{
			App: "mutate_state_b", Name: "0002_add_field",
			Dependencies: []migrations.Key{k("mutate_state_b", "0001_initial")},
			Operations: []migrations.Operation{
				stateOnly(migrations.AddField{Model: "B", Field: migrations.FieldState{
					Name: "Added", Column: "added", Kind: orm.KindText,
				}}),
			},
		},
	}
}

// registryOf collects migration sets in a fresh registry.
func registryOf(sets ...[]migrations.Migration) *migrations.Registry {
	reg := migrations.NewRegistry()
	for _, set := range sets {
		for _, m := range set {
			reg.Add(m)
		}
	}
	return reg
}

// newExecutor returns an executor over the given migration sets.
func newExecutor(t *testing.T, db *orm.DB, sets ...[]migrations.Migration) *migrations.Executor {
	t.Helper()
	x, err := migrations.NewExecutor(context.Background(), db, registryOf(sets...), nil)
	if err != nil {
		t.Fatalf("NewExecutor: %v", err)
	}
	return x
}

// newLoader returns a loader over the given migration sets. db may be nil.
func newLoader(t *testing.T, db *orm.DB, sets ...[]migrations.Migration) *migrations.Loader {
	t.Helper()
	l, err := migrations.NewLoader(context.Background(), registryOf(sets...), db, nil)
	if err != nil {
		t.Fatalf("NewLoader: %v", err)
	}
	return l
}

// planned is one expected step of a migration plan.
type planned struct {
	Key       migrations.Key
	Backwards bool
}

// forwards is a plan step that applies a migration.
func forwards(app, name string) planned { return planned{Key: k(app, name)} }

// backwards is a plan step that unapplies a migration.
func backwards(app, name string) planned { return planned{Key: k(app, name), Backwards: true} }

// steps reduces a plan to keys and directions, so it compares with ==.
func steps(plan []migrations.PlanStep) []planned {
	out := make([]planned, 0, len(plan))
	for _, s := range plan {
		out = append(out, planned{Key: s.Migration.Key(), Backwards: s.Backwards})
	}
	return out
}

// migrationPlan returns the plan to the targets and fails the test on an error.
func migrationPlan(t *testing.T, x *migrations.Executor, targets ...migrations.Key) []migrations.PlanStep {
	t.Helper()
	p, err := x.MigrationPlan(targets, false)
	if err != nil {
		t.Fatalf("MigrationPlan(%v): %v", targets, err)
	}
	return p
}

// migrate brings the database to the targets and returns the resulting state.
func migrate(t *testing.T, x *migrations.Executor, targets ...migrations.Key) *migrations.ProjectState {
	t.Helper()
	state, err := x.Migrate(context.Background(), targets, nil, false)
	if err != nil {
		t.Fatalf("Migrate(%v): %v", targets, err)
	}
	return state
}

// zero is the target that unapplies every migration of app, Django's
// (app, None).
func zero(app string) migrations.Key { return migrations.Key{App: app} }
