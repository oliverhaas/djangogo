package admin_test

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/admin"
	"github.com/oliverhaas/djangogo/auth"
	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/messages"
	"github.com/oliverhaas/djangogo/orm"
)

// changelistTestEnv sets up the data of Django's auth_tests ChangelistTests:
// the superuser testclient, logged in, and the user staff, both with the
// password "password". Django serves the admin of auth_tests.urls_admin under
// the namespace "auth_test_admin"; here it is the default site "admin".
func changelistTestEnv(t *testing.T, db *orm.DB) (*adminTestEnv, *auth.User) {
	t.Helper()
	withoutPasswordValidators(t)
	ctx := context.Background()
	testclient := createTestUser(t, db, "testclient", "password", true, true)
	testclient.Email = "testclient@example.com"
	// Django's setUp logs in, which sets last_login.
	now := time.Now().UTC().Truncate(time.Second)
	testclient.LastLogin = &now
	if err := orm.Query[auth.User](db).Save(ctx, testclient); err != nil {
		t.Fatalf("Update(testclient): %v", err)
	}
	staff := createTestUser(t, db, "staff", "password", false, false)
	staff.Email = "staffmember@example.com"
	if err := orm.Query[auth.User](db).Save(ctx, staff); err != nil {
		t.Fatalf("Update(staff): %v", err)
	}
	env := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
		admin.Register[auth.User](site, admin.UserAdmin(db))
	})
	if !env.client.Login("testclient", "password") {
		t.Fatalf("Login(testclient) failed")
	}
	return env, testclient
}

// withoutPasswordValidators turns the password validators off for the test,
// as in Django's test settings, which have none.
func withoutPasswordValidators(t *testing.T) {
	t.Helper()
	cfg := auth.CurrentConfig()
	cfg.PasswordValidators = []auth.PasswordValidator{}
	t.Cleanup(auth.Configure(cfg))
}

// userData returns the POST data of the change form of user, Django's
// ChangelistTests.get_user_data.
func userData(user *auth.User) url.Values {
	data := url.Values{
		"username":              {user.Username},
		"password":              {user.Password},
		"email":                 {user.Email},
		"first_name":            {user.FirstName},
		"last_name":             {user.LastName},
		"date_joined_0":         {user.DateJoined.Format(time.DateOnly)},
		"date_joined_1":         {user.DateJoined.Format(time.TimeOnly)},
		"initial-date_joined_0": {user.DateJoined.Format(time.DateOnly)},
		"initial-date_joined_1": {user.DateJoined.Format(time.TimeOnly)},
	}
	if user.LastLogin != nil {
		for _, prefix := range []string{"", "initial-"} {
			data.Set(prefix+"last_login_0", user.LastLogin.Format(time.DateOnly))
			data.Set(prefix+"last_login_1", user.LastLogin.Format(time.TimeOnly))
		}
	}
	for name, on := range map[string]bool{"is_active": user.IsActive, "is_staff": user.IsStaff, "is_superuser": user.IsSuperuser} {
		if on {
			data.Set(name, "on")
		}
	}
	return data
}

// getUser reloads the user with the primary key id.
func getUser(t *testing.T, db *orm.DB, id int64) auth.User {
	t.Helper()
	u, err := orm.Query[auth.User](db).Get(context.Background(), "id", id)
	if err != nil {
		t.Fatalf("Get user %d: %v", id, err)
	}
	return u
}

// Django logs the rejected lookup to django.security.DisallowedModelAdminLookup;
// the Go admin answers it with the same 400 and logs nothing.
//
// Ported from Django: tests/auth_tests/test_views.py::ChangelistTests.test_changelist_disallows_password_lookups
func TestChangelistDisallowsPasswordLookups(t *testing.T) {
	djangotest.EachDB(t, adminViewsModels(), func(t *testing.T, db *orm.DB) {
		env, _ := changelistTestEnv(t, db)
		// A lookup that tries to filter on password isn't OK.
		resp := env.client.Get(mustReverse(t, env.router, "admin:auth_user_changelist") + "?password__startswith=sha1$")
		djangotest.AssertStatus(t, resp, http.StatusBadRequest)
	})
}

