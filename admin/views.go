package admin

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"

	"github.com/oliverhaas/djangogo/auth"
	"github.com/oliverhaas/djangogo/csrf"
	"github.com/oliverhaas/djangogo/forms"
	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/sessions"
	"github.com/oliverhaas/djangogo/urls"
)

// Routes returns the admin's URL routes: the login page, the index, the
// per-model changelist, and the form-driven add, change, and delete write views.
// The write routes are registered without a method token so each handler sees
// both GET and POST and dispatches on r.Method. Every handler except login is
// wrapped with staffRequired so only authenticated staff users reach it; the
// login view must stay reachable anonymously, so it is mounted unwrapped. Its
// exact path (LoginURL) takes precedence over the {model} changelist pattern
// under net/http's ServeMux specificity rules.
func (s *AdminSite) Routes() []urls.Route {
	staff := s.staffRequired
	p := s.Prefix
	return []urls.Route{
		urls.Path(s.LoginURL+"{$}", http.HandlerFunc(s.login), "admin-login"),
		urls.Path(p+"/{$}", staff(http.HandlerFunc(s.index)), "admin-index"),
		urls.Path(p+"/{model}/{$}", staff(http.HandlerFunc(s.changelist)), "admin-changelist"),
		urls.Path(p+"/{model}/add/{$}", staff(http.HandlerFunc(s.add)), "admin-add"),
		urls.Path(p+"/{model}/{pk}/change/{$}", staff(http.HandlerFunc(s.change)), "admin-change"),
		urls.Path(p+"/{model}/{pk}/delete/{$}", staff(http.HandlerFunc(s.delete)), "admin-delete"),
	}
}

