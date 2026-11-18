package auth_test

import (
	"context"
	"errors"
	"testing"

	"github.com/oliverhaas/djangogo/auth"
	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
)

// simpleBackend is tests/auth_tests/test_auth_backends.py::SimpleBackend: a
// backend that only lists permissions.
type simpleBackend struct{ auth.BaseBackend }

func (simpleBackend) Name() string { return "auth_test.simpleBackend" }

func (simpleBackend) GetUserPermissions(context.Context, *orm.DB, *auth.User) ([]string, error) {
	return []string{"user_perm"}, nil
}

func (simpleBackend) GetGroupPermissions(context.Context, *orm.DB, *auth.User) ([]string, error) {
	return []string{"group_perm"}, nil
}

// permissionDeniedBackend is
// tests/auth_tests/test_auth_backends.py::PermissionDeniedBackend: it stops
// every authentication and every permission check.
type permissionDeniedBackend struct{}

func (permissionDeniedBackend) Name() string { return "auth_test.permissionDeniedBackend" }

func (permissionDeniedBackend) Authenticate(context.Context, *orm.DB, string, string) (*auth.User, error) {
	return nil, auth.ErrPermissionDenied
}

func (permissionDeniedBackend) GetUser(context.Context, *orm.DB, int64) (*auth.User, error) {
	return nil, nil
}

func (permissionDeniedBackend) HasPerm(context.Context, *orm.DB, *auth.User, string) (bool, error) {
	return false, auth.ErrPermissionDenied
}

func (permissionDeniedBackend) HasModulePerms(context.Context, *orm.DB, *auth.User, string) (bool, error) {
	return false, auth.ErrPermissionDenied
}

// useBackends configures the given backends for the duration of the test.
func useBackends(t *testing.T, backends ...auth.Backend) {
	t.Helper()
	cfg := auth.CurrentConfig()
	cfg.Backends = backends
	t.Cleanup(auth.Configure(cfg))
}

// Ported from Django: tests/auth_tests/test_auth_backends.py::BaseBackendTest.test_get_user_permissions
func TestBaseBackendGetUserPermissions(t *testing.T) {
	useBackends(t, simpleBackend{})
	db := newTestDB(t)
	user := createUser(t, db, "test", "test")
	perms, err := auth.GetUserPermissions(context.Background(), db, user)
	must(t, err)
	djangotest.AssertSequenceEqual(t, perms, []string{"user_perm"})
}

// Ported from Django: tests/auth_tests/test_auth_backends.py::BaseBackendTest.test_get_group_permissions
func TestBaseBackendGetGroupPermissions(t *testing.T) {
	useBackends(t, simpleBackend{})
	db := newTestDB(t)
	user := createUser(t, db, "test", "test")
	perms, err := auth.GetGroupPermissions(context.Background(), db, user)
	must(t, err)
	djangotest.AssertSequenceEqual(t, perms, []string{"group_perm"})
}

// Ported from Django: tests/auth_tests/test_auth_backends.py::BaseBackendTest.test_get_all_permissions
func TestBaseBackendGetAllPermissions(t *testing.T) {
	useBackends(t, simpleBackend{})
	db := newTestDB(t)
	user := createUser(t, db, "test", "test")
	perms, err := auth.GetAllPermissions(context.Background(), db, user)
	must(t, err)
	djangotest.AssertSequenceEqual(t, perms, []string{"group_perm", "user_perm"})
}

// Ported from Django: tests/auth_tests/test_auth_backends.py::BaseBackendTest.test_has_perm
//
// Django also passes an object to has_perm; object permissions are not
// supported, so that line is left out.
func TestBaseBackendHasPerm(t *testing.T) {
	useBackends(t, simpleBackend{})
	db := newTestDB(t)
	ctx := context.Background()
	user := createUser(t, db, "test", "test")
	for perm, want := range map[string]bool{"user_perm": true, "group_perm": true, "other_perm": false} {
		got, err := auth.HasPerm(ctx, db, user, perm)
		must(t, err)
		if got != want {
			t.Errorf("HasPerm(%q) = %v, want %v", perm, got, want)
		}
	}
}

