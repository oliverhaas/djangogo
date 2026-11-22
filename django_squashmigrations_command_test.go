package djangogo_test

import (
	"cmp"
	"context"
	"errors"
	"maps"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/migrations"
	"github.com/oliverhaas/djangogo/orm"
)

// squashedMigrations mirrors Django's migrations.test_migrations_squashed:
// 0001_squashed_0002 replaces 0001_initial and 0002_second.
func squashedMigrations() []migrations.Migration {
	rating := migrations.FieldState{Name: "Rating", Column: "rating", Kind: orm.KindInt, Default: int64(0)}
	return []migrations.Migration{
		{
			App: "migrations", Name: "0001_initial",
			Operations: []migrations.Operation{
				migrations.CreateModel{Name: "Author", Fields: authorFields()},
				tribble(),
			},
		},
		{
			App: "migrations", Name: "0001_squashed_0002",
			Replaces: []migrations.Key{key("migrations", "0001_initial"), key("migrations", "0002_second")},
			Operations: []migrations.Operation{
				migrations.CreateModel{Name: "Author", Fields: append(authorFields()[:4], rating)},
				secondOperations()[3],
			},
		},
		{
			App: "migrations", Name: "0002_second",
			Dependencies: []migrations.Key{key("migrations", "0001_initial")},
			Operations:   secondOperations(),
		},
	}
}

// squashedNoReplacesMigrations mirrors Django's
// migrations.test_migrations_squashed_no_replaces: the squashed migration
// after its replaced migrations and its Replaces were deleted.
func squashedNoReplacesMigrations() []migrations.Migration {
	return []migrations.Migration{{
		App: "migrations", Name: "0001_squashed_0002",
		Operations: []migrations.Operation{
			migrations.CreateModel{Name: "Author", Fields: authorFields()[:2]},
			secondOperations()[3],
		},
	}}
}

// squashedWithReplacesMigrations2 mirrors Django's
// migrations2.test_migrations_2_squashed_with_replaces: the squashed
// migration still lists the deleted migrations it replaced.
func squashedWithReplacesMigrations2() []migrations.Migration {
	return []migrations.Migration{{
		App: "migrations2", Name: "0001_squashed_0002",
		Replaces: []migrations.Key{key("migrations2", "0001_initial"), key("migrations2", "0002_second")},
		Operations: []migrations.Operation{
			migrations.CreateModel{Name: "OtherAuthor", Fields: authorFields()[:2]},
			migrations.CreateModel{Name: "OtherBook", Fields: []migrations.FieldState{
				idField,
				{
					Name: "Author", Column: "author_id", Kind: orm.KindInt, Null: true,
					RelKind: orm.RelFK, RelTo: "migrations2.OtherAuthor", RelOnDelete: orm.OnDeleteSetNull,
				},
			}},
		},
	}}
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
		noopMigration(app, "2_auto", key(app, "1_auto")),
		noopMigration(app, "3_auto", key(app, "2_auto")),
		replacing(noopMigration(app, "3_squashed_5", key(app, "2_auto")),
			key(app, "3_auto"), key(app, "4_auto"), key(app, "5_auto")),
		noopMigration(app, "4_auto", key(app, "3_auto")),
		noopMigration(app, "5_auto", key(app, "4_auto")),
		noopMigration(app, "6_auto", key(app, "5_auto")),
		noopMigration(app, "7_auto", key(app, "6_auto")),
	}
}

// squashedLoopMigrations mirrors Django's migrations.test_migrations_squashed_loop:
// 2_auto and 2_squashed replace each other.
func squashedLoopMigrations() []migrations.Migration {
	app := "migrations"
	return []migrations.Migration{
		noopMigration(app, "1_auto"),
		replacing(noopMigration(app, "2_auto", key(app, "1_auto")), key(app, "2_squashed")),
		replacing(noopMigration(app, "2_squashed", key(app, "1_auto")), key(app, "2_auto")),
	}
}

