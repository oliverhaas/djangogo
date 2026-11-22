package orm

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
)

// filterUnfetched returns the subset of objs that do not yet have step cached
// under cacheKey.
func filterUnfetched(parentModel *Model, objs []reflect.Value, step prefetchStep, cacheKey string, isToAttr bool) []reflect.Value {
	var need []reflect.Value
	for _, sv := range objs {
		if !isStepFetched(parentModel, sv, step, cacheKey, isToAttr) {
			need = append(need, sv)
		}
	}
	return need
}

// isStepFetched reports whether sv already holds a cached value for step under
// cacheKey.
func isStepFetched(parentModel *Model, sv reflect.Value, step prefetchStep, cacheKey string, isToAttr bool) bool {
	if isToAttr {
		return hasRelCacheKey(parentModel, sv, cacheKey)
	}
	switch step.kind {
	case stepForwardFK, stepForwardO2O:
		fv := sv.Field(step.field.Index)
		if keyed, ok := fv.Interface().(interface{ PK() int64 }); ok && keyed.PK() == 0 {
			return true
		}
		objFn := fv.MethodByName("Object")
		if objFn.IsValid() {
			res := objFn.Call(nil)
			return res[1].Bool()
		}
		return false
	case stepReverseO2O:
		_, _, ok := getReverseOneCache(parentModel, sv, cacheKey)
		return ok
	case stepReverseFK, stepForwardM2M, stepReverseM2M, stepGeneric, stepGenericOwner:
		_, _, ok := getPrefetchListCache(parentModel, sv, cacheKey)
		return ok
	case stepObject:
		_, ok := reflect.Zero(step.private.Type).Interface().(ObjectPrefetcher).CachedObject(step.private, sv.Addr().Interface())
		return ok
	default:
		return false
	}
}

// collectStepObjects gathers the addressable child struct reflect.Values for
// step from all parents in objs after step has been populated.
func collectStepObjects(parentModel *Model, objs []reflect.Value, step prefetchStep, cacheKey string, isToAttr bool) []reflect.Value {
	var out []reflect.Value
	seenPtr := make(map[uintptr]bool)
	appendPtr := func(p reflect.Value) {
		if !p.IsValid() || p.Kind() != reflect.Pointer || p.IsNil() {
			return
		}
		addr := p.Pointer()
		if seenPtr[addr] {
			return
		}
		seenPtr[addr] = true
		out = append(out, p.Elem())
	}
	appendSlice := func(items any) {
		if items == nil {
			return
		}
		sv := reflect.ValueOf(items)
		if sv.Kind() != reflect.Slice {
			return
		}
		for i := range sv.Len() {
			out = append(out, sv.Index(i))
		}
	}

	for _, sv := range objs {
		if isToAttr {
			if step.single {
				if obj, missing, ok := getReverseOneCache(parentModel, sv, cacheKey); ok && !missing && obj != nil {
					appendPtr(reflect.ValueOf(obj))
				}
			} else {
				if items, _, ok := getPrefetchListCache(parentModel, sv, cacheKey); ok {
					appendSlice(items)
				}
			}
			continue
		}
		switch step.kind {
		case stepForwardFK, stepForwardO2O:
			objFn := sv.Field(step.field.Index).MethodByName("Object")
			if objFn.IsValid() {
				res := objFn.Call(nil)
				if res[1].Bool() {
					appendPtr(res[0])
				}
			}
		case stepReverseO2O:
			if obj, missing, ok := getReverseOneCache(parentModel, sv, cacheKey); ok && !missing && obj != nil {
				appendPtr(reflect.ValueOf(obj))
			}
		case stepReverseFK, stepForwardM2M, stepReverseM2M, stepGeneric, stepGenericOwner:
			if items, _, ok := getPrefetchListCache(parentModel, sv, cacheKey); ok {
				appendSlice(items)
			}
		}
	}
	return out
}

