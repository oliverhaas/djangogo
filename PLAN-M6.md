# Djan-Go-Go -- Milestone 6: Forms + Admin

> Detailed plan for M6: the headline auto-CRUD admin. Adds `forms` (Form + ModelForm
> from `*orm.Model`, widgets, validation, rendering), a CSRF enforcement middleware,
> and `admin` (register a model -> zero-config CRUD list/add/change/delete with a
> ModelAdmin customization surface, auth-gated, pongo2 templates). Conventions: TDD,
> conventional commits straight to `main`, `gofmt`/`go vet`/`golangci-lint` (0 issues)
> per commit. No em-dashes (use `--`). Go on PATH: `export PATH="$HOME/.local/go/bin:$HOME/go/bin:$PATH"`.

**Goal:** Registering a model yields a working admin list + add + change + delete in
the browser.

**Exit criteria:** an httptest-driven flow over a staff-gated admin: the changelist
lists rows, add creates a row (valid form), change edits it, delete removes it; invalid
form input re-renders with errors; CSRF is enforced on unsafe methods. `go test ./...`,
`go vet`, `golangci-lint` green.

---

## Work packages

### W1 -- forms
Package `forms`:
- `Widget` interface: `Render(name, value string, attrs map[string]string) string`. Impls:
  `TextInput`, `Textarea`, `NumberInput`, `CheckboxInput`, `EmailInput`, `PasswordInput`,
  `Select{Choices}`. All HTML-escape values/attrs.
- `Field`: `Name, Label string; Required bool; Widget Widget; HelpText string;
  Kind FieldKind` (Char/Text/Int/Bool/Email/DateTime/Choice) plus per-kind config
  (MaxLength, Choices). `Field.Clean(raw string) (any, error)` parses + validates (required,
  max length, int parse, email regex, bool checkbox, choice membership).
- `Form`: ordered `[]*Field`, bound data (`url.Values`), `errors map[string][]string`,
  `cleaned map[string]any`. `NewForm(fields...)`; `Bind(url.Values)`; `IsValid() bool`
  (cleans all fields, collects errors); `Cleaned() map[string]any`; `Errors()`;
  `Render() string` (as-p: label + widget + field errors). A non-field error list too.
- `ModelForm`: `FromModel(m *orm.Model, excludePK bool) *Form` derives Fields from the
  model's fields (KindChar->CharField with MaxLength, KindText->Textarea, KindInt->
  IntegerField, KindBool->CheckboxInput, KindDateTime->DateTime, FK column->IntegerField
  for now), skipping the auto PK. `PopulateStruct(cleaned, dest any) error` sets struct
  fields by name from cleaned data via reflection (for Save). `FromStruct(m, obj any)
  *Form` builds a form pre-filled from an existing instance (for change).
- Tests: each widget renders + escapes; field cleaning (valid/invalid/required/maxlen/
  email/int/choice); form bind+validate+render; ModelForm derivation from a model; populate
  a struct from cleaned data.

### W2 -- CSRF enforcement (closes the M5 gap)
Package `csrf` (or `middleware`):
- `Middleware(next http.Handler) http.Handler`: ensure a per-session CSRF token (stored
  in the session via `sessions.FromContext`; generate with crypto/rand if absent) and put
  it in the request context. On unsafe methods (POST/PUT/PATCH/DELETE), read the submitted
  token from form field `csrfmiddlewaretoken` or header `X-CSRFToken`, constant-time
  compare to the session token; 403 on mismatch/absent. Safe methods (GET/HEAD/OPTIONS)
  pass through.
- `Token(ctx) string` to fetch the token (so views/templates inject it as `csrf_token`).
- Tests: GET seeds a token; POST without/with-wrong token -> 403; POST with the right
  token -> passes; header form works; safe methods bypass.

### W3 -- admin core + CRUD views + templates
Package `admin`. The admin must operate on models known at registration time via a
generic `Register[T]` that captures T into type-erased operations (the ORM stays
generic-only):
- `ModelAdmin`: customization surface -- `ListDisplay []string` (columns; default all
  displayable), `SearchFields []string`, `Ordering []string`, `ReadonlyFields []string`,
  `ExcludeFields []string`. (list_filter/inlines/bulk-actions may be partial/noted.)
- `AdminSite`: holds registered entries + a `*templates.Engine` + the `*orm.DB`.
  `NewAdminSite(db, engine) *AdminSite`; `Register[T any](site, ModelAdmin)` builds an
  `entry{ model *orm.Model; admin ModelAdmin; ops modelOps }` where `modelOps` are
  closures over T: `all(ctx) ([]any, error)`, `get(ctx, pk int64) (any, error)`,
  `create(ctx, obj any) error`, `update(ctx, obj any) error`, `delete(ctx, pk int64)
  error`, `newPtr() any`. Reflection maps struct<->display rows and form data.
- Routing: `AdminSite.Routes(prefix string) []urls.Route` exposing
  `GET <prefix>/` (index: list models), `GET <prefix>/<model>/` (changelist),
  `GET|POST <prefix>/<model>/add/`, `GET|POST <prefix>/<model>/<pk>/change/`,
  `GET|POST <prefix>/<model>/<pk>/delete/`. All wrapped so only `IsStaff` users pass
  (redirect to a login URL otherwise) -- reuse `auth.CurrentUser`.
- Views render pongo2 templates (embedded via `embed.FS` in the admin package, or a
  templates dir): `base.html`, `index.html`, `changelist.html`, `change_form.html`
  (used for add + change, driven by a ModelForm), `delete_confirm.html`. Inject
  `csrf_token` into form templates.
- add/change use `forms.ModelForm.FromModel`/`FromStruct`; on valid POST, populate a
  `newPtr()` struct and `create`/`update`; on invalid, re-render with errors. delete
  shows a confirm page then `delete`s on POST.
- Tests: registering a model builds the entry + routes; changelist lists seeded rows
  with list_display columns; add (GET form, POST valid -> row created, POST invalid ->
  errors); change (GET prefilled, POST -> updated); delete (GET confirm, POST -> gone);
  a non-staff user is redirected; CSRF enforced on the POSTs.

### W4 -- admin exit integration + review
An httptest end-to-end: build a sqlite DB + auth + sessions + CSRF + an AdminSite with
a demo model registered; log in a staff user; drive changelist -> add -> change ->
delete through real HTTP requests (with CSRF tokens and the session cookie), asserting
the rendered HTML and the resulting DB state. Then the M6 review (security: staff
gating, CSRF, no SQL injection via admin params, template escaping).

Scope note: list_filter, search execution, pagination, inlines, and FK/M2M chooser
widgets are implemented where cheap and clearly noted where slimmed; the exit criterion
is a working list/add/change/delete for a registered model, auth-gated and CSRF-safe.
