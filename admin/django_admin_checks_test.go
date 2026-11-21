package admin_test

import (
	"testing"

	"github.com/oliverhaas/djangogo/admin"
	"github.com/oliverhaas/djangogo/auth"
	"github.com/oliverhaas/djangogo/contenttypes"
	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/forms"
	"github.com/oliverhaas/djangogo/orm"
)

// The models of tests/admin_checks/models.py. Their Go names carry a Check
// prefix because the package already has a Song and a Book, so the labels
// in the messages are "admin_checks.CheckSong" where Django says
// "admin_checks.Song".

// CheckAlbum stands in for tests/admin_checks/models.py::Album.
type CheckAlbum struct {
	ID    int64
	Title string `orm:"max_length=150"`
}

// CheckSong stands in for tests/admin_checks/models.py::Song.
type CheckSong struct {
	ID              int64
	Title           string             `orm:"max_length=150"`
	Album           orm.FK[CheckAlbum] `orm:"on_delete=cascade"`
	OriginalRelease orm.Date           `orm:"editable=false"`
}

// CheckAuthor stands in for tests/admin_checks/models.py::Author.
type CheckAuthor struct {
	ID   int64
	Name string `orm:"max_length=100"`
}

// CheckBook stands in for tests/admin_checks/models.py::Book.
type CheckBook struct {
	ID       int64
	Name     string `orm:"max_length=100"`
	Subtitle string `orm:"max_length=100"`
	Price    float64
	Authors  orm.M2M[CheckAuthor] `orm:"through=CheckAuthorsBooks"`
}

// CheckAuthorsBooks stands in for tests/admin_checks/models.py::AuthorsBooks.
type CheckAuthorsBooks struct {
	ID       int64
	Author   orm.FK[CheckAuthor] `orm:"on_delete=cascade"`
	Book     orm.FK[CheckBook]   `orm:"on_delete=cascade"`
	Featured bool
}

// adminChecksModels returns the model set of the ported
// tests/admin_checks/tests.py tests.
func adminChecksModels() []any {
	return []any{
		auth.App{},
		contenttypes.App{},
		djangotest.App("admin_checks",
			&CheckAlbum{}, &CheckSong{}, &CheckAuthor{}, &CheckBook{}, &CheckAuthorsBooks{},
		),
	}
}

