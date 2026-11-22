package orm

import (
	"context"
	"fmt"
	"time"
)

// runInTx runs fn inside the active transaction if one exists, or starts a new
// transaction via db.Atomic. Unlike an Atomic block without a savepoint, an
// error of fn does not mark the active transaction for rollback: Django runs
// the related manager's create(), get_or_create() and update_or_create()
// outside an atomic block, and only their writes mark it.
func runInTx(ctx context.Context, db *DB, fn func(context.Context) error) error {
	if txStateFrom(ctx) != nil {
		return fn(ctx)
	}
	return db.Atomic(ctx, fn)
}

// normalizeAndDedupePKs normalizes raw PK values against targetModel and
// removes duplicates while preserving first-seen order.
func (c *manyCore) normalizeAndDedupePKs(raw []any) ([]any, error) {
	seen := make(map[any]bool, len(raw))
	out := make([]any, 0, len(raw))
	for _, r := range raw {
		pk, err := normalizePKArg(c.db.Registry(), c.targetModel, r)
		if err != nil {
			return nil, err
		}
		if !seen[pk] {
			seen[pk] = true
			out = append(out, pk)
		}
	}
	return out, nil
}

// resolveThroughDefaults evaluates any callable entries in defaults once and
// maps keys to concrete *Field values on c.through.
func (c *manyCore) resolveThroughDefaults(defaults Defaults) ([]*Field, map[*Field]any, error) {
	if len(defaults) == 0 {
		return nil, nil, nil
	}
	keys := sortedKeys(defaults)
	var fields []*Field
	vals := make(map[*Field]any, len(keys))
	for _, k := range keys {
		f, ok := lookupField(c.through, k)
		if !ok {
			return nil, nil, &FieldError{
				Model:   c.through.Name(),
				Message: "invalid field name(s) in through_defaults: '" + k + "'",
			}
		}
		v := defaults[k]
		if fn, ok := v.(func() any); ok {
			v = fn()
		}
		if f.Rel != nil && f.Rel.Target != nil {
			var err error
			v, err = plainValue(c.db.Registry(), f, v)
			if err != nil {
				return nil, nil, err
			}
		}
		if _, exists := vals[f]; !exists {
			fields = append(fields, f)
		}
		vals[f] = v
	}
	return fields, vals, nil
}

// addPKs associates targetPKs with c.ownerPK, using Django's single-query fast
// path when possible and otherwise computing missing PKs and firing signals.
func (c *manyCore) addPKs(ctx context.Context, raw []any, opts ...ManyOption) error {
	if c.err != nil {
		return c.err
	}
	if len(raw) == 0 {
		return nil
	}
	pks, err := c.normalizeAndDedupePKs(raw)
	if err != nil {
		return err
	}
	cfg := parseManyConfig(opts)
	extraFields, extraVals, err := c.resolveThroughDefaults(cfg.throughDefaults)
	if err != nil {
		return err
	}

	autoCreated := c.field.Rel.AutoCreated
	hasListeners := m2mSignals.has(c.declaringModel, c.field, c.through)

	// Fast path: auto-created through table with no m2m_changed listeners.
	if autoCreated && !hasListeners && len(extraFields) == 0 {
		pairs := make([][2]any, 0, len(pks)*2)
		for _, pk := range pks {
			pairs = append(pairs, [2]any{c.ownerPK, pk})
		}
		if c.symmetrical {
			for _, pk := range pks {
				if pk != c.ownerPK {
					pairs = append(pairs, [2]any{pk, c.ownerPK})
				}
			}
		}
		if err := c.insertPairs(ctx, c.sourceFK, c.targetFK, pairs, nil, nil, true); err != nil {
			return err
		}
		c.clearPrefetch()
		return nil
	}

	err = c.db.Atomic(ctx, func(txCtx context.Context) error {
		if err := c.addDirection(txCtx, c.sourceFK, c.targetFK, pks, extraFields, extraVals, autoCreated, hasListeners); err != nil {
			return err
		}
		if c.symmetrical {
			// Symmetrical mirror inserts do not re-emit m2m_changed signals.
			if err := c.addDirection(txCtx, c.targetFK, c.sourceFK, pks, extraFields, extraVals, autoCreated, false); err != nil {
				return err
			}
		}
		return nil
	}, WithoutSavepoint())
	if err != nil {
		return err
	}
	c.clearPrefetch()
	return nil
}

