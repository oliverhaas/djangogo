package orm

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/oliverhaas/djangogo/validators"
)

// The exprCall functions of Extract and the Trunc family. The value of the
// Expr is the lookup name of Extract and the kind of Trunc.
const (
	fnExtract   = "EXTRACT"
	fnTrunc     = "TRUNC"
	fnTruncDate = "TRUNC_DATE"
	fnTruncTime = "TRUNC_TIME"
)

// datetimeCall builds the call fn of Extract or Trunc on arg, a field key or
// an Expr, with the lookup name or the kind name.
func datetimeCall(fn string, arg any, name string) Expr {
	return Expr{kind: exprCall, fn: fn, args: []Expr{fieldOrExpr(arg)}, value: name}
}

// Extract returns the part lookupName of a date, a datetime, a time of day
// or a duration, Django's Extract. arg is a field key or an Expr. The parts
// are the date and time transforms: "year", "iso_year", "quarter", "month",
// "week", "week_day" (Sunday = 1), "iso_week_day" (Monday = 1), "day",
// "hour", "minute" and "second". The value is an int64, and nil when arg is
// NULL:
//
//	Annotate("year", orm.Extract("pub_date", "year"))
//	Filter("start_datetime__year", orm.Extract("end_datetime", "year"))
//
// A datetime is read in TransformTimeZone, or in the zone of In. A date has
// no hour, minute or second, and a duration has only the day and the time
// parts; SQLite cannot extract from a duration. A wrong part or argument
// makes the query fail with Django's message.
func Extract(arg any, lookupName string) Expr { return datetimeCall(fnExtract, arg, lookupName) }

// ExtractYear is Extract(arg, "year"), Django's ExtractYear.
func ExtractYear(arg any) Expr { return Extract(arg, transformYear) }

// ExtractIsoYear is Extract(arg, "iso_year"), Django's ExtractIsoYear: the
// year of the ISO 8601 week.
func ExtractIsoYear(arg any) Expr { return Extract(arg, transformIsoYear) }

// ExtractMonth is Extract(arg, "month"), Django's ExtractMonth.
func ExtractMonth(arg any) Expr { return Extract(arg, transformMonth) }

// ExtractDay is Extract(arg, "day"), Django's ExtractDay.
func ExtractDay(arg any) Expr { return Extract(arg, transformDay) }

// ExtractWeek is Extract(arg, "week"), Django's ExtractWeek: the ISO 8601
// week number.
func ExtractWeek(arg any) Expr { return Extract(arg, transformWeek) }

// ExtractWeekDay is Extract(arg, "week_day"), Django's ExtractWeekDay:
// Sunday is 1 and Saturday is 7.
func ExtractWeekDay(arg any) Expr { return Extract(arg, transformWeekDay) }

// ExtractIsoWeekDay is Extract(arg, "iso_week_day"), Django's
// ExtractIsoWeekDay: Monday is 1 and Sunday is 7.
func ExtractIsoWeekDay(arg any) Expr { return Extract(arg, transformIsoWeekDay) }

// ExtractQuarter is Extract(arg, "quarter"), Django's ExtractQuarter.
func ExtractQuarter(arg any) Expr { return Extract(arg, transformQuarter) }

// ExtractHour is Extract(arg, "hour"), Django's ExtractHour.
func ExtractHour(arg any) Expr { return Extract(arg, transformHour) }

// ExtractMinute is Extract(arg, "minute"), Django's ExtractMinute.
func ExtractMinute(arg any) Expr { return Extract(arg, transformMinute) }

// ExtractSecond is Extract(arg, "second"), Django's ExtractSecond. The
// fraction of the second is cut off.
func ExtractSecond(arg any) Expr { return Extract(arg, transformSecond) }

