package auth

import (
	"context"
	"errors"
	"fmt"

	"github.com/oliverhaas/djangogo/orm"
)

// ErrInvalidCredentials is returned by Authenticate when no backend accepts the
// credentials. It does not say whether the username or the password was wrong.
var ErrInvalidCredentials = errors.New("auth: invalid credentials")

// ErrPermissionDenied is returned by a backend to stop the search: no later
// backend is asked, and the authentication or the permission check fails
// (Django's PermissionDenied).
var ErrPermissionDenied = errors.New("auth: permission denied")

// Backend is one way to authenticate a user (Django's authentication backend).
// The configured backends are asked in order.
type Backend interface {
	// Name identifies the backend. Login stores it in the session, and the
	// user is loaded through the backend of that name on later requests.
	Name() string
	// Authenticate returns the user the credentials belong to. A nil user
	// with a nil error means that the backend does not know the credentials
	// and the next one is asked; ErrPermissionDenied ends the search.
	Authenticate(ctx context.Context, db *orm.DB, username, password string) (*User, error)
	// GetUser returns the user with the given id, or nil when there is none
	// or the backend does not let the user in.
	GetUser(ctx context.Context, db *orm.DB, id int64) (*User, error)
}

// UserPermissionsBackend is implemented by a backend that knows the
// permissions granted to a user directly (Django's get_user_permissions).
type UserPermissionsBackend interface {
	GetUserPermissions(ctx context.Context, db *orm.DB, u *User) ([]string, error)
}

// GroupPermissionsBackend is implemented by a backend that knows the
// permissions a user has through groups (Django's get_group_permissions).
type GroupPermissionsBackend interface {
	GetGroupPermissions(ctx context.Context, db *orm.DB, u *User) ([]string, error)
}

// PermissionChecker is implemented by a backend that decides a single
// permission itself (Django's has_perm). A backend without it holds a
// permission when it is among its user and group permissions. Returning
// ErrPermissionDenied denies the permission without asking later backends.
type PermissionChecker interface {
	HasPerm(ctx context.Context, db *orm.DB, u *User, perm string) (bool, error)
}

// ModulePermissionChecker is implemented by a backend that tells whether a
// user holds any permission of an app (Django's has_module_perms).
type ModulePermissionChecker interface {
	HasModulePerms(ctx context.Context, db *orm.DB, u *User, appLabel string) (bool, error)
}

// BaseBackend is a backend that authenticates nobody (Django's BaseBackend).
// Embed it in a backend that only answers permission checks, or that
// implements one of the two methods, and add the Name method.
type BaseBackend struct{}

// Authenticate knows no credentials.
func (BaseBackend) Authenticate(context.Context, *orm.DB, string, string) (*User, error) {
	return nil, nil
}

// GetUser knows no user.
func (BaseBackend) GetUser(context.Context, *orm.DB, int64) (*User, error) {
	return nil, nil
}

// Authenticate asks the configured backends in order for the user the
// credentials belong to (Django's authenticate). The returned user remembers
// the backend that accepted it. Authenticate returns ErrInvalidCredentials when
// no backend does.
func Authenticate(ctx context.Context, db *orm.DB, username, password string) (*User, error) {
	for _, b := range backends() {
		u, err := b.Authenticate(ctx, db, username, password)
		if errors.Is(err, ErrPermissionDenied) {
			// This backend says to stop in our tracks: the user must not
			// be allowed in at all.
			break
		}
		if err != nil {
			return nil, err
		}
		if u == nil {
			continue
		}
		u.backend = b.Name()
		return u, nil
	}
	return nil, ErrInvalidCredentials
}

// ModelBackend authenticates against the auth_user table and answers
// permission checks from the auth_permission tables (Django's ModelBackend).
// It rejects inactive users.
type ModelBackend struct{}

// Name returns "auth.ModelBackend".
func (ModelBackend) Name() string { return "auth.ModelBackend" }

// Authenticate returns the active user with the given username when the
// password matches, and nil otherwise.
func (ModelBackend) Authenticate(ctx context.Context, db *orm.DB, username, password string) (*User, error) {
	return modelAuthenticate(ctx, db, username, password, false)
}

