package middleware

import (
	"io"
	"maps"
	"mime"
	"net/http"
	"net/url"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"
)

// cleansedSubstitute is what the technical 500 page shows in the place of a
// sensitive value (SafeExceptionReporterFilter.cleansed_substitute).
const cleansedSubstitute = "********************"

// debugBodyLimit bounds how much of a request body is kept and read for the
// POST section of the technical 500 page.
const debugBodyLimit = 1 << 20

// hiddenNameRE is SafeExceptionReporterFilter.hidden_settings: a name it
// matches hints at a secret, and its value is not shown.
var hiddenNameRE = regexp.MustCompile(`(?i)API|AUTH|TOKEN|KEY|SECRET|PASS|SIGNATURE|HTTP_COOKIE`)

// debugReport is everything the technical 500 page shows.
type debugReport struct {
	// request is the request as the wrapped handler got it.
	request *http.Request
	// body recorded what the handler read of the request body. It is nil when
	// the body is not a form.
	body *bodyRecorder
	// panic describes the panic.
	panic *panicReport
	// sessionCookieName is the cookie whose value is never shown.
	sessionCookieName string
	// now is the server time shown on the page.
	now time.Time
}

// debugVar is one row of a table of the page: a name and the text shown as its
// value.
type debugVar struct {
	name, value string
}

// bodyRecorder is a request body that keeps a copy of what is read from it, up
// to debugBodyLimit bytes. A handler consumes the body, and the technical 500
// page shows the POST data afterwards.
type bodyRecorder struct {
	io.ReadCloser
	// data is the start of the body, as far as it was read.
	data []byte
}

// Read reads from the body and keeps a copy.
func (b *bodyRecorder) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if room := debugBodyLimit - len(b.data); room > 0 {
		b.data = append(b.data, p[:min(n, room)]...)
	}
	return n, err
}

// recordBody returns a copy of the request whose body is recorded, and the
// recorder. A request without a URL-encoded form as its body is returned as it
// is, with a nil recorder.
func recordBody(r *http.Request) (*http.Request, *bodyRecorder) {
	if r.Body == nil || r.Body == http.NoBody {
		return r, nil
	}
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/x-www-form-urlencoded" {
		return r, nil
	}
	recorder := &bodyRecorder{ReadCloser: r.Body}
	// WithContext makes the shallow copy. The request of the caller keeps its
	// own body.
	inner := r.WithContext(r.Context())
	inner.Body = recorder
	return inner, recorder
}

// postData returns the POST data of the request: the form the handler parsed,
// or else the form in the recorded body. The part of the body the handler did
// not read is read first, as far as the limit allows.
func (rep *debugReport) postData() url.Values {
	if rep.request.PostForm != nil {
		return rep.request.PostForm
	}
	if rep.body == nil {
		return nil
	}
	if room := int64(debugBodyLimit - len(rep.body.data)); room > 0 {
		// An error leaves the form incomplete, which is all it can do.
		_, _ = io.Copy(io.Discard, io.LimitReader(rep.body, room))
	}
	// A form cut off at the limit gives an error and the values before it.
	values, _ := url.ParseQuery(string(rep.body.data))
	return values
}

// isHiddenName reports whether a name hints at a secret.
func isHiddenName(name string) bool {
	return hiddenNameRE.MatchString(name)
}

// reprValues formats the values of one name the way the page shows them: the
// Python repr of the value, or of the list when there is more than one.
func reprValues(values []string) string {
	if len(values) == 1 {
		return pyRepr(values[0])
	}
	reprs := make([]string, len(values))
	for i, value := range values {
		reprs[i] = pyRepr(value)
	}
	return "[" + strings.Join(reprs, ", ") + "]"
}

// cleanse returns the text shown for a value: the value itself, or the
// substitute when hidden is set.
func cleanse(hidden bool, value string) string {
	if hidden {
		return pyRepr(cleansedSubstitute)
	}
	return value
}

// formVars returns the rows of GET or POST data, sorted by name.
func formVars(values url.Values) []debugVar {
	vars := make([]debugVar, 0, len(values))
	for _, name := range slices.Sorted(maps.Keys(values)) {
		vars = append(vars, debugVar{name, cleanse(isHiddenName(name), reprValues(values[name]))})
	}
	return vars
}

// cookieVars returns the rows of the cookies, in the order of the request.
func (rep *debugReport) cookieVars() []debugVar {
	cookies := rep.request.Cookies()
	vars := make([]debugVar, 0, len(cookies))
	for _, cookie := range cookies {
		hidden := cookie.Name == rep.sessionCookieName || isHiddenName(cookie.Name)
		vars = append(vars, debugVar{cookie.Name, cleanse(hidden, pyRepr(cookie.Value))})
	}
	return vars
}

