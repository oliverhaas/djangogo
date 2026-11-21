package auth_test

import (
	"context"
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/auth"
	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
)

// Ported from Django: tests/auth_tests/test_models.py::NaturalKeysTestCase.test_user_natural_key
//
// There is no manager method get_by_natural_key: the natural key is the
// username, and the lookup is an ordinary query on it.
func TestUserNaturalKey(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		staff := createUser(t, db, "staff", "")
		got, err := orm.Query[auth.User](db).Get(context.Background(), "username", staff.NaturalKey())
		must(t, err)
		djangotest.AssertEqual(t, got.ID, staff.ID)
		djangotest.AssertEqual(t, staff.NaturalKey(), "staff")
	})
}

// Ported from Django: tests/auth_tests/test_models.py::NaturalKeysTestCase.test_group_natural_key
func TestGroupNaturalKey(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		users := createGroup(t, db, "users")
		got, err := orm.Query[auth.Group](db).Get(context.Background(), "name", users.NaturalKey())
		must(t, err)
		djangotest.AssertEqual(t, got.ID, users.ID)
	})
}

// Ported from Django: tests/auth_tests/test_models.py::UserManagerTestCase.test_create_user
func TestCreateUser(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		const emailLowercase = "normal@normal.com"
		user, err := auth.CreateUser(context.Background(), db, "user", emailLowercase, "", false, false, true)
		must(t, err)
		djangotest.AssertEqual(t, user.Email, emailLowercase)
		djangotest.AssertEqual(t, user.Username, "user")
		djangotest.AssertEqual(t, user.HasUsablePassword(), false)
	})
}

// Ported from Django: tests/auth_tests/test_models.py::UserManagerTestCase.test_create_user_email_domain_normalize_rfc3696
//
// According to RFC 3696 Section 3 the "@" symbol can be part of the local part
// of an email address.
func TestCreateUserEmailDomainNormalizeRFC3696(t *testing.T) {
	djangotest.AssertEqual(t, auth.NormalizeEmail(`Abc\@DEF@EXAMPLE.com`), `Abc\@DEF@example.com`)
}

// Ported from Django: tests/auth_tests/test_models.py::UserManagerTestCase.test_create_user_email_domain_normalize
func TestCreateUserEmailDomainNormalize(t *testing.T) {
	djangotest.AssertEqual(t, auth.NormalizeEmail("normal@DOMAIN.COM"), "normal@domain.com")
}

// Ported from Django: tests/auth_tests/test_models.py::UserManagerTestCase.test_create_user_email_domain_normalize_with_whitespace
func TestCreateUserEmailDomainNormalizeWithWhitespace(t *testing.T) {
	djangotest.AssertEqual(t, auth.NormalizeEmail(`email\ with_whitespace@D.COM`), `email\ with_whitespace@d.com`)
}

// Ported from Django: tests/auth_tests/test_models.py::UserManagerTestCase.test_empty_username
//
// Django raises ValueError("The given username must be set"); here the caller
// gets the sentinel error ErrEmptyUsername.
func TestCreateUserEmptyUsername(t *testing.T) {
	db := newTestDB(t)
	_, err := auth.CreateUser(context.Background(), db, "", "", "", false, false, true)
	djangotest.AssertErrorIs(t, err, auth.ErrEmptyUsername)
}

// Ported from Django: tests/auth_tests/test_models.py::UserManagerTestCase.test_create_user_is_staff
func TestCreateUserIsStaff(t *testing.T) {
	db := newTestDB(t)
	const email = "normal@normal.com"
	user, err := auth.CreateUser(context.Background(), db, "user", email, "", true, false, true)
	must(t, err)
	djangotest.AssertEqual(t, user.Email, email)
	djangotest.AssertEqual(t, user.Username, "user")
	djangotest.AssertEqual(t, user.IsStaff, true)
}

// Ported from Django: tests/auth_tests/test_models.py::AbstractBaseUserTests.test_has_usable_password
//
// Passwords are usable even if they don't correspond to a configured hasher.
func TestHasUsablePassword(t *testing.T) {
	user := &auth.User{Password: "some-gibbberish"}
	djangotest.AssertEqual(t, user.HasUsablePassword(), true)
}

// Ported from Django: tests/auth_tests/test_models.py::AbstractUserTestCase.test_last_login_default
//
// The users are also loaded again, to check that NULL is read back as nil.
func TestLastLoginDefault(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		user1 := &auth.User{Username: "user1"}
		must(t, orm.Query[auth.User](db).Create(ctx, user1))
		djangotest.AssertEqual(t, user1.LastLogin, (*time.Time)(nil))
		djangotest.AssertEqual(t, reload(t, db, user1.ID).LastLogin, (*time.Time)(nil))

		user2 := createUser(t, db, "user2", "")
		djangotest.AssertEqual(t, user2.LastLogin, (*time.Time)(nil))
		djangotest.AssertEqual(t, reload(t, db, user2.ID).LastLogin, (*time.Time)(nil))
	})
}

