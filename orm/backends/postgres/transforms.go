package postgres

import (
	"strings"

	"github.com/oliverhaas/djangogo/orm"
)

// TransformSQL returns the SQL of a date or time transform, as Django's
// PostgreSQL backend writes it: EXTRACT of the part, with a datetime first
// converted to tzname by AT TIME ZONE, and a cast for "date" and "time". The
// seconds are truncated, as Django truncates them, and week_day counts from
// Sunday = 1. A duration, which only Extract reads, is first given its days
// by JUSTIFY_HOURS: the driver binds it as microseconds, which PostgreSQL
// keeps as hours.
func (Dialect) TransformSQL(name string, input orm.Kind, tzname string) (string, []any, bool) {
	tzname = prepareTimeZone(tzname)
	switch input {
	case orm.KindDuration:
		if name == "second" {
			return "EXTRACT(SECOND FROM DATE_TRUNC(%s, JUSTIFY_HOURS({})))", []any{"second"}, true
		}
		return dateExtract(name, "JUSTIFY_HOURS({})"), nil, true
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

// TruncSQL returns the SQL of Trunc, Django's date_trunc_sql,
// datetime_trunc_sql and time_trunc_sql: DATE_TRUNC of the value, a datetime
// first converted to tzname by AT TIME ZONE, and a cast to a date or a time
// of day. A truncated datetime is converted back from tzname to an instant,
// which Django does in Python.
func (Dialect) TruncSQL(kind string, input, output orm.Kind, tzname string) (string, []any, bool) {
	value, params := "{}", []any{kind}
	if input == orm.KindDateTime {
		tzname = prepareTimeZone(tzname)
		value, params = "{} AT TIME ZONE %s", append(params, tzname)
	}
	switch output {
	case orm.KindDateTime:
		return "DATE_TRUNC(%s, " + value + ") AT TIME ZONE %s", append(params, tzname), true
	case orm.KindDate:
		return "(DATE_TRUNC(%s, " + value + "))::date", params, true
	case orm.KindTime:
		return "DATE_TRUNC(%s, " + value + ")::time", params, true
	}
	return "", nil, false
}

// prepareTimeZone returns the name of a zone for AT TIME ZONE, Django's
// _prepare_tzname_delta: the sign of an offset is flipped, since PostgreSQL
// reads "UTC+05:00" as five hours west of UTC.
func prepareTimeZone(tzname string) string {
	name, sign, offset := orm.SplitTimeZoneName(tzname)
	if offset == "" {
		return tzname
	}
	if sign == "+" {
		return name + "-" + offset
	}
	return name + "+" + offset
}

// CastSQL returns the SQL of Cast in the form of Django's PostgreSQL backend,
// (value)::type. A datetime becomes a date or a time of day in tzname, and a
// date becomes midnight in tzname, where Django relies on the time zone of
// the connection.
func (Dialect) CastSQL(input, output orm.Kind, typ, tzname string) (string, []any, bool) {
	switch {
	case input == orm.KindDateTime && (output == orm.KindDate || output == orm.KindTime):
		return "(({}) AT TIME ZONE %s)::" + typ, []any{tzname}, true
	case input == orm.KindDate && output == orm.KindDateTime:
		return "(({})::timestamp AT TIME ZONE %s)", []any{tzname}, true
	}
	return "({})::" + typ, nil, true
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
