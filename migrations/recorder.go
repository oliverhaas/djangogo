package migrations

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/oliverhaas/djangogo/orm"
)

// TableName is the migration-tracking table.
const TableName = "djangogo_migrations"

// trackingTableSQL returns the CREATE TABLE IF NOT EXISTS statement for the
// migration-tracking table, rendered for the given dialect.
func trackingTableSQL(d orm.Dialect) string {
	idCol := d.Quote("id") + " " + d.ColumnType(&orm.Field{Kind: orm.KindAuto, PrimaryKey: true})
	appCol := d.Quote("app") + " " + d.ColumnType(&orm.Field{Kind: orm.KindChar, MaxLength: 255})
	nameCol := d.Quote("name") + " " + d.ColumnType(&orm.Field{Kind: orm.KindChar, MaxLength: 255})
	appliedAtCol := d.Quote("applied_at") + " " + d.ColumnType(&orm.Field{Kind: orm.KindDateTime})
	cols := strings.Join([]string{idCol, appCol, nameCol, appliedAtCol}, ", ")
	return "CREATE TABLE IF NOT EXISTS " + d.Quote(TableName) + " (" + cols + ")"
}

// EnsureTable creates the tracking table if it does not exist.
func EnsureTable(ctx context.Context, db *orm.DB) error {
	_, err := db.SQL().ExecContext(ctx, trackingTableSQL(db.Dialect()))
	return err
}

// AppliedSet returns the set of already-applied migrations, keyed "<app>/<name>".
func AppliedSet(ctx context.Context, db *orm.DB) (map[string]bool, error) {
	d := db.Dialect()
	q := "SELECT " + d.Quote("app") + ", " + d.Quote("name") +
		" FROM " + d.Quote(TableName)
	rows, err := db.SQL().QueryContext(ctx, q)
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
// insert participates in the migration's transaction. The dialect d is used to render
// the correct placeholder syntax for the backend.
func recordApplied(ctx context.Context, tx *sql.Tx, app, name string, d orm.Dialect) error {
	q := "INSERT INTO " + d.Quote(TableName) +
		" (" + d.Quote("app") + ", " + d.Quote("name") + ", " + d.Quote("applied_at") + ")" +
		" VALUES (" + d.Placeholder(1) + ", " + d.Placeholder(2) + ", " + d.Placeholder(3) + ")"
	_, err := tx.ExecContext(ctx, q, app, name, time.Now().UTC())
	return err
}
