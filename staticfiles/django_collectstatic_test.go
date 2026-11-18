package staticfiles_test

import (
	"os"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/staticfiles"
)

// The tests in this file are ports of the collection tests of Django's
// tests/staticfiles_tests/test_management.py: which files collectstatic
// copies. The tests of what it prints are in django_collectstatic_output_test.go
// and the findstatic tests in django_findstatic_test.go.
//
// Not ported, because they test the command line or a feature outside this
// package: TestCollectionHelpSubcommand, TestInteractiveMessages (the
// confirmation prompt), the tests of TestCollectionLinks that need --link, the
// post-processing and manifest storage tests, TestCollectionPathLib (a
// pathlib.Path as STATIC_ROOT, which has no counterpart where paths are
// strings), and the tests of storages that are not directories
// (TestCollectionNonLocalStorage,
// TestCollectionClear.test_handle_path_notimplemented,
// TestConfiguration.test_local_storage_detection_helper).
// TestConfiguration.test_collectstatis_check is about a setting of the wrong
// Python type, which cannot be written here.

// Ported from Django: tests/staticfiles_tests/test_management.py::TestCollection.test_staticfiles_dirs
func TestCollectionStaticfilesDirs(t *testing.T) {
	checkStaticfilesDirs(t, collection(t, staticfiles.CollectOptions{}))
}

// Ported from Django: tests/staticfiles_tests/test_management.py::TestCollection.test_staticfiles_dirs_subdir
func TestCollectionStaticfilesDirsSubdir(t *testing.T) {
	checkStaticfilesDirsSubdir(t, collection(t, staticfiles.CollectOptions{}))
}

// Ported from Django: tests/staticfiles_tests/test_management.py::TestCollection.test_staticfiles_dirs_priority
func TestCollectionStaticfilesDirsPriority(t *testing.T) {
	checkStaticfilesDirsPriority(t, collection(t, staticfiles.CollectOptions{}))
}

// Ported from Django: tests/staticfiles_tests/test_management.py::TestCollection.test_app_files
func TestCollectionAppFiles(t *testing.T) {
	checkAppFiles(t, collection(t, staticfiles.CollectOptions{}))
}

// Ported from Django: tests/staticfiles_tests/test_management.py::TestCollection.test_nonascii_filenames
func TestCollectionNonasciiFilenames(t *testing.T) {
	checkNonasciiFilenames(t, collection(t, staticfiles.CollectOptions{}))
}

// Ported from Django: tests/staticfiles_tests/test_management.py::TestCollection.test_camelcase_filenames
func TestCollectionCamelcaseFilenames(t *testing.T) {
	checkCamelcaseFilenames(t, collection(t, staticfiles.CollectOptions{}))
}

// Ported from Django: tests/staticfiles_tests/test_management.py::TestCollection.test_filename_with_percent_sign
func TestCollectionFilenameWithPercentSign(t *testing.T) {
	checkFilenameWithPercentSign(t, collection(t, staticfiles.CollectOptions{}))
}

// TestCollectionIgnore leaves out the files that match a -i pattern.
//
// Ported from Django: tests/staticfiles_tests/test_management.py::TestCollection.test_ignore
func TestCollectionIgnore(t *testing.T) {
	get := collection(t, staticfiles.CollectOptions{})
	assertFileNotFound(t, get, "test/test.ignoreme")
}

// TestCollectionCommonIgnorePatterns leaves out the files that match the
// common ignore patterns (*~, .*, CVS).
//
// Ported from Django: tests/staticfiles_tests/test_management.py::TestCollection.test_common_ignore_patterns
func TestCollectionCommonIgnorePatterns(t *testing.T) {
	get := collection(t, staticfiles.CollectOptions{})
	assertFileNotFound(t, get, "test/.hidden")
	assertFileNotFound(t, get, "test/backup~")
	assertFileNotFound(t, get, "test/CVS")
}

// TestCollectionPathlib collects from the STATICFILES_DIRS entry that Django
// configures as a pathlib.Path; here it is a directory like the others.
//
// Ported from Django: tests/staticfiles_tests/test_management.py::TestCollection.test_pathlib
func TestCollectionPathlib(t *testing.T) {
	get := collection(t, staticfiles.CollectOptions{})
	assertFileContains(t, get, "pathlib.txt", "pathlib")
}

// Ported from Django: tests/staticfiles_tests/test_management.py::TestCollectionNoDefaultIgnore.test_staticfiles_dirs
func TestCollectionNoDefaultIgnoreStaticfilesDirs(t *testing.T) {
	checkStaticfilesDirs(t, collection(t, staticfiles.CollectOptions{NoDefaultIgnore: true}))
}

