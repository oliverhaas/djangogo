// Package middleware holds the request and response middleware of Djan-Go-Go,
// the counterparts of the middleware Django ships in django.middleware and of
// the parts of its request handler that every project relies on.
//
// A middleware is a func(http.Handler) http.Handler built from an options
// struct. The zero value of every options struct is safe: it switches the
// optional behavior off, as the matching Django setting does when it is False,
// 0, or None. A project that wants Django's defaults has to ask for them (see
// "Defaults" below).
//
// # Mapping from Django
//
//	the MIDDLEWARE setting                   Chain
//	the exception handling of the handler    Recover (plain 500 page, technical 500 page)
//	HttpRequest.get_host and ALLOWED_HOSTS   AllowedHosts, GetHost, ValidateHost, SplitDomainPort
//	HttpRequest.get_port                     GetPort
//	HttpRequest.is_secure                    IsSecure
//	SecurityMiddleware                       Security
//	XFrameOptionsMiddleware                  XFrameOptions
//	the xframe_options_exempt decorator      ExemptXFrameOptions
//	GZipMiddleware                           GZip
//	the request log of runserver             RequestLog
//
// # Order
//
// The first argument of Chain is the outermost middleware. The recommended
// order is
//
//	middleware.Chain(
//		middleware.RequestLog(...),   // sees the final status and size of every response
//		middleware.Recover(...),      // turns a panic of anything below into a 500
//		middleware.AllowedHosts(...), // rejects a bad Host before anything trusts it
//		security,                     // the result of middleware.Security(...)
//		middleware.GZip(),
//		// sessions, CSRF, authentication, ...
//		middleware.XFrameOptions("DENY"),
//	)(handler)
//
// Three constraints are behind it. RequestLog only logs a request whose handler
// returned, so it has to be outside Recover to log a 500. Security redirects to
// the host AllowedHosts validated when AllowedHosts is outside it, and to the
// raw Host header otherwise. GZip has to be inside Recover: a response it still
// buffers when the handler panics is dropped, and Recover can answer with a
// clean error page.
//
// # Defaults
//
// Django's settings start with a few of these protections on. To get the same
// behavior set ContentTypeNosniff to true, ReferrerPolicy to "same-origin", and
// CrossOriginOpenerPolicy to "same-origin" in SecurityOptions, and use
// XFrameOptions with "DENY".
//
// # Differences from Django
//
// A header a middleware adds is added right before the response header is
// sent, and never replaces a value the handler set, as in Django.
//
// A Go request carries no SERVER_NAME and no SERVER_PORT. GetHost rejects a
// request without a host, and GetPort falls back to the default port of the
// scheme.
//
// The technical 500 page shows a Go stack trace in the place of Python frames
// with their local variables. It masks sensitive names in the GET and POST data
// as well, where Django masks them only in the request headers, the cookies,
// and the settings. For a name that is sent several times it shows all the
// values, where Django shows the last one.
//
// The response to a panic keeps the headers that were set before the panic,
// except those that describe the response that was not sent (see Recover). In
// Django that response is discarded as a whole.
package middleware
