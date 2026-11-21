package admin_test

import (
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/oliverhaas/djangogo/admin"
	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/forms"
	"github.com/oliverhaas/djangogo/orm"
)

// Django's tests in tests/modeladmin/tests.py read the form class and the
// layout from the ModelAdmin methods. The Go admin keeps them inside the add
// and change views, so these ports read the same facts from the page: the
// inputs of the form in order, and the read-only fields.

// bandPageUsers numbers the superusers of bandAdminPage, which a test may
// call more than once on one database.
var bandPageUsers atomic.Int64

// bandAdminPage registers Band with ma on a fresh site, logs a superuser in,
// saves the band of ModelAdminTests.setUpTestData and returns the add page,
// or the change page of that band when change is true.
func bandAdminPage(t *testing.T, db *orm.DB, ma admin.ModelAdmin, change bool) *djangotest.Response {
	t.Helper()
	env := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
		admin.Register[Band](site, ma)
	})
	user := fmt.Sprintf("super%d", bandPageUsers.Add(1))
	env.client.ForceLogin(createTestUser(t, db, user, "secret", true, true))
	band := &Band{Name: "The Doors", SignDate: orm.Date{Year: 1965, Month: 1, Day: 1}}
	if err := orm.Query[Band](db).Create(t.Context(), band); err != nil {
		t.Fatalf("create band: %v", err)
	}
	url := mustReverse(t, env.router, "admin:modeladmin_band_add")
	if change {
		url = mustReverse(t, env.router, "admin:modeladmin_band_change", band.ID)
	}
	resp := env.client.Get(url)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s = %d", url, resp.StatusCode)
	}
	return resp
}

// assertFormInputs asserts that the form fields of page are want, in order:
// each one renders an element named after it, and no other field of Band or
// the extra fields of the tests does.
func assertFormInputs(t *testing.T, resp *djangotest.Response, want ...string) {
	t.Helper()
	page := string(resp.Body)
	pos := -1
	for _, name := range want {
		i := strings.Index(page, `name="`+name+`"`)
		if i < 0 {
			t.Errorf("page has no input %q", name)
			continue
		}
		if i < pos {
			t.Errorf("input %q is out of order, want %v", name, want)
		}
		pos = i
	}
	for _, name := range []string{"name", "bio", "sign_date", "delete"} {
		if !slices.Contains(want, name) && strings.Contains(page, `name="`+name+`"`) {
			t.Errorf("page has input %q, want only %v", name, want)
		}
	}
}

// Ported from Django: tests/modeladmin/tests.py::ModelAdminTests.test_default_fields
// Ported from Django: tests/modeladmin/tests.py::ModelAdminTests.test_default_fieldsets
func TestModelAdminDefaultFields(t *testing.T) {
	djangotest.EachDB(t, modeladminCheckModels(), func(t *testing.T, db *orm.DB) {
		for _, change := range []bool{false, true} {
			resp := bandAdminPage(t, db, admin.ModelAdmin{}, change)
			assertFormInputs(t, resp, "name", "bio", "sign_date")
			// One fieldset without a name holds them.
			if n := strings.Count(string(resp.Body), `<fieldset class="module aligned`); n != 1 {
				t.Errorf("page has %d fieldsets, want 1", n)
			}
			djangotest.AssertNotContains(t, resp, "fieldset-heading")
		}
	})
}

// concertFormFields returns the fields of the first form of the Concert
// inline of cfg on the add page of Band, or the change page with change,
// Django's list(ma.get_formsets_with_inlines(request))[0][0]().forms[0].fields.
// The inline uses the foreign key main_band.
func concertFormFields(t *testing.T, db *orm.DB, cfg admin.InlineModelAdmin, change bool) []string {
	t.Helper()
	cfg.FKName = "main_band"
	resp := bandAdminPage(t, db, admin.ModelAdmin{Inlines: []admin.Inline{
		admin.TabularInline[Band, Concert](cfg),
	}}, change)
	form := inlineFormSet(t, resp, 0)["forms"].([]any)[0].(forms.TemplateForm)
	fields := form["fields"].([]forms.TemplateField)
	names := make([]string, 0, len(fields))
	for _, f := range fields {
		names = append(names, f["name"].(string))
	}
	return names
}

