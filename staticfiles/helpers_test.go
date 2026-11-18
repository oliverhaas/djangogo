package staticfiles_test

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/staticfiles"
)

// The fixtures in this file rebuild the trees of Django's
// tests/staticfiles_tests: project/ holds the STATICFILES_DIRS entries and the
// media root as directories on disk, and apps/ holds the static directories of
// the installed apps, which here are in-memory filesystems, as the embedded
// files of an app are. File names and contents are Django's.

// projectFiles lists tests/staticfiles_tests/project/, slash-separated.
var projectFiles = map[string]string{
	"documents/test.txt":              "Can we find this file?\n",
	"documents/subdir/test.txt":       "Can we find this file?\n",
	"documents/test/backup~":          "should be ignored\n",
	"documents/test/camelCase.txt":    "This file is named with camelCase.",
	"documents/test/file.txt":         "In STATICFILES_DIRS directory.\n\n",
	"prefixed/test.txt":               "Prefix!",
	"pathlib/pathlib.txt":             "pathlib\n",
	"site_media/media/media-file.txt": "Media file.\n",
}

// testAppFiles lists tests/staticfiles_tests/apps/test/static/.
var testAppFiles = map[string]string{
	"test/%2F.txt":          "%2F content\n",
	"test/CVS":              "This file should be ignored.\n",
	"test/file1.txt":        "file1 in the app dir",
	"test/file.txt":         "In static directory.\n",
	"test/.hidden":          "should be ignored\n",
	"test/nonascii.css":     "body {\n  background: url('window.png');\n}\n\n.snowman:before { content: \"☃\"; }\n",
	"test/test.ignoreme":    "This file should be ignored.\n",
	"test/⊗.txt":            "⊗ in the app dir\n",
	"test/vendor/module.js": "",
	"test/window.png":       "\x89PNG\r\n\x1a\n",
}

// noLabelAppFiles lists tests/staticfiles_tests/apps/no_label/static/.
var noLabelAppFiles = map[string]string{
	"file2.txt": "file2 in no_label_app\n",
}

// writeTree creates the files below root; the keys are slash-separated paths.
func writeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		full := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("create fixture directory: %v", err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatalf("write fixture file: %v", err)
		}
	}
}

// mapFS returns the files as an in-memory filesystem. Like an embed.FS it
// reports no modification times.
func mapFS(files map[string]string) fstest.MapFS {
	fsys := fstest.MapFS{}
	for name, content := range files {
		fsys[name] = &fstest.MapFile{Data: []byte(content)}
	}
	return fsys
}

// site is the counterpart of Django's TEST_SETTINGS (settings.py): the sources
// of each of the three finders named in STATICFILES_FINDERS.
type site struct {
	// project is the directory that stands for tests/staticfiles_tests/project.
	project string
	// dirs are the STATICFILES_DIRS entries, searched by FileSystemFinder.
	dirs []staticfiles.Source
	// apps are the static directories of INSTALLED_APPS, searched by
	// AppDirectoriesFinder.
	apps []staticfiles.Source
	// media is MEDIA_ROOT, searched by DefaultStorageFinder.
	media staticfiles.Source
}

// newSite writes the project tree into a temporary directory and returns the
// sources of TEST_SETTINGS.
func newSite(t *testing.T) *site {
	t.Helper()
	project := filepath.Join(t.TempDir(), "project")
	writeTree(t, project, projectFiles)
	prefixed := staticfiles.DirSource(filepath.Join(project, "prefixed"))
	prefixed.Prefix = "prefix"
	return &site{
		project: project,
		dirs: []staticfiles.Source{
			staticfiles.DirSource(filepath.Join(project, "documents")),
			prefixed,
			staticfiles.DirSource(filepath.Join(project, "pathlib")),
		},
		apps: []staticfiles.Source{
			{Name: "test", FS: mapFS(testAppFiles)},
			{Name: "no_label", FS: mapFS(noLabelAppFiles)},
		},
		media: staticfiles.DirSource(filepath.Join(project, "site_media", "media")),
	}
}

// finder returns a Finder over every source, in the order of
// STATICFILES_FINDERS: the directories, the apps, the media root.
func (s *site) finder() *staticfiles.Finder {
	sources := append([]staticfiles.Source{}, s.dirs...)
	sources = append(sources, s.apps...)
	sources = append(sources, s.media)
	return staticfiles.NewFinder(sources...)
}

// finderWithDirs is override_settings(STATICFILES_DIRS=dirs): the finder of
// the site with the given directories in place of its STATICFILES_DIRS.
func (s *site) finderWithDirs(dirs ...string) *staticfiles.Finder {
	sources := make([]staticfiles.Source, 0, len(dirs)+len(s.apps)+1)
	for _, dir := range dirs {
		sources = append(sources, staticfiles.DirSource(dir))
	}
	sources = append(sources, s.apps...)
	sources = append(sources, s.media)
	return staticfiles.NewFinder(sources...)
}

// path returns a path below the project directory.
func (s *site) path(elem ...string) string {
	return filepath.Join(append([]string{s.project}, elem...)...)
}

// assertIn is assertIn(needle, output) for command output.
func assertIn(t *testing.T, output, needle string) {
	t.Helper()
	if !strings.Contains(output, needle) {
		t.Errorf("%q not found in %q", needle, output)
	}
}

