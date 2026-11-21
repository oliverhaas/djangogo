package admin_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/admin"
	"github.com/oliverhaas/djangogo/auth"
	"github.com/oliverhaas/djangogo/contenttypes"
	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
)

// MethodArticle stands in for tests/admin_views/models.py::Article with the
// fields and the display methods of the sorting tests. A Go method stands in
// for a method and for a property.
type MethodArticle struct {
	ID      int64
	Title   string `orm:"max_length=100"`
	Content string `orm:"type=text"`
	Date    time.Time
}

// String returns the title.
func (a MethodArticle) String() string { return a.Title }

// ModelYear is Article.model_year.
func (a MethodArticle) ModelYear() int { return a.Date.Year() }

// ModelYearReversed is Article.model_year_reversed.
func (a MethodArticle) ModelYearReversed() int { return a.Date.Year() }

// ModelPropertyYear is the property Article.model_property_year.
func (a MethodArticle) ModelPropertyYear() int { return a.Date.Year() }

// ModelMonth is the property Article.model_month.
func (a MethodArticle) ModelMonth() int { return int(a.Date.Month()) }

// ModelPropertyIsFromPast is the property
// Article.model_property_is_from_past.
func (a MethodArticle) ModelPropertyIsFromPast() bool { return a.Date.Before(time.Now()) }

// Badge returns HTML, which the admin shows as it is.
func (MethodArticle) Badge() template.HTML { return `<span class="badge">new</span>` }

// Published returns a time, which a changelist cell does not wrap.
func (a MethodArticle) Published() time.Time { return a.Date }

// Editor fails as a missing related object does.
func (MethodArticle) Editor() (string, error) { return "", orm.ErrDoesNotExist }

// Broken fails.
func (MethodArticle) Broken() (string, error) { return "", errors.New("broken") }

// AdminOrderedField stands in for
// tests/admin_views/models.py::AdminOrderedField.
type AdminOrderedField struct {
	ID    int64
	Order int64
	Stuff string `orm:"max_length=200"`
}

// AdminOrderedModelMethod stands in for
// tests/admin_views/models.py::AdminOrderedModelMethod.
type AdminOrderedModelMethod struct {
	ID    int64
	Order int64
	Stuff string `orm:"max_length=200"`
}

// SomeOrder is AdminOrderedModelMethod.some_order.
func (o AdminOrderedModelMethod) SomeOrder() int64 { return o.Order }

// AdminOrderedAdminMethod stands in for
// tests/admin_views/models.py::AdminOrderedAdminMethod.
type AdminOrderedAdminMethod struct {
	ID    int64
	Order int64
	Stuff string `orm:"max_length=200"`
}

// AdminOrderedCallable stands in for
// tests/admin_views/models.py::AdminOrderedCallable.
type AdminOrderedCallable struct {
	ID    int64
	Order int64
	Stuff string `orm:"max_length=200"`
}

// ComplexSortedPerson stands in for
// tests/admin_views/models.py::ComplexSortedPerson.
type ComplexSortedPerson struct {
	ID         int64
	Name       string `orm:"max_length=100"`
	Age        int64
	IsEmployee *bool `orm:"null"`
}

// sortingModels returns the models of the sorting tests.
func sortingModels() []any {
	return []any{
		auth.App{},
		contenttypes.App{},
		admin.App{},
		djangotest.App("admin_views",
			&MethodArticle{}, &AdminOrderedField{}, &AdminOrderedModelMethod{}, &AdminOrderedAdminMethod{},
			&AdminOrderedCallable{}, &ComplexSortedPerson{}, &Podcast{}),
	}
}

// methodArticleAdmin is the part of tests/admin_views/admin.py::ArticleAdmin
// that the sorting tests use. A DisplayColumns entry without a Value carries
// the @admin.display options of the model method of its name.
func methodArticleAdmin() admin.ModelAdmin {
	year := func(obj any) any { return obj.(*MethodArticle).Date.Year() }
	return admin.ModelAdmin{
		ListDisplay: []string{
			"content", "date", "callable_year", "model_year", "modeladmin_year", "model_year_reversed",
			"model_property_year", "model_month", "model_property_is_from_past",
		},
		DisplayColumns: map[string]admin.DisplayColumn{
			"callable_year":               {OrderField: "date", Value: year},
			"model_year":                  {OrderField: "date"},
			"modeladmin_year":             {OrderField: "date", Value: year},
			"model_year_reversed":         {OrderField: "-date"},
			"model_property_year":         {OrderField: "date"},
			"model_property_is_from_past": {Description: "Is from past?", Boolean: true},
		},
	}
}

