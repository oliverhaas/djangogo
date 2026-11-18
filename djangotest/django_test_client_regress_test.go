package djangotest_test

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
)

// The tests in this file are ports of Django's
// tests/test_client_regress/tests.py: the tests of the assertions themselves
// (what they accept and the messages they fail with) and of the client's
// redirect, session, JSON, and header handling.
//
// Where a Django test expects an AssertionError, the port runs the assertion
// against a recording djangotest.TB and checks the recorded message (see
// helpers_test.go). Django's messages quote the response content as a bytes
// literal (b'...'); here it is the content itself.

// mustFailPrefixed checks that call fails with want, and with "abc: " + want
// when given MsgPrefix("abc"). It is the counterpart of
// ExtraAssertMixin.assertRaisesPrefixedMessage.
func mustFailPrefixed(t *testing.T, want string, call func(ft djangotest.TB, opts ...djangotest.AssertOption)) {
	t.Helper()
	mustFail(t, want, func(ft djangotest.TB) { call(ft) })
	mustFail(t, "abc: "+want, func(ft djangotest.TB) { call(ft, djangotest.MsgPrefix("abc")) })
}

// with returns extra followed by opts, to pass fixed options along with the
// optional message prefix.
func with(opts []djangotest.AssertOption, extra ...djangotest.AssertOption) []djangotest.AssertOption {
	return append(extra, opts...)
}

// Ported from Django: tests/test_client_regress/tests.py::AssertContainsTests.test_basic_contains_not_contains
func TestBasicContainsNotContains(t *testing.T) {
	resp := newSite(t, false).client(t).Get("/no_template_view/")

	t.Run("assertNotContains", func(t *testing.T) {
		djangotest.AssertNotContains(t, resp, "never")
	})

	cases := []struct {
		text  string
		count int // -1 stands for Django's count=None
	}{
		{"never", 0},
		{"once", -1},
		{"once", 1},
		{"twice", -1},
		{"twice", 2},
	}
	for _, tc := range cases {
		t.Run(tc.text+"/"+strconv.Itoa(tc.count), func(t *testing.T) {
			if tc.count >= 0 {
				djangotest.AssertContains(t, resp, tc.text, djangotest.Count(tc.count))
			} else {
				djangotest.AssertContains(t, resp, tc.text)
			}
		})
	}
}

// Ported from Django: tests/test_client_regress/tests.py::AssertContainsTests.test_contains_with_wrong_status_code
func TestContainsWithWrongStatusCode(t *testing.T) {
	resp := newSite(t, false).client(t).Get("/no_template_view/")
	msg := "Couldn't retrieve content: Response code was 200 (expected 999)"
	mustFailPrefixed(t, msg, func(ft djangotest.TB, opts ...djangotest.AssertOption) {
		djangotest.AssertContains(ft, resp, "text", with(opts, djangotest.StatusCode(999))...)
	})
}

// Ported from Django: tests/test_client_regress/tests.py::AssertContainsTests.test_not_contains_with_wrong_status_code
func TestNotContainsWithWrongStatusCode(t *testing.T) {
	resp := newSite(t, false).client(t).Get("/no_template_view/")
	msg := "Couldn't retrieve content: Response code was 200 (expected 999)"
	mustFailPrefixed(t, msg, func(ft djangotest.TB, opts ...djangotest.AssertOption) {
		djangotest.AssertNotContains(ft, resp, "text", with(opts, djangotest.StatusCode(999))...)
	})
}

// Ported from Django: tests/test_client_regress/tests.py::AssertContainsTests.test_not_contains_failure
func TestNotContainsFailure(t *testing.T) {
	resp := newSite(t, false).client(t).Get("/no_template_view/")
	msg := "'once' unexpectedly found in the following response\n" + resp.Content()
	mustFailPrefixed(t, msg, func(ft djangotest.TB, opts ...djangotest.AssertOption) {
		djangotest.AssertNotContains(ft, resp, "once", opts...)
	})
}

// Ported from Django: tests/test_client_regress/tests.py::AssertContainsTests.test_count_mismatch
func TestCountMismatch(t *testing.T) {
	resp := newSite(t, false).client(t).Get("/no_template_view/")
	msg := "Found 0 instances of 'never' (expected 1) in the following response\n" + resp.Content()
	mustFailPrefixed(t, msg, func(ft djangotest.TB, opts ...djangotest.AssertOption) {
		djangotest.AssertContains(ft, resp, "never", with(opts, djangotest.Count(1))...)
	})
}

