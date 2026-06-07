# Djan-Go-Go -- Milestone 2: ORM Core

> Detailed plan for M2 (ORM core), the metadata keystone every later subsystem reads.
> Conventions: TDD (red, green, commit), tests beside code in the same package, one
> conventional commit per task, `export PATH="$HOME/.local/go/bin:$PATH"` for Go,
> `gofmt`/`go vet`/`golangci-lint` clean (golangci-lint at `~/go/bin`).

**Goal:** Define a model as a plain Go struct plus `orm:"..."` tags, reflect it once
at boot into an immutable `*orm.Model`, and do CRUD against SQLite through a lazy,
clone-per-method generic `QuerySet[T]`.

**Exit criteria:** Register a model, open an in-memory SQLite DB, create the schema,
and have `Create`/`Get`/`Filter`/`All` round-trip; `go test ./...`, `go vet`,
`golangci-lint` all green; no import cycles.

**Driver:** `modernc.org/sqlite` (pure Go, no cgo, so a single static binary). The
in-memory test DSN is `file::memory:?cache=shared` with `SetMaxOpenConns(1)` so the
pool stays on one in-memory database.

---

## Architecture

Import direction (no cycles): `orm` imports only stdlib. `orm/backends/sqlite`
imports `orm` plus the driver. The root/app code wires a backend into `orm.NewDB`.
`orm` never imports a backend; a backend is injected as a `Dialect`.

```
orm/
  field.go        Kind enum, Field struct, struct-tag plus Go-type inference
  model.go        immutable *Model (ordered fields, PK, column maps, table, goType)
  registry.go     reflect-at-startup Registry: Register(&T{}) builds a frozen *Model
  meta.go         optional Meta() hook (table name; later: ordering, indexes)
  errors.go       sentinels: ErrDoesNotExist, ErrMultipleObjectsReturned
  dialect.go      Dialect interface (placeholder, quoting, column types, CREATE TABLE)
  lookups.go      __-lookup parsing into a SQL predicate plus args
  queryset.go     generic QuerySet[T]: clone-per-method chain plus terminals
  compiler.go     compile QuerySet into SELECT/UPDATE/DELETE SQL plus args
  db.go           DB (wraps *sql.DB plus Dialect plus *Registry); Query[T]
  execute.go      reflection scanning plus All/Get/Count/Exists/Create/Update/Delete
  backends/sqlite/
    sqlite.go     Dialect impl plus Open helper
```

### Field (`field.go`)

```go
type Kind uint8
const (
    KindAuto     Kind = iota // integer PK, autoincrement
    KindInt                  // int / int64 / int32
    KindChar                 // string, VARCHAR(MaxLength) (default for string)
    KindText                 // string, TEXT (tag type=text)
    KindBool                 // bool
    KindDateTime             // time.Time
)

type Field struct {
    Name       string // Go field name, e.g. "Title"
    Column     string // db column, e.g. "title"
    Kind       Kind
    PrimaryKey bool
    Null       bool
    Unique     bool
    MaxLength  int    // Char only; default 255
    Index      int    // reflect struct-field index
}
```

Inference: `string` becomes KindChar (or KindText with `type=text`); `int*` becomes
KindInt; `bool` becomes KindBool; `time.Time` becomes KindDateTime. PK: a field
tagged `pk`, else a field named `ID` of integer type becomes a KindAuto PK. Tag
grammar (`;`-separated): `pk`, `column=<name>`, `max_length=<n>`, `null`, `unique`,
`type=text`, `-` (skip). Unknown options are an error. The default column is
`strings.ToLower(Name)`.

### Model plus Registry (`model.go`, `registry.go`)

