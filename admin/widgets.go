package admin

import (
	"context"
	"fmt"
	"html"
	"maps"
	"net/url"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/oliverhaas/djangogo/forms"
	"github.com/oliverhaas/djangogo/orm"
)

// FilteredSelectMultiple is the widget of a many-to-many field named in
// ModelAdmin.FilterHorizontal or FilterVertical, Django's
// admin.widgets.FilteredSelectMultiple: a <select multiple> with the class and
// data attributes that Django's SelectFilter2.js reads to build its two
// filtered lists.
type FilteredSelectMultiple struct {
	forms.SelectMultiple
	// VerboseName is the name of the field the script shows above the lists.
	VerboseName string
	// IsStacked puts the two lists above each other (filter_vertical) instead
	// of side by side (filter_horizontal).
	IsStacked bool
}

// Render renders the list with the class "selectfilter" or
// "selectfilterstacked" and the data-field-name and data-is-stacked
// attributes, which replace any of the same name.
func (w FilteredSelectMultiple) Render(name string, value any, attrs forms.Attrs) string {
	merged := forms.Attrs{}
	maps.Copy(merged, attrs)
	merged["class"] = "selectfilter"
	stacked := 0
	if w.IsStacked {
		merged["class"] = "selectfilterstacked"
		stacked = 1
	}
	merged["data-field-name"] = w.VerboseName
	merged["data-is-stacked"] = stacked
	return w.SelectMultiple.Render(name, value, merged)
}

// WithChoices implements forms.ChoiceWidget.
func (w FilteredSelectMultiple) WithChoices(choices []forms.Choice) forms.Widget {
	w.Choices = choices
	return w
}

// UseFieldset reports true, as Django's widget does.
func (FilteredSelectMultiple) UseFieldset() bool { return true }

// ForeignKeyRawIDWidget is the widget of a foreign key named in
// ModelAdmin.RawIDFields, Django's admin.widgets.ForeignKeyRawIdWidget: a
// text input for the primary key, a lookup link to the changelist of the
// related model, and the label of the selected object.
type ForeignKeyRawIDWidget struct {
	forms.TextInput
	// RelatedURL is the URL of the changelist of the related model with its
	// query string, or "" when the related model has no admin on the site.
	// Without it the widget has no lookup link and no default class.
	RelatedURL string
	// Lookup returns the label of the related object whose key is value and
	// the URL of its change page. The label is "" when there is no such
	// object, the URL when the related model has no admin. Nil shows no
	// label.
	Lookup func(value string) (label, url string)
}

// NewForeignKeyRawIDWidget returns the widget of the foreign key f on site,
// Django's ForeignKeyRawIdWidget(rel, admin_site). Its Lookup reads the
// related row with ctx.
func NewForeignKeyRawIDWidget(ctx context.Context, site *AdminSite, f *orm.Field) ForeignKeyRawIDWidget {
	w := ForeignKeyRawIDWidget{}
	if f.Rel == nil || f.Rel.Target == nil {
		return w
	}
	target := f.Rel.Target
	if rel := site.entryByModel(target); rel != nil {
		w.RelatedURL = site.changelistURL(rel)
		if pk := target.PrimaryKey(); pk != nil {
			w.RelatedURL += "?" + url.Values{toFieldVar: {pk.Attr}}.Encode()
		}
	}
	w.Lookup = func(value string) (string, string) {
		return site.rawIDLabel(ctx, target, value)
	}
	return w
}

// Render renders the input, with the class vForeignKeyRawIdAdminField unless
// the attributes set a class, the lookup link and the label.
func (w ForeignKeyRawIDWidget) Render(name string, value any, attrs forms.Attrs) string {
	merged := mergeWidgetAttrs(w.Attrs, attrs)
	if _, ok := merged["class"]; !ok && w.RelatedURL != "" {
		merged["class"] = "vForeignKeyRawIdAdminField"
	}
	var label, link string
	if s := keyText(value); s != "" && w.Lookup != nil {
		label, link = w.Lookup(s)
	}
	return rawIDHTML(name, w.TextInput.Render(name, value, merged), w.RelatedURL, label, link)
}

// ManyToManyRawIDWidget is the widget of a many-to-many field named in
// ModelAdmin.RawIDFields, Django's admin.widgets.ManyToManyRawIdWidget: a
// text input for the comma-separated primary keys and a lookup link to the
// changelist of the related model.
type ManyToManyRawIDWidget struct {
	forms.TextInput
	// RelatedURL is the URL of the changelist of the related model, or ""
	// when the related model has no admin on the site. Without it the
	// widget has no lookup link and no class.
	RelatedURL string
}

