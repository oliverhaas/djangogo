# Djan-Go-Go -- Milestone 4: ORM relations + PostgreSQL

> Detailed plan for M4. Adds a second backend (PostgreSQL via pgx) and Django-style
> relations (FK forward + reverse, select_related JOIN, prefetch_related batched IN),
> context-bound transactions, and pre/post save/delete signals. Conventions: TDD,
> conventional commits straight to `main`, `gofmt`/`go vet`/`golangci-lint` (0 issues)
> per commit. No em-dashes (use `--`). Go on PATH: `export PATH="$HOME/.local/go/bin:$HOME/go/bin:$PATH"`.

**Goal:** Traverse relations (FK forward + reverse) with `select_related` and
`prefetch_related`, run them inside transactions, fire signals, and have it all work
on BOTH SQLite and PostgreSQL.

**Exit criteria:** FK traversal + select_related + prefetch_related work on SQLite and
PostgreSQL; transactions commit/rollback; signals fire; dual-dialect integration tests
green (Postgres tests skip when no DSN is configured, run in CI via a container).
`go test ./...`, `go vet`, `golangci-lint` all green.

**Testing Postgres:** tests read `DJANGOGO_TEST_POSTGRES_DSN`; when unset they
`t.Skip`. Locally/CI a Postgres container provides the DSN (Docker is available here).

---

## Work packages

### W1 -- PostgreSQL backend + RETURNING-aware Create
New `orm/backends/postgres` package (pgx stdlib `database/sql` via `pgx/v5/stdlib`, or
`jackc/pgx/v5` pool wrapped to `database/sql`). Implement `orm.Dialect`:
- `Name() "postgres"`, `Placeholder(n) "$"+n`, `Quote` double-quotes, `SupportsReturning() true`.
- `ColumnType`: KindAuto -> `BIGSERIAL PRIMARY KEY`; KindInt -> `BIGINT`; KindChar ->
  `VARCHAR(n)`; KindText -> `TEXT`; KindBool -> `BOOLEAN`; KindDateTime -> `TIMESTAMPTZ`;
  with `NOT NULL`/`PRIMARY KEY`/`UNIQUE` like sqlite.
- `CreateTableSQL` deterministic; `Open(dsn) (*sql.DB, error)`.
Executor refactor (`orm/execute.go`): `Create` uses `INSERT ... RETURNING <pk>` and
`QueryRowContext().Scan(&pk)` when `dialect.SupportsReturning()`, else `LastInsertId`
(sqlite path unchanged). Keep both paths covered by tests. Add a postgres integration
test (skips without DSN) doing the same CRUD round-trip as the sqlite exec test.

### W2 -- FK[T] forward relations (metadata + DDL + scan/save + loader)
- `orm/relations.go`: generic `FK[T any]` value type holding the related PK and an
  optional loaded `*T`. API: `FK.PK() int64`, `FK.Set(pk int64)`, `FK.SetObject(*T)`,
  `FK.Object() (*T, bool)`. A marker interface `relation { relKind() RelKind;
  targetType() reflect.Type }` implemented by FK[T] so reflection can detect it.
- `orm/field.go`/`model.go`/`registry.go`: detect a relation-typed struct field;
  produce a column `<snake(field)>_id` of `KindInt` carrying a `*Relation{Kind:RelFK,
  Target:<model name>, Column:"<f>_id"}` (add `Rel *Relation` to `Field`). Relations
  resolve AFTER all models register (cross-app), so add a registry resolve pass run by
  `Freeze` (or an explicit `Resolve()` before Freeze) that links each relation to its
  target `*Model` and validates the target exists.
- Dialect DDL: an FK column emits its integer type plus a table-level (or inline)
  `REFERENCES <target_table>(<target_pk>)` foreign-key constraint. Extend
  `CreateTableSQL` in both dialects to append FK constraints.
- Executor: scan/insert the FK id (the FK column maps to `FK[T].pk`); reflection in
  `scanRows`/`Create` reads/writes the embedded pk through the FK value.
- `migrations`: `FieldState` gains relation info (RelKind, Target, Column); DDL ops
  emit the FK constraint; autodetect treats relation columns like any column.
- Loader: `FK[T].Fetch(ctx, db) (*T, error)` runs `Query[T](db).Get(ctx, "id", pk)`.

### W3 -- select_related (JOIN) + reverse FK
- `QuerySet[T].SelectRelated(fields ...string) *QuerySet[T]`: compile a LEFT JOIN to
  each named FK's target table, select the target columns, and on scan populate the
  FK field's loaded `*T`. (Single-level FK first; nested is a later refinement.)
- Reverse FK: `RelatedManager` helper, e.g. `orm.Related[Child](db, parent, "fkField")`
  returning a `QuerySet[Child]` filtered by the FK column = parent PK. (Or a
  `ReverseFK[T]` field; keep it a query helper for the PoC.)

### W4 -- transactions + signals
- Transactions: `DB.Atomic(ctx, func(ctx context.Context) error) error` running the fn
  inside a `database/sql` tx bound to the context; commit on nil, rollback on error OR
  panic (recover, rollback, re-panic). Nested `Atomic` uses SAVEPOINTs. Terminal
  QuerySet methods use the context's tx when present (a context key holding the `*sql.Tx`).
- Signals: a small `orm/signals.go` with `PreSave/PostSave/PreDelete/PostDelete`
  registries keyed by model; `Create`/`Update`/`Delete` fire them. Generic-friendly
  registration `orm.OnPostSave[T](func(ctx, *T) error)`.

### W5 -- prefetch_related + dual-dialect exit test
- `QuerySet[Parent].PrefetchRelated(...)` for reverse FK: after the main query, batch
  a single `WHERE fk_col IN (parent pks...)` and attach children. (Expose results via a
  side map returned alongside, or a typed helper, since Go structs cannot hold an open
  reverse manager easily; keep the API pragmatic and documented.)
- Exit integration test (`orm`-level, external package): models `Author` and `Article`
  (Article has `FK[Author]`); run CRUD + FK Fetch + select_related JOIN + reverse query
  + prefetch, inside a transaction, with signals firing, on SQLite AND (skip-guarded)
  PostgreSQL. Then the M4 review.

Scope note: O2O and M2M are partially covered (O2O = FK + UNIQUE is cheap; M2M
through-table + managers may be slimmed or deferred and clearly noted) to keep M4
focused on the FK traversal + dual-dialect exit criterion.
