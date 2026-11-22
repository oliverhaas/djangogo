package forms

import (
	"context"
	"fmt"
	"reflect"
	"strconv"

	"github.com/oliverhaas/djangogo/orm"
)

// choiceField is implemented by the fields that hand choices to a ChoiceWidget.
type choiceField interface {
	// widgetChoices returns the options the widget offers.
	widgetChoices() []Choice
	// setChoices replaces the options.
	setChoices(choices []Choice)
}

// ChoiceField is a field whose value is one of a fixed set of options; its
// cleaned value is a string.
type ChoiceField struct {
	BaseField
	// Choices are the options.
	Choices []Choice
}

// invalidChoiceMessage is the message for a value outside the choices.
const invalidChoiceMessage = "Select a valid choice. %(value)s is not one of the available choices."

// NewChoiceField returns a ChoiceField. It reads the options Choices and
// ChoiceList beside the common ones.
func NewChoiceField(opts ...FieldOption) *ChoiceField {
	o := collectOptions(opts)
	f := &ChoiceField{Choices: o.choices}
	f.init(f, Select{}, map[string]string{"invalid_choice": invalidChoiceMessage}, o)
	return f
}

// widgetChoices implements choiceField.
func (f *ChoiceField) widgetChoices() []Choice { return f.Choices }

// setChoices implements choiceField.
func (f *ChoiceField) setChoices(choices []Choice) { f.Choices = choices }

// ValidValue reports whether value is one of the choices, also inside an
// option group. The name of a group is not a choice.
func (f *ChoiceField) ValidValue(value string) bool {
	for _, c := range f.Choices {
		if len(c.Group) == 0 && c.Value == value {
			return true
		}
		for _, sub := range c.Group {
			if sub.Value == value {
				return true
			}
		}
	}
	return false
}

// ToGo returns the value as a string.
func (f *ChoiceField) ToGo(value any) (any, error) {
	if isEmpty(value) {
		return "", nil
	}
	return pyString(value), nil
}

// Validate rejects a value that is not one of the choices.
func (f *ChoiceField) Validate(value any) error {
	if err := f.BaseField.Validate(value); err != nil {
		return err
	}
	if s, _ := value.(string); s != "" && !f.ValidValue(s) {
		return f.Error("invalid_choice", map[string]any{"value": s})
	}
	return nil
}

// TypedChoiceField is a ChoiceField that converts the chosen value with a
// function; its cleaned value is what the function returns.
type TypedChoiceField struct {
	ChoiceField
	// Coerce converts a valid choice. An error makes the choice invalid.
	Coerce func(string) (any, error)
	// EmptyValue is the cleaned value of an empty submission, "" by default.
	EmptyValue any
}

// NewTypedChoiceField returns a TypedChoiceField. It reads the options Coerce
// and EmptyValue beside those of a ChoiceField.
func NewTypedChoiceField(opts ...FieldOption) *TypedChoiceField {
	o := collectOptions(opts)
	f := &TypedChoiceField{Coerce: o.coerce, EmptyValue: ""}
	if o.hasEmptyValue {
		f.EmptyValue = o.emptyValue
	}
	f.Choices = o.choices
	f.init(f, Select{}, map[string]string{"invalid_choice": invalidChoiceMessage}, o)
	return f
}

// Clean validates the choice and then converts it.
func (f *TypedChoiceField) Clean(value any) (any, error) {
	v, err := f.BaseField.Clean(value)
	if err != nil {
		return nil, err
	}
	return f.coerce(v)
}

// coerce converts a validated choice.
func (f *TypedChoiceField) coerce(v any) (any, error) {
	s, _ := v.(string)
	if isEmpty(v) || v == f.EmptyValue {
		return f.EmptyValue, nil
	}
	if f.Coerce == nil {
		return s, nil
	}
	out, err := f.Coerce(s)
	if err != nil {
		return nil, f.Error("invalid_choice", map[string]any{"value": s})
	}
	return out, nil
}

