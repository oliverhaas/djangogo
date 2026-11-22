package djangogo_test

import (
	"bytes"
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/migrations"
	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/orm/backends/sqlite"
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
			"To fix them run 'manage.py makemigrations --merge'", 1)
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
	files := c.files()
	djangotest.AssertEqual(t, len(files), 1)
	assertIn(t, c.read("migrations", files[0]), "Default: true,")
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
	files := c.files()
	djangotest.AssertEqual(t, len(files), 1)
	assertIn(t, c.read("migrations", files[0]), `Default: "none",`)
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

// assertNoFile checks that a file does not exist.
func assertNoFile(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err == nil {
		t.Errorf("%s exists", path)
	} else if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("%s: %v", path, err)
	}
}

// noAncestorMigrations mirrors Django's migrations.test_migrations_no_ancestor,
// whose 0002_conflicting_second depends on nothing.
func noAncestorMigrations() []migrations.Migration {
	migs := conflictMigrations()
	migs[1] = migrations.Migration{App: "migrations", Name: "0002_conflicting_second", Operations: secondOperations()}
	return migs
}

// conflictLongNameMigrations mirrors Django's
// migrations.test_migrations_conflict_long_name.
func conflictLongNameMigrations() []migrations.Migration {
	deps := []migrations.Key{key("migrations", "0001_initial")}
	return []migrations.Migration{
		{
			App: "migrations", Name: "0001_initial", Initial: true,
			Operations: []migrations.Operation{migrations.CreateModel{Name: "Author", Fields: []migrations.FieldState{idField}}},
		},
		{
			App: "migrations", Name: "0002_conflicting_second_migration_with_long_name", Dependencies: deps,
			Operations: []migrations.Operation{migrations.CreateModel{Name: "SomethingElse", Fields: []migrations.FieldState{idField}}},
		},
		{
			App: "migrations", Name: "0002_second", Dependencies: deps,
			Operations: []migrations.Operation{migrations.CreateModel{Name: "Something", Fields: []migrations.FieldState{idField}}},
		},
	}
}

// conflictingAppMigrations mirrors the migrations of Django's test apps
// unspecified_app_with_conflict and conflicting_app_with_dependencies, which
// differ only in the extra dependencies of 0002_second.
func conflictingAppMigrations(app string, secondDeps ...migrations.Key) []migrations.Migration {
	initial := []migrations.Key{key(app, "0001_initial")}
	return []migrations.Migration{
		{
			App: app, Name: "0001_initial",
			Operations: []migrations.Operation{
				migrations.CreateModel{Name: "Author", Fields: authorFields()},
				tribble(),
			},
		},
		{
			App: app, Name: "0002_conflicting_second", Dependencies: initial,
			Operations: []migrations.Operation{migrations.CreateModel{Name: "Something", Fields: []migrations.FieldState{idField}}},
		},
		{
			App: app, Name: "0002_second", Dependencies: slices.Concat(initial, secondDeps),
			Operations: []migrations.Operation{
				migrations.DeleteModel{Name: "Tribble"},
				migrations.RemoveField{Model: "Author", Field: "SillyField"},
				migrations.AddField{Model: "Author", Field: migrations.FieldState{
					Name: "Rating", Column: "rating", Kind: orm.KindInt, Default: int64(0),
				}},
				migrations.CreateModel{Name: "Book", Fields: []migrations.FieldState{idField}},
			},
		},
	}
}

// Ported from Django: tests/migrations/test_commands.py::MakeMigrationsTests.test_makemigrations_merge_no_conflict
func TestMakemigrationsMergeNoConflict(t *testing.T) {
	c := newCommand(t, nil, testMigrations(), migrationsApp())
	assertIn(t, c.call("makemigrations", "--merge"), "No conflicts detected to merge.")
}

