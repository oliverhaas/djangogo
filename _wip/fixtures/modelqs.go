package orm

import (
	"context"
	"fmt"
	"reflect"
)

// ModelQuerySet is a queryset for a model known only at run time, such as the
// model that a fixture names. Its rows are pointers to new instances of the
// model. Like a QuerySet it is lazy, and each chain method returns a clone.
type ModelQuerySet struct {
	q *query
}

// QueryModel starts a ModelQuerySet of every row of the model m, in the
// default ordering of m. It is Query[T](db) for code that knows the model only
// at run time.
func QueryModel(db *DB, m *Model) *ModelQuerySet {
	q := newQuery(db, m)
	if m == nil {
		q.setErr(fmt.Errorf("orm: QueryModel needs a model"))
	}
	return &ModelQuerySet{q: q}
}

// RelatedMany returns a ModelQuerySet of the targets related to owner, a
// pointer to an instance of a model, through the forward many-to-many field f
// of that model, in the default ordering of the target model. It is the
// queryset of Many for code that knows the target model only at run time.
func RelatedMany(db *DB, owner any, f *Field) (*ModelQuerySet, error) {
	if f == nil || f.Rel == nil || f.Rel.Kind != RelM2M {
		return nil, fmt.Errorf("orm: RelatedMany needs a many-to-many field")
	}
	core, err := newDynamicManyManager(db, owner, f)
	if err != nil {
		return nil, err
	}
	q := newQuery(db, core.targetModel)
	q.m2mFilter = &m2mCoreFilter{
		through:     core.through,
		targetFKCol: core.targetFK.Column,
		sourceFKCol: core.sourceFK.Column,
		sourceField: core.sourceFK,
		ownerPK:     core.ownerPK,
	}
	return &ModelQuerySet{q: q}, nil
}

// subquery exposes the query, so that a ModelQuerySet can be the value of
// Subquery, Exists and an "in" lookup.
func (qs *ModelQuerySet) subquery() *query { return qs.q }

// Model returns the model of the rows.
func (qs *ModelQuerySet) Model() *Model { return qs.q.model }

// Filter returns a clone restricted to the rows matching the conditions; see
// QuerySet.Filter.
func (qs *ModelQuerySet) Filter(args ...any) *ModelQuerySet {
	return &ModelQuerySet{q: qs.q.filter(args, false)}
}

// Exclude returns a clone without the rows matching the conditions; see
// QuerySet.Exclude.
func (qs *ModelQuerySet) Exclude(args ...any) *ModelQuerySet {
	return &ModelQuerySet{q: qs.q.filter(args, true)}
}

// OrderBy returns a clone ordered by the given terms; see QuerySet.OrderBy.
func (qs *ModelQuerySet) OrderBy(terms ...any) *ModelQuerySet {
	return &ModelQuerySet{q: qs.q.orderByArgs(terms)}
}

// Only returns a clone that loads the primary key and the named fields only;
// see QuerySet.Only.
func (qs *ModelQuerySet) Only(keys ...string) *ModelQuerySet {
	return &ModelQuerySet{q: qs.q.onlyFields(keys)}
}

// All runs the query and returns every matching row as a pointer to a new
// instance of the model.
func (qs *ModelQuerySet) All(ctx context.Context) ([]any, error) {
	if qs.q.err != nil {
		return nil, qs.q.err
	}
	var out []any
	err := qs.q.fetch(ctx, func(row reflect.Value, _ *rowPlan, _ []any) {
		out = append(out, row.Addr().Interface())
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Count returns the number of matching rows.
func (qs *ModelQuerySet) Count(ctx context.Context) (int64, error) {
	if qs.q.err != nil {
		return 0, qs.q.err
	}
	return qs.q.count(ctx)
}
