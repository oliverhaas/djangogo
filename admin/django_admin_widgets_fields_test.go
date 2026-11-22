package admin_test

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
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

// The Django tests ask formfield_for_dbfield for the form field of a model
// field; the Go admin builds its forms through a form field callback that is
// not exported, so these tests read the field off the add page and its
// context form.
//
// FIXME-PORT: test_FileField, test_inheritance and
// test_formfield_overrides_for_custom_field need file fields.
//
// Ported from Django: tests/admin_widgets/tests.py::AdminFormfieldForDBFieldTests.test_DateField
// Ported from Django: tests/admin_widgets/tests.py::AdminFormfieldForDBFieldTests.test_DateTimeField
// Ported from Django: tests/admin_widgets/tests.py::AdminFormfieldForDBFieldTests.test_TimeField
// Ported from Django: tests/admin_widgets/tests.py::AdminFormfieldForDBFieldTests.test_TextField
// Ported from Django: tests/admin_widgets/tests.py::AdminFormfieldForDBFieldTests.test_URLField
// Ported from Django: tests/admin_widgets/tests.py::AdminFormfieldForDBFieldTests.test_IntegerField
// Ported from Django: tests/admin_widgets/tests.py::AdminFormfieldForDBFieldTests.test_CharField
// Ported from Django: tests/admin_widgets/tests.py::AdminFormfieldForDBFieldTests.test_EmailField
// Ported from Django: tests/admin_widgets/tests.py::AdminFormfieldForDBFieldTests.test_ForeignKey
// Ported from Django: tests/admin_widgets/tests.py::AdminFormfieldForDBFieldTests.test_field_with_choices
// Ported from Django: tests/admin_widgets/tests.py::AdminFormfieldForDBFieldTests.test_formfield_overrides
// Ported from Django: tests/admin_widgets/tests.py::AdminFormfieldForDBFieldTests.test_formfield_overrides_widget_instances
// Ported from Django: tests/admin_widgets/tests.py::AdminFormfieldForDBFieldTests.test_formfield_overrides_m2m_filter_widget
// Ported from Django: tests/admin_widgets/tests.py::AdminFormfieldForDBFieldTests.test_formfield_overrides_for_datetime_field
// Ported from Django: tests/admin_widgets/tests.py::AdminFormfieldForDBFieldTests.test_radio_fields_ForeignKey
// Ported from Django: tests/admin_widgets/tests.py::AdminFormfieldForDBFieldTests.test_radio_fields_foreignkey_formfield_overrides_empty_label
// Ported from Django: tests/admin_widgets/tests.py::AdminFormfieldForDBFieldTests.test_choices_with_radio_fields
func TestAdminFormfieldForDBField(t *testing.T) {
	djangotest.EachDB(t, widgetRelatedModels(), func(t *testing.T, db *orm.DB) {
		super := createTestUser(t, db, "super", "secret", true, true)
		// addPage returns the add page of model, with each model under its
		// admin in admins.
		addPage := func(t *testing.T, model string, admins map[string]admin.ModelAdmin) *djangotest.Response {
			t.Helper()
			env := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
				admin.Register[WidgetBand](site, admins["widgetband"])
				admin.Register[WidgetMember](site, admins["widgetmember"])
				admin.Register[WidgetEvent](site, admins["widgetevent"])
				admin.Register[WidgetInventory](site, admins["widgetinventory"])
			})
			env.client.ForceLogin(super)
			resp := env.client.Get(mustReverse(t, env.router, "admin:admin_widgets_"+model+"_add"))
			djangotest.AssertStatus(t, resp, 200)
			return resp
		}
		event := func(t *testing.T) *djangotest.Response {
			t.Helper()
			return addPage(t, "widgetevent", nil)
		}
		member := func(t *testing.T) *djangotest.Response {
			t.Helper()
			return addPage(t, "widgetmember", nil)
		}

		t.Run("test_DateField", func(t *testing.T) {
			resp := event(t)
			djangotest.AssertContains(t, resp,
				`<p class="date"><input type="text" name="start_date" class="vDateField" size="10" `+
					`aria-describedby="id_start_date_timezone_warning_helptext" id="id_start_date"></p>`,
				djangotest.HTML())
			djangotest.AssertContains(t, resp, `<script src="/static/admin/js/calendar.js"></script>`)
			djangotest.AssertContains(t, resp, `<script src="/static/admin/js/admin/DateTimeShortcuts.js"></script>`)
		})
		t.Run("test_DateTimeField", func(t *testing.T) {
			djangotest.AssertContains(t, member(t),
				`<p class="datetime"><label for="id_birthdate_0">Date:</label> `+
					`<input type="text" name="birthdate_0" class="vDateField" size="10" `+
					`aria-describedby="id_birthdate_timezone_warning_helptext" id="id_birthdate_0"><br>`+
					`<label for="id_birthdate_1">Time:</label> `+
					`<input type="text" name="birthdate_1" class="vTimeField" size="8" `+
					`aria-describedby="id_birthdate_timezone_warning_helptext" id="id_birthdate_1"></p>`,
				djangotest.HTML())
		})
		t.Run("test_TimeField", func(t *testing.T) {
			djangotest.AssertContains(t, event(t),
				`<p class="time"><input type="text" name="start_time" class="vTimeField" size="8" `+
					`aria-describedby="id_start_time_timezone_warning_helptext" id="id_start_time"></p>`,
				djangotest.HTML())
		})
		t.Run("test_TextField", func(t *testing.T) {
			djangotest.AssertContains(t, event(t),
				`<textarea name="description" cols="40" rows="10" class="vLargeTextField" id="id_description"></textarea>`,
				djangotest.HTML())
		})
		t.Run("test_URLField", func(t *testing.T) {
			djangotest.AssertContains(t, event(t),
				`<input type="url" name="link" class="vURLField" maxlength="200" id="id_link">`,
				djangotest.HTML())
		})
		t.Run("test_IntegerField", func(t *testing.T) {
			djangotest.AssertContains(t, event(t),
				`<input type="number" name="min_age" class="vIntegerField" id="id_min_age">`,
				djangotest.HTML())
		})
		t.Run("test_CharField", func(t *testing.T) {
			djangotest.AssertContains(t, member(t),
				`<input type="text" name="name" class="vTextField" maxlength="100" required id="id_name">`,
				djangotest.HTML())
		})
		t.Run("test_EmailField", func(t *testing.T) {
			djangotest.AssertContains(t, member(t),
				`<input type="email" name="email" class="vTextField" maxlength="254" id="id_email">`,
				djangotest.HTML())
		})
		t.Run("test_ForeignKey", func(t *testing.T) {
			djangotest.AssertContains(t, event(t),
				`<select name="main_band" required id="id_main_band" data-context="available-source">`+
					`<option value="" selected>---------</option></select>`,
				djangotest.HTML())
		})
		t.Run("test_field_with_choices", func(t *testing.T) {
			djangotest.AssertContains(t, member(t),
				`<select name="gender" id="id_gender"><option value="" selected>---------</option>`+
					`<option value="M">Male</option><option value="F">Female</option></select>`,
				djangotest.HTML())
		})
		t.Run("test_formfield_overrides", func(t *testing.T) {
			resp := addPage(t, "widgetevent", map[string]admin.ModelAdmin{"widgetevent": {
				FormfieldOverrides: map[admin.FieldClass]admin.FormfieldOverride{admin.DateField: {Widget: forms.TextInput{}}},
			}})
			djangotest.AssertContains(t, resp, `<input type="text" name="start_date" id="id_start_date">`, djangotest.HTML())
		})
		// Each field renders its own maximum length next to the shared size.
		t.Run("test_formfield_overrides_widget_instances", func(t *testing.T) {
			resp := addPage(t, "widgetband", map[string]admin.ModelAdmin{"widgetband": {
				FormfieldOverrides: map[admin.FieldClass]admin.FormfieldOverride{
					admin.CharField: {Widget: forms.TextInput{Attrs: forms.Attrs{"size": "10"}}},
				},
			}})
			djangotest.AssertContains(t, resp,
				`<input type="text" name="name" size="10" maxlength="100" required id="id_name">`, djangotest.HTML())
			djangotest.AssertContains(t, resp,
				`<input type="text" name="style" size="10" maxlength="20" required id="id_style">`, djangotest.HTML())
		})
		t.Run("test_formfield_overrides_m2m_filter_widget", func(t *testing.T) {
			resp := addPage(t, "widgetband", map[string]admin.ModelAdmin{"widgetband": {
				FilterVertical: []string{"members"},
				FormfieldOverrides: map[admin.FieldClass]admin.FormfieldOverride{
					admin.ManyToManyField: {Widget: forms.CheckboxSelectMultiple{}},
				},
			}})
			w := formField(t, resp, "members").Base().Widget
			wrapper, ok := w.(admin.RelatedFieldWidgetWrapper)
			if !ok {
				t.Fatalf("members widget = %T, want admin.RelatedFieldWidgetWrapper", w)
			}
			if _, ok := wrapper.Widget.(forms.CheckboxSelectMultiple); !ok {
				t.Errorf("wrapped widget = %T, want forms.CheckboxSelectMultiple", wrapper.Widget)
			}
		})
		t.Run("test_formfield_overrides_for_datetime_field", func(t *testing.T) {
			resp := addPage(t, "widgetmember", map[string]admin.ModelAdmin{"widgetmember": {
				FormfieldOverrides: map[admin.FieldClass]admin.FormfieldOverride{
					admin.DateTimeField: {Widget: admin.NewAdminSplitDateTime(nil)},
				},
			}})
			f := formField(t, resp, "birthdate")
			if _, ok := f.Base().Widget.(admin.AdminSplitDateTime); !ok {
				t.Errorf("birthdate widget = %T, want admin.AdminSplitDateTime", f.Base().Widget)
			}
			if _, ok := f.(*forms.SplitDateTimeField); !ok {
				t.Errorf("birthdate = %T, want *forms.SplitDateTimeField", f)
			}
		})
		// The foreign key cannot be blank, so the buttons have no empty
		// choice: Django's empty_label is None.
		t.Run("test_radio_fields_ForeignKey", func(t *testing.T) {
			band := mustCreate(t, db, &WidgetBand{Name: "Linkin Park", Style: "rock"})
			t.Cleanup(func() {
				if _, err := orm.Query[WidgetBand](db).Filter("id", band.ID).Delete(context.Background()); err != nil {
					t.Error(err)
				}
			})
			resp := addPage(t, "widgetevent", map[string]admin.ModelAdmin{"widgetevent": {
				RadioFields: map[string]admin.RadioStyle{"main_band": admin.Vertical},
			}})
			djangotest.AssertContains(t, resp, fmt.Sprintf(`<div id="id_main_band" class="radiolist"><div>`+
				`<label for="id_main_band_0"><input type="radio" name="main_band" value="%d" class="radiolist" `+
				`required id="id_main_band_0" data-context="available-source"> Linkin Park</label></div></div>`, band.ID),
				djangotest.HTML())
		})
		t.Run("test_radio_fields_foreignkey_formfield_overrides_empty_label", func(t *testing.T) {
			resp := addPage(t, "widgetinventory", map[string]admin.ModelAdmin{"widgetinventory": {
				RadioFields: map[string]admin.RadioStyle{"parent": admin.Vertical},
				FormfieldOverrides: map[admin.FieldClass]admin.FormfieldOverride{
					admin.ForeignKey: {EmptyLabel: new("Custom empty label")},
				},
			}})
			f := formField(t, resp, "parent")
			field, ok := f.(*forms.ModelChoiceField)
			if !ok || field.EmptyLabel == nil {
				t.Fatalf("parent = %#v, want a ModelChoiceField with an empty label", f)
			}
			djangotest.AssertEqual(t, *field.EmptyLabel, "Custom empty label")
		})
		t.Run("test_choices_with_radio_fields", func(t *testing.T) {
			resp := addPage(t, "widgetmember", map[string]admin.ModelAdmin{"widgetmember": {
				RadioFields: map[string]admin.RadioStyle{"gender": admin.Vertical},
			}})
			djangotest.AssertContains(t, resp, `<div id="id_gender" class="radiolist"><div>`+
				`<label for="id_gender_0"><input type="radio" name="gender" value="" class="radiolist" `+
				`id="id_gender_0" checked> None</label></div><div>`+
				`<label for="id_gender_1"><input type="radio" name="gender" value="M" class="radiolist" `+
				`id="id_gender_1"> Male</label></div><div>`+
				`<label for="id_gender_2"><input type="radio" name="gender" value="F" class="radiolist" `+
				`id="id_gender_2"> Female</label></div></div>`,
				djangotest.HTML())
		})
	})
}

