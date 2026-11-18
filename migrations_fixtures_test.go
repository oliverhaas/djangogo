package djangogo_test

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo"
	"github.com/oliverhaas/djangogo/apps"
	"github.com/oliverhaas/djangogo/conf"
	"github.com/oliverhaas/djangogo/migrations"
	"github.com/oliverhaas/djangogo/orm"
)

// The fixtures below are the Go counterparts of the migration modules that
// Django's command tests load through MIGRATION_MODULES. Models and fields
// carry Go names; tables and columns are the ones Django creates.

// idField is the primary key every fixture model has.
var idField = migrations.FieldState{Name: "ID", Column: "id", Kind: orm.KindAuto, PrimaryKey: true}

// key builds a migration key.
func key(app, name string) migrations.Key { return migrations.Key{App: app, Name: name} }

// authorFields are the columns of Author in most of Django's fixtures.
func authorFields() []migrations.FieldState {
	return []migrations.FieldState{
		idField,
		{Name: "Name", Column: "name", Kind: orm.KindChar, MaxLength: 255},
		{Name: "Slug", Column: "slug", Kind: orm.KindChar, MaxLength: 50, Null: true, DBIndex: true},
		{Name: "Age", Column: "age", Kind: orm.KindInt, Default: int64(0)},
		{Name: "SillyField", Column: "silly_field", Kind: orm.KindBool, Default: false},
	}
}

// tribble is the CreateModel operation of Tribble.
func tribble() migrations.CreateModel {
	return migrations.CreateModel{Name: "Tribble", Fields: []migrations.FieldState{
		idField,
		{Name: "Fluffy", Column: "fluffy", Kind: orm.KindBool, Default: true},
	}}
}

// secondOperations are the operations of 0002_second in several fixtures:
// drop Tribble, swap a field of Author and add Book.
func secondOperations() []migrations.Operation {
	return []migrations.Operation{
		migrations.DeleteModel{Name: "Tribble"},
		migrations.RemoveField{Model: "Author", Field: "SillyField"},
		migrations.AddField{Model: "Author", Field: migrations.FieldState{
			Name: "Rating", Column: "rating", Kind: orm.KindInt, Default: int64(0),
		}},
		migrations.CreateModel{Name: "Book", Fields: []migrations.FieldState{
			idField,
			{
				Name: "Author", Column: "author_id", Kind: orm.KindInt, Null: true,
				RelKind: orm.RelFK, RelTo: "migrations.Author", RelOnDelete: orm.OnDeleteSetNull,
			},
		}},
	}
}

// testMigrations mirrors Django's migrations.test_migrations.
func testMigrations() []migrations.Migration {
	return []migrations.Migration{
		{
			App: "migrations", Name: "0001_initial", Initial: true,
			Operations: []migrations.Operation{
				migrations.CreateModel{Name: "Author", Fields: authorFields()},
				tribble(),
				migrations.AddField{Model: "Tribble", Field: migrations.FieldState{
					Name: "Bool", Column: "bool", Kind: orm.KindBool, Default: false,
				}},
				migrations.AlterUniqueTogether{Name: "Author", UniqueTogether: [][]string{{"Name", "Slug"}}},
			},
		},
		{
			App: "migrations", Name: "0002_second",
			Dependencies: []migrations.Key{key("migrations", "0001_initial")},
			Operations:   secondOperations(),
		},
	}
}

// conflictMigrations mirrors Django's migrations.test_migrations_conflict,
// which has two second migrations.
func conflictMigrations() []migrations.Migration {
	deps := []migrations.Key{key("migrations", "0001_initial")}
	return []migrations.Migration{
		{
			App: "migrations", Name: "0001_initial",
			Operations: []migrations.Operation{
				migrations.CreateModel{Name: "Author", Fields: authorFields()},
				tribble(),
			},
		},
		{
			App: "migrations", Name: "0002_conflicting_second", Dependencies: deps,
			Operations: []migrations.Operation{
				migrations.CreateModel{Name: "Something", Fields: []migrations.FieldState{idField}},
			},
		},
		{App: "migrations", Name: "0002_second", Dependencies: deps, Operations: secondOperations()},
	}
}

