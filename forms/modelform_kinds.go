package forms

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/netip"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/oliverhaas/djangogo/orm"
)

// Reflected target types used when converting cleaned form values into model
// struct fields and when reading initial values back out of model instances.
var (
	ormDecimalType     = reflect.TypeOf(orm.Decimal{})
	ormDateType        = reflect.TypeOf(orm.Date{})
	ormTimeType        = reflect.TypeOf(orm.Time{})
	ormUUIDType        = reflect.TypeOf(orm.UUID{})
	goTimeType         = reflect.TypeOf(time.Time{})
	netipAddrType      = reflect.TypeOf(netip.Addr{})
	jsonRawMessageType = reflect.TypeOf(json.RawMessage(nil))
	byteSliceType      = reflect.TypeOf([]byte(nil))
)

// formFieldFor returns the form field of a model field, Django's
// Field.formfield, or nil for a kind that has none (such as KindAuto). extra
// are the keyword arguments of formfield: they apply after the options that
// mf implies, so they win.
func formFieldFor(db *orm.DB, mf *orm.Field, factory fieldFactory, class fieldClass, extra []FieldOption) Field {
	opts := []FieldOption{Required(!mf.Blank), Label(fieldLabel(mf)), HelpText(mf.HelpText)}
	if mf.HasDefault {
		if mf.Default == orm.DefaultUUID4 {
			opts = append(opts, Initial(func() any { return orm.NewUUID().String() }))
		} else {
			opts = append(opts, Initial(mf.Default))
		}
	}
	if mf.Rel != nil {
		opts = append(opts, extra...)
		var field Field
		if factory != nil {
			field = factory(db, mf, opts)
		}
		if field == nil && isM2M(mf) {
			field = newRelatedMultipleChoiceField(db, mf.Rel, collectOptions(opts))
		}
		if field == nil {
			field = newRelatedChoiceField(db, mf.Rel, collectOptions(opts))
		}
		if choice, ok := field.(*ModelChoiceField); ok {
			choice.Blank = mf.Blank
		}
		return field
	}
	if mf.HasChoices {
		includeBlank := mf.Blank || !mf.HasDefault
		if mf.Kind == orm.KindBool {
			includeBlank = !mf.HasDefault
		}
		choices := ormChoices(mf.GetChoices(includeBlank))
		opts = append(opts, ChoiceList(choices), Coerce(coerceForField(mf)))
		if mf.Null {
			opts = append(opts, EmptyValue(nil))
		}
		return NewTypedChoiceField(append(opts, extra...)...)
	}
	opts, def := kindField(mf, opts)
	opts = append(opts, extra...)
	switch {
	case def == nil:
		return nil
	case class != nil:
		return class(opts...)
	default:
		return def(opts...)
	}
}

