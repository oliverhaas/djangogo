package djangogo

import (
	"fmt"
)

// versionCommand prints the framework version, as "manage.py version" and
// "--version" print Django's.
type versionCommand struct {
	app *Application
}

// Name returns the command name.
func (*versionCommand) Name() string { return "version" }

// Help returns the help text.
func (*versionCommand) Help() string { return "Prints the Djan-Go-Go version." }

// SkipSystemChecks reports that version runs no checks first.
func (*versionCommand) SkipSystemChecks() bool { return true }

// Run prints the version.
func (c *versionCommand) Run(_ []string) error {
	_, _ = fmt.Fprintln(c.app.out(), Version)
	return nil
}
