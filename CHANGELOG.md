# Changelog

All notable changes to this project are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project aims
to follow [Semantic Versioning](https://semver.org/) once it reaches a release.

## [Unreleased]

### Added

- Initial repository scaffold: module, CI, linting, CLI placeholder, and the
  build roadmap in `PLAN.md`.
- M1 Spine: typed settings (`conf`), the app registry (`apps`), and the
  `manage`-style command dispatcher with a runserver.
- M2 ORM core: struct-tag model metadata, the model registry, and the lazy
  generic `Query[T]` builder (Filter, Exclude, OrderBy, Limit, Get, All, Count,
  Create, Update, Delete).
- M3 Migrations: state autodetection, generated migration files, the runner, and
  the recorder, wired to `makemigrations` and `migrate`.
- M4 relations and PostgreSQL: `FK[T]` foreign keys, relation resolution,
  `SelectRelated` joins, ORM signals, transactions, and the PostgreSQL backend
  alongside SQLite.
- M5 web layer: the `urls` router with named-route reverse, the pongo2 template
  engine with `{% static %}`, `{% url %}`, and `{% csrf_token %}` tags, and the
  generic `ListView`/`DetailView`.
- M6 forms and admin: form fields and widgets, `ModelForm` from model metadata,
  sessions, CSRF middleware, password hashing and auth, and the staff-gated
  admin site with add/change/delete views.
- M7 fidelity and scaffolding: the Django-as-oracle template fidelity harness
  (12 cases byte-identical to Django 6.0.3), project/app scaffolding, an expanded
  README, and a runnable blog example under `examples/blog`.
- M8 Django fidelity: the `createsuperuser` command; snake_case template field
  access and the `__str__` label hook used by the admin, templates, and FK
  `<select>` options; end-to-end `{% url %}` reverse resolution; a real
  Django-style `ModelForm` (`FromModel`/`FromStruct` with field options and
  foreign keys rendered as a `<select>`); and the `auto_now`/`auto_now_add`,
  `default=`, and `choices=` model-field tags. `runserver` now mounts each app's
  `URLs()` and the admin, the scaffold generates a runnable admin/URLs/`__str__`
  project, and the blog example gains a `Comment` model with a working comment
  form.

- FK `on_delete` actions: `orm:"on_delete=cascade|set_null|restrict"` (default no
  action) parsed from the struct tag and emitted into the FK DDL on both the
  SQLite and PostgreSQL backends and through the migrations state/DDL/writer.
  `set_null` requires a nullable column, as Django enforces. Enforced on
  PostgreSQL; SQLite does not check foreign keys without the `foreign_keys` pragma.
- `makemigrations` warns on a likely field rename (a removed field and an added
  field of the same type), which the autodetector would otherwise emit as a
  drop + add that discards the column's data without notice.
- `djangotest`, the counterpart of `django.test`: you can now write Django-style
  tests in Go, with a stateful client, Django's assertions, and private test
  databases on SQLite and PostgreSQL. `NewClient` drives an `http.Handler` in
  process with cookies, sessions, login, redirects, and CSRF checks. The
  assertions (`AssertContains`, `AssertRedirects`, `AssertTemplateUsed`,
  `AssertNumQueries`, and others) fail with Django's messages. `NewDB` and
  `EachDB` give each test its own database or schema, and `EachDB` also runs on
  PostgreSQL when `DJANGOGO_TEST_POSTGRES_DSN` is set. Tests ported from
  Django's suite carry a `// Ported from Django: tests/<file>.py::<Class>.<test>`
  comment, and `LICENSE-DJANGO` holds Django's BSD 3-Clause license for them.
- ORM queries: `Filter`, `Exclude`, `OrderBy` and `Get` accept Django attribute
  names and `pk`, follow relations with `__`, and report an unknown name with
  Django's "Cannot resolve keyword" message. New: `Q` objects (OR, AND, NOT),
  `Values`, `ValuesList`, `Pluck`, `Distinct`, `Aggregate`, `Annotate` and `F`
  expressions; the `iexact`, `istartswith`, `iendswith`, `range`, `regex` and
  `iregex` lookups; `First`, `Last`, `GetOrCreate` (safe under concurrent
  creates), `UpdateOrCreate`, `BulkCreate`, `Save` and `InBulk`; raw SQL (`Raw`,
  `Exec`, `QueryRows`) that runs inside `Atomic` and counts in
  `AssertNumQueries`; `AtomicDurable`; and `ErrUniqueViolation`,
  `ErrForeignKeyViolation`, `ErrNotNullViolation` and `ErrCheckViolation` for
  `errors.Is`.
- ORM queries: Django's date and time transforms (`year`, `iso_year`, `month`,
  `day`, `week`, `week_day`, `iso_week_day`, `quarter`, `hour`, `minute`,
  `second`, `date` and `time`) work in `Filter`, `Exclude`, `OrderBy`, `Values`,
  `F` and aggregates, also across relations and chained with a lookup, for
  example `Filter("pub_date__year__gte", 2005)` or
  `Values("created_at__date")`. A datetime is read in UTC
  (`orm.TransformTimeZone`). The SQL is Django's on both databases: a year
  lookup is a `BETWEEN` on the column, and `Values` with `Annotate` groups a
  transform by its position in the SELECT list. A third-party backend gives the
  SQL through the new `orm.TransformDialect` interface.
