package admin

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/oliverhaas/djangogo/orm"
)

// Sentinel errors returned while resolving changelist filters and pagination.
var (
	// errDisallowedLookup corresponds to Django's DisallowedModelAdminLookup
	// (SuspiciousOperation -> HTTP 400).
	errDisallowedLookup = errors.New("admin: disallowed model admin lookup")
	// errIncorrectLookup corresponds to Django's IncorrectLookupParameters
	// (redirects changelist to ?e=1).
	errIncorrectLookup = errors.New("admin: incorrect lookup parameters")
)

// filterChoice is one clickable link rendered inside a sidebar filter <details>.
type filterChoice struct {
	Selected    bool
	QueryString string
	Display     string
}

// filterSpec is one evaluated sidebar filter section ready to filter the
// queryset and render choices in the template.
type filterSpec interface {
	Title() string
	HasOutput() bool
	// Choices returns the links of the filter. counts holds the facet counts
	// by key, nil when the changelist shows no counts.
	Choices(cl *changeList, counts map[string]int64) []filterChoice
	Apply() (orm.Cond, bool, error)
	// ExpectedParameters names the query parameters that the filter reads,
	// Django's expected_parameters.
	ExpectedParameters() []string
	// Facets returns the counts that the filter shows next to its choices,
	// Django's get_facet_counts.
	Facets() []facet
}

// facet is one count of a filter choice: the rows of the facet query that
// match cond. With subquery, cond filters the facet query, and the count is
// of the rows whose primary key the filtered query selects, as Django counts
// a SimpleListFilter choice.
type facet struct {
	key      string
	cond     orm.Cond
	subquery bool
}

// withCount appends the facet count of key to title, as Django shows it:
// "title (3)". Without counts it returns title unchanged.
func withCount(title string, counts map[string]int64, key string) string {
	if counts == nil {
		return title
	}
	return fmt.Sprintf("%s (%d)", title, counts[key])
}

// indexKey returns the facet key of the i-th choice, "<i>__c".
func indexKey(i int) string { return strconv.Itoa(i) + "__c" }

// appliedFilter is a filter that filters the changelist, with its condition.
type appliedFilter struct {
	spec filterSpec
	cond orm.Cond
}

// filterResult is what buildFilters resolves from the query string.
type filterResult struct {
	// specs are the filters that the sidebar shows.
	specs []filterSpec
	// applied are the filters that filter the changelist.
	applied []appliedFilter
	// otherConds are the conditions of the date hierarchy and of the lookup
	// parameters that no filter reads.
	otherConds []orm.Cond
	// remaining are the lookup parameters that no filter reads, with the
	// bounds of the date hierarchy.
	remaining         url.Values
	mayHaveDuplicates bool
	hasActiveFilters  bool
}

// conds returns the conditions of the applied filters and the others.
func (fr *filterResult) conds() []orm.Cond {
	out := make([]orm.Cond, 0, len(fr.applied)+len(fr.otherConds))
	for _, a := range fr.applied {
		out = append(out, a.cond)
	}
	return append(out, fr.otherConds...)
}

// lookupAllowed checks whether filtering by lookup (a query parameter key)
// with value is permitted on e, matching Django's ModelAdmin.lookup_allowed.
func lookupAllowed(e *entry, lookup, value string, listFilter []any) bool {
	if lookup == "" {
		return true
	}
	if e.admin.hooks != nil && e.admin.hooks.lookupAllowed != nil && !e.admin.hooks.lookupAllowed(lookup) {
		return false
	}
	if relatedLookupAllowed(e.model, lookup, value) {
		return true
	}

	// Build set of valid relational paths and SimpleListFilter parameter names.
	validLookups := make(map[string]bool, len(listFilter)+1)
	if e.admin.DateHierarchy != "" {
		validLookups[normalizeFieldPath(e.model, e.admin.DateHierarchy)] = true
	}
	for _, item := range listFilter {
		switch v := item.(type) {
		case string:
			validLookups[normalizeFieldPath(e.model, v)] = true
		case FieldFilter:
			validLookups[normalizeFieldPath(e.model, v.Field)] = true
		case SimpleListFilter:
			if v.ParameterName != "" {
				validLookups[v.ParameterName] = true
			}
		case *SimpleListFilter:
			if v != nil && v.ParameterName != "" {
				validLookups[v.ParameterName] = true
			}
		}
	}
	if validLookups[lookup] {
		return true
	}

	// Strip custom filter suffixes such as "__isempty" before checking relation depth.
	checkPath := strings.TrimSuffix(lookup, "__isempty")
	parts := strings.Split(checkPath, "__")
	var relationParts []string
	cur := e.model
	var prevField *orm.Field
	var prevRel bool
	var lastPart string
	for _, part := range parts {
		lastPart = part
		if cur == nil {
			break
		}
		if part == "pk" {
			if !prevRel {
				relationParts = append(relationParts, part)
			}
			break
		}
		f, ok := resolveModelField(cur, part)
		if ok {
			isRel := f.Rel != nil
			if !prevRel || (isRel || !f.PrimaryKey) {
				relationParts = append(relationParts, f.Attr)
			}
			if !isRel || f.Rel.Target == nil {
				break
			}
			prevField = f
			prevRel = true
			cur = f.Rel.Target
			continue
		}
		if mf, ok := cur.ManyToManyField(part); ok && mf.Rel != nil && mf.Rel.Target != nil {
			relationParts = append(relationParts, mf.Attr)
			prevField = mf
			prevRel = true
			cur = mf.Rel.Target
			continue
		}
		if info, ok := orm.ResolveLookupPath(cur, part); ok && info.Reverse != nil && info.TargetModel != nil {
			relationParts = append(relationParts, info.Reverse.QueryName)
			prevField = nil
			prevRel = true
			cur = info.TargetModel
			continue
		}
		// Non-field trailing lookup (e.g. exact, isnull, startswith).
		break
	}
	_ = prevField

	if len(relationParts) <= 1 {
		return true
	}
	joined := strings.Join(relationParts, "__")
	if validLookups[joined] {
		return true
	}
	withLast := joined + "__" + lastPart
	return validLookups[withLast]
}

