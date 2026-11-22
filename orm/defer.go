package orm

import (
	"context"
	"fmt"
	"reflect"
	"slices"
	"strings"
)

// Defer returns a clone that does not load the named fields, Django's
// defer(). A deferred field holds its zero value in the loaded objects;
// DeferredFields names it, and RefreshFromDB loads it. A key is a field
// attribute or column, and a double underscore names a field of a relation
// that SelectRelated loads ("author__bio"). Defer adds to the fields of an
// earlier Defer, and removes the keys from the fields of an earlier Only.
// Defer without keys loads every field again, Django's defer(None).
//
// The objects record their deferred fields in orm.RelCache, so the model, and
// each related model that SelectRelated loads with deferred fields, must
// embed it. Save of an object with deferred fields writes its loaded fields
// and the deferred fields that no longer hold their zero value, and fails
// when the row no longer exists. Values and ValuesList ignore Defer and Only.
func (qs *QuerySet[T]) Defer(keys ...string) *QuerySet[T] {
	return &QuerySet[T]{q: qs.q.deferFields(keys)}
}

// Only returns a clone that loads the primary key and the named fields only,
// Django's only(). The keys are those of Defer, and "pk" names the primary
// key. Only replaces the keys of an earlier Only, and the fields of an
// earlier Defer stay deferred. Only without keys loads every field. Defer
// describes how the objects record their deferred fields.
func (qs *QuerySet[T]) Only(keys ...string) *QuerySet[T] {
	return &QuerySet[T]{q: qs.q.onlyFields(keys)}
}

// deferFields implements QuerySet.Defer, Django's add_deferred_loading.
func (q *query) deferFields(names []string) *query {
	cp := q.clone()
	switch {
	case len(names) == 0:
		cp.deferred, cp.only = nil, false
	case !q.only:
		cp.deferred = withoutNames(append(slices.Clone(q.deferred), names...), nil)
	default:
		if rest := withoutNames(q.deferred, names); len(rest) > 0 {
			cp.deferred = rest
		} else {
			cp.deferred, cp.only = withoutNames(names, q.deferred), false
		}
	}
	return cp
}

// onlyFields implements QuerySet.Only, Django's add_immediate_loading.
func (q *query) onlyFields(names []string) *query {
	cp := q.clone()
	names = slices.Clone(names)
	if q.model != nil && q.model.pk != nil {
		for i, name := range names {
			if name == "pk" {
				names[i] = q.model.pk.Attr
			}
		}
	}
	if q.only {
		cp.deferred = withoutNames(names, nil)
	} else {
		cp.deferred = withoutNames(names, q.deferred)
	}
	cp.only = true
	return cp
}

// withoutNames returns the names that drop does not hold, once each, in the
// order of names.
func withoutNames(names, drop []string) []string {
	var out []string
	for _, name := range names {
		if !slices.Contains(drop, name) && !slices.Contains(out, name) {
			out = append(out, name)
		}
	}
	return out
}

// loading returns a clone that loads fields whatever Defer and Only say, for
// a prefetch that reads them to match the rows to their parents.
func (q *query) loading(fields ...*Field) *query {
	if len(q.deferred) == 0 {
		return q
	}
	cp := q.clone()
	cp.keep = append(cp.keep, fields...)
	return cp
}

// nameTree holds the keys of Defer or Only split at the double underscores,
// Django's mask in get_select_mask: each name maps to the names below it.
type nameTree struct {
	// names are the names of this level, in the order they were first given.
	names []string
	subs  map[string]*nameTree
}

// newNameTree returns the tree of keys.
func newNameTree(keys []string) *nameTree {
	t := &nameTree{}
	for _, key := range keys {
		n := t
		for part := range strings.SplitSeq(key, lookupSep) {
			n = n.child(part)
		}
	}
	return t
}

// child returns the subtree of name, which it adds when it is missing.
func (t *nameTree) child(name string) *nameTree {
	if c, ok := t.subs[name]; ok {
		return c
	}
	if t.subs == nil {
		t.subs = make(map[string]*nameTree)
	}
	c := &nameTree{}
	t.subs[name] = c
	t.names = append(t.names, name)
	return c
}

// selectMask is Django's select mask: the fields and reverse relations of a
// model to load, keyed by *Field or *ReverseRelation, each with the mask of
// its related model. An empty mask loads every field.
type selectMask map[any]selectMask

// at returns the mask of key, which it adds when it is missing, Django's
// select_mask.setdefault(key, {}).
func (s selectMask) at(key any) selectMask {
	sub := s[key]
	if sub == nil {
		sub = selectMask{}
		s[key] = sub
	}
	return sub
}

// selectMask returns the mask of the fields to load that Defer and Only give,
// Django's get_select_mask, or nil to load every field.
func (q *query) selectMask() (selectMask, error) {
	if len(q.deferred) == 0 {
		return nil, nil
	}
	mask := selectMask{}
	tree := newNameTree(q.deferred)
	if q.only {
		return mask, onlyMask(q.model, tree, mask)
	}
	return mask, deferMask(q.model, tree, mask)
}

