package staticfiles_test

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/staticfiles"
)

// The tests in this file are ports of the tests of Django's
// tests/staticfiles_tests/test_management.py that are about what collectstatic
// prints and about which of two files wins: --clear, the verbosity levels, the
// conflict warning, and the order of the sources.

// Ported from Django: tests/staticfiles_tests/test_management.py::TestCollectionVerbosity.test_verbosity_0
func TestCollectionVerbosity0(t *testing.T) {
	out := runCollectstatic(t, newSite(t).finder(), staticfiles.CollectOptions{Root: t.TempDir(), Verbosity: 0})
	djangotest.AssertEqual(t, out, "")
}

// Ported from Django: tests/staticfiles_tests/test_management.py::TestCollectionVerbosity.test_verbosity_1
func TestCollectionVerbosity1(t *testing.T) {
	out := runCollectstatic(t, newSite(t).finder(), staticfiles.CollectOptions{Root: t.TempDir(), Verbosity: 1})
	assertIn(t, out, "static files copied to")
	assertNotIn(t, out, "Copying ")
}

// Ported from Django: tests/staticfiles_tests/test_management.py::TestCollectionVerbosity.test_verbosity_2
func TestCollectionVerbosity2(t *testing.T) {
	out := runCollectstatic(t, newSite(t).finder(), staticfiles.CollectOptions{Root: t.TempDir(), Verbosity: 2})
	assertIn(t, out, "static files copied to")
	assertIn(t, out, "Copying ")
}

// clearCollectstatic is TestCollectionClear.run_collectstatic: it leaves a
// file in STATIC_ROOT and collects with --clear.
func clearCollectstatic(t *testing.T, finder *staticfiles.Finder, opts staticfiles.CollectOptions) string {
	t.Helper()
	cleared := filepath.Join(opts.Root, "cleared.txt")
	if err := os.WriteFile(cleared, []byte("should be cleared"), 0o644); err != nil {
		t.Fatalf("write cleared.txt: %v", err)
	}
	opts.Clear = true
	return runCollectstatic(t, finder, opts)
}

// TestCollectionClearClearedNotFound deletes a stale file under --clear.
// Django's test never runs the command (run_collectstatic_in_setUp is off for
// the class), so upstream it passes on an empty directory; the port runs it.
//
// Ported from Django: tests/staticfiles_tests/test_management.py::TestCollectionClear.test_cleared_not_found
func TestCollectionClearClearedNotFound(t *testing.T) {
	root := t.TempDir()
	clearCollectstatic(t, newSite(t).finder(), staticfiles.CollectOptions{Root: root})

	get := staticRoot(root)
	assertFileNotFound(t, get, "cleared.txt")
	assertFileContains(t, get, "test.txt", "Can we find")
}

// TestCollectionClearDirNotExists clears a STATIC_ROOT that does not exist.
//
// Ported from Django: tests/staticfiles_tests/test_management.py::TestCollectionClear.test_dir_not_exists
func TestCollectionClearDirNotExists(t *testing.T) {
	root := t.TempDir()
	if err := os.RemoveAll(root); err != nil {
		t.Fatalf("remove STATIC_ROOT: %v", err)
	}

	runCollectstatic(t, newSite(t).finder(), staticfiles.CollectOptions{Root: root, Clear: true})

	assertFileContains(t, staticRoot(root), "test.txt", "Can we find")
}

// Ported from Django: tests/staticfiles_tests/test_management.py::TestCollectionClear.test_verbosity_0
func TestCollectionClearVerbosity0(t *testing.T) {
	finder := newSite(t).finder()
	root := t.TempDir()
	for _, dryRun := range []bool{false, true} {
		t.Run(fmt.Sprintf("dry_run=%t", dryRun), func(t *testing.T) {
			out := clearCollectstatic(t, finder, staticfiles.CollectOptions{Root: root, Verbosity: 0, DryRun: dryRun})
			djangotest.AssertEqual(t, out, "")
		})
	}
}

