package admin_test

import (
	"net/http"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/admin"
	"github.com/oliverhaas/djangogo/auth"
	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/templates"
)

// CustomArticle stands in for tests/admin_views/models.py::CustomArticle.
type CustomArticle struct {
	ID      int64
	Content string `orm:"type=text"`
	Date    time.Time
}

// customArticleAdmin is tests/admin_views/admin.py::CustomArticleAdmin. Its
// changelist_view passes extra_context={"extra_var": "Hello!"}.
func customArticleAdmin() admin.ModelAdmin {
	return admin.ModelAdmin{
		ExtraContext: func(_ *http.Request, view string) map[string]any {
			if view == "changelist" {
				return map[string]any{"extra_var": "Hello!"}
			}
			return nil
		},
		ChangeListTemplate:                 "custom_admin/change_list.html",
		ChangeFormTemplate:                 "custom_admin/change_form.html",
		AddFormTemplate:                    "custom_admin/add_form.html",
		ObjectHistoryTemplate:              "custom_admin/object_history.html",
		DeleteConfirmationTemplate:         "custom_admin/delete_confirmation.html",
		DeleteSelectedConfirmationTemplate: "custom_admin/delete_selected_confirmation.html",
		PopupResponseTemplate:              "custom_admin/popup_response.html",
	}
}

// adminViewsTemplates are the templates of tests/admin_views/templates that
// these tests use.
var adminViewsTemplates = map[string]string{
	"admin/base_site.html": `{% extends "admin/base.html" %}

{% block bodyclass %}bodyclass_consistency_check{% endblock %}

{% block extrabody %}extrabody_check{% endblock extrabody %}
`,
	"admin/admin_views/article/actions.html": `{% extends "admin/actions.html" %}

{% block actions_submit %}
<button type="submit" class="button override-actions" name="index" value="{{ action_index|default:0 }}">Run</button>
{% endblock %}
`,
	"admin/admin_views/article/change_form_object_tools.html": `{% extends "admin/change_form_object_tools.html" %}

{% block object_tools_items %}
<li><a href="#" id="change-form-export" class="override-change_form_object_tools change-form-object-tools-item">Export</a></li>
{{ block.super }}
{% endblock %}
`,
	"admin/admin_views/article/change_list_object_tools.html": `{% extends "admin/change_list_object_tools.html" %}

{% block object_tools_items %}
<li><a href="#" id="change-list-export" class="override-change_list_object_tools change-list-object-tools-item">Export</a></li>
{{ block.super }}
{% endblock %}
`,
	"admin/admin_views/article/change_list_results.html": `{% if rows %}
<div class="results override-change_list_results">
<table id="result_list">
<tbody>
{% for row in rows %}
<tr>{% for cell in row.cells_html %}{{ cell|safe }}{% endfor %}</tr>
{% endfor %}
</tbody>
</table>
</div>
{% endif %}
`,
	"admin/admin_views/article/pagination.html": `<p class="paginator override-pagination">
{{ result_count }} {{ result_count_label }}
</p>
`,
	"admin/admin_views/article/prepopulated_fields_js.html": `<script id="django-admin-prepopulated-fields-constants"
        class="override-prepopulated_fields_js"
        src="{% static "admin/js/prepopulate_init.js" %}"
        data-prepopulated-fields="{{ prepopulated_fields_json }}">
</script>
`,
	"admin/admin_views/article/search_form.html": `{% if has_search %}
<div id="toolbar" class="override-search_form"><form id="changelist-search" method="get">
<div>
<input type="text" size="40" name="{{ search_var }}" value="{{ search_query }}" id="searchbar" autofocus>
<input type="submit" value="Search">
</div>
</form></div>
{% endif %}
`,
	"admin/admin_views/article/submit_line.html": `{% extends "admin/submit_line.html" %}

{% block submit_row %}
{% if show_publish %}<input type="submit" value="Publish" class="default" name="_publish">{% endif %}
{{ block.super }}
{% endblock %}
`,
}

