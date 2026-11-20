package orm

import (
	"database/sql"
	"database/sql/driver"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"net/netip"
	"reflect"
	"strconv"
	"strings"
	"time"
)

/*
Field Kinds Design Table

Each model field maps a Go struct field type (plus optional orm:"..." tags) to
a logical Kind. Pointer fields (*T) unwrap one level, infer the same Kind as T,
and imply Null=true so Go nil cleanly represents SQL NULL distinct from a zero
value (such as 0, false, "", 0s, midnight, or the nil UUID).

Go Type Decisions (stdlib only, zero external go.mod dependencies):
  - Decimal: orm.Decimal (struct with sign, decimal digit string, base-10
    exponent). Preserves arbitrary precision and trailing fractional zeros,
    implements driver.Valuer/sql.Scanner and ROUND_HALF_EVEN Quantize.
  - Date and Time: dedicated comparable structs orm.Date and orm.Time (inferred
    without a tag, no timezone/monotonic equality pitfalls). time.Time with
    orm:"type=date" or orm:"type=time" is also supported.
  - Duration: stdlib time.Duration (inferred automatically). Stored as signed
    int64 microseconds in SQLite BIGINT and as native INTERVAL in PostgreSQL.
  - UUID: orm.UUID ([16]byte), comparable value type with crypto/rand v4
    generation and stdlib hex formatting. string with orm:"type=uuid" is also
    supported. Stored as CHAR(32) without hyphens in SQLite and native UUID in
    PostgreSQL.
  - JSON: stdlib json.RawMessage (inferred automatically), or any struct, map,
    slice, or interface tagged orm:"type=json".
  - Binary: []byte (inferred automatically, defaults to Editable=false).
  - GenericIPAddress: stdlib net/netip.Addr (inferred automatically) or string
    tagged orm:"type=ip" / orm:"type=generic_ip".

Kind Table:

  Kind                 | Go type(s)                   | Tag spelling                                   | SQLite column                                     | PostgreSQL column                    | Bind / Scan                                             | Default form field        | Migrations default
  ---------------------+------------------------------+------------------------------------------------+---------------------------------------------------+--------------------------------------+---------------------------------------------------------+---------------------------+-------------------------------
  KindAuto             | int, int32, int64 (ID or pk) | pk (or field named ID)                         | INTEGER PRIMARY KEY AUTOINCREMENT                 | BIGSERIAL PRIMARY KEY                | int64                                                   | (excluded)                | none
  KindInt              | int, int32, int64            | default (or type=int)                          | INTEGER                                           | BIGINT                               | int64                                                   | forms.IntegerField        | int64(n)
  KindSmallInt         | int8, int16 (or signed int)  | inferred / type=smallint                       | SMALLINT                                          | SMALLINT                             | int64 [-32768, 32767]                                   | forms.IntegerField        | int64(n)
  KindBigInt           | int, int64                   | type=bigint                                    | BIGINT                                            | BIGINT                               | int64 [-9223372036854775808, 9223372036854775807]       | forms.IntegerField        | int64(n)
  KindPositiveSmallInt | uint8, uint16 (or int/uint)  | inferred / type=positive_smallint              | SMALLINT UNSIGNED CHECK ("col" >= 0)              | SMALLINT CHECK ("col" >= 0)          | int64 [0, 32767]                                        | forms.IntegerField        | int64(n)
  KindPositiveInt      | uint32 (or int/uint)         | inferred / type=positive_int                   | INTEGER UNSIGNED CHECK ("col" >= 0)               | INTEGER CHECK ("col" >= 0)           | int64 [0, 2147483647]                                   | forms.IntegerField        | int64(n)
  KindPositiveBigInt   | uint, uint64 (or int/uint)   | inferred / type=positive_bigint                | BIGINT UNSIGNED CHECK ("col" >= 0)                | BIGINT CHECK ("col" >= 0)            | int64 [0, 9223372036854775807]                          | forms.IntegerField        | int64(n)
  KindFloat            | float64, float32             | inferred (or type=float)                       | REAL                                              | DOUBLE PRECISION                     | float64                                                 | forms.FloatField          | float64(v)
  KindDecimal          | orm.Decimal                  | inferred / type=decimal; max_digits, decimal_places | DECIMAL                                      | NUMERIC(M, D) or NUMERIC             | exact decimal text (SQLite quantizes on scan, prec=15)  | forms.DecimalField        | orm.MustDecimal("...")
  KindChar             | string                       | default; max_length=N (default 255)            | VARCHAR(N)                                        | VARCHAR(N)                           | string                                                  | forms.CharField           | "..."
  KindText             | string                       | type=text                                      | TEXT                                              | TEXT                                 | string                                                  | forms.CharField(Textarea) | "..."
  KindSlug             | string                       | type=slug; max_length (50), allow_unicode, index=false | VARCHAR(N) (indexed by default)           | VARCHAR(N) (indexed by default)      | string                                                  | forms.SlugField           | "..."
  KindEmail            | string                       | type=email; max_length (254)                   | VARCHAR(N)                                        | VARCHAR(N)                           | string                                                  | forms.EmailField          | "..."
  KindURL              | string                       | type=url; max_length (200)                     | VARCHAR(N)                                        | VARCHAR(N)                           | string                                                  | forms.URLField            | "..."
  KindGenericIPAddress | netip.Addr, string           | inferred / type=ip; protocol=both|ipv4|ipv6    | CHAR(39)                                          | INET                                 | trimmed canonical IP text ("" -> NULL when nullable)    | forms.GenericIPAddressField | "..."
  KindBool             | bool                         | inferred                                       | BOOLEAN                                           | BOOLEAN                              | bool (SQLite 0/1)                                       | forms.BooleanField        | true / false
  KindDateTime         | time.Time                    | inferred; auto_now, auto_now_add               | DATETIME                                          | TIMESTAMPTZ                          | time.Time                                               | forms.DateTimeField       | migrations.Now
  KindDate             | orm.Date, time.Time          | inferred / type=date; auto_now, auto_now_add   | DATE                                              | DATE                                 | "YYYY-MM-DD" / orm.Date (time.Time midnight UTC)        | forms.DateField           | orm.MustDate("...")
  KindTime             | orm.Time, time.Time          | inferred / type=time; auto_now, auto_now_add   | TIME                                              | TIME                                 | "HH:MM:SS[.ffffff]" / orm.Time                          | forms.TimeField           | orm.MustTime("...")
  KindDuration         | time.Duration                | inferred (or type=duration)                    | BIGINT (microseconds)                             | INTERVAL                             | SQLite int64 us; Postgres INTERVAL string               | forms.DurationField       | time.Duration(n)
  KindUUID             | orm.UUID, string             | inferred / type=uuid; default=uuid4            | CHAR(32) (32-char lowercase hex)                  | UUID                                 | SQLite 32-char hex; Postgres canonical hyphenated       | forms.UUIDField           | orm.MustUUID("...") / orm.DefaultUUID4
  KindJSON             | json.RawMessage, any+tag     | inferred / type=json                           | TEXT CHECK (JSON_VALID("col") OR "col" IS NULL)   | JSONB                                | JSON text string / json.RawMessage or JSON unmarshal    | forms.JSONField           | json.RawMessage("...")
  KindBinary           | []byte                       | inferred / type=binary; editable=false default | BLOB                                              | BYTEA                                | []byte (copied on scan)                                 | (excluded unless editable)| []byte{...}
*/

