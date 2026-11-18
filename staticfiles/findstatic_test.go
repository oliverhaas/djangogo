package staticfiles_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/staticfiles"
)

// TestFindStaticOutputText pins the text for every combination of --first and
// the verbosity. The expectations are what Django 6.0 prints for the same
// files, with the location of an in-memory source in place of an absolute
// path.
func TestFindStaticOutputText(t *testing.T) {
	finder := staticfiles.NewFinder(
		staticfiles.Source{Name: "first", FS: mapFS(map[string]string{"css/site.css": ""})},
		staticfiles.Source{Name: "second", FS: mapFS(map[string]string{"css/site.css": "", "app.js": ""})},
		staticfiles.Source{Name: "third", Prefix: "downloads", FS: mapFS(map[string]string{"a.zip": ""})},
	)
	const everywhere = "\nLooking in the following locations:\n  first\n  second\n  third\n"
	for _, tc := range []struct {
		name      string
		path      string
		all       bool
		verbosity int
		want      string
		found     bool
	}{
		{"all, verbosity 0", "css/site.css", true, 0, "first:css/site.css\nsecond:css/site.css\n", true},
		{
			"all, verbosity 1", "css/site.css", true, 1,
			"Found 'css/site.css' here:\n  first:css/site.css\n  second:css/site.css\n", true,
		},
		{
			"all, verbosity 2", "css/site.css", true, 2,
			"Found 'css/site.css' here:\n  first:css/site.css\n  second:css/site.css" + everywhere, true,
		},
		{"first, verbosity 0", "css/site.css", false, 0, "first:css/site.css\n", true},
		{"first, verbosity 1", "css/site.css", false, 1, "Found 'css/site.css' here:\n  first:css/site.css\n", true},
		{
			// The search ends at the first match, and so does the list.
			"first, verbosity 2", "css/site.css", false, 2,
			"Found 'css/site.css' here:\n  first:css/site.css\nLooking in the following locations:\n  first\n", true,
		},
		{"first, in a later source", "app.js", false, 0, "second:app.js\n", true},
		{"below a prefix", "downloads/a.zip", true, 0, "third:a.zip\n", true},
		{"a directory", "css", false, 0, "first:css\n", true},
		{"missing, verbosity 0", "missing.css", true, 0, "", false},
		{"missing, verbosity 1", "missing.css", true, 1, "No matching file found for 'missing.css'.\n", false},
		{"missing, verbosity 2", "missing.css", true, 2, "No matching file found for 'missing.css'.\n" + everywhere, false},
		{"missing, first, verbosity 2", "missing.css", false, 2, "No matching file found for 'missing.css'.\n" + everywhere, false},
		{"not a relative path", "../css/site.css", true, 1, "No matching file found for '../css/site.css'.\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, found := staticfiles.FindStaticOutput(finder, tc.path, tc.all, tc.verbosity)

			djangotest.AssertEqual(t, out, tc.want)
			djangotest.AssertEqual(t, found, tc.found)
		})
	}
}

// TestFindStaticOutputListsADirectoryOnce names a directory once among the
// searched locations although two sources search it, each under its own
// prefix, as Django does for such STATICFILES_DIRS entries.
func TestFindStaticOutputListsADirectoryOnce(t *testing.T) {
	dir := t.TempDir()
	prefixed := staticfiles.DirSource(dir)
	prefixed.Prefix = "vendor"
	finder := staticfiles.NewFinder(staticfiles.DirSource(dir), prefixed)

	out, found := staticfiles.FindStaticOutput(finder, "missing.css", true, 2)

	djangotest.AssertEqual(t, found, false)
	djangotest.AssertEqual(t, out,
		"No matching file found for 'missing.css'.\n\nLooking in the following locations:\n  "+dir+"\n")
}

// TestFindStaticOutputResolvesSymlinks prints where a file really is, as
// findstatic does with os.path.realpath, while the searched location stays
// the directory as it was configured.
func TestFindStaticOutputResolvesSymlinks(t *testing.T) {
	base := realPath(t, t.TempDir())
	writeTree(t, base, map[string]string{"real/site.css": "body {}"})
	link := filepath.Join(base, "link")
	if err := os.Symlink(filepath.Join(base, "real"), link); err != nil {
		t.Skipf("symbolic links are not supported: %v", err)
	}
	finder := staticfiles.NewFinder(staticfiles.DirSource(link))

	out, found := staticfiles.FindStaticOutput(finder, "site.css", true, 2)

	djangotest.AssertEqual(t, found, true)
	djangotest.AssertEqual(t, out, "Found 'site.css' here:\n  "+filepath.Join(base, "real", "site.css")+
		"\nLooking in the following locations:\n  "+link+"\n")
}