// NewManyToManyRawIDWidget returns the widget of the many-to-many field f on
// site, Django's ManyToManyRawIdWidget(rel, admin_site).
func NewManyToManyRawIDWidget(site *AdminSite, f *orm.Field) ManyToManyRawIDWidget {
	w := ManyToManyRawIDWidget{}
	if f.Rel != nil {
		if rel := site.entryByModel(f.Rel.Target); rel != nil {
			w.RelatedURL = site.changelistURL(rel)
		}
	}
	return w
}

// Render renders the input with the keys joined by commas, the class
// vManyToManyRawIdAdminField in place of any other, and the lookup link.
// The value attribute is there even when no key is selected, as in Django.
func (w ManyToManyRawIDWidget) Render(name string, value any, attrs forms.Attrs) string {
	merged := mergeWidgetAttrs(w.Attrs, attrs)
	if w.RelatedURL != "" {
		merged["class"] = "vManyToManyRawIdAdminField"
	}
	keys := strings.Join(keyTexts(value), ",")
	if keys == "" {
		merged["value"] = ""
	}
	return rawIDHTML(name, w.TextInput.Render(name, keys, merged), w.RelatedURL, "", "")
}

// ValueFromData returns the comma-separated keys submitted under name, nil
// when there are none.
func (ManyToManyRawIDWidget) ValueFromData(data url.Values, name string) any {
	vs := data[name]
	if len(vs) == 0 || vs[len(vs)-1] == "" {
		return nil
	}
	return strings.Split(vs[len(vs)-1], ",")
}

// rawIDHTML assembles the HTML of Django's foreign_key_raw_id.html template
// around the rendered input: the lookup link and the <div> around both when
// relatedURL is set, then the label, with a link when link is set.
func rawIDHTML(name, input, relatedURL, label, link string) string {
	var b strings.Builder
	if relatedURL != "" {
		b.WriteString("<div>")
	}
	b.WriteString(input + "\n")
	if relatedURL != "" {
		b.WriteString(`<a href="` + html.EscapeString(relatedURL) + `" class="related-lookup" id="lookup_id_` +
			html.EscapeString(name) + `" title="Lookup"></a>`)
	}
	if label != "" {
		b.WriteString("\n<strong>")
		if link != "" {
			b.WriteString(`<a href="` + html.EscapeString(link) + `">` + html.EscapeString(label) + "</a>")
		} else {
			b.WriteString(html.EscapeString(label))
		}
		b.WriteString("</strong>")
	}
	if relatedURL != "" {
		b.WriteString("</div>")
	}
	return b.String()
}

// AutocompleteSelect is the widget of a foreign key named in
// ModelAdmin.AutocompleteFields, Django's admin.widgets.AutocompleteSelect: a
// <select> with the data attributes of Django's autocomplete.js, which loads
// the options from the site's autocomplete view as the user types. It
// renders only the selected options and, unless IsRequired, an empty one.
type AutocompleteSelect struct {
	forms.Select
	autocomplete
}

// AutocompleteSelectMultiple is the widget of a many-to-many field named in
// ModelAdmin.AutocompleteFields, Django's
// admin.widgets.AutocompleteSelectMultiple: the multiple-choice
// AutocompleteSelect, without the empty option.
type AutocompleteSelectMultiple struct {
	forms.SelectMultiple
	autocomplete
}

// autocomplete holds what Django's AutocompleteMixin adds to a select.
type autocomplete struct {
	// URL is the URL of the site's autocomplete view.
	URL string
	// AppLabel and ModelName are the app label and the lowercase name of
	// the model of the field, FieldName its name. The view finds the
	// related model through them.
	AppLabel, ModelName, FieldName string
	// IsRequired is whether the form field is required (Django's
	// widget.is_required): a required field has no empty option and the
	// script offers no clear button. The admin sets it from the field.
	IsRequired bool
	// Debug loads the unminified jQuery and Select2, Django's settings.DEBUG.
	// The admin sets it from AdminSite.Debug.
	Debug bool
}

// NewAutocompleteSelect returns the widget of the foreign key f of model m on
// site, Django's AutocompleteSelect(field, admin_site, attrs).
func NewAutocompleteSelect(site *AdminSite, m *orm.Model, f *orm.Field, attrs forms.Attrs) AutocompleteSelect {
	return AutocompleteSelect{Select: forms.Select{Attrs: attrs}, autocomplete: newAutocomplete(site, m, f)}
}

