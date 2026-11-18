package djangotest

import (
	"github.com/oliverhaas/djangogo/urls"
)

// URLResolver resolves a path to its route. *urls.Router implements it, and
// so can an application type that delegates to its router.
type URLResolver interface {
	Resolve(path string) (*urls.ResolverMatch, error)
}

// ResolverMatch returns the route the request behind resp resolved to,
// Django's response.resolver_match. It resolves the request path again, so it
// needs the URL configuration: pass the router, or leave it out when the
// handler the client was built with is one (or implements URLResolver).
//
//	c := djangotest.NewClient(t, router)
//	m := djangotest.ResolverMatch(t, c.Get("/articles/2024/"))
//	// m.URLName == "article-year", m.Kwargs["year"] == 2024
//
// The test fails, and the result is nil, when there is no URL configuration
// to ask or the path resolves to no route (Django's Resolver404).
func ResolverMatch(t TB, resp *Response, resolver ...URLResolver) *urls.ResolverMatch {
	t.Helper()
	if resp == nil || resp.Request == nil {
		t.Fatalf("ResolverMatch needs a response that came from a Client request.")
		return nil
	}
	var r URLResolver
	switch {
	case len(resolver) > 0 && resolver[0] != nil:
		r = resolver[0]
	case resp.client != nil:
		r, _ = resp.client.handler.(URLResolver)
	}
	if r == nil {
		t.Fatalf("ResolverMatch has no URL configuration to resolve %q with: "+
			"pass the router, or build the client with a handler that implements URLResolver.",
			resp.Request.URL.Path)
		return nil
	}
	m, err := r.Resolve(resp.Request.URL.Path)
	if err != nil {
		t.Fatalf("The path %q of the request did not resolve to a route: %v", resp.Request.URL.Path, err)
		return nil
	}
	return m
}
