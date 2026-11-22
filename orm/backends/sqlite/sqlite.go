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
// SQLite has it since 3.35.0, which the bundled engine exceeds; a multi-row
// INSERT relies on it to hand back the generated primary keys.
func (Dialect) SupportsReturning() bool { return true }

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

	var base, check string

	switch f.Kind {
	case orm.KindInt:
		base = "INTEGER"
	case orm.KindSmallInt:
		base = "SMALLINT"
	case orm.KindBigInt:
		base = "BIGINT"
	case orm.KindPositiveSmallInt:
		base = "SMALLINT UNSIGNED"
		if f.Column != "" {
			check = "CHECK (" + d.Quote(f.Column) + " >= 0)"
		}
	case orm.KindPositiveInt:
		base = "INTEGER UNSIGNED"
		if f.Column != "" {
			check = "CHECK (" + d.Quote(f.Column) + " >= 0)"
		}
	case orm.KindPositiveBigInt:
		base = "BIGINT UNSIGNED"
		if f.Column != "" {
			check = "CHECK (" + d.Quote(f.Column) + " >= 0)"
		}
	case orm.KindFloat:
		base = "REAL"
	case orm.KindDecimal:
		base = "DECIMAL"
	case orm.KindChar, orm.KindSlug, orm.KindEmail, orm.KindURL:
		base = fmt.Sprintf("VARCHAR(%d)", f.MaxLength)
	case orm.KindText:
		base = "TEXT"
	case orm.KindBool:
		base = "BOOLEAN"
	case orm.KindDateTime:
		base = "DATETIME"
	case orm.KindDate:
		base = "DATE"
	case orm.KindTime:
		base = "TIME"
	case orm.KindDuration:
		base = "BIGINT"
	case orm.KindUUID:
		base = "CHAR(32)"
	case orm.KindJSON:
		base = "TEXT"
		if f.Column != "" {
			q := d.Quote(f.Column)
			check = "CHECK (JSON_VALID(" + q + ") OR " + q + " IS NULL)"
		}
	case orm.KindBinary:
		base = "BLOB"
	case orm.KindGenericIPAddress:
		base = "CHAR(39)"
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
	fields := m.Fields()
	defs := make([]string, 0, len(fields))

	for _, f := range fields {
		defs = append(defs, d.Quote(f.Column)+" "+d.ColumnType(f))
	}
	for _, f := range fields {
		if f.Rel != nil && f.Rel.Target != nil {
			target := f.Rel.Target
			defs = append(defs, "FOREIGN KEY ("+d.Quote(f.Column)+") REFERENCES "+
				d.Quote(target.Table())+" ("+d.Quote(target.PrimaryKey().Column)+")"+
				f.Rel.OnDelete.Clause()+orm.DeferredClause)
		}
	}

	return "CREATE TABLE " + d.Quote(m.Table()) + " (" + strings.Join(defs, ", ") + ")"
}

// Open opens a SQLite database at dsn and returns the *sql.DB. Every connection
// of the pool enforces foreign keys, as with Django's SQLite backend. For in-memory
// databases it pins the pool to a single connection so the in-memory database is
// shared across queries. This covers both the ":memory:" form and the named
// shared form "file:name?mode=memory&cache=shared" (dsn containing "mode=memory").
func Open(dsn string) (*sql.DB, error) {
	if functionRegistration != nil {
		return nil, fmt.Errorf("sqlite: register SQL functions: %w", functionRegistration)
	}
	db, err := sql.Open("sqlite", withForeignKeys(dsn))
	if err != nil {
		return nil, err
	}
	if strings.Contains(dsn, ":memory:") || strings.Contains(dsn, "mode=memory") {
		db.SetMaxOpenConns(1)
	}
	return db, nil
}

// withForeignKeys adds the foreign_keys pragma to the query of dsn, so the
// driver runs it on each new connection of this pool. The DSN keeps the
// setting local to the pool, where a driver connection hook would also change
// the SQLite connections that the program opens elsewhere. A DSN that already
// names foreign_keys is left as it is, so a caller can turn enforcement off.
func withForeignKeys(dsn string) string {
	if strings.Contains(dsn, "foreign_keys") {
		return dsn
	}
	sep := "?"
	if strings.Contains(dsn, "?") {
		sep = "&"
	}
	return dsn + sep + "_pragma=foreign_keys(1)"
}
