// Package autoreload restarts the development server when the source of the
// program changes. It is the port of django.utils.autoreload for a compiled
// program: Django restarts the Python process, which reads the changed
// modules again; a Go program must be built again first.
//
// The runserver command becomes a supervisor. It starts the server in a child
// process, the same program with the same arguments and RUN_MAIN=true in the
// environment, checks the source files once per second, and on a change runs
// go build and restarts the child with the new binary. When the build fails,
// the old server keeps running and the build errors are printed.
package autoreload

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

// EnvVar is the environment variable that marks the server process started
// by the supervisor (Django's DJANGO_AUTORELOAD_ENV). Its value there is
// "true".
const EnvVar = "RUN_MAIN"

// IsChild reports whether this process is a server process that a
// supervisor started.
func IsChild() bool { return os.Getenv(EnvVar) == "true" }

// StopCode is the exit status of a server process that cannot listen on its
// address. No change of the sources fixes that, so the supervisor stops with
// status 1, as Django's runserver does. A server process that fails for
// another reason, such as a system check error, is started again after the
// next change.
const StopCode = 4

// Interval is the time between two checks of the watched files (Django's
// StatReloader.SLEEP_TIME).
const Interval = time.Second

// StatReloader finds changed files by their modification times (Django's
// StatReloader). Files returns the watched files on every check, so that a
// file that appears later is watched from then on.
type StatReloader struct {
	// Files returns the paths of the watched files.
	Files func() []string

	// mtimes holds the last modification time seen for each file.
	mtimes map[string]time.Time
}

// Snapshot returns the modification time of each watched file, once per path
// (Django's snapshot_files). A file that does not exist is left out.
func (r *StatReloader) Snapshot() map[string]time.Time {
	snapshot := make(map[string]time.Time)
	for _, name := range r.Files() {
		if _, seen := snapshot[name]; seen {
			continue
		}
		info, err := os.Stat(name)
		if err != nil {
			continue
		}
		snapshot[name] = info.ModTime()
	}
	return snapshot
}

// Tick checks the watched files once and returns the ones modified since the
// last check, sorted (one pass of Django's tick). A file seen for the first
// time is not a change.
func (r *StatReloader) Tick() []string {
	if r.mtimes == nil {
		r.mtimes = make(map[string]time.Time)
	}
	var changed []string
	for name, mtime := range r.Snapshot() {
		old, ok := r.mtimes[name]
		r.mtimes[name] = mtime
		if ok && mtime.After(old) {
			changed = append(changed, name)
		}
	}
	slices.Sort(changed)
	return changed
}

// Program says how to build the running program again.
type Program struct {
	// Dir is the directory the go command runs in, inside the module of the
	// program.
	Dir string
	// Package is the import path of the main package.
	Package string
	// Flags are the go build flags the program was built with, such as
	// -tags=dev.
	Flags []string
	// Env holds the environment variables of the build, such as
	// CGO_ENABLED=0.
	Env []string
}

// buildFlags are the build settings that go build takes as flags.
var buildFlags = []string{
	"-asan", "-asmflags", "-buildmode", "-compiler", "-gcflags", "-ldflags", "-msan", "-race", "-tags", "-trimpath",
}

// ProgramOf returns how to build the program with the build information bi
// again, running the go command in dir. A program built from a list of files
// (go run main.go) has no package to build.
func ProgramOf(bi *debug.BuildInfo, dir string) (Program, error) {
	if bi.Path == "" || bi.Path == "command-line-arguments" {
		return Program{}, errors.New("the program was built from a list of .go files, not from a package")
	}
	p := Program{Dir: dir, Package: bi.Path}
	for _, s := range bi.Settings {
		switch {
		case slices.Contains(buildFlags, s.Key):
			p.Flags = append(p.Flags, s.Key+"="+s.Value)
		case strings.HasPrefix(s.Key, "CGO_") && s.Value != "":
			p.Env = append(p.Env, s.Key+"="+s.Value)
		}
	}
	return p, nil
}