// customAdminTemplates are the templates of tests/templates/custom_admin.
var customAdminTemplates = map[string]string{
	"custom_admin/change_list.html": `{% extends "admin/change_list.html" %}

{% block extrahead %}
<script>
var hello = '{{ extra_var }}';
</script>
{% endblock %}
`,
	"custom_admin/add_form.html":                     `{% extends "admin/change_form.html" %}`,
	"custom_admin/change_form.html":                  `{% extends "admin/change_form.html" %}`,
	"custom_admin/delete_confirmation.html":          `{% extends "admin/delete_confirmation.html" %}`,
	"custom_admin/delete_selected_confirmation.html": `{% extends "admin/delete_selected_confirmation.html" %}`,
	"custom_admin/object_history.html":               `{% extends "admin/object_history.html" %}`,
	"custom_admin/popup_response.html":               `{% extends "admin/popup_response.html" %}`,
	"custom_admin/index.html": `{% extends "admin/index.html" %}

{% block content %}
Hello from a custom index template {{ foo }}
{{ block.super }}
{% endblock %}
`,
	"custom_admin/app_index.html": `{% extends "admin/app_index.html" %}

{% block content %}
Hello from a custom app_index template
{{ block.super }}
{% endblock %}
`,
	"custom_admin/login.html": `{% extends "admin/login.html" %}

{% block content %}
Hello from a custom login template
{{ block.super }}
{% endblock %}
`,
	"custom_admin/logout.html": `{% extends "registration/logged_out.html" %}

{% block content %}
Hello from a custom logout template
{{ block.super }}
{% endblock %}
`,
	"custom_admin/password_change_form.html": `{% extends "registration/password_change_form.html" %}

{% block content %}
{{ spam }}
Hello from a custom password change form template
{{ block.super }}
{% endblock %}
`,
	"custom_admin/password_change_done.html": `{% extends "registration/password_change_done.html" %}

{% block content %}
Hello from a custom password change done template
{{ block.super }}
{% endblock %}
`,
}

// customTemplateModels are the models of these tests.
func customTemplateModels() []any {
	return append(adminViewsModels(), djangotest.App("admin_views", &CustomArticle{}))
}

// customTemplateEnv builds the site of AdminCustomTemplateTests: the
// templates of tests/admin_views/templates and tests/templates in front of
// the admin's, as its TEMPLATES setting puts them in DIRS. It registers
// Section, Article, CustomArticle, Group and User, and logs in the
// superuser, as AdminViewBasicTestCase.setUpTestData and setUp do.
func customTemplateEnv(t *testing.T, db *orm.DB) (*adminTestEnv, *auth.User) {
	t.Helper()
	env := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
		site.SetTemplateSources(
			templates.MapSource("admin_views", adminViewsTemplates),
			templates.MapSource("tests", customAdminTemplates),
			templates.FSSource("admin", admin.Templates()),
		)
		admin.Register[Section](site, admin.ModelAdmin{})
		admin.Register[Article](site, admin.ModelAdmin{})
		admin.Register[CustomArticle](site, customArticleAdmin())
		admin.Register[auth.Group](site, admin.GroupAdmin())
		admin.Register[auth.User](site, admin.UserAdmin(db))
	})
	super := createTestUser(t, db, "super", "secret", true, true)
	env.client.ForceLogin(super)
	return env, super
}

