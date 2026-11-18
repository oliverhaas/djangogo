package djangogo

import (
	"errors"
	"flag"
	"fmt"
	"go/build"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/oliverhaas/djangogo/manage"
	"github.com/oliverhaas/djangogo/scaffold"
)

// startprojectCommand creates a project, Django's startproject.
type startprojectCommand struct {
	app       *Application
	verbosity verbosityValue
}

// Name returns the command name.
func (*startprojectCommand) Name() string { return "startproject" }

// Help returns Django's help text.
func (*startprojectCommand) Help() string {
	return "Creates a Djan-Go-Go project directory structure for the given project name in " +
		"the current directory or optionally in the given directory."
}

// Usage returns the positional arguments.
func (*startprojectCommand) Usage() string { return "name [directory]" }

// Flags declares --verbosity.
func (c *startprojectCommand) Flags(fs *flag.FlagSet) {
	c.verbosity = 1
	c.verbosity.declare(fs)
}

// Run creates the project named by the first argument in the directory of
// the second, or in a new directory of that name. The go.mod of the project
// requires no version of Djan-Go-Go: "go mod tidy" adds the latest release.
func (c *startprojectCommand) Run(args []string) error {
	t := templateCommand{app: c.app, kind: "project", verbosity: c.verbosity}
	top, err := t.prepare(args, "You must provide a project name.")
	if err != nil {
		return err
	}
	return t.result(scaffold.Project(top, args[0], scaffold.Options{Created: t.created}))
}

// templateCommand is the part startproject and startapp share, Django's
// TemplateCommand.
type templateCommand struct {
	app       *Application
	kind      string // "project" or "app"
	verbosity verbosityValue
}

// article returns the indefinite article of the kind.
func (t templateCommand) article() string {
	if t.kind == "app" {
		return "an"
	}
	return "a"
}

// prepare checks the arguments, the name and the directory and returns the
// absolute directory to generate into, which it creates, as
// TemplateCommand.handle does: the given directory, created when missing,
// or a new directory named after the project or app in the working
// directory.
func (t templateCommand) prepare(args []string, missing string) (string, error) {
	switch {
	case len(args) == 0:
		return "", &manage.UsageError{Msg: missing}
	case len(args) > 2:
		return "", &manage.UsageError{Msg: "unrecognized arguments: " + strings.Join(args[2:], " ")}
	}
	if err := t.validateName(args[0], "name"); err != nil {
		return "", err
	}
	if len(args) == 1 {
		top, err := filepath.Abs(args[0])
		if err != nil {
			return "", manage.Errorf("%w", err)
		}
		if err := os.Mkdir(top, 0o755); err != nil {
			if errors.Is(err, fs.ErrExist) {
				return "", manage.Errorf("'%s' already exists", top)
			}
			return "", manage.Errorf("%w", err)
		}
		return top, nil
	}
	top, err := filepath.Abs(args[1])
	if err != nil {
		return "", manage.Errorf("%w", err)
	}
	if t.kind == "app" {
		if err := t.validateName(filepath.Base(top), "directory"); err != nil {
			return "", err
		}
	}
	if err := os.MkdirAll(top, 0o755); err != nil {
		return "", manage.Errorf("%w", err)
	}
	return top, nil
}

// validateName checks a name, or the directory of an app, as
// TemplateCommand.validate_name does: it must be an identifier and must not
// be the name of a standard library package, the counterpart of an existing
// Python module. Go also reserves main, init and _ for packages.
func (t templateCommand) validateName(name, typ string) error {
	if !token.IsIdentifier(name) {
		return manage.Errorf("'%s' is not a valid %s %s. Please make sure the %s is a valid identifier.",
			name, t.kind, typ, typ)
	}
	if name == "main" || name == "init" || name == "_" {
		return manage.Errorf("'%s' is reserved by Go and cannot be used as %s %s %s. Please try another %s.",
			name, t.article(), t.kind, typ, typ)
	}
	if stdPackage(name) {
		return manage.Errorf("'%s' conflicts with the name of an existing Go package and cannot be used "+
			"as %s %s %s. Please try another %s.", name, t.article(), t.kind, typ, typ)
	}
	return nil
}

// created prints a generated file at verbosity 2.
func (t templateCommand) created(path string) {
	if t.verbosity >= 2 {
		_, _ = fmt.Fprintf(t.app.out(), "Creating %s\n", path)
	}
}

// result turns the error of a generation into a CommandError, an existing
// file into Django's overlay message.
func (t templateCommand) result(err error) error {
	var exists *scaffold.ExistsError
	switch {
	case err == nil:
		return nil
	case errors.As(err, &exists):
		return manage.Errorf("%s already exists. Overlaying %s %s into an existing directory won't "+
			"replace conflicting files.", exists.Path, t.article(), t.kind)
	default:
		return manage.Errorf("%w", err)
	}
}

// stdPackage reports whether name is a directory of the standard library. It
// reports false when the Go root is unknown, as in a binary built with
// -trimpath.
func stdPackage(name string) bool {
	root := build.Default.GOROOT
	if root == "" {
		return false
	}
	info, err := os.Stat(filepath.Join(root, "src", name))
	return err == nil && info.IsDir()
}
