package orm

import (
	"context"
	"fmt"
	"reflect"
	"time"
)

// The lookups of a DateCheck.
const (
	// DateCheckDate compares the day, month and year of the date field.
	DateCheckDate = "date"
	// DateCheckYear compares the year of the date field.
	DateCheckYear = "year"
	// DateCheckMonth compares the month of the date field, in any year.
	DateCheckMonth = "month"
)

// DateCheck is one unique_for_date, unique_for_year or unique_for_month check
// of a model, an entry of the second list Django's Model._get_unique_checks
// returns.
type DateCheck struct {
	// Lookup is DateCheckDate, DateCheckYear or DateCheckMonth.
	Lookup string
	// Field is the attribute name of the field whose value must be unique.
	Field string
	// DateField is the attribute name of the date field that sets the period.
	DateField string
}

// DateChecks returns the unique_for_date, unique_for_year and unique_for_month
// checks that ValidateUnique performs on m, in field order and for each field
// in the order date, year, month, as Django's _get_unique_checks does. A check
// is left out when its field or its date field is in exclude.
func (m *Model) DateChecks(exclude ...string) []DateCheck {
	return m.dateChecks(m.normalizeExclude(exclude))
}

// dateChecks returns the date checks of m without the fields in exclude, a set
// of attribute names.
func (m *Model) dateChecks(exclude map[string]bool) []DateCheck {
	var checks []DateCheck
	for _, f := range m.fields {
		if exclude[f.Attr] {
			continue
		}
		for _, c := range []DateCheck{
			{Lookup: DateCheckDate, DateField: f.UniqueForDate},
			{Lookup: DateCheckYear, DateField: f.UniqueForYear},
			{Lookup: DateCheckMonth, DateField: f.UniqueForMonth},
		} {
			if c.DateField != "" && !exclude[c.DateField] {
				c.Field = f.Attr
				checks = append(checks, c)
			}
		}
	}
	return checks
}

// DatePeriod returns the part of value, a value of the date field of check,
// that the check compares: the whole date for DateCheckDate, a Date that holds
// only the year for DateCheckYear and one that holds only the month for
// DateCheckMonth. A datetime is read in UTC, the time zone of the date
// transforms. It reports false for a null or zero value.
func (m *Model) DatePeriod(check DateCheck, value any) (Date, bool) {
	f, ok := m.byAttr[check.DateField]
	if !ok {
		return Date{}, false
	}
	unwrapped, isNull, _ := fieldValueState(f, value)
	if isNull {
		return Date{}, false
	}
	var d Date
	switch v := unwrapped.(type) {
	case Date:
		d = v
	case time.Time:
		if f.Kind == KindDateTime {
			v = v.UTC()
		}
		d = DateFromTime(v)
	default:
		return Date{}, false
	}
	switch check.Lookup {
	case DateCheckYear:
		return Date{Year: d.Year}, true
	case DateCheckMonth:
		return Date{Month: d.Month}, true
	}
	return d, true
}

// resolveUniqueFor replaces the date field that each unique_for_date,
// unique_for_year and unique_for_month option of m names with its attribute
// name. It fails for a name that is not a field of m, or not a date or
// datetime field.
func resolveUniqueFor(m *Model) error {
	for _, f := range m.fields {
		for _, opt := range []struct {
			name string
			ref  *string
		}{
			{"unique_for_date", &f.UniqueForDate},
			{"unique_for_year", &f.UniqueForYear},
			{"unique_for_month", &f.UniqueForMonth},
		} {
			if *opt.ref == "" {
				continue
			}
			target, ok := resolveMetaField(*opt.ref, m.byName, m.byAttr, m.byColumn)
			if !ok {
				return fmt.Errorf("orm: model %s: field %s: %s names unknown field %q", m.name, f.Name, opt.name, *opt.ref)
			}
			if target.Kind != KindDate && target.Kind != KindDateTime {
				return fmt.Errorf("orm: model %s: field %s: %s names %s, which is not a date or datetime field",
					m.name, f.Name, opt.name, target.Name)
			}
			*opt.ref = target.Attr
		}
	}
	return nil
}

// performDateChecks runs the date checks of m on the instance rv, Django's
// _perform_date_checks, and adds an error to errs for each check that another
// row fails. A check whose date is null is skipped. When adding is false the
// row of the instance itself does not count.
func performDateChecks(ctx context.Context, db *DB, m *Model, rv reflect.Value, exclude map[string]bool,
	adding bool, errs map[string][]*ValidationError,
) error {
	pk := m.PrimaryKey()
	for _, check := range m.dateChecks(exclude) {
		f := m.byAttr[check.Field]
		df := m.byAttr[check.DateField]
		period, ok := m.DatePeriod(check, rv.Field(df.Index).Interface())
		if !ok {
			continue
		}
		filterArgs := make([]any, 0, 8)
		switch check.Lookup {
		case DateCheckDate:
			filterArgs = append(filterArgs,
				df.Attr+"__day", period.Day,
				df.Attr+"__month", int(period.Month),
				df.Attr+"__year", period.Year,
			)
		case DateCheckYear:
			filterArgs = append(filterArgs, df.Attr+"__year", period.Year)
		default:
			filterArgs = append(filterArgs, df.Attr+"__month", int(period.Month))
		}
		// A null value matches the rows whose value is null, as in Django.
		value, isNull, _ := fieldValueState(f, rv.Field(f.Index).Interface())
		if isNull {
			value = nil
		}
		filterArgs = append(filterArgs, f.Attr, value)

		q := newQuery(db, m).filter(filterArgs, false)
		if !adding && pk != nil && !rv.Field(pk.Index).IsZero() {
			q = q.filter([]any{pk.Column, rv.Field(pk.Index).Interface()}, true)
		}
		exists, err := queryExists(ctx, q)
		if err != nil {
			return err
		}
		if exists {
			errs[check.Field] = append(errs[check.Field], dateErrorMessage(m, check))
		}
	}
	return nil
}

// dateErrorMessage returns the error of a failed date check, Django's
// Model.date_error_message.
func dateErrorMessage(m *Model, check DateCheck) *ValidationError {
	f := m.byAttr[check.Field]
	df := m.byAttr[check.DateField]
	return NewValidationError(f.ErrorMessage("unique_for_date"), "unique_for_date", map[string]any{
		"model_name":       capFirst(m.VerboseName()),
		"lookup_type":      check.Lookup,
		"field":            check.Field,
		"field_label":      capFirst(fieldVerboseName(f)),
		"date_field":       check.DateField,
		"date_field_label": capFirst(fieldVerboseName(df)),
	})
}
