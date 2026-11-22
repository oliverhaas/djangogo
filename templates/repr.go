package templates

import (
	"fmt"
	"math"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/flosch/pongo2/v6"

	"github.com/oliverhaas/djangogo/orm"
)

// maxReprDepth bounds the nesting pyRepr follows, so a map that contains
// itself ends.
const maxReprDepth = 64

// pyRepr returns what Python's repr() gives for the value a template sees:
// None, True, 'text', 1.5, [1, 2], {'a': 1}, <Model: label>,
// datetime.date(2012, 2, 14). Map keys are in sorted order, as Go keeps no
// insertion order. A value without a Python counterpart prints as Go
// prints it.
func pyRepr(v any) string {
	var b strings.Builder
	writeRepr(&b, v, 0)
	return b.String()
}

// pyStr returns what Python's str() gives: the text of a string, the label
// of a model instance, the HTML of an HTMLer, the ISO form of a date or a
// datetime, and the repr of everything else.
func pyStr(v any) string {
	switch x := unwrapValue(v).(type) {
	case string:
		return x
	case modelView:
		return x.label()
	case orm.Date, orm.Time:
		return x.(fmt.Stringer).String()
	case time.Time:
		if x.Nanosecond()/1000 != 0 {
			return x.Format("2006-01-02 15:04:05.000000-07:00")
		}
		return x.Format("2006-01-02 15:04:05-07:00")
	case time.Duration:
		return formatDurationPython(x)
	case HTMLer:
		return x.HTML()
	}
	if rv := reflect.ValueOf(unwrapValue(v)); rv.Kind() == reflect.String {
		return rv.String()
	}
	return pyRepr(v)
}

// unwrapValue returns the Go value inside a pongo2 value.
func unwrapValue(v any) any {
	for {
		pv, ok := v.(*pongo2.Value)
		if !ok || pv == nil {
			return v
		}
		v = pv.Interface()
	}
}

// writeRepr writes the repr of v to b.
func writeRepr(b *strings.Builder, v any, depth int) {
	v = unwrapValue(v)
	if depth > maxReprDepth {
		b.WriteString("...")
		return
	}
	switch x := v.(type) {
	case nil:
		b.WriteString("None")
		return
	case bool:
		if x {
			b.WriteString("True")
		} else {
			b.WriteString("False")
		}
		return
	case string:
		b.WriteString(pyStrRepr(x))
		return
	case float32:
		b.WriteString(pyFloatRepr(float64(x)))
		return
	case float64:
		b.WriteString(pyFloatRepr(x))
		return
	case modelView:
		b.WriteString("<" + x.className() + ": " + x.label() + ">")
		return
	case groupedResult:
		b.WriteString("GroupedResult(grouper=")
		writeRepr(b, x["grouper"], depth+1)
		b.WriteString(", list=")
		writeRepr(b, x["list"], depth+1)
		b.WriteString(")")
		return
	case orm.Date:
		fmt.Fprintf(b, "datetime.date(%d, %d, %d)", x.Year, int(x.Month), x.Day)
		return
	case orm.Time:
		b.WriteString("datetime.time(" + clockArgs(x.Hour, x.Minute, x.Second, x.Nanosecond/1000) + ")")
		return
	case time.Time:
		b.WriteString(datetimeRepr(x))
		return
	case time.Duration:
		b.WriteString(timedeltaRepr(x))
		return
	}
	rv := reflect.ValueOf(v)
	if s, ok := v.(fmt.Stringer); ok {
		if rv.Kind() == reflect.Pointer && rv.IsNil() {
			b.WriteString("None")
		} else {
			b.WriteString(s.String())
		}
		return
	}
	switch rv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		b.WriteString(strconv.FormatInt(rv.Int(), 10))
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		b.WriteString(strconv.FormatUint(rv.Uint(), 10))
	case reflect.String:
		b.WriteString(pyStrRepr(rv.String()))
	case reflect.Slice, reflect.Array:
		if rv.Kind() == reflect.Slice && rv.IsNil() {
			b.WriteString("[]")
			return
		}
		b.WriteByte('[')
		for i := range rv.Len() {
			if i > 0 {
				b.WriteString(", ")
			}
			writeRepr(b, rv.Index(i).Interface(), depth+1)
		}
		b.WriteByte(']')
	case reflect.Map:
		b.WriteByte('{')
		for i, k := range sortedKeys(rv) {
			if i > 0 {
				b.WriteString(", ")
			}
			writeRepr(b, k.Interface(), depth+1)
			b.WriteString(": ")
			writeRepr(b, rv.MapIndex(k).Interface(), depth+1)
		}
		b.WriteByte('}')
	case reflect.Pointer:
		if rv.IsNil() {
			b.WriteString("None")
			return
		}
		writeRepr(b, rv.Elem().Interface(), depth+1)
	default:
		fmt.Fprint(b, v)
	}
}

