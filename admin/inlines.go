package admin

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"reflect"
	"slices"
	"strings"

	"github.com/oliverhaas/djangogo/checks"
	"github.com/oliverhaas/djangogo/forms"
	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/templates"
)

// defaultInlineExtra is the number of extra forms of an inline, Django's
// InlineModelAdmin.extra.
const defaultInlineExtra = 3

// InlineModelAdmin configures the inline editing of the rows of a model that
// point at the edited row through a foreign key, Django's InlineModelAdmin.
// TabularInline and StackedInline turn it into an Inline for
// ModelAdmin.Inlines. The fields shared with ModelAdmin mean the same for the
// forms of the inline. The obj argument of the hooks is the edited row of the
// parent model, nil on the add page.
type InlineModelAdmin struct {
	// FKName names the foreign key to the parent model, Django's fk_name. It
	// is needed when the model has more than one.
	FKName string
	// Extra is the number of empty forms shown for new rows (default 3).
	Extra *int
	// MinNum is the number of forms shown at least, Django's min_num.
	MinNum int
	// MaxNum is the number of forms shown at most, Django's max_num. Nil
	// leaves the formset's default, 1000, or 1 for a unique foreign key.
	MaxNum *int
	// CanDelete shows the delete boxes of the rows (default true).
	CanDelete *bool
	// ShowChangeLink adds a link to the change page of each row when its
	// model is registered on the site, Django's show_change_link.
	ShowChangeLink bool
	// ViewOnSite shows a "View on site" link on each row, as
	// ModelAdmin.ViewOnSite does.
	ViewOnSite *bool
	// ViewOnSiteURL returns the URL of the "View on site" link of the row
	// obj, as ModelAdmin.ViewOnSiteURL does.
	ViewOnSiteURL func(obj any) string
	// VerboseName overrides the singular name of the model in the inline.
	VerboseName string
	// VerboseNamePlural overrides the plural name. It defaults to
	// VerboseName with an "s" when VerboseName is set, else the model's.
	VerboseNamePlural string
	// Classes are CSS classes of the inline's fieldset: "collapse" folds it
	// while none of its forms has an error.
	Classes []string
	// Fields lays out each form, as ModelAdmin.Fields does.
	Fields []any
	// Fieldsets groups each form into fieldsets, as ModelAdmin.Fieldsets
	// does.
	Fieldsets []Fieldset
	// ReadonlyFields names what each form shows as values, as
	// ModelAdmin.ReadonlyFields does.
	ReadonlyFields []string
	// ExcludeFields names fields left out of the forms. It replaces the
	// forms.WithExclude of FormOptions, as ModelAdmin.ExcludeFields does.
	ExcludeFields []string
	// FormOptions customize the model form of the rows, as
	// ModelAdmin.FormOptions does.
	FormOptions []forms.ModelFormOption
	// FormfieldOverrides sets the form fields by model field class, as
	// ModelAdmin.FormfieldOverrides does.
	FormfieldOverrides map[FieldClass]FormfieldOverride
	// RawIDFields names relation fields edited as raw primary keys.
	RawIDFields []string
	// AutocompleteFields names relation fields edited with autocomplete.
	AutocompleteFields []string
	// FilterHorizontal names many-to-many fields with side-by-side lists.
	FilterHorizontal []string
	// FilterVertical names many-to-many fields with stacked lists.
	FilterVertical []string
	// PrepopulatedFields maps a field to the fields that fill it in the
	// forms that add a row, as ModelAdmin.PrepopulatedFields does.
	PrepopulatedFields map[string][]string
	// RadioFields shows foreign keys and fields with choices as radio
	// buttons, as ModelAdmin.RadioFields does.
	RadioFields map[string]RadioStyle
	// DisplayColumns defines computed values that ReadonlyFields can name.
	DisplayColumns map[string]DisplayColumn
	// Ordering orders the rows of the inline.
	Ordering []string
	// EmptyValueDisplay overrides the site's empty-value placeholder.
	EmptyValueDisplay string
	// DeleteConfirmationMaxDisplay caps the number of protected objects
	// that the error of a blocked deletion lists (nil means no cap; 0 lists
	// none).
	DeleteConfirmationMaxDisplay *int
	// Media adds stylesheets and scripts to the page, Django's Media class
	// of an inline.
	Media forms.Media

	// GetExtra returns the number of extra forms for r and obj in place of
	// Extra, Django's get_extra.
	GetExtra func(r *http.Request, obj any) int
	// GetMinNum returns the minimum number of forms in place of MinNum,
	// Django's get_min_num.
	GetMinNum func(r *http.Request, obj any) int
	// GetMaxNum returns the maximum number of forms in place of MaxNum,
	// Django's get_max_num.
	GetMaxNum func(r *http.Request, obj any) int
	// GetFields returns the layout of the forms, Django's get_fields.
	GetFields func(r *http.Request, obj any) []any
	// GetFieldsets returns the fieldsets of the forms, Django's
	// get_fieldsets.
	GetFieldsets func(r *http.Request, obj any) []Fieldset
	// GetReadonlyFields returns the read-only fields, Django's
	// get_readonly_fields.
	GetReadonlyFields func(r *http.Request, obj any) []string
	// GetPrepopulatedFields returns the prepopulated fields for the parent
	// row obj, Django's get_prepopulated_fields.
	GetPrepopulatedFields func(r *http.Request, obj any) map[string][]string
	// GetExclude returns the fields that the forms leave out, Django's
	// get_exclude. It replaces ExcludeFields.
	GetExclude func(r *http.Request, obj any) []string

	// HasAddPermission overrides the "<app>.add_<model>" check.
	HasAddPermission func(r *http.Request, obj any) bool
	// HasChangePermission overrides the "<app>.change_<model>" check.
	HasChangePermission func(r *http.Request, obj any) bool
	// HasDeletePermission overrides the "<app>.delete_<model>" check.
	HasDeletePermission func(r *http.Request, obj any) bool
	// HasViewPermission overrides the "<app>.view_<model>" or
	// "<app>.change_<model>" check.
	HasViewPermission func(r *http.Request, obj any) bool
}

