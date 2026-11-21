package admin_test

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/admin"
	"github.com/oliverhaas/djangogo/auth"
	"github.com/oliverhaas/djangogo/contenttypes"
	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
)

// Podcast stands in for tests/admin_views/models.py::Podcast, with the name
// of its parent Media as its own field, as the Go ORM has no model
// inheritance.
type Podcast struct {
	ID          int64
	Name        string `orm:"max_length=60"`
	ReleaseDate orm.Date
}

// Meta orders podcasts by release date, as Django's Podcast does.
func (Podcast) Meta() orm.Meta { return orm.Meta{Ordering: []string{"release_date"}} }

// dateHierarchyModels returns the models of the ported date hierarchy tests.
func dateHierarchyModels() []any {
	return []any{
		auth.App{},
		contenttypes.App{},
		admin.App{},
		djangotest.App("admin_views", &Podcast{}, &Question{}, &Answer{}),
	}
}

// The bounds are aware datetimes in UTC, the time zone the ORM reads a
// datetime in, where Django's test runs with USE_TZ = False.
//
// Ported from Django: tests/admin_changelist/test_date_hierarchy.py::DateHierarchyTests.test_bounded_params
func TestDateHierarchyBoundedParams(t *testing.T) {
	utc := func(year int, month time.Month, day int) time.Time {
		return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
	}
	tests := []struct {
		query    url.Values
		from, to time.Time
	}{
		{url.Values{"date__year": {"2017"}}, utc(2017, 1, 1), utc(2018, 1, 1)},
		{url.Values{"date__year": {"2017"}, "date__month": {"2"}}, utc(2017, 2, 1), utc(2017, 3, 1)},
		{url.Values{"date__year": {"2017"}, "date__month": {"12"}}, utc(2017, 12, 1), utc(2018, 1, 1)},
		{
			url.Values{"date__year": {"2017"}, "date__month": {"12"}, "date__day": {"15"}},
			utc(2017, 12, 15), utc(2017, 12, 16),
		},
		{
			url.Values{"date__year": {"2017"}, "date__month": {"12"}, "date__day": {"31"}},
			utc(2017, 12, 31), utc(2018, 1, 1),
		},
		{
			url.Values{"date__year": {"2017"}, "date__month": {"2"}, "date__day": {"28"}},
			utc(2017, 2, 28), utc(2017, 3, 1),
		},
	}
	for _, tt := range tests {
		t.Run(tt.query.Encode(), func(t *testing.T) {
			from, to, err := admin.DateHierarchyBounds(tt.query, "date")
			if err != nil {
				t.Fatalf("DateHierarchyBounds: %v", err)
			}
			if !from.Equal(tt.from) || !to.Equal(tt.to) {
				t.Errorf("bounds = %v, %v, want %v, %v", from, to, tt.from, tt.to)
			}
		})
	}
}

// No Go counterpart: tests/admin_changelist/test_date_hierarchy.py::DateHierarchyTests.test_bounded_params_with_time_zone
// and test_bounded_params_with_dst_time_zone: the bounds are in UTC, as the
// ORM reads a datetime in no other time zone.

// Ported from Django: tests/admin_changelist/test_date_hierarchy.py::DateHierarchyTests.test_invalid_params
func TestDateHierarchyInvalidParams(t *testing.T) {
	for _, query := range []url.Values{
		{"date__year": {"x"}},
		{"date__year": {"2017"}, "date__month": {"x"}},
		{"date__year": {"2017"}, "date__month": {"12"}, "date__day": {"x"}},
		{"date__year": {"2017"}, "date__month": {"13"}},
		{"date__year": {"2017"}, "date__month": {"12"}, "date__day": {"32"}},
		{"date__year": {"2017"}, "date__month": {"0"}},
		{"date__year": {"2017"}, "date__month": {"12"}, "date__day": {"0"}},
	} {
		t.Run(query.Encode(), func(t *testing.T) {
			if _, _, err := admin.DateHierarchyBounds(query, "date"); !errors.Is(err, admin.ErrIncorrectLookup) {
				t.Errorf("DateHierarchyBounds error = %v, want ErrIncorrectLookup", err)
			}
		})
	}
}

