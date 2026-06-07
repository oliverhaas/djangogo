package auth_test

import (
	"context"
	"testing"

	"github.com/oliverhaas/djangogo/auth"
	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/orm/backends/sqlite"
	"github.com/oliverhaas/djangogo/sessions"
)

// newTestDB registers every auth model, resolves relations, freezes the
// registry, opens a fresh in-memory SQLite database, and creates all tables.
func newTestDB(t *testing.T) *orm.DB {
	t.Helper()

	reg := orm.NewRegistry()
	for _, m := range auth.AppModels() {
		if _, err := reg.Register(m); err != nil {
			t.Fatalf("Register(%T): %v", m, err)
		}
	}
	if err := reg.Resolve(); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	reg.Freeze()

	dsn := "file:" + t.Name() + "?mode=memory&cache=shared"
	sdb, err := sqlite.Open(dsn)
	if err != nil {
		t.Fatalf("sqlite.Open: %v", err)
	}
	t.Cleanup(func() { _ = sdb.Close() })

	db := orm.NewDB(sdb, sqlite.New(), reg)
	ctx := context.Background()
	for _, m := range reg.Models() {
		if err := db.CreateTable(ctx, m); err != nil {
			t.Fatalf("CreateTable(%s): %v", m.Name(), err)
		}
	}
	return db
}

// createUser inserts a user (hashing the given password) and returns it.
func createUser(t *testing.T, db *orm.DB, username, password string) *auth.User {
	t.Helper()
	u := &auth.User{Username: username, IsActive: true}
	if err := u.SetPassword(password); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}
	if err := orm.Query[auth.User](db).Create(context.Background(), u); err != nil {
		t.Fatalf("Create(user): %v", err)
	}
	return u
}

func TestModelsRegisterAndMigrate(t *testing.T) {
	db := newTestDB(t)
	// A trivial insert+read proves the tables exist and round-trip.
	u := createUser(t, db, "alice", "pw")
	got, err := orm.Query[auth.User](db).Get(context.Background(), "id", u.ID)
	if err != nil {
		t.Fatalf("Get(user): %v", err)
	}
	if got.Username != "alice" {
		t.Fatalf("got username %q, want alice", got.Username)
	}
}

func TestUserSetCheckPassword(t *testing.T) {
	var u auth.User
	if err := u.SetPassword("hunter2"); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}
	if u.PasswordHash == "hunter2" {
		t.Fatal("PasswordHash stored the raw password")
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

func TestUserFromSessionJSONFloat(t *testing.T) {
	db := newTestDB(t)
	u := createUser(t, db, "carol", "pw")

	// Simulate a JSON round-trip where the stored id came back as a float64.
	sess := &sessions.Session{}
	sess.Set("_auth_user_id", float64(u.ID))

	loaded, ok := auth.UserFromSession(context.Background(), db, sess)
	if !ok {
		t.Fatal("UserFromSession did not handle a float64 user id")
	}
	if loaded.ID != u.ID {
		t.Fatalf("loaded id %d, want %d", loaded.ID, u.ID)
	}
}
