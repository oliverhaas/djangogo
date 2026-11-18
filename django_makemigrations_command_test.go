package djangogo_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/migrations"
	"github.com/oliverhaas/djangogo/orm"
)

// migrationsApp is Django's "migrations" test app with the two models its
// models.py registers.
func migrationsApp() djangotest.AppModels {
	return djangotest.App("migrations", &ModelWithCustomBase{}, &UnmigratedModel{})
}

// assertFile checks that a file exists.
func assertFile(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Errorf("migration file is missing: %v", err)
	}
}

// Django's model carries non-ASCII verbose names. Go does not write
// presentation options into a migration, so the port checks the default,
// which Django checks as well. A Go package needs no __init__ file.
//
// Ported from Django: tests/migrations/test_commands.py::MakeMigrationsTests.test_files_content
func TestFilesContent(t *testing.T) {
	type UnicodeModel struct {
		ID    int64
		Title string `orm:"max_length=20;default=“Ðjáñgó”"`
	}
	c := newCommand(t, nil, nil, djangotest.App("migrations", &UnicodeModel{}))
	c.call("makemigrations", "migrations")

	content := c.read("migrations", "0001_initial.go")
	assertIn(t, content, "migrations.CreateModel")
	assertIn(t, content, "Initial: true")
	assertIn(t, content, "“Ðjáñgó”")
}

// Ported from Django: tests/migrations/test_commands.py::MakeMigrationsTests.test_makemigrations_order
func TestMakemigrationsOrder(t *testing.T) {
	c := newCommand(t, nil, []migrations.Migration{{App: "migrations", Name: "0001"}}, migrationsApp())
	c.call("makemigrations", "migrations", "--empty", "-n", "a")
	assertFile(t, c.path("migrations", "0002_a.go"))
}

// Ported from Django: tests/migrations/test_commands.py::MakeMigrationsTests.test_makemigrations_conflict_exit
func TestMakemigrationsConflictExit(t *testing.T) {
	c := newCommand(t, nil, conflictMigrations(), migrationsApp())
	_, err := c.fail("makemigrations")
	assertCommandError(t, err,
		"Conflicting migrations detected; multiple leaf nodes in the migration graph: "+
			"(0002_conflicting_second, 0002_second in migrations).\n"+
			"To fix them run 'python manage.py makemigrations --merge'", 1)
}

// Ported from Django: tests/migrations/test_commands.py::MakeMigrationsTests.test_makemigrations_empty_no_app_specified
func TestMakemigrationsEmptyNoAppSpecified(t *testing.T) {
	c := newCommand(t, nil, nil, migrationsApp())
	_, err := c.fail("makemigrations", "--empty")
	assertCommandError(t, err, "You must supply at least one app label when using --empty.", 1)
}

// Ported from Django: tests/migrations/test_commands.py::MakeMigrationsTests.test_makemigrations_empty_migration
func TestMakemigrationsEmptyMigration(t *testing.T) {
	c := newCommand(t, nil, nil, migrationsApp())
	c.call("makemigrations", "migrations", "--empty")

	// An empty migration has no dependencies and no operations.
	content := c.read("migrations", "0001_initial.go")
	assertNotIn(t, content, "Dependencies")
	assertIn(t, content, "Operations: []migrations.Operation{},")
}

// Ported from Django: tests/migrations/test_commands.py::MakeMigrationsTests.test_makemigrations_no_changes_no_apps
func TestMakemigrationsNoChangesNoApps(t *testing.T) {
	c := newCommand(t, nil, nil)
	assertIn(t, c.call("makemigrations"), "No changes detected")
}

// Ported from Django: tests/migrations/test_commands.py::MakeMigrationsTests.test_makemigrations_no_changes
func TestMakemigrationsNoChanges(t *testing.T) {
	c := newCommand(t, nil, noChangesMigrations(), migrationsApp())
	assertIn(t, c.call("makemigrations", "migrations"), "No changes detected in app 'migrations'")
}

// Ported from Django: tests/migrations/test_commands.py::MakeMigrationsTests.test_makemigrations_no_apps_initial
func TestMakemigrationsNoAppsInitial(t *testing.T) {
	c := newCommand(t, nil, nil, migrationsApp())
	assertIn(t, c.call("makemigrations"), "0001_initial.go")
}

