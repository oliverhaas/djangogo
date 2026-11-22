package orm

import (
	"fmt"
	"slices"
)

// QuerySet is a lazy, immutable, clone-per-method query builder for model T.
// Chain methods never mutate their receiver; each returns a fresh clone. No
// SQL runs until a terminal method (All, Get, Count, ...) is called, and an
// error made while building, such as an unknown field, is reported there.
//
// A field is named by a key: the attribute name (Field.Attr, "created_at"),
// the column name, or "pk" for the primary key. A foreign key is addressed by
// its attribute ("author") or its column ("author_id") alike. A key follows
// relations with a double underscore ("author__name"), and a filter key may
// end with a lookup ("author__name__icontains").
type QuerySet[T any] struct {
	q *query
}

// Query starts a new QuerySet for model T resolved from db's registry.
func Query[T any](db *DB) *QuerySet[T] {
	var zero T
	m, ok := db.Registry().ModelOf(zero)
	q := newQuery(db, m)
	if !ok {
		q.err = fmt.Errorf("orm: no model registered for %T", zero)
	}
	return &QuerySet[T]{q: q}
}

// subquery exposes the query, so that a QuerySet can be the value of an "in"
// lookup: Filter("author__in", orm.Query[Author](db).Filter(...)).
func (qs *QuerySet[T]) subquery() *query { return qs.q }

// prefetchQuery exposes the underlying *query for Prefetch.QuerySet.
func (qs *QuerySet[T]) prefetchQuery() *query { return qs.q }

// SelectRelated returns a clone that loads the named foreign keys in the same
// query, with a JOIN, and stores each related object on its FK field. A name
// is the attribute of a forward foreign key ("author"); a double underscore
// follows further relations ("author__publisher"). The Go field name
// ("Author") is accepted for a single relation. A name that is not a forward
// foreign key is reported when the queryset runs.
//
// The JOIN is INNER for a non-null foreign key and LEFT OUTER for a nullable
// one, so a row without a related object is kept and its FK stays unloaded.
//
// SelectRelated() without names clears the relations of earlier calls, as
// Django's select_related(None) does. Django 6.1 deprecates select_related()
// without arguments, which follows every non-null foreign key.
func (qs *QuerySet[T]) SelectRelated(fields ...string) *QuerySet[T] {
	cp := qs.q.clone()
	if len(fields) == 0 {
		cp.selectRelated = nil
		return &QuerySet[T]{q: cp}
	}
	cp.selectRelated = append(cp.selectRelated, fields...)
	return &QuerySet[T]{q: cp}
}

// PrefetchRelated returns a clone that loads the named relations using one
// batched IN query per relation level and caches the results on the returned
// model instances. Lookups may be plain "__"-separated relation paths
// ("comment_set", "tags", "author__profile", "books__read_by") or Prefetch
// values carrying a custom QuerySet and/or ToAttr cache key.
//
// Forward FK and O2O relations are stored directly on the field via SetObject
// (even on models without orm.RelCache). Reverse FK, reverse O2O, M2M (both
// sides), and custom ToAttr results are cached in orm.RelCache so subsequent
// RelatedManager, ManyManager, RelatedOne, and Prefetched calls execute no
// queries. Calling PrefetchRelated() with no arguments or PrefetchRelated(nil)
// clears previously configured prefetches, matching Django's
// prefetch_related(None).
func (qs *QuerySet[T]) PrefetchRelated(lookups ...any) *QuerySet[T] {
	cp := qs.q.clone()
	specs, reset, err := parsePrefetchLookups(lookups)
	if err != nil {
		cp.setErr(err)
		return &QuerySet[T]{q: cp}
	}
	if reset {
		cp.prefetchRelated = nil
		return &QuerySet[T]{q: cp}
	}
	cp.prefetchRelated = append(cp.prefetchRelated, specs...)
	return &QuerySet[T]{q: cp}
}

