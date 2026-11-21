package admin_test

import (
	"fmt"
	"net/http"
	"net/url"
	"testing"

	"github.com/oliverhaas/djangogo/admin"
	"github.com/oliverhaas/djangogo/auth"
	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/messages"
	"github.com/oliverhaas/djangogo/orm"
)

// Album stands in for tests/admin_views/models.py::Album.
type Album struct {
	ID    int64
	Owner orm.FK[auth.User] `orm:"null;blank;on_delete=set_null"`
	Title string            `orm:"max_length=30"`
}

// popupEnv returns the admin_views test environment with the album
// registered, logged in as a superuser.
func popupEnv(t *testing.T, db *orm.DB) *adminTestEnv {
	t.Helper()
	super := createTestUser(t, db, "super", "secret", true, true)
	env := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
		registerBasicAdminModels(db)(site)
		admin.Register[Album](site, admin.ModelAdmin{})
	})
	env.client.ForceLogin(super)
	return env
}

// The Go Article has a title, a view count and a published flag in place of
// Django's content and date.
//
// FIXME-PORT: test_popup_add_POST_with_optgroups,
// test_popup_add_POST_with_object_optgroups and
// test_popup_add_POST_with_dynamic_optgroups need choices in option groups,
// which Go form fields do not have.
//
// Ported from Django: tests/admin_views/tests.py::AdminViewBasicTest.test_popup_add_POST
// Ported from Django: tests/admin_views/tests.py::AdminViewBasicTest.test_popup_add_POST_with_valid_source_model
// Ported from Django: tests/admin_views/tests.py::AdminViewBasicTest.test_popup_add_POST_without_optgroups
// Ported from Django: tests/admin_views/tests.py::AdminViewBasicTest.test_popup_add_POST_with_invalid_source_model
// Ported from Django: tests/admin_views/tests.py::AdminViewBasicTest.test_popup_add_POST_with_unregistered_source_model
func TestPopupAddPOST(t *testing.T) {
	djangotest.EachDB(t, adminViewsModels(), func(t *testing.T, db *orm.DB) {
		env := popupEnv(t, db)
		addURL := mustReverse(t, env.router, "admin:admin_views_article_add")
		postData := func(title, sourceModel string) url.Values {
			data := url.Values{"_popup": {"1"}, "title": {title}, "views": {"0"}}
			if sourceModel != "" {
				data.Set("_source_model", sourceModel)
			}
			return data
		}

		t.Run("test_popup_add_POST", func(t *testing.T) {
			// HTTP response from a popup is properly escaped.
			resp := env.client.Post(addURL, postData("title with a new\nline", ""))
			djangotest.AssertContains(t, resp, `title with a new\nline`)
		})

		t.Run("test_popup_add_POST_with_valid_source_model", func(t *testing.T) {
			// Popup add with a valid source_model returns a successful
			// response.
			resp := env.client.Post(addURL, postData("Test Article", "admin_views.section"))
			djangotest.AssertStatus(t, resp, http.StatusOK)
			djangotest.AssertContains(t, resp, "data-popup-response")
			djangotest.AssertMessages(t, resp, []messages.Message{})
		})

		t.Run("test_popup_add_POST_without_optgroups", func(t *testing.T) {
			// Popup add where source_model form exists but doesn't have the
			// field should work without crashing.
			resp := env.client.Post(addURL, postData("Test Article 2", "admin_views.section"))
			djangotest.AssertStatus(t, resp, http.StatusOK)
			djangotest.AssertContains(t, resp, "data-popup-response")
			djangotest.AssertNotContains(t, resp, "&quot;optgroup&quot;")
		})

		t.Run("test_popup_add_POST_with_invalid_source_model", func(t *testing.T) {
			// Popup add with an invalid source_model (non-existent app/model)
			// shows an error message on a subsequent page load instead of
			// crashing.
			for _, invalidModel := range []string{"admin_views.nonexistent", "invalid"} {
				t.Run(invalidModel, func(t *testing.T) {
					popupResponse := env.client.Post(addURL, postData("Test Article", invalidModel))
					djangotest.AssertStatus(t, popupResponse, http.StatusOK)
					djangotest.AssertContains(t, popupResponse, "data-popup-response")
					// The message is visible on the next request.
					resp := env.client.Get(addURL)
					djangotest.AssertMessages(t, resp, []messages.Message{{
						Level:   messages.LevelError,
						Message: fmt.Sprintf(`The app "%s" could not be found.`, invalidModel),
					}})
				})
			}
		})

		t.Run("test_popup_add_POST_with_unregistered_source_model", func(t *testing.T) {
			// Popup add where source_model is a valid model but is not
			// registered in the admin site should succeed. Django's Chapter
			// is only an inline; RowLevelChangePermissionModel has no admin
			// on this site.
			resp := env.client.Post(addURL, postData("Test Article", "admin_views.rowlevelchangepermissionmodel"))
			djangotest.AssertStatus(t, resp, http.StatusOK)
			djangotest.AssertContains(t, resp, "data-popup-response")
			// No error messages - unregistered model is silently skipped.
			djangotest.AssertMessages(t, resp, []messages.Message{})
			// No optgroup in the response.
			djangotest.AssertNotContains(t, resp, "&quot;optgroup&quot;")
		})
	})
}