// Inline is an inline of a ModelAdmin, made by TabularInline or
// StackedInline for a parent model P.
type Inline struct {
	cfg      InlineModelAdmin
	template string
	parent   reflect.Type
	// resolve builds the admin of the inline on site. When the model of
	// the inline is registered but its foreign key to the parent is not
	// usable, it returns the admin without a foreign key together with the
	// error, so that the checks still see the options of the inline.
	resolve func(site *AdminSite) (*inlineAdmin, error)
}

// TabularInline returns an inline that edits the rows of T that point at a
// row of P as the rows of a table, Django's TabularInline.
func TabularInline[P, T any](cfg InlineModelAdmin) Inline {
	return newInline[P, T](cfg, "admin/edit_inline/tabular.html")
}

// StackedInline returns an inline that edits the rows of T that point at a
// row of P one below the other, each laid out as the change form, Django's
// StackedInline.
func StackedInline[P, T any](cfg InlineModelAdmin) Inline {
	return newInline[P, T](cfg, "admin/edit_inline/stacked.html")
}

// newInline returns the inline of T in P drawn with template.
func newInline[P, T any](cfg InlineModelAdmin, template string) Inline {
	return Inline{
		cfg:      cfg,
		template: template,
		parent:   reflect.TypeFor[P](),
		resolve: func(site *AdminSite) (*inlineAdmin, error) {
			db := site.db
			var zero T
			m, ok := db.Registry().ModelOf(zero)
			if !ok {
				return nil, fmt.Errorf("admin: inline: no ORM model registered for %T", zero)
			}
			in := &inlineAdmin{
				cfg:      cfg,
				template: template,
				entry: &entry{
					app:   m.App(),
					slug:  strings.ToLower(m.Name()),
					model: m,
					admin: cfg.modelAdmin(),
					ops:   buildOps[T](db, m),
				},
			}
			probe := forms.NewInlineFormSet[P, T](db, cfg.FKName)
			if err := probe.Err(); err != nil {
				return in, err
			}
			in.fk = probe.FK()
			in.newFormSet = func(parent any, ordering []string, none, saveAsNew bool, opts []forms.ModelFormOption) inlineSet {
				fs := forms.NewInlineFormSet[P, T](db, cfg.FKName, opts...)
				qs := orm.Query[T](db)
				if len(ordering) > 0 {
					qs = qs.OrderBy(ordering...)
				}
				if none {
					qs = qs.Filter("pk__in", []any{})
				}
				fs.QuerySet = qs
				p, _ := parent.(*P)
				set := fs.WithInstance(p)
				set.SaveAsNew = saveAsNew
				return &typedInlineSet[P, T]{fs: set}
			}
			return in, nil
		},
	}
}

// modelAdmin returns the ModelAdmin with the options of the inline that the
// add and change forms share, so that the helpers of the forms serve both.
func (c InlineModelAdmin) modelAdmin() ModelAdmin {
	return ModelAdmin{
		Fields:                       c.Fields,
		Fieldsets:                    c.Fieldsets,
		ReadonlyFields:               c.ReadonlyFields,
		ExcludeFields:                c.ExcludeFields,
		FormOptions:                  c.FormOptions,
		FormfieldOverrides:           c.FormfieldOverrides,
		RawIDFields:                  c.RawIDFields,
		AutocompleteFields:           c.AutocompleteFields,
		FilterHorizontal:             c.FilterHorizontal,
		FilterVertical:               c.FilterVertical,
		PrepopulatedFields:           c.PrepopulatedFields,
		RadioFields:                  c.RadioFields,
		DisplayColumns:               c.DisplayColumns,
		Ordering:                     c.Ordering,
		EmptyValueDisplay:            c.EmptyValueDisplay,
		DeleteConfirmationMaxDisplay: c.DeleteConfirmationMaxDisplay,
		GetFields:                    c.GetFields,
		GetFieldsets:                 c.GetFieldsets,
		GetReadonlyFields:            c.GetReadonlyFields,
		GetPrepopulatedFields:        c.GetPrepopulatedFields,
		GetExclude:                   c.GetExclude,
	}
}