// Ported from Django: tests/staticfiles_tests/test_management.py::TestCollectionClear.test_verbosity_1
func TestCollectionClearVerbosity1(t *testing.T) {
	finder := newSite(t).finder()
	root := t.TempDir()
	for _, tc := range []struct {
		deletionMessage string
		dryRun          bool
	}{
		{"Deleting", false},
		{"Pretending to delete", true},
	} {
		t.Run(fmt.Sprintf("dry_run=%t", tc.dryRun), func(t *testing.T) {
			out := clearCollectstatic(t, finder, staticfiles.CollectOptions{Root: root, Verbosity: 1, DryRun: tc.dryRun})
			assertIn(t, out, "static file")
			assertIn(t, out, "deleted")
			assertNotIn(t, out, tc.deletionMessage)
		})
	}
}

// Ported from Django: tests/staticfiles_tests/test_management.py::TestCollectionClear.test_verbosity_2
func TestCollectionClearVerbosity2(t *testing.T) {
	finder := newSite(t).finder()
	root := t.TempDir()
	for _, tc := range []struct {
		deletionMessage string
		dryRun          bool
	}{
		{"Deleting", false},
		{"Pretending to delete", true},
	} {
		t.Run(fmt.Sprintf("dry_run=%t", tc.dryRun), func(t *testing.T) {
			out := clearCollectstatic(t, finder, staticfiles.CollectOptions{Root: root, Verbosity: 2, DryRun: tc.dryRun})
			assertIn(t, out, "static file")
			assertIn(t, out, "deleted")
			assertIn(t, out, tc.deletionMessage)
		})
	}
}

// TestCollectionFilesOverrideOrderingOverride collects the file of the first
// app that has it, although the file of a later app is newer. Both runs must
// keep it: the second one finds the collected file not older than the first
// source and must not let the newer duplicate overwrite it.
//
// Ported from Django: tests/staticfiles_tests/test_management.py::TestCollectionFilesOverride.test_ordering_override
func TestCollectionFilesOverrideOrderingOverride(t *testing.T) {
	s := newSite(t)
	apps := t.TempDir()

	// The no_label app is a directory here, because the test is about
	// modification times, which an in-memory filesystem does not report.
	noLabel := filepath.Join(apps, "no_label", "static")
	writeTree(t, noLabel, noLabelAppFiles)
	orig, err := os.Stat(filepath.Join(noLabel, "file2.txt"))
	if err != nil {
		t.Fatalf("stat file2.txt: %v", err)
	}

	// The duplicate of file2.txt is older than the original, and its app comes
	// before no_label in INSTALLED_APPS.
	testApp := filepath.Join(apps, "staticfiles_test_app", "static")
	writeTree(t, testApp, map[string]string{"file2.txt": "duplicate of file2.txt"})
	older := orig.ModTime().Add(-time.Second)
	if err := os.Chtimes(filepath.Join(testApp, "file2.txt"), older, older); err != nil {
		t.Fatalf("set modification time: %v", err)
	}

	sources := append([]staticfiles.Source{}, s.dirs...)
	sources = append(sources,
		staticfiles.Source{Name: "staticfiles_test_app", FS: os.DirFS(testApp), Root: testApp},
		s.apps[0],
		staticfiles.Source{Name: "no_label", FS: os.DirFS(noLabel), Root: noLabel},
		s.media,
	)
	finder := staticfiles.NewFinder(sources...)
	root := t.TempDir()
	get := staticRoot(root)

	runCollectstatic(t, finder, staticfiles.CollectOptions{Root: root})
	assertFileContains(t, get, "file2.txt", "duplicate of file2.txt")

	runCollectstatic(t, finder, staticfiles.CollectOptions{Root: root})
	assertFileContains(t, get, "file2.txt", "duplicate of file2.txt")
}

// The TestCollectionOverwriteWarning tests run without STATICFILES_DIRS,
// because the settings of the other tests already hold a conflict: test/file.txt
// is in project/documents and in the test app. Each test adds the conflicting
// directory it needs.

// warningString is in the output when the conflict warning was printed.
const warningString = "Found another file"

