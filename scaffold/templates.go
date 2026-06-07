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

// projectMain is main.go: it wires the generated app into djangogo.New and runs
// the management CLI, mirroring cmd/djangogo/main.go.
const projectMain = `package main

import (
	"fmt"
	"os"

	"github.com/oliverhaas/djangogo"

	"{{.Module}}/{{.AppName}}"
)

func main() {
	app, err := djangogo.New(settings(), &{{.AppName}}.App{})
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

// appModels is models.go: a sample model exercising several orm field tags.
const appModels = `package {{.AppName}}

// Post is a sample model. Replace it with your own models and run
// "go run . makemigrations" to generate migrations.
type Post struct {
	ID        int64
	Title     string ` + "`orm:\"max_length=200\"`" + `
	Body      string ` + "`orm:\"type=text\"`" + `
	Published bool
}
`

// appAdmin is admin.go: a commented example showing how to register the app's
// models with the admin, without forcing the admin import on a fresh app.
const appAdmin = `package {{.AppName}}

// Register this app's models with the admin by uncommenting the import and the
// init function below, then wiring the returned site into your URL config.
//
// import "github.com/oliverhaas/djangogo/admin"
//
// func registerAdmin(site *admin.AdminSite) {
// 	admin.Register[Post](site, admin.ModelAdmin{})
// }
`

// appURLs is urls.go: a URL config stub returning this app's routes.
const appURLs = `package {{.AppName}}

import "github.com/oliverhaas/djangogo/urls"

// URLs returns this app's routes. Add routes with urls.Path and urls.PathFunc.
func URLs() []urls.Route {
	return []urls.Route{}
}
`
