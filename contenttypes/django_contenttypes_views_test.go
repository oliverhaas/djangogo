package contenttypes_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/contenttypes"
	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/urls"
	"github.com/oliverhaas/djangogo/views"
)

// GetAbsoluteURL is Author.get_absolute_url of tests/contenttypes_tests/models.py.
func (a Author) GetAbsoluteURL() string { return fmt.Sprintf("/authors/%d/", a.ID) }

// GetAbsoluteURL is FooWithUrl.get_absolute_url of
// tests/contenttypes_tests/models.py.
func (f FooWithURL) GetAbsoluteURL() string { return "/users/" + url.PathEscape(f.Name) + "/" }

// Article is tests/contenttypes_tests/models.py::Article. It has no
// GetAbsoluteURL.
type Article struct {
	ID          int64
	Title       string         `orm:"max_length=100"`
	Slug        string         `orm:"type=slug"`
	Author      orm.FK[Author] `orm:"on_delete=cascade"`
	DateCreated time.Time
}

// SchemeIncludedURL is tests/contenttypes_tests/models.py::SchemeIncludedURL.
type SchemeIncludedURL struct {
	ID  int64
	URL string `orm:"type=url;max_length=100"`
}

// GetAbsoluteURL returns the stored URL, which includes a scheme.
func (s SchemeIncludedURL) GetAbsoluteURL() string { return s.URL }

// FooWithoutURL is tests/contenttypes_tests/models.py::FooWithoutUrl.
type FooWithoutURL struct {
	ID   int64
	Name string `orm:"max_length=30;unique"`
}

// UUIDModel is tests/contenttypes_tests/models.py::UUIDModel.
type UUIDModel struct {
	ID orm.UUID `orm:"pk;default=uuid4"`
}

// GetAbsoluteURL is UUIDModel.get_absolute_url.
func (u UUIDModel) GetAbsoluteURL() string { return "/uuid/" + u.ID.String() + "/" }

// viewModels is the registry of the ported view tests. It is kept apart from
// testModels, whose tests count the content types.
func viewModels() []any {
	return []any{
		contenttypes.App{},
		djangotest.App("contenttypes_tests",
			&Author{}, &Article{}, &SchemeIncludedURL{}, &FooWithoutURL{}, &FooWithURL{}, &UUIDModel{}),
	}
}

// viewsData holds the rows of ContentTypesViewsTests.setUpTestData.
type viewsData struct {
	authors  []*Author
	articles []*Article
	schemes  []*SchemeIncludedURL
}

// mustCreate inserts obj and returns it.
func mustCreate[T any](t *testing.T, db *orm.DB, obj *T) *T {
	t.Helper()
	if err := orm.Query[T](db).Create(context.Background(), obj); err != nil {
		t.Fatalf("create %T: %v", obj, err)
	}
	return obj
}

// viewsSetUp creates the rows of ContentTypesViewsTests.setUpTestData and
// returns a client for "contenttypes_tests.urls". The site of Django's
// setUpTestData exists for the sites framework, which Go does not have.
func viewsSetUp(t *testing.T, db *orm.DB) (*djangotest.Client, viewsData) {
	t.Helper()
	var d viewsData
	author := mustCreate(t, db, &Author{Name: "Boris"})
	d.authors = []*Author{author}
	for _, a := range []struct {
		title, slug string
		created     time.Time
	}{
		{"Old Article", "old_article", time.Date(2001, 1, 1, 21, 22, 23, 0, time.UTC)},
		{"Current Article", "current_article", time.Date(2007, 9, 17, 21, 22, 23, 0, time.UTC)},
		{"Future Article", "future_article", time.Date(3000, 1, 1, 21, 22, 23, 0, time.UTC)},
	} {
		article := &Article{Title: a.title, Slug: a.slug, DateCreated: a.created}
		article.Author.SetPK(author.ID)
		d.articles = append(d.articles, mustCreate(t, db, article))
	}
	for _, u := range []string{"http://test_scheme_included_http/", "https://test_scheme_included_https/", "//test_default_scheme_kept/"} {
		d.schemes = append(d.schemes, mustCreate(t, db, &SchemeIncludedURL{URL: u}))
	}
	router := urls.NewRouter(
		urls.RePath(`^shortcut/(?P<content_type_id>[0-9]+)/(?P<object_id>.*)/$`, views.Handler(nil, views.ContentTypeShortcut(db)), ""),
	)
	return djangotest.NewClient(t, router), d
}

