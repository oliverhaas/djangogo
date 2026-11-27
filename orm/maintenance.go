package orm

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// Flush deletes every row of the named tables and restarts their auto primary
// keys, Django's sql_flush with reset_sequences that the flush command runs.
// On PostgreSQL it runs one TRUNCATE ... RESTART IDENTITY statement. On SQLite
// it empties each table with DELETE and resets the rows of the tables in
// sqlite_sequence. Run it inside Atomic, so that the foreign keys between the
// tables are checked once all of them are empty.
func Flush(ctx context.Context, db *DB, tables ...string) error {
	if len(tables) == 0 {
		return nil
	}
	d := db.dialect
	var stmts []string
	if d.Name() == "postgres" {
		quoted := make([]string, len(tables))
		for i, t := range tables {
			quoted[i] = d.Quote(t)
		}
		stmts = append(stmts, "TRUNCATE "+strings.Join(quoted, ", ")+" RESTART IDENTITY")
	} else {
		names := make([]string, len(tables))
		for i, t := range tables {
			stmts = append(stmts, "DELETE FROM "+d.Quote(t))
			names[i] = "'" + strings.ReplaceAll(t, "'", "''") + "'"
		}
		if d.Name() == "sqlite" {
			exists, err := sqliteSequenceExists(ctx, db)
			if err != nil {
				return err
			}
			if exists {
				stmts = append(stmts, `UPDATE "sqlite_sequence" SET "seq" = 0 WHERE "name" IN (`+strings.Join(names, ", ")+")")
			}
		}
	}
	for _, stmt := range stmts {
		if _, err := db.conn(ctx).ExecContext(ctx, stmt); err != nil {
			markForRollbackOnError(ctx, err)
			return fmt.Errorf("orm: flush: %w", db.classify(err))
		}
	}
	return nil
}

// sqliteSequenceExists reports whether the SQLite database has the
// sqlite_sequence table, which SQLite creates with the first AUTOINCREMENT
// table.
func sqliteSequenceExists(ctx context.Context, db *DB) (bool, error) {
	var n int
	err := scanOne(ctx, db.conn(ctx), `SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = 'sqlite_sequence'`, nil, &n)
	if err != nil {
		return false, fmt.Errorf("orm: flush: %w", err)
	}
	return n > 0, nil
}

// ResetSequences sets the sequence of the auto primary key of each model to
// the largest key in its table, Django's sequence_reset_sql. loaddata runs it
// after it inserted rows with explicit keys, so that the next row that the
// database numbers does not reuse a key. It does nothing on SQLite, whose
// AUTOINCREMENT counter follows the inserted keys.
func ResetSequences(ctx context.Context, db *DB, models ...*Model) error {
	d := db.dialect
	if d.Name() != "postgres" {
		return nil
	}
	for _, m := range models {
		pk := m.PrimaryKey()
		if pk == nil || pk.Kind != KindAuto {
			continue
		}
		col := d.Quote(pk.Column)
		stmt := "SELECT setval(pg_get_serial_sequence('" + strings.ReplaceAll(d.Quote(m.Table()), "'", "''") + "','" +
			strings.ReplaceAll(pk.Column, "'", "''") + "'), coalesce(max(" + col + "), 1), max(" + col +
			") IS NOT null) FROM " + d.Quote(m.Table())
		if _, err := db.conn(ctx).ExecContext(ctx, stmt); err != nil {
			markForRollbackOnError(ctx, err)
			return fmt.Errorf("orm: reset sequence of %s: %w", m.Table(), err)
		}
	}
	return nil
}

// CheckForeignKeys reports a row of the named tables whose foreign key refers
// to a missing row, Django's connection.check_constraints(table_names). The
// foreign keys are checked when a transaction commits, so a transaction can
// insert rows that refer to each other in any order. loaddata runs
// CheckForeignKeys inside its transaction, before the commit, to report a
// broken reference with the fixture. On PostgreSQL it sets the constraints of
// the transaction immediate and back to deferred, which checks every table.
// On SQLite it runs PRAGMA foreign_key_check on each table and reports the
// first violation with Django's message. The error matches
// ErrForeignKeyViolation.
func CheckForeignKeys(ctx context.Context, db *DB, tables ...string) error {
	switch db.dialect.Name() {
	case "postgres":
		if _, err := db.conn(ctx).ExecContext(ctx, "SET CONSTRAINTS ALL IMMEDIATE"); err != nil {
			markForRollbackOnError(ctx, err)
			return db.classify(err)
		}
		if _, err := db.conn(ctx).ExecContext(ctx, "SET CONSTRAINTS ALL DEFERRED"); err != nil {
			markForRollbackOnError(ctx, err)
			return err
		}
		return nil
	case "sqlite":
		for _, table := range tables {
			if err := sqliteCheckForeignKeys(ctx, db, table); err != nil {
				return err
			}
		}
	}
	return nil
}

