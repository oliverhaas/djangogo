package pyjson

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"
)

// Options holds the json.dumps arguments that Marshal supports.
type Options struct {
	// Indent is the indent argument. A negative value means None, the
	// json.dumps default; zero puts each item on its own line without spaces.
	Indent int
	// ItemSep is the first of the separators; "" means the json.dumps default
	// for Indent.
	ItemSep string
	// KeySep is the second of the separators; "" means ": ".
	KeySep string
	// EnsureASCII escapes every character outside printable ASCII.
	EnsureASCII bool
	// Default is the default argument: it converts a value that Marshal
	// cannot encode. Without it, such a value fails with Python's TypeError
	// message.
	Default func(v any) (any, error)
}

// Marshal encodes v as a JSON string matching Python's json.dumps with opts.
func Marshal(v any, opts Options) (string, error) {
	var b strings.Builder
	if err := Encode(&b, v, opts); err != nil {
		return "", err
	}
	return b.String(), nil
}

// Encode writes the JSON encoding of v to b matching Python's json.dumps with opts.
func Encode(b *strings.Builder, v any, opts Options) error {
	e := encoder{
		b:           b,
		indent:      opts.Indent,
		itemSep:     opts.ItemSep,
		keySep:      opts.KeySep,
		ensureASCII: opts.EnsureASCII,
		defaultFn:   opts.Default,
	}
	if e.itemSep == "" {
		if e.indent >= 0 {
			e.itemSep = ","
		} else {
			e.itemSep = ", "
		}
	}
	if e.keySep == "" {
		e.keySep = ": "
	}
	return e.encode(v, 0)
}

// encoder holds the state and configuration for one JSON encoding pass.
type encoder struct {
	b           *strings.Builder
	indent      int
	itemSep     string
	keySep      string
	ensureASCII bool
	defaultFn   func(v any) (any, error)
}

// writeString writes s to the builder according to ensureASCII.
func (e *encoder) writeString(s string) {
	if e.ensureASCII {
		WriteString(e.b, s)
		return
	}
	e.b.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '"':
			e.b.WriteString(`\"`)
		case r == '\\':
			e.b.WriteString(`\\`)
		case r == '\n':
			e.b.WriteString(`\n`)
		case r == '\r':
			e.b.WriteString(`\r`)
		case r == '\t':
			e.b.WriteString(`\t`)
		case r == '\b':
			e.b.WriteString(`\b`)
		case r == '\f':
			e.b.WriteString(`\f`)
		case r >= 0 && r < 0x20:
			fmt.Fprintf(e.b, `\u%04x`, r)
		default:
			e.b.WriteRune(r)
		}
	}
	e.b.WriteByte('"')
}

// writeNewlineIndent writes a newline and the spaces for level.
func (e *encoder) writeNewlineIndent(level int) {
	e.b.WriteByte('\n')
	if e.indent > 0 && level > 0 {
		e.b.WriteString(strings.Repeat(" ", e.indent*level))
	}
}

// encode writes v at indentation level to the builder.
func (e *encoder) encode(v any, level int) error {
	switch val := v.(type) {
	case nil:
		e.b.WriteString("null")
		return nil
	case bool:
		if val {
			e.b.WriteString("true")
		} else {
			e.b.WriteString("false")
		}
		return nil
	case string:
		e.writeString(val)
		return nil
	case json.Number:
		e.b.WriteString(string(val))
		return nil
	case int:
		e.b.WriteString(strconv.FormatInt(int64(val), 10))
		return nil
	case int8:
		e.b.WriteString(strconv.FormatInt(int64(val), 10))
		return nil
	case int16:
		e.b.WriteString(strconv.FormatInt(int64(val), 10))
		return nil
	case int32:
		e.b.WriteString(strconv.FormatInt(int64(val), 10))
		return nil
	case int64:
		e.b.WriteString(strconv.FormatInt(val, 10))
		return nil
	case uint:
		e.b.WriteString(strconv.FormatUint(uint64(val), 10))
		return nil
	case uint8:
		e.b.WriteString(strconv.FormatUint(uint64(val), 10))
		return nil
	case uint16:
		e.b.WriteString(strconv.FormatUint(uint64(val), 10))
		return nil
	case uint32:
		e.b.WriteString(strconv.FormatUint(uint64(val), 10))
		return nil
	case uint64:
		e.b.WriteString(strconv.FormatUint(val, 10))
		return nil
	case uintptr:
		e.b.WriteString(strconv.FormatUint(uint64(val), 10))
		return nil
	case float64:
		e.b.WriteString(FormatFloat(val))
		return nil
	case float32:
		f, _ := strconv.ParseFloat(strconv.FormatFloat(float64(val), 'g', -1, 32), 64)
		e.b.WriteString(FormatFloat(f))
		return nil
	case Object:
		return e.encodeObject(val, level)
	case []any:
		return e.encodeSlice(val, level)
	case map[string]any:
		return e.encodeStringMap(val, level)
	}

	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Slice, reflect.Array:
		if rv.Type() != reflect.TypeFor[[]byte]() {
			return e.encodeReflectList(rv, level)
		}
	case reflect.Map:
		if rv.Type().Key().Kind() == reflect.String {
			return e.encodeReflectMap(rv, level)
		}
	}

	return e.encodeDefault(v, level)
}