// maskKey resolves name on m to the key of a select mask: the field, or the
// reverse relation, and the model a name below it is resolved on, which is
// nil for a field that is not a relation. It returns a nil key for a
// many-to-many relation, which Defer and Only ignore, as Django does.
func maskKey(m *Model, name string) (key any, target *Model, err error) {
	f, rel, ok := lookupName(m, name)
	switch {
	case !ok:
		return nil, nil, &FieldError{Model: m.Name(), Message: "has no field named '" + name + "'"}
	case f != nil && rel != nil && rel.local != nil:
		return f, rel.target, nil
	case f != nil:
		return f, nil, nil
	case rel.reverse != nil && rel.reverse.Kind != RelM2M:
		return rel.reverse, rel.reverse.Source, nil
	}
	return nil, nil, nil
}

// noFieldBelow is Django's FieldError for a name below a field that is not a
// relation.
func noFieldBelow(m *Model, f *Field, name string) error {
	return &FieldError{
		Model:   m.Name(),
		Message: "the field '" + f.Attr + "' is not a relation and has no field named '" + name + "'",
	}
}

// deferMask fills mask with the fields of m that the names of t do not
// defer, Django's _get_defer_select_mask: every field and reverse relation
// that no name of t names, and each named relation with names below it, with
// the mask of its related model.
func deferMask(m *Model, t *nameTree, mask selectMask) error {
	mask.at(m.pk)
	named := make(map[any]*nameTree)
	targets := make(map[any]*Model)
	for _, name := range t.names {
		key, target, err := maskKey(m, name)
		if err != nil {
			return err
		}
		if key == nil {
			continue
		}
		sub := named[key]
		if sub == nil {
			sub = &nameTree{}
			named[key] = sub
		}
		// A relation named by its attribute and by its column merges the
		// names below both.
		for _, below := range t.subs[name].names {
			graft(sub.child(below), t.subs[name].subs[below])
		}
		targets[key] = target
	}
	keys := make([]any, 0, len(m.fields)+len(m.reverseRels))
	for _, f := range m.fields {
		keys = append(keys, f)
	}
	for _, rev := range m.reverseRels {
		keys = append(keys, rev)
	}
	for _, key := range keys {
		sub, ok := named[key]
		switch {
		case !ok:
			mask.at(key)
		case len(sub.names) == 0:
			// The name defers the field.
		case targets[key] == nil:
			return noFieldBelow(m, key.(*Field), sub.names[0])
		default:
			if err := deferMask(targets[key], sub, mask.at(key)); err != nil {
				return err
			}
		}
	}
	return nil
}

// graft adds the names of src below dst.
func graft(dst, src *nameTree) {
	for _, name := range src.names {
		graft(dst.child(name), src.subs[name])
	}
}

// onlyMask fills mask with the primary key of m and the fields that the
// names of t name, Django's _get_only_select_mask, with the mask of the
// related model of a relation that has names below it.
func onlyMask(m *Model, t *nameTree, mask selectMask) error {
	mask.at(m.pk)
	for _, name := range t.names {
		key, target, err := maskKey(m, name)
		if err != nil {
			return err
		}
		if key == nil {
			continue
		}
		sub := t.subs[name]
		if len(sub.names) == 0 {
			mask.at(key)
			continue
		}
		if target == nil {
			return noFieldBelow(m, key.(*Field), sub.names[0])
		}
		if err := onlyMask(target, sub, mask.at(key)); err != nil {
			return err
		}
	}
	return nil
}

// loadedFields returns the fields of m that mask loads, together with keep,
// and the attnames of the others, in the order of m's fields. Both are nil
// when every field is loaded.
func loadedFields(m *Model, mask selectMask, keep ...*Field) ([]*Field, []string) {
	if len(mask) == 0 {
		return nil, nil
	}
	var (
		fields   []*Field
		deferred []string
	)
	for _, f := range m.fields {
		if _, ok := mask[f]; ok || slices.Contains(keep, f) {
			fields = append(fields, f)
		} else {
			deferred = append(deferred, attname(f))
		}
	}
	if len(deferred) == 0 {
		return nil, nil
	}
	return fields, deferred
}

// checkDeferrable refuses deferred fields of m when m does not embed
// orm.RelCache, where an object records them.
func checkDeferrable(m *Model, deferred []string) error {
	if len(deferred) > 0 && m.cacheIndex < 0 {
		return fmt.Errorf("orm: %s must embed orm.RelCache to defer fields: "+
			"an object records its deferred fields there", m.Name())
	}
	return nil
}

// DeferredFields returns the attnames of the fields of obj, a model struct or
// a pointer to one, that Defer or Only left unloaded, Django's
// get_deferred_fields. They are in the order of the model's fields, and a
// relation is named "<attribute>_id". It is empty for an object loaded with
// every field.
func DeferredFields(obj any) []string {
	c := cacheFromAny(obj)
	if c == nil {
		return nil
	}
	return slices.Clone(c.deferred)
}

// deferredOf returns the attnames of the deferred fields of sv, a struct of
// model m.
func deferredOf(m *Model, sv reflect.Value) []string {
	if c := getRelCache(m, sv, false); c != nil {
		return c.deferred
	}
	return nil
}

