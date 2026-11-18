package djangogo

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/oliverhaas/djangogo/migrations"
)

// makemigrationsCommand compares the registered models with the state the
// migrations produce and writes the missing migrations, one history per app
// (Django's makemigrations).
type makemigrationsCommand struct {
	app *Application
}

func (*makemigrationsCommand) Name() string { return "makemigrations" }
func (*makemigrationsCommand) Help() string { return "Create new migrations from model changes" }

// makemigrationsOptions are the parsed command-line options.
type makemigrationsOptions struct {
	labels  []string
	dryRun  bool
	check   bool
	empty   bool
	noInput bool
	name    string
}

// parse reads the options. ok is false when only the usage was asked for.
func (c *makemigrationsCommand) parse(args []string) (opts makemigrationsOptions, ok bool, err error) {
	fs := flag.NewFlagSet("makemigrations", flag.ContinueOnError)
	fs.SetOutput(c.app.Out)
	fs.Usage = func() {
		_, _ = fmt.Fprintln(c.app.Out,
			"usage: makemigrations [app_label ...] [--dry-run] [--check] [--name NAME] [--empty] [--noinput]")
		fs.PrintDefaults()
	}
	fs.BoolVar(&opts.dryRun, "dry-run", false, "show what migrations would be made, without writing them")
	fs.BoolVar(&opts.check, "check", false, "exit with a non-zero status when model changes are missing migrations; implies --dry-run")
	fs.BoolVar(&opts.empty, "empty", false, "create an empty migration")
	fs.BoolVar(&opts.noInput, "noinput", false, "never prompt for input")
	fs.BoolVar(&opts.noInput, "no-input", false, "never prompt for input")
	fs.StringVar(&opts.name, "name", "", "use this name for the migration file(s)")
	fs.StringVar(&opts.name, "n", "", "short for --name")
	opts.labels, err = parseInterleaved(fs, args)
	if errors.Is(err, flag.ErrHelp) {
		return opts, false, nil
	}
	if err != nil {
		return opts, false, err
	}
	if opts.check {
		opts.dryRun = true
	}
	return opts, true, nil
}

func (c *makemigrationsCommand) Run(args []string) error {
	app := c.app
	opts, ok, err := c.parse(args)
	if err != nil || !ok {
		return err
	}
	if app.Models == nil {
		return errors.New("djangogo: no model registry configured")
	}
	// Django asks for a Python identifier. The name becomes part of a Go
	// file name here; the rule is the same.
	if opts.name != "" && !isIdentifier(opts.name) {
		return commandError(1, "The migration name must be a valid Go identifier.")
	}

	ctx := context.Background()
	// Like Django, build the graph without the database, so migrations can
	// be made before the database exists.
	loader, err := migrations.NewLoader(ctx, app.Migrations, nil, app.Apps.Names())
	if err != nil {
		return err
	}
	if err := app.checkAppLabels(loader, opts.labels, 2); err != nil {
		return err
	}
	if err := c.checkHistory(ctx, loader); err != nil {
		return err
	}
	conflicts := loader.DetectConflicts()
	if len(opts.labels) > 0 {
		for label := range conflicts {
			if !slices.Contains(opts.labels, label) {
				delete(conflicts, label)
			}
		}
	}
	if err := conflictError(conflicts); err != nil {
		return err
	}

	if opts.empty {
		if len(opts.labels) == 0 {
			return commandError(1, "You must supply at least one app label when using --empty.")
		}
		changes := map[string][]*migrations.Migration{}
		for _, label := range opts.labels {
			changes[label] = []*migrations.Migration{{App: label, Name: "custom"}}
		}
		return c.write(migrations.ArrangeForGraph(changes, loader.Graph, opts.name), opts.dryRun)
	}

	var questioner migrations.Questioner
	nonInteractive := &migrations.NonInteractiveQuestioner{Log: func(msg string) { _, _ = fmt.Fprintln(app.Out, msg) }}
	if opts.noInput {
		questioner = nonInteractive
	} else {
		questioner = migrations.NewInteractiveQuestioner(app.in(), app.Out, opts.dryRun)
	}
	detector, err := app.newAutodetector(loader, questioner)
	if err != nil {
		return err
	}
	changes, err := detector.Changes(loader.Graph, opts.labels, opts.name)
	if err != nil {
		return err
	}
	if len(changes) == 0 {
		switch len(opts.labels) {
		case 0:
			_, _ = fmt.Fprintln(app.Out, "No changes detected")
		case 1:
			_, _ = fmt.Fprintf(app.Out, "No changes detected in app '%s'\n", opts.labels[0])
		default:
			_, _ = fmt.Fprintf(app.Out, "No changes detected in apps '%s'\n", strings.Join(opts.labels, "', '"))
		}
		return nil
	}
	if err := c.write(changes, opts.dryRun); err != nil {
		return err
	}
	c.warnDeclinedRenames(nonInteractive.Declined, changes)
	if opts.check {
		return commandError(1, "Model changes are missing from the migrations.")
	}
	return nil
}

