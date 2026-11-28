package orm

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/oliverhaas/djangogo/internal/pyjson"
)

// The lookups of a JSON value that other values lack, Django's ContainedBy,
// HasKey, HasKeys and HasAnyKeys. "contains" of a JSON value is Django's
// DataContains, a test of containment instead of a text pattern.
const (
	lookupContainedBy = "contained_by"
	lookupHasKey      = "has_key"
	lookupHasKeys     = "has_keys"
	lookupHasAnyKeys  = "has_any_keys"
)

// jsonLookupNames lists the lookups of a JSON value that other values lack.
var jsonLookupNames = []string{lookupContainedBy, lookupHasKey, lookupHasKeys, lookupHasAnyKeys}

// lookupsOf returns the lookups of a value of kind k.
func lookupsOf(k Kind) []string {
	if k == KindJSON {
		return slices.Concat(lookupNames, jsonLookupNames)
	}
	return lookupNames
}

// sqliteJSONTypes are the results of SQLite's JSON_TYPE that a key transform
// gives instead of the value JSON_EXTRACT reads, which would be SQL NULL, 0
// or 1: Django's jsonfield_datatype_values.
var sqliteJSONTypes = []string{"null", "false", "true"}

// jsonField is the field a value compared with a JSON value is bound for: its
// JSON text.
var jsonField = &Field{Kind: KindJSON}

// KeyTransform returns the value under key of the JSON value expr, Django's
// KeyTransform. expr is a field or annotation name, such as "value", or an
// expression. key is an object key, or an array index when it reads as an
// integer: KeyTransform(0, "tags") is the first element of tags. The filter
// key "value__a__0" and F("value__a__0") are key transforms too.
func KeyTransform(key, expr any) Expr {
	a := fieldOrExpr(expr)
	return Expr{kind: exprKeyTransform, key: fmt.Sprint(key), arg: &a}
}

// KeyTextTransform is KeyTransform with the value read as text, Django's
// KeyTextTransform: a JSON string without its quotes.
func KeyTextTransform(key, expr any) Expr {
	e := KeyTransform(key, expr)
	e.typed, e.output = true, KindText
	return e
}

// KT returns the text of the value under the path lookup, such as
// KT("value__d__0"), Django's KT: KeyTextTransform of each key in turn.
func KT(lookup string) Expr {
	parts := strings.Split(lookup, lookupSep)
	if len(parts) < 2 {
		return Expr{kind: exprKeyTransform, err: errors.New("orm: Lookup must contain key or index transforms.")} //nolint:revive,staticcheck // Django's message
	}
	e := F(parts[0])
	for _, key := range parts[1:] {
		e = KeyTextTransform(key, e)
	}
	return e
}

// jsonKeyNode is the value under a path of keys of a JSON value, Django's
// KeyTransform, or with text set its text, KeyTextTransform. A key transform
// of a key transform is one node with both keys, as Django's preprocess_lhs
// collects the chain.
type jsonKeyNode struct {
	inner sqlNode
	keys  []string
	text  bool
}

// render writes the operators of PostgreSQL, -> and #> or ->> and #>>, or
// SQLite's JSON_EXTRACT, which reads JSON null and the booleans with
// JSON_TYPE. The keys are SQL literals, as Django binds them on the client.
func (n jsonKeyNode) render(w *sqlWriter) {
	if w.d.Name() != "postgres" {
		path := sqlString(jsonPath(n.keys))
		w.write("(CASE WHEN JSON_TYPE(")
		n.inner.render(w)
		w.write(", " + path + ") IN ('" + strings.Join(sqliteJSONTypes, "', '") + "') THEN JSON_TYPE(")
		n.inner.render(w)
		w.write(", " + path + ") ELSE JSON_EXTRACT(")
		n.inner.render(w)
		w.write(", " + path + ") END)")
		return
	}
	op := "->"
	if len(n.keys) > 1 {
		op = "#>"
	}
	if n.text {
		op += ">"
	}
	w.write("(")
	n.inner.render(w)
	w.write(" " + op + " ")
	switch i, ok := pyInt(n.keys[0]); {
	case len(n.keys) > 1:
		quoted := make([]string, len(n.keys))
		for j, k := range n.keys {
			quoted[j] = sqlString(k)
		}
		w.write("ARRAY[" + strings.Join(quoted, ", ") + "]")
	case ok:
		w.write(strconv.FormatInt(i, 10))
	default:
		w.write(sqlString(n.keys[0]))
	}
	w.write(")")
}

