package orm

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"strings"
)

// Exec runs a statement that returns no rows, such as an UPDATE or DDL. It is
// the escape hatch for SQL the ORM cannot express. The statement runs in the
// transaction of an enclosing Atomic block and is seen by the query hooks,
// like every statement of the ORM.
//
// Values must be passed as args, never formatted into the SQL text. Write a
// parameter as "?"; it is rewritten to the dialect's placeholder ("$1" on
// PostgreSQL), see Rebind.
func (db *DB) Exec(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return db.conn(ctx).ExecContext(ctx, db.Rebind(query), args...)
}

// QueryRows runs a statement that returns rows and hands back the raw
// *sql.Rows, which the caller must close. See Exec for transactions, hooks,
// and parameters, and Raw for mapping the rows to a model.
func (db *DB) QueryRows(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return db.conn(ctx).QueryContext(ctx, db.Rebind(query), args...)
}

// Rebind rewrites the "?" parameters of query to the dialect's placeholders,
// so that the same SQL text runs on SQLite and PostgreSQL. A question mark
// inside a quoted string, a quoted identifier, or a comment is left alone. A
// query that already uses the dialect's own placeholders is unchanged.
func (db *DB) Rebind(query string) string {
	if db.dialect.Placeholder(1) == "?" || !strings.Contains(query, "?") {
		return query
	}
	return replaceParams(query, db.dialect.Placeholder)
}

// replaceParams returns query with its n-th "?" parameter (counted from 1)
// replaced by sub(n). A question mark inside a quoted string, a quoted
// identifier, or a comment is not a parameter.
func replaceParams(query string, sub func(n int) string) string {
	var b strings.Builder
	n := 0
	for i := 0; i < len(query); i++ {
		ch := query[i]
		end := -1
		switch {
		case ch == '?':
			n++
			b.WriteString(sub(n))
			continue
		case ch == '\'' || ch == '"':
			end = skipQuoted(query, i)
		case ch == '-' && strings.HasPrefix(query[i:], "--"):
			end = len(query)
			if nl := strings.IndexByte(query[i:], '\n'); nl >= 0 {
				end = i + nl
			}
		case ch == '/' && strings.HasPrefix(query[i:], "/*"):
			end = len(query)
			if stop := strings.Index(query[i+2:], "*/"); stop >= 0 {
				end = i + 2 + stop + 2
			}
		}
		if end < 0 {
			b.WriteByte(ch)
			continue
		}
		b.WriteString(query[i:end])
		i = end - 1
	}
	return b.String()
}

// skipQuoted returns the index just past the quoted section of s that starts
// at start. A doubled quote inside the section is an escaped quote.
func skipQuoted(s string, start int) int {
	quote := s[start]
	for i := start + 1; i < len(s); i++ {
		if s[i] != quote {
			continue
		}
		if i+1 < len(s) && s[i+1] == quote {
			i++
			continue
		}
		return i + 1
	}
	return len(s)
}

// RawQuerySet maps the rows of a hand-written SELECT to model T, the
// counterpart of Django's Manager.raw(). Build one with Raw.
type RawQuerySet[T any] struct {
	db              *DB
	model           *Model
	query           string
	args            []any
	translations    map[string]string
	prefetchRelated []Prefetch
	err             error
}

// isRawQuerySet marks RawQuerySet so Prefetch rejects raw querysets.
func (*RawQuerySet[T]) isRawQuerySet() {}

// PrefetchRelated returns a copy of r that prefetches the given lookups after
// executing the raw SQL query. Calling PrefetchRelated() with no arguments or
// PrefetchRelated(nil) clears previously configured prefetches.
func (r *RawQuerySet[T]) PrefetchRelated(lookups ...any) *RawQuerySet[T] {
	out := *r
	out.prefetchRelated = append([]Prefetch(nil), r.prefetchRelated...)
	specs, reset, err := parsePrefetchLookups(lookups)
	if err != nil {
		if out.err == nil {
			out.err = err
		}
		return &out
	}
	if reset {
		out.prefetchRelated = nil
		return &out
	}
	out.prefetchRelated = append(out.prefetchRelated, specs...)
	return &out
}

// Raw prepares a hand-written SELECT whose rows are mapped to model T by
// column name:
//
//	authors, err := orm.Raw[Author](db,
//		"SELECT * FROM author WHERE age > ? ORDER BY name", 30).All(ctx)
//
// A result column is matched to the field with that column name or attribute
// name; Translations maps other column names to fields. The primary key must
// be selected. A field without a column in the result keeps its zero value,
// where Django would load it on first access, and a column that matches no
// field is ignored by All and returned by AllAnnotated.
//
// Values must be passed as args, written as "?" in the SQL; see DB.Exec. The
// query runs in the transaction of an enclosing Atomic block and is seen by
// the query hooks.
func Raw[T any](db *DB, query string, args ...any) *RawQuerySet[T] {
	var zero T
	r := &RawQuerySet[T]{db: db, query: query, args: args}
	m, ok := db.Registry().ModelOf(zero)
	if !ok {
		r.err = fmt.Errorf("orm: no model registered for %T", zero)
	}
	r.model = m
	return r
}

