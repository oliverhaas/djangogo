package djangotest_test

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"testing"

	"github.com/oliverhaas/djangogo/auth"
	"github.com/oliverhaas/djangogo/djangotest"
)

// The tests in this file are ports of Django's tests/test_client/tests.py. They
// run the Go test client against the Go port of the same views (see
// django_views_test.go). Tests of features this client does not have (file
// uploads, mail, signals, the resolver match, async) are not ported; the form
// tests are ported with the forms package.

// send issues a request through the client method named like the Django one,
// the counterpart of getattr(self.client, method)(path, data=data, ...). As in
// Django, GET and HEAD carry data in the query string; OPTIONS and TRACE carry
// none here.
func send(c *djangotest.Client, method, path string, data url.Values, opts ...djangotest.RequestOption) *djangotest.Response {
	switch method {
	case "get":
		return c.Get(path, append(opts, djangotest.Query(data))...)
	case "head":
		return c.Head(path, append(opts, djangotest.Query(data))...)
	case "options":
		return c.Options(path, opts...)
	case "trace":
		return c.Trace(path, opts...)
	case "post":
		return c.Post(path, data, opts...)
	case "put":
		return c.Put(path, data, opts...)
	case "patch":
		return c.Patch(path, data, opts...)
	case "delete":
		return c.Delete(path, data, opts...)
	}
	panic("send: unknown method " + method)
}

// contextValue returns response.context[key], failing the test when the key is
// missing.
func contextValue(t *testing.T, resp *djangotest.Response, key string) any {
	t.Helper()
	v, ok := resp.Context(key)
	if !ok {
		t.Fatalf("context has no %q; rendered templates: %v", key, resp.TemplateNames())
	}
	return v
}

// contextUsername returns response.context["user"].username.
func contextUsername(t *testing.T, resp *djangotest.Response) string {
	t.Helper()
	user, ok := contextValue(t, resp, "user").(*auth.User)
	if !ok || user == nil {
		t.Fatalf("context user is %#v, want a *auth.User", contextValue(t, resp, "user"))
	}
	return user.Username
}

// TestGetView GETs a view.
//
// Ported from Django: tests/test_client/tests.py::ClientTest.test_get_view
func TestGetView(t *testing.T) {
	c := newSite(t, false).client(t)
	// The data is ignored; the request must still not crash the system.
	resp := c.Get("/get_view/", djangotest.Query(url.Values{"var": {"ò"}}))

	djangotest.AssertContains(t, resp, "This is a test")
	djangotest.AssertEqual(t, contextValue(t, resp, "var"), any("ò"))
	djangotest.AssertEqual(t, resp.Templates[0].Name, "GET Template")
}

// Ported from Django: tests/test_client/tests.py::ClientTest.test_query_string_encoding
func TestQueryStringEncoding(t *testing.T) {
	c := newSite(t, false).client(t)
	resp := c.Get("/get_view/?var=1�")
	djangotest.AssertEqual(t, contextValue(t, resp, "var"), any("1�"))
}

// TestGetPostView GETs a view that normally expects POSTs.
//
// Ported from Django: tests/test_client/tests.py::ClientTest.test_get_post_view
func TestGetPostView(t *testing.T) {
	c := newSite(t, false).client(t)
	resp := c.Get("/post_view/")

	djangotest.AssertStatus(t, resp, 200)
	djangotest.AssertEqual(t, resp.Templates[0].Name, "Empty GET Template")
	djangotest.AssertTemplateUsed(t, resp, "Empty GET Template")
	djangotest.AssertTemplateNotUsed(t, resp, "Empty POST Template")
}

// TestEmptyPost POSTs an empty dictionary to a view.
//
// Ported from Django: tests/test_client/tests.py::ClientTest.test_empty_post
func TestEmptyPost(t *testing.T) {
	c := newSite(t, false).client(t)
	resp := c.Post("/post_view/", url.Values{})

	djangotest.AssertStatus(t, resp, 200)
	djangotest.AssertEqual(t, resp.Templates[0].Name, "Empty POST Template")
	djangotest.AssertTemplateNotUsed(t, resp, "Empty GET Template")
	djangotest.AssertTemplateUsed(t, resp, "Empty POST Template")
}

