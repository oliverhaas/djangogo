package admin

import (
	"context"
	"net/url"
	"reflect"

	"github.com/oliverhaas/djangogo/forms"
	"github.com/oliverhaas/djangogo/orm"
)

// querySpec describes a type-independent changelist or action query executed
// through modelOps closures.
type querySpec struct {
	filters       []orm.Cond
	ordering      []string
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
