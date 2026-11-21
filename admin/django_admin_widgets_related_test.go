package admin_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/admin"
	"github.com/oliverhaas/djangogo/auth"
	"github.com/oliverhaas/djangogo/contenttypes"
	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/forms"
	"github.com/oliverhaas/djangogo/orm"
)

// WidgetMember stands in for tests/admin_widgets/models.py::Member.
type WidgetMember struct {
	ID        int64
	Name      string     `orm:"max_length=100"`
	Birthdate *time.Time `orm:"null;blank"`
	Gender    string     `orm:"max_length=1;blank"`
	Email     string     `orm:"type=email;blank"`
}

// Meta gives the gender its choices, as Django's Member does.
func (WidgetMember) Meta() orm.Meta {
	return orm.Meta{Choices: map[string][]orm.Choice{
		"Gender": {{Value: "M", Label: "Male"}, {Value: "F", Label: "Female"}},
	}}
}

// String returns the member name.
func (m WidgetMember) String() string { return m.Name }

// WidgetBand stands in for tests/admin_widgets/models.py::Band, without the
// uuid field: a Go foreign key targets the primary key.
type WidgetBand struct {
	ID      int64
	Name    string                `orm:"max_length=100"`
	Style   string                `orm:"max_length=20"`
	Members orm.M2M[WidgetMember] `orm:"related_name=bands"`
}

// String returns the band name.
func (b WidgetBand) String() string { return b.Name }

// WidgetAlbum stands in for tests/admin_widgets/models.py::Album.
type WidgetAlbum struct {
	ID        int64
	Band      orm.FK[WidgetBand]  `orm:"on_delete=cascade"`
	Featuring orm.M2M[WidgetBand] `orm:"related_name=featured"`
	Name      string              `orm:"max_length=100"`
}

// String returns the album name.
func (a WidgetAlbum) String() string { return a.Name }

// WidgetReleaseEvent stands in for tests/admin_widgets/models.py::ReleaseEvent,
// with its own primary key: a Go foreign key cannot be the primary key.
type WidgetReleaseEvent struct {
	ID    int64
	Album orm.FK[WidgetAlbum] `orm:"on_delete=cascade"`
	Name  string              `orm:"max_length=100"`
}

// String returns the release event name.
func (e WidgetReleaseEvent) String() string { return e.Name }

// Meta orders release events by name, as Django's ReleaseEvent does.
func (WidgetReleaseEvent) Meta() orm.Meta { return orm.Meta{Ordering: []string{"name"}} }

// WidgetEvent stands in for tests/admin_widgets/models.py::Event.
type WidgetEvent struct {
	ID              int64
	MainBand        orm.FK[WidgetBand]  `orm:"on_delete=cascade;related_name=events_main_band_at"`
	SupportingBands orm.M2M[WidgetBand] `orm:"blank;related_name=events_supporting_band_at"`
	StartDate       *orm.Date           `orm:"null;blank"`
	StartTime       *orm.Time           `orm:"null;blank"`
	Description     string              `orm:"type=text;blank"`
	Link            string              `orm:"type=url;blank"`
	MinAge          *int64              `orm:"null;blank"`
}

// Honeycomb stands in for tests/admin_widgets/models.py::Honeycomb, with an
// integer primary key: a Go foreign key targets an integer key.
type Honeycomb struct {
	ID       int64
	Location string `orm:"max_length=20"`
}

// Bee stands in for tests/admin_widgets/models.py::Bee: a foreign key to a
// model without an admin.
type Bee struct {
	ID        int64
	Honeycomb orm.FK[Honeycomb] `orm:"on_delete=cascade"`
}

// Individual stands in for tests/admin_widgets/models.py::Individual: two
// foreign keys to itself, without an admin.
type Individual struct {
	ID       int64
	Name     string             `orm:"max_length=20"`
	Parent   orm.FK[Individual] `orm:"null;on_delete=set_null"`
	Soulmate orm.FK[Individual] `orm:"null;on_delete=cascade;related_name=soulmates"`
}

// WidgetInventory stands in for tests/admin_widgets/models.py::Inventory. Its
// parent references the primary key, as the Go ORM has no to_field.
type WidgetInventory struct {
	ID     int64
	Parent orm.FK[WidgetInventory] `orm:"null;blank;on_delete=set_null"`
	Name   string                  `orm:"max_length=20"`
}

// String returns the inventory name.
func (i WidgetInventory) String() string { return i.Name }

// WidgetVideoStream stands in for tests/admin_widgets/models.py::VideoStream.
type WidgetVideoStream struct {
	ID           int64
	ReleaseEvent orm.FK[WidgetReleaseEvent] `orm:"on_delete=cascade"`
}

// widgetRelatedModels returns the model set of the ported raw-id and
// autocomplete widget tests.
func widgetRelatedModels() []any {
	return []any{auth.App{}, contenttypes.App{}, admin.App{}, djangotest.App("admin_widgets",
		&Company{}, &Advisor{}, &WidgetMember{}, &WidgetBand{}, &WidgetAlbum{}, &WidgetReleaseEvent{},
		&WidgetEvent{}, &Honeycomb{}, &Bee{}, &Individual{}, &WidgetVideoStream{}, &WidgetInventory{},
	)}
}