// TestPost POSTs some data to a view.
//
// Ported from Django: tests/test_client/tests.py::ClientTest.test_post
func TestPost(t *testing.T) {
	c := newSite(t, false).client(t)
	resp := c.Post("/post_view/", url.Values{"value": {"37"}})

	djangotest.AssertContains(t, resp, "Data received")
	djangotest.AssertEqual(t, contextValue(t, resp, "data"), any("37"))
	djangotest.AssertEqual(t, resp.Templates[0].Name, "POST Template")
}

// TestJSONSerialization checks that the test client serializes JSON data. The
// expected values are what encoding/json decodes them to: a JSON number
// arrives as a float64.
//
// Ported from Django: tests/test_client/tests.py::ClientTest.test_json_serialization
func TestJSONSerialization(t *testing.T) {
	c := newSite(t, false).client(t)
	cases := []struct {
		data     any
		expected any
	}{
		{map[string]int{"value": 37}, map[string]any{"value": float64(37)}},
		{[]any{37, true}, []any{float64(37), true}},
		{[2]any{37, false}, []any{float64(37), false}},
	}
	methods := map[string]func(string, any, ...djangotest.RequestOption) *djangotest.Response{
		"POST": c.Post, "PUT": c.Put, "PATCH": c.Patch, "DELETE": c.Delete,
	}
	for method, call := range methods {
		for i, tc := range cases {
			t.Run(method+"/"+strconv.Itoa(i), func(t *testing.T) {
				resp := call("/json_view/", tc.data, djangotest.ContentType(djangotest.JSONContentType))
				djangotest.AssertContains(t, resp, "Viewing "+method+" page.")
				djangotest.AssertEqual(t, contextValue(t, resp, "data"), tc.expected)
			})
		}
	}
}

// TestPut sends a PUT request. Django passes a dict, which its client sends as
// the dict's repr under the default content type; the port sends that string.
//
// Ported from Django: tests/test_client/tests.py::ClientTest.test_put
func TestPut(t *testing.T) {
	c := newSite(t, false).client(t)
	resp := c.Put("/put_view/", "{'foo': 'bar'}")

	djangotest.AssertStatus(t, resp, 200)
	djangotest.AssertEqual(t, resp.Templates[0].Name, "PUT Template")
	djangotest.AssertEqual(t, contextValue(t, resp, "data"), any("{'foo': 'bar'}"))
	djangotest.AssertEqual(t, contextValue(t, resp, "content_length"), any("14"))
}

// TestTrace TRACEs a view.
//
// Ported from Django: tests/test_client/tests.py::ClientTest.test_trace
func TestTrace(t *testing.T) {
	c := newSite(t, false).client(t)
	resp := c.Trace("/trace_view/")

	djangotest.AssertStatus(t, resp, 200)
	djangotest.AssertEqual(t, contextValue(t, resp, "method"), any("TRACE"))
	djangotest.AssertEqual(t, resp.Templates[0].Name, "TRACE Template")
}

// TestResponseHeaders checks the value of HTTP headers returned in a response.
//
// Ported from Django: tests/test_client/tests.py::ClientTest.test_response_headers
func TestResponseHeaders(t *testing.T) {
	c := newSite(t, false).client(t)
	resp := c.Get("/header_view/")
	djangotest.AssertEqual(t, resp.Header.Get("X-DJANGO-TEST"), "Slartibartfast")
}

// TestResponseAttachedRequest checks that the returned response carries the
// originating request. Django attaches a WSGI environ dict and a WSGIRequest;
// here it is the *http.Request the handler was called with.
//
// Ported from Django: tests/test_client/tests.py::ClientTest.test_response_attached_request
func TestResponseAttachedRequest(t *testing.T) {
	c := newSite(t, false).client(t)
	resp := c.Get("/header_view/")

	if resp.Request == nil {
		t.Fatal("response has no request")
	}
	djangotest.AssertEqual(t, resp.Request.Method, http.MethodGet)
	djangotest.AssertEqual(t, resp.Request.URL.Path, "/header_view/")
	djangotest.AssertEqual(t, resp.Request.Host, djangotest.DefaultHost)
	if resp.Client() != c {
		t.Error("response does not name the client that produced it")
	}
}