// collectCachedAttr gathers the target *Model and addressable struct values
// from an already-populated ToAttr entry in RelCache.
func collectCachedAttr(parentModel *Model, objs []reflect.Value, attr string) (*Model, []reflect.Value) {
	var (
		targetModel *Model
		out         []reflect.Value
		seenPtr     = make(map[uintptr]bool)
	)
	for _, sv := range objs {
		if items, tm, ok := getPrefetchListCache(parentModel, sv, attr); ok {
			if targetModel == nil {
				targetModel = tm
			}
			if items != nil {
				rv := reflect.ValueOf(items)
				for i := range rv.Len() {
					out = append(out, rv.Index(i))
				}
			}
			continue
		}
		if e, ok := getReverseOneCacheEntry(parentModel, sv, attr); ok {
			if targetModel == nil {
				targetModel = e.model
			}
			if !e.missing && e.obj != nil {
				p := reflect.ValueOf(e.obj)
				if p.Kind() == reflect.Pointer && !p.IsNil() {
					addr := p.Pointer()
					if !seenPtr[addr] {
						seenPtr[addr] = true
						out = append(out, p.Elem())
					}
				}
			}
		}
	}
	return targetModel, out
}

// nestedPrefetchesField reports whether any nested Prefetch targets attr
// as its first segment.
func nestedPrefetchesField(nested []Prefetch, attr string) bool {
	for _, s := range nested {
		first, _, _ := strings.Cut(s.Lookup, lookupSep)
		if first == attr {
			return true
		}
	}
	return false
}

// prefetchOneLevel runs the batched IN query for one relation hop and attaches
// the results onto parents.
func prefetchOneLevel(
	ctx context.Context,
	db *DB,
	parentModel *Model,
	parents []reflect.Value,
	step prefetchStep,
	customQ *query,
	cacheKey string,
	isToAttr bool,
	nestedSpecs []Prefetch,
) ([]reflect.Value, error) {
	switch step.kind {
	case stepForwardFK, stepForwardO2O:
		return prefetchForwardToOne(ctx, db, parentModel, parents, step, customQ, cacheKey, isToAttr, nestedSpecs)
	case stepReverseO2O:
		return prefetchReverseO2O(ctx, db, parentModel, parents, step, customQ, cacheKey, isToAttr, nestedSpecs)
	case stepReverseFK:
		return prefetchReverseFK(ctx, db, parentModel, parents, step, customQ, cacheKey, isToAttr, nestedSpecs)
	case stepForwardM2M, stepReverseM2M:
		return prefetchM2M(ctx, db, parentModel, parents, step, customQ, cacheKey)
	case stepGenericOwner:
		return prefetchGenericOwners(ctx, db, parentModel, parents, step, customQ, cacheKey)
	default:
		return nil, fmt.Errorf("orm: unsupported prefetch step kind %d", step.kind)
	}
}

// prefetchBatchSize returns the safe IN-clause chunk size for db's dialect.
func prefetchBatchSize(db *DB) int {
	if limit := dialectFeatures(db.dialect).MaxQueryParams; limit > 10 {
		return limit - 10
	}
	return 1000
}