// Django checks the list of template names the response tries; the Go admin
// renders admin/popup_response.html.
//
// Ported from Django: tests/admin_views/test_actions.py::AdminActionsTest.test_popup_template_response_on_add
// Ported from Django: tests/admin_views/test_actions.py::AdminActionsTest.test_popup_template_response_on_change
// Ported from Django: tests/admin_views/test_actions.py::AdminActionsTest.test_popup_template_response_on_delete
func TestPopupTemplateResponse(t *testing.T) {
	djangotest.EachDB(t, adminViewsModels(), func(t *testing.T, db *orm.DB) {
		env := popupEnv(t, db)

		t.Run("test_popup_template_response_on_add", func(t *testing.T) {
			resp := env.client.Post(mustReverse(t, env.router, "admin:admin_views_actor_add")+"?_popup=1",
				url.Values{"name": {"Troy McClure"}, "age": {"55"}, "_popup": {"1"}})
			djangotest.AssertStatus(t, resp, http.StatusOK)
			djangotest.AssertTemplateUsed(t, resp, "admin/popup_response.html")
		})

		t.Run("test_popup_template_response_on_change", func(t *testing.T) {
			instance := create(t, db, &Actor{Name: "David Tennant", Age: 45})
			resp := env.client.Post(mustReverse(t, env.router, "admin:admin_views_actor_change", instance.ID)+"?_popup=1",
				url.Values{"name": {"David Tennant"}, "age": {"46"}, "_popup": {"1"}})
			djangotest.AssertStatus(t, resp, http.StatusOK)
			djangotest.AssertTemplateUsed(t, resp, "admin/popup_response.html")
		})

		t.Run("test_popup_template_response_on_delete", func(t *testing.T) {
			instance := create(t, db, &Actor{Name: "David Tennant", Age: 45})
			resp := env.client.Post(mustReverse(t, env.router, "admin:admin_views_actor_delete", instance.ID)+"?_popup=1",
				url.Values{"_popup": {"1"}})
			djangotest.AssertStatus(t, resp, http.StatusOK)
			djangotest.AssertTemplateUsed(t, resp, "admin/popup_response.html")
		})
	})
}

