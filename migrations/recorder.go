package migrations

import (
	"context"
	"database/sql"
	"time"

	"github.com/oliverhaas/djangogo/orm"
)

// TableName is the migration-tracking table.
const TableName = "djangogo_migrations"

// EnsureTable creates the tracking table if it does not exist.
func EnsureTable(ctx context.Context, db *orm.DB) error {
	const stmt = `CREATE TABLE IF NOT EXISTS "djangogo_migrations" ` +
		`("id" INTEGER PRIMARY KEY AUTOINCREMENT, "app" TEXT NOT NULL, ` +
		`"name" TEXT NOT NULL, "applied_at" DATETIME NOT NULL)`
	_, err := db.SQL().ExecContext(ctx, stmt)
	return err
}

// AppliedSet returns the set of already-applied migrations, keyed "<app>/<name>".
func AppliedSet(ctx context.Context, db *orm.DB) (map[string]bool, error) {
	rows, err := db.SQL().QueryContext(ctx, `SELECT app, name FROM "djangogo_migrations"`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	applied := map[string]bool{}
	for rows.Next() {
		var app, name string
		if err := rows.Scan(&app, &name); err != nil {
			return nil, err
		}
		applied[app+"/"+name] = true
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return applied, nil
}

// recordApplied inserts a tracking row for the (app, name) migration using tx so the
// insert participates in the migration's transaction.
func recordApplied(ctx context.Context, tx *sql.Tx, app, name string) error {
	_, err := tx.ExecContext(ctx,
		`INSERT INTO "djangogo_migrations" ("app", "name", "applied_at") VALUES (?, ?, ?)`,
		app, name, time.Now().UTC())
	return err
}
