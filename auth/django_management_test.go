package auth_test

import (
	"context"
	"testing"

	"github.com/oliverhaas/djangogo/auth"
	"github.com/oliverhaas/djangogo/contenttypes"
	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
)

// Report declares a permission beyond the default four, like the model of
// test_default_permissions after Permission._meta.permissions is set.
type Report struct {
	ID    int64
	Title string `orm:"max_length=50"`
}

// Permissions is Django's Meta.permissions.
func (Report) Permissions() []auth.ModelPermission {
	return []auth.ModelPermission{{Codename: "my_custom_permission", Name: "Some permission"}}
}

// CustomOnly has no default permissions, like the model of
// test_default_permissions after Permission._meta.default_permissions is
// emptied.
type CustomOnly struct {
	ID int64
}

// Permissions is Django's Meta.permissions.
func (CustomOnly) Permissions() []auth.ModelPermission {
	return []auth.ModelPermission{{Codename: "my_custom_permission", Name: "Some permission"}}
}

// DefaultPermissions is Django's Meta.default_permissions.
func (CustomOnly) DefaultPermissions() []string { return nil }

// permissionModels is authModels plus the models that declare permissions.
func permissionModels() []any {
	return append(authModels(), djangotest.App("auth_tests", &Report{}, &CustomOnly{}))
}

// countPerms returns the number of permissions of the content type of
// instance.
func countPerms(t *testing.T, db *orm.DB, instance any) int64 {
	t.Helper()
	ctx := context.Background()
	ct, err := contenttypes.GetForInstance(ctx, db, instance)
	must(t, err)
	n, err := orm.Query[auth.Permission](db).Filter("content_type_id", ct.ID).Count(ctx)
	must(t, err)
	return n
}

// Ported from Django: tests/auth_tests/test_management.py::CreatePermissionsTests.test_default_permissions
//
// Django changes the Meta options of Permission between two runs. Model
// options are fixed at compile time here, so two models stand for the two
// states.
func TestCreatePermissionsDefaultPermissions(t *testing.T) {
	djangotest.EachDB(t, permissionModels(), func(t *testing.T, db *orm.DB) {
		_, err := auth.SyncPermissions(context.Background(), db)
		must(t, err)

		// view/add/change/delete permission by default + custom permission
		djangotest.AssertEqual(t, countPerms(t, db, &Report{}), int64(5))
		// custom permission only since default permissions is empty
		djangotest.AssertEqual(t, countPerms(t, db, &CustomOnly{}), int64(1))
	})
}

// Ported from Django: tests/auth_tests/test_management.py::CreatePermissionsTests.test_unavailable_models
//
// #24075 - Permissions shouldn't be created or deleted if the ContentType or
// Permission models aren't available.
func TestCreatePermissionsUnavailableModels(t *testing.T) {
	ctx := context.Background()
	// Unavailable contenttypes.ContentType
	db := djangotest.NewDB(t, auth.App{})
	djangotest.AssertNumQueries(t, db, 0, func() {
		_, err := auth.SyncPermissions(ctx, db)
		must(t, err)
	})
	// Unavailable auth.Permission
	db = djangotest.NewDB(t, contenttypes.App{})
	djangotest.AssertNumQueries(t, db, 0, func() {
		_, err := auth.SyncPermissions(ctx, db)
		must(t, err)
	})
}

// Ported from Django: tests/auth_tests/test_management.py::CreatePermissionsTests.test_create_permissions_checks_contenttypes_created
//
// The content type of Group is cached and then removed from the table, as a
// flush would do. SyncPermissions has to create it again instead of pointing
// permissions at the id that is gone.
func TestCreatePermissionsChecksContentTypesCreated(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		// Warm the cache.
		stale, err := contenttypes.GetForInstance(ctx, db, &auth.Group{})
		must(t, err)
		_, err = orm.Query[contenttypes.ContentType](db).Filter("app_label", "auth", "model", "group").Delete(ctx)
		must(t, err)

		_, err = auth.SyncPermissions(ctx, db)
		must(t, err)

		fresh, err := contenttypes.GetByNaturalKey(ctx, db, "auth", "group")
		must(t, err)
		djangotest.AssertNotEqual(t, fresh.ID, stale.ID)
		djangotest.AssertEqual(t, countPerms(t, db, &auth.Group{}), int64(4))
	})
}

// TestSyncPermissionsNamesAndIdempotence checks the codenames and names of
// the default permissions and that a second run creates nothing.
func TestSyncPermissionsNamesAndIdempotence(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		created, err := auth.SyncPermissions(ctx, db)
		must(t, err)

		ct, err := contenttypes.GetForInstance(ctx, db, &contenttypes.ContentType{})
		must(t, err)
		perms, err := orm.Query[auth.Permission](db).Filter("content_type_id", ct.ID).OrderBy("id").All(ctx)
		must(t, err)
		got := make([][2]string, 0, len(perms))
		for _, p := range perms {
			got = append(got, [2]string{p.Codename, p.Name})
		}
		djangotest.AssertSequenceEqual(t, got, [][2]string{
			{"add_contenttype", "Can add content type"},
			{"change_contenttype", "Can change content type"},
			{"delete_contenttype", "Can delete content type"},
			{"view_contenttype", "Can view content type"},
		})

		// ContentType, User, Group, Permission and the session Record get four
		// permissions each; the three membership tables get none, as in Django.
		djangotest.AssertEqual(t, len(created), 20)
		djangotest.AssertEqual(t, countPerms(t, db, &auth.UserGroup{}), int64(0))

		again, err := auth.SyncPermissions(ctx, db)
		must(t, err)
		djangotest.AssertEqual(t, len(again), 0)
		total, err := orm.Query[auth.Permission](db).Count(ctx)
		must(t, err)
		djangotest.AssertEqual(t, total, int64(20))
	})
}
