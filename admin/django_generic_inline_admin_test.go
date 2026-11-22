package admin_test

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"testing"

	"github.com/oliverhaas/djangogo/admin"
	"github.com/oliverhaas/djangogo/auth"
	"github.com/oliverhaas/djangogo/contenttypes"
	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/forms"
	"github.com/oliverhaas/djangogo/orm"
)

// Episode stands in for tests/generic_inline_admin/models.py::Episode.
type Episode struct {
	ID     int64
	Name   string `orm:"max_length=100"`
	Length string `orm:"max_length=100;blank"`
	Author string `orm:"max_length=100;blank"`
}

// Media stands in for tests/generic_inline_admin/models.py::Media.
type Media struct {
	ID            int64
	ContentType   orm.FK[contenttypes.ContentType]
	ObjectID      int64 `orm:"type=positive_int"`
	ContentObject contenttypes.GenericForeignKey
	URL           string `orm:"type=url"`
	Description   string `orm:"max_length=100;blank"`
	Keywords      string `orm:"max_length=100;blank"`
}

// String returns the URL, as Media.__str__ does.
func (m Media) String() string { return m.URL }

// GenericCategory stands in for tests/generic_inline_admin/models.py::Category.
// The package already has a Category.
type GenericCategory struct {
	ID   int64
	Name string `orm:"max_length=50"`
}

// PhoneNumber stands in for tests/generic_inline_admin/models.py::PhoneNumber.
type PhoneNumber struct {
	ID            int64
	ContentType   orm.FK[contenttypes.ContentType]
	ObjectID      int64 `orm:"type=positive_int"`
	ContentObject contenttypes.GenericForeignKey
	PhoneNumber   string                  `orm:"max_length=30"`
	Category      orm.FK[GenericCategory] `orm:"null;blank;on_delete=set_null"`
}

// Meta declares the unique_together of PhoneNumber.
func (PhoneNumber) Meta() orm.Meta {
	return orm.Meta{UniqueTogether: [][]string{{"content_type", "object_id", "phone_number"}}}
}

// GenericContact stands in for tests/generic_inline_admin/models.py::Contact.
// The package already has a Contact, so the admin URLs name the model
// "genericcontact".
type GenericContact struct {
	ID           int64
	Name         string                                    `orm:"max_length=50"`
	PhoneNumbers contenttypes.GenericRelation[PhoneNumber] `orm:"related_query_name=phone_numbers"`
}

// EpisodePermanent stands in for
// tests/generic_inline_admin/models.py::EpisodePermanent, a copy of Episode,
// since the ORM has no multi-table inheritance.
type EpisodePermanent struct {
	ID     int64
	Name   string `orm:"max_length=100"`
	Length string `orm:"max_length=100;blank"`
	Author string `orm:"max_length=100;blank"`
}

// genericInlineAdminModels returns the model set of the ported
// tests/generic_inline_admin/tests.py tests.
func genericInlineAdminModels() []any {
	return []any{auth.App{}, contenttypes.App{}, admin.App{}, djangotest.App("generic_inline_admin",
		&Episode{}, &Media{}, &GenericCategory{}, &PhoneNumber{}, &GenericContact{}, &EpisodePermanent{},
	)}
}

// mediaPrefix is the formset prefix of the Media inline.
const mediaPrefix = "generic_inline_admin-media-content_type-object_id"

// genericInlineEnv returns an environment with the admins of
// tests/generic_inline_admin/admin.py and a superuser logged in. episode
// replaces the inline of the Episode admin, MediaInline, when it is not nil.
func genericInlineEnv(t *testing.T, db *orm.DB, episode []admin.Inline) *adminTestEnv {
	t.Helper()
	if episode == nil {
		episode = []admin.Inline{admin.GenericTabularInline[Media](admin.InlineModelAdmin{})}
	}
	env := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
		admin.Register[Episode](site, admin.ModelAdmin{Inlines: episode})
		admin.Register[GenericContact](site, admin.ModelAdmin{Inlines: []admin.Inline{
			admin.GenericTabularInline[PhoneNumber](admin.InlineModelAdmin{}),
		}})
		admin.Register[GenericCategory](site, admin.ModelAdmin{})
		admin.Register[EpisodePermanent](site, admin.ModelAdmin{Inlines: []admin.Inline{
			admin.GenericTabularInline[Media](admin.InlineModelAdmin{CanDelete: new(false)}),
		}})
	})
	env.client.ForceLogin(genericSuperuser(t, db))
	return env
}

