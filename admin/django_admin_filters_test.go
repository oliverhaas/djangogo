package admin_test

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/admin"
	"github.com/oliverhaas/djangogo/auth"
	"github.com/oliverhaas/djangogo/contenttypes"
	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
)

// Models ported from tests/admin_filters/models.py.

type AFDepartment struct {
	ID          int64
	Code        string `orm:"max_length=4;unique"`
	Description string `orm:"max_length=50;blank"`
}

func (d AFDepartment) String() string { return d.Description }

type AFEmployee struct {
	ID         int64
	Name       string `orm:"max_length=100"`
	Department orm.FK[AFDepartment]
}

func (e AFEmployee) String() string { return e.Name }

func (AFEmployee) Meta() orm.Meta {
	return orm.Meta{Ordering: []string{"name"}}
}

type AFBook struct {
	ID             int64
	Title          string             `orm:"max_length=50"`
	Year           *int64             `orm:"null;blank"`
	Author         orm.FK[auth.User]  `orm:"null;blank;verbose_name=Verbose Author;related_name=books_authored"`
	Contributors   orm.M2M[auth.User] `orm:"blank;verbose_name=Verbose Contributors;related_name=books_contributed"`
	Employee       orm.FK[AFEmployee] `orm:"null;blank"`
	IsBestSeller   *bool              `orm:"null;blank"`
	DateRegistered *orm.Date          `orm:"null;blank"`
	Category       *string            `orm:"max_length=20;null;blank"`
	Status         string             `orm:"max_length=20;blank"`
	Availability   *bool              `orm:"null;blank"`
}

func (b AFBook) String() string { return b.Title }

func (AFBook) Meta() orm.Meta {
	return orm.Meta{
		VerboseName:       "book",
		VerboseNamePlural: "books",
		Choices: map[string][]orm.Choice{
			"Status": {
				orm.ChoiceOf("draft", "Draft"),
				orm.ChoiceOf("pub", "Published"),
			},
			"Availability": {
				orm.ChoiceOf(false, "Paid"),
				orm.ChoiceOf(true, "Free"),
				orm.ChoiceOf(nil, "Obscure"),
			},
			"Category": {
				orm.ChoiceOf("non-fiction", "Non-Fictional"),
				orm.ChoiceOf("fiction", "Fictional"),
				orm.ChoiceOf(nil, "Not categorized"),
				orm.ChoiceOf("", "We don't know"),
			},
		},
	}
}

func adminFilterModels() []any {
	return []any{
		auth.App{},
		contenttypes.App{},
		admin.App{},
		djangotest.App("admin_filters",
			&AFDepartment{},
			&AFEmployee{},
			&AFBook{},
		),
	}
}

type afFixture struct {
	today      orm.Date
	tomorrow   orm.Date
	oneWeekAgo orm.Date
	alfred     *auth.User
	bob        *auth.User
	lisa       *auth.User
	dev        *AFDepartment
	design     *AFDepartment
	john       *AFEmployee
	jack       *AFEmployee
	djangonaut *AFBook
	bio        *AFBook
	djangoBook *AFBook
	guitar     *AFBook
}