// vtype is a string for the text of the value, and JSON otherwise.
func (n jsonKeyNode) vtype() valueType {
	if n.text {
		return typeString
	}
	return typeJSON
}

// aggregate reports whether the JSON value holds an aggregate.
func (n jsonKeyNode) aggregate() bool { return n.inner.aggregate() }

// sqlString returns s as an SQL string literal. keyTransform refuses the NUL
// byte, the one character a literal cannot hold.
func sqlString(s string) string {
	q, _ := quoteString(s)
	return q
}

// pyInt parses s as Python's int() does: an optional sign and decimal
// digits, single underscores between digits, and whitespace around.
func pyInt(s string) (int64, bool) {
	s = strings.TrimSpace(s)
	digits := s
	if digits != "" && (digits[0] == '+' || digits[0] == '-') {
		digits = digits[1:]
	}
	if strings.HasPrefix(digits, "_") || strings.HasSuffix(digits, "_") || strings.Contains(digits, "__") {
		return 0, false
	}
	n, err := strconv.ParseInt(strings.ReplaceAll(s, "_", ""), 10, 64)
	return n, err == nil
}

// jsonPath returns SQLite's JSON path of keys, Django's compile_json_path: an
// integer key is an array index, counted from the end when negative, and any
// other key an object key.
func jsonPath(keys []string) string {
	var b strings.Builder
	b.WriteByte('$')
	writeJSONPath(&b, keys)
	return b.String()
}

// writeJSONPath writes the steps of jsonPath without the root.
func writeJSONPath(b *strings.Builder, keys []string) {
	for _, key := range keys {
		i, ok := pyInt(key)
		switch {
		case !ok:
			b.WriteByte('.')
			pyjson.WriteString(b, key)
		case i < 0:
			fmt.Fprintf(b, "[#%d]", i)
		default:
			fmt.Fprintf(b, "[%d]", i)
		}
	}
}

// keyTransform returns the value under key of inner, a JSON value, or its
// text when text is set. A key transform of a key transform extends its
// path.
func (c *compiler) keyTransform(inner sqlNode, key string, text bool) (sqlNode, error) {
	if name := c.d.Name(); name != "postgres" && name != "sqlite" {
		return nil, fmt.Errorf("orm: the %s dialect does not support key transforms", name)
	}
	if strings.ContainsRune(key, 0) {
		return nil, errors.New("orm: a JSON key must not contain a NUL byte")
	}
	keys := []string{key}
	if k, ok := inner.(jsonKeyNode); ok {
		inner, keys = k.inner, append(slices.Clip(k.keys), key)
	}
	return jsonKeyNode{inner: inner, keys: keys, text: text}, nil
}

// jsonOperand reports whether a lookup of lhs, whose field is field, is a
// lookup of a JSON value.
func jsonOperand(lhs sqlNode, field *Field) bool {
	if k, ok := lhs.(jsonKeyNode); ok {
		return !k.text
	}
	return field != nil && field.Kind == KindJSON
}

// jsonPredicate returns the predicate of a lookup that Django defines for a
// JSON value or a key transform, or nil for a lookup that compares a JSON
// value as any other value.
func (c *compiler) jsonPredicate(lookup string, lhs sqlNode, value any) (*predicate, error) {
	switch lookup {
	case lookupContains, lookupContainedBy:
		return c.containsPredicate(lookup, lhs, value)
	case lookupHasKey, lookupHasKeys, lookupHasAnyKeys:
		return c.hasKeyPredicate(lookup, lhs, value)
	}
	if lookup == lookupExact && value == nil {
		// Django's JSONExact compares nil with JSON null, not SQL NULL.
		return &predicate{lookup: lookupExact, lhs: lhs, rhs: []sqlNode{paramNode{value: "null"}}}, nil
	}
	if k, ok := lhs.(jsonKeyNode); ok {
		return c.keyPredicate(lookup, k, value)
	}
	return nil, nil
}

