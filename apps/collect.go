package apps

import (
	"io/fs"
	"slices"

	"github.com/oliverhaas/djangogo/checks"
	"github.com/oliverhaas/djangogo/manage"
	"github.com/oliverhaas/djangogo/urls"
)

// The collectors below ask the apps on every call and keep no result, so a
// call after Ready changes nothing in the registry.

// AppFS is the filesystem of one app.
type AppFS struct {
	// App is the label of the app.
	App string
	// FS is the tree the app contributes.
	FS fs.FS
}

// AppCommand is a command with the label of the app it comes from.
type AppCommand struct {
	// App is the label of the app.
	App string
	// Command is the management command.
	Command manage.Command
}

// AppModels holds the models of one app with the label to register them
// under (Django's app_label).
type AppModels struct {
	// App is the label of the app.
	App string
	// Models holds pointers to zero-valued model structs.
	Models []any
}

// TemplateFS returns the template trees of the apps that have one, in
// INSTALLED_APPS order, which is the order Django's app directories loader
// searches them in. A nil tree is skipped.
func (r *Registry) TemplateFS() []AppFS {
	var out []AppFS
	for _, e := range r.entries {
		provider, ok := e.config.(TemplateProvider)
		if !ok {
			continue
		}
		if fsys := provider.Templates(); fsys != nil {
			out = append(out, AppFS{App: e.label, FS: fsys})
		}
	}
	return out
}

// StaticFS returns the static trees of the apps that have one, in
// INSTALLED_APPS order, which is the order Django's AppDirectoriesFinder
// searches them in. A nil tree is skipped.
func (r *Registry) StaticFS() []AppFS {
	var out []AppFS
	for _, e := range r.entries {
		provider, ok := e.config.(StaticProvider)
		if !ok {
			continue
		}
		if fsys := provider.Static(); fsys != nil {
			out = append(out, AppFS{App: e.label, FS: fsys})
		}
	}
	return out
}

// Commands returns every app command in the order to REGISTER them so that
// Django's precedence holds when a later registration replaces an earlier one:
// the apps in REVERSE installed order (an app listed first overrides the
// command of an app listed later, see django.core.management.get_commands).
// The commands of one app keep the order the app gives them in; a nil command
// is skipped.
func (r *Registry) Commands() []AppCommand {
	var out []AppCommand
	for _, e := range slices.Backward(r.entries) {
		provider, ok := e.config.(CommandProvider)
		if !ok {
			continue
		}
		for _, command := range provider.Commands() {
			if command != nil {
				out = append(out, AppCommand{App: e.label, Command: command})
			}
		}
	}
	return out
}

// Checks returns the checks of every CheckProvider, in INSTALLED_APPS order.
func (r *Registry) Checks() []checks.Check {
	var out []checks.Check
	for _, e := range r.entries {
		if provider, ok := e.config.(CheckProvider); ok {
			out = append(out, provider.Checks()...)
		}
	}
	return out
}

// Models returns the model pointers of every ModelProvider with the label to
// register them under, in start order (so a model's relation targets in a
// dependency are registered first). Before Resolve the start order is the
// registration order. A ModelProvider without models has an entry too.
func (r *Registry) Models() []AppModels {
	var out []AppModels
	for _, e := range r.inStartOrder() {
		if provider, ok := e.config.(ModelProvider); ok {
			out = append(out, AppModels{App: e.label, Models: provider.Models()})
		}
	}
	return out
}

// Routes returns the routes of every URLProvider in installed order.
func (r *Registry) Routes() []urls.Route {
	var out []urls.Route
	for _, e := range r.entries {
		if provider, ok := e.config.(URLProvider); ok {
			out = append(out, provider.URLs()...)
		}
	}
	return out
}
