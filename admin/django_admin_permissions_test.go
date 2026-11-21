package admin_test

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"testing"

	"github.com/oliverhaas/djangogo/admin"
	"github.com/oliverhaas/djangogo/auth"
	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/messages"
	"github.com/oliverhaas/djangogo/orm"
)

// adminLoginErrorMessage is Django's AdminAuthenticationForm invalid_login text.
const adminLoginErrorMessage = "Please enter the correct username and password for a staff account. Note that both fields may be case-sensitive."

// RowLevelChangePermissionModel stands in for
// tests/admin_views/models.py::RowLevelChangePermissionModel.
type RowLevelChangePermissionModel struct {
	ID        int64
	Name      string `orm:"max_length=100"`
	CanChange bool
	CanView   bool
}

// String returns the row name.
func (r RowLevelChangePermissionModel) String() string { return r.Name }

// permUsers holds the standard Django AdminViewPermissionsTest user roster.
type permUsers struct {
	super      *auth.User
	viewUser   *auth.User
	addUser    *auth.User
	changeUser *auth.User
	deleteUser *auth.User
	joePublic  *auth.User
	noStaff    *auth.User
	a1         *Article
	s1         *Section
}

// setupPermFixture creates the AdminViewPermissionsTest users, permissions, and
// sample rows in db.
func setupPermFixture(t *testing.T, db *orm.DB, env *adminTestEnv) *permUsers {
	t.Helper()
	ctx := context.Background()
	u := &permUsers{
		super:      createTestUser(t, db, "super", "secret", true, true),
		viewUser:   createTestUser(t, db, "viewuser", "secret", true, false),
		addUser:    createTestUser(t, db, "adduser", "secret", true, false),
		changeUser: createTestUser(t, db, "changeuser", "secret", true, false),
		deleteUser: createTestUser(t, db, "deleteuser", "secret", true, false),
		joePublic:  createTestUser(t, db, "joepublic", "secret", false, false),
		noStaff:    createTestUser(t, db, "nostaff", "secret", false, false),
	}
	u.super.Email = "super@example.com"
	if err := orm.Query[auth.User](db).Save(ctx, u.super); err != nil {
		t.Fatalf("save super email: %v", err)
	}

	grantPerm(t, db, u.viewUser, "admin_views", "view_article")
	grantPerm(t, db, u.addUser, "admin_views", "add_article")
	grantPerm(t, db, u.changeUser, "admin_views", "change_article")
	grantPerm(t, db, u.noStaff, "admin_views", "change_article")
	grantPerm(t, db, u.deleteUser, "admin_views", "delete_article")
	grantPerm(t, db, u.deleteUser, "admin_views", "delete_section")

	s1 := &Section{Name: "Test section"}
	if err := orm.Query[Section](db).Create(ctx, s1); err != nil {
		t.Fatalf("Create(Section): %v", err)
	}
	u.s1 = s1

	a1 := &Article{Title: "", Views: 10, Published: true}
	a2 := &Article{Title: "Oldest", Views: 20, Published: false}
	a3 := &Article{Title: "Newest", Views: 30, Published: true}
	for _, a := range []*Article{a1, a2, a3} {
		if err := orm.Query[Article](db).Create(ctx, a); err != nil {
			t.Fatalf("Create(Article): %v", err)
		}
	}
	u.a1 = a1
	_ = env
	return u
}

// registerPermModels registers the models used by AdminViewPermissionsTest,
// including RowLevelChangePermissionModel with per-object permission hooks.
func registerPermModels(site *admin.AdminSite) {
	// The fieldsets of ArticleAdmin in tests/admin_views/admin.py over the
	// fields of the Go Article.
	admin.Register[Article](site, admin.ModelAdmin{Fieldsets: []admin.Fieldset{
		{Name: "Some fields", Classes: []string{"collapse"}, Fields: []any{"title"}},
		{Name: "Some other fields", Classes: []string{"wide"}, Fields: []any{"views"}},
		{Name: "이름", Fields: []any{"published"}},
	}})
	admin.Register[Section](site, admin.ModelAdmin{})
	admin.Register[RowLevelChangePermissionModel](site, admin.ModelAdmin{
		ReadonlyFields: []string{"CanChange", "CanView"},
		HasChangePermission: func(r *http.Request, obj any) bool {
			u, ok := auth.CurrentUser(r.Context())
			if !ok || !u.IsStaff {
				return false
			}
			row, ok := obj.(*RowLevelChangePermissionModel)
			return ok && row != nil && row.CanChange
		},
		HasViewPermission: func(r *http.Request, obj any) bool {
			u, ok := auth.CurrentUser(r.Context())
			if !ok || !u.IsStaff {
				return false
			}
			row, ok := obj.(*RowLevelChangePermissionModel)
			return ok && row != nil && row.CanView
		},
	})
}

// Ported from Django: tests/admin_views/tests.py::AdminViewPermissionsTest.test_login
func TestAdminLogin(t *testing.T) {
	djangotest.EachDB(t, adminViewsModels(), func(t *testing.T, db *orm.DB) {
		env := newAdminTestEnv(t, db, registerPermModels)
		setupPermFixture(t, db, env)

		indexURL := mustReverse(t, env.router, "admin:index")
		loginURL := mustReverse(t, env.router, "admin:login") + "?next=" + indexURL
		logoutURL := mustReverse(t, env.router, "admin:logout")

		// Every staff user (superuser, view, add, change, delete) can log in.
		for _, username := range []string{"super", "viewuser", "adduser", "changeuser", "deleteuser"} {
			resp := env.client.Get(indexURL)
			djangotest.AssertRedirects(t, resp, loginURL)
			login := env.client.Post(loginURL, url.Values{
				"next":     {indexURL},
				"username": {username},
				"password": {"secret"},
			})
			djangotest.AssertRedirects(t, login, indexURL)
			env.client.Post(logoutURL, nil)
		}

		// Email address is rejected with the staff login error message.
		resp := env.client.Get(indexURL)
		djangotest.AssertStatus(t, resp, http.StatusFound)
		login := env.client.Post(loginURL, url.Values{
			"next":     {indexURL},
			"username": {"super@example.com"},
			"password": {"secret"},
		})
		djangotest.AssertContains(t, login, adminLoginErrorMessage)

		// Regular non-staff user cannot log in.
		resp = env.client.Get(indexURL)
		djangotest.AssertStatus(t, resp, http.StatusFound)
		login = env.client.Post(loginURL, url.Values{
			"next":     {indexURL},
			"username": {"joepublic"},
			"password": {"secret"},
		})
		djangotest.AssertContains(t, login, adminLoginErrorMessage)

		// Missing username re-renders 200 with a required field error.
		resp = env.client.Get(indexURL)
		djangotest.AssertStatus(t, resp, http.StatusFound)
		login = env.client.Post(loginURL, url.Values{
			"next":     {indexURL},
			"password": {"secret"},
		})
		djangotest.AssertStatus(t, login, http.StatusOK)
		djangotest.AssertFormError(t, ctxVal(t, login, "form"), "username", []string{"This field is required."})
	})
}