// Ported from Django: tests/migrations/test_commands.py::MakeMigrationsTests.test_makemigrations_no_common_ancestor
func TestMakemigrationsNoCommonAncestor(t *testing.T) {
	c := newCommand(t, nil, noAncestorMigrations(), migrationsApp())
	_, err := c.fail("makemigrations", "migrations", "--merge")
	assertIn(t, err.Error(), "Could not find common ancestor of")
	assertIn(t, err.Error(), "0002_second")
	assertIn(t, err.Error(), "0002_conflicting_second")
}

// Ported from Django: tests/migrations/test_commands.py::MakeMigrationsTests.test_makemigrations_interactive_reject
func TestMakemigrationsInteractiveReject(t *testing.T) {
	c := newCommand(t, nil, conflictMigrations(), migrationsApp())
	c.input("N\n")
	c.call("makemigrations", "migrations", "--name", "merge", "--merge", "-v", "0")
	assertNoFile(t, c.path("migrations", "0003_merge.go"))
}

// Ported from Django: tests/migrations/test_commands.py::MakeMigrationsTests.test_makemigrations_interactive_accept
func TestMakemigrationsInteractiveAccept(t *testing.T) {
	c := newCommand(t, nil, conflictMigrations(), migrationsApp())
	c.input("y\n")
	out := c.call("makemigrations", "migrations", "--name", "merge", "--merge")
	assertFile(t, c.path("migrations", "0003_merge.go"))
	assertIn(t, out, "Created new merge migration")
}

// Ported from Django: tests/migrations/test_commands.py::MakeMigrationsTests.test_makemigrations_default_merge_name
func TestMakemigrationsDefaultMergeName(t *testing.T) {
	c := newCommand(t, nil, conflictMigrations(), migrationsApp())
	out := c.call("makemigrations", "migrations", "--merge", "--noinput")
	mergeFile := c.path("migrations", "0003_merge_0002_conflicting_second_0002_second.go")
	assertFile(t, mergeFile)
	content := c.read("migrations", "0003_merge_0002_conflicting_second_0002_second.go")
	assertIn(t, content, `{App: "migrations", Name: "0002_conflicting_second"}`)
	assertIn(t, out, "Created new merge migration "+mergeFile)
}

// Django patches the clock to get a fixed name. The clock of the migrations
// package cannot be replaced from here, so the test checks the shape of the
// name.
//
// Ported from Django: tests/migrations/test_commands.py::MakeMigrationsTests.test_makemigrations_auto_merge_name
func TestMakemigrationsAutoMergeName(t *testing.T) {
	c := newCommand(t, nil, conflictLongNameMigrations(), migrationsApp())
	c.input("y\n")
	out := c.call("makemigrations", "migrations", "--merge")
	files := c.files()
	if !slices.ContainsFunc(files, regexp.MustCompile(`^0003_merge_\d{8}_\d{4}\.go$`).MatchString) {
		t.Errorf("no merge migration named after the time in %v", files)
	}
	assertIn(t, out, "Created new merge migration")
}

// Ported from Django: tests/migrations/test_commands.py::MakeMigrationsTests.test_makemigrations_handle_merge
func TestMakemigrationsHandleMerge(t *testing.T) {
	c := newCommand(t, nil, conflictMigrations(), migrationsApp())
	out := c.call("makemigrations", "migrations", "--name", "merge", "--merge", "--noinput")
	assertFile(t, c.path("migrations", "0003_merge.go"))
	assertIn(t, out, "Merging migrations")
	assertIn(t, out, "Branch 0002_second")
	assertIn(t, out, "Branch 0002_conflicting_second")
	assertIn(t, out, "Created new merge migration")
}

// Ported from Django: tests/migrations/test_commands.py::MakeMigrationsTests.test_makemigration_merge_dry_run
func TestMakemigrationMergeDryRun(t *testing.T) {
	c := newCommand(t, nil, conflictMigrations(), migrationsApp())
	out := c.call("makemigrations", "migrations", "--name", "merge", "--dry-run", "--merge", "--noinput")
	assertNoFile(t, c.path("migrations", "0003_merge.go"))
	assertIn(t, out, "Merging migrations")
	assertIn(t, out, "Branch 0002_second")
	assertIn(t, out, "Branch 0002_conflicting_second")
	assertNotIn(t, out, "Created new merge migration")
}

