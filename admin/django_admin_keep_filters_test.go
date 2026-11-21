package admin_test

import (
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/admin"
	"github.com/oliverhaas/djangogo/auth"
	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/urls"
)

// keepFilters holds the environment of AdminKeepChangeListFiltersTests on
// one site: the user admin, the superuser logged in, and joepublic.
type keepFilters struct {
	env  *adminTestEnv
	site string
	joe  *auth.User
}

// keepFiltersSites are the sites of AdminKeepChangeListFiltersTests and of
// NamespacedAdminKeepChangeListFiltersTests, which runs its tests on site2.
var keepFiltersSites = []string{"admin", "namespaced_admin"}

// eachKeepFiltersSite runs fn on both sites and both databases.
func eachKeepFiltersSite(t *testing.T, fn func(t *testing.T, db *orm.DB, k *keepFilters)) {
	t.Helper()
	withoutPasswordValidators(t)
	for _, siteName := range keepFiltersSites {
		t.Run(siteName, func(t *testing.T) {
			djangotest.EachDB(t, adminViewsModels(), func(t *testing.T, db *orm.DB) {
				env := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
					if siteName != "admin" {
						site.Name = siteName
						site.Prefix = "/test_admin/admin2"
					}
					admin.Register[auth.User](site, admin.UserAdmin(db))
				})
				super := createTestUser(t, db, "super", "secret", true, true)
				joe := createTestUser(t, db, "joepublic", "secret", false, false)
				env.client.ForceLogin(super)
				fn(t, db, &keepFilters{env: env, site: siteName, joe: joe})
			})
		})
	}
}

// keepFiltersURLEqual reports whether two URLs are equal despite the order of
// their query string and of their _changelist_filters, as
// AdminKeepChangeListFiltersTests.assertURLEqual.
func keepFiltersURLEqual(url1, url2 string) bool {
	parse := func(raw string) (string, map[string]any) {
		u, err := url.Parse(raw)
		if err != nil {
			return raw, nil
		}
		qs := parseQSLMap(u.RawQuery)
		if f, ok := qs["_changelist_filters"]; ok {
			qs["_changelist_filters"] = parseQSLMap(f.(string))
		}
		return u.Path, qs
	}
	path1, qs1 := parse(url1)
	path2, qs2 := parse(url2)
	return path1 == path2 && reflect.DeepEqual(qs1, qs2)
}

// parseQSLMap returns dict(parse_qsl(q)): the last value of each key, without
// the keys of blank values.
func parseQSLMap(q string) map[string]any {
	out := make(map[string]any)
	for part := range strings.SplitSeq(q, "&") {
		k, v, _ := strings.Cut(part, "=")
		k, _ = url.QueryUnescape(k)
		v, _ = url.QueryUnescape(v)
		if v != "" {
			out[k] = v
		}
	}
	return out
}

// assertKeepFiltersURLEqual fails t unless keepFiltersURLEqual holds.
func assertKeepFiltersURLEqual(t *testing.T, url1, url2 string) {
	t.Helper()
	if !keepFiltersURLEqual(url1, url2) {
		t.Errorf("URL %q != %q", url1, url2)
	}
}

// assertKeepFiltersRedirects asserts that resp redirects to want, which loads,
// as assertRedirects with the assertURLEqual of the class.
func (k *keepFilters) assertRedirects(t *testing.T, resp *djangotest.Response, want string) {
	t.Helper()
	djangotest.AssertStatus(t, resp, http.StatusFound)
	assertKeepFiltersURLEqual(t, resp.URL(), want)
	djangotest.AssertStatus(t, k.env.client.Get(resp.URL()), http.StatusOK)
}

// changelistFiltersQuerystring is get_changelist_filters_querystring().
const changelistFiltersQuerystring = "is_superuser__exact=0&is_staff__exact=0"

// preservedFiltersQuerystring is get_preserved_filters_querystring().
var preservedFiltersQuerystring = url.Values{"_changelist_filters": {changelistFiltersQuerystring}}.Encode()

// reverse returns the URL of the user view name on the site of k.
func (k *keepFilters) reverse(t *testing.T, name string, args ...any) string {
	t.Helper()
	return mustReverse(t, k.env.router, k.site+":auth_user_"+name, args...)
}

