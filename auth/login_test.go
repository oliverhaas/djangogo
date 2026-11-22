package auth_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"

	"github.com/oliverhaas/djangogo/auth"
	"github.com/oliverhaas/djangogo/csrf"
	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/sessions"
)

// The expected values come from Django 6.0.3:
// salted_hmac("django.contrib.auth.models.AbstractBaseUser.get_session_auth_hash",
// password, secret=secret, algorithm="sha256").hexdigest().
func TestSessionAuthHashMatchesDjango(t *testing.T) {
	tests := []struct{ password, secret, want string }{
		{
			"md5$abc$0123456789abcdef0123456789abcdef", "secret",
			"b4c04fd721259b1f08738023ce85e6d2a10b3bb5e94a5265ece0f3e9e0688818",
		},
		{"", "secret", "f7c7ac4a7c80912c420e0a6f61e213127a3791a5d442fd3ffa83ebb717f497fd"},
		{
			"pbkdf2_sha256$1000000$salt$hash=", "django-insecure-key",
			"703864ef1a2e77ad6a35771b526b8060e5056f2123434b3f2d0f3c8a80130817",
		},
	}
	for _, tc := range tests {
		useSecret(t, tc.secret)
		u := &auth.User{Password: tc.password}
		djangotest.AssertEqual(t, u.SessionAuthHash(), tc.want)
	}
}

func TestSessionAuthHashWithoutSecretKey(t *testing.T) {
	u := &auth.User{Password: "md5$abc$0123456789abcdef0123456789abcdef"}
	useSecret(t, "secret")
	configured := u.SessionAuthHash()

	useSecret(t, "")
	random := u.SessionAuthHash()
	djangotest.AssertEqual(t, len(random), 64)
	djangotest.AssertEqual(t, u.SessionAuthHash(), random)
	djangotest.AssertNotEqual(t, random, configured)
}

func TestLoginStoresDjangoSessionValues(t *testing.T) {
	db := newTestDB(t)
	u := createUser(t, db, "alice", "pw")
	sess := sessions.NewDBStore(db).New()
	auth.Login(sess, u)

	id, _ := sess.Get(auth.SessionKey)
	djangotest.AssertEqual(t, id, any(strconv.FormatInt(u.ID, 10)))
	backend, _ := sess.Get(auth.BackendSessionKey)
	djangotest.AssertEqual(t, backend, any("auth.ModelBackend"))
	hash, _ := sess.Get(auth.HashSessionKey)
	djangotest.AssertEqual(t, hash, any(u.SessionAuthHash()))
	if sess.Key() == "" {
		t.Error("the session got no key at login")
	}
}

func TestLoginKeepsBackendOfAuthenticate(t *testing.T) {
	useBackends(t, permissionDeniedBackendLast...)
	db := newTestDB(t)
	createUser(t, db, "alice", "pw")
	u, err := auth.Authenticate(context.Background(), db, "alice", "pw")
	must(t, err)
	djangotest.AssertEqual(t, u.Backend(), "auth.ModelBackend")

	sess := sessions.NewDBStore(db).New()
	auth.Login(sess, u)
	backend, _ := sess.Get(auth.BackendSessionKey)
	djangotest.AssertEqual(t, backend, any("auth.ModelBackend"))
}

// permissionDeniedBackendLast is a backend list with a second backend after
// ModelBackend, so that the backend of a user is not implied.
var permissionDeniedBackendLast = []auth.Backend{auth.ModelBackend{}, permissionDeniedBackend{}}

// A session key that was known before the login must not name the logged-in
// session (session fixation).
func TestLoginCyclesKeyAndKeepsValues(t *testing.T) {
	db := newTestDB(t)
	u := createUser(t, db, "alice", "pw")
	s := newSite(t, db, whoami)
	sess := s.Session()
	sess.Set("cart", "book")
	s.SaveSession(sess)
	before := s.Session().Key()
	if before == "" {
		t.Fatal("the saved session has no key")
	}

	s.ForceLogin(u)
	after := s.Session()
	djangotest.AssertNotEqual(t, after.Key(), before)
	cart, _ := after.Get("cart")
	djangotest.AssertEqual(t, cart, any("book"))
	if exists, err := s.store.Exists(context.Background(), before); err != nil || exists {
		t.Errorf("row of the old key: exists=%v err=%v, want it removed", exists, err)
	}
}

func TestLoginFlushesSessionOfAnotherUser(t *testing.T) {
	db := newTestDB(t)
	alice, bob := createUser(t, db, "alice", "pw"), createUser(t, db, "bob", "pw")
	s := newSite(t, db, whoami)
	s.ForceLogin(alice)
	sess := s.Session()
	sess.Set("cart", "book")
	s.SaveSession(sess)

	s.ForceLogin(bob)
	if _, ok := s.Session().Get("cart"); ok {
		t.Error("the session of alice leaked its values to bob")
	}
	djangotest.AssertEqual(t, s.Get("/").Content(), "bob")
}

