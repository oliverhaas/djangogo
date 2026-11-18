package djangogo_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/migrations"
	"github.com/oliverhaas/djangogo/orm"
)

// Go has no verbosity option. The lines Django prints only with verbosity 2
// ("Running pre-migrate handlers ...") are not part of the output.
//
// Ported from Django: tests/migrations/test_commands.py::MigrateTests.test_migrate
func TestMigrate(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		c := newCommand(t, db, testMigrations(), djangotest.App("migrations"))
		djangotest.AssertTableNotExists(t, db, "migrations_author")
		djangotest.AssertTableNotExists(t, db, "migrations_tribble")
		djangotest.AssertTableNotExists(t, db, "migrations_book")

		// Run the migrations to 0001 only.
		stdout := c.call("migrate", "migrations", "0001")
		assertIn(t, stdout, "Target specific migration: 0001_initial, from migrations")
		assertIn(t, stdout, "Applying migrations.0001_initial... OK")
		djangotest.AssertTableExists(t, db, "migrations_author")
		djangotest.AssertTableExists(t, db, "migrations_tribble")
		djangotest.AssertTableNotExists(t, db, "migrations_book")

		// Run migrations all the way.
		c.call("migrate")
		djangotest.AssertTableExists(t, db, "migrations_author")
		djangotest.AssertTableNotExists(t, db, "migrations_tribble")
		djangotest.AssertTableExists(t, db, "migrations_book")

		// Unmigrate everything.
		stdout = c.call("migrate", "migrations", "zero")
		assertIn(t, stdout, "Unapply all migrations: migrations")
		assertIn(t, stdout, "Unapplying migrations.0002_second... OK")
		djangotest.AssertTableNotExists(t, db, "migrations_author")
		djangotest.AssertTableNotExists(t, db, "migrations_tribble")
		djangotest.AssertTableNotExists(t, db, "migrations_book")
	})
}

// Ported from Django: tests/migrations/test_commands.py::MigrateTests.test_app_without_migrations
func TestAppWithoutMigrations(t *testing.T) {
	db := djangotest.NewDB(t)
	c := newCommand(t, db, testMigrations(),
		djangotest.App("migrations"), djangotest.App("unmigrated_app_syncdb"))
	_, err := c.fail("migrate", "unmigrated_app_syncdb")
	assertCommandError(t, err, "App 'unmigrated_app_syncdb' does not have migrations.", 1)
}

// Ported from Django: tests/migrations/test_commands.py::MigrateTests.test_ambiguous_prefix
func TestAmbiguousPrefix(t *testing.T) {
	db := djangotest.NewDB(t)
	clashing := []migrations.Migration{
		{App: "migrations", Name: "a"},
		{App: "migrations", Name: "ab", Dependencies: []migrations.Key{key("migrations", "a")}},
	}
	c := newCommand(t, db, clashing, djangotest.App("migrations"))
	_, err := c.fail("migrate", "migrations", "a")
	assertCommandError(t, err,
		"More than one migration matches 'a' in app 'migrations'. Please be more specific.", 1)
}

// Ported from Django: tests/migrations/test_commands.py::MigrateTests.test_unknown_prefix
func TestUnknownPrefix(t *testing.T) {
	db := djangotest.NewDB(t)
	c := newCommand(t, db, testMigrations(), djangotest.App("migrations"))
	_, err := c.fail("migrate", "migrations", "nonexistent")
	assertCommandError(t, err, "Cannot find a migration matching 'nonexistent' from app 'migrations'.", 1)
}

// Ported from Django: tests/migrations/test_commands.py::MigrateTests.test_migrate_conflict_exit
func TestMigrateConflictExit(t *testing.T) {
	db := djangotest.NewDB(t)
	c := newCommand(t, db, conflictMigrations(), djangotest.App("migrations"))
	_, err := c.fail("migrate", "migrations")
	assertCommandError(t, err,
		"Conflicting migrations detected; multiple leaf nodes in the migration graph: "+
			"(0002_conflicting_second, 0002_second in migrations).\n"+
			"To fix them run 'python manage.py makemigrations --merge'", 1)
}

