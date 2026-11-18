package views_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/views"
)

// PaginationArticle is the Article model of Django's tests/pagination/models.py.
type PaginationArticle struct {
	ID       int64
	Headline string `orm:"max_length=100"`
	PubDate  time.Time
}

// paginationArticles creates the nine articles of ModelPaginationTests and
// returns them in creation order.
func paginationArticles(t *testing.T, db *orm.DB) []PaginationArticle {
	t.Helper()
	pubDate := time.Date(2005, 7, 29, 0, 0, 0, 0, time.UTC)
	articles := make([]PaginationArticle, 0, 9)
	for x := 1; x < 10; x++ {
		a := PaginationArticle{Headline: fmt.Sprintf("Article %d", x), PubDate: pubDate}
		if err := orm.Query[PaginationArticle](db).Create(context.Background(), &a); err != nil {
			t.Fatalf("create article: %v", err)
		}
		articles = append(articles, a)
	}
	return articles
}

// articleHeadlines returns the headlines of the articles, which identify them
// without comparing time values across database round trips.
func articleHeadlines(articles []PaginationArticle) []string {
	out := make([]string, len(articles))
	for i, a := range articles {
		out[i] = a.Headline
	}
	return out
}

// Ported from Django: tests/pagination/tests.py::ModelPaginationTests.test_first_page
//
// The Go test also asserts the query budget: one COUNT and one LIMIT/OFFSET
// query for the page.
func TestFirstPage(t *testing.T) {
	djangotest.EachDB(t, []any{&PaginationArticle{}}, func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		articles := paginationArticles(t, db)
		paginator := views.NewPaginator(views.QuerySetList(orm.Query[PaginationArticle](db).OrderBy("id")), 5)
		var p *views.Page[PaginationArticle]
		djangotest.AssertNumQueries(t, db, 2, func() {
			var err error
			if p, err = paginator.Page(ctx, 1); err != nil {
				t.Fatal(err)
			}
		})
		djangotest.AssertEqual(t, p.String(), "<Page 1 of 2>")
		djangotest.AssertEqual(t, articleHeadlines(p.ObjectList), articleHeadlines(articles[:5]))
		djangotest.AssertEqual(t, p.HasNext(), true)
		djangotest.AssertEqual(t, p.HasPrevious(), false)
		djangotest.AssertEqual(t, p.HasOtherPages(), true)
		next, err := p.NextPageNumber()
		djangotest.AssertEqual(t, err, nil)
		djangotest.AssertEqual(t, next, 2)
		_, err = p.PreviousPageNumber()
		djangotest.AssertErrorIs(t, err, views.ErrInvalidPage)
		djangotest.AssertEqual(t, p.StartIndex(), 1)
		djangotest.AssertEqual(t, p.EndIndex(), 5)
	})
}

// Ported from Django: tests/pagination/tests.py::ModelPaginationTests.test_last_page
//
// The Go test also asserts that a second page of the same paginator costs one
// query, because the count is cached.
func TestLastPage(t *testing.T) {
	djangotest.EachDB(t, []any{&PaginationArticle{}}, func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		articles := paginationArticles(t, db)
		paginator := views.NewPaginator(views.QuerySetList(orm.Query[PaginationArticle](db).OrderBy("id")), 5)
		if _, err := paginator.Page(ctx, 1); err != nil {
			t.Fatal(err)
		}
		var p *views.Page[PaginationArticle]
		djangotest.AssertNumQueries(t, db, 1, func() {
			var err error
			if p, err = paginator.Page(ctx, 2); err != nil {
				t.Fatal(err)
			}
		})
		djangotest.AssertEqual(t, p.String(), "<Page 2 of 2>")
		djangotest.AssertEqual(t, articleHeadlines(p.ObjectList), articleHeadlines(articles[5:]))
		djangotest.AssertEqual(t, p.HasNext(), false)
		djangotest.AssertEqual(t, p.HasPrevious(), true)
		djangotest.AssertEqual(t, p.HasOtherPages(), true)
		_, err := p.NextPageNumber()
		djangotest.AssertErrorIs(t, err, views.ErrInvalidPage)
		previous, err := p.PreviousPageNumber()
		djangotest.AssertEqual(t, err, nil)
		djangotest.AssertEqual(t, previous, 1)
		djangotest.AssertEqual(t, p.StartIndex(), 6)
		djangotest.AssertEqual(t, p.EndIndex(), 9)
	})
}

