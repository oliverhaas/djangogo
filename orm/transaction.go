package orm

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"runtime"
	"slices"
	"strconv"
	"sync"
)

// querier is the subset of *sql.DB and *sql.Tx that the executor needs. Routing
// every database call through a querier lets a terminal run either directly on
// the pool or inside the transaction bound to its context.
type querier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// scanOne runs a query that returns one row on q and scans the row into dest.
func scanOne(ctx context.Context, q querier, query string, args []any, dest ...any) error {
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return err
		}
		return sql.ErrNoRows
	}
	if err := rows.Scan(dest...); err != nil {
		return err
	}
	return rows.Close()
}

// txKey is the context key under which the state of the active transaction is
// stored.
type txKey struct{}

// txState is the state of one transaction, shared by the atomic blocks of one
// outermost Atomic call. It holds what Django keeps on the database connection
// during an atomic block.
type txState struct {
	tx *sql.Tx

	mu sync.Mutex
	// needsRollback is Django's needs_rollback: the innermost enclosing block
	// with a savepoint rolls back when it ends, and queries fail until then.
	needsRollback bool
	// rollbackErr is the error that set needsRollback, if any.
	rollbackErr error
	// savepoints is the stack of the open nested atomic blocks, innermost
	// last. A block without a savepoint has "".
	savepoints []string
	// seq numbers the savepoints of the transaction.
	seq int
	// hooks are the OnCommit callbacks, in the order of registration.
	hooks []commitHook
}

// commitHook is a callback registered with OnCommit inside a transaction.
type commitHook struct {
	// savepoints are the savepoints of the atomic blocks that were open when
	// the callback was registered. A rollback to one of them drops it.
	savepoints []string
	fn         func(ctx context.Context) error
	robust     bool
}

// txStateFrom returns the state of the transaction bound to ctx, or nil.
func txStateFrom(ctx context.Context) *txState {
	st, _ := ctx.Value(txKey{}).(*txState)
	return st
}

// brokenErr returns the error of a query in a transaction marked for rollback,
// Django's validate_no_broken_transaction, or nil.
func (st *txState) brokenErr() error {
	st.mu.Lock()
	defer st.mu.Unlock()
	if !st.needsRollback {
		return nil
	}
	return &transactionError{
		msg: "orm: an error occurred in the current transaction. You can't execute queries " +
			"until the end of the 'atomic' block.",
		cause: st.rollbackErr,
	}
}

// markForRollback sets the rollback flag of the transaction. A non-nil err
// becomes its cause.
func (st *txState) markForRollback(err error) {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.needsRollback = true
	if err != nil {
		st.rollbackErr = err
	}
}

// rollbackFlag returns the rollback flag.
func (st *txState) rollbackFlag() bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.needsRollback
}

// setRollbackFlag sets the rollback flag. Clearing it also drops its cause.
func (st *txState) setRollbackFlag(rollback bool) {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.needsRollback = rollback
	if !rollback {
		st.rollbackErr = nil
	}
}

// nextSavepoint returns a savepoint name not used before in the transaction.
func (st *txState) nextSavepoint() string {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.seq++
	return "sp_" + strconv.Itoa(st.seq)
}

// push records a nested atomic block with the savepoint sid, or "" for none.
func (st *txState) push(sid string) {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.savepoints = append(st.savepoints, sid)
}

// pop removes the innermost nested atomic block.
func (st *txState) pop() {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.savepoints = st.savepoints[:len(st.savepoints)-1]
}

// addHook registers fn to run after the commit.
func (st *txState) addHook(fn func(ctx context.Context) error, robust bool) {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.hooks = append(st.hooks, commitHook{savepoints: slices.Clone(st.savepoints), fn: fn, robust: robust})
}

// takeHooks returns the registered callbacks and forgets them.
func (st *txState) takeHooks() []commitHook {
	st.mu.Lock()
	defer st.mu.Unlock()
	hooks := st.hooks
	st.hooks = nil
	return hooks
}

// dropHooks removes the callbacks registered while the savepoint sid was open.
func (st *txState) dropHooks(sid string) {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.hooks = slices.DeleteFunc(st.hooks, func(h commitHook) bool {
		return slices.Contains(h.savepoints, sid)
	})
}

