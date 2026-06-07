# Djan-Go-Go -- Milestone 3: Migrations

> Detailed plan for M3. `makemigrations` autodetect plus `migrate` apply against
> SQLite, reaching parity with the validated go-django spike. Operates on the M2
> field kinds (Char/Text/Int/Bool/DateTime); relations (FK) arrive in M4, so the
> dependency ordering here is by model creation order, not FK edges.
> Conventions: TDD (red, green, commit), tests beside code, conventional commits
> straight to `main`, `export PATH="$HOME/.local/go/bin:$HOME/go/bin:$PATH"`,
> `gofmt`/`go vet`/`golangci-lint` (0 issues) clean per commit. No em-dashes (use `--`).

**Goal:** Reflect the current model registry, diff it against the prior migration
state, emit typed operations, write editable Go migration files, and apply them
(in a transaction, recorded in a tracking table) to build the SQLite schema.

**Exit criteria:** `manage makemigrations` writes a migration file for a new model;
`manage migrate` builds the schema; re-running `makemigrations` detects no changes.
`go test ./...`, `go vet`, `golangci-lint` all green.

---

## Architecture

New package `migrations` (imports `orm` plus stdlib). The `orm` package gains one
accessor (`Registry.Models() []*Model`). Wiring touches `conf`, `apps`, and the root
`djangogo` package (Application plus two commands).

```
orm/registry.go      + (r *Registry) Models() []*Model   // ordered snapshot accessor
migrations/
  state.go      FieldState, ModelState, ProjectState; StateFromRegistry; Clone; equality
  operations.go Operation interface + CreateModel/DeleteModel/AddField/RemoveField/AlterField
  ddl.go        SQLite DDL per op (CREATE/DROP/ADD COLUMN; temp-table rebuild for Remove/Alter)
  autodetect.go Diff(old, new *ProjectState) []Operation
  migration.go  Migration{App,Name,Dependencies,Operations}; StateFromMigrations replay
  registry.go   migrations.Registry: apps register their Migration sets; ordered per app
  recorder.go   tracking table (djangogo_migrations) + applied-set query + record
  runner.go     Apply(ctx, *orm.DB, []Migration): tx per migration, run DDL, record
  writer.go     emit an editable Go migration file (source) from a Migration
  errors.go     ErrNoChanges
conf/settings.go     + Database{Driver, DSN string} field (single default DB for the PoC)
apps/config.go       + ModelProvider optional interface { Models() []any }
application.go        build orm.Registry from app models, open DB by Driver, wire commands
makemigrations.go     manage command (pkg djangogo)
migrate.go            manage command (pkg djangogo)
```

### State (`state.go`)
```go
type FieldState struct {
	Name, Column string
	Kind         orm.Kind
	PrimaryKey, Null, Unique bool
	MaxLength    int
}
type ModelState struct {
	Name, Table string
	Fields      []FieldState // ordered
}
type ProjectState struct {
	Models map[string]*ModelState
	Order  []string // model names in creation order
}
func StateFromRegistry(r *orm.Registry) *ProjectState
func (ps *ProjectState) Clone() *ProjectState
func (ms *ModelState) fieldByName(name string) (*FieldState, bool)
```
`FieldState` is built from an `orm.Field` (all the fields above are exported on
`orm.Field`). To render DDL, migrations reconstruct `*orm.Field` values from
`FieldState` (orm.Field is a plain exported struct) and call `dialect.ColumnType` and
`dialect.Quote`.

### Operations (`operations.go`, `ddl.go`)
```go
type Operation interface {
	// Apply mutates ps to reflect this op (advances the project state).
	Apply(ps *ProjectState)
	// SQL returns the DDL statements for this op against the PRE-op state ps.
	SQL(d orm.Dialect, ps *ProjectState) ([]string, error)
	// Describe is a short human/string form (used by the Go-file writer + logs).
	Describe() string
}
```
Concrete ops and their SQLite DDL (computed from the PRE-op state):
- `CreateModel{Name, Table string, Fields []FieldState}` -> one `CREATE TABLE`.
- `DeleteModel{Name string}` -> `DROP TABLE`.
- `AddField{Model string, Field FieldState}` -> `ALTER TABLE <t> ADD COLUMN <coldef>`
  (a NOT NULL non-nullable add requires the field to be nullable or have a default;
  for the PoC, AddField requires `Null` true OR errors with a clear message).
- `RemoveField{Model, Field string}` -> SQLite temp-table rebuild (new table without
  the column, copy the remaining columns, drop, rename).
- `AlterField{Model string, Field FieldState}` -> SQLite temp-table rebuild (new table
  with the changed column def, copy all columns by name, drop, rename).
The temp-table rebuild emits four statements: `CREATE TABLE <t>__new (...)`,
`INSERT INTO <t>__new (<cols>) SELECT <cols> FROM <t>`, `DROP TABLE <t>`,
`ALTER TABLE <t>__new RENAME TO <t>`. `<cols>` is the set of columns common to old
and new. The runner executes these inside the migration's transaction.

