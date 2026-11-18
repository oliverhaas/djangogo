package auth

import (
	"context"
	"errors"

	"github.com/oliverhaas/djangogo/orm"
)

// ErrInvalidCredentials is returned by Authenticate when the username is
// unknown, the password does not match, or the account is inactive. The three
// cases are deliberately indistinguishable to the caller.
var ErrInvalidCredentials = errors.New("auth: invalid credentials")

// Authenticate verifies a username and password and returns the matching active
// user, mirroring Django's ModelBackend.authenticate. It returns
// ErrInvalidCredentials for an unknown username, a wrong password, or an
// inactive account. For an unknown username it still runs the password hasher
// once, so the response time does not reveal whether the user exists.
func Authenticate(ctx context.Context, db *orm.DB, username, password string) (*User, error) {
	u, err := orm.Query[User](db).Get(ctx, "username", username)
	if err != nil {
		if !errors.Is(err, orm.ErrDoesNotExist) {
			return nil, err
		}
		// Run the default hasher once to reduce the timing difference between an
		// existing and a nonexistent user (Django ticket #20760).
		_, _ = MakePassword(password)
		return nil, ErrInvalidCredentials
	}
	if !u.CheckPassword(password) || !u.IsActive {
		return nil, ErrInvalidCredentials
	}
	return &u, nil
}
