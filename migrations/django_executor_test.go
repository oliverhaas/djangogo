package migrations_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/migrations"
	"github.com/oliverhaas/djangogo/orm"
)

// Django rebuilds the loader graph after every migrate call to pick up the new
// database state. The Go executor keeps Loader.Applied current while it runs,
// so the ports below go on with the same executor.

// Ported from Django: tests/migrations/test_executor.py::ExecutorTests.test_run
func TestRun(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		x := newExecutor(t, db, testMigrations())
		djangotest.AssertSequenceEqual(t, steps(migrationPlan(t, x, k("migrations", "0002_second"))), []planned{
			forwards("migrations", "0001_initial"),
			forwards("migrations", "0002_second"),
		})
		djangotest.AssertTableNotExists(t, db, "migrations_author")
		djangotest.AssertTableNotExists(t, db, "migrations_book")
		migrate(t, x, k("migrations", "0002_second"))
		djangotest.AssertTableExists(t, db, "migrations_author")
		djangotest.AssertTableExists(t, db, "migrations_book")
		djangotest.AssertSequenceEqual(t, steps(migrationPlan(t, x, zero("migrations"))), []planned{
			backwards("migrations", "0002_second"),
			backwards("migrations", "0001_initial"),
		})
		migrate(t, x, zero("migrations"))
		djangotest.AssertTableNotExists(t, db, "migrations_author")
		djangotest.AssertTableNotExists(t, db, "migrations_book")
	})
}

// Ported from Django: tests/migrations/test_executor.py::ExecutorTests.test_empty_plan
func TestEmptyPlan(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		x := newExecutor(t, db, testMigrations(), testMigrations2())
		targets := []migrations.Key{k("migrations", "0002_second"), k("migrations2", "0001_initial")}
		djangotest.AssertSequenceEqual(t, steps(migrationPlan(t, x, targets...)), []planned{
			forwards("migrations", "0001_initial"),
			forwards("migrations", "0002_second"),
			forwards("migrations2", "0001_initial"),
		})
		// Fake-apply all migrations.
		if _, err := x.Migrate(ctx, targets, nil, true); err != nil {
			t.Fatalf("fake migrate: %v", err)
		}
		djangotest.AssertTableNotExists(t, db, "migrations_author")
		// A second plan is empty.
		djangotest.AssertEqual(t, len(migrationPlan(t, x, targets...)), 0)
		// The resulting state includes the applied migrations.
		state := migrate(t, x, targets...)
		modelOf(t, state, "migrations", "Book")
		modelOf(t, state, "migrations", "Author")
		modelOf(t, state, "migrations2", "OtherAuthor")
		// Erase the fake records.
		for _, key := range []migrations.Key{k("migrations2", "0001_initial"), k("migrations", "0002_second"), k("migrations", "0001_initial")} {
			if err := x.Recorder.RecordUnapplied(ctx, key); err != nil {
				t.Fatalf("RecordUnapplied(%v): %v", key, err)
			}
		}
		applied, err := x.Recorder.AppliedMigrations(ctx)
		if err != nil {
			t.Fatalf("AppliedMigrations: %v", err)
		}
		djangotest.AssertEqual(t, len(applied), 0)
	})
}

