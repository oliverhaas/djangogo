package djangotest

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"sort"
	"strings"
)

// TB is the part of testing.TB the assertions use. *testing.T and *testing.B
// satisfy it. An assertion that fails reports through Errorf and lets the test
// continue, so one run shows every failed assertion.
type TB interface {
	Helper()
	Errorf(format string, args ...any)
	Fatalf(format string, args ...any)
}

// assertConfig collects the optional arguments of the response assertions.
type assertConfig struct {
	statusCode       int
	hasStatusCode    bool
	targetStatusCode int
	count            int
	hasCount         bool
	html             bool
	fetch            bool
	msgPrefix        string
}

// AssertOption is an optional argument of the assertions that take keyword
// arguments in Django (AssertContains, AssertRedirects, AssertTemplateUsed,
// AssertInHTML, AssertURLEqual, and their negations). The options carry the
// names of those keyword arguments. The failure messages are Django's, so a
// test of an assertion ports unchanged.
type AssertOption func(*assertConfig)

// StatusCode sets the status code the response must have (Django's
// status_code): 200 by default for AssertContains and AssertNotContains, 302
// by default for AssertRedirects.
func StatusCode(code int) AssertOption {
	return func(c *assertConfig) { c.statusCode, c.hasStatusCode = code, true }
}

// TargetStatusCode sets the status code the redirect target must answer with
// (Django's target_status_code, default 200).
func TargetStatusCode(code int) AssertOption {
	return func(c *assertConfig) { c.targetStatusCode = code }
}

// Count requires the text (or, for AssertTemplateUsed, the template) to occur
// exactly n times (Django's count). Without it one occurrence is enough.
func Count(n int) AssertOption {
	return func(c *assertConfig) { c.count, c.hasCount = n, true }
}

// HTML compares the text and the response as HTML rather than as plain
// substrings (Django's html=True); see AssertHTMLEqual for the rules.
func HTML() AssertOption {
	return func(c *assertConfig) { c.html = true }
}

// FetchRedirectResponse controls whether AssertRedirects requests the redirect
// target to check its status (Django's fetch_redirect_response, default true).
func FetchRedirectResponse(fetch bool) AssertOption {
	return func(c *assertConfig) { c.fetch = fetch }
}

// MsgPrefix prefixes the failure message (Django's msg_prefix).
func MsgPrefix(prefix string) AssertOption {
	return func(c *assertConfig) { c.msgPrefix = prefix }
}

// newAssertConfig applies opts over the defaults.
func newAssertConfig(defaultStatus int, opts []AssertOption) *assertConfig {
	c := &assertConfig{statusCode: defaultStatus, targetStatusCode: http.StatusOK, fetch: true}
	for _, opt := range opts {
		opt(c)
	}
	if c.msgPrefix != "" {
		c.msgPrefix += ": "
	}
	return c
}

// AssertEqual asserts that got and want are deeply equal (Django's
// assertEqual).
func AssertEqual[T any](t TB, got, want T) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v, want %#v", got, want)
	}
}

// AssertNotEqual asserts that got and want are not deeply equal (Django's
// assertNotEqual).
func AssertNotEqual[T any](t TB, got, want T) {
	t.Helper()
	if reflect.DeepEqual(got, want) {
		t.Errorf("got %#v, which must differ", got)
	}
}

// AssertErrorIs asserts that err matches target under errors.Is, the
// counterpart of assertRaises with an exception class.
func AssertErrorIs(t TB, err, target error) {
	t.Helper()
	if !errors.Is(err, target) {
		t.Errorf("got error %v, want %v", err, target)
	}
}

// AssertStatus asserts the status code of a response (Django's
// assertEqual(response.status_code, code)).
func AssertStatus(t TB, resp *Response, code int) {
	t.Helper()
	if resp.StatusCode != code {
		t.Errorf("response status was %d (expected %d)", resp.StatusCode, code)
	}
}