// Ported from Django: tests/migrations/test_commands.py::MakeMigrationsTests.test_makemigration_merge_dry_run_verbosity_3
func TestMakemigrationMergeDryRunVerbosity3(t *testing.T) {
	c := newCommand(t, nil, conflictMigrations(), migrationsApp())
	out := c.call("makemigrations", "migrations", "--name", "merge", "--dry-run", "--merge", "--noinput", "-v", "3")
	assertNoFile(t, c.path("migrations", "0003_merge.go"))
	assertIn(t, out, "Merging migrations")
	assertIn(t, out, "Branch 0002_second")
	assertIn(t, out, "Branch 0002_conflicting_second")
	assertNotIn(t, out, "Created new merge migration")

	// The complete merge migration file that would be written.
	assertIn(t, out, "Full merge migrations file '0003_merge.go':")
	assertIn(t, out, "migrations.Migration{")
	assertIn(t, out, "Dependencies: []migrations.Key{")
	assertIn(t, out, `{App: "migrations", Name: "0002_second"}`)
	assertIn(t, out, `{App: "migrations", Name: "0002_conflicting_second"}`)
	assertIn(t, out, "Operations: []migrations.Operation{}")
}

// Ported from Django: tests/migrations/test_commands.py::MakeMigrationsTests.test_makemigrations_dry_run_verbosity_3
func TestMakemigrationsDryRunVerbosity3(t *testing.T) {
	type SillyModel struct {
		ID         int64
		SillyField bool   `orm:"default=false"`
		SillyChar  string `orm:"default="`
	}
	c := newCommand(t, nil, noDefaultMigrations(), djangotest.App("migrations", &SillyModel{}))
	out := c.call("makemigrations", "migrations", "--dry-run", "-v", "3")

	// Normal --dry-run output.
	assertIn(t, out, "+ Add field silly_char to sillymodel")

	// The complete migrations file that would be written.
	assertIn(t, out, "Full migrations file '0002_sillymodel_silly_char.go':")
	assertIn(t, out, "migrations.Migration{")
	assertIn(t, out, `{App: "migrations", Name: "0001_initial"}`)
	assertIn(t, out, "migrations.AddField{")
	assertIn(t, out, `Model: "SillyModel",`)
	assertIn(t, out, `Name: "SillyChar",`)
}

// Ported from Django: tests/migrations/test_commands.py::MakeMigrationsTests.test_makemigrations_scriptable
func TestMakemigrationsScriptable(t *testing.T) {
	c := newCommand(t, nil, nil, migrationsApp())
	errOut := &bytes.Buffer{}
	c.app.Err = errOut
	out := c.call("makemigrations", "migrations", "--scriptable")
	djangotest.AssertEqual(t, out, c.path("migrations", "0001_initial.go")+"\n")
	assertIn(t, errOut.String(), "    + Create model ModelWithCustomBase\n")
}

// Ported from Django: tests/migrations/test_commands.py::MakeMigrationsTests.test_makemigrations_scriptable_merge
func TestMakemigrationsScriptableMerge(t *testing.T) {
	c := newCommand(t, nil, conflictMigrations(), migrationsApp())
	errOut := &bytes.Buffer{}
	c.app.Err = errOut
	c.input("Y\n")
	out := c.call("makemigrations", "migrations", "--merge", "--name", "merge", "--scriptable")
	mergeFile := c.path("migrations", "0003_merge.go")
	djangotest.AssertEqual(t, out, mergeFile+"\n")
	assertIn(t, errOut.String(), "Created new merge migration "+mergeFile)
}

// Ported from Django: tests/migrations/test_commands.py::MakeMigrationsTests.test_makemigrations_interactive_by_default
func TestMakemigrationsInteractiveByDefault(t *testing.T) {
	c := newCommand(t, nil, conflictMigrations(), migrationsApp())
	c.input("N\n")
	out := c.call("makemigrations", "migrations", "--name", "merge", "--merge")
	// This fails if the command does not ask by default.
	assertNoFile(t, c.path("migrations", "0003_merge.go"))
	assertNotIn(t, out, "Created new merge migration")
}

