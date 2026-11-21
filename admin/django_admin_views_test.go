package admin_test

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
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
		env := newAdminTestEnv(t, db, registerBasicAdminModels(db))
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
		env := newAdminTestEnv(t, db, registerBasicAdminModels(db))
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
		env := newAdminTestEnv(t, db, registerBasicAdminModels(db))
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

// Person stands in for tests/admin_views/models.py::Person.
type Person struct {
	ID     int64
	Name   string `orm:"max_length=100"`
	Gender int64  `orm:"verbose_name=very very very very very very very very very loooooooooooooooooooooooooooooooooooooooooong name"`
	Age    int64  `orm:"default=21"`
	Alive  bool   `orm:"default=true"`
}

// Meta declares the choices of Gender.
func (Person) Meta() orm.Meta {
	return orm.Meta{Choices: map[string][]orm.Choice{
		"Gender": {orm.ChoiceOf(int64(1), "Male"), orm.ChoiceOf(int64(2), "Female")},
	}}
}

// String returns the name.
func (p Person) String() string { return p.Name }

// Parent stands in for tests/admin_views/models.py::Parent.
type Parent struct {
	ID   int64
	Name string `orm:"max_length=128"`
}

// Clean rejects the name "_invalid".
func (p Parent) Clean() error {
	if p.Name == "_invalid" {
		return orm.NewValidationError("invalid", "")
	}
	return nil
}

// Child stands in for tests/admin_views/models.py::Child.
type Child struct {
	ID     int64
	Parent orm.FK[Parent] `orm:"on_delete=cascade;editable=false"`
	Name   string         `orm:"max_length=30;blank"`
}

// Clean rejects the name "_invalid".
func (c Child) Clean() error {
	if c.Name == "_invalid" {
		return orm.NewValidationError("invalid", "")
	}
	return nil
}

// saveAsModels returns the models of SaveAsTests.
func saveAsModels() []any {
	return append(adminViewsModels(), djangotest.App("admin_views", &Person{}, &Parent{}, &Child{}))
}

// parentAdmin returns ParentAdmin of tests/admin_views/admin.py.
func parentAdmin() admin.ModelAdmin {
	return admin.ModelAdmin{
		SaveAs:           true,
		Inlines:          []admin.Inline{admin.StackedInline[Parent, Child](admin.InlineModelAdmin{})},
		ListDisplay:      []string{"id", "name"},
		ListDisplayLinks: []string{"id"},
		ListEditable:     []string{"name"},
		// Give each child with one name the last name of the parent.
		SaveRelated: func(r *http.Request, sc *admin.SaveContext) error {
			ctx := r.Context()
			if err := sc.SaveRelated(ctx); err != nil {
				return err
			}
			parent := sc.Obj.(*Parent)
			names := strings.Fields(parent.Name)
			if len(names) != 2 {
				return fmt.Errorf("the name %q is not a first and a last name", parent.Name)
			}
			children, err := orm.Query[Child](sc.DB).Filter("parent", parent.ID).All(ctx)
			if err != nil {
				return err
			}
			for i := range children {
				if len(strings.Fields(children[i].Name)) < 2 {
					children[i].Name += " " + names[1]
					if err := orm.Query[Child](sc.DB).Save(ctx, &children[i]); err != nil {
						return err
					}
				}
			}
			return nil
		},
	}
}

// saveAsEnv registers PersonAdmin and ParentAdmin of tests/admin_views/admin.py
// on the site "admin" and Person with save_as_continue=False on the site
// "namespaced_admin", Django's site2, creates per1 and logs in the
// superuser, as SaveAsTests.setUpTestData and setUp do.
func saveAsEnv(t *testing.T, db *orm.DB) (env, env2 *adminTestEnv, per1 *Person) {
	t.Helper()
	env = newAdminTestEnv(t, db, func(site *admin.AdminSite) {
		admin.Register[Person](site, personAdmin())
		admin.Register[Parent](site, parentAdmin())
	})
	continueFalse := false
	env2 = newAdminTestEnv(t, db, func(site *admin.AdminSite) {
		site.Name = "namespaced_admin"
		site.Prefix = "/test_admin/admin2"
		admin.Register[Person](site, admin.ModelAdmin{SaveAsContinue: &continueFalse})
	})
	super := createTestUser(t, db, "super", "secret", true, true)
	env.client.ForceLogin(super)
	env2.client.ForceLogin(super)
	per1 = mustCreate(t, db, &Person{Name: "John Mauchly", Gender: 1, Alive: true})
	return env, env2, per1
}

