# Djan-Go-Go -- Milestone 5: Web layer

> Detailed plan for M5: serve real pages with auth. Adds `templates` (pongo2),
> `urls` (Path/Include/Reverse over stdlib ServeMux), `views` (function + generic
> CRUD-ish), `sessions` (signed-cookie + db-backed), and `auth` (User/Group/Permission,
> pbkdf2 hashers, login/logout, login-required guard). Conventions: TDD, conventional
> commits straight to `main`, `gofmt`/`go vet`/`golangci-lint` (0 issues) per commit.
> No em-dashes (use `--`). Go on PATH: `export PATH="$HOME/.local/go/bin:$HOME/go/bin:$PATH"`.

**Goal:** A login-gated page renders via a template and a generic view.

**Exit criteria:** an unauthenticated request to a login-required page redirects to
login (or 403); after logging in (session established), the page renders 200 with
template content driven by a view. `go test ./...`, `go vet`, `golangci-lint` green.

---

## Work packages

### W1 -- templates (pongo2)
Dependency `github.com/flosch/pongo2/v6`. Package `templates`:
- `Engine` wraps a `pongo2.TemplateSet` over one or more template directories (and/or
  an `fs.FS`). `NewEngine(dirs ...string) *Engine`; `Render(name string, ctx map[string]any) (string, error)`; `RenderTo(w io.Writer, name string, ctx map[string]any) error`.
- Register basic tags/filters: `{% csrf_token %}` (renders a hidden input from a ctx
  `csrf_token`), `{% static "path" %}` (prefix with a static URL base), and a `url`
  tag/filter that calls `urls.Reverse`. Keep tag impls small; a missing tag must not
  break the engine. (If a tag is awkward in pongo2 v6, implement it as a global
  function/filter and document.)
- Tests: render a template string/dir with context; csrf and static produce expected
  output.

### W2 -- urls (routing + reverse) over ServeMux
Package `urls`:
- `Route{ Pattern, Name string; Handler http.Handler }`. `Path(pattern string, h http.Handler, name string) Route` (pattern is a Go 1.22 ServeMux pattern, e.g. `GET /articles/{id}/`, or a method-less path). `Include(prefix string, routes ...Route) []Route` (prefix-join patterns, namespacing names with the prefix).
- `Router` builds a `*http.ServeMux` from routes and keeps a `name -> pattern` map.
  `NewRouter(routes ...Route) *Router`; `Router.ServeMux() *http.ServeMux`;
  `Router.Reverse(name string, args ...any) (string, error)` substitutes `{param}`
  placeholders in order (error on arity mismatch / unknown name). A process-global
  default router or a settable resolver lets the template `url` tag call Reverse.
- Tests: register routes, serve via the mux (200, path params), Reverse round-trips
  (named -> URL with args), Include prefixes correctly.

### W3 -- sessions
Package `sessions`:
- `Session` holds a string-keyed map plus a key and a `modified` flag; `Get/Set/Delete/Pop`.
- `Store` interface: `Load(ctx, key string) (*Session, error)`, `Save(ctx, *Session) error`, `Delete(ctx, key) error`, `New() *Session`.
- `SignedCookieStore` (HMAC-SHA256 signed, optionally with a secret from settings;
  value is base64(json(data)) + "." + signature) and `DBStore` (orm-backed table
  `sessions(key TEXT PK, data TEXT, expires DATETIME)`).
- `Middleware(store, cookieName)` loads the session from the request cookie into the
  context, and on response writes the cookie / persists. `Rotate(*Session)` issues a
  new key (session-fixation safe) -- call on login.
- Tests: round-trip a signed cookie (tamper -> rejected), db store save/load, rotation
  changes the key while preserving data.

### W4 -- auth
Package `auth`:
- ORM models: `User{ID; Username (unique); PasswordHash; Email; IsActive; IsStaff;
  IsSuperuser; DateJoined}`, `Group{ID; Name}`, `Permission{ID; Codename; Name;
  ContentType}` (+ a simple user-permission/group link for the PoC; M2M can be
  represented with explicit link models since ORM M2M is not built). Register them in
  an `auth` app (ModelProvider) so migrations/admin pick them up.
- Hashers: pbkdf2-sha256 (Django-format `pbkdf2_sha256$<iter>$<salt>$<b64hash>`).
  `MakePassword(raw string) string`, `CheckPassword(raw, encoded string) bool`,
  `User.SetPassword/CheckPassword`.
- Session auth: `Login(ctx, sess *sessions.Session, u *User)` (rotates + stores user
  id), `Logout(ctx, sess)`, `UserFromSession(ctx, db, sess) (*User, bool)`.
- Guard: `LoginRequired(next http.Handler) http.Handler` (redirects to the login URL
  when no authenticated user in context) and a context helper to fetch the current
  user. A `PermissionRequired(codename)` guard checking the user's permissions.
- Tests: hasher round-trip + Django-format parsing; login sets/rotates session;
  LoginRequired blocks anonymous, allows authenticated; permission check.

### W5 -- views + exit integration
Package `views`:
- Request/response helpers: `Render(w, engine, name, ctx)`, `Redirect(w, r, url)`,
  JSON helper, method dispatch.
- Function views plus a generic `DetailView[T]` and `ListView[T]` that read the ORM
  (`Query[T](db).Get/All`) and render a template with the object(s). Keep generic
  views minimal but real (fetch + render).
- **Exit integration test**: wire a tiny app -- a `templates.Engine`, a `urls.Router`
  with a login page and a `LoginRequired`-wrapped detail page backed by a generic
  view, sessions middleware, and auth. Using `httptest`: an anonymous GET to the
  protected page redirects to login; POST valid credentials -> session set; GET the
  page again -> 200 with the rendered template content. Then the M5 review.

Scope note: full generic CRUD (Create/Update/Delete views with forms) leans on M6
forms; M5 delivers Detail/List + the auth/session/template/url spine and the login
gate. Create/Update/Delete views are completed in M6 alongside ModelForm.
