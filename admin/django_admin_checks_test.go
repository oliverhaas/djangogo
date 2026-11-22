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

// ReadonlyMethodOnModel is Song.readonly_method_on_model, which does nothing.
func (CheckSong) ReadonlyMethodOnModel() any { return nil }

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

// CheckTwoAlbumFKAndAnE stands in for
// tests/admin_checks/models.py::TwoAlbumFKAndAnE.
type CheckTwoAlbumFKAndAnE struct {
	ID     int64
	Album1 orm.FK[CheckAlbum] `orm:"on_delete=cascade;related_name=album1_set"`
	Album2 orm.FK[CheckAlbum] `orm:"on_delete=cascade;related_name=album2_set"`
	E      string             `orm:"max_length=1"`
}

// CheckState stands in for tests/admin_checks/models.py::State.
type CheckState struct {
	ID   int64
	Name string `orm:"max_length=15"`
}

// CheckCity stands in for tests/admin_checks/models.py::City.
type CheckCity struct {
	ID    int64
	State orm.FK[CheckState] `orm:"on_delete=cascade"`
}

// CheckInfluence stands in for tests/admin_checks/models.py::Influence.
type CheckInfluence struct {
	ID            int64
	Name          string `orm:"type=text"`
	ContentType   orm.FK[contenttypes.ContentType]
	ObjectID      int64 `orm:"type=positive_int"`
	ContentObject contenttypes.GenericForeignKey
}

// adminChecksModels returns the model set of the ported
// tests/admin_checks/tests.py tests.
func adminChecksModels() []any {
	return []any{
		auth.App{},
		contenttypes.App{},
		admin.App{},
		djangotest.App("admin_checks",
			&CheckAlbum{}, &CheckSong{}, &CheckAuthor{}, &CheckBook{}, &CheckAuthorsBooks{},
			&CheckTwoAlbumFKAndAnE{}, &CheckState{}, &CheckCity{}, &CheckInfluence{},
		),
	}
}

