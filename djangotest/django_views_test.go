package djangotest_test

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"io"
	"net/http"
	"strconv"
	"testing"
	"testing/fstest"

	"github.com/oliverhaas/djangogo/auth"
	"github.com/oliverhaas/djangogo/csrf"
	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/sessions"
	"github.com/oliverhaas/djangogo/templates"
	"github.com/oliverhaas/djangogo/views"
)

// The handlers in this file are a Go port of the views and URL configurations
// that Django's own test client suites run against:
//
//	tests/test_client/views.py and tests/test_client/urls.py
//	tests/test_client_regress/views.py and tests/test_client_regress/urls.py
//
// Django builds most of these templates inline with
// Template("...", name="GET Template"). Here they are files of an in-memory
// template directory under the same names, so the template assertions of the
// ported tests keep their arguments. Two Django views reuse the name
// "GET Template" for different sources; the second one is "Success Template"
// here.

// clientTemplates is the template directory of the ported views.
var clientTemplates = fstest.MapFS{
	"GET Template":                    {Data: []byte("This is a test. {{ var }} is the value.")},
	"TRACE Template":                  {Data: []byte("{{ method }} {{ uri }} {{ version }}")},
	"PUT Template":                    {Data: []byte("Data received: {{ data }} is the body.")},
	"Empty GET Template":              {Data: []byte("Viewing GET page.")},
	"POST Template":                   {Data: []byte("Data received: {{ data }} is the value.")},
	"Empty POST Template":             {Data: []byte("Viewing POST page.")},
	"Success Template":                {Data: []byte("The value of success is {{ value }}.")},
	"JSON Template":                   {Data: []byte("Viewing {{ method }} page. With data {{ data }}.")},
	"Book template":                   {Data: []byte("{{ title }} - {{ author }}")},
	"Book GET template":               {Data: []byte("GET request.")},
	"Login Template":                  {Data: []byte("This is a login protected test. Username is {{ user.Username }}.")},
	"Session Modifying View Template": {Data: []byte("This is a view that modifies the session.")},
	"login.html":                      {Data: []byte(`<form method="post">{% csrf_token %}<input name="username"></form>`)},
	"base.html":                       {Data: []byte("base template")},
	"unicode.html": {Data: []byte("* 峠 (とうげ tōge \"mountain pass\")\n" +
		"* 榊 (さかき sakaki \"tree, genus Cleyera\")\n")},
}

// loginURL is where the login-protected views send anonymous users, Django's
// default LOGIN_URL.
const loginURL = "/accounts/login/"

// site is the application under test: the ported views behind the session,
// CSRF, and authentication middleware, with the database and session store the
// tests reach behind the handler.
type site struct {
	handler http.Handler
	db      *orm.DB
	store   sessions.Store
	engine  *templates.Engine
}

// newSite builds the application under test on a fresh database. withCSRF adds
// the CSRF middleware, as CSRFEnabledClientTests does through its settings.
func newSite(t *testing.T, withCSRF bool) *site {
	t.Helper()
	engine, err := templates.NewEngineFS(clientTemplates)
	if err != nil {
		t.Fatalf("NewEngineFS: %v", err)
	}
	db := djangotest.NewDB(t, auth.App{}, &sessions.Record{})
	s := &site{db: db, store: sessions.NewDBStore(db), engine: engine}

	handler := auth.Middleware(db)(s.routes())
	if withCSRF {
		handler = csrf.Middleware(handler)
	}
	s.handler = sessions.Middleware(s.store, djangotest.DefaultSessionCookie)(handler)
	return s
}

// client returns a test client wired to the site the way Django's self.client
// is wired to the project under test.
func (s *site) client(t djangotest.TB, opts ...djangotest.ClientOption) *djangotest.Client {
	t.Helper()
	opts = append([]djangotest.ClientOption{
		djangotest.WithDB(s.db),
		djangotest.WithSessions(s.store, ""),
	}, opts...)
	return djangotest.NewClient(t, s.handler, opts...)
}

