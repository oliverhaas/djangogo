package orm

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"

	"github.com/oliverhaas/djangogo/validators"
)

// defaultViolationMessage is the message of a violated constraint that sets
// no ViolationErrorMessage (Django's BaseConstraint.default_violation_error_message).
const defaultViolationMessage = "Constraint “%(name)s” is violated."

// Constraint is a table constraint of Meta.Constraints, a CheckConstraint or
// a UniqueConstraint (Django's BaseConstraint). Both work as a value or as a
// pointer.
type Constraint interface {
	// ConstraintName returns the name of the constraint in the database.
	ConstraintName() string
	// ViolationMessage returns the message of the validation error that
	// Validate reports, with "%(name)s" replaced by the constraint's name.
	ViolationMessage() string
	// Validate checks instance, a registered model of db, against the
	// constraint, as Django's constraint.validate(model, instance, exclude)
	// does. A violation is a *ValidationError. A constraint that refers to a
	// field named in exclude is skipped, and an instance with a set primary
	// key is not compared with its own row.
	Validate(ctx context.Context, db *DB, instance any, exclude ...string) error

	// prepare checks the options of the constraint for model m and returns
	// the copy that m keeps, named name, with its fields as Go names.
	prepare(m *Model, name string) (Constraint, error)
	// validate is Validate for a prepared constraint, with the exclusions
	// normalized to attribute names.
	validate(ctx context.Context, db *DB, m *Model, rv reflect.Value, exclude map[string]bool, adding bool) error
	// ddl returns the SQL of a prepared constraint: a table constraint of
	// CREATE TABLE, or a statement that runs after it. Both are empty when
	// the database cannot enforce the constraint.
	ddl(db *DB, m *Model) (tableDef, stmt string, err error)
}

// CheckConstraint requires every row of the table to meet Condition (Django's
// CheckConstraint). The database checks it on every write, and FullClean
// checks an instance before it is saved. As in SQL, a condition that is
// unknown because of a NULL value is met.
type CheckConstraint struct {
	// Name is the name of the constraint in the database. "%(app_label)s"
	// and "%(class)s" are replaced by the lowercased app label and model name
	// when the model is registered.
	Name string
	// Condition is the condition every row meets. It refers to fields of the
	// model's own table only.
	Condition Cond
	// ViolationErrorCode is the code of the validation error of a violation.
	ViolationErrorCode string
	// ViolationErrorMessage overrides the message of the validation error of
	// a violation. "%(name)s" is replaced by the constraint's name.
	ViolationErrorMessage string
}

// UniqueConstraint requires the values of Fields, or of Expressions, to be
// unique across the rows of the table that meet Condition (Django's
// UniqueConstraint). Rows whose values hold a NULL never conflict, unless
// NullsNotDistinct is set.
type UniqueConstraint struct {
	// Name is the name of the constraint in the database, which takes the
	// placeholders of CheckConstraint.Name.
	Name string
	// Fields lists the fields whose values are unique together. Entries
	// accept Go struct names, attribute names, or column names.
	Fields []string
	// Expressions lists expressions whose values are unique together, such
	// as orm.Lower("name"), in place of Fields.
	Expressions []Expr
	// Condition limits the constraint to the rows that meet it (a partial
	// unique index).
	Condition Cond
	// NullsNotDistinct makes NULL values equal to each other, so that two
	// rows with NULL in the same place conflict (Django's nulls_distinct=False).
	// PostgreSQL 15 enforces it; on SQLite, which cannot, the constraint is
	// validated by FullClean only and not created in the database, as Django
	// does.
	NullsNotDistinct bool
	// ViolationErrorCode is the code of the validation error of a violation.
	ViolationErrorCode string
	// ViolationErrorMessage overrides the message of the validation error of
	// a violation. Without it, a constraint on Fields without a Condition
	// reports the message of a unique field or of unique_together.
	ViolationErrorMessage string
}

// ConstraintName returns the name of the constraint.
func (c CheckConstraint) ConstraintName() string { return c.Name }

// ViolationMessage returns the message of a violation.
func (c CheckConstraint) ViolationMessage() string {
	return violationMessage(c.Name, c.ViolationErrorMessage)
}

// Validate checks instance against the constraint.
func (c CheckConstraint) Validate(ctx context.Context, db *DB, instance any, exclude ...string) error {
	return validateOne(ctx, db, c, c.Name, instance, exclude)
}

// ConstraintName returns the name of the constraint.
func (c UniqueConstraint) ConstraintName() string { return c.Name }