// Build runs go build for the program and writes the binary to out. The
// error holds the output of the go command.
func (p Program) Build(ctx context.Context, out string) error {
	args := append([]string{"build", "-o", out}, p.Flags...)
	args = append(args, p.Package)
	cmd := exec.CommandContext(ctx, "go", args...)
	cmd.Dir = p.Dir
	cmd.Env = append(os.Environ(), p.Env...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%w\n%s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

// listedPackage holds the fields of go list -json that the watcher needs.
type listedPackage struct {
	Dir        string
	Standard   bool
	GoFiles    []string
	CgoFiles   []string
	EmbedFiles []string
	Module     *struct {
		Main    bool
		GoMod   string
		Replace *struct{ Version string }
	}
}

// Sources returns the files the program is built from that can change while
// it runs: the files of its packages in the main module, in the other modules
// of a workspace and in modules replaced by a local directory, and their
// go.mod and go.sum. Packages of the standard library and of the module cache
// are left out. dirs are the package directories.
func (p Program) Sources(ctx context.Context) (files, dirs []string, err error) {
	args := append([]string{"list", "-deps", "-json=Dir,Standard,GoFiles,CgoFiles,EmbedFiles,Module"}, p.Flags...)
	args = append(args, p.Package)
	cmd := exec.CommandContext(ctx, "go", args...)
	cmd.Dir = p.Dir
	cmd.Env = append(os.Environ(), p.Env...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return nil, nil, fmt.Errorf("go list %s: %w: %s", p.Package, err, msg)
		}
		return nil, nil, fmt.Errorf("go list %s: %w", p.Package, err)
	}
	dec := json.NewDecoder(bytes.NewReader(out))
	gomods := make(map[string]bool)
	for {
		var pkg listedPackage
		if err := dec.Decode(&pkg); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return nil, nil, fmt.Errorf("go list %s: %w", p.Package, err)
		}
		if pkg.Standard || pkg.Module == nil {
			continue
		}
		local := pkg.Module.Main || (pkg.Module.Replace != nil && pkg.Module.Replace.Version == "")
		if !local {
			continue
		}
		dirs = append(dirs, pkg.Dir)
		for _, list := range [][]string{pkg.GoFiles, pkg.CgoFiles, pkg.EmbedFiles} {
			for _, name := range list {
				files = append(files, filepath.Join(pkg.Dir, name))
			}
		}
		if gomod := pkg.Module.GoMod; gomod != "" && !gomods[gomod] {
			gomods[gomod] = true
			files = append(files, gomod, filepath.Join(filepath.Dir(gomod), "go.sum"))
		}
	}
	return files, dirs, nil
}

// goFiles returns the .go files of dir that go build compiles, without the
// test files.
func goFiles(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var files []string
	for _, e := range entries {
		name := e.Name()
		if e.Type().IsRegular() && strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, "_test.go") &&
			!strings.HasPrefix(name, ".") && !strings.HasPrefix(name, "_") {
			files = append(files, filepath.Join(dir, name))
		}
	}
	return files
}

// sources holds the watched files of a program: the files go list named, and
// the package directories, whose .go files are listed on every check so that
// a new or removed file starts a build too.
type sources struct {
	files, dirs []string
	// known holds the .go files of each package directory at the last poll.
	known map[string][]string
}

// update lists the sources of p again.
func (w *sources) update(ctx context.Context, p Program) error {
	files, dirs, err := p.Sources(ctx)
	if err != nil {
		return err
	}
	w.files, w.dirs = files, dirs
	return nil
}

// list returns the watched files.
func (w *sources) list() []string {
	files := slices.Clone(w.files)
	for _, dir := range w.dirs {
		files = append(files, goFiles(dir)...)
	}
	return files
}

