// Package views provides Django's view layer in Go: the shortcuts of
// django.shortcuts, the error views of django.views.defaults, the method
// decorators of django.views.decorators.http, the Paginator of
// django.core.paginator and generic views in the style of
// django.views.generic.
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
// ServeError is the single mapping from errors to responses, used by Handler
// and by the generic views:
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
// map. That is how a Page reaches a template with Django's attribute names
// (page_obj.number, page_obj.has_next, page_obj.paginator.num_pages). A loop
// over the page yields its objects, see TemplatePage:
//
//	{% for post in page_obj %}...{% endfor %}
//
// Model instances reach templates through templates.ModelContext, so
// {{ post.created_at }} reads a column and {{ post }} writes the label of the
// model. The template engine escapes string values only: {{ post }} writes the
// label without HTML escaping. For a model whose String method returns text
// from a user, write the column instead ({{ post.title }}).
//
// # Generic views
//
// Django configures a class-based view by subclassing and by keyword arguments
// to as_view(). A Go generic view is a struct value that implements
// http.Handler: a class attribute is an exported field with Django's name in
// Go case, and a method that a subclass would override is a function field.
//
//	View          dispatch by HTTP method: 405 with an Allow header for a
//	              method without handler, HEAD served by Get, OPTIONS
//	              answered with the allowed methods
//	TemplateView  render a template with the path values in the context
//	RedirectView  redirect to a URL or to a named route, 410 without target
//	ListView[T]   object_list and <model>_list, with PaginateBy also
//	              paginator, page_obj and is_paginated, default template
//	              <app>/<model>_list.html
//	DetailView[T] object and <model>, looked up by pk or slug, default
//	              template <app>/<model>_detail.html
//	FormView      form on GET, validation on POST and PUT: redirect for
//	              valid data, the form with its errors otherwise
//	CreateView[T] form for a new object, saved on valid POST and PUT data,
//	              default template <app>/<model>_form.html
//	UpdateView[T] form for an object looked up as by DetailView, saved on
//	              valid data, default template <app>/<model>_form.html
//	DeleteView[T] confirmation page on GET, delete and redirect on POST and
//	              DELETE, default template <app>/<model>_confirm_delete.html
//
// The fields, by Django attribute or method:
//
//	template_name, template_name_suffix   TemplateName, TemplateNameSuffix
//	template_name_field                   TemplateNameField
//	content_type, extra_context           ContentType, ExtraContext
//	context_object_name                   ContextObjectName
//	model                                 the type parameter T and DB
//	queryset, ordering                    Queryset, Ordering
//	allow_empty                           AllowEmpty (a *bool, nil means true)
//	paginate_by, paginate_orphans         PaginateBy, PaginateOrphans
//	page_kwarg                            PageKwarg
//	pk_url_kwarg, slug_url_kwarg          PKURLKwarg, SlugURLKwarg
//	slug_field, query_pk_and_slug         SlugField, QueryPKAndSlug
//	success_url                           SuccessURL
//	fields                                Fields ([]string{forms.AllFields}
//	                                      for "__all__")
//	form_class, get_form_class()          Form, a function that returns a
//	                                      new unbound form
//	initial, prefix                       Initial, Prefix
//	url, pattern_name                     URL, PatternName
//	permanent, query_string               Permanent, QueryString
//	preserve_request                      PreserveRequest
//	get_context_data()                    GetContextData
//	get_template_names()                  GetTemplateNames
//	get_queryset(), get_object()          GetQueryset, GetObject
//	get_paginator()                       GetPaginator
//	get_success_url()                     GetSuccessURL
//	form_valid()                          FormValid
//	success_message                       SuccessMessage
//	get_success_message()                 GetSuccessMessage
//	get_redirect_url()                    GetRedirectURL
//	delete() of the object                DeleteObject
//
// GetContextData receives the context the view has built and returns the one
// to render, the Go form of calling super().get_context_data() and updating
// the result. The values it sees are typed (T, []T, *Page[T], *Paginator[T],
// *forms.Form, *forms.ModelForm[T]). The view converts them to their template
// form after the hook.
//
// FormValid runs for valid data before a form view saves and redirects, the
// Go form of overriding form_valid and calling super().form_valid(). It can
// change the object of a model form (form.Instance()) or do the work of a
// FormView. An error it adds to the form (form.AddError) turns the request
// into form_invalid: the form is rendered again and nothing is saved. A form
// view binds the form to the body of a POST or a PUT, url-encoded or
// multipart.
//
// SuccessMessage and GetSuccessMessage take the place of Django's
// SuccessMessageMixin on FormView, CreateView, UpdateView and DeleteView. The
// message is added after the save or the deletion and before the redirect,
// so the view needs messages.Middleware.
//
// The keyword arguments of a Django URL are the path values of the route
// pattern ({pk}, {slug}, {page}), and they are strings. Mount a generic view
// on a pattern without a method ("/posts/{pk}/", not "GET /posts/{pk}/"),
// because the view answers the other methods itself. Lookups and placeholders
// use the column names of the model. A {name} placeholder in RedirectView.URL
// or in the SuccessURL of a model view is filled percent-encoded.
//
// A model view reads the model name, the default template name and the
// template form of its objects from the registry of DB, so set DB also when
// Queryset is set. The Go ORM has no Meta.ordering: a paginated ListView needs
// Ordering or an ordered Queryset for stable pages.
//
// A generic view answers a failure through ServeError: a missing object with
// the 404 page, a configuration mistake (ErrImproperlyConfigured) with a
// logged 500.
package views
