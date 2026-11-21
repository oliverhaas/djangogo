package admin

import (
	"context"
	"errors"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/oliverhaas/djangogo/forms"
	"github.com/oliverhaas/djangogo/messages"
	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/templates"
	"github.com/oliverhaas/djangogo/views"
)

// Changelist query parameter names, matching django.contrib.admin.views.main.
const (
	allVar         = "all"
	orderVar       = "o"
	pageVar        = "p"
	searchVar      = "q"
	errorFlag      = "e"
	isPopupVar     = "_popup"
	sourceModelVar = "_source_model"
	toFieldVar     = "_to_field"
	isFacetsVar    = "_facets"
)

// defaultListPerPage and defaultListMaxShowAll mirror Django's ModelAdmin defaults.
const (
	defaultListPerPage    = 100
	defaultListMaxShowAll = 200
)

// ignoredFilterParams are query parameters excluded from filter lookup handling.
var ignoredFilterParams = []string{
	allVar,
	orderVar,
	searchVar,
	isFacetsVar,
	isPopupVar,
	sourceModelVar,
	toFieldVar,
}

// changeList holds the resolved state of one changelist request (Django's
// django.contrib.admin.views.main.ChangeList).
type changeList struct {
	site  *AdminSite
	entry *entry

	listDisplay      []resolvedColumn
	listDisplayLinks map[string]bool
	noLinks          bool
	listEditable     []string
	hasActions       bool
	sortableBy       map[string]bool
	sortableRestrict bool
	searchFields     []string
	searchHelpText   string
	listPerPage      int
	listMaxShowAll   int

	params       map[string]string
	filterParams url.Values

	query            string
	pageNum          int
	showAll          bool
	isPopup          bool
	preservedFilters string
	filterSpecs      []filterSpec
	// appliedFilters are the filters that filter the changelist, and
	// otherConds the conditions of the date hierarchy, the other lookup
	// parameters, and the search.
	appliedFilters []appliedFilter
	otherConds     []orm.Cond
	// addFacets shows the facet counts, and isFacetsOptional the links
	// that toggle them.
	addFacets         bool
	isFacetsOptional  bool
	hasFilters        bool
	hasActiveFilters  bool
	clearAllFiltersQS string
	// dateHierarchyPath is the normalized DateHierarchy path, and
	// dateHierarchyField its date or datetime field, nil without one.
	dateHierarchyPath   string
	dateHierarchyField  *orm.Field
	baseSpec            querySpec
	filteredSpec        querySpec
	resultCount         int
	fullResultCount     int
	showFullResultCount bool
	canShowAll          bool
	multiPage           bool
	showAdminActions    bool
	pageRange           []any
	objects             []any
}

// orderedCol records an active sort column index and direction ("asc" or "desc").
type orderedCol struct {
	index int
	dir   string
}

// emptyValue returns e's effective empty-value display string.
func (e *entry) emptyValue(s *AdminSite) string {
	if e.admin.EmptyValueDisplay != "" {
		return e.admin.EmptyValueDisplay
	}
	return s.emptyValue()
}

// listPerPage returns e's configured ListPerPage or Django's default (100).
func (e *entry) listPerPage() int {
	if e.admin.ListPerPage > 0 {
		return e.admin.ListPerPage
	}
	return defaultListPerPage
}

// listMaxShowAll returns e's configured ListMaxShowAll or Django's default (200).
func (e *entry) listMaxShowAll() int {
	if e.admin.ListMaxShowAll > 0 {
		return e.admin.ListMaxShowAll
	}
	return defaultListMaxShowAll
}

// showFullResultCount returns e's ShowFullResultCount (default true).
func (e *entry) showFullResultCount() bool {
	if e.admin.ShowFullResultCount != nil {
		return *e.admin.ShowFullResultCount
	}
	return true
}

// actionsOnTop returns e's ActionsOnTop (default true).
func (e *entry) actionsOnTop() bool {
	if e.admin.ActionsOnTop != nil {
		return *e.admin.ActionsOnTop
	}
	return true
}

// actionsSelectionCounter returns e's ActionsSelectionCounter (default true).
func (e *entry) actionsSelectionCounter() bool {
	if e.admin.ActionsSelectionCounter != nil {
		return *e.admin.ActionsSelectionCounter
	}
	return true
}

// joins returns the relations that the changelist query for r joins and
// those it loads in one more query each, Django's apply_select_related.
// shown are the relations that the columns show. Without ListSelectRelated
// the query joins them; with it, the query joins its relations and loads
// the shown ones that no join covers, since a join of "a__b" loads "a" too.
func (e *entry) joins(r *http.Request, shown []string) (joined, loaded []string) {
	joined = e.admin.ListSelectRelated
	if e.admin.GetListSelectRelated != nil {
		joined = e.admin.GetListSelectRelated(r)
	}
	if joined == nil {
		return shown, nil
	}
	for _, p := range shown {
		if !slices.ContainsFunc(joined, func(j string) bool { return j == p || strings.HasPrefix(j, p+"__") }) {
			loaded = append(loaded, p)
		}
	}
	return joined, loaded
}

