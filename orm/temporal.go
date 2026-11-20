package orm

import (
	"database/sql/driver"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Date is a calendar date without a time of day or timezone, the Go
// counterpart of Python's datetime.date for KindDate model fields.
//
// Using a dedicated comparable struct lets model fields infer KindDate without
// a tag, avoids time.Time timezone/monotonic equality pitfalls, and formats
// cleanly as "YYYY-MM-DD". Models may also use time.Time with orm:"type=date".
type Date struct {
	Year  int
	Month time.Month
	Day   int
}

// NewDate returns a Date for the given year, month, and day.
func NewDate(year int, month time.Month, day int) Date {
	return Date{Year: year, Month: month, Day: day}
}

// DateFromTime returns the calendar date of t.
func DateFromTime(t time.Time) Date {
	if t.IsZero() {
		return Date{}
	}
	y, m, d := t.Date()
	return Date{Year: y, Month: m, Day: d}
}

// ParseDate parses an ISO-8601 calendar date ("YYYY-MM-DD"). It also accepts a
// timestamp prefix when a database driver returns a date with a time part.
func ParseDate(s string) (Date, error) {
	s = strings.TrimSpace(s)
	if len(s) >= 10 && s[4] == '-' && s[7] == '-' {
		t, err := time.Parse("2006-01-02", s[:10])
		if err == nil && (len(s) == 10 || s[10] == ' ' || s[10] == 'T') {
			return DateFromTime(t), nil
		}
	}
	return Date{}, fmt.Errorf("orm: invalid date %q: expected YYYY-MM-DD", s)
}

// MustDate parses s with ParseDate and panics on error.
func MustDate(s string) Date {
	d, err := ParseDate(s)
	if err != nil {
		panic(fmt.Sprintf("orm.MustDate(%q): %v", s, err))
	}
	return d
}

// IsZero reports whether d is the zero Date.
func (d Date) IsZero() bool { return d.Year == 0 && d.Month == 0 && d.Day == 0 }

// Time returns d as a time.Time at midnight UTC, or the zero time.Time when d
// is zero.
func (d Date) Time() time.Time {
	if d.IsZero() {
		return time.Time{}
	}
	return time.Date(d.Year, d.Month, d.Day, 0, 0, 0, 0, time.UTC)
}

// String formats d as "YYYY-MM-DD", or "" for the zero Date.
func (d Date) String() string {
	if d.IsZero() {
		return ""
	}
	return fmt.Sprintf("%04d-%02d-%02d", d.Year, int(d.Month), d.Day)
}

// Value implements driver.Valuer. Zero Date maps to nil (SQL NULL).
func (d Date) Value() (driver.Value, error) {
	if d.IsZero() {
		return nil, nil
	}
	return d.String(), nil
}

// Scan implements sql.Scanner.
func (d *Date) Scan(src any) error {
	switch v := src.(type) {
	case nil:
		*d = Date{}
		return nil
	case time.Time:
		*d = DateFromTime(v)
		return nil
	case string:
		parsed, err := ParseDate(v)
		if err != nil {
			return err
		}
		*d = parsed
		return nil
	case []byte:
		parsed, err := ParseDate(string(v))
		if err != nil {
			return err
		}
		*d = parsed
		return nil
	default:
		return fmt.Errorf("orm: cannot scan %T into Date", src)
	}
}

// MarshalText implements encoding.TextMarshaler.
func (d Date) MarshalText() ([]byte, error) { return []byte(d.String()), nil }

// UnmarshalText implements encoding.TextUnmarshaler.
func (d *Date) UnmarshalText(text []byte) error {
	if len(strings.TrimSpace(string(text))) == 0 {
		*d = Date{}
		return nil
	}
	parsed, err := ParseDate(string(text))
	if err != nil {
		return err
	}
	*d = parsed
	return nil
}

// Time is a wall-clock time of day without a date or timezone, the Go
// counterpart of Python's datetime.time for KindTime model fields.
//
// Using a dedicated comparable struct lets model fields infer KindTime without
// a tag and preserves microsecond precision across SQLite and PostgreSQL. The
// zero value represents midnight (00:00:00); use *orm.Time for a nullable time
// column that distinguishes NULL from midnight. Models may also use time.Time
// with orm:"type=time".
type Time struct {
	Hour       int
	Minute     int
	Second     int
	Nanosecond int
}

// NewTime returns a Time for the given hour, minute, second, and nanosecond.
func NewTime(hour, minute, second, nanosecond int) Time {
	return Time{Hour: hour, Minute: minute, Second: second, Nanosecond: nanosecond}
}

// TimeFromTime returns the time-of-day portion of t, truncated to microsecond
// precision to match SQLite and PostgreSQL TIME storage.
func TimeFromTime(t time.Time) Time {
	return Time{
		Hour:       t.Hour(),
		Minute:     t.Minute(),
		Second:     t.Second(),
		Nanosecond: (t.Nanosecond() / 1000) * 1000,
	}
}

// ParseTime parses "HH:MM", "HH:MM:SS", or "HH:MM:SS.ffffff" (1 to 9
// fractional digits).
func ParseTime(s string) (Time, error) {
	s = strings.TrimSpace(s)
	parts := strings.Split(s, ":")
	if len(parts) < 2 || len(parts) > 3 {
		return Time{}, fmt.Errorf("orm: invalid time %q", s)
	}
	hour, err := strconv.Atoi(parts[0])
	if err != nil || hour < 0 || hour > 23 {
		return Time{}, fmt.Errorf("orm: invalid time %q", s)
	}
	minute, err := strconv.Atoi(parts[1])
	if err != nil || minute < 0 || minute > 59 {
		return Time{}, fmt.Errorf("orm: invalid time %q", s)
	}
	second, nano := 0, 0
	if len(parts) == 3 {
		secStr, fracStr, hasFrac := strings.Cut(parts[2], ".")
		second, err = strconv.Atoi(secStr)
		if err != nil || second < 0 || second > 59 {
			return Time{}, fmt.Errorf("orm: invalid time %q", s)
		}
		if hasFrac {
			if fracStr == "" || len(fracStr) > 9 || strings.Trim(fracStr, "0123456789") != "" {
				return Time{}, fmt.Errorf("orm: invalid time %q", s)
			}
			padded := (fracStr + "000000000")[:9]
			nano, _ = strconv.Atoi(padded)
		}
	}
	return Time{Hour: hour, Minute: minute, Second: second, Nanosecond: nano}, nil
}

// MustTime parses s with ParseTime and panics on error.
func MustTime(s string) Time {
	t, err := ParseTime(s)
	if err != nil {
		panic(fmt.Sprintf("orm.MustTime(%q): %v", s, err))
	}
	return t
}

// IsZero reports whether t is midnight (00:00:00).
func (t Time) IsZero() bool {
	return t.Hour == 0 && t.Minute == 0 && t.Second == 0 && t.Nanosecond == 0
}

// String formats t as "HH:MM:SS" when Nanosecond is 0, or "HH:MM:SS.ffffff"
// otherwise, matching Python's datetime.time.__str__.
func (t Time) String() string {
	base := fmt.Sprintf("%02d:%02d:%02d", t.Hour, t.Minute, t.Second)
	if t.Nanosecond == 0 {
		return base
	}
	return fmt.Sprintf("%s.%06d", base, t.Nanosecond/1000)
}

// Value implements driver.Valuer.
func (t Time) Value() (driver.Value, error) {
	return t.String(), nil
}

// Scan implements sql.Scanner.
func (t *Time) Scan(src any) error {
	switch v := src.(type) {
	case nil:
		*t = Time{}
		return nil
	case time.Time:
		*t = TimeFromTime(v)
		return nil
	case string:
		parsed, err := ParseTime(v)
		if err != nil {
			return err
		}
		*t = parsed
		return nil
	case []byte:
		parsed, err := ParseTime(string(v))
		if err != nil {
			return err
		}
		*t = parsed
		return nil
	default:
		return fmt.Errorf("orm: cannot scan %T into Time", src)
	}
}

// MarshalText implements encoding.TextMarshaler.
func (t Time) MarshalText() ([]byte, error) { return []byte(t.String()), nil }

// UnmarshalText implements encoding.TextUnmarshaler.
func (t *Time) UnmarshalText(text []byte) error {
	parsed, err := ParseTime(string(text))
	if err != nil {
		return err
	}
	*t = parsed
	return nil
}

// FormatPGInterval formats d as an exact PostgreSQL INTERVAL input string in
// microseconds, preserving sign and sub-second precision without day/month
// ambiguity.
func FormatPGInterval(d time.Duration) string {
	return strconv.FormatInt(int64(d/time.Microsecond), 10) + " microseconds"
}

// ParsePGInterval parses a PostgreSQL INTERVAL string (postgres IntervalStyle,
// such as "1 day 01:02:03.450000", "-00:00:01", or "2499999:59:59.999999") into
// a time.Duration.
func ParsePGInterval(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, errors.New("orm: empty interval string")
	}
	tokens := strings.Fields(s)
	var totalMicros int64
	for i := 0; i < len(tokens); i++ {
		tok := tokens[i]
		if strings.Contains(tok, ":") {
			us, err := parsePGIntervalClock(tok)
			if err != nil {
				return 0, err
			}
			totalMicros += us
			continue
		}
		if i+1 >= len(tokens) {
			return 0, fmt.Errorf("orm: invalid PostgreSQL interval %q", s)
		}
		n, err := strconv.ParseInt(tok, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("orm: invalid PostgreSQL interval %q: %w", s, err)
		}
		unit := strings.ToLower(tokens[i+1])
		i++
		switch unit {
		case "day", "days":
			totalMicros += n * 86400 * 1_000_000
		case "mon", "mons", "month", "months":
			totalMicros += n * 30 * 86400 * 1_000_000
		case "year", "years":
			totalMicros += n * 365 * 86400 * 1_000_000
		default:
			return 0, fmt.Errorf("orm: unsupported PostgreSQL interval unit %q in %q", unit, s)
		}
	}
	return time.Duration(totalMicros) * time.Microsecond, nil
}

