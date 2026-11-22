package djangogo

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/oliverhaas/djangogo/checks"
	"github.com/oliverhaas/djangogo/manage"
	"github.com/oliverhaas/djangogo/staticfiles"
)

// collectstaticCommand is Django's collectstatic: it copies the static files
// of Settings.StaticFilesDirs and of the apps into Settings.StaticRoot, or
// with --link links the files of Settings.StaticFilesDirs and copies the
// embedded files of the apps. There is no post-processing: --no-post-process
// is accepted and does nothing.
type collectstaticCommand struct {
	app *Application

	noInput, noPostProcess, dryRun, clear, link, noDefaultIgnore bool
	ignore                                                       stringList
	verbosity                                                    verbosityValue
}

// Name returns the command name.
func (*collectstaticCommand) Name() string { return "collectstatic" }

// Help returns the help text.
func (*collectstaticCommand) Help() string { return "Collect static files in a single location." }

// CheckTags limits the system checks before the command to the staticfiles
// ones (requires_system_checks = [Tags.staticfiles]).
func (*collectstaticCommand) CheckTags() []string { return []string{checks.TagStaticfiles} }

// Flags declares Django's options of collectstatic. It resets the values of
// an earlier run first.
func (c *collectstaticCommand) Flags(fs *flag.FlagSet) {
	c.ignore, c.verbosity = nil, 1
	const noInputHelp = "Do NOT prompt the user for input of any kind."
	fs.BoolVar(&c.noInput, "noinput", false, noInputHelp)
	fs.BoolVar(&c.noInput, "no-input", false, noInputHelp)
	fs.BoolVar(&c.noPostProcess, "no-post-process", false, "Do NOT post process collected files.")
	const ignoreHelp = "Ignore files or directories matching this glob-style pattern. Use multiple times to ignore more."
	fs.Var(&c.ignore, "ignore", ignoreHelp)
	fs.Var(&c.ignore, "i", ignoreHelp)
	const dryRunHelp = "Do everything except modify the filesystem."
	fs.BoolVar(&c.dryRun, "dry-run", false, dryRunHelp)
	fs.BoolVar(&c.dryRun, "n", false, dryRunHelp)
	const clearHelp = "Clear the existing files using the storage before trying to copy or link the original file."
	fs.BoolVar(&c.clear, "clear", false, clearHelp)
	fs.BoolVar(&c.clear, "c", false, clearHelp)
	const linkHelp = "Create a symbolic link to each file instead of copying."
	fs.BoolVar(&c.link, "link", false, linkHelp)
	fs.BoolVar(&c.link, "l", false, linkHelp)
	fs.BoolVar(&c.noDefaultIgnore, "no-default-ignore", false,
		"Don't ignore the common private glob-style patterns (defaults to 'CVS', '.*' and '*~').")
	c.verbosity.declare(fs)
}

// Run asks for confirmation when files would be overwritten, collects the
// files and prints Django's summary.
func (c *collectstaticCommand) Run(_ []string) error {
	a := c.app
	root := a.Settings.StaticRoot
	if root == "" {
		return manage.Errorf("%w", staticfiles.ErrStaticRootNotSet)
	}
	if abs, err := filepath.Abs(root); err == nil {
		root = abs
	}
	if !c.noInput && dirHasEntries(root) {
		if err := c.confirm(root); err != nil {
			return err
		}
	}

	result, err := staticfiles.Collect(a.Static, staticfiles.CollectOptions{
		Root:            root,
		Clear:           c.clear,
		DryRun:          c.dryRun,
		Link:            c.link,
		IgnorePatterns:  c.ignore,
		NoDefaultIgnore: c.noDefaultIgnore,
		Verbosity:       int(c.verbosity),
		Out:             a.out(),
	})
	if err != nil {
		return manage.Errorf("%w", err)
	}
	if c.verbosity >= 1 {
		_, _ = fmt.Fprintf(a.out(), "\n%s\n", result.Summary(root))
	}
	return nil
}