// Ported from Django: tests/auth_tests/test_views.py::ChangelistTests.test_user_change_email
// Ported from Django: tests/auth_tests/test_views.py::ChangelistTests.test_user_not_change
func TestUserChangeEmail(t *testing.T) {
	djangotest.EachDB(t, adminViewsModels(), func(t *testing.T, db *orm.DB) {
		env, testclient := changelistTestEnv(t, db)
		changeURL := mustReverse(t, env.router, "admin:auth_user_change", testclient.ID)
		changelistURL := mustReverse(t, env.router, "admin:auth_user_changelist")

		data := userData(testclient)
		data.Set("email", "new_"+data.Get("email"))
		resp := env.client.Post(changeURL, data)
		djangotest.AssertRedirects(t, resp, changelistURL)
		got := getUser(t, db, testclient.ID)
		djangotest.AssertEqual(t, got.Email, "new_testclient@example.com")
		// The password is a disabled field of the form, so it keeps its value.
		djangotest.AssertEqual(t, got.Password, testclient.Password)
		row := newestLogEntry(t, db)
		djangotest.AssertEqual(t, row.GetChangeMessage(), "Changed Email address.")

		resp = env.client.Post(changeURL, userData(&got))
		djangotest.AssertRedirects(t, resp, changelistURL)
		row = newestLogEntry(t, db)
		djangotest.AssertEqual(t, row.GetChangeMessage(), "No fields changed.")
	})
}

// Ported from Django: tests/auth_tests/test_views.py::ChangelistTests.test_user_with_usable_password_change_password
func TestUserWithUsablePasswordChangePassword(t *testing.T) {
	djangotest.EachDB(t, adminViewsModels(), func(t *testing.T, db *orm.DB) {
		env, testclient := changelistTestEnv(t, db)
		userChangeURL := mustReverse(t, env.router, "admin:auth_user_change", testclient.ID)
		passwordChangeURL := mustReverse(t, env.router, "admin:auth_user_password_change", testclient.ID)

		resp := env.client.Get(userChangeURL)
		// Test the link inside password field help_text.
		assertPasswordLink(t, resp, "Reset password", userChangeURL, passwordChangeURL)

		resp = env.client.Get(passwordChangeURL)
		// Test the form title with original (usable) password.
		djangotest.AssertContains(t, resp, "<h1>Change password: testclient</h1>")
		// Breadcrumb.
		djangotest.AssertContains(t, resp, "testclient</a></li>\n<li aria-current=\"page\">Change password</li>")
		// Usable password field.
		djangotest.AssertContains(t, resp,
			`<fieldset class="flex-container"><legend>Password-based authentication:</legend>`)
		// Submit buttons.
		djangotest.AssertContains(t, resp, `<input type="submit" name="set-password"`)
		djangotest.AssertContains(t, resp, `<input type="submit" name="unset-password"`)

		// Password change.
		resp = env.client.Post(passwordChangeURL, url.Values{"password1": {"password1"}, "password2": {"password1"}})
		djangotest.AssertRedirects(t, resp, userChangeURL)
		djangotest.AssertMessages(t, resp, []messages.Message{{Level: messages.LevelSuccess, Message: "Password changed successfully."}})
		row := newestLogEntry(t, db)
		djangotest.AssertEqual(t, row.GetChangeMessage(), "Changed password.")
		env.client.Logout()
		if !env.client.Login("testclient", "password1") {
			t.Fatalf("Login with the new password failed")
		}

		// Disable password-based authentication without proper submit button.
		resp = env.client.Post(passwordChangeURL, url.Values{
			"password1":       {"password1"},
			"password2":       {"password1"},
			"usable_password": {"false"},
		})
		djangotest.AssertRedirects(t, resp, passwordChangeURL)
		djangotest.AssertMessages(t, resp, []messages.Message{
			{Level: messages.LevelError, Message: "Conflicting form data submitted. Please try again."},
		})
		// No password change yet.
		if !env.client.Login("testclient", "password1") {
			t.Fatalf("Login after the conflicting form failed")
		}

		// Disable password-based authentication with proper submit button.
		resp = env.client.Post(passwordChangeURL, url.Values{
			"password1":       {"password1"},
			"password2":       {"password1"},
			"usable_password": {"false"},
			"unset-password":  {"1"},
		})
		djangotest.AssertRedirects(t, resp, userChangeURL)
		djangotest.AssertMessages(t, resp, []messages.Message{
			{Level: messages.LevelSuccess, Message: "Password-based authentication was disabled."},
		})
		row = newestLogEntry(t, db)
		djangotest.AssertEqual(t, row.GetChangeMessage(), "Changed password.")
		env.client.Logout()
		// Password-based authentication was disabled.
		if env.client.Login("testclient", "password1") {
			t.Errorf("Login succeeded after password-based authentication was disabled")
		}
		got := getUser(t, db, testclient.ID)
		djangotest.AssertEqual(t, got.HasUsablePassword(), false)
	})
}