// inlineAdmin is an Inline resolved against a site: the model of its rows
// as an entry, so that the helpers of the change form serve its forms, and
// the constructor of its formset.
type inlineAdmin struct {
	cfg      InlineModelAdmin
	template string
	fk       *orm.Field
	entry    *entry
	// newFormSet returns the formset of the rows of parent, ordered by
	// ordering, empty when none is true. saveAsNew makes every form add a
	// row, Django's save_as_new.
	newFormSet func(parent any, ordering []string, none, saveAsNew bool, opts []forms.ModelFormOption) inlineSet
}

// inlineSet is the type-independent view of a forms.InlineFormSet.
type inlineSet interface {
	// base returns the formset.
	base() *forms.FormSet
	// bind binds the formset to data.
	bind(data url.Values)
	// instances returns the instance of each form, aligned with Forms.
	instances() []any
	// save saves the rows of the forms.
	save(ctx context.Context) error
	// changes returns the rows that the last save added, changed and
	// deleted.
	changes() (added []any, changed []changedRow, deleted []any)
}

// changedRow is a row that an inline save changed, with the names of the
// changed fields.
type changedRow struct {
	obj    any
	fields []string
}

// typedInlineSet is the inlineSet of an InlineFormSet of T in P.
type typedInlineSet[P, T any] struct {
	fs *forms.InlineFormSet[P, T]
}

// base returns the formset.
func (t *typedInlineSet[P, T]) base() *forms.FormSet { return t.fs.FormSet }

// bind binds the formset to data.
func (t *typedInlineSet[P, T]) bind(data url.Values) { t.fs = t.fs.Bind(data) }

// instances returns the instance of each form, aligned with Forms.
func (t *typedInlineSet[P, T]) instances() []any {
	mfs := t.fs.ModelForms()
	out := make([]any, len(mfs))
	for i, mf := range mfs {
		out[i] = mf.Instance()
	}
	return out
}

// save saves the rows of the forms.
func (t *typedInlineSet[P, T]) save(ctx context.Context) error {
	_, err := t.fs.Save(ctx)
	return err
}

// changes returns the rows that the last save added, changed and deleted.
func (t *typedInlineSet[P, T]) changes() (added []any, changed []changedRow, deleted []any) {
	for _, o := range t.fs.NewObjects() {
		added = append(added, o)
	}
	for _, c := range t.fs.ChangedObjects() {
		changed = append(changed, changedRow{obj: c.Object, fields: c.Fields})
	}
	for _, o := range t.fs.DeletedObjects() {
		deleted = append(deleted, o)
	}
	return added, changed, deleted
}

// hasAdd reports whether r may add rows of the inline, Django's
// InlineModelAdmin.has_add_permission.
func (in *inlineAdmin) hasAdd(s *AdminSite, r *http.Request, obj any) bool {
	if in.cfg.HasAddPermission != nil {
		return in.cfg.HasAddPermission(r, obj)
	}
	return s.userHasPerm(r, permName(in.entry.model, "add"))
}

// hasChange reports whether r may change rows of the inline.
func (in *inlineAdmin) hasChange(s *AdminSite, r *http.Request, obj any) bool {
	if in.cfg.HasChangePermission != nil {
		return in.cfg.HasChangePermission(r, obj)
	}
	return s.userHasPerm(r, permName(in.entry.model, "change"))
}

// hasDelete reports whether r may delete rows of the inline.
func (in *inlineAdmin) hasDelete(s *AdminSite, r *http.Request, obj any) bool {
	if in.cfg.HasDeletePermission != nil {
		return in.cfg.HasDeletePermission(r, obj)
	}
	return s.userHasPerm(r, permName(in.entry.model, "delete"))
}

// hasView reports whether r may view rows of the inline; the change
// permission implies it.
func (in *inlineAdmin) hasView(s *AdminSite, r *http.Request, obj any) bool {
	if in.cfg.HasViewPermission != nil {
		return in.cfg.HasViewPermission(r, obj)
	}
	return s.userHasPerm(r, permName(in.entry.model, "view")) ||
		s.userHasPerm(r, permName(in.entry.model, "change"))
}

// extra returns the number of extra forms, Django's get_extra.
func (in *inlineAdmin) extra(r *http.Request, obj any) int {
	if in.cfg.GetExtra != nil {
		return in.cfg.GetExtra(r, obj)
	}
	if in.cfg.Extra != nil {
		return *in.cfg.Extra
	}
	return defaultInlineExtra
}

// minNum returns the minimum number of forms, Django's get_min_num.
func (in *inlineAdmin) minNum(r *http.Request, obj any) int {
	if in.cfg.GetMinNum != nil {
		return in.cfg.GetMinNum(r, obj)
	}
	return in.cfg.MinNum
}