// squashedDoubleMigrations mirrors Django's
// migrations.test_migrations_squashed_double: 0005 replaces the squashed
// migration 0003 and 0004.
func squashedDoubleMigrations() []migrations.Migration {
	app := "migrations"
	foo := func(def any) migrations.FieldState {
		return migrations.FieldState{Name: "Foo", Column: "foo", Kind: orm.KindBool, Default: def}
	}
	createA := func(def any) migrations.Operation {
		return migrations.CreateModel{Name: "A", Fields: []migrations.FieldState{idField, foo(def)}}
	}
	return []migrations.Migration{
		{App: app, Name: "0001_initial", Operations: []migrations.Operation{createA(nil)}},
		{
			App: app, Name: "0002_auto", Dependencies: []migrations.Key{key(app, "0001_initial")},
			Operations: []migrations.Operation{migrations.AlterField{Model: "A", Field: foo(true)}},
		},
		{
			App: app, Name: "0003_squashed_0001_and_0002",
			Replaces:   []migrations.Key{key(app, "0001_initial"), key(app, "0002_auto")},
			Operations: []migrations.Operation{createA(true)},
		},
		{
			App: app, Name: "0004_auto", Dependencies: []migrations.Key{key(app, "0002_auto")},
			Operations: []migrations.Operation{migrations.AlterField{Model: "A", Field: foo(false)}},
		},
		{
			App: app, Name: "0005_squashed_0003_and_0004",
			Replaces:   []migrations.Key{key(app, "0003_squashed_0001_and_0002"), key(app, "0004_auto")},
			Operations: []migrations.Operation{createA(false)},
		},
	}
}

// squashedPartiallyAppliedMigrations mirrors Django's
// migrations.test_migrations_squashed_partially_applied.
func squashedPartiallyAppliedMigrations() []migrations.Migration {
	app := "migrations"
	intField := func(name, column string, null bool) migrations.FieldState {
		return migrations.FieldState{Name: name, Column: column, Kind: orm.KindInt, Null: null}
	}
	return []migrations.Migration{
		{
			App: app, Name: "0001_initial",
			Operations: []migrations.Operation{
				migrations.CreateModel{Name: "MyModel1", Fields: []migrations.FieldState{idField}},
				migrations.CreateModel{Name: "MyModel2", Fields: []migrations.FieldState{
					idField, intField("Field1", "field_1", false),
				}},
			},
		},
		{
			App: app, Name: "0002_mymodel1_field_1_mymodel2_field_2_and_more",
			Dependencies: []migrations.Key{key(app, "0001_initial")},
			Operations: []migrations.Operation{
				migrations.AddField{Model: "MyModel1", Field: intField("Field1", "field_1", true)},
				migrations.AddField{Model: "MyModel2", Field: intField("Field2", "field_2", true)},
				migrations.AlterField{Model: "MyModel2", Field: intField("Field1", "field_1", true)},
			},
		},
		{
			App: app, Name: "0003_alter_mymodel2_unique_together",
			Dependencies: []migrations.Key{key(app, "0002_mymodel1_field_1_mymodel2_field_2_and_more")},
			Operations: []migrations.Operation{
				migrations.AlterUniqueTogether{Name: "MyModel2", UniqueTogether: [][]string{{"Field1", "Field2"}}},
			},
		},
		{
			App: app, Name: "0004_remove_mymodel1_field_1_mymodel1_field_3_and_more",
			Dependencies: []migrations.Key{key(app, "0003_alter_mymodel2_unique_together")},
			Operations: []migrations.Operation{
				migrations.RemoveField{Model: "MyModel1", Field: "Field1"},
				migrations.AddField{Model: "MyModel1", Field: intField("Field3", "field_3", true)},
				migrations.AddField{Model: "MyModel1", Field: intField("Field4", "field_4", true)},
			},
		},
	}
}

// manualPortingMigrations mirrors Django's
// migrations.test_migrations_manual_porting. Its RunGo operations call a
// function of 0002_manual_porting_test.go.
func manualPortingMigrations() []migrations.Migration {
	app := "migrations"
	runGo := migrations.RunGo{Forward: ManualPortingForwards, Reverse: migrations.Noop}
	uniqueTwice := []migrations.Operation{
		migrations.AlterUniqueTogether{Name: "SomeModel", UniqueTogether: [][]string{{"ID", "Name"}}},
		migrations.AlterUniqueTogether{Name: "SomeModel", UniqueTogether: [][]string{{"Name"}}},
		runGo,
	}
	return []migrations.Migration{
		{
			App: app, Name: "0001_initial", Initial: true,
			Operations: []migrations.Operation{
				migrations.CreateModel{Name: "SomeModel", Fields: authorFields()[:2]},
			},
		},
		{
			App: app, Name: "0002_second", Dependencies: []migrations.Key{key(app, "0001_initial")},
			Operations: []migrations.Operation{runGo},
		},
		{
			App: app, Name: "0003_third", Dependencies: []migrations.Key{key(app, "0002_second")},
			Operations: uniqueTwice,
		},
		{
			App: app, Name: "0004_fourth", Dependencies: []migrations.Key{key(app, "0002_second")},
			Replaces:   []migrations.Key{key(app, "0003_third")},
			Operations: uniqueTwice,
		},
	}
}