// createMethodArticles creates the three articles of
// AdminViewBasicTestCase.setUpTestData.
func createMethodArticles(t *testing.T, db *orm.DB) []MethodArticle {
	t.Helper()
	articles := []MethodArticle{
		{Title: "Article 1", Content: "<p>Middle content</p>", Date: time.Date(2008, 3, 18, 11, 54, 58, 0, time.UTC)},
		{Title: "Article 2", Content: "<p>Oldest content</p>", Date: time.Date(2000, 3, 18, 11, 54, 58, 0, time.UTC)},
		{Title: "Article 3", Content: "<p>Newest content</p>", Date: time.Date(2009, 3, 18, 11, 54, 58, 0, time.UTC)},
	}
	for i := range articles {
		if err := orm.Query[MethodArticle](db).Create(context.Background(), &articles[i]); err != nil {
			t.Fatalf("Create: %v", err)
		}
	}
	return articles
}

// assertContentBefore mirrors AdminViewBasicTestCase.assertContentBefore:
// the body has first before second.
func assertContentBefore(t *testing.T, body []byte, first, second, msg string) {
	t.Helper()
	i, j := bytes.Index(body, []byte(first)), bytes.Index(body, []byte(second))
	if i < 0 || j < 0 || i >= j {
		t.Errorf("%s: %q at %d, %q at %d", msg, first, i, second, j)
	}
}

// resultListHeadRe matches the head of the result list, as Django's
// result_list_table_re does.
var resultListHeadRe = regexp.MustCompile(`(?s)<table id="result_list">.*?</thead>`)

// assertHeaderColumns asserts that the head of the result list in body has
// want columns.
func assertHeaderColumns(t *testing.T, body []byte, want int) {
	t.Helper()
	head := resultListHeadRe.Find(body)
	if got := bytes.Count(head, []byte(`<th scope="col"`)); got != want {
		t.Errorf("result list head has %d columns, want %d", got, want)
	}
}

