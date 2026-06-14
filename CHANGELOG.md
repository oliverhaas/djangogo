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