// maxNum returns the maximum number of forms, Django's get_max_num, and
// false when the inline leaves the default.
func (in *inlineAdmin) maxNum(r *http.Request, obj any) (int, bool) {
	if in.cfg.GetMaxNum != nil {
		return in.cfg.GetMaxNum(r, obj), true
	}
	if in.cfg.MaxNum != nil {
		return *in.cfg.MaxNum, true
	}
	return 0, false
}

// verboseNames returns the singular and plural names of the inline's model.
func (in *inlineAdmin) verboseNames() (singular, plural string) {
	singular, plural = modelVerboseNames(in.entry.model)
	switch {
	case in.cfg.VerboseNamePlural != "":
		plural = in.cfg.VerboseNamePlural
	case in.cfg.VerboseName != "":
		plural = in.cfg.VerboseName + "s"
	}
	if in.cfg.VerboseName != "" {
		singular = in.cfg.VerboseName
	}
	return singular, plural
}

// media returns the scripts of the inline, Django's InlineModelAdmin.media:
// jQuery and inlines.js, the scripts of the filtered lists when the inline
// has any, and the inline's own Media.
func (in *inlineAdmin) media(s *AdminSite) forms.Media {
	names := []string{"vendor/jquery/jquery" + s.minified() + ".js", "jquery.init.js", "inlines.js"}
	if len(in.cfg.FilterVertical) > 0 || len(in.cfg.FilterHorizontal) > 0 {
		names = append(names, "SelectBox.js", "SelectFilter2.js")
	}
	return adminJS(names...).Add(in.cfg.Media)
}

// inlineAdmins returns the inlines of e for r and obj, Django's
// get_inline_instances without the permission filter.
func (s *AdminSite) inlineAdmins(r *http.Request, e *entry, obj any) ([]*inlineAdmin, error) {
	inlines := e.admin.Inlines
	if e.admin.GetInlines != nil {
		inlines = e.admin.GetInlines(r, obj)
	}
	out := make([]*inlineAdmin, 0, len(inlines))
	for _, inline := range inlines {
		if inline.resolve == nil {
			return nil, errors.New("admin: an Inline must be made by TabularInline or StackedInline")
		}
		if parent := reflect.TypeOf(e.ops.newPtr()).Elem(); inline.parent != parent {
			return nil, fmt.Errorf("admin: an inline of %v cannot edit %v", inline.parent, parent)
		}
		in, err := inline.resolve(s)
		if err != nil {
			return nil, err
		}
		out = append(out, in)
	}
	return out, nil
}

// inlineFormSet is one inline of an add or change page: its formset with the
// permissions and layout of the page, Django's InlineAdminFormSet.
type inlineFormSet struct {
	in       *inlineAdmin
	set      inlineSet
	sets     []Fieldset
	readonly []string
	// exclude are the names left out of the forms.
	exclude []string

	hasAdd, hasChange, hasDelete, hasView bool
}

// formSet returns the formset.
func (ifs *inlineFormSet) formSet() *forms.FormSet { return ifs.set.base() }

// addForms returns the forms that add a row, those that Django's
// InlineAdminFormSet yields without an original: the extra forms and, with
// the add permission, the empty form.
func (ifs *inlineFormSet) addForms() []*forms.Form {
	fs := ifs.formSet()
	instances := ifs.set.instances()
	initial := fs.InitialFormCount()
	var out []*forms.Form
	for i, form := range fs.Forms() {
		if i < initial && i < len(instances) && instanceHasPK(ifs.in.entry.model, instances[i]) {
			continue
		}
		out = append(out, form)
	}
	if ifs.hasAdd {
		out = append(out, fs.EmptyForm())
	}
	return out
}

// inlineFormSets builds the formsets of the inlines of e for the parent row
// parent, Django's _create_formsets and get_inline_formsets. obj is the
// edited row, nil on the add page, where parent is the new row the form
// fills. data binds the formsets to a submission; nil leaves them unbound.
// An inline the user may neither view, change, add nor delete is left out.
func (s *AdminSite) inlineFormSets(r *http.Request, e *entry, parent, obj any, data url.Values) ([]*inlineFormSet, error) {
	ins, err := s.inlineAdmins(r, e, obj)
	if err != nil || len(ins) == 0 {
		return nil, err
	}
	canEditParent := e.hasAddPermission(s, r)
	if obj != nil {
		canEditParent = e.hasChangePermission(s, r, obj)
	}
	saveAsNew := data.Has(saveAsNewVar)
	prefixes := map[string]int{}
	out := make([]*inlineFormSet, 0, len(ins))
	for _, in := range ins {
		ifs := &inlineFormSet{
			in:        in,
			hasAdd:    in.hasAdd(s, r, obj),
			hasChange: in.hasChange(s, r, obj),
			hasDelete: in.hasDelete(s, r, obj),
			hasView:   in.hasView(s, r, obj),
		}
		if !ifs.hasView && !ifs.hasChange && !ifs.hasAdd && !ifs.hasDelete {
			continue
		}
		if err := s.buildInlineFormSet(r, ifs, parent, obj, prefixes, saveAsNew); err != nil {
			return nil, err
		}
		if !canEditParent {
			// A parent the user may not edit shows its inlines read-only.
			ifs.hasAdd, ifs.hasChange, ifs.hasDelete = false, false, false
			ifs.formSet().Extra, ifs.formSet().MaxNum = 0, 0
		}
		if data != nil {
			bound := data
			if saveAsNew && !ifs.hasAdd {
				// A copy of the parent gets no rows of an inline the user
				// may not add to, as in Django's _create_formsets.
				prefix := ifs.formSet().Prefix
				bound = maps.Clone(data)
				bound.Set(prefix+"-TOTAL_FORMS", "0")
				bound.Set(prefix+"-INITIAL_FORMS", "0")
			}
			ifs.set.bind(bound)
		}
		fs := ifs.formSet()
		fs.SetContext(r.Context())
		for _, form := range fs.Forms() {
			if err := s.loadSelectedChoices(r, in.entry, form); err != nil {
				return nil, err
			}
		}
		if err := fs.Err(); err != nil {
			return nil, err
		}
		out = append(out, ifs)
	}
	return out, nil
}