// TestCollectionOverwriteWarningNoWarning prints no warning when no two files
// share a destination.
//
// Ported from Django: tests/staticfiles_tests/test_management.py::TestCollectionOverwriteWarning.test_no_warning
func TestCollectionOverwriteWarningNoWarning(t *testing.T) {
	finder := newSite(t).finderWithDirs()

	out := callCollectstatic(t, finder, staticfiles.CollectOptions{Root: t.TempDir(), Clear: true, Verbosity: 3})

	assertNotIn(t, out, warningString)
}

// TestCollectionOverwriteWarningWarningAtVerbosity2 prints the warning for a
// duplicate destination at verbosity 2 and above.
//
// Ported from Django: tests/staticfiles_tests/test_management.py::TestCollectionOverwriteWarning.test_warning_at_verbosity_2
func TestCollectionOverwriteWarningWarningAtVerbosity2(t *testing.T) {
	staticDir := t.TempDir()
	writeTree(t, staticDir, map[string]string{"test/file.txt": "duplicate of file.txt"})
	finder := newSite(t).finderWithDirs(staticDir)

	out := callCollectstatic(t, finder, staticfiles.CollectOptions{Root: t.TempDir(), Clear: true, Verbosity: 2})

	assertIn(t, out, warningString)
}

// TestCollectionOverwriteWarningNoWarningAtVerbosity1 prints no warning per
// file at verbosity 1, but counts the conflict in the summary.
//
// Ported from Django: tests/staticfiles_tests/test_management.py::TestCollectionOverwriteWarning.test_no_warning_at_verbosity_1
func TestCollectionOverwriteWarningNoWarningAtVerbosity1(t *testing.T) {
	staticDir := t.TempDir()
	writeTree(t, staticDir, map[string]string{"test/file.txt": "duplicate of file.txt"})
	finder := newSite(t).finderWithDirs(staticDir)

	out := callCollectstatic(t, finder, staticfiles.CollectOptions{Root: t.TempDir(), Clear: true, Verbosity: 1})

	assertNotIn(t, out, warningString)
	assertIn(t, out, "1 skipped due to conflict")
}

// TestCollectionOverwriteWarningSummaryMultipleConflicts counts every
// conflict in the summary.
//
// Ported from Django: tests/staticfiles_tests/test_management.py::TestCollectionOverwriteWarning.test_summary_multiple_conflicts
func TestCollectionOverwriteWarningSummaryMultipleConflicts(t *testing.T) {
	staticDir := t.TempDir()
	writeTree(t, staticDir, map[string]string{
		"test/file.txt":     "duplicate of file.txt",
		"test/file1.txt":    "duplicate of file1.txt",
		"test/nonascii.css": "duplicate of file.txt",
	})
	finder := newSite(t).finderWithDirs(staticDir)

	out := callCollectstatic(t, finder, staticfiles.CollectOptions{Root: t.TempDir(), Clear: true, Verbosity: 1})

	assertIn(t, out, "3 skipped due to conflict")
}

// TestCollectionNeverCopyStorageSkipsNewerFilesInRemoteStorage skips the
// files whose collected copy is newer. Django swaps in a remote storage that
// reports a modification time in the future for every file. Collect writes to
// a directory only, so the port moves the modification times of the collected
// files into the future instead.
//
// Ported from Django: tests/staticfiles_tests/test_management.py::TestCollectionNeverCopyStorage.test_skips_newer_files_in_remote_storage
func TestCollectionNeverCopyStorageSkipsNewerFilesInRemoteStorage(t *testing.T) {
	finder := newSite(t).finder()
	root := t.TempDir()
	runCollectstatic(t, finder, staticfiles.CollectOptions{Root: root})

	future := time.Now().Add(30 * 24 * time.Hour)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		return os.Chtimes(path, future, future)
	})
	if err != nil {
		t.Fatalf("set modification times: %v", err)
	}

	out := runCollectstatic(t, finder, staticfiles.CollectOptions{Root: root, Verbosity: 2})

	assertIn(t, out, "Skipping 'test.txt' (not modified)")
}
