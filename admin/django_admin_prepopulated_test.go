package admin_test

import (
	"net/http"
	"testing"

	"github.com/oliverhaas/djangogo/admin"
	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
)

// PrePopulatedPost stands in for tests/admin_views/models.py::PrePopulatedPost.
type PrePopulatedPost struct {
	ID        int64
	Title     string `orm:"max_length=100"`
	Published bool
	Slug      string `orm:"type=slug"`
}

// PrePopulatedSubPost stands in for
// tests/admin_views/models.py::PrePopulatedSubPost.
type PrePopulatedSubPost struct {
	ID       int64
	Post     orm.FK[PrePopulatedPost] `orm:"on_delete=cascade"`
	Subtitle string                   `orm:"max_length=100"`
	Subslug  string                   `orm:"type=slug"`
}

// PrePopulatedPostLargeSlug stands in for
// tests/admin_views/models.py::PrePopulatedPostLargeSlug.
type PrePopulatedPostLargeSlug struct {
	ID        int64
	Title     string `orm:"max_length=100"`
	Published bool
	Slug      string `orm:"type=slug;max_length=1000"`
}

// published reports whether obj is a published PrePopulatedPost.
func published(obj any) bool {
	p, ok := obj.(*PrePopulatedPost)
	return ok && p != nil && p.Published
}

// subPostInline is tests/admin_views/admin.py::SubPostInline.
func subPostInline() admin.Inline {
	prepopulated := map[string][]string{"subslug": {"subtitle"}}
	return admin.TabularInline[PrePopulatedPost, PrePopulatedSubPost](admin.InlineModelAdmin{
		PrepopulatedFields: prepopulated,
		GetReadonlyFields: func(_ *http.Request, obj any) []string {
			if published(obj) {
				return []string{"subslug"}
			}
			return nil
		},
		GetPrepopulatedFields: func(_ *http.Request, obj any) map[string][]string {
			if published(obj) {
				return nil
			}
			return prepopulated
		},
	})
}

// prePopulatedPostAdmin is tests/admin_views/admin.py::PrePopulatedPostAdmin.
func prePopulatedPostAdmin() admin.ModelAdmin {
	prepopulated := map[string][]string{"slug": {"title"}}
	return admin.ModelAdmin{
		ListDisplay:        []string{"title", "slug"},
		PrepopulatedFields: prepopulated,
		Inlines:            []admin.Inline{subPostInline()},
		GetReadonlyFields: func(_ *http.Request, obj any) []string {
			if published(obj) {
				return []string{"slug"}
			}
			return nil
		},
		GetPrepopulatedFields: func(_ *http.Request, obj any) map[string][]string {
			if published(obj) {
				return nil
			}
			return prepopulated
		},
	}
}

// prePopulatedModels are the models of PrePopulatedTest.
func prePopulatedModels() []any {
	return append(adminViewsModels(), djangotest.App("admin_views",
		&PrePopulatedPost{}, &PrePopulatedSubPost{}, &PrePopulatedPostLargeSlug{}))
}

// prePopulatedEnv builds the site "admin" of PrePopulatedTest, or the site
// "admin7" of tests/admin_views/admin.py with readOnly, and creates p1 and the
// superuser, as PrePopulatedTest.setUpTestData and setUp do.
func prePopulatedEnv(t *testing.T, db *orm.DB, readOnly bool) (*adminTestEnv, *PrePopulatedPost) {
	t.Helper()
	env := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
		if readOnly {
			site.Name = "admin7"
			site.Prefix = "/test_admin/admin7"
			// tests/admin_views/admin.py::PrePopulatedPostReadOnlyAdmin.
			admin.Register[PrePopulatedPost](site, admin.ModelAdmin{
				PrepopulatedFields:  map[string][]string{"slug": {"title"}},
				HasChangePermission: func(*http.Request, any) bool { return false },
			})
			return
		}
		admin.Register[PrePopulatedPost](site, prePopulatedPostAdmin())
		// tests/admin_views/admin.py::PrePopulatedPostLargeSlugAdmin.
		admin.Register[PrePopulatedPostLargeSlug](site, admin.ModelAdmin{
			PrepopulatedFields: map[string][]string{"slug": {"title"}},
		})
	})
	env.client.ForceLogin(createTestUser(t, db, "super", "secret", true, true))
	p1 := mustCreate(t, db, &PrePopulatedPost{Title: "A Long Title", Published: true, Slug: "a-long-title"})
	return env, p1
}