// relatedLookupAllowed reports whether lookup=value is a parameter of the
// lookup link of a raw-id widget for a foreign key to m, Django's
// Options.related_fkey_lookups: the LimitChoicesTo of each foreign key to m
// that has a reverse accessor.
func relatedLookupAllowed(m *orm.Model, lookup, value string) bool {
	for _, rev := range m.ReverseRelations() {
		if rev.Field == nil || rev.Field.Rel == nil {
			continue
		}
		params := urlParamsFromLookups(rev.Field.Rel.LimitChoicesTo())
		if v, ok := params.values[lookup]; ok && v == value {
			return true
		}
	}
	return false
}

// buildFilters constructs all active filterSpecs for cl, consumes their query
// parameters from a copy of filterParams, validates any remaining lookup
// parameters, and returns the filters with their conditions, the remaining
// lookup params, the duplicate flag, and whether any declared filter is
// active.
func (s *AdminSite) buildFilters(
	ctx context.Context,
	r *http.Request,
	e *entry,
	cl *changeList,
	listFilter []any,
	baseSpec querySpec,
) (*filterResult, error) {
	lookupParams := cl.getFiltersParams()

	// Security check: reject disallowed relational lookups with 400.
	for key, values := range lookupParams {
		if len(values) == 0 {
			values = []string{""}
		}
		for _, value := range values {
			if !lookupAllowed(e, key, value, listFilter) {
				return nil, fmt.Errorf("%w: %s", errDisallowedLookup, key)
			}
		}
	}

	var specs []filterSpec
	mayHaveDuplicates := false
	hasActiveFilters := false

	for _, item := range listFilter {
		before := len(lookupParams)
		var (
			spec      filterSpec
			fieldPath string
			err       error
		)
		switch v := item.(type) {
		case SimpleListFilter:
			sf := v
			spec = newSimpleFilterSpec(r, &sf, lookupParams)
		case *SimpleListFilter:
			if v != nil {
				spec = newSimpleFilterSpec(r, v, lookupParams)
			}
		case string:
			fieldPath = normalizeFieldPath(e.model, v)
			spec, err = s.newFieldFilterSpec(ctx, r, e, fieldPath, FilterAuto, "", lookupParams, baseSpec)
		case FieldFilter:
			fieldPath = normalizeFieldPath(e.model, v.Field)
			spec, err = s.newFieldFilterSpec(ctx, r, e, fieldPath, v.Kind, v.Title, lookupParams, baseSpec)
		default:
			continue
		}
		if err != nil {
			return nil, err
		}
		if fieldPath != "" && len(lookupParams) < before {
			if orm.LookupSpawnsDuplicates(e.model, fieldPath) {
				mayHaveDuplicates = true
			}
		}
		if spec != nil && spec.HasOutput() {
			specs = append(specs, spec)
			if len(lookupParams) < before {
				hasActiveFilters = true
			}
		}
	}

	fr := &filterResult{specs: specs}
	for _, spec := range specs {
		c, ok, err := spec.Apply()
		if err != nil {
			return nil, err
		}
		if ok {
			fr.applied = append(fr.applied, appliedFilter{spec: spec, cond: c})
		}
	}

	// Bound the date hierarchy's year, month or day as a range, which the
	// database answers faster than the extracts.
	hierarchyCond, hierarchyBounds, hasHierarchy, err := cl.dateHierarchyFilter(lookupParams)
	if err != nil {
		return nil, err
	}
	if hasHierarchy {
		fr.otherConds = append(fr.otherConds, hierarchyCond)
	}

	// Validate and apply any remaining lookup parameters not consumed by a filter.
	remainingCopy := cloneValues(lookupParams)
	for k, v := range hierarchyBounds {
		remainingCopy[k] = v
	}
	for key, vals := range lookupParams {
		if len(vals) == 0 {
			continue
		}
		info, ok := orm.ResolveLookupPath(e.model, key)
		if !ok {
			return nil, fmt.Errorf("%w: unknown lookup %q", errIncorrectLookup, key)
		}
		if info.SpawnsDuplicates {
			mayHaveDuplicates = true
		}
		c, err := buildRemainingParamCond(key, vals, info)
		if err != nil {
			return nil, err
		}
		fr.otherConds = append(fr.otherConds, c)
	}

	fr.remaining = remainingCopy
	fr.mayHaveDuplicates = mayHaveDuplicates
	fr.hasActiveFilters = hasActiveFilters
	return fr, nil
}

// buildRemainingParamCond builds an OR-combined orm.Cond for one remaining
// query parameter key and its value list.
func buildRemainingParamCond(key string, vals []string, info orm.LookupPathInfo) (orm.Cond, error) {
	var combined orm.Cond
	has := false
	for _, raw := range vals {
		val, err := prepareLookupValue(key, raw, info)
		if err != nil {
			return orm.Cond{}, err
		}
		leaf := orm.Q(key, val)
		if !has {
			combined = leaf
			has = true
		} else {
			combined = combined.Or(leaf)
		}
	}
	return combined, nil
}

