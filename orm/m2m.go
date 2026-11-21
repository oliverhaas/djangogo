package orm

import (
	"context"
	"database/sql/driver"
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

// M2M declares a many-to-many relation from the enclosing model to T, the
// counterpart of Django's ManyToManyField. It occupies no column on the
// declaring model's table; associations live in an auto-created join table
// ("<model_table>_<attr>") or an explicit intermediate model named by the
// through= tag option.
//
// Supported orm struct tag options:
//   - related_name=<name>        reverse accessor name on T (default "<source>_set"; "+" suppresses reverse access)
//   - related_query_name=<name>  reverse query lookup name on T
//   - through=<Model>            explicit intermediate model name or "<app>.<Model>" label
//   - through_fields=<src>,<dst> FK field names on the through model when multiple FKs target the same model
//   - symmetrical=true|false     for self-referential M2M only (default true for self-ref)
//   - db_table=<name>            override the auto-created join table name
//   - blank                      validation/form flag (blank=True)
//   - editable=true|false        form/admin flag
//   - verbose_name=<text>        human-readable field name
//   - help_text=<text>           form help text
type M2M[T any] struct{}

// relKind reports RelM2M so reflection recognizes many-to-many struct fields.
func (M2M[T]) relKind() RelKind { return RelM2M }

// relTarget reports the related target struct type.
func (M2M[T]) relTarget() reflect.Type { var z T; return reflect.TypeOf(z) }

// applyM2MTagOption applies one parsed orm tag token to an M2M field.
func applyM2MTagOption(f *Field, fieldName, token string) error {
	switch {
	case token == "blank":
		f.Blank = true
	case token == "editable=false":
		f.Editable = false
	case token == "editable=true":
		f.Editable = true
	case strings.HasPrefix(token, "editable="):
		return fmt.Errorf("orm: field %s: invalid %q: editable must be true or false", fieldName, token)
	case strings.HasPrefix(token, "verbose_name="):
		f.VerboseName = token[len("verbose_name="):]
	case strings.HasPrefix(token, "help_text="):
		f.HelpText = token[len("help_text="):]
	case strings.HasPrefix(token, "related_name="):
		rn := token[len("related_name="):]
		if rn == "" {
			return fmt.Errorf("orm: field %s: related_name must not be empty", fieldName)
		}
		f.Rel.RelatedName = rn
	case strings.HasPrefix(token, "related_query_name="):
		rqn := token[len("related_query_name="):]
		if rqn == "" {
			return fmt.Errorf("orm: field %s: related_query_name must not be empty", fieldName)
		}
		f.Rel.RelatedQueryName = rqn
	case strings.HasPrefix(token, "through="):
		th := strings.TrimSpace(token[len("through="):])
		if th == "" {
			return fmt.Errorf("orm: field %s: through must not be empty", fieldName)
		}
		f.Rel.ThroughName = th
	case strings.HasPrefix(token, "through_fields="):
		raw := token[len("through_fields="):]
		parts := strings.Split(raw, ",")
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
			return fmt.Errorf("orm: field %s: through_fields must specify two comma-separated field names", fieldName)
		}
		f.Rel.ThroughFields = [2]string{strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])}
	case token == "symmetrical=true":
		f.Rel.Symmetrical = true
		f.Rel.symmetricalSet = true
	case token == "symmetrical=false":
		f.Rel.Symmetrical = false
		f.Rel.symmetricalSet = true
	case strings.HasPrefix(token, "symmetrical="):
		return fmt.Errorf("orm: field %s: invalid %q: symmetrical must be true or false", fieldName, token)
	case strings.HasPrefix(token, "db_table="):
		tbl := strings.TrimSpace(token[len("db_table="):])
		if tbl == "" {
			return fmt.Errorf("orm: field %s: db_table must not be empty", fieldName)
		}
		f.Rel.DBTable = tbl
	case token == "pk", token == "unique", token == "index", token == "null", token == "null=true", token == "null=false",
		strings.HasPrefix(token, "column="), strings.HasPrefix(token, "on_delete="),
		token == "type=text", strings.HasPrefix(token, "max_length="),
		token == "auto_now", token == "auto_now_add", strings.HasPrefix(token, "default="):
		return fmt.Errorf("orm: field %s: %q is not valid on a many-to-many field", fieldName, token)
	default:
		return fmt.Errorf("orm: field %s: unknown tag option %q", fieldName, token)
	}
	return nil
}

