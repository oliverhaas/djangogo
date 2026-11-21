package admin_test

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/admin"
	"github.com/oliverhaas/djangogo/auth"
	"github.com/oliverhaas/djangogo/contenttypes"
	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/messages"
	"github.com/oliverhaas/djangogo/orm"
)

// Models ported from tests/admin_changelist/models.py.

type CLParent struct {
	ID   int64
	Name string `orm:"max_length=128"`
}

func (p CLParent) String() string { return p.Name }

func (CLParent) Meta() orm.Meta {
	return orm.Meta{VerboseName: "parent", VerboseNamePlural: "parents"}
}

type CLChild struct {
	ID       int64
	Parent   orm.FK[CLParent] `orm:"null;blank;related_name=children;related_query_name=child"`
	Name     string           `orm:"max_length=30;blank"`
	Age      *int64           `orm:"null;blank"`
	IsActive bool
}

func (c CLChild) String() string { return c.Name }

func (CLChild) Meta() orm.Meta {
	return orm.Meta{VerboseName: "child", VerboseNamePlural: "children"}
}

type CLGrandChild struct {
	ID     int64
	Parent orm.FK[CLChild] `orm:"null;blank"`
	Name   string          `orm:"max_length=30;blank"`
}

func (g CLGrandChild) String() string { return g.Name }

func (CLGrandChild) Meta() orm.Meta {
	return orm.Meta{VerboseName: "grand child", VerboseNamePlural: "grand children"}
}

type CLGenre struct {
	ID   int64
	Name string `orm:"max_length=20"`
	URL  string `orm:"type=url;blank"`
}

func (g CLGenre) String() string { return g.Name }

func (CLGenre) Meta() orm.Meta {
	return orm.Meta{VerboseName: "genre", VerboseNamePlural: "genres"}
}

type CLBand struct {
	ID          int64
	Name        string           `orm:"max_length=20"`
	NrOfMembers int64            `orm:"verbose_name=number of members"`
	Genres      orm.M2M[CLGenre] `orm:"blank"`
}

func (b CLBand) String() string { return b.Name }

func (CLBand) Meta() orm.Meta {
	return orm.Meta{VerboseName: "band", VerboseNamePlural: "bands"}
}

type CLMusician struct {
	ID   int64
	Name string `orm:"max_length=30"`
	Age  *int64 `orm:"null;blank"`
}

func (m CLMusician) String() string { return m.Name }

type CLGroup struct {
	ID      int64
	Name    string              `orm:"max_length=30"`
	Members orm.M2M[CLMusician] `orm:"blank"`
}

func (g CLGroup) String() string { return g.Name }

func (CLGroup) Meta() orm.Meta {
	return orm.Meta{VerboseName: "group", VerboseNamePlural: "groups"}
}

type CLConcert struct {
	ID    int64
	Name  string          `orm:"max_length=30"`
	Group orm.FK[CLGroup] `orm:"related_query_name=concert"`
}

func (c CLConcert) String() string { return c.Name }

func (CLConcert) Meta() orm.Meta {
	return orm.Meta{VerboseName: "concert", VerboseNamePlural: "concerts"}
}

type CLSwallow struct {
	ID     int64
	Origin string `orm:"max_length=255"`
	Load   float64
	Speed  float64
}

func (s CLSwallow) String() string { return s.Origin }

func (CLSwallow) Meta() orm.Meta {
	return orm.Meta{
		Ordering:          []string{"speed", "load"},
		VerboseName:       "swallow",
		VerboseNamePlural: "swallows",
	}
}

type CLUnorderedObject struct {
	ID   int64
	Bool bool
}

func (u CLUnorderedObject) String() string { return fmt.Sprintf("UnorderedObject (%d)", u.ID) }

type CLOrderedObject struct {
	ID     int64
	Name   string `orm:"max_length=255;blank"`
	Bool   bool
	Number int64
}

func (o CLOrderedObject) String() string { return o.Name }

func (CLOrderedObject) Meta() orm.Meta {
	return orm.Meta{Ordering: []string{"number"}}
}

type CLQuartet struct {
	ID            int64
	Name          string `orm:"max_length=30;blank"`
	PlaysWeddings *bool  `orm:"null;blank"`
}

func (q CLQuartet) String() string { return q.Name }

type CLMixedFields struct {
	ID          int64
	Name        string `orm:"max_length=50;blank"`
	ChoiceField int64
}

func (m CLMixedFields) String() string { return m.Name }

func (CLMixedFields) Meta() orm.Meta {
	return orm.Meta{
		Choices: map[string][]orm.Choice{
			"ChoiceField": {
				orm.ChoiceOf(int64(1), "First Choice"),
				orm.ChoiceOf(int64(2), "Second Choice"),
			},
		},
	}
}

func changelistModels() []any {
	return []any{
		auth.App{},
		contenttypes.App{},
		admin.App{},
		djangotest.App("admin_changelist",
			&CLParent{},
			&CLChild{},
			&CLGrandChild{},
			&CLGenre{},
			&CLBand{},
			&CLMusician{},
			&CLGroup{},
			&CLConcert{},
			&CLSwallow{},
			&CLUnorderedObject{},
			&CLOrderedObject{},
			&CLQuartet{},
			&CLMixedFields{},
		),
	}
}

func int64Ptr(v int64) *int64 { return &v }
func boolPtr(v bool) *bool    { return &v }

// Ported from Django: tests/admin_changelist/tests.py::ChangeListTests.test_default_str_column_is_not_sortable
func TestDefaultStrColumnIsNotSortable(t *testing.T) {
	djangotest.EachDB(t, changelistModels(), func(t *testing.T, db *orm.DB) {
		super := createTestUser(t, db, "super", "secret", true, true)
		_ = orm.Query[CLGrandChild](db).Create(context.Background(), &CLGrandChild{Name: "Grandchild"})
		env := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
			admin.Register[CLGrandChild](site, admin.ModelAdmin{
				ListDisplay: []string{"__str__"},
			})
		})
		env.client.ForceLogin(super)

		resp := env.client.Get(mustReverse(t, env.router, "admin:admin_changelist_clgrandchild_changelist"))
		djangotest.AssertStatus(t, resp, http.StatusOK)
		djangotest.AssertContains(t, resp, `<th scope="col" class="column-__str__">`)
		djangotest.AssertNotContains(t, resp, `<th scope="col" class="sortable column-__str__">`)
	})
}

// Ported from Django: tests/admin_changelist/tests.py::ChangeListTests.test_many_search_terms
func TestManySearchTerms(t *testing.T) {
	djangotest.EachDB(t, changelistModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		super := createTestUser(t, db, "super", "secret", true, true)
		p := &CLParent{Name: "Mary"}
		_ = orm.Query[CLParent](db).Create(ctx, p)
		c1 := &CLChild{Name: "Danielle"}
		c1.Parent.SetPK(p.ID)
		c2 := &CLChild{Name: "Daniel"}
		c2.Parent.SetPK(p.ID)
		_ = orm.Query[CLChild](db).Create(ctx, c1)
		_ = orm.Query[CLChild](db).Create(ctx, c2)

		env := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
			admin.Register[CLParent](site, admin.ModelAdmin{
				ListDisplay:  []string{"name"},
				SearchFields: []string{"child__name"},
			})
		})
		env.client.ForceLogin(super)

		q := strings.Repeat("daniel ", 20)
		urlStr := mustReverse(t, env.router, "admin:admin_changelist_clparent_changelist")
		resp := env.client.Get(urlStr, djangotest.Query(url.Values{"q": {q}}))
		djangotest.AssertStatus(t, resp, http.StatusOK)
		djangotest.AssertEqual(t, ctxVal(t, resp, "result_count"), any(1))
	})
}

