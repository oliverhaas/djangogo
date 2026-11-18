package migrations

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/oliverhaas/djangogo/orm"
)

// WithSchemaEditor runs fn with a schema editor inside one transaction and
// commits when fn returns nil; any error rolls everything back. Both dialects
// have transactional DDL, so a failed migration leaves no partial schema.
//
// On SQLite, foreign-key enforcement is switched off for the duration, as
// Django does: a table rebuild drops and recreates a table that other tables
// refer to. The foreign keys are checked before the commit.
//
// The transaction holds a database connection. fn must do its work through
// the editor (Execute, DB) and not through the original handle; on a
// single-connection database, such as SQLite in memory, a query on the
// original handle would wait forever.
func WithSchemaEditor(ctx context.Context, db *orm.DB, fn func(e *SchemaEditor) error) (err error) {
	conn, err := db.SQL().Conn(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()

	enforce := false
	if db.Dialect().Name() == "sqlite" {
		var on int
		if err := conn.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&on); err != nil {
			return err
		}
		if enforce = on == 1; enforce {
			// The pragma is ignored inside a transaction, so it is set before.
			if _, err := conn.ExecContext(ctx, "PRAGMA foreign_keys = OFF"); err != nil {
				return err
			}
			defer func() {
				_, restoreErr := conn.ExecContext(context.WithoutCancel(ctx), "PRAGMA foreign_keys = ON")
				err = errors.Join(err, restoreErr)
			}()
		}
	}

	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	committed := false
	e := &SchemaEditor{dialect: db.Dialect(), run: tx, base: db}
	defer func() {
		e.close()
		if !committed {
			_ = tx.Rollback()
		}
	}()

	if err := fn(e); err != nil {
		return err
	}
	if enforce {
		if err := checkForeignKeys(ctx, tx); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	committed = true
	return nil
}

// checkForeignKeys fails when a row violates a foreign key (SQLite only).
func checkForeignKeys(ctx context.Context, tx *sql.Tx) error {
	rows, err := tx.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	if rows.Next() {
		var table string
		var rowid sql.NullInt64
		var parent string
		var fkid int
		if err := rows.Scan(&table, &rowid, &parent, &fkid); err != nil {
			return err
		}
		return fmt.Errorf("migrations: the row with rowid %d of table %q violates a foreign key to table %q",
			rowid.Int64, table, parent)
	}
	return rows.Err()
}

// CollectSQL runs fn with a schema editor that records its statements for
// the dialect d instead of executing them, and returns what it recorded.
func CollectSQL(d orm.Dialect, fn func(e *SchemaEditor) error) ([]string, error) {
	e := &SchemaEditor{dialect: d, collecting: true}
	if err := fn(e); err != nil {
		return nil, err
	}
	return e.collected, nil
}
