package djangotest

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/oliverhaas/djangogo/templates"
)

// Redirect is one hop of a followed redirect chain: the URL the client was sent
// to and the status code that sent it there.
type Redirect struct {
	URL        string
	StatusCode int
}

// Response is what a Client request returns: the handler's response plus the
// test-only annotations Django's client adds (the request, the templates and
// contexts rendered while serving it, and the redirect chain).
type Response struct {
	// StatusCode is the HTTP status of the (final) response.
	StatusCode int
	// Header holds the response headers.
	Header http.Header
	// Body is the raw response body.
	Body []byte
	// Request is the request that produced this response.
	Request *http.Request
	// Templates lists the request-bound template renders, in render order, each
	// with the template name and the context it was rendered with.
	Templates []templates.Rendered
	// RedirectChain lists the redirects that were followed to reach this
	// response. It is nil unless the request was made with Follow, and empty
	// (but non-nil) when Follow was given and nothing redirected.
	RedirectChain []Redirect

	client *Client
	// headers are the per-request headers the response was requested with, which
	// AssertRedirects repeats when it fetches the redirect target.
	headers http.Header
}

// Content returns the response body as a string.
func (r *Response) Content() string { return string(r.Body) }

// URL returns the Location header, the target of a redirect response (Django's
// response.url). It is empty for a response that does not redirect.
func (r *Response) URL() string { return r.Header.Get("Location") }

// Client returns the client that produced the response.
func (r *Response) Client() *Client { return r.client }

// TemplateNames returns the names of the rendered templates, in render order.
func (r *Response) TemplateNames() []string {
	names := make([]string, len(r.Templates))
	for i, t := range r.Templates {
		names[i] = t.Name
	}
	return names
}

// Context returns the value stored under key in the context of a rendered
// template. Like Django's response.context[key] it searches every rendered
// context, most recent first, and reports whether the key was found.
func (r *Response) Context(key string) (any, bool) {
	for i := len(r.Templates) - 1; i >= 0; i-- {
		if v, ok := r.Templates[i].Context[key]; ok {
			return v, true
		}
	}
	return nil, false
}

// JSON decodes the response body into v. Like Django's response.json() it
// refuses a response whose Content-Type is not JSON.
func (r *Response) JSON(v any) error {
	if ct := r.Header.Get("Content-Type"); !isJSONContentType(ct) {
		return fmt.Errorf("djangotest: Content-Type header is %q, not %q", ct, JSONContentType)
	}
	if err := json.Unmarshal(r.Body, v); err != nil {
		return fmt.Errorf("djangotest: decode JSON response: %w", err)
	}
	return nil
}

// Cookies returns the cookies set by this response.
func (r *Response) Cookies() []*http.Cookie {
	return (&http.Response{Header: r.Header}).Cookies()
}
