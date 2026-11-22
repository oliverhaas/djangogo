package admin

import (
	"context"
	"net/url"
	"reflect"
	"time"

	"github.com/oliverhaas/djangogo/forms"
	"github.com/oliverhaas/djangogo/orm"
)

// querySpec describes a type-independent changelist or action query executed
// through modelOps closures.
type querySpec struct {
	filters       []orm.Cond
	ordering      []any
	selectRelated []string
	// prefetchRelated are forward relations loaded in one query each.
	prefetchRelated []string
	distinct        bool
	limit           int // <= 0 means unlimited (unless limitZero is true)
	limitZero       bool
	offset          int
}

// applySpec translates spec onto a typed *orm.QuerySet[T].
func applySpec[T any](qs *orm.QuerySet[T], spec querySpec) *orm.QuerySet[T] {
	for _, f := range spec.filters {
		qs = qs.Filter(f)
	}
	if len(spec.selectRelated) > 0 {
		qs = qs.SelectRelated(spec.selectRelated...)
	}
	for _, p := range spec.prefetchRelated {
		qs = qs.PrefetchRelated(p)
	}
	if len(spec.ordering) > 0 {
		qs = qs.OrderBy(spec.ordering...)
	}
	if spec.distinct {
		qs = qs.Distinct()
	}
	if spec.offset > 0 {
		qs = qs.Offset(spec.offset)
	}
	if spec.limitZero {
		qs = qs.Limit(0)
	} else if spec.limit > 0 {
		qs = qs.Limit(spec.limit)
	}
	return qs
}

// modelOps is the type-erased set of CRUD and query operations for one
// registered model. Register[T] builds these closures over the generic ORM so
// that views and the site can manipulate rows without knowing the concrete
// model type. Every closure works with any values that are concretely *T.
type modelOps struct {
	// newPtr returns a fresh *T as an any.
	newPtr func() any
	// totallyOrdered reports whether ordering decides the position of every
	// row, orm.QuerySet.TotallyOrdered. An empty ordering does not.
	totallyOrdered func(ordering []any) bool
	// all returns every row, ordered by the given OrderBy terms, as a []any
	// whose elements are *T pointing into a stable backing slice.
	all func(ctx context.Context, ordering []string) ([]any, error)
	// query executes spec and returns matching rows as []any of *T.
	query func(ctx context.Context, spec querySpec) ([]any, error)
	// count executes COUNT(*) for spec (ignoring limit/offset/ordering).
	count func(ctx context.Context, spec querySpec) (int64, error)
	// pks returns the primary keys of rows matching spec.
	pks func(ctx context.Context, spec querySpec) ([]any, error)
	// distinctValues returns the distinct values of key matching spec, ordered by key.
	distinctValues func(ctx context.Context, spec querySpec, key string) ([]any, error)
	// deleteSpec deletes all rows matching spec and returns the deleted count.
	deleteSpec func(ctx context.Context, spec querySpec) (int64, error)
	// dateRange returns the earliest and the latest value of the date or
	// datetime field key of the rows matching spec, zero times when no row
	// has one.
	dateRange func(ctx context.Context, spec querySpec, key string) (first, last time.Time, err error)
	// dates returns the distinct values of the date or datetime field key of
	// the rows matching spec, truncated to kind, ascending: orm.QuerySet.Dates,
	// or DateTimes when datetimes is set.
	dates func(ctx context.Context, spec querySpec, key, kind string, datetimes bool) ([]time.Time, error)
	// facetCounts counts, for each facet, the rows matching spec that match
	// the facet, by facet key, in one query: Django's get_facet_queryset.
	facetCounts func(ctx context.Context, spec querySpec, facets []facet) (map[string]int64, error)
	// get returns the row with the given primary key as a *T, or
	// orm.ErrDoesNotExist when no such row exists.
	get func(ctx context.Context, pk int64) (any, error)
	// create inserts obj (a *T) as a new row, writing back the assigned PK.
	create func(ctx context.Context, obj any) error
	// update writes obj's non-auto fields onto the row with the given PK.
	update func(ctx context.Context, pk int64, obj any) error
	// del removes the row with the given primary key.
	del func(ctx context.Context, pk int64) error
	// changelistSet returns the formset of the list_editable fields with
	// opts and no extra form, Django's get_changelist_formset: over rows,
	// which the changelist has loaded, when spec is nil, else over the rows
	// that spec selects.
	changelistSet func(opts []forms.ModelFormOption, rows []any, spec *querySpec) changelistSet
}