// sortedKeys returns the keys of a map in the order compareValues gives.
func sortedKeys(rv reflect.Value) []reflect.Value {
	keys := rv.MapKeys()
	slices.SortFunc(keys, func(a, b reflect.Value) int {
		return compareValues(a.Interface(), b.Interface())
	})
	return keys
}

// pyStrRepr returns the repr of a Python string: in single quotes, or in
// double quotes when the text has a single quote and no double quote, with
// backslash escapes for the quote, the backslash and the characters that
// are not printable.
func pyStrRepr(s string) string {
	quote := byte('\'')
	if strings.IndexByte(s, '\'') >= 0 && strings.IndexByte(s, '"') < 0 {
		quote = '"'
	}
	var b strings.Builder
	b.Grow(len(s) + 2)
	b.WriteByte(quote)
	for _, r := range s {
		switch {
		case r == rune(quote) || r == '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		case r == '\t':
			b.WriteString(`\t`)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\r':
			b.WriteString(`\r`)
		case r == ' ' || (r != utf8.RuneError && unicode.IsPrint(r)):
			b.WriteRune(r)
		case r < 0x100:
			fmt.Fprintf(&b, `\x%02x`, r)
		case r < 0x10000:
			fmt.Fprintf(&b, `\u%04x`, r)
		default:
			fmt.Fprintf(&b, `\U%08x`, r)
		}
	}
	b.WriteByte(quote)
	return b.String()
}

// pyFloatRepr returns the repr of a Python float: the shortest text that
// reads back as f, with ".0" on a whole number and an exponent below 1e-4
// and from 1e16 on.
func pyFloatRepr(f float64) string {
	abs := math.Abs(f)
	if f != 0 && (abs < 1e-4 || abs >= 1e16) && !math.IsInf(f, 0) && !math.IsNaN(f) {
		return strconv.FormatFloat(f, 'e', -1, 64)
	}
	return pyFloatText(f)
}

// clockArgs returns the arguments of a Python time or datetime repr from the
// hour on: a zero microsecond is left out, and then a zero second.
func clockArgs(hour, minute, second, micro int) string {
	args := []int{hour, minute, second, micro}
	for range 2 {
		if args[len(args)-1] == 0 {
			args = args[:len(args)-1]
		}
	}
	parts := make([]string, len(args))
	for i, a := range args {
		parts[i] = strconv.Itoa(a)
	}
	return strings.Join(parts, ", ")
}

// datetimeRepr returns the repr of an aware Python datetime.
func datetimeRepr(t time.Time) string {
	s := fmt.Sprintf("datetime.datetime(%d, %d, %d, %s", t.Year(), int(t.Month()), t.Day(),
		clockArgs(t.Hour(), t.Minute(), t.Second(), t.Nanosecond()/1000))
	return s + ", tzinfo=" + tzinfoRepr(t) + ")"
}

// tzinfoRepr returns the repr of the Python tzinfo of t's location: the UTC
// singleton, a ZoneInfo for a named zone and a fixed offset otherwise.
func tzinfoRepr(t time.Time) string {
	loc := t.Location()
	switch name := loc.String(); {
	case loc == time.UTC || name == "UTC":
		return "datetime.timezone.utc"
	case name != "" && name != "Local" && !strings.HasPrefix(name, "UTC+") && !strings.HasPrefix(name, "UTC-"):
		return "zoneinfo.ZoneInfo(key=" + pyStrRepr(name) + ")"
	}
	_, offset := t.Zone()
	return "datetime.timezone(" + timedeltaRepr(time.Duration(offset)*time.Second) + ")"
}

// timedeltaRepr returns the repr of a Python timedelta: days, seconds and
// microseconds, each left out when zero, normalized so that only days is
// negative.
func timedeltaRepr(d time.Duration) string {
	micro := int64(d / time.Microsecond)
	const microPerDay = 86400 * 1_000_000
	days := micro / microPerDay
	rest := micro % microPerDay
	if rest < 0 {
		days--
		rest += microPerDay
	}
	var parts []string
	if days != 0 {
		parts = append(parts, "days="+strconv.FormatInt(days, 10))
	}
	if secs := rest / 1_000_000; secs != 0 {
		parts = append(parts, "seconds="+strconv.FormatInt(secs, 10))
	}
	if us := rest % 1_000_000; us != 0 {
		parts = append(parts, "microseconds="+strconv.FormatInt(us, 10))
	}
	if len(parts) == 0 {
		return "datetime.timedelta(0)"
	}
	return "datetime.timedelta(" + strings.Join(parts, ", ") + ")"
}
