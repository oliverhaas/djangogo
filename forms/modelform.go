package forms

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"maps"
	"net/url"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/oliverhaas/djangogo/orm"
)

// ErrInvalid is returned by ModelForm.Save for a form whose data did not
// validate. Check IsValid before saving.
var ErrInvalid = errors.New("forms: the data didn't validate")

// invalidSaveError is the error of Save for a form that is not valid, with
// the message of Django's ValueError. It wraps ErrInvalid.
type invalidSaveError string

// Error returns the message.
func (e invalidSaveError) Error() string { return string(e) }

// Unwrap returns ErrInvalid.
func (invalidSaveError) Unwrap() error { return ErrInvalid }

// FromModel derives an unbound form from a model, as a Django ModelForm does.
// Each editable model field becomes a form field named after its attribute
// ("created_at"); the auto primary key and non-editable fields are left out.
// Required follows the blank option, the label verbose_name, and help text and
// default carry over. A foreign key becomes a ModelChoiceField whose options
// the caller sets with Form.SetChoices; NewModelForm loads them itself.
func FromModel(m *orm.Model, opts ...ModelFormOption) *Form {
	form, _ := buildModelForm(nil, m, newModelFormConfig(opts))
	return form
}

// buildModelForm derives the form of a model; db may be nil. It also returns
// the set of model field attribute names included by WithFields/WithExclude
// (excluding fields declared only via WithFormField outside Meta.fields or
// inside Meta.exclude). A WithFields name that the form cannot have makes the
// form fail with the FieldError message of Django's ModelFormMetaclass and
// fields_for_model.
func buildModelForm(db *orm.DB, m *orm.Model, cfg *modelFormConfig) (*Form, map[string]bool) {
	cfg.normalize(m)
	var all []string
	for _, mf := range formModelFields(m) {
		if editable(mf) {
			all = append(all, mf.Attr)
		}
	}
	form := New()
	included := map[string]bool{}
	var unknown []string
	for _, name := range cfg.includedNames(all) {
		field := cfg.formFields[name]
		mf, isModelField := modelField(m, name)
		switch {
		case isModelField && !editable(mf) && !isAutoPK(mf):
			form.fail(fmt.Errorf("forms: '%s' cannot be specified for %s model form as it is a non-editable field",
				name, m.Name()))
			continue
		case field != nil:
		case !isModelField:
			if name != AllFields {
				unknown = append(unknown, name)
			}
			continue
		case isAutoPK(mf):
			continue
		default:
			field = formFieldFor(db, mf, cfg.factories[name], cfg.classes[name], cfg.extraOptions[name])
			if cfg.callback != nil && field != nil {
				field = cfg.callback(mf, field)
			}
		}
		if field == nil {
			continue
		}
		if isModelField {
			included[name] = true
		}
		cfg.apply(name, field)
		form.AddField(name, field)
	}
	// The declared fields that WithFields leaves out or WithExclude drops are
	// part of the form all the same, as in Django.
	for _, name := range cfg.declared {
		if _, ok := form.FieldDef(name); !ok {
			cfg.apply(name, cfg.formFields[name])
			form.AddField(name, cfg.formFields[name])
		}
	}
	for _, fn := range cfg.cleaners {
		form.CleanWith(fn)
	}
	for name, fn := range cfg.fieldCleaners {
		form.CleanFieldWith(name, fn)
	}
	if len(unknown) > 0 {
		form.fail(fmt.Errorf("forms: Unknown field(s) (%s) specified for %s", strings.Join(unknown, ", "), m.Name()))
	}
	return form, included
}

// editable reports whether a model field appears in a model form.
func editable(mf *orm.Field) bool {
	if isM2M(mf) {
		return mf.Editable
	}
	return mf.Editable && mf.Kind != orm.KindAuto && !mf.AutoNow && !mf.AutoNowAdd
}

// isM2M reports whether mf is a forward many-to-many field.
func isM2M(mf *orm.Field) bool { return mf.Rel != nil && mf.Rel.Kind == orm.RelM2M }