// genericSuperuser returns the superuser of TestDataMixin, created on the
// first call for db.
func genericSuperuser(t *testing.T, db *orm.DB) *auth.User {
	t.Helper()
	if u, err := orm.Query[auth.User](db).Get(context.Background(), "username", "super"); err == nil {
		return &u
	}
	return createTestUser(t, db, "super", "secret", true, true)
}

// createWithMedia creates an episode named "This Week in Django" with one
// Media row per URL attached through the generic foreign key, Django's
// _create_object and GenericAdminViewTest.setUp.
func createWithMedia[T any](t *testing.T, db *orm.DB, obj *T, urls ...string) (*T, []*Media) {
	t.Helper()
	ctx := context.Background()
	create(t, db, obj)
	media := make([]*Media, 0, len(urls))
	for _, u := range urls {
		m := &Media{URL: u}
		if err := contenttypes.SetObject(ctx, db, m, "content_object", obj); err != nil {
			t.Fatalf("SetObject: %v", err)
		}
		media = append(media, create(t, db, m))
	}
	return obj, media
}

// managementInput returns the hidden input of the management form field
// name of the Media inline with value.
func managementInput(name string, value int) string {
	return fmt.Sprintf(`<input id="id_%s-%s" name="%s-%s" type="hidden" value="%d">`,
		mediaPrefix, name, mediaPrefix, name, value)
}

// Ported from Django: tests/generic_inline_admin/tests.py::GenericAdminViewTest.test_basic_add_GET
// Ported from Django: tests/generic_inline_admin/tests.py::GenericAdminViewTest.test_basic_edit_GET
// Ported from Django: tests/generic_inline_admin/tests.py::GenericAdminViewTest.test_basic_add_POST
// Ported from Django: tests/generic_inline_admin/tests.py::GenericAdminViewTest.test_basic_edit_POST
func TestGenericAdminView(t *testing.T) {
	djangotest.EachDB(t, genericInlineAdminModels(), func(t *testing.T, db *orm.DB) {
		env := genericInlineEnv(t, db, nil)
		e, media := createWithMedia(t, db, &Episode{Name: "This Week in Django"},
			"http://example.com/podcast.mp3", "http://example.com/logo.png")

		t.Run("test_basic_add_GET", func(t *testing.T) {
			resp := env.client.Get(mustReverse(t, env.router, "admin:generic_inline_admin_episode_add"))
			djangotest.AssertStatus(t, resp, http.StatusOK)
		})

		t.Run("test_basic_edit_GET", func(t *testing.T) {
			resp := env.client.Get(mustReverse(t, env.router, "admin:generic_inline_admin_episode_change", e.ID))
			djangotest.AssertStatus(t, resp, http.StatusOK)
		})

		t.Run("test_basic_add_POST", func(t *testing.T) {
			resp := env.client.Post(mustReverse(t, env.router, "admin:generic_inline_admin_episode_add"), url.Values{
				"name":                         {"This Week in Django"},
				mediaPrefix + "-TOTAL_FORMS":   {"1"},
				mediaPrefix + "-INITIAL_FORMS": {"0"},
				mediaPrefix + "-MAX_NUM_FORMS": {"0"},
			})
			djangotest.AssertStatus(t, resp, http.StatusFound)
		})

		t.Run("test_basic_edit_POST", func(t *testing.T) {
			resp := env.client.Post(mustReverse(t, env.router, "admin:generic_inline_admin_episode_change", e.ID), url.Values{
				"name":                         {"This Week in Django"},
				mediaPrefix + "-TOTAL_FORMS":   {"3"},
				mediaPrefix + "-INITIAL_FORMS": {"2"},
				mediaPrefix + "-MAX_NUM_FORMS": {"0"},
				mediaPrefix + "-0-id":          {fmt.Sprint(media[0].ID)},
				mediaPrefix + "-0-url":         {"http://example.com/podcast.mp3"},
				mediaPrefix + "-1-id":          {fmt.Sprint(media[1].ID)},
				mediaPrefix + "-1-url":         {"http://example.com/logo.png"},
				mediaPrefix + "-2-id":          {""},
				mediaPrefix + "-2-url":         {""},
			})
			djangotest.AssertStatus(t, resp, http.StatusFound)
		})
	})
}

