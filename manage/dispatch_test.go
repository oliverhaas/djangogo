package manage_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/manage"
)

// Base provides the streams of a command to the registry.
var _ manage.StreamSetter = (*manage.Base)(nil)

// versionCommand prints a version text of its own, as an application does
// that registers a command under the name the registry handles itself.
type versionCommand struct{ manage.Base }

func (*versionCommand) Name() string           { return "version" }
func (*versionCommand) Help() string           { return "Print the version" }
func (*versionCommand) SkipSystemChecks() bool { return true }

func (c *versionCommand) Run(_ []string) error {
	_, err := fmt.Fprintln(c.Stdout(), "djangogo 0.9")
	return err
}

// TestRegisterDuplicate: Register refuses a name that is taken, by a
// built-in command or by the command of an app, and keeps the first command.
func TestRegisterDuplicate(t *testing.T) {
	r := newRegistry()
	mustRegister(t, r, stubCommand{name: "migrate", help: "first"})
	r.RegisterApp("polls", stubCommand{name: "closepoll"})

	err := r.Register(stubCommand{name: "migrate", help: "second"})

	if err == nil {
		t.Fatal("Register of a duplicate name returned no error")
	}
	assertEqual(t, err.Error(), `manage: duplicate command "migrate"`)
	cmd, ok := r.Get("migrate")
	if !ok || cmd.Help() != "first" {
		t.Errorf("Get = %v, %v; want the first command", cmd, ok)
	}
	if err := r.Register(stubCommand{name: "closepoll"}); err == nil {
		t.Error("Register over the command of an app returned no error")
	}
	assertEqual(t, r.App("closepoll"), "polls")
}

// TestRegisterAppOverridesBuiltin: the command of an app replaces a built-in
// command of the same name and moves it to the help group of the app, as
// staticfiles does with runserver in Django.
func TestRegisterAppOverridesBuiltin(t *testing.T) {
	r := newRegistry()
	mustRegister(t, r, stubCommand{name: "runserver", help: "built-in"}, stubCommand{name: "migrate"})
	assertEqual(t, r.App("runserver"), "djangogo")

	r.RegisterApp("staticfiles", stubCommand{name: "runserver", help: "staticfiles"})

	assertEqual(t, r.App("runserver"), "staticfiles")
	cmd, ok := r.Get("runserver")
	if !ok || cmd.Help() != "staticfiles" {
		t.Errorf("Get = %v, %v; want the command of the app", cmd, ok)
	}
	assertStrings(t, r.Names(), []string{"migrate", "runserver"})
	assertEqual(t, r.MainHelp(false), `
Type 'manage.py help <subcommand>' for help on a specific subcommand.

Available subcommands:

[djangogo]
    migrate

[staticfiles]
    runserver`)
}

// TestUnknownName: Get and App report a name that is not registered.
func TestUnknownName(t *testing.T) {
	r := djangoRegistry(t)

	if cmd, ok := r.Get("nosuch"); ok || cmd != nil {
		t.Errorf("Get = %v, %v; want nil, false", cmd, ok)
	}
	assertEqual(t, r.App("nosuch"), "")
	assertEqual(t, r.App("migrate"), "djangogo")
	assertEqual(t, r.App("createsuperuser"), "auth")
}

// TestRegisteredVersionCommand: a command registered as "version" runs in
// place of the built-in output; --version keeps printing Version.
func TestRegisteredVersionCommand(t *testing.T) {
	r := newRegistry()
	assertEqual(t, run(r, "version"), result{out: testVersion + "\n"})

	mustRegister(t, r, &versionCommand{})

	assertEqual(t, run(r, "version"), result{out: "djangogo 0.9\n"})
	assertEqual(t, run(r, "--version"), result{out: testVersion + "\n"})
}

// TestHelpIsNotACommand: help is handled before the commands, so it has no
// help of its own, and a command of that name is never run. The error is the
// one Django prints for "manage.py help help".
func TestHelpIsNotACommand(t *testing.T) {
	r := djangoRegistry(t)

	got := run(r, "help", "help")

	assertEqual(t, got, result{
		err:  "Unknown command: 'help'. Did you mean shell?\nType 'manage.py help' for usage.\n",
		code: 1,
	})

	cmd := &rawCommand{name: "help"}
	mustRegister(t, r, cmd)
	assertOutput(t, run(r, "help").out, "Available subcommands:")
	assertEqual(t, cmd.runs, 0)
}

// TestZeroRegistry: the zero value of Registry is ready to use.
func TestZeroRegistry(t *testing.T) {
	var r manage.Registry
	var out, errOut strings.Builder
	r.Out, r.Err, r.Prog = &out, &errOut, "app"

	assertEqual(t, len(r.Names()), 0)
	if _, ok := r.Get("legacy"); ok {
		t.Error("Get found a command in an empty registry")
	}
	assertEqual(t, r.MainHelp(true), "")
	assertEqual(t, r.Run([]string{"legacy"}), 1)
	assertEqual(t, errOut.String(), "Unknown command: 'legacy'\nType 'app help' for usage.\n")

	r.ExemptFromChecks("legacy")
	cmd := &rawCommand{name: "legacy"}
	if err := r.Register(cmd); err != nil {
		t.Fatalf("Register: %v", err)
	}
	r.RegisterApp("polls", stubCommand{name: "closepoll"})

	assertEqual(t, r.Run([]string{"legacy", "a"}), 0)
	assertStrings(t, cmd.args, []string{"a"})
	assertStrings(t, r.Names(), []string{"closepoll", "legacy"})
	assertEqual(t, r.Run(nil), 0)
	assertEqual(t, out.String(), `
Type 'app help <subcommand>' for help on a specific subcommand.

Available subcommands:

[djangogo]
    legacy

[polls]
    closepoll
`)
}

// TestNewRegistryDefaults: a new registry writes to the streams of the
// process and names the running program in its texts.
func TestNewRegistryDefaults(t *testing.T) {
	r := manage.NewRegistry()
	if r.Out != os.Stdout || r.Err != os.Stderr {
		t.Errorf("Out, Err = %v, %v; want os.Stdout, os.Stderr", r.Out, r.Err)
	}
	mustRegister(t, r, stubCommand{name: "migrate"})
	prog := filepath.Base(os.Args[0])

	assertOutput(t, r.MainHelp(false), "Type '"+prog+" help <subcommand>' for help on a specific subcommand.")
	text, err := r.CommandHelp("migrate")
	if err != nil {
		t.Fatalf("CommandHelp: %v", err)
	}
	assertOutput(t, text, "usage: "+prog+" migrate\n")
	assertOutput(t, run(r, "nosuch").err, "Type '"+prog+" help' for usage.")
}

// TestBaseStreams: the zero value of Base writes to the streams of the
// process, and SetStreams replaces both.
func TestBaseStreams(t *testing.T) {
	var b manage.Base
	if b.Stdout() != os.Stdout || b.Stderr() != os.Stderr {
		t.Errorf("zero Base: Stdout, Stderr = %v, %v; want os.Stdout, os.Stderr", b.Stdout(), b.Stderr())
	}

	var out, errOut strings.Builder
	b.SetStreams(&out, &errOut)

	if b.Stdout() != &out || b.Stderr() != &errOut {
		t.Errorf("Stdout, Stderr = %v, %v; want the streams set", b.Stdout(), b.Stderr())
	}
}