// Ported from Django: tests/admin_views/tests.py::AdminViewPermissionsTest.test_login_redirect_for_direct_get
func TestLoginRedirectForDirectGet(t *testing.T) {
	djangotest.EachDB(t, adminViewsModels(), func(t *testing.T, db *orm.DB) {
		env := newAdminTestEnv(t, db, registerPermModels)
		resp := env.client.Get(mustReverse(t, env.router, "admin:login"))
		djangotest.AssertStatus(t, resp, http.StatusOK)
		djangotest.AssertEqual(t, ctxVal(t, resp, "next"), any(mustReverse(t, env.router, "admin:index")))
	})
}

// Ported from Django: tests/admin_views/tests.py::AdminViewPermissionsTest.test_login_redirect_when_logged_in
func TestLoginRedirectWhenLoggedIn(t *testing.T) {
	djangotest.EachDB(t, adminViewsModels(), func(t *testing.T, db *orm.DB) {
		env := newAdminTestEnv(t, db, registerPermModels)
		u := setupPermFixture(t, db, env)
		env.client.ForceLogin(u.super)

		resp := env.client.Get(mustReverse(t, env.router, "admin:login"))
		djangotest.AssertRedirects(t, resp, mustReverse(t, env.router, "admin:index"))
	})
}

// Ported from Django: tests/admin_views/tests.py::AdminViewPermissionsTest.test_login_redirect_to_next_url_when_logged_in
func TestLoginRedirectToNextURLWhenLoggedIn(t *testing.T) {
	djangotest.EachDB(t, adminViewsModels(), func(t *testing.T, db *orm.DB) {
		env := newAdminTestEnv(t, db, registerPermModels)
		u := setupPermFixture(t, db, env)
		env.client.ForceLogin(u.super)

		nextURL := mustReverse(t, env.router, "admin:admin_views_article_add")
		resp := env.client.Get(mustReverse(t, env.router, "admin:login"), djangotest.Query(url.Values{"next": {nextURL}}))
		djangotest.AssertRedirects(t, resp, nextURL)
	})
}

// Ported from Django: tests/admin_views/tests.py::AdminViewPermissionsTest.test_login_redirect_unsafe_next_url_when_logged_in
func TestLoginRedirectUnsafeNextURLWhenLoggedIn(t *testing.T) {
	djangotest.EachDB(t, adminViewsModels(), func(t *testing.T, db *orm.DB) {
		env := newAdminTestEnv(t, db, registerPermModels)
		u := setupPermFixture(t, db, env)
		env.client.ForceLogin(u.super)

		resp := env.client.Get(mustReverse(t, env.router, "admin:login"), djangotest.Query(url.Values{"next": {"https://example.com/bad"}}))
		djangotest.AssertRedirects(t, resp, mustReverse(t, env.router, "admin:index"), djangotest.FetchRedirectResponse(false))
	})
}

// Ported from Django: tests/admin_views/tests.py::AdminViewPermissionsTest.test_login_has_permission
func TestLoginHasPermission(t *testing.T) {
	djangotest.EachDB(t, adminViewsModels(), func(t *testing.T, db *orm.DB) {
		permName := "admin_views.change_article"
		env := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
			site.Name = "has_permission_admin"
			site.Prefix = "/has_permission_admin"
			site.HasPermissionFunc = func(r *http.Request) bool {
				u, ok := auth.CurrentUser(r.Context())
				if !ok || !u.IsActive {
					return false
				}
				if u.IsStaff {
					return true
				}
				has, _ := auth.HasPerm(r.Context(), db, u, permName)
				return has
			}
			site.LoginConfirmAllowed = func(r *http.Request, u *auth.User) error {
				if !u.IsActive {
					return errors.New("permission denied")
				}
				if u.IsStaff {
					return nil
				}
				has, _ := auth.HasPerm(r.Context(), db, u, permName)
				if !has {
					return errors.New("permission denied")
				}
				return nil
			}
			admin.Register[Article](site, admin.ModelAdmin{})
		})
		setupPermFixture(t, db, env)

		indexURL := mustReverse(t, env.router, "has_permission_admin:index")
		loginURL := mustReverse(t, env.router, "has_permission_admin:login")
		logoutURL := mustReverse(t, env.router, "has_permission_admin:logout")

		// Regular user without perm is rejected.
		resp := env.client.Get(indexURL)
		djangotest.AssertStatus(t, resp, http.StatusFound)
		login := env.client.Post(loginURL, url.Values{"next": {indexURL}, "username": {"joepublic"}, "password": {"secret"}})
		djangotest.AssertContains(t, login, "permission denied")

		// Non-staff user with the required permission can log in.
		resp = env.client.Get(indexURL)
		djangotest.AssertStatus(t, resp, http.StatusFound)
		login = env.client.Post(loginURL, url.Values{"next": {indexURL}, "username": {"nostaff"}, "password": {"secret"}})
		djangotest.AssertRedirects(t, login, indexURL)
		env.client.Post(logoutURL, nil)

		// Staff user can log in.
		resp = env.client.Get(indexURL)
		djangotest.AssertStatus(t, resp, http.StatusFound)
		login = env.client.Post(loginURL, url.Values{"next": {indexURL}, "username": {"deleteuser"}, "password": {"secret"}})
		djangotest.AssertRedirects(t, login, indexURL)
		env.client.Post(logoutURL, nil)
	})
}