// countIn checks the response status and returns how often text occurs in the
// body, as plain text or (with HTML) as an HTML fragment.
func countIn(t TB, resp *Response, text string, c *assertConfig) (n int, ok bool) {
	t.Helper()
	if resp.StatusCode != c.statusCode {
		t.Errorf("%sCouldn't retrieve content: Response code was %d (expected %d)", c.msgPrefix, resp.StatusCode, c.statusCode)
		return 0, false
	}
	if !c.html {
		return strings.Count(resp.Content(), text), true
	}
	content, err := parseHTML(resp.Content())
	if err != nil {
		t.Errorf("Response's content is not valid HTML:\n%v", err)
		return 0, false
	}
	needle, err := parseHTML(text)
	if err != nil {
		t.Errorf("Second argument is not valid HTML:\n%v", err)
		return 0, false
	}
	return content.count(needle), true
}

// AssertContains asserts that the response has the expected status (200 unless
// StatusCode is given) and that text occurs in its body: at least once, or
// exactly Count times (Django's assertContains).
func AssertContains(t TB, resp *Response, text string, opts ...AssertOption) {
	t.Helper()
	c := newAssertConfig(http.StatusOK, opts)
	n, ok := countIn(t, resp, text, c)
	if !ok {
		return
	}
	switch {
	case c.hasCount && n != c.count:
		t.Errorf("%sFound %d instances of '%s' (expected %d) in the following response\n%s", c.msgPrefix, n, text, c.count, resp.Content())
	case !c.hasCount && n == 0:
		t.Errorf("%sCouldn't find '%s' in the following response\n%s", c.msgPrefix, text, resp.Content())
	}
}

// AssertNotContains asserts that the response has the expected status (200
// unless StatusCode is given) and that text does not occur in its body
// (Django's assertNotContains).
func AssertNotContains(t TB, resp *Response, text string, opts ...AssertOption) {
	t.Helper()
	c := newAssertConfig(http.StatusOK, opts)
	n, ok := countIn(t, resp, text, c)
	if ok && n != 0 {
		t.Errorf("%s'%s' unexpectedly found in the following response\n%s", c.msgPrefix, text, resp.Content())
	}
}

// AssertRedirects asserts that the response redirected to expectedURL and that
// the target can be loaded (Django's assertRedirects). For a response fetched
// with Follow it checks the first hop's status, the final status, and the last
// URL of the chain. Otherwise it checks the response status (302 unless
// StatusCode is given) and, unless FetchRedirectResponse(false) is given,
// requests the target with the same client and the headers of the original
// request, and checks its status (200 unless TargetStatusCode is given). URLs
// are compared as AssertURLEqual does.
//
// The target is only requested when it is served by the handler under test: a
// location without a host, the client's own host, or a host named with
// WithAllowedHosts. Any other host stops the test, because the client cannot
// fetch remote URLs.
func AssertRedirects(t TB, resp *Response, expectedURL string, opts ...AssertOption) {
	t.Helper()
	c := newAssertConfig(http.StatusFound, opts)

	var got string
	if resp.RedirectChain != nil {
		// The request was a followed redirect.
		if len(resp.RedirectChain) == 0 {
			t.Errorf("%sResponse didn't redirect as expected: Response code was %d (expected %d)", c.msgPrefix, resp.StatusCode, c.statusCode)
			return
		}
		if first := resp.RedirectChain[0].StatusCode; first != c.statusCode {
			t.Errorf("%sInitial response didn't redirect as expected: Response code was %d (expected %d)", c.msgPrefix, first, c.statusCode)
		}
		got = resp.RedirectChain[len(resp.RedirectChain)-1].URL
		if resp.StatusCode != c.targetStatusCode {
			t.Errorf("%sResponse didn't redirect as expected: Final Response code was %d (expected %d)", c.msgPrefix, resp.StatusCode, c.targetStatusCode)
		}
	} else {
		// Not a followed redirect.
		if resp.StatusCode != c.statusCode {
			t.Errorf("%sResponse didn't redirect as expected: Response code was %d (expected %d)", c.msgPrefix, resp.StatusCode, c.statusCode)
			return
		}
		got = resp.URL()
		location, err := url.Parse(got)
		if err != nil {
			t.Errorf("%sRedirect location '%s' is not a valid URL: %v", c.msgPrefix, got, err)
			return
		}
		target, err := resolveLocation(resp.Request, got)
		if err != nil {
			t.Errorf("%sRedirect location '%s' is not a valid URL: %v", c.msgPrefix, got, err)
			return
		}
		// Prepend the request path to handle relative path redirects.
		if location.Host == "" && !strings.HasPrefix(location.Path, "/") {
			got = target.RequestURI()
			if target.Fragment != "" {
				got += "#" + target.EscapedFragment()
			}
		}
		if c.fetch {
			if resp.client == nil {
				t.Fatalf("AssertRedirects needs a response returned by a Client to fetch the redirect target. " +
					"Use AssertRedirects(..., FetchRedirectResponse(false)) for a response built by hand.")
				return
			}
			if host := location.Hostname(); host != "" && !resp.client.servesHost(host) {
				t.Fatalf("The test client is unable to fetch remote URLs (got %s). "+
					"If the host is served by the handler under test, build the client with WithAllowedHosts(%q). "+
					"Otherwise, use AssertRedirects(..., FetchRedirectResponse(false)).", got, host)
				return
			}
			fetched := resp.client.fetchRedirectTarget(resp, target)
			if fetched.StatusCode != c.targetStatusCode {
				t.Errorf("%sCouldn't retrieve redirection page '%s': response code was %d (expected %d)", c.msgPrefix, target.Path, fetched.StatusCode, c.targetStatusCode)
			}
		}
	}
	if !urlsEqual(got, expectedURL) {
		t.Errorf("%sResponse redirected to '%s', expected '%s'", c.msgPrefix, got, expectedURL)
	}
}

