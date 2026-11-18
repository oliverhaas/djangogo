package auth

import (
	"context"
	"fmt"

	"github.com/oliverhaas/djangogo/orm"
)

// SetPassword hashes raw with the preferred hasher and stores the result in
// u.Password (Django's set_password). It does not save the user.
func (u *User) SetPassword(raw string) error {
	encoded, err := MakePassword(raw)
	if err != nil {
		return err
	}
	u.Password = encoded
	return nil
}

// ChangePassword hashes raw, makes it the password of u and stores it
// (Django's set_password followed by save). Sessions of u that were logged in
// with the old password end; UpdateSessionAuthHash keeps the current one.
func ChangePassword(ctx context.Context, db *orm.DB, u *User, raw string) error {
	if err := u.SetPassword(raw); err != nil {
		return err
	}
	if _, err := orm.Query[User](db).Filter("id", u.ID).Update(ctx, "password", u.Password); err != nil {
		return fmt.Errorf("auth: store password: %w", err)
	}
	return nil
}

// CheckPassword reports whether raw matches the stored password hash of u
// (Django's check_password). It never matches an unusable password.
func (u *User) CheckPassword(raw string) bool {
	return CheckPassword(raw, u.Password)
}

// SetUnusablePassword marks u as having no password: CheckPassword fails for
// every input (Django's set_unusable_password). It does not save the user.
func (u *User) SetUnusablePassword() error {
	encoded, err := MakeUnusablePassword()
	if err != nil {
		return err
	}
	u.Password = encoded
	return nil
}

// HasUsablePassword reports whether a password was set for u, as opposed to
// SetUnusablePassword (Django's has_usable_password).
func (u *User) HasUsablePassword() bool {
	return IsPasswordUsable(u.Password)
}
