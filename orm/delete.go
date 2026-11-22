package orm

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
)

// ErrProtected is matched by *ProtectedError when Delete cannot remove a row
// referenced through an on_delete=protect foreign key, like Django's
// ProtectedError.
var ErrProtected = errors.New("orm: protected foreign key prevents deletion")

// ErrRestricted is matched by *RestrictedError when Delete cannot remove a row
// referenced through an on_delete=restrict foreign key, like Django's
// RestrictedError.
var ErrRestricted = errors.New("orm: restricted foreign key prevents deletion")

// ProtectedError is returned by Delete when rows cannot be deleted because
// they are referenced through on_delete=protect foreign keys. It matches both
// ErrProtected and ErrIntegrity with errors.Is, mirroring Django's
// ProtectedError (a subclass of IntegrityError).
type ProtectedError struct {
	// Message is Django's ProtectedError message.
	Message string
	// ProtectedObjects holds the referencing model struct values blocking
	// deletion.
	ProtectedObjects []any
}

// Error returns Django's ProtectedError message.
func (e *ProtectedError) Error() string { return e.Message }

// Is reports whether target is ErrProtected or ErrIntegrity.
func (e *ProtectedError) Is(target error) bool {
	return target == ErrProtected || target == ErrIntegrity
}

// RestrictedError is returned by Delete when rows cannot be deleted because
// they are referenced through on_delete=restrict foreign keys and are not also
// being deleted via a CASCADE path. It matches both ErrRestricted and
// ErrIntegrity with errors.Is, mirroring Django's RestrictedError.
type RestrictedError struct {
	// Message is Django's RestrictedError message.
	Message string
	// RestrictedObjects holds the referencing model struct values blocking
	// deletion.
	RestrictedObjects []any
}

// Error returns Django's RestrictedError message.
func (e *RestrictedError) Error() string { return e.Message }

// Is reports whether target is ErrRestricted or ErrIntegrity.
func (e *RestrictedError) Is(target error) bool {
	return target == ErrRestricted || target == ErrIntegrity
}

// incomingFK pairs a child model with its FK field that references a target.
type incomingFK struct {
	model *Model
	field *Field
}

// incomingFKs returns every resolved FK in r (including auto-created M2M
// through tables) that points to target, in registration and field-declaration
// order.
func incomingFKs(r *Registry, target *Model) []incomingFK {
	var out []incomingFK
	for _, m := range r.Models() {
		for _, f := range m.fields {
			if f.Rel != nil && f.Rel.Target == target {
				out = append(out, incomingFK{model: m, field: f})
			}
		}
	}
	for _, tm := range r.autoThroughModels {
		for _, f := range tm.fields {
			if f.Rel != nil && f.Rel.Target == target {
				out = append(out, incomingFK{model: tm, field: f})
			}
		}
	}
	return out
}

// hasModelDeleteHandlers reports whether any PreDelete or PostDelete handler
// is registered for m.
func hasModelDeleteHandlers(m *Model) bool {
	if m.goType == nil {
		return false
	}
	return signals.has(m.goType, preDelete) || signals.has(m.goType, postDelete)
}

// canFastDelete reports whether Delete on m can execute as a single SQL DELETE
// statement without running the collector.
func canFastDelete(r *Registry, m *Model) bool {
	if hasModelDeleteHandlers(m) || len(m.genericRels) > 0 {
		return false
	}
	for _, inc := range incomingFKs(r, m) {
		if inc.field.Rel.OnDelete != OnDeleteDoNothing {
			return false
		}
	}
	return true
}

// setNullUpdate records a pending ON DELETE SET NULL update on a child table.
type setNullUpdate struct {
	model     *Model
	column    string
	parentPKs []any
}

// restrictedEntry records referencing rows discovered along an ON DELETE
// RESTRICT relation, checked once full cascade collection finishes.
type restrictedEntry struct {
	key   string
	child *Model
	pks   []any
}

