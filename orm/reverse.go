package orm

import (
	"context"
	"fmt"
	"reflect"
	"slices"
	"strings"
)

// ReverseRelation describes the reverse side of a forward FK or O2O relation
// attached to its target model by Registry.Resolve.
type ReverseRelation struct {
	// Name is the reverse accessor name on the target model (Django's
	// accessor_name): related_name if set, otherwise "<model>_set" for FK or
	// "<model>" for O2O (with <model> the lowercased source model name).
	Name string
	// QueryName is the lookup name used in Filter, Exclude, OrderBy, Values,
	// and SelectRelated (Django's related_query_name): related_query_name if
	// set, else related_name if set, else the lowercased source model name.
	QueryName string
	// Kind is the forward relation's kind (RelFK or RelO2O).
	Kind RelKind
	// Source is the model that declares the forward relation field.
	Source *Model
	// Field is the forward relation field on Source.
	Field *Field
	// Hidden reports whether reverse access was suppressed with a related_name
	// ending in "+" (e.g. related_name=+ or related_name=hidden+).
	Hidden bool
}

// ReverseRelations returns a shallow copy of the non-hidden reverse relations
// targeting m, in registration and field-declaration order.
func (m *Model) ReverseRelations() []*ReverseRelation {
	cp := make([]*ReverseRelation, len(m.reverseRels))
	copy(cp, m.reverseRels)
	return cp
}

// ReverseRelation looks up a non-hidden reverse relation targeting m by its
// accessor name (e.g. "article_set", "cities", or "restaurant"). It returns
// false when no unique non-hidden reverse relation carries that name.
func (m *Model) ReverseRelation(name string) (*ReverseRelation, bool) {
	if m.ambiguousReverseName[name] {
		return nil, false
	}
	rev, ok := m.byReverseName[name]
	return rev, ok
}

// lookupReverseAccessor looks up a non-hidden reverse relation by accessor
// name and reports whether the name is ambiguous across multiple incoming FKs.
func (m *Model) lookupReverseAccessor(name string) (rev *ReverseRelation, found, ambiguous bool) {
	if m.ambiguousReverseName[name] {
		return nil, false, true
	}
	rev, ok := m.byReverseName[name]
	return rev, ok, false
}

// lookupReverseQuery looks up a reverse relation by its query name (Django's
// related_query_name) for use in filter/order/values/select_related paths.
func (m *Model) lookupReverseQuery(name string) (*ReverseRelation, bool) {
	if m.ambiguousReverseQuery[name] {
		return nil, false
	}
	rev, ok := m.byReverseQuery[name]
	return rev, ok
}

// reverseForField returns the non-hidden reverse relation on m that
// corresponds to forward field f, or nil if none exists.
func (m *Model) reverseForField(f *Field) *ReverseRelation {
	for _, rev := range m.reverseRels {
		if rev.Field == f {
			return rev
		}
	}
	return nil
}

// resolveReverseRelations populates reverse relation metadata on every target
// model in r. Existing reverse metadata is cleared first so Resolve remains
// idempotent.
func (r *Registry) resolveReverseRelations() {
	for _, m := range r.models {
		m.reverseRels = nil
		m.byReverseName = make(map[string]*ReverseRelation)
		m.byReverseQuery = make(map[string]*ReverseRelation)
		m.ambiguousReverseName = make(map[string]bool)
		m.ambiguousReverseQuery = make(map[string]bool)
	}
	for _, src := range r.models {
		lowerSrc := strings.ToLower(src.name)
		registerReverse := func(f *Field) {
			if f.Rel == nil || f.Rel.Target == nil {
				return
			}
			// Symmetrical self-referential M2M fields expose no reverse accessor
			// or reverse query name, matching Django's hidden internal reverse.
			if f.Rel.Kind == RelM2M && f.Rel.Symmetrical {
				return
			}
			target := f.Rel.Target
			hidden := strings.HasSuffix(f.Rel.RelatedName, "+")

			accessorName := f.Rel.RelatedName
			if accessorName == "" {
				if f.Rel.Kind == RelO2O {
					accessorName = lowerSrc
				} else {
					accessorName = lowerSrc + "_set"
				}
			}
			queryName := f.Rel.RelatedQueryName
			if queryName == "" {
				if f.Rel.RelatedName != "" {
					queryName = f.Rel.RelatedName
				} else {
					queryName = lowerSrc
				}
			}

			rev := &ReverseRelation{
				Name:      accessorName,
				QueryName: queryName,
				Kind:      f.Rel.Kind,
				Source:    src,
				Field:     f,
				Hidden:    hidden,
			}

			if !hidden {
				target.reverseRels = append(target.reverseRels, rev)
				if _, dup := target.byReverseName[accessorName]; dup || target.ambiguousReverseName[accessorName] {
					delete(target.byReverseName, accessorName)
					target.ambiguousReverseName[accessorName] = true
				} else {
					target.byReverseName[accessorName] = rev
				}
			}

			if !hidden || f.Rel.RelatedQueryName != "" {
				if _, dup := target.byReverseQuery[queryName]; dup || target.ambiguousReverseQuery[queryName] {
					delete(target.byReverseQuery, queryName)
					target.ambiguousReverseQuery[queryName] = true
				} else {
					target.byReverseQuery[queryName] = rev
				}
			}
		}
		for _, f := range src.fields {
			registerReverse(f)
		}
		for _, f := range src.m2mFields {
			registerReverse(f)
		}
	}
}