// Ported from Django: tests/auth_tests/test_auth_backends.py::ModelBackendTest.test_has_perm
func TestModelBackendHasPerm(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		id := createUser(t, db, "test", "test").ID
		hasPerm := func(u *auth.User) bool {
			t.Helper()
			ok, err := auth.HasPerm(ctx, db, u, "auth.test")
			must(t, err)
			return ok
		}

		user := reload(t, db, id)
		djangotest.AssertEqual(t, hasPerm(user), false)

		user.IsStaff = true
		saveFlags(t, db, user)
		djangotest.AssertEqual(t, hasPerm(user), false)

		user.IsSuperuser = true
		saveFlags(t, db, user)
		djangotest.AssertEqual(t, hasPerm(user), true)

		user.IsStaff = true
		user.IsSuperuser = true
		user.IsActive = false
		saveFlags(t, db, user)
		djangotest.AssertEqual(t, hasPerm(user), false)
	})
}

// Ported from Django: tests/auth_tests/test_auth_backends.py::ModelBackendTest.test_custom_perms
func TestModelBackendCustomPerms(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		id := createUser(t, db, "test", "test").ID
		all := func(u *auth.User) []string {
			t.Helper()
			perms, err := auth.GetAllPermissions(ctx, db, u)
			must(t, err)
			return perms
		}
		userPerms := func(u *auth.User) []string {
			t.Helper()
			perms, err := auth.GetUserPermissions(ctx, db, u)
			must(t, err)
			return perms
		}
		groupPerms := func(u *auth.User) []string {
			t.Helper()
			perms, err := auth.GetGroupPermissions(ctx, db, u)
			must(t, err)
			return perms
		}
		hasPerms := func(u *auth.User, perms ...string) bool {
			t.Helper()
			ok, err := auth.HasPerms(ctx, db, u, perms...)
			must(t, err)
			return ok
		}
		hasModulePerms := func(u *auth.User, app string) bool {
			t.Helper()
			ok, err := auth.HasModulePerms(ctx, db, u, app)
			must(t, err)
			return ok
		}

		user := reload(t, db, id)
		must(t, user.AddPermissions(ctx, db, groupPerm(t, db, "test", "test")))

		// Reloading the user purges the permission cache.
		user = reload(t, db, id)
		djangotest.AssertSequenceEqual(t, all(user), []string{"auth.test"})
		djangotest.AssertSequenceEqual(t, userPerms(user), []string{"auth.test"})
		djangotest.AssertSequenceEqual(t, groupPerms(user), []string{})
		djangotest.AssertEqual(t, hasModulePerms(user, "Group"), false)
		djangotest.AssertEqual(t, hasModulePerms(user, "auth"), true)

		must(t, user.AddPermissions(ctx, db, groupPerm(t, db, "test2", "test2")))
		must(t, user.AddPermissions(ctx, db, groupPerm(t, db, "test3", "test3")))
		user = reload(t, db, id)
		expectedUserPerms := []string{"auth.test", "auth.test2", "auth.test3"}
		djangotest.AssertSequenceEqual(t, all(user), expectedUserPerms)
		djangotest.AssertEqual(t, hasPerms(user, "test"), false)
		djangotest.AssertEqual(t, hasPerms(user, "auth.test"), true)
		djangotest.AssertEqual(t, hasPerms(user, "auth.test2", "auth.test3"), true)

		group := createGroup(t, db, "test_group")
		must(t, group.AddPermissions(ctx, db, groupPerm(t, db, "test_group", "test_group")))
		must(t, user.AddGroups(ctx, db, group))
		user = reload(t, db, id)
		djangotest.AssertSequenceEqual(t, all(user), []string{"auth.test", "auth.test2", "auth.test3", "auth.test_group"})
		djangotest.AssertSequenceEqual(t, userPerms(user), expectedUserPerms)
		djangotest.AssertSequenceEqual(t, groupPerms(user), []string{"auth.test_group"})
		djangotest.AssertEqual(t, hasPerms(user, "auth.test3", "auth.test_group"), true)

		// The zero User is the anonymous user.
		anonymous := &auth.User{}
		djangotest.AssertEqual(t, hasPerms(anonymous, "test"), false)
		djangotest.AssertEqual(t, hasPerms(anonymous, "auth.test2", "auth.test3"), false)
	})
}

