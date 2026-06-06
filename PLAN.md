# Djan-Go-Go Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a Go web framework that reproduces Django's developer experience (one model definition drives the ORM, migrations, admin, and forms) on a pure-Go, single-binary foundation.

**Architecture:** Idiomatic Go packages mirroring Django's mental model (`conf`, `apps`, `manage`, `orm`, `migrations`, `auth`, `urls`, `views`, `forms`, `templates`, `admin`). The keystone is a reflect-at-startup, immutable model-metadata registry that every subsystem reads. Built in 7 milestones; this plan details Milestone 1 (the Spine) task-by-task and sketches the rest.

**Tech Stack:** Go 1.26, `database/sql` + pgx/sqlite drivers (later milestones), stdlib `net/http` (Go 1.22+ ServeMux), [pongo2](https://github.com/flosch/pongo2) for templates. MIT. Design spec: `../ideas/packages/go/djangogo.md` (in the sibling `ideas` repo).

**Conventions for every task:**
- TDD: write the failing test, run it red, implement the minimum, run it green, commit.
- Tests live beside code in the same package (`foo.go` + `foo_test.go`, `package foo`).
- Run all commands from the repo root (`~/e1+/djangogo`) with Go on PATH (`export PATH="$HOME/.local/go/bin:$PATH"`).
- Conventional commits. One commit per task.

---

## Milestone roadmap (high level)

Each milestone produces working, demoable software and may later be split into its own detailed plan.

### M1 -- Spine (detailed below)
**Goal:** A bootable application with settings, an app registry, a management-command dispatcher, and a working `runserver`.
**Deliverables:** `conf`, `apps`, `manage` packages; `djangogo.Application` wiring them; `runserver` + `version` commands; the `cmd/djangogo` CLI serving a hello page.
**Exit criteria:** `go run ./cmd/djangogo runserver` serves HTTP 200; `go test ./...` green; `go vet` and `golangci-lint` clean.

### M2 -- ORM core
**Goal:** Define a model struct and do CRUD against SQLite.
**Deliverables:** `orm` package: field kinds (`Char`, `Text`, `Int`, `Bool`, `DateTime`), the reflect-at-startup `Registry` producing an immutable `*orm.Model` (fields, columns, PK, table name) from `struct + orm:"..."` tags and an optional `Meta()` hook; a lazy, clone-per-method `QuerySet` (`Filter`/`OrderBy`/`Limit`/`All`/`Get`/`Count`/`Exists`/`Create`/`Update`/`Delete`) with `__`-lookups; a `sqlite` backend (compiler + executor over `database/sql`).
**Exit criteria:** Register a model, `Create`/`Get`/`Filter`/`All` round-trip against an in-memory SQLite DB; all green.

### M3 -- Migrations
**Goal:** `makemigrations` + `migrate` parity with the validated go-django spike.
**Deliverables:** `migrations` package: reflect current `Registry` state, diff against deserialized prior state, emit typed ops (`CreateModel`/`AddField`/`AlterField`/`RemoveField`/`AddIndex`), topo-sort an FK-derived dependency graph, write editable Go migration files (per-app, linear history), apply in a transaction against a tracking table, SQLite DDL (incl. temp-table rebuild for `AlterField`).
**Exit criteria:** `manage makemigrations` writes a migration for a new model; `manage migrate` builds the schema; re-running detects no changes.

### M4 -- ORM relations + PostgreSQL
**Goal:** Relations, eager loading, transactions, and a second backend.
**Deliverables:** `FK[T]`/`O2O[T]`/`M2M[T]` + reverse managers; `select_related` (JOIN) and `prefetch_related` (batched `IN`); context-bound transactions with nesting + panic-recovery rollback; pre/post save/delete signals; PostgreSQL backend (pgx, `$N` placeholders, RETURNING); integration tests run against both dialects (testcontainers in CI).
**Exit criteria:** Relation traversal + prefetch work on SQLite and Postgres; dual-dialect tests green.

### M5 -- Web layer
**Goal:** Serve real pages with auth.
**Deliverables:** `urls` (`Path`/`RePath`/`Include`/`Reverse` over ServeMux), `views` (function + generic List/Detail/Create/Update/Delete), `templates` (pongo2 loader + per-app dirs + `{% csrf_token %}`/`{% static %}`/`url` tags), `auth` (User with is_staff/is_superuser, Group, Permission with content_type+codename, pbkdf2 hashers, login/logout, decorators/mixins, wired permission checks), `sessions` (db-backed + signed-cookie, fixation-safe rotation).
**Exit criteria:** A login-gated page renders via a template and a generic view.

### M6 -- Forms + Admin
**Goal:** The headline auto-CRUD admin.
**Deliverables:** `forms` (`Form` + `ModelForm` derived from `*orm.Model`, validators, widgets, rendering); `admin` (register a model -> zero-config CRUD list/add/change/delete with a `ModelAdmin`-style customization surface: `list_display`/`list_filter`/`search_fields`/`ordering`/`readonly_fields`/inlines/bulk actions; FK/M2M chooser widgets; auth-gated; pongo2 templates).
**Exit criteria:** Registering a model yields a working admin list + add + edit + delete in the browser.

### M7 -- Fidelity + polish
**Goal:** Measured Django fidelity and a real onboarding path.
**Deliverables:** Django-as-oracle differential test harness (CI-only Python emits golden template renders / migration SQL / query SQL; Go asserts equivalence); `startproject`/`startapp` scaffolding templates; docs; example apps.
**Exit criteria:** Differential tests pass in CI; `djangogo startproject` produces a runnable project.

---

## Milestone 1: The Spine

### File structure

| File | Responsibility |
|---|---|
| `conf/settings.go` | `Settings` struct, default application, validation (`Check`) |
| `conf/active.go` | Process-wide active settings (`Configure`/`Active`) for the Django-style global accessor |
| `conf/settings_test.go` | Tests for defaults, validation, Configure/Active |
| `apps/config.go` | `Config` interface + optional `Initializer` lifecycle hook |
| `apps/registry.go` | Ordered app `Registry` (Register/Get/Names/Ready) |
| `apps/registry_test.go` | Tests for registration, duplicates, ordered Ready |
| `manage/command.go` | `Command` interface |
| `manage/registry.go` | Command `Registry` + `Execute` dispatch + usage listing |
| `manage/registry_test.go` | Tests for dispatch, unknown command, usage output |
| `application.go` (pkg `djangogo`) | `Application` wiring settings + apps + commands + root handler |
| `runserver.go` (pkg `djangogo`) | `runserverCommand` + `defaultHandler` |
| `version.go` (pkg `djangogo`) | `versionCommand` |
| `application_test.go` (pkg `djangogo`) | Tests for `New`, `Execute`, default handler, version output |
| `cmd/djangogo/main.go` | CLI entrypoint building a demo `Application` (replaces placeholder) |

Import direction (no cycles): `djangogo` -> {`conf`, `apps`, `manage`}; the three sub-packages import only stdlib.

---

### Task 1: `conf.Settings` with defaults and validation

**Files:**
- Create: `conf/settings.go`
- Test: `conf/settings_test.go`

- [ ] **Step 1: Write the failing test**

```go
// conf/settings_test.go
package conf

import "testing"

func TestSettingsApplyDefaults(t *testing.T) {
	s := Settings{SecretKey: "x"}
	s.applyDefaults()

	if s.Host != "127.0.0.1" {
		t.Errorf("Host = %q, want 127.0.0.1", s.Host)
	}
	if s.Port != "8000" {
		t.Errorf("Port = %q, want 8000", s.Port)
	}
	if len(s.AllowedHosts) == 0 {
		t.Error("AllowedHosts should default to a non-empty list")
	}
}

func TestSettingsCheck(t *testing.T) {
	if err := (&Settings{SecretKey: "set"}).Check(); err != nil {
		t.Errorf("valid settings returned error: %v", err)
	}
	if err := (&Settings{}).Check(); err == nil {
		t.Error("missing SecretKey should be an error")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./conf/ -run TestSettings -v`
Expected: FAIL (build error: `undefined: Settings`).

- [ ] **Step 3: Write minimal implementation**

```go
// conf/settings.go
// Package conf holds Djan-Go-Go application settings (the analog of Django's settings module).
package conf

import "errors"

// Settings is the typed configuration for an application. Later milestones add
// Databases, Templates, and an app-extensible registry; the Spine keeps it minimal.
type Settings struct {
	Debug         bool
	SecretKey     string
	AllowedHosts  []string
	InstalledApps []string
	Host          string
	Port          string
}

const (
	defaultHost = "127.0.0.1"
	defaultPort = "8000"
)

// applyDefaults fills empty fields with their defaults. Called by Configure.
func (s *Settings) applyDefaults() {
	if s.Host == "" {
		s.Host = defaultHost
	}
	if s.Port == "" {
		s.Port = defaultPort
	}
	if len(s.AllowedHosts) == 0 {
		s.AllowedHosts = []string{"localhost", "127.0.0.1"}
	}
}

// Check validates settings at boot. It returns a non-nil error for misconfiguration.
func (s *Settings) Check() error {
	if s.SecretKey == "" {
		return errors.New("conf: SecretKey must be set")
	}
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./conf/ -run TestSettings -v`
Expected: PASS (both tests).

- [ ] **Step 5: Commit**

```bash
git add conf/settings.go conf/settings_test.go
git commit -m "feat(conf): add Settings with defaults and validation"
```

---

### Task 2: Process-wide active settings (`Configure`/`Active`)

**Files:**
- Create: `conf/active.go`
- Test: append to `conf/settings_test.go`

- [ ] **Step 1: Write the failing test**

```go
// append to conf/settings_test.go
func TestConfigureAndActive(t *testing.T) {
	got := Configure(Settings{SecretKey: "k"})
	if got.Host != "127.0.0.1" {
		t.Errorf("Configure should apply defaults; Host = %q", got.Host)
	}
	if Active() != got {
		t.Error("Active() should return the configured settings")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./conf/ -run TestConfigureAndActive -v`
Expected: FAIL (build error: `undefined: Configure`, `undefined: Active`).

- [ ] **Step 3: Write minimal implementation**

```go
// conf/active.go
package conf

// active is the process-wide settings object, set by Configure. It gives the
// Django-familiar global accessor (conf.Active()). The application configures it
// once at boot; tests may reconfigure freely.
var active *Settings

// Configure sets the active settings (applying defaults) and returns them.
func Configure(s Settings) *Settings {
	s.applyDefaults()
	active = &s
	return active
}

// Active returns the configured settings. It panics if Configure was never called,
// which always indicates a boot-ordering bug rather than a runtime condition.
func Active() *Settings {
	if active == nil {
		panic("conf: settings not configured; call conf.Configure first")
	}
	return active
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./conf/ -v`
Expected: PASS (all conf tests).

- [ ] **Step 5: Commit**

```bash
git add conf/active.go conf/settings_test.go
git commit -m "feat(conf): add process-wide Configure/Active accessor"
```

---

### Task 3: `apps.Config` interface and `Registry`

**Files:**
- Create: `apps/config.go`, `apps/registry.go`
- Test: `apps/registry_test.go`

- [ ] **Step 1: Write the failing test**

```go
// apps/registry_test.go
package apps

import "testing"

type fakeApp struct{ name string }

func (f fakeApp) Name() string { return f.name }

func TestRegistryRegisterAndGet(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(fakeApp{"blog"}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := r.Register(fakeApp{"shop"}); err != nil {
		t.Fatalf("Register: %v", err)
	}

	got, ok := r.Get("blog")
	if !ok || got.Name() != "blog" {
		t.Fatalf("Get(blog) = %v, %v", got, ok)
	}

	want := []string{"blog", "shop"} // registration order preserved
	names := r.Names()
	if len(names) != 2 || names[0] != want[0] || names[1] != want[1] {
		t.Errorf("Names() = %v, want %v", names, want)
	}
}

func TestRegistryDuplicate(t *testing.T) {
	r := NewRegistry()
	_ = r.Register(fakeApp{"blog"})
	if err := r.Register(fakeApp{"blog"}); err == nil {
		t.Error("duplicate app name should error")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./apps/ -v`
Expected: FAIL (build error: `undefined: NewRegistry`).

- [ ] **Step 3: Write minimal implementation**

```go
// apps/config.go
// Package apps provides the application registry (Django's AppConfig + apps registry).
package apps

// Config is the minimal app contract: every app reports a unique name.
type Config interface {
	Name() string
}

// Initializer is an optional hook run after all apps are registered (Django's ready()).
type Initializer interface {
	Ready() error
}
```

```go
// apps/registry.go
package apps

import "fmt"

// Registry holds the installed apps in registration order (the INSTALLED_APPS analog).
type Registry struct {
	order  []string
	byName map[string]Config
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{byName: make(map[string]Config)}
}

// Register adds an app. It errors on an empty or duplicate name.
func (r *Registry) Register(c Config) error {
	name := c.Name()
	if name == "" {
		return fmt.Errorf("apps: config %T has an empty Name()", c)
	}
	if _, dup := r.byName[name]; dup {
		return fmt.Errorf("apps: duplicate app %q", name)
	}
	r.byName[name] = c
	r.order = append(r.order, name)
	return nil
}

// Get returns the app registered under name.
func (r *Registry) Get(name string) (Config, bool) {
	c, ok := r.byName[name]
	return c, ok
}

// Names returns app names in registration order.
func (r *Registry) Names() []string {
	out := make([]string, len(r.order))
	copy(out, r.order)
	return out
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./apps/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add apps/config.go apps/registry.go apps/registry_test.go
git commit -m "feat(apps): add Config interface and ordered Registry"
```

---

### Task 4: `Registry.Ready` runs `Initializer` hooks in order

**Files:**
- Modify: `apps/registry.go`
- Test: append to `apps/registry_test.go`

- [ ] **Step 1: Write the failing test**

```go
// append to apps/registry_test.go
type readyApp struct {
	name string
	log  *[]string
}

func (a readyApp) Name() string { return a.name }
func (a readyApp) Ready() error { *a.log = append(*a.log, a.name); return nil }

func TestRegistryReadyOrder(t *testing.T) {
	var log []string
	r := NewRegistry()
	_ = r.Register(readyApp{"first", &log})
	_ = r.Register(fakeApp{"plain"}) // no Ready(), must be skipped without error
	_ = r.Register(readyApp{"second", &log})

	if err := r.Ready(); err != nil {
		t.Fatalf("Ready: %v", err)
	}
	if len(log) != 2 || log[0] != "first" || log[1] != "second" {
		t.Errorf("Ready order = %v, want [first second]", log)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./apps/ -run TestRegistryReadyOrder -v`
Expected: FAIL (build error: `r.Ready undefined`).

- [ ] **Step 3: Write minimal implementation**

```go
// append to apps/registry.go
// Ready runs Ready() on every app implementing Initializer, in registration order.
func (r *Registry) Ready() error {
	for _, name := range r.order {
		if init, ok := r.byName[name].(Initializer); ok {
			if err := init.Ready(); err != nil {
				return fmt.Errorf("apps: %s.Ready: %w", name, err)
			}
		}
	}
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./apps/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add apps/registry.go apps/registry_test.go
git commit -m "feat(apps): run Initializer.Ready hooks in registration order"
```

---

### Task 5: `manage.Command` interface and dispatching `Registry`

**Files:**
- Create: `manage/command.go`, `manage/registry.go`
- Test: `manage/registry_test.go`

- [ ] **Step 1: Write the failing test**

```go
// manage/registry_test.go
package manage

import (
	"bytes"
	"strings"
	"testing"
)

type fakeCmd struct {
	name string
	ran  *[]string
}

func (c fakeCmd) Name() string { return c.name }
func (c fakeCmd) Help() string { return "help for " + c.name }
func (c fakeCmd) Run(args []string) error {
	*c.ran = append(*c.ran, c.name+":"+strings.Join(args, ","))
	return nil
}

func TestExecuteDispatches(t *testing.T) {
	var ran []string
	r := NewRegistry()
	if err := r.Register(fakeCmd{"runserver", &ran}); err != nil {
		t.Fatalf("Register: %v", err)
	}

	if err := r.Execute([]string{"runserver", "--port", "9000"}); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(ran) != 1 || ran[0] != "runserver:--port,9000" {
		t.Errorf("ran = %v", ran)
	}
}

func TestExecuteUnknown(t *testing.T) {
	r := NewRegistry()
	if err := r.Execute([]string{"nope"}); err == nil {
		t.Error("unknown command should error")
	}
}

func TestExecuteNoArgsPrintsUsage(t *testing.T) {
	var buf bytes.Buffer
	r := NewRegistry()
	r.Out = &buf
	var ran []string
	_ = r.Register(fakeCmd{"runserver", &ran})

	if err := r.Execute(nil); err != nil {
		t.Fatalf("Execute(nil): %v", err)
	}
	if !strings.Contains(buf.String(), "runserver") {
		t.Errorf("usage output missing command name: %q", buf.String())
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./manage/ -v`
Expected: FAIL (build error: `undefined: NewRegistry`).

- [ ] **Step 3: Write minimal implementation**

```go
// manage/command.go
// Package manage provides the management-command dispatcher (Django's manage.py).
package manage

// Command is a management subcommand.
type Command interface {
	Name() string
	Help() string
	Run(args []string) error
}
```

```go
// manage/registry.go
package manage

import (
	"fmt"
	"io"
	"os"
	"sort"
)

// Registry holds management commands and dispatches them by name.
type Registry struct {
	byName map[string]Command
	Out    io.Writer // where usage is printed; defaults to os.Stdout
}

// NewRegistry returns an empty registry writing usage to stdout.
func NewRegistry() *Registry {
	return &Registry{byName: make(map[string]Command), Out: os.Stdout}
}

// Register adds a command. It errors on a duplicate name.
func (r *Registry) Register(c Command) error {
	name := c.Name()
	if _, dup := r.byName[name]; dup {
		return fmt.Errorf("manage: duplicate command %q", name)
	}
	r.byName[name] = c
	return nil
}

// Names returns the registered command names, sorted.
func (r *Registry) Names() []string {
	out := make([]string, 0, len(r.byName))
	for n := range r.byName {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// Execute dispatches args[0] to the matching command, passing the rest as its args.
// With no args it prints usage and returns nil.
func (r *Registry) Execute(args []string) error {
	if len(args) == 0 {
		r.printUsage()
		return nil
	}
	cmd, ok := r.byName[args[0]]
	if !ok {
		return fmt.Errorf("manage: unknown command %q (run with no arguments to list commands)", args[0])
	}
	return cmd.Run(args[1:])
}

func (r *Registry) printUsage() {
	fmt.Fprintln(r.Out, "usage: djangogo <command> [args]")
	fmt.Fprintln(r.Out, "\nAvailable commands:")
	for _, n := range r.Names() {
		fmt.Fprintf(r.Out, "  %-16s %s\n", n, r.byName[n].Help())
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./manage/ -v`
Expected: PASS (all three tests).

- [ ] **Step 5: Commit**

```bash
git add manage/command.go manage/registry.go manage/registry_test.go
git commit -m "feat(manage): add Command interface and dispatching Registry"
```

---

### Task 6: `version` command

**Files:**
- Create: `version.go` (package `djangogo`)
- Test: `version_test.go` (package `djangogo`)

- [ ] **Step 1: Write the failing test**

```go
// version_test.go
package djangogo

import (
	"bytes"
	"strings"
	"testing"
)

func TestVersionCommand(t *testing.T) {
	var buf bytes.Buffer
	cmd := versionCommand{out: &buf}

	if cmd.Name() != "version" {
		t.Errorf("Name() = %q", cmd.Name())
	}
	if err := cmd.Run(nil); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(buf.String(), Version) {
		t.Errorf("output %q does not contain version %q", buf.String(), Version)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test . -run TestVersionCommand -v`
Expected: FAIL (build error: `undefined: versionCommand`).

- [ ] **Step 3: Write minimal implementation**

```go
// version.go
package djangogo

import (
	"fmt"
	"io"
)

// versionCommand prints the framework version.
type versionCommand struct {
	out io.Writer
}

func (versionCommand) Name() string { return "version" }
func (versionCommand) Help() string { return "Print the Djan-Go-Go version" }

func (c versionCommand) Run(_ []string) error {
	fmt.Fprintf(c.out, "djangogo %s\n", Version)
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test . -run TestVersionCommand -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add version.go version_test.go
git commit -m "feat: add version management command"
```

---

### Task 7: `runserver` command and default handler

**Files:**
- Create: `runserver.go` (package `djangogo`)
- Test: `runserver_test.go` (package `djangogo`)

The serve loop (`ListenAndServe` + graceful shutdown) is covered by a manual smoke test in Task 9. Here we unit-test the parts that can be tested without binding a port: the default handler (via `httptest`) and the command metadata.

- [ ] **Step 1: Write the failing test**

```go
// runserver_test.go
package djangogo

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDefaultHandlerServesOK(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	defaultHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Djan-Go-Go") {
		t.Errorf("body = %q, want it to mention Djan-Go-Go", rec.Body.String())
	}
}

func TestRunserverMetadata(t *testing.T) {
	c := &runserverCommand{}
	if c.Name() != "runserver" {
		t.Errorf("Name() = %q", c.Name())
	}
	if c.Help() == "" {
		t.Error("Help() should be non-empty")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test . -run 'TestDefaultHandler|TestRunserver' -v`
Expected: FAIL (build error: `undefined: defaultHandler`, `undefined: runserverCommand`).

- [ ] **Step 3: Write minimal implementation**

```go
// runserver.go
package djangogo

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// defaultHandler is the placeholder root handler served by runserver until the
// urls/views layer lands (Milestone 5). It answers GET / with a liveness line.
func defaultHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprintf(w, "Djan-Go-Go %s is running.\n", Version)
	})
	return mux
}

// runserverCommand starts the development HTTP server with graceful shutdown.
type runserverCommand struct {
	app *Application
}

func (*runserverCommand) Name() string { return "runserver" }
func (*runserverCommand) Help() string { return "Start the development HTTP server" }

func (c *runserverCommand) Run(_ []string) error {
	addr := c.app.Settings.Host + ":" + c.app.Settings.Port
	srv := &http.Server{
		Addr:              addr,
		Handler:           c.app.Handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		fmt.Fprintf(c.app.Out, "Djan-Go-Go development server at http://%s/  (Ctrl-C to quit)\n", addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	}
}
```

> Note: `runserverCommand` references `c.app.Settings`, `c.app.Handler`, and `c.app.Out`. The `Application` type providing these is built in Task 8; this file compiles only once Task 8 lands. The Task 7 test exercises `defaultHandler()` and the metadata methods, which do not touch `app`, so build Task 8 immediately after if `go test .` reports `undefined: Application`. (If you prefer strict per-task green, temporarily stub `type Application struct{ Settings *conf.Settings; Handler http.Handler; Out io.Writer }` and remove it in Task 8.)

- [ ] **Step 4: Run test to verify it passes**

Run: `go test . -run 'TestDefaultHandler|TestRunserver' -v`
Expected: PASS (after Task 8 provides `Application`; see note).

- [ ] **Step 5: Commit**

```bash
git add runserver.go runserver_test.go
git commit -m "feat: add runserver command and default handler"
```

---

### Task 8: `djangogo.Application` wiring it all together

**Files:**
- Create: `application.go` (package `djangogo`)
- Test: `application_test.go` (package `djangogo`)

- [ ] **Step 1: Write the failing test**

```go
// application_test.go
package djangogo

import (
	"testing"

	"github.com/oliverhaas/djangogo/conf"
)

func TestNewWiresRegistries(t *testing.T) {
	app, err := New(conf.Settings{SecretKey: "k"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if app.Settings.Host != "127.0.0.1" {
		t.Errorf("defaults not applied; Host = %q", app.Settings.Host)
	}
	if app.Handler == nil {
		t.Error("Handler should be set")
	}
	// built-in commands are registered
	names := app.Commands.Names()
	found := map[string]bool{}
	for _, n := range names {
		found[n] = true
	}
	if !found["runserver"] || !found["version"] {
		t.Errorf("built-in commands missing: %v", names)
	}
}

func TestNewRejectsBadSettings(t *testing.T) {
	if _, err := New(conf.Settings{}); err == nil {
		t.Error("New should reject settings without a SecretKey")
	}
}

func TestExecuteRunsCommand(t *testing.T) {
	app, err := New(conf.Settings{SecretKey: "k"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := app.Execute([]string{"version"}); err != nil {
		t.Errorf("Execute(version): %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test . -run 'TestNew|TestExecute' -v`
Expected: FAIL (build error: `undefined: New`).

- [ ] **Step 3: Write minimal implementation**

```go
// application.go
package djangogo

import (
	"io"
	"net/http"
	"os"

	"github.com/oliverhaas/djangogo/apps"
	"github.com/oliverhaas/djangogo/conf"
	"github.com/oliverhaas/djangogo/manage"
)

// Application is the wired-up framework instance: settings, the app registry,
// the command dispatcher, and the root HTTP handler.
type Application struct {
	Settings *conf.Settings
	Apps     *apps.Registry
	Commands *manage.Registry
	Handler  http.Handler
	Out      io.Writer
}

// New configures settings, registers and readies apps, registers built-in
// commands, and returns the Application. It returns an error for invalid
// settings or a failing app Ready hook.
func New(settings conf.Settings, appConfigs ...apps.Config) (*Application, error) {
	s := conf.Configure(settings)
	if err := s.Check(); err != nil {
		return nil, err
	}

	appReg := apps.NewRegistry()
	for _, c := range appConfigs {
		if err := appReg.Register(c); err != nil {
			return nil, err
		}
	}
	if err := appReg.Ready(); err != nil {
		return nil, err
	}

	app := &Application{
		Settings: s,
		Apps:     appReg,
		Commands: manage.NewRegistry(),
		Handler:  defaultHandler(),
		Out:      os.Stdout,
	}
	app.Commands.Out = app.Out

	// Built-in commands. Registration cannot collide here, so ignore the error.
	_ = app.Commands.Register(versionCommand{out: app.Out})
	_ = app.Commands.Register(&runserverCommand{app: app})

	return app, nil
}

// Execute dispatches the given CLI arguments (typically os.Args[1:]).
func (a *Application) Execute(args []string) error {
	return a.Commands.Execute(args)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test . -v`
Expected: PASS (application, version, and runserver tests all green now that `Application` exists).

- [ ] **Step 5: Commit**

```bash
git add application.go application_test.go
git commit -m "feat: add Application wiring settings, apps, and commands"
```

---

### Task 9: Wire the `cmd/djangogo` CLI and smoke-test the server

**Files:**
- Modify: `cmd/djangogo/main.go` (replace the placeholder)

- [ ] **Step 1: Replace the CLI entrypoint**

```go
// cmd/djangogo/main.go
// Command djangogo is the Djan-Go-Go management CLI. In a real project this is
// generated by `startproject` with the project's settings and apps; for now it
// boots a demo application so the framework is runnable.
package main

import (
	"fmt"
	"os"

	"github.com/oliverhaas/djangogo"
	"github.com/oliverhaas/djangogo/conf"
)

func main() {
	app, err := djangogo.New(conf.Settings{
		Debug:     true,
		SecretKey: "dev-insecure-key-change-me",
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	if err := app.Execute(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
```

- [ ] **Step 2: Verify the whole module builds, vets, and tests green**

Run:
```bash
go build ./... && go vet ./... && go test ./...
```
Expected: build OK, vet silent, all packages `ok` (or `[no test files]` for `cmd/djangogo`).

- [ ] **Step 3: Smoke-test the commands**

Run: `go run ./cmd/djangogo`
Expected: usage listing including `runserver` and `version`.

Run: `go run ./cmd/djangogo version`
Expected: `djangogo 0.0.1-dev`.

- [ ] **Step 4: Smoke-test the server**

Run (in one shell): `go run ./cmd/djangogo runserver`
Expected: `Djan-Go-Go development server at http://127.0.0.1:8000/  (Ctrl-C to quit)`

In another shell: `curl -s http://127.0.0.1:8000/`
Expected: `Djan-Go-Go 0.0.1-dev is running.`

Then Ctrl-C the server; it should exit cleanly (graceful shutdown), not with a panic or non-zero crash.

- [ ] **Step 5: Lint and commit**

```bash
gofmt -l .            # expect no output
golangci-lint run     # expect no findings (install: https://golangci-lint.run)
git add cmd/djangogo/main.go
git commit -m "feat(cmd): boot a real Application from the djangogo CLI"
```

---

## Milestone 1 self-review

- **Spec coverage:** `conf` (Task 1-2), `apps` registry + lifecycle (Task 3-4), `manage` dispatch (Task 5), `runserver` (Task 7), the `Application` spine (Task 8), and the CLI (Task 9) cover the Spine's "conf + apps + manage + runserver" exit criteria. `startapp`/`startproject` scaffolding and per-app `Commands()`/`URLs()` are intentionally deferred (the `apps.Config` interface is kept minimal and extended in later milestones).
- **Type consistency:** `conf.Settings`/`Configure`/`Active`; `apps.Config`/`Initializer`/`Registry` (`Register`/`Get`/`Names`/`Ready`); `manage.Command`/`Registry` (`Out`/`Register`/`Names`/`Execute`); `djangogo.Application` (`Settings`/`Apps`/`Commands`/`Handler`/`Out`), `New`, `Execute`, `defaultHandler`, `versionCommand{out}`, `runserverCommand{app}` are used identically across tasks.
- **Known cross-task build edge:** Task 7's `runserver.go` references `Application`, defined in Task 8. Build Task 8 right after Task 7 (or use the documented temporary stub) so `go test .` goes green. This is the one place where two files in the same package must land together.

## Next milestone

After M1 is green and committed, start M2 (ORM core) as its own detailed plan: invoke the writing-plans skill against the spec's `orm` subsystem and metadata-keystone sections. The reflect-at-startup `Registry` built there becomes the foundation every later milestone reads.