// widgetEnv returns the test environment of the admin_widgets tests, a site
// with the registrations of Django's widget_admin_site, and the album under
// album.
func widgetEnv(t *testing.T, db *orm.DB, album admin.ModelAdmin) *adminTestEnv {
	t.Helper()
	return newAdminTestEnv(t, db, func(site *admin.AdminSite) {
		admin.Register[WidgetMember](site, admin.ModelAdmin{})
		admin.Register[WidgetBand](site, admin.ModelAdmin{SearchFields: []string{"name"}})
		admin.Register[WidgetEvent](site, admin.ModelAdmin{RawIDFields: []string{"main_band", "supporting_bands"}})
		admin.Register[WidgetAlbum](site, album)
		admin.Register[WidgetReleaseEvent](site, admin.ModelAdmin{SearchFields: []string{"name"}})
		admin.Register[Bee](site, admin.ModelAdmin{})
		admin.Register[Advisor](site, admin.ModelAdmin{})
	})
}

// modelField returns the field or many-to-many field name of the model with
// the label given.
func modelField(t *testing.T, db *orm.DB, label, name string) (*orm.Model, *orm.Field) {
	t.Helper()
	m, ok := db.Registry().Get(label)
	if !ok {
		t.Fatalf("no model %s", label)
	}
	if f, ok := m.FieldByName(name); ok {
		return m, f
	}
	if f, ok := m.ManyToManyField(name); ok {
		return m, f
	}
	t.Fatalf("no field %s.%s", label, name)
	return nil, nil
}

// create saves obj or fails the test.
func create[T any](t *testing.T, db *orm.DB, obj *T) *T {
	t.Helper()
	if err := orm.Query[T](db).Create(t.Context(), obj); err != nil {
		t.Fatalf("create %T: %v", obj, err)
	}
	return obj
}

// Ported from Django: tests/admin_widgets/tests.py::ForeignKeyRawIdWidgetTest.test_render
// Ported from Django: tests/admin_widgets/tests.py::ForeignKeyRawIdWidgetTest.test_fk_related_model_not_in_admin
// Ported from Django: tests/admin_widgets/tests.py::ForeignKeyRawIdWidgetTest.test_fk_to_self_model_not_in_admin
func TestForeignKeyRawIDWidget(t *testing.T) {
	djangotest.EachDB(t, widgetRelatedModels(), func(t *testing.T, db *orm.DB) {
		// FIXME-PORT: test_relations_to_non_primary_key needs a foreign key
		// with to_field, test_proper_manager_for_label_lookup a custom
		// manager, test_render_unsafe_limit_choices_to limit_choices_to and
		// test_render_fk_as_pk_model a foreign key as the primary key; the Go
		// ORM has none of them.
		ctx := context.Background()
		env := widgetEnv(t, db, admin.ModelAdmin{})

		t.Run("test_render", func(t *testing.T) {
			// Django renders the band by its uuid, the to_field of
			// Album.band; the Go foreign key targets the primary key.
			band := create(t, db, &WidgetBand{Name: "Linkin Park"})
			album := &WidgetAlbum{Name: "Hybrid Theory"}
			album.Band.SetPK(band.ID)
			create(t, db, album)
			_, f := modelField(t, db, "admin_widgets.WidgetAlbum", "Band")
			w := admin.NewForeignKeyRawIDWidget(ctx, env.site, f)
			djangotest.AssertHTMLEqual(t, w.Render("test", band.ID, forms.Attrs{}), fmt.Sprintf(
				`<div><input type="text" name="test" value="%d" class="vForeignKeyRawIdAdminField">`+
					`<a href="%s?_to_field=id" class="related-lookup" id="lookup_id_test" title="Lookup"></a>&nbsp;<strong>`+
					`<a href="%s">Linkin Park</a></strong></div>`,
				band.ID, mustReverse(t, env.router, "admin:admin_widgets_widgetband_changelist"),
				mustReverse(t, env.router, "admin:admin_widgets_widgetband_change", band.ID)))

			_, f = modelField(t, db, "admin_widgets.WidgetReleaseEvent", "Album")
			w = admin.NewForeignKeyRawIDWidget(ctx, env.site, f)
			djangotest.AssertHTMLEqual(t, w.Render("test", nil, forms.Attrs{}),
				`<div><input type="text" name="test" class="vForeignKeyRawIdAdminField">`+
					`<a href="`+mustReverse(t, env.router, "admin:admin_widgets_widgetalbum_changelist")+
					`?_to_field=id" class="related-lookup" id="lookup_id_test" title="Lookup"></a></div>`)
		})

		t.Run("test_fk_related_model_not_in_admin", func(t *testing.T) {
			// FK to a model not registered with admin site. Raw ID widget
			// should have no magnifying glass link. See #16542
			bigHoneycomb := create(t, db, &Honeycomb{Location: "Old tree"})
			bee := &Bee{}
			bee.Honeycomb.SetPK(bigHoneycomb.ID)
			create(t, db, bee)
			_, f := modelField(t, db, "admin_widgets.Bee", "Honeycomb")
			w := admin.NewForeignKeyRawIDWidget(ctx, env.site, f)
			djangotest.AssertHTMLEqual(t, w.Render("honeycomb_widget", bigHoneycomb.ID, forms.Attrs{}), fmt.Sprintf(
				`<input type="text" name="honeycomb_widget" value="%d">&nbsp;<strong>Honeycomb object (%d)</strong>`,
				bigHoneycomb.ID, bigHoneycomb.ID))
		})

		t.Run("test_fk_to_self_model_not_in_admin", func(t *testing.T) {
			// FK to self, not registered with admin site. Raw ID widget
			// should have no magnifying glass link. See #16542
			subject1 := create(t, db, &Individual{Name: "Subject #1"})
			child := &Individual{Name: "Child"}
			child.Parent.SetPK(subject1.ID)
			create(t, db, child)
			_, f := modelField(t, db, "admin_widgets.Individual", "Parent")
			w := admin.NewForeignKeyRawIDWidget(ctx, env.site, f)
			djangotest.AssertHTMLEqual(t, w.Render("individual_widget", subject1.ID, forms.Attrs{}), fmt.Sprintf(
				`<input type="text" name="individual_widget" value="%d">&nbsp;<strong>Individual object (%d)</strong>`,
				subject1.ID, subject1.ID))
		})
	})
}

