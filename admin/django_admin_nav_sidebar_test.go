package admin_test

import (
	"testing"

	"github.com/oliverhaas/djangogo/admin"
	"github.com/oliverhaas/djangogo/auth"
	"github.com/oliverhaas/djangogo/contenttypes"
	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
)

// Héllo stands in for tests/admin_views/models.py::Héllo, a model with a
// non-ASCII name.
type Héllo struct {
	ID int64
}

// navSidebarModels returns the models of the ported test_nav_sidebar tests.
func navSidebarModels() []any {
	return []any{
		auth.App{},
		contenttypes.App{},
		djangotest.App("admin_views", &Héllo{}),
	}
}

// navSidebarEnv returns a logged-in superuser's client of Django's
// site_with_sidebar, or of site_without_sidebar when sidebar is false.
func navSidebarEnv(t *testing.T, db *orm.DB, sidebar bool) *adminTestEnv {
	t.Helper()
	super := createTestUser(t, db, "super", "secret", true, true)
	env := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
		if sidebar {
			site.Name = "test_with_sidebar"
			site.Prefix = "/test_sidebar/admin"
		} else {
			site.Name = "test_without_sidebar"
			site.Prefix = "/test_wihout_sidebar/admin"
			site.EnableNavSidebar = false
		}
		admin.Register[auth.User](site, admin.ModelAdmin{})
		admin.Register[Héllo](site, admin.ModelAdmin{})
	})
	env.client.ForceLogin(super)
	return env
}

// navSidebarTag is the opening tag of the sidebar.
const navSidebarTag = `<nav class="sticky" id="nav-sidebar" aria-label="Sidebar">`

// FIXME-PORT: test_sidebar_aria_current_page_missing_without_request_context_processor
// needs a template engine without the request context processor, and the Go
// admin always renders with it. test_included_app_list_template_context_fully_set
// asserts on Django's template debug log, which the Go engine does not write.
// PlaywrightTests run the sidebar's JavaScript in a browser.

// Ported from Django: tests/admin_views/test_nav_sidebar.py::AdminSidebarTests.test_sidebar_not_on_index
func TestSidebarNotOnIndex(t *testing.T) {
	djangotest.EachDB(t, navSidebarModels(), func(t *testing.T, db *orm.DB) {
		env := navSidebarEnv(t, db, true)
		resp := env.client.Get(mustReverse(t, env.router, "test_with_sidebar:index"))
		djangotest.AssertContains(t, resp, `<div class="main" id="main">`)
		djangotest.AssertNotContains(t, resp, navSidebarTag)
	})
}

// Ported from Django: tests/admin_views/test_nav_sidebar.py::AdminSidebarTests.test_sidebar_disabled
func TestSidebarDisabled(t *testing.T) {
	djangotest.EachDB(t, navSidebarModels(), func(t *testing.T, db *orm.DB) {
		env := navSidebarEnv(t, db, false)
		resp := env.client.Get(mustReverse(t, env.router, "test_without_sidebar:index"))
		djangotest.AssertNotContains(t, resp, navSidebarTag)
	})
}

// Ported from Django: tests/admin_views/test_nav_sidebar.py::AdminSidebarTests.test_sidebar_unauthenticated
func TestSidebarUnauthenticated(t *testing.T) {
	djangotest.EachDB(t, navSidebarModels(), func(t *testing.T, db *orm.DB) {
		env := navSidebarEnv(t, db, true)
		env.client.Logout()
		resp := env.client.Get(mustReverse(t, env.router, "test_with_sidebar:login"))
		djangotest.AssertNotContains(t, resp, navSidebarTag)
	})
}

// Ported from Django: tests/admin_views/test_nav_sidebar.py::AdminSidebarTests.test_sidebar_aria_current_page
func TestSidebarAriaCurrentPage(t *testing.T) {
	djangotest.EachDB(t, navSidebarModels(), func(t *testing.T, db *orm.DB) {
		env := navSidebarEnv(t, db, true)
		u := mustReverse(t, env.router, "test_with_sidebar:auth_user_changelist")
		resp := env.client.Get(u)
		djangotest.AssertContains(t, resp, navSidebarTag)
		djangotest.AssertContains(t, resp, `<a href="`+u+`" aria-current="page">Users</a>`)
	})
}

// Ported from Django: tests/admin_views/test_nav_sidebar.py::AdminSidebarTests.test_sidebar_model_name_non_ascii
func TestSidebarModelNameNonASCII(t *testing.T) {
	djangotest.EachDB(t, navSidebarModels(), func(t *testing.T, db *orm.DB) {
		env := navSidebarEnv(t, db, true)
		resp := env.client.Get(mustReverse(t, env.router, "test_with_sidebar:admin_views_héllo_changelist"))
		djangotest.AssertContains(t, resp, `<div class="app-admin_views module current-app">`)
		djangotest.AssertContains(t, resp, `<tr class="model-héllo current-model">`)
		djangotest.AssertContains(t, resp,
			`<th scope="row" id="admin_views-héllo">`+
				`<a href="/test_sidebar/admin/admin_views/h%C3%A9llo/" aria-current="page">`+
				`Héllos</a></th>`,
			djangotest.HTML())
	})
}
