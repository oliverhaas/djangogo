package djangogo

import (
	"bufio"
	"cmp"
	"context"
	"errors"
	"flag"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/oliverhaas/djangogo/migrations"
)

// squashmigrationsCommand replaces the migrations of an app up to a given one
// by a single migration with their optimized operations (Django's
// squashmigrations).
type squashmigrationsCommand struct {
	app *Application
}

func (*squashmigrationsCommand) Name() string { return "squashmigrations" }
func (*squashmigrationsCommand) Help() string {
	return "Squash the migrations of an app up to the given one into a single new migration"
}

// squashOptions are the parsed command-line options.
type squashOptions struct {
	label, start, end string
	noOptimize        bool
	noInput           bool
	squashedName      string
	noHeader          bool
	verbosity         verbosityValue
}

// parse reads the options. ok is false when only the usage was asked for.
func (c *squashmigrationsCommand) parse(args []string) (opts squashOptions, ok bool, err error) {
	fs := flag.NewFlagSet("squashmigrations", flag.ContinueOnError)
	fs.SetOutput(c.app.Out)
	fs.Usage = func() {
		_, _ = fmt.Fprintln(c.app.Out, "usage: squashmigrations app_label [start_migration_name] migration_name "+
			"[--no-optimize] [--noinput] [--squashed-name NAME] [--no-header]")
		fs.PrintDefaults()
	}
	opts.verbosity = 1
	opts.verbosity.declare(fs)
	fs.BoolVar(&opts.noOptimize, "no-optimize", false, "do not try to optimize the squashed operations")
	fs.BoolVar(&opts.noInput, "noinput", false, "never prompt for input")
	fs.BoolVar(&opts.noInput, "no-input", false, "never prompt for input")
	fs.StringVar(&opts.squashedName, "squashed-name", "", "the name of the new squashed migration")
	fs.BoolVar(&opts.noHeader, "no-header", false, "do not add a header comment to the new squashed migration")
	pos, err := parseInterleaved(fs, args)
	if errors.Is(err, flag.ErrHelp) {
		return opts, false, nil
	}
	if err != nil {
		return opts, false, err
	}
	switch len(pos) {
	case 2:
		opts.label, opts.end = pos[0], pos[1]
	case 3:
		opts.label, opts.start, opts.end = pos[0], pos[1], pos[2]
	default:
		return opts, false, commandError(2, "squashmigrations takes an app label, an optional start migration and a migration name")
	}
	return opts, true, nil
}

