package orm

import (
	"fmt"
	"reflect"
	"strings"
)

// parseLookup splits a filter key "<column>__<lookup>" into its column and lookup
// parts. A key with no "__" uses the "exact" lookup. When "__" is present the
// suffix after the last occurrence is returned as the lookup candidate; the
// caller (buildPredicate) is responsible for validating it.
func parseLookup(key string) (column, lookup string) {
	idx := strings.LastIndex(key, "__")
	if idx < 0 {
		return key, "exact"
	}
	return key[:idx], key[idx+2:]
}

// escapeLike escapes special LIKE pattern characters in s so that the value is
// matched literally when used with ESCAPE '\'.
// Order: backslash first, then percent, then underscore.
func escapeLike(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	return s
}

// toString returns a best-effort string representation of v for LIKE predicates.
func toString(v any) string {
	return fmt.Sprint(v)
}

// buildPredicate resolves a filter key/value against the model and renders one SQL
// predicate plus its args. next yields successive bind placeholders (so the caller
// controls numbering across predicates); for SQLite each call returns "?". When
// qualify is non-empty, the column reference is prefixed with that table name so
// the predicate is unambiguous in a JOIN query.
func buildPredicate(d Dialect, m *Model, key string, value any, next func() string, qualify string) (sql string, args []any, err error) {
	column, lookup := parseLookup(key)

	f, ok := m.byColumn[column]
	if !ok {
		return "", nil, fmt.Errorf("orm: unknown field %q on model %s", column, m.Name())
	}

	qcol := d.Quote(f.Column)
	if qualify != "" {
		qcol = d.Quote(qualify) + "." + qcol
	}

	switch lookup {
	case "exact":
		if value == nil {
			return qcol + " IS NULL", nil, nil
		}
		return qcol + " = " + next(), []any{value}, nil

	case "gt":
		return qcol + " > " + next(), []any{value}, nil

	case "gte":
		return qcol + " >= " + next(), []any{value}, nil

	case "lt":
		return qcol + " < " + next(), []any{value}, nil

	case "lte":
		return qcol + " <= " + next(), []any{value}, nil

	case "contains", "icontains":
		// icontains is rendered identically to contains for the SQLite backend
		// because SQLite's default LIKE is case-insensitive for ASCII characters;
		// other backends may need explicit case-folding (e.g. ILIKE or LOWER()).
		arg := "%" + escapeLike(toString(value)) + "%"
		return qcol + ` LIKE ` + next() + ` ESCAPE '\'`, []any{arg}, nil

	case "startswith":
		arg := escapeLike(toString(value)) + "%"
		return qcol + ` LIKE ` + next() + ` ESCAPE '\'`, []any{arg}, nil

	case "endswith":
		arg := "%" + escapeLike(toString(value))
		return qcol + ` LIKE ` + next() + ` ESCAPE '\'`, []any{arg}, nil

	case "in":
		rv := reflect.ValueOf(value)
		if rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array {
			return "", nil, fmt.Errorf("orm: in lookup requires a slice, got %T", value)
		}
		n := rv.Len()
		if n == 0 {
			return "0 = 1", nil, nil
		}
		phs := make([]string, n)
		elems := make([]any, n)
		for i := range n {
			phs[i] = next()
			elems[i] = rv.Index(i).Interface()
		}
		return qcol + " IN (" + strings.Join(phs, ", ") + ")", elems, nil

	case "isnull":
		b, ok := value.(bool)
		if !ok {
			return "", nil, fmt.Errorf("orm: isnull lookup requires a bool, got %T", value)
		}
		if b {
			return qcol + " IS NULL", nil, nil
		}
		return qcol + " IS NOT NULL", nil, nil

	default:
		return "", nil, fmt.Errorf("orm: unknown lookup %q", lookup)
	}
}