// ValueDialect is an optional capability of a Dialect: it converts Go values
// into the driver representation expected by its backend (such as microseconds
// vs INTERVAL for Duration, or 32-char hex vs hyphenated text for UUID) and
// reports whether the backend stores UUIDs in a native hyphenated column type.
type ValueDialect interface {
	BindValue(f *Field, v any) (any, error)
	HasNativeUUID() bool
}

// Reflected types used during field kind inference and value conversion.
var (
	decimalType        = reflect.TypeOf(Decimal{})
	dateType           = reflect.TypeOf(Date{})
	timeOfDayType      = reflect.TypeOf(Time{})
	durationType       = reflect.TypeOf(time.Duration(0))
	uuidType           = reflect.TypeOf(UUID{})
	jsonRawMessageType = reflect.TypeOf(json.RawMessage(nil))
	byteSliceType      = reflect.TypeOf([]byte(nil))
	netipAddrType      = reflect.TypeOf(netip.Addr{})
)

// kindString returns the name of every Kind added beyond the initial six.
func kindString(k Kind) string {
	switch k {
	case KindSmallInt:
		return "SmallInt"
	case KindBigInt:
		return "BigInt"
	case KindPositiveSmallInt:
		return "PositiveSmallInt"
	case KindPositiveInt:
		return "PositiveInt"
	case KindPositiveBigInt:
		return "PositiveBigInt"
	case KindFloat:
		return "Float"
	case KindDecimal:
		return "Decimal"
	case KindDate:
		return "Date"
	case KindTime:
		return "Time"
	case KindDuration:
		return "Duration"
	case KindUUID:
		return "UUID"
	case KindJSON:
		return "JSON"
	case KindBinary:
		return "Binary"
	case KindSlug:
		return "Slug"
	case KindEmail:
		return "Email"
	case KindURL:
		return "URL"
	case KindGenericIPAddress:
		return "GenericIPAddress"
	default:
		return fmt.Sprintf("Kind(%d)", k)
	}
}

// djangoKindFieldName returns the Django field class name for error messages.
func djangoKindFieldName(k Kind) string {
	switch k {
	case KindSmallInt:
		return "SmallIntegerField"
	case KindBigInt:
		return "BigIntegerField"
	case KindPositiveSmallInt:
		return "PositiveSmallIntegerField"
	case KindPositiveInt:
		return "PositiveIntegerField"
	case KindPositiveBigInt:
		return "PositiveBigIntegerField"
	case KindFloat:
		return "FloatField"
	case KindDecimal:
		return "DecimalField"
	case KindDate:
		return "DateField"
	case KindTime:
		return "TimeField"
	case KindDuration:
		return "DurationField"
	case KindUUID:
		return "UUIDField"
	case KindJSON:
		return "JSONField"
	case KindBinary:
		return "BinaryField"
	case KindSlug:
		return "SlugField"
	case KindEmail:
		return "EmailField"
	case KindURL:
		return "URLField"
	case KindGenericIPAddress:
		return "GenericIPAddressField"
	default:
		return "Field"
	}
}

