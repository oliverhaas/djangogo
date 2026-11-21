package forms

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"
)

// The names of the management form fields and of the fields a formset adds to
// its forms, as Django's formsets module names them.
const (
	// TotalFormCount is the management form field with the number of forms.
	TotalFormCount = "TOTAL_FORMS"
	// InitialFormCount is the management form field with the number of
	// forms that show existing data.
	InitialFormCount = "INITIAL_FORMS"
	// MinNumFormCount is the management form field with MinNum, a hint for
	// client-side code.
	MinNumFormCount = "MIN_NUM_FORMS"
	// MaxNumFormCount is the management form field with MaxNum, a hint for
	// client-side code.
	MaxNumFormCount = "MAX_NUM_FORMS"
	// OrderingFieldName is the field a formset with CanOrder adds to each
	// form.
	OrderingFieldName = "ORDER"
	// DeletionFieldName is the field a formset with CanDelete adds to each
	// form.
	DeletionFieldName = "DELETE"
)

// DefaultMaxNum is the MaxNum of a new formset, and what AbsoluteMax adds to
// MaxNum by default, Django's DEFAULT_MAX_NUM.
const DefaultMaxNum = 1000

// newManagementForm returns the form that tells the server how many forms a
// submitted formset holds, Django's ManagementForm. When its data is invalid
// it cleans both counts to 0.
func newManagementForm() *Form {
	f := New(
		F(TotalFormCount, NewIntegerField(UseWidget(HiddenInput{}))),
		F(InitialFormCount, NewIntegerField(UseWidget(HiddenInput{}))),
		// The minimum and maximum are output for client-side code; the
		// server does not read them back.
		F(MinNumFormCount, NewIntegerField(Required(false), UseWidget(HiddenInput{}))),
		F(MaxNumFormCount, NewIntegerField(Required(false), UseWidget(HiddenInput{}))),
	)
	f.postCleaners = append(f.postCleaners, func(f *Form) {
		for _, name := range []string{TotalFormCount, InitialFormCount} {
			if _, ok := f.cleaned[name]; !ok {
				f.cleaned[name] = int64(0)
			}
		}
	})
	return f
}

// FormSet is a list of forms of the same kind on one page, the counterpart of
// a Django formset. NewFormSet makes one from a constructor of the form:
//
//	fs := forms.NewFormSet(newArticleForm)
//	fs.Extra = 2
//	fs = fs.Bind(r.PostForm)
//	if fs.IsValid() {
//		for _, data := range fs.CleanedData() {
//			...
//		}
//	}
//
// A hidden management form carries the number of forms, so that the server
// knows how many forms a submission holds. The exported fields are the
// arguments of Django's formset_factory and of the formset constructor; set
// them before the formset is rendered or validated.
type FormSet struct {
	// Extra is the number of empty forms shown after the initial ones; 1 by
	// default.
	Extra int
	// MinNum is the minimum number of forms; that many forms are shown and,
	// with ValidateMin, must be filled in.
	MinNum int
	// MaxNum is the maximum number of forms shown, DefaultMaxNum by default.
	// More initial forms than MaxNum are all shown, with no extra form.
	// ValidateMax rejects a submission with more forms.
	MaxNum int
	// AbsoluteMax is the most forms a submission may create, a limit
	// against memory exhaustion; 0 means MaxNum plus DefaultMaxNum. It must
	// not be below MaxNum.
	AbsoluteMax int
	// CanOrder adds the integer field ORDER to each form; OrderedForms
	// returns the forms in its order.
	CanOrder bool
	// CanDelete adds the checkbox DELETE to each form; DeletedForms returns
	// the forms with it checked, which validation skips.
	CanDelete bool
	// CanDeleteExtra adds DELETE to the extra forms too; true by default.
	CanDeleteExtra bool
	// ValidateMin rejects a submission with fewer than MinNum filled-in
	// forms.
	ValidateMin bool
	// ValidateMax rejects a submission with more than MaxNum forms.
	ValidateMax bool

	// Prefix is put before the names of the fields of the forms, as
	// "prefix-<index>-name"; "form" by default.
	Prefix string
	// AutoID is the id pattern of the forms, as Form.AutoID.
	AutoID string
	// Initial holds the initial data of the first forms, one map per form.
	Initial []map[string]any
	// ErrorMessages overrides the messages missing_management_form,
	// too_many_forms and too_few_forms.
	ErrorMessages map[string]string
	// DeletionWidget is the widget of the DELETE field, a CheckboxInput by
	// default.
	DeletionWidget Widget
	// OrderingWidget is the widget of the ORDER field, a NumberInput by
	// default.
	OrderingWidget Widget
	// ConfigureForm, when set, is called for each new form before the
	// formset adds its fields, with the index of the form or -1 for the
	// empty form, Django's form_kwargs and get_form_kwargs. What it sets
	// overrides the settings of the formset, except for the empty form,
	// which stays optional.
	ConfigureForm func(form *Form, index int)

	newForm  func() *Form
	cleaners []func(*FormSet) error
	ctx      context.Context

	bound bool
	data  url.Values

	management    *Form
	forms         []*Form
	formsBuilt    bool
	errors        []*ErrorDict
	nonFormErrors *ErrorList

	// hooks lets a model formset build its forms from rows.
	hooks *formSetHooks
}