// headerVars returns the rows of the request headers, sorted by name. The Host
// header, which net/http keeps apart, is among them. Whether a header is
// sensitive is decided on its name in Django's META spelling, which makes the
// Cookie header (HTTP_COOKIE) one of them.
func (rep *debugReport) headerVars() []debugVar {
	r := rep.request
	headers := make(map[string]string, len(r.Header)+1)
	for name, values := range r.Header {
		headers[name] = strings.Join(values, ", ")
	}
	if r.Host != "" {
		headers["Host"] = r.Host
	}

	vars := make([]debugVar, 0, len(headers))
	for _, name := range slices.Sorted(maps.Keys(headers)) {
		meta := "HTTP_" + strings.ToUpper(strings.ReplaceAll(name, "-", "_"))
		vars = append(vars, debugVar{name, cleanse(isHiddenName(meta), pyRepr(headers[name]))})
	}
	return vars
}

// requestURL returns the URL of the request for the page: the scheme, the host
// as the client sent it, and the request target. Nothing in it is validated.
// The values of sensitive query parameters are replaced.
func requestURL(r *http.Request) string {
	target := r.RequestURI
	if target == "" && r.URL != nil {
		// A request that was not read from a connection has no RequestURI.
		target = r.URL.RequestURI()
	}
	if path, query, found := strings.Cut(target, "?"); found {
		target = path + "?" + cleanseRawQuery(query)
	}
	if !strings.HasPrefix(target, "/") {
		// The target of a proxy request is a URL of its own.
		return target
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host + target
}

// cleanseRawQuery replaces the values of the sensitive parameters of a raw
// query string and leaves everything else as it was sent.
func cleanseRawQuery(query string) string {
	params := strings.Split(query, "&")
	for i, param := range params {
		key, _, _ := strings.Cut(param, "=")
		name, err := url.QueryUnescape(key)
		if err != nil {
			name = key
		}
		if isHiddenName(name) {
			params[i] = key + "=" + cleansedSubstitute
		}
	}
	return strings.Join(params, "&")
}

// renderDebugPage renders the technical 500 page, the counterpart of Django's
// technical_500.html. Every value that comes from the request or the panic is
// HTML-escaped.
func renderDebugPage(rep *debugReport) string {
	r, p := rep.request, rep.panic
	path := ""
	var query url.Values
	if r.URL != nil {
		path, query = r.URL.Path, r.URL.Query()
	}
	heading := escapeHTML(p.typ + " at " + path)
	value := escapeHTML(p.value)

	var b strings.Builder
	b.WriteString(debugPageHead)
	b.WriteString("  <title>" + heading + "</title>\n")
	b.WriteString(debugPageStyle)
	b.WriteString("</head>\n<body>\n<header id=\"summary\">\n")
	b.WriteString("  <h1>" + heading + "</h1>\n")
	if value != "" {
		b.WriteString(`  <pre class="exception_value">` + value + "</pre>\n")
	} else {
		b.WriteString(`  <pre class="exception_value">No exception message supplied</pre>` + "\n")
	}

	b.WriteString("  <table class=\"meta\">\n")
	writeMetaRow(&b, "Request Method:", escapeHTML(r.Method))
	writeMetaRow(&b, "Request URL:", escapeHTML(requestURL(r)))
	writeMetaRow(&b, "Exception Type:", escapeHTML(p.typ))
	if value != "" {
		writeMetaRow(&b, "Exception Value:", "<pre>"+value+"</pre>")
	}
	if p.function != "" {
		writeMetaRow(&b, "Exception Location:", `<span class="fname">`+escapeHTML(p.file)+
			"</span>, line "+strconv.Itoa(p.line)+", in "+escapeHTML(p.function))
	}
	writeMetaRow(&b, "Go Version:", escapeHTML(runtime.Version()))
	writeMetaRow(&b, "Server time:", escapeHTML(rep.now.Format(time.RFC1123Z)))
	b.WriteString("  </table>\n</header>\n\n<main id=\"info\">\n")

	b.WriteString("<div id=\"traceback\">\n")
	b.WriteString("  <h2>Traceback <span class=\"commands\">(most recent call first)</span></h2>\n")
	b.WriteString("  <pre>" + escapeHTML("panic: "+p.typ+": "+p.value+"\n\n"+string(p.stack)) + "</pre>\n")
	b.WriteString("</div>\n\n")

	b.WriteString("<div id=\"requestinfo\">\n  <h2>Request information</h2>\n\n")
	writeVarsSection(&b, "get-info", "GET", "No GET data", formVars(query))
	writeVarsSection(&b, "post-info", "POST", "No POST data", formVars(rep.postData()))
	writeVarsSection(&b, "cookie-info", "COOKIES", "No cookie data", rep.cookieVars())
	writeVarsSection(&b, "meta-info", "Request headers", "No request headers", rep.headerVars())
	b.WriteString("</div>\n</main>\n\n")

	b.WriteString(debugPageFoot)
	return b.String()
}

// writeMetaRow writes one row of the summary table. value is HTML.
func writeMetaRow(b *strings.Builder, label, value string) {
	b.WriteString("    <tr><th scope=\"row\">" + label + "</th><td>" + value + "</td></tr>\n")
}

// writeVarsSection writes one section of the request information: its heading
// and the table of its rows, or the text for an empty section.
func writeVarsSection(b *strings.Builder, id, title, empty string, vars []debugVar) {
	b.WriteString("  <h3 id=\"" + id + "\">" + title + "</h3>\n")
	if len(vars) == 0 {
		b.WriteString("  <p>" + empty + "</p>\n\n")
		return
	}
	b.WriteString(debugTableHead)
	for _, v := range vars {
		b.WriteString("      <tr><td>" + escapeHTML(v.name) + `</td><td class="code"><pre>` +
			escapeHTML(v.value) + "</pre></td></tr>\n")
	}
	b.WriteString("    </tbody>\n  </table>\n\n")
}

// debugPageHead opens the technical 500 page, up to the title.
const debugPageHead = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta http-equiv="content-type" content="text/html; charset=utf-8">
  <meta name="color-scheme" content="light dark">
  <meta name="robots" content="NONE,NOARCHIVE">
`

// debugPageStyle is the style sheet of the page, the rules of Django's
// technical_500.html for the elements this page has.
const debugPageStyle = `  <style>
    html * { padding:0; margin:0; }
    body * { padding:10px 20px; }
    body * * { padding:0; }
    body { font-family: sans-serif; background-color: light-dark(#fff, #121212); color: light-dark(#000, #e0e0e0); }
    body > header, body > main, body > footer { border-bottom:1px solid light-dark(#ddd, #444); }
    h1 { font-weight:normal; }
    h2 { margin-bottom:.8em; }
    h3 { margin:1em 0 .5em 0; }
    code, pre { font-size: 100%; white-space: pre-wrap; word-break: break-word; }
    table { border:1px solid light-dark(#ccc, #444); border-collapse: collapse; width:100%; background: light-dark(white, #1e1e1e); }
    tbody td, tbody th { vertical-align:top; padding:2px 3px; }
    thead th {
      padding:1px 6px 1px 3px; background: light-dark(#fefefe, #2a2a2a); text-align:left;
      font-weight:normal; font-size: 0.6875rem; border:1px solid light-dark(#ddd, #444);
    }
    tbody th { width:12em; text-align:right; color: light-dark(#666, #aaa); padding-right:.5em; }
    table.req td { font-family:monospace; }
    table td.code { width:100%; }
    table td.code pre { overflow:hidden; }
    #summary { background: light-dark(#ffc, #2b2b2b); }
    #info { padding: 0; }
    #info > * { padding:10px 20px; }
    #explanation { background: light-dark(#eee, #1f1f1f); }
    #traceback { background: light-dark(#eee, #121212); }
    #traceback pre { font-family:monospace; }
    #requestinfo { background: light-dark(#f6f6f6, #1e1e1e); padding-left:120px; }
    #summary table { border:none; background:transparent; }
    #requestinfo h2, #requestinfo h3 { position:relative; margin-left:-100px; }
    #requestinfo h3 { margin-bottom:-1em; }
    h2 span.commands { font-size: 0.7rem; font-weight:normal; }
    pre.exception_value { font-family: sans-serif; color: light-dark(#575757, #e0e0e0); font-size: 1.5rem; margin: 10px 0 10px 0; }
    .fname { user-select: all; }
    code { color: light-dark(#000, #e0e0e0); }
  </style>
`

// debugTableHead opens a table of the request information.
const debugTableHead = `  <table class="req">
    <thead>
      <tr>
        <th scope="col">Variable</th>
        <th scope="col">Value</th>
      </tr>
    </thead>
    <tbody>
`

// debugPageFoot closes the page with Django's explanation of why it is shown,
// in the terms of the Debug option.
const debugPageFoot = `<footer id="explanation">
  <p>
    You’re seeing this error because you have <code>Debug: true</code> in your
    Djan-Go-Go settings. Change that to <code>false</code>, and Djan-Go-Go will
    display a standard page generated by the handler for this status code.
  </p>
</footer>
</body>
</html>
`