// Ported from Django: tests/test_client_regress/tests.py::AssertContainsTests.test_unexpected_presence
func TestUnexpectedPresence(t *testing.T) {
	resp := newSite(t, false).client(t).Get("/no_template_view/")
	msg := "Found 1 instances of 'once' (expected 0) in the following response\n" + resp.Content()
	mustFailPrefixed(t, msg, func(ft djangotest.TB, opts ...djangotest.AssertOption) {
		djangotest.AssertContains(ft, resp, "once", with(opts, djangotest.Count(0))...)
	})
}

// Ported from Django: tests/test_client_regress/tests.py::AssertContainsTests.test_insufficient_count
func TestInsufficientCount(t *testing.T) {
	resp := newSite(t, false).client(t).Get("/no_template_view/")
	msg := "Found 1 instances of 'once' (expected 2) in the following response\n" + resp.Content()
	mustFailPrefixed(t, msg, func(ft djangotest.TB, opts ...djangotest.AssertOption) {
		djangotest.AssertContains(ft, resp, "once", with(opts, djangotest.Count(2))...)
	})
}

// Ported from Django: tests/test_client_regress/tests.py::AssertContainsTests.test_excessive_count
func TestExcessiveCount(t *testing.T) {
	resp := newSite(t, false).client(t).Get("/no_template_view/")
	msg := "Found 2 instances of 'twice' (expected 1) in the following response\n" + resp.Content()
	mustFailPrefixed(t, msg, func(ft djangotest.TB, opts ...djangotest.AssertOption) {
		djangotest.AssertContains(ft, resp, "twice", with(opts, djangotest.Count(1))...)
	})
}

// TestMissingContent keeps the misspelled prefix of the Django test.
//
// Ported from Django: tests/test_client_regress/tests.py::AssertContainsTests.test_missing_content
func TestMissingContent(t *testing.T) {
	resp := newSite(t, false).client(t).Get("/no_template_view/")
	msg := "Couldn't find 'thrice' in the following response\n" + resp.Content()
	mustFail(t, msg, func(ft djangotest.TB) {
		djangotest.AssertContains(ft, resp, "thrice")
	})
	mustFail(t, "Custom prexix: "+msg, func(ft djangotest.TB) {
		djangotest.AssertContains(ft, resp, "thrice", djangotest.MsgPrefix("Custom prexix"))
	})
}

// Ported from Django: tests/test_client_regress/tests.py::AssertContainsTests.test_missing_content_with_count
func TestMissingContentWithCount(t *testing.T) {
	resp := newSite(t, false).client(t).Get("/no_template_view/")
	msg := "Found 0 instances of 'thrice' (expected 3) in the following response\n" + resp.Content()
	mustFailPrefixed(t, msg, func(ft djangotest.TB, opts ...djangotest.AssertOption) {
		djangotest.AssertContains(ft, resp, "thrice", with(opts, djangotest.Count(3))...)
	})
}

// TestLongContent checks that a long response is quoted in full. Django builds
// an HttpResponse by hand; the port builds a Response by hand.
//
// Ported from Django: tests/test_client_regress/tests.py::AssertContainsTests.test_long_content
func TestLongContent(t *testing.T) {
	longContent := "This is a very very very very very very very very long message which " +
		"exceeds the max limit of truncation."
	resp := &djangotest.Response{StatusCode: http.StatusOK, Body: []byte(longContent)}

	mustFail(t, "Couldn't find 'thrice' in the following response\n"+longContent, func(ft djangotest.TB) {
		djangotest.AssertContains(ft, resp, "thrice")
	})
	mustFail(t, "Found 1 instances of 'This' (expected 3) in the following response\n"+longContent, func(ft djangotest.TB) {
		djangotest.AssertContains(ft, resp, "This", djangotest.Count(3))
	})
	mustFail(t, "'very' unexpectedly found in the following response\n"+longContent, func(ft djangotest.TB) {
		djangotest.AssertNotContains(ft, resp, "very")
	})
}

// TestUnicodeContains checks that Unicode characters can be found in template
// context.
//
// Ported from Django: tests/test_client_regress/tests.py::AssertContainsTests.test_unicode_contains
func TestUnicodeContains(t *testing.T) {
	resp := newSite(t, false).client(t).Get("/check_unicode/")
	djangotest.AssertContains(t, resp, "さかき")
	djangotest.AssertContains(t, resp, string([]byte{0xe5, 0xb3, 0xa0}))
}

// TestUnicodeNotContains checks that Unicode characters can be searched for,
// and not found in template context.
//
// Ported from Django: tests/test_client_regress/tests.py::AssertContainsTests.test_unicode_not_contains
func TestUnicodeNotContains(t *testing.T) {
	resp := newSite(t, false).client(t).Get("/check_unicode/")
	djangotest.AssertNotContains(t, resp, "はたけ")
	djangotest.AssertNotContains(t, resp, string([]byte{0xe3, 0x81, 0xaf, 0xe3, 0x81, 0x9f, 0xe3, 0x81, 0x91}))
}

