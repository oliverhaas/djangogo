package orm

import (
	"context"
	"errors"
	"fmt"
	"reflect"
)

// valuesItems returns the SELECT list of a Values or ValuesList query and the
// key of each item. Without requested keys it selects every field of the
// model and every annotation that existed when Values was called. Annotations
// added afterwards are always appended.
func (c *compiler) valuesItems() ([]selectItem, []string, error) {
	spec := c.q.values
	var (
		items []selectItem
		keys  []string
	)
	have := make(map[string]bool)
	add := func(key string, it selectItem) {
		items = append(items, it)
		keys = append(keys, key)
		have[key] = true
	}
	if len(spec.keys) == 0 {
		for _, f := range c.q.model.fields {
			add(attname(f), selectItem{node: c.baseColumn(f)})
		}
		for _, name := range c.annotNames[:spec.annotated] {
			add(name, selectItem{node: c.annots[name], alias: name})
		}
	}
	for _, key := range spec.keys {
		if n, ok := c.annots[key]; ok {
			it := selectItem{node: n}
			if !have[key] {
				it.alias = key
			}
			add(key, it)
			continue
		}
		p, err := resolveFieldPath(c.q.model, key, c.annotNames)
		if err != nil {
			return nil, nil, err
		}
		n, err := c.keyNode(p)
		if err != nil {
			return nil, nil, err
		}
		it := selectItem{node: n}
		if _, isColumn := n.(colNode); !isColumn {
			// A transform is selected under its key, as Django selects
			// every values() item, so that GROUP BY and ORDER BY can
			// refer to it.
			it.alias = key
		}
		add(key, it)
	}
	for _, name := range c.annotNames[spec.annotated:] {
		if !have[name] {
			add(name, selectItem{node: c.annots[name], alias: name})
		}
	}
	return items, keys, nil
}