// IntRange returns the documented inclusive [min, max] integer range of f when
// f is an integer kind, matching Django's integer_field_ranges.
func (f *Field) IntRange() (minVal, maxVal int64, ok bool) {
	switch f.Kind {
	case KindSmallInt:
		return -32768, 32767, true
	case KindInt:
		return -2147483648, 2147483647, true
	case KindAuto, KindBigInt:
		return math.MinInt64, math.MaxInt64, true
	case KindPositiveSmallInt:
		return 0, 32767, true
	case KindPositiveInt:
		return 0, 2147483647, true
	case KindPositiveBigInt:
		return 0, math.MaxInt64, true
	default:
		return 0, 0, false
	}
}

// isIntegerKind reports whether k is any signed, unsigned, or auto integer kind.
func isIntegerKind(k Kind) bool {
	switch k {
	case KindAuto, KindInt, KindSmallInt, KindBigInt,
		KindPositiveSmallInt, KindPositiveInt, KindPositiveBigInt:
		return true
	default:
		return false
	}
}

// isTemporalAutoKind reports whether k supports auto_now and auto_now_add.
func isTemporalAutoKind(k Kind) bool {
	return k == KindDateTime || k == KindDate || k == KindTime
}

// tagHasToken reports whether a semicolon-separated orm tag contains token.
func tagHasToken(tag, want string) bool {
	for _, tok := range strings.Split(tag, ";") {
		if strings.TrimSpace(tok) == want {
			return true
		}
	}
	return false
}

// inferFieldKind infers the initial Kind and defaults on f from Go type rt and
// its raw orm tag. Pointer types unwrap one level and imply Null=true.
func inferFieldKind(f *Field, rt reflect.Type, fieldName, tag string) error {
	if rt.Kind() == reflect.Pointer {
		f.Null = true
		rt = rt.Elem()
	}

	// Explicit type=json allows any Go struct, slice, map, or interface.
	if tagHasToken(tag, "type=json") {
		f.Kind = KindJSON
		return nil
	}

	switch rt {
	case timeType:
		f.Kind = KindDateTime
		return nil
	case decimalType:
		f.Kind = KindDecimal
		return nil
	case dateType:
		f.Kind = KindDate
		return nil
	case timeOfDayType:
		f.Kind = KindTime
		return nil
	case durationType:
		f.Kind = KindDuration
		return nil
	case uuidType:
		f.Kind = KindUUID
		return nil
	case jsonRawMessageType:
		f.Kind = KindJSON
		return nil
	case byteSliceType:
		f.Kind = KindBinary
		f.Editable = false
		return nil
	case netipAddrType:
		f.Kind = KindGenericIPAddress
		f.Protocol = "both"
		return nil
	}

	switch rt.Kind() {
	case reflect.String:
		f.Kind = KindChar
		f.MaxLength = 255
	case reflect.Int, reflect.Int32, reflect.Int64:
		f.Kind = KindInt
	case reflect.Int8, reflect.Int16:
		f.Kind = KindSmallInt
	case reflect.Uint8, reflect.Uint16:
		f.Kind = KindPositiveSmallInt
	case reflect.Uint32:
		f.Kind = KindPositiveInt
	case reflect.Uint, reflect.Uint64:
		f.Kind = KindPositiveBigInt
	case reflect.Float32, reflect.Float64:
		f.Kind = KindFloat
	case reflect.Bool:
		f.Kind = KindBool
	default:
		return fmt.Errorf("orm: field %s: unsupported type %s", fieldName, rt)
	}
	return nil
}

// applyKindTagOption handles tag options introduced for the extended field
// kinds. It reports whether it handled token.
func applyKindTagOption(f *Field, fieldName, token string) (bool, error) {
	switch {
	case token == "index=false":
		f.DBIndex = false
		return true, nil
	case token == "index=true":
		f.DBIndex = true
		return true, nil
	case token == "allow_unicode":
		if f.Kind != KindSlug {
			return true, fmt.Errorf("orm: field %s: allow_unicode is only valid for slug fields", fieldName)
		}
		f.AllowUnicode = true
		return true, nil
	case strings.HasPrefix(token, "protocol="):
		proto := strings.ToLower(token[len("protocol="):])
		switch proto {
		case "both", "ipv4", "ipv6":
			f.Protocol = proto
		default:
			return true, fmt.Errorf("orm: field %s: invalid protocol %q: must be both, ipv4, or ipv6", fieldName, proto)
		}
		return true, nil
	case strings.HasPrefix(token, "max_digits="):
		raw := token[len("max_digits="):]
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 {
			return true, fmt.Errorf("orm: field %s: invalid max_digits %q: must be a positive integer", fieldName, raw)
		}
		f.MaxDigits = n
		return true, nil
	case strings.HasPrefix(token, "decimal_places="):
		raw := token[len("decimal_places="):]
		n, err := strconv.Atoi(raw)
		if err != nil || n < 0 {
			return true, fmt.Errorf("orm: field %s: invalid decimal_places %q: must be a non-negative integer", fieldName, raw)
		}
		f.DecimalPlaces = n
		return true, nil
	case strings.HasPrefix(token, "type="):
		return true, applyTypeTag(f, fieldName, token[len("type="):])
	default:
		return false, nil
	}
}

