package admin_test

import (
	"context"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/admin"
	"github.com/oliverhaas/djangogo/auth"
	"github.com/oliverhaas/djangogo/contenttypes"
	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/forms"
	"github.com/oliverhaas/djangogo/orm"
)

// ROPost stands in for tests/admin_views/models.py::Post. Django's posted
// defaults to date.today, a callable a tag cannot express; a fixed date stands
// in for it.
type ROPost struct {
	ID              int64
	Title           string   `orm:"max_length=100;help_text=Some help text for the title (with Unicode ŠĐĆŽćžšđ)"`
	Content         string   `orm:"type=text;help_text=Some help text for the content (with Unicode ŠĐĆŽćžšđ)"`
	ReadonlyContent string   `orm:"type=text"`
	Posted          orm.Date `orm:"default=2000-01-01;help_text=Some help text for the date (with Unicode ŠĐĆŽćžšđ)"`
	Public          *bool    `orm:"null;blank"`
}

// ROLink stands in for tests/admin_views/models.py::Link. Django's posted
// defaults to a week before today; a week before the fixed default of
// ROPost.Posted stands in for it. The related name keeps Django's inline
// prefix "link_set".
type ROLink struct {
	ID                  int64
	Posted              orm.Date       `orm:"default=1999-12-25"`
	URL                 string         `orm:"type=url"`
	Post                orm.FK[ROPost] `orm:"on_delete=cascade;related_name=link_set"`
	ReadonlyLinkContent string         `orm:"type=text"`
}

// ROChoice stands in for tests/admin_views/models.py::Choice.
type ROChoice struct {
	ID     int64
	Choice *int64 `orm:"null;blank"`
}

// Choices declares the options of Choice, with a label for NULL.
func (ROChoice) Choices() map[string][]orm.Choice {
	return map[string][]orm.Choice{"Choice": {
		orm.ChoiceOf(1, "Yes"), orm.ChoiceOf(0, "No"), {Value: "", Label: "No opinion"},
	}}
}

// ROBook stands in for tests/admin_views/models.py::Book.
type ROBook struct {
	ID   int64
	Name string `orm:"max_length=100;verbose_name=¿Name?"`
}

// String returns the book name.
func (b ROBook) String() string { return b.Name }

// ROChapter stands in for tests/admin_views/models.py::Chapter.
type ROChapter struct {
	ID      int64
	Title   string `orm:"max_length=100;verbose_name=¿Title?"`
	Content string `orm:"type=text"`
	Book    orm.FK[ROBook]
}

// String returns the chapter title.
func (c ROChapter) String() string { return c.Title }

// ROReadOnlyRelatedField stands in for
// tests/admin_views/models.py::ReadOnlyRelatedField. The language foreign key
// is left out: its target has a string primary key, which orm.FK does not
// support.
type ROReadOnlyRelatedField struct {
	ID      int64
	Chapter orm.FK[ROChapter]
	User    orm.FK[auth.User]
}

// LVEmptyModelVisible stands in for
// tests/admin_views/models.py::EmptyModelVisible.
type LVEmptyModelVisible struct {
	ID int64
}

// LVEmptyModelHidden stands in for
// tests/admin_views/models.py::EmptyModelHidden.
type LVEmptyModelHidden struct {
	ID int64
}

// LVEmptyModelMixin stands in for
// tests/admin_views/models.py::EmptyModelMixin.
type LVEmptyModelMixin struct {
	ID int64
}

// readonlyModels returns the model set of the ported ReadonlyTest tests.
func readonlyModels() []any {
	return []any{auth.App{}, contenttypes.App{}, djangotest.App("admin_views",
		&Section{}, &ROPost{}, &ROLink{}, &ROChoice{}, &ROBook{}, &ROChapter{}, &ROReadOnlyRelatedField{},
		&LVEmptyModelVisible{}, &LVEmptyModelHidden{}, &LVEmptyModelMixin{},
	)}
}

