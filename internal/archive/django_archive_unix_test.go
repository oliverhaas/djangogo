//go:build unix

package archive_test

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/oliverhaas/djangogo/internal/archive"
)

// Ported from Django: tests/utils_tests/test_archive.py::TestArchive.test_extract_file_permissions
func TestExtractFilePermissions(t *testing.T) {
	umask := syscall.Umask(0)
	syscall.Umask(umask)
	entries, err := os.ReadDir(archivesDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "leadpath_") {
			continue
		}
		t.Run(entry.Name(), func(t *testing.T) {
			dir := t.TempDir()
			if err := archive.Extract(filepath.Join(archivesDir, entry.Name()), dir); err != nil {
				t.Fatal(err)
			}
			// An executable file in the archive has executable permissions.
			assertMode(t, filepath.Join(dir, "executable"), 0o775)
			// A file is readable even if permission data is missing.
			assertMode(t, filepath.Join(dir, "no_permissions"), 0o666&^os.FileMode(umask))
		})
	}
}

// assertMode fails unless the permission bits of path are want.
func assertMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Errorf("%s: mode %o, want %o", filepath.Base(path), got, want)
	}
}