// ViolationMessage returns the message of a violation.
func (c UniqueConstraint) ViolationMessage() string {
	return violationMessage(c.Name, c.ViolationErrorMessage)
}

// Validate checks instance against the constraint.
func (c UniqueConstraint) Validate(ctx context.Context, db *DB, instance any, exclude ...string) error {
	return validateOne(ctx, db, c, c.Name, instance, exclude)
}

// violationMessage returns message, or the default message, with the name
// of the constraint filled in (Django's get_violation_error_message).
func violationMessage(name, message string) string {
	if message == "" {
		message = defaultViolationMessage
	}
	return validators.FormatMessage(message, map[string]any{"name": name})
}

// Constraints returns the constraints of Meta.Constraints, with their names
// and fields resolved as the model registered them.
func (m *Model) Constraints() []Constraint { return slices.Clone(m.constraints) }

// applyConstraints checks Meta.Constraints and stores them on m.
func applyConstraints(m *Model, constraints []Constraint) error {
	seen := make(map[string]bool, len(constraints))
	for _, c := range constraints {
		if c == nil {
			return fmt.Errorf("orm: model %s: Meta.Constraints holds a nil constraint", m.name)
		}
		name := strings.NewReplacer(
			"%(app_label)s", strings.ToLower(m.app),
			"%(class)s", strings.ToLower(m.name),
		).Replace(c.ConstraintName())
		prepared, err := c.prepare(m, name)
		if err != nil {
			return fmt.Errorf("orm: model %s: %w", m.name, err)
		}
		if seen[name] {
			return fmt.Errorf("orm: constraint name '%s' is not unique for model %s", name, m.Label())
		}
		seen[name] = true
		m.constraints = append(m.constraints, prepared)
	}
	return nil
}

// prepare checks the condition of the constraint.
func (c CheckConstraint) prepare(_ *Model, name string) (Constraint, error) {
	if name == "" {
		return nil, errors.New("a check constraint must be named")
	}
	if c.Condition.err != nil {
		return nil, fmt.Errorf("check constraint %q: %w", name, c.Condition.err)
	}
	if c.Condition.empty() {
		return nil, fmt.Errorf("check constraint %q: CheckConstraint.Condition must not be empty", name)
	}
	c.Name = name
	return c, nil
}

// prepare checks the options of the constraint and resolves its fields.
func (c UniqueConstraint) prepare(m *Model, name string) (Constraint, error) {
	switch {
	case name == "":
		return nil, errors.New("A unique constraint must be named.") //nolint:revive,staticcheck // Django's message
	case len(c.Fields) == 0 && len(c.Expressions) == 0:
		return nil, errors.New("At least one field or expression is required to define a unique constraint.") //nolint:revive,staticcheck // Django's message
	case len(c.Fields) > 0 && len(c.Expressions) > 0:
		return nil, errors.New("UniqueConstraint.fields and expressions are mutually exclusive.") //nolint:revive,staticcheck // Django's message
	case c.Condition.err != nil:
		return nil, fmt.Errorf("unique constraint %q: %w", name, c.Condition.err)
	}
	fields := make([]string, len(c.Fields))
	for i, ref := range c.Fields {
		f, ok := resolveMetaField(ref, m.byName, m.byAttr, m.byColumn)
		if !ok {
			return nil, fmt.Errorf("unique constraint %q names unknown field %q", name, ref)
		}
		fields[i] = f.Name
	}
	c.Name = name
	c.Fields = fields
	c.Expressions = slices.Clone(c.Expressions)
	return c, nil
}

// validateOne runs the Validate method of a constraint that model m may not
// declare.
func validateOne(ctx context.Context, db *DB, c Constraint, name string, instance any, exclude []string) error {
	if db == nil {
		return errors.New("orm: Validate requires a non-nil *DB")
	}
	m, ok := db.Registry().ModelOf(instance)
	if !ok {
		return fmt.Errorf("orm: %T is not a registered model", instance)
	}
	rv, err := m.inspectInstance(instance)
	if err != nil {
		return err
	}
	prepared, err := c.prepare(m, name)
	if err != nil {
		return fmt.Errorf("orm: model %s: %w", m.name, err)
	}
	return prepared.validate(ctx, db, m, rv, m.normalizeExclude(exclude), instanceAdding(m, rv, cleanConfig{}))
}