// roPostAdmin is PostAdmin of tests/admin_views/admin.py without its lambda,
// which has no name a Go map key could hold. Its inline is LinkInline.
func roPostAdmin() admin.ModelAdmin {
	return admin.ModelAdmin{
		Inlines: []admin.Inline{admin.TabularInline[ROPost, ROLink](admin.InlineModelAdmin{
			Extra:          new(1),
			ReadonlyFields: []string{"posted", "multiline", "readonly_link_content"},
			DisplayColumns: map[string]admin.DisplayColumn{
				// A method of the inline in Django.
				"multiline": {Value: func(any) any { return "InlineMultiline\ntest\nstring" }},
			},
		})},
		ListDisplay: []string{"title", "public"},
		ReadonlyFields: []string{
			"posted", "awesomeness_level", "coolness", "value", "multiline", "multiline_html", "readonly_content",
		},
		DisplayColumns: map[string]admin.DisplayColumn{
			// A method of the model in Django.
			"awesomeness_level": {Value: func(any) any { return "Very awesome." }},
			"coolness": {Value: func(obj any) any {
				if p := obj.(*ROPost); p.ID != 0 {
					return fmt.Sprintf("%d amount of cool.", p.ID)
				}
				return "Unknown coolness."
			}},
			"value":     {Description: "Value in $US", Value: func(any) any { return 1000 }},
			"multiline": {Value: func(any) any { return "Multiline\ntest\nstring" }},
			"multiline_html": {Value: func(any) any {
				return template.HTML("Multiline<br>\nhtml<br>\ncontent")
			}},
		},
	}
}

// readonlyEnv registers the admins of the ReadonlyTest tests under the site
// name and logs in a superuser.
func readonlyEnv(t *testing.T, db *orm.DB, siteName string) (*adminTestEnv, *auth.User) {
	t.Helper()
	env := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
		site.Name = siteName
		admin.Register[ROPost](site, roPostAdmin())
		admin.Register[ROChoice](site, admin.ModelAdmin{
			ListDisplay: []string{"choice"}, ReadonlyFields: []string{"choice"}, Fields: []any{"choice"},
		})
		admin.Register[ROReadOnlyRelatedField](site, admin.ModelAdmin{ReadonlyFields: []string{"chapter", "user"}})
		admin.Register[Section](site, admin.ModelAdmin{
			ReadonlyFields: []string{"name_property"},
			SearchFields:   []string{"name"},
			DisplayColumns: map[string]admin.DisplayColumn{
				// A property of the model in Django.
				"name_property": {Value: func(obj any) any { return obj.(*Section).Name }},
			},
		})
		admin.Register[auth.User](site, admin.ModelAdmin{})
		fieldsets := []admin.Fieldset{{Fields: []any{[]string{"first", "second"}}}}
		hidden := func() forms.Field { return forms.NewCharField(forms.UseWidget(forms.HiddenInput{})) }
		admin.Register[LVEmptyModelVisible](site, admin.ModelAdmin{
			Fieldsets: fieldsets,
			FormOptions: []forms.ModelFormOption{
				forms.WithFormField("first", forms.NewCharField()),
				forms.WithFormField("second", forms.NewCharField()),
			},
		})
		admin.Register[LVEmptyModelHidden](site, admin.ModelAdmin{
			Fieldsets: fieldsets,
			FormOptions: []forms.ModelFormOption{
				forms.WithFormField("first", hidden()),
				forms.WithFormField("second", hidden()),
			},
		})
		admin.Register[LVEmptyModelMixin](site, admin.ModelAdmin{
			Fieldsets: fieldsets,
			FormOptions: []forms.ModelFormOption{
				forms.WithFormField("first", hidden()),
				forms.WithFormField("second", forms.NewCharField()),
			},
		})
	})
	super := createTestUser(t, db, "super", "secret", true, true)
	env.client.ForceLogin(super)
	return env, super
}