// buildChangeList constructs and executes the changelist query for r and e.
func (s *AdminSite) buildChangeList(r *http.Request, e *entry, actions []Action) (*changeList, error) {
	ctx := r.Context()
	rawQuery := r.URL.Query()

	// Search query validation (reject NUL characters as Django's ChangeListSearchForm does).
	searchTerm := rawQuery.Get(searchVar)
	if strings.ContainsRune(searchTerm, 0) {
		_ = messages.Error(r, "Null characters are not allowed.")
		searchTerm = ""
	}

	pageNum := 1
	if pStr := rawQuery.Get(pageVar); pStr != "" {
		if n, err := strconv.Atoi(pStr); err == nil {
			pageNum = n
		}
	}

	_, showAll := rawQuery[allVar]
	_, isPopup := rawQuery[isPopupVar]

	filterParams := cloneValues(rawQuery)
	delete(filterParams, pageVar)
	delete(filterParams, errorFlag)

	params := make(map[string]string, len(filterParams))
	for k, vals := range filterParams {
		if len(vals) > 0 {
			params[k] = vals[len(vals)-1]
		}
	}

	// Resolve list_display.
	var rawDisplay []string
	if e.admin.GetListDisplay != nil {
		rawDisplay = e.admin.GetListDisplay(r)
	} else {
		rawDisplay = e.admin.ListDisplay
	}
	if len(rawDisplay) == 0 {
		rawDisplay = []string{"__str__"}
	}

	// Resolve list_display_links before prepending action_checkbox.
	var rawLinks []string
	var noLinks bool
	if e.admin.GetListDisplayLinks != nil {
		rawLinks = e.admin.GetListDisplayLinks(r, rawDisplay)
		noLinks = rawLinks != nil && len(rawLinks) == 0
	} else {
		rawLinks = e.admin.ListDisplayLinks
		noLinks = rawLinks != nil && len(rawLinks) == 0
	}
	linkSet := make(map[string]bool)
	if !noLinks {
		if len(rawLinks) > 0 {
			for _, l := range rawLinks {
				linkSet[l] = true
				linkSet[normalizeFieldPath(e.model, l)] = true
			}
		} else if len(rawDisplay) > 0 {
			linkSet[rawDisplay[0]] = true
			linkSet[normalizeFieldPath(e.model, rawDisplay[0])] = true
		}
	}

	hasActions := len(actions) > 0 && !isPopup
	cols := make([]resolvedColumn, 0, len(rawDisplay)+1)
	if hasActions {
		cb, _ := resolveDisplayColumn(e, "action_checkbox")
		cols = append(cols, cb)
	}
	for _, name := range rawDisplay {
		if col, ok := resolveDisplayColumn(e, name); ok {
			cols = append(cols, col)
		}
	}

	var sortableSet map[string]bool
	sortableRestrict := e.admin.SortableBy != nil
	if sortableRestrict {
		sortableSet = make(map[string]bool, len(e.admin.SortableBy))
		for _, sb := range e.admin.SortableBy {
			sortableSet[sb] = true
			sortableSet[normalizeFieldPath(e.model, sb)] = true
		}
	}

	searchFields := e.searchFields(r)

	var listFilter []any
	if e.admin.GetListFilter != nil {
		listFilter = e.admin.GetListFilter(r)
	} else {
		listFilter = e.admin.ListFilter
	}

	// A popup edits nothing in place, as Django's ChangeList drops
	// list_editable there.
	var listEditable []string
	if !isPopup {
		listEditable = e.admin.ListEditable
	}

	var baseSpec querySpec
	if e.admin.GetQuerysetFilter != nil {
		if c := e.admin.GetQuerysetFilter(r); c.String() != "(AND: )" {
			baseSpec.filters = append(baseSpec.filters, c)
		}
	}

	cl := &changeList{
		site:                s,
		entry:               e,
		listDisplay:         cols,
		listDisplayLinks:    linkSet,
		noLinks:             noLinks,
		listEditable:        listEditable,
		hasActions:          hasActions,
		sortableBy:          sortableSet,
		sortableRestrict:    sortableRestrict,
		searchFields:        searchFields,
		searchHelpText:      e.admin.SearchHelpText,
		listPerPage:         e.listPerPage(),
		listMaxShowAll:      e.listMaxShowAll(),
		params:              params,
		filterParams:        filterParams,
		query:               searchTerm,
		pageNum:             pageNum,
		showAll:             showAll,
		isPopup:             isPopup,
		preservedFilters:    s.preservedFilters(r, e),
		baseSpec:            baseSpec,
		showFullResultCount: e.showFullResultCount(),
	}
	if cl.dateHierarchyField = dateHierarchyField(e); cl.dateHierarchyField != nil {
		cl.dateHierarchyPath = normalizeFieldPath(e.model, e.admin.DateHierarchy)
	}

	_, hasFacetsVar := rawQuery[isFacetsVar]
	cl.addFacets = e.admin.ShowFacets == ShowFacetsAlways || (e.admin.ShowFacets == ShowFacetsAllow && hasFacetsVar)
	cl.isFacetsOptional = e.admin.ShowFacets == ShowFacetsAllow

	// Build and apply filters.
	fr, err := s.buildFilters(ctx, r, e, cl, listFilter, baseSpec)
	if err != nil {
		return nil, err
	}
	remainingParams := fr.remaining
	cl.filterSpecs = fr.specs
	cl.appliedFilters = fr.applied
	cl.otherConds = fr.otherConds
	cl.hasFilters = len(fr.specs) > 0
	cl.hasActiveFilters = fr.hasActiveFilters

	// Build clear-all-filters link preserving remaining params and non-filter params.
	allFilterKeys := make([]string, 0, len(cl.getFiltersParams()))
	for k := range cl.getFiltersParams() {
		allFilterKeys = append(allFilterKeys, k)
	}
	newRem := make(map[string]any, len(remainingParams))
	for k, vals := range remainingParams {
		newRem[k] = vals
	}
	cl.clearAllFiltersQS = cl.getQueryString(newRem, allFilterKeys)

	joined, loaded := e.joins(r, selectRelatedForColumns(cols))
	spec := querySpec{
		filters:         append([]orm.Cond(nil), baseSpec.filters...),
		selectRelated:   joined,
		prefetchRelated: loaded,
		distinct:        fr.mayHaveDuplicates,
	}
	spec.filters = append(spec.filters, fr.conds()...)

	// Apply search terms.
	searchCond, searchDups, hasSearch := buildSearchCond(e.model, searchFields, searchTerm)
	if hasSearch {
		spec.filters = append(spec.filters, searchCond)
		cl.otherConds = append(cl.otherConds, searchCond)
	}
	if searchDups {
		spec.distinct = true
	}

	// Apply ordering: ?o= -> ModelAdmin.GetOrdering/Ordering -> Model.Ordering -> -pk.
	spec.ordering = cl.resolveOrdering(r)
	cl.filteredSpec = spec

	return cl, nil
}

