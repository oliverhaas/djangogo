package admin_test

import (
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/admin"
	"github.com/oliverhaas/djangogo/auth"
	"github.com/oliverhaas/djangogo/checks"
	"github.com/oliverhaas/djangogo/contenttypes"
	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
)

// Band stands in for tests/modeladmin/models.py::Band.
type Band struct {
	ID       int64
	Name     string `orm:"max_length=100"`
	Bio      string `orm:"type=text"`
	SignDate orm.Date
}

// String returns the band name.
func (b Band) String() string { return b.Name }

// Meta orders bands by name, as Django's Band does.
func (Band) Meta() orm.Meta { return orm.Meta{Ordering: []string{"name"}} }

// Song stands in for tests/modeladmin/models.py::Song.
type Song struct {
	ID        int64
	Name      string        `orm:"max_length=100"`
	Band      orm.FK[Band]  `orm:"on_delete=cascade"`
	Featuring orm.M2M[Band] `orm:"related_name=featured"`
}

// Concert stands in for tests/modeladmin/models.py::Concert.
type Concert struct {
	ID          int64
	MainBand    orm.FK[Band] `orm:"on_delete=cascade;related_name=main_concerts"`
	OpeningBand orm.FK[Band] `orm:"on_delete=cascade;related_name=opening_concerts;blank"`
	Day         string       `orm:"max_length=3"`
	Transport   string       `orm:"max_length=100;blank"`
}

// Meta gives day and transport the choices of Django's Concert.
func (Concert) Meta() orm.Meta {
	return orm.Meta{Choices: map[string][]orm.Choice{
		"Day":       {{Value: "1", Label: "Fri"}, {Value: "2", Label: "Sat"}},
		"Transport": {{Value: "1", Label: "Plane"}, {Value: "2", Label: "Train"}, {Value: "3", Label: "Bus"}},
	}}
}

// ValidationTestModel stands in for
// tests/modeladmin/models.py::ValidationTestModel, with the fields the ported
// checks address.
type ValidationTestModel struct {
	ID         int64
	Name       string             `orm:"max_length=100"`
	Slug       string             `orm:"type=slug"`
	Users      orm.M2M[auth.User] `orm:"related_name=validationtestmodels"`
	State      string             `orm:"max_length=2"`
	PubDate    time.Time
	Band       orm.FK[Band]       `orm:"on_delete=cascade"`
	BestFriend orm.O2O[auth.User] `orm:"on_delete=cascade;related_name=best_friend"`
}

// Meta gives state the choices of Django's ValidationTestModel.
func (ValidationTestModel) Meta() orm.Meta {
	return orm.Meta{Choices: map[string][]orm.Choice{
		"State": {{Value: "CO", Label: "Colorado"}, {Value: "WA", Label: "Washington"}},
	}}
}

// ValidationTestInlineModel stands in for
// tests/modeladmin/models.py::ValidationTestInlineModel.
type ValidationTestInlineModel struct {
	ID     int64
	Parent orm.FK[ValidationTestModel] `orm:"on_delete=cascade"`
}

// NotAModel is a type that no app registers as a model.
type NotAModel struct{ ID int64 }

// Contact and Customer stand in for the models that
// FilterVerticalCheckTests.test_invalid_reverse_m2m_field_with_related_name
// declares.
type Contact struct {
	ID int64
}

// Customer has a many-to-many relation to Contact named "customers" on the
// Contact side.
type Customer struct {
	ID       int64
	Contacts orm.M2M[Contact] `orm:"related_name=customers"`
}

// Artist and BandArtist stand in for the models that
// FilterVerticalCheckTests.test_invalid_m2m_field_with_through declares.
type Artist struct {
	ID    int64
	Bands orm.M2M[Band] `orm:"through=BandArtist"`
}

// BandArtist is the explicit through model of Artist.Bands.
type BandArtist struct {
	ID     int64
	Artist orm.FK[Artist] `orm:"on_delete=cascade"`
	Band   orm.FK[Band]   `orm:"on_delete=cascade"`
}

// modeladminCheckModels returns the model set of the ported
// tests/modeladmin/test_checks.py tests.
func modeladminCheckModels() []any {
	return []any{
		auth.App{},
		contenttypes.App{},
		admin.App{},
		djangotest.App("modeladmin",
			&Band{}, &Song{}, &Concert{}, &ValidationTestModel{}, &ValidationTestInlineModel{},
			&Contact{}, &Customer{}, &Artist{}, &BandArtist{},
		),
	}
}

// checkSite returns a fresh site on db with one registration.
func checkSite(t *testing.T, db *orm.DB, register func(site *admin.AdminSite)) *admin.AdminSite {
	t.Helper()
	site, err := admin.NewAdminSite(db)
	if err != nil {
		t.Fatalf("NewAdminSite: %v", err)
	}
	register(site)
	return site
}

// assertIsInvalid mirrors CheckTestCase.assertIsInvalid: the site reports
// exactly one error, with the message, ID and optional hint given, about the
// ModelAdmin of model.
func assertIsInvalid(t *testing.T, site *admin.AdminSite, model, msg, id string, hint ...string) {
	t.Helper()
	got := site.Check()
	if len(got) != 1 {
		t.Fatalf("Check() = %v, want one error %s %q", got, id, msg)
	}
	assertCheckError(t, got[0], model, msg, id, hint...)
}

// assertCheckError asserts that m is an error with the message, ID and
// optional hint given, about the ModelAdmin of model.
func assertCheckError(t *testing.T, m checks.Message, model, msg, id string, hint ...string) {
	t.Helper()
	assertCheckErrorOn(t, m, "ModelAdmin of "+model, msg, id, hint...)
}

