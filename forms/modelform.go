package forms

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"maps"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"time"
	"unicode"

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
	return buildModelForm(nil, m, newModelFormConfig(opts))
}

// buildModelForm derives the form of a model; db may be nil. A WithFields
// name that the form cannot have makes the form fail with the FieldError
// message of Django's ModelFormMetaclass and fields_for_model.
func buildModelForm(db *orm.DB, m *orm.Model, cfg *modelFormConfig) *Form {
	cfg.normalize(m)
	var all []string
	for _, mf := range m.Fields() {
		if editable(mf) {
			all = append(all, mf.Attr)
		}
	}
	form := New()
	var unknown []string
	for _, name := range cfg.includedNames(all) {
		field := cfg.formFields[name]
		mf, isModelField := m.FieldByAttr(name)
		switch {
		case isModelField && !editable(mf) && mf.Kind != orm.KindAuto:
			form.fail(fmt.Errorf("forms: '%s' cannot be specified for %s model form as it is a non-editable field",
				name, m.Name()))
			continue
		case field != nil:
		case !isModelField:
			if name != AllFields {
				unknown = append(unknown, name)
			}
			continue
		case mf.Kind == orm.KindAuto:
			continue
		default:
			field = formFieldFor(db, mf, cfg.factories[name])
		}
		if field == nil {
			continue
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
	if len(unknown) > 0 {
		form.fail(fmt.Errorf("forms: Unknown field(s) (%s) specified for %s", strings.Join(unknown, ", "), m.Name()))
	}
	return form
}

// editable reports whether a model field appears in a model form.
func editable(mf *orm.Field) bool {
	return mf.Editable && mf.Kind != orm.KindAuto && !mf.AutoNow && !mf.AutoNowAdd
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

// formFieldFor returns the form field of a model field, Django's
// Field.formfield, or nil for a kind that has none.
func formFieldFor(db *orm.DB, mf *orm.Field, factory fieldFactory) Field {
	opts := []FieldOption{Required(!mf.Blank), Label(fieldLabel(mf)), HelpText(mf.HelpText)}
	if mf.HasDefault {
		opts = append(opts, Initial(mf.Default))
	}
	if mf.Rel != nil {
		var field Field
		if factory != nil {
			field = factory(db, opts)
		}
		if field == nil {
			field = newRelatedChoiceField(db, mf.Rel.Target, collectOptions(opts))
		}
		if choice, ok := field.(*ModelChoiceField); ok {
			choice.Blank = mf.Blank
		}
		return field
	}
	if len(mf.Choices) > 0 {
		choices := make([]Choice, 0, len(mf.Choices)+1)
		// The empty option lets the user leave the field unset; a field with a
		// default starts on that default and needs none.
		if mf.Blank || !mf.HasDefault {
			choices = append(choices, Choice{Value: "", Label: EmptyChoiceLabel})
		}
		for _, c := range mf.Choices {
			choices = append(choices, Choice{Value: c.Value, Label: c.Label})
		}
		return NewTypedChoiceField(append(opts, ChoiceList(choices))...)
	}
	switch mf.Kind {
	case orm.KindChar:
		return NewCharField(append(opts, MaxLength(mf.MaxLength))...)
	case orm.KindText:
		return NewCharField(append(opts, UseWidget(Textarea{}))...)
	case orm.KindInt:
		return NewIntegerField(opts...)
	case orm.KindBool:
		// An unchecked box is a valid "false", so the field is never required.
		return NewBooleanField(append(opts, Required(false))...)
	case orm.KindDateTime:
		return NewDateTimeField(opts...)
	default:
		return nil
	}
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

// fieldInitial returns the value of a model field of an instance as a form
// shows it: the primary key for a relation, nil for an unset relation or time.
func fieldInitial(mf *orm.Field, fv reflect.Value) any {
	if mf.Rel != nil {
		pk := relationPK(fv)
		if pk == 0 {
			return nil
		}
		return pk
	}
	v := fv.Interface()
	if t, ok := v.(time.Time); ok && t.IsZero() {
		return nil
	}
	return v
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
// string) or an instance of the related model.
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

// setScalar assigns a cleaned value to a struct field, converting between the
// numeric types and filling pointers and sql.Scanner types such as
// sql.NullString.
func setScalar(mf *orm.Field, fv reflect.Value, val any) error {
	if val == nil {
		fv.SetZero()
		return nil
	}
	rv := reflect.ValueOf(val)
	if rv.Type().AssignableTo(fv.Type()) {
		fv.Set(rv)
		return nil
	}
	switch fv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if n, ok := toInt(val); ok && !fv.OverflowInt(n) {
			fv.SetInt(n)
			return nil
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if n, ok := toInt(val); ok && n >= 0 && !fv.OverflowUint(uint64(n)) {
			fv.SetUint(uint64(n))
			return nil
		}
	case reflect.Float32, reflect.Float64:
		if n, ok := toFloat(val); ok {
			fv.SetFloat(n)
			return nil
		}
	case reflect.String:
		if s, ok := val.(string); ok {
			fv.SetString(s)
			return nil
		}
	case reflect.Pointer:
		target := reflect.New(fv.Type().Elem())
		if err := setScalar(mf, target.Elem(), val); err != nil {
			return err
		}
		fv.Set(target)
		return nil
	}
	if scanner, ok := fv.Addr().Interface().(sql.Scanner); ok {
		if err := scanner.Scan(val); err != nil {
			return fmt.Errorf("forms: field %s: %w", mf.Attr, err)
		}
		return nil
	}
	return fmt.Errorf("forms: field %s: cannot assign %T to %s", mf.Attr, val, fv.Type())
}

// modelVerboseName returns the human-readable name of a model, derived from
// its Go type name: "BlogPost" becomes "blog post".
func modelVerboseName(m *orm.Model) string {
	var b strings.Builder
	runes := []rune(m.Name())
	for i, c := range runes {
		if i > 0 && unicode.IsUpper(c) &&
			(unicode.IsLower(runes[i-1]) || (i+1 < len(runes) && unicode.IsLower(runes[i+1]))) {
			b.WriteByte(' ')
		}
		b.WriteRune(unicode.ToLower(c))
	}
	return b.String()
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
// Validation fills the instance from the cleaned data and checks the unique
// fields against the database, so that a duplicate is a form error ("Post with
// this Slug already exists.") and not a failed INSERT.
type ModelForm[T any] struct {
	*Form

	db       *orm.DB
	model    *orm.Model
	instance *T
	own      bool // the caller supplied the instance
}

// NewModelForm returns an unbound form for a new instance of T. A foreign key
// field loads its options from the database when the form renders or
// validates. T must be registered with the database's registry; otherwise the
// form has no fields and Err reports the mistake.
func NewModelForm[T any](db *orm.DB, opts ...ModelFormOption) *ModelForm[T] {
	mf := &ModelForm[T]{db: db, instance: new(T)}
	model, ok := db.Registry().ModelOf(mf.instance)
	if !ok {
		mf.Form = New()
		mf.fail(fmt.Errorf("forms: %T is not a registered model", *mf.instance))
		return mf
	}
	mf.model = model
	mf.instance = mf.newInstance()
	mf.Form = buildModelForm(db, model, newModelFormConfig(opts))
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
// its row when it has a primary key.
func (mf *ModelForm[T]) WithInstance(obj *T) *ModelForm[T] {
	c := *mf
	c.Form = mf.clone()
	c.bound, c.data = mf.bound, mf.data
	c.instance, c.own = obj, true
	if mf.model != nil && obj != nil {
		c.Initial = InitialFromStruct(mf.model, obj, c.names...)
		for k, v := range mf.Initial {
			c.Initial[k] = v
		}
	}
	c.install()
	return &c
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
	pk := mf.model.PrimaryKey()
	return pk == nil || reflect.ValueOf(mf.instance).Elem().Field(pk.Index).IsZero()
}

// constructData returns the cleaned data that goes into the instance, Django's
// construct_instance: a field with a model default keeps that default when the
// data has no value for it and its cleaned value is empty.
func (mf *ModelForm[T]) constructData(f *Form) map[string]any {
	data, cloned := f.cleaned, false
	for _, field := range mf.model.Fields() {
		value, ok := data[field.Attr]
		if !field.HasDefault || !ok || !isEmpty(value) {
			continue
		}
		def, ok := f.fields[field.Attr]
		if !ok || !def.Base().Widget.ValueOmittedFromData(f.data, f.AddPrefix(field.Attr)) {
			continue
		}
		if !cloned {
			data, cloned = maps.Clone(f.cleaned), true
		}
		delete(data, field.Attr)
	}
	return data
}

// postClean is the model part of validation, Django's _post_clean: it fills
// the instance from the cleaned data and validates the unique fields,
// including a primary key that the form sets.
func (mf *ModelForm[T]) postClean(f *Form) {
	if mf.model == nil || mf.instance == nil {
		return
	}
	if err := PopulateStruct(mf.model, mf.constructData(f), mf.instance); err != nil {
		f.fail(err)
		return
	}
	rv := reflect.ValueOf(mf.instance).Elem()
	pk := mf.model.PrimaryKey()
	for _, field := range mf.model.Fields() {
		// A field that failed its own validation, or is not part of the form,
		// is not checked, as in Django.
		if _, ok := f.cleaned[field.Attr]; !ok || !field.Unique && !field.PrimaryKey {
			continue
		}
		value := fieldInitial(field, rv.Field(field.Index))
		if value == nil {
			continue // NULL never collides
		}
		q := orm.Query[T](mf.db).Filter(field.Column, value)
		if pk != nil && !mf.adding() {
			q = q.Exclude(pk.Column, rv.Field(pk.Index).Interface())
		}
		exists, err := q.Exists(f.context())
		if err != nil {
			f.fail(fmt.Errorf("forms: unique check of %s: %w", field.Attr, err))
			continue
		}
		if exists {
			f.AddError(field.Attr, NewValidationError(uniqueMessage(f, field.Attr), "unique", map[string]any{
				"model_name":  capfirst(modelVerboseName(mf.model)),
				"field_label": fieldLabel(field),
			}))
		}
	}
}

// uniqueMessage returns the message of a unique error on a field: the
// "unique" message of the form field when it has one (WithErrorMessages), as
// Django's ModelForm._update_errors replaces the message of a model error,
// otherwise the default of the model field.
func uniqueMessage(f *Form, name string) string {
	if def, ok := f.fields[name]; ok {
		if msg, ok := def.Base().ErrorMessages["unique"]; ok {
			return msg
		}
	}
	return "%(model_name)s with this %(field_label)s already exists."
}

// Save validates the form when that has not happened yet and writes the
// instance to the database: an INSERT when the form creates a row (it has no
// WithInstance, or the instance has no primary key), an UPDATE of its row
// otherwise. An UPDATE that finds no row inserts it, as Django's Model.save
// does. It returns the saved instance. For a form that is not valid it returns
// an error wrapping ErrInvalid and writes nothing.
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
		return mf.instance, nil
	}
	n, err := orm.Query[T](mf.db).Filter(pk.Column, rv.Field(pk.Index).Interface()).Update(ctx, assignments...)
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return mf.create(ctx)
	}
	return mf.instance, nil
}

// create inserts the instance. From then on the form edits the saved row, as
// a Django instance is no longer adding after its save.
func (mf *ModelForm[T]) create(ctx context.Context) (*T, error) {
	if err := orm.Query[T](mf.db).Create(ctx, mf.instance); err != nil {
		return nil, err
	}
	mf.own = true
	return mf.instance, nil
}