// loadResults executes pagination and loads the objects for the current page.
func (cl *changeList) loadResults(ctx context.Context) error {
	objList := &changelistObjectList{
		ops:  cl.entry.ops,
		spec: cl.filteredSpec,
	}
	pag := views.NewPaginator[any](objList, cl.listPerPage)
	resultCount, err := pag.Count(ctx)
	if err != nil {
		return err
	}
	cl.resultCount = resultCount

	if cl.showFullResultCount {
		fullCount, err := cl.entry.ops.count(ctx, cl.baseSpec)
		if err != nil {
			return err
		}
		cl.fullResultCount = int(fullCount)
	}

	cl.canShowAll = resultCount <= cl.listMaxShowAll
	cl.multiPage = resultCount > cl.listPerPage
	cl.showAdminActions = !cl.showFullResultCount || cl.fullResultCount > 0

	if (cl.showAll && cl.canShowAll) || !cl.multiPage {
		objs, err := cl.entry.ops.query(ctx, cl.filteredSpec)
		if err != nil {
			return err
		}
		cl.objects = objs
	} else {
		page, err := pag.Page(ctx, cl.pageNum)
		if err != nil {
			if errors.Is(err, views.ErrInvalidPage) {
				return fmt.Errorf("%w: %w", errIncorrectLookup, err)
			}
			return err
		}
		cl.objects = page.ObjectList
	}

	paginationRequired := (!cl.showAll || !cl.canShowAll) && cl.multiPage
	if paginationRequired {
		pageRange, err := pag.GetElidedPageRange(ctx, cl.pageNum, views.DefaultOnEachSide, views.DefaultOnEnds)
		if err != nil {
			return err
		}
		cl.pageRange = pageRange
	}
	return nil
}

// facetSpec returns the query whose rows the facets of spec count, Django's
// ChangeList.get_queryset with exclude_parameters: the changelist query
// without the filters that read the same parameters as spec.
func (cl *changeList) facetSpec(spec filterSpec) querySpec {
	exclude := spec.ExpectedParameters()
	filters := slices.Clone(cl.baseSpec.filters)
	for _, a := range cl.appliedFilters {
		if !slices.Equal(a.spec.ExpectedParameters(), exclude) {
			filters = append(filters, a.cond)
		}
	}
	filters = append(filters, cl.otherConds...)
	return querySpec{filters: filters, distinct: cl.filteredSpec.distinct}
}

// filterContext returns the template context of the sidebar filters, with
// the facet counts when the changelist shows them.
func (cl *changeList) filterContext(ctx context.Context) ([]map[string]any, error) {
	out := make([]map[string]any, 0, len(cl.filterSpecs))
	for _, spec := range cl.filterSpecs {
		var counts map[string]int64
		if cl.addFacets {
			var err error
			if counts, err = cl.entry.ops.facetCounts(ctx, cl.facetSpec(spec), spec.Facets()); err != nil {
				return nil, err
			}
		}
		choices := spec.Choices(cl, counts)
		chMaps := make([]map[string]any, len(choices))
		for i, ch := range choices {
			chMaps[i] = map[string]any{
				"selected":     ch.Selected,
				"query_string": ch.QueryString,
				"display":      ch.Display,
			}
		}
		out = append(out, map[string]any{
			"title":   spec.Title(),
			"choices": chMaps,
		})
	}
	return out, nil
}