// ErrTransactionManagement is matched by the error of a transaction API used
// where it does not apply, and by the error of a query in a transaction that
// is marked for rollback, like Django's TransactionManagementError.
var ErrTransactionManagement = errors.New("orm: transaction management error")

// transactionError is an error matched by ErrTransactionManagement. It keeps
// Django's message and wraps the error that caused it, if any.
type transactionError struct {
	msg   string
	cause error
}

// Error returns Django's message.
func (e *transactionError) Error() string { return e.msg }

// Unwrap returns the cause.
func (e *transactionError) Unwrap() error { return e.cause }

// Is reports whether target is ErrTransactionManagement.
func (e *transactionError) Is(target error) bool { return target == ErrTransactionManagement }

// errNoAtomicBlock is the error of the rollback flag API outside a
// transaction.
func errNoAtomicBlock() error {
	return &transactionError{msg: "orm: the rollback flag doesn't work outside of an 'atomic' block."}
}

// conn returns the active transaction bound to ctx, or the underlying *sql.DB,
// wrapped so that every statement is reported to the DB's query hooks.
func (db *DB) conn(ctx context.Context) querier {
	if st := txStateFrom(ctx); st != nil {
		return hookedQuerier{db: db, q: st.tx, st: st}
	}
	return hookedQuerier{db: db, q: db.sqlDB}
}

// AtomicOption configures an Atomic block.
type AtomicOption func(*atomicConfig)

// atomicConfig collects the options of Atomic.
type atomicConfig struct {
	noSavepoint bool
}

// WithoutSavepoint makes a nested Atomic block run without a savepoint, like
// Django's atomic(savepoint=False). Such a block cannot roll back on its own:
// when it fails, it marks the transaction for rollback, and the innermost
// enclosing block with a savepoint rolls back when it ends. The option has no
// effect on an outermost block.
func WithoutSavepoint() AtomicOption {
	return func(c *atomicConfig) { c.noSavepoint = true }
}

// Atomic runs fn inside a database transaction bound to the context it passes
// to fn, like Django's transaction.atomic(). It commits if fn returns nil,
// rolls back on a non-nil error, and on a panic rolls back and re-panics. A
// nested Atomic (ctx already in a transaction) uses a SAVEPOINT, so the inner
// block can roll back independently of the outer transaction.
//
// When the transaction is marked for rollback, by SetRollback or by a failed
// write as described at MarkForRollbackOnError, the block rolls back even if
// fn returns nil, and Atomic returns nil, as in Django. After the commit,
// Atomic runs the OnCommit callbacks and returns the error of the first one
// that fails.
func (db *DB) Atomic(ctx context.Context, fn func(ctx context.Context) error, opts ...AtomicOption) error {
	var cfg atomicConfig
	for _, opt := range opts {
		opt(&cfg)
	}
	if st := txStateFrom(ctx); st != nil {
		return db.atomicNested(ctx, st, fn, !cfg.noSavepoint)
	}
	return db.atomicBegin(ctx, fn)
}

// ErrDurableNested is returned by AtomicDurable when it is called inside
// another atomic block.
var ErrDurableNested = errors.New("orm: a durable atomic block cannot be nested within another atomic block")

// AtomicDurable is Atomic for a block that must be the outermost one, like
// Django's atomic(durable=True): when it returns nil, the writes of fn are
// committed and not merely released to an enclosing transaction. Called
// inside another atomic block it does not run fn and returns
// ErrDurableNested. Atomic blocks nested in fn work as usual.
func (db *DB) AtomicDurable(ctx context.Context, fn func(ctx context.Context) error) error {
	if txStateFrom(ctx) != nil {
		return ErrDurableNested
	}
	return db.atomicBegin(ctx, fn)
}

// atomicBegin runs fn inside a fresh transaction begun on the underlying pool,
// and runs the OnCommit callbacks after the commit.
func (db *DB) atomicBegin(ctx context.Context, fn func(ctx context.Context) error) error {
	tx, err := db.sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("orm: begin transaction: %w", err)
	}
	st := &txState{tx: tx}
	done := false
	defer func() {
		// fn panicked.
		if !done {
			_ = tx.Rollback()
		}
	}()
	err = fn(context.WithValue(ctx, txKey{}, st))
	done = true
	if err != nil || st.rollbackFlag() {
		_ = tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("orm: commit transaction: %w", db.classify(err))
	}
	return runCommitHooks(ctx, st.takeHooks())
}

