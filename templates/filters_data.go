package templates

import (
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/flosch/pongo2/v6"

	"github.com/oliverhaas/djangogo/orm"
)

// pyJSON writes v as Python's json.dumps does by default: ", " and ": " as
// separators and every non-ASCII character as a \u escape. Map keys are
// sorted, since a Go map has no order to keep.
func pyJSON(b *strings.Builder, v any) error {
	switch x := v.(type) {
	case nil:
		b.WriteString("null")
		return nil
	case string:
		pyJSONString(b, x)
		return nil
	case modelView:
		pyJSONString(b, x.label())
		return nil
	case templateTime:
		pyJSONString(b, x.Format("2006-01-02T15:04:05.999999Z07:00"))
		return nil
	case time.Time:
		pyJSONString(b, x.Format("2006-01-02T15:04:05.999999Z07:00"))
		return nil
	case templateDate:
		pyJSONString(b, x.Date.String())
		return nil
	case orm.Date:
		pyJSONString(b, x.String())
		return nil
	case templateTimeOfDay:
		pyJSONString(b, x.Time.String())
		return nil
	case orm.Time:
		pyJSONString(b, x.String())
		return nil
	case orm.Decimal:
		pyJSONString(b, x.String())
		return nil
	case orm.UUID:
		pyJSONString(b, x.String())
		return nil
	case templateDuration:
		pyJSONString(b, x.String())
		return nil
	case time.Duration:
		pyJSONString(b, formatDurationPython(x))
		return nil
	case *pongo2.Value:
		return pyJSON(b, x.Interface())
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Pointer, reflect.Interface:
		if rv.IsNil() {
			b.WriteString("null")
			return nil
		}
		return pyJSON(b, rv.Elem().Interface())
	case reflect.Slice, reflect.Array:
		if rv.Kind() == reflect.Slice && rv.IsNil() {
			b.WriteString("null")
			return nil
		}
		b.WriteByte('[')
		for i := range rv.Len() {
			if i > 0 {
				b.WriteString(", ")
			}
			if err := pyJSON(b, rv.Index(i).Interface()); err != nil {
				return err
			}
		}
		b.WriteByte(']')
		return nil
	case reflect.Map:
		if rv.Type().Key().Kind() != reflect.String {
			return fmt.Errorf("json_script: map key of type %s", rv.Type().Key())
		}
		keys := make([]string, 0, rv.Len())
		for _, k := range rv.MapKeys() {
			keys = append(keys, k.String())
		}
		sort.Strings(keys)
		b.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				b.WriteString(", ")
			}
			pyJSONString(b, k)
			b.WriteString(": ")
			if err := pyJSON(b, rv.MapIndex(reflect.ValueOf(k).Convert(rv.Type().Key())).Interface()); err != nil {
				return err
			}
		}
		b.WriteByte('}')
		return nil
	}
	// Numbers, booleans and types with their own encoding.
	raw, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("json_script: %w", err)
	}
	b.Write(raw)
	return nil
}

// QuoteJSON returns s as a JSON string the way Python's json.dumps writes it:
// in ASCII, with every non-ASCII character as a \u escape. Code that builds
// the JSON of a Django view by hand uses it for the strings.
func QuoteJSON(s string) string {
	var b strings.Builder
	pyJSONString(&b, s)
	return b.String()
}

// pyJSONString writes s as a JSON string in ASCII.
func pyJSONString(b *strings.Builder, s string) {
	b.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '"':
			b.WriteString(`\"`)
		case r == '\\':
			b.WriteString(`\\`)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\r':
			b.WriteString(`\r`)
		case r == '\t':
			b.WriteString(`\t`)
		case r == '\b':
			b.WriteString(`\b`)
		case r == '\f':
			b.WriteString(`\f`)
		case r < 0x20 || (r >= 0x7f && r <= 0xffff):
			fmt.Fprintf(b, `\u%04x`, r)
		case r > 0xffff:
			r -= 0x10000
			fmt.Fprintf(b, `\u%04x\u%04x`, 0xd800+(r>>10), 0xdc00+(r&0x3ff))
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
}

// jsonEscape returns the JSON escape of r in the form Django writes it, with
// upper case hex digits.
func jsonEscape(r rune) string { return fmt.Sprintf(`\u%04X`, r) }

// jsonScriptEscaper keeps JSON from ending the script element or starting
// markup inside it.
var jsonScriptEscaper = strings.NewReplacer(
	"<", jsonEscape('<'), ">", jsonEscape('>'), "&", jsonEscape('&'),
)