// Ported from Django: tests/admin_changelist/tests.py::ChangeListTests.test_related_field_multiple_search_terms
func TestRelatedFieldMultipleSearchTerms(t *testing.T) {
	djangotest.EachDB(t, changelistModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		super := createTestUser(t, db, "super", "secret", true, true)
		p := &CLParent{Name: "Mary"}
		_ = orm.Query[CLParent](db).Create(ctx, p)
		c1 := &CLChild{Name: "Danielle", Age: int64Ptr(18)}
		c1.Parent.SetPK(p.ID)
		c2 := &CLChild{Name: "Daniel", Age: int64Ptr(19)}
		c2.Parent.SetPK(p.ID)
		_ = orm.Query[CLChild](db).Create(ctx, c1)
		_ = orm.Query[CLChild](db).Create(ctx, c2)

		env := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
			admin.Register[CLParent](site, admin.ModelAdmin{
				ListDisplay:  []string{"name"},
				SearchFields: []string{"child__name", "child__age"},
			})
		})
		env.client.ForceLogin(super)

		urlStr := mustReverse(t, env.router, "admin:admin_changelist_clparent_changelist")
		resp := env.client.Get(urlStr, djangotest.Query(url.Values{"q": {"danielle 19"}}))
		djangotest.AssertEqual(t, ctxVal(t, resp, "result_count"), any(0))

		resp = env.client.Get(urlStr, djangotest.Query(url.Values{"q": {"daniel 19"}}))
		djangotest.AssertEqual(t, ctxVal(t, resp, "result_count"), any(1))
	})
}

// Ported from Django: tests/admin_changelist/tests.py::ChangeListTests.test_result_list_empty_changelist_value
// Ported from Django: tests/admin_changelist/tests.py::ChangeListTests.test_result_list_empty_changelist_value_blank_string
// Ported from Django: tests/admin_changelist/tests.py::ChangeListTests.test_result_list_set_empty_value_display_on_admin_site
// Ported from Django: tests/admin_changelist/tests.py::ChangeListTests.test_result_list_set_empty_value_display_in_model_admin
func TestResultListEmptyValueDisplay(t *testing.T) {
	djangotest.EachDB(t, changelistModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		super := createTestUser(t, db, "super", "secret", true, true)
		child := &CLChild{Name: ""}
		_ = orm.Query[CLChild](db).Create(ctx, child)

		// Default "-" for blank string and NULL FK.
		env := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
			admin.Register[CLChild](site, admin.ModelAdmin{
				ListDisplay: []string{"name", "parent"},
			})
		})
		env.client.ForceLogin(super)
		urlStr := mustReverse(t, env.router, "admin:admin_changelist_clchild_changelist")
		changeURL := mustReverse(t, env.router, "admin:admin_changelist_clchild_change", child.ID)
		resp := env.client.Get(urlStr)
		djangotest.AssertContains(t, resp, `<a href="`+changeURL+`">-</a>`)
		djangotest.AssertContains(t, resp, `<td class="field-parent nowrap">-</td>`)

		// Site-level EmptyValueDisplay.
		envSite := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
			site.EmptyValueDisplay = "???"
			admin.Register[CLChild](site, admin.ModelAdmin{
				ListDisplay: []string{"name", "parent"},
			})
		})
		envSite.client.ForceLogin(super)
		resp = envSite.client.Get(mustReverse(t, envSite.router, "admin:admin_changelist_clchild_changelist"))
		djangotest.AssertContains(t, resp, `<td class="field-parent nowrap">???</td>`)

		// ModelAdmin-level and per-column EmptyValueDisplay.
		envMA := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
			admin.Register[CLChild](site, admin.ModelAdmin{
				ListDisplay:       []string{"name", "age_display", "age"},
				EmptyValueDisplay: "-empty-",
				DisplayColumns: map[string]admin.DisplayColumn{
					"age_display": {
						Description: "Age display",
						EmptyValue:  "&dagger;",
						Value: func(obj any) any {
							c := obj.(*CLChild)
							if c.Age == nil {
								return nil
							}
							return *c.Age
						},
					},
				},
			})
		})
		envMA.client.ForceLogin(super)
		resp = envMA.client.Get(mustReverse(t, envMA.router, "admin:admin_changelist_clchild_changelist"))
		djangotest.AssertContains(t, resp, `<td class="field-age_display">&amp;dagger;</td>`)
		djangotest.AssertContains(t, resp, `<td class="field-age">-empty-</td>`)
	})
}

// Ported from Django: tests/admin_changelist/tests.py::ChangeListTests.test_result_list_html
func TestResultListHTML(t *testing.T) {
	djangotest.EachDB(t, changelistModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		super := createTestUser(t, db, "super", "secret", true, true)
		p := &CLParent{Name: "parent"}
		_ = orm.Query[CLParent](db).Create(ctx, p)
		c := &CLChild{Name: "name"}
		c.Parent.SetPK(p.ID)
		_ = orm.Query[CLChild](db).Create(ctx, c)

		env := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
			admin.Register[CLChild](site, admin.ModelAdmin{
				ListDisplay: []string{"name", "parent"},
			})
		})
		env.client.ForceLogin(super)

		resp := env.client.Get(mustReverse(t, env.router, "admin:admin_changelist_clchild_changelist"))
		djangotest.AssertStatus(t, resp, http.StatusOK)
		changeURL := mustReverse(t, env.router, "admin:admin_changelist_clchild_change", c.ID)
		djangotest.AssertContains(t, resp,
			`<input type="checkbox" id="action-toggle" aria-label="Select all objects on this page for an action">`,
			djangotest.HTML(),
		)
		djangotest.AssertContains(t, resp,
			`<td class="action-checkbox"><input type="checkbox" name="_selected_action" value="`+strconv.FormatInt(c.ID, 10)+`" class="action-select" aria-label="Select this object for an action - name"></td>`,
			djangotest.HTML(),
		)
		djangotest.AssertContains(t, resp,
			`<th class="field-name"><a href="`+changeURL+`">name</a></th>`,
			djangotest.HTML(),
		)
		djangotest.AssertContains(t, resp, `<td class="field-parent nowrap">parent</td>`, djangotest.HTML())
	})
}