// users are the accounts ClientTest.setUpTestData creates.
type users struct {
	active   *auth.User
	inactive *auth.User
}

// createUsers ports ClientTest.setUpTestData: an active user "testclient" and
// an inactive user "inactive", both with the password "password". It is called
// only by the tests that log in, because hashing a password is slow by design.
func (s *site) createUsers(t *testing.T) users {
	t.Helper()
	ctx := context.Background()
	active, err := auth.CreateUser(ctx, s.db, "testclient", "", "password", false, false, true)
	if err != nil {
		t.Fatalf("create testclient: %v", err)
	}
	inactive, err := auth.CreateUser(ctx, s.db, "inactive", "", "password", false, false, false)
	if err != nil {
		t.Fatalf("create inactive: %v", err)
	}
	return users{active: active, inactive: inactive}
}

// routes is the port of both urls.py files. A pattern ending in {$} matches
// exactly that path, as a Django path() does.
func (s *site) routes() http.Handler {
	mux := http.NewServeMux()
	loginRequired := auth.LoginRequired(loginURL)

	// tests/test_client/urls.py
	mux.HandleFunc("/get_view/{$}", s.getView)
	mux.HandleFunc("/post_view/{$}", s.postView)
	mux.HandleFunc("/post_then_get_view/{$}", s.postThenGetView)
	mux.HandleFunc("/put_view/{$}", s.putView)
	mux.HandleFunc("/trace_view/{$}", s.traceView)
	mux.HandleFunc("/header_view/{$}", viewWithHeader)
	mux.HandleFunc("/raw_post_view/{$}", s.rawPostView)
	mux.HandleFunc("/redirect_view/{$}", redirectView)
	mux.HandleFunc("/redirect_view_307/{$}", postViewRedirect(http.StatusTemporaryRedirect))
	mux.HandleFunc("/redirect_view_307_query_string/{$}", redirectTo("/post_view/?hello=world", http.StatusTemporaryRedirect))
	mux.HandleFunc("/redirect_view_308/{$}", postViewRedirect(http.StatusPermanentRedirect))
	mux.HandleFunc("/redirect_view_308_query_string/{$}", redirectTo("/post_view/?hello=world", http.StatusPermanentRedirect))
	mux.HandleFunc("/redirect_to_different_hostname/{$}", redirectTo("https://hostname2/get_host_view/", http.StatusFound))
	mux.HandleFunc("/get_host_view/{$}", getHostView)
	mux.HandleFunc("/secure_view/{$}", viewWithSecure)
	mux.HandleFunc("/permanent_redirect_view/{$}", redirectTo("/get_view/", http.StatusMovedPermanently))
	mux.HandleFunc("/temporary_redirect_view/{$}", redirectTo("/get_view/", http.StatusFound))
	mux.HandleFunc("/http_redirect_view/{$}", redirectTo("/secure_view/", http.StatusFound))
	mux.HandleFunc("/https_redirect_view/{$}", redirectTo("https://testserver/secure_view/", http.StatusFound))
	mux.HandleFunc("/double_redirect_view/{$}", redirectTo("/permanent_redirect_view/", http.StatusFound))
	mux.HandleFunc("/bad_view/{$}", badView)
	mux.HandleFunc("/json_view/{$}", s.jsonView)
	mux.Handle("/login_protected_view/{$}", loginRequired(http.HandlerFunc(s.loginProtectedView)))
	mux.HandleFunc("/session_view/{$}", s.sessionView)
	mux.HandleFunc("/broken_view/{$}", brokenView)
	mux.HandleFunc("/django_project_redirect/{$}", redirectTo("https://www.djangoproject.com/", http.StatusFound))
	mux.HandleFunc("/no_trailing_slash_external_redirect/{$}", redirectTo("https://testserver", http.StatusFound))
	mux.HandleFunc("/{$}", indexView)
	mux.HandleFunc("/accounts/{$}", redirectTo("login/", http.StatusFound))
	mux.HandleFunc("/accounts/no_trailing_slash", redirectTo("login/", http.StatusFound))
	mux.HandleFunc("/accounts/login/{$}", s.loginView)

	// tests/test_client_regress/urls.py
	mux.HandleFunc("/no_template_view/{$}", noTemplateView)
	mux.HandleFunc("/redirects/{$}", redirectTo("/redirects/further/", http.StatusFound))
	mux.HandleFunc("/redirects/further/{$}", redirectTo("/redirects/further/more/", http.StatusFound))
	mux.HandleFunc("/redirects/further/more/{$}", redirectTo("/no_template_view/", http.StatusFound))
	mux.HandleFunc("/redirect_to_non_existent_view/{$}", redirectTo("/non_existent_view/", http.StatusFound))
	mux.HandleFunc("/redirect_to_non_existent_view2/{$}", redirectTo("/redirect_to_non_existent_view/", http.StatusFound))
	mux.HandleFunc("/redirect_to_self/{$}", redirectTo("/redirect_to_self/", http.StatusFound))
	mux.HandleFunc("/redirect_to_self_with_changing_query_view/{$}", redirectToSelfWithChangingQueryView)
	mux.HandleFunc("/circular_redirect_1/{$}", redirectTo("/circular_redirect_2/", http.StatusFound))
	mux.HandleFunc("/circular_redirect_2/{$}", redirectTo("/circular_redirect_3/", http.StatusFound))
	mux.HandleFunc("/circular_redirect_3/{$}", redirectTo("/circular_redirect_1/", http.StatusFound))
	mux.HandleFunc("/redirect_other_host/{$}", redirectTo("https://otherserver:8443/no_template_view/", http.StatusFound))
	mux.HandleFunc("/redirect_based_on_extra_headers_1/{$}", redirectOnHeader("/redirect_based_on_extra_headers_2/"))
	mux.HandleFunc("/redirect_based_on_extra_headers_2/{$}", redirectOnHeader("/redirects/further/more/"))
	mux.HandleFunc("/set_session/{$}", setSessionView)
	mux.HandleFunc("/check_session/{$}", checkSessionView)
	mux.HandleFunc("/request_methods/{$}", requestMethodsView)
	mux.HandleFunc("/check_unicode/{$}", s.returnUnicode)
	mux.HandleFunc("/json_response/{$}", returnJSONResponse)
	mux.HandleFunc("/check_headers/{$}", checkHeaders)
	mux.HandleFunc("/check_headers_redirect/{$}", redirectTo("/check_headers/", http.StatusFound))
	mux.HandleFunc("/body/{$}", bodyView)
	mux.HandleFunc("/render_template_multiple_times/{$}", s.renderTemplateMultipleTimes)
	return mux
}