// Ported from Django: tests/test_client_regress/tests.py::AssertContainsTests.test_assert_contains_using_non_template_response
func TestAssertContainsUsingNonTemplateResponse(t *testing.T) {
	resp := &djangotest.Response{StatusCode: http.StatusOK, Body: []byte("Hello")}
	djangotest.AssertContains(t, resp, "Hello")
}

// Ported from Django: tests/test_client_regress/tests.py::AssertContainsTests.test_assert_not_contains_using_non_template_response
func TestAssertNotContainsUsingNonTemplateResponse(t *testing.T) {
	resp := &djangotest.Response{StatusCode: http.StatusOK, Body: []byte("Hello")}
	djangotest.AssertNotContains(t, resp, "Bye")
}

// TestNoContext checks that template usage assertions work when templates
// aren't in use.
//
// Ported from Django: tests/test_client_regress/tests.py::AssertTemplateUsedTests.test_no_context
func TestNoContext(t *testing.T) {
	resp := newSite(t, false).client(t).Get("/no_template_view/")

	// The no template case doesn't mess with the template assertions.
	djangotest.AssertTemplateNotUsed(t, resp, "GET Template")

	msg := "No templates used to render the response"
	mustFail(t, msg, func(ft djangotest.TB) {
		djangotest.AssertTemplateUsed(ft, resp, "GET Template")
	})
	mustFail(t, msg, func(ft djangotest.TB) {
		djangotest.AssertTemplateUsed(ft, resp, "GET Template", djangotest.MsgPrefix("abc"))
	})
	mustFail(t, msg, func(ft djangotest.TB) {
		djangotest.AssertTemplateUsed(ft, resp, "GET Template", djangotest.Count(2))
	})
}

// TestSingleContext checks that template assertions work when there is a
// single context. The messages of the Django test start with ": " because
// unittest puts "True is not false" in front of them; a Go failure message has
// no such part.
//
// Ported from Django: tests/test_client_regress/tests.py::AssertTemplateUsedTests.test_single_context
func TestSingleContext(t *testing.T) {
	resp := newSite(t, false).client(t).Get("/post_view/")

	msg := "Template 'Empty GET Template' was used unexpectedly in rendering the response"
	mustFailPrefixed(t, msg, func(ft djangotest.TB, opts ...djangotest.AssertOption) {
		djangotest.AssertTemplateNotUsed(ft, resp, "Empty GET Template", opts...)
	})

	msg = "Template 'Empty POST Template' was not a template used to " +
		"render the response. Actual template(s) used: Empty GET Template"
	mustFailPrefixed(t, msg, func(ft djangotest.TB, opts ...djangotest.AssertOption) {
		djangotest.AssertTemplateUsed(ft, resp, "Empty POST Template", opts...)
	})

	msg = "Template 'Empty GET Template' was expected to be rendered 2 " +
		"time(s) but was actually rendered 1 time(s)."
	mustFailPrefixed(t, msg, func(ft djangotest.TB, opts ...djangotest.AssertOption) {
		djangotest.AssertTemplateUsed(ft, resp, "Empty GET Template", with(opts, djangotest.Count(2))...)
	})
}

// TestTemplateRenderedMultipleTimes checks that template assertions work when
// a template is rendered multiple times.
//
// Ported from Django: tests/test_client_regress/tests.py::AssertTemplateUsedTests.test_template_rendered_multiple_times
func TestTemplateRenderedMultipleTimes(t *testing.T) {
	resp := newSite(t, false).client(t).Get("/render_template_multiple_times/")
	djangotest.AssertTemplateUsed(t, resp, "base.html", djangotest.Count(2))
}

// TestRedirectPage checks that an assertion fails if the original page
// couldn't be retrieved as expected.
//
// Ported from Django: tests/test_client_regress/tests.py::AssertRedirectsTests.test_redirect_page
func TestRedirectPage(t *testing.T) {
	// This page will redirect with code 301, not 302.
	resp := newSite(t, false).client(t).Get("/permanent_redirect_view/")
	msg := "Response didn't redirect as expected: Response code was 301 (expected 302)"
	mustFailPrefixed(t, msg, func(ft djangotest.TB, opts ...djangotest.AssertOption) {
		djangotest.AssertRedirects(ft, resp, "/get_view/", opts...)
	})
}