// Ported from Django: tests/admin_widgets/tests.py::ManyToManyRawIdWidgetTest.test_render
// Ported from Django: tests/admin_widgets/tests.py::ManyToManyRawIdWidgetTest.test_m2m_related_model_not_in_admin
func TestManyToManyRawIDWidget(t *testing.T) {
	djangotest.EachDB(t, widgetRelatedModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		env := widgetEnv(t, db, admin.ModelAdmin{})

		t.Run("test_render", func(t *testing.T) {
			band := create(t, db, &WidgetBand{Name: "Linkin Park"})
			m1 := create(t, db, &WidgetMember{Name: "Chester"})
			m2 := create(t, db, &WidgetMember{Name: "Mike"})
			if err := orm.Many[WidgetMember](db, band, "members").Add(ctx, m1, m2); err != nil {
				t.Fatalf("add members: %v", err)
			}
			_, f := modelField(t, db, "admin_widgets.WidgetBand", "members")
			w := admin.NewManyToManyRawIDWidget(env.site, f)
			lookup := `<a href="` + mustReverse(t, env.router, "admin:admin_widgets_widgetmember_changelist") +
				`" class="related-lookup" id="lookup_id_test" title="Lookup"></a></div>`
			djangotest.AssertHTMLEqual(t, w.Render("test", []int64{m1.ID, m2.ID}, forms.Attrs{}), fmt.Sprintf(
				`<div><input type="text" name="test" value="%d,%d" class="vManyToManyRawIdAdminField">`,
				m1.ID, m2.ID)+lookup)
			djangotest.AssertHTMLEqual(t, w.Render("test", []int64{m1.ID}, nil), fmt.Sprintf(
				`<div><input type="text" name="test" value="%d" class="vManyToManyRawIdAdminField">`, m1.ID)+lookup)
		})

		t.Run("test_m2m_related_model_not_in_admin", func(t *testing.T) {
			// M2M relationship with model not registered with admin site.
			// Raw ID widget should have no magnifying glass link. See #16542
			consultor1 := create(t, db, &Advisor{Name: "Rockstar Techie"})
			c1 := create(t, db, &Company{Name: "Doodle"})
			c2 := create(t, db, &Company{Name: "Pear"})
			if err := orm.Many[Company](db, consultor1, "companies").Add(ctx, c1, c2); err != nil {
				t.Fatalf("add companies: %v", err)
			}
			_, f := modelField(t, db, "admin_widgets.Advisor", "companies")
			w := admin.NewManyToManyRawIDWidget(env.site, f)
			djangotest.AssertHTMLEqual(t, w.Render("company_widget1", []int64{c1.ID, c2.ID}, forms.Attrs{}),
				fmt.Sprintf(`<input type="text" name="company_widget1" value="%d,%d">`, c1.ID, c2.ID))
			djangotest.AssertHTMLEqual(t, w.Render("company_widget2", []int64{c1.ID}, nil),
				fmt.Sprintf(`<input type="text" name="company_widget2" value="%d">`, c1.ID))
		})
	})
}

// invalidChoice is the message of Django's ModelChoiceField for a key that
// names no row.
const invalidChoice = "Select a valid choice. That choice is not one of the available choices."