// Ported from Django: tests/staticfiles_tests/test_management.py::TestCollectionNoDefaultIgnore.test_staticfiles_dirs_subdir
func TestCollectionNoDefaultIgnoreStaticfilesDirsSubdir(t *testing.T) {
	checkStaticfilesDirsSubdir(t, collection(t, staticfiles.CollectOptions{NoDefaultIgnore: true}))
}

// Ported from Django: tests/staticfiles_tests/test_management.py::TestCollectionNoDefaultIgnore.test_staticfiles_dirs_priority
func TestCollectionNoDefaultIgnoreStaticfilesDirsPriority(t *testing.T) {
	checkStaticfilesDirsPriority(t, collection(t, staticfiles.CollectOptions{NoDefaultIgnore: true}))
}

// Ported from Django: tests/staticfiles_tests/test_management.py::TestCollectionNoDefaultIgnore.test_app_files
func TestCollectionNoDefaultIgnoreAppFiles(t *testing.T) {
	checkAppFiles(t, collection(t, staticfiles.CollectOptions{NoDefaultIgnore: true}))
}

// Ported from Django: tests/staticfiles_tests/test_management.py::TestCollectionNoDefaultIgnore.test_nonascii_filenames
func TestCollectionNoDefaultIgnoreNonasciiFilenames(t *testing.T) {
	checkNonasciiFilenames(t, collection(t, staticfiles.CollectOptions{NoDefaultIgnore: true}))
}

// Ported from Django: tests/staticfiles_tests/test_management.py::TestCollectionNoDefaultIgnore.test_camelcase_filenames
func TestCollectionNoDefaultIgnoreCamelcaseFilenames(t *testing.T) {
	checkCamelcaseFilenames(t, collection(t, staticfiles.CollectOptions{NoDefaultIgnore: true}))
}

// Ported from Django: tests/staticfiles_tests/test_management.py::TestCollectionNoDefaultIgnore.test_filename_with_percent_sign
func TestCollectionNoDefaultIgnoreFilenameWithPercentSign(t *testing.T) {
	checkFilenameWithPercentSign(t, collection(t, staticfiles.CollectOptions{NoDefaultIgnore: true}))
}

// TestCollectionNoDefaultIgnoreNoCommonIgnorePatterns collects the files that
// match the common ignore patterns (*~, .*, CVS) under --no-default-ignore.
//
// Ported from Django: tests/staticfiles_tests/test_management.py::TestCollectionNoDefaultIgnore.test_no_common_ignore_patterns
func TestCollectionNoDefaultIgnoreNoCommonIgnorePatterns(t *testing.T) {
	get := collection(t, staticfiles.CollectOptions{NoDefaultIgnore: true})
	assertFileContains(t, get, "test/.hidden", "should be ignored")
	assertFileContains(t, get, "test/backup~", "should be ignored")
	assertFileContains(t, get, "test/CVS", "should be ignored")
}

// TestCollectionCustomIgnorePatterns replaces the default patterns with
// "*.css" and "*/vendor/*.js". Django reads the replacement from the
// ignore_patterns attribute of the app config; here the caller passes it with
// NoDefaultIgnore set.
//
// Ported from Django: tests/staticfiles_tests/test_management.py::TestCollectionCustomIgnorePatterns.test_custom_ignore_patterns
func TestCollectionCustomIgnorePatterns(t *testing.T) {
	get := collection(t, staticfiles.CollectOptions{
		NoDefaultIgnore: true,
		IgnorePatterns:  []string{"*.css", "*/vendor/*.js"},
	})
	assertFileNotFound(t, get, "test/nonascii.css")
	assertFileContains(t, get, "test/.hidden", "should be ignored")
	assertFileNotFound(t, get, "test/vendor/module.js")
}

// TestCollectionDryRunNoFilesCreated creates no file in the destination
// directory under --dry-run.
//
// Ported from Django: tests/staticfiles_tests/test_management.py::TestCollectionDryRun.test_no_files_created
func TestCollectionDryRunNoFilesCreated(t *testing.T) {
	root := t.TempDir()
	runCollectstatic(t, newSite(t).finder(), staticfiles.CollectOptions{Root: root, DryRun: true})

	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read STATIC_ROOT: %v", err)
	}
	djangotest.AssertEqual(t, len(entries), 0)
}

// TestConfigurationLocationEmpty refuses to collect without a STATIC_ROOT.
// Django also tries None, which a string setting cannot hold.
//
// Ported from Django: tests/staticfiles_tests/test_management.py::TestConfiguration.test_location_empty
func TestConfigurationLocationEmpty(t *testing.T) {
	msg := "without having set the STATIC_ROOT setting to a filesystem path"

	_, err := staticfiles.Collect(newSite(t).finder(), staticfiles.CollectOptions{Root: ""})

	if err == nil || !strings.Contains(err.Error(), msg) {
		t.Errorf("Collect error = %v, want one containing %q", err, msg)
	}
	djangotest.AssertErrorIs(t, err, staticfiles.ErrStaticRootNotSet)
}