// formSetHooks are the parts of a formset that a model formset replaces,
// the methods a Django BaseModelFormSet overrides.
type formSetHooks struct {
	// form returns form i before the formset settings apply, -1 for the
	// empty form.
	form func(fs *FormSet, i int) *Form
	// bind binds form i to the data of the formset.
	bind func(fs *FormSet, i int, f *Form) *Form
	// initialCount returns the number of initial forms of an unbound
	// formset.
	initialCount func(fs *FormSet) int
	// initial returns the initial data of form i from Initial.
	initial func(fs *FormSet, i int) map[string]any
	// addFields adds fields to form i after those of the formset.
	addFields func(fs *FormSet, f *Form, i int)
	// configured runs on form i after the formset settings apply.
	configured func(fs *FormSet, f *Form, i int)
	// clean validates the forms together before the CleanWith functions.
	clean func(fs *FormSet) error
}

// NewFormSet returns an unbound formset of forms made by newForm. newForm is
// called once per form and must return a new form each time.
func NewFormSet(newForm func() *Form) *FormSet {
	return &FormSet{
		Extra:          1,
		MaxNum:         DefaultMaxNum,
		CanDeleteExtra: true,
		Prefix:         "form",
		AutoID:         "id_%s",
		DeletionWidget: CheckboxInput{},
		OrderingWidget: NumberInput{},
		newForm:        newForm,
	}
}

// Bind returns a copy of the formset bound to data, the counterpart of
// passing data to a Django formset's constructor. The receiver stays unbound.
// Nil data binds an empty submission.
func (fs *FormSet) Bind(data url.Values) *FormSet {
	c := fs.clone()
	if data == nil {
		data = url.Values{}
	}
	c.bound, c.data = true, data
	return c
}

// clone returns a copy without the state of rendering or validation.
func (fs *FormSet) clone() *FormSet {
	c := *fs
	c.cleaners = slices.Clone(fs.cleaners)
	c.management, c.forms, c.formsBuilt = nil, nil, false
	c.errors, c.nonFormErrors = nil, nil
	return &c
}

// CleanWith registers a function that runs after the forms have been
// cleaned, the counterpart of the clean() method of a formset. It reads the
// forms or CleanedData and returns an error that belongs to the formset as a
// whole; NonFormErrors reports it.
func (fs *FormSet) CleanWith(fn func(fs *FormSet) error) *FormSet {
	fs.cleaners = append(fs.cleaners, fn)
	return fs
}

// SetContext sets the context for the database queries of the forms. Without
// it the queries run with context.Background.
func (fs *FormSet) SetContext(ctx context.Context) { fs.ctx = ctx }

// context returns the formset's context.
func (fs *FormSet) context() context.Context {
	if fs.ctx == nil {
		return context.Background()
	}
	return fs.ctx
}