func setupAdminFiltersFixture(t *testing.T, db *orm.DB) *afFixture {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	today := orm.Date{Year: now.Year(), Month: now.Month(), Day: now.Day()}
	tomorrowTime := now.AddDate(0, 0, 1)
	tomorrow := orm.Date{Year: tomorrowTime.Year(), Month: tomorrowTime.Month(), Day: tomorrowTime.Day()}
	weekAgoTime := now.AddDate(0, 0, -7)
	oneWeekAgo := orm.Date{Year: weekAgoTime.Year(), Month: weekAgoTime.Month(), Day: weekAgoTime.Day()}

	alfred := createTestUser(t, db, "alfred", "password", true, true)
	alfred.Email = "alfred@example.com"
	_ = orm.Query[auth.User](db).Save(ctx, alfred)
	bob := createTestUser(t, db, "bob", "password", false, false)
	bob.Email = "bob@example.com"
	_ = orm.Query[auth.User](db).Save(ctx, bob)
	lisa := createTestUser(t, db, "lisa", "password", false, false)
	lisa.Email = "lisa@example.com"
	_ = orm.Query[auth.User](db).Save(ctx, lisa)

	dev := &AFDepartment{Code: "DEV", Description: "Development"}
	design := &AFDepartment{Code: "DSN", Description: "Design"}
	_ = orm.Query[AFDepartment](db).Create(ctx, dev)
	_ = orm.Query[AFDepartment](db).Create(ctx, design)

	john := &AFEmployee{Name: "John Blue"}
	john.Department.SetPK(dev.ID)
	jack := &AFEmployee{Name: "Jack Red"}
	jack.Department.SetPK(design.ID)
	_ = orm.Query[AFEmployee](db).Create(ctx, john)
	_ = orm.Query[AFEmployee](db).Create(ctx, jack)

	dToday := today
	dWeek := oneWeekAgo
	b1 := &AFBook{
		Title:          "Djangonaut: an art of living",
		Year:           int64Ptr(2009),
		IsBestSeller:   boolPtr(true),
		DateRegistered: &dToday,
		Availability:   boolPtr(true),
		Category:       strPtr("non-fiction"),
		Status:         "pub",
	}
	b1.Author.SetPK(alfred.ID)
	b1.Employee.SetPK(john.ID)
	_ = orm.Query[AFBook](db).Create(ctx, b1)

	b2 := &AFBook{
		Title:        "Django: a biography",
		Year:         int64Ptr(1999),
		IsBestSeller: boolPtr(false),
		Availability: boolPtr(false),
		Category:     strPtr("fiction"),
		Status:       "draft",
	}
	b2.Author.SetPK(alfred.ID)
	b2.Employee.SetPK(john.ID)
	_ = orm.Query[AFBook](db).Create(ctx, b2)

	b3 := &AFBook{
		Title:          "The Django Book",
		Year:           nil,
		IsBestSeller:   nil,
		DateRegistered: &dToday,
		Availability:   boolPtr(true),
		Status:         "pub",
	}
	b3.Author.SetPK(bob.ID)
	b3.Employee.SetPK(jack.ID)
	_ = orm.Query[AFBook](db).Create(ctx, b3)

	b4 := &AFBook{
		Title:          "Guitar for dummies",
		Year:           int64Ptr(2002),
		IsBestSeller:   boolPtr(true),
		DateRegistered: &dWeek,
		Availability:   nil,
		Category:       strPtr(""),
		Status:         "draft",
	}
	_ = orm.Query[AFBook](db).Create(ctx, b4)
	_ = orm.Many[auth.User](db, b4, "contributors").Add(ctx, bob, lisa)

	return &afFixture{
		today:      today,
		tomorrow:   tomorrow,
		oneWeekAgo: oneWeekAgo,
		alfred:     alfred,
		bob:        bob,
		lisa:       lisa,
		dev:        dev,
		design:     design,
		john:       john,
		jack:       jack,
		djangonaut: b1,
		bio:        b2,
		djangoBook: b3,
		guitar:     b4,
	}
}

func bookAdminDefault() admin.ModelAdmin {
	return admin.ModelAdmin{
		ListDisplay: []string{"title", "year", "author", "status"},
		ListFilter: []any{
			"year",
			"author",
			"contributors",
			"is_best_seller",
			"date_registered",
			"status",
			"availability",
		},
		Ordering: []string{"-id"},
	}
}

// Ported from Django: tests/admin_filters/tests.py::ListFiltersTests.test_booleanfieldlistfilter
// Ported from Django: tests/admin_filters/tests.py::ListFiltersTests.test_booleanfieldlistfilter_choices
func TestBooleanFieldListFilter(t *testing.T) {
	djangotest.EachDB(t, adminFilterModels(), func(t *testing.T, db *orm.DB) {
		f := setupAdminFiltersFixture(t, db)
		env := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
			admin.Register[AFBook](site, bookAdminDefault())
		})
		env.client.ForceLogin(f.alfred)
		bookURL := mustReverse(t, env.router, "admin:admin_filters_afbook_changelist")

		// is_best_seller__exact=0 -> only bio_book.
		resp := env.client.Get(bookURL, djangotest.Query(url.Values{"is_best_seller__exact": {"0"}}))
		djangotest.AssertEqual(t, ctxVal(t, resp, "result_pks"), any([]int64{f.bio.ID}))
		djangotest.AssertContains(t, resp, `<li class="selected"><a href="?is_best_seller__exact=0">No</a></li>`, djangotest.HTML())

		// is_best_seller__exact=1 -> guitar, djangonaut.
		resp = env.client.Get(bookURL, djangotest.Query(url.Values{"is_best_seller__exact": {"1"}}))
		djangotest.AssertEqual(t, ctxVal(t, resp, "result_pks"), any([]int64{f.guitar.ID, f.djangonaut.ID}))
		djangotest.AssertContains(t, resp, `<li class="selected"><a href="?is_best_seller__exact=1">Yes</a></li>`, djangotest.HTML())

		// is_best_seller__isnull=True -> django_book.
		resp = env.client.Get(bookURL, djangotest.Query(url.Values{"is_best_seller__isnull": {"True"}}))
		djangotest.AssertEqual(t, ctxVal(t, resp, "result_pks"), any([]int64{f.djangoBook.ID}))
		djangotest.AssertContains(t, resp, `<li class="selected"><a href="?is_best_seller__isnull=True">Unknown</a></li>`, djangotest.HTML())

		// Boolean with custom choices (Paid / Free / Obscure).
		resp = env.client.Get(bookURL, djangotest.Query(url.Values{"availability__exact": {"0"}}))
		djangotest.AssertEqual(t, ctxVal(t, resp, "result_pks"), any([]int64{f.bio.ID}))
		djangotest.AssertContains(t, resp, `<li class="selected"><a href="?availability__exact=0">Paid</a></li>`, djangotest.HTML())

		resp = env.client.Get(bookURL, djangotest.Query(url.Values{"availability__exact": {"1"}}))
		djangotest.AssertEqual(t, ctxVal(t, resp, "result_pks"), any([]int64{f.djangoBook.ID, f.djangonaut.ID}))
		djangotest.AssertContains(t, resp, `<li class="selected"><a href="?availability__exact=1">Free</a></li>`, djangotest.HTML())

		resp = env.client.Get(bookURL, djangotest.Query(url.Values{"availability__isnull": {"True"}}))
		djangotest.AssertEqual(t, ctxVal(t, resp, "result_pks"), any([]int64{f.guitar.ID}))
		djangotest.AssertContains(t, resp, `<li class="selected"><a href="?availability__isnull=True">Obscure</a></li>`, djangotest.HTML())
	})
}

