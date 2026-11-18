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

// Ported from Django: tests/auth_tests/test_middleware.py::TestAuthenticationMiddleware.test_no_password_change_doesnt_invalidate_session
func TestNoPasswordChangeDoesntInvalidateSession(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		user := createUser(t, db, "test_user", "test_password")
		var got *auth.User
		s := newSite(t, db, http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			u, err := auth.GetUser(r)
			must(t, err)
			got = u
		}))
		s.ForceLogin(user)
		s.Get("/")
		if got == nil || got.IsAnonymous() {
			t.Fatalf("request user = %v, want the logged-in user", got)
		}
		djangotest.AssertEqual(t, got.ID, user.ID)
	})
}

// Ported from Django: tests/auth_tests/test_middleware.py::TestAuthenticationMiddleware.test_changed_password_invalidates_session
func TestChangedPasswordInvalidatesSession(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		user := createUser(t, db, "test_user", "test_password")
		var got *auth.User
		s := newSite(t, db, http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			u, err := auth.GetUser(r)
			must(t, err)
			got = u
		}))
		s.ForceLogin(user)
		key := s.Session().Key()

		setPassword(t, db, user, "new_password")
		s.Get("/")
		if got == nil || !got.IsAnonymous() {
			t.Fatalf("request user = %v, want the anonymous user", got)
		}
		// The session is flushed: its cookie and its row are gone.
		if _, ok := s.Cookie(sessionCookieName); ok {
			t.Error("the session cookie was kept")
		}
		if exists, err := s.store.Exists(context.Background(), key); err != nil || exists {
			t.Errorf("session row: exists=%v err=%v, want it removed", exists, err)
		}

		sess := s.Session()
		djangotest.AssertEqual(t, sess.Key(), "")
	})
}

// Django raises ImproperlyConfigured for a request without a session. The Go
// middleware has no settings to blame and treats the request as anonymous.
//
// Ported from Django: tests/auth_tests/test_middleware.py::TestAuthenticationMiddleware.test_no_session
func TestNoSession(t *testing.T) {
	db := newTestDB(t)
	var got *auth.User
	handler := auth.Middleware(db)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		u, err := auth.GetUser(r)
		must(t, err)
		got = u
		if _, ok := auth.CurrentUser(r.Context()); ok {
			t.Error("CurrentUser found a user on a request without a session")
		}
	}))
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", http.NoBody))
	if got == nil || !got.IsAnonymous() {
		t.Fatalf("request user = %v, want the anonymous user", got)
	}
}
