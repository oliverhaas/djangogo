// Package admin provides a Django-style admin site: an AdminSite that registers
// models, gates access to active staff users, enforces per-model permissions,
// and renders the app-grouped index, app index, changelist, and add, change,
// delete, and history views from embedded pongo2 templates.
package admin

import (
	"context"
	"net/http"

	"github.com/oliverhaas/djangogo/forms"
	"github.com/oliverhaas/djangogo/orm"
)

// ActionCheckboxName is the HTML input name of the per-row selection checkbox
// on the changelist (Django's helpers.ACTION_CHECKBOX_NAME).
const ActionCheckboxName = "_selected_action"

// DisplayColumn defines a custom computed column referenced by name in
// ModelAdmin.ListDisplay (the Go analog of a ModelAdmin method decorated with
// @admin.display). An entry without a Value shows the model method of its
// name, and its other options are the @admin.display options of that
// method.
type DisplayColumn struct {
	// Description is the column header text (Django's short_description).
	// When empty, derived from the column name.
	Description string
	// OrderField is the ORM field path used when sorting by this column
	// (Django's admin_order_field). Empty means the column is not sortable.
	OrderField string
	// Boolean renders the returned value as a Django boolean indicator
	// (Django's @display(boolean=True)).
	Boolean bool
	// EmptyValue overrides the empty-value placeholder for this column.
	EmptyValue string
	// Value computes the display value for obj (a *T). A template.HTML
	// value is HTML that the page shows as it is.
	Value func(obj any) any
}

// FilterLookup is one selectable choice returned by SimpleListFilter.Lookups.
type FilterLookup struct {
	// Value is the query-string value submitted when this choice is clicked.
	Value string
	// Display is the human-readable label rendered in the filter sidebar.
	Display string
}

// SimpleListFilter is a custom changelist sidebar filter that does not map
// directly to a single model field (Django's SimpleListFilter).
type SimpleListFilter struct {
	// Title is the human-readable heading rendered after "By " in the sidebar.
	Title string
	// ParameterName is the URL query parameter controlled by this filter.
	ParameterName string
	// Lookups returns the selectable (value, display) pairs for r.
	Lookups func(r *http.Request) []FilterLookup
	// Filter returns the orm.Cond applied to the changelist queryset for the
	// active parameter value (selected is false when the parameter is absent).
	// Return ok=false to leave the queryset unchanged.
	Filter func(r *http.Request, value string, selected bool) (orm.Cond, bool)
}

// FieldFilterKind selects a specific built-in FieldListFilter class for a
// model field or relation path.
type FieldFilterKind uint8

const (
	// FilterAuto picks the filter class from field metadata, matching Django's
	// FieldListFilter.create registration order.
	FilterAuto FieldFilterKind = iota
	// FilterRelated filters by all instances of a related model.
	FilterRelated
	// FilterRelatedOnly filters by related instances referenced by the model's
	// current queryset (Django's RelatedOnlyFieldListFilter).
	FilterRelatedOnly
	// FilterBoolean filters a boolean field with Yes/No[/Unknown].
	FilterBoolean
	// FilterChoices filters a field with declared choices.
	FilterChoices
	// FilterDate filters a Date/DateTime field with Today/Past 7 days/This month/This year.
	FilterDate
	// FilterAllValues filters by every distinct value present in the queryset.
	FilterAllValues
	// FilterEmpty filters by empty string or NULL (Django's EmptyFieldListFilter).
	FilterEmpty
)

// FieldFilter binds a field name or "__"-spanning relation path to a specific
// FieldFilterKind (the Go analog of Django's (field, FilterClass) tuple in
// list_filter).
type FieldFilter struct {
	// Field is a model field name, attribute name, or "__"-spanning path.
	Field string
	// Kind selects which built-in FieldListFilter implementation to use.
	Kind FieldFilterKind
	// Title optionally overrides the sidebar heading for this filter.
	Title string
}

// RelatedOnlyFilter constructs a FieldFilter using RelatedOnlyFieldListFilter.
func RelatedOnlyFilter(field string) FieldFilter {
	return FieldFilter{Field: field, Kind: FilterRelatedOnly}
}