// Ported from Django: tests/migrations/test_executor.py::ExecutorTests.test_mixed_plan_not_supported
func TestMixedPlanNotSupported(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		x := newExecutor(t, db, testMigrations(), testMigrations2NoDeps())
		plan := migrationPlan(t, x, k("migrations", "0002_second"))
		djangotest.AssertSequenceEqual(t, steps(plan), []planned{
			forwards("migrations", "0001_initial"),
			forwards("migrations", "0002_second"),
		})
		if _, err := x.Migrate(ctx, nil, plan, false); err != nil {
			t.Fatalf("Migrate: %v", err)
		}
		applied, err := x.Recorder.AppliedMigrations(ctx)
		if err != nil {
			t.Fatalf("AppliedMigrations: %v", err)
		}
		for key, want := range map[migrations.Key]bool{
			k("migrations", "0001_initial"):  true,
			k("migrations", "0002_second"):   true,
			k("migrations2", "0001_initial"): false,
		} {
			_, inDB := applied[key]
			_, inLoader := x.Loader.Applied[key]
			if inDB != want || inLoader != want {
				t.Errorf("%v applied: database %v, loader %v, want %v", key, inDB, inLoader, want)
			}
		}

		// Generate a mixed plan.
		plan = migrationPlan(t, x, zero("migrations"), k("migrations2", "0001_initial"))
		_, err = x.Migrate(ctx, nil, plan, false)
		var invalid *migrations.InvalidMigrationPlanError
		if !errors.As(err, &invalid) {
			t.Fatalf("Migrate error = %v, want InvalidMigrationPlanError", err)
		}
		djangotest.AssertEqual(t, invalid.Error(),
			"Migration plans with both forwards and backwards migrations are "+
				"not supported. Please split your migration process into separate "+
				"plans of only forwards OR backwards migrations.")
		djangotest.AssertSequenceEqual(t, steps(invalid.Plan), []planned{
			backwards("migrations", "0002_second"),
			backwards("migrations", "0001_initial"),
			forwards("migrations2", "0001_initial"),
		})

		migrate(t, x, zero("migrations"), zero("migrations2"))
		djangotest.AssertTableNotExists(t, db, "migrations_author")
		djangotest.AssertTableNotExists(t, db, "migrations_book")
		djangotest.AssertTableNotExists(t, db, "migrations2_otherauthor")
	})
}

// Ported from Django: tests/migrations/test_executor.py::ExecutorTests.test_unrelated_model_lookups_forwards
func TestUnrelatedModelLookupsForwards(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		x := newExecutor(t, db, lookupErrorMigrations())
		djangotest.AssertTableNotExists(t, db, "lookuperror_a_a1")
		djangotest.AssertTableNotExists(t, db, "lookuperror_b_b1")
		djangotest.AssertTableNotExists(t, db, "lookuperror_c_c1")
		migrate(t, x, k("lookuperror_b", "0003_b3"))
		djangotest.AssertTableExists(t, db, "lookuperror_b_b3")

		// lookuperror_b.B2 is already applied when these run.
		migrate(t, x, k("lookuperror_a", "0004_a4"), k("lookuperror_c", "0003_c3"))
		djangotest.AssertTableExists(t, db, "lookuperror_a_a4")
		djangotest.AssertTableExists(t, db, "lookuperror_c_c3")

		migrate(t, x, zero("lookuperror_a"), zero("lookuperror_b"), zero("lookuperror_c"))
		djangotest.AssertTableNotExists(t, db, "lookuperror_a_a1")
		djangotest.AssertTableNotExists(t, db, "lookuperror_b_b1")
		djangotest.AssertTableNotExists(t, db, "lookuperror_c_c1")
	})
}

// Ported from Django: tests/migrations/test_executor.py::ExecutorTests.test_unrelated_model_lookups_backwards
func TestUnrelatedModelLookupsBackwards(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		x := newExecutor(t, db, lookupErrorMigrations())
		djangotest.AssertTableNotExists(t, db, "lookuperror_a_a1")
		djangotest.AssertTableNotExists(t, db, "lookuperror_b_b1")
		djangotest.AssertTableNotExists(t, db, "lookuperror_c_c1")
		migrate(t, x, k("lookuperror_a", "0004_a4"), k("lookuperror_b", "0003_b3"), k("lookuperror_c", "0003_c3"))
		djangotest.AssertTableExists(t, db, "lookuperror_b_b3")
		djangotest.AssertTableExists(t, db, "lookuperror_a_a4")
		djangotest.AssertTableExists(t, db, "lookuperror_c_c3")

		// lookuperror_b.B2 is unrelated to app c, but is part of the state
		// the backward steps start from.
		migrate(t, x, zero("lookuperror_a"))

		migrate(t, x, zero("lookuperror_b"), zero("lookuperror_c"))
		djangotest.AssertTableNotExists(t, db, "lookuperror_a_a1")
		djangotest.AssertTableNotExists(t, db, "lookuperror_b_b1")
		djangotest.AssertTableNotExists(t, db, "lookuperror_c_c1")
	})
}