// Ported from Django: tests/pagination/tests.py::ModelPaginationTests.test_page_getitem
//
// Django loads the page lazily on first item access. The Go page holds its
// items as a slice loaded by Paginator.Page, so the test asserts indexing and
// slicing of ObjectList and that an empty first page runs no page query.
func TestPageGetitem(t *testing.T) {
	djangotest.EachDB(t, []any{&PaginationArticle{}}, func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		articles := paginationArticles(t, db)
		paginator := views.NewPaginator(views.QuerySetList(orm.Query[PaginationArticle](db).OrderBy("id")), 5)
		p, err := paginator.Page(ctx, 1)
		if err != nil {
			t.Fatal(err)
		}
		djangotest.AssertEqual(t, p.ObjectList[0].Headline, articles[0].Headline)
		djangotest.AssertEqual(t, articleHeadlines(p.ObjectList[:2]), articleHeadlines(articles[:2]))

		empty := views.NewPaginator(views.QuerySetList(orm.Query[PaginationArticle](db).Filter("headline", "none")), 5)
		djangotest.AssertNumQueries(t, db, 1, func() {
			page, err := empty.Page(ctx, 1)
			if err != nil {
				t.Fatal(err)
			}
			djangotest.AssertEqual(t, page.Len(), 0)
		})
	})
}

// TestPageTemplateValue covers the template view of a page and its paginator,
// which has no Django counterpart: Django templates read the attributes of
// the Page object directly.
func TestPageTemplateValue(t *testing.T) {
	ctx := context.Background()
	paginator := views.NewPaginator(views.SliceList(intsUpTo(25)), 10)
	paginator.TemplateObject = func(n int) any { return fmt.Sprintf("item %d", n) }
	page, err := paginator.Page(ctx, 2)
	if err != nil {
		t.Fatal(err)
	}
	eng := memEngine(t, map[string]string{"page.html": strings.Join([]string{
		"{{ page_obj.number }}/{{ page_obj.paginator.num_pages }}",
		"{% for obj in page_obj.object_list %}{{ obj }},{% endfor %}",
		"{{ page_obj.has_next }} {{ page_obj.has_previous }} {{ page_obj.has_other_pages }}",
		"{{ page_obj.previous_page_number }} {{ page_obj.next_page_number }}",
		"{{ page_obj.start_index }}-{{ page_obj.end_index }} of {{ page_obj.paginator.count }}",
		"{{ page_obj.paginator.per_page }} {{ page_obj.paginator.page_range|join:\",\" }}",
		"{% for n in page_obj.paginator.get_elided_page_range(page_obj.number) %}{{ n }} {% endfor %}",
	}, "\n")})
	out, err := eng.Render("page.html", map[string]any{"page_obj": page.TemplateValue()})
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Join([]string{
		"2/3",
		"item 11,item 12,item 13,item 14,item 15,item 16,item 17,item 18,item 19,item 20,",
		"True True True",
		"1 3",
		"11-20 of 25",
		"10 1,2,3",
		"1 2 3 ",
	}, "\n")
	djangotest.AssertEqual(t, out, want)

	// Using the number of a page that does not exist fails the render, as it
	// raises EmptyPage in a Django template.
	last, err := paginator.Page(ctx, 3)
	if err != nil {
		t.Fatal(err)
	}
	eng = memEngine(t, map[string]string{"next.html": "{{ page_obj.next_page_number }}"})
	if _, err = eng.Render("next.html", map[string]any{"page_obj": last.TemplateValue()}); err == nil {
		t.Error("render of next_page_number on the last page succeeded, want an error")
	}

	// A paginator that has not counted yet exposes lazy entries.
	lazy := views.NewPaginator(views.SliceList(intsUpTo(25)), 10)
	eng = memEngine(t, map[string]string{"lazy.html": "{{ paginator.count }} {{ paginator.num_pages }}"})
	out, err = eng.Render("lazy.html", map[string]any{"paginator": lazy.TemplateValue()})
	djangotest.AssertEqual(t, err, nil)
	djangotest.AssertEqual(t, out, "25 3")
}