// assertInlineIsInvalid mirrors CheckTestCase.assertIsInvalid with an
// invalid_obj that is an inline: the site reports exactly one error, about
// the inline of model.
func assertInlineIsInvalid(t *testing.T, site *admin.AdminSite, model, msg, id string, hint ...string) {
	t.Helper()
	got := site.Check()
	if len(got) != 1 {
		t.Fatalf("Check() = %v, want one error %s %q", got, id, msg)
	}
	assertCheckErrorOn(t, got[0], "InlineModelAdmin of "+model, msg, id, hint...)
}

// assertCheckErrorOn asserts that m is an error with the message, ID and
// optional hint given, about the admin object with the label obj.
func assertCheckErrorOn(t *testing.T, m checks.Message, obj, msg, id string, hint ...string) {
	t.Helper()
	wantHint := ""
	if len(hint) > 0 {
		wantHint = hint[0]
	}
	if m.Level != checks.LevelError || m.Msg != msg || m.ID != id || m.Hint != wantHint {
		t.Errorf("Check() = %+v, want error %s %q hint %q", m, id, msg, wantHint)
	}
	got, ok := m.Obj.(interface{ Label() string })
	if !ok || got.Label() != obj {
		t.Errorf("Check() object = %v, want the %s", m.Obj, obj)
	}
}

// assertIsValid mirrors CheckTestCase.assertIsValid.
func assertIsValid(t *testing.T, site *admin.AdminSite) {
	t.Helper()
	if got := site.Check(); len(got) != 0 {
		t.Errorf("Check() = %v, want no messages", got)
	}
}

// filterCheckTests runs the ported FilterVerticalCheckTests or
// FilterHorizontalCheckTests; option is "filter_vertical" or
// "filter_horizontal".
func filterCheckTests(t *testing.T, option string) {
	t.Helper()
	with := func(names ...string) admin.ModelAdmin {
		if option == "filter_vertical" {
			return admin.ModelAdmin{FilterVertical: names}
		}
		return admin.ModelAdmin{FilterHorizontal: names}
	}
	djangotest.EachDB(t, modeladminCheckModels(), func(t *testing.T, db *orm.DB) {
		// FIXME-PORT: test_not_iterable (admin.E017, admin.E018) has no Go
		// counterpart: FilterVertical and FilterHorizontal are []string.

		t.Run("test_missing_field", func(t *testing.T) {
			site := checkSite(t, db, func(site *admin.AdminSite) {
				admin.Register[ValidationTestModel](site, with("non_existent_field"))
			})
			assertIsInvalid(t, site, "modeladmin.ValidationTestModel",
				"The value of '"+option+"[0]' refers to 'non_existent_field', "+
					"which is not a field of 'modeladmin.ValidationTestModel'.",
				"admin.E019")
		})

		t.Run("test_invalid_field_type", func(t *testing.T) {
			site := checkSite(t, db, func(site *admin.AdminSite) {
				admin.Register[ValidationTestModel](site, with("name"))
			})
			assertIsInvalid(t, site, "modeladmin.ValidationTestModel",
				"The value of '"+option+"[0]' must be a many-to-many field.", "admin.E020")
		})

		t.Run("test_invalid_reverse_m2m_field_with_related_name", func(t *testing.T) {
			site := checkSite(t, db, func(site *admin.AdminSite) {
				admin.Register[Contact](site, with("customers"))
			})
			assertIsInvalid(t, site, "modeladmin.Contact",
				"The value of '"+option+"[0]' must be a many-to-many field.", "admin.E020")
		})

		t.Run("test_invalid_m2m_field_with_through", func(t *testing.T) {
			site := checkSite(t, db, func(site *admin.AdminSite) {
				admin.Register[Artist](site, with("bands"))
			})
			assertIsInvalid(t, site, "modeladmin.Artist",
				"The value of '"+option+"[0]' cannot include the ManyToManyField "+
					"'bands', because that field manually specifies a relationship model.",
				"admin.E013")
		})

		t.Run("test_valid_case", func(t *testing.T) {
			site := checkSite(t, db, func(site *admin.AdminSite) {
				admin.Register[ValidationTestModel](site, with("users"))
			})
			assertIsValid(t, site)
		})
	})
}

// Ported from Django: tests/modeladmin/test_checks.py::FilterVerticalCheckTests.test_missing_field
// Ported from Django: tests/modeladmin/test_checks.py::FilterVerticalCheckTests.test_invalid_field_type
// Ported from Django: tests/modeladmin/test_checks.py::FilterVerticalCheckTests.test_invalid_reverse_m2m_field_with_related_name
// Ported from Django: tests/modeladmin/test_checks.py::FilterVerticalCheckTests.test_invalid_m2m_field_with_through
// Ported from Django: tests/modeladmin/test_checks.py::FilterVerticalCheckTests.test_valid_case
func TestFilterVerticalCheck(t *testing.T) {
	filterCheckTests(t, "filter_vertical")
}

// Ported from Django: tests/modeladmin/test_checks.py::FilterHorizontalCheckTests.test_missing_field
// Ported from Django: tests/modeladmin/test_checks.py::FilterHorizontalCheckTests.test_invalid_field_type
// Ported from Django: tests/modeladmin/test_checks.py::FilterHorizontalCheckTests.test_invalid_reverse_m2m_field_with_related_name
// Ported from Django: tests/modeladmin/test_checks.py::FilterHorizontalCheckTests.test_invalid_m2m_field_with_through
// Ported from Django: tests/modeladmin/test_checks.py::FilterHorizontalCheckTests.test_valid_case
func TestFilterHorizontalCheck(t *testing.T) {
	filterCheckTests(t, "filter_horizontal")
}

