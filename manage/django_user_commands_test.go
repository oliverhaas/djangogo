package manage_test

import (
	"errors"
	"flag"
	"fmt"
	"slices"
	"testing"

	"github.com/oliverhaas/djangogo/manage"
)

// The tests in this file are ports of Django's tests/user_commands/tests.py.
// Django calls most of them through call_command, which takes options as
// keyword arguments and skips the system checks by default. There is one
// entry point here, the command line: a test passes the options as arguments
// to Registry.Execute or Registry.Run, and the system checks run unless
// --skip-checks is given. Tests of translations, eggs, the PATH lookup of
// external programs, required options, mutually exclusive groups,
// subparsers, argparse's own parser options, the output wrapper, the
// migration check and the transaction wrapper are not ported: the flag
// package has no counterpart, or the feature belongs to another package.

// noSystemChecksCommand ports
// user_commands/management/commands/no_system_checks.py.
type noSystemChecksCommand struct{}

func (noSystemChecksCommand) Name() string           { return "no_system_checks" }
func (noSystemChecksCommand) Help() string           { return "" }
func (noSystemChecksCommand) SkipSystemChecks() bool { return true }
func (noSystemChecksCommand) Flags(_ *flag.FlagSet)  {}
func (noSystemChecksCommand) Run(_ []string) error   { return nil }

// specificSystemChecksCommand ports
// user_commands/management/commands/specific_system_checks.py: the command
// names the tags of the checks it requires.
type specificSystemChecksCommand struct{}

func (specificSystemChecksCommand) Name() string         { return "specific_system_checks" }
func (specificSystemChecksCommand) Help() string         { return "" }
func (specificSystemChecksCommand) CheckTags() []string  { return []string{"staticfiles", "models"} }
func (specificSystemChecksCommand) Run(_ []string) error { return nil }

// commonArgsCommand ports user_commands/management/commands/common_args.py:
// it finds the common option defined when it declares its own. The common
// option of Django is --version, the one here is --skip-checks.
type commonArgsCommand struct {
	manage.Base
	exists bool
}

func (*commonArgsCommand) Name() string { return "common_args" }
func (*commonArgsCommand) Help() string { return "" }

func (c *commonArgsCommand) Flags(fs *flag.FlagSet) {
	c.exists = fs.Lookup("skip-checks") != nil
}

func (c *commonArgsCommand) Run(_ []string) error {
	if !c.exists {
		return manage.Errorf("--skip-checks argument does no yet exist")
	}
	_, err := fmt.Fprintln(c.Stdout(), "Detected that --skip-checks already exists")
	return err
}

// danceRegistry returns a registry with the commands of the user_commands
// app and a hook that counts the system checks.
func danceRegistry(t *testing.T) (*manage.Registry, *checkCounter) {
	t.Helper()
	r := newRegistry()
	checks := &checkCounter{}
	r.SystemChecks = checks.hook
	for _, c := range []manage.Command{
		&danceCommand{},
		&halCommand{},
		&setOptionCommand{},
		&commonArgsCommand{},
		noSystemChecksCommand{},
		specificSystemChecksCommand{},
	} {
		r.RegisterApp("user_commands", c)
	}
	return r, checks
}

// Ported from Django: tests/user_commands/tests.py::CommandTests.test_command
func TestCommand(t *testing.T) {
	r, _ := danceRegistry(t)

	got := run(r, "dance")

	assertOutput(t, got.out, "I don't feel like dancing Rock'n'Roll.\n")
}

// TestCommandStyle passes the option as arguments; the keyword form of
// call_command has no counterpart.
//
// Ported from Django: tests/user_commands/tests.py::CommandTests.test_command_style
func TestCommandStyle(t *testing.T) {
	r, _ := danceRegistry(t)

	got := run(r, "dance", "--style", "Jive")

	assertOutput(t, got.out, "I don't feel like dancing Jive.\n")
}

// TestExplode: an unknown command is an error. Django raises CommandError;
// here the error matches ErrUnknownCommand and carries Django's message.
//
// Ported from Django: tests/user_commands/tests.py::CommandTests.test_explode
func TestExplode(t *testing.T) {
	r, _ := danceRegistry(t)

	err := r.Execute([]string{"explode"})

	if !errors.Is(err, manage.ErrUnknownCommand) {
		t.Fatalf("Execute error = %v, want ErrUnknownCommand", err)
	}
	assertOutput(t, err.Error(), "Unknown command: 'explode'")
}