// keyPredicate returns the predicate of a lookup of a key transform that
// Django's KeyTransform registers for itself, or nil for a lookup that
// compares the value under the key as a JSON value.
func (c *compiler) keyPredicate(lookup string, key jsonKeyNode, value any) (*predicate, error) {
	sqlite := c.d.Name() == "sqlite"
	switch lookup {
	case lookupIsNull:
		// KeyTransformIsNull tests that the key is missing, which JSON_TYPE
		// tells apart from JSON null on SQLite.
		b, ok := value.(bool)
		if !ok || !sqlite {
			return nil, nil
		}
		return &predicate{lookup: lookupIsNull, lhs: jsonType(key.inner, jsonPath(key.keys)), flag: b}, nil
	case lookupExact:
		if _, ok := value.(AnyQuerySet); ok {
			return nil, nil
		}
		rhs, err := c.prepValue(jsonField, value, nil)
		if err != nil || rhs == nil {
			return nil, err
		}
		return &predicate{lookup: lookup, lhs: key, rhs: []sqlNode{c.extractValue(rhs)}, empty: emptySubquery(rhs)}, nil
	case lookupIn:
		p := &predicate{lookup: lookup, lhs: key}
		if err := c.buildIn(p, jsonField, value, nil); err != nil {
			return nil, err
		}
		for i, r := range p.rhs {
			p.rhs[i] = c.extractValue(r)
		}
		return p, nil
	case lookupLt, lookupLte, lookupGt, lookupGte:
		// SQLite compares the SQL value JSON_EXTRACT reads, so the value is
		// bound as itself, not as JSON text.
		if !sqlite {
			return nil, nil
		}
		rhs, err := c.prepValue(nil, value, nil)
		if err != nil {
			return nil, err
		}
		if rhs == nil {
			return nil, errors.New("orm: cannot use nil as a query value")
		}
		return &predicate{lookup: lookup, lhs: key, rhs: []sqlNode{rhs}}, nil
	case lookupIExact, lookupIContains, lookupStartsWith, lookupIStartsWith,
		lookupEndsWith, lookupIEndsWith, lookupRegex, lookupIRegex:
		if value == nil {
			// Django turns iexact=None into the isnull of the key.
			return c.keyPredicate(lookupIsNull, key, true)
		}
		// The text lookups match the text of the value, Django's
		// KeyTransformTextLookupMixin.
		key.text = true
		return c.buildPredicate(lookup, key, &Field{Kind: KindText}, value, nil)
	}
	return nil, nil
}

// extractValue returns the JSON text n compared with a key transform: on
// SQLite the value that JSON_EXTRACT reads from it, unless it is JSON null
// or a boolean, which the key transform reads with JSON_TYPE, as Django's
// KeyTransformExact and KeyTransformIn do. An expression is compared as it
// is.
func (c *compiler) extractValue(n sqlNode) sqlNode {
	p, ok := n.(paramNode)
	if !ok || c.d.Name() != "sqlite" {
		return n
	}
	if s, ok := p.value.(string); ok && slices.Contains(sqliteJSONTypes, s) {
		return n
	}
	return callNode{fn: "JSON_EXTRACT", args: []sqlNode{n, rawNode{sql: "'$'", typ: typeString}}, typ: typeJSON}
}

// jsonType returns SQLite's JSON_TYPE of the value under path of root, which
// is SQL NULL when the path does not exist.
func jsonType(root sqlNode, path string) sqlNode {
	return callNode{fn: "JSON_TYPE", args: []sqlNode{root, rawNode{sql: sqlString(path), typ: typeString}}, typ: typeString}
}

// containsPredicate returns the predicate of the contains or contained_by
// lookup of a JSON value: PostgreSQL's @> or <@ operator. SQLite has no
// containment test, as in Django.
func (c *compiler) containsPredicate(lookup string, lhs sqlNode, value any) (*predicate, error) {
	if c.d.Name() != "postgres" {
		return nil, fmt.Errorf("orm: %s lookup is not supported on this database backend.", lookup) //nolint:revive,staticcheck // Django's message
	}
	rhs, err := c.prepValue(jsonField, value, nil)
	if err != nil {
		return nil, err
	}
	if rhs == nil {
		return nil, errors.New("orm: cannot use nil as a query value")
	}
	op := "@>"
	if lookup == lookupContainedBy {
		op = "<@"
	}
	return &predicate{lookup: lookup, op: op, lhs: lhs, rhs: []sqlNode{rhs}}, nil
}

