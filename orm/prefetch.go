package orm

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
)

// Prefetch configures one PrefetchRelated lookup with an optional custom
// QuerySet and destination cache attribute, mirroring Django's Prefetch object.
//
// Pass a Prefetch (or *Prefetch) alongside plain lookup strings to
// QuerySet.PrefetchRelated, RawQuerySet.PrefetchRelated, or
// PrefetchRelatedObjects:
//
//	orm.Query[Person](db).PrefetchRelated(orm.Prefetch{
//		Lookup:   "houses",
//		QuerySet: orm.Query[House](db).Filter("active", true),
//		ToAttr:   "active_houses",
//	})
type Prefetch struct {
	// Lookup is the "__"-separated relation path to prefetch (e.g. "authors",
	// "books__read_by", or "author__profile").
	Lookup string
	// QuerySet optionally overrides the queryset executed for the final
	// segment of Lookup. It must be a *QuerySet[T] for the final segment's
	// target model; ValuesQuerySet, ValuesListQuerySet, and RawQuerySet are
	// rejected.
	QuerySet any
	// QuerySets holds the querysets of a lookup that ends in a generic
	// foreign key, at most one per model, Django's GenericPrefetch: the
	// objects of a model that has a queryset here are loaded through it.
	// QuerySet, when set, counts as one more entry. A lookup that ends in any
	// other relation takes one queryset at most.
	QuerySets []any
	// ToAttr optionally stores the prefetched result under a custom cache key
	// on each parent instance's RelCache instead of the relation's default
	// cache slot. Read it with orm.Prefetched[T] or orm.PrefetchedOne[T].
	ToAttr string
}

// m2mPrefetchFilter joins the M2M intermediate table onto the target model's
// query, filters by parent PKs IN (...), and selects the parent PK column so
// one batched query populates every parent instance.
type m2mPrefetchFilter struct {
	through       *Model
	targetFK      *Field // FK on through pointing to target model
	sourceFK      *Field // FK on through pointing to parent model
	parentPKField *Field // PK field of parent model (for normalizing scanned parent PK)
	parentPKs     []any
}

// prefetchQuerier is implemented by *QuerySet[T] to expose its underlying
// *query for Prefetch.QuerySet without matching Values or Raw querysets.
type prefetchQuerier interface {
	prefetchQuery() *query
}

// rawQuerier is implemented by *RawQuerySet[T] so Prefetch rejects raw
// querysets with Django's error message.
type rawQuerier interface {
	isRawQuerySet()
}

// parsePrefetchLookups validates and normalizes the arguments passed to
// PrefetchRelated. Calling with no arguments or a single nil signals reset=true.
func parsePrefetchLookups(lookups []any) (specs []Prefetch, reset bool, err error) {
	if len(lookups) == 0 || (len(lookups) == 1 && lookups[0] == nil) {
		return nil, true, nil
	}
	specs = make([]Prefetch, 0, len(lookups))
	for _, raw := range lookups {
		var spec Prefetch
		switch v := raw.(type) {
		case string:
			spec = Prefetch{Lookup: v}
		case Prefetch:
			spec = v
		case *Prefetch:
			if v == nil {
				return nil, false, errors.New("orm: prefetch_related lookup must not be nil")
			}
			spec = *v
		default:
			return nil, false, fmt.Errorf("orm: prefetch_related expects string or Prefetch, got %T", raw)
		}
		if spec.Lookup == "" {
			return nil, false, errors.New("orm: prefetch_related lookup must not be empty")
		}
		for _, qs := range append([]any{spec.QuerySet}, spec.QuerySets...) {
			if qs == nil {
				continue
			}
			if _, err := extractPrefetchQuery(qs); err != nil {
				return nil, false, err
			}
		}
		specs = append(specs, spec)
	}
	return specs, false, nil
}

// extractPrefetchQuery extracts a cloned *query from a Prefetch.QuerySet,
// rejecting RawQuerySet, ValuesQuerySet, ValuesListQuerySet, and sliced
// querysets.
func extractPrefetchQuery(raw any) (*query, error) {
	switch raw.(type) {
	case *ValuesQuerySet, *ValuesListQuerySet, rawQuerier:
		return nil, errors.New("orm: Prefetch querysets cannot use raw(), values(), and values_list()")
	}
	pq, ok := raw.(prefetchQuerier)
	if !ok {
		return nil, fmt.Errorf("orm: Prefetch.QuerySet must be a *orm.QuerySet[T], got %T", raw)
	}
	q := pq.prefetchQuery()
	if q == nil {
		return nil, errors.New("orm: Prefetch.QuerySet must not be nil")
	}
	if q.err != nil {
		return nil, q.err
	}
	if q.values != nil {
		return nil, errors.New("orm: Prefetch querysets cannot use raw(), values(), and values_list()")
	}
	if q.sliced() {
		return nil, errors.New("orm: prefetch_related does not support sliced querysets (Limit/Offset)")
	}
	return q.clone(), nil
}

