package migrations_test

import (
	"context"
	"slices"
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/migrations"
	"github.com/oliverhaas/djangogo/orm"
)

// squashedMigrations mirrors Django's migrations.test_migrations_squashed:
// 0001_squashed_0002 replaces 0001_initial and 0002_second.
func squashedMigrations() []migrations.Migration {
	book := fkField("Author", "author_id", "migrations.Author")
	book.Null = true
	book.RelOnDelete = orm.OnDeleteSetNull
	createBook := migrations.CreateModel{Name: "Book", Fields: []migrations.FieldState{idField, book}}
	rating := migrations.FieldState{Name: "Rating", Column: "rating", Kind: orm.KindInt, Default: int64(0)}
	squashedAuthor := append(otherAuthorFields()[:4:4], rating)
	return []migrations.Migration{
		{
			App: "migrations", Name: "0001_initial",
			Operations: []migrations.Operation{
				migrations.CreateModel{Name: "Author", Fields: otherAuthorFields()},
				migrations.CreateModel{Name: "Tribble", Fields: []migrations.FieldState{
					idField,
					{Name: "Fluffy", Column: "fluffy", Kind: orm.KindBool, Default: true},
				}},
			},
		},
		{
			App: "migrations", Name: "0001_squashed_0002",
			Replaces: []migrations.Key{k("migrations", "0001_initial"), k("migrations", "0002_second")},
			Operations: []migrations.Operation{
				migrations.CreateModel{Name: "Author", Fields: squashedAuthor},
				createBook,
			},
		},
		{
			App: "migrations", Name: "0002_second",
			Dependencies: []migrations.Key{k("migrations", "0001_initial")},
			Operations: []migrations.Operation{
				migrations.DeleteModel{Name: "Tribble"},
				migrations.RemoveField{Model: "Author", Field: "SillyField"},
				migrations.AddField{Model: "Author", Field: rating},
				createBook,
			},
		},
	}
}

// noopMigration returns a migration whose only operation is a RunGo that does
// nothing, the shape of the migrations in Django's squashed_complex fixtures.
func noopMigration(app, name string, deps ...migrations.Key) migrations.Migration {
	return migrations.Migration{
		App: app, Name: name, Dependencies: deps,
		Operations: []migrations.Operation{migrations.RunGo{Forward: migrations.Noop, Reverse: migrations.Noop}},
	}
}

// replacing marks m as the replacement of the given migrations.
func replacing(m migrations.Migration, replaces ...migrations.Key) migrations.Migration {
	m.Replaces = replaces
	return m
}

// squashedComplexMigrations mirrors Django's
// migrations.test_migrations_squashed_complex: 3_squashed_5 replaces 3_auto,
// 4_auto and 5_auto in a line of seven migrations.
func squashedComplexMigrations() []migrations.Migration {
	app := "migrations"
	return []migrations.Migration{
		noopMigration(app, "1_auto"),
		noopMigration(app, "2_auto", k(app, "1_auto")),
		noopMigration(app, "3_auto", k(app, "2_auto")),
		replacing(noopMigration(app, "3_squashed_5", k(app, "2_auto")),
			k(app, "3_auto"), k(app, "4_auto"), k(app, "5_auto")),
		noopMigration(app, "4_auto", k(app, "3_auto")),
		noopMigration(app, "5_auto", k(app, "4_auto")),
		noopMigration(app, "6_auto", k(app, "5_auto")),
		noopMigration(app, "7_auto", k(app, "6_auto")),
	}
}

