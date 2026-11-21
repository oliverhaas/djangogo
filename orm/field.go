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
	// KindSmallInt maps to 16-bit signed integers (SMALLINT).
	KindSmallInt
	// KindBigInt maps to 64-bit signed integers (BIGINT).
	KindBigInt
	// KindPositiveSmallInt maps to non-negative 16-bit integers with CHECK >= 0.
	KindPositiveSmallInt
	// KindPositiveInt maps to non-negative 32-bit integers with CHECK >= 0.
	KindPositiveInt
	// KindPositiveBigInt maps to non-negative 64-bit integers with CHECK >= 0.
	KindPositiveBigInt
	// KindFloat maps to floating-point numbers (float64/float32).
	KindFloat
	// KindDecimal maps to exact fixed-point decimals (orm.Decimal).
	KindDecimal
	// KindDate maps to calendar dates (orm.Date or time.Time with type=date).
	KindDate
	// KindTime maps to wall-clock times (orm.Time or time.Time with type=time).
	KindTime
	// KindDuration maps to durations (time.Duration).
	KindDuration
	// KindUUID maps to 128-bit UUIDs (orm.UUID or string with type=uuid).
	KindUUID
	// KindJSON maps to JSON documents (json.RawMessage or any type with type=json).
	KindJSON
	// KindBinary maps to raw byte slices ([]byte).
	KindBinary
	// KindSlug maps to slug strings (type=slug, default max_length=50, indexed).
	KindSlug
	// KindEmail maps to email strings (type=email, default max_length=254).
	KindEmail
	// KindURL maps to URL strings (type=url, default max_length=200).
	KindURL
	// KindGenericIPAddress maps to IPv4/IPv6 addresses (netip.Addr or string with type=ip).
	KindGenericIPAddress
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
		return kindString(k)
	}
}

// Field describes one column of a model as derived from a Go struct field and its orm tag.
type Field struct {
	// Name is the Go struct field name, e.g. "Title".
	Name string
	// Attr is the snake_case attribute name of the field, the analog of Django's
	// field name: "created_at" for CreatedAt, and "author" for an Author FK whose
	// Column is "author_id". Unlike Column it is never overridden by a column= tag.
	// It is the key forms and templates use, and one filters accept.
	Attr string
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
	// Blank reports whether the field may be left empty in a form (Django's
	// blank=True). It is a validation-level flag, independent of Null, which is
	// the database-level one.
	Blank bool
	// Editable reports whether the field appears in a ModelForm and the admin
	// (Django's editable). It defaults to true; an editable=false tag and the
	// auto_now/auto_now_add options clear it.
	Editable bool
	// VerboseName is the human-readable field name (Django's verbose_name), or ""
	// to derive one from Attr.
	VerboseName string
	// HelpText is descriptive text shown beside the field's form widget.
	HelpText string
	// DBIndex requests a single-column database index (Django's db_index=True).
	DBIndex bool
	// MaxDigits is the maximum total digit count for KindDecimal (0 means unspecified).
	MaxDigits int
	// DecimalPlaces is the number of fractional digits for KindDecimal.
	DecimalPlaces int
	// AllowUnicode allows Unicode letters in KindSlug fields (Django's allow_unicode).
	AllowUnicode bool
	// Protocol restricts KindGenericIPAddress to "both", "ipv4", or "ipv6".
	Protocol string
	// UnpackIPv4 unpacks IPv4-mapped IPv6 addresses (::ffff:a.b.c.d) to IPv4 on
	// KindGenericIPAddress fields (Django's unpack_ipv4).
	UnpackIPv4 bool
	// HasChoices reports whether choices were explicitly declared on the field
	// (distinguishing an empty choice set from an unconstrained field).
	HasChoices bool
	// ChoicesFunc provides dynamic choices evaluated on demand (Django's
	// callable choices=).
	ChoicesFunc func() []Choice
	// Validators holds custom field-level validators declared via Meta.Validators.
	Validators []Validator
	// ErrorMessages holds per-code custom error message templates declared via
	// Meta.ErrorMessages.
	ErrorMessages map[string]string
}

// timeType is the reflect.Type for time.Time, used for KindDateTime inference.
var timeType = reflect.TypeOf(time.Time{})

