package admin_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/admin"
	"github.com/oliverhaas/djangogo/apps"
	"github.com/oliverhaas/djangogo/auth"
	"github.com/oliverhaas/djangogo/contenttypes"
	"github.com/oliverhaas/djangogo/csrf"
	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/messages"
	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/sessions"
	"github.com/oliverhaas/djangogo/urls"
)

// Section stands in for tests/admin_views/models.py::Section.
type Section struct {
	ID   int64
	Name string `orm:"max_length=100"`
}

// String returns the section name.
func (s Section) String() string { return s.Name }

// Actor stands in for tests/admin_views/models.py::Actor.
type Actor struct {
	ID    int64
	Name  string `orm:"max_length=50"`
	Age   int64
	Title *string `orm:"max_length=50;null;blank"`
}

// String returns the actor name.
func (a Actor) String() string { return a.Name }

// Worker stands in for tests/admin_views/models.py::Worker.
type Worker struct {
	ID      int64
	WorkAt  orm.FK[Restaurant] `orm:"on_delete=cascade"`
	Name    string             `orm:"max_length=50"`
	Surname string             `orm:"max_length=50"`
}

// String returns the worker name.
func (w Worker) String() string { return w.Name }

// adminViewsModels returns the model set used by the ported admin_views tests.
func adminViewsModels() []any {
	return []any{
		auth.App{},
		contenttypes.App{},
		admin.App{},
		djangotest.App("admin_views",
			&Article{},
			&Section{},
			&Actor{},
			&State{},
			&City{},
			&Restaurant{},
			&Worker{},
			&RowLevelChangePermissionModel{},
			&Album{},
		),
	}
}

// adminTestEnv holds a wired admin site, router, middleware stack, test client,
// and database for ported Django admin_views tests.
type adminTestEnv struct {
	db     *orm.DB
	site   *admin.AdminSite
	router *urls.Router
	store  sessions.Store
	client *djangotest.Client
}

// newAdminTestEnv builds an AdminSite registered with the given callback, mounts
// its routes on a urls.Router behind the standard middleware stack (sessions ->
// csrf -> auth -> messages -> router), syncs auth permissions, and returns a
// djangotest.Client configured for it.
func newAdminTestEnv(t *testing.T, db *orm.DB, configure func(site *admin.AdminSite), extraRoutes ...urls.Route) *adminTestEnv {
	t.Helper()
	if _, err := auth.SyncPermissions(context.Background(), db); err != nil {
		t.Fatalf("SyncPermissions: %v", err)
	}
	site, err := admin.NewAdminSite(db)
	if err != nil {
		t.Fatalf("NewAdminSite: %v", err)
	}
	if configure != nil {
		configure(site)
	}
	siteRoutes := site.Routes()
	allRoutes := make([]urls.Route, 0, len(extraRoutes)+len(siteRoutes))
	allRoutes = append(allRoutes, extraRoutes...)
	allRoutes = append(allRoutes, siteRoutes...)
	router := urls.NewRouter(allRoutes...)
	store := sessions.NewSignedCookieStore([]byte("admin-test-secret"))
	var h http.Handler = router
	h = messages.Middleware(h)
	h = auth.Middleware(db)(h)
	h = csrf.Middleware(h)
	h = sessions.Middleware(store, djangotest.DefaultSessionCookie)(h)
	client := djangotest.NewClient(t, h,
		djangotest.WithDB(db),
		djangotest.WithSessions(store, djangotest.DefaultSessionCookie),
		djangotest.WithMessages(),
	)
	return &adminTestEnv{
		db:     db,
		site:   site,
		router: router,
		store:  store,
		client: client,
	}
}

// createTestUser creates a user in db with the given password and flags.
func createTestUser(t *testing.T, db *orm.DB, username, password string, isStaff, isSuperuser bool) *auth.User {
	t.Helper()
	u := &auth.User{
		Username:    username,
		IsStaff:     isStaff,
		IsSuperuser: isSuperuser,
		IsActive:    true,
		// Django's create_user sets date_joined through its default.
		DateJoined: time.Now().UTC(),
	}
	if err := u.SetPassword(password); err != nil {
		t.Fatalf("SetPassword(%s): %v", username, err)
	}
	if err := orm.Query[auth.User](db).Create(context.Background(), u); err != nil {
		t.Fatalf("Create(%s): %v", username, err)
	}
	return u
}