// ValidateConstraints checks instance against the constraints of its model's
// Meta.Constraints and returns a dictionary *ValidationError of the
// violations, as Django's Model.validate_constraints(exclude=...) does. The
// error of a unique constraint on one field is stored under that field, and
// any other under NonFieldErrors. It takes WithExclude and WithAdding.
func ValidateConstraints(ctx context.Context, db *DB, instance any, opts ...CleanOption) error {
	if db == nil {
		return errors.New("orm: ValidateConstraints requires a non-nil *DB")
	}
	m, ok := db.Registry().ModelOf(instance)
	if !ok {
		return fmt.Errorf("orm: %T is not a registered model", instance)
	}
	return m.ValidateConstraints(ctx, db, instance, opts...)
}

// ValidateConstraints checks instance against the constraints of
// Meta.Constraints; see the function ValidateConstraints.
func (m *Model) ValidateConstraints(ctx context.Context, db *DB, instance any, opts ...CleanOption) error {
	cfg := defaultCleanConfig(opts)
	return validateConstraintsInternal(ctx, db, m, instance, m.normalizeExclude(cfg.exclude), cfg)
}

// validateConstraintsInternal checks instance against every constraint of m.
func validateConstraintsInternal(ctx context.Context, db *DB, m *Model, instance any, exclude map[string]bool, cfg cleanConfig) error {
	if len(m.constraints) == 0 {
		return nil
	}
	rv, err := m.inspectInstance(instance)
	if err != nil {
		return err
	}
	adding := instanceAdding(m, rv, cfg)
	errs := map[string][]*ValidationError{}
	for _, c := range m.constraints {
		err := c.validate(ctx, db, m, rv, exclude, adding)
		if err == nil {
			continue
		}
		var ve *ValidationError
		if !errors.As(err, &ve) {
			return err
		}
		if u, ok := c.(UniqueConstraint); ok && ve.Code == "unique" && len(u.Fields) == 1 {
			attr := m.byName[u.Fields[0]].Attr
			errs[attr] = append(errs[attr], ve)
			continue
		}
		errs = ve.UpdateErrorDict(errs)
	}
	if len(errs) > 0 {
		return &ValidationError{Fields: errs}
	}
	return nil
}

// instanceAdding reports whether the instance is a new row: the WithAdding
// option when given, and otherwise whether its primary key is unset.
func instanceAdding(m *Model, rv reflect.Value, cfg cleanConfig) bool {
	if cfg.hasAdding {
		return cfg.adding
	}
	pk := m.PrimaryKey()
	return pk == nil || rv.Field(pk.Index).IsZero()
}

// validate checks the condition against the values of the instance.
func (c CheckConstraint) validate(ctx context.Context, db *DB, m *Model, rv reflect.Value, exclude map[string]bool, _ bool) error {
	skip, err := refsExcluded(m, c.Name, exclude, c.Condition)
	if err != nil || skip {
		return err
	}
	holds, err := holdsFor(ctx, db, m, rv, c.Condition)
	if err != nil {
		return err
	}
	if !holds {
		return NewValidationError(c.ViolationMessage(), c.ViolationErrorCode)
	}
	return nil
}

