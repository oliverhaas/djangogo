package views_test

import (
	"context"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/views"
)

// dictList is DictList of Django's tests/generic_views/views.py: a list view
// over items that are not model instances.
func dictList(t *testing.T, paginateBy int) views.ListView[map[string]string] {
	t.Helper()
	return views.ListView[map[string]string]{
		Engine:       genericEngine(t),
		TemplateName: "generic_views/list.html",
		PaginateBy:   paginateBy,
		ObjectList: views.SliceList([]map[string]string{
			{"first": "John", "last": "Lennon"},
			{"first": "Yoko", "last": "Ono"},
		}),
	}
}

// authorList is AuthorList of Django's tests/generic_views/views.py. Django
// orders the authors by the Meta.ordering of the model, the Go view by its
// Ordering field.
func authorList(t *testing.T, db *orm.DB) views.ListView[Author] {
	t.Helper()
	return views.ListView[Author]{DB: db, Engine: genericEngine(t), Ordering: []string{"name"}}
}

// paginatedAuthors is AuthorList with paginate_by=30.
func paginatedAuthors(t *testing.T, db *orm.DB) views.ListView[Author] {
	t.Helper()
	view := authorList(t, db)
	view.PaginateBy = 30
	return view
}

// assertUnpaginated asserts the pagination entries of an unpaginated list.
func assertUnpaginated(t *testing.T, resp *djangotest.Response) {
	t.Helper()
	djangotest.AssertEqual(t, contextValue(t, resp, "paginator"), nil)
	djangotest.AssertEqual(t, contextValue(t, resp, "page_obj"), nil)
	djangotest.AssertEqual(t, contextValue(t, resp, "is_paginated"), any(false))
}

// assertPage asserts the size of the rendered page, the name of its first
// author and its page number.
func assertPage(t *testing.T, resp *djangotest.Response, size int, first string, number int) {
	t.Helper()
	djangotest.AssertStatus(t, resp, http.StatusOK)
	list := contextList(t, resp, "object_list")
	djangotest.AssertEqual(t, len(list), size)
	assertSameList(t, resp, "author_list", "object_list")
	djangotest.AssertEqual(t, objectName(t, list[0]), first)
	djangotest.AssertEqual(t, contextMap(t, resp, "page_obj")["number"], any(number))
}

// Ported from Django: tests/generic_views/test_list.py::ListViewTests.test_items
func TestListViewItems(t *testing.T) {
	resp := mount(t, "/list/dict/{$}", dictList(t, 0)).Get("/list/dict/")
	djangotest.AssertStatus(t, resp, http.StatusOK)
	djangotest.AssertTemplateUsed(t, resp, "generic_views/list.html")
	items, ok := contextValue(t, resp, "object_list").([]map[string]string)
	if !ok || len(items) == 0 {
		t.Fatalf("object_list = %v, want the items of the view", contextValue(t, resp, "object_list"))
	}
	djangotest.AssertEqual(t, items[0]["first"], "John")
}

// Ported from Django: tests/generic_views/test_list.py::ListViewTests.test_queryset
func TestListViewQueryset(t *testing.T) {
	eachGenericDB(t, func(t *testing.T, db *orm.DB, _ genericData) {
		resp := mount(t, "/list/authors/{$}", authorList(t, db)).Get("/list/authors/")
		djangotest.AssertStatus(t, resp, http.StatusOK)
		djangotest.AssertTemplateUsed(t, resp, "generic_views/author_list.html")
		assertContextObjects(t, db, resp, "object_list", allOf[Author](t, db, "name"))
		if view, ok := contextValue(t, resp, "view").(views.ListView[Author]); !ok {
			t.Errorf("context[\"view\"] is %T, want views.ListView[Author]", view)
		}
		assertSameList(t, resp, "author_list", "object_list")
		assertUnpaginated(t, resp)
	})
}

// Ported from Django: tests/generic_views/test_list.py::ListViewTests.test_paginated_queryset
func TestListViewPaginatedQueryset(t *testing.T) {
	eachGenericDB(t, func(t *testing.T, db *orm.DB, _ genericData) {
		makeAuthors(t, db, 100)
		resp := mount(t, "/list/authors/paginated/{$}", paginatedAuthors(t, db)).Get("/list/authors/paginated/")
		assertPage(t, resp, 30, "Author 00", 1)
		djangotest.AssertTemplateUsed(t, resp, "generic_views/author_list.html")
		djangotest.AssertEqual(t, contextValue(t, resp, "is_paginated"), any(true))
		djangotest.AssertEqual(t, contextMap(t, resp, "paginator")["num_pages"], any(4))
		djangotest.AssertEqual(t, objectName(t, contextList(t, resp, "author_list")[29]), "Author 29")
	})
}

