package djangogo

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/oliverhaas/djangogo/migrations"
)

// makemigrationsCommand compares the registered models with the state the
// migrations produce and writes the missing migrations, one history per app
// (Django's makemigrations).
type makemigrationsCommand struct {
	app *Application
	// opts are the options of the running command.
	opts makemigrationsOptions
	// log receives the messages and prompts: the standard output, or the
	// error output with --scriptable.
	log io.Writer
}

func (*makemigrationsCommand) Name() string { return "makemigrations" }
func (*makemigrationsCommand) Help() string { return "Create new migrations from model changes" }

// makemigrationsOptions are the parsed command-line options.
type makemigrationsOptions struct {
	labels     []string
	dryRun     bool
	merge      bool
	check      bool
	empty      bool
	noInput    bool
	name       string
	noHeader   bool
	scriptable bool
	update     bool
	verbosity  verbosityValue
}

// parse reads the options. ok is false when only the usage was asked for.
func (c *makemigrationsCommand) parse(args []string) (opts makemigrationsOptions, ok bool, err error) {
	fs := flag.NewFlagSet("makemigrations", flag.ContinueOnError)
	fs.SetOutput(c.app.Out)
	fs.Usage = func() {
		_, _ = fmt.Fprintln(c.app.Out, "usage: makemigrations [app_label ...] [--dry-run] [--merge] [--empty] "+
			"[--noinput] [--name NAME] [--no-header] [--check] [--scriptable] [--update] [-v {0,1,2,3}]")
		fs.PrintDefaults()
	}
	opts.verbosity = 1
	opts.verbosity.declare(fs)
	fs.BoolVar(&opts.dryRun, "dry-run", false, "show what migrations would be made, without writing them")
	fs.BoolVar(&opts.merge, "merge", false, "fix migration conflicts")
	fs.BoolVar(&opts.empty, "empty", false, "create an empty migration")
	fs.BoolVar(&opts.noInput, "noinput", false, "never prompt for input")
	fs.BoolVar(&opts.noInput, "no-input", false, "never prompt for input")
	fs.StringVar(&opts.name, "name", "", "use this name for the migration file(s)")
	fs.StringVar(&opts.name, "n", "", "short for --name")
	fs.BoolVar(&opts.noHeader, "no-header", false, "do not add a header comment to new migration files")
	fs.BoolVar(&opts.check, "check", false,
		"exit with a non-zero status when model changes are missing migrations; implies --dry-run")
	fs.BoolVar(&opts.scriptable, "scriptable", false,
		"write messages and prompts to the error output, and only the paths of new migration files to the output")
	fs.BoolVar(&opts.update, "update", false, "merge model changes into the latest migration and optimize the operations")
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
	c.opts, c.log = opts, app.Out
	if opts.scriptable {
		c.log = app.errOut()
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
	switch {
	case !opts.merge:
		if err := conflictError(conflicts, c.app.prog()); err != nil {
			return err
		}
	case len(conflicts) == 0:
		c.logf("No conflicts detected to merge.\n")
		return nil
	default:
		return c.handleMerge(loader, conflicts)
	}

	if opts.empty {
		if len(opts.labels) == 0 {
			return commandError(1, "You must supply at least one app label when using --empty.")
		}
		changes := map[string][]*migrations.Migration{}
		for _, label := range opts.labels {
			changes[label] = []*migrations.Migration{{App: label, Name: "custom"}}
		}
		return c.write(migrations.ArrangeForGraph(changes, loader.Graph, opts.name), nil)
	}

	var questioner migrations.Questioner
	nonInteractive := &migrations.NonInteractiveQuestioner{}
	if opts.verbosity >= 1 {
		nonInteractive.Log = func(msg string) { c.logf("%s\n", msg) }
	}
	if opts.noInput {
		questioner = nonInteractive
	} else {
		questioner = migrations.NewInteractiveQuestioner(app.in(), c.log, opts.dryRun)
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
		if opts.verbosity < 1 {
			return nil
		}
		switch len(opts.labels) {
		case 0:
			c.logf("No changes detected\n")
		case 1:
			c.logf("No changes detected in app '%s'\n", opts.labels[0])
		default:
			c.logf("No changes detected in apps '%s'\n", strings.Join(opts.labels, "', '"))
		}
		return nil
	}
	if opts.update {
		err = c.updateLast(ctx, changes)
	} else {
		err = c.write(changes, nil)
	}
	if err != nil {
		return err
	}
	c.warnDeclinedRenames(nonInteractive.Declined, changes)
	if opts.check {
		return commandError(1, "Model changes are missing from the migrations.")
	}
	return nil
}

// logf prints a message to the log output.
func (c *makemigrationsCommand) logf(format string, args ...any) {
	_, _ = fmt.Fprintf(c.log, format, args...)
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
		_, _ = fmt.Fprintf(c.app.errOut(), "Got an error checking a consistent migration history performed for "+
			"database connection 'default': %v\n", err)
		return nil
	}
	loader.Applied = applied
	return loader.CheckConsistentHistory()
}