// Ported from Django: tests/auth_tests/test_views.py::ChangelistTests.test_user_with_unusable_password_change_password
func TestUserWithUnusablePasswordChangePassword(t *testing.T) {
	djangotest.EachDB(t, adminViewsModels(), func(t *testing.T, db *orm.DB) {
		env, _ := changelistTestEnv(t, db)
		// Test for title with unusable password with a test user.
		testUser, err := orm.Query[auth.User](db).Get(context.Background(), "email", "staffmember@example.com")
		if err != nil {
			t.Fatalf("Get staff: %v", err)
		}
		if err := testUser.SetUnusablePassword(); err != nil {
			t.Fatalf("SetUnusablePassword: %v", err)
		}
		if err := orm.Query[auth.User](db).Save(context.Background(), &testUser); err != nil {
			t.Fatalf("Update(staff): %v", err)
		}
		userChangeURL := mustReverse(t, env.router, "admin:auth_user_change", testUser.ID)
		passwordChangeURL := mustReverse(t, env.router, "admin:auth_user_password_change", testUser.ID)

		resp := env.client.Get(userChangeURL)
		// Test the link inside password field help_text.
		assertPasswordLink(t, resp, "Set password", userChangeURL, passwordChangeURL)

		resp = env.client.Get(passwordChangeURL)
		// Test the form title with original (usable) password.
		djangotest.AssertContains(t, resp, "<h1>Set password: staff</h1>")
		// Breadcrumb.
		djangotest.AssertContains(t, resp, "staff</a></li>\n<li aria-current=\"page\">Set password</li>")
		// Submit buttons.
		djangotest.AssertContains(t, resp, `<input type="submit" name="set-password"`)
		djangotest.AssertNotContains(t, resp, `<input type="submit" name="unset-password"`)

		resp = env.client.Post(passwordChangeURL, url.Values{"password1": {"password1"}, "password2": {"password1"}})
		djangotest.AssertRedirects(t, resp, userChangeURL)
		djangotest.AssertMessages(t, resp, []messages.Message{{Level: messages.LevelSuccess, Message: "Password changed successfully."}})
		row := newestLogEntry(t, db)
		djangotest.AssertEqual(t, row.GetChangeMessage(), "Changed password.")
	})
}

// assertPasswordLink asserts that the change page resp links to the password
// page with a button named label, by a link relative to changeURL.
func assertPasswordLink(t *testing.T, resp *djangotest.Response, label, changeURL, passwordChangeURL string) {
	t.Helper()
	m := regexp.MustCompile(`<a role="button" class="button" href="([^"]*)">` + label + `</a>`).FindStringSubmatch(string(resp.Body))
	if m == nil {
		t.Fatalf("no %q link in the change page:\n%s", label, string(resp.Body))
	}
	base, _ := url.Parse(changeURL)
	rel, err := url.Parse(m[1])
	if err != nil {
		t.Fatalf("parse %q: %v", m[1], err)
	}
	djangotest.AssertEqual(t, base.ResolveReference(rel).String(), passwordChangeURL)
}