// isAutoPK reports whether mf is an auto primary key. A many-to-many field has
// the zero Kind as well, but it is not one.
func isAutoPK(mf *orm.Field) bool { return mf.Kind == orm.KindAuto && !isM2M(mf) }

// formModelFields returns the fields of m that a model form can have: the
// concrete fields and the many-to-many fields, in declaration order, as
// Django's fields_for_model sorts them.
func formModelFields(m *orm.Model) []*orm.Field {
	fields := slices.Concat(m.Fields(), m.ManyToMany())
	slices.SortStableFunc(fields, func(a, b *orm.Field) int { return cmp.Compare(a.Index, b.Index) })
	return fields
}

// modelField returns the concrete or many-to-many field of m with the given
// attribute name.
func modelField(m *orm.Model, name string) (*orm.Field, bool) {
	if mf, ok := m.FieldByAttr(name); ok {
		return mf, true
	}
	if mf, ok := m.ManyToManyField(name); ok && mf.Attr == name {
		return mf, true
	}
	return nil, false
}

// fieldLabel returns the label of a model field: its verbose name, or its
// attribute name with spaces, with the first letter capitalized.
func fieldLabel(mf *orm.Field) string {
	name := mf.VerboseName
	if name == "" {
		name = strings.ReplaceAll(mf.Attr, "_", " ")
	}
	return capfirst(name)
}

// structValue returns the struct that obj is or points to.
func structValue(obj any) (reflect.Value, bool) {
	rv := reflect.ValueOf(obj)
	for rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return reflect.Value{}, false
		}
		rv = rv.Elem()
	}
	return rv, rv.Kind() == reflect.Struct
}

// relationPK returns the primary key held by a relation field.
func relationPK(fv reflect.Value) int64 {
	method := fv.MethodByName("PK")
	if !method.IsValid() {
		return 0
	}
	out := method.Call(nil)
	if len(out) != 1 || !out[0].CanInt() {
		return 0
	}
	return out[0].Int()
}

// FromInstance derives an unbound form from a model for obj, a pointer to an
// instance of it, and validates the bound form against the model as a
// ModelForm does: validation fills obj from the cleaned data, also when the
// form is invalid, and runs the model validation and the unique checks of
// orm.Model.FullClean and orm.ValidateUnique with db. Unlike FromStruct, it
// leaves the initial values to the caller, and unlike NewModelForm, the
// foreign key fields get their options from the caller, through
// Form.SetChoices. It accepts the options of FromModel.
//
//	form := forms.FromInstance(db, model, post, forms.WithFields("title")).Bind(r.PostForm)
//	if form.IsValid() {
//		// post holds the cleaned data.
//	}
func FromInstance(db *orm.DB, m *orm.Model, obj any, opts ...ModelFormOption) *Form {
	cfg := newModelFormConfig(opts)
	form, included := buildModelForm(nil, m, cfg)
	side := newModelSide(db, m, cfg, included)
	adding := !hasPK(m, obj)
	form.postCleaners = []func(*Form){func(f *Form) {
		side.postClean(f, obj, func() bool { return adding })
	}}
	return form
}

// InitialFromStruct returns the initial values of a model form for obj, an
// instance of the model or a pointer to one, Django's model_to_dict. Only the
// named fields are included; without names, all are.
func InitialFromStruct(m *orm.Model, obj any, names ...string) map[string]any {
	out := map[string]any{}
	rv, ok := structValue(obj)
	if !ok || rv.Type() != m.GoType() {
		return out
	}
	wanted := map[string]bool{}
	for _, n := range names {
		wanted[n] = true
	}
	for _, mf := range m.Fields() {
		if len(wanted) > 0 && !wanted[mf.Attr] {
			continue
		}
		out[mf.Attr] = fieldInitial(mf, rv.Field(mf.Index))
	}
	return out
}

