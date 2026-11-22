package orm

import (
	"context"
	"database/sql"
	"sync"
)

// QueryHook observes one SQL statement just before the DB sends it to the
// driver. query is the final SQL text and args its bound parameters. Hooks see
// every statement the ORM issues (SELECT, INSERT, UPDATE, DELETE, DDL, and
// SAVEPOINT control statements), but not the BEGIN/COMMIT/ROLLBACK that
// database/sql issues for the outermost Atomic block.
type QueryHook func(ctx context.Context, query string, args []any)

// queryHooks is the concurrency-safe set of hooks registered on a DB.
type queryHooks struct {
	mu    sync.RWMutex
	next  int
	hooks map[int]QueryHook
}

// AddQueryHook registers h to observe every statement db executes and returns a
// function that removes it again. It is safe for concurrent use; the test
// toolkit uses it to count queries (the equivalent of Django's
// assertNumQueries and CaptureQueriesContext).
func (db *DB) AddQueryHook(h QueryHook) (remove func()) {
	db.hooks.mu.Lock()
	defer db.hooks.mu.Unlock()
	if db.hooks.hooks == nil {
		db.hooks.hooks = make(map[int]QueryHook)
	}
	id := db.hooks.next
	db.hooks.next++
	db.hooks.hooks[id] = h
	return func() {
		db.hooks.mu.Lock()
		defer db.hooks.mu.Unlock()
		delete(db.hooks.hooks, id)
	}
}

// notify calls every registered hook with the statement about to run.
func (db *DB) notify(ctx context.Context, query string, args []any) {
	db.hooks.mu.RLock()
	if len(db.hooks.hooks) == 0 {
		db.hooks.mu.RUnlock()
		return
	}
	hooks := make([]QueryHook, 0, len(db.hooks.hooks))
	for _, h := range db.hooks.hooks {
		hooks = append(hooks, h)
	}
	db.hooks.mu.RUnlock()
	for _, h := range hooks {
		h(ctx, query, args)
	}
}

// hookedQuerier wraps a querier so that every statement is reported to the DB's
// query hooks before it runs, and a constraint violation comes back as an
// *IntegrityError. In a transaction marked for rollback it runs no statement
// and returns the error of a broken transaction, as Django does.
type hookedQuerier struct {
	db *DB
	q  querier
	// st is the transaction q belongs to, or nil outside a transaction.
	st *txState
}

// ExecContext reports the statement to the hooks and executes it.
func (h hookedQuerier) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	if h.st != nil {
		if err := h.st.brokenErr(); err != nil {
			return nil, err
		}
	}
	h.db.notify(ctx, query, args)
	result, err := h.q.ExecContext(ctx, query, args...)
	return result, h.db.classify(err)
}

// QueryContext reports the statement to the hooks and runs the query.
func (h hookedQuerier) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	if h.st != nil {
		if err := h.st.brokenErr(); err != nil {
			return nil, err
		}
	}
	h.db.notify(ctx, query, args)
	rows, err := h.q.QueryContext(ctx, query, args...)
	return rows, h.db.classify(err)
}
