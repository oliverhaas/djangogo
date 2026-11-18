package manage_test

import (
	"bytes"
	"flag"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/manage"
)

// The helpers in this file stand in for Django's AdminScriptTestCase: where
// Django starts manage.py in a subprocess and reads its streams, a test here
// drives Registry.Run with buffers for Out and Err.

// testVersion is what the registries of the tests print for --version.
const testVersion = "6.0.3"

// result is what one run of the registry produced, the counterpart of the
// (out, err) pair of AdminScriptTestCase.run_manage plus the exit code.
type result struct {
	out, err string
	code     int
}

// newRegistry returns an empty registry named manage.py, as the generated
// script of a Django project is.
func newRegistry() *manage.Registry {
	r := manage.NewRegistry()
	r.Prog = "manage.py"
	r.Version = testVersion
	return r
}

// run executes args on r with fresh buffers, the counterpart of run_manage.
func run(r *manage.Registry, args ...string) result {
	var out, errOut bytes.Buffer
	r.Out, r.Err = &out, &errOut
	code := r.Run(args)
	return result{out: out.String(), err: errOut.String(), code: code}
}

// mustRegister adds built-in commands and fails the test on a duplicate.
func mustRegister(t *testing.T, r *manage.Registry, cmds ...manage.Command) {
	t.Helper()
	for _, c := range cmds {
		if err := r.Register(c); err != nil {
			t.Fatalf("Register(%s): %v", c.Name(), err)
		}
	}
}

// assertOutput is AdminScriptTestCase.assertOutput: stream contains msg.
func assertOutput(t *testing.T, stream, msg string) {
	t.Helper()
	if !strings.Contains(stream, msg) {
		t.Errorf("'%s' does not match actual output text '%s'", msg, stream)
	}
}

// assertNotInOutput is AdminScriptTestCase.assertNotInOutput.
func assertNotInOutput(t *testing.T, stream, msg string) {
	t.Helper()
	if strings.Contains(stream, msg) {
		t.Errorf("'%s' matches actual output text '%s'", msg, stream)
	}
}

// assertNoOutput is AdminScriptTestCase.assertNoOutput.
func assertNoOutput(t *testing.T, stream string) {
	t.Helper()
	if stream != "" {
		t.Errorf("Stream should be empty: actually contains '%s'", stream)
	}
}

// assertEqual fails the test when got differs from want.
func assertEqual[T comparable](t *testing.T, got, want T) {
	t.Helper()
	if got != want {
		t.Errorf("got %#v, want %#v", got, want)
	}
}

// pyStr renders s the way Python's repr does for the plain strings of the
// ported tests; the empty string stands for None, the default of an option
// without a value.
func pyStr(s string) string {
	if s == "" {
		return "None"
	}
	return "'" + s + "'"
}

// pyTuple renders labels as Python prints a tuple of strings.
func pyTuple(labels []string) string {
	switch len(labels) {
	case 0:
		return "()"
	case 1:
		return "(" + pyStr(labels[0]) + ",)"
	}
	quoted := make([]string, len(labels))
	for i, l := range labels {
		quoted[i] = pyStr(l)
	}
	return "(" + strings.Join(quoted, ", ") + ")"
}

// stubCommand is a command that only has a name; the dispatch tests fill a
// registry with Django's command names through it.
type stubCommand struct {
	name string
	help string
}

func (c stubCommand) Name() string         { return c.name }
func (c stubCommand) Help() string         { return c.help }
func (stubCommand) Run(_ []string) error   { return nil }
func (stubCommand) SkipSystemChecks() bool { return true }

// djangoCommands maps the help groups of a new Django 6.0 project to their
// commands; "djangogo" holds what Django lists under "django".
var djangoCommands = map[string][]string{
	"auth":         {"changepassword", "createsuperuser"},
	"contenttypes": {"remove_stale_contenttypes"},
	"djangogo": {
		"check", "compilemessages", "createcachetable", "dbshell", "diffsettings", "dumpdata",
		"flush", "inspectdb", "loaddata", "makemessages", "makemigrations", "migrate",
		"optimizemigration", "sendtestemail", "shell", "showmigrations", "sqlflush", "sqlmigrate",
		"sqlsequencereset", "squashmigrations", "startapp", "startproject", "test", "testserver",
	},
	"sessions":    {"clearsessions"},
	"staticfiles": {"collectstatic", "findstatic", "runserver"},
}

// djangoRegistry returns a registry that holds a stub for every command of a
// new Django project, in Django's help groups.
func djangoRegistry(t *testing.T) *manage.Registry {
	t.Helper()
	r := newRegistry()
	for app, names := range djangoCommands {
		for _, name := range names {
			if app == "djangogo" {
				mustRegister(t, r, stubCommand{name: name})
				continue
			}
			r.RegisterApp(app, stubCommand{name: name})
		}
	}
	return r
}