// Ported from Django: tests/admin_views/tests.py::SaveAsTests.test_save_as_duplication
func TestSaveAsDuplication(t *testing.T) {
	djangotest.EachDB(t, saveAsModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		env, _, per1 := saveAsEnv(t, db)
		postData := url.Values{"_saveasnew": {""}, "name": {"John M"}, "gender": {"1"}, "age": {"42"}}
		resp := env.client.Post(mustReverse(t, env.router, "admin:admin_views_person_change", per1.ID), postData)
		n, _ := orm.Query[Person](db).Filter("name", "John M").Count(ctx)
		djangotest.AssertEqual(t, n, int64(1))
		n, _ = orm.Query[Person](db).Filter("id", per1.ID).Count(ctx)
		djangotest.AssertEqual(t, n, int64(1))
		newPerson, err := orm.Query[Person](db).OrderBy("-id").First(ctx)
		if err != nil {
			t.Fatalf("latest person: %v", err)
		}
		djangotest.AssertRedirects(t, resp, mustReverse(t, env.router, "admin:admin_views_person_change", newPerson.ID))
	})
}

// Ported from Django: tests/admin_views/tests.py::SaveAsTests.test_save_as_continue_false
func TestSaveAsContinueFalse(t *testing.T) {
	djangotest.EachDB(t, saveAsModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		_, env2, per1 := saveAsEnv(t, db)
		postData := url.Values{"_saveasnew": {""}, "name": {"John M"}, "gender": {"1"}, "age": {"42"}}
		u := mustReverse(t, env2.router, "namespaced_admin:admin_views_person_change", per1.ID)
		resp := env2.client.Post(u, postData)
		n, _ := orm.Query[Person](db).Filter("name", "John M").Count(ctx)
		djangotest.AssertEqual(t, n, int64(1))
		n, _ = orm.Query[Person](db).Filter("id", per1.ID).Count(ctx)
		djangotest.AssertEqual(t, n, int64(1))
		djangotest.AssertRedirects(t, resp, mustReverse(t, env2.router, "namespaced_admin:admin_views_person_changelist"))
	})
}

// When you click "Save as new" and have a validation error, you only see the
// "Save as new" button and not the other save buttons.
//
// Ported from Django: tests/admin_views/tests.py::SaveAsTests.test_save_as_new_with_validation_errors
func TestSaveAsNewWithValidationErrors(t *testing.T) {
	djangotest.EachDB(t, saveAsModels(), func(t *testing.T, db *orm.DB) {
		env, _, per1 := saveAsEnv(t, db)
		resp := env.client.Post(mustReverse(t, env.router, "admin:admin_views_person_change", per1.ID), url.Values{
			"_saveasnew":  {""},
			"gender":      {"invalid"},
			"_addanother": {"fail"},
		})
		djangotest.AssertContains(t, resp, "Please correct the errors below.")
		djangotest.AssertEqual(t, ctxVal(t, resp, "show_save_and_add_another"), any(false))
		djangotest.AssertEqual(t, ctxVal(t, resp, "show_save_and_continue"), any(false))
		djangotest.AssertEqual(t, ctxVal(t, resp, "show_save_as_new"), any(true))
	})
}

// saveAsParentData returns the data of SaveAsTests that saves parent with
// child as new, with the names of the parent and the child.
func saveAsParentData(parent *Parent, child *Child, parentName, childName string) url.Values {
	return url.Values{
		"_saveasnew":              {"Save as new"},
		"child_set-0-parent":      {fmt.Sprint(parent.ID)},
		"child_set-0-id":          {fmt.Sprint(child.ID)},
		"child_set-0-name":        {childName},
		"child_set-INITIAL_FORMS": {"1"},
		"child_set-MAX_NUM_FORMS": {"1000"},
		"child_set-MIN_NUM_FORMS": {"0"},
		"child_set-TOTAL_FORMS":   {"4"},
		"name":                    {parentName},
	}
}