// grantPerm grants "<appLabel>.<codename>" to u in db.
func grantPerm(t *testing.T, db *orm.DB, u *auth.User, appLabel, codename string) {
	t.Helper()
	ctx := context.Background()
	ct, err := contenttypes.GetByNaturalKey(ctx, db, appLabel, modelFromCodename(codename))
	if err != nil {
		t.Fatalf("GetByNaturalKey(%s, %s): %v", appLabel, codename, err)
	}
	p, err := orm.Query[auth.Permission](db).Get(ctx, "content_type_id", ct.ID, "codename", codename)
	if err != nil {
		t.Fatalf("Get permission %s.%s: %v", appLabel, codename, err)
	}
	if err := u.AddPermissions(ctx, db, &p); err != nil {
		t.Fatalf("AddPermissions %s.%s: %v", appLabel, codename, err)
	}
}

// revokePerm revokes "<appLabel>.<codename>" from u in db.
func revokePerm(t *testing.T, db *orm.DB, u *auth.User, appLabel, codename string) {
	t.Helper()
	ctx := context.Background()
	ct, err := contenttypes.GetByNaturalKey(ctx, db, appLabel, modelFromCodename(codename))
	if err != nil {
		t.Fatalf("GetByNaturalKey(%s, %s): %v", appLabel, codename, err)
	}
	p, err := orm.Query[auth.Permission](db).Get(ctx, "content_type_id", ct.ID, "codename", codename)
	if err != nil {
		t.Fatalf("Get permission %s.%s: %v", appLabel, codename, err)
	}
	if err := orm.Many[auth.Permission](db, u, "user_permissions").Remove(ctx, &p); err != nil {
		t.Fatalf("Remove permission %s.%s: %v", appLabel, codename, err)
	}
}

// modelFromCodename extracts "<model>" from "<action>_<model>".
func modelFromCodename(codename string) string {
	for _, prefix := range []string{"add_", "change_", "delete_", "view_"} {
		if len(codename) > len(prefix) && codename[:len(prefix)] == prefix {
			return codename[len(prefix):]
		}
	}
	return codename
}

// mustReverse reverses name on router, failing the test on error.
func mustReverse(t *testing.T, router *urls.Router, name string, args ...any) string {
	t.Helper()
	u, err := router.Reverse(name, args...)
	if err != nil {
		t.Fatalf("Reverse(%q, %v): %v", name, args, err)
	}
	return u
}

// ctxVal returns resp.Context(key), failing the test when key is absent.
func ctxVal(t *testing.T, resp *djangotest.Response, key string) any {
	t.Helper()
	v, ok := resp.Context(key)
	if !ok {
		t.Fatalf("template context missing key %q", key)
	}
	return v
}

// registerBasicAdminModels returns the registration of the standard
// admin_views models, with users and groups under Django's UserAdmin and
// GroupAdmin as in tests/admin_views/admin.py.
func registerBasicAdminModels(db *orm.DB) func(site *admin.AdminSite) {
	return func(site *admin.AdminSite) {
		admin.Register[Article](site, admin.ModelAdmin{})
		admin.Register[Section](site, admin.ModelAdmin{})
		admin.Register[Actor](site, admin.ModelAdmin{})
		admin.Register[Worker](site, admin.ModelAdmin{})
		admin.Register[auth.User](site, admin.UserAdmin(db))
		admin.Register[auth.Group](site, admin.GroupAdmin())
	}
}

// Ported from Django: tests/admin_views/test_adminsite.py::SiteEachContextTest.test_each_context
func TestEachContext(t *testing.T) {
	djangotest.EachDB(t, adminViewsModels(), func(t *testing.T, db *orm.DB) {
		super := createTestUser(t, db, "super", "secret", true, true)
		env := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
			site.Name = "test_adminsite"
			site.Prefix = "/test_admin/admin"
			admin.Register[auth.User](site, admin.ModelAdmin{})
			admin.Register[Article](site, admin.ModelAdmin{})
		})
		req := httptest.NewRequest(http.MethodGet, mustReverse(t, env.router, "test_adminsite:index"), nil)
		req = req.WithContext(auth.WithUser(req.Context(), super))
		ctx := env.site.EachContext(req)
		djangotest.AssertEqual(t, ctx["site_header"], any("Django administration"))
		djangotest.AssertEqual(t, ctx["site_title"], any("Django site admin"))
		djangotest.AssertEqual(t, ctx["site_url"], any("/"))
		djangotest.AssertEqual(t, ctx["has_permission"], any(true))
	})
}