// FromStruct derives an unbound form from a model and fills its initial
// values from obj, an instance of the model or a pointer to one, as a Django
// ModelForm does for instance=obj. It accepts the options of FromModel.
func FromStruct(m *orm.Model, obj any, opts ...ModelFormOption) *Form {
	f := FromModel(m, opts...)
	f.Initial = InitialFromStruct(m, obj, f.names...)
	return f
}

// PopulateStruct copies cleaned form data into dest, a pointer to an instance
// of the model, Django's construct_instance. Entries are matched by attribute
// name; a model field without an entry keeps its value. A nil value sets the
// field to its zero value, and a relation takes a primary key (int64 or
// string) or an instance of the related model; a pointer to an instance is
// kept on the relation.
func PopulateStruct(m *orm.Model, cleaned map[string]any, dest any) error {
	rv := reflect.ValueOf(dest)
	if rv.Kind() != reflect.Pointer || rv.IsNil() || rv.Elem().Kind() != reflect.Struct {
		return fmt.Errorf("forms: PopulateStruct requires a pointer to a struct, got %T", dest)
	}
	elem := rv.Elem()
	if elem.Type() != m.GoType() {
		return fmt.Errorf("forms: PopulateStruct: %T is not an instance of the model %s", dest, m.Name())
	}
	for _, mf := range m.Fields() {
		val, ok := cleaned[mf.Attr]
		if !ok {
			continue
		}
		fv := elem.Field(mf.Index)
		if mf.Rel != nil {
			pk, err := relatedPK(mf, val)
			if err != nil {
				return err
			}
			// An instance stays on the relation, as Django's
			// construct_instance assigns it; saving takes its primary key
			// then, also when it was saved after the form was cleaned.
			if obj := reflect.ValueOf(val); obj.Kind() == reflect.Pointer && !obj.IsNil() &&
				obj.Elem().Type() == mf.Rel.Target.GoType() {
				if setObject := fv.Addr().MethodByName("SetObject"); setObject.IsValid() &&
					setObject.Type().In(0) == obj.Type() {
					setObject.Call([]reflect.Value{obj, reflect.ValueOf(pk)})
					continue
				}
			}
			setPK := fv.Addr().MethodByName("SetPK")
			if !setPK.IsValid() {
				return fmt.Errorf("forms: field %s: relation type %s has no SetPK method", mf.Attr, fv.Type())
			}
			setPK.Call([]reflect.Value{reflect.ValueOf(pk)})
			continue
		}
		if err := setScalar(mf, fv, val); err != nil {
			return err
		}
	}
	return nil
}

// relatedPK converts the cleaned value of a relation to the primary key of the
// related row; nil and "" mean no row.
func relatedPK(mf *orm.Field, val any) (int64, error) {
	switch v := val.(type) {
	case nil:
		return 0, nil
	case string:
		if strings.TrimSpace(v) == "" {
			return 0, nil
		}
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("forms: field %s: %q is not a primary key", mf.Attr, v)
		}
		return n, nil
	}
	if n, ok := toInt(val); ok {
		return n, nil
	}
	if s, ok := instancePK(mf.Rel.Target, val); ok {
		if n, err := strconv.ParseInt(s, 10, 64); err == nil {
			return n, nil
		}
	}
	return 0, fmt.Errorf("forms: field %s: cannot use %T as a primary key", mf.Attr, val)
}

// ModelForm is a form derived from the model T that saves an instance of it,
// the counterpart of Django's ModelForm. It embeds the Form, so everything a
// form does is available on it.
//
//	form := forms.NewModelForm[Post](db, forms.WithFields("title", "slug")).Bind(r.PostForm)
//	form.SetContext(r.Context())
//	if form.IsValid() {
//		post, err := form.Save(r.Context())
//	}
//
// Validation fills the instance from the cleaned data, runs model validation
// (orm.Model.FullClean with WithValidateUnique(false)), and validates unique
// and unique_together constraints against the database (orm.ValidateUnique),
// attaching errors to form fields or non-field errors just as Django's
// BaseModelForm._post_clean does.
type ModelForm[T any] struct {
	*Form
	modelSide

	instance *T
	own      bool   // the caller supplied the instance
	pkWidget Widget // the WithWidget of the primary key, for a model formset
}