// No Go counterpart: tests/modeladmin/test_checks.py::RadioFieldsCheckTests.test_not_dictionary
// (admin.E021): RadioFields is a map[string]RadioStyle.

// Ported from Django: tests/modeladmin/test_checks.py::RadioFieldsCheckTests.test_missing_field
// Ported from Django: tests/modeladmin/test_checks.py::RadioFieldsCheckTests.test_invalid_field_type
// Ported from Django: tests/modeladmin/test_checks.py::RadioFieldsCheckTests.test_invalid_value
// Ported from Django: tests/modeladmin/test_checks.py::RadioFieldsCheckTests.test_valid_case
func TestRadioFieldsCheck(t *testing.T) {
	djangotest.EachDB(t, modeladminCheckModels(), func(t *testing.T, db *orm.DB) {
		register := func(radio map[string]admin.RadioStyle) *admin.AdminSite {
			return checkSite(t, db, func(site *admin.AdminSite) {
				admin.Register[ValidationTestModel](site, admin.ModelAdmin{RadioFields: radio})
			})
		}
		t.Run("test_missing_field", func(t *testing.T) {
			assertIsInvalid(t, register(map[string]admin.RadioStyle{"non_existent_field": admin.Vertical}),
				"modeladmin.ValidationTestModel",
				"The value of 'radio_fields' refers to 'non_existent_field', "+
					"which is not a field of 'modeladmin.ValidationTestModel'.",
				"admin.E022")
		})
		t.Run("test_invalid_field_type", func(t *testing.T) {
			assertIsInvalid(t, register(map[string]admin.RadioStyle{"name": admin.Vertical}),
				"modeladmin.ValidationTestModel",
				"The value of 'radio_fields' refers to 'name', which is not an instance "+
					"of ForeignKey, and does not have a 'choices' definition.",
				"admin.E023")
		})
		// Django sets the value None; the zero RadioStyle stands in for it.
		t.Run("test_invalid_value", func(t *testing.T) {
			assertIsInvalid(t, register(map[string]admin.RadioStyle{"state": 0}),
				"modeladmin.ValidationTestModel",
				`The value of 'radio_fields["state"]' must be either admin.HORIZONTAL or admin.VERTICAL.`,
				"admin.E024")
		})
		t.Run("test_valid_case", func(t *testing.T) {
			assertIsValid(t, register(map[string]admin.RadioStyle{"state": admin.Vertical}))
		})
	})
}

// No Go counterpart: tests/modeladmin/test_checks.py::PrepopulatedFieldsCheckTests.test_not_list_or_tuple
// (admin.E029) and test_not_dictionary (admin.E026): PrepopulatedFields is a
// map[string][]string.

// Ported from Django: tests/modeladmin/test_checks.py::PrepopulatedFieldsCheckTests.test_missing_field
// Ported from Django: tests/modeladmin/test_checks.py::PrepopulatedFieldsCheckTests.test_missing_field_again
// Ported from Django: tests/modeladmin/test_checks.py::PrepopulatedFieldsCheckTests.test_invalid_field_type
// Ported from Django: tests/modeladmin/test_checks.py::PrepopulatedFieldsCheckTests.test_valid_case
// Ported from Django: tests/modeladmin/test_checks.py::PrepopulatedFieldsCheckTests.test_one_to_one_field
func TestPrepopulatedFieldsCheck(t *testing.T) {
	djangotest.EachDB(t, modeladminCheckModels(), func(t *testing.T, db *orm.DB) {
		with := func(t *testing.T, fields map[string][]string) *admin.AdminSite {
			t.Helper()
			return checkSite(t, db, func(site *admin.AdminSite) {
				admin.Register[ValidationTestModel](site, admin.ModelAdmin{PrepopulatedFields: fields})
			})
		}
		t.Run("test_missing_field", func(t *testing.T) {
			assertIsInvalid(t, with(t, map[string][]string{"non_existent_field": {"slug"}}),
				"modeladmin.ValidationTestModel",
				"The value of 'prepopulated_fields' refers to 'non_existent_field', "+
					"which is not a field of 'modeladmin.ValidationTestModel'.",
				"admin.E027")
		})
		t.Run("test_missing_field_again", func(t *testing.T) {
			assertIsInvalid(t, with(t, map[string][]string{"slug": {"non_existent_field"}}),
				"modeladmin.ValidationTestModel",
				"The value of 'prepopulated_fields[\"slug\"][0]' refers to "+
					"'non_existent_field', which is not a field of "+
					"'modeladmin.ValidationTestModel'.",
				"admin.E030")
		})
		t.Run("test_invalid_field_type", func(t *testing.T) {
			assertIsInvalid(t, with(t, map[string][]string{"users": {"name"}}),
				"modeladmin.ValidationTestModel",
				"The value of 'prepopulated_fields' refers to 'users', which must not be "+
					"a DateTimeField, a ForeignKey, a OneToOneField, or a ManyToManyField.",
				"admin.E028")
		})
		t.Run("test_valid_case", func(t *testing.T) {
			assertIsValid(t, with(t, map[string][]string{"slug": {"name"}}))
		})
		t.Run("test_one_to_one_field", func(t *testing.T) {
			assertIsInvalid(t, with(t, map[string][]string{"best_friend": {"name"}}),
				"modeladmin.ValidationTestModel",
				"The value of 'prepopulated_fields' refers to 'best_friend', which must "+
					"not be a DateTimeField, a ForeignKey, a OneToOneField, or a "+
					"ManyToManyField.",
				"admin.E028")
		})
	})
}

