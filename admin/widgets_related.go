package admin

import (
	"maps"
	"net/http"
	"reflect"
	"slices"
	"strings"

	"github.com/oliverhaas/djangogo/forms"
	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/templates"
)

// RelatedActions holds which links a RelatedFieldWidgetWrapper offers next to
// its widget: add a related object, and change, delete or view the selected
// one.
type RelatedActions struct {
	Add, Change, Delete, View bool
}

// RelatedFieldWidgetWrapper wraps the widget of a relation field with the
// links that add a related object, or change, delete or view the selected
// one, each in a popup window, Django's admin.widgets.RelatedFieldWidgetWrapper.
// The admin wraps the widget of every foreign key and many-to-many field of
// the change form that is not a raw-id field. It reads values and reports
// what the wrapped widget does.
type RelatedFieldWidgetWrapper struct {
	// Widget is the wrapped widget.
	forms.Widget
	// CanAddRelated shows the link that adds a related object.
	CanAddRelated bool
	// CanChangeRelated shows the link that changes the selected object.
	// Only a widget for a single choice that is a forms.Select offers it.
	CanChangeRelated bool
	// CanDeleteRelated shows the link that deletes the selected object. A
	// foreign key whose rows are deleted with the related object does not
	// offer it, nor does a widget that offers no change link.
	CanDeleteRelated bool
	// CanViewRelated shows the link that views the selected object, under
	// the same rule as the change link.
	CanViewRelated bool

	// modelName and verboseName name the related model.
	modelName, verboseName string
	// toField is the attribute name of the related primary key.
	toField string
	// urlParams is the query string of the popup links.
	urlParams string
	// addURL is the URL of the related add view; changeURL and deleteURL are
	// the URLs of the change and delete views with __fk__ in place of the
	// key. Each is "" when the related model has no admin on the site.
	addURL, changeURL, deleteURL string
}

// NewRelatedFieldWidgetWrapper returns w wrapped with the links of can for the
// relation field f of model m on site, Django's RelatedFieldWidgetWrapper(
// widget, rel, admin_site, can_add_related, ...). A link to a view that the
// related model does not have on site is left out when the widget renders.
func NewRelatedFieldWidgetWrapper(w forms.Widget, site *AdminSite, m *orm.Model, f *orm.Field, can RelatedActions) RelatedFieldWidgetWrapper {
	supported := isSingleSelect(w)
	wrapper := RelatedFieldWidgetWrapper{
		Widget:           w,
		CanAddRelated:    can.Add,
		CanChangeRelated: supported && can.Change,
		CanDeleteRelated: supported && can.Delete && f.Rel.OnDelete != orm.OnDeleteCascade,
		CanViewRelated:   supported && can.View,
	}
	target := f.Rel.Target
	wrapper.modelName = strings.ToLower(target.Name())
	wrapper.verboseName = target.VerboseName()
	if pk := target.PrimaryKey(); pk != nil {
		wrapper.toField = pk.Attr
	}
	wrapper.urlParams = toFieldVar + "=" + wrapper.toField + "&" + isPopupVar + "=1&" +
		sourceModelVar + "=" + m.LabelLower()
	if rel := site.entryByModel(target); rel != nil {
		wrapper.addURL = site.reverse(rel.urlName("add"))
		wrapper.changeURL = site.reverse(rel.urlName("change"), "__fk__")
		wrapper.deleteURL = site.reverse(rel.urlName("delete"), "__fk__")
	}
	return wrapper
}

// selectType is the type of the single-choice select widget.
var selectType = reflect.TypeFor[forms.Select]()

// isSingleSelect reports whether w is a forms.Select or embeds one, the
// isinstance(widget, Select) of Django's wrapper. A forms.SelectMultiple is
// not one.
func isSingleSelect(w forms.Widget) bool {
	t := reflect.TypeOf(w)
	for t != nil && t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t == nil || t.Kind() != reflect.Struct {
		return false
	}
	if t == selectType {
		return true
	}
	f, ok := t.FieldByName("Select")
	return ok && f.Anonymous && f.Type == selectType
}