### Autodetect (`autodetect.go`)
```go
func Diff(old, new *ProjectState) []Operation
```
Order: CreateModel (for models in new not old, in new.Order) first; then per model in
both, AddField then AlterField then RemoveField (field comparison by Name; "altered"
means Kind/Null/Unique/MaxLength/Column differ); then DeleteModel (models in old not
new) last. Deterministic ordering (follow Order slices / sorted names) so output is
stable and testable.

### Migration, replay, registry (`migration.go`, `registry.go`)
```go
type Migration struct {
	App          string
	Name         string   // "0001_initial", "0002_auto", ...
	Dependencies []string // prior migration names in the same app (linear)
	Operations   []Operation
}
// StateFromMigrations replays every op of migs (in order) onto an empty ProjectState.
func StateFromMigrations(migs []Migration) *ProjectState

type Registry struct { /* per-app ordered []Migration */ }
func NewRegistry() *Registry
func (r *Registry) Add(m Migration)
func (r *Registry) ForApp(app string) []Migration
func (r *Registry) All() []Migration
```

### Recorder + runner (`recorder.go`, `runner.go`)
- Tracking table `djangogo_migrations(id INTEGER PRIMARY KEY, app TEXT, name TEXT, applied_at DATETIME)`.
- `ensureTable(ctx, db)`; `appliedSet(ctx, db) (map[string]bool, error)` keyed `app+"/"+name`.
- `Apply(ctx, db *orm.DB, migs []Migration) (applied []string, err error)`: ensure
  table; for each migration not in the applied set (in dependency order), open a
  `tx`, run every op's SQL statements via the tx, insert the tracking row, commit;
  roll back on any error (and wrap with `%w`). Uses `db.SQL().BeginTx`.

### Writer (`writer.go`)
```go
func WriteMigration(dir string, m Migration) (path string, err error)
func RenderMigration(m Migration) (string, error) // Go source as a string
```
Emit an editable Go file (e.g. `<dir>/<app>/0001_initial.go`) that, when compiled,
reconstructs the same `Migration` value and registers it. For testability the renderer
returns the source string; a test asserts it `go/parser`-parses and contains the ops.

### makemigrations / migrate logic
- `makemigrations`: prior = `StateFromMigrations(reg.ForApp(app))`; current =
  `StateFromRegistry(ormRegistry)` (filtered per app once apps own models); `ops =
  Diff(prior, current)`; if none -> `ErrNoChanges` ("No changes detected"); else build
  a `Migration` (next number, dependency on the previous) and `WriteMigration`.
- `migrate`: `runner.Apply(ctx, db, reg.All())`; report applied names (or "no
  migrations to apply").

### Wiring
- `conf.Settings` gains `Database struct { Driver, DSN string }` (default Driver
  "sqlite", DSN "file::memory:?cache=shared" when Debug, else a file path; keep it
  simple, validated by Check only if non-empty).
- `apps.ModelProvider interface { Models() []any }` (optional, like `Initializer`).
- `Application`: after readying apps, collect `Models()` from every app, register each
  into a fresh `orm.Registry`, `Freeze()`, open the DB via the Driver (sqlite backend
  for M3), build an `orm.DB`, build a `migrations.Registry` (apps later provide their
  migration sets; for M3 an in-process registry is enough), and register the
  `makemigrations` and `migrate` commands. Expose `App.DB` and `App.Models`.

---

## Tasks (each: red, green, lint-clean, commit straight to main)
- [ ] **M3-A** orm `Registry.Models()` accessor + `migrations/state.go` (+ tests).
- [ ] **M3-B** `migrations/operations.go` + `ddl.go` (CreateModel/DeleteModel/AddField/
  RemoveField/AlterField with Apply + SQLite DDL incl. temp-table rebuild) (+ tests:
  assert emitted DDL strings and that they execute against in-memory sqlite).
- [ ] **M3-C** `migrations/autodetect.go` `Diff` (+ table-driven tests: new model,
  added/removed/altered field, deleted model, no-change).
- [ ] **M3-D** `migration.go` (Migration, StateFromMigrations replay) + `registry.go` +
  `recorder.go` + `runner.go` (+ integration test: apply migrations to in-memory sqlite,
  tracking table records them, re-apply is a no-op).
- [ ] **M3-E** `writer.go` (RenderMigration/WriteMigration) + `errors.go` + the
  makemigrations diff-and-build logic (+ tests: rendered source parses; round-trip
  diff produces the expected ops; ErrNoChanges when identical).
- [ ] **M3-F** wiring: `conf.Database`, `apps.ModelProvider`, `Application` model
  collection + DB open + command registration, `makemigrations.go` + `migrate.go`
  manage commands (+ tests).
- [ ] **M3-G** end-to-end exit-criterion integration test: a demo app with a model ->
  makemigrations writes a migration -> migrate builds the schema (table exists,
  CRUD works via the ORM) -> makemigrations again detects no changes. Then milestone
  review.
