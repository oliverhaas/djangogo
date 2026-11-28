package forms

import (
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/oliverhaas/djangogo/internal/duration"
)

// strftimeLayouts maps the strftime directives a form uses to Go layout
// elements. The numeric elements are the unpadded ones, which parse both "5"
// and "05" as strptime does; formatting pads them where strftime would.
var strftimeLayouts = map[byte]string{
	'Y': "2006", 'y': "06", 'm': "01", 'd': "02", 'H': "15", 'I': "03", 'M': "04", 'S': "05",
	'f': "000000", 'b': "Jan", 'B': "January", 'a': "Mon", 'A': "Monday", 'p': "PM",
	'z': "-0700", 'Z': "MST", '%': "%",
}

// goLayout converts a strftime format to a Go layout. A format without "%" is
// taken to be a Go layout already.
func goLayout(format string) string {
	if !strings.Contains(format, "%") {
		return format
	}
	var b strings.Builder
	for i := 0; i < len(format); i++ {
		if format[i] != '%' || i+1 >= len(format) {
			b.WriteByte(format[i])
			continue
		}
		i++
		if layout, ok := strftimeLayouts[format[i]]; ok {
			b.WriteString(layout)
		} else {
			b.WriteString("%" + string(format[i]))
		}
	}
	return b.String()
}

// parseLayout is goLayout for parsing: the padded numeric elements become the
// unpadded ones, so that "1/2/2006" parses under "%m/%d/%Y" and "2:30 PM"
// under "%I:%M %p", as strptime allows.
func parseLayout(format string) string {
	layout := goLayout(format)
	if !strings.Contains(format, "%") {
		return layout
	}
	// ".999999" takes any number of fractional digits, as %f takes one to six.
	return strings.NewReplacer("01", "1", "02", "2", "03", "3", "15", "15", "04", "4", "05", "5", "2006", "2006",
		"000000", "999999").Replace(layout)
}

// Default input formats, Django's DATE_INPUT_FORMATS, TIME_INPUT_FORMATS and
// DATETIME_INPUT_FORMATS for the "en" locale. Fractional seconds need no
// format of their own: Go parses them after any seconds element.
var (
	dateInputFormats = []string{
		"%Y-%m-%d", "%m/%d/%Y", "%m/%d/%y", "%b %d %Y", "%b %d, %Y", "%d %b %Y", "%d %b, %Y",
		"%B %d %Y", "%B %d, %Y", "%d %B %Y", "%d %B, %Y",
	}
	timeInputFormats = []string{"%H:%M:%S", "%H:%M"}
	// A DateTimeField also accepts the date formats, as Django's
	// DateTimeFormatsIterator yields both lists.
	dateTimeInputFormats = append([]string{
		"%Y-%m-%d %H:%M:%S", "%Y-%m-%d %H:%M", "%m/%d/%Y %H:%M:%S", "%m/%d/%Y %H:%M",
		"%m/%d/%y %H:%M:%S", "%m/%d/%y %H:%M",
	}, dateInputFormats...)
	// isoDateTimeRE is Django's datetime_re, extended by the date-only form
	// that Python's datetime.fromisoformat accepts.
	isoDateTimeRE = regexp.MustCompile(`^(\d{4})-(\d{1,2})-(\d{1,2})(?:[T ](\d{1,2}):(\d{1,2})` +
		`(?::(\d{1,2})(?:[.,](\d{1,6})\d{0,6})?)?\s*(Z|[+-]\d{2}(?::?\d{2})?)?)?$`)
)

// parseISODateTime parses an ISO 8601 date and time, Django's parse_datetime.
// A value without an offset is read as UTC.
func parseISODateTime(s string) (time.Time, bool) {
	m := isoDateTimeRE.FindStringSubmatch(s)
	if m == nil {
		return time.Time{}, false
	}
	num := func(i int) int {
		n, _ := strconv.Atoi(m[i])
		return n
	}
	nanos := 0
	if m[7] != "" {
		nanos, _ = strconv.Atoi((m[7] + "00000000")[:9])
	}
	loc := time.UTC
	if tz := m[8]; tz != "" && tz != "Z" {
		digits := strings.ReplaceAll(tz[1:], ":", "")
		hours, _ := strconv.Atoi(digits[:2])
		minutes := 0
		if len(digits) == 4 {
			minutes, _ = strconv.Atoi(digits[2:])
		}
		offset := hours*3600 + minutes*60
		if tz[0] == '-' {
			offset = -offset
		}
		loc = time.FixedZone("", offset)
	}
	year, month, day, hour, minute, second := num(1), num(2), num(3), num(4), num(5), num(6)
	t := time.Date(year, time.Month(month), day, hour, minute, second, nanos, loc)
	// time.Date normalizes out-of-range parts (February 30 becomes March 2);
	// Python rejects them.
	if t.Month() != time.Month(month) || t.Day() != day || t.Hour() != hour || t.Minute() != minute ||
		t.Second() != second {
		return time.Time{}, false
	}
	return t, true
}