// Render renders the wrapped widget and, unless it is hidden, the links,
// inside a <div> that names the related model. The widget gets the
// attribute data-context="available-source" unless it is an autocomplete
// widget.
func (w RelatedFieldWidgetWrapper) Render(name string, value any, attrs forms.Attrs) string {
	if _, ok := w.Widget.(interface{ isAutocomplete() }); !ok {
		merged := forms.Attrs{"data-context": "available-source"}
		maps.Copy(merged, attrs)
		attrs = merged
	}
	var b strings.Builder
	b.WriteString(`<div class="related-widget-wrapper" data-model-ref="` + forms.Escape(w.modelName) + `">` + "\n")
	b.WriteString(w.Widget.Render(name, value, attrs) + "\n")
	if !w.IsHidden() {
		params := forms.Escape(w.urlParams)
		model := forms.Escape(w.verboseName)
		id := forms.Escape(name)
		if w.CanChangeRelated && w.changeURL != "" {
			b.WriteString(`<a class="related-widget-wrapper-link change-related" id="change_id_` + id +
				`" data-href-template="` + forms.Escape(w.changeURL) + "?" + params +
				`" data-popup="yes" title="Change selected ` + model + `">` + relatedIcon("changelink") + "</a>")
		}
		if w.CanAddRelated && w.addURL != "" {
			b.WriteString(`<a class="related-widget-wrapper-link add-related" id="add_id_` + id +
				`" data-popup="yes" href="` + forms.Escape(w.addURL) + "?" + params +
				`" title="Add another ` + model + `">` + relatedIcon("addlink") + "</a>")
		}
		if w.CanDeleteRelated && w.deleteURL != "" {
			b.WriteString(`<a class="related-widget-wrapper-link delete-related" id="delete_id_` + id +
				`" data-href-template="` + forms.Escape(w.deleteURL) + "?" + params +
				`" data-popup="yes" title="Delete selected ` + model + `">` + relatedIcon("deletelink") + "</a>")
		}
		if w.CanViewRelated && w.changeURL != "" {
			b.WriteString(`<a class="related-widget-wrapper-link view-related" id="view_id_` + id +
				`" data-href-template="` + forms.Escape(w.changeURL) + "?" + toFieldVar + "=" + forms.Escape(w.toField) +
				`" title="View selected ` + model + `">` + relatedIcon("viewlink") + "</a>")
		}
	}
	b.WriteString("\n</div>")
	return b.String()
}

// relatedIcon returns the <img> of the admin icon icon-<name>.svg.
func relatedIcon(name string) string {
	return `<img src="` + forms.Escape(templates.Static("admin/img/icon-"+name+".svg")) +
		`" alt="" width="24" height="24">`
}

// WithChoices implements forms.ChoiceWidget: the wrapped widget gets the
// choices when it takes choices.
func (w RelatedFieldWidgetWrapper) WithChoices(choices []forms.Choice) forms.Widget {
	if cw, ok := w.Widget.(forms.ChoiceWidget); ok {
		w.Widget = cw.WithChoices(choices)
	}
	return w
}

// Media returns the media of the wrapped widget.
func (w RelatedFieldWidgetWrapper) Media() forms.Media { return forms.WidgetMedia(w.Widget) }

// Unwrap returns the wrapped widget.
func (w RelatedFieldWidgetWrapper) Unwrap() forms.Widget { return w.Widget }

// wrapRelatedWidgets wraps the widget of each relation field of form in a
// RelatedFieldWidgetWrapper, with the links that the user of r may use on the
// admin of the related model, as Django's formfield_for_dbfield does. Only
// the fields named in built, the ones the model form built from the model,
// are wrapped, and no raw-id field.
func (s *AdminSite) wrapRelatedWidgets(r *http.Request, e *entry, form *forms.Form, built map[string]bool) {
	for _, f := range slices.Concat(e.model.Relations(), e.model.ManyToMany()) {
		if f.Rel == nil || f.Rel.Target == nil || !built[f.Attr] || namesField(e.admin.RawIDFields, f) {
			continue
		}
		fd, ok := form.FieldDef(f.Attr)
		if !ok {
			continue
		}
		base := fd.Base()
		base.Widget = NewRelatedFieldWidgetWrapper(base.Widget, s, e.model, f, s.relatedActions(r, f.Rel.Target))
	}
}

// relatedActions returns the links a wrapper offers for a relation to target:
// the permissions of the user of r on the admin of target, and none when
// target has no admin on s.
func (s *AdminSite) relatedActions(r *http.Request, target *orm.Model) RelatedActions {
	rel := s.entryByModel(target)
	if rel == nil {
		return RelatedActions{}
	}
	return RelatedActions{
		Add:    rel.hasAddPermission(s, r),
		Change: rel.hasChangePermission(s, r, nil),
		Delete: rel.hasDeletePermission(s, r, nil),
		View:   rel.hasViewPermission(s, r, nil),
	}
}
