package djangotest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/oliverhaas/djangogo/auth"
	"github.com/oliverhaas/djangogo/csrf"
	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/sessions"
	"github.com/oliverhaas/djangogo/templates"
)

// DefaultHost is the Host header the client sends, the same name Django's test
// client uses.
const DefaultHost = "testserver"

// DefaultSessionCookie is the session cookie name the client assumes unless
// WithSessions names another one. It matches Django's SESSION_COOKIE_NAME.
const DefaultSessionCookie = "sessionid"

// maxRedirects is the longest redirect chain Follow accepts, the value Django's
// client takes from Firefox's network.http.redirection-limit.
const maxRedirects = 20

// Content types the client knows how to encode a request body for.
const (
	// FormContentType is the default content type of Post bodies.
	FormContentType = "application/x-www-form-urlencoded"
	// JSONContentType makes the client JSON-encode a non-string body.
	JSONContentType = "application/json"
	// octetStream is the default content type of Put, Patch, and Delete bodies.
	octetStream = "application/octet-stream"
)

// Client is a stateful test client that drives an http.Handler in process, the
// counterpart of django.test.Client. It keeps cookies (and therefore the
// session) between requests, can log a user in without going through a login
// view, and returns responses annotated with the templates and contexts that
// were rendered while serving them.
//
// A Client is bound to one test: harness failures (an unencodable body, a
// redirect loop) stop the test through t.Fatalf. Like Django's client it skips
// CSRF checks unless built with WithCSRFChecks. A panic in the handler is not
// recovered and fails the test, which matches Django's default of re-raising
// view exceptions.
type Client struct {
	t             TB
	handler       http.Handler
	host          string
	enforceCSRF   bool
	headers       http.Header
	cookies       map[string]*http.Cookie
	db            *orm.DB
	store         sessions.Store
	sessionCookie string
	allowedHosts  map[string]bool
}

// ClientOption configures a Client built by NewClient.
type ClientOption func(*Client)

// WithCSRFChecks makes the client subject to CSRF checks, like Django's
// Client(enforce_csrf_checks=True). By default the checks are skipped.
func WithCSRFChecks() ClientOption {
	return func(c *Client) { c.enforceCSRF = true }
}

// WithSessions tells the client which session store and cookie name the handler
// under test uses, which Login, ForceLogin, Logout, Session, and SaveSession
// need. An empty cookieName selects DefaultSessionCookie.
func WithSessions(store sessions.Store, cookieName string) ClientOption {
	return func(c *Client) {
		c.store = store
		if cookieName != "" {
			c.sessionCookie = cookieName
		}
	}
}

// WithDB gives the client the database Login uses to look up and verify users.
func WithDB(db *orm.DB) ClientOption {
	return func(c *Client) { c.db = db }
}

// WithHost overrides the Host header the client sends (DefaultHost otherwise).
func WithHost(host string) ClientOption {
	return func(c *Client) { c.host = host }
}

// WithAllowedHosts names further hosts the handler under test answers for, the
// counterpart of adding them to Django's ALLOWED_HOSTS: AssertRedirects fetches
// a redirect target on such a host instead of refusing it as a remote URL.
func WithAllowedHosts(hosts ...string) ClientOption {
	return func(c *Client) {
		for _, h := range hosts {
			c.allowedHosts[h] = true
		}
	}
}

// WithHeader adds a header sent with every request, like the headers argument
// of Django's Client constructor.
func WithHeader(key, value string) ClientOption {
	return func(c *Client) { c.headers.Add(key, value) }
}