// changelistURL is get_changelist_url().
func (k *keepFilters) changelistURL(t *testing.T) string {
	t.Helper()
	return k.reverse(t, "changelist") + "?" + changelistFiltersQuerystring
}

// addURL is get_add_url().
func (k *keepFilters) addURL(t *testing.T, addPreservedFilters bool) string {
	t.Helper()
	u := k.reverse(t, "add")
	if addPreservedFilters {
		u += "?" + preservedFiltersQuerystring
	}
	return u
}

// changeURL is get_change_url(); a userID of 0 means joepublic.
func (k *keepFilters) changeURL(t *testing.T, userID int64, addPreservedFilters bool) string {
	t.Helper()
	if userID == 0 {
		userID = k.joe.ID
	}
	u := k.reverse(t, "change", userID)
	if addPreservedFilters {
		u += "?" + preservedFiltersQuerystring
	}
	return u
}

// historyURL is get_history_url() of joepublic.
func (k *keepFilters) historyURL(t *testing.T) string {
	t.Helper()
	return k.reverse(t, "history", k.joe.ID) + "?" + preservedFiltersQuerystring
}

// deleteURL is get_delete_url() of joepublic.
func (k *keepFilters) deleteURL(t *testing.T) string {
	t.Helper()
	return k.reverse(t, "delete", k.joe.ID) + "?" + preservedFiltersQuerystring
}

// search returns the first group of re in the body of resp.
func search(t *testing.T, resp *djangotest.Response, re string) string {
	t.Helper()
	m := regexp.MustCompile(re).FindStringSubmatch(resp.Content())
	if m == nil {
		t.Fatalf("no match of %s in the response", re)
	}
	return m[1]
}

// Ported from Django: tests/admin_views/tests.py::AdminKeepChangeListFiltersTests.test_assert_url_equal
// Ported from Django: tests/admin_views/tests.py::NamespacedAdminKeepChangeListFiltersTests.test_assert_url_equal
func TestKeepChangeListFiltersAssertURLEqual(t *testing.T) {
	eachKeepFiltersSite(t, func(t *testing.T, _ *orm.DB, k *keepFilters) {
		changeUserURL := k.reverse(t, "change", k.joe.ID)
		// Test equality.
		assertKeepFiltersURLEqual(t,
			"http://testserver"+changeUserURL+"?_changelist_filters=is_staff__exact%3D0%26is_superuser__exact%3D0",
			"http://testserver"+changeUserURL+"?_changelist_filters=is_staff__exact%3D0%26is_superuser__exact%3D0")
		// Test inequality.
		if keepFiltersURLEqual(
			"http://testserver"+changeUserURL+"?_changelist_filters=is_staff__exact%3D0%26is_superuser__exact%3D0",
			"http://testserver"+changeUserURL+"?_changelist_filters=is_staff__exact%3D1%26is_superuser__exact%3D1") {
			t.Error("URLs with other filters are equal")
		}
		// Ignore scheme and host.
		assertKeepFiltersURLEqual(t,
			"http://testserver"+changeUserURL+"?_changelist_filters=is_staff__exact%3D0%26is_superuser__exact%3D0",
			changeUserURL+"?_changelist_filters=is_staff__exact%3D0%26is_superuser__exact%3D0")
		// Ignore ordering of querystring.
		changelist := k.reverse(t, "changelist")
		assertKeepFiltersURLEqual(t,
			changelist+"?is_staff__exact=0&is_superuser__exact=0",
			changelist+"?is_superuser__exact=0&is_staff__exact=0")
		// Ignore ordering of _changelist_filters.
		assertKeepFiltersURLEqual(t,
			changeUserURL+"?_changelist_filters=is_staff__exact%3D0%26is_superuser__exact%3D0",
			changeUserURL+"?_changelist_filters=is_superuser__exact%3D0%26is_staff__exact%3D0")
	})
}

// Ported from Django: tests/admin_views/tests.py::AdminKeepChangeListFiltersTests.test_changelist_view
// Ported from Django: tests/admin_views/tests.py::NamespacedAdminKeepChangeListFiltersTests.test_changelist_view
func TestKeepChangeListFiltersChangelistView(t *testing.T) {
	eachKeepFiltersSite(t, func(t *testing.T, _ *orm.DB, k *keepFilters) {
		resp := k.env.client.Get(k.changelistURL(t))
		djangotest.AssertStatus(t, resp, http.StatusOK)
		// Check the `change_view` link has the correct querystring.
		detailLink := search(t, resp, fmt.Sprintf(`<a href="(.*?)">%s</a>`, k.joe.Username))
		assertKeepFiltersURLEqual(t, detailLink, k.changeURL(t, 0, true))
	})
}