// prefetchForwardToOne fetches targets for a forward FK or O2O in one batched
// IN query and attaches each loaded pointer onto its parent(s).
func prefetchForwardToOne(
	ctx context.Context,
	db *DB,
	parentModel *Model,
	parents []reflect.Value,
	step prefetchStep,
	customQ *query,
	cacheKey string,
	isToAttr bool,
	nestedSpecs []Prefetch,
) ([]reflect.Value, error) {
	targetModel := step.targetModel
	seenPK := make(map[int64]bool, len(parents))
	pks := make([]int64, 0, len(parents))
	parentFKPKs := make([]int64, len(parents))
	for i, sv := range parents {
		keyed, ok := sv.Field(step.field.Index).Interface().(interface{ PK() int64 })
		if !ok {
			continue
		}
		pk := keyed.PK()
		parentFKPKs[i] = pk
		if pk != 0 && !seenPK[pk] {
			seenPK[pk] = true
			pks = append(pks, pk)
		}
	}

	targetByPK := make(map[int64]reflect.Value, len(pks))
	var nextObjs []reflect.Value
	if len(pks) > 0 {
		base := customQ
		if base == nil {
			base = newQuery(db, targetModel)
		}
		bs := prefetchBatchSize(db)
		for start := 0; start < len(pks); start += bs {
			chunk := pks[start:min(start+bs, len(pks))]
			q := base.filter([]any{"pk__in", chunk}, false)
			err := q.fetch(ctx, func(row reflect.Value, _ *rowPlan, _ []any) {
				pk, pkErr := extractModelPKInt64(targetModel, row)
				if pkErr != nil || pk == 0 {
					return
				}
				ptr := row.Addr()
				targetByPK[pk] = ptr
				nextObjs = append(nextObjs, row)
			})
			if err != nil {
				return nil, err
			}
		}
	}

	var backRev *ReverseRelation
	if step.kind == stepForwardO2O && !isToAttr {
		if rev := targetModel.reverseForField(step.field); rev != nil && !rev.Hidden && !nestedPrefetchesField(nestedSpecs, rev.Name) {
			backRev = rev
		}
	}

	for i, sv := range parents {
		fkPK := parentFKPKs[i]
		targetPtr, found := targetByPK[fkPK]
		if isToAttr {
			if found {
				setReverseOneCacheModel(parentModel, sv, cacheKey, targetPtr.Interface(), targetModel, false)
			} else {
				setReverseOneCacheModel(parentModel, sv, cacheKey, nil, targetModel, true)
			}
			continue
		}
		if fkPK == 0 {
			continue
		}
		if found {
			setForwardObject(sv, step.field, targetPtr, fkPK)
			if backRev != nil {
				if _, _, already := getReverseOneCache(targetModel, targetPtr.Elem(), backRev.Name); !already {
					setReverseOneCacheModel(targetModel, targetPtr.Elem(), backRev.Name, sv.Addr().Interface(), parentModel, false)
				}
			}
		} else {
			// Record loaded=true with nil object so Fetch returns ErrDoesNotExist
			// without issuing a fallback query (e.g. custom queryset filtered out target).
			setForwardObject(sv, step.field, reflect.Zero(reflect.PointerTo(targetModel.goType)), fkPK)
		}
	}
	return nextObjs, nil
}

// prefetchReverseO2O fetches reverse one-to-one targets in one batched IN
// query and stores each result (or missing marker) in the parent's RelCache.
func prefetchReverseO2O(
	ctx context.Context,
	db *DB,
	parentModel *Model,
	parents []reflect.Value,
	step prefetchStep,
	customQ *query,
	cacheKey string,
	isToAttr bool,
	nestedSpecs []Prefetch,
) ([]reflect.Value, error) {
	targetModel := step.targetModel
	seenPK := make(map[any]bool, len(parents))
	pks := make([]any, 0, len(parents))
	parentPKs := make([]any, len(parents))
	for i, sv := range parents {
		pk, zero, err := extractModelPK(parentModel, sv)
		if err != nil {
			return nil, err
		}
		if zero {
			continue
		}
		parentPKs[i] = pk
		if !seenPK[pk] {
			seenPK[pk] = true
			pks = append(pks, pk)
		}
	}

	childByParentPK := make(map[any]reflect.Value, len(pks))
	var nextObjs []reflect.Value
	if len(pks) > 0 {
		base := customQ
		if base == nil {
			base = newQuery(db, targetModel)
		}
		// The foreign key matches a row to its parent.
		base = base.loading(step.rev.Field)
		bs := prefetchBatchSize(db)
		fkFilterKey := step.rev.Field.Column + "__in"
		for start := 0; start < len(pks); start += bs {
			chunk := pks[start:min(start+bs, len(pks))]
			q := base.filter([]any{fkFilterKey, chunk}, false)
			err := q.fetch(ctx, func(row reflect.Value, _ *rowPlan, _ []any) {
				fkPK, ok := extractForwardFKKey(row, step.rev.Field)
				if !ok {
					return
				}
				childByParentPK[fkPK] = row.Addr()
				nextObjs = append(nextObjs, row)
			})
			if err != nil {
				return nil, err
			}
		}
	}

	populateBackRef := !isToAttr && !nestedPrefetchesField(nestedSpecs, step.rev.Field.Attr)
	for i, sv := range parents {
		pk := parentPKs[i]
		childPtr, found := childByParentPK[pk]
		if !found {
			setReverseOneCacheModel(parentModel, sv, cacheKey, nil, targetModel, true)
			continue
		}
		setReverseOneCacheModel(parentModel, sv, cacheKey, childPtr.Interface(), targetModel, false)
		if populateBackRef && !isForwardLoaded(childPtr.Elem(), step.rev.Field) {
			if pkInt, ok := pk.(int64); ok {
				setForwardObject(childPtr.Elem(), step.rev.Field, sv.Addr(), pkInt)
			}
		}
	}
	return nextObjs, nil
}

