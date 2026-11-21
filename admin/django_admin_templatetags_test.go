package admin_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/oliverhaas/djangogo/admin"
	"github.com/oliverhaas/djangogo/auth"
	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
)

// Ported from Django: tests/admin_views/test_templatetags.py::AdminTemplateTagsTest.test_submit_row_save_as_new_add_permission_required
func TestSubmitRowSaveAsNewAddPermissionRequired(t *testing.T) {
	djangotest.EachDB(t, adminViewsModels(), func(t *testing.T, db *orm.DB) {
		env := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
			ua := admin.UserAdmin(db)
			ua.SaveAs = true
			admin.Register[auth.User](site, ua)
		})
		super := createTestUser(t, db, "super", "secret", true, true)
		changeURL := mustReverse(t, env.router, "admin:auth_user_change", super.ID)

		changeUser := createTestUser(t, db, "change_user", "secret", true, false)
		grantPerm(t, db, changeUser, "auth", "change_user")
		env.client.ForceLogin(changeUser)
		djangotest.AssertEqual(t, ctxVal(t, env.client.Get(changeURL), "show_save_as_new"), any(false))

		addUser := createTestUser(t, db, "add_user", "secret", true, false)
		grantPerm(t, db, addUser, "auth", "add_user")
		grantPerm(t, db, addUser, "auth", "change_user")
		env.client.ForceLogin(addUser)
		djangotest.AssertEqual(t, ctxVal(t, env.client.Get(changeURL), "show_save_as_new"), any(true))
	})
}

// Ported from Django: tests/admin_views/test_templatetags.py::AdminTemplateTagsTest.test_submit_row
func TestSubmitRow(t *testing.T) {
	djangotest.EachDB(t, adminViewsModels(), func(t *testing.T, db *orm.DB) {
		// submit_row template tag should pass whole context.
		env := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
			ua := admin.UserAdmin(db)
			ua.ExtraContext = func(*http.Request, string) map[string]any {
				return map[string]any{"extra": true}
			}
			admin.Register[auth.User](site, ua)
		})
		super := createTestUser(t, db, "super", "secret", true, true)
		env.client.ForceLogin(super)
		resp := env.client.Get(mustReverse(t, env.router, "admin:auth_user_change", super.ID))
		djangotest.AssertEqual(t, ctxVal(t, resp, "extra"), any(true))
		djangotest.AssertEqual(t, ctxVal(t, resp, "show_save"), any(true))
	})
}

// Ported from Django: tests/admin_views/test_templatetags.py::AdminTemplateTagsTest.test_override_show_save_and_add_another
func TestOverrideShowSaveAndAddAnother(t *testing.T) {
	djangotest.EachDB(t, adminViewsModels(), func(t *testing.T, db *orm.DB) {
		var extra map[string]any
		env := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
			ua := admin.UserAdmin(db)
			ua.ExtraContext = func(*http.Request, string) map[string]any { return extra }
			admin.Register[auth.User](site, ua)
		})
		super := createTestUser(t, db, "super", "secret", true, true)
		env.client.ForceLogin(super)
		changeURL := mustReverse(t, env.router, "admin:auth_user_change", super.ID)
		for _, tc := range []struct {
			extra map[string]any
			want  bool
		}{
			{map[string]any{}, true}, // Default.
			{map[string]any{"show_save_and_add_another": false}, false},
		} {
			t.Run(fmt.Sprintf("show_save_and_add_another=%v", tc.want), func(t *testing.T) {
				extra = tc.extra
				resp := env.client.Get(changeURL)
				djangotest.AssertEqual(t, ctxVal(t, resp, "show_save_and_add_another"), any(tc.want))
			})
		}
	})
}