// applyTypeTag applies a type=<name> tag token to f, validating compatibility
// with the Go type inferred earlier.
func applyTypeTag(f *Field, fieldName, typeName string) error {
	switch typeName {
	case "text":
		if f.Kind != KindChar {
			return fmt.Errorf("orm: field %s: type=text is only valid for string fields", fieldName)
		}
		f.Kind = KindText
		f.MaxLength = 0
	case "slug":
		if f.Kind != KindChar {
			return fmt.Errorf("orm: field %s: type=slug is only valid for string fields", fieldName)
		}
		f.Kind = KindSlug
		// Django's SlugField defaults to max_length=50 and db_index=True.
		if f.MaxLength == 255 {
			f.MaxLength = 50
		}
		f.DBIndex = true
	case "email":
		if f.Kind != KindChar {
			return fmt.Errorf("orm: field %s: type=email is only valid for string fields", fieldName)
		}
		f.Kind = KindEmail
		if f.MaxLength == 255 {
			f.MaxLength = 254
		}
	case "url":
		if f.Kind != KindChar {
			return fmt.Errorf("orm: field %s: type=url is only valid for string fields", fieldName)
		}
		f.Kind = KindURL
		if f.MaxLength == 255 {
			f.MaxLength = 200
		}
	case "ip", "generic_ip":
		if f.Kind != KindChar && f.Kind != KindGenericIPAddress {
			return fmt.Errorf("orm: field %s: type=%s is only valid for string or netip.Addr fields", fieldName, typeName)
		}
		f.Kind = KindGenericIPAddress
		f.MaxLength = 0
		if f.Protocol == "" {
			f.Protocol = "both"
		}
	case "uuid":
		if f.Kind != KindChar && f.Kind != KindUUID {
			return fmt.Errorf("orm: field %s: type=uuid is only valid for orm.UUID or string fields", fieldName)
		}
		f.Kind = KindUUID
		f.MaxLength = 0
	case "date":
		if f.Kind != KindDateTime && f.Kind != KindDate {
			return fmt.Errorf("orm: field %s: type=date is only valid for orm.Date or time.Time fields", fieldName)
		}
		f.Kind = KindDate
	case "time":
		if f.Kind != KindDateTime && f.Kind != KindTime {
			return fmt.Errorf("orm: field %s: type=time is only valid for orm.Time or time.Time fields", fieldName)
		}
		f.Kind = KindTime
	case "duration":
		if f.Kind != KindDuration {
			return fmt.Errorf("orm: field %s: type=duration is only valid for time.Duration fields", fieldName)
		}
	case "float":
		if f.Kind != KindFloat {
			return fmt.Errorf("orm: field %s: type=float is only valid for float fields", fieldName)
		}
	case "decimal":
		if f.Kind != KindDecimal {
			return fmt.Errorf("orm: field %s: type=decimal is only valid for orm.Decimal fields", fieldName)
		}
	case "json":
		if f.Kind != KindJSON {
			return fmt.Errorf("orm: field %s: type=json is not compatible with %s", fieldName, f.Kind)
		}
	case "binary":
		if f.Kind != KindBinary {
			return fmt.Errorf("orm: field %s: type=binary is only valid for []byte fields", fieldName)
		}
	case "int":
		if !isIntegerKind(f.Kind) {
			return fmt.Errorf("orm: field %s: type=int is only valid for integer fields", fieldName)
		}
		f.Kind = KindInt
	case "smallint":
		if !isIntegerKind(f.Kind) {
			return fmt.Errorf("orm: field %s: type=smallint is only valid for integer fields", fieldName)
		}
		f.Kind = KindSmallInt
	case "bigint":
		if !isIntegerKind(f.Kind) {
			return fmt.Errorf("orm: field %s: type=bigint is only valid for integer fields", fieldName)
		}
		f.Kind = KindBigInt
	case "positive_smallint":
		if !isIntegerKind(f.Kind) {
			return fmt.Errorf("orm: field %s: type=positive_smallint is only valid for integer fields", fieldName)
		}
		f.Kind = KindPositiveSmallInt
	case "positive_int":
		if !isIntegerKind(f.Kind) {
			return fmt.Errorf("orm: field %s: type=positive_int is only valid for integer fields", fieldName)
		}
		f.Kind = KindPositiveInt
	case "positive_bigint":
		if !isIntegerKind(f.Kind) {
			return fmt.Errorf("orm: field %s: type=positive_bigint is only valid for integer fields", fieldName)
		}
		f.Kind = KindPositiveBigInt
	default:
		return fmt.Errorf("orm: field %s: unknown field type %q", fieldName, typeName)
	}
	return nil
}

// validateFieldOptions checks cross-option invariants after all tags on f are
// parsed.
func validateFieldOptions(f *Field, fieldName string) error {
	if f.Kind == KindDecimal && f.MaxDigits > 0 && f.DecimalPlaces > f.MaxDigits {
		return fmt.Errorf("orm: field %s: decimal_places (%d) must not exceed max_digits (%d)",
			fieldName, f.DecimalPlaces, f.MaxDigits)
	}
	return nil
}

