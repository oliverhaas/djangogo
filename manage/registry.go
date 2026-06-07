package manage

import (
	"fmt"
	"io"
	"os"
	"sort"
)

// Registry holds management commands and dispatches them by name.
type Registry struct {
	byName map[string]Command
	Out    io.Writer // where usage is printed; defaults to os.Stdout
}

// NewRegistry returns an empty registry writing usage to stdout.
func NewRegistry() *Registry {
	return &Registry{byName: make(map[string]Command), Out: os.Stdout}
}

// Register adds a command. It errors on a duplicate name.
func (r *Registry) Register(c Command) error {
	name := c.Name()
	if _, dup := r.byName[name]; dup {
		return fmt.Errorf("manage: duplicate command %q", name)
	}
	r.byName[name] = c
	return nil
}

// Names returns the registered command names, sorted.
func (r *Registry) Names() []string {
	out := make([]string, 0, len(r.byName))
	for n := range r.byName {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// Execute dispatches args[0] to the matching command, passing the rest as its args.
// With no args it prints usage and returns nil.
func (r *Registry) Execute(args []string) error {
	if len(args) == 0 {
		r.printUsage()
		return nil
	}
	cmd, ok := r.byName[args[0]]
	if !ok {
		return fmt.Errorf("manage: unknown command %q (run with no arguments to list commands)", args[0])
	}
	return cmd.Run(args[1:])
}

func (r *Registry) printUsage() {
	_, _ = fmt.Fprintln(r.Out, "usage: djangogo <command> [args]")
	_, _ = fmt.Fprintln(r.Out, "\nAvailable commands:")
	for _, n := range r.Names() {
		_, _ = fmt.Fprintf(r.Out, "  %-16s %s\n", n, r.byName[n].Help())
	}
}