// NewAutocompleteSelectMultiple returns the widget of the many-to-many field
// f of model m on site, Django's AutocompleteSelectMultiple(field,
// admin_site, attrs).
func NewAutocompleteSelectMultiple(site *AdminSite, m *orm.Model, f *orm.Field, attrs forms.Attrs) AutocompleteSelectMultiple {
	return AutocompleteSelectMultiple{
		SelectMultiple: forms.SelectMultiple{Attrs: attrs},
		autocomplete:   newAutocomplete(site, m, f),
	}
}

// newAutocomplete returns the settings of an autocomplete widget for the
// field f of m on site.
func newAutocomplete(site *AdminSite, m *orm.Model, f *orm.Field) autocomplete {
	return autocomplete{
		URL:       site.autocompleteURL(),
		AppLabel:  m.App(),
		ModelName: strings.ToLower(m.Name()),
		FieldName: f.Attr,
		Debug:     site.Debug,
	}
}

// attrs returns the attributes of Django's AutocompleteMixin.build_attrs over
// the widget's own and the given ones: the class admin-autocomplete after any
// other class, then the data attributes the script reads.
func (a autocomplete) attrs(own, given forms.Attrs) forms.Attrs {
	merged := mergeWidgetAttrs(own, given)
	class, _ := merged["class"].(string)
	if class != "" {
		class += " "
	}
	merged["class"] = class + "admin-autocomplete"
	merged["data-ajax--cache"] = "true"
	merged["data-ajax--delay"] = 250
	merged["data-ajax--type"] = "GET"
	merged["data-ajax--url"] = a.URL
	merged["data-app-label"] = a.AppLabel
	merged["data-model-name"] = a.ModelName
	merged["data-field-name"] = a.FieldName
	merged["data-theme"] = "admin-autocomplete"
	merged["data-allow-clear"] = strconv.FormatBool(!a.IsRequired)
	merged["data-placeholder"] = ""
	merged["lang"] = "en"
	return merged
}

// options returns the options of Django's AutocompleteMixin.optgroups: the
// empty option when empty is set, then the choices whose value is among
// selected, in their order.
func (autocomplete) options(choices []forms.Choice, selected []string, empty bool) []forms.Choice {
	var out []forms.Choice
	if empty {
		out = append(out, forms.Choice{})
	}
	for _, c := range choices {
		if c.Value != "" && slices.Contains(selected, c.Value) {
			out = append(out, c)
		}
	}
	return out
}

// Render renders the select with the autocomplete attributes and the
// selected options.
func (w AutocompleteSelect) Render(name string, value any, attrs forms.Attrs) string {
	selected := keyTexts(value)
	w.Choices = w.options(w.Choices, selected, !w.IsRequired)
	return w.Select.Render(name, selected, w.attrs(w.Attrs, attrs))
}

// WithChoices implements forms.ChoiceWidget.
func (w AutocompleteSelect) WithChoices(choices []forms.Choice) forms.Widget {
	w.Choices = choices
	return w
}

// Render renders the select with the autocomplete attributes and the
// selected options.
func (w AutocompleteSelectMultiple) Render(name string, value any, attrs forms.Attrs) string {
	selected := keyTexts(value)
	w.Choices = w.options(w.Choices, selected, false)
	return w.SelectMultiple.Render(name, selected, w.attrs(w.Attrs, attrs))
}

// WithChoices implements forms.ChoiceWidget.
func (w AutocompleteSelectMultiple) WithChoices(choices []forms.Choice) forms.Widget {
	w.Choices = choices
	return w
}

// mergeWidgetAttrs returns a new map with the widget's own attributes and the
// given ones over them.
func mergeWidgetAttrs(own, given forms.Attrs) forms.Attrs {
	merged := forms.Attrs{}
	maps.Copy(merged, own)
	maps.Copy(merged, given)
	return merged
}

// keyText returns the text of a single key value, "" for nil.
func keyText(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return v
	default:
		return fmt.Sprint(v)
	}
}

// keyTexts returns the texts of the keys in value, a single key or a slice
// of keys, without the empty ones.
func keyTexts(value any) []string {
	var out []string
	add := func(v any) {
		if s := keyText(v); s != "" {
			out = append(out, s)
		}
	}
	if rv := reflect.ValueOf(value); rv.Kind() == reflect.Slice {
		for i := range rv.Len() {
			add(rv.Index(i).Interface())
		}
		return out
	}
	add(value)
	return out
}