// HasChanged compares the converted values.
func (f *TypedChoiceField) HasChanged(initial, data any) bool {
	if f.Disabled {
		return false
	}
	a, errA := f.coerce(orEmptyString(data))
	b, errB := f.coerce(pyString(orEmptyString(initial)))
	if errA != nil || errB != nil {
		return true
	}
	return !valuesEqual(a, b)
}

// MultipleChoiceField is a field whose value is any number of the options;
// its cleaned value is a []string.
type MultipleChoiceField struct {
	ChoiceField
}

// NewMultipleChoiceField returns a MultipleChoiceField.
func NewMultipleChoiceField(opts ...FieldOption) *MultipleChoiceField {
	o := collectOptions(opts)
	f := &MultipleChoiceField{}
	f.Choices = o.choices
	f.init(f, SelectMultiple{}, map[string]string{
		"invalid_choice": invalidChoiceMessage,
		"invalid_list":   "Enter a list of values.",
	}, o)
	return f
}

// stringList converts a list value to strings. The second result is false for
// a value that is not a list.
func stringList(value any) ([]string, bool) {
	switch v := value.(type) {
	case nil:
		return []string{}, true
	case []string:
		return append([]string{}, v...), true
	case []any:
		out := make([]string, len(v))
		for i, item := range v {
			out[i] = pyString(item)
		}
		return out, true
	case string:
		if v == "" {
			return []string{}, true
		}
	}
	return nil, false
}

// ToGo returns the submitted values as a []string.
func (f *MultipleChoiceField) ToGo(value any) (any, error) {
	list, ok := stringList(value)
	if !ok {
		return nil, f.Error("invalid_list")
	}
	return list, nil
}

// Validate rejects an empty list for a required field and any value that is
// not one of the choices.
func (f *MultipleChoiceField) Validate(value any) error {
	list, _ := value.([]string)
	if f.Required && len(list) == 0 {
		return f.Error("required")
	}
	for _, v := range list {
		if !f.ValidValue(v) {
			return f.Error("invalid_choice", map[string]any{"value": v})
		}
	}
	return nil
}

// HasChanged compares both lists as sets.
func (f *MultipleChoiceField) HasChanged(initial, data any) bool {
	if f.Disabled {
		return false
	}
	a, _ := stringList(initial)
	b, _ := stringList(data)
	if len(a) != len(b) {
		return true
	}
	seen := map[string]bool{}
	for _, v := range a {
		seen[v] = true
	}
	for _, v := range b {
		if !seen[v] {
			return true
		}
	}
	return false
}

// TypedMultipleChoiceField is a MultipleChoiceField that converts each chosen
// value with a function; its cleaned value is a []any of the converted values.
type TypedMultipleChoiceField struct {
	MultipleChoiceField
	// Coerce converts a valid choice. An error makes the choice invalid.
	Coerce func(string) (any, error)
	// EmptyValue is the cleaned value of an empty submission, an empty []any
	// by default.
	EmptyValue any
}

// NewTypedMultipleChoiceField returns a TypedMultipleChoiceField. It reads the
// options Coerce and EmptyValue beside those of a MultipleChoiceField.
func NewTypedMultipleChoiceField(opts ...FieldOption) *TypedMultipleChoiceField {
	o := collectOptions(opts)
	f := &TypedMultipleChoiceField{Coerce: o.coerce, EmptyValue: []any{}}
	if o.hasEmptyValue {
		f.EmptyValue = o.emptyValue
	}
	f.Choices = o.choices
	f.init(f, SelectMultiple{}, map[string]string{
		"invalid_choice": invalidChoiceMessage,
		"invalid_list":   "Enter a list of values.",
	}, o)
	return f
}

// Clean validates the choices and then converts them.
func (f *TypedMultipleChoiceField) Clean(value any) (any, error) {
	v, err := f.BaseField.Clean(value)
	if err != nil {
		return nil, err
	}
	list, _ := v.([]string)
	if len(list) == 0 {
		return f.EmptyValue, nil
	}
	out := make([]any, len(list))
	for i, s := range list {
		out[i] = s
		if f.Coerce == nil {
			continue
		}
		if out[i], err = f.Coerce(s); err != nil {
			return nil, f.Error("invalid_choice", map[string]any{"value": s})
		}
	}
	return out, nil
}

