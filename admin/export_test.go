package admin

import (
	"context"
	"net/url"

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

// AddPreservedFilters returns u with the preserved filters added for the model
// app.slug of site, as Django's add_preserved_filters template tag.
func AddPreservedFilters(site *AdminSite, app, slug, u, preserved string) string {
	return site.addPreservedFilters(site.byKey[entryKey{app: app, slug: slug}], u, preserved, nil, false, "")
}

// EditedObjectPKs returns the primary keys that data submits for the formset
// with prefix on the changelist of the model app.slug of site, Django's
// ModelAdmin._get_edited_object_pks.
func EditedObjectPKs(site *AdminSite, app, slug string, data url.Values, prefix string) []string {
	e := site.byKey[entryKey{app: app, slug: slug}]
	return editedObjectPKs(data, prefix, e.model.PrimaryKey().Attr)
}

// ListEditableCount counts the rows that the changelist of the model
// app.slug of site binds to the formset with prefix for data, Django's
// ModelAdmin._get_list_editable_queryset(...).count().
func ListEditableCount(ctx context.Context, site *AdminSite, app, slug string, data url.Values, prefix string) (int64, error) {
	e := site.byKey[entryKey{app: app, slug: slug}]
	spec := listEditableSpec(e, querySpec{}, editedObjectPKs(data, prefix, e.model.PrimaryKey().Attr))
	return e.ops.count(ctx, spec)
}

// ViewOnSiteURL returns the URL of the "View on site" link of obj on the
// change page of the model app.slug of site, Django's
// ModelAdmin.get_view_on_site_url.
func ViewOnSiteURL(ctx context.Context, site *AdminSite, app, slug string, obj any) string {
	e := site.byKey[entryKey{app: app, slug: slug}]
	return site.viewOnSiteURL(ctx, e, e.admin.ViewOnSite, e.admin.ViewOnSiteURL, obj)
}
