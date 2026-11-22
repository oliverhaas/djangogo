package admin_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/oliverhaas/djangogo/admin"
	"github.com/oliverhaas/djangogo/auth"
	"github.com/oliverhaas/djangogo/contenttypes"
	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
)

// AOBand is tests/admin_ordering/models.py::Band.
type AOBand struct {
	ID   int64
	Name string `orm:"max_length=100"`
	Bio  string `orm:"type=text"`
	Rank int
}

// Meta orders by name.
func (AOBand) Meta() orm.Meta { return orm.Meta{Ordering: []string{"name"}} }

// String returns the name.
func (b AOBand) String() string { return b.Name }

// AOSong is tests/admin_ordering/models.py::Song.
type AOSong struct {
	ID                int64
	Band              orm.FK[AOBand] `orm:"on_delete=cascade"`
	Name              string         `orm:"max_length=100"`
	Duration          int
	OtherInterpreters orm.M2M[AOBand] `orm:"related_name=covers"`
}

// Meta orders by name.
func (AOSong) Meta() orm.Meta { return orm.Meta{Ordering: []string{"name"}} }

// adminOrderingModels returns the models of Django's admin_ordering tests.
func adminOrderingModels() []any {
	return []any{auth.App{}, contenttypes.App{}, admin.App{}, djangotest.App("admin_ordering", &AOBand{}, &AOSong{})}
}

// aoBandNames returns the names of the bands that the ModelAdmin ma of AOBand
// gives r, Django's [b.name for b in ma.get_queryset(request)].
func aoBandNames(t *testing.T, db *orm.DB, ma admin.ModelAdmin, r *http.Request) []string {
	t.Helper()
	site, err := admin.NewAdminSite(db)
	if err != nil {
		t.Fatalf("NewAdminSite: %v", err)
	}
	admin.Register[AOBand](site, ma)
	rows, err := admin.Queryset(context.Background(), site, "admin_ordering", "aoband", r)
	if err != nil {
		t.Fatalf("Queryset: %v", err)
	}
	names := make([]string, len(rows))
	for i, row := range rows {
		names[i] = row.(*AOBand).Name
	}
	return names
}

// createAOBands creates the bands of TestAdminOrdering.setUpTestData.
func createAOBands(t *testing.T, db *orm.DB) {
	t.Helper()
	mustCreate(t, db, &AOBand{Name: "Aerosmith", Rank: 3})
	mustCreate(t, db, &AOBand{Name: "Radiohead", Rank: 1})
	mustCreate(t, db, &AOBand{Name: "Van Halen", Rank: 2})
}

// Ported from Django: tests/admin_ordering/tests.py::TestAdminOrdering.test_default_ordering
func TestAdminOrderingDefaultOrdering(t *testing.T) {
	djangotest.EachDB(t, adminOrderingModels(), func(t *testing.T, db *orm.DB) {
		createAOBands(t, db)
		names := aoBandNames(t, db, admin.ModelAdmin{}, httptest.NewRequest(http.MethodGet, "/", nil))
		djangotest.AssertEqual(t, names, []string{"Aerosmith", "Radiohead", "Van Halen"})
	})
}

// Ported from Django: tests/admin_ordering/tests.py::TestAdminOrdering.test_specified_ordering
func TestAdminOrderingSpecifiedOrdering(t *testing.T) {
	djangotest.EachDB(t, adminOrderingModels(), func(t *testing.T, db *orm.DB) {
		createAOBands(t, db)
		names := aoBandNames(t, db, admin.ModelAdmin{Ordering: []string{"rank"}}, httptest.NewRequest(http.MethodGet, "/", nil))
		djangotest.AssertEqual(t, names, []string{"Radiohead", "Van Halen", "Aerosmith"})
	})
}

// Ported from Django: tests/admin_ordering/tests.py::TestAdminOrdering.test_specified_ordering_by_f_expression
func TestAdminOrderingSpecifiedOrderingByFExpression(t *testing.T) {
	djangotest.EachDB(t, adminOrderingModels(), func(t *testing.T, db *orm.DB) {
		createAOBands(t, db)
		ma := admin.ModelAdmin{OrderingExprs: []any{orm.F("rank").Desc().NullsLast()}}
		names := aoBandNames(t, db, ma, httptest.NewRequest(http.MethodGet, "/", nil))
		djangotest.AssertEqual(t, names, []string{"Aerosmith", "Van Halen", "Radiohead"})
	})
}