// deleteCollector mirrors Django's Collector: it walks incoming foreign keys
// to enforce PROTECT and RESTRICT, collect CASCADE dependents, schedule
// SET_NULL updates, and delete rows in dependency order (children before
// parents) while firing PreDelete and PostDelete signals.
type deleteCollector struct {
	db             *DB
	collected      map[*Model][]any
	collectedSet   map[*Model]map[any]bool
	collectedOrder []*Model
	// deps[parent][child] = true means child must be deleted before parent.
	deps       map[*Model]map[*Model]bool
	setNulls   []setNullUpdate
	restricted []restrictedEntry
}

// newDeleteCollector returns an empty collector for db.
func newDeleteCollector(db *DB) *deleteCollector {
	return &deleteCollector{
		db:           db,
		collected:    make(map[*Model][]any),
		collectedSet: make(map[*Model]map[any]bool),
		deps:         make(map[*Model]map[*Model]bool),
	}
}

// deleteWithCollector runs qs.Delete using the collector when incoming
// relations or delete signals require it, and falls back to a single DELETE
// statement on the fast path.
func (qs *QuerySet[T]) deleteWithCollector(ctx context.Context) (int64, error) {
	if qs.q.err != nil {
		return 0, qs.q.err
	}
	if qs.q.sliced() {
		return 0, errDeleteSliced
	}
	db, m := qs.q.db, qs.q.model
	if canFastDelete(db.registry, m) {
		return qs.deleteRows(ctx)
	}

	var total int64
	run := func(txCtx context.Context) error {
		pks, err := qs.q.selectPKs(txCtx)
		if err != nil {
			return err
		}
		if len(pks) == 0 {
			return nil
		}
		col := newDeleteCollector(db)
		if err := col.collect(txCtx, m, pks, true); err != nil {
			return err
		}
		n, err := col.execute(txCtx)
		if err != nil {
			return err
		}
		total = n
		return nil
	}

	// Mirror Django's transaction.atomic(savepoint=False): start a transaction
	// when outside one, otherwise reuse the active transaction without adding
	// savepoint queries.
	if _, inTx := txFromContext(ctx); inTx {
		if err := run(ctx); err != nil {
			return 0, err
		}
		return total, nil
	}
	if err := db.Atomic(ctx, run); err != nil {
		return 0, err
	}
	return total, nil
}

// selectPKs fetches the primary keys of every row matching q.
func (q *query) selectPKs(ctx context.Context) ([]any, error) {
	c, err := q.compile()
	if err != nil {
		return nil, err
	}
	if c.empty {
		return nil, nil
	}
	pk := q.model.PrimaryKey()
	if pk == nil {
		return nil, fmt.Errorf("orm: model %s has no primary key", q.model.Name())
	}
	stmt, err := c.build([]selectItem{{node: c.baseColumn(pk)}}, false)
	if err != nil {
		return nil, err
	}
	w := newSQLWriter(c.d)
	stmt.render(w)
	rows, err := q.db.conn(ctx).QueryContext(ctx, w.String(), w.args...)
	if err != nil {
		return nil, fmt.Errorf("orm: delete %s: %w", q.model.Name(), err)
	}
	defer func() { _ = rows.Close() }()
	var pks []any
	for rows.Next() {
		var raw any
		if err := rows.Scan(&raw); err != nil {
			return nil, fmt.Errorf("orm: delete %s: %w", q.model.Name(), err)
		}
		pks = append(pks, normalizePK(pk, raw))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("orm: delete %s: %w", q.model.Name(), err)
	}
	return pks, nil
}

// normalizePK converts a scanned primary key value into a comparable Go key so
// PKs read from different queries match reliably in maps.
func normalizePK(pk *Field, v any) any {
	switch pk.Kind {
	case KindAuto, KindInt:
		if n, err := toInt64(v); err == nil {
			return n
		}
	case KindChar, KindText:
		if b, ok := v.([]byte); ok {
			return string(b)
		}
	}
	return v
}

// addDep records that child must be deleted before parent.
func (c *deleteCollector) addDep(parent, child *Model) {
	if parent == child {
		return
	}
	if c.deps[parent] == nil {
		c.deps[parent] = make(map[*Model]bool)
	}
	c.deps[parent][child] = true
}

