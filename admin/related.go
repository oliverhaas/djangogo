package admin

import (
	"context"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/oliverhaas/djangogo/forms"
	"github.com/oliverhaas/djangogo/orm"
)

// rawIDLabelWords is the number of words of a label a raw-id widget shows,
// the Truncator(obj).words(14) of Django's label_and_url_for_value.
const rawIDLabelWords = 14

// autocompleteFields returns the autocomplete fields for r, Django's
// get_autocomplete_fields.
func (e *entry) autocompleteFields(r *http.Request) []string {
	if e.admin.GetAutocompleteFields != nil {
		return e.admin.GetAutocompleteFields(r)
	}
	return e.admin.AutocompleteFields
}

// searchFields returns the search fields for r, Django's get_search_fields.
func (e *entry) searchFields(r *http.Request) []string {
	if e.admin.GetSearchFields != nil {
		return e.admin.GetSearchFields(r)
	}
	return e.admin.SearchFields
}

// limitedFields returns the relation fields of e whose form field loads only
// the related rows it shows or receives: the autocomplete fields for r and
// the raw-id fields, without the fields that FormfieldOverrides gives a
// widget.
func (e *entry) limitedFields(r *http.Request) []*orm.Field {
	autocomplete := e.autocompleteFields(r)
	var out []*orm.Field
	for _, f := range slices.Concat(e.model.Relations(), e.model.ManyToMany()) {
		if _, o := e.formfieldOverride(f); o.Widget != nil {
			continue
		}
		if namesField(autocomplete, f) || namesField(e.admin.RawIDFields, f) {
			out = append(out, f)
		}
	}
	return out
}

// relationWidgets returns the widget options of Django's
// formfield_for_foreignkey and formfield_for_manytomany for the relation
// fields of e: the widget of FormfieldOverrides, else an autocomplete widget
// for an autocomplete field, else a raw-id widget for a raw-id field, else a
// FilteredSelectMultiple for a many-to-many field named in FilterHorizontal
// or FilterVertical. A many-to-many field shown as a list gets the multiple
// selection hint after its help text.
func (s *AdminSite) relationWidgets(r *http.Request, e *entry) []forms.ModelFormOption {
	autocomplete := e.autocompleteFields(r)
	var opts []forms.ModelFormOption
	for _, f := range e.model.Relations() {
		_, o := e.formfieldOverride(f)
		switch {
		case o.Widget != nil:
			opts = append(opts, forms.WithWidget(f.Attr, o.Widget))
		case namesField(autocomplete, f):
			opts = append(opts, forms.WithWidget(f.Attr, NewAutocompleteSelect(s, e.model, f, nil)))
		case namesField(e.admin.RawIDFields, f):
			opts = append(opts, forms.WithWidget(f.Attr, NewForeignKeyRawIDWidget(r.Context(), s, f)))
		}
	}
	for _, mf := range e.model.ManyToMany() {
		if mf.Rel == nil || !mf.Rel.AutoCreated {
			continue
		}
		_, o := e.formfieldOverride(mf)
		w := o.Widget
		vertical := namesField(e.admin.FilterVertical, mf)
		switch {
		case w != nil:
		case namesField(autocomplete, mf):
			w = NewAutocompleteSelectMultiple(s, e.model, mf, nil)
		case namesField(e.admin.RawIDFields, mf):
			w = NewManyToManyRawIDWidget(s, mf)
		case vertical || namesField(e.admin.FilterHorizontal, mf):
			w = FilteredSelectMultiple{VerboseName: fieldVerboseName(mf), IsStacked: vertical}
		}
		if w != nil {
			opts = append(opts, forms.WithWidget(mf.Attr, w))
		}
		if !selectsMultiple(w) {
			continue
		}
		help := multipleSelectHint
		if mf.HelpText != "" {
			help = mf.HelpText + " " + multipleSelectHint
		}
		opts = append(opts, forms.WithHelpText(mf.Attr, help))
	}
	return opts
}