// Ported from Django: tests/admin_filters/tests.py::ListFiltersTests.test_choicesfieldlistfilter_has_none_choice
func TestChoicesFieldListFilter(t *testing.T) {
	djangotest.EachDB(t, adminFilterModels(), func(t *testing.T, db *orm.DB) {
		f := setupAdminFiltersFixture(t, db)
		env := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
			admin.Register[AFBook](site, bookAdminDefault())
		})
		env.client.ForceLogin(f.alfred)
		bookURL := mustReverse(t, env.router, "admin:admin_filters_afbook_changelist")

		resp := env.client.Get(bookURL, djangotest.Query(url.Values{"status__exact": {"pub"}}))
		djangotest.AssertEqual(t, ctxVal(t, resp, "result_pks"), any([]int64{f.djangoBook.ID, f.djangonaut.ID}))
		djangotest.AssertContains(t, resp, `<li class="selected"><a href="?status__exact=pub">Published</a></li>`, djangotest.HTML())
		djangotest.AssertContains(t, resp, `<a href="?status__exact=draft">Draft</a>`)
	})
}

// Ported from Django: tests/admin_filters/tests.py::ListFiltersTests.test_datefieldlistfilter
func TestDateFieldListFilter(t *testing.T) {
	djangotest.EachDB(t, adminFilterModels(), func(t *testing.T, db *orm.DB) {
		f := setupAdminFiltersFixture(t, db)
		env := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
			admin.Register[AFBook](site, bookAdminDefault())
		})
		env.client.ForceLogin(f.alfred)
		bookURL := mustReverse(t, env.router, "admin:admin_filters_afbook_changelist")

		// Today.
		resp := env.client.Get(bookURL, djangotest.Query(url.Values{
			"date_registered__gte": {f.today.String()},
			"date_registered__lt":  {f.tomorrow.String()},
		}))
		djangotest.AssertEqual(t, ctxVal(t, resp, "result_pks"), any([]int64{f.djangoBook.ID, f.djangonaut.ID}))
		djangotest.AssertContains(t, resp, `>Today</a>`)

		// Past 7 days.
		resp = env.client.Get(bookURL, djangotest.Query(url.Values{
			"date_registered__gte": {f.oneWeekAgo.String()},
			"date_registered__lt":  {f.tomorrow.String()},
		}))
		djangotest.AssertEqual(t, ctxVal(t, resp, "result_pks"), any([]int64{f.guitar.ID, f.djangoBook.ID, f.djangonaut.ID}))

		// No date (isnull=True).
		resp = env.client.Get(bookURL, djangotest.Query(url.Values{"date_registered__isnull": {"True"}}))
		djangotest.AssertEqual(t, ctxVal(t, resp, "result_pks"), any([]int64{f.bio.ID}))
		djangotest.AssertContains(t, resp, `<li class="selected"><a href="?date_registered__isnull=True">No date</a></li>`, djangotest.HTML())

		// Has date (isnull=False).
		resp = env.client.Get(bookURL, djangotest.Query(url.Values{"date_registered__isnull": {"False"}}))
		djangotest.AssertEqual(t, ctxVal(t, resp, "result_pks"), any([]int64{f.guitar.ID, f.djangoBook.ID, f.djangonaut.ID}))
		djangotest.AssertContains(t, resp, `<li class="selected"><a href="?date_registered__isnull=False">Has date</a></li>`, djangotest.HTML())
	})
}

