package djangogo_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/apps"
	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/migrations"
	"github.com/oliverhaas/djangogo/orm"
)

// The tests in this file cover behavior of the migration commands that
// Django's test suite has no fitting test for.

// Writer and Novel are the models of two apps with a foreign key from one
// app to the other.
type Writer struct {
	ID   int64
	Name string `orm:"max_length=100"`
}

type Novel struct {
	ID     int64
	Title  string         `orm:"max_length=200;index"`
	Writer orm.FK[Writer] `orm:"on_delete=cascade"`
}

// Club and Member are the models of two apps that reference each other.
type Club struct {
	ID      int64
	Founder orm.FK[Member] `orm:"null;on_delete=set_null"`
}

type Member struct {
	ID   int64
	Club orm.FK[Club] `orm:"on_delete=cascade"`
}

// next returns the project after the developer changed the models: a new
// application with the given apps that shares the database, the migrations
// and the migrations directory of c.
func (c *command) next(configs ...apps.Config) *command {
	c.t.Helper()
	n := newCommand(c.t, c.app.DB, nil, configs...)
	n.app.Migrations = c.app.Migrations
	n.app.MigrationsDir = c.app.MigrationsDir
	return n
}

// exec runs a statement on the database of the command.
func (c *command) exec(query string) {
	c.t.Helper()
	if _, err := c.app.DB.SQL().ExecContext(context.Background(), query); err != nil {
		c.t.Fatalf("%s: %v", query, err)
	}
}

// queryString returns the single text value a query selects.
func (c *command) queryString(query string) string {
	c.t.Helper()
	var value string
	if err := c.app.DB.SQL().QueryRowContext(context.Background(), query).Scan(&value); err != nil {
		c.t.Fatalf("%s: %v", query, err)
	}
	return value
}

// queryInt returns the single integer a query selects.
func (c *command) queryInt(query string) int64 {
	c.t.Helper()
	var value int64
	if err := c.app.DB.SQL().QueryRowContext(context.Background(), query).Scan(&value); err != nil {
		c.t.Fatalf("%s: %v", query, err)
	}
	return value
}

// TestMakemigrationsCrossAppDependency checks that every app gets its own
// history and that a foreign key into another app becomes a dependency on
// the migration that creates the target.
func TestMakemigrationsCrossAppDependency(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		c := newCommand(t, db, nil, djangotest.App("books", &Novel{}), djangotest.App("people", &Writer{}))
		output := c.call("makemigrations")
		assertIn(t, output, "Migrations for 'books':")
		assertIn(t, output, "Migrations for 'people':")

		books := c.read("books", "0001_initial.go")
		assertIn(t, books, "package books")
		assertIn(t, books, `{App: "people", Name: "0001_initial"},`)
		people := c.read("people", "0001_initial.go")
		assertIn(t, people, "package people")
		assertNotIn(t, people, "Dependencies")

		// The target of the foreign key is created first.
		output = c.call("migrate")
		first := strings.Index(output, "Applying people.0001_initial... OK")
		second := strings.Index(output, "Applying books.0001_initial... OK")
		if first < 0 || second < first {
			t.Errorf("people is not migrated before books:\n%s", output)
		}
		djangotest.AssertFKExists(t, db, "books_novel", "writer_id", "people_writer", "id")
		djangotest.AssertIndexExists(t, db, "books_novel", "title")

		djangotest.AssertEqual(t, c.call("makemigrations"), "No changes detected\n")
		djangotest.AssertEqual(t, c.call("showmigrations"),
			"books\n [X] 0001_initial\npeople\n [X] 0001_initial\n")

		// Unapplying the target takes the app that depends on it along.
		output = c.call("migrate", "people", "zero")
		assertIn(t, output, "Unapplying books.0001_initial... OK")
		assertIn(t, output, "Unapplying people.0001_initial... OK")
		djangotest.AssertTableNotExists(t, db, "books_novel")
		djangotest.AssertTableNotExists(t, db, "people_writer")
	})
}

// TestMakemigrationsSplitsMutuallyReferencingApps checks that two apps whose
// models point at each other get initial migrations without a dependency
// cycle: one of the foreign keys moves into a second migration.
func TestMakemigrationsSplitsMutuallyReferencingApps(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		c := newCommand(t, db, nil, djangotest.App("clubs", &Club{}), djangotest.App("members", &Member{}))
		c.call("makemigrations")
		files := c.files()
		slices.Sort(files)
		djangotest.AssertEqual(t, len(files), 3)
		djangotest.AssertEqual(t, files[0], "0001_initial.go")
		djangotest.AssertEqual(t, files[1], "0001_initial.go")
		if !strings.HasPrefix(files[2], "0002_") {
			t.Errorf("third migration = %q, want a second migration of one app", files[2])
		}

		c.call("migrate")
		djangotest.AssertFKExists(t, db, "clubs_club", "founder_id", "members_member", "id")
		djangotest.AssertFKExists(t, db, "members_member", "club_id", "clubs_club", "id")
		djangotest.AssertEqual(t, c.call("makemigrations"), "No changes detected\n")

		c.call("migrate", "clubs", "zero")
		c.call("migrate", "members", "zero")
		djangotest.AssertTableNotExists(t, db, "clubs_club")
		djangotest.AssertTableNotExists(t, db, "members_member")
	})
}

