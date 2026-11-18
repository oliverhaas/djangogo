package manage_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/manage"
)

// The tests in this file are ports of Django's tests/admin_scripts/tests.py.
// Django runs manage.py in a subprocess; here the registry runs in process
// with buffers for its streams (see helpers_test.go). Tests of settings
// modules, of the color styles, of AppCommand (it resolves labels against the
// app registry), of the commands themselves (check, runserver, startproject,
// startapp, diffsettings, dumpdata, listurls) and of the options common to
// all Django commands that have no counterpart (--version per command,
// --verbosity, --settings, --pythonpath, --traceback, --no-color,
// --force-color) are not ported.

// baseCommandOutput is the line base_command prints for the given labels and
// options, the counterpart of the expected_out of _test_base_command.
func baseCommandOutput(labels, optionA, optionB, settings string) string {
	return fmt.Sprintf(
		"EXECUTE:BaseCommand labels=%s, options=[('option_a', %s), ('option_b', %s), ('option_c', '3'), ('settings', %s)]",
		labels, optionA, optionB, settings)
}

// testBaseCommand ports CommandTypes._test_base_command.
func testBaseCommand(t *testing.T, args []string, labels, optionA, optionB string) {
	t.Helper()
	r := newRegistry()
	mustRegister(t, r, &baseCommand{})

	got := run(r, args...)

	assertNoOutput(t, got.err)
	assertOutput(t, got.out, baseCommandOutput(labels, optionA, optionB, "None"))
}

// TestVersion: version is handled as a special case.
//
// Ported from Django: tests/admin_scripts/tests.py::CommandTypes.test_version
func TestVersion(t *testing.T) {
	got := run(djangoRegistry(t), "version")

	assertNoOutput(t, got.err)
	assertOutput(t, got.out, testVersion)
}

// TestVersionAlternative: --version is equivalent to version.
//
// Ported from Django: tests/admin_scripts/tests.py::CommandTypes.test_version_alternative
func TestVersionAlternative(t *testing.T) {
	r := djangoRegistry(t)

	assertEqual(t, run(r, "--version"), run(r, "version"))
}

// TestHelp: help is handled as a special case. The group of the built-in
// commands is called "djangogo" where Django says "django".
//
// Ported from Django: tests/admin_scripts/tests.py::CommandTypes.test_help
func TestHelp(t *testing.T) {
	got := run(djangoRegistry(t), "help")

	assertOutput(t, got.out, "Type 'manage.py help <subcommand>' for help on a specific subcommand.")
	assertOutput(t, got.out, "[djangogo]")
	assertOutput(t, got.out, "startapp")
	assertOutput(t, got.out, "startproject")
}

// TestHelpCommands: help --commands shows the list of all available commands.
//
// Ported from Django: tests/admin_scripts/tests.py::CommandTypes.test_help_commands
func TestHelpCommands(t *testing.T) {
	got := run(djangoRegistry(t), "help", "--commands")

	assertNotInOutput(t, got.out, "usage:")
	assertNotInOutput(t, got.out, "Options:")
	assertNotInOutput(t, got.out, "[djangogo]")
	assertOutput(t, got.out, "startapp")
	assertOutput(t, got.out, "startproject")
	assertNotInOutput(t, got.out, "\n\n")
}

// TestHelpAlternative: --help is equivalent to help.
//
// Ported from Django: tests/admin_scripts/tests.py::CommandTypes.test_help_alternative
func TestHelpAlternative(t *testing.T) {
	r := djangoRegistry(t)

	assertEqual(t, run(r, "--help"), run(r, "help"))
}

// TestHelpShortAltert: -h is handled as a short form of --help.
//
// Ported from Django: tests/admin_scripts/tests.py::CommandTypes.test_help_short_altert
func TestHelpShortAltert(t *testing.T) {
	r := djangoRegistry(t)

	assertEqual(t, run(r, "-h"), run(r, "--help"))
}