// modelRow is one selectable object of a ModelChoiceField.
type modelRow struct {
	key   string // the submitted value: the primary key or the ToFieldName field
	label string
	value any // the cleaned value: the object or its primary key
}

// ContextField is implemented by a field that needs a database to know its
// options. A form calls WithContext once before it renders or validates and
// uses the returned field from then on.
type ContextField interface {
	Field
	// WithContext returns a copy of the field with its data loaded.
	WithContext(ctx context.Context) (Field, error)
}

// ModelChoiceField is a field whose options are the rows of a model, the
// counterpart of Django's ModelChoiceField. It renders as a <select> with an
// empty first option and submits the primary key, or the field named with
// ToFieldName. As in Django, the empty option is left out for a required field
// with an initial value and for a RadioSelect unless Blank is set.
//
// The cleaned value depends on the constructor: NewModelChoiceField yields the
// selected object (*T), a foreign key of a model form yields the primary key
// (int64). An empty selection cleans to nil.
type ModelChoiceField struct {
	BaseField
	// EmptyLabel is the label of the empty option; nil leaves the option out.
	EmptyLabel *string
	// Blank keeps the empty option on a RadioSelect, Django's blank argument.
	// The field of a foreign key takes it from the blank option of the model
	// field.
	Blank bool

	rows    []modelRow
	loaded  bool
	charKey bool
	// toFieldName is the attribute of ToFieldName, empty for the primary key.
	toFieldName string
	load        func(ctx context.Context) ([]modelRow, error)
	// keyOf returns the submitted key of an initial value that is an object.
	keyOf func(value any) (string, bool)
}

// keyField returns the field of m whose value a model choice field submits:
// the primary key, or the field that ToFieldName names. It panics when m has
// no such field, a programming error.
func keyField(m *orm.Model, toFieldName string) *orm.Field {
	if m == nil {
		return nil
	}
	if toFieldName == "" {
		return m.PrimaryKey()
	}
	f, ok := m.FieldByAttr(toFieldName)
	if !ok {
		panic(fmt.Sprintf("forms: ToFieldName(%q): model %s has no such field", toFieldName, m.Name()))
	}
	return f
}

// isStringField reports whether f is a character-based field whose input
// must prohibit NUL characters, matching Django's ModelChoiceField validation.
func isStringField(f *orm.Field) bool {
	if f == nil {
		return false
	}
	switch f.Kind {
	case orm.KindChar, orm.KindText, orm.KindSlug, orm.KindEmail, orm.KindURL:
		return true
	default:
		return false
	}
}

// invalidModelChoiceMessage is the message for a key that selects no row.
const invalidModelChoiceMessage = "Select a valid choice. That choice is not one of the available choices."

// newModelChoiceField returns a ModelChoiceField without a data source.
func newModelChoiceField(o *fieldOptions) *ModelChoiceField {
	f := &ModelChoiceField{EmptyLabel: o.emptyLabel}
	f.init(f, Select{}, map[string]string{"invalid_choice": invalidModelChoiceMessage}, o)
	return f
}

// NewModelChoiceField returns a field that offers the rows of qs, or every row
// of T when qs is nil. The cleaned value is the selected *T. Beside the common
// options it reads EmptyLabel, NoEmptyLabel, ToFieldName and
// LabelFromInstance.
//
// The rows are loaded when the form renders or validates, with the context
// given to Form.SetContext.
func NewModelChoiceField[T any](db *orm.DB, qs *orm.QuerySet[T], opts ...FieldOption) *ModelChoiceField {
	o := collectOptions(opts)
	f := newModelChoiceField(o)
	model, _ := db.Registry().ModelOf(new(T))
	key := keyField(model, o.toFieldName)
	f.charKey, f.toFieldName = isStringField(key), o.toFieldName
	f.keyOf = func(value any) (string, bool) { return instanceKey(model, key, value) }
	f.load = queryRows(db, qs, model, key, labelFunc[T](o))
	return f
}