// parseKindDefault parses default= tag values for the extended field kinds.
func parseKindDefault(kind Kind, raw string) (any, error) {
	switch kind {
	case KindSmallInt, KindBigInt:
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid default %q: must be an integer", raw)
		}
		return n, nil
	case KindPositiveSmallInt, KindPositiveInt, KindPositiveBigInt:
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || n < 0 {
			return nil, fmt.Errorf("invalid default %q: must be a non-negative integer", raw)
		}
		return n, nil
	case KindFloat:
		f, err := strconv.ParseFloat(raw, 64)
		if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
			return nil, fmt.Errorf("invalid default %q: must be a finite float", raw)
		}
		return f, nil
	case KindDecimal:
		d, err := ParseDecimal(raw)
		if err != nil {
			return nil, fmt.Errorf("invalid default %q: %w", raw, err)
		}
		return d, nil
	case KindSlug, KindEmail, KindURL:
		return raw, nil
	case KindGenericIPAddress:
		ip, err := normalizeIPAddress(raw, "both")
		if err != nil {
			return nil, fmt.Errorf("invalid default %q: %w", raw, err)
		}
		return ip, nil
	case KindDate:
		d, err := ParseDate(raw)
		if err != nil {
			return nil, fmt.Errorf("invalid default %q: %w", raw, err)
		}
		return d, nil
	case KindTime:
		t, err := ParseTime(raw)
		if err != nil {
			return nil, fmt.Errorf("invalid default %q: %w", raw, err)
		}
		return t, nil
	case KindDuration:
		d, err := time.ParseDuration(raw)
		if err != nil {
			return nil, fmt.Errorf("invalid default %q: %w", raw, err)
		}
		return d, nil
	case KindUUID:
		if raw == "uuid4" {
			return DefaultUUID4, nil
		}
		u, err := ParseUUID(raw)
		if err != nil {
			return nil, fmt.Errorf("invalid default %q: %w", raw, err)
		}
		return u, nil
	case KindJSON:
		if !json.Valid([]byte(raw)) {
			return nil, fmt.Errorf("invalid default %q: must be valid JSON", raw)
		}
		return json.RawMessage(raw), nil
	case KindBinary:
		if strings.HasPrefix(raw, "0x") || strings.HasPrefix(raw, "0X") {
			b, err := hex.DecodeString(raw[2:])
			if err != nil {
				return nil, fmt.Errorf("invalid hex default %q: %w", raw, err)
			}
			return b, nil
		}
		return []byte(raw), nil
	default:
		return nil, fmt.Errorf("default= is not supported for this field type")
	}
}

// setKindDefaultValue assigns parsed defaults for the extended kinds (and
// pointer fields) onto fv.
func setKindDefaultValue(fv reflect.Value, def any) {
	if fv.Kind() == reflect.Pointer {
		elem := reflect.New(fv.Type().Elem())
		setDefaultValue(elem.Elem(), def)
		fv.Set(elem)
		return
	}
	switch d := def.(type) {
	case float64:
		if fv.Kind() == reflect.Float32 || fv.Kind() == reflect.Float64 {
			fv.SetFloat(d)
		}
	case Decimal:
		if fv.Type() == decimalType {
			fv.Set(reflect.ValueOf(d))
		}
	case Date:
		switch fv.Type() {
		case dateType:
			fv.Set(reflect.ValueOf(d))
		case timeType:
			fv.Set(reflect.ValueOf(d.Time()))
		}
	case Time:
		switch fv.Type() {
		case timeOfDayType:
			fv.Set(reflect.ValueOf(d))
		case timeType:
			fv.Set(reflect.ValueOf(time.Date(0, 1, 1, d.Hour, d.Minute, d.Second, d.Nanosecond, time.UTC)))
		}
	case time.Duration:
		if fv.Type() == durationType {
			fv.Set(reflect.ValueOf(d))
		}
	case UUID:
		switch fv.Type() {
		case uuidType:
			fv.Set(reflect.ValueOf(d))
		case reflect.TypeOf(""):
			fv.SetString(d.String())
		}
	case uuid4Default:
		u := NewUUID()
		switch fv.Type() {
		case uuidType:
			fv.Set(reflect.ValueOf(u))
		case reflect.TypeOf(""):
			fv.SetString(u.String())
		}
	case json.RawMessage:
		if fv.Type() == jsonRawMessageType {
			cp := make(json.RawMessage, len(d))
			copy(cp, d)
			fv.Set(reflect.ValueOf(cp))
		} else {
			_ = json.Unmarshal(d, fv.Addr().Interface())
		}
	case []byte:
		if fv.Type() == byteSliceType {
			cp := make([]byte, len(d))
			copy(cp, d)
			fv.Set(reflect.ValueOf(cp))
		}
	case string:
		if fv.Type() == netipAddrType {
			if addr, err := netip.ParseAddr(d); err == nil {
				fv.Set(reflect.ValueOf(addr))
			}
		}
	}
}

// stampTemporalAuto writes now into an auto_now or auto_now_add field of kind
// DateTime, Date, or Time (handling both value and pointer struct fields).
func stampTemporalAuto(fv reflect.Value, kind Kind, now time.Time) {
	if fv.Kind() == reflect.Pointer {
		elem := reflect.New(fv.Type().Elem())
		stampTemporalAuto(elem.Elem(), kind, now)
		fv.Set(elem)
		return
	}
	switch kind {
	case KindDateTime:
		fv.Set(reflect.ValueOf(now))
	case KindDate:
		if fv.Type() == dateType {
			fv.Set(reflect.ValueOf(DateFromTime(now)))
		} else if fv.Type() == timeType {
			fv.Set(reflect.ValueOf(DateFromTime(now).Time()))
		}
	case KindTime:
		if fv.Type() == timeOfDayType {
			fv.Set(reflect.ValueOf(TimeFromTime(now)))
		} else if fv.Type() == timeType {
			t := TimeFromTime(now)
			fv.Set(reflect.ValueOf(time.Date(0, 1, 1, t.Hour, t.Minute, t.Second, t.Nanosecond, time.UTC)))
		}
	}
}