// Trunc truncates a date, a datetime or a time of day to kind, Django's
// Trunc: "year", "quarter", "month", "week" (the Monday), "day", "hour",
// "minute" or "second". arg is a field key or an Expr. The value has the
// kind of arg, unless Output gives another: a time.Time for a datetime, a
// Date for a date and a Time for a time of day, and nil when arg is NULL:
//
//	Annotate("month", orm.Trunc("pub_date", "month"))
//	Annotate("day", orm.Trunc("start_datetime", "day").Output(orm.KindDate))
//
// A datetime is truncated in TransformTimeZone, or in the zone of In, and the
// value is the instant the truncated wall time has in that zone. A date
// cannot be truncated to a time part and a time of day not to a date part.
func Trunc(arg any, kind string) Expr { return datetimeCall(fnTrunc, arg, kind) }

// TruncYear is Trunc(arg, "year"), Django's TruncYear.
func TruncYear(arg any) Expr { return Trunc(arg, transformYear) }

// TruncQuarter is Trunc(arg, "quarter"), Django's TruncQuarter.
func TruncQuarter(arg any) Expr { return Trunc(arg, transformQuarter) }

// TruncMonth is Trunc(arg, "month"), Django's TruncMonth.
func TruncMonth(arg any) Expr { return Trunc(arg, transformMonth) }

// TruncWeek is Trunc(arg, "week"), Django's TruncWeek: midnight on the
// Monday of the week.
func TruncWeek(arg any) Expr { return Trunc(arg, transformWeek) }

// TruncDay is Trunc(arg, "day"), Django's TruncDay.
func TruncDay(arg any) Expr { return Trunc(arg, transformDay) }

// TruncHour is Trunc(arg, "hour"), Django's TruncHour.
func TruncHour(arg any) Expr { return Trunc(arg, transformHour) }

// TruncMinute is Trunc(arg, "minute"), Django's TruncMinute.
func TruncMinute(arg any) Expr { return Trunc(arg, transformMinute) }

// TruncSecond is Trunc(arg, "second"), Django's TruncSecond.
func TruncSecond(arg any) Expr { return Trunc(arg, transformSecond) }

// TruncDate returns the date of a datetime, Django's TruncDate: the SQL of
// the "date" transform, in TransformTimeZone or the zone of In. The value is
// a Date. A date is returned as it is.
func TruncDate(arg any) Expr { return datetimeCall(fnTruncDate, arg, transformDate) }

// TruncTime returns the time of day of a datetime, Django's TruncTime: the
// SQL of the "time" transform. The value is a Time. A time of day is
// returned as it is.
func TruncTime(arg any) Expr { return datetimeCall(fnTruncTime, arg, transformTime) }

// Output gives a Trunc function the kind of its value, Django's output_field
// of Trunc: KindDateTime, KindDate or KindTime. A datetime truncated to a
// date is read as a Date:
//
//	orm.Trunc("start_datetime", "year").Output(orm.KindDate)
//
// TruncDate and TruncTime keep their own kind, as in Django, but check it
// against the argument. On any other expression the query fails.
func (e Expr) Output(k Kind) Expr {
	e.output, e.hasOutput = k, true
	return e
}

// In gives an Extract or Trunc function the time zone that a datetime is
// read in, Django's tzinfo:
//
//	melb, _ := time.LoadLocation("Australia/Melbourne")
//	orm.ExtractHour("start_datetime").In(melb)
//
// The zone is passed to the database by its name, so it must be a zone of
// the IANA database, as time.LoadLocation returns it, or have a fixed
// offset, as time.FixedZone gives it. On any other expression, and on an
// argument that is not a datetime, the query fails.
func (e Expr) In(tz *time.Location) Expr {
	e.zone = tz
	return e
}

// datetimeInputKinds are the kinds Extract reads; Trunc reads the first
// three.
var datetimeInputKinds = []Kind{KindDate, KindDateTime, KindTime, KindDuration}

// lookupNamePattern matches the lookup name of Extract that the SQL can
// hold, Django's DatabaseOperations._extract_format_re of PostgreSQL. It
// keeps a name such as "day' FROM ...)) OR 1=1" out of the query.
var lookupNamePattern = regexp.MustCompile(`^[A-Za-z_]+$`)

