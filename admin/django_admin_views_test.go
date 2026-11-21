package admin_test

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"testing"

	"github.com/oliverhaas/djangogo/admin"
	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
)

// A relation always targets a primary key in the Go ORM, so the primary key
// is the one field that _to_field may name.
//
// FIXME-PORT: the cases of fields that a many-to-many field, a parent model,
// an inline or a generic relation references need relations to fields other
// than the primary key, and generic relations.
//
// Ported from Django: tests/admin_views/tests.py::AdminViewBasicTest.test_disallowed_to_field
func TestDisallowedToField(t *testing.T) {
	djangotest.EachDB(t, adminViewsModels(), func(t *testing.T, db *orm.DB) {
		super := createTestUser(t, db, "super", "secret", true, true)
		env := newAdminTestEnv(t, db, registerBasicAdminModels)
		env.client.ForceLogin(super)

		u := mustReverse(t, env.router, "admin:admin_views_section_changelist")
		resp := env.client.Get(u, djangotest.Query(url.Values{"_to_field": {"missing_field"}}))
		djangotest.AssertStatus(t, resp, http.StatusBadRequest)

		// Specifying a field that is not referred by any other model registered
		// to this admin site should raise an exception.
		resp = env.client.Get(u, djangotest.Query(url.Values{"_to_field": {"name"}}))
		djangotest.AssertStatus(t, resp, http.StatusBadRequest)

		// Primary key should always be allowed, even if the referenced model
		// isn't registered.
		resp = env.client.Get(u, djangotest.Query(url.Values{"_to_field": {"id"}}))
		djangotest.AssertStatus(t, resp, http.StatusOK)

		// We also want to prevent the add, change, and delete views from
		// leaking a disallowed field value.
		resp = env.client.Post(mustReverse(t, env.router, "admin:admin_views_section_add"),
			url.Values{"_to_field": {"name"}})
		djangotest.AssertStatus(t, resp, http.StatusBadRequest)

		section := &Section{}
		if err := orm.Query[Section](db).Create(context.Background(), section); err != nil {
			t.Fatalf("create section: %v", err)
		}
		resp = env.client.Post(mustReverse(t, env.router, "admin:admin_views_section_change", section.ID),
			url.Values{"_to_field": {"name"}})
		djangotest.AssertStatus(t, resp, http.StatusBadRequest)

		resp = env.client.Post(mustReverse(t, env.router, "admin:admin_views_section_delete", section.ID),
			url.Values{"_to_field": {"name"}})
		djangotest.AssertStatus(t, resp, http.StatusBadRequest)
	})
}

// Ported from Django: tests/admin_views/tests.py::AdminViewBasicTest.test_popup_dismiss_related
func TestPopupDismissRelated(t *testing.T) {
	djangotest.EachDB(t, adminViewsModels(), func(t *testing.T, db *orm.DB) {
		super := createTestUser(t, db, "super", "secret", true, true)
		env := newAdminTestEnv(t, db, registerBasicAdminModels)
		env.client.ForceLogin(super)

		actor := &Actor{Name: "Palin", Age: 27}
		if err := orm.Query[Actor](db).Create(context.Background(), actor); err != nil {
			t.Fatalf("create actor: %v", err)
		}
		resp := env.client.Get(mustReverse(t, env.router, "admin:admin_views_actor_changelist") + "?_popup")
		djangotest.AssertContains(t, resp, fmt.Sprintf(`data-popup-opener="%d"`, actor.ID))
	})
}

// Django's override_settings(DEBUG=...) maps to AdminSite.Debug, which the
// application sets from Settings.Debug. Django's SectionAdmin has an inline of
// the articles of a section; the Article of this package has no foreign key
// to Section, so SectionedPost stands in for it.
//
// Ported from Django: tests/admin_views/tests.py::AdminJavaScriptTest.test_js_minified_only_if_debug_is_false
func TestJSMinifiedOnlyIfDebugIsFalse(t *testing.T) {
	models := append(adminViewsModels(), djangotest.App("blog", &SectionedPost{}))
	djangotest.EachDB(t, models, func(t *testing.T, db *orm.DB) {
		super := createTestUser(t, db, "super", "secret", true, true)
		for _, debug := range []bool{false, true} {
			env := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
				site.Debug = debug
				admin.Register[Section](site, admin.ModelAdmin{Inlines: []admin.Inline{
					admin.TabularInline[Section, SectionedPost](admin.InlineModelAdmin{}),
				}})
			})
			env.client.ForceLogin(super)
			resp := env.client.Get(mustReverse(t, env.router, "admin:admin_views_section_add"))
			if debug {
				djangotest.AssertContains(t, resp, "vendor/jquery/jquery.js")
				djangotest.AssertNotContains(t, resp, "vendor/jquery/jquery.min.js")
			} else {
				djangotest.AssertNotContains(t, resp, "vendor/jquery/jquery.js")
				djangotest.AssertContains(t, resp, "vendor/jquery/jquery.min.js")
			}
			djangotest.AssertContains(t, resp, "prepopulate.js")
			djangotest.AssertContains(t, resp, "actions.js")
			djangotest.AssertContains(t, resp, "inlines.js")
		}
	})
}

// The Go admin has no translations, so its catalog always holds the English
// formats.
//
// FIXME-PORT: test_jsi18n_with_context needs a site whose views take an extra
// context, which the Go admin does not have.
//
// Ported from Django: tests/admin_views/tests.py::AdminViewBasicTest.test_jsi18n_format_fallback
func TestJSI18nFormatFallback(t *testing.T) {
	djangotest.EachDB(t, adminViewsModels(), func(t *testing.T, db *orm.DB) {
		super := createTestUser(t, db, "super", "secret", true, true)
		env := newAdminTestEnv(t, db, registerBasicAdminModels)
		env.client.ForceLogin(super)

		resp := env.client.Get(mustReverse(t, env.router, "admin:jsi18n"))
		djangotest.AssertNotContains(t, resp, "%d.%m.%Y %H:%M:%S")
		djangotest.AssertContains(t, resp, "%Y-%m-%d %H:%M:%S")
	})
}
