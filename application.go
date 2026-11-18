package djangogo

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"

	"github.com/oliverhaas/djangogo/admin"
	"github.com/oliverhaas/djangogo/apps"
	"github.com/oliverhaas/djangogo/auth"
	"github.com/oliverhaas/djangogo/checks"
	"github.com/oliverhaas/djangogo/conf"
	"github.com/oliverhaas/djangogo/contenttypes"
	"github.com/oliverhaas/djangogo/csrf"
	"github.com/oliverhaas/djangogo/manage"
	"github.com/oliverhaas/djangogo/messages"
	"github.com/oliverhaas/djangogo/middleware"
	"github.com/oliverhaas/djangogo/migrations"
	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/orm/backends/postgres"
	"github.com/oliverhaas/djangogo/orm/backends/sqlite"
	"github.com/oliverhaas/djangogo/sessions"
	"github.com/oliverhaas/djangogo/staticfiles"
	"github.com/oliverhaas/djangogo/templates"
	"github.com/oliverhaas/djangogo/urls"
	"github.com/oliverhaas/djangogo/views"
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
	// Err receives the diagnostics: system check warnings, command errors,
	// the request log of runserver and the panic log of the Recover
	// middleware (default os.Stderr). Execute and Run pass Out and Err on to
	// Commands, so they may be replaced after New.
	Err io.Writer
	// Templates is the project's template engine: the directories of
	// Settings.Templates.Dirs first, then, with Settings.Templates.AppDirs,
	// the template trees of the apps in INSTALLED_APPS order.
	Templates *templates.Engine
	// Checks holds the system checks of the project: the built-in ones of
	// checks.Default and those the apps contribute.
	Checks *checks.Registry
	// Static finds the static files of Settings.StaticFilesDirs and of the
	// apps, in this order (Django's FileSystemFinder, then
	// AppDirectoriesFinder).
	Static *staticfiles.Finder
	// stack is the default middleware stack, outermost first, project the
	// middleware added with Use, and root the handler they wrap.
	stack, project []func(http.Handler) http.Handler
	root           http.Handler

	DB            *orm.DB              // nil if no database configured
	Models        *orm.Registry        // frozen registry built from app models
	Migrations    *migrations.Registry // known migrations, seeded from the default registry
	MigrationsDir string               // base dir for generated migration files (default "migrations")

	postMigrate []func(ctx context.Context, db *orm.DB) error // hooks run after a successful migrate

	// sessions is the store of the session middleware, which clearsessions
	// cleans.
	sessions sessions.Store
}

// OnPostMigrate registers a hook that runs after the migrate command has
// brought the database to its target, the counterpart of a receiver of
// Django's post_migrate signal. Hooks run in the order of registration; the
// first error stops the command.
func (a *Application) OnPostMigrate(hook func(ctx context.Context, db *orm.DB) error) {
	a.postMigrate = append(a.postMigrate, hook)
}