// Ported from Django: tests/admin_filters/tests.py::ListFiltersTests.test_allvaluesfieldlistfilter
// Ported from Django: tests/admin_filters/tests.py::ListFiltersTests.test_allvaluesfieldlistfilter_custom_qs
func TestAllValuesFieldListFilter(t *testing.T) {
	djangotest.EachDB(t, adminFilterModels(), func(t *testing.T, db *orm.DB) {
		f := setupAdminFiltersFixture(t, db)
		env := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
			admin.Register[AFBook](site, bookAdminDefault())
		})
		env.client.ForceLogin(f.alfred)
		bookURL := mustReverse(t, env.router, "admin:admin_filters_afbook_changelist")

		resp := env.client.Get(bookURL, djangotest.Query(url.Values{"year__isnull": {"True"}}))
		djangotest.AssertEqual(t, ctxVal(t, resp, "result_pks"), any([]int64{f.djangoBook.ID}))
		djangotest.AssertContains(t, resp, `<li class="selected"><a href="?year__isnull=True">-</a></li>`, djangotest.HTML())

		resp = env.client.Get(bookURL, djangotest.Query(url.Values{"year": {"2002"}}))
		djangotest.AssertEqual(t, ctxVal(t, resp, "result_pks"), any([]int64{f.guitar.ID}))
		djangotest.AssertContains(t, resp, `<li class="selected"><a href="?year=2002">2002</a></li>`, djangotest.HTML())

		// Custom base queryset restricts displayed year choices to alfred's books (1999, 2009).
		envCustom := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
			admin.Register[AFBook](site, admin.ModelAdmin{
				ListFilter: []any{"year"},
				GetQuerysetFilter: func(*http.Request) orm.Cond {
					return orm.Q("author", f.alfred.ID)
				},
			})
		})
		envCustom.client.ForceLogin(f.alfred)
		resp = envCustom.client.Get(mustReverse(t, envCustom.router, "admin:admin_filters_afbook_changelist"))
		djangotest.AssertContains(t, resp, `?year=1999`)
		djangotest.AssertContains(t, resp, `?year=2009`)
		djangotest.AssertNotContains(t, resp, `?year=2002`)
	})
}

// Ported from Django: tests/admin_filters/tests.py::ListFiltersTests.test_relatedfieldlistfilter_foreignkey
// Ported from Django: tests/admin_filters/tests.py::ListFiltersTests.test_relatedfieldlistfilter_foreignkey_ordering
// Ported from Django: tests/admin_filters/tests.py::ListFiltersTests.test_relatedfieldlistfilter_foreignkey_ordering_reverse
// Ported from Django: tests/admin_filters/tests.py::ListFiltersTests.test_relatedfieldlistfilter_foreignkey_default_ordering
// Ported from Django: tests/admin_filters/tests.py::ListFiltersTests.test_relatedfieldlistfilter_manytomany
// Ported from Django: tests/admin_filters/tests.py::ListFiltersTests.test_relatedfieldlistfilter_reverse_relationships
func TestRelatedFieldListFilter(t *testing.T) {
	djangotest.EachDB(t, adminFilterModels(), func(t *testing.T, db *orm.DB) {
		f := setupAdminFiltersFixture(t, db)
		env := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
			admin.Register[AFBook](site, bookAdminDefault())
			admin.Register[AFEmployee](site, admin.ModelAdmin{
				Ordering: []string{"-name"},
			})
			admin.Register[auth.User](site, admin.ModelAdmin{
				ListFilter: []any{"books_authored", "books_contributed"},
				Ordering:   []string{"-id"},
			})
		})
		env.client.ForceLogin(f.alfred)
		bookURL := mustReverse(t, env.router, "admin:admin_filters_afbook_changelist")

		// FK isnull=True -> guitar.
		resp := env.client.Get(bookURL, djangotest.Query(url.Values{"author__isnull": {"True"}}))
		djangotest.AssertEqual(t, ctxVal(t, resp, "result_pks"), any([]int64{f.guitar.ID}))
		djangotest.AssertContains(t, resp, `By Verbose Author`)
		djangotest.AssertContains(t, resp, `<li class="selected"><a href="?author__isnull=True">-</a></li>`, djangotest.HTML())

		// FK exact -> alfred's books.
		alfredPK := strconv.FormatInt(f.alfred.ID, 10)
		resp = env.client.Get(bookURL, djangotest.Query(url.Values{"author__id__exact": {alfredPK}}))
		djangotest.AssertEqual(t, ctxVal(t, resp, "result_pks"), any([]int64{f.bio.ID, f.djangonaut.ID}))
		djangotest.AssertContains(t, resp, `<li class="selected"><a href="?author__id__exact=`+alfredPK+`">alfred</a></li>`, djangotest.HTML())

		// M2M isnull=True -> books without contributors.
		resp = env.client.Get(bookURL, djangotest.Query(url.Values{"contributors__isnull": {"True"}}))
		djangotest.AssertEqual(t, ctxVal(t, resp, "result_pks"), any([]int64{f.djangoBook.ID, f.bio.ID, f.djangonaut.ID}))
		djangotest.AssertContains(t, resp, `By Verbose Contributors`)

		// M2M exact -> guitar_book.
		bobPK := strconv.FormatInt(f.bob.ID, 10)
		resp = env.client.Get(bookURL, djangotest.Query(url.Values{"contributors__id__exact": {bobPK}}))
		djangotest.AssertEqual(t, ctxVal(t, resp, "result_pks"), any([]int64{f.guitar.ID}))

		// Reverse FK & reverse M2M on User.
		userURL := mustReverse(t, env.router, "admin:auth_user_changelist")
		resp = env.client.Get(userURL, djangotest.Query(url.Values{"books_authored__isnull": {"True"}}))
		djangotest.AssertEqual(t, ctxVal(t, resp, "result_pks"), any([]int64{f.lisa.ID}))

		resp = env.client.Get(userURL, djangotest.Query(url.Values{"books_contributed__isnull": {"True"}}))
		djangotest.AssertEqual(t, ctxVal(t, resp, "result_pks"), any([]int64{f.alfred.ID}))

		// Related ordering respects related ModelAdmin.Ordering (-name puts John Blue before Jack Red).
		envOrder := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
			admin.Register[AFEmployee](site, admin.ModelAdmin{Ordering: []string{"-name"}})
			admin.Register[AFBook](site, admin.ModelAdmin{ListFilter: []any{"employee"}})
		})
		envOrder.client.ForceLogin(f.alfred)
		resp = envOrder.client.Get(mustReverse(t, envOrder.router, "admin:admin_filters_afbook_changelist"))
		idxJohn := indexOf(string(resp.Body), "John Blue")
		idxJack := indexOf(string(resp.Body), "Jack Red")
		if idxJohn < 0 || idxJohn >= idxJack {
			t.Fatalf("expected John Blue before Jack Red under reverse ordering, got %d, %d", idxJohn, idxJack)
		}

		// Without related ModelAdmin, respects Meta.Ordering (name puts Jack Red before John Blue).
		envMetaOrder := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
			admin.Register[AFBook](site, admin.ModelAdmin{ListFilter: []any{"employee"}})
		})
		envMetaOrder.client.ForceLogin(f.alfred)
		resp = envMetaOrder.client.Get(mustReverse(t, envMetaOrder.router, "admin:admin_filters_afbook_changelist"))
		idxJohn = indexOf(string(resp.Body), "John Blue")
		idxJack = indexOf(string(resp.Body), "Jack Red")
		if idxJack < 0 || idxJack >= idxJohn {
			t.Fatalf("expected Jack Red before John Blue under Meta.Ordering, got %d, %d", idxJack, idxJohn)
		}
	})
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}

