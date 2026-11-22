package orm

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"
)

// truncParts are the extracts that identify a value truncated to a kind of
// Dates or DateTimes: a week is its ISO year and week number, any other kind
// its year and the parts down to the kind.
var truncParts = map[string][]string{
	"year":   {transformYear},
	"month":  {transformYear, transformMonth},
	"week":   {transformIsoYear, transformWeek},
	"day":    {transformYear, transformMonth, transformDay},
	"hour":   {transformYear, transformMonth, transformDay, transformHour},
	"minute": {transformYear, transformMonth, transformDay, transformHour, transformMinute},
	"second": {transformYear, transformMonth, transformDay, transformHour, transformMinute, transformSecond},
}

// Dates returns the distinct values of a date or datetime field of the rows,
// each truncated to kind, Django's QuerySet.dates: "year" gives the first day
// of the year, "month" the first day of the month, "week" the Monday of the
// ISO week and "day" the date. order is "ASC" or "DESC". The field may span
// relations ("article__pub_date"). A datetime is read in TransformTimeZone,
// and the rows where the field is NULL are left out.
//
//	years, err := orm.Query[Article](db).Dates(ctx, "pub_date", "year", "ASC")
func (qs *QuerySet[T]) Dates(ctx context.Context, field, kind, order string) ([]Date, error) {
	if !slices.Contains([]string{"year", "month", "week", "day"}, kind) {
		return nil, errors.New("orm: 'kind' must be one of 'year', 'month', 'week', or 'day'")
	}
	times, err := qs.q.truncated(ctx, field, kind, order, false)
	if err != nil {
		return nil, err
	}
	out := make([]Date, len(times))
	for i, t := range times {
		out[i] = DateFromTime(t)
	}
	return out, nil
}

// DateTimes returns the distinct values of a datetime field of the rows, each
// truncated to kind, Django's QuerySet.datetimes: the kinds of Dates, and
// "hour", "minute" and "second". The values are in TransformTimeZone.
func (qs *QuerySet[T]) DateTimes(ctx context.Context, field, kind, order string) ([]time.Time, error) {
	if _, ok := truncParts[kind]; !ok {
		return nil, errors.New("orm: 'kind' must be one of 'year', 'month', 'week', 'day', 'hour', 'minute', or 'second'")
	}
	return qs.q.truncated(ctx, field, kind, order, true)
}

// truncated runs Dates (datetimes false) and DateTimes. Where Django selects
// the distinct values of a Trunc, it selects the distinct extracts of
// truncParts and builds each value from them in Go: the same values, from the
// transforms that every dialect has.
func (q *query) truncated(ctx context.Context, field, kind, order string, datetimes bool) ([]time.Time, error) {
	if order != "ASC" && order != "DESC" {
		return nil, errors.New("orm: 'order' must be either 'ASC' or 'DESC'")
	}
	if q.err != nil {
		return nil, q.err
	}
	if err := truncatable(q.model, field, kind, datetimes); err != nil {
		return nil, err
	}
	parts := truncParts[kind]
	keys := make([]string, len(parts))
	for i, part := range parts {
		keys[i] = field + lookupSep + part
	}
	vq := q.filter([]any{Q(field+"__isnull", false)}, false).orderBy(nil).withValues(keys)
	vq.distinct = true
	_, rows, err := vq.valuesRows(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]time.Time, 0, len(rows))
	for _, row := range rows {
		if slices.Contains(row, nil) {
			continue
		}
		n := make([]int, len(row))
		for i, v := range row {
			x, ok := v.(int64)
			if !ok {
				return nil, fmt.Errorf("orm: %s of %q is %T, not an integer", parts[i], field, v)
			}
			n[i] = int(x)
		}
		out = append(out, truncatedTime(kind, n))
	}
	slices.SortFunc(out, func(a, b time.Time) int {
		if order == "DESC" {
			return b.Compare(a)
		}
		return a.Compare(b)
	})
	return out, nil
}

// truncatable returns the error of Django's Trunc when field of m cannot be
// truncated to kind, as a date (datetimes false) or as a datetime.
func truncatable(m *Model, field, kind string, datetimes bool) error {
	p, err := resolveFieldPath(m, field, nil)
	if err != nil {
		return err
	}
	name, k := p.field.Attr, p.field.Kind
	for _, t := range p.rest {
		name, k = t, outputOf(k, t)
	}
	date := !p.relation && k == KindDate
	output := "DateField"
	if datetimes {
		output = "DateTimeField"
	}
	switch {
	case !p.relation && k == KindTime:
		return fmt.Errorf("orm: Cannot truncate TimeField '%s' to %s", name, output)
	case date && (datetimes || slices.Contains([]string{"hour", "minute", "second"}, kind)):
		return fmt.Errorf("orm: Cannot truncate DateField '%s' to %s", name, output)
	case !date && (p.relation || k != KindDateTime):
		return fmt.Errorf("orm: '%s' isn't a DateField, TimeField, or DateTimeField", name)
	}
	return nil
}

// truncatedTime returns the value in TransformTimeZone that the extracts n of
// truncParts[kind] identify.
func truncatedTime(kind string, n []int) time.Time {
	if kind == "week" {
		// January 4 is in week 1 of its ISO year.
		jan4 := time.Date(n[0], time.January, 4, 0, 0, 0, 0, time.UTC)
		monday := jan4.AddDate(0, 0, -((int(jan4.Weekday()) + 6) % 7))
		return monday.AddDate(0, 0, 7*(n[1]-1))
	}
	d := []int{0, 1, 1, 0, 0, 0}
	copy(d, n)
	return time.Date(d[0], time.Month(d[1]), d[2], d[3], d[4], d[5], 0, time.UTC)
}
