package admin_test

import (
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