// Go prints neither colors nor, without a verbosity option, the time at which
// a migration was applied.
//
// Ported from Django: tests/migrations/test_commands.py::MigrateTests.test_showmigrations_list
func TestShowmigrationsList(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		c := newCommand(t, db, testMigrations(), djangotest.App("migrations"))
		djangotest.AssertEqual(t, c.call("showmigrations", "--list"),
			"migrations\n [ ] 0001_initial\n [ ] 0002_second\n")

		c.call("migrate", "migrations", "0001")
		djangotest.AssertEqual(t, c.call("showmigrations", "migrations", "--list"),
			"migrations\n [X] 0001_initial\n [ ] 0002_second\n")
		// The list is the default format.
		djangotest.AssertEqual(t, c.call("showmigrations", "migrations"),
			"migrations\n [X] 0001_initial\n [ ] 0002_second\n")
		c.call("migrate", "migrations", "zero")
	})
}

// Without a verbosity option Go does not print the dependencies of each
// migration.
//
// Ported from Django: tests/migrations/test_commands.py::MigrateTests.test_showmigrations_plan
func TestShowmigrationsPlan(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		c := newCommand(t, db, runBeforeMigrations(), djangotest.App("migrations"))
		djangotest.AssertEqual(t, c.call("showmigrations", "--plan"),
			"[ ]  migrations.0001_initial\n"+
				"[ ]  migrations.0003_third\n"+
				"[ ]  migrations.0002_second\n")

		c.call("migrate", "migrations", "0003")
		djangotest.AssertEqual(t, c.call("showmigrations", "--plan"),
			"[X]  migrations.0001_initial\n"+
				"[X]  migrations.0003_third\n"+
				"[ ]  migrations.0002_second\n")
		c.call("migrate", "migrations", "zero")
	})
}

// Django shows the docstring of a RunPython function and the list of SQL
// statements as Python prints it. A Go function has no docstring, so RunGo
// shows no action, and the statements of RunSQL are shown as they are.
//
// Ported from Django: tests/migrations/test_commands.py::MigrateTests.test_migrate_plan
func TestMigratePlan(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		c := newCommand(t, db, planMigrations(), djangotest.App("migrations"))
		// Show the plan up to the third migration.
		djangotest.AssertEqual(t, c.call("migrate", "migrations", "0003", "--plan"),
			"Planned operations:\n"+
				"migrations.0001_initial\n"+
				"    Create model Salamander\n"+
				"    Raw Go operation\n"+
				"migrations.0002_second\n"+
				"    Create model Book\n"+
				"    Raw SQL operation -> SELECT * FROM migrations_book\n"+
				"migrations.0003_third\n"+
				"    Create model Author\n"+
				"    Raw SQL operation -> SELECT * FROM migrations_author\n")
		// Showing the plan creates no tables.
		djangotest.AssertTableNotExists(t, db, "migrations_salamander")

		// Migrate to the third migration.
		c.call("migrate", "migrations", "0003")
		djangotest.AssertEqual(t, c.call("migrate", "migrations", "0003", "--plan"),
			"Planned operations:\n  No planned migration operations.\n")

		// Show the plan when an operation is irreversible.
		djangotest.AssertEqual(t, c.call("migrate", "migrations", "0001", "--plan"),
			"Planned operations:\n"+
				"migrations.0003_third\n"+
				"    Undo Create model Author\n"+
				"    Raw SQL operation -> SELECT * FROM migrations_book\n"+
				"migrations.0002_second\n"+
				"    Undo Create model Book\n"+
				"    Raw SQL operation -> SELECT * FROM migrations_salamander\n")
		// A long action is cut off.
		djangotest.AssertEqual(t, c.call("migrate", "migrations", "0004", "--plan"),
			"Planned operations:\n"+
				"migrations.0004_fourth\n"+
				"    Raw SQL operation -> SELECT * FROM migrations_author WHE…\n")

		// Migrate to the fourth migration.
		c.call("migrate", "migrations", "0004")
		djangotest.AssertEqual(t, c.call("migrate", "migrations", "0003", "--plan"),
			"Planned operations:\n"+
				"migrations.0004_fourth\n"+
				"    Raw SQL operation -> IRREVERSIBLE\n")
		djangotest.AssertEqual(t, c.call("migrate", "migrations", "0005", "--plan"),
			"Planned operations:\n"+
				"migrations.0005_fifth\n"+
				"    Raw Go operation\n"+
				"    Raw Go operation\n"+
				"    Raw Go operation\n")

		c.call("migrate", "migrations", "0005")
		djangotest.AssertEqual(t, c.call("migrate", "migrations", "0004", "--plan"),
			"Planned operations:\n"+
				"migrations.0005_fifth\n"+
				"    Raw Go operation -> IRREVERSIBLE\n"+
				"    Raw Go operation -> IRREVERSIBLE\n"+
				"    Raw Go operation\n")

		// The fourth and fifth migrations cannot be unapplied, so fake the
		// way back before the rest is undone.
		stdout := c.call("migrate", "migrations", "0003", "--fake")
		assertIn(t, stdout, "Unapplying migrations.0005_fifth... FAKED")
		assertIn(t, stdout, "Unapplying migrations.0004_fourth... FAKED")
		c.call("migrate", "migrations", "zero")
		djangotest.AssertTableNotExists(t, db, "migrations_salamander")
	})
}