// Ported from Django: tests/admin_checks/tests.py::SystemChecksTestCase.test_custom_modelforms_with_fields_fieldsets
// Ported from Django: tests/admin_checks/tests.py::SystemChecksTestCase.test_custom_get_form_with_fieldsets
// Ported from Django: tests/admin_checks/tests.py::SystemChecksTestCase.test_exclude_duplicate_values
// Ported from Django: tests/admin_checks/tests.py::SystemChecksTestCase.test_readonly
// Ported from Django: tests/admin_checks/tests.py::SystemChecksTestCase.test_readonly_on_modeladmin
// Ported from Django: tests/admin_checks/tests.py::SystemChecksTestCase.test_nonexistent_field
// Ported from Django: tests/admin_checks/tests.py::SystemChecksTestCase.test_graceful_m2m_fail
// Ported from Django: tests/admin_checks/tests.py::SystemChecksTestCase.test_cannot_include_through
// Ported from Django: tests/admin_checks/tests.py::SystemChecksTestCase.test_nested_fields
// Ported from Django: tests/admin_checks/tests.py::SystemChecksTestCase.test_nested_fieldsets
// Ported from Django: tests/admin_checks/tests.py::SystemChecksTestCase.test_non_model_fields
// Ported from Django: tests/admin_checks/tests.py::SystemChecksTestCase.test_non_model_first_field
// Ported from Django: tests/admin_checks/tests.py::SystemChecksTestCase.test_check_sublists_for_duplicates
// Ported from Django: tests/admin_checks/tests.py::SystemChecksTestCase.test_check_fieldset_sublists_for_duplicates
// Ported from Django: tests/admin_checks/tests.py::SystemChecksTestCase.test_check_multiple_duplicates_across_fieldsets
func TestAdminChecksFieldLayout(t *testing.T) {
	djangotest.EachDB(t, adminChecksModels(), func(t *testing.T, db *orm.DB) {
		// FIXME-PORT: test_readonly_on_method, test_readonly_lambda and
		// test_readonly_dynamic_attribute_on_modeladmin name a callable;
		// the Go counterpart of each is a DisplayColumns key, as in
		// test_readonly_on_modeladmin. test_readonly_method_on_model has no
		// Go counterpart: a read-only field shows a model field or a
		// DisplayColumn, not a model method. test_fieldsets_fields_non_tuple,
		// test_nonfirst_fieldset, test_exclude_values and
		// test_readonly_fields_not_list_or_tuple check option types that Go
		// fixes at compile time.
		extraField := []forms.ModelFormOption{forms.WithFormField("extra_data", forms.NewCharField())}
		const song = "admin_checks.CheckSong"
		const book = "admin_checks.CheckBook"

		valid := []struct {
			test     string
			register func(site *admin.AdminSite)
		}{
			{"test_custom_modelforms_with_fields_fieldsets", func(site *admin.AdminSite) {
				admin.Register[CheckSong](site, admin.ModelAdmin{Fields: []any{"title"}})
			}},
			{"test_custom_get_form_with_fieldsets", func(site *admin.AdminSite) {
				admin.Register[CheckSong](site, admin.ModelAdmin{
					Fieldsets:   []admin.Fieldset{{Fields: []any{"name"}}},
					FormOptions: []forms.ModelFormOption{forms.WithFormField("name", forms.NewCharField())},
				})
			}},
			{"test_readonly", func(site *admin.AdminSite) {
				admin.Register[CheckSong](site, admin.ModelAdmin{ReadonlyFields: []string{"title"}})
			}},
			{"test_readonly_on_modeladmin", func(site *admin.AdminSite) {
				admin.Register[CheckSong](site, admin.ModelAdmin{
					ReadonlyFields: []string{"readonly_method_on_modeladmin"},
					DisplayColumns: map[string]admin.DisplayColumn{
						"readonly_method_on_modeladmin": {Value: func(any) any { return nil }},
					},
				})
			}},
			{"test_nested_fields", func(site *admin.AdminSite) {
				admin.Register[CheckBook](site, admin.ModelAdmin{Fields: []any{"price", []string{"name", "subtitle"}}})
			}},
			{"test_nested_fieldsets", func(site *admin.AdminSite) {
				admin.Register[CheckBook](site, admin.ModelAdmin{Fieldsets: []admin.Fieldset{
					{Name: "Main", Fields: []any{"price", []string{"name", "subtitle"}}},
				}})
			}},
			{"test_non_model_fields", func(site *admin.AdminSite) {
				admin.Register[CheckSong](site, admin.ModelAdmin{Fields: []any{"title", "extra_data"}, FormOptions: extraField})
			}},
			{"test_non_model_first_field", func(site *admin.AdminSite) {
				admin.Register[CheckSong](site, admin.ModelAdmin{Fields: []any{"extra_data", "title"}, FormOptions: extraField})
			}},
		}
		for _, tc := range valid {
			t.Run(tc.test, func(t *testing.T) {
				assertIsValid(t, checkSite(t, db, tc.register))
			})
		}

		t.Run("test_exclude_duplicate_values", func(t *testing.T) {
			site := checkSite(t, db, func(site *admin.AdminSite) {
				admin.Register[CheckBook](site, admin.ModelAdmin{ExcludeFields: []string{"name", "name"}})
			})
			assertIsInvalid(t, site, book, "The value of 'exclude' contains duplicate field(s).",
				"admin.E015", "Remove duplicates of 'name'.")
		})

		t.Run("test_nonexistent_field", func(t *testing.T) {
			site := checkSite(t, db, func(site *admin.AdminSite) {
				admin.Register[CheckSong](site, admin.ModelAdmin{ReadonlyFields: []string{"title", "nonexistent"}})
			})
			assertIsInvalid(t, site, song,
				"The value of 'readonly_fields[1]' refers to 'nonexistent', which is not a callable, "+
					"an attribute of 'ModelAdmin', or an attribute of '"+song+"'.",
				"admin.E035")
		})

		t.Run("test_graceful_m2m_fail", func(t *testing.T) {
			site := checkSite(t, db, func(site *admin.AdminSite) {
				admin.Register[CheckBook](site, admin.ModelAdmin{Fields: []any{"authors"}})
			})
			assertIsInvalid(t, site, book,
				"The value of 'fields' cannot include the ManyToManyField 'authors', "+
					"because that field manually specifies a relationship model.",
				"admin.E013")
		})

		t.Run("test_cannot_include_through", func(t *testing.T) {
			site := checkSite(t, db, func(site *admin.AdminSite) {
				admin.Register[CheckBook](site, admin.ModelAdmin{Fieldsets: []admin.Fieldset{
					{Name: "Header 1", Fields: []any{"name"}},
					{Name: "Header 2", Fields: []any{"authors"}},
				}})
			})
			assertIsInvalid(t, site, book,
				`The value of 'fieldsets[1][1]["fields"]' cannot include the ManyToManyField 'authors', `+
					"because that field manually specifies a relationship model.",
				"admin.E013")
		})

		t.Run("test_check_sublists_for_duplicates", func(t *testing.T) {
			site := checkSite(t, db, func(site *admin.AdminSite) {
				admin.Register[CheckSong](site, admin.ModelAdmin{Fields: []any{"state", []string{"state"}}})
			})
			assertIsInvalid(t, site, song, "The value of 'fields' contains duplicate field(s).",
				"admin.E006", "Remove duplicates of 'state'.")
		})

		t.Run("test_check_fieldset_sublists_for_duplicates", func(t *testing.T) {
			site := checkSite(t, db, func(site *admin.AdminSite) {
				admin.Register[CheckSong](site, admin.ModelAdmin{Fieldsets: []admin.Fieldset{
					{Fields: []any{"title", "album", []string{"title", "album"}}},
				}})
			})
			assertIsInvalid(t, site, song, "There are duplicate field(s) in 'fieldsets[0][1]'.",
				"admin.E012", "Remove duplicates of 'title', 'album'.")
		})

		t.Run("test_check_multiple_duplicates_across_fieldsets", func(t *testing.T) {
			site := checkSite(t, db, func(site *admin.AdminSite) {
				admin.Register[CheckSong](site, admin.ModelAdmin{Fieldsets: []admin.Fieldset{
					{Name: "Header 1", Fields: []any{"title", "album"}},
					{Name: "Header 2", Fields: []any{"album", "name"}},
					{Name: "Header 3", Fields: []any{"name", "other", "title"}},
				}})
			})
			got := site.Check()
			if len(got) != 2 {
				t.Fatalf("Check() = %v, want two errors", got)
			}
			assertCheckError(t, got[0], song, "There are duplicate field(s) in 'fieldsets[1][1]'.",
				"admin.E012", "Remove duplicates of 'album'.")
			assertCheckError(t, got[1], song, "There are duplicate field(s) in 'fieldsets[2][1]'.",
				"admin.E012", "Remove duplicates of 'title', 'name'.")
		})
	})
}