// Ported from Django: tests/admin_filters/tests.py::ListFiltersTests.test_relatedonlyfieldlistfilter_foreignkey
// Ported from Django: tests/admin_filters/tests.py::ListFiltersTests.test_relatedonlyfieldlistfilter_manytomany
func TestRelatedOnlyFieldListFilter(t *testing.T) {
	djangotest.EachDB(t, adminFilterModels(), func(t *testing.T, db *orm.DB) {
		f := setupAdminFiltersFixture(t, db)
		env := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
			admin.Register[AFBook](site, admin.ModelAdmin{
				ListFilter: []any{
					admin.RelatedOnlyFilter("author"),
					admin.RelatedOnlyFilter("contributors"),
				},
			})
		})
		env.client.ForceLogin(f.alfred)
		resp := env.client.Get(mustReverse(t, env.router, "admin:admin_filters_afbook_changelist"))
		// Lisa authored no books, so she must NOT appear under author filter.
		djangotest.AssertContains(t, resp, "?author__id__exact="+strconv.FormatInt(f.alfred.ID, 10))
		djangotest.AssertContains(t, resp, "?author__id__exact="+strconv.FormatInt(f.bob.ID, 10))
		djangotest.AssertNotContains(t, resp, "?author__id__exact="+strconv.FormatInt(f.lisa.ID, 10))
		// Alfred contributed to no books, so he must NOT appear under contributors filter.
		djangotest.AssertNotContains(t, resp, "?contributors__id__exact="+strconv.FormatInt(f.alfred.ID, 10))
		djangotest.AssertContains(t, resp, "?contributors__id__exact="+strconv.FormatInt(f.bob.ID, 10))
		djangotest.AssertContains(t, resp, "?contributors__id__exact="+strconv.FormatInt(f.lisa.ID, 10))
	})
}