// The add page has no lambda column, so the Django assertions on it ("foo"
// and the form row without a field class) are left out.
//
// Ported from Django: tests/admin_views/tests.py::ReadonlyTest.test_readonly_get
func TestReadonlyGet(t *testing.T) {
	djangotest.EachDB(t, readonlyModels(), func(t *testing.T, db *orm.DB) {
		env, _ := readonlyEnv(t, db, "admin")
		resp := env.client.Get(mustReverse(t, env.router, "admin:admin_views_ropost_add"))
		djangotest.AssertNotContains(t, resp, `name="posted"`)
		// 3 fields + 2 submit buttons + 5 inline management form fields, + 2
		// hidden fields for inlines + 1 field for the inline + 2 empty form
		// + 1 logout form.
		djangotest.AssertContains(t, resp, "<input", djangotest.Count(17))
		djangotest.AssertContains(t, resp, "Jan. 1, 2000")
		djangotest.AssertContains(t, resp, "<label>Awesomeness level:</label>")
		djangotest.AssertContains(t, resp, "Very awesome.")
		djangotest.AssertContains(t, resp, "Unknown coolness.")

		// Multiline text in a readonly field gets <br> tags.
		djangotest.AssertContains(t, resp, "Multiline<br>test<br>string")
		djangotest.AssertContains(t, resp, `<div class="readonly">Multiline<br>html<br>content</div>`, djangotest.HTML())
		djangotest.AssertContains(t, resp, "InlineMultiline<br>test<br>string")

		djangotest.AssertContains(t, resp, "Dec. 25, 1999")

		djangotest.AssertContains(t, resp, `<div class="form-row field-coolness">`)
		djangotest.AssertContains(t, resp, `<div class="form-row field-awesomeness_level">`)
		djangotest.AssertContains(t, resp, `<div class="form-row field-posted">`)
		djangotest.AssertContains(t, resp, `<div class="form-row field-value">`)
		djangotest.AssertContains(t, resp, `<div class="help"`, djangotest.Count(3))
		djangotest.AssertContains(t, resp,
			`<div class="help" id="id_title_helptext"><div>Some help text for the `+
				`title (with Unicode ŠĐĆŽćžšđ)</div></div>`, djangotest.HTML())
		djangotest.AssertContains(t, resp,
			`<div class="help" id="id_content_helptext"><div>Some help text for the `+
				`content (with Unicode ŠĐĆŽćžšđ)</div></div>`, djangotest.HTML())
		djangotest.AssertContains(t, resp,
			`<div class="help"><div>Some help text for the date (with Unicode ŠĐĆŽćžšđ)</div></div>`,
			djangotest.HTML())

		p := &ROPost{Title: "I worked on readonly_fields", Content: "Its good stuff"}
		if err := orm.Query[ROPost](db).Create(context.Background(), p); err != nil {
			t.Fatalf("Create(ROPost): %v", err)
		}
		resp = env.client.Get(mustReverse(t, env.router, "admin:admin_views_ropost_change", p.ID))
		djangotest.AssertContains(t, resp, fmt.Sprintf("%d amount of cool", p.ID))
	})
}

// Ported from Django: tests/admin_views/tests.py::ReadonlyTest.test_readonly_text_field
func TestReadonlyTextField(t *testing.T) {
	djangotest.EachDB(t, readonlyModels(), func(t *testing.T, db *orm.DB) {
		env, _ := readonlyEnv(t, db, "admin")
		p := &ROPost{
			Title:           "Readonly test",
			Content:         "test",
			ReadonlyContent: "test\r\n\r\ntest\r\n\r\ntest\r\n\r\ntest",
		}
		if err := orm.Query[ROPost](db).Create(context.Background(), p); err != nil {
			t.Fatalf("Create(ROPost): %v", err)
		}
		link := &ROLink{URL: "http://www.djangoproject.com", ReadonlyLinkContent: "test\r\nlink"}
		link.Post.SetPK(p.ID)
		if err := orm.Query[ROLink](db).Create(context.Background(), link); err != nil {
			t.Fatalf("Create(ROLink): %v", err)
		}
		resp := env.client.Get(mustReverse(t, env.router, "admin:admin_views_ropost_change", p.ID))
		// Checking readonly field.
		djangotest.AssertContains(t, resp, "test<br><br>test<br><br>test<br><br>test")
		// Checking readonly field in inline.
		djangotest.AssertContains(t, resp, "test<br>link")
	})
}