// modelSide is the part of a model form that does not depend on the Go type
// of the instance: what the model validation of FromInstance and ModelForm
// needs.
type modelSide struct {
	db                    *orm.DB
	model                 *orm.Model
	includedFields        map[string]bool
	nonFieldErrorMessages map[string]string
}

// newModelSide returns the model side of a form built from cfg, with the
// model fields the form includes.
func newModelSide(db *orm.DB, m *orm.Model, cfg *modelFormConfig, included map[string]bool) modelSide {
	side := modelSide{db: db, model: m, includedFields: included}
	if msgs, ok := cfg.errorMessages[NonFieldErrors]; ok && len(msgs) > 0 {
		side.nonFieldErrorMessages = maps.Clone(msgs)
	}
	return side
}

// NewModelForm returns an unbound form for a new instance of T. A foreign key
// field loads its options from the database when the form renders or
// validates. T must be registered with the database's registry; otherwise the
// form has no fields and Err reports the mistake.
func NewModelForm[T any](db *orm.DB, opts ...ModelFormOption) *ModelForm[T] {
	mf := &ModelForm[T]{modelSide: modelSide{db: db}, instance: new(T)}
	model, ok := db.Registry().ModelOf(mf.instance)
	if !ok {
		mf.Form = New()
		mf.fail(fmt.Errorf("forms: %T is not a registered model", *mf.instance))
		return mf
	}
	cfg := newModelFormConfig(opts)
	mf.model = model
	mf.instance = mf.newInstance()
	form, included := buildModelForm(db, model, cfg)
	mf.Form, mf.modelSide = form, newModelSide(db, model, cfg, included)
	if pk := model.PrimaryKey(); pk != nil {
		mf.pkWidget = cfg.widgets[pk.Attr]
	}
	mf.install()
	return mf
}

// newInstance returns a new instance of T with the defaults of the model
// fields, as Django's Model() starts with them.
func (mf *ModelForm[T]) newInstance() *T {
	obj := new(T)
	if mf.model == nil {
		return obj
	}
	rv := reflect.ValueOf(obj).Elem()
	for _, field := range mf.model.Fields() {
		if field.HasDefault {
			// A default that does not fit the Go field is left to the ORM.
			_ = setScalar(field, rv.Field(field.Index), field.Default)
		}
	}
	return obj
}

// install makes the form run the model validation of this ModelForm.
func (mf *ModelForm[T]) install() {
	mf.postCleaners = []func(*Form){mf.postClean}
}

// WithInstance returns a copy of the form that edits obj: the form shows the
// values of obj, validation writes the cleaned data into it, and Save updates
// its row when it has a primary key. A nil obj is a new instance, as
// Django's instance=None.
func (mf *ModelForm[T]) WithInstance(obj *T) *ModelForm[T] {
	c := *mf
	c.Form = mf.clone()
	c.bound, c.data = mf.bound, mf.data
	if obj == nil {
		c.instance, c.own = mf.newInstance(), false
		c.install()
		return &c
	}
	c.instance, c.own = obj, true
	if mf.model != nil {
		c.Initial = InitialFromStruct(mf.model, obj, c.names...)
		for k, v := range mf.Initial {
			c.Initial[k] = v
		}
		c.loadInitial = c.manyInitial(obj)
		c.resolved = false
	}
	c.install()
	return &c
}

