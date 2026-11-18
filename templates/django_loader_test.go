package templates_test

import (
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"testing/fstest"

	"github.com/oliverhaas/djangogo/templates"
)

// writeTemplates writes the given files below a new temporary directory and
// returns it.
func writeTemplates(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, src := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	return dir
}

// loaderEngine is the setup of Django's TemplateLoaderTests. Django configures
// two engines, searched in order; here one engine has two sources, a project
// directory searched first and the templates of an app after it.
func loaderEngine(t *testing.T) *templates.Engine {
	t.Helper()
	project := writeTemplates(t, map[string]string{
		"template_loader/hello.html": "Hello! (project directory)\n",
	})
	app := fstest.MapFS{
		"template_loader/hello.html":   {Data: []byte("Hello! (app templates)\n")},
		"template_loader/goodbye.html": {Data: []byte("Goodbye! (app templates)\n")},
		"template_loader/request.html": {Data: []byte("{{ request.path }}\n")},
	}
	engine := templates.New(templates.DirSource(project))
	engine.AddSource(templates.FSSource("template_loader", app))
	engine.AddContextProcessor(templates.RequestProcessor)
	return engine
}

// mustRender renders a named template and fails the test on an error.
func mustRender(t *testing.T, engine *templates.Engine, name string, ctx map[string]any) string {
	t.Helper()
	out, err := engine.Render(name, ctx)
	if err != nil {
		t.Fatalf("render %s: %v", name, err)
	}
	return out
}

// wantMissing asserts that err is a TemplateDoesNotExist and returns it.
func wantMissing(t *testing.T, err error) *templates.TemplateDoesNotExist {
	t.Helper()
	var miss *templates.TemplateDoesNotExist
	if !errors.As(err, &miss) || !errors.Is(err, templates.ErrTemplateDoesNotExist) {
		t.Fatalf("error = %v, want a TemplateDoesNotExist", err)
	}
	return miss
}

// TestGetTemplate checks that the sources are searched in order: the project
// directory wins over the app for a name both have, and the app serves what
// the project does not have.
//
// Ported from Django: tests/template_loader/tests.py::TemplateLoaderTests.test_get_template_first_engine
// Ported from Django: tests/template_loader/tests.py::TemplateLoaderTests.test_get_template_second_engine
// Ported from Django: tests/template_loader/tests.py::TemplateLoaderTests.test_render_to_string_first_engine
// Ported from Django: tests/template_loader/tests.py::TemplateLoaderTests.test_render_to_string_second_engine
func TestGetTemplate(t *testing.T) {
	engine := loaderEngine(t)
	if got, want := mustRender(t, engine, "template_loader/hello.html", nil), "Hello! (project directory)\n"; got != want {
		t.Errorf("hello = %q, want %q", got, want)
	}
	if got, want := mustRender(t, engine, "template_loader/goodbye.html", nil), "Goodbye! (app templates)\n"; got != want {
		t.Errorf("goodbye = %q, want %q", got, want)
	}
}

// TestGetTemplateNotFound checks the error for a name no source has: it lists
// every candidate, in search order.
//
// Ported from Django: tests/template_loader/tests.py::TemplateLoaderTests.test_get_template_not_found
// Ported from Django: tests/template_loader/tests.py::TemplateLoaderTests.test_render_to_string_not_found
func TestGetTemplateNotFound(t *testing.T) {
	engine := loaderEngine(t)
	_, err := engine.Render("template_loader/unknown.html", nil)
	miss := wantMissing(t, err)
	if miss.Name != "template_loader/unknown.html" {
		t.Errorf("Name = %q", miss.Name)
	}
	if len(miss.Tried) != 2 {
		t.Fatalf("Tried = %v, want one entry per source", miss.Tried)
	}
	if !strings.HasSuffix(filepath.ToSlash(miss.Tried[0].Origin), "/template_loader/unknown.html") ||
		miss.Tried[0].Reason != "Source does not exist" {
		t.Errorf("Tried[0] = %+v", miss.Tried[0])
	}
	if got, want := miss.Tried[1].Origin, "template_loader:template_loader/unknown.html"; got != want {
		t.Errorf("Tried[1].Origin = %q, want %q", got, want)
	}
	if !strings.Contains(err.Error(), "TemplateDoesNotExist: template_loader/unknown.html") {
		t.Errorf("message = %q", err.Error())
	}
}

