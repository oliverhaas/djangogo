package djangogo

import (
	"flag"

	"github.com/oliverhaas/djangogo/scaffold"
)

// startappCommand creates an app, Django's startapp.
type startappCommand struct {
	app   *Application
	flags templateFlags
}

// Name returns the command name.
func (*startappCommand) Name() string { return "startapp" }

// Help returns Django's help text.
func (*startappCommand) Help() string {
	return "Creates a Djan-Go-Go app directory structure for the given app name in the " +
		"current directory or optionally in the given directory."
}

// Usage returns the positional arguments.
func (*startappCommand) Usage() string { return "name [directory]" }

// Flags declares the options of TemplateCommand.
func (c *startappCommand) Flags(fs *flag.FlagSet) { c.flags.declare(fs) }

// Run creates the app named by the first argument in the directory of the
// second, or in a new directory of that name. The name is the package and
// the label of the app; install it by passing its App to djangogo.New.
func (c *startappCommand) Run(args []string) error {
	t := &templateCommand{app: c.app, kind: "app", templateFlags: c.flags}
	top, err := t.prepare(args, "You must provide an application name.")
	if err != nil {
		return err
	}
	return t.generate(top, args[0], func() error {
		return scaffold.App(top, args[0], scaffold.Options{Created: t.created})
	})
}
