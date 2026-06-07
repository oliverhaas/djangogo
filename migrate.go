package djangogo

import (
	"context"
	"errors"
	"fmt"

	"github.com/oliverhaas/djangogo/migrations"
)

// migrateCommand applies any unapplied migrations to the configured database.
type migrateCommand struct {
	app *Application
}

func (*migrateCommand) Name() string { return "migrate" }
func (*migrateCommand) Help() string { return "Apply database migrations" }

func (c *migrateCommand) Run(_ []string) error {
	app := c.app
	if app.DB == nil {
		return errors.New("djangogo: no database configured (set Settings.Database.DSN)")
	}

	done, err := migrations.Apply(context.Background(), app.DB, app.Migrations.All())
	if err != nil {
		return err
	}

	if len(done) == 0 {
		_, _ = fmt.Fprintln(app.Out, "No migrations to apply.")
		return nil
	}

	_, _ = fmt.Fprintln(app.Out, "Applied:")
	for _, name := range done {
		_, _ = fmt.Fprintln(app.Out, "  "+name)
	}
	return nil
}
