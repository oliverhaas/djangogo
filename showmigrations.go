package djangogo

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"strings"
	"time"

	"github.com/oliverhaas/djangogo/migrations"
)

// showmigrationsCommand lists the migrations of the project and whether they
// are applied (Django's showmigrations).
type showmigrationsCommand struct {
	app       *Application
	verbosity verbosityValue
}

func (*showmigrationsCommand) Name() string { return "showmigrations" }
func (*showmigrationsCommand) Help() string { return "Show migrations and whether they are applied" }

func (c *showmigrationsCommand) Run(args []string) error {
	app := c.app
	fs := flag.NewFlagSet("showmigrations", flag.ContinueOnError)
	fs.SetOutput(app.Out)
	fs.Usage = func() {
		_, _ = fmt.Fprintln(app.Out, "usage: showmigrations [app_label ...] [--list | --plan] [-v {0,1,2,3}]")
		fs.PrintDefaults()
	}
	var list, plan bool
	fs.BoolVar(&list, "list", false, "show the migrations of each app (the default)")
	fs.BoolVar(&list, "l", false, "short for --list")
	fs.BoolVar(&plan, "plan", false, "show all migrations in the order they are applied")
	fs.BoolVar(&plan, "p", false, "short for --plan")
	c.verbosity = 1
	c.verbosity.declare(fs)
	labels, err := parseInterleaved(fs, args)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if list && plan {
		return commandError(2, "argument --plan: not allowed with argument --list")
	}

	// Without a database nothing counts as applied.
	ctx := context.Background()
	loader, err := migrations.NewLoader(ctx, app.Migrations, app.DB, app.Apps.Names())
	if err != nil {
		return err
	}
	if err := app.checkAppLabels(loader, labels, 2); err != nil {
		return err
	}
	if plan {
		c.showPlan(loader, labels)
		return nil
	}
	recorded := map[migrations.Key]time.Time{}
	if app.DB != nil {
		if recorded, err = migrations.NewRecorder(app.DB).AppliedMigrations(ctx); err != nil {
			return err
		}
	}
	return c.showList(loader, recorded, labels)
}

// println writes one line to the command output.
func (c *showmigrationsCommand) println(line string) {
	_, _ = fmt.Fprintln(c.app.Out, line)
}

// showList prints every app with its migrations from oldest to newest, each
// marked "[X]" when applied and "[ ]" when not. A squashed migration whose
// replaced migrations are applied but that has no row in the tracking table
// yet (recorded) is marked "[-]".
func (c *showmigrationsCommand) showList(loader *migrations.Loader, recorded map[migrations.Key]time.Time, labels []string) error {
	if len(labels) == 0 {
		labels = loader.MigratedApps()
	}
	for _, label := range labels {
		c.println(label)
		shown := map[migrations.Key]bool{}
		for _, leaf := range loader.Graph.LeafNodes(label) {
			steps, err := loader.Graph.ForwardsPlan(leaf)
			if err != nil {
				return err
			}
			for _, key := range steps {
				if shown[key] || key.App != label {
					continue
				}
				shown[key] = true
				title := key.Name
				squashed, isSquash := loader.Replacement(key)
				if isSquash {
					title += fmt.Sprintf(" (%d squashed migrations)", len(squashed.Replaces))
				}
				at, applied := loader.Applied[key]
				if !applied {
					c.println(" [ ] " + title)
					continue
				}
				line := " [X] " + title
				if _, ok := recorded[key]; !ok {
					line = " [-] " + title + " Run '" + c.app.prog() + " migrate' to finish recording."
				}
				// Like Django, show no time for a squashed migration.
				if c.verbosity >= 2 && !isSquash {
					line += " (applied at " + at.UTC().Format(time.DateTime) + ")"
				}
				c.println(line)
			}
		}
		if len(shown) == 0 {
			c.println(" (no migrations)")
		}
	}
	return nil
}

// showPlan prints all migrations, or those the given apps need, in the order
// in which migrate applies them. From verbosity 2 on, each line names the
// dependencies of the migration.
func (c *showmigrationsCommand) showPlan(loader *migrations.Loader, labels []string) {
	want := map[string]bool{}
	for _, label := range labels {
		want[label] = true
	}
	seen := map[migrations.Key]bool{}
	for _, leaf := range loader.Graph.LeafNodes() {
		if len(labels) > 0 && !want[leaf.App] {
			continue
		}
		// The loader checked the graph when it built it, so the plan of a
		// leaf node cannot fail.
		steps, _ := loader.Graph.ForwardsPlan(leaf)
		for _, key := range steps {
			if seen[key] {
				continue
			}
			seen[key] = true
			mark := "[ ]  "
			if _, ok := loader.Applied[key]; ok {
				mark = "[X]  "
			}
			deps := ""
			if parents := loader.Graph.Parents(key); c.verbosity >= 2 && len(parents) > 0 {
				names := make([]string, len(parents))
				for i, parent := range parents {
					names[i] = parent.String()
				}
				deps = " ... (" + strings.Join(names, ", ") + ")"
			}
			c.println(mark + key.String() + deps)
		}
	}
	if len(seen) == 0 {
		c.println("(no migrations)")
	}
}