// IsBound reports whether the formset holds submitted data.
func (fs *FormSet) IsBound() bool { return fs.bound }

// Data returns the submitted data, nil for an unbound formset.
func (fs *FormSet) Data() url.Values { return fs.data }

// Err returns the first failure that is not a validation error: a MaxNum
// above AbsoluteMax, or a failure of a form, such as a database error while
// loading choices. A formset with such a failure is never valid.
func (fs *FormSet) Err() error {
	if fs.maxAbsolute() < fs.MaxNum {
		return errors.New("forms: 'absolute_max' must be greater or equal to 'max_num'")
	}
	for _, f := range fs.forms {
		if err := f.Err(); err != nil {
			return err
		}
	}
	return nil
}

// maxAbsolute returns AbsoluteMax with its default.
func (fs *FormSet) maxAbsolute() int {
	if fs.AbsoluteMax == 0 {
		return fs.MaxNum + DefaultMaxNum
	}
	return fs.AbsoluteMax
}

// errorMessage returns the message of code, overridden by ErrorMessages.
func (fs *FormSet) errorMessage(code string, num int) string {
	if msg, ok := fs.ErrorMessages[code]; ok {
		return msg
	}
	switch code {
	case "missing_management_form":
		return "ManagementForm data is missing or has been tampered with. Missing fields: " +
			"%(field_names)s. You may need to file a bug report if the issue persists."
	case "too_many_forms":
		if num == 1 {
			return "Please submit at most %(num)d form."
		}
		return "Please submit at most %(num)d forms."
	case "too_few_forms":
		if num == 1 {
			return "Please submit at least %(num)d form."
		}
		return "Please submit at least %(num)d forms."
	}
	return ""
}

// ManagementForm returns the management form: for a bound formset the form
// bound to the submitted counts, for an unbound one the form that renders
// them.
func (fs *FormSet) ManagementForm() *Form {
	if fs.management != nil {
		return fs.management
	}
	f := newManagementForm()
	f.Prefix, f.AutoID = fs.Prefix, fs.AutoID
	if fs.bound {
		f = f.Bind(fs.data)
		f.Errors()
	} else {
		f.Initial = map[string]any{
			TotalFormCount:   fs.TotalFormCount(),
			InitialFormCount: fs.InitialFormCount(),
			MinNumFormCount:  fs.MinNum,
			MaxNumFormCount:  fs.MaxNum,
		}
	}
	fs.management = f
	return f
}

// managementCount returns a cleaned count of the bound management form.
func (fs *FormSet) managementCount(name string) int {
	n, _ := fs.ManagementForm().Cleaned()[name].(int64)
	return int(n)
}

// TotalFormCount returns the number of forms. For a bound formset it is the
// submitted count, at most AbsoluteMax; for an unbound one the initial forms,
// at least MinNum, plus Extra, at most MaxNum unless the initial forms are
// more.
func (fs *FormSet) TotalFormCount() int {
	if fs.bound {
		return min(fs.managementCount(TotalFormCount), fs.maxAbsolute())
	}
	initial := fs.InitialFormCount()
	total := max(initial, fs.MinNum) + fs.Extra
	switch {
	case initial > fs.MaxNum && fs.MaxNum >= 0:
		total = initial
	case total > fs.MaxNum && fs.MaxNum >= 0:
		total = fs.MaxNum
	}
	return total
}

// InitialFormCount returns the number of forms that show existing data: the
// submitted count for a bound formset, the number of Initial maps for an
// unbound one.
func (fs *FormSet) InitialFormCount() int {
	if fs.bound {
		return fs.managementCount(InitialFormCount)
	}
	if fs.hooks != nil && fs.hooks.initialCount != nil {
		return fs.hooks.initialCount(fs)
	}
	return len(fs.Initial)
}

// Forms returns the forms in order, built on the first call.
func (fs *FormSet) Forms() []*Form {
	if fs.formsBuilt {
		return fs.forms
	}
	fs.formsBuilt = true
	total := fs.TotalFormCount()
	fs.forms = make([]*Form, 0, total)
	for i := range total {
		fs.forms = append(fs.forms, fs.constructForm(i))
	}
	return fs.forms
}

