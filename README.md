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

Proof-of-concept. The milestones below are implemented. The public APIs are
unstable and can change between commits.

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
- **M8 Django fidelity.** `createsuperuser`, snake_case template field access, a
  Django-style `ModelForm`, and the `auto_now`, `default=` and `choices=` tags.
- **M9 spec completion.** Django's URL dispatch, auth tables and password
  hashers, sessions, CSRF, messages, generic views and commands; Django's field
  kinds, `orm.Meta`, model validation, reverse, one-to-one and many-to-many
  relations with `PrefetchRelated`; the admin's URL layout, permissions, changelist and actions; and
  SQL fidelity tests for queries and migrations.

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
| `validators`      | Django's validators and `ValidationError`, shared by models and forms.     |
| `admin`           | The admin site: changelist, actions, add/change/delete, history, permissions. |
| `scaffold`        | Project and app scaffolding used by `startproject`/`startapp`.             |
| `contenttypes`    | Content types of the installed models.                                     |
| `messages`        | One-time messages for the next page, stored in the session.                |
| `middleware`      | Host validation, security headers, X-Frame-Options, gzip, error pages.     |
| `staticfiles`     | Static file finders, `collectstatic`, `findstatic` and debug serving.      |
| `checks`          | The system check framework behind the `check` command.                     |
| `djangotest`      | Django's test client, assertions and per-test databases.                   |
| `fidelity`        | Django-as-oracle harnesses for templates, query SQL and migration SQL.     |

## Django fidelity

`fidelity/` holds Django-as-oracle differential harnesses. Django 6.0.3
writes committed golden files (see `fidelity/oracle`), and the Go tests compare
against them without Python:

- **Templates:** canonical template and context cases render with the pongo2
  engine and must match Django's DTL output byte for byte.
- **Query SQL:** canonical ORM queries run on SQLite and PostgreSQL. The tests
  compare the normalized SQL and parameters with Django's compiler and the
  results on a fixture dataset.
- **Migration SQL:** canonical operations compile on both dialects and are
  compared with Django's `sqlmigrate`. The tests also check the resulting SQLite
  schema and run every case on a live PostgreSQL server.

`go generate ./fidelity/...` regenerates all golden files. Known differences are
skipped with a reason and listed in `fidelity/divergences.md`,
`fidelity/query_sql_divergences.md` and `fidelity/migration_sql_divergences.md`.

## Known limitations

Djan-Go-Go covers a large part of Django's developer experience, but not all of
it. The known gaps are below. The template engine's divergences from Django are
listed one by one in [`fidelity/divergences.md`](fidelity/divergences.md).

- **Model fields:** no file, image or generated fields and no composite
  primary keys. A foreign key, one-to-one or many-to-many field can only target
  an integer primary key, and there is no `to_field`. Use a pointer field
  (`*int64`) for a nullable column: a plain field reads NULL as its zero value.
  A declared `default=` replaces an explicit Go zero value, where Django keeps
  the explicit value. A Go field name that ends in digits snake-cases without an
  underscore (`Parent1` becomes `parent1`).
- **Model options:** `orm.Meta` has `ordering`, `unique_together`, `indexes`,
  verbose names, `db_table` and per-field choices, validators and error
  messages. There is no `Meta.constraints`, no `unique_for_date`, no
  `limit_choices_to`, no custom managers and no abstract, proxy or multi-table
  inheritance.
- **Database:** one database per application. Two models with foreign keys to
  each other cannot be created on PostgreSQL.
- **Querying:** of Django's transforms only the date and time ones exist
  (`__year`, `__date`, `__hour` and the others), and they read a datetime in
  UTC. There are no text transforms or functions such as `Lower` and `Length`,
  no `extra()`, no fetch modes and no `select_related()` without arguments. A model keeps prefetched
  reverse and many-to-many sets only when it embeds `orm.RelCache`, and the
  queryset of an `orm.Prefetch` cannot be sliced. `Raw` leaves the fields missing
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
  literal name, and `{% lorem %}` is pongo2's tag. A template never runs a
  query: a relation renders empty unless the view loaded it with
  `SelectRelated` or `PrefetchRelated`.
- **Forms:** no file fields or option groups, no localization and no custom
  renderers. A model formset has no to_field relations. A template
  cannot loop over a form with `{% for field in form %}`: use `form.fields`.
  `ModelChoiceField` and `ModelMultipleChoiceField` have no `to_field_name` or
  `label_from_instance`, and they load their options once per form, where
  Django loads them on each render.
- **Admin:** no `list_editable`, `date_hierarchy` or facets and no "View on
  site" links. A registered model needs an integer primary key. An
  inline cannot edit the auto-created through model of a many-to-many field,
  there are no generic inlines, and an inline has no custom formset class.
  There are no `radio_fields`, and `FormOptions` sets widgets per field name,
  not per model field class as `formfield_overrides` does. The scripts and
  pages are in English only. There is no custom form class: `FormOptions`
  adds fields and options to the model form, and its exclusions add to
  `ExcludeFields` instead of being overridden by it. A read-only field shows
  a model field or a `DisplayColumns` value, not a model method. The system
  checks cover `AutocompleteFields`, `RawIDFields`, `Fields`, `Fieldsets`,
  `ExcludeFields`, `FilterHorizontal`, `FilterVertical`, `ReadonlyFields`,
  `Inlines` and `ListDisplay` only. A search field with `@` uses
  `icontains`, not full-text search. The views take no `extra_context`, and
  a template option such as `ChangeFormTemplate` names one template, not a
  list. Blocks that Django names with hyphens, such as `object-tools-items`,
  have underscores.
- **Auth:** no custom user model. The login, logout and password change views
  are mounted by hand: there is no counterpart of `django.contrib.auth.urls`. No
  `LoginRequiredMiddleware`. In templates, `perms` does not grant an
  active superuser the permissions that are missing from the database, and
  `user` has no related objects such as groups. `LoginRequired` escapes the
  slashes of `next` (`%2F`), where Django keeps them.
- **Sessions and messages:** no cache, cache_db or file session stores. The
  database store needs a database and the installed `sessions` app; without
  them, sessions use signed cookies. Messages use the session only: no cookie or
  fallback storage. A message added after the response
  headers are written is lost, and reading `messages` in a template, also in
  `{% if messages %}`, marks them as read.
- **contenttypes:** no `GenericForeignKey` and no removal of stale content types.
- **Commands:** `runserver` has no autoreload, and `shell` is a query shell over
  the models, not a Go REPL. `collectstatic` has no `--link` and no
  post-processing (manifest or hashed names). `startproject` and `startapp` have
  no `--template`, `--extension`, `--name` or `--exclude`. `createsuperuser`
  does not suggest the system user name as the default, and it reads answers line
  by line when stdin is not a terminal.

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
