package archive_test

import (
	"archive/zip"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/internal/archive"
)

// archivesDir holds Django's test archives. Django's lzma and xz archives are
// in unsupported: Go has no reader for them, as Django skips them without the
// lzma module.
const archivesDir = "testdata/archives"

// assertFile fails when rel is not a regular file in dir.
func assertFile(t *testing.T, dir, rel string) {
	t.Helper()
	info, err := os.Stat(filepath.Join(dir, filepath.FromSlash(rel)))
	if err != nil {
		t.Errorf("%s: %v", rel, err)
		return
	}
	if !info.Mode().IsRegular() {
		t.Errorf("%s is not a regular file", rel)
	}
}

// assertErrorContains fails unless err contains msg.
func assertErrorContains(t *testing.T, err error, msg string) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), msg) {
		t.Errorf("error %v does not contain %q", err, msg)
	}
}

// Ported from Django: tests/utils_tests/test_archive.py::TestArchive.test_extract_function
func TestExtractFunction(t *testing.T) {
	entries, err := os.ReadDir(archivesDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		t.Run(entry.Name(), func(t *testing.T) {
			dir := t.TempDir()
			if err := archive.Extract(filepath.Join(archivesDir, entry.Name()), dir); err != nil {
				t.Fatal(err)
			}
			for _, rel := range []string{"1", "2", "foo/1", "foo/2", "foo/bar/1", "foo/bar/2"} {
				assertFile(t, dir, rel)
			}
		})
	}
}

// Ported from Django: tests/utils_tests/test_archive.py::TestArchiveInvalid.test_extract_function_traversal
//
// Django tests the Windows drive names on Windows only; the port runs the
// cases of the other platforms.
func TestExtractFunctionTraversal(t *testing.T) {
	tests := []struct{ entry, invalidPath string }{
		{"traversal.tar", ".."},
		{"traversal_absolute.tar", "/tmp/evil.py"},
	}
	for _, tt := range tests {
		t.Run(tt.entry, func(t *testing.T) {
			err := archive.Extract(filepath.Join("testdata/traversal_archives", tt.entry), t.TempDir())
			var invalid *archive.InvalidPathError
			if !errors.As(err, &invalid) {
				t.Fatalf("Extract: %v, want an InvalidPathError", err)
			}
			assertErrorContains(t, err, "Archive contains invalid path: '"+tt.invalidPath+"'")
		})
	}
}

// Ported from Django: tests/utils_tests/test_archive.py::TestArchiveInvalid.test_extract_function_traversal_startswith
func TestExtractFunctionTraversalStartswith(t *testing.T) {
	base, err := filepath.Abs(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	zipPath := filepath.Join(t.TempDir(), "archive.zip")
	f, err := os.Create(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for name, content := range map[string]string{
		filepath.Join(base+"abc", "evil.txt"): "evil\n",
		"test.txt":                            "data\n",
	} {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	assertErrorContains(t, archive.Extract(zipPath, base), "Archive contains invalid path")
}

// TestExtractUnrecognizedFormat refuses a path without an archive extension,
// before it opens the file.
//
// No Django test exists.
func TestExtractUnrecognizedFormat(t *testing.T) {
	for _, name := range []string{"template", "template.gz", "template.tar.gz.bak"} {
		err := archive.Extract(name, t.TempDir())
		var unrecognized *archive.UnrecognizedFormatError
		if !errors.As(err, &unrecognized) {
			t.Errorf("Extract(%q): %v, want an UnrecognizedFormatError", name, err)
		}
		assertErrorContains(t, err, "Path not a recognized archive format: "+name)
	}
}

// TestExtractUnsupportedCompression reports the lzma and xz compression of
// Django's test archives, which Go cannot read.
//
// No Django test exists.
func TestExtractUnsupportedCompression(t *testing.T) {
	for name, msg := range map[string]string{
		"foobar.tar.lzma": "lzma compression is not supported",
		"foobar.tar.xz":   "xz compression is not supported",
	} {
		assertErrorContains(t, archive.Extract(filepath.Join("testdata/unsupported", name), t.TempDir()), msg)
	}
}