// EmptyFilter constructs a FieldFilter using EmptyFieldListFilter.
func EmptyFilter(field string) FieldFilter {
	return FieldFilter{Field: field, Kind: FilterEmpty}
}

// AllValuesFilter constructs a FieldFilter using AllValuesFieldListFilter.
func AllValuesFilter(field string) FieldFilter {
	return FieldFilter{Field: field, Kind: FilterAllValues}
}

// BooleanFilter constructs a FieldFilter using BooleanFieldListFilter.
func BooleanFilter(field string) FieldFilter {
	return FieldFilter{Field: field, Kind: FilterBoolean}
}

// ChoicesFilter constructs a FieldFilter using ChoicesFieldListFilter.
func ChoicesFilter(field string) FieldFilter {
	return FieldFilter{Field: field, Kind: FilterChoices}
}

// DateFilter constructs a FieldFilter using DateFieldListFilter.
func DateFilter(field string) FieldFilter {
	return FieldFilter{Field: field, Kind: FilterDate}
}

// ActionContext provides an admin bulk action with access to the site, database,
// model metadata, and the selected queryset (either the checked rows or every
// row matching the active changelist filters when SelectAcross is true).
type ActionContext struct {
	// Site is the AdminSite handling the request.
	Site *AdminSite
	// DB is the ORM database handle.
	DB *orm.DB
	// Model is the registered model descriptor.
	Model *orm.Model
	// SelectAcross reports whether the user selected all rows across pages.
	SelectAcross bool
	// Objects loads all selected model instances (each element is *T).
	Objects func(ctx context.Context) ([]any, error)
	// Count returns the number of selected rows.
	Count func(ctx context.Context) (int64, error)
	// PKs returns the primary keys of all selected rows.
	PKs func(ctx context.Context) ([]any, error)
	// Delete deletes all selected rows through the ORM and returns the deleted count.
	Delete func(ctx context.Context) (int64, error)
}

// SaveContext gives the SaveModel and SaveRelated hooks the object that a
// valid add or change form filled, and the default saves.
type SaveContext struct {
	// Site is the AdminSite handling the request.
	Site *AdminSite
	// DB is the ORM database handle.
	DB *orm.DB
	// Model is the registered model descriptor.
	Model *orm.Model
	// Obj is the *T that the form filled. After Save it holds its primary
	// key.
	Obj any
	// Form is the valid model form.
	Form *forms.Form
	// Change is false for a new object, from the add page or "Save as new",
	// and true when the change page saves it (Django's change argument).
	Change bool
	// Save writes Obj to the database: an INSERT when Change is false and
	// an UPDATE otherwise (Django's ModelAdmin.save_model).
	Save func(ctx context.Context) error
	// SaveRelated saves the many-to-many fields of the form and the inline
	// formsets (Django's ModelAdmin.save_related).
	SaveRelated func(ctx context.Context) error
}

// DeleteContext gives the DeleteModel hook the object that the delete page
// deletes, and the default deletion.
type DeleteContext struct {
	// Site is the AdminSite handling the request.
	Site *AdminSite
	// DB is the ORM database handle.
	DB *orm.DB
	// Model is the registered model descriptor.
	Model *orm.Model
	// Obj is the *T to delete.
	Obj any
	// Delete deletes Obj and the rows that cascade from it (Django's
	// ModelAdmin.delete_model).
	Delete func(ctx context.Context) error
}

// Action is a bulk action selectable from the changelist action bar (Django's
// ModelAdmin.actions entry).
type Action struct {
	// Name is the internal identifier submitted in the "action" form field.
	Name string
	// Description is the option label shown in the dropdown. Supports Django's
	// "%(verbose_name)s" and "%(verbose_name_plural)s" placeholders. When empty,
	// derived by capitalizing Name with underscores replaced by spaces.
	Description string
	// Permissions lists permission names ("add", "change", "delete", "view", or
	// a custom permission name). When non-empty, the user must hold at least one
	// listed permission to see or execute the action.
	Permissions []string
	// Handler executes the action. Return handled=true if the handler wrote the
	// HTTP response itself (e.g. a confirmation page, redirect, or download).
	// Return handled=false to redirect back to the changelist URL preserving
	// the query string.
	Handler func(w http.ResponseWriter, r *http.Request, act *ActionContext) (handled bool, err error)
}