// prepareLookupValue converts raw into the Go value expected by key under info,
// returning errIncorrectLookup when conversion fails.
func prepareLookupValue(key, raw string, info orm.LookupPathInfo) (any, error) {
	if strings.HasSuffix(key, "__isnull") {
		return parseIsNullParam(raw), nil
	}
	if strings.HasSuffix(key, "__in") {
		parts := strings.Split(raw, ",")
		out := make([]any, 0, len(parts))
		for _, p := range parts {
			v, ok := parseFieldValue(info.Field, p, false)
			if !ok {
				return nil, fmt.Errorf("%w: invalid value %q for %s", errIncorrectLookup, raw, key)
			}
			out = append(out, v)
		}
		return out, nil
	}
	// Pattern lookups always accept strings.
	switch info.Lookup {
	case "contains", "icontains", "startswith", "istartswith", "endswith", "iendswith", "iexact", "regex", "iregex":
		return raw, nil
	}
	f := info.Field
	if info.TargetModel != nil && info.TargetModel.PrimaryKey() != nil && (info.IsRelation || info.Lookup == "exact" || info.Lookup == "") {
		f = info.TargetModel.PrimaryKey()
	}
	v, ok := parseFieldValue(f, raw, false)
	if !ok {
		return nil, fmt.Errorf("%w: invalid value %q for %s", errIncorrectLookup, raw, key)
	}
	return v, nil
}

// parseIsNullParam matches Django's prepare_lookup_value for __isnull: "",
// "false", and "0" (case-insensitive) mean false; everything else means true.
func parseIsNullParam(raw string) bool {
	switch strings.ToLower(raw) {
	case "", "false", "0":
		return false
	default:
		return true
	}
}

// newFieldFilterSpec creates the appropriate built-in FieldListFilter for
// fieldPath on e.
func (s *AdminSite) newFieldFilterSpec(
	ctx context.Context,
	r *http.Request,
	e *entry,
	fieldPath string,
	kind FieldFilterKind,
	titleOverride string,
	params url.Values,
	baseSpec querySpec,
) (filterSpec, error) {
	info, ok := orm.ResolveLookupPath(e.model, fieldPath)
	if !ok {
		return nil, nil
	}

	if kind == FilterAuto {
		switch {
		case info.IsRelation:
			kind = FilterRelated
		case info.Field != nil && info.Field.Kind == orm.KindBool:
			kind = FilterBoolean
		case info.Field != nil && info.Field.HasChoices && len(info.Field.FlatChoices()) > 0:
			kind = FilterChoices
		case info.Field != nil && (info.Field.Kind == orm.KindDate || info.Field.Kind == orm.KindDateTime):
			kind = FilterDate
		default:
			kind = FilterAllValues
		}
	}

	title := titleOverride
	if title == "" {
		switch {
		case info.Reverse != nil && info.TargetModel != nil:
			title = info.TargetModel.VerboseName()
		case info.Field != nil:
			title = fieldVerboseName(info.Field)
		default:
			title = strings.ReplaceAll(fieldPath, "_", " ")
		}
	}

	emptyVal := e.emptyValue(s)

	switch kind {
	case FilterRelated, FilterRelatedOnly:
		return s.newRelatedFilterSpec(ctx, r, e, fieldPath, title, info, kind == FilterRelatedOnly, emptyVal, params, baseSpec)
	case FilterBoolean:
		return newBooleanFilterSpec(fieldPath, title, info.Field, params), nil
	case FilterChoices:
		return newChoicesFilterSpec(fieldPath, title, info.Field, params), nil
	case FilterDate:
		return newDateFilterSpec(fieldPath, title, info.Field, params), nil
	case FilterEmpty:
		return newEmptyFilterSpec(fieldPath, title, info.Field, params), nil
	default:
		return s.newAllValuesFilterSpec(ctx, e, fieldPath, title, info, emptyVal, params, baseSpec)
	}
}

// --- SimpleListFilter ---

type simpleFilterSpec struct {
	def      *SimpleListFilter
	r        *http.Request
	lookups  []FilterLookup
	val      string
	selected bool
}

func newSimpleFilterSpec(r *http.Request, def *SimpleListFilter, params url.Values) *simpleFilterSpec {
	var val string
	selected := false
	if vals, ok := params[def.ParameterName]; ok && len(vals) > 0 {
		val = vals[len(vals)-1]
		selected = true
		delete(params, def.ParameterName)
	}
	var lookups []FilterLookup
	if def.Lookups != nil {
		lookups = def.Lookups(r)
	}
	return &simpleFilterSpec{
		def:      def,
		r:        r,
		lookups:  lookups,
		val:      val,
		selected: selected,
	}
}

func (f *simpleFilterSpec) Title() string   { return f.def.Title }
func (f *simpleFilterSpec) HasOutput() bool { return len(f.lookups) > 0 }

func (f *simpleFilterSpec) Apply() (orm.Cond, bool, error) {
	if f.def.Filter == nil {
		return orm.Cond{}, false, nil
	}
	c, ok := f.def.Filter(f.r, f.val, f.selected)
	return c, ok, nil
}

func (f *simpleFilterSpec) ExpectedParameters() []string { return []string{f.def.ParameterName} }

// Facets counts each lookup whose Filter returns a condition. A lookup that
// filters nothing gets no count, as a Django queryset() that returns None.
func (f *simpleFilterSpec) Facets() []facet {
	if f.def.Filter == nil {
		return nil
	}
	var out []facet
	for i, lk := range f.lookups {
		if c, ok := f.def.Filter(f.r, lk.Value, true); ok {
			out = append(out, facet{key: indexKey(i), cond: c, subquery: true})
		}
	}
	return out
}