// No Django test saves a date and time through the split widget on its own:
// Django's admin_views tests post date_0 and date_1 for an Article, and the Go
// Article has no date.
func TestAdminSplitDateTimeSave(t *testing.T) {
	djangotest.EachDB(t, widgetRelatedModels(), func(t *testing.T, db *orm.DB) {
		super := createTestUser(t, db, "super", "secret", true, true)
		env := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
			admin.Register[WidgetMember](site, admin.ModelAdmin{})
		})
		env.client.ForceLogin(super)
		resp := env.client.Post(mustReverse(t, env.router, "admin:admin_widgets_widgetmember_add"), url.Values{
			"name": {"Ringo"}, "birthdate_0": {"1940-07-07"}, "birthdate_1": {"12:30:00"}, "_save": {"Save"},
		})
		djangotest.AssertStatus(t, resp, http.StatusFound)
		member, err := orm.Query[WidgetMember](db).Get(t.Context(), "name", "Ringo")
		if err != nil {
			t.Fatal(err)
		}
		if want := time.Date(1940, 7, 7, 12, 30, 0, 0, time.UTC); member.Birthdate == nil || !member.Birthdate.Equal(want) {
			t.Errorf("Birthdate = %v, want %v", member.Birthdate, want)
		}
		resp = env.client.Get(mustReverse(t, env.router, "admin:admin_widgets_widgetmember_change", member.ID))
		djangotest.AssertContains(t, resp, `value="1940-07-07"`)
		djangotest.AssertContains(t, resp, `value="12:30:00"`)

		// Only a date is an error of the time.
		resp = env.client.Post(mustReverse(t, env.router, "admin:admin_widgets_widgetmember_add"), url.Values{
			"name": {"Paul"}, "birthdate_0": {"1942-06-18"}, "birthdate_1": {""},
		})
		djangotest.AssertStatus(t, resp, http.StatusOK)
		djangotest.AssertContains(t, resp, "Enter a valid time.")
	})
}

