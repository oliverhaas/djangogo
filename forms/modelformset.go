package forms

import (
	"context"
	"fmt"
	"maps"
	"net/url"
	"reflect"
	"strconv"
	"strings"

	"github.com/oliverhaas/djangogo/orm"
)

// ChangedObject is a row that ModelFormSet.Save changed, with the names of the
// fields that changed, an entry of Django's changed_objects.
type ChangedObject[T any] struct {
	// Object is the saved row.
	Object *T
	// Fields are the names of the changed fields, Django's form.changed_data.
	Fields []string
}

// ModelFormSet is a formset of model forms for the rows of T, Django's
// modelformset_factory and BaseModelFormSet. It embeds the FormSet, so the
// settings and methods of a formset are available on it. The initial forms
// edit the rows of QuerySet, one form per row, and the extra forms add rows:
//
//	fs := forms.NewModelFormSet[Author](db, forms.WithFields("name"))
//	fs.QuerySet = orm.Query[Author](db).Filter("name__startswith", "O")
//	bound := fs.Bind(r.PostForm)
//	bound.SetContext(r.Context())
//	if bound.IsValid() {
//		authors, err := bound.Save(r.Context())
//	}
//
// Each form carries the primary key of its row in a hidden field, so a bound
// form edits the row it was rendered for. Validation also checks the unique
// fields across the forms: two forms that would create the same unique value
// fail with Django's "Please correct the duplicate values below." Initial
// holds the initial data of the extra forms, Django's initial argument of a
// model formset.
type ModelFormSet[T any] struct {
	*FormSet
	// QuerySet selects the rows of the initial forms; nil selects every row.
	// Rows that the queryset does not order completely are also ordered by
	// primary key, so that a form stays with its row between requests.
	QuerySet *orm.QuerySet[T]
	// EditOnly makes Save change and delete the existing rows only and never
	// add one, Django's edit_only.
	EditOnly bool
	// NewForm returns a new form for one row, Django's form argument. It
	// defaults to NewModelForm with the options of NewModelFormSet.
	NewForm func() *ModelForm[T]
	// SaveNew saves the instance of an extra form, Django's save_new. Nil
	// saves it with the form; with commit false it only returns the instance.
	SaveNew func(ctx context.Context, form *ModelForm[T], commit bool) (*T, error)
	// SaveExisting saves the changed row of an initial form, Django's
	// save_existing. Nil saves it with the form; with commit false it only
	// returns the instance.
	SaveExisting func(ctx context.Context, form *ModelForm[T], obj *T, commit bool) (*T, error)
	// DeleteExisting deletes a row whose form is marked for deletion,
	// Django's delete_existing. Nil deletes the row when commit is true.
	DeleteExisting func(ctx context.Context, obj *T, commit bool) error

	db     *orm.DB
	model  *orm.Model
	inline *inlineSpec
	st     *modelFormSetState[T]
}

// modelFormSetState is what a model formset learns while it builds,
// validates and saves its forms.
type modelFormSetState[T any] struct {
	objects      []*T
	loaded       bool
	byPK         map[string]*T
	pkRows       []modelRow
	pkRowsLoaded bool
	data         url.Values // the data of the forms when it differs from the formset's
	forms        map[*Form]*ModelForm[T]

	newObjects     []*T
	changedObjects []ChangedObject[T]
	deletedObjects []*T
	savedForms     []*ModelForm[T]
}

// NewModelFormSet returns an unbound formset of model forms for T, built
// with opts as NewModelForm builds one form. T must be registered with the
// database's registry; otherwise Err reports the mistake.
func NewModelFormSet[T any](db *orm.DB, opts ...ModelFormOption) *ModelFormSet[T] {
	s := &ModelFormSet[T]{FormSet: NewFormSet(nil), db: db}
	s.NewForm = func() *ModelForm[T] { return NewModelForm[T](db, opts...) }
	if model, ok := db.Registry().ModelOf(new(T)); ok {
		s.model = model
	} else {
		s.fail(fmt.Errorf("forms: %T is not a registered model", *new(T)))
	}
	s.install()
	return s
}