// filterJSONScript implements Django's json_script: the value as JSON inside
// a <script type="application/json"> element, with the argument as its id.
// The characters that could end the element are escaped, so the data may come
// from users.
func filterJSONScript(in, param *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	var b strings.Builder
	if err := pyJSON(&b, in.Interface()); err != nil {
		return nil, &pongo2.Error{Sender: "filter:json_script", OrigError: err}
	}
	id := ""
	if !param.IsNil() && param.String() != "" {
		id = ` id="` + escapeHTML(param.String()) + `"`
	}
	return safe(`<script` + id + ` type="application/json">` + jsonScriptEscaper.Replace(b.String()) + `</script>`)
}

// lookupKey resolves a dictsort key ("name", "author.name", "0") on an
// element. ok is false when the element has no such key.
func lookupKey(v reflect.Value, key string) (value any, ok bool) {
	for _, part := range strings.Split(key, ".") {
		for v.IsValid() && (v.Kind() == reflect.Interface || v.Kind() == reflect.Pointer) {
			if v.IsNil() {
				return nil, false
			}
			v = v.Elem()
		}
		if !v.IsValid() {
			return nil, false
		}
		switch v.Kind() {
		case reflect.Map:
			if v.Type().Key().Kind() != reflect.String {
				return nil, false
			}
			v = v.MapIndex(reflect.ValueOf(part).Convert(v.Type().Key()))
		case reflect.Slice, reflect.Array:
			i, err := strconv.Atoi(part)
			if err != nil || i < 0 || i >= v.Len() {
				return nil, false
			}
			v = v.Index(i)
		case reflect.Struct:
			v = v.FieldByName(part)
		default:
			return nil, false
		}
	}
	for v.IsValid() && v.Kind() == reflect.Interface && !v.IsNil() {
		v = v.Elem()
	}
	if !v.IsValid() || !v.CanInterface() {
		return nil, false
	}
	return v.Interface(), true
}

// sortKind is the kind of a dictsort key that Python can order against
// another key of the same kind.
func sortKind(v any) string {
	if v == nil {
		return "none"
	}
	pv := pongo2.AsValue(v)
	switch {
	case pv.IsNumber() || pv.IsBool():
		return "number"
	case pv.IsString():
		return "string"
	}
	if _, ok := asTime(pv); ok {
		return "time"
	}
	return "other"
}

// compareValues orders two sort keys: numbers by value, times by instant,
// everything else by its text. nil sorts first.
func compareValues(a, b any) int {
	if a == nil || b == nil {
		switch {
		case a == nil && b == nil:
			return 0
		case a == nil:
			return -1
		}
		return 1
	}
	va, vb := pongo2.AsValue(a), pongo2.AsValue(b)
	if va.IsNumber() && vb.IsNumber() {
		fa, fb := va.Float(), vb.Float()
		switch {
		case fa < fb:
			return -1
		case fa > fb:
			return 1
		}
		return 0
	}
	if ta, ok := asTime(va); ok {
		if tb, ok := asTime(vb); ok {
			return ta.Compare(tb)
		}
	}
	return strings.Compare(text(va), text(vb))
}

// dictsortFilter returns Django's dictsort or dictsortreversed: a list of
// maps (or of lists) ordered by the given key. The sort is stable. Like
// Django it gives an empty string for a key that starts with "_" (or has a
// part that does), for an element without the key, and for keys Python
// cannot order against each other.
func dictsortFilter(reversed bool) pongo2.FilterFunction {
	return func(in, param *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
		rv := reflect.ValueOf(in.Interface())
		key := param.String()
		if !isList(rv) || strings.HasPrefix(key, "_") || strings.Contains(key, "._") {
			return str("")
		}
		type keyed struct {
			key  any
			item any
		}
		rows := make([]keyed, rv.Len())
		kinds := map[string]bool{}
		for i := range rows {
			k, ok := lookupKey(rv.Index(i), key)
			if !ok {
				return str("")
			}
			kinds[sortKind(k)] = true
			rows[i] = keyed{key: k, item: rv.Index(i).Interface()}
		}
		if len(rows) > 1 && (len(kinds) > 1 || kinds["none"]) {
			return str("")
		}
		slices.SortStableFunc(rows, func(a, b keyed) int {
			if reversed {
				return compareValues(b.key, a.key)
			}
			return compareValues(a.key, b.key)
		})
		out := make([]any, len(rows))
		for i, row := range rows {
			out[i] = row.item
		}
		return pongo2.AsValue(out), nil
	}
}