// Ported from Django: tests/admin_widgets/tests.py::AdminDateWidgetTest.test_attrs
func TestAdminDateWidgetAttrs(t *testing.T) {
	w := admin.NewAdminDateWidget(nil)
	djangotest.AssertHTMLEqual(t, w.Render("test", time.Date(2007, 12, 1, 9, 30, 0, 0, time.UTC), nil),
		`<p class="date">`+
			`<input aria-describedby="id_test_timezone_warning_helptext" `+
			`value="2007-12-01" type="text" class="vDateField" name="test" `+
			`size="10"></p>`)
	// pass attrs to widget
	w = admin.NewAdminDateWidget(forms.Attrs{"size": 20, "class": "myDateField"})
	djangotest.AssertHTMLEqual(t, w.Render("test", time.Date(2007, 12, 1, 9, 30, 0, 0, time.UTC), nil),
		`<p class="date">`+
			`<input aria-describedby="id_test_timezone_warning_helptext" `+
			`value="2007-12-01" type="text" class="myDateField" name="test" `+
			`size="20"></p>`)
}

// Ported from Django: tests/admin_widgets/tests.py::AdminTimeWidgetTest.test_attrs
func TestAdminTimeWidgetAttrs(t *testing.T) {
	w := admin.NewAdminTimeWidget(nil)
	djangotest.AssertHTMLEqual(t, w.Render("test", time.Date(2007, 12, 1, 9, 30, 0, 0, time.UTC), nil),
		`<p class="time">`+
			`<input aria-describedby="id_test_timezone_warning_helptext" `+
			`value="09:30:00" type="text" class="vTimeField" name="test" `+
			`size="8"></p>`)
	// pass attrs to widget
	w = admin.NewAdminTimeWidget(forms.Attrs{"size": 20, "class": "myTimeField"})
	djangotest.AssertHTMLEqual(t, w.Render("test", time.Date(2007, 12, 1, 9, 30, 0, 0, time.UTC), nil),
		`<p class="time">`+
			`<input aria-describedby="id_test_timezone_warning_helptext" `+
			`value="09:30:00" type="text" class="myTimeField" name="test" `+
			`size="20"></p>`)
}