// Ported from Django: tests/admin_views/tests.py::AdminKeepChangeListFiltersTests.test_change_view
// Ported from Django: tests/admin_views/tests.py::NamespacedAdminKeepChangeListFiltersTests.test_change_view
func TestKeepChangeListFiltersChangeView(t *testing.T) {
	eachKeepFiltersSite(t, func(t *testing.T, _ *orm.DB, k *keepFilters) {
		// Get the `change_view`.
		resp := k.env.client.Get(k.changeURL(t, 0, true))
		djangotest.AssertStatus(t, resp, http.StatusOK)

		// Check the form action.
		formAction := search(t, resp, `<form action="(.*?)" method="post" id="user_form" novalidate>`)
		assertKeepFiltersURLEqual(t, formAction, "?"+preservedFiltersQuerystring)

		// Check the history link.
		historyLink := search(t, resp, `<a href="(.*?)" class="historylink">History</a>`)
		assertKeepFiltersURLEqual(t, historyLink, k.historyURL(t))

		// Check the delete link.
		deleteLink := search(t, resp, `<a role="button" href="(.*?)" class="deletelink">Delete</a>`)
		assertKeepFiltersURLEqual(t, deleteLink, k.deleteURL(t))

		// Test redirect on "Save".
		postData := url.Values{
			"username":      {"joepublic"},
			"last_login_0":  {"2007-05-30"},
			"last_login_1":  {"13:20:10"},
			"date_joined_0": {"2007-05-30"},
			"date_joined_1": {"13:20:10"},
		}
		postData.Set("_save", "1")
		resp = k.env.client.Post(k.changeURL(t, 0, true), postData)
		k.assertRedirects(t, resp, k.changelistURL(t))
		postData.Del("_save")

		// Test redirect on "Save and continue".
		postData.Set("_continue", "1")
		resp = k.env.client.Post(k.changeURL(t, 0, true), postData)
		k.assertRedirects(t, resp, k.changeURL(t, 0, true))
		postData.Del("_continue")

		// Test redirect on "Save and add new".
		postData.Set("_addanother", "1")
		resp = k.env.client.Post(k.changeURL(t, 0, true), postData)
		k.assertRedirects(t, resp, k.addURL(t, true))
	})
}

// Ported from Django: tests/admin_views/tests.py::AdminKeepChangeListFiltersTests.test_change_view_close_link
// Ported from Django: tests/admin_views/tests.py::NamespacedAdminKeepChangeListFiltersTests.test_change_view_close_link
func TestKeepChangeListFiltersChangeViewCloseLink(t *testing.T) {
	eachKeepFiltersSite(t, func(t *testing.T, db *orm.DB, k *keepFilters) {
		viewuser := createTestUser(t, db, "view", "secret", true, false)
		grantPerm(t, db, viewuser, "auth", "view_user")
		k.env.client.ForceLogin(viewuser)
		resp := k.env.client.Get(k.changeURL(t, 0, true))
		closeLink := search(t, resp, `<a role="button" href="(.*?)" class="closelink">Close</a>`)
		closeLink = strings.ReplaceAll(closeLink, "&amp;", "&")
		assertKeepFiltersURLEqual(t, closeLink, k.changelistURL(t))
	})
}

// Ported from Django: tests/admin_views/tests.py::AdminKeepChangeListFiltersTests.test_change_view_without_preserved_filters
// Ported from Django: tests/admin_views/tests.py::NamespacedAdminKeepChangeListFiltersTests.test_change_view_without_preserved_filters
func TestKeepChangeListFiltersChangeViewWithoutPreservedFilters(t *testing.T) {
	eachKeepFiltersSite(t, func(t *testing.T, _ *orm.DB, k *keepFilters) {
		resp := k.env.client.Get(k.changeURL(t, 0, false))
		// The action attribute is omitted.
		djangotest.AssertContains(t, resp, `<form method="post" id="user_form" novalidate>`)
	})
}