```go
type Model struct { /* unexported fields */ }
func (m *Model) Name() string
func (m *Model) Table() string
func (m *Model) Fields() []*Field          // ordered, copy-safe
func (m *Model) PrimaryKey() *Field
func (m *Model) FieldByName(name string) (*Field, bool)
func (m *Model) Columns() []string         // db column names, field order
func (m *Model) GoType() reflect.Type

type Registry struct { /* ... */ }
func NewRegistry() *Registry
func (r *Registry) Register(model any) (*Model, error) // &T{} pointer-to-struct
func (r *Registry) Freeze()
func (r *Registry) Get(name string) (*Model, bool)
func (r *Registry) ModelOf(v any) (*Model, bool)       // by reflect.Type of T
```

Register reflects the struct, builds ordered `[]*Field`, resolves the table (Meta
override or lowercased struct name), requires exactly one PK, builds name/column
maps, and returns a `*Model`. Errors: not a pointer-to-struct, duplicate model, no
PK, multiple PKs, register-after-freeze. `Freeze` makes later `Register` calls error
(the immutability property).

### Meta hook (`meta.go`)

```go
type Meta struct { Table string }
type withMeta interface { Meta() Meta }
```

### Dialect (`dialect.go`) plus SQLite (`backends/sqlite/sqlite.go`)

```go
type Dialect interface {
    Name() string
    Placeholder(n int) string     // sqlite "?"; pg "$N"
    Quote(ident string) string    // `"ident"`
    ColumnType(f *Field) string   // DDL type fragment incl. PK/NULL/UNIQUE
    CreateTableSQL(m *Model) string
    SupportsReturning() bool       // sqlite: false
}
```

SQLite column types: KindAuto is `INTEGER PRIMARY KEY AUTOINCREMENT`, KindInt is
`INTEGER`, KindChar is `VARCHAR(n)`, KindText is `TEXT`, KindBool is `BOOLEAN`,
KindDateTime is `DATETIME`. Emit `NOT NULL` unless `Null`; emit `UNIQUE` when set (a
PK is already unique).

### Lookups (`lookups.go`)

`field__lookup` becomes a predicate. Supported: `exact` (default), `gt`, `gte`,
`lt`, `lte`, `contains`, `icontains`, `startswith`, `endswith`, `in`, `isnull`.
`in` takes a slice (expands to `col IN (?,?,...)`); `isnull` takes a bool
(`IS NULL`/`IS NOT NULL`); `contains`/`startswith`/`endswith` use `LIKE` with escaped
`%`/`_`; the `i*` variants use `LIKE` (sqlite `LIKE` is case-insensitive for ASCII by
default). An unknown lookup or unknown field is an error.

### QuerySet (`queryset.go`) plus compiler (`compiler.go`)

```go
type QuerySet[T any] struct { /* db, model, wheres, excludes, order, limit, offset */ }

func Query[T any](db *DB) *QuerySet[T] // resolves *Model for T from db.Registry

// chainable (each returns a fresh clone; the receiver is never mutated)
func (q *QuerySet[T]) Filter(pairs ...any) *QuerySet[T]  // "lookup", val, ... (even arity)
func (q *QuerySet[T]) Exclude(pairs ...any) *QuerySet[T]
func (q *QuerySet[T]) OrderBy(fields ...string) *QuerySet[T] // "-field" is DESC
func (q *QuerySet[T]) Limit(n int) *QuerySet[T]
func (q *QuerySet[T]) Offset(n int) *QuerySet[T]

// terminal
func (q *QuerySet[T]) All(ctx context.Context) ([]T, error)
func (q *QuerySet[T]) Get(ctx context.Context, pairs ...any) (T, error)
func (q *QuerySet[T]) Count(ctx context.Context) (int64, error)
func (q *QuerySet[T]) Exists(ctx context.Context) (bool, error)
func (q *QuerySet[T]) Create(ctx context.Context, obj *T) error
func (q *QuerySet[T]) Update(ctx context.Context, assignments ...any) (int64, error)
func (q *QuerySet[T]) Delete(ctx context.Context) (int64, error)
```