// TestSpecificHelp: --help can be used on a specific command. Django asserts
// that the options of the command come before the common ones with --version;
// the common option here is --skip-checks.
//
// Ported from Django: tests/admin_scripts/tests.py::CommandTypes.test_specific_help
func TestSpecificHelp(t *testing.T) {
	r := newRegistry()
	r.SystemChecks = func(manage.Command) error { return nil }
	mustRegister(t, r, &checkCommand{})

	got := run(r, "check", "--help")

	assertNoOutput(t, got.err)
	tagLocation := strings.Index(got.out, "--tag")
	skipChecksLocation := strings.Index(got.out, "--skip-checks")
	if tagLocation == -1 || skipChecksLocation == -1 {
		t.Fatalf("--tag or --skip-checks missing in %q", got.out)
	}
	if tagLocation >= skipChecksLocation {
		t.Errorf("--tag at %d is not before --skip-checks at %d", tagLocation, skipChecksLocation)
	}
	assertOutput(t, got.out, "Checks the entire Django project for potential problems.")
}

// TestHelpDefaultOptionsWithCustomArguments asserts the Go help format: the
// usage is one line with "[options]" in place of one bracket per option, an
// option and its one-letter alias share a line with the alias first, and of
// the default options only -h exists.
//
// Ported from Django: tests/admin_scripts/tests.py::CommandTypes.test_help_default_options_with_custom_arguments
func TestHelpDefaultOptionsWithCustomArguments(t *testing.T) {
	r := newRegistry()
	mustRegister(t, r, &baseCommand{})

	got := run(r, "base_command", "--help")

	assertNoOutput(t, got.err)
	assertOutput(t, got.out, "usage: manage.py base_command [options] [args ...]\n")
	assertOutput(t, got.out, "-h, --help")
	assertOutput(t, got.out, "-a, --option_a OPTION_A")
	assertOutput(t, got.out, "-b, --option_b OPTION_B")
	assertOutput(t, got.out, "-c, --option_c OPTION_C")
}

// TestCustomStdout: a command writes to the stdout it is given. Django passes
// the stream to the command or to call_command; here it is the Out of the
// registry, handed to the command before every run.
//
// Ported from Django: tests/admin_scripts/tests.py::CommandTypes.test_custom_stdout
func TestCustomStdout(t *testing.T) {
	r := newRegistry()
	mustRegister(t, r, &helloCommand{})
	var out, newOut, errOut strings.Builder
	r.Out, r.Err = &out, &errOut

	assertEqual(t, r.Run([]string{"hello"}), 0)
	assertEqual(t, out.String(), "Hello, World!\n")

	out.Reset()
	r.Out = &newOut
	assertEqual(t, r.Run([]string{"hello"}), 0)
	assertEqual(t, out.String(), "")
	assertEqual(t, newOut.String(), "Hello, World!\n")
}

// TestCustomStderr: a command writes to the stderr it is given; see
// TestCustomStdout for the mapping.
//
// Ported from Django: tests/admin_scripts/tests.py::CommandTypes.test_custom_stderr
func TestCustomStderr(t *testing.T) {
	r := newRegistry()
	mustRegister(t, r, &helloCommand{toStderr: true})
	var out, errOut, newErr strings.Builder
	r.Out, r.Err = &out, &errOut

	assertEqual(t, r.Run([]string{"hello"}), 0)
	assertEqual(t, errOut.String(), "Hello, World!\n")

	errOut.Reset()
	r.Err = &newErr
	assertEqual(t, r.Run([]string{"hello"}), 0)
	assertEqual(t, errOut.String(), "")
	assertEqual(t, newErr.String(), "Hello, World!\n")
}

// TestBaseCommand: user commands can execute when a label is provided.
//
// Ported from Django: tests/admin_scripts/tests.py::CommandTypes.test_base_command
func TestBaseCommand(t *testing.T) {
	testBaseCommand(t, []string{"base_command", "testlabel"}, "('testlabel',)", "'1'", "'2'")
}

// TestBaseCommandNoLabel: user commands can execute when no labels are
// provided.
//
// Ported from Django: tests/admin_scripts/tests.py::CommandTypes.test_base_command_no_label
func TestBaseCommandNoLabel(t *testing.T) {
	testBaseCommand(t, []string{"base_command"}, "()", "'1'", "'2'")
}