// Ported from Django: tests/migrations/test_commands.py::MigrateTests.test_showmigrations_unmigrated_app
func TestShowmigrationsUnmigratedApp(t *testing.T) {
	c := newCommand(t, djangotest.NewDB(t), nil, djangotest.App("unmigrated_app"))
	djangotest.AssertEqual(t, c.call("showmigrations", "unmigrated_app"), "unmigrated_app\n (no migrations)\n")
}

// Ported from Django: tests/migrations/test_commands.py::MigrateTests.test_showmigrations_plan_no_migrations
func TestShowmigrationsPlanNoMigrations(t *testing.T) {
	c := newCommand(t, djangotest.NewDB(t), nil, djangotest.App("migrations"))
	djangotest.AssertEqual(t, c.call("showmigrations", "--plan"), "(no migrations)\n")
}

// alterFKApps are the apps of the two tests below.
func alterFKApps(t *testing.T, db *orm.DB) *command {
	t.Helper()
	return newCommand(t, db, join(mutateStateBMigrations(), alterFKMigrations()),
		djangotest.App("mutate_state_b"), djangotest.App("author_app"), djangotest.App("book_app"))
}

// Ported from Django: tests/migrations/test_commands.py::MigrateTests.test_showmigrations_plan_single_app_label
func TestShowmigrationsPlanSingleAppLabel(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		c := alterFKApps(t, db)
		// A single app with no dependencies on other apps.
		djangotest.AssertEqual(t, c.call("showmigrations", "mutate_state_b", "--plan"),
			"[ ]  mutate_state_b.0001_initial\n"+
				"[ ]  mutate_state_b.0002_add_field\n")
		// A single app with dependencies.
		djangotest.AssertEqual(t, c.call("showmigrations", "author_app", "--plan"),
			"[ ]  author_app.0001_initial\n"+
				"[ ]  book_app.0001_initial\n"+
				"[ ]  author_app.0002_alter_id\n")
		// Some migrations already applied.
		c.call("migrate", "author_app", "0001")
		djangotest.AssertEqual(t, c.call("showmigrations", "author_app", "--plan"),
			"[X]  author_app.0001_initial\n"+
				"[ ]  book_app.0001_initial\n"+
				"[ ]  author_app.0002_alter_id\n")
		c.call("migrate", "author_app", "zero")
	})
}

// Ported from Django: tests/migrations/test_commands.py::MigrateTests.test_showmigrations_plan_multiple_app_labels
func TestShowmigrationsPlanMultipleAppLabels(t *testing.T) {
	c := alterFKApps(t, djangotest.NewDB(t))
	want := "[ ]  author_app.0001_initial\n" +
		"[ ]  book_app.0001_initial\n" +
		"[ ]  author_app.0002_alter_id\n" +
		"[ ]  mutate_state_b.0001_initial\n" +
		"[ ]  mutate_state_b.0002_add_field\n"
	// Multiple apps: the plan is independent of the order of the labels.
	djangotest.AssertEqual(t, c.call("showmigrations", "mutate_state_b", "author_app", "--plan"), want)
	djangotest.AssertEqual(t, c.call("showmigrations", "author_app", "mutate_state_b", "--plan"), want)
}

// Ported from Django: tests/migrations/test_commands.py::MigrateTests.test_showmigrations_plan_app_label_no_migrations
func TestShowmigrationsPlanAppLabelNoMigrations(t *testing.T) {
	c := newCommand(t, djangotest.NewDB(t), nil, djangotest.App("unmigrated_app"))
	djangotest.AssertEqual(t, c.call("showmigrations", "unmigrated_app", "--plan"), "(no migrations)\n")
}

// sqlLines returns the lines sqlmigrate printed without the transaction
// around them. SQLite and PostgreSQL both roll back DDL, so it is always
// there.
func sqlLines(t *testing.T, output string) []string {
	t.Helper()
	lines := strings.Split(strings.TrimSuffix(output, "\n"), "\n")
	if len(lines) < 2 || lines[0] != "BEGIN;" || lines[len(lines)-1] != "COMMIT;" {
		t.Fatalf("output is not wrapped in a transaction:\n%s", output)
	}
	return lines[1 : len(lines)-1]
}

