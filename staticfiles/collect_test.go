package staticfiles_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/staticfiles"
)

// conflictWarning is the line that collectstatic prints for css/site.css, the
// file that two sources of conflictSources hold.
const conflictWarning = "Found another file with the destination path 'css/site.css'. It will be ignored since " +
	"only the first encountered file is collected. If this is not what you want, make sure every static file " +
	"has a unique path.\n"

// conflictSources writes three source directories below a new directory and
// returns it with a finder over them: css/site.css is in the first two
// directories, and the third is collected under the prefix "downloads".
func conflictSources(t *testing.T) (string, *staticfiles.Finder) {
	t.Helper()
	base := t.TempDir()
	writeTree(t, base, map[string]string{
		"first/css/site.css":  "the first style sheet",
		"second/css/site.css": "the second style sheet",
		"second/app.js":       "the script",
		"pref/css/p.css":      "the download",
	})
	prefixed := staticfiles.DirSource(filepath.Join(base, "pref"))
	prefixed.Prefix = "downloads"
	return base, staticfiles.NewFinder(
		staticfiles.DirSource(filepath.Join(base, "first")),
		staticfiles.DirSource(filepath.Join(base, "second")),
		prefixed,
	)
}

// TestCollectOutput pins what a command prints, line for line as Django 6.0
// prints it for the same files. The runs build on each other. One thing
// differs from Django: it deletes in the order in which the operating system
// lists a directory, Collect in sorted order.
func TestCollectOutput(t *testing.T) {
	base, finder := conflictSources(t)
	root := filepath.Join(base, "root")
	first := filepath.Join(base, "first", "css", "site.css")
	script := filepath.Join(base, "second", "app.js")
	download := filepath.Join(base, "pref", "css", "p.css")
	// The prefixed file is named without its prefix: Django logs the path
	// inside the source here.
	skipping := "Skipping 'css/site.css' (not modified)\nSkipping 'app.js' (not modified)\n" +
		conflictWarning + "Skipping 'css/p.css' (not modified)\n"
	copying := "Copying '" + first + "'\nCopying '" + script + "'\n" + conflictWarning + "Copying '" + download + "'\n"
	for _, tc := range []struct {
		name string
		opts staticfiles.CollectOptions
		want string
	}{
		{
			"into an empty root",
			staticfiles.CollectOptions{Verbosity: 2},
			copying + "\n3 static files copied to '" + root + "', 1 skipped due to conflict.\n",
		},
		{
			"nothing changed",
			staticfiles.CollectOptions{Verbosity: 2},
			skipping + "\n0 static files copied to '" + root + "', 3 unmodified, 1 skipped due to conflict.\n",
		},
		{
			"nothing changed, verbosity 1",
			staticfiles.CollectOptions{Verbosity: 1},
			"\n0 static files copied to '" + root + "', 3 unmodified, 1 skipped due to conflict.\n",
		},
		{
			"dry run of a clear",
			staticfiles.CollectOptions{Verbosity: 2, Clear: true, DryRun: true},
			"Pretending to delete 'app.js'\nPretending to delete 'css/site.css'\n" +
				"Pretending to delete 'downloads/css/p.css'\n" + skipping +
				"\n3 static files deleted, 0 static files copied to '" + root +
				"', 3 unmodified, 1 skipped due to conflict.\n",
		},
		{
			"clear",
			staticfiles.CollectOptions{Verbosity: 2, Clear: true},
			"Deleting 'app.js'\nDeleting 'css/site.css'\nDeleting 'downloads/css/p.css'\n" + copying +
				"\n3 static files deleted, 3 static files copied to '" + root + "', 1 skipped due to conflict.\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.opts.Root = root

			out := callCollectstatic(t, finder, tc.opts)

			djangotest.AssertEqual(t, out, tc.want)
		})
	}
}

// TestCollectDryRunPrintsWhatItWouldCopy names every file at verbosity 1 in a
// dry run, where a real run prints only the summary, and creates nothing.
func TestCollectDryRunPrintsWhatItWouldCopy(t *testing.T) {
	base, finder := conflictSources(t)
	root := filepath.Join(base, "root")

	out := callCollectstatic(t, finder, staticfiles.CollectOptions{Root: root, DryRun: true, Verbosity: 1})

	djangotest.AssertEqual(t, out, "Pretending to copy '"+filepath.Join(base, "first", "css", "site.css")+
		"'\nPretending to copy '"+filepath.Join(base, "second", "app.js")+
		"'\nPretending to copy '"+filepath.Join(base, "pref", "css", "p.css")+
		"'\n\n3 static files copied to '"+root+"', 1 skipped due to conflict.\n")
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Errorf("the dry run created the static root (Stat error: %v)", err)
	}
}