// Ported from Django: tests/admin_filters/tests.py::ListFiltersTests.test_fieldlistfilter_underscorelookup_tuple
// Ported from Django: tests/admin_filters/tests.py::ListFiltersTests.test_fieldlistfilter_invalid_lookup_parameters
// Ported from Django: tests/admin_filters/tests.py::ListFiltersTests.test_simplelistfilter
func TestSimpleEmptyAndInvalidListFilters(t *testing.T) {
	djangotest.EachDB(t, adminFilterModels(), func(t *testing.T, db *orm.DB) {
		f := setupAdminFiltersFixture(t, db)
		env := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
			admin.Register[AFBook](site, admin.ModelAdmin{
				ListDisplay: []string{"title", "year"},
				ListFilter: []any{
					"author",
					afDecadeFilter(),
					admin.AllValuesFilter("author__email"),
					admin.EmptyFilter("category"),
				},
				Ordering: []string{"-id"},
			})
		})
		env.client.ForceLogin(f.alfred)
		bookURL := mustReverse(t, env.router, "admin:admin_filters_afbook_changelist")

		// SimpleListFilter alone and combined with FK filter.
		resp := env.client.Get(bookURL, djangotest.Query(url.Values{"publication-decade": {"the 80s"}}))
		djangotest.AssertEqual(t, ctxVal(t, resp, "result_count"), any(0))

		resp = env.client.Get(bookURL, djangotest.Query(url.Values{"publication-decade": {"the 90s"}}))
		djangotest.AssertEqual(t, ctxVal(t, resp, "result_pks"), any([]int64{f.bio.ID}))

		resp = env.client.Get(bookURL, djangotest.Query(url.Values{"publication-decade": {"the 00s"}}))
		djangotest.AssertEqual(t, ctxVal(t, resp, "result_pks"), any([]int64{f.guitar.ID, f.djangonaut.ID}))

		resp = env.client.Get(bookURL, djangotest.Query(url.Values{
			"publication-decade": {"the 00s"},
			"author__id__exact":  {strconv.FormatInt(f.alfred.ID, 10)},
		}))
		djangotest.AssertEqual(t, ctxVal(t, resp, "result_pks"), any([]int64{f.djangonaut.ID}))

		// Spanning AllValuesFieldListFilter on author__email.
		resp = env.client.Get(bookURL, djangotest.Query(url.Values{"author__email": {"alfred@example.com"}}))
		djangotest.AssertEqual(t, ctxVal(t, resp, "result_pks"), any([]int64{f.bio.ID, f.djangonaut.ID}))

		// EmptyFieldListFilter on category (empty string or NULL).
		resp = env.client.Get(bookURL, djangotest.Query(url.Values{"category__isempty": {"1"}}))
		djangotest.AssertEqual(t, ctxVal(t, resp, "result_pks"), any([]int64{f.guitar.ID, f.djangoBook.ID}))
		resp = env.client.Get(bookURL, djangotest.Query(url.Values{"category__isempty": {"0"}}))
		djangotest.AssertEqual(t, ctxVal(t, resp, "result_pks"), any([]int64{f.bio.ID, f.djangonaut.ID}))

		// Invalid lookup value redirects to ?e=1.
		resp = env.client.Get(bookURL, djangotest.Query(url.Values{"author__id__exact": {"StringNotInteger!"}}))
		djangotest.AssertRedirects(t, resp, bookURL+"?e=1", djangotest.FetchRedirectResponse(false))

		// Disallowed relational lookup returns 400 Bad Request.
		resp = env.client.Get(bookURL, djangotest.Query(url.Values{"author__password__startswith": {"pbkdf2"}}))
		djangotest.AssertStatus(t, resp, http.StatusBadRequest)
	})
}

// afDecadeFilter is Django's DecadeListFilterWithTitleAndParameter. Its
// "other" lookup filters nothing, so the filter shows no count for it.
func afDecadeFilter() admin.SimpleListFilter {
	return admin.SimpleListFilter{
		Title:         "publication decade",
		ParameterName: "publication-decade",
		Lookups: func(*http.Request) []admin.FilterLookup {
			return []admin.FilterLookup{
				{Value: "the 80s", Display: "the 1980's"},
				{Value: "the 90s", Display: "the 1990's"},
				{Value: "the 00s", Display: "the 2000's"},
				{Value: "other", Display: "other decades"},
			}
		},
		Filter: func(_ *http.Request, val string, _ bool) (orm.Cond, bool) {
			switch val {
			case "the 80s":
				return orm.Q("year__gte", 1980, "year__lte", 1989), true
			case "the 90s":
				return orm.Q("year__gte", 1990, "year__lte", 1999), true
			case "the 00s":
				return orm.Q("year__gte", 2000, "year__lte", 2009), true
			default:
				return orm.Cond{}, false
			}
		},
	}
}

// afDepartmentOwnershipFilter is Django's DepartmentOwnershipListFilter. It
// annotates the books with the count of their DEV department as Django's
// does, and selects them with a pk__in subquery, since a Go filter returns a
// condition and not a queryset.
func afDepartmentOwnershipFilter(db *orm.DB) admin.SimpleListFilter {
	return admin.SimpleListFilter{
		Title:         "Department Ownership",
		ParameterName: "department_ownership",
		Lookups: func(*http.Request) []admin.FilterLookup {
			return []admin.FilterLookup{
				{Value: "DEV_OWNED", Display: "Owned by Dev Department"},
				{Value: "OTHER", Display: "Other"},
			}
		},
		Filter: func(_ *http.Request, val string, _ bool) (orm.Cond, bool) {
			owned := orm.Query[AFBook](db).Annotate(
				"owned_book_count",
				orm.Count("employee__department").Filter("employee__department__code", "DEV"),
			)
			switch val {
			case "DEV_OWNED":
				return orm.Q("pk__in", owned.Filter("owned_book_count__gt", 0).ValuesList("pk")), true
			case "OTHER":
				return orm.Q("pk__in", owned.Filter("owned_book_count", 0).ValuesList("pk")), true
			default:
				return orm.Cond{}, false
			}
		},
	}
}