// validate looks for another row with the values of the instance, as
// Django's UniqueConstraint.validate does.
func (c UniqueConstraint) validate(ctx context.Context, db *DB, m *Model, rv reflect.Value, exclude map[string]bool, adding bool) error {
	q := newQuery(db, m)
	if len(c.Fields) > 0 {
		args := make([]any, 0, 2*len(c.Fields))
		for _, name := range c.Fields {
			f := m.byName[name]
			if exclude[f.Attr] {
				return nil
			}
			fv := rv.Field(f.Index)
			value, isNull, _ := fieldValueState(f, fv.Interface())
			if f.Kind == KindAuto && fv.IsZero() {
				isNull = true
			}
			switch {
			case isNull && !c.NullsNotDistinct:
				// Values that hold a NULL never conflict, since NULL is not
				// equal to NULL in SQL.
				return nil
			case isNull:
				args = append(args, f.Column+lookupSep+lookupIsNull, true)
			default:
				args = append(args, f.Column, value)
			}
		}
		q = q.filter(args, false)
	} else {
		skip, err := refsExcluded(m, c.Name, exclude, Cond{}, c.Expressions...)
		if err != nil || skip {
			return err
		}
		against, err := instanceValues(db.Dialect(), m, rv)
		if err != nil {
			return err
		}
		conds := make([]any, 0, len(c.Expressions))
		for i, e := range c.Expressions {
			rhs, err := resolveAgainst(db, m, against, e)
			if err != nil {
				return err
			}
			// The names hold no "__", which would separate a lookup.
			lhsName := fmt.Sprintf("_constraint_%d", i)
			rhsName := lhsName + "_value"
			q.annotations = append(q.annotations,
				annotation{name: lhsName, expr: e, filters: len(q.filters)},
				annotation{name: rhsName, expr: Expr{kind: exprNode, node: rhs}, filters: len(q.filters)},
			)
			cond := Q(lhsName, F(rhsName))
			if c.NullsNotDistinct {
				cond = cond.Or(Q(lhsName+lookupSep+lookupIsNull, true, rhsName+lookupSep+lookupIsNull, true))
			}
			conds = append(conds, cond)
		}
		q = q.filter(conds, false)
	}
	if pk := m.PrimaryKey(); !adding && pk != nil && !rv.Field(pk.Index).IsZero() {
		q = q.filter([]any{pk.Column, rv.Field(pk.Index).Interface()}, true)
	}
	if c.Condition.empty() {
		exists, err := queryExists(ctx, q)
		if err != nil || !exists {
			return err
		}
		if len(c.Fields) > 0 && c.ViolationErrorMessage == "" {
			attrs := make([]string, len(c.Fields))
			for i, name := range c.Fields {
				attrs[i] = m.byName[name].Attr
			}
			return uniqueErrorMessage(m, attrs)
		}
		return NewValidationError(c.ViolationMessage(), c.ViolationErrorCode)
	}
	skip, err := refsExcluded(m, c.Name, exclude, c.Condition)
	if err != nil || skip {
		return err
	}
	// The instance conflicts when it meets the condition and another row
	// that meets it holds the same values.
	holds, err := holdsFor(ctx, db, m, rv, c.Condition)
	if err != nil || !holds {
		return err
	}
	exists, err := queryExists(ctx, q.filter([]any{c.Condition}, false))
	if err != nil || !exists {
		return err
	}
	return NewValidationError(c.ViolationMessage(), c.ViolationErrorCode)
}

// constraintRef is a field that a constraint refers to: the first part of
// the key as written, and the field of the model's table it resolves to.
type constraintRef struct {
	name  string
	field *Field
}

// constraintRefs returns the fields that cond and exprs refer to. A
// reference to a field of another table is an error: a constraint applies to
// the rows of one table.
func constraintRefs(m *Model, name string, cond Cond, exprs ...Expr) ([]constraintRef, error) {
	var refs []constraintRef
	// add records the field of key, which may end in transforms and, in a
	// condition, a lookup.
	add := func(key string) error {
		p, err := resolvePath(m, strings.Split(key, lookupSep), nil)
		if err != nil {
			return err
		}
		if len(p.hops) > 0 {
			return fmt.Errorf("orm: constraint %q refers to the joined field %q", name, key)
		}
		first, _, _ := strings.Cut(key, lookupSep)
		refs = append(refs, constraintRef{name: first, field: p.field})
		return nil
	}
	var walkExpr func(e *Expr) error
	var walkCond func(c Cond) error
	walkExpr = func(e *Expr) error {
		if e == nil {
			return nil
		}
		switch e.kind {
		case exprField:
			return add(e.key)
		case exprSubquery, exprOuterRef:
			return fmt.Errorf("orm: constraint %q uses a subquery", name)
		}
		for _, sub := range []*Expr{e.left, e.right, e.arg, e.def} {
			if err := walkExpr(sub); err != nil {
				return err
			}
		}
		for i := range e.args {
			if err := walkExpr(&e.args[i]); err != nil {
				return err
			}
		}
		for _, w := range e.whens {
			then := fieldOrExpr(w.Then)
			if err := walkCond(w.Cond); err != nil {
				return err
			}
			if err := walkExpr(&then); err != nil {
				return err
			}
		}
		if e.filter != nil {
			return walkCond(*e.filter)
		}
		return nil
	}
	walkCond = func(c Cond) error {
		if c.err != nil {
			return c.err
		}
		for _, ch := range c.children {
			switch {
			case ch.sub != nil:
				if err := walkCond(*ch.sub); err != nil {
					return err
				}
				continue
			case ch.expr != nil:
				if err := walkExpr(ch.expr); err != nil {
					return err
				}
				continue
			}
			if err := add(ch.key); err != nil {
				return err
			}
			var err error
			switch v := ch.value.(type) {
			case Expr:
				err = walkExpr(&v)
			case *Expr:
				err = walkExpr(v)
			}
			if err != nil {
				return err
			}
		}
		return nil
	}
	if err := walkCond(cond); err != nil {
		return nil, err
	}
	for i := range exprs {
		if err := walkExpr(&exprs[i]); err != nil {
			return nil, err
		}
	}
	return refs, nil
}