// Ported from Django: tests/admin_ordering/tests.py::TestAdminOrdering.test_dynamic_ordering
func TestAdminOrderingDynamicOrdering(t *testing.T) {
	djangotest.EachDB(t, adminOrderingModels(), func(t *testing.T, db *orm.DB) {
		createAOBands(t, db)
		superUser := mustCreate(t, db, &auth.User{Username: "admin", IsSuperuser: true})
		otherUser := mustCreate(t, db, &auth.User{Username: "other"})
		// DynOrderingBandAdmin of tests/admin_ordering/models.py.
		ma := admin.ModelAdmin{GetOrdering: func(r *http.Request) []string {
			if u, _ := auth.GetUser(r); u.IsSuperuser {
				return []string{"rank"}
			}
			return []string{"name"}
		}}
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		names := aoBandNames(t, db, ma, r.WithContext(auth.WithUser(r.Context(), superUser)))
		djangotest.AssertEqual(t, names, []string{"Radiohead", "Van Halen", "Aerosmith"})
		names = aoBandNames(t, db, ma, r.WithContext(auth.WithUser(r.Context(), otherUser)))
		djangotest.AssertEqual(t, names, []string{"Aerosmith", "Radiohead", "Van Halen"})
	})
}

// aoInlineSongNames returns the names of the songs that the inline of the
// change page of band lists, Django's [s.name for s in
// inline.get_queryset(request)] for the songs of band.
func aoInlineSongNames(t *testing.T, db *orm.DB, inline admin.InlineModelAdmin, band *AOBand) []string {
	t.Helper()
	env := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
		admin.Register[AOBand](site, admin.ModelAdmin{Inlines: []admin.Inline{admin.StackedInline[AOBand, AOSong](inline)}})
	})
	env.client.ForceLogin(createTestUser(t, db, "super", "secret", true, true))
	resp := env.client.Get(mustReverse(t, env.router, "admin:admin_ordering_aoband_change", band.ID))
	djangotest.AssertStatus(t, resp, http.StatusOK)
	matches := regexp.MustCompile(`name="[a-z_]+-\d+-name"[^>]*?value="([^"]*)"`).FindAllStringSubmatch(string(resp.Body), -1)
	names := make([]string, len(matches))
	for i, m := range matches {
		names[i] = m[1]
	}
	return names
}

// createAOSongs creates the band and the songs of
// TestInlineModelAdminOrdering.setUpTestData.
func createAOSongs(t *testing.T, db *orm.DB) *AOBand {
	t.Helper()
	band := mustCreate(t, db, &AOBand{Name: "Aerosmith", Rank: 3})
	for _, s := range []AOSong{
		{Name: "Pink", Duration: 235},
		{Name: "Dude (Looks Like a Lady)", Duration: 264},
		{Name: "Jaded", Duration: 214},
	} {
		s.Band.SetPK(band.ID)
		mustCreate(t, db, &s)
	}
	return band
}

// Ported from Django: tests/admin_ordering/tests.py::TestInlineModelAdminOrdering.test_default_ordering
func TestInlineModelAdminOrderingDefaultOrdering(t *testing.T) {
	djangotest.EachDB(t, adminOrderingModels(), func(t *testing.T, db *orm.DB) {
		band := createAOSongs(t, db)
		names := aoInlineSongNames(t, db, admin.InlineModelAdmin{}, band)
		djangotest.AssertEqual(t, names, []string{"Dude (Looks Like a Lady)", "Jaded", "Pink"})
	})
}

// Ported from Django: tests/admin_ordering/tests.py::TestInlineModelAdminOrdering.test_specified_ordering
func TestInlineModelAdminOrderingSpecifiedOrdering(t *testing.T) {
	djangotest.EachDB(t, adminOrderingModels(), func(t *testing.T, db *orm.DB) {
		band := createAOSongs(t, db)
		names := aoInlineSongNames(t, db, admin.InlineModelAdmin{Ordering: []string{"duration"}}, band)
		djangotest.AssertEqual(t, names, []string{"Jaded", "Pink", "Dude (Looks Like a Lady)"})
	})
}

