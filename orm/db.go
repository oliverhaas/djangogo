package orm

import (
	"database/sql"
	"time"
)

// DB is a handle bundling a *sql.DB, its Dialect, and the model Registry.
type DB struct {
	sqlDB    *sql.DB
	dialect  Dialect
	registry *Registry
	// Now overrides the clock used for auto_now / auto_now_add timestamps. When
	// nil, now() returns time.Now().UTC(). It is a per-handle field (not a global)
	// so parallel tests can each set a deterministic clock without racing.
	Now func() time.Time
}

// NewDB returns a DB that pairs the given *sql.DB with a Dialect and Registry.
func NewDB(sqlDB *sql.DB, d Dialect, r *Registry) *DB {
	return &DB{sqlDB: sqlDB, dialect: d, registry: r}
}

// Dialect returns the DB's SQL dialect.
func (db *DB) Dialect() Dialect { return db.dialect }

// Registry returns the DB's model registry.
func (db *DB) Registry() *Registry { return db.registry }

// SQL returns the underlying *sql.DB handle.
func (db *DB) SQL() *sql.DB { return db.sqlDB }

// now returns the DB's clock time, defaulting to the current UTC time. Django
// stores aware UTC timestamps under USE_TZ=True; defaulting to UTC also keeps
// SQLite (which has no time zone) deterministic.
func (db *DB) now() time.Time {
	if db.Now != nil {
		return db.Now()
	}
	return time.Now().UTC()
}