// Filter returns a clone restricted to the rows matching every condition. The
// arguments are alternating keys and values, conditions built with Q, or a mix
// of both:
//
//	Filter("name", "x")                       name = 'x'
//	Filter("age__gte", 18, "name__in", names) both conditions
//	Filter("author__name__icontains", "ada")  across a relation, with a JOIN
//	Filter(orm.Q("a", 1).Or(orm.Q("b", 2)))   a = 1 OR b = 2
//
// The lookups are exact (the default), iexact, gt, gte, lt, lte, contains,
// icontains, startswith, istartswith, endswith, iendswith, in, range, isnull,
// regex, and iregex. A nil value with exact means IS NULL. The value of a
// foreign key may be a primary key, an orm.FK[T], or a model instance. A value
// may also be an Expr, to compare with another column: Filter("a__gt",
// orm.F("b")).
func (qs *QuerySet[T]) Filter(args ...any) *QuerySet[T] {
	return &QuerySet[T]{q: qs.q.filter(args, false)}
}

// Exclude returns a clone without the rows matching the conditions, which take
// the same form as in Filter. Several conditions in one call are negated
// together: Exclude("a", 1, "b", 2) drops the rows where both hold, as
// Django's exclude(a=1, b=2) does. A row whose compared column is NULL is
// kept, also as in Django.
func (qs *QuerySet[T]) Exclude(args ...any) *QuerySet[T] {
	return &QuerySet[T]{q: qs.q.filter(args, true)}
}

// OrderBy returns a clone ordered by the given terms, after any ordering it
// already has. A term is a key or an expression. A leading "-" orders a key
// descending, a key may span relations ("author__name") or name an
// annotation, and "?" orders randomly. An Expr orders ascending, and an
// OrderExpr, which Expr.Asc and Expr.Desc return, in its direction:
//
//	OrderBy("-pub_date", orm.Lower("headline"), orm.F("author").Desc().NullsLast())
//
// Calling OrderBy without arguments clears the ordering. Any other argument
// type is an error when the queryset runs.
//
// Django's order_by replaces the previous ordering; OrderBy appends to it, so
// that orderings can be built up step by step.
func (qs *QuerySet[T]) OrderBy(terms ...any) *QuerySet[T] {
	return &QuerySet[T]{q: qs.q.orderByArgs(terms)}
}

// Ordering returns the terms the rows are ordered by, in the form OrderBy
// takes them: a key, "-" and a key, "?" or an OrderExpr. They are the terms of
// OrderBy, or the default ordering of the model when OrderBy was not called
// and no aggregate annotation groups the rows. It is empty for an unordered
// queryset.
func (qs *QuerySet[T]) Ordering() []any {
	return orderArgs(qs.q.effectiveOrders())
}

// Ordered reports whether the rows come in a defined order, Django's
// qs.ordered: Ordering is not empty, or the queryset is one of None, which
// has no rows to order.
func (qs *QuerySet[T]) Ordered() bool {
	return qs.q.ordered()
}

// None returns a clone that matches no row, Django's none(). Reading it runs
// no statement: All returns no rows, Count zero, Exists false, Update and
// Delete change nothing, and Aggregate returns the value of each aggregate
// over no rows. Filters, ordering and the other builders apply to it as to
// any queryset, and it stays empty.
func (qs *QuerySet[T]) None() *QuerySet[T] {
	return &QuerySet[T]{q: qs.q.setNone()}
}

// TotallyOrdered reports whether the ordering decides the position of every
// row, Django's totally_ordered: it names the primary key, a unique field that
// is not null, or every field of a unique_together set or of a unique
// constraint on fields without a condition, when none of them is nullable. An
// annotation that is F of a field of the model counts as that field. An
// ordering by a relation name, which orders by the related model, by a field
// of a related model or by an expression other than F does not count.
func (qs *QuerySet[T]) TotallyOrdered() bool {
	return qs.q.totallyOrdered()
}

// totallyOrdered implements QuerySet.TotallyOrdered.
func (q *query) totallyOrdered() bool {
	m := q.model
	if m == nil {
		return false
	}
	candidates := map[*Field]bool{}
	for _, t := range q.effectiveOrders() {
		if t.random {
			continue
		}
		key := t.key
		if t.expr != nil {
			if t.expr.kind != exprField {
				continue
			}
			key = t.expr.key
		}
		if a, ok := q.annotation(key); ok {
			if a.kind != exprField {
				continue
			}
			if f, ok := m.orderingField(a.key); ok {
				candidates[f] = true
			}
			continue
		}
		if key == "pk" {
			return true
		}
		f, ok := m.orderingField(key)
		// An ordering by a relation name orders by the related model.
		if !ok || (f.Rel != nil && key == f.Attr) {
			continue
		}
		candidates[f] = true
	}
	for f := range candidates {
		if (f.PrimaryKey || f.Unique) && !f.Null {
			return true
		}
	}
	sets := slices.Clone(m.uniqueTogether)
	for _, c := range m.constraints {
		if u, ok := c.(UniqueConstraint); ok && len(u.Fields) > 0 && u.Condition.empty() {
			sets = append(sets, u.Fields)
		}
	}
	for _, set := range sets {
		if len(set) > 0 && slices.IndexFunc(set, func(name string) bool {
			f, ok := m.byName[name]
			return !ok || !candidates[f] || f.Null
		}) < 0 {
			return true
		}
	}
	return false
}