// setDeferred records names as the deferred fields of sv, a struct of model
// m that embeds orm.RelCache. The slice is shared, and never changed in
// place.
func setDeferred(m *Model, sv reflect.Value, names []string) {
	if len(names) == 0 {
		if c := getRelCache(m, sv, false); c != nil {
			c.deferred = nil
		}
		return
	}
	if c := getRelCache(m, sv, true); c != nil {
		c.deferred = names
	}
}

// markLoaded removes the attnames of fields from the deferred fields of sv, a
// struct of model m.
func markLoaded(m *Model, sv reflect.Value, fields []*Field) {
	deferred := deferredOf(m, sv)
	if len(deferred) == 0 {
		return
	}
	names := make([]string, len(fields))
	for i, f := range fields {
		names[i] = attname(f)
	}
	setDeferred(m, sv, withoutNames(deferred, names))
}

// checkInsertDeferred refuses to insert sv, a struct of model m, while one of
// its deferred fields holds its zero value, Django's "Cannot retrieve deferred
// field from an unsaved model": the value of the field is not known.
func checkInsertDeferred(m *Model, sv reflect.Value) error {
	for _, name := range deferredOf(m, sv) {
		if f, ok := lookupField(m, name); ok && sv.Field(f.Index).IsZero() {
			return fmt.Errorf("orm: cannot retrieve deferred field '%s' from an unsaved model", name)
		}
	}
	return nil
}

// RefreshFromDB loads the fields of obj again from its row, Django's
// refresh_from_db with the queryset as from_queryset: the row is the one with
// obj's primary key among the rows of the queryset. Without keys it loads the
// fields that obj has loaded, and with keys the named fields, which are field
// attributes or columns. A loaded relation field drops its cached object,
// unless the queryset loads it with SelectRelated.
//
// Without keys RefreshFromDB also drops the cached reverse relations and the
// prefetched lists of obj's orm.RelCache. A key that names a prefetched list
// drops that list and loads nothing, and a key that names a reverse
// relation drops its cached object. A key that spans relations is an error.
// The error matches ErrDoesNotExist when the row no longer exists.
func (qs *QuerySet[T]) RefreshFromDB(ctx context.Context, obj *T, keys ...string) error {
	q := qs.q
	if q.err != nil {
		return q.err
	}
	m := q.model
	v := reflect.ValueOf(obj).Elem()
	cache := getRelCache(m, v, false)
	if len(keys) > 0 {
		var rest []string
		for _, key := range keys {
			if cache != nil && cache.lists != nil {
				if _, ok := cache.lists[key]; ok {
					delete(cache.lists, key)
					continue
				}
			}
			rest = append(rest, key)
		}
		if len(rest) == 0 {
			return nil
		}
		if slices.ContainsFunc(rest, func(key string) bool { return strings.Contains(key, lookupSep) }) {
			return fmt.Errorf("orm: refresh %s: found %q in the keys: relations and transforms are not allowed",
				m.Name(), lookupSep)
		}
		keys = rest
	} else if cache != nil {
		clear(cache.lists)
	}
	if m.pk == nil {
		return fmt.Errorf("orm: refresh %s: the model has no primary key", m.Name())
	}
	q = q.filter([]any{"pk", v.Field(m.pk.Index).Interface()}, false)
	deferred := deferredOf(m, v)
	switch {
	case len(keys) > 0:
		q = q.onlyFields(keys)
	case len(deferred) > 0:
		var loaded []string
		for _, f := range m.fields {
			if name := attname(f); !slices.Contains(deferred, name) {
				loaded = append(loaded, name)
			}
		}
		q = q.onlyFields(loaded)
	}
	q.refresh = true
	mask, err := q.selectMask()
	if err != nil {
		return err
	}
	fields, _ := loadedFields(m, mask)
	if fields == nil {
		fields = m.fields
	}
	fresh, err := (&QuerySet[T]{q: q}).Get(ctx)
	if err != nil {
		return err
	}
	fv := reflect.ValueOf(&fresh).Elem()
	for _, f := range fields {
		v.Field(f.Index).Set(fv.Field(f.Index))
	}
	markLoaded(m, v, fields)
	refreshCaches(m, v, keys)
	return nil
}

// refreshCaches drops the cached reverse relations and generic foreign key
// objects of sv, a struct of model m, that keys name, or all of them without
// keys, as Django's refresh_from_db does.
func refreshCaches(m *Model, sv reflect.Value, keys []string) {
	named := func(names ...string) bool {
		return len(keys) == 0 || slices.ContainsFunc(names, func(name string) bool { return slices.Contains(keys, name) })
	}
	if cache := getRelCache(m, sv, false); cache != nil {
		for _, rev := range m.reverseRels {
			if named(rev.Name, rev.QueryName) {
				delete(cache.ones, rev.Name)
			}
		}
	}
	for _, pf := range m.privateFields {
		fv := sv.Field(pf.Index)
		if _, ok := fv.Interface().(ObjectHolder); ok && named(pf.Attr) {
			fv.SetZero()
		}
	}
}