// hostname returns the client's host without a port.
func (c *Client) hostname() string {
	return (&url.URL{Host: c.host}).Hostname()
}

// servesHost reports whether host is one the handler under test answers for:
// the client's own host or one named with WithAllowedHosts.
func (c *Client) servesHost(host string) bool {
	return host == c.hostname() || c.allowedHosts[host]
}

// fetchRedirectTarget requests the target of the redirect resp, as Django's
// assertRedirects does: a GET of the target's path and query on the client's
// own host, over HTTPS when the target says so, with the headers of the
// request that was redirected.
func (c *Client) fetchRedirectTarget(resp *Response, target *url.URL) *Response {
	c.t.Helper()
	rc := &requestConfig{headers: withoutHost(resp.headers), query: make(url.Values), secure: target.Scheme == "https"}
	return c.send(http.MethodGet, target.RequestURI(), nil, "", rc)
}

// AssertURLEqual asserts that two URLs are the same, ignoring how their
// query-string parameters are percent-encoded and the order of the parameters,
// except for parameters with the same name (Django's assertURLEqual):
// /path/?x=1&y=2 equals /path/?y=2&x=1, but /path/?a=1&a=2 does not equal
// /path/?a=2&a=1. A parameter with a blank value counts as absent. MsgPrefix is
// the only option that applies.
//
// Django documents the rule for parameters with the same name but does not
// enforce it, because it sorts the name/value pairs; this function enforces it.
func AssertURLEqual(t TB, got, want string, opts ...AssertOption) {
	t.Helper()
	c := newAssertConfig(0, opts)
	if !urlsEqual(got, want) {
		t.Errorf("%sExpected '%s' to equal '%s'.", c.msgPrefix, got, want)
	}
}

// urlsEqual compares two URLs after normalizing their query strings.
func urlsEqual(a, b string) bool {
	return normalizeURL(a) == normalizeURL(b)
}

// normalizeURL returns a canonical form of raw: the query string is decoded
// into name/value pairs, pairs with a blank value are dropped (as Python's
// parse_qsl drops them), the pairs are sorted by name with the values of one
// name kept in their order, and they are encoded again.
func normalizeURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	var pairs [][2]string
	for _, part := range strings.Split(u.RawQuery, "&") {
		if part == "" {
			continue
		}
		name, value, _ := strings.Cut(part, "=")
		if decoded, derr := url.QueryUnescape(name); derr == nil {
			name = decoded
		}
		if decoded, derr := url.QueryUnescape(value); derr == nil {
			value = decoded
		}
		if value == "" {
			continue
		}
		pairs = append(pairs, [2]string{name, value})
	}
	sort.SliceStable(pairs, func(i, j int) bool { return pairs[i][0] < pairs[j][0] })
	encoded := make([]string, len(pairs))
	for i, p := range pairs {
		encoded[i] = url.QueryEscape(p[0]) + "=" + url.QueryEscape(p[1])
	}
	return fmt.Sprintf("%s://%s%s?%s#%s", u.Scheme, u.Host, u.EscapedPath(), strings.Join(encoded, "&"), u.EscapedFragment())
}

