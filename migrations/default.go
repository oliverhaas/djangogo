package migrations

// DefaultRegistry is the process-global registry that generated migration files
// register themselves into via init() calling Register.
var DefaultRegistry = NewRegistry()

// Register adds m to the DefaultRegistry. Generated migration files call this in init().
func Register(m Migration) { DefaultRegistry.Add(m) }