// plannedMigrations returns the names of the migrations in the output of
// migrate --plan, as the regular expression of Django's tests finds them.
func plannedMigrations(out string) []string {
	matches := regexp.MustCompile(`migrations.(.+)\n`).FindAllStringSubmatch(out, -1)
	names := make([]string, 0, len(matches))
	for _, match := range matches {
		names = append(names, match[1])
	}
	return names
}

// recorded returns the rows of the tracking table.
func recorded(t *testing.T, db *orm.DB) map[migrations.Key]bool {
	t.Helper()
	applied, err := migrations.NewRecorder(db).AppliedMigrations(context.Background())
	if err != nil {
		t.Fatalf("applied migrations: %v", err)
	}
	out := map[migrations.Key]bool{}
	for k := range applied {
		out[k] = true
	}
	return out
}

// recordedApps returns the sorted app labels of the rows of the tracking
// table.
func recordedApps(t *testing.T, db *orm.DB) []string {
	t.Helper()
	var labels []string
	for k := range recorded(t, db) {
		if !slices.Contains(labels, k.App) {
			labels = append(labels, k.App)
		}
	}
	slices.Sort(labels)
	return labels
}

// recordApplied writes rows into the tracking table.
func recordApplied(t *testing.T, db *orm.DB, keys ...migrations.Key) {
	t.Helper()
	recorder := migrations.NewRecorder(db)
	for _, k := range keys {
		if err := recorder.RecordApplied(context.Background(), k); err != nil {
			t.Fatalf("record applied: %v", err)
		}
	}
}

// createdSquashed is the message squashmigrations prints after writing the
// file at path.
func createdSquashed(path string) string {
	return "Created new squashed migration " + path + "\n" +
		"  You should commit this migration but leave the old ones in place;\n" +
		"  the new migration will be used for new installs. Once you are sure\n" +
		"  all instances of the codebase have applied the migrations you squashed,\n" +
		"  you can delete them.\n"
}

// Ported from Django: tests/migrations/test_commands.py::SquashMigrationsTests.test_squashmigrations_squashes
func TestSquashmigrationsSquashes(t *testing.T) {
	c := newCommand(t, nil, testMigrations(), djangotest.App("migrations"))
	out := c.call("squashmigrations", "migrations", "0002", "--noinput")
	path := c.path("migrations", "0001_squashed_0002_second.go")
	assertFile(t, path)
	djangotest.AssertEqual(t, out,
		"Will squash the following migrations:\n"+
			" - 0001_initial\n"+
			" - 0002_second\n"+
			"Optimizing...\n"+
			"  Optimized from 8 operations to 2 operations.\n"+
			createdSquashed(path))
}

// Ported from Django: tests/migrations/test_commands.py::SquashMigrationsTests.test_squashmigrations_replacement_cycle
func TestSquashmigrationsReplacementCycle(t *testing.T) {
	c := newCommand(t, djangotest.NewDB(t), squashedLoopMigrations(), djangotest.App("migrations"))
	_, err := c.fail("migrate", "migrations", "--plan", "--noinput")
	var cerr *migrations.CommandError
	if !errors.As(err, &cerr) {
		t.Fatalf("error = %v (%T), want a CommandError", err, err)
	}
	pattern := regexp.MustCompile(`Cyclical squash replacement found, starting at \('migrations', '2_(squashed|auto)'\)`)
	if !pattern.MatchString(cerr.Message) {
		t.Errorf("message = %q, want a match for %s", cerr.Message, pattern)
	}
}

// Ported from Django: tests/migrations/test_commands.py::SquashMigrationsTests.test_squashmigrations_squashes_already_squashed
func TestSquashmigrationsSquashesAlreadySquashed(t *testing.T) {
	c := newCommand(t, djangotest.NewDB(t), squashedComplexMigrations(), djangotest.App("migrations"))
	c.call("squashmigrations", "migrations", "3_squashed_5", "--squashed-name", "double_squash", "--noinput")
	// The squashed migration replaces 3_squashed_5, not what that one
	// replaces: its operations are the ones used.
	djangotest.AssertSequenceEqual(t, registered(t, c, "migrations", "0001_double_squash").Replaces, []migrations.Key{
		key("migrations", "1_auto"), key("migrations", "2_auto"), key("migrations", "3_squashed_5"),
	})

	out := c.call("migrate", "migrations", "--plan", "--noinput")
	djangotest.AssertSequenceEqual(t, plannedMigrations(out), []string{"0001_double_squash", "6_auto", "7_auto"})
}