// changelistObjectList adapts modelOps + querySpec to views.ObjectList[any].
type changelistObjectList struct {
	ops  modelOps
	spec querySpec
}

func (l *changelistObjectList) Count(ctx context.Context) (int, error) {
	n, err := l.ops.count(ctx, l.spec)
	return int(n), err
}

func (l *changelistObjectList) Slice(ctx context.Context, offset, limit int) ([]any, error) {
	s := l.spec
	s.offset = offset
	if limit == 0 {
		s.limitZero = true
	} else {
		s.limit = limit
	}
	return l.ops.query(ctx, s)
}

// getFiltersParams returns a copy of filterParams with ignoredFilterParams removed.
func (cl *changeList) getFiltersParams() url.Values {
	out := cloneValues(cl.filterParams)
	for _, ig := range ignoredFilterParams {
		delete(out, ig)
	}
	return out
}

// getQueryString builds a sorted query string starting with "?" based on the
// current changelist filterParams, removing any keys prefixed by entries in
// remove and setting/deleting entries from newParams (nil deletes a key).
func (cl *changeList) getQueryString(newParams map[string]any, remove []string) string {
	p := cloneValues(cl.filterParams)
	for _, r := range remove {
		for k := range p {
			if strings.HasPrefix(k, r) {
				delete(p, k)
			}
		}
	}
	for k, v := range newParams {
		if v == nil {
			delete(p, k)
			continue
		}
		switch tv := v.(type) {
		case []string:
			p[k] = slices.Clone(tv)
		case string:
			p[k] = []string{tv}
		default:
			p[k] = []string{fmt.Sprint(tv)}
		}
	}
	return "?" + p.Encode()
}

// defaultOrdering returns ModelAdmin.GetOrdering/Ordering, else Model.Ordering().
func (cl *changeList) defaultOrdering(r *http.Request) []string {
	var raw []string
	if cl.entry.admin.GetOrdering != nil && r != nil {
		raw = cl.entry.admin.GetOrdering(r)
	}
	if len(raw) == 0 {
		raw = cl.entry.admin.Ordering
	}
	if len(raw) == 0 {
		raw = cl.entry.model.Ordering()
	}
	if len(raw) == 0 {
		return nil
	}
	out := make([]string, len(raw))
	for i, term := range raw {
		out[i] = normalizeFieldPath(cl.entry.model, term)
	}
	return out
}

// resolveOrdering computes the final ORM OrderBy slice for the changelist:
// ?o= overrides default ordering, and "-pk" is appended unless the primary key
// is already among the ordering terms.
func (cl *changeList) resolveOrdering(r *http.Request) []string {
	var ordering []string
	if oParam, ok := cl.params[orderVar]; ok && oParam != "" {
		for _, token := range strings.Split(oParam, ".") {
			pfx := ""
			idxStr := token
			if strings.HasPrefix(token, "-") {
				pfx = "-"
				idxStr = token[1:]
			}
			idx, err := strconv.Atoi(idxStr)
			if err != nil || idx < 0 || idx >= len(cl.listDisplay) {
				continue
			}
			orderField := cl.listDisplay[idx].orderField
			if orderField == "" {
				continue
			}
			if pfx == "-" && strings.HasPrefix(orderField, "-") {
				ordering = append(ordering, strings.TrimPrefix(orderField, "-"))
			} else {
				ordering = append(ordering, pfx+orderField)
			}
		}
	} else {
		ordering = cl.defaultOrdering(r)
	}

	pkAttr := ""
	if pk := cl.entry.model.PrimaryKey(); pk != nil {
		pkAttr = pk.Attr
	}
	hasPK := false
	for _, term := range ordering {
		bare := strings.TrimPrefix(term, "-")
		if bare == "pk" || (pkAttr != "" && bare == pkAttr) {
			hasPK = true
			break
		}
	}
	if !hasPK {
		ordering = append(ordering, "-pk")
	}
	return ordering
}

// orderingFieldColumns returns the active ordered columns in priority order,
// matching Django's ChangeList.get_ordering_field_columns.
func (cl *changeList) orderingFieldColumns(r *http.Request) []orderedCol {
	var out []orderedCol
	seen := make(map[int]bool)

	if oParam, hasO := cl.params[orderVar]; !hasO {
		for _, field := range cl.defaultOrdering(r) {
			dir := "asc"
			bare := field
			if strings.HasPrefix(field, "-") {
				dir = "desc"
				bare = field[1:]
			}
			for i, col := range cl.listDisplay {
				if col.orderField == bare && !seen[i] {
					seen[i] = true
					out = append(out, orderedCol{index: i, dir: dir})
					break
				}
			}
		}
	} else if oParam != "" {
		for _, token := range strings.Split(oParam, ".") {
			dir := "asc"
			idxStr := token
			if strings.HasPrefix(token, "-") {
				dir = "desc"
				idxStr = token[1:]
			}
			idx, err := strconv.Atoi(idxStr)
			if err != nil || idx < 0 || idx >= len(cl.listDisplay) || seen[idx] {
				continue
			}
			seen[idx] = true
			out = append(out, orderedCol{index: idx, dir: dir})
		}
	}
	return out
}

