package admin_test

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"testing"

	"github.com/oliverhaas/djangogo/admin"
	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/forms"
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

// ParentWithDependentChildren stands in for
// tests/admin_views/models.py::ParentWithDependentChildren: the validation of
// its children depends on the validation of the parent.
type ParentWithDependentChildren struct {
	ID               int64
	SomeRequiredInfo uint32
	FamilyName       string `orm:"max_length=255"`
}

// DependentChild stands in for tests/admin_views/models.py::DependentChild.
type DependentChild struct {
	ID         int64
	Parent     orm.FK[ParentWithDependentChildren]
	FamilyName string `orm:"max_length=255"`
}

// dependentChildrenModels returns the models of the admin_views tests with
// ParentWithDependentChildren and DependentChild.
func dependentChildrenModels() []any {
	return append(adminViewsModels(), djangotest.App("admin_views", &ParentWithDependentChildren{}, &DependentChild{}))
}

// dependentChildrenEnv registers ParentWithDependentChildrenAdmin, whose
// inline uses DependentChildAdminForm, and logs a superuser in.
func dependentChildrenEnv(t *testing.T, db *orm.DB) *adminTestEnv {
	t.Helper()
	env := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
		admin.Register[ParentWithDependentChildren](site, admin.ModelAdmin{Inlines: []admin.Inline{
			admin.TabularInline[ParentWithDependentChildren, DependentChild](admin.InlineModelAdmin{
				FormOptions: []forms.ModelFormOption{forms.WithClean(func(form *forms.Form) error {
					data := form.CleanedData()
					parent, _ := data["parent"].(*ParentWithDependentChildren)
					if parent != nil && parent.FamilyName != "" && parent.FamilyName != data["family_name"] {
						return forms.NewValidationError("Children must share a family name with their parents in this contrived test case", "")
					}
					return nil
				})},
			}),
		}})
	})
	env.client.ForceLogin(createTestUser(t, db, "super", "secret", true, true))
	return env
}

// assertDependentChildrenErrors checks the errors that both tests expect: the
// parent form misses some_required_info and the child form has the non-field
// error of its clean.
func assertDependentChildrenErrors(t *testing.T, resp *djangotest.Response) {
	t.Helper()
	djangotest.AssertStatus(t, resp, http.StatusOK)
	djangotest.AssertFormError(t, ctxVal(t, resp, "form"), "some_required_info", []string{"This field is required."})
	djangotest.AssertFormError(t, ctxVal(t, resp, "form"), "", nil)
	child := inlineFormSet(t, resp, 0)["forms"].([]any)[0]
	djangotest.AssertFormError(t, child, "", []string{"Children must share a family name with their parents in this contrived test case"})
	djangotest.AssertEqual(t, len(nonFormErrors(t, resp)), 0)
}

// Django's response.context["adminform"] and ["inline_admin_formset"] are the
// form and the first inline formset here.
//
// Ported from Django: tests/admin_views/tests.py::AdminViewOnSiteTests.test_add_view_form_and_formsets_run_validation
func TestAddViewFormAndFormsetsRunValidation(t *testing.T) {
	djangotest.EachDB(t, dependentChildrenModels(), func(t *testing.T, db *orm.DB) {
		env := dependentChildrenEnv(t, db)
		// The form validation should fail because 'some_required_info' is
		// not included on the parent form, and the family_name of the parent
		// does not match that of the child
		resp := env.client.Post(mustReverse(t, env.router, "admin:admin_views_parentwithdependentchildren_add"), url.Values{
			"family_name":                      {"Test1"},
			"dependentchild_set-TOTAL_FORMS":   {"1"},
			"dependentchild_set-INITIAL_FORMS": {"0"},
			"dependentchild_set-MAX_NUM_FORMS": {"1"},
			"dependentchild_set-0-id":          {""},
			"dependentchild_set-0-parent":      {""},
			"dependentchild_set-0-family_name": {"Test2"},
		})
		assertDependentChildrenErrors(t, resp)
	})
}

// Ported from Django: tests/admin_views/tests.py::AdminViewOnSiteTests.test_change_view_form_and_formsets_run_validation
func TestChangeViewFormAndFormsetsRunValidation(t *testing.T) {
	djangotest.EachDB(t, dependentChildrenModels(), func(t *testing.T, db *orm.DB) {
		env := dependentChildrenEnv(t, db)
		pwdc := create(t, db, &ParentWithDependentChildren{SomeRequiredInfo: 6, FamilyName: "Test1"})
		// The form validation should fail because 'some_required_info' is
		// not included on the parent form, and the family_name of the parent
		// does not match that of the child
		resp := env.client.Post(mustReverse(t, env.router, "admin:admin_views_parentwithdependentchildren_change", pwdc.ID), url.Values{
			"family_name":                      {"Test2"},
			"dependentchild_set-TOTAL_FORMS":   {"1"},
			"dependentchild_set-INITIAL_FORMS": {"0"},
			"dependentchild_set-MAX_NUM_FORMS": {"1"},
			"dependentchild_set-0-id":          {""},
			"dependentchild_set-0-parent":      {fmt.Sprint(pwdc.ID)},
			"dependentchild_set-0-family_name": {"Test1"},
		})
		assertDependentChildrenErrors(t, resp)
	})
}

// No Django test exists: Django's admin form is a ModelForm, whose unique
// checks the forms tests cover. The test checks that the add and change views
// show a duplicate value of a unique field as a form error.
func TestChangeFormValidatesUnique(t *testing.T) {
	djangotest.EachDB(t, adminFilterModels(), func(t *testing.T, db *orm.DB) {
		env := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
			admin.Register[AFDepartment](site, admin.ModelAdmin{})
		})
		env.client.ForceLogin(createTestUser(t, db, "super", "secret", true, true))
		create(t, db, &AFDepartment{Code: "DEV", Description: "Development"})
		design := create(t, db, &AFDepartment{Code: "DSGN", Description: "Design"})
		want := []string{"Af department with this Code already exists."}

		resp := env.client.Post(mustReverse(t, env.router, "admin:admin_filters_afdepartment_add"),
			url.Values{"code": {"DEV"}, "description": {"Other"}})
		djangotest.AssertStatus(t, resp, http.StatusOK)
		djangotest.AssertFormError(t, ctxVal(t, resp, "form"), "code", want)

		resp = env.client.Post(mustReverse(t, env.router, "admin:admin_filters_afdepartment_change", design.ID),
			url.Values{"code": {"DEV"}, "description": {"Design"}})
		djangotest.AssertStatus(t, resp, http.StatusOK)
		djangotest.AssertFormError(t, ctxVal(t, resp, "form"), "code", want)
		n, err := orm.Query[AFDepartment](db).Filter("code", "DEV").Count(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		djangotest.AssertEqual(t, n, int64(1))
	})
}