// No Go counterpart: tests/modeladmin/test_checks.py::ListSelectRelatedCheckTests.test_invalid_type
// (admin.E117) and test_valid_case: ListSelectRelated is a []string. Its
// value True, which Django 6.1 deprecates, has no counterpart.

// listDisplayE108 is Django's admin.E108 message for name on
// ValidationTestModel. Django names the ModelAdmin class; the Go ModelAdmin
// has no class name, so the message says 'ModelAdmin'.
func listDisplayE108(name string) string {
	return "The value of 'list_display[0]' refers to '" + name + "', " +
		"which is not a callable or attribute of 'ModelAdmin', " +
		"or an attribute, method, or field on 'modeladmin.ValidationTestModel'."
}

// listDisplayE109 is Django's admin.E109 message.
const listDisplayE109 = "The value of 'list_display[0]' must not be a many-to-many field or a reverse foreign key."

// Ported from Django: tests/modeladmin/test_checks.py::ListDisplayTests.test_missing_field
// Ported from Django: tests/modeladmin/test_checks.py::ListDisplayTests.test_missing_related_field
// Ported from Django: tests/modeladmin/test_checks.py::ListDisplayTests.test_invalid_field_type
// Ported from Django: tests/modeladmin/test_checks.py::ListDisplayTests.test_invalid_reverse_related_field
// Ported from Django: tests/modeladmin/test_checks.py::ListDisplayTests.test_invalid_related_field
// Ported from Django: tests/modeladmin/test_checks.py::ListDisplayTests.test_invalid_m2m_related_name
// Ported from Django: tests/modeladmin/test_checks.py::ListDisplayTests.test_valid_case
func TestListDisplayCheck(t *testing.T) {
	djangotest.EachDB(t, modeladminCheckModels(), func(t *testing.T, db *orm.DB) {
		// FIXME-PORT: test_not_iterable (admin.E107) has no Go counterpart:
		// ListDisplay is a []string. test_valid_field_accessible_via_instance
		// needs a custom model field class, which Go models do not have.

		t.Run("test_missing_field", func(t *testing.T) {
			site := checkSite(t, db, func(site *admin.AdminSite) {
				admin.Register[ValidationTestModel](site, admin.ModelAdmin{ListDisplay: []string{"non_existent_field"}})
			})
			assertIsInvalid(t, site, "modeladmin.ValidationTestModel", listDisplayE108("non_existent_field"), "admin.E108")
		})

		t.Run("test_missing_related_field", func(t *testing.T) {
			site := checkSite(t, db, func(site *admin.AdminSite) {
				admin.Register[ValidationTestModel](site, admin.ModelAdmin{ListDisplay: []string{"band__non_existent_field"}})
			})
			assertIsInvalid(t, site, "modeladmin.ValidationTestModel", listDisplayE108("band__non_existent_field"), "admin.E108")
		})

		t.Run("test_invalid_field_type", func(t *testing.T) {
			site := checkSite(t, db, func(site *admin.AdminSite) {
				admin.Register[ValidationTestModel](site, admin.ModelAdmin{ListDisplay: []string{"users"}})
			})
			assertIsInvalid(t, site, "modeladmin.ValidationTestModel", listDisplayE109, "admin.E109")
		})

		for _, tc := range []struct{ test, name string }{
			{"test_invalid_reverse_related_field", "song_set"},
			{"test_invalid_related_field", "song"},
			{"test_invalid_m2m_related_name", "featured"},
		} {
			t.Run(tc.test, func(t *testing.T) {
				site := checkSite(t, db, func(site *admin.AdminSite) {
					admin.Register[Band](site, admin.ModelAdmin{ListDisplay: []string{tc.name}})
				})
				assertIsInvalid(t, site, "modeladmin.Band", listDisplayE109, "admin.E109")
			})
		}

		t.Run("test_valid_case", func(t *testing.T) {
			site := checkSite(t, db, func(site *admin.AdminSite) {
				admin.Register[ValidationTestModel](site, admin.ModelAdmin{
					ListDisplay: []string{"name", "decade_published_in", "a_method"},
					DisplayColumns: map[string]admin.DisplayColumn{
						"decade_published_in": {Value: func(any) any { return "" }},
						"a_method":            {Value: func(any) any { return "" }},
					},
				})
			})
			assertIsValid(t, site)
		})
	})
}