// parseTemporal tries each format in turn and returns the first time that
// parses, in UTC unless the value carries an offset.
func parseTemporal(s string, formats []string) (time.Time, bool) {
	for _, format := range formats {
		if t, err := time.Parse(parseLayout(format), s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// formatDateTime renders t as Python's str(datetime) does.
func formatDateTime(t time.Time) string {
	layout := "2006-01-02 15:04:05"
	if t.Nanosecond() != 0 {
		layout += ".000000"
	}
	if t.Location() != time.UTC {
		layout += "-07:00"
	}
	return t.Format(layout)
}

// temporalField is the common part of the date and time fields.
type temporalField struct {
	BaseField
	// InputFormats are the accepted layouts, tried in order.
	InputFormats []string
}

// parse converts a submitted value with the field's formats.
func (f *temporalField) parse(value any) (any, error) {
	if isEmpty(value) {
		return nil, nil
	}
	if t, ok := value.(time.Time); ok {
		return t, nil
	}
	if t, ok := parseTemporal(strings.TrimSpace(pyString(value)), f.InputFormats); ok {
		return t, nil
	}
	return nil, f.Error("invalid")
}

// temporalValidators returns the validators of the min and max options.
func temporalValidators(o *fieldOptions) []Validator {
	var out []Validator
	if o.maxValue != nil {
		out = append(out, MaxValueValidator(o.maxValue))
	}
	if o.minValue != nil {
		out = append(out, MinValueValidator(o.minValue))
	}
	return out
}

// formatsOr returns the configured input formats or the defaults.
func formatsOr(o *fieldOptions, defaults []string) []string {
	if len(o.inputFormats) > 0 {
		return o.inputFormats
	}
	return defaults
}

// DateField is a date field; its cleaned value is a time.Time at midnight UTC,
// or nil when empty.
type DateField struct {
	temporalField
}

// NewDateField returns a DateField. It reads the options InputFormats,
// MinValue and MaxValue beside the common ones.
func NewDateField(opts ...FieldOption) *DateField {
	o := collectOptions(opts)
	f := &DateField{}
	f.InputFormats = formatsOr(o, dateInputFormats)
	f.init(f, DateInput{}, map[string]string{"invalid": "Enter a valid date."}, o)
	f.Validators = append(temporalValidators(o), f.Validators...)
	return f
}

// ToGo parses a date; the time of day of a time.Time is dropped.
func (f *DateField) ToGo(value any) (any, error) {
	v, err := f.parse(value)
	if t, ok := v.(time.Time); ok {
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC), nil
	}
	return v, err
}

// TimeField is a time-of-day field; its cleaned value is a time.Time on
// January 1 of year 0 in UTC, or nil when empty.
type TimeField struct {
	temporalField
}

// NewTimeField returns a TimeField. It reads the option InputFormats beside
// the common ones.
func NewTimeField(opts ...FieldOption) *TimeField {
	o := collectOptions(opts)
	f := &TimeField{}
	f.InputFormats = formatsOr(o, timeInputFormats)
	f.init(f, TimeInput{}, map[string]string{"invalid": "Enter a valid time."}, o)
	return f
}

// ToGo parses a time of day; the date of a time.Time is dropped.
func (f *TimeField) ToGo(value any) (any, error) {
	v, err := f.parse(value)
	if t, ok := v.(time.Time); ok {
		return time.Date(0, 1, 1, t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), time.UTC), nil
	}
	return v, err
}

// DateTimeField is a date and time field; its cleaned value is a time.Time, or
// nil when empty. A value without an offset is read as UTC.
type DateTimeField struct {
	temporalField
}

// NewDateTimeField returns a DateTimeField. It reads the options InputFormats,
// MinValue and MaxValue beside the common ones.
func NewDateTimeField(opts ...FieldOption) *DateTimeField {
	o := collectOptions(opts)
	f := &DateTimeField{}
	f.InputFormats = formatsOr(o, dateTimeInputFormats)
	f.init(f, DateTimeInput{}, map[string]string{"invalid": "Enter a valid date/time."}, o)
	f.Validators = append(temporalValidators(o), f.Validators...)
	return f
}

// PrepareValue shows a time.Time in UTC, the current time zone of the
// package, as Django's to_current_timezone does: a database driver may return
// it in the local time zone.
func (f *DateTimeField) PrepareValue(value any) any {
	if t, ok := value.(time.Time); ok {
		return t.UTC()
	}
	return value
}

// ToGo parses a date and time. ISO 8601 is tried first, as the values of a
// datetime-local input and of an API client have that form.
func (f *DateTimeField) ToGo(value any) (any, error) {
	if s, ok := value.(string); ok {
		if t, ok := parseISODateTime(strings.TrimSpace(s)); ok {
			return t, nil
		}
	}
	return f.parse(value)
}

// DurationField is a duration field; its cleaned value is a time.Duration, or
// nil when empty. It accepts Django's format "[days] [[hours:]minutes:]seconds"
// and ISO 8601 durations such as "P1DT2H".
type DurationField struct {
	BaseField
}

// NewDurationField returns a DurationField.
func NewDurationField(opts ...FieldOption) *DurationField {
	f := &DurationField{}
	f.init(f, TextInput{}, map[string]string{
		"invalid":  "Enter a valid duration.",
		"overflow": "The number of days must be between %(min_days)s and %(max_days)s.",
	}, collectOptions(opts))
	return f
}

// PrepareValue formats a time.Duration for display.
func (f *DurationField) PrepareValue(value any) any {
	if d, ok := value.(time.Duration); ok {
		return duration.String(d)
	}
	return value
}

// ToGo parses a duration.
func (f *DurationField) ToGo(value any) (any, error) {
	if isEmpty(value) {
		return nil, nil
	}
	if d, ok := value.(time.Duration); ok {
		return d, nil
	}
	d, ok, overflow := duration.Parse(strings.TrimSpace(pyString(value)))
	switch {
	case overflow:
		maxDays := int64(time.Duration(math.MaxInt64) / (24 * time.Hour))
		return nil, f.Error("overflow", map[string]any{"min_days": -maxDays, "max_days": maxDays})
	case !ok:
		return nil, f.Error("invalid")
	}
	return d, nil
}
