package middleware

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"crypto/rand"
	"io"
	"math/big"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

const (
	// gzipMinLength is the size below which a complete response is not worth
	// compressing, as in Django.
	gzipMinLength = 200
	// gzipBufferLimit is how much of a response is held back to learn its
	// size. A larger response is compressed as a stream.
	gzipBufferLimit = 1 << 20
	// gzipLevel is the compression level Django uses.
	gzipLevel = 6
	// gzipMaxRandomBytes is max_random_bytes of Django's GZipMiddleware: the
	// random file name in the gzip header is shorter than this.
	gzipMaxRandomBytes = 100
)

// acceptsGzipRE is re_accepts_gzip of django/middleware/gzip.py.
var acceptsGzipRE = regexp.MustCompile(`\bgzip\b`)

// gzipWriterPool recycles gzip writers, which are expensive to allocate.
var gzipWriterPool = sync.Pool{New: func() any {
	// The level is a valid constant, so the error cannot occur.
	zw, _ := gzip.NewWriterLevel(io.Discard, gzipLevel)
	return zw
}}

// GZip is Django's GZipMiddleware: it compresses the response body with gzip
// for a client that sends "Accept-Encoding: gzip".
//
// A response is left alone when it is complete and shorter than 200 bytes, when
// it already has a Content-Encoding, and when compression would not make it
// shorter. A compressed response gets "Content-Encoding: gzip", a Content-Length
// that matches or none at all, and a weak ETag in the place of a strong one,
// because the bytes are no longer those the ETag was made for. Every response
// that could have been compressed gets "Vary: Accept-Encoding", whatever the
// client accepts, so that a cache keeps the two forms apart.
//
// A response that is partial (206, or one with a Content-Range header) is never
// compressed, and a compressed response loses its Accept-Ranges header: ranges
// count bytes of the body as it was before the compression.
//
// The response is buffered until the handler returns, so that its size is
// known. A handler that flushes, or that writes more than 1 MiB, gets a stream:
// from then on every write is compressed and passed on, and every flush
// reaches the client. Headers the handler sets before that point still count,
// even after its call to WriteHeader.
//
// The gzip header carries a file name of random length, Django's mitigation of
// the BREACH attack ("Heal The Breach"): the length of a compressed response
// no longer tells an attacker how well a guess compressed.
//
// Put GZip inside Recover. A response that is still buffered when the handler
// panics is dropped, and Recover can answer with an error page.
func GZip() func(http.Handler) http.Handler {
	return newGZip(cryptoRandBelow)
}

// newGZip builds the GZip middleware with randBelow as the source of the
// length of the random file name. randBelow(n) returns a number from 0 up to,
// not including, n.
func newGZip(randBelow func(n int) int) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gw := &gzipWriter{
				ResponseWriter: w,
				acceptsGzip:    acceptsGzipRE.MatchString(strings.Join(r.Header.Values("Accept-Encoding"), ", ")),
				randBelow:      randBelow,
			}
			next.ServeHTTP(gw, r)
			// Not deferred on purpose: after a panic the buffered response
			// must not be sent.
			gw.finish()
		})
	}
}

// cryptoRandBelow returns a uniform random number from 0 up to, not including,
// n, from the random source of the operating system (Python's
// secrets.randbelow). The length it chooses must not be predictable.
func cryptoRandBelow(n int) int {
	if n <= 0 {
		return 0
	}
	v, err := rand.Int(rand.Reader, big.NewInt(int64(n)))
	if err != nil {
		return 0
	}
	return int(v.Int64())
}

// gzipWriter is the http.ResponseWriter GZip hands to the wrapped handler. It
// holds the response back until it can decide about the compression, and then
// passes it on, compressed or not.
type gzipWriter struct {
	http.ResponseWriter
	// acceptsGzip tells whether the client accepts gzip.
	acceptsGzip bool
	// randBelow chooses the length of the random file name.
	randBelow func(n int) int

	// status is the status code the handler chose, or 0 while it chose none.
	status int
	// buf holds the body written while the decision is open.
	buf bytes.Buffer
	// committed is set once the decision is made and the header is sent.
	committed bool
	// gz compresses the stream. It is nil unless the response is compressed
	// as a stream.
	gz *gzip.Writer
	// hijacked is set once the handler took over the connection.
	hijacked bool
}

// bodyAllowed reports whether a response with the status can have a body.
func bodyAllowed(status int) bool {
	return (status < 100 || status > 199) && status != http.StatusNoContent && status != http.StatusNotModified
}

// WriteHeader records the status code. The header is sent later, when the
// decision about the compression is made. A status without a body needs no
// decision and is sent at once, and so is an informational status.
func (w *gzipWriter) WriteHeader(code int) {
	switch {
	case w.committed || w.hijacked:
		w.ResponseWriter.WriteHeader(code)
	case code >= 100 && code < 200 && code != http.StatusSwitchingProtocols:
		w.ResponseWriter.WriteHeader(code)
	case w.status != 0:
		// A second status is ignored, as net/http ignores it.
	case !bodyAllowed(code):
		w.status = code
		_ = w.commit(false)
	default:
		w.status = code
	}
}

// Write takes body bytes. While the decision about the compression is open
// they are buffered. A client that does not accept gzip only needs the
// decision about the Vary header, which is made after 200 bytes.
func (w *gzipWriter) Write(p []byte) (int, error) {
	switch {
	case w.hijacked:
		return 0, http.ErrHijacked
	case w.gz != nil:
		return w.gz.Write(p)
	case w.committed:
		return w.ResponseWriter.Write(p)
	}

	w.buf.Write(p)
	limit := gzipBufferLimit
	if !w.acceptsGzip {
		limit = gzipMinLength
	}
	if w.buf.Len() >= limit {
		if err := w.commit(true); err != nil {
			return 0, err
		}
	}
	return len(p), nil
}

