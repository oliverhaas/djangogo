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

	"github.com/oliverhaas/djangogo/messages"
	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/views"
)

// Changelist query parameter names, matching django.contrib.admin.views.main.
const (
	allVar      = "all"
	orderVar    = "o"
	pageVar     = "p"
	searchVar   = "q"
	errorFlag   = "e"
	isPopupVar  = "_popup"
	toFieldVar  = "_to_field"
	isFacetsVar = "_facets"
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
	"_source_model",
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
	hasActions       bool
	sortableBy       map[string]bool
	sortableRestrict bool
	searchFields     []string
	searchHelpText   string
	listPerPage      int
	listMaxShowAll   int

	params       map[string]string
	filterParams url.Values

	query               string
	pageNum             int
	showAll             bool
	isPopup             bool
	filterSpecs         []filterSpec
	hasFilters          bool
	hasActiveFilters    bool
	clearAllFiltersQS   string
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
		for _, f := range e.model.Fields() {
			rawDisplay = append(rawDisplay, f.Name)
		}
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
		baseSpec:            baseSpec,
		showFullResultCount: e.showFullResultCount(),
	}

	// Build and apply filters.
	filterSpecs, filterConds, remainingParams, filterDups, hasActiveFilters, err := s.buildFilters(
		ctx, r, e, cl, listFilter, baseSpec,
	)
	if err != nil {
		return nil, err
	}
	cl.filterSpecs = filterSpecs
	cl.hasFilters = len(filterSpecs) > 0
	cl.hasActiveFilters = hasActiveFilters

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

	spec := querySpec{
		filters:       append([]orm.Cond(nil), baseSpec.filters...),
		selectRelated: selectRelatedForColumns(cols),
		distinct:      filterDups,
	}
	spec.filters = append(spec.filters, filterConds...)

	// Apply search terms.
	searchCond, searchDups, hasSearch := buildSearchCond(e.model, searchFields, searchTerm)
	if hasSearch {
		spec.filters = append(spec.filters, searchCond)
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
// matching Django's items_for_result.
func (cl *changeList) buildRows() ([]map[string]any, []int64) {
	emptyVal := cl.entry.emptyValue(cl.site)
	rows := make([]map[string]any, len(cl.objects))
	pks := make([]int64, len(cl.objects))

	for i, obj := range cl.objects {
		elem := reflect.ValueOf(obj).Elem()
		pk := pkOf(elem, cl.entry)
		pks[i] = pk
		changeURL := cl.site.changeURL(cl.entry, pk)
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
			valHTML, extraClasses := renderColumnCell(col, obj, cl.entry.model, emptyVal, isLink)
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
				cellsHTML = append(cellsHTML, fmt.Sprintf(`<td class="%s">%s</td>`, classAttr, valHTML))
			}
		}

		rows[i] = map[string]any{
			"pk":         pk,
			"label":      objLabel,
			"change_url": changeURL,
			"cells_html": cellsHTML,
		}
	}
	return rows, pks
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
	if _, hasFacets := cl.params[isFacetsVar]; hasFacets {
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

	// Dispatch bulk actions on POST before loading the paginated page.
	if r.Method == http.MethodPost && len(actions) > 0 && !cl.isPopup {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
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

	if err := cl.loadResults(r.Context()); err != nil {
		s.handleChangeListError(w, r, err)
		return
	}

	headers, numSorted := cl.resultHeaders(r)
	rows, pks := cl.buildRows()

	// Build filter sidebar context.
	filterContext := make([]map[string]any, 0, len(cl.filterSpecs))
	for _, spec := range cl.filterSpecs {
		choices := spec.Choices(cl)
		chMaps := make([]map[string]any, len(choices))
		for i, ch := range choices {
			chMaps[i] = map[string]any{
				"selected":     ch.Selected,
				"query_string": ch.QueryString,
				"display":      ch.Display,
			}
		}
		filterContext = append(filterContext, map[string]any{
			"title":   spec.Title(),
			"choices": chMaps,
		})
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
	canChange := e.hasChangePermission(s, r, nil)
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
	ctx["add_url"] = s.addURL(e)

	// Filter sidebar context.
	ctx["has_filters"] = cl.hasFilters
	ctx["has_active_filters"] = cl.hasActiveFilters
	ctx["clear_all_filters_qs"] = cl.clearAllFiltersQS
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

	s.render(w, r, "admin/change_list.html", ctx)
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
