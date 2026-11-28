package serializers

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"iter"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/oliverhaas/djangogo/internal/duration"
	"github.com/oliverhaas/djangogo/internal/pyjson"
	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/validators"
)

// DTDForbiddenError reports a forbidden DOCTYPE declaration in an XML stream,
// porting Django's django.core.serializers.xml_serializer.DTDForbidden.
type DTDForbiddenError struct {
	// Name is the declared document type name.
	Name string
	// SystemID is the SYSTEM identifier, or "" when absent.
	SystemID string
	// PublicID is the PUBLIC identifier, or "" when absent.
	PublicID string
	// HasSystemID reports whether a SYSTEM identifier was present.
	HasSystemID bool
	// HasPublicID reports whether a PUBLIC identifier was present.
	HasPublicID bool
}

// Error formats the error as Django's DTDForbidden.__str__:
// "DTDForbidden(name='...', system_id=..., public_id=...)".
func (e *DTDForbiddenError) Error() string {
	sysRepr := "None"
	if e.HasSystemID {
		sysRepr = validators.PyRepr(e.SystemID)
	}
	pubRepr := "None"
	if e.HasPublicID {
		pubRepr = validators.PyRepr(e.PublicID)
	}
	return fmt.Sprintf("DTDForbidden(name='%s', system_id=%s, public_id=%s)", e.Name, sysRepr, pubRepr)
}

// XMLFormat implements Format for Django's "xml" serializer, porting
// django.core.serializers.xml_serializer.
type XMLFormat struct{}

// xmlAttr is a single XML attribute key-value pair.
type xmlAttr struct {
	name  string
	value string
}

// simplerXMLGenerator writes XML matching Python's xml.sax.saxutils.XMLGenerator
// and Django's SimplerXMLGenerator.
type simplerXMLGenerator struct {
	b strings.Builder
}

// startDocument writes the XML 1.0 declaration with a trailing newline.
func (g *simplerXMLGenerator) startDocument() {
	g.b.WriteString(`<?xml version="1.0" encoding="utf-8"?>` + "\n")
}

// startElement writes an opening tag with attributes sorted by name and quoted
// with Python's xml.sax.saxutils.quoteattr rules.
func (g *simplerXMLGenerator) startElement(name string, attrs []xmlAttr) {
	if len(attrs) > 1 {
		sorted := make([]xmlAttr, len(attrs))
		copy(sorted, attrs)
		slices.SortFunc(sorted, func(a, b xmlAttr) int {
			return strings.Compare(a.name, b.name)
		})
		attrs = sorted
	}
	g.b.WriteByte('<')
	g.b.WriteString(name)
	for _, a := range attrs {
		g.b.WriteByte(' ')
		g.b.WriteString(a.name)
		g.b.WriteByte('=')
		g.b.WriteString(xmlQuoteAttr(a.value))
	}
	g.b.WriteByte('>')
}

// endElement writes a closing tag.
func (g *simplerXMLGenerator) endElement(name string) {
	g.b.WriteString("</")
	g.b.WriteString(name)
	g.b.WriteByte('>')
}

// addQuickElement writes an element with optional text contents and no child
// elements.
func (g *simplerXMLGenerator) addQuickElement(name string, contents *string, attrs []xmlAttr) error {
	g.startElement(name, attrs)
	if contents != nil {
		if err := g.characters(*contents); err != nil {
			return err
		}
	}
	g.endElement(name)
	return nil
}

// ignorableWhitespace writes raw indentation whitespace without escaping.
func (g *simplerXMLGenerator) ignorableWhitespace(s string) {
	g.b.WriteString(s)
}

// characters checks content for XML 1.0 disallowed control characters and
// writes the escaped character data.
func (g *simplerXMLGenerator) characters(content string) error {
	if content == "" {
		return nil
	}
	if hasXMLControlChar(content) {
		return errUnserializableContent
	}
	writeXMLEscapedText(&g.b, content)
	return nil
}

// errUnserializableContent signals that a string contains control characters
// forbidden in XML 1.0, mirroring Django's UnserializableContentError.
var errUnserializableContent = errors.New("Control characters are not supported in XML 1.0") //nolint:staticcheck // Django's message