// Ported from Django: tests/modeladmin/test_checks.py::FieldsetsCheckTests.test_valid_case
// Ported from Django: tests/modeladmin/test_checks.py::FieldsetsCheckTests.test_missing_fields_key
// Ported from Django: tests/modeladmin/test_checks.py::FieldsetsCheckTests.test_specified_both_fields_and_fieldsets
// Ported from Django: tests/modeladmin/test_checks.py::FieldsetsCheckTests.test_duplicate_fields
// Ported from Django: tests/modeladmin/test_checks.py::FieldsetsCheckTests.test_duplicate_fields_in_fieldsets
// Ported from Django: tests/modeladmin/test_checks.py::FieldsetsCheckTests.test_fieldsets_with_custom_form_validation
func TestFieldsetsCheck(t *testing.T) {
	djangotest.EachDB(t, modeladminCheckModels(), func(t *testing.T, db *orm.DB) {
		// FIXME-PORT: test_not_iterable (admin.E007), test_non_iterable_item
		// (admin.E008), test_item_not_a_pair (admin.E009) and
		// test_second_element_of_item_not_a_dict (admin.E010) have no Go
		// counterpart: Fieldsets is a []Fieldset.
		general := []admin.Fieldset{{Name: "General", Fields: []any{"name"}}}

		t.Run("test_valid_case", func(t *testing.T) {
			site := checkSite(t, db, func(site *admin.AdminSite) {
				admin.Register[ValidationTestModel](site, admin.ModelAdmin{Fieldsets: general})
			})
			assertIsValid(t, site)
		})

		t.Run("test_missing_fields_key", func(t *testing.T) {
			// A Fieldset with nil Fields stands in for a fieldset without
			// the "fields" key.
			site := checkSite(t, db, func(site *admin.AdminSite) {
				admin.Register[ValidationTestModel](site, admin.ModelAdmin{Fieldsets: []admin.Fieldset{{Name: "General"}}})
			})
			assertIsInvalid(t, site, "modeladmin.ValidationTestModel",
				"The value of 'fieldsets[0][1]' must contain the key 'fields'.", "admin.E011")

			site = checkSite(t, db, func(site *admin.AdminSite) {
				admin.Register[ValidationTestModel](site, admin.ModelAdmin{Fieldsets: general})
			})
			assertIsValid(t, site)
		})

		t.Run("test_specified_both_fields_and_fieldsets", func(t *testing.T) {
			site := checkSite(t, db, func(site *admin.AdminSite) {
				admin.Register[ValidationTestModel](site, admin.ModelAdmin{Fieldsets: general, Fields: []any{"name"}})
			})
			assertIsInvalid(t, site, "modeladmin.ValidationTestModel",
				"Both 'fieldsets' and 'fields' are specified.", "admin.E005")
		})

		t.Run("test_duplicate_fields", func(t *testing.T) {
			site := checkSite(t, db, func(site *admin.AdminSite) {
				admin.Register[ValidationTestModel](site, admin.ModelAdmin{Fieldsets: []admin.Fieldset{
					{Fields: []any{"name", "name"}},
				}})
			})
			assertIsInvalid(t, site, "modeladmin.ValidationTestModel",
				"There are duplicate field(s) in 'fieldsets[0][1]'.", "admin.E012", "Remove duplicates of 'name'.")
		})

		t.Run("test_duplicate_fields_in_fieldsets", func(t *testing.T) {
			site := checkSite(t, db, func(site *admin.AdminSite) {
				admin.Register[ValidationTestModel](site, admin.ModelAdmin{Fieldsets: []admin.Fieldset{
					{Fields: []any{"name"}},
					{Fields: []any{"name"}},
				}})
			})
			assertIsInvalid(t, site, "modeladmin.ValidationTestModel",
				"There are duplicate field(s) in 'fieldsets[1][1]'.", "admin.E012", "Remove duplicates of 'name'.")
		})

		t.Run("test_fieldsets_with_custom_form_validation", func(t *testing.T) {
			site := checkSite(t, db, func(site *admin.AdminSite) {
				admin.Register[Band](site, admin.ModelAdmin{Fieldsets: []admin.Fieldset{{Name: "Band", Fields: []any{"name"}}}})
			})
			assertIsValid(t, site)
		})
	})
}

// Ported from Django: tests/modeladmin/test_checks.py::FieldsCheckTests.test_duplicate_fields_in_fields
// Ported from Django: tests/modeladmin/test_checks.py::FieldsCheckTests.test_inline
func TestFieldsCheck(t *testing.T) {
	djangotest.EachDB(t, modeladminCheckModels(), func(t *testing.T, db *orm.DB) {
		site := checkSite(t, db, func(site *admin.AdminSite) {
			admin.Register[ValidationTestModel](site, admin.ModelAdmin{Fields: []any{"name", "name"}})
		})
		assertIsInvalid(t, site, "modeladmin.ValidationTestModel",
			"The value of 'fields' contains duplicate field(s).", "admin.E006", "Remove duplicates of 'name'.")

		// Django sets the fields of the inline to 10; Fields is a Go []any,
		// so the Go counterpart is an entry that is not a field name.
		site = checkSite(t, db, func(site *admin.AdminSite) {
			admin.Register[ValidationTestModel](site, admin.ModelAdmin{Inlines: []admin.Inline{
				admin.TabularInline[ValidationTestModel, ValidationTestInlineModel](admin.InlineModelAdmin{Fields: []any{10}}),
			}})
		})
		assertInlineIsInvalid(t, site, "modeladmin.ValidationTestInlineModel",
			"The value of 'fields[0]' must be a field name or a []string.", "admin.E004")
	})
}