// Fieldset is one group of fields on the add and change forms, an entry of
// Django's ModelAdmin.fieldsets.
type Fieldset struct {
	// Name is the heading of the group; empty shows none.
	Name string
	// Fields lists the field names: each entry is a name or a []string of
	// names shown on one line.
	Fields []any
	// Classes are CSS classes of the group: "collapse" folds it while none
	// of its fields has an error, "wide" gives it more room.
	Classes []string
	// Description is shown under the heading. It is HTML and not escaped.
	Description string
}

// RadioStyle lays out the radio buttons of a field of ModelAdmin.RadioFields.
type RadioStyle int

// The layouts of RadioFields, Django's admin.HORIZONTAL and admin.VERTICAL.
const (
	// Horizontal puts the radio buttons side by side.
	Horizontal RadioStyle = 1
	// Vertical stacks the radio buttons.
	Vertical RadioStyle = 2
)

// ShowFacets controls the counts next to the choices of the changelist
// filters, Django's admin.ShowFacets.
type ShowFacets int

// The ShowFacets values. The zero value is ShowFacetsAllow, Django's default.
const (
	// ShowFacetsAllow shows the counts when the query string has _facets,
	// and links "Show counts" and "Hide counts" to toggle them.
	ShowFacetsAllow ShowFacets = iota
	// ShowFacetsAlways always shows the counts.
	ShowFacetsAlways
	// ShowFacetsNever never shows the counts.
	ShowFacetsNever
)

// FieldClass names a Django model field class, a key of
// ModelAdmin.FormfieldOverrides.
type FieldClass int

// The model field classes of FormfieldOverrides, Django's models.CharField and
// the like. A model field has the class of its kind and the classes that the
// Django class derives from, such as SlugField, CharField and Field.
const (
	Field FieldClass = iota + 1
	BooleanField
	CharField
	SlugField
	EmailField
	URLField
	TextField
	DateField
	DateTimeField
	TimeField
	DurationField
	DecimalField
	FloatField
	IntegerField
	SmallIntegerField
	BigIntegerField
	PositiveIntegerField
	PositiveSmallIntegerField
	PositiveBigIntegerField
	UUIDField
	JSONField
	BinaryField
	GenericIPAddressField
	ForeignKey
	OneToOneField
	ManyToManyField
)

// FormfieldOverride sets the form field of the model fields of a class, the
// keyword arguments of an entry of Django's formfield_overrides.
type FormfieldOverride struct {
	// Widget is the widget of the form field. For a foreign key or a
	// many-to-many field it wins over AutocompleteFields, RawIDFields,
	// RadioFields, FilterHorizontal and FilterVertical.
	Widget forms.Widget
	// EmptyLabel is the label of the empty option of a foreign key.
	EmptyLabel *string
}

// DeletedObjects holds what the delete page and the delete_selected
// confirmation page show (returned by ModelAdmin.GetDeletedObjects), Django's
// get_deleted_objects result.
type DeletedObjects struct {
	// Items lists the objects to delete, each with the objects that its
	// deletion cascades to.
	Items []DeletedObject
	// ModelCount maps each affected model's plural verbose name to its count.
	ModelCount []DeletedModelCount
	// PermsNeeded lists verbose names of models the user lacks permission to
	// delete, as plain text.
	PermsNeeded []string
	// Protected lists the protected objects as HTML.
	Protected []string
}

// DeletedObject is one object of DeletedObjects.Items.
type DeletedObject struct {
	// HTML is the object as HTML, as "Plot: <a href="...">World Domination</a>".
	HTML string
	// Children lists the objects deleted because of this one.
	Children []DeletedObject
}

// DeletedModelCount is one entry in the delete confirmation Summary list.
type DeletedModelCount struct {
	// Name is the model's plural verbose name (capitalized in the template).
	Name string
	// Count is the number of rows of that model that would be deleted.
	Count int
}

