package djangogo

import (
	"context"
	"errors"
	"flag"
	"fmt"

	"github.com/oliverhaas/djangogo/migrations"
)

// showmigrationsCommand lists the migrations of the project and whether they
// are applied (Django's showmigrations).
type showmigrationsCommand struct {
	app *Application
}

func (*showmigrationsCommand) Name() string { return "showmigrations" }
func (*showmigrationsCommand) Help() string { return "Show migrations and whether they are applied" }

func (c *showmigrationsCommand) Run(args []string) error {
	app := c.app
	fs := flag.NewFlagSet("showmigrations", flag.ContinueOnError)
	fs.SetOutput(app.Out)
	fs.Usage = func() {
		_, _ = fmt.Fprintln(app.Out, "usage: showmigrations [app_label ...] [--list | --plan]")
		fs.PrintDefaults()
	}
	var list, plan bool
	fs.BoolVar(&list, "list", false, "show the migrations of each app (the default)")
	fs.BoolVar(&list, "l", false, "short for --list")
	fs.BoolVar(&plan, "plan", false, "show all migrations in the order they are applied")
	fs.BoolVar(&plan, "p", false, "short for --plan")
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
	loader, err := migrations.NewLoader(context.Background(), app.Migrations, app.DB, app.Apps.Names())
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
	return c.showList(loader, labels)
}

// println writes one line to the command output.
func (c *showmigrationsCommand) println(line string) {
	_, _ = fmt.Fprintln(c.app.Out, line)
}

// showList prints every app with its migrations from oldest to newest, each
// marked "[X]" when applied and "[ ]" when not.
func (c *showmigrationsCommand) showList(loader *migrations.Loader, labels []string) error {
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
				mark := " [ ] "
				if _, ok := loader.Applied[key]; ok {
					mark = " [X] "
				}
				c.println(mark + key.Name)
				shown[key] = true
			}
		}
		if len(shown) == 0 {
			c.println(" (no migrations)")
		}
	}
	return nil
}

// showPlan prints all migrations, or those the given apps need, in the order
// in which migrate applies them.
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
			c.println(mark + key.String())
		}
	}
	if len(seen) == 0 {
		c.println("(no migrations)")
	}
}
