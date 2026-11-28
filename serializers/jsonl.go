package serializers

import (
	"bufio"
	"io"
	"iter"
	"strings"

	"github.com/oliverhaas/djangogo/internal/pyjson"
)

// JSONLFormat implements Format for Django's "jsonl" serializer, porting
// django.core.serializers.jsonl.
type JSONLFormat struct{}

// Serialize writes objs to w as JSON Lines matching Django's jsonl.Serializer
// byte for byte. It ignores Options.Indent and writes each object with
// separators (",", ": ") followed by "\n".
func (JSONLFormat) Serialize(w io.Writer, objs iter.Seq2[*Object, error], _ *Options) error {
	pyOpts := pyjson.Options{
		Indent:      -1,
		ItemSep:     ",",
		KeySep:      ": ",
		EnsureASCII: false,
		Default:     DjangoJSONDefault,
	}

	var objBuf strings.Builder
	for obj, err := range objs {
		if err != nil {
			return err
		}
		objBuf.Reset()
		if err := pyjson.Encode(&objBuf, dumpObject(obj), pyOpts); err != nil {
			return err
		}
		objBuf.WriteByte('\n')
		if _, err := io.WriteString(w, objBuf.String()); err != nil {
			return err
		}
	}
	return nil
}

// Deserialize reads JSON Lines from r, skipping blank lines, and yields each
// *Object. A JSON syntax error yields a DeserializationError with an empty
// message; structural errors yield "Error deserializing object: ...".
func (JSONLFormat) Deserialize(r io.Reader) iter.Seq2[*Object, error] {
	return func(yield func(*Object, error) bool) {
		if r == nil {
			yield(nil, NewDeserializationError(""))
			return
		}
		br := bufio.NewReader(r)
		for {
			line, readErr := br.ReadString('\n')
			if readErr != nil && readErr != io.EOF {
				yield(nil, NewDeserializationError(""))
				return
			}
			trimmed := strings.TrimSpace(line)
			if trimmed != "" {
				decoded, err := pyjson.Decode([]byte(trimmed))
				if err != nil {
					yield(nil, NewDeserializationError(""))
					return
				}
				obj, err := objectFromJSON(decoded)
				if err != nil {
					yield(nil, err)
					return
				}
				if !yield(obj, nil) {
					return
				}
			}
			if readErr == io.EOF {
				return
			}
		}
	}
}