// squashedComplexMultiAppsMigrations mirrors Django's
// migrations.test_migrations_squashed_complex_multi_apps.
func squashedComplexMultiAppsMigrations() []migrations.Migration {
	return []migrations.Migration{
		noopMigration("app1", "1_auto"),
		noopMigration("app1", "2_auto", k("app1", "1_auto")),
		replacing(noopMigration("app1", "2_squashed_3", k("app1", "1_auto"), k("app2", "2_auto")),
			k("app1", "2_auto"), k("app1", "3_auto")),
		noopMigration("app1", "3_auto", k("app1", "2_auto"), k("app2", "2_auto")),
		noopMigration("app1", "4_auto", k("app1", "3_auto")),
		noopMigration("app2", "1_auto", k("app1", "1_auto")),
		replacing(noopMigration("app2", "1_squashed_2", k("app1", "1_auto")),
			k("app2", "1_auto"), k("app2", "2_auto")),
		noopMigration("app2", "2_auto", k("app2", "1_auto")),
	}
}

// squashedErroneousMigrations mirrors Django's
// migrations.test_migrations_squashed_erroneous, which lacks the replaced
// migrations 3_auto to 5_auto.
func squashedErroneousMigrations() []migrations.Migration {
	app := "migrations"
	return []migrations.Migration{
		noopMigration(app, "1_auto"),
		noopMigration(app, "2_auto", k(app, "1_auto")),
		replacing(noopMigration(app, "3_squashed_5", k(app, "2_auto")),
			k(app, "3_auto"), k(app, "4_auto"), k(app, "5_auto")),
		noopMigration(app, "6_auto", k(app, "5_auto")),
		noopMigration(app, "7_auto", k(app, "6_auto")),
	}
}

// squashedExtraMigrations mirrors Django's
// migrations.test_migrations_squashed_extra.
func squashedExtraMigrations() []migrations.Migration {
	app := "migrations"
	return []migrations.Migration{
		{App: app, Name: "0001_initial"},
		{App: app, Name: "0001_squashed_0002", Replaces: []migrations.Key{k(app, "0001_initial"), k(app, "0002_second")}},
		{App: app, Name: "0002_second", Dependencies: []migrations.Key{k(app, "0001_initial")}},
		{App: app, Name: "0003_third", Dependencies: []migrations.Key{k(app, "0002_second")}},
	}
}

// squashedRefSquashedMigrations mirrors Django's
// migrations.test_migrations_squashed_ref_squashed.
func squashedRefSquashedMigrations() []migrations.Migration {
	return []migrations.Migration{
		{App: "app1", Name: "1_auto"},
		{App: "app1", Name: "2_auto", Dependencies: []migrations.Key{k("app1", "1_auto")}},
		{
			App: "app1", Name: "2_squashed_3",
			Dependencies: []migrations.Key{k("app1", "1_auto"), k("app2", "1_squashed_2")},
			Replaces:     []migrations.Key{k("app1", "2_auto"), k("app1", "3_auto")},
		},
		{App: "app1", Name: "3_auto", Dependencies: []migrations.Key{k("app1", "2_auto"), k("app2", "2_auto")}},
		{App: "app1", Name: "4_auto", Dependencies: []migrations.Key{k("app1", "2_squashed_3")}},
		{App: "app2", Name: "1_auto", Dependencies: []migrations.Key{k("app1", "1_auto")}},
		{
			App: "app2", Name: "1_squashed_2",
			Dependencies: []migrations.Key{k("app1", "1_auto")},
			Replaces:     []migrations.Key{k("app2", "1_auto"), k("app2", "2_auto")},
		},
		{App: "app2", Name: "2_auto", Dependencies: []migrations.Key{k("app2", "1_auto")}},
	}
}

// recordApplied marks migrations as applied in db, Django's
// MigrationRecorder.record_applied.
func recordApplied(t *testing.T, db *orm.DB, keys ...migrations.Key) {
	t.Helper()
	r := migrations.NewRecorder(db)
	for _, key := range keys {
		if err := r.RecordApplied(context.Background(), key); err != nil {
			t.Fatalf("RecordApplied(%v): %v", key, err)
		}
	}
}

// buildGraph rebuilds the graph of l from its migrations and the database.
func buildGraph(t *testing.T, l *migrations.Loader) {
	t.Helper()
	if err := l.BuildGraph(context.Background()); err != nil {
		t.Fatalf("BuildGraph: %v", err)
	}
}