// Ported from Django: tests/admin_views/tests.py::AdminViewPermissionsTest.test_login_successfully_redirects_to_original_URL
func TestLoginSuccessfullyRedirectsToOriginalURL(t *testing.T) {
	djangotest.EachDB(t, adminViewsModels(), func(t *testing.T, db *orm.DB) {
		env := newAdminTestEnv(t, db, registerPermModels)
		setupPermFixture(t, db, env)

		indexURL := mustReverse(t, env.router, "admin:index")
		redirectURL := indexURL + "?the-answer=42"
		loginURL := mustReverse(t, env.router, "admin:login") + "?" + url.Values{"next": {redirectURL}}.Encode()
		login := env.client.Post(loginURL, url.Values{"username": {"super"}, "password": {"secret"}})
		djangotest.AssertRedirects(t, login, redirectURL)
	})
}

// Ported from Django: tests/admin_views/tests.py::AdminViewPermissionsTest.test_double_login_is_not_allowed
func TestDoubleLoginIsNotAllowed(t *testing.T) {
	djangotest.EachDB(t, adminViewsModels(), func(t *testing.T, db *orm.DB) {
		env := newAdminTestEnv(t, db, registerPermModels)
		setupPermFixture(t, db, env)

		indexURL := mustReverse(t, env.router, "admin:index")
		loginURL := mustReverse(t, env.router, "admin:login") + "?next=" + indexURL
		superForm := url.Values{"next": {indexURL}, "username": {"super"}, "password": {"secret"}}
		joeForm := url.Values{"next": {indexURL}, "username": {"joepublic"}, "password": {"secret"}}

		login := env.client.Post(loginURL, superForm)
		djangotest.AssertRedirects(t, login, indexURL)

		login = env.client.Post(loginURL, joeForm)
		djangotest.AssertContains(t, login, adminLoginErrorMessage)

		login = env.client.Post(loginURL, superForm)
		djangotest.AssertRedirects(t, login, indexURL)

		login = env.client.Post(loginURL, superForm)
		djangotest.AssertRedirects(t, login, indexURL)
	})
}

// Ported from Django: tests/admin_views/tests.py::AdminViewPermissionsTest.test_login_page_notice_for_non_staff_users
func TestLoginPageNoticeForNonStaffUsers(t *testing.T) {
	djangotest.EachDB(t, adminViewsModels(), func(t *testing.T, db *orm.DB) {
		env := newAdminTestEnv(t, db, registerPermModels)
		u := setupPermFixture(t, db, env)
		indexURL := mustReverse(t, env.router, "admin:index")

		// Anonymous user does not see the authenticated notice.
		resp := env.client.Get(indexURL, djangotest.Follow())
		djangotest.AssertContains(t, resp, "login-form")
		djangotest.AssertNotContains(t, resp, "You are authenticated as ")

		// Authenticated non-staff user sees the notice with their username.
		env.client.ForceLogin(u.noStaff)
		resp = env.client.Get(indexURL, djangotest.Follow())
		djangotest.AssertContains(t, resp, "login-form")
		djangotest.AssertContains(t, resp, "You are authenticated as "+u.noStaff.Username)
	})
}

// Ported from Django: tests/admin_views/tests.py::AdminViewPermissionsTest.test_add_view
func TestAddViewPermissions(t *testing.T) {
	djangotest.EachDB(t, adminViewsModels(), func(t *testing.T, db *orm.DB) {
		env := newAdminTestEnv(t, db, registerPermModels)
		u := setupPermFixture(t, db, env)
		ctx := context.Background()

		addURL := mustReverse(t, env.router, "admin:admin_views_article_add")
		changelistURL := mustReverse(t, env.router, "admin:admin_views_article_changelist")
		indexURL := mustReverse(t, env.router, "admin:index")
		logoutURL := mustReverse(t, env.router, "admin:logout")
		addData := url.Values{
			"title":     {"Døm ikke"},
			"views":     {"10"},
			"published": {"on"},
		}

		// Change user cannot access add view (GET or POST).
		env.client.ForceLogin(u.changeUser)
		djangotest.AssertStatus(t, env.client.Get(addURL), http.StatusForbidden)
		djangotest.AssertStatus(t, env.client.Post(addURL, addData), http.StatusForbidden)
		count, _ := orm.Query[Article](db).Count(ctx)
		djangotest.AssertEqual(t, count, int64(3))
		env.client.Post(logoutURL, nil)

		// View user cannot access add view without add perm.
		env.client.ForceLogin(u.viewUser)
		djangotest.AssertStatus(t, env.client.Get(addURL), http.StatusForbidden)
		djangotest.AssertStatus(t, env.client.Post(addURL, addData), http.StatusForbidden)
		count, _ = orm.Query[Article](db).Count(ctx)
		djangotest.AssertEqual(t, count, int64(3))

		// Grant viewuser add permission (add + view, no change).
		grantPerm(t, db, u.viewUser, "admin_views", "add_article")
		resp := env.client.Get(addURL)
		djangotest.AssertEqual(t, ctxVal(t, resp, "title"), any("Add article"))
		djangotest.AssertContains(t, resp, "<title>Add article | Django site admin</title>")
		djangotest.AssertContains(t, resp, `<input type="submit" value="Save and view" name="_continue">`)
		djangotest.AssertContains(t, resp, `<h2 id="fieldset-0-0-heading" class="fieldset-heading">Some fields</h2>`)
		djangotest.AssertContains(t, resp,
			`<h2 id="fieldset-0-1-heading" class="fieldset-heading">Some other fields</h2>`)
		djangotest.AssertContains(t, resp, `<h2 id="fieldset-0-2-heading" class="fieldset-heading">이름</h2>`)

		post := env.client.Post(addURL, addData)
		djangotest.AssertStatus(t, post, http.StatusFound)
		count, _ = orm.Query[Article](db).Count(ctx)
		djangotest.AssertEqual(t, count, int64(4))
		added, err := orm.Query[Article](db).OrderBy("-id").First(ctx)
		if err != nil {
			t.Fatalf("load added article: %v", err)
		}
		resp = env.client.Get(mustReverse(t, env.router, "admin:admin_views_article_change", added.ID))
		djangotest.AssertContains(t, resp, `<li class="success">The article “Døm ikke” was added successfully.</li>`)
		_, _ = orm.Query[Article](db).Filter("id", added.ID).Delete(ctx)
		env.client.Post(logoutURL, nil)

		// Add-only user sees no changelist link in breadcrumbs and redirects to index.
		env.client.ForceLogin(u.addUser)
		addPage := env.client.Get(addURL)
		changeListLink := `<a href="` + changelistURL + `">Articles</a>`
		djangotest.AssertNotContains(t, addPage, changeListLink)
		post = env.client.Post(addURL, addData)
		djangotest.AssertRedirects(t, post, indexURL)
		count, _ = orm.Query[Article](db).Count(ctx)
		djangotest.AssertEqual(t, count, int64(4))
		env.client.Post(logoutURL, nil)

		// Superuser sees changelist link in breadcrumbs and redirects to changelist.
		env.client.ForceLogin(u.super)
		addPage = env.client.Get(addURL)
		djangotest.AssertContains(t, addPage, changeListLink)
		post = env.client.Post(addURL, addData)
		djangotest.AssertRedirects(t, post, changelistURL)
		count, _ = orm.Query[Article](db).Count(ctx)
		djangotest.AssertEqual(t, count, int64(5))
	})
}

