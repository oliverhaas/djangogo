package middleware_test

import (
	"bytes"
	"compress/gzip"
	"crypto/rand"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/middleware"
)

// The tests in this file are ports of GZipMiddlewareTest and
// ETagGZipMiddlewareTest in Django's tests/middleware/tests.py. An HttpResponse
// becomes a view that writes its body in one call, and a StreamingHttpResponse
// becomes a view that flushes after every chunk.

// gzipFNAME is the flag bit of a gzip header that announces a file name
// (gzip.FNAME in Python).
const gzipFNAME byte = 0x08

var (
	// shortString is too short to be worth compressing.
	shortString = []byte("This string is too short to be worth compressing.")
	// compressibleString compresses well.
	compressibleString = bytes.Repeat([]byte("a"), 500)
	// sequence is the body of the streaming responses, chunk by chunk.
	sequence = [][]byte{
		bytes.Repeat([]byte("a"), 500),
		bytes.Repeat([]byte("b"), 200),
		bytes.Repeat([]byte("a"), 300),
	}
	// sequenceUnicode is a streaming body with characters outside ASCII.
	sequenceUnicode = [][]byte{
		bytes.Repeat([]byte("a"), 500),
		[]byte(strings.Repeat("é", 200)),
		bytes.Repeat([]byte("a"), 300),
	}
)

// contentView is the port of the HttpResponse the Django tests build in setUp:
// a view that answers status with body and the given extra headers.
func contentView(status int, body []byte, headers map[string]string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=UTF-8")
		for k, v := range headers {
			w.Header().Set(k, v)
		}
		w.WriteHeader(status)
		_, _ = w.Write(body)
	})
}

// streamView is the port of a StreamingHttpResponse: a view that writes the
// chunks one by one and flushes after each of them.
func streamView(chunks [][]byte) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=UTF-8")
		for _, chunk := range chunks {
			_, _ = w.Write(chunk)
			_ = http.NewResponseController(w).Flush()
		}
	})
}

// gzipGet sends the request the Django tests build in setUp: a GET that accepts
// gzip and names a browser.
func gzipGet(t *testing.T, handler http.Handler, opts ...djangotest.RequestOption) *djangotest.Response {
	t.Helper()
	opts = append([]djangotest.RequestOption{
		djangotest.Header("Accept-Encoding", "gzip, deflate"),
		djangotest.Header("User-Agent", "Mozilla/5.0 (Windows NT 5.1; rv:9.0.1) Gecko/20100101 Firefox/9.0.1"),
	}, opts...)
	return djangotest.NewClient(t, handler).Get("/", opts...)
}

// decompress returns the content of a gzip stream. Like the Django helper it
// makes sure the input holds exactly one gzip stream and nothing after it.
func decompress(t *testing.T, gzipped []byte) []byte {
	t.Helper()
	plain, _ := readGzip(t, gzipped)
	return plain
}

// gzipMTime returns the modification time stored in the header of a gzip
// stream (get_mtime in the Django tests). The zero time stands for mtime 0.
func gzipMTime(t *testing.T, gzipped []byte) time.Time {
	t.Helper()
	_, header := readGzip(t, gzipped)
	return header.ModTime
}

// readGzip reads one gzip stream and returns its content and header.
func readGzip(t *testing.T, gzipped []byte) ([]byte, gzip.Header) {
	t.Helper()
	src := bytes.NewReader(gzipped)
	zr, err := gzip.NewReader(src)
	if err != nil {
		t.Fatalf("not a gzip stream: %v (%d bytes)", err, len(gzipped))
	}
	zr.Multistream(false)
	plain, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("read the gzip stream: %v", err)
	}
	if src.Len() != 0 {
		t.Fatalf("%d bytes follow the gzip stream", src.Len())
	}
	return plain, zr.Header
}

// TestCompressResponse checks that compression is performed on responses with
// compressible content.
//
// Ported from Django: tests/middleware/tests.py::GZipMiddlewareTest.test_compress_response
func TestCompressResponse(t *testing.T) {
	resp := gzipGet(t, middleware.GZip()(contentView(http.StatusOK, compressibleString, nil)))
	djangotest.AssertEqual(t, decompress(t, resp.Body), compressibleString)
	djangotest.AssertEqual(t, resp.Header.Get("Content-Encoding"), "gzip")
	djangotest.AssertEqual(t, resp.Header.Get("Content-Length"), strconv.Itoa(len(resp.Body)))
}