// Ported from Django: tests/admin_views/tests.py::AdminCustomTemplateTests.test_custom_model_admin_templates
func TestCustomModelAdminTemplates(t *testing.T) {
	djangotest.EachDB(t, customTemplateModels(), func(t *testing.T, db *orm.DB) {
		env, _ := customTemplateEnv(t, db)
		// Test custom change list template.
		resp := env.client.Get(mustReverse(t, env.router, "admin:admin_views_customarticle_changelist"))
		djangotest.AssertContains(t, resp, "var hello = 'Hello!';")
		djangotest.AssertTemplateUsed(t, resp, "custom_admin/change_list.html")

		// Test custom add form template.
		addURL := mustReverse(t, env.router, "admin:admin_views_customarticle_add")
		resp = env.client.Get(addURL)
		djangotest.AssertTemplateUsed(t, resp, "custom_admin/add_form.html")

		// Add an article so we can test delete, change, and history views.
		post := env.client.Post(addURL, url.Values{
			"content": {"<p>great article</p>"},
			"date_0":  {"2008-03-18"},
			"date_1":  {"10:54:39"},
		})
		djangotest.AssertRedirects(t, post, mustReverse(t, env.router, "admin:admin_views_customarticle_changelist"))
		articles, err := orm.Query[CustomArticle](db).All(t.Context())
		if err != nil {
			t.Fatalf("All: %v", err)
		}
		if len(articles) != 1 {
			t.Fatalf("CustomArticle count = %d, want 1", len(articles))
		}
		articlePK := articles[0].ID

		// Test custom delete, change, and object history templates.
		// Test custom change form template.
		resp = env.client.Get(mustReverse(t, env.router, "admin:admin_views_customarticle_change", articlePK))
		djangotest.AssertTemplateUsed(t, resp, "custom_admin/change_form.html")
		resp = env.client.Get(mustReverse(t, env.router, "admin:admin_views_customarticle_delete", articlePK))
		djangotest.AssertTemplateUsed(t, resp, "custom_admin/delete_confirmation.html")
		resp = env.client.Post(mustReverse(t, env.router, "admin:admin_views_customarticle_changelist"), url.Values{
			"index":            {"0"},
			"action":           {"delete_selected"},
			"_selected_action": {strconv.FormatInt(articlePK, 10)},
		})
		djangotest.AssertTemplateUsed(t, resp, "custom_admin/delete_selected_confirmation.html")
		resp = env.client.Get(mustReverse(t, env.router, "admin:admin_views_customarticle_history", articlePK))
		djangotest.AssertTemplateUsed(t, resp, "custom_admin/object_history.html")

		// A custom popup response template may be specified by
		// ModelAdmin.popup_response_template.
		resp = env.client.Post(addURL+"?_popup=1", url.Values{
			"content": {"<p>great article</p>"},
			"date_0":  {"2008-03-18"},
			"date_1":  {"10:54:39"},
			"_popup":  {"1"},
		})
		djangotest.AssertTemplateUsed(t, resp, "custom_admin/popup_response.html")
	})
}

// Ported from Django: tests/admin_views/tests.py::AdminCustomTemplateTests.test_extended_bodyclass_template_change_form
func TestExtendedBodyclassTemplateChangeForm(t *testing.T) {
	djangotest.EachDB(t, customTemplateModels(), func(t *testing.T, db *orm.DB) {
		env, _ := customTemplateEnv(t, db)
		// The admin/change_form.html template uses block.super in the
		// bodyclass block.
		resp := env.client.Get(mustReverse(t, env.router, "admin:admin_views_section_add"))
		djangotest.AssertContains(t, resp, "bodyclass_consistency_check ")
	})
}

// Ported from Django: tests/admin_views/tests.py::AdminCustomTemplateTests.test_extended_extrabody
func TestExtendedExtrabody(t *testing.T) {
	djangotest.EachDB(t, customTemplateModels(), func(t *testing.T, db *orm.DB) {
		env, _ := customTemplateEnv(t, db)
		resp := env.client.Get(mustReverse(t, env.router, "admin:admin_views_section_add"))
		djangotest.AssertContains(t, resp, "extrabody_check\n</body>")
	})
}

// Ported from Django: tests/admin_views/tests.py::AdminCustomTemplateTests.test_change_password_template
func TestChangePasswordTemplate(t *testing.T) {
	cfg := auth.CurrentConfig()
	cfg.PasswordValidators = []auth.PasswordValidator{
		auth.UserAttributeSimilarityValidator{},
		auth.NumericPasswordValidator{},
	}
	t.Cleanup(auth.Configure(cfg))
	djangotest.EachDB(t, customTemplateModels(), func(t *testing.T, db *orm.DB) {
		env, user := customTemplateEnv(t, db)
		resp := env.client.Get(mustReverse(t, env.router, "admin:auth_user_password_change", user.ID))
		// The auth/user/change_password.html template uses super in the
		// bodyclass block.
		djangotest.AssertContains(t, resp, "bodyclass_consistency_check ")

		// When a site has multiple passwords in the browser's password
		// manager, a browser pop up asks which user the new password is for.
		// To prevent this, the username is added to the change password form.
		djangotest.AssertContains(t, resp, `<input type="text" name="username" value="super" class="hidden">`)

		// help text for passwords has an id.
		djangotest.AssertContains(t, resp,
			`<div class="help" id="id_password1_helptext"><ul><li>`+
				"Your password can’t be too similar to your other personal information."+
				"</li><li>Your password can’t be entirely numeric.</li></ul></div>")
		djangotest.AssertContains(t, resp,
			`<div class="help" id="id_password2_helptext">`+
				"Enter the same password as before, for verification.</div>")
	})
}

