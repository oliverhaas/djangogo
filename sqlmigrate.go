package djangogo

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"strings"

	"github.com/oliverhaas/djangogo/migrations"
)

// sqlmigrateCommand prints the SQL of one migration without running it
// (Django's sqlmigrate).
type sqlmigrateCommand struct {
	app *Application
}

func (*sqlmigrateCommand) Name() string { return "sqlmigrate" }
func (*sqlmigrateCommand) Help() string { return "Print the SQL statements of a migration" }

func (c *sqlmigrateCommand) Run(args []string) error {
	app := c.app
	fs := flag.NewFlagSet("sqlmigrate", flag.ContinueOnError)
	fs.SetOutput(app.Out)
	fs.Usage = func() {
		_, _ = fmt.Fprintln(app.Out, "usage: sqlmigrate app_label migration_name [--backwards]")
		fs.PrintDefaults()
	}
	backwards := fs.Bool("backwards", false, "print the SQL that unapplies the migration")
	pos, err := parseInterleaved(fs, args)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if len(pos) != 2 {
		return commandError(2, "sqlmigrate takes an app label and a migration name")
	}
	// The dialect of the database decides how the SQL looks.
	if app.DB == nil {
		return errNoDatabase
	}

	ctx := context.Background()
	loader, err := migrations.NewLoader(ctx, app.Migrations, app.DB, app.Apps.Names())
	if err != nil {
		return err
	}
	// Keep squashed and replaced migrations side by side, so both can be shown.
	loader.ReplaceMigrations = false
	if err := loader.BuildGraph(ctx); err != nil {
		return err
	}
	label, name := pos[0], pos[1]
	if err := app.checkAppLabels(loader, []string{label}, 1); err != nil {
		return err
	}
	if !loader.HasMigrations(label) {
		return commandError(1, "App '%s' does not have migrations", label)
	}
	m, err := findMigration(loader, label, name, " Is it in INSTALLED_APPS?")
	if err != nil {
		return err
	}

	statements, err := loader.CollectSQL(ctx, app.DB.Dialect(), []migrations.PlanStep{{Migration: m, Backwards: *backwards}})
	if err != nil {
		return err
	}
	if len(statements) == 0 {
		_, _ = fmt.Fprintln(app.Out, "No operations found.")
		return nil
	}
	// Both dialects roll back DDL, so a migration runs in a transaction.
	_, _ = fmt.Fprintf(app.Out, "BEGIN;\n%s\nCOMMIT;\n", strings.Join(statements, "\n"))
	return nil
}