// Ported from Django: tests/admin_views/test_adminsite.py::SiteEachContextTest.test_custom_admin_titles
func TestCustomAdminTitles(t *testing.T) {
	djangotest.EachDB(t, adminViewsModels(), func(t *testing.T, db *orm.DB) {
		super := createTestUser(t, db, "super", "secret", true, true)
		env := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
			site.Name = "test_custom_adminsite"
			site.Prefix = "/test_custom_admin/admin"
			site.SiteTitle = "Custom title"
			site.SiteHeader = "Custom site"
			admin.Register[auth.User](site, admin.ModelAdmin{})
		})
		req := httptest.NewRequest(http.MethodGet, mustReverse(t, env.router, "test_custom_adminsite:index"), nil)
		req = req.WithContext(auth.WithUser(req.Context(), super))
		ctx := env.site.EachContext(req)
		djangotest.AssertEqual(t, ctx["site_title"], any("Custom title"))
		djangotest.AssertEqual(t, ctx["site_header"], any("Custom site"))
	})
}

// Ported from Django: tests/admin_views/test_adminsite.py::SiteEachContextTest.test_available_apps
func TestAvailableApps(t *testing.T) {
	djangotest.EachDB(t, adminViewsModels(), func(t *testing.T, db *orm.DB) {
		super := createTestUser(t, db, "super", "secret", true, true)
		env := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
			site.Name = "test_adminsite"
			site.Prefix = "/test_admin/admin"
			admin.Register[auth.User](site, admin.ModelAdmin{})
			admin.Register[Article](site, admin.ModelAdmin{})
		})
		req := httptest.NewRequest(http.MethodGet, mustReverse(t, env.router, "test_adminsite:index"), nil)
		req = req.WithContext(auth.WithUser(req.Context(), super))
		ctx := env.site.EachContext(req)

		appList, ok := ctx["available_apps"].([]map[string]any)
		if !ok {
			t.Fatalf("available_apps is %T, want []map[string]any", ctx["available_apps"])
		}
		djangotest.AssertEqual(t, len(appList), 2)

		// admin_views.Article sorts before auth.User.
		adminViews := appList[0]
		djangotest.AssertEqual(t, adminViews["app_label"], any("admin_views"))
		avModels := adminViews["models"].([]map[string]any)
		djangotest.AssertEqual(t, len(avModels), 1)
		djangotest.AssertEqual(t, avModels[0]["object_name"], any("Article"))

		authApp := appList[1]
		djangotest.AssertEqual(t, authApp["app_label"], any("auth"))
		djangotest.AssertEqual(t, authApp["app_url"], any("/test_admin/admin/auth/"))
		djangotest.AssertEqual(t, authApp["has_module_perms"], any(true))
		authModels := authApp["models"].([]map[string]any)
		djangotest.AssertEqual(t, len(authModels), 1)
		userModel := authModels[0]
		djangotest.AssertEqual(t, userModel["object_name"], any("User"))
		perms := userModel["perms"].(map[string]bool)
		djangotest.AssertEqual(t, perms["add"], true)
		djangotest.AssertEqual(t, perms["change"], true)
		djangotest.AssertEqual(t, perms["delete"], true)
		djangotest.AssertEqual(t, perms["view"], true)
		djangotest.AssertEqual(t, userModel["admin_url"], any("/test_admin/admin/auth/user/"))
		djangotest.AssertEqual(t, userModel["add_url"], any("/test_admin/admin/auth/user/add/"))
		djangotest.AssertEqual(t, userModel["name"], any("Users"))
	})
}

// Ported from Django: tests/admin_views/tests.py::AdminViewBasicTest.test_trailing_slash_required
func TestTrailingSlashRequired(t *testing.T) {
	djangotest.EachDB(t, adminViewsModels(), func(t *testing.T, db *orm.DB) {
		super := createTestUser(t, db, "super", "secret", true, true)
		env := newAdminTestEnv(t, db, registerBasicAdminModels(db))
		env.client.ForceLogin(super)

		addURL := mustReverse(t, env.router, "admin:admin_views_article_add")
		resp := env.client.Get(addURL[:len(addURL)-1])
		djangotest.AssertRedirects(t, resp, addURL, djangotest.StatusCode(http.StatusMovedPermanently))
	})
}

