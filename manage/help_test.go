package manage_test

import (
	"errors"
	"flag"
	"strings"
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/manage"
)

// mainHelpDjango60 is what "manage.py help" prints in a new Django 6.0.3
// project, with "djangogo" for the group Django calls "django".
const mainHelpDjango60 = `
Type 'manage.py help <subcommand>' for help on a specific subcommand.

Available subcommands:

[auth]
    changepassword
    createsuperuser

[contenttypes]
    remove_stale_contenttypes

[djangogo]
    check
    compilemessages
    createcachetable
    dbshell
    diffsettings
    dumpdata
    flush
    inspectdb
    loaddata
    makemessages
    makemigrations
    migrate
    optimizemigration
    sendtestemail
    shell
    showmigrations
    sqlflush
    sqlmigrate
    sqlsequencereset
    squashmigrations
    startapp
    startproject
    test
    testserver

[sessions]
    clearsessions

[staticfiles]
    collectstatic
    findstatic
    runserver
`

// TestMainHelpIsDjangos compares the main help with the output of Django,
// byte for byte.
func TestMainHelpIsDjangos(t *testing.T) {
	r := djangoRegistry(t)

	for _, args := range [][]string{nil, {"help"}, {"--help"}, {"-h"}, {"help", "--bogus"}} {
		got := run(r, args...)
		assertEqual(t, got.out, mainHelpDjango60)
		assertNoOutput(t, got.err)
		assertEqual(t, got.code, 0)
	}
	assertEqual(t, r.MainHelp(false)+"\n", mainHelpDjango60)
}

// TestMainHelpCommandsOnly: the bare list is sorted over all groups.
func TestMainHelpCommandsOnly(t *testing.T) {
	r := newRegistry()
	mustRegister(t, r, stubCommand{name: "migrate"}, stubCommand{name: "check"})
	r.RegisterApp("auth", stubCommand{name: "createsuperuser"})

	assertEqual(t, r.MainHelp(true), "check\ncreatesuperuser\nmigrate")
	assertEqual(t, run(r, "help", "--commands").out, "check\ncreatesuperuser\nmigrate\n")
}

// TestCommandHelpFormat pins the help of a command: the one-line usage, the
// help text, and the options sorted by name with -h first and --skip-checks
// last.
func TestCommandHelpFormat(t *testing.T) {
	r := newRegistry()
	r.SystemChecks = func(manage.Command) error { return nil }
	mustRegister(t, r, &checkCommand{})
	const want = `usage: manage.py check [options] [app_label ...]

Checks the entire Django project for potential problems.

options:
  -h, --help            show this help message and exit
  --deploy              Check deployment settings.
  --tag TAG             Run only checks labeled with given tag.
  --skip-checks         Skip system checks.
`

	for _, args := range [][]string{{"check", "--help"}, {"check", "-h"}, {"help", "check"}, {"check", "label", "-h"}} {
		got := run(r, args...)
		assertEqual(t, got.out, want)
		assertNoOutput(t, got.err)
		assertEqual(t, got.code, 0)
	}
	text, err := r.CommandHelp("check")
	if err != nil {
		t.Fatalf("CommandHelp: %v", err)
	}
	assertEqual(t, text, want)
}

// TestCommandHelpHidesSkipChecksWithoutSystemChecks: --skip-checks is
// accepted without configured system checks, but the help does not list an
// option that does nothing.
func TestCommandHelpHidesSkipChecksWithoutSystemChecks(t *testing.T) {
	r := newRegistry()
	mustRegister(t, r, &setOptionCommand{})

	got := run(r, "set_option", "--help")

	assertEqual(t, got.out, `usage: manage.py set_option [options]

options:
  -h, --help            show this help message and exit
  --set SET
`)
	assertEqual(t, run(r, "set_option", "--skip-checks", "--set", "foo").out, "Set foo\n")
}

// wrappingCommand declares options with the help texts of Django's common
// options, to compare the layout with argparse's.
type wrappingCommand struct{}

func (wrappingCommand) Name() string         { return "wrapping" }
func (wrappingCommand) Run(_ []string) error { return nil }
func (wrappingCommand) Help() string {
	return "Updates database schema. Manages both apps with migrations and those without. " +
		"The    text of the help\nis filled to the width of the terminal."
}