// Ported from Django: tests/admin_widgets/tests.py::AdminForeignKeyRawIdWidget.test_nonexistent_target_id
// Ported from Django: tests/admin_widgets/tests.py::AdminForeignKeyRawIdWidget.test_invalid_target_id
// Ported from Django: tests/admin_widgets/tests.py::AdminForeignKeyRawIdWidget.test_label_and_url_for_value_invalid_uuid
func TestAdminForeignKeyRawIDWidget(t *testing.T) {
	djangotest.EachDB(t, widgetRelatedModels(), func(t *testing.T, db *orm.DB) {
		// FIXME-PORT: test_url_params_from_lookup_dict_any_iterable and
		// test_url_params_from_lookup_dict_callable test the
		// limit_choices_to parameters of the lookup link, which the Go ORM
		// does not have.
		ctx := context.Background()
		env := widgetEnv(t, db, admin.ModelAdmin{})
		env.client.ForceLogin(createTestUser(t, db, "super", "secret", true, true))
		addURL := mustReverse(t, env.router, "admin:admin_widgets_widgetevent_add")

		t.Run("test_nonexistent_target_id", func(t *testing.T) {
			band := create(t, db, &WidgetBand{Name: "Bogey Blues"})
			pk := band.ID
			if _, err := orm.Query[WidgetBand](db).Filter("id", pk).Delete(ctx); err != nil {
				t.Fatalf("delete band: %v", err)
			}
			// Try posting with a nonexistent pk in a raw id field: this
			// should result in an error message, not a server exception.
			resp := env.client.Post(addURL, url.Values{"main_band": {strconv.FormatInt(pk, 10)}})
			djangotest.AssertContains(t, resp, invalidChoice)
		})

		t.Run("test_invalid_target_id", func(t *testing.T) {
			for _, testStr := range []string{"Iñtërnâtiônàlizætiøn", "1234'", "-1234"} {
				// This should result in an error message, not a server
				// exception.
				resp := env.client.Post(addURL, url.Values{"main_band": {testStr}})
				djangotest.AssertContains(t, resp, invalidChoice)
			}
		})

		t.Run("test_label_and_url_for_value_invalid_uuid", func(t *testing.T) {
			// Django's Honeycomb has a UUID key and the widget is asked for
			// the label of a value that is no UUID. The Go Honeycomb has an
			// integer key, so the value is no integer.
			_, f := modelField(t, db, "admin_widgets.Bee", "Honeycomb")
			w := admin.NewForeignKeyRawIDWidget(ctx, env.site, f)
			label, link := w.Lookup("invalid-uuid")
			djangotest.AssertEqual(t, []string{label, link}, []string{"", ""})
		})
	})
}

// The Django tests ask formfield_for_dbfield for the widget; the Go admin has
// no such hook, so these tests read the widget off the rendered add page.
//
// Ported from Django: tests/admin_widgets/tests.py::AdminFormfieldForDBFieldTests.test_raw_id_ForeignKey
// Ported from Django: tests/admin_widgets/tests.py::AdminFormfieldForDBFieldTests.test_raw_id_many_to_many
func TestAdminFormfieldForDBFieldRawID(t *testing.T) {
	djangotest.EachDB(t, widgetRelatedModels(), func(t *testing.T, db *orm.DB) {
		env := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
			admin.Register[WidgetBand](site, admin.ModelAdmin{RawIDFields: []string{"members"}})
			admin.Register[WidgetMember](site, admin.ModelAdmin{})
			admin.Register[WidgetEvent](site, admin.ModelAdmin{RawIDFields: []string{"main_band"}})
		})
		env.client.ForceLogin(createTestUser(t, db, "super", "secret", true, true))

		t.Run("test_raw_id_ForeignKey", func(t *testing.T) {
			resp := env.client.Get(mustReverse(t, env.router, "admin:admin_widgets_widgetevent_add"))
			djangotest.AssertContains(t, resp,
				`<input type="text" name="main_band" class="vForeignKeyRawIdAdminField" required id="id_main_band">`,
				djangotest.HTML())
		})

		t.Run("test_raw_id_many_to_many", func(t *testing.T) {
			resp := env.client.Get(mustReverse(t, env.router, "admin:admin_widgets_widgetband_add"))
			djangotest.AssertContains(t, resp,
				`<input type="text" name="members" value="" class="vManyToManyRawIdAdminField" required id="id_members">`,
				djangotest.HTML())
		})
	})
}

// No Django test saves a form through raw-id fields and reads the label back
// on the change page; this Go test does.
func TestRawIDFieldsSaveAndShowLabel(t *testing.T) {
	djangotest.EachDB(t, widgetRelatedModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		env := widgetEnv(t, db, admin.ModelAdmin{})
		env.client.ForceLogin(createTestUser(t, db, "super", "secret", true, true))
		main := create(t, db, &WidgetBand{Name: "Linkin Park"})
		b1 := create(t, db, &WidgetBand{Name: "Support One"})
		b2 := create(t, db, &WidgetBand{Name: "Support Two"})

		resp := env.client.Post(mustReverse(t, env.router, "admin:admin_widgets_widgetevent_add"), url.Values{
			"main_band":        {strconv.FormatInt(main.ID, 10)},
			"supporting_bands": {fmt.Sprintf("%d,%d", b1.ID, b2.ID)},
		})
		djangotest.AssertRedirects(t, resp, mustReverse(t, env.router, "admin:admin_widgets_widgetevent_changelist"))
		event, err := orm.Query[WidgetEvent](db).First(ctx)
		if err != nil {
			t.Fatalf("load event: %v", err)
		}
		djangotest.AssertEqual(t, event.MainBand.PK(), main.ID)
		supporting, err := orm.Many[WidgetBand](db, &event, "supporting_bands").OrderBy("id").All(ctx)
		if err != nil {
			t.Fatalf("load supporting bands: %v", err)
		}
		names := make([]string, 0, len(supporting))
		for _, b := range supporting {
			names = append(names, b.Name)
		}
		djangotest.AssertEqual(t, names, []string{"Support One", "Support Two"})

		resp = env.client.Get(mustReverse(t, env.router, "admin:admin_widgets_widgetevent_change", event.ID))
		djangotest.AssertContains(t, resp, `<strong><a href="`+
			mustReverse(t, env.router, "admin:admin_widgets_widgetband_change", main.ID)+
			`">Linkin Park</a></strong>`, djangotest.HTML())
		djangotest.AssertContains(t, resp, fmt.Sprintf(`value="%d,%d"`, b1.ID, b2.ID))

		// A key of a band that does not exist fails on the many-to-many
		// field too.
		resp = env.client.Post(mustReverse(t, env.router, "admin:admin_widgets_widgetevent_add"), url.Values{
			"main_band":        {strconv.FormatInt(main.ID, 10)},
			"supporting_bands": {fmt.Sprintf("%d,%d", b1.ID, b2.ID+100)},
		})
		djangotest.AssertContains(t, resp, fmt.Sprintf(
			"Select a valid choice. %d is not one of the available choices.", b2.ID+100))
	})
}