// Ported from Django: tests/migrations/test_commands.py::MakeMigrationsTests.test_makemigrations_migrations_announce
func TestMakemigrationsMigrationsAnnounce(t *testing.T) {
	c := newCommand(t, nil, nil, migrationsApp())
	assertIn(t, c.call("makemigrations", "migrations"), "Migrations for 'migrations'")
}

// Django prints the message and exits with status 3. The Go command returns
// it as an error that carries the status.
//
// Ported from Django: tests/migrations/test_commands.py::MakeMigrationsTests.test_makemigrations_non_interactive_not_null_addition
func TestMakemigrationsNonInteractiveNotNullAddition(t *testing.T) {
	type SillyModel struct {
		ID         int64
		SillyField bool `orm:"default=false"`
		SillyInt   int64
	}
	c := newCommand(t, nil, noDefaultMigrations(), djangotest.App("migrations", &SillyModel{}))
	_, err := c.fail("makemigrations", "migrations", "--noinput")
	assertCommandError(t, err,
		"Field 'silly_int' on model 'sillymodel' not migrated: it is impossible to add a non-nullable field "+
			"without specifying a default.", 3)
	djangotest.AssertEqual(t, len(c.files()), 0)
}

// The texts are Django's except where Django names Python: the last choice
// says "in the model" for "in models.py", and the default is entered as a Go
// literal.
//
// Ported from Django: tests/migrations/test_commands.py::MakeMigrationsTests.test_makemigrations_interactive_not_null_addition
func TestMakemigrationsInteractiveNotNullAddition(t *testing.T) {
	type Author struct {
		ID         int64
		SillyField bool
	}
	inputMsg := "It is impossible to add a non-nullable field 'silly_field' to author without specifying a default. " +
		"This is because the database needs something to populate existing rows.\n" +
		"Please select a fix:\n" +
		" 1) Provide a one-off default now (will be set on all existing rows with a null value for this column)\n" +
		" 2) Quit and manually define a default value in the model."
	c := newCommand(t, nil, testMigrations(), djangotest.App("migrations", &Author{}))

	// 2 - quit.
	c.input("2\n")
	output, err := c.fail("makemigrations", "migrations")
	assertIn(t, output, inputMsg)
	var cerr *migrations.CommandError
	if !errors.As(err, &cerr) || cerr.Code != 3 {
		t.Errorf("error = %v, want a CommandError with exit status 3", err)
	}
	djangotest.AssertEqual(t, len(c.files()), 0)

	// 1 - provide a default.
	c.input("1\ntrue\n")
	output = c.call("makemigrations", "migrations")
	assertIn(t, output, inputMsg)
	assertIn(t, output, "Please enter the default value as a Go literal.")
	assertIn(t, output, "Type 'exit' to exit this prompt")
	assertIn(t, c.read("migrations", "0003_remove_author_age_remove_author_name_and_more.go"), "Default: true,")
}

// Ported from Django: tests/migrations/test_commands.py::MakeMigrationsTests.test_makemigrations_non_interactive_not_null_alteration
func TestMakemigrationsNonInteractiveNotNullAlteration(t *testing.T) {
	type Author struct {
		ID   int64
		Name string
		Slug string `orm:"max_length=50;index"`
		Age  int64  `orm:"default=0"`
	}
	c := newCommand(t, nil, testMigrations(), djangotest.App("migrations", &Author{}))
	output := c.call("makemigrations", "migrations", "--noinput")
	assertIn(t, output, "Alter field slug on author")
	assertIn(t, output, "Field 'slug' on model 'author' given a default of NOT PROVIDED and must be corrected.")
}

