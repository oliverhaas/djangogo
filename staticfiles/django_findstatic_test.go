package staticfiles_test

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/staticfiles"
)

// The tests in this file are ports of TestFindStatic in Django's
// tests/staticfiles_tests/test_management.py. The findstatic command prints
// what FindStaticOutput returns, so the tests call the function.
//
// Django prints the absolute path of every match. A file of an app that is an
// in-memory filesystem has none, so its location names the app and the path
// inside it, and the assertions on the "apps" directory are adapted to that.
//
// Not ported: TestFindStatic.test_missing_args_message, which is about the
// argument parser of the command.

// findstaticFile is TestFindStatic._get_file: it reads the file that
// findstatic --first names at verbosity 0.
func findstaticFile(t *testing.T, finder *staticfiles.Finder) getFile {
	t.Helper()
	return func(path string) (string, error) {
		out, found := staticfiles.FindStaticOutput(finder, path, false, 0)
		if !found {
			return "", fmt.Errorf("findstatic: no match for '%s'", path)
		}
		location := strings.TrimSuffix(out, "\n")
		match, _ := finder.Find(path)
		if match.Source.Root == "" {
			// The file is in memory: the output names it and the merged
			// filesystem of the finder reads it.
			if location != match.Location {
				return "", fmt.Errorf("findstatic printed %q, want %q", location, match.Location)
			}
			data, err := fs.ReadFile(finder, path)
			return string(data), err
		}
		data, err := os.ReadFile(location)
		return string(data), err
	}
}

// outputLines splits the output of a command into its lines, without the
// final newline, as Django's tests split the returned string.
func outputLines(out string) []string {
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	for i, line := range lines {
		lines[i] = strings.TrimSpace(line)
	}
	return lines
}

// realPath is os.path.realpath, which findstatic applies to every match.
func realPath(t *testing.T, path string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatalf("resolve %s: %v", path, err)
	}
	return resolved
}

// Ported from Django: tests/staticfiles_tests/test_management.py::TestFindStatic.test_staticfiles_dirs
func TestFindStaticStaticfilesDirs(t *testing.T) {
	checkStaticfilesDirs(t, findstaticFile(t, newSite(t).finder()))
}

// Ported from Django: tests/staticfiles_tests/test_management.py::TestFindStatic.test_staticfiles_dirs_subdir
func TestFindStaticStaticfilesDirsSubdir(t *testing.T) {
	checkStaticfilesDirsSubdir(t, findstaticFile(t, newSite(t).finder()))
}

// Ported from Django: tests/staticfiles_tests/test_management.py::TestFindStatic.test_staticfiles_dirs_priority
func TestFindStaticStaticfilesDirsPriority(t *testing.T) {
	checkStaticfilesDirsPriority(t, findstaticFile(t, newSite(t).finder()))
}

// Ported from Django: tests/staticfiles_tests/test_management.py::TestFindStatic.test_app_files
func TestFindStaticAppFiles(t *testing.T) {
	checkAppFiles(t, findstaticFile(t, newSite(t).finder()))
}

// Ported from Django: tests/staticfiles_tests/test_management.py::TestFindStatic.test_nonascii_filenames
func TestFindStaticNonasciiFilenames(t *testing.T) {
	checkNonasciiFilenames(t, findstaticFile(t, newSite(t).finder()))
}

// Ported from Django: tests/staticfiles_tests/test_management.py::TestFindStatic.test_camelcase_filenames
func TestFindStaticCamelcaseFilenames(t *testing.T) {
	checkCamelcaseFilenames(t, findstaticFile(t, newSite(t).finder()))
}

// Ported from Django: tests/staticfiles_tests/test_management.py::TestFindStatic.test_filename_with_percent_sign
func TestFindStaticFilenameWithPercentSign(t *testing.T) {
	checkFilenameWithPercentSign(t, findstaticFile(t, newSite(t).finder()))
}

// TestFindStaticAllFiles returns all candidate files when run without --first
// at verbosity 1.
//
// Ported from Django: tests/staticfiles_tests/test_management.py::TestFindStatic.test_all_files
func TestFindStaticAllFiles(t *testing.T) {
	s := newSite(t)

	out, found := staticfiles.FindStaticOutput(s.finder(), "test/file.txt", true, 1)

	djangotest.AssertEqual(t, found, true)
	// Three lines, because there is also the "Found <file> here" line.
	djangotest.AssertSequenceEqual(t, outputLines(out), []string{
		"Found 'test/file.txt' here:",
		realPath(t, s.path("documents", "test", "file.txt")),
		"test:test/file.txt",
	})
}

// TestFindStaticAllFilesLessVerbose returns all candidate files when run
// without --first at verbosity 0.
//
// Ported from Django: tests/staticfiles_tests/test_management.py::TestFindStatic.test_all_files_less_verbose
func TestFindStaticAllFilesLessVerbose(t *testing.T) {
	s := newSite(t)

	out, found := staticfiles.FindStaticOutput(s.finder(), "test/file.txt", true, 0)

	djangotest.AssertEqual(t, found, true)
	djangotest.AssertSequenceEqual(t, outputLines(out), []string{
		realPath(t, s.path("documents", "test", "file.txt")),
		"test:test/file.txt",
	})
}

// TestFindStaticAllFilesMoreVerbose returns all candidate files and the
// searched locations when run without --first at verbosity 2.
//
// Ported from Django: tests/staticfiles_tests/test_management.py::TestFindStatic.test_all_files_more_verbose
func TestFindStaticAllFilesMoreVerbose(t *testing.T) {
	s := newSite(t)

	out, found := staticfiles.FindStaticOutput(s.finder(), "test/file.txt", true, 2)

	djangotest.AssertEqual(t, found, true)
	lines := outputLines(out)
	if len(lines) < 4 {
		t.Fatalf("findstatic printed %d lines, want at least 4:\n%s", len(lines), out)
	}
	assertIn(t, lines[1], "project")
	djangotest.AssertEqual(t, lines[2], "test:test/file.txt")
	assertIn(t, lines[3], "Looking in the following locations:")
	djangotest.AssertSequenceEqual(t, lines[4:], []string{
		// The FileSystemFinder locations.
		s.path("documents"),
		s.path("prefixed"),
		s.path("pathlib"),
		// The AppDirectoriesFinder locations, which are app names here.
		"test",
		"no_label",
		// The DefaultStorageFinder location.
		s.path("site_media", "media"),
	})
}