// Ported from Django: tests/admin_changelist/tests.py::ChangeListTests.test_distinct_for_m2m_in_list_filter
// Ported from Django: tests/admin_changelist/tests.py::ChangeListTests.test_distinct_for_non_unique_related_object_in_list_filter
// Ported from Django: tests/admin_changelist/tests.py::ChangeListTests.test_distinct_for_non_unique_related_object_in_search_fields
// Ported from Django: tests/admin_changelist/tests.py::ChangeListTests.test_distinct_for_many_to_many_at_second_level_in_search_fields
func TestDistinctForMultiValuedRelations(t *testing.T) {
	djangotest.EachDB(t, changelistModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		super := createTestUser(t, db, "super", "secret", true, true)

		// Reverse FK duplicates in filter and search.
		p := &CLParent{Name: "Mary"}
		_ = orm.Query[CLParent](db).Create(ctx, p)
		c1 := &CLChild{Name: "Daniel"}
		c1.Parent.SetPK(p.ID)
		c2 := &CLChild{Name: "Danielle"}
		c2.Parent.SetPK(p.ID)
		c3 := &CLChild{Name: "Daniel"}
		c3.Parent.SetPK(p.ID)
		for _, c := range []*CLChild{c1, c2, c3} {
			_ = orm.Query[CLChild](db).Create(ctx, c)
		}

		// M2M at second level behind FK in search.
		vox := &CLMusician{Name: "Vox"}
		_ = orm.Query[CLMusician](db).Create(ctx, vox)
		vox2 := &CLMusician{Name: "Vox Junior"}
		_ = orm.Query[CLMusician](db).Create(ctx, vox2)
		grp := &CLGroup{Name: "The Hype"}
		_ = orm.Query[CLGroup](db).Create(ctx, grp)
		_ = orm.Many[CLMusician](db, grp, "members").Add(ctx, vox, vox2)
		concert := &CLConcert{Name: "Woodstock"}
		concert.Group.SetPK(grp.ID)
		_ = orm.Query[CLConcert](db).Create(ctx, concert)

		// M2M in list_filter.
		blues := &CLGenre{Name: "Blues"}
		_ = orm.Query[CLGenre](db).Create(ctx, blues)
		band := &CLBand{Name: "B.B. King Review", NrOfMembers: 11}
		_ = orm.Query[CLBand](db).Create(ctx, band)
		_ = orm.Many[CLGenre](db, band, "genres").Add(ctx, blues)

		env := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
			admin.Register[CLParent](site, admin.ModelAdmin{
				ListDisplay:  []string{"name"},
				ListFilter:   []any{"child__name"},
				SearchFields: []string{"child__name"},
			})
			admin.Register[CLConcert](site, admin.ModelAdmin{
				ListDisplay:  []string{"name"},
				SearchFields: []string{"group__members__name"},
			})
			admin.Register[CLBand](site, admin.ModelAdmin{
				ListDisplay: []string{"name"},
				ListFilter:  []any{"genres"},
			})
		})
		env.client.ForceLogin(super)

		// Reverse FK list_filter deduplicates.
		resp := env.client.Get(
			mustReverse(t, env.router, "admin:admin_changelist_clparent_changelist"),
			djangotest.Query(url.Values{"child__name": {"Daniel"}}),
		)
		djangotest.AssertEqual(t, ctxVal(t, resp, "result_count"), any(1))

		// Reverse FK search deduplicates.
		resp = env.client.Get(
			mustReverse(t, env.router, "admin:admin_changelist_clparent_changelist"),
			djangotest.Query(url.Values{"q": {"daniel"}}),
		)
		djangotest.AssertEqual(t, ctxVal(t, resp, "result_count"), any(1))

		// Second-level M2M behind FK search deduplicates.
		resp = env.client.Get(
			mustReverse(t, env.router, "admin:admin_changelist_clconcert_changelist"),
			djangotest.Query(url.Values{"q": {"vox"}}),
		)
		djangotest.AssertEqual(t, ctxVal(t, resp, "result_count"), any(1))

		// M2M filter returns 1 band.
		resp = env.client.Get(
			mustReverse(t, env.router, "admin:admin_changelist_clband_changelist"),
			djangotest.Query(url.Values{"genres__id__exact": {strconv.FormatInt(blues.ID, 10)}}),
		)
		djangotest.AssertEqual(t, ctxVal(t, resp, "result_count"), any(1))
	})
}

// Ported from Django: tests/admin_changelist/tests.py::ChangeListTests.test_changelist_search_form_validation
func TestChangelistSearchFormValidation(t *testing.T) {
	djangotest.EachDB(t, changelistModels(), func(t *testing.T, db *orm.DB) {
		super := createTestUser(t, db, "super", "secret", true, true)
		env := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
			admin.Register[CLConcert](site, admin.ModelAdmin{
				SearchFields: []string{"name"},
			})
		})
		env.client.ForceLogin(super)
		urlStr := mustReverse(t, env.router, "admin:admin_changelist_clconcert_changelist")

		for _, bad := range []string{"\x00", "some\x00thing"} {
			resp := env.client.Get(urlStr, djangotest.Query(url.Values{"q": {bad}}))
			djangotest.AssertStatus(t, resp, http.StatusOK)
			djangotest.AssertMessages(t, resp, []messages.Message{
				{Level: messages.LevelError, Message: "Null characters are not allowed."},
			})
		}
	})
}

