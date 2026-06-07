package orm

import (
	"fmt"
	"reflect"
	"strings"
	"time"
)

// joinedRel captures everything the compiler and the scanner need about one
// eager-loaded forward FK: the FK field on the main model, the resolved target
// model, and the JOIN alias used to disambiguate the target's columns.
type joinedRel struct {
	field  *Field // the FK field on the main model
	target *Model // the related model
	alias  string // table alias for the target in SELECT/JOIN/scan
}

// resolveSelectRelated validates each requested field name against the model and
// returns the joinedRel descriptors in request order. A name that is unknown or
// does not identify a forward FK is an error, as is an FK whose target has not
// been resolved (call Registry.Resolve first).
func (q *QuerySet[T]) resolveSelectRelated() ([]joinedRel, error) {
	rels := make([]joinedRel, 0, len(q.selectRelated))
	for _, name := range q.selectRelated {
		f, ok := q.model.FieldByName(name)
		if !ok {
			return nil, fmt.Errorf("orm: SelectRelated: unknown field %q on model %s", name, q.model.Name())
		}
		if f.Rel == nil || f.Rel.Kind != RelFK {
			return nil, fmt.Errorf("orm: SelectRelated: field %q on model %s is not a forward foreign key", name, q.model.Name())
		}
		if f.Rel.Target == nil {
			return nil, fmt.Errorf(
				"orm: SelectRelated: field %q on model %s has an unresolved target; call Registry.Resolve",
				name, q.model.Name(),
			)
		}
		rels = append(rels, joinedRel{field: f, target: f.Rel.Target, alias: "st_" + f.Column})
	}
	return rels, nil
}

// compileSelectRelated renders a single LEFT JOIN query that selects the main
// table's columns followed, for each requested FK, by the target's columns. The
// main table is referenced by its own name; each joined target uses a per-FK
// alias. WHERE and ORDER BY columns are qualified by the main table name so they
// remain unambiguous in the JOIN.
func (q *QuerySet[T]) compileSelectRelated() (string, []any, error) {
	d := q.db.Dialect()
	rels, err := q.resolveSelectRelated()
	if err != nil {
		return "", nil, err
	}
	mainTable := q.model.Table()
	qMain := d.Quote(mainTable)

	var sel []string
	for _, c := range q.model.Columns() {
		sel = append(sel, qMain+"."+d.Quote(c))
	}
	var joins strings.Builder
	for _, r := range rels {
		qAlias := d.Quote(r.alias)
		for _, c := range r.target.Columns() {
			sel = append(sel, qAlias+"."+d.Quote(c))
		}
		targetPK := r.target.PrimaryKey()
		if targetPK == nil {
			return "", nil, fmt.Errorf(
				"orm: SelectRelated: target model %s of field %s has no primary key",
				r.target.Name(), r.field.Name,
			)
		}
		joins.WriteString(" LEFT JOIN ")
		joins.WriteString(d.Quote(r.target.Table()))
		joins.WriteString(" AS ")
		joins.WriteString(qAlias)
		joins.WriteString(" ON ")
		joins.WriteString(qMain + "." + d.Quote(r.field.Column))
		joins.WriteString(" = ")
		joins.WriteString(qAlias + "." + d.Quote(targetPK.Column))
	}

	var b strings.Builder
	b.WriteString("SELECT ")
	b.WriteString(strings.Join(sel, ", "))
	b.WriteString(" FROM ")
	b.WriteString(qMain)
	b.WriteString(joins.String())

	n := 0
	next := func() string {
		n++
		return d.Placeholder(n)
	}
	where, args, err := q.compileWhere(next, mainTable)
	if err != nil {
		return "", nil, err
	}
	b.WriteString(where)

	order, err := q.compileOrderBy(mainTable)
	if err != nil {
		return "", nil, err
	}
	b.WriteString(order)

	b.WriteString(q.compileLimitOffset())

	return b.String(), args, nil
}

// assignScanned writes a driver-native scanned value v into the struct field
// dst, converting between common SQL/Go representations. A nil value leaves dst
// at its zero value.
func assignScanned(dst reflect.Value, v any) error {
	if v == nil {
		return nil
	}
	if t, ok := v.(time.Time); ok {
		if dst.Type() == timeType {
			dst.Set(reflect.ValueOf(t))
			return nil
		}
		return fmt.Errorf("orm: assignScanned: cannot assign time.Time into %s", dst.Type())
	}
	switch dst.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		i, err := asInt64(v)
		if err != nil {
			return err
		}
		dst.SetInt(i)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		i, err := asInt64(v)
		if err != nil {
			return err
		}
		dst.SetUint(uint64(i))
	case reflect.Float32, reflect.Float64:
		f, ok := v.(float64)
		if !ok {
			return fmt.Errorf("orm: assignScanned: cannot assign %T into float field", v)
		}
		dst.SetFloat(f)
	case reflect.Bool:
		b, ok := v.(bool)
		if !ok {
			return fmt.Errorf("orm: assignScanned: cannot assign %T into bool field", v)
		}
		dst.SetBool(b)
	case reflect.String:
		switch s := v.(type) {
		case string:
			dst.SetString(s)
		case []byte:
			dst.SetString(string(s))
		default:
			return fmt.Errorf("orm: assignScanned: cannot assign %T into string field", v)
		}
	default:
		return fmt.Errorf("orm: assignScanned: unsupported destination kind %s", dst.Kind())
	}
	return nil
}

// asInt64 coerces a driver-native integer-ish value into an int64.
func asInt64(v any) (int64, error) {
	switch n := v.(type) {
	case int64:
		return n, nil
	case int32:
		return int64(n), nil
	case int:
		return int64(n), nil
	default:
		return 0, fmt.Errorf("orm: assignScanned: cannot coerce %T into an integer", v)
	}
}
