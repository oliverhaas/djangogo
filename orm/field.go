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
	// Rel describes the relation when this field is a relation field (e.g. FK[T]);
	// it is nil for scalar fields.
	Rel *Relation
	// HasDefault reports whether a default= tag was given. It distinguishes a
	// zero-valued default (e.g. default=0) from "no default at all".
	HasDefault bool
	// Default is the parsed default value, typed by Kind: int64 for KindInt, bool
	// for KindBool, string for KindChar/KindText. It is nil when HasDefault is false.
	Default any
	// AutoNowAdd stamps a KindDateTime field with the current time on Create only.
	AutoNowAdd bool
	// AutoNow stamps a KindDateTime field with the current time on every Create
	// and Update.
	AutoNow bool
	// Choices restricts a KindChar field to a fixed (value, label) set; it is nil
	// when the field is unconstrained.
	Choices []Choice
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

	// Relation fields (e.g. FK[T]) are detected before the scalar type switch.
	// They map to an integer FK column and carry a *Relation; the column defaults
	// to "<snake>_id" and may be overridden by a column= tag.
	if rm, ok := reflect.Zero(sf.Type).Interface().(relationMarker); ok {
		f.Kind = KindInt
		f.Column = toSnakeCase(sf.Name) + "_id"
		f.Rel = &Relation{Kind: rm.relKind(), targetType: rm.relTarget()}
		if tag != "" {
			for _, token := range strings.Split(tag, ";") {
				token = strings.TrimSpace(token)
				if token == "" {
					continue
				}
				if err := applyRelationTagOption(f, sf.Name, token); err != nil {
					return nil, false, err
				}
			}
		}
		// ON DELETE SET NULL requires a nullable column, as Django enforces.
		if f.Rel.OnDelete == OnDeleteSetNull && !f.Null {
			return nil, false, fmt.Errorf("orm: field %s: on_delete=set_null requires null", sf.Name)
		}
		f.Rel.Column = f.Column
		return f, true, nil
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

	// auto_now and auto_now_add are mutually exclusive (Django raises the same).
	if f.AutoNow && f.AutoNowAdd {
		return nil, false, fmt.Errorf("orm: field %s: auto_now and auto_now_add are mutually exclusive", sf.Name)
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
	case token == "auto_now_add":
		if f.Kind != KindDateTime {
			return fmt.Errorf("orm: field %s: auto_now_add is only valid for time.Time fields", fieldName)
		}
		f.AutoNowAdd = true
	case token == "auto_now":
		if f.Kind != KindDateTime {
			return fmt.Errorf("orm: field %s: auto_now is only valid for time.Time fields", fieldName)
		}
		f.AutoNow = true
	case strings.HasPrefix(token, "default="):
		val, err := parseDefault(f.Kind, token[len("default="):])
		if err != nil {
			return fmt.Errorf("orm: field %s: %w", fieldName, err)
		}
		f.HasDefault = true
		f.Default = val
	default:
		return fmt.Errorf("orm: field %s: unknown tag option %q", fieldName, token)
	}
	return nil
}

// parseDefault parses a default= tag value into a Go value typed by the field's
// Kind: int64 for KindInt, bool for KindBool, and the raw string verbatim for
// KindChar/KindText (an empty string is a legal default). It rejects default= on
// datetime fields, whose Django defaults are callables a tag cannot express.
func parseDefault(kind Kind, raw string) (any, error) {
	switch kind {
	case KindInt:
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid default %q: must be an integer", raw)
		}
		return n, nil
	case KindBool:
		switch raw {
		case "true":
			return true, nil
		case "false":
			return false, nil
		default:
			return nil, fmt.Errorf("invalid default %q: must be true or false", raw)
		}
	case KindChar, KindText:
		return raw, nil
	case KindDateTime:
		return nil, fmt.Errorf("default= is not supported for datetime fields; use auto_now_add")
	default:
		return nil, fmt.Errorf("default= is not supported for this field type")
	}
}

// applyRelationTagOption applies a single parsed tag token to a relation field.
// A relation field accepts null, unique, and column= but rejects scalar-only
// options such as max_length, type=text, and pk.
func applyRelationTagOption(f *Field, fieldName, token string) error {
	switch {
	case token == "null":
		f.Null = true
	case token == "unique":
		f.Unique = true
	case strings.HasPrefix(token, "column="):
		name := token[len("column="):]
		if name == "" {
			return fmt.Errorf("orm: field %s: column name must not be empty", fieldName)
		}
		f.Column = name
	case strings.HasPrefix(token, "on_delete="):
		od, err := ParseOnDelete(token[len("on_delete="):])
		if err != nil {
			return fmt.Errorf("orm: field %s: %w", fieldName, err)
		}
		f.Rel.OnDelete = od
	case token == "pk":
		return fmt.Errorf("orm: field %s: pk is not valid on a relation field", fieldName)
	case token == "type=text" || strings.HasPrefix(token, "max_length="),
		token == "auto_now", token == "auto_now_add", strings.HasPrefix(token, "default="):
		return fmt.Errorf("orm: field %s: %q is not valid on a relation field", fieldName, token)
	default:
		return fmt.Errorf("orm: field %s: unknown tag option %q", fieldName, token)
	}
	return nil
}