// prefetchReverseFK fetches reverse foreign-key children in one batched IN
// query and stores each parent's typed []Child slice in RelCache.
func prefetchReverseFK(
	ctx context.Context,
	db *DB,
	parentModel *Model,
	parents []reflect.Value,
	step prefetchStep,
	customQ *query,
	cacheKey string,
	isToAttr bool,
	nestedSpecs []Prefetch,
) ([]reflect.Value, error) {
	targetModel := step.targetModel
	seenPK := make(map[any]bool, len(parents))
	pks := make([]any, 0, len(parents))
	parentPKs := make([]any, len(parents))
	for i, sv := range parents {
		pk, zero, err := extractModelPK(parentModel, sv)
		if err != nil {
			return nil, err
		}
		if zero {
			continue
		}
		parentPKs[i] = pk
		if !seenPK[pk] {
			seenPK[pk] = true
			pks = append(pks, pk)
		}
	}

	childrenByParent := make(map[any][]reflect.Value, len(pks))
	if len(pks) > 0 {
		base := customQ
		if base == nil {
			base = newQuery(db, targetModel)
		}
		// The foreign key matches a row to its parent.
		base = base.loading(step.rev.Field)
		bs := prefetchBatchSize(db)
		fkFilterKey := step.rev.Field.Column + "__in"
		for start := 0; start < len(pks); start += bs {
			chunk := pks[start:min(start+bs, len(pks))]
			q := base.filter([]any{fkFilterKey, chunk}, false)
			err := q.fetch(ctx, func(row reflect.Value, _ *rowPlan, _ []any) {
				fkPK, ok := extractForwardFKKey(row, step.rev.Field)
				if !ok {
					return
				}
				childrenByParent[fkPK] = append(childrenByParent[fkPK], row)
			})
			if err != nil {
				return nil, err
			}
		}
	}

	populateBackRef := !isToAttr && !nestedPrefetchesField(nestedSpecs, step.rev.Field.Attr)
	sliceType := reflect.SliceOf(targetModel.goType)
	var nextObjs []reflect.Value
	for i, sv := range parents {
		pk := parentPKs[i]
		rows := childrenByParent[pk]
		typedSlice := reflect.MakeSlice(sliceType, len(rows), len(rows))
		pkInt, hasIntPK := pk.(int64)
		for j, child := range rows {
			// Ensure shared RelCache pointer if child is copied across parents.
			getRelCache(targetModel, child, true)
			slot := typedSlice.Index(j)
			slot.Set(child)
			if populateBackRef && hasIntPK && !isForwardLoaded(slot, step.rev.Field) {
				setForwardObject(slot, step.rev.Field, sv.Addr(), pkInt)
			}
			nextObjs = append(nextObjs, slot)
		}
		setPrefetchListCache(parentModel, sv, cacheKey, typedSlice.Interface(), targetModel)
	}
	return nextObjs, nil
}

// genericJob is one group of parents whose generic relation PrefetchRelated
// loads.
type genericJob struct {
	group    prefetchGroup
	step     prefetchStep
	parents  []reflect.Value // the parents without a cached result
	cacheKey string
}

// genericRelKey identifies the fields of a generic foreign key on the related
// model of a generic relation.
type genericRelKey struct {
	related     *Model
	contentType *Field
	objectID    *Field
}