// parsePGIntervalClock parses "[+-]H+:MM:SS[.ffffff]" into signed microseconds.
func parsePGIntervalClock(tok string) (int64, error) {
	neg := false
	switch {
	case strings.HasPrefix(tok, "-"):
		neg = true
		tok = tok[1:]
	case strings.HasPrefix(tok, "+"):
		tok = tok[1:]
	}
	parts := strings.Split(tok, ":")
	if len(parts) != 3 {
		return 0, fmt.Errorf("orm: invalid PostgreSQL interval clock %q", tok)
	}
	hours, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("orm: invalid PostgreSQL interval clock %q", tok)
	}
	minutes, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("orm: invalid PostgreSQL interval clock %q", tok)
	}
	secStr, fracStr, hasFrac := strings.Cut(parts[2], ".")
	seconds, err := strconv.ParseInt(secStr, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("orm: invalid PostgreSQL interval clock %q", tok)
	}
	var micros int64
	if hasFrac {
		if fracStr == "" || strings.Trim(fracStr, "0123456789") != "" {
			return 0, fmt.Errorf("orm: invalid PostgreSQL interval clock %q", tok)
		}
		padded := (fracStr + "000000")[:6]
		micros, _ = strconv.ParseInt(padded, 10, 64)
	}
	total := (hours*3600+minutes*60+seconds)*1_000_000 + micros
	if neg {
		total = -total
	}
	return total, nil
}