// The texts are Django's except where Django names Python: "RunGo" stands for
// "RunPython", and the last choice says "in the model" for "in models.py".
//
// Ported from Django: tests/migrations/test_commands.py::MakeMigrationsTests.test_makemigrations_interactive_not_null_alteration
func TestMakemigrationsInteractiveNotNullAlteration(t *testing.T) {
	type Author struct {
		ID   int64
		Slug string `orm:"max_length=50;index"`
	}
	inputMsg := "It is impossible to change a nullable field 'slug' on author to non-nullable without providing a " +
		"default. This is because the database needs something to populate existing rows.\n" +
		"Please select a fix:\n" +
		" 1) Provide a one-off default now (will be set on all existing rows with a null value for this column)\n" +
		" 2) Ignore for now. Existing rows that contain NULL values will have to be handled manually, for " +
		"example with a RunGo or RunSQL operation.\n" +
		" 3) Quit and manually define a default value in the model."
	c := newCommand(t, nil, testMigrations(), djangotest.App("migrations", &Author{}))

	// No message appears if --dry-run.
	assertNotIn(t, c.call("makemigrations", "migrations", "--dry-run"), inputMsg)

	// 3 - quit.
	c.input("3\n")
	output, err := c.fail("makemigrations", "migrations")
	assertIn(t, output, inputMsg)
	var cerr *migrations.CommandError
	if !errors.As(err, &cerr) || cerr.Code != 3 {
		t.Errorf("error = %v, want a CommandError with exit status 3", err)
	}

	// 1 - provide a default.
	c.input("1\nnone\n")
	output = c.call("makemigrations", "migrations")
	assertIn(t, output, inputMsg)
	assertIn(t, output, "Please enter the default value as a Go literal.")
	assertIn(t, output, "Type 'exit' to exit this prompt")
	assertIn(t, c.read("migrations", "0003_remove_author_age_remove_author_name_and_more.go"), `Default: "none",`)
}

// declinedRenameWarning is the Go addition to Django's output: under
// --noinput a likely rename is reported, because nobody was asked.
const declinedRenameWarning = "looks like a renamed"

// Ported from Django: tests/migrations/test_commands.py::MakeMigrationsTests.test_makemigrations_non_interactive_no_model_rename
func TestMakemigrationsNonInteractiveNoModelRename(t *testing.T) {
	type RenamedModel struct {
		ID         int64
		SillyField bool `orm:"default=false"`
	}
	c := newCommand(t, nil, noDefaultMigrations(), djangotest.App("migrations", &RenamedModel{}))
	output := c.call("makemigrations", "migrations", "--noinput")
	assertIn(t, output, "Delete model SillyModel")
	assertIn(t, output, "Create model RenamedModel")
	assertIn(t, output, "Warning: migrations.SillyModel -> migrations.RenamedModel "+declinedRenameWarning+" model;")
}

// Ported from Django: tests/migrations/test_commands.py::MakeMigrationsTests.test_makemigrations_non_interactive_no_field_rename
func TestMakemigrationsNonInteractiveNoFieldRename(t *testing.T) {
	type SillyModel struct {
		ID          int64
		SillyRename bool `orm:"default=false"`
	}
	c := newCommand(t, nil, noDefaultMigrations(), djangotest.App("migrations", &SillyModel{}))
	output := c.call("makemigrations", "migrations", "--noinput")
	assertIn(t, output, "Remove field silly_field from sillymodel")
	assertIn(t, output, "Add field silly_rename to sillymodel")
	assertIn(t, output, "Warning: migrations.SillyModel.SillyField -> migrations.SillyModel.SillyRename "+
		declinedRenameWarning+" field;")
}

// Ported from Django: tests/migrations/test_commands.py::MakeMigrationsTests.test_makemigrations_model_rename_interactive
func TestMakemigrationsModelRenameInteractive(t *testing.T) {
	type RenamedModel struct {
		ID         int64
		SillyField bool `orm:"default=false"`
	}
	c := newCommand(t, nil, noDefaultMigrations(), djangotest.App("migrations", &RenamedModel{}))
	c.input("Y\n")
	output := c.call("makemigrations", "migrations")
	assertIn(t, output, "Was the model migrations.SillyModel renamed to RenamedModel? [y/N]")
	assertIn(t, output, "Rename model SillyModel to RenamedModel")
	assertNotIn(t, output, declinedRenameWarning)
}

// Ported from Django: tests/migrations/test_commands.py::MakeMigrationsTests.test_makemigrations_field_rename_interactive
func TestMakemigrationsFieldRenameInteractive(t *testing.T) {
	type SillyModel struct {
		ID          int64
		SillyRename bool `orm:"default=false"`
	}
	c := newCommand(t, nil, noDefaultMigrations(), djangotest.App("migrations", &SillyModel{}))
	c.input("Y\n")
	output := c.call("makemigrations", "migrations")
	assertIn(t, output, "Was sillymodel.silly_field renamed to sillymodel.silly_rename (a BooleanField)? [y/N]")
	assertIn(t, output, "Rename field silly_field on sillymodel to silly_rename")
	assertNotIn(t, output, declinedRenameWarning)
}