// registered returns the migration of the app registered under name.
func registered(t *testing.T, c *command, app, name string) migrations.Migration {
	t.Helper()
	for _, m := range c.app.Migrations.ForApp(app) {
		if m.Name == name {
			return m
		}
	}
	t.Fatalf("migration %s.%s is not registered", app, name)
	return migrations.Migration{}
}

// Replacement migrations are partly applied. Squashing them again leaves
// only the unapplied migrations to migrate.
//
// Ported from Django: tests/migrations/test_commands.py::SquashMigrationsTests.test_squash_partially_applied
func TestSquashPartiallyApplied(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		c := newCommand(t, db, squashedPartiallyAppliedMigrations(), djangotest.App("migrations"))
		defer c.call("migrate", "migrations", "zero")
		// Apply the first two migrations.
		c.call("migrate", "migrations", "0002", "--noinput")
		// Squash those two and the next one.
		c.call("squashmigrations", "migrations", "0001", "0003", "--squashed-name", "squashed_0001_0003", "--noinput")
		// Make the fourth migration depend on the squashed migration.
		fourth := registered(t, c, "migrations", "0004_remove_mymodel1_field_1_mymodel1_field_3_and_more")
		fourth.Dependencies = []migrations.Key{key("migrations", "0001_squashed_0001_0003")}
		c.app.Migrations.Add(fourth)
		// Squash the squashed migration with the fourth one.
		c.call("squashmigrations", "migrations", "0001_squashed_0001_0003", "0004",
			"--squashed-name", "squashed_0001_0004", "--noinput")
		djangotest.AssertSequenceEqual(t, registered(t, c, "migrations", "0001_squashed_0001_0004").Replaces,
			[]migrations.Key{
				key("migrations", "0001_squashed_0001_0003"),
				key("migrations", "0004_remove_mymodel1_field_1_mymodel1_field_3_and_more"),
			})

		// Only the unapplied migrations are applied.
		out := c.call("migrate", "migrations", "--plan", "--noinput")
		djangotest.AssertSequenceEqual(t, plannedMigrations(out), []string{
			"0003_alter_mymodel2_unique_together",
			"0004_remove_mymodel1_field_1_mymodel1_field_3_and_more",
		})
	})
}

// Migrating an app with a squashed migration that replaces another squashed
// migration records and unrecords all the replaced migrations.
//
// Ported from Django: tests/migrations/test_commands.py::SquashMigrationsTests.test_double_replaced_migrations_are_recorded
func TestDoubleReplacedMigrationsAreRecorded(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		c := newCommand(t, db, squashedDoubleMigrations(), djangotest.App("migrations"))
		djangotest.AssertEqual(t, slices.Contains(recordedApps(t, db), "migrations"), false)

		out := c.call("migrate", "migrations", "--plan", "--noinput")
		// Only the outermost squashed migration is applied.
		djangotest.AssertSequenceEqual(t, plannedMigrations(out), []string{"0005_squashed_0003_and_0004"})

		c.call("migrate", "migrations", "--noinput")
		rows := recorded(t, db)
		for _, name := range []string{
			"0001_initial", "0002_auto", "0003_squashed_0001_and_0002", "0004_auto", "0005_squashed_0003_and_0004",
		} {
			if !rows[key("migrations", name)] {
				t.Errorf("migrations.%s is not recorded", name)
			}
		}

		// Unapply all migrations of the app.
		c.call("migrate", "migrations", "zero", "--noinput")
		djangotest.AssertEqual(t, slices.Contains(recordedApps(t, db), "migrations"), false)
	})
}

// A migrate after squashing migrations that are all applied does not fail
// with an inconsistent history.
//
// Ported from Django: tests/migrations/test_commands.py::SquashMigrationsTests.test_double_replaced_migrations_are_checked_correctly
func TestDoubleReplacedMigrationsAreCheckedCorrectly(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		c := newCommand(t, db, nil, djangotest.App("migrations"))
		for range 4 {
			c.call("makemigrations", "migrations", "--empty", "--noinput")
		}
		defer c.call("migrate", "migrations", "zero")
		c.call("migrate", "migrations", "--noinput")
		c.call("squashmigrations", "migrations", "0001", "0002", "--noinput", "-v", "0")
		c.call("squashmigrations", "migrations", "0001_initial_squashed", "0003", "--noinput", "-v", "0")
		c.call("migrate", "migrations", "--noinput")
	})
}

