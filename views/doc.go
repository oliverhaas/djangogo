// Package views provides Django's view layer in Go: the shortcuts of
// django.shortcuts, the error views of django.views.defaults, the method
// decorators of django.views.decorators.http and generic views in the style
// of django.views.generic.
//
// # Views that return errors
//
// A Django view raises Http404 or PermissionDenied and the framework builds
// the response. The Go shape is a HandlerFunc, a handler that returns an
// error, adapted with Handler:
//
//	mux.Handle("GET /posts/{pk}/", views.Handler(engine, func(w http.ResponseWriter, r *http.Request) error {
//		post, err := views.GetObjectOr404(r.Context(), orm.Query[Post](db), "id", r.PathValue("pk"))
//		if err != nil {
//			return err
//		}
//		return views.Render(w, r, engine, "blog/post_detail.html", map[string]any{"post": post})
//	}))
//
// ServeError is the single mapping from errors to responses:
//
//	raise Http404(msg)             return views.NotFound(msg), or any error wrapping
//	                               views.ErrNotFound or orm.ErrDoesNotExist    404
//	raise PermissionDenied(msg)    return views.PermissionDenied(msg)          403
//	raise BadRequest(msg)          return views.BadRequest(msg)                400
//	raise SuspiciousOperation(msg) return views.SuspiciousOperation(msg)       400
//	any other exception            any other error                             500
//
// The body comes from the default error views NotFoundPage,
// PermissionDeniedPage, BadRequestPage and ServerErrorPage (Django's
// page_not_found, permission_denied, bad_request and server_error). They
// render 404.html, 403.html, 400.html and 500.html when the engine has the
// template and Django's default page otherwise.
//
// # Shortcuts
//
//	render(request, name, context)      Render(w, r, engine, name, ctx)
//	render(..., status=, content_type=) Render(..., WithStatus(n), WithContentType(s))
//	render(request, [names], context)   RenderFirst(w, r, engine, names, ctx)
//	render_to_string(name, context)     RenderToString(r, engine, name, ctx)
//	redirect(to, *args)                 RedirectTo(w, r, to, args...)
//	redirect(to, permanent=True)        RedirectPermanent(w, r, to, args...)
//	redirect(to, preserve_request=True) Redirector{PreserveRequest: true}.To(w, r, to)
//	resolve_url(to, *args)              ResolveURL(to, args...)
//	get_object_or_404(qs, **lookups)    GetObjectOr404(ctx, qs, pairs...)
//	get_list_or_404(qs, **lookups)      GetListOr404(ctx, qs, pairs...)
//	JsonResponse(data, status=)         JSONResponse(w, data, WithStatus(n))
//	HttpResponseNotAllowed(methods)     NotAllowed(w, r, methods...)
//	@require_http_methods([...])        RequireMethods(methods...)
//	@require_GET, @require_POST         RequireGET, RequirePOST
//	@require_safe                       RequireSafe
//
// URL names are reversed through templates.URLResolverFunc, the resolver the
// {% url %} tag uses, or through the Resolver field of a Redirector.
//
// # Template context
//
// A top-level context value that implements TemplateValuer is replaced by its
// template value before rendering. A type whose fields a template should read
// under Django's names (a form, a page of results) implements it and returns a
// map.
//
// Model instances reach templates through templates.ModelContext, so
// {{ post.created_at }} and {{ post }} work as in Django.
package views