// addDirection inserts missing (ownerPK, pk) rows for one direction, firing
// pre_add and post_add signals when sendSignals is true.
func (c *manyCore) addDirection(
	ctx context.Context,
	srcFK, dstFK *Field,
	pks []any,
	extraFields []*Field,
	extraVals map[*Field]any,
	ignoreConflicts bool,
	sendSignals bool,
) error {
	existing, err := c.existingTargetPKSet(ctx, srcFK, dstFK, pks)
	if err != nil {
		return err
	}
	missing := make([]any, 0, len(pks))
	for _, pk := range pks {
		if !existing[pk] {
			missing = append(missing, pk)
		}
	}

	if sendSignals {
		if err := fireM2MChanged(ctx, c.declaringModel, c.field, M2MChangedEvent{
			Sender:   c.through,
			Instance: c.ownerInstance,
			Action:   M2MPreAdd,
			Reverse:  c.reverse,
			Model:    c.targetModel,
			PKSet:    missing,
		}); err != nil {
			return err
		}
	}

	if len(missing) > 0 {
		pairs := make([][2]any, len(missing))
		for i, pk := range missing {
			pairs[i] = [2]any{c.ownerPK, pk}
		}
		if err := c.insertPairs(ctx, srcFK, dstFK, pairs, extraFields, extraVals, ignoreConflicts); err != nil {
			return err
		}
	}

	if sendSignals {
		if err := fireM2MChanged(ctx, c.declaringModel, c.field, M2MChangedEvent{
			Sender:   c.through,
			Instance: c.ownerInstance,
			Action:   M2MPostAdd,
			Reverse:  c.reverse,
			Model:    c.targetModel,
			PKSet:    missing,
		}); err != nil {
			return err
		}
	}
	return nil
}

// insertPairs inserts through-table rows for pairs in dialect-sized batches.
// Unset extra fields without defaults are omitted from the column list so the
// database enforces NOT NULL constraints as Django expects.
func (c *manyCore) insertPairs(
	ctx context.Context,
	srcFK, dstFK *Field,
	pairs [][2]any,
	extraFields []*Field,
	extraVals map[*Field]any,
	ignoreConflicts bool,
) error {
	if len(pairs) == 0 {
		return nil
	}
	cols := []*Field{srcFK, dstFK}
	included := map[*Field]bool{srcFK: true, dstFK: true}
	vals := make(map[*Field]any, len(extraVals))
	for _, f := range extraFields {
		if !included[f] {
			included[f] = true
			cols = append(cols, f)
			vals[f] = extraVals[f]
		}
	}
	now := c.db.now()
	for _, f := range c.through.fields {
		if included[f] || f.PrimaryKey {
			continue
		}
		switch {
		case (f.AutoNow || f.AutoNowAdd) && isTemporalAutoKind(f.Kind):
			cols = append(cols, f)
			vals[f] = temporalAutoValue(f.Kind, now)
		case f.HasDefault:
			cols = append(cols, f)
			vals[f] = f.Default
		}
	}

	d := c.db.dialect
	bs := batchRows(d, len(cols), 0, len(pairs))
	for start := 0; start < len(pairs); start += bs {
		chunk := pairs[start:min(start+bs, len(pairs))]
		w := newSQLWriter(d)
		w.write("INSERT INTO ")
		w.ident(c.through.Table())
		w.write(" (")
		for i, f := range cols {
			if i > 0 {
				w.write(", ")
			}
			w.ident(f.Column)
		}
		w.write(") VALUES ")
		for r, pair := range chunk {
			if r > 0 {
				w.write(", ")
			}
			w.write("(")
			for i, f := range cols {
				if i > 0 {
					w.write(", ")
				}
				var raw any
				switch i {
				case 0:
					raw = pair[0]
				case 1:
					raw = pair[1]
				default:
					raw = vals[f]
				}
				arg, err := bindField(d, f, raw)
				if err != nil {
					return fmt.Errorf("orm: M2M insert %s: %w", c.through.Table(), err)
				}
				w.param(arg)
			}
			w.write(")")
		}
		if ignoreConflicts {
			w.write(" ON CONFLICT DO NOTHING")
		}
		if _, err := c.db.conn(ctx).ExecContext(ctx, w.String(), w.args...); err != nil {
			return fmt.Errorf("orm: M2M insert %s: %w", c.through.Table(), err)
		}
	}
	return nil
}