// autocompleteSelectHTML is the <select> of Django's AutocompleteSelect for
// WidgetAlbum.band named name on a site at /admin/, without its options.
func autocompleteSelectHTML(name, class, allowClear, field, multiple string) string {
	return `<select name="` + name + `"` + multiple + ` class="` + class + `" data-ajax--cache="true" ` +
		`data-ajax--delay="250" data-ajax--type="GET" data-ajax--url="/admin/autocomplete/" ` +
		`data-theme="admin-autocomplete" data-allow-clear="` + allowClear + `" ` +
		`data-app-label="admin_widgets" data-field-name="` + field + `" data-model-name="widgetalbum" ` +
		`data-placeholder="" lang="en">`
}

// Django's AlbumForm, NotRequiredBandForm and RequiredBandForm set
// widget.is_required from the form field. A Go widget has no is_required that
// the form sets, so the tests set IsRequired as the admin does from the
// field; TestAutocompleteAdminForm covers the admin.
//
// Ported from Django: tests/admin_widgets/test_autocomplete_widget.py::AutocompleteMixinTests.test_build_attrs
// Ported from Django: tests/admin_widgets/test_autocomplete_widget.py::AutocompleteMixinTests.test_build_attrs_no_custom_class
// Ported from Django: tests/admin_widgets/test_autocomplete_widget.py::AutocompleteMixinTests.test_build_attrs_not_required_field
// Ported from Django: tests/admin_widgets/test_autocomplete_widget.py::AutocompleteMixinTests.test_build_attrs_required_field
// Ported from Django: tests/admin_widgets/test_autocomplete_widget.py::AutocompleteMixinTests.test_get_url
// Ported from Django: tests/admin_widgets/test_autocomplete_widget.py::AutocompleteMixinTests.test_render_options_required_field
// Ported from Django: tests/admin_widgets/test_autocomplete_widget.py::AutocompleteMixinTests.test_render_options_not_required_field
// Ported from Django: tests/admin_widgets/test_autocomplete_widget.py::AutocompleteMixinTests.test_media
func TestAutocompleteMixin(t *testing.T) {
	djangotest.EachDB(t, widgetRelatedModels(), func(t *testing.T, db *orm.DB) {
		// FIXME-PORT: test_render_options_fk_as_pk needs a foreign key as
		// the primary key, which the Go ORM does not have. test_media checks
		// English only: the Go admin has no translations, so it always loads
		// Select2's English messages.
		env := widgetEnv(t, db, admin.ModelAdmin{})
		album, band := modelField(t, db, "admin_widgets.WidgetAlbum", "Band")
		_, featuring := modelField(t, db, "admin_widgets.WidgetAlbum", "featuring")
		const emptyOption = `<option value=""></option>`

		t.Run("test_build_attrs", func(t *testing.T) {
			w := admin.NewAutocompleteSelect(env.site, album, band, forms.Attrs{"class": "my-class"})
			w.IsRequired = true
			djangotest.AssertHTMLEqual(t, w.Render("my_field", nil, forms.Attrs{}),
				autocompleteSelectHTML("my_field", "my-class admin-autocomplete", "false", "band", "")+"</select>")
		})

		t.Run("test_build_attrs_no_custom_class", func(t *testing.T) {
			w := admin.NewAutocompleteSelectMultiple(env.site, album, featuring, nil)
			w.IsRequired = true
			djangotest.AssertHTMLEqual(t, w.Render("name", nil, forms.Attrs{}),
				autocompleteSelectHTML("name", "admin-autocomplete", "false", "featuring", " multiple")+"</select>")
		})

		t.Run("test_build_attrs_not_required_field", func(t *testing.T) {
			w := admin.NewAutocompleteSelect(env.site, album, band, nil)
			djangotest.AssertContains(t, &djangotest.Response{StatusCode: 200, Body: []byte(w.Render("band", nil, nil))},
				`data-allow-clear="true"`)
		})

		t.Run("test_build_attrs_required_field", func(t *testing.T) {
			w := admin.NewAutocompleteSelect(env.site, album, band, nil)
			w.IsRequired = true
			djangotest.AssertContains(t, &djangotest.Response{StatusCode: 200, Body: []byte(w.Render("band", nil, nil))},
				`data-allow-clear="false"`)
		})

		t.Run("test_get_url", func(t *testing.T) {
			w := admin.NewAutocompleteSelect(env.site, album, band, nil)
			djangotest.AssertEqual(t, w.URL, mustReverse(t, env.router, "admin:autocomplete"))
		})

		t.Run("test_render_options_required_field", func(t *testing.T) {
			// Empty option is present if the field isn't required.
			w := admin.NewAutocompleteSelect(env.site, album, band, nil)
			if out := w.Render("band", nil, nil); !strings.Contains(out, emptyOption) {
				t.Errorf("Render() = %s, want the empty option", out)
			}
		})

		t.Run("test_media", func(t *testing.T) {
			w := admin.NewAutocompleteSelect(env.site, album, band, nil)
			djangotest.AssertEqual(t, w.Media().JS(), []string{
				"admin/js/vendor/jquery/jquery.min.js",
				"admin/js/vendor/select2/select2.full.min.js",
				"admin/js/vendor/select2/i18n/en.js",
				"admin/js/jquery.init.js",
				"admin/js/autocomplete.js",
			})
		})

		t.Run("test_render_options_not_required_field", func(t *testing.T) {
			// Empty option isn't present if the field is required.
			w := admin.NewAutocompleteSelect(env.site, album, band, nil)
			w.IsRequired = true
			if out := w.Render("band", nil, nil); strings.Contains(out, emptyOption) {
				t.Errorf("Render() = %s, want no empty option", out)
			}
		})
	})
}