// appNodes counts the nodes of app in the graph of l.
func appNodes(l *migrations.Loader, app string) int {
	n := 0
	for _, key := range l.Graph.Keys() {
		if key.App == app {
			n++
		}
	}
	return n
}

// unappliedPlan returns the forwards plan to target without the applied
// migrations, sorted, Django's set(forwards_plan) - applied_migrations.
func unappliedPlan(t *testing.T, l *migrations.Loader, target migrations.Key) []migrations.Key {
	t.Helper()
	plan, err := l.Graph.ForwardsPlan(target)
	if err != nil {
		t.Fatalf("ForwardsPlan(%v): %v", target, err)
	}
	var out []migrations.Key
	for _, key := range plan {
		if _, ok := l.Applied[key]; !ok {
			out = append(out, key)
		}
	}
	slices.SortFunc(out, compareKeys)
	return out
}

// sortedKeys returns keys in ascending order, for comparing sets.
func sortedKeys(keys ...migrations.Key) []migrations.Key {
	out := slices.Clone(keys)
	slices.SortFunc(out, compareKeys)
	return out
}

// compareKeys orders keys by app, then name.
func compareKeys(a, b migrations.Key) int {
	if a.App != b.App {
		if a.App < b.App {
			return -1
		}
		return 1
	}
	switch {
	case a.Name < b.Name:
		return -1
	case a.Name > b.Name:
		return 1
	}
	return 0
}

// Ported from Django: tests/migrations/test_graph.py::GraphTests.test_remove_replaced_nodes
func TestRemoveReplacedNodes(t *testing.T) {
	g := migrations.NewGraph()
	g.AddDummyNode(k("app_a", "0001"), "app_a.0002", "BAD!")
	g.AddDummyNode(k("app_a", "0002"), "app_b.0001", "BAD!")
	_ = g.AddDependency("app_a.0002", k("app_a", "0002"), k("app_a", "0001"), true)
	// Normal parent and child nodes to check the remapped dependencies.
	g.AddNode(k("app_c", "0001"), nil)
	g.AddNode(k("app_b", "0001"), nil)
	_ = g.AddDependency("app_a.0001", k("app_a", "0001"), k("app_c", "0001"), true)
	_ = g.AddDependency("app_b.0001", k("app_b", "0001"), k("app_a", "0002"), true)
	// Replacing before the replacement node exists fails.
	replaced := []migrations.Key{k("app_a", "0001"), k("app_a", "0002")}
	assertNodeNotFound(t, g.RemoveReplacedNodes(k("app_a", "0001_squashed_0002"), replaced),
		"Unable to find replacement node ('app_a', '0001_squashed_0002'). It was either never added "+
			"to the migration graph, or has been removed.")
	g.AddNode(k("app_a", "0001_squashed_0002"), nil)
	// The dummy nodes still make the graph inconsistent.
	assertNodeNotFound(t, g.ValidateConsistency(), "BAD!")
	if err := g.RemoveReplacedNodes(k("app_a", "0001_squashed_0002"), replaced); err != nil {
		t.Fatalf("RemoveReplacedNodes: %v", err)
	}
	// The graph is consistent now and the dependencies point at the
	// replacement.
	if err := g.ValidateConsistency(); err != nil {
		t.Fatalf("ValidateConsistency: %v", err)
	}
	djangotest.AssertSequenceEqual(t, g.Parents(k("app_a", "0001_squashed_0002")), []migrations.Key{k("app_c", "0001")})
	djangotest.AssertSequenceEqual(t, g.Children(k("app_c", "0001")), []migrations.Key{k("app_a", "0001_squashed_0002")})
	djangotest.AssertSequenceEqual(t, g.Children(k("app_a", "0001_squashed_0002")), []migrations.Key{k("app_b", "0001")})
	djangotest.AssertSequenceEqual(t, g.Parents(k("app_b", "0001")), []migrations.Key{k("app_a", "0001_squashed_0002")})
}