// GetUser returns the active user with the given id, or nil.
func (ModelBackend) GetUser(ctx context.Context, db *orm.DB, id int64) (*User, error) {
	return modelGetUser(ctx, db, id, false)
}

// GetUserPermissions returns the permissions granted to u directly, as
// "<app_label>.<codename>". An inactive or anonymous user has none, an active
// superuser has all.
func (ModelBackend) GetUserPermissions(ctx context.Context, db *orm.DB, u *User) ([]string, error) {
	set, err := modelPermissions(ctx, db, u, fromUser)
	return sortedPerms(set), err
}

// GetGroupPermissions returns the permissions u has through its groups, with
// the same rules as GetUserPermissions.
func (ModelBackend) GetGroupPermissions(ctx context.Context, db *orm.DB, u *User) ([]string, error) {
	set, err := modelPermissions(ctx, db, u, fromGroup)
	return sortedPerms(set), err
}

// HasPerm reports whether the active user u holds perm.
func (ModelBackend) HasPerm(ctx context.Context, db *orm.DB, u *User, perm string) (bool, error) {
	if u == nil || !u.IsActive {
		return false, nil
	}
	for _, from := range []permSource{fromUser, fromGroup} {
		set, err := modelPermissions(ctx, db, u, from)
		if err != nil {
			return false, err
		}
		if _, ok := set[perm]; ok {
			return true, nil
		}
	}
	return false, nil
}

// HasModulePerms reports whether the active user u holds any permission of
// the app labeled appLabel.
func (ModelBackend) HasModulePerms(ctx context.Context, db *orm.DB, u *User, appLabel string) (bool, error) {
	if u == nil || !u.IsActive {
		return false, nil
	}
	for _, from := range []permSource{fromUser, fromGroup} {
		set, err := modelPermissions(ctx, db, u, from)
		if err != nil {
			return false, err
		}
		for perm := range set {
			if permApp(perm) == appLabel {
				return true, nil
			}
		}
	}
	return false, nil
}

// AllowAllUsersModelBackend is ModelBackend without the check for is_active:
// an inactive user can authenticate (Django's AllowAllUsersModelBackend). Use
// it with a login view that tells an inactive user why the login is refused.
type AllowAllUsersModelBackend struct{ ModelBackend }

// Name returns "auth.AllowAllUsersModelBackend".
func (AllowAllUsersModelBackend) Name() string { return "auth.AllowAllUsersModelBackend" }

// Authenticate returns the user with the given username when the password
// matches, active or not, and nil otherwise.
func (AllowAllUsersModelBackend) Authenticate(ctx context.Context, db *orm.DB, username, password string) (*User, error) {
	return modelAuthenticate(ctx, db, username, password, true)
}

// GetUser returns the user with the given id, active or not, or nil.
func (AllowAllUsersModelBackend) GetUser(ctx context.Context, db *orm.DB, id int64) (*User, error) {
	return modelGetUser(ctx, db, id, true)
}

// modelAuthenticate is the Authenticate of the model backends. allowInactive
// turns the is_active check off.
func modelAuthenticate(ctx context.Context, db *orm.DB, username, password string, allowInactive bool) (*User, error) {
	if username == "" || password == "" {
		return nil, nil
	}
	u, err := orm.Query[User](db).Get(ctx, "username", username)
	if errors.Is(err, orm.ErrDoesNotExist) {
		// Run the default password hasher once to reduce the timing
		// difference between an existing and a nonexistent user (Django
		// ticket #20760).
		_, _ = MakePassword(password)
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("auth: load user: %w", err)
	}
	if !u.CheckPassword(password) {
		return nil, nil
	}
	if !allowInactive && !u.IsActive {
		return nil, nil
	}
	return &u, nil
}

// modelGetUser is the GetUser of the model backends.
func modelGetUser(ctx context.Context, db *orm.DB, id int64, allowInactive bool) (*User, error) {
	u, err := orm.Query[User](db).Get(ctx, "id", id)
	if errors.Is(err, orm.ErrDoesNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("auth: load user: %w", err)
	}
	if !allowInactive && !u.IsActive {
		return nil, nil
	}
	return &u, nil
}