// normalizeIPAddress trims s, validates it as an IPv4/IPv6 address according to
// protocol ("both", "ipv4", or "ipv6"), and returns its compressed canonical
// form.
func normalizeIPAddress(s, protocol string) (string, error) {
	trimmed := strings.TrimSpace(s)
	addr, err := netip.ParseAddr(trimmed)
	if err != nil || addr.Zone() != "" {
		if strings.Contains(trimmed, ":") && protocol != "ipv4" {
			return "", fmt.Errorf("orm: Enter a valid IPv6 address")
		}
		switch protocol {
		case "ipv4":
			return "", fmt.Errorf("orm: Enter a valid IPv4 address")
		case "ipv6":
			return "", fmt.Errorf("orm: Enter a valid IPv6 address")
		default:
			return "", fmt.Errorf("orm: Enter a valid IPv4 or IPv6 address")
		}
	}
	switch protocol {
	case "ipv4":
		if !addr.Is4() {
			return "", fmt.Errorf("orm: Enter a valid IPv4 address")
		}
	case "ipv6":
		if !addr.Is6() {
			return "", fmt.Errorf("orm: Enter a valid IPv6 address")
		}
	}
	return addr.String(), nil
}

// bindField converts a Go value v for field f into the parameter value expected
// by dialect d's database/sql driver.
func bindField(d Dialect, f *Field, v any) (any, error) {
	if v == nil {
		return nil, nil
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return nil, nil
		}
		v = rv.Elem().Interface()
	}
	if f == nil {
		return bindUntyped(v)
	}

	switch f.Kind {
	case KindGenericIPAddress:
		return bindIPAddress(f, v)
	case KindUUID:
		u, err := toUUID(v)
		if err != nil {
			return nil, err
		}
		if vd, ok := d.(ValueDialect); ok {
			return vd.BindValue(f, u)
		}
		return u.String(), nil
	case KindDuration:
		dur, ok := v.(time.Duration)
		if !ok {
			return nil, fmt.Errorf("orm: field %s: expected time.Duration, got %T", f.Name, v)
		}
		if vd, ok := d.(ValueDialect); ok {
			return vd.BindValue(f, dur)
		}
		return int64(dur / time.Microsecond), nil
	case KindDecimal:
		dec, err := toDecimal(v)
		if err != nil {
			return nil, err
		}
		return dec.FixedString(), nil
	case KindDate:
		return bindDate(v)
	case KindTime:
		return bindTimeOfDay(v)
	case KindJSON:
		return bindJSON(f, v)
	case KindBinary:
		return bindBinary(f, v)
	}

	// Let ValueDialect customize any other kind if needed.
	if vd, ok := d.(ValueDialect); ok {
		return vd.BindValue(f, v)
	}
	return bindUntyped(v)
}

// bindUntyped normalizes values that database/sql does not accept directly
// (such as uint64 or custom Value types) when no field metadata is available.
func bindUntyped(v any) (any, error) {
	switch x := v.(type) {
	case uint:
		if uint64(x) > math.MaxInt64 {
			return nil, fmt.Errorf("orm: unsigned value %d exceeds int64 range", x)
		}
		return int64(x), nil
	case uint8:
		return int64(x), nil
	case uint16:
		return int64(x), nil
	case uint32:
		return int64(x), nil
	case uint64:
		if x > math.MaxInt64 {
			return nil, fmt.Errorf("orm: unsigned value %d exceeds int64 range", x)
		}
		return int64(x), nil
	case netip.Addr:
		if !x.IsValid() {
			return nil, nil
		}
		return x.String(), nil
	}
	return v, nil
}

// bindIPAddress converts a string or netip.Addr for a KindGenericIPAddress field.
func bindIPAddress(f *Field, v any) (any, error) {
	switch x := v.(type) {
	case netip.Addr:
		if !x.IsValid() {
			if f.Null {
				return nil, nil
			}
			return nil, fmt.Errorf("orm: field %s: invalid IP address", f.Name)
		}
		return normalizeIPAddress(x.String(), f.Protocol)
	case string:
		if strings.TrimSpace(x) == "" {
			if f.Null {
				return nil, nil
			}
			return nil, fmt.Errorf("orm: field %s: IP address cannot be blank", f.Name)
		}
		return normalizeIPAddress(x, f.Protocol)
	default:
		return nil, fmt.Errorf("orm: field %s: unsupported IP value %T", f.Name, v)
	}
}

// toUUID converts a UUID or string into a UUID.
func toUUID(v any) (UUID, error) {
	switch x := v.(type) {
	case UUID:
		return x, nil
	case string:
		return ParseUUID(x)
	default:
		return UUID{}, fmt.Errorf("orm: %v is not a valid UUID", v)
	}
}

