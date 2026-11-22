package admin

import (
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"slices"
	"strings"

	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/views"
)

// autocompletePerPage is the number of results of one page of the
// autocomplete view, Django's AutocompleteJsonView.paginate_by.
const autocompletePerPage = 20

// autocompleteResult is one object of the autocomplete view: its primary key
// and its label.
type autocompleteResult struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

// autocompletePagination tells the script whether a next page exists.
type autocompletePagination struct {
	More bool `json:"more"`
}

// autocompleteResponse is the body of the autocomplete view.
type autocompleteResponse struct {
	Results    []autocompleteResult   `json:"results"`
	Pagination autocompletePagination `json:"pagination"`
}

// autocompleteURL returns the URL of the site's autocomplete view.
func (s *AdminSite) autocompleteURL() string {
	if u := s.reverse("autocomplete"); u != "" {
		return u
	}
	return s.prefix() + "/autocomplete/"
}

// autocompleteView answers the requests of the autocomplete widgets, Django's
// AutocompleteJsonView: the objects of the related model of the field named
// by the app_label, model_name and field_name parameters that match the
// search term, 20 per page, as
// {"results": [{"id": "1", "text": "label"}], "pagination": {"more": false}}.
// A request for a field that is no relation to a model with an admin on the
// site, or by a user without the view or change permission on that model, is
// forbidden. A related admin without search fields answers 404.
func (s *AdminSite) autocompleteView(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if !q.Has("app_label") || !q.Has("model_name") || !q.Has("field_name") {
		forbidden(w)
		return
	}
	source := s.modelByName(q.Get("app_label"), q.Get("model_name"))
	if source == nil {
		forbidden(w)
		return
	}
	f := relationFieldNamed(source, q.Get("field_name"))
	if f == nil {
		forbidden(w)
		return
	}
	target := s.entryByModel(f.Rel.Target)
	if target == nil {
		forbidden(w)
		return
	}
	searchFields := target.searchFields(r)
	if len(searchFields) == 0 {
		http.Error(w, fmt.Sprintf("ModelAdmin of %s must have search_fields for the autocomplete_view.",
			target.model.Label()), http.StatusNotFound)
		return
	}
	if !target.hasViewOrChangePermission(s, r, nil) {
		forbidden(w)
		return
	}

	var spec querySpec
	if target.admin.GetQuerysetFilter != nil {
		if c := target.admin.GetQuerysetFilter(r); c.String() != "(AND: )" {
			spec.filters = append(spec.filters, c)
		}
	}
	spec.filters = append(spec.filters, f.Rel.LimitChoicesTo())
	if cond, distinct, ok := buildSearchCond(target.model, searchFields, q.Get("term")); ok {
		spec.filters = append(spec.filters, cond)
		spec.distinct = distinct
	}
	spec.ordering = s.relatedOrdering(r, target.model)
	if len(spec.ordering) == 0 && len(target.model.Ordering()) == 0 {
		spec.ordering = []string{"pk"}
	}

	pag := views.NewPaginator[any](&changelistObjectList{ops: target.ops, spec: spec}, autocompletePerPage)
	page, err := autocompletePage(r, pag, q.Get("page"))
	if errors.Is(err, views.ErrInvalidPage) {
		http.Error(w, "Invalid page.", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "failed to load results", http.StatusInternalServerError)
		return
	}
	body := autocompleteResponse{Results: make([]autocompleteResult, 0, len(page.ObjectList))}
	for _, obj := range page.ObjectList {
		body.Results = append(body.Results, autocompleteResult{
			ID:   fmt.Sprint(reflect.ValueOf(obj).Elem().Field(target.model.PrimaryKey().Index).Interface()),
			Text: orm.Label(target.model, obj),
		})
	}
	body.Pagination.More = page.HasNext()
	if err := views.JSONResponse(w, body); err != nil {
		http.Error(w, "failed to write results", http.StatusInternalServerError)
	}
}

// autocompletePage returns the page of pag that the page parameter number
// selects, as Django's MultipleObjectMixin.paginate_queryset: the first page
// when it is empty, the last page for "last".
func autocompletePage(r *http.Request, pag *views.Paginator[any], number string) (*views.Page[any], error) {
	ctx := r.Context()
	if number == "last" {
		n, err := pag.NumPages(ctx)
		if err != nil {
			return nil, err
		}
		return pag.Page(ctx, n)
	}
	if number == "" {
		return pag.Page(ctx, 1)
	}
	n, err := pag.ValidateNumber(ctx, number)
	if err != nil {
		return nil, err
	}
	return pag.Page(ctx, n)
}

// modelByName returns the model of the database with the app label app and
// the name model, compared without case as Django's apps.get_model does, or
// nil.
func (s *AdminSite) modelByName(app, model string) *orm.Model {
	for _, m := range s.db.Registry().AppModels(app) {
		if strings.EqualFold(m.Name(), model) {
			return m
		}
	}
	return nil
}

// relationFieldNamed returns the foreign key, one-to-one or many-to-many
// field of m whose Go name or attribute name is name, or nil.
func relationFieldNamed(m *orm.Model, name string) *orm.Field {
	for _, f := range slices.Concat(m.Relations(), m.ManyToMany()) {
		if f.Rel != nil && f.Rel.Target != nil && (f.Name == name || f.Attr == name) {
			return f
		}
	}
	return nil
}