// Ported from Django: tests/migrations/test_graph.py::GraphTests.test_remove_replacement_node
func TestRemoveReplacementNode(t *testing.T) {
	g := migrations.NewGraph()
	g.AddNode(k("app_a", "0001"), nil)
	g.AddNode(k("app_a", "0002"), nil)
	if err := g.AddDependency("app_a.0002", k("app_a", "0002"), k("app_a", "0001"), false); err != nil {
		t.Fatalf("AddDependency: %v", err)
	}
	// Removing the replacement node before it exists fails.
	replaced := []migrations.Key{k("app_a", "0001"), k("app_a", "0002")}
	assertNodeNotFound(t, g.RemoveReplacementNode(k("app_a", "0001_squashed_0002"), replaced),
		"Unable to remove replacement node ('app_a', '0001_squashed_0002'). It was either never added "+
			"to the migration graph, or has been removed already.")
	g.AddNode(k("app_a", "0001_squashed_0002"), nil)
	// A child node to check the remapped dependency.
	g.AddNode(k("app_b", "0001"), nil)
	if err := g.AddDependency("app_b.0001", k("app_b", "0001"), k("app_a", "0001_squashed_0002"), false); err != nil {
		t.Fatalf("AddDependency: %v", err)
	}
	if err := g.RemoveReplacementNode(k("app_a", "0001_squashed_0002"), replaced); err != nil {
		t.Fatalf("RemoveReplacementNode: %v", err)
	}
	if err := g.ValidateConsistency(); err != nil {
		t.Fatalf("ValidateConsistency: %v", err)
	}
	djangotest.AssertSequenceEqual(t, g.Children(k("app_a", "0002")), []migrations.Key{k("app_b", "0001")})
	djangotest.AssertSequenceEqual(t, g.Parents(k("app_b", "0001")), []migrations.Key{k("app_a", "0002")})
	// The child is not remapped to the other replaced node as well.
	djangotest.AssertSequenceEqual(t, g.Children(k("app_a", "0001")), []migrations.Key{k("app_a", "0002")})
}

// Ported from Django: tests/migrations/test_loader.py::LoaderTests.test_loading_squashed
func TestLoadingSquashed(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		l := newLoader(t, db, squashedMigrations())
		// With nothing applied the squashed migration is the only node.
		djangotest.AssertEqual(t, appNodes(l, "migrations"), 1)
		// With one replaced migration applied the two old ones are used.
		recordApplied(t, db, k("migrations", "0001_initial"))
		buildGraph(t, l)
		djangotest.AssertEqual(t, appNodes(l, "migrations"), 2)
	})
}

// Ported from Django: tests/migrations/test_loader.py::LoaderTests.test_loading_squashed_complex
func TestLoadingSquashedComplex(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		l := newLoader(t, db, squashedComplexMigrations())
		numNodes := func() int { return len(unappliedPlan(t, l, k("migrations", "7_auto"))) }
		// An empty database uses the squashed migration.
		buildGraph(t, l)
		djangotest.AssertEqual(t, numNodes(), 5)
		// Starting at 1 or 2 uses the squashed migration too.
		recordApplied(t, db, k("migrations", "1_auto"))
		buildGraph(t, l)
		djangotest.AssertEqual(t, numNodes(), 4)
		recordApplied(t, db, k("migrations", "2_auto"))
		buildGraph(t, l)
		djangotest.AssertEqual(t, numNodes(), 3)
		// Starting at 3 to 5 cannot use the squashed migration.
		recordApplied(t, db, k("migrations", "3_auto"))
		buildGraph(t, l)
		djangotest.AssertEqual(t, numNodes(), 4)
		recordApplied(t, db, k("migrations", "4_auto"))
		buildGraph(t, l)
		djangotest.AssertEqual(t, numNodes(), 3)
		// Starting at 5 to 7 is past the squashed migrations.
		recordApplied(t, db, k("migrations", "5_auto"))
		buildGraph(t, l)
		djangotest.AssertEqual(t, numNodes(), 2)
		recordApplied(t, db, k("migrations", "6_auto"))
		buildGraph(t, l)
		djangotest.AssertEqual(t, numNodes(), 1)
		recordApplied(t, db, k("migrations", "7_auto"))
		buildGraph(t, l)
		djangotest.AssertEqual(t, numNodes(), 0)
	})
}

