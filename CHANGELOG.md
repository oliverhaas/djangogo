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
- Admin: `Fields` and `Fieldsets` lay out the add and change forms with
  Django's markup: several fields on one line, named fieldsets with a
  description, and the `collapse` and `wide` classes. `ReadonlyFields` shows
  model fields and `DisplayColumns` values in the form, and a foreign key as a
  link to the change page of the related object. `FormOptions` adds form
  fields, widgets and labels to the model form of the admin. `GetFields`,
  `GetFieldsets` and `GetReadonlyFields` choose them per request and object.
  The `check` command also reports Django's errors for `Fields`, `Fieldsets`,
  `ExcludeFields` and `ReadonlyFields` (`admin.E005`, `E006`, `E011`, `E012`,
  `E013`, `E015` and `E035`).
- Admin: a large related table no longer has to fit into one `<select>`. A
  foreign key or many-to-many field in `RawIDFields` is a text input for the
  primary keys with a link to the related changelist and, for a foreign key,
  the label of the selected row. A field in `AutocompleteFields` (or
  `GetAutocompleteFields`) renders Django's `AutocompleteSelect` markup, and
  the new `autocomplete/` view answers its search requests as Django's
  `AutocompleteJsonView` does. Both kinds of field load only the related rows
  they show or receive. The `check` command reports Django's errors for them
  (`admin.E002`, `E003` and `E037` to `E040`).
- Admin: the pages load Django's admin stylesheets and scripts, which the
  admin serves as static files (`admin.StaticFS`) when an app registers with
  the admin. A project's or an app's file of the same name replaces the
  admin's file. This brings Django's look, the light and dark themes, the
  navigation sidebar (`AdminSite.EnableNavSidebar`), the two filtered lists
  of `FilterHorizontal` and `FilterVertical`, the search box of
  `AutocompleteFields`, the lookup popup of `RawIDFields` and the selection
  counter of the bulk actions. The `jsi18n/` view serves the English
  catalog of the scripts. `AdminSite.Debug`, set from `Settings.Debug`, loads
  the unminified jQuery and Select2.
- Admin: the add and change forms use Django's admin widgets. A date field
  gets a calendar and a "Today" link, a time field a clock and a "Now" link,
  and a date and time field is split into a date and a time input
  (`AdminSplitDateTime`). Text, URL, integer, email and UUID fields get the
  admin's classes, and a URL field shows its current value as a link. A
  foreign key or many-to-many field gets the links that add, change, view and
  delete the related object, as far as the user's permissions allow
  (`RelatedFieldWidgetWrapper`). These links open the related admin in a
  popup, and saving or deleting there updates the field in the form that
  opened it. `FormOptions` still replaces any of these widgets.
- Admin: the add and change pages edit the rows of related models in place,
  as Django's inlines do. `ModelAdmin.Inlines` (or `GetInlines`) lists them,
  made by `admin.TabularInline` or `admin.StackedInline` from an
  `InlineModelAdmin`. An inline takes the layout options of a `ModelAdmin`
  and `FKName`, `Extra`, `MinNum`, `MaxNum`, `CanDelete`, `ShowChangeLink`,
  verbose names, `Classes`, `Media` and the permission hooks. The page adds,
  changes and deletes the rows together with the parent row, with Django's
  "Add another" links and error messages, and a user sees only the inlines
  that their permissions allow. The `check` command reports Django's inline
  errors (`admin.E201` and `admin.E202`) and runs the field checks on each
  inline.
- Admin: `ModelAdmin.GetExclude` and `InlineModelAdmin.GetExclude` choose the
  excluded fields per request, as Django's `get_exclude` does.
- Forms: `forms.FromInstance` derives a form from a model for an instance and
  validates the bound form against the model, as `NewModelForm` does, while
  the caller sets the options of the foreign key fields.
- Forms: `forms.WithClean` adds a form-level clean function to the options of
  a model form, so a formset or the admin that builds the form from the
  options runs it too. `forms.ModelFormLabels` returns the labels and help
  texts that the options set.
- Forms: `MultiValueField` cleans one value made of several fields, and
  `SplitDateTimeField` takes a date and a time in two inputs. `MultiWidget`
  renders several widgets as one; `SplitDateTimeWidget` and
  `SplitHiddenDateTimeWidget` are its date and time forms. A widget outside
  the package can implement `forms.MultiValueWidget` to lay out its own
  subwidgets. The options `InputDateFormats`, `InputTimeFormats` and
  `RequireAllFields` configure the new fields.
- Forms: `WithFormFieldCallback` builds the form fields of a model form
  through a function, as Django's `formfield_callback` does. `forms.Escape`
  escapes HTML as Django does, and `templates.QuoteJSON` quotes a string as
  Python's `json.dumps` does.
- Forms: `forms.Media` holds the stylesheets and scripts of a widget or a form,
  as Django's `Media` does: a widget declares them with a `Media` method,
  `Form.Media` adds up the media of its widgets and `Form.ExtraMedia`, and a
  template prints `{{ form.media }}`. `templates.Static` returns the URL of a
  static file.
- `templates.Localize` formats a `time.Time`, `orm.Date` or `orm.Time` as a
  template prints it, and `orm.Model.ApplyDefaults` sets the field defaults on
  a new object.
- The fidelity harness also compares query SQL (86 cases) and migration SQL (29
  operations) with Django 6.0.3 on SQLite and PostgreSQL, and
  `go generate ./fidelity/...` regenerates the golden files from Django.
- Forms: `forms.FormSet` edits several copies of a form on one page, as
  Django's formsets do: the management form, `Extra`, `MinNum`, `MaxNum`,
  `AbsoluteMax`, `ValidateMin`, `ValidateMax`, ordering (`CanOrder`,
  `OrderedForms`) and deletion (`CanDelete`, `CanDeleteExtra`,
  `DeletedForms`), the empty form for JavaScript, non-form errors from
  `FormSet.CleanWith`, and Django's error messages and HTML. `ConfigureForm`
  takes the place of `form_kwargs`, and `forms.AllValid` validates several
  formsets.
