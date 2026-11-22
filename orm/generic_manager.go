package orm

import (
	"context"
	"errors"
	"fmt"
	"reflect"
)

// GenericRelatedManager manages the rows of a generic relation of one owner
// instance, Django's GenericRelatedObjectManager: the rows of Child whose
// content type is the owner's and whose object id is the owner's primary
// key. Construct one with GenericSet.
type GenericRelatedManager[Child any] struct {
	db         *DB
	gr         *GenericRelation
	ownerModel *Model
	ownerVal   reflect.Value
	ownerPK    any
	err        error
}

// GenericSet returns the manager of the generic relation named name of owner,
// a saved model instance (Django's instance.<name>). Errors are reported when
// the manager is used.
func GenericSet[Child any](db *DB, owner any, name string) *GenericRelatedManager[Child] {
	mgr := &GenericRelatedManager[Child]{db: db}
	ownerModel, ownerVal, _, err := inspectOwner(db.Registry(), owner)
	if err != nil {
		mgr.err = err
		return mgr
	}
	gr, ok := ownerModel.GenericRelation(name)
	if !ok || gr.Related == nil {
		mgr.err = &FieldError{Model: ownerModel.Name(), Message: "cannot resolve generic relation '" + name + "'"}
		return mgr
	}
	if childType := reflect.TypeFor[Child](); childType != gr.Related.goType {
		mgr.err = fmt.Errorf("orm: generic relation %s.%s relates %s, got %s",
			ownerModel.Name(), name, gr.Related.Name(), childType)
		return mgr
	}
	pk, zero, err := extractModelPK(ownerModel, ownerVal)
	if err != nil {
		mgr.err = err
		return mgr
	}
	if zero {
		mgr.err = fmt.Errorf(
			"orm: '%s' instance needs to have a primary key value before this relationship can be used",
			ownerModel.Name())
		return mgr
	}
	mgr.gr, mgr.ownerModel, mgr.ownerVal, mgr.ownerPK = gr, ownerModel, ownerVal, pk
	return mgr
}

// QuerySet returns the rows of the relation. A result that PrefetchRelated
// cached on the owner serves All, Count, Exists, First and Last until the
// QuerySet is refined.
func (m *GenericRelatedManager[Child]) QuerySet() *QuerySet[Child] {
	if m.err != nil {
		var zero Child
		model, _ := m.db.Registry().ModelOf(zero)
		q := newQuery(m.db, model)
		q.setErr(m.err)
		return &QuerySet[Child]{q: q}
	}
	qs := &QuerySet[Child]{q: newQuery(m.db, m.gr.Related).genericFilter(m.gr, []any{m.ownerPK})}
	if items, _, ok := getPrefetchListCache(m.ownerModel, m.ownerVal, m.gr.Name); ok {
		qs.q.prefetchedList = items
	}
	return qs
}

// clearPrefetch drops the result PrefetchRelated cached on the owner.
func (m *GenericRelatedManager[Child]) clearPrefetch() {
	clearPrefetchListCache(m.ownerModel, m.ownerVal, m.gr.Name)
}

// All returns the rows of the relation.
func (m *GenericRelatedManager[Child]) All(ctx context.Context) ([]Child, error) {
	return m.QuerySet().All(ctx)
}

// Filter returns the rows of the relation that match args.
func (m *GenericRelatedManager[Child]) Filter(args ...any) *QuerySet[Child] {
	return m.QuerySet().Filter(args...)
}

// Exclude returns the rows of the relation that do not match args.
func (m *GenericRelatedManager[Child]) Exclude(args ...any) *QuerySet[Child] {
	return m.QuerySet().Exclude(args...)
}

// Get returns the single row of the relation that matches args.
func (m *GenericRelatedManager[Child]) Get(ctx context.Context, args ...any) (Child, error) {
	return m.QuerySet().Get(ctx, args...)
}

// Count returns the number of rows of the relation.
func (m *GenericRelatedManager[Child]) Count(ctx context.Context) (int64, error) {
	return m.QuerySet().Count(ctx)
}

// Exists reports whether the relation has any row.
func (m *GenericRelatedManager[Child]) Exists(ctx context.Context) (bool, error) {
	return m.QuerySet().Exists(ctx)
}

