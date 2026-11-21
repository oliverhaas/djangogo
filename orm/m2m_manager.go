package orm

import (
	"context"
	"errors"
	"fmt"
	"reflect"
)

// m2mCoreFilter restricts a QuerySet to rows linked to one owner through an
// M2M intermediate table.
type m2mCoreFilter struct {
	through     *Model
	targetFKCol string
	sourceFKCol string
	sourceField *Field
	ownerPK     any
}

// manyConfig holds resolved options for M2M mutations.
type manyConfig struct {
	throughDefaults Defaults
	clear           bool
}

// ManyOption configures M2M operations such as AddWith, Set, SetPKs, and Create.
type ManyOption func(*manyConfig)

// ThroughDefaults supplies extra field values when inserting rows into an
// explicit M2M through model, mirroring Django's through_defaults argument.
// Callable values (func() any) are evaluated once per operation.
func ThroughDefaults(defaults Defaults) ManyOption {
	return func(c *manyConfig) { c.throughDefaults = defaults }
}

// SetClear makes Set and SetPKs clear all existing associations before adding
// the new ones, mirroring Django's set(..., clear=True).
func SetClear() ManyOption {
	return func(c *manyConfig) { c.clear = true }
}

// parseManyConfig applies opts into a manyConfig.
func parseManyConfig(opts []ManyOption) manyConfig {
	var cfg manyConfig
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	return cfg
}

// manyCore holds the non-generic state shared by ManyManager[T] and the
// reflection helpers ManyValues / SetManyPKs.
type manyCore struct {
	db             *DB
	ownerModel     *Model
	ownerInstance  any // pointer to owner struct
	ownerPK        any
	declaringModel *Model
	field          *Field
	targetModel    *Model
	through        *Model
	sourceFK       *Field // FK on through pointing to ownerModel
	targetFK       *Field // FK on through pointing to targetModel
	reverse        bool
	symmetrical    bool
	err            error
}

// newManyCore builds a manyCore by resolving forward or reverse M2M name on owner.
func newManyCore(db *DB, owner any, name string, wantTarget reflect.Type) manyCore {
	core := manyCore{db: db}
	ownerModel, ownerVal, ownerPtr, err := inspectOwner(db.Registry(), owner)
	if err != nil {
		core.err = err
		return core
	}
	if !ownerPtr.IsValid() {
		cp := reflect.New(ownerModel.goType)
		cp.Elem().Set(ownerVal)
		ownerPtr = cp
	}

	var (
		declaring *Model
		field     *Field
		target    *Model
		sourceFK  *Field
		targetFK  *Field
		reverse   bool
	)

	if f, ok := ownerModel.ManyToManyField(name); ok && f.Rel != nil && f.Rel.Target != nil && f.Rel.Through != nil {
		declaring = ownerModel
		field = f
		target = f.Rel.Target
		sourceFK = f.Rel.SourceField
		targetFK = f.Rel.TargetField
		reverse = false
	} else {
		rev, found, ambiguous := ownerModel.lookupReverseAccessor(name)
		if ambiguous {
			core.err = fmt.Errorf("orm: reverse relation %s.%s is ambiguous; set related_name to disambiguate", ownerModel.Name(), name)
			return core
		}
		if !found || rev.Kind != RelM2M || rev.Field == nil || rev.Field.Rel == nil || rev.Field.Rel.Through == nil {
			core.err = &FieldError{
				Model:   ownerModel.Name(),
				Message: "cannot resolve many-to-many relation '" + name + "'",
			}
			return core
		}
		declaring = rev.Source
		field = rev.Field
		target = rev.Source
		sourceFK = rev.Field.Rel.TargetField
		targetFK = rev.Field.Rel.SourceField
		reverse = true
	}

	if wantTarget != nil && wantTarget != target.goType {
		core.err = fmt.Errorf("orm: many-to-many relation %s.%s targets %s, got %s", ownerModel.Name(), name, target.Name(), wantTarget)
		return core
	}

	ownerPK, zero, err := extractModelPK(ownerModel, ownerVal)
	if err != nil {
		core.err = err
		return core
	}
	if zero {
		core.err = fmt.Errorf(
			"orm: '%s' instance needs to have a primary key value before this many-to-many relationship can be used",
			ownerModel.Name(),
		)
		return core
	}

	core.ownerModel = ownerModel
	core.ownerInstance = ownerPtr.Interface()
	core.ownerPK = ownerPK
	core.declaringModel = declaring
	core.field = field
	core.targetModel = target
	core.through = field.Rel.Through
	core.sourceFK = sourceFK
	core.targetFK = targetFK
	core.reverse = reverse
	core.symmetrical = field.Rel.Symmetrical
	return core
}