// TestSelectTemplate checks the lookup of the first existing name.
//
// Ported from Django: tests/template_loader/tests.py::TemplateLoaderTests.test_select_template_first_engine
// Ported from Django: tests/template_loader/tests.py::TemplateLoaderTests.test_select_template_second_engine
// Ported from Django: tests/template_loader/tests.py::TemplateLoaderTests.test_select_template_tries_all_engines_before_names
// Ported from Django: tests/template_loader/tests.py::TemplateLoaderTests.test_render_to_string_with_list_first_engine
// Ported from Django: tests/template_loader/tests.py::TemplateLoaderTests.test_render_to_string_with_list_second_engine
// Ported from Django: tests/template_loader/tests.py::TemplateLoaderTests.test_render_to_string_with_list_tries_all_engines_before_names
func TestSelectTemplate(t *testing.T) {
	engine := loaderEngine(t)
	tests := []struct {
		names []string
		want  string
	}{
		{[]string{"template_loader/unknown.html", "template_loader/hello.html"}, "Hello! (project directory)\n"},
		{[]string{"template_loader/unknown.html", "template_loader/goodbye.html"}, "Goodbye! (app templates)\n"},
		// Every source is asked for the first name before the second name is
		// considered at all.
		{[]string{"template_loader/goodbye.html", "template_loader/hello.html"}, "Goodbye! (app templates)\n"},
	}
	for _, tt := range tests {
		name, err := engine.SelectTemplate(tt.names...)
		if err != nil {
			t.Fatalf("SelectTemplate(%v): %v", tt.names, err)
		}
		if got := mustRender(t, engine, name, nil); got != tt.want {
			t.Errorf("SelectTemplate(%v) rendered %q, want %q", tt.names, got, tt.want)
		}
	}
}

// TestSelectTemplateEmpty checks the error for an empty list of names. Django's
// test_select_template_string (a string in place of a list) has no
// counterpart: the Go signature takes names only.
//
// Ported from Django: tests/template_loader/tests.py::TemplateLoaderTests.test_select_template_empty
// Ported from Django: tests/template_loader/tests.py::TemplateLoaderTests.test_render_to_string_with_list_empty
func TestSelectTemplateEmpty(t *testing.T) {
	_, err := loaderEngine(t).SelectTemplate()
	if miss := wantMissing(t, err); miss.Name != "No template names provided" {
		t.Errorf("Name = %q", miss.Name)
	}
}

// TestSelectTemplateNotFound checks that the error names every name and lists
// the candidates of all of them.
//
// Ported from Django: tests/template_loader/tests.py::TemplateLoaderTests.test_select_template_not_found
// Ported from Django: tests/template_loader/tests.py::TemplateLoaderTests.test_render_to_string_with_list_not_found
func TestSelectTemplateNotFound(t *testing.T) {
	_, err := loaderEngine(t).SelectTemplate("template_loader/unknown.html", "template_loader/missing.html")
	miss := wantMissing(t, err)
	if got, want := miss.Name, "template_loader/unknown.html, template_loader/missing.html"; got != want {
		t.Errorf("Name = %q, want %q", got, want)
	}
	if len(miss.Tried) != 4 {
		t.Fatalf("Tried has %d entries, want 4: %v", len(miss.Tried), miss.Tried)
	}
	if !strings.HasSuffix(miss.Tried[0].Origin, "unknown.html") || !strings.HasSuffix(miss.Tried[3].Origin, "missing.html") {
		t.Errorf("Tried = %v", miss.Tried)
	}
}