// TestCompressStreamingResponse checks that compression is performed on
// responses with streaming content.
//
// Ported from Django: tests/middleware/tests.py::GZipMiddlewareTest.test_compress_streaming_response
func TestCompressStreamingResponse(t *testing.T) {
	resp := gzipGet(t, middleware.GZip()(streamView(sequence)))
	djangotest.AssertEqual(t, decompress(t, resp.Body), bytes.Join(sequence, nil))
	djangotest.AssertEqual(t, resp.Header.Get("Content-Encoding"), "gzip")
	assertNoHeader(t, resp, "Content-Length")
}

// TestCompressStreamingResponseUnicode checks that compression is performed on
// responses with streaming Unicode content.
//
// Ported from Django: tests/middleware/tests.py::GZipMiddlewareTest.test_compress_streaming_response_unicode
func TestCompressStreamingResponseUnicode(t *testing.T) {
	resp := gzipGet(t, middleware.GZip()(streamView(sequenceUnicode)))
	djangotest.AssertEqual(t, decompress(t, resp.Body), bytes.Join(sequenceUnicode, nil))
	djangotest.AssertEqual(t, resp.Header.Get("Content-Encoding"), "gzip")
	assertNoHeader(t, resp, "Content-Length")
}

// TestCompressFileResponse checks that compression is performed on a file
// response. http.ServeFile takes the place of FileResponse, and this test file
// is the file served, as in Django. The assertion about file_to_stream has no
// counterpart.
//
// Ported from Django: tests/middleware/tests.py::GZipMiddlewareTest.test_compress_file_response
func TestCompressFileResponse(t *testing.T) {
	const name = "django_gzip_test.go"
	view := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=UTF-8")
		http.ServeFile(w, r, name)
	})
	want, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}

	resp := gzipGet(t, middleware.GZip()(view))
	djangotest.AssertEqual(t, decompress(t, resp.Body), want)
	djangotest.AssertEqual(t, resp.Header.Get("Content-Encoding"), "gzip")
}

// TestCompressNon200Response checks that compression is performed on responses
// with a status other than 200 (Django ticket #10762).
//
// Ported from Django: tests/middleware/tests.py::GZipMiddlewareTest.test_compress_non_200_response
func TestCompressNon200Response(t *testing.T) {
	resp := gzipGet(t, middleware.GZip()(contentView(http.StatusNotFound, compressibleString, nil)))
	djangotest.AssertStatus(t, resp, http.StatusNotFound)
	djangotest.AssertEqual(t, decompress(t, resp.Body), compressibleString)
	djangotest.AssertEqual(t, resp.Header.Get("Content-Encoding"), "gzip")
}

// TestNoCompressShortResponse checks that compression is not performed on
// responses with short content.
//
// Ported from Django: tests/middleware/tests.py::GZipMiddlewareTest.test_no_compress_short_response
func TestNoCompressShortResponse(t *testing.T) {
	resp := gzipGet(t, middleware.GZip()(contentView(http.StatusOK, shortString, nil)))
	djangotest.AssertEqual(t, resp.Body, shortString)
	assertNoHeader(t, resp, "Content-Encoding")
}

// TestNoCompressCompressedResponse checks that compression is not performed on
// responses that are already compressed.
//
// Ported from Django: tests/middleware/tests.py::GZipMiddlewareTest.test_no_compress_compressed_response
func TestNoCompressCompressedResponse(t *testing.T) {
	view := contentView(http.StatusOK, compressibleString, map[string]string{"Content-Encoding": "deflate"})
	resp := gzipGet(t, middleware.GZip()(view))
	djangotest.AssertEqual(t, resp.Body, compressibleString)
	djangotest.AssertEqual(t, resp.Header.Get("Content-Encoding"), "deflate")
}

// TestNoCompressIncompressibleResponse checks that compression is not performed
// on responses with incompressible content.
//
// Ported from Django: tests/middleware/tests.py::GZipMiddlewareTest.test_no_compress_incompressible_response
func TestNoCompressIncompressibleResponse(t *testing.T) {
	incompressibleString := make([]byte, 500)
	if _, err := rand.Read(incompressibleString); err != nil {
		t.Fatal(err)
	}
	resp := gzipGet(t, middleware.GZip()(contentView(http.StatusOK, incompressibleString, nil)))
	djangotest.AssertEqual(t, resp.Body, incompressibleString)
	assertNoHeader(t, resp, "Content-Encoding")
}

// TestCompressDeterministic checks that compression results are the same for
// the same content and do not include a modification time, since that would
// make the results of compression non-deterministic and keep a conditional GET
// from recognizing a match on gzipped content. Django switches the random file
// name off with max_random_bytes = 0 on a subclass; here the source of the
// random length always answers 0.
//
// Ported from Django: tests/middleware/tests.py::GZipMiddlewareTest.test_compress_deterministic
func TestCompressDeterministic(t *testing.T) {
	deterministicGZip := middleware.GZipWithRandBelow(func(int) int { return 0 })
	view := contentView(http.StatusOK, compressibleString, nil)

	r1 := gzipGet(t, deterministicGZip(view))
	r2 := gzipGet(t, deterministicGZip(view))
	djangotest.AssertEqual(t, r1.Body, r2.Body)
	djangotest.AssertEqual(t, gzipMTime(t, r1.Body).IsZero(), true)
	djangotest.AssertEqual(t, gzipMTime(t, r2.Body).IsZero(), true)
}