// Ported from Django: tests/modeladmin/test_checks.py::InlinesCheckTests.test_not_model_admin
// Ported from Django: tests/modeladmin/test_checks.py::InlinesCheckTests.test_invalid_model_type
// Ported from Django: tests/modeladmin/test_checks.py::InlinesCheckTests.test_valid_case
// Ported from Django: tests/modeladmin/test_checks.py::FkNameCheckTests.test_missing_field
// Ported from Django: tests/modeladmin/test_checks.py::FkNameCheckTests.test_valid_case
// Ported from Django: tests/modeladmin/test_checks.py::ExtraCheckTests.test_valid_case
// Ported from Django: tests/modeladmin/test_checks.py::MaxNumCheckTests.test_valid_case
// Ported from Django: tests/modeladmin/test_checks.py::MinNumCheckTests.test_valid_case
func TestInlinesCheck(t *testing.T) {
	// No Go counterpart: InlinesCheckTests.test_not_iterable,
	// test_not_correct_inline_field, test_invalid_callable and
	// test_invalid_model, ExtraCheckTests, MaxNumCheckTests and
	// MinNumCheckTests.test_not_integer and FormsetCheckTests check option
	// types that Go fixes at compile time: Inlines is a []admin.Inline,
	// Extra and MaxNum are *int, MinNum is an int, and an inline has no
	// formset class.
	// test_missing_model_field has none either: TabularInline and
	// StackedInline take the model as a type parameter. The proxy tests of
	// FkNameCheckTests have none, as the ORM has no proxy models.
	const model = "modeladmin.ValidationTestModel"
	const inlineModel = "modeladmin.ValidationTestInlineModel"
	inlineSite := func(t *testing.T, db *orm.DB, inlines ...admin.Inline) *admin.AdminSite {
		t.Helper()
		return checkSite(t, db, func(site *admin.AdminSite) {
			admin.Register[ValidationTestModel](site, admin.ModelAdmin{Inlines: inlines})
		})
	}
	tabular := admin.TabularInline[ValidationTestModel, ValidationTestInlineModel]
	djangotest.EachDB(t, modeladminCheckModels(), func(t *testing.T, db *orm.DB) {
		t.Run("test_not_model_admin", func(t *testing.T) {
			// An admin.Inline that TabularInline or StackedInline did not
			// make stands in for a class that is not an InlineModelAdmin.
			// Register rejects it at boot, where Django reports admin.E104.
			assertPanics(t, "admin: Register: an Inline must be made by TabularInline or StackedInline", func() {
				inlineSite(t, db, admin.Inline{})
			})
		})

		t.Run("test_invalid_model_type", func(t *testing.T) {
			// A type that is not a registered model stands in for
			// SomethingBad.
			assertIsInvalid(t, inlineSite(t, db, admin.TabularInline[ValidationTestModel, NotAModel](admin.InlineModelAdmin{})),
				model, "The value of 'inlines[0].model' must be a Model.", "admin.E106")
		})

		t.Run("test_valid_case", func(t *testing.T) {
			assertIsValid(t, inlineSite(t, db, tabular(admin.InlineModelAdmin{})))
		})

		t.Run("FkNameCheckTests.test_missing_field", func(t *testing.T) {
			site := inlineSite(t, db, tabular(admin.InlineModelAdmin{FKName: "non_existent_field"}))
			assertInlineIsInvalid(t, site, inlineModel,
				"'"+inlineModel+"' has no field named 'non_existent_field'.", "admin.E202")
		})

		t.Run("FkNameCheckTests.test_valid_case", func(t *testing.T) {
			assertIsValid(t, inlineSite(t, db, tabular(admin.InlineModelAdmin{FKName: "parent"})))
		})

		t.Run("ExtraCheckTests.test_valid_case", func(t *testing.T) {
			assertIsValid(t, inlineSite(t, db, tabular(admin.InlineModelAdmin{Extra: new(2)})))
		})

		t.Run("MaxNumCheckTests.test_valid_case", func(t *testing.T) {
			assertIsValid(t, inlineSite(t, db, tabular(admin.InlineModelAdmin{MaxNum: new(2)})))
		})

		t.Run("MinNumCheckTests.test_valid_case", func(t *testing.T) {
			assertIsValid(t, inlineSite(t, db, tabular(admin.InlineModelAdmin{MinNum: 2})))
		})
	})
}

// TestInlineParentCheck has no Django counterpart: a Django inline takes
// its parent model from the ModelAdmin, while TabularInline and
// StackedInline name it as a type parameter, which Register checks.
func TestInlineParentCheck(t *testing.T) {
	djangotest.EachDB(t, modeladminCheckModels(), func(t *testing.T, db *orm.DB) {
		assertPanics(t, "admin: Register: an inline of admin_test.ValidationTestModel cannot edit admin_test.Band", func() {
			checkSite(t, db, func(site *admin.AdminSite) {
				admin.Register[Band](site, admin.ModelAdmin{Inlines: []admin.Inline{
					admin.TabularInline[ValidationTestModel, ValidationTestInlineModel](admin.InlineModelAdmin{}),
				}})
			})
		})
	})
}

// assertPanics asserts that fn panics with the message want.
func assertPanics(t *testing.T, want string, fn func()) {
	t.Helper()
	defer func() {
		t.Helper()
		if got := recover(); got != want {
			t.Errorf("panic = %v, want %q", got, want)
		}
	}()
	fn()
}

// Ported from Django: tests/modeladmin/test_checks.py::RawIdCheckTests.test_missing_field
// Ported from Django: tests/modeladmin/test_checks.py::RawIdCheckTests.test_invalid_field_type
// Ported from Django: tests/modeladmin/test_checks.py::RawIdCheckTests.test_valid_case
// Ported from Django: tests/modeladmin/test_checks.py::RawIdCheckTests.test_field_attname
func TestRawIDCheck(t *testing.T) {
	djangotest.EachDB(t, modeladminCheckModels(), func(t *testing.T, db *orm.DB) {
		// FIXME-PORT: test_not_iterable (admin.E001) has no Go counterpart:
		// RawIDFields is a []string. test_valid_case_stored_generated_field
		// and test_valid_case_virtual_generated_field check a GeneratedField,
		// which the Go ORM does not have.

		t.Run("test_missing_field", func(t *testing.T) {
			site := checkSite(t, db, func(site *admin.AdminSite) {
				admin.Register[ValidationTestModel](site, admin.ModelAdmin{RawIDFields: []string{"non_existent_field"}})
			})
			assertIsInvalid(t, site, "modeladmin.ValidationTestModel",
				"The value of 'raw_id_fields[0]' refers to 'non_existent_field', "+
					"which is not a field of 'modeladmin.ValidationTestModel'.",
				"admin.E002")
		})

		t.Run("test_invalid_field_type", func(t *testing.T) {
			site := checkSite(t, db, func(site *admin.AdminSite) {
				admin.Register[ValidationTestModel](site, admin.ModelAdmin{RawIDFields: []string{"name"}})
			})
			assertIsInvalid(t, site, "modeladmin.ValidationTestModel",
				"The value of 'raw_id_fields[0]' must be a foreign key or a many-to-many field.", "admin.E003")
		})

		t.Run("test_valid_case", func(t *testing.T) {
			site := checkSite(t, db, func(site *admin.AdminSite) {
				admin.Register[ValidationTestModel](site, admin.ModelAdmin{RawIDFields: []string{"users"}})
			})
			assertIsValid(t, site)
		})

		t.Run("test_field_attname", func(t *testing.T) {
			site := checkSite(t, db, func(site *admin.AdminSite) {
				admin.Register[ValidationTestModel](site, admin.ModelAdmin{RawIDFields: []string{"band_id"}})
			})
			assertIsInvalid(t, site, "modeladmin.ValidationTestModel",
				"The value of 'raw_id_fields[0]' refers to 'band_id', "+
					"which is not a field of 'modeladmin.ValidationTestModel'.",
				"admin.E002")
		})
	})
}

