package orm

import (
	"context"
	"database/sql"
	"fmt"
)

// querier is the subset of *sql.DB and *sql.Tx that the executor needs. Routing
// every database call through a querier lets a terminal run either directly on
// the pool or inside the transaction bound to its context.
type querier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// txKey is the context key under which the active *sql.Tx is stored.
type txKey struct{}

// depthKey is the context key under which the current savepoint nesting depth is
// stored. The outermost Atomic runs at depth 0; each nested Atomic increments it.
type depthKey struct{}

// contextWithTx returns a copy of ctx carrying tx as its active transaction.
func contextWithTx(ctx context.Context, tx *sql.Tx) context.Context {
	return context.WithValue(ctx, txKey{}, tx)
}

// txFromContext returns the active transaction bound to ctx, if any.
func txFromContext(ctx context.Context) (*sql.Tx, bool) {
	tx, ok := ctx.Value(txKey{}).(*sql.Tx)
	return tx, ok
}

// depthFromContext returns the current savepoint nesting depth bound to ctx, or
// 0 when none is set.
func depthFromContext(ctx context.Context) int {
	d, _ := ctx.Value(depthKey{}).(int)
	return d
}

// conn returns the active transaction bound to ctx, or the underlying *sql.DB.
func (db *DB) conn(ctx context.Context) querier {
	if tx, ok := txFromContext(ctx); ok {
		return tx
	}
	return db.sqlDB
}

// Atomic runs fn inside a database transaction bound to the context it passes to
// fn. It commits if fn returns nil, rolls back on a non-nil error, and on a panic
// rolls back and re-panics. A nested Atomic (ctx already in a transaction) uses a
// SAVEPOINT, so the inner block can roll back independently of the outer
// transaction.
func (db *DB) Atomic(ctx context.Context, fn func(ctx context.Context) error) error {
	if tx, ok := txFromContext(ctx); ok {
		return db.atomicSavepoint(ctx, tx, fn)
	}
	return db.atomicBegin(ctx, fn)
}

// atomicBegin runs fn inside a fresh transaction begun on the underlying pool.
func (db *DB) atomicBegin(ctx context.Context, fn func(ctx context.Context) error) error {
	tx, err := db.sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("orm: begin transaction: %w", err)
	}
	committed := false
	defer func() {
		if r := recover(); r != nil {
			_ = tx.Rollback()
			panic(r)
		}
		if !committed {
			_ = tx.Rollback()
		}
	}()

	txCtx := contextWithTx(ctx, tx)
	if err := fn(txCtx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("orm: commit transaction: %w", err)
	}
	committed = true
	return nil
}

// atomicSavepoint runs fn against a SAVEPOINT within the active transaction, so a
// failure rolls back only the nested block and not the enclosing transaction.
func (db *DB) atomicSavepoint(ctx context.Context, tx *sql.Tx, fn func(ctx context.Context) error) error {
	depth := depthFromContext(ctx) + 1
	name := fmt.Sprintf("sp_%d", depth)

	if _, err := tx.ExecContext(ctx, "SAVEPOINT "+name); err != nil {
		return fmt.Errorf("orm: savepoint %s: %w", name, err)
	}
	released := false
	defer func() {
		if r := recover(); r != nil {
			_, _ = tx.ExecContext(ctx, "ROLLBACK TO SAVEPOINT "+name)
			panic(r)
		}
		if !released {
			_, _ = tx.ExecContext(ctx, "ROLLBACK TO SAVEPOINT "+name)
		}
	}()

	innerCtx := context.WithValue(ctx, depthKey{}, depth)
	if err := fn(innerCtx); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "RELEASE SAVEPOINT "+name); err != nil {
		return fmt.Errorf("orm: release savepoint %s: %w", name, err)
	}
	released = true
	return nil
}
