package sqlite

import (
	"database/sql/driver"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/oliverhaas/djangogo/orm"

	moderncsqlite "modernc.org/sqlite"
)

// TransformSQL returns the SQL of a date or time transform: a call of one of
// the functions this package registers, with Django's names and arguments,
// since SQLite has no EXTRACT. A datetime is read in conn_tzname and
// converted to tzname; both are tzname, as Django passes them with USE_TZ and
// a connection without its own TIME_ZONE.
func (Dialect) TransformSQL(name string, input orm.Kind, tzname string) (string, []any, bool) {
	switch input {
	case orm.KindDateTime:
		switch name {
		case "date":
			return "django_datetime_cast_date({}, %s, %s)", []any{tzname, tzname}, true
		case "time":
			return "django_datetime_cast_time({}, %s, %s)", []any{tzname, tzname}, true
		}
		return "django_datetime_extract(%s, {}, %s, %s)", []any{name, tzname, tzname}, true
	case orm.KindDate:
		return "django_date_extract(%s, {})", []any{name}, true
	case orm.KindTime:
		return "django_time_extract(%s, {})", []any{name}, true
	}
	return "", nil, false
}

// CastSQL returns the SQL of Cast to a datetime, a date or a time of day,
// which SQLite stores as text: Django's strftime and date calls, and for a
// stored datetime the functions of the date and time transforms, which read
// the text the driver writes. Other casts are CAST(value AS type).
func (Dialect) CastSQL(input, output orm.Kind, _, tzname string) (string, []any, bool) {
	switch output {
	case orm.KindDateTime:
		if input == orm.KindDateTime {
			return "{}", nil, true
		}
		return "strftime(%s, {})", []any{"%Y-%m-%d %H:%M:%f"}, true
	case orm.KindDate:
		if input == orm.KindDateTime {
			return "django_datetime_cast_date({}, %s, %s)", []any{tzname, tzname}, true
		}
		return "date({})", nil, true
	case orm.KindTime:
		switch input {
		case orm.KindDateTime:
			return "django_datetime_cast_time({}, %s, %s)", []any{tzname, tzname}, true
		case orm.KindTime:
			return "{}", nil, true
		}
		return "strftime(%s, {})", []any{"%H:%M:%f"}, true
	}
	return "", nil, false
}

// datetimeLayouts are the forms a datetime is read in: Go's time.String,
// which the driver writes, and the ISO 8601 forms Django writes, without and
// with an offset. A value without an offset is in the connection's zone.
var datetimeLayouts = []string{
	"2006-01-02 15:04:05.999999999 -0700 MST",
	"2006-01-02 15:04:05.999999999-07:00",
	"2006-01-02T15:04:05.999999999-07:00",
	"2006-01-02 15:04:05.999999999",
	"2006-01-02T15:04:05.999999999",
	"2006-01-02 15:04",
	"2006-01-02T15:04",
	"2006-01-02",
}

// locations caches the zones the datetime functions convert to.
var locations sync.Map // string -> *time.Location

// location returns the zone named name.
func location(name string) (*time.Location, error) {
	if loc, ok := locations.Load(name); ok {
		return loc.(*time.Location), nil
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return nil, err
	}
	locations.Store(name, loc)
	return loc, nil
}

// parseDatetime reads a stored datetime, Django's _sqlite_datetime_parse: a
// value without an offset is in conn_tzname, and the result is converted to
// tzname. ok is false for NULL and for text that is not a datetime, which
// the functions return as NULL.
func parseDatetime(v driver.Value, tzname, connTZ driver.Value) (time.Time, bool, error) {
	conn, target := time.UTC, time.UTC
	for _, z := range []struct {
		name driver.Value
		loc  **time.Location
	}{{connTZ, &conn}, {tzname, &target}} {
		if z.name == nil {
			continue
		}
		loc, err := location(textOf(z.name))
		if err != nil {
			return time.Time{}, false, err
		}
		*z.loc = loc
	}
	var t time.Time
	switch x := v.(type) {
	case nil:
		return t, false, nil
	case time.Time:
		t = x
	default:
		s := textOf(x)
		if i := strings.Index(s, " m="); i >= 0 {
			s = s[:i]
		}
		if rest, ok := strings.CutSuffix(s, "Z"); ok {
			s = rest + "+00:00"
		}
		parsed := false
		for _, layout := range datetimeLayouts {
			if p, err := time.ParseInLocation(layout, s, conn); err == nil {
				t, parsed = p, true
				break
			}
		}
		if !parsed {
			return t, false, nil
		}
	}
	return t.In(target), true, nil
}

