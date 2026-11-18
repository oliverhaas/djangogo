package auth_test

import (
	"context"
	"testing"

	"github.com/oliverhaas/djangogo/auth"
	"github.com/oliverhaas/djangogo/orm"
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