// Ported from Django: tests/admin_views/tests.py::AdminViewBasicTest.test_basic_add_GET
func TestBasicAddGET(t *testing.T) {
	djangotest.EachDB(t, adminViewsModels(), func(t *testing.T, db *orm.DB) {
		super := createTestUser(t, db, "super", "secret", true, true)
		env := newAdminTestEnv(t, db, registerBasicAdminModels(db))
		env.client.ForceLogin(super)

		resp := env.client.Get(mustReverse(t, env.router, "admin:admin_views_section_add"))
		djangotest.AssertStatus(t, resp, http.StatusOK)
		djangotest.AssertTemplateUsed(t, resp, "admin/change_form.html")
	})
}

// Ported from Django: tests/admin_views/tests.py::AdminViewBasicTest.test_basic_edit_GET
func TestBasicEditGET(t *testing.T) {
	djangotest.EachDB(t, adminViewsModels(), func(t *testing.T, db *orm.DB) {
		super := createTestUser(t, db, "super", "secret", true, true)
		s1 := &Section{Name: "Test section"}
		if err := orm.Query[Section](db).Create(context.Background(), s1); err != nil {
			t.Fatalf("Create(Section): %v", err)
		}
		env := newAdminTestEnv(t, db, registerBasicAdminModels(db))
		env.client.ForceLogin(super)

		resp := env.client.Get(mustReverse(t, env.router, "admin:admin_views_section_change", s1.ID))
		djangotest.AssertStatus(t, resp, http.StatusOK)
		djangotest.AssertTemplateUsed(t, resp, "admin/change_form.html")
	})
}

// Ported from Django: tests/admin_views/tests.py::AdminViewBasicTest.test_basic_edit_GET_string_PK
func TestBasicEditGETStringPK(t *testing.T) {
	djangotest.EachDB(t, adminViewsModels(), func(t *testing.T, db *orm.DB) {
		super := createTestUser(t, db, "super", "secret", true, true)
		env := newAdminTestEnv(t, db, registerBasicAdminModels(db))
		env.client.ForceLogin(super)

		resp := env.client.Get(mustReverse(t, env.router, "admin:admin_views_section_change", "abc"), djangotest.Follow())
		djangotest.AssertRedirects(t, resp, mustReverse(t, env.router, "admin:index"))
		djangotest.AssertMessages(t, resp, []messages.Message{
			{Level: messages.LevelWarning, Message: "section with ID “abc” doesn’t exist. Perhaps it was deleted?"},
		})
	})
}

// Ported from Django: tests/admin_views/tests.py::AdminViewBasicTest.test_basic_edit_GET_old_url_redirect
func TestBasicEditGETOldURLRedirect(t *testing.T) {
	djangotest.EachDB(t, adminViewsModels(), func(t *testing.T, db *orm.DB) {
		super := createTestUser(t, db, "super", "secret", true, true)
		s1 := &Section{Name: "Test section"}
		if err := orm.Query[Section](db).Create(context.Background(), s1); err != nil {
			t.Fatalf("Create(Section): %v", err)
		}
		env := newAdminTestEnv(t, db, registerBasicAdminModels(db))
		env.client.ForceLogin(super)

		changeURL := mustReverse(t, env.router, "admin:admin_views_section_change", s1.ID)
		oldURL := "/admin/admin_views/section/" + strconv.FormatInt(s1.ID, 10) + "/"
		resp := env.client.Get(oldURL)
		djangotest.AssertRedirects(t, resp, changeURL)
	})
}

// Ported from Django: tests/admin_views/tests.py::AdminViewBasicTest.test_logout_and_password_change_URLs
func TestLogoutAndPasswordChangeURLs(t *testing.T) {
	djangotest.EachDB(t, adminViewsModels(), func(t *testing.T, db *orm.DB) {
		super := createTestUser(t, db, "super", "secret", true, true)
		env := newAdminTestEnv(t, db, registerBasicAdminModels(db))
		env.client.ForceLogin(super)

		resp := env.client.Get(mustReverse(t, env.router, "admin:admin_views_article_changelist"))
		djangotest.AssertContains(t, resp, `<form id="logout-form" method="post" action="`+mustReverse(t, env.router, "admin:logout")+`">`)
		djangotest.AssertContains(t, resp, `<a href="`+mustReverse(t, env.router, "admin:password_change")+`">`)
	})
}

