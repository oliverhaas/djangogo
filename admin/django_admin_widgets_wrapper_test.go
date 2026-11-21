package admin_test

import (
	"net/url"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/admin"
	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/forms"
	"github.com/oliverhaas/djangogo/orm"
)

// customRenderSelect stands in for the CustomWidget of
// RelatedFieldWidgetWrapperTests.test_custom_widget_render.
type customRenderSelect struct{ forms.Select }

// Render returns a fixed text.
func (customRenderSelect) Render(string, any, forms.Attrs) string { return "custom render output" }

// presentSelect stands in for the CustomWidget of
// RelatedFieldWidgetWrapperTests.test_widget_delegates_value_omitted_from_data.
type presentSelect struct{ forms.Select }

// ValueOmittedFromData reports that the data always holds a value.
func (presentSelect) ValueOmittedFromData(url.Values, string) bool { return false }

// Django's wrapper offers the add link when its can_add_related argument is
// left out and the related model has an admin. The Go wrapper takes the links
// it offers from the admin, so the cases that leave can_add_related out pass
// RelatedActions{Add: true} for a related model with an admin. Django's
// AdminRadioSelect is a RadioSelect; the Go admin has no radio_fields yet, so
// the tests use forms.RadioSelect.
//
// Ported from Django: tests/admin_widgets/tests.py::RelatedFieldWidgetWrapperTests.test_select_multiple_widget_cant_change_delete_related
// Ported from Django: tests/admin_widgets/tests.py::RelatedFieldWidgetWrapperTests.test_on_delete_cascade_rel_cant_delete_related
// Ported from Django: tests/admin_widgets/tests.py::RelatedFieldWidgetWrapperTests.test_custom_widget_render
// Ported from Django: tests/admin_widgets/tests.py::RelatedFieldWidgetWrapperTests.test_widget_delegates_value_omitted_from_data
// Ported from Django: tests/admin_widgets/tests.py::RelatedFieldWidgetWrapperTests.test_widget_is_hidden
// Ported from Django: tests/admin_widgets/tests.py::RelatedFieldWidgetWrapperTests.test_widget_is_not_hidden
// Ported from Django: tests/admin_widgets/tests.py::RelatedFieldWidgetWrapperTests.test_data_model_ref_when_model_name_is_camel_case
// Ported from Django: tests/admin_widgets/tests.py::RelatedFieldWidgetWrapperTests.test_non_select_widget_cant_change_delete_related
func TestRelatedFieldWidgetWrapper(t *testing.T) {
	djangotest.EachDB(t, widgetRelatedModels(), func(t *testing.T, db *orm.DB) {
		env := widgetEnv(t, db, admin.ModelAdmin{})
		all := admin.RelatedActions{Add: true, Change: true, Delete: true}

		t.Run("test_select_multiple_widget_cant_change_delete_related", func(t *testing.T) {
			m, f := modelField(t, db, "admin_widgets.Individual", "Parent")
			wrapper := admin.NewRelatedFieldWidgetWrapper(forms.SelectMultiple{}, env.site, m, f, all)
			djangotest.AssertEqual(t, wrapper.CanAddRelated, true)
			djangotest.AssertEqual(t, wrapper.CanChangeRelated, false)
			djangotest.AssertEqual(t, wrapper.CanDeleteRelated, false)
		})

		t.Run("test_on_delete_cascade_rel_cant_delete_related", func(t *testing.T) {
			m, f := modelField(t, db, "admin_widgets.Individual", "Soulmate")
			wrapper := admin.NewRelatedFieldWidgetWrapper(forms.Select{}, env.site, m, f, all)
			djangotest.AssertEqual(t, wrapper.CanAddRelated, true)
			djangotest.AssertEqual(t, wrapper.CanChangeRelated, true)
			djangotest.AssertEqual(t, wrapper.CanDeleteRelated, false)
		})

		t.Run("test_custom_widget_render", func(t *testing.T) {
			m, f := modelField(t, db, "admin_widgets.WidgetAlbum", "Band")
			wrapper := admin.NewRelatedFieldWidgetWrapper(customRenderSelect{}, env.site, m, f, all)
			output := wrapper.Render("name", "value", nil)
			assertIn(t, "custom render output", output)
		})

		t.Run("test_widget_delegates_value_omitted_from_data", func(t *testing.T) {
			m, f := modelField(t, db, "admin_widgets.WidgetAlbum", "Band")
			wrapper := admin.NewRelatedFieldWidgetWrapper(presentSelect{}, env.site, m, f, admin.RelatedActions{Add: true})
			djangotest.AssertEqual(t, wrapper.ValueOmittedFromData(url.Values{}, "band"), false)
		})

		t.Run("test_widget_is_hidden", func(t *testing.T) {
			m, f := modelField(t, db, "admin_widgets.WidgetAlbum", "Band")
			wrapper := admin.NewRelatedFieldWidgetWrapper(forms.HiddenInput{}, env.site, m, f, admin.RelatedActions{Add: true})
			djangotest.AssertEqual(t, wrapper.IsHidden(), true)
			output := wrapper.Render("name", "value", nil)
			// Related item links are hidden.
			if strings.Contains(output, "<a ") {
				t.Errorf("links in %s", output)
			}
		})

		t.Run("test_widget_is_not_hidden", func(t *testing.T) {
			m, f := modelField(t, db, "admin_widgets.WidgetAlbum", "Band")
			wrapper := admin.NewRelatedFieldWidgetWrapper(forms.Select{}, env.site, m, f, admin.RelatedActions{Add: true})
			djangotest.AssertEqual(t, wrapper.IsHidden(), false)
			output := wrapper.Render("name", "value", nil)
			// Related item links are present.
			assertIn(t, "<a ", output)
		})

		t.Run("test_data_model_ref_when_model_name_is_camel_case", func(t *testing.T) {
			// Django's ReleaseEvent has the album as its primary key; the Go
			// stand-in has its own id, and its model is named
			// WidgetReleaseEvent.
			m, f := modelField(t, db, "admin_widgets.WidgetVideoStream", "ReleaseEvent")
			wrapper := admin.NewRelatedFieldWidgetWrapper(forms.Select{}, env.site, m, f, admin.RelatedActions{Add: true})
			djangotest.AssertEqual(t, wrapper.IsHidden(), false)
			output := wrapper.Render("stream", "value", nil)
			addURL := mustReverse(t, env.router, "admin:admin_widgets_widgetreleaseevent_add")
			expected := `
			<div class="related-widget-wrapper" data-model-ref="widgetreleaseevent">
			  <select name="stream" data-context="available-source">
			  </select>
			  <a class="related-widget-wrapper-link add-related" id="add_id_stream"
			     data-popup="yes" title="Add another widget release event"
			     href="` + addURL + `?_to_field=id&amp;_popup=1&_source_model=admin_widgets.widgetvideostream">
			    <img src="/static/admin/img/icon-addlink.svg" alt="" width="24" height="24">
			  </a>
			</div>
			`
			djangotest.AssertHTMLEqual(t, output, expected)
		})

		t.Run("test_non_select_widget_cant_change_delete_related", func(t *testing.T) {
			m, f := modelField(t, db, "admin_widgets.WidgetEvent", "MainBand")
			wrapper := admin.NewRelatedFieldWidgetWrapper(forms.RadioSelect{}, env.site, m, f, all)
			djangotest.AssertEqual(t, wrapper.CanAddRelated, true)
			djangotest.AssertEqual(t, wrapper.CanChangeRelated, false)
			djangotest.AssertEqual(t, wrapper.CanDeleteRelated, false)
		})
	})
}

