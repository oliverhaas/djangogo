package orm

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// Kind identifies the logical type of a model field.
type Kind uint8

const (
	// KindAuto is an integer primary key with autoincrement, assigned by the registry.
	KindAuto Kind = iota
	// KindInt maps to integer Go types.
	KindInt
	// KindChar maps to a string stored as VARCHAR(MaxLength).
	KindChar
	// KindText maps to a string stored as TEXT.
	KindText
	// KindBool maps to a bool.
	KindBool
	// KindDateTime maps to time.Time.
	KindDateTime
)

// String returns a human-readable name for the Kind.
func (k Kind) String() string {
	switch k {
	case KindAuto:
		return "Auto"
	case KindInt:
		return "Int"
	case KindChar:
		return "Char"
	case KindText:
		return "Text"
	case KindBool:
		return "Bool"
	case KindDateTime:
		return "DateTime"
	default:
		return fmt.Sprintf("Kind(%d)", k)
	}
}

// Field describes one column of a model as derived from a Go struct field and its orm tag.
type Field struct {
	// Name is the Go struct field name, e.g. "Title".
	Name string
	// Column is the database column name, e.g. "title".
	Column string
	// Kind is the logical type of the field.
	Kind Kind
	// PrimaryKey indicates whether this field is the primary key.
	PrimaryKey bool
	// Null indicates whether the column allows NULL.
	Null bool
	// Unique indicates whether the column has a UNIQUE constraint.
	Unique bool
	// MaxLength is the maximum character length for KindChar fields; default 255.
	MaxLength int
	// Index is the index of this field within the parent struct (for reflect).
	Index int
}

// timeType is the reflect.Type for time.Time, used for KindDateTime inference.
var timeType = reflect.TypeOf(time.Time{})

// parseStructField derives a *Field from a reflected struct field and its orm tag.
// It returns (field, true, nil) for included fields, (nil, false, nil) for skipped
// fields, and (nil, false, err) on a tag or type error.
func parseStructField(sf reflect.StructField, index int) (*Field, bool, error) {
	// Step 1: skip unexported fields and fields tagged orm:"-".
	if sf.PkgPath != "" {
		return nil, false, nil
	}
	tag := sf.Tag.Get("orm")
	if tag == "-" {
		return nil, false, nil
	}

	// Step 2: infer Kind from the Go type.
	f := &Field{
		Name:   sf.Name,
		Column: toSnakeCase(sf.Name),
		Index:  index,
	}

	switch sf.Type.Kind() { // unsupported kinds handled by default
	case reflect.String:
		f.Kind = KindChar
		f.MaxLength = 255
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		f.Kind = KindInt
	case reflect.Bool:
		f.Kind = KindBool
	case reflect.Struct:
		if sf.Type == timeType {
			f.Kind = KindDateTime
		} else {
			return nil, false, fmt.Errorf("orm: field %s: unsupported type %s", sf.Name, sf.Type)
		}
	default:
		return nil, false, fmt.Errorf("orm: field %s: unsupported type %s", sf.Name, sf.Type)
	}

	// Step 3: apply orm tag options.
	if tag != "" {
		for _, token := range strings.Split(tag, ";") {
			token = strings.TrimSpace(token)
			if token == "" {
				continue
			}
			if err := applyTagOption(f, sf.Name, token); err != nil {
				return nil, false, err
			}
		}
	}

	return f, true, nil
}

// toSnakeCase converts a Go identifier (PascalCase/camelCase, possibly with acronym
// runs) into snake_case for use as a default column name.
func toSnakeCase(s string) string {
	runes := []rune(s)
	var b strings.Builder
	for i, r := range runes {
		if unicode.IsUpper(r) {
			if i > 0 && (unicode.IsLower(runes[i-1]) || unicode.IsDigit(runes[i-1]) ||
				(i+1 < len(runes) && unicode.IsLower(runes[i+1]))) {
				b.WriteByte('_')
			}
			b.WriteRune(unicode.ToLower(r))
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// applyTagOption applies a single parsed tag token to the field.
func applyTagOption(f *Field, fieldName, token string) error {
	switch {
	case token == "pk":
		f.PrimaryKey = true
	case token == "null":
		f.Null = true
	case token == "unique":
		f.Unique = true
	case token == "type=text":
		if f.Kind != KindChar {
			return fmt.Errorf("orm: field %s: type=text is only valid for string fields", fieldName)
		}
		f.Kind = KindText
		f.MaxLength = 0
	case strings.HasPrefix(token, "column="):
		name := token[len("column="):]
		if name == "" {
			return fmt.Errorf("orm: field %s: column name must not be empty", fieldName)
		}
		f.Column = name
	case strings.HasPrefix(token, "max_length="):
		raw := token[len("max_length="):]
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 {
			return fmt.Errorf("orm: field %s: invalid max_length %q: must be a positive integer", fieldName, raw)
		}
		f.MaxLength = n
	default:
		return fmt.Errorf("orm: field %s: unknown tag option %q", fieldName, token)
	}
	return nil
}