// Ported from Django: tests/admin_views/tests.py::AdminCustomTemplateTests.test_extended_bodyclass_template_index
func TestExtendedBodyclassTemplateIndex(t *testing.T) {
	djangotest.EachDB(t, customTemplateModels(), func(t *testing.T, db *orm.DB) {
		env, _ := customTemplateEnv(t, db)
		// The admin/index.html template uses block.super in the bodyclass
		// block.
		resp := env.client.Get(mustReverse(t, env.router, "admin:index"))
		djangotest.AssertContains(t, resp, "bodyclass_consistency_check ")
	})
}

// Ported from Django: tests/admin_views/tests.py::AdminCustomTemplateTests.test_extended_bodyclass_change_list
func TestExtendedBodyclassChangeList(t *testing.T) {
	djangotest.EachDB(t, customTemplateModels(), func(t *testing.T, db *orm.DB) {
		env, _ := customTemplateEnv(t, db)
		// The admin/change_list.html' template uses block.super in the
		// bodyclass block.
		resp := env.client.Get(mustReverse(t, env.router, "admin:admin_views_article_changelist"))
		djangotest.AssertContains(t, resp, "bodyclass_consistency_check ")
	})
}

// Ported from Django: tests/admin_views/tests.py::AdminCustomTemplateTests.test_extended_bodyclass_template_login
func TestExtendedBodyclassTemplateLogin(t *testing.T) {
	djangotest.EachDB(t, customTemplateModels(), func(t *testing.T, db *orm.DB) {
		env, _ := customTemplateEnv(t, db)
		// The admin/login.html template uses block.super in the bodyclass
		// block.
		env.client.Logout()
		resp := env.client.Get(mustReverse(t, env.router, "admin:login"))
		djangotest.AssertContains(t, resp, "bodyclass_consistency_check ")
	})
}

// Ported from Django: tests/admin_views/tests.py::AdminCustomTemplateTests.test_extended_bodyclass_template_delete_confirmation
func TestExtendedBodyclassTemplateDeleteConfirmation(t *testing.T) {
	djangotest.EachDB(t, customTemplateModels(), func(t *testing.T, db *orm.DB) {
		env, _ := customTemplateEnv(t, db)
		// The admin/delete_confirmation.html template uses block.super in
		// the bodyclass block.
		group := mustCreate(t, db, &auth.Group{Name: "foogroup"})
		resp := env.client.Get(mustReverse(t, env.router, "admin:auth_group_delete", group.ID))
		djangotest.AssertContains(t, resp, "bodyclass_consistency_check ")
	})
}

// Ported from Django: tests/admin_views/tests.py::AdminCustomTemplateTests.test_extended_bodyclass_template_delete_selected_confirmation
func TestExtendedBodyclassTemplateDeleteSelectedConfirmation(t *testing.T) {
	djangotest.EachDB(t, customTemplateModels(), func(t *testing.T, db *orm.DB) {
		env, _ := customTemplateEnv(t, db)
		// The admin/delete_selected_confirmation.html template uses
		// block.super in bodyclass block.
		group := mustCreate(t, db, &auth.Group{Name: "foogroup"})
		resp := env.client.Post(mustReverse(t, env.router, "admin:auth_group_changelist"), url.Values{
			"action":           {"delete_selected"},
			"selected_across":  {"0"},
			"index":            {"0"},
			"_selected_action": {strconv.FormatInt(group.ID, 10)},
		})
		if got := ctxVal(t, resp, "site_header"); got != "Django administration" {
			t.Errorf("site_header = %v, want Django administration", got)
		}
		djangotest.AssertContains(t, resp, "bodyclass_consistency_check ")
	})
}

