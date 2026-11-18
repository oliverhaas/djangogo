package staticfiles_test

import (
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/staticfiles"
)

// siteClient returns a client of a Handler that serves the files of
// TEST_SETTINGS under /static/, and the site.
func siteClient(t *testing.T) (*djangotest.Client, *site) {
	t.Helper()
	s := newSite(t)
	return djangotest.NewClient(t, staticfiles.Handler("/static/", s.finder())), s
}

func TestHandlerRefusesOtherMethods(t *testing.T) {
	client, _ := siteClient(t)

	resp := client.Post("/static/test.txt", nil)

	djangotest.AssertEqual(t, resp.StatusCode, http.StatusMethodNotAllowed)
	djangotest.AssertEqual(t, resp.Header.Get("Allow"), "GET, HEAD")
	assertNotIn(t, resp.Content(), "Can we find")
}

func TestHandlerHeadSendsNoBody(t *testing.T) {
	client, _ := siteClient(t)

	resp := client.Head("/static/test.txt")

	djangotest.AssertEqual(t, resp.StatusCode, http.StatusOK)
	djangotest.AssertEqual(t, resp.Content(), "")
	djangotest.AssertEqual(t, resp.Header.Get("Content-Length"), strconv.Itoa(len(projectFiles["documents/test.txt"])))
}

// TestHandlerNeverListsADirectory answers 404 for every way of naming a
// directory, with Django's message.
func TestHandlerNeverListsADirectory(t *testing.T) {
	client, _ := siteClient(t)
	for _, path := range []string{"/static/", "/static/subdir", "/static/subdir/", "/static/missing/"} {
		t.Run(path, func(t *testing.T) {
			resp := client.Get(path)

			djangotest.AssertEqual(t, resp.StatusCode, http.StatusNotFound)
			assertIn(t, resp.Content(), "Directory indexes are not allowed here.")
		})
	}
}

// TestHandlerRefusesPathOutsideTheRoot keeps a request from climbing out of
// the directory of a source. Django answers 400 for such a path.
func TestHandlerRefusesPathOutsideTheRoot(t *testing.T) {
	client, s := siteClient(t)
	writeTree(t, s.project, map[string]string{"secret.txt": "the secret"})
	for _, path := range []string{"/static/../secret.txt", "/static/subdir/../../secret.txt"} {
		t.Run(path, func(t *testing.T) {
			resp := client.Get(path)

			djangotest.AssertEqual(t, resp.StatusCode, http.StatusBadRequest)
			assertNotIn(t, resp.Content(), "the secret")
		})
	}
}

// TestHandlerDirectoryHidesLaterFile lets the first source that holds a path
// decide, as Django's finders do: a directory there is a 404 although a later
// source has a file of that name.
func TestHandlerDirectoryHidesLaterFile(t *testing.T) {
	finder := staticfiles.NewFinder(
		staticfiles.Source{Name: "first", FS: mapFS(map[string]string{"logo/dark.svg": "<svg/>"})},
		staticfiles.Source{Name: "second", FS: mapFS(map[string]string{"logo": "a file"})},
	)
	client := djangotest.NewClient(t, staticfiles.Handler("/static/", finder))

	resp := client.Get("/static/logo")

	djangotest.AssertEqual(t, resp.StatusCode, http.StatusNotFound)
}

func TestHandlerSetsLastModifiedFromTheFile(t *testing.T) {
	client, s := siteClient(t)
	info, err := os.Stat(s.path("documents", "test.txt"))
	if err != nil {
		t.Fatalf("stat fixture: %v", err)
	}

	resp := client.Get("/static/test.txt")

	djangotest.AssertEqual(t, resp.Header.Get("Last-Modified"), info.ModTime().UTC().Format(http.TimeFormat))
}

// TestHandlerSendsFileWithoutModificationTimeInFull serves a file of a source
// that keeps no modification times, as an embed.FS does: it has no
// Last-Modified header and a conditional request gets the whole file.
func TestHandlerSendsFileWithoutModificationTimeInFull(t *testing.T) {
	client, _ := siteClient(t)

	resp := client.Get("/static/test/file1.txt", djangotest.Header("If-Modified-Since", "Mon, 18 Jan 2038 05:14:07 GMT"))

	djangotest.AssertEqual(t, resp.StatusCode, http.StatusOK)
	djangotest.AssertEqual(t, resp.Content(), testAppFiles["test/file1.txt"])
	djangotest.AssertEqual(t, resp.Header.Get("Last-Modified"), "")
}