// extract returns part of t, Django's _sqlite_datetime_extract: week_day
// counts from Sunday = 1, iso_week_day from Monday = 1, and week and iso_year
// follow ISO 8601.
func extract(part string, t time.Time) (driver.Value, error) {
	switch part {
	case "year":
		return int64(t.Year()), nil
	case "month":
		return int64(t.Month()), nil
	case "day":
		return int64(t.Day()), nil
	case "hour":
		return int64(t.Hour()), nil
	case "minute":
		return int64(t.Minute()), nil
	case "second":
		return int64(t.Second()), nil
	case "week_day":
		return int64(t.Weekday()) + 1, nil
	case "iso_week_day":
		return int64((int(t.Weekday())+6)%7) + 1, nil
	case "week":
		_, week := t.ISOWeek()
		return int64(week), nil
	case "iso_year":
		year, _ := t.ISOWeek()
		return int64(year), nil
	case "quarter":
		return int64((int(t.Month()) + 2) / 3), nil
	}
	return nil, fmt.Errorf("sqlite: unsupported lookup type %q", part)
}

// timeOfDay formats the time of t as Python's time.isoformat does:
// "HH:MM:SS", with ".ffffff" when there are microseconds.
func timeOfDay(t time.Time) string {
	s := t.Format("15:04:05")
	if us := t.Nanosecond() / 1000; us != 0 {
		s += fmt.Sprintf(".%06d", us)
	}
	return s
}

// dateExtractFunc implements django_date_extract(lookup_type, value).
func dateExtractFunc(_ *moderncsqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	t, ok, err := parseDatetime(args[1], nil, nil)
	if !ok || err != nil {
		return nil, err
	}
	return extract(textOf(args[0]), t)
}

// datetimeExtractFunc implements django_datetime_extract(lookup_type, value,
// tzname, conn_tzname).
func datetimeExtractFunc(_ *moderncsqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	t, ok, err := parseDatetime(args[1], args[2], args[3])
	if !ok || err != nil {
		return nil, err
	}
	return extract(textOf(args[0]), t)
}

// datetimeCastDateFunc implements django_datetime_cast_date(value, tzname,
// conn_tzname): the date as "YYYY-MM-DD".
func datetimeCastDateFunc(_ *moderncsqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	t, ok, err := parseDatetime(args[0], args[1], args[2])
	if !ok || err != nil {
		return nil, err
	}
	return t.Format("2006-01-02"), nil
}

// datetimeCastTimeFunc implements django_datetime_cast_time(value, tzname,
// conn_tzname): the time of day as "HH:MM:SS[.ffffff]".
func datetimeCastTimeFunc(_ *moderncsqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	t, ok, err := parseDatetime(args[0], args[1], args[2])
	if !ok || err != nil {
		return nil, err
	}
	return timeOfDay(t), nil
}

// timeExtractFunc implements django_time_extract(lookup_type, value) for a
// time of day stored as "HH:MM:SS[.ffffff]". Text that is not a time of day
// gives NULL.
func timeExtractFunc(_ *moderncsqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	if args[1] == nil {
		return nil, nil
	}
	tod, err := orm.ParseTime(textOf(args[1]))
	if err != nil {
		return nil, nil //nolint:nilerr // Django's typecast_time failure is NULL.
	}
	t := time.Date(1, 1, 1, tod.Hour, tod.Minute, tod.Second, tod.Nanosecond, time.UTC)
	return extract(textOf(args[0]), t)
}

// functionRegistration is the outcome of installing the SQL functions of this
// package for every connection the driver opens; Open reports a failure.
// SQLite ships the REGEXP operator without an implementation, and Django's
// date and time functions exist only where Django or this package registers
// them.
var functionRegistration = errors.Join(
	moderncsqlite.RegisterDeterministicScalarFunction("regexp", 2, regexpFunc),
	moderncsqlite.RegisterDeterministicScalarFunction("django_date_extract", 2, dateExtractFunc),
	moderncsqlite.RegisterDeterministicScalarFunction("django_datetime_extract", 4, datetimeExtractFunc),
	moderncsqlite.RegisterDeterministicScalarFunction("django_datetime_cast_date", 3, datetimeCastDateFunc),
	moderncsqlite.RegisterDeterministicScalarFunction("django_datetime_cast_time", 3, datetimeCastTimeFunc),
	moderncsqlite.RegisterDeterministicScalarFunction("django_time_extract", 2, timeExtractFunc),
)