// Ported from Django: tests/admin_views/tests.py::AdminViewPermissionsTest.test_change_view
func TestChangeViewPermissions(t *testing.T) {
	djangotest.EachDB(t, adminViewsModels(), func(t *testing.T, db *orm.DB) {
		env := newAdminTestEnv(t, db, registerPermModels)
		u := setupPermFixture(t, db, env)
		ctx := context.Background()

		articleChangeURL := mustReverse(t, env.router, "admin:admin_views_article_change", u.a1.ID)
		articleChangelistURL := mustReverse(t, env.router, "admin:admin_views_article_changelist")
		indexURL := mustReverse(t, env.router, "admin:index")
		logoutURL := mustReverse(t, env.router, "admin:logout")
		changeData := url.Values{
			"title":     {"Ikke fordømt"},
			"views":     {"99"},
			"published": {"on"},
		}

		// Add-only user cannot view changelist or change form.
		env.client.ForceLogin(u.addUser)
		djangotest.AssertStatus(t, env.client.Get(articleChangelistURL), http.StatusForbidden)
		djangotest.AssertStatus(t, env.client.Get(articleChangeURL), http.StatusForbidden)
		djangotest.AssertStatus(t, env.client.Post(articleChangeURL, changeData), http.StatusForbidden)
		env.client.Post(logoutURL, nil)

		// View-only user sees "Select article to view", read-only change form,
		// Close link, no save buttons, and POST returns 403.
		env.client.ForceLogin(u.viewUser)
		resp := env.client.Get(articleChangelistURL)
		djangotest.AssertContains(t, resp, "<title>Select article to view | Django site admin</title>")
		djangotest.AssertContains(t, resp, "<h1>Select article to view</h1>")
		djangotest.AssertEqual(t, ctxVal(t, resp, "title"), any("Select article to view"))

		resp = env.client.Get(articleChangeURL)
		djangotest.AssertContains(t, resp, "<title>- | View article | Django site admin</title>")
		djangotest.AssertContains(t, resp, "<h1>View article</h1>")
		djangotest.AssertContains(t, resp, `<a role="button" href="`+articleChangelistURL+`" class="closelink">Close</a>`)
		djangotest.AssertContains(t, resp, `<div class="readonly">`)
		djangotest.AssertNotContains(t, resp, `name="_save"`)
		djangotest.AssertNotContains(t, resp, `name="title"`)
		djangotest.AssertEqual(t, ctxVal(t, resp, "title"), any("View article"))

		post := env.client.Post(articleChangeURL, changeData)
		djangotest.AssertStatus(t, post, http.StatusForbidden)
		got, _ := orm.Query[Article](db).Get(ctx, "id", u.a1.ID)
		djangotest.AssertEqual(t, got.Views, int64(10))
		env.client.Post(logoutURL, nil)

		// Change user can view changelist and edit items.
		env.client.ForceLogin(u.changeUser)
		resp = env.client.Get(articleChangelistURL)
		djangotest.AssertEqual(t, ctxVal(t, resp, "title"), any("Select article to change"))
		djangotest.AssertContains(t, resp, "<title>Select article to change | Django site admin</title>")
		djangotest.AssertContains(t, resp, "<h1>Select article to change</h1>")

		resp = env.client.Get(articleChangeURL)
		djangotest.AssertEqual(t, ctxVal(t, resp, "title"), any("Change article"))
		djangotest.AssertContains(t, resp, "<title>- | Change article | Django site admin</title>")
		djangotest.AssertContains(t, resp, "<h1>Change article</h1>")

		post = env.client.Post(articleChangeURL, changeData)
		djangotest.AssertRedirects(t, post, articleChangelistURL)
		got, _ = orm.Query[Article](db).Get(ctx, "id", u.a1.ID)
		djangotest.AssertEqual(t, got.Title, "Ikke fordømt")
		djangotest.AssertEqual(t, got.Views, int64(99))

		// Singular vs plural error messages on validation failure.
		oneErr := env.client.Post(articleChangeURL, url.Values{"title": {""}, "views": {"99"}})
		djangotest.AssertContains(t, oneErr, "Please correct the error below.")
		twoErrs := env.client.Post(articleChangeURL, url.Values{"title": {""}, "views": {"notanint"}})
		djangotest.AssertContains(t, twoErrs, "Please correct the errors below.")
		env.client.Post(logoutURL, nil)

		// Row-level change/view permissions.
		r1 := &RowLevelChangePermissionModel{Name: "A", CanChange: false, CanView: false}
		r2 := &RowLevelChangePermissionModel{Name: "B", CanChange: true, CanView: false}
		r3 := &RowLevelChangePermissionModel{Name: "C", CanChange: false, CanView: true}
		r4 := &RowLevelChangePermissionModel{Name: "D", CanChange: true, CanView: true}
		for _, r := range []*RowLevelChangePermissionModel{r1, r2, r3, r4} {
			if err := orm.Query[RowLevelChangePermissionModel](db).Create(ctx, r); err != nil {
				t.Fatalf("Create(RowLevel): %v", err)
			}
		}
		changeURL1 := mustReverse(t, env.router, "admin:admin_views_rowlevelchangepermissionmodel_change", r1.ID)
		changeURL2 := mustReverse(t, env.router, "admin:admin_views_rowlevelchangepermissionmodel_change", r2.ID)
		changeURL3 := mustReverse(t, env.router, "admin:admin_views_rowlevelchangepermissionmodel_change", r3.ID)
		changeURL4 := mustReverse(t, env.router, "admin:admin_views_rowlevelchangepermissionmodel_change", r4.ID)

		for _, staffUser := range []*auth.User{u.super, u.viewUser, u.addUser, u.changeUser, u.deleteUser} {
			env.client.ForceLogin(staffUser)
			djangotest.AssertStatus(t, env.client.Get(changeURL1), http.StatusForbidden)
			djangotest.AssertStatus(t, env.client.Post(changeURL1, url.Values{"name": {"changed"}}), http.StatusForbidden)
			gotR1, _ := orm.Query[RowLevelChangePermissionModel](db).Get(ctx, "id", r1.ID)
			djangotest.AssertEqual(t, gotR1.Name, "A")

			djangotest.AssertStatus(t, env.client.Get(changeURL2), http.StatusOK)
			resp2 := env.client.Post(changeURL2, url.Values{"name": {"changed"}})
			djangotest.AssertRedirects(t, resp2, indexURL)
			gotR2, _ := orm.Query[RowLevelChangePermissionModel](db).Get(ctx, "id", r2.ID)
			djangotest.AssertEqual(t, gotR2.Name, "changed")

			djangotest.AssertStatus(t, env.client.Get(changeURL3), http.StatusOK)
			djangotest.AssertStatus(t, env.client.Post(changeURL3, url.Values{"name": {"changed"}}), http.StatusForbidden)
			gotR3, _ := orm.Query[RowLevelChangePermissionModel](db).Get(ctx, "id", r3.ID)
			djangotest.AssertEqual(t, gotR3.Name, "C")

			djangotest.AssertStatus(t, env.client.Get(changeURL4), http.StatusOK)
			resp4 := env.client.Post(changeURL4, url.Values{"name": {"changed"}})
			djangotest.AssertRedirects(t, resp4, indexURL)
			gotR4, _ := orm.Query[RowLevelChangePermissionModel](db).Get(ctx, "id", r4.ID)
			djangotest.AssertEqual(t, gotR4.Name, "changed")

			env.client.Post(logoutURL, nil)
		}

		for _, nonStaffUser := range []*auth.User{u.joePublic, u.noStaff} {
			env.client.ForceLogin(nonStaffUser)
			resp1 := env.client.Get(changeURL1, djangotest.Follow())
			djangotest.AssertContains(t, resp1, "login-form")
			resp1p := env.client.Post(changeURL1, url.Values{"name": {"changed"}}, djangotest.Follow())
			djangotest.AssertContains(t, resp1p, "login-form")
			gotR1, _ := orm.Query[RowLevelChangePermissionModel](db).Get(ctx, "id", r1.ID)
			djangotest.AssertEqual(t, gotR1.Name, "A")

			resp2 := env.client.Get(changeURL2, djangotest.Follow())
			djangotest.AssertContains(t, resp2, "login-form")
			resp2p := env.client.Post(changeURL2, url.Values{"name": {"changed again"}}, djangotest.Follow())
			djangotest.AssertContains(t, resp2p, "login-form")
			gotR2, _ := orm.Query[RowLevelChangePermissionModel](db).Get(ctx, "id", r2.ID)
			djangotest.AssertEqual(t, gotR2.Name, "changed")
			env.client.Post(logoutURL, nil)
		}
	})
}

