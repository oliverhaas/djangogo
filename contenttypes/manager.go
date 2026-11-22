package contenttypes

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"weak"

	"github.com/oliverhaas/djangogo/orm"
)

// naturalKey is the cache key of a content type: app label and model name.
type naturalKey struct {
	appLabel string
	model    string
}

// cache holds the content types already seen for one database handle, by
// natural key and by id. The mutex guards the maps only and is never held
// across a query: a caller inside a transaction may own the only connection of
// the pool, and waiting for it under a lock would deadlock.
type cache struct {
	mu    sync.Mutex
	byKey map[naturalKey]ContentType
	byID  map[int64]ContentType
}

// caches maps a database handle to its cache. The keys are weak pointers, so a
// handle that is no longer used (every test builds its own) does not keep its
// cache alive.
var caches sync.Map // weak.Pointer[orm.DB] -> *cache

// cacheFor returns the cache of db, creating it on first use.
func cacheFor(db *orm.DB) *cache {
	key := weak.Make(db)
	if c, ok := caches.Load(key); ok {
		return c.(*cache)
	}
	c, loaded := caches.LoadOrStore(key, &cache{
		byKey: make(map[naturalKey]ContentType),
		byID:  make(map[int64]ContentType),
	})
	if !loaded {
		runtime.AddCleanup(db, func(k weak.Pointer[orm.DB]) { caches.Delete(k) }, key)
	}
	return c.(*cache)
}

// add stores ct under both keys, unless another row is already cached for its
// natural key, and returns the row the cache holds for that key.
func (c *cache) add(ct ContentType) ContentType {
	c.mu.Lock()
	defer c.mu.Unlock()
	key := naturalKey{ct.AppLabel, ct.Model}
	if cached, ok := c.byKey[key]; ok {
		return cached
	}
	c.byKey[key] = ct
	c.byID[ct.ID] = ct
	return ct
}

// get returns the cached content type for key.
func (c *cache) get(key naturalKey) (ContentType, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	ct, ok := c.byKey[key]
	return ct, ok
}

// getID returns the cached content type with the given id.
func (c *cache) getID(id int64) (ContentType, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	ct, ok := c.byID[id]
	return ct, ok
}

// addID stores a row that was looked up by id under both keys.
func (c *cache) addID(ct ContentType) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.byID[ct.ID] = ct
	c.byKey[naturalKey{ct.AppLabel, ct.Model}] = ct
}

// reset empties the cache.
func (c *cache) reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	clear(c.byKey)
	clear(c.byID)
}

// keyOf returns the natural key of a registered model: its app label and its
// lowercased type name, Django's (app_label, model_name).
func keyOf(m *orm.Model) naturalKey {
	return naturalKey{appLabel: m.App(), model: strings.ToLower(m.Name())}
}

// ClearCache forgets the cached content types of db (Django's clear_cache).
// Call it after changing the django_content_type table behind the cache's back,
// for example after deleting rows.
func ClearCache(db *orm.DB) {
	cacheFor(db).reset()
}

// GetForModel returns the content type of model, creating the row when it does
// not exist yet. Lookups are cached per database handle, so only the first one
// for a model reaches the database.
func GetForModel(ctx context.Context, db *orm.DB, model *orm.Model) (ContentType, error) {
	if model == nil {
		return ContentType{}, errors.New("contenttypes: GetForModel: nil model")
	}
	key := keyOf(model)
	c := cacheFor(db)
	if ct, ok := c.get(key); ok {
		return ct, nil
	}
	ct, err := load(ctx, db, key)
	if errors.Is(err, orm.ErrDoesNotExist) {
		ct, err = create(ctx, db, key)
	}
	if err != nil {
		return ContentType{}, err
	}
	return c.add(ct), nil
}

// GetForInstance returns the content type of the model of instance, which is a
// model struct or a pointer to one. It fails for a type that is not registered.
func GetForInstance(ctx context.Context, db *orm.DB, instance any) (ContentType, error) {
	model, ok := db.Registry().ModelOf(instance)
	if !ok {
		return ContentType{}, fmt.Errorf("contenttypes: %T is not a registered model", instance)
	}
	return GetForModel(ctx, db, model)
}