// temporalAutoValue returns the stamped Go value for an auto_now or
// auto_now_add field of kind k at timestamp now.
func temporalAutoValue(k Kind, now time.Time) any {
	switch k {
	case KindDate:
		return DateFromTime(now)
	case KindTime:
		return TimeFromTime(now)
	default:
		return now
	}
}

// existingTargetPKSet returns the subset of candidate target PKs already linked
// to c.ownerPK in c.through for the given (srcFK, dstFK) direction.
func (c *manyCore) existingTargetPKSet(ctx context.Context, srcFK, dstFK *Field, candidates []any) (map[any]bool, error) {
	out := make(map[any]bool)
	if len(candidates) == 0 {
		return out, nil
	}
	d := c.db.dialect
	pkField := c.targetModel.PrimaryKey()
	bs := 1000
	if limit := dialectFeatures(d).MaxQueryParams; limit > 1 {
		bs = limit - 1
	}
	ownerArg, err := bindField(d, srcFK, c.ownerPK)
	if err != nil {
		return nil, err
	}
	for start := 0; start < len(candidates); start += bs {
		chunk := candidates[start:min(start+bs, len(candidates))]
		w := newSQLWriter(d)
		w.write("SELECT ")
		w.ident(dstFK.Column)
		w.write(" FROM ")
		w.ident(c.through.Table())
		w.write(" WHERE ")
		w.ident(srcFK.Column)
		w.write(" = ")
		w.param(ownerArg)
		w.write(" AND ")
		w.ident(dstFK.Column)
		w.write(" IN (")
		for i, pk := range chunk {
			if i > 0 {
				w.write(", ")
			}
			arg, err := bindField(d, dstFK, pk)
			if err != nil {
				return nil, err
			}
			w.param(arg)
		}
		w.write(")")
		rows, err := c.db.conn(ctx).QueryContext(ctx, w.String(), w.args...)
		if err != nil {
			return nil, fmt.Errorf("orm: M2M existing %s: %w", c.through.Table(), err)
		}
		for rows.Next() {
			var raw any
			if err := rows.Scan(&raw); err != nil {
				_ = rows.Close()
				return nil, fmt.Errorf("orm: M2M existing %s: %w", c.through.Table(), err)
			}
			if raw != nil {
				out[normalizePK(pkField, raw)] = true
			}
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("orm: M2M existing %s: %w", c.through.Table(), err)
		}
		_ = rows.Close()
	}
	return out, nil
}