// resolveM2MRelations links every M2M field to its target model and resolves
// or synthesizes its intermediate (through) model. Existing auto-created
// through models are cleared first so Resolve remains idempotent.
func (r *Registry) resolveM2MRelations() error {
	r.autoThroughModels = nil
	autoTables := make(map[string]*Model)

	for _, m := range r.models {
		for _, f := range m.m2mFields {
			tm, ok := r.byType[f.Rel.targetType]
			if !ok {
				return fmt.Errorf(
					"orm: model %s field %s references unregistered model %s",
					m.name, f.Name, f.Rel.targetType,
				)
			}
			f.Rel.Target = tm
			if f.Rel.ThroughName != "" && f.Rel.DBTable != "" {
				return fmt.Errorf("orm: model %s field %s: cannot specify both through and db_table", m.name, f.Name)
			}
			if m == tm {
				if !f.Rel.symmetricalSet {
					f.Rel.Symmetrical = true
				}
			} else if f.Rel.Symmetrical {
				return fmt.Errorf("orm: model %s field %s: symmetrical=true is only valid on self-referential M2M fields", m.name, f.Name)
			}

			if f.Rel.ThroughName != "" {
				if err := r.resolveExplicitThrough(m, f, tm); err != nil {
					return err
				}
				continue
			}

			through, srcField, dstField, err := r.synthesizeThroughModel(m, f, tm, autoTables)
			if err != nil {
				return err
			}
			f.Rel.AutoCreated = true
			f.Rel.Through = through
			f.Rel.SourceField = srcField
			f.Rel.TargetField = dstField
			r.autoThroughModels = append(r.autoThroughModels, through)
		}
	}
	return nil
}

// lookupThroughModel resolves a through= name against r, preferring the
// declaring model's app when no app prefix is present.
func (r *Registry) lookupThroughModel(declaring *Model, throughName string) (*Model, bool) {
	if strings.Contains(throughName, ".") {
		return r.Get(throughName)
	}
	if declaring.app != "" {
		if m, ok := r.byLabel[declaring.app+"."+throughName]; ok {
			return m, true
		}
	}
	return r.Get(throughName)
}

// resolveExplicitThrough validates and attaches an explicit through= model and
// its source and target foreign keys to f.Rel.
func (r *Registry) resolveExplicitThrough(src *Model, f *Field, target *Model) error {
	through, ok := r.lookupThroughModel(src, f.Rel.ThroughName)
	if !ok {
		return fmt.Errorf("orm: model %s field %s: through model %q is not registered", src.name, f.Name, f.Rel.ThroughName)
	}
	var srcField, dstField *Field
	switch {
	case f.Rel.ThroughFields[0] != "":
		var err error
		srcField, err = resolveThroughFK(through, f.Rel.ThroughFields[0], src, src.name, f.Name)
		if err != nil {
			return err
		}
		dstField, err = resolveThroughFK(through, f.Rel.ThroughFields[1], target, src.name, f.Name)
		if err != nil {
			return err
		}
	case src == target:
		var matches []*Field
		for _, tf := range through.fields {
			if tf.Rel != nil && isForwardToOne(tf.Rel.Kind) && tf.Rel.Target == src {
				matches = append(matches, tf)
			}
		}
		if len(matches) < 2 {
			return fmt.Errorf("orm: model %s field %s: through model %s must have two foreign keys to %s", src.name, f.Name, through.name, src.name)
		}
		if len(matches) > 2 {
			return fmt.Errorf("orm: model %s field %s: through model %s has more than two foreign keys to %s; set through_fields=", src.name, f.Name, through.name, src.name)
		}
		srcField, dstField = matches[0], matches[1]
	default:
		var srcMatches, dstMatches []*Field
		for _, tf := range through.fields {
			if tf.Rel == nil || !isForwardToOne(tf.Rel.Kind) {
				continue
			}
			if tf.Rel.Target == src {
				srcMatches = append(srcMatches, tf)
			}
			if tf.Rel.Target == target {
				dstMatches = append(dstMatches, tf)
			}
		}
		if len(srcMatches) == 0 {
			return fmt.Errorf("orm: model %s field %s: through model %s has no foreign key to %s", src.name, f.Name, through.name, src.name)
		}
		if len(srcMatches) > 1 {
			return fmt.Errorf("orm: model %s field %s: through model %s has multiple foreign keys to %s; set through_fields=", src.name, f.Name, through.name, src.name)
		}
		if len(dstMatches) == 0 {
			return fmt.Errorf("orm: model %s field %s: through model %s has no foreign key to %s", src.name, f.Name, through.name, target.name)
		}
		if len(dstMatches) > 1 {
			return fmt.Errorf("orm: model %s field %s: through model %s has multiple foreign keys to %s; set through_fields=", src.name, f.Name, through.name, target.name)
		}
		srcField, dstField = srcMatches[0], dstMatches[0]
	}

	f.Rel.AutoCreated = false
	f.Rel.Through = through
	f.Rel.SourceField = srcField
	f.Rel.TargetField = dstField
	return nil
}

