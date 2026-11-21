package orm

import (
	"context"
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
	case stepReverseFK, stepForwardM2M, stepReverseM2M:
		_, _, ok := getPrefetchListCache(parentModel, sv, cacheKey)
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
		case stepReverseFK, stepForwardM2M, stepReverseM2M:
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

// nestedPrefetchesField reports whether any nested PrefetchSpec targets attr
// as its first segment.
func nestedPrefetchesField(nested []PrefetchSpec, attr string) bool {
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
	nestedSpecs []PrefetchSpec,
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
	nestedSpecs []PrefetchSpec,
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
	nestedSpecs []PrefetchSpec,
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
	nestedSpecs []PrefetchSpec,
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