// kindField maps each non-relation, non-choice orm.Kind to the constructor of
// its Django form field and returns the options with the length, digit,
// protocol and integer range of mf added. The constructor is nil for a kind
// without a form field.
func kindField(mf *orm.Field, opts []FieldOption) ([]FieldOption, fieldClass) {
	switch mf.Kind {
	case orm.KindChar:
		if mf.MaxLength > 0 {
			opts = append(opts, MaxLength(mf.MaxLength))
		}
		if mf.Null {
			opts = append(opts, EmptyValue(nil))
		}
		return opts, Class(NewCharField)
	case orm.KindText:
		opts = append(opts, UseWidget(Textarea{}))
		if mf.MaxLength > 0 {
			opts = append(opts, MaxLength(mf.MaxLength))
		}
		return opts, Class(NewCharField)
	case orm.KindSlug:
		if mf.MaxLength > 0 {
			opts = append(opts, MaxLength(mf.MaxLength))
		}
		if mf.AllowUnicode {
			opts = append(opts, AllowUnicode())
		}
		if mf.Null {
			opts = append(opts, EmptyValue(nil))
		}
		return opts, Class(NewSlugField)
	case orm.KindEmail:
		if mf.MaxLength > 0 {
			opts = append(opts, MaxLength(mf.MaxLength))
		}
		if mf.Null {
			opts = append(opts, EmptyValue(nil))
		}
		return opts, Class(NewEmailField)
	case orm.KindURL:
		if mf.MaxLength > 0 {
			opts = append(opts, MaxLength(mf.MaxLength))
		}
		if mf.Null {
			opts = append(opts, EmptyValue(nil))
		}
		return opts, Class(NewURLField)
	case orm.KindGenericIPAddress:
		if mf.Protocol != "" {
			opts = append(opts, Protocol(mf.Protocol))
		}
		if mf.UnpackIPv4 {
			opts = append(opts, UnpackIPv4())
		}
		if mf.Null {
			opts = append(opts, EmptyValue(nil))
		}
		return opts, Class(NewGenericIPAddressField)
	case orm.KindInt, orm.KindSmallInt:
		if minVal, maxVal, ok := mf.IntRange(); ok {
			// Match Django's IntegerField/SmallIntegerField widget (no min/max
			// HTML attributes) while still enforcing the storage range before
			// PopulateStruct writes to int16/int32.
			opts = append(opts, Validators(MinValueValidator(minVal), MaxValueValidator(maxVal)))
		}
		return opts, Class(NewIntegerField)
	case orm.KindBigInt:
		if minVal, maxVal, ok := mf.IntRange(); ok {
			opts = append(opts, MinValue(minVal), MaxValue(maxVal))
		}
		return opts, Class(NewIntegerField)
	case orm.KindPositiveSmallInt, orm.KindPositiveInt:
		if _, maxVal, ok := mf.IntRange(); ok {
			opts = append(opts, MinValue(int64(0)), Validators(MaxValueValidator(maxVal)))
		}
		return opts, Class(NewIntegerField)
	case orm.KindPositiveBigInt:
		if _, maxVal, ok := mf.IntRange(); ok {
			opts = append(opts, MinValue(int64(0)), MaxValue(maxVal))
		}
		return opts, Class(NewIntegerField)
	case orm.KindFloat:
		return opts, Class(NewFloatField)
	case orm.KindDecimal:
		if mf.MaxDigits > 0 {
			opts = append(opts, MaxDigits(mf.MaxDigits), DecimalPlaces(mf.DecimalPlaces))
		} else if mf.DecimalPlaces > 0 {
			opts = append(opts, DecimalPlaces(mf.DecimalPlaces))
		}
		return opts, Class(NewDecimalField)
	case orm.KindBool:
		// An unchecked box is a valid "false", so the field is never required.
		if mf.Null {
			return append(opts, Required(false)), Class(NewNullBooleanField)
		}
		return append(opts, Required(false)), Class(NewBooleanField)
	case orm.KindDateTime:
		return opts, Class(NewDateTimeField)
	case orm.KindDate:
		return opts, Class(NewDateField)
	case orm.KindTime:
		return opts, Class(NewTimeField)
	case orm.KindDuration:
		return opts, Class(NewDurationField)
	case orm.KindUUID:
		return opts, Class(NewUUIDField)
	case orm.KindJSON:
		return opts, Class(NewJSONField)
	case orm.KindBinary:
		if !mf.Editable {
			return nil, nil
		}
		if mf.MaxLength > 0 {
			opts = append(opts, MaxLength(mf.MaxLength))
		}
		return opts, Class(NewCharField)
	default:
		return nil, nil
	}
}

// Class returns the constructor of a form field class as a constructor of a
// Field, for an option that takes any class, such as the FormClass of an
// admin FormfieldOverride: Class(NewEmailField).
func Class[F Field](fn func(opts ...FieldOption) F) func(opts ...FieldOption) Field {
	return func(opts ...FieldOption) Field { return fn(opts...) }
}

// ormChoices converts the choices of a model field to form choices, with
// their option groups.
func ormChoices(in []orm.Choice) []Choice {
	if len(in) == 0 {
		return nil
	}
	out := make([]Choice, len(in))
	for i, c := range in {
		out[i] = Choice{Value: c.Value, Label: c.Label, Group: ormChoices(c.Group)}
	}
	return out
}

// coerceForField returns the TypedChoiceField coercion function for mf,
// matching Django's Field.to_python for choices.
func coerceForField(mf *orm.Field) func(string) (any, error) {
	switch mf.Kind {
	case orm.KindInt, orm.KindSmallInt, orm.KindBigInt,
		orm.KindPositiveSmallInt, orm.KindPositiveInt, orm.KindPositiveBigInt:
		return func(s string) (any, error) {
			return strconv.ParseInt(s, 10, 64)
		}
	case orm.KindFloat:
		return func(s string) (any, error) {
			return strconv.ParseFloat(s, 64)
		}
	case orm.KindDecimal:
		return func(s string) (any, error) {
			return ParseDecimal(s)
		}
	case orm.KindBool:
		return func(s string) (any, error) {
			switch strings.ToLower(s) {
			case "true", "1":
				return true, nil
			case "false", "0":
				return false, nil
			default:
				return nil, fmt.Errorf("forms: invalid boolean choice %q", s)
			}
		}
	case orm.KindDate:
		return func(s string) (any, error) {
			d, err := orm.ParseDate(s)
			if err != nil {
				return nil, err
			}
			return d.Time(), nil
		}
	case orm.KindTime:
		return func(s string) (any, error) {
			return orm.ParseTime(s)
		}
	case orm.KindDuration:
		return func(s string) (any, error) {
			d, ok, overflow := parseDuration(s)
			if !ok || overflow {
				return nil, fmt.Errorf("forms: invalid duration choice %q", s)
			}
			return d, nil
		}
	case orm.KindUUID:
		return func(s string) (any, error) {
			u, err := orm.ParseUUID(s)
			if err != nil {
				return nil, err
			}
			return u.String(), nil
		}
	default:
		return nil
	}
}

