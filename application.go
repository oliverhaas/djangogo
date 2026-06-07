package djangogo

import (
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/oliverhaas/djangogo/apps"
	"github.com/oliverhaas/djangogo/conf"
	"github.com/oliverhaas/djangogo/manage"
	"github.com/oliverhaas/djangogo/migrations"
	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/orm/backends/sqlite"
)

// Application is the wired-up framework instance: settings, the app registry,
// the command dispatcher, the root HTTP handler, and the database/migration plumbing.
type Application struct {
	Settings *conf.Settings
	Apps     *apps.Registry
	Commands *manage.Registry
	Handler  http.Handler
	Out      io.Writer

	DB            *orm.DB              // nil if no database configured
	Models        *orm.Registry        // frozen registry built from app models
	Migrations    *migrations.Registry // known migrations, seeded from the default registry
	MigrationsApp string               // the app name migrations are attributed to
	MigrationsDir string               // base dir for generated migration files (default "migrations")
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
		Settings:      s,
		Apps:          appReg,
		Commands:      manage.NewRegistry(),
		Handler:       defaultHandler(),
		Out:           os.Stdout,
		MigrationsDir: "migrations",
	}
	app.Commands.Out = app.Out

	// Build the model registry from every app that declares ORM models.
	modelReg := orm.NewRegistry()
	for _, c := range appConfigs {
		mp, ok := c.(apps.ModelProvider)
		if !ok {
			continue
		}
		for _, m := range mp.Models() {
			if _, err := modelReg.Register(m); err != nil {
				return nil, fmt.Errorf("djangogo: app %q: %w", c.Name(), err)
			}
		}
	}
	if err := modelReg.Resolve(); err != nil {
		return nil, fmt.Errorf("djangogo: resolve model relations: %w", err)
	}
	modelReg.Freeze()
	app.Models = modelReg

	app.MigrationsApp = migrationsApp(appConfigs)

	// Seed the migration registry from the process-global default so migrations
	// registered by generated files' init() are present.
	app.Migrations = migrations.NewRegistry()
	for _, m := range migrations.DefaultRegistry.All() {
		app.Migrations.Add(m)
	}

	// Open the database only if a DSN is configured.
	if s.Database.DSN != "" {
		switch s.Database.Driver {
		case "sqlite":
			sdb, err := sqlite.Open(s.Database.DSN)
			if err != nil {
				return nil, err
			}
			app.DB = orm.NewDB(sdb, sqlite.New(), modelReg)
		default:
			return nil, fmt.Errorf("djangogo: unsupported database driver %q", s.Database.Driver)
		}
	}

	// Built-in commands. Registration cannot collide here, so ignore the error.
	_ = app.Commands.Register(versionCommand{out: app.Out})
	_ = app.Commands.Register(&runserverCommand{app: app})
	_ = app.Commands.Register(&makemigrationsCommand{app: app})
	_ = app.Commands.Register(&migrateCommand{app: app})

	return app, nil
}

// migrationsApp determines the app name migrations are attributed to. If exactly
// one app declares models it uses that app's name; otherwise it falls back to "app".
func migrationsApp(appConfigs []apps.Config) string {
	name := "app"
	count := 0
	for _, c := range appConfigs {
		if _, ok := c.(apps.ModelProvider); ok {
			count++
			name = c.Name()
		}
	}
	if count == 1 {
		return name
	}
	return "app"
}

// Execute dispatches the given CLI arguments (typically os.Args[1:]).
func (a *Application) Execute(args []string) error {
	return a.Commands.Execute(args)
}
