package orm

// Dialect abstracts the SQL differences between database backends.
type Dialect interface {
	Name() string               // e.g. "sqlite"
	Placeholder(n int) string   // bind-parameter for the n-th arg (1-based); sqlite ignores n and returns "?"
	Quote(ident string) string  // quote an identifier, e.g. "title"
	ColumnType(f *Field) string // full column definition minus the name, e.g. `VARCHAR(100) NOT NULL UNIQUE`
	CreateTableSQL(m *Model) string
	SupportsReturning() bool
}