// isColumnSortable reports whether col is sortable under ModelAdmin.SortableBy
// and column metadata.
func (cl *changeList) isColumnSortable(col resolvedColumn) bool {
	if col.isActionCheckbox || col.orderField == "" {
		return false
	}
	if !cl.sortableRestrict {
		return true
	}
	return cl.sortableBy[col.rawName] || cl.sortableBy[col.cssName]
}

// resultHeaders builds the header template dictionaries matching Django's
// templatetags/admin_list.py::result_headers.
func (cl *changeList) resultHeaders(r *http.Request) ([]map[string]any, int) {
	activeOrder := cl.orderingFieldColumns(r)
	orderDir := make(map[int]string, len(activeOrder))
	orderPrio := make(map[int]int, len(activeOrder))
	for pos, oc := range activeOrder {
		orderDir[oc.index] = oc.dir
		orderPrio[oc.index] = pos + 1
	}

	makeParam := func(dir string, idx int) string {
		if dir == "desc" {
			return "-" + strconv.Itoa(idx)
		}
		return strconv.Itoa(idx)
	}

	headers := make([]map[string]any, 0, len(cl.listDisplay))
	numSorted := 0

	for i, col := range cl.listDisplay {
		if col.isActionCheckbox {
			headers = append(headers, map[string]any{
				"is_action_checkbox": true,
				"sortable":           false,
				"sorted":             false,
				"sort_priority":      0,
				"class_attrib":       ` class="action-checkbox-column"`,
				"text":               `<input type="checkbox" id="action-toggle" aria-label="Select all objects on this page for an action">`,
			})
			continue
		}

		if !cl.isColumnSortable(col) {
			headers = append(headers, map[string]any{
				"is_action_checkbox": false,
				"sortable":           false,
				"sorted":             false,
				"sort_priority":      0,
				"class_attrib":       fmt.Sprintf(` class="column-%s"`, html.EscapeString(col.cssName)),
				"text":               col.label,
			})
			continue
		}

		classes := []string{"sortable", "column-" + col.cssName}
		dir, isSorted := orderDir[i]
		prio := 0
		newDir := "asc"
		if isSorted {
			numSorted++
			prio = orderPrio[i]
			classes = append(classes, "sorted", dir+"ending")
			if dir == "asc" {
				newDir = "desc"
			}
		}

		var oPrimary, oRemove, oToggle []string
		for _, oc := range activeOrder {
			if oc.index == i {
				p := makeParam(newDir, oc.index)
				oPrimary = append([]string{p}, oPrimary...)
				oToggle = append(oToggle, p)
			} else {
				p := makeParam(oc.dir, oc.index)
				oPrimary = append(oPrimary, p)
				oToggle = append(oToggle, p)
				oRemove = append(oRemove, p)
			}
		}
		if !isSorted {
			oPrimary = append([]string{makeParam(newDir, i)}, oPrimary...)
		}

		headers = append(headers, map[string]any{
			"is_action_checkbox": false,
			"sortable":           true,
			"sorted":             isSorted,
			"ascending":          dir == "asc",
			"sort_priority":      prio,
			"url_primary":        cl.getQueryString(map[string]any{orderVar: strings.Join(oPrimary, ".")}, nil),
			"url_remove":         cl.getQueryString(map[string]any{orderVar: strings.Join(oRemove, ".")}, nil),
			"url_toggle":         cl.getQueryString(map[string]any{orderVar: strings.Join(oToggle, ".")}, nil),
			"class_attrib":       fmt.Sprintf(` class="%s"`, html.EscapeString(strings.Join(classes, " "))),
			"text":               col.label,
		})
	}
	return headers, numSorted
}