// TestBaseCommandMultipleLabel: user commands can execute when several labels
// are provided.
//
// Ported from Django: tests/admin_scripts/tests.py::CommandTypes.test_base_command_multiple_label
func TestBaseCommandMultipleLabel(t *testing.T) {
	testBaseCommand(t, []string{"base_command", "testlabel", "anotherlabel"},
		"('testlabel', 'anotherlabel')", "'1'", "'2'")
}

// TestBaseCommandWithOption: user commands can execute with options when a
// label is provided.
//
// Ported from Django: tests/admin_scripts/tests.py::CommandTypes.test_base_command_with_option
func TestBaseCommandWithOption(t *testing.T) {
	testBaseCommand(t, []string{"base_command", "testlabel", "--option_a=x"}, "('testlabel',)", "'x'", "'2'")
}

// TestBaseCommandWithOptions: user commands can execute with multiple options
// when a label is provided.
//
// Ported from Django: tests/admin_scripts/tests.py::CommandTypes.test_base_command_with_options
func TestBaseCommandWithOptions(t *testing.T) {
	testBaseCommand(t, []string{"base_command", "testlabel", "-a", "x", "--option_b=y"},
		"('testlabel',)", "'x'", "'y'")
}

// TestBaseCommandWithWrongOption: user commands output the command usage when
// a wrong option is specified.
//
// Ported from Django: tests/admin_scripts/tests.py::CommandTypes.test_base_command_with_wrong_option
func TestBaseCommandWithWrongOption(t *testing.T) {
	r := newRegistry()
	mustRegister(t, r, &baseCommand{})

	got := run(r, "base_command", "--invalid")

	assertNoOutput(t, got.out)
	assertOutput(t, got.err, "usage: manage.py base_command")
	assertOutput(t, got.err, "error: unrecognized arguments: --invalid")
	assertEqual(t, got.code, 2)
}

// TestBaseRunFromArgv: a CommandError ends the run with an exit code and a
// message on stderr, and without a stack trace. Django re-raises an exception
// that is not a CommandError, and a CommandError under --traceback; here
// every other error is printed as "error: ..." with exit code 1, and there is
// no --traceback.
//
// Ported from Django: tests/admin_scripts/tests.py::CommandTypes.test_base_run_from_argv
func TestBaseRunFromArgv(t *testing.T) {
	r := newRegistry()
	mustRegister(t, r, failingCommand{err: errors.New("execute() takes 1 positional argument")})

	got := run(r, "failing")

	assertEqual(t, got.code, 1)
	assertEqual(t, got.err, "error: execute() takes 1 positional argument\n")

	r = newRegistry()
	mustRegister(t, r, failingCommand{err: manage.Errorf("Custom error")})

	got = run(r, "failing")

	assertEqual(t, got.code, 1)
	assertNotInOutput(t, got.err, "Traceback")
	assertNotInOutput(t, got.err, "goroutine")
	assertOutput(t, got.err, "CommandError")
	assertEqual(t, got.err, "CommandError: Custom error\n")
}

// TestRunFromArgvNonASCIIError: a non-ASCII message of a CommandError is
// printed as it is.
//
// Ported from Django: tests/admin_scripts/tests.py::CommandTypes.test_run_from_argv_non_ascii_error
func TestRunFromArgvNonASCIIError(t *testing.T) {
	r := newRegistry()
	mustRegister(t, r, failingCommand{err: manage.Errorf("Erreur personnalisée")})

	got := run(r, "failing")

	assertEqual(t, got.code, 1)
	assertEqual(t, got.err, "CommandError: Erreur personnalisée\n")
}

// TestNoargs: commands without arguments can be executed.
//
// Ported from Django: tests/admin_scripts/tests.py::CommandTypes.test_noargs
func TestNoargs(t *testing.T) {
	r := newRegistry()
	mustRegister(t, r, &noargsCommand{})

	got := run(r, "noargs_command")

	assertNoOutput(t, got.err)
	assertOutput(t, got.out, "EXECUTE: noargs_command options=[]")
}

