package admin_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/oliverhaas/djangogo/admin"
	"github.com/oliverhaas/djangogo/contenttypes"
	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/forms"
	"github.com/oliverhaas/djangogo/orm"
)

// GetAbsoluteURL is Django's City.get_absolute_url.
func (c City) GetAbsoluteURL() string { return fmt.Sprintf("/dummy/%d/", c.ID) }

// Restaurant is Django's admin_views.Restaurant.
type Restaurant struct {
	ID   int64
	City orm.FK[City] `orm:"on_delete=cascade"`
	Name string       `orm:"max_length=100"`
}

// Meta gives Restaurant Django's long verbose name.
func (Restaurant) Meta() orm.Meta {
	return orm.Meta{VerboseName: "very very very very very very very very very " +
		"loooooooooooooooooooooooooooooooooooooooooong name"}
}

// GetAbsoluteURL is Django's Restaurant.get_absolute_url.
func (r Restaurant) GetAbsoluteURL() string { return fmt.Sprintf("/dummy/%d/", r.ID) }

// viewOnSiteData holds the rows of the setUpTestData of AdminViewOnSiteTests
// and InlineAdminViewOnSiteTest.
type viewOnSiteData struct {
	s1 *State
	c1 *City
	r1 *Restaurant
	w1 *Worker
}

// viewOnSiteRows creates the states, cities, restaurants and workers of
// Django's setUpTestData.
func viewOnSiteRows(t *testing.T, db *orm.DB) viewOnSiteData {
	t.Helper()
	var d viewOnSiteData
	states := map[string]*State{}
	for _, name := range []string{"New York", "Illinois", "California"} {
		states[name] = mustCreate(t, db, &State{Name: name})
	}
	d.s1 = states["New York"]
	city := func(state, name string) *City {
		c := &City{Name: name}
		c.State.SetPK(states[state].ID)
		return mustCreate(t, db, c)
	}
	d.c1 = city("New York", "New York")
	c2 := city("Illinois", "Chicago")
	c3 := city("California", "San Francisco")
	restaurant := func(c *City, name string) *Restaurant {
		r := &Restaurant{Name: name}
		r.City.SetPK(c.ID)
		return mustCreate(t, db, r)
	}
	d.r1 = restaurant(d.c1, "Italian Pizza")
	restaurant(d.c1, "Boulevard")
	restaurant(c2, "Chinese Dinner")
	restaurant(c2, "Angels")
	restaurant(c2, "Take Away")
	restaurant(c3, "The Unknown Restaurant")
	worker := func(name, surname string) *Worker {
		w := &Worker{Name: name, Surname: surname}
		w.WorkAt.SetPK(d.r1.ID)
		return mustCreate(t, db, w)
	}
	d.w1 = worker("Mario", "Rossi")
	worker("Antonio", "Bianchi")
	worker("John", "Doe")
	return d
}

// registerViewOnSiteAdmins registers Django's StateAdmin, CityAdmin,
// WorkerAdmin and RestaurantAdmin with their inlines.
func registerViewOnSiteAdmins(site *admin.AdminSite) {
	no, yes := false, true
	admin.Register[State](site, admin.ModelAdmin{Inlines: []admin.Inline{
		admin.TabularInline[State, City](admin.InlineModelAdmin{ViewOnSite: &no}),
	}})
	admin.Register[City](site, admin.ModelAdmin{
		ViewOnSite: &yes,
		Inlines: []admin.Inline{
			admin.TabularInline[City, Restaurant](admin.InlineModelAdmin{
				ViewOnSite: &yes,
				// Django's CityAdmin.get_formset_kwargs gives each form of the
				// inline the initial name "overridden_name".
				FormSet: func(_ *http.Request, _ any, fs *forms.FormSet) {
					configure := fs.ConfigureForm
					fs.ConfigureForm = func(form *forms.Form, i int) {
						configure(form, i)
						if form.Initial == nil {
							form.Initial = map[string]any{}
						}
						form.Initial["name"] = "overridden_name"
					}
				},
			}),
		},
	})
	admin.Register[Worker](site, admin.ModelAdmin{ViewOnSiteURL: func(obj any) string {
		w := obj.(*Worker)
		return fmt.Sprintf("/worker/%s/%s/", w.Surname, w.Name)
	}})
	admin.Register[Restaurant](site, admin.ModelAdmin{
		ViewOnSite: &no,
		Inlines: []admin.Inline{
			admin.TabularInline[Restaurant, Worker](admin.InlineModelAdmin{ViewOnSiteURL: func(obj any) string {
				w := obj.(*Worker)
				return fmt.Sprintf("/worker_inline/%s/%s/", w.Surname, w.Name)
			}}),
		},
	})
}