// Ported from Django: tests/migrations/test_commands.py::MakeMigrationsTests.test_makemigrations_unspecified_app_with_conflict_no_merge
func TestMakemigrationsUnspecifiedAppWithConflictNoMerge(t *testing.T) {
	c := newCommand(t, nil,
		join(noChangesMigrations(), conflictingAppMigrations("unspecified_app_with_conflict")),
		migrationsApp(), djangotest.App("unspecified_app_with_conflict"))
	c.call("makemigrations", "migrations", "-v", "0")
}

// Ported from Django: tests/migrations/test_commands.py::MakeMigrationsTests.test_makemigrations_unspecified_app_with_conflict_merge
func TestMakemigrationsUnspecifiedAppWithConflictMerge(t *testing.T) {
	c := newCommand(t, nil,
		join(migratedAppsMigrations()[:1], conflictingAppMigrations("unspecified_app_with_conflict")),
		djangotest.App("migrated_app"), djangotest.App("unspecified_app_with_conflict"))
	c.input("y\n")
	out := c.call("makemigrations", "migrated_app", "--name", "merge", "--merge")
	assertNoFile(t, c.path("migrated_app", "0003_merge.go"))
	assertIn(t, out, "No conflicts detected to merge.")
}

// Ported from Django: tests/migrations/test_commands.py::MakeMigrationsTests.test_makemigrations_merge_dont_output_dependency_operations
func TestMakemigrationsMergeDontOutputDependencyOperations(t *testing.T) {
	const app = "conflicting_app_with_dependencies"
	c := newCommand(t, nil,
		join(migratedAppsMigrations()[:1], conflictingAppMigrations(app, key("migrated_app", "0001_initial"))),
		djangotest.App("migrated_app"), djangotest.App(app))
	c.input("N\n")
	djangotest.AssertEqual(t, strings.ToLower(c.call("makemigrations", app, "--merge")),
		"merging conflicting_app_with_dependencies\n"+
			"  branch 0002_conflicting_second\n"+
			"    + create model something\n"+
			"  branch 0002_second\n"+
			"    - delete model tribble\n"+
			"    - remove field silly_field from author\n"+
			"    + add field rating to author\n"+
			"    + create model book\n"+
			"\n"+
			"merging will only work if the operations printed above do not conflict\n"+
			"with each other (working on different fields or models)\n"+
			"should these migration branches be merged? [y/n] ")
}

// No Go counterpart: MakeMigrationsTests.test_merge_makemigrations_failure_to_format_code
// runs the code formatters on the merge migration; the writer formats with
// go/format itself.

// writeFiles writes the files of migs below the migrations directory, so a
// command that deletes or keeps them finds them, as Django's tests copy the
// migration modules into a temporary directory.
func (c *command) writeFiles(migs []migrations.Migration) {
	c.t.Helper()
	for _, m := range migs {
		if _, err := migrations.WriteMigration(filepath.Join(c.app.MigrationsDir, m.App), migrations.PackageName(m.App), m); err != nil {
			c.t.Fatalf("write %s: %v", m.Key(), err)
		}
	}
}

// Ported from Django: tests/migrations/test_commands.py::MakeMigrationsTests.test_makemigrations_update
func TestMakemigrationsUpdate(t *testing.T) {
	c := newCommand(t, nil, testMigrations(), migrationsApp())
	c.writeFiles(testMigrations())
	migrationFile := c.path("migrations", "0002_second.go")
	initialContent := c.read("migrations", "0002_second.go")

	out := c.call("makemigrations", "migrations", "--update")
	for _, name := range c.files() {
		if strings.HasPrefix(name, "0003") {
			t.Errorf("unexpected new migration %s", name)
		}
	}
	assertNoFile(t, migrationFile)
	newContent := c.read("migrations", "0002_delete_tribble_author_rating_modelwithcustombase_and_more.go")
	if newContent == initialContent {
		t.Error("the updated migration has the content of the old one")
	}
	assertIn(t, out, "Deleted "+migrationFile)
}