// Ported from Django: tests/admin_views/tests.py::AdminKeepChangeListFiltersTests.test_add_view
// Ported from Django: tests/admin_views/tests.py::NamespacedAdminKeepChangeListFiltersTests.test_add_view
func TestKeepChangeListFiltersAddView(t *testing.T) {
	eachKeepFiltersSite(t, func(t *testing.T, db *orm.DB, k *keepFilters) {
		// Get the `add_view`.
		resp := k.env.client.Get(k.addURL(t, true))
		djangotest.AssertStatus(t, resp, http.StatusOK)

		// Check the form action.
		formAction := search(t, resp, `<form action="(.*?)" method="post" id="user_form" novalidate>`)
		assertKeepFiltersURLEqual(t, formAction, "?"+preservedFiltersQuerystring)

		postData := url.Values{
			"username":  {"dummy"},
			"password1": {"test"},
			"password2": {"test"},
		}
		userID := func(username string) int64 {
			t.Helper()
			u, err := orm.Query[auth.User](db).Get(t.Context(), "username", username)
			if err != nil {
				t.Fatalf("get user %s: %v", username, err)
			}
			return u.ID
		}

		// Test redirect on "Save".
		postData.Set("_save", "1")
		resp = k.env.client.Post(k.addURL(t, true), postData)
		k.assertRedirects(t, resp, k.changeURL(t, userID("dummy"), true))
		postData.Del("_save")

		// Test redirect on "Save and continue".
		postData.Set("username", "dummy2")
		postData.Set("_continue", "1")
		resp = k.env.client.Post(k.addURL(t, true), postData)
		k.assertRedirects(t, resp, k.changeURL(t, userID("dummy2"), true))
		postData.Del("_continue")

		// Test redirect on "Save and add new".
		postData.Set("username", "dummy3")
		postData.Set("_addanother", "1")
		resp = k.env.client.Post(k.addURL(t, true), postData)
		k.assertRedirects(t, resp, k.addURL(t, true))
	})
}

// Ported from Django: tests/admin_views/tests.py::AdminKeepChangeListFiltersTests.test_add_view_without_preserved_filters
// Ported from Django: tests/admin_views/tests.py::NamespacedAdminKeepChangeListFiltersTests.test_add_view_without_preserved_filters
func TestKeepChangeListFiltersAddViewWithoutPreservedFilters(t *testing.T) {
	eachKeepFiltersSite(t, func(t *testing.T, _ *orm.DB, k *keepFilters) {
		resp := k.env.client.Get(k.addURL(t, false))
		// The action attribute is omitted.
		djangotest.AssertContains(t, resp, `<form method="post" id="user_form" novalidate>`)
	})
}

// Ported from Django: tests/admin_views/tests.py::AdminKeepChangeListFiltersTests.test_delete_view
// Ported from Django: tests/admin_views/tests.py::NamespacedAdminKeepChangeListFiltersTests.test_delete_view
func TestKeepChangeListFiltersDeleteView(t *testing.T) {
	eachKeepFiltersSite(t, func(t *testing.T, _ *orm.DB, k *keepFilters) {
		// Test redirect on "Delete".
		resp := k.env.client.Post(k.deleteURL(t), url.Values{"post": {"yes"}})
		k.assertRedirects(t, resp, k.changelistURL(t))
	})
}

// Go has no script prefix: the prefix of the site stands in for it.
//
// Ported from Django: tests/admin_views/tests.py::AdminKeepChangeListFiltersTests.test_url_prefix
// Ported from Django: tests/admin_views/tests.py::NamespacedAdminKeepChangeListFiltersTests.test_url_prefix
func TestKeepChangeListFiltersURLPrefix(t *testing.T) {
	djangotest.EachDB(t, adminViewsModels(), func(t *testing.T, db *orm.DB) {
		for _, siteName := range keepFiltersSites {
			for _, prefix := range []string{"", "/prefix", "/後台"} {
				t.Run(siteName+prefix, func(t *testing.T) {
					site, err := admin.NewAdminSite(db)
					if err != nil {
						t.Fatalf("NewAdminSite: %v", err)
					}
					site.Name = siteName
					site.Prefix = prefix + "/test_admin/admin"
					admin.Register[auth.User](site, admin.UserAdmin(db))
					router := urls.NewRouter(site.Routes()...)
					changelist := mustReverse(t, router, siteName+":auth_user_changelist")
					assertKeepFiltersURLEqual(t,
						admin.AddPreservedFilters(site, "auth", "user", changelist, preservedFiltersQuerystring),
						changelist+"?"+changelistFiltersQuerystring)
				})
			}
		}
	})
}

