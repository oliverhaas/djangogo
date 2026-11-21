package admin_test

import (
	"testing"

	"github.com/oliverhaas/djangogo/admin"
	"github.com/oliverhaas/djangogo/auth"
	"github.com/oliverhaas/djangogo/checks"
	"github.com/oliverhaas/djangogo/contenttypes"
	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
)

// Band stands in for tests/modeladmin/models.py::Band.
type Band struct {
	ID   int64
	Name string `orm:"max_length=100"`
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

// ValidationTestModel stands in for
// tests/modeladmin/models.py::ValidationTestModel, with the fields the ported
// checks address.
type ValidationTestModel struct {
	ID    int64
	Name  string             `orm:"max_length=100"`
	Users orm.M2M[auth.User] `orm:"related_name=validationtestmodels"`
	Band  orm.FK[Band]       `orm:"on_delete=cascade"`
}

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
		djangotest.App("modeladmin",
			&Band{}, &Song{}, &ValidationTestModel{}, &Contact{}, &Customer{}, &Artist{}, &BandArtist{},
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
// exactly one error, with the message and ID given, about the ModelAdmin of
// model.
func assertIsInvalid(t *testing.T, site *admin.AdminSite, model, msg, id string) {
	t.Helper()
	got := site.Check()
	if len(got) != 1 {
		t.Fatalf("Check() = %v, want one error %s %q", got, id, msg)
	}
	m := got[0]
	if m.Level != checks.LevelError || m.Msg != msg || m.ID != id || m.Hint != "" {
		t.Errorf("Check() = %+v, want error %s %q", m, id, msg)
	}
	obj, ok := m.Obj.(interface{ Label() string })
	if !ok || obj.Label() != "ModelAdmin of "+model {
		t.Errorf("Check() object = %v, want the ModelAdmin of %s", m.Obj, model)
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