// manyInitial returns the loader of the initial values of the many-to-many
// fields of the form, Django's model_to_dict for them: the primary keys of
// the rows related to obj, and none for an unsaved obj. A field with
// ToFieldName gets the keys of the rows instead, which Django's prepare_value
// takes from the related objects. An initial value the caller set wins.
func (mf *ModelForm[T]) manyInitial(obj *T) func(ctx context.Context, f *Form) error {
	model, db, included := mf.model, mf.db, mf.includedFields
	return func(ctx context.Context, f *Form) error {
		for _, field := range model.ManyToMany() {
			if !included[field.Attr] {
				continue
			}
			if _, set := f.Initial[field.Attr]; set {
				continue
			}
			if f.Initial == nil {
				f.Initial = map[string]any{}
			}
			if !hasPK(model, obj) {
				f.Initial[field.Attr] = []any{}
				continue
			}
			pks, err := orm.ManyValues(ctx, db, obj, field)
			if err != nil {
				return fmt.Errorf("forms: initial value of %s: %w", field.Attr, err)
			}
			f.Initial[field.Attr] = pks
			if err := keyedInitial(ctx, f, field, pks); err != nil {
				return err
			}
		}
		return nil
	}
}

// keyedInitial replaces the initial primary keys of the many-to-many field
// with the keys of a form field that has ToFieldName. It loads the form field
// to map them and keeps the loaded field, so that the form does not load it
// again.
func keyedInitial(ctx context.Context, f *Form, field *orm.Field, pks []any) error {
	mf, ok := f.fields[field.Attr].(*ModelMultipleChoiceField)
	if !ok || mf.toFieldName == "" {
		return nil
	}
	loaded, err := mf.WithContext(ctx)
	if err != nil {
		return fmt.Errorf("forms: initial value of %s: %w", field.Attr, err)
	}
	f.fields[field.Attr] = loaded
	f.Initial[field.Attr] = loaded.(*ModelMultipleChoiceField).keysOfPKs(field.Rel.Target, pks)
	return nil
}

// hasPK reports whether obj, an instance of m, has a primary key value.
func hasPK(m *orm.Model, obj any) bool {
	pk := m.PrimaryKey()
	rv, ok := structValue(obj)
	return ok && pk != nil && !rv.Field(pk.Index).IsZero()
}

// Bind returns a copy of the form bound to data. Without WithInstance the
// copy edits a new instance of its own.
func (mf *ModelForm[T]) Bind(data url.Values) *ModelForm[T] {
	c := *mf
	c.Form = mf.Form.Bind(data)
	if !mf.own {
		c.instance = mf.newInstance()
	}
	c.install()
	return &c
}

// Instance returns the instance the form edits. After validation it holds the
// cleaned data.
func (mf *ModelForm[T]) Instance() *T { return mf.instance }

// Model returns the model of the form.
func (mf *ModelForm[T]) Model() *orm.Model { return mf.model }

// adding reports whether the form creates a row, Django's
// instance._state.adding: the form made its own instance, or the instance of
// WithInstance has no primary key yet.
func (mf *ModelForm[T]) adding() bool {
	if !mf.own {
		return true
	}
	return !hasPK(mf.model, mf.instance)
}

// constructData returns the cleaned data that goes into the instance, Django's
// construct_instance: model fields omitted by WithFields/WithExclude are
// skipped, and a field with a model default keeps that default when the data
// has no value for it and its cleaned value is empty.
func (mf *modelSide) constructData(f *Form) map[string]any {
	data, cloned := f.cleaned, false
	cloneIfNeeded := func() {
		if !cloned {
			data, cloned = maps.Clone(f.cleaned), true
		}
	}
	for _, field := range mf.model.Fields() {
		value, ok := data[field.Attr]
		if !ok {
			continue
		}
		if !editable(field) || !mf.includedFields[field.Attr] {
			cloneIfNeeded()
			delete(data, field.Attr)
			continue
		}
		if !field.HasDefault || !isEmpty(value) {
			continue
		}
		def, ok := f.fields[field.Attr]
		if !ok || !def.Base().Widget.ValueOmittedFromData(f.data, f.AddPrefix(field.Attr)) {
			continue
		}
		cloneIfNeeded()
		delete(data, field.Attr)
	}
	return data
}