// Ported from Django: tests/admin_views/tests.py::SaveAsTests.test_save_as_new_with_validation_errors_with_inlines
func TestSaveAsNewWithValidationErrorsWithInlines(t *testing.T) {
	djangotest.EachDB(t, saveAsModels(), func(t *testing.T, db *orm.DB) {
		env, _, _ := saveAsEnv(t, db)
		parent := mustCreate(t, db, &Parent{Name: "Father"})
		child := &Child{Name: "Child"}
		child.Parent.SetPK(parent.ID)
		mustCreate(t, db, child)
		resp := env.client.Post(mustReverse(t, env.router, "admin:admin_views_parent_change", parent.ID),
			saveAsParentData(parent, child, "_invalid", "Child"))
		djangotest.AssertContains(t, resp, "Please correct the error below.")
		djangotest.AssertEqual(t, ctxVal(t, resp, "show_save_and_add_another"), any(false))
		djangotest.AssertEqual(t, ctxVal(t, resp, "show_save_and_continue"), any(false))
		djangotest.AssertEqual(t, ctxVal(t, resp, "show_save_as_new"), any(true))
	})
}

// Ported from Django: tests/admin_views/tests.py::SaveAsTests.test_save_as_new_with_inlines_with_validation_errors
func TestSaveAsNewWithInlinesWithValidationErrors(t *testing.T) {
	djangotest.EachDB(t, saveAsModels(), func(t *testing.T, db *orm.DB) {
		env, _, _ := saveAsEnv(t, db)
		parent := mustCreate(t, db, &Parent{Name: "Father"})
		child := &Child{Name: "Child"}
		child.Parent.SetPK(parent.ID)
		mustCreate(t, db, child)
		resp := env.client.Post(mustReverse(t, env.router, "admin:admin_views_parent_change", parent.ID),
			saveAsParentData(parent, child, "Father", "_invalid"))
		djangotest.AssertContains(t, resp, "Please correct the error below.")
		djangotest.AssertEqual(t, ctxVal(t, resp, "show_save_and_add_another"), any(false))
		djangotest.AssertEqual(t, ctxVal(t, resp, "show_save_and_continue"), any(false))
		djangotest.AssertEqual(t, ctxVal(t, resp, "show_save_as_new"), any(true))
	})
}

// saveRelatedEnv registers ParentAdmin and logs in the superuser, as
// AdminCustomSaveRelatedTests.setUpTestData and setUp do.
func saveRelatedEnv(t *testing.T, db *orm.DB) *adminTestEnv {
	t.Helper()
	env := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
		admin.Register[Parent](site, parentAdmin())
	})
	env.client.ForceLogin(createTestUser(t, db, "super", "secret", true, true))
	return env
}

// assertParentAndChildren asserts that the latest parent is named name and
// that the names of the children, in order, are children.
func assertParentAndChildren(t *testing.T, db *orm.DB, name string, children []string) {
	t.Helper()
	ctx := context.Background()
	parent, err := orm.Query[Parent](db).OrderBy("-id").First(ctx)
	if err != nil {
		t.Fatalf("latest parent: %v", err)
	}
	djangotest.AssertEqual(t, parent.Name, name)
	rows, err := orm.Query[Child](db).OrderBy("name").All(ctx)
	if err != nil {
		t.Fatalf("children: %v", err)
	}
	names := make([]string, len(rows))
	for i, c := range rows {
		names[i] = c.Name
	}
	djangotest.AssertEqual(t, names, children)
}

// Ported from Django: tests/admin_views/tests.py::AdminCustomSaveRelatedTests.test_should_be_able_to_edit_related_objects_on_add_view
func TestShouldBeAbleToEditRelatedObjectsOnAddView(t *testing.T) {
	djangotest.EachDB(t, saveAsModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		env := saveRelatedEnv(t, db)
		env.client.Post(mustReverse(t, env.router, "admin:admin_views_parent_add"), url.Values{
			"child_set-TOTAL_FORMS":   {"3"},
			"child_set-INITIAL_FORMS": {"0"},
			"name":                    {"Josh Stone"},
			"child_set-0-name":        {"Paul"},
			"child_set-1-name":        {"Catherine"},
		})
		n, _ := orm.Query[Parent](db).Count(ctx)
		djangotest.AssertEqual(t, n, int64(1))
		n, _ = orm.Query[Child](db).Count(ctx)
		djangotest.AssertEqual(t, n, int64(2))
		assertParentAndChildren(t, db, "Josh Stone", []string{"Catherine Stone", "Paul Stone"})
	})
}