// Ported from Django: tests/generic_views/test_list.py::ListViewTests.test_paginated_queryset_shortdata
func TestListViewPaginatedQuerysetShortdata(t *testing.T) {
	eachGenericDB(t, func(t *testing.T, db *orm.DB, _ genericData) {
		resp := mount(t, "/list/authors/paginated/{$}", paginatedAuthors(t, db)).Get("/list/authors/paginated/")
		djangotest.AssertStatus(t, resp, http.StatusOK)
		djangotest.AssertTemplateUsed(t, resp, "generic_views/author_list.html")
		assertContextObjects(t, db, resp, "object_list", allOf[Author](t, db, "name"))
		assertSameList(t, resp, "author_list", "object_list")
		djangotest.AssertEqual(t, contextMap(t, resp, "page_obj")["number"], any(1))
		djangotest.AssertEqual(t, contextMap(t, resp, "paginator")["num_pages"], any(1))
		djangotest.AssertEqual(t, contextValue(t, resp, "is_paginated"), any(false))
	})
}

// Ported from Django: tests/generic_views/test_list.py::ListViewTests.test_paginated_get_page_by_query_string
func TestListViewPaginatedGetPageByQueryString(t *testing.T) {
	eachGenericDB(t, func(t *testing.T, db *orm.DB, _ genericData) {
		makeAuthors(t, db, 100)
		c := mount(t, "/list/authors/paginated/{$}", paginatedAuthors(t, db))
		resp := c.Get("/list/authors/paginated/", djangotest.Query(url.Values{"page": {"2"}}))
		assertPage(t, resp, 30, "Author 30", 2)
		djangotest.AssertTemplateUsed(t, resp, "generic_views/author_list.html")
	})
}

// Ported from Django: tests/generic_views/test_list.py::ListViewTests.test_paginated_get_last_page_by_query_string
func TestListViewPaginatedGetLastPageByQueryString(t *testing.T) {
	eachGenericDB(t, func(t *testing.T, db *orm.DB, _ genericData) {
		makeAuthors(t, db, 100)
		c := mount(t, "/list/authors/paginated/{$}", paginatedAuthors(t, db))
		resp := c.Get("/list/authors/paginated/", djangotest.Query(url.Values{"page": {"last"}}))
		assertPage(t, resp, 10, "Author 90", 4)
	})
}

// Ported from Django: tests/generic_views/test_list.py::ListViewTests.test_paginated_get_page_by_urlvar
func TestListViewPaginatedGetPageByURLVar(t *testing.T) {
	eachGenericDB(t, func(t *testing.T, db *orm.DB, _ genericData) {
		makeAuthors(t, db, 100)
		c := mount(t, "/list/authors/paginated/{page}/{$}", paginatedAuthors(t, db))
		resp := c.Get("/list/authors/paginated/3/")
		assertPage(t, resp, 30, "Author 60", 3)
		djangotest.AssertTemplateUsed(t, resp, "generic_views/author_list.html")
	})
}

// Ported from Django: tests/generic_views/test_list.py::ListViewTests.test_paginated_page_out_of_range
func TestListViewPaginatedPageOutOfRange(t *testing.T) {
	eachGenericDB(t, func(t *testing.T, db *orm.DB, _ genericData) {
		makeAuthors(t, db, 100)
		c := mount(t, "/list/authors/paginated/{page}/{$}", paginatedAuthors(t, db))
		djangotest.AssertStatus(t, c.Get("/list/authors/paginated/42/"), http.StatusNotFound)
	})
}

// Ported from Django: tests/generic_views/test_list.py::ListViewTests.test_paginated_invalid_page
func TestListViewPaginatedInvalidPage(t *testing.T) {
	eachGenericDB(t, func(t *testing.T, db *orm.DB, _ genericData) {
		makeAuthors(t, db, 100)
		c := mount(t, "/list/authors/paginated/{$}", paginatedAuthors(t, db))
		resp := c.Get("/list/authors/paginated/?page=frog")
		djangotest.AssertStatus(t, resp, http.StatusNotFound)
		djangotest.AssertEqual(t, contextValue(t, resp, "exception"),
			any("Page is not “last”, nor can it be converted to an int."))
	})
}