// TestRawPost POSTs raw data (with a content type) to a view.
//
// Ported from Django: tests/test_client/tests.py::ClientTest.test_raw_post
func TestRawPost(t *testing.T) {
	c := newSite(t, false).client(t)
	testDoc := `<?xml version="1.0" encoding="utf-8"?>
        <library><book><title>Blink</title><author>Malcolm Gladwell</author></book>
        </library>
        `
	resp := c.Post("/raw_post_view/", testDoc, djangotest.ContentType("text/xml"))

	djangotest.AssertStatus(t, resp, 200)
	djangotest.AssertEqual(t, resp.Templates[0].Name, "Book template")
	djangotest.AssertEqual(t, resp.Content(), "Blink - Malcolm Gladwell")
}

// TestInsecure GETs a URL through http. Django also checks the server port,
// which a Go handler has no counterpart for.
//
// Ported from Django: tests/test_client/tests.py::ClientTest.test_insecure
func TestInsecure(t *testing.T) {
	c := newSite(t, false).client(t)
	resp := c.Get("/secure_view/")
	djangotest.AssertEqual(t, resp.Header.Get(secureHeader), "false")
}

// TestSecure GETs a URL through https.
//
// Ported from Django: tests/test_client/tests.py::ClientTest.test_secure
func TestSecure(t *testing.T) {
	c := newSite(t, false).client(t)
	resp := c.Get("/secure_view/", djangotest.Secure())
	djangotest.AssertEqual(t, resp.Header.Get(secureHeader), "true")
}

// TestRedirect GETs a URL that redirects elsewhere.
//
// Ported from Django: tests/test_client/tests.py::ClientTest.test_redirect
func TestRedirect(t *testing.T) {
	c := newSite(t, false).client(t)
	resp := c.Get("/redirect_view/")
	djangotest.AssertRedirects(t, resp, "/get_view/")
}

// TestRedirectWithQuery GETs a URL that redirects with given GET parameters.
//
// Ported from Django: tests/test_client/tests.py::ClientTest.test_redirect_with_query
func TestRedirectWithQuery(t *testing.T) {
	c := newSite(t, false).client(t)
	resp := c.Get("/redirect_view/", djangotest.Query(url.Values{"var": {"value"}}))
	djangotest.AssertRedirects(t, resp, "/get_view/?var=value")
}

// TestRedirectWithQueryOrdering checks that AssertRedirects ignores the order
// of query string parameters.
//
// Ported from Django: tests/test_client/tests.py::ClientTest.test_redirect_with_query_ordering
func TestRedirectWithQueryOrdering(t *testing.T) {
	c := newSite(t, false).client(t)
	resp := c.Get("/redirect_view/", djangotest.Query(url.Values{"var": {"value"}, "foo": {"bar"}}))
	djangotest.AssertRedirects(t, resp, "/get_view/?var=value&foo=bar")
	djangotest.AssertRedirects(t, resp, "/get_view/?foo=bar&var=value")
}

// TestPermanentRedirect GETs a URL that redirects permanently elsewhere.
//
// Ported from Django: tests/test_client/tests.py::ClientTest.test_permanent_redirect
func TestPermanentRedirect(t *testing.T) {
	c := newSite(t, false).client(t)
	resp := c.Get("/permanent_redirect_view/")
	djangotest.AssertRedirects(t, resp, "/get_view/", djangotest.StatusCode(301))
}

// TestTemporaryRedirect GETs a URL that does a non-permanent redirect.
//
// Ported from Django: tests/test_client/tests.py::ClientTest.test_temporary_redirect
func TestTemporaryRedirect(t *testing.T) {
	c := newSite(t, false).client(t)
	resp := c.Get("/temporary_redirect_view/")
	djangotest.AssertRedirects(t, resp, "/get_view/", djangotest.StatusCode(302))
}

