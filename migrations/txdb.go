package migrations

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"sync/atomic"

	"github.com/oliverhaas/djangogo/orm"
)

// newTxBoundDB returns an orm.DB whose every statement runs on run, the open
// transaction of a migration. The ORM offers no way to hand it an existing
// *sql.Tx, so this file adapts the transaction to a database/sql driver: the
// *sql.DB inside the returned handle forwards statements to the transaction,
// and a nested transaction (orm.DB.Atomic) becomes a savepoint.
func newTxBoundDB(base *orm.DB, run sqlRunner) *orm.DB {
	db := orm.NewDB(sql.OpenDB(&txConnector{run: run}), base.Dialect(), base.Registry())
	db.Now = base.Now
	return db
}

// txConnector hands out connections that all write to the same transaction.
type txConnector struct {
	run        sqlRunner
	savepoints atomic.Int64
}

// Connect returns a connection bound to the transaction.
func (c *txConnector) Connect(context.Context) (driver.Conn, error) {
	return &txConn{connector: c}, nil
}

// Driver returns a driver that cannot open connections by name.
func (c *txConnector) Driver() driver.Driver { return txDriver{} }

// txDriver exists to satisfy driver.Connector; a transaction-bound handle is
// never opened from a data source name.
type txDriver struct{}

// Open always fails.
func (txDriver) Open(string) (driver.Conn, error) {
	return nil, errors.New("migrations: a transaction-bound database cannot be opened by name")
}

// txConn forwards statements to the migration transaction.
type txConn struct {
	connector *txConnector
}

// Prepare returns a statement that executes on the transaction.
func (c *txConn) Prepare(query string) (driver.Stmt, error) {
	return &txStmt{conn: c, query: query}, nil
}

// Close does nothing: the transaction belongs to the migration.
func (c *txConn) Close() error { return nil }

// Begin starts a savepoint.
func (c *txConn) Begin() (driver.Tx, error) {
	return c.BeginTx(context.Background(), driver.TxOptions{})
}

// BeginTx starts a savepoint, the nested form of a transaction.
func (c *txConn) BeginTx(ctx context.Context, _ driver.TxOptions) (driver.Tx, error) {
	name := fmt.Sprintf("djangogo_migration_%d", c.connector.savepoints.Add(1))
	if _, err := c.connector.run.ExecContext(ctx, "SAVEPOINT "+name); err != nil {
		return nil, err
	}
	return &txSavepoint{run: c.connector.run, name: name}, nil
}

// ExecContext executes a statement on the transaction.
func (c *txConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	return c.connector.run.ExecContext(ctx, query, namedArgs(args)...)
}

// QueryContext runs a query on the transaction.
func (c *txConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	rows, err := c.connector.run.QueryContext(ctx, query, namedArgs(args)...)
	if err != nil {
		return nil, err
	}
	cols, err := rows.Columns()
	if err != nil {
		_ = rows.Close()
		return nil, err
	}
	return &txRows{rows: rows, cols: cols}, nil
}

// namedArgs converts driver arguments back to database/sql arguments.
func namedArgs(args []driver.NamedValue) []any {
	out := make([]any, len(args))
	for i, a := range args {
		if a.Name != "" {
			out[i] = sql.Named(a.Name, a.Value)
		} else {
			out[i] = a.Value
		}
	}
	return out
}

// txSavepoint is a nested transaction inside the migration transaction.
type txSavepoint struct {
	run  sqlRunner
	name string
}

// Commit releases the savepoint.
func (s *txSavepoint) Commit() error {
	_, err := s.run.ExecContext(context.Background(), "RELEASE SAVEPOINT "+s.name)
	return err
}

// Rollback undoes the work since the savepoint.
func (s *txSavepoint) Rollback() error {
	if _, err := s.run.ExecContext(context.Background(), "ROLLBACK TO SAVEPOINT "+s.name); err != nil {
		return err
	}
	_, err := s.run.ExecContext(context.Background(), "RELEASE SAVEPOINT "+s.name)
	return err
}

// txStmt is a prepared statement that is executed on the transaction as plain text.
type txStmt struct {
	conn  *txConn
	query string
}

// Close does nothing.
func (s *txStmt) Close() error { return nil }

// NumInput reports that the number of placeholders is unknown.
func (s *txStmt) NumInput() int { return -1 }

// Exec executes the statement.
func (s *txStmt) Exec(args []driver.Value) (driver.Result, error) {
	return s.conn.ExecContext(context.Background(), s.query, positional(args))
}

// Query runs the statement as a query.
func (s *txStmt) Query(args []driver.Value) (driver.Rows, error) {
	return s.conn.QueryContext(context.Background(), s.query, positional(args))
}

// ExecContext executes the statement.
func (s *txStmt) ExecContext(ctx context.Context, args []driver.NamedValue) (driver.Result, error) {
	return s.conn.ExecContext(ctx, s.query, args)
}

// QueryContext runs the statement as a query.
func (s *txStmt) QueryContext(ctx context.Context, args []driver.NamedValue) (driver.Rows, error) {
	return s.conn.QueryContext(ctx, s.query, args)
}

// positional numbers plain driver values.
func positional(args []driver.Value) []driver.NamedValue {
	out := make([]driver.NamedValue, len(args))
	for i, a := range args {
		out[i] = driver.NamedValue{Ordinal: i + 1, Value: a}
	}
	return out
}

// txRows adapts *sql.Rows to driver.Rows.
type txRows struct {
	rows *sql.Rows
	cols []string
}

// Columns returns the column names.
func (r *txRows) Columns() []string { return r.cols }

// Close closes the underlying rows.
func (r *txRows) Close() error { return r.rows.Close() }

// Next copies the next row into dest.
func (r *txRows) Next(dest []driver.Value) error {
	if !r.rows.Next() {
		if err := r.rows.Err(); err != nil {
			return err
		}
		return io.EOF
	}
	vals := make([]any, len(dest))
	ptrs := make([]any, len(dest))
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	if err := r.rows.Scan(ptrs...); err != nil {
		return err
	}
	for i, v := range vals {
		dest[i] = v
	}
	return nil
}