// permissionFixture gives the user one direct permission and one through a
// group, as test_anonymous_has_no_permissions and
// test_inactive_has_no_permissions set up.
func permissionFixture(t *testing.T, db *orm.DB) int64 {
	t.Helper()
	ctx := context.Background()
	user := createUser(t, db, "test", "test")
	must(t, user.AddPermissions(ctx, db, groupPerm(t, db, "test", "test_user")))
	group := createGroup(t, db, "test_group")
	must(t, user.AddGroups(ctx, db, group))
	must(t, group.AddPermissions(ctx, db, groupPerm(t, db, "test2", "test_group")))
	return user.ID
}

// backendPerms returns the user, group and all permissions the model backend
// reports for u.
func backendPerms(t *testing.T, db *orm.DB, u *auth.User) (user, group, all []string) {
	t.Helper()
	ctx := context.Background()
	backend := auth.ModelBackend{}
	user, err := backend.GetUserPermissions(ctx, db, u)
	must(t, err)
	group, err = backend.GetGroupPermissions(ctx, db, u)
	must(t, err)
	all = append(append([]string{}, group...), user...)
	return user, group, all
}

// Ported from Django: tests/auth_tests/test_auth_backends.py::ModelBackendTest.test_anonymous_has_no_permissions
//
// Django patches is_anonymous on the user model. Here the anonymous user is
// the User without an id, so the test asks with such a value.
func TestModelBackendAnonymousHasNoPermissions(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		id := permissionFixture(t, db)
		user, group, all := backendPerms(t, db, reload(t, db, id))
		djangotest.AssertSequenceEqual(t, all, []string{"auth.test_group", "auth.test_user"})
		djangotest.AssertSequenceEqual(t, user, []string{"auth.test_user"})
		djangotest.AssertSequenceEqual(t, group, []string{"auth.test_group"})

		anonymous := reload(t, db, id)
		anonymous.ID = 0
		user, group, all = backendPerms(t, db, anonymous)
		djangotest.AssertSequenceEqual(t, all, []string{})
		djangotest.AssertSequenceEqual(t, user, []string{})
		djangotest.AssertSequenceEqual(t, group, []string{})
	})
}

// Ported from Django: tests/auth_tests/test_auth_backends.py::ModelBackendTest.test_inactive_has_no_permissions
func TestModelBackendInactiveHasNoPermissions(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		id := permissionFixture(t, db)
		u := reload(t, db, id)
		user, group, all := backendPerms(t, db, u)
		djangotest.AssertSequenceEqual(t, all, []string{"auth.test_group", "auth.test_user"})
		djangotest.AssertSequenceEqual(t, user, []string{"auth.test_user"})
		djangotest.AssertSequenceEqual(t, group, []string{"auth.test_group"})

		u.IsActive = false
		saveFlags(t, db, u)
		user, group, all = backendPerms(t, db, u)
		djangotest.AssertSequenceEqual(t, all, []string{})
		djangotest.AssertSequenceEqual(t, user, []string{})
		djangotest.AssertSequenceEqual(t, group, []string{})
	})
}

// Ported from Django: tests/auth_tests/test_auth_backends.py::ModelBackendTest.test_get_all_superuser_permissions
//
// A superuser has all permissions. Refs #14795.
func TestModelBackendGetAllSuperuserPermissions(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		_, err := auth.SyncPermissions(ctx, db)
		must(t, err)
		superuser, err := auth.CreateSuperuser(ctx, db, "test2", "test2@example.com", "test")
		must(t, err)

		perms, err := auth.GetAllPermissions(ctx, db, reload(t, db, superuser.ID))
		must(t, err)
		count, err := orm.Query[auth.Permission](db).Count(ctx)
		must(t, err)
		if count == 0 {
			t.Fatal("SyncPermissions created no permission")
		}
		djangotest.AssertEqual(t, int64(len(perms)), count)
	})
}

// Ported from Django: tests/auth_tests/test_auth_backends.py::ModelBackendTest.test_authenticate_inactive
//
// An inactive user can't authenticate.
func TestAuthenticateInactive(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	user := createUser(t, db, "test", "test")

	got, err := auth.Authenticate(ctx, db, "test", "test")
	if err != nil {
		t.Fatalf("Authenticate(active user): %v", err)
	}
	if got.ID != user.ID {
		t.Errorf("Authenticate returned user %d, want %d", got.ID, user.ID)
	}

	user.IsActive = false
	saveFlags(t, db, user)
	_, err = auth.Authenticate(ctx, db, "test", "test")
	djangotest.AssertErrorIs(t, err, auth.ErrInvalidCredentials)
}