// Ported from Django: tests/test_client_regress/tests.py::AssertRedirectsTests.test_followed_redirect_unexpected_initial_status_code
func TestFollowedRedirectUnexpectedInitialStatusCode(t *testing.T) {
	resp := newSite(t, false).client(t).Get("/permanent_redirect_view/", djangotest.Follow())
	msg := "Initial response didn't redirect as expected: Response code was 301 (expected 302)"
	mustFailPrefixed(t, msg, func(ft djangotest.TB, opts ...djangotest.AssertOption) {
		djangotest.AssertRedirects(ft, resp, "/get_view/", opts...)
	})
}

// Ported from Django: tests/test_client_regress/tests.py::AssertRedirectsTests.test_followed_redirect_unexpected_final_status_code
func TestFollowedRedirectUnexpectedFinalStatusCode(t *testing.T) {
	resp := newSite(t, false).client(t).Get("/redirect_view/", djangotest.Follow())
	msg := "Response didn't redirect as expected: Final Response code was 200 (expected 403)"
	mustFailPrefixed(t, msg, func(ft djangotest.TB, opts ...djangotest.AssertOption) {
		djangotest.AssertRedirects(ft, resp, "/get_view/",
			with(opts, djangotest.StatusCode(302), djangotest.TargetStatusCode(403))...)
	})
}

// TestLostQuery checks that an assertion fails if the redirect location
// doesn't preserve GET parameters.
//
// Ported from Django: tests/test_client_regress/tests.py::AssertRedirectsTests.test_lost_query
func TestLostQuery(t *testing.T) {
	resp := newSite(t, false).client(t).Get("/redirect_view/", djangotest.Query(url.Values{"var": {"value"}}))
	msg := "Response redirected to '/get_view/?var=value', expected '/get_view/'"
	mustFailPrefixed(t, msg, func(ft djangotest.TB, opts ...djangotest.AssertOption) {
		djangotest.AssertRedirects(ft, resp, "/get_view/", opts...)
	})
}

// TestIncorrectTarget checks that an assertion fails if the response redirects
// to another target.
//
// Ported from Django: tests/test_client_regress/tests.py::AssertRedirectsTests.test_incorrect_target
func TestIncorrectTarget(t *testing.T) {
	resp := newSite(t, false).client(t).Get("/permanent_redirect_view/")
	msg := "Response didn't redirect as expected: Response code was 301 (expected 302)"
	mustFail(t, msg, func(ft djangotest.TB) {
		// Should redirect to get_view.
		djangotest.AssertRedirects(ft, resp, "/some_view/")
	})
}

// TestTargetPage checks that an assertion fails if the response redirect
// target cannot be retrieved as expected.
//
// Ported from Django: tests/test_client_regress/tests.py::AssertRedirectsTests.test_target_page
func TestTargetPage(t *testing.T) {
	resp := newSite(t, false).client(t).Get("/double_redirect_view/")
	msg := "Couldn't retrieve redirection page '/permanent_redirect_view/': " +
		"response code was 301 (expected 200)"
	mustFailPrefixed(t, msg, func(ft djangotest.TB, opts ...djangotest.AssertOption) {
		// The redirect target responds with a 301 code, not 200.
		djangotest.AssertRedirects(ft, resp, "http://testserver/permanent_redirect_view/", opts...)
	})
}

// TestRedirectChain checks that a chain of one redirect can be followed.
//
// Ported from Django: tests/test_client_regress/tests.py::AssertRedirectsTests.test_redirect_chain
func TestRedirectChain(t *testing.T) {
	resp := newSite(t, false).client(t).Get("/redirects/further/more/", djangotest.Follow())
	djangotest.AssertRedirects(t, resp, "/no_template_view/",
		djangotest.StatusCode(302), djangotest.TargetStatusCode(200))

	djangotest.AssertSequenceEqual(t, resp.RedirectChain, []djangotest.Redirect{
		{URL: "/no_template_view/", StatusCode: 302},
	})
}

// TestMultipleRedirectChain checks that a chain of multiple redirects can be
// followed.
//
// Ported from Django: tests/test_client_regress/tests.py::AssertRedirectsTests.test_multiple_redirect_chain
func TestMultipleRedirectChain(t *testing.T) {
	resp := newSite(t, false).client(t).Get("/redirects/", djangotest.Follow())
	djangotest.AssertRedirects(t, resp, "/no_template_view/",
		djangotest.StatusCode(302), djangotest.TargetStatusCode(200))

	djangotest.AssertSequenceEqual(t, resp.RedirectChain, []djangotest.Redirect{
		{URL: "/redirects/further/", StatusCode: 302},
		{URL: "/redirects/further/more/", StatusCode: 302},
		{URL: "/no_template_view/", StatusCode: 302},
	})
}