- Forms: `forms.NewModelFormSet` and `forms.NewInlineFormSet` edit the rows of
  a model on one page, as Django's `modelformset_factory` and
  `inlineformset_factory` do. Each form carries the primary key of its row in
  a hidden field, validation checks unique fields and `unique_together`
  across the forms with Django's "Please correct the duplicate data" errors,
  and `Save` adds, changes and deletes rows and reports them in `NewObjects`,
  `ChangedObjects` and `DeletedObjects`. `QuerySet`, `EditOnly`, `SaveAsNew`
  and the `SaveNew`, `SaveExisting` and `DeleteExisting` hooks take the place
  of Django's arguments and method overrides. An inline formset finds the
  foreign key to the parent, or takes its name, and keeps it in a hidden
  field.
- ORM: `QuerySet.Ordering` returns the keys a queryset is ordered by, and
  `QuerySet.TotallyOrdered` reports whether they decide the position of every
  row, Django's `ordered` and `totally_ordered`.
- auth: `Hasher.SafeSummary` returns the parts of an encoded password that
  can be shown, with the salt and the hash masked (Django's `safe_summary`).
  The username field of `User` validates with Django's validator and reports a
  taken name with Django's message.
- Forms: `forms.RenderAttrs` renders HTML attributes for widgets of other
  packages.
- Forms: `forms.WithFieldClass` builds the form field of a model field with
  another constructor, Django's `Meta.field_classes`, and
  `forms.WithCleanField` adds a clean function for one field to the options
  of a model form, the counterpart of a `clean_<field>()` method. `Form.Context`
  returns the context that `Form.SetContext` set, for clean functions that
  query the database.
- auth: the user forms of `django.contrib.auth.forms`. `NewUserCreationForm`
  and `NewAdminUserCreationForm` create a user with a password typed twice,
  check the username without regard to case, validate the password with the
  password validators and save the hash; the admin form can create a user
  without a usable password. `NewUserChangeForm` edits a user and shows the
  password as `ReadOnlyPasswordHashField`, a summary of the hash with a link
  to the password page. `NewAdminPasswordChangeForm` sets or removes the
  password of a user. `UsernameField` normalizes usernames to NFKC, and
  `RenderPasswordAsHash` is Django's `render_password_as_hash` template tag.
- Admin: users and groups have an admin, Django's `UserAdmin` and
  `GroupAdmin`. The application registers them when the auth app is
  installed and an app registers models with the admin; `admin.UserAdmin(db)`
  and `admin.GroupAdmin()` return them for a site of your own or for changes.
  A new user is added with a username and a password typed twice, then edited
  on the change page, which shows the password as a summary of its hash. The
  page at `<id>/password/` (URL name `auth_user_password_change`) sets a new
  password or disables password-based authentication. Only a user who can
  change users can add them, and the changelist rejects lookups on the
  password. `auth.PrepareNewUser` readies a user that a creation form filled
  without saving it.
- Admin: `ModelAdmin.GetFormOptions` returns the form options per request and
  object, Django's `get_form`, for example to use another form on the add
  page.
- ORM: a model whose label needs the database implements
  `orm.ContextLabeler`, `Label(ctx, db) (string, error)`, in place of
  `String`. `orm.LabelContext` returns such a label, and the choice fields of
  forms and the admin use it. `auth.Permission` labels itself as Django's
  `Permission.__str__` does, "app | model | name", so the permission choices
  of the user and group admin read as in Django.
- Messages: `messages.AddSafe` and `Storage.AddSafe` add a message whose text
  is HTML, Django's message from `mark_safe`. `Message.Safe` reports it, and
  the session keeps the flag, as Django's `is_safe`.
- ORM: `DeletePlan.Tree` holds the collected objects as a tree of
  `DeleteNode`, each object with the objects that its deletion cascades to,
  Django's `NestedObjects.nested()`.
- Admin: the delete page and the `delete_selected` confirmation list the
  objects to delete as a nested list, each with the objects that cascade from
  it, followed by a summary of the count per model, as in Django. When rows
  protect the object, or the user lacks the delete permission of a model that
  the deletion reaches, the page shows "Cannot delete" with those objects or
  models.
- Admin: the admin keeps a history of the changes made through it, Django's
  `LogEntry` in the `django_admin_log` table. To use it in an existing
  project, add `admin.App{}` to the installed apps, then run `makemigrations`
  and `migrate`. A project from `startproject` installs it. The add, change,
  delete, `delete_selected` and password pages log each change with Django's
  change message ("Added.", "Changed Title.", "Changed password."). The
  history page of an object lists its entries, 100 per page, and the index
  shows the recent actions of the user. `AdminSite.GetLogEntries` filters the
  recent actions, Django's `get_log_entries`. `admin.LogActions` logs entries
  of your own, and `LogEntry.GetChangeMessage`, `GetEditedObject` and
  `GetAdminURL` read them.
- ORM: `orm.GetByPK` loads an instance of a registered model by its primary
  key when only the model metadata is known.
- Admin: the change page links the history of the object, Django's "History"
  object tool. `ModelAdmin.SaveAs` replaces "Save and add another" on the
  change page with "Save as new", which saves the form and the inlines as a
  new row and needs the add permission. `ModelAdmin.SaveAsContinue` (default
  true) opens the new row after the save; false returns to the changelist.
  `ModelAdmin.SaveOnTop` shows the save buttons above the form as well. The
  submit row is in its own template, `admin/submit_line.html`, as in Django.
- Admin: the filters, search and page of a changelist stay in place when you
  open, save or delete one of its rows, and the "Save and continue" and "Save
  and add another" redirects keep the query string of the form, as Django's
  `preserve_filters`. `ModelAdmin.PreserveFilters`, which had no effect until
  now, turns this off when set to false. The change form posts to
  `?_changelist_filters=...` with filters and has no `action` attribute
  without them.
- Admin: a project can override the admin's templates. Until now the admin
  read only its own templates, so an `admin/base_site.html` in a template
  folder of the project had no effect. A file of the same name in
  `Settings.Templates.Dirs` now replaces the admin's, and the template trees
  of the apps are searched after the admin's, as in a Django project with the
  admin first in `INSTALLED_APPS`. As in Django, the change form,
  changelist, delete, history and popup pages and their parts
  (`submit_line.html`, `change_form_object_tools.html`,
  `change_list_object_tools.html`, `search_form.html`, `actions.html`,
  `change_list_results.html`, `pagination.html`) are looked up as
  `admin/<app>/<model>/<name>`, then `admin/<app>/<name>`, then
  `admin/<name>`, so one model's pages can differ. `ModelAdmin` takes
  Django's template options `ChangeFormTemplate`, `AddFormTemplate`,
  `ChangeListTemplate`, `DeleteConfirmationTemplate`,
  `DeleteSelectedConfirmationTemplate`, `ObjectHistoryTemplate` and
  `PopupResponseTemplate`, and `UserAdmin` sets `AddFormTemplate` to
  `admin/auth/user/add_form.html`. `AdminSite` takes `IndexTemplate`,
  `AppIndexTemplate`, `LoginTemplate`, `LogoutTemplate`,
  `PasswordChangeTemplate` and `PasswordChangeDoneTemplate`. For a site
  built with `NewAdminSite`, `AdminSite.SetTemplateSources` sets the lookup
  and `admin.Templates()` returns the admin's own templates. The change form
  has Django's `field_sets`, `after_field_sets`, `inline_field_sets` and
  `after_related_objects` blocks and loads `admin/js/change_form.js`, and
  the changelist has the `filters`, `search`, `result_list` and `pagination`
  blocks. The change form, changelist and delete pages keep the `bodyclass`
  of `admin/base_site.html`. A block that Django names with hyphens has
  underscores here: `object_tools`, `object_tools_items`, `submit_row`,
  `actions_form`, `actions_submit` and `actions_counter`.
- Admin: `ModelAdmin.PrepopulatedFields` and `InlineModelAdmin.PrepopulatedFields`
  fill a field from other fields as the user types, Django's
  `prepopulated_fields`, such as `{"slug": {"title"}}`.
  `GetPrepopulatedFields` picks them per request and object. The change form
  renders Django's `admin/prepopulated_fields_js.html` with the fields of the
  form, unless the user can only view it, and those of the inline forms that
  add a row. The checks report admin.E027, admin.E028 and admin.E030.
- Admin: `ModelAdmin.ListSelectRelated` and `GetListSelectRelated` name the
  relations that the changelist query joins, Django's `list_select_related`
  as a list. A non-nil empty slice joins none. A relation that a column
  shows and the query does not join is loaded in one more query, so the
  column shows its label, not its primary key. Without the option the query
  joins the foreign keys that the columns show, as before. Django's `True`,
  which Django 6.1 deprecates, has no counterpart.
- Admin: the hooks `ModelAdmin.SaveModel`, `SaveRelated` and `DeleteModel`
  replace how the add, change and delete pages save or delete the object,
  Django's `save_model`, `save_related` and `delete_model`. A hook gets a
  `SaveContext` or `DeleteContext` whose `Save`, `SaveRelated` and `Delete`
  run the default, the counterpart of calling `super()`. The hooks run in
  the transaction of the page, and `r.Context()` carries it.
- Admin: `ModelAdmin.ListEditable` edits fields in the changelist rows,
  Django's `list_editable`. Each row shows a form with the fields, and the
  Save button saves the changed rows in one transaction through the
  `SaveModel` and `SaveRelated` hooks, with a log entry each. A row that the
  user cannot change keeps its read-only cells. Invalid rows show their
  errors in place, under an "Error:" title. `ChangelistFormSet` customizes
  the formset, for example with a check across the rows. The checks report
  admin.E121 to admin.E125.
- Forms: `ModelFormSet.Objects` sets the objects of the initial forms in
  place of the queryset.
- Admin: the change page and the inline rows show a "View on site" link for
  an object with a `GetAbsoluteURL` method, Django's `view_on_site`. The link
  goes through the new `admin:view_on_site` URL, which redirects to the URL
  of the object. `ModelAdmin.ViewOnSite` and `InlineModelAdmin.ViewOnSite`
  set to false hide the link, and `ViewOnSiteURL` returns a custom URL.
- Views: `ContentTypeShortcut` is Django's contenttypes `shortcut` view. It
  redirects to the `GetAbsoluteURL` of the object of a content type and a
  primary key.
- Admin: `ModelAdmin.RadioFields` and `InlineModelAdmin.RadioFields` show a
  foreign key or a field with choices as radio buttons in place of a select,
  Django's `radio_fields`. `Horizontal` and `Vertical` set the layout. Only a
  field that can be blank gets the empty choice, labeled "None". The checks
  report admin.E022 to admin.E024.
- Admin: `ModelAdmin.FormfieldOverrides` and
  `InlineModelAdmin.FormfieldOverrides` set the widget and the empty label of
  the form fields by model field class, Django's `formfield_overrides`, such
  as `{admin.TextField: {Widget: forms.Textarea{}}}`. An entry for a class
  also applies to the classes that derive from it, such as `SlugField` from
  `CharField`. The widget of a foreign key or a many-to-many field wins over
  the autocomplete, raw-id, radio and filter widgets.
- ORM: `QuerySet.Dates` returns the distinct dates of a date or datetime
  field, truncated to the year, the month, the ISO week or the day, Django's
  `dates()`. `QuerySet.DateTimes` returns the distinct datetimes of a datetime
  field and also truncates to the hour, the minute or the second, Django's
  `datetimes()`. The field can span relations, and a datetime is read in UTC.
- ORM: `Expr.Filter` makes an aggregate consider only the rows that match a
  condition, Django's `filter=` argument:
  `orm.Count("pk").Filter("rating__gt", 3)`. It takes the arguments of `Q`
  and compiles to `FILTER (WHERE ...)` on SQLite and PostgreSQL.
- Admin: `ModelAdmin.DateHierarchy` adds Django's `date_hierarchy` to the
  changelist. Links above the rows go from the years that have rows to the
  months of a year and the days of a month, and each link shows the rows of
  its range. The field can be on a related model, such as
  `"question__posted"`. The checks report admin.E127 and admin.E128.
- Admin: the changelist filters show facets, Django's `show_facets`: each
  filter choice shows the count of the rows that it selects, as in
  "Fiction (3)". The count applies the other filters, the date hierarchy and
  the search. `ModelAdmin.ShowFacets` defaults to `admin.ShowFacetsAllow`,
  which shows the counts after a click on "Show counts" in the filter
  sidebar. `admin.ShowFacetsAlways` always shows them, and
  `admin.ShowFacetsNever` hides the counts and the link. Each filter with
  counts runs one more query. A choice of a `SimpleListFilter` whose `Filter`
  returns no condition shows "(-)".
- Admin: `check` reports these configuration mistakes, which the changelist
  ignores or fails on: a `ListDisplayLinks` entry that is not a column of
  `ListDisplay` (admin.E111); a `ListFilter` entry that is not a field name, a
  `FieldFilter` or a `SimpleListFilter` (admin.E113), or that names no field
  (admin.E116); an `Ordering` with `"?"` and other names (admin.E032), or with
  a name that is no field of the model, also on an inline (admin.E033); two
  `Actions` with the same name (admin.E130). Run `check` after the upgrade.
- Admin: `ListDisplay` and `ReadonlyFields` show model methods, as Django
  shows a method or a property of the model, and `check` accepts them. A
  model method is exported, takes no arguments, and returns a value, or a
  value and an error. `model_year` and `ModelYear` both name the method
  `ModelYear`, as in templates. A `DisplayColumns` entry without a `Value`
  sets the `@admin.display` options of the method of its name: the
  description, the order field, the boolean icon and the empty value. An
  error of a method fails the changelist with 500 Internal Server Error,
  except `orm.ErrDoesNotExist`, which shows the empty value. A read-only
  field shows the empty value for any error.
- Admin: `ModelAdmin.ExtraContext` and `AdminSite.ExtraContext` add values to
  the template context of a page, as Django's `extra_context` argument of the
  views. The hook gets the request and the name of the view, such as
  `"changelist"`, `"change"` or `"index"`. On the add and change pages the
  values do not replace the permission flags and the buttons, and a false
  `show_save`, `show_save_and_continue`, `show_save_and_add_another` or
  `show_delete` hides that button, as Django's `submit_row` tag does.
- Admin: `FormfieldOverride.Options` and `FormfieldOverride.FormClass` give
  the model fields of a class the other arguments of Django's
  `formfield_overrides`, such as `{admin.CharField: {Options:
  []forms.FieldOption{forms.Strip(false)}}}` for `strip=False` and
  `FormClass: forms.Class(forms.NewEmailField)` for `form_class`. A
  `FormClass` on `DateTimeField` replaces the split date and time field.
- Admin: `InlineModelAdmin.FormSet` customizes the formset of an inline,
  Django's `formset` class of an inline and `get_formset_kwargs`. Add a
  check across the rows with `fs.CleanWith`, or wrap `fs.ConfigureForm` to
  change each form.
- forms: `WithFieldOptions` adds field options to the form field of a model
  field, the arguments of Django's `Field.formfield()`, and `forms.Class`
  turns a field constructor such as `forms.NewEmailField` into a
  `func(...forms.FieldOption) forms.Field`.
- Templates: `{% partialdef name %}` defines a named piece of a template and
  `{% partial name %}` renders it, as in Django 6.0. With `inline` it also
  renders in place. `Render("page.html#name", ...)` and
  `{% include "page.html#name" %}` render only that piece.
- Templates: the `{% regroup %}`, `{% querystring %}` and `{% debug %}` tags
  and the `pprint` filter. `{% debug %}` prints the context only when
  `Engine.SetDebug(true)` is set.
- Templates: `{% extends %}` takes a variable or a filter expression, as in
  Django: `{% extends layout %}` extends the template that `layout` names.
  This also works in a parent template, in an included template and in
  `RenderString`.
- Templates: `{% extends %}` and `{% include %}` take a name relative to the
  template of the tag, as in Django: in `blog/post.html`,
  `{% extends "./base.html" %}` extends `blog/base.html` and
  `{% include "../card.html" %}` includes `card.html`. A relative name that
  leaves the template directory, or that names the template itself in
  `{% extends %}`, fails with Django's message.
- Templates: a template can extend a template of the same name in a later
  source, as in Django. A project template `admin/base_site.html` that
  starts with `{% extends "admin/base_site.html" %}` extends the one the
  admin ships and overrides only some of its blocks. A template that extends
  itself, directly or through other templates, with no namesake left fails
  with a `TemplateDoesNotExist` whose `Tried` entry says "Skipped to avoid
  recursion".
- Forms: a template can loop over a form, a field and a formset, as in
  Django. `{% for field in form %}` yields the fields in order, and
  `{% for form in formset %}` the forms. `{% for radio in form.language %}`
  yields the radio buttons, checkboxes or `<option>`s of the widget, with
  Django's `tag`, `choice_label`, `id_for_label` and `data`. A template no
  longer needs `form.fields` or `formset.forms` for this. In Go,
  `BoundField.Subwidgets` returns the same parts, and a type that implements
  `templates.Iterable` defines what a loop over it yields.
- Views: `{% for obj in page_obj %}` loops over the objects of a page, as in
  Django. A template no longer needs `page_obj.object_list` for this.
  `Page.TemplateValue` returns a `TemplatePage`. The model instances, times
  and dates in a template value of a type defined as `map[string]any`, such
  as a page or a form, are now converted as in a plain map.
- Forms: `NewModelChoiceField` and `NewModelMultipleChoiceField` take the
  options `ToFieldName`, Django's `to_field_name`, and `LabelFromInstance`,
  Django's `label_from_instance`. With `ToFieldName("slug")` the widget
  submits the slug and the field selects the row by it. A model form shows
  the keys of the related rows as the initial value of such a many-to-many
  field. Both constructors panic on a field name the model does not have or
  a label function for another model.
- Auth: `auth.LoginRequiredMiddleware`, Django's `LoginRequiredMiddleware`,
  sends anonymous users to the login page unless the view opts out with
  `auth.LoginNotRequired`, Django's `login_not_required`. The login view and
  the admin login page opt out. Add it with
  `app.Use(auth.LoginRequiredMiddleware(app.HandlerFor))`. A view behind
  `LoginRequired` or another guard keeps the login URL and the redirect field
  name of its guard, and an admin view sends anonymous users to the admin
  login page. `auth.URLs` returns the login, logout and password change
  routes of `django.contrib.auth.urls` with their names.
- URLs: `Router.HandlerFor` and `Application.HandlerFor` return the view that
  serves a request, the resolve function of `auth.LoginRequiredMiddleware` and
  of `csrf.Options.Resolve`.
- ORM: the text functions `orm.Lower`, `orm.Upper`, `orm.Length`,
  `orm.Trim`, `orm.LTrim` and `orm.RTrim`, Django's functions of the same
  names, work in `Annotate`, `Filter`, `Update` and aggregates:
  `Update(ctx, "name", orm.Lower("name"))`. `orm.RegisterTransform`, Django's
  `register_lookup`, makes a function a transform of a field kind:
  after `orm.RegisterTransform(orm.KindChar, "lower", orm.Lower)` the key
  `"name__lower"` works in `Filter`, `Values`, `OrderBy` and `F`.
- ORM: `Meta.LimitChoicesTo`, Django's `limit_choices_to`, limits the rows
  that a foreign key or many-to-many field can point to:
  `LimitChoicesTo: map[string]orm.Cond{"Color": orm.Q("warm", true)}`. Model
  forms and the admin offer only the matching rows, in the select, the
  autocomplete and the list filter of the relation. The lookup link of a
  raw-id field opens the changelist filtered by them. `FullClean` rejects a
  foreign key to any other row. `Meta.LimitChoicesToFunc` calls a function
  for the condition each time it applies. `Relation.LimitChoicesTo` returns
  the condition, `orm.LabeledRowsWhere` loads the labeled rows that match a
  condition, and `Cond.Pairs` returns the keys and values of a condition
  built from them.
- ORM: `Meta.Constraints`, Django's `Meta.constraints`, declares
  `orm.CheckConstraint` and `orm.UniqueConstraint` values. `CreateTable`
  creates them: a check as a table constraint, a unique constraint on fields
  as a table constraint, and one with expressions, a condition or
  `NullsNotDistinct` as a unique index. `FullClean` checks them after the
  unique checks and reports Django's messages, or `ViolationErrorMessage` and
  `ViolationErrorCode`. `orm.WithValidateConstraints(false)` skips them,
  `orm.ValidateConstraints` and `Constraint.Validate` run them alone, and
  `Model.UniqueChecksWithConstraints` adds the fields of the unique
  constraints without a condition to the unique checks.
  `%(app_label)s` and `%(class)s` in a name are replaced.
- Forms: a model form checks the constraints of its model after the unique
  checks and skips a constraint that refers to a field outside the form. A
  model formset reports duplicate rows of a unique constraint as it does
  for `unique_together`.
- ORM: `orm.QuoteValue` renders a Go value as an SQL literal of a dialect,
  for statements that cannot bind parameters.
- Testing: `djangotest.AssertConstraintExists` and
  `djangotest.AssertConstraintNotExists` check a named constraint or index
  of a table.
- Migrations: `makemigrations` records `Meta.Constraints`. `CreateModel`
  takes `Constraints`, and the operations `AddConstraint`, `RemoveConstraint`
  and `AlterConstraint` add, drop and change a constraint. A change of only
  the violation message or code gives an `AlterConstraint`, which leaves the
  database alone; any other change drops the constraint and adds it again.
  On SQLite, adding or dropping a check constraint or a unique constraint on
  fields rebuilds the table. The migration writer writes conditions and
  expressions as the Go calls that build them, and the optimizer folds the
  constraint operations into `CreateModel` as Django's does.
  `orm.ConstraintSource` returns that Go source, and `orm.NewStateModel` and
  `orm.ConstraintSQL` render the SQL of a constraint on a model of a
  migration state.
- Migrations: the new `squashmigrations` command replaces the migrations of
  an app up to a given one, or from a given start, with one migration of
  their optimized operations, with Django's options `--no-optimize`,
  `--squashed-name`, `--no-header` and `--noinput`. The new migration lists
  the ones it replaces in `Migration.Replaces`. `migrate` applies it on a
  new database and the replaced migrations on a database that applied some
  of them, and records it once all of them are applied. The optimizer drops
  `RunSQL` and `RunGo` operations marked `Elidable`. A `RunGo` function is
  written by its name, and a function defined in a migration file is listed
  in a comment at the top of the new file: copy it over before the old files
  are deleted. `migrate --prune` deletes the tracking rows of migrations
  that no longer exist. `showmigrations` marks a squashed migration with the
  number of migrations it replaces and an applied but unrecorded one with
  `[-]`, and `-v 2` adds the time a migration was applied and, with
  `--plan`, its dependencies.
- Migrations: `makemigrations --merge` resolves conflicting migrations. It
  prints the operations of each branch, asks, and writes a merge migration
  that depends on the last migration of each branch, as Django does.
  `makemigrations --update` adds the model changes to the last migration of
  the app, optimizes it and writes it under a new name in place of the old
  file; the last migration must not be squashed, applied or a dependency of
  another one. `makemigrations` also takes Django's `-v`, `--scriptable`
  (messages and prompts go to the error output, the paths of new files to the
  output) and `--no-header`, and `--dry-run -v 3` prints the files it would
  write. The Go API adds `Loader.MergeBranches`, `NewMergeMigration`,
  `InteractiveQuestioner.AskMerge` and `Registry.Remove`.
- Commands: `startproject` and `startapp` take Django's `--template`, which
  reads a template from a directory, a `file://` path, a path with `~`, a tar
  archive (plain, gzip or bzip2), a zip archive or an http(s) URL of one.
  `-e`/`--extension` and `-n`/`--name` select the files rendered with Go's
  `text/template` between `[[` and `]]` (default: `.go` files and `go.mod`),
  and `-x`/`--exclude` names the directories to skip in place of the hidden
  ones. A `project_name` or `app_name` in a path becomes the given name, a
  `-tpl` suffix is dropped, and rendered `.go` files are formatted with gofmt.
  The new files honor the umask. `-v 2` reports the rendering, the download
  and the cleanup. `scaffold.Tree` renders such a template from Go, and
  `manage.OptionalValue` gives a flag whose value can be left out.
- Static files: `collectstatic -l`/`--link` creates a symbolic link to each
  file of `Settings.StaticFilesDirs` instead of a copy, as Django does. The
  embedded static files of the apps and the admin have no path on disk and
  are copied. A run with `--link` replaces the copies of an earlier run with
  links, and a run without it replaces the links with copies. The summary
  reads "N static files symlinked" and adds ", M copied" for the embedded
  files. In Go, `staticfiles.CollectOptions.Link` selects it and
  `CollectResult.Linked` lists the links.
- ORM: `DB.CreateTables` creates the tables of several models in an order
  that works, including two models with foreign keys to each other on
  PostgreSQL. A table that refers to a table created later gets its foreign
  keys once every table exists, as Django's schema editor does. The test
  databases of `djangotest` use it, so mutual foreign keys work in tests on
  PostgreSQL too. A dialect provides this through `orm.ForeignKeyDialect`.
- contenttypes: generic relations. A `contenttypes.GenericForeignKey` field
  points a row at an object of any model through a content type and an object
  id, and `contenttypes.GetObject` and `SetObject` read and set it. A
  `contenttypes.GenericRelation[T]` field gives the reverse side:
  `orm.GenericSet` adds, removes, sets and clears the rows, filters and
  `OrderBy` follow it (`Filter("tags__tag", "lion")`), and annotations
  aggregate over it. Its `related_query_name` tag lets the rows filter back
  on the owners (`Filter("bookmarks__url", ...)`), and `orm.Related` lists the
  owners of a row. `PrefetchRelated` loads a generic foreign key, also across
  content types, with one query per content type, and loads a generic
  relation in one query. `orm.Prefetch{QuerySets: ...}` gives a queryset per
  content type, as Django's `GenericPrefetch` does. Deleting an object deletes
  the rows of its generic relations and sends their delete signals.
  `GetOrCreate` and `UpdateOrCreate` take a generic foreign key in their
  defaults. The object id can be an integer or a text column, and the target
  can have an integer, text or UUID primary key. On PostgreSQL, a join from
  an integer primary key to a text object id casts the column, as Django does.
- Commands: `remove_stale_contenttypes` deletes the content types whose model
  no longer exists, with the rows that depend on them, as Django's command
  does. It lists them and asks first unless `--noinput` is given, and
  `--include-stale-apps` also covers apps that are no longer installed. It
  is available when the contenttypes app is installed.
- Forms: `forms.NewGenericInlineFormSet` edits the rows that point at one
  object of any model through a content type field and an object id field,
  as Django's `generic_inlineformset_factory` does. The forms leave out both
  fields, and `Save` sets them on the rows it adds. `WithInstance`,
  `SaveAsNew`, the `QuerySet` of the rows and Django's defaults (three extra
  forms, `CanDelete` and the prefix `<app>-<model>-<ct field>-<fk field>`)
  work as for an inline formset.
