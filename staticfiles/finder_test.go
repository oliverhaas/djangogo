package staticfiles_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/staticfiles"
)

// paths returns the Path of every match.
func paths(matches []staticfiles.Match) []string {
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		out = append(out, m.Path)
	}
	return out
}

// listed returns the paths that List reports for the files under the given
// ignore patterns.
func listed(t *testing.T, files map[string]string, patterns ...string) []string {
	t.Helper()
	finder := staticfiles.NewFinder(staticfiles.Source{Name: "app", FS: mapFS(files)})
	matches, err := finder.List(patterns...)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	return paths(matches)
}

// TestListOrder lists the sources in their order and the files of a source in
// the order of fs.WalkDir, with the prefix of a source in front of its paths.
// A path that two sources hold is listed once, at the first source.
func TestListOrder(t *testing.T) {
	prefixed := staticfiles.Source{Name: "downloads", Prefix: "dl", FS: mapFS(map[string]string{"a.zip": ""})}
	finder := staticfiles.NewFinder(
		staticfiles.Source{Name: "second", FS: mapFS(map[string]string{"z.css": "", "js/b.js": "", "js/a.js": "", "a.css": ""})},
		prefixed,
		staticfiles.Source{Name: "last", FS: mapFS(map[string]string{"a.css": "", "0.css": "", "dl/a.zip": ""})},
	)

	matches, err := finder.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	djangotest.AssertSequenceEqual(t, paths(matches), []string{"a.css", "js/a.js", "js/b.js", "z.css", "dl/a.zip", "0.css"})
	djangotest.AssertEqual(t, matches[0].Location, "second:a.css")
	djangotest.AssertEqual(t, matches[4].Location, "downloads:a.zip")
}

// TestListIgnorePatternSyntax matches ignore patterns as Python's
// fnmatch.fnmatchcase does, which is what Django's ignore_patterns go through.
// The expectations are the results of that function.
func TestListIgnorePatternSyntax(t *testing.T) {
	for _, tc := range []struct {
		pattern, name string
		ignored       bool
	}{
		{"CVS", "CVS", true},
		{"CVS", "cvs", false},
		{".*", ".hidden", true},
		{".*", "a.txt", false},
		{"*~", "backup~", true},
		{"*.TXT", "a.txt", false},
		{"*", "abc", true},
		{"?", "a", true},
		{"?", "ab", false},
		{"a?c", "abc", true},
		{"t*t", "tXt", true},
		{"t*t", "test", true},
		{"*a*b*c*", "xaybzc", true},
		{"a**b", "aXXb", true},
		{"a**b", "ab", true},
		{"é*", "éa", true},
		{"[abc]", "b", true},
		{"[abc]", "d", false},
		{"[a-c]x", "bx", true},
		{"[a-c]x", "dx", false},
		{"[!a-c]x", "dx", true},
		{"[!a-c]x", "bx", false},
		// A set is negated with "!" only: "^" is a member like any other.
		{"[^a]", "^", true},
		{"[^a]", "b", false},
		// A "]" right after the opening bracket is a member.
		{"[]a]", "]", true},
		{"[!]a]", "]", false},
		{"[!]a]", "b", true},
		// A hyphen at either end of a set is a member.
		{"[a-]", "-", true},
		{"[-a]", "-", true},
		{"[a-c-e]", "-", true},
		{"[a-c-e]", "d", false},
		// A range that runs backwards is empty.
		{"[c-a]x", "bx", false},
		// A "[" without its "]" matches itself.
		{"[", "[", true},
		{"a[", "a[", true},
		{"[!a", "[!a", true},
		{"x[]y", "x[]y", true},
		// A backslash does not quote.
		{`a\*`, `a\*`, true},
		{`a\*`, "a*", false},
		{"[*]", "*", true},
		{"[*]", "a", false},
		{"[é]", "é", true},
	} {
		t.Run(tc.pattern+" on "+tc.name, func(t *testing.T) {
			got := listed(t, map[string]string{tc.name: ""}, tc.pattern)

			djangotest.AssertEqual(t, len(got) == 0, tc.ignored)
		})
	}
}

// TestListIgnorePatternScope tries a pattern on the base name of every file
// and directory and on the path of a file inside its source.
func TestListIgnorePatternScope(t *testing.T) {
	files := map[string]string{
		"site.css":            "",
		"vendor/lib.js":       "",
		"vendor/lib.css":      "",
		"js/vendor/module.js": "",
		"js/app.js":           "",
	}
	for _, tc := range []struct {
		name     string
		patterns []string
		want     []string
	}{
		{"a file name anywhere", []string{"*.css"}, []string{"js/app.js", "js/vendor/module.js", "vendor/lib.js"}},
		{"a directory with all it holds", []string{"vendor"}, []string{"js/app.js", "site.css"}},
		{"a path, the star crossing slashes", []string{"*/vendor/*.js"}, []string{"js/app.js", "site.css", "vendor/lib.css", "vendor/lib.js"}},
		{"a whole path", []string{"vendor/lib.js"}, []string{"js/app.js", "js/vendor/module.js", "site.css", "vendor/lib.css"}},
		{"several patterns", []string{"*.css", "js"}, []string{"vendor/lib.js"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			djangotest.AssertSequenceEqual(t, listed(t, files, tc.patterns...), tc.want)
		})
	}
}

// TestListIgnorePatternNotTriedOnPrefix keeps the files of a source whose
// prefix matches a pattern: the prefix is not a name inside the source.
func TestListIgnorePatternNotTriedOnPrefix(t *testing.T) {
	source := staticfiles.Source{Name: "app", Prefix: "vendor", FS: mapFS(map[string]string{"lib.js": ""})}

	matches, err := staticfiles.NewFinder(source).List("vendor", "vendor/*")
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	djangotest.AssertSequenceEqual(t, paths(matches), []string{"vendor/lib.js"})
}