// TestFieldLayoutEntryTypesCheck has no Django counterpart: a Fields entry
// of the Go []any that is neither a name nor a []string is an error, and the
// Go name and the attribute name of one field count as a duplicate.
func TestFieldLayoutEntryTypesCheck(t *testing.T) {
	djangotest.EachDB(t, adminChecksModels(), func(t *testing.T, db *orm.DB) {
		const song = "admin_checks.CheckSong"
		site := checkSite(t, db, func(site *admin.AdminSite) {
			admin.Register[CheckSong](site, admin.ModelAdmin{Fields: []any{"title", []any{"album"}}})
		})
		assertIsInvalid(t, site, song, "The value of 'fields[1]' must be a field name or a []string.", "admin.E004")

		site = checkSite(t, db, func(site *admin.AdminSite) {
			admin.Register[CheckSong](site, admin.ModelAdmin{Fieldsets: []admin.Fieldset{{Fields: []any{1}}}})
		})
		assertIsInvalid(t, site, song, "The value of 'fieldsets[0][1]['fields'][0]' must be a field name or a []string.",
			"admin.E008")

		site = checkSite(t, db, func(site *admin.AdminSite) {
			admin.Register[CheckSong](site, admin.ModelAdmin{Fields: []any{"Title", "title"}})
		})
		assertIsInvalid(t, site, song, "The value of 'fields' contains duplicate field(s).",
			"admin.E006", "Remove duplicates of 'title'.")
	})
}
