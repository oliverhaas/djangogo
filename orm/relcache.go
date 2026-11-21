package orm

import "reflect"

// RelCache is an optional embedded struct for models that participate in
// cached reverse relations (reverse O2O loaded via SelectRelated or
// RelatedOne). Models without RelCache still support all queries and reverse
// lookups; embedding RelCache allows repeated lookups and SelectRelated on
// reverse one-to-one relations to read from memory without extra queries.
//
// RelCache is skipped by the field parser and adds no database column. Copying
// a model value copies its cache pointer, so a value returned by QuerySet.Get
// or QuerySet.All retains its populated cache.
type RelCache struct {
	cache *relCache
}

// relCacheType is the reflect.Type of RelCache, recognized by the registry.
var relCacheType = reflect.TypeOf(RelCache{})

// relCache holds cached reverse relation entries for one model instance.
type relCache struct {
	ones map[string]relOneEntry
}

// relOneEntry records one cached reverse one-to-one lookup: either a pointer
// to the related struct or missing=true when no related row exists.
type relOneEntry struct {
	obj     any
	missing bool
}

// ClearRelCache removes all cached reverse relation entries from this instance.
func (r *RelCache) ClearRelCache() {
	if r == nil || r.cache == nil {
		return
	}
	clear(r.cache.ones)
}

// ClearRelation removes the cached reverse relation entry for name from this
// instance, forcing the next lookup to query the database.
func (r *RelCache) ClearRelation(name string) {
	if r == nil || r.cache == nil {
		return
	}
	delete(r.cache.ones, name)
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
		c := &relCache{ones: make(map[string]relOneEntry)}
		fv.Set(reflect.ValueOf(RelCache{cache: c}))
		return c
	}
	return rc.cache
}

// setReverseOneCache stores a reverse O2O lookup result (or a missing marker)
// on sv if m embeds RelCache.
func setReverseOneCache(m *Model, sv reflect.Value, name string, obj any, missing bool) {
	c := getRelCache(m, sv, true)
	if c == nil {
		return
	}
	if c.ones == nil {
		c.ones = make(map[string]relOneEntry)
	}
	c.ones[name] = relOneEntry{obj: obj, missing: missing}
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