// Ported from Django: tests/admin_views/tests.py::AdminCustomSaveRelatedTests.test_should_be_able_to_edit_related_objects_on_change_view
func TestShouldBeAbleToEditRelatedObjectsOnChangeView(t *testing.T) {
	djangotest.EachDB(t, saveAsModels(), func(t *testing.T, db *orm.DB) {
		env := saveRelatedEnv(t, db)
		parent := mustCreate(t, db, &Parent{Name: "Josh Stone"})
		paul := &Child{Name: "Paul"}
		paul.Parent.SetPK(parent.ID)
		mustCreate(t, db, paul)
		catherine := &Child{Name: "Catherine"}
		catherine.Parent.SetPK(parent.ID)
		mustCreate(t, db, catherine)
		env.client.Post(mustReverse(t, env.router, "admin:admin_views_parent_change", parent.ID), url.Values{
			"child_set-TOTAL_FORMS":   {"5"},
			"child_set-INITIAL_FORMS": {"2"},
			"name":                    {"Josh Stone"},
			"child_set-0-name":        {"Paul"},
			"child_set-0-id":          {fmt.Sprint(paul.ID)},
			"child_set-1-name":        {"Catherine"},
			"child_set-1-id":          {fmt.Sprint(catherine.ID)},
		})
		assertParentAndChildren(t, db, "Josh Stone", []string{"Catherine Stone", "Paul Stone"})
	})
}

// Ported from Django: tests/admin_views/tests.py::AdminCustomSaveRelatedTests.test_should_be_able_to_edit_related_objects_on_changelist_view
func TestShouldBeAbleToEditRelatedObjectsOnChangelistView(t *testing.T) {
	djangotest.EachDB(t, saveAsModels(), func(t *testing.T, db *orm.DB) {
		env := saveRelatedEnv(t, db)
		parent := mustCreate(t, db, &Parent{Name: "Josh Rock"})
		paul := &Child{Name: "Paul"}
		paul.Parent.SetPK(parent.ID)
		mustCreate(t, db, paul)
		catherine := &Child{Name: "Catherine"}
		catherine.Parent.SetPK(parent.ID)
		mustCreate(t, db, catherine)
		env.client.Post(mustReverse(t, env.router, "admin:admin_views_parent_changelist"), url.Values{
			"form-TOTAL_FORMS":   {"1"},
			"form-INITIAL_FORMS": {"1"},
			"form-MAX_NUM_FORMS": {"0"},
			"form-0-id":          {fmt.Sprint(parent.ID)},
			"form-0-name":        {"Josh Stone"},
			"_save":              {"Save"},
		})
		assertParentAndChildren(t, db, "Josh Stone", []string{"Catherine Stone", "Paul Stone"})
	})
}

