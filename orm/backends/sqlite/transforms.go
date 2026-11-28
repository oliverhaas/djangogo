package sqlite

import (
	"database/sql/driver"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/oliverhaas/djangogo/orm"

	moderncsqlite "modernc.org/sqlite"
)

// connTimeZone is the conn_tzname of Django's SQLite functions, the zone of
// a stored datetime without an offset: the time zone of a connection
// without its own TIME_ZONE when USE_TZ is set.
const connTimeZone = orm.TransformTimeZone

// TransformSQL returns the SQL of a date or time transform: a call of one of
// the functions this package registers, with Django's names and arguments,
// since SQLite has no EXTRACT. A datetime is read in conn_tzname and
// converted to tzname.
func (Dialect) TransformSQL(name string, input orm.Kind, tzname string) (string, []any, bool) {
	switch input {
	case orm.KindDateTime:
		switch name {
		case "date":
			return "django_datetime_cast_date({}, %s, %s)", []any{tzname, connTimeZone}, true
		case "time":
			return "django_datetime_cast_time({}, %s, %s)", []any{tzname, connTimeZone}, true
		}
		return "django_datetime_extract(%s, {}, %s, %s)", []any{name, tzname, connTimeZone}, true
	case orm.KindDate:
		return "django_date_extract(%s, {})", []any{name}, true
	case orm.KindTime:
		return "django_time_extract(%s, {})", []any{name}, true
	}
	return "", nil, false
}

