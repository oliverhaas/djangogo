package orm

import (
	"context"
	"reflect"
	"slices"
	"sync"
)

// M2MAction identifies the lifecycle phase of an m2m_changed signal,
// matching Django's action argument values.
type M2MAction string

const (
	// M2MPreAdd fires before rows are inserted into the M2M through table.
	M2MPreAdd M2MAction = "pre_add"
	// M2MPostAdd fires after rows are inserted into the M2M through table.
	M2MPostAdd M2MAction = "post_add"
	// M2MPreRemove fires before selected rows are deleted from the M2M through table.
	M2MPreRemove M2MAction = "pre_remove"
	// M2MPostRemove fires after selected rows are deleted from the M2M through table.
	M2MPostRemove M2MAction = "post_remove"
	// M2MPreClear fires before all rows for an instance are deleted from the through table.
	M2MPreClear M2MAction = "pre_clear"
	// M2MPostClear fires after all rows for an instance are deleted from the through table.
	M2MPostClear M2MAction = "post_clear"
)

// M2MChangedEvent carries the arguments of Django's m2m_changed signal.
type M2MChangedEvent struct {
	// Sender is the intermediate (through) model whose rows are changing.
	Sender *Model
	// Instance is a pointer to the model instance whose relation is modified.
	Instance any
	// Action is one of M2MPreAdd, M2MPostAdd, M2MPreRemove, M2MPostRemove,
	// M2MPreClear, or M2MPostClear.
	Action M2MAction
	// Reverse is false when modifying from the forward M2M side and true when
	// modifying from the reverse side.
	Reverse bool
	// Model is the *Model of the related objects being added, removed, or cleared.
	Model *Model
	// PKSet holds the deduplicated primary keys being added or removed. It is
	// nil for M2MPreClear and M2MPostClear, and a non-nil empty slice when an
	// Add call inserts no new rows.
	PKSet []any
}

// m2mFieldKey identifies a declared M2M field by declaring struct type and
// snake_case attribute name.
type m2mFieldKey struct {
	ownerType reflect.Type
	attr      string
}

// m2mHandlerEntry pairs a registered m2m_changed callback with its unique id.
type m2mHandlerEntry struct {
	id int64
	fn func(ctx context.Context, ev M2MChangedEvent) error
}

// m2mSignalRegistry stores process-global m2m_changed signal listeners.
type m2mSignalRegistry struct {
	mu             sync.RWMutex
	nextID         int64
	byField        map[m2mFieldKey][]m2mHandlerEntry
	byOwnerType    map[reflect.Type][]m2mHandlerEntry
	byThroughType  map[reflect.Type][]m2mHandlerEntry
	byThroughModel map[*Model][]m2mHandlerEntry
	global         []m2mHandlerEntry
}

// m2mSignals is the single process-global registry for m2m_changed handlers.
var m2mSignals = &m2mSignalRegistry{
	byField:        make(map[m2mFieldKey][]m2mHandlerEntry),
	byOwnerType:    make(map[reflect.Type][]m2mHandlerEntry),
	byThroughType:  make(map[reflect.Type][]m2mHandlerEntry),
	byThroughModel: make(map[*Model][]m2mHandlerEntry),
}

// OnM2MChanged registers a handler fired on m2m_changed events for the M2M
// field named fieldName (attribute name or Go struct field name) declared on T.
// When fieldName is empty, the handler receives events for all M2M fields
// declared on T (or all M2M relations when T is any). It returns a cancel func
// that unregisters the handler.
func OnM2MChanged[T any](fieldName string, fn func(ctx context.Context, ev M2MChangedEvent) error) (cancel func()) {
	t := reflect.TypeFor[T]()
	m2mSignals.mu.Lock()
	defer m2mSignals.mu.Unlock()

	m2mSignals.nextID++
	id := m2mSignals.nextID
	entry := m2mHandlerEntry{id: id, fn: fn}

	if t.Kind() == reflect.Interface {
		m2mSignals.global = append(m2mSignals.global, entry)
		return func() { m2mSignals.removeGlobal(id) }
	}
	if fieldName == "" {
		m2mSignals.byOwnerType[t] = append(m2mSignals.byOwnerType[t], entry)
		return func() { m2mSignals.removeOwnerType(t, id) }
	}
	key := m2mFieldKey{ownerType: t, attr: toSnakeCase(fieldName)}
	m2mSignals.byField[key] = append(m2mSignals.byField[key], entry)
	return func() { m2mSignals.removeField(key, id) }
}