// Ported from Django: tests/admin_views/tests.py::AdminViewBasicTest.test_invalid_appindex_url
func TestInvalidAppIndexURL(t *testing.T) {
	djangotest.EachDB(t, adminViewsModels(), func(t *testing.T, db *orm.DB) {
		env := newAdminTestEnv(t, db, registerBasicAdminModels(db))

		goodURL := "/admin/admin_views/"
		confirmGoodURL := mustReverse(t, env.router, "admin:app_list", urls.Kwargs{"app_label": "admin_views"})
		djangotest.AssertEqual(t, confirmGoodURL, goodURL)

		_, err := env.router.Reverse("admin:app_list", urls.Kwargs{"app_label": "this_should_fail"})
		djangotest.AssertErrorIs(t, err, urls.ErrNoReverseMatch)

		_, err = env.router.Reverse("admin:app_list", "admin_views2")
		djangotest.AssertErrorIs(t, err, urls.ErrNoReverseMatch)
	})
}

// Ported from Django: tests/admin_views/tests.py::AdminViewBasicTest.test_adminsite_display_site_url
func TestAdminSiteDisplaySiteURL(t *testing.T) {
	djangotest.EachDB(t, adminViewsModels(), func(t *testing.T, db *orm.DB) {
		super := createTestUser(t, db, "super", "secret", true, true)
		env := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
			site.SiteURL = "/my-site-url/"
			registerBasicAdminModels(db)(site)
		})
		env.client.ForceLogin(super)

		resp := env.client.Get(mustReverse(t, env.router, "admin:index"))
		djangotest.AssertEqual(t, ctxVal(t, resp, "site_url"), any("/my-site-url/"))
		djangotest.AssertContains(t, resp, `<a href="/my-site-url/">View site</a>`)
	})
}

// Ported from Django: tests/admin_views/tests.py::AdminViewBasicTest.test_app_index_context
func TestAppIndexContext(t *testing.T) {
	djangotest.EachDB(t, adminViewsModels(), func(t *testing.T, db *orm.DB) {
		super := createTestUser(t, db, "super", "secret", true, true)
		env := newAdminTestEnv(t, db, registerBasicAdminModels(db))
		env.client.ForceLogin(super)

		resp := env.client.Get(mustReverse(t, env.router, "admin:app_list", "admin_views"))
		djangotest.AssertContains(t, resp, "<title>Admin_Views administration | Django site admin</title>")
		djangotest.AssertEqual(t, ctxVal(t, resp, "title"), any("Admin_Views administration"))
		djangotest.AssertEqual(t, ctxVal(t, resp, "app_label"), any("admin_views"))

		appList := ctxVal(t, resp, "app_list").([]map[string]any)
		rawModels := appList[0]["models"].([]map[string]any)
		names := make([]string, len(rawModels))
		for i, m := range rawModels {
			names[i] = m["name"].(string)
		}
		sorted := slices.Clone(names)
		slices.Sort(sorted)
		djangotest.AssertSequenceEqual(t, names, sorted)
	})
}

// Ported from Django: tests/admin_views/tests.py::AdminViewBasicTest.test_password_change_helptext
func TestPasswordChangeHelptext(t *testing.T) {
	djangotest.EachDB(t, adminViewsModels(), func(t *testing.T, db *orm.DB) {
		super := createTestUser(t, db, "super", "secret", true, true)
		env := newAdminTestEnv(t, db, registerBasicAdminModels(db))
		env.client.ForceLogin(super)

		resp := env.client.Get(mustReverse(t, env.router, "admin:password_change"))
		djangotest.AssertStatus(t, resp, http.StatusOK)
		djangotest.AssertContains(t, resp, `<div class="help" id="id_new_password1_helptext">`)
	})
}

// Ported from Django: tests/admin_views/tests.py::AdminViewBasicTest.test_header
func TestHeader(t *testing.T) {
	djangotest.EachDB(t, adminViewsModels(), func(t *testing.T, db *orm.DB) {
		super := createTestUser(t, db, "super", "secret", true, true)
		env := newAdminTestEnv(t, db, registerBasicAdminModels(db))
		env.client.ForceLogin(super)

		resp := env.client.Get(mustReverse(t, env.router, "admin:index"))
		djangotest.AssertContains(t, resp, `<header id="header">`)
		env.client.Logout()
		resp = env.client.Get(mustReverse(t, env.router, "admin:login"))
		djangotest.AssertContains(t, resp, `<header id="header">`)
	})
}

