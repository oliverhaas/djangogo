package auth_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/auth"
	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
)

// callableView is a handler that is not a function (Django's CallableView).
type callableView struct{}

// ServeHTTP answers 200.
func (callableView) ServeHTTP(http.ResponseWriter, *http.Request) {}

// Ported from Django: tests/auth_tests/test_decorators.py::LoginRequiredTestCase.test_callable
func TestDjangoLoginRequiredCallable(t *testing.T) {
	user := &auth.User{ID: 1, Username: "testclient", IsActive: true}
	guard := func(http.Handler) http.Handler { return auth.LoginRequired("")(callableView{}) }
	djangotest.AssertEqual(t, serveGuarded(t, guard, user).Code, http.StatusOK)
}

// Ported from Django: tests/auth_tests/test_decorators.py::LoginRequiredTestCase.test_view
func TestDjangoLoginRequiredView(t *testing.T) {
	user := &auth.User{ID: 1, Username: "testclient", IsActive: true}
	normalView := func(http.ResponseWriter, *http.Request) {}
	guard := func(http.Handler) http.Handler { return auth.LoginRequired("")(http.HandlerFunc(normalView)) }
	djangotest.AssertEqual(t, serveGuarded(t, guard, user).Code, http.StatusOK)
}

// checkLoginRequired is the body of Django's test_login_required: the view
// redirects to loginURL until the client is logged in.
func checkLoginRequired(t *testing.T, db *orm.DB, viewURL, loginURL string) {
	t.Helper()
	user := createUser(t, db, "testclient", "password")
	mux := http.NewServeMux()
	mux.Handle("/login_required/", auth.LoginRequired("")(emptyResponseView))
	mux.Handle("/login_required_login_url/", auth.LoginRequired("/somewhere/")(emptyResponseView))
	s := newSite(t, db, mux)

	resp := s.Get(viewURL)
	djangotest.AssertStatus(t, resp, http.StatusFound)
	if location := resp.Header.Get("Location"); !strings.Contains(location, loginURL) {
		t.Errorf("Location = %q, want it to contain %q", location, loginURL)
	}
	s.ForceLogin(user)
	djangotest.AssertStatus(t, s.Get(viewURL), http.StatusOK)
}

// Ported from Django: tests/auth_tests/test_decorators.py::LoginRequiredTestCase.test_login_required
func TestDjangoLoginRequiredLoginRequired(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		checkLoginRequired(t, db, "/login_required/", auth.DefaultLoginURL)
	})
}

// Ported from Django: tests/auth_tests/test_decorators.py::LoginRequiredTestCase.test_login_required_next_url
func TestDjangoLoginRequiredLoginRequiredNextURL(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		checkLoginRequired(t, db, "/login_required_login_url/", "/somewhere/")
	})
}

// Ported from Django: tests/auth_tests/test_decorators.py::PermissionsRequiredDecoratorTest.test_many_permissions_pass
func TestDjangoPermissionRequiredDecoratorManyPermissionsPass(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		user := joe(t, db, "add_customuser", "change_customuser")
		guard := auth.PermissionsRequired(customUserPerms, auth.WithGuardDB(db))
		djangotest.AssertEqual(t, serveGuarded(t, guard, user).Code, http.StatusOK)
	})
}

// Ported from Django: tests/auth_tests/test_decorators.py::PermissionsRequiredDecoratorTest.test_many_permissions_in_set_pass
//
// There is no set type to pass: the order of the slice must not matter.
func TestDjangoPermissionRequiredDecoratorManyPermissionsInSetPass(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		user := joe(t, db, "add_customuser", "change_customuser")
		reversed := []string{customUserPerms[1], customUserPerms[0]}
		guard := auth.PermissionsRequired(reversed, auth.WithGuardDB(db))
		djangotest.AssertEqual(t, serveGuarded(t, guard, user).Code, http.StatusOK)
	})
}

// Ported from Django: tests/auth_tests/test_decorators.py::PermissionsRequiredDecoratorTest.test_single_permission_pass
func TestDjangoPermissionRequiredDecoratorSinglePermissionPass(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		user := joe(t, db, "add_customuser", "change_customuser")
		guard := auth.PermissionRequired("auth.add_customuser", auth.WithGuardDB(db))
		djangotest.AssertEqual(t, serveGuarded(t, guard, user).Code, http.StatusOK)
	})
}

// Ported from Django: tests/auth_tests/test_decorators.py::PermissionsRequiredDecoratorTest.test_permissioned_denied_redirect
//
// Django's decorator sends the logged-in user to the login page. The guards
// answer like Django's PermissionRequiredMixin instead: 403 for a user that
// is logged in, because a second login cannot give that user the permission.
func TestDjangoPermissionRequiredDecoratorPermissionDeniedRedirect(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		user := joe(t, db, "add_customuser", "change_customuser")
		guard := auth.PermissionsRequired(append(customUserPerms, "nonexistent-permission"), auth.WithGuardDB(db))
		djangotest.AssertEqual(t, serveGuarded(t, guard, user).Code, http.StatusForbidden)
		djangotest.AssertEqual(t, serveGuarded(t, guard, nil).Code, http.StatusFound)
	})
}

// Ported from Django: tests/auth_tests/test_decorators.py::PermissionsRequiredDecoratorTest.test_permissioned_denied_exception_raised
func TestDjangoPermissionRequiredDecoratorPermissionDeniedExceptionRaised(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		user := joe(t, db, "add_customuser", "change_customuser")
		guard := auth.PermissionsRequired(append(customUserPerms, "nonexistent-permission"),
			auth.WithRaiseException(), auth.WithGuardDB(db))
		djangotest.AssertEqual(t, serveGuarded(t, guard, user).Code, http.StatusForbidden)
	})
}

// Ported from Django: tests/auth_tests/test_decorators.py::UserPassesTestDecoratorTest.test_decorator
//
// Django's decorator answers the failing user with a redirect to the login
// page; the guards answer 403 to a user that is logged in, like Django's
// UserPassesTestMixin.
func TestDjangoUserPassesTestDecorator(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		userPass := createUser(t, db, "joe", "qwerty")
		userDeny := createUser(t, db, "jim", "qwerty")
		createGroup(t, db, "Joe group")

		// The test passes for a user with a group whose name starts with
		// the username, ignoring case.
		hasGroup := func(u *auth.User) bool {
			groups, err := orm.Query[auth.Group](db).All(context.Background())
			must(t, err)
			for _, g := range groups {
				if strings.HasPrefix(strings.ToLower(g.Name), strings.ToLower(u.Username)) {
					return true
				}
			}
			return false
		}
		guard := auth.UserPassesTest(hasGroup)
		djangotest.AssertEqual(t, serveGuarded(t, guard, userPass).Code, http.StatusOK)
		djangotest.AssertEqual(t, serveGuarded(t, guard, userDeny).Code, http.StatusForbidden)
	})
}