// OnM2MChangedThrough registers a handler fired on m2m_changed events whose
// explicit intermediate (through) model has Go type Through, mirroring Django's
// m2m_changed.connect(receiver, sender=Through). It returns a cancel func.
func OnM2MChangedThrough[Through any](fn func(ctx context.Context, ev M2MChangedEvent) error) (cancel func()) {
	t := reflect.TypeFor[Through]()
	m2mSignals.mu.Lock()
	defer m2mSignals.mu.Unlock()

	m2mSignals.nextID++
	id := m2mSignals.nextID
	entry := m2mHandlerEntry{id: id, fn: fn}
	if t.Kind() == reflect.Interface {
		m2mSignals.global = append(m2mSignals.global, entry)
		return func() { m2mSignals.removeGlobal(id) }
	}
	m2mSignals.byThroughType[t] = append(m2mSignals.byThroughType[t], entry)
	return func() { m2mSignals.removeThroughType(t, id) }
}

// OnM2MChangedModel registers a handler fired on m2m_changed events for the
// given intermediate (through) *Model, or for all M2M relations when through is
// nil. It returns a cancel func that unregisters the handler.
func OnM2MChangedModel(through *Model, fn func(ctx context.Context, ev M2MChangedEvent) error) (cancel func()) {
	m2mSignals.mu.Lock()
	defer m2mSignals.mu.Unlock()

	m2mSignals.nextID++
	id := m2mSignals.nextID
	entry := m2mHandlerEntry{id: id, fn: fn}
	if through == nil {
		m2mSignals.global = append(m2mSignals.global, entry)
		return func() { m2mSignals.removeGlobal(id) }
	}
	m2mSignals.byThroughModel[through] = append(m2mSignals.byThroughModel[through], entry)
	return func() { m2mSignals.removeThroughModel(through, id) }
}

// deleteM2MEntry removes the entry with the given id from slice.
func deleteM2MEntry(entries []m2mHandlerEntry, id int64) []m2mHandlerEntry {
	for i, e := range entries {
		if e.id == id {
			return append(entries[:i:i], entries[i+1:]...)
		}
	}
	return entries
}

// removeGlobal removes a global m2m_changed listener by id.
func (r *m2mSignalRegistry) removeGlobal(id int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.global = deleteM2MEntry(r.global, id)
}

// removeOwnerType removes an owner-type listener by id.
func (r *m2mSignalRegistry) removeOwnerType(t reflect.Type, id int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byOwnerType[t] = deleteM2MEntry(r.byOwnerType[t], id)
}

// removeField removes a field-scoped listener by id.
func (r *m2mSignalRegistry) removeField(key m2mFieldKey, id int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byField[key] = deleteM2MEntry(r.byField[key], id)
}

// removeThroughType removes a through-type listener by id.
func (r *m2mSignalRegistry) removeThroughType(t reflect.Type, id int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byThroughType[t] = deleteM2MEntry(r.byThroughType[t], id)
}

// removeThroughModel removes a through-model listener by id.
func (r *m2mSignalRegistry) removeThroughModel(m *Model, id int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byThroughModel[m] = deleteM2MEntry(r.byThroughModel[m], id)
}

// has reports whether any m2m_changed listener matches the given relation.
func (r *m2mSignalRegistry) has(declaring *Model, field *Field, through *Model) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if len(r.global) > 0 {
		return true
	}
	if declaring != nil && declaring.goType != nil {
		if len(r.byOwnerType[declaring.goType]) > 0 {
			return true
		}
		if field != nil && len(r.byField[m2mFieldKey{ownerType: declaring.goType, attr: field.Attr}]) > 0 {
			return true
		}
	}
	if through != nil {
		if len(r.byThroughModel[through]) > 0 {
			return true
		}
		if through.goType != nil && len(r.byThroughType[through.goType]) > 0 {
			return true
		}
	}
	return false
}

// snapshot returns all matching listeners sorted by registration order.
func (r *m2mSignalRegistry) snapshot(declaring *Model, field *Field, through *Model) []m2mHandlerEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var out []m2mHandlerEntry
	out = append(out, r.global...)
	if declaring != nil && declaring.goType != nil {
		out = append(out, r.byOwnerType[declaring.goType]...)
		if field != nil {
			out = append(out, r.byField[m2mFieldKey{ownerType: declaring.goType, attr: field.Attr}]...)
		}
	}
	if through != nil {
		out = append(out, r.byThroughModel[through]...)
		if through.goType != nil {
			out = append(out, r.byThroughType[through.goType]...)
		}
	}
	if len(out) > 1 {
		slices.SortFunc(out, func(a, b m2mHandlerEntry) int {
			switch {
			case a.id < b.id:
				return -1
			case a.id > b.id:
				return 1
			default:
				return 0
			}
		})
	}
	return out
}

// fireM2MChanged dispatches ev to every matching m2m_changed listener in
// registration order, returning the first handler error.
func fireM2MChanged(ctx context.Context, declaring *Model, field *Field, ev M2MChangedEvent) error {
	for _, entry := range m2mSignals.snapshot(declaring, field, ev.Sender) {
		if err := entry.fn(ctx, ev); err != nil {
			return err
		}
	}
	return nil
}