// autoNowAddMigrations mirrors Django's migrations.test_auto_now_add.
func autoNowAddMigrations() []migrations.Migration {
	return []migrations.Migration{{
		App: "migrations", Name: "0001_initial", Initial: true,
		Operations: []migrations.Operation{
			migrations.CreateModel{Name: "Entry", Fields: []migrations.FieldState{
				idField,
				{Name: "Title", Column: "title", Kind: orm.KindChar, MaxLength: 255},
			}},
		},
	}}
}

// Ported from Django: tests/migrations/test_commands.py::MakeMigrationsTests.test_makemigrations_update_existing_name
func TestMakemigrationsUpdateExistingName(t *testing.T) {
	c := newCommand(t, nil, autoNowAddMigrations(), migrationsApp())
	c.writeFiles(autoNowAddMigrations())
	migrationFile := c.path("migrations", "0001_initial.go")
	initialContent := c.read("migrations", "0001_initial.go")

	out := c.call("makemigrations", "migrations", "--update")
	assertNoFile(t, migrationFile)
	if c.read("migrations", "0001_initial_updated.go") == initialContent {
		t.Error("the updated migration has the content of the old one")
	}
	assertIn(t, out, "Deleted "+migrationFile)
}

// Ported from Django: tests/migrations/test_commands.py::MakeMigrationsTests.test_makemigrations_update_custom_name
func TestMakemigrationsUpdateCustomName(t *testing.T) {
	const customName = "delete_something"
	c := newCommand(t, nil, testMigrations(), migrationsApp())
	c.writeFiles(testMigrations())
	oldMigrationFile := c.path("migrations", "0002_second.go")
	initialContent := c.read("migrations", "0002_second.go")

	out := c.call("makemigrations", "migrations", "--update", "--name", customName)
	for _, name := range c.files() {
		if strings.HasPrefix(name, "0003") {
			t.Errorf("unexpected new migration %s", name)
		}
	}
	assertNoFile(t, oldMigrationFile)
	if c.read("migrations", "0002_"+customName+".go") == initialContent {
		t.Error("the updated migration has the content of the old one")
	}
	assertIn(t, out, "Deleted "+oldMigrationFile)
}

// Ported from Django: tests/migrations/test_commands.py::MakeMigrationsTests.test_makemigrations_update_applied_migration
func TestMakemigrationsUpdateAppliedMigration(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		c := newCommand(t, db, testMigrations(), migrationsApp())
		recordApplied(t, db, key("migrations", "0001_initial"), key("migrations", "0002_second"))
		_, err := c.fail("makemigrations", "migrations", "--update")
		assertCommandError(t, err, "Cannot update applied migration 'migrations.0002_second'.", 1)
	})
}

// Ported from Django: tests/migrations/test_commands.py::MakeMigrationsTests.test_makemigrations_update_no_migration
func TestMakemigrationsUpdateNoMigration(t *testing.T) {
	c := newCommand(t, nil, nil, migrationsApp())
	_, err := c.fail("makemigrations", "migrations", "--update")
	assertCommandError(t, err, "App migrations has no migration, cannot update last migration.", 1)
}

// Ported from Django: tests/migrations/test_commands.py::MakeMigrationsTests.test_makemigrations_update_squash_migration
func TestMakemigrationsUpdateSquashMigration(t *testing.T) {
	c := newCommand(t, nil, squashedMigrations(), migrationsApp())
	_, err := c.fail("makemigrations", "migrations", "--update")
	assertCommandError(t, err, "Cannot update squash migration 'migrations.0001_squashed_0002'.", 1)
}