// collect adds pks of model to the deletion set and recursively inspects
// incoming foreign keys.
func (c *deleteCollector) collect(ctx context.Context, model *Model, pks []any, failOnRestricted bool) error {
	set := c.collectedSet[model]
	if set == nil {
		set = make(map[any]bool, len(pks))
		c.collectedSet[model] = set
		c.collectedOrder = append(c.collectedOrder, model)
	}
	newPKs := make([]any, 0, len(pks))
	for _, pk := range pks {
		if !set[pk] {
			set[pk] = true
			c.collected[model] = append(c.collected[model], pk)
			newPKs = append(newPKs, pk)
		}
	}
	if len(newPKs) == 0 {
		return nil
	}

	var protectedKeys []string
	protectedByKey := make(map[string][]any)
	addProtected := func(key string, objs []any) {
		if _, seen := protectedByKey[key]; !seen {
			protectedKeys = append(protectedKeys, key)
		}
		protectedByKey[key] = append(protectedByKey[key], objs...)
	}

	for _, inc := range incomingFKs(c.db.registry, model) {
		od := inc.field.Rel.OnDelete
		switch od {
		case OnDeleteDoNothing:
			continue
		case OnDeleteProtect:
			objs, err := c.fetchObjectsByColumn(ctx, inc.model, inc.field.Column, newPKs)
			if err != nil {
				return err
			}
			if len(objs) > 0 {
				key := fmt.Sprintf("'%s.%s'", inc.model.Name(), inc.field.Attr)
				addProtected(key, objs)
			}
		case OnDeleteRestrict:
			childPKs, err := c.fetchPKsByColumn(ctx, inc.model, inc.field.Column, newPKs)
			if err != nil {
				return err
			}
			if len(childPKs) > 0 {
				c.addDep(model, inc.model)
				key := fmt.Sprintf("'%s.%s'", inc.model.Name(), inc.field.Attr)
				c.restricted = append(c.restricted, restrictedEntry{
					key:   key,
					child: inc.model,
					pks:   childPKs,
				})
			}
		case OnDeleteSetNull:
			c.setNulls = append(c.setNulls, setNullUpdate{
				model:     inc.model,
				column:    inc.field.Column,
				parentPKs: newPKs,
			})
		case OnDeleteCascade:
			childPKs, err := c.fetchPKsByColumn(ctx, inc.model, inc.field.Column, newPKs)
			if err != nil {
				return err
			}
			if len(childPKs) == 0 {
				continue
			}
			c.addDep(model, inc.model)
			if err := c.collect(ctx, inc.model, childPKs, false); err != nil {
				var pe *ProtectedError
				if errors.As(err, &pe) {
					key := fmt.Sprintf("'%s.%s'", inc.model.Name(), inc.field.Attr)
					addProtected(key, pe.ProtectedObjects)
					continue
				}
				return err
			}
		}
	}

	if len(protectedKeys) > 0 {
		var objs []any
		for _, k := range protectedKeys {
			objs = append(objs, protectedByKey[k]...)
		}
		return &ProtectedError{
			Message: fmt.Sprintf(
				"Cannot delete some instances of model '%s' because they are referenced through protected foreign keys: %s.",
				model.Name(), strings.Join(protectedKeys, ", "),
			),
			ProtectedObjects: objs,
		}
	}

	// The rows of a generic relation are deleted with the rows they belong
	// to, like Django's bulk_related_objects. They have no foreign key to the
	// model, so the order of the deletes does not matter.
	for _, gr := range model.genericRels {
		childPKs, err := newQuery(c.db, gr.Related).genericFilter(gr, newPKs).pks(ctx)
		if err != nil {
			return err
		}
		if len(childPKs) == 0 {
			continue
		}
		if err := c.collect(ctx, gr.Related, childPKs, false); err != nil {
			return err
		}
	}

	if failOnRestricted && len(c.restricted) > 0 {
		var restrictedKeys []string
		restrictedByKey := make(map[string][]any)
		for _, entry := range c.restricted {
			deleted := c.collectedSet[entry.child]
			var remaining []any
			for _, pk := range entry.pks {
				if !deleted[pk] {
					remaining = append(remaining, pk)
				}
			}
			if len(remaining) == 0 {
				continue
			}
			objs, err := c.fetchObjectsByColumn(ctx, entry.child, entry.child.PrimaryKey().Column, remaining)
			if err != nil {
				return err
			}
			if _, seen := restrictedByKey[entry.key]; !seen {
				restrictedKeys = append(restrictedKeys, entry.key)
			}
			restrictedByKey[entry.key] = append(restrictedByKey[entry.key], objs...)
		}
		if len(restrictedKeys) > 0 {
			var objs []any
			for _, k := range restrictedKeys {
				objs = append(objs, restrictedByKey[k]...)
			}
			return &RestrictedError{
				Message: fmt.Sprintf(
					"Cannot delete some instances of model '%s' because they are referenced through restricted foreign keys: %s.",
					model.Name(), strings.Join(restrictedKeys, ", "),
				),
				RestrictedObjects: objs,
			}
		}
	}

	return nil
}