// TestRedirectToStrangeLocation GETs a URL that redirects to a non-200 page.
//
// Ported from Django: tests/test_client/tests.py::ClientTest.test_redirect_to_strange_location
func TestRedirectToStrangeLocation(t *testing.T) {
	c := newSite(t, false).client(t)
	resp := c.Get("/double_redirect_view/")
	// The response was a 302, and the attempt to get the redirection location
	// returned 301 when retrieved.
	djangotest.AssertRedirects(t, resp, "/permanent_redirect_view/", djangotest.TargetStatusCode(301))
}

// TestFollowRedirect checks that a URL that redirects can be followed to
// termination.
//
// Ported from Django: tests/test_client/tests.py::ClientTest.test_follow_redirect
func TestFollowRedirect(t *testing.T) {
	c := newSite(t, false).client(t)
	resp := c.Get("/double_redirect_view/", djangotest.Follow())
	djangotest.AssertRedirects(t, resp, "/get_view/", djangotest.StatusCode(302), djangotest.TargetStatusCode(200))
	djangotest.AssertEqual(t, len(resp.RedirectChain), 2)
}

// TestFollowRelativeRedirect checks that a URL with a relative redirect can be
// followed.
//
// Ported from Django: tests/test_client/tests.py::ClientTest.test_follow_relative_redirect
func TestFollowRelativeRedirect(t *testing.T) {
	c := newSite(t, false).client(t)
	resp := c.Get("/accounts/", djangotest.Follow())
	djangotest.AssertStatus(t, resp, 200)
	djangotest.AssertEqual(t, resp.Request.URL.Path, "/accounts/login/")
}

// TestFollowRelativeRedirectNoTrailingSlash checks that a URL with a relative
// redirect with no trailing slash can be followed.
//
// Ported from Django: tests/test_client/tests.py::ClientTest.test_follow_relative_redirect_no_trailing_slash
func TestFollowRelativeRedirectNoTrailingSlash(t *testing.T) {
	c := newSite(t, false).client(t)
	resp := c.Get("/accounts/no_trailing_slash", djangotest.Follow())
	djangotest.AssertStatus(t, resp, 200)
	djangotest.AssertEqual(t, resp.Request.URL.Path, "/accounts/login/")
}

// TestRedirectToQuerystringOnly checks that a URL that consists of a
// querystring only can be followed.
//
// Ported from Django: tests/test_client/tests.py::ClientTest.test_redirect_to_querystring_only
func TestRedirectToQuerystringOnly(t *testing.T) {
	c := newSite(t, false).client(t)
	resp := c.Post("/post_then_get_view/", nil, djangotest.Follow())
	djangotest.AssertStatus(t, resp, 200)
	djangotest.AssertEqual(t, resp.Request.URL.Path, "/post_then_get_view/")
	djangotest.AssertEqual(t, resp.Content(), "The value of success is true.")
}

// allMethods are the client methods Django's tests iterate over.
var allMethods = []string{"get", "post", "head", "options", "put", "patch", "delete", "trace"}

// redirectCodes are the redirects that preserve the request method.
var redirectCodes = []int{307, 308}

// TestFollow307And308Redirect checks that a 307 or 308 redirect preserves the
// request method after the redirect.
//
// Ported from Django: tests/test_client/tests.py::ClientTest.test_follow_307_and_308_redirect
func TestFollow307And308Redirect(t *testing.T) {
	c := newSite(t, false).client(t)
	for _, method := range allMethods {
		for _, code := range redirectCodes {
			t.Run(method+"/"+strconv.Itoa(code), func(t *testing.T) {
				path := "/redirect_view_" + strconv.Itoa(code) + "/"
				resp := send(c, method, path, url.Values{"value": {"test"}}, djangotest.Follow())
				djangotest.AssertStatus(t, resp, 200)
				djangotest.AssertEqual(t, resp.Request.URL.Path, "/post_view/")
				djangotest.AssertEqual(t, resp.Request.Method, methodName(method))
			})
		}
	}
}

