package auth_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/oliverhaas/djangogo/auth"
	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
)

// emptyResponseView answers 200 with an empty body (Django's
// EmptyResponseView).
var emptyResponseView = http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})

// Tests of Django's UserPassesTestMixin with a fixed outcome.
var (
	// alwaysTrue lets every user through.
	alwaysTrue = func(*auth.User) bool { return true }
	// alwaysFalse lets no user through.
	alwaysFalse = func(*auth.User) bool { return false }
)

// serveGuarded sends GET /rand on the host testserver through guard, as user;
// a nil user is an anonymous visitor (Django's RequestFactory with
// request.user set).
func serveGuarded(t *testing.T, guard func(http.Handler) http.Handler, user *auth.User) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/rand", nil)
	req.Host = "testserver"
	if user != nil {
		req = req.WithContext(auth.WithUser(req.Context(), user))
	}
	rec := httptest.NewRecorder()
	guard(emptyResponseView).ServeHTTP(rec, req)
	return rec
}

// assertGuardRedirect checks that an anonymous visitor is sent to want.
func assertGuardRedirect(t *testing.T, guard func(http.Handler) http.Handler, want string) {
	t.Helper()
	rec := serveGuarded(t, guard, nil)
	djangotest.AssertEqual(t, rec.Code, http.StatusFound)
	djangotest.AssertEqual(t, rec.Header().Get("Location"), want)
}

// customUserPerms are the two permissions of the Django tests, as permission
// strings of the auth app.
var customUserPerms = []string{"auth.add_customuser", "auth.change_customuser"}

// joe creates the user joe with the permissions named by codenames.
func joe(t *testing.T, db *orm.DB, codenames ...string) *auth.User {
	t.Helper()
	user := createUser(t, db, "joe", "qwerty")
	for _, codename := range codenames {
		must(t, user.AddPermissions(context.Background(), db, groupPerm(t, db, "Can "+codename, codename)))
	}
	return user
}

// stackedGuards returns Django's StackedMixinsView1 and StackedMixinsView2:
// the login and the permission guard in both orders, both raising.
func stackedGuards(db *orm.DB) []func(http.Handler) http.Handler {
	login := auth.LoginRequired("", auth.WithRaiseException())
	perms := auth.PermissionsRequired(customUserPerms, auth.WithRaiseException(), auth.WithGuardDB(db))
	return []func(http.Handler) http.Handler{
		func(next http.Handler) http.Handler { return login(perms(next)) },
		func(next http.Handler) http.Handler { return perms(login(next)) },
	}
}

// Ported from Django: tests/auth_tests/test_mixins.py::AccessMixinTests.test_stacked_mixins_success
func TestDjangoAccessMixinStackedMixinsSuccess(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		user := joe(t, db, "add_customuser", "change_customuser")
		for _, guard := range stackedGuards(db) {
			djangotest.AssertEqual(t, serveGuarded(t, guard, user).Code, http.StatusOK)
		}
	})
}

// Ported from Django: tests/auth_tests/test_mixins.py::AccessMixinTests.test_stacked_mixins_missing_permission
func TestDjangoAccessMixinStackedMixinsMissingPermission(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		user := joe(t, db, "add_customuser")
		for _, guard := range stackedGuards(db) {
			djangotest.AssertEqual(t, serveGuarded(t, guard, user).Code, http.StatusForbidden)
		}
	})
}

// Ported from Django: tests/auth_tests/test_mixins.py::AccessMixinTests.test_access_mixin_permission_denied_response
func TestDjangoAccessMixinPermissionDeniedResponse(t *testing.T) {
	db := newTestDB(t)
	guard := auth.UserPassesTest(alwaysFalse)
	// A logged-in user gets 403.
	djangotest.AssertEqual(t, serveGuarded(t, guard, joe(t, db)).Code, http.StatusForbidden)
	// An anonymous user is sent to the login page.
	assertGuardRedirect(t, guard, "/accounts/login/?next=/rand")
}

// Ported from Django: tests/auth_tests/test_mixins.py::AccessMixinTests.test_access_mixin_permission_denied_remote_login_url
func TestDjangoAccessMixinPermissionDeniedRemoteLoginURL(t *testing.T) {
	guard := auth.UserPassesTest(alwaysFalse, auth.WithLoginURL("https://www.remote.example.com/login"))
	assertGuardRedirect(t, guard, "https://www.remote.example.com/login?next=http%3A//testserver/rand")
}

// Ported from Django: tests/auth_tests/test_mixins.py::AccessMixinTests.test_stacked_mixins_not_logged_in
//
// Django patches is_authenticated of a user with the permissions to False.
// The anonymous visitor is the only user here that is not authenticated.
func TestDjangoAccessMixinStackedMixinsNotLoggedIn(t *testing.T) {
	db := newTestDB(t)
	for _, guard := range stackedGuards(db) {
		djangotest.AssertEqual(t, serveGuarded(t, guard, nil).Code, http.StatusForbidden)
	}
}

// Ported from Django: tests/auth_tests/test_mixins.py::UserPassesTestTests.test_default
func TestDjangoUserPassesTestDefault(t *testing.T) {
	assertGuardRedirect(t, auth.UserPassesTest(alwaysFalse), "/accounts/login/?next=/rand")
}

