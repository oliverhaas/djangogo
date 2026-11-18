package auth

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/oliverhaas/djangogo/orm"
)

// User is an account, with the fields and the table of Django's AbstractUser
// (auth_user). The zero value stands for the anonymous user: it has no id, is
// not active and holds no permission.
//
// Password holds the encoded hash ("pbkdf2_sha256$..."), never a raw password.
// Use SetPassword and CheckPassword to manage it.
//
// LastLogin is the zero time for a user that never logged in. The ORM stores a
// zero time as year 1 and cannot read NULL into a time.Time, so a row written
// by Django with a NULL last_login cannot be loaded yet.
type User struct {
	ID          int64
	Password    string    `orm:"max_length=128"`
	LastLogin   time.Time `orm:"null;blank"`
	IsSuperuser bool      `orm:"verbose_name=superuser status"`
	Username    string    `orm:"unique;max_length=150"`
	FirstName   string    `orm:"max_length=150;blank"`
	LastName    string    `orm:"max_length=150;blank"`
	Email       string    `orm:"max_length=254;blank;verbose_name=email address"`
	IsStaff     bool      `orm:"verbose_name=staff status"`
	IsActive    bool      `orm:"verbose_name=active"`
	DateJoined  time.Time

	// backend names the backend that authenticated the user, which Login
	// records in the session (Django's user.backend).
	backend string
	// perms caches the permissions of the user for as long as the value lives,
	// like Django's _perm_cache: load the user again to see a changed
	// permission.
	perms *permCache
}

// Meta names the table as Django does.
func (User) Meta() orm.Meta { return orm.Meta{Table: "auth_user"} }

// IsAuthenticated reports whether u is a stored user, as opposed to the
// anonymous user (Django's is_authenticated).
func (u *User) IsAuthenticated() bool { return u != nil && u.ID != 0 }

// IsAnonymous reports whether u is the anonymous user (Django's is_anonymous).
func (u *User) IsAnonymous() bool { return !u.IsAuthenticated() }

// GetUsername returns the username (Django's get_username).
func (u *User) GetUsername() string { return u.Username }

// GetFullName returns the first and the last name with a space in between
// (Django's get_full_name).
func (u *User) GetFullName() string {
	return strings.TrimSpace(u.FirstName + " " + u.LastName)
}

// GetShortName returns the first name (Django's get_short_name).
func (u *User) GetShortName() string { return u.FirstName }

// NaturalKey returns the username, which identifies a user independently of
// the primary key (Django's natural_key).
func (u *User) NaturalKey() string { return u.Username }

// String returns the username, or "AnonymousUser" for the anonymous user.
func (u *User) String() string {
	if u.IsAnonymous() {
		return "AnonymousUser"
	}
	return u.Username
}

// Backend returns the name of the backend that authenticated u, or "" for a
// user that was loaded in another way.
func (u *User) Backend() string { return u.backend }

// SetBackend records the backend that Login stores in the session for u. Call
// it before Login for a user that did not come from Authenticate when more
// than one backend is configured.
func (u *User) SetBackend(name string) { u.backend = name }

// Group is a named set of users that share permissions (auth_group).
type Group struct {
	ID   int64
	Name string `orm:"unique;max_length=150"`
}

// Meta names the table as Django does.
func (Group) Meta() orm.Meta { return orm.Meta{Table: "auth_group"} }

// String returns the group name.
func (g Group) String() string { return g.Name }

// NaturalKey returns the group name (Django's natural_key).
func (g Group) NaturalKey() string { return g.Name }

// Permission is one capability on one model (auth_permission): Codename
// ("add_post") is unique per content type, Name is the readable form ("Can add
// post").
//
// ContentTypeID is the id of the row of django_content_type the permission
// belongs to. It is a plain column and not a foreign key, so that the auth
// models can be registered without the contenttypes app; no permission exists
// then.
type Permission struct {
	ID            int64
	Name          string `orm:"max_length=255"`
	ContentTypeID int64  `orm:"index"`
	Codename      string `orm:"max_length=100"`
}

// Meta names the table as Django does.
func (Permission) Meta() orm.Meta { return orm.Meta{Table: "auth_permission"} }