- Migrations: `makemigrations` asks whether a field or a model was renamed and
  writes `RenameField` or `RenameModel`, so a rename keeps the column's data
  (`--noinput` answers no and warns). Each app has its own history under
  `migrations/<app>/` with dependencies across apps. `migrate <app> <name>` goes
  forward or back to a migration, `migrate <app> zero` unapplies an app, and
  each migration applies atomically. New commands `showmigrations` and
  `sqlmigrate`; new flags `makemigrations --dry-run --check --name --empty
  --noinput` and `migrate --fake --plan --noinput`; new operations `AddIndex`,
  `RemoveIndex`, `AlterUniqueTogether`, `AlterModelTable`, `RunSQL` and `RunGo`.
  A new required column on a table with rows is filled from the field default or
  from a one-off value that `makemigrations` asks for. On PostgreSQL, `AlterField`
  uses `ALTER COLUMN` instead of a table rebuild. `Application.OnPostMigrate`
  registers hooks that run after `migrate`, as Django's post_migrate.
- URLs and templates: Django URL dispatch with path converters, `re_path`,
  namespaces, `reverse` and `resolve`, `APPEND_SLASH` and 404 and 405 handlers.
  Template loading from several sources with `TemplateDoesNotExist`, the full
  `{% url %}` tag (keyword arguments and `as`), context processors, model
  contexts with relation attributes (`{{ post.author }}`, `{{ post.author_id }}`),
  `resetcycle` and named cycles, and Django's lexer, escaping, filters and float
  output.
- Auth: users, groups, permissions, content types and sessions use Django's
  tables (`auth_*`, `django_content_type`, `django_session`), so a Djan-Go-Go
  project and a Django project can share a database. Passwords use Django's hash
  formats (pbkdf2_sha256 at 1,200,000 iterations, argon2, bcrypt_sha256, scrypt)
  and are hashed again at login when the algorithm or work factor is old.
  Permissions are created for every model after `migrate`. New login, logout and
  password change views with Django's templates and safe `next` handling, the
  `LoginRequired`, `PermissionRequired`, `StaffRequired` and `UserPassesTest`
  guards, and password validators with Django's messages. Login cycles the
  session key and the CSRF token, and a password change logs out the other
  sessions of the user. Templates can use `{{ user.username }}`,
  `{% if user.is_authenticated %}` and `{% if perms.blog.add_post %}`.
- Sessions and CSRF: sessions support Django's cookie settings, expiry,
  expire-at-browser-close and save-every-request, and send `Vary: Cookie`. CSRF
  uses masked tokens, trusted origins with wildcards and `csrf.Exempt`, and
  answers a failure with Django's reason.
- Messages: a messages framework (`messages.Info(r, "...")`) with a template
  context processor, and `djangotest.AssertMessages`.
- Forms and views: forms render and validate like Django (input names, ids,
  labels, messages and HTML). A ModelForm keeps the defaults of omitted fields
  and reports an unknown field name. Generic views (`TemplateView`,
  `RedirectView`, `ListView`, `DetailView`, `FormView`, `CreateView`,
  `UpdateView`, `DeleteView`), pagination, `GetObjectOr404`, `GetListOr404`,
  Django-style 400, 403, 404 and 500 pages, and `djangotest.AssertFormError`.
- Commands: `changepassword`, `clearsessions`, `check` (with Django's deploy
  checks; checks also run before `runserver` and `migrate`), `collectstatic`,
  `findstatic`, `dbshell` and `shell` (a query shell over the models).
  `createsuperuser` validates the password, asks again after an invalid answer,
  and supports `--noinput` with `DJANGO_SUPERUSER_PASSWORD`. `help` groups the
  commands by app, and an unknown command suggests the closest one. `runserver`
  accepts `8080` and `0.0.0.0:8000`, prints Django's banner and a notice about
  unapplied migrations, logs requests and shuts down gracefully.
