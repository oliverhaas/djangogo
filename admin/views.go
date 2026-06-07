package admin

import (
	"fmt"
	"net/http"
	"net/url"
	"reflect"

	"github.com/oliverhaas/djangogo/auth"
	"github.com/oliverhaas/djangogo/urls"
)

// Routes returns the admin's URL routes: the index, the per-model changelist,
// and the add, change, and delete write views (the latter three render
// placeholders here and are fleshed out in a follow-up). Every handler is
// wrapped with staffRequired so only authenticated staff users reach it.
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

// add renders the add form. Placeholder for the follow-up package.
func (s *AdminSite) add(w http.ResponseWriter, r *http.Request) {
	s.notImplemented(w, r)
}

// change renders the change form. Placeholder for the follow-up package.
func (s *AdminSite) change(w http.ResponseWriter, r *http.Request) {
	s.notImplemented(w, r)
}

// delete renders the delete confirmation. Placeholder for the follow-up package.
func (s *AdminSite) delete(w http.ResponseWriter, r *http.Request) {
	s.notImplemented(w, r)
}

// notImplemented serves a 200 placeholder for the write views that the follow-up
// package implements.
func (s *AdminSite) notImplemented(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.bySlug[r.PathValue("model")]; !ok {
		http.NotFound(w, r)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprint(w, "not implemented yet")
}

// render writes the named template with ctx, reporting a 500 on failure.
func (s *AdminSite) render(w http.ResponseWriter, _ *http.Request, name string, ctx map[string]any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.engine.RenderTo(w, name, ctx); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}