// Ported from Django: tests/generic_inline_admin/tests.py::GenericInlineAdminParametersTest.test_no_param
// Ported from Django: tests/generic_inline_admin/tests.py::GenericInlineAdminParametersTest.test_extra_param
// Ported from Django: tests/generic_inline_admin/tests.py::GenericInlineAdminParametersTest.test_max_num_param
// Ported from Django: tests/generic_inline_admin/tests.py::GenericInlineAdminParametersTest.test_min_num_param
// Ported from Django: tests/generic_inline_admin/tests.py::GenericInlineAdminParametersTest.test_get_extra
// Ported from Django: tests/generic_inline_admin/tests.py::GenericInlineAdminParametersTest.test_get_min_num
// Ported from Django: tests/generic_inline_admin/tests.py::GenericInlineAdminParametersTest.test_get_max_num
func TestGenericInlineAdminParameters(t *testing.T) {
	// Django calls changeform_view on a ModelAdmin built by hand with a
	// request from RequestFactory; the Go tests register the inline and get
	// the change page. The formset of the template context has no extra, so
	// the extra forms show in TOTAL_FORMS next to the one initial form.
	tests := []struct {
		name                string
		cfg                 admin.InlineModelAdmin
		total, minNum, maxN int
	}{
		{"test_no_param", admin.InlineModelAdmin{}, 4, 0, 1000},
		{"test_extra_param", admin.InlineModelAdmin{Extra: new(0)}, 1, 0, 1000},
		{"test_max_num_param", admin.InlineModelAdmin{Extra: new(5), MaxNum: new(2)}, 2, 0, 2},
		{"test_min_num_param", admin.InlineModelAdmin{Extra: new(3), MinNum: 2}, 5, 2, 1000},
		{"test_get_extra", admin.InlineModelAdmin{
			Extra:    new(4),
			GetExtra: func(*http.Request, any) int { return 2 },
		}, 3, 0, 1000},
		{"test_get_min_num", admin.InlineModelAdmin{
			MinNum:    5,
			GetMinNum: func(*http.Request, any) int { return 2 },
		}, 5, 2, 1000},
		{"test_get_max_num", admin.InlineModelAdmin{
			Extra:     new(5),
			GetMaxNum: func(*http.Request, any) int { return 2 },
		}, 2, 0, 2},
	}
	djangotest.EachDB(t, genericInlineAdminModels(), func(t *testing.T, db *orm.DB) {
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				env := genericInlineEnv(t, db, []admin.Inline{admin.GenericTabularInline[Media](tt.cfg)})
				e, _ := createWithMedia(t, db, &Episode{Name: "This Week in Django"}, "http://example.com/podcast.mp3")
				resp := env.client.Get(mustReverse(t, env.router, "admin:generic_inline_admin_episode_change", e.ID))
				djangotest.AssertStatus(t, resp, http.StatusOK)
				djangotest.AssertInHTML(t, managementInput("TOTAL_FORMS", tt.total), resp.Content())
				djangotest.AssertInHTML(t, managementInput("INITIAL_FORMS", 1), resp.Content())
				fs := inlineFormSet(t, resp, 0)
				djangotest.AssertEqual(t, fs["min_num"], any(tt.minNum))
				djangotest.AssertEqual(t, fs["max_num"], any(tt.maxN))
			})
		}
	})
}

