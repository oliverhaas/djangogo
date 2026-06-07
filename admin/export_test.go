package admin

import (
	"context"

	"github.com/oliverhaas/djangogo/orm"
)

// TestOps exposes the unexported modelOps closures to external tests so the
// type-erased CRUD operations can be exercised directly. It exists only in test
// builds.
type TestOps struct {
	ops modelOps
}

// OpsFor builds the type-erased operations for model T against db, mirroring what
// Register[T] captures, and wraps them for external tests.
func OpsFor[T any](db *orm.DB) TestOps {
	m, ok := db.Registry().ModelOf(*new(T))
	if !ok {
		panic("admin: OpsFor: no model registered")
	}
	return TestOps{ops: buildOps[T](db, m)}
}

// NewPtr returns a fresh *T as an any.
func (t TestOps) NewPtr() any { return t.ops.newPtr() }

// All returns every row ordered by ordering, as []any of *T.
func (t TestOps) All(ctx context.Context, ordering []string) ([]any, error) {
	return t.ops.all(ctx, ordering)
}

// Get returns the *T with the given primary key, or orm.ErrDoesNotExist.
func (t TestOps) Get(ctx context.Context, pk int64) (any, error) {
	return t.ops.get(ctx, pk)
}

// Create inserts obj (a *T).
func (t TestOps) Create(ctx context.Context, obj any) error {
	return t.ops.create(ctx, obj)
}

// Update writes obj's fields onto the row with the given primary key.
func (t TestOps) Update(ctx context.Context, pk int64, obj any) error {
	return t.ops.update(ctx, pk, obj)
}

// Del removes the row with the given primary key.
func (t TestOps) Del(ctx context.Context, pk int64) error {
	return t.ops.del(ctx, pk)
}
