package admin

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/oliverhaas/djangogo/checks"
	"github.com/oliverhaas/djangogo/orm"
)

// The date hierarchy reads and bounds a datetime in UTC, the zone the ORM
// extracts its year, month and day in (orm.TransformTimeZone), where Django
// uses TIME_ZONE.

// dateHierarchyField returns the field of the DateHierarchy path of e, or nil
// when the path names no date or datetime field.
func dateHierarchyField(e *entry) *orm.Field {
	if e.admin.DateHierarchy == "" {
		return nil
	}
	info, ok := orm.ResolveLookupPath(e.model, normalizeFieldPath(e.model, e.admin.DateHierarchy))
	if !ok || info.Lookup != "" || info.IsRelation || info.Field == nil {
		return nil
	}
	if info.Field.Kind != orm.KindDate && info.Field.Kind != orm.KindDateTime {
		return nil
	}
	return info.Field
}

// checkDateHierarchy checks DateHierarchy, Django's _check_date_hierarchy:
// the path names a field (admin.E127), and the field is a date or a datetime
// (admin.E128).
func checkDateHierarchy(e *entry, obj adminObject) []checks.Message {
	name := e.admin.DateHierarchy
	if name == "" {
		return nil
	}
	info, ok := orm.ResolveLookupPath(e.model, normalizeFieldPath(e.model, name))
	if !ok || info.Lookup != "" {
		return []checks.Message{checks.Error(fmt.Sprintf(
			"The value of 'date_hierarchy' refers to '%s', which does not refer to a Field.", name),
			checks.Obj(obj), checks.ID("admin.E127"))}
	}
	if dateHierarchyField(e) == nil {
		return []checks.Message{checks.Error("The value of 'date_hierarchy' must be a DateField or DateTimeField.",
			checks.Obj(obj), checks.ID("admin.E128"))}
	}
	return nil
}

// dateHierarchyBounds removes the year, month and day of the date hierarchy
// field from params and returns the range they select, Django's bounded
// lookup parameters in ChangeList.get_filters: from the first moment of the
// year, month or day to the first moment after it. ok is false when params
// has no year. A value that gives no date is errIncorrectLookup.
func dateHierarchyBounds(params url.Values, field string) (from, to time.Time, ok bool, err error) {
	year, hasYear := params[field+"__year"]
	if !hasYear {
		return time.Time{}, time.Time{}, false, nil
	}
	month, hasMonth := params[field+"__month"]
	day, hasDay := params[field+"__day"]
	delete(params, field+"__year")
	delete(params, field+"__month")
	delete(params, field+"__day")
	part := func(vals []string, present bool) (int, error) {
		if !present || len(vals) == 0 {
			return 1, nil
		}
		return strconv.Atoi(strings.TrimSpace(vals[len(vals)-1]))
	}
	y, errY := part(year, true)
	m, errM := part(month, hasMonth)
	d, errD := part(day, hasDay)
	from = time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.UTC)
	if err := errors.Join(errY, errM, errD); err != nil {
		return time.Time{}, time.Time{}, false, fmt.Errorf("%w: %w", errIncorrectLookup, err)
	}
	if y < 1 || y > 9999 || from.Year() != y || int(from.Month()) != m || from.Day() != d {
		return time.Time{}, time.Time{}, false, fmt.Errorf("%w: no date %d-%d-%d", errIncorrectLookup, y, m, d)
	}
	switch {
	case hasDay:
		to = from.AddDate(0, 0, 1)
	case hasMonth:
		to = from.AddDate(0, 1, 0)
	default:
		to = from.AddDate(1, 0, 0)
	}
	return from, to, true, nil
}

// dateHierarchyFilter removes the date hierarchy parameters from params and
// returns the condition of their range. bounds holds the two parameters
// Django adds to the remaining lookup parameters in their place, as str()
// prints an aware datetime.
func (cl *changeList) dateHierarchyFilter(params url.Values) (cond orm.Cond, bounds url.Values, ok bool, err error) {
	field := cl.dateHierarchyPath
	if field == "" {
		return orm.Cond{}, nil, false, nil
	}
	from, to, ok, err := dateHierarchyBounds(params, field)
	if !ok || err != nil {
		return orm.Cond{}, nil, false, err
	}
	const pyDatetime = "2006-01-02 15:04:05-07:00"
	bounds = url.Values{
		field + "__gte": {from.Format(pyDatetime)},
		field + "__lt":  {to.Format(pyDatetime)},
	}
	if cl.dateHierarchyField.Kind == orm.KindDate {
		return orm.Q(field+"__gte", orm.DateFromTime(from), field+"__lt", orm.DateFromTime(to)), bounds, true, nil
	}
	return orm.Q(field+"__gte", from, field+"__lt", to), bounds, true, nil
}