// render renders a template for the request, the counterpart of
// HttpResponse(t.render(c)) in the Django views.
//
// A template that fails to render is a bug of this fixture, so it panics
// instead of answering 500.
func (s *site) render(w http.ResponseWriter, r *http.Request, name string, ctx map[string]any) {
	if err := views.Render(w, r, s.engine, name, ctx); err != nil {
		panic(err)
	}
}

// redirectTo answers with a redirect whose Location header is exactly
// location, as HttpResponseRedirect and RedirectView do. (http.Redirect would
// rewrite a relative location into an absolute path, and following a relative
// location is one of the things under test.)
func redirectTo(location string, status int) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", location)
		w.WriteHeader(status)
	}
}

// getView expects a GET request and returns a rendered template.
func (s *site) getView(w http.ResponseWriter, r *http.Request) {
	var value any = 42
	if query := r.URL.Query(); query.Has("var") {
		value = query.Get("var")
	}
	s.render(w, r, "GET Template", map[string]any{"var": value})
}

// traceView expects a TRACE request and echoes its status line. A TRACE request
// must not have an entity; the view answers 400 if it has one.
func (s *site) traceView(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodTrace {
		w.Header().Set("Allow", http.MethodTrace)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if body, _ := io.ReadAll(r.Body); len(body) > 0 {
		http.Error(w, "TRACE requests MUST NOT include an entity", http.StatusBadRequest)
		return
	}
	s.render(w, r, "TRACE Template", map[string]any{
		"method":  r.Method,
		"uri":     r.URL.Path,
		"version": r.Proto,
	})
}

// putView echoes the body of a PUT request. Django names the context variable
// "Content-Length"; a pongo2 context key must be an identifier, so it is
// content_length here.
func (s *site) putView(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		s.render(w, r, "Empty GET Template", nil)
		return
	}
	body, _ := io.ReadAll(r.Body)
	s.render(w, r, "PUT Template", map[string]any{
		"content_length": strconv.FormatInt(r.ContentLength, 10),
		"data":           string(body),
	})
}