// buildOps constructs the type-erased modelOps for model T against db, capturing
// m so update can build column assignments from the model's non-auto fields.
func buildOps[T any](db *orm.DB, m *orm.Model) modelOps {
	return modelOps{
		newPtr: func() any { return new(T) },

		totallyOrdered: func(ordering []any) bool {
			return len(ordering) > 0 && orm.Query[T](db).OrderBy(ordering...).TotallyOrdered()
		},

		all: func(ctx context.Context, ordering []string) ([]any, error) {
			qs := orm.Query[T](db)
			for _, o := range ordering {
				qs = qs.OrderBy(o)
			}
			rows, err := qs.All(ctx)
			if err != nil {
				return nil, err
			}
			out := make([]any, len(rows))
			for i := range rows {
				// Address into the slice; element addresses are stable for
				// the lifetime of the returned slice so reflection on each
				// *T reads the right struct.
				out[i] = &rows[i]
			}
			return out, nil
		},

		query: func(ctx context.Context, spec querySpec) ([]any, error) {
			qs := applySpec(orm.Query[T](db), spec)
			rows, err := qs.All(ctx)
			if err != nil {
				return nil, err
			}
			out := make([]any, len(rows))
			for i := range rows {
				out[i] = &rows[i]
			}
			return out, nil
		},

		count: func(ctx context.Context, spec querySpec) (int64, error) {
			countSpec := querySpec{
				filters:  spec.filters,
				distinct: spec.distinct,
			}
			return applySpec(orm.Query[T](db), countSpec).Count(ctx)
		},

		pks: func(ctx context.Context, spec querySpec) ([]any, error) {
			pkSpec := querySpec{
				filters:  spec.filters,
				ordering: spec.ordering,
				distinct: spec.distinct,
			}
			return applySpec(orm.Query[T](db), pkSpec).ValuesList("pk").Flat(ctx)
		},

		distinctValues: func(ctx context.Context, spec querySpec, key string) ([]any, error) {
			valSpec := querySpec{
				filters: spec.filters,
			}
			return applySpec(orm.Query[T](db), valSpec).ValuesList(key).Distinct().OrderBy(key).Flat(ctx)
		},

		deleteSpec: func(ctx context.Context, spec querySpec) (int64, error) {
			delSpec := querySpec{
				filters:  spec.filters,
				distinct: spec.distinct,
			}
			return applySpec(orm.Query[T](db), delSpec).Delete(ctx)
		},

		dateRange: func(ctx context.Context, spec querySpec, key string) (time.Time, time.Time, error) {
			res, err := applySpec(orm.Query[T](db), querySpec{filters: spec.filters}).
				Aggregate(ctx, "first", orm.Min(key), "last", orm.Max(key))
			if err != nil {
				return time.Time{}, time.Time{}, err
			}
			return aggregateTime(res["first"]), aggregateTime(res["last"]), nil
		},

		dates: func(ctx context.Context, spec querySpec, key, kind string, datetimes bool) ([]time.Time, error) {
			qs := applySpec(orm.Query[T](db), querySpec{filters: spec.filters})
			if datetimes {
				return qs.DateTimes(ctx, key, kind, "ASC")
			}
			dates, err := qs.Dates(ctx, key, kind, "ASC")
			if err != nil {
				return nil, err
			}
			out := make([]time.Time, len(dates))
			for i, d := range dates {
				out[i] = d.Time()
			}
			return out, nil
		},

		facetCounts: func(ctx context.Context, spec querySpec, facets []facet) (map[string]int64, error) {
			out := make(map[string]int64, len(facets))
			if len(facets) == 0 {
				return out, nil
			}
			qs := applySpec(orm.Query[T](db), querySpec{filters: spec.filters, distinct: spec.distinct})
			args := make([]any, 0, 2*len(facets))
			for _, f := range facets {
				cond := f.cond
				if f.subquery {
					cond = orm.Q("pk__in", qs.Filter(f.cond).ValuesList("pk"))
				}
				args = append(args, f.key, orm.Count("pk").Filter(cond))
			}
			res, err := qs.Aggregate(ctx, args...)
			if err != nil {
				return nil, err
			}
			for k, v := range res {
				out[k], _ = v.(int64)
			}
			return out, nil
		},

		get: func(ctx context.Context, pk int64) (any, error) {
			v, err := orm.Query[T](db).Get(ctx, "id", pk)
			if err != nil {
				return nil, err
			}
			return &v, nil
		},

		create: func(ctx context.Context, obj any) error {
			return orm.Query[T](db).Create(ctx, obj.(*T))
		},

		update: func(ctx context.Context, pk int64, obj any) error {
			// Django's ModelAdmin.save_model calls obj.save(): auto_now
			// fields are stamped and the save signals fire.
			if pkField := m.PrimaryKey(); pkField != nil {
				if f := reflect.ValueOf(obj).Elem().Field(pkField.Index); f.CanInt() {
					f.SetInt(pk)
				}
			}
			return orm.Query[T](db).Save(ctx, obj.(*T))
		},

		del: func(ctx context.Context, pk int64) error {
			_, err := orm.Query[T](db).Filter("id", pk).Delete(ctx)
			return err
		},

		changelistSet: func(opts []forms.ModelFormOption, rows []any, spec *querySpec) changelistSet {
			fs := forms.NewModelFormSet[T](db, opts...)
			fs.Extra = 0
			if spec != nil {
				fs.QuerySet = applySpec(orm.Query[T](db), *spec)
			} else {
				fs.Objects = make([]*T, len(rows))
				for i, row := range rows {
					fs.Objects[i] = row.(*T)
				}
			}
			return &typedChangelistSet[T]{fs: fs}
		},
	}
}

// changelistSet is the type-independent view of the forms.ModelFormSet of a
// changelist with list_editable.
type changelistSet interface {
	// base returns the formset.
	base() *forms.FormSet
	// bind binds the formset to data.
	bind(data url.Values)
	// instances returns the instance of each form, aligned with Forms.
	instances() []any
}

// typedChangelistSet is the changelistSet of a ModelFormSet of T.
type typedChangelistSet[T any] struct {
	fs *forms.ModelFormSet[T]
}

// base returns the formset.
func (t *typedChangelistSet[T]) base() *forms.FormSet { return t.fs.FormSet }

// bind binds the formset to data.
func (t *typedChangelistSet[T]) bind(data url.Values) { t.fs = t.fs.Bind(data) }

// instances returns the instance of each form, aligned with Forms.
func (t *typedChangelistSet[T]) instances() []any {
	mfs := t.fs.ModelForms()
	out := make([]any, len(mfs))
	for i, mf := range mfs {
		out[i] = mf.Instance()
	}
	return out
}

// aggregateTime returns the time of a Min or Max of a date (an orm.Date) or a
// datetime field, the zero time for NULL.
func aggregateTime(v any) time.Time {
	switch v := v.(type) {
	case time.Time:
		return v
	case orm.Date:
		return v.Time()
	}
	return time.Time{}
}