// customAdminSiteEnv builds the site "admin2" of
// tests/admin_views/customadmin.py: the custom templates of Admin2 and the
// templates of tests/templates in front of the admin's. Admin2.index and
// Admin2.password_change pass the extra_context {"foo": "*bar*"} and
// {"spam": "eggs"}.
func customAdminSiteEnv(t *testing.T, db *orm.DB) (*adminTestEnv, *auth.User) {
	t.Helper()
	env := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
		site.Name = "admin2"
		site.Prefix = "/test_admin/admin2"
		site.AppIndexTemplate = "custom_admin/app_index.html"
		site.LoginTemplate = "custom_admin/login.html"
		site.LogoutTemplate = "custom_admin/logout.html"
		site.IndexTemplate = "custom_admin/index.html"
		site.PasswordChangeTemplate = "custom_admin/password_change_form.html"
		site.PasswordChangeDoneTemplate = "custom_admin/password_change_done.html"
		site.ExtraContext = func(_ *http.Request, view string) map[string]any {
			switch view {
			case "index":
				return map[string]any{"foo": "*bar*"}
			case "password_change":
				return map[string]any{"spam": "eggs"}
			}
			return nil
		}
		site.SetTemplateSources(
			templates.MapSource("tests", customAdminTemplates),
			templates.FSSource("admin", admin.Templates()),
		)
		admin.Register[Article](site, admin.ModelAdmin{})
	})
	super := createTestUser(t, db, "super", "secret", true, true)
	env.client.ForceLogin(super)
	return env, super
}

// Ported from Django: tests/admin_views/tests.py::CustomModelAdminTest.test_custom_admin_site_login_template
func TestCustomAdminSiteLoginTemplate(t *testing.T) {
	djangotest.EachDB(t, adminViewsModels(), func(t *testing.T, db *orm.DB) {
		env, _ := customAdminSiteEnv(t, db)
		env.client.Logout()
		resp := env.client.Get(mustReverse(t, env.router, "admin2:index"), djangotest.Follow())
		djangotest.AssertTemplateUsed(t, resp, "custom_admin/login.html")
		djangotest.AssertContains(t, resp, "Hello from a custom login template")
	})
}

// Ported from Django: tests/admin_views/tests.py::CustomModelAdminTest.test_custom_admin_site_logout_template
func TestCustomAdminSiteLogoutTemplate(t *testing.T) {
	djangotest.EachDB(t, adminViewsModels(), func(t *testing.T, db *orm.DB) {
		env, _ := customAdminSiteEnv(t, db)
		resp := env.client.Post(mustReverse(t, env.router, "admin2:logout"), nil)
		djangotest.AssertTemplateUsed(t, resp, "custom_admin/logout.html")
		djangotest.AssertContains(t, resp, "Hello from a custom logout template")
	})
}

// Ported from Django: tests/admin_views/tests.py::CustomModelAdminTest.test_custom_admin_site_index_view_and_template
func TestCustomAdminSiteIndexViewAndTemplate(t *testing.T) {
	djangotest.EachDB(t, adminViewsModels(), func(t *testing.T, db *orm.DB) {
		env, _ := customAdminSiteEnv(t, db)
		resp := env.client.Get(mustReverse(t, env.router, "admin2:index"))
		djangotest.AssertTemplateUsed(t, resp, "custom_admin/index.html")
		djangotest.AssertContains(t, resp, "Hello from a custom index template *bar*")
	})
}

// Ported from Django: tests/admin_views/tests.py::CustomModelAdminTest.test_custom_admin_site_app_index_view_and_template
func TestCustomAdminSiteAppIndexViewAndTemplate(t *testing.T) {
	djangotest.EachDB(t, adminViewsModels(), func(t *testing.T, db *orm.DB) {
		env, _ := customAdminSiteEnv(t, db)
		resp := env.client.Get(mustReverse(t, env.router, "admin2:app_list", "admin_views"))
		djangotest.AssertTemplateUsed(t, resp, "custom_admin/app_index.html")
		djangotest.AssertContains(t, resp, "Hello from a custom app_index template")
	})
}

// Ported from Django: tests/admin_views/tests.py::CustomModelAdminTest.test_custom_admin_site_password_change_template
func TestCustomAdminSitePasswordChangeTemplate(t *testing.T) {
	djangotest.EachDB(t, adminViewsModels(), func(t *testing.T, db *orm.DB) {
		env, _ := customAdminSiteEnv(t, db)
		resp := env.client.Get(mustReverse(t, env.router, "admin2:password_change"))
		djangotest.AssertTemplateUsed(t, resp, "custom_admin/password_change_form.html")
		djangotest.AssertContains(t, resp, "Hello from a custom password change form template")
	})
}