// TruncSQL returns the SQL of Trunc: a call of django_date_trunc,
// django_datetime_trunc or django_time_trunc by the kind of the value, with
// Django's arguments. tzname and conn_tzname are NULL for a value that is not
// a datetime.
func (Dialect) TruncSQL(kind string, input, output orm.Kind, tzname string) (string, []any, bool) {
	var fn string
	switch output {
	case orm.KindDate:
		fn = "django_date_trunc"
	case orm.KindDateTime:
		fn = "django_datetime_trunc"
	case orm.KindTime:
		fn = "django_time_trunc"
	default:
		return "", nil, false
	}
	params := []any{kind, nil, nil}
	if input == orm.KindDateTime {
		params = []any{kind, tzname, connTimeZone}
	}
	return fn + "(%s, {}, %s, %s)", params, true
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
			return "django_datetime_cast_date({}, %s, %s)", []any{tzname, connTimeZone}, true
		}
		return "date({})", nil, true
	case orm.KindTime:
		switch input {
		case orm.KindDateTime:
			return "django_datetime_cast_time({}, %s, %s)", []any{tzname, connTimeZone}, true
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
// tzname. A tzname that ends in an offset, such as "UTC-04:43", is read with
// Django's split_tzname_delta: the result is in a fixed zone of the offset
// added to that of the named part, conn_tzname when it has none. ok is false
// for NULL and for text that is not a datetime, which the functions return
// as NULL.
func parseDatetime(v driver.Value, tzname, connTZ driver.Value) (time.Time, bool, error) {
	conn := time.UTC
	if connTZ != nil {
		loc, err := location(textOf(connTZ))
		if err != nil {
			return time.Time{}, false, err
		}
		conn = loc
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
	if tzname == nil || connTZ != nil && textOf(tzname) == textOf(connTZ) {
		return t.In(conn), true, nil
	}
	name := textOf(tzname)
	base, sign, offset := orm.SplitTimeZoneName(name)
	if offset == "" {
		loc, err := location(name)
		if err != nil {
			return time.Time{}, false, err
		}
		return t.In(loc), true, nil
	}
	shift, err := offsetSeconds(offset)
	if err != nil {
		return time.Time{}, false, err
	}
	if sign == "-" {
		shift = -shift
	}
	baseLoc := conn
	if base != "" {
		if baseLoc, err = location(base); err != nil {
			return time.Time{}, false, err
		}
	}
	_, baseOffset := t.In(baseLoc).Zone()
	return t.In(time.FixedZone(name, baseOffset+shift)), true, nil
}

// offsetSeconds returns the seconds of the "HH:MM" offset of a zone name.
func offsetSeconds(offset string) (int, error) {
	hours, minutes, _ := strings.Cut(offset, ":")
	h, err := strconv.Atoi(hours)
	if err != nil {
		return 0, fmt.Errorf("sqlite: invalid time zone offset %q", offset)
	}
	m, err := strconv.Atoi(strings.SplitN(minutes, ":", 2)[0])
	if err != nil {
		return 0, fmt.Errorf("sqlite: invalid time zone offset %q", offset)
	}
	return h*3600 + m*60, nil
}

// truncate returns t truncated to kind in its location, Django's
// _sqlite_datetime_trunc: "week" is midnight of the Monday.
func truncate(kind string, t time.Time) (time.Time, error) {
	y, mo, d := t.Date()
	h, mi, sec := t.Clock()
	loc := t.Location()
	switch kind {
	case "year":
		return time.Date(y, 1, 1, 0, 0, 0, 0, loc), nil
	case "quarter":
		return time.Date(y, mo-(mo-1)%3, 1, 0, 0, 0, 0, loc), nil
	case "month":
		return time.Date(y, mo, 1, 0, 0, 0, 0, loc), nil
	case "week":
		return time.Date(y, mo, d-(int(t.Weekday())+6)%7, 0, 0, 0, 0, loc), nil
	case "day":
		return time.Date(y, mo, d, 0, 0, 0, 0, loc), nil
	case "hour":
		return time.Date(y, mo, d, h, 0, 0, 0, loc), nil
	case "minute":
		return time.Date(y, mo, d, h, mi, 0, 0, loc), nil
	case "second":
		return time.Date(y, mo, d, h, mi, sec, 0, loc), nil
	}
	return time.Time{}, fmt.Errorf("sqlite: unsupported lookup type %q", kind)
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

// dateTruncFunc implements django_date_trunc(lookup_type, value, tzname,
// conn_tzname): the date of the truncated value as "YYYY-MM-DD".
func dateTruncFunc(_ *moderncsqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	t, ok, err := parseDatetime(args[1], args[2], args[3])
	if !ok || err != nil {
		return nil, err
	}
	switch kind := textOf(args[0]); kind {
	case "year", "quarter", "month", "week", "day":
		t, _ = truncate(kind, t)
	default:
		return nil, fmt.Errorf("sqlite: unsupported lookup type %q", kind)
	}
	return fmt.Sprintf("%04d-%02d-%02d", t.Year(), t.Month(), t.Day()), nil
}

// datetimeTruncFunc implements django_datetime_trunc(lookup_type, value,
// tzname, conn_tzname). Where Django's returns the truncated wall time in
// tzname, which Python reads in that zone, this one returns its instant in
// the form the driver writes a datetime in, so that the value compares equal
// with a stored datetime.
func datetimeTruncFunc(_ *moderncsqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	t, ok, err := parseDatetime(args[1], args[2], args[3])
	if !ok || err != nil {
		return nil, err
	}
	t, err = truncate(textOf(args[0]), t)
	if err != nil {
		return nil, err
	}
	return t.UTC().String(), nil
}

// timeTruncFunc implements django_time_trunc(lookup_type, value, tzname,
// conn_tzname) for a datetime or a time of day: the truncated time of day as
// "HH:MM:SS".
func timeTruncFunc(_ *moderncsqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	if args[1] == nil {
		return nil, nil
	}
	t, ok, err := parseDatetime(args[1], args[2], args[3])
	if err != nil {
		return nil, err
	}
	if !ok {
		tod, err := orm.ParseTime(textOf(args[1]))
		if err != nil {
			return nil, nil //nolint:nilerr // Django's typecast_time failure is NULL.
		}
		t = time.Date(1, 1, 1, tod.Hour, tod.Minute, tod.Second, tod.Nanosecond, time.UTC)
	}
	switch kind := textOf(args[0]); kind {
	case "hour", "minute", "second":
		t, _ = truncate(kind, t)
	default:
		return nil, fmt.Errorf("sqlite: unsupported lookup type %q", kind)
	}
	return t.Format("15:04:05"), nil
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
// date and time functions, COT, RAND, LPAD, RPAD, REPEAT, REVERSE, MD5, SHA1
// to SHA512 and the aggregates StdDev, Variance and AnyValue exist only where
// Django or this package registers them.
var functionRegistration = errors.Join(
	moderncsqlite.RegisterDeterministicScalarFunction("regexp", 2, regexpFunc),
	moderncsqlite.RegisterDeterministicScalarFunction("django_date_extract", 2, dateExtractFunc),
	moderncsqlite.RegisterDeterministicScalarFunction("django_datetime_extract", 4, datetimeExtractFunc),
	moderncsqlite.RegisterDeterministicScalarFunction("django_datetime_cast_date", 3, datetimeCastDateFunc),
	moderncsqlite.RegisterDeterministicScalarFunction("django_datetime_cast_time", 3, datetimeCastTimeFunc),
	moderncsqlite.RegisterDeterministicScalarFunction("django_time_extract", 2, timeExtractFunc),
	moderncsqlite.RegisterDeterministicScalarFunction("django_date_trunc", 4, dateTruncFunc),
	moderncsqlite.RegisterDeterministicScalarFunction("django_datetime_trunc", 4, datetimeTruncFunc),
	moderncsqlite.RegisterDeterministicScalarFunction("django_time_trunc", 4, timeTruncFunc),
	moderncsqlite.RegisterDeterministicScalarFunction("django_format_dtdelta", 3, formatDtdeltaFunc),
	moderncsqlite.RegisterDeterministicScalarFunction("django_timestamp_diff", 2, timestampDiffFunc),
	moderncsqlite.RegisterDeterministicScalarFunction("django_time_diff", 2, timeDiffFunc),
	moderncsqlite.RegisterDeterministicScalarFunction("COT", 1, cotFunc),
	moderncsqlite.RegisterDeterministicScalarFunction("LPAD", 3, lpadFunc),
	moderncsqlite.RegisterDeterministicScalarFunction("MD5", 1, md5Func),
	moderncsqlite.RegisterDeterministicScalarFunction("REPEAT", 2, repeatFunc),
	moderncsqlite.RegisterDeterministicScalarFunction("REVERSE", 1, reverseFunc),
	moderncsqlite.RegisterDeterministicScalarFunction("RPAD", 3, rpadFunc),
	moderncsqlite.RegisterDeterministicScalarFunction("SHA1", 1, sha1Func),
	moderncsqlite.RegisterDeterministicScalarFunction("SHA224", 1, sha224Func),
	moderncsqlite.RegisterDeterministicScalarFunction("SHA256", 1, sha256Func),
	moderncsqlite.RegisterDeterministicScalarFunction("SHA384", 1, sha384Func),
	moderncsqlite.RegisterDeterministicScalarFunction("SHA512", 1, sha512Func),
	moderncsqlite.RegisterScalarFunction("RAND", 0, randFunc),
	moderncsqlite.RegisterFunction("STDDEV_POP", statFunction(false, true)),
	moderncsqlite.RegisterFunction("STDDEV_SAMP", statFunction(true, true)),
	moderncsqlite.RegisterFunction("VAR_POP", statFunction(false, false)),
	moderncsqlite.RegisterFunction("VAR_SAMP", statFunction(true, false)),
	moderncsqlite.RegisterFunction("ANY_VALUE", anyValueFunction),
)
