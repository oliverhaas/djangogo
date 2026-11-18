package djangogo

import (
	"context"
	"flag"
	"fmt"
	"strings"

	"github.com/oliverhaas/djangogo/manage"
	"github.com/oliverhaas/djangogo/sessions"
)

// clearsessionsCommand deletes the expired sessions of the session store
// (Django's clearsessions).
type clearsessionsCommand struct {
	app *Application
	// store is the session store to clean; nil means the database store on
	// the application's database.
	store sessions.Store
}

// Name returns "clearsessions".
func (*clearsessionsCommand) Name() string { return "clearsessions" }

// Help returns Django's help text of the command.
func (*clearsessionsCommand) Help() string {
	return "Can be run as a cronjob or directly to clean out expired sessions when the backend supports it."
}

// Run deletes the expired sessions.
func (c *clearsessionsCommand) Run(args []string) error {
	fs := flag.NewFlagSet("clearsessions", flag.ContinueOnError)
	fs.SetOutput(c.app.Out)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("clearsessions: unrecognized arguments: %s", strings.Join(fs.Args(), " "))
	}
	store := c.store
	if store == nil {
		if c.app.DB == nil {
			return errNoDatabase
		}
		store = sessions.NewDBStore(c.app.DB)
	}
	clearer, ok := store.(sessions.ExpiredClearer)
	if !ok {
		return &manage.CommandError{Msg: fmt.Sprintf("Session engine '%T' doesn't support clearing expired sessions.", store)}
	}
	return clearer.ClearExpired(context.Background())
}