- Admin: `admin.GenericTabularInline[T]` and `admin.GenericStackedInline[T]`
  edit the rows of `T` that point at the edited row through a generic foreign
  key, on the add and change pages of any model, as Django's generic inlines
  of `django.contrib.contenttypes.admin` do. `CTField` and `CTFKField` of
  `InlineModelAdmin` name the two fields when they are not `content_type` and
  `object_id`. The checks report a model without a matching generic foreign
  key (`admin.E301` to `admin.E304`). The forms keep the permission guards and
  the check of protected rows of the other inlines, which Django's generic
  inlines leave out.
- ORM: the database functions `orm.Coalesce`, `orm.Concat` and `orm.Cast`, and
  conditional expressions with `orm.Case`, `orm.When` and `Default`, as
  Django's `Coalesce`, `Concat`, `Cast`, `Case` and `When`. They work in
  `Annotate`, `Filter`, `Update`, `OrderBy` through an annotation, the
  aggregates (`Sum(Case(...))`) and the expressions and conditions of
  constraints, which migrations write as Go code. `Cast` takes an
  `orm.Field` whose kind, length and digits give the SQL type; a dialect
  whose `CAST` does not convert as Django's does implements
  `orm.CastDialect`. A condition of `When` that spans a relation joins it
  with a LEFT OUTER JOIN.
