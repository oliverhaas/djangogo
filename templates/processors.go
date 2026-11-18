package templates

import (
	"net"
	"net/http"
	"slices"

	"github.com/oliverhaas/djangogo/urls"
)

// RequestProcessor is Django's context processor "request": it adds the
// variable request, a view of the current request with Django's attribute
// names:
//
//	request.path             /articles/2024/
//	request.path_info        the same (there is no script prefix)
//	request.get_full_path    /articles/2024/?page=2
//	request.method           GET
//	request.scheme           http or https
//	request.is_secure        whether the request came over TLS
//	request.get_host         the Host header
//	request.GET.page         a query parameter (the last value given)
//	request.POST.title       a form parameter (the last value given)
//	request.COOKIES.name     a cookie value
//	request.headers.Accept   a header, by its canonical name
//	request.resolver_match   url_name, view_name, app_name, namespace,
//	                         route, kwargs and args of the matched route
//
// Add it with Engine.AddContextProcessor; it runs for RenderRequest.
func RequestProcessor(r *http.Request) map[string]any {
	return map[string]any{"request": requestView(r)}
}

// requestView builds the template view of r. The parts that cost something to
// compute are functions, which a template calls by naming them.
func requestView(r *http.Request) map[string]any {
	secure := r.TLS != nil
	scheme := "http"
	if secure {
		scheme = "https"
	}
	return map[string]any{
		"path":          r.URL.Path,
		"path_info":     r.URL.Path,
		"method":        r.Method,
		"scheme":        scheme,
		"is_secure":     secure,
		"get_host":      r.Host,
		"get_full_path": func() string { return fullPath(r) },
		"GET":           func() map[string]string { return lastValues(r.URL.Query()) },
		"POST":          func() map[string]string { return postValues(r) },
		"COOKIES":       func() map[string]string { return cookieValues(r) },
		"headers":       func() map[string]string { return lastValues(r.Header) },
		"resolver_match": func() map[string]any {
			return resolverMatchView(urls.Match(r))
		},
	}
}

// fullPath returns the path with the query string, Django's get_full_path.
func fullPath(r *http.Request) string {
	path := r.URL.EscapedPath()
	if r.URL.RawQuery != "" {
		path += "?" + r.URL.RawQuery
	}
	return path
}

// lastValues flattens a multi-value map to the last value of each key, which
// is what a Django QueryDict gives for a plain key lookup.
func lastValues(values map[string][]string) map[string]string {
	out := make(map[string]string, len(values))
	for k, v := range values {
		if len(v) > 0 {
			out[k] = v[len(v)-1]
		}
	}
	return out
}

// postValues returns the form parameters of the request body. A body the
// handler did not parse is parsed here.
func postValues(r *http.Request) map[string]string {
	if r.PostForm == nil {
		if err := r.ParseForm(); err != nil {
			return map[string]string{}
		}
	}
	return lastValues(r.PostForm)
}

// cookieValues returns the cookies of the request by name.
func cookieValues(r *http.Request) map[string]string {
	cookies := r.Cookies()
	out := make(map[string]string, len(cookies))
	for _, c := range cookies {
		out[c.Name] = c.Value
	}
	return out
}

// resolverMatchView returns the template view of a resolver match, nil when
// the request did not come through a router.
func resolverMatchView(m *urls.ResolverMatch) map[string]any {
	if m == nil {
		return nil
	}
	return map[string]any{
		"url_name":   m.URLName,
		"view_name":  m.ViewName,
		"app_name":   m.AppName(),
		"app_names":  m.AppNames,
		"namespace":  m.Namespace(),
		"namespaces": m.Namespaces,
		"route":      m.Route,
		"kwargs":     map[string]any(m.Kwargs),
		"args":       m.Args,
	}
}

// StaticProcessor is Django's context processor "static": it adds STATIC_URL,
// the prefix {% static %} uses (StaticBase).
func StaticProcessor(*http.Request) map[string]any {
	return map[string]any{"STATIC_URL": StaticBase}
}

// DebugProcessor returns Django's context processor "debug". It adds
// debug = true when debug is on and the request comes from one of
// internalIPs (Django's INTERNAL_IPS), and nothing otherwise, so a template
// can show diagnostics to developers only. Django's sql_queries is not
// provided.
func DebugProcessor(debug bool, internalIPs ...string) ContextProcessor {
	return func(r *http.Request) map[string]any {
		if !debug || !slices.Contains(internalIPs, remoteIP(r)) {
			return nil
		}
		return map[string]any{"debug": true}
	}
}

// remoteIP returns the address of the peer without the port.
func remoteIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