// valuesRows runs the Values query and returns its keys and rows, each value
// converted to the Go type of its field or expression.
func (q *query) valuesRows(ctx context.Context) ([]string, [][]any, error) {
	c, err := q.compile()
	if err != nil {
		return nil, nil, err
	}
	items, keys, err := c.valuesItems()
	if err != nil {
		return nil, nil, err
	}
	stmt, err := c.build(items, true)
	if err != nil {
		return nil, nil, err
	}
	if c.empty {
		return keys, nil, nil
	}
	w := newSQLWriter(c.d)
	stmt.render(w)
	rows, err := q.db.conn(ctx).QueryContext(ctx, w.String(), w.args...)
	if err != nil {
		return nil, nil, fmt.Errorf("orm: query %s: %w", q.model.Name(), err)
	}
	defer func() { _ = rows.Close() }()

	var out [][]any
	for rows.Next() {
		// The statement may select hidden ordering columns after the keys.
		raw := make([]any, len(stmt.items))
		targets := make([]any, len(raw))
		for i := range raw {
			targets[i] = &raw[i]
		}
		if err := rows.Scan(targets...); err != nil {
			return nil, nil, fmt.Errorf("orm: scan %s: %w", q.model.Name(), err)
		}
		row := make([]any, len(keys))
		for i := range keys {
			row[i], err = convertValue(raw[i], items[i].node.vtype())
			if err != nil {
				return nil, nil, fmt.Errorf("orm: scan %s: %s: %w", q.model.Name(), keys[i], err)
			}
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("orm: scan %s: %w", q.model.Name(), err)
	}
	return keys, out, nil
}

// ValuesQuerySet is a queryset whose rows are maps from key to value, the
// counterpart of Django's values(). Build one with QuerySet.Values.
//
// A value has the Go type of its field: int64 for an integer or a foreign
// key, string, bool, or time.Time, and nil for NULL. For the type of an
// annotation see Aggregate.
type ValuesQuerySet struct {
	q *query
}

// subquery exposes the query, so that a one-column ValuesQuerySet can be the
// value of an "in" lookup.
func (vs *ValuesQuerySet) subquery() *query { return vs.q }

// Filter returns a clone restricted by the conditions; see QuerySet.Filter.
func (vs *ValuesQuerySet) Filter(args ...any) *ValuesQuerySet {
	return &ValuesQuerySet{q: vs.q.filter(args, false)}
}

// Exclude returns a clone without the matching rows; see QuerySet.Exclude.
func (vs *ValuesQuerySet) Exclude(args ...any) *ValuesQuerySet {
	return &ValuesQuerySet{q: vs.q.filter(args, true)}
}

// OrderBy returns a clone with the keys and expressions appended to the
// ordering; see QuerySet.OrderBy.
func (vs *ValuesQuerySet) OrderBy(terms ...any) *ValuesQuerySet {
	return &ValuesQuerySet{q: vs.q.orderByArgs(terms)}
}

// Annotate returns a clone with the named expressions added to every row. An
// aggregate added after Values groups the rows by the Values keys; see
// QuerySet.Annotate.
func (vs *ValuesQuerySet) Annotate(args ...any) *ValuesQuerySet {
	return &ValuesQuerySet{q: vs.q.annotate(args)}
}

// Ordered reports whether the rows come in a defined order; see
// QuerySet.Ordered.
func (vs *ValuesQuerySet) Ordered() bool {
	return vs.q.ordered()
}

// None returns a clone that matches no row; see QuerySet.None.
func (vs *ValuesQuerySet) None() *ValuesQuerySet {
	return &ValuesQuerySet{q: vs.q.setNone()}
}

// Distinct returns a clone that removes duplicate rows.
func (vs *ValuesQuerySet) Distinct() *ValuesQuerySet {
	cp := vs.q.clone()
	cp.distinct = true
	return &ValuesQuerySet{q: cp}
}

// Limit returns a clone capped to at most n rows.
func (vs *ValuesQuerySet) Limit(n int) *ValuesQuerySet {
	cp := vs.q.clone()
	cp.limit = n
	return &ValuesQuerySet{q: cp}
}

// Offset returns a clone that skips the first n rows.
func (vs *ValuesQuerySet) Offset(n int) *ValuesQuerySet {
	cp := vs.q.clone()
	cp.offset = n
	return &ValuesQuerySet{q: cp}
}

// All runs the query and returns one map per row.
func (vs *ValuesQuerySet) All(ctx context.Context) ([]map[string]any, error) {
	keys, rows, err := vs.q.valuesRows(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, len(rows))
	for i, row := range rows {
		m := make(map[string]any, len(keys))
		for k, key := range keys {
			m[key] = row[k]
		}
		out[i] = m
	}
	return out, nil
}

// Count returns the number of rows the query yields.
func (vs *ValuesQuerySet) Count(ctx context.Context) (int64, error) {
	return vs.q.count(ctx)
}

// Update writes the assignments to the rows the query matches, as
// QuerySet.Update does; the keys of Values play no part in it.
func (vs *ValuesQuerySet) Update(ctx context.Context, assignments ...any) (int64, error) {
	return vs.q.update(ctx, assignments)
}

// ValuesListQuerySet is a queryset whose rows are slices of values in key
// order, the counterpart of Django's values_list(). Build one with
// QuerySet.ValuesList. The values have the types described at ValuesQuerySet.
type ValuesListQuerySet struct {
	q *query
}

// subquery exposes the query, so that a one-column ValuesListQuerySet can be
// the value of an "in" lookup.
func (vl *ValuesListQuerySet) subquery() *query { return vl.q }

// Filter returns a clone restricted by the conditions; see QuerySet.Filter.
func (vl *ValuesListQuerySet) Filter(args ...any) *ValuesListQuerySet {
	return &ValuesListQuerySet{q: vl.q.filter(args, false)}
}

// Exclude returns a clone without the matching rows; see QuerySet.Exclude.
func (vl *ValuesListQuerySet) Exclude(args ...any) *ValuesListQuerySet {
	return &ValuesListQuerySet{q: vl.q.filter(args, true)}
}

// OrderBy returns a clone with the keys and expressions appended to the
// ordering; see QuerySet.OrderBy.
func (vl *ValuesListQuerySet) OrderBy(terms ...any) *ValuesListQuerySet {
	return &ValuesListQuerySet{q: vl.q.orderByArgs(terms)}
}

// Annotate returns a clone with the named expressions added to every row; see
// ValuesQuerySet.Annotate.
func (vl *ValuesListQuerySet) Annotate(args ...any) *ValuesListQuerySet {
	return &ValuesListQuerySet{q: vl.q.annotate(args)}
}

// Ordered reports whether the rows come in a defined order; see
// QuerySet.Ordered.
func (vl *ValuesListQuerySet) Ordered() bool {
	return vl.q.ordered()
}

// None returns a clone that matches no row; see QuerySet.None.
func (vl *ValuesListQuerySet) None() *ValuesListQuerySet {
	return &ValuesListQuerySet{q: vl.q.setNone()}
}

// Distinct returns a clone that removes duplicate rows.
func (vl *ValuesListQuerySet) Distinct() *ValuesListQuerySet {
	cp := vl.q.clone()
	cp.distinct = true
	return &ValuesListQuerySet{q: cp}
}

// Limit returns a clone capped to at most n rows.
func (vl *ValuesListQuerySet) Limit(n int) *ValuesListQuerySet {
	cp := vl.q.clone()
	cp.limit = n
	return &ValuesListQuerySet{q: cp}
}

// Offset returns a clone that skips the first n rows.
func (vl *ValuesListQuerySet) Offset(n int) *ValuesListQuerySet {
	cp := vl.q.clone()
	cp.offset = n
	return &ValuesListQuerySet{q: cp}
}

// All runs the query and returns one slice per row.
func (vl *ValuesListQuerySet) All(ctx context.Context) ([][]any, error) {
	_, rows, err := vl.q.valuesRows(ctx)
	return rows, err
}

// Flat runs the query and returns the first selected value of every row, like
// Django's values_list(flat=True). It is an error when ValuesList was called
// with more than one key.
func (vl *ValuesListQuerySet) Flat(ctx context.Context) ([]any, error) {
	if vl.q.values != nil && len(vl.q.values.keys) > 1 {
		return nil, errors.New("orm: Flat is not valid when ValuesList is called with more than one field")
	}
	_, rows, err := vl.q.valuesRows(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]any, len(rows))
	for i, row := range rows {
		out[i] = row[0]
	}
	return out, nil
}