// Ported from Django: tests/auth_tests/test_views.py::ChangelistTests.test_user_change_different_user_password
func TestUserChangeDifferentUserPassword(t *testing.T) {
	djangotest.EachDB(t, adminViewsModels(), func(t *testing.T, db *orm.DB) {
		env, admin := changelistTestEnv(t, db)
		u, err := orm.Query[auth.User](db).Get(context.Background(), "email", "staffmember@example.com")
		if err != nil {
			t.Fatalf("Get staff: %v", err)
		}
		resp := env.client.Post(mustReverse(t, env.router, "admin:auth_user_password_change", u.ID),
			url.Values{"password1": {"password1"}, "password2": {"password1"}})
		djangotest.AssertRedirects(t, resp, mustReverse(t, env.router, "admin:auth_user_change", u.ID))
		row := newestLogEntry(t, db)
		djangotest.AssertEqual(t, row.User.PK(), admin.ID)
		djangotest.AssertEqual(t, *row.ObjectID, fmt.Sprint(u.ID))
		djangotest.AssertEqual(t, row.GetChangeMessage(), "Changed password.")
	})
}

// Ported from Django: tests/auth_tests/test_views.py::ChangelistTests.test_password_change_bad_url
func TestPasswordChangeBadURL(t *testing.T) {
	djangotest.EachDB(t, adminViewsModels(), func(t *testing.T, db *orm.DB) {
		env, _ := changelistTestEnv(t, db)
		resp := env.client.Get(mustReverse(t, env.router, "admin:auth_user_password_change", "foobar"))
		djangotest.AssertStatus(t, resp, http.StatusNotFound)
	})
}

// No Go counterpart: test_user_change_password_passes_user_to_has_change_permission
// mocks the has_change_permission method of UserAdmin. The Go admin passes
// the user to HasChangePermission, which TestUserChangePasswordChecksTheUser
// observes.
//
// Ported from Django: tests/auth_tests/test_views.py::ChangelistTests.test_view_user_password_is_readonly
func TestViewUserPasswordIsReadonly(t *testing.T) {
	djangotest.EachDB(t, adminViewsModels(), func(t *testing.T, db *orm.DB) {
		env, u := changelistTestEnv(t, db)
		u.IsSuperuser = false
		if err := orm.Query[auth.User](db).Save(context.Background(), u); err != nil {
			t.Fatalf("Update(testclient): %v", err)
		}
		originalPassword := u.Password
		grantPerm(t, db, u, "auth", "view_user")
		changeURL := mustReverse(t, env.router, "admin:auth_user_change", u.ID)
		resp := env.client.Get(changeURL)
		parts := strings.Split(u.Password, "$")
		if len(parts) != 3 {
			t.Fatalf("password %q is not algorithm$salt$hash", u.Password)
		}
		algo, salt, hash := parts[0], parts[1], parts[2]
		djangotest.AssertContains(t, resp, `<div class="readonly">testclient</div>`)
		// The password value is hashed.
		djangotest.AssertContains(t, resp, fmt.Sprintf(
			"<strong>algorithm</strong>: <bdi>%s</bdi>\n\n"+
				"<strong>salt</strong>: <bdi>%s********************</bdi>\n\n"+
				"<strong>hash</strong>: <bdi>%s**************************</bdi>\n\n",
			algo, salt[:2], hash[:6]), djangotest.HTML())
		djangotest.AssertNotContains(t, resp, `<a role="button" class="button" href="../password/">Reset password</a>`)
		// Value in POST data is ignored.
		data := userData(u)
		data.Set("password", "shouldnotchange")
		resp = env.client.Post(changeURL, data)
		djangotest.AssertStatus(t, resp, http.StatusForbidden)
		djangotest.AssertEqual(t, getUser(t, db, u.ID).Password, originalPassword)
	})
}

// No Django test exists: Django's test of the permission check of the
// password page mocks UserAdmin.has_change_permission. A HasChangePermission
// that allows only the change of other users lets the page of testclient
// answer 403 and the page of staff work.
func TestUserChangePasswordChecksTheUser(t *testing.T) {
	djangotest.EachDB(t, adminViewsModels(), func(t *testing.T, db *orm.DB) {
		withoutPasswordValidators(t)
		testclient := createTestUser(t, db, "testclient", "password", true, true)
		staff := createTestUser(t, db, "staff", "password", false, false)
		env := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
			ma := admin.UserAdmin(db)
			ma.HasChangePermission = func(_ *http.Request, obj any) bool {
				u, ok := obj.(*auth.User)
				return !ok || u.ID != testclient.ID
			}
			admin.Register[auth.User](site, ma)
		})
		env.client.ForceLogin(testclient)
		data := url.Values{"password1": {"password1"}, "password2": {"password1"}}
		resp := env.client.Post(mustReverse(t, env.router, "admin:auth_user_password_change", testclient.ID), data)
		djangotest.AssertStatus(t, resp, http.StatusForbidden)
		resp = env.client.Post(mustReverse(t, env.router, "admin:auth_user_password_change", staff.ID), data)
		djangotest.AssertRedirects(t, resp, mustReverse(t, env.router, "admin:auth_user_change", staff.ID))
	})
}