// Ported from Django: tests/admin_changelist/tests.py::ChangeListTests.test_multiple_search_fields
// Ported from Django: tests/admin_changelist/tests.py::ChangeListTests.test_pk_in_search_fields
// Ported from Django: tests/admin_changelist/tests.py::ChangeListTests.test_builtin_lookup_in_search_fields
func TestSearchFieldsAndPrefixes(t *testing.T) {
	djangotest.EachDB(t, changelistModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		super := createTestUser(t, db, "super", "secret", true, true)

		duo := &CLGroup{Name: "Duo"}
		hype := &CLGroup{Name: "The Hype"}
		_ = orm.Query[CLGroup](db).Create(ctx, duo)
		_ = orm.Query[CLGroup](db).Create(ctx, hype)
		mary := &CLMusician{Name: "Mary Halvorson"}
		jonathan := &CLMusician{Name: "Jonathan Finlayson"}
		_ = orm.Query[CLMusician](db).Create(ctx, mary)
		_ = orm.Query[CLMusician](db).Create(ctx, jonathan)
		_ = orm.Many[CLMusician](db, duo, "members").Add(ctx, mary, jonathan)

		c1 := &CLConcert{Name: "Tiny desk concert"}
		c1.Group.SetPK(duo.ID)
		c2 := &CLConcert{Name: "Woodstock concert"}
		c2.Group.SetPK(hype.ID)
		_ = orm.Query[CLConcert](db).Create(ctx, c1)
		_ = orm.Query[CLConcert](db).Create(ctx, c2)

		env := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
			admin.Register[CLConcert](site, admin.ModelAdmin{
				SearchFields: []string{"group__name", "name"},
			})
			admin.Register[CLGroup](site, admin.ModelAdmin{
				SearchFields: []string{"name", "concert__name", "members__name"},
			})
		})
		env.client.ForceLogin(super)

		concertURL := mustReverse(t, env.router, "admin:admin_changelist_clconcert_changelist")
		groupURL := mustReverse(t, env.router, "admin:admin_changelist_clgroup_changelist")

		for _, tc := range []struct {
			q    string
			want int
		}{
			{"Duo Concert", 1},
			{"Tiny Desk Concert", 1},
			{"Concert", 2},
			{"Other Concert", 0},
			{"Duo Woodstock", 0},
		} {
			resp := env.client.Get(concertURL, djangotest.Query(url.Values{"q": {tc.q}}))
			djangotest.AssertEqual(t, ctxVal(t, resp, "result_count"), any(tc.want))

			resp = env.client.Get(groupURL, djangotest.Query(url.Values{"q": {tc.q}}))
			djangotest.AssertEqual(t, ctxVal(t, resp, "result_count"), any(tc.want))
		}

		for _, tc := range []struct {
			q    string
			want int
		}{
			{"Finlayson Duo Tiny", 1},
			{"Finlayson", 1},
			{"Finlayson Hype", 0},
			{"Jonathan Finlayson Duo", 1},
			{"Mary Jonathan Duo", 0},
			{"Oscar Finlayson Duo", 0},
		} {
			resp := env.client.Get(groupURL, djangotest.Query(url.Values{"q": {tc.q}}))
			djangotest.AssertEqual(t, ctxVal(t, resp, "result_count"), any(tc.want))
		}

		// Prefix "^" (istartswith) and "=" (iexact) and explicit lookup "__iexact" and "group__pk".
		envPrefixes := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
			admin.Register[CLConcert](site, admin.ModelAdmin{
				SearchFields: []string{"^name"},
			})
			admin.Register[CLGroup](site, admin.ModelAdmin{
				SearchFields: []string{"=name"},
			})
			admin.Register[CLMusician](site, admin.ModelAdmin{
				SearchFields: []string{"name__iexact"},
			})
		})
		envPrefixes.client.ForceLogin(super)

		// ^name matches "tiny" at start, not "desk".
		resp := envPrefixes.client.Get(
			mustReverse(t, envPrefixes.router, "admin:admin_changelist_clconcert_changelist"),
			djangotest.Query(url.Values{"q": {"tiny"}}),
		)
		djangotest.AssertEqual(t, ctxVal(t, resp, "result_count"), any(1))
		resp = envPrefixes.client.Get(
			mustReverse(t, envPrefixes.router, "admin:admin_changelist_clconcert_changelist"),
			djangotest.Query(url.Values{"q": {"desk"}}),
		)
		djangotest.AssertEqual(t, ctxVal(t, resp, "result_count"), any(0))

		// =name matches "duo" case-insensitively, not "du".
		resp = envPrefixes.client.Get(
			mustReverse(t, envPrefixes.router, "admin:admin_changelist_clgroup_changelist"),
			djangotest.Query(url.Values{"q": {"duo"}}),
		)
		djangotest.AssertEqual(t, ctxVal(t, resp, "result_count"), any(1))
		resp = envPrefixes.client.Get(
			mustReverse(t, envPrefixes.router, "admin:admin_changelist_clgroup_changelist"),
			djangotest.Query(url.Values{"q": {"du"}}),
		)
		djangotest.AssertEqual(t, ctxVal(t, resp, "result_count"), any(0))

		// name__iexact with quoted phrase matches full name, not substring.
		resp = envPrefixes.client.Get(
			mustReverse(t, envPrefixes.router, "admin:admin_changelist_clmusician_changelist"),
			djangotest.Query(url.Values{"q": {`"mary halvorson"`}}),
		)
		djangotest.AssertEqual(t, ctxVal(t, resp, "result_count"), any(1))
		resp = envPrefixes.client.Get(
			mustReverse(t, envPrefixes.router, "admin:admin_changelist_clmusician_changelist"),
			djangotest.Query(url.Values{"q": {"mary"}}),
		)
		djangotest.AssertEqual(t, ctxVal(t, resp, "result_count"), any(0))

		// group__pk search field.
		envPK := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
			admin.Register[CLConcert](site, admin.ModelAdmin{
				SearchFields: []string{"group__pk"},
			})
		})
		envPK.client.ForceLogin(super)
		pkURL := mustReverse(t, envPK.router, "admin:admin_changelist_clconcert_changelist")
		resp = envPK.client.Get(pkURL, djangotest.Query(url.Values{"q": {strconv.FormatInt(hype.ID, 10)}}))
		djangotest.AssertEqual(t, ctxVal(t, resp, "result_count"), any(1))
		resp = envPK.client.Get(pkURL, djangotest.Query(url.Values{"q": {strconv.FormatInt(hype.ID+50, 10)}}))
		djangotest.AssertEqual(t, ctxVal(t, resp, "result_count"), any(0))
	})
}

// Ported from Django: tests/admin_changelist/tests.py::ChangeListTests.test_exact_lookup_with_invalid_value
// Ported from Django: tests/admin_changelist/tests.py::ChangeListTests.test_exact_lookup_mixed_terms
// Ported from Django: tests/admin_changelist/tests.py::ChangeListTests.test_exact_boolean_lookup_is_case_insensitive
// Ported from Django: tests/admin_changelist/tests.py::ChangeListTests.test_exact_boolean_lookup_explicit_none
// Ported from Django: tests/admin_changelist/tests.py::ChangeListTests.test_exact_lookup_for_choices_field
// Ported from Django: tests/admin_changelist/tests.py::ChangeListTests.test_exact_lookup_for_boolean_field
// Ported from Django: tests/admin_changelist/tests.py::ChangeListTests.test_exact_lookup_for_null_boolean_field
// Ported from Django: tests/admin_changelist/tests.py::ChangeListTests.test_search_with_exact_lookup_relationship_field
func TestSearchExactLookupValidation(t *testing.T) {
	djangotest.EachDB(t, changelistModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		super := createTestUser(t, db, "super", "secret", true, true)

		child := &CLChild{Name: "foo123", Age: int64Ptr(123)}
		_ = orm.Query[CLChild](db).Create(ctx, child)
		_ = orm.Query[CLChild](db).Create(ctx, &CLChild{Name: "other", Age: int64Ptr(456)})

		relChild := &CLChild{Name: "I am a child", Age: int64Ptr(11)}
		_ = orm.Query[CLChild](db).Create(ctx, relChild)
		gc := &CLGrandChild{Name: "I am a grandchild"}
		gc.Parent.SetPK(relChild.ID)
		_ = orm.Query[CLGrandChild](db).Create(ctx, gc)

		_ = orm.Query[CLOrderedObject](db).Create(ctx, &CLOrderedObject{Name: "john", Bool: true, Number: 1})
		_ = orm.Query[CLOrderedObject](db).Create(ctx, &CLOrderedObject{Name: "mary", Bool: true, Number: 2})
		_ = orm.Query[CLOrderedObject](db).Create(ctx, &CLOrderedObject{Name: "pete", Bool: false, Number: 3})

		_ = orm.Query[CLQuartet](db).Create(ctx, &CLQuartet{Name: "qnull", PlaysWeddings: nil})
		_ = orm.Query[CLQuartet](db).Create(ctx, &CLQuartet{Name: "qfalse", PlaysWeddings: boolPtr(false)})
		_ = orm.Query[CLQuartet](db).Create(ctx, &CLQuartet{Name: "qtrue", PlaysWeddings: boolPtr(true)})

		_ = orm.Query[CLMixedFields](db).Create(ctx, &CLMixedFields{Name: "john", ChoiceField: 1})
		_ = orm.Query[CLMixedFields](db).Create(ctx, &CLMixedFields{Name: "mary", ChoiceField: 2})

		env := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
			admin.Register[CLChild](site, admin.ModelAdmin{
				SearchFields: []string{"name__icontains", "age__exact"},
			})
			admin.Register[CLGrandChild](site, admin.ModelAdmin{
				SearchFields: []string{"parent__name__exact", "parent__age__exact"},
			})
			admin.Register[CLOrderedObject](site, admin.ModelAdmin{
				SearchFields: []string{"name", "bool__exact"},
			})
			admin.Register[CLQuartet](site, admin.ModelAdmin{
				SearchFields: []string{"plays_weddings__exact"},
			})
			admin.Register[CLMixedFields](site, admin.ModelAdmin{
				ListDisplay:  []string{"name", "choice_field"},
				SearchFields: []string{"name", "choice_field__exact"},
			})
		})
		env.client.ForceLogin(super)

		// Mixed terms: "foo 123" matches foo123; "xyz" matches nothing.
		childURL := mustReverse(t, env.router, "admin:admin_changelist_clchild_changelist")
		resp := env.client.Get(childURL, djangotest.Query(url.Values{"q": {"foo 123"}}))
		djangotest.AssertEqual(t, ctxVal(t, resp, "result_count"), any(1))
		resp = env.client.Get(childURL, djangotest.Query(url.Values{"q": {"xyz"}}))
		djangotest.AssertEqual(t, ctxVal(t, resp, "result_count"), any(0))

		// Relationship exact lookup with quoted phrase and integer.
		gcURL := mustReverse(t, env.router, "admin:admin_changelist_clgrandchild_changelist")
		for _, tc := range []struct {
			q    string
			want int
		}{
			{"11", 1},
			{"'I am a child'", 1},
			{"1", 0},
			{"random", 0},
		} {
			resp = env.client.Get(gcURL, djangotest.Query(url.Values{"q": {tc.q}}))
			djangotest.AssertEqual(t, ctxVal(t, resp, "result_count"), any(tc.want))
		}

		// Non-nullable Boolean exact lookup.
		ordURL := mustReverse(t, env.router, "admin:admin_changelist_clorderedobject_changelist")
		for _, tc := range []struct {
			q    string
			want int
		}{
			{"john", 1},
			{"random", 0},
			{"true", 2},
			{"True", 2},
			{"1", 2},
			{"false", 1},
			{"FALSE", 1},
			{"0", 1},
			{"None", 0},
		} {
			resp = env.client.Get(ordURL, djangotest.Query(url.Values{"q": {tc.q}}))
			djangotest.AssertEqual(t, ctxVal(t, resp, "result_count"), any(tc.want))
		}

		// Nullable Boolean exact lookup including explicit "None".
		qURL := mustReverse(t, env.router, "admin:admin_changelist_clquartet_changelist")
		for _, tc := range []struct {
			q    string
			want int
		}{
			{"arbitrary", 0},
			{"FALSE", 1},
			{"None", 1},
		} {
			resp = env.client.Get(qURL, djangotest.Query(url.Values{"q": {tc.q}}))
			djangotest.AssertEqual(t, ctxVal(t, resp, "result_count"), any(tc.want))
		}

		// Choices field exact lookup and choice label display in list_display.
		mixURL := mustReverse(t, env.router, "admin:admin_changelist_clmixedfields_changelist")
		resp = env.client.Get(mixURL)
		djangotest.AssertContains(t, resp, "First Choice")
		djangotest.AssertContains(t, resp, "Second Choice")
		for _, tc := range []struct {
			q    string
			want int
		}{
			{"john", 1},
			{"1", 1},
			{"2", 1},
			{"random", 0},
		} {
			resp = env.client.Get(mixURL, djangotest.Query(url.Values{"q": {tc.q}}))
			djangotest.AssertEqual(t, ctxVal(t, resp, "result_count"), any(tc.want))
		}
	})
}

