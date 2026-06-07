package djangogo

import (
	"io"
	"net/http"
	"os"

	"github.com/oliverhaas/djangogo/apps"
	"github.com/oliverhaas/djangogo/conf"
	"github.com/oliverhaas/djangogo/manage"
)

// Application is the wired-up framework instance: settings, the app registry,
// the command dispatcher, and the root HTTP handler.
type Application struct {
	Settings *conf.Settings
	Apps     *apps.Registry
	Commands *manage.Registry
	Handler  http.Handler
	Out      io.Writer
}

// New configures settings, registers and readies apps, registers built-in
// commands, and returns the Application. It returns an error for invalid
// settings or a failing app Ready hook.
func New(settings conf.Settings, appConfigs ...apps.Config) (*Application, error) {
	s := conf.Configure(settings)
	if err := s.Check(); err != nil {
		return nil, err
	}

	appReg := apps.NewRegistry()
	for _, c := range appConfigs {
		if err := appReg.Register(c); err != nil {
			return nil, err
		}
	}
	if err := appReg.Ready(); err != nil {
		return nil, err
	}

	app := &Application{
		Settings: s,
		Apps:     appReg,
		Commands: manage.NewRegistry(),
		Handler:  defaultHandler(),
		Out:      os.Stdout,
	}
	app.Commands.Out = app.Out

	// Built-in commands. Registration cannot collide here, so ignore the error.
	_ = app.Commands.Register(versionCommand{out: app.Out})
	_ = app.Commands.Register(&runserverCommand{app: app})

	return app, nil
}

// Execute dispatches the given CLI arguments (typically os.Args[1:]).
func (a *Application) Execute(args []string) error {
	return a.Commands.Execute(args)
}
