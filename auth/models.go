package auth

import (
	"time"

	"github.com/oliverhaas/djangogo/orm"
)

// User is an authenticatable account. PasswordHash holds the encoded
// "pbkdf2_sha256$..." string; never store a raw password in it. Use SetPassword
// and CheckPassword to manage it.
type User struct {
	ID           int64
	Username     string `orm:"unique;max_length=150"`
	PasswordHash string `orm:"max_length=255"`
	Email        string `orm:"max_length=254"`
	IsActive     bool
	IsStaff      bool
	IsSuperuser  bool
	DateJoined   time.Time
}

// Group is a named collection of users that can be granted permissions in bulk.
type Group struct {
	ID   int64
	Name string `orm:"unique;max_length=150"`
}

// Permission is a single named capability identified by Codename and scoped to a
// ContentType (the target model's name).
type Permission struct {
	ID          int64
	Codename    string `orm:"max_length=100"`
	Name        string `orm:"max_length=255"`
	ContentType string `orm:"max_length=100"`
}

// UserGroup links a User to a Group (the through model for user-group membership).
type UserGroup struct {
	ID    int64
	User  orm.FK[User]
	Group orm.FK[Group]
}

// UserPermission links a User to a Permission granted directly to that user.
type UserPermission struct {
	ID         int64
	User       orm.FK[User]
	Permission orm.FK[Permission]
}

// GroupPermission links a Group to a Permission granted to every member of the group.
type GroupPermission struct {
	ID         int64
	Group      orm.FK[Group]
	Permission orm.FK[Permission]
}

// SetPassword hashes raw and stores the result in u.PasswordHash.
func (u *User) SetPassword(raw string) error {
	encoded, err := MakePassword(raw)
	if err != nil {
		return err
	}
	u.PasswordHash = encoded
	return nil
}

// CheckPassword reports whether raw matches u's stored password hash.
func (u *User) CheckPassword(raw string) bool {
	return CheckPassword(raw, u.PasswordHash)
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

// App is a minimal apps.Config + apps.ModelProvider for the auth models, so the
// app registry can mount auth and migrate its tables.
type App struct{}

// Name returns the app label "auth".
func (App) Name() string { return "auth" }

// Models returns the auth models (see AppModels).
func (App) Models() []any { return AppModels() }