// Ported from Django: tests/auth_tests/test_auth_backends.py::PermissionDeniedBackendTest.test_permission_denied
//
// The user is not authenticated after a backend raises permission denied
// (#2550). Django also checks the user_login_failed signal, which does not
// exist here.
func TestPermissionDeniedBackendStopsAuthentication(t *testing.T) {
	useBackends(t, permissionDeniedBackend{}, auth.ModelBackend{})
	db := newTestDB(t)
	createUser(t, db, "test", "test")
	_, err := auth.Authenticate(context.Background(), db, "test", "test")
	djangotest.AssertErrorIs(t, err, auth.ErrInvalidCredentials)
}

// Ported from Django: tests/auth_tests/test_auth_backends.py::PermissionDeniedBackendTest.test_authenticates
func TestPermissionDeniedBackendAfterModelBackend(t *testing.T) {
	useBackends(t, auth.ModelBackend{}, permissionDeniedBackend{})
	db := newTestDB(t)
	user := createUser(t, db, "test", "test")
	got, err := auth.Authenticate(context.Background(), db, "test", "test")
	must(t, err)
	djangotest.AssertEqual(t, got.ID, user.ID)
	djangotest.AssertEqual(t, got.Backend(), "auth.ModelBackend")
}

// Ported from Django: tests/auth_tests/test_auth_backends.py::PermissionDeniedBackendTest.test_has_perm_denied
func TestPermissionDeniedBackendDeniesPerm(t *testing.T) {
	useBackends(t, permissionDeniedBackend{}, auth.ModelBackend{})
	db := newTestDB(t)
	ctx := context.Background()
	user := createUser(t, db, "test", "test")
	must(t, user.AddPermissions(ctx, db, groupPerm(t, db, "test", "test")))

	ok, err := auth.HasPerm(ctx, db, user, "auth.test")
	must(t, err)
	djangotest.AssertEqual(t, ok, false)
	ok, err = auth.HasModulePerms(ctx, db, user, "auth")
	must(t, err)
	djangotest.AssertEqual(t, ok, false)
}

// Ported from Django: tests/auth_tests/test_auth_backends.py::PermissionDeniedBackendTest.test_has_perm
func TestPermissionDeniedBackendAfterModelBackendHasPerm(t *testing.T) {
	useBackends(t, auth.ModelBackend{}, permissionDeniedBackend{})
	db := newTestDB(t)
	ctx := context.Background()
	user := createUser(t, db, "test", "test")
	must(t, user.AddPermissions(ctx, db, groupPerm(t, db, "test", "test")))

	ok, err := auth.HasPerm(ctx, db, user, "auth.test")
	must(t, err)
	djangotest.AssertEqual(t, ok, true)
	ok, err = auth.HasModulePerms(ctx, db, user, "auth")
	must(t, err)
	djangotest.AssertEqual(t, ok, true)
}

// Ported from Django: tests/auth_tests/test_auth_backends.py::AllowAllUsersModelBackendTest.test_authenticate
func TestAllowAllUsersModelBackendAuthenticate(t *testing.T) {
	useBackends(t, auth.AllowAllUsersModelBackend{})
	db := newTestDB(t)
	ctx := context.Background()
	user, err := auth.CreateUser(ctx, db, "test", "test@example.com", "test", false, false, false)
	must(t, err)

	djangotest.AssertEqual(t, user.IsActive, false)
	got, err := auth.Authenticate(ctx, db, "test", "test")
	must(t, err)
	djangotest.AssertEqual(t, got.ID, user.ID)
}

// TestAuthenticateBackendError checks that an unexpected backend error is
// returned to the caller and not reported as bad credentials.
func TestAuthenticateBackendError(t *testing.T) {
	useBackends(t, failingBackend{})
	db := newTestDB(t)
	_, err := auth.Authenticate(context.Background(), db, "test", "test")
	if !errors.Is(err, errBackend) {
		t.Fatalf("Authenticate error = %v, want errBackend", err)
	}
}

// errBackend is the error failingBackend returns.
var errBackend = errors.New("backend unavailable")

// failingBackend fails every authentication with errBackend.
type failingBackend struct{ auth.BaseBackend }

func (failingBackend) Name() string { return "auth_test.failingBackend" }

func (failingBackend) Authenticate(context.Context, *orm.DB, string, string) (*auth.User, error) {
	return nil, errBackend
}