// buildInlineFormSet makes the unbound formset of ifs for parent, Django's
// InlineModelAdmin.get_formset: the forms of the inline's layout without the
// read-only fields, the admin's widgets, the counts of the inline, and the
// guards of Django's DeleteProtectedModelForm. saveAsNew makes every form add
// a row, for a parent saved as a new row.
func (s *AdminSite) buildInlineFormSet(r *http.Request, ifs *inlineFormSet, parent, obj any, prefixes map[string]int, saveAsNew bool) error {
	in, ie := ifs.in, ifs.in.entry
	ifs.sets = s.fieldsets(r, ie, obj)
	ifs.readonly = ie.readonlyFields(r, obj)
	fields := formNames(ie.model, flattenFieldsets(ifs.sets))
	excluded, own := s.excluded(r, ie, obj)
	ifs.exclude = slices.Concat(excluded, ifs.readonly)
	built := map[string]bool{}
	opts := slices.Concat([]forms.ModelFormOption{
		forms.WithFields(fields...),
		forms.WithFormFieldCallback(ie.formfieldCallback(r, built)),
	}, s.relationWidgets(r, ie), ie.overrideOptions(), in.cfg.FormOptions, []forms.ModelFormOption{excludeOption(ie.model, ifs.exclude, own)})
	ordering := make([]string, len(in.cfg.Ordering))
	for i, o := range in.cfg.Ordering {
		ordering[i] = normalizeFieldPath(ie.model, o)
	}
	ifs.set = in.newFormSet(parent, ordering, !ifs.hasView && !ifs.hasChange, saveAsNew, opts)
	fs := ifs.formSet()
	fs.Extra = in.extra(r, obj)
	fs.MinNum = in.minNum(r, obj)
	if !in.fk.Unique && in.fk.Rel.Kind != orm.RelO2O {
		if n, ok := in.maxNum(r, obj); ok {
			fs.MaxNum = n
		}
		if !ifs.hasAdd {
			fs.MaxNum = 0
		}
	}
	fs.CanDelete = (in.cfg.CanDelete == nil || *in.cfg.CanDelete) && ifs.hasDelete

	prefix := fs.Prefix
	prefixes[prefix]++
	if prefixes[prefix] != 1 || prefix == "" {
		prefix = fmt.Sprintf("%s-%d", prefix, prefixes[prefix])
	}
	fs.Prefix = prefix

	var choices map[string][][2]string
	fs.ConfigureForm = func(form *forms.Form, i int) {
		syncAutocompleteRequired(form)
		s.wrapRelatedWidgets(r, ie, form, built)
		if choices == nil {
			choices = s.relationChoices(r, ie, form, in.fk.Attr)
		}
		for name, c := range choices {
			form.SetChoices(name, c)
		}
		ifs.guardForm(form, i)
	}
	fs.CleanWith(func(fs *forms.FormSet) error { return s.protectedDeleteError(r, ifs, fs) })
	return nil
}

// guardForm keeps a submitted form i from changing what the user may not:
// an existing row without the change permission, unless the user deletes
// it, and a new row without the add permission. Its fields are disabled, so
// they keep their initial values and the form never counts as changed, as
// Django's _create_formsets skips the validation of such a form and its
// DeleteProtectedModelForm.has_changed returns False.
func (ifs *inlineFormSet) guardForm(form *forms.Form, i int) {
	fs := ifs.formSet()
	if !fs.IsBound() || i < 0 {
		return
	}
	var guard bool
	if i < fs.InitialFormCount() {
		deleted := ifs.hasDelete && fs.Data().Has(fs.AddPrefix(i)+"-"+forms.DeletionFieldName)
		guard = !ifs.hasChange && !deleted
	} else {
		guard = !ifs.hasAdd
	}
	if !guard {
		return
	}
	for _, name := range form.FieldNames() {
		if fd, ok := form.FieldDef(name); ok {
			fd.Base().Disabled = true
		}
	}
}