// noChangesMigrations mirrors Django's migrations.test_migrations_no_changes,
// which ends with the models ModelWithCustomBase and UnmigratedModel.
func noChangesMigrations() []migrations.Migration {
	return []migrations.Migration{
		{
			App: "migrations", Name: "0001_initial",
			Operations: []migrations.Operation{
				migrations.CreateModel{Name: "Author", Fields: authorFields()},
				tribble(),
			},
		},
		{
			App: "migrations", Name: "0002_second",
			Dependencies: []migrations.Key{key("migrations", "0001_initial")},
			Operations:   secondOperations(),
		},
		{
			App: "migrations", Name: "0003_third",
			Dependencies: []migrations.Key{key("migrations", "0002_second")},
			Operations: []migrations.Operation{
				migrations.CreateModel{Name: "ModelWithCustomBase", Fields: []migrations.FieldState{idField}},
				migrations.CreateModel{Name: "UnmigratedModel", Fields: []migrations.FieldState{idField}},
				migrations.DeleteModel{Name: "Author"},
				migrations.DeleteModel{Name: "Book"},
			},
		},
	}
}

// ModelWithCustomBase and UnmigratedModel are the models of Django's
// "migrations" test app that noChangesMigrations ends with.
type (
	ModelWithCustomBase struct{ ID int64 }
	UnmigratedModel     struct{ ID int64 }
)

// noDefaultMigrations mirrors Django's migrations.test_migrations_no_default.
func noDefaultMigrations() []migrations.Migration {
	return []migrations.Migration{{
		App: "migrations", Name: "0001_initial",
		Operations: []migrations.Operation{
			migrations.CreateModel{Name: "SillyModel", Fields: []migrations.FieldState{
				idField,
				{Name: "SillyField", Column: "silly_field", Kind: orm.KindBool, Default: false},
			}},
		},
	}}
}

// runBeforeMigrations mirrors Django's migrations.test_migrations_run_before:
// the third migration declares that it runs before the second.
func runBeforeMigrations() []migrations.Migration {
	deps := []migrations.Key{key("migrations", "0001_initial")}
	return []migrations.Migration{
		{
			App: "migrations", Name: "0001_initial",
			Operations: []migrations.Operation{
				migrations.CreateModel{Name: "Salamander", Fields: []migrations.FieldState{
					idField,
					{Name: "Size", Column: "size", Kind: orm.KindInt, Default: int64(0)},
					{Name: "SillyField", Column: "silly_field", Kind: orm.KindBool, Default: false},
				}},
			},
		},
		{
			App: "migrations", Name: "0002_second", Dependencies: deps,
			Operations: []migrations.Operation{
				migrations.CreateModel{Name: "Book", Fields: []migrations.FieldState{
					idField,
					{
						Name: "Author", Column: "author_id", Kind: orm.KindInt, Null: true,
						RelKind: orm.RelFK, RelTo: "migrations.Author", RelOnDelete: orm.OnDeleteSetNull,
					},
				}},
			},
		},
		{
			App: "migrations", Name: "0003_third", Dependencies: deps,
			RunBefore: []migrations.Key{key("migrations", "0002_second")},
			Operations: []migrations.Operation{
				migrations.CreateModel{Name: "Author", Fields: authorFields()[:4]},
			},
		},
	}
}