// resolveThroughFK finds a named FK field on through and verifies it targets wantTarget.
func resolveThroughFK(through *Model, name string, wantTarget *Model, srcName, fieldName string) (*Field, error) {
	tf, ok := resolveMetaField(name, through.byName, through.byAttr, through.byColumn)
	if !ok {
		return nil, fmt.Errorf("orm: model %s field %s: through_fields names unknown field %q on %s", srcName, fieldName, name, through.name)
	}
	if tf.Rel == nil || !isForwardToOne(tf.Rel.Kind) || tf.Rel.Target != wantTarget {
		return nil, fmt.Errorf("orm: model %s field %s: through_fields field %s.%s is not a foreign key to %s", srcName, fieldName, through.name, tf.Name, wantTarget.name)
	}
	return tf, nil
}

// fkKindForPK returns the column Kind for a foreign key referencing pk.
func fkKindForPK(pk *Field) Kind {
	if pk == nil || pk.Kind == KindAuto {
		return KindInt
	}
	return pk.Kind
}

// synthesizeThroughModel constructs the synthetic auto-created intermediate
// *Model for an M2M field, matching Django's naming for table, columns, and
// unique_together constraints.
func (r *Registry) synthesizeThroughModel(src *Model, f *Field, target *Model, autoTables map[string]*Model) (*Model, *Field, *Field, error) {
	table := f.Rel.DBTable
	if table == "" {
		table = src.Table() + "_" + f.Attr
	}
	if other, exists := r.byTable[table]; exists {
		return nil, nil, nil, fmt.Errorf("orm: model %s field %s: M2M table %q is already used by model %s", src.Label(), f.Name, table, other.Label())
	}
	if other, exists := autoTables[table]; exists {
		return nil, nil, nil, fmt.Errorf("orm: model %s field %s: M2M table %q conflicts with %s", src.Label(), f.Name, table, other.Label())
	}

	srcLower := strings.ToLower(src.name)
	dstLower := strings.ToLower(target.name)
	var srcAttr, dstAttr, srcFieldName, dstFieldName string
	if srcLower == dstLower {
		srcAttr = "from_" + srcLower
		dstAttr = "to_" + dstLower
		srcFieldName = "From" + src.name
		dstFieldName = "To" + target.name
	} else {
		srcAttr = srcLower
		dstAttr = dstLower
		srcFieldName = src.name
		dstFieldName = target.name
	}

	srcPK := src.PrimaryKey()
	dstPK := target.PrimaryKey()

	idField := &Field{
		Name:       "ID",
		Attr:       "id",
		Column:     "id",
		Kind:       KindAuto,
		PrimaryKey: true,
		Index:      -1,
	}
	srcCol := srcAttr + "_id"
	srcField := &Field{
		Name:          srcFieldName,
		Attr:          srcAttr,
		Column:        srcCol,
		Kind:          fkKindForPK(srcPK),
		MaxLength:     srcPK.MaxLength,
		MaxDigits:     srcPK.MaxDigits,
		DecimalPlaces: srcPK.DecimalPlaces,
		Index:         -1,
		Rel: &Relation{
			Kind:       RelFK,
			Column:     srcCol,
			OnDelete:   OnDeleteCascade,
			targetType: src.goType,
			Target:     src,
		},
	}
	dstCol := dstAttr + "_id"
	dstField := &Field{
		Name:          dstFieldName,
		Attr:          dstAttr,
		Column:        dstCol,
		Kind:          fkKindForPK(dstPK),
		MaxLength:     dstPK.MaxLength,
		MaxDigits:     dstPK.MaxDigits,
		DecimalPlaces: dstPK.DecimalPlaces,
		Index:         -1,
		Rel: &Relation{
			Kind:       RelFK,
			Column:     dstCol,
			OnDelete:   OnDeleteCascade,
			targetType: target.goType,
			Target:     target,
		},
	}

	fields := []*Field{idField, srcField, dstField}
	byName := map[string]*Field{idField.Name: idField, srcField.Name: srcField, dstField.Name: dstField}
	byAttr := map[string]*Field{idField.Attr: idField, srcField.Attr: srcField, dstField.Attr: dstField}
	byColumn := map[string]*Field{idField.Column: idField, srcField.Column: srcField, dstField.Column: dstField}

	throughName := src.name + "_" + f.Attr
	through := &Model{
		app:            src.app,
		name:           throughName,
		table:          table,
		fields:         fields,
		pk:             idField,
		byName:         byName,
		byAttr:         byAttr,
		byColumn:       byColumn,
		cacheIndex:     -1,
		autoCreated:    true,
		uniqueTogether: [][]string{{srcField.Name, dstField.Name}},
		verboseName:    src.name + "-" + target.name + " relationship",
	}
	through.verboseNamePlural = through.verboseName + "s"
	autoTables[table] = through
	return through, srcField, dstField, nil
}