// NewClient returns a Client that sends its requests to handler.
func NewClient(t TB, handler http.Handler, opts ...ClientOption) *Client {
	t.Helper()
	c := &Client{
		t:             t,
		handler:       handler,
		host:          DefaultHost,
		headers:       make(http.Header),
		cookies:       make(map[string]*http.Cookie),
		sessionCookie: DefaultSessionCookie,
		allowedHosts:  make(map[string]bool),
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// requestConfig collects the per-request options.
type requestConfig struct {
	follow      bool
	secure      bool
	headers     http.Header
	query       url.Values
	contentType string
}

// RequestOption adjusts a single request.
type RequestOption func(*requestConfig)

// Follow makes the client follow redirects and return the final response, with
// the intermediate hops in Response.RedirectChain (Django's follow=True).
func Follow() RequestOption {
	return func(rc *requestConfig) { rc.follow = true }
}

// Secure sends the request as HTTPS (Django's secure=True).
func Secure() RequestOption {
	return func(rc *requestConfig) { rc.secure = true }
}

// Header adds a request header (Django's headers argument). A Host header
// sets the host the request is addressed to, as HTTP_HOST does in Django.
func Header(key, value string) RequestOption {
	return func(rc *requestConfig) { rc.headers.Add(key, value) }
}

// Query appends query-string parameters to the path (Django's query_params
// argument, and the data argument of get and head).
func Query(values url.Values) RequestOption {
	return func(rc *requestConfig) {
		for k, vs := range values {
			for _, v := range vs {
				rc.query.Add(k, v)
			}
		}
	}
}

// ContentType sets the Content-Type of the request body (Django's content_type
// argument). With JSONContentType a non-string body is JSON-encoded.
func ContentType(contentType string) RequestOption {
	return func(rc *requestConfig) { rc.contentType = contentType }
}

// newRequestConfig applies opts on top of an empty configuration.
func newRequestConfig(opts []RequestOption) *requestConfig {
	rc := &requestConfig{headers: make(http.Header), query: make(url.Values)}
	for _, opt := range opts {
		opt(rc)
	}
	return rc
}

// Get sends a GET request.
func (c *Client) Get(path string, opts ...RequestOption) *Response {
	c.t.Helper()
	return c.do(http.MethodGet, path, nil, "", opts)
}

// Head sends a HEAD request.
func (c *Client) Head(path string, opts ...RequestOption) *Response {
	c.t.Helper()
	return c.do(http.MethodHead, path, nil, "", opts)
}

// Options sends an OPTIONS request.
func (c *Client) Options(path string, opts ...RequestOption) *Response {
	c.t.Helper()
	return c.do(http.MethodOptions, path, nil, "", opts)
}

// Trace sends a TRACE request, which carries no body.
func (c *Client) Trace(path string, opts ...RequestOption) *Response {
	c.t.Helper()
	return c.do(http.MethodTrace, path, nil, "", opts)
}

// Post sends a POST request. data is encoded as described on Request; the
// default content type is FormContentType. (Django defaults to multipart form
// data; file uploads are not supported by this client.)
func (c *Client) Post(path string, data any, opts ...RequestOption) *Response {
	c.t.Helper()
	return c.do(http.MethodPost, path, data, FormContentType, opts)
}

// Put sends a PUT request. The default content type is
// application/octet-stream, as in Django.
func (c *Client) Put(path string, data any, opts ...RequestOption) *Response {
	c.t.Helper()
	return c.do(http.MethodPut, path, data, octetStream, opts)
}

// Patch sends a PATCH request. The default content type is
// application/octet-stream, as in Django.
func (c *Client) Patch(path string, data any, opts ...RequestOption) *Response {
	c.t.Helper()
	return c.do(http.MethodPatch, path, data, octetStream, opts)
}

// Delete sends a DELETE request. The default content type is
// application/octet-stream, as in Django.
func (c *Client) Delete(path string, data any, opts ...RequestOption) *Response {
	c.t.Helper()
	return c.do(http.MethodDelete, path, data, octetStream, opts)
}

// Request sends a request with an arbitrary method (Django's generic). The body
// is built from data: nil sends no body; url.Values and map[string]string are
// form-encoded; string, []byte, and io.Reader are sent as they are; any other
// value is JSON-encoded when the content type is JSON and rejected otherwise.
// Use the ContentType option to choose the content type.
func (c *Client) Request(method, path string, data any, opts ...RequestOption) *Response {
	c.t.Helper()
	return c.do(method, path, data, octetStream, opts)
}

// do builds and sends one request, then follows redirects when asked to.
func (c *Client) do(method, path string, data any, defaultContentType string, opts []RequestOption) *Response {
	c.t.Helper()
	rc := newRequestConfig(opts)
	contentType := rc.contentType
	if contentType == "" {
		contentType = defaultContentType
	}
	body, err := encodeBody(data, contentType)
	if err != nil {
		c.t.Fatalf("djangotest: %s %s: %v", method, path, err)
	}
	if data == nil {
		contentType = ""
	}
	resp := c.send(method, path, body, contentType, rc)
	if rc.follow {
		resp = c.followRedirects(resp, body, contentType, rc)
	}
	return resp
}

// send performs exactly one request against the handler and records the
// response, the rendered templates, and any cookies the handler set.
func (c *Client) send(method, path string, body []byte, contentType string, rc *requestConfig) *Response {
	c.t.Helper()
	target, err := c.target(path, rc)
	if err != nil {
		c.t.Fatalf("djangotest: %s %s: %v", method, path, err)
	}
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req := httptest.NewRequest(method, target, reader)
	// A request parsed by a real server carries only the path and query in its
	// URL; the host lives in Host and the scheme is implied by TLS. Match that,
	// so handlers that echo r.URL behave as they do in production.
	req.URL.Scheme, req.URL.Host = "", ""
	req.RequestURI = req.URL.RequestURI()
	req.RemoteAddr = "127.0.0.1:12345"
	for k, vs := range c.headers {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	for k, vs := range rc.headers {
		req.Header[http.CanonicalHeaderKey(k)] = append([]string(nil), vs...)
	}
	if contentType != "" && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", contentType)
	}
	// net/http keeps the Host header in Request.Host, not in the header map.
	if host := req.Header.Get("Host"); host != "" {
		req.Host = host
		req.Header.Del("Host")
	}
	for _, name := range c.cookieNames() {
		req.AddCookie(&http.Cookie{Name: name, Value: c.cookies[name].Value})
	}

	recorder := &templates.Recorder{}
	ctx := templates.WithRecorder(req.Context(), recorder)
	if !c.enforceCSRF {
		ctx = csrf.WithoutEnforcement(ctx)
	}
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	c.handler.ServeHTTP(rec, req)

	result := rec.Result()
	defer func() { _ = result.Body.Close() }()
	c.storeCookies(result.Cookies())

	// The response to a HEAD request has no body: net/http discards what the
	// handler writes, and Django's test client does the same.
	content := rec.Body.Bytes()
	if method == http.MethodHead {
		content = nil
	}

	return &Response{
		StatusCode: rec.Code,
		Header:     result.Header,
		Body:       content,
		Request:    req,
		Templates:  recorder.Rendered(),
		client:     c,
		headers:    rc.headers,
	}
}

// target builds the absolute request URL for path, adding the request's query
// parameters and choosing the scheme. An absolute URL keeps its own host.
func (c *Client) target(path string, rc *requestConfig) (string, error) {
	u, err := url.Parse(path)
	if err != nil {
		return "", fmt.Errorf("parse path: %w", err)
	}
	if len(rc.query) > 0 {
		q := u.Query()
		for k, vs := range rc.query {
			for _, v := range vs {
				q.Add(k, v)
			}
		}
		u.RawQuery = q.Encode()
	}
	if u.Scheme == "" {
		u.Scheme = "http"
		if rc.secure {
			u.Scheme = "https"
		}
	}
	if u.Host == "" {
		u.Host = c.host
	}
	if u.Path == "" {
		u.Path = "/"
	}
	return u.String(), nil
}

// followRedirects follows the redirect chain starting at resp, mirroring
// Django's Client._handle_redirects: 301, 302, and 303 are followed with a GET;
// 307 and 308 repeat the original method and body.
func (c *Client) followRedirects(resp *Response, body []byte, contentType string, rc *requestConfig) *Response {
	c.t.Helper()
	chain := []Redirect{}
	for isRedirect(resp.StatusCode) {
		location := resp.URL()
		hop := Redirect{URL: location, StatusCode: resp.StatusCode}
		looped := slices.Contains(chain, hop)
		chain = append(chain, hop)

		next, err := resolveLocation(resp.Request, location)
		if err != nil {
			c.t.Fatalf("djangotest: follow redirect %q: %v", location, err)
		}
		// The location decides the host of the next hop, so a Host header given
		// for the first request does not travel along.
		hopConfig := &requestConfig{headers: withoutHost(rc.headers), query: make(url.Values), secure: next.Scheme == "https"}
		method, hopBody, hopType := http.MethodGet, []byte(nil), ""
		if resp.StatusCode == http.StatusTemporaryRedirect || resp.StatusCode == http.StatusPermanentRedirect {
			method, hopBody, hopType = resp.Request.Method, body, contentType
			// Django repeats a GET or HEAD with the query parameters of the
			// original request, replacing the query of the redirect location.
			if (method == http.MethodGet || method == http.MethodHead) && len(rc.query) > 0 {
				next.RawQuery = ""
				hopConfig.query = rc.query
			}
		}
		resp = c.send(method, next.String(), hopBody, hopType, hopConfig)
		// As in Django, the hop is followed before the chain is checked, and
		// the messages are those of its RedirectCycleError.
		if looped || len(chain) > maxRedirects {
			resp.RedirectChain = chain
			if looped {
				c.t.Fatalf("Redirect loop detected.")
			} else {
				c.t.Fatalf("Too many redirects.")
			}
			return resp
		}
	}
	resp.RedirectChain = chain
	return resp
}

// withoutHost returns a copy of headers without the Host header.
func withoutHost(headers http.Header) http.Header {
	out := headers.Clone()
	if out == nil {
		out = make(http.Header)
	}
	out.Del("Host")
	return out
}

// isRedirect reports whether status is one of the redirect codes the client
// follows.
func isRedirect(status int) bool {
	switch status {
	case http.StatusMovedPermanently, http.StatusFound, http.StatusSeeOther,
		http.StatusTemporaryRedirect, http.StatusPermanentRedirect:
		return true
	}
	return false
}

// resolveLocation resolves a Location header against the URL of the request
// that produced it, so relative redirects work.
func resolveLocation(req *http.Request, location string) (*url.URL, error) {
	u, err := url.Parse(location)
	if err != nil {
		return nil, err
	}
	if req == nil {
		// A response built by hand has no request to resolve against.
		return u, nil
	}
	base := *req.URL
	if base.Host == "" {
		base.Host = req.Host
	}
	if base.Scheme == "" {
		base.Scheme = "http"
		if req.TLS != nil {
			base.Scheme = "https"
		}
	}
	return base.ResolveReference(u), nil
}

// encodeBody turns the data argument of a request method into body bytes.
func encodeBody(data any, contentType string) ([]byte, error) {
	switch v := data.(type) {
	case nil:
		return nil, nil
	case url.Values:
		return []byte(v.Encode()), nil
	case map[string]string:
		values := make(url.Values, len(v))
		for k, val := range v {
			values.Set(k, val)
		}
		return []byte(values.Encode()), nil
	case string:
		return []byte(v), nil
	case []byte:
		return v, nil
	case io.Reader:
		return io.ReadAll(v)
	}
	if isJSONContentType(contentType) {
		b, err := json.Marshal(data)
		if err != nil {
			return nil, fmt.Errorf("encode JSON body: %w", err)
		}
		return b, nil
	}
	return nil, fmt.Errorf("cannot encode a %T body as %q; pass url.Values, a string, or use ContentType(JSONContentType)", data, contentType)
}

// isJSONContentType reports whether contentType is application/json or a
// structured-syntax JSON type such as application/problem+json.
func isJSONContentType(contentType string) bool {
	mediaType := strings.TrimSpace(strings.SplitN(contentType, ";", 2)[0])
	return mediaType == JSONContentType ||
		(strings.HasPrefix(mediaType, "application/") && strings.HasSuffix(mediaType, "+json"))
}

// Cookies returns the cookies the client currently holds, sorted by name.
func (c *Client) Cookies() []*http.Cookie {
	out := make([]*http.Cookie, 0, len(c.cookies))
	for _, name := range c.cookieNames() {
		out = append(out, c.cookies[name])
	}
	return out
}

// Cookie returns the named cookie held by the client, if any.
func (c *Client) Cookie(name string) (*http.Cookie, bool) {
	ck, ok := c.cookies[name]
	return ck, ok
}

// SetCookie stores a cookie that is sent with every following request.
func (c *Client) SetCookie(cookie *http.Cookie) {
	c.cookies[cookie.Name] = cookie
}

// cookieNames returns the names of the held cookies in sorted order, so the
// Cookie header is deterministic.
func (c *Client) cookieNames() []string {
	names := make([]string, 0, len(c.cookies))
	for name := range c.cookies {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// storeCookies applies the Set-Cookie headers of a response: a cookie that is
// expired or has a negative Max-Age is dropped, any other one is kept.
func (c *Client) storeCookies(cookies []*http.Cookie) {
	for _, ck := range cookies {
		expired := ck.MaxAge < 0 || (!ck.Expires.IsZero() && ck.Expires.Before(time.Now()))
		if expired {
			delete(c.cookies, ck.Name)
			continue
		}
		c.cookies[ck.Name] = ck
	}
}

// Login authenticates the credentials and, on success, logs the user in by
// writing a session and its cookie, exactly as if the user had gone through a
// login view. It reports whether the credentials were accepted. It needs
// WithDB and WithSessions.
func (c *Client) Login(username, password string) bool {
	c.t.Helper()
	if c.db == nil {
		c.t.Fatalf("djangotest: Client.Login needs a database; build the client with WithDB")
	}
	user, err := auth.Authenticate(context.Background(), c.db, username, password)
	if err != nil {
		return false
	}
	c.ForceLogin(user)
	return true
}

// ForceLogin logs user in without checking credentials (Django's force_login).
// It needs WithSessions.
func (c *Client) ForceLogin(user *auth.User) {
	c.t.Helper()
	sess := c.Session()
	auth.Login(sess, user)
	c.SaveSession(sess)
}

// Logout logs the user out by deleting the session from the store and dropping
// every cookie the client holds, as Django's client does.
func (c *Client) Logout() {
	c.t.Helper()
	store := c.requireStore("Logout")
	if ck, ok := c.cookies[c.sessionCookie]; ok {
		if sess, err := store.Decode(context.Background(), ck.Value); err == nil {
			_ = store.Delete(context.Background(), sess)
		}
	}
	c.cookies = make(map[string]*http.Cookie)
}

// Session returns the client's current session: the one named by its session
// cookie, or a new empty one. After changing it call SaveSession, which is what
// session.save() does in a Django test. It needs WithSessions.
func (c *Client) Session() *sessions.Session {
	c.t.Helper()
	store := c.requireStore("Session")
	if ck, ok := c.cookies[c.sessionCookie]; ok {
		sess, err := store.Decode(context.Background(), ck.Value)
		if err != nil {
			c.t.Fatalf("djangotest: decode session cookie: %v", err)
		}
		return sess
	}
	return store.New()
}

// SaveSession persists sess in the store and points the client's session cookie
// at it.
func (c *Client) SaveSession(sess *sessions.Session) {
	c.t.Helper()
	store := c.requireStore("SaveSession")
	value, err := store.Encode(context.Background(), sess)
	if err != nil {
		c.t.Fatalf("djangotest: save session: %v", err)
	}
	c.cookies[c.sessionCookie] = &http.Cookie{Name: c.sessionCookie, Value: value, Path: "/"}
}

// requireStore returns the configured session store or stops the test.
func (c *Client) requireStore(method string) sessions.Store {
	c.t.Helper()
	if c.store == nil {
		c.t.Fatalf("djangotest: Client.%s needs a session store; build the client with WithSessions", method)
	}
	return c.store
}
