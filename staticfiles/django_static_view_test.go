package staticfiles_test

import (
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/staticfiles"
)

// The tests in this file are ports of Django's
// tests/view_tests/tests/test_static.py. They test django.views.static.serve,
// the view that Django's staticfiles serve view hands a found file to; Handler
// does both steps.
//
// Not ported: the directory index tests (Handler never lists a directory, as
// Django's staticfiles view never does), test_chunked (the chunk size of
// Django's FileResponse), StaticHelperTest.test_prefix (a URL regex) and
// StaticHelperTest.test_debug_off (the DEBUG gate is the caller's).
// StaticHelperTest repeats every StaticTests test with a URL pattern built by
// the static() helper, which is the same Handler here, so the repeats are not
// ported either.

// mediaFiles lists tests/view_tests/media/. The content of the .gz file only
// has to look compressed.
var mediaFiles = map[string]string{
	"file.txt":       "An example media file.",
	"file.txt.gz":    "\x1f\x8b\x08\x00\x00\x00\x00\x00\x00\x03not really deflated",
	"%2F.txt":        "%2F content\n",
	"file.unknown":   "An unknown file extension.\n",
	"subdir/visible": "",
	"subdir/.hidden": "hidden\n",
}

// newMedia writes the media directory and returns a client of a handler that
// serves it under /site_media/, as view_tests/urls.py routes it.
func newMedia(t *testing.T) (client *djangotest.Client, dir string) {
	t.Helper()
	dir = filepath.Join(t.TempDir(), "media")
	writeTree(t, dir, mediaFiles)
	handler := staticfiles.Handler("/site_media/", staticfiles.NewFinder(staticfiles.DirSource(dir)))
	return djangotest.NewClient(t, handler), dir
}

// TestStaticServe serves static media. Django answers a .gz file with the type
// of the inner file and "Content-Encoding: gzip", which makes a browser unpack
// a download; Handler serves the file as it is, without a Content-Encoding.
//
// Ported from Django: tests/view_tests/tests/test_static.py::StaticTests.test_serve
func TestStaticServe(t *testing.T) {
	client, dir := newMedia(t)
	for _, filename := range []string{"file.txt", "file.txt.gz", "%2F.txt"} {
		t.Run(filename, func(t *testing.T) {
			resp := client.Get("/site_media/" + quote(filename))

			want, err := os.ReadFile(filepath.Join(dir, filename))
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}
			djangotest.AssertEqual(t, resp.Content(), string(want))
			djangotest.AssertEqual(t, resp.Header.Get("Content-Length"), strconv.Itoa(len(want)))
			djangotest.AssertEqual(t, resp.Header.Get("Content-Encoding"), "")
		})
	}
}

// Ported from Django: tests/view_tests/tests/test_static.py::StaticTests.test_unknown_mime_type
func TestStaticUnknownMimeType(t *testing.T) {
	client, _ := newMedia(t)

	resp := client.Get("/site_media/file.unknown")

	djangotest.AssertEqual(t, resp.Header.Get("Content-Type"), "application/octet-stream")
}

// Ported from Django: tests/view_tests/tests/test_static.py::StaticTests.test_copes_with_empty_path_component
func TestStaticCopesWithEmptyPathComponent(t *testing.T) {
	client, _ := newMedia(t)

	resp := client.Get("/site_media//file.txt")

	djangotest.AssertEqual(t, resp.Content(), mediaFiles["file.txt"])
}

// Ported from Django: tests/view_tests/tests/test_static.py::StaticTests.test_is_modified_since
func TestStaticIsModifiedSince(t *testing.T) {
	client, _ := newMedia(t)

	resp := client.Get("/site_media/file.txt", djangotest.Header("If-Modified-Since", "Thu, 1 Jan 1970 00:00:00 GMT"))

	djangotest.AssertEqual(t, resp.Content(), mediaFiles["file.txt"])
}

// Ported from Django: tests/view_tests/tests/test_static.py::StaticTests.test_not_modified_since
func TestStaticNotModifiedSince(t *testing.T) {
	client, _ := newMedia(t)

	// This is 24h before max Unix time.
	resp := client.Get("/site_media/file.txt", djangotest.Header("If-Modified-Since", "Mon, 18 Jan 2038 05:14:07 GMT"))

	djangotest.AssertEqual(t, resp.StatusCode, http.StatusNotModified)
}

// TestStaticInvalidIfModifiedSince handles a bogus If-Modified-Since value: a
// file counts as modified since an invalid timestamp (RFC 9110, section
// 13.1.3).
//
// Ported from Django: tests/view_tests/tests/test_static.py::StaticTests.test_invalid_if_modified_since
func TestStaticInvalidIfModifiedSince(t *testing.T) {
	client, _ := newMedia(t)

	resp := client.Get("/site_media/file.txt", djangotest.Header("If-Modified-Since", "Mon, 28 May 999999999999 28:25:26 GMT"))

	djangotest.AssertEqual(t, resp.Content(), mediaFiles["file.txt"])
	djangotest.AssertEqual(t, resp.Header.Get("Content-Length"), strconv.Itoa(len(mediaFiles["file.txt"])))
}

