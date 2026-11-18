package migrations

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/oliverhaas/djangogo/orm"
)

// TableName is the table that records the applied migrations. Name and
// columns (id, app, name, applied) are Django's.
const TableName = "django_migrations"

// legacyTableName is the tracking table of earlier Djan-Go-Go releases. It had
// the same columns, with "applied_at" in place of "applied".
const legacyTableName = "djangogo_migrations"

// Recorder reads and writes the table of applied migrations, the counterpart
// of Django's MigrationRecorder.
type Recorder struct {
	db *orm.DB
}

// NewRecorder returns a recorder for db.
func NewRecorder(db *orm.DB) *Recorder { return &Recorder{db: db} }

// tableExists reports whether a table of the given name is visible.
func tableExists(ctx context.Context, run sqlRunner, d orm.Dialect, table string) (bool, error) {
	query := "SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = " + d.Placeholder(1)
	if d.Name() != "sqlite" {
		query = "SELECT 1 FROM pg_class WHERE relname = " + d.Placeholder(1) +
			" AND relkind = 'r' AND pg_table_is_visible(oid)"
	}
	var one int
	err := run.QueryRowContext(ctx, query, table).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

// HasTable reports whether the tracking table exists, under its current or
// its legacy name.
func (r *Recorder) HasTable(ctx context.Context) (bool, error) {
	for _, name := range []string{TableName, legacyTableName} {
		ok, err := tableExists(ctx, r.db.SQL(), r.db.Dialect(), name)
		if err != nil || ok {
			return ok, err
		}
	}
	return false, nil
}

// EnsureSchema creates the tracking table when it is missing. A table in the
// legacy layout is renamed in place, so the record of applied migrations of a
// database made by an earlier release is kept.
func (r *Recorder) EnsureSchema(ctx context.Context) error {
	d := r.db.Dialect()
	exists, err := tableExists(ctx, r.db.SQL(), d, TableName)
	if err != nil || exists {
		return err
	}
	legacy, err := tableExists(ctx, r.db.SQL(), d, legacyTableName)
	if err != nil {
		return err
	}
	if legacy {
		return r.upgradeLegacy(ctx)
	}
	cols := []string{
		d.Quote("id") + " " + d.ColumnType(&orm.Field{Kind: orm.KindAuto, PrimaryKey: true}),
		d.Quote("app") + " " + d.ColumnType(&orm.Field{Kind: orm.KindChar, MaxLength: 255}),
		d.Quote("name") + " " + d.ColumnType(&orm.Field{Kind: orm.KindChar, MaxLength: 255}),
		d.Quote("applied") + " " + d.ColumnType(&orm.Field{Kind: orm.KindDateTime}),
	}
	_, err = r.db.SQL().ExecContext(ctx, "CREATE TABLE "+d.Quote(TableName)+" ("+strings.Join(cols, ", ")+")")
	return err
}

// upgradeLegacy renames the legacy table and its timestamp column in one
// transaction.
func (r *Recorder) upgradeLegacy(ctx context.Context) error {
	d := r.db.Dialect()
	tx, err := r.db.SQL().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	for _, stmt := range []string{
		"ALTER TABLE " + d.Quote(legacyTableName) + " RENAME TO " + d.Quote(TableName),
		"ALTER TABLE " + d.Quote(TableName) + " RENAME COLUMN " + d.Quote("applied_at") + " TO " + d.Quote("applied"),
	} {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

// AppliedMigrations returns the applied migrations with the time each was
// applied. A missing tracking table means that nothing is applied.
func (r *Recorder) AppliedMigrations(ctx context.Context) (map[Key]time.Time, error) {
	d := r.db.Dialect()
	table, column := TableName, "applied"
	exists, err := tableExists(ctx, r.db.SQL(), d, TableName)
	if err != nil {
		return nil, err
	}
	if !exists {
		legacy, err := tableExists(ctx, r.db.SQL(), d, legacyTableName)
		if err != nil || !legacy {
			return map[Key]time.Time{}, err
		}
		table, column = legacyTableName, "applied_at"
	}
	rows, err := r.db.SQL().QueryContext(ctx, "SELECT "+d.Quote("app")+", "+d.Quote("name")+", "+
		d.Quote(column)+" FROM "+d.Quote(table))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	applied := map[Key]time.Time{}
	for rows.Next() {
		var key Key
		var at sql.NullTime
		if err := rows.Scan(&key.App, &key.Name, &at); err != nil {
			return nil, err
		}
		applied[key] = at.Time
	}
	return applied, rows.Err()
}

// RecordApplied records that a migration was applied.
func (r *Recorder) RecordApplied(ctx context.Context, key Key) error {
	if err := r.EnsureSchema(ctx); err != nil {
		return err
	}
	return recordApplied(ctx, r.db.SQL(), r.db.Dialect(), key)
}

// RecordUnapplied records that a migration was unapplied.
func (r *Recorder) RecordUnapplied(ctx context.Context, key Key) error {
	if err := r.EnsureSchema(ctx); err != nil {
		return err
	}
	return recordUnapplied(ctx, r.db.SQL(), r.db.Dialect(), key)
}

// Flush deletes all migration records. It is meant for tests.
func (r *Recorder) Flush(ctx context.Context) error {
	if err := r.EnsureSchema(ctx); err != nil {
		return err
	}
	_, err := r.db.SQL().ExecContext(ctx, "DELETE FROM "+r.db.Dialect().Quote(TableName))
	return err
}

// recordApplied inserts the record through run, so it is part of the
// transaction that applies the migration.
func recordApplied(ctx context.Context, run sqlRunner, d orm.Dialect, key Key) error {
	_, err := run.ExecContext(ctx, "INSERT INTO "+d.Quote(TableName)+
		" ("+d.Quote("app")+", "+d.Quote("name")+", "+d.Quote("applied")+") VALUES ("+
		d.Placeholder(1)+", "+d.Placeholder(2)+", "+d.Placeholder(3)+")",
		key.App, key.Name, time.Now().UTC())
	return err
}

// recordUnapplied deletes the record through run.
func recordUnapplied(ctx context.Context, run sqlRunner, d orm.Dialect, key Key) error {
	_, err := run.ExecContext(ctx, "DELETE FROM "+d.Quote(TableName)+" WHERE "+
		d.Quote("app")+" = "+d.Placeholder(1)+" AND "+d.Quote("name")+" = "+d.Placeholder(2),
		key.App, key.Name)
	return err
}
