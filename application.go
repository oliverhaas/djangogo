package djangogo

import (
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/oliverhaas/djangogo/admin"
	"github.com/oliverhaas/djangogo/apps"
	"github.com/oliverhaas/djangogo/auth"
	"github.com/oliverhaas/djangogo/conf"
	"github.com/oliverhaas/djangogo/csrf"
	"github.com/oliverhaas/djangogo/manage"
	"github.com/oliverhaas/djangogo/migrations"
	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/orm/backends/sqlite"
	"github.com/oliverhaas/djangogo/sessions"
	"github.com/oliverhaas/djangogo/templates"
	"github.com/oliverhaas/djangogo/urls"
)

// Application is the wired-up framework instance: settings, the app registry,
// the command dispatcher, the root HTTP handler, and the database/migration plumbing.
type Application struct {
	Settings *conf.Settings
	Apps     *apps.Registry
	Commands *manage.Registry
	Handler  http.Handler
	Out      io.Writer
	In       io.Reader // command input (default os.Stdin); used by interactive prompts

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
		Out:           os.Stdout,
		In:            os.Stdin,
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

	// Assemble the root HTTP handler from the apps' URL configs and the admin.
	handler, err := app.buildHandler(appConfigs)
	if err != nil {
		return nil, err
	}
	app.Handler = handler

	// Built-in commands. Registration cannot collide here, so ignore the error.
	_ = app.Commands.Register(versionCommand{out: app.Out})
	_ = app.Commands.Register(&runserverCommand{app: app})
	_ = app.Commands.Register(&makemigrationsCommand{app: app})
	_ = app.Commands.Register(&migrateCommand{app: app})
	_ = app.Commands.Register(&createsuperuserCommand{app: app})
	_ = app.Commands.Register(&startprojectCommand{out: app.Out})
	_ = app.Commands.Register(&startappCommand{out: app.Out})

	return app, nil
}

// buildHandler assembles the root HTTP handler: every URLProvider app's routes
// plus, when a database is configured and an app opts into the admin, the mounted
// admin site. The router is wrapped in the sessions -> csrf -> auth middleware
// chain (auth only when a database is present, since it loads the request user),
// and the router's Reverse is wired in via templates.SetURLResolver so {% url %}
// resolves against it (Django's single root URLconf). With no routes it falls
// back to the liveness handler.
func (a *Application) buildHandler(appConfigs []apps.Config) (http.Handler, error) {
	var routes []urls.Route
	for _, c := range appConfigs {
		if up, ok := c.(apps.URLProvider); ok {
			routes = append(routes, up.URLs()...)
		}
	}

	if a.DB != nil {
		site, err := a.adminSite(appConfigs)
		if err != nil {
			return nil, err
		}
		if site != nil {
			routes = append(routes, site.Routes()...)
		}
	}

	if len(routes) == 0 {
		return defaultHandler(), nil
	}

	router, err := urls.NewRouterChecked(routes...)
	if err != nil {
		return nil, fmt.Errorf("djangogo: build router: %w", err)
	}
	templates.SetURLResolver(router.Reverse)

	var handler http.Handler = router
	if a.DB != nil {
		handler = auth.Middleware(a.DB)(handler)
	}
	handler = csrf.Middleware(handler)
	store := sessions.NewSignedCookieStore([]byte(a.Settings.SecretKey))
	handler = sessions.Middleware(store, "sessionid")(handler)
	return handler, nil
}

// adminSite builds the admin site and lets every app that implements
// RegisterAdmin(*admin.AdminSite) register its models with it. It returns a nil
// site (and nil error) when no app opts in, so the admin is mounted only when at
// least one app registers a model -- mirroring Django's contrib.admin opt-in.
func (a *Application) adminSite(appConfigs []apps.Config) (*admin.AdminSite, error) {
	type adminRegistrar interface {
		RegisterAdmin(*admin.AdminSite)
	}
	var registrars []adminRegistrar
	for _, c := range appConfigs {
		if ar, ok := c.(adminRegistrar); ok {
			registrars = append(registrars, ar)
		}
	}
	if len(registrars) == 0 {
		return nil, nil
	}
	site, err := admin.NewAdminSite(a.DB)
	if err != nil {
		return nil, fmt.Errorf("djangogo: build admin site: %w", err)
	}
	for _, ar := range registrars {
		ar.RegisterAdmin(site)
	}
	return site, nil
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

// in returns the configured command input, defaulting to os.Stdin when unset.
func (a *Application) in() io.Reader {
	if a.In != nil {
		return a.In
	}
	return os.Stdin
}
