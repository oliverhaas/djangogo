package djangogo

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/oliverhaas/djangogo/migrations"
)

// errNoDatabase is returned by the commands that need a database.
var errNoDatabase = errors.New("djangogo: no database configured (set Settings.Database.DSN)")

// parseInterleaved parses args with fs and returns the positional arguments.
// The flag package stops at the first positional argument; Django's commands
// accept options after it ("migrate blog --plan"), so parsing resumes behind
// each one. Everything after "--" is positional.
func parseInterleaved(fs *flag.FlagSet, args []string) ([]string, error) {
	var rest []string
	if i := slices.Index(args, "--"); i >= 0 {
		args, rest = args[:i], args[i+1:]
	}
	var positional []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		args = fs.Args()
		if len(args) == 0 {
			return append(positional, rest...), nil
		}
		positional = append(positional, args[0])
		args = args[1:]
	}
}

// commandError builds an error whose message is shown to the user as it is.
func commandError(code int, format string, args ...any) error {
	return &migrations.CommandError{Message: fmt.Sprintf(format, args...), Code: code}
}

// checkAppLabels returns Django's error for the labels that name no app. An
// app counts as known when it is registered or has migrations. code is the
// exit status Django uses in the calling command.
func (a *Application) checkAppLabels(loader *migrations.Loader, labels []string, code int) error {
	var bad []string
	for _, label := range labels {
		if _, ok := a.Apps.Get(label); ok || loader.HasMigrations(label) {
			continue
		}
		bad = append(bad, fmt.Sprintf("No installed app with label '%s'.", label))
	}
	if len(bad) == 0 {
		return nil
	}
	return commandError(code, "%s", strings.Join(bad, "\n"))
}

// conflictError returns Django's error for apps with more than one leaf
// migration, or nil when there is none. prog is the program name the hint
// names in place of "python manage.py".
func conflictError(conflicts map[string][]string, prog string) error {
	if len(conflicts) == 0 {
		return nil
	}
	apps := make([]string, 0, len(conflicts))
	for app := range conflicts {
		apps = append(apps, app)
	}
	sort.Strings(apps)
	parts := make([]string, len(apps))
	for i, app := range apps {
		parts[i] = fmt.Sprintf("%s in %s", strings.Join(conflicts[app], ", "), app)
	}
	return commandError(1, "Conflicting migrations detected; multiple leaf nodes in the migration graph: (%s).\n"+
		"To fix them run '%s makemigrations --merge'", strings.Join(parts, "; "), prog)
}

// findMigration resolves a migration name prefix with Django's error
// messages. notFoundHint is appended to the message for an unknown name.
func findMigration(loader *migrations.Loader, app, prefix, notFoundHint string) (*migrations.Migration, error) {
	m, err := loader.MigrationByPrefix(app, prefix)
	var ambiguous *migrations.AmbiguityError
	switch {
	case errors.As(err, &ambiguous):
		return nil, commandError(1, "More than one migration matches '%s' in app '%s'. Please be more specific.", prefix, app)
	case errors.Is(err, migrations.ErrMigrationNotFound):
		return nil, commandError(1, "Cannot find a migration matching '%s' from app '%s'.%s", prefix, app, notFoundHint)
	case err != nil:
		return nil, err
	}
	return m, nil
}

// newAutodetector returns the autodetector that leads from the state of the
// migrations to the state of the registered models.
func (a *Application) newAutodetector(loader *migrations.Loader, q migrations.Questioner) (*migrations.Autodetector, error) {
	from, err := loader.ProjectState(nil, true)
	if err != nil {
		return nil, err
	}
	detector := migrations.NewAutodetector(from, migrations.StateFromRegistry(a.Models), q)
	// The model registry cannot express model-level indexes and
	// unique-together sets, so the ones declared in migrations stay.
	detector.PreserveModelOptions = true
	return detector, nil
}

// pendingModelChanges returns the sorted apps whose models differ from the
// state the migrations produce.
func (a *Application) pendingModelChanges(loader *migrations.Loader) ([]string, error) {
	if a.Models == nil {
		return nil, nil
	}
	detector, err := a.newAutodetector(loader, nil)
	if err != nil {
		return nil, err
	}
	changes, err := detector.Changes(loader.Graph, nil, "")
	if err != nil {
		return nil, err
	}
	apps := make([]string, 0, len(changes))
	for app := range changes {
		apps = append(apps, app)
	}
	sort.Strings(apps)
	return apps, nil
}

// migrateCommand brings the database to the latest migrations, or forwards
// or backwards to a named one (Django's migrate).
type migrateCommand struct {
	app *Application
}

func (*migrateCommand) Name() string { return "migrate" }
func (*migrateCommand) Help() string { return "Apply or unapply database migrations" }

