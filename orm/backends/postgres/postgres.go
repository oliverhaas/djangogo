// Package postgres provides an orm.Dialect implementation for PostgreSQL using
// the pgx driver via its database/sql stdlib adapter.
package postgres

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"

	"github.com/oliverhaas/djangogo/orm"

	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" database/sql driver
)

// Dialect implements orm.Dialect for PostgreSQL (pgx stdlib adapter).
type Dialect struct{}

// New returns the PostgreSQL dialect.
func New() orm.Dialect { return Dialect{} }

// Name returns "postgres".
func (Dialect) Name() string { return "postgres" }

// Placeholder returns the n-th positional bind parameter, e.g. "$1" for n == 1.
func (Dialect) Placeholder(n int) string { return "$" + strconv.Itoa(n) }

// Quote returns the identifier surrounded by double quotes with any internal
// double quotes escaped by doubling them.
func (Dialect) Quote(ident string) string {
	return `"` + strings.ReplaceAll(ident, `"`, `""`) + `"`
}

// SupportsReturning reports whether the backend supports a RETURNING clause.
// PostgreSQL supports it, so this returns true.
func (Dialect) SupportsReturning() bool { return true }

// ColumnType returns the SQL column definition for f, excluding the leading
// quoted column name.
//
// Rules:
//   - KindAuto -> "BIGSERIAL PRIMARY KEY"
//   - Otherwise: base type + optional constraints (NOT NULL, PRIMARY KEY, UNIQUE)
func (Dialect) ColumnType(f *orm.Field) string {
	if f.Kind == orm.KindAuto {
		return "BIGSERIAL PRIMARY KEY"
	}

	var base string

	switch f.Kind {
	case orm.KindInt:
		base = "BIGINT"
	case orm.KindChar:
		base = fmt.Sprintf("VARCHAR(%d)", f.MaxLength)
	case orm.KindText:
		base = "TEXT"
	case orm.KindBool:
		base = "BOOLEAN"
	case orm.KindDateTime:
		base = "TIMESTAMPTZ"
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

// Open opens a PostgreSQL database at dsn using the pgx stdlib driver and
// returns the *sql.DB.
func Open(dsn string) (*sql.DB, error) {
	return sql.Open("pgx", dsn)
}