// Ported from Django: tests/admin_views/tests.py::AdminViewPermissionsTest.test_change_view_without_object_change_permission
func TestChangeViewWithoutObjectChangePermission(t *testing.T) {
	djangotest.EachDB(t, adminViewsModels(), func(t *testing.T, db *orm.DB) {
		env := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
			site.Name = "admin9"
			site.Prefix = "/test_admin/admin9"
			admin.Register[Article](site, admin.ModelAdmin{
				HasChangePermission: func(_ *http.Request, obj any) bool {
					return obj == nil
				},
			})
		})
		u := setupPermFixture(t, db, env)
		env.client.ForceLogin(u.viewUser)

		changeURL := mustReverse(t, env.router, "admin9:admin_views_article_change", u.a1.ID)
		resp := env.client.Get(changeURL)
		djangotest.AssertEqual(t, ctxVal(t, resp, "title"), any("View article"))
		djangotest.AssertContains(t, resp, "<title>- | View article | Django site admin</title>")
		djangotest.AssertContains(t, resp, "<h1>View article</h1>")
		djangotest.AssertContains(t, resp, `<a role="button" href="/test_admin/admin9/admin_views/article/" class="closelink">Close</a>`)
	})
}

// Ported from Django: tests/admin_views/tests.py::AdminViewPermissionsTest.test_delete_view
func TestDeleteViewPermissions(t *testing.T) {
	djangotest.EachDB(t, adminViewsModels(), func(t *testing.T, db *orm.DB) {
		env := newAdminTestEnv(t, db, registerPermModels)
		u := setupPermFixture(t, db, env)
		ctx := context.Background()

		deleteURL := mustReverse(t, env.router, "admin:admin_views_article_delete", u.a1.ID)
		indexURL := mustReverse(t, env.router, "admin:index")
		deleteData := url.Values{"post": {"yes"}}

		// Add user cannot delete.
		env.client.ForceLogin(u.addUser)
		djangotest.AssertStatus(t, env.client.Get(deleteURL), http.StatusForbidden)
		djangotest.AssertStatus(t, env.client.Post(deleteURL, deleteData), http.StatusForbidden)
		count, _ := orm.Query[Article](db).Count(ctx)
		djangotest.AssertEqual(t, count, int64(3))
		env.client.Logout()

		// View user cannot delete.
		env.client.ForceLogin(u.viewUser)
		djangotest.AssertStatus(t, env.client.Get(deleteURL), http.StatusForbidden)
		djangotest.AssertStatus(t, env.client.Post(deleteURL, deleteData), http.StatusForbidden)
		count, _ = orm.Query[Article](db).Count(ctx)
		djangotest.AssertEqual(t, count, int64(3))
		env.client.Logout()

		// Delete-only user can delete and is redirected to admin index.
		env.client.ForceLogin(u.deleteUser)
		resp := env.client.Get(deleteURL)
		djangotest.AssertStatus(t, resp, http.StatusOK)
		djangotest.AssertContains(t, resp, "<h1>Delete</h1>")
		post := env.client.Post(deleteURL, deleteData)
		djangotest.AssertRedirects(t, post, indexURL)
		count, _ = orm.Query[Article](db).Count(ctx)
		djangotest.AssertEqual(t, count, int64(2))
	})
}