// shortcutURL returns the shortcut URL of the object objectID of the model of
// instance.
func shortcutURL(t *testing.T, db *orm.DB, instance any, objectID any) string {
	t.Helper()
	return fmt.Sprintf("/shortcut/%d/%v/", mustGet(t, db, instance).ID, objectID)
}

// Ported from Django: tests/contenttypes_tests/test_views.py::ContentTypesViewsTests.test_shortcut_with_absolute_url
func TestShortcutWithAbsoluteURL(t *testing.T) {
	djangotest.EachDB(t, viewModels(), func(t *testing.T, db *orm.DB) {
		client, d := viewsSetUp(t, db)
		for _, obj := range d.authors {
			t.Run(obj.Name, func(t *testing.T) {
				resp := client.Get(shortcutURL(t, db, &Author{}, obj.ID))
				djangotest.AssertRedirects(t, resp, "http://testserver"+obj.GetAbsoluteURL(),
					djangotest.TargetStatusCode(http.StatusNotFound))
			})
		}
	})
}

// Ported from Django: tests/contenttypes_tests/test_views.py::ContentTypesViewsTests.test_shortcut_with_absolute_url_including_scheme
func TestShortcutWithAbsoluteURLIncludingScheme(t *testing.T) {
	djangotest.EachDB(t, viewModels(), func(t *testing.T, db *orm.DB) {
		client, d := viewsSetUp(t, db)
		for _, obj := range d.schemes {
			t.Run(obj.URL, func(t *testing.T) {
				resp := client.Get(shortcutURL(t, db, &SchemeIncludedURL{}, obj.ID))
				djangotest.AssertRedirects(t, resp, obj.GetAbsoluteURL(), djangotest.FetchRedirectResponse(false))
			})
		}
	})
}

// Ported from Django: tests/contenttypes_tests/test_views.py::ContentTypesViewsTests.test_shortcut_no_absolute_url
func TestShortcutNoAbsoluteURL(t *testing.T) {
	djangotest.EachDB(t, viewModels(), func(t *testing.T, db *orm.DB) {
		client, d := viewsSetUp(t, db)
		for _, obj := range d.articles {
			t.Run(obj.Title, func(t *testing.T) {
				resp := client.Get(shortcutURL(t, db, &Article{}, obj.ID))
				djangotest.AssertEqual(t, resp.StatusCode, http.StatusNotFound)
			})
		}
	})
}

// Ported from Django: tests/contenttypes_tests/test_views.py::ContentTypesViewsTests.test_wrong_type_pk
func TestShortcutWrongTypePK(t *testing.T) {
	djangotest.EachDB(t, viewModels(), func(t *testing.T, db *orm.DB) {
		client, _ := viewsSetUp(t, db)
		resp := client.Get(shortcutURL(t, db, &Author{}, "nobody/expects"))
		djangotest.AssertEqual(t, resp.StatusCode, http.StatusNotFound)
	})
}

// Ported from Django: tests/contenttypes_tests/test_views.py::ContentTypesViewsTests.test_shortcut_bad_pk
func TestShortcutBadPK(t *testing.T) {
	djangotest.EachDB(t, viewModels(), func(t *testing.T, db *orm.DB) {
		client, _ := viewsSetUp(t, db)
		resp := client.Get(shortcutURL(t, db, &Author{}, "42424242"))
		djangotest.AssertEqual(t, resp.StatusCode, http.StatusNotFound)
	})
}

// Ported from Django: tests/contenttypes_tests/test_views.py::ContentTypesViewsTests.test_nonint_content_type
func TestShortcutNonintContentType(t *testing.T) {
	djangotest.EachDB(t, viewModels(), func(t *testing.T, db *orm.DB) {
		client, d := viewsSetUp(t, db)
		resp := client.Get(fmt.Sprintf("/shortcut/%s/%d/", "spam", d.authors[0].ID))
		djangotest.AssertEqual(t, resp.StatusCode, http.StatusNotFound)
	})
}

