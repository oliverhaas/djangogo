package apps_test

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/apps"
	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/manage"
)

// duplicateCommand ports management/commands/duplicate.py of the apps
// simple_app and complex_app in tests/admin_scripts: both apps have a command
// of this name, and each prints the name of its app.
type duplicateCommand struct {
	app string
	out io.Writer
}

// Name returns the command name both apps share.
func (duplicateCommand) Name() string { return "duplicate" }

// Help returns no help text, as the Django commands have none.
func (duplicateCommand) Help() string { return "" }

// Run prints the name of the app the command belongs to.
func (c duplicateCommand) Run([]string) error {
	_, err := fmt.Fprintln(c.out, c.app)
	return err
}

// commandApp is an app with management commands (a management/commands
// package).
type commandApp struct {
	name     string
	commands []manage.Command
}

// Name returns the app name.
func (a commandApp) Name() string { return a.name }

// Commands returns the management commands of the app.
func (a commandApp) Commands() []manage.Command { return a.commands }

// callCommand ports call_command(name, stdout=out) for the commands of the
// apps: it registers them in the order Registry.Commands gives, a later
// registration replacing an earlier one (the commands.update of
// django.core.management.get_commands), and runs the one registered under
// name.
func callCommand(t *testing.T, reg *apps.Registry, name string) {
	t.Helper()
	commands := map[string]manage.Command{}
	for _, c := range reg.Commands() {
		commands[c.Command.Name()] = c.Command
	}
	command, ok := commands[name]
	if !ok {
		t.Fatalf("Unknown command: %q", name)
	}
	if err := command.Run(nil); err != nil {
		t.Fatalf("command %s: %v", name, err)
	}
}

// TestPrecedence tests that apps listed first in INSTALLED_APPS have
// precedence: the command of the app listed first is registered last.
//
// Ported from Django: tests/admin_scripts/tests.py::Discovery.test_precedence
func TestPrecedence(t *testing.T) {
	out := &bytes.Buffer{}
	complexApp := commandApp{"admin_scripts.complex_app", []manage.Command{duplicateCommand{"complex_app", out}}}
	simpleApp := commandApp{"admin_scripts.simple_app", []manage.Command{duplicateCommand{"simple_app", out}}}

	reg := newRegistry(t,
		complexApp,
		simpleApp,
		plainApp{"django.contrib.auth"},
		plainApp{"django.contrib.contenttypes"},
	)
	callCommand(t, reg, "duplicate")
	djangotest.AssertEqual(t, strings.TrimSpace(out.String()), "complex_app")
	commands := reg.Commands()
	djangotest.AssertEqual(t, commands[len(commands)-1].App, "complex_app")

	out.Reset()
	reg = newRegistry(t,
		simpleApp,
		complexApp,
		plainApp{"django.contrib.auth"},
		plainApp{"django.contrib.contenttypes"},
	)
	callCommand(t, reg, "duplicate")
	djangotest.AssertEqual(t, strings.TrimSpace(out.String()), "simple_app")
	commands = reg.Commands()
	djangotest.AssertEqual(t, commands[len(commands)-1].App, "simple_app")
}