// Django's test renders an AlbumForm with initial values; the Go test reads
// the same options off the change page of an album with that band and those
// featured bands.
//
// Ported from Django: tests/admin_widgets/test_autocomplete_widget.py::AutocompleteMixinTests.test_render_options
func TestAutocompleteAdminForm(t *testing.T) {
	djangotest.EachDB(t, widgetRelatedModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		env := widgetEnv(t, db, admin.ModelAdmin{AutocompleteFields: []string{"band", "featuring"}})
		env.client.ForceLogin(createTestUser(t, db, "super", "secret", true, true))
		beatles := create(t, db, &WidgetBand{Name: "The Beatles", Style: "rock"})
		who := create(t, db, &WidgetBand{Name: "The Who", Style: "rock"})
		album := &WidgetAlbum{Name: "Rubber Soul"}
		album.Band.SetPK(beatles.ID)
		create(t, db, album)
		if err := orm.Many[WidgetBand](db, album, "featuring").Add(ctx, beatles, who); err != nil {
			t.Fatalf("add featuring: %v", err)
		}

		resp := env.client.Get(mustReverse(t, env.router, "admin:admin_widgets_widgetalbum_change", album.ID))
		// With 'band', a ForeignKey: only the selected band, and no empty
		// option because the field is required.
		djangotest.AssertContains(t, resp,
			autocompleteSelectHTML("band", "admin-autocomplete", "false", "band", "")[:len(`<select name="band"`)])
		djangotest.AssertContains(t, resp,
			fmt.Sprintf(`<option value="%d" selected>The Beatles</option>`, beatles.ID), djangotest.HTML(), djangotest.Count(2))
		djangotest.AssertNotContains(t, resp, fmt.Sprintf(`<option value="%d">The Who</option>`, who.ID), djangotest.HTML())
		// With 'featuring', a ManyToManyField: both selected bands.
		djangotest.AssertContains(t, resp,
			fmt.Sprintf(`<option value="%d" selected>The Who</option>`, who.ID), djangotest.HTML())
		djangotest.AssertNotContains(t, resp, `<option value=""></option>`, djangotest.HTML())
		djangotest.AssertContains(t, resp, `data-allow-clear="false"`, djangotest.Count(2))
	})
}

// Question stands in for tests/admin_views/models.py::Question, with an
// integer primary key in place of big_id and without the uuid field.
type Question struct {
	ID               int64
	Question         string            `orm:"max_length=20"`
	Posted           orm.Date          `orm:"null"`
	Expires          *time.Time        `orm:"null;blank"`
	RelatedQuestions orm.M2M[Question] `orm:"related_name=+"`
}

// String returns the question text.
func (q Question) String() string { return q.Question }

// Answer stands in for tests/admin_views/models.py::Answer, without
// question_with_to_field and related_answers.
type Answer struct {
	ID       int64
	Question orm.FK[Question] `orm:"on_delete=protect"`
	Answer   string           `orm:"max_length=20"`
}

// String returns the answer text.
func (a Answer) String() string { return a.Answer }

// autocompleteViewModels returns the model set of the ported
// AutocompleteJsonViewTests.
func autocompleteViewModels() []any {
	return []any{auth.App{}, contenttypes.App{}, admin.App{}, djangotest.App("admin_views", &Question{}, &Answer{})}
}

// questionAdmin is the QuestionAdmin of tests/admin_views/admin.py.
func questionAdmin() admin.ModelAdmin {
	return admin.ModelAdmin{
		Ordering:           []string{"-posted"},
		SearchFields:       []string{"question"},
		AutocompleteFields: []string{"related_questions"},
	}
}

// autocompleteEnv registers Question under question and Answer with the
// AnswerAdmin of tests/admin_views/admin.py on a fresh site.
func autocompleteEnv(t *testing.T, db *orm.DB, question admin.ModelAdmin) *adminTestEnv {
	t.Helper()
	return newAdminTestEnv(t, db, func(site *admin.AdminSite) {
		admin.Register[Question](site, question)
		admin.Register[Answer](site, admin.ModelAdmin{AutocompleteFields: []string{"question"}})
	})
}

// autocompleteResults is the JSON body of the autocomplete view.
type autocompleteResults struct {
	Results []struct {
		ID   string `json:"id"`
		Text string `json:"text"`
	} `json:"results"`
	Pagination struct {
		More bool `json:"more"`
	} `json:"pagination"`
}