// sortModels orders collected models so every dependent child appears before
// its parent.
func (c *deleteCollector) sortModels() []*Model {
	var sorted []*Model
	visited := make(map[*Model]bool, len(c.collectedOrder))
	inStack := make(map[*Model]bool, len(c.collectedOrder))

	var visit func(m *Model)
	visit = func(m *Model) {
		if visited[m] || inStack[m] {
			return
		}
		inStack[m] = true
		for _, cand := range c.collectedOrder {
			if c.deps[m][cand] {
				visit(cand)
			}
		}
		delete(inStack, m)
		visited[m] = true
		sorted = append(sorted, m)
	}
	for _, m := range c.collectedOrder {
		visit(m)
	}
	return sorted
}

// execute fires PreDelete signals, applies SET_NULL updates, deletes each
// collected model in dependency order, and fires PostDelete signals.
func (c *deleteCollector) execute(ctx context.Context) (int64, error) {
	sorted := c.sortModels()

	// Load full model instances for models that have PreDelete or PostDelete
	// handlers before any row is modified or removed.
	loaded := make(map[*Model][]reflect.Value)
	for _, m := range sorted {
		if !hasModelDeleteHandlers(m) {
			continue
		}
		vals, err := c.fetchReflectByPK(ctx, m, c.collected[m])
		if err != nil {
			return 0, err
		}
		loaded[m] = vals
	}

	for _, m := range sorted {
		if !signals.has(m.goType, preDelete) {
			continue
		}
		if err := fireModelSignal(ctx, m.goType, preDelete, loaded[m]); err != nil {
			return 0, err
		}
	}

	for _, upd := range c.setNulls {
		if err := c.execSetNull(ctx, upd); err != nil {
			return 0, err
		}
	}

	var total int64
	for _, m := range sorted {
		pks := c.collected[m]
		if len(pks) == 0 {
			continue
		}
		// Null out nullable self-referential RESTRICT FKs on rows being deleted
		// so row-by-row FK checks on SQLite do not reject deleting a parent row
		// before its child in the same table.
		for _, f := range m.fields {
			if f.Rel != nil && f.Rel.Target == m && f.Null && f.Rel.OnDelete == OnDeleteRestrict {
				if err := c.execSetNullByPK(ctx, m, f.Column, pks); err != nil {
					return 0, err
				}
			}
		}
		n, err := c.execDeleteByPK(ctx, m, pks)
		if err != nil {
			return 0, err
		}
		total += n
		if signals.has(m.goType, postDelete) {
			if err := fireModelSignal(ctx, m.goType, postDelete, loaded[m]); err != nil {
				return 0, err
			}
		}
	}

	return total, nil
}

// fireModelSignal invokes all signal handlers registered for t and kind on
// each addressable struct value in vals.
func fireModelSignal(ctx context.Context, t reflect.Type, kind signalKind, vals []reflect.Value) error {
	entries := signals.snapshot(t, kind)
	if len(entries) == 0 {
		return nil
	}
	for i := range vals {
		ptr := vals[i].Addr().Interface()
		for _, e := range entries {
			if err := e.fn(ctx, ptr); err != nil {
				return err
			}
		}
	}
	return nil
}