// userAdminTestEnv sets up the data of Django's admin_views UserAdminTest that
// the ported tests use: the superuser super, logged in, and the staff users
// adduser and changeuser.
func userAdminTestEnv(t *testing.T, db *orm.DB) *adminTestEnv {
	t.Helper()
	withoutPasswordValidators(t)
	super := createTestUser(t, db, "super", "secret", true, true)
	createTestUser(t, db, "adduser", "secret", true, false)
	createTestUser(t, db, "changeuser", "secret", true, false)
	env := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
		registerBasicAdminModels(db)(site)
		admin.Register[Album](site, admin.ModelAdmin{})
	})
	env.client.ForceLogin(super)
	return env
}

// countUsers returns the number of users.
func countUsers(t *testing.T, db *orm.DB) int64 {
	t.Helper()
	n, err := orm.Query[auth.User](db).Count(context.Background())
	if err != nil {
		t.Fatalf("Count users: %v", err)
	}
	return n
}

// Ported from Django: tests/admin_views/tests.py::UserAdminTest.test_save_button
func TestUserAdminSaveButton(t *testing.T) {
	djangotest.EachDB(t, adminViewsModels(), func(t *testing.T, db *orm.DB) {
		env := userAdminTestEnv(t, db)
		userCount := countUsers(t, db)
		resp := env.client.Post(mustReverse(t, env.router, "admin:auth_user_add"), url.Values{
			"username":  {"newuser"},
			"password1": {"newpassword"},
			"password2": {"newpassword"},
		})
		newUser, err := orm.Query[auth.User](db).Get(context.Background(), "username", "newuser")
		if err != nil {
			t.Fatalf("Get newuser: %v", err)
		}
		djangotest.AssertRedirects(t, resp, mustReverse(t, env.router, "admin:auth_user_change", newUser.ID))
		djangotest.AssertEqual(t, countUsers(t, db), userCount+1)
		djangotest.AssertEqual(t, newUser.HasUsablePassword(), true)
	})
}

// Ported from Django: tests/admin_views/tests.py::UserAdminTest.test_save_continue_editing_button
func TestUserAdminSaveContinueEditingButton(t *testing.T) {
	djangotest.EachDB(t, adminViewsModels(), func(t *testing.T, db *orm.DB) {
		env := userAdminTestEnv(t, db)
		userCount := countUsers(t, db)
		resp := env.client.Post(mustReverse(t, env.router, "admin:auth_user_add"), url.Values{
			"username":  {"newuser"},
			"password1": {"newpassword"},
			"password2": {"newpassword"},
			"_continue": {"1"},
		})
		newUser, err := orm.Query[auth.User](db).Get(context.Background(), "username", "newuser")
		if err != nil {
			t.Fatalf("Get newuser: %v", err)
		}
		newUserURL := mustReverse(t, env.router, "admin:auth_user_change", newUser.ID)
		djangotest.AssertRedirects(t, resp, newUserURL, djangotest.FetchRedirectResponse(false))
		djangotest.AssertEqual(t, countUsers(t, db), userCount+1)
		djangotest.AssertEqual(t, newUser.HasUsablePassword(), true)
		resp = env.client.Get(newUserURL)
		djangotest.AssertContains(t, resp, fmt.Sprintf(
			`<li class="success">The user “<a href="%s">%s</a>” was added successfully. You may edit it again below.</li>`,
			newUserURL, newUser.Username), djangotest.HTML())
	})
}