// TestRedirectChainToNonExistent checks that a chain to a nonexistent view can
// be followed.
//
// Ported from Django: tests/test_client_regress/tests.py::AssertRedirectsTests.test_redirect_chain_to_non_existent
func TestRedirectChainToNonExistent(t *testing.T) {
	resp := newSite(t, false).client(t).Get("/redirect_to_non_existent_view2/", djangotest.Follow())
	djangotest.AssertRedirects(t, resp, "/non_existent_view/",
		djangotest.StatusCode(302), djangotest.TargetStatusCode(404))
}

// TestRedirectChainToSelf checks that redirections to self are caught and
// escaped. Django raises RedirectCycleError, which carries the last response;
// the Go client stops the test with the same message, so the assertions the
// Django test makes on that last response have nothing to run on.
//
// Ported from Django: tests/test_client_regress/tests.py::AssertRedirectsTests.test_redirect_chain_to_self
func TestRedirectChainToSelf(t *testing.T) {
	s := newSite(t, false)
	mustFail(t, "Redirect loop detected.", func(ft djangotest.TB) {
		s.client(ft).Get("/redirect_to_self/", djangotest.Follow())
	})
}

// TestRedirectToSelfWithChangingQuery checks that redirections don't loop
// forever even if the query is changing.
//
// Ported from Django: tests/test_client_regress/tests.py::AssertRedirectsTests.test_redirect_to_self_with_changing_query
func TestRedirectToSelfWithChangingQuery(t *testing.T) {
	s := newSite(t, false)
	mustFail(t, "Too many redirects.", func(ft djangotest.TB) {
		s.client(ft).Get("/redirect_to_self_with_changing_query_view/",
			djangotest.Query(url.Values{"counter": {"0"}}), djangotest.Follow())
	})
}

// TestCircularRedirect checks that circular redirect chains are caught and
// escaped.
//
// Ported from Django: tests/test_client_regress/tests.py::AssertRedirectsTests.test_circular_redirect
func TestCircularRedirect(t *testing.T) {
	s := newSite(t, false)
	mustFail(t, "Redirect loop detected.", func(ft djangotest.TB) {
		s.client(ft).Get("/circular_redirect_1/", djangotest.Follow())
	})
}

// TestRedirectChainMethods checks that a redirect chain is followed from an
// initial POST, HEAD, OPTIONS, PUT, or DELETE request.
//
// Ported from Django: tests/test_client_regress/tests.py::AssertRedirectsTests.test_redirect_chain_post
// Ported from Django: tests/test_client_regress/tests.py::AssertRedirectsTests.test_redirect_chain_head
// Ported from Django: tests/test_client_regress/tests.py::AssertRedirectsTests.test_redirect_chain_options
// Ported from Django: tests/test_client_regress/tests.py::AssertRedirectsTests.test_redirect_chain_put
// Ported from Django: tests/test_client_regress/tests.py::AssertRedirectsTests.test_redirect_chain_delete
func TestRedirectChainMethods(t *testing.T) {
	c := newSite(t, false).client(t)
	nothing := url.Values{"nothing": {"to_send"}}
	requests := map[string]func() *djangotest.Response{
		"post": func() *djangotest.Response { return c.Post("/redirects/", nothing, djangotest.Follow()) },
		"head": func() *djangotest.Response {
			return c.Head("/redirects/", djangotest.Query(nothing), djangotest.Follow())
		},
		"options": func() *djangotest.Response { return c.Options("/redirects/", djangotest.Follow()) },
		"put":     func() *djangotest.Response { return c.Put("/redirects/", nil, djangotest.Follow()) },
		"delete":  func() *djangotest.Response { return c.Delete("/redirects/", nil, djangotest.Follow()) },
	}
	for method, request := range requests {
		t.Run(method, func(t *testing.T) {
			resp := request()
			djangotest.AssertRedirects(t, resp, "/no_template_view/",
				djangotest.StatusCode(302), djangotest.TargetStatusCode(200))
			djangotest.AssertEqual(t, len(resp.RedirectChain), 3)
		})
	}
}