// hasXMLControlChar reports whether s contains a rune in
// [\x00-\x08\x0B-\x0C\x0E-\x1F].
func hasXMLControlChar(s string) bool {
	for _, r := range s {
		if r < 0x20 && r != '\t' && r != '\n' && r != '\r' {
			return true
		}
	}
	return false
}

// writeXMLEscapedText writes s to b escaping &, <, and > as Python's
// xml.sax.saxutils.escape does.
func writeXMLEscapedText(b *strings.Builder, s string) {
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		default:
			b.WriteByte(s[i])
		}
	}
}

// xmlQuoteAttr escapes and quotes an attribute value matching Python's
// xml.sax.saxutils.quoteattr(data).
func xmlQuoteAttr(data string) string {
	hasDouble := strings.ContainsRune(data, '"')
	hasSingle := strings.ContainsRune(data, '\'')
	escapeDouble := hasDouble && hasSingle
	quote := byte('"')
	if hasDouble && !hasSingle {
		quote = '\''
	}

	var b strings.Builder
	b.Grow(len(data) + 2)
	b.WriteByte(quote)
	for i := 0; i < len(data); i++ {
		switch data[i] {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		case '\n':
			b.WriteString("&#10;")
		case '\r':
			b.WriteString("&#13;")
		case '\t':
			b.WriteString("&#9;")
		case '"':
			if escapeDouble {
				b.WriteString("&quot;")
			} else {
				b.WriteByte('"')
			}
		default:
			b.WriteByte(data[i])
		}
	}
	b.WriteByte(quote)
	return b.String()
}

// Serialize writes objs to w as an XML document matching Django's
// xml_serializer.Serializer byte for byte.
func (XMLFormat) Serialize(w io.Writer, objs iter.Seq2[*Object, error], o *Options) error {
	if o == nil {
		o = NewOptions()
	}
	var reg *orm.Registry
	if o.DB != nil {
		reg = o.DB.Registry()
	}

	var gen simplerXMLGenerator
	indent := func(level int) {
		if o.Indent >= 0 {
			gen.ignorableWhitespace("\n" + strings.Repeat(" ", o.Indent*level))
		}
	}

	gen.startDocument()
	gen.startElement("django-objects", []xmlAttr{{name: "version", value: "1.0"}})

	for obj, err := range objs {
		if err != nil {
			return err
		}
		var m *orm.Model
		if reg != nil {
			m, _ = lookupModel(reg, obj.Model)
		}

		indent(1)
		objAttrs := []xmlAttr{{name: "model", value: obj.Model}}
		if obj.HasPK && obj.PK != nil {
			var pkField *orm.Field
			if m != nil {
				pkField = m.PrimaryKey()
			}
			pkStr, err := xmlFieldValueToString(pkField, obj.PK)
			if err != nil {
				return err
			}
			objAttrs = append(objAttrs, xmlAttr{name: "pk", value: pkStr})
		}
		gen.startElement("object", objAttrs)

		for _, fv := range obj.Fields {
			if err := writeXMLField(&gen, indent, m, obj, fv); err != nil {
				return err
			}
		}

		indent(1)
		gen.endElement("object")
	}

	indent(0)
	gen.endElement("django-objects")
	_, err := io.WriteString(w, gen.b.String())
	return err
}

// writeXMLField writes a single <field> element for fv onto gen.
func writeXMLField(
	gen *simplerXMLGenerator,
	indent func(int),
	m *orm.Model,
	obj *Object,
	fv FieldValue,
) error {
	if m != nil {
		if m2mField, ok := m.ManyToManyField(fv.Name); ok {
			return writeXMLM2MField(gen, indent, m2mField, fv)
		}
		if field, ok := m.FieldByAttr(fv.Name); ok {
			if field.Rel != nil && (field.Rel.Kind == orm.RelFK || field.Rel.Kind == orm.RelO2O) {
				return writeXMLFKField(gen, indent, field, fv)
			}
			return writeXMLScalarField(gen, indent, m, obj, field, fv)
		}
	}
	return writeXMLScalarField(gen, indent, m, obj, nil, fv)
}