// protectedDeleteError returns the error of a form marked for deletion whose
// row other rows protect from deletion, Django's
// DeleteProtectedModelForm.hand_clean_DELETE: "Deleting <model> <row> would
// require deleting the following protected related objects: ...".
func (s *AdminSite) protectedDeleteError(r *http.Request, ifs *inlineFormSet, fs *forms.FormSet) error {
	if !fs.CanDelete {
		return nil
	}
	ie := ifs.in.entry
	instances := ifs.set.instances()
	for i, form := range fs.Forms() {
		if deleteFlag, _ := form.Cleaned()[forms.DeletionFieldName].(bool); !deleteFlag || i >= len(instances) {
			continue
		}
		obj := instances[i]
		if !instanceHasPK(ie.model, obj) {
			continue
		}
		pk := reflect.ValueOf(obj).Elem().Field(ie.model.PrimaryKey().Index).Interface()
		plan, err := orm.CollectDelete(r.Context(), s.db, ie.model, []any{pk})
		if err != nil {
			return err
		}
		if len(plan.Protected) == 0 {
			continue
		}
		return protectedError(ie, obj, plan.Protected)
	}
	return nil
}

// protectedError returns Django's error for the deletion of obj that the
// protected rows block, listing at most DeleteConfirmationMaxDisplay of
// them.
func protectedError(ie *entry, obj any, protected []orm.DeleteModelGroup) *forms.ValidationError {
	singular, _ := modelVerboseNames(ie.model)
	instance := orm.Label(ie.model, obj)
	var all []string
	for _, grp := range protected {
		name, _ := modelVerboseNames(grp.Model)
		for _, p := range grp.Objects {
			all = append(all, name+" "+orm.Label(grp.Model, p))
		}
	}
	limit := len(all)
	if m := ie.admin.DeleteConfirmationMaxDisplay; m != nil {
		limit = min(*m, len(all))
	}
	if limit == 0 {
		return forms.NewValidationError(fmt.Sprintf(
			"Deleting %s %s would require deleting some protected related objects.", singular, instance),
			"deleting_protected")
	}
	related := textList(all[:limit], "and")
	if remaining := len(all) - limit; remaining > 0 {
		more := "…and %d more objects."
		if remaining == 1 {
			more = "…and %d more object."
		}
		related = strings.Join(all[:limit], ", ") + ", " + fmt.Sprintf(more, remaining)
	}
	return forms.NewValidationError(fmt.Sprintf(
		"Deleting %s %s would require deleting the following protected related objects: %s",
		singular, instance, related), "deleting_protected")
}

// textList joins items as "a, b and c", Django's get_text_list.
func textList(items []string, lastWord string) string {
	switch len(items) {
	case 0:
		return ""
	case 1:
		return items[0]
	}
	return strings.Join(items[:len(items)-1], ", ") + " " + lastWord + " " + items[len(items)-1]
}

// instanceHasPK reports whether obj, a *T of m, has a primary key value.
func instanceHasPK(m *orm.Model, obj any) bool {
	pk := m.PrimaryKey()
	rv := reflect.ValueOf(obj)
	if pk == nil || rv.Kind() != reflect.Pointer || rv.IsNil() {
		return false
	}
	return !rv.Elem().Field(pk.Index).IsZero()
}

// inlinesValid validates every formset and reports whether all are valid,
// Django's all_valid.
func inlinesValid(sets []*inlineFormSet) bool {
	valid := true
	for _, ifs := range sets {
		if !ifs.formSet().IsValid() {
			valid = false
		}
	}
	return valid
}

// saveInlines saves the rows of the formsets, Django's save_formset for each.
func saveInlines(ctx context.Context, sets []*inlineFormSet) error {
	for _, ifs := range sets {
		if err := ifs.set.save(ctx); err != nil {
			return err
		}
	}
	return nil
}

// inlineErrorCount returns the number of errors of the formsets as Django's
// AdminErrorList counts them: each error of a formset as a whole and each
// field with errors of each form not marked for deletion.
func inlineErrorCount(sets []*inlineFormSet) int {
	n := 0
	for _, ifs := range sets {
		fs := ifs.formSet()
		if !fs.IsBound() {
			continue
		}
		n += fs.NonFormErrors().Len()
		for _, errs := range fs.Errors() {
			n += errs.Len()
		}
	}
	return n
}

// inlinesMedia returns the media of all inlines, the sum that Django's
// render_change_form adds to the page media.
func (s *AdminSite) inlinesMedia(sets []*inlineFormSet) forms.Media {
	var media forms.Media
	for _, ifs := range sets {
		media = media.Add(ifs.media(s))
	}
	return media
}

// media returns the media of one inline, Django's InlineAdminFormSet.media:
// the inline's own, the formset's and that of each form, the empty one
// included when it is shown.
func (ifs *inlineFormSet) media(s *AdminSite) forms.Media {
	fs := ifs.formSet()
	media := ifs.in.media(s).Add(fs.Media())
	for _, form := range fs.Forms() {
		media = media.Add(form.Media())
	}
	if ifs.hasAdd {
		media = media.Add(fs.EmptyForm().Media())
	}
	return media
}

