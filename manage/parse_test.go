package manage_test

import (
	"flag"
	"slices"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/manage"
)

// rawCommand records the arguments it is run with. It has no Flags method,
// as a command that parses its own arguments.
type rawCommand struct {
	name string
	args []string
	runs int
}

func (c *rawCommand) Name() string { return c.name }
func (*rawCommand) Help() string   { return "" }

func (c *rawCommand) Run(args []string) error {
	c.args = args
	c.runs++
	return nil
}

// keepingCommand keeps the flag set it is given, as a command does that reads
// its arguments from the set.
type keepingCommand struct {
	fs     *flag.FlagSet
	parsed bool
	args   []string
	set    []string
}

func (*keepingCommand) Name() string  { return "keeping" }
func (*keepingCommand) Help() string  { return "" }
func (*keepingCommand) Usage() string { return "[arg ...]" }

func (c *keepingCommand) Flags(fs *flag.FlagSet) {
	c.fs = fs
	fs.Bool("force", false, "")
	fs.String("name", "", "")
}

func (c *keepingCommand) Run(_ []string) error {
	c.parsed = c.fs.Parsed()
	c.args = c.fs.Args()
	c.set = nil
	c.fs.Visit(func(f *flag.Flag) { c.set = append(c.set, f.Name) })
	return nil
}

// assertStrings fails the test when got and want differ.
func assertStrings(t *testing.T, got, want []string) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestArgumentErrors pins what a mistake in the arguments prints: the usage
// line, then "prog command: error: problem" worded as argparse words it, with
// exit code 2 and without running the command.
func TestArgumentErrors(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"unknown flag", []string{"--invalid"}, "unrecognized arguments: --invalid"},
		{
			"unknown flags in the order given",
			[]string{"--foo", "name", "-bar=1", "--exclude", "x", "-z"},
			"unrecognized arguments: --foo -bar=1 -z",
		},
		{"bad int", []string{"--verbosity", "x"}, "argument -v/--verbosity: invalid int value: 'x'"},
		{"bad int of an alias", []string{"-v", "it's"}, `argument -v/--verbosity: invalid int value: "it's"`},
		{"empty int", []string{"--verbosity="}, "argument -v/--verbosity: invalid int value: ''"},
		{
			"int out of range",
			[]string{"-v=99999999999999999999"},
			"argument -v/--verbosity: invalid int value: '99999999999999999999'",
		},
		{"bad duration", []string{"--timeout", "soon"}, "argument --timeout: invalid duration value: 'soon'"},
		{"bad boolean", []string{"--interactive=maybe"}, "argument --interactive: invalid boolean value: 'maybe'"},
		{"error of a custom value", []string{"--exclude="}, "argument --exclude: an app_label must not be empty"},
		{"missing value", []string{"--database"}, "argument --database: expected one argument"},
		{"missing value of an alias", []string{"name", "-v"}, "argument -v/--verbosity: expected one argument"},
		{"missing value of a flag named h", []string{"-h"}, "argument -h: expected one argument"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newRegistry()
			cmd := &optionsCommand{}
			mustRegister(t, r, cmd)

			got := run(r, append([]string{"options"}, tt.args...)...)

			assertNoOutput(t, got.out)
			assertEqual(t, got.err, "usage: manage.py options [options] name [directory]\n"+
				"manage.py options: error: "+tt.want+"\n")
			assertEqual(t, got.code, 2)
			assertEqual(t, cmd.runs, 0)
		})
	}
}

// TestPositionalArgumentsOfCommandWithoutUsage: a command with flags and
// without a synopsis takes no positional arguments, so they are reported
// together with the unknown flags, in the order given. The terminator is
// reported too, as argparse does for a command without positional arguments.
func TestPositionalArgumentsOfCommandWithoutUsage(t *testing.T) {
	tests := []struct {
		args []string
		want string
	}{
		{[]string{"argument"}, "argument"},
		{[]string{"a", "--invalid", "b"}, "a --invalid b"},
		{[]string{"a", "--invalid", "b", "--", "-c"}, "a --invalid b -- -c"},
		{[]string{"--", "-c"}, "-- -c"},
		{[]string{"--"}, "--"},
		{[]string{"-"}, "-"},
	}
	for _, tt := range tests {
		r := newRegistry()
		mustRegister(t, r, &noargsCommand{})

		got := run(r, append([]string{"noargs_command"}, tt.args...)...)

		assertNoOutput(t, got.out)
		assertEqual(t, got.err, "usage: manage.py noargs_command\n"+
			"manage.py noargs_command: error: unrecognized arguments: "+tt.want+"\n")
		assertEqual(t, got.code, 2)
	}
}