// indexFrom returns the position of the first "--" line at or after start.
func indexFrom(t *testing.T, lines []string, start int) int {
	t.Helper()
	for i := start; i < len(lines); i++ {
		if lines[i] == "--" {
			return i
		}
	}
	t.Fatalf("no comment at or after line %d:\n%s", start, strings.Join(lines, "\n"))
	return -1
}

// assertHeader checks that lines has the comment of an operation at pos.
func assertHeader(t *testing.T, lines []string, pos int, description string) {
	t.Helper()
	if pos+3 > len(lines) {
		t.Fatalf("no comment at line %d:\n%s", pos, strings.Join(lines, "\n"))
	}
	djangotest.AssertSequenceEqual(t, lines[pos:pos+3], []string{"--", "-- " + description, "--"})
}

// Ported from Django: tests/migrations/test_commands.py::MigrateTests.test_sqlmigrate_forwards
func TestSqlmigrateForwards(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		c := newCommand(t, db, testMigrations(), djangotest.App("migrations"))
		lines := sqlLines(t, c.call("sqlmigrate", "migrations", "0001"))
		quote := db.Dialect().Quote

		assertHeader(t, lines, 0, "Create model Author")
		assertIn(t, strings.ToLower(lines[3]), "create table "+quote("migrations_author"))
		pos := indexFrom(t, lines, 3)
		assertHeader(t, lines, pos, "Create model Tribble")
		assertIn(t, strings.ToLower(lines[pos+3]), "create table "+quote("migrations_tribble"))
		pos = indexFrom(t, lines, pos+3)
		assertHeader(t, lines, pos, "Add field bool to tribble")
		pos = indexFrom(t, lines, pos+3)
		assertHeader(t, lines, pos, "Alter unique_together for author (1 constraint(s))")

		// Printing the SQL runs nothing.
		djangotest.AssertTableNotExists(t, db, "migrations_author")
	})
}

// Ported from Django: tests/migrations/test_commands.py::MigrateTests.test_sqlmigrate_backwards
func TestSqlmigrateBackwards(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		c := newCommand(t, db, testMigrations(), djangotest.App("migrations"))
		// Cannot generate the reverse SQL unless the migration is applied.
		c.call("migrate", "migrations")
		lines := sqlLines(t, c.call("sqlmigrate", "migrations", "0001", "--backwards"))
		quote := db.Dialect().Quote

		assertHeader(t, lines, 0, "Alter unique_together for author (1 constraint(s))")
		pos := indexFrom(t, lines, 3)
		assertHeader(t, lines, pos, "Add field bool to tribble")
		pos = indexFrom(t, lines, pos+3)
		assertHeader(t, lines, pos, "Create model Tribble")
		next := indexFrom(t, lines, pos+3)
		assertIn(t, strings.ToLower(strings.Join(lines[pos+3:next], "\n")), "drop table "+quote("migrations_tribble"))
		assertHeader(t, lines, next, "Create model Author")
		assertIn(t, strings.ToLower(strings.Join(lines[next+3:], "\n")), "drop table "+quote("migrations_author"))

		c.call("migrate", "migrations", "zero")
	})
}

// Django prints the message to stderr. A Go command has one output.
//
// Ported from Django: tests/migrations/test_commands.py::MigrateTests.test_sqlmigrate_no_operations
func TestSqlmigrateNoOperations(t *testing.T) {
	empty := []migrations.Migration{{App: "migrations", Name: "0001_initial"}}
	c := newCommand(t, djangotest.NewDB(t), empty, djangotest.App("migrations"))
	djangotest.AssertEqual(t, c.call("sqlmigrate", "migrations", "0001_initial"), "No operations found.\n")
}

// Ported from Django: tests/migrations/test_commands.py::MigrateTests.test_sqlmigrate_noop
func TestSqlmigrateNoop(t *testing.T) {
	noop := []migrations.Migration{{
		App: "migrations", Name: "0001_initial", Initial: true,
		Operations: []migrations.Operation{migrations.RunSQL{SQL: []string{""}, ReverseSQL: []string{""}}},
	}}
	c := newCommand(t, djangotest.NewDB(t), noop, djangotest.App("migrations"))
	lines := sqlLines(t, c.call("sqlmigrate", "migrations", "0001"))
	djangotest.AssertSequenceEqual(t, lines, []string{"--", "-- Raw SQL operation", "--", "-- (no-op)"})
}