func (f *simpleFilterSpec) Choices(cl *changeList, counts map[string]int64) []filterChoice {
	out := make([]filterChoice, 0, len(f.lookups)+1)
	out = append(out, filterChoice{
		Selected:    !f.selected,
		QueryString: cl.getQueryString(nil, []string{f.def.ParameterName}),
		Display:     "All",
	})
	for i, lk := range f.lookups {
		display := lk.Display
		if counts != nil {
			if n, ok := counts[indexKey(i)]; ok {
				display = fmt.Sprintf("%s (%d)", display, n)
			} else {
				display += " (-)"
			}
		}
		out = append(out, filterChoice{
			Selected:    f.selected && f.val == lk.Value,
			QueryString: cl.getQueryString(map[string]any{f.def.ParameterName: lk.Value}, nil),
			Display:     display,
		})
	}
	return out
}

// --- BooleanFieldListFilter ---

type booleanFilterSpec struct {
	fieldPath   string
	title       string
	field       *orm.Field
	lookupKwarg string
	lookupNull  string
	val         string
	hasVal      bool
	valNull     string
	hasNull     bool
}

func newBooleanFilterSpec(fieldPath, title string, field *orm.Field, params url.Values) *booleanFilterSpec {
	kwarg := fieldPath + "__exact"
	kwargNull := fieldPath + "__isnull"
	spec := &booleanFilterSpec{
		fieldPath:   fieldPath,
		title:       title,
		field:       field,
		lookupKwarg: kwarg,
		lookupNull:  kwargNull,
	}
	if vals, ok := params[kwarg]; ok && len(vals) > 0 {
		spec.val = vals[len(vals)-1]
		spec.hasVal = true
		delete(params, kwarg)
	}
	if vals, ok := params[kwargNull]; ok && len(vals) > 0 {
		spec.valNull = vals[len(vals)-1]
		spec.hasNull = true
		delete(params, kwargNull)
	}
	return spec
}

func (f *booleanFilterSpec) Title() string   { return f.title }
func (f *booleanFilterSpec) HasOutput() bool { return true }

func (f *booleanFilterSpec) Apply() (orm.Cond, bool, error) {
	var cond orm.Cond
	has := false
	if f.hasVal {
		b, ok := parseFieldValue(f.field, f.val, false)
		if !ok {
			return orm.Cond{}, false, fmt.Errorf("%w: invalid boolean value %q", errIncorrectLookup, f.val)
		}
		cond = orm.Q(f.lookupKwarg, b)
		has = true
	}
	if f.hasNull {
		nc := orm.Q(f.lookupNull, parseIsNullParam(f.valNull))
		if !has {
			cond = nc
			has = true
		} else {
			cond = cond.And(nc)
		}
	}
	return cond, has, nil
}

func (f *booleanFilterSpec) ExpectedParameters() []string {
	return []string{f.lookupKwarg, f.lookupNull}
}

func (f *booleanFilterSpec) Facets() []facet {
	return []facet{
		{key: "true__c", cond: orm.Q(f.fieldPath, true)},
		{key: "false__c", cond: orm.Q(f.fieldPath, false)},
		{key: "null__c", cond: orm.Q(f.lookupNull, true)},
	}
}

func (f *booleanFilterSpec) Choices(cl *changeList, counts map[string]int64) []filterChoice {
	yesLabel, noLabel, unknownLabel := "Yes", "No", "Unknown"
	if f.field != nil && f.field.HasChoices {
		for _, ch := range f.field.FlatChoices() {
			switch raw := ch.Raw.(type) {
			case bool:
				if raw {
					yesLabel = ch.Label
				} else {
					noLabel = ch.Label
				}
			case nil:
				if ch.Value == "" {
					unknownLabel = ch.Label
				}
			}
		}
	}
	out := []filterChoice{
		{
			Selected:    !f.hasVal && !f.hasNull,
			QueryString: cl.getQueryString(map[string]any{f.lookupKwarg: nil}, []string{f.lookupNull}),
			Display:     "All",
		},
		{
			Selected:    f.hasVal && f.val == "1" && !f.hasNull,
			QueryString: cl.getQueryString(map[string]any{f.lookupKwarg: "1"}, []string{f.lookupNull}),
			Display:     withCount(yesLabel, counts, "true__c"),
		},
		{
			Selected:    f.hasVal && f.val == "0" && !f.hasNull,
			QueryString: cl.getQueryString(map[string]any{f.lookupKwarg: "0"}, []string{f.lookupNull}),
			Display:     withCount(noLabel, counts, "false__c"),
		},
	}
	if f.field != nil && f.field.Null {
		out = append(out, filterChoice{
			Selected:    f.hasNull && f.valNull == "True",
			QueryString: cl.getQueryString(map[string]any{f.lookupNull: "True"}, []string{f.lookupKwarg}),
			Display:     withCount(unknownLabel, counts, "null__c"),
		})
	}
	return out
}

// --- ChoicesFieldListFilter ---

type choicesFilterSpec struct {
	fieldPath   string
	title       string
	field       *orm.Field
	lookupKwarg string
	lookupNull  string
	vals        []string
	valNull     string
	hasNull     bool
}

func newChoicesFilterSpec(fieldPath, title string, field *orm.Field, params url.Values) *choicesFilterSpec {
	kwarg := fieldPath + "__exact"
	kwargNull := fieldPath + "__isnull"
	spec := &choicesFilterSpec{
		fieldPath:   fieldPath,
		title:       title,
		field:       field,
		lookupKwarg: kwarg,
		lookupNull:  kwargNull,
	}
	if vals, ok := params[kwarg]; ok && len(vals) > 0 {
		spec.vals = slices.Clone(vals)
		delete(params, kwarg)
	}
	if vals, ok := params[kwargNull]; ok && len(vals) > 0 {
		spec.valNull = vals[len(vals)-1]
		spec.hasNull = true
		delete(params, kwargNull)
	}
	return spec
}