// Ported from Django: tests/admin_views/tests.py::UserAdminTest.test_user_fk_delete_popup
func TestUserFKDeletePopup(t *testing.T) {
	djangotest.EachDB(t, adminViewsModels(), func(t *testing.T, db *orm.DB) {
		env := popupEnv(t, db)
		user := createTestUser(t, db, "changeuser", "secret", true, false)

		// User deletion through a FK popup should return the appropriate
		// JavaScript response.
		resp := env.client.Get(mustReverse(t, env.router, "admin:admin_views_album_add"))
		djangotest.AssertContains(t, resp, mustReverse(t, env.router, "admin:auth_user_delete", "__fk__"))
		djangotest.AssertContains(t, resp, `class="related-widget-wrapper-link change-related" id="change_id_owner"`)
		u := mustReverse(t, env.router, "admin:auth_user_delete", user.ID) + "?_popup=1"
		resp = env.client.Get(u)
		djangotest.AssertStatus(t, resp, http.StatusOK)
		data := url.Values{"post": {"yes"}, "_popup": {"1"}}
		resp = env.client.Post(u, data, djangotest.Follow())
		djangotest.AssertContains(t, resp, "&quot;action&quot;: &quot;delete&quot;")
	})
}

// No Django test reads the whole data of the popup responses, nor the change
// form of a popup; the expected values follow Django's response_add,
// response_change and response_delete and the submit_row tag.
func TestPopupResponseData(t *testing.T) {
	djangotest.EachDB(t, adminViewsModels(), func(t *testing.T, db *orm.DB) {
		env := popupEnv(t, db)

		addURL := mustReverse(t, env.router, "admin:admin_views_actor_add")
		resp := env.client.Get(addURL + "?_popup=1&_to_field=id&_source_model=admin_views.album")
		djangotest.AssertContains(t, resp, `<input type="hidden" name="_popup" value="1">`, djangotest.HTML())
		djangotest.AssertContains(t, resp, `<input type="hidden" name="_to_field" value="id">`, djangotest.HTML())
		djangotest.AssertContains(t, resp,
			`<input type="hidden" name="_source_model" value="admin_views.album">`, djangotest.HTML())
		djangotest.AssertContains(t, resp, `name="_save"`)
		djangotest.AssertNotContains(t, resp, `name="_continue"`)
		djangotest.AssertNotContains(t, resp, `name="_addanother"`)

		resp = env.client.Post(addURL, url.Values{"name": {"Palin"}, "age": {"27"}, "_popup": {"1"}, "_to_field": {"id"}})
		actor, err := orm.Query[Actor](db).Get(t.Context(), "name", "Palin")
		if err != nil {
			t.Fatalf("get actor: %v", err)
		}
		djangotest.AssertContains(t, resp, fmt.Sprintf(
			`data-popup-response="{&quot;value&quot;: &quot;%d&quot;, &quot;obj&quot;: &quot;Palin&quot;}"`, actor.ID))

		changeURL := mustReverse(t, env.router, "admin:admin_views_actor_change", actor.ID)
		resp = env.client.Get(changeURL + "?_popup=1")
		djangotest.AssertNotContains(t, resp, `name="_continue"`)
		djangotest.AssertNotContains(t, resp, `class="deletelink"`)
		resp = env.client.Post(changeURL, url.Values{"name": {"Palin “Monty”"}, "age": {"28"}, "_popup": {"1"}})
		djangotest.AssertContains(t, resp, fmt.Sprintf(
			`data-popup-response="{&quot;action&quot;: &quot;change&quot;, &quot;value&quot;: &quot;%d&quot;, `+
				`&quot;obj&quot;: &quot;Palin \u201cMonty\u201d&quot;, &quot;new_value&quot;: &quot;%d&quot;}"`,
			actor.ID, actor.ID))

		deleteURL := mustReverse(t, env.router, "admin:admin_views_actor_delete", actor.ID)
		resp = env.client.Get(deleteURL + "?_popup=1&_to_field=id")
		djangotest.AssertContains(t, resp, `<input type="hidden" name="_popup" value="1">`, djangotest.HTML())
		djangotest.AssertContains(t, resp, `<input type="hidden" name="_to_field" value="id">`, djangotest.HTML())
		resp = env.client.Post(deleteURL, url.Values{"post": {"yes"}, "_popup": {"1"}})
		djangotest.AssertContains(t, resp, fmt.Sprintf(
			`data-popup-response="{&quot;action&quot;: &quot;delete&quot;, &quot;value&quot;: &quot;%d&quot;}"`, actor.ID))
	})
}