// GetForModels returns the content type of every given model, keyed by model.
// The models missing from the cache are read with one query, and the ones
// missing from the database are created.
func GetForModels(ctx context.Context, db *orm.DB, models ...*orm.Model) (map[*orm.Model]ContentType, error) {
	results := make(map[*orm.Model]ContentType, len(models))
	c := cacheFor(db)

	// The models that are not cached, by natural key, and their app labels in
	// first-seen order.
	needed := make(map[naturalKey][]*orm.Model)
	var order []naturalKey
	var appLabels []any
	seenLabel := make(map[string]bool)
	for _, m := range models {
		if m == nil {
			return nil, errors.New("contenttypes: GetForModels: nil model")
		}
		key := keyOf(m)
		if ct, ok := c.get(key); ok {
			results[m] = ct
			continue
		}
		if _, ok := needed[key]; !ok {
			order = append(order, key)
		}
		needed[key] = append(needed[key], m)
		if !seenLabel[key.appLabel] {
			seenLabel[key.appLabel] = true
			appLabels = append(appLabels, key.appLabel)
		}
	}
	if len(needed) == 0 {
		return results, nil
	}

	// One query for every app label involved. A row of another model of these
	// apps is cached as well.
	rows, err := orm.Query[ContentType](db).Filter("app_label__in", appLabels).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("contenttypes: load content types: %w", err)
	}
	for _, ct := range rows {
		c.add(ct)
	}
	for _, key := range order {
		ct, ok := c.get(key)
		if !ok {
			if ct, err = create(ctx, db, key); err != nil {
				return nil, err
			}
			ct = c.add(ct)
		}
		for _, m := range needed[key] {
			results[m] = ct
		}
	}
	return results, nil
}

// GetForID returns the content type with the given primary key. It shares the
// cache of GetForModel, but never creates a row: an unknown id is reported as
// orm.ErrDoesNotExist.
func GetForID(ctx context.Context, db *orm.DB, id int64) (ContentType, error) {
	c := cacheFor(db)
	if ct, ok := c.getID(id); ok {
		return ct, nil
	}
	ct, err := orm.Query[ContentType](db).Get(ctx, "id", id)
	if err != nil {
		return ContentType{}, fmt.Errorf("contenttypes: content type %d: %w", id, err)
	}
	c.addID(ct)
	return ct, nil
}

// GetByNaturalKey returns the content type with the given app label and model
// name. It shares the cache of GetForModel, but never creates a row: an unknown
// pair is reported as orm.ErrDoesNotExist.
func GetByNaturalKey(ctx context.Context, db *orm.DB, appLabel, model string) (ContentType, error) {
	key := naturalKey{appLabel, model}
	c := cacheFor(db)
	if ct, ok := c.get(key); ok {
		return ct, nil
	}
	ct, err := load(ctx, db, key)
	if err != nil {
		return ContentType{}, err
	}
	return c.add(ct), nil
}

// ModelClass returns the registered model a content type stands for (Django's
// model_class). It reports false for a stale content type, whose model is no
// longer installed.
func ModelClass(db *orm.DB, ct ContentType) (*orm.Model, bool) {
	want := naturalKey{ct.AppLabel, ct.Model}
	for _, m := range db.Registry().Models() {
		if keyOf(m) == want {
			return m, true
		}
	}
	return nil, false
}

// Sync creates the content type of every model in db's registry that has none
// yet, in registration order, and returns the created rows. It is the
// counterpart of the create_contenttypes handler Django runs after migrate, and
// like it is safe to run again: a second run creates nothing.
func Sync(ctx context.Context, db *orm.DB) ([]ContentType, error) {
	// The table is read afresh: rows may have been removed since they were
	// cached, and Sync is the operation that has to notice.
	rows, err := orm.Query[ContentType](db).OrderBy("id").All(ctx)
	if err != nil {
		return nil, fmt.Errorf("contenttypes: load content types: %w", err)
	}
	c := cacheFor(db)
	c.reset()
	for _, ct := range rows {
		c.add(ct)
	}

	var created []ContentType
	for _, m := range db.Registry().Models() {
		key := keyOf(m)
		if _, ok := c.get(key); ok {
			continue
		}
		ct, own, err := insert(ctx, db, key)
		if err != nil {
			return created, err
		}
		c.add(ct)
		// A row that a concurrent caller created is not reported.
		if own {
			created = append(created, ct)
		}
	}
	return created, nil
}

// load reads the row for key from the database. A missing row is reported as
// orm.ErrDoesNotExist.
func load(ctx context.Context, db *orm.DB, key naturalKey) (ContentType, error) {
	ct, err := orm.Query[ContentType](db).Get(ctx, "app_label", key.appLabel, "model", key.model)
	if err != nil {
		return ContentType{}, fmt.Errorf("contenttypes: %s.%s: %w", key.appLabel, key.model, err)
	}
	return ct, nil
}

// create inserts the row for key and returns the row that now stands for it.
func create(ctx context.Context, db *orm.DB, key naturalKey) (ContentType, error) {
	ct, _, err := insert(ctx, db, key)
	return ct, err
}

// insert adds the row for key and returns the row that now stands for the
// pair, and whether that is the row inserted here. It is Django's
// get_or_create: when another caller inserts the same pair first, the unique
// constraint rejects this insert and the other caller's row is returned.
func insert(ctx context.Context, db *orm.DB, key naturalKey) (ContentType, bool, error) {
	ct, created, err := orm.Query[ContentType](db).GetOrCreate(ctx, nil, "app_label", key.appLabel, "model", key.model)
	if err != nil {
		return ContentType{}, false, fmt.Errorf("contenttypes: create %s.%s: %w", key.appLabel, key.model, err)
	}
	return ct, created, nil
}