// New configures the settings, registers the apps, builds the model registry,
// opens the database, starts the apps (wire and ready passes), assembles the
// root handler and registers the commands. It returns an error for invalid
// settings, an app that cannot be registered or started, a database that
// cannot be opened, and invalid middleware settings. Call Close when done.
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
	if err := appReg.Resolve(); err != nil {
		return nil, err
	}

	app := &Application{
		Settings:      s,
		Apps:          appReg,
		Commands:      manage.NewRegistry(),
		Out:           os.Stdout,
		Err:           os.Stderr,
		In:            os.Stdin,
		MigrationsDir: "migrations",
	}
	app.Commands.Out = app.Out

	// Build the model registry from every app that declares ORM models, under
	// the app label and in start order, so the models of a dependency come
	// before the models that relate to them.
	modelReg := orm.NewRegistry()
	for _, am := range appReg.Models() {
		for _, m := range am.Models {
			if _, err := modelReg.RegisterApp(am.App, m); err != nil {
				return nil, fmt.Errorf("djangogo: app %q: %w", am.App, err)
			}
		}
	}
	if err := modelReg.Resolve(); err != nil {
		return nil, fmt.Errorf("djangogo: resolve model relations: %w", err)
	}
	modelReg.Freeze()
	app.Models = modelReg

	// Seed the migration registry from the process-global default so migrations
	// registered by generated files' init() are present.
	app.Migrations = migrations.NewRegistry()
	for _, m := range migrations.DefaultRegistry.All() {
		app.Migrations.Add(m)
	}

	db, err := openDatabase(s.DefaultDatabase(), modelReg)
	if err != nil {
		return nil, err
	}
	app.DB = db

	if err := app.configureAuth(); err != nil {
		_ = app.Close()
		return nil, err
	}
	// The receivers of post_migrate that contenttypes and auth connect when
	// they are ready, so they run before the hooks of the project.
	if appReg.IsInstalled(contenttypes.App{}.Name()) {
		app.OnPostMigrate(app.syncContentTypes)
	}

	if err := app.start(); err != nil {
		_ = app.Close()
		return nil, err
	}

	// Built-in commands. Registration cannot collide here, so ignore the error.
	_ = app.Commands.Register(&versionCommand{app: app})
	_ = app.Commands.Register(&runserverCommand{app: app})
	_ = app.Commands.Register(&makemigrationsCommand{app: app})
	_ = app.Commands.Register(&migrateCommand{app: app})
	_ = app.Commands.Register(&showmigrationsCommand{app: app})
	_ = app.Commands.Register(&sqlmigrateCommand{app: app})
	_ = app.Commands.Register(&createsuperuserCommand{app: app})
	_ = app.Commands.Register(&changepasswordCommand{app: app})
	_ = app.Commands.Register(&clearsessionsCommand{app: app, store: app.sessions})
	_ = app.Commands.Register(&startprojectCommand{app: app})
	_ = app.Commands.Register(&startappCommand{app: app})
	app.registerCoreCommands()

	return app, nil
}

// openDatabase opens the database of the settings, or returns nil when it has
// no DSN. The driver names "postgres", "postgresql" and "pgx" select
// PostgreSQL, "sqlite" and "sqlite3" SQLite.
func openDatabase(db conf.Database, reg *orm.Registry) (*orm.DB, error) {
	if db.DSN == "" {
		return nil, nil
	}
	driver, ok := conf.NormalizeDriver(db.Driver)
	if !ok {
		return nil, fmt.Errorf("djangogo: unsupported database driver %q (want %q or %q)",
			db.Driver, conf.DriverSQLite, conf.DriverPostgres)
	}
	var (
		open    = sqlite.Open
		dialect = sqlite.New()
	)
	if driver == conf.DriverPostgres {
		open, dialect = postgres.Open, postgres.New()
	}
	sqlDB, err := open(db.DSN)
	if err != nil {
		return nil, fmt.Errorf("djangogo: open %s database: %w", driver, err)
	}
	return orm.NewDB(sqlDB, dialect, reg), nil
}

// start runs the wire and ready passes of the apps and builds what depends
// on them: the template engine handed to the wire pass, the system checks,
// the static file finder and the root handler.
func (a *Application) start() error {
	engine, err := a.templateEngine()
	if err != nil {
		return err
	}
	a.Templates = engine
	ctx := &apps.Context{Settings: a.Settings, Models: a.Models, DB: a.DB, Templates: engine}
	if err := a.Apps.Wire(ctx); err != nil {
		return err
	}
	if err := a.Apps.Ready(); err != nil {
		return err
	}

	a.Checks = checks.Default.Clone()
	a.Checks.Add(a.Apps.Checks()...)
	a.Static = a.staticFinder()
	return a.buildHandler()
}

// templateEngine builds the project's template engine over the template
// directories of the settings and the template trees of the apps, searched
// in this order as Django's DIRS and APP_DIRS are, with the models of the
// project, the context processors of the settings and template caching off
// under Debug. The router sets its resolver once it is built.
func (a *Application) templateEngine() (*templates.Engine, error) {
	s := a.Settings
	t := s.Templates
	sources := make([]templates.Source, 0, len(t.Dirs))
	for _, dir := range t.Dirs {
		sources = append(sources, templates.DirSource(dir))
	}
	if t.AppDirs {
		for _, tree := range a.Apps.TemplateFS() {
			sources = append(sources, templates.FSSource(tree.App, tree.FS))
		}
	}
	engine := templates.New(sources...)
	engine.SetModels(a.Models)
	engine.SetDebug(s.Debug)
	processors, err := a.contextProcessors()
	if err != nil {
		return nil, err
	}
	engine.AddContextProcessor(processors...)
	// {% static %} prefixes STATIC_URL. The prefix is global to the
	// templates package; it is written only when it changes.
	if s.StaticURL != "" && templates.StaticBase != s.StaticURL {
		templates.StaticBase = s.StaticURL
	}
	return engine, nil
}