- Messages: Django's cookie and fallback storages. Applications keep the
  messages in a signed `messages` cookie and the ones that do not fit into
  2048 bytes in the session, as Django's default `FallbackStorage` does. The
  new setting `MESSAGE_STORAGE` (`fallback`, `cookie` or `session`) selects
  the storage, and the cookie takes the `SESSION_COOKIE_*` attributes and is
  signed with `SECRET_KEY`, in Django's format, so a Django process with the
  same secret reads it. `messages.MiddlewareWithOptions`,
  `messages.NewCookieStorage` and `messages.NewFallbackStorage` select the
  storage outside an application; `messages.Middleware` keeps the session
  storage. `Storage.Update` now takes the response header and returns the
  messages that no storage kept.
- Sessions: the file store of Django's file backend. `SESSION_ENGINE=file`
  keeps each session in a file of the directory `SESSION_FILE_PATH` (the
  temporary directory of the system when empty), named by the session cookie
  name and the key. A session without a custom expiry expires
  `SESSION_COOKIE_AGE` after its file was last written, and `clearsessions`
  deletes the expired files. `New` fails with Django's `ImproperlyConfigured`
  text when the directory does not exist. `sessions.NewFileStore` creates the
  store outside an application.
- Commands: `runserver` restarts the server when a source file changes, as
  Django's auto-reloader does. The command starts the server in a child
  process, checks the files of the packages of the main module, of local
  replacements and of a workspace once per second, and on a change runs
  `go build` with the build settings of the running program and restarts the
  child. When the build fails, the old server keeps running and the build
  errors are printed. `--noreload` turns the reloader off. A program built
  from a list of files (`go run main.go`) or on a machine without the `go`
  command serves without the reloader, after a notice.