// install gives the formset fresh state and the hooks that build its forms
// from rows.
func (s *ModelFormSet[T]) install() {
	s.st = &modelFormSetState[T]{forms: map[*Form]*ModelForm[T]{}}
	s.hooks = &formSetHooks{
		form:         func(_ *FormSet, i int) *Form { return s.makeForm(i) },
		bind:         func(_ *FormSet, i int, f *Form) *Form { return s.bindForm(i, f) },
		initialCount: func(*FormSet) int { return s.initialCount() },
		initial:      func(_ *FormSet, i int) map[string]any { return s.extraInitial(i) },
		addFields:    func(_ *FormSet, f *Form, i int) { s.addFields(f, i) },
		clean:        func(*FormSet) error { return s.validateUnique() },
	}
}

// derive returns a copy of the formset around fs with fresh state.
func (s *ModelFormSet[T]) derive(fs *FormSet) *ModelFormSet[T] {
	c := *s
	c.FormSet = fs
	if s.inline != nil {
		in := *s.inline
		c.inline = &in
	}
	c.install()
	return &c
}

// Bind returns a copy of the formset bound to data. The receiver stays
// unbound.
func (s *ModelFormSet[T]) Bind(data url.Values) *ModelFormSet[T] {
	return s.derive(s.FormSet.Bind(data))
}

// Model returns the model of the formset.
func (s *ModelFormSet[T]) Model() *orm.Model { return s.model }

// GetQuerySet returns the rows of the initial forms as a queryset, Django's
// get_queryset: QuerySet or every row, ordered by primary key after its own
// ordering unless that ordering is total.
func (s *ModelFormSet[T]) GetQuerySet() *orm.QuerySet[T] {
	qs := s.QuerySet
	if qs == nil {
		qs = orm.Query[T](s.db)
	}
	if s.inline != nil {
		if s.inline.parentHasPK() {
			qs = qs.Filter(s.inline.fk.Attr, s.inline.parentPKValue())
		} else {
			// A parent without a primary key has no rows, Django's
			// queryset.none(); a filter on a null key would match the
			// rows without a parent.
			qs = qs.Filter("pk__in", []any{})
		}
	}
	if !qs.TotallyOrdered() {
		qs = qs.OrderBy().OrderBy(append(qs.Ordering(), "pk")...)
	}
	return qs
}

// objects returns the rows of the initial forms, loaded on the first call.
func (s *ModelFormSet[T]) objects() []*T {
	if s.st.loaded {
		return s.st.objects
	}
	s.st.loaded = true
	s.st.byPK = map[string]*T{}
	// An inline formset of a parent without a primary key has no rows,
	// Django's queryset.none().
	if s.model == nil || (s.inline != nil && !s.inline.parentHasPK()) {
		return nil
	}
	rows, err := s.GetQuerySet().All(s.context())
	if err != nil {
		s.fail(fmt.Errorf("forms: loading the rows of the formset: %w", err))
		return nil
	}
	s.st.objects = make([]*T, len(rows))
	for i := range rows {
		obj := &rows[i]
		s.st.objects[i] = obj
		if key, ok := instancePK(s.model, obj); ok {
			s.st.byPK[key] = obj
		}
	}
	return s.st.objects
}

// initialCount returns the number of initial forms: none for an inline
// formset that saves as new, the submitted count for a bound formset, and
// one per row otherwise.
func (s *ModelFormSet[T]) initialCount() int {
	if s.inline != nil && *s.inline.saveAsNew {
		return 0
	}
	if s.bound {
		return s.baseInitialFormCount()
	}
	return len(s.objects())
}

// extraInitial returns the entry of Initial for extra form i.
func (s *ModelFormSet[T]) extraInitial(i int) map[string]any {
	n := s.InitialFormCount()
	if i >= n && i-n < len(s.Initial) {
		return s.Initial[i-n]
	}
	return nil
}

// pkKey converts a submitted primary key to the key of its row, as the
// to_python of the primary key field does. The second result is false for a
// value that is not a primary key.
func (s *ModelFormSet[T]) pkKey(raw string) (string, any, bool) {
	pk := s.model.PrimaryKey()
	kind := pk.Kind
	if pk.Rel != nil {
		// A relation is keyed by the integer primary key of its row.
		kind = orm.KindBigInt
	}
	switch kind {
	case orm.KindAuto, orm.KindInt, orm.KindSmallInt, orm.KindBigInt,
		orm.KindPositiveSmallInt, orm.KindPositiveInt, orm.KindPositiveBigInt:
		n, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
		if err != nil {
			return "", nil, false
		}
		return strconv.FormatInt(n, 10), n, true
	case orm.KindUUID:
		v, err := NewUUIDField().ToGo(raw)
		key, ok := v.(string)
		if err != nil || !ok {
			return "", nil, false
		}
		return key, key, true
	}
	return raw, raw, true
}

