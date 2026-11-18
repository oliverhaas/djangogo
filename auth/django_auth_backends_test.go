package auth_test

import (
	"context"
	"errors"
	"testing"

	"github.com/oliverhaas/djangogo/auth"
	"github.com/oliverhaas/djangogo/orm"
)

// Ported from Django: tests/auth_tests/test_auth_backends.py::ModelBackendTest.test_authenticate_inactive
//
// An inactive user can't authenticate.
func TestAuthenticateInactive(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	user := createUser(t, db, "test", "test")

	got, err := auth.Authenticate(ctx, db, "test", "test")
	if err != nil {
		t.Fatalf("Authenticate(active user): %v", err)
	}
	if got.ID != user.ID {
		t.Errorf("Authenticate returned user %d, want %d", got.ID, user.ID)
	}

	if _, err := orm.Query[auth.User](db).Filter("id", user.ID).Update(ctx, "is_active", false); err != nil {
		t.Fatalf("Update(is_active): %v", err)
	}
	if _, err := auth.Authenticate(ctx, db, "test", "test"); !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Errorf("Authenticate(inactive user) error = %v, want ErrInvalidCredentials", err)
	}
}

// Ported from Django: tests/auth_tests/test_auth_backends.py::BaseModelBackendTest.test_authentication_timing
//
// Django counts hasher calls to prove the hasher runs once whether or not the
// user exists (refs #20760). The hasher here is not pluggable yet, so this port
// asserts the observable contract instead: an unknown user and a wrong password
// are reported with the same error.
func TestAuthenticateUnknownUserAndWrongPassword(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	createUser(t, db, "test", "test")

	if _, err := auth.Authenticate(ctx, db, "no_such_user", "test"); !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Errorf("Authenticate(unknown user) error = %v, want ErrInvalidCredentials", err)
	}
	if _, err := auth.Authenticate(ctx, db, "test", "wrong"); !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Errorf("Authenticate(wrong password) error = %v, want ErrInvalidCredentials", err)
	}
}