// staticFinder builds the finder over Settings.StaticFilesDirs and the static
// trees of the apps.
func (a *Application) staticFinder() *staticfiles.Finder {
	trees := a.Apps.StaticFS()
	sources := make([]staticfiles.Source, 0, len(a.Settings.StaticFilesDirs)+len(trees))
	for _, dir := range a.Settings.StaticFilesDirs {
		sources = append(sources, staticfiles.DirSource(dir))
	}
	for _, tree := range trees {
		sources = append(sources, staticfiles.Source{Name: tree.App, FS: tree.FS})
	}
	return staticfiles.NewFinder(sources...)
}

// buildHandler builds the router and the default middleware stack and
// composes them into Handler.
func (a *Application) buildHandler() error {
	root, err := a.router()
	if err != nil {
		return err
	}
	a.root = root
	stack, err := a.defaultMiddleware()
	if err != nil {
		return err
	}
	a.stack = stack
	a.compose()
	return nil
}

// compose sets Handler to the default stack and the project middleware
// around the root handler.
func (a *Application) compose() {
	mws := append(slices.Clone(a.stack), a.project...)
	a.Handler = middleware.Chain(mws...)(a.root)
}

// defaultMiddleware returns the default middleware stack, outermost first, in
// the order of the MIDDLEWARE setting of Django's project template:
// security, sessions, common, CSRF, authentication, messages, clickjacking.
// Recover comes before all of them, so a panic anywhere below it becomes a
// 500 response, and host validation follows it, because Django validates the
// host when the first middleware reads it. The APPEND_SLASH redirect of
// CommonMiddleware is done by the router. It reads the root handler, so
// buildHandler sets that first.
func (a *Application) defaultMiddleware() ([]func(http.Handler) http.Handler, error) {
	s := a.Settings
	security, err := middleware.Security(middleware.SecurityOptions{
		SSLRedirect:             s.SecureSSLRedirect,
		SSLHost:                 s.SecureSSLHost,
		RedirectExempt:          s.SecureRedirectExempt,
		HSTSSeconds:             s.SecureHSTSSeconds,
		HSTSIncludeSubdomains:   s.SecureHSTSIncludeSubdomains,
		HSTSPreload:             s.SecureHSTSPreload,
		ContentTypeNosniff:      s.SecureContentTypeNosniff,
		ReferrerPolicy:          s.SecureReferrerPolicy,
		CrossOriginOpenerPolicy: s.SecureCrossOriginOpenerPolicy,
		ProxySSLHeader:          s.SecureProxySSLHeader,
	})
	if err != nil {
		return nil, fmt.Errorf("djangogo: security middleware: %w", err)
	}

	a.sessions = a.sessionStore()
	stack := []func(http.Handler) http.Handler{
		middleware.Recover(middleware.RecoverOptions{
			Debug:             s.Debug,
			Log:               errWriter{a},
			SessionCookieName: s.SessionCookieName,
		}),
		middleware.AllowedHosts(middleware.AllowedHostsOptions{
			AllowedHosts:      s.AllowedHosts,
			Debug:             s.Debug,
			UseXForwardedHost: s.UseXForwardedHost,
			UseXForwardedPort: s.UseXForwardedPort,
		}),
		security,
		sessions.MiddlewareWithOptions(a.sessions, a.sessionOptions()),
		csrf.MiddlewareWithOptions(a.csrfOptions(a.resolveView())),
	}
	// Authentication loads the request user from the database.
	if a.DB != nil {
		stack = append(stack, auth.Middleware(a.DB))
	}
	stack = append(stack, messages.Middleware, middleware.XFrameOptions(s.XFrameOptions))
	return stack, nil
}