// atomicNested runs fn inside the transaction st, with a savepoint unless
// savepoint is false or the transaction is already marked for rollback, as
// Django's Atomic does.
func (db *DB) atomicNested(ctx context.Context, st *txState, fn func(ctx context.Context) error, savepoint bool) error {
	sid := ""
	if savepoint && !st.rollbackFlag() {
		var err error
		if sid, err = db.SavepointCreate(ctx); err != nil {
			return err
		}
	}
	st.push(sid)
	done := false
	defer func() {
		// fn panicked.
		if !done {
			st.pop()
			db.atomicNestedFailed(ctx, st, sid, nil)
		}
	}()
	err := fn(ctx)
	done = true
	st.pop()
	if err != nil || st.rollbackFlag() {
		db.atomicNestedFailed(ctx, st, sid, err)
		return err
	}
	if sid == "" {
		return nil
	}
	if err := db.SavepointCommit(ctx, sid); err != nil {
		// Roll back to the savepoint and release it, or leave the rollback
		// to an enclosing block when that fails too.
		if db.SavepointRollback(ctx, sid) != nil || db.SavepointCommit(ctx, sid) != nil {
			st.markForRollback(err)
		}
		return err
	}
	return nil
}

// atomicNestedFailed ends a nested block that failed with err, or whose
// transaction is marked for rollback. A block with a savepoint rolls back to
// it and clears the rollback flag. A block without one marks the transaction
// for rollback, so the enclosing block rolls back.
func (db *DB) atomicNestedFailed(ctx context.Context, st *txState, sid string, err error) {
	if sid == "" {
		st.markForRollback(err)
		return
	}
	st.setRollbackFlag(false)
	// The savepoint is not used again: release it. When that fails, leave the
	// rollback to an enclosing block.
	if db.SavepointRollback(ctx, sid) != nil || db.SavepointCommit(ctx, sid) != nil {
		st.markForRollback(err)
	}
}

// SavepointCreate creates a savepoint in the transaction of ctx and returns its
// name, like Django's transaction.savepoint_create() (savepoint() before
// Django 6.1). Outside a transaction it does nothing and returns "".
func (db *DB) SavepointCreate(ctx context.Context) (string, error) {
	st := txStateFrom(ctx)
	if st == nil {
		return "", nil
	}
	sid := st.nextSavepoint()
	if _, err := db.conn(ctx).ExecContext(ctx, "SAVEPOINT "+sid); err != nil {
		return "", fmt.Errorf("orm: savepoint %s: %w", sid, err)
	}
	return sid, nil
}

// SavepointRollback rolls the transaction of ctx back to the savepoint sid,
// like Django's transaction.savepoint_rollback(). It drops the OnCommit
// callbacks registered inside the Atomic block of that savepoint. Outside a
// transaction it does nothing.
func (db *DB) SavepointRollback(ctx context.Context, sid string) error {
	st := txStateFrom(ctx)
	if st == nil {
		return nil
	}
	if _, err := db.conn(ctx).ExecContext(ctx, "ROLLBACK TO SAVEPOINT "+sid); err != nil {
		return fmt.Errorf("orm: rollback to savepoint %s: %w", sid, err)
	}
	st.dropHooks(sid)
	return nil
}

// SavepointCommit releases the savepoint sid of the transaction of ctx, like
// Django's transaction.savepoint_commit(). Outside a transaction it does
// nothing.
func (db *DB) SavepointCommit(ctx context.Context, sid string) error {
	if txStateFrom(ctx) == nil {
		return nil
	}
	if _, err := db.conn(ctx).ExecContext(ctx, "RELEASE SAVEPOINT "+sid); err != nil {
		return fmt.Errorf("orm: release savepoint %s: %w", sid, err)
	}
	return nil
}

// GetRollback reports whether the transaction of ctx is marked for rollback,
// like Django's transaction.get_rollback(). Outside a transaction it returns
// an error matched by ErrTransactionManagement.
func (db *DB) GetRollback(ctx context.Context) (bool, error) {
	st := txStateFrom(ctx)
	if st == nil {
		return false, errNoAtomicBlock()
	}
	return st.rollbackFlag(), nil
}

