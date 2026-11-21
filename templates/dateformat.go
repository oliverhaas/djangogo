package templates

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/flosch/pongo2/v6"

	"github.com/oliverhaas/djangogo/orm"
)

// The formats Django uses for the English locale, by setting name.
var namedDateFormats = map[string]string{
	"DATE_FORMAT":           "N j, Y",
	"DATETIME_FORMAT":       "N j, Y, P",
	"TIME_FORMAT":           "P",
	"SHORT_DATE_FORMAT":     "m/d/Y",
	"SHORT_DATETIME_FORMAT": "m/d/Y P",
	"YEAR_MONTH_FORMAT":     "F Y",
	"MONTH_DAY_FORMAT":      "F j",
}

// apMonths are the Associated Press month abbreviations of the format
// character N.
var apMonths = [...]string{
	"Jan.", "Feb.", "March", "April", "May", "June", "July", "Aug.", "Sept.", "Oct.", "Nov.", "Dec.",
}

// Localize returns a date or time value as a template prints it, Django's
// formats.localize for the English locale: a time.Time in DATETIME_FORMAT
// ("Dec. 25, 2024, 3:30 p.m."), an orm.Date in DATE_FORMAT ("Dec. 25, 2024",
// "" for the zero Date) and an orm.Time in TIME_FORMAT ("3:30 p.m."). ok is
// false for any other value.
func Localize(v any) (s string, ok bool) {
	switch x := v.(type) {
	case time.Time:
		return templateTime{x}.String(), true
	case orm.Date:
		return templateDate{x}.String(), true
	case orm.Time:
		return templateTimeOfDay{x}.String(), true
	}
	return "", false
}

// templateTime is the form a time.Time has inside a render. Printed bare it
// reads like Django prints a datetime ("Dec. 25, 2024, 3:30 p.m."), where Go's
// own String gives "2024-12-25 15:30:00 +0000 UTC". The methods of time.Time
// stay reachable from a template ({{ t.Year }}).
type templateTime struct {
	time.Time
}

// String formats the time with Django's DATETIME_FORMAT.
func (t templateTime) String() string { return formatDate(t.Time, namedDateFormats["DATETIME_FORMAT"]) }

// templateDate is the render-side view of orm.Date. Printed bare it uses
// Django's DATE_FORMAT ("Dec. 25, 2024") instead of ISO "2024-12-25", while
// keeping Year, Month, and Day reachable.
type templateDate struct {
	orm.Date
}

// String formats the date with Django's DATE_FORMAT, or "" for the zero Date.
func (d templateDate) String() string {
	if d.IsZero() {
		return ""
	}
	return formatDateOpts(d.Time(), namedDateFormats["DATE_FORMAT"], true, false, false)
}

// templateTimeOfDay is the render-side view of orm.Time. Printed bare it uses
// Django's TIME_FORMAT ("3:30 p.m.", "midnight", "noon") while keeping Hour,
// Minute, Second, and Nanosecond reachable.
type templateTimeOfDay struct {
	orm.Time
}

// String formats the time of day with Django's TIME_FORMAT.
func (t templateTimeOfDay) String() string {
	return formatDateOpts(timeOfDayAsTime(t.Time), namedDateFormats["TIME_FORMAT"], false, true, false)
}

// templateDuration is the render-side view of time.Duration. Printed bare it
// matches Python's str(datetime.timedelta) ("1:30:00", "2 days, 3:04:05",
// "-1 day, 23:58:30", "0:00:05.123456").
type templateDuration struct {
	time.Duration
}

// String formats d as Python's datetime.timedelta.__str__.
func (d templateDuration) String() string { return formatDurationPython(d.Duration) }

// formatDurationPython formats d as Python's str(datetime.timedelta), using
// floor division into (days, seconds, microseconds) with 0 <= seconds < 86400
// and 0 <= microseconds < 1_000_000.
func formatDurationPython(d time.Duration) string {
	const usPerDay = 86400 * 1_000_000
	totalUS := int64(d / time.Microsecond)
	days := totalUS / usPerDay
	rem := totalUS % usPerDay
	if rem < 0 {
		days--
		rem += usPerDay
	}
	secs := rem / 1_000_000
	micros := rem % 1_000_000
	hh := secs / 3600
	mm := (secs % 3600) / 60
	ss := secs % 60
	out := fmt.Sprintf("%d:%02d:%02d", hh, mm, ss)
	if days != 0 {
		plural := "s"
		if days == 1 || days == -1 {
			plural = ""
		}
		out = fmt.Sprintf("%d day%s, %s", days, plural, out)
	}
	if micros != 0 {
		out += fmt.Sprintf(".%06d", micros)
	}
	return out
}