func TestLoginAgainKeepsSessionOfSameUser(t *testing.T) {
	db := newTestDB(t)
	alice := createUser(t, db, "alice", "pw")
	s := newSite(t, db, whoami)
	s.ForceLogin(alice)
	sess := s.Session()
	sess.Set("cart", "book")
	s.SaveSession(sess)
	key := s.Session().Key()

	s.ForceLogin(alice)
	again := s.Session()
	djangotest.AssertEqual(t, again.Key(), key)
	cart, _ := again.Get("cart")
	djangotest.AssertEqual(t, cart, any("book"))
}

// A session that holds the hash of an older password of the same user is
// emptied at login.
func TestLoginFlushesSessionWithOldPasswordHash(t *testing.T) {
	db := newTestDB(t)
	alice := createUser(t, db, "alice", "pw")
	s := newSite(t, db, whoami)
	s.ForceLogin(alice)
	sess := s.Session()
	sess.Set("cart", "book")
	s.SaveSession(sess)

	setPassword(t, db, alice, "new")
	s.ForceLogin(alice)
	if _, ok := s.Session().Get("cart"); ok {
		t.Error("the values of the session with the old password hash were kept")
	}
	djangotest.AssertEqual(t, s.Get("/").Content(), "alice")
}

// loginSite serves a login view at /login/, a logout view at /logout/, a
// password change at /password/ and whoami everywhere else, behind the
// sessions, CSRF and auth middleware.
func loginSite(t *testing.T, db *orm.DB) *site {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/login/", func(w http.ResponseWriter, r *http.Request) {
		u, err := auth.Authenticate(r.Context(), db, r.FormValue("username"), r.FormValue("password"))
		if err != nil {
			http.Error(w, "bad credentials", http.StatusForbidden)
			return
		}
		if err := auth.LoginRequest(r, db, u); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		whoami(w, r)
	})
	mux.HandleFunc("/logout/", func(w http.ResponseWriter, r *http.Request) {
		auth.LogoutRequest(r)
		whoami(w, r)
	})
	mux.HandleFunc("/password/", func(w http.ResponseWriter, r *http.Request) {
		u, ok := auth.CurrentUser(r.Context())
		if !ok {
			http.Error(w, "anonymous", http.StatusForbidden)
			return
		}
		setPassword(t, db, u, r.FormValue("password"))
		if r.FormValue("keep") != "" {
			must(t, auth.UpdateSessionAuthHash(r, u))
		}
		whoami(w, r)
	})
	mux.HandleFunc("/token/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(csrf.GetToken(r)))
	})
	mux.Handle("/", whoami)
	return newSite(t, db, csrf.Middleware(mux))
}

func TestLoginRequest(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		created := createUser(t, db, "alice", "pw")
		if created.LastLogin != nil {
			t.Fatalf("last login of a new user = %v, want nil", created.LastLogin)
		}
		s := loginSite(t, db)
		djangotest.AssertEqual(t, s.Get("/").Content(), "anonymous")

		resp := s.Post("/login/", url.Values{"username": {"alice"}, "password": {"pw"}})
		djangotest.AssertStatus(t, resp, http.StatusOK)
		// The user is known to the request that logged it in.
		djangotest.AssertEqual(t, resp.Content(), "alice")
		djangotest.AssertEqual(t, s.Get("/").Content(), "alice")
		if reload(t, db, created.ID).LastLogin == nil {
			t.Error("the login did not set last_login")
		}

		resp = s.Post("/login/", url.Values{"username": {"alice"}, "password": {"wrong"}})
		djangotest.AssertStatus(t, resp, http.StatusForbidden)
	})
}

// A CSRF secret that was set before the login is not accepted afterwards.
func TestLoginRequestRotatesCSRFSecret(t *testing.T) {
	db := newTestDB(t)
	createUser(t, db, "alice", "pw")
	s := loginSite(t, db)
	s.Get("/token/")
	before, ok := s.Cookie(csrf.DefaultCookieName)
	if !ok {
		t.Fatal("the token view set no CSRF cookie")
	}

	s.Post("/login/", url.Values{"username": {"alice"}, "password": {"pw"}})
	after, ok := s.Cookie(csrf.DefaultCookieName)
	if !ok {
		t.Fatal("the login dropped the CSRF cookie")
	}
	djangotest.AssertNotEqual(t, after.Value, before.Value)
}