// Ported from Django: tests/admin_views/tests.py::ReadonlyTest.test_readonly_post
func TestReadonlyPost(t *testing.T) {
	djangotest.EachDB(t, readonlyModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		env, _ := readonlyEnv(t, db, "admin")
		addURL := mustReverse(t, env.router, "admin:admin_views_ropost_add")
		data := url.Values{
			"title":                  {"Django Got Readonly Fields"},
			"content":                {"This is an incredible development."},
			"link_set-TOTAL_FORMS":   {"1"},
			"link_set-INITIAL_FORMS": {"0"},
			"link_set-MAX_NUM_FORMS": {"0"},
		}
		resp := env.client.Post(addURL, data)
		djangotest.AssertStatus(t, resp, http.StatusFound)
		posts, err := orm.Query[ROPost](db).OrderBy("id").All(ctx)
		if err != nil {
			t.Fatalf("All: %v", err)
		}
		djangotest.AssertEqual(t, len(posts), 1)
		djangotest.AssertEqual(t, posts[0].Posted, orm.MustDate("2000-01-01"))

		data.Set("posted", "10-8-1990") // some date that's not the default
		resp = env.client.Post(addURL, data)
		djangotest.AssertStatus(t, resp, http.StatusFound)
		posts, err = orm.Query[ROPost](db).OrderBy("-id").All(ctx)
		if err != nil {
			t.Fatalf("All: %v", err)
		}
		djangotest.AssertEqual(t, len(posts), 2)
		djangotest.AssertEqual(t, posts[0].Posted, orm.MustDate("2000-01-01"))
	})
}

// Ported from Django: tests/admin_views/tests.py::ReadonlyTest.test_change_form_renders_correct_null_choice_value
func TestChangeFormRendersCorrectNullChoiceValue(t *testing.T) {
	djangotest.EachDB(t, readonlyModels(), func(t *testing.T, db *orm.DB) {
		env, _ := readonlyEnv(t, db, "admin")
		choice := &ROChoice{}
		if err := orm.Query[ROChoice](db).Create(context.Background(), choice); err != nil {
			t.Fatalf("Create(ROChoice): %v", err)
		}
		resp := env.client.Get(mustReverse(t, env.router, "admin:admin_views_rochoice_change", choice.ID))
		djangotest.AssertContains(t, resp, `<div class="readonly">No opinion</div>`, djangotest.HTML())
	})
}

// readonlyForeignKeyLinks is ReadonlyTest._test_readonly_foreignkey_links: a
// readonly foreign key renders as a link when the related model has an admin.
// The language half is left out with the language foreign key.
func readonlyForeignKeyLinks(t *testing.T, siteName string) {
	djangotest.EachDB(t, readonlyModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		env, super := readonlyEnv(t, db, siteName)
		book := &ROBook{Name: "Book 1"}
		if err := orm.Query[ROBook](db).Create(ctx, book); err != nil {
			t.Fatalf("Create(ROBook): %v", err)
		}
		chapter := &ROChapter{Title: "Chapter 1", Content: "content"}
		chapter.Book.SetPK(book.ID)
		if err := orm.Query[ROChapter](db).Create(ctx, chapter); err != nil {
			t.Fatalf("Create(ROChapter): %v", err)
		}
		obj := &ROReadOnlyRelatedField{}
		obj.Chapter.SetPK(chapter.ID)
		obj.User.SetPK(super.ID)
		if err := orm.Query[ROReadOnlyRelatedField](db).Create(ctx, obj); err != nil {
			t.Fatalf("Create(ROReadOnlyRelatedField): %v", err)
		}
		resp := env.client.Get(mustReverse(t, env.router, siteName+":admin_views_roreadonlyrelatedfield_change", obj.ID))
		// Related ForeignKey object registered in admin.
		userURL := mustReverse(t, env.router, siteName+":auth_user_change", super.ID)
		djangotest.AssertContains(t, resp, `<div class="readonly"><a href="`+userURL+`">super</a></div>`, djangotest.HTML())
		// Related ForeignKey object not registered in admin.
		djangotest.AssertContains(t, resp, `<div class="readonly">Chapter 1</div>`, djangotest.HTML())
	})
}