// Ported from Django: tests/admin_changelist/tests.py::ChangeListTests.test_pagination
// Ported from Django: tests/admin_changelist/tests.py::ChangeListTests.test_pagination_render
// Ported from Django: tests/admin_changelist/tests.py::ChangeListTests.test_show_all
func TestChangelistPaginationAndShowAll(t *testing.T) {
	djangotest.EachDB(t, changelistModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		super := createTestUser(t, db, "super", "secret", true, true)
		p := &CLParent{Name: "anything"}
		_ = orm.Query[CLParent](db).Create(ctx, p)
		for i := 1; i <= 15; i++ {
			c1 := &CLChild{Name: fmt.Sprintf("name %d", i)}
			c1.Parent.SetPK(p.ID)
			c2 := &CLChild{Name: fmt.Sprintf("filtered %d", i)}
			c2.Parent.SetPK(p.ID)
			_ = orm.Query[CLChild](db).Create(ctx, c1)
			_ = orm.Query[CLChild](db).Create(ctx, c2)
		}
		for i := 1; i <= 4; i++ {
			_ = orm.Query[CLSwallow](db).Create(ctx, &CLSwallow{
				Origin: fmt.Sprintf("Swallow %d", i),
				Load:   float64(i),
				Speed:  float64(i),
			})
		}

		env := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
			admin.Register[CLChild](site, admin.ModelAdmin{
				ListDisplay:    []string{"name"},
				ListPerPage:    10,
				ListMaxShowAll: 200,
			})
			admin.Register[CLSwallow](site, admin.ModelAdmin{
				ListDisplay: []string{"origin", "load", "speed"},
				ListPerPage: 3,
			})
		})
		env.client.ForceLogin(super)

		// Default queryset: 30 children, 3 pages of 10.
		childURL := mustReverse(t, env.router, "admin:admin_changelist_clchild_changelist")
		resp := env.client.Get(childURL)
		djangotest.AssertEqual(t, ctxVal(t, resp, "result_count"), any(30))
		djangotest.AssertEqual(t, ctxVal(t, resp, "page_count"), any(10))
		djangotest.AssertContains(t, resp, `30 children`)
		djangotest.AssertContains(t, resp, `class="showall"`)

		// Show all within ListMaxShowAll returns all 30.
		resp = env.client.Get(childURL, djangotest.Query(url.Values{"all": {""}}))
		djangotest.AssertEqual(t, ctxVal(t, resp, "page_count"), any(30))

		// Invalid page number redirects to ?e=1.
		resp = env.client.Get(childURL, djangotest.Query(url.Values{"p": {"-1"}}))
		djangotest.AssertRedirects(t, resp, childURL+"?e=1", djangotest.FetchRedirectResponse(false))

		// Show all over ListMaxShowAll falls back to paginated page.
		envSmallMax := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
			admin.Register[CLChild](site, admin.ModelAdmin{
				ListDisplay:    []string{"name"},
				ListPerPage:    10,
				ListMaxShowAll: 15,
			})
		})
		envSmallMax.client.ForceLogin(super)
		resp = envSmallMax.client.Get(
			mustReverse(t, envSmallMax.router, "admin:admin_changelist_clchild_changelist"),
			djangotest.Query(url.Values{"all": {""}}),
		)
		djangotest.AssertEqual(t, ctxVal(t, resp, "page_count"), any(10))

		// Custom queryset filter reduces total count and pages.
		envFiltered := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
			admin.Register[CLChild](site, admin.ModelAdmin{
				ListDisplay: []string{"name"},
				ListPerPage: 10,
				GetQuerysetFilter: func(*http.Request) orm.Cond {
					return orm.Q("name__contains", "filtered")
				},
			})
		})
		envFiltered.client.ForceLogin(super)
		resp = envFiltered.client.Get(mustReverse(t, envFiltered.router, "admin:admin_changelist_clchild_changelist"))
		djangotest.AssertEqual(t, ctxVal(t, resp, "result_count"), any(15))
		djangotest.AssertEqual(t, ctxVal(t, resp, "page_count"), any(10))

		// Pagination HTML markup matches Django's paginator.
		swallowURL := mustReverse(t, env.router, "admin:admin_changelist_clswallow_changelist")
		resp = env.client.Get(swallowURL)
		djangotest.AssertContains(t, resp, `<nav class="paginator" aria-labelledby="pagination">`)
		djangotest.AssertContains(t, resp, `<h2 id="pagination" class="visually-hidden">Pagination swallows</h2>`)
		djangotest.AssertContains(t, resp, `<li><a role="button" href="" aria-current="page">1</a></li>`, djangotest.HTML())
		djangotest.AssertContains(t, resp, `<li><a role="button" href="?p=2">2</a></li>`, djangotest.HTML())
	})
}

