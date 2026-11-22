package admin_test

import (
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/admin"
	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
)

// Thing stands in for tests/admin_views/models.py::Thing.
type Thing struct {
	ID      int64
	Title   string        `orm:"max_length=20"`
	Color   orm.FK[Color] `orm:"on_delete=cascade"`
	PubDate *orm.Date     `orm:"null;blank"`
}

// String returns the title.
func (t Thing) String() string { return t.Title }

// Meta limits the colors of a thing to the warm ones.
func (Thing) Meta() orm.Meta {
	return orm.Meta{LimitChoicesTo: map[string]orm.Cond{"Color": orm.Q("warm", true)}}
}

// Ported from Django: tests/admin_views/tests.py::AdminViewBasicTest.test_limited_filter
//
// Admin changelist filters do not contain objects excluded via
// limit_choices_to.
func TestLimitedFilter(t *testing.T) {
	models := append(adminViewsModels(), djangotest.App("admin_views", &Color{}, &Thing{}))
	djangotest.EachDB(t, models, func(t *testing.T, db *orm.DB) {
		env := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
			admin.Register[Thing](site, admin.ModelAdmin{
				ListFilter: []any{"color", "color__warm", "color__value", "pub_date"},
			})
		})
		env.client.ForceLogin(createTestUser(t, db, "super", "secret", true, true))
		for _, c := range []*Color{
			{Value: "Red", Warm: true},
			{Value: "Orange", Warm: true},
			{Value: "Blue"},
			{Value: "Green"},
		} {
			create(t, db, c)
		}
		resp := env.client.Get(mustReverse(t, env.router, "admin:admin_views_thing_changelist"))
		djangotest.AssertContains(t, resp,
			`<search id="changelist-filter" aria-labelledby="changelist-filter-header">`)
		djangotest.AssertNotContains(t, resp, `<a href="?color__id__exact=3">Blue</a>`)
		// No Django test checks that a warm color is listed.
		djangotest.AssertContains(t, resp, `<a href="?color__id__exact=1">Red</a>`)
	})
}

// Character stands in for tests/admin_views/models.py::Character.
type Character struct {
	ID         int64
	Username   string `orm:"max_length=100"`
	LastAction time.Time
}

// String returns the user name.
func (c Character) String() string { return c.Username }

// StumpJoke stands in for tests/admin_views/models.py::StumpJoke.
type StumpJoke struct {
	ID                 int64
	Variation          string             `orm:"max_length=100"`
	MostRecentlyFooled orm.FK[Character]  `orm:"on_delete=cascade;related_name=+"`
	HasFooledToday     orm.M2M[Character] `orm:"related_name=+"`
}

// String returns the variation.
func (j StumpJoke) String() string { return j.Variation }

// Meta limits both relations to the characters whose last action is not in
// the past, Django's today_callable_dict and today_callable_q.
func (StumpJoke) Meta() orm.Meta {
	today := func() orm.Cond { return orm.Q("last_action__gte", time.Now()) }
	return orm.Meta{LimitChoicesToFunc: map[string]func() orm.Cond{
		"MostRecentlyFooled": today,
		"HasFooledToday":     today,
	}}
}

// Ported from Django: tests/admin_views/tests.py::LimitChoicesToInAdminTest.test_limit_choices_to_as_callable
func TestLimitChoicesToAsCallableInAdmin(t *testing.T) {
	models := append(adminViewsModels(), djangotest.App("admin_views", &Character{}, &StumpJoke{}))
	djangotest.EachDB(t, models, func(t *testing.T, db *orm.DB) {
		env := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
			admin.Register[StumpJoke](site, admin.ModelAdmin{})
		})
		env.client.ForceLogin(createTestUser(t, db, "super", "secret", true, true))
		now := time.Now().UTC()
		threepwood := create(t, db, &Character{Username: "threepwood", LastAction: now.AddDate(0, 0, 1)})
		marley := create(t, db, &Character{Username: "marley", LastAction: now.AddDate(0, 0, -1)})
		resp := env.client.Get(mustReverse(t, env.router, "admin:admin_views_stumpjoke_add"))
		// The allowed option should appear twice; the limited option should
		// not appear.
		djangotest.AssertContains(t, resp, threepwood.Username, djangotest.Count(2))
		djangotest.AssertNotContains(t, resp, marley.Username)
	})
}
