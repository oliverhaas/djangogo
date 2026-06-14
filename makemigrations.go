package djangogo

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/oliverhaas/djangogo/migrations"
)

// makemigrationsCommand diffs the model registry against the known migrations
// and writes a new migration file when the schema has changed.
type makemigrationsCommand struct {
	app *Application
}

func (*makemigrationsCommand) Name() string { return "makemigrations" }
func (*makemigrationsCommand) Help() string { return "Create new migrations from model changes" }

func (c *makemigrationsCommand) Run(_ []string) error {
	app := c.app
	if app.Models == nil {
		return errors.New("djangogo: no model registry configured")
	}

	current := migrations.StateFromRegistry(app.Models)
	prior := app.Migrations.ForApp(app.MigrationsApp)

	dir := filepath.Join(app.MigrationsDir, app.MigrationsApp)
	pkgName := app.MigrationsApp
	if !isValidGoIdent(pkgName) {
		pkgName = "migrations"
	}

	mig, path, err := migrations.MakeMigrations(app.MigrationsApp, current, prior, dir, pkgName)
	if errors.Is(err, migrations.ErrNoChanges) {
		_, _ = fmt.Fprintln(app.Out, "No changes detected")
		return nil
	}
	if err != nil {
		return err
	}

	app.Migrations.Add(*mig)

	msg := "Created migration " + mig.Name
	if path != "" {
		msg += " -> " + path
	}
	_, _ = fmt.Fprintln(app.Out, msg)

	// Warn on field pairs that look like renames. The generated migration drops
	// the old column and adds a new empty one, discarding its data; Djan-Go-Go
	// has no interactive rename prompt, so the developer must edit the migration
	// to preserve the data if a rename was intended.
	priorState := migrations.StateFromMigrations(prior)
	for _, r := range migrations.DetectPotentialRenames(priorState, current) {
		_, _ = fmt.Fprintf(app.Out,
			"Warning: %s looks like a renamed field; the migration drops the old "+
				"column and adds a new empty one, discarding its data. Edit the "+
				"migration to rename the column instead if you meant to keep it.\n",
			r)
	}
	return nil
}

// isValidGoIdent reports whether s is a non-empty valid Go identifier: it starts
// with a letter or underscore and contains only letters, digits, and underscores.
func isValidGoIdent(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		switch {
		case r == '_':
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
			if i == 0 {
				return false
			}
		default:
			return false
		}
	}
	return true
}
