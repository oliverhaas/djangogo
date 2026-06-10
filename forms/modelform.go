package forms

import (
	"fmt"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/oliverhaas/djangogo/orm"
)

// FromModel derives a Form from an orm model. The auto primary key is skipped.
// Each model field maps to a form Field whose Kind and default Widget follow the
// orm Kind. A foreign-key column (Rel != nil) maps to a ChoiceField rendered as a
// <select>, named after the model field; its options start empty and are filled
// in with the related rows via Form.SetChoices, mirroring Django's ModelChoiceField.
func FromModel(m *orm.Model) *Form {
	var fields []*Field
	for _, mf := range m.Fields() {
		ff := formFieldFor(mf)
		if ff == nil {
			continue
		}
		fields = append(fields, ff)
	}
	return New(fields...)
}

// formFieldFor builds a form Field for a single orm field, or returns nil to
// skip it (the auto primary key).
func formFieldFor(mf *orm.Field) *Field {
	// Foreign keys map to a <select> before the scalar Kind switch, since an FK
	// column has orm Kind KindInt but a non-nil Rel. Choices start empty; the
	// caller fills them from the related model via Form.SetChoices.
	if mf.Rel != nil {
		return &Field{
			Name:     mf.Name,
			Label:    humanize(mf.Name),
			Required: !mf.Null,
			Kind:     ChoiceField,
			Widget:   Select{},
		}
	}

	switch mf.Kind {
	case orm.KindAuto:
		return nil
	case orm.KindChar:
		// A KindChar field carrying choices renders as a <select> validated
		// against its value set, mirroring Django's choices= ModelForm field.
		if len(mf.Choices) > 0 {
			choices := toFormChoices(mf.Choices)
			return &Field{
				Name:     mf.Name,
				Label:    humanize(mf.Name),
				Required: !mf.Null,
				Kind:     ChoiceField,
				Choices:  choices,
				Widget:   Select{Choices: choices},
			}
		}
		return &Field{
			Name:      mf.Name,
			Label:     humanize(mf.Name),
			Required:  !mf.Null,
			Kind:      CharField,
			MaxLength: mf.MaxLength,
			Widget:    TextInput{},
		}
	case orm.KindText:
		return &Field{
			Name:     mf.Name,
			Label:    humanize(mf.Name),
			Required: !mf.Null,
			Kind:     TextField,
			Widget:   Textarea{},
		}
	case orm.KindInt:
		return &Field{
			Name:     mf.Name,
			Label:    humanize(mf.Name),
			Required: !mf.Null,
			Kind:     IntegerField,
			Widget:   NumberInput{},
		}
	case orm.KindBool:
		return &Field{
			Name:   mf.Name,
			Label:  humanize(mf.Name),
			Kind:   BoolField,
			Widget: CheckboxInput{},
		}
	case orm.KindDateTime:
		return &Field{
			Name:     mf.Name,
			Label:    humanize(mf.Name),
			Required: !mf.Null,
			Kind:     DateTimeField,
			Widget:   TextInput{},
		}
	default:
		return nil
	}
}

// toFormChoices maps orm choices to the forms layer's value/label pairs.
func toFormChoices(choices []orm.Choice) [][2]string {
	out := make([][2]string, len(choices))
	for i, c := range choices {
		out[i] = [2]string{c.Value, c.Label}
	}
	return out
}

// FromStruct derives a Form from a model and pre-fills it with the current field
// values of obj (a pointer to an instance of the model's struct type).
func FromStruct(m *orm.Model, obj any) *Form {
	f := FromModel(m)
	rv := reflect.ValueOf(obj)
	if rv.Kind() == reflect.Pointer {
		rv = rv.Elem()
	}
	data := url.Values{}
	if rv.Kind() == reflect.Struct {
		for _, mf := range m.Fields() {
			ff := formFieldFor(mf)
			if ff == nil {
				continue
			}
			fv := rv.Field(mf.Index)
			if s, ok := stringifyFieldValue(mf, fv); ok {
				data.Set(mf.Name, s)
			}
		}
	}
	return f.Bind(data)
}

// stringifyFieldValue renders a struct field value as the string a widget would
// re-display. It returns false when the value should be omitted.
func stringifyFieldValue(mf *orm.Field, fv reflect.Value) (string, bool) {
	if mf.Rel != nil {
		pk := fv.MethodByName("PK").Call(nil)[0].Int()
		if pk == 0 {
			return "", false
		}
		return strconv.FormatInt(pk, 10), true
	}
	switch fv.Kind() {
	case reflect.String:
		return fv.String(), true
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(fv.Int(), 10), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return strconv.FormatUint(fv.Uint(), 10), true
	case reflect.Bool:
		if fv.Bool() {
			return "true", true
		}
		return "", false
	case reflect.Struct:
		if t, ok := fv.Interface().(time.Time); ok {
			if t.IsZero() {
				return "", false
			}
			return t.Format(time.RFC3339), true
		}
		return "", false
	default:
		return "", false
	}
}