// handleMerge joins the branches of each app in conflicts with a migration
// that depends on all of them, as Django's makemigrations --merge does. It
// prints the operations of each branch and asks first, unless --noinput is
// set.
func (c *makemigrationsCommand) handleMerge(loader *migrations.Loader, conflicts map[string][]string) error {
	app, opts := c.app, c.opts
	ask := func(string) (bool, error) { return true, nil }
	if !opts.noInput {
		ask = migrations.NewInteractiveQuestioner(app.in(), c.log, false).AskMerge
	}
	for _, label := range slices.Sorted(maps.Keys(conflicts)) {
		leaves := conflicts[label]
		branches, err := loader.MergeBranches(label, leaves)
		if err != nil {
			return err
		}
		if opts.verbosity > 0 {
			c.logf("Merging %s\n", label)
			for _, branch := range branches {
				c.logf("  Branch %s\n", branch.Leaf.Name)
				for _, op := range branch.Operations {
					c.logf("    %s\n", migrations.FormattedDescription(op))
				}
			}
		}
		merge, err := ask(label)
		if err != nil {
			return err
		}
		if !merge {
			continue
		}
		m := migrations.NewMergeMigration(label, leaves, opts.name)
		dir := filepath.Join(app.MigrationsDir, label)
		src, _, err := c.render(dir, m)
		if err != nil {
			return err
		}
		path := filepath.Join(dir, migrations.FileName(m.Name))
		if opts.dryRun {
			if opts.verbosity == 3 {
				c.logf("Full merge migrations file '%s':\n%s", migrations.FileName(m.Name), src)
			}
			continue
		}
		if err := writeFile(path, src); err != nil {
			return err
		}
		app.Migrations.Add(m)
		if opts.verbosity > 0 {
			c.logf("\nCreated new merge migration %s\n", displayPath(path))
			if opts.scriptable {
				_, _ = fmt.Fprintln(app.Out, displayPath(path))
			}
		}
	}
	return nil
}

// updateLast merges changes into the last migration of each app, as Django's
// makemigrations --update does, and writes the result under a new name in
// place of the last migration. The last migration must not be a squashed,
// applied or depended-on migration.
func (c *makemigrationsCommand) updateLast(ctx context.Context, changes map[string][]*migrations.Migration) error {
	app := c.app
	loader, err := migrations.NewLoader(ctx, app.Migrations, app.DB, app.Apps.Names())
	if err != nil {
		return err
	}
	updated := map[string][]*migrations.Migration{}
	replaced := map[string]migrations.Key{}
	for _, label := range slices.Sorted(maps.Keys(changes)) {
		leaves := loader.Graph.LeafNodes(label)
		if len(leaves) == 0 {
			return commandError(1, "App %s has no migration, cannot update last migration.", label)
		}
		leaf, _ := loader.Migration(leaves[0])
		if len(leaf.Replaces) > 0 {
			return commandError(1, "Cannot update squash migration '%s'.", leaf.Key())
		}
		if _, ok := loader.Applied[leaf.Key()]; ok {
			return commandError(1, "Cannot update applied migration '%s'.", leaf.Key())
		}
		var depending []string
		for _, m := range app.Migrations.All() {
			if slices.Contains(m.Dependencies, leaf.Key()) {
				depending = append(depending, fmt.Sprintf("'%s'", m.Key()))
			}
		}
		if len(depending) > 0 {
			return commandError(1, "Cannot update migration '%s' that migrations %s depend on.",
				leaf.Key(), strings.Join(depending, ", "))
		}
		m := *leaf
		m.Operations = slices.Clone(leaf.Operations)
		m.Dependencies = slices.Clone(leaf.Dependencies)
		for _, change := range changes[label] {
			m.Operations = append(m.Operations, change.Operations...)
			for _, dep := range change.Dependencies {
				if dep.App != label && !slices.Contains(m.Dependencies, dep) {
					m.Dependencies = append(m.Dependencies, dep)
				}
			}
		}
		m.Operations = migrations.Optimize(m.Operations, label)
		fragment := c.opts.name
		if fragment == "" {
			fragment = m.SuggestName()
		}
		name := leaf.Name[:min(4, len(leaf.Name))] + "_" + fragment
		if name == leaf.Name {
			name += "_updated"
		}
		m.Name = name
		updated[label] = []*migrations.Migration{&m}
		replaced[label] = leaf.Key()
	}
	return c.write(updated, replaced)
}

