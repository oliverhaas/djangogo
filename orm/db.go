package orm

import "database/sql"

// DB is a handle bundling a *sql.DB, its Dialect, and the model Registry.
type DB struct {
	sqlDB    *sql.DB
	dialect  Dialect
	registry *Registry
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
