package views

import (
	"fmt"
	"net/http"
	"net/url"

	"github.com/oliverhaas/djangogo/templates"
)

// TemplateView renders a template, Django's TemplateView. It answers GET and
// HEAD. The context holds the path values of the route under their wildcard
// names (as strings), the view under "view", and ExtraContext.
//
//	mux.Handle("/about/", views.TemplateView{Engine: engine, TemplateName: "about.html"})
type TemplateView struct {
	// Engine renders the template and the error templates.
	Engine *templates.Engine
	// TemplateName is the template to render (template_name). It is required
	// unless GetTemplateNames is set.
	TemplateName string
	// ContentType overrides the Content-Type header (content_type).
	ContentType string
	// ExtraContext is added to the context (extra_context).
	ExtraContext map[string]any
	// GetContextData replaces overriding get_context_data.
	GetContextData ContextFunc
	// GetTemplateNames replaces overriding get_template_names. It returns the
	// candidate names, of which the first existing one is rendered.
	GetTemplateNames func(r *http.Request) ([]string, error)
}

// ServeHTTP renders the template for GET and HEAD.
func (v TemplateView) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	dispatch(w, r, v.Engine, methodHandlers{get: v.get})
}

// get renders the template with the path values as context.
func (v TemplateView) get(w http.ResponseWriter, r *http.Request) error {
	base := make(map[string]any)
	for _, pv := range pathValues(r) {
		base[pv.name] = pv.value
	}
	ctx, err := viewContext(r, v, base, v.ExtraContext, v.GetContextData)
	if err != nil {
		return err
	}
	var names []string
	if v.GetTemplateNames != nil {
		names, err = v.GetTemplateNames(r)
	} else {
		names, err = baseTemplateNames("TemplateView", v.TemplateName)
	}
	if err != nil {
		return err
	}
	return renderResponse(w, r, v.Engine, names, ctx, contentTypeOption(v.ContentType))
}

// RedirectView redirects every GET, HEAD, POST, PUT, PATCH, DELETE and OPTIONS
// request, Django's RedirectView. The target is URL, or the route named
// PatternName. Without a target the view answers 410 Gone.
//
//	mux.Handle("/old/{id}/", views.RedirectView{URL: "/new/{id}/", Permanent: true})
type RedirectView struct {
	// Engine renders the error templates. It may be nil.
	Engine *templates.Engine
	// URL is the target (url). A {name} placeholder is replaced by the path
	// value of that name, where Django writes %(name)s. The value is
	// percent-encoded, so it stays one path segment or one query value.
	URL string
	// PatternName is the name of the route to redirect to when URL is empty
	// (pattern_name). It is reversed with the path values of the request, in
	// the order of the route pattern.
	PatternName string
	// Permanent selects 301 instead of 302 (permanent).
	Permanent bool
	// QueryString appends the query string of the request to the target
	// (query_string).
	QueryString bool
	// PreserveRequest selects 307, or 308 with Permanent, so the client
	// repeats the request with its method and body (preserve_request).
	PreserveRequest bool
	// Resolver reverses PatternName. Nil uses templates.URLResolverFunc.
	Resolver templates.Resolver
	// GetRedirectURL replaces overriding get_redirect_url. It returns the
	// complete target, and "" for 410 Gone.
	GetRedirectURL func(r *http.Request) (string, error)
}

// ServeHTTP redirects the request.
func (v RedirectView) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	dispatch(w, r, v.Engine, methodHandlers{
		get: v.redirect, post: v.redirect, put: v.redirect, patch: v.redirect,
		delete: v.redirect, head: v.redirect, options: v.redirect,
	})
}

// redirect answers with the redirect, or with 410 Gone when there is no
// target.
func (v RedirectView) redirect(w http.ResponseWriter, r *http.Request) error {
	target, err := v.redirectURL(r)
	if err != nil {
		return err
	}
	if target == "" {
		logResponse(r, http.StatusGone, "Gone: "+escapeLogValue(r.URL.Path))
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusGone)
		return nil
	}
	rd := Redirector{Permanent: v.Permanent, PreserveRequest: v.PreserveRequest}
	return rd.redirect(w, r, target)
}

// redirectURL returns the target of the redirect, Django's get_redirect_url,
// or "" when the view has none.
func (v RedirectView) redirectURL(r *http.Request) (string, error) {
	if v.GetRedirectURL != nil {
		return v.GetRedirectURL(r)
	}
	values := pathValues(r)
	var target string
	switch {
	case v.URL != "":
		filled, err := formatURL(v.URL, func(name string) (any, bool) {
			for _, pv := range values {
				if pv.name == name {
					return pv.value, true
				}
			}
			return nil, false
		})
		if err != nil {
			return "", err
		}
		target = filled
	case v.PatternName != "":
		args := make([]any, len(values))
		for i, pv := range values {
			args[i] = pv.value
		}
		reversed, err := v.reverse(args)
		if err != nil {
			return "", err
		}
		target = reversed
	default:
		return "", nil
	}
	if query := r.URL.RawQuery; query != "" && v.QueryString {
		separator := "?"
		if u, err := url.Parse(target); err == nil && u.RawQuery != "" {
			separator = "&"
		}
		target += separator + query
	}
	return target, nil
}

// reverse returns the URL of the route PatternName for args.
func (v RedirectView) reverse(args []any) (string, error) {
	resolver := v.Resolver
	if resolver == nil {
		resolver = templates.URLResolverFunc()
	}
	if resolver == nil {
		return "", improperlyConfigured("RedirectView has no URL resolver to reverse %q", v.PatternName)
	}
	target, err := resolver(v.PatternName, args...)
	if err != nil {
		return "", fmt.Errorf("views: RedirectView cannot reverse %q: %w", v.PatternName, err)
	}
	return target, nil
}