// selectsMultiple reports whether w, the widget of a many-to-many field, is
// a list that selects several rows with the Control key, which Django's
// formfield_for_manytomany explains in the help text: the default list (nil),
// a SelectMultiple or a FilteredSelectMultiple.
func selectsMultiple(w forms.Widget) bool {
	switch w.(type) {
	case nil, forms.SelectMultiple, FilteredSelectMultiple:
		return true
	}
	return false
}

// syncAutocompleteRequired sets IsRequired of every autocomplete widget of
// form from its field, as Django's Field sets widget.is_required.
func syncAutocompleteRequired(form *forms.Form) {
	for _, name := range form.FieldNames() {
		fd, _ := form.FieldDef(name)
		base := fd.Base()
		switch w := base.Widget.(type) {
		case AutocompleteSelect:
			w.IsRequired = base.Required
			base.Widget = w
		case AutocompleteSelectMultiple:
			w.IsRequired = base.Required
			base.Widget = w
		}
	}
}

// loadSelectedChoices sets the choices of the raw-id and autocomplete fields
// of form to the related rows whose keys the fields show: the submitted
// values of a bound form, else the initial ones. A key that names no row has
// no choice, so validation rejects it with Django's invalid_choice message.
func (s *AdminSite) loadSelectedChoices(r *http.Request, e *entry, form *forms.Form) error {
	present := formFieldNames(form)
	for _, f := range e.limitedFields(r) {
		if f.Rel == nil || f.Rel.Target == nil || !present[f.Attr] {
			continue
		}
		target := f.Rel.Target
		keys := relatedKeys(target, keyTexts(form.Field(f.Attr).Value()))
		rows, err := orm.LabeledRowsOrdered(r.Context(), s.db, target, s.relatedOrdering(r, target), keys)
		if err != nil {
			return err
		}
		form.SetChoices(f.Attr, rows)
	}
	return nil
}

// relatedKeys returns the texts that are primary keys of target, converted
// to the type of the key. A text that is no such key is left out, so the
// result is never nil.
func relatedKeys(target *orm.Model, texts []string) []any {
	out := make([]any, 0, len(texts))
	pk := target.PrimaryKey()
	if pk == nil {
		return out
	}
	for _, text := range texts {
		switch pk.Kind {
		case orm.KindAuto, orm.KindInt, orm.KindSmallInt, orm.KindBigInt,
			orm.KindPositiveSmallInt, orm.KindPositiveInt, orm.KindPositiveBigInt:
			if n, err := strconv.ParseInt(text, 10, 64); err == nil {
				out = append(out, n)
			}
		case orm.KindUUID:
			if u, err := orm.ParseUUID(text); err == nil {
				out = append(out, u)
			}
		default:
			out = append(out, text)
		}
	}
	return out
}

// rawIDLabel returns the label of the row of target whose key is value,
// shortened to rawIDLabelWords words, and the URL of its change page,
// Django's ForeignKeyRawIdWidget.label_and_url_for_value. Both are "" when
// no row has the key; the URL is "" when target has no admin on s.
func (s *AdminSite) rawIDLabel(ctx context.Context, target *orm.Model, value string) (label, link string) {
	keys := relatedKeys(target, []string{value})
	if len(keys) == 0 {
		return "", ""
	}
	rows, err := orm.LabeledRowsOrdered(ctx, s.db, target, nil, keys)
	if err != nil || len(rows) == 0 {
		return "", ""
	}
	if rel := s.entryByModel(target); rel != nil {
		if pk, ok := keys[0].(int64); ok {
			link = s.changeURL(rel, pk)
		}
	}
	return truncateWords(rows[0][1], rawIDLabelWords), link
}

// truncateWords returns the first n words of s joined by single spaces, with
// "…" after them when s has more, Django's Truncator.words.
func truncateWords(s string, n int) string {
	words := strings.Fields(s)
	if len(words) <= n {
		return strings.Join(words, " ")
	}
	return strings.Join(words[:n], " ") + "…"
}
