package middleware_test

import (
	"bytes"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/middleware"
)

// TestGZipClientWithoutGzip checks the response for a client that does not
// accept gzip: the body and its length are untouched, and the Vary header
// still tells a cache that the response depends on Accept-Encoding.
func TestGZipClientWithoutGzip(t *testing.T) {
	view := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(len(compressibleString)))
		_, _ = w.Write(compressibleString[:150])
		_, _ = w.Write(compressibleString[150:])
	})

	for _, acceptEncoding := range []string{"", "deflate, br", "gzipped"} {
		t.Run(acceptEncoding, func(t *testing.T) {
			var opts []djangotest.RequestOption
			if acceptEncoding != "" {
				opts = append(opts, djangotest.Header("Accept-Encoding", acceptEncoding))
			}
			resp := djangotest.NewClient(t, middleware.GZip()(view)).Get("/", opts...)
			djangotest.AssertEqual(t, resp.Body, compressibleString)
			djangotest.AssertEqual(t, resp.Header.Get("Content-Length"), "500")
			djangotest.AssertEqual(t, resp.Header.Get("Vary"), "Accept-Encoding")
			assertNoHeader(t, resp, "Content-Encoding")
		})
	}
}

// TestGZipVaryHeader checks how Accept-Encoding joins a Vary header the
// handler set. The expected values are what patch_vary_headers of Django 6.0
// gives for the same headers.
func TestGZipVaryHeader(t *testing.T) {
	for _, tc := range []struct{ vary, want string }{
		{"Cookie", "Cookie, Accept-Encoding"},
		{"accept-encoding", "accept-encoding"},
		{"*", "*"},
		{"Cookie, *", "*"},
		{"Cookie,Origin", "Cookie, Origin, Accept-Encoding"},
		{"Cookie ,  Origin", "Cookie, Origin, Accept-Encoding"},
	} {
		t.Run(tc.vary, func(t *testing.T) {
			view := contentView(http.StatusOK, compressibleString, map[string]string{"Vary": tc.vary})
			resp := gzipGet(t, middleware.GZip()(view))
			djangotest.AssertEqual(t, resp.Header.Get("Content-Encoding"), "gzip")
			djangotest.AssertEqual(t, resp.Header.Values("Vary"), []string{tc.want})
		})
	}

	// A response that is too short to be compressed for anybody does not vary.
	resp := gzipGet(t, middleware.GZip()(contentView(http.StatusOK, shortString, nil)))
	assertNoHeader(t, resp, "Vary")
}

// TestGZipKeepsStatus checks that the status the handler chose is sent with
// the compressed response.
func TestGZipKeepsStatus(t *testing.T) {
	resp := gzipGet(t, middleware.GZip()(contentView(http.StatusNotFound, compressibleString, nil)))
	djangotest.AssertStatus(t, resp, http.StatusNotFound)
	djangotest.AssertEqual(t, resp.Header.Get("Content-Encoding"), "gzip")
	djangotest.AssertEqual(t, decompress(t, resp.Body), compressibleString)
}

// TestGZipStatusWithoutBody checks that a response whose status allows no body
// passes as it is, with the headers that describe the resource.
func TestGZipStatusWithoutBody(t *testing.T) {
	for _, status := range []int{http.StatusNoContent, http.StatusNotModified} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			view := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("ETag", `"resource-1"`)
				w.WriteHeader(status)
			})
			resp := gzipGet(t, middleware.GZip()(view))
			djangotest.AssertStatus(t, resp, status)
			djangotest.AssertEqual(t, len(resp.Body), 0)
			djangotest.AssertEqual(t, resp.Header.Get("ETag"), `"resource-1"`)
			assertNoHeader(t, resp, "Content-Encoding")
		})
	}
}

// TestGZipPartialContent checks that a partial response is not compressed: its
// Content-Range counts bytes of the body as it is.
func TestGZipPartialContent(t *testing.T) {
	view := contentView(http.StatusPartialContent, compressibleString, map[string]string{
		"Content-Range": "bytes 0-499/2000",
	})
	resp := gzipGet(t, middleware.GZip()(view))
	djangotest.AssertStatus(t, resp, http.StatusPartialContent)
	djangotest.AssertEqual(t, resp.Body, compressibleString)
	djangotest.AssertEqual(t, resp.Header.Get("Content-Range"), "bytes 0-499/2000")
	assertNoHeader(t, resp, "Content-Encoding")
}