// Color stands in for tests/admin_views/models.py::Color.
type Color struct {
	ID    int64
	Value string `orm:"max_length=10"`
	Warm  bool
}

// String returns the value.
func (c Color) String() string { return c.Value }

// queryStringPersistsEnv registers the user admin and Color on the site
// "admin", creates color1 and logs in the superuser, as
// AdminViewBasicTestCase.setUpTestData and setUp do.
func queryStringPersistsEnv(t *testing.T, db *orm.DB) (*adminTestEnv, *Color) {
	t.Helper()
	withoutPasswordValidators(t)
	env := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
		admin.Register[auth.User](site, admin.UserAdmin(db))
		admin.Register[Color](site, admin.ModelAdmin{})
	})
	env.client.ForceLogin(createTestUser(t, db, "super", "secret", true, true))
	return env, mustCreate(t, db, &Color{Value: "Red", Warm: true})
}

// Ported from Django: tests/admin_views/tests.py::AdminViewBasicTest.test_add_query_string_persists
func TestAddQueryStringPersists(t *testing.T) {
	models := append(adminViewsModels(), djangotest.App("admin_views", &Color{}))
	djangotest.EachDB(t, models, func(t *testing.T, db *orm.DB) {
		env, _ := queryStringPersistsEnv(t, db)
		saveOptions := []string{
			"_addanother", // "Save and add another".
			"_continue",   // "Save and continue editing".
			"_saveasnew",  // "Save as new".
		}
		otherOptions := []string{
			"",
			"_changelist_filters=is_staff__exact%3D0",
			"_popup=1",
			"_to_field=id",
		}
		u := mustReverse(t, env.router, "admin:auth_user_add")
		for i, saveOption := range saveOptions {
			for j, otherOption := range otherOptions {
				t.Run(saveOption+"/"+otherOption, func(t *testing.T) {
					qsl := "username=newuser"
					if otherOption != "" {
						qsl += "&" + otherOption
					}
					resp := env.client.Post(u+"?"+qsl, url.Values{
						"username":  {fmt.Sprintf("newuser%d%d", i, j)},
						"password1": {"newpassword"},
						"password2": {"newpassword"},
						saveOption:  {"1"},
					})
					parsed, err := url.Parse(resp.URL())
					if err != nil {
						t.Fatalf("parse %q: %v", resp.URL(), err)
					}
					djangotest.AssertEqual(t, parsed.RawQuery, qsl)
				})
			}
		}
	})
}

// Ported from Django: tests/admin_views/tests.py::AdminViewBasicTest.test_change_query_string_persists
func TestChangeQueryStringPersists(t *testing.T) {
	models := append(adminViewsModels(), djangotest.App("admin_views", &Color{}))
	djangotest.EachDB(t, models, func(t *testing.T, db *orm.DB) {
		env, color1 := queryStringPersistsEnv(t, db)
		saveOptions := []string{
			"_addanother", // "Save and add another".
			"_continue",   // "Save and continue editing".
		}
		otherOptions := []string{
			"",
			"_changelist_filters=warm%3D1",
			"_popup=1",
			"_to_field=id",
		}
		u := mustReverse(t, env.router, "admin:admin_views_color_change", color1.ID)
		for _, saveOption := range saveOptions {
			for _, otherOption := range otherOptions {
				t.Run(saveOption+"/"+otherOption, func(t *testing.T) {
					qsl := "value=blue"
					if otherOption != "" {
						qsl += "&" + otherOption
					}
					resp := env.client.Post(u+"?"+qsl, url.Values{
						"value":    {"gold"},
						"warm":     {"True"},
						saveOption: {"1"},
					})
					parsed, err := url.Parse(resp.URL())
					if err != nil {
						t.Fatalf("parse %q: %v", resp.URL(), err)
					}
					djangotest.AssertEqual(t, parsed.RawQuery, qsl)
				})
			}
		}
	})
}
