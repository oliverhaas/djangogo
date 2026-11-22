package orm

import (
	"context"
	"errors"
	"fmt"
	"reflect"
)

// bulkConfig collects the options of BulkCreate.
type bulkConfig struct {
	// batchSize is the requested number of rows per statement, zero when
	// BatchSize was not given.
	batchSize       int
	batchSizeErr    error
	ignoreConflicts bool
}

// errBatchSize is returned for a batch size below one. It is Django's "Batch
// size must be a positive integer."
var errBatchSize = errors.New("orm: batch size must be a positive integer")

// BulkOption is an optional argument of BulkCreate.
type BulkOption func(*bulkConfig)

// BatchSize caps the number of rows BulkCreate writes per INSERT statement,
// like Django's batch_size. A size below one is an error.
func BatchSize(n int) BulkOption {
	return func(c *bulkConfig) {
		c.batchSize = n
		if n < 1 {
			c.batchSizeErr = errBatchSize
		}
	}
}

// IgnoreConflicts makes BulkCreate skip the rows that violate a constraint
// (ON CONFLICT DO NOTHING), like Django's ignore_conflicts=True. The primary
// keys of the objects are then not written back.
func IgnoreConflicts() BulkOption {
	return func(c *bulkConfig) { c.ignoreConflicts = true }
}

// BulkCreate inserts the objects with one multi-row INSERT, like Django's
// bulk_create(). Defaults are applied and auto_now and auto_now_add fields
// are stamped, but no save signal is fired. The generated primary keys are
// written back into the objects on a dialect that can return them (SQLite and
// PostgreSQL both can).
//
// The objects are split over several statements when the dialect caps the
// parameters of one statement, when BatchSize is given, or when only some of
// them carry a primary key. A model with no column but its generated key is
// written one row per statement. Several statements run in one transaction.
func (qs *QuerySet[T]) BulkCreate(ctx context.Context, objs []*T, opts ...BulkOption) error {
	if qs.q.err != nil {
		return qs.q.err
	}
	cfg := &bulkConfig{}
	for _, opt := range opts {
		opt(cfg)
	}
	if cfg.batchSizeErr != nil {
		return cfg.batchSizeErr
	}
	if len(objs) == 0 {
		return nil
	}
	db, m := qs.q.db, qs.q.model
	pk := m.PrimaryKey()
	auto := pk != nil && pk.Kind == KindAuto
	now := db.now()

	// A statement either names the primary key column or leaves it out, so the
	// objects that carry a key and those that do not are inserted separately.
	var withPK, withoutPK []reflect.Value
	for i, obj := range objs {
		if obj == nil {
			return fmt.Errorf("orm: bulk create %s: object %d is nil", m.Name(), i)
		}
		v := reflect.ValueOf(obj).Elem()
		if err := syncLoadedRelations(db.Registry(), m, v, "bulk_create"); err != nil {
			return err
		}
		if err := checkInsertDeferred(m, v); err != nil {
			return err
		}
		applyDefaults(v, m.fields)
		applyAutoNowOnCreate(v, m.fields, now)
		if auto && v.Field(pk.Index).IsZero() {
			withoutPK = append(withoutPK, v)
		} else {
			withPK = append(withPK, v)
		}
	}

	var batches [][]reflect.Value
	for i, group := range [][]reflect.Value{withPK, withoutPK} {
		// The second group leaves the key column to the database.
		columns := len(m.fields) - i
		// A row without columns is written as DEFAULT VALUES, one per statement.
		size := 1
		if columns > 0 {
			size = batchRows(db.dialect, columns, cfg.batchSize, len(group))
		}
		for start := 0; start < len(group); start += size {
			batches = append(batches, group[start:min(start+size, len(group))])
		}
	}
	run := func(ctx context.Context) error {
		for _, batch := range batches {
			if err := db.insert(ctx, m, batch, cfg.ignoreConflicts); err != nil {
				return err
			}
		}
		return nil
	}
	// One statement is atomic by itself. Inside a transaction a failure marks
	// it for rollback, as with Django's atomic(savepoint=False).
	if txStateFrom(ctx) == nil && len(batches) == 1 {
		return run(ctx)
	}
	return db.Atomic(ctx, run, WithoutSavepoint())
}

// batchRows returns the number of rows per INSERT for rows of ncols columns:
// the requested batch size, lowered to what the dialect's parameter cap
// allows, or all n rows when nothing limits the statement.
func batchRows(d Dialect, ncols, requested, n int) int {
	size := n
	if maxParams := dialectFeatures(d).MaxQueryParams; maxParams > 0 && ncols > 0 {
		size = min(size, max(maxParams/ncols, 1))
	}
	if requested > 0 {
		size = min(size, requested)
	}
	return max(size, 1)
}

// InBulk returns the rows of qs whose primary key is in pks, as a map from
// primary key to row, like Django's in_bulk(). It runs no query when pks is
// empty, and otherwise one query per chunk of keys that fits the dialect's cap
// on the parameters of a statement. A queryset with a limit or offset is an
// error.
//
//	byID, err := orm.InBulk(ctx, orm.Query[Article](db), []int64{1, 2, 3})
func InBulk[K comparable, T any](ctx context.Context, qs *QuerySet[T], pks []K) (map[K]T, error) {
	out := make(map[K]T, len(pks))
	if qs.q.err != nil {
		return nil, qs.q.err
	}
	if qs.q.sliced() {
		return nil, errors.New("orm: cannot use 'limit' or 'offset' with in_bulk()")
	}
	if len(pks) == 0 {
		return out, nil
	}
	pk := qs.q.model.PrimaryKey()
	if pk == nil {
		return nil, fmt.Errorf("orm: model %s has no primary key", qs.q.model.Name())
	}
	size := len(pks)
	if maxParams := dialectFeatures(qs.q.db.dialect).MaxQueryParams; maxParams > 0 {
		size = min(size, maxParams)
	}
	for start := 0; start < len(pks); start += size {
		rows, err := qs.Filter("pk__in", pks[start:min(start+size, len(pks))]).All(ctx)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			key, err := convertTo[K](reflect.ValueOf(row).Field(pk.Index).Interface())
			if err != nil {
				return nil, fmt.Errorf("orm: InBulk %s: %w", qs.q.model.Name(), err)
			}
			out[key] = row
		}
	}
	return out, nil
}