// makeForm returns a new form i, -1 for the empty form.
func (fs *FormSet) makeForm(i int) *Form {
	var f *Form
	if fs.hooks != nil && fs.hooks.form != nil {
		f = fs.hooks.form(fs, i)
	} else {
		f = fs.newForm()
	}
	f.SetContext(fs.context())
	return f
}

// constructForm builds form i, Django's _construct_form.
func (fs *FormSet) constructForm(i int) *Form {
	f := fs.makeForm(i)
	f.AutoID = fs.AutoID
	f.Prefix = fs.AddPrefix(i)
	// The required attribute would stop the browser from submitting the
	// extra, optional and deleted forms.
	f.UseRequiredAttribute = false
	if init := fs.formInitial(i); init != nil {
		f.Initial = mergeInitial(f.Initial, init)
	}
	// The extra forms may stay empty, unless they are part of the minimum.
	if i >= fs.InitialFormCount() && i >= fs.MinNum {
		f.EmptyPermitted = true
	}
	if fs.hooks != nil && fs.hooks.configured != nil {
		fs.hooks.configured(fs, f, i)
	}
	if fs.ConfigureForm != nil {
		fs.ConfigureForm(f, i)
	}
	fs.addFields(f, i)
	if fs.bound {
		if fs.hooks != nil && fs.hooks.bind != nil {
			return fs.hooks.bind(fs, i, f)
		}
		return f.Bind(fs.data)
	}
	return f
}

// formInitial returns the entry of Initial for form i, nil when it has none.
func (fs *FormSet) formInitial(i int) map[string]any {
	if fs.hooks != nil && fs.hooks.initial != nil {
		return fs.hooks.initial(fs, i)
	}
	if i < len(fs.Initial) {
		return fs.Initial[i]
	}
	return nil
}

// mergeInitial returns base with the entries of over added.
func mergeInitial(base, over map[string]any) map[string]any {
	out := make(map[string]any, len(base)+len(over))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range over {
		out[k] = v
	}
	return out
}

// addFields adds ORDER and DELETE to form i, -1 for the empty form, Django's
// add_fields.
func (fs *FormSet) addFields(f *Form, i int) {
	initial := fs.InitialFormCount()
	if fs.CanOrder {
		opts := []FieldOption{Label("Order"), Required(false), UseWidget(fs.OrderingWidget)}
		// Only the initial forms get their position as the order.
		if i >= 0 && i < initial {
			opts = append(opts, Initial(i+1))
		}
		f.AddField(OrderingFieldName, NewIntegerField(opts...))
	}
	if fs.CanDelete && (fs.CanDeleteExtra || (i >= 0 && i < initial)) {
		f.AddField(DeletionFieldName, NewBooleanField(Label("Delete"), Required(false), UseWidget(fs.DeletionWidget)))
	}
	if fs.hooks != nil && fs.hooks.addFields != nil {
		fs.hooks.addFields(fs, f, i)
	}
}

// AddPrefix returns the prefix of form i, "prefix-i", and the prefix
// "prefix-__prefix__" of the empty form for -1.
func (fs *FormSet) AddPrefix(i int) string {
	if i < 0 {
		return fs.Prefix + "-__prefix__"
	}
	return fs.Prefix + "-" + strconv.Itoa(i)
}

// InitialForms returns the forms that show existing data.
func (fs *FormSet) InitialForms() []*Form {
	forms := fs.Forms()
	return forms[:min(fs.InitialFormCount(), len(forms))]
}

// ExtraForms returns the forms after the initial ones.
func (fs *FormSet) ExtraForms() []*Form {
	forms := fs.Forms()
	return forms[min(fs.InitialFormCount(), len(forms)):]
}