// Ported from Django: tests/migrations/test_commands.py::SquashMigrationsTests.test_squashmigrations_initial_attribute
func TestSquashmigrationsInitialAttribute(t *testing.T) {
	c := newCommand(t, nil, testMigrations(), djangotest.App("migrations"))
	c.call("squashmigrations", "migrations", "0002", "--noinput", "-v", "0")
	assertIn(t, c.read("migrations", "0001_squashed_0002_second.go"), "Initial: true,")
}

// Ported from Django: tests/migrations/test_commands.py::SquashMigrationsTests.test_squashmigrations_optimizes
func TestSquashmigrationsOptimizes(t *testing.T) {
	c := newCommand(t, nil, testMigrations(), djangotest.App("migrations"))
	out := c.call("squashmigrations", "migrations", "0002", "--noinput", "-v", "1")
	assertIn(t, out, "Optimized from 8 operations to 2 operations.")
}

// Ported from Django: tests/migrations/test_commands.py::SquashMigrationsTests.test_ticket_23799_squashmigrations_no_optimize
func TestTicket23799SquashmigrationsNoOptimize(t *testing.T) {
	c := newCommand(t, nil, testMigrations(), djangotest.App("migrations"))
	out := c.call("squashmigrations", "migrations", "0002", "--noinput", "-v", "1", "--no-optimize")
	assertIn(t, out, "Skipping optimization")
}

// Ported from Django: tests/migrations/test_commands.py::SquashMigrationsTests.test_squashmigrations_valid_start
func TestSquashmigrationsValidStart(t *testing.T) {
	c := newCommand(t, nil, noChangesMigrations(), djangotest.App("migrations"))
	out := c.call("squashmigrations", "migrations", "0002", "0003", "--noinput", "-v", "1")
	content := c.read("migrations", "0002_second_squashed_0003_third.go")
	assertIn(t, content, "\t\t\t{App: \"migrations\", Name: \"0001_initial\"},\n")
	assertNotIn(t, content, "Initial: true")
	assertNotIn(t, out, " - 0001_initial")
	assertIn(t, out, " - 0002_second")
	assertIn(t, out, " - 0003_third")
}

// Ported from Django: tests/migrations/test_commands.py::SquashMigrationsTests.test_squashmigrations_invalid_start
func TestSquashmigrationsInvalidStart(t *testing.T) {
	c := newCommand(t, nil, noChangesMigrations(), djangotest.App("migrations"))
	_, err := c.fail("squashmigrations", "migrations", "0003", "0002", "--noinput", "-v", "0")
	var cerr *migrations.CommandError
	if !errors.As(err, &cerr) {
		t.Fatalf("error = %v (%T), want a CommandError", err, err)
	}
	assertIn(t, cerr.Message,
		"The migration 'migrations.0003_third' cannot be found. Maybe it comes after the migration 'migrations.0002_second'")
}

// Ported from Django: tests/migrations/test_commands.py::SquashMigrationsTests.test_squashed_name_with_start_migration_name
func TestSquashedNameWithStartMigrationName(t *testing.T) {
	c := newCommand(t, nil, testMigrations(), djangotest.App("migrations"))
	c.call("squashmigrations", "migrations", "0001", "0002", "--squashed-name", "squashed_name", "--noinput", "-v", "0")
	assertFile(t, c.path("migrations", "0001_squashed_name.go"))
}

// Ported from Django: tests/migrations/test_commands.py::SquashMigrationsTests.test_squashed_name_without_start_migration_name
func TestSquashedNameWithoutStartMigrationName(t *testing.T) {
	c := newCommand(t, nil, testMigrations(), djangotest.App("migrations"))
	c.call("squashmigrations", "migrations", "0001", "--squashed-name", "squashed_name", "--noinput", "-v", "0")
	assertFile(t, c.path("migrations", "0001_squashed_name.go"))
}

// Ported from Django: tests/migrations/test_commands.py::SquashMigrationsTests.test_squashed_name_exists
func TestSquashedNameExists(t *testing.T) {
	c := newCommand(t, nil, testMigrations(), djangotest.App("migrations"))
	_, err := c.fail("squashmigrations", "migrations", "0001", "0002", "--squashed-name", "initial", "--noinput", "-v", "0")
	assertCommandError(t, err, "Migration 0001_initial already exists. Use a different name.", 1)
}

