package sessions_test

import (
	"context"
	"testing"

	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/orm/backends/sqlite"
	"github.com/oliverhaas/djangogo/sessions"
)

// newDBStore builds a fresh in-memory SQLite DB with the sessions table created and
// returns the store alongside the DB.
func newDBStore(t *testing.T) (*sessions.DBStore, *orm.DB) {
	t.Helper()
	reg := orm.NewRegistry()
	model, err := reg.Register(&sessions.Record{})
	if err != nil {
		t.Fatalf("Register(Record): %v", err)
	}
	if err := reg.Resolve(); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	reg.Freeze()

	dsn := "file:" + t.Name() + "?mode=memory&cache=shared"
	sdb, err := sqlite.Open(dsn)
	if err != nil {
		t.Fatalf("sqlite.Open: %v", err)
	}
	t.Cleanup(func() { _ = sdb.Close() })

	db := orm.NewDB(sdb, sqlite.New(), reg)
	if err := db.CreateTable(context.Background(), model); err != nil {
		t.Fatalf("CreateTable: %v", err)
	}
	return sessions.NewDBStore(db), db
}

func TestDBStoreRoundTrip(t *testing.T) {
	ctx := context.Background()
	store, _ := newDBStore(t)

	s := store.New()
	if s.Key() != "" {
		t.Fatalf("New session already has a key: %q", s.Key())
	}
	s.Set("uid", float64(42))

	key, err := store.Encode(ctx, s)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if key == "" {
		t.Fatal("Encode returned an empty key")
	}

	got, err := store.Decode(ctx, key)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if got.Key() != key {
		t.Fatalf("Decode key = %q; want %q", got.Key(), key)
	}
	if v, ok := got.Get("uid"); !ok || v != float64(42) {
		t.Fatalf("Decode uid = %v, %v; want 42", v, ok)
	}
}

func TestDBStoreUpdateSameRow(t *testing.T) {
	ctx := context.Background()
	store, db := newDBStore(t)

	s := store.New()
	s.Set("uid", float64(1))
	key, err := store.Encode(ctx, s)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	// Re-encode the same session (same key) with a new value: it must update the row,
	// not insert a second one.
	s.Set("uid", float64(2))
	key2, err := store.Encode(ctx, s)
	if err != nil {
		t.Fatalf("re-Encode: %v", err)
	}
	if key2 != key {
		t.Fatalf("re-Encode changed the key: %q -> %q", key, key2)
	}

	n, err := orm.Query[sessions.Record](db).Count(ctx)
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected exactly one row, got %d", n)
	}

	got, err := store.Decode(ctx, key)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if v, _ := got.Get("uid"); v != float64(2) {
		t.Fatalf("updated uid = %v; want 2", v)
	}
}

func TestDBStoreDelete(t *testing.T) {
	ctx := context.Background()
	store, db := newDBStore(t)

	s := store.New()
	s.Set("uid", float64(1))
	key, err := store.Encode(ctx, s)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	if err := store.Delete(ctx, s); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	n, err := orm.Query[sessions.Record](db).Count(ctx)
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if n != 0 {
		t.Fatalf("row still present after Delete, count = %d", n)
	}

	// Decoding a now-unknown key yields a fresh empty session, no error.
	got, err := store.Decode(ctx, key)
	if err != nil {
		t.Fatalf("Decode of deleted key returned error: %v", err)
	}
	if len(got.Data()) != 0 || got.Key() != "" {
		t.Fatal("Decode of deleted key did not return a fresh empty session")
	}
}

func TestDBStoreDecodeUnknownKey(t *testing.T) {
	ctx := context.Background()
	store, _ := newDBStore(t)

	got, err := store.Decode(ctx, "does-not-exist")
	if err != nil {
		t.Fatalf("Decode of unknown key returned error: %v", err)
	}
	if len(got.Data()) != 0 {
		t.Fatal("Decode of unknown key returned non-empty session")
	}
}