// TestMakemigrationsImportHint checks that makemigrations names the import
// path of a new migrations package, which a separate migrate process needs,
// and stays silent once the package is part of the binary or on a dry run.
func TestMakemigrationsImportHint(t *testing.T) {
	const hint = "Import the new migrations package in your main package and rebuild, so that migrate finds it:\n" +
		"  _ \"example.com/site/migrations/people\"\n"
	module := t.TempDir()
	if err := os.WriteFile(filepath.Join(module, "go.mod"), []byte("module example.com/site\n\ngo 1.26\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	c := newCommand(t, nil, nil, djangotest.App("people", &Writer{}))
	c.app.MigrationsDir = filepath.Join(module, "migrations")

	assertNotIn(t, c.call("makemigrations", "--dry-run"), "Import the new migrations package")
	output := c.call("makemigrations")
	if !strings.HasSuffix(output, hint) {
		t.Errorf("output does not end with the import hint:\n%s", output)
	}

	type Writer struct {
		ID   int64
		Name string `orm:"max_length=100"`
		Bio  string `orm:"type=text;default=''"`
	}
	output = c.next(djangotest.App("people", &Writer{})).call("makemigrations")
	assertIn(t, output, "Add field bio to writer")
	assertNotIn(t, output, "Import the new migrations package")
}

// TestMakemigrationsAndMigrateModelChanges walks through the life of a model:
// create it, fill it, rename a field and add a required one, and go back.
// The rows survive every step.
func TestMakemigrationsAndMigrateModelChanges(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		var c *command
		{
			type Item struct {
				ID   int64
				Name string `orm:"max_length=50"`
			}
			c = newCommand(t, db, nil, djangotest.App("shop", &Item{}))
		}
		c.call("makemigrations")
		c.call("migrate")
		c.exec(`INSERT INTO "shop_item" ("name") VALUES ('pen')`)

		{
			type Item struct {
				ID    int64
				Title string `orm:"max_length=50"`
				Stock int64  `orm:"default=7"`
			}
			c = c.next(djangotest.App("shop", &Item{}))
		}
		c.input("y\n")
		output := c.call("makemigrations")
		assertIn(t, output, "Was item.name renamed to item.title (a CharField)? [y/N]")
		assertIn(t, output, "Rename field name on item to title")
		assertIn(t, output, "Add field stock to item")

		// The existing row keeps its value and gets the default of the new
		// required column.
		c.call("migrate")
		djangotest.AssertColumnNotExists(t, db, "shop_item", "name")
		djangotest.AssertColumnNotNull(t, db, "shop_item", "stock")
		djangotest.AssertEqual(t, c.queryString(`SELECT "title" FROM "shop_item"`), "pen")
		djangotest.AssertEqual(t, c.queryInt(`SELECT "stock" FROM "shop_item"`), 7)

		// Going back one migration undoes both changes.
		output = c.call("migrate", "shop", "0001")
		assertIn(t, output, "Target specific migration: 0001_initial, from shop")
		djangotest.AssertColumnNotExists(t, db, "shop_item", "title")
		djangotest.AssertColumnNotExists(t, db, "shop_item", "stock")
		djangotest.AssertEqual(t, c.queryString(`SELECT "name" FROM "shop_item"`), "pen")
	})
}

// TestMigrateFake checks that --fake records migrations without running them.
func TestMigrateFake(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		c := newCommand(t, db, testMigrations(), djangotest.App("migrations"))
		output := c.call("migrate", "--fake")
		assertIn(t, output, "Applying migrations.0001_initial... FAKED")
		assertIn(t, output, "Applying migrations.0002_second... FAKED")
		djangotest.AssertTableNotExists(t, db, "migrations_author")
		djangotest.AssertEqual(t, c.call("showmigrations"),
			"migrations\n [X] 0001_initial\n [X] 0002_second\n")

		output = c.call("migrate", "migrations", "zero", "--fake")
		assertIn(t, output, "Unapplying migrations.0002_second... FAKED")
		djangotest.AssertEqual(t, c.call("showmigrations"),
			"migrations\n [ ] 0001_initial\n [ ] 0002_second\n")
	})
}