// Ported from Django: tests/admin_views/tests.py::CustomModelAdminTest.test_custom_admin_site_password_change_with_extra_context
func TestCustomAdminSitePasswordChangeWithExtraContext(t *testing.T) {
	djangotest.EachDB(t, adminViewsModels(), func(t *testing.T, db *orm.DB) {
		env, _ := customAdminSiteEnv(t, db)
		resp := env.client.Get(mustReverse(t, env.router, "admin2:password_change"))
		djangotest.AssertTemplateUsed(t, resp, "custom_admin/password_change_form.html")
		djangotest.AssertContains(t, resp, "eggs")
	})
}

// Ported from Django: tests/admin_views/tests.py::CustomModelAdminTest.test_custom_admin_site_password_change_done_template
func TestCustomAdminSitePasswordChangeDoneTemplate(t *testing.T) {
	djangotest.EachDB(t, adminViewsModels(), func(t *testing.T, db *orm.DB) {
		env, _ := customAdminSiteEnv(t, db)
		resp := env.client.Get(mustReverse(t, env.router, "admin2:password_change_done"))
		djangotest.AssertTemplateUsed(t, resp, "custom_admin/password_change_done.html")
		djangotest.AssertContains(t, resp, "Hello from a custom password change done template")
	})
}

// templateTagsEnv builds the site of AdminTemplateTagsTest. Its settings keep
// Django's default TEMPLATES of the test runner: tests/templates in DIRS, then
// the admin's templates, then those of the admin_views app.
func templateTagsEnv(t *testing.T, db *orm.DB, articleAdmin admin.ModelAdmin) *adminTestEnv {
	t.Helper()
	env := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
		site.SetTemplateSources(
			templates.MapSource("tests", customAdminTemplates),
			templates.FSSource("admin", admin.Templates()),
			templates.MapSource("admin_views", adminViewsTemplates),
		)
		admin.Register[Article](site, articleAdmin)
	})
	env.client.ForceLogin(createTestUser(t, db, "super", "secret", true, true))
	return env
}

// Ported from Django: tests/admin_views/test_templatetags.py::AdminTemplateTagsTest.test_override_change_form_template_tags
func TestOverrideChangeFormTemplateTags(t *testing.T) {
	djangotest.EachDB(t, adminViewsModels(), func(t *testing.T, db *orm.DB) {
		// admin_modify template tags follow the standard search pattern
		// admin/app_label/model/template.html.
		env := templateTagsEnv(t, db, admin.ModelAdmin{
			ExtraContext: func(_ *http.Request, view string) map[string]any {
				if view == "change" {
					return map[string]any{"show_publish": true, "extra": true}
				}
				return nil
			},
		})
		article := mustCreate(t, db, &Article{Title: "Article"})
		resp := env.client.Get(mustReverse(t, env.router, "admin:admin_views_article_change", article.ID))
		djangotest.AssertStatus(t, resp, http.StatusOK)
		djangotest.AssertEqual(t, ctxVal(t, resp, "show_publish"), any(true))
		djangotest.AssertEqual(t, ctxVal(t, resp, "extra"), any(true))
		djangotest.AssertContains(t, resp, `name="_save"`)
		djangotest.AssertContains(t, resp, `name="_publish"`)
		djangotest.AssertContains(t, resp, "override-change_form_object_tools")
		djangotest.AssertContains(t, resp, "override-prepopulated_fields_js")
	})
}

// Ported from Django: tests/admin_views/test_templatetags.py::AdminTemplateTagsTest.test_override_change_list_template_tags
func TestOverrideChangeListTemplateTags(t *testing.T) {
	djangotest.EachDB(t, adminViewsModels(), func(t *testing.T, db *orm.DB) {
		// admin_list template tags follow the standard search pattern
		// admin/app_label/model/template.html.
		// FIXME-PORT: Django also sets date_hierarchy = "date" and asserts
		// "override-date_hierarchy"; the Go Article has no date field.
		env := templateTagsEnv(t, db, admin.ModelAdmin{SearchFields: []string{"title"}})
		mustCreate(t, db, &Article{Title: "Article"})
		resp := env.client.Get(mustReverse(t, env.router, "admin:admin_views_article_changelist"))
		djangotest.AssertContains(t, resp, "override-actions")
		djangotest.AssertContains(t, resp, "override-change_list_object_tools")
		djangotest.AssertContains(t, resp, "override-change_list_results")
		djangotest.AssertContains(t, resp, "override-pagination")
		djangotest.AssertContains(t, resp, "override-search_form")
	})
}