// annotation returns the expression of the annotation name.
func (q *query) annotation(name string) (Expr, bool) {
	for _, a := range q.annotations {
		if a.name == name {
			return a.expr, true
		}
	}
	return Expr{}, false
}

// orderingField returns the field of m that key names: "pk", an attribute
// name or a column name. A key that spans a relation names no field.
func (m *Model) orderingField(key string) (*Field, bool) {
	if key == "pk" {
		f := m.PrimaryKey()
		return f, f != nil
	}
	if f, ok := m.FieldByAttr(key); ok {
		return f, true
	}
	return m.FieldByColumn(key)
}

// Reverse returns a clone with the ordering reversed. A queryset without an
// ordering is ordered by descending primary key.
func (qs *QuerySet[T]) Reverse() *QuerySet[T] {
	return &QuerySet[T]{q: qs.q.reversed()}
}

// Limit returns a clone capped to at most n rows.
func (qs *QuerySet[T]) Limit(n int) *QuerySet[T] {
	cp := qs.q.clone()
	cp.limit = n
	return &QuerySet[T]{q: cp}
}

// Offset returns a clone that skips the first n rows.
func (qs *QuerySet[T]) Offset(n int) *QuerySet[T] {
	cp := qs.q.clone()
	cp.offset = n
	return &QuerySet[T]{q: cp}
}

// Distinct returns a clone that removes duplicate rows (SELECT DISTINCT).
func (qs *QuerySet[T]) Distinct() *QuerySet[T] {
	cp := qs.q.clone()
	cp.distinct = true
	return &QuerySet[T]{q: cp}
}

// SelectForUpdate returns a clone whose rows are locked until the end of the
// transaction (SELECT ... FOR UPDATE). It only has an effect inside Atomic,
// and none on a dialect without row locks, such as SQLite.
func (qs *QuerySet[T]) SelectForUpdate() *QuerySet[T] {
	cp := qs.q.clone()
	cp.forUpdate = true
	return &QuerySet[T]{q: cp}
}

// Annotate returns a clone with named expressions added to every row. The
// arguments are alternating names and expressions; an aggregate over a plain
// field may be passed without a name and is then named "<key>__<function>":
//
//	Annotate("n", orm.Count("id"))
//	Annotate(orm.Max("age"))               named "age__max"
//	Annotate("total", orm.F("a").Add(orm.F("b")))
//
// An annotation can be used as a key in Filter, Exclude, OrderBy, Values, and
// ValuesList. An aggregate annotation groups the rows: by the row itself, or
// by the Values keys when Values was called first, which is Django's
// values().annotate() idiom for GROUP BY:
//
//	Query[Book](db).Values("author").Annotate("n", orm.Count("id"))
//
// The model struct has no place for an annotation value; read it with Values,
// ValuesList, or AllAnnotated.
func (qs *QuerySet[T]) Annotate(args ...any) *QuerySet[T] {
	return &QuerySet[T]{q: qs.q.annotate(args)}
}

// Values returns a queryset of rows as maps from key to value instead of
// model structs. Without keys every field of the model is included, under its
// attribute name, or "<attribute>_id" for a foreign key, followed by the
// annotations. A key may span relations ("author__name") or name an
// annotation; the map uses the key as it was given.
func (qs *QuerySet[T]) Values(keys ...string) *ValuesQuerySet {
	return &ValuesQuerySet{q: qs.q.withValues(keys)}
}

// ValuesList returns a queryset of rows as slices of values, in key order,
// instead of model structs. The keys are those of Values.
func (qs *QuerySet[T]) ValuesList(keys ...string) *ValuesListQuerySet {
	return &ValuesListQuerySet{q: qs.q.withValues(keys)}
}