// afDecadeFilterBookAdmin is Django's DecadeFilterBookAdmin with show_facets.
func afDecadeFilterBookAdmin(db *orm.DB, show admin.ShowFacets) admin.ModelAdmin {
	return admin.ModelAdmin{
		EmptyValueDisplay: "???",
		ListFilter: []any{
			"author",
			afDecadeFilter(),
			"is_best_seller",
			"category",
			"date_registered",
			admin.AllValuesFilter("author__email"),
			admin.RelatedOnlyFilter("contributors"),
			admin.EmptyFilter("category"),
			afDepartmentOwnershipFilter(db),
		},
		Ordering:   []string{"-id"},
		ShowFacets: show,
	}
}

// afFilterChoices returns the displays and the query strings of the choices
// of each filter of the changelist in resp.
func afFilterChoices(t *testing.T, resp *djangotest.Response) (displays, queryStrings [][]string) {
	t.Helper()
	specs, ok := ctxVal(t, resp, "filter_specs").([]map[string]any)
	if !ok {
		t.Fatalf("filter_specs = %T, want []map[string]any", ctxVal(t, resp, "filter_specs"))
	}
	for _, spec := range specs {
		var ds, qs []string
		for _, ch := range spec["choices"].([]map[string]any) {
			ds = append(ds, ch["display"].(string))
			qs = append(qs, ch["query_string"].(string))
		}
		displays = append(displays, ds)
		queryStrings = append(queryStrings, qs)
	}
	return displays, queryStrings
}

// afFacetsSetUp registers DecadeFilterBookAdmin with show and returns the
// URL of its changelist, logged in as alfred.
func afFacetsSetUp(t *testing.T, db *orm.DB, f *afFixture, show admin.ShowFacets) (*adminTestEnv, string) {
	t.Helper()
	env := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
		admin.Register[AFBook](site, afDecadeFilterBookAdmin(db, show))
	})
	env.client.ForceLogin(f.alfred)
	return env, mustReverse(t, env.router, "admin:admin_filters_afbook_changelist")
}

// assertFacets is Django's ListFiltersTests._test_facets: every filter counts
// the four books, and each link keeps queryString when it is not empty.
func assertFacets(t *testing.T, f *afFixture, resp *djangotest.Response, queryString string) {
	t.Helper()
	djangotest.AssertEqual(t, ctxVal(t, resp, "result_pks"), any([]int64{f.guitar.ID, f.djangoBook.ID, f.bio.ID, f.djangonaut.ID}))
	expectedDateFilters := []string{"Any date (4)", "Today (2)", "Past 7 days (3)"}
	switch {
	case f.today.Month == f.oneWeekAgo.Month && f.today.Year == f.oneWeekAgo.Year:
		expectedDateFilters = append(expectedDateFilters, "This month (3)", "This year (3)")
	case f.today.Year == f.oneWeekAgo.Year:
		expectedDateFilters = append(expectedDateFilters, "This month (2)", "This year (3)")
	default:
		expectedDateFilters = append(expectedDateFilters, "This month (2)", "This year (2)")
	}
	expectedDateFilters = append(expectedDateFilters, "No date (1)", "Has date (3)")
	// No Go counterpart: connection.features.interprets_empty_strings_as_nulls
	// (Oracle) is false on every supported database.
	want := [][]string{
		// RelatedFieldListFilter.
		{"All", "alfred (2)", "bob (1)", "lisa (0)", "??? (1)"},
		// SimpleListFilter.
		{"All", "the 1980's (0)", "the 1990's (1)", "the 2000's (2)", "other decades (-)"},
		// BooleanFieldListFilter.
		{"All", "Yes (2)", "No (1)", "Unknown (1)"},
		// ChoicesFieldListFilter.
		{"All", "Non-Fictional (1)", "Fictional (1)", "We don't know (1)", "Not categorized (1)"},
		// DateFieldListFilter.
		expectedDateFilters,
		// AllValuesFieldListFilter.
		{"All", "alfred@example.com (2)", "bob@example.com (1)", "lisa@example.com (0)"},
		// RelatedOnlyFieldListFilter.
		{"All", "bob (1)", "lisa (1)", "??? (3)"},
		// EmptyFieldListFilter.
		{"All", "Empty (2)", "Not empty (2)"},
		// SimpleListFilter with join relations.
		{"All", "Owned by Dev Department (2)", "Other (2)"},
	}
	displays, queryStrings := afFilterChoices(t, resp)
	djangotest.AssertEqual(t, displays, want)
	if queryString != "" {
		for _, qs := range queryStrings {
			for _, q := range qs {
				if !strings.Contains(q, queryString) {
					t.Errorf("query string %q lacks %q", q, queryString)
				}
			}
		}
	}
}

// Ported from Django: tests/admin_filters/tests.py::ListFiltersTests.test_facets_always
func TestFacetsAlways(t *testing.T) {
	djangotest.EachDB(t, adminFilterModels(), func(t *testing.T, db *orm.DB) {
		f := setupAdminFiltersFixture(t, db)
		env, bookURL := afFacetsSetUp(t, db, f, admin.ShowFacetsAlways)
		assertFacets(t, f, env.client.Get(bookURL), "")
	})
}