// contentTypeID returns the primary key of the owner's content type.
func (m *GenericRelatedManager[Child]) contentTypeID(ctx context.Context) (int64, error) {
	id, err := m.gr.impl.ContentTypeID(ctx, m.db, m.ownerModel)
	if err != nil {
		return 0, err
	}
	n, err := toInt64(id)
	if err != nil {
		return 0, fmt.Errorf("orm: content type id %v of %s: %w", id, m.ownerModel.Name(), err)
	}
	return n, nil
}

// link sets the content type and object id of obj to the owner's.
func (m *GenericRelatedManager[Child]) link(ctx context.Context, obj *Child, ctID int64) error {
	if obj == nil {
		return fmt.Errorf("orm: generic related operation requires non-nil %s instances", m.gr.Related.Name())
	}
	elem := reflect.ValueOf(obj).Elem()
	setForwardPK(elem, m.gr.ContentType, ctID)
	return setAttr(ctx, m.db, m.gr.Related, elem, m.gr.ObjectID.Attr, m.ownerPK)
}

// Create saves obj as a new row of the relation.
func (m *GenericRelatedManager[Child]) Create(ctx context.Context, obj *Child) error {
	if m.err != nil {
		return m.err
	}
	if obj == nil {
		return errors.New("orm: generic related Create requires a non-nil object")
	}
	m.clearPrefetch()
	ctID, err := m.contentTypeID(ctx)
	if err != nil {
		return err
	}
	if err := m.link(ctx, obj, ctID); err != nil {
		return err
	}
	return Query[Child](m.db).Create(ctx, obj)
}

// Add moves saved rows into the relation with one UPDATE, Django's
// add(bulk=True). An unsaved object is an error; AddSave saves each object
// instead.
func (m *GenericRelatedManager[Child]) Add(ctx context.Context, objs ...*Child) error {
	if m.err != nil {
		return m.err
	}
	m.clearPrefetch()
	if len(objs) == 0 {
		return nil
	}
	pks, err := collectSavedPKs(m.gr.Related, objs)
	if err != nil {
		return err
	}
	ctID, err := m.contentTypeID(ctx)
	if err != nil {
		return err
	}
	for _, obj := range objs {
		if err := m.link(ctx, obj, ctID); err != nil {
			return err
		}
	}
	_, err = Query[Child](m.db).Filter("pk__in", pks).Update(ctx,
		m.gr.ContentType.Column, ctID, m.gr.ObjectID.Column, m.ownerPK)
	return err
}

// AddSave moves each object into the relation and saves it, Django's
// add(bulk=False). Unsaved objects are inserted.
func (m *GenericRelatedManager[Child]) AddSave(ctx context.Context, objs ...*Child) error {
	if m.err != nil {
		return m.err
	}
	m.clearPrefetch()
	if len(objs) == 0 {
		return nil
	}
	ctID, err := m.contentTypeID(ctx)
	if err != nil {
		return err
	}
	for _, obj := range objs {
		if err := m.link(ctx, obj, ctID); err != nil {
			return err
		}
		if err := Query[Child](m.db).Save(ctx, obj); err != nil {
			return err
		}
	}
	return nil
}

// Remove deletes the given rows of the relation. A generic relation has no
// column to set to NULL, so Django deletes the rows.
func (m *GenericRelatedManager[Child]) Remove(ctx context.Context, objs ...*Child) error {
	if m.err != nil {
		return m.err
	}
	m.clearPrefetch()
	if len(objs) == 0 {
		return nil
	}
	pks, err := collectSavedPKs(m.gr.Related, objs)
	if err != nil {
		return err
	}
	_, err = m.QuerySet().Filter("pk__in", pks).Delete(ctx)
	return err
}

// Clear deletes every row of the relation.
func (m *GenericRelatedManager[Child]) Clear(ctx context.Context) error {
	if m.err != nil {
		return m.err
	}
	m.clearPrefetch()
	_, err := m.QuerySet().Delete(ctx)
	return err
}

