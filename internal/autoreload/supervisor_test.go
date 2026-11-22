package autoreload_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/internal/autoreload"
)

// helperEnv selects the helper mode of a child process started by the tests:
// "serve" waits for an interrupt and exits with status 0, "exit:N" exits with
// status N at once.
const helperEnv = "DJANGOGO_AUTORELOAD_HELPER"

// helperNameEnv holds the name the helper prints in its events.
const helperNameEnv = "DJANGOGO_AUTORELOAD_HELPER_NAME"

// TestMain runs the helper instead of the tests in a child process.
func TestMain(m *testing.M) {
	if mode := os.Getenv(helperEnv); mode != "" {
		os.Exit(helper(mode, os.Getenv(helperNameEnv)))
	}
	os.Exit(m.Run())
}

// helper is the server of a child process. It prints "started NAME" when it
// handles interrupts, and "interrupted NAME" before it exits after one.
func helper(mode, name string) int {
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)
	fmt.Println("started", name)
	if code, ok := strings.CutPrefix(mode, "exit:"); ok {
		n, err := strconv.Atoi(code)
		if err != nil {
			return 99
		}
		return n
	}
	<-interrupt
	fmt.Println("interrupted", name)
	return 0
}

// lineWriter sends each line written to it to lines.
type lineWriter struct {
	lines chan<- string
	buf   []byte
}

// Write sends the complete lines of p and keeps the rest for the next call.
func (w *lineWriter) Write(p []byte) (int, error) {
	w.buf = append(w.buf, p...)
	for {
		i := bytes.IndexByte(w.buf, '\n')
		if i < 0 {
			return len(p), nil
		}
		w.lines <- string(w.buf[:i])
		w.buf = w.buf[i+1:]
	}
}

// syncBuffer is a bytes.Buffer that the supervisor and the test can use at
// the same time.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

// Write appends p.
func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

// String returns what was written.
func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// harness runs a Supervisor whose children are helper processes of the test
// binary.
type harness struct {
	t *testing.T
	// watched is the file whose changes start a build.
	watched string
	// touches counts the changes of watched.
	touches int
	// events receives the lines the children print.
	events chan string
	// builds receives the output path of each build.
	builds chan string
	out    syncBuffer
	cancel context.CancelFunc
	// finished is closed when Run has returned err.
	finished chan struct{}
	err      error
}

// waitTimeout bounds every wait of the harness.
const waitTimeout = 30 * time.Second