// validationExclusions returns the model field attribute names excluded from
// model validation and uniqueness checks, matching Django's
// BaseModelForm._get_validation_exclusions.
func (mf *modelSide) validationExclusions(f *Form) []string {
	var exclude []string
	for _, field := range mf.model.Fields() {
		name := field.Attr
		formField, onForm := f.fields[name]
		switch {
		case !onForm:
			exclude = append(exclude, name)
		case !mf.includedFields[name]:
			exclude = append(exclude, name)
		case f.HasError(name):
			exclude = append(exclude, name)
		case !field.Blank && !formField.Base().Required && isEmpty(f.cleaned[name]):
			exclude = append(exclude, name)
		}
	}
	return exclude
}

// updateErrors merges a model-level *ValidationError into f, overriding
// message templates by code from the matching form field (or
// WithErrorMessages(NonFieldErrors, ...)) as Django's
// BaseModelForm._update_errors does.
func (mf *modelSide) updateErrors(f *Form, ve *ValidationError) {
	errorDict := ve.Fields
	if !ve.IsDict() {
		errorDict = map[string][]*ValidationError{NonFieldErrors: ve.ErrorList()}
	}
	patched := make(map[string][]*ValidationError, len(errorDict))
	for field, list := range errorDict {
		var overrides map[string]string
		if field == NonFieldErrors {
			overrides = mf.nonFieldErrorMessages
		} else if ff, ok := f.fields[field]; ok {
			overrides = ff.Base().ErrorMessages
		}
		target := field
		if field != NonFieldErrors && f.fields[field] == nil {
			target = NonFieldErrors
		}
		for _, item := range list {
			cp := *item
			if overrides != nil && cp.Code != "" {
				if msg, ok := overrides[cp.Code]; ok {
					cp.Message = msg
				}
			}
			patched[target] = append(patched[target], &cp)
		}
	}
	f.AddError("", &ValidationError{Fields: patched})
}

// postClean runs the model validation of the form for its instance.
func (mf *ModelForm[T]) postClean(f *Form) {
	if mf.model == nil || mf.instance == nil {
		return
	}
	mf.modelSide.postClean(f, mf.instance, mf.adding)
}

// postClean is the model part of validation, Django's BaseModelForm._post_clean:
// it fills instance, a pointer to an instance of the model, from the cleaned
// data, runs instance.FullClean with validate_unique disabled, and then runs
// orm.ValidateUnique with recomputed exclusions. adding reports whether the
// instance is a new row, Django's instance._state.adding.
func (mf *modelSide) postClean(f *Form, instance any, adding func() bool) {
	if err := PopulateStruct(mf.model, mf.constructData(f), instance); err != nil {
		var ve *ValidationError
		if errors.As(err, &ve) {
			mf.updateErrors(f, ve)
		} else {
			f.fail(err)
			return
		}
	}

	exclude := mf.validationExclusions(f)
	// The foreign key of an inline formset holds a parent that may not be
	// saved yet, so model validation leaves it out.
	for name, field := range f.fields {
		if _, ok := field.(*InlineForeignKeyField); ok {
			exclude = append(exclude, name)
		}
	}
	if err := mf.model.FullClean(f.Context(), mf.db, instance,
		orm.WithExclude(exclude...),
		orm.WithValidateUnique(false),
	); err != nil {
		var ve *ValidationError
		if errors.As(err, &ve) {
			mf.updateErrors(f, ve)
		} else {
			f.fail(fmt.Errorf("forms: model clean: %w", err))
		}
	}

	if mf.db != nil {
		uniqueExclude := mf.validationExclusions(f)
		if err := orm.ValidateUnique(f.Context(), mf.db, instance,
			orm.WithExclude(uniqueExclude...),
			orm.WithAdding(adding()),
		); err != nil {
			var ve *ValidationError
			if errors.As(err, &ve) {
				mf.updateErrors(f, ve)
			} else {
				f.fail(fmt.Errorf("forms: unique check: %w", err))
			}
		}
	}
}