// router collects the routes of every URLProvider app plus, when a database
// is configured and an app opts into the admin, the mounted admin site. The
// router's Reverse is wired in via templates.SetURLResolver so {% url %}
// resolves against it (Django's single root URLconf). With no routes it
// falls back to the liveness handler.
func (a *Application) router() (http.Handler, error) {
	routes := a.Apps.Routes()
	if a.DB != nil {
		site, err := a.adminSite(a.Apps.Configs())
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
	router.AppendSlash = a.Settings.AppendSlash
	router.Debug = a.Settings.Debug
	if !a.Settings.Debug {
		router.NotFoundHandler = http.HandlerFunc(a.notFound)
	}
	a.Templates.SetResolver(router.Reverse)
	// The engines the admin and the auth views build for themselves use the
	// global resolver.
	templates.SetURLResolver(router.Reverse)
	return router, nil
}

// resolveView returns the function the CSRF middleware asks for the view of
// a request, so that a view marked with csrf.Exempt is not checked. It is nil
// for a project without routes.
func (a *Application) resolveView() func(*http.Request) http.Handler {
	router, ok := a.root.(*urls.Router)
	if !ok {
		return nil
	}
	return func(r *http.Request) http.Handler {
		m, err := router.Resolve(r.URL.Path)
		if err != nil {
			return nil
		}
		return m.Handler
	}
}

// notFound answers a request no route matches with the 404.html of the
// project, or Django's default page without one (handler404). A 404.html that
// fails to render is an error of the server, as in Django.
func (a *Application) notFound(w http.ResponseWriter, r *http.Request) {
	if err := views.NotFoundPage(w, r, a.Templates, nil, ""); err != nil {
		_, _ = fmt.Fprintf(a.errOut(), "djangogo: render the 404 page: %v\n", err)
		http.Error(w, "Server Error (500)", http.StatusInternalServerError)
	}
}

// Use adds project middleware, the project's own entries at the end of
// Django's MIDDLEWARE: they run inside the default stack, after
// authentication and clickjacking protection, around the router, in the order
// given (the first wraps the rest). Use rebuilds Handler, so call it before
// serving.
func (a *Application) Use(mws ...func(http.Handler) http.Handler) {
	a.project = append(a.project, mws...)
	a.compose()
}

// Close releases the database connections. It does nothing for an
// application without a database, and a second call does nothing.
func (a *Application) Close() error {
	if a.DB == nil {
		return nil
	}
	if err := a.DB.SQL().Close(); err != nil {
		return fmt.Errorf("djangogo: close database: %w", err)
	}
	return nil
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

// Execute dispatches the given CLI arguments (typically os.Args[1:]) and
// returns the error of the command without printing it.
func (a *Application) Execute(args []string) error {
	a.syncStreams()
	return a.Commands.Execute(args)
}

// Run is Execute for a main function, Django's execute_from_command_line: it
// prints an error to Err as manage.py does and returns the exit code of the
// process (0 on success, 1 for a failed command or failed system checks, 2
// for a usage error).
//
//	os.Exit(app.Run(os.Args[1:]))
func (a *Application) Run(args []string) int {
	a.syncStreams()
	return a.Commands.Run(args)
}

// syncStreams points the command registry at Out and Err, which a caller may
// have replaced after New.
func (a *Application) syncStreams() {
	a.Commands.Out = a.out()
	a.Commands.Err = a.errOut()
}

// in returns the configured command input, defaulting to os.Stdin when unset.
func (a *Application) in() io.Reader {
	if a.In != nil {
		return a.In
	}
	return os.Stdin
}

// out returns the command output, defaulting to os.Stdout when unset.
func (a *Application) out() io.Writer {
	if a.Out != nil {
		return a.Out
	}
	return os.Stdout
}

// errOut returns the diagnostics stream, defaulting to os.Stderr when unset.
func (a *Application) errOut() io.Writer {
	if a.Err != nil {
		return a.Err
	}
	return os.Stderr
}

// errWriter writes to the Err of an application as it is at the time of the
// write, so a caller may replace Err after New.
type errWriter struct{ a *Application }

// Write writes p to the current Err.
func (w errWriter) Write(p []byte) (int, error) { return w.a.errOut().Write(p) }

// prog returns the program name the commands print in hints, as the help
// of Commands shows it.
func (a *Application) prog() string {
	if a.Commands.Prog != "" {
		return a.Commands.Prog
	}
	if len(os.Args) == 0 {
		return ""
	}
	return filepath.Base(os.Args[0])
}