// PrefetchRelatedObjects populates prefetched relation caches on an already
// fetched collection of model instances, mirroring Django's
// prefetch_related_objects(model_instances, *related_lookups).
//
// instances may be a slice of model structs ([]T), a slice of model pointers
// ([]*T), or a single non-nil model pointer (*T). Instances that already have a
// requested relation cached are not queried again.
func PrefetchRelatedObjects(ctx context.Context, db *DB, instances any, lookups ...any) error {
	specs, reset, err := parsePrefetchLookups(lookups)
	if err != nil {
		return err
	}
	if reset || len(specs) == 0 || instances == nil {
		return nil
	}
	model, elems, err := unwrapPrefetchInstances(db.Registry(), instances)
	if err != nil {
		return err
	}
	if len(elems) == 0 {
		return nil
	}
	return prefetchRelatedObjects(ctx, db, model, elems, specs)
}

// unwrapPrefetchInstances extracts the *Model and settable struct reflect.Values
// from []T, []*T, or *T.
func unwrapPrefetchInstances(reg *Registry, instances any) (*Model, []reflect.Value, error) {
	rv := reflect.ValueOf(instances)
	if rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return nil, nil, nil
		}
		if rv.Elem().Kind() != reflect.Struct {
			return nil, nil, fmt.Errorf("orm: PrefetchRelatedObjects expects a model slice or pointer, got %T", instances)
		}
		m, ok := reg.ModelOf(instances)
		if !ok {
			return nil, nil, fmt.Errorf("orm: no model registered for %T", instances)
		}
		return m, []reflect.Value{rv.Elem()}, nil
	}
	if rv.Kind() != reflect.Slice {
		return nil, nil, fmt.Errorf("orm: PrefetchRelatedObjects expects a model slice or pointer, got %T", instances)
	}
	if rv.Len() == 0 {
		return nil, nil, nil
	}
	elemType := rv.Type().Elem()
	isPtr := elemType.Kind() == reflect.Pointer
	if isPtr {
		elemType = elemType.Elem()
	}
	if elemType.Kind() != reflect.Struct {
		return nil, nil, fmt.Errorf("orm: PrefetchRelatedObjects expects a slice of model structs or pointers, got %T", instances)
	}
	m, ok := reg.ModelOf(reflect.New(elemType).Interface())
	if !ok {
		return nil, nil, fmt.Errorf("orm: no model registered for %s", elemType)
	}
	elems := make([]reflect.Value, 0, rv.Len())
	for i := range rv.Len() {
		item := rv.Index(i)
		if isPtr {
			if item.IsNil() {
				continue
			}
			item = item.Elem()
		}
		elems = append(elems, item)
	}
	return m, elems, nil
}

// prefetchTask is one normalized entry in the prefetch_related work queue.
type prefetchTask struct {
	throughParts []string
	toAttrParts  []string
	rawLookup    string
	finalQS      *query
	finalQSs     []*query // Prefetch.QuerySets
	finalToAttr  string
}

// prefetchTo returns the cumulative cache path through level (inclusive).
func (t *prefetchTask) prefetchTo(level int) string {
	return strings.Join(t.toAttrParts[:level+1], lookupSep)
}

// prefetchGroup holds the instances of one model that a prefetch path
// reached. A path through a generic foreign key reaches several models.
type prefetchGroup struct {
	model *Model
	objs  []reflect.Value
}

// addPrefetchGroup adds objs, instances of m, to groups, merging them into
// the group of m when there is one.
func addPrefetchGroup(groups []prefetchGroup, m *Model, objs []reflect.Value) []prefetchGroup {
	if m == nil || len(objs) == 0 {
		return groups
	}
	for i := range groups {
		if groups[i].model == m {
			groups[i].objs = append(groups[i].objs, objs...)
			return groups
		}
	}
	return append(groups, prefetchGroup{model: m, objs: objs})
}