// Ported from Django: tests/migrations/test_loader.py::LoaderTests.test_loading_squashed_complex_multi_apps
func TestLoadingSquashedComplexMultiApps(t *testing.T) {
	l := newLoader(t, nil, squashedComplexMultiAppsMigrations())
	djangotest.AssertSequenceEqual(t, unappliedPlan(t, l, k("app1", "4_auto")), sortedKeys(
		k("app1", "1_auto"), k("app2", "1_squashed_2"), k("app1", "2_squashed_3"), k("app1", "4_auto"),
	))
}

// Ported from Django: tests/migrations/test_loader.py::LoaderTests.test_loading_squashed_complex_multi_apps_partially_applied
func TestLoadingSquashedComplexMultiAppsPartiallyApplied(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		recordApplied(t, db, k("app1", "1_auto"), k("app1", "2_auto"))
		l := newLoader(t, db, squashedComplexMultiAppsMigrations())
		djangotest.AssertSequenceEqual(t, unappliedPlan(t, l, k("app1", "4_auto")), sortedKeys(
			k("app2", "1_squashed_2"), k("app1", "3_auto"), k("app1", "4_auto"),
		))
	})
}

// Django's message names Django; the Go message names djangogo.
//
// Ported from Django: tests/migrations/test_loader.py::LoaderTests.test_loading_squashed_erroneous
func TestLoadingSquashedErroneous(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		l := newLoader(t, db, squashedErroneousMigrations())
		numNodes := func() int { return len(unappliedPlan(t, l, k("migrations", "7_auto"))) }
		// An empty database uses the squashed migration.
		buildGraph(t, l)
		djangotest.AssertEqual(t, numNodes(), 5)
		// Starting at 1 or 2 uses the squashed migration too.
		recordApplied(t, db, k("migrations", "1_auto"))
		buildGraph(t, l)
		djangotest.AssertEqual(t, numNodes(), 4)
		recordApplied(t, db, k("migrations", "2_auto"))
		buildGraph(t, l)
		djangotest.AssertEqual(t, numNodes(), 3)
		// Starting at 3 or 4 needs migrations that do not exist.
		msg := "Migration migrations.6_auto depends on nonexistent node ('migrations', '5_auto'). " +
			"djangogo tried to replace migration migrations.5_auto with any of [migrations.3_squashed_5] " +
			"but wasn't able to because some of the replaced migrations are already applied."
		recordApplied(t, db, k("migrations", "3_auto"))
		assertNodeNotFound(t, l.BuildGraph(context.Background()), msg)
		recordApplied(t, db, k("migrations", "4_auto"))
		assertNodeNotFound(t, l.BuildGraph(context.Background()), msg)
		// Starting at 5 to 7 is past the squashed migrations.
		recordApplied(t, db, k("migrations", "5_auto"))
		buildGraph(t, l)
		djangotest.AssertEqual(t, numNodes(), 2)
		recordApplied(t, db, k("migrations", "6_auto"))
		buildGraph(t, l)
		djangotest.AssertEqual(t, numNodes(), 1)
		recordApplied(t, db, k("migrations", "7_auto"))
		buildGraph(t, l)
		djangotest.AssertEqual(t, numNodes(), 0)
	})
}