// sqliteCheckForeignKeys reports the first row of table that PRAGMA
// foreign_key_check finds, with the message of Django's SQLite backend.
func sqliteCheckForeignKeys(ctx context.Context, db *DB, table string) error {
	d := db.dialect
	conn := db.conn(ctx)
	rows, err := conn.QueryContext(ctx, "PRAGMA foreign_key_check("+d.Quote(table)+")")
	if err != nil {
		return fmt.Errorf("orm: check foreign keys of %s: %w", table, err)
	}
	type violation struct {
		rowid int64
		fkid  int
	}
	var found []violation
	for rows.Next() {
		var tbl, parent string
		var rowid *int64
		var fkid int
		if err := rows.Scan(&tbl, &rowid, &parent, &fkid); err != nil {
			_ = rows.Close()
			return fmt.Errorf("orm: check foreign keys of %s: %w", table, err)
		}
		if rowid != nil {
			found = append(found, violation{rowid: *rowid, fkid: fkid})
		}
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("orm: check foreign keys of %s: %w", table, err)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("orm: check foreign keys of %s: %w", table, err)
	}
	if len(found) == 0 {
		return nil
	}
	v := found[0]
	column, refTable, refColumn, err := sqliteForeignKey(ctx, db, table, v.fkid)
	if err != nil {
		return err
	}
	pkColumn, err := sqlitePrimaryKeyColumn(ctx, db, table)
	if err != nil {
		return err
	}
	var pkValue, bad any
	query := "SELECT " + d.Quote(pkColumn) + ", " + d.Quote(column) + " FROM " + d.Quote(table) + " WHERE rowid = ?"
	if err := scanOne(ctx, conn, query, []any{v.rowid}, &pkValue, &bad); err != nil {
		return fmt.Errorf("orm: check foreign keys of %s: %w", table, err)
	}
	msg := fmt.Sprintf("The row in table '%s' with primary key '%s' has an invalid foreign key: "+
		"%s.%s contains a value '%s' that does not have a corresponding value in %s.%s.",
		table, sqlText(pkValue), table, column, sqlText(bad), refTable, refColumn)
	return &IntegrityError{Kind: IntegrityForeignKey, Err: errors.New(msg)}
}

// sqliteForeignKey returns the column, the referenced table and the
// referenced column of the foreign key with the id fkid of table.
func sqliteForeignKey(ctx context.Context, db *DB, table string, fkid int) (column, refTable, refColumn string, err error) {
	rows, err := db.conn(ctx).QueryContext(ctx, "PRAGMA foreign_key_list("+db.dialect.Quote(table)+")")
	if err != nil {
		return "", "", "", fmt.Errorf("orm: foreign keys of %s: %w", table, err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id, seq int
		var refTbl, from string
		var to *string
		var onUpdate, onDelete, match string
		if err := rows.Scan(&id, &seq, &refTbl, &from, &to, &onUpdate, &onDelete, &match); err != nil {
			return "", "", "", fmt.Errorf("orm: foreign keys of %s: %w", table, err)
		}
		if id == fkid {
			refCol := ""
			if to != nil {
				refCol = *to
			}
			return from, refTbl, refCol, nil
		}
	}
	if err := rows.Err(); err != nil {
		return "", "", "", fmt.Errorf("orm: foreign keys of %s: %w", table, err)
	}
	return "", "", "", fmt.Errorf("orm: table %s has no foreign key %d", table, fkid)
}

// sqlitePrimaryKeyColumn returns the primary key column of table.
func sqlitePrimaryKeyColumn(ctx context.Context, db *DB, table string) (string, error) {
	var name string
	query := "SELECT name FROM pragma_table_info(?) WHERE pk = 1"
	if err := scanOne(ctx, db.conn(ctx), query, []any{table}, &name); err != nil {
		return "", fmt.Errorf("orm: primary key of %s: %w", table, err)
	}
	return name, nil
}

// sqlText returns a value read from SQLite as Python's str() writes it.
func sqlText(v any) string {
	switch x := v.(type) {
	case nil:
		return "None"
	case []byte:
		return string(x)
	default:
		return fmt.Sprint(x)
	}
}