// baseCommand ports admin_scripts/management/commands/base_command.py. Django
// prints the options common to all commands as well (force_color, no_color,
// pythonpath, settings, traceback, verbosity); none of them exists here, so
// the output lists the options of the command itself. The settings option is
// declared by the command to keep the argument lists of the ArgumentOrder
// tests.
type baseCommand struct {
	manage.Base
	optionA, optionB, optionC, settings string
}

func (*baseCommand) Name() string           { return "base_command" }
func (*baseCommand) Help() string           { return "Test basic commands" }
func (*baseCommand) Usage() string          { return "[args ...]" }
func (*baseCommand) SkipSystemChecks() bool { return true }

func (c *baseCommand) Flags(fs *flag.FlagSet) {
	fs.StringVar(&c.optionA, "option_a", "1", "")
	fs.StringVar(&c.optionA, "a", "1", "")
	fs.StringVar(&c.optionB, "option_b", "2", "")
	fs.StringVar(&c.optionB, "b", "2", "")
	fs.StringVar(&c.optionC, "option_c", "3", "")
	fs.StringVar(&c.optionC, "c", "3", "")
	fs.StringVar(&c.settings, "settings", "", "")
}

func (c *baseCommand) Run(labels []string) error {
	_, err := fmt.Fprintf(c.Stdout(),
		"EXECUTE:BaseCommand labels=%s, options=[('option_a', %s), ('option_b', %s), ('option_c', %s), ('settings', %s)]\n",
		pyTuple(labels), pyStr(c.optionA), pyStr(c.optionB), pyStr(c.optionC), pyStr(c.settings))
	return err
}

// noargsCommand ports admin_scripts/management/commands/noargs_command.py: a
// command without options and without positional arguments.
type noargsCommand struct{ manage.Base }

func (*noargsCommand) Name() string           { return "noargs_command" }
func (*noargsCommand) Help() string           { return "Test No-args commands" }
func (*noargsCommand) SkipSystemChecks() bool { return true }
func (*noargsCommand) Flags(_ *flag.FlagSet)  {}

func (c *noargsCommand) Run(_ []string) error {
	_, err := fmt.Fprintln(c.Stdout(), "EXECUTE: noargs_command options=[]")
	return err
}

// labelCommand ports admin_scripts/management/commands/label_command.py.
// LabelCommand has no counterpart type: the command walks its labels itself
// and reports a missing label as a usage error, as LabelCommand does through
// parser.error.
type labelCommand struct{ manage.Base }

func (*labelCommand) Name() string           { return "label_command" }
func (*labelCommand) Help() string           { return "Test Label-based commands" }
func (*labelCommand) Usage() string          { return "label [label ...]" }
func (*labelCommand) SkipSystemChecks() bool { return true }
func (*labelCommand) Flags(_ *flag.FlagSet)  {}

func (c *labelCommand) Run(labels []string) error {
	if len(labels) == 0 {
		return &manage.UsageError{Msg: "Enter at least one label."}
	}
	for _, label := range labels {
		if _, err := fmt.Fprintf(c.Stdout(), "EXECUTE:LabelCommand label=%s, options=[]\n", label); err != nil {
			return err
		}
	}
	return nil
}

// danceCommand ports user_commands/management/commands/dance.py. It requires
// the system checks, as the Django command does.
type danceCommand struct {
	manage.Base
	style, example string
	option3        bool
}

func (*danceCommand) Name() string  { return "dance" }
func (*danceCommand) Help() string  { return "Dance around like a madman." }
func (*danceCommand) Usage() string { return "[integer]" }

func (c *danceCommand) Flags(fs *flag.FlagSet) {
	fs.StringVar(&c.style, "style", "Rock'n'Roll", "")
	fs.StringVar(&c.style, "s", "Rock'n'Roll", "")
	fs.StringVar(&c.example, "example", "", "")
	fs.StringVar(&c.example, "x", "", "")
	fs.BoolVar(&c.option3, "opt-3", false, "")
}

func (c *danceCommand) Run(args []string) error {
	if c.example == "raise" {
		return &manage.CommandError{Code: 3}
	}
	out := c.Stdout()
	if _, err := fmt.Fprintf(out, "I don't feel like dancing %s.\n", c.style); err != nil {
		return err
	}
	// Django prints the keys of the options dictionary; the option given as
	// --opt-3 is known to the command under its destination, option3.
	options := []string{"example", "integer", "style"}
	if c.option3 {
		options = append(options, "option3")
	}
	if _, err := fmt.Fprintln(out, strings.Join(options, ",")); err != nil {
		return err
	}
	if len(args) == 0 {
		return nil
	}
	// argparse converts the argument (type=int); the command does it here.
	integer, err := strconv.Atoi(args[0])
	if err != nil {
		return &manage.UsageError{Msg: fmt.Sprintf("argument integer: invalid int value: '%s'", args[0])}
	}
	if integer > 0 {
		_, err = fmt.Fprintf(out, "You passed %d as a positional argument.\n", integer)
	}
	return err
}