// Ported from Django: tests/generic_views/test_list.py::ListViewTests.test_paginated_custom_paginator_class
// Ported from Django: tests/generic_views/test_list.py::ListViewTests.test_paginated_custom_paginator_constructor
//
// Django sets paginator_class in the first test and overrides get_paginator in
// the second. Both are the GetPaginator hook in Go.
func TestListViewPaginatedCustomPaginator(t *testing.T) {
	eachGenericDB(t, func(t *testing.T, db *orm.DB, _ genericData) {
		makeAuthors(t, db, 7)
		view := authorList(t, db)
		view.PaginateBy = 5
		view.GetPaginator = func(
			list views.ObjectList[Author], perPage, _ int, allowEmptyFirstPage bool,
		) *views.Paginator[Author] {
			return views.NewPaginator(list, perPage,
				views.WithOrphans(2), views.WithAllowEmptyFirstPage(allowEmptyFirstPage))
		}
		resp := mount(t, "/list/authors/paginated/custom_class/{$}", view).Get("/list/authors/paginated/custom_class/")
		djangotest.AssertStatus(t, resp, http.StatusOK)
		djangotest.AssertEqual(t, contextMap(t, resp, "paginator")["num_pages"], any(1))
		// Custom pagination allows for 2 orphans on a page size of 5.
		djangotest.AssertEqual(t, len(contextList(t, resp, "object_list")), 7)
	})
}

// Ported from Django: tests/generic_views/test_list.py::ListViewTests.test_paginated_custom_page_kwarg
func TestListViewPaginatedCustomPageKwarg(t *testing.T) {
	eachGenericDB(t, func(t *testing.T, db *orm.DB, _ genericData) {
		makeAuthors(t, db, 100)
		view := paginatedAuthors(t, db)
		view.PageKwarg = "pagina"
		c := mount(t, "/list/authors/paginated/custom_page_kwarg/{$}", view)
		resp := c.Get("/list/authors/paginated/custom_page_kwarg/", djangotest.Query(url.Values{"pagina": {"2"}}))
		assertPage(t, resp, 30, "Author 30", 2)
		djangotest.AssertTemplateUsed(t, resp, "generic_views/author_list.html")
	})
}

// Ported from Django: tests/generic_views/test_list.py::ListViewTests.test_paginated_orphaned_queryset
func TestListViewPaginatedOrphanedQueryset(t *testing.T) {
	eachGenericDB(t, func(t *testing.T, db *orm.DB, _ genericData) {
		makeAuthors(t, db, 92)
		view := paginatedAuthors(t, db)
		view.PaginateOrphans = 2
		c := mount(t, "/list/authors/paginated-orphaned/{$}", view)
		for _, tc := range []struct {
			page   string
			number int
		}{{"", 1}, {"last", 3}, {"3", 3}} {
			query := url.Values{}
			if tc.page != "" {
				query.Set("page", tc.page)
			}
			resp := c.Get("/list/authors/paginated-orphaned/", djangotest.Query(query))
			djangotest.AssertStatus(t, resp, http.StatusOK)
			djangotest.AssertEqual(t, contextMap(t, resp, "page_obj")["number"], any(tc.number))
		}
		resp := c.Get("/list/authors/paginated-orphaned/", djangotest.Query(url.Values{"page": {"4"}}))
		djangotest.AssertStatus(t, resp, http.StatusNotFound)
	})
}

// Ported from Django: tests/generic_views/test_list.py::ListViewTests.test_paginated_non_queryset
func TestListViewPaginatedNonQueryset(t *testing.T) {
	resp := mount(t, "/list/dict/paginated/{$}", dictList(t, 1)).Get("/list/dict/paginated/")
	djangotest.AssertStatus(t, resp, http.StatusOK)
	items, _ := contextValue(t, resp, "object_list").([]map[string]string)
	djangotest.AssertEqual(t, len(items), 1)
}

// Ported from Django: tests/generic_views/test_list.py::ListViewTests.test_verbose_name
func TestListViewVerboseName(t *testing.T) {
	eachGenericDB(t, func(t *testing.T, db *orm.DB, _ genericData) {
		view := views.ListView[Artist]{DB: db, Engine: genericEngine(t), TemplateName: "generic_views/list.html"}
		resp := mount(t, "/list/artists/{$}", view).Get("/list/artists/")
		djangotest.AssertStatus(t, resp, http.StatusOK)
		djangotest.AssertTemplateUsed(t, resp, "generic_views/list.html")
		assertContextObjects(t, db, resp, "object_list", allOf[Artist](t, db, "id"))
		assertSameList(t, resp, "artist_list", "object_list")
		assertUnpaginated(t, resp)
	})
}

