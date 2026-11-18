package auth_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/oliverhaas/djangogo/auth"
	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
)

// Go has no None password: set_password(None) is SetUnusablePassword, which
// the test covers already.
//
// Ported from Django: tests/auth_tests/test_basic.py::BasicTestCase.test_user
func TestUser(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		u, err := auth.CreateUser(ctx, db, "testuser", "test@example.com", "testpw", false, false, true)
		must(t, err)
		djangotest.AssertEqual(t, u.HasUsablePassword(), true)
		djangotest.AssertEqual(t, u.CheckPassword("bad"), false)
		djangotest.AssertEqual(t, u.CheckPassword("testpw"), true)

		must(t, u.SetUnusablePassword())
		djangotest.AssertEqual(t, u.CheckPassword("testpw"), false)
		djangotest.AssertEqual(t, u.HasUsablePassword(), false)
		must(t, u.SetPassword("testpw"))
		djangotest.AssertEqual(t, u.CheckPassword("testpw"), true)

		djangotest.AssertEqual(t, u.GetUsername(), "testuser")

		djangotest.AssertEqual(t, u.IsAnonymous(), false)
		djangotest.AssertEqual(t, u.IsAuthenticated(), true)
		djangotest.AssertEqual(t, u.IsStaff, false)
		djangotest.AssertEqual(t, u.IsActive, true)
		djangotest.AssertEqual(t, u.IsSuperuser, false)

		u2, err := auth.CreateUser(ctx, db, "testuser2", "test2@example.com", "", false, false, true)
		must(t, err)
		djangotest.AssertEqual(t, u2.HasUsablePassword(), false)
	})
}

// Django fails on the unique constraint; CreateUser looks the name up first
// and returns ErrUserExists.
//
// Ported from Django: tests/auth_tests/test_basic.py::BasicTestCase.test_unicode_username
func TestUnicodeUsername(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		for _, name := range []string{"jörg", "Григорий"} {
			_, err := auth.CreateUser(ctx, db, name, "", "", false, false, true)
			must(t, err)
		}
		// Two usernames that are equal after Unicode normalization are
		// duplicates: U+03A9 GREEK CAPITAL LETTER OMEGA and U+2126 OHM SIGN.
		omega, ohm := "iamthe"+string(rune(0x03A9)), "iamthe"+string(rune(0x2126))
		_, err := auth.CreateUser(ctx, db, ohm, "", "", false, false, true)
		must(t, err)
		_, err = auth.CreateUser(ctx, db, omega, "", "", false, false, true)
		djangotest.AssertErrorIs(t, err, auth.ErrUserExists)
	})
}

// Ported from Django: tests/auth_tests/test_basic.py::BasicTestCase.test_user_no_email
func TestUserNoEmail(t *testing.T) {
	db := newTestDB(t)
	for i, email := range []string{"", " "} {
		u, err := auth.CreateUser(context.Background(), db, fmt.Sprintf("testuser%d", i), email, "", false, false, true)
		must(t, err)
		djangotest.AssertEqual(t, u.Email, "")
	}
}

// Ported from Django: tests/auth_tests/test_basic.py::BasicTestCase.test_superuser
func TestSuperuser(t *testing.T) {
	db := newTestDB(t)
	super, err := auth.CreateSuperuser(context.Background(), db, "super", "super@example.com", "super")
	must(t, err)
	djangotest.AssertEqual(t, super.IsSuperuser, true)
	djangotest.AssertEqual(t, super.IsActive, true)
	djangotest.AssertEqual(t, super.IsStaff, true)
}

// Ported from Django: tests/auth_tests/test_basic.py::BasicTestCase.test_superuser_no_email_or_password
func TestSuperuserNoEmailOrPassword(t *testing.T) {
	db := newTestDB(t)
	superuser, err := auth.CreateSuperuser(context.Background(), db, "super0", "", "")
	must(t, err)
	djangotest.AssertEqual(t, superuser.Email, "")
	djangotest.AssertEqual(t, superuser.HasUsablePassword(), false)
}

// Ported from Django: tests/auth_tests/test_basic.py::TestGetUser.test_get_user_anonymous
func TestGetUserAnonymous(t *testing.T) {
	db := newTestDB(t)
	s := newSite(t, db, whoami)
	if u, ok := auth.UserFromSession(context.Background(), db, s.Session()); ok {
		t.Errorf("UserFromSession = %v, want an anonymous session", u)
	}

	u, err := auth.GetUser(httptest.NewRequest(http.MethodGet, "/", http.NoBody))
	must(t, err)
	if u == nil || !u.IsAnonymous() {
		t.Errorf("GetUser = %v, want the anonymous user", u)
	}
	djangotest.AssertEqual(t, u.String(), "AnonymousUser")
}

// Ported from Django: tests/auth_tests/test_basic.py::TestGetUser.test_get_user
func TestGetUser(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		created := createUser(t, db, "testuser", "testpw")
		s := newSite(t, db, whoami)
		if !s.Login("testuser", "testpw") {
			t.Fatal("login failed")
		}
		u, ok := auth.UserFromSession(context.Background(), db, s.Session())
		if !ok {
			t.Fatal("UserFromSession found no user")
		}
		djangotest.AssertEqual(t, u.Username, created.Username)
	})
}

// Ported from Django: tests/auth_tests/test_basic.py::TestGetUser.test_get_user_fallback_secret
func TestGetUserFallbackSecret(t *testing.T) {
	useSecret(t, "oldsecret")
	db := newTestDB(t)
	ctx := context.Background()
	created := createUser(t, db, "testuser", "testpw")
	s := newSite(t, db, whoami)
	if !s.Login("testuser", "testpw") {
		t.Fatal("login failed")
	}
	sess := s.Session()
	prevKey := sess.Key()

	useSecret(t, "newsecret", "oldsecret")
	u, ok := auth.UserFromSession(ctx, db, sess)
	if !ok {
		t.Fatal("the session signed with the fallback secret was not accepted")
	}
	djangotest.AssertEqual(t, u.Username, created.Username)
	djangotest.AssertNotEqual(t, sess.Key(), prevKey)

	// Without the fallback the session still verifies: its hash was made
	// again with the current secret.
	useSecret(t, "newsecret")
	u, ok = auth.UserFromSession(ctx, db, sess)
	if !ok {
		t.Fatal("the session hash was not updated to the current secret")
	}
	djangotest.AssertEqual(t, u.Username, created.Username)
}