// TestNoargsWithArgs: commands without arguments raise an error if an
// argument is provided.
//
// Ported from Django: tests/admin_scripts/tests.py::CommandTypes.test_noargs_with_args
func TestNoargsWithArgs(t *testing.T) {
	r := newRegistry()
	mustRegister(t, r, &noargsCommand{})

	got := run(r, "noargs_command", "argument")

	assertOutput(t, got.err, "error: unrecognized arguments: argument")
	assertEqual(t, got.code, 2)
}

// TestLabelCommand: label commands can execute when a label is provided.
//
// Ported from Django: tests/admin_scripts/tests.py::CommandTypes.test_label_command
func TestLabelCommand(t *testing.T) {
	r := newRegistry()
	mustRegister(t, r, &labelCommand{})

	got := run(r, "label_command", "testlabel")

	assertNoOutput(t, got.err)
	assertOutput(t, got.out, "EXECUTE:LabelCommand label=testlabel, options=[]")
}

// TestLabelCommandNoLabel: label commands raise an error if no label is
// provided.
//
// Ported from Django: tests/admin_scripts/tests.py::CommandTypes.test_label_command_no_label
func TestLabelCommandNoLabel(t *testing.T) {
	r := newRegistry()
	mustRegister(t, r, &labelCommand{})

	got := run(r, "label_command")

	assertOutput(t, got.err, "Enter at least one label")
	assertEqual(t, got.err,
		"usage: manage.py label_command label [label ...]\n"+
			"manage.py label_command: error: Enter at least one label.\n")
	assertEqual(t, got.code, 2)
}

// TestLabelCommandMultipleLabel: label commands are executed multiple times
// if multiple labels are provided.
//
// Ported from Django: tests/admin_scripts/tests.py::CommandTypes.test_label_command_multiple_label
func TestLabelCommandMultipleLabel(t *testing.T) {
	r := newRegistry()
	mustRegister(t, r, &labelCommand{})

	got := run(r, "label_command", "testlabel", "anotherlabel")

	assertNoOutput(t, got.err)
	assertOutput(t, got.out, "EXECUTE:LabelCommand label=testlabel, options=[]")
	assertOutput(t, got.out, "EXECUTE:LabelCommand label=anotherlabel, options=[]")
}

// TestNonExistentCommandOutput: an unknown command is reported on stderr.
//
// Ported from Django: tests/admin_scripts/tests.py::ManageManuallyConfiguredSettings.test_non_existent_command_output
func TestNonExistentCommandOutput(t *testing.T) {
	got := run(djangoRegistry(t), "invalid_command")

	assertNoOutput(t, got.out)
	assertOutput(t, got.err, "Unknown command: 'invalid_command'")
	assertNotInOutput(t, got.err, "No Django settings specified")
	assertEqual(t, got.code, 1)
}

// TestPrecedence: apps listed first in INSTALLED_APPS have precedence. The
// registry holds the rule that makes this work: RegisterApp replaces a command
// of the same name. The caller registers the apps in reverse INSTALLED_APPS
// order, which registerApps does here.
//
// Ported from Django: tests/admin_scripts/tests.py::Discovery.test_precedence
func TestPrecedence(t *testing.T) {
	// registerApps registers the duplicate command of every app, last app
	// first, so the app listed first registers last and wins.
	registerApps := func(r *manage.Registry, installedApps ...string) {
		for i := len(installedApps) - 1; i >= 0; i-- {
			r.RegisterApp(installedApps[i], &duplicateCommand{app: installedApps[i]})
		}
	}

	r := newRegistry()
	registerApps(r, "complex_app", "simple_app")
	got := run(r, "duplicate")
	assertEqual(t, strings.TrimSpace(got.out), "complex_app")
	assertEqual(t, r.App("duplicate"), "complex_app")

	r = newRegistry()
	registerApps(r, "simple_app", "complex_app")
	got = run(r, "duplicate")
	assertEqual(t, strings.TrimSpace(got.out), "simple_app")
	assertEqual(t, r.App("duplicate"), "simple_app")
}