// Ported from Django: tests/admin_changelist/tests.py::ChangeListTests.test_dynamic_list_display
// Ported from Django: tests/admin_changelist/tests.py::ChangeListTests.test_dynamic_list_display_links
// Ported from Django: tests/admin_changelist/tests.py::ChangeListTests.test_no_list_display_links
// Ported from Django: tests/admin_changelist/tests.py::ChangeListTests.test_link_field_display_links
func TestListDisplayAndDisplayLinks(t *testing.T) {
	djangotest.EachDB(t, changelistModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		super := createTestUser(t, db, "super", "secret", true, true)
		noparents := createTestUser(t, db, "noparents", "secret", true, true)

		p := &CLParent{Name: "Test Parent"}
		_ = orm.Query[CLParent](db).Create(ctx, p)
		c := &CLChild{Name: "child 1", Age: int64Ptr(7)}
		c.Parent.SetPK(p.ID)
		_ = orm.Query[CLChild](db).Create(ctx, c)
		g := &CLGenre{Name: "Blues", URL: "http://blues_history.com"}
		_ = orm.Query[CLGenre](db).Create(ctx, g)

		env := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
			admin.Register[CLChild](site, admin.ModelAdmin{
				GetListDisplay: func(r *http.Request) []string {
					u, _ := auth.GetUser(r)
					if u != nil && u.Username == "noparents" {
						return []string{"name", "age"}
					}
					return []string{"parent", "name", "age"}
				},
				GetListDisplayLinks: func(_ *http.Request, _ []string) []string {
					return []string{"age"}
				},
			})
			admin.Register[CLParent](site, admin.ModelAdmin{
				ListDisplay:      []string{"name"},
				ListDisplayLinks: []string{}, // No links.
			})
			admin.Register[CLGenre](site, admin.ModelAdmin{
				ListDisplay:      []string{"name", "url"},
				ListDisplayLinks: []string{"name", "url"},
			})
		})

		// Dynamic list_display hides parent column for "noparents", shows for "super".
		childURL := mustReverse(t, env.router, "admin:admin_changelist_clchild_changelist")
		childChangeURL := mustReverse(t, env.router, "admin:admin_changelist_clchild_change", c.ID)
		env.client.ForceLogin(noparents)
		resp := env.client.Get(childURL)
		djangotest.AssertNotContains(t, resp, "Test Parent")
		djangotest.AssertContains(t, resp, `<a href="`+childChangeURL+`">7</a>`)

		env.client.ForceLogin(super)
		resp = env.client.Get(childURL)
		djangotest.AssertContains(t, resp, "Test Parent")
		djangotest.AssertContains(t, resp, `<a href="`+childChangeURL+`">7</a>`)

		// No list_display_links renders no change link.
		parentChangeURL := mustReverse(t, env.router, "admin:admin_changelist_clparent_change", p.ID)
		resp = env.client.Get(mustReverse(t, env.router, "admin:admin_changelist_clparent_changelist"))
		djangotest.AssertNotContains(t, resp, `<a href="`+parentChangeURL+`">`)

		// URL field inside list_display_links links to change page, not nested <a>.
		genreChangeURL := mustReverse(t, env.router, "admin:admin_changelist_clgenre_change", g.ID)
		resp = env.client.Get(mustReverse(t, env.router, "admin:admin_changelist_clgenre_changelist"))
		djangotest.AssertContains(t, resp, `<a href="`+genreChangeURL+`">http://blues_history.com</a>`)
	})
}

// Ported from Django: tests/admin_changelist/tests.py::ChangeListTests.test_clear_all_filters_link
// Ported from Django: tests/admin_changelist/tests.py::ChangeListTests.test_clear_all_filters_link_callable_filter
// Ported from Django: tests/admin_changelist/tests.py::ChangeListTests.test_no_clear_all_filters_link
func TestClearAllFiltersLink(t *testing.T) {
	djangotest.EachDB(t, changelistModels(), func(t *testing.T, db *orm.DB) {
		super := createTestUser(t, db, "super", "secret", true, true)
		env := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
			admin.Register[auth.User](site, admin.ModelAdmin{
				ListDisplay:  []string{"username", "is_staff"},
				ListFilter:   []any{"is_staff"},
				SearchFields: []string{"username"},
			})
			admin.Register[CLBand](site, admin.ModelAdmin{
				ListDisplay: []string{"name", "nr_of_members"},
				ListFilter: []any{
					admin.SimpleListFilter{
						Title:         "number of members",
						ParameterName: "nr_of_members_partition",
						Lookups: func(*http.Request) []admin.FilterLookup {
							return []admin.FilterLookup{
								{Value: "5", Display: "0 - 5"},
								{Value: "more", Display: "more than 5"},
							}
						},
						Filter: func(_ *http.Request, val string, selected bool) (orm.Cond, bool) {
							if !selected {
								return orm.Cond{}, false
							}
							if val == "5" {
								return orm.Q("nr_of_members__lte", 5), true
							}
							return orm.Q("nr_of_members__gt", 5), true
						},
					},
				},
			})
		})
		env.client.ForceLogin(super)

		userURL := mustReverse(t, env.router, "admin:auth_user_changelist")
		resp := env.client.Get(userURL)
		djangotest.AssertNotContains(t, resp, "&#10006; Clear all filters")

		for _, tc := range []struct {
			q    url.Values
			href string
		}{
			{url.Values{"is_staff__exact": {"0"}}, "?"},
			{url.Values{"is_staff__exact": {"0"}, "username__startswith": {"test"}}, "?username__startswith=test"},
			{url.Values{"is_staff__exact": {"0"}, "q": {"test"}}, "?q=test"},
			{url.Values{"is_staff__exact": {"0"}, "_popup": {"id"}}, "?_popup=id"},
		} {
			resp = env.client.Get(userURL, djangotest.Query(tc.q))
			djangotest.AssertContains(t, resp, `<a href="`+tc.href+`">&#10006; Clear all filters</a>`)
		}

		// Non-filter params do not trigger Clear all filters link.
		for _, q := range []url.Values{
			{"q": {"test"}},
			{"o": {"-1"}},
			{"p": {"1"}},
			{"_popup": {"1"}},
			{"username__startswith": {"test"}},
		} {
			resp = env.client.Get(userURL, djangotest.Query(q))
			djangotest.AssertNotContains(t, resp, `>&#10006; Clear all filters</a>`)
		}

		// Callable SimpleListFilter triggers Clear all filters link too.
		bandURL := mustReverse(t, env.router, "admin:admin_changelist_clband_changelist")
		resp = env.client.Get(bandURL, djangotest.Query(url.Values{"nr_of_members_partition": {"5"}}))
		djangotest.AssertContains(t, resp, `<a href="?">&#10006; Clear all filters</a>`)
	})
}