func (f *choicesFilterSpec) Title() string   { return f.title }
func (f *choicesFilterSpec) HasOutput() bool { return true }

func (f *choicesFilterSpec) Apply() (orm.Cond, bool, error) {
	var cond orm.Cond
	has := false
	if len(f.vals) > 0 {
		var orCond orm.Cond
		for i, raw := range f.vals {
			v, ok := parseFieldValue(f.field, raw, false)
			if !ok {
				return orm.Cond{}, false, fmt.Errorf("%w: invalid choice %q", errIncorrectLookup, raw)
			}
			leaf := orm.Q(f.lookupKwarg, v)
			if i == 0 {
				orCond = leaf
			} else {
				orCond = orCond.Or(leaf)
			}
		}
		cond = orCond
		has = true
	}
	if f.hasNull {
		nc := orm.Q(f.lookupNull, parseIsNullParam(f.valNull))
		if !has {
			cond = nc
			has = true
		} else {
			cond = cond.And(nc)
		}
	}
	return cond, has, nil
}

func (f *choicesFilterSpec) ExpectedParameters() []string {
	return []string{f.lookupKwarg, f.lookupNull}
}

// Facets counts each choice, the None choice by isnull.
func (f *choicesFilterSpec) Facets() []facet {
	flat := f.field.FlatChoices()
	out := make([]facet, len(flat))
	for i, ch := range flat {
		out[i] = facet{key: indexKey(i), cond: orm.Q(f.lookupNull, true)}
		if !isNoneChoice(ch) {
			out[i].cond = orm.Q(f.lookupKwarg, choiceValue(f.field, ch))
		}
	}
	return out
}

// isNoneChoice reports whether ch is the choice of a NULL value, Django's
// (None, label).
func isNoneChoice(ch orm.Choice) bool { return ch.Raw == nil && ch.Value == "" }

// choiceValue returns the value of ch for a lookup on f.
func choiceValue(f *orm.Field, ch orm.Choice) any {
	if ch.Raw != nil {
		return ch.Raw
	}
	if v, ok := parseFieldValue(f, ch.Value, false); ok {
		return v
	}
	return ch.Value
}

func (f *choicesFilterSpec) Choices(cl *changeList, counts map[string]int64) []filterChoice {
	flat := f.field.FlatChoices()
	out := make([]filterChoice, 0, len(flat)+1)
	out = append(out, filterChoice{
		Selected:    len(f.vals) == 0 && !f.hasNull,
		QueryString: cl.getQueryString(nil, []string{f.lookupKwarg, f.lookupNull}),
		Display:     "All",
	})
	noneTitle := ""
	for i, ch := range flat {
		if isNoneChoice(ch) {
			noneTitle = withCount(ch.Label, counts, indexKey(i))
			continue
		}
		out = append(out, filterChoice{
			Selected:    slices.Contains(f.vals, ch.Value),
			QueryString: cl.getQueryString(map[string]any{f.lookupKwarg: ch.Value}, []string{f.lookupNull}),
			Display:     withCount(ch.Label, counts, indexKey(i)),
		})
	}
	if noneTitle != "" {
		out = append(out, filterChoice{
			Selected:    f.hasNull && parseIsNullParam(f.valNull),
			QueryString: cl.getQueryString(map[string]any{f.lookupNull: "True"}, []string{f.lookupKwarg}),
			Display:     noneTitle,
		})
	}
	return out
}

// --- RelatedFieldListFilter & RelatedOnlyFieldListFilter ---

type relatedFilterSpec struct {
	fieldPath    string
	title        string
	targetModel  *orm.Model
	lookupKwarg  string
	lookupNull   string
	vals         []string
	valNull      string
	hasNull      bool
	choices      [][2]string
	includeEmpty bool
	emptyDisplay string
}

func (s *AdminSite) newRelatedFilterSpec(
	ctx context.Context,
	r *http.Request,
	e *entry,
	fieldPath, title string,
	info orm.LookupPathInfo,
	onlyUsed bool,
	emptyDisplay string,
	params url.Values,
	baseSpec querySpec,
) (*relatedFilterSpec, error) {
	target := info.TargetModel
	if target == nil || target.PrimaryKey() == nil {
		return nil, nil
	}
	pkAttr := target.PrimaryKey().Attr
	kwarg := fieldPath + "__" + pkAttr + "__exact"
	kwargNull := fieldPath + "__isnull"

	spec := &relatedFilterSpec{
		fieldPath:    fieldPath,
		title:        title,
		targetModel:  target,
		lookupKwarg:  kwarg,
		lookupNull:   kwargNull,
		includeEmpty: info.IsM2M || info.Reverse != nil || (info.Field != nil && info.Field.Null),
		emptyDisplay: emptyDisplay,
	}
	if vals, ok := params[kwarg]; ok && len(vals) > 0 {
		spec.vals = slices.Clone(vals)
		delete(params, kwarg)
	}
	if vals, ok := params[kwargNull]; ok && len(vals) > 0 {
		spec.valNull = vals[len(vals)-1]
		spec.hasNull = true
		delete(params, kwargNull)
	}

	// Determine ordering from the related model's ModelAdmin (if registered),
	// falling back to related Model.Ordering(), then PK.
	ordering := s.relatedOrdering(r, target)

	var limitPKs []any
	if onlyUsed {
		rawPKs, err := e.ops.distinctValues(ctx, baseSpec, fieldPath+"__pk")
		if err != nil {
			return nil, err
		}
		limitPKs = make([]any, 0, len(rawPKs))
		for _, v := range rawPKs {
			if v != nil {
				limitPKs = append(limitPKs, v)
			}
		}
	}

	// Like Django's Field.get_choices the choices of a forward relation are
	// the rows that match its LimitChoicesTo; the rows in use replace them.
	var where orm.Cond
	if !onlyUsed && info.Field != nil {
		where = info.Field.Rel.LimitChoicesTo()
	}
	rows, err := orm.LabeledRowsWhere(ctx, s.db, target, where, ordering, limitPKs)
	if err != nil {
		return nil, err
	}
	spec.choices = rows
	return spec, nil
}