// TestRenderWithRequest checks that a request-bound render has the request.
//
// Ported from Django: tests/template_loader/tests.py::TemplateLoaderTests.test_render_to_string_with_request
func TestRenderWithRequest(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/foobar/", nil)
	got, err := loaderEngine(t).RenderRequest(r, "template_loader/request.html", nil)
	if err != nil {
		t.Fatalf("RenderRequest: %v", err)
	}
	if want := "/foobar/\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestFileSystemLoader checks loading from directories.
//
// Ported from Django: tests/template_tests/test_loaders.py::FileSystemLoaderTests.test_get_template
// Ported from Django: tests/template_tests/test_loaders.py::FileSystemLoaderTests.test_loaders_dirs
// Ported from Django: tests/template_tests/test_loaders.py::FileSystemLoaderTests.test_unicode_template_name
// Ported from Django: tests/template_tests/test_loaders.py::FileSystemLoaderTests.test_unicode_dir_name
// Ported from Django: tests/template_tests/test_loaders.py::FileSystemLoaderTests.test_file_does_not_exist
func TestFileSystemLoader(t *testing.T) {
	dir1 := writeTemplates(t, map[string]string{"index.html": "index\n", "Ångström": "dir1"})
	dir2 := writeTemplates(t, map[string]string{"Straße/Ångström": "dir2", "second.html": "second"})
	engine := templates.New(templates.DirSource(dir1), templates.DirSource(filepath.Join(dir2, "Straße")))
	if got := mustRender(t, engine, "index.html", nil); got != "index\n" {
		t.Errorf("index.html = %q", got)
	}
	if got := mustRender(t, engine, "Ångström", nil); got != "dir1" {
		t.Errorf("Ångström = %q, want the first directory's", got)
	}
	_, err := engine.Render("doesnotexist.html", nil)
	miss := wantMissing(t, err)
	want := []string{filepath.Join(dir1, "doesnotexist.html"), filepath.Join(dir2, "Straße", "doesnotexist.html")}
	if len(miss.Tried) != 2 || miss.Tried[0].Origin != want[0] || miss.Tried[1].Origin != want[1] {
		t.Errorf("Tried = %v, want origins %v", miss.Tried, want)
	}
}

// TestLoadersDirsEmpty checks an engine without sources.
//
// Ported from Django: tests/template_tests/test_loaders.py::FileSystemLoaderTests.test_loaders_dirs_empty
// Ported from Django: tests/template_tests/test_loaders.py::AppDirectoriesLoaderTests.test_not_installed
func TestLoadersDirsEmpty(t *testing.T) {
	_, err := templates.New().Render("index.html", nil)
	if miss := wantMissing(t, err); len(miss.Tried) != 0 {
		t.Errorf("Tried = %v, want none", miss.Tried)
	}
}

// TestDirectorySecurity checks that a name cannot reach outside a source.
// Django accepts an absolute name that lies inside a template directory and a
// "../" name that lands in another one; here a name is always relative to the
// source and every such name is in no source.
//
// Ported from Django: tests/template_tests/test_loaders.py::FileSystemLoaderTests.test_directory_security
func TestDirectorySecurity(t *testing.T) {
	root := writeTemplates(t, map[string]string{
		"dir1/index.html":      "one",
		"dir1/etc/passwd":      "inside",
		"dir2/index.html":      "two",
		"dir1blah":             "outside",
		"etc/passwd":           "outside",
		"dir1/sub/nested.html": "nested",
	})
	dir1, dir2 := filepath.Join(root, "dir1"), filepath.Join(root, "dir2")
	engine := templates.New(templates.DirSource(dir1), templates.DirSource(dir2))
	if got := mustRender(t, engine, "index.html", nil); got != "one" {
		t.Errorf("index.html = %q", got)
	}
	if got := mustRender(t, engine, "etc/passwd", nil); got != "inside" {
		t.Errorf("etc/passwd = %q", got)
	}
	if got := mustRender(t, engine, "sub/../index.html", nil); got != "one" {
		t.Errorf("a name that stays inside the source = %q", got)
	}
	for _, name := range []string{
		"/etc/passwd", "../etc/passwd", "../../../etc/passwd", filepath.Join(dir1, "index.html"),
		"../dir2/index.html", "/dir1blah", "../dir1blah", "sub/../../dir1blah", "",
	} {
		_, err := engine.Render(name, nil)
		if !errors.Is(err, templates.ErrTemplateDoesNotExist) {
			t.Errorf("Render(%q) error = %v, want TemplateDoesNotExist", name, err)
		}
	}
}

// TestCaseSensitivity checks that names are matched exactly.
//
// Ported from Django: tests/template_tests/test_loaders.py::FileSystemLoaderTests.test_case_sensitivity
func TestCaseSensitivity(t *testing.T) {
	engine := templates.New(templates.MapSource("mem", map[string]string{"index.html": "index"}))
	if _, err := engine.Render("index.HTML", nil); !errors.Is(err, templates.ErrTemplateDoesNotExist) {
		t.Errorf("Render(index.HTML) error = %v, want TemplateDoesNotExist", err)
	}
}

// TestNotAFileError checks that a source that has the name but cannot deliver
// it fails the load with the cause, not with TemplateDoesNotExist.
//
// Ported from Django: tests/template_tests/test_loaders.py::FileSystemLoaderTests.test_notafile_error
func TestNotAFileError(t *testing.T) {
	dir := writeTemplates(t, map[string]string{"first/test.html": "x"})
	_, err := templates.New(templates.DirSource(dir)).Render("first", nil)
	if err == nil || errors.Is(err, templates.ErrTemplateDoesNotExist) {
		t.Fatalf("error = %v, want the read error of the directory", err)
	}
	if !strings.Contains(err.Error(), "is a directory") {
		t.Errorf("message = %q, want it to say the name is a directory", err.Error())
	}
}

// TestPermissionsError checks an unreadable template file.
//
// Ported from Django: tests/template_tests/test_loaders.py::FileSystemLoaderTests.test_permissions_error
func TestPermissionsError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads files of any mode")
	}
	dir := writeTemplates(t, map[string]string{"secret.html": "x"})
	if err := os.Chmod(filepath.Join(dir, "secret.html"), 0o222); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	_, err := templates.New(templates.DirSource(dir)).Render("secret.html", nil)
	if !errors.Is(err, fs.ErrPermission) {
		t.Fatalf("error = %v, want a permission error", err)
	}
}

