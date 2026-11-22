//go:build unix

package staticfiles_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/staticfiles"
)

// The tests in this file are ports of Django's TestCollectionLinks, which
// collectstatic --link. Django skips them where symbolic links are not
// supported; they run on Unix here.

// linkCollection is TestCollectionLinks.setUp: it collects the files of
// TEST_SETTINGS with Link into a new STATIC_ROOT and returns the site and the
// root.
func linkCollection(t *testing.T) (s *site, root string) {
	t.Helper()
	s, root = newSite(t), t.TempDir()
	runCollectstatic(t, s.finder(), staticfiles.CollectOptions{Root: root, Link: true})
	return s, root
}

// isSymlink reports whether path is a symbolic link.
func isSymlink(t *testing.T, path string) bool {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info.Mode()&fs.ModeSymlink != 0
}

// Ported from Django: tests/staticfiles_tests/test_management.py::TestCollectionLinks.test_staticfiles_dirs
func TestCollectionLinksStaticfilesDirs(t *testing.T) {
	checkStaticfilesDirs(t, collection(t, staticfiles.CollectOptions{Link: true}))
}

// Ported from Django: tests/staticfiles_tests/test_management.py::TestCollectionLinks.test_staticfiles_dirs_subdir
func TestCollectionLinksStaticfilesDirsSubdir(t *testing.T) {
	checkStaticfilesDirsSubdir(t, collection(t, staticfiles.CollectOptions{Link: true}))
}

// Ported from Django: tests/staticfiles_tests/test_management.py::TestCollectionLinks.test_staticfiles_dirs_priority
func TestCollectionLinksStaticfilesDirsPriority(t *testing.T) {
	checkStaticfilesDirsPriority(t, collection(t, staticfiles.CollectOptions{Link: true}))
}

// The app sources of the test site are embedded, so their files are copied.
//
// Ported from Django: tests/staticfiles_tests/test_management.py::TestCollectionLinks.test_app_files
func TestCollectionLinksAppFiles(t *testing.T) {
	checkAppFiles(t, collection(t, staticfiles.CollectOptions{Link: true}))
}

// Ported from Django: tests/staticfiles_tests/test_management.py::TestCollectionLinks.test_nonascii_filenames
func TestCollectionLinksNonasciiFilenames(t *testing.T) {
	checkNonasciiFilenames(t, collection(t, staticfiles.CollectOptions{Link: true}))
}

// Ported from Django: tests/staticfiles_tests/test_management.py::TestCollectionLinks.test_camelcase_filenames
func TestCollectionLinksCamelcaseFilenames(t *testing.T) {
	checkCamelcaseFilenames(t, collection(t, staticfiles.CollectOptions{Link: true}))
}

// Ported from Django: tests/staticfiles_tests/test_management.py::TestCollectionLinks.test_filename_with_percent_sign
func TestCollectionLinksFilenameWithPercentSign(t *testing.T) {
	checkFilenameWithPercentSign(t, collection(t, staticfiles.CollectOptions{Link: true}))
}

// Ported from Django: tests/staticfiles_tests/test_management.py::TestCollectionLinks.test_links_created
func TestCollectionLinksLinksCreated(t *testing.T) {
	_, root := linkCollection(t)
	if !isSymlink(t, filepath.Join(root, "test.txt")) {
		t.Error("test.txt is not a symbolic link")
	}
}

// Ported from Django: tests/staticfiles_tests/test_management.py::TestCollectionLinks.test_broken_symlink
func TestCollectionLinksBrokenSymlink(t *testing.T) {
	s, root := linkCollection(t)
	path := filepath.Join(root, "test.txt")
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	runCollectstatic(t, s.finder(), staticfiles.CollectOptions{Root: root, Link: true})
	if !isSymlink(t, path) {
		t.Error("test.txt is not a symbolic link")
	}
}

