package sqlite

import (
	"database/sql/driver"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/oliverhaas/djangogo/orm"

	moderncsqlite "modernc.org/sqlite"
)

// microsPerDay is the number of microseconds in a day.
const microsPerDay = 24 * 60 * 60 * 1_000_000

// DurationSQL returns the call of django_format_dtdelta for a duration
// combined with a value of another type, Django's
// combine_duration_expression: SQLite has no duration type, and a duration is
// stored as a number of microseconds.
func (Dialect) DurationSQL(op string) (string, error) {
	switch op {
	case "+", "-", "*", "/":
		return "django_format_dtdelta('" + op + "', {}, {})", nil
	}
	return "", fmt.Errorf("sqlite: invalid connector for timedelta: %s", op)
}

// SubtractTemporalSQL returns the call that gives the difference of two
// times of day, or of two dates or datetimes, in microseconds, Django's
// subtract_temporals.
func (Dialect) SubtractTemporalSQL(kind orm.Kind) string {
	if kind == orm.KindTime {
		return "django_time_diff({}, {})"
	}
	return "django_timestamp_diff({}, {})"
}

// formatDtdeltaFunc implements django_format_dtdelta(connector, lhs, rhs),
// Django's _sqlite_format_dtdelta. For + and - an integer is a duration in
// microseconds, and the other side is a date, a datetime or a time of day,
// which the result has the form of; two durations give a duration. For * and
// / the result is the product or the quotient of two numbers, and / divides
// as Python's / does. A NULL or a value it cannot read gives NULL.
func formatDtdeltaFunc(_ *moderncsqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	op := strings.TrimSpace(textOf(args[0]))
	lhs, rhs := args[1], args[2]
	if lhs == nil || rhs == nil {
		return nil, nil
	}
	switch op {
	case "+", "-":
		return shiftTemporal(op, lhs, rhs), nil
	case "*", "/":
		return scaleDuration(op, lhs, rhs), nil
	}
	return nil, fmt.Errorf("sqlite: invalid connector for timedelta: %s", op)
}

// shiftTemporal returns lhs + rhs or lhs - rhs where one side is a duration in
// microseconds, or nil when the sides are not of that form.
func shiftTemporal(op string, lhs, rhs driver.Value) driver.Value {
	lus, lok := lhs.(int64)
	rus, rok := rhs.(int64)
	switch {
	case lok && rok:
		if op == "-" {
			return lus - rus
		}
		return lus + rus
	case rok:
		if op == "-" {
			rus = -rus
		}
		return shiftBy(textOf(lhs), rus)
	case lok && op == "+":
		return shiftBy(textOf(rhs), lus)
	}
	return nil
}

// shiftBy returns the date, datetime or time of day s moved by us
// microseconds, in the form of s: "YYYY-MM-DD" for a date, which moves by
// whole days as Python's date + timedelta does, Go's time.String in UTC for a
// datetime, and "HH:MM:SS[.ffffff]" for a time of day, which wraps around
// midnight as PostgreSQL's time + interval does. It is nil for other text.
func shiftBy(s string, us int64) driver.Value {
	if !strings.Contains(s, " ") && !strings.Contains(s, "T") {
		if d, err := time.Parse(time.DateOnly, s); err == nil {
			days := us / microsPerDay
			if us%microsPerDay < 0 {
				days--
			}
			return d.AddDate(0, 0, int(days)).Format(time.DateOnly)
		}
		tod, err := orm.ParseTime(s)
		if err != nil {
			return nil
		}
		day := ((microsOfDay(tod)+us)%microsPerDay + microsPerDay) % microsPerDay
		return orm.NewTime(int(day/3600_000_000), int(day/60_000_000%60), int(day/1_000_000%60),
			int(day%1_000_000)*1000).String()
	}
	t, ok, err := parseDatetime(s, nil, nil)
	if !ok || err != nil {
		return nil
	}
	// Move by whole days first: a time.Duration holds about 292 years only.
	return t.AddDate(0, 0, int(us/microsPerDay)).Add(time.Duration(us%microsPerDay) * time.Microsecond).UTC().String()
}

// scaleDuration returns lhs * rhs or lhs / rhs: an integer for the product of
// two integers, and otherwise a float. It is nil for a value that is not a
// number and for a division by zero.
func scaleDuration(op string, lhs, rhs driver.Value) driver.Value {
	li, lInt := lhs.(int64)
	ri, rInt := rhs.(int64)
	if op == "*" && lInt && rInt {
		return li * ri
	}
	l, lok := number(lhs)
	r, rok := number(rhs)
	if !lok || !rok {
		return nil
	}
	if op == "*" {
		return l * r
	}
	if r == 0 {
		return nil
	}
	return l / r
}

// number returns v as a float: an integer, a float, or the text of a number,
// such as a decimal bound as text.
func number(v driver.Value) (float64, bool) {
	switch x := v.(type) {
	case int64:
		return float64(x), true
	case float64:
		return x, true
	}
	f, err := strconv.ParseFloat(strings.TrimSpace(textOf(v)), 64)
	return f, err == nil && !math.IsNaN(f)
}

// timestampDiffFunc implements django_timestamp_diff(lhs, rhs), Django's
// _sqlite_timestamp_diff: the difference of two dates or datetimes in
// microseconds. A date is midnight.
func timestampDiffFunc(_ *moderncsqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	left, lok, err := parseDatetime(args[0], nil, nil)
	if !lok || err != nil {
		return nil, err
	}
	right, rok, err := parseDatetime(args[1], nil, nil)
	if !rok || err != nil {
		return nil, err
	}
	return left.UnixMicro() - right.UnixMicro(), nil
}

// timeDiffFunc implements django_time_diff(lhs, rhs), Django's
// _sqlite_time_diff: the difference of two times of day in microseconds.
func timeDiffFunc(_ *moderncsqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	if args[0] == nil || args[1] == nil {
		return nil, nil
	}
	left, err := orm.ParseTime(textOf(args[0]))
	if err != nil {
		return nil, nil //nolint:nilerr // Django's typecast_time failure is NULL.
	}
	right, err := orm.ParseTime(textOf(args[1]))
	if err != nil {
		return nil, nil //nolint:nilerr // Django's typecast_time failure is NULL.
	}
	return microsOfDay(left) - microsOfDay(right), nil
}

// microsOfDay returns the microseconds of t since midnight.
func microsOfDay(t orm.Time) int64 {
	return int64(t.Hour)*3600_000_000 + int64(t.Minute)*60_000_000 + int64(t.Second)*1_000_000 +
		int64(t.Nanosecond/1000)
}
