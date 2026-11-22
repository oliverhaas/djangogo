package forms

import (
	"context"
	"maps"
	"reflect"

	"github.com/oliverhaas/djangogo/orm"
)

// ModelMultipleChoiceField is a field whose value is any number of the rows of
// a model, the counterpart of Django's ModelMultipleChoiceField. It renders as
// a <select multiple> without an empty option and submits the primary keys, or
// the values of the field named with ToFieldName.
//
// The cleaned value depends on the constructor: NewModelMultipleChoiceField
// yields the selected objects ([]*T, in the order of the rows), a many-to-many
// field of a model form yields their primary keys ([]any). A field that is not
// required cleans an empty selection to an empty list.
type ModelMultipleChoiceField struct {
	ModelChoiceField

	// collect returns the cleaned value of the selected rows.
	collect func(rows []modelRow) any
}

// newModelMultipleChoiceField returns a ModelMultipleChoiceField without a
// data source whose cleaned value is the list of row values.
func newModelMultipleChoiceField(o *fieldOptions) *ModelMultipleChoiceField {
	f := &ModelMultipleChoiceField{collect: rowValues}
	f.init(f, SelectMultiple{}, map[string]string{
		"invalid_list":     "Enter a list of values.",
		"invalid_choice":   invalidChoiceMessage,
		"invalid_pk_value": "“%(pk)s” is not a valid value.",
	}, o)
	return f
}

// rowValues returns the cleaned values of rows as a []any.
func rowValues(rows []modelRow) any {
	out := make([]any, len(rows))
	for i, r := range rows {
		out[i] = r.value
	}
	return out
}

// NewModelMultipleChoiceField returns a field that offers the rows of qs, or
// every row of T when qs is nil. The cleaned value is a []*T of the selected
// rows. Beside the common options it reads ToFieldName and LabelFromInstance.
//
// Each render queries the rows, and cleaning queries the submitted ones, with
// the context given to Form.SetContext.
func NewModelMultipleChoiceField[T any](db *orm.DB, qs *orm.QuerySet[T], opts ...FieldOption) *ModelMultipleChoiceField {
	o := collectOptions(opts)
	f := newModelMultipleChoiceField(o)
	model, _ := db.Registry().ModelOf(new(T))
	key := keyField(model, o.toFieldName)
	f.key, f.toFieldName = key, o.toFieldName
	f.keyOf = func(value any) (string, bool) { return instanceKey(model, key, value) }
	f.load, f.find = queryRows(db, qs, model, key, labelFunc[T](o))
	f.collect = func(rows []modelRow) any {
		out := make([]*T, len(rows))
		for i, r := range rows {
			out[i], _ = r.value.(*T)
		}
		return out
	}
	return f
}

// newRelatedMultipleChoiceField returns the field of the many-to-many relation
// rel. With a database the rows of its target that match its LimitChoicesTo
// are the options; without one the options are set with Form.SetChoices. The
// cleaned value is the list of primary keys.
func newRelatedMultipleChoiceField(db *orm.DB, rel *orm.Relation, o *fieldOptions) *ModelMultipleChoiceField {
	f := newModelMultipleChoiceField(o)
	target := rel.Target
	f.key = keyField(target, "")
	f.keyOf = func(value any) (string, bool) { return instancePK(target, value) }
	if db != nil && target != nil {
		f.load, f.find = relatedRows(db, rel)
	}
	return f
}

// WithContext implements ContextField: it returns a copy whose queries of
// cleaning run with ctx. It queries nothing itself.
func (f *ModelMultipleChoiceField) WithContext(ctx context.Context) (Field, error) {
	clone := *f
	clone.self = &clone
	clone.ctx = ctx
	return &clone, nil
}