// write prints the migrations in Django's format and, unless --dry-run is
// set, writes their files and adds them to the registry of the application.
// replaced holds the migration that --update replaces for an app: its file is
// deleted, unless it defines a function the new migration uses. An app
// without migrations in the registry gets a new Go package, which the user
// must import; the hint follows Django's output.
func (c *makemigrationsCommand) write(changes map[string][]*migrations.Migration, replaced map[string]migrations.Key) error {
	app, opts := c.app, c.opts
	var newPackages []string
	for _, label := range slices.Sorted(maps.Keys(changes)) {
		if opts.verbosity >= 1 {
			c.logf("Migrations for '%s':\n", label)
		}
		dir := filepath.Join(app.MigrationsDir, label)
		if !opts.dryRun && len(app.Migrations.ForApp(label)) == 0 {
			newPackages = append(newPackages, dir)
		}
		for _, m := range changes[label] {
			path := filepath.Join(dir, migrations.FileName(m.Name))
			if opts.verbosity >= 1 {
				c.logf("  %s\n", displayPath(path))
				for _, op := range m.Operations {
					c.logf("    %s\n", migrations.FormattedDescription(op))
				}
				if opts.scriptable {
					_, _ = fmt.Fprintln(app.Out, displayPath(path))
				}
			}
			if opts.dryRun && opts.verbosity < 3 {
				continue
			}
			src, porting, err := c.render(dir, *m)
			if err != nil {
				return err
			}
			if opts.dryRun {
				c.logf("Full migrations file '%s':\n%s", migrations.FileName(m.Name), src)
				continue
			}
			if err := writeFile(path, src); err != nil {
				return err
			}
			app.Migrations.Add(*m)
			if old, ok := replaced[label]; ok {
				if err := c.dropReplaced(old, path, porting); err != nil {
					return err
				}
			}
		}
	}
	if opts.verbosity >= 1 {
		c.importHint(newPackages)
	}
	return nil
}

// dropReplaced deletes the file of the migration old that --update replaced
// with the migration at path. porting lists the files that define a function
// the new migration uses; the old file is kept then, as in Django, so the
// functions can be copied first.
func (c *makemigrationsCommand) dropReplaced(old migrations.Key, path string, porting []string) error {
	oldPath := filepath.Join(c.app.MigrationsDir, old.App, migrations.FileName(old.Name))
	if len(porting) > 0 {
		c.logf("Updated migration %s requires manual porting.\n"+
			"Previous migration %s was kept and must be deleted after porting functions manually.\n",
			displayPath(path), displayPath(oldPath))
		return nil
	}
	if err := os.Remove(oldPath); err != nil {
		return fmt.Errorf("djangogo: delete the replaced migration: %w", err)
	}
	c.app.Migrations.Remove(old)
	c.logf("Deleted %s\n", displayPath(oldPath))
	return nil
}

// render returns the source of the file of m in dir.
func (c *makemigrationsCommand) render(dir string, m migrations.Migration) (src string, porting []string, err error) {
	header := "Generated by djangogo makemigrations."
	if c.opts.noHeader {
		header = ""
	}
	return renderMigration(dir, m, header)
}

// renderMigration returns the source of the file of m in dir. header is the
// text of the comment at the top, empty for none. porting lists the files
// that define a function a RunGo operation of m uses.
func renderMigration(dir string, m migrations.Migration, header string) (src string, porting []string, err error) {
	opts := migrations.RenderOptions{Header: header}
	opts.ImportPath, _ = goImportPath(dir)
	return migrations.RenderMigrationWith(migrations.PackageName(m.App), m, opts)
}

// writeFile writes src to path and creates its directory.
func writeFile(path, src string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("djangogo: create %q: %w", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		return fmt.Errorf("djangogo: write %q: %w", path, err)
	}
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
		c.logf("Import the new migrations package in your main package and rebuild, " +
			"so that migrate finds it:\n")
	default:
		c.logf("Import the new migrations packages in your main package and rebuild, " +
			"so that migrate finds them:\n")
	}
	for _, path := range paths {
		c.logf("  _ %q\n", path)
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
		c.logf(
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