// planMigrations mirrors Django's migrations.test_migrations_plan. Its
// RunPython operations are RunGo operations here.
func planMigrations() []migrations.Migration {
	next := func(name, prev string, ops ...migrations.Operation) migrations.Migration {
		return migrations.Migration{
			App: "migrations", Name: name,
			Dependencies: []migrations.Key{key("migrations", prev)},
			Operations:   ops,
		}
	}
	return []migrations.Migration{
		{
			App: "migrations", Name: "0001_initial", Initial: true,
			Operations: []migrations.Operation{
				migrations.CreateModel{Name: "Salamander", Fields: []migrations.FieldState{
					idField,
					{Name: "Tail", Column: "tail", Kind: orm.KindInt, Default: int64(0)},
					{Name: "SillyField", Column: "silly_field", Kind: orm.KindBool, Default: false},
				}},
				migrations.RunGo{Forward: migrations.Noop, Reverse: migrations.Noop},
			},
		},
		next("0002_second", "0001_initial",
			migrations.CreateModel{Name: "Book", Fields: []migrations.FieldState{idField}},
			migrations.RunSQL{
				SQL:        []string{"SELECT * FROM migrations_book"},
				ReverseSQL: []string{"SELECT * FROM migrations_salamander"},
			},
		),
		next("0003_third", "0002_second",
			migrations.CreateModel{Name: "Author", Fields: []migrations.FieldState{idField}},
			migrations.RunSQL{
				SQL:        []string{"SELECT * FROM migrations_author"},
				ReverseSQL: []string{"SELECT * FROM migrations_book"},
			},
		),
		next("0004_fourth", "0003_third",
			migrations.RunSQL{SQL: []string{"SELECT * FROM migrations_author WHERE id = 1"}},
		),
		next("0005_fifth", "0004_fourth",
			migrations.RunGo{Forward: migrations.Noop},
			migrations.RunGo{Forward: migrations.Noop},
			migrations.RunGo{Forward: migrations.Noop, Reverse: migrations.Noop},
		),
	}
}

// stateOnly wraps operations so that they change the migration state but not
// the database, as Django's SeparateDatabaseAndState with only state
// operations does.
func stateOnly(ops ...migrations.Operation) migrations.Operation {
	return migrations.RunSQL{ReverseSQL: []string{}, StateOperations: ops}
}

// mutateStateBMigrations mirrors Django's migrations_test_apps.mutate_state_b.
func mutateStateBMigrations() []migrations.Migration {
	return []migrations.Migration{
		{
			App: "mutate_state_b", Name: "0001_initial",
			Operations: []migrations.Operation{
				stateOnly(migrations.CreateModel{Name: "B", Fields: []migrations.FieldState{idField}}),
			},
		},
		{
			App: "mutate_state_b", Name: "0002_add_field",
			Dependencies: []migrations.Key{key("mutate_state_b", "0001_initial")},
			Operations: []migrations.Operation{
				stateOnly(migrations.AddField{Model: "B", Field: migrations.FieldState{
					Name: "Added", Column: "added", Kind: orm.KindText,
				}}),
			},
		},
	}
}

// alterFKMigrations mirrors Django's migrations_test_apps.alter_fk, where the
// second migration of author_app depends on the first one of book_app.
// Django's 0002_alter_id turns the primary key into a CharField, which Go
// does not support; the fixture widens the name instead. No test applies it.
func alterFKMigrations() []migrations.Migration {
	name := migrations.FieldState{Name: "Name", Column: "name", Kind: orm.KindChar, MaxLength: 50}
	wide := name
	wide.MaxLength = 100
	return []migrations.Migration{
		{
			App: "author_app", Name: "0001_initial",
			Operations: []migrations.Operation{
				migrations.CreateModel{Name: "Author", Fields: []migrations.FieldState{idField, name}},
			},
		},
		{
			App: "author_app", Name: "0002_alter_id",
			Dependencies: []migrations.Key{key("author_app", "0001_initial"), key("book_app", "0001_initial")},
			Operations:   []migrations.Operation{migrations.AlterField{Model: "Author", Field: wide}},
		},
		{
			App: "book_app", Name: "0001_initial",
			Dependencies: []migrations.Key{key("author_app", "0001_initial")},
			Operations: []migrations.Operation{
				migrations.CreateModel{Name: "Book", Fields: []migrations.FieldState{
					idField,
					{Name: "Title", Column: "title", Kind: orm.KindChar, MaxLength: 50},
					{
						Name: "Author", Column: "author_id", Kind: orm.KindInt,
						RelKind: orm.RelFK, RelTo: "author_app.Author", RelOnDelete: orm.OnDeleteCascade,
					},
				}},
			},
		},
	}
}

