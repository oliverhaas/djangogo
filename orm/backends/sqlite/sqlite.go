// Package sqlite provides an orm.Dialect implementation for SQLite using the
// pure-Go modernc.org/sqlite driver.
package sqlite

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/oliverhaas/djangogo/orm"

	_ "modernc.org/sqlite" // registers the "sqlite" driver
)

// Dialect implements orm.Dialect for SQLite (modernc.org/sqlite, pure Go).
type Dialect struct{}

// New returns the SQLite dialect.
func New() orm.Dialect { return Dialect{} }

// Name returns "sqlite".
func (Dialect) Name() string { return "sqlite" }

// Placeholder always returns "?" for SQLite positional bind parameters.
func (Dialect) Placeholder(int) string { return "?" }

// Quote returns the identifier surrounded by double quotes with any internal
// double quotes escaped by doubling them.
func (Dialect) Quote(ident string) string {
	return `"` + strings.ReplaceAll(ident, `"`, `""`) + `"`
}

// SupportsReturning reports whether the backend supports a RETURNING clause.
// SQLite supports it from 3.35.0, but for simplicity this backend returns false.
func (Dialect) SupportsReturning() bool { return false }

// ColumnType returns the SQL column definition for f, excluding the leading
// quoted column name.
//
// Rules:
//   - KindAuto -> "INTEGER PRIMARY KEY AUTOINCREMENT"
//   - Otherwise: base type + optional constraints (NOT NULL, PRIMARY KEY, UNIQUE)
func (d Dialect) ColumnType(f *orm.Field) string {
	if f.Kind == orm.KindAuto {
		return "INTEGER PRIMARY KEY AUTOINCREMENT"
	}

	var base string

	switch f.Kind {
	case orm.KindInt:
		base = "INTEGER"
	case orm.KindChar:
		base = fmt.Sprintf("VARCHAR(%d)", f.MaxLength)
	case orm.KindText:
		base = "TEXT"
	case orm.KindBool:
		base = "BOOLEAN"
	case orm.KindDateTime:
		base = "DATETIME"
	default:
		base = "TEXT"
	}

	var b strings.Builder

	b.WriteString(base)

	if !f.Null {
		b.WriteString(" NOT NULL")
	}

	if f.PrimaryKey {
		b.WriteString(" PRIMARY KEY")
	} else if f.Unique {
		b.WriteString(" UNIQUE")
	}

	return b.String()
}

// CreateTableSQL returns a CREATE TABLE statement for m in field-declaration order.
func (d Dialect) CreateTableSQL(m *orm.Model) string {
	fields := m.Fields()
	defs := make([]string, len(fields))

	for i, f := range fields {
		defs[i] = d.Quote(f.Column) + " " + d.ColumnType(f)
	}

	return "CREATE TABLE " + d.Quote(m.Table()) + " (" + strings.Join(defs, ", ") + ")"
}

// Open opens a SQLite database at dsn and returns the *sql.DB. For in-memory
// databases (dsn containing ":memory:") it pins the pool to a single connection
// so the in-memory database is shared across queries.
func Open(dsn string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	if strings.Contains(dsn, ":memory:") {
		db.SetMaxOpenConns(1)
	}
	return db, nil
}