// The fifth migration uses a function of a migration file, as Django's
// grow_tail and feed are.
//
// Ported from Django: tests/migrations/test_commands.py::MakeMigrationsTests.test_makemigrations_update_manual_porting
func TestMakemigrationsUpdateManualPorting(t *testing.T) {
	migs := planMigrations()
	migs[4].Operations = []migrations.Operation{
		migrations.RunGo{Forward: migrations.Noop},
		migrations.RunGo{Forward: ManualPortingForwards},
		migrations.RunGo{Forward: ManualPortingForwards, Reverse: migrations.Noop},
	}
	c := newCommand(t, nil, migs, migrationsApp())
	c.writeFiles(migs)

	out := c.call("makemigrations", "migrations", "--update")
	// The previous migration exists.
	previousMigrationFile := c.path("migrations", "0005_fifth.go")
	assertFile(t, previousMigrationFile)
	// The updated migration exists.
	var updated string
	for _, name := range c.files() {
		if strings.HasPrefix(name, "0005_auto") {
			updated = c.path("migrations", name)
		}
	}
	assertFile(t, updated)
	assertIn(t, out, "Updated migration "+updated+" requires manual porting.\n"+
		"Previous migration "+previousMigrationFile+" was kept and must be deleted after porting functions manually.")
}

// Ported from Django: tests/migrations/test_commands.py::MakeMigrationsTests.test_makemigrations_update_dependency_migration
func TestMakemigrationsUpdateDependencyMigration(t *testing.T) {
	c := newCommand(t, nil, alterFKMigrations(), djangotest.App("author_app"), djangotest.App("book_app"))
	_, err := c.fail("makemigrations", "book_app", "--update")
	assertCommandError(t, err,
		"Cannot update migration 'book_app.0001_initial' that migrations 'author_app.0002_alter_id' depend on.", 1)
}

// Ported from Django: tests/migrations/test_commands.py::MakeMigrationsTests.test_makemigrations_empty_connections
func TestMakemigrationsEmptyConnections(t *testing.T) {
	// With no apps.
	c := newCommand(t, nil, nil)
	assertIn(t, c.call("makemigrations"), "No changes detected")
	// With an app. A Go package needs no __init__ file; the port checks the
	// migration file instead.
	c = newCommand(t, nil, nil, migrationsApp())
	c.call("makemigrations", "migrations", "-v", "0")
	assertFile(t, c.path("migrations", "0001_initial.go"))
}

// Ported from Django: tests/migrations/test_commands.py::MakeMigrationsTests.test_makemigrations_migrations_modules_path_not_exist
func TestMakemigrationsMigrationsModulesPathNotExist(t *testing.T) {
	type SillyModel struct {
		ID         int64
		SillyField bool `orm:"default=false"`
	}
	c := newCommand(t, nil, nil, djangotest.App("migrations", &SillyModel{}))
	c.app.MigrationsDir = filepath.Join(c.app.MigrationsDir, "test_migrations_path_doesnt_exist", "foo", "bar")
	out := c.call("makemigrations", "migrations")

	// The migration file is created in the expected path.
	assertFile(t, c.path("migrations", "0001_initial.go"))
	// The output says the migration is created.
	assertIn(t, out, " + Create model SillyModel")
}

// unreachableDB connects to nothing, as Django's test makes the history check
// raise OperationalError.
type unreachableDB struct{}

func (unreachableDB) Connect(context.Context) (driver.Conn, error) {
	return nil, errors.New("could not connect to server")
}

func (unreachableDB) Driver() driver.Driver { return nil }

// Django warns with a RuntimeWarning; the port reads the warning from the
// error output.
//
// Ported from Django: tests/migrations/test_commands.py::MakeMigrationsTests.test_makemigrations_inconsistent_history_db_failure
func TestMakemigrationsInconsistentHistoryDBFailure(t *testing.T) {
	db := orm.NewDB(sql.OpenDB(unreachableDB{}), sqlite.New(), orm.NewRegistry())
	c := newCommand(t, db, testMigrations(), migrationsApp())
	errOut := &bytes.Buffer{}
	c.app.Err = errOut
	c.call("makemigrations", "-v", "0")
	assertIn(t, errOut.String(), "Got an error checking a consistent migration history performed "+
		"for database connection 'default': ")
	assertIn(t, errOut.String(), "could not connect to server")
}

