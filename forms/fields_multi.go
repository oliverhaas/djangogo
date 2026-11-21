package forms

import (
	"maps"
	"time"
)

// MultiValueField cleans a list of values, one per subfield, and compresses
// the cleaned values into one, Django's MultiValueField. Its widget is
// usually a MultiWidget with one subwidget per subfield.
//
// By default every subfield must have a value when the field is required, and
// the subfields themselves are made optional. With RequireAllFields(false) a
// required field only rejects a list without any value, and each required
// subfield without a value adds the subfield's "incomplete" message.
type MultiValueField struct {
	BaseField
	// Fields are the subfields; value i of the list is cleaned by field i.
	Fields []Field
	// RequireAllFields requires every subfield to have a value when the field
	// is required.
	RequireAllFields bool

	// compress turns the cleaned values into the cleaned value of the field.
	compress func(values []any) (any, error)
}

// NewMultiValueField returns a MultiValueField over fields. compress turns
// the list of cleaned values into the field's cleaned value, Django's
// compress; it gets an empty list for an optional field without data. A nil
// compress keeps the list. It reads the option RequireAllFields beside the
// common ones. The subfields are changed as described for MultiValueField.
func NewMultiValueField(fields []Field, compress func(values []any) (any, error), opts ...FieldOption) *MultiValueField {
	f := &MultiValueField{}
	f.initMulti(f, fields, compress, TextInput{}, nil, collectOptions(opts))
	return f
}

// initMulti prepares a MultiValueField embedded in self: the subfields get the
// incomplete message unless they have their own, the disabled flag of the
// field, and become optional when every subfield is required.
func (f *MultiValueField) initMulti(self Field, fields []Field, compress func([]any) (any, error), widget Widget,
	messages map[string]string, o *fieldOptions,
) {
	f.Fields, f.compress = fields, compress
	f.RequireAllFields = o.requireAll == nil || *o.requireAll
	all := map[string]string{"invalid": "Enter a list of values.", "incomplete": "Enter a complete value."}
	maps.Copy(all, messages)
	f.init(self, widget, all, o)
	for _, sub := range fields {
		base := sub.Base()
		if _, ok := base.ErrorMessages["incomplete"]; !ok {
			base.ErrorMessages["incomplete"] = f.ErrorMessages["incomplete"]
		}
		if f.Disabled {
			base.Disabled = true
		}
		if f.RequireAllFields {
			base.Required = false
		}
	}
}

// multiValue returns the field, for the BoundField that renders the required
// attribute of each subwidget.
func (f *MultiValueField) multiValue() *MultiValueField { return f }

// Validate implements Field; the subfields validate the values.
func (f *MultiValueField) Validate(any) error { return nil }

// Clean cleans each value of the list with its subfield, compresses the
// cleaned values and runs the field's validators over the result. A disabled
// field gets the initial value, which the widget splits unless it is a list.
func (f *MultiValueField) Clean(value any) (any, error) {
	if _, isList := asList(value); f.Disabled && !isList {
		value = subValues(f.Widget, value)
	}
	list, isList := asList(value)
	switch {
	case !isList && !isEmpty(value):
		return nil, f.Error("invalid")
	case allEmpty(list):
		if f.Required {
			return nil, f.Error("required")
		}
		return f.compressValues([]any{})
	}
	var cleaned []any
	var errs []*ValidationError
	for i, field := range f.Fields {
		var v any
		if i < len(list) {
			v = list[i]
		}
		if isEmpty(v) {
			if f.RequireAllFields {
				// A required field rejects any subfield without a value.
				if f.Required {
					return nil, f.Error("required")
				}
			} else if field.Base().Required {
				// A required subfield without a value adds its incomplete
				// message and is not cleaned.
				errs = appendNew(errs, NewValidationError(field.Base().ErrorMessages["incomplete"], ""))
				continue
			}
		}
		c, err := field.Clean(v)
		if err != nil {
			errs = appendNew(errs, toValidationError(err).ErrorList()...)
			continue
		}
		cleaned = append(cleaned, c)
	}
	if len(errs) > 0 {
		return nil, &ValidationError{Errors: errs}
	}
	out, err := f.compressValues(cleaned)
	if err != nil {
		return nil, err
	}
	if err := f.self.Validate(out); err != nil {
		return nil, err
	}
	if err := f.self.RunValidators(out); err != nil {
		return nil, err
	}
	return out, nil
}