`Filter`/`Exclude`/`Get` take even-arity `("field__lookup", value)` pairs (AND-ed;
multiple `Filter` calls also AND). Odd arity or a non-string key is a build error
stored on the queryset and returned by the terminal. The `compiler` builds
`SELECT <cols> FROM <table> [WHERE ...] [ORDER BY ...] [LIMIT ? OFFSET ?]` plus the
arg list using `Dialect.Placeholder`. Clone-per-method: each chain method deep-copies
the slices so the parent queryset is never mutated.

### Execute (`execute.go`) plus DB (`db.go`)

```go
type DB struct { /* sqlDB *sql.DB; dialect Dialect; registry *Registry */ }
func NewDB(sqlDB *sql.DB, d Dialect, r *Registry) *DB
func (db *DB) Dialect() Dialect
func (db *DB) Registry() *Registry
func (db *DB) CreateTable(ctx context.Context, m *Model) error
```

Scanning: `dest := reflect.New(model.GoType())`; for each selected column build a
pointer into the matching struct field; `rows.Scan(ptrs...)`; append
`dest.Elem().Interface().(T)`. `Get` returns `ErrDoesNotExist` on 0 rows and
`ErrMultipleObjectsReturned` on more than 1. `Create` inserts non-auto columns, reads
`LastInsertId`, and writes the new PK back onto `*obj`. `Update`/`Delete` compile to
UPDATE/DELETE and return rows-affected.

---

## Tasks

Each task: write the failing test, run red, implement, run green, then
`gofmt`/`go vet`/lint clean, then commit. Integration tasks run against in-memory
SQLite.

- [ ] **Task 1 -- `field.go`**: `Kind`, `Field`, and `parseStructField` (tag plus
  type inference, skip `-`, unknown-option error). Unit tests over many struct fields.
- [ ] **Task 2 -- `meta.go` plus `model.go` plus `registry.go`**: immutable `*Model`,
  reflection-based `Registry.Register`, Meta() table override, PK rules, and the
  duplicate / no-PK / not-a-struct / freeze errors. Unit tests.
- [ ] **Task 3 -- `errors.go` plus `dialect.go` plus `backends/sqlite/sqlite.go`**:
  `Dialect` interface, the sqlite dialect (`Placeholder`/`Quote`/`ColumnType`/
  `CreateTableSQL`/`SupportsReturning`), and the sentinels. Unit-test the generated
  DDL and actually `CREATE TABLE` in an in-memory db.
- [ ] **Task 4 -- `lookups.go`**: parse `field__lookup` against a `*Model` and emit
  predicate SQL plus args via a `Dialect`. Table-driven unit tests with a stub dialect
  (cover every supported lookup, `in`, `isnull`, and unknown field/lookup errors).
- [ ] **Task 5 -- `queryset.go` plus `compiler.go`**: generic `QuerySet[T]`,
  clone-per-method chain, `Query[T]`, and SELECT/UPDATE/DELETE compilation to SQL plus
  args. Unit tests assert generated SQL strings/args with a stub dialect and verify
  the parent queryset is never mutated by a chained call.
- [ ] **Task 6 -- `db.go` plus `execute.go` read path**: `DB`, `CreateTable`,
  reflection scanning, and `All`/`Get`/`Count`/`Exists`. Integration test vs in-memory
  sqlite (seed rows with raw SQL, read them back through the QuerySet; assert
  `ErrDoesNotExist` and `ErrMultipleObjectsReturned`).
- [ ] **Task 7 -- `execute.go` write path**: `Create` (insert plus write-back PK),
  `Update`, and `Delete`. Integration test vs in-memory sqlite.
- [ ] **Task 8 -- end-to-end round-trip**: a single integration test that registers a
  model, opens sqlite, creates the schema, and exercises Create, Get, Filter, All,
  Update, Delete. This is the milestone exit criterion. Wire a small
  `sqlite.Open(dsn)` helper returning a ready `*orm.DB`.
