package djangotest

import (
	"context"
	"database/sql"
	"slices"
	"strings"

	"github.com/oliverhaas/djangogo/orm"
)

// schemaIndex is one index of a table as the database reports it. A unique
// constraint appears as a unique index on both dialects.
type schemaIndex struct {
	name    string
	columns []string
	unique  bool
}

// schemaFK is one single-column foreign key of a table.
type schemaFK struct {
	column   string
	toTable  string
	toColumn string
}

// isPostgres reports whether db speaks PostgreSQL. The introspection queries
// differ per dialect.
func isPostgres(db *orm.DB) bool { return db.Dialect().Name() == "postgres" }

// schemaRows runs an introspection query and passes every row to scan.
func schemaRows(t TB, db *orm.DB, query string, args []any, scan func(*sql.Rows) error) {
	t.Helper()
	rows, err := db.SQL().QueryContext(context.Background(), query, args...)
	if err != nil {
		t.Fatalf("djangotest: introspection: %v", err)
		return
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		if err := scan(rows); err != nil {
			t.Fatalf("djangotest: introspection: %v", err)
			return
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("djangotest: introspection: %v", err)
	}
}

// schemaTableExists reports whether the table exists. PostgreSQL looks only at
// the current schema, which is the private one of a test database.
func schemaTableExists(t TB, db *orm.DB, table string) bool {
	t.Helper()
	query := `SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?`
	if isPostgres(db) {
		query = `SELECT count(*) FROM information_schema.tables ` +
			`WHERE table_schema = current_schema() AND table_name = $1`
	}
	var n int
	schemaRows(t, db, query, []any{table}, func(rows *sql.Rows) error { return rows.Scan(&n) })
	return n > 0
}

// schemaColumns returns the columns of a table mapped to whether they accept
// NULL.
func schemaColumns(t TB, db *orm.DB, table string) map[string]bool {
	t.Helper()
	cols := map[string]bool{}
	if isPostgres(db) {
		query := `SELECT column_name, is_nullable FROM information_schema.columns ` +
			`WHERE table_schema = current_schema() AND table_name = $1`
		schemaRows(t, db, query, []any{table}, func(rows *sql.Rows) error {
			var name, nullable string
			if err := rows.Scan(&name, &nullable); err != nil {
				return err
			}
			cols[name] = nullable == "YES"
			return nil
		})
		return cols
	}
	query := `SELECT name, "notnull", pk FROM pragma_table_info(?)`
	schemaRows(t, db, query, []any{table}, func(rows *sql.Rows) error {
		var name string
		var notNull, pk int
		if err := rows.Scan(&name, &notNull, &pk); err != nil {
			return err
		}
		// SQLite reports an INTEGER PRIMARY KEY as nullable although it never
		// holds NULL.
		cols[name] = notNull == 0 && pk == 0
		return nil
	})
	return cols
}

// schemaIndexes returns the indexes of a table, the ones that back primary-key
// and unique constraints included.
func schemaIndexes(t TB, db *orm.DB, table string) []schemaIndex {
	t.Helper()
	var out []schemaIndex
	if isPostgres(db) {
		query := `SELECT i.relname, ix.indisunique, array_to_string(ARRAY(` +
			`SELECT a.attname FROM unnest(ix.indkey) WITH ORDINALITY AS k(attnum, ord) ` +
			`JOIN pg_attribute a ON a.attrelid = t.oid AND a.attnum = k.attnum ORDER BY k.ord), ',') ` +
			`FROM pg_index ix ` +
			`JOIN pg_class i ON i.oid = ix.indexrelid ` +
			`JOIN pg_class t ON t.oid = ix.indrelid ` +
			`JOIN pg_namespace n ON n.oid = t.relnamespace ` +
			`WHERE n.nspname = current_schema() AND t.relname = $1 ORDER BY i.relname`
		schemaRows(t, db, query, []any{table}, func(rows *sql.Rows) error {
			var idx schemaIndex
			var cols string
			if err := rows.Scan(&idx.name, &idx.unique, &cols); err != nil {
				return err
			}
			idx.columns = strings.Split(cols, ",")
			out = append(out, idx)
			return nil
		})
		return out
	}
	schemaRows(t, db, `SELECT name, "unique" FROM pragma_index_list(?)`, []any{table}, func(rows *sql.Rows) error {
		var idx schemaIndex
		var unique int
		if err := rows.Scan(&idx.name, &unique); err != nil {
			return err
		}
		idx.unique = unique != 0
		out = append(out, idx)
		return nil
	})
	for i := range out {
		query := `SELECT name FROM pragma_index_info(?) ORDER BY seqno`
		schemaRows(t, db, query, []any{out[i].name}, func(rows *sql.Rows) error {
			var col sql.NullString
			if err := rows.Scan(&col); err != nil {
				return err
			}
			out[i].columns = append(out[i].columns, col.String)
			return nil
		})
	}
	return out
}

// schemaForeignKeys returns the foreign keys of a table.
func schemaForeignKeys(t TB, db *orm.DB, table string) []schemaFK {
	t.Helper()
	var out []schemaFK
	query := `SELECT "from", "table", "to" FROM pragma_foreign_key_list(?)`
	if isPostgres(db) {
		query = `SELECT a.attname, ft.relname, fa.attname FROM pg_constraint c ` +
			`JOIN pg_class t ON t.oid = c.conrelid ` +
			`JOIN pg_namespace n ON n.oid = t.relnamespace ` +
			`JOIN pg_class ft ON ft.oid = c.confrelid ` +
			`JOIN pg_attribute a ON a.attrelid = c.conrelid AND a.attnum = c.conkey[1] ` +
			`JOIN pg_attribute fa ON fa.attrelid = c.confrelid AND fa.attnum = c.confkey[1] ` +
			`WHERE c.contype = 'f' AND n.nspname = current_schema() AND t.relname = $1`
	}
	schemaRows(t, db, query, []any{table}, func(rows *sql.Rows) error {
		var fk schemaFK
		var to sql.NullString
		if err := rows.Scan(&fk.column, &fk.toTable, &to); err != nil {
			return err
		}
		fk.toColumn = to.String
		out = append(out, fk)
		return nil
	})
	return out
}

// AssertTableExists asserts that the database has the table (Django's
// assertTableExists).
func AssertTableExists(t TB, db *orm.DB, table string) {
	t.Helper()
	if !schemaTableExists(t, db, table) {
		t.Errorf("table %q does not exist", table)
	}
}

// AssertTableNotExists asserts that the database has no such table (Django's
// assertTableNotExists).
func AssertTableNotExists(t TB, db *orm.DB, table string) {
	t.Helper()
	if schemaTableExists(t, db, table) {
		t.Errorf("table %q exists", table)
	}
}

// AssertColumnExists asserts that the table has the column (Django's
// assertColumnExists).
func AssertColumnExists(t TB, db *orm.DB, table, column string) {
	t.Helper()
	if _, ok := schemaColumns(t, db, table)[column]; !ok {
		t.Errorf("column %q does not exist in table %q", column, table)
	}
}

// AssertColumnNotExists asserts that the table has no such column (Django's
// assertColumnNotExists).
func AssertColumnNotExists(t TB, db *orm.DB, table, column string) {
	t.Helper()
	if _, ok := schemaColumns(t, db, table)[column]; ok {
		t.Errorf("column %q exists in table %q", column, table)
	}
}

// AssertColumnNull asserts that the column accepts NULL (Django's
// assertColumnNull).
func AssertColumnNull(t TB, db *orm.DB, table, column string) {
	t.Helper()
	null, ok := schemaColumns(t, db, table)[column]
	switch {
	case !ok:
		t.Errorf("column %q does not exist in table %q", column, table)
	case !null:
		t.Errorf("column %q of table %q is NOT NULL", column, table)
	}
}

// AssertColumnNotNull asserts that the column rejects NULL (Django's
// assertColumnNotNull).
func AssertColumnNotNull(t TB, db *orm.DB, table, column string) {
	t.Helper()
	null, ok := schemaColumns(t, db, table)[column]
	switch {
	case !ok:
		t.Errorf("column %q does not exist in table %q", column, table)
	case null:
		t.Errorf("column %q of table %q accepts NULL", column, table)
	}
}

// hasIndexOn reports whether the table has an index over exactly the given
// columns, in order. With uniqueOnly set only unique indexes count.
func hasIndexOn(t TB, db *orm.DB, table string, columns []string, uniqueOnly bool) bool {
	t.Helper()
	for _, idx := range schemaIndexes(t, db, table) {
		if slices.Equal(idx.columns, columns) && (idx.unique || !uniqueOnly) {
			return true
		}
	}
	return false
}

// AssertIndexExists asserts that the table has an index over exactly the given
// columns, in order (Django's assertIndexExists). The index of a unique
// constraint counts.
func AssertIndexExists(t TB, db *orm.DB, table string, columns ...string) {
	t.Helper()
	if !hasIndexOn(t, db, table, columns, false) {
		t.Errorf("table %q has no index on (%s)", table, strings.Join(columns, ", "))
	}
}

// AssertIndexNotExists asserts that the table has no index over exactly the
// given columns (Django's assertIndexNotExists).
func AssertIndexNotExists(t TB, db *orm.DB, table string, columns ...string) {
	t.Helper()
	if hasIndexOn(t, db, table, columns, false) {
		t.Errorf("table %q has an index on (%s)", table, strings.Join(columns, ", "))
	}
}

// hasIndexNamed reports whether the table has an index of that name.
func hasIndexNamed(t TB, db *orm.DB, table, name string) bool {
	t.Helper()
	return slices.ContainsFunc(schemaIndexes(t, db, table), func(idx schemaIndex) bool { return idx.name == name })
}

// AssertIndexNameExists asserts that the table has an index of that name
// (Django's assertIndexNameExists).
func AssertIndexNameExists(t TB, db *orm.DB, table, name string) {
	t.Helper()
	if !hasIndexNamed(t, db, table, name) {
		t.Errorf("table %q has no index named %q", table, name)
	}
}

// AssertIndexNameNotExists asserts that the table has no index of that name
// (Django's assertIndexNameNotExists).
func AssertIndexNameNotExists(t TB, db *orm.DB, table, name string) {
	t.Helper()
	if hasIndexNamed(t, db, table, name) {
		t.Errorf("table %q has an index named %q", table, name)
	}
}

// AssertUniqueConstraintExists asserts that exactly the given columns are
// unique together, in order (Django's assertUniqueConstraintExists).
func AssertUniqueConstraintExists(t TB, db *orm.DB, table string, columns ...string) {
	t.Helper()
	if !hasIndexOn(t, db, table, columns, true) {
		t.Errorf("table %q has no unique constraint on (%s)", table, strings.Join(columns, ", "))
	}
}

// AssertUniqueConstraintNotExists asserts that the given columns are not
// unique together (Django's assertUniqueConstraintExists with value=False).
func AssertUniqueConstraintNotExists(t TB, db *orm.DB, table string, columns ...string) {
	t.Helper()
	if hasIndexOn(t, db, table, columns, true) {
		t.Errorf("table %q has a unique constraint on (%s)", table, strings.Join(columns, ", "))
	}
}

// hasFK reports whether column references toTable(toColumn). SQLite leaves
// the target column empty when the key points at the primary key implicitly;
// that matches any toColumn.
func hasFK(t TB, db *orm.DB, table, column, toTable, toColumn string) bool {
	t.Helper()
	for _, fk := range schemaForeignKeys(t, db, table) {
		if fk.column == column && fk.toTable == toTable && (fk.toColumn == toColumn || fk.toColumn == "") {
			return true
		}
	}
	return false
}

// AssertFKExists asserts that column of table is a foreign key to
// toTable(toColumn) (Django's assertFKExists).
func AssertFKExists(t TB, db *orm.DB, table, column, toTable, toColumn string) {
	t.Helper()
	if !hasFK(t, db, table, column, toTable, toColumn) {
		t.Errorf("table %q has no foreign key %s -> %s(%s)", table, column, toTable, toColumn)
	}
}

// AssertFKNotExists asserts that column of table is no foreign key to
// toTable(toColumn) (Django's assertFKNotExists).
func AssertFKNotExists(t TB, db *orm.DB, table, column, toTable, toColumn string) {
	t.Helper()
	if hasFK(t, db, table, column, toTable, toColumn) {
		t.Errorf("table %q has a foreign key %s -> %s(%s)", table, column, toTable, toColumn)
	}
}