// AssertTemplateUsed asserts that the template with the given name was rendered
// while serving the response (Django's assertTemplateUsed). With Count it must
// have been rendered exactly that many times. MsgPrefix also applies.
func AssertTemplateUsed(t TB, resp *Response, name string, opts ...AssertOption) {
	t.Helper()
	c := newAssertConfig(0, opts)
	names := resp.TemplateNames()
	if len(names) == 0 {
		t.Errorf("%sNo templates used to render the response", c.msgPrefix)
		return
	}
	used := 0
	for _, n := range names {
		if n == name {
			used++
		}
	}
	if used == 0 {
		t.Errorf("%sTemplate '%s' was not a template used to render the response. Actual template(s) used: %s", c.msgPrefix, name, strings.Join(names, ", "))
		return
	}
	if c.hasCount && used != c.count {
		t.Errorf("%sTemplate '%s' was expected to be rendered %d time(s) but was actually rendered %d time(s).", c.msgPrefix, name, c.count, used)
	}
}

// AssertTemplateNotUsed asserts that the template with the given name was not
// rendered while serving the response (Django's assertTemplateNotUsed).
// MsgPrefix is the only option that applies.
func AssertTemplateNotUsed(t TB, resp *Response, name string, opts ...AssertOption) {
	t.Helper()
	c := newAssertConfig(0, opts)
	for _, n := range resp.TemplateNames() {
		if n == name {
			t.Errorf("%sTemplate '%s' was used unexpectedly in rendering the response", c.msgPrefix, name)
			return
		}
	}
}

// AssertJSONEqual asserts that the JSON document raw equals expected (Django's
// assertJSONEqual). expected is either a JSON string or a Go value, which is
// compared by its JSON encoding. Key order and whitespace do not matter.
func AssertJSONEqual(t TB, raw string, expected any) {
	t.Helper()
	got, want, ok := decodeJSONPair(t, raw, expected)
	if ok && !reflect.DeepEqual(got, want) {
		t.Errorf("JSON differs.\n--- got ---\n%s\n--- want ---\n%s", canonicalJSON(got), canonicalJSON(want))
	}
}

// AssertJSONNotEqual asserts that the JSON document raw does not equal expected
// (Django's assertJSONNotEqual).
func AssertJSONNotEqual(t TB, raw string, expected any) {
	t.Helper()
	got, want, ok := decodeJSONPair(t, raw, expected)
	if ok && reflect.DeepEqual(got, want) {
		t.Errorf("JSON is unexpectedly equal:\n%s", canonicalJSON(got))
	}
}

// decodeJSONPair decodes raw and expected into comparable generic values,
// reporting invalid JSON on t.
func decodeJSONPair(t TB, raw string, expected any) (got, want any, ok bool) {
	t.Helper()
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		t.Errorf("First argument is not valid JSON: '%s' (%v)", raw, err)
		return nil, nil, false
	}
	expectedJSON, isString := expected.(string)
	if !isString {
		b, err := json.Marshal(expected)
		if err != nil {
			t.Errorf("Second argument cannot be encoded as JSON: %v", err)
			return nil, nil, false
		}
		expectedJSON = string(b)
	}
	if err := json.Unmarshal([]byte(expectedJSON), &want); err != nil {
		t.Errorf("Second argument is not valid JSON: '%s' (%v)", expectedJSON, err)
		return nil, nil, false
	}
	return got, want, true
}

// canonicalJSON renders a decoded JSON value with sorted keys and indentation.
func canonicalJSON(v any) string {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Sprint(v)
	}
	return string(b)
}