// Ported from Django: tests/auth_tests/test_models.py::IsActiveTestCase.test_builtin_user_isactive
//
// Django's is_active defaults to true. A Go struct has no field defaults, so
// the flag is passed to CreateUser; the port checks that it is saved.
func TestBuiltinUserIsActive(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		user := createUser(t, db, "foo", "")
		djangotest.AssertEqual(t, user.IsActive, true)
		user.IsActive = false
		saveFlags(t, db, user)
		djangotest.AssertEqual(t, reload(t, db, user.ID).IsActive, false)
	})
}

// Ported from Django: tests/auth_tests/test_models.py::AnonymousUserTests.test_properties
//
// The anonymous user is the zero User.
func TestAnonymousUserProperties(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	user := &auth.User{}
	djangotest.AssertEqual(t, user.ID, int64(0))
	djangotest.AssertEqual(t, user.Username, "")
	djangotest.AssertEqual(t, user.GetUsername(), "")
	djangotest.AssertEqual(t, user.IsAnonymous(), true)
	djangotest.AssertEqual(t, user.IsAuthenticated(), false)
	djangotest.AssertEqual(t, user.IsStaff, false)
	djangotest.AssertEqual(t, user.IsActive, false)
	djangotest.AssertEqual(t, user.IsSuperuser, false)
	userPerms, err := auth.GetUserPermissions(ctx, db, user)
	must(t, err)
	djangotest.AssertSequenceEqual(t, userPerms, []string{})
	groupPerms, err := auth.GetGroupPermissions(ctx, db, user)
	must(t, err)
	djangotest.AssertSequenceEqual(t, groupPerms, []string{})
}

// Ported from Django: tests/auth_tests/test_models.py::AnonymousUserTests.test_str
func TestAnonymousUserStr(t *testing.T) {
	djangotest.AssertEqual(t, (&auth.User{}).String(), "AnonymousUser")
}

// Ported from Django: tests/auth_tests/test_models.py::AnonymousUserTests.test_check_password
//
// Django raises NotImplementedError; the anonymous user here has no password
// hash, so no password matches.
func TestAnonymousUserCheckPassword(t *testing.T) {
	djangotest.AssertEqual(t, (&auth.User{}).CheckPassword("password"), false)
}

// Ported from Django: tests/auth_tests/test_models.py::GroupTests.test_str
func TestGroupStr(t *testing.T) {
	djangotest.AssertEqual(t, auth.Group{Name: "Users"}.String(), "Users")
}

// Ported from Django: tests/auth_tests/test_models.py::PermissionTests.test_user_perm_str
func TestPermissionUserPermStr(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		_, err := auth.SyncPermissions(ctx, db)
		must(t, err)
		p, err := orm.Query[auth.Permission](db).Get(ctx, "codename", "view_group")
		must(t, err)
		got, err := p.UserPermStr(ctx, db)
		must(t, err)
		djangotest.AssertEqual(t, got, "auth.view_group")
	})
}

// TestTableNames pins the table of every auth model and of its many-to-many
// relations to Django's name.
func TestTableNames(t *testing.T) {
	db := newTestDB(t)
	want := map[string]any{
		"auth_user":       &auth.User{},
		"auth_group":      &auth.Group{},
		"auth_permission": &auth.Permission{},
	}
	for table, instance := range want {
		m, ok := db.Registry().ModelOf(instance)
		if !ok {
			t.Fatalf("%T is not registered", instance)
		}
		djangotest.AssertEqual(t, m.Table(), table)
	}
	user, _ := db.Registry().ModelOf(&auth.User{})
	group, _ := db.Registry().ModelOf(&auth.Group{})
	through := map[string][]string{}
	for _, f := range append(user.ManyToMany(), group.ManyToMany()...) {
		through[f.Rel.Through.Table()] = f.Rel.Through.Columns()
	}
	djangotest.AssertEqual(t, len(through), 3)
	djangotest.AssertSequenceEqual(t, through["auth_user_groups"], []string{"id", "user_id", "group_id"})
	djangotest.AssertSequenceEqual(t, through["auth_user_user_permissions"], []string{"id", "user_id", "permission_id"})
	djangotest.AssertSequenceEqual(t, through["auth_group_permissions"], []string{"id", "group_id", "permission_id"})
	djangotest.AssertSequenceEqual(t, user.Columns(), []string{
		"id", "password", "last_login", "is_superuser", "username", "first_name",
		"last_name", "email", "is_staff", "is_active", "date_joined",
	})
	perm, _ := db.Registry().ModelOf(&auth.Permission{})
	djangotest.AssertSequenceEqual(t, perm.Columns(), []string{"id", "name", "content_type_id", "codename"})
}