// The "o" parameter of Django's test_change_list_sorting_property is 10;
// the port leaves out the section, the lambda and the expression columns
// 7 to 9 of ArticleAdmin.list_display.
//
// Ported from Django: tests/admin_views/tests.py::AdminViewBasicTest.test_change_list_column_field_classes
// Ported from Django: tests/admin_views/tests.py::AdminViewBasicTest.test_change_list_sorting_callable
// Ported from Django: tests/admin_views/tests.py::AdminViewBasicTest.test_change_list_boolean_display_property
// Ported from Django: tests/admin_views/tests.py::AdminViewBasicTest.test_change_list_sorting_property
// Ported from Django: tests/admin_views/tests.py::AdminViewBasicTest.test_change_list_sorting_model
// Ported from Django: tests/admin_views/tests.py::AdminViewBasicTest.test_change_list_sorting_model_admin
// Ported from Django: tests/admin_views/tests.py::AdminViewBasicTest.test_change_list_sorting_model_admin_reverse
func TestChangeListSortingModelMethod(t *testing.T) {
	djangotest.EachDB(t, sortingModels(), func(t *testing.T, db *orm.DB) {
		// No Go counterpart: the lambda half of
		// test_change_list_column_field_classes; a Go column has a name.
		// FIXME-PORT: test_change_list_sorting_callable_query_expression and
		// its reverse order by expressions, which OrderField does not take.
		env := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
			admin.Register[MethodArticle](site, methodArticleAdmin())
		})
		createMethodArticles(t, db)
		env.client.ForceLogin(createTestUser(t, db, "super", "secret", true, true))
		changelist := mustReverse(t, env.router, "admin:admin_views_methodarticle_changelist")
		get := func(o string) []byte {
			return env.client.Get(changelist, djangotest.Query(url.Values{"o": {o}})).Body
		}

		t.Run("test_change_list_column_field_classes", func(t *testing.T) {
			resp := env.client.Get(changelist)
			// callables display the callable name.
			djangotest.AssertContains(t, resp, "column-callable_year")
			djangotest.AssertContains(t, resp, "field-callable_year")
		})

		t.Run("test_change_list_sorting_callable", func(t *testing.T) {
			body := get("2")
			assertContentBefore(t, body, "Oldest content", "Middle content", "Results of sorting on callable are out of order.")
			assertContentBefore(t, body, "Middle content", "Newest content", "Results of sorting on callable are out of order.")
		})

		t.Run("test_change_list_boolean_display_property", func(t *testing.T) {
			resp := env.client.Get(changelist)
			djangotest.AssertContains(t, resp, `<td class="field-model_property_is_from_past">`+
				`<img src="/static/admin/img/icon-yes.svg" alt="True"></td>`)
		})

		t.Run("test_change_list_sorting_property", func(t *testing.T) {
			body := get("7")
			assertContentBefore(t, body, "Oldest content", "Middle content", "Results of sorting on property are out of order.")
			assertContentBefore(t, body, "Middle content", "Newest content", "Results of sorting on property are out of order.")
		})

		t.Run("test_change_list_sorting_model", func(t *testing.T) {
			body := get("-3")
			assertContentBefore(t, body, "Newest content", "Middle content", "Results of sorting on Model method are out of order.")
			assertContentBefore(t, body, "Middle content", "Oldest content", "Results of sorting on Model method are out of order.")
		})

		t.Run("test_change_list_sorting_model_admin", func(t *testing.T) {
			body := get("4")
			assertContentBefore(t, body, "Oldest content", "Middle content", "Results of sorting on ModelAdmin method are out of order.")
			assertContentBefore(t, body, "Middle content", "Newest content", "Results of sorting on ModelAdmin method are out of order.")
		})

		t.Run("test_change_list_sorting_model_admin_reverse", func(t *testing.T) {
			td := func(year int) string {
				return fmt.Sprintf(`<td class="field-model_property_year">%d</td>`, year)
			}
			body := get("6")
			assertContentBefore(t, body, td(2009), td(2008), "Results of sorting on ModelAdmin method are out of order.")
			assertContentBefore(t, body, td(2008), td(2000), "Results of sorting on ModelAdmin method are out of order.")
			// The descending order sorts too.
			body = get("-6")
			assertContentBefore(t, body, td(2000), td(2008), "Results of sorting on ModelAdmin method are out of order.")
			assertContentBefore(t, body, td(2008), td(2009), "Results of sorting on ModelAdmin method are out of order.")
		})

		// No Django test exists: a method without display options is a
		// column with the pretty name of the method, which does not sort.
		t.Run("plain_method", func(t *testing.T) {
			resp := env.client.Get(changelist)
			djangotest.AssertContains(t, resp, `<th scope="col" class="column-model_month">`)
			djangotest.AssertContains(t, resp, `<span>Model month</span>`)
			djangotest.AssertContains(t, resp, `<td class="field-model_month">3</td>`)
		})
	})
}