// Ported from Django: tests/modeladmin/test_checks.py::DateHierarchyCheckTests.test_missing_field
// Ported from Django: tests/modeladmin/test_checks.py::DateHierarchyCheckTests.test_invalid_field_type
// Ported from Django: tests/modeladmin/test_checks.py::DateHierarchyCheckTests.test_valid_case
// Ported from Django: tests/modeladmin/test_checks.py::DateHierarchyCheckTests.test_related_valid_case
// Ported from Django: tests/modeladmin/test_checks.py::DateHierarchyCheckTests.test_related_invalid_field_type
func TestDateHierarchyCheck(t *testing.T) {
	djangotest.EachDB(t, modeladminCheckModels(), func(t *testing.T, db *orm.DB) {
		// No Go counterpart: test_related_invalid_field_type_stored_generated_field
		// and test_related_invalid_field_type_virtual_generated_field check a
		// GeneratedField, which the Go ORM does not have.
		register := func(dateHierarchy string) *admin.AdminSite {
			return checkSite(t, db, func(site *admin.AdminSite) {
				admin.Register[ValidationTestModel](site, admin.ModelAdmin{DateHierarchy: dateHierarchy})
			})
		}

		t.Run("test_missing_field", func(t *testing.T) {
			assertIsInvalid(t, register("non_existent_field"), "modeladmin.ValidationTestModel",
				"The value of 'date_hierarchy' refers to 'non_existent_field', which does not refer to a Field.",
				"admin.E127")
		})

		t.Run("test_invalid_field_type", func(t *testing.T) {
			assertIsInvalid(t, register("name"), "modeladmin.ValidationTestModel",
				"The value of 'date_hierarchy' must be a DateField or DateTimeField.", "admin.E128")
		})

		t.Run("test_valid_case", func(t *testing.T) {
			assertIsValid(t, register("pub_date"))
		})

		t.Run("test_related_valid_case", func(t *testing.T) {
			assertIsValid(t, register("band__sign_date"))
		})

		t.Run("test_related_invalid_field_type", func(t *testing.T) {
			assertIsInvalid(t, register("band__name"), "modeladmin.ValidationTestModel",
				"The value of 'date_hierarchy' must be a DateField or DateTimeField.", "admin.E128")
		})
	})
}

// Ported from Django: tests/modeladmin/test_checks.py::AutocompleteFieldsTests.test_autocomplete_e037
// Ported from Django: tests/modeladmin/test_checks.py::AutocompleteFieldsTests.test_autocomplete_e38
// Ported from Django: tests/modeladmin/test_checks.py::AutocompleteFieldsTests.test_autocomplete_e039
// Ported from Django: tests/modeladmin/test_checks.py::AutocompleteFieldsTests.test_autocomplete_e040
// Ported from Django: tests/modeladmin/test_checks.py::AutocompleteFieldsTests.test_autocomplete_is_valid
// Ported from Django: tests/modeladmin/test_checks.py::AutocompleteFieldsTests.test_autocomplete_is_onetoone
//
// Django's admin.E039 and admin.E040 messages name the ModelAdmin classes. A
// Go ModelAdmin has no class name, so the messages say 'ModelAdmin' for the
// referencing admin and name the model of the related one.
func TestAutocompleteFieldsCheck(t *testing.T) {
	djangotest.EachDB(t, modeladminCheckModels(), func(t *testing.T, db *orm.DB) {
		// FIXME-PORT: test_autocomplete_e036 has no Go counterpart:
		// AutocompleteFields is a []string. test_autocomplete_e039_unresolved_model
		// needs a foreign key to a model that is not installed, which the Go
		// registry rejects.

		t.Run("test_autocomplete_e037", func(t *testing.T) {
			site := checkSite(t, db, func(site *admin.AdminSite) {
				admin.Register[ValidationTestModel](site, admin.ModelAdmin{AutocompleteFields: []string{"nonexistent"}})
			})
			assertIsInvalid(t, site, "modeladmin.ValidationTestModel",
				"The value of 'autocomplete_fields[0]' refers to 'nonexistent', "+
					"which is not a field of 'modeladmin.ValidationTestModel'.",
				"admin.E037")
		})

		t.Run("test_autocomplete_e38", func(t *testing.T) {
			site := checkSite(t, db, func(site *admin.AdminSite) {
				admin.Register[ValidationTestModel](site, admin.ModelAdmin{AutocompleteFields: []string{"name"}})
			})
			assertIsInvalid(t, site, "modeladmin.ValidationTestModel",
				"The value of 'autocomplete_fields[0]' must be a foreign key or a many-to-many field.",
				"admin.E038")
		})

		t.Run("test_autocomplete_e039", func(t *testing.T) {
			site := checkSite(t, db, func(site *admin.AdminSite) {
				admin.Register[Song](site, admin.ModelAdmin{AutocompleteFields: []string{"band"}})
			})
			assertIsInvalid(t, site, "modeladmin.Song",
				`An admin for model "Band" has to be registered to be referenced by ModelAdmin.autocomplete_fields.`,
				"admin.E039")
		})

		t.Run("test_autocomplete_e040", func(t *testing.T) {
			site := checkSite(t, db, func(site *admin.AdminSite) {
				admin.Register[Band](site, admin.ModelAdmin{})
				admin.Register[Song](site, admin.ModelAdmin{AutocompleteFields: []string{"featuring"}})
			})
			assertIsInvalid(t, site, "modeladmin.Song",
				`ModelAdmin of modeladmin.Band must define "search_fields", `+
					"because it's referenced by ModelAdmin.autocomplete_fields.",
				"admin.E040")
		})

		t.Run("test_autocomplete_is_valid", func(t *testing.T) {
			site := checkSite(t, db, func(site *admin.AdminSite) {
				admin.Register[Band](site, admin.ModelAdmin{SearchFields: []string{"name"}})
				admin.Register[Song](site, admin.ModelAdmin{AutocompleteFields: []string{"featuring"}})
			})
			assertIsValid(t, site)
		})

		t.Run("test_autocomplete_is_onetoone", func(t *testing.T) {
			site := checkSite(t, db, func(site *admin.AdminSite) {
				admin.Register[auth.User](site, admin.ModelAdmin{SearchFields: []string{"username"}})
				admin.Register[ValidationTestModel](site, admin.ModelAdmin{AutocompleteFields: []string{"best_friend"}})
			})
			assertIsValid(t, site)
		})
	})
}