// Django also warns when black cannot format the file. Go migration files
// are written formatted.
//
// Ported from Django: tests/migrations/test_commands.py::SquashMigrationsTests.test_squashmigrations_manual_porting
func TestSquashmigrationsManualPorting(t *testing.T) {
	c := newCommand(t, nil, manualPortingMigrations(), djangotest.App("migrations"))
	out := c.call("squashmigrations", "migrations", "0002", "--noinput")
	path := c.path("migrations", "0001_squashed_0002_second.go")
	assertFile(t, path)
	djangotest.AssertEqual(t, out,
		"Will squash the following migrations:\n"+
			" - 0001_initial\n"+
			" - 0002_second\n"+
			"Optimizing...\n"+
			"  No optimizations possible.\n"+
			createdSquashed(path)+
			"Manual porting required\n"+
			"  Your migrations contained functions that must be manually copied over,\n"+
			"  as we could not safely copy their implementation.\n"+
			"  See the comment at the top of the squashed migration for details.\n")
	assertIn(t, c.read("migrations", "0001_squashed_0002_second.go"),
		"// Functions from the following migrations need manual copying.\n")
}

// No Go counterpart: SquashMigrationsTests.test_failure_to_format_code. The
// migration writer formats the file itself and runs no formatter command.

// Ported from Django: tests/migrations/test_commands.py::AppLabelErrorTests.test_squashmigrations_nonexistent_app_label
func TestSquashmigrationsNonexistentAppLabel(t *testing.T) {
	c := newCommand(t, nil, testMigrations(), djangotest.App("migrations"))
	_, err := c.fail("squashmigrations", "nonexistent_app", "0002")
	assertCommandError(t, err, "No installed app with label 'nonexistent_app'.", 1)
}

// Go has no verbosity option for migrate, so Django's "Running pre-migrate
// handlers" line and the timings are not part of the output.
//
// Ported from Django: tests/migrations/test_commands.py::MigrateTests.test_showmigrations_list_squashed
func TestShowmigrationsListSquashed(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		c := newCommand(t, db, squashedMigrations(), djangotest.App("migrations"))
		djangotest.AssertEqual(t, c.call("showmigrations", "--list", "-v", "2"),
			"migrations\n [ ] 0001_squashed_0002 (2 squashed migrations)\n")
		out := c.call("migrate", "migrations", "0001_squashed_0002")
		defer c.call("migrate", "migrations", "zero")
		assertIn(t, out, "Operations to perform:\n"+
			"  Target specific migration: 0001_squashed_0002, from migrations\n"+
			"Running migrations:\n"+
			"  Applying migrations.0001_squashed_0002... OK")
		djangotest.AssertEqual(t, c.call("showmigrations", "--list", "-v", "2"),
			"migrations\n [X] 0001_squashed_0002 (2 squashed migrations)\n")
	})
}

// Ported from Django: tests/migrations/test_commands.py::MigrateTests.test_showmigrations_plan_squashed
func TestShowmigrationsPlanSquashed(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		c := newCommand(t, db, squashedComplexMigrations(), djangotest.App("migrations"))
		djangotest.AssertEqual(t, c.call("showmigrations", "--plan"),
			"[ ]  migrations.1_auto\n"+
				"[ ]  migrations.2_auto\n"+
				"[ ]  migrations.3_squashed_5\n"+
				"[ ]  migrations.6_auto\n"+
				"[ ]  migrations.7_auto\n")
		djangotest.AssertEqual(t, c.call("showmigrations", "--plan", "-v", "2"),
			"[ ]  migrations.1_auto\n"+
				"[ ]  migrations.2_auto ... (migrations.1_auto)\n"+
				"[ ]  migrations.3_squashed_5 ... (migrations.2_auto)\n"+
				"[ ]  migrations.6_auto ... (migrations.3_squashed_5)\n"+
				"[ ]  migrations.7_auto ... (migrations.6_auto)\n")

		c.call("migrate", "migrations", "3_squashed_5")
		defer c.call("migrate", "migrations", "zero")
		djangotest.AssertEqual(t, c.call("showmigrations", "--plan"),
			"[X]  migrations.1_auto\n"+
				"[X]  migrations.2_auto\n"+
				"[X]  migrations.3_squashed_5\n"+
				"[ ]  migrations.6_auto\n"+
				"[ ]  migrations.7_auto\n")
		djangotest.AssertEqual(t, c.call("showmigrations", "--plan", "-v", "2"),
			"[X]  migrations.1_auto\n"+
				"[X]  migrations.2_auto ... (migrations.1_auto)\n"+
				"[X]  migrations.3_squashed_5 ... (migrations.2_auto)\n"+
				"[ ]  migrations.6_auto ... (migrations.3_squashed_5)\n"+
				"[ ]  migrations.7_auto ... (migrations.6_auto)\n")
	})
}

