package djangogo

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/oliverhaas/djangogo/apps"
	"github.com/oliverhaas/djangogo/conf"
	"github.com/oliverhaas/djangogo/manage"
	"github.com/oliverhaas/djangogo/orm"
)

// flushCommand empties all tables of the installed models and runs the
// post-migrate hooks (Django's flush).
type flushCommand struct {
	app       *Application
	noInput   bool
	database  string
	verbosity verbosityValue
}

// Name returns "flush".
func (*flushCommand) Name() string { return "flush" }

// Help returns Django's help text of the command.
func (*flushCommand) Help() string {
	return "Removes ALL DATA from the database, including data added during " +
		"migrations. Does not achieve a \"fresh install\" state."
}

// Flags declares Django's options of flush. It resets the values of an earlier
// run first.
func (c *flushCommand) Flags(fs *flag.FlagSet) {
	*c = flushCommand{
		app:       c.app,
		database:  conf.DefaultDatabaseAlias,
		verbosity: 1,
	}
	const noInputHelp = "Tells Django to NOT prompt the user for input of any kind."
	fs.BoolVar(&c.noInput, "noinput", false, noInputHelp)
	fs.BoolVar(&c.noInput, "no-input", false, noInputHelp)
	fs.StringVar(&c.database, "database", conf.DefaultDatabaseAlias,
		"Nominates a database to flush. Defaults to the \"default\" database.")
	c.verbosity.declare(fs)
}

// Run prompts for confirmation unless --noinput is set, flushes all model
// tables, and runs the application's post-migrate hooks.
func (c *flushCommand) Run(_ []string) error {
	a := c.app
	if err := checkDatabaseAliases(a.Settings, []string{c.database}); err != nil {
		return err
	}
	if a.DB == nil {
		return errNoDatabase
	}

	dbCfg := a.Settings.DefaultDatabase()
	if c.database != conf.DefaultDatabaseAlias {
		dbCfg = a.Settings.Databases[c.database]
	}
	dbName := dbCfg.DSN

	confirm := "yes"
	if !c.noInput {
		_, _ = fmt.Fprintf(a.out(), "You have requested a flush of the database.\n"+
			"This will IRREVERSIBLY DESTROY all data currently in the %q database,\n"+
			"and return each table to an empty state.\n"+
			"Are you sure you want to do this?\n\n"+
			"    Type 'yes' to continue, or 'no' to cancel: ", dbName)
		answer, err := bufio.NewReader(a.in()).ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return fmt.Errorf("djangogo: read answer: %w", err)
		}
		confirm = strings.TrimRight(answer, "\r\n")
	}

	if confirm != "yes" {
		_, _ = fmt.Fprintln(a.out(), "Flush cancelled.")
		return nil
	}

	tables := installedModelTables(a.Models)
	ctx := context.Background()
	if len(tables) > 0 {
		err := a.DB.Atomic(ctx, func(txCtx context.Context) error {
			return orm.Flush(txCtx, a.DB, tables...)
		})
		if err != nil {
			return manage.Errorf(
				"Database %s couldn't be flushed. Possible reasons:\n"+
					"  * The database isn't running or isn't configured correctly.\n"+
					"  * At least one of the expected database tables doesn't exist.\n"+
					"  * The SQL was invalid.\n"+
					"Hint: Look at the output of 'django-admin sqlflush'. "+
					"That's the SQL this command wasn't able to run.",
				dbName,
			)
		}

		if c.verbosity >= 2 {
			for _, cfg := range a.Apps.Configs() {
				if _, ok := cfg.(apps.ModelProvider); !ok {
					continue
				}
				_, _ = fmt.Fprintf(a.out(), "Running post-migrate handlers for application %s\n", apps.Label(cfg))
			}
		}
		for _, hook := range a.postMigrate {
			if err := hook(ctx, a.DB); err != nil {
				return fmt.Errorf("djangogo: post-migrate hook: %w", err)
			}
		}
	}
	return nil
}

// installedModelTables returns the table names of all registered models in reg
// together with their auto-created many-to-many join tables.
func installedModelTables(reg *orm.Registry) []string {
	if reg == nil {
		return nil
	}
	seen := make(map[string]bool)
	var tables []string
	add := func(t string) {
		if t != "" && !seen[t] {
			seen[t] = true
			tables = append(tables, t)
		}
	}
	for _, m := range reg.Models() {
		add(m.Table())
		for _, f := range m.ManyToMany() {
			if f.Rel != nil && f.Rel.Through != nil && f.Rel.AutoCreated {
				add(f.Rel.Through.Table())
			}
		}
	}
	return tables
}