// poll returns the .go files that appeared in or disappeared from a package
// directory since the last poll. The StatReloader does not report them: it
// takes a file seen for the first time as no change, and it ignores a missing
// one. The files of a directory polled for the first time are no change, so
// that a package the last build added does not start another build.
func (w *sources) poll() []string {
	if w.known == nil {
		w.known = make(map[string][]string)
	}
	var changed []string
	for _, dir := range w.dirs {
		now := goFiles(dir)
		old, seen := w.known[dir]
		w.known[dir] = now
		if !seen {
			continue
		}
		for _, name := range now {
			if !slices.Contains(old, name) {
				changed = append(changed, name)
			}
		}
		for _, name := range old {
			if !slices.Contains(now, name) {
				changed = append(changed, name)
			}
		}
	}
	return changed
}

// ErrTestBinary is the cause of the *UnavailableError of NewSupervisor in a
// test binary, which go build does not make.
var ErrTestBinary = errors.New("the program is a test binary")

// UnavailableError reports that the program cannot be built again here, for
// example because the go command is missing. Nothing has started then, so the
// caller can serve without the reloader.
type UnavailableError struct {
	// Err is the cause.
	Err error
}

// Error returns the text of the cause.
func (e *UnavailableError) Error() string { return e.Err.Error() }

// Unwrap returns the cause.
func (e *UnavailableError) Unwrap() error { return e.Err }

// stopError is the error of Run when the server process exited with
// StopCode. The process has printed the reason.
type stopError struct{}

// Error returns a short description.
func (stopError) Error() string { return "the server process stopped" }

// Quiet reports that the error is printed already, for the manage registry.
func (stopError) Quiet() bool { return true }

// Supervisor runs the server in a child process and restarts it when a
// source file changes (Django's restart_with_reloader; the reloader that
// Django runs in the child runs here in the parent, which also builds).
type Supervisor struct {
	// Program builds the new binary and names the sources.
	Program Program
	// Build replaces Program.Build, for tests.
	Build func(ctx context.Context, out string) error
	// Exe is the binary of the first child, the running program.
	Exe string
	// Args are the arguments of the child, without the program name.
	Args []string
	// Out receives the messages of the supervisor.
	Out io.Writer
	// Interval is the time between two checks of the files. Zero means
	// Interval.
	Interval time.Duration
	// Command returns the child process for a binary. Nil runs the binary
	// with Args, the environment of this process plus RUN_MAIN=true, and the
	// standard streams of this process.
	Command func(exe string) *exec.Cmd
	// Reloader finds the changed files. Nil watches the sources of Program,
	// listed again after each build.
	Reloader *StatReloader
}

// NewSupervisor returns a Supervisor for the running program: it builds the
// main package of the program with its build settings in the working
// directory and starts the result with the arguments of this process. The
// error is an *UnavailableError when that is not possible, for example in a
// test binary or a program built from a list of files.
func NewSupervisor() (*Supervisor, error) {
	if testing.Testing() {
		return nil, &UnavailableError{Err: ErrTestBinary}
	}
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return nil, &UnavailableError{Err: errors.New("the program has no build information")}
	}
	dir, err := os.Getwd()
	if err != nil {
		return nil, &UnavailableError{Err: err}
	}
	p, err := ProgramOf(bi, dir)
	if err != nil {
		return nil, &UnavailableError{Err: err}
	}
	exe, err := os.Executable()
	if err != nil {
		return nil, &UnavailableError{Err: err}
	}
	return &Supervisor{Program: p, Exe: exe, Args: os.Args[1:]}, nil
}

// stopTimeout bounds the wait for a child to shut down after the interrupt.
// The server gives open requests 5 seconds.
const stopTimeout = 10 * time.Second

