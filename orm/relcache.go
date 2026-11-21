package orm

import (
	"reflect"
	"sort"
)

// RelCache is an optional embedded struct for models that participate in
// cached reverse relations and PrefetchRelated results (reverse O2O, reverse
// FK, M2M, and custom to_attr prefetches). Models without RelCache still
// support all queries, reverse lookups, and forward FK/O2O prefetching into
// FK.SetObject / O2O.SetObject; embedding RelCache allows repeated lookups,
// SelectRelated on reverse one-to-one relations, and PrefetchRelated on
// reverse and many-to-many relations to read from memory without extra queries.
//
// RelCache is skipped by the field parser and adds no database column. Copying
// a model value copies its cache pointer, so a value returned by QuerySet.Get
// or QuerySet.All retains its populated cache.
type RelCache struct {
	cache *relCache
}

// relCacheType is the reflect.Type of RelCache, recognized by the registry.
var relCacheType = reflect.TypeOf(RelCache{})

// relCacheHolder is satisfied by any model struct (or pointer to one) that
// embeds RelCache, allowing package-level cache readers to inspect RelCache
// without requiring a Registry lookup.
type relCacheHolder interface {
	relCachePtr() *relCache
}

// relCachePtr returns the underlying cache pointer.
func (r RelCache) relCachePtr() *relCache { return r.cache }

// relCache holds cached relation entries for one model instance.
type relCache struct {
	ones  map[string]relOneEntry
	lists map[string]relListEntry
}

// relOneEntry records one cached single-object lookup: either a pointer to the
// related struct or missing=true when no related row exists.
type relOneEntry struct {
	obj     any
	model   *Model
	missing bool
}

// relListEntry records one cached multi-valued prefetch result: a typed slice
// []Child and its target *Model.
type relListEntry struct {
	items any
	model *Model
}

// ClearRelCache removes all cached relation entries from this instance.
func (r *RelCache) ClearRelCache() {
	if r == nil || r.cache == nil {
		return
	}
	clear(r.cache.ones)
	clear(r.cache.lists)
}

// ClearRelation removes the cached relation entry for name from this instance,
// forcing the next lookup to query the database.
func (r *RelCache) ClearRelation(name string) {
	if r == nil || r.cache == nil {
		return
	}
	delete(r.cache.ones, name)
	delete(r.cache.lists, name)
}

// getRelCache returns the relCache of struct value sv for model m. When create
// is true and sv is settable, a nil cache is allocated and stored into sv.
func getRelCache(m *Model, sv reflect.Value, create bool) *relCache {
	if m == nil || m.cacheIndex < 0 {
		return nil
	}
	fv := sv.Field(m.cacheIndex)
	rc, ok := fv.Interface().(RelCache)
	if !ok {
		return nil
	}
	if rc.cache == nil && create && fv.CanSet() {
		c := &relCache{
			ones:  make(map[string]relOneEntry),
			lists: make(map[string]relListEntry),
		}
		fv.Set(reflect.ValueOf(RelCache{cache: c}))
		return c
	}
	return rc.cache
}

// cacheFromAny extracts the *relCache from a model struct value or pointer
// without requiring a Registry.
func cacheFromAny(owner any) *relCache {
	if owner == nil {
		return nil
	}
	rv := reflect.ValueOf(owner)
	if rv.Kind() == reflect.Pointer && rv.IsNil() {
		return nil
	}
	if h, ok := owner.(relCacheHolder); ok {
		return h.relCachePtr()
	}
	return nil
}

// setReverseOneCacheModel stores a single-object cache entry together with its
// target *Model on sv if m embeds RelCache.
func setReverseOneCacheModel(m *Model, sv reflect.Value, name string, obj any, target *Model, missing bool) {
	c := getRelCache(m, sv, true)
	if c == nil {
		return
	}
	if c.ones == nil {
		c.ones = make(map[string]relOneEntry)
	}
	c.ones[name] = relOneEntry{obj: obj, model: target, missing: missing}
}

// getReverseOneCache looks up a cached reverse O2O entry on sv.
func getReverseOneCache(m *Model, sv reflect.Value, name string) (obj any, missing, ok bool) {
	c := getRelCache(m, sv, false)
	if c == nil || c.ones == nil {
		return nil, false, false
	}
	e, found := c.ones[name]
	if !found {
		return nil, false, false
	}
	return e.obj, e.missing, true
}

// getReverseOneCacheEntry returns the full relOneEntry for name on sv.
func getReverseOneCacheEntry(m *Model, sv reflect.Value, name string) (relOneEntry, bool) {
	c := getRelCache(m, sv, false)
	if c == nil || c.ones == nil {
		return relOneEntry{}, false
	}
	e, found := c.ones[name]
	return e, found
}

// setPrefetchListCache stores a prefetched multi-valued slice ([]Child) and
// its target *Model on sv if m embeds RelCache.
func setPrefetchListCache(m *Model, sv reflect.Value, name string, items any, target *Model) {
	c := getRelCache(m, sv, true)
	if c == nil {
		return
	}
	if c.lists == nil {
		c.lists = make(map[string]relListEntry)
	}
	c.lists[name] = relListEntry{items: items, model: target}
}

