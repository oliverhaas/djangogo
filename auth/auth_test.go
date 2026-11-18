package auth_test

import (
	"context"
	"testing"

	"github.com/oliverhaas/djangogo/auth"
	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/sessions"
)

func TestModelsRegisterAndMigrate(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		// A trivial insert+read proves the tables exist and round-trip.
		u := createUser(t, db, "alice", "pw")
		got, err := orm.Query[auth.User](db).Get(context.Background(), "id", u.ID)
		if err != nil {
			t.Fatalf("Get(user): %v", err)
		}
		if got.Username != "alice" {
			t.Fatalf("got username %q, want alice", got.Username)
		}
	})
}

func TestUserSetCheckPassword(t *testing.T) {
	var u auth.User
	if err := u.SetPassword("hunter2"); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}
	if u.Password == "hunter2" {
		t.Fatal("Password stored the raw password")
	}
	if !u.CheckPassword("hunter2") {
		t.Fatal("CheckPassword false for the correct password")
	}
	if u.CheckPassword("hunter3") {
		t.Fatal("CheckPassword true for the wrong password")
	}
}

func TestLoginRotatesAndSetsUser(t *testing.T) {
	db := newTestDB(t)
	u := createUser(t, db, "bob", "pw")

	sess := &sessions.Session{}
	sessions.Rotate(sess) // give it an initial key as if it were persisted once
	before := sess.Key()

	auth.Login(sess, u)
	if sess.Key() == before {
		t.Fatal("Login did not rotate the session key")
	}
	if v, ok := sess.Get("_auth_user_id"); !ok || v.(int64) != u.ID {
		t.Fatalf("session _auth_user_id = %v (ok=%v), want %d", v, ok, u.ID)
	}

	loaded, ok := auth.UserFromSession(context.Background(), db, sess)
	if !ok {
		t.Fatal("UserFromSession did not load the logged-in user")
	}
	if loaded.Username != "bob" {
		t.Fatalf("loaded user %q, want bob", loaded.Username)
	}

	auth.Logout(sess)
	if _, ok := sess.Get("_auth_user_id"); ok {
		t.Fatal("Logout did not clear _auth_user_id")
	}
	if _, ok := auth.UserFromSession(context.Background(), db, sess); ok {
		t.Fatal("UserFromSession returned a user after Logout")
	}
}

func TestUserFromSessionRejectsInactiveUser(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	// A deactivated account with a valid session must not load as the request
	// user: Django's ModelBackend.get_user rejects is_active=False via
	// user_can_authenticate, so request.user becomes AnonymousUser.
	u, err := auth.CreateUser(ctx, db, "deactivated", "", "pw", false, false, false)
	must(t, err)

	sess := &sessions.Session{}
	auth.Login(sess, u)

	if _, ok := auth.UserFromSession(ctx, db, sess); ok {
		t.Fatal("UserFromSession loaded a deactivated user")
	}
}