// No Go counterpart: tests/modeladmin/test_checks.py::SaveAsCheckTests and
// SaveOnTopCheckTests check that save_as and save_on_top are booleans, which
// Go fixes at compile time: ModelAdmin.SaveAs and SaveOnTop are bools.

// Ported from Django: tests/modeladmin/test_checks.py::ListDisplayEditableTests.test_list_display_links_is_none
// Ported from Django: tests/modeladmin/test_checks.py::ListDisplayEditableTests.test_list_display_first_item_same_as_list_editable_first_item
// Ported from Django: tests/modeladmin/test_checks.py::ListDisplayEditableTests.test_list_display_first_item_in_list_editable
// Ported from Django: tests/modeladmin/test_checks.py::ListDisplayEditableTests.test_list_display_first_item_same_as_list_editable_no_list_display_links
// Ported from Django: tests/modeladmin/test_checks.py::ListDisplayEditableTests.test_list_display_first_item_in_list_editable_no_list_display_links
// Ported from Django: tests/modeladmin/test_checks.py::ListDisplayEditableTests.test_both_list_editable_and_list_display_links
func TestListDisplayEditableCheck(t *testing.T) {
	djangotest.EachDB(t, modeladminCheckModels(), func(t *testing.T, db *orm.DB) {
		const model = "modeladmin.ValidationTestModel"
		register := func(a admin.ModelAdmin) *admin.AdminSite {
			return checkSite(t, db, func(site *admin.AdminSite) { admin.Register[ValidationTestModel](site, a) })
		}

		t.Run("test_list_display_links_is_none", func(t *testing.T) {
			// An empty ListDisplayLinks is Django's list_display_links = None.
			display := []string{"name", "slug", "pub_date"}
			assertIsValid(t, register(admin.ModelAdmin{
				ListDisplay: display, ListEditable: display, ListDisplayLinks: []string{},
			}))
		})

		t.Run("test_list_display_first_item_same_as_list_editable_first_item", func(t *testing.T) {
			assertIsValid(t, register(admin.ModelAdmin{
				ListDisplay:      []string{"name", "slug", "pub_date"},
				ListEditable:     []string{"name", "slug"},
				ListDisplayLinks: []string{"pub_date"},
			}))
		})

		t.Run("test_list_display_first_item_in_list_editable", func(t *testing.T) {
			assertIsValid(t, register(admin.ModelAdmin{
				ListDisplay:      []string{"name", "slug", "pub_date"},
				ListEditable:     []string{"slug", "name"},
				ListDisplayLinks: []string{"pub_date"},
			}))
		})

		t.Run("test_list_display_first_item_same_as_list_editable_no_list_display_links", func(t *testing.T) {
			site := register(admin.ModelAdmin{ListDisplay: []string{"name"}, ListEditable: []string{"name"}})
			assertIsInvalid(t, site, model,
				"The value of 'list_editable[0]' refers to the first field in 'list_display' ('name'), "+
					"which cannot be used unless 'list_display_links' is set.",
				"admin.E124")
		})

		t.Run("test_list_display_first_item_in_list_editable_no_list_display_links", func(t *testing.T) {
			site := register(admin.ModelAdmin{
				ListDisplay:  []string{"name", "slug", "pub_date"},
				ListEditable: []string{"slug", "name"},
			})
			assertIsInvalid(t, site, model,
				"The value of 'list_editable[1]' refers to the first field in 'list_display' ('name'), "+
					"which cannot be used unless 'list_display_links' is set.",
				"admin.E124")
		})

		t.Run("test_both_list_editable_and_list_display_links", func(t *testing.T) {
			site := register(admin.ModelAdmin{
				ListEditable:     []string{"name"},
				ListDisplay:      []string{"name"},
				ListDisplayLinks: []string{"name"},
			})
			assertIsInvalid(t, site, model,
				"The value of 'name' cannot be in both 'list_editable' and 'list_display_links'.",
				"admin.E123")
		})
	})
}