// Flush implements http.Flusher. It does nothing when the wrapped writer
// cannot flush.
func (w *gzipWriter) Flush() {
	_ = w.FlushError()
}

// FlushError sends everything written so far to the client and reports whether
// that worked. It is the method http.ResponseController calls. A handler that
// flushes wants its output delivered now, so the response becomes a stream.
func (w *gzipWriter) FlushError() error {
	if !w.committed && !w.hijacked {
		if err := w.commit(true); err != nil {
			return err
		}
	}
	if w.gz != nil {
		if err := w.gz.Flush(); err != nil {
			return err
		}
	}
	return http.NewResponseController(w.ResponseWriter).Flush()
}

// Hijack implements http.Hijacker by delegation. After it the handler owns the
// connection, and what is still buffered is dropped.
func (w *gzipWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	conn, rw, err := http.NewResponseController(w.ResponseWriter).Hijack()
	if err == nil {
		w.hijacked = true
		w.buf.Reset()
	}
	return conn, rw, err
}

// Unwrap returns the wrapped writer, so that http.ResponseController reaches
// the features this type does not delegate itself.
func (w *gzipWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

// finish completes the response after the handler returned: it sends what is
// still buffered and ends the gzip stream.
func (w *gzipWriter) finish() {
	if w.hijacked {
		return
	}
	// A failed write means the client is gone. There is nobody to tell.
	if !w.committed {
		_ = w.commit(false)
	}
	if w.gz != nil {
		_ = w.gz.Close()
		gzipWriterPool.Put(w.gz)
		w.gz = nil
	}
}

// commit ends the buffering: it decides about the compression, sets the
// headers that go with it, and sends the header and the buffered body.
// streaming tells that more of the body may follow, so its size is not known.
func (w *gzipWriter) commit(streaming bool) error {
	w.committed = true
	h := w.Header()
	body := w.buf.Bytes()

	compress := w.shouldCompress(streaming)
	var compressed []byte
	if compress && !streaming {
		// Return the compressed content only if it is shorter.
		compressed = w.compress(body)
		compress = len(compressed) < len(body)
	}

	if compress {
		if _, ok := h["Content-Type"]; !ok && len(body) > 0 {
			// net/http would guess the type from the compressed bytes, or not
			// at all. Guess it from the content while it is at hand.
			h.Set("Content-Type", http.DetectContentType(body))
		}
		if streaming {
			h.Del("Content-Length")
		} else {
			h.Set("Content-Length", strconv.Itoa(len(compressed)))
		}
		// Make a strong ETag weak to fulfill the requirements of RFC 9110
		// Section 8.8.1 while conditional requests still match on it.
		if etag := h.Get("ETag"); strings.HasPrefix(etag, `"`) {
			h.Set("ETag", "W/"+etag)
		}
		h.Set("Content-Encoding", "gzip")
		h.Del("Accept-Ranges")
	}

	if w.status != 0 {
		w.ResponseWriter.WriteHeader(w.status)
	}
	var err error
	switch {
	case compress && streaming:
		w.gz = w.newGzipWriter(w.ResponseWriter)
		if len(body) > 0 {
			_, err = w.gz.Write(body)
		}
	case compress:
		_, err = w.ResponseWriter.Write(compressed)
	case len(body) > 0:
		_, err = w.ResponseWriter.Write(body)
	}
	w.buf = bytes.Buffer{}
	return err
}

// shouldCompress makes the decision of GZipMiddleware.process_response, in its
// order, and adds the Vary header on the way. The check that the compressed
// form is shorter comes after it.
func (w *gzipWriter) shouldCompress(streaming bool) bool {
	h := w.Header()
	switch {
	case !streaming && w.buf.Len() < gzipMinLength:
		// It is not worth attempting to compress really short responses.
		return false
	case len(h.Values("Content-Encoding")) > 0:
		// Avoid gzipping if there is a content encoding already.
		return false
	case w.status == http.StatusPartialContent || len(h.Values("Content-Range")) > 0:
		return false
	}
	patchVaryHeader(h, "Accept-Encoding")
	return w.acceptsGzip
}

// newGzipWriter returns a gzip writer on dst with a file name of random
// length in its header and no modification time, so that equal content gives
// equal output but for that name.
func (w *gzipWriter) newGzipWriter(dst io.Writer) *gzip.Writer {
	zw, _ := gzipWriterPool.Get().(*gzip.Writer)
	zw.Reset(dst)
	zw.Name = strings.Repeat("a", w.randBelow(gzipMaxRandomBytes))
	return zw
}

// compress is Django's compress_string: it returns body as one gzip stream.
func (w *gzipWriter) compress(body []byte) []byte {
	var out bytes.Buffer
	zw := w.newGzipWriter(&out)
	// Writing to a bytes.Buffer cannot fail.
	_, _ = zw.Write(body)
	_ = zw.Close()
	gzipWriterPool.Put(zw)
	return out.Bytes()
}

// patchVaryHeader is django.utils.cache.patch_vary_headers for one header: it
// adds name to the Vary header unless it is there already, in any case. A "*"
// in the header stands for everything and is all that is kept.
func patchVaryHeader(h http.Header, name string) {
	var vary []string
	for field := range strings.SplitSeq(strings.Join(h.Values("Vary"), ","), ",") {
		if field = strings.TrimSpace(field); field != "" {
			vary = append(vary, field)
		}
	}

	present := false
	for _, field := range vary {
		if field == "*" {
			h.Set("Vary", "*")
			return
		}
		if strings.EqualFold(field, name) {
			present = true
		}
	}
	if !present {
		vary = append(vary, name)
	}
	h.Set("Vary", strings.Join(vary, ", "))
}