// newDynamicManyManager builds a manyCore directly from a forward M2M *Field.
func newDynamicManyManager(db *DB, owner any, f *Field) (*manyCore, error) {
	core := newManyCore(db, owner, f.Attr, nil)
	if core.err != nil {
		return nil, core.err
	}
	return &core, nil
}

// ManyManager manages one side (forward or reverse) of a many-to-many relation
// for a single owner instance, mirroring Django's ManyRelatedManager. Construct
// one with Many.
type ManyManager[T any] struct {
	core manyCore
}

// Many returns a ManyManager for the many-to-many relation named name on owner.
// On the forward side, name is the M2M field's attribute or Go struct name
// (e.g. "publications" or "Publications"). On the reverse side, name is the
// related_name tag or "<source>_set" by default.
//
// Any resolution error (unknown relation, wrong T type, or unsaved owner) is
// recorded on the manager and reported when a query or mutation executes.
func Many[T any](db *DB, owner any, name string) *ManyManager[T] {
	return &ManyManager[T]{core: newManyCore(db, owner, name, reflect.TypeFor[T]())}
}

// QuerySet returns a QuerySet of T rows currently related to the owner.
func (m *ManyManager[T]) QuerySet() *QuerySet[T] {
	if m.core.err != nil {
		var zero T
		model, _ := m.core.db.Registry().ModelOf(zero)
		q := newQuery(m.core.db, model)
		q.setErr(m.core.err)
		return &QuerySet[T]{q: q}
	}
	qs := Query[T](m.core.db)
	cp := qs.q.clone()
	cp.m2mFilter = &m2mCoreFilter{
		through:     m.core.through,
		targetFKCol: m.core.targetFK.Column,
		sourceFKCol: m.core.sourceFK.Column,
		sourceField: m.core.sourceFK,
		ownerPK:     m.core.ownerPK,
	}
	return &QuerySet[T]{q: cp}
}

// All returns every T row related to the owner.
func (m *ManyManager[T]) All(ctx context.Context) ([]T, error) {
	return m.QuerySet().All(ctx)
}

// Filter returns a QuerySet of related T rows further restricted by args.
func (m *ManyManager[T]) Filter(args ...any) *QuerySet[T] {
	return m.QuerySet().Filter(args...)
}

// Exclude returns a QuerySet of related T rows excluding rows matching args.
func (m *ManyManager[T]) Exclude(args ...any) *QuerySet[T] {
	return m.QuerySet().Exclude(args...)
}

// OrderBy returns a QuerySet of related T rows ordered by fields.
func (m *ManyManager[T]) OrderBy(fields ...string) *QuerySet[T] {
	return m.QuerySet().OrderBy(fields...)
}

// Get returns the single related T row matching args.
func (m *ManyManager[T]) Get(ctx context.Context, args ...any) (T, error) {
	return m.QuerySet().Get(ctx, args...)
}

// First returns the first related T row.
func (m *ManyManager[T]) First(ctx context.Context) (T, error) {
	return m.QuerySet().First(ctx)
}

// Count returns the number of related T rows, querying the through table
// directly without joining the target table (matching Django's optimization).
func (m *ManyManager[T]) Count(ctx context.Context) (int64, error) {
	if m.core.err != nil {
		return 0, m.core.err
	}
	d := m.core.db.dialect
	w := newSQLWriter(d)
	w.write("SELECT COUNT(*) FROM ")
	w.ident(m.core.through.Table())
	w.write(" WHERE ")
	w.ident(m.core.sourceFK.Column)
	w.write(" = ")
	arg, err := bindField(d, m.core.sourceFK, m.core.ownerPK)
	if err != nil {
		return 0, err
	}
	w.param(arg)
	if m.core.targetFK.Null {
		w.write(" AND ")
		w.ident(m.core.targetFK.Column)
		w.write(" IS NOT NULL")
	}
	var count int64
	if err := m.core.db.conn(ctx).QueryRowContext(ctx, w.String(), w.args...).Scan(&count); err != nil {
		return 0, fmt.Errorf("orm: M2M count %s.%s: %w", m.core.ownerModel.Name(), m.core.field.Attr, err)
	}
	return count, nil
}