// TestFlagSyntax: the arguments follow the rules of the flag package (one or
// two dashes, "name=value" or the value in the next argument, whatever it
// looks like), except that flags are found after positional arguments too.
func TestFlagSyntax(t *testing.T) {
	tests := []struct {
		name             string
		args             []string
		labels           string
		optionA, optionB string
	}{
		{"terminator", []string{"--option_a", "x", "--", "--option_b", "y"}, "('--option_b', 'y')", "'x'", "'2'"},
		{"terminator last", []string{"label", "--"}, "('label',)", "'1'", "'2'"},
		{"terminator alone", []string{"--"}, "()", "'1'", "'2'"},
		{"second terminator is an argument", []string{"--", "a", "--", "b"}, "('a', '--', 'b')", "'1'", "'2'"},
		{"help after the terminator", []string{"--", "-h", "--help"}, "('-h', '--help')", "'1'", "'2'"},
		{"single dash is an argument", []string{"-", "-a", "x"}, "('-',)", "'x'", "'2'"},
		{"one dash, long name", []string{"-option_a=x", "-option_b", "y"}, "()", "'x'", "'y'"},
		{"two dashes, short name", []string{"--a=x", "--b", "y"}, "()", "'x'", "'y'"},
		{"value that looks like a flag", []string{"--option_a", "--option_b"}, "()", "'--option_b'", "'2'"},
		{"value with an equals sign", []string{"-a=b=c"}, "()", "'b=c'", "'2'"},
		{"last value wins", []string{"-a", "x", "l1", "--option_a", "z", "l2"}, "('l1', 'l2')", "'z'", "'2'"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testBaseCommand(t, append([]string{"base_command"}, tt.args...), tt.labels, tt.optionA, tt.optionB)
		})
	}
}

// TestBooleanFlagSyntax: a boolean flag takes a value only with "=".
func TestBooleanFlagSyntax(t *testing.T) {
	r, _ := danceRegistry(t)

	assertOutput(t, run(r, "hal", "--empty").out, "Dave, I can't do that.")
	assertOutput(t, run(r, "hal", "-empty=true").out, "Dave, I can't do that.")
	// "false" is a positional argument, not the value of the flag.
	assertOutput(t, run(r, "hal", "--empty", "false").out, "Dave, I can't do that.")
	assertOutput(t, run(r, "hal", "--empty=false", "myapp").out, "Dave, my mind is going.")
}

// TestFlagsResetBetweenRuns: every run declares the flags again, so an option
// of one run does not leak into the next.
func TestFlagsResetBetweenRuns(t *testing.T) {
	r := newRegistry()
	mustRegister(t, r, &baseCommand{})

	assertOutput(t, run(r, "base_command", "-a", "x").out, baseCommandOutput("()", "'x'", "'2'", "None"))
	assertOutput(t, run(r, "base_command").out, baseCommandOutput("()", "'1'", "'2'", "None"))
}

// TestFlagSetIsParsed: a command that keeps its flag set finds it parsed,
// with the positional arguments as its Args and the given flags marked as
// set.
func TestFlagSetIsParsed(t *testing.T) {
	r := newRegistry()
	cmd := &keepingCommand{}
	mustRegister(t, r, cmd)

	got := run(r, "keeping", "a", "--force", "b", "--name=x", "c")

	assertEqual(t, got, result{})
	assertEqual(t, cmd.parsed, true)
	assertStrings(t, cmd.args, []string{"a", "b", "c"})
	assertStrings(t, cmd.set, []string{"force", "name"})
}

// TestSkipChecksOfCommandWithoutFlags: a command that parses its own
// arguments gets them as they are, without --skip-checks, which is looked for
// up to the terminator.
func TestSkipChecksOfCommandWithoutFlags(t *testing.T) {
	r := newRegistry()
	checks := &checkCounter{}
	r.SystemChecks = checks.hook
	cmd := &rawCommand{name: "legacy"}
	mustRegister(t, r, cmd)

	assertEqual(t, run(r, "legacy", "a", "--flag", "-x=1").code, 0)
	assertStrings(t, cmd.args, []string{"a", "--flag", "-x=1"})
	assertEqual(t, len(checks.calls), 1)
	if checks.calls[0] != manage.Command(cmd) {
		t.Errorf("the checks ran for %v, want the command", checks.calls[0])
	}

	assertEqual(t, run(r, "legacy", "a", "--skip-checks", "b").code, 0)
	assertStrings(t, cmd.args, []string{"a", "b"})
	assertEqual(t, len(checks.calls), 1)

	assertEqual(t, run(r, "legacy", "-skip-checks").code, 0)
	assertStrings(t, cmd.args, []string{})
	assertEqual(t, len(checks.calls), 1)

	assertEqual(t, run(r, "legacy", "--", "--skip-checks").code, 0)
	assertStrings(t, cmd.args, []string{"--", "--skip-checks"})
	assertEqual(t, len(checks.calls), 2)
	assertEqual(t, cmd.runs, 4)
}

