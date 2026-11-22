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
func (d Dialect) ColumnType(f *orm.Field) string {
	if f.Kind == orm.KindAuto {
		return "BIGSERIAL PRIMARY KEY"
	}

	var base, check string

	switch f.Kind {
	case orm.KindInt:
		base = "BIGINT"
	case orm.KindSmallInt:
		base = "SMALLINT"
	case orm.KindBigInt:
		base = "BIGINT"
	case orm.KindPositiveSmallInt:
		base = "SMALLINT"
		if f.Column != "" {
			check = "CHECK (" + d.Quote(f.Column) + " >= 0)"
		}
	case orm.KindPositiveInt:
		base = "INTEGER"
		if f.Column != "" {
			check = "CHECK (" + d.Quote(f.Column) + " >= 0)"
		}
	case orm.KindPositiveBigInt:
		base = "BIGINT"
		if f.Column != "" {
			check = "CHECK (" + d.Quote(f.Column) + " >= 0)"
		}
	case orm.KindFloat:
		base = "DOUBLE PRECISION"
	case orm.KindDecimal:
		if f.MaxDigits > 0 {
			base = fmt.Sprintf("NUMERIC(%d, %d)", f.MaxDigits, f.DecimalPlaces)
		} else {
			base = "NUMERIC"
		}
	case orm.KindChar, orm.KindSlug, orm.KindEmail, orm.KindURL:
		base = fmt.Sprintf("VARCHAR(%d)", f.MaxLength)
	case orm.KindText:
		base = "TEXT"
	case orm.KindBool:
		base = "BOOLEAN"
	case orm.KindDateTime:
		base = "TIMESTAMPTZ"
	case orm.KindDate:
		base = "DATE"
	case orm.KindTime:
		base = "TIME"
	case orm.KindDuration:
		base = "INTERVAL"
	case orm.KindUUID:
		base = "UUID"
	case orm.KindJSON:
		base = "JSONB"
	case orm.KindBinary:
		base = "BYTEA"
	case orm.KindGenericIPAddress:
		base = "INET"
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

	if check != "" {
		b.WriteByte(' ')
		b.WriteString(check)
	}

	return b.String()
}

// CreateTableSQL returns a CREATE TABLE statement for m in field-declaration
// order. Each resolved foreign-key field also emits a table-level FOREIGN KEY
// constraint after the column definitions.
func (d Dialect) CreateTableSQL(m *orm.Model) string {
	return d.createTableSQL(m, true)
}

// CreateTableNoForeignKeysSQL returns CreateTableSQL without the FOREIGN KEY
// constraints, which AddForeignKeySQL adds once the referenced tables exist:
// PostgreSQL refuses a reference to a table that does not exist yet.
func (d Dialect) CreateTableNoForeignKeysSQL(m *orm.Model) string {
	return d.createTableSQL(m, false)
}

// AddForeignKeySQL returns the ALTER TABLE statement that adds the FOREIGN KEY
// constraint of the field f of m. The constraint gets the name PostgreSQL
// gives the one of CreateTableSQL.
func (d Dialect) AddForeignKeySQL(m *orm.Model, f *orm.Field) string {
	return "ALTER TABLE " + d.Quote(m.Table()) + " ADD " + d.foreignKey(f)
}

// createTableSQL returns the CREATE TABLE statement of m, with the FOREIGN
// KEY constraints when foreignKeys is true.
func (d Dialect) createTableSQL(m *orm.Model, foreignKeys bool) string {
	fields := m.Fields()
	defs := make([]string, 0, len(fields))

	for _, f := range fields {
		defs = append(defs, d.Quote(f.Column)+" "+d.ColumnType(f))
	}
	for _, f := range fields {
		if foreignKeys && f.Rel != nil && f.Rel.Target != nil {
			defs = append(defs, d.foreignKey(f))
		}
	}

	return "CREATE TABLE " + d.Quote(m.Table()) + " (" + strings.Join(defs, ", ") + ")"
}

// foreignKey returns the FOREIGN KEY constraint of a field with a resolved
// relation.
func (d Dialect) foreignKey(f *orm.Field) string {
	target := f.Rel.Target
	return "FOREIGN KEY (" + d.Quote(f.Column) + ") REFERENCES " +
		d.Quote(target.Table()) + " (" + d.Quote(target.PrimaryKey().Column) + ")" +
		f.Rel.OnDelete.Clause() + orm.DeferredClause
}

// Open opens a PostgreSQL database at dsn using the pgx stdlib driver and
// returns the *sql.DB.
func Open(dsn string) (*sql.DB, error) {
	return sql.Open("pgx", dsn)
}
