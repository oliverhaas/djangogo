package apps_test

import (
	"io/fs"
	"testing"
	"testing/fstest"

	"github.com/oliverhaas/djangogo/apps"
	"github.com/oliverhaas/djangogo/checks"
	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/manage"
	"github.com/oliverhaas/djangogo/urls"
)

// providerApp is an app that implements every provider interface and
// contributes what its fields hold, so an unset field is an empty
// contribution of an app that has the interface.
type providerApp struct {
	name      string
	deps      []string
	templates fs.FS
	static    fs.FS
	commands  []manage.Command
	checks    []checks.Check
	models    []any
	routes    []urls.Route
}

// Name returns the app name.
func (a providerApp) Name() string { return a.name }

// Dependencies returns the declared dependencies.
func (a providerApp) Dependencies() []string { return a.deps }

// Templates returns the template tree.
func (a providerApp) Templates() fs.FS { return a.templates }

// Static returns the static tree.
func (a providerApp) Static() fs.FS { return a.static }

// Commands returns the management commands.
func (a providerApp) Commands() []manage.Command { return a.commands }

// Checks returns the system checks.
func (a providerApp) Checks() []checks.Check { return a.checks }

// Models returns the models.
func (a providerApp) Models() []any { return a.models }

// URLs returns the routes.
func (a providerApp) URLs() []urls.Route { return a.routes }

// namedCommand is a management command that only has a name.
type namedCommand string

// Name returns the command name.
func (c namedCommand) Name() string { return string(c) }

// Help returns no help text.
func (namedCommand) Help() string { return "" }

// Run does nothing.
func (namedCommand) Run([]string) error { return nil }

// fullApp is an app with a contribution of every kind.
type fullApp struct{}

// Name returns the app name.
func (fullApp) Name() string { return "full" }

// Templates returns a template tree.
func (fullApp) Templates() fs.FS { return tree("full/index.html") }

// Static returns a static tree.
func (fullApp) Static() fs.FS { return tree("full/site.css") }

// Commands returns a management command.
func (fullApp) Commands() []manage.Command { return []manage.Command{namedCommand("full")} }

// Checks returns a system check.
func (fullApp) Checks() []checks.Check { return []checks.Check{{Tags: []string{"full"}}} }

// Models returns a model.
func (fullApp) Models() []any { return []any{&TotallyNormal{}} }

// URLs returns a route.
func (fullApp) URLs() []urls.Route { return []urls.Route{{Pattern: "/full/", Name: "full"}} }

// tree returns a filesystem with one file, whose content is its own path.
func tree(path string) fs.FS {
	return fstest.MapFS{path: {Data: []byte(path)}}
}

// resolved registers the configs and resolves the start order, so a
// collector that follows the start order where it must not is caught.
func resolved(t *testing.T, configs ...apps.Config) *apps.Registry {
	t.Helper()
	reg := newRegistry(t, configs...)
	if err := reg.Resolve(); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	return reg
}

func TestTemplateFS(t *testing.T) {
	blog, admin := tree("blog/index.html"), tree("admin/index.html")
	reg := resolved(t,
		// The dependency moves blog behind admin in the start order only.
		providerApp{name: "github.com/acme/blog", templates: blog, deps: []string{"admin"}},
		plainApp{"sessions"},
		providerApp{name: "notemplates", static: tree("x.css")},
		providerApp{name: "django.contrib.admin", templates: admin},
	)
	djangotest.AssertEqual(t, startNames(reg), "sessions notemplates django.contrib.admin github.com/acme/blog")

	got := reg.TemplateFS()
	djangotest.AssertSequenceEqual(t, got, []apps.AppFS{{App: "blog", FS: blog}, {App: "admin", FS: admin}})

	data, err := fs.ReadFile(got[0].FS, "blog/index.html")
	if err != nil {
		t.Fatalf("read the template of blog: %v", err)
	}
	djangotest.AssertEqual(t, string(data), "blog/index.html")
}

func TestStaticFS(t *testing.T) {
	blog, admin := tree("css/blog.css"), tree("css/admin.css")
	reg := resolved(t,
		providerApp{name: "github.com/acme/blog", static: blog, deps: []string{"admin"}},
		plainApp{"sessions"},
		providerApp{name: "nostatic", templates: tree("x.html")},
		providerApp{name: "django.contrib.admin", static: admin},
	)

	got := reg.StaticFS()
	djangotest.AssertSequenceEqual(t, got, []apps.AppFS{{App: "blog", FS: blog}, {App: "admin", FS: admin}})

	data, err := fs.ReadFile(got[1].FS, "css/admin.css")
	if err != nil {
		t.Fatalf("read the static file of admin: %v", err)
	}
	djangotest.AssertEqual(t, string(data), "css/admin.css")
}