// prefetchGeneric loads the rows of the generic relations of jobs and stores
// each parent's typed []Child slice in RelCache. The jobs that share a
// related model load with one query per chunk, as Django ORs one condition
// per content type.
func prefetchGeneric(ctx context.Context, db *DB, jobs []genericJob, customQ *query) error {
	var order []genericRelKey
	byKey := make(map[genericRelKey][]genericJob)
	for _, job := range jobs {
		if len(job.parents) == 0 {
			continue
		}
		gr := job.step.generic
		key := genericRelKey{related: gr.Related, contentType: gr.ContentType, objectID: gr.ObjectID}
		if _, ok := byKey[key]; !ok {
			order = append(order, key)
		}
		byKey[key] = append(byKey[key], job)
	}
	for _, key := range order {
		if err := prefetchGenericRows(ctx, db, byKey[key], customQ); err != nil {
			return err
		}
	}
	return nil
}

// genericJobKeys holds the primary keys of the parents of one genericJob.
type genericJobKeys struct {
	ctID      int64
	parentPKs []any // per parent, nil for an unsaved one
	pks       []any // distinct
	children  map[any][]reflect.Value
}

// prefetchGenericRows loads the rows of jobs, whose generic relations share
// the related model and its fields.
func prefetchGenericRows(ctx context.Context, db *DB, jobs []genericJob, customQ *query) error {
	gr := jobs[0].step.generic
	targetModel := gr.Related
	multi := len(jobs) > 1
	keys := make([]genericJobKeys, len(jobs))
	jobByCT := make(map[int64]int, len(jobs))
	for i, job := range jobs {
		k := &keys[i]
		k.parentPKs = make([]any, len(job.parents))
		k.children = make(map[any][]reflect.Value)
		seen := make(map[any]bool, len(job.parents))
		for j, sv := range job.parents {
			pk, zero, err := extractModelPK(job.group.model, sv)
			if err != nil {
				return err
			}
			if zero {
				continue
			}
			k.parentPKs[j] = pk
			if !seen[pk] {
				seen[pk] = true
				k.pks = append(k.pks, pk)
			}
		}
		if multi {
			id, err := job.step.generic.impl.ContentTypeID(ctx, db, job.group.model)
			if err != nil {
				return err
			}
			if k.ctID, err = toInt64(id); err != nil {
				return fmt.Errorf("orm: content type id %v of %s: %w", id, job.group.model.Name(), err)
			}
			jobByCT[k.ctID] = i
		}
	}

	base := customQ
	if base == nil {
		base = newQuery(db, targetModel)
	}
	// The generic foreign key matches a row to its parent.
	base = base.loading(gr.ContentType, gr.ObjectID)
	var convErr error
	collect := func(row reflect.Value, _ *rowPlan, _ []any) {
		job := 0
		if multi {
			ct, ok := row.Field(gr.ContentType.Index).Interface().(interface{ PK() int64 })
			if !ok {
				return
			}
			if job, ok = jobByCT[ct.PK()]; !ok {
				return
			}
		}
		fv := row.Field(gr.ObjectID.Index)
		if fv.Kind() == reflect.Pointer {
			if fv.IsNil() {
				return
			}
			fv = fv.Elem()
		}
		key, err := convertPrefetchParentKey(jobs[job].group.model.pk, fv.Interface())
		if err != nil {
			convErr = err
			return
		}
		keys[job].children[key] = append(keys[job].children[key], row)
	}
	run := func(q *query) error {
		if err := q.fetch(ctx, collect); err != nil {
			return err
		}
		return convErr
	}

	bs := prefetchBatchSize(db)
	if !multi {
		pks := keys[0].pks
		for start := 0; start < len(pks); start += bs {
			if err := run(base.genericFilter(gr, pks[start:min(start+bs, len(pks))])); err != nil {
				return err
			}
		}
	} else {
		var (
			conds []Cond
			size  int
		)
		flush := func() error {
			if len(conds) == 0 {
				return nil
			}
			cond := conds[0].Or(conds[1:]...)
			conds, size = nil, 0
			return run(base.filter([]any{cond}, false))
		}
		for _, k := range keys {
			for start := 0; start < len(k.pks); start += bs {
				chunk := k.pks[start:min(start+bs, len(k.pks))]
				if size+len(chunk) > bs {
					if err := flush(); err != nil {
						return err
					}
				}
				conds = append(conds, Q(gr.ContentType.Attr, k.ctID, gr.ObjectID.Attr+"__in", chunk))
				size += len(chunk)
			}
		}
		if err := flush(); err != nil {
			return err
		}
	}

	sliceType := reflect.SliceOf(targetModel.goType)
	for i, job := range jobs {
		for j, sv := range job.parents {
			rows := keys[i].children[keys[i].parentPKs[j]]
			typedSlice := reflect.MakeSlice(sliceType, len(rows), len(rows))
			for n, child := range rows {
				getRelCache(targetModel, child, true)
				typedSlice.Index(n).Set(child)
			}
			setPrefetchListCache(job.group.model, sv, job.cacheKey, typedSlice.Interface(), targetModel)
		}
	}
	return nil
}

