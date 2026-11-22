package autoreload_test

import (
	"errors"
	"os"
	"path/filepath"
	"runtime/debug"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/internal/autoreload"
	"github.com/oliverhaas/djangogo/manage"
)

// No Go counterpart: most tests of Django's utils_tests/test_autoreload.py
// cover Python's module system (iter_modules_and_files, sys.path, zip
// imports, bytecode, __main__.__spec__, interpreter flags such as -W, -X and
// -u), Watchman, and the reloader thread that waits for the app registry. A
// Go program has none of them: the go command lists the sources and builds
// the program, and the reloader runs in the supervising process.

// reloaderFiles returns a file that exists, with the current time as its
// modification time, and one that does not (the setUp of ReloaderTests).
func reloaderFiles(t *testing.T) (existing, nonexistent string) {
	t.Helper()
	dir := t.TempDir()
	existing = filepath.Join(dir, "test.go")
	if err := os.WriteFile(existing, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	setMtime(t, existing, time.Now())
	return existing, filepath.Join(dir, "does_not_exist.go")
}

// setMtime sets the modification time of name.
func setMtime(t *testing.T, name string, mtime time.Time) {
	t.Helper()
	if err := os.Chtimes(name, mtime, mtime); err != nil {
		t.Fatal(err)
	}
}

// incrementMtime moves the modification time of name one second past now.
func incrementMtime(t *testing.T, name string) {
	t.Helper()
	setMtime(t, name, time.Now().Add(time.Second))
}

// watching returns a reloader that watches files.
func watching(files ...string) *autoreload.StatReloader {
	return &autoreload.StatReloader{Files: func() []string { return files }}
}

// Ported from Django: tests/utils_tests/test_autoreload.py::StatReloaderTests.test_tick_does_not_trigger_twice
func TestStatReloaderTickDoesNotTriggerTwice(t *testing.T) {
	existing, _ := reloaderFiles(t)
	reloader := watching(existing)
	var changes int
	changes += len(reloader.Tick())
	incrementMtime(t, existing)
	changes += len(reloader.Tick())
	changes += len(reloader.Tick())
	if changes != 1 {
		t.Errorf("changes = %d, want 1", changes)
	}
}

// Ported from Django: tests/utils_tests/test_autoreload.py::StatReloaderTests.test_snapshot_files_ignores_missing_files
func TestStatReloaderSnapshotFilesIgnoresMissingFiles(t *testing.T) {
	_, nonexistent := reloaderFiles(t)
	if snapshot := watching(nonexistent).Snapshot(); len(snapshot) != 0 {
		t.Errorf("snapshot = %v, want empty", snapshot)
	}
}

// Ported from Django: tests/utils_tests/test_autoreload.py::StatReloaderTests.test_snapshot_files_updates
func TestStatReloaderSnapshotFilesUpdates(t *testing.T) {
	existing, _ := reloaderFiles(t)
	reloader := watching(existing)
	snapshot1 := reloader.Snapshot()
	if _, ok := snapshot1[existing]; !ok {
		t.Fatalf("snapshot = %v, want %s in it", snapshot1, existing)
	}
	incrementMtime(t, existing)
	snapshot2 := reloader.Snapshot()
	if snapshot1[existing].Equal(snapshot2[existing]) {
		t.Errorf("mtime = %v in both snapshots, want a change", snapshot1[existing])
	}
}

// Ported from Django: tests/utils_tests/test_autoreload.py::StatReloaderTests.test_snapshot_files_with_duplicates
func TestStatReloaderSnapshotFilesWithDuplicates(t *testing.T) {
	existing, _ := reloaderFiles(t)
	snapshot := watching(existing, existing).Snapshot()
	if _, ok := snapshot[existing]; len(snapshot) != 1 || !ok {
		t.Errorf("snapshot = %v, want only %s", snapshot, existing)
	}
}

// TestStatReloaderTickSeesNewFile watches a file that appears after the first
// check. No Django test exists: Django finds new modules when they are
// imported, the Go reloader when Files lists them.
func TestStatReloaderTickSeesNewFile(t *testing.T) {
	existing, nonexistent := reloaderFiles(t)
	reloader := watching(existing, nonexistent)
	reloader.Tick()
	if err := os.WriteFile(nonexistent, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if changed := reloader.Tick(); len(changed) != 0 {
		t.Errorf("changed = %v after the file appeared, want none: a file seen first is no change", changed)
	}
	incrementMtime(t, nonexistent)
	if changed := reloader.Tick(); !slices.Equal(changed, []string{nonexistent}) {
		t.Errorf("changed = %v, want [%s]", changed, nonexistent)
	}
}

// TestProgramOf derives the go build call from the build information. No
// Django test exists: Django restarts the interpreter with its own flags
// (TestChildArguments), a Go program is built again with its build settings.
func TestProgramOf(t *testing.T) {
	bi := &debug.BuildInfo{
		Path: "example.com/site",
		Settings: []debug.BuildSetting{
			{Key: "-buildmode", Value: "exe"},
			{Key: "-compiler", Value: "gc"},
			{Key: "-race", Value: "true"},
			{Key: "-tags", Value: "dev,sqlite"},
			{Key: "-ldflags", Value: "-X main.version=1 -s"},
			{Key: "CGO_ENABLED", Value: "1"},
			{Key: "CGO_CFLAGS", Value: "-O2 -g"},
			{Key: "GOARCH", Value: "amd64"},
			{Key: "vcs.revision", Value: "abc"},
		},
	}
	p, err := autoreload.ProgramOf(bi, "/src/site")
	if err != nil {
		t.Fatal(err)
	}
	wantFlags := []string{"-buildmode=exe", "-compiler=gc", "-race=true", "-tags=dev,sqlite", "-ldflags=-X main.version=1 -s"}
	if p.Dir != "/src/site" || p.Package != "example.com/site" || !slices.Equal(p.Flags, wantFlags) ||
		!slices.Equal(p.Env, []string{"CGO_ENABLED=1", "CGO_CFLAGS=-O2 -g"}) {
		t.Errorf("ProgramOf = %+v, want flags %q", p, wantFlags)
	}

	for _, path := range []string{"", "command-line-arguments"} {
		if _, err := autoreload.ProgramOf(&debug.BuildInfo{Path: path}, "/src"); err == nil {
			t.Errorf("ProgramOf(Path %q) = nil error, want one: there is no package to build", path)
		}
	}
}

// Ported from Django: tests/utils_tests/test_autoreload.py::RestartWithReloaderTests.test_manage_py
func TestRestartWithReloaderManagePy(t *testing.T) {
	exe := filepath.Join(t.TempDir(), "site")
	s := &autoreload.Supervisor{Exe: exe, Args: []string{"runserver"}}
	cmd := autoreload.ChildCommand(s, exe)
	if cmd.Path != exe || !slices.Equal(cmd.Args, []string{exe, "runserver"}) {
		t.Errorf("command = %s %q, want %s with the arguments of the program", cmd.Path, cmd.Args, exe)
	}
	if !slices.Contains(cmd.Env, "RUN_MAIN=true") {
		t.Errorf("environment = %q, want RUN_MAIN=true in it", cmd.Env)
	}
	if cmd.Stdin != os.Stdin || cmd.Stdout != os.Stdout || cmd.Stderr != os.Stderr {
		t.Error("the child does not use the standard streams of the program")
	}
}

// No Go counterpart: RestartWithReloaderTests.test_python_m_django,
// test_propagates_unbuffered_from_parent and
// test_does_not_propagate_unbuffered_from_parent cover the interpreter
// options of the restarted Python process.

// Ported from Django: tests/utils_tests/test_autoreload.py::RunWithReloaderTests.test_swallows_keyboard_interrupt
func TestRunWithReloaderSwallowsKeyboardInterrupt(t *testing.T) {
	h := supervise(t, nil, nil)
	h.expect("started site.exe")
	if err := h.interrupt(); err != nil {
		t.Errorf("Run = %v after an interrupt, want nil", err)
	}
	h.expect("interrupted site.exe")
}

// Ported from Django: tests/utils_tests/test_autoreload.py::RunWithReloaderTests.test_calls_sys_exit
func TestRunWithReloaderCallsSysExit(t *testing.T) {
	// Django's parent exits with the status of the child. The Go server
	// process exits with StopCode when it cannot go on, and the supervisor
	// then exits with status 1 without printing more.
	h := supervise(t, map[string]string{"site.exe": "exit:" + strconv.Itoa(autoreload.StopCode)}, nil)
	h.expect("started site.exe")
	err := h.wait()
	var quiet interface{ Quiet() bool }
	if !errors.As(err, &quiet) || !quiet.Quiet() {
		t.Errorf("Run = %v, want a quiet error", err)
	}
	if code := manage.ExitCode(err); code != 1 {
		t.Errorf("ExitCode = %d, want 1", code)
	}

	h = supervise(t, map[string]string{"site.exe": "exit:0"}, nil)
	h.expect("started site.exe")
	if err := h.wait(); err != nil {
		t.Errorf("Run = %v after the server exited with status 0, want nil", err)
	}
}