// Ported from Django: tests/admin_checks/tests.py::SystemChecksTestCase.test_custom_modelforms_with_fields_fieldsets
// Ported from Django: tests/admin_checks/tests.py::SystemChecksTestCase.test_custom_get_form_with_fieldsets
// Ported from Django: tests/admin_checks/tests.py::SystemChecksTestCase.test_exclude_duplicate_values
// Ported from Django: tests/admin_checks/tests.py::SystemChecksTestCase.test_readonly
// Ported from Django: tests/admin_checks/tests.py::SystemChecksTestCase.test_readonly_on_modeladmin
// Ported from Django: tests/admin_checks/tests.py::SystemChecksTestCase.test_readonly_method_on_model
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
		// test_readonly_on_modeladmin. test_fieldsets_fields_non_tuple,
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
			{"test_readonly_method_on_model", func(site *admin.AdminSite) {
				admin.Register[CheckSong](site, admin.ModelAdmin{ReadonlyFields: []string{"readonly_method_on_model"}})
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

// Ported from Django: tests/admin_checks/tests.py::SystemChecksTestCase.test_exclude_inline_model_admin
// Ported from Django: tests/admin_checks/tests.py::SystemChecksTestCase.test_fk_exclusion
// Ported from Django: tests/admin_checks/tests.py::SystemChecksTestCase.test_inline_self_check
// Ported from Django: tests/admin_checks/tests.py::SystemChecksTestCase.test_inline_with_specified
// Ported from Django: tests/admin_checks/tests.py::SystemChecksTestCase.test_inlines_property
// Ported from Django: tests/admin_checks/tests.py::SystemChecksTestCase.test_nonexistent_field_on_inline
func TestAdminChecksInlines(t *testing.T) {
	// No Go counterpart: test_exclude_in_inline checks the type of exclude,
	// which Go fixes at compile time. test_explicit_through_override needs an
	// inline of a many-to-many through model, which the Go admin does not
	// have.
	twoFK := admin.TabularInline[CheckAlbum, CheckTwoAlbumFKAndAnE]
	albumSite := func(t *testing.T, db *orm.DB, inline admin.Inline) *admin.AdminSite {
		t.Helper()
		return checkSite(t, db, func(site *admin.AdminSite) {
			admin.Register[CheckAlbum](site, admin.ModelAdmin{Inlines: []admin.Inline{inline}})
		})
	}
	djangotest.EachDB(t, adminChecksModels(), func(t *testing.T, db *orm.DB) {
		t.Run("test_exclude_inline_model_admin", func(t *testing.T) {
			site := albumSite(t, db, admin.StackedInline[CheckAlbum, CheckSong](admin.InlineModelAdmin{
				ExcludeFields: []string{"album"},
			}))
			assertInlineIsInvalid(t, site, "admin_checks.CheckSong",
				"Cannot exclude the field 'album', because it is the foreign key to the parent model "+
					"'admin_checks.CheckAlbum'.", "admin.E201")
		})

		t.Run("test_fk_exclusion", func(t *testing.T) {
			assertIsValid(t, albumSite(t, db, twoFK(admin.InlineModelAdmin{ExcludeFields: []string{"e"}, FKName: "album1"})))
		})

		t.Run("test_inline_self_check", func(t *testing.T) {
			assertInlineIsInvalid(t, albumSite(t, db, twoFK(admin.InlineModelAdmin{})), "admin_checks.CheckTwoAlbumFKAndAnE",
				"'admin_checks.CheckTwoAlbumFKAndAnE' has more than one ForeignKey to 'admin_checks.CheckAlbum'. "+
					"You must specify a 'fk_name' attribute.", "admin.E202")
		})

		t.Run("test_inline_with_specified", func(t *testing.T) {
			assertIsValid(t, albumSite(t, db, twoFK(admin.InlineModelAdmin{FKName: "album1"})))
		})

		t.Run("test_inlines_property", func(t *testing.T) {
			// Django returns the inlines from a property; the checks read
			// Inlines, since GetInlines needs a request.
			site := checkSite(t, db, func(site *admin.AdminSite) {
				admin.Register[CheckState](site, admin.ModelAdmin{Inlines: []admin.Inline{
					admin.TabularInline[CheckState, CheckCity](admin.InlineModelAdmin{}),
				}})
			})
			assertIsValid(t, site)
		})

		t.Run("test_nonexistent_field_on_inline", func(t *testing.T) {
			site := checkSite(t, db, func(site *admin.AdminSite) {
				admin.Register[CheckState](site, admin.ModelAdmin{Inlines: []admin.Inline{
					admin.TabularInline[CheckState, CheckCity](admin.InlineModelAdmin{ReadonlyFields: []string{"i_dont_exist"}}),
				}})
			})
			assertInlineIsInvalid(t, site, "admin_checks.CheckCity",
				"The value of 'readonly_fields[0]' refers to 'i_dont_exist', which is not a callable, "+
					"an attribute of 'InlineModelAdmin', or an attribute of 'admin_checks.CheckCity'.", "admin.E035")
		})
	})
}

// Ported from Django: tests/admin_checks/tests.py::SystemChecksTestCase.test_valid_generic_inline_model_admin
// Ported from Django: tests/admin_checks/tests.py::SystemChecksTestCase.test_generic_inline_model_admin_non_generic_model
// Ported from Django: tests/admin_checks/tests.py::SystemChecksTestCase.test_generic_inline_model_admin_bad_ct_field
// Ported from Django: tests/admin_checks/tests.py::SystemChecksTestCase.test_generic_inline_model_admin_bad_fk_field
// Ported from Django: tests/admin_checks/tests.py::SystemChecksTestCase.test_generic_inline_model_admin_non_gfk_ct_field
// Ported from Django: tests/admin_checks/tests.py::SystemChecksTestCase.test_generic_inline_model_admin_non_gfk_fk_field
func TestAdminChecksGenericInlines(t *testing.T) {
	const influence = "admin_checks.CheckInfluence"
	songSite := func(t *testing.T, db *orm.DB, inline admin.Inline) *admin.AdminSite {
		t.Helper()
		return checkSite(t, db, func(site *admin.AdminSite) {
			admin.Register[CheckSong](site, admin.ModelAdmin{Inlines: []admin.Inline{inline}})
		})
	}
	djangotest.EachDB(t, adminChecksModels(), func(t *testing.T, db *orm.DB) {
		t.Run("test_valid_generic_inline_model_admin", func(t *testing.T) {
			// Generic inlines don't look for normal ForeignKey relations
			// (#22034).
			assertIsValid(t, songSite(t, db, admin.GenericStackedInline[CheckInfluence](admin.InlineModelAdmin{})))
		})

		t.Run("test_generic_inline_model_admin_non_generic_model", func(t *testing.T) {
			assertInlineIsInvalid(t, songSite(t, db, admin.GenericStackedInline[CheckBook](admin.InlineModelAdmin{})),
				"admin_checks.CheckBook", "'admin_checks.CheckBook' has no GenericForeignKey.", "admin.E301")
		})

		t.Run("test_generic_inline_model_admin_bad_ct_field", func(t *testing.T) {
			site := songSite(t, db, admin.GenericStackedInline[CheckInfluence](admin.InlineModelAdmin{CTField: "nonexistent"}))
			assertInlineIsInvalid(t, site, influence,
				"'ct_field' references 'nonexistent', which is not a field on 'admin_checks.CheckInfluence'.", "admin.E302")
		})

		t.Run("test_generic_inline_model_admin_bad_fk_field", func(t *testing.T) {
			site := songSite(t, db, admin.GenericStackedInline[CheckInfluence](admin.InlineModelAdmin{CTFKField: "nonexistent"}))
			assertInlineIsInvalid(t, site, influence,
				"'ct_fk_field' references 'nonexistent', which is not a field on 'admin_checks.CheckInfluence'.", "admin.E303")
		})

		t.Run("test_generic_inline_model_admin_non_gfk_ct_field", func(t *testing.T) {
			site := songSite(t, db, admin.GenericStackedInline[CheckInfluence](admin.InlineModelAdmin{CTField: "name"}))
			assertInlineIsInvalid(t, site, influence,
				"'admin_checks.CheckInfluence' has no GenericForeignKey using content type field 'name' and "+
					"object ID field 'object_id'.", "admin.E304")
		})

		t.Run("test_generic_inline_model_admin_non_gfk_fk_field", func(t *testing.T) {
			site := songSite(t, db, admin.GenericStackedInline[CheckInfluence](admin.InlineModelAdmin{CTFKField: "name"}))
			assertInlineIsInvalid(t, site, influence,
				"'admin_checks.CheckInfluence' has no GenericForeignKey using content type field 'content_type' and "+
					"object ID field 'name'.", "admin.E304")
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

// Ported from Django: tests/admin_checks/tests.py::SystemChecksTestCase.test_field_name_not_in_list_display
// Ported from Django: tests/admin_checks/tests.py::SystemChecksTestCase.test_list_editable_missing_field
// Ported from Django: tests/admin_checks/tests.py::SystemChecksTestCase.test_readonly_and_editable
// Ported from Django: tests/admin_checks/tests.py::SystemChecksTestCase.test_pk_not_editable
// Ported from Django: tests/admin_checks/tests.py::SystemChecksTestCase.test_editable
func TestAdminChecksListEditable(t *testing.T) {
	djangotest.EachDB(t, adminChecksModels(), func(t *testing.T, db *orm.DB) {
		// No Go counterpart: test_list_editable_not_a_list_or_tuple
		// (admin.E120) checks an option type that Go fixes at compile time:
		// ListEditable is a []string.
		const song = "admin_checks.CheckSong"
		register := func(a admin.ModelAdmin) *admin.AdminSite {
			return checkSite(t, db, func(site *admin.AdminSite) { admin.Register[CheckSong](site, a) })
		}
		fieldsets := []admin.Fieldset{{Fields: []any{"title", "original_release"}}}

		t.Run("test_field_name_not_in_list_display", func(t *testing.T) {
			site := register(admin.ModelAdmin{ListEditable: []string{"original_release"}})
			assertIsInvalid(t, site, song,
				"The value of 'list_editable[0]' refers to 'original_release', which is not contained in 'list_display'.",
				"admin.E122")
		})

		t.Run("test_list_editable_missing_field", func(t *testing.T) {
			site := register(admin.ModelAdmin{ListEditable: []string{"test"}})
			assertIsInvalid(t, site, song,
				"The value of 'list_editable[0]' refers to 'test', which is not a field of '"+song+"'.",
				"admin.E121")
		})

		t.Run("test_readonly_and_editable", func(t *testing.T) {
			site := register(admin.ModelAdmin{
				ReadonlyFields: []string{"original_release"},
				ListDisplay:    []string{"pk", "original_release"},
				ListEditable:   []string{"original_release"},
				Fieldsets:      fieldsets,
			})
			assertIsInvalid(t, site, song,
				"The value of 'list_editable[0]' refers to 'original_release', which is not editable through the admin.",
				"admin.E125")
		})

		t.Run("test_pk_not_editable", func(t *testing.T) {
			site := register(admin.ModelAdmin{ListDisplay: []string{"title", "id"}, ListEditable: []string{"id"}})
			assertIsInvalid(t, site, song,
				"The value of 'list_editable[0]' refers to 'id', which is not editable through the admin.",
				"admin.E125")
		})

		t.Run("test_editable", func(t *testing.T) {
			assertIsValid(t, register(admin.ModelAdmin{
				ListDisplay:  []string{"pk", "title"},
				ListEditable: []string{"title"},
				Fieldsets:    fieldsets,
			}))
		})
	})
}