// TestRedirectToDifferentHost checks that the test client preserves scheme,
// host and port changes. Django appends "otherserver" to ALLOWED_HOSTS; here
// the client is built with WithAllowedHosts.
//
// Ported from Django: tests/test_client_regress/tests.py::AssertRedirectsTests.test_redirect_to_different_host
func TestRedirectToDifferentHost(t *testing.T) {
	c := newSite(t, false).client(t, djangotest.WithAllowedHosts("otherserver"))

	resp := c.Get("/redirect_other_host/", djangotest.Follow())
	djangotest.AssertRedirects(t, resp, "https://otherserver:8443/no_template_view/",
		djangotest.StatusCode(302), djangotest.TargetStatusCode(200))
	if resp.Request.TLS == nil {
		t.Error("the followed request is not secure")
	}
	djangotest.AssertEqual(t, resp.Request.Host, "otherserver:8443")

	// AssertRedirects can follow the redirect to 'otherserver' too.
	resp = c.Get("/redirect_other_host/")
	djangotest.AssertRedirects(t, resp, "https://otherserver:8443/no_template_view/",
		djangotest.StatusCode(302), djangotest.TargetStatusCode(200))
}

// TestRedirectChainOnNonRedirectPage checks that an assertion fails if the
// original page couldn't be retrieved as expected.
//
// Ported from Django: tests/test_client_regress/tests.py::AssertRedirectsTests.test_redirect_chain_on_non_redirect_page
func TestRedirectChainOnNonRedirectPage(t *testing.T) {
	resp := newSite(t, false).client(t).Get("/get_view/", djangotest.Follow())
	msg := "Response didn't redirect as expected: Response code was 200 (expected 302)"
	mustFailPrefixed(t, msg, func(ft djangotest.TB, opts ...djangotest.AssertOption) {
		djangotest.AssertRedirects(ft, resp, "/get_view/", opts...)
	})
}

// TestRedirectOnNonRedirectPage checks that an assertion fails if the original
// page couldn't be retrieved as expected.
//
// Ported from Django: tests/test_client_regress/tests.py::AssertRedirectsTests.test_redirect_on_non_redirect_page
func TestRedirectOnNonRedirectPage(t *testing.T) {
	resp := newSite(t, false).client(t).Get("/get_view/")
	msg := "Response didn't redirect as expected: Response code was 200 (expected 302)"
	mustFailPrefixed(t, msg, func(ft djangotest.TB, opts ...djangotest.AssertOption) {
		djangotest.AssertRedirects(ft, resp, "/get_view/", opts...)
	})
}

// TestRedirectScheme checks that an assertion fails if the response doesn't
// have the scheme specified in the expected URL.
//
// Ported from Django: tests/test_client_regress/tests.py::AssertRedirectsTests.test_redirect_scheme
func TestRedirectScheme(t *testing.T) {
	c := newSite(t, false).client(t)
	// For all possible true/false combinations of follow and secure.
	for _, follow := range []bool{true, false} {
		for _, secure := range []bool{true, false} {
			t.Run("follow="+strconv.FormatBool(follow)+"/secure="+strconv.FormatBool(secure), func(t *testing.T) {
				var opts []djangotest.RequestOption
				if follow {
					opts = append(opts, djangotest.Follow())
				}
				if secure {
					opts = append(opts, djangotest.Secure())
				}
				// Always redirects to https.
				resp := c.Get("/https_redirect_view/", opts...)
				// The goal scheme is https.
				djangotest.AssertRedirects(t, resp, "https://testserver/secure_view/", djangotest.StatusCode(302))
				mustFail(t, "", func(ft djangotest.TB) {
					djangotest.AssertRedirects(ft, resp, "http://testserver/secure_view/", djangotest.StatusCode(302))
				})
			})
		}
	}
}

// TestRedirectFetchRedirectResponse checks that the extra headers of a request
// are preserved when AssertRedirects fetches the redirect target. Django passes
// the header once through "extra" (HTTP_REDIRECT) and once through "headers";
// the Go client has one way to set a request header.
//
// Ported from Django: tests/test_client_regress/tests.py::AssertRedirectsTests.test_redirect_fetch_redirect_response
func TestRedirectFetchRedirectResponse(t *testing.T) {
	c := newSite(t, false).client(t)
	for _, method := range allMethods {
		t.Run(method, func(t *testing.T) {
			resp := send(c, method, "/redirect_based_on_extra_headers_1/", nil, djangotest.Header("Redirect", "val"))
			djangotest.AssertRedirects(t, resp, "/redirect_based_on_extra_headers_2/",
				djangotest.FetchRedirectResponse(true),
				djangotest.StatusCode(302), djangotest.TargetStatusCode(302))
		})
	}
}

// TestRedirectFetchWithoutClient is not a Django test: a response built by hand
// has no client to fetch the redirect target with, and the assertion says so
// instead of failing on a nil client.
func TestRedirectFetchWithoutClient(t *testing.T) {
	resp := &djangotest.Response{
		StatusCode: 302,
		Header:     http.Header{"Location": {"/get_view/"}},
	}
	mustFail(t, "AssertRedirects needs a response returned by a Client", func(ft djangotest.TB) {
		djangotest.AssertRedirects(ft, resp, "/get_view/")
	})

	// Without the fetch, the same response is checked on its own.
	djangotest.AssertRedirects(t, resp, "/get_view/", djangotest.FetchRedirectResponse(false))
}