// dateHierarchyEnv returns a client, logged in as a superuser, on a site with
// the PodcastAdmin, QuestionAdmin and Answer registration of
// tests/admin_views/admin.py.
func dateHierarchyEnv(t *testing.T, db *orm.DB) *adminTestEnv {
	t.Helper()
	env := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
		admin.Register[Podcast](site, admin.ModelAdmin{
			ListDisplay:   []string{"name", "release_date"},
			ListEditable:  []string{"release_date"},
			DateHierarchy: "release_date",
			ListFilter:    []any{"name"},
			SearchFields:  []string{"name"},
			Ordering:      []string{"name"},
		})
		admin.Register[Question](site, questionAdmin())
		admin.Register[Answer](site, admin.ModelAdmin{
			AutocompleteFields: []string{"question"},
			DateHierarchy:      "question__posted",
		})
	})
	env.client.ForceLogin(createTestUser(t, db, "super", "secret", true, true))
	return env
}

// assertNonLocalizedYear asserts that the page does not print the year with
// a thousand separator (#15234).
func assertNonLocalizedYear(t *testing.T, resp *djangotest.Response, year int) {
	t.Helper()
	djangotest.AssertNotContains(t, resp, fmt.Sprintf("%d,%03d", year/1000, year%1000))
}

// assertYearLink asserts that the page links to the year of date.
func assertYearLink(t *testing.T, resp *djangotest.Response, date orm.Date) {
	t.Helper()
	djangotest.AssertContains(t, resp, fmt.Sprintf(`?release_date__year=%d"`, date.Year))
}

// assertMonthLink asserts that the page links to the month of date.
func assertMonthLink(t *testing.T, resp *djangotest.Response, date orm.Date) {
	t.Helper()
	djangotest.AssertContains(t, resp,
		fmt.Sprintf(`?release_date__month=%d&amp;release_date__year=%d"`, date.Month, date.Year))
}

// assertDayLink asserts that the page links to the day of date.
func assertDayLink(t *testing.T, resp *djangotest.Response, date orm.Date) {
	t.Helper()
	djangotest.AssertContains(t, resp, fmt.Sprintf(
		`?release_date__day=%d&amp;release_date__month=%d&amp;release_date__year=%d"`,
		date.Day, date.Month, date.Year))
}