// TestExemptFromChecks: a command named with ExemptFromChecks never runs the
// system checks and has no --skip-checks option: a command with flags
// rejects it, another command gets it with its arguments.
func TestExemptFromChecks(t *testing.T) {
	r := newRegistry()
	checks := &checkCounter{}
	r.SystemChecks = checks.hook
	legacy := &rawCommand{name: "legacy"}
	mustRegister(t, r, legacy, &checkCommand{})
	r.ExemptFromChecks("legacy", "check")

	assertEqual(t, run(r, "legacy", "--skip-checks").code, 0)
	assertStrings(t, legacy.args, []string{"--skip-checks"})

	assertEqual(t, run(r, "check").code, 0)
	got := run(r, "check", "--skip-checks")
	assertEqual(t, got.err, "usage: manage.py check [options] [app_label ...]\n"+
		"manage.py check: error: unrecognized arguments: --skip-checks\n")
	assertEqual(t, got.code, 2)
	assertNotInOutput(t, run(r, "check", "--help").out, "--skip-checks")

	assertEqual(t, len(checks.calls), 0)
}

// TestHelpRequestWinsOverUnknownFlag: argparse prints the help as soon as it
// finds -h, before it reports the arguments it does not know.
func TestHelpRequestWinsOverUnknownFlag(t *testing.T) {
	r := newRegistry()
	mustRegister(t, r, &checkCommand{})

	got := run(r, "check", "--invalid", "-h")

	assertNoOutput(t, got.err)
	assertOutput(t, got.out, "usage: manage.py check [options] [app_label ...]\n")
	assertEqual(t, got.code, 0)
}

// TestHelpRequestOfCommandWithoutFlags: only a first argument asks for the
// help of a command that parses its own arguments; elsewhere -h is the
// command's.
func TestHelpRequestOfCommandWithoutFlags(t *testing.T) {
	r := newRegistry()
	cmd := &rawCommand{name: "legacy"}
	mustRegister(t, r, cmd)

	assertOutput(t, run(r, "legacy", "-h", "x").out, "usage: manage.py legacy\n")
	assertEqual(t, cmd.runs, 0)

	assertNoOutput(t, run(r, "legacy", "x", "-h").out)
	assertStrings(t, cmd.args, []string{"x", "-h"})
}

// optionalList collects the values of a flag that may be given without one,
// argparse's action="append" with nargs="?" and const="".
type optionalList struct{ items *[]string }

func (v optionalList) String() string {
	if v.items == nil {
		return ""
	}
	return strings.Join(*v.items, ",")
}

func (v optionalList) Set(s string) error {
	*v.items = append(*v.items, s)
	return nil
}

func (optionalList) NoValue() string { return "" }

// excludingCommand has an option with an optional value.
type excludingCommand struct{ exclude, args []string }

func (*excludingCommand) Name() string  { return "excluding" }
func (*excludingCommand) Help() string  { return "" }
func (*excludingCommand) Usage() string { return "[arg ...]" }

func (c *excludingCommand) Flags(fs *flag.FlagSet) {
	c.exclude = nil
	// One value for both names makes them aliases.
	v := &optionalList{&c.exclude}
	fs.Var(v, "exclude", "A directory to exclude.")
	fs.Var(v, "x", "")
}

func (c *excludingCommand) Run(args []string) error {
	c.args = args
	return nil
}

// TestOptionalFlagValue: an OptionalValue takes the next argument unless it
// is a flag or the terminator, and is set to its NoValue otherwise, as
// argparse treats nargs="?". The help shows the value name in brackets.
//
// No Django test exists: argparse's own tests cover nargs="?".
func TestOptionalFlagValue(t *testing.T) {
	tests := []struct {
		name          string
		args          []string
		exclude, rest []string
	}{
		{"alone", []string{"--exclude"}, []string{""}, nil},
		{"with a value", []string{"--exclude", "a"}, []string{"a"}, nil},
		{"with equals", []string{"-x=a"}, []string{"a"}, nil},
		{"before a flag", []string{"--exclude", "-x", "b"}, []string{"", "b"}, nil},
		{"before the terminator", []string{"-x", "--", "a"}, []string{""}, []string{"a"}},
		{"after an argument", []string{"a", "--exclude"}, []string{""}, []string{"a"}},
		{"single dash is a value", []string{"-x", "-"}, []string{"-"}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newRegistry()
			cmd := &excludingCommand{}
			mustRegister(t, r, cmd)
			got := run(r, append([]string{"excluding"}, tt.args...)...)
			assertEqual(t, got, result{})
			assertStrings(t, cmd.exclude, tt.exclude)
			assertStrings(t, cmd.args, tt.rest)
		})
	}

	r := newRegistry()
	mustRegister(t, r, &excludingCommand{})
	assertOutput(t, run(r, "excluding", "--help").out, "  -x, --exclude [EXCLUDE]\n")
}