// Save validates the form when that has not happened yet and writes the
// instance to the database: an INSERT when the form creates a row (it has no
// WithInstance, or the instance has no primary key), an UPDATE of its row
// otherwise. An UPDATE that finds no row inserts it, as Django's Model.save
// does. Then it writes the many-to-many fields with SaveM2M. It returns the
// saved instance. For a form that is not valid it returns an error wrapping
// ErrInvalid and writes nothing.
func (mf *ModelForm[T]) Save(ctx context.Context) (*T, error) {
	if mf.model == nil {
		return nil, mf.Err()
	}
	if mf.ctx == nil {
		mf.SetContext(ctx)
	}
	adding := mf.adding()
	if !mf.IsValid() {
		if err := mf.Err(); err != nil {
			return nil, err
		}
		verb := "changed"
		if adding {
			verb = "created"
		}
		return nil, invalidSaveError(fmt.Sprintf("forms: The %s could not be %s because the data didn't validate.",
			mf.model.Name(), verb))
	}
	if err := mf.saveInstance(ctx, adding); err != nil {
		return nil, err
	}
	if err := mf.SaveM2M(ctx); err != nil {
		return nil, err
	}
	return mf.instance, nil
}

// saveInstance writes the instance: an INSERT when adding, an UPDATE of its
// row otherwise, and an INSERT when the UPDATE finds no row.
func (mf *ModelForm[T]) saveInstance(ctx context.Context, adding bool) error {
	if adding {
		return mf.create(ctx)
	}
	rv := reflect.ValueOf(mf.instance).Elem()
	pk := mf.model.PrimaryKey()
	var assignments []any
	for _, field := range mf.model.Fields() {
		// The ORM stamps the automatic timestamps itself.
		if field.PrimaryKey || field.AutoNow || field.AutoNowAdd {
			continue
		}
		assignments = append(assignments, field.Column, rv.Field(field.Index).Interface())
	}
	if len(assignments) == 0 {
		return nil
	}
	n, err := orm.Query[T](mf.db).Filter(pk.Column, rv.Field(pk.Index).Interface()).Update(ctx, assignments...)
	if err != nil {
		return err
	}
	if n == 0 {
		return mf.create(ctx)
	}
	return nil
}

// create inserts the instance. From then on the form edits the saved row, as
// a Django instance is no longer adding after its save.
func (mf *ModelForm[T]) create(ctx context.Context) error {
	if err := orm.Query[T](mf.db).Create(ctx, mf.instance); err != nil {
		return err
	}
	mf.own = true
	return nil
}

// SaveM2M writes the many-to-many fields of a valid form for its saved
// instance, Django's save_m2m: the related rows of each field become those of
// its cleaned value, a list of primary keys or of related objects. Save calls
// it after it writes the instance. A caller that saves Instance() itself, the
// counterpart of Django's save(commit=False), calls it afterwards. A
// many-to-many field that WithFields or WithExclude leaves out is not written.
func (mf *ModelForm[T]) SaveM2M(ctx context.Context) error {
	if mf.model == nil {
		return mf.Err()
	}
	if mf.ctx == nil {
		mf.SetContext(ctx)
	}
	if !mf.IsValid() {
		if err := mf.Err(); err != nil {
			return err
		}
		return fmt.Errorf("forms: SaveM2M: %w", ErrInvalid)
	}
	cleaned := mf.CleanedData()
	for _, field := range mf.model.ManyToMany() {
		if !mf.includedFields[field.Attr] {
			continue
		}
		value, ok := cleaned[field.Attr]
		if !ok {
			continue
		}
		items, isList := listItems(value)
		if !isList && value != nil {
			return fmt.Errorf("forms: field %s: cannot use %T as a list of related rows", field.Attr, value)
		}
		if err := orm.SetManyPKs(ctx, mf.db, mf.instance, field, items); err != nil {
			return err
		}
	}
	return nil
}