// Go has no DateField, so the added field without a default is a date-time.
//
// Ported from Django: tests/migrations/test_commands.py::MakeMigrationsTests.test_makemigrations_dry_run
func TestMakemigrationsDryRun(t *testing.T) {
	type SillyModel struct {
		ID           int64
		SillyField   bool `orm:"default=false"`
		SillyDate    time.Time
		SillyAutoNow time.Time `orm:"auto_now_add"`
	}
	c := newCommand(t, nil, noDefaultMigrations(), djangotest.App("migrations", &SillyModel{}))
	output := c.call("makemigrations", "migrations", "--dry-run")
	// Output the expected changes directly, without asking for defaults.
	assertIn(t, output, "Add field silly_date to sillymodel")
	djangotest.AssertEqual(t, len(c.files()), 0)
}

// Ported from Django: tests/migrations/test_commands.py::MakeMigrationsTests.test_makemigrations_with_custom_name
func TestMakemigrationsWithCustomName(t *testing.T) {
	c := newCommand(t, nil, nil, migrationsApp())

	// The initial migration has no dependencies.
	c.call("makemigrations", "migrations", "--name", "my_initial_migration")
	content := c.read("migrations", "0001_my_initial_migration.go")
	assertNotIn(t, content, "Dependencies")

	// An empty migration with a custom name depends on it.
	c.call("makemigrations", "migrations", "--name", "my_custom_migration", "--empty")
	content = c.read("migrations", "0002_my_custom_migration.go")
	assertIn(t, content, `{App: "migrations", Name: "0001_my_initial_migration"},`)
	assertIn(t, content, "Operations: []migrations.Operation{},")
}

// The name becomes part of a Go file name, so the message says "Go
// identifier" where Django says "Python identifier".
//
// Ported from Django: tests/migrations/test_commands.py::MakeMigrationsTests.test_makemigrations_with_invalid_custom_name
func TestMakemigrationsWithInvalidCustomName(t *testing.T) {
	c := newCommand(t, nil, nil, migrationsApp())
	_, err := c.fail("makemigrations", "migrations", "--name", "invalid name", "--empty")
	assertCommandError(t, err, "The migration name must be a valid Go identifier.", 1)
}

// Django exits with status 1 without a message. A Go command reports its
// status through the error it returns, which needs one.
//
// Ported from Django: tests/migrations/test_commands.py::MakeMigrationsTests.test_makemigrations_check_with_changes
func TestMakemigrationsCheckWithChanges(t *testing.T) {
	c := newCommand(t, nil, nil, migrationsApp())
	output, err := c.fail("makemigrations", "--check", "migrations")
	djangotest.AssertEqual(t, len(c.files()), 0)
	assertCommandError(t, err, "Model changes are missing from the migrations.", 1)
	assertIn(t, output, "Migrations for 'migrations':")
}

// Ported from Django: tests/migrations/test_commands.py::MakeMigrationsTests.test_makemigrations_check_no_changes
func TestMakemigrationsCheckNoChanges(t *testing.T) {
	c := newCommand(t, nil, noChangesMigrations(), migrationsApp())
	djangotest.AssertEqual(t, c.call("makemigrations", "--check", "migrations"),
		"No changes detected in app 'migrations'\n")
}

// Ported from Django: tests/migrations/test_commands.py::MakeMigrationsTests.test_makemigrations_migration_path_output
func TestMakemigrationsMigrationPathOutput(t *testing.T) {
	c := newCommand(t, nil, nil, migrationsApp())
	assertIn(t, c.call("makemigrations", "migrations"), c.path("migrations", "0001_initial.go"))
}

// Ported from Django: tests/migrations/test_commands.py::MakeMigrationsTests.test_makemigrations_inconsistent_history
func TestMakemigrationsInconsistentHistory(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		c := newCommand(t, db, testMigrations(), migrationsApp())
		if err := migrations.NewRecorder(db).RecordApplied(context.Background(), key("migrations", "0002_second")); err != nil {
			t.Fatalf("record applied: %v", err)
		}
		_, err := c.fail("makemigrations")
		var inconsistent *migrations.InconsistentMigrationHistoryError
		if !errors.As(err, &inconsistent) {
			t.Fatalf("error = %v (%T), want an InconsistentMigrationHistoryError", err, err)
		}
		assertIn(t, err.Error(),
			"Migration migrations.0002_second is applied before its dependency migrations.0001_initial")
	})
}
