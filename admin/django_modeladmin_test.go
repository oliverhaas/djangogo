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

// Ported from Django: tests/modeladmin/tests.py::ModelAdminTests.test_get_fieldsets
func TestModelAdminGetFieldsets(t *testing.T) {
	djangotest.EachDB(t, modeladminCheckModels(), func(t *testing.T, db *orm.DB) {
		// FIXME-PORT: the InlineBandAdmin half needs inlines, which the Go
		// admin does not have yet.
		resp := bandAdminPage(t, db, admin.ModelAdmin{
			GetFieldsets: func(*http.Request, any) []admin.Fieldset {
				return []admin.Fieldset{{Fields: []any{"name", "bio"}}}
			},
		}, false)
		assertFormInputs(t, resp, "name", "bio")
	})
}

// Ported from Django: tests/modeladmin/tests.py::ModelAdminTests.test_custom_form_meta_exclude_with_readonly
// Ported from Django: tests/modeladmin/tests.py::ModelAdminTests.test_custom_formfield_override_readonly
// Ported from Django: tests/modeladmin/tests.py::ModelAdminTests.test_custom_form_validation
func TestModelAdminCustomForm(t *testing.T) {
	djangotest.EachDB(t, modeladminCheckModels(), func(t *testing.T, db *orm.DB) {
		// FIXME-PORT: the InlineModelAdmin halves need inlines. Django's
		// test_custom_form_meta_exclude has no Go counterpart: the
		// exclusions of FormOptions add to ExcludeFields, there is no form
		// class whose Meta.exclude ExcludeFields could override. The
		// AdminDateWidget assertion of test_custom_form_validation is left
		// out because the admin ships no date widget.
		t.Run("test_custom_form_meta_exclude_with_readonly", func(t *testing.T) {
			resp := bandAdminPage(t, db, admin.ModelAdmin{
				ReadonlyFields: []string{"name"},
				FormOptions:    []forms.ModelFormOption{forms.WithExclude("bio")},
			}, false)
			assertFormInputs(t, resp, "sign_date")
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
		})
	})
}