// viewOnSiteEnv returns an admin with the view-on-site admins and a logged-in
// superuser, and the rows of Django's setUpTestData.
func viewOnSiteEnv(t *testing.T, db *orm.DB) (*adminTestEnv, viewOnSiteData) {
	t.Helper()
	d := viewOnSiteRows(t, db)
	env := newAdminTestEnv(t, db, registerViewOnSiteAdmins)
	env.client.ForceLogin(createTestUser(t, db, "super", "secret", true, true))
	return env, d
}

// contentTypeID returns the ID of the content type of the model of instance.
func contentTypeID(t *testing.T, db *orm.DB, instance any) int64 {
	t.Helper()
	m, ok := db.Registry().ModelOf(instance)
	if !ok {
		t.Fatalf("model of %T is not registered", instance)
	}
	ct, err := contenttypes.GetForModel(context.Background(), db, m)
	if err != nil {
		t.Fatalf("GetForModel(%T): %v", instance, err)
	}
	return ct.ID
}

// No Go counterpart: the last case of
// tests/admin_views/tests.py::AdminViewOnSiteTests.test_check sets
// view_on_site to a list (admin.E025), which Go rules out at compile time:
// ModelAdmin.ViewOnSite is a *bool and ViewOnSiteURL a function.
//
// Ported from Django: tests/admin_views/tests.py::AdminViewOnSiteTests.test_check
func TestAdminViewOnSiteCheck(t *testing.T) {
	djangotest.EachDB(t, adminViewsModels(), func(t *testing.T, db *orm.DB) {
		no, yes := false, true
		for _, a := range []admin.ModelAdmin{
			{ViewOnSite: &yes},
			{ViewOnSite: &no},
			{ViewOnSiteURL: func(obj any) string { return obj.(*City).GetAbsoluteURL() }},
		} {
			assertIsValid(t, checkSite(t, db, func(site *admin.AdminSite) { admin.Register[City](site, a) }))
		}
	})
}

// Ported from Django: tests/admin_views/tests.py::AdminViewOnSiteTests.test_false
func TestAdminViewOnSiteFalse(t *testing.T) {
	djangotest.EachDB(t, adminViewsModels(), func(t *testing.T, db *orm.DB) {
		env, d := viewOnSiteEnv(t, db)
		resp := env.client.Get(mustReverse(t, env.router, "admin:admin_views_restaurant_change", d.r1.ID))
		djangotest.AssertStatus(t, resp, http.StatusOK)
		djangotest.AssertNotContains(t, resp,
			mustReverse(t, env.router, "admin:view_on_site", contentTypeID(t, db, &Restaurant{}), d.r1.ID))
	})
}

// Ported from Django: tests/admin_views/tests.py::AdminViewOnSiteTests.test_true
func TestAdminViewOnSiteTrue(t *testing.T) {
	djangotest.EachDB(t, adminViewsModels(), func(t *testing.T, db *orm.DB) {
		env, d := viewOnSiteEnv(t, db)
		resp := env.client.Get(mustReverse(t, env.router, "admin:admin_views_city_change", d.c1.ID))
		djangotest.AssertContains(t, resp,
			mustReverse(t, env.router, "admin:view_on_site", contentTypeID(t, db, &City{}), d.c1.ID))
	})
}

// Ported from Django: tests/admin_views/tests.py::AdminViewOnSiteTests.test_callable
func TestAdminViewOnSiteCallable(t *testing.T) {
	djangotest.EachDB(t, adminViewsModels(), func(t *testing.T, db *orm.DB) {
		env, d := viewOnSiteEnv(t, db)
		resp := env.client.Get(mustReverse(t, env.router, "admin:admin_views_worker_change", d.w1.ID))
		djangotest.AssertContains(t, resp, fmt.Sprintf(`"/worker/%s/%s/"`, d.w1.Surname, d.w1.Name))
	})
}

// Ported from Django: tests/admin_views/tests.py::AdminViewOnSiteTests.test_missing_get_absolute_url
func TestAdminViewOnSiteMissingGetAbsoluteURL(t *testing.T) {
	djangotest.EachDB(t, adminViewsModels(), func(t *testing.T, db *orm.DB) {
		site := checkSite(t, db, func(site *admin.AdminSite) { admin.Register[Worker](site, admin.ModelAdmin{}) })
		djangotest.AssertEqual(t, admin.ViewOnSiteURL(context.Background(), site, "admin_views", "worker", &Worker{}), "")
	})
}