// hasKeyPredicate returns the predicate of the has_key, has_keys or
// has_any_keys lookup of lhs, a JSON value. The value of has_key is a key or
// a key transform, whose path below its JSON value is the path tested, and
// the value of the others a slice of keys.
func (c *compiler) hasKeyPredicate(lookup string, lhs sqlNode, value any) (*predicate, error) {
	var paths [][]string
	if lookup == lookupHasKey {
		path, err := c.keyPathOf(value)
		if err != nil {
			return nil, err
		}
		paths = [][]string{path}
	} else {
		rv := reflect.ValueOf(value)
		if !rv.IsValid() || (rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array) {
			return nil, fmt.Errorf("orm: %s lookup requires a slice, got %T", lookup, value)
		}
		for i := range rv.Len() {
			key := fmt.Sprint(rv.Index(i).Interface())
			if strings.ContainsRune(key, 0) {
				return nil, errors.New("orm: a JSON key must not contain a NUL byte")
			}
			paths = append(paths, []string{key})
		}
	}
	if c.d.Name() == "postgres" {
		return c.pgHasKeyPredicate(lookup, lhs, paths)
	}
	root, base := lhs, "$"
	if k, ok := lhs.(jsonKeyNode); ok {
		root, base = k.inner, jsonPath(k.keys)
	}
	conn := connAnd
	if lookup == lookupHasAnyKeys {
		conn = connOr
	}
	where := &whereNode{conn: conn}
	for _, path := range paths {
		var b strings.Builder
		b.WriteString(base)
		writeJSONPath(&b, path[:len(path)-1])
		// The last key is an object key even when it reads as an integer.
		b.WriteByte('.')
		pyjson.WriteString(&b, path[len(path)-1])
		where.children = append(where.children, whereChild{pred: &predicate{lookup: lookupIsNull, lhs: jsonType(root, b.String())}})
	}
	return &predicate{lookup: lookupExpr, lhs: condNode{where: where}}, nil
}

// pgHasKeyPredicate returns the predicate of a has_key lookup on
// PostgreSQL: the ? operator, or ?& or ?| with an array of keys. The leading
// keys of a key transform value extend lhs, as in Django's
// HasKeyLookup.as_postgresql.
func (c *compiler) pgHasKeyPredicate(lookup string, lhs sqlNode, paths [][]string) (*predicate, error) {
	if lookup == lookupHasKey {
		path := paths[0]
		for _, key := range path[:len(path)-1] {
			n, err := c.keyTransform(lhs, key, false)
			if err != nil {
				return nil, err
			}
			lhs = n
		}
		rhs := rawNode{sql: sqlString(path[len(path)-1]), typ: typeString}
		return &predicate{lookup: lookup, op: "?", lhs: lhs, rhs: []sqlNode{rhs}}, nil
	}
	quoted := make([]string, len(paths))
	for i, path := range paths {
		quoted[i] = sqlString(path[0])
	}
	op := "?&"
	if lookup == lookupHasAnyKeys {
		op = "?|"
	}
	rhs := rawNode{sql: "ARRAY[" + strings.Join(quoted, ", ") + "]::text[]", typ: typeUnknown}
	return &predicate{lookup: lookup, op: op, lhs: lhs, rhs: []sqlNode{rhs}}, nil
}

// keyPathOf returns the path of keys that the value of a has_key lookup
// names: the key itself, or the keys of a key transform.
func (c *compiler) keyPathOf(value any) ([]string, error) {
	var e Expr
	switch v := value.(type) {
	case nil:
		return nil, errors.New("orm: cannot use nil as a query value")
	case Expr:
		e = v
	case *Expr:
		if v == nil {
			return nil, errors.New("orm: cannot use nil as a query value")
		}
		e = *v
	default:
		key := fmt.Sprint(v)
		if strings.ContainsRune(key, 0) {
			return nil, errors.New("orm: a JSON key must not contain a NUL byte")
		}
		return []string{key}, nil
	}
	n, err := c.resolveExpr(e)
	if err != nil {
		return nil, err
	}
	k, ok := n.(jsonKeyNode)
	if !ok {
		return nil, errors.New("orm: the value of a has_key lookup must be a key or a key transform")
	}
	return k.keys, nil
}