// TestSession checks that the session isn't lost if a user logs in.
//
// Ported from Django: tests/test_client_regress/tests.py::SessionTests.test_session
func TestSession(t *testing.T) {
	s := newSite(t, false)
	s.createUsers(t)
	c := s.client(t)

	// The session doesn't exist to start.
	resp := c.Get("/check_session/")
	djangotest.AssertStatus(t, resp, 200)
	djangotest.AssertEqual(t, resp.Content(), "NO")

	// This request sets a session variable.
	resp = c.Get("/set_session/")
	djangotest.AssertStatus(t, resp, 200)
	djangotest.AssertEqual(t, resp.Content(), "set_session")

	// The session has been modified.
	resp = c.Get("/check_session/")
	djangotest.AssertStatus(t, resp, 200)
	djangotest.AssertEqual(t, resp.Content(), "YES")

	// Log in.
	if !c.Login("testclient", "password") {
		t.Fatal("Could not log in")
	}

	// Session should still contain the modified value.
	resp = c.Get("/check_session/")
	djangotest.AssertStatus(t, resp, 200)
	djangotest.AssertEqual(t, resp.Content(), "YES")
}

// Ported from Django: tests/test_client_regress/tests.py::SessionTests.test_session_initiated
func TestSessionInitiated(t *testing.T) {
	c := newSite(t, false).client(t)
	session := c.Session()
	session.Set("session_var", "foo")
	c.SaveSession(session)

	resp := c.Get("/check_session/")
	djangotest.AssertEqual(t, resp.Content(), "foo")
}

// TestSessionLogout checks that logout works whether the user is logged in or
// not (#9978). The name carries the class of the Django test because TestLogout
// is the port of ClientTest.test_logout.
//
// Ported from Django: tests/test_client_regress/tests.py::SessionTests.test_logout
func TestSessionLogout(t *testing.T) {
	s := newSite(t, false)
	s.createUsers(t)
	c := s.client(t)

	c.Logout()
	if !c.Login("testclient", "password") {
		t.Fatal("Could not log in")
	}
	c.Logout()
	c.Logout()
}

// TestRequestMethods requests a view via each request method.
//
// Ported from Django: tests/test_client_regress/tests.py::RequestMethodTests.test_get
// Ported from Django: tests/test_client_regress/tests.py::RequestMethodTests.test_post
// Ported from Django: tests/test_client_regress/tests.py::RequestMethodTests.test_options
// Ported from Django: tests/test_client_regress/tests.py::RequestMethodTests.test_put
// Ported from Django: tests/test_client_regress/tests.py::RequestMethodTests.test_delete
// Ported from Django: tests/test_client_regress/tests.py::RequestMethodTests.test_patch
func TestRequestMethods(t *testing.T) {
	c := newSite(t, false).client(t)
	for _, method := range []string{"get", "post", "options", "put", "delete", "patch"} {
		t.Run(method, func(t *testing.T) {
			resp := send(c, method, "/request_methods/", nil)
			djangotest.AssertStatus(t, resp, 200)
			djangotest.AssertEqual(t, resp.Content(), "request method: "+strings.ToUpper(method))
		})
	}
}

// TestRequestMethodHead requests a view via request method HEAD.
//
// Ported from Django: tests/test_client_regress/tests.py::RequestMethodTests.test_head
func TestRequestMethodHead(t *testing.T) {
	resp := newSite(t, false).client(t).Head("/request_methods/")
	djangotest.AssertStatus(t, resp, 200)
	// A HEAD request doesn't return any content.
	djangotest.AssertNotEqual(t, resp.Content(), "request method: HEAD")
	djangotest.AssertEqual(t, resp.Content(), "")
}

// TestRequestMethodStringData requests a view with string data via the request
// methods POST, PUT, and PATCH (#11371, #17797).
//
// Ported from Django: tests/test_client_regress/tests.py::RequestMethodStringDataTests.test_post
// Ported from Django: tests/test_client_regress/tests.py::RequestMethodStringDataTests.test_put
// Ported from Django: tests/test_client_regress/tests.py::RequestMethodStringDataTests.test_patch
func TestRequestMethodStringData(t *testing.T) {
	c := newSite(t, false).client(t)
	data := `{"test": "json"}`
	requests := map[string]func(string, any, ...djangotest.RequestOption) *djangotest.Response{
		"POST": c.Post, "PUT": c.Put, "PATCH": c.Patch,
	}
	for method, request := range requests {
		t.Run(method, func(t *testing.T) {
			resp := request("/request_methods/", data, djangotest.ContentType(djangotest.JSONContentType))
			djangotest.AssertStatus(t, resp, 200)
			djangotest.AssertEqual(t, resp.Content(), "request method: "+method)
		})
	}
}

