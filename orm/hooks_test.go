package orm_test

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/orm/backends/sqlite"
)

type hookItem struct {
	ID   int64
	Name string `orm:"max_length=50"`
}

func newHookDB(t *testing.T) *orm.DB {
	t.Helper()
	reg := orm.NewRegistry()
	m, err := reg.Register(&hookItem{})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := reg.Resolve(); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	reg.Freeze()
	sdb, err := sqlite.Open("file:" + t.Name() + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = sdb.Close() })
	db := orm.NewDB(sdb, sqlite.New(), reg)
	if err := db.CreateTable(context.Background(), m); err != nil {
		t.Fatalf("CreateTable: %v", err)
	}
	return db
}

func TestQueryHookObservesStatements(t *testing.T) {
	db := newHookDB(t)
	ctx := context.Background()

	var mu sync.Mutex
	var seen []string
	remove := db.AddQueryHook(func(_ context.Context, query string, _ []any) {
		mu.Lock()
		defer mu.Unlock()
		seen = append(seen, query)
	})

	item := hookItem{Name: "a"}
	if err := orm.Query[hookItem](db).Create(ctx, &item); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := orm.Query[hookItem](db).All(ctx); err != nil {
		t.Fatalf("All: %v", err)
	}
	if n, err := orm.Query[hookItem](db).Count(ctx); err != nil || n != 1 {
		t.Fatalf("Count = %d, %v", n, err)
	}
	if len(seen) != 3 {
		t.Fatalf("hook saw %d statements, want 3: %q", len(seen), seen)
	}
	if !strings.HasPrefix(seen[0], "INSERT") || !strings.HasPrefix(seen[1], "SELECT") {
		t.Errorf("unexpected statements: %q", seen)
	}

	remove()
	if _, err := orm.Query[hookItem](db).All(ctx); err != nil {
		t.Fatalf("All: %v", err)
	}
	if len(seen) != 3 {
		t.Errorf("hook still called after remove: %q", seen)
	}
}

func TestQueryHookSeesSavepointsAndTransactionQueries(t *testing.T) {
	db := newHookDB(t)
	ctx := context.Background()

	var seen []string
	defer db.AddQueryHook(func(_ context.Context, query string, _ []any) {
		seen = append(seen, query)
	})()

	err := db.Atomic(ctx, func(ctx context.Context) error {
		return db.Atomic(ctx, func(ctx context.Context) error {
			item := hookItem{Name: "nested"}
			return orm.Query[hookItem](db).Create(ctx, &item)
		})
	})
	if err != nil {
		t.Fatalf("Atomic: %v", err)
	}
	want := []string{"SAVEPOINT sp_1", "INSERT", "RELEASE SAVEPOINT sp_1"}
	if len(seen) != len(want) {
		t.Fatalf("hook saw %q, want prefixes %q", seen, want)
	}
	for i, prefix := range want {
		if !strings.HasPrefix(seen[i], prefix) {
			t.Errorf("statement %d = %q, want prefix %q", i, seen[i], prefix)
		}
	}
}