// methodName returns the HTTP method of a client method name.
func methodName(clientMethod string) string {
	return map[string]string{
		"get": http.MethodGet, "post": http.MethodPost, "head": http.MethodHead,
		"options": http.MethodOptions, "put": http.MethodPut, "patch": http.MethodPatch,
		"delete": http.MethodDelete, "trace": http.MethodTrace,
	}[clientMethod]
}

// Ported from Django: tests/test_client/tests.py::ClientTest.test_follow_307_and_308_preserves_query_string
func TestFollow307And308PreservesQueryString(t *testing.T) {
	c := newSite(t, false).client(t)
	for _, method := range []string{"post", "options", "put", "patch", "delete", "trace"} {
		for _, code := range redirectCodes {
			t.Run(method+"/"+strconv.Itoa(code), func(t *testing.T) {
				path := "/redirect_view_" + strconv.Itoa(code) + "_query_string/"
				resp := send(c, method, path, url.Values{"value": {"test"}}, djangotest.Follow())
				djangotest.AssertRedirects(t, resp, "/post_view/?hello=world", djangotest.StatusCode(code))
				djangotest.AssertEqual(t, resp.Request.URL.RawQuery, "hello=world")
			})
		}
	}
}

// Ported from Django: tests/test_client/tests.py::ClientTest.test_follow_307_and_308_get_head_query_string
func TestFollow307And308GetHeadQueryString(t *testing.T) {
	c := newSite(t, false).client(t)
	for _, method := range []string{"get", "head"} {
		for _, code := range redirectCodes {
			t.Run(method+"/"+strconv.Itoa(code), func(t *testing.T) {
				path := "/redirect_view_" + strconv.Itoa(code) + "_query_string/"
				resp := send(c, method, path, url.Values{"value": {"test"}}, djangotest.Follow())
				djangotest.AssertRedirects(t, resp, "/post_view/?hello=world", djangotest.StatusCode(code))
				djangotest.AssertEqual(t, resp.Request.URL.RawQuery, "value=test")
			})
		}
	}
}

// Ported from Django: tests/test_client/tests.py::ClientTest.test_follow_307_and_308_preserves_post_data
func TestFollow307And308PreservesPostData(t *testing.T) {
	c := newSite(t, false).client(t)
	for _, code := range redirectCodes {
		t.Run(strconv.Itoa(code), func(t *testing.T) {
			path := "/redirect_view_" + strconv.Itoa(code) + "/"
			resp := c.Post(path, url.Values{"value": {"test"}}, djangotest.Follow())
			djangotest.AssertContains(t, resp, "test is the value")
		})
	}
}

// Ported from Django: tests/test_client/tests.py::ClientTest.test_follow_307_and_308_preserves_put_body
func TestFollow307And308PreservesPutBody(t *testing.T) {
	c := newSite(t, false).client(t)
	for _, code := range redirectCodes {
		t.Run(strconv.Itoa(code), func(t *testing.T) {
			path := "/redirect_view_" + strconv.Itoa(code) + "/?to=/put_view/"
			resp := c.Put(path, "a=b", djangotest.Follow())
			djangotest.AssertContains(t, resp, "a=b is the body")
		})
	}
}

// Ported from Django: tests/test_client/tests.py::ClientTest.test_follow_307_and_308_preserves_get_params
func TestFollow307And308PreservesGetParams(t *testing.T) {
	c := newSite(t, false).client(t)
	data := url.Values{"var": {"30"}, "to": {"/get_view/"}}
	for _, code := range redirectCodes {
		t.Run(strconv.Itoa(code), func(t *testing.T) {
			path := "/redirect_view_" + strconv.Itoa(code) + "/"
			resp := c.Get(path, djangotest.Query(data), djangotest.Follow())
			djangotest.AssertContains(t, resp, "30 is the value")
		})
	}
}

// TestRedirectHTTP GETs a URL that redirects to an HTTP URI.
//
// Ported from Django: tests/test_client/tests.py::ClientTest.test_redirect_http
func TestRedirectHTTP(t *testing.T) {
	c := newSite(t, false).client(t)
	resp := c.Get("/http_redirect_view/", djangotest.Follow())
	djangotest.AssertEqual(t, resp.Header.Get(secureHeader), "false")
}