// submittedPK returns the primary key submitted for form i.
func (s *ModelFormSet[T]) submittedPK(i int) (string, bool) {
	values, ok := s.data[s.AddPrefix(i)+"-"+s.model.PrimaryKey().Attr]
	if !ok || len(values) == 0 {
		return "", false
	}
	return values[len(values)-1], true
}

// formObject returns the row that initial form i edits: the row of the
// submitted primary key for a bound formset, row i otherwise. It is nil
// when the submitted key selects no row of the queryset.
func (s *ModelFormSet[T]) formObject(i int) *T {
	if !s.bound {
		if objs := s.objects(); i < len(objs) {
			return objs[i]
		}
		return nil
	}
	raw, ok := s.submittedPK(i)
	if !ok {
		return nil
	}
	key, _, ok := s.pkKey(raw)
	if !ok {
		return nil
	}
	s.objects()
	return s.st.byPK[key]
}

// makeForm returns a new form i, -1 for the empty form, Django's
// _construct_form up to the formset settings.
func (s *ModelFormSet[T]) makeForm(i int) *Form {
	mf := s.NewForm()
	if s.inline != nil && mf.model != nil {
		mf.includedFields = maps.Clone(mf.includedFields)
		mf.includedFields[s.inline.fk.Attr] = true
	}
	if s.model != nil && i >= 0 && i < s.InitialFormCount() {
		if obj := s.formObject(i); obj != nil {
			mf = mf.WithInstance(obj)
		}
	}
	if s.inline != nil && i >= 0 && !s.bound {
		s.setParentFK(mf)
	}
	s.st.forms[mf.Form] = mf
	return mf.Form
}

// bindForm binds form i to the data of the formset.
func (s *ModelFormSet[T]) bindForm(_ int, f *Form) *Form {
	mf := s.st.forms[f]
	data := s.data
	if s.inline != nil && *s.inline.saveAsNew {
		// A parent saved as new gets new rows: the forms lose the primary
		// key and the foreign key they were rendered with.
		if s.st.data == nil {
			s.st.data = maps.Clone(s.data)
		}
		data = s.st.data
		data[f.AddPrefix(s.model.PrimaryKey().Attr)] = []string{""}
		data[f.AddPrefix(s.inline.fk.Attr)] = []string{""}
	}
	b := mf.Bind(data)
	if s.inline != nil {
		s.setParentFK(b)
	}
	s.st.forms[b.Form] = b
	return b.Form
}

// addFields adds the hidden primary key field, ORDER and DELETE, and the
// foreign key of an inline formset to form i, Django's add_fields of
// BaseModelFormSet and BaseInlineFormSet. The primary key is required on
// the initial forms.
func (s *ModelFormSet[T]) addFields(f *Form, i int) {
	mf := s.st.forms[f]
	if s.model == nil || mf == nil {
		s.baseAddFields(f, i)
		return
	}
	pk := s.model.PrimaryKey()
	initial := i >= 0 && i < s.InitialFormCount()
	// A primary key that is not on the form travels in a hidden field, so
	// that the submitted form finds its row again.
	if _, onForm := f.fields[pk.Attr]; isAutoPK(pk) || !pk.Editable || !onForm {
		f.AddField(pk.Attr, s.pkField(mf, initial))
	}
	s.baseAddFields(f, i)
	if s.inline != nil {
		s.inline.addField(f)
	}
	if field := f.fields[pk.Attr]; initial && field != nil && !field.Base().Required {
		field = copyField(field)
		field.Base().Required = true
		f.fields[pk.Attr] = field
	}
}