// ModelAdmin is the per-model admin customization surface. A zero value is a
// valid configuration that lists each object by its label, edits every field,
// orders by Meta.Ordering or -pk, enables delete_selected, and checks Django's
// default auth permissions (<app>.add_<model>, etc.).
type ModelAdmin struct {
	// ListDisplay names the columns shown on the changelist (model struct field
	// names, snake_case attribute names, "__str__", "__"-spanning relation paths,
	// keys in DisplayColumns, or model methods). An empty slice shows the
	// object's label, Django's default ("__str__",). A model method is
	// exported, takes no arguments, and returns a value, or a value and an
	// error; a name refers to it by its Go name or in snake_case, as a
	// template does: ModelYear is model_year. An error fails the page, and
	// orm.ErrDoesNotExist shows the empty value.
	ListDisplay []string
	// ListDisplayLinks names the columns in ListDisplay that link to each row's
	// change page. nil defaults to the first column; a non-nil empty slice
	// ([]string{}) disables links on all columns (Django's list_display_links=None).
	ListDisplayLinks []string
	// ListEditable names the fields of ListDisplay that the changelist edits
	// in place (Django's list_editable): each row shows a form with these
	// fields and the page a Save button that saves the changed rows. A field
	// cannot also be a link, and a popup edits nothing.
	ListEditable []string
	// ChangelistFormSet customizes the formset of ListEditable for r (Django's
	// get_changelist_formset with a formset class of its own). Add a check
	// across the rows with fs.CleanWith: the cleaned value of the primary key
	// of each form is its row.
	ChangelistFormSet func(r *http.Request, fs *forms.FormSet)
	// DisplayColumns defines custom computed columns referenced by name in
	// ListDisplay.
	DisplayColumns map[string]DisplayColumn
	// ListFilter configures the right-hand filter sidebar. Each element may be
	// a string (field name or "__" relation path), a FieldFilter, or a
	// SimpleListFilter (or *SimpleListFilter).
	ListFilter []any
	// DateHierarchy names a date or datetime field, or a "__" path to one
	// ("question__posted"), that the changelist drills into above the rows
	// (Django's date_hierarchy): links to the years that have rows, then to
	// the months of a year, then to the days of a month. A datetime is read
	// in UTC.
	DateHierarchy string
	// ListPerPage is the maximum number of items per changelist page (default 100).
	ListPerPage int
	// ListMaxShowAll is the maximum number of items shown when "Show all" is
	// clicked (default 200).
	ListMaxShowAll int
	// ShowFullResultCount controls whether the total unfiltered count is
	// queried and displayed in the search bar (default true when nil).
	ShowFullResultCount *bool
	// ShowFacets controls the counts next to the filter choices. Each
	// counted filter runs one more query, so ShowFacetsNever suits a large
	// table.
	ShowFacets ShowFacets
	// ListSelectRelated names the relations that the changelist query joins
	// (Django's list_select_related as a list). nil joins the foreign keys
	// that the columns show, as Django's default does, and a non-nil empty
	// slice joins none. A relation that a column shows and the query does
	// not join is loaded in one more query, where Django loads it once per
	// row.
	ListSelectRelated []string
	// SortableBy restricts which ListDisplay columns are sortable. nil allows
	// sorting on every sortable column; a non-nil empty slice disables sorting.
	SortableBy []string
	// Ordering is the default OrderBy applied to the changelist
	// (e.g. "-id"); an empty slice falls back to Meta.Ordering, then -pk.
	Ordering []string
	// SearchFields names the fields searched by the changelist search box,
	// supporting Django's prefixes ("^" istartswith, "=" iexact, "@" search),
	// explicit lookups ("__exact", "__iexact"), and "__"-spanning relations.
	SearchFields []string
	// SearchHelpText is displayed below the changelist search input when non-empty.
	SearchHelpText string
	// EmptyValueDisplay overrides AdminSite.EmptyValueDisplay for this model.
	EmptyValueDisplay string
	// PreserveFilters preserves changelist query parameters across edits when
	// true (default true when nil).
	PreserveFilters *bool
	// ReadonlyFields names what the add and change forms show as values in
	// place of inputs (Django's readonly_fields): model fields, whose stored
	// values a save keeps, keys of DisplayColumns, and model methods, named
	// as in ListDisplay. A model method that returns an error shows the
	// empty value.
	ReadonlyFields []string
	// ExcludeFields names fields omitted from the add and change forms. A
	// list that is not empty replaces the forms.WithExclude of FormOptions,
	// as Django's ModelAdmin.exclude replaces the Meta.exclude of the form.
	ExcludeFields []string
	// FormOptions customize the model form of the add and change pages, the
	// counterpart of a ModelAdmin.form with Meta options and declared
	// fields: forms.WithWidget, WithLabel, WithHelpText, WithFormField,
	// WithExclude and the like. They apply after the admin's own widgets, so
	// they win. The fields of the form come from Fields, Fieldsets and
	// ExcludeFields, not from a forms.WithFields here.
	FormOptions []forms.ModelFormOption
	// GetFormOptions returns the options of the model form for r and obj, nil
	// on the add page, in place of FormOptions (Django's get_form, which can
	// pick a form class per page, as the admin of users does).
	GetFormOptions func(r *http.Request, obj any) []forms.ModelFormOption
	// FormfieldOverrides sets the form fields of the add and change forms by
	// model field class (Django's formfield_overrides), such as
	// {admin.TextField: {Widget: forms.Textarea{Attrs: forms.Attrs{"rows": "4"}}}}.
	// An entry merges over the admin's default for its class. A field takes
	// the entry or the default of the first of its classes that has one, so
	// an entry for CharField applies to a SlugField but not to an EmailField,
	// which has a default of its own. A foreign key, a one-to-one field and a
	// many-to-many field take only the entry of their own class, and a field
	// with choices takes none. FormOptions win over the entries.
	FormfieldOverrides map[FieldClass]FormfieldOverride
	// Fields lays out the add and change forms (Django's fields). Each entry
	// is a field name or a []string of names shown on one line. Empty shows
	// the form fields in model order, then the ReadonlyFields.
	Fields []any
	// Fieldsets groups the add and change forms into fieldsets (Django's
	// fieldsets). Set Fields or Fieldsets, not both.
	Fieldsets []Fieldset
	// FilterHorizontal names many-to-many fields whose list is a
	// FilteredSelectMultiple with the two lists side by side (Django's
	// filter_horizontal).
	FilterHorizontal []string
	// FilterVertical names many-to-many fields whose list is a
	// FilteredSelectMultiple with the two lists stacked (Django's
	// filter_vertical).
	FilterVertical []string
	// PrepopulatedFields maps a field to the fields that a script fills it
	// from as the user types (Django's prepopulated_fields), such as
	// {"slug": {"title"}}: the script joins their values and turns them
	// into a slug. The field must not be a date and time, a foreign key or
	// a many-to-many field.
	PrepopulatedFields map[string][]string
	// RadioFields names foreign keys and fields with choices shown as radio
	// buttons in place of a select (Django's radio_fields), laid out
	// Horizontal or Vertical. Only a field that can be blank gets the empty
	// choice, labeled "None". AutocompleteFields, RawIDFields and the
	// widget of FormfieldOverrides win over it.
	RadioFields map[string]RadioStyle
	// RawIDFields names foreign keys and many-to-many fields edited as a text
	// input for the primary keys with a link to the related changelist
	// (Django's raw_id_fields). The form loads only the related rows it
	// shows or receives, not the whole table.
	RawIDFields []string
	// AutocompleteFields names foreign keys and many-to-many fields edited as
	// a select that a script fills from the site's autocomplete view
	// (Django's autocomplete_fields). The admin of the related model needs
	// SearchFields. The form loads only the related rows it shows or
	// receives.
	AutocompleteFields []string
	// GetAutocompleteFields returns the autocomplete fields for r in place of
	// AutocompleteFields (Django's get_autocomplete_fields).
	GetAutocompleteFields func(r *http.Request) []string
	// Inlines edit the rows of other models that point at the edited row on
	// its add and change pages (Django's inlines), made by TabularInline or
	// StackedInline.
	Inlines []Inline
	// GetInlines returns the inlines for r and obj, nil on the add page, in
	// place of Inlines (Django's get_inlines).
	GetInlines func(r *http.Request, obj any) []Inline
	// Media adds stylesheets and scripts to the add and change pages
	// (Django's ModelAdmin Media class).
	Media forms.Media
	// SaveAs shows a "Save as new" button on the change page in place of
	// "Save and add another" (Django's save_as). The button saves the form
	// and the inlines as a new row and needs the add permission.
	SaveAs bool
	// SaveAsContinue redirects to the change page of the new row after
	// "Save as new" (Django's save_as_continue, default true when nil); false
	// redirects to the changelist.
	SaveAsContinue *bool
	// SaveOnTop shows the save buttons above the form as well as below it
	// (Django's save_on_top).
	SaveOnTop bool
	// ViewOnSite shows a "View on site" link on the change page of an object
	// with a GetAbsoluteURL method (Django's view_on_site, default true when
	// nil). The link points at the view_on_site URL of the site, which
	// redirects to the URL of the object. False hides the link.
	ViewOnSite *bool
	// ViewOnSiteURL returns the URL of the "View on site" link of obj in
	// place of the view_on_site URL, Django's callable view_on_site. An empty
	// URL hides the link.
	ViewOnSiteURL func(obj any) string

	// AddFormTemplate is the template of the add page (Django's
	// add_form_template). Empty, the add page uses the change form's.
	AddFormTemplate string
	// ChangeFormTemplate is the template of the add and change pages
	// (Django's change_form_template). Empty, it is the first that exists of
	// "admin/<app>/<model>/change_form.html", "admin/<app>/change_form.html"
	// and "admin/change_form.html". The other templates below default the
	// same way.
	ChangeFormTemplate string
	// ChangeListTemplate is the template of the changelist (Django's
	// change_list_template, default change_list.html).
	ChangeListTemplate string
	// DeleteConfirmationTemplate is the template of the delete page (Django's
	// delete_confirmation_template, default delete_confirmation.html).
	DeleteConfirmationTemplate string
	// DeleteSelectedConfirmationTemplate is the template of the confirmation
	// page of the "delete selected" action (Django's
	// delete_selected_confirmation_template, default
	// delete_selected_confirmation.html).
	DeleteSelectedConfirmationTemplate string
	// ObjectHistoryTemplate is the template of the history page (Django's
	// object_history_template, default object_history.html).
	ObjectHistoryTemplate string
	// PopupResponseTemplate is the template of the page that closes a popup
	// window after a save or delete (Django's popup_response_template,
	// default popup_response.html).
	PopupResponseTemplate string

	// Actions registers custom bulk actions on this ModelAdmin (appended after
	// site-wide actions such as delete_selected; an action with the same Name
	// overrides the site-wide action).
	Actions []Action
	// DisableActions disables all actions (including site-wide delete_selected)
	// on this ModelAdmin (Django's actions = None).
	DisableActions bool
	// ActionsOnTop renders the action bar above the result table (default true when nil).
	ActionsOnTop *bool
	// ActionsOnBottom renders the action bar below the result table (default false).
	ActionsOnBottom bool
	// ActionsSelectionCounter shows the "0 of N selected" counter (default true when nil).
	ActionsSelectionCounter *bool
	// DeleteConfirmationMaxDisplay caps the number of items listed on the
	// delete_selected confirmation page (nil means unlimited; 0 hides the list).
	DeleteConfirmationMaxDisplay *int

	// GetListDisplay dynamically returns the changelist columns for r.
	GetListDisplay func(r *http.Request) []string
	// GetListDisplayLinks dynamically returns the linked columns for r.
	GetListDisplayLinks func(r *http.Request, listDisplay []string) []string
	// GetListFilter dynamically returns the list_filter entries for r.
	GetListFilter func(r *http.Request) []any
	// GetListSelectRelated returns the joined relations for r in place of
	// ListSelectRelated (Django's get_list_select_related).
	GetListSelectRelated func(r *http.Request) []string
	// GetSearchFields dynamically returns the search_fields for r.
	GetSearchFields func(r *http.Request) []string
	// GetOrdering dynamically returns the default ordering for r.
	GetOrdering func(r *http.Request) []string
	// GetFields returns the layout of the form for r and obj, nil on the add
	// page (Django's get_fields).
	GetFields func(r *http.Request, obj any) []any
	// GetFieldsets returns the fieldsets of the form for r and obj, nil on
	// the add page (Django's get_fieldsets).
	GetFieldsets func(r *http.Request, obj any) []Fieldset
	// GetReadonlyFields returns the read-only fields for r and obj, nil on
	// the add page (Django's get_readonly_fields).
	GetReadonlyFields func(r *http.Request, obj any) []string
	// GetPrepopulatedFields returns the prepopulated fields for r and obj,
	// nil on the add page, in place of PrepopulatedFields (Django's
	// get_prepopulated_fields).
	GetPrepopulatedFields func(r *http.Request, obj any) map[string][]string
	// GetExclude returns the fields that the form leaves out for r and obj,
	// nil on the add page (Django's get_exclude). It replaces ExcludeFields,
	// and a result that is not nil replaces the forms.WithExclude of the form
	// options.
	GetExclude func(r *http.Request, obj any) []string
	// GetQuerysetFilter returns an extra orm.Cond applied to the base
	// changelist queryset for r (the Go analog of overriding ModelAdmin.get_queryset).
	GetQuerysetFilter func(r *http.Request) orm.Cond
	// GetActions filters or customizes the resolved action list for r.
	GetActions func(r *http.Request, actions []Action) []Action
	// SaveModel replaces how the add and change pages, and the changelist
	// rows of ListEditable, save the object (Django's save_model). Call
	// sc.Save to keep the default save. The hook runs in the transaction of
	// the save, which r.Context() carries.
	SaveModel func(r *http.Request, sc *SaveContext) error
	// SaveRelated replaces how the add and change pages, and the changelist
	// rows of ListEditable, save the many-to-many fields and the inlines
	// after the object (Django's save_related). Call sc.SaveRelated to keep
	// the default. The hook runs in the transaction of the save, which
	// r.Context() carries.
	SaveRelated func(r *http.Request, sc *SaveContext) error
	// DeleteModel replaces how the delete page deletes the object (Django's
	// delete_model). Call dc.Delete to keep the default deletion. The hook
	// runs in the transaction of the deletion, which r.Context() carries.
	DeleteModel func(r *http.Request, dc *DeleteContext) error
	// DeleteQueryset overrides how delete_selected deletes the selected rows.
	DeleteQueryset func(r *http.Request, act *ActionContext) error
	// GetDeletedObjects overrides how the delete page and delete_selected
	// compute the confirmation summary, deletable list, missing permissions,
	// and protected objects (Django's get_deleted_objects).
	GetDeletedObjects func(r *http.Request, objs []any) (*DeletedObjects, error)
	// ExtraContext returns values to add to the template context of a page,
	// Django's extra_context argument of the views. view is "changelist",
	// "add", "change", "delete" or "history". The values replace those of
	// the page, except on the add and change pages: there the permission
	// flags, the URLs and the buttons win, as in Django's
	// render_change_form. A false "show_save", "show_save_and_continue",
	// "show_save_and_add_another" or "show_delete" hides that button of the
	// add and change pages.
	ExtraContext func(r *http.Request, view string) map[string]any

	// HasAddPermission overrides the default "<app>.add_<model>" check
	// (Django's ModelAdmin.has_add_permission).
	HasAddPermission func(r *http.Request) bool
	// HasChangePermission overrides the default "<app>.change_<model>" check
	// (Django's ModelAdmin.has_change_permission). obj is nil for model-level
	// checks and *T for object-level checks.
	HasChangePermission func(r *http.Request, obj any) bool
	// HasDeletePermission overrides the default "<app>.delete_<model>" check
	// (Django's ModelAdmin.has_delete_permission). obj is nil for model-level
	// checks and *T for object-level checks.
	HasDeletePermission func(r *http.Request, obj any) bool
	// HasViewPermission overrides the default "<app>.view_<model>" or
	// "<app>.change_<model>" check (Django's ModelAdmin.has_view_permission).
	// obj is nil for model-level checks and *T for object-level checks.
	HasViewPermission func(r *http.Request, obj any) bool
	// HasModulePermission overrides the default app-level module permission
	// check (Django's ModelAdmin.has_module_permission).
	HasModulePermission func(r *http.Request) bool
	// CustomPermissions maps custom action permission names (used in
	// Action.Permissions) to request permission predicates.
	CustomPermissions map[string]func(r *http.Request) bool

	// hooks hold the behavior of the admins of the auth models that the
	// fields above do not express.
	hooks *adminHooks
}