// compressValues compresses the cleaned values; without a compress function
// the list is the value.
func (f *MultiValueField) compressValues(values []any) (any, error) {
	if f.compress == nil {
		return values, nil
	}
	return f.compress(values)
}

// HasChanged reports whether the value of any subfield changed. An initial
// value that is not a list is split by the widget; one that a subfield cannot
// convert counts as changed.
func (f *MultiValueField) HasChanged(initial, data any) bool {
	if f.Disabled {
		return false
	}
	dataList, _ := asList(data)
	var initialList []any
	if initial == nil {
		initialList = make([]any, len(dataList))
		for i := range initialList {
			initialList[i] = ""
		}
	} else if list, ok := asList(initial); ok {
		initialList = list
	} else {
		initialList = subValues(f.Widget, initial)
	}
	for i, field := range f.Fields {
		if i >= len(initialList) || i >= len(dataList) {
			break
		}
		init, err := field.ToGo(initialList[i])
		if err != nil {
			return true
		}
		if field.HasChanged(init, dataList[i]) {
			return true
		}
	}
	return false
}

// subValues splits value into the values of the subwidgets of w, nil when w
// is not a MultiValueWidget.
func subValues(w Widget, value any) []any {
	if mw, ok := w.(MultiValueWidget); ok {
		return mw.SubValues(value)
	}
	return nil
}

// allEmpty reports whether no value of list is set.
func allEmpty(list []any) bool {
	for _, v := range list {
		if !isEmpty(v) {
			return false
		}
	}
	return true
}

// appendNew appends each error of add that errs does not hold yet.
func appendNew(errs []*ValidationError, add ...*ValidationError) []*ValidationError {
	for _, e := range add {
		dup := false
		for _, have := range errs {
			if have.Code == e.Code && have.Format() == e.Format() {
				dup = true
				break
			}
		}
		if !dup {
			errs = append(errs, e)
		}
	}
	return errs
}

// SplitDateTimeField is a date and time entered as a date and a time of day,
// Django's SplitDateTimeField. Its widget is a SplitDateTimeWidget; its
// cleaned value is a time.Time in UTC, or nil when empty.
type SplitDateTimeField struct {
	MultiValueField
}

// NewSplitDateTimeField returns a SplitDateTimeField. It reads the options
// InputDateFormats, InputTimeFormats and RequireAllFields beside the common
// ones; the error messages invalid_date and invalid_time become the invalid
// messages of the date and the time.
func NewSplitDateTimeField(opts ...FieldOption) *SplitDateTimeField {
	o := collectOptions(opts)
	messages := map[string]string{"invalid_date": "Enter a valid date.", "invalid_time": "Enter a valid time."}
	maps.Copy(messages, o.errorMessages)
	fields := []Field{
		NewDateField(InputFormats(o.dateFormats...), ErrorMessages(map[string]string{"invalid": messages["invalid_date"]})),
		NewTimeField(InputFormats(o.timeFormats...), ErrorMessages(map[string]string{"invalid": messages["invalid_time"]})),
	}
	f := &SplitDateTimeField{}
	f.initMulti(f, fields, f.compressDateTime, SplitDateTimeWidget{}, messages, o)
	return f
}

// compressDateTime combines the cleaned date and time. Either one missing is
// an error, which an optional field allows.
func (f *SplitDateTimeField) compressDateTime(values []any) (any, error) {
	if len(values) == 0 {
		return nil, nil
	}
	d, ok := values[0].(time.Time)
	if !ok {
		return nil, f.Error("invalid_date")
	}
	var tm time.Time
	if len(values) > 1 {
		tm, ok = values[1].(time.Time)
	}
	if !ok || len(values) < 2 {
		return nil, f.Error("invalid_time")
	}
	return time.Date(d.Year(), d.Month(), d.Day(), tm.Hour(), tm.Minute(), tm.Second(), tm.Nanosecond(), time.UTC), nil
}