// TestSystemExit: an error of a command is returned by Execute as a
// CommandError with its exit code, and Run turns it into that exit code and a
// message on stderr.
//
// Ported from Django: tests/user_commands/tests.py::CommandTests.test_system_exit
func TestSystemExit(t *testing.T) {
	r, _ := danceRegistry(t)

	err := r.Execute([]string{"dance", "--example=raise"})

	var cmdErr *manage.CommandError
	if !errors.As(err, &cmdErr) {
		t.Fatalf("Execute error = %v, want a *manage.CommandError", err)
	}
	assertEqual(t, cmdErr.Code, 3)

	got := run(r, "dance", "--example=raise")

	assertEqual(t, got.code, 3)
	assertOutput(t, got.err, "CommandError")
}

// TestCallCommandOptionParsing: the command reads an option under its
// destination, not under the name of the flag (#22985).
//
// Ported from Django: tests/user_commands/tests.py::CommandTests.test_call_command_option_parsing
func TestCallCommandOptionParsing(t *testing.T) {
	r, _ := danceRegistry(t)

	got := run(r, "dance", "--opt-3")

	assertOutput(t, got.out, "option3")
	assertNotInOutput(t, got.out, "opt_3")
	assertNotInOutput(t, got.out, "opt-3")
}

// TestCallCommandOptionParsingNonStringArg: a positional argument reaches the
// command. Arguments are strings here, so the command converts the integer.
//
// Ported from Django: tests/user_commands/tests.py::CommandTests.test_call_command_option_parsing_non_string_arg
func TestCallCommandOptionParsingNonStringArg(t *testing.T) {
	r, _ := danceRegistry(t)

	got := run(r, "dance", "1")

	assertOutput(t, got.out, "You passed 1 as a positional argument.")
}

// Ported from Django: tests/user_commands/tests.py::CommandTests.test_calling_a_command_with_only_empty_parameter_should_ends_gracefully
func TestCallingACommandWithOnlyEmptyParameterShouldEndsGracefully(t *testing.T) {
	r, _ := danceRegistry(t)

	got := run(r, "hal", "--empty")

	assertEqual(t, got.out, "\nDave, I can't do that.\n")
}

// Ported from Django: tests/user_commands/tests.py::CommandTests.test_calling_command_with_app_labels_and_parameters_should_be_ok
func TestCallingCommandWithAppLabelsAndParametersShouldBeOk(t *testing.T) {
	r, _ := danceRegistry(t)

	got := run(r, "hal", "myapp", "--verbosity", "3")

	assertOutput(t, got.out, "Dave, my mind is going. I can feel it. I can feel it.\n")
}

// Ported from Django: tests/user_commands/tests.py::CommandTests.test_calling_command_with_parameters_and_app_labels_at_the_end_should_be_ok
func TestCallingCommandWithParametersAndAppLabelsAtTheEndShouldBeOk(t *testing.T) {
	r, _ := danceRegistry(t)

	got := run(r, "hal", "--verbosity", "3", "myapp")

	assertOutput(t, got.out, "Dave, my mind is going. I can feel it. I can feel it.\n")
}

// Ported from Django: tests/user_commands/tests.py::CommandTests.test_calling_a_command_with_no_app_labels_and_parameters_raise_command_error
func TestCallingACommandWithNoAppLabelsAndParametersRaiseCommandError(t *testing.T) {
	r, _ := danceRegistry(t)

	err := r.Execute([]string{"hal"})

	var cmdErr *manage.CommandError
	if !errors.As(err, &cmdErr) {
		t.Fatalf("Execute error = %v, want a *manage.CommandError", err)
	}
}

// TestCallCommandNoChecks asserts the Go rule, which is the one of Django's
// command line and the opposite of call_command's default: the system checks
// run once per command unless --skip-checks is given.
//
// Ported from Django: tests/user_commands/tests.py::CommandTests.test_call_command_no_checks
func TestCallCommandNoChecks(t *testing.T) {
	r, checks := danceRegistry(t)

	got := run(r, "dance", "--skip-checks")
	assertEqual(t, got.code, 0)
	assertEqual(t, len(checks.calls), 0)

	got = run(r, "dance")
	assertEqual(t, got.code, 0)
	assertEqual(t, len(checks.calls), 1)
	assertEqual(t, checks.calls[0].Name(), "dance")
}