// labelFunc returns the function of LabelFromInstance, nil without one. It
// panics when the function is for another model than T.
func labelFunc[T any](o *fieldOptions) func(*T) string {
	if o.labelOf == nil {
		return nil
	}
	fn, ok := o.labelOf.(func(*T) string)
	if !ok {
		panic(fmt.Sprintf("forms: LabelFromInstance: got a %T for a field of %s", o.labelOf, reflect.TypeFor[T]()))
	}
	return fn
}

// queryRows returns the loader of the rows of qs, or of every row of T when
// qs is nil, whose cleaned values are the objects (*T). The key of a row is
// the value of key, and its label comes from labelOf or, when it is nil, the
// model's String method.
func queryRows[T any](db *orm.DB, qs *orm.QuerySet[T], model *orm.Model, key *orm.Field, labelOf func(*T) string) func(context.Context) ([]modelRow, error) {
	if qs == nil {
		qs = orm.Query[T](db)
	}
	return func(ctx context.Context) ([]modelRow, error) {
		objs, err := qs.All(ctx)
		if err != nil {
			return nil, err
		}
		rows := make([]modelRow, len(objs))
		for i := range objs {
			obj := &objs[i]
			k, _ := instanceKey(model, key, obj)
			var label string
			if labelOf != nil {
				label = labelOf(obj)
			} else if label, err = orm.LabelContext(ctx, db, model, obj); err != nil {
				return nil, err
			}
			rows[i] = modelRow{key: k, label: label, value: obj}
		}
		return rows, nil
	}
}

// newRelatedChoiceField returns the field of the foreign key rel. With a
// database the rows of its target that match its LimitChoicesTo are the
// options; without one the options are set with Form.SetChoices. The cleaned
// value is the primary key.
func newRelatedChoiceField(db *orm.DB, rel *orm.Relation, o *fieldOptions) *ModelChoiceField {
	f := newModelChoiceField(o)
	target := rel.Target
	f.charKey = isStringField(keyField(target, ""))
	f.keyOf = func(value any) (string, bool) { return instancePK(target, value) }
	if db != nil && target != nil {
		f.load = relatedRows(db, rel)
	}
	return f
}

// relatedRows returns the loader of the options of a relation field: the rows
// of the target of rel that match its LimitChoicesTo, which is evaluated on
// each load.
func relatedRows(db *orm.DB, rel *orm.Relation) func(context.Context) ([]modelRow, error) {
	return func(ctx context.Context) ([]modelRow, error) {
		pairs, err := orm.LabeledRowsWhere(ctx, db, rel.Target, rel.LimitChoicesTo(), nil, nil)
		if err != nil {
			return nil, err
		}
		return pairRows(pairs), nil
	}
}

// pairRows converts (primary key, label) pairs to rows whose cleaned value is
// the integer primary key.
func pairRows(pairs [][2]string) []modelRow {
	rows := make([]modelRow, len(pairs))
	for i, p := range pairs {
		var value any = p[0]
		if pk, err := strconv.ParseInt(p[0], 10, 64); err == nil {
			value = pk
		}
		rows[i] = modelRow{key: p[0], label: p[1], value: value}
	}
	return rows
}

// instancePK returns the primary key of a model instance as the string a
// widget submits. The second result is false when value is not an instance of
// the model.
func instancePK(m *orm.Model, value any) (string, bool) {
	return instanceKey(m, keyField(m, ""), value)
}

// instanceKey returns the value of the field key of a model instance as the
// string a widget submits. The second result is false when value is not an
// instance of the model.
func instanceKey(m *orm.Model, key *orm.Field, value any) (string, bool) {
	if m == nil || key == nil || value == nil {
		return "", false
	}
	rv := reflect.ValueOf(value)
	for rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return "", false
		}
		rv = rv.Elem()
	}
	if rv.Type() != m.GoType() {
		return "", false
	}
	v := rv.Field(key.Index).Interface()
	// A key that is a relation, such as a one-to-one primary key, is the
	// primary key of its row.
	if rel, ok := v.(interface{ PK() int64 }); ok && key.Rel != nil {
		return strconv.FormatInt(rel.PK(), 10), true
	}
	return pyString(v), true
}