// TestRedirectHTTPS GETs a URL that redirects to an HTTPS URI.
//
// Ported from Django: tests/test_client/tests.py::ClientTest.test_redirect_https
func TestRedirectHTTPS(t *testing.T) {
	c := newSite(t, false).client(t)
	resp := c.Get("/https_redirect_view/", djangotest.Follow())
	djangotest.AssertEqual(t, resp.Header.Get(secureHeader), "true")
}

// TestNotfoundResponse GETs a URL that responds as "404: Not Found".
//
// Ported from Django: tests/test_client/tests.py::ClientTest.test_notfound_response
func TestNotfoundResponse(t *testing.T) {
	c := newSite(t, false).client(t)
	resp := c.Get("/bad_view/")
	djangotest.AssertContains(t, resp, "MAGIC", djangotest.StatusCode(404))
}

// TestUnknownPage GETs an invalid URL.
//
// Ported from Django: tests/test_client/tests.py::ClientTest.test_unknown_page
func TestUnknownPage(t *testing.T) {
	c := newSite(t, false).client(t)
	resp := c.Get("/unknown_view/")
	djangotest.AssertStatus(t, resp, 404)
}

// TestURLParameters makes sure that URL ;-parameters are not stripped.
//
// Ported from Django: tests/test_client/tests.py::ClientTest.test_url_parameters
func TestURLParameters(t *testing.T) {
	c := newSite(t, false).client(t)
	resp := c.Get("/unknown_view/;some-parameter")
	// The path of the request includes it (ignore that it's a 404).
	djangotest.AssertEqual(t, resp.Request.URL.Path, "/unknown_view/;some-parameter")
}

// TestViewWithLogin requests a page that is protected with LoginRequired.
//
// Ported from Django: tests/test_client/tests.py::ClientTest.test_view_with_login
func TestViewWithLogin(t *testing.T) {
	s := newSite(t, false)
	s.createUsers(t)
	c := s.client(t)

	// Get the page without logging in. Should result in 302.
	resp := c.Get("/login_protected_view/")
	djangotest.AssertRedirects(t, resp, "/accounts/login/?next=/login_protected_view/")

	// Log in.
	if !c.Login("testclient", "password") {
		t.Fatal("Could not log in")
	}

	// Request a page that requires a login.
	resp = c.Get("/login_protected_view/")
	djangotest.AssertStatus(t, resp, 200)
	djangotest.AssertEqual(t, contextUsername(t, resp), "testclient")
}

// TestViewWithForceLogin requests a page that is protected with LoginRequired
// after logging in without credentials.
//
// Ported from Django: tests/test_client/tests.py::ClientTest.test_view_with_force_login
func TestViewWithForceLogin(t *testing.T) {
	s := newSite(t, false)
	u := s.createUsers(t)
	c := s.client(t)

	// Get the page without logging in. Should result in 302.
	resp := c.Get("/login_protected_view/")
	djangotest.AssertRedirects(t, resp, "/accounts/login/?next=/login_protected_view/")

	// Log in.
	c.ForceLogin(u.active)

	// Request a page that requires a login.
	resp = c.Get("/login_protected_view/")
	djangotest.AssertStatus(t, resp, 200)
	djangotest.AssertEqual(t, contextUsername(t, resp), "testclient")
}

// TestViewWithBadLogin requests a page that is protected with LoginRequired,
// but uses bad credentials.
//
// Ported from Django: tests/test_client/tests.py::ClientTest.test_view_with_bad_login
func TestViewWithBadLogin(t *testing.T) {
	s := newSite(t, false)
	s.createUsers(t)
	c := s.client(t)

	if c.Login("otheruser", "nopassword") {
		t.Error("logged in with bad credentials")
	}
}

// TestViewWithInactiveLogin checks that an inactive user cannot log in. The
// second half of the Django test switches to AllowAllUsersModelBackend, which
// has no counterpart here: authentication backends are not pluggable.
//
// Ported from Django: tests/test_client/tests.py::ClientTest.test_view_with_inactive_login
func TestViewWithInactiveLogin(t *testing.T) {
	s := newSite(t, false)
	s.createUsers(t)
	c := s.client(t)

	if c.Login("inactive", "password") {
		t.Error("an inactive user logged in")
	}
}