// UserGroup is the membership of a user in a group (auth_user_groups).
type UserGroup struct {
	ID    int64
	User  orm.FK[User]
	Group orm.FK[Group]
}

// Meta names the table as Django does.
func (UserGroup) Meta() orm.Meta { return orm.Meta{Table: "auth_user_groups"} }

// UserPermission is a permission granted to one user directly
// (auth_user_user_permissions).
type UserPermission struct {
	ID         int64
	User       orm.FK[User]
	Permission orm.FK[Permission]
}

// Meta names the table as Django does.
func (UserPermission) Meta() orm.Meta { return orm.Meta{Table: "auth_user_user_permissions"} }

// GroupPermission is a permission granted to every member of a group
// (auth_group_permissions).
type GroupPermission struct {
	ID         int64
	Group      orm.FK[Group]
	Permission orm.FK[Permission]
}

// Meta names the table as Django does.
func (GroupPermission) Meta() orm.Meta { return orm.Meta{Table: "auth_group_permissions"} }

// AddPermissions grants perms to u directly (Django's
// user.user_permissions.add). A permission the user already holds is left
// alone. Load the user again to see the change in HasPerm, whose result is
// cached on the value.
func (u *User) AddPermissions(ctx context.Context, db *orm.DB, perms ...*Permission) error {
	for _, p := range perms {
		exists, err := orm.Query[UserPermission](db).Filter("user_id", u.ID, "permission_id", p.ID).Exists(ctx)
		if err != nil {
			return fmt.Errorf("auth: add user permission: %w", err)
		}
		if exists {
			continue
		}
		link := &UserPermission{}
		link.User.SetPK(u.ID)
		link.Permission.SetPK(p.ID)
		if err := orm.Query[UserPermission](db).Create(ctx, link); err != nil {
			return fmt.Errorf("auth: add user permission: %w", err)
		}
	}
	return nil
}

// AddGroups makes u a member of groups (Django's user.groups.add). A group the
// user already belongs to is left alone.
func (u *User) AddGroups(ctx context.Context, db *orm.DB, groups ...*Group) error {
	for _, g := range groups {
		exists, err := orm.Query[UserGroup](db).Filter("user_id", u.ID, "group_id", g.ID).Exists(ctx)
		if err != nil {
			return fmt.Errorf("auth: add user to group: %w", err)
		}
		if exists {
			continue
		}
		link := &UserGroup{}
		link.User.SetPK(u.ID)
		link.Group.SetPK(g.ID)
		if err := orm.Query[UserGroup](db).Create(ctx, link); err != nil {
			return fmt.Errorf("auth: add user to group: %w", err)
		}
	}
	return nil
}

// AddPermissions grants perms to every member of g (Django's
// group.permissions.add). A permission the group already holds is left alone.
func (g *Group) AddPermissions(ctx context.Context, db *orm.DB, perms ...*Permission) error {
	for _, p := range perms {
		exists, err := orm.Query[GroupPermission](db).Filter("group_id", g.ID, "permission_id", p.ID).Exists(ctx)
		if err != nil {
			return fmt.Errorf("auth: add group permission: %w", err)
		}
		if exists {
			continue
		}
		link := &GroupPermission{}
		link.Group.SetPK(g.ID)
		link.Permission.SetPK(p.ID)
		if err := orm.Query[GroupPermission](db).Create(ctx, link); err != nil {
			return fmt.Errorf("auth: add group permission: %w", err)
		}
	}
	return nil
}

// AppModels returns pointers to zero-valued instances of every auth model, in
// dependency order (referenced models before their referrers), so a caller can
// register and migrate them.
func AppModels() []any {
	return []any{
		&User{},
		&Group{},
		&Permission{},
		&UserGroup{},
		&UserPermission{},
		&GroupPermission{},
	}
}

// App is the app config of the auth app. Install contenttypes.App next to it:
// permissions belong to content types, and SyncPermissions fails without them.
type App struct{}

// Name returns the app label "auth".
func (App) Name() string { return "auth" }

// Models returns the auth models (see AppModels).
func (App) Models() []any { return AppModels() }