// Ported from Django: tests/generic_views/test_list.py::ListViewTests.test_allow_empty_false
func TestListViewAllowEmptyFalse(t *testing.T) {
	eachGenericDB(t, func(t *testing.T, db *orm.DB, _ genericData) {
		view := authorList(t, db)
		view.AllowEmpty = new(false)
		c := mount(t, "/list/authors/notempty/{$}", view)
		djangotest.AssertStatus(t, c.Get("/list/authors/notempty/"), http.StatusOK)
		if _, err := orm.Query[Author](db).Delete(context.Background()); err != nil {
			t.Fatal(err)
		}
		resp := c.Get("/list/authors/notempty/")
		djangotest.AssertStatus(t, resp, http.StatusNotFound)
		djangotest.AssertEqual(t, contextValue(t, resp, "exception"),
			any("Empty list and “ListView.allow_empty” is False."))
	})
}

// Ported from Django: tests/generic_views/test_list.py::ListViewTests.test_template_name
func TestListViewTemplateName(t *testing.T) {
	eachGenericDB(t, func(t *testing.T, db *orm.DB, _ genericData) {
		view := authorList(t, db)
		view.TemplateName = "generic_views/list.html"
		resp := mount(t, "/list/authors/template_name/{$}", view).Get("/list/authors/template_name/")
		djangotest.AssertStatus(t, resp, http.StatusOK)
		assertContextObjects(t, db, resp, "object_list", allOf[Author](t, db, "name"))
		assertSameList(t, resp, "author_list", "object_list")
		djangotest.AssertTemplateUsed(t, resp, "generic_views/list.html")
	})
}

// Ported from Django: tests/generic_views/test_list.py::ListViewTests.test_template_name_suffix
func TestListViewTemplateNameSuffix(t *testing.T) {
	eachGenericDB(t, func(t *testing.T, db *orm.DB, _ genericData) {
		view := authorList(t, db)
		view.TemplateNameSuffix = "_objects"
		resp := mount(t, "/list/authors/template_name_suffix/{$}", view).Get("/list/authors/template_name_suffix/")
		djangotest.AssertStatus(t, resp, http.StatusOK)
		assertContextObjects(t, db, resp, "object_list", allOf[Author](t, db, "name"))
		assertSameList(t, resp, "author_list", "object_list")
		djangotest.AssertTemplateUsed(t, resp, "generic_views/author_objects.html")
	})
}

// Ported from Django: tests/generic_views/test_list.py::ListViewTests.test_context_object_name
func TestListViewContextObjectName(t *testing.T) {
	eachGenericDB(t, func(t *testing.T, db *orm.DB, _ genericData) {
		view := authorList(t, db)
		view.ContextObjectName = "author_list"
		resp := mount(t, "/list/authors/context_object_name/{$}", view).Get("/list/authors/context_object_name/")
		djangotest.AssertStatus(t, resp, http.StatusOK)
		assertContextObjects(t, db, resp, "object_list", allOf[Author](t, db, "name"))
		assertNotInContext(t, resp, "authors")
		assertSameList(t, resp, "author_list", "object_list")
		djangotest.AssertTemplateUsed(t, resp, "generic_views/author_list.html")
	})
}

// Ported from Django: tests/generic_views/test_list.py::ListViewTests.test_duplicate_context_object_name
func TestListViewDuplicateContextObjectName(t *testing.T) {
	eachGenericDB(t, func(t *testing.T, db *orm.DB, _ genericData) {
		view := authorList(t, db)
		view.ContextObjectName = "object_list"
		c := mount(t, "/list/authors/dupe_context_object_name/{$}", view)
		resp := c.Get("/list/authors/dupe_context_object_name/")
		djangotest.AssertStatus(t, resp, http.StatusOK)
		assertContextObjects(t, db, resp, "object_list", allOf[Author](t, db, "name"))
		assertNotInContext(t, resp, "authors")
		assertNotInContext(t, resp, "author_list")
		djangotest.AssertTemplateUsed(t, resp, "generic_views/author_list.html")
	})
}