// TestCollectResultListsPaths reports the paths of a run relative to the
// static root, the prefix of a source included, and writes nothing without a
// writer.
func TestCollectResultListsPaths(t *testing.T) {
	base, finder := conflictSources(t)
	root := filepath.Join(base, "root")
	writeTree(t, root, map[string]string{"stale/old.css": "", "old.js": ""})
	opts := staticfiles.CollectOptions{Root: root, Verbosity: 2}

	opts.Clear = true
	cleared, err := staticfiles.Collect(finder, opts)
	if err != nil {
		t.Fatalf("Collect with Clear: %v", err)
	}
	opts.Clear = false
	again, err := staticfiles.Collect(finder, opts)
	if err != nil {
		t.Fatalf("Collect again: %v", err)
	}

	collected := []string{"css/site.css", "app.js", "downloads/css/p.css"}
	djangotest.AssertSequenceEqual(t, cleared.Deleted, []string{"old.js", "stale/old.css"})
	djangotest.AssertSequenceEqual(t, cleared.Copied, collected)
	djangotest.AssertSequenceEqual(t, cleared.Skipped, []string{"css/site.css"})
	djangotest.AssertEqual(t, len(cleared.Unmodified), 0)
	djangotest.AssertSequenceEqual(t, again.Unmodified, collected)
	djangotest.AssertSequenceEqual(t, again.Skipped, []string{"css/site.css"})
	djangotest.AssertEqual(t, len(again.Copied)+len(again.Deleted), 0)
	assertFileContains(t, staticRoot(root), "css/site.css", "the first style sheet")
	assertFileContains(t, staticRoot(root), "downloads/css/p.css", "the download")
}

// TestCollectAlwaysCopiesFromSourceWithoutModificationTimes copies the file of
// an in-memory source on every run. Such a source, an embed.FS for one,
// reports no modification time, so a collected copy can never be told to be
// current: skipping it would keep the file of an older build forever.
func TestCollectAlwaysCopiesFromSourceWithoutModificationTimes(t *testing.T) {
	root := t.TempDir()
	older := staticfiles.NewFinder(staticfiles.Source{Name: "app", FS: mapFS(map[string]string{"site.css": "old"})})
	newer := staticfiles.NewFinder(staticfiles.Source{Name: "app", FS: mapFS(map[string]string{"site.css": "new"})})
	callCollectstatic(t, older, staticfiles.CollectOptions{Root: root})

	out := callCollectstatic(t, newer, staticfiles.CollectOptions{Root: root, Verbosity: 2})

	djangotest.AssertEqual(t, out, "Deleting 'site.css'\nCopying 'app:site.css'\n\n1 static file copied to '"+root+"'.\n")
	assertFileContains(t, staticRoot(root), "site.css", "new")
}

// TestCollectReplacesFileOlderThanItsSource copies a file again when the
// collected copy is older than the source.
func TestCollectReplacesFileOlderThanItsSource(t *testing.T) {
	base, finder := conflictSources(t)
	root := filepath.Join(base, "root")
	writeTree(t, root, map[string]string{"app.js": "an old version"})
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(filepath.Join(root, "app.js"), old, old); err != nil {
		t.Fatalf("set modification time: %v", err)
	}

	out := callCollectstatic(t, finder, staticfiles.CollectOptions{Root: root, Verbosity: 2})

	assertIn(t, out, "Deleting 'app.js'\nCopying '"+filepath.Join(base, "second", "app.js")+"'\n")
	assertFileContains(t, staticRoot(root), "app.js", "the script")
}