// The Go Article has no section, so SectionedPost stands in for Article in
// the inline of Section.
//
// saveAsSectionEnv registers Section with the inline of SectionedPost and
// save_as, as tests/admin_views/admin.py registers Section with
// ArticleInline, creates s1 with three posts and logs in the superuser. It
// returns the data of AdminViewBasicTestCase.inline_post_data for the posts.
func saveAsSectionEnv(t *testing.T, db *orm.DB) (*adminTestEnv, *Section, url.Values) {
	t.Helper()
	env := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
		admin.Register[Section](site, admin.ModelAdmin{
			SaveAs:  true,
			Inlines: []admin.Inline{admin.TabularInline[Section, SectionedPost](admin.InlineModelAdmin{})},
		})
		admin.Register[SectionedPost](site, admin.ModelAdmin{})
	})
	env.client.ForceLogin(createTestUser(t, db, "super", "secret", true, true))
	s1 := mustCreate(t, db, &Section{Name: "Test section"})
	posts := make([]*SectionedPost, 3)
	for i, title := range []string{"", "Oldest", "Newest"} {
		p := &SectionedPost{Title: title}
		p.Section.SetPK(s1.ID)
		posts[i] = mustCreate(t, db, p)
	}
	data := url.Values{
		"name":                            {"Test section"},
		"sectionedpost_set-TOTAL_FORMS":   {"6"},
		"sectionedpost_set-INITIAL_FORMS": {"3"},
		"sectionedpost_set-MAX_NUM_FORMS": {"0"},
		"sectionedpost_set-0-id":          {fmt.Sprint(posts[0].ID)},
		"sectionedpost_set-0-title":       {"Norske bostaver æøå skaper problemer"},
		"sectionedpost_set-0-section":     {fmt.Sprint(s1.ID)},
		"sectionedpost_set-1-id":          {fmt.Sprint(posts[1].ID)},
		"sectionedpost_set-1-title":       {"Need a title."},
		"sectionedpost_set-2-id":          {fmt.Sprint(posts[2].ID)},
		"sectionedpost_set-2-title":       {"Need a title."},
	}
	for i := 3; i < 6; i++ {
		data.Set(fmt.Sprintf("sectionedpost_set-%d-id", i), "")
		data.Set(fmt.Sprintf("sectionedpost_set-%d-title", i), "")
	}
	return env, s1, data
}

// Ported from Django: tests/admin_views/tests.py::AdminViewBasicTest.test_edit_save_as
func TestEditSaveAs(t *testing.T) {
	models := append(adminViewsModels(), djangotest.App("blog", &SectionedPost{}))
	djangotest.EachDB(t, models, func(t *testing.T, db *orm.DB) {
		env, s1, postData := saveAsSectionEnv(t, db)
		postData.Set("_saveasnew", "Save+as+new")
		for i := 1; i <= 5; i++ {
			postData.Set(fmt.Sprintf("sectionedpost_set-%d-section", i), "1")
		}
		resp := env.client.Post(mustReverse(t, env.router, "admin:admin_views_section_change", s1.ID), postData)
		djangotest.AssertStatus(t, resp, http.StatusFound)
	})
}

// Should be able to "Save as new" while also deleting an inline.
//
// Ported from Django: tests/admin_views/tests.py::AdminViewBasicTest.test_edit_save_as_delete_inline
func TestEditSaveAsDeleteInline(t *testing.T) {
	models := append(adminViewsModels(), djangotest.App("blog", &SectionedPost{}))
	djangotest.EachDB(t, models, func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		env, s1, postData := saveAsSectionEnv(t, db)
		postData.Set("_saveasnew", "Save+as+new")
		postData.Set("sectionedpost_set-1-section", "1")
		postData.Set("sectionedpost_set-2-section", "1")
		postData.Set("sectionedpost_set-2-DELETE", "1")
		postData.Set("sectionedpost_set-3-section", "1")
		resp := env.client.Post(mustReverse(t, env.router, "admin:admin_views_section_change", s1.ID), postData)
		djangotest.AssertStatus(t, resp, http.StatusFound)
		// started with 3 articles, one was deleted.
		latest, err := orm.Query[Section](db).OrderBy("-id").First(ctx)
		if err != nil {
			t.Fatalf("latest section: %v", err)
		}
		n, _ := orm.Query[SectionedPost](db).Filter("section", latest.ID).Count(ctx)
		djangotest.AssertEqual(t, n, int64(2))
	})
}

// The admin URLs take an integer primary key, so Section stands in for
// ModelWithStringPrimaryKey and the primary key needs no quoting.
//
// Ported from Django: tests/admin_views/tests.py::AdminViewStringPrimaryKeyTest.test_change_view_history_link
func TestChangeViewHistoryLink(t *testing.T) {
	djangotest.EachDB(t, adminViewsModels(), func(t *testing.T, db *orm.DB) {
		env := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
			admin.Register[Section](site, admin.ModelAdmin{})
		})
		env.client.ForceLogin(createTestUser(t, db, "super", "secret", true, true))
		s := mustCreate(t, db, &Section{Name: "Test section"})
		resp := env.client.Get(mustReverse(t, env.router, "admin:admin_views_section_change", s.ID))
		djangotest.AssertStatus(t, resp, http.StatusOK)
		expectedLink := mustReverse(t, env.router, "admin:admin_views_section_history", s.ID)
		djangotest.AssertContains(t, resp, fmt.Sprintf(`<a href="%s" class="historylink"`, expectedLink))
	})
}