// fetchAllTargetPKs returns all deduplicated non-null target PKs currently
// linked to c.ownerPK in c.through.
func (c *manyCore) fetchAllTargetPKs(ctx context.Context) ([]any, error) {
	d := c.db.dialect
	pkField := c.targetModel.PrimaryKey()
	ownerArg, err := bindField(d, c.sourceFK, c.ownerPK)
	if err != nil {
		return nil, err
	}
	w := newSQLWriter(d)
	w.write("SELECT ")
	w.ident(c.targetFK.Column)
	w.write(" FROM ")
	w.ident(c.through.Table())
	w.write(" WHERE ")
	w.ident(c.sourceFK.Column)
	w.write(" = ")
	w.param(ownerArg)
	if c.targetFK.Null {
		w.write(" AND ")
		w.ident(c.targetFK.Column)
		w.write(" IS NOT NULL")
	}
	rows, err := c.db.conn(ctx).QueryContext(ctx, w.String(), w.args...)
	if err != nil {
		return nil, fmt.Errorf("orm: M2M fetch %s: %w", c.through.Table(), err)
	}
	defer func() { _ = rows.Close() }()

	seen := make(map[any]bool)
	var out []any
	for rows.Next() {
		var raw any
		if err := rows.Scan(&raw); err != nil {
			return nil, fmt.Errorf("orm: M2M fetch %s: %w", c.through.Table(), err)
		}
		if raw == nil {
			continue
		}
		pk := normalizePK(pkField, raw)
		if !seen[pk] {
			seen[pk] = true
			out = append(out, pk)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("orm: M2M fetch %s: %w", c.through.Table(), err)
	}
	return out, nil
}

// removePKs disassociates raw target PKs from c.ownerPK, firing pre_remove and
// post_remove signals with all requested PKs (matching Django's behavior).
func (c *manyCore) removePKs(ctx context.Context, raw []any) error {
	if c.err != nil {
		return c.err
	}
	if len(raw) == 0 {
		return nil
	}
	pks, err := c.normalizeAndDedupePKs(raw)
	if err != nil {
		return err
	}
	err = c.db.Atomic(ctx, func(txCtx context.Context) error {
		if err := fireM2MChanged(txCtx, c.declaringModel, c.field, M2MChangedEvent{
			Sender:   c.through,
			Instance: c.ownerInstance,
			Action:   M2MPreRemove,
			Reverse:  c.reverse,
			Model:    c.targetModel,
			PKSet:    pks,
		}); err != nil {
			return err
		}
		if err := c.execDeletePairs(txCtx, pks); err != nil {
			return err
		}
		return fireM2MChanged(txCtx, c.declaringModel, c.field, M2MChangedEvent{
			Sender:   c.through,
			Instance: c.ownerInstance,
			Action:   M2MPostRemove,
			Reverse:  c.reverse,
			Model:    c.targetModel,
			PKSet:    pks,
		})
	}, WithoutSavepoint())
	if err != nil {
		return err
	}
	c.clearPrefetch()
	return nil
}

// execDeletePairs deletes rows matching (c.ownerPK, pks) and, when symmetrical,
// the mirror (pks, c.ownerPK) rows.
func (c *manyCore) execDeletePairs(ctx context.Context, pks []any) error {
	d := c.db.dialect
	ownerSrcArg, err := bindField(d, c.sourceFK, c.ownerPK)
	if err != nil {
		return err
	}
	var ownerDstArg any
	if c.symmetrical {
		ownerDstArg, err = bindField(d, c.targetFK, c.ownerPK)
		if err != nil {
			return err
		}
	}
	bs := 1000
	if limit := dialectFeatures(d).MaxQueryParams; limit > 2 {
		if c.symmetrical {
			bs = max((limit-2)/2, 1)
		} else {
			bs = limit - 1
		}
	}
	for start := 0; start < len(pks); start += bs {
		chunk := pks[start:min(start+bs, len(pks))]
		w := newSQLWriter(d)
		w.write("DELETE FROM ")
		w.ident(c.through.Table())
		w.write(" WHERE (")
		w.ident(c.sourceFK.Column)
		w.write(" = ")
		w.param(ownerSrcArg)
		w.write(" AND ")
		w.ident(c.targetFK.Column)
		w.write(" IN (")
		for i, pk := range chunk {
			if i > 0 {
				w.write(", ")
			}
			arg, err := bindField(d, c.targetFK, pk)
			if err != nil {
				return err
			}
			w.param(arg)
		}
		w.write("))")
		if c.symmetrical {
			w.write(" OR (")
			w.ident(c.targetFK.Column)
			w.write(" = ")
			w.param(ownerDstArg)
			w.write(" AND ")
			w.ident(c.sourceFK.Column)
			w.write(" IN (")
			for i, pk := range chunk {
				if i > 0 {
					w.write(", ")
				}
				arg, err := bindField(d, c.sourceFK, pk)
				if err != nil {
					return err
				}
				w.param(arg)
			}
			w.write("))")
		}
		if _, err := c.db.conn(ctx).ExecContext(ctx, w.String(), w.args...); err != nil {
			return fmt.Errorf("orm: M2M remove %s: %w", c.through.Table(), err)
		}
	}
	return nil
}

// clear removes all associations for c.ownerPK and fires pre_clear / post_clear
// signals with a nil PKSet.
func (c *manyCore) clear(ctx context.Context) error {
	if c.err != nil {
		return c.err
	}
	err := c.db.Atomic(ctx, func(txCtx context.Context) error {
		if err := fireM2MChanged(txCtx, c.declaringModel, c.field, M2MChangedEvent{
			Sender:   c.through,
			Instance: c.ownerInstance,
			Action:   M2MPreClear,
			Reverse:  c.reverse,
			Model:    c.targetModel,
			PKSet:    nil,
		}); err != nil {
			return err
		}
		d := c.db.dialect
		ownerSrcArg, err := bindField(d, c.sourceFK, c.ownerPK)
		if err != nil {
			return err
		}
		w := newSQLWriter(d)
		w.write("DELETE FROM ")
		w.ident(c.through.Table())
		w.write(" WHERE ")
		w.ident(c.sourceFK.Column)
		w.write(" = ")
		w.param(ownerSrcArg)
		if c.symmetrical {
			ownerDstArg, err := bindField(d, c.targetFK, c.ownerPK)
			if err != nil {
				return err
			}
			w.write(" OR ")
			w.ident(c.targetFK.Column)
			w.write(" = ")
			w.param(ownerDstArg)
		}
		if _, err := c.db.conn(txCtx).ExecContext(txCtx, w.String(), w.args...); err != nil {
			return fmt.Errorf("orm: M2M clear %s: %w", c.through.Table(), err)
		}
		return fireM2MChanged(txCtx, c.declaringModel, c.field, M2MChangedEvent{
			Sender:   c.through,
			Instance: c.ownerInstance,
			Action:   M2MPostClear,
			Reverse:  c.reverse,
			Model:    c.targetModel,
			PKSet:    nil,
		})
	}, WithoutSavepoint())
	if err != nil {
		return err
	}
	c.clearPrefetch()
	return nil
}

// setPKs replaces the related target set for c.ownerPK.
func (c *manyCore) setPKs(ctx context.Context, raw []any, opts ...ManyOption) error {
	if c.err != nil {
		return c.err
	}
	// Validate and normalize all inputs before touching existing links.
	newPKs, err := c.normalizeAndDedupePKs(raw)
	if err != nil {
		return err
	}
	cfg := parseManyConfig(opts)
	err = c.db.Atomic(ctx, func(txCtx context.Context) error {
		if cfg.clear {
			if err := c.clear(txCtx); err != nil {
				return err
			}
			if len(newPKs) > 0 {
				return c.addPKs(txCtx, newPKs, opts...)
			}
			return nil
		}
		oldList, err := c.fetchAllTargetPKs(txCtx)
		if err != nil {
			return err
		}
		oldSet := make(map[any]bool, len(oldList))
		for _, pk := range oldList {
			oldSet[pk] = true
		}
		newSet := make(map[any]bool, len(newPKs))
		var toAdd []any
		for _, pk := range newPKs {
			newSet[pk] = true
			if !oldSet[pk] {
				toAdd = append(toAdd, pk)
			}
		}
		var toRemove []any
		for _, pk := range oldList {
			if !newSet[pk] {
				toRemove = append(toRemove, pk)
			}
		}
		if len(toRemove) > 0 {
			if err := c.removePKs(txCtx, toRemove); err != nil {
				return err
			}
		}
		if len(toAdd) > 0 {
			if err := c.addPKs(txCtx, toAdd, opts...); err != nil {
				return err
			}
		}
		return nil
	}, WithoutSavepoint())
	if err != nil {
		return err
	}
	c.clearPrefetch()
	return nil
}
