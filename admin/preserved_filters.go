package admin

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/oliverhaas/djangogo/urls"
)

// changelistFiltersVar is the query parameter that carries the filters of a
// changelist to the other pages of its model, Django's "_changelist_filters".
const changelistFiltersVar = "_changelist_filters"

// preserveFilters returns e's PreserveFilters (default true).
func (e *entry) preserveFilters() bool {
	if e.admin.PreserveFilters != nil {
		return *e.admin.PreserveFilters
	}
	return true
}

// preservedFilters returns the changelist filters of the request as the query
// string "_changelist_filters=...", or "" when there are none, as Django's
// ModelAdmin.get_preserved_filters. On the changelist the filters are its
// query string; on the other pages of the model they are the value of
// _changelist_filters.
func (s *AdminSite) preservedFilters(r *http.Request, e *entry) string {
	m := urls.Match(r)
	if !e.preserveFilters() || m == nil {
		return ""
	}
	var filters string
	if m.AppName()+":"+m.URLName == defaultSiteName+":"+e.urlName("changelist") {
		filters = encodeQueryDict(parseQSL(r.URL.RawQuery, true))
	} else {
		filters, _ = lastValue(parseQSL(r.URL.RawQuery, true), changelistFiltersVar)
	}
	if filters == "" {
		return ""
	}
	return changelistFiltersVar + "=" + url.QueryEscape(filters)
}

// preservedQSL returns the parameters of the query string of the request
// other than the preserved filters, Django's ModelAdmin._get_preserved_qsl.
func preservedQSL(r *http.Request, preserved string) []queryPair {
	return parseQSL(strings.ReplaceAll(r.URL.RawQuery, preserved, ""), false)
}

// addPreservedFilters returns u with the query parameters of the page added,
// Django's add_preserved_filters template tag. It adds qsl, the preserved
// filters, the popup and to_field parameters and the query of u, in this
// order; a parameter replaces the value of an earlier one with the same key.
// A URL of the changelist of e takes the changelist filters themselves.
func (s *AdminSite) addPreservedFilters(e *entry, u, preserved string, qsl []queryPair, popup bool, toField string) string {
	rest, fragment, hasFragment := strings.Cut(u, "#")
	path, query, _ := strings.Cut(rest, "?")
	var merged queryDict
	merged.update(qsl)
	if preserved != "" {
		filters := parseQSL(preserved, false)
		if s.isChangelistURL(e, path) {
			if v, ok := lastValue(filters, changelistFiltersVar); ok {
				filters = parseQSL(v, false)
			}
		}
		merged.update(filters)
	}
	if popup {
		merged.set(isPopupVar, "1")
	}
	if toField != "" {
		merged.set(toFieldVar, toField)
	}
	merged.update(parseQSL(query, false))
	out := path
	if q := merged.encode(); q != "" {
		out += "?" + q
	}
	if hasFragment && fragment != "" {
		out += "#" + fragment
	}
	return out
}

// isChangelistURL reports whether path resolves to the changelist of e.
func (s *AdminSite) isChangelistURL(e *entry, path string) bool {
	if unescaped, err := url.PathUnescape(path); err == nil {
		path = unescaped
	}
	m, err := s.reverseRouter().Resolve(path)
	return err == nil && m.AppName()+":"+m.URLName == defaultSiteName+":"+e.urlName("changelist")
}

// queryPair is one parameter of a query string.
type queryPair struct{ key, value string }

// parseQSL splits the query string q into its parameters in order, as
// Python's urllib.parse.parse_qsl: "+" and percent escapes decode, and a
// parameter with an empty value is dropped unless keepBlank is set.
func parseQSL(q string, keepBlank bool) []queryPair {
	var out []queryPair
	for part := range strings.SplitSeq(q, "&") {
		if part == "" {
			continue
		}
		k, v, _ := strings.Cut(part, "=")
		if v == "" && !keepBlank {
			continue
		}
		out = append(out, queryPair{unquotePlus(k), unquotePlus(v)})
	}
	return out
}

// unquotePlus decodes s as Python's urllib.parse.unquote_plus; it keeps an
// invalid escape as it is.
func unquotePlus(s string) string {
	if u, err := url.QueryUnescape(s); err == nil {
		return u
	}
	return strings.ReplaceAll(s, "+", " ")
}

// lastValue returns the last value of key in pairs, as Django's
// QueryDict.get.
func lastValue(pairs []queryPair, key string) (string, bool) {
	for i := len(pairs) - 1; i >= 0; i-- {
		if pairs[i].key == key {
			return pairs[i].value, true
		}
	}
	return "", false
}

// encodeQueryDict returns pairs as a query string, as Django's
// QueryDict.urlencode: the values of a key together, the keys in the order
// they first appear.
func encodeQueryDict(pairs []queryPair) string {
	var keys []string
	values := make(map[string][]string)
	for _, p := range pairs {
		if _, ok := values[p.key]; !ok {
			keys = append(keys, p.key)
		}
		values[p.key] = append(values[p.key], p.value)
	}
	var parts []string
	for _, k := range keys {
		for _, v := range values[k] {
			parts = append(parts, url.QueryEscape(k)+"="+url.QueryEscape(v))
		}
	}
	return strings.Join(parts, "&")
}

// queryDict holds query parameters as a Python dict: one value per key, the
// last one set, the keys in the order they were first set.
type queryDict struct {
	keys   []string
	values map[string]string
}

// set sets the value of key.
func (d *queryDict) set(key, value string) {
	if d.values == nil {
		d.values = make(map[string]string)
	}
	if _, ok := d.values[key]; !ok {
		d.keys = append(d.keys, key)
	}
	d.values[key] = value
}

// update sets the parameters of pairs in order.
func (d *queryDict) update(pairs []queryPair) {
	for _, p := range pairs {
		d.set(p.key, p.value)
	}
}

// encode returns the parameters as a query string, as Python's urlencode.
func (d *queryDict) encode() string {
	parts := make([]string, len(d.keys))
	for i, k := range d.keys {
		parts[i] = url.QueryEscape(k) + "=" + url.QueryEscape(d.values[k])
	}
	return strings.Join(parts, "&")
}
