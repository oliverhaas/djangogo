package admin

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"strconv"

	"github.com/oliverhaas/djangogo/auth"
	"github.com/oliverhaas/djangogo/csrf"
	"github.com/oliverhaas/djangogo/forms"
	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/urls"
)

// Routes returns the admin's URL routes: the index, the per-model changelist,
// and the form-driven add, change, and delete write views. The write routes are
// registered without a method token so each handler sees both GET and POST and
// dispatches on r.Method. Every handler is wrapped with staffRequired so only
// authenticated staff users reach it.
func (s *AdminSite) Routes() []urls.Route {
	staff := s.staffRequired
	p := s.Prefix
	return []urls.Route{
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

	rows := make([]map[string]any, len(objs))
	for i, obj := range objs {
		elem := reflect.ValueOf(obj).Elem()
		cells := make([]string, len(cols))
		for j, f := range cols {
			cells[j] = formatCell(elem.Field(f.Index))
		}
		rows[i] = map[string]any{"pk": pkOf(elem, e), "cells": cells}
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
// dropped by forms.FromModel). The kept fields are reassembled into a fresh form.
func (s *AdminSite) formFor(e *entry) *forms.Form {
	return forms.New(s.keptFields(forms.FromModel(e.model), e)...)
}

// keptFields returns f's fields with the entry's excluded and read-only field
// names removed, preserving declaration order.
func (s *AdminSite) keptFields(f *forms.Form, e *entry) []*forms.Field {
	drop := make(map[string]bool, len(e.admin.ExcludeFields)+len(e.admin.ReadonlyFields))
	for _, name := range e.admin.ExcludeFields {
		drop[name] = true
	}
	for _, name := range e.admin.ReadonlyFields {
		drop[name] = true
	}
	kept := make([]*forms.Field, 0, len(f.Fields()))
	for _, field := range f.Fields() {
		if drop[field.Name] {
			continue
		}
		kept = append(kept, field)
	}
	return kept
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
		form := s.formFor(e).Bind(r.PostForm)
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

	s.renderForm(w, r, e, "Add "+e.model.Name(), s.formFor(e))
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
		form := s.formFor(e).Bind(r.PostForm)
		if form.IsValid() {
			obj := e.ops.newPtr()
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
	// Pre-fill from the struct, then drop excluded fields the same way add does.
	// Rebuild the bound data over the kept fields so the new form re-displays the
	// existing row's values.
	prefilled := forms.FromStruct(e.model, obj)
	kept := s.keptFields(prefilled, e)
	data := url.Values{}
	for _, field := range kept {
		if v := prefilled.BoundValue(field.Name); v != "" {
			data.Set(field.Name, v)
		}
	}
	form := forms.New(kept...).Bind(data)
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

	if _, err := e.ops.get(r.Context(), pk); errors.Is(err, orm.ErrDoesNotExist) {
		http.NotFound(w, r)
		return
	} else if err != nil {
		http.Error(w, "failed to load row", http.StatusInternalServerError)
		return
	}

	s.render(w, r, "delete_confirm.html", map[string]any{
		"title":      "Delete " + e.model.Name(),
		"object":     fmt.Sprintf("%s %d", e.model.Name(), pk),
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