// decodeResults decodes the body of an autocomplete view response.
func decodeResults(t *testing.T, resp *djangotest.Response) autocompleteResults {
	t.Helper()
	var data autocompleteResults
	if err := json.Unmarshal(resp.Body, &data); err != nil {
		t.Fatalf("decode %s: %v", resp.Body, err)
	}
	return data
}

// autocompleteGet requests the autocomplete view of env for the field
// field_name of Answer with the extra parameters given.
func autocompleteGet(t *testing.T, env *adminTestEnv, field string, extra url.Values) *djangotest.Response {
	t.Helper()
	q := url.Values{"app_label": {"admin_views"}, "model_name": {"answer"}, "field_name": {field}}
	for k, v := range extra {
		q[k] = v
	}
	return env.client.Get(mustReverse(t, env.router, "admin:autocomplete") + "?" + q.Encode())
}

// Ported from Django: tests/admin_views/test_autocomplete_view.py::AutocompleteJsonViewTests.test_success
// Ported from Django: tests/admin_views/test_autocomplete_view.py::AutocompleteJsonViewTests.test_field_does_not_exist
// Ported from Django: tests/admin_views/test_autocomplete_view.py::AutocompleteJsonViewTests.test_field_no_related_field
// Ported from Django: tests/admin_views/test_autocomplete_view.py::AutocompleteJsonViewTests.test_field_does_not_allowed
// Ported from Django: tests/admin_views/test_autocomplete_view.py::AutocompleteJsonViewTests.test_must_be_logged_in
// Ported from Django: tests/admin_views/test_autocomplete_view.py::AutocompleteJsonViewTests.test_has_view_or_change_permission_required
// Ported from Django: tests/admin_views/test_autocomplete_view.py::AutocompleteJsonViewTests.test_search_use_distinct
// Ported from Django: tests/admin_views/test_autocomplete_view.py::AutocompleteJsonViewTests.test_missing_search_fields
// Ported from Django: tests/admin_views/test_autocomplete_view.py::AutocompleteJsonViewTests.test_get_paginator
func TestAutocompleteJSONView(t *testing.T) {
	djangotest.EachDB(t, autocompleteViewModels(), func(t *testing.T, db *orm.DB) {
		// FIXME-PORT: test_custom_to_field, test_custom_to_field_permission_denied,
		// test_custom_to_field_custom_pk, test_to_field_resolution_with_mti and
		// test_to_field_resolution_with_fk_pk need foreign keys with
		// to_field, test_limit_choices_to needs limit_choices_to, and
		// test_serialize_result a subclass of the view; the Go ORM and admin
		// have none of them.
		ctx := context.Background()
		superuser := createTestUser(t, db, "super", "secret", true, true)

		t.Run("test_success", func(t *testing.T) {
			env := autocompleteEnv(t, db, questionAdmin())
			env.client.ForceLogin(superuser)
			q := create(t, db, &Question{Question: "Is this a question?"})
			resp := autocompleteGet(t, env, "question", url.Values{"term": {"is"}})
			djangotest.AssertStatus(t, resp, 200)
			djangotest.AssertJSONEqual(t, string(resp.Body), map[string]any{
				"results":    []any{map[string]any{"id": strconv.FormatInt(q.ID, 10), "text": q.Question}},
				"pagination": map[string]any{"more": false},
			})
			if _, err := orm.Query[Question](db).Filter("id", q.ID).Delete(ctx); err != nil {
				t.Fatalf("delete: %v", err)
			}
		})

		env := autocompleteEnv(t, db, questionAdmin())
		env.client.ForceLogin(superuser)
		for name, field := range map[string]string{
			"test_field_does_not_exist":   "does_not_exist",
			"test_field_no_related_field": "answer",
			"test_field_does_not_allowed": "related_questions",
		} {
			t.Run(name, func(t *testing.T) {
				djangotest.AssertStatus(t, autocompleteGet(t, env, field, url.Values{"term": {"is"}}), 403)
			})
		}

		t.Run("test_must_be_logged_in", func(t *testing.T) {
			env := autocompleteEnv(t, db, questionAdmin())
			env.client.ForceLogin(superuser)
			djangotest.AssertStatus(t, autocompleteGet(t, env, "question", url.Values{"term": {""}}), 200)
			env.client.Logout()
			djangotest.AssertStatus(t, autocompleteGet(t, env, "question", url.Values{"term": {""}}), 302)
		})

		t.Run("test_has_view_or_change_permission_required", func(t *testing.T) {
			// Users require the change permission for the related model to
			// the autocomplete view for it.
			env := autocompleteEnv(t, db, questionAdmin())
			user := createTestUser(t, db, "user", "secret", true, false)
			env.client.ForceLogin(user)
			djangotest.AssertStatus(t, autocompleteGet(t, env, "question", url.Values{"term": {"is"}}), 403)
			for _, permission := range []string{"view", "change"} {
				t.Run(permission, func(t *testing.T) {
					u := createTestUser(t, db, "user_"+permission, "secret", true, false)
					grantPerm(t, db, u, "admin_views", permission+"_question")
					env.client.ForceLogin(u)
					djangotest.AssertStatus(t, autocompleteGet(t, env, "question", url.Values{"term": {"is"}}), 200)
				})
			}
		})

		t.Run("test_search_use_distinct", func(t *testing.T) {
			// Searching across model relations use QuerySet.distinct() to
			// avoid duplicates.
			ma := questionAdmin()
			ma.SearchFields = []string{"related_questions__question", "question"}
			env := autocompleteEnv(t, db, ma)
			env.client.ForceLogin(superuser)
			q1 := create(t, db, &Question{Question: "question 1"})
			q2 := create(t, db, &Question{Question: "question 2"})
			if err := orm.Many[Question](db, q2, "related_questions").Add(ctx, q1); err != nil {
				t.Fatalf("add: %v", err)
			}
			q3 := create(t, db, &Question{Question: "question 3"})
			if err := orm.Many[Question](db, q3, "related_questions").Add(ctx, q1); err != nil {
				t.Fatalf("add: %v", err)
			}
			resp := autocompleteGet(t, env, "question", url.Values{"term": {"question"}})
			djangotest.AssertStatus(t, resp, 200)
			djangotest.AssertEqual(t, len(decodeResults(t, resp).Results), 3)
			if _, err := orm.Query[Question](db).Delete(ctx); err != nil {
				t.Fatalf("delete: %v", err)
			}
		})

		t.Run("test_missing_search_fields", func(t *testing.T) {
			ma := questionAdmin()
			ma.SearchFields = nil
			env := autocompleteEnv(t, db, ma)
			env.client.ForceLogin(superuser)
			resp := autocompleteGet(t, env, "question", url.Values{"term": {""}})
			djangotest.AssertContains(t, resp,
				"ModelAdmin of admin_views.Question must have search_fields for the autocomplete_view.",
				djangotest.StatusCode(404))
		})

		t.Run("test_get_paginator", func(t *testing.T) {
			// Search results are paginated.
			ma := questionAdmin()
			ma.Ordering = []string{"pk"}
			env := autocompleteEnv(t, db, ma)
			env.client.ForceLogin(superuser)
			const paginatorSize = 20
			want := make([]string, 0, paginatorSize+10)
			for i := range paginatorSize + 10 {
				q := create(t, db, &Question{Question: strconv.Itoa(i)})
				want = append(want, strconv.FormatInt(q.ID, 10)+":"+q.Question)
			}
			page := func(params url.Values) ([]string, bool) {
				resp := autocompleteGet(t, env, "question", params)
				djangotest.AssertStatus(t, resp, 200)
				data := decodeResults(t, resp)
				got := make([]string, 0, len(data.Results))
				for _, r := range data.Results {
					got = append(got, r.ID+":"+r.Text)
				}
				return got, data.Pagination.More
			}
			// The first page of results.
			got, more := page(url.Values{"term": {""}})
			djangotest.AssertEqual(t, got, want[:paginatorSize])
			djangotest.AssertEqual(t, more, true)
			// The second page of results.
			got, more = page(url.Values{"term": {""}, "page": {"2"}})
			djangotest.AssertEqual(t, got, want[paginatorSize:])
			djangotest.AssertEqual(t, more, false)

			// No Django test asks for the last page or a page that does not
			// exist; BaseListView answers them as below.
			got, _ = page(url.Values{"term": {""}, "page": {"last"}})
			djangotest.AssertEqual(t, got, want[paginatorSize:])
			djangotest.AssertStatus(t, autocompleteGet(t, env, "question", url.Values{"page": {"3"}}), 404)
			djangotest.AssertStatus(t, autocompleteGet(t, env, "question", url.Values{"page": {"x"}}), 404)
		})
	})
}