// checkOrderingOfFieldChoices checks that the band and other_interpreters
// fields of the add page of AOSong offer the bands in the order of want, with
// AOSong registered under songAdmin and AOBand under bandAdmin when it is not
// nil, Django's TestRelatedFieldsAdminOrdering.check_ordering_of_field_choices.
func checkOrderingOfFieldChoices(t *testing.T, db *orm.DB, songAdmin admin.ModelAdmin, bandAdmin *admin.ModelAdmin, want ...*AOBand) {
	t.Helper()
	env := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
		admin.Register[AOSong](site, songAdmin)
		if bandAdmin != nil {
			admin.Register[AOBand](site, *bandAdmin)
		}
	})
	env.client.ForceLogin(createTestUser(t, db, "super", "secret", true, true))
	resp := env.client.Get(mustReverse(t, env.router, "admin:admin_ordering_aosong_add"))
	djangotest.AssertStatus(t, resp, http.StatusOK)
	choices := make([]string, len(want))
	for i, b := range want {
		choices[i] = fmt.Sprintf("%d:%s", b.ID, b.Name)
	}
	djangotest.AssertEqual(t, pageChoices(t, resp, "band"), append([]string{":---------"}, choices...))
	djangotest.AssertEqual(t, pageChoices(t, resp, "other_interpreters"), choices)
}

// createAORelatedBands creates the bands of
// TestRelatedFieldsAdminOrdering.setUpTestData.
func createAORelatedBands(t *testing.T, db *orm.DB) (b1, b2 *AOBand) {
	t.Helper()
	return mustCreate(t, db, &AOBand{Name: "Pink Floyd", Rank: 1}), mustCreate(t, db, &AOBand{Name: "Foo Fighters", Rank: 5})
}

// Ported from Django: tests/admin_ordering/tests.py::TestRelatedFieldsAdminOrdering.test_no_admin_fallback_to_model_ordering
func TestRelatedFieldsAdminOrderingNoAdminFallbackToModelOrdering(t *testing.T) {
	djangotest.EachDB(t, adminOrderingModels(), func(t *testing.T, db *orm.DB) {
		b1, b2 := createAORelatedBands(t, db)
		checkOrderingOfFieldChoices(t, db, admin.ModelAdmin{}, nil, b2, b1)
	})
}

// Ported from Django: tests/admin_ordering/tests.py::TestRelatedFieldsAdminOrdering.test_admin_with_no_ordering_fallback_to_model_ordering
func TestRelatedFieldsAdminOrderingAdminWithNoOrderingFallbackToModelOrdering(t *testing.T) {
	djangotest.EachDB(t, adminOrderingModels(), func(t *testing.T, db *orm.DB) {
		b1, b2 := createAORelatedBands(t, db)
		checkOrderingOfFieldChoices(t, db, admin.ModelAdmin{}, &admin.ModelAdmin{}, b2, b1)
	})
}

// Ported from Django: tests/admin_ordering/tests.py::TestRelatedFieldsAdminOrdering.test_admin_ordering_beats_model_ordering
func TestRelatedFieldsAdminOrderingAdminOrderingBeatsModelOrdering(t *testing.T) {
	djangotest.EachDB(t, adminOrderingModels(), func(t *testing.T, db *orm.DB) {
		b1, b2 := createAORelatedBands(t, db)
		checkOrderingOfFieldChoices(t, db, admin.ModelAdmin{}, &admin.ModelAdmin{Ordering: []string{"rank"}}, b1, b2)
	})
}

// Ported from Django: tests/admin_ordering/tests.py::TestRelatedFieldsAdminOrdering.test_custom_queryset_still_wins
func TestRelatedFieldsAdminOrderingCustomQuerysetStillWins(t *testing.T) {
	djangotest.EachDB(t, adminOrderingModels(), func(t *testing.T, db *orm.DB) {
		_, b2 := createAORelatedBands(t, db)
		songAdmin := admin.ModelAdmin{
			// Exclude one of the two bands from the querysets.
			FormfieldQuerySet: func(_ *http.Request, f *orm.Field) orm.AnyQuerySet {
				if f.Attr == "band" || f.Attr == "other_interpreters" {
					return orm.Query[AOBand](db).Filter("rank__gt", 2)
				}
				return nil
			},
		}
		checkOrderingOfFieldChoices(t, db, songAdmin, &admin.ModelAdmin{Ordering: []string{"rank"}}, b2)
	})
}