// supervise starts a Supervisor for the binary site.exe. modes maps the base
// name of a binary to the helper mode of its child, "serve" when missing. The
// nth build fails with fail(n) when fail returns an error, and writes an
// empty binary otherwise.
func supervise(t *testing.T, modes map[string]string, fail func(n int) error) *harness {
	t.Helper()
	dir := t.TempDir()
	h := &harness{
		t:        t,
		watched:  filepath.Join(dir, "main.go"),
		events:   make(chan string, 100),
		builds:   make(chan string, 100),
		finished: make(chan struct{}),
	}
	if err := os.WriteFile(h.watched, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	n := 0
	s := &autoreload.Supervisor{
		Exe:      filepath.Join(dir, "site.exe"),
		Out:      &h.out,
		Interval: 10 * time.Millisecond,
		Reloader: watching(h.watched),
		Build: func(_ context.Context, out string) error {
			n++
			h.builds <- out
			if fail != nil {
				if err := fail(n); err != nil {
					return err
				}
			}
			return os.WriteFile(out, nil, 0o600)
		},
		Command: func(exe string) *exec.Cmd {
			name := filepath.Base(exe)
			mode := modes[name]
			if mode == "" {
				mode = "serve"
			}
			cmd := exec.Command(os.Args[0])
			// Under -race, a process waits a second at exit unless GORACE
			// says otherwise.
			cmd.Env = append(os.Environ(), helperEnv+"="+mode, helperNameEnv+"="+name, "GORACE=atexit_sleep_ms=0")
			cmd.Stdout = &lineWriter{lines: h.events}
			cmd.Stderr = os.Stderr
			return cmd
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	h.cancel = cancel
	go func() {
		h.err = s.Run(ctx)
		close(h.finished)
	}()
	t.Cleanup(func() {
		cancel()
		<-h.finished
	})
	return h
}

// touch changes the modification time of the watched file.
func (h *harness) touch() {
	h.t.Helper()
	h.touches++
	setMtime(h.t, h.watched, time.Now().Add(time.Duration(h.touches)*time.Second))
}

// expect waits for the next line of a child and checks it.
func (h *harness) expect(want string) {
	h.t.Helper()
	select {
	case got := <-h.events:
		if got != want {
			h.t.Fatalf("child printed %q, want %q", got, want)
		}
	case <-time.After(waitTimeout):
		h.t.Fatalf("no line from a child, want %q", want)
	}
}

// expectBuild waits for the next build and checks the base name of its
// output. It returns the output path.
func (h *harness) expectBuild(want string) string {
	h.t.Helper()
	select {
	case out := <-h.builds:
		if got := filepath.Base(out); got != want {
			h.t.Fatalf("build of %q, want %q", got, want)
		}
		return out
	case <-time.After(waitTimeout):
		h.t.Fatalf("no build, want one of %q", want)
		return ""
	}
}

// wait waits for Run to return and returns its error.
func (h *harness) wait() error {
	h.t.Helper()
	select {
	case <-h.finished:
		return h.err
	case <-time.After(waitTimeout):
		h.t.Fatal("Run did not return")
		return nil
	}
}

// interrupt ends Run as an interrupt of the supervisor does and returns its
// error.
func (h *harness) interrupt() error {
	h.t.Helper()
	h.cancel()
	return h.wait()
}

// watchingLine is the line the supervisor prints for each child.
const watchingLine = "Watching for file changes with StatReloader\n"

// TestSupervisorRestartsOnChange builds the program after a change, stops the
// old server and starts the new binary. No Django test exists: Django's tests
// replace subprocess.run and the reloader thread with mocks.
func TestSupervisorRestartsOnChange(t *testing.T) {
	h := supervise(t, nil, nil)
	h.expect("started site.exe")

	h.touch()
	first := h.expectBuild("site-1.exe")
	h.expect("interrupted site.exe")
	h.expect("started site-1.exe")

	h.touch()
	h.expectBuild("site-2.exe")
	h.expect("interrupted site-1.exe")
	h.expect("started site-2.exe")
	if _, err := os.Stat(first); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("Stat(%s) = %v, want ErrNotExist: the binary of a stopped server is removed", first, err)
	}

	if err := h.interrupt(); err != nil {
		t.Fatalf("Run = %v, want nil", err)
	}
	h.expect("interrupted site-2.exe")
	changed := h.watched + " changed, reloading.\n"
	if got, want := h.out.String(), watchingLine+changed+watchingLine+changed+watchingLine; got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

// TestSupervisorKeepsServerWhenBuildFails prints the build errors and keeps
// the old server until a build succeeds. No Django test exists: a Python
// program needs no build.
func TestSupervisorKeepsServerWhenBuildFails(t *testing.T) {
	h := supervise(t, nil, func(n int) error {
		if n == 1 {
			return errors.New("main.go:3:1: syntax error")
		}
		return nil
	})
	h.expect("started site.exe")

	h.touch()
	h.expectBuild("site-1.exe")
	h.touch()
	h.expectBuild("site-2.exe")
	// The next line comes from the first server: it ran until now.
	h.expect("interrupted site.exe")
	h.expect("started site-2.exe")

	if err := h.interrupt(); err != nil {
		t.Fatalf("Run = %v, want nil", err)
	}
	changed := h.watched + " changed, reloading.\n"
	want := watchingLine + changed + "main.go:3:1: syntax error\nThe build failed. Waiting for file changes.\n" +
		changed + watchingLine
	if got := h.out.String(); got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

// TestSupervisorKeepsWatchingAfterServerFails starts the server again after
// the next change when it exited with an error, as Django's reloader keeps
// watching after an error in the child. No Django test exists.
func TestSupervisorKeepsWatchingAfterServerFails(t *testing.T) {
	h := supervise(t, map[string]string{"site.exe": "exit:1"}, nil)
	h.expect("started site.exe")
	select {
	case <-h.finished:
		t.Fatalf("Run = %v after the server failed, want it to keep watching", h.err)
	case <-time.After(50 * time.Millisecond):
	}

	h.touch()
	h.expectBuild("site-1.exe")
	h.expect("started site-1.exe")
	if err := h.interrupt(); err != nil {
		t.Fatalf("Run = %v, want nil", err)
	}
}

// TestRunWithoutSourcesIsUnavailable reports that the sources cannot be
// listed before it starts a server. No Django test exists: Django always
// finds the modules it runs.
func TestRunWithoutSourcesIsUnavailable(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("the go command is missing")
	}
	t.Setenv("GOWORK", "off")
	t.Setenv("GOFLAGS", "")
	t.Setenv("GOPROXY", "off")
	started := false
	s := &autoreload.Supervisor{
		Program: autoreload.Program{Dir: t.TempDir(), Package: "example.com/missing"},
		Exe:     "site",
		Out:     &bytes.Buffer{},
		Command: func(string) *exec.Cmd {
			started = true
			return exec.Command("true")
		},
	}
	err := s.Run(context.Background())
	if unavailable := (*autoreload.UnavailableError)(nil); !errors.As(err, &unavailable) {
		t.Errorf("Run = %v, want an *UnavailableError", err)
	}
	if started {
		t.Error("Run started a server without sources")
	}
}

// TestNewSupervisorRefusesTestBinary leaves test binaries to serve in the
// process: they are not built by go build. No Django test exists.
func TestNewSupervisorRefusesTestBinary(t *testing.T) {
	_, err := autoreload.NewSupervisor()
	if unavailable := (*autoreload.UnavailableError)(nil); !errors.As(err, &unavailable) {
		t.Errorf("NewSupervisor = %v, want an *UnavailableError", err)
	}
}

// writeFiles writes the files to dir, creating directories as needed.
func writeFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

// TestProgramBuildAndSources builds a module with the go command and lists
// its sources. No Django test exists: a Python program needs no build.
func TestProgramBuildAndSources(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("the go command is missing")
	}
	t.Setenv("GOWORK", "off")
	t.Setenv("GOFLAGS", "")
	t.Setenv("GOPROXY", "off")
	t.Setenv("GOTOOLCHAIN", "local")
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"go.mod": "module example.com/site\n\ngo 1.21\n",
		"main.go": "package main\n\nimport (\n\t_ \"embed\"\n\n\t\"example.com/site/inner\"\n)\n\n" +
			"//go:embed static.txt\nvar static string\n\nfunc main() { println(inner.Name, static) }\n",
		"static.txt":           "static",
		"inner/inner.go":       "package inner\n\n// Name is a name.\nconst Name = \"inner\"\n",
		"inner/inner_test.go":  "package inner\n",
		"inner/_ignored.go":    "package inner\n",
		"inner/.hidden.go":     "package inner\n",
		"inner/notes.txt":      "notes",
		"inner/nested/keep.go": "package nested\n",
	})
	p := autoreload.Program{Dir: dir, Package: "example.com/site"}
	ctx := context.Background()

	files, dirs, err := p.Sources(ctx)
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(files)
	wantFiles := []string{
		filepath.Join(dir, "go.mod"), filepath.Join(dir, "go.sum"), filepath.Join(dir, "inner", "inner.go"),
		filepath.Join(dir, "main.go"), filepath.Join(dir, "static.txt"),
	}
	if !slices.Equal(files, wantFiles) {
		t.Errorf("files = %q, want %q", files, wantFiles)
	}
	slices.Sort(dirs)
	if wantDirs := []string{dir, filepath.Join(dir, "inner")}; !slices.Equal(dirs, wantDirs) {
		t.Errorf("dirs = %q, want %q", dirs, wantDirs)
	}
	if got, want := autoreload.GoFiles(filepath.Join(dir, "inner")), []string{filepath.Join(dir, "inner", "inner.go")}; !slices.Equal(got, want) {
		t.Errorf("GoFiles = %q, want %q: test, ignored and hidden files and subdirectories are no sources", got, want)
	}

	out := filepath.Join(t.TempDir(), "site")
	if err := p.Build(ctx, out); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(out); err != nil {
		t.Errorf("Stat(%s) = %v after the build", out, err)
	}

	writeFiles(t, dir, map[string]string{"inner/inner.go": "package inner\n\nconst Name = \n"})
	if err := p.Build(ctx, out); err == nil || !strings.Contains(err.Error(), "inner.go") {
		t.Errorf("Build = %v, want the compiler error about inner.go", err)
	}
}

// TestPollSourcesSeesAddedAndRemovedFiles reports a .go file that appears in
// or disappears from a package directory: either changes the build. No Django
// test exists: Django watches only the modules it has imported.
func TestPollSourcesSeesAddedAndRemovedFiles(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"a.go": "package p\n"})
	poll := autoreload.PollSources(dir)
	if changed := poll(); len(changed) != 0 {
		t.Errorf("first poll = %q, want none: the files of a new directory are no change", changed)
	}
	writeFiles(t, dir, map[string]string{"b.go": "package p\n", "b_test.go": "package p\n", "notes.txt": ""})
	if changed, want := poll(), []string{filepath.Join(dir, "b.go")}; !slices.Equal(changed, want) {
		t.Errorf("poll after adding = %q, want %q", changed, want)
	}
	if err := os.Remove(filepath.Join(dir, "a.go")); err != nil {
		t.Fatal(err)
	}
	if changed, want := poll(), []string{filepath.Join(dir, "a.go")}; !slices.Equal(changed, want) {
		t.Errorf("poll after removing = %q, want %q", changed, want)
	}
	if changed := poll(); len(changed) != 0 {
		t.Errorf("poll without changes = %q, want none", changed)
	}
}