// Ported from Django: tests/admin_views/tests.py::AdminViewBasicTest.test_main_content
func TestMainContent(t *testing.T) {
	djangotest.EachDB(t, adminViewsModels(), func(t *testing.T, db *orm.DB) {
		super := createTestUser(t, db, "super", "secret", true, true)
		env := newAdminTestEnv(t, db, registerBasicAdminModels(db))
		env.client.ForceLogin(super)

		resp := env.client.Get(mustReverse(t, env.router, "admin:index"))
		djangotest.AssertContains(t, resp, `<main id="content-start" class="content" tabindex="-1">`)
	})
}

// Ported from Django: tests/admin_views/tests.py::AdminViewBasicTest.test_footer
func TestFooter(t *testing.T) {
	djangotest.EachDB(t, adminViewsModels(), func(t *testing.T, db *orm.DB) {
		super := createTestUser(t, db, "super", "secret", true, true)
		env := newAdminTestEnv(t, db, registerBasicAdminModels(db))
		env.client.ForceLogin(super)

		resp := env.client.Get(mustReverse(t, env.router, "admin:index"))
		djangotest.AssertContains(t, resp, `<footer id="footer">`)
		env.client.Logout()
		resp = env.client.Get(mustReverse(t, env.router, "admin:login"))
		djangotest.AssertContains(t, resp, `<footer id="footer">`)
	})
}

// Ported from Django: tests/admin_views/tests.py::AdminViewBasicTest.test_aria_describedby_for_add_and_change_links
func TestAriaDescribedbyForAddAndChangeLinks(t *testing.T) {
	djangotest.EachDB(t, adminViewsModels(), func(t *testing.T, db *orm.DB) {
		super := createTestUser(t, db, "super", "secret", true, true)
		env := newAdminTestEnv(t, db, registerBasicAdminModels(db))
		env.client.ForceLogin(super)

		resp := env.client.Get(mustReverse(t, env.router, "admin:index"))
		cases := [][2]string{
			{"admin_views", "actor"},
			{"admin_views", "worker"},
			{"auth", "group"},
			{"auth", "user"},
		}
		for _, tc := range cases {
			appLabel, modelName := tc[0], tc[1]
			rowID := appLabel + "-" + modelName
			djangotest.AssertContains(t, resp, `<th scope="row" id="`+rowID+`">`)
			djangotest.AssertContains(t, resp,
				`<a href="/admin/`+appLabel+`/`+modelName+`/" class="changelink" aria-describedby="`+rowID+`">Change</a>`)
			djangotest.AssertContains(t, resp,
				`<a href="/admin/`+appLabel+`/`+modelName+`/add/" class="addlink" aria-describedby="`+rowID+`">Add</a>`)
		}
	})
}

// Ported from Django: tests/admin_views/tests.py::AdminViewLogoutTests.test_logout
func TestLogout(t *testing.T) {
	djangotest.EachDB(t, adminViewsModels(), func(t *testing.T, db *orm.DB) {
		super := createTestUser(t, db, "super", "secret", true, true)
		env := newAdminTestEnv(t, db, registerBasicAdminModels(db))
		env.client.ForceLogin(super)

		logoutURL := mustReverse(t, env.router, "admin:logout")
		resp := env.client.Post(logoutURL, nil)
		djangotest.AssertStatus(t, resp, http.StatusOK)
		djangotest.AssertTemplateUsed(t, resp, "registration/logged_out.html")
		djangotest.AssertEqual(t, resp.Request.URL.Path, logoutURL)
		djangotest.AssertEqual(t, ctxVal(t, resp, "has_permission"), any(false))
		djangotest.AssertNotContains(t, resp, "user-tools")
	})
}

// Ported from Django: tests/admin_views/tests.py::AdminViewLogoutTests.test_client_logout_url_can_be_used_to_login
func TestClientLogoutURLCanBeUsedToLogin(t *testing.T) {
	djangotest.EachDB(t, adminViewsModels(), func(t *testing.T, db *orm.DB) {
		env := newAdminTestEnv(t, db, registerBasicAdminModels(db))

		logoutURL := mustReverse(t, env.router, "admin:logout")
		resp := env.client.Post(logoutURL, nil)
		djangotest.AssertStatus(t, resp, http.StatusFound)

		resp = env.client.Post(logoutURL, nil, djangotest.Follow())
		djangotest.AssertContains(t, resp, `<input type="hidden" name="next" value="`+mustReverse(t, env.router, "admin:index")+`">`)
		djangotest.AssertTemplateUsed(t, resp, "admin/login.html")
		djangotest.AssertEqual(t, resp.Request.URL.Path, mustReverse(t, env.router, "admin:login"))
	})
}