// fieldInitial returns the value of a model field of an instance as a form
// shows it: the primary key for a relation, nil for nil pointers or zero dates
// and times, and the form-side representation for ORM value types.
func fieldInitial(mf *orm.Field, fv reflect.Value) any {
	if mf.Rel != nil {
		pk := relationPK(fv)
		if pk == 0 {
			return nil
		}
		return pk
	}
	if fv.Kind() == reflect.Pointer {
		if fv.IsNil() {
			return nil
		}
		fv = fv.Elem()
	}
	v := fv.Interface()
	switch x := v.(type) {
	case time.Time:
		if x.IsZero() {
			return nil
		}
		return x
	case orm.Date:
		if x.IsZero() {
			return nil
		}
		return x.Time()
	case orm.Time:
		return time.Date(0, 1, 1, x.Hour, x.Minute, x.Second, x.Nanosecond, time.UTC)
	case orm.Decimal:
		if d, err := ParseDecimal(x.String()); err == nil {
			return d
		}
		return x
	case orm.UUID:
		return x.String()
	case netip.Addr:
		if !x.IsValid() {
			return nil
		}
		return x.String()
	case json.RawMessage:
		if len(x) == 0 {
			return nil
		}
		var decoded any
		if err := json.Unmarshal(x, &decoded); err == nil {
			return decoded
		}
		return string(x)
	case []byte:
		if mf.Kind == orm.KindBinary {
			return string(x)
		}
	}
	return v
}

// setScalar assigns a cleaned value to a struct field, converting between the
// form-side types (forms.Decimal, time.Time, canonical strings, decoded JSON)
// and the model field's Go type (orm.Decimal, orm.Date, orm.Time, orm.UUID,
// netip.Addr, json.RawMessage, arbitrary JSON structs, pointers, and
// sql.Scanner types).
func setScalar(mf *orm.Field, fv reflect.Value, val any) error {
	if val == nil {
		fv.SetZero()
		return nil
	}
	if fv.Kind() == reflect.Pointer {
		target := reflect.New(fv.Type().Elem())
		if err := setScalar(mf, target.Elem(), val); err != nil {
			return err
		}
		fv.Set(target)
		return nil
	}
	rv := reflect.ValueOf(val)
	if rv.Type().AssignableTo(fv.Type()) {
		fv.Set(rv)
		return nil
	}

	// Kind-specific conversions for ORM value types and JSON targets.
	if handled, err := setKindScalar(mf, fv, val); handled || err != nil {
		return err
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
		switch s := val.(type) {
		case string:
			fv.SetString(s)
			return nil
		case JSONString:
			fv.SetString(string(s))
			return nil
		case orm.UUID:
			fv.SetString(s.String())
			return nil
		case netip.Addr:
			if s.IsValid() {
				fv.SetString(s.String())
			} else {
				fv.SetString("")
			}
			return nil
		case Decimal:
			fv.SetString(s.String())
			return nil
		case orm.Decimal:
			fv.SetString(s.String())
			return nil
		}
	}
	if scanner, ok := fv.Addr().Interface().(sql.Scanner); ok {
		if err := scanner.Scan(val); err != nil {
			return fmt.Errorf("forms: field %s: %w", mf.Attr, err)
		}
		return nil
	}
	// A text field takes the string form of any other value, as the
	// to_python of Django's CharField does with str(value).
	if fv.Kind() == reflect.String {
		fv.SetString(pyString(val))
		return nil
	}
	return fmt.Errorf("forms: field %s: cannot assign %T to %s", mf.Attr, val, fv.Type())
}

