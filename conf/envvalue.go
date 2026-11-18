package conf

import (
	"encoding"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
	"time"
	"unicode"
)

var (
	// durationType is handled before the integer kinds: its text is seconds or
	// a Go duration, not nanoseconds.
	durationType = reflect.TypeFor[time.Duration]()
	// textUnmarshalerType marks the types that parse their own text.
	textUnmarshalerType = reflect.TypeFor[encoding.TextUnmarshaler]()
)

// errUnsupportedType reports a field whose type has no environment syntax.
var errUnsupportedType = errors.New("unsupported type")

// syntaxError reports a value that does not have the syntax of its field type.
type syntaxError struct {
	// kind names the expected syntax, for example "boolean".
	kind string
	// value is the malformed text.
	value string
	// hint says what is accepted; it never holds a part of the value.
	hint string
	// cause is the error of a type that parsed its own text; it may quote the
	// value.
	cause error
}

// Error describes the problem and quotes the value.
func (e *syntaxError) Error() string { return e.describe(false) }

// describe returns the problem. With hidden, the value and the cause are left
// out, for a variable that may hold a secret.
func (e *syntaxError) describe(hidden bool) string {
	var b strings.Builder
	b.WriteString("invalid " + e.kind)
	if hidden {
		b.WriteString(" (the value is not shown)")
	} else {
		b.WriteString(" " + strconv.Quote(e.value))
		if e.cause != nil {
			b.WriteString(": " + e.cause.Error())
		}
	}
	if e.hint != "" {
		b.WriteString(" (" + e.hint + ")")
	}
	return b.String()
}

// envField is one struct field that the environment can set.
type envField struct {
	// index locates the field for reflect.Value.FieldByIndex.
	index []int
	// name is the variable that holds the value, without the prefix.
	name string
}

// envFields lists the fields of the struct type t that the environment can
// set. path is put in front of every name. A field is read under its env tag;
// the tag "-" excludes it. With derive, a field without a tag is read under
// the upper snake case of its name; without, such a field is not read. A
// nested struct adds its name and an underscore to the path of its fields,
// unless it has no name of its own: an embedded struct, and without derive
// every struct that has no tag.
func envFields(t reflect.Type, path string, derive bool) []envField {
	var fields []envField
	for i := range t.NumField() {
		f := t.Field(i)
		nested := isNestedStruct(f.Type)
		if !f.IsExported() && !isEmbeddedStruct(f) {
			continue
		}
		tag := f.Tag.Get("env")
		if tag == "-" {
			continue
		}
		segment := tag
		if segment == "" && derive && (!nested || !f.Anonymous) {
			segment = envName(f.Name)
		}
		switch {
		case nested:
			sub := path
			if segment != "" {
				sub += segment + "_"
			}
			for _, inner := range envFields(f.Type, sub, derive) {
				inner.index = append([]int{i}, inner.index...)
				fields = append(fields, inner)
			}
		case segment != "":
			fields = append(fields, envField{index: []int{i}, name: path + segment})
		}
	}
	return fields
}

// isNestedStruct reports whether the fields of t are read one by one: t is a
// struct that does not parse its own text.
func isNestedStruct(t reflect.Type) bool {
	return t.Kind() == reflect.Struct && !reflect.PointerTo(t).Implements(textUnmarshalerType)
}

// isEmbeddedStruct reports whether f is a struct embedded by value. Go
// promotes the exported fields of such a struct, and reflection can set them,
// also when the type of the struct is unexported.
func isEmbeddedStruct(f reflect.StructField) bool {
	return f.Anonymous && isNestedStruct(f.Type)
}

// envName turns a Go field name into a variable name: upper snake case, with
// an acronym kept together. PageSize gives PAGE_SIZE, APIKey gives API_KEY and
// HTTPOnly gives HTTP_ONLY.
func envName(field string) string {
	runes := []rune(field)
	var b strings.Builder
	for i, r := range runes {
		if i > 0 && unicode.IsUpper(r) {
			prev := runes[i-1]
			endsWord := unicode.IsLower(prev) || unicode.IsDigit(prev)
			// The last capital of an acronym starts the next word when a
			// lower-case letter follows it: the K of APIKey.
			endsAcronym := unicode.IsUpper(prev) && i+1 < len(runes) && unicode.IsLower(runes[i+1])
			if endsWord || endsAcronym {
				b.WriteByte('_')
			}
		}
		b.WriteRune(unicode.ToUpper(r))
	}
	return b.String()
}

// apply sets the fields of the struct v from the environment and returns
// one error per malformed variable. A field whose variable is unset or
// malformed keeps its value. path and derive are those of envFields.
func (e *envSource) apply(v reflect.Value, path string, derive bool) []error {
	var errs []error
	for _, f := range envFields(v.Type(), path, derive) {
		raw, set := e.lookup(e.prefix + f.name)
		if !set {
			continue
		}
		field := v.FieldByIndex(f.index)
		if err := setFromText(field, raw); err != nil {
			errs = append(errs, e.describe(f.name, field.Type(), err))
		}
	}
	return errs
}

