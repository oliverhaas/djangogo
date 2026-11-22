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

// Features describes the optional SQL capabilities of a dialect. The zero
// value is plain standard SQL.
type Features struct {
	// NoLimitValue is the LIMIT a statement needs when it has an OFFSET but no
	// limit: "-1" on SQLite, which cannot write OFFSET alone. Empty means that
	// OFFSET can stand alone.
	NoLimitValue string
	// MaxQueryParams caps the bound parameters of one statement; BulkCreate
	// splits its INSERT to stay below it. Zero means no cap.
	MaxQueryParams int
	// SelectForUpdate reports that SELECT ... FOR UPDATE is supported. On a
	// dialect without it SelectForUpdate is a no-op.
	SelectForUpdate bool
	// UniqueNullsNotDistinct reports that a unique index can treat NULL
	// values as equal (NULLS NOT DISTINCT, PostgreSQL 15).
	UniqueNullsNotDistinct bool
}

// FeatureDialect is an optional capability of a Dialect: it reports the
// dialect's Features. A dialect that does not implement it gets the zero
// Features.
type FeatureDialect interface {
	Features() Features
}

// ForeignKeyDialect is an optional capability of a Dialect whose CREATE TABLE
// cannot refer to a table that does not exist yet, as PostgreSQL's cannot.
// DB.CreateTables creates such a table without its foreign keys when it
// refers to a table created later, and adds them once every table exists.
type ForeignKeyDialect interface {
	// CreateTableNoForeignKeysSQL returns CreateTableSQL without the
	// FOREIGN KEY constraints.
	CreateTableNoForeignKeysSQL(m *Model) string
	// AddForeignKeySQL returns the statement that adds the FOREIGN KEY
	// constraint of the field f of m.
	AddForeignKeySQL(m *Model, f *Field) string
}

// LiteralDialect is an optional capability of a Dialect that cannot tell the
// type of a bound parameter where the statement gives no hint, as in
// "SELECT MAX($1)". LiteralSQL returns the SQL for the placeholder of a
// literal of an expression (orm.Value, or the number in F("n").Add(1)), which
// the dialect may wrap in a cast that fits the Go type of v.
type LiteralDialect interface {
	LiteralSQL(placeholder string, v any) string
}

// dialectFeatures returns the features of d.
func dialectFeatures(d Dialect) Features {
	if fd, ok := d.(FeatureDialect); ok {
		return fd.Features()
	}
	return Features{}
}
