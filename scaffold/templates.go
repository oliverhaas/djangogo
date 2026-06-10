package scaffold

// projectGoMod is the go.mod for a generated project. The require uses the
// zero pseudo-version; go mod tidy resolves it (or the replace overrides it).
const projectGoMod = `module {{.Module}}

go {{.GoVersion}}

require github.com/oliverhaas/djangogo v0.0.0-00010101000000-000000000000
{{if .ReplacePath}}
replace github.com/oliverhaas/djangogo => {{.ReplacePath}}
{{end}}`

// projectSettings is settings.go: a settings() constructor returning a Debug
// SQLite configuration.
const projectSettings = `package main

import "github.com/oliverhaas/djangogo/conf"

// settings returns the configuration for this project. Edit it to point at your
// database and to set a real SecretKey before deploying.
func settings() conf.Settings {
	return conf.Settings{
		Debug:     true,
		SecretKey: "change-me-to-a-real-secret-key",
		Database: conf.Database{
			Driver: "sqlite",
			DSN:    "file:db.sqlite3?cache=shared",
		},
	}
}
`

// projectMain is main.go: it wires the generated app (and the built-in auth app,
// which the admin needs) into djangogo.New and runs the management CLI, mirroring
// cmd/djangogo/main.go.
const projectMain = `package main

import (
	"fmt"
	"os"

	"github.com/oliverhaas/djangogo"
	"github.com/oliverhaas/djangogo/auth"

	"{{.Module}}/{{.AppName}}"
)

func main() {
	app, err := djangogo.New(settings(), &{{.AppName}}.App{}, auth.App{})
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	if err := app.Execute(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
`

// appConfig is app.go: the App struct implementing apps.Config and
// apps.ModelProvider.
const appConfig = `package {{.AppName}}

// App is the application config for the {{.AppName}} package. It reports the app
// name and the models the app declares.
type App struct{}

// Name reports the app's unique name.
func (App) Name() string { return "{{.AppName}}" }

// Models returns pointers to the zero-valued models this app declares.
func (App) Models() []any { return []any{&Post{}} }
`

// appModels is models.go: a sample model exercising several orm field tags and a
// String() method (Django's __str__) so the admin and templates show a label.
const appModels = `package {{.AppName}}

// Post is a sample model. Replace it with your own models and run
// "go run . makemigrations" to generate migrations.
type Post struct {
	ID        int64
	Title     string ` + "`orm:\"max_length=200\"`" + `
	Body      string ` + "`orm:\"type=text\"`" + `
	Published bool
}

// String is Django's __str__: the admin changelist and templates use it as the
// object's label.
func (p Post) String() string { return p.Title }
`

// appAdmin is admin.go: the app's RegisterAdmin hook. djangogo.New calls it with
// the project's admin site (mounted at /admin/) when a database is configured.
const appAdmin = `package {{.AppName}}

import "github.com/oliverhaas/djangogo/admin"

// RegisterAdmin registers this app's models with the project's admin site.
// djangogo.New calls it automatically and mounts the admin at /admin/.
func (App) RegisterAdmin(site *admin.AdminSite) {
	admin.Register[Post](site, admin.ModelAdmin{
		ListDisplay: []string{"ID", "Title", "Published"},
		Ordering:    []string{"-id"},
	})
}
`

// appURLs is urls.go: the app's URL config. djangogo.New mounts these routes at
// the site root. Add data-backed pages with views.ListView / views.DetailView.
const appURLs = `package {{.AppName}}

import (
	"net/http"

	"github.com/oliverhaas/djangogo/urls"
)

// URLs returns this app's routes, mounted at the site root by djangogo.New.
func (App) URLs() []urls.Route {
	return []urls.Route{
		urls.PathFunc("GET /{$}", welcome, "home"),
	}
}

// welcome is a placeholder landing page. Replace it with your own views.
func welcome(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(` + "`" + `<h1>It worked!</h1>
<p>Your {{.AppName}} app is running. The admin is at <a href="/admin/">/admin/</a>.</p>` + "`" + `))
}
`
