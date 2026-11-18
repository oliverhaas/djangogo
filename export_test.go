package djangogo

import (
	"context"
	"net"
	"time"
)

// DBShellArgs exposes dbshellArgs to the external tests, the counterpart of
// Django's DatabaseClient.settings_to_cmd_args_env.
var DBShellArgs = dbshellArgs

// RunserverAddr returns the host and the port runserver of app listens on
// for the addrport argument, with --ipv6 set as given: the address Django's
// runserver stores in self.addr and self.port.
func RunserverAddr(app *Application, addrport string, ipv6 bool) (host, port string, err error) {
	c := &runserverCommand{app: app, useIPv6: ipv6}
	return c.listenAddr(addrport)
}

// SetRunserverHooks sets the test hooks of the runserver command of app: the
// server stops when ctx is done, ready is called with the address once the
// server serves, and now gives the time of the banner. A nil hook keeps the
// default.
func SetRunserverHooks(ctx context.Context, app *Application, ready func(net.Addr), now func() time.Time) {
	cmd, _ := app.Commands.Get("runserver")
	c := cmd.(*runserverCommand)
	c.ctx, c.ready, c.now = ctx, ready, now
}

// CheckMigrations runs the unapplied-migrations notice of runserver of app.
func CheckMigrations(ctx context.Context, app *Application) {
	(&runserverCommand{app: app}).checkMigrations(ctx)
}
