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
| `contenttypes`    | Content types of the installed models.                                     |
| `messages`        | One-time messages for the next page, stored in the session.                |
| `middleware`      | Host validation, security headers, X-Frame-Options, gzip, error pages.     |
| `staticfiles`     | Static file finders, `collectstatic`, `findstatic` and debug serving.      |
| `checks`          | The system check framework behind the `check` command.                     |
| `djangotest`      | Django's test client, assertions and per-test databases.                   |
| `fidelity`        | The Django-as-oracle template fidelity harness.                            |

## Django fidelity

`fidelity/` holds a Django-as-oracle differential harness. Canonical
template+context cases are rendered by Django's DTL (see `fidelity/oracle`) into
committed golden files; the Go test renders the same cases with the project's
pongo2 engine and asserts byte-equality. Twelve cases are byte-identical to
Django 6.0.3. The one documented cosmetic divergence (apostrophe escaping) is
skipped with a reason; divergences are catalogued in `fidelity/divergences.md`.

## Known limitations

Djan-Go-Go covers a large part of Django's developer experience, but not all of
it. The known gaps are below. The template engine's divergences from Django are
listed one by one in [`fidelity/divergences.md`](fidelity/divergences.md).

- **Model fields:** there are no float, decimal, date, duration, file or JSON
  field kinds, and no one-to-one or many-to-many fields: model these relations
  with explicit link models. A foreign key can only target an integer primary
  key. A nullable integer field reads NULL as 0 in a struct (`Values` returns
  nil), and `Create` stores 0 for an unset one. A declared `default=` replaces an
  explicit Go zero value, where Django keeps the explicit value. A Go field name
  that ends in digits snake-cases without an underscore (`Parent1` becomes
  `parent1`).
- **Model options:** there is no `Meta` API yet, so no model-level indexes,
  `unique_together`, default ordering, custom managers, model validators or
  `full_clean`. The migration operations for indexes and unique-together sets
  exist, but `makemigrations` does not read them from the models.
- **Database:** one database per application. SQLite does not enforce foreign
  keys, because `modernc.org/sqlite` does not enable the `foreign_keys` pragma;
  PostgreSQL enforces them. Two models with foreign keys to each other cannot be
  created on PostgreSQL.
- **Querying:** no transforms (`__date`, `Length`), no `extra()`, no fetch modes
  and no `select_related()` without arguments. `Raw` leaves the fields missing
  from a result at their zero value, where Django defers and loads them.
  Transactions have no `savepoint=False` or `set_rollback`, so on PostgreSQL a
  failed statement inside `Atomic` aborts the transaction until the block ends.
  `Pow` returns a float.
- **Migrations:** generated migration files register through `init()`. A
  separate `migrate` process sees them only after each `migrations/<app>` package
  is blank-imported in the main package and the binary is rebuilt;
  `makemigrations` prints the import line. `makemigrations` cannot write `RunGo`
  operations: add them by hand, as with Django's `RunPython`. There is no
  squashing and no `makemigrations --merge`: conflicts are reported only. A change
  to the type of a primary key does not change the foreign keys that reference
  it, and PostgreSQL rejects it.
- **Templates:** the engine is built on pongo2. Missing tags: `regroup`,
  `querystring`, `debug`, `partialdef`/`partial`, and the `i18n`, `l10n`, `tz` and
  `cache` libraries. Missing filter: `pprint`. Output is English only. Lists and
  maps print in Go syntax, not as Python's repr. `{% extends %}` takes only a
  literal name, and `{% lorem %}` is pongo2's tag.
- **Forms:** no formsets, file fields, option groups, `MultiValueField` or
  `SplitDateTimeField`, no localization and no custom renderers. A template
  cannot loop over a form with `{% for field in form %}`: use `form.fields`.
  `ModelChoiceField` has no `to_field_name` or `label_from_instance`, and it loads
  its options once per form, where Django loads them on each render.