// Ported from Django: tests/admin_widgets/tests.py::AdminSplitDateTimeWidgetTest.test_render
//
// FIXME-PORT: tests/admin_widgets/tests.py::AdminSplitDateTimeWidgetTest.test_localization
// needs localized formats and translations.
func TestAdminSplitDateTimeWidgetRender(t *testing.T) {
	w := admin.NewAdminSplitDateTime(nil)
	djangotest.AssertHTMLEqual(t, w.Render("test", time.Date(2007, 12, 1, 9, 30, 0, 0, time.UTC), forms.Attrs{"id": "id_test"}),
		`<p class="datetime">`+
			`<label for="id_test_0">Date:</label> `+
			`<input aria-describedby="id_test_timezone_warning_helptext" `+
			`value="2007-12-01" type="text" class="vDateField" `+
			`name="test_0" size="10" id="id_test_0"><br>`+
			`<label for="id_test_1">Time:</label> `+
			`<input aria-describedby="id_test_timezone_warning_helptext" `+
			`value="09:30:00" type="text" class="vTimeField" `+
			`name="test_1" size="8" id="id_test_1"></p>`)
}

// The Go widget has no get_context; a valid URL is the one the widget shows
// as a "Currently:" link.
//
// Ported from Django: tests/admin_widgets/tests.py::AdminURLWidgetTest.test_get_context_validates_url
func TestAdminURLWidgetValidatesURL(t *testing.T) {
	w := admin.NewAdminURLFieldWidget(nil)
	for _, invalid := range []string{
		"",
		"/not/a/full/url/",
		`javascript:alert("Danger XSS!")`,
		"http://" + strings.Repeat("한.글.", 1_000_000) + "com",
	} {
		if out := w.Render("name", invalid, nil); strings.Contains(out, `<p class="url">`) {
			t.Errorf("Render(%.40q) shows a link: %.200s", invalid, out)
		}
	}
	if out := w.Render("name", "http://example.com", nil); !strings.Contains(out, `<p class="url">`) {
		t.Errorf("Render(http://example.com) shows no link: %s", out)
	}
}

