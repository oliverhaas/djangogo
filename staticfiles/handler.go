package staticfiles

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"net/url"
	"path"
	"strings"
)

// Handler serves the files of the finder under urlPrefix, as Django's serve
// view does in DEBUG. It strips urlPrefix itself, so it is mounted on the mux
// pattern urlPrefix, and it answers 404 to a path outside the prefix.
//
// It answers GET and HEAD and refuses other methods with 405. A file that no
// source holds is a 404 and so is a directory: no directory is ever listed.
// The first source that holds the path decides, so a directory in one source
// hides a file of the same name in a later one, as in Django. A path that
// leaves the static root through ".." is a 400.
//
// The Content-Type comes from the file extension (package mime) and is
// application/octet-stream when the extension is unknown; the content is
// never sniffed. A file with a modification time gets a Last-Modified header
// and conditional requests (If-Modified-Since) are answered with 304. Files of
// an embed.FS have no modification time and are always sent in full. Range
// requests work.
//
// The urlPrefix is STATIC_URL: "/static/", "static/" and "/static" name the
// same prefix. An empty prefix and one that names a host, such as the URL of
// a CDN, serve nothing, as Django's static() helper returns no URL pattern for
// them.
func Handler(urlPrefix string, finder *Finder) http.Handler {
	return &handler{prefix: servedPrefix(urlPrefix), finder: finder}
}

// Middleware serves static files for the paths under urlPrefix and passes
// every other request on, for a project that has no route for static files
// (Django's StaticFilesHandler, which runserver wraps around the project). A
// request under the prefix is answered as Handler answers it and never reaches
// the next handler, also when the file is missing.
func Middleware(urlPrefix string, finder *Finder) func(http.Handler) http.Handler {
	static := &handler{prefix: servedPrefix(urlPrefix), finder: finder}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if rel, ok := static.strip(r.URL.Path); ok {
				static.serve(w, r, rel)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// noIndexes is Django's message for a request that names a directory.
const noIndexes = "Directory indexes are not allowed here."

// handler serves the files of a finder under a URL prefix.
type handler struct {
	// prefix is the URL path of the static files with a slash at both ends,
	// or "" when nothing is served.
	prefix string
	finder *Finder
}

// servedPrefix returns the URL path under which files are served, with a
// slash at both ends. It returns "" for a prefix that serves nothing: an empty
// one, which would take over every path of the site, and one with a scheme or
// a host, whose files another server delivers.
func servedPrefix(urlPrefix string) string {
	if urlPrefix == "" {
		return ""
	}
	u, err := url.Parse(urlPrefix)
	if err != nil || u.Scheme != "" || u.Host != "" {
		return ""
	}
	inner := strings.Trim(u.Path, "/")
	if inner == "" {
		return "/"
	}
	return "/" + inner + "/"
}

// strip returns the part of a request path below the prefix. It reports false
// for a path that is not below it.
func (h *handler) strip(urlPath string) (string, bool) {
	if h.prefix == "" {
		return "", false
	}
	return strings.CutPrefix(urlPath, h.prefix)
}

// ServeHTTP serves the file that the request path names.
func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rel, ok := h.strip(r.URL.Path)
	if !ok {
		http.NotFound(w, r)
		return
	}
	h.serve(w, r, rel)
}

// serve answers a request for rel, the request path below the prefix. It
// follows Django's serve views: the path is normalized, so an empty element or
// a trailing slash does not hide a file.
func (h *handler) serve(w http.ResponseWriter, r *http.Request, rel string) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		httpError(w, http.StatusMethodNotAllowed)
		return
	}
	name := strings.TrimLeft(path.Clean(rel), "/")
	if name == "" {
		name = "."
	}
	if !fs.ValidPath(name) {
		// The path climbs out of the static root. Django answers 400 here
		// (SuspiciousFileOperation).
		httpError(w, http.StatusBadRequest)
		return
	}
	match, info, ok := h.finder.find(name)
	if !ok || !info.Mode().IsRegular() {
		// Django has two messages: one for a request that names a directory,
		// found or not, and one for a missing file.
		msg := fmt.Sprintf("'%s' could not be found", rel)
		if rel == "" || strings.HasSuffix(rel, "/") || (ok && info.IsDir()) {
			msg = noIndexes
		}
		http.Error(w, msg, http.StatusNotFound)
		return
	}
	file, err := match.open()
	if err != nil {
		httpError(w, openStatus(err))
		return
	}
	defer func() { _ = file.Close() }()
	serveFile(w, r, name, file, info)
}

// serveFile writes the file as the response. http.ServeContent does the work
// of the conditional and range requests; it needs to seek, so a file of a
// filesystem that cannot is read into memory first.
func serveFile(w http.ResponseWriter, r *http.Request, name string, file fs.File, info fs.FileInfo) {
	content, ok := file.(io.ReadSeeker)
	if !ok {
		data, err := io.ReadAll(file)
		if err != nil {
			httpError(w, http.StatusInternalServerError)
			return
		}
		content = bytes.NewReader(data)
	}
	// Set the type here: without it ServeContent guesses from the content,
	// and Django's view answers application/octet-stream for an unknown
	// extension.
	ctype := mime.TypeByExtension(path.Ext(name))
	if ctype == "" {
		ctype = "application/octet-stream"
	}
	w.Header().Set("Content-Type", ctype)
	// ServeContent leaves Last-Modified out and ignores If-Modified-Since for
	// the zero time, which is what an embedded file reports.
	http.ServeContent(w, r, name, info.ModTime(), content)
}

// openStatus returns the status for a file that was found and then failed to
// open.
func openStatus(err error) int {
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return http.StatusNotFound
	case errors.Is(err, fs.ErrPermission):
		return http.StatusForbidden
	default:
		return http.StatusInternalServerError
	}
}

// httpError answers with a status and its standard text.
func httpError(w http.ResponseWriter, status int) {
	http.Error(w, http.StatusText(status), status)
}
