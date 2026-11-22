package djangogo_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo"
	"github.com/oliverhaas/djangogo/checks"
	"github.com/oliverhaas/djangogo/conf"
	"github.com/oliverhaas/djangogo/staticfiles"
)

// staticFixture is an application with a static source directory and the
// buffers that capture its streams.
type staticFixture struct {
	app       *djangogo.Application
	out, errb *bytes.Buffer
	src, root string
}

// writeStaticFile writes content to path, creating the directories.
func writeStaticFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// newStaticFixture builds an application whose StaticFilesDirs holds
// test/file.txt and test/other.css and whose StaticRoot is a new empty
// directory. A false withRoot leaves StaticRoot unset. The input of the
// application is answer.
func newStaticFixture(t *testing.T, withRoot bool, answer string) *staticFixture {
	t.Helper()
	f := &staticFixture{src: t.TempDir(), out: &bytes.Buffer{}, errb: &bytes.Buffer{}}
	writeStaticFile(t, filepath.Join(f.src, "test", "file.txt"), "In STATICFILES_DIRS directory.\n")
	writeStaticFile(t, filepath.Join(f.src, "test", "other.css"), "body {}\n")
	s := conf.Settings{SecretKey: "k", StaticFilesDirs: []string{f.src}}
	if withRoot {
		f.root = filepath.Join(t.TempDir(), "static_root")
		if err := os.MkdirAll(f.root, 0o755); err != nil {
			t.Fatal(err)
		}
		s.StaticRoot = f.root
	}
	app, err := djangogo.New(s)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Close() })
	app.Out, app.Err, app.In = f.out, f.errb, strings.NewReader(answer)
	f.app = app
	return f
}

// exists reports whether a file exists in the static root.
func (f *staticFixture) exists(rel string) bool {
	_, err := os.Stat(filepath.Join(f.root, rel))
	return err == nil
}

// TestCollectstaticNoinputCopiesFiles checks the copy and the summary line.
func TestCollectstaticNoinputCopiesFiles(t *testing.T) {
	f := newStaticFixture(t, true, "")
	if err := f.app.Execute([]string{"collectstatic", "--noinput", "--ignore", "*.css"}); err != nil {
		t.Fatal(err)
	}
	want := "\n" + staticfiles.CollectResult{Copied: make([]string, 1)}.Summary(f.root) + "\n"
	if !strings.HasSuffix(f.out.String(), want) {
		t.Errorf("output = %q, want suffix %q", f.out.String(), want)
	}
	if !strings.HasSuffix(want, "\n1 static file copied to '"+f.root+"'.\n") {
		t.Errorf("summary = %q", want)
	}
	if !f.exists("test/file.txt") {
		t.Error("test/file.txt was not copied")
	}
}