// timeOfDayAsTime maps an orm.Time onto a neutral date in UTC for formatting.
func timeOfDayAsTime(t orm.Time) time.Time {
	return time.Date(2000, 1, 1, t.Hour, t.Minute, t.Second, t.Nanosecond, time.UTC)
}

// asTime returns the time a template value holds, accepting full timestamps
// and non-zero orm.Date values (at midnight UTC).
func asTime(v *pongo2.Value) (time.Time, bool) {
	switch t := v.Interface().(type) {
	case time.Time:
		return t, true
	case templateTime:
		return t.Time, true
	case *time.Time:
		if t != nil {
			return *t, true
		}
	case orm.Date:
		if !t.IsZero() {
			return t.Time(), true
		}
	case templateDate:
		if !t.IsZero() {
			return t.Time(), true
		}
	case *orm.Date:
		if t != nil && !t.IsZero() {
			return t.Time(), true
		}
	}
	return time.Time{}, false
}

// asDateOrTime extracts a temporal value and flags describing which components
// (date, time, zone) it carries.
func asDateOrTime(v *pongo2.Value) (tm time.Time, hasDate, hasTime, hasZone, ok bool) {
	switch t := v.Interface().(type) {
	case time.Time:
		return t, true, true, true, true
	case templateTime:
		return t.Time, true, true, true, true
	case *time.Time:
		if t != nil {
			return *t, true, true, true, true
		}
	case orm.Date:
		if !t.IsZero() {
			return t.Time(), true, false, false, true
		}
	case templateDate:
		if !t.IsZero() {
			return t.Time(), true, false, false, true
		}
	case *orm.Date:
		if t != nil && !t.IsZero() {
			return t.Time(), true, false, false, true
		}
	case orm.Time:
		return timeOfDayAsTime(t), false, true, false, true
	case templateTimeOfDay:
		return timeOfDayAsTime(t.Time), false, true, false, true
	case *orm.Time:
		if t != nil {
			return timeOfDayAsTime(*t), false, true, false, true
		}
	}
	return time.Time{}, false, false, false, false
}

// formatDate formats t with a Django date format string: a format name such
// as SHORT_DATE_FORMAT, or format characters as documented for Django's date
// filter (d, j, D, l, S, m, n, M, N, F, y, Y, H, G, h, g, i, s, a, A, f, P,
// c, r, U, ...). A backslash keeps the next character as it is.
//
// A Go time always carries a zone, so the zone characters (e, T, O, Z, I) and
// the offset in c behave as they do for an aware datetime in Django. The time
// is formatted in its own location.
func formatDate(t time.Time, format string) string {
	return formatDateOpts(t, format, true, true, true)
}

// isDateOnlySpecifier reports whether ch requires calendar date components
// that a bare orm.Time lacks.
func isDateOnlySpecifier(ch rune) bool {
	switch ch {
	case 'c', 'd', 'D', 'E', 'F', 'j', 'l', 'L', 'm', 'M', 'n', 'N', 'o', 'r', 'S', 't', 'U', 'w', 'W', 'y', 'Y', 'z':
		return true
	default:
		return false
	}
}

// isZoneSpecifier reports whether ch formats timezone information.
func isZoneSpecifier(ch rune) bool {
	switch ch {
	case 'e', 'I', 'O', 'T', 'Z':
		return true
	default:
		return false
	}
}

// formatDateOpts formats t with a Django format string, honoring which
// components (date, time, zone) the source value carries.
func formatDateOpts(t time.Time, format string, hasDate, hasTime, hasZone bool) string {
	if named, ok := namedDateFormats[format]; ok {
		format = named
	}
	var b strings.Builder
	escaped := false
	for _, ch := range format {
		if escaped {
			b.WriteRune(ch)
			escaped = false
			continue
		}
		if ch == '\\' {
			escaped = true
			continue
		}
		if !hasDate && isDateOnlySpecifier(ch) {
			return ""
		}
		if !hasZone && isZoneSpecifier(ch) {
			continue
		}
		if !hasTime && ch == 'c' {
			b.WriteString(t.Format("2006-01-02"))
			continue
		}
		writeDateChar(&b, t, ch)
	}
	return b.String()
}

// hour12 returns the hour on a 12-hour clock.
func hour12(t time.Time) int {
	if h := t.Hour() % 12; h != 0 {
		return h
	}
	return 12
}

// meridiem returns Django's "a.m." or "p.m.".
func meridiem(t time.Time) string {
	if t.Hour() < 12 {
		return "a.m."
	}
	return "p.m."
}