// Ported from Django: tests/auth_tests/test_mixins.py::UserPassesTestTests.test_custom_redirect_url
func TestDjangoUserPassesTestCustomRedirectURL(t *testing.T) {
	guard := auth.UserPassesTest(alwaysFalse, auth.WithLoginURL("/login/"))
	assertGuardRedirect(t, guard, "/login/?next=/rand")
}

// Ported from Django: tests/auth_tests/test_mixins.py::UserPassesTestTests.test_custom_redirect_parameter
func TestDjangoUserPassesTestCustomRedirectParameter(t *testing.T) {
	guard := auth.UserPassesTest(alwaysFalse, auth.WithRedirectFieldName("goto"))
	assertGuardRedirect(t, guard, "/accounts/login/?goto=/rand")
}

// Ported from Django: tests/auth_tests/test_mixins.py::UserPassesTestTests.test_no_redirect_parameter
func TestDjangoUserPassesTestNoRedirectParameter(t *testing.T) {
	guard := auth.UserPassesTest(alwaysFalse, auth.WithRedirectFieldName(""))
	assertGuardRedirect(t, guard, "/accounts/login/")
}

// Ported from Django: tests/auth_tests/test_mixins.py::UserPassesTestTests.test_raise_exception
func TestDjangoUserPassesTestRaiseException(t *testing.T) {
	rec := serveGuarded(t, auth.UserPassesTest(alwaysFalse, auth.WithRaiseException()), nil)
	djangotest.AssertEqual(t, rec.Code, http.StatusForbidden)
	djangotest.AssertEqual(t, rec.Body.String(), "<h1>403 Forbidden</h1>")
}

// Ported from Django: tests/auth_tests/test_mixins.py::UserPassesTestTests.test_raise_exception_custom_message
//
// Django carries the message in the PermissionDenied exception. Here the
// handler of the 403 page reads it from the request.
func TestDjangoUserPassesTestRaiseExceptionCustomMessage(t *testing.T) {
	const msg = "You don't have access here"
	var got string
	page := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = auth.PermissionDeniedMessage(r.Context())
		w.WriteHeader(http.StatusForbidden)
	})
	guard := auth.UserPassesTest(alwaysFalse, auth.WithRaiseException(),
		auth.WithPermissionDeniedMessage(msg), auth.WithForbiddenHandler(page))
	djangotest.AssertEqual(t, serveGuarded(t, guard, nil).Code, http.StatusForbidden)
	djangotest.AssertEqual(t, got, msg)
}

// Ported from Django: tests/auth_tests/test_mixins.py::UserPassesTestTests.test_user_passes
func TestDjangoUserPassesTestUserPasses(t *testing.T) {
	djangotest.AssertEqual(t, serveGuarded(t, auth.UserPassesTest(alwaysTrue), nil).Code, http.StatusOK)
}

// Ported from Django: tests/auth_tests/test_mixins.py::LoginRequiredMixinTests.test_login_required
func TestDjangoLoginRequiredMixinLoginRequired(t *testing.T) {
	db := newTestDB(t)
	guard := auth.LoginRequired("")
	assertGuardRedirect(t, guard, "/accounts/login/?next=/rand")
	djangotest.AssertEqual(t, serveGuarded(t, guard, joe(t, db)).Code, http.StatusOK)
}

// Ported from Django: tests/auth_tests/test_mixins.py::PermissionsRequiredMixinTests.test_many_permissions_pass
func TestDjangoPermissionsRequiredMixinManyPermissionsPass(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		user := joe(t, db, "add_customuser", "change_customuser")
		guard := auth.PermissionsRequired(customUserPerms, auth.WithGuardDB(db))
		djangotest.AssertEqual(t, serveGuarded(t, guard, user).Code, http.StatusOK)
	})
}

// Ported from Django: tests/auth_tests/test_mixins.py::PermissionsRequiredMixinTests.test_single_permission_pass
func TestDjangoPermissionsRequiredMixinSinglePermissionPass(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		user := joe(t, db, "add_customuser", "change_customuser")
		guard := auth.PermissionRequired("auth.add_customuser", auth.WithGuardDB(db))
		djangotest.AssertEqual(t, serveGuarded(t, guard, user).Code, http.StatusOK)
	})
}

// Ported from Django: tests/auth_tests/test_mixins.py::PermissionsRequiredMixinTests.test_permissioned_denied_redirect
func TestDjangoPermissionsRequiredMixinPermissionDeniedRedirect(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		user := joe(t, db, "add_customuser", "change_customuser")
		guard := auth.PermissionsRequired(append(customUserPerms, "nonexistent-permission"), auth.WithGuardDB(db))
		// A logged-in user gets 403.
		djangotest.AssertEqual(t, serveGuarded(t, guard, user).Code, http.StatusForbidden)
		// An anonymous user is sent to the login page.
		djangotest.AssertEqual(t, serveGuarded(t, guard, nil).Code, http.StatusFound)
	})
}

// Ported from Django: tests/auth_tests/test_mixins.py::PermissionsRequiredMixinTests.test_permissioned_denied_exception_raised
func TestDjangoPermissionsRequiredMixinPermissionDeniedExceptionRaised(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		user := joe(t, db, "add_customuser", "change_customuser")
		guard := auth.PermissionsRequired(append(customUserPerms, "nonexistent-permission"),
			auth.WithRaiseException(), auth.WithGuardDB(db))
		djangotest.AssertEqual(t, serveGuarded(t, guard, user).Code, http.StatusForbidden)
	})
}