// EmptyForm returns an unbound form with the prefix "prefix-__prefix__", a
// template for client-side code that adds forms.
func (fs *FormSet) EmptyForm() *Form {
	f := fs.makeForm(-1)
	if fs.hooks != nil && fs.hooks.configured != nil {
		fs.hooks.configured(fs, f, -1)
	}
	if fs.ConfigureForm != nil {
		fs.ConfigureForm(f, -1)
	}
	f.AutoID = fs.AutoID
	f.Prefix = fs.AddPrefix(-1)
	f.EmptyPermitted = true
	f.UseRequiredAttribute = false
	fs.addFields(f, -1)
	return f
}

// CleanedData returns the cleaned data of each form, validating the formset
// when needed. It is nil when the formset is not valid. A form that was left
// empty has an empty map.
func (fs *FormSet) CleanedData() []map[string]any {
	if !fs.IsValid() {
		return nil
	}
	out := make([]map[string]any, len(fs.forms))
	for i, f := range fs.forms {
		out[i] = f.CleanedData()
	}
	return out
}

// shouldDelete reports whether the form is marked for deletion.
func shouldDelete(f *Form) bool {
	del, _ := f.CleanedData()[DeletionFieldName].(bool)
	return del
}

// unchangedExtra reports whether form i is an extra form left unchanged.
func (fs *FormSet) unchangedExtra(i int, f *Form) bool {
	return i >= fs.InitialFormCount() && !f.HasChanged()
}

// DeletedForms returns the forms marked for deletion; it is empty unless the
// formset is valid and has CanDelete. An unchanged extra form is never
// deleted.
func (fs *FormSet) DeletedForms() []*Form {
	if !fs.IsValid() || !fs.CanDelete {
		return []*Form{}
	}
	return fs.deletedForms()
}

// deletedForms returns the forms marked for deletion without validating the
// formset first.
func (fs *FormSet) deletedForms() []*Form {
	out := []*Form{}
	for i, f := range fs.Forms() {
		if !fs.unchangedExtra(i, f) && shouldDelete(f) {
			out = append(out, f)
		}
	}
	return out
}

// OrderedForms returns the forms in the order of their ORDER field, with an
// empty order last and the index deciding a tie. Unchanged extra forms and
// forms marked for deletion are left out. It is nil unless the formset is
// valid and has CanOrder.
func (fs *FormSet) OrderedForms() []*Form {
	if !fs.IsValid() || !fs.CanOrder {
		return nil
	}
	type entry struct {
		form  *Form
		order int64
		blank bool
	}
	var entries []entry
	for i, f := range fs.forms {
		if fs.unchangedExtra(i, f) || (fs.CanDelete && shouldDelete(f)) {
			continue
		}
		order, ok := f.CleanedData()[OrderingFieldName].(int64)
		entries = append(entries, entry{form: f, order: order, blank: !ok})
	}
	slices.SortStableFunc(entries, func(a, b entry) int {
		switch {
		case a.blank != b.blank:
			if a.blank {
				return 1
			}
			return -1
		case a.order < b.order:
			return -1
		case a.order > b.order:
			return 1
		}
		return 0
	})
	out := make([]*Form, len(entries))
	for i, e := range entries {
		out[i] = e.form
	}
	return out
}

// NonFormErrors returns the errors of the formset as a whole: an invalid
// management form, too many or too few forms, and the errors of the
// CleanWith functions. It validates the formset when needed.
func (fs *FormSet) NonFormErrors() ErrorList {
	if fs.nonFormErrors == nil {
		fs.fullClean()
	}
	return *fs.nonFormErrors
}

// Errors returns the errors of each form, validating the formset when needed.
// A form marked for deletion has no entry.
func (fs *FormSet) Errors() []*ErrorDict {
	if fs.nonFormErrors == nil {
		fs.fullClean()
	}
	return fs.errors
}

// TotalErrorCount returns the number of errors of the formset and of all its
// forms.
func (fs *FormSet) TotalErrorCount() int {
	n := fs.NonFormErrors().Len()
	for _, e := range fs.Errors() {
		n += e.Len()
	}
	return n
}

// IsValid reports whether the formset is bound and it and every form not
// marked for deletion have no errors.
func (fs *FormSet) IsValid() bool {
	if !fs.bound {
		return false
	}
	fs.Errors()
	return fs.formsValid()
}

