# Djan-Go-Go

[![Go Reference](https://pkg.go.dev/badge/github.com/oliverhaas/djangogo.svg)](https://pkg.go.dev/github.com/oliverhaas/djangogo)
[![CI](https://github.com/oliverhaas/djangogo/actions/workflows/ci.yml/badge.svg)](https://github.com/oliverhaas/djangogo/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/oliverhaas/djangogo)](https://goreportcard.com/report/github.com/oliverhaas/djangogo)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

Djan-Go-Go ports the Django developer experience to Go. You declare a model as a
Go struct with `orm` tags; at startup the framework reflects over the registered
structs to build model metadata, and that one source of metadata drives the ORM,
migrations, admin, and forms. It compiles to a single static binary with no
runtime Python and supports SQLite and PostgreSQL.

## Status

Proof-of-concept. The seven milestones below are implemented. The public APIs
are unstable and may change between commits.

- **M1 Spine.** Settings, the app registry, and a `manage`-style command
  dispatcher with a runserver.
- **M2 ORM core.** Struct-tag model metadata, a registry, and a lazy generic
  `Query[T]` query builder (Filter, Exclude, OrderBy, Limit, Get, All, Count,
  Create, Update, Delete).
- **M3 Migrations.** State autodetection, generated migration files, a runner,
  and a `migrations` recorder, wired to `makemigrations` and `migrate`.
- **M4 relations + PostgreSQL.** `FK[T]` foreign keys, relation resolution,
  `SelectRelated` joins, signals, transactions, and a PostgreSQL backend
  alongside SQLite.
- **M5 web layer.** A `urls` router with named-route reverse, a pongo2 template
  engine with Django-style tags, and generic `ListView`/`DetailView`.
- **M6 forms + admin.** Form fields and widgets, `ModelForm` from model
  metadata, sessions, CSRF, password hashing and auth, and a staff-gated admin
  site with add/change/delete views.
- **M7 fidelity + scaffolding.** A Django-as-oracle template fidelity harness,
  project/app scaffolding, and this documentation plus a runnable example.

## Quickstart

Declare a model. An integer field named `ID` is auto-promoted to the primary
key; `orm` tags set column types and constraints.

```go
type Post struct {
	ID        int64
	Title     string `orm:"max_length=200"`
	Body      string `orm:"type=text"`
	Published bool
	CreatedAt time.Time
}
```

Register the model, resolve relations, freeze the registry, and open a database:

```go
reg := orm.NewRegistry()
reg.Register(&Post{})
reg.Resolve()
reg.Freeze()

sdb, _ := sqlite.Open("blog.sqlite3")
db := orm.NewDB(sdb, sqlite.New(), reg)
```

Generate and apply migrations from the CLI built on `djangogo.New`:

```console
go run . makemigrations
go run . migrate
```

Query with the generic `Query[T]` builder:

```go
posts, _ := orm.Query[Post](db).
	Filter("published", true).
	OrderBy("-created_at").
	All(ctx)

post, _ := orm.Query[Post](db).Get(ctx, "id", 1)
```

Register the model in the admin site:

```go
site, _ := admin.NewAdminSite(db)
admin.Register[Post](site, admin.ModelAdmin{
	ListDisplay: []string{"ID", "Title", "Published"},
	Ordering:    []string{"-id"},
})
router := urls.NewRouter(site.Routes()...)
```

See [`examples/blog`](examples/blog) for a full application that wires the public
list and detail pages and the admin together; run it with `go run ./examples/blog`.

## Package map

| Package           | Responsibility                                                              |
| ----------------- | -------------------------------------------------------------------------- |
| `conf`            | Typed application settings and boot-time validation.                       |
| `apps`            | The app registry and the `Config`/`ModelProvider` app contracts.           |
| `manage`          | The CLI command dispatcher (the `manage`-style entry point).               |
| `orm`             | Model metadata, the registry, and the generic `Query[T]` builder.          |
| `orm/backends`    | Dialect implementations: `sqlite` and `postgres`.                          |
| `migrations`      | Autodetection, generated migration files, the runner, and the recorder.    |
| `auth`            | Users, groups, permissions, password hashing, and auth middleware.         |
| `sessions`        | Per-request sessions with signed-cookie and database-backed stores.        |
| `csrf`            | CSRF token middleware.                                                      |
| `urls`            | Route declaration, a `ServeMux` router, and named-route reverse.           |
| `views`           | Request/response helpers and generic `ListView`/`DetailView`.              |
| `templates`       | A pongo2 engine with `{% static %}`, `{% url %}`, `{% csrf_token %}` tags. |
| `forms`           | Form fields, widgets, and `ModelForm` derived from model metadata.         |
| `admin`           | The staff-gated admin site with add/change/delete views.                   |
| `scaffold`        | Project and app scaffolding used by `startproject`/`startapp`.             |
| `fidelity`        | The Django-as-oracle template fidelity harness.                            |

## Django fidelity

`fidelity/` holds a Django-as-oracle differential harness. Canonical
template+context cases are rendered by Django's DTL (see `fidelity/oracle`) into
committed golden files; the Go test renders the same cases with the project's
pongo2 engine and asserts byte-equality. Twelve cases are byte-identical to
Django 6.0.3. The one documented cosmetic divergence (apostrophe escaping) is
skipped with a reason; divergences are catalogued in `fidelity/divergences.md`.

## Known limitations

This is a proof of concept. The notable slimmed or deferred areas are:

- **Relations:** only forward `FK[T]` is built. One-to-one and many-to-many are
  represented with explicit link models, not dedicated `O2O[T]`/`M2M[T]` types.
- **Migrations:** generated migration files register via `init()`, so a separate
  `migrate` process only sees migrations compiled into the binary (rebuild after
  `makemigrations`). An in-process flow works without a rebuild.
- **Admin:** `SearchFields`/`ListFilter`/pagination/inlines and FK chooser widgets
  are not implemented. `ReadonlyFields` are currently omitted from the change form
  rather than rendered as disabled inputs.
- **Sessions/CSRF:** the session cookie sets `HttpOnly` and `SameSite=Lax` but not
  `Secure`; enable `Secure` behind TLS in a real deployment.
- **Fidelity:** the differential harness covers template rendering only. Query-SQL
  and migration-SQL differential comparison are future work.

## Testing

```console
go test ./...
```

PostgreSQL integration tests are skipped unless `DJANGOGO_TEST_POSTGRES_DSN`
points at a reachable database:

```console
DJANGOGO_TEST_POSTGRES_DSN="postgres://user:pass@localhost:5432/djangogo?sslmode=disable" go test ./...
```

The `Makefile` provides `make` (fmt + vet + test), `make test-race`,
`make cover`, and `make lint`. Requires Go 1.26+.

## License

[MIT](LICENSE)