// Ported from Django: tests/admin_views/tests.py::AdminSiteFinalCatchAllPatternTests.test_unknown_url_redirects_login_if_not_authenticated
func TestUnknownURLRedirectsLoginIfNotAuthenticated(t *testing.T) {
	djangotest.EachDB(t, adminViewsModels(), func(t *testing.T, db *orm.DB) {
		env := newAdminTestEnv(t, db, registerBasicAdminModels(db))
		resp := env.client.Get("/admin/unknown/")
		djangotest.AssertRedirects(t, resp, "/admin/login/?next=/admin/unknown/")
	})
}

// Ported from Django: tests/admin_views/tests.py::AdminSiteFinalCatchAllPatternTests.test_unknown_url_404_if_authenticated
func TestUnknownURL404IfAuthenticated(t *testing.T) {
	djangotest.EachDB(t, adminViewsModels(), func(t *testing.T, db *orm.DB) {
		super := createTestUser(t, db, "super", "secret", true, true)
		env := newAdminTestEnv(t, db, registerBasicAdminModels(db))
		env.client.ForceLogin(super)

		resp := env.client.Get("/admin/unknown/")
		djangotest.AssertStatus(t, resp, http.StatusNotFound)
	})
}

// Ported from Django: tests/admin_views/tests.py::AdminSiteFinalCatchAllPatternTests.test_known_url_redirects_login_if_not_authenticated
func TestKnownURLRedirectsLoginIfNotAuthenticated(t *testing.T) {
	djangotest.EachDB(t, adminViewsModels(), func(t *testing.T, db *orm.DB) {
		env := newAdminTestEnv(t, db, registerBasicAdminModels(db))
		changelistURL := mustReverse(t, env.router, "admin:admin_views_article_changelist")
		resp := env.client.Get(changelistURL)
		djangotest.AssertRedirects(t, resp, "/admin/login/?next="+changelistURL)
	})
}

// Ported from Django: tests/admin_views/tests.py::AdminSiteFinalCatchAllPatternTests.test_known_url_missing_slash_redirects_login_if_not_authenticated
func TestKnownURLMissingSlashRedirectsLoginIfNotAuthenticated(t *testing.T) {
	djangotest.EachDB(t, adminViewsModels(), func(t *testing.T, db *orm.DB) {
		env := newAdminTestEnv(t, db, registerBasicAdminModels(db))
		changelistURL := mustReverse(t, env.router, "admin:admin_views_article_changelist")
		noSlash := changelistURL[:len(changelistURL)-1]
		resp := env.client.Get(noSlash)
		djangotest.AssertRedirects(t, resp, "/admin/login/?next="+noSlash)
	})
}

// Ported from Django: tests/admin_views/tests.py::AdminSiteFinalCatchAllPatternTests.test_unknown_url_404_if_not_authenticated_without_final_catch_all_view
func TestUnknownURL404IfNotAuthenticatedWithoutFinalCatchAllView(t *testing.T) {
	djangotest.EachDB(t, adminViewsModels(), func(t *testing.T, db *orm.DB) {
		env := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
			site.FinalCatchAllView = false
			registerBasicAdminModels(db)(site)
		})
		resp := env.client.Get("/admin/unknown/")
		djangotest.AssertStatus(t, resp, http.StatusNotFound)
	})
}

// Ported from Django: tests/admin_views/tests.py::AdminSiteFinalCatchAllPatternTests.test_non_admin_url_shares_url_prefix
func TestNonAdminURLSharesURLPrefix(t *testing.T) {
	djangotest.EachDB(t, adminViewsModels(), func(t *testing.T, db *orm.DB) {
		nonAdmin := urls.PathFunc("admin/non_admin_view/", func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("non-admin-ok"))
		}, "non_admin")
		env := newAdminTestEnv(t, db, registerBasicAdminModels(db), nonAdmin)

		resp := env.client.Get("/admin/non_admin_view/")
		djangotest.AssertStatus(t, resp, http.StatusOK)
		djangotest.AssertContains(t, resp, "non-admin-ok")
	})
}