// fullClean validates the forms and the formset, Django's full_clean.
func (fs *FormSet) fullClean() {
	fs.errors = []*ErrorDict{}
	fs.nonFormErrors = newErrorList("nonform", "")
	if !fs.bound {
		return
	}
	management := fs.ManagementForm()
	if !management.IsValid() {
		names := make([]string, 0, len(management.Errors().Fields()))
		for _, name := range management.Errors().Fields() {
			names = append(names, management.AddPrefix(name))
		}
		fs.nonFormErrors.data = append(fs.nonFormErrors.data, NewValidationError(
			fs.errorMessage("missing_management_form", 0), "missing_management_form",
			map[string]any{"field_names": strings.Join(names, ", ")}))
	}
	emptyForms := 0
	for i, f := range fs.Forms() {
		// Empty forms are the unchanged ones beyond those with initial data.
		if fs.unchangedExtra(i, f) {
			emptyForms++
		}
		errs := f.Errors()
		if fs.CanDelete && shouldDelete(f) {
			continue
		}
		fs.errors = append(fs.errors, errs)
	}
	if err := fs.validate(emptyForms); err != nil {
		list := newErrorList("nonform", "")
		list.data = toValidationError(err).ErrorList()
		fs.nonFormErrors = list
	}
}

// validate checks the number of forms and runs the clean functions. As in
// Django, the deleted forms only count when every other form is valid.
func (fs *FormSet) validate(emptyForms int) error {
	deleted := 0
	if fs.CanDelete && fs.formsValid() {
		deleted = len(fs.deletedForms())
	}
	if (fs.ValidateMax && fs.TotalFormCount()-deleted > fs.MaxNum) ||
		fs.managementCount(TotalFormCount) > fs.maxAbsolute() {
		return NewValidationError(fs.errorMessage("too_many_forms", fs.MaxNum), "too_many_forms",
			map[string]any{"num": fs.MaxNum})
	}
	if fs.ValidateMin && fs.TotalFormCount()-deleted-emptyForms < fs.MinNum {
		return NewValidationError(fs.errorMessage("too_few_forms", fs.MinNum), "too_few_forms",
			map[string]any{"num": fs.MinNum})
	}
	if fs.hooks != nil && fs.hooks.clean != nil {
		if err := fs.hooks.clean(fs); err != nil {
			return err
		}
	}
	for _, clean := range fs.cleaners {
		if err := clean(fs); err != nil {
			return err
		}
	}
	return nil
}

// formsValid reports whether the forms not marked for deletion are valid and
// the formset has no error yet.
func (fs *FormSet) formsValid() bool {
	for _, f := range fs.Forms() {
		if (!fs.CanDelete || !shouldDelete(f)) && !f.IsValid() {
			return false
		}
	}
	return fs.nonFormErrors.Len() == 0 && fs.Err() == nil
}

// HasChanged reports whether any form differs from its initial data.
func (fs *FormSet) HasChanged() bool {
	for _, f := range fs.Forms() {
		if f.HasChanged() {
			return true
		}
	}
	return false
}

// IsMultipart reports whether the formset needs multipart encoding, as its
// forms do.
func (fs *FormSet) IsMultipart() bool {
	if forms := fs.Forms(); len(forms) > 0 {
		return forms[0].IsMultipart()
	}
	return fs.EmptyForm().IsMultipart()
}

// Media returns the media of the forms, which are all alike.
func (fs *FormSet) Media() Media {
	if forms := fs.Forms(); len(forms) > 0 {
		return forms[0].Media()
	}
	return fs.EmptyForm().Media()
}

// render joins the management form and each form in the given layout,
// Django's formset templates.
func (fs *FormSet) render(layout func(*Form) string) string {
	var b strings.Builder
	b.WriteString(fs.ManagementForm().String())
	for _, f := range fs.Forms() {
		b.WriteString(layout(f))
	}
	return b.String()
}

// AsDiv renders the management form and each form as <div> elements. It is
// the default layout.
func (fs *FormSet) AsDiv() string { return fs.render((*Form).AsDiv) }