// inlineContexts returns the template data of the inlines, one map per
// inline in the shape that admin/edit_inline/tabular.html and stacked.html
// read.
func (s *AdminSite) inlineContexts(ctx context.Context, sets []*inlineFormSet) []map[string]any {
	out := make([]map[string]any, 0, len(sets))
	for _, ifs := range sets {
		out = append(out, s.inlineContext(ctx, ifs))
	}
	return out
}

// inlineContext returns the template data of one inline, Django's
// InlineAdminFormSet: the heading, the management form, the column headers
// and one entry per form, the forms of the rows first, then the extra forms
// and, with the add permission, the empty form that the script copies.
func (s *AdminSite) inlineContext(ctx context.Context, ifs *inlineFormSet) map[string]any {
	in, ie, fs := ifs.in, ifs.in.entry, ifs.formSet()
	singular, plural := in.verboseNames()
	empty := fs.EmptyForm()
	readonlyAll := slices.Concat(ifs.readonly, formNames(ie.model, flattenFieldsets(ifs.sets)))

	collapsible := slices.Contains(in.cfg.Classes, "collapse")
	for _, errs := range fs.Errors() {
		if errs.Len() > 0 {
			collapsible = false
		}
	}

	instances := ifs.set.instances()
	initial := fs.InitialFormCount()
	formCtx := make([]map[string]any, 0, len(fs.Forms())+1)
	for i, form := range fs.Forms() {
		var obj any
		if i < len(instances) {
			obj = instances[i]
		}
		original := i < initial && obj != nil && instanceHasPK(ie.model, obj)
		readonly := ifs.readonly
		if i < initial && !ifs.hasChange {
			readonly = readonlyAll
		}
		if !original && i < initial {
			obj = nil
		}
		formCtx = append(formCtx, s.inlineFormContext(ctx, ifs, form, obj, original, readonly))
	}
	if ifs.hasAdd {
		formCtx = append(formCtx, s.inlineFormContext(ctx, ifs, empty, nil, false, ifs.readonly))
	}

	return map[string]any{
		"template":               in.template,
		"formset":                fs,
		"has_registered_model":   s.entryByModel(ie.model) != nil,
		"media":                  ifs.media(s),
		"prefix":                 fs.Prefix,
		"inline_formset_data":    inlineFormSetData(fs.Prefix, singular),
		"management_form":        templates.MarkSafe(fs.ManagementForm().String()),
		"non_form_errors":        templates.MarkSafe(fs.NonFormErrors().AsUL()),
		"classes":                strings.Join(in.cfg.Classes, " "),
		"is_collapsible":         collapsible,
		"max_num":                fs.MaxNum,
		"verbose_name":           singular,
		"verbose_name_plural":    plural,
		"can_delete":             fs.CanDelete,
		"has_add_permission":     ifs.hasAdd,
		"has_change_permission":  ifs.hasChange,
		"has_delete_permission":  ifs.hasDelete,
		"has_view_permission":    ifs.hasView,
		"fields":                 s.inlineHeaders(ifs, empty),
		"forms":                  formCtx,
		"show_delete_column_txt": fs.CanDelete && ifs.hasDelete,
	}
}

// inlineFormSetData returns the data-inline-formset attribute that
// inlines.js reads, the JSON of Django's inline_formset_data.
func inlineFormSetData(prefix, verboseName string) string {
	return `{"name": ` + templates.QuoteJSON("#"+prefix) +
		`, "options": {"prefix": ` + templates.QuoteJSON(prefix) +
		`, "addText": ` + templates.QuoteJSON("Add another "+capFirst(verboseName)) +
		`, "deleteText": "Remove"}}`
}

// inlineFormContext returns the template data of one form of an inline,
// Django's InlineAdminForm. obj is the row of the form when original is
// true; the read-only fields of the other forms show a new row with the
// field defaults.
func (s *AdminSite) inlineFormContext(
	ctx context.Context,
	ifs *inlineFormSet,
	form *forms.Form,
	obj any,
	original bool,
	readonly []string,
) map[string]any {
	in, ie := ifs.in, ifs.in.entry
	shown := obj
	if shown == nil {
		shown = ie.ops.newPtr()
		ie.model.ApplyDefaults(shown)
	}
	af := &adminForm{form: form, sets: ifs.sets, readonly: readonly, obj: shown, opts: ie.admin.FormOptions, skip: in.fk.Attr}
	sets := s.fieldsetContext(ctx, ie, af)

	out := map[string]any{
		"original":         "",
		"has_original":     original,
		"show_url":         false,
		"absolute_url":     "",
		"change_link":      nil,
		"non_field_errors": templates.MarkSafe(form.NonFieldErrors().AsUL()),
		"fieldsets":        sets,
		"pk_field":         "",
		"fk_field":         "",
		"deletion_field":   "",
		"deletion_label":   "",
		"cell_count":       cellCount(sets, ifs.formSet().CanDelete),
	}
	if original {
		out["original"] = orm.Label(ie.model, obj)
		u := s.viewOnSiteURL(ctx, ie, in.cfg.ViewOnSite, in.cfg.ViewOnSiteURL, obj)
		out["show_url"] = u != ""
		out["absolute_url"] = u
		if rel := s.entryByModel(ie.model); rel != nil && in.cfg.ShowChangeLink {
			class, text := "inlineviewlink", "View"
			if ifs.hasChange {
				class, text = "inlinechangelink", "Change"
			}
			out["change_link"] = map[string]any{
				"url":   s.changeURL(rel, pkOf(reflect.ValueOf(obj).Elem(), rel)),
				"class": class,
				"text":  text,
			}
		}
	}
	pk := ie.model.PrimaryKey()
	if pk != nil && (pk.Kind == orm.KindAuto || !pk.Editable || slices.Contains(ifs.exclude, pk.Attr)) {
		if bf := form.Field(pk.Attr); bf != nil {
			out["pk_field"] = templates.MarkSafe(bf.String())
		}
	}
	if bf := form.Field(in.fk.Attr); bf != nil {
		out["fk_field"] = templates.MarkSafe(bf.String())
	}
	if bf := form.Field(forms.DeletionFieldName); bf != nil {
		del := adminFieldContext(bf, false)
		out["deletion_field"] = del["html"]
		out["deletion_label"] = del["label_tag"]
	}
	return out
}

