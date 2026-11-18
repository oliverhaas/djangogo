package apps

import (
	"io/fs"

	"github.com/oliverhaas/djangogo/checks"
	"github.com/oliverhaas/djangogo/conf"
	"github.com/oliverhaas/djangogo/manage"
	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/templates"
	"github.com/oliverhaas/djangogo/urls"
)

// Config is the minimal app contract: every app reports a unique name
// (AppConfig.name). The name is free-form; a Go import path such as
// "github.com/acme/blog" and a dotted path such as "django.contrib.admin" both
// work, and the label is derived from its last component.
type Config interface {
	Name() string
}

// Initializer is an optional hook run after all apps are registered (Django's ready()).
type Initializer interface {
	Ready() error
}

// ModelProvider is implemented by apps that declare ORM models. Models returns
// pointers to zero-valued model structs (e.g. &Article{}).
type ModelProvider interface {
	Models() []any
}

// URLProvider is implemented by apps that contribute URL routes (Django's
// urls.py). The framework collects every provider's routes into the root router.
type URLProvider interface {
	URLs() []urls.Route
}

// Labeler gives an app a label distinct from its name (AppConfig.label). The
// default label is the last dot- or slash-separated component of Name(). An
// empty label selects the default.
type Labeler interface {
	Label() string
}

// VerboseNamer gives the human-readable name (AppConfig.verbose_name); the
// default is the label in title case ("Blog"). An empty name selects the
// default.
type VerboseNamer interface {
	VerboseName() string
}

// DependencyProvider declares the apps (by label or name) that must be
// installed and started before this one.
type DependencyProvider interface {
	Dependencies() []string
}

// CommandProvider contributes management commands (an app's
// management/commands).
type CommandProvider interface {
	Commands() []manage.Command
}

// TemplateProvider contributes a template tree (an app's templates/
// directory).
type TemplateProvider interface {
	Templates() fs.FS
}

// StaticProvider contributes static files (an app's static/ directory), rooted
// so that "css/site.css" is served as STATIC_URL + "css/site.css".
type StaticProvider interface {
	Static() fs.FS
}

// CheckProvider contributes system checks.
type CheckProvider interface {
	Checks() []checks.Check
}

// Wirer is implemented by an app that needs framework services (the database,
// the template engine, the settings) to build its views; Wire runs in the
// second pass, before the app's routes are collected.
type Wirer interface {
	Wire(ctx *Context) error
}

// Context carries what the framework hands an app in the wire pass.
type Context struct {
	// Settings is the project configuration.
	Settings *conf.Settings
	// Models is the model registry built from the models of every app.
	Models *orm.Registry
	// DB is the default database; nil when no database is configured.
	DB *orm.DB
	// Templates is the project's template engine.
	Templates *templates.Engine
}

// Label returns the label of an app config: Labeler.Label when the config
// implements it and returns a non-empty label, otherwise the last dot- or
// slash-separated component of the name ("django.contrib.admin" and
// "github.com/acme/admin" both give "admin").
func Label(c Config) string {
	if l, ok := c.(Labeler); ok {
		if label := l.Label(); label != "" {
			return label
		}
	}
	return defaultLabel(c.Name())
}

// VerboseName returns the verbose name of an app config:
// VerboseNamer.VerboseName when the config implements it and returns a
// non-empty name, otherwise the label in title case, as Django's
// label.title() ("blog" gives "Blog", "my_app" gives "My_App").
func VerboseName(c Config) string {
	if v, ok := c.(VerboseNamer); ok {
		if name := v.VerboseName(); name != "" {
			return name
		}
	}
	return title(Label(c))
}