// TestCollectstaticLink checks that --link links the files of a static
// directory and words the summary as Django does.
//
// No Django test exists: TestCollectionLinks calls the command function, the
// port is in staticfiles.
func TestCollectstaticLink(t *testing.T) {
	f := newStaticFixture(t, true, "")
	if err := f.app.Execute([]string{"collectstatic", "--noinput", "-l", "--ignore", "*.css"}); err != nil {
		t.Fatal(err)
	}
	if want := "\n1 static file symlinked to '" + f.root + "'.\n"; !strings.HasSuffix(f.out.String(), want) {
		t.Errorf("output = %q, want suffix %q", f.out.String(), want)
	}
	target, err := os.Readlink(filepath.Join(f.root, "test", "file.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(f.src, "test", "file.txt"); target != want {
		t.Errorf("link target = %q, want %q", target, want)
	}
}

// TestCollectstaticWithoutStaticRoot checks the error of a missing root.
func TestCollectstaticWithoutStaticRoot(t *testing.T) {
	f := newStaticFixture(t, false, "")
	if code := f.app.Run([]string{"collectstatic", "--noinput"}); code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	want := "CommandError: " + staticfiles.ErrStaticRootNotSet.Error() + "\n"
	if f.errb.String() != want {
		t.Errorf("Err = %q, want %q", f.errb.String(), want)
	}
}

// TestCollectstaticDryRun checks that --dry-run copies nothing.
func TestCollectstaticDryRun(t *testing.T) {
	f := newStaticFixture(t, true, "")
	if err := f.app.Execute([]string{"collectstatic", "--noinput", "--dry-run"}); err != nil {
		t.Fatal(err)
	}
	if f.exists("test/file.txt") {
		t.Error("a dry run copied a file")
	}
}

// TestCollectstaticClear checks that --clear removes a stale file first.
func TestCollectstaticClear(t *testing.T) {
	f := newStaticFixture(t, true, "")
	writeStaticFile(t, filepath.Join(f.root, "stale.txt"), "stale")
	if err := f.app.Execute([]string{"collectstatic", "--noinput", "--clear"}); err != nil {
		t.Fatal(err)
	}
	if f.exists("stale.txt") {
		t.Error("stale.txt survived --clear")
	}
	if !f.exists("test/file.txt") {
		t.Error("test/file.txt was not copied")
	}
}

// TestCollectstaticVerbosity checks -v 0 and the argparse errors of -v.
func TestCollectstaticVerbosity(t *testing.T) {
	f := newStaticFixture(t, true, "")
	if err := f.app.Execute([]string{"collectstatic", "--noinput", "-v", "0"}); err != nil {
		t.Fatal(err)
	}
	if f.out.Len() != 0 {
		t.Errorf("-v 0 printed %q", f.out.String())
	}
	for _, tc := range []struct{ arg, want string }{
		{"5", "invalid choice: 5 (choose from 0, 1, 2, 3)"},
		{"x", "invalid int value: 'x'"},
	} {
		g := newStaticFixture(t, true, "")
		if code := g.app.Run([]string{"collectstatic", "--noinput", "-v", tc.arg}); code != 2 {
			t.Errorf("-v %s: exit code = %d, want 2", tc.arg, code)
		}
		if !strings.Contains(g.errb.String(), tc.want) {
			t.Errorf("-v %s: Err = %q, want it to contain %q", tc.arg, g.errb.String(), tc.want)
		}
	}
}

// TestCollectstaticPromptText checks the question and the dry-run notice.
func TestCollectstaticPromptText(t *testing.T) {
	f := newStaticFixture(t, true, "no\n")
	writeStaticFile(t, filepath.Join(f.root, "existing.txt"), "x")
	_ = f.app.Execute([]string{"collectstatic"})
	const question = "Type 'yes' to continue, or 'no' to cancel: "
	const notice = "You have activated the --dry-run option so no files will be modified."
	if !strings.Contains(f.out.String(), question) {
		t.Errorf("output = %q, want the question", f.out.String())
	}
	if strings.Contains(f.out.String(), notice) {
		t.Errorf("output = %q, want no dry-run notice", f.out.String())
	}

	g := newStaticFixture(t, true, "no\n")
	writeStaticFile(t, filepath.Join(g.root, "existing.txt"), "x")
	_ = g.app.Execute([]string{"collectstatic", "--dry-run"})
	if !strings.Contains(g.out.String(), question) || !strings.Contains(g.out.String(), notice) {
		t.Errorf("output = %q, want the question and the dry-run notice", g.out.String())
	}
}

// TestCollectstaticIgnore checks --ignore and -i.
func TestCollectstaticIgnore(t *testing.T) {
	for _, flag := range []string{"--ignore", "-i"} {
		f := newStaticFixture(t, true, "")
		if err := f.app.Execute([]string{"collectstatic", "--noinput", flag, "*.txt"}); err != nil {
			t.Fatal(err)
		}
		if f.exists("test/file.txt") {
			t.Errorf("%s: test/file.txt was not ignored", flag)
		}
		if !f.exists("test/other.css") {
			t.Errorf("%s: test/other.css was not copied", flag)
		}
	}
}

// TestFindstatic checks the output of findstatic.
func TestFindstatic(t *testing.T) {
	f := newStaticFixture(t, true, "")
	if err := f.app.Execute([]string{"findstatic", "test/file.txt"}); err != nil {
		t.Fatal(err)
	}
	path, err := filepath.EvalSymlinks(filepath.Join(f.src, "test", "file.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if want := "Found 'test/file.txt' here:\n  " + path + "\n"; f.out.String() != want {
		t.Errorf("Out = %q, want %q", f.out.String(), want)
	}

	f.out.Reset()
	if err := f.app.Execute([]string{"findstatic", "--first", "-v", "0", "test/file.txt"}); err != nil {
		t.Fatal(err)
	}
	if want := path + "\n"; f.out.String() != want {
		t.Errorf("--first -v 0: Out = %q, want %q", f.out.String(), want)
	}
}

// TestFindstaticMissing checks the message for a file that is not found.
func TestFindstaticMissing(t *testing.T) {
	f := newStaticFixture(t, true, "")
	if err := f.app.Execute([]string{"findstatic", "missing.txt"}); err != nil {
		t.Fatal(err)
	}
	if want := "No matching file found for 'missing.txt'.\n"; f.errb.String() != want {
		t.Errorf("Err = %q, want %q", f.errb.String(), want)
	}
	if f.out.Len() != 0 {
		t.Errorf("Out = %q, want nothing", f.out.String())
	}
}

// TestFindstaticNoArgument checks the usage error without a path.
func TestFindstaticNoArgument(t *testing.T) {
	f := newStaticFixture(t, true, "")
	if code := f.app.Run([]string{"findstatic"}); code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
	if !strings.Contains(f.errb.String(), "Enter at least one staticfile.") {
		t.Errorf("Err = %q", f.errb.String())
	}
}

// TestCollectstaticRunsOnlyStaticfilesChecks checks that an untagged failing
// check does not stop collectstatic and a staticfiles one does.
func TestCollectstaticRunsOnlyStaticfilesChecks(t *testing.T) {
	failing := func(*checks.Context) []checks.Message { return []checks.Message{checks.Error("broken")} }

	f := newStaticFixture(t, true, "")
	f.app.Checks = checks.NewRegistry()
	f.app.Checks.Register(failing)
	if code := f.app.Run([]string{"collectstatic", "--noinput"}); code != 0 {
		t.Errorf("untagged check: exit code = %d, want 0; Err = %q", code, f.errb.String())
	}

	g := newStaticFixture(t, true, "")
	g.app.Checks = checks.NewRegistry()
	g.app.Checks.Register(failing, checks.TagStaticfiles)
	if code := g.app.Run([]string{"collectstatic", "--noinput"}); code != 1 {
		t.Errorf("staticfiles check: exit code = %d, want 1", code)
	}
}