// Ported from Django: tests/admin_views/tests.py::DateHierarchyTests.test_empty
// Ported from Django: tests/admin_views/tests.py::DateHierarchyTests.test_single
// Ported from Django: tests/admin_views/tests.py::DateHierarchyTests.test_within_month
// Ported from Django: tests/admin_views/tests.py::DateHierarchyTests.test_within_year
// Ported from Django: tests/admin_views/tests.py::DateHierarchyTests.test_multiple_years
// Ported from Django: tests/admin_views/tests.py::DateHierarchyTests.test_related_field
func TestAdminViewsDateHierarchy(t *testing.T) {
	djangotest.EachDB(t, dateHierarchyModels(), func(t *testing.T, db *orm.DB) {
		env := dateHierarchyEnv(t, db)
		changelist := mustReverse(t, env.router, "admin:admin_views_podcast_changelist")
		createPodcasts := func(t *testing.T, dates ...orm.Date) {
			t.Helper()
			if _, err := orm.Query[Podcast](db).Delete(context.Background()); err != nil {
				t.Fatalf("delete podcasts: %v", err)
			}
			for _, date := range dates {
				mustCreate(t, db, &Podcast{ReleaseDate: date})
			}
		}

		t.Run("test_empty", func(t *testing.T) {
			createPodcasts(t)
			resp := env.client.Get(changelist)
			djangotest.AssertNotContains(t, resp, "release_date__year=")
			djangotest.AssertNotContains(t, resp, "release_date__month=")
			djangotest.AssertNotContains(t, resp, "release_date__day=")
		})

		t.Run("test_single", func(t *testing.T) {
			date := orm.NewDate(2000, 6, 30)
			createPodcasts(t, date)
			resp := env.client.Get(changelist)
			assertDayLink(t, resp, date)
			assertNonLocalizedYear(t, resp, 2000)
		})

		t.Run("test_within_month", func(t *testing.T) {
			dates := []orm.Date{orm.NewDate(2000, 6, 30), orm.NewDate(2000, 6, 15), orm.NewDate(2000, 6, 3)}
			createPodcasts(t, dates...)
			resp := env.client.Get(changelist)
			for _, date := range dates {
				assertDayLink(t, resp, date)
			}
			assertNonLocalizedYear(t, resp, 2000)
		})

		t.Run("test_within_year", func(t *testing.T) {
			dates := []orm.Date{orm.NewDate(2000, 1, 30), orm.NewDate(2000, 3, 15), orm.NewDate(2000, 5, 3)}
			createPodcasts(t, dates...)
			resp := env.client.Get(changelist)
			// No day-level links.
			djangotest.AssertNotContains(t, resp, "release_date__day=")
			for _, date := range dates {
				assertMonthLink(t, resp, date)
			}
			assertNonLocalizedYear(t, resp, 2000)
		})

		t.Run("test_multiple_years", func(t *testing.T) {
			dates := []orm.Date{orm.NewDate(2001, 1, 30), orm.NewDate(2003, 3, 15), orm.NewDate(2005, 5, 3)}
			createPodcasts(t, dates...)
			resp := env.client.Get(changelist)
			// No day- or month-level links.
			djangotest.AssertNotContains(t, resp, "release_date__day=")
			djangotest.AssertNotContains(t, resp, "release_date__month=")
			for _, date := range dates {
				assertYearLink(t, resp, date)
			}
			// The GET parameters still behave correctly.
			for _, date := range dates {
				resp := env.client.Get(fmt.Sprintf("%s?release_date__year=%d", changelist, date.Year))
				assertMonthLink(t, resp, date)
				for _, year := range []int{2000, 2003, 2005} {
					assertNonLocalizedYear(t, resp, year)
				}
				resp = env.client.Get(fmt.Sprintf("%s?release_date__year=%d&release_date__month=%d",
					changelist, date.Year, date.Month))
				assertDayLink(t, resp, date)
				for _, year := range []int{2000, 2003, 2005} {
					assertNonLocalizedYear(t, resp, year)
				}
			}
		})

		t.Run("test_related_field", func(t *testing.T) {
			questionsData := []struct {
				posted  orm.Date
				answers int
			}{
				{orm.NewDate(2001, 1, 30), 0},
				{orm.NewDate(2003, 3, 15), 1},
				{orm.NewDate(2005, 5, 3), 2},
			}
			for _, q := range questionsData {
				question := mustCreate(t, db, &Question{Posted: q.posted})
				for range q.answers {
					answer := &Answer{}
					answer.Question.SetPK(question.ID)
					mustCreate(t, db, answer)
				}
			}
			resp := env.client.Get(mustReverse(t, env.router, "admin:admin_views_answer_changelist"))
			for _, q := range questionsData {
				link := fmt.Sprintf(`?question__posted__year=%d"`, q.posted.Year)
				if q.answers > 0 {
					djangotest.AssertContains(t, resp, link)
				} else {
					djangotest.AssertNotContains(t, resp, link)
				}
			}
		})
	})
}

// choiceLinksCase is one query of DateHierarchyTests.test_choice_links and
// the parameters of the links of its choices.
type choiceLinksCase struct {
	query   map[string]string
	choices [][]string
}