// Ported from Django: tests/admin_widgets/tests.py::AdminURLWidgetTest.test_render
func TestAdminURLWidgetRender(t *testing.T) {
	w := admin.NewAdminURLFieldWidget(nil)
	djangotest.AssertHTMLEqual(t, w.Render("test", "", nil), `<input class="vURLField" name="test" type="url">`)
	djangotest.AssertHTMLEqual(t, w.Render("test", "http://example.com", nil),
		`<p class="url">Currently:<a href="http://example.com">`+
			`http://example.com</a><br>`+
			`Change:<input class="vURLField" name="test" type="url" `+
			`value="http://example.com"></p>`)
}

// Ported from Django: tests/admin_widgets/tests.py::AdminURLWidgetTest.test_render_idn
func TestAdminURLWidgetRenderIDN(t *testing.T) {
	w := admin.NewAdminURLFieldWidget(nil)
	djangotest.AssertHTMLEqual(t, w.Render("test", "http://example-äüö.com", nil),
		`<p class="url">Currently: <a href="http://example-%C3%A4%C3%BC%C3%B6.com">`+
			`http://example-äüö.com</a><br>`+
			`Change:<input class="vURLField" name="test" type="url" `+
			`value="http://example-äüö.com"></p>`)
	// Does not use obsolete IDNA-2003 encoding (#36013).
	if out := w.Render("test", "http://faß.example.com", nil); strings.Contains(out, "fass.example.com") {
		t.Errorf("Render uses IDNA-2003: %s", out)
	}
}

// This test doesn't compare HTML, which would get rid of some escapes that
// are tested here.
//
// Ported from Django: tests/admin_widgets/tests.py::AdminURLWidgetTest.test_render_quoting
func TestAdminURLWidgetRenderQuoting(t *testing.T) {
	hrefRE := regexp.MustCompile(`href="([^"]+)"`)
	valueRE := regexp.MustCompile(`value="([^"]+)"`)
	textRE := regexp.MustCompile(`<a[^>]+>([^>]+)</a>`)
	w := admin.NewAdminURLFieldWidget(nil)
	check := func(value, href, text, val string) {
		t.Helper()
		output := w.Render("test", value, nil)
		djangotest.AssertEqual(t, hrefRE.FindStringSubmatch(output)[1], href)
		djangotest.AssertEqual(t, textRE.FindStringSubmatch(output)[1], text)
		djangotest.AssertEqual(t, valueRE.FindStringSubmatch(output)[1], val)
	}
	check("http://example.com/<sometag>some-text</sometag>",
		"http://example.com/%3Csometag%3Esome-text%3C/sometag%3E",
		"http://example.com/&lt;sometag&gt;some-text&lt;/sometag&gt;",
		"http://example.com/&lt;sometag&gt;some-text&lt;/sometag&gt;")
	check("http://example-äüö.com/<sometag>some-text</sometag>",
		"http://example-%C3%A4%C3%BC%C3%B6.com/%3Csometag%3Esome-text%3C/sometag%3E",
		"http://example-äüö.com/&lt;sometag&gt;some-text&lt;/sometag&gt;",
		"http://example-äüö.com/&lt;sometag&gt;some-text&lt;/sometag&gt;")
	check(`http://www.example.com/%C3%A4"><script>alert("XSS!")</script>"`,
		"http://www.example.com/%C3%A4%22%3E%3Cscript%3Ealert(%22XSS!%22)%3C/script%3E%22",
		"http://www.example.com/%C3%A4&quot;&gt;&lt;script&gt;alert(&quot;XSS!&quot;)&lt;/script&gt;&quot;",
		"http://www.example.com/%C3%A4&quot;&gt;&lt;script&gt;alert(&quot;XSS!&quot;)&lt;/script&gt;&quot;")
}

