package admin_test

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/admin"
	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/forms"
	"github.com/oliverhaas/djangogo/orm"
)

// The Django tests ask formfield_for_dbfield for the widget of a model field;
// the Go admin builds its forms through a form field callback that is not
// exported, so these tests read the widget off the rendered add page.
//
// FIXME-PORT: test_FileField and test_inheritance need file fields. test_radio_fields_ForeignKey,
// test_radio_fields_foreignkey_formfield_overrides_empty_label and
// test_choices_with_radio_fields need radio_fields.
// test_formfield_overrides_widget_instances,
// test_formfield_overrides_m2m_filter_widget,
// test_formfield_overrides_for_datetime_field and
// test_formfield_overrides_for_custom_field set formfield_overrides by model
// field class, which the Go admin does not have; FormOptions set widgets by
// field name.
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
func TestAdminFormfieldForDBField(t *testing.T) {
	djangotest.EachDB(t, widgetRelatedModels(), func(t *testing.T, db *orm.DB) {
		super := createTestUser(t, db, "super", "secret", true, true)
		addPage := func(t *testing.T, event admin.ModelAdmin, route string) *djangotest.Response {
			t.Helper()
			env := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
				admin.Register[WidgetBand](site, admin.ModelAdmin{})
				admin.Register[WidgetMember](site, admin.ModelAdmin{})
				admin.Register[WidgetEvent](site, event)
			})
			env.client.ForceLogin(super)
			resp := env.client.Get(mustReverse(t, env.router, route))
			djangotest.AssertStatus(t, resp, 200)
			return resp
		}
		event := func(t *testing.T) *djangotest.Response {
			t.Helper()
			return addPage(t, admin.ModelAdmin{}, "admin:admin_widgets_widgetevent_add")
		}
		member := func(t *testing.T) *djangotest.Response {
			t.Helper()
			return addPage(t, admin.ModelAdmin{}, "admin:admin_widgets_widgetmember_add")
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
			resp := addPage(t, admin.ModelAdmin{FormOptions: []forms.ModelFormOption{
				forms.WithWidget("start_date", forms.TextInput{}),
			}}, "admin:admin_widgets_widgetevent_add")
			djangotest.AssertContains(t, resp, `<input type="text" name="start_date" id="id_start_date">`, djangotest.HTML())
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