func TestLoginRequestWithoutSession(t *testing.T) {
	db := newTestDB(t)
	u := createUser(t, db, "alice", "pw")
	r := httptest.NewRequest(http.MethodPost, "/login/", http.NoBody)
	djangotest.AssertErrorIs(t, auth.LoginRequest(r, db, u), auth.ErrNoSession)
	djangotest.AssertErrorIs(t, auth.UpdateSessionAuthHash(r, u), auth.ErrNoSession)
	// Logging out a request without a session is not an error.
	auth.LogoutRequest(r)
}

func TestLogoutRequest(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		alice := createUser(t, db, "alice", "pw")
		s := loginSite(t, db)
		s.ForceLogin(alice)
		key := s.Session().Key()

		// The request that logs out is anonymous from then on.
		djangotest.AssertEqual(t, s.Post("/logout/", nil).Content(), "anonymous")
		if _, ok := s.Cookie(sessionCookieName); ok {
			t.Error("the logout kept the session cookie")
		}
		if exists, err := s.store.Exists(context.Background(), key); err != nil || exists {
			t.Errorf("session row after logout: exists=%v err=%v, want it removed", exists, err)
		}
		djangotest.AssertEqual(t, s.Get("/").Content(), "anonymous")
	})
}

func TestUpdateSessionAuthHash(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		alice := createUser(t, db, "alice", "pw")
		here, elsewhere := loginSite(t, db), loginSite(t, db)
		here.ForceLogin(alice)
		elsewhere.ForceLogin(alice)
		key := here.Session().Key()

		resp := here.Post("/password/", url.Values{"password": {"new"}, "keep": {"1"}})
		djangotest.AssertEqual(t, resp.Content(), "alice")
		djangotest.AssertEqual(t, here.Get("/").Content(), "alice")
		djangotest.AssertNotEqual(t, here.Session().Key(), key)
		// Every other session of the user ends with the old password.
		djangotest.AssertEqual(t, elsewhere.Get("/").Content(), "anonymous")
	})
}

func TestPasswordChangeWithoutUpdateLogsOut(t *testing.T) {
	db := newTestDB(t)
	alice := createUser(t, db, "alice", "pw")
	s := loginSite(t, db)
	s.ForceLogin(alice)
	s.Post("/password/", url.Values{"password": {"new"}})
	djangotest.AssertEqual(t, s.Get("/").Content(), "anonymous")
}

// UpdateSessionAuthHash for a user other than the one of the session must not
// sign the session for that user's password.
func TestUpdateSessionAuthHashOfAnotherUser(t *testing.T) {
	db := newTestDB(t)
	alice, bob := createUser(t, db, "alice", "pw"), createUser(t, db, "bob", "pw")
	var before, after any
	handler := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		sess, _ := sessions.FromContext(r.Context())
		before, _ = sess.Get(auth.HashSessionKey)
		must(t, auth.UpdateSessionAuthHash(r, bob))
		after, _ = sess.Get(auth.HashSessionKey)
	})
	s := newSite(t, db, handler)
	s.ForceLogin(alice)
	s.Get("/")
	djangotest.AssertEqual(t, after, before)
	djangotest.AssertEqual(t, after, any(alice.SessionAuthHash()))
}

// A request that never asks for the user costs no user query.
func TestUserIsLoadedLazily(t *testing.T) {
	db := newTestDB(t)
	alice := createUser(t, db, "alice", "pw")
	ran := false
	idle, asking := newSite(t, db, okHandler(&ran)), newSite(t, db, whoami)
	idle.ForceLogin(alice)
	asking.ForceLogin(alice)

	djangotest.AssertNumQueries(t, db, 0, func() { idle.Get("/") })
	if !ran {
		t.Fatal("the handler did not run")
	}
	djangotest.AssertNumQueries(t, db, 2, func() {
		djangotest.AssertEqual(t, asking.Get("/").Content(), "alice")
	})
}

// The user is loaded once per request, however often it is asked for.
func TestUserIsLoadedOncePerRequest(t *testing.T) {
	db := newTestDB(t)
	alice := createUser(t, db, "alice", "pw")
	s := newSite(t, db, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		first, _ := auth.CurrentUser(r.Context())
		second, _ := auth.CurrentUser(r.Context())
		third, err := auth.GetUser(r)
		must(t, err)
		if first != second || first != third {
			t.Error("the request returned different user values")
		}
		whoami(w, r)
	}))
	s.ForceLogin(alice)
	djangotest.AssertNumQueries(t, db, 2, func() {
		djangotest.AssertEqual(t, s.Get("/").Content(), "alice")
	})
}