// normalizePrefetchTasks turns Prefetch options into queued tasks, optionally
// prefixed by an ancestor prefetchTo path for nested prefetches.
func normalizePrefetchTasks(specs []Prefetch, prefix string) ([]prefetchTask, error) {
	var prefixParts []string
	if prefix != "" {
		prefixParts = strings.Split(prefix, lookupSep)
	}
	tasks := make([]prefetchTask, 0, len(specs))
	for _, spec := range specs {
		parts := strings.Split(spec.Lookup, lookupSep)
		for _, p := range parts {
			if p == "" {
				return nil, fmt.Errorf("orm: invalid prefetch_related lookup %q", spec.Lookup)
			}
		}
		var customQ *query
		if spec.QuerySet != nil {
			var err error
			customQ, err = extractPrefetchQuery(spec.QuerySet)
			if err != nil {
				return nil, err
			}
		}
		var customQs []*query
		for _, qs := range spec.QuerySets {
			if qs == nil {
				continue
			}
			q, err := extractPrefetchQuery(qs)
			if err != nil {
				return nil, err
			}
			customQs = append(customQs, q)
		}
		through := make([]string, 0, len(prefixParts)+len(parts))
		through = append(through, prefixParts...)
		through = append(through, parts...)

		toAttrs := make([]string, len(through))
		copy(toAttrs, through)
		if spec.ToAttr != "" {
			toAttrs[len(toAttrs)-1] = spec.ToAttr
		}
		tasks = append(tasks, prefetchTask{
			throughParts: through,
			toAttrParts:  toAttrs,
			rawLookup:    spec.Lookup,
			finalQS:      customQ,
			finalQSs:     customQs,
			finalToAttr:  spec.ToAttr,
		})
	}
	return tasks, nil
}

// prefetchStepKind classifies one relation hop resolved during prefetching.
type prefetchStepKind uint8

const (
	stepNone prefetchStepKind = iota
	stepForwardFK
	stepForwardO2O
	stepReverseO2O
	stepReverseFK
	stepForwardM2M
	stepReverseM2M
	stepCachedAttr
	stepGeneric
	stepGenericOwner // the related_query_name of a generic relation
	stepObject       // a private field that refers to an instance of any model
)

// prefetchStep describes one resolved hop from a parent model to its target.
type prefetchStep struct {
	kind        prefetchStepKind
	field       *Field
	rev         *ReverseRelation
	generic     *GenericRelation
	private     *PrivateField // the field of a stepObject
	targetModel *Model
	through     *Model
	sourceFK    *Field // FK on through pointing to parent
	targetFK    *Field // FK on through pointing to target
	cacheName   string // canonical default cache name (without ToAttr override)
	single      bool
	nonRelField bool // true when name matched a concrete non-relational field
}

// resolvePrefetchStep resolves part against model m (or against already-cached
// ToAttr entries on objs).
func resolvePrefetchStep(m *Model, objs []reflect.Value, part string) prefetchStep {
	if f, ok := m.byAttr[part]; ok {
		return stepFromConcreteField(f)
	}
	if f, ok := m.byName[part]; ok {
		return stepFromConcreteField(f)
	}
	if f, ok := m.ManyToManyField(part); ok && f.Rel != nil && f.Rel.Target != nil && f.Rel.Through != nil {
		return prefetchStep{
			kind:        stepForwardM2M,
			field:       f,
			targetModel: f.Rel.Target,
			through:     f.Rel.Through,
			sourceFK:    f.Rel.SourceField,
			targetFK:    f.Rel.TargetField,
			cacheName:   f.Attr,
		}
	}
	if rev, found, _ := m.lookupReverseAccessor(part); found && !rev.Hidden {
		switch rev.Kind {
		case RelO2O:
			return prefetchStep{
				kind:        stepReverseO2O,
				rev:         rev,
				targetModel: rev.Source,
				cacheName:   rev.Name,
				single:      true,
			}
		case RelFK:
			return prefetchStep{
				kind:        stepReverseFK,
				rev:         rev,
				targetModel: rev.Source,
				cacheName:   rev.Name,
			}
		case RelM2M:
			if rev.Field != nil && rev.Field.Rel != nil && rev.Field.Rel.Through != nil {
				return prefetchStep{
					kind:        stepReverseM2M,
					rev:         rev,
					field:       rev.Field,
					targetModel: rev.Source,
					through:     rev.Field.Rel.Through,
					sourceFK:    rev.Field.Rel.TargetField,
					targetFK:    rev.Field.Rel.SourceField,
					cacheName:   rev.Name,
				}
			}
		}
	}
	if gr, ok := m.GenericRelation(part); ok && gr.Related != nil {
		return prefetchStep{kind: stepGeneric, generic: gr, targetModel: gr.Related, cacheName: gr.Name}
	}
	if gr, ok := m.byGenericQuery[part]; ok {
		return prefetchStep{kind: stepGenericOwner, generic: gr, targetModel: gr.Model, cacheName: part}
	}
	if pf, ok := m.PrivateField(part); ok {
		if _, ok := reflect.Zero(pf.Type).Interface().(ObjectPrefetcher); ok {
			return prefetchStep{kind: stepObject, private: pf, cacheName: pf.Attr, single: true}
		}
		return prefetchStep{kind: stepNone, nonRelField: true}
	}
	// Check whether part was already populated as a custom ToAttr on objs.
	for _, sv := range objs {
		if hasRelCacheKey(m, sv, part) {
			return prefetchStep{kind: stepCachedAttr, cacheName: part}
		}
	}
	return prefetchStep{kind: stepNone}
}