// pkField returns the hidden primary key field of a form. It shows the key
// of the row the form edits and cleans to that row.
func (s *ModelFormSet[T]) pkField(mf *ModelForm[T], required bool) Field {
	widget := mf.pkWidget
	if widget == nil {
		widget = HiddenInput{}
	}
	opts := []FieldOption{Required(required), UseWidget(widget)}
	if !mf.adding() {
		rv := reflect.ValueOf(mf.instance).Elem()
		opts = append(opts, Initial(rv.Field(s.model.PrimaryKey().Index).Interface()))
	}
	field := newModelChoiceField(collectOptions(opts))
	model := s.model
	field.charPK = isStringPK(model)
	field.pkOf = func(value any) (string, bool) { return instancePK(model, value) }
	field.load = s.loadPKRows
	return field
}

// loadPKRows returns the rows the hidden primary key fields accept, loaded
// once for all forms: the rows of the submitted keys for a bound formset, as
// Django's field accepts any row of the model, and the rows of the queryset
// for an unbound one.
func (s *ModelFormSet[T]) loadPKRows(ctx context.Context) ([]modelRow, error) {
	if s.st.pkRowsLoaded {
		return s.st.pkRows, nil
	}
	var objs []*T
	if s.bound {
		var keys []any
		for i := range s.TotalFormCount() {
			if raw, ok := s.submittedPK(i); ok {
				if _, value, ok := s.pkKey(raw); ok {
					keys = append(keys, value)
				}
			}
		}
		if len(keys) > 0 {
			rows, err := orm.Query[T](s.db).Filter("pk__in", keys).All(ctx)
			if err != nil {
				return nil, err
			}
			for i := range rows {
				objs = append(objs, &rows[i])
			}
		}
	} else {
		objs = s.objects()
	}
	s.st.pkRows = make([]modelRow, len(objs))
	for i, obj := range objs {
		key, _ := instancePK(s.model, obj)
		s.st.pkRows[i] = modelRow{pk: key, value: obj}
	}
	s.st.pkRowsLoaded = true
	return s.st.pkRows, nil
}

// copyField returns a shallow copy of a field, for a change that must not
// reach the other forms that share the field.
func copyField(field Field) Field {
	rv := reflect.ValueOf(field)
	if rv.Kind() != reflect.Pointer || rv.IsNil() {
		return field
	}
	cp := reflect.New(rv.Elem().Type())
	cp.Elem().Set(rv.Elem())
	c, ok := cp.Interface().(Field)
	if !ok {
		return field
	}
	if b := c.Base(); b.self == field {
		b.self = c
	}
	return c
}

// ModelForms returns the forms of the formset as model forms, in order.
func (s *ModelFormSet[T]) ModelForms() []*ModelForm[T] {
	forms := s.Forms()
	out := make([]*ModelForm[T], len(forms))
	for i, f := range forms {
		out[i] = s.st.forms[f]
	}
	return out
}

// validateUnique checks the unique fields across the valid forms that are
// not marked for deletion, Django's validate_unique of a model formset. A
// form that repeats the values of an earlier one gets the non-field error
// "Please correct the duplicate values below." and loses those values from
// its cleaned data; the formset gets one error per repetition.
func (s *ModelFormSet[T]) validateUnique() error {
	if s.model == nil {
		return nil
	}
	deleted := map[*Form]bool{}
	if s.CanDelete && s.formsValid() {
		for _, f := range s.deletedForms() {
			deleted[f] = true
		}
	}
	var valid []*Form
	var checks [][]string
	seenCheck := map[string]bool{}
	for _, f := range s.Forms() {
		if !f.IsValid() || deleted[f] {
			continue
		}
		valid = append(valid, f)
		mf := s.st.forms[f]
		for _, check := range s.model.UniqueChecks(mf.validationExclusions(f)...) {
			if key := strings.Join(check, "\x00"); !seenCheck[key] {
				seenCheck[key] = true
				checks = append(checks, check)
			}
		}
	}
	var errs []*ValidationError
	for _, check := range checks {
		seen := map[string]bool{}
		for _, f := range valid {
			key, ok := s.uniqueKey(f, check)
			if !ok {
				continue
			}
			if seen[key] {
				errs = append(errs, NewValidationError(s.uniqueErrorMessage(check), ""))
				f.replaceNonFieldErrors(NewValidationError("Please correct the duplicate values below.", ""))
				for _, name := range check {
					delete(f.cleaned, name)
				}
			}
			seen[key] = true
		}
	}
	if len(errs) > 0 {
		return &ValidationError{Errors: errs}
	}
	return nil
}

