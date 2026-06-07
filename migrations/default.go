// Package migrations -- cross-process loading note:
// Generated migration files call Register in their init() function, which adds
// them to DefaultRegistry. A fresh process only sees migrations whose packages
// are compiled into the binary and therefore imported. After makemigrations
// writes a new file, the binary must be rebuilt before a separate migrate
// invocation sees it. Within a single process, makemigrations also adds the
// migration directly to the running Application.Migrations, so an in-process
// flow works without a rebuild.
package migrations

// DefaultRegistry is the process-global registry that generated migration files
// register themselves into via init() calling Register.
var DefaultRegistry = NewRegistry()

// Register adds m to the DefaultRegistry. Generated migration files call this in init().
func Register(m Migration) { DefaultRegistry.Add(m) }
