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

### Fixed

- ModelForm foreign-key `<select>` now offers Django's empty `---------` option,
  so a nullable relation can be cleared and an unset relation is no longer
  silently saved as the first related row.
- `auto_now`/`auto_now_add` fields are excluded from the ModelForm (non-editable
  in Django) instead of rendered as editable, soon-overwritten inputs.
- The process-global `{% url %}` resolver is now race-free (guarded behind
  `SetURLResolver`/`URLResolverFunc`) rather than a plainly-mutated package var.
- The admin no longer runs a full-table foreign-key option query for fields
  excluded from the form.