// uniqueKey returns the values of a unique check in the cleaned data of f as
// one comparable key. The second result is false when f has none of the
// values or one of them is empty. Rows are compared by primary key, and the
// foreign key of an inline formset is the same in every form.
func (s *ModelFormSet[T]) uniqueKey(f *Form, check []string) (string, bool) {
	var parts []string
	for _, name := range check {
		value, ok := f.cleaned[name]
		if !ok {
			continue
		}
		if s.inline != nil && name == s.inline.fk.Attr {
			parts = append(parts, name)
			continue
		}
		if value == nil {
			return "", false
		}
		if m, isModel := s.db.Registry().ModelOf(value); isModel {
			if pk, ok := instancePK(m, value); ok {
				value = pk
			}
		}
		parts = append(parts, fmt.Sprintf("%T:%v", value, value))
	}
	return strings.Join(parts, "\x00"), len(parts) > 0
}

// uniqueErrorMessage returns the error of the formset for a repeated unique
// check, Django's get_unique_error_message. The foreign key of an inline
// formset is left out.
func (s *ModelFormSet[T]) uniqueErrorMessage(check []string) string {
	var names []string
	for _, name := range check {
		if s.inline == nil || name != s.inline.fk.Attr {
			names = append(names, name)
		}
	}
	if len(names) == 1 {
		return "Please correct the duplicate data for " + names[0] + "."
	}
	return "Please correct the duplicate data for " + textList(names, "and") + ", which must be unique."
}

// textList joins items with commas and lastWord before the last one,
// Django's get_text_list.
func textList(items []string, lastWord string) string {
	if len(items) < 2 {
		return strings.Join(items, "")
	}
	return strings.Join(items[:len(items)-1], ", ") + " " + lastWord + " " + items[len(items)-1]
}

// Save validates the formset when needed and saves it: it changes the rows
// of the changed initial forms, deletes those of the forms marked for
// deletion, and adds a row for each changed extra form not marked for
// deletion, unless EditOnly is set. It returns the changed and the added
// rows. NewObjects, ChangedObjects and DeletedObjects report what it did.
// For a formset that is not valid it returns an error wrapping ErrInvalid
// and writes nothing.
func (s *ModelFormSet[T]) Save(ctx context.Context) ([]*T, error) {
	return s.SaveCommit(ctx, true)
}

// SaveCommit is Save with Django's commit argument. With commit false it
// writes nothing: it returns the instances of the changed and the added
// rows, which the caller saves before calling SaveM2M, and leaves the rows
// of the deleted forms in DeletedObjects for the caller to delete.
func (s *ModelFormSet[T]) SaveCommit(ctx context.Context, commit bool) ([]*T, error) {
	if s.ctx == nil {
		s.SetContext(ctx)
	}
	if !s.IsValid() {
		if err := s.Err(); err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("forms: the formset: %w", ErrInvalid)
	}
	s.st.savedForms = nil
	saved, err := s.saveExistingObjects(ctx, commit)
	if err != nil || s.EditOnly {
		return saved, err
	}
	added, err := s.saveNewObjects(ctx, commit)
	return append(saved, added...), err
}

// saveExistingObjects saves and deletes the rows of the initial forms,
// Django's save_existing_objects.
func (s *ModelFormSet[T]) saveExistingObjects(ctx context.Context, commit bool) ([]*T, error) {
	s.st.changedObjects, s.st.deletedObjects = []ChangedObject[T]{}, []*T{}
	deleted := map[*Form]bool{}
	for _, f := range s.DeletedForms() {
		deleted[f] = true
	}
	saved := []*T{}
	for _, f := range s.InitialForms() {
		mf := s.st.forms[f]
		// A form without its row (a key outside the queryset, or a row
		// deleted meanwhile) has nothing to change.
		if mf.adding() {
			continue
		}
		obj := mf.instance
		switch {
		case deleted[f]:
			s.st.deletedObjects = append(s.st.deletedObjects, obj)
			if err := s.deleteExisting(ctx, obj, commit); err != nil {
				return saved, err
			}
		case f.HasChanged():
			s.st.changedObjects = append(s.st.changedObjects, ChangedObject[T]{Object: obj, Fields: f.ChangedData()})
			out, err := s.saveExisting(ctx, mf, obj, commit)
			if err != nil {
				return saved, err
			}
			saved = append(saved, out)
			if !commit {
				s.st.savedForms = append(s.st.savedForms, mf)
			}
		}
	}
	return saved, nil
}