// batchSize returns the maximum number of IN parameters per statement for db.
func (c *deleteCollector) batchSize() int {
	if limit := dialectFeatures(c.db.dialect).MaxQueryParams; limit > 0 {
		return limit
	}
	return 1000
}

// nestDeleted returns the DeleteNode of key with its children from edges,
// Django's NestedObjects._nested(): a key already in seen yields nothing, so
// each row appears once. A row that was not loaded, the row of an
// auto-created through model, yields nothing either.
func nestDeleted(key deleteKey, edges map[deleteKey][]deleteKey, loaded map[deleteKey]any, seen map[deleteKey]bool) []DeleteNode {
	if seen[key] {
		return nil
	}
	seen[key] = true
	obj, ok := loaded[key]
	if !ok {
		return nil
	}
	var children []DeleteNode
	for _, child := range edges[key] {
		children = append(children, nestDeleted(child, edges, loaded, seen)...)
	}
	return []DeleteNode{{Model: key.model, Object: obj, Children: children}}
}

// fetchPKsByColumn returns the primary keys of the rows of m where col IN values.
func (c *deleteCollector) fetchPKsByColumn(ctx context.Context, m *Model, col string, values []any) ([]any, error) {
	pk := m.PrimaryKey()
	if pk == nil {
		return nil, fmt.Errorf("orm: model %s has no primary key", m.Name())
	}
	rows, err := c.fetchColumnsByColumn(ctx, m, []string{pk.Column}, col, values)
	if err != nil {
		return nil, err
	}
	out := make([]any, len(rows))
	for i, row := range rows {
		out[i] = normalizePK(pk, row[0])
	}
	return out, nil
}

// fetchColumnsByColumn returns the raw values of the columns cols of the rows
// of m where col IN values, one slice per row. It queries in batches of the
// dialect's parameter limit.
func (c *deleteCollector) fetchColumnsByColumn(ctx context.Context, m *Model, cols []string, col string, values []any) ([][]any, error) {
	d := c.db.dialect
	bs := c.batchSize()
	var out [][]any
	for start := 0; start < len(values); start += bs {
		chunk := values[start:min(start+bs, len(values))]
		w := newSQLWriter(d)
		w.write("SELECT ")
		for i, sc := range cols {
			if i > 0 {
				w.write(", ")
			}
			w.ident(sc)
		}
		w.write(" FROM ")
		w.ident(m.Table())
		w.write(" WHERE ")
		w.ident(col)
		w.write(" IN (")
		for i, v := range chunk {
			if i > 0 {
				w.write(", ")
			}
			w.param(v)
		}
		w.write(")")
		rows, err := c.db.conn(ctx).QueryContext(ctx, w.String(), w.args...)
		if err != nil {
			return nil, fmt.Errorf("orm: delete collect %s: %w", m.Name(), err)
		}
		for rows.Next() {
			raw := make([]any, len(cols))
			dest := make([]any, len(cols))
			for i := range raw {
				dest[i] = &raw[i]
			}
			if err := rows.Scan(dest...); err != nil {
				_ = rows.Close()
				return nil, fmt.Errorf("orm: delete collect %s: %w", m.Name(), err)
			}
			out = append(out, raw)
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("orm: delete collect %s: %w", m.Name(), err)
		}
		_ = rows.Close()
	}
	return out, nil
}