// checkHistory fails when the database has a migration applied before one of
// its dependencies. A database that cannot be read only gets a warning, as in
// Django: making migrations does not need it.
func (c *makemigrationsCommand) checkHistory(ctx context.Context, loader *migrations.Loader) error {
	if c.app.DB == nil {
		return nil
	}
	applied, err := migrations.NewRecorder(c.app.DB).AppliedMigrations(ctx)
	if err != nil {
		_, _ = fmt.Fprintf(c.app.Out, "Got an error checking a consistent migration history performed for "+
			"database connection 'default': %v\n", err)
		return nil
	}
	loader.Applied = applied
	return loader.CheckConsistentHistory()
}

// write prints the migrations in Django's format and, unless dryRun is set,
// writes their files and adds them to the registry of the application. An
// app without migrations in the registry gets a new Go package, which the
// user must import; the hint follows Django's output.
func (c *makemigrationsCommand) write(changes map[string][]*migrations.Migration, dryRun bool) error {
	app := c.app
	labels := make([]string, 0, len(changes))
	for label := range changes {
		labels = append(labels, label)
	}
	sort.Strings(labels)
	var newPackages []string
	for _, label := range labels {
		_, _ = fmt.Fprintf(app.Out, "Migrations for '%s':\n", label)
		dir := filepath.Join(app.MigrationsDir, label)
		if !dryRun && len(app.Migrations.ForApp(label)) == 0 {
			newPackages = append(newPackages, dir)
		}
		for _, m := range changes[label] {
			path := filepath.Join(dir, migrations.FileName(m.Name))
			_, _ = fmt.Fprintf(app.Out, "  %s\n", displayPath(path))
			for _, op := range m.Operations {
				_, _ = fmt.Fprintf(app.Out, "    %s\n", migrations.FormattedDescription(op))
			}
			if dryRun {
				continue
			}
			if _, err := migrations.WriteMigration(dir, migrations.PackageName(label), *m); err != nil {
				return err
			}
			app.Migrations.Add(*m)
		}
	}
	c.importHint(newPackages)
	return nil
}

// importHint tells the user to import the migrations packages in dirs, which
// the binary does not contain yet. Generated files register through init(),
// so a separate migrate process sees them only after the packages are
// imported and the binary rebuilt; Django needs no such step because Python
// finds the files at run time. A directory outside a Go module is skipped.
func (c *makemigrationsCommand) importHint(dirs []string) {
	var paths []string
	for _, dir := range dirs {
		if path, ok := goImportPath(dir); ok {
			paths = append(paths, path)
		}
	}
	switch len(paths) {
	case 0:
		return
	case 1:
		_, _ = fmt.Fprintln(c.app.Out, "Import the new migrations package in your main package and rebuild, "+
			"so that migrate finds it:")
	default:
		_, _ = fmt.Fprintln(c.app.Out, "Import the new migrations packages in your main package and rebuild, "+
			"so that migrate finds them:")
	}
	for _, path := range paths {
		_, _ = fmt.Fprintf(c.app.Out, "  _ %q\n", path)
	}
}

// goImportPath returns the Go import path of dir, built from the module path
// in the nearest go.mod above it.
func goImportPath(dir string) (string, bool) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", false
	}
	for root := abs; ; root = filepath.Dir(root) {
		data, err := os.ReadFile(filepath.Join(root, "go.mod"))
		if err == nil {
			module := modulePath(string(data))
			rel, err := filepath.Rel(root, abs)
			if module == "" || err != nil {
				return "", false
			}
			if rel == "." {
				return module, true
			}
			return module + "/" + filepath.ToSlash(rel), true
		}
		if filepath.Dir(root) == root {
			return "", false
		}
	}
}

// modulePath returns the module path a go.mod file declares, or "".
func modulePath(gomod string) string {
	for line := range strings.Lines(gomod) {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == "module" {
			return strings.Trim(fields[1], "\"`")
		}
	}
	return ""
}

// warnDeclinedRenames tells the user which removed and added pairs looked
// like a rename under --noinput, where nobody can confirm it. Django says
// nothing there; the warning stays because the data loss is easy to miss.
func (c *makemigrationsCommand) warnDeclinedRenames(declined []migrations.PotentialRename, changes map[string][]*migrations.Migration) {
	for _, r := range declined {
		label := r.Model
		if r.IsModel() {
			label = r.To
		}
		appLabel, _, _ := strings.Cut(label, ".")
		if _, ok := changes[appLabel]; !ok {
			continue
		}
		what, lost := "field", "column"
		if r.IsModel() {
			what, lost = "model", "table"
		}
		_, _ = fmt.Fprintf(c.app.Out,
			"Warning: %s looks like a renamed %s; the migration drops the old %s and adds a new empty one, "+
				"discarding its data. Edit the migration to rename the %s instead if you meant to keep it.\n",
			r, what, lost, lost)
	}
}

// displayPath returns path relative to the working directory when it lies
// below it, and the absolute path otherwise, as Django prints file names.
func displayPath(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	wd, err := os.Getwd()
	if err != nil {
		return abs
	}
	rel, err := filepath.Rel(wd, abs)
	if err != nil || strings.HasPrefix(rel, "..") {
		return abs
	}
	return rel
}

// isIdentifier reports whether s consists of ASCII letters, digits and
// underscores and does not start with a digit.
func isIdentifier(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		switch {
		case r == '_', r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9' && i > 0:
		default:
			return false
		}
	}
	return true
}