// Ported from Django: tests/admin_views/tests.py::ReadonlyTest.test_readonly_foreignkey_links_default_admin_site
func TestReadonlyForeignKeyLinksDefaultAdminSite(t *testing.T) {
	readonlyForeignKeyLinks(t, "admin")
}

// Ported from Django: tests/admin_views/tests.py::ReadonlyTest.test_readonly_foreignkey_links_custom_admin_site
func TestReadonlyForeignKeyLinksCustomAdminSite(t *testing.T) {
	readonlyForeignKeyLinks(t, "namespaced_admin")
}

// Ported from Django: tests/admin_views/tests.py::ReadonlyTest.test_correct_autoescaping
func TestReadonlyCorrectAutoescaping(t *testing.T) {
	djangotest.EachDB(t, readonlyModels(), func(t *testing.T, db *orm.DB) {
		env, _ := readonlyEnv(t, db, "admin")
		section := &Section{Name: "<a>evil</a>"}
		if err := orm.Query[Section](db).Create(context.Background(), section); err != nil {
			t.Fatalf("Create(Section): %v", err)
		}
		resp := env.client.Get(mustReverse(t, env.router, "admin:admin_views_section_change", section.ID))
		djangotest.AssertStatus(t, resp, http.StatusOK)
		djangotest.AssertNotContains(t, resp, "<a>evil</a>")
		djangotest.AssertContains(t, resp, "&lt;a&gt;evil&lt;/a&gt;")
	})
}

// No Django counterpart: a readonly DisplayColumn that returns nil shows
// "None", as Django's linebreaksbr prints a callable's None.
func TestReadonlyDisplayColumnNil(t *testing.T) {
	djangotest.EachDB(t, readonlyModels(), func(t *testing.T, db *orm.DB) {
		env := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
			admin.Register[Section](site, admin.ModelAdmin{
				ReadonlyFields: []string{"nothing"},
				DisplayColumns: map[string]admin.DisplayColumn{"nothing": {Value: func(any) any { return nil }}},
			})
		})
		env.client.ForceLogin(createTestUser(t, db, "super", "secret", true, true))
		resp := env.client.Get(mustReverse(t, env.router, "admin:admin_views_section_add"))
		djangotest.AssertContains(t, resp, `<div class="readonly">None</div>`, djangotest.HTML())
	})
}

// labelVisibilityPage returns the add page of the model with the URL name.
func labelVisibilityPage(t *testing.T, db *orm.DB, name string) *djangotest.Response {
	t.Helper()
	env, _ := readonlyEnv(t, db, "admin")
	resp := env.client.Get(mustReverse(t, env.router, "admin:admin_views_"+name+"_add"))
	djangotest.AssertStatus(t, resp, http.StatusOK)
	return resp
}

// assertFieldVisible is TestLabelVisibility.assert_field_visible.
func assertFieldVisible(t *testing.T, resp *djangotest.Response, field string) {
	t.Helper()
	djangotest.AssertContains(t, resp, `<div class="flex-container fieldBox field-`+field+`">`)
}

// assertFieldHidden is TestLabelVisibility.assert_field_hidden.
func assertFieldHidden(t *testing.T, resp *djangotest.Response, field string) {
	t.Helper()
	djangotest.AssertContains(t, resp, `<div class="flex-container fieldBox field-`+field+` hidden">`)
}