// prefetchGenericOwners loads, for each parent, an instance of the related
// model of a generic relation, the row of the model that declares the
// relation that its generic foreign key refers to. Django reaches it through
// the related_query_name of the GenericRelation. Each parent's typed slice,
// with one row at most, goes to RelCache.
func prefetchGenericOwners(
	ctx context.Context,
	db *DB,
	parentModel *Model,
	parents []reflect.Value,
	step prefetchStep,
	customQ *query,
	cacheKey string,
) ([]reflect.Value, error) {
	gr := step.generic
	targetModel := step.targetModel
	id, err := gr.impl.ContentTypeID(ctx, db, targetModel)
	if err != nil {
		return nil, err
	}
	ctID, err := toInt64(id)
	if err != nil {
		return nil, fmt.Errorf("orm: content type id %v of %s: %w", id, targetModel.Name(), err)
	}
	keys := make([]any, len(parents))
	var pks []any
	seen := make(map[any]bool, len(parents))
	for i, sv := range parents {
		ct, ok := sv.Field(gr.ContentType.Index).Interface().(interface{ PK() int64 })
		if !ok || ct.PK() != ctID {
			continue
		}
		fv := sv.Field(gr.ObjectID.Index)
		if fv.Kind() == reflect.Pointer {
			if fv.IsNil() {
				continue
			}
			fv = fv.Elem()
		}
		key, err := convertPrefetchParentKey(targetModel.pk, fv.Interface())
		if err != nil {
			return nil, fmt.Errorf("orm: %s.%s: %w", parentModel.Name(), gr.ObjectID.Name, err)
		}
		keys[i] = key
		if key != nil && !seen[key] {
			seen[key] = true
			pks = append(pks, key)
		}
	}

	byPK := make(map[any]reflect.Value, len(pks))
	base := customQ
	if base == nil {
		base = newQuery(db, targetModel)
	}
	bs := prefetchBatchSize(db)
	for start := 0; start < len(pks); start += bs {
		var pkErr error
		err := base.filter([]any{"pk__in", pks[start:min(start+bs, len(pks))]}, false).fetch(ctx,
			func(row reflect.Value, _ *rowPlan, _ []any) {
				pk, _, err := extractModelPK(targetModel, row)
				if err != nil {
					pkErr = err
					return
				}
				byPK[pk] = row
			})
		if err == nil {
			err = pkErr
		}
		if err != nil {
			return nil, err
		}
	}

	sliceType := reflect.SliceOf(targetModel.goType)
	var nextObjs []reflect.Value
	for i, sv := range parents {
		row, ok := byPK[keys[i]]
		n := 0
		if ok {
			n = 1
		}
		typedSlice := reflect.MakeSlice(sliceType, n, n)
		if ok {
			slot := typedSlice.Index(0)
			slot.Set(row)
			getRelCache(targetModel, slot, true)
			nextObjs = append(nextObjs, slot)
		}
		setPrefetchListCache(parentModel, sv, cacheKey, typedSlice.Interface(), targetModel)
	}
	return nextObjs, nil
}