// TestViewWithInactiveForceLogin requests a page that is protected with
// LoginRequired as a force-logged-in inactive user. Django serves the page
// here because the test configures AllowAllUsersModelBackend; with the
// equivalent of the default ModelBackend, which is what the middleware
// implements, an inactive user is not authenticated and is sent to the login
// page.
//
// Ported from Django: tests/test_client/tests.py::ClientTest.test_view_with_inactive_force_login
func TestViewWithInactiveForceLogin(t *testing.T) {
	s := newSite(t, false)
	u := s.createUsers(t)
	c := s.client(t)

	c.ForceLogin(u.inactive)

	resp := c.Get("/login_protected_view/")
	djangotest.AssertRedirects(t, resp, "/accounts/login/?next=/login_protected_view/")
}

// TestLogout requests a logout after logging in.
//
// Ported from Django: tests/test_client/tests.py::ClientTest.test_logout
func TestLogout(t *testing.T) {
	s := newSite(t, false)
	s.createUsers(t)
	c := s.client(t)

	// Log in.
	c.Login("testclient", "password")

	// Request a page that requires a login.
	resp := c.Get("/login_protected_view/")
	djangotest.AssertStatus(t, resp, 200)
	djangotest.AssertEqual(t, contextUsername(t, resp), "testclient")

	// Log out.
	c.Logout()

	// Request a page that requires a login.
	resp = c.Get("/login_protected_view/")
	djangotest.AssertRedirects(t, resp, "/accounts/login/?next=/login_protected_view/")
}

// TestLogoutWithForceLogin requests a logout after logging in without
// credentials.
//
// Ported from Django: tests/test_client/tests.py::ClientTest.test_logout_with_force_login
func TestLogoutWithForceLogin(t *testing.T) {
	s := newSite(t, false)
	u := s.createUsers(t)
	c := s.client(t)

	// Log in.
	c.ForceLogin(u.active)

	// Request a page that requires a login.
	resp := c.Get("/login_protected_view/")
	djangotest.AssertStatus(t, resp, 200)
	djangotest.AssertEqual(t, contextUsername(t, resp), "testclient")

	// Log out.
	c.Logout()

	// Request a page that requires a login.
	resp = c.Get("/login_protected_view/")
	djangotest.AssertRedirects(t, resp, "/accounts/login/?next=/login_protected_view/")
}

// Ported from Django: tests/test_client/tests.py::ClientTest.test_external_redirect
func TestExternalRedirect(t *testing.T) {
	c := newSite(t, false).client(t)
	resp := c.Get("/django_project_redirect/")
	djangotest.AssertRedirects(t, resp, "https://www.djangoproject.com/", djangotest.FetchRedirectResponse(false))
}

// TestRedirectWithHTTPHost follows a redirect to another host, starting from a
// request with an explicit Host header (HTTP_HOST in Django).
//
// Ported from Django: tests/test_client/tests.py::ClientTest.test_redirect_with_http_host
func TestRedirectWithHTTPHost(t *testing.T) {
	c := newSite(t, false).client(t)
	resp := c.Get("/redirect_to_different_hostname/", djangotest.Follow(), djangotest.Header("Host", "hostname1"))
	djangotest.AssertEqual(t, resp.Content(), "hostname2")
}

// TestExternalRedirectWithoutTrailingSlash follows a redirect whose location
// has an empty path.
//
// Ported from Django: tests/test_client/tests.py::ClientTest.test_external_redirect_without_trailing_slash
func TestExternalRedirectWithoutTrailingSlash(t *testing.T) {
	c := newSite(t, false).client(t)
	resp := c.Get("/no_trailing_slash_external_redirect/", djangotest.Follow())
	djangotest.AssertRedirects(t, resp, "https://testserver")
}