// Ported from Django: tests/admin_views/tests.py::TestLabelVisibility.test_all_fields_visible
func TestLabelVisibilityAllFieldsVisible(t *testing.T) {
	djangotest.EachDB(t, readonlyModels(), func(t *testing.T, db *orm.DB) {
		resp := labelVisibilityPage(t, db, "lvemptymodelvisible")
		djangotest.AssertContains(t, resp, `<div class="form-row flex-container form-multiline field-first field-second">`)
		assertFieldVisible(t, resp, "first")
		assertFieldVisible(t, resp, "second")
	})
}

// Ported from Django: tests/admin_views/tests.py::TestLabelVisibility.test_all_fields_hidden
func TestLabelVisibilityAllFieldsHidden(t *testing.T) {
	djangotest.EachDB(t, readonlyModels(), func(t *testing.T, db *orm.DB) {
		resp := labelVisibilityPage(t, db, "lvemptymodelhidden")
		djangotest.AssertContains(t, resp,
			`<div class="form-row flex-container form-multiline hidden field-first field-second">`)
		assertFieldHidden(t, resp, "first")
		assertFieldHidden(t, resp, "second")
	})
}

// Ported from Django: tests/admin_views/tests.py::TestLabelVisibility.test_mixin
func TestLabelVisibilityMixin(t *testing.T) {
	djangotest.EachDB(t, readonlyModels(), func(t *testing.T, db *orm.DB) {
		resp := labelVisibilityPage(t, db, "lvemptymodelmixin")
		djangotest.AssertContains(t, resp, `<div class="form-row flex-container form-multiline field-first field-second">`)
		assertFieldHidden(t, resp, "first")
		assertFieldVisible(t, resp, "second")
	})
}

// Ported from Django: tests/admin_views/tests.py::AdminViewBasicTest.test_error_in_titles
func TestErrorInTitles(t *testing.T) {
	djangotest.EachDB(t, adminViewsModels(), func(t *testing.T, db *orm.DB) {
		super := createTestUser(t, db, "super", "secret", true, true)
		a1 := &Article{Title: "Article 1"}
		if err := orm.Query[Article](db).Create(context.Background(), a1); err != nil {
			t.Fatalf("Create(Article): %v", err)
		}
		env := newAdminTestEnv(t, db, registerBasicAdminModels(db))
		for _, tc := range []struct{ url, subtitle string }{
			{mustReverse(t, env.router, "admin:admin_views_article_change", a1.ID), "Article 1 | Change article"},
			{mustReverse(t, env.router, "admin:admin_views_article_add"), "Add article"},
			{mustReverse(t, env.router, "admin:login"), "Log in"},
			{mustReverse(t, env.router, "admin:password_change"), "Password change"},
			{mustReverse(t, env.router, "admin:auth_user_password_change", super.ID), "Change password: super"},
		} {
			t.Run(strings.Trim(tc.url, "/"), func(t *testing.T) {
				env.client.ForceLogin(super)
				resp := env.client.Post(tc.url, url.Values{})
				djangotest.AssertContains(t, resp, "<title>Error: "+tc.subtitle)
			})
		}
	})
}

// No Django counterpart: the errornote counts the fields with errors, not the
// messages, as the length of Django's AdminErrorList.
func TestErrorNoteCountsFields(t *testing.T) {
	djangotest.EachDB(t, readonlyModels(), func(t *testing.T, db *orm.DB) {
		env, _ := readonlyEnv(t, db, "admin")
		addURL := mustReverse(t, env.router, "admin:admin_views_ropost_add")
		// The management form of the LinkInline, with no forms.
		data := func(values url.Values) url.Values {
			values.Set("link_set-TOTAL_FORMS", "0")
			values.Set("link_set-INITIAL_FORMS", "0")
			return values
		}
		resp := env.client.Post(addURL, data(url.Values{"content": {"x"}}))
		djangotest.AssertContains(t, resp, "Please correct the error below.")
		djangotest.AssertContains(t, resp, `<div class="flex-container errors">`)
		resp = env.client.Post(addURL, data(url.Values{}))
		djangotest.AssertContains(t, resp, "Please correct the errors below.")
	})
}