// cellCount returns the number of cells of a row of a tabular inline,
// Django's cell_count filter: the cell of the hidden fields, one per visible
// field, and the delete box.
func cellCount(sets []map[string]any, canDelete bool) int {
	n := 1
	for _, fs := range sets {
		for _, line := range fs["lines"].([]map[string]any) {
			for _, item := range line["items"].([]map[string]any) {
				if hidden, _ := item["is_hidden"].(bool); !hidden {
					n++
				}
			}
		}
	}
	if canDelete {
		n++
	}
	return n
}

// inlineHeaders returns the column headers of a tabular inline, Django's
// InlineAdminFormSet.fields: the name, label, help text and whether the
// field is required or hidden, for every field of the layout but the
// foreign key to the parent. empty is the empty form of the formset.
func (s *AdminSite) inlineHeaders(ifs *inlineFormSet, empty *forms.Form) []map[string]any {
	ie := ifs.in.entry
	var out []map[string]any
	for _, name := range flattenFieldsets(ifs.sets) {
		attr := formName(ie.model, name)
		if attr == ifs.in.fk.Attr {
			continue
		}
		bf := empty.Field(attr)
		if !ifs.hasChange || slices.Contains(ifs.readonly, attr) {
			label, help := readonlyLabel(ie, ie.admin.FormOptions, readonlyModelField(ie.model, attr), attr)
			out = append(out, map[string]any{
				"name":      attr,
				"label":     label,
				"required":  false,
				"is_hidden": bf != nil && bf.IsHidden(),
				"help_text": help,
			})
			continue
		}
		if bf == nil {
			continue
		}
		out = append(out, map[string]any{
			"name":      attr,
			"label":     bf.Label(),
			"required":  bf.Field().Base().Required,
			"is_hidden": bf.IsHidden(),
			"help_text": bf.HelpText(),
		})
	}
	return out
}

// checkInlines returns the problems of the inlines of e, Django's
// _check_inlines and InlineModelAdminChecks. On the ModelAdmin (parent): an
// inline of a type that is not a registered model (admin.E106). On each
// inline, named "InlineModelAdmin of <app>.<Model>": the checks that a
// ModelAdmin has too (see checkBase), a foreign key to the parent that is
// missing or ambiguous (admin.E202), and an ExcludeFields that names that
// foreign key (admin.E201). Register already rejects an Inline that
// TabularInline or StackedInline did not make for the model (Django's
// admin.E104). Django's checks of extra, max_num, min_num and formset
// (admin.E203 to admin.E206) have no Go counterpart: the types of the
// options already hold them.
func (s *AdminSite) checkInlines(e *entry, parent adminObject) []checks.Message {
	var out []checks.Message
	for i, inline := range e.admin.Inlines {
		in, err := inline.resolve(s)
		if in == nil {
			out = append(out, checks.Error(fmt.Sprintf("The value of 'inlines[%d].model' must be a Model.", i),
				checks.Obj(parent), checks.ID("admin.E106")))
			continue
		}
		obj := adminObject{kind: "InlineModelAdmin", model: in.entry.model}
		out = append(out, s.checkBase(in.entry, obj)...)
		if err != nil {
			msg := strings.TrimPrefix(err.Error(), "forms: ")
			out = append(out, checks.Error(msg, checks.Obj(obj), checks.ID("admin.E202")))
			continue
		}
		// Django skips the check when the exclude check failed.
		exclude := in.cfg.ExcludeFields
		if len(duplicates(in.entry.model, exclude)) > 0 {
			continue
		}
		if slices.Contains(formNames(in.entry.model, exclude), in.fk.Attr) {
			out = append(out, checks.Error(fmt.Sprintf(
				"Cannot exclude the field '%s', because it is the foreign key to the parent model '%s'.",
				in.fk.Attr, e.model.Label()), checks.Obj(obj), checks.ID("admin.E201")))
		}
	}
	return out
}