// Django's message ends after the dependency when the database alias is not
// part of the assertion; the full text names the database 'default', as here.
//
// Ported from Django: tests/migrations/test_commands.py::MigrateTests.test_migrate_inconsistent_history
func TestMigrateInconsistentHistory(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		c := newCommand(t, db, testMigrations(), djangotest.App("migrations"))
		recorder := migrations.NewRecorder(db)
		if err := recorder.RecordApplied(ctx, key("migrations", "0002_second")); err != nil {
			t.Fatalf("record applied: %v", err)
		}
		_, err := c.fail("migrate")
		var inconsistent *migrations.InconsistentMigrationHistoryError
		if !errors.As(err, &inconsistent) {
			t.Fatalf("error = %v (%T), want an InconsistentMigrationHistoryError", err, err)
		}
		assertIn(t, err.Error(),
			"Migration migrations.0002_second is applied before its dependency migrations.0001_initial")
		applied, err := recorder.AppliedMigrations(ctx)
		if err != nil {
			t.Fatalf("applied migrations: %v", err)
		}
		if _, ok := applied[key("migrations", "0001_initial")]; ok {
			t.Error("0001_initial is recorded as applied")
		}
	})
}

// Ported from Django: tests/migrations/test_commands.py::MigrateTests.test_migrate_not_reflected_changes
func TestMigrateNotReflectedChanges(t *testing.T) {
	type NewModel1 struct{ ID int64 }
	type NewModel2 struct{ ID int64 }
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		c := newCommand(t, db, migratedAppsMigrations(),
			djangotest.App("migrated_unapplied_app", &NewModel2{}),
			djangotest.App("migrated_app", &NewModel1{}))
		c.call("migrate")
		djangotest.AssertEqual(t, c.call("migrate"),
			"Operations to perform:\n"+
				"  Apply all migrations: migrated_app, migrated_unapplied_app\n"+
				"Running migrations:\n"+
				"  No migrations to apply.\n"+
				"  Your models in app(s): 'migrated_app', 'migrated_unapplied_app' have changes that are not "+
				"yet reflected in a migration, and so won't be applied.\n"+
				"  Run 'manage.py makemigrations' to make new migrations, and then re-run 'manage.py migrate' "+
				"to apply them.\n")
		c.call("migrate", "migrated_app", "zero")
		c.call("migrate", "migrated_unapplied_app", "zero")
	})
}

// nonexistentAppError is Django's message for a label that names no app.
const nonexistentAppError = "No installed app with label 'nonexistent_app'."

// Django's makemigrations prints the message and exits with status 2. The Go
// command returns it as an error that carries the status.
//
// Ported from Django: tests/migrations/test_commands.py::AppLabelErrorTests.test_makemigrations_nonexistent_app_label
func TestMakemigrationsNonexistentAppLabel(t *testing.T) {
	c := newCommand(t, nil, nil, djangotest.App("migrations"))
	_, err := c.fail("makemigrations", "nonexistent_app")
	assertCommandError(t, err, nonexistentAppError, 2)
}

// Ported from Django: tests/migrations/test_commands.py::AppLabelErrorTests.test_migrate_nonexistent_app_label
func TestMigrateNonexistentAppLabel(t *testing.T) {
	c := newCommand(t, djangotest.NewDB(t), nil, djangotest.App("migrations"))
	_, err := c.fail("migrate", "nonexistent_app")
	assertCommandError(t, err, nonexistentAppError, 1)
}

// Ported from Django: tests/migrations/test_commands.py::AppLabelErrorTests.test_showmigrations_nonexistent_app_label
func TestShowmigrationsNonexistentAppLabel(t *testing.T) {
	c := newCommand(t, djangotest.NewDB(t), nil, djangotest.App("migrations"))
	_, err := c.fail("showmigrations", "nonexistent_app")
	assertCommandError(t, err, nonexistentAppError, 2)
}

// Ported from Django: tests/migrations/test_commands.py::AppLabelErrorTests.test_sqlmigrate_nonexistent_app_label
func TestSqlmigrateNonexistentAppLabel(t *testing.T) {
	c := newCommand(t, djangotest.NewDB(t), nil, djangotest.App("migrations"))
	_, err := c.fail("sqlmigrate", "nonexistent_app", "0002")
	assertCommandError(t, err, nonexistentAppError, 1)
}
