# Djan-Go-Go -- Milestone 8: Django-fidelity pass

> Closes the gaps an adversarial review found between the framework and a "typical
> Django project": the example must read like a Django blog (admin + models +
> templates + a real form). Conventions unchanged: TDD (red, green, commit), one
> conventional commit per work package, `gofmt`/`go vet`/`golangci-lint` clean,
> no em/en-dashes (use `--`). `export PATH="$HOME/.local/go/bin:$HOME/go/bin:$PATH"`.

**Goal:** Make a Django developer's muscle memory work: snake_case template fields,
`__str__` labels, a `createsuperuser`-reachable admin, model `default=`/`auto_now_add`/
`choices`, FK `<select>` editing, a real `ModelForm`, `{% url %}` reverse, and a
scaffold/runserver that actually serves an admin. Then rebuild `examples/blog` as a
typical Post+Comment blog with a public comment form.

**Exit criteria:** `examples/blog` is a Post+Comment blog whose templates use
`{{ post.title }}`/`{% url %}`, whose detail page renders a `{{ form }}` + `{% csrf_token %}`
comment form that persists on POST, and whose admin (reachable via `createsuperuser`)
lists both models with a FK dropdown. Whole suite green on SQLite and Postgres;
lint/vet/gofmt clean.

---

## Locked shared contracts

- **`__str__`** is Go's `fmt.Stringer`. `orm.Label(*Model, any) string` returns
  `String()` when implemented, else Django's default `"<Model> object (<pk>)"`.
  `type orm.Stringer = fmt.Stringer` so models need no djangogo import to implement it.
- **Template object** is `templates.ModelMap` (a `map[string]any` with a `String()`
  method). `templates.ModelContext(*orm.Model, instance) ModelMap` exposes scalar
  fields under snake_case `Field.Column` keys, plus `id`/`pk` aliases and the
  `__str__` label. pongo2 resolves map keys by exact string (`variable.go:298`), so
  `{{ post.title }}` works; bare `{{ post }}` renders the label via `fmt.Stringer`.
  Relations are deferred (skip `f.Rel != nil`) in v1.
- **Clock seam**: `orm.DB.Now func() time.Time` (nil => `time.Now().UTC()`), used by
  `auto_now`/`auto_now_add` so tests are deterministic and parallel-safe (no global).
- **FK select seam**: forms map an FK to a `ChoiceField`+`Select` with *empty* choices;
  the admin (which holds `*orm.DB`) populates `(pk, orm.Label)` options before render
  and validates the submitted pk -> a form error, not a 500.

---

## Work packages (one commit each)

**WP1 -- `__str__` + snake_case templates.** New `orm/label.go` (`Stringer`, `Label`,
`defaultLabel`/`pkInt`). New `templates/model.go` (`ModelMap`, `ModelContext`).
`views/generic.go`: wrap Detail/List objects via `ModelContext`, add `object_list`
alias. `admin/views.go`+`display.go`: changelist row label and delete confirmation use
`orm.Label`. (Closes review #2 template PascalCase, #4 `__str__`.)

**WP2 -- `createsuperuser`.** New `auth/create.go` (`CreateUser`/`CreateSuperuser`,
`ErrUserExists`/`ErrEmptyUsername`). `application.go`: add `In io.Reader` (default
`os.Stdin`), register command. New `createsuperuser.go` (interactive prompts +
`--username/--email/--noinput` + `DJANGO_SUPERUSER_PASSWORD`). (Closes review #3 admin
unreachable.)

**WP3 -- model-field gaps.** `orm/field.go`: `HasDefault/Default/AutoNow/AutoNowAdd/
Choices` + tag arms (`default=`, `auto_now`, `auto_now_add`) + `parseDefault` +
mutual-exclusion check; reject all three on relation fields. `orm/meta.go`: `Choice`
+ `withChoices` (model method hook). `orm/db.go`: `Now`/`now()`. `orm/execute.go`:
`applyDefaults`+`applyAutoNow` on Create, `auto_now` injection on Update.
`orm/registry.go`: apply `withChoices`. `forms/modelform.go`: `KindChar`+choices ->
`ChoiceField`+`Select`. No DDL/migration-state change (Go-side defaults, runtime
timestamps, no DB enum -- matches Django). (Closes review #6.)

**WP4 -- FK `<select>`.** `forms/modelform.go`: FK -> `ChoiceField`+`Select` (empty
choices). `forms/form.go`: `SetChoices(field, [][2]string)`; `PopulateStruct` FK branch
parses string|int64 -> int64 SetPK. `admin/views.go`: populate FK options from the
related model via `orm.Label`, invalid pk -> form error; `admin/display.go`: FK cell
shows label. (Closes review #6 FK dropdown.)

**WP5 -- `ModelForm` options.** New `forms/modelform_options.go`
(`WithFields/WithExclude/WithLabel/WithWidget/WithHelpText/WithRequired`). `FromModel`
gains `opts ...ModelFormOption` (back-compatible). Refactor admin `formFor` onto
`WithExclude`. (Closes review #5 ModelForm.)

**WP6 -- `{% url %}` resolver.** `templates/engine.go`: `resolver` field +
`SetResolver` injected into the render paths. `templates/tags.go`: prefer per-render
context resolver over the global. `urls/router.go`: fix `placeholderRe` so the `/{$}`
anchor is not treated as a `{param}` (reverse of `post-list` must yield `/`).

**WP7 -- runserver mounts URLs+admin; scaffold batteries.** `apps/config.go`:
`URLProvider`. `application.go`: build the real `app.Handler` from app `URLs()` + a
mounted admin (when `app.DB != nil`) wrapped in sessions->csrf->auth, `SetResolver`.
`scaffold`: `Project()` emits `urls.go`+`admin.go`; `appURLs`/`appAdmin` become real
(a served index route and a working `admin.Register`). (Closes review #7 dead scaffold.)

**WP8 -- rebuild `examples/blog`.** `blog.go`: add `Comment{ID, Post FK[Post], Name,
Body type=text, CreatedAt auto_now_add}`, `Post.String()`/`Comment.String()`.
`main.go`: register Comment, `admin.Register[Comment]` (FK dropdown), `SetResolver`,
swap detail route to a `postDetailView` (GET renders + POST validates/saves a comment
via a `ModelForm`, PRG). `views.go`: the handler. Templates: snake_case + `{% url %}`
+ comment form (`{{ form_html|safe }}` + `{% csrf_token %}` + `{% for %}{% empty %}`).
Tests: form render, persist+appear, invalid->errors, no-CSRF->403, `{% url %}` resolves.

**Final.** Adversarial review workflow ("is this now a typical Django project?"),
dual-dialect (`go test ./...` + Postgres DSN) + race + lint, README "Known limitations"
refresh, memory update.
