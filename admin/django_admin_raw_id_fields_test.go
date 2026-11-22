package admin_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/admin"
	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
)

// Inquisition stands in for tests/admin_views/models.py::Inquisition.
type Inquisition struct {
	ID       int64
	Expected bool
	Leader   orm.FK[Actor] `orm:"on_delete=cascade"`
	Country  string        `orm:"max_length=20"`
}

// String returns the country. Django's __str__ also names the leader, which
// a String method cannot load.
func (i Inquisition) String() string { return "from " + i.Country }

// Sketch stands in for tests/admin_views/models.py::Sketch.
type Sketch struct {
	ID          int64
	Title       string              `orm:"max_length=100"`
	Inquisition orm.FK[Inquisition] `orm:"on_delete=cascade"`
	Defendant0  orm.FK[Actor]       `orm:"on_delete=cascade;related_name=as_defendant0"`
	Defendant1  orm.FK[Actor]       `orm:"on_delete=cascade;related_name=as_defendant1"`
}

// String returns the title.
func (s Sketch) String() string { return s.Title }

// Meta limits the choices of the three foreign keys as Django's Sketch does.
func (Sketch) Meta() orm.Meta {
	return orm.Meta{LimitChoicesTo: map[string]orm.Cond{
		"Inquisition": orm.Q("leader__name", "Palin", "leader__age", 27, "expected", false),
		"Defendant0":  orm.Q("title__isnull", false),
		"Defendant1":  orm.Q("title__isnull", true),
	}}
}

// rawIDFieldsEnv returns the client of a superuser on a site with the
// registrations of Django's InquisitionAdmin and SketchAdmin.
func rawIDFieldsEnv(t *testing.T, db *orm.DB) *adminTestEnv {
	t.Helper()
	env := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
		admin.Register[Actor](site, admin.ModelAdmin{})
		admin.Register[Inquisition](site, admin.ModelAdmin{
			ListDisplay: []string{"leader", "country", "expected", "sketch"},
			DisplayColumns: map[string]admin.DisplayColumn{
				// A method with the same name as a reverse accessor.
				"sketch": {Value: func(any) any { return "list-display-sketch" }},
			},
		})
		admin.Register[Sketch](site, admin.ModelAdmin{
			RawIDFields: []string{"inquisition", "defendant0", "defendant1"},
		})
	})
	env.client.ForceLogin(createTestUser(t, db, "super", "secret", true, true))
	return env
}

// rawIDFieldsModels returns the model set of the RawIdFieldsTest ports.
func rawIDFieldsModels() []any {
	return append(adminViewsModels(), djangotest.App("admin_views", &Inquisition{}, &Sketch{}))
}

// popupURL returns the URL of the lookup link with the id given on the page of
// resp.
func popupURL(t *testing.T, resp *djangotest.Response, id string) string {
	t.Helper()
	m := regexp.MustCompile(`<a href="([^"]*)"[^>]* id="` + id + `"`).FindStringSubmatch(resp.Content())
	if m == nil {
		t.Fatalf("no lookup link %s in:\n%s", id, resp.Content())
	}
	return strings.ReplaceAll(m[1], "&amp;", "&")
}

// Ported from Django: tests/admin_views/tests.py::RawIdFieldsTest.test_limit_choices_to
// Ported from Django: tests/admin_views/tests.py::RawIdFieldsTest.test_limit_choices_to_isnull_false
// Ported from Django: tests/admin_views/tests.py::RawIdFieldsTest.test_limit_choices_to_isnull_true
// Ported from Django: tests/admin_views/tests.py::RawIdFieldsTest.test_list_display_method_same_name_as_reverse_accessor
func TestRawIDFields(t *testing.T) {
	djangotest.EachDB(t, rawIDFieldsModels(), func(t *testing.T, db *orm.DB) {
		env := rawIDFieldsEnv(t, db)
		addURL := mustReverse(t, env.router, "admin:admin_views_sketch_add")

		t.Run("test_limit_choices_to", func(t *testing.T) {
			// Regression test for 14880
			actor := create(t, db, &Actor{Name: "Palin", Age: 27})
			for _, inq := range []*Inquisition{
				{Expected: true, Country: "England"},
				{Expected: false, Country: "Spain"},
			} {
				inq.Leader.SetPK(actor.ID)
				create(t, db, inq)
			}
			resp := env.client.Get(addURL)
			// Get the popup and verify the correct objects show up in the
			// resulting page. This step also tests integers, strings and
			// booleans in the lookup query string; in model we define
			// inquisition field to have a limit_choices_to option that
			// includes a filter on a string field (inquisition__actor__name),
			// a filter on an integer field (inquisition__actor__age), and a
			// filter on a boolean field (inquisition__expected).
			resp2 := env.client.Get(popupURL(t, resp, "lookup_id_inquisition"))
			djangotest.AssertContains(t, resp2, "Spain")
			djangotest.AssertNotContains(t, resp2, "England")
		})

		t.Run("test_limit_choices_to_isnull_false", func(t *testing.T) {
			// Regression test for 20182
			create(t, db, &Actor{Name: "Palin", Age: 27})
			judge := "Judge"
			create(t, db, &Actor{Name: "Kilbraken", Age: 50, Title: &judge})
			resp := env.client.Get(addURL)
			// Get the popup and verify the correct objects show up in the
			// resulting page. This step tests field__isnull=0 gets parsed
			// correctly from the lookup query string; in model we define
			// defendant0 field to have a limit_choices_to option that
			// includes "actor__title__isnull=False".
			resp2 := env.client.Get(popupURL(t, resp, "lookup_id_defendant0"))
			djangotest.AssertContains(t, resp2, "Kilbraken")
			djangotest.AssertNotContains(t, resp2, "Palin")
		})

		t.Run("test_limit_choices_to_isnull_true", func(t *testing.T) {
			// Regression test for 20182
			create(t, db, &Actor{Name: "Palin", Age: 27})
			judge := "Judge"
			create(t, db, &Actor{Name: "Kilbraken", Age: 50, Title: &judge})
			resp := env.client.Get(addURL)
			// Get the popup and verify the correct objects show up in the
			// resulting page. This step tests field__isnull=1 gets parsed
			// correctly from the lookup query string; in model we define
			// defendant1 field to have a limit_choices_to option that
			// includes "actor__title__isnull=True".
			resp2 := env.client.Get(popupURL(t, resp, "lookup_id_defendant1"))
			djangotest.AssertNotContains(t, resp2, "Kilbraken")
			djangotest.AssertContains(t, resp2, "Palin")
		})

		t.Run("test_list_display_method_same_name_as_reverse_accessor", func(t *testing.T) {
			// Should be able to use a ModelAdmin method in list_display that
			// has the same name as a reverse model field ("sketch" in this
			// case).
			actor := create(t, db, &Actor{Name: "Palin", Age: 27})
			inq := &Inquisition{Expected: true, Country: "England"}
			inq.Leader.SetPK(actor.ID)
			create(t, db, inq)
			resp := env.client.Get(mustReverse(t, env.router, "admin:admin_views_inquisition_changelist"))
			djangotest.AssertContains(t, resp, "list-display-sketch")
		})
	})
}
