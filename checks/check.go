package checks

import (
	"github.com/oliverhaas/djangogo/conf"
	"github.com/oliverhaas/djangogo/orm"
)

// Level is the severity of a Message. The values are Django's.
type Level int

// The levels of a message, as in django.core.checks.
const (
	LevelDebug    Level = 10
	LevelInfo     Level = 20
	LevelWarning  Level = 30
	LevelError    Level = 40
	LevelCritical Level = 50
)

// Message is one finding of a system check (django.core.checks.CheckMessage).
type Message struct {
	// Level is the severity.
	Level Level
	// Msg describes the problem.
	Msg string
	// Hint says how to fix it; empty when there is nothing to suggest.
	Hint string
	// Obj is what the message is about (a model, a field, an app), or nil for
	// a project-wide message.
	Obj any
	// ID identifies the message, for example "security.W004".
	ID string
}

// Context is what a check function inspects. It replaces the globals a Django
// check reads (settings, the app registry) and its app_configs and databases
// arguments.
type Context struct {
	// Settings is the project configuration.
	Settings *conf.Settings
	// Models is the model registry; nil when the project has no models.
	Models *orm.Registry
	// Apps restricts the run to the apps with these labels (Django's
	// app_configs); nil means every app.
	Apps []string
	// Databases lists the database aliases that checks may query (Django's
	// databases); nil means none.
	Databases []string
}

// Func is a check function: it inspects the context and returns its findings.
type Func func(ctx *Context) []Message

// Check is a check function with its registration data.
type Check struct {
	// Run is the check function.
	Run Func
	// Tags label the check, so a run can be limited to some tags.
	Tags []string
	// Deploy marks a check that runs only with the deployment checks
	// (check --deploy).
	Deploy bool
}