// buildRows renders every row's table cells as pre-escaped HTML strings
// matching Django's items_for_result, and the hidden primary keys of the
// forms of list_editable (Django's result_hidden_fields). Row i shows the
// editable fields of rowForms[i] when there is one. It returns the error of
// a model method that a column shows.
func (cl *changeList) buildRows(rowForms []*forms.Form) ([]map[string]any, []int64, []string, error) {
	emptyVal := cl.entry.emptyValue(cl.site)
	rows := make([]map[string]any, len(cl.objects))
	pks := make([]int64, len(cl.objects))
	var hidden []string
	pkName := ""
	if pkf := cl.entry.model.PrimaryKey(); pkf != nil {
		pkName = pkf.Attr
	}

	for i, obj := range cl.objects {
		var form *forms.Form
		if i < len(rowForms) {
			form = rowForms[i]
		}
		var pkField *forms.BoundField
		if form != nil {
			pkField = form.Field(pkName)
		}
		elem := reflect.ValueOf(obj).Elem()
		pk := pkOf(elem, cl.entry)
		pks[i] = pk
		changeURL := cl.site.addPreservedFilters(cl.entry, cl.site.changeURL(cl.entry, pk), cl.preservedFilters, nil, false, "")
		objLabel := orm.Label(cl.entry.model, obj)
		if objLabel == "" {
			objLabel = emptyVal
		}

		cellsHTML := make([]string, 0, len(cl.listDisplay))
		firstLink := true

		for _, col := range cl.listDisplay {
			if col.isActionCheckbox {
				aria := html.EscapeString("Select this object for an action - " + orm.Label(cl.entry.model, obj))
				cell := fmt.Sprintf(
					`<td class="action-checkbox"><input type="checkbox" name="%s" value="%d" class="action-select" aria-label="%s"></td>`,
					ActionCheckboxName, pk, aria,
				)
				cellsHTML = append(cellsHTML, cell)
				continue
			}

			isLink := !cl.noLinks && (cl.listDisplayLinks[col.rawName] || cl.listDisplayLinks[col.cssName])
			valHTML, extraClasses, err := renderColumnCell(col, obj, cl.entry.model, emptyVal, isLink)
			if err != nil {
				return nil, nil, nil, err
			}
			classes := make([]string, 0, 1+len(extraClasses))
			classes = append(classes, "field-"+col.cssName)
			classes = append(classes, extraClasses...)
			classAttr := html.EscapeString(strings.Join(classes, " "))

			if isLink {
				tag := "td"
				if firstLink {
					tag = "th"
					firstLink = false
				}
				// A popup's link carries the value the opener's field takes.
				opener := ""
				if cl.isPopup {
					opener = fmt.Sprintf(` data-popup-opener="%d"`, pk)
				}
				linkHTML := fmt.Sprintf(`<a href="%s"%s>%s</a>`, html.EscapeString(changeURL), opener, valHTML)
				cellsHTML = append(cellsHTML, fmt.Sprintf(`<%s class="%s">%s</%s>`, tag, classAttr, linkHTML, tag))
			} else {
				if form != nil {
					name := formName(cl.entry.model, col.rawName)
					if bf := form.Field(name); bf != nil && (name != pkName || !bf.IsHidden()) {
						valHTML = bf.Errors().HTML() + bf.HTML()
					}
				}
				cellsHTML = append(cellsHTML, fmt.Sprintf(`<td class="%s">%s</td>`, classAttr, valHTML))
			}
		}
		if pkField != nil {
			if pkField.IsHidden() {
				hidden = append(hidden, pkField.HTML())
			} else {
				cellsHTML = append(cellsHTML, "<td>"+pkField.HTML()+"</td>")
			}
		}

		row := map[string]any{
			"pk":         pk,
			"label":      objLabel,
			"change_url": changeURL,
			"cells_html": cellsHTML,
			"colspan":    len(cellsHTML),
		}
		if form != nil {
			if errs := form.NonFieldErrors(); errs.Len() > 0 {
				row["non_field_errors"] = errs.HTML()
			}
		}
		rows[i] = row
	}
	return rows, pks, hidden, nil
}

// buildPageLinks formats cl.pageRange into template-ready pagination entries.
func (cl *changeList) buildPageLinks() []map[string]any {
	if len(cl.pageRange) == 0 {
		return nil
	}
	out := make([]map[string]any, 0, len(cl.pageRange))
	for _, item := range cl.pageRange {
		switch v := item.(type) {
		case string:
			out = append(out, map[string]any{
				"is_ellipsis": true,
				"text":        v,
			})
		case int:
			if v == cl.pageNum {
				out = append(out, map[string]any{
					"is_current": true,
					"text":       strconv.Itoa(v),
					"href":       "",
				})
			} else {
				out = append(out, map[string]any{
					"is_current": false,
					"text":       strconv.Itoa(v),
					"href":       cl.getQueryString(map[string]any{pageVar: v}, nil),
				})
			}
		}
	}
	return out
}

// resetSearchURL returns the "?" link shown inside the search bar result count
// ("N results (M total)"), preserving _popup and _facets as Django does.
func (cl *changeList) resetSearchURL() string {
	var parts []string
	if cl.isPopup {
		parts = append(parts, isPopupVar+"=1")
	}
	if cl.addFacets {
		parts = append(parts, isFacetsVar)
	}
	return "?" + strings.Join(parts, "&")
}

// toFieldAllowed reports whether a page may reference the field called name
// through the _to_field parameter, Django's ModelAdmin.to_field_allowed. A
// relation always targets a primary key here, so the primary key is the only
// field a page may reference.
func (e *entry) toFieldAllowed(name string) bool {
	f, ok := e.model.FieldByAttr(name)
	return ok && f.PrimaryKey
}

// rejectToField answers 400 Bad Request and reports true when name is a
// _to_field value that toFieldAllowed refuses, Django's
// DisallowedModelAdminToField. Without this check a popup would show the
// values of any field.
func (e *entry) rejectToField(w http.ResponseWriter, name string) bool {
	if name == "" || e.toFieldAllowed(name) {
		return false
	}
	http.Error(w, "Bad Request", http.StatusBadRequest)
	return true
}