// Django's test reads the widget class off the form of get_form; the Go test
// reads the select of the add page.
//
// Ported from Django: tests/modeladmin/tests.py::ModelAdminTests.test_get_autocomplete_fields
func TestModelAdminGetAutocompleteFields(t *testing.T) {
	djangotest.EachDB(t, modeladminCheckModels(), func(t *testing.T, db *orm.DB) {
		superuser := createTestUser(t, db, "super", "secret", true, true)
		songAdmin := admin.ModelAdmin{AutocompleteFields: []string{"featuring"}, Fields: []any{"featuring", "band"}}
		songPage := func(ma admin.ModelAdmin) string {
			env := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
				admin.Register[Band](site, admin.ModelAdmin{SearchFields: []string{"name"}})
				admin.Register[Song](site, ma)
			})
			env.client.ForceLogin(superuser)
			resp := env.client.Get(mustReverse(t, env.router, "admin:modeladmin_song_add"))
			djangotest.AssertStatus(t, resp, 200)
			return string(resp.Body)
		}
		autocomplete := func(page, name string) bool {
			i := strings.Index(page, `<select name="`+name+`"`)
			if i < 0 {
				t.Fatalf("page has no select %q", name)
			}
			end := strings.Index(page[i:], ">")
			return strings.Contains(page[i:i+end], "admin-autocomplete")
		}

		// Uses autocomplete_fields if not overridden.
		page := songPage(songAdmin)
		djangotest.AssertEqual(t, autocomplete(page, "featuring"), true)
		djangotest.AssertEqual(t, autocomplete(page, "band"), false)
		// Uses overridden get_autocomplete_fields
		other := songAdmin
		other.GetAutocompleteFields = func(*http.Request) []string { return []string{"band"} }
		page = songPage(other)
		djangotest.AssertEqual(t, autocomplete(page, "featuring"), false)
		djangotest.AssertEqual(t, autocomplete(page, "band"), true)
	})
}