// Ported from Django: tests/generic_inline_admin/tests.py::GenericInlineAdminWithUniqueTogetherTest.test_add
// Ported from Django: tests/generic_inline_admin/tests.py::GenericInlineAdminWithUniqueTogetherTest.test_delete
func TestGenericInlineAdminWithUniqueTogether(t *testing.T) {
	djangotest.EachDB(t, genericInlineAdminModels(), func(t *testing.T, db *orm.DB) {
		env := genericInlineEnv(t, db, nil)
		ctx := context.Background()

		t.Run("test_add", func(t *testing.T) {
			category := create(t, db, &GenericCategory{Name: "male"})
			const prefix = "generic_inline_admin-phonenumber-content_type-object_id"
			addURL := mustReverse(t, env.router, "admin:generic_inline_admin_genericcontact_add")
			djangotest.AssertStatus(t, env.client.Get(addURL), http.StatusOK)
			resp := env.client.Post(addURL, url.Values{
				"name":                     {"John Doe"},
				prefix + "-TOTAL_FORMS":    {"1"},
				prefix + "-INITIAL_FORMS":  {"0"},
				prefix + "-MAX_NUM_FORMS":  {"0"},
				prefix + "-0-id":           {""},
				prefix + "-0-phone_number": {"555-555-5555"},
				prefix + "-0-category":     {fmt.Sprint(category.ID)},
			})
			djangotest.AssertStatus(t, resp, http.StatusFound)
			// No Django assertion: the row points at the new contact.
			numbers, err := orm.Query[PhoneNumber](db).Filter("phone_numbers__name", "John Doe").All(ctx)
			if err != nil {
				t.Fatal(err)
			}
			djangotest.AssertEqual(t, len(numbers), 1)
		})

		t.Run("test_delete", func(t *testing.T) {
			c := create(t, db, &GenericContact{Name: "foo"})
			ct, err := contenttypes.GetForModel(ctx, db, mustModel[GenericContact](t, db))
			if err != nil {
				t.Fatal(err)
			}
			create(t, db, &PhoneNumber{ObjectID: c.ID, ContentType: fk[contenttypes.ContentType](ct.ID), PhoneNumber: "555-555-5555"})
			resp := env.client.Post(mustReverse(t, env.router, "admin:generic_inline_admin_genericcontact_delete", c.ID), nil)
			djangotest.AssertContains(t, resp, "Are you sure you want to delete")
		})
	})
}

// mustModel returns the registered model of T.
func mustModel[T any](t *testing.T, db *orm.DB) *orm.Model {
	t.Helper()
	m, ok := db.Registry().ModelOf(new(T))
	if !ok {
		t.Fatalf("no model registered for %T", *new(T))
	}
	return m
}

// Ported from Django: tests/generic_inline_admin/tests.py::NoInlineDeletionTest.test_no_deletion
func TestGenericInlineNoDeletion(t *testing.T) {
	djangotest.EachDB(t, genericInlineAdminModels(), func(t *testing.T, db *orm.DB) {
		env := genericInlineEnv(t, db, nil)
		resp := env.client.Get(mustReverse(t, env.router, "admin:generic_inline_admin_episodepermanent_add"))
		djangotest.AssertEqual(t, inlineFormSet(t, resp, 0)["can_delete"], any(false))
	})
}

// mediaFormFields returns the field names of the first form of the first
// inline on the Episode add page with the inlines given.
func mediaFormFields(t *testing.T, db *orm.DB, inlines ...admin.Inline) []string {
	t.Helper()
	env := genericInlineEnv(t, db, inlines)
	resp := env.client.Get(mustReverse(t, env.router, "admin:generic_inline_admin_episode_add"))
	djangotest.AssertStatus(t, resp, http.StatusOK)
	form := inlineFormSet(t, resp, 0)["forms"].([]any)[0].(forms.TemplateForm)
	fields := form["fields"].([]forms.TemplateField)
	names := make([]string, 0, len(fields))
	for _, f := range fields {
		names = append(names, f["name"].(string))
	}
	return names
}

