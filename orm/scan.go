package orm

import (
	"database/sql"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"
)

// timeLayouts are the text forms of a timestamp the drivers produce for a
// value without a declared column type, such as MIN(created_at) on SQLite.
var timeLayouts = []string{
	"2006-01-02 15:04:05.999999999 -0700 MST",
	"2006-01-02 15:04:05.999999999-07:00",
	"2006-01-02T15:04:05.999999999-07:00",
	time.RFC3339Nano,
	"2006-01-02 15:04:05.999999999",
	"2006-01-02T15:04:05.999999999",
	"2006-01-02 15:04",
	"2006-01-02T15:04",
	"2006-01-02",
}

// parseTime parses the text form of a timestamp.
func parseTime(s string) (time.Time, error) {
	// time.Time.String appends the monotonic clock reading, which no layout
	// accepts.
	if i := strings.Index(s, " m="); i >= 0 {
		s = s[:i]
	}
	for _, layout := range timeLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("orm: cannot parse %q as a time", s)
}

// convertValue converts a value read from the driver to the Go type that
// stands for t: int64, float64, string, bool, or time.Time. NULL stays nil.
// The drivers disagree on the type of a computed column (PostgreSQL returns
// SUM and AVG as text, SQLite returns a timestamp aggregate as text), and the
// conversion gives both the same result.
func convertValue(v any, t valueType) (any, error) {
	if v == nil {
		return nil, nil
	}
	if t == typeBinary || t == typeJSON || t == typeDecimal || t == typeDate ||
		t == typeTimeOfDay || t == typeDuration || t == typeUUID {
		return convertKindValue(v, t)
	}
	if b, ok := v.([]byte); ok {
		v = string(b)
	}
	switch t {
	case typeInt:
		return toInt64(v)
	case typeFloat:
		return toFloat64(v)
	case typeString:
		if s, ok := v.(string); ok {
			return s, nil
		}
		return fmt.Sprint(v), nil
	case typeBool:
		return toBool(v)
	case typeTime:
		switch x := v.(type) {
		case time.Time:
			return x.UTC(), nil
		case string:
			t, err := parseTime(x)
			if err != nil {
				return nil, err
			}
			return t.UTC(), nil
		}
		return nil, fmt.Errorf("orm: cannot convert %T to a time", v)
	default:
		return v, nil
	}
}

// toInt64 converts a driver value to an int64. A float or a decimal text
// without a fraction is accepted, since SUM of integers is a numeric on
// PostgreSQL.
func toInt64(v any) (int64, error) {
	switch x := v.(type) {
	case int64:
		return x, nil
	case int32:
		return int64(x), nil
	case int:
		return int64(x), nil
	case float64:
		return int64(x), nil
	case bool:
		if x {
			return 1, nil
		}
		return 0, nil
	case string:
		if n, err := strconv.ParseInt(x, 10, 64); err == nil {
			return n, nil
		}
		f, err := strconv.ParseFloat(x, 64)
		if err != nil {
			return 0, fmt.Errorf("orm: cannot convert %q to an integer", x)
		}
		return int64(f), nil
	default:
		return 0, fmt.Errorf("orm: cannot convert %T to an integer", v)
	}
}

// toFloat64 converts a driver value to a float64.
func toFloat64(v any) (float64, error) {
	switch x := v.(type) {
	case float64:
		return x, nil
	case float32:
		return float64(x), nil
	case int64:
		return float64(x), nil
	case string:
		f, err := strconv.ParseFloat(x, 64)
		if err != nil {
			return 0, fmt.Errorf("orm: cannot convert %q to a float", x)
		}
		return f, nil
	default:
		return 0, fmt.Errorf("orm: cannot convert %T to a float", v)
	}
}

// toBool converts a driver value to a bool. SQLite stores a boolean as 0 or 1.
func toBool(v any) (bool, error) {
	switch x := v.(type) {
	case bool:
		return x, nil
	case int64:
		return x != 0, nil
	case string:
		b, err := strconv.ParseBool(x)
		if err != nil {
			return false, fmt.Errorf("orm: cannot convert %q to a bool", x)
		}
		return b, nil
	default:
		return false, fmt.Errorf("orm: cannot convert %T to a bool", v)
	}
}

// assignScanned writes a value read from the driver into the struct field
// dst, converting between the common SQL and Go representations. NULL leaves
// a plain field at its zero value.
func assignScanned(dst reflect.Value, v any) error {
	if sc, ok := dst.Addr().Interface().(sql.Scanner); ok {
		return sc.Scan(v)
	}
	if v == nil {
		dst.SetZero()
		return nil
	}
	if dst.Type() == timeType {
		t, err := convertValue(v, typeTime)
		if err != nil {
			return err
		}
		dst.Set(reflect.ValueOf(t))
		return nil
	}
	switch dst.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		i, err := toInt64(v)
		if err != nil {
			return err
		}
		dst.SetInt(i)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		i, err := toInt64(v)
		if err != nil {
			return err
		}
		dst.SetUint(uint64(i))
	case reflect.Float32, reflect.Float64:
		f, err := toFloat64(v)
		if err != nil {
			return err
		}
		dst.SetFloat(f)
	case reflect.Bool:
		b, err := toBool(v)
		if err != nil {
			return err
		}
		dst.SetBool(b)
	case reflect.String:
		switch s := v.(type) {
		case string:
			dst.SetString(s)
		case []byte:
			dst.SetString(string(s))
		default:
			return fmt.Errorf("orm: cannot assign %T into a string field", v)
		}
	default:
		return fmt.Errorf("orm: unsupported destination kind %s", dst.Kind())
	}
	return nil
}