// writeXMLScalarField writes a non-relational <field> element.
func writeXMLScalarField(
	gen *simplerXMLGenerator,
	indent func(int),
	m *orm.Model,
	obj *Object,
	field *orm.Field,
	fv FieldValue,
) error {
	internalType := "CharField"
	if field != nil {
		internalType = field.InternalType()
	}
	indent(2)
	gen.startElement("field", []xmlAttr{
		{name: "name", value: fv.Name},
		{name: "type", value: internalType},
	})
	if fv.Value != nil {
		valStr, err := xmlFieldValueToString(field, fv.Value)
		if err != nil {
			return err
		}
		if err := gen.characters(valStr); err != nil {
			modelName := obj.Model
			if m != nil {
				modelName = m.Name()
			} else if _, after, ok := strings.Cut(obj.Model, "."); ok {
				modelName = after
			}
			return &SerializationError{
				Message: fmt.Sprintf(
					"%s.%s (pk:%s) contains unserializable characters",
					modelName,
					fv.Name,
					validators.PyString(obj.PK),
				),
				Err: err,
			}
		}
	} else {
		_ = gen.addQuickElement("None", nil, nil)
	}
	gen.endElement("field")
	return nil
}

// writeXMLFKField writes a ForeignKey or OneToOneField <field> element.
func writeXMLFKField(
	gen *simplerXMLGenerator,
	indent func(int),
	field *orm.Field,
	fv FieldValue,
) error {
	relName := "ManyToOneRel"
	if field.Rel.Kind == orm.RelO2O {
		relName = "OneToOneRel"
	}
	toLabel := ""
	if field.Rel.Target != nil {
		toLabel = field.Rel.Target.LabelLower()
	}
	indent(2)
	gen.startElement("field", []xmlAttr{
		{name: "name", value: fv.Name},
		{name: "rel", value: relName},
		{name: "to", value: toLabel},
	})
	if fv.Value != nil {
		if natKey, isList := asIterableSlice(fv.Value); isList {
			for _, keyVal := range natKey {
				gen.startElement("natural", nil)
				if err := gen.characters(validators.PyString(keyVal)); err != nil {
					return err
				}
				gen.endElement("natural")
			}
		} else {
			if err := gen.characters(validators.PyString(fv.Value)); err != nil {
				return err
			}
		}
	} else {
		_ = gen.addQuickElement("None", nil, nil)
	}
	gen.endElement("field")
	return nil
}

// writeXMLM2MField writes a ManyToManyField <field> element.
func writeXMLM2MField(
	gen *simplerXMLGenerator,
	indent func(int),
	field *orm.Field,
	fv FieldValue,
) error {
	toLabel := ""
	if field.Rel != nil && field.Rel.Target != nil {
		toLabel = field.Rel.Target.LabelLower()
	}
	indent(2)
	gen.startElement("field", []xmlAttr{
		{name: "name", value: fv.Name},
		{name: "rel", value: "ManyToManyRel"},
		{name: "to", value: toLabel},
	})
	if items, ok := asIterableSlice(fv.Value); ok {
		for _, item := range items {
			if natKey, isList := asIterableSlice(item); isList {
				gen.startElement("object", nil)
				for _, keyVal := range natKey {
					gen.startElement("natural", nil)
					if err := gen.characters(validators.PyString(keyVal)); err != nil {
						return err
					}
					gen.endElement("natural")
				}
				gen.endElement("object")
			} else {
				_ = gen.addQuickElement("object", nil, []xmlAttr{
					{name: "pk", value: validators.PyString(item)},
				})
			}
		}
	}
	gen.endElement("field")
	return nil
}