- Settings and the application: settings carry Django's names and defaults,
  and `DJANGOGO_*` environment variables and a `.env` file override them.
  `djangogo.New` opens PostgreSQL (`postgres`, `postgresql`, `pgx`), reads
  `DATABASE_URL`, and applies the settings to the components: sessions (the
  database store when `SessionEngine` is `db` and the `sessions` app is
  installed, else signed cookies), CSRF, auth (secret keys, login URLs,
  password hashers and validators by Django name), template directories and app
  templates, context processors by name, `APPEND_SLASH`, `DEBUG` and the
  `404.html` page. An unknown hasher, validator or context processor makes `New`
  fail with `ImproperlyConfigured`. The default middleware adds host validation,
  security headers, X-Frame-Options, gzip, the messages middleware and a
  technical 500 page in debug; `Application.Use` adds project middleware. Static
  files are served under `STATIC_URL` in debug.
- `startproject` generates a Django-style project: settings with an environment
  overlay and a new secret key, the `auth`, `contenttypes` and `sessions` apps,
  a sample app with a model, admin, views, namespaced URLs, templates, static
  files and a djangotest test, and migration packages that `main.go` already
  imports. `go run . makemigrations && go run . migrate` works without edits.
- Field kinds: models can use Django's other field kinds. Small, big and
  positive integers, float, decimal (`orm.Decimal`), date (`orm.Date`), time
  (`orm.Time`), duration (`time.Duration`), UUID (`orm.UUID`), JSON, binary,
  slug, email, URL and generic IP address get Django's column types and CHECK
  constraints on both databases. A pointer field (`*int64`, `*orm.Date`) reads
  and writes NULL as nil. Every kind takes `choices`, and `orm.GetDisplay`
  returns the label of the stored value. `AlterField` changes a field's kind and
  keeps the data on both databases.
- `orm.Meta` gives a model Django's `Meta` options: `ordering`,
  `unique_together`, `indexes`, verbose names, `db_table`, and the choices,
  validators and error messages of its fields. `makemigrations` writes
  `AddIndex` and `AlterUniqueTogether` from them, and each foreign key gets an
  index, as in Django.
- Model validation: `orm.FullClean`, `CleanFields` and `ValidateUnique` check
  null and blank, choices, the validators of each kind, custom validators, the
  model's `Clean()` method, foreign key targets, `unique` and `unique_together`,
  with Django's messages. The new `validators` package holds Django's
  validators; `forms.ValidationError` and `forms.Validator` are aliases to it.
- Relations: a foreign key adds the reverse accessor `<model>_set` (or its
  `related_name`) to its target. Read it with `orm.Related` and change it with
  `orm.RelatedSet` (Create, Add, Remove, Clear, Set). Filters follow reverse
  relations with `__` and reuse joins as Django does. `O2O[T]` declares a
  one-to-one field with the reverse accessor `orm.RelatedOne`, and
  `SelectRelated` follows it both ways. `M2M[T]` declares a many-to-many field
  with an automatic or an explicit `through` table: `orm.Many` returns its
  manager (Add, Remove, Clear, Set, Create, GetOrCreate, filters), filters
  follow it with `__`, `orm.OnM2MChanged` receives the `m2m_changed` signal,
  and migrations create, alter and drop the join tables.
- `PrefetchRelated` loads forward and reverse foreign keys, one-to-one and
  many-to-many relations, and chained `__` paths, with one batched `IN` query
  per level. `orm.Prefetch` takes a custom queryset and a `ToAttr` name, as
  Django's `Prefetch`, and `orm.PrefetchRelatedObjects` prefetches onto rows
  already loaded. The related managers read `All`, `Count`, `Exists` and
  `First` from the prefetched rows; a model keeps them when it embeds
  `orm.RelCache`.
- Templates read the relations a view loaded: one-to-one fields both ways
  (`{{ user.profile }}`), foreign keys loaded by `PrefetchRelated`, and
  prefetched sets (`{% for c in post.comment_set.all %}`,
  `{{ post.tags.count }}`). Rendering still runs no query.
- Deletes run Django's collector: `on_delete=cascade` and `set_null` also apply
  where the database does not enforce them, and `restrict` and the new
  `protect` and `do_nothing` options work as in Django: a blocked delete fails
  with `ErrRestricted` or `ErrProtected` and Django's message.
