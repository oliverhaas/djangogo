package orm

import (
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"time"
)

// The date and time transforms, the ones Django registers on DateField,
// DateTimeField and TimeField. An extract ("pub_date__year") yields an
// integer; "date" and "time" cast a datetime to a date or a time of day.
const (
	transformYear       = "year"
	transformIsoYear    = "iso_year"
	transformMonth      = "month"
	transformDay        = "day"
	transformWeek       = "week"
	transformWeekDay    = "week_day"
	transformIsoWeekDay = "iso_week_day"
	transformQuarter    = "quarter"
	transformHour       = "hour"
	transformMinute     = "minute"
	transformSecond     = "second"
	transformDate       = "date"
	transformTime       = "time"
)

// TransformTimeZone is the time zone a datetime is converted to before a
// transform reads it: Django's TIME_ZONE with USE_TZ = True. The ORM has no
// other time zone, so "pub_date__date" is the date in UTC.
const TransformTimeZone = "UTC"

// dateTransforms are the transforms of a date, in Django's registration order.
var dateTransforms = []string{
	transformYear, transformMonth, transformDay, transformWeekDay,
	transformIsoWeekDay, transformWeek, transformIsoYear, transformQuarter,
}

// timeTransforms are the transforms of a time of day.
var timeTransforms = []string{transformHour, transformMinute, transformSecond}

// datetimeTransforms are the transforms of a datetime: those of a date and
// of a time, and the two casts.
var datetimeTransforms = slices.Concat(dateTransforms, timeTransforms, []string{transformDate, transformTime})

// transformsOf returns the transforms of a value of kind k: the date and
// time transforms and those of RegisterTransform.
func transformsOf(k Kind) []string {
	var builtin []string
	switch k {
	case KindDate:
		builtin = dateTransforms
	case KindTime:
		builtin = timeTransforms
	case KindDateTime:
		builtin = datetimeTransforms
	}
	out := builtin
	for _, name := range registeredNames(k) {
		if !slices.Contains(builtin, name) {
			out = append(slices.Clip(out), name)
		}
	}
	return out
}

// outputOf returns the kind of the value that transform name yields for a
// value of kind input.
func outputOf(input Kind, name string) Kind {
	if fn, ok := registered(input, name); ok {
		return fn(Expr{kind: exprNode}).outputKind(input)
	}
	return transformOutput(name)
}

// transformOutput returns the kind of the value date or time transform name
// yields: a date for "date", a time of day for "time", and an integer for an
// extract.
func transformOutput(name string) Kind {
	switch name {
	case transformDate:
		return KindDate
	case transformTime:
		return KindTime
	default:
		return KindInt
	}
}

// lookupCandidates returns the names Django's get_lookups() lists for a value
// of kind k, which "perhaps you meant" suggestions are chosen from: the
// lookups and the transforms.
func lookupCandidates(k Kind) []string {
	return slices.Concat(lookupNames, transformsOf(k))
}

// TransformDialect is an optional capability of a Dialect: the SQL of a date
// or time transform. input is the kind of the transformed expression:
// KindDate, KindDateTime or KindTime. tzname is the time zone a datetime is
// converted to first (TransformTimeZone).
//
// The SQL is returned as a template and its parameters: "{}" in the template
// stands for the transformed expression and each "%s" for the next
// parameter. A dialect that does not implement the interface, or returns ok
// false, cannot run the transform, and a query that uses it is an error.
type TransformDialect interface {
	TransformSQL(name string, input Kind, tzname string) (template string, params []any, ok bool)
}

// transformNode is a date or time transform of an expression.
type transformNode struct {
	name  string
	inner sqlNode
	// input is the kind of inner.
	input Kind
}

// render writes the SQL of the transform that the dialect gives, with the
// inner expression and the parameters in their places.
func (n transformNode) render(w *sqlWriter) {
	td, _ := w.d.(TransformDialect)
	tmpl, params, _ := td.TransformSQL(n.name, n.input, TransformTimeZone)
	renderTemplate(w, tmpl, params, n.inner)
}

// renderTemplate writes the SQL template of a dialect: inner in the place of
// each "{}", and the next of params, bound, in the place of each "%s".
func renderTemplate(w *sqlWriter, tmpl string, params []any, inner sqlNode) {
	for tmpl != "" {
		i := strings.IndexAny(tmpl, "%{")
		if i < 0 {
			w.write(tmpl)
			return
		}
		w.write(tmpl[:i])
		switch {
		case strings.HasPrefix(tmpl[i:], "%s"):
			w.param(params[0])
			params = params[1:]
			tmpl = tmpl[i+2:]
		case strings.HasPrefix(tmpl[i:], "{}"):
			inner.render(w)
			tmpl = tmpl[i+2:]
		default:
			w.write(tmpl[i : i+1])
			tmpl = tmpl[i+1:]
		}
	}
}

// vtype is the type of the transform's output: an integer, a date or a time
// of day.
func (n transformNode) vtype() valueType {
	return fieldValueType(&Field{Kind: transformOutput(n.name)})
}

// aggregate reports whether the transformed expression holds an aggregate.
func (n transformNode) aggregate() bool { return n.inner.aggregate() }