// Ported from Django: tests/admin_widgets/tests.py::AdminUUIDWidgetTests.test_attrs
func TestAdminUUIDWidgetAttrs(t *testing.T) {
	w := admin.NewAdminUUIDInputWidget(nil)
	djangotest.AssertHTMLEqual(t, w.Render("test", "550e8400-e29b-41d4-a716-446655440000", nil),
		`<input value="550e8400-e29b-41d4-a716-446655440000" type="text" `+
			`class="vUUIDField" name="test">`)
	w = admin.NewAdminUUIDInputWidget(forms.Attrs{"class": "myUUIDInput"})
	djangotest.AssertHTMLEqual(t, w.Render("test", "550e8400-e29b-41d4-a716-446655440000", nil),
		`<input value="550e8400-e29b-41d4-a716-446655440000" type="text" `+
			`class="myUUIDInput" name="test">`)
}

// No Django test exists: Django's formfield_overrides passes any argument of
// the model field's formfield(), such as the strip=False of CoverLetterAdmin
// and MainPrepopulatedAdmin in tests/admin_views/admin.py, or a form_class.
// FormfieldOverride.Options and FormClass are the Go form of those arguments.
func TestFormfieldOverridesOptionsAndFormClass(t *testing.T) {
	djangotest.EachDB(t, widgetRelatedModels(), func(t *testing.T, db *orm.DB) {
		super := createTestUser(t, db, "super", "secret", true, true)
		// addPage returns the add page of model under ma, with the other
		// models under a default admin.
		addPage := func(t *testing.T, model string, ma admin.ModelAdmin) *djangotest.Response {
			t.Helper()
			admins := map[string]admin.ModelAdmin{model: ma}
			env := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
				admin.Register[WidgetBand](site, admins["widgetband"])
				admin.Register[WidgetMember](site, admins["widgetmember"])
				admin.Register[WidgetEvent](site, admins["widgetevent"])
			})
			env.client.ForceLogin(super)
			resp := env.client.Get(mustReverse(t, env.router, "admin:admin_widgets_"+model+"_add"))
			djangotest.AssertStatus(t, resp, 200)
			return resp
		}

		// The arguments win over those that the model field implies.
		t.Run("options", func(t *testing.T) {
			resp := addPage(t, "widgetband", admin.ModelAdmin{
				FormfieldOverrides: map[admin.FieldClass]admin.FormfieldOverride{
					admin.CharField: {Options: []forms.FieldOption{forms.Strip(false), forms.MaxLength(50)}},
				},
			})
			f, ok := formField(t, resp, "name").(*forms.CharField)
			if !ok {
				t.Fatalf("name = %T, want *forms.CharField", formField(t, resp, "name"))
			}
			djangotest.AssertEqual(t, f.Strip, false)
			djangotest.AssertContains(t, resp,
				`<input type="text" name="name" class="vTextField" maxlength="50" required id="id_name">`,
				djangotest.HTML())
		})

		// The admin's widget of the class stays, as the entry merges over
		// Django's FORMFIELD_FOR_DBFIELD_DEFAULTS.
		t.Run("form_class", func(t *testing.T) {
			resp := addPage(t, "widgetband", admin.ModelAdmin{
				FormfieldOverrides: map[admin.FieldClass]admin.FormfieldOverride{
					admin.CharField: {FormClass: forms.Class(forms.NewEmailField)},
				},
			})
			if f := formField(t, resp, "name"); !isEmailField(f) {
				t.Errorf("name = %T, want *forms.EmailField", f)
			}
			djangotest.AssertContains(t, resp,
				`<input type="text" name="name" class="vTextField" maxlength="100" required id="id_name">`,
				djangotest.HTML())
		})

		// A field class of FormOptions wins, as Meta.field_classes passes a
		// form_class that wins over formfield_overrides.
		t.Run("form_options_win", func(t *testing.T) {
			resp := addPage(t, "widgetband", admin.ModelAdmin{
				FormfieldOverrides: map[admin.FieldClass]admin.FormfieldOverride{
					admin.CharField: {FormClass: forms.Class(forms.NewEmailField)},
				},
				FormOptions: []forms.ModelFormOption{forms.WithFieldClass("name", forms.NewSlugField)},
			})
			if f, ok := formField(t, resp, "name").(*forms.SlugField); !ok {
				t.Errorf("name = %T, want *forms.SlugField", f)
			}
			if f := formField(t, resp, "style"); !isEmailField(f) {
				t.Errorf("style = %T, want *forms.EmailField", f)
			}
		})

		// A form class replaces the SplitDateTimeField of a date and time
		// field.
		t.Run("datetime_form_class", func(t *testing.T) {
			resp := addPage(t, "widgetmember", admin.ModelAdmin{
				FormfieldOverrides: map[admin.FieldClass]admin.FormfieldOverride{
					admin.DateTimeField: {FormClass: forms.Class(forms.NewDateTimeField), Widget: forms.DateTimeInput{}},
				},
			})
			if f, ok := formField(t, resp, "birthdate").(*forms.DateTimeField); !ok {
				t.Errorf("birthdate = %T, want *forms.DateTimeField", f)
			}
		})

		// A field with choices takes no arguments, as Django's
		// formfield_for_dbfield returns formfield_for_choice_field first.
		t.Run("choices", func(t *testing.T) {
			resp := addPage(t, "widgetmember", admin.ModelAdmin{
				FormfieldOverrides: map[admin.FieldClass]admin.FormfieldOverride{
					admin.CharField: {Options: []forms.FieldOption{forms.HelpText("Some help")}},
				},
			})
			djangotest.AssertEqual(t, formField(t, resp, "gender").Base().HelpText, "")
			djangotest.AssertEqual(t, formField(t, resp, "name").Base().HelpText, "Some help")
		})

		// A relation takes the arguments of its class.
		t.Run("foreign_key", func(t *testing.T) {
			resp := addPage(t, "widgetevent", admin.ModelAdmin{
				FormfieldOverrides: map[admin.FieldClass]admin.FormfieldOverride{
					admin.ForeignKey: {Options: []forms.FieldOption{forms.HelpText("Pick a band")}},
				},
			})
			djangotest.AssertEqual(t, formField(t, resp, "main_band").Base().HelpText, "Pick a band")
		})
	})
}