// Ported from Django: tests/modeladmin/tests.py::ModelAdminTests.test_get_fieldsets
func TestModelAdminGetFieldsets(t *testing.T) {
	djangotest.EachDB(t, modeladminCheckModels(), func(t *testing.T, db *orm.DB) {
		resp := bandAdminPage(t, db, admin.ModelAdmin{
			GetFieldsets: func(*http.Request, any) []admin.Fieldset {
				return []admin.Fieldset{{Fields: []any{"name", "bio"}}}
			},
		}, false)
		assertFormInputs(t, resp, "name", "bio")

		// The form of an inline holds the fields of its fieldsets, the
		// foreign key to the parent and the primary key.
		fields := concertFormFields(t, db, admin.InlineModelAdmin{
			CanDelete: new(false),
			GetFieldsets: func(*http.Request, any) []admin.Fieldset {
				return []admin.Fieldset{{Fields: []any{"day", "transport"}}}
			},
		}, false)
		djangotest.AssertEqual(t, fields, []string{"day", "transport", "id", "main_band"})
	})
}

// Ported from Django: tests/modeladmin/tests.py::ModelAdminTests.test_overriding_get_exclude
// Ported from Django: tests/modeladmin/tests.py::ModelAdminTests.test_get_exclude_overrides_exclude
// Ported from Django: tests/modeladmin/tests.py::ModelAdminTests.test_get_exclude_takes_obj
func TestModelAdminGetExclude(t *testing.T) {
	djangotest.EachDB(t, modeladminCheckModels(), func(t *testing.T, db *orm.DB) {
		t.Run("test_overriding_get_exclude", func(t *testing.T) {
			resp := bandAdminPage(t, db, admin.ModelAdmin{
				GetExclude: func(*http.Request, any) []string { return []string{"name"} },
			}, false)
			assertFormInputs(t, resp, "bio", "sign_date")
		})

		t.Run("test_get_exclude_overrides_exclude", func(t *testing.T) {
			resp := bandAdminPage(t, db, admin.ModelAdmin{
				ExcludeFields: []string{"bio"},
				GetExclude:    func(*http.Request, any) []string { return []string{"name"} },
			}, false)
			assertFormInputs(t, resp, "bio", "sign_date")
		})

		t.Run("test_get_exclude_takes_obj", func(t *testing.T) {
			resp := bandAdminPage(t, db, admin.ModelAdmin{
				GetExclude: func(_ *http.Request, obj any) []string {
					if obj != nil {
						return []string{"sign_date"}
					}
					return []string{"name"}
				},
			}, true)
			assertFormInputs(t, resp, "name", "bio")
		})
	})
}

// Ported from Django: tests/modeladmin/tests.py::ModelAdminTests.test_formset_overriding_get_exclude_with_form_fields
func TestModelAdminFormsetOverridingGetExcludeWithFormFields(t *testing.T) {
	djangotest.EachDB(t, modeladminCheckModels(), func(t *testing.T, db *orm.DB) {
		fields := concertFormFields(t, db, admin.InlineModelAdmin{
			FormOptions: []forms.ModelFormOption{forms.WithFields("main_band", "opening_band", "day", "transport")},
			GetExclude:  func(*http.Request, any) []string { return []string{"opening_band"} },
		}, false)
		djangotest.AssertEqual(t, fields, []string{"main_band", "day", "transport", "id", "DELETE"})
	})
}

// No Go counterpart: tests/modeladmin/tests.py::ModelAdminTests.test_formset_overriding_get_exclude_with_form_exclude
// lets get_exclude override the Meta.exclude of a custom form. The exclusions
// of FormOptions add to those of the admin; there is no form class whose
// Meta.exclude the admin could override.

// No Go counterpart: tests/modeladmin/tests.py::ModelAdminTests.test_form_exclude_kwarg_override,
// test_formset_exclude_kwarg_override, test_raw_id_fields_widget_override and
// test_regression_for_ticket_15820 override get_form or get_formset to change
// the keyword arguments of the form factory. The Go admin has no such hook;
// GetExclude, GetFields, GetFieldsets and FormOptions configure the forms.