- Forms: `GenericIPAddressField` and `JSONField`. A ModelForm maps every field
  kind to Django's form field (integer ranges, decimal digits and places, dates,
  times, durations, UUIDs, JSON, choices as a `TypedChoiceField`) and saves the
  cleaned values as the model's Go types.
- Forms: `ModelMultipleChoiceField`. A ModelForm edits many-to-many fields: it
  lists the related rows, `WithQuerySet` limits them, and `Save` writes the
  selection. A caller that saves the instance itself calls `SaveM2M`.
- Admin: the add and change forms edit many-to-many fields and save them in
  the same transaction as the row. `FilterHorizontal` and `FilterVertical`
  render them with the markup of Django's `FilteredSelectMultiple`, and
  `ReadonlyFields` shows the selected rows. The `check` command reports
  Django's admin errors for `FilterHorizontal`, `FilterVertical` and
  `ListDisplay` (`admin.E013`, `E019`, `E020`, `E108` and `E109`).
- Admin: Django's URL layout (`/admin/<app>/<model>/<pk>/change/`), route
  names and templates; an index and app pages grouped by app; logout and
  password change views; per-model permissions, with a read-only form for
  users with only the view permission. The changelist paginates
  (`ListPerPage`, Show all), sorts by several columns, searches `SearchFields`
  (with `^`, `=` and `@`, and across relations) and filters with `ListFilter`,
  including `SimpleListFilter`. Bulk actions: `delete_selected` with Django's
  confirmation page, custom actions on `ModelAdmin.Actions` or
  `AdminSite.AddAction`, and selection across pages.
- The fidelity harness also compares query SQL (86 cases) and migration SQL (29
  operations) with Django 6.0.3 on SQLite and PostgreSQL, and
  `go generate ./fidelity/...` regenerates the golden files from Django.

### Changed

- **BREAKING**: migrations generated by an earlier version must be generated
  again. The `Operation` interface changed, `Migration.Dependencies` is now
  `[]migrations.Key`, and `Application.MigrationsApp`, `migrations.Diff`,
  `MakeMigrations`, `EnsureTable`, `AppliedSet` and `ErrNoChanges` are removed
  (use `Recorder.EnsureSchema` and `Recorder.AppliedMigrations`). The tracking
  table is now `django_migrations` with an `applied` column; an existing
  `djangogo_migrations` table is renamed automatically.
- **BREAKING**: the auth models match Django's tables and columns, so the auth
  migrations must be generated again. `User.PasswordHash` is now
  `User.Password`, `HasPerm` and `PermissionRequired` take
  `"app_label.codename"`, `auth.Iterations` is replaced by
  `PBKDF2Iterations`, and the fields of `sessions.Record` are now `SessionKey`,
  `SessionData` and `ExpireDate`.
- **BREAKING**: the default CSRF middleware keeps the secret in the
  `csrftoken` cookie instead of the session, and `csrf.SessionKey` is now
  `_csrftoken`.
- **BREAKING**: form input names are the attribute names (`name="title"`), not
  the Go field names (`name="Title"`). Update templates and clients that post
  the old names. The old `forms.Field` API (`FieldKind`, `Errors()`,
  `Fields()`, `AddError(string, string)`, `BoundValue` and the widget `Render`
  methods) is replaced by Django's form, field and widget API.
- **BREAKING**: `ListView` and `DetailView` use Django's field names:
  `TemplateName`, `ContextObjectName`, `Queryset` or `GetQueryset`, `PKURLKwarg`
  and `ExtraContext` or `GetContextData`. A list is in `object_list` and
  `<model>_list`.
- **BREAKING**: `QuerySet.Update` no longer sets `auto_now` fields, as Django's
  `update()`. Use `Save` to set them.
- **BREAKING**: `startapp <name>` creates `<name>/` instead of `<name>app/`, and
  `scaffold.Project` takes `scaffold.Options`.
- **BREAKING**: the admin moved to Django's URLs: `/admin/<model>/` is now
  `/admin/<app>/<model>/`, and so on for add, change and delete. The route
  names `admin-index`, `admin-login`, `admin-changelist`, `admin-add`,
  `admin-change` and `admin-delete` are now `admin:index`, `admin:login` and
  `admin:<app>_<model>_changelist`, `_add`, `_change` and `_delete`. Update
  links and `{% url %}` tags. The embedded templates moved to Django's paths
  (`admin/change_list.html`, `admin/change_form.html`, `registration/...`), so
  an override must use the new name.