// isEmailField reports whether f is a *forms.EmailField.
func isEmailField(f forms.Field) bool {
	_, ok := f.(*forms.EmailField)
	return ok
}

// WidgetMedia has a field whose choices are in option groups.
type WidgetMedia struct {
	ID   int64
	Kind string `orm:"max_length=10;blank"`
}

// Meta puts the choices of Kind in the option groups Audio and Video.
func (WidgetMedia) Meta() orm.Meta {
	return orm.Meta{Choices: map[string][]orm.Choice{
		"Kind": {orm.ChoiceGroup("Audio", orm.ChoiceOf("cd", "CD")), orm.ChoiceGroup("Video", orm.ChoiceOf("dvd", "DVD"))},
	}}
}

// No Django test exists: the radio buttons of a field of RadioFields keep the
// option groups of its choices, because Django's formfield_for_choice_field
// hands get_choices() to the form field unchanged.
func TestAdminRadioFieldsOptionGroups(t *testing.T) {
	models := []any{auth.App{}, contenttypes.App{}, admin.App{}, djangotest.App("admin_widgets", &WidgetMedia{})}
	djangotest.EachDB(t, models, func(t *testing.T, db *orm.DB) {
		super := createTestUser(t, db, "super", "secret", true, true)
		env := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
			admin.Register[WidgetMedia](site, admin.ModelAdmin{RadioFields: map[string]admin.RadioStyle{"kind": admin.Vertical}})
		})
		env.client.ForceLogin(super)
		resp := env.client.Get(mustReverse(t, env.router, "admin:admin_widgets_widgetmedia_add"))
		djangotest.AssertContains(t, resp, `<div id="id_kind" class="radiolist"><div>`+
			`<label for="id_kind_0"><input type="radio" name="kind" value="" class="radiolist" `+
			`id="id_kind_0" checked> None</label></div>`+
			`<div><label>Audio</label><div><label for="id_kind_1_0"><input type="radio" name="kind" value="cd" `+
			`class="radiolist" id="id_kind_1_0"> CD</label></div></div>`+
			`<div><label>Video</label><div><label for="id_kind_2_0"><input type="radio" name="kind" value="dvd" `+
			`class="radiolist" id="id_kind_2_0"> DVD</label></div></div></div>`,
			djangotest.HTML())
	})
}