// Ported from Django: tests/migrations/test_executor.py::ExecutorTests.test_unrelated_applied_migrations_mutate_state
func TestUnrelatedAppliedMigrationsMutateState(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		x := newExecutor(t, db, mutateStateMigrations())
		migrate(t, x, k("mutate_state_b", "0002_add_field"))
		// Migrate forward.
		state := migrate(t, x, k("mutate_state_a", "0001_initial"))
		fieldOf(t, state, "mutate_state_b", "B", "Added")
		// Migrate backward.
		state = migrate(t, x, zero("mutate_state_a"))
		fieldOf(t, state, "mutate_state_b", "B", "Added")
		migrate(t, x, zero("mutate_state_b"))
	})
}

// Django also reports "render_start" and "render_success" around the model
// rendering. The Go executor renders no model classes and reports the apply
// and unapply events only.
//
// Ported from Django: tests/migrations/test_executor.py::ExecutorTests.test_process_callback
func TestProcessCallback(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		type call struct {
			event string
			key   migrations.Key
			fake  bool
		}
		var calls []call
		x := newExecutor(t, db, testMigrations())
		x.Progress = func(event string, m *migrations.Migration, fake bool) {
			calls = append(calls, call{event: event, key: m.Key(), fake: fake})
		}
		djangotest.AssertTableNotExists(t, db, "migrations_author")
		djangotest.AssertTableNotExists(t, db, "migrations_tribble")
		migrate(t, x, k("migrations", "0001_initial"), k("migrations", "0002_second"))
		migrate(t, x, zero("migrations"), zero("migrations"))
		djangotest.AssertTableNotExists(t, db, "migrations_author")
		djangotest.AssertTableNotExists(t, db, "migrations_tribble")

		first, second := k("migrations", "0001_initial"), k("migrations", "0002_second")
		djangotest.AssertSequenceEqual(t, calls, []call{
			{"apply_start", first, false},
			{"apply_success", first, false},
			{"apply_start", second, false},
			{"apply_success", second, false},
			{"unapply_start", second, false},
			{"unapply_success", second, false},
			{"unapply_start", first, false},
			{"unapply_success", first, false},
		})
	})
}

// Django makes the record fail by patching record_migration. Go has no such
// seam, so the test prepares a tracking table with a check constraint that
// rejects the record after the model table was created.
//
// Ported from Django: tests/migrations/test_executor.py::ExecutorTests.test_migrations_applied_and_recorded_atomically
func TestMigrationsAppliedAndRecordedAtomically(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		mustExec(t, db, `CREATE TABLE "django_migrations" ("id" integer, `+
			`"app" varchar(255) CHECK ("app" <> 'record_migration'), "name" varchar(255), "applied" timestamp)`)
		x := newExecutor(t, db, []migrations.Migration{
			createOnly("record_migration", "0001_initial", "Model", nil),
		})
		_, err := x.Migrate(context.Background(), []migrations.Key{k("record_migration", "0001_initial")}, nil, false)
		if err == nil {
			t.Fatal("Migrate succeeded although the record cannot be written")
		}
		// The migration is not recorded as applied since it failed.
		djangotest.AssertEqual(t, queryInt(t, db,
			`SELECT COUNT(*) FROM "django_migrations" WHERE "app" = ? AND "name" = ?`,
			"record_migration", "0001_initial"), int64(0))
		if _, ok := x.Loader.Applied[k("record_migration", "0001_initial")]; ok {
			t.Error("the loader counts the failed migration as applied")
		}
		djangotest.AssertTableNotExists(t, db, "record_migration_model")
	})
}