// errTimeZoneField refuses In on an argument that is not a datetime.
var errTimeZoneField = errors.New("orm: tzinfo can only be used with DateTimeField.") //nolint:revive,staticcheck // Django's message

// resolveDatetime binds Extract and the Trunc functions after Django's checks
// of Extract.resolve_expression and TruncBase.resolve_expression.
func (c *compiler) resolveDatetime(e Expr) (sqlNode, error) {
	name, _ := e.value.(string)
	if e.fn == fnExtract && name == "" {
		return nil, errors.New("orm: lookup_name must be provided")
	}
	arg, err := c.resolveExpr(e.args[0])
	if err != nil {
		return nil, err
	}
	input := c.annotationKind(e.args[0], arg)
	field, named := nodeFieldName(arg)
	tzname := ""
	if e.zone != nil {
		if tzname, err = timeZoneName(e.zone); err != nil {
			return nil, err
		}
	}
	if e.fn == fnExtract {
		return c.resolveExtract(name, arg, input, field, tzname)
	}
	return c.resolveTrunc(e, arg, input, field, named, tzname)
}

// resolveExtract binds Extract of part name from arg, a value of kind input.
func (c *compiler) resolveExtract(name string, arg sqlNode, input Kind, field, tzname string) (sqlNode, error) {
	if !slices.Contains(datetimeInputKinds, input) {
		return nil, errors.New("orm: Extract input expression must be DateField, DateTimeField, TimeField, or DurationField.") //nolint:revive,staticcheck // Django's message
	}
	if input == KindDate && slices.Contains(timeTransforms, name) {
		return nil, fmt.Errorf("orm: Cannot extract time component '%s' from DateField '%s'.", name, field) //nolint:revive,staticcheck // Django's message
	}
	if input == KindDuration && (slices.Contains(dateTransforms, name) && name != transformDay) {
		return nil, fmt.Errorf("orm: Cannot extract component '%s' from DurationField '%s'.", name, field) //nolint:revive,staticcheck // Django's message
	}
	if tzname != "" && input != KindDateTime {
		return nil, errTimeZoneField
	}
	td, ok := c.d.(TransformDialect)
	if ok {
		_, _, ok = td.TransformSQL(transformDay, input, TransformTimeZone)
	}
	if !ok && input == KindDuration {
		return nil, errors.New("orm: Extract requires native DurationField database support.") //nolint:revive,staticcheck // Django's message
	}
	if !lookupNamePattern.MatchString(name) {
		return nil, fmt.Errorf("orm: Invalid lookup type: %s", validators.PyRepr(strings.ToUpper(name))) //nolint:revive,staticcheck // Django's message
	}
	if ok {
		_, _, ok = td.TransformSQL(name, input, TransformTimeZone)
	}
	if !ok {
		return nil, fmt.Errorf("orm: the %s dialect does not support the %s transform", c.d.Name(), name)
	}
	return transformNode{name: name, inner: arg, input: input, tzname: tzname}, nil
}