// TestStaticInvalidIfModifiedSince2 handles an even more bogus
// If-Modified-Since value.
//
// Ported from Django: tests/view_tests/tests/test_static.py::StaticTests.test_invalid_if_modified_since2
func TestStaticInvalidIfModifiedSince2(t *testing.T) {
	client, _ := newMedia(t)

	resp := client.Get("/site_media/file.txt", djangotest.Header("If-Modified-Since", ": 1291108438, Wed, 20 Oct 2010 14:05:00 GMT"))

	djangotest.AssertEqual(t, resp.Content(), mediaFiles["file.txt"])
	djangotest.AssertEqual(t, resp.Header.Get("Content-Length"), strconv.Itoa(len(mediaFiles["file.txt"])))
}

// Ported from Django: tests/view_tests/tests/test_static.py::StaticTests.test_404
func TestStatic404(t *testing.T) {
	client, _ := newMedia(t)

	resp := client.Get("/site_media/nonexistent_resource")

	djangotest.AssertEqual(t, resp.StatusCode, http.StatusNotFound)
}

// passedOn is the handler behind a Middleware in the prefix tests: it marks
// the requests that the middleware did not serve.
var passedOn = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusTeapot)
})

// TestStaticHelperSpecialPrefix serves nothing when the prefix names a host,
// as a STATIC_URL that points at a CDN does: Django's static() helper returns
// no URL pattern, and the middleware passes every request on.
//
// Ported from Django: tests/view_tests/tests/test_static.py::StaticHelperTest.test_special_prefix
func TestStaticHelperSpecialPrefix(t *testing.T) {
	_, dir := newMedia(t)
	finder := staticfiles.NewFinder(staticfiles.DirSource(dir))
	for _, prefix := range []string{"http://example.org", "//example.org"} {
		t.Run(prefix, func(t *testing.T) {
			client := djangotest.NewClient(t, staticfiles.Middleware(prefix, finder)(passedOn))
			djangotest.AssertEqual(t, client.Get("/file.txt").StatusCode, http.StatusTeapot)

			client = djangotest.NewClient(t, staticfiles.Handler(prefix, finder))
			djangotest.AssertEqual(t, client.Get("/file.txt").StatusCode, http.StatusNotFound)
		})
	}
}

// TestStaticHelperEmptyPrefix serves nothing for an empty prefix. Django's
// static() helper raises ImproperlyConfigured("Empty static prefix not
// permitted"); a constructor of a handler has no error to return, and serving
// every path of the site would be worse, so the handler stays out of the way.
//
// Ported from Django: tests/view_tests/tests/test_static.py::StaticHelperTest.test_empty_prefix
func TestStaticHelperEmptyPrefix(t *testing.T) {
	_, dir := newMedia(t)
	finder := staticfiles.NewFinder(staticfiles.DirSource(dir))

	client := djangotest.NewClient(t, staticfiles.Middleware("", finder)(passedOn))
	djangotest.AssertEqual(t, client.Get("/file.txt").StatusCode, http.StatusTeapot)

	client = djangotest.NewClient(t, staticfiles.Handler("", finder))
	djangotest.AssertEqual(t, client.Get("/file.txt").StatusCode, http.StatusNotFound)
}

// TestWasModifiedSinceFp answers 304 for a file whose modification time has a
// fraction of a second (#18675). Django tests its was_modified_since function;
// the port sends the request that calls for the comparison.
//
// Ported from Django: tests/view_tests/tests/test_static.py::StaticUtilsTests.test_was_modified_since_fp
func TestWasModifiedSinceFp(t *testing.T) {
	client, dir := newMedia(t)
	mtime := time.Unix(1343416141, 107817000)
	if err := os.Chtimes(filepath.Join(dir, "file.txt"), mtime, mtime); err != nil {
		t.Fatalf("set modification time: %v", err)
	}
	header := mtime.UTC().Format(http.TimeFormat)

	resp := client.Get("/site_media/file.txt", djangotest.Header("If-Modified-Since", header))

	djangotest.AssertEqual(t, resp.StatusCode, http.StatusNotModified)
}

// TestWasModifiedSinceEmptyString serves the file for an empty
// If-Modified-Since header.
//
// Ported from Django: tests/view_tests/tests/test_static.py::StaticUtilsTests.test_was_modified_since_empty_string
func TestWasModifiedSinceEmptyString(t *testing.T) {
	client, _ := newMedia(t)

	resp := client.Get("/site_media/file.txt", djangotest.Header("If-Modified-Since", ""))

	djangotest.AssertEqual(t, resp.StatusCode, http.StatusOK)
	djangotest.AssertEqual(t, resp.Content(), mediaFiles["file.txt"])
}