// applyTransform wraps n, a value of kind input, in transform name, after
// checking that dialect d can run it.
func applyTransform(d Dialect, n sqlNode, input Kind, name string) (transformNode, error) {
	td, ok := d.(TransformDialect)
	if ok {
		_, _, ok = td.TransformSQL(name, input, TransformTimeZone)
	}
	if !ok {
		return transformNode{}, fmt.Errorf("orm: the %s dialect does not support the %s transform", d.Name(), name)
	}
	return transformNode{name: name, inner: n, input: input}, nil
}

// transform wraps n, a value of kind input, in transform name: a transform
// of RegisterTransform, or else a date or time transform.
func (c *compiler) transform(n sqlNode, input Kind, name string) (sqlNode, error) {
	if fn, ok := registered(input, name); ok {
		return c.resolveExpr(fn(Expr{kind: exprNode, node: n}))
	}
	return applyTransform(c.d, n, input, name)
}

// withColumn returns n with its innermost column replaced by col. It moves a
// transformed column into the subquery of an exclude across a multi-valued
// relation.
func withColumn(n sqlNode, col colNode) sqlNode {
	switch v := n.(type) {
	case colNode:
		return col
	case transformNode:
		v.inner = withColumn(v.inner, col)
		return v
	case funcNode:
		v.arg = withColumn(v.arg, col)
		return v
	default:
		return n
	}
}

// validTransforms reports whether names is a chain of transforms that a
// value of kind k accepts, each one a transform of the value before it.
func validTransforms(k Kind, names []string) bool {
	for _, name := range names {
		if !slices.Contains(transformsOf(k), name) {
			return false
		}
		k = outputOf(k, name)
	}
	return true
}

// keyNode returns the expression of a path that resolveFieldPath resolved:
// its column, wrapped in the transforms that follow the field.
func (c *compiler) keyNode(p fieldPath) (sqlNode, error) {
	var n sqlNode = c.column(p)
	kind := p.field.Kind
	for _, name := range p.rest {
		tn, err := c.transform(n, kind, name)
		if err != nil {
			return nil, err
		}
		n, kind = tn, outputOf(kind, name)
	}
	return n, nil
}

// yearPredicate returns the predicate of a year or iso_year extract compared
// with a direct value, or nil when Django's YearLookup does not apply. Like
// YearLookup it compares the column itself with the bounds of the year, so
// that an index on the column can serve the query: "exact" becomes BETWEEN
// the first and the last moment of the year, "gt" and "lte" compare with the
// last, and "gte" and "lt" with the first.
func (c *compiler) yearPredicate(lookup string, tn transformNode, value any) (*predicate, error) {
	if tn.name != transformYear && tn.name != transformIsoYear {
		return nil, nil
	}
	if _, ok := comparisonOps[lookup]; !ok {
		return nil, nil
	}
	switch value.(type) {
	case nil, Expr, *Expr, subqueryer:
		return nil, nil
	}
	year, err := yearValue(value)
	if err != nil {
		return nil, err
	}
	start, finish := yearBounds(year, tn.input, tn.name == transformIsoYear)
	bound := &Field{Kind: tn.input}
	pick := func(vs ...any) ([]sqlNode, error) {
		out := make([]sqlNode, 0, len(vs))
		for _, v := range vs {
			n, err := c.prepValue(bound, v, nil)
			if err != nil {
				return nil, err
			}
			out = append(out, n)
		}
		return out, nil
	}
	p := &predicate{lookup: lookup, lhs: tn.inner}
	switch lookup {
	case lookupExact:
		p.lookup = lookupRange
		p.rhs, err = pick(start, finish)
	case lookupGt, lookupLte:
		p.rhs, err = pick(finish)
	default:
		p.rhs, err = pick(start)
	}
	if err != nil {
		return nil, err
	}
	return p, nil
}

// yearValue returns the year a year lookup compares with: a Go integer, or a
// string that holds one, as Django's IntegerField accepts "2015".
func yearValue(v any) (int, error) {
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return int(rv.Int()), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return int(rv.Uint()), nil
	case reflect.String:
		if n, err := strconv.Atoi(strings.TrimSpace(rv.String())); err == nil {
			return n, nil
		}
	}
	return 0, fmt.Errorf("orm: the year lookup takes an integer, got %T", v)
}

// yearBounds returns the first and the last value of a year for a column of
// kind input, Django's year_lookup_bounds_for_date_field and
// year_lookup_bounds_for_datetime_field: dates for a date, and the first
// and the last microsecond, in TransformTimeZone, for a datetime. An ISO year
// starts on the Monday of its first ISO week.
func yearBounds(year int, input Kind, iso bool) (start, finish any) {
	first := time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC)
	next := time.Date(year+1, 1, 1, 0, 0, 0, 0, time.UTC)
	if iso {
		first, next = isoYearStart(year), isoYearStart(year+1)
	}
	if input == KindDateTime {
		return first, next.Add(-time.Microsecond)
	}
	return DateFromTime(first), DateFromTime(next.AddDate(0, 0, -1))
}

// isoYearStart returns the Monday of week 1 of ISO year year, Python's
// date.fromisocalendar(year, 1, 1): the Monday of the week that holds the
// fourth of January.
func isoYearStart(year int) time.Time {
	jan4 := time.Date(year, 1, 4, 0, 0, 0, 0, time.UTC)
	offset := (int(jan4.Weekday()) + 6) % 7
	return jan4.AddDate(0, 0, -offset)
}