// Ported from Django: tests/admin_changelist/tests.py::ChangeListTests.test_deterministic_order_for_unordered_model
// Ported from Django: tests/admin_changelist/tests.py::ChangeListTests.test_ordering_from_model_meta
func TestChangelistOrderingPrecedenceAndDeterministicPK(t *testing.T) {
	djangotest.EachDB(t, changelistModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		super := createTestUser(t, db, "super", "secret", true, true)

		var uIDs []int64
		for i := 1; i <= 15; i++ {
			u := &CLUnorderedObject{Bool: true}
			_ = orm.Query[CLUnorderedObject](db).Create(ctx, u)
			uIDs = append(uIDs, u.ID)
		}

		_ = orm.Query[CLSwallow](db).Create(ctx, &CLSwallow{Origin: "Swallow A", Load: 4, Speed: 2})
		_ = orm.Query[CLSwallow](db).Create(ctx, &CLSwallow{Origin: "Swallow B", Load: 2, Speed: 1})
		_ = orm.Query[CLSwallow](db).Create(ctx, &CLSwallow{Origin: "Swallow C", Load: 5, Speed: 1})

		// Unordered model defaults to -pk across pages.
		env := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
			admin.Register[CLUnorderedObject](site, admin.ModelAdmin{
				ListDisplay: []string{"id", "bool"},
				ListPerPage: 10,
			})
			admin.Register[CLSwallow](site, admin.ModelAdmin{
				ListDisplay: []string{"origin", "load", "speed"},
			})
		})
		env.client.ForceLogin(super)

		uoURL := mustReverse(t, env.router, "admin:admin_changelist_clunorderedobject_changelist")
		resp1 := env.client.Get(uoURL, djangotest.Query(url.Values{"p": {"1"}}))
		resp2 := env.client.Get(uoURL, djangotest.Query(url.Values{"p": {"2"}}))
		pks1 := ctxVal(t, resp1, "result_pks").([]int64)
		pks2 := ctxVal(t, resp2, "result_pks").([]int64)
		combined := append(pks1, pks2...)
		wantDesc := make([]int64, len(uIDs))
		for i := range uIDs {
			wantDesc[i] = uIDs[len(uIDs)-1-i]
		}
		djangotest.AssertSequenceEqual(t, combined, wantDesc)

		// Meta.Ordering ("speed", "load") applies when ModelAdmin.Ordering is empty.
		swallowURL := mustReverse(t, env.router, "admin:admin_changelist_clswallow_changelist")
		resp := env.client.Get(swallowURL, djangotest.Query(url.Values{"o": {""}}))
		body := string(resp.Body)
		idxB := strings.Index(body, "Swallow B")
		idxC := strings.Index(body, "Swallow C")
		idxA := strings.Index(body, "Swallow A")
		if idxB < 0 || idxB >= idxC || idxC >= idxA {
			t.Fatalf("Meta.Ordering expected Swallow B < Swallow C < Swallow A, got indices %d, %d, %d", idxB, idxC, idxA)
		}

		// ?o=-1 sorts by column 1 (origin) descending (column 0 is action_checkbox).
		resp = env.client.Get(swallowURL, djangotest.Query(url.Values{"o": {"-1"}}))
		body = string(resp.Body)
		idxC = strings.Index(body, "Swallow C")
		idxB = strings.Index(body, "Swallow B")
		idxA = strings.Index(body, "Swallow A")
		if idxC < 0 || idxC >= idxB || idxB >= idxA {
			t.Fatalf("?o=-1 expected Swallow C < Swallow B < Swallow A, got indices %d, %d, %d", idxC, idxB, idxA)
		}
	})
}

// Ported from Django: tests/admin_changelist/tests.py::ChangeListTests.test_object_tools_displayed_no_add_permission
// Ported from Django: tests/admin_changelist/tests.py::ChangeListTests.test_search_help_text
// Ported from Django: tests/admin_changelist/tests.py::ChangeListTests.test_search_role
// Ported from Django: tests/admin_changelist/tests.py::ChangeListTests.test_search_bar_total_link_preserves_options
func TestSearchFormMarkupAndObjectTools(t *testing.T) {
	djangotest.EachDB(t, changelistModels(), func(t *testing.T, db *orm.DB) {
		super := createTestUser(t, db, "super", "secret", true, true)
		env := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
			admin.Register[CLBand](site, admin.ModelAdmin{
				SearchFields:     []string{"name"},
				SearchHelpText:   "Search help text",
				HasAddPermission: func(*http.Request) bool { return false },
			})
			admin.Register[auth.User](site, admin.ModelAdmin{
				SearchFields: []string{"username"},
				ListFilter:   []any{"is_staff"},
			})
		})
		env.client.ForceLogin(super)

		bandURL := mustReverse(t, env.router, "admin:admin_changelist_clband_changelist")
		resp := env.client.Get(bandURL)
		djangotest.AssertContains(t, resp, `<ul class="object-tools">`)
		djangotest.AssertNotContains(t, resp, "Add band")
		djangotest.AssertContains(t, resp, `<div class="help" id="searchbar_helptext">Search help text</div>`)
		djangotest.AssertContains(t, resp, `aria-describedby="searchbar_helptext"`)
		djangotest.AssertContains(t, resp, `<h2 id="changelist-search-form" class="visually-hidden">Search bands</h2>`)
		djangotest.AssertContains(t, resp, `<form id="changelist-search" method="get" role="search" aria-labelledby="changelist-search-form">`)

		// Result count in search bar when filtered.
		userURL := mustReverse(t, env.router, "admin:auth_user_changelist")
		resp = env.client.Get(userURL, djangotest.Query(url.Values{"is_staff__exact": {"0"}}))
		djangotest.AssertContains(t, resp, `0 results (<a href="?">1 total</a>)`)

		resp = env.client.Get(userURL, djangotest.Query(url.Values{"is_staff__exact": {"0"}, "_popup": {"1"}}))
		djangotest.AssertContains(t, resp, `0 results (<a href="?_popup=1">1 total</a>)`)
	})
}