// saveNewObjects adds the rows of the changed extra forms, Django's
// save_new_objects.
func (s *ModelFormSet[T]) saveNewObjects(ctx context.Context, commit bool) ([]*T, error) {
	s.st.newObjects = []*T{}
	for _, f := range s.ExtraForms() {
		if !f.HasChanged() || (s.CanDelete && shouldDelete(f)) {
			continue
		}
		mf := s.st.forms[f]
		obj, err := s.saveNew(ctx, mf, commit)
		if err != nil {
			return s.st.newObjects, err
		}
		s.st.newObjects = append(s.st.newObjects, obj)
		if !commit {
			s.st.savedForms = append(s.st.savedForms, mf)
		}
	}
	return s.st.newObjects, nil
}

// saveNew saves the instance of an extra form with SaveNew or the form. An
// inline formset first points it at the parent as it is now, which may have
// been saved since the formset was built.
func (s *ModelFormSet[T]) saveNew(ctx context.Context, mf *ModelForm[T], commit bool) (*T, error) {
	if s.inline != nil {
		s.setParentFK(mf)
	}
	if s.SaveNew != nil {
		return s.SaveNew(ctx, mf, commit)
	}
	return saveForm(ctx, mf, commit)
}

// saveExisting saves the row of an initial form with SaveExisting or the
// form.
func (s *ModelFormSet[T]) saveExisting(ctx context.Context, mf *ModelForm[T], obj *T, commit bool) (*T, error) {
	if s.SaveExisting != nil {
		return s.SaveExisting(ctx, mf, obj, commit)
	}
	return saveForm(ctx, mf, commit)
}

// saveForm saves a model form, or with commit false returns its instance,
// Django's form.save(commit).
func saveForm[T any](ctx context.Context, mf *ModelForm[T], commit bool) (*T, error) {
	if !commit {
		return mf.Instance(), nil
	}
	return mf.Save(ctx)
}

// deleteExisting deletes a row with DeleteExisting, or itself when commit is
// true.
func (s *ModelFormSet[T]) deleteExisting(ctx context.Context, obj *T, commit bool) error {
	if s.DeleteExisting != nil {
		return s.DeleteExisting(ctx, obj, commit)
	}
	if !commit {
		return nil
	}
	pk := s.model.PrimaryKey()
	value := reflect.ValueOf(obj).Elem().Field(pk.Index).Interface()
	_, err := orm.Query[T](s.db).Filter("pk", value).Delete(ctx)
	return err
}

// SaveM2M writes the many-to-many fields of the forms that SaveCommit with
// commit false returned, once the caller has saved their instances.
func (s *ModelFormSet[T]) SaveM2M(ctx context.Context) error {
	for _, mf := range s.st.savedForms {
		if err := mf.SaveM2M(ctx); err != nil {
			return err
		}
	}
	return nil
}

// NewObjects returns the rows that the last Save added.
func (s *ModelFormSet[T]) NewObjects() []*T { return s.st.newObjects }

// ChangedObjects returns the rows that the last Save changed, with the
// changed fields.
func (s *ModelFormSet[T]) ChangedObjects() []ChangedObject[T] { return s.st.changedObjects }

// DeletedObjects returns the rows whose forms the last Save found marked for
// deletion.
func (s *ModelFormSet[T]) DeletedObjects() []*T { return s.st.deletedObjects }

// setParentFK points the instance of an inline form at the parent, as
// Django's BaseInlineFormSet._construct_form and save_new do.
func (s *ModelFormSet[T]) setParentFK(mf *ModelForm[T]) {
	if mf.model == nil || mf.instance == nil {
		return
	}
	if err := PopulateStruct(mf.model, map[string]any{s.inline.fk.Attr: s.inline.parent}, mf.instance); err != nil {
		s.fail(err)
	}
}