// choiceLinksCases are the cases of test_choice_links and
// test_choice_links_datetime.
var choiceLinksCases = []choiceLinksCase{
	{map[string]string{}, [][]string{{"year=2017"}, {"year=2018"}}},
	{map[string]string{"year": "2016"}, nil},
	{map[string]string{"year": "2017"}, [][]string{{"month=10", "year=2017"}, {"month=12", "year=2017"}}},
	{map[string]string{"year": "2017", "month": "9"}, nil},
	{map[string]string{"year": "2017", "month": "12"}, [][]string{
		{"day=15", "month=12", "year=2017"},
		{"day=31", "month=12", "year=2017"},
	}},
}

// assertChoiceLinks registers a ModelAdmin of Question with the date
// hierarchy field, requests its changelist with the query of each case and
// asserts the links of the choices of the date hierarchy.
func assertChoiceLinks(t *testing.T, db *orm.DB, field string) {
	t.Helper()
	env := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
		admin.Register[Question](site, admin.ModelAdmin{DateHierarchy: field})
	})
	env.client.ForceLogin(createTestUser(t, db, "super", "secret", true, true))
	changelist := mustReverse(t, env.router, "admin:admin_views_question_changelist")
	for _, tt := range choiceLinksCases {
		query := url.Values{}
		for k, v := range tt.query {
			query.Set(field+"__"+k, v)
		}
		t.Run(query.Encode(), func(t *testing.T) {
			want := make([]string, 0, len(tt.choices))
			for _, choice := range tt.choices {
				parts := make([]string, len(choice))
				for i, c := range choice {
					parts[i] = field + "__" + c
				}
				want = append(want, "?"+strings.Join(parts, "&"))
			}
			resp := env.client.Get(changelist + "?" + query.Encode())
			spec, ok := ctxVal(t, resp, "date_hierarchy").(map[string]any)
			if !ok {
				t.Fatalf("date_hierarchy = %#v, want a map", ctxVal(t, resp, "date_hierarchy"))
			}
			choices, _ := spec["choices"].([]map[string]any)
			got := make([]string, 0, len(choices))
			for _, choice := range choices {
				link, _ := choice["link"].(string)
				got = append(got, link)
			}
			if !slices.Equal(got, want) {
				t.Errorf("choice links = %q, want %q", got, want)
			}
		})
	}
}

// Ported from Django: tests/admin_views/test_templatetags.py::DateHierarchyTests.test_choice_links
func TestDateHierarchyChoiceLinks(t *testing.T) {
	djangotest.EachDB(t, dateHierarchyModels(), func(t *testing.T, db *orm.DB) {
		for _, posted := range []orm.Date{
			orm.NewDate(2017, 10, 1), orm.NewDate(2017, 10, 1), orm.NewDate(2017, 12, 15),
			orm.NewDate(2017, 12, 15), orm.NewDate(2017, 12, 31), orm.NewDate(2018, 2, 1),
		} {
			mustCreate(t, db, &Question{Question: "q", Posted: posted})
		}
		assertChoiceLinks(t, db, "posted")
	})
}

// Ported from Django: tests/admin_views/test_templatetags.py::DateHierarchyTests.test_choice_links_datetime
func TestDateHierarchyChoiceLinksDatetime(t *testing.T) {
	djangotest.EachDB(t, dateHierarchyModels(), func(t *testing.T, db *orm.DB) {
		for i, day := range []int{1, 1, 15, 15, 31} {
			month := time.December
			if i < 2 {
				month = time.October
			}
			expires := time.Date(2017, month, day, 0, 0, 0, 0, time.UTC)
			mustCreate(t, db, &Question{Question: fmt.Sprintf("q%d", i+1), Expires: &expires})
		}
		expires := time.Date(2018, time.February, 1, 0, 0, 0, 0, time.UTC)
		mustCreate(t, db, &Question{Question: "q6", Expires: &expires})
		assertChoiceLinks(t, db, "expires")
	})
}