func (c *migrateCommand) Run(args []string) error {
	app := c.app
	fs := flag.NewFlagSet("migrate", flag.ContinueOnError)
	fs.SetOutput(app.Out)
	fs.Usage = func() {
		_, _ = fmt.Fprintln(app.Out, "usage: migrate [app_label] [migration_name] [--fake] [--plan] [--noinput]")
		fs.PrintDefaults()
	}
	fake := fs.Bool("fake", false, "mark migrations as run without actually running them")
	showPlan := fs.Bool("plan", false, "show the operations that would be performed, without running them")
	noInput := fs.Bool("noinput", false, "never prompt for input")
	fs.BoolVar(noInput, "no-input", false, "never prompt for input")
	pos, err := parseInterleaved(fs, args)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if len(pos) > 2 {
		return commandError(2, "migrate takes at most an app label and a migration name, got: %s", strings.Join(pos, " "))
	}
	if app.DB == nil {
		return errNoDatabase
	}

	ctx := context.Background()
	executor, err := migrations.NewExecutor(ctx, app.DB, app.Migrations, app.Apps.Names())
	if err != nil {
		return err
	}
	loader := executor.Loader
	if err := loader.CheckConsistentHistory(); err != nil {
		return err
	}
	if err := conflictError(loader.DetectConflicts(), c.app.prog()); err != nil {
		return err
	}

	targets, err := c.targets(loader, pos)
	if err != nil {
		return err
	}
	plan, err := executor.MigrationPlan(targets, false)
	if err != nil {
		return err
	}
	if *showPlan {
		c.printPlan(plan)
		return nil
	}

	c.printTargets(targets, len(pos) == 2)
	c.println("Running migrations:")
	if len(plan) == 0 {
		c.println("  No migrations to apply.")
		pending, err := app.pendingModelChanges(loader)
		if err != nil {
			return err
		}
		if len(pending) > 0 {
			quoted := make([]string, len(pending))
			for i, label := range pending {
				quoted[i] = "'" + label + "'"
			}
			c.println("  Your models in app(s): " + strings.Join(quoted, ", ") +
				" have changes that are not yet reflected in a migration, and so won't be applied.")
			prog := c.app.prog()
			c.println("  Run '" + prog + " makemigrations' to make new migrations, and then re-run '" +
				prog + " migrate' to apply them.")
		}
	}

	open := false
	executor.Progress = func(event string, m *migrations.Migration, faked bool) {
		switch event {
		case migrations.ApplyStart:
			_, _ = fmt.Fprintf(app.Out, "  Applying %s...", m)
			open = true
		case migrations.UnapplyStart:
			_, _ = fmt.Fprintf(app.Out, "  Unapplying %s...", m)
			open = true
		case migrations.ApplySuccess, migrations.UnapplySuccess:
			if faked {
				c.println(" FAKED")
			} else {
				c.println(" OK")
			}
			open = false
		}
	}
	if len(plan) > 0 {
		if _, err := executor.Migrate(ctx, targets, plan, *fake); err != nil {
			if open {
				// End the "Applying ..." line the failed step left open.
				c.println("")
			}
			return err
		}
	}

	for _, hook := range app.postMigrate {
		if err := hook(ctx, app.DB); err != nil {
			return fmt.Errorf("djangogo: post-migrate hook: %w", err)
		}
	}
	return nil
}

// println writes one line to the command output.
func (c *migrateCommand) println(line string) {
	_, _ = fmt.Fprintln(c.app.Out, line)
}

// targets turns the positional arguments into migration targets: all leaf
// migrations, the leaves of one app, one named migration, or "zero" for an
// app without any applied migration.
func (c *migrateCommand) targets(loader *migrations.Loader, pos []string) ([]migrations.Key, error) {
	if len(pos) == 0 {
		return loader.Graph.LeafNodes(), nil
	}
	label := pos[0]
	if err := c.app.checkAppLabels(loader, []string{label}, 1); err != nil {
		return nil, err
	}
	if !loader.HasMigrations(label) {
		return nil, commandError(1, "App '%s' does not have migrations.", label)
	}
	if len(pos) == 1 {
		return loader.Graph.LeafNodes(label), nil
	}
	if pos[1] == "zero" {
		return []migrations.Key{{App: label}}, nil
	}
	m, err := findMigration(loader, label, pos[1], "")
	if err != nil {
		return nil, err
	}
	return []migrations.Key{m.Key()}, nil
}

// printTargets prints the "Operations to perform" block.
func (c *migrateCommand) printTargets(targets []migrations.Key, named bool) {
	c.println("Operations to perform:")
	switch {
	case !named:
		set := map[string]bool{}
		for _, t := range targets {
			set[t.App] = true
		}
		apps := make([]string, 0, len(set))
		for app := range set {
			apps = append(apps, app)
		}
		sort.Strings(apps)
		list := strings.Join(apps, ", ")
		if list == "" {
			list = "(none)"
		}
		c.println("  Apply all migrations: " + list)
	case targets[0].Name == "":
		c.println("  Unapply all migrations: " + targets[0].App)
	default:
		c.println(fmt.Sprintf("  Target specific migration: %s, from %s", targets[0].Name, targets[0].App))
	}
}

// printPlan prints what migrate --plan shows: every migration of the plan
// with its operations.
func (c *migrateCommand) printPlan(plan []migrations.PlanStep) {
	c.println("Planned operations:")
	if len(plan) == 0 {
		c.println("  No planned migration operations.")
		return
	}
	for _, step := range plan {
		c.println(step.Migration.String())
		for _, op := range step.Migration.Operations {
			message, _ := migrations.DescribeStep(op, step.Backwards)
			c.println("    " + message)
		}
	}
}
