package migrations

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/oliverhaas/djangogo/orm"
)

// This file holds the values that existing rows get when a migration adds or
// alters a NOT NULL column, and their rendering as SQL literals.

// nowDefault marks "the current time" as the value for existing rows of an
// auto_now or auto_now_add column.
type nowDefault struct{}

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
	case !f.Null && f.Blank && (f.Kind == orm.KindChar || f.Kind == orm.KindText):
		return ""
	case f.AutoNow || f.AutoNowAdd:
		return nowDefault{}
	default:
		return nil
	}
}

// quoteValue renders a default value as an SQL literal. Strings are escaped by
// doubling the single quote, which is safe on both dialects with standard
// conforming strings; a NUL byte is rejected because neither accepts it.
func quoteValue(d orm.Dialect, v any) (string, error) {
	switch x := v.(type) {
	case nil:
		return "NULL", nil
	case nowDefault:
		return "CURRENT_TIMESTAMP", nil
	case bool:
		if d.Name() == "sqlite" {
			if x {
				return "1", nil
			}
			return "0", nil
		}
		return strconv.FormatBool(x), nil
	case int:
		return strconv.Itoa(x), nil
	case int64:
		return strconv.FormatInt(x, 10), nil
	case string:
		if strings.ContainsRune(x, 0) {
			return "", errors.New("migrations: a default value must not contain a NUL byte")
		}
		return "'" + strings.ReplaceAll(x, "'", "''") + "'", nil
	case time.Time:
		return "'" + x.UTC().Format("2006-01-02 15:04:05.999999-07:00") + "'", nil
	default:
		return "", fmt.Errorf("migrations: unsupported default value of type %T", v)
	}
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