// prefetchObjects loads the objects that the private field of step refers to
// on the instances of g, with one query per model, and caches each object on
// its instance. querysets hold at most one custom queryset per model, and
// their nested prefetches run on the objects they load. It returns every
// object the instances refer to, grouped by model.
func prefetchObjects(
	ctx context.Context,
	db *DB,
	g prefetchGroup,
	step prefetchStep,
	querysets []*query,
	cacheKey string,
	isToAttr bool,
) ([]prefetchGroup, error) {
	pf := step.private
	impl := reflect.Zero(pf.Type).Interface().(ObjectPrefetcher)
	custom := make(map[*Model]*query, len(querysets))
	for _, q := range querysets {
		if _, dup := custom[q.model]; dup {
			return nil, errors.New("orm: only one queryset is allowed for each content type")
		}
		custom[q.model] = q
	}

	type target struct {
		model *Model
		pk    any
	}
	var models []*Model
	pksByModel := make(map[*Model][]any)
	seen := make(map[target]bool)
	targets := make([]target, len(g.objs))
	fetched := make([]bool, len(g.objs))
	for i, sv := range g.objs {
		if isStepFetched(g.model, sv, step, cacheKey, isToAttr) {
			fetched[i] = true
			continue
		}
		m, raw, err := impl.PrefetchTarget(ctx, db, pf, sv.Addr().Interface())
		if err != nil {
			return nil, err
		}
		if m == nil || raw == nil {
			continue
		}
		pk, err := convertPrefetchParentKey(m.pk, raw)
		if err != nil {
			return nil, fmt.Errorf("orm: %s.%s: %w", g.model.Name(), pf.Name, err)
		}
		t := target{model: m, pk: pk}
		targets[i] = t
		if seen[t] {
			continue
		}
		seen[t] = true
		if _, ok := pksByModel[m]; !ok {
			models = append(models, m)
		}
		pksByModel[m] = append(pksByModel[m], pk)
	}

	loaded := make(map[target]any)
	bs := prefetchBatchSize(db)
	for _, m := range models {
		base := custom[m]
		if base == nil {
			base = newQuery(db, m)
		}
		var nested []Prefetch
		if len(base.prefetchRelated) > 0 {
			nested = base.prefetchRelated
			base = base.clone()
			base.prefetchRelated = nil
		}
		var rows []reflect.Value
		pks := pksByModel[m]
		for start := 0; start < len(pks); start += bs {
			q := base.filter([]any{"pk__in", pks[start:min(start+bs, len(pks))]}, false)
			var pkErr error
			err := q.fetch(ctx, func(row reflect.Value, _ *rowPlan, _ []any) {
				pk, _, err := extractModelPK(m, row)
				if err != nil {
					pkErr = err
					return
				}
				loaded[target{model: m, pk: pk}] = row.Addr().Interface()
				rows = append(rows, row)
			})
			if err == nil {
				err = pkErr
			}
			if err != nil {
				return nil, err
			}
		}
		if len(nested) > 0 {
			if err := prefetchRelatedObjects(ctx, db, m, rows, nested); err != nil {
				return nil, err
			}
		}
	}

	var reached []prefetchGroup
	seenObj := make(map[any]bool)
	for i, sv := range g.objs {
		var obj any
		if fetched[i] {
			if isToAttr {
				obj, _, _ = getReverseOneCache(g.model, sv, cacheKey)
			} else {
				obj, _ = impl.CachedObject(pf, sv.Addr().Interface())
			}
		} else {
			if targets[i].model != nil {
				obj = loaded[targets[i]]
			}
			switch {
			case isToAttr && obj != nil:
				setReverseOneCacheModel(g.model, sv, cacheKey, obj, targets[i].model, false)
			case isToAttr:
				setReverseOneCacheModel(g.model, sv, cacheKey, nil, nil, true)
			default:
				impl.CacheObject(pf, sv.Addr().Interface(), obj)
			}
		}
		p := reflect.ValueOf(obj)
		if obj == nil || p.Kind() != reflect.Pointer || p.IsNil() || seenObj[obj] {
			continue
		}
		seenObj[obj] = true
		m, ok := db.Registry().ModelOf(obj)
		if !ok {
			continue
		}
		reached = addPrefetchGroup(reached, m, []reflect.Value{p.Elem()})
	}
	return reached, nil
}