// fetchReflectByColumn loads addressable struct values of m where col IN values.
func (c *deleteCollector) fetchReflectByColumn(ctx context.Context, m *Model, col string, values []any) ([]reflect.Value, error) {
	d := c.db.dialect
	bs := c.batchSize()
	plan := &rowPlan{model: m, ncols: len(m.fields)}
	var out []reflect.Value
	for start := 0; start < len(values); start += bs {
		chunk := values[start:min(start+bs, len(values))]
		w := newSQLWriter(d)
		w.write("SELECT ")
		for i, f := range m.fields {
			if i > 0 {
				w.write(", ")
			}
			w.ident(f.Column)
		}
		w.write(" FROM ")
		w.ident(m.Table())
		w.write(" WHERE ")
		w.ident(col)
		w.write(" IN (")
		for i, v := range chunk {
			if i > 0 {
				w.write(", ")
			}
			w.param(v)
		}
		w.write(")")
		rows, err := c.db.conn(ctx).QueryContext(ctx, w.String(), w.args...)
		if err != nil {
			return nil, fmt.Errorf("orm: delete load %s: %w", m.Name(), err)
		}
		for rows.Next() {
			elem, _, err := plan.scan(rows)
			if err != nil {
				_ = rows.Close()
				return nil, fmt.Errorf("orm: delete load %s: %w", m.Name(), err)
			}
			out = append(out, elem)
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("orm: delete load %s: %w", m.Name(), err)
		}
		_ = rows.Close()
	}
	return out, nil
}

// fetchReflectByPK loads addressable struct values of m for the given PKs.
func (c *deleteCollector) fetchReflectByPK(ctx context.Context, m *Model, pks []any) ([]reflect.Value, error) {
	return c.fetchReflectByColumn(ctx, m, m.PrimaryKey().Column, pks)
}

// fetchObjectsByColumn loads struct values of m as []any where col IN values.
func (c *deleteCollector) fetchObjectsByColumn(ctx context.Context, m *Model, col string, values []any) ([]any, error) {
	vals, err := c.fetchReflectByColumn(ctx, m, col, values)
	if err != nil {
		return nil, err
	}
	out := make([]any, len(vals))
	for i, v := range vals {
		out[i] = v.Interface()
	}
	return out, nil
}

// execSetNull sets upd.column = NULL on upd.model where upd.column IN upd.parentPKs.
func (c *deleteCollector) execSetNull(ctx context.Context, upd setNullUpdate) error {
	d := c.db.dialect
	bs := c.batchSize()
	for start := 0; start < len(upd.parentPKs); start += bs {
		chunk := upd.parentPKs[start:min(start+bs, len(upd.parentPKs))]
		w := newSQLWriter(d)
		w.write("UPDATE ")
		w.ident(upd.model.Table())
		w.write(" SET ")
		w.ident(upd.column)
		w.write(" = NULL WHERE ")
		w.ident(upd.column)
		w.write(" IN (")
		for i, v := range chunk {
			if i > 0 {
				w.write(", ")
			}
			w.param(v)
		}
		w.write(")")
		if _, err := c.db.conn(ctx).ExecContext(ctx, w.String(), w.args...); err != nil {
			return fmt.Errorf("orm: delete set_null %s.%s: %w", upd.model.Name(), upd.column, err)
		}
	}
	return nil
}

// execSetNullByPK sets col = NULL on m for rows with pk IN pks.
func (c *deleteCollector) execSetNullByPK(ctx context.Context, m *Model, col string, pks []any) error {
	d := c.db.dialect
	bs := c.batchSize()
	pkCol := m.PrimaryKey().Column
	for start := 0; start < len(pks); start += bs {
		chunk := pks[start:min(start+bs, len(pks))]
		w := newSQLWriter(d)
		w.write("UPDATE ")
		w.ident(m.Table())
		w.write(" SET ")
		w.ident(col)
		w.write(" = NULL WHERE ")
		w.ident(pkCol)
		w.write(" IN (")
		for i, v := range chunk {
			if i > 0 {
				w.write(", ")
			}
			w.param(v)
		}
		w.write(")")
		if _, err := c.db.conn(ctx).ExecContext(ctx, w.String(), w.args...); err != nil {
			return fmt.Errorf("orm: delete clear %s.%s: %w", m.Name(), col, err)
		}
	}
	return nil
}