- **Admin:** no search (`SearchFields` is stored but not used), list filters,
  inlines or changelist pagination. A foreign key renders as a `<select>` of the
  whole related table, with no raw-id or autocomplete widget. `ReadonlyFields`
  are left out of the change form instead of shown as read-only values.
- **Auth:** no custom user model. The login, logout and password change views
  are mounted by hand: there is no counterpart of `django.contrib.auth.urls`. No
  `LoginRequiredMiddleware`. A user row that Django wrote with a NULL
  `last_login` cannot be loaded yet. In templates, `perms` does not grant an
  active superuser the permissions that are missing from the database, and
  `user` has no related objects such as groups. `LoginRequired` escapes the
  slashes of `next` (`%2F`), where Django keeps them.
- **Sessions and messages:** no cache, cache_db or file session stores. The
  database store needs a database and the installed `sessions` app; without
  them, sessions use signed cookies. Messages use the session only: no cookie or
  fallback storage and no safe messages. A message added after the response
  headers are written is lost, and reading `messages` in a template, also in
  `{% if messages %}`, marks them as read.
- **contenttypes:** no `GenericForeignKey` and no removal of stale content types.
- **Commands:** `runserver` has no autoreload, and `shell` is a query shell over
  the models, not a Go REPL. `collectstatic` has no `--link` and no
  post-processing (manifest or hashed names). `startproject` and `startapp` have
  no `--template`, `--extension`, `--name` or `--exclude`. `createsuperuser`
  does not suggest the system user name as the default, and it reads answers line
  by line when stdin is not a terminal.
- **Fidelity:** the differential harness compares template output only. The
  query SQL and the migration SQL are not compared with Django's.

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

### The `djangotest` package

`djangotest` is the counterpart of Django's `django.test`. It offers:

- a stateful test client that drives an `http.Handler` in process, with cookies,
  sessions, login, redirects, and CSRF checks;
- Django's assertions as plain functions, with Django's failure messages;
- query capture (`AssertNumQueries`, `CaptureQueries`);
- private test databases on SQLite and PostgreSQL.

A test that drives a view:

```go
func TestHome(t *testing.T) {
	c := djangotest.NewClient(t, handler)
	resp := c.Get("/")

	djangotest.AssertContains(t, resp, "Welcome")
	djangotest.AssertTemplateUsed(t, resp, "home.html")
}
```

A test that runs on every configured database:

```go
func TestCreatePost(t *testing.T) {
	djangotest.EachDB(t, []any{&Post{}}, func(t *testing.T, db *orm.DB) {
		err := orm.Query[Post](db).Create(context.Background(), &Post{Title: "Hello"})
		djangotest.AssertEqual(t, err, nil)

		n, _ := orm.Query[Post](db).Count(context.Background())
		djangotest.AssertEqual(t, n, int64(1))
	})
}
```

`EachDB` always runs on SQLite. It also runs on PostgreSQL when
`DJANGOGO_TEST_POSTGRES_DSN` is set. Each run gets its own database or schema,
so tests can run in parallel.

Django's test methods become plain Go test functions:

| Django                               | Go                                          |
| ------------------------------------ | ------------------------------------------- |
| `self.client.get("/x/")`             | `c.Get("/x/")`                              |
| `self.assertContains(response, "x")` | `djangotest.AssertContains(t, resp, "x")`   |
| `self.assertNumQueries(2, fn)`       | `djangotest.AssertNumQueries(t, db, 2, fn)` |

[`djangotest/doc.go`](djangotest/doc.go) has the full mapping and the
differences from Django.

Features are developed test first. Where Django has a test for the behavior,
that test is ported, and its doc comment cites the source in this form:

```go
// Ported from Django: tests/<file>.py::<Class>.<test>
```

Ported tests live in external test packages, in files named
`django_<area>_test.go`.

## License

[MIT](LICENSE)

The ported tests in `django_*_test.go` files derive from Django's test suite,
which uses the BSD 3-Clause license. See [LICENSE-DJANGO](LICENSE-DJANGO).