// prefetchM2M fetches forward or reverse many-to-many targets in one batched
// INNER JOIN + IN query and stores each parent's typed []Target slice in
// RelCache.
func prefetchM2M(
	ctx context.Context,
	db *DB,
	parentModel *Model,
	parents []reflect.Value,
	step prefetchStep,
	customQ *query,
	cacheKey string,
) ([]reflect.Value, error) {
	targetModel := step.targetModel
	seenPK := make(map[any]bool, len(parents))
	pks := make([]any, 0, len(parents))
	parentPKs := make([]any, len(parents))
	for i, sv := range parents {
		pk, zero, err := extractModelPK(parentModel, sv)
		if err != nil {
			return nil, err
		}
		if zero {
			continue
		}
		parentPKs[i] = pk
		if !seenPK[pk] {
			seenPK[pk] = true
			pks = append(pks, pk)
		}
	}

	rowsByParent := make(map[any][]reflect.Value, len(pks))
	if len(pks) > 0 {
		base := customQ
		if base == nil {
			base = newQuery(db, targetModel)
		}
		bs := prefetchBatchSize(db)
		parentPKField := parentModel.PrimaryKey()
		for start := 0; start < len(pks); start += bs {
			chunk := pks[start:min(start+bs, len(pks))]
			q := base.clone()
			q.m2mPrefetch = &m2mPrefetchFilter{
				through:       step.through,
				targetFK:      step.targetFK,
				sourceFK:      step.sourceFK,
				parentPKField: parentPKField,
				parentPKs:     chunk,
			}
			err := q.fetchWithParent(ctx, func(row reflect.Value, parentKey any, _ *rowPlan, _ []any) {
				if parentKey == nil {
					return
				}
				rowsByParent[parentKey] = append(rowsByParent[parentKey], row)
			})
			if err != nil {
				return nil, err
			}
		}
	}

	sliceType := reflect.SliceOf(targetModel.goType)
	var nextObjs []reflect.Value
	for i, sv := range parents {
		pk := parentPKs[i]
		rows := rowsByParent[pk]
		typedSlice := reflect.MakeSlice(sliceType, len(rows), len(rows))
		for j, child := range rows {
			getRelCache(targetModel, child, true)
			slot := typedSlice.Index(j)
			slot.Set(child)
			nextObjs = append(nextObjs, slot)
		}
		setPrefetchListCache(parentModel, sv, cacheKey, typedSlice.Interface(), targetModel)
	}
	return nextObjs, nil
}

// extractForwardFKKey returns the non-zero primary key stored in forward FK/O2O
// field f of struct value sv.
func extractForwardFKKey(sv reflect.Value, f *Field) (any, bool) {
	if keyed, ok := sv.Field(f.Index).Interface().(interface{ PK() int64 }); ok {
		pk := keyed.PK()
		if pk == 0 {
			return nil, false
		}
		return pk, true
	}
	return nil, false
}

// isForwardLoaded reports whether forward FK/O2O field f on sv already holds a
// loaded object.
func isForwardLoaded(sv reflect.Value, f *Field) bool {
	objFn := sv.Field(f.Index).MethodByName("Object")
	if !objFn.IsValid() {
		return false
	}
	res := objFn.Call(nil)
	return res[1].Bool()
}

// convertPrefetchParentKey normalizes a raw driver value scanned from a
// through-table parent FK column into the exact Go key type returned by
// extractModelPK for parentPKField.
func convertPrefetchParentKey(parentPKField *Field, raw any) (any, error) {
	if raw == nil || parentPKField == nil {
		return nil, nil
	}
	if parentPKField.Rel != nil {
		return toInt64(raw)
	}
	switch parentPKField.Kind {
	case KindAuto, KindInt, KindSmallInt, KindBigInt, KindPositiveSmallInt, KindPositiveInt, KindPositiveBigInt:
		return toInt64(raw)
	case KindChar, KindText, KindSlug, KindEmail, KindURL:
		if b, ok := raw.([]byte); ok {
			return string(b), nil
		}
		if s, ok := raw.(string); ok {
			return s, nil
		}
		return fmt.Sprint(raw), nil
	case KindUUID:
		return convertKindValue(raw, typeUUID)
	default:
		return normalizePK(parentPKField, raw), nil
	}
}