// fieldTarget returns the Scan destination for field f of the struct elem.
func fieldTarget(d Dialect, elem reflect.Value, f *Field) any {
	fv := elem.Field(f.Index)
	if f.Rel == nil {
		return fieldScanner{dialect: d, field: f, dst: fv}
	}
	return fv.Addr().Interface()
}

// relPlan describes one relation loaded by SelectRelated: either a forward
// FK/O2O field (field != nil) or a reverse O2O relation (rev != nil), the join
// its columns come from, and the plan of the object that owns the relation.
type relPlan struct {
	field  *Field
	rev    *ReverseRelation
	join   *join
	parent int // index of the owning plan, or -1 for the row itself
}

// rowPlan describes the columns of a SELECT of model rows, in order: the
// model's own fields, the fields of each select_related target, the
// annotations, and any hidden ordering columns.
type rowPlan struct {
	dialect             Dialect
	model               *Model
	prefetchParentField *Field
	rels                []relPlan
	annots              []string
	annotTypes          []valueType
	ncols               int
}

// scan reads the current row. It returns the model struct, addressable, and
// the annotation values in plan order.
func (p *rowPlan) scan(rows *sql.Rows) (reflect.Value, []any, error) {
	elem, _, annots, err := p.scanWithParent(rows)
	return elem, annots, err
}

// scanWithParent reads the current row together with the optional M2M prefetch
// parent primary key column.
func (p *rowPlan) scanWithParent(rows *sql.Rows) (reflect.Value, any, []any, error) {
	elem := reflect.New(p.model.goType).Elem()
	targets := make([]any, 0, p.ncols)
	var rawParentKey any
	if p.prefetchParentField != nil {
		targets = append(targets, &rawParentKey)
	}
	for _, f := range p.model.fields {
		targets = append(targets, fieldTarget(p.dialect, elem, f))
	}
	holders := make([][]any, len(p.rels))
	for i, r := range p.rels {
		holders[i] = make([]any, len(r.join.model.fields))
		for k := range holders[i] {
			targets = append(targets, &holders[i][k])
		}
	}
	annots := make([]any, len(p.annots))
	for i := range annots {
		targets = append(targets, &annots[i])
	}
	for len(targets) < p.ncols {
		targets = append(targets, new(any))
	}
	if err := rows.Scan(targets...); err != nil {
		return elem, nil, nil, err
	}
	if err := p.attach(elem, holders); err != nil {
		return elem, nil, nil, err
	}
	for i, t := range p.annotTypes {
		v, err := convertValue(annots[i], t)
		if err != nil {
			return elem, nil, nil, err
		}
		annots[i] = v
	}
	var parentKey any
	if p.prefetchParentField != nil {
		var err error
		parentKey, err = convertPrefetchParentKey(p.prefetchParentField, rawParentKey)
		if err != nil {
			return elem, nil, nil, err
		}
	}
	return elem, parentKey, annots, nil
}

// attach builds the select_related objects from their scanned columns and
// stores each on its forward relation field or reverse O2O RelCache. A
// relation whose primary key is NULL matched no row: a forward relation stays
// unloaded, a reverse O2O records a missing entry in RelCache (so RelatedOne
// returns ErrDoesNotExist without querying), and everything below it is
// skipped.
func (p *rowPlan) attach(elem reflect.Value, holders [][]any) error {
	objs := make([]reflect.Value, len(p.rels))
	for i, r := range p.rels {
		owner := elem
		ownerModel := p.model
		if r.parent >= 0 {
			if !objs[r.parent].IsValid() {
				continue
			}
			owner = objs[r.parent].Elem()
			ownerModel = p.rels[r.parent].join.model
		}
		target := r.join.model
		var pk any
		for k, f := range target.fields {
			if f == target.pk {
				pk = holders[i][k]
			}
		}
		if pk == nil {
			if r.rev != nil {
				setReverseOneCacheModel(ownerModel, owner, r.rev.Name, nil, target, true)
			}
			continue
		}
		pkInt, err := toInt64(pk)
		if err != nil {
			return err
		}
		obj := reflect.New(target.goType)
		for k, f := range target.fields {
			if err := assignFieldScanned(p.dialect, f, obj.Elem().Field(f.Index), holders[i][k]); err != nil {
				return fmt.Errorf("field %s.%s: %w", target.Name(), f.Name, err)
			}
		}
		if r.rev != nil {
			setReverseOneCacheModel(ownerModel, owner, r.rev.Name, obj.Interface(), target, false)
			// Cache the back-reference on obj's forward O2O field so
			// obj.<field>.Fetch requires no query.
			if ownerPK, pkErr := extractModelPKInt64(ownerModel, owner); pkErr == nil && ownerPK != 0 {
				setForwardObject(obj.Elem(), r.rev.Field, owner.Addr(), ownerPK)
			}
			objs[i] = obj
			continue
		}
		set := owner.Field(r.field.Index).Addr().MethodByName("SetObject")
		if !set.IsValid() {
			return fmt.Errorf("field %s does not expose SetObject", r.field.Name)
		}
		set.Call([]reflect.Value{obj, reflect.ValueOf(pkInt)})
		// For a forward O2O, also cache the owning object on target's reverse
		// RelCache so following forward then back requires no query.
		if r.field.Rel != nil && r.field.Rel.Kind == RelO2O {
			if backRev := target.reverseForField(r.field); backRev != nil && !backRev.Hidden {
				setReverseOneCacheModel(target, obj.Elem(), backRev.Name, owner.Addr().Interface(), ownerModel, false)
			}
		}
		objs[i] = obj
	}
	return nil
}