// Ported from Django: tests/admin_changelist/tests.py::ChangeListTests.test_list_display_related_field
// Ported from Django: tests/admin_changelist/tests.py::ChangeListTests.test_list_display_related_field_null
// Ported from Django: tests/admin_changelist/tests.py::ChangeListTests.test_list_display_related_field_ordering
// Ported from Django: tests/admin_changelist/tests.py::ChangeListTests.test_list_display_related_field_boolean_display
func TestListDisplayRelatedFieldAndOrdering(t *testing.T) {
	djangotest.EachDB(t, changelistModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		super := createTestUser(t, db, "super", "secret", true, true)

		pa := &CLParent{Name: "Alice"}
		pz := &CLParent{Name: "Zara"}
		_ = orm.Query[CLParent](db).Create(ctx, pa)
		_ = orm.Query[CLParent](db).Create(ctx, pz)

		ca := &CLChild{Name: "Alice's child", IsActive: true}
		ca.Parent.SetPK(pa.ID)
		cz := &CLChild{Name: "Zara's child", IsActive: false}
		cz.Parent.SetPK(pz.ID)
		_ = orm.Query[CLChild](db).Create(ctx, ca)
		_ = orm.Query[CLChild](db).Create(ctx, cz)

		gc1 := &CLGrandChild{Name: "GC Active"}
		gc1.Parent.SetPK(ca.ID)
		gc2 := &CLGrandChild{Name: "GC Inactive"}
		gc2.Parent.SetPK(cz.ID)
		gcNull := &CLGrandChild{Name: "GC Parentless"}
		for _, g := range []*CLGrandChild{gc1, gc2, gcNull} {
			_ = orm.Query[CLGrandChild](db).Create(ctx, g)
		}

		env := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
			admin.Register[CLGrandChild](site, admin.ModelAdmin{
				ListDisplay: []string{"name", "parent__name", "parent__parent__name", "parent__is_active"},
			})
			admin.Register[CLChild](site, admin.ModelAdmin{
				ListDisplay: []string{"name", "parent__name"},
				ListPerPage: 1,
			})
		})
		env.client.ForceLogin(super)

		gcURL := mustReverse(t, env.router, "admin:admin_changelist_clgrandchild_changelist")
		resp := env.client.Get(gcURL)
		djangotest.AssertContains(t, resp, "Alice")
		djangotest.AssertContains(t, resp, "Alice&#39;s child")
		djangotest.AssertContains(t, resp, `<th scope="col" class="sortable column-parent__name">`)
		djangotest.AssertContains(t, resp, `<th scope="col" class="sortable column-parent__parent__name">`)
		djangotest.AssertContains(t, resp, `<td class="field-parent__name">-</td>`)
		djangotest.AssertContains(t, resp, `<td class="field-parent__parent__name">-</td>`)
		djangotest.AssertContains(t, resp, `alt="True"`)
		djangotest.AssertContains(t, resp, `alt="False"`)
		djangotest.AssertNotContains(t, resp, ">True<")
		djangotest.AssertNotContains(t, resp, ">False<")

		// Sort ascending and descending by related column parent__name (index 2 with action checkbox).
		childURL := mustReverse(t, env.router, "admin:admin_changelist_clchild_changelist")
		resp = env.client.Get(childURL, djangotest.Query(url.Values{"o": {"2"}}))
		djangotest.AssertContains(t, resp, "Alice")
		djangotest.AssertNotContains(t, resp, "Zara")

		resp = env.client.Get(childURL, djangotest.Query(url.Values{"o": {"-2"}}))
		djangotest.AssertNotContains(t, resp, "Alice")
		djangotest.AssertContains(t, resp, "Zara")
	})
}

// Ported from Django: tests/admin_changelist/tests.py::ChangeListTests.test_pagination_page_range
func TestPaginationElidedPageRange(t *testing.T) {
	djangotest.EachDB(t, changelistModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		super := createTestUser(t, db, "super", "secret", true, true)
		for i := 1; i <= 15; i++ {
			_ = orm.Query[CLGroup](db).Create(ctx, &CLGroup{Name: fmt.Sprintf("Band %02d", i)})
		}
		env := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
			admin.Register[CLGroup](site, admin.ModelAdmin{
				ListDisplay: []string{"name"},
				ListPerPage: 1,
			})
		})
		env.client.ForceLogin(super)
		grpURL := mustReverse(t, env.router, "admin:admin_changelist_clgroup_changelist")

		// Page 8 of 15 elides both sides: [1, 2, …, 5..11, …, 14, 15].
		resp := env.client.Get(grpURL, djangotest.Query(url.Values{"p": {"8"}}))
		djangotest.AssertStatus(t, resp, http.StatusOK)
		djangotest.AssertContains(t, resp, "<li>… </li>", djangotest.Count(2))
		djangotest.AssertContains(t, resp, `<a role="button" href="" aria-current="page">8</a>`, djangotest.HTML())
		djangotest.AssertContains(t, resp, `<a role="button" href="?p=1">1</a>`, djangotest.HTML())
		djangotest.AssertContains(t, resp, `<a role="button" href="?p=15">15</a>`, djangotest.HTML())
		djangotest.AssertNotContains(t, resp, `href="?p=3"`)
		djangotest.AssertNotContains(t, resp, `href="?p=13"`)
	})
}

// TestColumnSortingMultiToggleAndSortableBy verifies multi-column ?o= sorting,
// sortpriority/sortremove/toggle links, SortableBy restrictions, and custom
// DisplayColumn sorting via OrderField.
func TestColumnSortingMultiToggleAndSortableBy(t *testing.T) {
	djangotest.EachDB(t, changelistModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		super := createTestUser(t, db, "super", "secret", true, true)
		s1 := &CLSwallow{Origin: "B", Load: 10, Speed: 1}
		s2 := &CLSwallow{Origin: "A", Load: 20, Speed: 1}
		s3 := &CLSwallow{Origin: "C", Load: 5, Speed: 2}
		for _, s := range []*CLSwallow{s1, s2, s3} {
			_ = orm.Query[CLSwallow](db).Create(ctx, s)
		}

		env := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
			admin.Register[CLSwallow](site, admin.ModelAdmin{
				ListDisplay: []string{"origin", "speed", "load_display"},
				SortableBy:  []string{"origin", "speed", "load_display"},
				DisplayColumns: map[string]admin.DisplayColumn{
					"load_display": {
						Description: "Load (kg)",
						OrderField:  "load",
						Value: func(obj any) any {
							return obj.(*CLSwallow).Load
						},
					},
				},
			})
		})
		env.client.ForceLogin(super)
		swURL := mustReverse(t, env.router, "admin:admin_changelist_clswallow_changelist")

		// Multi-column sort: speed asc (col 2), then load_display desc (col -3).
		resp := env.client.Get(swURL, djangotest.Query(url.Values{"o": {"2.-3"}}))
		djangotest.AssertStatus(t, resp, http.StatusOK)
		djangotest.AssertEqual(t, ctxVal(t, resp, "result_pks"), any([]int64{s2.ID, s1.ID, s3.ID}))
		djangotest.AssertContains(t, resp, `class="sortable column-speed sorted ascending"`)
		djangotest.AssertContains(t, resp, `class="sortable column-load_display sorted descending"`)
		djangotest.AssertContains(t, resp, `<span class="sortpriority" title="Sorting priority: 1">1</span>`)
		djangotest.AssertContains(t, resp, `<span class="sortpriority" title="Sorting priority: 2">2</span>`)
		djangotest.AssertContains(t, resp, `class="sortremove" href="?o=-3"`)
		djangotest.AssertContains(t, resp, `class="sortremove" href="?o=2"`)
		djangotest.AssertContains(t, resp, `href="?o=-2.-3" class="toggle ascending"`)
		djangotest.AssertContains(t, resp, `href="?o=2.3" class="toggle descending"`)

		// SortableBy empty slice disables sorting on all columns.
		envNoSort := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
			admin.Register[CLSwallow](site, admin.ModelAdmin{
				ListDisplay: []string{"origin", "speed"},
				SortableBy:  []string{},
			})
		})
		envNoSort.client.ForceLogin(super)
		resp = envNoSort.client.Get(mustReverse(t, envNoSort.router, "admin:admin_changelist_clswallow_changelist"))
		djangotest.AssertNotContains(t, resp, `class="sortable`)
	})
}