// Translations returns a copy of r that maps the result columns named by the
// keys of columns to the fields named by their values, like the translations
// argument of Django's raw(). A value is a field key as in Filter, without
// lookups or relation hops. A translation of a column the result does not
// have is ignored.
func (r *RawQuerySet[T]) Translations(columns map[string]string) *RawQuerySet[T] {
	out := *r
	out.translations = make(map[string]string, len(r.translations)+len(columns))
	for k, v := range r.translations {
		out.translations[k] = v
	}
	for k, v := range columns {
		out.translations[k] = v
	}
	return &out
}

// String returns the query with its parameters filled in, for logs and
// debugging, as Django's repr of a RawQuerySet shows it:
//
//	<RawQuerySet: SELECT * FROM author WHERE last_name = foo>
//
// The text is not quoted SQL and must never be executed.
func (r *RawQuerySet[T]) String() string {
	filled := replaceParams(r.query, func(n int) string {
		if n > len(r.args) {
			return "?"
		}
		return fmt.Sprint(r.args[n-1])
	})
	return "<RawQuerySet: " + filled + ">"
}

// errRawPrimaryKey is returned when a raw query does not select the model's
// primary key.
var errRawPrimaryKey = errors.New("orm: raw query must include the primary key")

// All runs the query and returns one T per row.
func (r *RawQuerySet[T]) All(ctx context.Context) ([]T, error) {
	var out []T
	err := r.fetch(ctx, func(row reflect.Value, _ map[string]any) {
		out = append(out, row.Interface().(T))
	})
	if err != nil {
		return nil, err
	}
	if len(out) > 0 && len(r.prefetchRelated) > 0 {
		elems := make([]reflect.Value, len(out))
		for i := range out {
			elems[i] = reflect.ValueOf(&out[i]).Elem()
		}
		if err := prefetchRelatedObjects(ctx, r.db, r.model, elems, r.prefetchRelated); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// AllAnnotated runs the query and returns one T per row together with the
// values of the columns that match no field, as Django sets them as extra
// attributes. Such a value is what the driver returned, with text as a string.
func (r *RawQuerySet[T]) AllAnnotated(ctx context.Context) ([]Annotated[T], error) {
	var out []Annotated[T]
	err := r.fetch(ctx, func(row reflect.Value, extra map[string]any) {
		out = append(out, Annotated[T]{Object: row.Interface().(T), Annotations: extra})
	})
	if err != nil {
		return nil, err
	}
	if len(out) > 0 && len(r.prefetchRelated) > 0 {
		elems := make([]reflect.Value, len(out))
		for i := range out {
			elems[i] = reflect.ValueOf(&out[i].Object).Elem()
		}
		if err := prefetchRelatedObjects(ctx, r.db, r.model, elems, r.prefetchRelated); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// fetch runs the query and calls each with every row and its extra columns.
func (r *RawQuerySet[T]) fetch(ctx context.Context, each func(row reflect.Value, extra map[string]any)) error {
	if r.err != nil {
		return r.err
	}
	m := r.model
	rows, err := r.db.QueryRows(ctx, r.query, r.args...)
	if err != nil {
		return fmt.Errorf("orm: raw query %s: %w", m.Name(), err)
	}
	defer func() { _ = rows.Close() }()
	columns, err := rows.Columns()
	if err != nil {
		return fmt.Errorf("orm: raw query %s: %w", m.Name(), err)
	}
	fields := make([]*Field, len(columns))
	hasPK := false
	for i, col := range columns {
		name := col
		if to, ok := r.translations[col]; ok {
			name = to
		}
		if f, ok := lookupField(m, name); ok {
			fields[i] = f
			hasPK = hasPK || f == m.pk
		}
	}
	if !hasPK {
		return errRawPrimaryKey
	}
	for rows.Next() {
		elem := reflect.New(m.goType).Elem()
		raw := make([]any, len(columns))
		targets := make([]any, len(columns))
		for i := range raw {
			targets[i] = &raw[i]
		}
		if err := rows.Scan(targets...); err != nil {
			return fmt.Errorf("orm: raw query %s: %w", m.Name(), err)
		}
		extra := make(map[string]any)
		for i, f := range fields {
			if f == nil {
				if b, ok := raw[i].([]byte); ok {
					raw[i] = string(b)
				}
				extra[columns[i]] = raw[i]
				continue
			}
			if err := assignFieldScanned(r.db.dialect, f, elem.Field(f.Index), raw[i]); err != nil {
				return fmt.Errorf("orm: raw query %s: column %s: %w", m.Name(), columns[i], err)
			}
		}
		each(elem, extra)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("orm: raw query %s: %w", m.Name(), err)
	}
	return nil
}