// confirm asks Django's question before files in root are overwritten or
// deleted and returns Django's error unless the answer is "yes".
func (c *collectstaticCommand) confirm(root string) error {
	var b strings.Builder
	b.WriteString("\n")
	if c.dryRun {
		b.WriteString("You have activated the --dry-run option so no files will be modified.\n\n")
	}
	b.WriteString("You have requested to collect static files at the destination\n" +
		"location as specified in your settings:\n\n    " + root + "\n\n")
	if c.clear {
		b.WriteString("This will DELETE ALL FILES in this location!\n")
	} else {
		b.WriteString("This will overwrite existing files!\n")
	}
	b.WriteString("Are you sure you want to do this?\n\nType 'yes' to continue, or 'no' to cancel: ")
	_, _ = io.WriteString(c.app.out(), b.String())

	answer, err := bufio.NewReader(c.app.in()).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("djangogo: read answer: %w", err)
	}
	if strings.TrimRight(answer, "\r\n") != "yes" {
		return manage.Errorf("Collecting static files cancelled.")
	}
	return nil
}

// dirHasEntries reports whether dir exists and is not empty, the condition
// on which Django asks before collecting.
func dirHasEntries(dir string) bool {
	f, err := os.Open(dir)
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()
	names, _ := f.Readdirnames(1)
	return len(names) > 0
}

// findstaticCommand is Django's findstatic: it prints where the static
// files with the given paths are found.
type findstaticCommand struct {
	app *Application

	first     bool
	verbosity verbosityValue
}

// Name returns the command name.
func (*findstaticCommand) Name() string { return "findstatic" }

// Help returns the help text.
func (*findstaticCommand) Help() string {
	return "Finds the absolute paths for the given static file(s)."
}

// Usage names the positional arguments.
func (*findstaticCommand) Usage() string { return "staticfile [staticfile ...]" }

// Flags declares Django's options of findstatic.
func (c *findstaticCommand) Flags(fs *flag.FlagSet) {
	c.verbosity = 1
	fs.BoolVar(&c.first, "first", false, "Only return the first match for each static file.")
	c.verbosity.declare(fs)
}

// Run looks every path up. The locations of the paths found go to Out,
// together at the end; a path not found is reported on Err at once, as
// Django's LabelCommand does.
func (c *findstaticCommand) Run(paths []string) error {
	if len(paths) == 0 {
		return &manage.UsageError{Msg: "Enter at least one staticfile."}
	}
	var found []string
	for _, path := range paths {
		text, ok := staticfiles.FindStaticOutput(c.app.Static, path, !c.first, int(c.verbosity))
		switch {
		case ok:
			found = append(found, strings.TrimSuffix(text, "\n"))
		case text != "":
			_, _ = io.WriteString(c.app.errOut(), text)
		}
	}
	if len(found) > 0 {
		_, _ = fmt.Fprintln(c.app.out(), strings.Join(found, "\n"))
	}
	return nil
}

// verbosityValue is Django's common --verbosity option: 0 minimal, 1
// normal, 2 verbose, 3 very verbose output.
type verbosityValue int

// declare defines -v and --verbosity on fs.
func (v *verbosityValue) declare(fs *flag.FlagSet) {
	const help = "Verbosity level; 0=minimal output, 1=normal output, 2=verbose output, 3=very verbose output"
	fs.Var(v, "verbosity", help)
	fs.Var(v, "v", help)
}

// String returns the level.
func (v *verbosityValue) String() string {
	if v == nil {
		return "0"
	}
	return strconv.Itoa(int(*v))
}

// Set parses a level with argparse's messages.
func (v *verbosityValue) Set(s string) error {
	n, err := strconv.Atoi(s)
	if err != nil {
		return fmt.Errorf("invalid int value: '%s'", s)
	}
	if n < 0 || n > 3 {
		return fmt.Errorf("invalid choice: %d (choose from 0, 1, 2, 3)", n)
	}
	*v = verbosityValue(n)
	return nil
}