// TestRandomBytes checks that a random number of bytes is added to mitigate the
// BREACH attack. Django patches secrets.randbelow to return 3; here the source
// of the random length is replaced the same way.
//
// Ported from Django: tests/middleware/tests.py::GZipMiddlewareTest.test_random_bytes
func TestRandomBytes(t *testing.T) {
	mw := middleware.GZipWithRandBelow(func(int) int { return 3 })
	resp := gzipGet(t, mw(contentView(http.StatusOK, compressibleString, nil)))
	if len(resp.Body) < 14 {
		t.Fatalf("response body has %d bytes, want a gzip stream", len(resp.Body))
	}
	// The fourth byte of a gzip stream contains flags.
	djangotest.AssertEqual(t, resp.Body[3], gzipFNAME)
	// A 3 byte filename "aaa" and a null byte are added.
	djangotest.AssertEqual(t, resp.Body[10:14], []byte("aaa\x00"))
	djangotest.AssertEqual(t, decompress(t, resp.Body), compressibleString)
}

// TestRandomBytesStreamingResponse checks that a random number of bytes is
// added to a streaming response as well.
//
// Ported from Django: tests/middleware/tests.py::GZipMiddlewareTest.test_random_bytes_streaming_response
func TestRandomBytesStreamingResponse(t *testing.T) {
	mw := middleware.GZipWithRandBelow(func(int) int { return 3 })
	resp := gzipGet(t, mw(streamView(sequence)))
	if len(resp.Body) < 14 {
		t.Fatalf("response body has %d bytes, want a gzip stream", len(resp.Body))
	}
	// The fourth byte of a gzip stream contains flags.
	djangotest.AssertEqual(t, resp.Body[3], gzipFNAME)
	// A 3 byte filename "aaa" and a null byte are added.
	djangotest.AssertEqual(t, resp.Body[10:14], []byte("aaa\x00"))
	djangotest.AssertEqual(t, decompress(t, resp.Body), bytes.Join(sequence, nil))
}

// TestStrongETagModified checks that GZip makes a strong ETag weak.
//
// Ported from Django: tests/middleware/tests.py::ETagGZipMiddlewareTest.test_strong_etag_modified
func TestStrongETagModified(t *testing.T) {
	view := contentView(http.StatusOK, compressibleString, map[string]string{"ETag": `"eggs"`})
	resp := gzipGet(t, middleware.GZip()(view))
	djangotest.AssertEqual(t, resp.Header.Get("ETag"), `W/"eggs"`)
}

// TestWeakETagNotModified checks that GZip does not modify a weak ETag.
//
// Ported from Django: tests/middleware/tests.py::ETagGZipMiddlewareTest.test_weak_etag_not_modified
func TestWeakETagNotModified(t *testing.T) {
	view := contentView(http.StatusOK, compressibleString, map[string]string{"ETag": `W/"eggs"`})
	resp := gzipGet(t, middleware.GZip()(view))
	djangotest.AssertEqual(t, resp.Header.Get("ETag"), `W/"eggs"`)
}

// TestETagMatch checks that GZip allows 304 Not Modified responses. Django
// answers the conditional request with ConditionalGetMiddleware. This package
// has no such middleware, so the view sets an ETag and http.ServeContent, which
// knows conditional requests, answers. The second request passes GZip as well,
// which adds the check that a 304 goes through untouched.
//
// Ported from Django: tests/middleware/tests.py::ETagGZipMiddlewareTest.test_etag_match
func TestETagMatch(t *testing.T) {
	condView := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `"eggs"`)
		http.ServeContent(w, r, "eggs.html", time.Time{}, bytes.NewReader(compressibleString))
	})
	handler := middleware.GZip()(condView)

	resp := gzipGet(t, handler)
	djangotest.AssertStatus(t, resp, http.StatusOK)
	djangotest.AssertEqual(t, decompress(t, resp.Body), compressibleString)
	gzipETag := resp.Header.Get("ETag")
	djangotest.AssertEqual(t, gzipETag, `W/"eggs"`)

	next := gzipGet(t, handler, djangotest.Header("If-None-Match", gzipETag))
	djangotest.AssertStatus(t, next, http.StatusNotModified)
	djangotest.AssertEqual(t, len(next.Body), 0)
	assertNoHeader(t, next, "Content-Encoding")
}
