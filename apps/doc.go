// Package apps provides the application registry, the counterpart of
// django.apps (AppConfig and the apps registry).
//
// # Mapping from Django
//
// An installed app is a Go value that implements Config. What an AppConfig
// subclass sets as attributes or ships as modules and directories, a Go app
// declares by implementing optional interfaces, which the registry discovers
// by type assertion:
//
//	AppConfig.name                    Config.Name
//	AppConfig.label                   Labeler.Label, default Label(c)
//	AppConfig.verbose_name            VerboseNamer.VerboseName, default VerboseName(c)
//	AppConfig.ready()                 Initializer.Ready
//	models.py                         ModelProvider.Models
//	urls.py                           URLProvider.URLs
//	management/commands/              CommandProvider.Commands
//	templates/                        TemplateProvider.Templates
//	static/                           StaticProvider.Static
//	checks registered in ready()      CheckProvider.Checks
//	apps.get_app_configs()            Registry.Configs
//	apps.get_app_config(label)        Registry.ByLabel
//	apps.is_installed(name)           Registry.IsInstalled
//	apps.ready                        Registry.IsReady
//
// The order in which the apps are registered is the INSTALLED_APPS order. It
// decides precedence exactly as in Django: the templates and static files of
// an app listed first are found first, and its management commands override
// those of an app listed later.
//
// # Differences from Django
//
// Django imports the modules of an app and has no explicit dependencies
// between apps. An app here may declare them (DependencyProvider) and may ask
// for framework services before its routes are collected (Wirer). The
// registry therefore starts the apps in three passes:
//
//  1. Register: every config is registered and validated, then
//     Registry.Resolve checks the declared dependencies and computes the start
//     order (dependencies first, registration order otherwise).
//  2. Wire: Registry.Wire hands every Wirer the Context, in start order.
//  3. Ready: Registry.Ready runs every Initializer, in start order.
//
// A missing dependency, a dependency cycle, a duplicate label or name and an
// invalid label are errors of type *conf.ImproperlyConfigured.
//
// The registry is built by one goroutine at startup. After Ready it does not
// change, so reading it from several goroutines is safe.
package apps