// extractModelPK reads the primary key value of struct value sv for any
// supported primary-key field kind (integer, O2O, string, UUID, etc.),
// reporting whether the primary key is unset (zero).
func extractModelPK(m *Model, sv reflect.Value) (val any, zero bool, err error) {
	pk := m.PrimaryKey()
	if pk == nil {
		return nil, true, fmt.Errorf("orm: model %s has no primary key", m.Name())
	}
	fv := sv.Field(pk.Index)
	if pk.Rel != nil {
		if keyed, ok := fv.Interface().(interface{ PK() int64 }); ok {
			id := keyed.PK()
			return id, id == 0, nil
		}
		return nil, true, fmt.Errorf("orm: model %s primary key %s has no PK() method", m.Name(), pk.Name)
	}
	switch fv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n := fv.Int()
		return n, n == 0, nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		n := int64(fv.Uint())
		return n, n == 0, nil
	case reflect.String:
		s := fv.String()
		return s, s == "", nil
	default:
		return fv.Interface(), fv.IsZero(), nil
	}
}

// normalizePKArg converts a caller-supplied primary key value (int, int64,
// numeric string, string, UUID, or model instance) into a canonical Go key
// matching targetModel's primary key field.
func normalizePKArg(reg *Registry, targetModel *Model, raw any) (any, error) {
	if raw == nil {
		return nil, fmt.Errorf("orm: M2M primary key must not be nil")
	}
	if m, ok := reg.ModelOf(raw); ok {
		if m != targetModel {
			return nil, fmt.Errorf("orm: '%s' instance expected, got %s", targetModel.Name(), m.Name())
		}
		rv := reflect.ValueOf(raw)
		if rv.Kind() == reflect.Pointer {
			if rv.IsNil() {
				return nil, fmt.Errorf("orm: '%s' instance must not be nil", targetModel.Name())
			}
			rv = rv.Elem()
		}
		pk, zero, err := extractModelPK(targetModel, rv)
		if err != nil {
			return nil, err
		}
		if zero {
			return nil, fmt.Errorf("orm: cannot add unsaved %s instance; save it first", targetModel.Name())
		}
		return pk, nil
	}
	pkField := targetModel.PrimaryKey()
	if pkField == nil {
		return nil, fmt.Errorf("orm: model %s has no primary key", targetModel.Name())
	}
	if valuer, ok := raw.(driver.Valuer); ok {
		v, err := valuer.Value()
		if err != nil {
			return nil, err
		}
		raw = v
	}
	switch pkField.Kind {
	case KindAuto, KindInt, KindSmallInt, KindBigInt, KindPositiveSmallInt, KindPositiveInt, KindPositiveBigInt:
		switch v := raw.(type) {
		case int64:
			return v, nil
		case int:
			return int64(v), nil
		case int32:
			return int64(v), nil
		case int16:
			return int64(v), nil
		case int8:
			return int64(v), nil
		case uint64:
			return int64(v), nil
		case uint:
			return int64(v), nil
		case uint32:
			return int64(v), nil
		case string:
			n, err := strconv.ParseInt(v, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("orm: Field '%s' expected a number but got '%s'", pkField.Attr, v)
			}
			return n, nil
		default:
			return nil, fmt.Errorf("orm: Field '%s' expected a number but got '%v'", pkField.Attr, raw)
		}
	case KindChar, KindText, KindSlug, KindEmail, KindURL:
		switch v := raw.(type) {
		case string:
			return v, nil
		case []byte:
			return string(v), nil
		default:
			return fmt.Sprint(v), nil
		}
	default:
		return raw, nil
	}
}

// ManyValues returns the related target primary keys for owner on the forward
// many-to-many field f. It is the reflection-friendly entry point used by
// forms and admin widgets that operate without a compile-time target type.
func ManyValues(ctx context.Context, db *DB, owner any, f *Field) ([]any, error) {
	if f == nil || f.Rel == nil || f.Rel.Kind != RelM2M {
		return nil, fmt.Errorf("orm: ManyValues requires an M2M field")
	}
	m, err := newDynamicManyManager(db, owner, f)
	if err != nil {
		return nil, err
	}
	return m.fetchAllTargetPKs(ctx)
}

// SetManyPKs replaces the related target primary keys for owner on the forward
// many-to-many field f, firing m2m_changed signals and honoring ManyOptions
// such as SetClear and ThroughDefaults. It is the reflection-friendly entry
// point used by ModelForm and admin widgets.
func SetManyPKs(ctx context.Context, db *DB, owner any, f *Field, pks []any, opts ...ManyOption) error {
	if f == nil || f.Rel == nil || f.Rel.Kind != RelM2M {
		return fmt.Errorf("orm: SetManyPKs requires an M2M field")
	}
	m, err := newDynamicManyManager(db, owner, f)
	if err != nil {
		return err
	}
	return m.setPKs(ctx, pks, opts...)
}