// Ported from Django: tests/admin_views/tests.py::PrePopulatedTest.test_prepopulated_on
func TestPrepopulatedOn(t *testing.T) {
	djangotest.EachDB(t, prePopulatedModels(), func(t *testing.T, db *orm.DB) {
		env, _ := prePopulatedEnv(t, db, false)
		resp := env.client.Get(mustReverse(t, env.router, "admin:admin_views_prepopulatedpost_add"))
		djangotest.AssertContains(t, resp, "&quot;id&quot;: &quot;#id_slug&quot;")
		djangotest.AssertContains(t, resp, "&quot;dependency_ids&quot;: [&quot;#id_title&quot;]")
		djangotest.AssertContains(t, resp, "&quot;id&quot;: &quot;#id_prepopulatedsubpost_set-0-subslug&quot;")
	})
}

// Ported from Django: tests/admin_views/tests.py::PrePopulatedTest.test_prepopulated_off
func TestPrepopulatedOff(t *testing.T) {
	djangotest.EachDB(t, prePopulatedModels(), func(t *testing.T, db *orm.DB) {
		env, p1 := prePopulatedEnv(t, db, false)
		resp := env.client.Get(mustReverse(t, env.router, "admin:admin_views_prepopulatedpost_change", p1.ID))
		djangotest.AssertContains(t, resp, "A Long Title")
		djangotest.AssertNotContains(t, resp, "&quot;id&quot;: &quot;#id_slug&quot;")
		djangotest.AssertNotContains(t, resp, "&quot;dependency_ids&quot;: [&quot;#id_title&quot;]")
		djangotest.AssertNotContains(t, resp, "&quot;id&quot;: &quot;#id_prepopulatedsubpost_set-0-subslug&quot;")
	})
}

// Ported from Django: tests/admin_views/tests.py::PrePopulatedTest.test_prepopulated_maxlength_localized
func TestPrepopulatedMaxlengthLocalized(t *testing.T) {
	djangotest.EachDB(t, prePopulatedModels(), func(t *testing.T, db *orm.DB) {
		// No Go counterpart: Django runs this test with
		// USE_THOUSAND_SEPARATOR=True. Numbers are never localized here.
		// maxLength is rendered without separators.
		env, _ := prePopulatedEnv(t, db, false)
		resp := env.client.Get(mustReverse(t, env.router, "admin:admin_views_prepopulatedpostlargeslug_add"))
		djangotest.AssertContains(t, resp, "&quot;maxLength&quot;: 1000") // instead of 1,000
	})
}

// Ported from Django: tests/admin_views/tests.py::PrePopulatedTest.test_view_only_add_form
func TestPrepopulatedViewOnlyAddForm(t *testing.T) {
	djangotest.EachDB(t, prePopulatedModels(), func(t *testing.T, db *orm.DB) {
		// PrePopulatedPostReadOnlyAdmin.prepopulated_fields includes 'slug'
		// which is present in the add view, even if the
		// ModelAdmin.has_change_permission() returns False.
		env, _ := prePopulatedEnv(t, db, true)
		resp := env.client.Get(mustReverse(t, env.router, "admin7:admin_views_prepopulatedpost_add"))
		djangotest.AssertContains(t, resp, "data-prepopulated-fields=")
		djangotest.AssertContains(t, resp, "&quot;id&quot;: &quot;#id_slug&quot;")
	})
}

// Ported from Django: tests/admin_views/tests.py::PrePopulatedTest.test_view_only_change_form
func TestPrepopulatedViewOnlyChangeForm(t *testing.T) {
	djangotest.EachDB(t, prePopulatedModels(), func(t *testing.T, db *orm.DB) {
		// PrePopulatedPostReadOnlyAdmin.prepopulated_fields includes 'slug'.
		// That doesn't break a view-only change view.
		env, p1 := prePopulatedEnv(t, db, true)
		resp := env.client.Get(mustReverse(t, env.router, "admin7:admin_views_prepopulatedpost_change", p1.ID))
		djangotest.AssertContains(t, resp, `data-prepopulated-fields="[]"`)
		djangotest.AssertContains(t, resp, `<div class="readonly">`+p1.Slug+`</div>`)
	})
}

// FIXME-PORT: tests/admin_views/tests.py::PlaywrightTests.test_prepopulated_fields
// and its siblings run the scripts in a browser.