// staffRequired wraps next so it runs only for an authenticated staff user.
// Anonymous or non-staff requests get a 302 to LoginURL carrying a ?next= of the
// original path.
func (s *AdminSite) staffRequired(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, ok := auth.CurrentUser(r.Context())
		if !ok || !u.IsStaff {
			target := s.LoginURL + "?next=" + url.QueryEscape(r.URL.Path)
			http.Redirect(w, r, target, http.StatusFound)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// login renders and processes the admin login form. It is mounted unwrapped by
// staffRequired so anonymous users can reach it. GET renders the form. POST looks
// up the user by username, and on a correct password for a staff account logs the
// user into the session and redirects to a safe next target (falling back to the
// admin index); any failure re-renders the form with an error.
func (s *AdminSite) login(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		s.loginPOST(w, r)
		return
	}
	s.renderLogin(w, r, safeNext(r.URL.Query().Get("next")), "")
}

// loginPOST validates submitted credentials and, on success, establishes the
// session and redirects; otherwise it re-renders the login form with an error.
func (s *AdminSite) loginPOST(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	next := safeNext(r.PostForm.Get("next"))
	username := r.PostForm.Get("username")
	password := r.PostForm.Get("password")

	user, err := orm.Query[auth.User](s.db).Get(r.Context(), "username", username)
	if err != nil || !user.CheckPassword(password) || !user.IsStaff {
		s.renderLogin(w, r, next, "Please enter a correct username and password, or you are not staff.")
		return
	}

	sess, ok := sessions.FromContext(r.Context())
	if !ok {
		http.Error(w, "no session", http.StatusInternalServerError)
		return
	}
	auth.Login(sess, &user)

	target := next
	if target == "" {
		target = s.Prefix + "/"
	}
	http.Redirect(w, r, target, http.StatusFound)
}

// renderLogin renders the login form with the optional next target and error.
func (s *AdminSite) renderLogin(w http.ResponseWriter, r *http.Request, next, errMsg string) {
	s.render(w, r, "login.html", map[string]any{
		"csrf_token": csrf.Token(r.Context()),
		"action":     s.LoginURL,
		"next":       next,
		"error":      errMsg,
		"prefix":     s.Prefix,
	})
}

// safeNext returns next when it is a safe, local path (begins with a single "/")
// and otherwise the empty string, guarding the login redirect against open
// redirects to "//host" or absolute URLs.
func safeNext(next string) string {
	if strings.HasPrefix(next, "/") && !strings.HasPrefix(next, "//") {
		return next
	}
	return ""
}

// index renders the admin landing page listing every registered model.
func (s *AdminSite) index(w http.ResponseWriter, r *http.Request) {
	list := make([]map[string]any, len(s.entries))
	for i, e := range s.entries {
		list[i] = map[string]any{"slug": e.slug, "name": e.model.Name()}
	}
	s.render(w, r, "index.html", map[string]any{
		"prefix":  s.Prefix,
		"entries": list,
	})
}

// changelist renders the read-only list of rows for the model named in the URL.
// It 404s for an unknown model.
func (s *AdminSite) changelist(w http.ResponseWriter, r *http.Request) {
	e, ok := s.bySlug[r.PathValue("model")]
	if !ok {
		http.NotFound(w, r)
		return
	}

	objs, err := e.ops.all(r.Context(), e.admin.Ordering)
	if err != nil {
		http.Error(w, "failed to load rows", http.StatusInternalServerError)
		return
	}

	cols := displayColumns(e)
	columns := make([]string, len(cols))
	for i, f := range cols {
		columns[i] = f.Name
	}

	// Pre-load foreign-key labels: one query per FK column, indexed by primary
	// key string, so each FK cell shows the related object's label (driven by its
	// String() method) rather than a raw id.
	fkLabels := make(map[int]map[string]string)
	for j, f := range cols {
		if f.Rel == nil || f.Rel.Target == nil {
			continue
		}
		opts, err := orm.LabeledRows(r.Context(), s.db, f.Rel.Target)
		if err != nil {
			continue
		}
		labels := make(map[string]string, len(opts))
		for _, o := range opts {
			labels[o[0]] = o[1]
		}
		fkLabels[j] = labels
	}

	rows := make([]map[string]any, len(objs))
	for i, obj := range objs {
		elem := reflect.ValueOf(obj).Elem()
		cells := make([]string, len(cols))
		for j, f := range cols {
			if f.Rel != nil {
				cells[j] = fkCellLabel(elem.Field(f.Index), fkLabels[j])
				continue
			}
			cells[j] = formatCell(elem.Field(f.Index))
		}
		rows[i] = map[string]any{
			"pk":    pkOf(elem, e),
			"cells": cells,
			"label": orm.Label(e.model, obj),
		}
	}

	s.render(w, r, "changelist.html", map[string]any{
		"prefix":  s.Prefix,
		"slug":    e.slug,
		"model":   e.model.Name(),
		"columns": columns,
		"rows":    rows,
	})
}

// formFor builds the editable form for an entry: the model form minus the
// admin's ExcludeFields and ReadonlyFields (the auto primary key is already
// dropped by forms.FromModel). Each foreign-key <select> is then filled with the
// related model's rows.
func (s *AdminSite) formFor(ctx context.Context, e *entry) *forms.Form {
	form := forms.FromModel(e.model, forms.WithExclude(s.excluded(e)...))
	s.populateChoices(ctx, form, e)
	return form
}

// excluded returns the field names the admin omits from a model's form: the
// configured ExcludeFields plus the ReadonlyFields.
func (s *AdminSite) excluded(e *entry) []string {
	out := make([]string, 0, len(e.admin.ExcludeFields)+len(e.admin.ReadonlyFields))
	out = append(out, e.admin.ExcludeFields...)
	out = append(out, e.admin.ReadonlyFields...)
	return out
}

// populateChoices fills every foreign-key field's <select> with the related
// model's (pk, label) rows. A load failure leaves the options empty, so a
// submitted pk fails ChoiceField validation rather than 500-ing the request.
// Relations whose field was excluded from the form (ExcludeFields/ReadonlyFields)
// are skipped, so a full-table LabeledRows query is not run for a select that
// would never be rendered.
func (s *AdminSite) populateChoices(ctx context.Context, form *forms.Form, e *entry) {
	present := make(map[string]bool, len(form.Fields()))
	for _, fld := range form.Fields() {
		present[fld.Name] = true
	}
	for _, mf := range e.model.Relations() {
		if mf.Rel == nil || mf.Rel.Target == nil || !present[mf.Name] {
			continue
		}
		opts, err := orm.LabeledRows(ctx, s.db, mf.Rel.Target)
		if err != nil {
			continue
		}
		form.SetChoices(mf.Name, opts)
	}
}

// add handles the add form: GET renders an empty form, POST validates the
// submission and creates a row before redirecting to the changelist.
func (s *AdminSite) add(w http.ResponseWriter, r *http.Request) {
	e, ok := s.bySlug[r.PathValue("model")]
	if !ok {
		http.NotFound(w, r)
		return
	}

	if r.Method == http.MethodPost {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		form := s.formFor(r.Context(), e).Bind(r.PostForm)
		if form.IsValid() {
			obj := e.ops.newPtr()
			if err := forms.PopulateStruct(e.model, form.Cleaned(), obj); err != nil {
				http.Error(w, "failed to populate object", http.StatusInternalServerError)
				return
			}
			if err := e.ops.create(r.Context(), obj); err != nil {
				http.Error(w, "failed to create row", http.StatusInternalServerError)
				return
			}
			http.Redirect(w, r, s.changelistURL(e), http.StatusFound)
			return
		}
		s.renderForm(w, r, e, "Add "+e.model.Name(), form)
		return
	}

	s.renderForm(w, r, e, "Add "+e.model.Name(), s.formFor(r.Context(), e))
}

// change handles the change form: GET renders a form pre-filled from the row,
// POST validates the submission and updates the row before redirecting.
func (s *AdminSite) change(w http.ResponseWriter, r *http.Request) {
	e, ok := s.bySlug[r.PathValue("model")]
	if !ok {
		http.NotFound(w, r)
		return
	}
	pk, err := strconv.ParseInt(r.PathValue("pk"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	if r.Method == http.MethodPost {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		form := s.formFor(r.Context(), e).Bind(r.PostForm)
		if form.IsValid() {
			obj, err := e.ops.get(r.Context(), pk)
			if errors.Is(err, orm.ErrDoesNotExist) {
				http.NotFound(w, r)
				return
			}
			if err != nil {
				http.Error(w, "failed to load row", http.StatusInternalServerError)
				return
			}
			if err := forms.PopulateStruct(e.model, form.Cleaned(), obj); err != nil {
				http.Error(w, "failed to populate object", http.StatusInternalServerError)
				return
			}
			if err := e.ops.update(r.Context(), pk, obj); err != nil {
				http.Error(w, "failed to update row", http.StatusInternalServerError)
				return
			}
			http.Redirect(w, r, s.changelistURL(e), http.StatusFound)
			return
		}
		s.renderForm(w, r, e, "Change "+e.model.Name(), form)
		return
	}

	obj, err := e.ops.get(r.Context(), pk)
	if errors.Is(err, orm.ErrDoesNotExist) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "failed to load row", http.StatusInternalServerError)
		return
	}
	// Pre-fill from the struct over the same field set add uses, so the form
	// re-displays the existing row's values.
	form := forms.FromStruct(e.model, obj, forms.WithExclude(s.excluded(e)...))
	s.populateChoices(r.Context(), form, e)
	s.renderForm(w, r, e, "Change "+e.model.Name(), form)
}

// delete handles the delete confirmation: GET renders the confirmation page,
// POST removes the row and redirects to the changelist.
func (s *AdminSite) delete(w http.ResponseWriter, r *http.Request) {
	e, ok := s.bySlug[r.PathValue("model")]
	if !ok {
		http.NotFound(w, r)
		return
	}
	pk, err := strconv.ParseInt(r.PathValue("pk"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	if r.Method == http.MethodPost {
		if err := e.ops.del(r.Context(), pk); err != nil {
			http.Error(w, "failed to delete row", http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, s.changelistURL(e), http.StatusFound)
		return
	}

	obj, err := e.ops.get(r.Context(), pk)
	if errors.Is(err, orm.ErrDoesNotExist) {
		http.NotFound(w, r)
		return
	} else if err != nil {
		http.Error(w, "failed to load row", http.StatusInternalServerError)
		return
	}

	s.render(w, r, "delete_confirm.html", map[string]any{
		"title":      "Delete " + e.model.Name(),
		"object":     orm.Label(e.model, obj),
		"csrf_token": csrf.Token(r.Context()),
		"action":     r.URL.Path,
		"prefix":     s.Prefix,
		"model":      e.slug,
	})
}

// renderForm renders the change_form template for an add or change view with the
// given title and (possibly bound) form.
func (s *AdminSite) renderForm(w http.ResponseWriter, r *http.Request, e *entry, title string, form *forms.Form) {
	s.render(w, r, "change_form.html", map[string]any{
		"title":      title,
		"form_html":  form.Render(),
		"csrf_token": csrf.Token(r.Context()),
		"action":     r.URL.Path,
		"prefix":     s.Prefix,
		"model":      e.slug,
	})
}

// changelistURL returns the URL of e's changelist.
func (s *AdminSite) changelistURL(e *entry) string {
	return s.Prefix + "/" + e.slug + "/"
}

// render writes the named template with ctx, reporting a 500 on failure.
func (s *AdminSite) render(w http.ResponseWriter, _ *http.Request, name string, ctx map[string]any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.engine.RenderTo(w, name, ctx); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}