// AsP renders the management form and each form as <p> elements.
func (fs *FormSet) AsP() string { return fs.render((*Form).AsP) }

// AsTable renders the management form and each form as table rows.
func (fs *FormSet) AsTable() string { return fs.render((*Form).AsTable) }

// AsUL renders the management form and each form as list items.
func (fs *FormSet) AsUL() string { return fs.render((*Form).AsUL) }

// String returns AsDiv, as str() of a Django formset does.
func (fs *FormSet) String() string { return fs.AsDiv() }

// HTML returns the rendered markup; templates print it without escaping.
func (fs *FormSet) HTML() string { return fs.String() }

// Repr returns a short description of the formset for messages, Django's
// repr(formset): "<FormSet: bound=True valid=False total_forms=2>". valid is
// Unknown until the formset has been validated; Repr does not validate it.
func (fs *FormSet) Repr() string {
	valid := "Unknown"
	if fs.nonFormErrors != nil {
		ok := fs.bound && fs.nonFormErrors.Len() == 0
		for _, e := range fs.errors {
			if e.Len() > 0 {
				ok = false
			}
		}
		valid = pyString(ok)
	}
	return fmt.Sprintf("<FormSet: bound=%s valid=%s total_forms=%d>", pyString(fs.bound), valid, fs.TotalFormCount())
}

// AllValid validates every formset, also after one is found invalid, and
// reports whether all are valid, Django's all_valid.
func AllValid(sets ...*FormSet) bool {
	valid := true
	for _, fs := range sets {
		if !fs.IsValid() {
			valid = false
		}
	}
	return valid
}

// TemplateFormSet is a formset in a template. Printed, it is the formset in
// its default layout. Its keys are the attributes of Django's formset:
// management_form, forms, initial_forms, extra_forms, empty_form, errors,
// non_form_errors, total_error_count, is_bound, is_valid, is_multipart,
// prefix, media, can_order, can_delete, min_num, max_num, as_div, as_p,
// as_table and as_ul. {% for form in formset %} is written
// {% for form in formset.forms %}.
type TemplateFormSet map[string]any

// String returns the formset in its default layout.
func (t TemplateFormSet) String() string {
	if fs, ok := t[templateSelf].(*FormSet); ok {
		return fs.AsDiv()
	}
	return ""
}

// HTML returns the rendered markup; templates print it without escaping.
func (t TemplateFormSet) HTML() string { return t.String() }

// templateForms converts forms for a template.
func templateForms(forms []*Form) []any {
	out := make([]any, len(forms))
	for i, f := range forms {
		out[i] = f.TemplateValue()
	}
	return out
}

// TemplateValue returns the formset in the form a template uses. A bound
// formset is validated by this call, as its errors are part of the value.
func (fs *FormSet) TemplateValue() any {
	errs := make([]any, 0, len(fs.Errors()))
	for _, e := range fs.Errors() {
		errs = append(errs, e.TemplateValue())
	}
	return TemplateFormSet{
		templateSelf:        fs,
		"management_form":   fs.ManagementForm().TemplateValue(),
		"forms":             templateForms(fs.Forms()),
		"initial_forms":     templateForms(fs.InitialForms()),
		"extra_forms":       templateForms(fs.ExtraForms()),
		"empty_form":        func() any { return fs.EmptyForm().TemplateValue() },
		"errors":            errs,
		"non_form_errors":   fs.NonFormErrors().TemplateValue(),
		"total_error_count": fs.TotalErrorCount(),
		"is_bound":          fs.bound,
		"is_valid":          fs.IsValid,
		"is_multipart":      fs.IsMultipart(),
		"prefix":            fs.Prefix,
		"media":             fs.Media().TemplateValue(),
		"can_order":         fs.CanOrder,
		"can_delete":        fs.CanDelete,
		"min_num":           fs.MinNum,
		"max_num":           fs.MaxNum,
		"as_div":            lazyHTML(fs.AsDiv),
		"as_p":              lazyHTML(fs.AsP),
		"as_table":          lazyHTML(fs.AsTable),
		"as_ul":             lazyHTML(fs.AsUL),
	}
}