// Ported from Django: tests/admin_views/tests.py::AdminViewOnSiteTests.test_custom_admin_site
func TestAdminViewOnSiteCustomAdminSite(t *testing.T) {
	djangotest.EachDB(t, adminViewsModels(), func(t *testing.T, db *orm.DB) {
		d := viewOnSiteRows(t, db)
		env := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
			site.Name = "admin2"
			admin.Register[City](site, admin.ModelAdmin{})
		})
		got := admin.ViewOnSiteURL(context.Background(), env.site, "admin_views", "city", d.c1)
		djangotest.AssertEqual(t, got,
			mustReverse(t, env.router, "admin2:view_on_site", contentTypeID(t, db, &City{}), d.c1.ID))
	})
}

// Ported from Django: tests/admin_views/tests.py::AdminViewOnSiteTests.test_view_on_site_url_non_integer_ids
func TestViewOnSiteURLNonIntegerIDs(t *testing.T) {
	djangotest.EachDB(t, adminViewsModels(), func(t *testing.T, db *orm.DB) {
		env := newAdminTestEnv(t, db, nil)
		djangotest.AssertEqual(t,
			mustReverse(t, env.router, "admin:view_on_site", "37156b6a-8a82", "37156b6a-8a83"),
			"/admin/r/37156b6a-8a82/37156b6a-8a83/")
	})
}

// No Django test exists: the admin_views tests check the link, and the
// contenttypes tests the shortcut view behind it. The test follows the link
// of the admin to the page of the object.
func TestAdminViewOnSiteRedirects(t *testing.T) {
	djangotest.EachDB(t, adminViewsModels(), func(t *testing.T, db *orm.DB) {
		env, d := viewOnSiteEnv(t, db)
		resp := env.client.Get(mustReverse(t, env.router, "admin:view_on_site", contentTypeID(t, db, &City{}), d.c1.ID))
		djangotest.AssertRedirects(t, resp, fmt.Sprintf("http://testserver/dummy/%d/", d.c1.ID),
			djangotest.FetchRedirectResponse(false))
	})
}

// Ported from Django: tests/admin_views/tests.py::InlineAdminViewOnSiteTest.test_false
func TestInlineAdminViewOnSiteFalse(t *testing.T) {
	djangotest.EachDB(t, adminViewsModels(), func(t *testing.T, db *orm.DB) {
		env, d := viewOnSiteEnv(t, db)
		resp := env.client.Get(mustReverse(t, env.router, "admin:admin_views_state_change", d.s1.ID))
		djangotest.AssertStatus(t, resp, http.StatusOK)
		djangotest.AssertNotContains(t, resp,
			mustReverse(t, env.router, "admin:view_on_site", contentTypeID(t, db, &City{}), d.c1.ID))
	})
}

// Ported from Django: tests/admin_views/tests.py::InlineAdminViewOnSiteTest.test_true
func TestInlineAdminViewOnSiteTrue(t *testing.T) {
	djangotest.EachDB(t, adminViewsModels(), func(t *testing.T, db *orm.DB) {
		env, d := viewOnSiteEnv(t, db)
		resp := env.client.Get(mustReverse(t, env.router, "admin:admin_views_city_change", d.c1.ID))
		djangotest.AssertContains(t, resp,
			mustReverse(t, env.router, "admin:view_on_site", contentTypeID(t, db, &Restaurant{}), d.r1.ID))
	})
}

// Ported from Django: tests/admin_views/tests.py::InlineAdminViewOnSiteTest.test_callable
func TestInlineAdminViewOnSiteCallable(t *testing.T) {
	djangotest.EachDB(t, adminViewsModels(), func(t *testing.T, db *orm.DB) {
		env, d := viewOnSiteEnv(t, db)
		resp := env.client.Get(mustReverse(t, env.router, "admin:admin_views_restaurant_change", d.r1.ID))
		djangotest.AssertContains(t, resp, fmt.Sprintf(`"/worker_inline/%s/%s/"`, d.w1.Surname, d.w1.Name))
	})
}

// Ported from Django: tests/admin_views/tests.py::AdminViewBasicTest.test_formset_kwargs_can_be_overridden
func TestFormsetKwargsCanBeOverridden(t *testing.T) {
	djangotest.EachDB(t, adminViewsModels(), func(t *testing.T, db *orm.DB) {
		env, _ := viewOnSiteEnv(t, db)
		resp := env.client.Get(mustReverse(t, env.router, "admin:admin_views_city_add"))
		djangotest.AssertContains(t, resp, "overridden_name")
	})
}