// TestHandlerContentTypeFromExtension names the type by the extension alone:
// the content of both files is plain text.
func TestHandlerContentTypeFromExtension(t *testing.T) {
	finder := staticfiles.NewFinder(staticfiles.Source{Name: "app", FS: mapFS(map[string]string{
		"site.css": "body {}",
		"logo.png": "not a picture",
		"LICENSE":  "plain text",
	})})
	client := djangotest.NewClient(t, staticfiles.Handler("/static/", finder))
	for name, want := range map[string]string{
		"site.css": "text/css; charset=utf-8",
		"logo.png": "image/png",
		"LICENSE":  "application/octet-stream",
	} {
		t.Run(name, func(t *testing.T) {
			resp := client.Get("/static/" + name)

			djangotest.AssertEqual(t, resp.Header.Get("Content-Type"), want)
		})
	}
}

// plainFS is a filesystem whose files can be read but not seeked, the least
// that fs.FS asks of a file.
type plainFS struct {
	fsys fs.FS
}

// Open opens the file and hides every method but those of fs.File.
func (p plainFS) Open(name string) (fs.File, error) {
	file, err := p.fsys.Open(name)
	if err != nil {
		return nil, err
	}
	return struct{ fs.File }{file}, nil
}

func TestHandlerServesFileThatCannotSeek(t *testing.T) {
	source := staticfiles.Source{Name: "app", FS: plainFS{fsys: fstest.MapFS{
		"site.css": &fstest.MapFile{Data: []byte("body {}")},
	}}}
	client := djangotest.NewClient(t, staticfiles.Handler("/static/", staticfiles.NewFinder(source)))

	resp := client.Get("/static/site.css")

	djangotest.AssertEqual(t, resp.StatusCode, http.StatusOK)
	djangotest.AssertEqual(t, resp.Content(), "body {}")
}

// TestHandlerPrefixSpellings accepts STATIC_URL with and without the slashes
// around it, and serves the whole site for "/".
func TestHandlerPrefixSpellings(t *testing.T) {
	finder := newSite(t).finder()
	for _, tc := range []struct{ prefix, request string }{
		{"/static/", "/static/test.txt"},
		{"static/", "/static/test.txt"},
		{"/static", "/static/test.txt"},
		{"static", "/static/test.txt"},
		{"/assets/static/", "/assets/static/test.txt"},
		{"/", "/test.txt"},
	} {
		t.Run(tc.prefix, func(t *testing.T) {
			client := djangotest.NewClient(t, staticfiles.Handler(tc.prefix, finder))

			resp := client.Get(tc.request)

			djangotest.AssertEqual(t, resp.StatusCode, http.StatusOK)
			assertIn(t, resp.Content(), "Can we find")
		})
	}
}

// TestHandlerOutsideThePrefix answers 404 for a path that is not below the
// prefix, also when it only starts with the same letters.
func TestHandlerOutsideThePrefix(t *testing.T) {
	client, _ := siteClient(t)
	for _, path := range []string{"/test.txt", "/media/test.txt", "/staticfiles/test.txt", "/static"} {
		t.Run(path, func(t *testing.T) {
			djangotest.AssertEqual(t, client.Get(path).StatusCode, http.StatusNotFound)
		})
	}
}

func TestMiddlewarePassesOtherPathsOn(t *testing.T) {
	handler := staticfiles.Middleware("/static/", newSite(t).finder())(passedOn)
	client := djangotest.NewClient(t, handler)
	for _, path := range []string{"/", "/about/", "/staticfiles/test.txt", "/static"} {
		t.Run(path, func(t *testing.T) {
			djangotest.AssertEqual(t, client.Get(path).StatusCode, http.StatusTeapot)
		})
	}
}

// TestHandlerFollowsSymlinkInSource serves a file through a symbolic link in
// a source directory, as Django does: a project may link its assets in.
func TestHandlerFollowsSymlinkInSource(t *testing.T) {
	client, s := siteClient(t)
	shared := filepath.Join(t.TempDir(), "shared")
	writeTree(t, shared, map[string]string{"font.txt": "a shared file"})
	if err := os.Symlink(shared, s.path("documents", "shared")); err != nil {
		t.Skipf("symbolic links are not supported: %v", err)
	}

	resp := client.Get("/static/shared/font.txt")

	djangotest.AssertEqual(t, resp.StatusCode, http.StatusOK)
	djangotest.AssertEqual(t, strings.TrimSpace(resp.Content()), "a shared file")
}
