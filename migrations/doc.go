// Package migrations ports Django's migration framework: per-app migration
// histories, a dependency graph, schema operations with a forward and a
// backward step, an executor that applies or unapplies them atomically, and an
// autodetector that writes new migrations as editable Go source.
//
// A migration belongs to an app and has a name ("0001_initial"). Its
// Dependencies name the migrations that run before it, also across apps. A
// generated migration file registers its migration from init, so a blank
// import of the package makes it known:
//
//	import _ "example.com/project/migrations/blog"
//
// The Loader builds the Graph from a Registry and reads the applied
// migrations through the Recorder (table "django_migrations"). The Executor
// plans a forward or backward run and executes it. The Autodetector compares
// the state the migrations produce with the state of the registered models
// and returns the missing migrations; RenderMigration and WriteMigration turn
// them into Go files.
package migrations