// Django counts the queries with has_table patched. The Go port checks the
// effect: the tracking table does not exist after an empty plan.
//
// Ported from Django: tests/migrations/test_executor.py::ExecutorTests.test_migrate_skips_schema_creation
func TestMigrateSkipsSchemaCreation(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		x := newExecutor(t, db, testMigrations())
		if _, err := x.Migrate(context.Background(), nil, []migrations.PlanStep{}, false); err != nil {
			t.Fatalf("Migrate: %v", err)
		}
		djangotest.AssertTableNotExists(t, db, migrations.TableName)
	})
}

// fakeExecutor returns an executor over a hand-built graph in which the given
// migrations count as applied (Django's FakeLoader). Every node of nodes gets
// a migration, deps are (child, parent) pairs.
func fakeExecutor(t *testing.T, nodes []migrations.Key, deps [][2]migrations.Key, applied ...migrations.Key) *migrations.Executor {
	t.Helper()
	g := migrations.NewGraph()
	for _, n := range nodes {
		g.AddNode(n, &migrations.Migration{App: n.App, Name: n.Name})
	}
	for _, d := range deps {
		if err := g.AddDependency("", d[0], d[1], false); err != nil {
			t.Fatalf("AddDependency(%v, %v): %v", d[0], d[1], err)
		}
	}
	set := map[migrations.Key]time.Time{}
	for _, key := range applied {
		set[key] = time.Time{}
	}
	return &migrations.Executor{Loader: &migrations.Loader{Graph: g, Applied: set}}
}

// Ported from Django: tests/migrations/test_executor.py::ExecutorUnitTests.test_minimize_rollbacks
func TestMinimizeRollbacks(t *testing.T) {
	a1, a2, b1 := k("a", "1"), k("a", "2"), k("b", "1")
	x := fakeExecutor(t,
		[]migrations.Key{a1, a2, b1},
		[][2]migrations.Key{{b1, a1}, {a2, a1}},
		a1, b1, a2)
	djangotest.AssertSequenceEqual(t, steps(migrationPlan(t, x, a1)), []planned{backwards("a", "2")})
}

// Ported from Django: tests/migrations/test_executor.py::ExecutorUnitTests.test_minimize_rollbacks_branchy
func TestMinimizeRollbacksBranchy(t *testing.T) {
	a1, a2, a3, a4 := k("a", "1"), k("a", "2"), k("a", "3"), k("a", "4")
	b1, b2 := k("b", "1"), k("b", "2")
	x := fakeExecutor(t,
		[]migrations.Key{a1, a2, a3, a4, b1, b2},
		[][2]migrations.Key{{a2, a1}, {a3, a1}, {a4, a2}, {a4, a3}, {b2, b1}, {b1, a1}, {b2, a2}},
		a1, b1, a2, b2, a3, a4)
	djangotest.AssertSequenceEqual(t, steps(migrationPlan(t, x, a1)), []planned{
		backwards("b", "2"), backwards("a", "4"), backwards("a", "2"), backwards("a", "3"),
	})
}

// Ported from Django: tests/migrations/test_executor.py::ExecutorUnitTests.test_backwards_nothing_to_do
func TestBackwardsNothingToDo(t *testing.T) {
	a1, a2, b1, c1 := k("a", "1"), k("a", "2"), k("b", "1"), k("c", "1")
	x := fakeExecutor(t,
		[]migrations.Key{a1, a2, b1, c1},
		[][2]migrations.Key{{a2, a1}, {b1, a1}, {c1, a1}},
		a1, b1)
	djangotest.AssertEqual(t, len(migrationPlan(t, x, a1)), 0)
}