// Ported from Django: tests/migrations/test_commands.py::MigrateTests.test_sqlmigrate_ambiguous_prefix_squashed_migrations
func TestSqlmigrateAmbiguousPrefixSquashedMigrations(t *testing.T) {
	c := newCommand(t, djangotest.NewDB(t), squashedMigrations(), djangotest.App("migrations"))
	_, err := c.fail("sqlmigrate", "migrations", "0001")
	assertCommandError(t, err, "More than one migration matches '0001' in app 'migrations'. Please be more specific.", 1)
}

// Ported from Django: tests/migrations/test_commands.py::MigrateTests.test_sqlmigrate_squashed_migration
func TestSqlmigrateSquashedMigration(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		c := newCommand(t, db, squashedMigrations(), djangotest.App("migrations"))
		out := c.call("sqlmigrate", "migrations", "0001_squashed_0002")
		assertIn(t, out, "-- Create model Author")
		assertIn(t, out, "-- Create model Book")
		assertNotIn(t, out, "-- Create model Tribble")
	})
}

// Ported from Django: tests/migrations/test_commands.py::MigrateTests.test_sqlmigrate_replaced_migration
func TestSqlmigrateReplacedMigration(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		c := newCommand(t, db, squashedMigrations(), djangotest.App("migrations"))
		out := c.call("sqlmigrate", "migrations", "0001_initial")
		assertIn(t, out, "-- Create model Author")
		assertIn(t, out, "-- Create model Tribble")
	})
}

// Running a squashed migration records the migrations it replaces as
// applied.
//
// Ported from Django: tests/migrations/test_commands.py::MigrateTests.test_migrate_record_replaced
func TestMigrateRecordReplaced(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		c := newCommand(t, db, squashedMigrations(), djangotest.App("migrations"))
		c.call("migrate", "migrations")
		defer c.call("migrate", "migrations", "zero")
		djangotest.AssertEqual(t, c.call("showmigrations", "migrations"),
			"migrations\n [X] 0001_squashed_0002 (2 squashed migrations)\n")
		rows := recorded(t, db)
		for _, name := range []string{"0001_initial", "0002_second", "0001_squashed_0002"} {
			if !rows[key("migrations", name)] {
				t.Errorf("migrations.%s is not recorded", name)
			}
		}
	})
}

// Migrate records a squashed migration as applied when all the migrations
// it replaces are applied.
//
// Ported from Django: tests/migrations/test_commands.py::MigrateTests.test_migrate_record_squashed
func TestMigrateRecordSquashed(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		c := newCommand(t, db, squashedMigrations(), djangotest.App("migrations"))
		recordApplied(t, db, key("migrations", "0001_initial"), key("migrations", "0002_second"))
		djangotest.AssertEqual(t, c.call("showmigrations", "migrations"),
			"migrations\n"+
				" [-] 0001_squashed_0002 (2 squashed migrations) Run 'manage.py migrate' to finish recording.\n")

		c.call("migrate", "migrations")
		djangotest.AssertEqual(t, c.call("showmigrations", "migrations"),
			"migrations\n [X] 0001_squashed_0002 (2 squashed migrations)\n")
		djangotest.AssertEqual(t, recorded(t, db)[key("migrations", "0001_squashed_0002")], true)
	})
}

// Migrating to a partly applied squashed migration by name works.
//
// Ported from Django: tests/migrations/test_commands.py::MigrateTests.test_migrate_partially_applied_squashed_migration
func TestMigratePartiallyAppliedSquashedMigration(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		c := newCommand(t, db, testMigrations(), djangotest.App("migrations"))
		defer c.call("migrate", "migrations", "zero")
		c.call("migrate", "migrations", "0001_initial")
		c.call("squashmigrations", "migrations", "0002", "--noinput", "-v", "0")
		c.call("migrate", "migrations", "0001_squashed_0002_second")
		djangotest.AssertEqual(t, recorded(t, db)[key("migrations", "0002_second")], true)
	})
}

// Ported from Django: tests/migrations/test_commands.py::MigrateTests.test_migrate_forward_to_squashed_migration
func TestMigrateForwardToSquashedMigration(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		c := newCommand(t, db, squashedMigrations(), djangotest.App("migrations"))
		defer c.call("migrate", "migrations", "zero")
		c.call("migrate", "migrations", "0001_initial")
	})
}

// Ported from Django: tests/migrations/test_commands.py::MigrateTests.test_migrate_backward_to_squashed_migration
func TestMigrateBackwardToSquashedMigration(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		c := newCommand(t, db, squashedMigrations(), djangotest.App("migrations"))
		defer c.call("migrate", "migrations", "zero")
		c.call("migrate", "migrations", "0001_squashed_0002")
		djangotest.AssertTableExists(t, db, "migrations_author")
		djangotest.AssertTableExists(t, db, "migrations_book")
		c.call("migrate", "migrations", "0001_initial")
		djangotest.AssertTableExists(t, db, "migrations_author")
		djangotest.AssertTableNotExists(t, db, "migrations_book")
	})
}