// postView expects a POST and renders a different template depending on
// whether any POST data is available.
func (s *site) postView(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.render(w, r, "Empty GET Template", nil)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if len(r.PostForm) == 0 {
		s.render(w, r, "Empty POST Template", nil)
		return
	}
	s.render(w, r, "POST Template", map[string]any{"data": r.PostForm.Get("value")})
}

// postThenGetView expects a POST and redirects to itself with only a
// ?success=true query string, whose value it renders on GET.
func (s *site) postThenGetView(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		redirectTo("?success=true", http.StatusFound)(w, r)
		return
	}
	value := "false"
	if query := r.URL.Query(); query.Has("success") {
		value = query.Get("success")
	}
	s.render(w, r, "Success Template", map[string]any{"value": value})
}

// jsonView expects a request with the content type application/json and JSON
// data, which it decodes and puts in the context.
func (s *site) jsonView(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Content-Type") != djangotest.JSONContentType {
		return
	}
	var data any
	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	s.render(w, r, "JSON Template", map[string]any{"method": r.Method, "data": data})
}

// viewWithHeader answers with a custom header.
func viewWithHeader(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("X-DJANGO-TEST", "Slartibartfast")
}

// library is the XML document rawPostView expects.
type library struct {
	Books []struct {
		Title  string `xml:"title"`
		Author string `xml:"author"`
	} `xml:"book"`
}

// rawPostView expects raw XML to be posted and returns content extracted from
// it.
func (s *site) rawPostView(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.render(w, r, "Book GET template", nil)
		return
	}
	var doc library
	if err := xml.NewDecoder(r.Body).Decode(&doc); err != nil || len(doc.Books) == 0 {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	s.render(w, r, "Book template", map[string]any{
		"title":  doc.Books[0].Title,
		"author": doc.Books[0].Author,
	})
}

// redirectView redirects every request to the GET view, keeping the query
// string.
func redirectView(w http.ResponseWriter, r *http.Request) {
	query := ""
	if values := r.URL.Query(); len(values) > 0 {
		query = "?" + values.Encode()
	}
	redirectTo("/get_view/"+query, http.StatusFound)(w, r)
}

// postViewRedirect redirects to /post_view/ (or the "to" query parameter) with
// the given status code.
func postViewRedirect(status int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		to := "/post_view/"
		if query := r.URL.Query(); query.Has("to") {
			to = query.Get("to")
		}
		redirectTo(to, status)(w, r)
	}
}

// getHostView answers with the host the request was addressed to.
func getHostView(w http.ResponseWriter, r *http.Request) {
	_, _ = io.WriteString(w, r.Host)
}

// secureHeader carries what Django's view_with_secure stores on the response
// object as test_was_secure_request.
const secureHeader = "X-Test-Was-Secure-Request"

// viewWithSecure indicates whether the request was secure.
func viewWithSecure(w http.ResponseWriter, r *http.Request) {
	w.Header().Set(secureHeader, strconv.FormatBool(r.TLS != nil))
}

// badView answers 404 with some error content.
func badView(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusNotFound)
	_, _ = io.WriteString(w, "Not found!. This page contains some MAGIC content")
}

// loginProtectedView is a simple view that is login protected.
func (s *site) loginProtectedView(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.CurrentUser(r.Context())
	s.render(w, r, "Login Template", map[string]any{"user": user})
}