// describe returns the error for the variable name whose value was refused
// with err. The value of a variable whose name suggests a secret is left out.
func (e *envSource) describe(name string, t reflect.Type, err error) error {
	full := e.prefix + name
	var syntax *syntaxError
	if errors.As(err, &syntax) {
		return fmt.Errorf("conf: %s: %s", full, syntax.describe(isSensitive(name)))
	}
	return fmt.Errorf("conf: %s: a field of the type %s cannot be set from the environment", full, t)
}

// sensitiveWords are the parts of a setting name that mark its value as a
// secret, as in Django's SafeExceptionReporterFilter.hidden_settings.
var sensitiveWords = []string{"API", "AUTH", "TOKEN", "KEY", "SECRET", "PASS", "SIGNATURE", "HTTP_COOKIE"}

// isSensitive reports whether a variable of this name may hold a secret.
func isSensitive(name string) bool {
	name = strings.ToUpper(name)
	for _, word := range sensitiveWords {
		if strings.Contains(name, word) {
			return true
		}
	}
	return false
}

// setFromText parses raw for the type of dst and stores the result. dst is
// left unchanged when raw is malformed.
func setFromText(dst reflect.Value, raw string) error {
	parsed := reflect.New(dst.Type()).Elem()
	if err := parseText(parsed, raw); err != nil {
		return err
	}
	dst.Set(parsed)
	return nil
}

// parseText parses raw into dst, which is settable and addressable.
func parseText(dst reflect.Value, raw string) error {
	t := dst.Type()
	if t == durationType {
		d, err := parseDuration(raw)
		if err != nil {
			return err
		}
		dst.SetInt(int64(d))
		return nil
	}
	if reflect.PointerTo(t).Implements(textUnmarshalerType) {
		u, _ := dst.Addr().Interface().(encoding.TextUnmarshaler)
		if err := u.UnmarshalText([]byte(raw)); err != nil {
			return &syntaxError{kind: "value", value: raw, cause: err}
		}
		return nil
	}
	switch t.Kind() {
	case reflect.String:
		dst.SetString(raw)
	case reflect.Bool:
		b, err := parseBool(raw)
		if err != nil {
			return err
		}
		dst.SetBool(b)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n, err := strconv.ParseInt(strings.TrimSpace(raw), 10, t.Bits())
		if err != nil {
			return numberError("integer", raw, err)
		}
		dst.SetInt(n)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		n, err := strconv.ParseUint(strings.TrimSpace(raw), 10, t.Bits())
		if err != nil {
			return numberError("unsigned integer", raw, err)
		}
		dst.SetUint(n)
	case reflect.Float32, reflect.Float64:
		n, err := strconv.ParseFloat(strings.TrimSpace(raw), t.Bits())
		if err != nil {
			return numberError("number", raw, err)
		}
		dst.SetFloat(n)
	case reflect.Slice:
		return parseList(dst, raw)
	default:
		return errUnsupportedType
	}
	return nil
}

// parseList parses a comma-separated list into the slice dst. Spaces around
// an element are trimmed and empty elements are dropped, so an empty value is
// an empty list.
func parseList(dst reflect.Value, raw string) error {
	if dst.Type().Elem().Kind() == reflect.Slice {
		return errUnsupportedType
	}
	var parts []string
	for part := range strings.SplitSeq(raw, ",") {
		if part = strings.TrimSpace(part); part != "" {
			parts = append(parts, part)
		}
	}
	list := reflect.MakeSlice(dst.Type(), len(parts), len(parts))
	for i, part := range parts {
		if err := parseText(list.Index(i), part); err != nil {
			return err
		}
	}
	dst.Set(list)
	return nil
}

// parseBool parses a boolean: 1, true, t, yes, y and on are true; 0, false, f,
// no, n and off are false, in any letter case.
func parseBool(raw string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "t", "yes", "y", "on":
		return true, nil
	case "0", "false", "f", "no", "n", "off":
		return false, nil
	}
	return false, &syntaxError{kind: "boolean", value: raw}
}

// parseDuration parses a whole number of seconds, the unit of Django's
// settings, or a Go duration string such as "36h".
func parseDuration(raw string) (time.Duration, error) {
	const hint = `want a number of seconds or a Go duration such as "36h"`
	text := strings.TrimSpace(raw)
	seconds, err := strconv.ParseInt(text, 10, 64)
	if err == nil {
		const limit = int64(math.MaxInt64 / time.Second)
		if seconds > limit || seconds < -limit {
			return 0, &syntaxError{kind: "duration", value: raw, hint: "out of range"}
		}
		return time.Duration(seconds) * time.Second, nil
	}
	if errors.Is(err, strconv.ErrRange) {
		return 0, &syntaxError{kind: "duration", value: raw, hint: "out of range"}
	}
	d, err := time.ParseDuration(text)
	if err != nil {
		return 0, &syntaxError{kind: "duration", value: raw, hint: hint}
	}
	return d, nil
}

// numberError returns the error for the text raw that strconv refused with err.
func numberError(kind, raw string, err error) error {
	if errors.Is(err, strconv.ErrRange) {
		return &syntaxError{kind: kind, value: raw, hint: "out of range"}
	}
	return &syntaxError{kind: kind, value: raw}
}