// Django's test builds the wrapper without can_add_related for a relation to
// a model without an admin. The Go admin decides the links from the related
// admin, so the test reads the add page of Bee, whose honeycomb has no admin:
// the field is wrapped and has no add link.
//
// Ported from Django: tests/admin_widgets/tests.py::RelatedFieldWidgetWrapperTests.test_no_can_add_related
func TestRelatedFieldWidgetWrapperNoCanAddRelated(t *testing.T) {
	djangotest.EachDB(t, widgetRelatedModels(), func(t *testing.T, db *orm.DB) {
		super := createTestUser(t, db, "super", "secret", true, true)
		env := widgetEnv(t, db, admin.ModelAdmin{})
		env.client.ForceLogin(super)
		resp := env.client.Get(mustReverse(t, env.router, "admin:admin_widgets_bee_add"))
		djangotest.AssertContains(t, resp, `<div class="related-widget-wrapper" data-model-ref="honeycomb">`)
		djangotest.AssertNotContains(t, resp, `id="add_id_honeycomb"`)
	})
}

// No Django test reads the links of a wrapped field off the change form for a
// user with some of the permissions of the related admin; Django's
// formfield_for_dbfield passes the related admin's has_*_permission results
// to the wrapper.
func TestRelatedFieldWidgetWrapperPermissions(t *testing.T) {
	djangotest.EachDB(t, widgetRelatedModels(), func(t *testing.T, db *orm.DB) {
		env := widgetEnv(t, db, admin.ModelAdmin{})
		user := createTestUser(t, db, "staff", "secret", true, false)
		grantPerm(t, db, user, "admin_widgets", "add_widgetalbum")
		grantPerm(t, db, user, "admin_widgets", "view_widgetband")
		env.client.ForceLogin(user)
		resp := env.client.Get(mustReverse(t, env.router, "admin:admin_widgets_widgetalbum_add"))
		body := resp.Content()
		djangotest.AssertNotContains(t, resp, `id="add_id_band"`)
		djangotest.AssertNotContains(t, resp, `id="change_id_band"`)
		djangotest.AssertNotContains(t, resp, `id="delete_id_band"`)
		djangotest.AssertContains(t, resp, `id="view_id_band"`)
		viewTemplate := mustReverse(t, env.router, "admin:admin_widgets_widgetband_change", "__fk__")
		if !strings.Contains(body, `data-href-template="`+viewTemplate+`?_to_field=id"`) {
			t.Errorf("no view link to %s in\n%s", viewTemplate, body)
		}
	})
}

// assertIn fails the test unless s contains sub, Python's assertIn on strings.
func assertIn(t *testing.T, sub, s string) {
	t.Helper()
	if !strings.Contains(s, sub) {
		t.Errorf("%q not found in %q", sub, s)
	}
}