// TestEmptyStringData requests a view with empty string data (#21740). Django
// also sends the empty body with GET and HEAD; the Go client sends a body only
// with the methods that take one, so those two requests carry none.
//
// Ported from Django: tests/test_client_regress/tests.py::RequestMethodStringDataTests.test_empty_string_data
func TestEmptyStringData(t *testing.T) {
	c := newSite(t, false).client(t)
	json := djangotest.ContentType(djangotest.JSONContentType)

	djangotest.AssertEqual(t, c.Get("/body/", json).Content(), "")
	djangotest.AssertEqual(t, c.Post("/body/", "", json).Content(), "")
	djangotest.AssertEqual(t, c.Head("/body/", json).Content(), "")
}

// Ported from Django: tests/test_client_regress/tests.py::RequestMethodStringDataTests.test_json_bytes
func TestJSONBytes(t *testing.T) {
	c := newSite(t, false).client(t)
	resp := c.Post("/body/", []byte("{'value': 37}"), djangotest.ContentType(djangotest.JSONContentType))
	djangotest.AssertEqual(t, resp.Content(), "{'value': 37}")
}

// Ported from Django: tests/test_client_regress/tests.py::RequestMethodStringDataTests.test_json
func TestJSON(t *testing.T) {
	resp := newSite(t, false).client(t).Get("/json_response/")
	var got map[string]string
	if err := resp.JSON(&got); err != nil {
		t.Fatal(err)
	}
	djangotest.AssertEqual(t, got, map[string]string{"key": "value"})
}

// Ported from Django: tests/test_client_regress/tests.py::RequestMethodStringDataTests.test_json_structured_suffixes
func TestJSONStructuredSuffixes(t *testing.T) {
	c := newSite(t, false).client(t)
	validTypes := []string{
		"application/vnd.api+json",
		"application/vnd.api.foo+json",
		"application/json; charset=utf-8",
		"application/activity+json",
		"application/activity+json; charset=utf-8",
	}
	for _, contentType := range validTypes {
		t.Run(contentType, func(t *testing.T) {
			resp := c.Get("/json_response/", djangotest.Query(url.Values{"content_type": {contentType}}))
			djangotest.AssertEqual(t, resp.Header.Get("Content-Type"), contentType)
			var got map[string]string
			if err := resp.JSON(&got); err != nil {
				t.Fatal(err)
			}
			djangotest.AssertEqual(t, got, map[string]string{"key": "value"})
		})
	}
}

// TestJSONWrongHeader checks that JSON refuses a response that is not JSON.
// Django raises ValueError; Response.JSON returns an error with the same text.
//
// Ported from Django: tests/test_client_regress/tests.py::RequestMethodStringDataTests.test_json_wrong_header
func TestJSONWrongHeader(t *testing.T) {
	resp := newSite(t, false).client(t).Get("/body/")
	msg := `Content-Type header is "text/html; charset=utf-8", not "application/json"`
	var got map[string]string
	err := resp.JSON(&got)
	if err == nil || !strings.Contains(err.Error(), msg) {
		t.Errorf("JSON error is %v, want one containing %q", err, msg)
	}
}

// TestClientHeaders checks that a test client can send custom headers.
//
// Ported from Django: tests/test_client_regress/tests.py::RequestHeadersTest.test_client_headers
func TestClientHeaders(t *testing.T) {
	c := newSite(t, false).client(t)
	resp := c.Get("/check_headers/", djangotest.Header("x-arg-check", "Testing 123"))
	djangotest.AssertEqual(t, resp.Content(), "HTTP_X_ARG_CHECK: Testing 123")
	djangotest.AssertStatus(t, resp, 200)
}

// TestClientHeadersRedirect checks that test client headers are preserved
// through redirects.
//
// Ported from Django: tests/test_client_regress/tests.py::RequestHeadersTest.test_client_headers_redirect
func TestClientHeadersRedirect(t *testing.T) {
	c := newSite(t, false).client(t)
	resp := c.Get("/check_headers_redirect/", djangotest.Follow(), djangotest.Header("x-arg-check", "Testing 123"))
	djangotest.AssertEqual(t, resp.Content(), "HTTP_X_ARG_CHECK: Testing 123")
	djangotest.AssertRedirects(t, resp, "/check_headers/",
		djangotest.StatusCode(302), djangotest.TargetStatusCode(200))
}