func (f *relatedFilterSpec) Title() string { return f.title }

func (f *relatedFilterSpec) HasOutput() bool {
	extra := 0
	if f.includeEmpty {
		extra = 1
	}
	return len(f.choices)+extra > 1
}

func (f *relatedFilterSpec) Apply() (orm.Cond, bool, error) {
	var cond orm.Cond
	has := false
	pkField := f.targetModel.PrimaryKey()
	if len(f.vals) > 0 {
		var orCond orm.Cond
		for i, raw := range f.vals {
			v, ok := parseFieldValue(pkField, raw, false)
			if !ok {
				return orm.Cond{}, false, fmt.Errorf("%w: invalid related pk %q", errIncorrectLookup, raw)
			}
			leaf := orm.Q(f.lookupKwarg, v)
			if i == 0 {
				orCond = leaf
			} else {
				orCond = orCond.Or(leaf)
			}
		}
		cond = orCond
		has = true
	}
	if f.hasNull {
		nc := orm.Q(f.lookupNull, parseIsNullParam(f.valNull))
		if !has {
			cond = nc
			has = true
		} else {
			cond = cond.And(nc)
		}
	}
	return cond, has, nil
}

func (f *relatedFilterSpec) ExpectedParameters() []string {
	return []string{f.lookupKwarg, f.lookupNull}
}

// Facets counts each related object by its primary key, and the rows
// without one.
func (f *relatedFilterSpec) Facets() []facet {
	pkField := f.targetModel.PrimaryKey()
	out := make([]facet, 0, len(f.choices)+1)
	for _, pair := range f.choices {
		if v, ok := parseFieldValue(pkField, pair[0], false); ok {
			out = append(out, facet{key: pair[0] + "__c", cond: orm.Q(f.lookupKwarg, v)})
		}
	}
	if f.includeEmpty {
		out = append(out, facet{key: "__c", cond: orm.Q(f.lookupNull, true)})
	}
	return out
}

func (f *relatedFilterSpec) Choices(cl *changeList, counts map[string]int64) []filterChoice {
	out := make([]filterChoice, 0, len(f.choices)+2)
	out = append(out, filterChoice{
		Selected:    len(f.vals) == 0 && !f.hasNull,
		QueryString: cl.getQueryString(nil, []string{f.lookupKwarg, f.lookupNull}),
		Display:     "All",
	})
	for _, pair := range f.choices {
		pkStr, label := pair[0], pair[1]
		out = append(out, filterChoice{
			Selected:    slices.Contains(f.vals, pkStr),
			QueryString: cl.getQueryString(map[string]any{f.lookupKwarg: pkStr}, []string{f.lookupNull}),
			Display:     withCount(label, counts, pkStr+"__c"),
		})
	}
	if f.includeEmpty {
		out = append(out, filterChoice{
			Selected:    f.hasNull && parseIsNullParam(f.valNull),
			QueryString: cl.getQueryString(map[string]any{f.lookupNull: "True"}, []string{f.lookupKwarg}),
			Display:     withCount(f.emptyDisplay, counts, "__c"),
		})
	}
	return out
}

// --- DateFieldListFilter ---

type dateFilterLink struct {
	title  string
	params map[string]string
}

type dateFilterSpec struct {
	fieldPath    string
	fieldGeneric string
	title        string
	field        *orm.Field
	sinceKwarg   string
	untilKwarg   string
	nullKwarg    string
	dateParams   map[string]string
	links        []dateFilterLink
}

func newDateFilterSpec(fieldPath, title string, field *orm.Field, params url.Values) *dateFilterSpec {
	generic := fieldPath + "__"
	sinceKwarg := fieldPath + "__gte"
	untilKwarg := fieldPath + "__lt"
	nullKwarg := fieldPath + "__isnull"

	dateParams := make(map[string]string)
	for k, vals := range params {
		if strings.HasPrefix(k, generic) && len(vals) > 0 {
			dateParams[k] = vals[len(vals)-1]
		}
	}
	delete(params, sinceKwarg)
	delete(params, untilKwarg)
	if field != nil && field.Null {
		delete(params, nullKwarg)
	}

	now := time.Now().UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	tomorrow := today.AddDate(0, 0, 1)
	past7 := today.AddDate(0, 0, -7)
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	nextMonth := monthStart.AddDate(0, 1, 0)
	yearStart := time.Date(now.Year(), 1, 1, 0, 0, 0, 0, time.UTC)
	nextYear := yearStart.AddDate(1, 0, 0)

	fmtVal := func(t time.Time) string {
		if field != nil && field.Kind == orm.KindDateTime {
			return t.Format(time.RFC3339)
		}
		return t.Format("2006-01-02")
	}

	links := []dateFilterLink{
		{title: "Any date", params: map[string]string{}},
		{title: "Today", params: map[string]string{sinceKwarg: fmtVal(today), untilKwarg: fmtVal(tomorrow)}},
		{title: "Past 7 days", params: map[string]string{sinceKwarg: fmtVal(past7), untilKwarg: fmtVal(tomorrow)}},
		{title: "This month", params: map[string]string{sinceKwarg: fmtVal(monthStart), untilKwarg: fmtVal(nextMonth)}},
		{title: "This year", params: map[string]string{sinceKwarg: fmtVal(yearStart), untilKwarg: fmtVal(nextYear)}},
	}
	if field != nil && field.Null {
		links = append(links,
			dateFilterLink{title: "No date", params: map[string]string{nullKwarg: "True"}},
			dateFilterLink{title: "Has date", params: map[string]string{nullKwarg: "False"}},
		)
	}

	return &dateFilterSpec{
		fieldPath:    fieldPath,
		fieldGeneric: generic,
		title:        title,
		field:        field,
		sinceKwarg:   sinceKwarg,
		untilKwarg:   untilKwarg,
		nullKwarg:    nullKwarg,
		dateParams:   dateParams,
		links:        links,
	}
}