// Ported from Django: tests/migrations/test_loader.py::LoaderTests.test_check_consistent_history_squashed
func TestCheckConsistentHistorySquashed(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		recordApplied(t, db, k("migrations", "0001_initial"), k("migrations", "0002_second"))
		if err := newLoader(t, db, squashedExtraMigrations()).CheckConsistentHistory(); err != nil {
			t.Fatalf("CheckConsistentHistory: %v", err)
		}
		recordApplied(t, db, k("migrations", "0003_third"))
		if err := newLoader(t, db, squashedExtraMigrations()).CheckConsistentHistory(); err != nil {
			t.Fatalf("CheckConsistentHistory: %v", err)
		}
	})
}

// Ported from Django: tests/migrations/test_loader.py::LoaderTests.test_loading_squashed_ref_squashed
func TestLoadingSquashedRefSquashed(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		l := newLoader(t, db, squashedRefSquashedMigrations())
		// With nothing applied both migrations are squashed.
		djangotest.AssertSequenceEqual(t, unappliedPlan(t, l, k("app1", "4_auto")), sortedKeys(
			k("app1", "1_auto"), k("app2", "1_squashed_2"), k("app1", "2_squashed_3"), k("app1", "4_auto"),
		))
		// A replaced migration is no target while the loader replaces
		// migrations, which it does by default.
		_, err := l.Graph.ForwardsPlan(k("app1", "3_auto"))
		assertNodeNotFound(t, err, "Node ('app1', '3_auto') not a valid node")
		// It is one without replacements.
		l.ReplaceMigrations = false
		buildGraph(t, l)
		djangotest.AssertSequenceEqual(t, unappliedPlan(t, l, k("app1", "3_auto")), sortedKeys(
			k("app1", "1_auto"), k("app2", "1_auto"), k("app2", "2_auto"), k("app1", "2_auto"), k("app1", "3_auto"),
		))
		l.ReplaceMigrations = true

		// Applying some of app1 undoes the squash in app1.
		recordApplied(t, db, k("app1", "1_auto"), k("app1", "2_auto"))
		buildGraph(t, l)
		djangotest.AssertSequenceEqual(t, unappliedPlan(t, l, k("app1", "4_auto")), sortedKeys(
			k("app2", "1_squashed_2"), k("app1", "3_auto"), k("app1", "4_auto"),
		))
		// Applying one of app2 undoes the squash in app2 too.
		recordApplied(t, db, k("app2", "1_auto"))
		buildGraph(t, l)
		djangotest.AssertSequenceEqual(t, unappliedPlan(t, l, k("app1", "4_auto")), sortedKeys(
			k("app2", "2_auto"), k("app1", "3_auto"), k("app1", "4_auto"),
		))
	})
}

// Ported from Django: tests/migrations/test_executor.py::ExecutorTests.test_run_with_squashed
func TestRunWithSquashed(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		x := newExecutor(t, db, squashedMigrations())
		// The leaf node is the squashed one.
		djangotest.AssertSequenceEqual(t, x.Loader.Graph.LeafNodes("migrations"),
			[]migrations.Key{k("migrations", "0001_squashed_0002")})
		djangotest.AssertSequenceEqual(t, steps(migrationPlan(t, x, k("migrations", "0001_squashed_0002"))),
			[]planned{forwards("migrations", "0001_squashed_0002")})
		djangotest.AssertTableNotExists(t, db, "migrations_author")
		djangotest.AssertTableNotExists(t, db, "migrations_book")
		migrate(t, x, k("migrations", "0001_squashed_0002"))
		djangotest.AssertTableExists(t, db, "migrations_author")
		djangotest.AssertTableExists(t, db, "migrations_book")
		// Rebuild the graph for the new state of the database. Undoing uses
		// the squashed migration as well.
		buildGraph(t, x.Loader)
		djangotest.AssertSequenceEqual(t, steps(migrationPlan(t, x, zero("migrations"))),
			[]planned{backwards("migrations", "0001_squashed_0002")})
		migrate(t, x, zero("migrations"))
		djangotest.AssertTableNotExists(t, db, "migrations_author")
		djangotest.AssertTableNotExists(t, db, "migrations_book")
	})
}