// changelist renders the model changelist view and dispatches bulk actions on POST.
func (s *AdminSite) changelist(w http.ResponseWriter, r *http.Request, e *entry) {
	if !e.hasViewOrChangePermission(s, r, nil) {
		forbidden(w)
		return
	}
	if e.rejectToField(w, r.URL.Query().Get(toFieldVar)) {
		return
	}

	actions := s.allowedActions(r, e)
	cl, err := s.buildChangeList(r, e, actions)
	if err != nil {
		s.handleChangeListError(w, r, err)
		return
	}

	if r.Method == http.MethodPost {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
	}

	// Dispatch bulk actions on POST before loading the paginated page.
	if r.Method == http.MethodPost && len(actions) > 0 && !cl.isPopup {
		if !r.PostForm.Has("_save") {
			selected := r.PostForm[ActionCheckboxName]
			selectAcross := isTruthyParam(r.PostForm.Get("select_across"))
			if r.PostForm.Has("index") {
				if len(selected) > 0 || selectAcross {
					if handled := s.responseAction(w, r, e, cl, actions); handled {
						return
					}
					http.Redirect(w, r, r.URL.RequestURI(), http.StatusFound)
					return
				}
				_ = messages.Warning(r, "Items must be selected in order to perform actions on them. No items have been changed.")
				http.Redirect(w, r, r.URL.RequestURI(), http.StatusFound)
				return
			}
			if r.PostForm.Has(ActionCheckboxName) {
				if len(selected) > 0 || selectAcross {
					if handled := s.responseAction(w, r, e, cl, actions); handled {
						return
					}
					http.Redirect(w, r, r.URL.RequestURI(), http.StatusFound)
					return
				}
			}
		}
	}

	// Save the rows of list_editable, or keep the bound formset with its
	// errors for the page.
	canChange := e.hasChangePermission(s, r, nil)
	var set changelistSet
	if r.Method == http.MethodPost && len(cl.listEditable) > 0 && r.PostForm.Has("_save") {
		var handled bool
		if set, handled = s.saveListEditable(w, r, e, cl); handled {
			return
		}
	}

	if err := cl.loadResults(r.Context()); err != nil {
		s.handleChangeListError(w, r, err)
		return
	}
	dateHierarchy, err := cl.dateHierarchy(r.Context())
	if err != nil {
		s.handleChangeListError(w, r, err)
		return
	}

	if set == nil && len(cl.listEditable) > 0 && canChange {
		if set, err = s.changelistFormSet(r, e, cl.listEditable, cl.objects, nil, nil); err != nil {
			http.Error(w, "invalid list_editable: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}
	var rowForms []*forms.Form
	if set != nil {
		rowForms = set.base().Forms()
	}

	headers, numSorted := cl.resultHeaders(r)
	rows, pks, hiddenFields, err := cl.buildRows(rowForms)
	if err != nil {
		s.handleChangeListError(w, r, err)
		return
	}

	filterContext, err := cl.filterContext(r.Context())
	if err != nil {
		s.handleChangeListError(w, r, err)
		return
	}

	// Build hidden inputs for the search form so filters/sort survive search.
	var searchHidden []map[string]string
	for k, v := range cl.params {
		if k == searchVar {
			continue
		}
		searchHidden = append(searchHidden, map[string]string{"name": k, "value": v})
	}

	singular, plural := modelVerboseNames(e.model)
	title := "Select " + singular + " to view"
	if cl.isPopup {
		title = "Select " + singular
	} else if canChange {
		title = "Select " + singular + " to change"
	}

	paginationRequired := (!cl.showAll || !cl.canShowAll) && cl.multiPage
	showAllURL := ""
	if cl.canShowAll && !cl.showAll && cl.multiPage {
		showAllURL = cl.getQueryString(map[string]any{allVar: ""}, nil)
	}

	resultCountLabel := plural
	if cl.resultCount == 1 {
		resultCountLabel = singular
	}

	selectionNoteAll := fmt.Sprintf("All %d selected", cl.resultCount)
	if cl.resultCount == 1 {
		selectionNoteAll = "1 selected"
	}

	actionChoices := s.actionChoices(e, actions)
	showActions := cl.hasActions && cl.showAdminActions

	ctx := s.EachContext(r)
	ctx["title"] = title
	ctx["subtitle"] = ""
	ctx["is_popup"] = cl.isPopup
	ctx["app_label"] = e.app
	ctx["app_verbose_name"] = s.appVerboseName(e.app)
	ctx["app_url"] = s.appURLOrEmpty(e.app)
	ctx["slug"] = e.slug
	ctx["model"] = e.model.Name()
	ctx["verbose_name"] = singular
	ctx["verbose_name_plural"] = capFirst(plural)
	ctx["verbose_name_plural_raw"] = plural
	ctx["module_name"] = plural
	ctx["has_add_permission"] = e.hasAddPermission(s, r)
	ctx["has_change_permission"] = canChange
	ctx["has_delete_permission"] = e.hasDeletePermission(s, r, nil)
	ctx["add_url"] = s.addPreservedFilters(e, s.addURL(e), cl.preservedFilters, nil, cl.isPopup, r.URL.Query().Get(toFieldVar))

	// Filter sidebar context.
	ctx["has_filters"] = cl.hasFilters
	ctx["has_active_filters"] = cl.hasActiveFilters
	ctx["clear_all_filters_qs"] = cl.clearAllFiltersQS
	ctx["is_facets_optional"] = cl.isFacetsOptional
	ctx["add_facets"] = cl.addFacets
	ctx["remove_facet_link"] = cl.getQueryString(nil, []string{isFacetsVar})
	ctx["add_facet_link"] = cl.getQueryString(map[string]any{isFacetsVar: "True"}, nil)
	if dateHierarchy != nil {
		ctx["date_hierarchy"] = dateHierarchy
	}
	ctx["filter_specs"] = filterContext

	// Search bar context.
	ctx["has_search"] = len(cl.searchFields) > 0
	ctx["search_var"] = searchVar
	ctx["search_query"] = cl.query
	ctx["search_help_text"] = cl.searchHelpText
	ctx["show_result_count"] = cl.resultCount != cl.fullResultCount
	ctx["show_full_result_count"] = cl.showFullResultCount
	ctx["full_result_count"] = cl.fullResultCount
	ctx["reset_search_url"] = cl.resetSearchURL()
	ctx["search_hidden_params"] = searchHidden

	// Actions context.
	ctx["has_actions"] = cl.hasActions
	ctx["show_admin_actions"] = showActions
	ctx["actions_on_top"] = e.actionsOnTop()
	ctx["actions_on_bottom"] = e.admin.ActionsOnBottom
	ctx["actions_selection_counter"] = e.actionsSelectionCounter()
	ctx["action_choices"] = actionChoices
	ctx["action_checkbox_name"] = ActionCheckboxName
	ctx["selection_note"] = fmt.Sprintf("0 of %d selected", len(cl.objects))
	ctx["selection_note_all"] = selectionNoteAll
	ctx["show_select_across"] = cl.resultCount != len(cl.objects)
	ctx["media"] = e.media(s)
	ctx["jsi18n_url"] = s.jsi18nURL()

	// The formset of list_editable.
	ctx["has_formset"] = set != nil
	ctx["list_editable"] = cl.listEditable
	ctx["result_hidden_fields"] = hiddenFields
	if set != nil {
		fs := set.base()
		ctx["media"] = e.media(s).Add(fs.Media())
		ctx["management_form"] = templates.MarkSafe(fs.ManagementForm().String())
		ctx["formset_multipart"] = fs.IsMultipart()
		// Django tests the list of the errors of the forms, which a bound
		// formset with forms always has.
		if fs.IsBound() && len(fs.Errors()) > 0 {
			ctx["formset_errors"] = true
			ctx["error_note"] = "Please correct the errors below."
			if fs.TotalErrorCount() == 1 {
				ctx["error_note"] = "Please correct the error below."
			}
			ctx["non_form_errors"] = templates.MarkSafe(fs.NonFormErrors().HTML())
		}
	}

	// Results & pagination context.
	ctx["result_headers"] = headers
	ctx["num_sorted_fields"] = numSorted
	ctx["rows"] = rows
	ctx["result_pks"] = pks
	ctx["result_count"] = cl.resultCount
	ctx["page_count"] = len(cl.objects)
	ctx["result_count_label"] = resultCountLabel
	ctx["pagination_required"] = paginationRequired
	ctx["page_links"] = cl.buildPageLinks()
	ctx["show_all_url"] = showAllURL

	ctx["includes"] = s.includes(e, "change_list_object_tools", "search_form", "date_hierarchy", "actions", "change_list_results", "pagination")
	s.renderPage(w, r, e, e.admin.ChangeListTemplate, "change_list.html", ctx)
}

// handleChangeListError maps errDisallowedLookup to 400 Bad Request,
// errIncorrectLookup to a ?e=1 redirect (or invalid_setup.html when ?e=1 is
// already set), and other errors to 500 Internal Server Error.
func (s *AdminSite) handleChangeListError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, errDisallowedLookup) {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}
	if errors.Is(err, errIncorrectLookup) {
		if _, hasErr := r.URL.Query()[errorFlag]; hasErr {
			ctx := s.EachContext(r)
			ctx["title"] = "Database error"
			s.render(w, r, "admin/invalid_setup.html", ctx)
			return
		}
		http.Redirect(w, r, r.URL.Path+"?"+errorFlag+"=1", http.StatusFound)
		return
	}
	http.Error(w, "failed to load rows", http.StatusInternalServerError)
}

// cloneValues returns a deep copy of url.Values.
func cloneValues(in url.Values) url.Values {
	out := make(url.Values, len(in))
	for k, vals := range in {
		out[k] = slices.Clone(vals)
	}
	return out
}

// isTruthyParam interprets HTML boolean form values ("1", "true", "on") as true.
func isTruthyParam(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "on", "yes":
		return true
	default:
		return false
	}
}