// dateHierarchy returns the context of the date hierarchy of cl, Django's
// date_hierarchy template tag: the link back up the hierarchy and the links
// to the years, the months of a year or the days of a month that have rows.
// It is nil when the admin has no date hierarchy.
func (cl *changeList) dateHierarchy(ctx context.Context) (map[string]any, error) {
	field := cl.dateHierarchyPath
	if field == "" || cl.dateHierarchyField == nil {
		return nil, nil
	}
	datetimes := cl.dateHierarchyField.Kind == orm.KindDateTime
	yearField, monthField, dayField := field+"__year", field+"__month", field+"__day"
	yearLookup, monthLookup, dayLookup := cl.params[yearField], cl.params[monthField], cl.params[dayField]
	link := func(filters map[string]any) string {
		return cl.getQueryString(filters, []string{field + "__"})
	}
	dates := func(kind string) ([]time.Time, error) {
		return cl.entry.ops.dates(ctx, cl.filteredSpec, field, kind, datetimes)
	}

	if yearLookup == "" && monthLookup == "" && dayLookup == "" {
		// Select the start level.
		first, last, err := cl.entry.ops.dateRange(ctx, cl.filteredSpec, field)
		if err != nil {
			return nil, err
		}
		if !first.IsZero() && !last.IsZero() && first.Year() == last.Year() {
			yearLookup = strconv.Itoa(first.Year())
			if first.Month() == last.Month() {
				monthLookup = strconv.Itoa(int(first.Month()))
			}
		}
	}

	switch {
	case yearLookup != "" && monthLookup != "" && dayLookup != "":
		y, _ := strconv.Atoi(strings.TrimSpace(yearLookup))
		m, _ := strconv.Atoi(strings.TrimSpace(monthLookup))
		d, _ := strconv.Atoi(strings.TrimSpace(dayLookup))
		day := time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.UTC)
		return map[string]any{
			"show": true,
			"back": map[string]any{
				"link":  link(map[string]any{yearField: yearLookup, monthField: monthLookup}),
				"title": day.Format(yearMonthFormat),
			},
			"choices": []map[string]any{{"title": day.Format(monthDayFormat)}},
		}, nil
	case yearLookup != "" && monthLookup != "":
		days, err := dates("day")
		if err != nil {
			return nil, err
		}
		choices := make([]map[string]any, len(days))
		for i, day := range days {
			choices[i] = map[string]any{
				"link":  link(map[string]any{yearField: yearLookup, monthField: monthLookup, dayField: day.Day()}),
				"title": day.Format(monthDayFormat),
			}
		}
		return map[string]any{
			"show":    true,
			"back":    map[string]any{"link": link(map[string]any{yearField: yearLookup}), "title": yearLookup},
			"choices": choices,
		}, nil
	case yearLookup != "":
		months, err := dates("month")
		if err != nil {
			return nil, err
		}
		choices := make([]map[string]any, len(months))
		for i, month := range months {
			choices[i] = map[string]any{
				"link":  link(map[string]any{yearField: yearLookup, monthField: int(month.Month())}),
				"title": month.Format(yearMonthFormat),
			}
		}
		return map[string]any{
			"show":    true,
			"back":    map[string]any{"link": link(map[string]any{}), "title": "All dates"},
			"choices": choices,
		}, nil
	default:
		years, err := dates("year")
		if err != nil {
			return nil, err
		}
		choices := make([]map[string]any, len(years))
		for i, year := range years {
			choices[i] = map[string]any{
				"link":  link(map[string]any{yearField: strconv.Itoa(year.Year())}),
				"title": strconv.Itoa(year.Year()),
			}
		}
		return map[string]any{"show": true, "back": nil, "choices": choices}, nil
	}
}

// yearMonthFormat and monthDayFormat are Django's YEAR_MONTH_FORMAT ("F Y")
// and MONTH_DAY_FORMAT ("F j") of the English locale.
const (
	yearMonthFormat = "January 2006"
	monthDayFormat  = "January 2"
)