func TestWithUserOverridesSession(t *testing.T) {
	db := newTestDB(t)
	alice, bob := createUser(t, db, "alice", "pw"), createUser(t, db, "bob", "pw")
	mux := http.NewServeMux()
	mux.HandleFunc("/as-bob/", func(w http.ResponseWriter, r *http.Request) {
		whoami(w, r.WithContext(auth.WithUser(r.Context(), bob)))
	})
	mux.HandleFunc("/as-nobody/", func(w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(auth.WithUser(r.Context(), nil))
		u, err := auth.GetUser(r)
		must(t, err)
		if u.IsAuthenticated() {
			t.Errorf("GetUser = %v, want the anonymous user", u)
		}
		whoami(w, r)
	})
	s := newSite(t, db, mux)
	s.ForceLogin(alice)
	djangotest.AssertEqual(t, s.Get("/as-bob/").Content(), "bob")
	djangotest.AssertEqual(t, s.Get("/as-nobody/").Content(), "anonymous")
}

func TestCurrentUserWithoutMiddleware(t *testing.T) {
	if u, ok := auth.CurrentUser(context.Background()); ok || u != nil {
		t.Errorf("CurrentUser = %v, %v, want no user", u, ok)
	}
	u := &auth.User{ID: 7, Username: "alice"}
	got, ok := auth.CurrentUser(auth.WithUser(context.Background(), u))
	if !ok || got != u {
		t.Errorf("CurrentUser = %v, %v, want the user of WithUser", got, ok)
	}
}

// A user that was deactivated after the login is anonymous, as with Django's
// ModelBackend.get_user.
func TestDeactivatedUserIsAnonymous(t *testing.T) {
	db := newTestDB(t)
	alice := createUser(t, db, "alice", "pw")
	s := newSite(t, db, whoami)
	s.ForceLogin(alice)
	djangotest.AssertEqual(t, s.Get("/").Content(), "alice")

	alice.IsActive = false
	saveFlags(t, db, alice)
	djangotest.AssertEqual(t, s.Get("/").Content(), "anonymous")
}

func TestDeletedUserIsAnonymous(t *testing.T) {
	db := newTestDB(t)
	alice := createUser(t, db, "alice", "pw")
	s := newSite(t, db, whoami)
	s.ForceLogin(alice)
	_, err := orm.Query[auth.User](db).Filter("id", alice.ID).Delete(context.Background())
	must(t, err)
	djangotest.AssertEqual(t, s.Get("/").Content(), "anonymous")
}

// A session without an auth hash does not log anyone in, and is emptied.
func TestSessionWithoutHashIsAnonymous(t *testing.T) {
	db := newTestDB(t)
	alice := createUser(t, db, "alice", "pw")
	s := newSite(t, db, whoami)
	sess := s.Session()
	sess.Set(auth.SessionKey, strconv.FormatInt(alice.ID, 10))
	sess.Set(auth.BackendSessionKey, "auth.ModelBackend")
	s.SaveSession(sess)

	djangotest.AssertEqual(t, s.Get("/").Content(), "anonymous")
	if _, ok := s.Cookie(sessionCookieName); ok {
		t.Error("the session without a hash was kept")
	}
}

func TestSessionWithForgedHashIsAnonymous(t *testing.T) {
	db := newTestDB(t)
	alice := createUser(t, db, "alice", "pw")
	s := newSite(t, db, whoami)
	s.ForceLogin(alice)
	sess := s.Session()
	sess.Set(auth.HashSessionKey, "0000000000000000000000000000000000000000000000000000000000000000")
	s.SaveSession(sess)
	djangotest.AssertEqual(t, s.Get("/").Content(), "anonymous")
}

func TestSessionWithMalformedUserIDIsAnonymous(t *testing.T) {
	db := newTestDB(t)
	alice := createUser(t, db, "alice", "pw")
	for _, id := range []any{"abc", "", true, []any{"1"}} {
		s := newSite(t, db, whoami)
		s.ForceLogin(alice)
		sess := s.Session()
		sess.Set(auth.SessionKey, id)
		s.SaveSession(sess)
		djangotest.AssertEqual(t, s.Get("/").Content(), "anonymous")
	}
}

func TestNormalizeUsername(t *testing.T) {
	ohm, omega := string(rune(0x2126)), string(rune(0x03A9))
	djangotest.AssertEqual(t, auth.NormalizeUsername("iamthe"+ohm), "iamthe"+omega)
	djangotest.AssertEqual(t, auth.NormalizeUsername("alice"), "alice")
	// The ligature U+FB01 is the two letters "fi" in form NFKC.
	djangotest.AssertEqual(t, auth.NormalizeUsername(string(rune(0xFB01))+"na"), "fina")
}