// assertNotIn is assertNotIn(needle, output) for command output.
func assertNotIn(t *testing.T, output, needle string) {
	t.Helper()
	if strings.Contains(output, needle) {
		t.Errorf("%q unexpectedly found in %q", needle, output)
	}
}

// getFile reads a static file the way one Django test class does (its
// _get_file or _response method): from STATIC_ROOT, through findstatic, or
// over HTTP.
type getFile func(path string) (string, error)

// assertFileContains is BaseStaticFilesMixin.assertFileContains.
func assertFileContains(t *testing.T, get getFile, path, text string) {
	t.Helper()
	content, err := get(path)
	if err != nil {
		t.Errorf("read '%s': %v", path, err)
		return
	}
	if !strings.Contains(content, text) {
		t.Errorf("'%s' not in '%s'", text, path)
	}
}

// assertFileNotFound is BaseStaticFilesMixin.assertFileNotFound.
func assertFileNotFound(t *testing.T, get getFile, path string) {
	t.Helper()
	if _, err := get(path); err == nil {
		t.Errorf("'%s' was found, want it missing", path)
	}
}

// The check functions below are the test methods of Django's TestDefaults
// mixin (cases.py). Every class that inherits the mixin runs them against its
// own way of reading a file, so each has one ported test function per class.

// checkStaticfilesDirs finds a file in a STATICFILES_DIRS directory.
func checkStaticfilesDirs(t *testing.T, get getFile) {
	t.Helper()
	assertFileContains(t, get, "test.txt", "Can we find")
	assertFileContains(t, get, "prefix/test.txt", "Prefix")
}

// checkStaticfilesDirsSubdir finds a file in a subdirectory of a
// STATICFILES_DIRS directory.
func checkStaticfilesDirsSubdir(t *testing.T, get getFile) {
	t.Helper()
	assertFileContains(t, get, "subdir/test.txt", "Can we find")
}

// checkStaticfilesDirsPriority gives a file in STATICFILES_DIRS priority over
// a file in an app.
func checkStaticfilesDirsPriority(t *testing.T, get getFile) {
	t.Helper()
	assertFileContains(t, get, "test/file.txt", "STATICFILES_DIRS")
}

// checkAppFiles finds a file in the static directory of an app.
func checkAppFiles(t *testing.T, get getFile) {
	t.Helper()
	assertFileContains(t, get, "test/file1.txt", "file1 in the app dir")
}

// checkNonasciiFilenames finds a file with a non-ASCII character in its name.
func checkNonasciiFilenames(t *testing.T, get getFile) {
	t.Helper()
	assertFileContains(t, get, "test/⊗.txt", "⊗ in the app dir")
}

// checkCamelcaseFilenames finds a file with capital letters in its name.
func checkCamelcaseFilenames(t *testing.T, get getFile) {
	t.Helper()
	assertFileContains(t, get, "test/camelCase.txt", "camelCase")
}

// checkFilenameWithPercentSign finds a file whose name looks percent-encoded.
func checkFilenameWithPercentSign(t *testing.T, get getFile) {
	t.Helper()
	assertFileContains(t, get, "test/%2F.txt", "%2F content")
}

// callCollectstatic is call_command("collectstatic", interactive=False, ...):
// Collect logs the per-file lines, and the command prints the summary after a
// blank line at verbosity 1 and above. It returns what the command printed.
func callCollectstatic(t *testing.T, finder *staticfiles.Finder, opts staticfiles.CollectOptions) string {
	t.Helper()
	var out strings.Builder
	opts.Out = &out
	result, err := staticfiles.Collect(finder, opts)
	if err != nil {
		t.Fatalf("collectstatic: %v", err)
	}
	if opts.Verbosity >= 1 {
		fmt.Fprintf(&out, "\n%s\n", result.Summary(opts.Root))
	}
	return out.String()
}

// runCollectstatic is CollectionTestCase.run_collectstatic, which ignores
// "*.ignoreme" on every run.
func runCollectstatic(t *testing.T, finder *staticfiles.Finder, opts staticfiles.CollectOptions) string {
	t.Helper()
	opts.IgnorePatterns = append([]string{"*.ignoreme"}, opts.IgnorePatterns...)
	return callCollectstatic(t, finder, opts)
}

// staticRoot is CollectionTestCase._get_file: it reads a collected file from
// STATIC_ROOT.
func staticRoot(root string) getFile {
	return func(path string) (string, error) {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
		return string(data), err
	}
}

// collection is CollectionTestCase.setUp: it collects the files of
// TEST_SETTINGS into a new STATIC_ROOT at verbosity 0 and returns a reader of
// the collected files.
func collection(t *testing.T, opts staticfiles.CollectOptions) getFile {
	t.Helper()
	opts.Root = t.TempDir()
	runCollectstatic(t, newSite(t).finder(), opts)
	return staticRoot(opts.Root)
}

// quote is urllib.parse.quote for a URL path.
func quote(path string) string {
	return (&url.URL{Path: path}).EscapedPath()
}

// served is TestServeStatic._response: it requests a file below STATIC_URL
// from the handler and returns the body of a 200 response.
func served(t *testing.T, handler http.Handler) getFile {
	t.Helper()
	client := djangotest.NewClient(t, handler)
	return func(path string) (string, error) {
		resp := client.Get(quote("/static/" + path))
		if resp.StatusCode != http.StatusOK {
			return "", fmt.Errorf("status %d", resp.StatusCode)
		}
		return resp.Content(), nil
	}
}