func (wrappingCommand) Flags(fs *flag.FlagSet) {
	fs.String("settings", "", `The Python path to a settings module, e.g. "myproject.settings.main". `+
		`If this isn't provided, the DJANGO_SETTINGS_MODULE environment variable will be used.`)
	fs.String("pythonpath", "", `A directory to add to the Python path, e.g. "/home/djangoprojects/myproject".`)
	fs.Bool("traceback", false, "Display a full stack trace on CommandError exceptions.")
	fs.Bool("no-color", false, "Don't colorize the command output.")
	fs.Bool("fake-initial", false, "Detect if tables already exist and fake-apply initial migrations if so. "+
		"Make sure that the current database schema matches your initial migration before using this flag. "+
		"Django will only check for an existing table name.")
}

// TestCommandHelpWrapsLikeArgparse: the option lines are the ones Django
// prints for the same options, and the help text is filled to 78 columns.
func TestCommandHelpWrapsLikeArgparse(t *testing.T) {
	r := newRegistry()
	r.ExemptFromChecks("wrapping")
	mustRegister(t, r, wrappingCommand{})

	got := run(r, "wrapping", "--help")

	assertEqual(t, got.out, `usage: manage.py wrapping [options]

Updates database schema. Manages both apps with migrations and those without.
The text of the help is filled to the width of the terminal.

options:
  -h, --help            show this help message and exit
  --fake-initial        Detect if tables already exist and fake-apply initial
                        migrations if so. Make sure that the current database
                        schema matches your initial migration before using
                        this flag. Django will only check for an existing
                        table name.
  --no-color            Don't colorize the command output.
  --pythonpath PYTHONPATH
                        A directory to add to the Python path, e.g.
                        "/home/djangoprojects/myproject".
  --settings SETTINGS   The Python path to a settings module, e.g.
                        "myproject.settings.main". If this isn't provided, the
                        DJANGO_SETTINGS_MODULE environment variable will be
                        used.
  --traceback           Display a full stack trace on CommandError exceptions.
`)
}

// optionsCommand declares one option of every kind the help distinguishes.
type optionsCommand struct {
	verbosity int
	database  string
	timeout   time.Duration
	exclude   []string
	host      string
	// runs counts the calls of Run, for the tests of the argument errors.
	runs int
}

func (*optionsCommand) Name() string  { return "options" }
func (*optionsCommand) Help() string  { return "Shows every kind of option." }
func (*optionsCommand) Usage() string { return "name [directory]" }

func (c *optionsCommand) Run(_ []string) error {
	c.runs++
	return nil
}

func (c *optionsCommand) Flags(fs *flag.FlagSet) {
	fs.IntVar(&c.verbosity, "verbosity", 1, "Verbosity level")
	fs.IntVar(&c.verbosity, "v", 1, "")
	fs.StringVar(&c.database, "database", "default", "Nominates the `alias` of a database.")
	fs.DurationVar(&c.timeout, "timeout", 0, "")
	fs.Bool("interactive", true, "Prompt the user for input.")
	fs.Func("exclude", "An app_label to exclude.", func(s string) error {
		if s == "" {
			return errors.New("an app_label must not be empty")
		}
		c.exclude = append(c.exclude, s)
		return nil
	})
	// A flag of the command takes the short name away from the help.
	fs.StringVar(&c.host, "h", "", "The `host` to bind.")
}

// TestCommandHelpOptionKinds: an alias shares the line of its option with
// the one-letter name first, a back-quoted word of the usage text names the
// value, another value is named after the flag, a default that is not the
// zero value is shown, and -h is not the help when the command uses it.
func TestCommandHelpOptionKinds(t *testing.T) {
	r := newRegistry()
	r.ExemptFromChecks("options")
	mustRegister(t, r, &optionsCommand{})

	got := run(r, "options", "--help")

	assertEqual(t, got.out, `usage: manage.py options [options] name [directory]

Shows every kind of option.

options:
  --help                show this help message and exit
  --database alias      Nominates the alias of a database. (default: default)
  --exclude EXCLUDE     An app_label to exclude.
  -h host               The host to bind.
  --interactive         Prompt the user for input. (default: true)
  --timeout TIMEOUT
  -v, --verbosity VERBOSITY
                        Verbosity level (default: 1)
`)
}