// Ported from Django: tests/generic_inline_admin/tests.py::GenericInlineModelAdminTest.test_get_formset_kwargs
// Ported from Django: tests/generic_inline_admin/tests.py::GenericInlineModelAdminTest.test_custom_form_meta_exclude_with_readonly
// Ported from Django: tests/generic_inline_admin/tests.py::GenericInlineModelAdminTest.test_custom_form_meta_exclude
// Ported from Django: tests/generic_inline_admin/tests.py::GenericInlineModelAdminTest.test_get_fieldsets
// Ported from Django: tests/generic_inline_admin/tests.py::GenericInlineModelAdminTest.test_get_inline_instances_override_get_inlines
func TestGenericInlineModelAdmin(t *testing.T) {
	// No Go counterpart: test_get_formsets_with_inlines_returns_tuples
	// compares the formset classes of get_formsets_with_inlines; an Inline
	// builds its formset itself.
	djangotest.EachDB(t, genericInlineAdminModels(), func(t *testing.T, db *orm.DB) {
		media := admin.GenericTabularInline[Media]

		t.Run("test_get_formset_kwargs", func(t *testing.T) {
			// Django passes the keyword arguments to get_formset; the
			// FormSet hook sets them on the formset.
			env := genericInlineEnv(t, db, nil)
			resp := env.client.Get(mustReverse(t, env.router, "admin:generic_inline_admin_episode_add"))
			fs := inlineFormSet(t, resp, 0)
			djangotest.AssertEqual(t, fs["max_num"], any(1000))
			djangotest.AssertEqual(t, fs["can_order"], any(false))

			env = genericInlineEnv(t, db, []admin.Inline{media(admin.InlineModelAdmin{
				FormSet: func(_ *http.Request, _ any, fs *forms.FormSet) {
					fs.MaxNum, fs.CanOrder = 100, true
				},
			})})
			resp = env.client.Get(mustReverse(t, env.router, "admin:generic_inline_admin_episode_add"))
			fs = inlineFormSet(t, resp, 0)
			djangotest.AssertEqual(t, fs["max_num"], any(100))
			djangotest.AssertEqual(t, fs["can_order"], any(true))
		})

		t.Run("test_custom_form_meta_exclude_with_readonly", func(t *testing.T) {
			// The custom form's Meta.exclude is the forms.WithExclude of
			// FormOptions.
			fields := mediaFormFields(t, db, media(admin.InlineModelAdmin{
				ReadonlyFields: []string{"description"},
				FormOptions:    []forms.ModelFormOption{forms.WithExclude("url")},
			}))
			djangotest.AssertEqual(t, fields, []string{"keywords", "id", "DELETE"})
		})

		t.Run("test_custom_form_meta_exclude", func(t *testing.T) {
			fields := mediaFormFields(t, db, media(admin.InlineModelAdmin{
				ExcludeFields: []string{"description"},
				FormOptions:   []forms.ModelFormOption{forms.WithExclude("url")},
			}))
			djangotest.AssertEqual(t, fields, []string{"url", "keywords", "id", "DELETE"})

			fields = mediaFormFields(t, db, media(admin.InlineModelAdmin{
				FormOptions: []forms.ModelFormOption{forms.WithExclude("url")},
			}))
			djangotest.AssertEqual(t, fields, []string{"description", "keywords", "id", "DELETE"})
		})

		t.Run("test_get_fieldsets", func(t *testing.T) {
			fields := mediaFormFields(t, db, media(admin.InlineModelAdmin{
				CanDelete: new(false),
				GetFieldsets: func(*http.Request, any) []admin.Fieldset {
					return []admin.Fieldset{{Fields: []any{"url", "description"}}}
				},
			}))
			djangotest.AssertEqual(t, fields, []string{"url", "description", "id"})
		})

		t.Run("test_get_inline_instances_override_get_inlines", func(t *testing.T) {
			alternate := media(admin.InlineModelAdmin{VerboseNamePlural: "alternate"})
			mediaInline := media(admin.InlineModelAdmin{VerboseNamePlural: "media"})
			env := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
				admin.Register[Episode](site, admin.ModelAdmin{
					Inlines: []admin.Inline{alternate, mediaInline},
					GetInlines: func(r *http.Request, _ any) []admin.Inline {
						switch r.URL.Query().Get("name") {
						case "alternate":
							return []admin.Inline{alternate}
						case "media":
							return []admin.Inline{mediaInline}
						}
						return nil
					},
				})
			})
			env.client.ForceLogin(genericSuperuser(t, db))
			addURL := mustReverse(t, env.router, "admin:generic_inline_admin_episode_add")
			djangotest.AssertEqual(t, len(inlineSets(t, env.client.Get(addURL))), 0)
			for _, name := range []string{"alternate", "media"} {
				sets := inlineSets(t, env.client.Get(addURL+"?name="+name))
				djangotest.AssertEqual(t, len(sets), 1)
				djangotest.AssertEqual(t, sets[0]["verbose_name_plural"], any(name))
			}
		})
	})
}