// resolveTrunc binds a Trunc function e of arg, a value of kind input.
func (c *compiler) resolveTrunc(e Expr, arg sqlNode, input Kind, field string, named bool, tzname string) (sqlNode, error) {
	kind, _ := e.value.(string)
	if !slices.Contains(datetimeInputKinds[:3], input) {
		name := "None"
		if named {
			name = validators.PyRepr(field)
		}
		return nil, fmt.Errorf("orm: %s isn't a DateField, TimeField, or DateTimeField.", name) //nolint:revive,staticcheck // Django's message
	}
	// classOutput is the output_field of the class of TruncDate and
	// TruncTime, zero for Trunc.
	var classOutput Kind
	switch e.fn {
	case fnTruncDate:
		classOutput = KindDate
	case fnTruncTime:
		classOutput = KindTime
	}
	output := input
	switch {
	case e.hasOutput:
		output = e.output
	case classOutput != 0:
		output = classOutput
	}
	if !slices.Contains(datetimeInputKinds[:3], output) {
		return nil, errors.New("orm: output_field must be either DateField, TimeField, or DateTimeField")
	}
	check := output
	if classOutput != 0 {
		check = classOutput
	}
	target := "DateTimeField"
	if classOutput != 0 || output != input {
		target = check.FieldClassName()
	}
	switch {
	case input == KindDate && (check == KindDateTime || slices.Contains(timeTransforms, kind) || kind == transformTime):
		return nil, fmt.Errorf("orm: Cannot truncate DateField '%s' to %s.", field, target) //nolint:revive,staticcheck // Django's message
	case input == KindTime && (check == KindDateTime || slices.Contains(truncDateKinds, kind)):
		return nil, fmt.Errorf("orm: Cannot truncate TimeField '%s' to %s.", field, target) //nolint:revive,staticcheck // Django's message
	}
	if tzname != "" && input != KindDateTime {
		return nil, errTimeZoneField
	}
	if classOutput != 0 {
		// TruncDate and TruncTime cast, as Django's do, with the SQL of the
		// date and time transforms. A value that has the kind already is
		// returned as it is.
		if input == classOutput {
			return arg, nil
		}
		return transformNode{name: kind, inner: arg, input: input, tzname: tzname}, nil
	}
	if input != KindDateTime {
		tzname = ""
	} else if tzname == "" {
		tzname = TransformTimeZone
	}
	td, ok := c.d.(TruncDialect)
	if ok {
		_, _, ok = td.TruncSQL(kind, input, output, tzname)
	}
	if !ok {
		return nil, fmt.Errorf("orm: the %s dialect does not support Trunc", c.d.Name())
	}
	return truncNode{kind: kind, inner: arg, input: input, output: output, tzname: tzname}, nil
}

// truncDateKinds are the kinds of Trunc that a time of day cannot be
// truncated to.
var truncDateKinds = []string{transformYear, transformQuarter, transformMonth, transformWeek, transformDay, transformDate}

// nodeFieldName returns the name of the field that n reads directly,
// Django's output_field.name, and false with "None" when n is no column.
func nodeFieldName(n sqlNode) (string, bool) {
	if r, ok := n.(outerRefNode); ok {
		n = r.inner
	}
	if col, ok := n.(colNode); ok && col.field != nil {
		return col.field.Attr, true
	}
	return "None", false
}

// TruncDialect is an optional capability of a Dialect: the SQL of Trunc.
// kind is the unit, such as "month"; input is the kind of the truncated
// expression and output the kind of the value, each KindDate, KindDateTime
// or KindTime. tzname is the zone a datetime is truncated in, and empty when
// input is not a datetime. The SQL is a template as for TransformDialect. A
// datetime value is the instant of the truncated wall time in tzname.
type TruncDialect interface {
	TruncSQL(kind string, input, output Kind, tzname string) (template string, params []any, ok bool)
}

// truncNode is a Trunc function of an expression.
type truncNode struct {
	kind  string
	inner sqlNode
	// input is the kind of inner and output the kind of the value.
	input, output Kind
	// tzname is the zone a datetime is truncated in.
	tzname string
}

// render writes the SQL that the dialect gives for the truncation.
func (n truncNode) render(w *sqlWriter) {
	td, _ := w.d.(TruncDialect)
	tmpl, params, _ := td.TruncSQL(n.kind, n.input, n.output, n.tzname)
	renderTemplate(w, tmpl, params, n.inner)
}

// vtype is the type of the value: a datetime, a date or a time of day.
func (n truncNode) vtype() valueType { return fieldValueType(&Field{Kind: n.output}) }

// aggregate reports whether the truncated expression holds an aggregate.
func (n truncNode) aggregate() bool { return n.inner.aggregate() }