- Views: `FormView`, `CreateView`, `UpdateView` and `DeleteView` add a success
  message after valid data, as Django's `SuccessMessageMixin` does. Set
  `SuccessMessage`, whose `%(name)s` placeholders are filled from the cleaned
  data, or `GetSuccessMessage`, which also receives the saved or deleted
  object. The view needs `messages.Middleware`. A `DELETE` request adds no
  message, as in Django.
- ORM: the field tag options `unique_for_date=`, `unique_for_year=` and
  `unique_for_month=` name a date or datetime field, as Django's options of
  the same names do. `FullClean`, `ValidateUnique`, model forms and the admin
  reject a value that another row already has on the same day, in the same
  year or in the same month of that field. A model formset also rejects such
  a value repeated in two of its forms. `Model.DateChecks` lists the checks.
  The database holds no constraint for them, so no migration is needed.
- ORM: the rest of Django's transaction API. `orm.WithoutSavepoint()` makes a
  nested `Atomic` block run without a savepoint, as `atomic(savepoint=False)`
  does. `SetRollback` and `GetRollback` set and read the rollback flag of the
  transaction. `SavepointCreate`, `SavepointRollback` and `SavepointCommit`
  manage savepoints by hand. `OnCommit` runs a callback after the outermost
  block commits, and drops it when the block that registered it rolls back.
  `OnCommitRobust` logs the error of its callback instead of returning it.
  `MarkForRollbackOnError` marks the transaction for rollback when its
  function fails. `ErrTransactionManagement` matches the error of a query in
  a transaction marked for rollback, and of the rollback flag functions
  outside a transaction.