// inlineSpec is what an inline formset adds to a model formset: the foreign
// key of the forms' model to the parent and the parent instance.
type inlineSpec struct {
	fk          *orm.Field
	parentModel *orm.Model
	parent      any // a pointer to the parent instance
	saveAsNew   *bool
}

// parentHasPK reports whether the parent has a primary key.
func (in *inlineSpec) parentHasPK() bool { return hasPK(in.parentModel, in.parent) }

// parentPKValue returns the primary key of the parent.
func (in *inlineSpec) parentPKValue() any {
	rv, ok := structValue(in.parent)
	if !ok {
		return nil
	}
	return rv.Field(in.parentModel.PrimaryKey().Index).Interface()
}

// addField puts the hidden foreign key field on a form, Django's add_fields
// of BaseInlineFormSet. Its label is that of the field it replaces, or the
// verbose name of the foreign key.
func (in *inlineSpec) addField(f *Form) {
	name := in.fk.Attr
	field := &InlineForeignKeyField{parent: in.parent, parentModel: in.parentModel, pkField: in.fk.PrimaryKey}
	opts := []FieldOption{Required(false)}
	if !in.fk.PrimaryKey {
		label := fieldLabel(in.fk)
		if existing, ok := f.fields[name]; ok && existing.Base().Label != nil {
			label = *existing.Base().Label
		}
		opts = append(opts, Label(label))
	}
	if in.parentHasPK() {
		opts = append(opts, Initial(in.parentPKValue()))
	}
	field.init(field, HiddenInput{}, map[string]string{
		"invalid_choice": "The inline value did not match the parent instance.",
	}, collectOptions(opts))
	f.AddField(name, field)
}

// InlineForeignKeyField is the hidden field of an inline formset's form that
// holds the primary key of the parent, Django's InlineForeignKeyField. It
// cleans to the parent instance whatever was submitted, unless the value
// names another row, which fails with "The inline value did not match the
// parent instance." It never counts as changed. Model validation of a model
// form skips it; the uniqueness checks include it.
type InlineForeignKeyField struct {
	BaseField

	parent      any
	parentModel *orm.Model
	pkField     bool
}

// Clean returns the parent for an empty value or the key of the parent;
// when the foreign key is the primary key, an empty value cleans to nil.
func (f *InlineForeignKeyField) Clean(value any) (any, error) {
	if isEmpty(value) {
		if f.pkField {
			return nil, nil
		}
		return f.parent, nil
	}
	orig := "None"
	if hasPK(f.parentModel, f.parent) {
		orig, _ = instancePK(f.parentModel, f.parent)
	}
	if pyString(value) != orig {
		return nil, f.Error("invalid_choice")
	}
	return f.parent, nil
}

// HasChanged returns false: the parent of the form never changes.
func (f *InlineForeignKeyField) HasChanged(_, _ any) bool { return false }

// InlineFormSet is a model formset of the rows of T that point at a parent
// row of P through a foreign key, Django's inlineformset_factory and
// BaseInlineFormSet. The initial forms edit the rows of the parent, and the
// rows the extra forms add point at it:
//
//	fs := forms.NewInlineFormSet[Author, Book](db, "", forms.WithFields("title"))
//	bound := fs.WithInstance(author).Bind(r.PostForm)
//	if bound.IsValid() {
//		books, err := bound.Save(r.Context())
//	}
//
// The forms carry the foreign key in a hidden InlineForeignKeyField instead
// of a choice of parents. The defaults differ from a model formset as in
// Django: three extra forms, CanDelete, a MaxNum of 1 for a unique foreign
// key, and the reverse accessor name of the foreign key ("book_set") as the
// prefix.
type InlineFormSet[P, T any] struct {
	*ModelFormSet[T]
	// SaveAsNew makes every form add a row, for a parent that is saved as a
	// new row ("save as new" in the admin), Django's save_as_new. The forms
	// drop the primary keys and foreign keys they were rendered with.
	SaveAsNew bool
}

