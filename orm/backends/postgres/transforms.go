package postgres

import (
	"strings"

	"github.com/oliverhaas/djangogo/orm"
)

// TransformSQL returns the SQL of a date or time transform, as Django's
// PostgreSQL backend writes it: EXTRACT of the part, with a datetime first
// converted to tzname by AT TIME ZONE, and a cast for "date" and "time". The
// seconds are truncated, as Django truncates them, and week_day counts from
// Sunday = 1.
func (Dialect) TransformSQL(name string, input orm.Kind, tzname string) (string, []any, bool) {
	switch input {
	case orm.KindDateTime:
		switch name {
		case "date":
			return "({} AT TIME ZONE %s)::date", []any{tzname}, true
		case "time":
			return "({} AT TIME ZONE %s)::time", []any{tzname}, true
		case "second":
			return "EXTRACT(SECOND FROM DATE_TRUNC(%s, {} AT TIME ZONE %s))", []any{"second", tzname}, true
		}
		return dateExtract(name, "{} AT TIME ZONE %s"), []any{tzname}, true
	case orm.KindTime:
		if name == "second" {
			return "EXTRACT(SECOND FROM DATE_TRUNC(%s, {}))", []any{"second"}, true
		}
		return dateExtract(name, "{}"), nil, true
	case orm.KindDate:
		return dateExtract(name, "{}"), nil, true
	}
	return "", nil, false
}

// dateExtract returns the EXTRACT of part from expr, Django's
// date_extract_sql.
func dateExtract(part, expr string) string {
	switch part {
	case "week_day":
		return "EXTRACT(DOW FROM " + expr + ") + 1"
	case "iso_week_day":
		return "EXTRACT(ISODOW FROM " + expr + ")"
	case "iso_year":
		return "EXTRACT(ISOYEAR FROM " + expr + ")"
	}
	return "EXTRACT(" + strings.ToUpper(part) + " FROM " + expr + ")"
}