// Ported from Django: tests/generic_views/test_list.py::ListViewTests.test_missing_items
//
// Django clears the queryset of a view without model. The Go counterpart is a
// view without DB, queryset and object list.
func TestListViewMissingItems(t *testing.T) {
	logs := captureLogs(t)
	c := mount(t, "/list/authors/invalid/{$}", views.ListView[Author]{Engine: genericEngine(t)})
	assertImproperlyConfigured(t, logs, c.Get("/list/authors/invalid/"), "ListView is missing a QuerySet.")
}

// Ported from Django: tests/generic_views/test_list.py::ListViewTests.test_invalid_get_queryset
//
// Django reports the missing queryset when it looks for the template name. The
// Go view reports it where the hook returns.
func TestListViewInvalidGetQueryset(t *testing.T) {
	logs := captureLogs(t)
	c := mount(t, "/list/authors/get_queryset/{$}", views.ListView[Author]{
		Engine:      genericEngine(t),
		GetQueryset: func(*http.Request) (*orm.QuerySet[Author], error) { return nil, nil },
	})
	assertImproperlyConfigured(t, logs, c.Get("/list/authors/get_queryset/"),
		"ListView.GetQueryset returned no queryset")
}

// Ported from Django: tests/generic_views/test_list.py::ListViewTests.test_paginated_list_view_does_not_load_entire_table
func TestListViewPaginatedDoesNotLoadEntireTable(t *testing.T) {
	eachGenericDB(t, func(t *testing.T, db *orm.DB, _ genericData) {
		makeAuthors(t, db, 3)
		notEmpty := authorList(t, db)
		notEmpty.AllowEmpty = new(false)
		paginated := notEmpty
		paginated.PaginateBy = 2
		c := mount(t, "/list/authors/notempty/{$}", notEmpty, "/list/authors/notempty/paginated/{$}", paginated)
		// 1 query for authors.
		djangotest.AssertNumQueries(t, db, 1, func() {
			djangotest.AssertStatus(t, c.Get("/list/authors/notempty/"), http.StatusOK)
		})
		// Same as above + 1 query to test if authors exist + 1 query for
		// pagination.
		djangotest.AssertNumQueries(t, db, 3, func() {
			djangotest.AssertStatus(t, c.Get("/list/authors/notempty/paginated/"), http.StatusOK)
		})
	})
}

// Ported from Django: tests/generic_views/test_list.py::ListViewTests.test_explicitly_ordered_list_view
func TestListViewExplicitlyOrdered(t *testing.T) {
	eachGenericDB(t, func(t *testing.T, db *orm.DB, _ genericData) {
		create(t, db, &Book{
			Name: "Zebras for Dummies", Pages: 800,
			Pubdate: time.Date(2006, 9, 1, 0, 0, 0, 0, time.UTC),
		})
		sorted := views.ListView[Book]{DB: db, Engine: genericEngine(t), Ordering: []string{"name"}}
		byPages := sorted
		byPages.Ordering = []string{"pages", "-name"}
		c := mount(t, "/list/books/sorted/{$}", sorted, "/list/books/sortedbypagesandnamedec/{$}", byPages)
		for path, want := range map[string][]string{
			"/list/books/sorted/":                  {"2066", "Dreaming in Code", "Zebras for Dummies"},
			"/list/books/sortedbypagesandnamedec/": {"Dreaming in Code", "Zebras for Dummies", "2066"},
		} {
			resp := c.Get(path)
			djangotest.AssertStatus(t, resp, http.StatusOK)
			var names []string
			for _, book := range contextList(t, resp, "object_list") {
				names = append(names, objectName(t, book))
			}
			djangotest.AssertSequenceEqual(t, names, want)
		}
	})
}

// Ported from Django: tests/generic_views/test_list.py::ListViewTests.test_paginated_list_view_returns_useful_message_on_invalid_page
//
// Django reads the message from the technical 404 page of DEBUG mode. The Go
// test reads it from the context of the 404 template.
func TestListViewPaginatedReturnsUsefulMessageOnInvalidPage(t *testing.T) {
	eachGenericDB(t, func(t *testing.T, db *orm.DB, _ genericData) {
		makeAuthors(t, db, 1)
		c := mount(t, "/list/authors/paginated/{page}/{$}", paginatedAuthors(t, db))
		resp := c.Get("/list/authors/paginated/2/")
		djangotest.AssertStatus(t, resp, http.StatusNotFound)
		djangotest.AssertEqual(t, contextValue(t, resp, "exception"),
			any("Invalid page (2): That page contains no results"))
	})
}
