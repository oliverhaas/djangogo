package djangotest

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/orm/backends/postgres"
	"github.com/oliverhaas/djangogo/orm/backends/sqlite"
)

// PostgresDSNEnv is the environment variable naming the PostgreSQL database
// used by NewPostgresDB and EachDB. PostgreSQL tests are skipped when it is
// unset.
const PostgresDSNEnv = "DJANGOGO_TEST_POSTGRES_DSN"

// AppModels is a set of models registered under one app label, the equivalent
// of the models.py of a Django test app. It satisfies the app contract
// (Name and Models), so it can be passed wherever an app is accepted.
type AppModels struct {
	label  string
	models []any
}

// App groups models under the app label, so they are registered as
// "<label>.<Model>" with the default table "<label>_<model>", as Django names
// them. Pass the result to NewDB, NewPostgresDB, or EachDB.
func App(label string, models ...any) AppModels {
	return AppModels{label: label, models: models}
}

// Name returns the app label.
func (a AppModels) Name() string { return a.label }

// Models returns the app's model pointers.
func (a AppModels) Models() []any { return a.models }

// modelApp is the part of the app contract the test databases need. Framework
// apps such as auth.App satisfy it, as does AppModels.
type modelApp interface {
	Name() string
	Models() []any
}

// NewRegistry builds a resolved, frozen model registry from models. Each
// element is either a pointer to a model struct, registered without an app
// label, or an app (anything with Name and Models methods, such as the result
// of App or auth.App{}), whose models are registered under its name.
func NewRegistry(t testing.TB, models ...any) *orm.Registry {
	t.Helper()
	reg := orm.NewRegistry()
	for _, entry := range models {
		if app, ok := entry.(modelApp); ok {
			for _, m := range app.Models() {
				if _, err := reg.RegisterApp(app.Name(), m); err != nil {
					t.Fatalf("djangotest: register %T in app %q: %v", m, app.Name(), err)
				}
			}
			continue
		}
		if _, err := reg.Register(entry); err != nil {
			t.Fatalf("djangotest: register %T: %v", entry, err)
		}
	}
	if err := reg.Resolve(); err != nil {
		t.Fatalf("djangotest: resolve relations: %v", err)
	}
	reg.Freeze()
	return reg
}

// NewDB returns a private in-memory SQLite database with a table created for
// every given model. See NewRegistry for what models accepts. The database is
// closed when the test ends.
func NewDB(t testing.TB, models ...any) *orm.DB {
	t.Helper()
	reg := NewRegistry(t, models...)
	sdb, err := sqlite.Open("file:djangotest_" + randomName(t) + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("djangotest: open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = sdb.Close() })
	db := orm.NewDB(sdb, sqlite.New(), reg)
	createTables(t, db)
	return db
}

// NewPostgresDB returns a PostgreSQL database handle confined to a fresh,
// private schema with a table created for every given model. The schema is
// dropped when the test ends. The test is skipped when the PostgresDSNEnv
// environment variable is unset.
func NewPostgresDB(t testing.TB, models ...any) *orm.DB {
	t.Helper()
	dsn := os.Getenv(PostgresDSNEnv)
	if dsn == "" {
		t.Skipf("%s not set; skipping PostgreSQL test", PostgresDSNEnv)
	}
	reg := NewRegistry(t, models...)
	ctx := context.Background()

	admin, err := postgres.Open(dsn)
	if err != nil {
		t.Fatalf("djangotest: open postgres: %v", err)
	}
	schema := "dt_" + randomName(t)
	if _, err := admin.ExecContext(ctx, `CREATE SCHEMA "`+schema+`"`); err != nil {
		_ = admin.Close()
		t.Fatalf("djangotest: create schema %s: %v", schema, err)
	}
	t.Cleanup(func() {
		_, _ = admin.ExecContext(context.Background(), `DROP SCHEMA IF EXISTS "`+schema+`" CASCADE`)
		_ = admin.Close()
	})

	scoped, err := schemaDSN(dsn, schema)
	if err != nil {
		t.Fatalf("djangotest: %v", err)
	}
	pdb, err := postgres.Open(scoped)
	if err != nil {
		t.Fatalf("djangotest: open postgres schema %s: %v", schema, err)
	}
	// Registered after the schema cleanup, so it runs first: the pool is closed
	// before the schema is dropped.
	t.Cleanup(func() { _ = pdb.Close() })

	db := orm.NewDB(pdb, postgres.New(), reg)
	createTables(t, db)
	return db
}

// EachDB runs fn once per database dialect as a subtest: "sqlite" always, and
// "postgres" when the PostgresDSNEnv environment variable is set (the subtest
// is skipped otherwise). Every run gets its own empty database built from
// models, so a test ported from Django exercises both backends from one body.
func EachDB(t *testing.T, models []any, fn func(t *testing.T, db *orm.DB)) {
	t.Helper()
	t.Run("sqlite", func(t *testing.T) {
		fn(t, NewDB(t, models...))
	})
	t.Run("postgres", func(t *testing.T) {
		fn(t, NewPostgresDB(t, models...))
	})
}

// createTables creates the table of every registered model, referenced tables
// before the tables that reference them. One transaction holds them all, so
// PostgreSQL commits the schema once instead of once per statement.
func createTables(t testing.TB, db *orm.DB) {
	t.Helper()
	err := db.Atomic(context.Background(), func(ctx context.Context) error {
		for _, m := range creationOrder(db.Registry().Models()) {
			if err := db.CreateTable(ctx, m); err != nil {
				return fmt.Errorf("create table for %s: %w", m.Label(), err)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("djangotest: %v", err)
	}
}

// creationOrder sorts models so that a model follows every model its foreign
// keys reference, keeping registration order otherwise. A reference cycle (or a
// self reference) cannot be ordered; its members keep registration order.
func creationOrder(models []*orm.Model) []*orm.Model {
	out := make([]*orm.Model, 0, len(models))
	state := make(map[*orm.Model]int, len(models)) // 0 new, 1 visiting, 2 done
	var visit func(m *orm.Model)
	visit = func(m *orm.Model) {
		if state[m] != 0 {
			return
		}
		state[m] = 1
		for _, f := range m.Fields() {
			if f.Rel != nil && f.Rel.Target != nil {
				visit(f.Rel.Target)
			}
		}
		for _, f := range m.ManyToMany() {
			if f.Rel != nil && f.Rel.AutoCreated && f.Rel.Target != nil {
				visit(f.Rel.Target)
			}
		}
		state[m] = 2
		out = append(out, m)
	}
	for _, m := range models {
		visit(m)
	}
	return out
}

// schemaDSN returns dsn with the connection's search_path set to schema, so
// every statement on the returned DSN's connections resolves unqualified table
// names inside that schema. Both URL and keyword/value DSNs are supported.
func schemaDSN(dsn, schema string) (string, error) {
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		u, err := url.Parse(dsn)
		if err != nil {
			return "", err
		}
		q := u.Query()
		q.Set("search_path", schema)
		u.RawQuery = q.Encode()
		return u.String(), nil
	}
	return dsn + " search_path=" + schema, nil
}

// randomName returns a short random lowercase identifier, unique per call, used
// to name private databases and schemas.
func randomName(t testing.TB) string {
	t.Helper()
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		t.Fatalf("djangotest: random name: %v", err)
	}
	return hex.EncodeToString(b)
}