// toDecimal converts a Decimal, string, integer, or float into a Decimal.
func toDecimal(v any) (Decimal, error) {
	switch x := v.(type) {
	case Decimal:
		return x, nil
	case string:
		return ParseDecimal(x)
	case int:
		return DecimalFromInt(int64(x)), nil
	case int8:
		return DecimalFromInt(int64(x)), nil
	case int16:
		return DecimalFromInt(int64(x)), nil
	case int32:
		return DecimalFromInt(int64(x)), nil
	case int64:
		return DecimalFromInt(x), nil
	case uint:
		return ParseDecimal(strconv.FormatUint(uint64(x), 10))
	case uint8:
		return DecimalFromInt(int64(x)), nil
	case uint16:
		return DecimalFromInt(int64(x)), nil
	case uint32:
		return DecimalFromInt(int64(x)), nil
	case uint64:
		return ParseDecimal(strconv.FormatUint(x, 10))
	case float32:
		return DecimalFromFloat(float64(x))
	case float64:
		return DecimalFromFloat(x)
	default:
		return Decimal{}, fmt.Errorf("orm: %v value must be a decimal number", v)
	}
}

// bindDate converts a Date, time.Time, or string into a "YYYY-MM-DD" parameter.
func bindDate(v any) (any, error) {
	switch x := v.(type) {
	case Date:
		if x.IsZero() {
			return nil, nil
		}
		return x.String(), nil
	case time.Time:
		if x.IsZero() {
			return nil, nil
		}
		return DateFromTime(x).String(), nil
	case string:
		d, err := ParseDate(x)
		if err != nil {
			return nil, err
		}
		return d.String(), nil
	default:
		return nil, fmt.Errorf("orm: cannot bind %T to a date field", v)
	}
}

// bindTimeOfDay converts a Time, time.Time, or string into a "HH:MM:SS[.ffffff]"
// parameter.
func bindTimeOfDay(v any) (any, error) {
	switch x := v.(type) {
	case Time:
		return x.String(), nil
	case time.Time:
		return TimeFromTime(x).String(), nil
	case string:
		t, err := ParseTime(x)
		if err != nil {
			return nil, err
		}
		return t.String(), nil
	default:
		return nil, fmt.Errorf("orm: cannot bind %T to a time field", v)
	}
}

// bindJSON converts json.RawMessage or any Go value into a JSON string for a
// KindJSON column.
func bindJSON(f *Field, v any) (any, error) {
	if raw, ok := v.(json.RawMessage); ok {
		if len(raw) == 0 {
			if f.Null {
				return nil, nil
			}
			return "null", nil
		}
		return string(raw), nil
	}
	rv := reflect.ValueOf(v)
	if (rv.Kind() == reflect.Slice || rv.Kind() == reflect.Map) && rv.IsNil() && f.Null {
		return nil, nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("orm: field %s: json marshal: %w", f.Name, err)
	}
	return string(b), nil
}

// bindBinary converts a []byte for a KindBinary column.
func bindBinary(f *Field, v any) (any, error) {
	switch x := v.(type) {
	case []byte:
		if x == nil {
			if f.Null {
				return nil, nil
			}
			return []byte{}, nil
		}
		return x, nil
	default:
		return nil, fmt.Errorf("orm: field %s: expected []byte, got %T", f.Name, v)
	}
}

// hasNativeUUID reports whether dialect d stores UUIDs natively with hyphens.
func hasNativeUUID(d Dialect) bool {
	if vd, ok := d.(ValueDialect); ok {
		return vd.HasNativeUUID()
	}
	return false
}

// fieldScanner adapts a driver value into a struct field, handling NULLs,
// pointer fields, and dialect-specific encodings such as SQLite decimals and
// PostgreSQL intervals.
type fieldScanner struct {
	dialect Dialect
	field   *Field
	dst     reflect.Value
}

// Scan implements sql.Scanner.
func (s fieldScanner) Scan(src any) error {
	return assignFieldScanned(s.dialect, s.field, s.dst, src)
}

// assignFieldScanned converts driver value v according to f and d and writes it
// into dst.
func assignFieldScanned(d Dialect, f *Field, dst reflect.Value, v any) error {
	if dst.Kind() == reflect.Pointer {
		if v == nil {
			dst.SetZero()
			return nil
		}
		elem := reflect.New(dst.Type().Elem())
		if err := assignFieldScanned(d, f, elem.Elem(), v); err != nil {
			return err
		}
		dst.Set(elem)
		return nil
	}
	if v == nil {
		dst.SetZero()
		return nil
	}
	if f == nil {
		return assignScanned(dst, v)
	}

	switch f.Kind {
	case KindDuration:
		dur, err := scanDuration(v)
		if err != nil {
			return err
		}
		dst.Set(reflect.ValueOf(dur))
		return nil
	case KindDecimal:
		var dec Decimal
		if err := dec.Scan(v); err != nil {
			return err
		}
		// SQLite strips trailing zeros on NUMERIC affinity; re-quantize to the
		// declared decimal_places just as Django's SQLite converter does.
		if f.MaxDigits > 0 && d != nil && d.Name() == "sqlite" {
			dec = dec.Quantize(f.DecimalPlaces)
		}
		dst.Set(reflect.ValueOf(dec))
		return nil
	case KindDate:
		var dt Date
		if err := dt.Scan(v); err != nil {
			return err
		}
		if dst.Type() == timeType {
			dst.Set(reflect.ValueOf(dt.Time()))
		} else {
			dst.Set(reflect.ValueOf(dt))
		}
		return nil
	case KindTime:
		var tod Time
		if err := tod.Scan(v); err != nil {
			return err
		}
		if dst.Type() == timeType {
			dst.Set(reflect.ValueOf(time.Date(0, 1, 1, tod.Hour, tod.Minute, tod.Second, tod.Nanosecond, time.UTC)))
		} else {
			dst.Set(reflect.ValueOf(tod))
		}
		return nil
	case KindUUID:
		var u UUID
		if err := u.Scan(v); err != nil {
			return err
		}
		if dst.Kind() == reflect.String {
			dst.SetString(u.String())
		} else {
			dst.Set(reflect.ValueOf(u))
		}
		return nil
	case KindJSON:
		return scanJSON(dst, v)
	case KindBinary:
		return scanBinary(dst, v)
	case KindGenericIPAddress:
		return scanIPAddress(dst, v)
	}

	return assignScanned(dst, v)
}