// TestGZipServeContent checks GZip around http.ServeContent, which serves
// files with ranges and validators. The whole file is compressed and no longer
// offers ranges, and a range of it is served as it is.
func TestGZipServeContent(t *testing.T) {
	content := []byte(strings.Repeat("The quick brown fox jumps over the lazy dog. ", 100))
	modified := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	handler := middleware.GZip()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeContent(w, r, "fox.txt", modified, bytes.NewReader(content))
	}))

	resp := gzipGet(t, handler)
	djangotest.AssertStatus(t, resp, http.StatusOK)
	djangotest.AssertEqual(t, resp.Header.Get("Content-Encoding"), "gzip")
	djangotest.AssertEqual(t, resp.Header.Get("Content-Length"), strconv.Itoa(len(resp.Body)))
	djangotest.AssertEqual(t, resp.Header.Get("Content-Type"), "text/plain; charset=utf-8")
	djangotest.AssertEqual(t, decompress(t, resp.Body), content)
	assertNoHeader(t, resp, "Accept-Ranges")

	resp = gzipGet(t, handler, djangotest.Header("Range", "bytes=4-8"))
	djangotest.AssertStatus(t, resp, http.StatusPartialContent)
	djangotest.AssertEqual(t, resp.Content(), "quick")
	djangotest.AssertEqual(t, resp.Header.Get("Content-Range"), "bytes 4-8/4500")
	assertNoHeader(t, resp, "Content-Encoding")

	resp = gzipGet(t, handler, djangotest.Header("If-Modified-Since", modified.Format(http.TimeFormat)))
	djangotest.AssertStatus(t, resp, http.StatusNotModified)
	djangotest.AssertEqual(t, len(resp.Body), 0)
}

// TestGZipSniffsContentType checks that a compressed response without a
// Content-Type gets the type of its content. Left to net/http, the type would
// be guessed from the compressed bytes or not at all.
func TestGZipSniffsContentType(t *testing.T) {
	page := []byte("<!doctype html><html><body>" + strings.Repeat("<p>paragraph</p>", 50) + "</body></html>")
	view := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(page)
	})

	resp := gzipGet(t, middleware.GZip()(view))
	djangotest.AssertEqual(t, resp.Header.Get("Content-Encoding"), "gzip")
	djangotest.AssertEqual(t, resp.Header.Get("Content-Type"), "text/html; charset=utf-8")
	djangotest.AssertEqual(t, decompress(t, resp.Body), page)
}

// TestGZipLargeResponse checks that a response beyond the buffer limit of
// 1 MiB is compressed as a stream: the content arrives whole, and the
// Content-Length the handler set for the plain body is gone.
func TestGZipLargeResponse(t *testing.T) {
	line := []byte("0123456789 the same line again and again, a megabyte and more of it\n")
	const lines = 40000
	view := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Content-Length", strconv.Itoa(lines*len(line)))
		for range lines {
			_, _ = w.Write(line)
		}
	})

	resp := gzipGet(t, middleware.GZip()(view))
	djangotest.AssertEqual(t, resp.Header.Get("Content-Encoding"), "gzip")
	assertNoHeader(t, resp, "Content-Length")
	plain := decompress(t, resp.Body)
	djangotest.AssertEqual(t, len(plain), lines*len(line))
	djangotest.AssertEqual(t, bytes.Equal(plain, bytes.Repeat(line, lines)), true)
}

// TestGZipPanicDropsBuffer checks the ordering rule for Recover: the handler
// panics after it wrote a compressible body, and with GZip inside Recover the
// client gets the plain error page and nothing of that body.
func TestGZipPanicDropsBuffer(t *testing.T) {
	view := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=UTF-8")
		_, _ = w.Write(compressibleString)
		panic("boom")
	})
	handler := middleware.Chain(
		middleware.Recover(middleware.RecoverOptions{Log: io.Discard}),
		middleware.GZip(),
	)(view)

	resp := gzipGet(t, handler)
	djangotest.AssertStatus(t, resp, http.StatusInternalServerError)
	djangotest.AssertEqual(t, resp.Content(), serverErrorPage)
	assertNoHeader(t, resp, "Content-Encoding")
}