func (f *dateFilterSpec) Title() string   { return f.title }
func (f *dateFilterSpec) HasOutput() bool { return true }

func (f *dateFilterSpec) Apply() (orm.Cond, bool, error) {
	var cond orm.Cond
	has := false
	for _, key := range []string{f.sinceKwarg, f.untilKwarg} {
		raw, ok := f.dateParams[key]
		if !ok {
			continue
		}
		v, valid := parseFieldValue(f.field, raw, false)
		if !valid {
			return orm.Cond{}, false, fmt.Errorf("%w: invalid date %q", errIncorrectLookup, raw)
		}
		leaf := orm.Q(key, v)
		if !has {
			cond = leaf
			has = true
		} else {
			cond = cond.And(leaf)
		}
	}
	if raw, ok := f.dateParams[f.nullKwarg]; ok && f.field != nil && f.field.Null {
		leaf := orm.Q(f.nullKwarg, parseIsNullParam(raw))
		if !has {
			cond = leaf
			has = true
		} else {
			cond = cond.And(leaf)
		}
	}
	return cond, has, nil
}

func (f *dateFilterSpec) ExpectedParameters() []string {
	params := []string{f.sinceKwarg, f.untilKwarg}
	if f.field != nil && f.field.Null {
		params = append(params, f.nullKwarg)
	}
	return params
}

// Facets counts the rows that each link selects.
func (f *dateFilterSpec) Facets() []facet {
	out := make([]facet, len(f.links))
	for i, lk := range f.links {
		args := make([]any, 0, 2*len(lk.params))
		for k, raw := range lk.params {
			var v any = parseIsNullParam(raw)
			if k != f.nullKwarg {
				v, _ = parseFieldValue(f.field, raw, false)
			}
			args = append(args, k, v)
		}
		out[i] = facet{key: indexKey(i), cond: orm.Q(args...)}
	}
	return out
}

func (f *dateFilterSpec) Choices(cl *changeList, counts map[string]int64) []filterChoice {
	out := make([]filterChoice, 0, len(f.links))
	for i, lk := range f.links {
		newParams := make(map[string]any, len(lk.params))
		for k, v := range lk.params {
			newParams[k] = v
		}
		out = append(out, filterChoice{
			Selected:    mapsEqualString(f.dateParams, lk.params),
			QueryString: cl.getQueryString(newParams, []string{f.fieldGeneric}),
			Display:     withCount(lk.title, counts, indexKey(i)),
		})
	}
	return out
}

func mapsEqualString(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if bv, ok := b[k]; !ok || bv != v {
			return false
		}
	}
	return true
}

// --- AllValuesFieldListFilter ---

type allValuesFilterSpec struct {
	fieldPath    string
	title        string
	field        *orm.Field
	lookupKwarg  string
	lookupNull   string
	vals         []string
	valNull      string
	hasNull      bool
	distinctVals []any
	emptyDisplay string
}

func (s *AdminSite) newAllValuesFilterSpec(
	ctx context.Context,
	e *entry,
	fieldPath, title string,
	info orm.LookupPathInfo,
	emptyDisplay string,
	params url.Values,
	baseSpec querySpec,
) (*allValuesFilterSpec, error) {
	kwarg := fieldPath
	kwargNull := fieldPath + "__isnull"
	spec := &allValuesFilterSpec{
		fieldPath:    fieldPath,
		title:        title,
		field:        info.Field,
		lookupKwarg:  kwarg,
		lookupNull:   kwargNull,
		emptyDisplay: emptyDisplay,
	}
	if vals, ok := params[kwarg]; ok && len(vals) > 0 {
		spec.vals = slices.Clone(vals)
		delete(params, kwarg)
	}
	if vals, ok := params[kwargNull]; ok && len(vals) > 0 {
		spec.valNull = vals[len(vals)-1]
		spec.hasNull = true
		delete(params, kwargNull)
	}
	// A field of a related model lists the values of every related row.
	var (
		distinctVals []any
		err          error
	)
	if strings.Contains(fieldPath, "__") && info.Model != nil && info.Field != nil {
		distinctVals, err = orm.DistinctValues(ctx, s.db, info.Model, info.Field.Attr)
	} else {
		distinctVals, err = e.ops.distinctValues(ctx, baseSpec, fieldPath)
	}
	if err != nil {
		return nil, err
	}
	spec.distinctVals = distinctVals
	return spec, nil
}

func (f *allValuesFilterSpec) Title() string   { return f.title }
func (f *allValuesFilterSpec) HasOutput() bool { return true }

