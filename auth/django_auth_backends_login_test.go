package auth_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/oliverhaas/djangogo/auth"
	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
)

// customModelBackend and otherModelBackend are
// tests/auth_tests/test_auth_backends.py::CustomModelBackend and
// OtherModelBackend: ModelBackend under other names.
type customModelBackend struct{ auth.ModelBackend }

func (customModelBackend) Name() string { return "auth_test.customModelBackend" }

type otherModelBackend struct{ auth.ModelBackend }

func (otherModelBackend) Name() string { return "auth_test.otherModelBackend" }

// countingHasher is
// tests/auth_tests/test_auth_backends.py::CountingMD5PasswordHasher: it counts
// the passwords it hashes, to store them or to check them.
type countingHasher struct {
	auth.MD5Hasher
	// calls is the number of hash computations.
	calls *int
}

func (h countingHasher) Encode(password string) (string, error) {
	*h.calls++
	return h.MD5Hasher.Encode(password)
}

func (h countingHasher) Verify(password, encoded string) bool {
	*h.calls++
	return h.MD5Hasher.Verify(password, encoded)
}

// loginWithoutAuthenticate logs user in through a view that calls
// LoginRequest for a user that did not come from Authenticate, which is what
// Django's client._login does. It returns the site and the result of
// LoginRequest.
func loginWithoutAuthenticate(t *testing.T, db *orm.DB, user *auth.User) (*site, error) {
	t.Helper()
	var loginErr error
	s := newSite(t, db, http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		loginErr = auth.LoginRequest(r, db, user)
	}))
	s.Post("/", nil)
	return s, loginErr
}

// assertBackendInSession checks the backend stored in the session of the
// client.
func assertBackendInSession(t *testing.T, s *site, backend string) {
	t.Helper()
	got, _ := s.Session().Get(auth.BackendSessionKey)
	djangotest.AssertEqual(t, got, any(backend))
}

// The hasher runs once whether the user exists or not (#20760).
//
// Ported from Django: tests/auth_tests/test_auth_backends.py::ModelBackendTest.test_authentication_timing
func TestAuthenticationTiming(t *testing.T) {
	calls := 0
	cfg := auth.CurrentConfig()
	cfg.PasswordHashers = []auth.Hasher{countingHasher{calls: &calls}}
	t.Cleanup(auth.Configure(cfg))

	db := newTestDB(t)
	ctx := context.Background()
	createUser(t, db, "test", "test")

	calls = 0
	_, err := auth.Authenticate(ctx, db, "test", "test")
	must(t, err)
	djangotest.AssertEqual(t, calls, 1)

	calls = 0
	_, err = auth.Authenticate(ctx, db, "no_such_user", "test")
	djangotest.AssertErrorIs(t, err, auth.ErrInvalidCredentials)
	djangotest.AssertEqual(t, calls, 1)
}

// Removing a backend from the configuration makes the users it logged in
// anonymous.
//
// Ported from Django: tests/auth_tests/test_auth_backends.py::ChangedBackendSettingsTest.test_changed_backend_settings
func TestChangedBackendSettings(t *testing.T) {
	useBackends(t, customModelBackend{})
	db := newTestDB(t)
	createUser(t, db, "test_user", "test_password")
	s := newSite(t, db, whoami)
	if !s.Login("test_user", "test_password") {
		t.Fatal("login failed")
	}
	djangotest.AssertEqual(t, s.Get("/").Content(), "test_user")

	useBackends(t, auth.ModelBackend{})
	if u, ok := auth.UserFromSession(context.Background(), db, s.Session()); ok {
		t.Errorf("UserFromSession = %v, want an anonymous session", u)
	}
	djangotest.AssertEqual(t, s.Get("/").Content(), "anonymous")
}

// Ported from Django: tests/auth_tests/test_auth_backends.py::SelectingBackendTests.test_backend_path_login_without_authenticate_single_backend
func TestBackendPathLoginWithoutAuthenticateSingleBackend(t *testing.T) {
	useBackends(t, customModelBackend{})
	db := newTestDB(t)
	user := createUser(t, db, "username", "password")
	s, err := loginWithoutAuthenticate(t, db, user)
	must(t, err)
	assertBackendInSession(t, s, "auth_test.customModelBackend")
}

// Django raises ValueError; LoginRequest returns ErrMultipleBackends and logs
// nobody in.
//
// Ported from Django: tests/auth_tests/test_auth_backends.py::SelectingBackendTests.test_backend_path_login_without_authenticate_multiple_backends
func TestBackendPathLoginWithoutAuthenticateMultipleBackends(t *testing.T) {
	useBackends(t, customModelBackend{}, otherModelBackend{})
	db := newTestDB(t)
	user := createUser(t, db, "username", "password")
	s, err := loginWithoutAuthenticate(t, db, user)
	djangotest.AssertErrorIs(t, err, auth.ErrMultipleBackends)
	if s.Session().Has(auth.SessionKey) {
		t.Error("the failed login wrote a user into the session")
	}
}

// Django passes the backend to login(); here the caller sets it on the user.
//
// Ported from Django: tests/auth_tests/test_auth_backends.py::SelectingBackendTests.test_backend_path_login_with_explicit_backends
func TestBackendPathLoginWithExplicitBackends(t *testing.T) {
	useBackends(t, customModelBackend{}, otherModelBackend{})
	db := newTestDB(t)
	user := createUser(t, db, "username", "password")
	user.SetBackend("auth_test.otherModelBackend")
	s, err := loginWithoutAuthenticate(t, db, user)
	must(t, err)
	assertBackendInSession(t, s, "auth_test.otherModelBackend")
}

// Ported from Django: tests/auth_tests/test_auth_backends.py::AllowAllUsersModelBackendTest.test_get_user
func TestAllowAllUsersModelBackendGetUser(t *testing.T) {
	useBackends(t, auth.AllowAllUsersModelBackend{})
	db := newTestDB(t)
	ctx := context.Background()
	user, err := auth.CreateUser(ctx, db, "test", "test@example.com", "test", false, false, false)
	must(t, err)

	s := newSite(t, db, whoami)
	s.ForceLogin(user)
	got, ok := auth.UserFromSession(ctx, db, s.Session())
	if !ok {
		t.Fatal("UserFromSession found no user for the inactive user")
	}
	djangotest.AssertEqual(t, got.ID, user.ID)
}