- **BREAKING**: a staff user who is not a superuser now needs the model's
  permissions (`<app>.view_<model>`, `add_`, `change_`, `delete_`) to see or
  change it in the admin, as in Django. Grant them to keep the old access. A
  missing object redirects to the admin index with a warning instead of a 404.
- **BREAKING**: a ModelForm runs the model's validation, as Django's does:
  `Clean()`, `Meta.Validators`, `Meta.ErrorMessages` and the unique and
  `unique_together` checks now show as form errors. A nullable text field
  cleans empty input to NULL instead of `""`.
- **BREAKING**: a ModelForm of a model with a many-to-many field now includes
  that field, as Django's does. The form requires a value unless the model
  field has `blank`. Leave the field out with `WithExclude` or `WithFields`.
- The options of a foreign key or many-to-many field follow the related
  model's `Meta.Ordering` in a ModelForm, and the ordering of the related
  model's admin in the admin, as in Django. Before, they were in primary key
  order.
- **BREAKING**: SQLite now enforces foreign keys: the backend turns on the
  `foreign_keys` pragma, so an insert or update with a missing target fails as
  on PostgreSQL. Add `_pragma=foreign_keys(0)` to the DSN to turn it off.
- **BREAKING**: the helper `orm.Prefetch[Child](ctx, db, fkColumn, pks)` is
  removed. Use `Query[T](db).PrefetchRelated(...)` or
  `orm.PrefetchRelatedObjects`. `orm.Prefetch` is now Django's `Prefetch`
  option.
- **BREAKING**: `migrations.Autodetector.PreserveModelOptions` is removed.
  `makemigrations` reads indexes and `unique_together` from `orm.Meta`.
- Templates print a `time.Duration` like Python's `timedelta` (`1:30:00`), a
  date with `DATE_FORMAT` and a time with `TIME_FORMAT`; the `date`, `time` and
  `timesince` filters accept `orm.Date` and `orm.Time`.
- `{% csrf_token %}` renders nothing when the request has no token, and escapes
  the token as Django does.
- A command error prints as `CommandError: ...` without a stack trace. Exit
  code 1 is a command error and 2 a usage error. The hints of `migrate` and
  `makemigrations` name the program that runs. `startproject` and `startapp`
  print nothing at the default verbosity (`-v 2` lists the files), and
  `djangogo version` prints the bare version.

### Fixed

- `is_active` is now enforced across authentication, mirroring Django's
  `ModelBackend`: an inactive user is rejected at admin login, holds no
  permissions (even a superuser), and is treated as anonymous when resolved from
  the session, rather than staying authenticated until the session expires.
- CSRF protection now verifies the request origin on unsafe methods before the
  token, like Django: an `Origin` header must match the host, falling back to a
  strict `Referer` check over HTTPS.
- ModelForm foreign-key `<select>` now offers Django's empty `---------` option,
  so a nullable relation can be cleared and an unset relation is no longer
  silently saved as the first related row.
- `auto_now`/`auto_now_add` fields are excluded from the ModelForm (non-editable
  in Django) instead of rendered as editable, soon-overwritten inputs.
- The process-global `{% url %}` resolver is now race-free (guarded behind
  `SetURLResolver`/`URLResolverFunc`) rather than a plainly-mutated package var.
- The admin no longer runs a full-table foreign-key option query for fields
  excluded from the form.
- A change in the admin did not update `auto_now` fields and did not send
  `pre_save` and `post_save`. The change view now saves through `Save`, as
  Django's `save_model`.
- Case-insensitive lookups such as `icontains` were case-sensitive on
  PostgreSQL. They now ignore case on both databases. A `%` or `_` in the value
  of `contains`, `startswith` or `endswith` matched any character; LIKE
  wildcards are now escaped.
- A template that made pongo2 panic now returns an error.
- A savepoint that was rolled back was not released. It is now released.
- On PostgreSQL, a larger `max_length` rewrote the whole table: `AlterField`
  now adds `USING` only when the base type changes. `RemoveField` no longer
  drops dependent objects with `CASCADE`, and `RemoveIndex` uses
  `DROP INDEX IF EXISTS`, as Django does.

### Security

- A value printed in a template through its `String` method was not
  HTML-escaped. Model instances, the post titles of the example and any other
  type with a `String` method are now escaped. Mark trusted HTML with
  `templates.HTMLer` or `html/template.HTML`.
