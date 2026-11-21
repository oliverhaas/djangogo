package admin

import (
	"maps"

	"github.com/oliverhaas/djangogo/forms"
)

// FilteredSelectMultiple is the widget of a many-to-many field named in
// ModelAdmin.FilterHorizontal or FilterVertical, Django's
// admin.widgets.FilteredSelectMultiple: a <select multiple> with the class and
// data attributes that Django's SelectFilter2.js reads to build its two
// filtered lists. The Go admin ships no JavaScript, so the browser shows a
// plain list.
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