// ordinalSuffix returns the English ordinal suffix of a day of the month.
func ordinalSuffix(day int) string {
	if day >= 11 && day <= 13 {
		return "th"
	}
	switch day % 10 {
	case 1:
		return "st"
	case 2:
		return "nd"
	case 3:
		return "rd"
	}
	return "th"
}

// isLeap reports whether year is a leap year.
func isLeap(year int) bool { return year%4 == 0 && (year%100 != 0 || year%400 == 0) }

// daysIn returns the number of days of the month t lies in.
func daysIn(t time.Time) int {
	return time.Date(t.Year(), t.Month()+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

// utcOffset formats the zone offset of t as +HHMM, or +HH:MM with a colon.
func utcOffset(t time.Time, colon bool) string {
	_, off := t.Zone()
	sign := '+'
	if off < 0 {
		sign = '-'
		off = -off
	}
	if colon {
		return fmt.Sprintf("%c%02d:%02d", sign, off/3600, off%3600/60)
	}
	return fmt.Sprintf("%c%02d%02d", sign, off/3600, off%3600/60)
}

// writeDateChar writes the expansion of one format character; a character
// without a meaning is written as it is.
func writeDateChar(b *strings.Builder, t time.Time, ch rune) {
	pad2 := func(n int) { fmt.Fprintf(b, "%02d", n) }
	switch ch {
	case 'a':
		b.WriteString(meridiem(t))
	case 'A':
		b.WriteString(t.Format("PM"))
	case 'b':
		b.WriteString(strings.ToLower(t.Format("Jan")))
	case 'c':
		b.WriteString(t.Format("2006-01-02T15:04:05"))
		if us := t.Nanosecond() / 1000; us != 0 {
			fmt.Fprintf(b, ".%06d", us)
		}
		b.WriteString(utcOffset(t, true))
	case 'd':
		pad2(t.Day())
	case 'D':
		b.WriteString(t.Format("Mon"))
	case 'e', 'T':
		name, _ := t.Zone()
		b.WriteString(name)
	case 'E', 'F':
		b.WriteString(t.Month().String())
	case 'f':
		b.WriteString(strconv.Itoa(hour12(t)))
		if t.Minute() != 0 {
			fmt.Fprintf(b, ":%02d", t.Minute())
		}
	case 'g':
		b.WriteString(strconv.Itoa(hour12(t)))
	case 'G':
		b.WriteString(strconv.Itoa(t.Hour()))
	case 'h':
		pad2(hour12(t))
	case 'H':
		pad2(t.Hour())
	case 'i':
		pad2(t.Minute())
	case 'I':
		if t.IsDST() {
			b.WriteByte('1')
		} else {
			b.WriteByte('0')
		}
	case 'j':
		b.WriteString(strconv.Itoa(t.Day()))
	case 'l':
		b.WriteString(t.Weekday().String())
	case 'L':
		if isLeap(t.Year()) {
			b.WriteString("True")
		} else {
			b.WriteString("False")
		}
	case 'm':
		pad2(int(t.Month()))
	case 'M':
		b.WriteString(t.Format("Jan"))
	case 'n':
		b.WriteString(strconv.Itoa(int(t.Month())))
	case 'N':
		b.WriteString(apMonths[t.Month()-1])
	case 'o':
		year, _ := t.ISOWeek()
		b.WriteString(strconv.Itoa(year))
	case 'O':
		b.WriteString(utcOffset(t, false))
	case 'P':
		switch {
		case t.Minute() == 0 && t.Hour() == 0:
			b.WriteString("midnight")
		case t.Minute() == 0 && t.Hour() == 12:
			b.WriteString("noon")
		default:
			writeDateChar(b, t, 'f')
			b.WriteByte(' ')
			b.WriteString(meridiem(t))
		}
	case 'r':
		b.WriteString(t.Format(time.RFC1123Z))
	case 's':
		pad2(t.Second())
	case 'S':
		b.WriteString(ordinalSuffix(t.Day()))
	case 't':
		b.WriteString(strconv.Itoa(daysIn(t)))
	case 'u':
		fmt.Fprintf(b, "%06d", t.Nanosecond()/1000)
	case 'U':
		b.WriteString(strconv.FormatInt(t.Unix(), 10))
	case 'w':
		b.WriteString(strconv.Itoa(int(t.Weekday())))
	case 'W':
		_, week := t.ISOWeek()
		b.WriteString(strconv.Itoa(week))
	case 'y':
		pad2(t.Year() % 100)
	case 'Y':
		fmt.Fprintf(b, "%04d", t.Year())
	case 'z':
		b.WriteString(strconv.Itoa(t.YearDay()))
	case 'Z':
		_, off := t.Zone()
		b.WriteString(strconv.Itoa(off))
	default:
		b.WriteRune(ch)
	}
}

// filterDate implements Django's date filter for time.Time, orm.Date, and
// orm.Time values. Non-temporal values return an empty string.
func filterDate(in, param *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	t, hasDate, hasTime, hasZone, ok := asDateOrTime(in)
	if !ok {
		return pongo2.AsValue(""), nil
	}
	format := "DATE_FORMAT"
	if !param.IsNil() && param.String() != "" {
		format = param.String()
	}
	return pongo2.AsValue(formatDateOpts(t, format, hasDate, hasTime, hasZone)), nil
}

// filterTime implements Django's time filter for time.Time and orm.Time
// values. Calendar-only orm.Date and non-temporal values return "".
func filterTime(in, param *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	t, hasDate, hasTime, hasZone, ok := asDateOrTime(in)
	if !ok || !hasTime {
		return pongo2.AsValue(""), nil
	}
	format := "TIME_FORMAT"
	if !param.IsNil() && param.String() != "" {
		format = param.String()
	}
	return pongo2.AsValue(formatDateOpts(t, format, hasDate, hasTime, hasZone)), nil
}

// nbsp is the no-break space (U+00A0) Django puts between a number and its
// unit.
const nbsp = string(rune(0xa0))

// sinceUnits are the units of timesince, largest first, with the length in
// seconds of the ones that have a fixed length.
var sinceUnits = [...]struct {
	name    string
	seconds int64
}{
	{"year", 0}, {"month", 0}, {"week", 7 * 24 * 3600}, {"day", 24 * 3600}, {"hour", 3600}, {"minute", 60},
}

// timeSince describes the time from d until now in at most two adjacent
// units ("2 weeks, 3 days"), Django's timesince. Years and months are counted
// on the calendar, the rest in fixed lengths. A time that is not in the past
// gives "0 minutes".
func timeSince(d, now time.Time) string {
	now = now.In(d.Location())
	if now.Unix()-d.Unix() <= 0 {
		return "0" + nbsp + "minutes"
	}
	months := (now.Year()-d.Year())*12 + int(now.Month()) - int(d.Month())
	dClock := d.Hour()*3600 + d.Minute()*60 + d.Second()
	nowClock := now.Hour()*3600 + now.Minute()*60 + now.Second()
	if d.Day() > now.Day() || (d.Day() == now.Day() && dClock > nowClock) {
		months--
	}
	years := months / 12
	months %= 12

	pivot := d
	if years > 0 || months > 0 {
		py, pm := d.Year()+years, int(d.Month())+months
		if pm > 12 {
			pm -= 12
			py++
		}
		// Django clamps the day to the month length of a common year.
		day := min(d.Day(), daysIn(time.Date(2001, time.Month(pm), 1, 0, 0, 0, 0, time.UTC)))
		pivot = time.Date(py, time.Month(pm), day, d.Hour(), d.Minute(), d.Second(), 0, d.Location())
	}
	remaining := now.Unix() - pivot.Unix()
	counts := [len(sinceUnits)]int64{int64(years), int64(months)}
	for i := 2; i < len(sinceUnits); i++ {
		counts[i] = remaining / sinceUnits[i].seconds
		remaining -= counts[i] * sinceUnits[i].seconds
	}

	first := 0
	for first < len(counts) && counts[first] == 0 {
		first++
	}
	if first == len(counts) {
		return "0" + nbsp + "minutes"
	}
	var parts []string
	for i := first; i < len(counts) && len(parts) < 2 && counts[i] != 0; i++ {
		unit := sinceUnits[i].name
		if counts[i] != 1 {
			unit += "s"
		}
		parts = append(parts, strconv.FormatInt(counts[i], 10)+nbsp+unit)
	}
	return strings.Join(parts, ", ")
}

// filterTimesince implements {{ date|timesince }} and
// {{ date|timesince:other }}: the time from date until now, or until other.
func filterTimesince(in, param *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	d, ok := asTime(in)
	if !ok {
		return pongo2.AsValue(""), nil
	}
	now, ok := asTime(param)
	if !ok {
		now = timeNow()
	}
	return pongo2.AsValue(timeSince(d, now)), nil
}

// filterTimeuntil implements {{ date|timeuntil }} and
// {{ date|timeuntil:other }}: the time from now, or from other, until date.
func filterTimeuntil(in, param *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	d, ok := asTime(in)
	if !ok {
		return pongo2.AsValue(""), nil
	}
	from, ok := asTime(param)
	if !ok {
		from = timeNow()
	}
	return pongo2.AsValue(timeSince(from, d)), nil
}
