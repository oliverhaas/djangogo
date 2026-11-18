package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"golang.org/x/text/unicode/norm"

	"github.com/oliverhaas/djangogo/orm"
)

// ErrEmptyUsername is returned by CreateUser when the username is blank.
var ErrEmptyUsername = errors.New("auth: username must not be empty")

// ErrUserExists is returned by CreateUser when the username is already taken.
var ErrUserExists = errors.New("auth: a user with that username already exists")

// NormalizeEmail lowercases the domain part of an email address and trims
// surrounding space (Django's normalize_email). The local part is kept as it
// is, because it may be case sensitive.
func NormalizeEmail(email string) string {
	email = strings.TrimSpace(email)
	at := strings.LastIndex(email, "@")
	if at < 0 {
		return email
	}
	return email[:at] + "@" + strings.ToLower(email[at+1:])
}

// NormalizeUsername returns the username in Unicode normalization form NFKC
// (Django's normalize_username). Names that look the same but are spelled with
// different code points, such as the ohm sign and the Greek capital omega,
// become one name, so that they cannot be registered as two users.
func NormalizeUsername(username string) string {
	return norm.NFKC.String(username)
}

// CreateUser hashes password and inserts a new user with the given flags,
// returning it with its primary key (Django's create_user). The username is
// normalized with NormalizeUsername and the email with NormalizeEmail. An empty
// password gives the user an unusable password, as create_user does without
// one. CreateUser returns ErrEmptyUsername for a blank username and
// ErrUserExists when the username is taken.
func CreateUser(ctx context.Context, db *orm.DB, username, email, password string, isStaff, isSuperuser, isActive bool) (*User, error) {
	if username == "" {
		return nil, ErrEmptyUsername
	}
	username = NormalizeUsername(username)
	switch _, err := orm.Query[User](db).Get(ctx, "username", username); {
	case err == nil:
		return nil, ErrUserExists
	case !errors.Is(err, orm.ErrDoesNotExist):
		return nil, fmt.Errorf("auth: look up username: %w", err)
	}

	u := &User{
		Username:    username,
		Email:       NormalizeEmail(email),
		IsActive:    isActive,
		IsStaff:     isStaff,
		IsSuperuser: isSuperuser,
		DateJoined:  time.Now().UTC(),
	}
	setPassword := u.SetUnusablePassword
	if password != "" {
		setPassword = func() error { return u.SetPassword(password) }
	}
	if err := setPassword(); err != nil {
		return nil, err
	}
	if err := orm.Query[User](db).Create(ctx, u); err != nil {
		return nil, fmt.Errorf("auth: create user: %w", err)
	}
	return u, nil
}

// CreateSuperuser creates an active staff superuser (Django's
// create_superuser).
func CreateSuperuser(ctx context.Context, db *orm.DB, username, email, password string) (*User, error) {
	return CreateUser(ctx, db, username, email, password, true, true, true)
}