// Exists reports whether any T row is related to the owner, querying the
// through table directly without joining the target table.
func (m *ManyManager[T]) Exists(ctx context.Context) (bool, error) {
	if m.core.err != nil {
		return false, m.core.err
	}
	d := m.core.db.dialect
	w := newSQLWriter(d)
	w.write("SELECT EXISTS(SELECT 1 FROM ")
	w.ident(m.core.through.Table())
	w.write(" WHERE ")
	w.ident(m.core.sourceFK.Column)
	w.write(" = ")
	arg, err := bindField(d, m.core.sourceFK, m.core.ownerPK)
	if err != nil {
		return false, err
	}
	w.param(arg)
	if m.core.targetFK.Null {
		w.write(" AND ")
		w.ident(m.core.targetFK.Column)
		w.write(" IS NOT NULL")
	}
	w.write(" LIMIT 1)")
	var exists bool
	if err := m.core.db.conn(ctx).QueryRowContext(ctx, w.String(), w.args...).Scan(&exists); err != nil {
		return false, fmt.Errorf("orm: M2M exists %s.%s: %w", m.core.ownerModel.Name(), m.core.field.Attr, err)
	}
	return exists, nil
}

// Add associates saved T instances with the owner.
func (m *ManyManager[T]) Add(ctx context.Context, objs ...*T) error {
	return m.AddWith(ctx, objs)
}

// AddWith associates saved T instances with the owner, applying opts such as
// ThroughDefaults for explicit through models.
func (m *ManyManager[T]) AddWith(ctx context.Context, objs []*T, opts ...ManyOption) error {
	if m.core.err != nil {
		return m.core.err
	}
	pks, err := extractObjPKs(m.core.targetModel, objs)
	if err != nil {
		return err
	}
	return m.core.addPKs(ctx, pks, opts...)
}

// AddPKs associates targets identified by primary key values with the owner.
func (m *ManyManager[T]) AddPKs(ctx context.Context, pks ...any) error {
	return m.core.addPKs(ctx, pks)
}

// AddPKsWith associates targets identified by primary key values with the
// owner, applying opts such as ThroughDefaults.
func (m *ManyManager[T]) AddPKsWith(ctx context.Context, pks []any, opts ...ManyOption) error {
	return m.core.addPKs(ctx, pks, opts...)
}

// Remove disassociates the given T instances from the owner.
func (m *ManyManager[T]) Remove(ctx context.Context, objs ...*T) error {
	if m.core.err != nil {
		return m.core.err
	}
	pks, err := extractObjPKs(m.core.targetModel, objs)
	if err != nil {
		return err
	}
	return m.core.removePKs(ctx, pks)
}

// RemovePKs disassociates targets identified by primary key values from the owner.
func (m *ManyManager[T]) RemovePKs(ctx context.Context, pks ...any) error {
	return m.core.removePKs(ctx, pks)
}

// Clear removes all associations for the owner.
func (m *ManyManager[T]) Clear(ctx context.Context) error {
	return m.core.clear(ctx)
}

// Set replaces the owner's related T set with objs. By default it diffs the
// existing set against objs (removing absent items and adding missing ones);
// pass SetClear() to clear all existing associations before adding objs.
func (m *ManyManager[T]) Set(ctx context.Context, objs []*T, opts ...ManyOption) error {
	if m.core.err != nil {
		return m.core.err
	}
	pks, err := extractObjPKs(m.core.targetModel, objs)
	if err != nil {
		return err
	}
	return m.core.setPKs(ctx, pks, opts...)
}

// SetPKs replaces the owner's related T set with the targets identified by pks.
func (m *ManyManager[T]) SetPKs(ctx context.Context, pks []any, opts ...ManyOption) error {
	return m.core.setPKs(ctx, pks, opts...)
}

