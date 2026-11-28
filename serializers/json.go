package serializers

import (
	"fmt"
	"io"
	"iter"
	"strings"
	"time"

	"github.com/oliverhaas/djangogo/internal/duration"
	"github.com/oliverhaas/djangogo/internal/pyjson"
	"github.com/oliverhaas/djangogo/orm"
)

// JSONFormat implements Format for Django's "json" serializer, porting
// django.core.serializers.json.
type JSONFormat struct{}

// Serialize writes objs to w as a JSON array matching Django's json.Serializer
// byte for byte.
func (JSONFormat) Serialize(w io.Writer, objs iter.Seq2[*Object, error], o *Options) error {
	if o == nil {
		o = NewOptions()
	}
	hasIndent := o.Indent > 0
	pyOpts := pyjson.Options{
		Indent:      o.Indent,
		KeySep:      ": ",
		EnsureASCII: false,
		Default:     DjangoJSONDefault,
	}

	if _, err := io.WriteString(w, "["); err != nil {
		return err
	}
	first := true
	var objBuf strings.Builder
	for obj, err := range objs {
		if err != nil {
			return err
		}
		if !first {
			if _, err := io.WriteString(w, ","); err != nil {
				return err
			}
			if !hasIndent {
				if _, err := io.WriteString(w, " "); err != nil {
					return err
				}
			}
		}
		if hasIndent {
			if _, err := io.WriteString(w, "\n"); err != nil {
				return err
			}
		}
		objBuf.Reset()
		if err := pyjson.Encode(&objBuf, dumpObject(obj), pyOpts); err != nil {
			return err
		}
		if _, err := io.WriteString(w, objBuf.String()); err != nil {
			return err
		}
		first = false
	}
	if hasIndent {
		if _, err := io.WriteString(w, "\n"); err != nil {
			return err
		}
	}
	_, err := io.WriteString(w, "]\n")
	return err
}

// dumpObject converts obj into an ordered pyjson.Object with keys "model",
// optional "pk", and "fields".
func dumpObject(obj *Object) pyjson.Object {
	fields := make(pyjson.Object, len(obj.Fields))
	for i, fv := range obj.Fields {
		fields[i] = pyjson.Member{Key: fv.Name, Value: fv.Value}
	}
	out := make(pyjson.Object, 0, 3)
	out = append(out, pyjson.Member{Key: "model", Value: obj.Model})
	if obj.HasPK {
		out = append(out, pyjson.Member{Key: "pk", Value: obj.PK})
	}
	out = append(out, pyjson.Member{Key: "fields", Value: fields})
	return out
}

// Deserialize reads a JSON array of serialized objects from r and yields each
// *Object. A JSON syntax error yields a DeserializationError with an empty
// message; structural errors yield "Error deserializing object: ...".
func (JSONFormat) Deserialize(r io.Reader) iter.Seq2[*Object, error] {
	return func(yield func(*Object, error) bool) {
		if r == nil {
			yield(nil, NewDeserializationError(""))
			return
		}
		data, err := io.ReadAll(r)
		if err != nil {
			yield(nil, NewDeserializationError(""))
			return
		}
		decoded, err := pyjson.Decode(data)
		if err != nil {
			yield(nil, NewDeserializationError(""))
			return
		}
		list, ok := decoded.([]any)
		if !ok {
			yield(nil, NewDeserializationError("Error deserializing object: expected a list of objects"))
			return
		}
		for _, item := range list {
			obj, err := objectFromJSON(item)
			if err != nil {
				yield(nil, err)
				return
			}
			if !yield(obj, nil) {
				return
			}
		}
	}
}

// objectFromJSON converts one decoded JSON value into an *Object for the json
// and jsonl formats.
func objectFromJSON(item any) (*Object, error) {
	rawObj, ok := item.(pyjson.Object)
	if !ok {
		return nil, NewDeserializationError("Error deserializing object: expected an object")
	}
	modelRaw, ok := rawObj.Get("model")
	if !ok {
		return nil, NewDeserializationError("Error deserializing object: 'model'")
	}
	modelStr, ok := modelRaw.(string)
	if !ok {
		return nil, NewDeserializationError(fmt.Sprintf("Invalid model identifier: '%v'", modelRaw))
	}
	fieldsRaw, ok := rawObj.Get("fields")
	if !ok {
		return nil, NewDeserializationError("Error deserializing object: 'fields'")
	}
	fieldsObj, ok := fieldsRaw.(pyjson.Object)
	if !ok {
		return nil, NewDeserializationError("Error deserializing object: 'fields' must be an object")
	}
	pkVal, hasPK := rawObj.Get("pk")
	fields := make([]FieldValue, len(fieldsObj))
	for i, m := range fieldsObj {
		fields[i] = FieldValue{Name: m.Key, Value: m.Value}
	}
	return &Object{Model: modelStr, PK: pkVal, HasPK: hasPK, Fields: fields}, nil
}

// DjangoJSONDefault converts temporal, duration, Decimal, and UUID values for
// pyjson.Encode, porting Django's DjangoJSONEncoder.default.
func DjangoJSONDefault(v any) (any, error) {
	switch x := v.(type) {
	case time.Time:
		return formatDjangoJSONDateTime(x), nil
	case orm.Date:
		return x.String(), nil
	case orm.Time:
		return formatDjangoJSONTime(x), nil
	case time.Duration:
		return duration.ISOString(x), nil
	case orm.Decimal:
		return x.String(), nil
	case orm.UUID:
		return x.String(), nil
	default:
		const notSerializableFmt = "Object of type %T is not JSON serializable"
		return nil, fmt.Errorf(notSerializableFmt, v)
	}
}

// formatDjangoJSONDateTime formats t as ECMA-262 ISO-8601 matching
// DjangoJSONEncoder.default: microseconds are truncated to 3-digit
// milliseconds when non-zero, and a UTC "+00:00" offset becomes "Z".
func formatDjangoJSONDateTime(t time.Time) string {
	base := fmt.Sprintf("%04d-%02d-%02dT%02d:%02d:%02d",
		t.Year(), int(t.Month()), t.Day(), t.Hour(), t.Minute(), t.Second())
	micro := (t.Nanosecond() / 1000) % 1_000_000
	if micro != 0 {
		base += fmt.Sprintf(".%03d", micro/1000)
	}
	_, offset := t.Zone()
	if offset == 0 {
		return base + "Z"
	}
	return base + t.Format("-07:00")
}

// formatDjangoJSONTime formats tm as ISO-8601 matching
// DjangoJSONEncoder.default: microseconds are truncated to 3-digit
// milliseconds when non-zero.
func formatDjangoJSONTime(tm orm.Time) string {
	base := fmt.Sprintf("%02d:%02d:%02d", tm.Hour, tm.Minute, tm.Second)
	micro := (tm.Nanosecond / 1000) % 1_000_000
	if micro != 0 {
		return fmt.Sprintf("%s.%03d", base, micro/1000)
	}
	return base
}