// TestCollectIgnorePatternWithTrailingSlash normalizes a pattern as
// collectstatic does, so that "vendor/" names the directory.
func TestCollectIgnorePatternWithTrailingSlash(t *testing.T) {
	finder := staticfiles.NewFinder(staticfiles.Source{
		Name: "app",
		FS:   mapFS(map[string]string{"vendor/lib.js": "", "site.css": ""}),
	})

	result, err := staticfiles.Collect(finder, staticfiles.CollectOptions{
		Root:           t.TempDir(),
		IgnorePatterns: []string{"vendor/"},
	})
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}

	djangotest.AssertSequenceEqual(t, result.Copied, []string{"site.css"})
}

// TestCollectRelativeRoot collects into a root that is given relative to the
// working directory, and names it by its absolute path in the summary.
func TestCollectRelativeRoot(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	finder := staticfiles.NewFinder(staticfiles.Source{Name: "app", FS: mapFS(map[string]string{"site.css": "body {}"})})

	out := callCollectstatic(t, finder, staticfiles.CollectOptions{Root: "static", Verbosity: 1})

	assertFileContains(t, staticRoot(filepath.Join(dir, "static")), "site.css", "body {}")
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("working directory: %v", err)
	}
	djangotest.AssertEqual(t, out, "\n1 static file copied to '"+filepath.Join(wd, "static")+"'.\n")
}

// TestCollectRefusesDestinationOutsideTheRoot fails for a source whose prefix
// leads out of the static root, and writes nothing there.
func TestCollectRefusesDestinationOutsideTheRoot(t *testing.T) {
	base := t.TempDir()
	for _, prefix := range []string{"../outside", "css/../../outside"} {
		t.Run(prefix, func(t *testing.T) {
			source := staticfiles.Source{Name: "app", Prefix: prefix, FS: mapFS(map[string]string{"site.css": "body {}"})}

			_, err := staticfiles.Collect(staticfiles.NewFinder(source), staticfiles.CollectOptions{
				Root: filepath.Join(base, "root"),
			})

			if err == nil {
				t.Error("Collect returned no error")
			}
			if _, statErr := os.Stat(filepath.Join(base, "outside")); !os.IsNotExist(statErr) {
				t.Errorf("Collect wrote outside the static root (Stat error: %v)", statErr)
			}
		})
	}
}

// TestCollectClearDoesNotFollowSymlinks removes a symbolic link in the static
// root under Clear and leaves the directory behind it alone: a link must not
// let a clear delete files outside the root.
func TestCollectClearDoesNotFollowSymlinks(t *testing.T) {
	_, finder := conflictSources(t)
	root := t.TempDir()
	outside := t.TempDir()
	writeTree(t, outside, map[string]string{"keep.txt": "not a static file"})
	link := filepath.Join(root, "linked")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symbolic links are not supported: %v", err)
	}

	callCollectstatic(t, finder, staticfiles.CollectOptions{Root: root, Clear: true})

	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Errorf("the symbolic link is still there (Lstat error: %v)", err)
	}
	assertFileContains(t, staticRoot(outside), "keep.txt", "not a static file")
}

// TestCollectResultSummary words the closing line as Django does: the singular
// for one file, and the parts for deleted, unmodified and conflicting files
// only when there are any.
func TestCollectResultSummary(t *testing.T) {
	root := t.TempDir()
	for _, tc := range []struct {
		name   string
		result staticfiles.CollectResult
		root   string
		want   string
	}{
		{"nothing", staticfiles.CollectResult{}, root, "0 static files copied to '" + root + "'."},
		{"one file", staticfiles.CollectResult{Copied: []string{"a"}}, root, "1 static file copied to '" + root + "'."},
		{
			"one deleted",
			staticfiles.CollectResult{Deleted: []string{"a"}, Copied: []string{"a", "b"}},
			root,
			"1 static file deleted, 2 static files copied to '" + root + "'.",
		},
		{
			"every part",
			staticfiles.CollectResult{
				Deleted:    []string{"a", "b"},
				Copied:     []string{"a"},
				Unmodified: []string{"b", "c", "d"},
				Skipped:    []string{"a"},
			},
			root,
			"2 static files deleted, 1 static file copied to '" + root + "', 3 unmodified, 1 skipped due to conflict.",
		},
		{"no root", staticfiles.CollectResult{Copied: []string{"a", "b"}}, "", "2 static files copied."},
	} {
		t.Run(tc.name, func(t *testing.T) {
			djangotest.AssertEqual(t, tc.result.Summary(tc.root), tc.want)
		})
	}
}
