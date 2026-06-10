package auth_test

import (
	"context"
	"errors"
	"testing"

	"github.com/oliverhaas/djangogo/auth"
	"github.com/oliverhaas/djangogo/orm"
)

func TestCreateUserHashesPasswordAndPersists(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	u, err := auth.CreateUser(ctx, db, "alice", "alice@example.com", "s3cret", true, false, true)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if u.ID == 0 {
		t.Error("CreateUser did not assign a primary key")
	}
	if u.PasswordHash == "s3cret" || u.PasswordHash == "" {
		t.Errorf("password was not hashed: %q", u.PasswordHash)
	}
	if !u.CheckPassword("s3cret") {
		t.Error("CheckPassword failed for the created user")
	}
	if !u.IsStaff || u.IsSuperuser || !u.IsActive {
		t.Errorf("flags = staff:%v super:%v active:%v, want staff:true super:false active:true",
			u.IsStaff, u.IsSuperuser, u.IsActive)
	}

	got, err := orm.Query[auth.User](db).Get(ctx, "username", "alice")
	if err != nil {
		t.Fatalf("reloading created user: %v", err)
	}
	if got.Email != "alice@example.com" {
		t.Errorf("persisted email = %q, want %q", got.Email, "alice@example.com")
	}
}

func TestCreateSuperuserSetsAllFlags(t *testing.T) {
	db := newTestDB(t)
	u, err := auth.CreateSuperuser(context.Background(), db, "root", "root@example.com", "pw")
	if err != nil {
		t.Fatalf("CreateSuperuser: %v", err)
	}
	if !u.IsStaff || !u.IsSuperuser || !u.IsActive {
		t.Errorf("superuser flags = staff:%v super:%v active:%v, want all true",
			u.IsStaff, u.IsSuperuser, u.IsActive)
	}
}

func TestCreateUserRejectsEmptyUsername(t *testing.T) {
	db := newTestDB(t)
	_, err := auth.CreateUser(context.Background(), db, "", "x@example.com", "pw", true, true, true)
	if !errors.Is(err, auth.ErrEmptyUsername) {
		t.Errorf("err = %v, want ErrEmptyUsername", err)
	}
}

func TestCreateUserRejectsDuplicate(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	if _, err := auth.CreateSuperuser(ctx, db, "dup", "a@example.com", "pw"); err != nil {
		t.Fatalf("first CreateSuperuser: %v", err)
	}
	_, err := auth.CreateSuperuser(ctx, db, "dup", "b@example.com", "pw")
	if !errors.Is(err, auth.ErrUserExists) {
		t.Errorf("err = %v, want ErrUserExists", err)
	}
}