// refsExcluded reports whether cond or exprs refer to a field in exclude, which
// makes validation skip the constraint (Django's _expression_refs_exclude).
// "pk" is excluded by name only, as Django's map of field values keeps it.
func refsExcluded(m *Model, name string, exclude map[string]bool, cond Cond, exprs ...Expr) (bool, error) {
	refs, err := constraintRefs(m, name, cond, exprs...)
	if err != nil {
		return false, err
	}
	for _, r := range refs {
		if exclude[r.name] || (r.name != "pk" && exclude[r.field.Attr]) {
			return true, nil
		}
	}
	return false, nil
}

// instanceValue is the value of a field of the instance that a constraint is
// checked against. It takes the place of the field's column, as the field
// values of Django's Q.check and UniqueConstraint.validate do.
type instanceValue struct {
	field *Field
	// value is the bound value, nil for NULL.
	value any
}

// render writes the value as a parameter of the field's column type. A
// dialect that types its parameters by their context needs the cast where
// the value stands alone, as in "$1 IS NULL". SQLite compares a decimal,
// bound as text, as text, unless it is cast to a number (Django's
// SQLiteNumericMixin).
func (v *instanceValue) render(w *sqlWriter) {
	typ := ""
	if _, ok := w.d.(LiteralDialect); ok {
		typ = castType(w.d, v.field)
	} else if v.field.Kind == KindDecimal {
		typ = "NUMERIC"
	}
	if typ == "" {
		w.param(v.value)
		return
	}
	w.write("CAST(")
	w.param(v.value)
	w.write(" AS " + typ + ")")
}

// instanceValues returns the values of the fields of the instance rv.
func instanceValues(d Dialect, m *Model, rv reflect.Value) (map[*Field]*instanceValue, error) {
	out := make(map[*Field]*instanceValue, len(m.fields))
	for _, f := range m.fields {
		fv := rv.Field(f.Index)
		var value any
		switch {
		case f.Kind == KindAuto && fv.IsZero():
			// An auto primary key that is not set yet is NULL.
		case f.Rel != nil:
			if pk, isNull, _ := fieldValueState(f, fv.Interface()); !isNull {
				value = pk
			}
		default:
			value = fv.Interface()
		}
		arg, err := bindField(d, f, value)
		if err != nil {
			return nil, err
		}
		out[f] = &instanceValue{field: f, value: arg}
	}
	return out, nil
}

// againstQuery returns a query on m whose columns of m's own table are the
// values of the instance rv.
func againstQuery(db *DB, m *Model, against map[*Field]*instanceValue) *query {
	q := newQuery(db, m)
	q.against = against
	return q
}

// resolveAgainst resolves e with the values of an instance in place of the
// columns.
func resolveAgainst(db *DB, m *Model, against map[*Field]*instanceValue, e Expr) (sqlNode, error) {
	q := againstQuery(db, m, against)
	c := &compiler{q: q, d: db.Dialect(), joins: newJoinSet(m), annots: map[string]sqlNode{}}
	return c.resolveExpr(e)
}

// holdsFor reports whether the instance rv meets cond, a condition on the
// fields of m. The database evaluates it with the values of the instance in
// place of the columns, and a condition that is unknown because of a NULL
// holds, as it does for a CHECK constraint (Django's Q.check).
func holdsFor(ctx context.Context, db *DB, m *Model, rv reflect.Value, cond Cond) (bool, error) {
	against, err := instanceValues(db.Dialect(), m, rv)
	if err != nil {
		return false, err
	}
	c, err := againstQuery(db, m, against).filter([]any{cond}, false).compile()
	switch {
	case err != nil:
		return false, err
	case c.empty:
		return false, nil
	case c.where == nil && c.having == nil:
		return true, nil
	case c.having != nil:
		return false, errors.New("orm: a constraint condition must not use an aggregate")
	}
	w := newSQLWriter(c.d)
	w.write("SELECT 1 WHERE COALESCE(")
	c.where.render(w)
	w.write(", TRUE)")
	rows, err := db.conn(ctx).QueryContext(ctx, w.String(), w.args...)
	if err != nil {
		return false, fmt.Errorf("orm: check constraint condition on %s: %w", m.Name(), err)
	}
	defer func() { _ = rows.Close() }()
	holds := rows.Next()
	if err := rows.Err(); err != nil {
		return false, fmt.Errorf("orm: check constraint condition on %s: %w", m.Name(), err)
	}
	return holds, nil
}