// migratedAppsMigrations mirrors Django's migrations_test_apps.migrated_app
// and migrated_unapplied_app.
func migratedAppsMigrations() []migrations.Migration {
	return []migrations.Migration{
		{
			App: "migrated_app", Name: "0001_initial",
			Operations: []migrations.Operation{
				migrations.CreateModel{Name: "Author", Fields: authorFields()},
				tribble(),
			},
		},
		{
			App: "migrated_unapplied_app", Name: "0001_initial",
			Operations: []migrations.Operation{
				migrations.CreateModel{Name: "OtherAuthor", Fields: authorFields()},
			},
		},
	}
}

// join returns the migrations of several fixtures as one list.
func join(sets ...[]migrations.Migration) []migrations.Migration {
	var all []migrations.Migration
	for _, set := range sets {
		all = append(all, set...)
	}
	return all
}

// command is an application under test together with its command output.
type command struct {
	t   *testing.T
	app *djangogo.Application
	out *bytes.Buffer
}

// newCommand returns an application with the given apps whose registry holds
// migs, the counterpart of INSTALLED_APPS and MIGRATION_MODULES in Django's
// tests. The commands work on db, which may be nil, write new migrations to a
// temporary directory and find no input unless a test supplies some.
func newCommand(t *testing.T, db *orm.DB, migs []migrations.Migration, configs ...apps.Config) *command {
	t.Helper()
	app, err := djangogo.New(conf.Settings{SecretKey: "k"}, configs...)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	c := &command{t: t, app: app, out: &bytes.Buffer{}}
	app.Out = c.out
	app.In = strings.NewReader("")
	app.DB = db
	app.MigrationsDir = t.TempDir()
	app.Migrations = migrations.NewRegistry()
	for _, m := range migs {
		app.Migrations.Add(m)
	}
	return c
}

// call runs a command, as Django's call_command does, and returns what it
// printed. A failing command fails the test.
func (c *command) call(args ...string) string {
	c.t.Helper()
	c.out.Reset()
	if err := c.app.Execute(args); err != nil {
		c.t.Fatalf("%s: %v", strings.Join(args, " "), err)
	}
	return c.out.String()
}

// fail runs a command that has to fail and returns what it printed before
// and its error.
func (c *command) fail(args ...string) (string, error) {
	c.t.Helper()
	c.out.Reset()
	err := c.app.Execute(args)
	if err == nil {
		c.t.Fatalf("%s: got no error", strings.Join(args, " "))
	}
	return c.out.String(), err
}

// input sets what the next command reads from its standard input.
func (c *command) input(text string) { c.app.In = strings.NewReader(text) }

// path returns the path of a file below the migrations directory of an app.
func (c *command) path(app, file string) string {
	return filepath.Join(c.app.MigrationsDir, app, file)
}

// read returns the content of a generated migration file.
func (c *command) read(app, file string) string {
	c.t.Helper()
	content, err := os.ReadFile(c.path(app, file))
	if err != nil {
		c.t.Fatalf("read migration: %v", err)
	}
	return string(content)
}

// files returns the names of the files below the migrations directory.
func (c *command) files() []string {
	c.t.Helper()
	var names []string
	err := filepath.WalkDir(c.app.MigrationsDir, func(_ string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			names = append(names, d.Name())
		}
		return nil
	})
	if err != nil {
		c.t.Fatalf("list migrations: %v", err)
	}
	return names
}

// assertCommandError checks that err is a CommandError with the message and
// the exit status, the counterpart of assertRaisesMessage(CommandError, msg).
func assertCommandError(t *testing.T, err error, message string, code int) {
	t.Helper()
	var cerr *migrations.CommandError
	if !errors.As(err, &cerr) {
		t.Fatalf("error = %v (%T), want a CommandError", err, err)
	}
	if cerr.Message != message {
		t.Errorf("message = %q, want %q", cerr.Message, message)
	}
	if cerr.Code != code {
		t.Errorf("exit status = %d, want %d", cerr.Code, code)
	}
}

// assertIn checks that text contains the part.
func assertIn(t *testing.T, text, part string) {
	t.Helper()
	if !strings.Contains(text, part) {
		t.Errorf("output does not contain %q:\n%s", part, text)
	}
}

// assertNotIn checks that text does not contain the part.
func assertNotIn(t *testing.T, text, part string) {
	t.Helper()
	if strings.Contains(text, part) {
		t.Errorf("output contains %q:\n%s", part, text)
	}
}
