package auth

import (
	"context"
	"fmt"
	"io/fs"
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
// LastLogin is nil for a user that never logged in. Groups and
// UserPermissions are the many-to-many relations of Django's PermissionsMixin;
// read and change them with orm.Many, or with AddGroups and AddPermissions.
//
// Django defaults is_active to true and date_joined to the current time. A Go
// zero value cannot carry these defaults: CreateUser and the user admin set
// them.
type User struct {
	ID          int64
	Password    string     `orm:"max_length=128"`
	LastLogin   *time.Time `orm:"null;blank"`
	IsSuperuser bool       `orm:"verbose_name=superuser status;help_text=Designates that this user has all permissions without explicitly assigning them."`
	//nolint:lll // The help text is Django's.
	Groups          orm.M2M[Group]      `orm:"blank;related_name=user_set;related_query_name=user;help_text=The groups this user belongs to. A user will get all permissions granted to each of their groups."`
	UserPermissions orm.M2M[Permission] `orm:"blank;related_name=user_set;related_query_name=user;help_text=Specific permissions for this user."`
	Username        string              `orm:"unique;max_length=150;help_text=Required. 150 characters or fewer. Letters, digits and @/./+/-/_ only."`
	FirstName       string              `orm:"max_length=150;blank"`
	LastName        string              `orm:"max_length=150;blank"`
	Email           string              `orm:"max_length=254;blank;verbose_name=email address"`
	IsStaff         bool                `orm:"verbose_name=staff status;help_text=Designates whether the user can log into this admin site."`
	//nolint:lll // The help text is Django's.
	IsActive   bool `orm:"verbose_name=active;help_text=Designates whether this user should be treated as active. Unselect this instead of deleting accounts."`
	DateJoined time.Time

	// backend names the backend that authenticated the user, which Login
	// records in the session (Django's user.backend).
	backend string
	// perms caches the permissions of the user for as long as the value lives,
	// like Django's _perm_cache: load the user again to see a changed
	// permission.
	perms *permCache
}

// Meta names the table as Django does and gives the username Django's
// validator and the message for a taken name.
func (User) Meta() orm.Meta {
	return orm.Meta{
		Table:      "auth_user",
		Validators: map[string][]orm.Validator{"username": {validateUsernameField}},
		ErrorMessages: map[string]map[string]string{
			"username": {"unique": "A user with that username already exists."},
		},
	}
}

// validateUsernameField is ValidateUsername as a validator of the username
// field, with Django's code "invalid".
func validateUsernameField(value any) error {
	if s, ok := value.(string); ok && ValidateUsername(s) == nil {
		return nil
	}
	return orm.NewValidationError(unicodeUsernameMessage, "invalid")
}

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

// NaturalKey returns the one-element slice [username], which identifies a user
// independently of the primary key (Django's natural_key).
func (u *User) NaturalKey() []any { return []any{u.Username} }

// GetByNaturalKey looks up a user by its username natural key (Django's
// BaseUserManager.get_by_natural_key).
func (*User) GetByNaturalKey(ctx context.Context, db *orm.DB, key ...any) (any, error) {
	if len(key) != 1 {
		return nil, orm.ErrDoesNotExist
	}
	username, ok := key[0].(string)
	if !ok {
		return nil, orm.ErrDoesNotExist
	}
	u, err := orm.Query[User](db).Get(ctx, "username", username)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

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
	ID          int64
	Name        string              `orm:"unique;max_length=150"`
	Permissions orm.M2M[Permission] `orm:"blank"`
}

// Meta names the table as Django does.
func (Group) Meta() orm.Meta { return orm.Meta{Table: "auth_group"} }

// String returns the group name.
func (g Group) String() string { return g.Name }

// NaturalKey returns the one-element slice [name] (Django's natural_key).
func (g Group) NaturalKey() []any { return []any{g.Name} }

// GetByNaturalKey looks up a group by its name natural key (Django's
// GroupManager.get_by_natural_key).
func (*Group) GetByNaturalKey(ctx context.Context, db *orm.DB, key ...any) (any, error) {
	if len(key) != 1 {
		return nil, orm.ErrDoesNotExist
	}
	name, ok := key[0].(string)
	if !ok {
		return nil, orm.ErrDoesNotExist
	}
	g, err := orm.Query[Group](db).Get(ctx, "name", name)
	if err != nil {
		return nil, err
	}
	return &g, nil
}

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

// AddPermissions grants perms to u directly (Django's
// user.user_permissions.add). A permission the user already holds is left
// alone. Load the user again to see the change in HasPerm, whose result is
// cached on the value.
func (u *User) AddPermissions(ctx context.Context, db *orm.DB, perms ...*Permission) error {
	if err := orm.Many[Permission](db, u, "user_permissions").Add(ctx, perms...); err != nil {
		return fmt.Errorf("auth: add user permission: %w", err)
	}
	return nil
}

// AddGroups makes u a member of groups (Django's user.groups.add). A group the
// user already belongs to is left alone.
func (u *User) AddGroups(ctx context.Context, db *orm.DB, groups ...*Group) error {
	if err := orm.Many[Group](db, u, "groups").Add(ctx, groups...); err != nil {
		return fmt.Errorf("auth: add user to group: %w", err)
	}
	return nil
}

// AddPermissions grants perms to every member of g (Django's
// group.permissions.add). A permission the group already holds is left alone.
func (g *Group) AddPermissions(ctx context.Context, db *orm.DB, perms ...*Permission) error {
	if err := orm.Many[Permission](db, g, "permissions").Add(ctx, perms...); err != nil {
		return fmt.Errorf("auth: add group permission: %w", err)
	}
	return nil
}

// AppModels returns pointers to zero-valued instances of every auth model, so
// a caller can register and migrate them. The tables of the many-to-many
// relations come with the models that declare them.
func AppModels() []any {
	return []any{
		&User{},
		&Group{},
		&Permission{},
	}
}

// App is the app config of the auth app. Install contenttypes.App next to it:
// permissions belong to content types, and SyncPermissions fails without them.
type App struct{}

// Name returns the app label "auth".
func (App) Name() string { return "auth" }

// VerboseName returns Django's name of the app, "Authentication and
// Authorization".
func (App) VerboseName() string { return "Authentication and Authorization" }

// Models returns the auth models (see AppModels).
func (App) Models() []any { return AppModels() }

// Templates returns the templates of the views (see the function Templates),
// so that the template engine of an application finds them after the
// project's own and in the order of the installed apps (Django's APP_DIRS).
func (App) Templates() fs.FS { return Templates() }