// The changelist marks the sorted column with "sorted ascending", the Go
// counterpart of the context value cl.get_ordering_field_columns() that
// Django's test reads.
//
// Ported from Django: tests/admin_views/tests.py::AdminViewBasicTest.test_sort_indicators_admin_order
func TestSortIndicatorsAdminOrder(t *testing.T) {
	djangotest.EachDB(t, sortingModels(), func(t *testing.T, db *orm.DB) {
		someAdminOrder := func(obj any) any { return obj.(*AdminOrderedAdminMethod).Order }
		adminOrderedCallable := func(obj any) any { return obj.(*AdminOrderedCallable).Order }
		env := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
			admin.Register[AdminOrderedField](site, admin.ModelAdmin{
				Ordering: []string{"order"}, ListDisplay: []string{"stuff", "order"},
			})
			admin.Register[AdminOrderedModelMethod](site, admin.ModelAdmin{
				Ordering: []string{"order"}, ListDisplay: []string{"stuff", "some_order"},
				DisplayColumns: map[string]admin.DisplayColumn{"some_order": {OrderField: "order"}},
			})
			admin.Register[AdminOrderedAdminMethod](site, admin.ModelAdmin{
				Ordering: []string{"order"}, ListDisplay: []string{"stuff", "some_admin_order"},
				DisplayColumns: map[string]admin.DisplayColumn{
					"some_admin_order": {OrderField: "order", Value: someAdminOrder},
				},
			})
			admin.Register[AdminOrderedCallable](site, admin.ModelAdmin{
				Ordering: []string{"order"}, ListDisplay: []string{"stuff", "admin_ordered_callable"},
				DisplayColumns: map[string]admin.DisplayColumn{
					"admin_ordered_callable": {OrderField: "order", Value: adminOrderedCallable},
				},
			})
		})
		env.client.ForceLogin(createTestUser(t, db, "super", "secret", true, true))
		items := []struct {
			order int64
			stuff string
		}{{3, "The Last Item"}, {1, "The First Item"}, {2, "The Middle Item"}}
		for _, it := range items {
			mustCreate(t, db, &AdminOrderedField{Order: it.order, Stuff: it.stuff})
			mustCreate(t, db, &AdminOrderedModelMethod{Order: it.order, Stuff: it.stuff})
			mustCreate(t, db, &AdminOrderedAdminMethod{Order: it.order, Stuff: it.stuff})
			mustCreate(t, db, &AdminOrderedCallable{Order: it.order, Stuff: it.stuff})
		}
		for _, tc := range []struct{ url, column string }{
			{"adminorderedfield", "order"},
			{"adminorderedmodelmethod", "some_order"},
			{"adminorderedadminmethod", "some_admin_order"},
			{"adminorderedcallable", "admin_ordered_callable"},
		} {
			t.Run(tc.url, func(t *testing.T) {
				resp := env.client.Get(mustReverse(t, env.router, "admin:admin_views_"+tc.url+"_changelist"))
				// Should have 3 columns including action checkbox col.
				assertHeaderColumns(t, resp.Body, 3)
				// Check if the correct column was selected.
				djangotest.AssertContains(t, resp, `class="sortable column-`+tc.column+` sorted ascending"`)
				// Check order of records.
				assertContentBefore(t, resp.Body, "The First Item", "The Middle Item", "out of order")
				assertContentBefore(t, resp.Body, "The Middle Item", "The Last Item", "out of order")
			})
		}
	})
}

// Ported from Django: tests/admin_views/tests.py::AdminViewBasicTest.test_multiple_sort_same_field
func TestMultipleSortSameField(t *testing.T) {
	djangotest.EachDB(t, sortingModels(), func(t *testing.T, db *orm.DB) {
		coloredName := func(obj any) any {
			return template.HTML(`<span style="color: #ff00ff;">` +
				template.HTMLEscapeString(obj.(*ComplexSortedPerson).Name) + `</span>`)
		}
		env := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
			admin.Register[Podcast](site, admin.ModelAdmin{
				ListDisplay:   []string{"name", "release_date"},
				ListEditable:  []string{"release_date"},
				DateHierarchy: "release_date",
				ListFilter:    []any{"name"},
				SearchFields:  []string{"name"},
				Ordering:      []string{"name"},
			})
			admin.Register[ComplexSortedPerson](site, admin.ModelAdmin{
				ListDisplay: []string{"name", "age", "is_employee", "colored_name"},
				Ordering:    []string{"name"},
				DisplayColumns: map[string]admin.DisplayColumn{
					"colored_name": {OrderField: "name", Value: coloredName},
				},
			})
		})
		env.client.ForceLogin(createTestUser(t, db, "super", "secret", true, true))

		// The changelist displays the correct columns if two columns
		// correspond to the same ordering field.
		dateOf := func(t time.Time) orm.Date { return orm.NewDate(t.Year(), t.Month(), t.Day()) }
		dt := time.Now()
		p1 := &Podcast{Name: "A", ReleaseDate: dateOf(dt)}
		p2 := &Podcast{Name: "B", ReleaseDate: dateOf(dt.AddDate(0, 0, -10))}
		mustCreate(t, db, p1)
		mustCreate(t, db, p2)
		link1 := mustReverse(t, env.router, "admin:admin_views_podcast_change", p1.ID)
		link2 := mustReverse(t, env.router, "admin:admin_views_podcast_change", p2.ID)
		resp := env.client.Get(mustReverse(t, env.router, "admin:admin_views_podcast_changelist"))
		assertContentBefore(t, resp.Body, link1, link2, "podcasts out of order")

		c1 := &ComplexSortedPerson{Name: "Bob", Age: 10}
		c2 := &ComplexSortedPerson{Name: "Amy", Age: 20}
		mustCreate(t, db, c1)
		mustCreate(t, db, c2)
		link1 = mustReverse(t, env.router, "admin:admin_views_complexsortedperson_change", c1.ID)
		link2 = mustReverse(t, env.router, "admin:admin_views_complexsortedperson_change", c2.ID)
		resp = env.client.Get(mustReverse(t, env.router, "admin:admin_views_complexsortedperson_changelist"))
		// Should have 5 columns (including action checkbox col)
		assertHeaderColumns(t, resp.Body, 5)
		djangotest.AssertContains(t, resp, "Name")
		djangotest.AssertContains(t, resp, "Colored name")
		// Check order
		assertContentBefore(t, resp.Body, "Name", "Colored name", "columns out of order")
		// Check sorting - should be by name
		assertContentBefore(t, resp.Body, link2, link1, "people out of order")
		// No Django assertion: the HTML of a column shows as it is.
		djangotest.AssertContains(t, resp, `<td class="field-colored_name"><span style="color: #ff00ff;">Amy</span></td>`)
	})
}