func TestCommands(t *testing.T) {
	reg := resolved(t,
		providerApp{name: "shop.blog", commands: []manage.Command{namedCommand("publish"), namedCommand("import")}, deps: []string{"auth"}},
		plainApp{"sessions"},
		providerApp{name: "nocommands"},
		providerApp{name: "auth", commands: []manage.Command{namedCommand("createsuperuser"), nil, namedCommand("import")}},
	)

	// The apps come in reverse installed order, so blog registers its
	// "import" after auth does and wins; a nil command is left out.
	djangotest.AssertSequenceEqual(t, reg.Commands(), []apps.AppCommand{
		{App: "auth", Command: namedCommand("createsuperuser")},
		{App: "auth", Command: namedCommand("import")},
		{App: "blog", Command: namedCommand("publish")},
		{App: "blog", Command: namedCommand("import")},
	})
}

func TestChecks(t *testing.T) {
	reg := resolved(t,
		providerApp{name: "blog", checks: []checks.Check{{Tags: []string{"blog.first"}}, {Tags: []string{"blog.second"}, Deploy: true}}, deps: []string{"auth"}},
		plainApp{"sessions"},
		providerApp{name: "nochecks"},
		providerApp{name: "auth", checks: []checks.Check{{Tags: []string{"auth"}}}},
	)

	djangotest.AssertSequenceEqual(t, reg.Checks(), []checks.Check{
		{Tags: []string{"blog.first"}},
		{Tags: []string{"blog.second"}, Deploy: true},
		{Tags: []string{"auth"}},
	})
}

func TestModels(t *testing.T) {
	reg := newRegistry(t,
		providerApp{name: "github.com/acme/blog", models: []any{&TotallyNormal{}}, deps: []string{"admin"}},
		plainApp{"sessions"},
		providerApp{name: "nomodels"},
		labeledApp{"django.contrib.auth", "myauth"},
		myAdmin{},
	)

	// Before Resolve the start order is the registration order.
	djangotest.AssertSequenceEqual(t, reg.Models(), []apps.AppModels{
		{App: "blog", Models: []any{&TotallyNormal{}}},
		{App: "nomodels"},
		{App: "admin", Models: []any{&LogEntry{}}},
	})

	if err := reg.Resolve(); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	// The models of a dependency come first, under the label of their app.
	djangotest.AssertSequenceEqual(t, reg.Models(), []apps.AppModels{
		{App: "nomodels"},
		{App: "admin", Models: []any{&LogEntry{}}},
		{App: "blog", Models: []any{&TotallyNormal{}}},
	})
}

func TestRoutes(t *testing.T) {
	reg := resolved(t,
		providerApp{name: "blog", routes: []urls.Route{{Pattern: "/blog/", Name: "blog-index"}, {Pattern: "/blog/{id}/", Name: "blog-detail"}}, deps: []string{"auth"}},
		plainApp{"sessions"},
		providerApp{name: "noroutes"},
		providerApp{name: "auth", routes: []urls.Route{{Pattern: "/login/", Name: "login"}}},
	)

	djangotest.AssertSequenceEqual(t, reg.Routes(), []urls.Route{
		{Pattern: "/blog/", Name: "blog-index"},
		{Pattern: "/blog/{id}/", Name: "blog-detail"},
		{Pattern: "/login/", Name: "login"},
	})
}

func TestCollectorsOfAnEmptyRegistry(t *testing.T) {
	reg := apps.NewRegistry()
	djangotest.AssertEqual(t, len(reg.TemplateFS()), 0)
	djangotest.AssertEqual(t, len(reg.StaticFS()), 0)
	djangotest.AssertEqual(t, len(reg.Commands()), 0)
	djangotest.AssertEqual(t, len(reg.Checks()), 0)
	djangotest.AssertEqual(t, len(reg.Models()), 0)
	djangotest.AssertEqual(t, len(reg.Routes()), 0)
	djangotest.AssertEqual(t, len(reg.Configs()), 0)
	djangotest.AssertEqual(t, len(reg.StartOrder()), 0)
}