// Ported from Django: tests/migrations/test_executor.py::ExecutorTests.test_migrate_backward_to_squashed_migration
func TestMigrateBackwardToSquashedMigration(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		x := newExecutor(t, db, squashedMigrations())
		djangotest.AssertTableNotExists(t, db, "migrations_author")
		djangotest.AssertTableNotExists(t, db, "migrations_book")
		migrate(t, x, k("migrations", "0001_squashed_0002"))
		djangotest.AssertTableExists(t, db, "migrations_author")
		djangotest.AssertTableExists(t, db, "migrations_book")
		buildGraph(t, x.Loader)
		// Migrate backward to a replaced migration.
		migrate(t, x, k("migrations", "0001_initial"))
		djangotest.AssertTableExists(t, db, "migrations_author")
		djangotest.AssertTableNotExists(t, db, "migrations_book")
		// Unmigrate everything.
		migrate(t, newExecutor(t, db, squashedMigrations()), zero("migrations"))
		djangotest.AssertTableNotExists(t, db, "migrations_author")
		djangotest.AssertTableNotExists(t, db, "migrations_book")
	})
}

// Ported from Django: tests/migrations/test_executor.py::ExecutorTests.test_apply_all_replaced_marks_replacement_as_applied
func TestApplyAllReplacedMarksReplacementAsApplied(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		// The replaced migrations are partially applied: 0001 is, 0002 is
		// not.
		recordApplied(t, db, k("migrations", "0001_initial"))
		x := newExecutor(t, db, squashedMigrations())
		// Fake, because the tables of the first migration do not exist.
		if _, err := x.Migrate(context.Background(), []migrations.Key{k("migrations", "0002_second")}, nil, true); err != nil {
			t.Fatalf("Migrate: %v", err)
		}
		// Both replaced migrations are applied now, so their replacement is
		// recorded as applied.
		assertRecorded(t, db, k("migrations", "0001_squashed_0002"))
	})
}

// Ported from Django: tests/migrations/test_executor.py::ExecutorTests.test_migrate_marks_replacement_applied_even_if_it_did_nothing
func TestMigrateMarksReplacementAppliedEvenIfItDidNothing(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		recordApplied(t, db, k("migrations", "0001_initial"), k("migrations", "0002_second"))
		migrate(t, newExecutor(t, db, squashedMigrations()), k("migrations", "0001_squashed_0002"))
		// The run applied nothing, but both replaced migrations are applied,
		// so their replacement is recorded as applied.
		assertRecorded(t, db, k("migrations", "0001_squashed_0002"))
	})
}

// Ported from Django: tests/migrations/test_executor.py::ExecutorTests.test_migrate_marks_replacement_unapplied
func TestMigrateMarksReplacementUnapplied(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		x := newExecutor(t, db, squashedMigrations())
		migrate(t, x, k("migrations", "0001_squashed_0002"))
		assertRecorded(t, db, k("migrations", "0001_squashed_0002"))
		buildGraph(t, x.Loader)
		migrate(t, x, zero("migrations"))
		assertNotRecorded(t, db, k("migrations", "0001_squashed_0002"))
	})
}

// assertRecorded checks that the tracking table of db lists key as applied.
func assertRecorded(t *testing.T, db *orm.DB, key migrations.Key) {
	t.Helper()
	applied, err := migrations.NewRecorder(db).AppliedMigrations(context.Background())
	if err != nil {
		t.Fatalf("AppliedMigrations: %v", err)
	}
	if _, ok := applied[key]; !ok {
		t.Errorf("%s is not recorded as applied", key)
	}
}

// assertNotRecorded checks that the tracking table of db does not list key.
func assertNotRecorded(t *testing.T, db *orm.DB, key migrations.Key) {
	t.Helper()
	applied, err := migrations.NewRecorder(db).AppliedMigrations(context.Background())
	if err != nil {
		t.Fatalf("AppliedMigrations: %v", err)
	}
	if _, ok := applied[key]; ok {
		t.Errorf("%s is recorded as applied", key)
	}
}