// scanDuration converts a driver value (SQLite int64 microseconds or PostgreSQL
// INTERVAL text) into a time.Duration.
func scanDuration(v any) (time.Duration, error) {
	switch x := v.(type) {
	case time.Duration:
		return x, nil
	case int64:
		return time.Duration(x) * time.Microsecond, nil
	case string:
		return ParsePGInterval(x)
	case []byte:
		return ParsePGInterval(string(x))
	default:
		return 0, fmt.Errorf("orm: cannot convert %T to time.Duration", v)
	}
}

// scanJSON writes JSON text/bytes from the driver into json.RawMessage or
// unmarshals it into a Go struct, map, slice, or interface.
func scanJSON(dst reflect.Value, v any) error {
	var raw []byte
	switch x := v.(type) {
	case []byte:
		raw = append([]byte(nil), x...)
	case string:
		raw = []byte(x)
	default:
		return fmt.Errorf("orm: cannot scan %T into JSON field", v)
	}
	if dst.Type() == jsonRawMessageType {
		dst.Set(reflect.ValueOf(json.RawMessage(raw)))
		return nil
	}
	if err := json.Unmarshal(raw, dst.Addr().Interface()); err != nil {
		return fmt.Errorf("orm: json unmarshal into %s: %w", dst.Type(), err)
	}
	return nil
}

// scanBinary copies binary bytes from the driver into a fresh []byte slice.
func scanBinary(dst reflect.Value, v any) error {
	switch x := v.(type) {
	case []byte:
		cp := make([]byte, len(x))
		copy(cp, x)
		dst.SetBytes(cp)
		return nil
	case string:
		dst.SetBytes([]byte(x))
		return nil
	default:
		return fmt.Errorf("orm: cannot scan %T into []byte", v)
	}
}

// scanIPAddress writes an IP address string/bytes from the driver into a
// netip.Addr or string field.
func scanIPAddress(dst reflect.Value, v any) error {
	var s string
	switch x := v.(type) {
	case string:
		s = x
	case []byte:
		s = string(x)
	case netip.Addr:
		s = x.String()
	case netip.Prefix:
		s = x.Addr().String()
	default:
		return fmt.Errorf("orm: cannot scan %T into IP address field", v)
	}
	if dst.Type() == netipAddrType {
		addr, err := netip.ParseAddr(strings.TrimSpace(s))
		if err != nil {
			return fmt.Errorf("orm: scan netip.Addr %q: %w", s, err)
		}
		dst.Set(reflect.ValueOf(addr))
		return nil
	}
	dst.SetString(s)
	return nil
}

// convertKindValue converts a raw driver value for the extended valueType
// constants used by Values, ValuesList, and Aggregate.
func convertKindValue(v any, t valueType) (any, error) {
	switch t {
	case typeDecimal:
		var d Decimal
		if err := d.Scan(v); err != nil {
			return nil, err
		}
		return d, nil
	case typeDate:
		var dt Date
		if err := dt.Scan(v); err != nil {
			return nil, err
		}
		return dt, nil
	case typeTimeOfDay:
		var tod Time
		if err := tod.Scan(v); err != nil {
			return nil, err
		}
		return tod, nil
	case typeDuration:
		return scanDuration(v)
	case typeUUID:
		var u UUID
		if err := u.Scan(v); err != nil {
			return nil, err
		}
		return u, nil
	case typeJSON:
		switch x := v.(type) {
		case []byte:
			return json.RawMessage(append([]byte(nil), x...)), nil
		case string:
			return json.RawMessage(x), nil
		default:
			return nil, fmt.Errorf("orm: cannot convert %T to json.RawMessage", v)
		}
	case typeBinary:
		switch x := v.(type) {
		case []byte:
			cp := make([]byte, len(x))
			copy(cp, x)
			return cp, nil
		case string:
			return []byte(x), nil
		default:
			return nil, fmt.Errorf("orm: cannot convert %T to []byte", v)
		}
	default:
		return v, nil
	}
}

// Ensure our custom types implement sql.Scanner and driver.Valuer at compile time.
var (
	_ sql.Scanner   = (*Decimal)(nil)
	_ driver.Valuer = Decimal{}
	_ sql.Scanner   = (*Date)(nil)
	_ driver.Valuer = Date{}
	_ sql.Scanner   = (*Time)(nil)
	_ driver.Valuer = Time{}
	_ sql.Scanner   = (*UUID)(nil)
	_ driver.Valuer = UUID{}
)