// With --prune, the rows of migrations deleted from the app, such as after
// squashing, leave the tracking table.
//
// Ported from Django: tests/migrations/test_commands.py::MigrateTests.test_migrate_prune
func TestMigratePrune(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		c := newCommand(t, db, squashedNoReplacesMigrations(), djangotest.App("migrations"))
		recordApplied(t, db, key("migrations", "0001_initial"), key("migrations", "0002_second"),
			key("migrations", "0001_squashed_0002"))
		djangotest.AssertEqual(t, c.call("migrate", "migrations", "--prune"),
			"Pruning migrations:\n"+
				"  Pruning migrations.0001_initial OK\n"+
				"  Pruning migrations.0002_second OK\n")
		djangotest.AssertSequenceEqual(t, slices.Collect(maps.Keys(recorded(t, db))),
			[]migrations.Key{key("migrations", "0001_squashed_0002")})
	})
}

// Ported from Django: tests/migrations/test_commands.py::MigrateTests.test_prune_deleted_squashed_migrations_in_replaces
func TestPruneDeletedSquashedMigrationsInReplaces(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		c := newCommand(t, db, squashedMigrations(), djangotest.App("migrations"))
		c.call("migrate", "migrations")
		defer c.call("migrate", "migrations", "zero")
		// Delete the replaced migrations.
		c.app.Migrations = migrations.NewRegistry()
		c.app.Migrations.Add(squashedMigrations()[1])
		// --prune refuses while the squashed migration has Replaces.
		djangotest.AssertEqual(t, c.call("migrate", "migrations", "--prune"),
			"Pruning migrations:\n"+
				"  Cannot use --prune because the following squashed migrations have their 'replaces' "+
				"attributes and may not be recorded as applied:\n"+
				"    migrations.0001_squashed_0002\n"+
				"  Re-run 'manage.py migrate' if they are not marked as applied, and remove 'replaces' "+
				"attributes in their Migration classes.\n")
	})
}

// Go has no verbosity option for migrate, so the second half of the Django
// test, which expects no output with verbosity 0, has no counterpart.
//
// Ported from Django: tests/migrations/test_commands.py::MigrateTests.test_prune_no_migrations_to_prune
func TestPruneNoMigrationsToPrune(t *testing.T) {
	c := newCommand(t, djangotest.NewDB(t), squashedMigrations(), djangotest.App("migrations"))
	djangotest.AssertEqual(t, c.call("migrate", "migrations", "--prune"),
		"Pruning migrations:\n  No migrations to prune.\n")
}

// Ported from Django: tests/migrations/test_commands.py::MigrateTests.test_prune_no_app_label
func TestPruneNoAppLabel(t *testing.T) {
	c := newCommand(t, djangotest.NewDB(t), testMigrations(), djangotest.App("migrations"))
	_, err := c.fail("migrate", "--prune")
	assertCommandError(t, err, "Migrations can be pruned only when an app is specified.", 1)
}

// Ported from Django: tests/migrations/test_commands.py::MigrateTests.test_prune_respect_app_label
func TestPruneRespectAppLabel(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		c := newCommand(t, db, join(squashedNoReplacesMigrations(), squashedWithReplacesMigrations2()),
			djangotest.App("migrations"), djangotest.App("migrations2"))
		recordApplied(t, db,
			key("migrations", "0001_initial"), key("migrations", "0002_second"), key("migrations", "0001_squashed_0002"),
			// The second app has a squashed migration with Replaces.
			key("migrations2", "0001_initial"), key("migrations2", "0002_second"), key("migrations2", "0001_squashed_0002"))
		djangotest.AssertEqual(t, c.call("migrate", "migrations", "--prune"),
			"Pruning migrations:\n"+
				"  Pruning migrations.0001_initial OK\n"+
				"  Pruning migrations.0002_second OK\n")
		rows := slices.SortedFunc(maps.Keys(recorded(t, db)), func(a, b migrations.Key) int {
			return cmp.Or(strings.Compare(a.App, b.App), strings.Compare(a.Name, b.Name))
		})
		djangotest.AssertSequenceEqual(t, rows, []migrations.Key{
			key("migrations", "0001_squashed_0002"),
			key("migrations2", "0001_initial"),
			key("migrations2", "0001_squashed_0002"),
			key("migrations2", "0002_second"),
		})
	})
}