- ORM: `SelectRelated()` without names clears the relations of earlier
  calls, as Django's `select_related(None)` does. Before, it did nothing.
- ORM: `UpdateOrCreate` takes an `orm.CreateDefaults` among its conditions,
  Django's `create_defaults`: the values a created row takes instead of the
  defaults. The many-to-many and generic relation managers take it too.
- ORM: the reverse foreign key manager of `RelatedSet` has `GetOrCreate` and
  `UpdateOrCreate`, which set the foreign key of a created row to the owner.
- ORM: `orm.Abs`, Django's `Abs`, and `Expr.Not`, the negation of a boolean
  expression, Django's `~F()`: `Update(ctx, "is_active",
  orm.F("is_active").Not())` switches every row. The negation of an
  expression that is not boolean fails with an error.
- ORM: `Update` on a `Values` or `ValuesList` queryset, as Django's
  `values().update()`.

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
- **BREAKING**: `User.Groups`, `User.UserPermissions` and `Group.Permissions`
  are many-to-many fields, as in Django, and the models `UserGroup`,
  `UserPermission` and `GroupPermission` are removed. The tables keep their
  names (`auth_user_groups`, `auth_user_user_permissions`,
  `auth_group_permissions`), so only the auth migrations must be generated
  again. Read and change the relations with `orm.Many`, or with
  `User.AddGroups`, `User.AddPermissions` and `Group.AddPermissions`.
  `User.LastLogin` is now a `*time.Time` that is nil for a user who never
  logged in.
- **BREAKING**: the `auth.Hasher` interface has a `SafeSummary` method. A
  custom hasher must implement it.
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
- Admin: `ReadonlyFields` were left out of the change form. They now show as
  values in the form. Dates and times in the changelist show as Django shows
  them (`Jan. 1, 2000`), not in Go's format. A form with errors puts "Error: "
  in the page title, also on the login and password change pages, and the
  error note counts the fields with errors as Django does. Templates that
  override `admin/change_form.html` and read `form_html`, `error_count`,
  `has_errors` or `readonly_fields` must use `adminform` and `errors`.
- Admin: the templates follow Django's `admin/base.html`. Templates that
  override admin pages can use the blocks `extrastyle`, `extrahead`,
  `coltype`, `nav_sidebar`, `nav_global`, `welcome_msg`, `pretitle`,
  `sidebar` and `extrabody`. The app list of the index hides its table head
  visually, and a link in a popup changelist carries `data-popup-opener`.
- `{% now %}` printed the time in the server's local time zone. It now prints
  it in `Settings.TimeZone` (`templates.TimeZone`, UTC by default), as Django
  prints it in `TIME_ZONE`.
- A command error prints as `CommandError: ...` without a stack trace. Exit
  code 1 is a command error and 2 a usage error. The hints of `migrate` and
  `makemigrations` name the program that runs. `startproject` and `startapp`
  print nothing at the default verbosity (`-v 2` lists the files), and
  `djangogo version` prints the bare version.
- **BREAKING**: `admin.DeletedObjects.Items` is now `[]admin.DeletedObject`,
  an HTML item with its cascaded children, and `Protected` holds HTML items.
  A `GetDeletedObjects` hook must build these values. Templates that override
  `admin/delete_confirmation.html` or
  `admin/delete_selected_confirmation.html` read `deleted_objects_html` or
  `deletable_objects_html`, `perms_lacking_html`, `protected_html` and
  `model_count`, as in Django.
- **BREAKING**: a `ModelAdmin` without `ListDisplay` shows one column with the
  label of each object, Django's default `("__str__",)`, instead of a column
  per field. Set `ListDisplay` to keep the columns.
- Admin: the success message after an add, a change or a delete links the
  object, as in Django. The delete page posts to its own URL, and its cancel
  link goes back in the browser history.
- djangotest: `NewDB` and `NewPostgresDB` create the tables in one
  transaction, which sets up a PostgreSQL test database faster.
- Admin: when a `ModelAdmin` or an inline sets `ExcludeFields` or
  `GetExclude`, a `forms.WithExclude` in its `FormOptions` no longer applies.
  The admin's list replaces it, as `ModelAdmin.exclude` replaces the
  `Meta.exclude` of a custom form in Django. Without `ExcludeFields` and
  `GetExclude`, the exclusions of `FormOptions` apply as before. Add the
  names to `ExcludeFields` to keep both. The new `forms.ReplaceExclude`
  option replaces the exclusions of the options before it.
- Templates print a list or a map as Python prints it (`['a', 'b']`,
  `{'k': 1}`), not in Go syntax (`[a b]`, `map[k:1]`). This also holds for
  `stringformat:"s"`, and `stringformat:"r"` gives Python's repr. `make_list`
  splits a value into its characters, also a number. `forloop.parentloop` is
  an empty map outside a nested loop, as in Django. Update templates and
  tests that expect Go's format.
- A template fails to compile with Django's message when a tag or a
  variable comes before `{% extends %}`, when it has two `{% extends %}`
  tags or when two `{% block %}` tags have the same name. Before, such a
  template rendered. Move the tags below `{% extends %}`. A syntax error in
  a child template is now reported before a missing parent.
- `makemigrations` prints the warning about a migration history it cannot
  read from the database to the error output, where Django's warning goes.
  Scripts that read the warning from the standard output must read the
  error output.
- A write that fails inside `Atomic` marks the transaction for rollback on
  both databases. Before, SQLite committed the other writes of the block,
  and PostgreSQL failed every later statement with "current transaction is
  aborted". Code that handles the error of a write and keeps using the
  transaction must wrap the write in its own `Atomic` block. Until the
  innermost enclosing block with a savepoint ends, queries fail with an
  error matched by `ErrTransactionManagement`, and that block then rolls
  back. The writes are `Save`, `Create`, `Update`, `Delete`, `BulkCreate`
  and the add, remove, clear and set methods of the related managers.
- Sessions: a request whose handler does not use the session no longer reads
  it from the store, which saves a database query on such requests with the
  database store. The middleware reads the session when a handler first uses
  it, as Django does. When the store fails, the handler sees an empty
  session and the response becomes a 500; before, the handler did not run.
  A session cookie of an unknown or expired session is deleted only when the
  handler uses the session.
- ORM: `Update` of a many-to-many field or a reverse relation fails with
  "cannot update model field <ManyToManyField: tags> (only concrete fields
  are permitted)", as in Django, instead of "cannot resolve keyword". An
  `Update` on a queryset ordered by an aggregate annotation fails with a
  `FieldError`, as in Django; before, it ignored the order.
- ORM: the "unsupported lookup" error of a key that goes on after a relation
  names the field class Django names: `ForeignKey` for a many-to-many
  relation in either direction, `ManyToOneRel` for a reverse foreign key and
  `OneToOneRel` for a reverse one-to-one relation. Before, it named
  `ManyToManyField` and `OneToOneField`, or `ForeignKey` for a reverse
  foreign key.

### Fixed

- ORM: on SQLite, datetimes saved with different UTC offsets sorted and
  compared by their text, not by their instant, so `OrderBy` and lookups such
  as `__gt` could return the wrong rows. A datetime field read back in the
  local time zone of the process on PostgreSQL and in its saved offset on
  SQLite. The ORM now writes and reads datetimes in UTC on both backends, as
  Django does with `USE_TZ`. Rows written before the upgrade with another
  offset keep their stored text until they are saved again.
- Admin: a disabled field of the change form, such as a field that
  `FormOptions` disables, was saved empty when the form was submitted. The
  change view now gives the form the stored values as initial data, so a
  disabled field keeps its value.
- auth: a user row with a NULL `last_login`, which Django writes for a user who
  never logged in, failed to load. It now loads with a nil `LastLogin`.
- Admin: a duplicate value of a unique field on the add or change page gave a
  500 error; the field now shows the error. The admin form runs the model
  validation as Django's `ModelForm` does, so an inline's clean also sees the
  posted values of the parent row.
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
- `templates.MarkSafe("")` in a map or a slice was true in `{% if %}`. It is
  now false, as an empty string is.
- A savepoint that was rolled back was not released. It is now released.
- On PostgreSQL, a larger `max_length` rewrote the whole table: `AlterField`
  now adds `USING` only when the base type changes. `RemoveField` no longer
  drops dependent objects with `CASCADE`, and `RemoveIndex` uses
  `DROP INDEX IF EXISTS`, as Django does.
- On PostgreSQL, a form showed a date and time in the server's local time
  zone, so saving the form unchanged could move the value. A `DateTimeField`
  now shows its value in UTC, the time zone in which it reads the input.
- `ModelForm.WithInstance(nil)` left the form without an instance. It now
  edits a new instance, as Django's `instance=None` does.
- A model form failed with "cannot assign" when a form field cleaned to a
  value other than a string for a text model field, such as a list. The
  field now stores the string form of the value, as Django's `CharField`
  does. `forms.PopulateStruct` also keeps a related instance on the relation,
  so a parent that is saved after the form was cleaned saves with its key.
- A `ModelChoiceField` over a model whose primary key is a one-to-one field
  used the printed relation as the value of its options. It now uses the
  primary key of the related row.
- Admin: confirming the delete page of an object that protected rows block
  answered 500 Internal Server Error. The page now shows "Cannot delete" with
  the protected objects, also after a POST. A user without the delete
  permission of a model that the deletion cascades to gets 403 Forbidden on
  the POST, and protected objects of such a model count toward the missing
  permissions, as in Django.
- Admin: an object whose label is empty or only whitespace showed as an empty
  link in the changelist, the breadcrumbs, the delete page and the messages.
  It now shows "-", as in Django. The breadcrumbs cut a long label after 18
  words.
- Admin: the changelist escaped a `DisplayColumns` value of type
  `template.HTML`, so a column built with HTML showed its tags as text. It
  now shows the HTML, as Django shows a `SafeString`. A column whose value is
  a date or a time no longer wraps, as in Django.
- ORM: a filter that compared a text field with an integer, such as
  `Filter("title", 9)`, failed on PostgreSQL with "unable to encode". The
  integer is now sent as its text, as Django's `CharField.get_prep_value`
  does.
- ORM: a filter on an annotation whose name holds `__`, such as the default
  name `links__sum` in `Filter("links__sum__isnull", true)`, failed with
  "unsupported lookup". It now finds the annotation, as Django does.
- ORM: `BulkCreate` stored a foreign key that held a parent saved after the
  child was built as NULL, and accepted a parent that was never saved. It now
  takes the primary key of the saved parent and rejects an unsaved one with
  Django's "bulk_create() prohibited to prevent data loss" error.
- ORM: an annotation of `orm.Value` that held an `orm.Decimal`, `orm.Date`,
  `orm.Time` or `orm.UUID` read back as a string. It now reads back as its Go
  type. An `orm.Value` that held a `netip.Addr` failed on SQLite with
  "unsupported type netip.Addr"; it is now sent as its text.
- contenttypes: two processes that created the content type of a model at the
  same moment could leave two rows for it, because the `django_content_type`
  table had no unique constraint on (`app_label`, `model`). The table now has
  Django's constraint. Run `makemigrations` and `migrate` after the upgrade.

### Security

- The admin answers 400 Bad Request when the `_to_field` parameter of a
  changelist, add, change or delete page names a field other than the primary
  key, as Django's `DisallowedModelAdminToField` does. Such a field is never
  shown.
- A value printed in a template through its `String` method was not
  HTML-escaped. Model instances, the post titles of the example and any other
  type with a `String` method are now escaped. Mark trusted HTML with
  `templates.HTMLer` or `html/template.HTML`.