// TestMigrateRollsBackFailedMigration checks that a migration is applied in
// one transaction: when an operation fails, the earlier ones are undone and
// nothing is recorded.
func TestMigrateRollsBackFailedMigration(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		failing := []migrations.Migration{{
			App: "migrations", Name: "0001_initial",
			Operations: []migrations.Operation{
				migrations.CreateModel{Name: "Author", Fields: authorFields()},
				migrations.RunSQL{SQL: []string{`INSERT INTO "migrations_missing" ("id") VALUES (1)`}},
			},
		}}
		c := newCommand(t, db, failing, djangotest.App("migrations"))
		output, err := c.fail("migrate")
		assertIn(t, err.Error(), "migrations.0001_initial")
		// The line of the failed migration is closed.
		assertIn(t, output, "  Applying migrations.0001_initial...\n")
		djangotest.AssertTableNotExists(t, db, "migrations_author")
		djangotest.AssertEqual(t, c.call("showmigrations"), "migrations\n [ ] 0001_initial\n")
	})
}

// TestOnPostMigrate checks that the hooks run in the order of registration
// after every migrate that touches the database, also when nothing was left
// to apply, and not for --plan.
func TestOnPostMigrate(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		c := newCommand(t, db, testMigrations(), djangotest.App("migrations"))
		var calls []string
		c.app.OnPostMigrate(func(_ context.Context, got *orm.DB) error {
			if got != db {
				t.Error("the hook did not get the database of the application")
			}
			// The hook sees the migrated schema.
			djangotest.AssertTableExists(t, got, "migrations_book")
			calls = append(calls, "first")
			return nil
		})
		c.app.OnPostMigrate(func(context.Context, *orm.DB) error {
			calls = append(calls, "second")
			return nil
		})

		c.call("migrate", "--plan")
		djangotest.AssertEqual(t, len(calls), 0)

		c.call("migrate")
		djangotest.AssertSequenceEqual(t, calls, []string{"first", "second"})

		assertIn(t, c.call("migrate"), "No migrations to apply.")
		djangotest.AssertSequenceEqual(t, calls, []string{"first", "second", "first", "second"})
	})
}

// TestOnPostMigrateError checks that a failing hook fails the command and
// stops the hooks after it, while the migrations stay applied.
func TestOnPostMigrateError(t *testing.T) {
	db := djangotest.NewDB(t)
	c := newCommand(t, db, testMigrations(), djangotest.App("migrations"))
	errHook := errors.New("no default data")
	later := false
	c.app.OnPostMigrate(func(context.Context, *orm.DB) error { return errHook })
	c.app.OnPostMigrate(func(context.Context, *orm.DB) error {
		later = true
		return nil
	})

	_, err := c.fail("migrate")
	djangotest.AssertErrorIs(t, err, errHook)
	if later {
		t.Error("a hook ran after the failing one")
	}
	djangotest.AssertTableExists(t, db, "migrations_book")
}

// TestMigrationCommandUsageErrors checks the errors for a wrong command line,
// which carry the exit status 2 of Django's argument parser.
func TestMigrationCommandUsageErrors(t *testing.T) {
	c := newCommand(t, djangotest.NewDB(t), testMigrations(), djangotest.App("migrations"))
	tests := []struct {
		args    []string
		message string
	}{
		{[]string{"showmigrations", "--list", "--plan"}, "argument --plan: not allowed with argument --list"},
		{[]string{"sqlmigrate", "migrations"}, "sqlmigrate takes an app label and a migration name"},
		{
			[]string{"migrate", "migrations", "0001", "0002"},
			"migrate takes at most an app label and a migration name, got: migrations 0001 0002",
		},
	}
	for _, tt := range tests {
		_, err := c.fail(tt.args...)
		assertCommandError(t, err, tt.message, 2)
	}
}

// TestSqlmigrateUnknownMigration checks Django's message for a migration
// name that matches nothing.
func TestSqlmigrateUnknownMigration(t *testing.T) {
	c := newCommand(t, djangotest.NewDB(t), testMigrations(), djangotest.App("migrations"))
	_, err := c.fail("sqlmigrate", "migrations", "nonexistent")
	assertCommandError(t, err,
		"Cannot find a migration matching 'nonexistent' from app 'migrations'. Is it in INSTALLED_APPS?", 1)
}

// TestMigrationCommandsWithoutDatabase checks which commands need a database:
// migrate and sqlmigrate do, makemigrations and showmigrations work without.
func TestMigrationCommandsWithoutDatabase(t *testing.T) {
	c := newCommand(t, nil, testMigrations(), djangotest.App("migrations"))
	for _, args := range [][]string{{"migrate"}, {"sqlmigrate", "migrations", "0001"}} {
		_, err := c.fail(args...)
		assertIn(t, err.Error(), "no database configured")
	}
	djangotest.AssertEqual(t, c.call("showmigrations"),
		"migrations\n [ ] 0001_initial\n [ ] 0002_second\n")
}