// zoneProbes are the instants at which timeZoneName compares the offsets of
// two zones: winter and summer of a year in the past and of the year 2030.
var zoneProbes = []time.Time{
	time.Date(2000, 1, 15, 12, 0, 0, 0, time.UTC),
	time.Date(2000, 7, 15, 12, 0, 0, 0, time.UTC),
	time.Date(2030, 1, 15, 12, 0, 0, 0, time.UTC),
	time.Date(2030, 7, 15, 12, 0, 0, 0, time.UTC),
}

// timeZoneName returns the name of loc that the database is given, Django's
// timezone._get_timezone_name: the name of a zone of the IANA database, and
// "UTC+05:00" for a fixed offset, the name of Python's datetime.timezone. A
// zone of the IANA database whose name ends in an offset, such as
// "Etc/GMT+10", has a fixed offset and gets the offset form, which Django
// reaches through the abbreviation "-10" of the zone: the backends read the
// offset of a name with Django's split_tzname_delta.
func timeZoneName(loc *time.Location) (string, error) {
	name := loc.String()
	if name == "UTC" {
		return name, nil
	}
	if name != "Local" && name != "" {
		if _, _, offset := SplitTimeZoneName(name); offset == "" {
			if named, err := time.LoadLocation(name); err == nil && sameOffsets(loc, named) {
				return name, nil
			}
		}
	}
	_, offset := zoneProbes[0].In(loc).Zone()
	for _, p := range zoneProbes[1:] {
		if _, o := p.In(loc).Zone(); o != offset {
			return "", fmt.Errorf("orm: the time zone %q of In is neither a zone of the IANA database nor a fixed offset", name)
		}
	}
	if offset%60 != 0 {
		return "", fmt.Errorf("orm: the offset of the time zone %q of In is not a whole number of minutes", name)
	}
	if offset == 0 {
		return "UTC", nil
	}
	sign := "+"
	if offset < 0 {
		sign, offset = "-", -offset
	}
	return fmt.Sprintf("UTC%s%02d:%02d", sign, offset/3600, offset%3600/60), nil
}

// sameOffsets reports whether a and b have the same offset at each of
// zoneProbes, so that b, loaded by the name of a, is the zone a.
func sameOffsets(a, b *time.Location) bool {
	for _, p := range zoneProbes {
		_, oa := p.In(a).Zone()
		_, ob := p.In(b).Zone()
		if oa != ob {
			return false
		}
	}
	return true
}

// SplitTimeZoneName splits a time zone name that ends in an offset, Django's
// split_tzname_delta: "UTC+05:00" gives "UTC", "+" and "05:00", and "-10"
// gives "", "-" and "10:00". A name without an offset, such as
// "Asia/Ust-Nera", gives the name and two empty strings. The backends use it
// to convert to a zone of In that has a fixed offset.
func SplitTimeZoneName(tzname string) (name, sign, offset string) {
	for _, s := range []string{"+", "-"} {
		i := strings.LastIndex(tzname, s)
		if i < 0 {
			continue
		}
		if off := tzname[i+1:]; isTimeOfDay(off) {
			if !strings.Contains(off, ":") {
				off += ":00"
			}
			return tzname[:i], s, off
		}
	}
	return tzname, "", ""
}

// timeOfDayText matches the forms of a time of day that Django's parse_time
// reads: those of Python's time.fromisoformat ("10", "1000", "10:00:00")
// and those of its time_re ("1:5").
var timeOfDayText = regexp.MustCompile(`^(\d{2})(?::?(\d{2})(?::?(\d{2})(?:[.,]\d+)?)?)?$|^(\d{1,2}):(\d{1,2})(?::(\d{1,2})(?:[.,]\d{1,12})?)?$`)

// isTimeOfDay reports whether s is a valid time of day in a form of
// timeOfDayText.
func isTimeOfDay(s string) bool {
	m := timeOfDayText.FindStringSubmatch(s)
	if m == nil {
		return false
	}
	limits := []int{24, 60, 60}
	for _, group := range [][]string{m[1:4], m[4:7]} {
		for j, part := range group {
			if part == "" {
				continue
			}
			if n, _ := strconv.Atoi(part); n >= limits[j] {
				return false
			}
		}
	}
	return true
}