// halCommand ports user_commands/management/commands/hal.py. The verbosity
// option is common to all commands in Django and declared by the command
// here.
type halCommand struct {
	manage.Base
	empty     bool
	verbosity int
}

func (*halCommand) Name() string  { return "hal" }
func (*halCommand) Help() string  { return "Useless command." }
func (*halCommand) Usage() string { return "[app_label ...]" }

func (c *halCommand) Flags(fs *flag.FlagSet) {
	fs.BoolVar(&c.empty, "empty", false, "Do nothing.")
	fs.IntVar(&c.verbosity, "verbosity", 1, "Verbosity level")
}

func (c *halCommand) Run(appLabels []string) error {
	out := c.Stdout()
	if c.empty {
		_, err := fmt.Fprint(out, "\nDave, I can't do that.\n")
		return err
	}
	if len(appLabels) == 0 {
		return manage.Errorf("I'm sorry Dave, I'm afraid I can't do that.")
	}
	// Fail when an option flows from the options into the arguments.
	for _, label := range appLabels {
		if strings.HasPrefix(label, "--") {
			return manage.Errorf("Sorry, Dave, I can't let you do that.")
		}
	}
	_, err := fmt.Fprintln(out, "Dave, my mind is going. I can feel it. I can feel it.")
	return err
}

// setOptionCommand ports user_commands/management/commands/set_option.py.
type setOptionCommand struct {
	manage.Base
	set string
}

func (*setOptionCommand) Name() string { return "set_option" }
func (*setOptionCommand) Help() string { return "" }

func (c *setOptionCommand) Flags(fs *flag.FlagSet) {
	fs.StringVar(&c.set, "set", "", "")
}

func (c *setOptionCommand) Run(_ []string) error {
	_, err := fmt.Fprintf(c.Stdout(), "Set %s\n", c.set)
	return err
}

// checkCommand stands in for Django's check command in the help tests: it
// has an option of its own and requires the system checks, so the common
// --skip-checks option is listed as well.
type checkCommand struct {
	manage.Base
	tag    string
	deploy bool
}

func (*checkCommand) Name() string  { return "check" }
func (*checkCommand) Usage() string { return "[app_label ...]" }
func (*checkCommand) Help() string {
	return "Checks the entire Django project for potential problems."
}

func (c *checkCommand) Flags(fs *flag.FlagSet) {
	fs.StringVar(&c.tag, "tag", "", "Run only checks labeled with given tag.")
	fs.BoolVar(&c.deploy, "deploy", false, "Check deployment settings.")
}

func (*checkCommand) Run(_ []string) error { return nil }

// helloCommand writes one line to the stream the test selects, the
// counterpart of the inline commands of test_custom_stdout and
// test_custom_stderr.
type helloCommand struct {
	manage.Base
	toStderr bool
}

func (*helloCommand) Name() string           { return "hello" }
func (*helloCommand) Help() string           { return "" }
func (*helloCommand) SkipSystemChecks() bool { return true }

func (c *helloCommand) Run(_ []string) error {
	w := c.Stdout()
	if c.toStderr {
		w = c.Stderr()
	}
	_, err := fmt.Fprintln(w, "Hello, World!")
	return err
}

// failingCommand returns the error the test gives it, the counterpart of a
// command whose execute method is replaced by one that raises.
type failingCommand struct{ err error }

func (failingCommand) Name() string           { return "failing" }
func (failingCommand) Help() string           { return "" }
func (failingCommand) SkipSystemChecks() bool { return true }
func (c failingCommand) Run(_ []string) error { return c.err }

// duplicateCommand ports the two commands named "duplicate" of
// admin_scripts.simple_app and admin_scripts.complex_app; each prints the
// name of its app.
type duplicateCommand struct {
	manage.Base
	app string
}

func (*duplicateCommand) Name() string           { return "duplicate" }
func (*duplicateCommand) Help() string           { return "" }
func (*duplicateCommand) SkipSystemChecks() bool { return true }

func (c *duplicateCommand) Run(_ []string) error {
	_, err := fmt.Fprintln(c.Stdout(), c.app)
	return err
}

// checkCounter is a SystemChecks hook that records the commands it was called
// for, the counterpart of mock.patch on BaseCommand.check.
type checkCounter struct {
	calls []manage.Command
	err   error
}

// hook is the function to assign to Registry.SystemChecks.
func (c *checkCounter) hook(cmd manage.Command) error {
	c.calls = append(c.calls, cmd)
	return c.err
}