// Ported from Django: tests/admin_views/tests.py::AdminViewPermissionsTest.test_delete_view_nonexistent_obj
func TestDeleteViewNonexistentObj(t *testing.T) {
	djangotest.EachDB(t, adminViewsModels(), func(t *testing.T, db *orm.DB) {
		env := newAdminTestEnv(t, db, registerPermModels)
		u := setupPermFixture(t, db, env)
		env.client.ForceLogin(u.deleteUser)

		deleteURL := mustReverse(t, env.router, "admin:admin_views_article_delete", "nonexistent")
		resp := env.client.Get(deleteURL, djangotest.Follow())
		djangotest.AssertRedirects(t, resp, mustReverse(t, env.router, "admin:index"))
		djangotest.AssertMessages(t, resp, []messages.Message{
			{Level: messages.LevelWarning, Message: "article with ID “nonexistent” doesn’t exist. Perhaps it was deleted?"},
		})
	})
}

// Ported from Django: tests/admin_views/tests.py::AdminViewPermissionsTest.test_history_view
func TestHistoryViewPermissions(t *testing.T) {
	djangotest.EachDB(t, adminViewsModels(), func(t *testing.T, db *orm.DB) {
		env := newAdminTestEnv(t, db, registerPermModels)
		u := setupPermFixture(t, db, env)
		ctx := context.Background()

		historyURL := mustReverse(t, env.router, "admin:admin_views_article_history", u.a1.ID)
		logoutURL := mustReverse(t, env.router, "admin:logout")

		env.client.ForceLogin(u.addUser)
		djangotest.AssertStatus(t, env.client.Get(historyURL), http.StatusForbidden)
		env.client.Post(logoutURL, nil)

		env.client.ForceLogin(u.viewUser)
		djangotest.AssertStatus(t, env.client.Get(historyURL), http.StatusOK)
		env.client.Post(logoutURL, nil)

		env.client.ForceLogin(u.changeUser)
		djangotest.AssertStatus(t, env.client.Get(historyURL), http.StatusOK)
		env.client.Post(logoutURL, nil)

		// Row-level permission check on history view.
		rl1 := &RowLevelChangePermissionModel{Name: "A", CanChange: false}
		rl2 := &RowLevelChangePermissionModel{Name: "B", CanChange: true}
		_ = orm.Query[RowLevelChangePermissionModel](db).Create(ctx, rl1)
		_ = orm.Query[RowLevelChangePermissionModel](db).Create(ctx, rl2)
		url1 := mustReverse(t, env.router, "admin:admin_views_rowlevelchangepermissionmodel_history", rl1.ID)
		url2 := mustReverse(t, env.router, "admin:admin_views_rowlevelchangepermissionmodel_history", rl2.ID)

		for _, staffUser := range []*auth.User{u.super, u.viewUser, u.addUser, u.changeUser, u.deleteUser} {
			env.client.ForceLogin(staffUser)
			djangotest.AssertStatus(t, env.client.Get(url1), http.StatusForbidden)
			djangotest.AssertStatus(t, env.client.Get(url2), http.StatusOK)
			env.client.Post(logoutURL, nil)
		}
		for _, nonStaffUser := range []*auth.User{u.joePublic, u.noStaff} {
			env.client.ForceLogin(nonStaffUser)
			djangotest.AssertContains(t, env.client.Get(url1, djangotest.Follow()), "login-form")
			djangotest.AssertContains(t, env.client.Get(url2, djangotest.Follow()), "login-form")
			env.client.Post(logoutURL, nil)
		}
	})
}

// Ported from Django: tests/admin_views/tests.py::AdminViewPermissionsTest.test_history_view_bad_url
func TestHistoryViewBadURL(t *testing.T) {
	djangotest.EachDB(t, adminViewsModels(), func(t *testing.T, db *orm.DB) {
		env := newAdminTestEnv(t, db, registerPermModels)
		u := setupPermFixture(t, db, env)
		env.client.ForceLogin(u.changeUser)

		resp := env.client.Get(mustReverse(t, env.router, "admin:admin_views_article_history", "foo"), djangotest.Follow())
		djangotest.AssertRedirects(t, resp, mustReverse(t, env.router, "admin:index"))
		djangotest.AssertMessages(t, resp, []messages.Message{
			{Level: messages.LevelWarning, Message: "article with ID “foo” doesn’t exist. Perhaps it was deleted?"},
		})
	})
}