// No Django test exists: tests/admin_views registers admins with save_on_top
// but checks the buttons above the form only in Selenium tests. The test
// checks that SaveOnTop renders the submit row above the form as well, and
// that the add page has no History link.
func TestSaveOnTopRendersSubmitRowTwice(t *testing.T) {
	djangotest.EachDB(t, adminViewsModels(), func(t *testing.T, db *orm.DB) {
		env := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
			admin.Register[Section](site, admin.ModelAdmin{SaveOnTop: true})
			admin.Register[Actor](site, admin.ModelAdmin{})
		})
		env.client.ForceLogin(createTestUser(t, db, "super", "secret", true, true))
		s := mustCreate(t, db, &Section{Name: "Test section"})
		resp := env.client.Get(mustReverse(t, env.router, "admin:admin_views_section_change", s.ID))
		djangotest.AssertContains(t, resp, `<div class="submit-row">`, djangotest.Count(2))
		resp = env.client.Get(mustReverse(t, env.router, "admin:admin_views_section_add"))
		djangotest.AssertContains(t, resp, `<div class="submit-row">`, djangotest.Count(2))
		djangotest.AssertNotContains(t, resp, `class="historylink"`)
		resp = env.client.Get(mustReverse(t, env.router, "admin:admin_views_actor_add"))
		djangotest.AssertContains(t, resp, `<div class="submit-row">`, djangotest.Count(1))
	})
}

// The Go test registers Article and Section in place of Django's WorkHour
// and FoodDelivery, each with a list filter, so that only ShowFacets hides
// the toggle.
//
// Ported from Django: tests/admin_views/tests.py::AdminViewBasicTest.test_change_list_facet_toggle
func TestChangeListFacetToggle(t *testing.T) {
	djangotest.EachDB(t, adminViewsModels(), func(t *testing.T, db *orm.DB) {
		env := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
			admin.Register[Album](site, admin.ModelAdmin{ListFilter: []any{"title"}})
			admin.Register[Article](site, admin.ModelAdmin{ListFilter: []any{"published"}, ShowFacets: admin.ShowFacetsAlways})
			admin.Register[Section](site, admin.ModelAdmin{ListFilter: []any{"name"}, ShowFacets: admin.ShowFacetsNever})
		})
		env.client.ForceLogin(createTestUser(t, db, "super", "secret", true, true))
		// Toggle is visible when show_facet is the default of
		// admin.ShowFacets.ALLOW.
		adminURL := mustReverse(t, env.router, "admin:admin_views_album_changelist")
		resp := env.client.Get(adminURL)
		djangotest.AssertContains(t, resp, `<a href="?_facets=True" class="viewlink">Show counts</a>`)
		resp = env.client.Get(adminURL + "?_facets=True")
		djangotest.AssertContains(t, resp, `<a href="?" class="hidelink">Hide counts</a>`)
		// Toggle is not visible when show_facet is admin.ShowFacets.ALWAYS.
		resp = env.client.Get(mustReverse(t, env.router, "admin:admin_views_article_changelist"))
		djangotest.AssertNotContains(t, resp, "Show counts")
		djangotest.AssertNotContains(t, resp, "Hide counts")
		// Toggle is not visible when show_facet is admin.ShowFacets.NEVER.
		resp = env.client.Get(mustReverse(t, env.router, "admin:admin_views_section_changelist"))
		djangotest.AssertNotContains(t, resp, "Show counts")
		djangotest.AssertNotContains(t, resp, "Hide counts")
	})
}

// UndeletableObject is tests/admin_views/models.py::UndeletableObject, the
// model whose admin hides the delete link of the change page.
type UndeletableObject struct {
	ID   int64
	Name string `orm:"max_length=255"`
}

