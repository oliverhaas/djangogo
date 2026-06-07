package djangogo

import (
	"fmt"
	"io"
)

// versionCommand prints the framework version.
type versionCommand struct {
	out io.Writer
}

func (versionCommand) Name() string { return "version" }
func (versionCommand) Help() string { return "Print the Djan-Go-Go version" }

func (c versionCommand) Run(_ []string) error {
	_, _ = fmt.Fprintf(c.out, "djangogo %s\n", Version)
	return nil
}