// setKindScalar handles conversions into ORM value types (orm.Decimal,
// orm.Date, orm.Time, orm.UUID, netip.Addr, time.Time, json.RawMessage,
// arbitrary KindJSON targets, and []byte).
func setKindScalar(mf *orm.Field, fv reflect.Value, val any) (bool, error) {
	if mf != nil && mf.Kind == orm.KindJSON {
		return true, setJSONScalar(mf, fv, val)
	}
	switch fv.Type() {
	case ormDecimalType:
		d, err := toORMDecimal(val)
		if err != nil {
			return true, fmt.Errorf("forms: field %s: %w", mf.Attr, err)
		}
		fv.Set(reflect.ValueOf(d))
		return true, nil
	case ormDateType:
		d, err := toORMDate(val)
		if err != nil {
			return true, fmt.Errorf("forms: field %s: %w", mf.Attr, err)
		}
		fv.Set(reflect.ValueOf(d))
		return true, nil
	case ormTimeType:
		t, err := toORMTime(val)
		if err != nil {
			return true, fmt.Errorf("forms: field %s: %w", mf.Attr, err)
		}
		fv.Set(reflect.ValueOf(t))
		return true, nil
	case goTimeType:
		switch x := val.(type) {
		case orm.Date:
			fv.Set(reflect.ValueOf(x.Time()))
			return true, nil
		case orm.Time:
			fv.Set(reflect.ValueOf(time.Date(0, 1, 1, x.Hour, x.Minute, x.Second, x.Nanosecond, time.UTC)))
			return true, nil
		}
	case ormUUIDType:
		if val == orm.DefaultUUID4 {
			fv.Set(reflect.ValueOf(orm.NewUUID()))
			return true, nil
		}
		if s, ok := val.(string); ok {
			if s == "" {
				fv.SetZero()
				return true, nil
			}
			u, err := orm.ParseUUID(s)
			if err != nil {
				return true, fmt.Errorf("forms: field %s: %w", mf.Attr, err)
			}
			fv.Set(reflect.ValueOf(u))
			return true, nil
		}
	case netipAddrType:
		if s, ok := val.(string); ok {
			if strings.TrimSpace(s) == "" {
				fv.SetZero()
				return true, nil
			}
			addr, err := netip.ParseAddr(strings.TrimSpace(s))
			if err != nil {
				return true, fmt.Errorf("forms: field %s: %w", mf.Attr, err)
			}
			fv.Set(reflect.ValueOf(addr))
			return true, nil
		}
	case byteSliceType:
		if s, ok := val.(string); ok {
			fv.SetBytes([]byte(s))
			return true, nil
		}
	}
	if val == orm.DefaultUUID4 && fv.Kind() == reflect.String {
		fv.SetString(orm.NewUUID().String())
		return true, nil
	}
	return false, nil
}

// setJSONScalar writes a cleaned JSONField value (or json.RawMessage default)
// into a json.RawMessage or arbitrary Go struct/map/slice target.
func setJSONScalar(mf *orm.Field, fv reflect.Value, val any) error {
	var raw []byte
	switch x := val.(type) {
	case json.RawMessage:
		raw = make([]byte, len(x))
		copy(raw, x)
	case JSONString:
		b, err := json.Marshal(string(x))
		if err != nil {
			return fmt.Errorf("forms: field %s: %w", mf.Attr, err)
		}
		raw = b
	default:
		b, err := json.Marshal(val)
		if err != nil {
			return fmt.Errorf("forms: field %s: %w", mf.Attr, err)
		}
		raw = b
	}
	if fv.Type() == jsonRawMessageType {
		fv.Set(reflect.ValueOf(json.RawMessage(raw)))
		return nil
	}
	if err := json.Unmarshal(raw, fv.Addr().Interface()); err != nil {
		return fmt.Errorf("forms: field %s: %w", mf.Attr, err)
	}
	return nil
}

// toORMDecimal converts a form-side decimal or numeric value to orm.Decimal.
func toORMDecimal(val any) (orm.Decimal, error) {
	switch x := val.(type) {
	case orm.Decimal:
		return x, nil
	case Decimal:
		return orm.ParseDecimal(x.String())
	case string:
		return orm.ParseDecimal(x)
	case float64:
		return orm.DecimalFromFloat(x)
	case float32:
		return orm.DecimalFromFloat(float64(x))
	}
	if n, ok := toInt(val); ok {
		return orm.DecimalFromInt(n), nil
	}
	return orm.Decimal{}, fmt.Errorf("cannot convert %T to orm.Decimal", val)
}

// toORMDate converts a form-side DateField value to orm.Date.
func toORMDate(val any) (orm.Date, error) {
	switch x := val.(type) {
	case orm.Date:
		return x, nil
	case time.Time:
		return orm.DateFromTime(x), nil
	case string:
		if strings.TrimSpace(x) == "" {
			return orm.Date{}, nil
		}
		return orm.ParseDate(x)
	default:
		return orm.Date{}, fmt.Errorf("cannot convert %T to orm.Date", val)
	}
}

// toORMTime converts a form-side TimeField value to orm.Time.
func toORMTime(val any) (orm.Time, error) {
	switch x := val.(type) {
	case orm.Time:
		return x, nil
	case time.Time:
		return orm.TimeFromTime(x), nil
	case string:
		return orm.ParseTime(x)
	default:
		return orm.Time{}, fmt.Errorf("cannot convert %T to orm.Time", val)
	}
}