// WithContext implements ContextField: it returns a copy with the rows loaded.
func (f *ModelChoiceField) WithContext(ctx context.Context) (Field, error) {
	if f.load == nil || f.loaded {
		return f, nil
	}
	rows, err := f.load(ctx)
	if err != nil {
		return f, err
	}
	clone := *f
	clone.self = &clone
	clone.rows, clone.loaded = rows, true
	return &clone, nil
}

// widgetChoices implements choiceField: the empty option, then one option per
// row.
func (f *ModelChoiceField) widgetChoices() []Choice {
	out := make([]Choice, 0, len(f.rows)+1)
	if f.hasEmptyOption() {
		out = append(out, Choice{Value: "", Label: *f.EmptyLabel})
	}
	for _, r := range f.rows {
		out = append(out, Choice{Value: r.key, Label: r.label})
	}
	return out
}

// hasEmptyOption reports whether the field offers the empty option, the rule
// of Django's ModelChoiceField.__init__ for empty_label.
func (f *ModelChoiceField) hasEmptyOption() bool {
	if f.EmptyLabel == nil || f.Required && f.Initial != nil {
		return false
	}
	_, radio := unwrapWidget(f.Widget).(RadioSelect)
	return !radio || f.Blank
}

// unwrapWidget returns the widget that w wraps, following the Unwrap methods
// of wrappers such as the admin's RelatedFieldWidgetWrapper, so that a rule
// that depends on the kind of widget sees through the wrapper.
func unwrapWidget(w Widget) Widget {
	for {
		u, ok := w.(interface{ Unwrap() Widget })
		if !ok {
			return w
		}
		w = u.Unwrap()
	}
}

// setChoices implements choiceField: the choices become the rows, and an
// empty-valued choice is dropped because the field adds its own.
func (f *ModelChoiceField) setChoices(choices []Choice) {
	pairs := make([][2]string, 0, len(choices))
	for _, c := range choices {
		if c.Value != "" {
			pairs = append(pairs, [2]string{c.Value, c.Label})
		}
	}
	f.rows, f.loaded = pairRows(pairs), true
}

// PrepareValue returns the submitted key of an object, its primary key or the
// ToFieldName field, and any other value unchanged.
func (f *ModelChoiceField) PrepareValue(value any) any {
	if f.keyOf != nil {
		if pk, ok := f.keyOf(value); ok {
			return pk
		}
	}
	return value
}

// ToGo returns the cleaned value of the row whose key was submitted.
func (f *ModelChoiceField) ToGo(value any) (any, error) {
	if isEmpty(value) {
		return nil, nil
	}
	key := pyString(f.PrepareValue(value))
	if f.charKey {
		if err := ProhibitNullCharactersValidator(key); err != nil {
			return nil, err
		}
	}
	for _, r := range f.rows {
		if r.key == key {
			return r.value, nil
		}
	}
	return nil, f.Error("invalid_choice", map[string]any{"value": key})
}

// keysOfPKs returns the keys of the loaded rows of target whose primary keys
// are pks, in the order of pks. A primary key without a loaded row is left
// out.
func (f *ModelChoiceField) keysOfPKs(target *orm.Model, pks []any) []any {
	byPK := make(map[string]string, len(f.rows))
	for _, r := range f.rows {
		if pk, ok := instancePK(target, r.value); ok {
			byPK[pk] = r.key
		}
	}
	out := make([]any, 0, len(pks))
	for _, pk := range pks {
		if key, ok := byPK[pyString(pk)]; ok {
			out = append(out, key)
		}
	}
	return out
}

// HasChanged compares the submitted key with the key of the initial value.
func (f *ModelChoiceField) HasChanged(initial, data any) bool {
	if f.Disabled {
		return false
	}
	return pyString(f.PrepareValue(orEmptyString(initial))) != pyString(orEmptyString(data))
}