// Django asks for a one-off default for a new auto_now_add field and offers
// timezone.now. Go asks nothing, as TestAddDateFieldsWithAutoNowAddAskingForDefault
// of the migrations package shows: existing rows get the time at which the
// migration runs. The port checks that the migration is made without a
// question.
//
// Ported from Django: tests/migrations/test_commands.py::MakeMigrationsTests.test_makemigrations_auto_now_add_interactive
func TestMakemigrationsAutoNowAddInteractive(t *testing.T) {
	type Entry struct {
		ID           int64
		Title        string    `orm:"max_length=255"`
		CreationDate time.Time `orm:"auto_now_add"`
	}
	c := newCommand(t, nil, autoNowAddMigrations(), djangotest.App("migrations", &Entry{}))
	out := c.call("makemigrations", "migrations")
	assertNotIn(t, out, "It is impossible to add the field 'creation_date'")
	assertIn(t, out, "Add field creation_date to entry")
}

// Django quits on the answer 2. Go asks nothing, so the migration is made;
// see TestMakemigrationsAutoNowAddInteractive.
//
// Ported from Django: tests/migrations/test_commands.py::MakeMigrationsTests.test_makemigrations_auto_now_add_interactive_quit
func TestMakemigrationsAutoNowAddInteractiveQuit(t *testing.T) {
	type Author struct {
		ID             int64
		PublishingDate orm.Date `orm:"auto_now_add"`
	}
	c := newCommand(t, nil, testMigrations(), djangotest.App("migrations", &Author{}))
	c.input("2\n")
	assertIn(t, c.call("makemigrations", "migrations"), "+ Add field publishing_date to author")
}

// Django exits with status 3 because nobody can supply the default. Go needs
// none; see TestMakemigrationsAutoNowAddInteractive.
//
// Ported from Django: tests/migrations/test_commands.py::MakeMigrationsTests.test_makemigrations_non_interactive_auto_now_add_addition
func TestMakemigrationsNonInteractiveAutoNowAddAddition(t *testing.T) {
	type Entry struct {
		ID           int64
		CreationDate time.Time `orm:"auto_now_add"`
	}
	c := newCommand(t, nil, autoNowAddMigrations(), djangotest.App("migrations", &Entry{}))
	out := c.call("makemigrations", "migrations", "--noinput")
	assertNotIn(t, out, "not migrated")
	assertIn(t, out, "+ Add field creation_date to entry")
}

// Ported from Django: tests/migrations/test_commands.py::MakeMigrationsTests.test_makemigrations_continues_number_sequence_after_squash
func TestMakemigrationsContinuesNumberSequenceAfterSquash(t *testing.T) {
	c := newCommand(t, nil, squashedMigrations(), migrationsApp())
	assertIn(t, c.call("makemigrations", "migrations", "--noinput", "--empty"), "0003_auto")
}

// No Go counterpart: these MakeMigrationsTests cover Python concepts.
//   - test_makemigrations_consistency_checks_respect_routers: there are no
//     database routers.
//   - test_failing_migration: a model default comes from a struct tag, which
//     the writer can always render, and the file is written only after it is
//     rendered.
//   - test_makemigrations_disabled_migrations_for_app and
//     test_makemigrations_migrations_modules_nonexistent_toplevel_package:
//     there is no MIGRATION_MODULES setting; the migrations of an app live in
//     a directory below MigrationsDir.
//   - test_makemigrations_no_init and test_makemigrations_no_init_ambiguous:
//     a Go package needs no __init__ file.
//   - test_makemigrations_failure_to_format_code: the writer formats with
//     go/format itself.
//   - test_makemigrations_migration_path_output_valueerror: filepath.Rel fails
//     only between Windows volumes, which a test cannot set up; displayPath
//     prints the absolute path then.
//   - test_makemigrations_interactive_unique_callable_default_addition and
//     test_makemigrations_non_interactive_unique_callable_default_addition: a
//     field default is a constant, never a callable.