// Ported from Django: tests/modeladmin/tests.py::ModelAdminPermissionTests.test_inline_has_add_permission_uses_obj
//
// Django asks get_inline_instances directly. The Go port reads the pages of a
// user who may add and change bands: the add page has no Concert inline, the
// change page has it. Django's inline has no fk_name, which the Go admin needs
// to build the forms of an inline with two foreign keys to the parent.
func TestInlineHasAddPermissionUsesObj(t *testing.T) {
	djangotest.EachDB(t, modeladminCheckModels(), func(t *testing.T, db *orm.DB) {
		env := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
			admin.Register[Band](site, admin.ModelAdmin{Inlines: []admin.Inline{
				admin.TabularInline[Band, Concert](admin.InlineModelAdmin{
					FKName:           "main_band",
					HasAddPermission: func(_ *http.Request, obj any) bool { return obj != nil },
				}),
			}})
		})
		user := createTestUser(t, db, "adduser", "secret", true, false)
		grantPerm(t, db, user, "modeladmin", "add_band")
		grantPerm(t, db, user, "modeladmin", "change_band")
		env.client.ForceLogin(user)
		resp := env.client.Get(mustReverse(t, env.router, "admin:modeladmin_band_add"))
		djangotest.AssertStatus(t, resp, http.StatusOK)
		djangotest.AssertEqual(t, len(inlineSets(t, resp)), 0)

		band := &Band{Name: "The Doors", SignDate: orm.Date{Year: 1965, Month: 1, Day: 1}}
		if err := orm.Query[Band](db).Create(t.Context(), band); err != nil {
			t.Fatalf("create band: %v", err)
		}
		resp = env.client.Get(mustReverse(t, env.router, "admin:modeladmin_band_change", band.ID))
		djangotest.AssertEqual(t, len(inlineSets(t, resp)), 1)
		djangotest.AssertContains(t, resp, `id="id_main_concerts-TOTAL_FORMS"`)
	})
}

// Ported from Django: tests/modeladmin/tests.py::ModelAdminTests.test_custom_form_meta_exclude_with_readonly
// Ported from Django: tests/modeladmin/tests.py::ModelAdminTests.test_custom_formfield_override_readonly
// Ported from Django: tests/modeladmin/tests.py::ModelAdminTests.test_custom_form_validation
func TestModelAdminCustomForm(t *testing.T) {
	djangotest.EachDB(t, modeladminCheckModels(), func(t *testing.T, db *orm.DB) {
		// Django's test_custom_form_meta_exclude has no Go counterpart: the
		// exclusions of FormOptions add to ExcludeFields, there is no form
		// class whose Meta.exclude ExcludeFields could override.
		t.Run("test_custom_form_meta_exclude_with_readonly", func(t *testing.T) {
			resp := bandAdminPage(t, db, admin.ModelAdmin{
				ReadonlyFields: []string{"name"},
				FormOptions:    []forms.ModelFormOption{forms.WithExclude("bio")},
			}, false)
			assertFormInputs(t, resp, "sign_date")

			// With an inline.
			fields := concertFormFields(t, db, admin.InlineModelAdmin{
				ReadonlyFields: []string{"transport"},
				FormOptions:    []forms.ModelFormOption{forms.WithExclude("day")},
			}, false)
			djangotest.AssertEqual(t, fields, []string{"main_band", "opening_band", "id", "DELETE"})
		})

		t.Run("test_custom_formfield_override_readonly", func(t *testing.T) {
			resp := bandAdminPage(t, db, admin.ModelAdmin{
				ReadonlyFields: []string{"name"},
				FormOptions:    []forms.ModelFormOption{forms.WithFormField("name", forms.NewCharField())},
			}, true)
			page := string(resp.Body)
			// name is no input but is shown as read-only, after the
			// form fields.
			assertFormInputs(t, resp, "bio", "sign_date")
			djangotest.AssertContains(t, resp, `<div class="form-row field-name">`)
			if strings.Index(page, `field-name"`) < strings.Index(page, `name="sign_date"`) {
				t.Error("the read-only name comes before sign_date, want it last")
			}
			djangotest.AssertContains(t, resp, `<div class="readonly">The Doors</div>`)
		})

		t.Run("test_custom_form_validation", func(t *testing.T) {
			resp := bandAdminPage(t, db, admin.ModelAdmin{
				FormOptions: []forms.ModelFormOption{forms.WithFormField("delete", forms.NewBooleanField())},
			}, false)
			assertFormInputs(t, resp, "name", "bio", "sign_date", "delete")
			djangotest.AssertContains(t, resp, `<input type="text" name="sign_date" class="vDateField"`)
		})
	})
}