// TestAdminPasswordChangeFlow verifies changing a staff user's password through
// /admin/password_change/ and landing on /admin/password_change/done/.
func TestAdminPasswordChangeFlow(t *testing.T) {
	djangotest.EachDB(t, adminViewsModels(), func(t *testing.T, db *orm.DB) {
		super := createTestUser(t, db, "super", "oldpass123", true, true)
		env := newAdminTestEnv(t, db, registerBasicAdminModels(db))
		env.client.ForceLogin(super)

		pwChangeURL := mustReverse(t, env.router, "admin:password_change")
		pwDoneURL := mustReverse(t, env.router, "admin:password_change_done")

		resp := env.client.Post(pwChangeURL, url.Values{
			"old_password":  {"oldpass123"},
			"new_password1": {"newstrongpass99"},
			"new_password2": {"newstrongpass99"},
		})
		djangotest.AssertRedirects(t, resp, pwDoneURL)

		done := env.client.Get(pwDoneURL)
		djangotest.AssertStatus(t, done, http.StatusOK)
		djangotest.AssertContains(t, done, "Your password was changed.")

		reloaded, err := orm.Query[auth.User](db).Get(context.Background(), "id", super.ID)
		if err != nil {
			t.Fatalf("reload super: %v", err)
		}
		if !reloaded.CheckPassword("newstrongpass99") {
			t.Fatal("password was not updated in the database")
		}
	})
}

// Ported from Django: tests/admin_views/tests.py::AdminViewBasicTest.test_hide_change_password
func TestHideChangePassword(t *testing.T) {
	djangotest.EachDB(t, adminViewsModels(), func(t *testing.T, db *orm.DB) {
		super := createTestUser(t, db, "super", "secret", true, true)
		if err := super.SetUnusablePassword(); err != nil {
			t.Fatalf("SetUnusablePassword: %v", err)
		}
		if err := orm.Query[auth.User](db).Save(context.Background(), super); err != nil {
			t.Fatalf("Save(super): %v", err)
		}
		env := newAdminTestEnv(t, db, registerBasicAdminModels(db))
		env.client.ForceLogin(super)

		resp := env.client.Get(mustReverse(t, env.router, "admin:index"))
		djangotest.AssertNotContains(t, resp, mustReverse(t, env.router, "admin:password_change"))
	})
}

// customVerboseAppConfig supplies a custom verbose name for admin_views.
type customVerboseAppConfig struct{}

// Name returns the app label.
func (customVerboseAppConfig) Name() string { return "admin_views" }

// VerboseName returns a custom human-readable app title.
func (customVerboseAppConfig) VerboseName() string { return "Editorial Content" }

// TestAppVerboseNameFromAppsRegistry verifies AdminSite.Apps supplies custom
// app verbose names for the admin index and app index pages.
func TestAppVerboseNameFromAppsRegistry(t *testing.T) {
	djangotest.EachDB(t, adminViewsModels(), func(t *testing.T, db *orm.DB) {
		super := createTestUser(t, db, "super", "secret", true, true)
		appReg := apps.NewRegistry()
		if err := appReg.Register(customVerboseAppConfig{}); err != nil {
			t.Fatalf("Register(customVerboseAppConfig): %v", err)
		}
		env := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
			site.Apps = appReg
			admin.Register[Article](site, admin.ModelAdmin{})
		})
		env.client.ForceLogin(super)

		indexResp := env.client.Get(mustReverse(t, env.router, "admin:index"))
		djangotest.AssertContains(t, indexResp, "Editorial Content")

		appResp := env.client.Get(mustReverse(t, env.router, "admin:app_list", "admin_views"))
		djangotest.AssertContains(t, appResp, "<title>Editorial Content administration | Django site admin</title>")
	})
}

// TestLogoutGETMethodNotAllowed verifies that GET /admin/logout/ is rejected
// with 405 Method Not Allowed, matching Django 5+.
func TestLogoutGETMethodNotAllowed(t *testing.T) {
	djangotest.EachDB(t, adminViewsModels(), func(t *testing.T, db *orm.DB) {
		super := createTestUser(t, db, "super", "secret", true, true)
		env := newAdminTestEnv(t, db, registerBasicAdminModels(db))
		env.client.ForceLogin(super)

		resp := env.client.Get(mustReverse(t, env.router, "admin:logout"))
		djangotest.AssertStatus(t, resp, http.StatusMethodNotAllowed)
	})
}
