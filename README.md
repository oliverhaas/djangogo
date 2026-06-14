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

This is a proof of concept: it implements a vertical slice of Django's developer
experience, not its full surface area. The notable slimmed or deferred areas are
below. Where a Django behaviour is matched closely the parity is called out so the
boundary is clear.

- **Relations:** only forward `FK[T]` is built, now carrying an `on_delete` action
  (`orm:"on_delete=cascade|set_null|restrict"`, default no action) that is emitted
  into the FK DDL on both dialects. One-to-one and many-to-many are still modelled
  with explicit link models, not dedicated `O2O[T]`/`M2M[T]` types. Note that
  `modernc.org/sqlite` does not enable the `foreign_keys` pragma, so `ON DELETE`
  (and FK constraints generally) are enforced on PostgreSQL but not on SQLite.
- **Querying:** `Filter`/`Exclude` support the `exact`, `gt`, `gte`, `lt`, `lte`,
  `contains`, `icontains`, `startswith`, `endswith`, `in`, and `isnull` lookups,
  plus `OrderBy`, `Limit`/`Offset`, `Count`, `SelectRelated` (FK join), and
  `Atomic` transactions with savepoints. Not implemented: aggregation/annotation,
  `Q`-object `OR` (predicates are combined with `AND`), `prefetch_related` for
  reverse/M2M sets, and `values()`/`values_list()` projection. `icontains` folds
  case via SQLite's ASCII-insensitive `LIKE`; on PostgreSQL it renders as a plain
  case-sensitive `LIKE` rather than `ILIKE`.
- **Pagination:** the query layer exposes `Limit`/`Offset`, but `ListView` and the
  admin changelist render every row -- there is no page-number paginator or UI.
- **Migrations:** generated migration files register via `init()`, so a separate
  `migrate` process only sees migrations compiled into the binary (rebuild after
  `makemigrations`); an in-process flow works without a rebuild. The autodetector
  has no `RenameField` operation: a renamed field is emitted as drop + add, which
  discards the column's data. `makemigrations` now prints a warning when it spots
  a likely rename (a removed field and an added field of the same type) so you can
  edit the migration to preserve the data. There are no data/`RunPython`-style
  migrations and no interactive prompts.
- **Admin:** `SearchFields`/`ListFilter`/inlines are not implemented (pagination is
  covered above). Foreign keys render as a `<select>` of the related rows (with
  Django's empty `---------` option), but there is no raw-id/autocomplete widget,
  so the select loads the whole related table. `ReadonlyFields` are omitted from
  the change form rather than rendered as disabled inputs.
- **Forms:** a field's `required` is derived from the database `null`, not a
  separate Django `blank` flag (the two are independent in Django). Rendered HTML
  input names use the Go struct field name (e.g. `name="Title"`), not Django's
  lowercase field name.
- **Templates / URLs:** `{% url %}` takes positional arguments only (no `pk=...`
  kwargs or `as var` capture), and `Reverse` substitutes them without
  URL-escaping. The template model context exposes scalar columns under their
  snake_case names and `{{ obj }}` as `__str__`, but relation fields
  (`{{ obj.author }}` / `{{ obj.author_id }}`) are not exposed.
- **ORM fields:** a declared `default=` overrides an explicitly-set Go zero value
  (Django keeps an explicit zero), and `Update` stamps `auto_now` fields whereas
  Django's `QuerySet.update()` does not (this ORM has no separate `save()` path).
- **Auth:** users, groups, and group/user permissions are modelled, and `is_active`
  is enforced the way Django's `ModelBackend` does -- a deactivated account holds
  no permissions (not even a superuser) and is treated as anonymous when resolved
  from the session. Not covered: per-object permissions and automatic permission
  creation from models. `createsuperuser` prompts once (no re-prompt on an empty or
  duplicate username), runs no password-strength validation, and reads input
  line-based (unmasked).
- **Sessions/CSRF:** unsafe requests are checked for a valid CSRF token and, like
  Django, for an `Origin` header matching the host (falling back to a strict
  `Referer` check over HTTPS). The session cookie sets `HttpOnly` and
  `SameSite=Lax` but not `Secure`; enable `Secure` behind TLS in a real deployment.
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