// NewInlineFormSet returns an unbound inline formset of T for a new parent
// of P, built with opts as NewModelForm builds one form. fkName names the
// foreign key of T to P, Django's fk_name; "" finds the only one. A foreign
// key that is missing, ambiguous or not to P makes the formset fail (Err)
// with Django's message.
func NewInlineFormSet[P, T any](db *orm.DB, fkName string, opts ...ModelFormOption) *InlineFormSet[P, T] {
	s := &InlineFormSet[P, T]{ModelFormSet: NewModelFormSet[T](db, opts...)}
	s.Extra, s.CanDelete = 3, true
	parentModel, ok := db.Registry().ModelOf(new(P))
	if !ok {
		s.fail(fmt.Errorf("forms: %T is not a registered model", *new(P)))
		return s
	}
	if s.model == nil {
		return s
	}
	fk, err := inlineForeignKey(parentModel, s.model, fkName)
	if err != nil {
		s.fail(err)
		return s
	}
	if fk.Unique || fk.Rel.Kind == orm.RelO2O {
		s.MaxNum = 1
	}
	s.Prefix = accessorName(s.model, fk)
	s.inline = &inlineSpec{fk: fk, parentModel: parentModel, parent: new(P), saveAsNew: &s.SaveAsNew}
	return s
}

// inlineForeignKey returns the foreign key of model to parent, Django's
// _get_foreign_key.
func inlineForeignKey(parent, model *orm.Model, fkName string) (*orm.Field, error) {
	isFK := func(f *orm.Field) bool {
		return f.Rel != nil && (f.Rel.Kind == orm.RelFK || f.Rel.Kind == orm.RelO2O)
	}
	if fkName != "" {
		f, ok := model.FieldByAttr(fkName)
		if !ok {
			return nil, fmt.Errorf("forms: '%s' has no field named '%s'.", model.Label(), fkName) //nolint:revive,staticcheck // Django's message
		}
		if !isFK(f) || f.Rel.Target != parent {
			return nil, fmt.Errorf("forms: fk_name '%s' is not a ForeignKey to '%s'.", fkName, parent.Label()) //nolint:revive,staticcheck // Django's message
		}
		return f, nil
	}
	var found []*orm.Field
	for _, f := range model.Fields() {
		if isFK(f) && f.Rel.Target == parent {
			found = append(found, f)
		}
	}
	switch len(found) {
	case 1:
		return found[0], nil
	case 0:
		return nil, fmt.Errorf("forms: '%s' has no ForeignKey to '%s'.", model.Label(), parent.Label()) //nolint:revive,staticcheck // Django's message
	}
	return nil, fmt.Errorf("forms: '%s' has more than one ForeignKey to '%s'. You must specify a 'fk_name' attribute.", //nolint:revive,staticcheck // Django's message
		model.Label(), parent.Label())
}

// accessorName returns the name of the reverse accessor of fk without "+",
// Django's get_default_prefix of an inline formset.
func accessorName(model *orm.Model, fk *orm.Field) string {
	name := fk.Rel.RelatedName
	if name == "" {
		name = strings.ToLower(model.Name())
		if fk.Rel.Kind != orm.RelO2O {
			name += "_set"
		}
	}
	return strings.ReplaceAll(name, "+", "")
}

// WithInstance returns a copy of the formset for the rows of parent,
// Django's instance argument. Without it, or with nil, the parent is a new,
// unsaved row and the formset has no initial forms.
func (s *InlineFormSet[P, T]) WithInstance(parent *P) *InlineFormSet[P, T] {
	if parent == nil {
		parent = new(P)
	}
	c := *s
	c.ModelFormSet = s.derive(s.clone())
	if c.inline != nil {
		c.inline.parent = parent
		c.inline.saveAsNew = &c.SaveAsNew
	}
	return &c
}

// Bind returns a copy of the formset bound to data. The receiver stays
// unbound.
func (s *InlineFormSet[P, T]) Bind(data url.Values) *InlineFormSet[P, T] {
	c := *s
	c.ModelFormSet = s.ModelFormSet.Bind(data)
	if c.inline != nil {
		c.inline.saveAsNew = &c.SaveAsNew
	}
	return &c
}

// Instance returns the parent of the rows.
func (s *InlineFormSet[P, T]) Instance() *P {
	if s.inline == nil {
		return nil
	}
	parent, _ := s.inline.parent.(*P)
	return parent
}

// FK returns the foreign key of T to P, nil when the formset failed to find
// it.
func (s *InlineFormSet[P, T]) FK() *orm.Field {
	if s.inline == nil {
		return nil
	}
	return s.inline.fk
}