// The Go admin keeps the form in the context key "form"; Django's
// assertFormError reads it through "adminform".
//
// Ported from Django: tests/admin_views/tests.py::UserAdminTest.test_password_mismatch
func TestUserAdminPasswordMismatch(t *testing.T) {
	djangotest.EachDB(t, adminViewsModels(), func(t *testing.T, db *orm.DB) {
		env := userAdminTestEnv(t, db)
		resp := env.client.Post(mustReverse(t, env.router, "admin:auth_user_add"), url.Values{
			"username":  {"newuser"},
			"password1": {"newpassword"},
			"password2": {"mismatch"},
		})
		djangotest.AssertStatus(t, resp, http.StatusOK)
		form := ctxVal(t, resp, "form")
		djangotest.AssertFormError(t, form, "password1", nil)
		djangotest.AssertFormError(t, form, "password2", []string{"The two password fields didn’t match."})
	})
}

// Ported from Django: tests/admin_views/tests.py::UserAdminTest.test_user_fk_add_popup
func TestUserFKAddPopup(t *testing.T) {
	djangotest.EachDB(t, adminViewsModels(), func(t *testing.T, db *orm.DB) {
		env := userAdminTestEnv(t, db)
		// User addition through a FK popup should return the appropriate
		// JavaScript response.
		addURL := mustReverse(t, env.router, "admin:auth_user_add")
		resp := env.client.Get(mustReverse(t, env.router, "admin:admin_views_album_add"))
		djangotest.AssertContains(t, resp, addURL)
		djangotest.AssertContains(t, resp, `class="related-widget-wrapper-link add-related" id="add_id_owner"`)
		resp = env.client.Get(addURL + "?_popup=1")
		djangotest.AssertNotContains(t, resp, `name="_continue"`)
		djangotest.AssertNotContains(t, resp, `name="_addanother"`)
		data := url.Values{
			"username":  {"newuser"},
			"password1": {"newpassword"},
			"password2": {"newpassword"},
			"_popup":    {"1"},
			"_save":     {"1"},
		}
		resp = env.client.Post(addURL+"?_popup=1", data, djangotest.Follow())
		djangotest.AssertContains(t, resp, "&quot;obj&quot;: &quot;newuser&quot;")
	})
}

// Ported from Django: tests/admin_views/tests.py::UserAdminTest.test_user_fk_change_popup
func TestUserFKChangePopup(t *testing.T) {
	djangotest.EachDB(t, adminViewsModels(), func(t *testing.T, db *orm.DB) {
		env := userAdminTestEnv(t, db)
		// User change through a FK popup should return the appropriate
		// JavaScript response.
		resp := env.client.Get(mustReverse(t, env.router, "admin:admin_views_album_add"))
		djangotest.AssertContains(t, resp, mustReverse(t, env.router, "admin:auth_user_change", "__fk__"))
		djangotest.AssertContains(t, resp, `class="related-widget-wrapper-link change-related" id="change_id_owner"`)
		user, err := orm.Query[auth.User](db).Get(context.Background(), "username", "changeuser")
		if err != nil {
			t.Fatalf("Get changeuser: %v", err)
		}
		u := mustReverse(t, env.router, "admin:auth_user_change", user.ID) + "?_popup=1"
		resp = env.client.Get(u)
		djangotest.AssertNotContains(t, resp, `name="_continue"`)
		djangotest.AssertNotContains(t, resp, `name="_addanother"`)
		data := url.Values{
			"username":      {"newuser"},
			"password1":     {"newpassword"},
			"password2":     {"newpassword"},
			"last_login_0":  {"2007-05-30"},
			"last_login_1":  {"13:20:10"},
			"date_joined_0": {"2007-05-30"},
			"date_joined_1": {"13:20:10"},
			"_popup":        {"1"},
			"_save":         {"1"},
		}
		resp = env.client.Post(u, data, djangotest.Follow())
		djangotest.AssertContains(t, resp, "&quot;obj&quot;: &quot;newuser&quot;")
		djangotest.AssertContains(t, resp, "&quot;action&quot;: &quot;change&quot;")
	})
}

