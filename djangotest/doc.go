// Package djangotest is the test toolkit of Djan-Go-Go, the counterpart of
// Django's django.test package. It provides a stateful test client that drives
// an http.Handler in process, the Django assertion vocabulary as plain
// functions, and isolated test databases on SQLite and PostgreSQL.
//
// # Mapping from Django
//
// Django's tests are methods on TestCase classes. Here they are ordinary Go
// test functions, and what Django reaches through self is passed explicitly:
//
//	self.client.get("/x/")                    c := djangotest.NewClient(t, handler); c.Get("/x/")
//	self.client.get("/x/", {"a": "b"})        c.Get("/x/", djangotest.Query(url.Values{"a": {"b"}}))
//	self.client.post("/x/", {"a": "b"})       c.Post("/x/", url.Values{"a": {"b"}})
//	self.client.get("/x/", follow=True)       c.Get("/x/", djangotest.Follow())
//	self.client.get("/x/", secure=True)       c.Get("/x/", djangotest.Secure())
//	self.client.get("/x/", headers={...})     c.Get("/x/", djangotest.Header("X-Name", "value"))
//	self.client.login(username=, password=)   c.Login(username, password)
//	self.client.force_login(user)             c.ForceLogin(user)
//	self.client.session                       c.Session(), c.SaveSession(session)
//	Client(enforce_csrf_checks=True)          djangotest.NewClient(t, handler, djangotest.WithCSRFChecks())
//	response.context["key"]                   resp.Context("key")
//	response.templates[0].name                resp.Templates[0].Name
//	response.redirect_chain                   resp.RedirectChain
//	response.json()                           resp.JSON(&v)
//	self.assertEqual(a, b)                    djangotest.AssertEqual(t, a, b)
//	self.assertContains(response, "x")        djangotest.AssertContains(t, resp, "x")
//	assertContains(r, "x", count=2)           djangotest.AssertContains(t, resp, "x", djangotest.Count(2))
//	assertContains(r, "<b>x</b>", html=True)  djangotest.AssertContains(t, resp, "<b>x</b>", djangotest.HTML())
//	self.assertRedirects(response, "/y/")     djangotest.AssertRedirects(t, resp, "/y/")
//	self.assertTemplateUsed(response, "t")    djangotest.AssertTemplateUsed(t, resp, "t")
//	self.assertHTMLEqual(a, b)                djangotest.AssertHTMLEqual(t, a, b)
//	self.assertInHTML(needle, haystack)       djangotest.AssertInHTML(t, needle, haystack)
//	self.assertJSONEqual(raw, expected)       djangotest.AssertJSONEqual(t, raw, expected)
//	self.assertURLEqual(a, b)                 djangotest.AssertURLEqual(t, a, b)
//	self.assertNumQueries(2, fn)              djangotest.AssertNumQueries(t, db, 2, fn)
//	CaptureQueriesContext(connection)         djangotest.CaptureQueries(db)
//	self.assertQuerySetEqual(qs, values, f)   djangotest.AssertQuerySetEqual(t, qs, values, f)
//	self.assertRaises(Model.DoesNotExist)     djangotest.AssertErrorIs(t, err, orm.ErrDoesNotExist)
//	models.py of a test app                   djangotest.App("label", &Model{}, ...) passed to NewDB
//	setUpTestData                             a plain helper function that inserts rows
//	@modify_settings(ALLOWED_HOSTS=...)       djangotest.WithAllowedHosts("otherserver")
//
// The keyword arguments of the assertions (status_code, target_status_code,
// count, html, fetch_redirect_response, msg_prefix) are options: StatusCode,
// TargetStatusCode, Count, HTML, FetchRedirectResponse, MsgPrefix.
//
// A test database is created per test: NewDB returns a private in-memory SQLite
// database, and EachDB runs a test body once per configured dialect (SQLite
// always, PostgreSQL when DJANGOGO_TEST_POSTGRES_DSN is set), each in its own
// database or schema. There is no shared state between tests, so tests may run
// in parallel.
//
// # Differences from Django
//
// The assertions fail with Django's messages, so a test of a message ports
// unchanged, with these exceptions: the part unittest puts in front of a
// message ("False is not true : ", "1 != 2 : ") is absent, a response body is
// quoted as it is and not as a Python bytes literal, and an HTML parse error is
// a Go error text.
//
// An assertion reports a failure through t.Errorf and the test goes on. A
// failure of the harness stops the test through t.Fatalf: where Django's client
// raises RedirectCycleError the test stops with the same message, and where
// assertRedirects raises ValueError for a remote URL the test stops with that
// message.
//
// A panic in the handler reaches the test, as an exception of a view reaches a
// Django test.
//
// AssertURLEqual treats parameters of the same name as ordered, as Django
// documents; Django's implementation accepts them in any order.
//
// Every assertion takes the minimal TB interface, so a test of an assertion
// can pass a recording implementation in place of *testing.T, which is the
// counterpart of assertRaises(AssertionError).
//
// # Porting tests from Django
//
// Features are developed test first, and where Django has a test for the
// behavior that test is ported rather than reinvented:
//
//  1. Find the matching test in Django's tests/ directory.
//  2. Port it to a Go test function. Keep its name recognizable
//     (test_ticket1050 becomes TestTicket1050), keep its data and expected
//     values, and port the models and views it needs.
//  3. Cite the source in the doc comment of the test, in the form shown below.
//  4. Run it, watch it fail, then implement until it passes.
//
// The citation is one line per Django test, in exactly this form, so a port can
// be traced back and checked against upstream again:
//
//	// Ported from Django: tests/queries/tests.py::Queries1Tests.test_ticket1050
//
// Ported tests live in external test packages (package orm_test, not package
// orm) in files named django_<area>_test.go. The external package is required:
// djangotest imports the framework packages, so an internal test package that
// imported djangotest would form an import cycle.
//
// Where the Go API deliberately differs from Django, the port asserts the Go
// behavior and says so in the comment. A Django test for a feature that is out
// of scope is not ported; a test for a feature that is planned but missing is
// ported and skipped with t.Skip and the reason.
//
// Ported tests are derived from Django's test suite, which is distributed under
// the BSD 3-Clause license; see LICENSE-DJANGO at the repository root.
package djangotest