// PopulateStruct sets the exported fields of dest (a pointer to a struct) from
// cleaned data, matching by orm Field.Name. It handles string, int64, bool, and
// time.Time values, and sets a foreign key's primary key via its SetPK method
// from the cleaned value (an int64 from an IntegerField or a string from a
// ChoiceField <select>).
func PopulateStruct(m *orm.Model, cleaned map[string]any, dest any) error {
	rv := reflect.ValueOf(dest)
	if rv.Kind() != reflect.Pointer || rv.IsNil() {
		return fmt.Errorf("forms: PopulateStruct requires a non-nil pointer, got %T", dest)
	}
	elem := rv.Elem()
	if elem.Kind() != reflect.Struct {
		return fmt.Errorf("forms: PopulateStruct requires a pointer to a struct, got %T", dest)
	}

	for _, mf := range m.Fields() {
		val, ok := cleaned[mf.Name]
		if !ok {
			continue
		}
		fv := elem.Field(mf.Index)

		if mf.Rel != nil {
			pk, err := fkPKFromCleaned(val)
			if err != nil {
				return fmt.Errorf("forms: field %s: %w", mf.Name, err)
			}
			fv.Addr().MethodByName("SetPK").Call([]reflect.Value{reflect.ValueOf(pk)})
			continue
		}

		if err := setScalar(mf, fv, val); err != nil {
			return err
		}
	}
	return nil
}

// fkPKFromCleaned coerces a cleaned foreign-key value into the related primary
// key. A ChoiceField <select> yields the chosen option's string value; an
// IntegerField yields an int64. An empty string means "no selection" and maps to
// pk 0 (a cleared, nullable relation).
func fkPKFromCleaned(val any) (int64, error) {
	switch v := val.(type) {
	case int64:
		return v, nil
	case string:
		if strings.TrimSpace(v) == "" {
			return 0, nil
		}
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("FK value %q is not a valid id", v)
		}
		return n, nil
	default:
		return 0, fmt.Errorf("FK value must be a string or int64, got %T", val)
	}
}

// setScalar assigns a cleaned scalar value to a struct field, converting integer
// widths as needed.
func setScalar(mf *orm.Field, fv reflect.Value, val any) error {
	switch v := val.(type) {
	case string:
		fv.SetString(v)
	case int64:
		switch fv.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			fv.SetInt(v)
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			fv.SetUint(uint64(v))
		default:
			return fmt.Errorf("forms: field %s: cannot assign int64 to %s", mf.Name, fv.Kind())
		}
	case bool:
		fv.SetBool(v)
	case time.Time:
		fv.Set(reflect.ValueOf(v))
	default:
		return fmt.Errorf("forms: field %s: unsupported cleaned type %T", mf.Name, val)
	}
	return nil
}

// humanize turns a Go identifier into a readable display label.
// It splits on camelCase/PascalCase word boundaries and on runs of upper-case
// letters (acronyms), then Title-cases the first word and lowercases the rest,
// while preserving all-caps acronyms.
//
// Examples:
//
//	"CreatedAt"  -> "Created at"
//	"FirstName"  -> "First name"
//	"ID"         -> "ID"
//	"URL"        -> "URL"
//	"Title"      -> "Title"
func humanize(name string) string {
	words := splitIdentifier(name)
	if len(words) == 0 {
		return name
	}
	var sb strings.Builder
	for i, w := range words {
		if i > 0 {
			sb.WriteByte(' ')
		}
		switch {
		case isAcronym(w):
			// All-caps short runs (e.g. ID, URL) stay upper-case.
			sb.WriteString(w)
		case i == 0:
			// First word: Title-case.
			sb.WriteString(strings.ToUpper(w[:1]) + strings.ToLower(w[1:]))
		default:
			sb.WriteString(strings.ToLower(w))
		}
	}
	return sb.String()
}

// splitIdentifier breaks a Go identifier into words at camelCase/PascalCase
// transitions and acronym runs. For example:
//
//	"CreatedAt" -> ["Created", "At"]
//	"HTTPSProxy" -> ["HTTPS", "Proxy"]
//	"ID" -> ["ID"]
func splitIdentifier(s string) []string {
	if s == "" {
		return nil
	}
	runes := []rune(s)
	n := len(runes)
	var words []string
	start := 0
	for i := 1; i < n; i++ {
		prev := runes[i-1]
		curr := runes[i]
		var next rune
		if i+1 < n {
			next = runes[i+1]
		}
		cut := false
		if isUpper(curr) {
			if isLower(prev) {
				// "camelCase" boundary: e before C.
				cut = true
			} else if isUpper(prev) && next != 0 && isLower(next) {
				// Acronym-to-word boundary: "HTTPSProxy" splits before P.
				cut = true
			}
		}
		if cut {
			words = append(words, string(runes[start:i]))
			start = i
		}
	}
	words = append(words, string(runes[start:]))
	return words
}

func isUpper(r rune) bool { return r >= 'A' && r <= 'Z' }
func isLower(r rune) bool { return r >= 'a' && r <= 'z' }

// isAcronym returns true when every rune in w is an upper-case ASCII letter.
func isAcronym(w string) bool {
	if w == "" {
		return false
	}
	for _, r := range w {
		if !isUpper(r) {
			return false
		}
	}
	return true
}