// Ported from Django: tests/staticfiles_tests/test_management.py::TestCollectionLinks.test_symlinks_and_files_replaced
func TestCollectionLinksSymlinksAndFilesReplaced(t *testing.T) {
	s, root := linkCollection(t)
	path := filepath.Join(root, "test.txt")
	if !isSymlink(t, path) {
		t.Fatal("test.txt is not a symbolic link")
	}
	runCollectstatic(t, s.finder(), staticfiles.CollectOptions{Root: root})
	if isSymlink(t, path) {
		t.Error("a run without Link kept the symbolic link")
	}
	// The copy replaced the link and did not write through it.
	assertFileContains(t, staticRoot(s.path("documents")), "test.txt", "Can we find")
	runCollectstatic(t, s.finder(), staticfiles.CollectOptions{Root: root, Link: true})
	if !isSymlink(t, path) {
		t.Error("a run with Link kept the copy")
	}
}

// Ported from Django: tests/staticfiles_tests/test_management.py::TestCollectionLinks.test_clear_broken_symlink
func TestCollectionLinksClearBrokenSymlink(t *testing.T) {
	s, root := linkCollection(t)
	nonexistent := filepath.Join(root, "nonexistent.txt")
	broken := filepath.Join(root, "symlink.txt")
	if err := os.Symlink(nonexistent, broken); err != nil {
		t.Fatal(err)
	}
	runCollectstatic(t, s.finder(), staticfiles.CollectOptions{Root: root, Link: true, Clear: true})
	if _, err := os.Lstat(broken); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the broken link survived the clear: %v", err)
	}
}

// No Go counterpart: TestCollectionLinks.test_no_remote_link. The static
// root is always a directory on disk.

// TestCollectLinkReplacesBrokenLink links a file again whose link at the
// destination points to nothing, because the modification time of a broken
// link cannot be compared.
//
// No Django test exists.
func TestCollectLinkReplacesBrokenLink(t *testing.T) {
	s, root := newSite(t), t.TempDir()
	path := filepath.Join(root, "test.txt")
	if err := os.Symlink(filepath.Join(root, "nonexistent.txt"), path); err != nil {
		t.Fatal(err)
	}
	runCollectstatic(t, s.finder(), staticfiles.CollectOptions{Root: root, Link: true})
	assertFileContains(t, staticRoot(root), "test.txt", "Can we find")
}

// TestCollectLinkOutput checks the log lines and the summary of a run with
// Link: a file on disk is linked, a file of an embedded source is copied, and
// a second run leaves the links alone.
//
// No Django test exists.
func TestCollectLinkOutput(t *testing.T) {
	src := t.TempDir()
	writeTree(t, src, map[string]string{"site.css": "body {}\n"})
	finder := staticfiles.NewFinder(
		staticfiles.DirSource(src),
		staticfiles.Source{Name: "blog", FS: mapFS(map[string]string{"blog.js": "//\n"})},
	)
	root := t.TempDir()
	cssSource := filepath.Join(src, "site.css")

	out := callCollectstatic(t, finder, staticfiles.CollectOptions{Root: root, Link: true, Verbosity: 2})
	assertIn(t, out, "Linking '"+cssSource+"'")
	assertIn(t, out, "Copying 'blog:blog.js'")
	assertIn(t, out, "\n1 static file symlinked to '"+root+"', 1 copied.\n")
	if target, err := os.Readlink(filepath.Join(root, "site.css")); err != nil || target != cssSource {
		t.Errorf("site.css links to %q (%v), want %q", target, err, cssSource)
	}

	out = callCollectstatic(t, finder, staticfiles.CollectOptions{Root: root, Link: true, Verbosity: 2})
	assertIn(t, out, "Skipping 'site.css' (not modified)")
	assertIn(t, out, "\n0 static files symlinked to '"+root+"', 1 copied, 1 unmodified.\n")

	dry := t.TempDir()
	out = callCollectstatic(t, finder, staticfiles.CollectOptions{Root: dry, Link: true, DryRun: true, Verbosity: 1})
	assertIn(t, out, "Pretending to link '"+cssSource+"'")
	if entries, _ := os.ReadDir(dry); len(entries) != 0 {
		t.Errorf("a dry run wrote %d entries", len(entries))
	}
	if !strings.HasSuffix(out, "\n1 static file symlinked to '"+dry+"', 1 copied.\n") {
		t.Errorf("dry-run output %q", out)
	}
}