// xmlFieldValueToString formats v as Django's field.value_to_string(obj) (plus
// json.dumps for JSONField).
func xmlFieldValueToString(field *orm.Field, v any) (string, error) {
	if field != nil && field.Kind == orm.KindJSON {
		return pyjson.Marshal(v, pyjson.Options{
			Indent:      -1,
			EnsureASCII: true,
			Default:     DjangoJSONDefault,
		})
	}
	switch x := v.(type) {
	case string:
		return x, nil
	case time.Time:
		if field != nil {
			switch field.Kind {
			case orm.KindDate:
				return orm.DateFromTime(x).String(), nil
			case orm.KindTime:
				return orm.TimeFromTime(x).String(), nil
			}
		}
		return formatDateTimeISO(x), nil
	case orm.Date:
		return x.String(), nil
	case orm.Time:
		return x.String(), nil
	case time.Duration:
		return duration.String(x), nil
	case orm.Decimal:
		return x.String(), nil
	case orm.UUID:
		return x.String(), nil
	case float64:
		return pyjson.FormatFloat(x), nil
	case float32:
		return pyjson.FormatFloat(float64(x)), nil
	default:
		return validators.PyString(v), nil
	}
}

// formatDateTimeISO formats t as Python's datetime.isoformat():
// "YYYY-MM-DDTHH:MM:SS[.ffffff][+HH:MM]".
func formatDateTimeISO(t time.Time) string {
	base := fmt.Sprintf("%04d-%02d-%02dT%02d:%02d:%02d",
		t.Year(), int(t.Month()), t.Day(), t.Hour(), t.Minute(), t.Second())
	micro := (t.Nanosecond() / 1000) % 1_000_000
	if micro != 0 {
		base += fmt.Sprintf(".%06d", micro)
	}
	return base + t.Format("-07:00")
}