// SetRollback marks the transaction of ctx for rollback, or clears the mark,
// like Django's transaction.set_rollback(). With true, the innermost enclosing
// Atomic block with a savepoint rolls back when it ends, without an error.
// With false, it prevents that rollback: use it only after rolling back to a
// known good state, for example with SavepointRollback. Outside a transaction
// it returns an error matched by ErrTransactionManagement.
func (db *DB) SetRollback(ctx context.Context, rollback bool) error {
	st := txStateFrom(ctx)
	if st == nil {
		return errNoAtomicBlock()
	}
	st.setRollbackFlag(rollback)
	return nil
}

// MarkForRollbackOnError runs fn and, when it returns an error inside a
// transaction, marks the transaction for rollback, like Django's
// transaction.mark_for_rollback_on_error(). It returns the error of fn.
//
// The ORM marks the transaction this way when a write fails: Save, Create,
// Update, Delete, BulkCreate and the related managers. Later queries in the
// transaction then fail with an error matched by ErrTransactionManagement
// until the innermost enclosing Atomic block with a savepoint ends. Wrap a
// write whose error you handle in its own Atomic block to keep using the
// transaction.
func (db *DB) MarkForRollbackOnError(ctx context.Context, fn func(ctx context.Context) error) error {
	err := fn(ctx)
	markForRollbackOnError(ctx, err)
	return err
}

// markForRollbackOnError marks the transaction of ctx for rollback when err is
// not nil.
func markForRollbackOnError(ctx context.Context, err error) {
	if err == nil {
		return
	}
	if st := txStateFrom(ctx); st != nil {
		st.markForRollback(err)
	}
}

// errNilCallback is the error of OnCommit for a nil callback.
var errNilCallback = errors.New("orm: OnCommit's callback must not be nil")

// OnCommit registers fn to run after the transaction of ctx commits, like
// Django's transaction.on_commit(). fn gets the context passed to the
// outermost Atomic, outside the transaction, and the callbacks run in the
// order of registration. A callback registered inside a nested Atomic block
// that rolls back does not run, and none runs when the transaction rolls
// back. When a callback returns an error, Atomic returns it and the callbacks
// after it do not run.
//
// Outside a transaction fn runs at once, and OnCommit returns its error.
func (db *DB) OnCommit(ctx context.Context, fn func(ctx context.Context) error) error {
	return onCommit(ctx, fn, false)
}

// OnCommitRobust is OnCommit for a callback whose error is logged with slog
// instead of stopping the other callbacks, like Django's on_commit(robust=True).
func (db *DB) OnCommitRobust(ctx context.Context, fn func(ctx context.Context) error) error {
	return onCommit(ctx, fn, true)
}

// onCommit registers fn on the transaction of ctx, or runs it at once outside
// a transaction.
func onCommit(ctx context.Context, fn func(ctx context.Context) error, robust bool) error {
	if fn == nil {
		return errNilCallback
	}
	if st := txStateFrom(ctx); st != nil {
		st.addHook(fn, robust)
		return nil
	}
	err := fn(ctx)
	if err != nil && robust {
		logCommitHookError(ctx, fn, err, "")
		return nil
	}
	return err
}

// runCommitHooks runs the callbacks of a committed transaction with ctx and
// returns the error of the first one that is not robust and fails.
func runCommitHooks(ctx context.Context, hooks []commitHook) error {
	for _, h := range hooks {
		err := h.fn(ctx)
		if err == nil {
			continue
		}
		if !h.robust {
			return err
		}
		logCommitHookError(ctx, h.fn, err, " during transaction")
	}
	return nil
}

// logCommitHookError logs the error of a robust OnCommit callback with
// Django's message.
func logCommitHookError(ctx context.Context, fn func(ctx context.Context) error, err error, where string) {
	name := runtime.FuncForPC(reflect.ValueOf(fn).Pointer()).Name()
	slog.ErrorContext(ctx, fmt.Sprintf("Error calling %s in OnCommit()%s (%v).", name, where, err), "error", err)
}