// FIXME-PORT: test_user_permission_performance counts the queries of the
// change page, which differ in the Go admin.
//
// No Go counterpart: test_form_url_present_in_context overrides the method
// user_change_password of a UserAdmin subclass to pass form_url; the Go admin
// has no such method to override.
//
// Ported from Django: tests/admin_views/tests.py::UserAdminTest.test_save_add_another_button
func TestUserAdminSaveAddAnotherButton(t *testing.T) {
	djangotest.EachDB(t, adminViewsModels(), func(t *testing.T, db *orm.DB) {
		env := userAdminTestEnv(t, db)
		userCount := countUsers(t, db)
		addURL := mustReverse(t, env.router, "admin:auth_user_add")
		resp := env.client.Post(addURL, url.Values{
			"username":    {"newuser"},
			"password1":   {"newpassword"},
			"password2":   {"newpassword"},
			"_addanother": {"1"},
		})
		newUser, err := orm.Query[auth.User](db).OrderBy("-id").First(context.Background())
		if err != nil {
			t.Fatalf("newest user: %v", err)
		}
		djangotest.AssertRedirects(t, resp, addURL)
		djangotest.AssertEqual(t, countUsers(t, db), userCount+1)
		djangotest.AssertEqual(t, newUser.HasUsablePassword(), true)
	})
}

// No Django test exists: Django's UserAdmin._add_view answers a user who can
// add users but cannot change them with a 404 in DEBUG mode and with
// PermissionDenied otherwise, as such a user could create a superuser.
func TestUserAdminAddNeedsChangePermission(t *testing.T) {
	djangotest.EachDB(t, adminViewsModels(), func(t *testing.T, db *orm.DB) {
		adder := createTestUser(t, db, "adduser", "secret", true, false)
		for _, debug := range []bool{false, true} {
			env := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
				site.Debug = debug
				admin.Register[auth.User](site, admin.UserAdmin(db))
			})
			grantPerm(t, db, adder, "auth", "add_user")
			env.client.ForceLogin(adder)
			addURL := mustReverse(t, env.router, "admin:auth_user_add")
			resp := env.client.Get(addURL)
			if debug {
				djangotest.AssertStatus(t, resp, http.StatusNotFound)
				djangotest.AssertContains(t, resp, `Your user does not have the "Change user" permission.`, djangotest.StatusCode(http.StatusNotFound))
			} else {
				djangotest.AssertStatus(t, resp, http.StatusForbidden)
			}
			grantPerm(t, db, adder, "auth", "change_user")
			resp = env.client.Get(addURL)
			djangotest.AssertStatus(t, resp, http.StatusOK)
			revokePerm(t, db, adder, "auth", "change_user")
			revokePerm(t, db, adder, "auth", "add_user")
		}
	})
}

// No Django test exists: the change form of a group labels each permission
// as Django's Permission.__str__ does, "app | model | name".
func TestGroupAdminPermissionLabels(t *testing.T) {
	djangotest.EachDB(t, adminViewsModels(), func(t *testing.T, db *orm.DB) {
		super := createTestUser(t, db, "super", "secret", true, true)
		env := newAdminTestEnv(t, db, registerBasicAdminModels(db))
		env.client.ForceLogin(super)
		resp := env.client.Get(mustReverse(t, env.router, "admin:auth_group_add"))
		djangotest.AssertContains(t, resp, "auth | group | Can view group")
		djangotest.AssertContains(t, resp, `class="selectfilter"`)
	})
}

// FIXME-PORT: test_group_permission_performance counts the queries of the
// change page, which differ in the Go admin.
//
// Ported from Django: tests/admin_views/tests.py::GroupAdminTest.test_save_button
func TestGroupAdminSaveButton(t *testing.T) {
	djangotest.EachDB(t, adminViewsModels(), func(t *testing.T, db *orm.DB) {
		super := createTestUser(t, db, "super", "secret", true, true)
		env := newAdminTestEnv(t, db, registerBasicAdminModels(db))
		env.client.ForceLogin(super)
		count, err := orm.Query[auth.Group](db).Count(context.Background())
		if err != nil {
			t.Fatalf("Count groups: %v", err)
		}
		resp := env.client.Post(mustReverse(t, env.router, "admin:auth_group_add"), url.Values{"name": {"newgroup"}})
		djangotest.AssertRedirects(t, resp, mustReverse(t, env.router, "admin:auth_group_changelist"))
		after, err := orm.Query[auth.Group](db).Count(context.Background())
		if err != nil {
			t.Fatalf("Count groups: %v", err)
		}
		djangotest.AssertEqual(t, after, count+1)
	})
}