// Count returns the number of rows the query yields.
func (vl *ValuesListQuerySet) Count(ctx context.Context) (int64, error) {
	return vl.q.count(ctx)
}

// Update writes the assignments to the rows the query matches, as
// QuerySet.Update does; the keys of ValuesList play no part in it.
func (vl *ValuesListQuerySet) Update(ctx context.Context, assignments ...any) (int64, error) {
	return vl.q.update(ctx, assignments)
}

// Pluck returns the values of one key for every row of qs, typed as V: the
// typed form of ValuesList(key).Flat. The key may span relations or name an
// annotation.
//
//	names, err := orm.Pluck[string](ctx, orm.Query[Author](db).OrderBy("name"), "name")
//
// V must match the type of the key (see ValuesQuerySet) or, for a number, be
// another numeric type. NULL becomes the zero value of V; use ValuesList to
// tell NULL apart.
func Pluck[V, T any](ctx context.Context, qs *QuerySet[T], key string) ([]V, error) {
	vals, err := qs.ValuesList(key).Flat(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]V, len(vals))
	for i, v := range vals {
		if v == nil {
			continue
		}
		typed, err := convertTo[V](v)
		if err != nil {
			return nil, fmt.Errorf("orm: Pluck %q: %w", key, err)
		}
		out[i] = typed
	}
	return out, nil
}

// convertTo returns v as a V. A value of another numeric type is converted;
// anything else must already be a V.
func convertTo[V any](v any) (V, error) {
	if typed, ok := v.(V); ok {
		return typed, nil
	}
	var zero V
	rv := reflect.ValueOf(v)
	rt := reflect.TypeFor[V]()
	if isNumeric(rv.Kind()) && isNumeric(rt.Kind()) {
		return rv.Convert(rt).Interface().(V), nil
	}
	return zero, fmt.Errorf("cannot use a %T value as %s", v, rt)
}

// isNumeric reports whether k is an integer or floating-point kind.
func isNumeric(k reflect.Kind) bool {
	switch k {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return true
	default:
		return false
	}
}