// Set makes objs the rows of the relation, Django's set(): it deletes the
// rows that are not in objs and adds the objects that are not rows of the
// relation yet. With SetClear it deletes every row first and adds all of
// objs. Bulk(false) adds through AddSave instead of Add, which also accepts
// unsaved objects.
func (m *GenericRelatedManager[Child]) Set(ctx context.Context, objs []*Child, opts ...ManyOption) error {
	if m.err != nil {
		return m.err
	}
	cfg := parseManyConfig(opts)
	add := m.Add
	if cfg.noBulk {
		add = m.AddSave
	}
	return m.db.Atomic(ctx, func(ctx context.Context) error {
		if cfg.clear {
			if err := m.Clear(ctx); err != nil {
				return err
			}
			return add(ctx, objs...)
		}
		m.clearPrefetch()
		current, err := m.QuerySet().q.pks(ctx)
		if err != nil {
			return err
		}
		old := make(map[int64]bool, len(current))
		for _, pk := range current {
			n, err := toInt64(pk)
			if err != nil {
				return err
			}
			old[n] = true
		}
		var fresh []*Child
		for _, obj := range objs {
			if obj == nil {
				return fmt.Errorf("orm: generic related Set requires non-nil %s instances", m.gr.Related.Name())
			}
			pk, err := extractModelPKInt64(m.gr.Related, reflect.ValueOf(obj).Elem())
			if err != nil {
				return err
			}
			if pk != 0 && old[pk] {
				delete(old, pk)
				continue
			}
			fresh = append(fresh, obj)
		}
		if len(old) > 0 {
			stale := make([]int64, 0, len(old))
			for pk := range old {
				stale = append(stale, pk)
			}
			if _, err := m.QuerySet().Filter("pk__in", stale).Delete(ctx); err != nil {
				return err
			}
		}
		return add(ctx, fresh...)
	}, WithoutSavepoint())
}

// GetOrCreate returns the row of the relation that matches args, or creates
// it in the relation, Django's get_or_create() on the manager.
func (m *GenericRelatedManager[Child]) GetOrCreate(ctx context.Context, defaults Defaults, args ...any) (Child, bool, error) {
	var zero Child
	if m.err != nil {
		return zero, false, m.err
	}
	ctID, err := m.contentTypeID(ctx)
	if err != nil {
		return zero, false, err
	}
	m.clearPrefetch()
	args = append(args, m.gr.ContentType.Attr, ctID, m.gr.ObjectID.Attr, m.ownerPK)
	return Query[Child](m.db).GetOrCreate(ctx, defaults, args...)
}

// UpdateOrCreate updates the row of the relation that matches args with
// defaults, or creates it in the relation, Django's update_or_create() on the
// manager.
func (m *GenericRelatedManager[Child]) UpdateOrCreate(ctx context.Context, defaults Defaults, args ...any) (Child, bool, error) {
	var zero Child
	if m.err != nil {
		return zero, false, m.err
	}
	ctID, err := m.contentTypeID(ctx)
	if err != nil {
		return zero, false, err
	}
	m.clearPrefetch()
	args = append(args, m.gr.ContentType.Attr, ctID, m.gr.ObjectID.Attr, m.ownerPK)
	return Query[Child](m.db).UpdateOrCreate(ctx, defaults, args...)
}

// genericOwners returns the rows of gr.Model whose generic relation gr holds
// owner, an instance of gr.Related: Django's accessor named by the
// related_query_name of a GenericRelation. A result that PrefetchRelated
// cached on owner serves it until the QuerySet is refined.
func genericOwners[Child any](db *DB, owner any, gr *GenericRelation) *QuerySet[Child] {
	q := newQuery(db, gr.Model)
	ownerModel, ownerVal, _, err := inspectOwner(db.Registry(), owner)
	if err == nil && reflect.TypeFor[Child]() != gr.Model.goType {
		err = fmt.Errorf("orm: %s.%s relates %s, got %s",
			ownerModel.Name(), gr.QueryName, gr.Model.Name(), reflect.TypeFor[Child]())
	}
	var pk any
	if err == nil {
		var zero bool
		pk, zero, err = extractModelPK(ownerModel, ownerVal)
		if err == nil && zero {
			err = fmt.Errorf(
				"orm: '%s' instance needs to have a primary key value before this relationship can be used",
				ownerModel.Name())
		}
	}
	if err != nil {
		q.setErr(err)
		return &QuerySet[Child]{q: q}
	}
	qs := &QuerySet[Child]{q: q.filter([]any{gr.Name + "__pk", pk}, false)}
	if items, _, ok := getPrefetchListCache(ownerModel, ownerVal, gr.QueryName); ok {
		qs.q.prefetchedList = items
	}
	return qs
}