// TestFindRejectsInvalidPaths finds nothing for a path that is not a plain
// relative path, whatever exists outside the source.
func TestFindRejectsInvalidPaths(t *testing.T) {
	s := newSite(t)
	finder := staticfiles.NewFinder(staticfiles.DirSource(s.path("documents")))
	for _, path := range []string{
		"../prefixed/test.txt",
		"subdir/../../prefixed/test.txt",
		s.path("prefixed", "test.txt"),
		"/test.txt",
		"subdir//test.txt",
		"",
	} {
		t.Run(path, func(t *testing.T) {
			_, ok := finder.Find(path)

			djangotest.AssertEqual(t, ok, false)
			djangotest.AssertEqual(t, len(finder.FindAll(path)), 0)
		})
	}
}

// TestNewFinderSearchesADirectoryOnce drops a directory that is listed a
// second time with the same prefix, as Django's FileSystemFinder does, and
// keeps it under another prefix.
func TestNewFinderSearchesADirectoryOnce(t *testing.T) {
	dir := t.TempDir()
	prefixed := staticfiles.DirSource(dir)
	prefixed.Prefix = "prefix"

	finder := staticfiles.NewFinder(staticfiles.DirSource(dir), prefixed, staticfiles.DirSource(dir))

	djangotest.AssertSequenceEqual(t, roots(finder.Sources()), []string{dir, dir})
	djangotest.AssertEqual(t, finder.Sources()[1].Prefix, "prefix")
}

// TestFinderIsAFilesystem checks the merged view with the test suite of the
// standard library, and that a file in it is the file of the first source.
func TestFinderIsAFilesystem(t *testing.T) {
	finder := newSite(t).finder()

	err := fstest.TestFS(finder,
		"test.txt",
		"subdir/test.txt",
		"prefix/test.txt",
		"pathlib.txt",
		"test/file.txt",
		"test/file1.txt",
		"test/vendor/module.js",
		"test/.hidden",
		"file2.txt",
		"media-file.txt",
	)
	if err != nil {
		t.Fatal(err)
	}

	data, err := fs.ReadFile(finder, "test/file.txt")
	if err != nil {
		t.Fatalf("read test/file.txt: %v", err)
	}
	djangotest.AssertEqual(t, string(data), projectFiles["documents/test/file.txt"])
}

// TestFinderFilesystemMergesPrefixes shows the prefix of a source as a
// directory of the merged view, also where the prefix has several elements
// and where another source has files below the same directory.
func TestFinderFilesystemMergesPrefixes(t *testing.T) {
	deep := staticfiles.Source{Name: "deep", Prefix: "vendor/lib", FS: mapFS(map[string]string{"lib.js": "lib"})}
	finder := staticfiles.NewFinder(
		deep,
		staticfiles.Source{Name: "app", FS: mapFS(map[string]string{"vendor/other.js": "other", "site.css": ""})},
		// A directory that does not exist adds no prefix directory.
		staticfiles.Source{Name: "missing", Prefix: "ghost", FS: os.DirFS(filepath.Join(t.TempDir(), "missing"))},
	)

	if err := fstest.TestFS(finder, "vendor/lib/lib.js", "vendor/other.js", "site.css"); err != nil {
		t.Fatal(err)
	}

	entries, err := fs.ReadDir(finder, ".")
	if err != nil {
		t.Fatalf("read the root: %v", err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	djangotest.AssertSequenceEqual(t, names, []string{"site.css", "vendor"})
}

// TestFinderFilesystemDirectoryHidesLaterFile opens what the first source has
// at a name: its directory, although a later source has a file of that name.
// A directory listing names the entry of the first source, and the two must
// agree.
func TestFinderFilesystemDirectoryHidesLaterFile(t *testing.T) {
	finder := staticfiles.NewFinder(
		staticfiles.Source{Name: "first", FS: mapFS(map[string]string{"logo/dark.svg": "<svg/>"})},
		staticfiles.Source{Name: "second", FS: mapFS(map[string]string{"logo": "a file", "site.css": ""})},
	)

	if err := fstest.TestFS(finder, "logo/dark.svg", "site.css"); err != nil {
		t.Fatal(err)
	}
}

// TestFinderFollowsSymlinks treats a symbolic link in a source directory as
// what it points to, in a listing and in the merged view, and leaves out a
// link to nothing. Django's listing follows links the same way.
func TestFinderFollowsSymlinks(t *testing.T) {
	dir := t.TempDir()
	shared := filepath.Join(t.TempDir(), "shared")
	writeTree(t, dir, map[string]string{"site.css": "body {}"})
	writeTree(t, shared, map[string]string{"font.txt": "a font"})
	if err := os.Symlink(shared, filepath.Join(dir, "linked")); err != nil {
		t.Skipf("symbolic links are not supported: %v", err)
	}
	if err := os.Symlink(filepath.Join(dir, "site.css"), filepath.Join(dir, "alias.css")); err != nil {
		t.Fatalf("link a file: %v", err)
	}
	if err := os.Symlink(filepath.Join(dir, "nothing"), filepath.Join(dir, "broken.css")); err != nil {
		t.Fatalf("link to nothing: %v", err)
	}
	finder := staticfiles.NewFinder(staticfiles.DirSource(dir))

	matches, err := finder.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	djangotest.AssertSequenceEqual(t, paths(matches), []string{"alias.css", "linked/font.txt", "site.css"})
	if err := fstest.TestFS(finder, "alias.css", "linked/font.txt", "site.css"); err != nil {
		t.Fatal(err)
	}
}