// Ported from Django: tests/user_commands/tests.py::CommandTests.test_requires_system_checks_empty
func TestRequiresSystemChecksEmpty(t *testing.T) {
	r, checks := danceRegistry(t)

	got := run(r, "no_system_checks")

	assertEqual(t, got.code, 0)
	assertEqual(t, len(checks.calls), 0)
}

// TestRequiresSystemChecksSpecific: Django passes the tags of the command to
// its check method. The hook gets the command and reads the tags from it.
//
// Ported from Django: tests/user_commands/tests.py::CommandTests.test_requires_system_checks_specific
func TestRequiresSystemChecksSpecific(t *testing.T) {
	r, checks := danceRegistry(t)

	got := run(r, "specific_system_checks")

	assertEqual(t, got.code, 0)
	if len(checks.calls) != 1 {
		t.Fatalf("system checks ran %d times, want once", len(checks.calls))
	}
	tagged, ok := checks.calls[0].(interface{ CheckTags() []string })
	if !ok {
		t.Fatalf("the hook got %T, want the command with its tags", checks.calls[0])
	}
	if got, want := tagged.CheckTags(), []string{"staticfiles", "models"}; !slices.Equal(got, want) {
		t.Errorf("tags = %v, want %v", got, want)
	}
}

// TestCallCommandUnrecognizedOption asserts the Go behavior: an unknown
// option is a usage error with argparse's message, where call_command lists
// the valid keyword arguments in a TypeError.
//
// Ported from Django: tests/user_commands/tests.py::CommandTests.test_call_command_unrecognized_option
func TestCallCommandUnrecognizedOption(t *testing.T) {
	r, _ := danceRegistry(t)

	err := r.Execute([]string{"dance", "--unrecognized=1"})

	var usageErr *manage.UsageError
	if !errors.As(err, &usageErr) {
		t.Fatalf("Execute error = %v, want a *manage.UsageError", err)
	}
	assertEqual(t, err.Error(), "manage.py dance: error: unrecognized arguments: --unrecognized=1")

	err = r.Execute([]string{"dance", "--unrecognized=1", "--unrecognized2=1"})

	if !errors.As(err, &usageErr) {
		t.Fatalf("Execute error = %v, want a *manage.UsageError", err)
	}
	assertEqual(t, err.Error(),
		"manage.py dance: error: unrecognized arguments: --unrecognized=1 --unrecognized2=1")
}

// TestCommandAddArgumentsAfterCommonArguments: the common options exist when
// the command declares its own.
//
// Ported from Django: tests/user_commands/tests.py::CommandTests.test_command_add_arguments_after_common_arguments
func TestCommandAddArgumentsAfterCommonArguments(t *testing.T) {
	r, _ := danceRegistry(t)

	got := run(r, "common_args")

	assertOutput(t, got.out, "Detected that --skip-checks already exists")
}

// TestDisallowedAbbreviatedOptions: an option is found by its full name only,
// so --set is the option of the command and not an abbreviation of another
// one.
//
// Ported from Django: tests/user_commands/tests.py::CommandRunTests.test_disallowed_abbreviated_options
func TestDisallowedAbbreviatedOptions(t *testing.T) {
	r, _ := danceRegistry(t)

	got := run(r, "set_option", "--set", "foo")

	assertNoOutput(t, got.err)
	assertEqual(t, got.out, "Set foo\n")
}

// TestSkipChecks: --skip-checks runs a command of a project whose system
// checks fail. The failing check of Django is staticfiles.E001; here the hook
// fails for every command.
//
// Ported from Django: tests/user_commands/tests.py::CommandRunTests.test_skip_checks
func TestSkipChecks(t *testing.T) {
	r, checks := danceRegistry(t)
	checks.err = manage.Errorf("System check identified some issues")

	got := run(r, "set_option", "--skip-checks", "--set", "foo")

	assertNoOutput(t, got.err)
	assertEqual(t, got.out, "Set foo\n")
	assertEqual(t, len(checks.calls), 0)

	got = run(r, "set_option", "--set", "foo")

	assertNoOutput(t, got.out)
	assertEqual(t, got.err, "CommandError: System check identified some issues\n")
	assertEqual(t, got.code, 1)
}
