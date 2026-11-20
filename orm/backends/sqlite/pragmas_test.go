package sqlite_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/oliverhaas/djangogo/orm/backends/sqlite"
)

// TestForeignKeysPragmaOnEveryConnection checks that PRAGMA foreign_keys is ON
// on every connection opened from the SQLite pool, including multiple
// concurrent connections to a file-backed database.
func TestForeignKeysPragmaOnEveryConnection(t *testing.T) {
	ctx := context.Background()

	t.Run("in-memory", func(t *testing.T) {
		db, err := sqlite.Open(":memory:")
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		defer db.Close()

		var fk int
		if err := db.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&fk); err != nil {
			t.Fatalf("PRAGMA foreign_keys: %v", err)
		}
		if fk != 1 {
			t.Fatalf("PRAGMA foreign_keys = %d, want 1", fk)
		}
	})

	t.Run("file multi-connection pool", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "fk.sqlite3")
		db, err := sqlite.Open(path)
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		defer db.Close()
		db.SetMaxOpenConns(4)

		// Hold three distinct connections simultaneously to force the pool to
		// open multiple SQLite connections, and check the pragma on each one.
		conns := make([]interface{ Close() error }, 0, 3)
		for i := range 3 {
			conn, err := db.Conn(ctx)
			if err != nil {
				t.Fatalf("Conn %d: %v", i, err)
			}
			conns = append(conns, conn)
			var fk int
			if err := conn.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&fk); err != nil {
				t.Fatalf("Conn %d PRAGMA foreign_keys: %v", i, err)
			}
			if fk != 1 {
				t.Errorf("Conn %d PRAGMA foreign_keys = %d, want 1", i, fk)
			}
		}
		for _, c := range conns {
			_ = c.Close()
		}
	})
}

// TestForeignKeysPragmaStaysLocal checks that the pragma reaches only the pools
// that Open returns, not other SQLite connections of the program.
func TestForeignKeysPragmaStaysLocal(t *testing.T) {
	ctx := context.Background()
	for _, dsn := range []string{":memory:", "file::memory:?cache=shared"} {
		db, err := sqlite.Open(dsn)
		if err != nil {
			t.Fatalf("Open(%q): %v", dsn, err)
		}
		_ = db.Close()
	}
	raw, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer raw.Close()
	var fk int
	if err := raw.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&fk); err != nil {
		t.Fatalf("PRAGMA foreign_keys: %v", err)
	}
	if fk != 0 {
		t.Fatalf("PRAGMA foreign_keys = %d on a connection opened outside the backend, want 0", fk)
	}
}

// TestForeignKeysPragmaOptOut checks that a DSN naming foreign_keys keeps its
// own setting.
func TestForeignKeysPragmaOptOut(t *testing.T) {
	db, err := sqlite.Open(":memory:?_pragma=foreign_keys(0)")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()
	var fk int
	if err := db.QueryRowContext(context.Background(), "PRAGMA foreign_keys").Scan(&fk); err != nil {
		t.Fatalf("PRAGMA foreign_keys: %v", err)
	}
	if fk != 0 {
		t.Fatalf("PRAGMA foreign_keys = %d, want 0", fk)
	}
}