// inspectOwner resolves owner (a struct value or non-nil pointer to a struct)
// to its registered *Model, struct reflect.Value, and pointer reflect.Value
// (invalid when owner was passed by value).
func inspectOwner(reg *Registry, owner any) (*Model, reflect.Value, reflect.Value, error) {
	if owner == nil {
		return nil, reflect.Value{}, reflect.Value{}, fmt.Errorf("orm: related owner must not be nil")
	}
	rv := reflect.ValueOf(owner)
	var ptrVal reflect.Value
	if rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return nil, reflect.Value{}, reflect.Value{}, fmt.Errorf("orm: related owner must not be a nil pointer")
		}
		ptrVal = rv
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return nil, reflect.Value{}, reflect.Value{}, fmt.Errorf("orm: related owner must be a model struct or pointer, got %T", owner)
	}
	m, ok := reg.ModelOf(owner)
	if !ok {
		return nil, reflect.Value{}, reflect.Value{}, fmt.Errorf("orm: no model registered for %T", owner)
	}
	return m, rv, ptrVal, nil
}

// lookupReverseForChild finds the reverse relation name on ownerModel and
// verifies that its kind matches wantKind (when non-zero) and its source model
// matches Child.
func lookupReverseForChild[Child any](ownerModel *Model, name string, wantKind RelKind) (*ReverseRelation, error) {
	rev, found, ambiguous := ownerModel.lookupReverseAccessor(name)
	if ambiguous {
		return nil, fmt.Errorf("orm: reverse relation %s.%s is ambiguous; set related_name to disambiguate", ownerModel.Name(), name)
	}
	if !found {
		return nil, &FieldError{
			Model:   ownerModel.Name(),
			Message: "cannot resolve reverse relation '" + name + "'",
		}
	}
	if wantKind != RelNone && rev.Kind != wantKind {
		return nil, fmt.Errorf("orm: reverse relation %s.%s has kind %d, want %d", ownerModel.Name(), name, rev.Kind, wantKind)
	}
	childType := reflect.TypeFor[Child]()
	if childType != rev.Source.goType {
		return nil, fmt.Errorf("orm: reverse relation %s.%s targets %s, got %s", ownerModel.Name(), name, rev.Source.Name(), childType)
	}
	return rev, nil
}

// Related returns a QuerySet of Child rows related to owner via the reverse
// FK relation, forward/reverse M2M relation or generic relation named name
// (the field attribute, related_name tag, or "<child>_set" by default). The
// related_query_name of a generic relation names the rows of the model that
// declares it whose relation holds owner.
//
// owner may be a model struct value or pointer. Any lookup error (unknown or
// hidden relation name, wrong Child type, or an unsaved owner with a zero
// primary key) is recorded on the returned QuerySet and reported when it runs.
func Related[Child any](db *DB, owner any, name string) *QuerySet[Child] {
	if owner != nil {
		if ownerModel, ok := db.Registry().ModelOf(owner); ok {
			if _, isM2M := ownerModel.ManyToManyField(name); isM2M {
				return Many[Child](db, owner, name).QuerySet()
			}
			if rev, found, _ := ownerModel.lookupReverseAccessor(name); found && rev.Kind == RelM2M {
				return Many[Child](db, owner, name).QuerySet()
			}
			if _, isGeneric := ownerModel.GenericRelation(name); isGeneric {
				return GenericSet[Child](db, owner, name).QuerySet()
			}
			if gr, ok := ownerModel.byGenericQuery[name]; ok {
				return genericOwners[Child](db, owner, gr)
			}
		}
	}
	return RelatedSet[Child](db, owner, name).QuerySet()
}

