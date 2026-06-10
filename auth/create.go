package auth

import (
	"context"
	"errors"
	"time"

	"github.com/oliverhaas/djangogo/orm"
)

// ErrEmptyUsername is returned by CreateUser when the username is blank.
var ErrEmptyUsername = errors.New("auth: username must not be empty")

// ErrUserExists is returned by CreateUser when the username is already taken.
var ErrUserExists = errors.New("auth: a user with that username already exists")

// CreateUser hashes password and inserts a new User row carrying the given
// flags, returning the created user with its assigned primary key. It returns
// ErrEmptyUsername for a blank username and ErrUserExists when the username is
// already taken.
func CreateUser(ctx context.Context, db *orm.DB, username, email, password string, isStaff, isSuperuser, isActive bool) (*User, error) {
	if username == "" {
		return nil, ErrEmptyUsername
	}
	switch _, err := orm.Query[User](db).Get(ctx, "username", username); {
	case err == nil:
		return nil, ErrUserExists
	case !errors.Is(err, orm.ErrDoesNotExist):
		return nil, err
	}

	u := &User{
		Username:    username,
		Email:       email,
		IsActive:    isActive,
		IsStaff:     isStaff,
		IsSuperuser: isSuperuser,
		DateJoined:  time.Now().UTC(),
	}
	if err := u.SetPassword(password); err != nil {
		return nil, err
	}
	if err := orm.Query[User](db).Create(ctx, u); err != nil {
		return nil, err
	}
	return u, nil
}

// CreateSuperuser creates an active staff superuser, mirroring Django's
// createsuperuser (is_staff, is_superuser, and is_active all true).
func CreateSuperuser(ctx context.Context, db *orm.DB, username, email, password string) (*User, error) {
	return CreateUser(ctx, db, username, email, password, true, true, true)
}
