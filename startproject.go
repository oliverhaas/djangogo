package djangogo

import (
	"errors"
	"fmt"
	"io"

	"github.com/oliverhaas/djangogo/scaffold"
)

// startprojectCommand generates a runnable project skeleton in a new directory.
type startprojectCommand struct {
	out io.Writer
}

func (*startprojectCommand) Name() string { return "startproject" }
func (*startprojectCommand) Help() string { return "Generate a new project skeleton" }

// Run creates ./<name>/ and scaffolds a runnable project into it. The generated
// go.mod assumes a published github.com/oliverhaas/djangogo module; a local
// checkout needs a replace directive added to the generated go.mod.
func (c *startprojectCommand) Run(args []string) error {
	if len(args) < 1 || args[0] == "" {
		return errors.New("startproject: usage: startproject <name>")
	}
	name := args[0]
	dir := "./" + name
	if err := scaffold.Project(dir, name, ""); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(c.out, "Created project %q in %s\n", name, dir)
	_, _ = fmt.Fprintf(c.out, "Next: cd %s && go mod tidy && go run . runserver\n", name)
	return nil
}
