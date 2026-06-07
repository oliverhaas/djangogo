package djangogo

import (
	"errors"
	"fmt"
	"io"

	"github.com/oliverhaas/djangogo/scaffold"
)

// startappCommand generates an app package skeleton in the current module.
type startappCommand struct {
	out io.Writer
}

func (*startappCommand) Name() string { return "startapp" }
func (*startappCommand) Help() string { return "Generate a new app package skeleton" }

// Run scaffolds an app package into ./<name>app (or ./<name> when name already
// ends in "app") within the current module.
func (c *startappCommand) Run(args []string) error {
	if len(args) < 1 || args[0] == "" {
		return errors.New("startapp: usage: startapp <name>")
	}
	name := args[0]
	if err := scaffold.App(".", name, ""); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(c.out, "Created app %q\n", name)
	return nil
}