// Ported from Django: tests/admin_views/tests.py::AdminViewPermissionsTest.test_history_view_without_permission_returns_403
func TestHistoryViewWithoutPermissionReturns403(t *testing.T) {
	djangotest.EachDB(t, adminViewsModels(), func(t *testing.T, db *orm.DB) {
		env := newAdminTestEnv(t, db, registerPermModels)
		u := setupPermFixture(t, db, env)
		env.client.ForceLogin(u.addUser)

		for _, pk := range []string{strconv.FormatInt(u.a1.ID, 10), "999999"} {
			resp := env.client.Get(mustReverse(t, env.router, "admin:admin_views_article_history", pk))
			djangotest.AssertStatus(t, resp, http.StatusForbidden)
		}
	})
}

// Ported from Django: tests/admin_views/tests.py::AdminViewPermissionsTest.test_history_view_with_view_or_change_permission_success
func TestHistoryViewWithViewOrChangePermissionSuccess(t *testing.T) {
	djangotest.EachDB(t, adminViewsModels(), func(t *testing.T, db *orm.DB) {
		env := newAdminTestEnv(t, db, registerPermModels)
		u := setupPermFixture(t, db, env)

		for _, usr := range []*auth.User{u.viewUser, u.changeUser} {
			env.client.ForceLogin(usr)
			resp := env.client.Get(mustReverse(t, env.router, "admin:admin_views_article_history", u.a1.ID))
			djangotest.AssertStatus(t, resp, http.StatusOK)
		}
	})
}

// Ported from Django: tests/admin_views/tests.py::AdminViewPermissionsTest.test_disabled_permissions_when_logged_in
func TestDisabledPermissionsWhenLoggedIn(t *testing.T) {
	djangotest.EachDB(t, adminViewsModels(), func(t *testing.T, db *orm.DB) {
		env := newAdminTestEnv(t, db, registerPermModels)
		u := setupPermFixture(t, db, env)
		env.client.ForceLogin(u.super)

		u.super.IsActive = false
		if err := orm.Query[auth.User](db).Save(context.Background(), u.super); err != nil {
			t.Fatalf("deactivate super: %v", err)
		}

		resp := env.client.Get(mustReverse(t, env.router, "admin:index"), djangotest.Follow())
		djangotest.AssertContains(t, resp, `id="login-form"`)
		djangotest.AssertNotContains(t, resp, "Log out")
	})
}

// Ported from Django: tests/admin_views/tests.py::AdminViewPermissionsTest.test_disabled_staff_permissions_when_logged_in
func TestDisabledStaffPermissionsWhenLoggedIn(t *testing.T) {
	djangotest.EachDB(t, adminViewsModels(), func(t *testing.T, db *orm.DB) {
		env := newAdminTestEnv(t, db, registerPermModels)
		u := setupPermFixture(t, db, env)
		env.client.ForceLogin(u.super)

		u.super.IsStaff = false
		if err := orm.Query[auth.User](db).Save(context.Background(), u.super); err != nil {
			t.Fatalf("remove staff from super: %v", err)
		}

		resp := env.client.Get(mustReverse(t, env.router, "admin:index"), djangotest.Follow())
		djangotest.AssertContains(t, resp, `id="login-form"`)
		djangotest.AssertNotContains(t, resp, "Log out")
	})
}

// Ported from Django: tests/admin_views/tests.py::AdminViewPermissionsTest.test_app_list_permissions
func TestAppListPermissions(t *testing.T) {
	djangotest.EachDB(t, adminViewsModels(), func(t *testing.T, db *orm.DB) {
		env := newAdminTestEnv(t, db, registerPermModels)
		u := setupPermFixture(t, db, env)
		env.client.ForceLogin(u.changeUser)

		appListURL := mustReverse(t, env.router, "admin:app_list", "admin_views")

		// Remove the user's only permission -> 404 on app index.
		revokePerm(t, db, u.changeUser, "admin_views", "change_article")
		djangotest.AssertStatus(t, env.client.Get(appListURL), http.StatusNotFound)

		// Restore the permission -> 200 on app index.
		grantPerm(t, db, u.changeUser, "admin_views", "change_article")
		djangotest.AssertStatus(t, env.client.Get(appListURL), http.StatusOK)
	})
}

// Ported from Django: tests/admin_views/tests.py::AdminViewPermissionsTest.test_has_module_permission
func TestHasModulePermission(t *testing.T) {
	djangotest.EachDB(t, adminViewsModels(), func(t *testing.T, db *orm.DB) {
		env := newAdminTestEnv(t, db, registerPermModels)
		u := setupPermFixture(t, db, env)
		indexURL := mustReverse(t, env.router, "admin:index")

		for _, usr := range []*auth.User{u.super, u.viewUser, u.addUser, u.changeUser, u.deleteUser} {
			env.client.ForceLogin(usr)
			resp := env.client.Get(indexURL)
			djangotest.AssertContains(t, resp, "admin_views")
			djangotest.AssertContains(t, resp, "Articles")
			env.client.Logout()
		}

		// A staff user with no permissions sees no models and gets the empty notice.
		noPermStaff := createTestUser(t, db, "noperm", "secret", true, false)
		env.client.ForceLogin(noPermStaff)
		resp := env.client.Get(indexURL)
		djangotest.AssertNotContains(t, resp, "Articles")
		djangotest.AssertNotContains(t, resp, "Sections")
		djangotest.AssertContains(t, resp, "You don’t have permission to view or edit anything.")
	})
}

// Ported from Django: tests/admin_views/tests.py::AdminViewPermissionsTest.test_overriding_has_module_permission
func TestOverridingHasModulePermission(t *testing.T) {
	djangotest.EachDB(t, adminViewsModels(), func(t *testing.T, db *orm.DB) {
		env := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
			site.Name = "admin7"
			site.Prefix = "/admin7"
			admin.Register[Article](site, admin.ModelAdmin{
				HasModulePermission: func(*http.Request) bool { return false },
			})
			admin.Register[Section](site, admin.ModelAdmin{})
		})
		u := setupPermFixture(t, db, env)
		indexURL := mustReverse(t, env.router, "admin7:index")

		env.client.ForceLogin(u.super)
		resp := env.client.Get(indexURL)
		djangotest.AssertContains(t, resp, "Sections")
		djangotest.AssertNotContains(t, resp, "Articles")
		env.client.Logout()

		for _, usr := range []*auth.User{u.viewUser, u.addUser, u.changeUser} {
			env.client.ForceLogin(usr)
			resp := env.client.Get(indexURL)
			djangotest.AssertNotContains(t, resp, "admin_views")
			djangotest.AssertNotContains(t, resp, "Articles")
			env.client.Logout()
		}

		env.client.ForceLogin(u.deleteUser)
		resp = env.client.Get(indexURL)
		djangotest.AssertNotContains(t, resp, "Articles")
		env.client.Logout()

		// App index for superuser lists Sections but not Articles.
		env.client.ForceLogin(u.super)
		resp = env.client.Get(mustReverse(t, env.router, "admin7:app_list", "admin_views"))
		djangotest.AssertContains(t, resp, "Sections")
		djangotest.AssertNotContains(t, resp, "Articles")
	})
}