// parseStructField derives a *Field from a reflected struct field and its orm tag.
// It returns (field, true, nil) for included fields, (nil, false, nil) for skipped
// fields, and (nil, false, err) on a tag or type error.
func parseStructField(sf reflect.StructField, index int) (*Field, bool, error) {
	// Step 1: skip unexported fields, embedded RelCache, and fields tagged orm:"-".
	if sf.PkgPath != "" || sf.Type == relCacheType {
		return nil, false, nil
	}
	tag := sf.Tag.Get("orm")
	if tag == "-" {
		return nil, false, nil
	}

	// Step 2: infer Kind from the Go type.
	f := &Field{
		Name:     sf.Name,
		Attr:     toSnakeCase(sf.Name),
		Column:   toSnakeCase(sf.Name),
		Index:    index,
		Editable: true,
	}

	// Relation fields (e.g. FK[T], O2O[T]) are detected before the scalar type switch.
	// They map to an integer FK column and carry a *Relation; the column defaults
	// to "<snake>_id" and may be overridden by a column= tag.
	if rm, ok := reflect.Zero(sf.Type).Interface().(relationMarker); ok {
		f.Kind = KindInt
		f.Column = toSnakeCase(sf.Name) + "_id"
		f.Rel = &Relation{Kind: rm.relKind(), targetType: rm.relTarget()}
		if f.Rel.Kind == RelO2O {
			f.Unique = true
		}
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

	if err := inferFieldKind(f, sf.Type, sf.Name, tag); err != nil {
		return nil, false, err
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
	if err := validateFieldOptions(f, sf.Name); err != nil {
		return nil, false, err
	}

	// auto_now and auto_now_add are mutually exclusive (Django raises the same).
	if f.AutoNow && f.AutoNowAdd {
		return nil, false, fmt.Errorf("orm: field %s: auto_now and auto_now_add are mutually exclusive", sf.Name)
	}
	// Django marks auto-stamped fields editable=False (and blank=True): the ORM
	// owns their value, so a form must neither show nor require them.
	if f.AutoNow || f.AutoNowAdd {
		f.Editable = false
		f.Blank = true
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

// applyCommonTagOption applies a tag token that is valid on both scalar and
// relation fields. It reports whether it recognized the token.
func applyCommonTagOption(f *Field, fieldName, token string) (bool, error) {
	switch {
	case token == "null", token == "null=true":
		f.Null = true
	case token == "null=false":
		f.Null = false
	case token == "unique":
		f.Unique = true
	case token == "blank":
		f.Blank = true
	case token == "index":
		f.DBIndex = true
	case token == "editable=false":
		f.Editable = false
	case token == "editable=true":
		f.Editable = true
	case strings.HasPrefix(token, "editable="):
		return true, fmt.Errorf("orm: field %s: invalid %q: editable must be true or false", fieldName, token)
	case strings.HasPrefix(token, "verbose_name="):
		f.VerboseName = token[len("verbose_name="):]
	case strings.HasPrefix(token, "help_text="):
		f.HelpText = token[len("help_text="):]
	case strings.HasPrefix(token, "column="):
		name := token[len("column="):]
		if name == "" {
			return true, fmt.Errorf("orm: field %s: column name must not be empty", fieldName)
		}
		f.Column = name
	default:
		return false, nil
	}
	return true, nil
}

// applyTagOption applies a single parsed tag token to the field.
func applyTagOption(f *Field, fieldName, token string) error {
	if handled, err := applyCommonTagOption(f, fieldName, token); handled || err != nil {
		return err
	}
	if handled, err := applyKindTagOption(f, fieldName, token); handled || err != nil {
		return err
	}
	switch {
	case token == "pk":
		f.PrimaryKey = true
	case strings.HasPrefix(token, "max_length="):
		raw := token[len("max_length="):]
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 {
			return fmt.Errorf("orm: field %s: invalid max_length %q: must be a positive integer", fieldName, raw)
		}
		f.MaxLength = n
	case token == "auto_now_add":
		if !isTemporalAutoKind(f.Kind) {
			return fmt.Errorf("orm: field %s: auto_now_add is only valid for date/time fields", fieldName)
		}
		f.AutoNowAdd = true
	case token == "auto_now":
		if !isTemporalAutoKind(f.Kind) {
			return fmt.Errorf("orm: field %s: auto_now is only valid for date/time fields", fieldName)
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
		return parseKindDefault(kind, raw)
	}
}

// applyRelationTagOption applies a single parsed tag token to a relation field.
// A relation field accepts the common options (null, unique, blank, index,
// editable=, verbose_name=, help_text=, column=) and on_delete=, but rejects
// scalar-only options such as max_length, type=text, and pk.
func applyRelationTagOption(f *Field, fieldName, token string) error {
	if handled, err := applyCommonTagOption(f, fieldName, token); handled || err != nil {
		return err
	}
	switch {
	case strings.HasPrefix(token, "on_delete="):
		od, err := ParseOnDelete(token[len("on_delete="):])
		if err != nil {
			return fmt.Errorf("orm: field %s: %w", fieldName, err)
		}
		f.Rel.OnDelete = od
	case strings.HasPrefix(token, "related_name="):
		rn := token[len("related_name="):]
		if rn == "" {
			return fmt.Errorf("orm: field %s: related_name must not be empty", fieldName)
		}
		f.Rel.RelatedName = rn
	case strings.HasPrefix(token, "related_query_name="):
		rqn := token[len("related_query_name="):]
		if rqn == "" {
			return fmt.Errorf("orm: field %s: related_query_name must not be empty", fieldName)
		}
		f.Rel.RelatedQueryName = rqn
	case token == "pk":
		if f.Rel.Kind != RelO2O {
			return fmt.Errorf("orm: field %s: pk is not valid on a relation field", fieldName)
		}
		f.PrimaryKey = true
	case token == "type=text" || strings.HasPrefix(token, "max_length="),
		token == "auto_now", token == "auto_now_add", strings.HasPrefix(token, "default="):
		return fmt.Errorf("orm: field %s: %q is not valid on a relation field", fieldName, token)
	default:
		return fmt.Errorf("orm: field %s: unknown tag option %q", fieldName, token)
	}
	return nil
}
