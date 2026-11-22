package migrations

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/oliverhaas/djangogo/orm"
)

// This file holds the values that existing rows get when a migration adds or
// alters a NOT NULL column, and their rendering as SQL literals.

// nowDefault marks "the current time" as the value for existing rows of an
// auto_now or auto_now_add column.
type nowDefault struct{}

// dateNowDefault marks CURRENT_DATE for an auto_now/auto_now_add KindDate column.
type dateNowDefault struct{}

// timeNowDefault marks CURRENT_TIME for an auto_now/auto_now_add KindTime column.
type timeNowDefault struct{}

// Now is a one-off default for a date-time column: the existing rows get the
// time at which the migration runs.
var Now any = nowDefault{}

// effectiveDefault returns the value existing rows get for a column, or nil
// when there is none. The order follows Django: a one-off or field default,
// then the empty string for a blank text column, then the current time for an
// auto_now column.
func effectiveDefault(f FieldState, oneOff any) any {
	switch {
	case oneOff != nil:
		return oneOff
	case f.Default != nil:
		return f.Default
	case !f.Null && f.Blank && (f.Kind == orm.KindChar || f.Kind == orm.KindText ||
		f.Kind == orm.KindSlug || f.Kind == orm.KindEmail || f.Kind == orm.KindURL):
		return ""
	case (f.AutoNow || f.AutoNowAdd) && f.Kind == orm.KindDate:
		return dateNowDefault{}
	case (f.AutoNow || f.AutoNowAdd) && f.Kind == orm.KindTime:
		return timeNowDefault{}
	case f.AutoNow || f.AutoNowAdd:
		return nowDefault{}
	default:
		return nil
	}
}

// quoteValue renders a default value as an SQL literal: the markers of the
// current date and time as their SQL functions, and any other value as
// orm.QuoteValue renders it.
func quoteValue(d orm.Dialect, v any) (string, error) {
	switch v.(type) {
	case nowDefault:
		return "CURRENT_TIMESTAMP", nil
	case dateNowDefault:
		return "CURRENT_DATE", nil
	case timeNowDefault:
		return "CURRENT_TIME", nil
	}
	s, err := orm.QuoteValue(d, v)
	if err != nil {
		return "", fmt.Errorf("migrations: default value: %w", err)
	}
	return s, nil
}

// tableHasRows reports whether table holds at least one row.
func (e *SchemaEditor) tableHasRows(ctx context.Context, table string) (bool, error) {
	var one int
	err := e.run.QueryRowContext(ctx, "SELECT 1 FROM "+e.q(table)+" LIMIT 1").Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}