// TestExternalRedirectWithFetchErrorMsg checks that AssertRedirects without
// FetchRedirectResponse(false) stops with a relevant message rather than a
// nondescript assertion failure. Django raises ValueError; here the test is
// stopped through Fatalf.
//
// Ported from Django: tests/test_client/tests.py::ClientTest.test_external_redirect_with_fetch_error_msg
func TestExternalRedirectWithFetchErrorMsg(t *testing.T) {
	c := newSite(t, false).client(t)
	resp := c.Get("/django_project_redirect/")
	mustFail(t, "The test client is unable to fetch remote URLs (got https://www.djangoproject.com/).", func(ft djangotest.TB) {
		djangotest.AssertRedirects(ft, resp, "https://www.djangoproject.com/")
	})
}

// TestSessionModifyingView requests a page that modifies the session.
//
// Ported from Django: tests/test_client/tests.py::ClientTest.test_session_modifying_view
func TestSessionModifyingView(t *testing.T) {
	c := newSite(t, false).client(t)
	// Session value isn't set initially.
	if _, ok := c.Session().Get("tobacconist"); ok {
		t.Fatal("session value is set before the view ran")
	}

	c.Post("/session_view/", nil)
	// The session was modified.
	value, _ := c.Session().Get("tobacconist")
	djangotest.AssertEqual(t, value, any("hovercraft"))
}

// TestViewWithException requests a page that is known to fail. Django's client
// re-raises the exception of the view in the test; the Go client lets the
// handler's panic through.
//
// Ported from Django: tests/test_client/tests.py::ClientTest.test_view_with_exception
func TestViewWithException(t *testing.T) {
	c := newSite(t, false).client(t)
	defer func() {
		err, _ := recover().(error)
		if !errors.Is(err, errBrokenView) {
			t.Errorf("recovered %v, want the panic of the view", err)
		}
	}()
	c.Get("/broken_view/")
	t.Error("the panic of the view did not reach the test")
}

// Ported from Django: tests/test_client/tests.py::ClientTest.test_relative_redirect
func TestRelativeRedirect(t *testing.T) {
	c := newSite(t, false).client(t)
	resp := c.Get("/accounts/")
	djangotest.AssertRedirects(t, resp, "/accounts/login/")
}

// Ported from Django: tests/test_client/tests.py::ClientTest.test_relative_redirect_no_trailing_slash
func TestRelativeRedirectNoTrailingSlash(t *testing.T) {
	c := newSite(t, false).client(t)
	resp := c.Get("/accounts/no_trailing_slash")
	djangotest.AssertRedirects(t, resp, "/accounts/login/")
}

// Ported from Django: tests/test_client/tests.py::ClientTest.test_query_params
func TestQueryParams(t *testing.T) {
	c := newSite(t, false).client(t)
	query := djangotest.Query(url.Values{"example": {"data"}})
	for _, method := range allMethods {
		t.Run(method, func(t *testing.T) {
			resp := send(c, method, "/get_view/", nil, query)
			djangotest.AssertEqual(t, resp.Request.URL.Query().Get("example"), "data")
		})
	}
}

// Ported from Django: tests/test_client/tests.py::ClientTest.test_follow_redirect_with_query_params
func TestFollowRedirectWithQueryParams(t *testing.T) {
	c := newSite(t, false).client(t)
	resp := c.Get("/redirect_view/", djangotest.Query(url.Values{"next": {"x"}}), djangotest.Follow())
	djangotest.AssertStatus(t, resp, 200)
	djangotest.AssertEqual(t, resp.Request.URL.RequestURI(), "/get_view/?next=x")
}

// TestCSRFEnabledClient checks that a client can be instantiated with CSRF
// checks enabled.
//
// Ported from Django: tests/test_client/tests.py::CSRFEnabledClientTests.test_csrf_enabled_client
func TestCSRFEnabledClient(t *testing.T) {
	s := newSite(t, true)
	c := s.client(t)
	csrfClient := s.client(t, djangotest.WithCSRFChecks())

	// The normal client allows the post.
	resp := c.Post("/post_view/", url.Values{})
	djangotest.AssertStatus(t, resp, 200)
	// The CSRF-enabled client rejects it.
	resp = csrfClient.Post("/post_view/", url.Values{})
	djangotest.AssertStatus(t, resp, 403)
}