// getPrefetchListCache looks up a cached multi-valued prefetch entry on sv.
func getPrefetchListCache(m *Model, sv reflect.Value, name string) (items any, target *Model, ok bool) {
	c := getRelCache(m, sv, false)
	if c == nil || c.lists == nil {
		return nil, nil, false
	}
	e, found := c.lists[name]
	if !found {
		return nil, nil, false
	}
	return e.items, e.model, true
}

// clearPrefetchListCache deletes the cached multi-valued prefetch entry for
// name on sv.
func clearPrefetchListCache(m *Model, sv reflect.Value, name string) {
	c := getRelCache(m, sv, false)
	if c == nil || c.lists == nil {
		return
	}
	delete(c.lists, name)
}

// hasRelCacheKey reports whether name is present in either the ones or lists
// cache of sv.
func hasRelCacheKey(m *Model, sv reflect.Value, name string) bool {
	c := getRelCache(m, sv, false)
	if c == nil {
		return false
	}
	if _, ok := c.ones[name]; ok {
		return true
	}
	if _, ok := c.lists[name]; ok {
		return true
	}
	return false
}

// Prefetched returns the cached multi-valued slice stored on owner under name
// (a relation accessor name or a Prefetch.ToAttr key). It returns (nil,
// false) when owner does not embed RelCache, name is not cached, or the cached
// slice does not have element type T.
func Prefetched[T any](owner any, name string) ([]T, bool) {
	c := cacheFromAny(owner)
	if c == nil || c.lists == nil {
		return nil, false
	}
	e, ok := c.lists[name]
	if !ok {
		return nil, false
	}
	slice, ok := e.items.([]T)
	if !ok {
		return nil, false
	}
	cp := make([]T, len(slice))
	copy(cp, slice)
	return cp, true
}

// PrefetchedOne returns the cached single related object stored on owner under
// name (a reverse O2O accessor name or a single-valued Prefetch.ToAttr
// key). When the relation was prefetched and had no matching row, it returns
// (nil, true). It returns (nil, false) when name is not cached or has another
// target type.
func PrefetchedOne[T any](owner any, name string) (*T, bool) {
	c := cacheFromAny(owner)
	if c == nil || c.ones == nil {
		return nil, false
	}
	e, ok := c.ones[name]
	if !ok {
		return nil, false
	}
	if e.missing || e.obj == nil {
		return nil, true
	}
	ptr, ok := e.obj.(*T)
	if !ok {
		return nil, false
	}
	return ptr, true
}

// CachedOne returns the cached single-object entry stored under name on
// instance (a struct value or pointer for model m) without querying the
// database. loaded is true when name was loaded (even if missing/nil).
func CachedOne(m *Model, instance any, name string) (obj any, loaded bool) {
	sv, ok := structValueOf(instance)
	if !ok {
		return nil, false
	}
	o, missing, found := getReverseOneCache(m, sv, name)
	if !found {
		return nil, false
	}
	if missing {
		return nil, true
	}
	return o, true
}

// CachedList returns the cached multi-valued slice and its target *Model
// stored under name on instance without querying the database.
func CachedList(m *Model, instance any, name string) (items any, target *Model, loaded bool) {
	sv, ok := structValueOf(instance)
	if !ok {
		return nil, nil, false
	}
	return getPrefetchListCache(m, sv, name)
}

// CachedAttr describes one custom Prefetch.ToAttr entry stored in an
// instance's RelCache.
type CachedAttr struct {
	// Name is the ToAttr key.
	Name string
	// Single reports whether the entry holds a single related object (Obj) or
	// a slice of related objects (Items).
	Single bool
	// Obj holds the related struct pointer when Single is true, or nil when
	// no matching row existed.
	Obj any
	// Items holds the typed slice []T when Single is false.
	Items any
	// Model is the target *Model of Obj or of Items' elements.
	Model *Model
}

// CachedAttrs returns all custom ToAttr entries stored on instance (entries
// whose key does not match a declared reverse relation or forward M2M field on
// m), sorted by Name. It never queries the database.
func CachedAttrs(m *Model, instance any) []CachedAttr {
	sv, ok := structValueOf(instance)
	if !ok {
		return nil
	}
	c := getRelCache(m, sv, false)
	if c == nil || (len(c.ones) == 0 && len(c.lists) == 0) {
		return nil
	}
	declared := make(map[string]bool)
	if m != nil {
		for _, f := range m.fields {
			declared[f.Attr] = true
		}
		for _, f := range m.m2mFields {
			declared[f.Attr] = true
		}
		for _, rev := range m.reverseRels {
			declared[rev.Name] = true
		}
	}
	var out []CachedAttr
	for name, e := range c.ones {
		if declared[name] {
			continue
		}
		var obj any
		if !e.missing {
			obj = e.obj
		}
		out = append(out, CachedAttr{Name: name, Single: true, Obj: obj, Model: e.model})
	}
	for name, e := range c.lists {
		if declared[name] {
			continue
		}
		out = append(out, CachedAttr{Name: name, Single: false, Items: e.items, Model: e.model})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// structValueOf unwraps a struct value or non-nil struct pointer.
func structValueOf(v any) (reflect.Value, bool) {
	if v == nil {
		return reflect.Value{}, false
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return reflect.Value{}, false
		}
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return reflect.Value{}, false
	}
	return rv, true
}