// listItems returns the items of a list value: a slice or an array other than
// a string or a []byte. The second result is false for any other value.
func listItems(value any) ([]any, bool) {
	switch v := value.(type) {
	case nil, string, []byte:
		return nil, false
	case []any:
		return v, true
	case []string:
		out := make([]any, len(v))
		for i, s := range v {
			out[i] = s
		}
		return out, true
	}
	rv := reflect.ValueOf(value)
	if rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array {
		return nil, false
	}
	out := make([]any, rv.Len())
	for i := range out {
		out[i] = rv.Index(i).Interface()
	}
	return out, true
}

// unhashable reports whether a Python counterpart of v could not be put in a
// set, which makes Django reject the list it is in.
func unhashable(v any) bool {
	switch reflect.ValueOf(v).Kind() {
	case reflect.Slice, reflect.Map, reflect.Func:
		_, isBytes := v.([]byte)
		return !isBytes
	default:
		return false
	}
}

// PrepareValue returns the keys of a list of objects or keys as strings, and
// the key of a single object.
func (f *ModelMultipleChoiceField) PrepareValue(value any) any {
	if items, ok := listItems(value); ok {
		out := make([]string, len(items))
		for i, item := range items {
			out[i] = pyString(f.ModelChoiceField.PrepareValue(item))
		}
		return out
	}
	return f.ModelChoiceField.PrepareValue(value)
}

// Clean returns the selected rows, Django's ModelMultipleChoiceField.clean: an
// empty selection is "required" or the empty list, a value that is not a list
// is "invalid_list", a key that cannot be a primary key is
// "invalid_pk_value" and a key that selects no row is "invalid_choice". The
// validators run over the list of submitted keys.
func (f *ModelMultipleChoiceField) Clean(value any) (any, error) {
	items, isList := listItems(value)
	if isList && len(items) == 0 || !isList && isEmpty(value) {
		if f.Required {
			return nil, f.Error("required")
		}
		return f.collect(nil), nil
	}
	if !isList {
		return nil, f.Error("invalid_list")
	}
	keys := make([]string, 0, len(items))
	wanted := make(map[string]bool, len(items))
	for _, item := range items {
		if unhashable(item) {
			return nil, f.Error("invalid_list")
		}
		key := pyString(f.ModelChoiceField.PrepareValue(item))
		if !wanted[key] {
			wanted[key] = true
			keys = append(keys, key)
		}
	}
	values := make([]any, len(keys))
	for i, key := range keys {
		if err := ProhibitNullCharactersValidator(key); err != nil {
			return nil, err
		}
		v, ok := keyValue(f.key, key)
		if !ok {
			return nil, f.Error("invalid_pk_value", map[string]any{"pk": key})
		}
		values[i] = v
	}
	rows, err := f.rowsOfKeys(keys, values)
	if err != nil {
		return nil, err
	}
	var chosen []modelRow
	found := make(map[string]bool, len(keys))
	for _, r := range rows {
		if wanted[r.key] {
			chosen = append(chosen, r)
			found[r.key] = true
		}
	}
	for _, key := range keys {
		if !found[key] {
			return nil, f.Error("invalid_choice", map[string]any{"value": key})
		}
	}
	if err := f.RunValidators(keys); err != nil {
		return nil, err
	}
	return f.collect(chosen), nil
}

// HasChanged compares the primary keys of the initial value with the
// submitted keys as sets.
func (f *ModelMultipleChoiceField) HasChanged(initial, data any) bool {
	if f.Disabled {
		return false
	}
	a, _ := f.PrepareValue(orEmptyList(initial)).([]string)
	b, _ := f.PrepareValue(orEmptyList(data)).([]string)
	if len(a) != len(b) {
		return true
	}
	return !maps.Equal(stringSet(a), stringSet(b))
}

// stringSet returns the set of the strings in list.
func stringSet(list []string) map[string]bool {
	out := make(map[string]bool, len(list))
	for _, s := range list {
		out[s] = true
	}
	return out
}

// orEmptyList returns an empty list for nil and v otherwise.
func orEmptyList(v any) any {
	if v == nil {
		return []any{}
	}
	return v
}