// RelatedManager manages the reverse foreign-key set of Child rows belonging
// to one owner instance, mirroring Django's RelatedManager (all, filter,
// create, add, remove, clear, set). Construct one with RelatedSet.
type RelatedManager[Child any] struct {
	db         *DB
	ownerModel *Model
	ownerVal   reflect.Value
	ownerPtr   reflect.Value
	ownerPK    int64
	rev        *ReverseRelation
	err        error
}

// RelatedSet returns a RelatedManager for the reverse foreign-key relation
// named name on owner (the related_name tag, or "<child>_set" by default).
func RelatedSet[Child any](db *DB, owner any, name string) *RelatedManager[Child] {
	mgr := &RelatedManager[Child]{db: db}
	ownerModel, ownerVal, ownerPtr, err := inspectOwner(db.Registry(), owner)
	if err != nil {
		mgr.err = err
		return mgr
	}
	rev, err := lookupReverseForChild[Child](ownerModel, name, RelNone)
	if err != nil {
		mgr.err = err
		return mgr
	}
	pk, err := extractModelPKInt64(ownerModel, ownerVal)
	if err != nil {
		mgr.err = err
		return mgr
	}
	if pk == 0 {
		mgr.err = fmt.Errorf(
			"orm: '%s' instance needs to have a primary key value before this relationship can be used",
			ownerModel.Name(),
		)
		return mgr
	}
	mgr.ownerModel = ownerModel
	mgr.ownerVal = ownerVal
	mgr.ownerPtr = ownerPtr
	mgr.ownerPK = pk
	mgr.rev = rev
	return mgr
}

// clearPrefetch removes any cached prefetch result for this relation on the
// manager's owner instance.
func (m *RelatedManager[Child]) clearPrefetch() {
	if m.ownerModel != nil && m.rev != nil && m.ownerVal.IsValid() {
		clearPrefetchListCache(m.ownerModel, m.ownerVal, m.rev.Name)
	}
}

// QuerySet returns a QuerySet restricted to Child rows pointing to the owner.
// When the owner has a prefetched result cached for this relation, terminal
// reads on the returned QuerySet (All, Count, Exists, First, Last) read from
// the cache without querying; any further chaining (Filter, Exclude, OrderBy,
// etc.) clears the cache and queries the database.
func (m *RelatedManager[Child]) QuerySet() *QuerySet[Child] {
	if m.err != nil {
		var zero Child
		model, _ := m.db.Registry().ModelOf(zero)
		q := newQuery(m.db, model)
		q.setErr(m.err)
		return &QuerySet[Child]{q: q}
	}
	qs := Query[Child](m.db).Filter(m.rev.Field.Column, m.ownerPK)
	if items, _, ok := getPrefetchListCache(m.ownerModel, m.ownerVal, m.rev.Name); ok {
		qs.q.prefetchedList = items
	}
	return qs
}

// All returns every Child row belonging to the owner.
func (m *RelatedManager[Child]) All(ctx context.Context) ([]Child, error) {
	return m.QuerySet().All(ctx)
}

// Filter returns a QuerySet of related Child rows further restricted by args.
func (m *RelatedManager[Child]) Filter(args ...any) *QuerySet[Child] {
	return m.QuerySet().Filter(args...)
}

// Exclude returns a QuerySet of related Child rows excluding rows matching args.
func (m *RelatedManager[Child]) Exclude(args ...any) *QuerySet[Child] {
	return m.QuerySet().Exclude(args...)
}

// OrderBy returns a QuerySet of related Child rows ordered by terms; see
// QuerySet.OrderBy.
func (m *RelatedManager[Child]) OrderBy(terms ...any) *QuerySet[Child] {
	return m.QuerySet().OrderBy(terms...)
}