// Create inserts obj as a new T row and associates it with the owner in a
// single transaction. Pass ThroughDefaults(...) for explicit through models.
func (m *ManyManager[T]) Create(ctx context.Context, obj *T, opts ...ManyOption) error {
	if m.core.err != nil {
		return m.core.err
	}
	if obj == nil {
		return fmt.Errorf("orm: M2M Create requires a non-nil object")
	}
	return runInTx(ctx, m.core.db, func(txCtx context.Context) error {
		if err := Query[T](m.core.db).Create(txCtx, obj); err != nil {
			return err
		}
		return m.AddWith(txCtx, []*T{obj}, opts...)
	})
}

// GetOrCreate finds a related T row matching args, or creates and associates
// one if none exists. Any ManyOption values in args (such as ThroughDefaults)
// are extracted and applied when adding the association.
func (m *ManyManager[T]) GetOrCreate(ctx context.Context, defaults Defaults, args ...any) (T, bool, error) {
	var zero T
	if m.core.err != nil {
		return zero, false, m.core.err
	}
	filterArgs, opts := splitManyOptions(args)
	var (
		obj     T
		created bool
	)
	err := runInTx(ctx, m.core.db, func(txCtx context.Context) error {
		existing, err := m.QuerySet().Get(txCtx, filterArgs...)
		if err == nil {
			obj = existing
			return nil
		}
		if !errors.Is(err, ErrDoesNotExist) {
			return err
		}
		var fresh T
		if err := Query[T](m.core.db).populate(&fresh, filterArgs, defaults); err != nil {
			return err
		}
		if err := Query[T](m.core.db).Create(txCtx, &fresh); err != nil {
			return err
		}
		if err := m.AddWith(txCtx, []*T{&fresh}, opts...); err != nil {
			return err
		}
		obj = fresh
		created = true
		return nil
	})
	if err != nil {
		return zero, false, err
	}
	return obj, created, nil
}

// UpdateOrCreate updates the related T row matching args with defaults, or
// creates and associates one if none exists. Any ManyOption values in args
// (such as ThroughDefaults) are applied when creating the association.
func (m *ManyManager[T]) UpdateOrCreate(ctx context.Context, defaults Defaults, args ...any) (T, bool, error) {
	var zero T
	if m.core.err != nil {
		return zero, false, m.core.err
	}
	filterArgs, opts := splitManyOptions(args)
	var (
		obj     T
		created bool
	)
	err := runInTx(ctx, m.core.db, func(txCtx context.Context) error {
		got, isNew, err := m.GetOrCreate(txCtx, defaults, append(filterArgs, toAnyOptions(opts)...)...)
		if err != nil {
			return err
		}
		obj, created = got, isNew
		if created {
			return nil
		}
		model := m.core.targetModel
		keys := sortedKeys(defaults)
		elem := reflect.ValueOf(&obj).Elem()
		for _, key := range keys {
			if err := setAttr(m.core.db.Registry(), model, elem, key, defaults[key]); err != nil {
				return err
			}
		}
		for _, f := range model.fields {
			if f.AutoNow {
				keys = append(keys, f.Attr)
			}
		}
		if len(keys) == 0 {
			return nil
		}
		return Query[T](m.core.db).Save(txCtx, &obj, UpdateFields(keys...))
	})
	if err != nil {
		return zero, false, err
	}
	return obj, created, nil
}

// splitManyOptions separates ManyOption values from QuerySet filter args.
func splitManyOptions(args []any) (filterArgs []any, opts []ManyOption) {
	for _, a := range args {
		if opt, ok := a.(ManyOption); ok {
			opts = append(opts, opt)
		} else {
			filterArgs = append(filterArgs, a)
		}
	}
	return filterArgs, opts
}

// toAnyOptions wraps opts as []any for forwarding to variadic functions.
func toAnyOptions(opts []ManyOption) []any {
	out := make([]any, len(opts))
	for i, o := range opts {
		out[i] = o
	}
	return out
}

// extractObjPKs extracts primary keys from saved *T pointers.
func extractObjPKs[T any](model *Model, objs []*T) ([]any, error) {
	pks := make([]any, len(objs))
	for i, obj := range objs {
		if obj == nil {
			return nil, fmt.Errorf("orm: M2M operation requires non-nil %s instances", model.Name())
		}
		pk, zero, err := extractModelPK(model, reflect.ValueOf(obj).Elem())
		if err != nil {
			return nil, err
		}
		if zero {
			return nil, fmt.Errorf("orm: %s instance isn't saved; save it before adding to an M2M relation", model.Name())
		}
		pks[i] = pk
	}
	return pks, nil
}