// testArgumentOrder ports ArgumentOrder._test. In Django --settings is an
// option of the first parsing stage; here it is an option of the command, and
// what the tests keep is that options are found before, between and after
// the positional argument.
func testArgumentOrder(t *testing.T, args []string, optionB string) {
	t.Helper()
	r := newRegistry()
	mustRegister(t, r, &baseCommand{})

	got := run(r, args...)

	assertNoOutput(t, got.err)
	assertOutput(t, got.out, baseCommandOutput("('testlabel',)", "'x'", optionB, "'alternate_settings'"))
}

// TestSettingThenOption: options passed after settings are correctly handled.
//
// Ported from Django: tests/admin_scripts/tests.py::ArgumentOrder.test_setting_then_option
func TestSettingThenOption(t *testing.T) {
	testArgumentOrder(t, []string{"base_command", "testlabel", "--settings=alternate_settings", "--option_a=x"}, "'2'")
}

// TestSettingThenShortOption: short options passed after settings are
// correctly handled.
//
// Ported from Django: tests/admin_scripts/tests.py::ArgumentOrder.test_setting_then_short_option
func TestSettingThenShortOption(t *testing.T) {
	testArgumentOrder(t, []string{"base_command", "testlabel", "--settings=alternate_settings", "-a", "x"}, "'2'")
}

// TestOptionThenSetting: options passed before settings are correctly
// handled.
//
// Ported from Django: tests/admin_scripts/tests.py::ArgumentOrder.test_option_then_setting
func TestOptionThenSetting(t *testing.T) {
	testArgumentOrder(t, []string{"base_command", "testlabel", "--option_a=x", "--settings=alternate_settings"}, "'2'")
}

// TestShortOptionThenSetting: short options passed before settings are
// correctly handled.
//
// Ported from Django: tests/admin_scripts/tests.py::ArgumentOrder.test_short_option_then_setting
func TestShortOptionThenSetting(t *testing.T) {
	testArgumentOrder(t, []string{"base_command", "testlabel", "-a", "x", "--settings=alternate_settings"}, "'2'")
}

// TestOptionThenSettingThenOption: options are correctly handled when they
// are passed before and after a setting.
//
// Ported from Django: tests/admin_scripts/tests.py::ArgumentOrder.test_option_then_setting_then_option
func TestOptionThenSettingThenOption(t *testing.T) {
	testArgumentOrder(t, []string{
		"base_command", "testlabel", "--option_a=x", "--settings=alternate_settings", "--option_b=y",
	}, "'y'")
}

// TestProgramNameFromArgv: the program name in the help of a command is the
// one the registry is given, not the one of the running process.
//
// Ported from Django: tests/admin_scripts/tests.py::ExecuteFromCommandLine.test_program_name_from_argv
func TestProgramNameFromArgv(t *testing.T) {
	r := newRegistry()
	r.Prog = "django-admin"
	mustRegister(t, r, stubCommand{name: "shell"})

	got := run(r, "help", "shell")

	assertOutput(t, got.out, "usage: django-admin shell")
	assertEqual(t, got.err, "")
}

// TestSuggestions: an unknown command close to a known one gets a
// suggestion.
//
// Ported from Django: tests/admin_scripts/tests.py::DjangoAdminSuggestions.test_suggestions
func TestSuggestions(t *testing.T) {
	got := run(djangoRegistry(t), "rnserver")

	assertNoOutput(t, got.out)
	assertOutput(t, got.err, "Unknown command: 'rnserver'. Did you mean runserver?")
}

// TestNoSuggestions: an unknown command far from every known one gets no
// suggestion.
//
// Ported from Django: tests/admin_scripts/tests.py::DjangoAdminSuggestions.test_no_suggestions
func TestNoSuggestions(t *testing.T) {
	got := run(djangoRegistry(t), "abcdef")

	assertNoOutput(t, got.out)
	assertNotInOutput(t, got.err, "Did you mean")
}