// TestAppDirectoriesLoader checks loading from the templates an app embeds.
//
// Ported from Django: tests/template_tests/test_loaders.py::AppDirectoriesLoaderTests.test_get_template
func TestAppDirectoriesLoader(t *testing.T) {
	app := fstest.MapFS{"index.html": {Data: []byte("index\n")}}
	engine := templates.New(templates.FSSource("template_tests", app))
	if got := mustRender(t, engine, "index.html", nil); got != "index\n" {
		t.Errorf("index.html = %q", got)
	}
	if got, want := engine.Sources(), []string{"template_tests"}; len(got) != 1 || got[0] != want[0] {
		t.Errorf("Sources = %v, want %v", got, want)
	}
}

// TestLocmemLoader checks loading from templates held in memory.
//
// Ported from Django: tests/template_tests/test_loaders.py::LocmemLoaderTests.test_get_template
func TestLocmemLoader(t *testing.T) {
	engine := templates.New(templates.MapSource("locmem", map[string]string{"index.html": "index"}))
	if got := mustRender(t, engine, "index.html", nil); got != "index" {
		t.Errorf("index.html = %q", got)
	}
}

// countingFS counts the reads of a file system.
type countingFS struct {
	fs.FS
	opens atomic.Int64
}

// Open counts and delegates.
func (c *countingFS) Open(name string) (fs.File, error) {
	c.opens.Add(1)
	return c.FS.Open(name)
}

// TestCachedLoader checks that a template is read and compiled once, and that
// debug mode reads it again on every render. Django configures this with the
// cached loader; here it is the engine's default and SetDebug turns it off.
//
// Ported from Django: tests/template_tests/test_loaders.py::CachedLoaderTests.test_get_template
// Ported from Django: tests/template_tests/test_loaders.py::CachedLoaderTests.test_get_template_missing_debug_off
// Ported from Django: tests/template_tests/test_loaders.py::CachedLoaderTests.test_get_template_missing_debug_on
func TestCachedLoader(t *testing.T) {
	counting := &countingFS{FS: fstest.MapFS{"index.html": {Data: []byte("index\n")}}}
	engine := templates.New(templates.FSSource("app", counting))
	for range 3 {
		mustRender(t, engine, "index.html", nil)
	}
	if got := counting.opens.Load(); got != 1 {
		t.Errorf("the template was read %d times, want once", got)
	}
	for _, debug := range []bool{false, true} {
		engine.SetDebug(debug)
		_, err := engine.Render("debug-template-missing.html", nil)
		if miss := wantMissing(t, err); len(miss.Tried) != 1 {
			t.Errorf("debug=%v: Tried = %v, want the one source", debug, miss.Tried)
		}
	}
	before := counting.opens.Load()
	mustRender(t, engine, "index.html", nil)
	mustRender(t, engine, "index.html", nil)
	if got := counting.opens.Load() - before; got != 2 {
		t.Errorf("with debug on the template was read %d times in two renders, want 2", got)
	}
}