// execDeleteByPK deletes rows of m with pk IN pks in dialect-sized batches.
func (c *deleteCollector) execDeleteByPK(ctx context.Context, m *Model, pks []any) (int64, error) {
	d := c.db.dialect
	bs := c.batchSize()
	pkCol := m.PrimaryKey().Column
	var total int64
	for start := 0; start < len(pks); start += bs {
		chunk := pks[start:min(start+bs, len(pks))]
		w := newSQLWriter(d)
		w.write("DELETE FROM ")
		w.ident(m.Table())
		w.write(" WHERE ")
		w.ident(pkCol)
		w.write(" IN (")
		for i, v := range chunk {
			if i > 0 {
				w.write(", ")
			}
			w.param(v)
		}
		w.write(")")
		res, err := c.db.conn(ctx).ExecContext(ctx, w.String(), w.args...)
		if err != nil {
			return 0, fmt.Errorf("orm: delete %s: %w", m.Name(), err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return 0, fmt.Errorf("orm: delete %s: rows affected: %w", m.Name(), err)
		}
		total += n
	}
	return total, nil
}

// DeleteModelGroup holds one model and the instances (as *T pointers) that
// would be deleted as part of a DeletePlan.
type DeleteModelGroup struct {
	Model   *Model
	Objects []any
}

// DeletePlan is the read-only preview of a cascading deletion produced by
// CollectDelete. Groups lists each affected model and its objects in
// collection order; Protected lists any referencing objects that block
// deletion via ON DELETE PROTECT or ON DELETE RESTRICT. Tree holds the same
// objects as Groups, nested under the objects whose deletion cascades to them.
type DeletePlan struct {
	Groups    []DeleteModelGroup
	Protected []DeleteModelGroup
	Tree      []DeleteNode
}

// DeleteNode is one object of a DeletePlan's Tree with the objects that its
// deletion cascades to, Django's NestedObjects.nested(). An object that more
// than one deleted object reaches is listed once, under the first of them.
type DeleteNode struct {
	// Model is the model of Object.
	Model *Model
	// Object is the instance as a *T pointer.
	Object any
	// Children lists the objects deleted because of Object.
	Children []DeleteNode
}

// deleteKey identifies one collected row of a DeletePlan by its model and
// the treeKey of its primary key.
type deleteKey struct {
	model *Model
	pk    any
}

// treeKey returns primary key v of the field pk in one comparable form,
// whether v comes from a database row or from a loaded struct.
func treeKey(pk *Field, v any) any {
	v = normalizePK(pk, v)
	switch x := v.(type) {
	case []byte:
		return string(x)
	case fmt.Stringer:
		return x.String()
	}
	return v
}

// CollectDelete inspects the database to compute which objects would be
// cascade-deleted and which objects would block deletion if rows of m with
// primary keys pks were deleted. It does not modify the database.
func CollectDelete(ctx context.Context, db *DB, m *Model, pks []any) (*DeletePlan, error) {
	plan := &DeletePlan{}
	if m == nil || m.PrimaryKey() == nil || len(pks) == 0 {
		return plan, nil
	}
	pkField := m.PrimaryKey()
	normPKs := make([]any, 0, len(pks))
	for _, pk := range pks {
		normPKs = append(normPKs, normalizePK(pkField, pk))
	}

	col := newDeleteCollector(db)
	protectedOrder := make([]*Model, 0)
	protectedSet := make(map[*Model]map[any]bool)
	protectedPKs := make(map[*Model][]any)
	addProtected := func(pm *Model, ppks []any) {
		if len(ppks) == 0 {
			return
		}
		set := protectedSet[pm]
		if set == nil {
			set = make(map[any]bool, len(ppks))
			protectedSet[pm] = set
			protectedOrder = append(protectedOrder, pm)
		}
		for _, pk := range ppks {
			if !set[pk] {
				set[pk] = true
				protectedPKs[pm] = append(protectedPKs[pm], pk)
			}
		}
	}

	// edges maps each collected row to the rows that its deletion cascades to,
	// in collection order, as Django's NestedObjects.add_edge records them.
	edges := make(map[deleteKey][]deleteKey)

	var walk func(model *Model, ids []any) error
	walk = func(model *Model, ids []any) error {
		set := col.collectedSet[model]
		if set == nil {
			set = make(map[any]bool, len(ids))
			col.collectedSet[model] = set
			col.collectedOrder = append(col.collectedOrder, model)
		}
		newPKs := make([]any, 0, len(ids))
		for _, pk := range ids {
			if !set[pk] {
				set[pk] = true
				col.collected[model] = append(col.collected[model], pk)
				newPKs = append(newPKs, pk)
			}
		}
		if len(newPKs) == 0 {
			return nil
		}
		for _, inc := range incomingFKs(db.registry, model) {
			switch inc.field.Rel.OnDelete {
			case OnDeleteDoNothing, OnDeleteSetNull:
				continue
			case OnDeleteProtect:
				childPKs, err := col.fetchPKsByColumn(ctx, inc.model, inc.field.Column, newPKs)
				if err != nil {
					return err
				}
				addProtected(inc.model, childPKs)
			case OnDeleteRestrict:
				childPKs, err := col.fetchPKsByColumn(ctx, inc.model, inc.field.Column, newPKs)
				if err != nil {
					return err
				}
				if len(childPKs) > 0 {
					col.restricted = append(col.restricted, restrictedEntry{
						child: inc.model,
						pks:   childPKs,
					})
				}
			case OnDeleteCascade:
				childPK := inc.model.PrimaryKey()
				if childPK == nil {
					return fmt.Errorf("orm: model %s has no primary key", inc.model.Name())
				}
				rows, err := col.fetchColumnsByColumn(ctx, inc.model,
					[]string{childPK.Column, inc.field.Column}, inc.field.Column, newPKs)
				if err != nil {
					return err
				}
				if len(rows) == 0 {
					continue
				}
				childPKs := make([]any, len(rows))
				for i, row := range rows {
					childPKs[i] = normalizePK(childPK, row[0])
					parent := deleteKey{model, treeKey(model.PrimaryKey(), row[1])}
					edges[parent] = append(edges[parent], deleteKey{inc.model, treeKey(childPK, row[0])})
				}
				if err := walk(inc.model, childPKs); err != nil {
					return err
				}
			}
		}
		return nil
	}

	if err := walk(m, normPKs); err != nil {
		return nil, err
	}

	for _, entry := range col.restricted {
		deleted := col.collectedSet[entry.child]
		var remaining []any
		for _, pk := range entry.pks {
			if !deleted[pk] {
				remaining = append(remaining, pk)
			}
		}
		addProtected(entry.child, remaining)
	}

	loaded := make(map[deleteKey]any)
	for _, gm := range col.collectedOrder {
		if gm.AutoCreated() || gm.goType == nil {
			continue
		}
		vals, err := col.fetchReflectByPK(ctx, gm, col.collected[gm])
		if err != nil {
			return nil, err
		}
		if len(vals) == 0 {
			continue
		}
		gpk := gm.PrimaryKey()
		objs := make([]any, len(vals))
		for i := range vals {
			objs[i] = vals[i].Addr().Interface()
			loaded[deleteKey{gm, treeKey(gpk, vals[i].Field(gpk.Index).Interface())}] = objs[i]
		}
		plan.Groups = append(plan.Groups, DeleteModelGroup{
			Model:   gm,
			Objects: objs,
		})
	}
	seen := make(map[deleteKey]bool)
	for _, pk := range normPKs {
		plan.Tree = append(plan.Tree, nestDeleted(deleteKey{m, treeKey(pkField, pk)}, edges, loaded, seen)...)
	}

	for _, pm := range protectedOrder {
		if pm.AutoCreated() || pm.goType == nil {
			continue
		}
		vals, err := col.fetchReflectByPK(ctx, pm, protectedPKs[pm])
		if err != nil {
			return nil, err
		}
		if len(vals) == 0 {
			continue
		}
		objs := make([]any, len(vals))
		for i := range vals {
			objs[i] = vals[i].Addr().Interface()
		}
		plan.Protected = append(plan.Protected, DeleteModelGroup{
			Model:   pm,
			Objects: objs,
		})
	}

	return plan, nil
}