// stepFromConcreteField maps a concrete model field to a prefetchStep.
func stepFromConcreteField(f *Field) prefetchStep {
	if f.Rel == nil || f.Rel.Target == nil {
		return prefetchStep{kind: stepNone, field: f, nonRelField: true}
	}
	switch f.Rel.Kind {
	case RelFK:
		return prefetchStep{
			kind:        stepForwardFK,
			field:       f,
			targetModel: f.Rel.Target,
			cacheName:   f.Attr,
			single:      true,
		}
	case RelO2O:
		return prefetchStep{
			kind:        stepForwardO2O,
			field:       f,
			targetModel: f.Rel.Target,
			cacheName:   f.Attr,
			single:      true,
		}
	default:
		return prefetchStep{kind: stepNone, field: f, nonRelField: true}
	}
}

// toAttrConflicts reports whether toAttr collides with any concrete field,
// M2M field, or reverse relation accessor on m.
func toAttrConflicts(m *Model, toAttr string) bool {
	if _, ok := m.byAttr[toAttr]; ok {
		return true
	}
	if _, ok := m.byName[toAttr]; ok {
		return true
	}
	if _, ok := m.byM2MAttr[toAttr]; ok {
		return true
	}
	if _, ok := m.byM2MName[toAttr]; ok {
		return true
	}
	if _, found, amb := m.lookupReverseAccessor(toAttr); found || amb {
		return true
	}
	if _, ok := m.PrivateField(toAttr); ok {
		return true
	}
	return false
}

// prefetchRelatedObjects executes all specs level by level over roots.
func prefetchRelatedObjects(ctx context.Context, db *DB, rootModel *Model, roots []reflect.Value, specs []Prefetch) error {
	if len(roots) == 0 || len(specs) == 0 {
		return nil
	}
	queue, err := normalizePrefetchTasks(specs, "")
	if err != nil {
		return err
	}
	// done holds the instances already reached at each prefetchTo path.
	done := make(map[string][]prefetchGroup)

	for len(queue) > 0 {
		task := queue[0]
		queue = queue[1:]

		fullKey := task.prefetchTo(len(task.throughParts) - 1)
		if _, seen := done[fullKey]; seen {
			if task.finalQS != nil || len(task.finalQSs) > 0 {
				return fmt.Errorf(
					"orm: '%s' lookup was already seen with a different queryset; adjust the ordering of your lookups",
					fullKey,
				)
			}
			continue
		}

		groups := []prefetchGroup{{model: rootModel, objs: roots}}
		for level, part := range task.throughParts {
			if len(groups) == 0 {
				break
			}
			pathKey := task.prefetchTo(level)
			if prev, ok := done[pathKey]; ok {
				groups = prev
				continue
			}
			next, nestedSpecs, err := prefetchLevel(ctx, db, &task, level, groups, part)
			if err != nil {
				return err
			}
			done[pathKey] = next
			if len(nestedSpecs) > 0 {
				added, err := normalizePrefetchTasks(nestedSpecs, pathKey)
				if err != nil {
					return err
				}
				queue = append(added, queue...)
			}
			groups = next
		}
	}
	return nil
}