// No Django test exists: Django shows a model method in list_display and
// readonly_fields and has no test of the results that only Go has. A method
// that returns template.HTML shows as it is; a time does not wrap in a
// changelist cell; orm.ErrDoesNotExist shows the empty value, as Django
// shows ObjectDoesNotExist; any other error fails the changelist, and a
// read-only field shows the empty value for it, as Django's
// AdminReadonlyField shows it for AttributeError, ValueError and
// ObjectDoesNotExist.
func TestModelMethodValues(t *testing.T) {
	djangotest.EachDB(t, sortingModels(), func(t *testing.T, db *orm.DB) {
		articles := createMethodArticles(t, db)
		super := createTestUser(t, db, "super", "secret", true, true)
		env := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
			admin.Register[MethodArticle](site, admin.ModelAdmin{
				ListDisplay:    []string{"title", "Badge", "published", "editor"},
				ReadonlyFields: []string{"model_year", "badge", "editor", "broken", "ModelMonth"},
			})
		})
		env.client.ForceLogin(super)

		t.Run("changelist", func(t *testing.T) {
			resp := env.client.Get(mustReverse(t, env.router, "admin:admin_views_methodarticle_changelist"))
			djangotest.AssertStatus(t, resp, http.StatusOK)
			djangotest.AssertContains(t, resp, `<th scope="col" class="column-badge">`)
			djangotest.AssertContains(t, resp, `<td class="field-badge"><span class="badge">new</span></td>`)
			djangotest.AssertContains(t, resp, `<td class="field-published nowrap">`)
			djangotest.AssertContains(t, resp, `<td class="field-editor">-</td>`)
		})

		t.Run("readonly_fields", func(t *testing.T) {
			resp := env.client.Get(mustReverse(t, env.router, "admin:admin_views_methodarticle_change", articles[0].ID))
			djangotest.AssertStatus(t, resp, http.StatusOK)
			body := string(resp.Body)
			for _, want := range []string{
				`<label>Model year:</label>`, `<div class="readonly">2008</div>`,
				`<div class="readonly"><span class="badge">new</span></div>`,
				`<label>Model month:</label>`, `<div class="readonly">3</div>`,
			} {
				if !strings.Contains(body, want) {
					t.Errorf("change page lacks %q", want)
				}
			}
			if got := strings.Count(body, `<div class="readonly">-</div>`); got != 2 {
				t.Errorf("change page shows the empty value %d times, want 2 (editor and broken)", got)
			}
		})

		t.Run("error", func(t *testing.T) {
			broken := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
				admin.Register[MethodArticle](site, admin.ModelAdmin{ListDisplay: []string{"title", "broken"}})
			})
			broken.client.ForceLogin(super)
			resp := broken.client.Get(mustReverse(t, broken.router, "admin:admin_views_methodarticle_changelist"))
			djangotest.AssertStatus(t, resp, http.StatusInternalServerError)
		})
	})
}