// Ported from Django: tests/contenttypes_tests/test_views.py::ContentTypesViewsTests.test_bad_content_type
func TestShortcutBadContentType(t *testing.T) {
	djangotest.EachDB(t, viewModels(), func(t *testing.T, db *orm.DB) {
		client, d := viewsSetUp(t, db)
		resp := client.Get(fmt.Sprintf("/shortcut/%d/%d/", 42424242, d.authors[0].ID))
		djangotest.AssertEqual(t, resp.StatusCode, http.StatusNotFound)
	})
}

// FIXME-PORT: tests/contenttypes_tests/test_views.py::ContentTypesViewsSiteRelTests
// (test_shortcut_view_with_null_site_fk and test_shortcut_view_with_site_m2m)
// find the domain through a relation to the Site model of the sites
// framework, which Go does not have.

// shortcut calls the view of ContentTypeShortcut for content type ctID and
// object objectID with the request of ShortcutViewTests.setUp, as Django's
// tests call shortcut(self.request, ...) directly. It returns the response
// and the error of the view.
func shortcut(t *testing.T, db *orm.DB, ctID int64, objectID string) (*httptest.ResponseRecorder, error) {
	t.Helper()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Host = "Example.com"
	r.SetPathValue("content_type_id", strconv.FormatInt(ctID, 10))
	r.SetPathValue("object_id", objectID)
	w := httptest.NewRecorder()
	return w, views.ContentTypeShortcut(db)(w, r)
}

// The sites part of the Django test has no Go counterpart: Go has no sites
// framework, so the view always takes the domain from the request.
//
// Ported from Django: tests/contenttypes_tests/test_views.py::ShortcutViewTests.test_not_dependent_on_sites_app
func TestShortcutNotDependentOnSitesApp(t *testing.T) {
	djangotest.EachDB(t, viewModels(), func(t *testing.T, db *orm.DB) {
		ct := mustGet(t, db, &FooWithURL{})
		obj := mustCreate(t, db, &FooWithURL{Name: "john"})
		resp, err := shortcut(t, db, ct.ID, strconv.FormatInt(obj.ID, 10))
		if err != nil {
			t.Fatalf("shortcut: %v", err)
		}
		djangotest.AssertEqual(t, resp.Header().Get("Location"), "http://Example.com/users/john/")
	})
}

// Ported from Django: tests/contenttypes_tests/test_views.py::ShortcutViewTests.test_model_without_get_absolute_url
func TestShortcutModelWithoutGetAbsoluteURL(t *testing.T) {
	djangotest.EachDB(t, viewModels(), func(t *testing.T, db *orm.DB) {
		ct := mustGet(t, db, &FooWithoutURL{})
		obj := mustCreate(t, db, &FooWithoutURL{Name: "john"})
		_, err := shortcut(t, db, ct.ID, strconv.FormatInt(obj.ID, 10))
		if !errors.Is(err, views.ErrNotFound) {
			t.Fatalf("shortcut: err = %v, want a 404", err)
		}
	})
}

// No Go counterpart: tests/contenttypes_tests/test_views.py::ShortcutViewTests.test_model_with_broken_get_absolute_url
// checks that the view lets the AttributeError of a broken
// get_absolute_url propagate. A Go method that compiles cannot name a missing
// field.

// Ported from Django: tests/contenttypes_tests/test_views.py::ShortcutViewTests.test_invalid_uuid_pk_raises_404
func TestShortcutInvalidUUIDPKRaises404(t *testing.T) {
	djangotest.EachDB(t, viewModels(), func(t *testing.T, db *orm.DB) {
		ct := mustGet(t, db, &UUIDModel{})
		invalidUUID := "1234-zzzz-5678-0000-invaliduuid"
		_, err := shortcut(t, db, ct.ID, invalidUUID)
		if !errors.Is(err, views.ErrNotFound) {
			t.Fatalf("shortcut: err = %v, want a 404", err)
		}
		djangotest.AssertEqual(t, err.Error(), fmt.Sprintf("Content type %d object %s doesn’t exist", ct.ID, invalidUUID))
	})
}