// Ported from Django: tests/admin_views/tests.py::AdminViewBasicTest.test_change_view_with_show_delete_extra_context
func TestChangeViewWithShowDeleteExtraContext(t *testing.T) {
	models := append(adminViewsModels(), djangotest.App("admin_views", &UndeletableObject{}))
	djangotest.EachDB(t, models, func(t *testing.T, db *orm.DB) {
		// The 'show_delete' context variable in the admin's change view
		// controls the display of the delete button.
		env := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
			// UndeletableObjectAdmin.
			admin.Register[UndeletableObject](site, admin.ModelAdmin{
				ExtraContext: func(_ *http.Request, view string) map[string]any {
					if view == "change" {
						return map[string]any{"show_delete": false}
					}
					return nil
				},
			})
		})
		env.client.ForceLogin(createTestUser(t, db, "super", "secret", true, true))
		instance := mustCreate(t, db, &UndeletableObject{Name: "foo"})
		resp := env.client.Get(mustReverse(t, env.router, "admin:admin_views_undeletableobject_change", instance.ID))
		djangotest.AssertNotContains(t, resp, "deletelink")
	})
}

// TestExtraContext checks that each page adds the values of the
// ExtraContext of its ModelAdmin or AdminSite, and that the add and change
// pages keep their own permission flags.
//
// No Django test exists: Django passes extra_context to each view method,
// and its tests cover the changelist, change, index and password change
// pages.
func TestExtraContext(t *testing.T) {
	djangotest.EachDB(t, adminViewsModels(), func(t *testing.T, db *orm.DB) {
		extra := func(_ *http.Request, view string) map[string]any {
			return map[string]any{"extra_view": view, "title": "Extra " + view, "has_add_permission": false}
		}
		env := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
			site.ExtraContext = extra
			admin.Register[Section](site, admin.ModelAdmin{ExtraContext: extra})
		})
		env.client.ForceLogin(createTestUser(t, db, "super", "secret", true, true))
		section := mustCreate(t, db, &Section{Name: "s"})
		for _, tc := range []struct {
			view, url string
			// addPermission is has_add_permission: the add and change pages
			// set it after the extra context, as Django's render_change_form.
			addPermission any
		}{
			{"index", mustReverse(t, env.router, "admin:index"), false},
			{"app_index", mustReverse(t, env.router, "admin:app_list", "admin_views"), nil},
			{"changelist", mustReverse(t, env.router, "admin:admin_views_section_changelist"), false},
			{"add", mustReverse(t, env.router, "admin:admin_views_section_add"), true},
			{"change", mustReverse(t, env.router, "admin:admin_views_section_change", section.ID), true},
			{"delete", mustReverse(t, env.router, "admin:admin_views_section_delete", section.ID), false},
			{"history", mustReverse(t, env.router, "admin:admin_views_section_history", section.ID), false},
			{"password_change", mustReverse(t, env.router, "admin:password_change"), false},
			{"password_change_done", mustReverse(t, env.router, "admin:password_change_done"), false},
		} {
			t.Run(tc.view, func(t *testing.T) {
				resp := env.client.Get(tc.url)
				djangotest.AssertStatus(t, resp, http.StatusOK)
				djangotest.AssertEqual(t, ctxVal(t, resp, "extra_view"), any(tc.view))
				djangotest.AssertEqual(t, ctxVal(t, resp, "title"), any("Extra "+tc.view))
				if tc.addPermission != nil {
					djangotest.AssertEqual(t, ctxVal(t, resp, "has_add_permission"), tc.addPermission)
				}
			})
		}
		t.Run("logout", func(t *testing.T) {
			resp := env.client.Post(mustReverse(t, env.router, "admin:logout"), nil)
			djangotest.AssertEqual(t, ctxVal(t, resp, "extra_view"), any("logout"))
			djangotest.AssertEqual(t, ctxVal(t, resp, "title"), any("Extra logout"))
		})
		t.Run("login", func(t *testing.T) {
			resp := env.client.Get(mustReverse(t, env.router, "admin:login"))
			djangotest.AssertEqual(t, ctxVal(t, resp, "extra_view"), any("login"))
			djangotest.AssertEqual(t, ctxVal(t, resp, "title"), any("Extra login"))
		})
	})
}
