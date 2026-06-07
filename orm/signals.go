package orm

import (
	"context"
	"reflect"
	"sync"
)

// signalKind identifies a lifecycle moment that handlers can subscribe to.
type signalKind uint8

const (
	// preSave fires before a row is created.
	preSave signalKind = iota
	// postSave fires after a row is created and its PK written back.
	postSave
	// preDelete fires before a matching row is deleted.
	preDelete
	// postDelete fires after a matching row is deleted.
	postDelete
)

// handlerEntry pairs a registered handler with the id used to cancel it.
type handlerEntry struct {
	id int64
	fn func(ctx context.Context, obj any) error
}

// signalRegistry is the process-global, mutex-guarded store of signal handlers
// keyed by model type and signal kind.
type signalRegistry struct {
	mu       sync.RWMutex
	nextID   int64
	handlers map[reflect.Type]map[signalKind][]handlerEntry
}

// signals is the single process-global signal registry.
var signals = &signalRegistry{
	handlers: make(map[reflect.Type]map[signalKind][]handlerEntry),
}

// register stores fn for the given type and kind and returns a cancel func that
// removes exactly that registration.
func (r *signalRegistry) register(t reflect.Type, kind signalKind, fn func(ctx context.Context, obj any) error) func() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.nextID++
	id := r.nextID
	byKind := r.handlers[t]
	if byKind == nil {
		byKind = make(map[signalKind][]handlerEntry)
		r.handlers[t] = byKind
	}
	byKind[kind] = append(byKind[kind], handlerEntry{id: id, fn: fn})

	return func() { r.remove(t, kind, id) }
}

// remove deletes the handler with the given id for the type and kind.
func (r *signalRegistry) remove(t reflect.Type, kind signalKind, id int64) {
	r.mu.Lock()
	defer r.mu.Unlock()

	byKind, ok := r.handlers[t]
	if !ok {
		return
	}
	entries := byKind[kind]
	for i, e := range entries {
		if e.id == id {
			byKind[kind] = append(entries[:i:i], entries[i+1:]...)
			break
		}
	}
}

// snapshot returns a copy of the handlers registered for the type and kind so
// firing can run without holding the lock.
func (r *signalRegistry) snapshot(t reflect.Type, kind signalKind) []handlerEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()

	entries := r.handlers[t][kind]
	if len(entries) == 0 {
		return nil
	}
	out := make([]handlerEntry, len(entries))
	copy(out, entries)
	return out
}

// has reports whether any handler is registered for the type and kind.
func (r *signalRegistry) has(t reflect.Type, kind signalKind) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.handlers[t][kind]) > 0
}

// onSignal is the shared registration path for the typed On* helpers. It wraps
// the typed fn so it can be stored as func(ctx, any) error.
func onSignal[T any](kind signalKind, fn func(ctx context.Context, obj *T) error) func() {
	wrapped := func(ctx context.Context, obj any) error {
		return fn(ctx, obj.(*T))
	}
	return signals.register(reflect.TypeFor[T](), kind, wrapped)
}

// OnPreSave registers a handler fired before a T is created. The handler may
// mutate *obj before the INSERT. It returns a cancel func that unregisters it.
func OnPreSave[T any](fn func(ctx context.Context, obj *T) error) (cancel func()) {
	return onSignal(preSave, fn)
}

// OnPostSave registers a handler fired after a T is created and its primary key
// written back. It returns a cancel func that unregisters it.
func OnPostSave[T any](fn func(ctx context.Context, obj *T) error) (cancel func()) {
	return onSignal(postSave, fn)
}

// OnPreDelete registers a handler fired before a matching T is deleted. It
// returns a cancel func that unregisters it.
func OnPreDelete[T any](fn func(ctx context.Context, obj *T) error) (cancel func()) {
	return onSignal(preDelete, fn)
}

// OnPostDelete registers a handler fired after a matching T is deleted. It
// returns a cancel func that unregisters it.
func OnPostDelete[T any](fn func(ctx context.Context, obj *T) error) (cancel func()) {
	return onSignal(postDelete, fn)
}

// fire runs every handler registered for T and kind in registration order,
// returning the first handler error.
func fire[T any](ctx context.Context, kind signalKind, obj *T) error {
	for _, e := range signals.snapshot(reflect.TypeFor[T](), kind) {
		if err := e.fn(ctx, obj); err != nil {
			return err
		}
	}
	return nil
}

// firePreSave fires PreSave handlers for obj. db is accepted for symmetry with
// future handlers that may need the DB handle and to keep call sites uniform.
func firePreSave[T any](ctx context.Context, _ *DB, obj *T) error {
	return fire(ctx, preSave, obj)
}

// firePostSave fires PostSave handlers for obj.
func firePostSave[T any](ctx context.Context, _ *DB, obj *T) error {
	return fire(ctx, postSave, obj)
}

// firePreDelete fires PreDelete handlers for obj.
func firePreDelete[T any](ctx context.Context, _ *DB, obj *T) error {
	return fire(ctx, preDelete, obj)
}

// firePostDelete fires PostDelete handlers for obj.
func firePostDelete[T any](ctx context.Context, _ *DB, obj *T) error {
	return fire(ctx, postDelete, obj)
}

// hasDeleteHandlers reports whether any PreDelete or PostDelete handler is
// registered for T, letting Delete choose the fetch-and-fire path only when
// needed.
func hasDeleteHandlers[T any]() bool {
	t := reflect.TypeFor[T]()
	return signals.has(t, preDelete) || signals.has(t, postDelete)
}