// encodeDefault invokes defaultFn on v or returns a TypeError-matching error.
func (e *encoder) encodeDefault(v any, level int) error {
	if e.defaultFn != nil {
		next, err := e.defaultFn(v)
		if err != nil {
			return err
		}
		return e.encode(next, level)
	}
	return notSerializableError(v)
}

// notSerializableError builds the not-JSON-serializable error for v.
func notSerializableError(v any) error {
	t := reflect.TypeOf(v)
	if t == nil {
		return errors.New("Object of type nil is not JSON serializable")
	}
	name := t.Name()
	if name == "" {
		name = t.String()
	}
	return fmt.Errorf("Object of type %s is not JSON serializable", name)
}

// encodeSlice writes items as a JSON array.
func (e *encoder) encodeSlice(items []any, level int) error {
	if len(items) == 0 {
		e.b.WriteString("[]")
		return nil
	}
	e.b.WriteByte('[')
	nextLevel := level + 1
	if e.indent >= 0 {
		e.writeNewlineIndent(nextLevel)
	}
	for i, item := range items {
		if i > 0 {
			e.b.WriteString(e.itemSep)
			if e.indent >= 0 {
				e.writeNewlineIndent(nextLevel)
			}
		}
		if err := e.encode(item, nextLevel); err != nil {
			return err
		}
	}
	if e.indent >= 0 {
		e.writeNewlineIndent(level)
	}
	e.b.WriteByte(']')
	return nil
}

// encodeReflectList writes a reflect slice or array as a JSON array.
func (e *encoder) encodeReflectList(rv reflect.Value, level int) error {
	n := rv.Len()
	if n == 0 {
		e.b.WriteString("[]")
		return nil
	}
	e.b.WriteByte('[')
	nextLevel := level + 1
	if e.indent >= 0 {
		e.writeNewlineIndent(nextLevel)
	}
	for i := range n {
		if i > 0 {
			e.b.WriteString(e.itemSep)
			if e.indent >= 0 {
				e.writeNewlineIndent(nextLevel)
			}
		}
		if err := e.encode(rv.Index(i).Interface(), nextLevel); err != nil {
			return err
		}
	}
	if e.indent >= 0 {
		e.writeNewlineIndent(level)
	}
	e.b.WriteByte(']')
	return nil
}

// encodeObject writes obj as a JSON object keeping member order.
func (e *encoder) encodeObject(obj Object, level int) error {
	if len(obj) == 0 {
		e.b.WriteString("{}")
		return nil
	}
	e.b.WriteByte('{')
	nextLevel := level + 1
	if e.indent >= 0 {
		e.writeNewlineIndent(nextLevel)
	}
	for i, m := range obj {
		if i > 0 {
			e.b.WriteString(e.itemSep)
			if e.indent >= 0 {
				e.writeNewlineIndent(nextLevel)
			}
		}
		e.writeString(m.Key)
		e.b.WriteString(e.keySep)
		if err := e.encode(m.Value, nextLevel); err != nil {
			return err
		}
	}
	if e.indent >= 0 {
		e.writeNewlineIndent(level)
	}
	e.b.WriteByte('}')
	return nil
}

// encodeStringMap writes m as a JSON object with sorted keys.
func (e *encoder) encodeStringMap(m map[string]any, level int) error {
	if len(m) == 0 {
		e.b.WriteString("{}")
		return nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	members := make(Object, len(keys))
	for i, k := range keys {
		members[i] = Member{Key: k, Value: m[k]}
	}
	return e.encodeObject(members, level)
}

// encodeReflectMap writes a string-keyed reflect map as a JSON object with sorted keys.
func (e *encoder) encodeReflectMap(rv reflect.Value, level int) error {
	if rv.Len() == 0 {
		e.b.WriteString("{}")
		return nil
	}
	type pair struct {
		key string
		val any
	}
	pairs := make([]pair, 0, rv.Len())
	iter := rv.MapRange()
	for iter.Next() {
		pairs = append(pairs, pair{
			key: iter.Key().String(),
			val: iter.Value().Interface(),
		})
	}
	slices.SortFunc(pairs, func(a, b pair) int {
		return strings.Compare(a.key, b.key)
	})
	members := make(Object, len(pairs))
	for i, p := range pairs {
		members[i] = Member{Key: p.key, Value: p.val}
	}
	return e.encodeObject(members, level)
}