// Ported from Django: tests/admin_filters/tests.py::ListFiltersTests.test_facets_no_filter
func TestFacetsNoFilter(t *testing.T) {
	djangotest.EachDB(t, adminFilterModels(), func(t *testing.T, db *orm.DB) {
		f := setupAdminFiltersFixture(t, db)
		env, bookURL := afFacetsSetUp(t, db, f, admin.ShowFacetsAllow)
		assertFacets(t, f, env.client.Get(bookURL+"?_facets"), "_facets")
	})
}

// Ported from Django: tests/admin_filters/tests.py::ListFiltersTests.test_facets_filter
func TestFacetsFilter(t *testing.T) {
	djangotest.EachDB(t, adminFilterModels(), func(t *testing.T, db *orm.DB) {
		f := setupAdminFiltersFixture(t, db)
		env, bookURL := afFacetsSetUp(t, db, f, admin.ShowFacetsAllow)
		resp := env.client.Get(bookURL, djangotest.Query(url.Values{
			"author__id__exact": {strconv.FormatInt(f.alfred.ID, 10)},
			"_facets":           {""},
		}))
		djangotest.AssertEqual(t, ctxVal(t, resp, "result_pks"), any([]int64{f.bio.ID, f.djangonaut.ID}))
		want := [][]string{
			// RelatedFieldListFilter.
			{"All", "alfred (2)", "bob (1)", "lisa (0)", "??? (1)"},
			// SimpleListFilter.
			{"All", "the 1980's (0)", "the 1990's (1)", "the 2000's (1)", "other decades (-)"},
			// BooleanFieldListFilter.
			{"All", "Yes (1)", "No (1)", "Unknown (0)"},
			// ChoicesFieldListFilter.
			{"All", "Non-Fictional (1)", "Fictional (1)", "We don't know (0)", "Not categorized (0)"},
			// DateFieldListFilter.
			{"Any date (2)", "Today (1)", "Past 7 days (1)", "This month (1)", "This year (1)", "No date (1)", "Has date (1)"},
			// AllValuesFieldListFilter.
			{"All", "alfred@example.com (2)", "bob@example.com (0)", "lisa@example.com (0)"},
			// RelatedOnlyFieldListFilter.
			{"All", "bob (0)", "lisa (0)", "??? (2)"},
			// EmptyFieldListFilter.
			{"All", "Empty (0)", "Not empty (2)"},
			// SimpleListFilter with join relations.
			{"All", "Owned by Dev Department (2)", "Other (0)"},
		}
		displays, queryStrings := afFilterChoices(t, resp)
		djangotest.AssertEqual(t, displays, want)
		for _, qs := range queryStrings {
			for _, q := range qs {
				if !strings.Contains(q, "_facets") {
					t.Errorf("query string %q lacks _facets", q)
				}
			}
		}
	})
}

// Ported from Django: tests/admin_filters/tests.py::ListFiltersTests.test_facets_disallowed
func TestFacetsDisallowed(t *testing.T) {
	djangotest.EachDB(t, adminFilterModels(), func(t *testing.T, db *orm.DB) {
		f := setupAdminFiltersFixture(t, db)
		env, bookURL := afFacetsSetUp(t, db, f, admin.ShowFacetsNever)
		// Facets are not visible even when in the url query.
		resp := env.client.Get(bookURL + "?_facets")
		djangotest.AssertEqual(t, ctxVal(t, resp, "result_pks"), any([]int64{f.guitar.ID, f.djangoBook.ID, f.bio.ID, f.djangonaut.ID}))
		want := [][]string{
			// RelatedFieldListFilter.
			{"All", "alfred", "bob", "lisa", "???"},
			// SimpleListFilter.
			{"All", "the 1980's", "the 1990's", "the 2000's", "other decades"},
			// BooleanFieldListFilter.
			{"All", "Yes", "No", "Unknown"},
			// ChoicesFieldListFilter.
			{"All", "Non-Fictional", "Fictional", "We don't know", "Not categorized"},
			// DateFieldListFilter.
			{"Any date", "Today", "Past 7 days", "This month", "This year", "No date", "Has date"},
			// AllValuesFieldListFilter.
			{"All", "alfred@example.com", "bob@example.com", "lisa@example.com"},
			// RelatedOnlyFieldListFilter.
			{"All", "bob", "lisa", "???"},
			// EmptyFieldListFilter.
			{"All", "Empty", "Not empty"},
			// SimpleListFilter with join relations.
			{"All", "Owned by Dev Department", "Other"},
		}
		displays, _ := afFilterChoices(t, resp)
		djangotest.AssertEqual(t, displays, want)
	})
}

// No Go counterpart: tests/admin_filters/tests.py::FacetsMixinTests.test_get_facet_counts
// checks the NotImplementedError of an abstract Python method.