func (f *allValuesFilterSpec) Apply() (orm.Cond, bool, error) {
	var cond orm.Cond
	has := false
	if len(f.vals) > 0 {
		var orCond orm.Cond
		for i, raw := range f.vals {
			v, ok := parseFieldValue(f.field, raw, false)
			if !ok {
				return orm.Cond{}, false, fmt.Errorf("%w: invalid value %q for %s", errIncorrectLookup, raw, f.lookupKwarg)
			}
			leaf := orm.Q(f.lookupKwarg, v)
			if i == 0 {
				orCond = leaf
			} else {
				orCond = orCond.Or(leaf)
			}
		}
		cond = orCond
		has = true
	}
	if f.hasNull {
		nc := orm.Q(f.lookupNull, parseIsNullParam(f.valNull))
		if !has {
			cond = nc
			has = true
		} else {
			cond = cond.And(nc)
		}
	}
	return cond, has, nil
}

func (f *allValuesFilterSpec) ExpectedParameters() []string {
	return []string{f.lookupKwarg, f.lookupNull}
}

// Facets counts each value, NULL by isnull.
func (f *allValuesFilterSpec) Facets() []facet {
	out := make([]facet, len(f.distinctVals))
	for i, val := range f.distinctVals {
		out[i] = facet{key: indexKey(i), cond: orm.Q(f.lookupNull, true)}
		if val != nil {
			out[i].cond = orm.Q(f.lookupKwarg, val)
		}
	}
	return out
}

func (f *allValuesFilterSpec) Choices(cl *changeList, counts map[string]int64) []filterChoice {
	out := make([]filterChoice, 0, len(f.distinctVals)+2)
	out = append(out, filterChoice{
		Selected:    len(f.vals) == 0 && !f.hasNull,
		QueryString: cl.getQueryString(nil, []string{f.lookupKwarg, f.lookupNull}),
		Display:     "All",
	})
	includeNone := false
	emptyTitle := f.emptyDisplay
	for i, val := range f.distinctVals {
		if val == nil {
			includeNone = true
			emptyTitle = withCount(emptyTitle, counts, indexKey(i))
			continue
		}
		s := fmt.Sprint(val)
		out = append(out, filterChoice{
			Selected:    slices.Contains(f.vals, s),
			QueryString: cl.getQueryString(map[string]any{f.lookupKwarg: s}, []string{f.lookupNull}),
			Display:     withCount(s, counts, indexKey(i)),
		})
	}
	if includeNone {
		out = append(out, filterChoice{
			Selected:    f.hasNull && parseIsNullParam(f.valNull),
			QueryString: cl.getQueryString(map[string]any{f.lookupNull: "True"}, []string{f.lookupKwarg}),
			Display:     emptyTitle,
		})
	}
	return out
}

// --- EmptyFieldListFilter ---

type emptyFilterSpec struct {
	fieldPath   string
	title       string
	field       *orm.Field
	lookupKwarg string
	val         string
	hasVal      bool
}

func newEmptyFilterSpec(fieldPath, title string, field *orm.Field, params url.Values) *emptyFilterSpec {
	kwarg := fieldPath + "__isempty"
	spec := &emptyFilterSpec{
		fieldPath:   fieldPath,
		title:       title,
		field:       field,
		lookupKwarg: kwarg,
	}
	if vals, ok := params[kwarg]; ok && len(vals) > 0 {
		spec.val = vals[len(vals)-1]
		spec.hasVal = true
		delete(params, kwarg)
	}
	return spec
}

func (f *emptyFilterSpec) Title() string   { return f.title }
func (f *emptyFilterSpec) HasOutput() bool { return true }

func (f *emptyFilterSpec) Apply() (orm.Cond, bool, error) {
	if !f.hasVal {
		return orm.Cond{}, false, nil
	}
	if f.val != "0" && f.val != "1" {
		return orm.Cond{}, false, fmt.Errorf("%w: invalid isempty value %q", errIncorrectLookup, f.val)
	}
	if f.val == "1" {
		return f.lookupCondition(), true, nil
	}
	return orm.Not(f.lookupCondition()), true, nil
}

// lookupCondition matches the rows whose field is empty: the empty string
// of a text field, or NULL, Django's get_lookup_condition.
func (f *emptyFilterSpec) lookupCondition() orm.Cond {
	var emptyCond orm.Cond
	hasEmpty := false
	if f.field != nil && isTextKind(f.field.Kind) {
		emptyCond = orm.Q(f.fieldPath, "")
		hasEmpty = true
	}
	if f.field != nil && (f.field.Null || f.field.Rel != nil) {
		nc := orm.Q(f.fieldPath+"__isnull", true)
		if !hasEmpty {
			emptyCond = nc
			hasEmpty = true
		} else {
			emptyCond = emptyCond.Or(nc)
		}
	}
	if !hasEmpty {
		emptyCond = orm.Q(f.fieldPath+"__isnull", true)
	}
	return emptyCond
}

func (f *emptyFilterSpec) ExpectedParameters() []string { return []string{f.lookupKwarg} }

func (f *emptyFilterSpec) Facets() []facet {
	return []facet{
		{key: "empty__c", cond: f.lookupCondition()},
		{key: "not_empty__c", cond: orm.Not(f.lookupCondition())},
	}
}

func (f *emptyFilterSpec) Choices(cl *changeList, counts map[string]int64) []filterChoice {
	return []filterChoice{
		{
			Selected:    !f.hasVal,
			QueryString: cl.getQueryString(map[string]any{f.lookupKwarg: nil}, nil),
			Display:     "All",
		},
		{
			Selected:    f.hasVal && f.val == "1",
			QueryString: cl.getQueryString(map[string]any{f.lookupKwarg: "1"}, nil),
			Display:     withCount("Empty", counts, "empty__c"),
		},
		{
			Selected:    f.hasVal && f.val == "0",
			QueryString: cl.getQueryString(map[string]any{f.lookupKwarg: "0"}, nil),
			Display:     withCount("Not empty", counts, "not_empty__c"),
		},
	}
}