// WidgetCar is tests/admin_widgets/models.py::Car.
type WidgetCar struct {
	ID    int64
	Owner orm.FK[auth.User] `orm:"on_delete=cascade"`
	Make  string            `orm:"max_length=30"`
	Model string            `orm:"max_length=30"`
}

// String returns the make and the model.
func (c WidgetCar) String() string { return c.Make + " " + c.Model }

// WidgetCarTire is tests/admin_widgets/models.py::CarTire, a single car tire,
// to test that a user can only select their own cars.
type WidgetCarTire struct {
	ID  int64
	Car orm.FK[WidgetCar] `orm:"on_delete=cascade"`
}

// carTireAdmin is tests/admin_widgets/widgetadmin.py::CarTireAdmin: the car
// field offers the cars of the user of the request.
func carTireAdmin(db *orm.DB) admin.ModelAdmin {
	return admin.ModelAdmin{
		FormfieldQuerySet: func(r *http.Request, f *orm.Field) orm.AnyQuerySet {
			if f.Attr != "car" {
				return nil
			}
			u, _ := auth.CurrentUser(r.Context())
			return orm.Query[WidgetCar](db).Filter("owner", u.ID)
		},
	}
}

// carTireEnv creates the users and cars of Django's TestDataMixin, registers
// WidgetCarTire under ma and logs the superuser in. It returns the cars of
// the superuser and of the other user.
func carTireEnv(t *testing.T, db *orm.DB, ma admin.ModelAdmin) (env *adminTestEnv, own, other *WidgetCar) {
	t.Helper()
	super := createTestUser(t, db, "super", "secret", true, true)
	u2 := createTestUser(t, db, "testser", "secret", false, false)
	own = &WidgetCar{Make: "Volkswagen", Model: "Passat"}
	own.Owner.SetPK(super.ID)
	other = &WidgetCar{Make: "BMW", Model: "M3"}
	other.Owner.SetPK(u2.ID)
	mustCreate(t, db, own)
	mustCreate(t, db, other)
	env = newAdminTestEnv(t, db, func(site *admin.AdminSite) {
		admin.Register[WidgetCarTire](site, ma)
	})
	env.client.ForceLogin(super)
	return env, own, other
}

// carTireModels returns the models of the CarTire tests.
func carTireModels() []any {
	return []any{auth.App{}, contenttypes.App{}, admin.App{}, djangotest.App("admin_widgets", &WidgetCar{}, &WidgetCarTire{})}
}

// Ported from Django: tests/admin_widgets/tests.py::AdminFormfieldForDBFieldWithRequestTests.test_filter_choices_by_request_user
func TestAdminFormfieldForDBFieldWithRequestFilterChoicesByRequestUser(t *testing.T) {
	djangotest.EachDB(t, carTireModels(), func(t *testing.T, db *orm.DB) {
		env, _, _ := carTireEnv(t, db, carTireAdmin(db))
		resp := env.client.Get(mustReverse(t, env.router, "admin:admin_widgets_widgetcartire_add"))
		djangotest.AssertNotContains(t, resp, "BMW M3")
		djangotest.AssertContains(t, resp, "Volkswagen Passat")
	})
}

// No Django test exists: Django's ModelChoiceField looks the submitted key up
// in the queryset of formfield_for_foreignkey, so a row outside it is an
// invalid choice, also in a raw-id field.
func TestAdminFormfieldQuerySetValidates(t *testing.T) {
	for _, rawID := range []bool{false, true} {
		t.Run(fmt.Sprintf("raw id %v", rawID), func(t *testing.T) {
			djangotest.EachDB(t, carTireModels(), func(t *testing.T, db *orm.DB) {
				ma := carTireAdmin(db)
				if rawID {
					ma.RawIDFields = []string{"car"}
				}
				env, own, other := carTireEnv(t, db, ma)
				add := mustReverse(t, env.router, "admin:admin_widgets_widgetcartire_add")
				resp := env.client.Post(add, url.Values{"car": {strconv.FormatInt(other.ID, 10)}})
				djangotest.AssertContains(t, resp, "Select a valid choice. That choice is not one of the available choices.")
				resp = env.client.Post(add, url.Values{"car": {strconv.FormatInt(own.ID, 10)}})
				djangotest.AssertStatus(t, resp, http.StatusFound)
			})
		})
	}
}