func (c *squashmigrationsCommand) Run(args []string) error {
	app := c.app
	opts, ok, err := c.parse(args)
	if err != nil || !ok {
		return err
	}
	ctx := context.Background()
	// Like Django, load the migrations without the database: a squashed
	// migration counts as applied by nobody and stands for the migrations
	// it replaces.
	loader, err := migrations.NewLoader(ctx, app.Migrations, nil, app.Apps.Names())
	if err != nil {
		return err
	}
	label := opts.label
	if err := app.checkAppLabels(loader, []string{label}, 1); err != nil {
		return err
	}
	if !loader.HasMigrations(label) {
		return commandError(1, "App '%s' does not have migrations (so squashmigrations on it makes no sense)", label)
	}
	end, err := findMigration(loader, label, opts.end, "")
	if err != nil {
		return err
	}
	plan, err := loader.Graph.ForwardsPlan(end.Key())
	if err != nil {
		return err
	}
	var squash []*migrations.Migration
	for _, key := range plan {
		if key.App == label {
			m, _ := loader.Migration(key)
			squash = append(squash, m)
		}
	}
	var start *migrations.Migration
	if opts.start != "" {
		if start, err = findMigration(loader, label, opts.start, ""); err != nil {
			return err
		}
		i := slices.Index(squash, start)
		if i < 0 {
			return commandError(1, "The migration '%s' cannot be found. Maybe it comes after the migration '%s'?\n"+
				"Have a look at:\n  %s showmigrations %s\nto debug this issue.", start, end, app.prog(), label)
		}
		squash = squash[i:]
	}

	if opts.verbosity > 0 || !opts.noInput {
		_, _ = fmt.Fprintln(app.Out, "Will squash the following migrations:")
		for _, m := range squash {
			_, _ = fmt.Fprintf(app.Out, " - %s\n", m.Name)
		}
		if !opts.noInput && !c.confirm() {
			return nil
		}
	}

	squashed := migrations.Migration{App: label}
	deps := map[migrations.Key]bool{}
	// The first migration keeps its dependencies on the own app, since
	// squashing can start after the first migration.
	for i, m := range squash {
		squashed.Operations = append(squashed.Operations, m.Operations...)
		for _, dep := range m.Dependencies {
			if dep.App != label || i == 0 {
				deps[dep] = true
			}
		}
		squashed.Replaces = append(squashed.Replaces, m.Key())
	}
	squashed.Dependencies = sortedKeys(deps)
	if opts.noOptimize {
		if opts.verbosity > 0 {
			_, _ = fmt.Fprintln(app.Out, "(Skipping optimization.)")
		}
	} else {
		if opts.verbosity > 0 {
			_, _ = fmt.Fprintln(app.Out, "Optimizing...")
		}
		before := len(squashed.Operations)
		squashed.Operations = migrations.Optimize(squashed.Operations, label)
		if opts.verbosity > 0 {
			if len(squashed.Operations) == before {
				_, _ = fmt.Fprintln(app.Out, "  No optimizations possible.")
			} else {
				_, _ = fmt.Fprintf(app.Out, "  Optimized from %d operations to %d operations.\n", before, len(squashed.Operations))
			}
		}
	}

	switch {
	case start != nil && opts.squashedName != "":
		prefix, _, _ := strings.Cut(start.Name, "_")
		squashed.Name = prefix + "_" + opts.squashedName
	case start != nil:
		squashed.Name = start.Name + "_squashed_" + end.Name
	case opts.squashedName != "":
		squashed.Name = "0001_" + opts.squashedName
		squashed.Initial = true
	default:
		squashed.Name = "0001_squashed_" + end.Name
		squashed.Initial = true
	}

	path, porting, err := c.write(loader, squashed, !opts.noHeader)
	if err != nil {
		return err
	}
	app.Migrations.Add(squashed)
	if opts.verbosity > 0 {
		_, _ = fmt.Fprintf(app.Out, "Created new squashed migration %s\n"+
			"  You should commit this migration but leave the old ones in place;\n"+
			"  the new migration will be used for new installs. Once you are sure\n"+
			"  all instances of the codebase have applied the migrations you squashed,\n"+
			"  you can delete them.\n", displayPath(path))
		if len(porting) > 0 {
			_, _ = fmt.Fprint(app.Out, "Manual porting required\n"+
				"  Your migrations contained functions that must be manually copied over,\n"+
				"  as we could not safely copy their implementation.\n"+
				"  See the comment at the top of the squashed migration for details.\n")
		}
	}
	return nil
}

// confirm asks whether to go on, as Django does: an empty answer is a no,
// and other answers are asked again until one starts with y or n.
func (c *squashmigrationsCommand) confirm() bool {
	in := bufio.NewReader(c.app.in())
	for {
		_, _ = fmt.Fprint(c.app.Out, "Do you wish to proceed? [y/N] ")
		line, err := in.ReadString('\n')
		answer := strings.ToLower(strings.TrimSpace(line))
		if answer == "" {
			return false
		}
		switch answer[0] {
		case 'y':
			return true
		case 'n':
			return false
		}
		if err != nil {
			return false
		}
	}
}

// write renders the squashed migration into the migrations directory of its
// app. It refuses to replace a migration of the same name. porting lists the
// files whose functions must be copied into the new one.
func (c *squashmigrationsCommand) write(loader *migrations.Loader, m migrations.Migration, header bool) (path string, porting []string, err error) {
	dir := filepath.Join(c.app.MigrationsDir, m.App)
	path = filepath.Join(dir, migrations.FileName(m.Name))
	_, exists := loader.Migration(m.Key())
	if _, err := os.Stat(path); exists || err == nil {
		return "", nil, commandError(1, "Migration %s already exists. Use a different name.", m.Name)
	}
	opts := migrations.RenderOptions{}
	if header {
		opts.Header = "Generated by djangogo squashmigrations."
	}
	opts.ImportPath, _ = goImportPath(dir)
	src, porting, err := migrations.RenderMigrationWith(migrations.PackageName(m.App), m, opts)
	if err != nil {
		return "", nil, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", nil, fmt.Errorf("djangogo: create %q: %w", dir, err)
	}
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		return "", nil, fmt.Errorf("djangogo: write %q: %w", path, err)
	}
	return path, porting, nil
}

// sortedKeys returns the keys of set ordered by app and name.
func sortedKeys(set map[migrations.Key]bool) []migrations.Key {
	return slices.SortedFunc(maps.Keys(set), func(a, b migrations.Key) int {
		return cmp.Or(cmp.Compare(a.App, b.App), cmp.Compare(a.Name, b.Name))
	})
}
