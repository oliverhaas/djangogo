package staticfiles_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/staticfiles"
)

// The tests in this file are ports of the finder checks in Django's
// tests/staticfiles_tests/test_checks.py. Django reports a misconfigured
// STATICFILES_DIRS through its system check framework, which collectstatic
// runs first. This package has no check framework: Collect itself refuses the
// one configuration that destroys files (STATIC_ROOT among the sources), and
// the other two findings are not errors here.
//
// Not ported: the tests of the check framework (test_check_finders,
// test_base_finder_check_not_implemented, test_no_errors_with_test_settings),
// test_dirs_not_tuple_or_list (a setting of the wrong Python type), and
// StoragesCheckTests (the STORAGES setting).

// staticRootError is the text of Django's check staticfiles.E002.
const staticRootError = "The STATICFILES_DIRS setting should not contain the STATIC_ROOT setting."

// TestDirsContainsStaticRoot refuses to collect into a directory that is also
// a source, before --clear deletes the files of that source.
//
// Ported from Django: tests/staticfiles_tests/test_checks.py::FindersCheckTests.test_dirs_contains_static_root
func TestDirsContainsStaticRoot(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{"site.css": "body {}\n"})
	finder := staticfiles.NewFinder(staticfiles.DirSource(root))

	_, err := staticfiles.Collect(finder, staticfiles.CollectOptions{Root: root, Clear: true})

	djangotest.AssertErrorIs(t, err, staticfiles.ErrStaticRootInSources)
	if err != nil {
		djangotest.AssertEqual(t, err.Error(), staticRootError)
	}
	assertFileContains(t, staticRoot(root), "site.css", "body {}")
}

// Ported from Django: tests/staticfiles_tests/test_checks.py::FindersCheckTests.test_dirs_contains_static_root_in_tuple
func TestDirsContainsStaticRootInTuple(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{"site.css": "body {}\n"})
	source := staticfiles.DirSource(root)
	source.Prefix = "prefix"
	// The same directory under another spelling is still the same directory.
	// The path is put together by hand because filepath.Join would clean it.
	sep := string(filepath.Separator)
	spelled := root + sep + "sub" + sep + ".."

	_, err := staticfiles.Collect(staticfiles.NewFinder(source), staticfiles.CollectOptions{Root: spelled, Clear: true})

	djangotest.AssertErrorIs(t, err, staticfiles.ErrStaticRootInSources)
	assertFileContains(t, staticRoot(root), "site.css", "body {}")
}

// TestPrefixContainsTrailingSlash accepts a prefix that ends with a slash.
// Django reports it as error staticfiles.E003, because its finder would never
// match such a prefix; Source ignores the slash, so nothing is wrong with it.
//
// Ported from Django: tests/staticfiles_tests/test_checks.py::FindersCheckTests.test_prefix_contains_trailing_slash
func TestPrefixContainsTrailingSlash(t *testing.T) {
	s := newSite(t)
	source := staticfiles.DirSource(s.path("documents"))
	source.Prefix = "prefix/"
	finder := staticfiles.NewFinder(source)

	found, ok := finder.Find("prefix/test.txt")

	djangotest.AssertEqual(t, ok, true)
	djangotest.AssertEqual(t, found.Location, s.path("documents", "test.txt"))
}

// TestNonexistentDirectories skips the directories that do not exist. Django
// also warns about each of them (staticfiles.W004); here a missing directory
// is an empty source and nothing reports it.
//
// Ported from Django: tests/staticfiles_tests/test_checks.py::FindersCheckTests.test_nonexistent_directories
func TestNonexistentDirectories(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "fake", "path")
	prefixed := staticfiles.DirSource(filepath.Join(t.TempDir(), "fake", "prefixed", "path"))
	prefixed.Prefix = "prefix"
	finder := staticfiles.NewFinder(staticfiles.DirSource(missing), prefixed)

	files, err := finder.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	djangotest.AssertEqual(t, len(files), 0)
	if _, err := os.Stat(missing); !os.IsNotExist(err) {
		t.Errorf("listing created %s", missing)
	}
}