// ddl returns the table constraint "CONSTRAINT name CHECK (condition)".
func (c CheckConstraint) ddl(db *DB, m *Model) (string, string, error) {
	cond, err := conditionSQL(db, m, c.Name, c.Condition)
	if err != nil {
		return "", "", err
	}
	d := db.Dialect()
	return "CONSTRAINT " + d.Quote(c.Name) + " CHECK (" + cond + ")", "", nil
}

// ddl returns the table constraint "CONSTRAINT name UNIQUE (columns)" of a
// constraint on fields, and otherwise a CREATE UNIQUE INDEX statement, which
// takes expressions, a condition, and NULLS NOT DISTINCT (Django's
// SchemaEditor._unique_sql).
func (c UniqueConstraint) ddl(db *DB, m *Model) (string, string, error) {
	d := db.Dialect()
	if c.NullsNotDistinct && !dialectFeatures(d).UniqueNullsNotDistinct {
		return "", "", nil
	}
	cols := make([]string, 0, len(c.Fields)+len(c.Expressions))
	for _, name := range c.Fields {
		cols = append(cols, d.Quote(m.byName[name].Column))
	}
	if len(c.Expressions) > 0 {
		if _, err := constraintRefs(m, c.Name, Cond{}, c.Expressions...); err != nil {
			return "", "", err
		}
		comp := &compiler{q: newQuery(db, m), d: d, joins: newJoinSet(m), annots: map[string]sqlNode{}}
		for _, e := range c.Expressions {
			n, err := comp.resolveExpr(e)
			if err != nil {
				return "", "", err
			}
			w := newSQLWriter(d)
			w.ddl = true
			n.render(w)
			if w.err != nil {
				return "", "", w.err
			}
			// An expression other than a plain column is written in
			// parentheses, as an index on an expression needs.
			if _, isColumn := n.(colNode); isColumn {
				cols = append(cols, w.String())
			} else {
				cols = append(cols, "("+w.String()+")")
			}
		}
	}
	if c.Condition.empty() && len(c.Expressions) == 0 && !c.NullsNotDistinct {
		return "CONSTRAINT " + d.Quote(c.Name) + " UNIQUE (" + strings.Join(cols, ", ") + ")", "", nil
	}
	stmt := "CREATE UNIQUE INDEX " + d.Quote(c.Name) + " ON " + d.Quote(m.Table()) + " (" + strings.Join(cols, ", ") + ")"
	if c.NullsNotDistinct {
		stmt += " NULLS NOT DISTINCT"
	}
	if !c.Condition.empty() {
		cond, err := conditionSQL(db, m, c.Name, c.Condition)
		if err != nil {
			return "", "", err
		}
		stmt += " WHERE " + cond
	}
	return "", stmt, nil
}

// conditionSQL renders cond, a condition on the fields of m's table, for a
// schema statement: the columns unqualified and the values as literals.
func conditionSQL(db *DB, m *Model, name string, cond Cond) (string, error) {
	if _, err := constraintRefs(m, name, cond); err != nil {
		return "", err
	}
	c, err := newQuery(db, m).filter([]any{cond}, false).compile()
	switch {
	case err != nil:
		return "", err
	case c.empty:
		return "", fmt.Errorf("orm: the condition of constraint %q matches no row", name)
	case c.having != nil:
		return "", errors.New("orm: a constraint condition must not use an aggregate")
	case c.where == nil:
		return "", fmt.Errorf("orm: the condition of constraint %q matches every row", name)
	}
	w := newSQLWriter(c.d)
	w.ddl = true
	c.where.render(w)
	if w.err != nil {
		return "", fmt.Errorf("orm: constraint %q: %w", name, w.err)
	}
	return w.String(), nil
}

// constraintsDDL returns the table constraints of m's Meta.Constraints for
// CREATE TABLE and the statements that create the others after it.
func (db *DB) constraintsDDL(m *Model) (tableDefs, stmts []string, err error) {
	for _, c := range m.constraints {
		def, stmt, err := c.ddl(db, m)
		if err != nil {
			return nil, nil, fmt.Errorf("orm: create table %s: %w", m.Table(), err)
		}
		if def != "" {
			tableDefs = append(tableDefs, def)
		}
		if stmt != "" {
			stmts = append(stmts, stmt)
		}
	}
	return tableDefs, stmts, nil
}