// Get returns the single related Child row matching args.
func (m *RelatedManager[Child]) Get(ctx context.Context, args ...any) (Child, error) {
	return m.QuerySet().Get(ctx, args...)
}

// First returns the first related Child row.
func (m *RelatedManager[Child]) First(ctx context.Context) (Child, error) {
	return m.QuerySet().First(ctx)
}

// Count returns the number of related Child rows.
func (m *RelatedManager[Child]) Count(ctx context.Context) (int64, error) {
	return m.QuerySet().Count(ctx)
}

// Exists reports whether any related Child row exists.
func (m *RelatedManager[Child]) Exists(ctx context.Context) (bool, error) {
	return m.QuerySet().Exists(ctx)
}

// Create sets obj's foreign key to the owner and inserts obj.
func (m *RelatedManager[Child]) Create(ctx context.Context, obj *Child) error {
	if m.err != nil {
		return m.err
	}
	if obj == nil {
		return fmt.Errorf("orm: related Create requires a non-nil object")
	}
	elem := reflect.ValueOf(obj).Elem()
	if m.ownerPtr.IsValid() {
		setForwardObject(elem, m.rev.Field, m.ownerPtr, m.ownerPK)
	} else {
		setForwardPK(elem, m.rev.Field, m.ownerPK)
	}
	if err := Query[Child](m.db).Create(ctx, obj); err != nil {
		return err
	}
	m.clearPrefetch()
	return nil
}

// GetOrCreate returns the related Child row matching args, or creates it with
// its foreign key set to the owner, like Django's get_or_create() on a
// related manager. It works as QuerySet.GetOrCreate.
func (m *RelatedManager[Child]) GetOrCreate(ctx context.Context, defaults Defaults, args ...any) (Child, bool, error) {
	if m.err != nil {
		var zero Child
		return zero, false, m.err
	}
	m.clearPrefetch()
	return Query[Child](m.db).GetOrCreate(ctx, defaults, m.ownerArgs(args)...)
}

// UpdateOrCreate updates the related Child row matching args with defaults,
// or creates it with its foreign key set to the owner, like Django's
// update_or_create() on a related manager. It works as
// QuerySet.UpdateOrCreate, CreateDefaults included.
func (m *RelatedManager[Child]) UpdateOrCreate(ctx context.Context, defaults Defaults, args ...any) (Child, bool, error) {
	if m.err != nil {
		var zero Child
		return zero, false, m.err
	}
	m.clearPrefetch()
	return Query[Child](m.db).UpdateOrCreate(ctx, defaults, m.ownerArgs(args)...)
}

// ownerArgs returns the conditions args followed by the condition on the
// foreign key to the owner, which also sets the key of a created row. The
// caller's slice is left unchanged.
func (m *RelatedManager[Child]) ownerArgs(args []any) []any {
	return append(slices.Clip(args), m.rev.Field.Attr, m.ownerPK)
}

// Add associates saved Child objects with the owner in a single UPDATE, like
// Django's RelatedManager.add(bulk=True). Passing an unsaved object (zero
// primary key) returns an error; use AddSave or save the object first.
func (m *RelatedManager[Child]) Add(ctx context.Context, objs ...*Child) error {
	if m.err != nil {
		return m.err
	}
	if len(objs) == 0 {
		return nil
	}
	pks, err := collectSavedPKs(m.rev.Source, objs)
	if err != nil {
		return err
	}
	if _, err := Query[Child](m.db).Filter("pk__in", pks).Update(ctx, m.rev.Field.Column, m.ownerPK); err != nil {
		return err
	}
	for _, obj := range objs {
		elem := reflect.ValueOf(obj).Elem()
		if m.ownerPtr.IsValid() {
			setForwardObject(elem, m.rev.Field, m.ownerPtr, m.ownerPK)
		} else {
			setForwardPK(elem, m.rev.Field, m.ownerPK)
		}
	}
	m.clearPrefetch()
	return nil
}