// loginView stands in for auth_views.LoginView: the tests only need the login
// page to exist, since the client logs in through Client.Login.
func (s *site) loginView(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "login.html", nil)
}

// sessionView modifies the session.
func (s *site) sessionView(w http.ResponseWriter, r *http.Request) {
	if sess, ok := sessions.FromContext(r.Context()); ok {
		sess.Set("tobacconist", "hovercraft")
	}
	s.render(w, r, "Session Modifying View Template", nil)
}

// errBrokenView is what brokenView fails with.
var errBrokenView = errors.New("oops, looks like you wrote some bad code")

// brokenView simulates a broken view, as Django's does by raising KeyError.
func brokenView(http.ResponseWriter, *http.Request) {
	panic(errBrokenView)
}

// indexView is the target of no_trailing_slash_external_redirect.
func indexView(w http.ResponseWriter, _ *http.Request) {
	_, _ = io.WriteString(w, "Hello world")
}

// noTemplateView answers without rendering a template.
func noTemplateView(w http.ResponseWriter, _ *http.Request) {
	_, _ = io.WriteString(w, "No template used. Sample content: twice once twice. Content ends.")
}

// redirectToSelfWithChangingQueryView redirects to itself with an incremented
// counter in the query string.
func redirectToSelfWithChangingQueryView(w http.ResponseWriter, r *http.Request) {
	counter, _ := strconv.Atoi(r.URL.Query().Get("counter"))
	location := "/redirect_to_self_with_changing_query_view/?counter=" + strconv.Itoa(counter+1)
	redirectTo(location, http.StatusFound)(w, r)
}

// redirectOnHeader redirects to location when the request carries a Redirect
// header and answers 200 otherwise.
func redirectOnHeader(location string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Redirect") != "" {
			redirectTo(location, http.StatusFound)(w, r)
		}
	}
}

// setSessionView sets a session variable.
func setSessionView(w http.ResponseWriter, r *http.Request) {
	if sess, ok := sessions.FromContext(r.Context()); ok {
		sess.Set("session_var", "YES")
	}
	_, _ = io.WriteString(w, "set_session")
}

// checkSessionView reads a session variable.
func checkSessionView(w http.ResponseWriter, r *http.Request) {
	value := "NO"
	if sess, ok := sessions.FromContext(r.Context()); ok {
		if v, found := sess.Get("session_var"); found {
			value, _ = v.(string)
		}
	}
	_, _ = io.WriteString(w, value)
}

// requestMethodsView answers with the request method.
func requestMethodsView(w http.ResponseWriter, r *http.Request) {
	_, _ = io.WriteString(w, "request method: "+r.Method)
}

// returnUnicode renders a template holding non-ASCII text.
func (s *site) returnUnicode(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "unicode.html", nil)
}

// returnJSONResponse answers with a JSON document, under the content type
// named by the content_type query parameter when there is one.
func returnJSONResponse(w http.ResponseWriter, r *http.Request) {
	contentType := djangotest.JSONContentType
	if query := r.URL.Query(); query.Has("content_type") {
		contentType = query.Get("content_type")
	}
	w.Header().Set("Content-Type", contentType)
	_, _ = io.WriteString(w, `{"key": "value"}`)
}

// checkHeaders answers with the value of the X-Arg-Check header.
func checkHeaders(w http.ResponseWriter, r *http.Request) {
	value := "Undefined"
	if v := r.Header.Get("X-Arg-Check"); v != "" {
		value = v
	}
	_, _ = io.WriteString(w, "HTTP_X_ARG_CHECK: "+value)
}

// bodyView echoes the request body.
func bodyView(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = io.Copy(w, r.Body)
}

// renderTemplateMultipleTimes renders the same template twice.
func (s *site) renderTemplateMultipleTimes(w http.ResponseWriter, r *http.Request) {
	for range 2 {
		out, err := s.engine.RenderRequest(r, "base.html", nil)
		if err != nil {
			http.Error(w, "template error", http.StatusInternalServerError)
			return
		}
		_, _ = io.WriteString(w, out)
	}
}