// Deserialize reads an XML document of serialized objects from r and yields
// each *Object.
func (XMLFormat) Deserialize(r io.Reader, o *Options) iter.Seq2[*Object, error] {
	return func(yield func(*Object, error) bool) {
		if r == nil {
			yield(nil, NewDeserializationError(""))
			return
		}
		if o == nil {
			o = NewOptions()
		}
		dec := xml.NewDecoder(r)
		dec.Strict = true

		for {
			tok, err := dec.Token()
			if err != nil {
				if errors.Is(err, io.EOF) {
					return
				}
				yield(nil, &DeserializationError{Message: err.Error(), Err: err})
				return
			}
			switch t := tok.(type) {
			case xml.Directive:
				if dtdErr := checkDOCTYPE(t); dtdErr != nil {
					yield(nil, dtdErr)
					return
				}
			case xml.StartElement:
				if t.Name.Local == "object" {
					node, err := readXMLSubtree(dec, t)
					if err != nil {
						var dtdErr *DTDForbiddenError
						if errors.As(err, &dtdErr) {
							yield(nil, dtdErr)
						} else {
							yield(nil, &DeserializationError{Message: err.Error(), Err: err})
						}
						return
					}
					obj, err := handleXMLObjectNode(node, o)
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
	}
}

// xmlNode represents an expanded XML element or character data node.
type xmlNode struct {
	name     string
	attrs    []xml.Attr
	text     string
	isText   bool
	children []*xmlNode
}

// attr returns the value of the named attribute on n, or "" when absent.
func (n *xmlNode) attr(name string) string {
	val, _ := n.lookupAttr(name)
	return val
}

// lookupAttr returns the value of the named attribute on n and whether it is
// present.
func (n *xmlNode) lookupAttr(name string) (string, bool) {
	for _, a := range n.attrs {
		if a.Name.Local == name && a.Name.Space == "" {
			return a.Value, true
		}
	}
	return "", false
}

// elementsByTagName returns all descendant element nodes of n with Local name
// tagName in document order, matching DOM's Element.getElementsByTagName.
func (n *xmlNode) elementsByTagName(tagName string) []*xmlNode {
	var out []*xmlNode
	collectElementsByTagName(n, tagName, &out)
	return out
}

// collectElementsByTagName appends descendant elements matching tagName to out.
func collectElementsByTagName(n *xmlNode, tagName string, out *[]*xmlNode) {
	for _, ch := range n.children {
		if ch.isText {
			continue
		}
		if ch.name == tagName {
			*out = append(*out, ch)
		}
		collectElementsByTagName(ch, tagName, out)
	}
}

// readXMLSubtree reads tokens from dec until the matching EndElement for start
// and returns the constructed *xmlNode tree.
func readXMLSubtree(dec *xml.Decoder, start xml.StartElement) (*xmlNode, error) {
	root := &xmlNode{
		name:  start.Name.Local,
		attrs: slices.Clone(start.Attr),
	}
	stack := []*xmlNode{root}
	for len(stack) > 0 {
		tok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		cur := stack[len(stack)-1]
		switch t := tok.(type) {
		case xml.Directive:
			if dtdErr := checkDOCTYPE(t); dtdErr != nil {
				return nil, dtdErr
			}
		case xml.StartElement:
			child := &xmlNode{
				name:  t.Name.Local,
				attrs: slices.Clone(t.Attr),
			}
			cur.children = append(cur.children, child)
			stack = append(stack, child)
		case xml.EndElement:
			stack = stack[:len(stack)-1]
		case xml.CharData:
			if len(t) > 0 {
				cur.children = append(cur.children, &xmlNode{
					isText: true,
					text:   string(t),
				})
			}
		}
	}
	return root, nil
}

// getInnerText concatenates all text within n recursively in linear time,
// porting Django's xml_serializer.getInnerText.
func getInnerText(n *xmlNode) string {
	var b strings.Builder
	writeInnerText(&b, n)
	return b.String()
}

// writeInnerText appends character data from n and its descendants to b.
func writeInnerText(b *strings.Builder, n *xmlNode) {
	for _, ch := range n.children {
		if ch.isText {
			b.WriteString(ch.text)
		} else {
			writeInnerText(b, ch)
		}
	}
}

// pyStrip trims leading and trailing Unicode whitespace like Python's
// str.strip().
func pyStrip(s string) string {
	return strings.TrimFunc(s, unicode.IsSpace)
}

// handleXMLObjectNode converts an expanded <object> xmlNode into an *Object,
// porting Django's Deserializer._handle_object.
func handleXMLObjectNode(node *xmlNode, o *Options) (*Object, error) {
	modelIdentifier := node.attr("model")
	if modelIdentifier == "" {
		return nil, NewDeserializationError("<object> node is missing the required 'model' attribute")
	}
	if o.DB == nil || o.DB.Registry() == nil {
		return nil, NewDeserializationError(fmt.Sprintf("<object> node has invalid model identifier: '%s'", modelIdentifier))
	}
	m, err := lookupModel(o.DB.Registry(), modelIdentifier)
	if err != nil {
		return nil, NewDeserializationError(fmt.Sprintf("<object> node has invalid model identifier: '%s'", modelIdentifier))
	}

	out := &Object{
		Model: m.LabelLower(),
	}
	if pkAttr, hasPK := node.lookupAttr("pk"); hasPK {
		out.HasPK = true
		out.PK = pkAttr
	}

	fieldNodes := node.elementsByTagName("field")
	out.Fields = make([]FieldValue, 0, len(fieldNodes))

	for _, fieldNode := range fieldNodes {
		fieldName := fieldNode.attr("name")
		if fieldName == "" {
			return nil, NewDeserializationError("<field> node is missing the 'name' attribute")
		}

		if m2mField, ok := m.ManyToManyField(fieldName); ok {
			val := handleXMLM2MFieldNode(fieldNode, m2mField)
			out.Fields = append(out.Fields, FieldValue{Name: fieldName, Value: val})
			continue
		}

		field, ok := m.FieldByAttr(fieldName)
		if !ok {
			if o.IgnoreNonexistent {
				continue
			}
			return nil, NewDeserializationError(
				fmt.Sprintf("Error deserializing object: %s has no field named '%s'", m.Name(), fieldName),
			)
		}

		if field.Rel != nil && (field.Rel.Kind == orm.RelFK || field.Rel.Kind == orm.RelO2O) {
			val := handleXMLFKFieldNode(fieldNode, field)
			out.Fields = append(out.Fields, FieldValue{Name: fieldName, Value: val})
			continue
		}

		var val any
		if len(fieldNode.elementsByTagName("None")) > 0 {
			val = nil
		} else {
			text := pyStrip(getInnerText(fieldNode))
			if field.InternalType() == "JSONField" {
				decoded, err := pyjson.Decode([]byte(text))
				if err != nil {
					return nil, DeserializationErrorWithData(err, out.Model, out.PK, text)
				}
				val = decoded
			} else {
				val = text
			}
		}
		out.Fields = append(out.Fields, FieldValue{Name: fieldName, Value: val})
	}

	return out, nil
}

// handleXMLFKFieldNode extracts the raw FK or O2O value from a <field> node:
// nil for <None>, a []any slice of strings for <natural> children on a model
// with GetByNaturalKey, or the trimmed inner text string.
func handleXMLFKFieldNode(fieldNode *xmlNode, field *orm.Field) any {
	if len(fieldNode.elementsByTagName("None")) > 0 {
		return nil
	}
	if field.Rel != nil && modelHasNaturalKeyGetter(field.Rel.Target) {
		keys := fieldNode.elementsByTagName("natural")
		if len(keys) > 0 {
			natKey := make([]any, len(keys))
			for i, k := range keys {
				natKey[i] = pyStrip(getInnerText(k))
			}
			return natKey
		}
	}
	return pyStrip(getInnerText(fieldNode))
}

// handleXMLM2MFieldNode extracts the raw M2M list from a <field> node: a slice
// of pk attribute strings or []any natural-key string slices.
func handleXMLM2MFieldNode(fieldNode *xmlNode, field *orm.Field) []any {
	hasNatGetter := field.Rel != nil && modelHasNaturalKeyGetter(field.Rel.Target)
	objNodes := fieldNode.elementsByTagName("object")
	values := make([]any, 0, len(objNodes))
	for _, objNode := range objNodes {
		if hasNatGetter {
			keys := objNode.elementsByTagName("natural")
			if len(keys) > 0 {
				natKey := make([]any, len(keys))
				for i, k := range keys {
					natKey[i] = pyStrip(getInnerText(k))
				}
				values = append(values, natKey)
				continue
			}
		}
		values = append(values, objNode.attr("pk"))
	}
	return values
}

// checkDOCTYPE inspects an xml.Directive and returns a *DTDForbiddenError when
// it is a DOCTYPE declaration.
func checkDOCTYPE(dir xml.Directive) *DTDForbiddenError {
	s := strings.TrimSpace(string(dir))
	if len(s) < len("DOCTYPE") || !strings.EqualFold(s[:len("DOCTYPE")], "DOCTYPE") {
		return nil
	}
	rest := strings.TrimSpace(s[len("DOCTYPE"):])
	name, rest := nextDOCTYPEToken(rest)
	dtdErr := &DTDForbiddenError{Name: name}

	kw, rest := nextDOCTYPEToken(rest)
	switch {
	case strings.EqualFold(kw, "SYSTEM"):
		sysID, ok, _ := nextDOCTYPEQuoted(rest)
		if ok {
			dtdErr.SystemID = sysID
			dtdErr.HasSystemID = true
		}
	case strings.EqualFold(kw, "PUBLIC"):
		pubID, ok, afterPub := nextDOCTYPEQuoted(rest)
		if ok {
			dtdErr.PublicID = pubID
			dtdErr.HasPublicID = true
		}
		sysID, ok, _ := nextDOCTYPEQuoted(afterPub)
		if ok {
			dtdErr.SystemID = sysID
			dtdErr.HasSystemID = true
		}
	}
	return dtdErr
}

// nextDOCTYPEToken reads the next whitespace-delimited non-bracket token from s.
func nextDOCTYPEToken(s string) (token, rest string) {
	s = strings.TrimSpace(s)
	if s == "" || s[0] == '[' {
		return "", s
	}
	i := 0
	for i < len(s) && !unicode.IsSpace(rune(s[i])) && s[i] != '[' {
		i++
	}
	return s[:i], strings.TrimSpace(s[i:])
}

// nextDOCTYPEQuoted reads a single- or double-quoted literal at the start of s.
func nextDOCTYPEQuoted(s string) (val string, ok bool, rest string) {
	s = strings.TrimSpace(s)
	if len(s) < 2 || (s[0] != '"' && s[0] != '\'') {
		return "", false, s
	}
	quote := s[0]
	end := strings.IndexByte(s[1:], quote)
	if end < 0 {
		return s[1:], true, ""
	}
	return s[1 : 1+end], true, strings.TrimSpace(s[2+end:])
}