// prefetchLevel loads the relation part of every group of instances at level
// of task. It returns the instances it reached, grouped by model, and the
// nested lookups of the custom queryset, which join the queue.
func prefetchLevel(
	ctx context.Context,
	db *DB,
	task *prefetchTask,
	level int,
	groups []prefetchGroup,
	part string,
) ([]prefetchGroup, []Prefetch, error) {
	isLast := level == len(task.throughParts)-1
	var (
		querysets []*query
		toAttr    string
	)
	if isLast {
		if task.finalQS != nil {
			querysets = append(querysets, task.finalQS)
		}
		querysets = append(querysets, task.finalQSs...)
		toAttr = task.finalToAttr
	}
	isToAttr := toAttr != ""

	var (
		next        []prefetchGroup
		nestedSpecs []Prefetch
		generic     []genericJob
		customQ     *query
		customDone  bool
	)
	// singleQuery returns the custom queryset of a relation other than a
	// generic foreign key. Its nested prefetches are stripped so they merge
	// into the queue with pathKey as prefix, matching Django's deduplication.
	singleQuery := func() (*query, error) {
		if customDone {
			return customQ, nil
		}
		customDone = true
		if len(querysets) > 1 {
			return nil, fmt.Errorf("orm: prefetch_related lookup '%s' takes one queryset, got %d",
				task.rawLookup, len(querysets))
		}
		if len(querysets) == 1 {
			customQ = querysets[0]
			if len(customQ.prefetchRelated) > 0 {
				nestedSpecs = append([]Prefetch(nil), customQ.prefetchRelated...)
				customQ = customQ.clone()
				customQ.prefetchRelated = nil
			}
		}
		return customQ, nil
	}

	for _, g := range groups {
		step := resolvePrefetchStep(g.model, g.objs, part)
		switch step.kind {
		case stepNone:
			if step.nonRelField {
				return nil, nil, fmt.Errorf(
					"orm: '%s' does not resolve to an item that supports prefetching - this is an invalid parameter to prefetch_related()",
					task.rawLookup,
				)
			}
			return nil, nil, fmt.Errorf(
				"orm: Cannot find '%s' on %s object, '%s' is an invalid parameter to prefetch_related()",
				part, g.model.Name(), task.rawLookup,
			)
		case stepCachedAttr:
			if isLast {
				return nil, nil, fmt.Errorf(
					"orm: '%s' does not resolve to an item that supports prefetching - this is an invalid parameter to prefetch_related()",
					task.rawLookup,
				)
			}
			m, objs := collectCachedAttr(g.model, g.objs, part)
			next = addPrefetchGroup(next, m, objs)
			continue
		}
		if isToAttr && toAttrConflicts(g.model, toAttr) {
			return nil, nil, fmt.Errorf("orm: to_attr=%s conflicts with a field on the %s model", toAttr, g.model.Name())
		}
		cacheKey := step.cacheName
		if isToAttr {
			cacheKey = toAttr
		}

		if step.kind == stepObject {
			reached, err := prefetchObjects(ctx, db, g, step, querysets, cacheKey, isToAttr)
			if err != nil {
				return nil, nil, err
			}
			for _, r := range reached {
				next = addPrefetchGroup(next, r.model, r.objs)
			}
			continue
		}

		relQ, err := singleQuery()
		if err != nil {
			return nil, nil, err
		}
		if relQ != nil && relQ.model != step.targetModel {
			return nil, nil, fmt.Errorf(
				"orm: prefetch_related queryset for '%s' is for %s, expected %s",
				task.rawLookup, relQ.model.Name(), step.targetModel.Name(),
			)
		}
		toFetch := filterUnfetched(g.model, g.objs, step, cacheKey, isToAttr)
		if step.kind == stepGeneric {
			// Generic relations of several models load with one query.
			generic = append(generic, genericJob{group: g, step: step, parents: toFetch, cacheKey: cacheKey})
			continue
		}
		var objs []reflect.Value
		if len(toFetch) > 0 {
			objs, err = prefetchOneLevel(ctx, db, g.model, toFetch, step, relQ, cacheKey, isToAttr, nestedSpecs)
			if err != nil {
				return nil, nil, err
			}
			if len(toFetch) < len(g.objs) {
				objs = collectStepObjects(g.model, g.objs, step, cacheKey, isToAttr)
			}
		} else {
			objs = collectStepObjects(g.model, g.objs, step, cacheKey, isToAttr)
		}
		next = addPrefetchGroup(next, step.targetModel, objs)
	}

	if len(generic) > 0 {
		if err := prefetchGeneric(ctx, db, generic, customQ); err != nil {
			return nil, nil, err
		}
		for _, job := range generic {
			objs := collectStepObjects(job.group.model, job.group.objs, job.step, job.cacheKey, isToAttr)
			next = addPrefetchGroup(next, job.step.targetModel, objs)
		}
	}
	return next, nestedSpecs, nil
}