// Run starts the child and supervises it until ctx is done or the child
// exits with status 0, as a server does after an interrupt. A child that
// exits with StopCode ends Run with an error that the manage registry does
// not print and that exits with status 1. A child that fails otherwise is
// started again after the next change. The error is an *UnavailableError
// when the sources cannot be listed.
func (s *Supervisor) Run(ctx context.Context) error {
	reloader := s.Reloader
	var src *sources
	if reloader == nil {
		src = &sources{}
		if err := src.update(ctx, s.Program); err != nil {
			return &UnavailableError{Err: err}
		}
		reloader = &StatReloader{Files: src.list}
		src.poll()
	}
	reloader.Tick()

	dir, err := os.MkdirTemp("", "djangogo-autoreload-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(dir) }()

	interval := s.Interval
	if interval == 0 {
		interval = Interval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	exe := s.Exe
	c, err := s.start(exe)
	if err != nil {
		return err
	}
	builds := 0
	for {
		select {
		case <-ctx.Done():
			c.stop()
			return nil
		case <-c.done():
			code := c.cmd.ProcessState.ExitCode()
			switch {
			case code == 0 || ctx.Err() != nil:
				return nil
			case code == StopCode:
				return stopError{}
			}
			c = nil
			continue
		case <-ticker.C:
		}
		changed := reloader.Tick()
		if src != nil {
			changed = append(changed, src.poll()...)
		}
		if len(changed) == 0 {
			continue
		}
		_, _ = fmt.Fprintf(s.Out, "%s changed, reloading.\n", changed[0])
		builds++
		next := filepath.Join(dir, binaryName(s.Exe, builds))
		if err := s.build(ctx, next); err != nil {
			if ctx.Err() != nil {
				c.stop()
				return nil
			}
			_, _ = fmt.Fprintf(s.Out, "%v\nThe build failed. Waiting for file changes.\n", err)
			continue
		}
		if src != nil {
			// A failed listing keeps the sources of the last one.
			_ = src.update(ctx, s.Program)
		}
		c.stop()
		if exe != s.Exe {
			_ = os.Remove(exe)
		}
		exe = next
		if c, err = s.start(exe); err != nil {
			return err
		}
	}
}

// binaryName returns the file name of the nth build of the program exe. It
// keeps the extension, which Windows needs to run the file.
func binaryName(exe string, n int) string {
	base := filepath.Base(exe)
	ext := filepath.Ext(base)
	return strings.TrimSuffix(base, ext) + "-" + strconv.Itoa(n) + ext
}

// build builds the program to out with Build, or Program.Build when Build is
// nil.
func (s *Supervisor) build(ctx context.Context, out string) error {
	if s.Build != nil {
		return s.Build(ctx, out)
	}
	return s.Program.Build(ctx, out)
}

// command returns the child process for exe (see Command).
func (s *Supervisor) command(exe string) *exec.Cmd {
	if s.Command != nil {
		return s.Command(exe)
	}
	cmd := exec.Command(exe, s.Args...)
	cmd.Env = append(os.Environ(), EnvVar+"=true")
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd
}

// child is a running server process.
type child struct {
	cmd *exec.Cmd
	// exited is closed when the process has exited.
	exited chan struct{}
}

// start prints the reloader line, as Django's child does, and starts a child
// with exe.
func (s *Supervisor) start(exe string) (*child, error) {
	_, _ = fmt.Fprintln(s.Out, "Watching for file changes with StatReloader")
	cmd := s.command(exe)
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	c := &child{cmd: cmd, exited: make(chan struct{})}
	go func() {
		_ = cmd.Wait()
		close(c.exited)
	}()
	return c, nil
}

// done returns a channel that is closed when the child has exited. It is nil,
// and blocks forever, when there is no child.
func (c *child) done() <-chan struct{} {
	if c == nil {
		return nil
	}
	return c.exited
}

// stop interrupts the child and waits for it to exit. A child that does not
// exit within stopTimeout, or that cannot be interrupted (Windows), is
// killed.
func (c *child) stop() {
	if c == nil {
		return
	}
	select {
	case <-c.exited:
		return
	default:
	}
	if err := c.cmd.Process.Signal(os.Interrupt); err == nil {
		select {
		case <-c.exited:
			return
		case <-time.After(stopTimeout):
		}
	}
	_ = c.cmd.Process.Kill()
	<-c.exited
}