// Ported from Django: tests/admin_views/tests.py::AdminViewPermissionsTest.test_post_save_message_no_forbidden_links_visible
func TestPostSaveMessageNoForbiddenLinksVisible(t *testing.T) {
	djangotest.EachDB(t, adminViewsModels(), func(t *testing.T, db *orm.DB) {
		env := newAdminTestEnv(t, db, registerPermModels)
		u := setupPermFixture(t, db, env)
		env.client.ForceLogin(u.addUser)

		resp := env.client.Post(mustReverse(t, env.router, "admin:admin_views_article_add"), url.Values{
			"title": {"Fun & games"},
			"views": {"5"},
			"_save": {"Save"},
		}, djangotest.Follow())
		djangotest.AssertContains(t, resp,
			`<li class="success">The article “Fun &amp; games” was added successfully.</li>`,
			djangotest.HTML(),
		)
	})
}

// SectionedPost has a foreign key to Section so the view-only change form test
// verifies both scalar fields and FK label resolution in read-only divs.
type SectionedPost struct {
	ID      int64
	Title   string `orm:"max_length=120"`
	Section orm.FK[Section]
}

// String returns the post title.
func (p SectionedPost) String() string { return p.Title }

// TestViewOnlyChangeFormRendersReadonlyValuesAndFK verifies that a view-only
// user sees scalar values and FK links in <div class="readonly"> elements without
// editable inputs or save/delete buttons, and sees the Delete link once delete
// permission is also granted.
func TestViewOnlyChangeFormRendersReadonlyValuesAndFK(t *testing.T) {
	models := append(adminViewsModels(), djangotest.App("blog", &SectionedPost{}))
	djangotest.EachDB(t, models, func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		env := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
			admin.Register[Section](site, admin.ModelAdmin{})
			admin.Register[SectionedPost](site, admin.ModelAdmin{})
		})
		sec := &Section{Name: "Tech"}
		if err := orm.Query[Section](db).Create(ctx, sec); err != nil {
			t.Fatalf("Create(Section): %v", err)
		}
		post := &SectionedPost{Title: "Read Only Entry"}
		post.Section.SetPK(sec.ID)
		if err := orm.Query[SectionedPost](db).Create(ctx, post); err != nil {
			t.Fatalf("Create(SectionedPost): %v", err)
		}

		viewer := createTestUser(t, db, "viewer", "secret", true, false)
		grantPerm(t, db, viewer, "blog", "view_sectionedpost")
		env.client.ForceLogin(viewer)

		changeURL := mustReverse(t, env.router, "admin:blog_sectionedpost_change", post.ID)
		resp := env.client.Get(changeURL)
		djangotest.AssertStatus(t, resp, http.StatusOK)
		djangotest.AssertContains(t, resp, `<div class="readonly">Read Only Entry</div>`)
		// The related model has an admin, so the value links to its change
		// page, as Django's AdminReadonlyField.get_admin_url.
		secURL := mustReverse(t, env.router, "admin:admin_views_section_change", sec.ID)
		djangotest.AssertContains(t, resp, `<div class="readonly"><a href="`+secURL+`">Tech</a></div>`)
		djangotest.AssertNotContains(t, resp, `<select`)
		djangotest.AssertNotContains(t, resp, `name="title"`)
		djangotest.AssertNotContains(t, resp, `name="_save"`)
		djangotest.AssertNotContains(t, resp, `class="deletelink"`)
		djangotest.AssertContains(t, resp, `class="closelink"`)

		// Grant delete permission too -> Delete link appears while form stays read-only.
		grantPerm(t, db, viewer, "blog", "delete_sectionedpost")
		resp = env.client.Get(changeURL)
		djangotest.AssertContains(t, resp, `class="deletelink"`)
		djangotest.AssertNotContains(t, resp, `name="_save"`)
	})
}

// TestModelAdminPermissionOverrides verifies HasAddPermission and
// HasDeletePermission hooks on ModelAdmin override the default auth checks.
func TestModelAdminPermissionOverrides(t *testing.T) {
	djangotest.EachDB(t, adminViewsModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		env := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
			admin.Register[Section](site, admin.ModelAdmin{
				HasAddPermission:    func(*http.Request) bool { return false },
				HasDeletePermission: func(*http.Request, any) bool { return true },
			})
		})
		sec := &Section{Name: "Pinned"}
		if err := orm.Query[Section](db).Create(ctx, sec); err != nil {
			t.Fatalf("Create(Section): %v", err)
		}

		// Even a superuser cannot add when HasAddPermission returns false.
		super := createTestUser(t, db, "super", "secret", true, true)
		env.client.ForceLogin(super)
		djangotest.AssertStatus(t, env.client.Get(mustReverse(t, env.router, "admin:admin_views_section_add")), http.StatusForbidden)
		env.client.Logout()

		// A plain staff user without DB delete_section perm can still delete
		// when HasDeletePermission returns true.
		staff := createTestUser(t, db, "staff", "secret", true, false)
		env.client.ForceLogin(staff)
		delURL := mustReverse(t, env.router, "admin:admin_views_section_delete", sec.ID)
		djangotest.AssertStatus(t, env.client.Get(delURL), http.StatusOK)
		djangotest.AssertRedirects(t, env.client.Post(delURL, url.Values{"post": {"yes"}}), mustReverse(t, env.router, "admin:index"))
	})
}