// listValue is a flag value that cannot print its zero value: String reads
// through a nil pointer there.
type listValue struct{ items *[]string }

func (v listValue) String() string { return strings.Join(*v.items, ",") }

func (v listValue) Set(s string) error {
	*v.items = append(*v.items, s)
	return nil
}

// listCommand declares options of a value type of its own.
type listCommand struct{ tags, apps []string }

func (*listCommand) Name() string         { return "list" }
func (*listCommand) Help() string         { return "" }
func (*listCommand) Run(_ []string) error { return nil }

func (c *listCommand) Flags(fs *flag.FlagSet) {
	c.tags, c.apps = []string{"models", "urls"}, nil
	fs.Var(listValue{&c.tags}, "tag", "Run the checks of a `label`.")
	fs.Var(listValue{&c.apps}, "app", "An app to check.")
}

// TestCommandHelpOfCustomValue: the default of a value type of the command is
// shown as its String method prints it, also when the method fails on the
// zero value the default is compared with; an empty default is not shown.
func TestCommandHelpOfCustomValue(t *testing.T) {
	r := newRegistry()
	r.ExemptFromChecks("list")
	mustRegister(t, r, &listCommand{})

	assertEqual(t, run(r, "list", "--help").out, `usage: manage.py list [options]

options:
  -h, --help            show this help message and exit
  --app APP             An app to check.
  --tag label           Run the checks of a label. (default: models,urls)
`)
}

// TestCommandHelpWithoutFlags: a command that parses its own arguments has a
// help with its text and the help option only.
func TestCommandHelpWithoutFlags(t *testing.T) {
	r := newRegistry()
	mustRegister(t, r, stubCommand{name: "runserver", help: "Start the development HTTP server"})
	const want = `usage: manage.py runserver

Start the development HTTP server

options:
  -h, --help            show this help message and exit
`

	assertEqual(t, run(r, "runserver", "--help").out, want)
	assertEqual(t, run(r, "runserver", "-h").out, want)
	assertEqual(t, run(r, "help", "runserver").out, want)

	// With system checks, a command that requires them has the common option.
	r.SystemChecks = func(manage.Command) error { return nil }
	mustRegister(t, r, &rawCommand{name: "legacy"})
	assertEqual(t, run(r, "runserver", "--help").out, want)
	assertEqual(t, run(r, "legacy", "--help").out, `usage: manage.py legacy [options]

options:
  -h, --help            show this help message and exit
  --skip-checks         Skip system checks.
`)
}

// TestCommandHelpUnknownCommand: the help of an unknown command is the
// unknown-command error.
func TestCommandHelpUnknownCommand(t *testing.T) {
	r := djangoRegistry(t)

	_, err := r.CommandHelp("migrat")
	if !errors.Is(err, manage.ErrUnknownCommand) {
		t.Fatalf("CommandHelp error = %v, want ErrUnknownCommand", err)
	}
	assertEqual(t, err.Error(), "Unknown command: 'migrat'. Did you mean migrate?")

	got := run(r, "help", "nosuch")
	assertNoOutput(t, got.out)
	assertEqual(t, got.err, "Unknown command: 'nosuch'\nType 'manage.py help' for usage.\n")
	assertEqual(t, got.code, 1)
}

// TestHelpOfCommandDoesNotRunIt: a help request stops before the system
// checks and before the command.
func TestHelpOfCommandDoesNotRunIt(t *testing.T) {
	r, checks := danceRegistry(t)

	got := run(r, "dance", "--style", "Jive", "--help")

	assertEqual(t, got.code, 0)
	assertEqual(t, len(checks.calls), 0)
	assertNotInOutput(t, got.out, "dancing")
	if !strings.HasPrefix(got.out, "usage: manage.py dance [options] [integer]\n") {
		t.Errorf("help does not start with the usage: %q", got.out)
	}
}