// AddSave associates each Child object with the owner and saves it
// individually, allowing unsaved objects (Django's add(..., bulk=False)).
func (m *RelatedManager[Child]) AddSave(ctx context.Context, objs ...*Child) error {
	if m.err != nil {
		return m.err
	}
	for _, obj := range objs {
		if obj == nil {
			return fmt.Errorf("orm: related AddSave requires non-nil objects")
		}
		elem := reflect.ValueOf(obj).Elem()
		if m.ownerPtr.IsValid() {
			setForwardObject(elem, m.rev.Field, m.ownerPtr, m.ownerPK)
		} else {
			setForwardPK(elem, m.rev.Field, m.ownerPK)
		}
		if err := Query[Child](m.db).Save(ctx, obj); err != nil {
			return err
		}
	}
	if len(objs) > 0 {
		m.clearPrefetch()
	}
	return nil
}

// Remove disassociates the given Child objects from the owner by setting their
// foreign key to NULL. It returns an error when the foreign key is not
// nullable.
func (m *RelatedManager[Child]) Remove(ctx context.Context, objs ...*Child) error {
	if m.err != nil {
		return m.err
	}
	if !m.rev.Field.Null {
		return fmt.Errorf("orm: cannot remove from non-nullable foreign key %s.%s", m.rev.Source.Name(), m.rev.Field.Attr)
	}
	if len(objs) == 0 {
		return nil
	}
	pks, err := collectSavedPKs(m.rev.Source, objs)
	if err != nil {
		return err
	}
	if _, err := Query[Child](m.db).Filter(m.rev.Field.Column, m.ownerPK, "pk__in", pks).Update(ctx, m.rev.Field.Column, nil); err != nil {
		return err
	}
	for _, obj := range objs {
		setForwardPK(reflect.ValueOf(obj).Elem(), m.rev.Field, 0)
	}
	m.clearPrefetch()
	return nil
}

// Clear disassociates all Child rows belonging to the owner by setting their
// foreign key to NULL. It returns an error when the foreign key is not
// nullable.
func (m *RelatedManager[Child]) Clear(ctx context.Context) error {
	if m.err != nil {
		return m.err
	}
	if !m.rev.Field.Null {
		return fmt.Errorf("orm: cannot clear non-nullable foreign key %s.%s", m.rev.Source.Name(), m.rev.Field.Attr)
	}
	if _, err := Query[Child](m.db).Filter(m.rev.Field.Column, m.ownerPK).Update(ctx, m.rev.Field.Column, nil); err != nil {
		return err
	}
	m.clearPrefetch()
	return nil
}

// Set replaces the set of Child objects associated with the owner, matching
// Django's RelatedManager.set(). For a nullable foreign key, existing members
// not in objs have their foreign key set to NULL before objs are added. For a
// non-nullable foreign key, existing members cannot be nulled and remain while
// objs are moved to the owner.
func (m *RelatedManager[Child]) Set(ctx context.Context, objs []*Child) error {
	if m.err != nil {
		return m.err
	}
	pks, err := collectSavedPKs(m.rev.Source, objs)
	if err != nil {
		return err
	}
	err = m.db.Atomic(ctx, func(txCtx context.Context) error {
		if m.rev.Field.Null {
			qs := Query[Child](m.db).Filter(m.rev.Field.Column, m.ownerPK)
			if len(pks) > 0 {
				qs = qs.Exclude("pk__in", pks)
			}
			if _, err := qs.Update(txCtx, m.rev.Field.Column, nil); err != nil {
				return err
			}
		}
		if len(objs) > 0 {
			return m.Add(txCtx, objs...)
		}
		return nil
	}, WithoutSavepoint())
	if err != nil {
		return err
	}
	m.clearPrefetch()
	return nil
}

// collectSavedPKs extracts the primary keys of objs and errors if any element
// is nil or unsaved (zero primary key).
func collectSavedPKs[Child any](model *Model, objs []*Child) ([]int64, error) {
	pks := make([]int64, len(objs))
	for i, obj := range objs {
		if obj == nil {
			return nil, fmt.Errorf("orm: related operation requires non-nil %s instances", model.Name())
		}
		pk, err := extractModelPKInt64(model, reflect.ValueOf(obj).Elem())
		if err != nil {
			return nil, err
		}
		if pk == 0 {
			return nil, fmt.Errorf(
				"orm: %s instance isn't saved: use AddSave or save the object first",
				model.Name(),
			)
		}
		pks[i] = pk
	}
	return pks, nil
}
