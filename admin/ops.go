package admin

import (
	"context"
	"reflect"

	"github.com/oliverhaas/djangogo/orm"
)

// modelOps is the type-erased set of CRUD operations for one registered model.
// Register[T] builds these closures over the generic ORM so that views and the
// site can manipulate rows without knowing the concrete model type. Every closure
// works with any values that are concretely *T.
type modelOps struct {
	// newPtr returns a fresh *T as an any.
	newPtr func() any
	// all returns every row, ordered by the given OrderBy terms, as a []any
	// whose elements are *T pointing into a stable backing slice.
	all func(ctx context.Context, ordering []string) ([]any, error)
	// get returns the row with the given primary key as a *T, or
	// orm.ErrDoesNotExist when no such row exists.
	get func(ctx context.Context, pk int64) (any, error)
	// create inserts obj (a *T) as a new row, writing back the assigned PK.
	create func(ctx context.Context, obj any) error
	// update writes obj's non-auto fields onto the row with the given PK.
	update func(ctx context.Context, pk int64, obj any) error
	// del removes the row with the given primary key.
	del func(ctx context.Context, pk int64) error
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
			elem := reflect.ValueOf(obj).Elem()
			pkField := m.PrimaryKey()
			assignments := make([]any, 0, len(m.Fields())*2)
			for _, f := range m.Fields() {
				// Skip the auto primary key: it is never updated.
				if pkField != nil && f == pkField && f.Kind == orm.KindAuto {
					continue
				}
				// FK fields are orm.FK[..] values (driver.Valuer); passing
				// them as-is binds the related PK. Scalars pass directly.
				assignments = append(assignments, f.Column, elem.Field(f.Index).Interface())
			}
			_, err := orm.Query[T](db).Filter("id", pk).Update(ctx, assignments...)
			return err
		},

		del: func(ctx context.Context, pk int64) error {
			_, err := orm.Query[T](db).Filter("id", pk).Delete(ctx)
			return err
		},
	}
}
