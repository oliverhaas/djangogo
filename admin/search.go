package admin

import (
	"encoding/json"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/oliverhaas/djangogo/orm"
)

// searchSpec is one compiled entry of ModelAdmin.SearchFields.
type searchSpec struct {
	lookup        string     // ORM filter key (e.g. "name__icontains", "age__exact")
	validateField *orm.Field // non-nil for non-text exact lookups
	spawnsDups    bool
}

// buildSearchCond translates searchFields and searchTerm into a single
// orm.Cond (AND over whitespace-split terms, OR over searchFields for each
// term) plus whether any search path traverses a multi-valued relation,
// matching Django's ModelAdmin.get_search_results.
func buildSearchCond(m *orm.Model, searchFields []string, searchTerm string) (orm.Cond, bool, bool) {
	if len(searchFields) == 0 || searchTerm == "" {
		return orm.Cond{}, false, false
	}
	specs := make([]searchSpec, 0, len(searchFields))
	mayHaveDuplicates := false
	for _, raw := range searchFields {
		sp := constructSearchSpec(m, raw)
		specs = append(specs, sp)
		if sp.spawnsDups {
			mayHaveDuplicates = true
		}
	}

	bits := smartSplit(searchTerm)
	if len(bits) == 0 {
		return orm.Cond{}, false, false
	}

	var combined orm.Cond
	hasCond := false
	for _, bit := range bits {
		if len(bit) >= 2 && (bit[0] == '"' || bit[0] == '\'') && bit[0] == bit[len(bit)-1] {
			bit = unescapeStringLiteral(bit)
		}
		var orCond orm.Cond
		hasOR := false
		for _, sp := range specs {
			var val any = bit
			if sp.validateField != nil {
				parsed, ok := coerceExactSearchValue(sp.validateField, bit)
				if !ok {
					continue
				}
				val = parsed
			}
			leaf := orm.Q(sp.lookup, val)
			if !hasOR {
				orCond = leaf
				hasOR = true
			} else {
				orCond = orCond.Or(leaf)
			}
		}
		if !hasOR {
			// No field accepted this search term: return an empty match.
			orCond = orm.Q("pk__in", []int64{})
		}
		if !hasCond {
			combined = orCond
			hasCond = true
		} else {
			combined = combined.And(orCond)
		}
	}
	return combined, mayHaveDuplicates, hasCond
}

// constructSearchSpec parses one entry of SearchFields (with optional "^", "=",
// or "@" prefix or explicit ORM lookup) into a searchSpec.
func constructSearchSpec(m *orm.Model, raw string) searchSpec {
	var lookup string
	var validateField *orm.Field

	switch {
	case strings.HasPrefix(raw, "^"):
		path := normalizeFieldPath(m, strings.TrimPrefix(raw, "^"))
		lookup = path + "__istartswith"
	case strings.HasPrefix(raw, "="):
		path := normalizeFieldPath(m, strings.TrimPrefix(raw, "="))
		lookup = path + "__iexact"
	case strings.HasPrefix(raw, "@"):
		// Full-text search backends are out of scope; fall back to icontains.
		path := normalizeFieldPath(m, strings.TrimPrefix(raw, "@"))
		lookup = path + "__icontains"
	default:
		norm := normalizeFieldPath(m, raw)
		if info, ok := orm.ResolveLookupPath(m, norm); ok && info.Lookup != "" {
			lookup = norm
			if info.Lookup == "exact" {
				targetField := info.Field
				if targetField != nil && targetField.Rel != nil && targetField.Rel.Target != nil && targetField.Rel.Target.PrimaryKey() != nil {
					targetField = targetField.Rel.Target.PrimaryKey()
				}
				if targetField != nil && !isTextKind(targetField.Kind) {
					validateField = targetField
				}
			}
		} else {
			lookup = norm + "__icontains"
		}
	}

	return searchSpec{
		lookup:        lookup,
		validateField: validateField,
		spawnsDups:    orm.LookupSpawnsDuplicates(m, lookup),
	}
}

// isTextKind reports whether k stores plain text where arbitrary strings are
// valid exact-match search terms.
func isTextKind(k orm.Kind) bool {
	switch k {
	case orm.KindChar, orm.KindText, orm.KindSlug, orm.KindEmail, orm.KindURL:
		return true
	default:
		return false
	}
}

// coerceExactSearchValue converts a raw search token into a typed value for a
// non-text exact lookup, returning false when the token is not valid for f
// (mirroring Django's field validation in ModelAdmin.get_search_results).
func coerceExactSearchValue(f *orm.Field, bit string) (any, bool) {
	if f == nil {
		return bit, true
	}
	val, ok := parseFieldValue(f, bit, true)
	if !ok {
		return nil, false
	}
	if f.HasChoices && len(f.FlatChoices()) > 0 {
		if val == nil {
			for _, ch := range f.FlatChoices() {
				if ch.Raw == nil && ch.Value == "" {
					return nil, true
				}
			}
			return nil, false
		}
		matched := false
		for _, ch := range f.FlatChoices() {
			if ch.Value == bit || (ch.Raw != nil && formatAny(ch.Raw) == formatAny(val)) {
				matched = true
				break
			}
		}
		if !matched {
			return nil, false
		}
	}
	return val, true
}

// parseFieldValue converts a query-string or search value to a Go value
// suitable for querying field f. When allowNoneLiteral is true, "None"
// (case-insensitive) converts to nil for nullable boolean fields.
func parseFieldValue(f *orm.Field, raw string, allowNoneLiteral bool) (any, bool) {
	if f == nil {
		return raw, true
	}
	if f.Rel != nil && f.Rel.Target != nil && f.Rel.Target.PrimaryKey() != nil {
		f = f.Rel.Target.PrimaryKey()
	}
	switch f.Kind {
	case orm.KindAuto, orm.KindInt, orm.KindSmallInt, orm.KindBigInt,
		orm.KindPositiveSmallInt, orm.KindPositiveInt, orm.KindPositiveBigInt:
		if n, err := strconv.ParseInt(raw, 10, 64); err == nil {
			return n, true
		}
		// Match Django IntegerField.to_python: accept integral float strings like "3.".
		if fv, err := strconv.ParseFloat(raw, 64); err == nil && !math.IsNaN(fv) && !math.IsInf(fv, 0) && fv == math.Trunc(fv) {
			return int64(fv), true
		}
		return nil, false
	case orm.KindFloat:
		fv, err := strconv.ParseFloat(raw, 64)
		if err != nil || math.IsNaN(fv) || math.IsInf(fv, 0) {
			return nil, false
		}
		return fv, true
	case orm.KindDecimal:
		d, err := orm.ParseDecimal(raw)
		if err != nil {
			return nil, false
		}
		return d, true
	case orm.KindBool:
		switch strings.ToLower(raw) {
		case "true", "1", "t":
			return true, true
		case "false", "0", "f":
			return false, true
		case "none":
			if allowNoneLiteral && f.Null {
				return nil, true
			}
			return nil, false
		default:
			return nil, false
		}
	case orm.KindDate:
		d, err := orm.ParseDate(raw)
		if err != nil {
			return nil, false
		}
		return d, true
	case orm.KindTime:
		t, err := orm.ParseTime(raw)
		if err != nil {
			return nil, false
		}
		return t, true
	case orm.KindDateTime:
		if t, ok := parseDateTimeParam(raw); ok {
			return t, true
		}
		return nil, false
	case orm.KindUUID:
		u, err := orm.ParseUUID(raw)
		if err != nil {
			return nil, false
		}
		return u, true
	case orm.KindJSON:
		if !json.Valid([]byte(raw)) {
			return nil, false
		}
		return json.RawMessage(raw), true
	default:
		return raw, true
	}
}

// parseDateTimeParam parses common ISO/SQL date and datetime strings submitted
// in query parameters or search terms.
func parseDateTimeParam(raw string) (time.Time, bool) {
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05.999999999-07:00",
		"2006-01-02 15:04:05-07:00",
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
		"2006-01-02",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, raw); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// formatAny returns a canonical string representation for comparing parsed values.
func formatAny(v any) string {
	switch x := v.(type) {
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case bool:
		return strconv.FormatBool(x)
	case string:
		return x
	default:
		return ""
	}
}

// smartSplit splits text on whitespace while keeping single- and double-quoted
// substrings intact (handling backslash escapes), porting Django's
// django.utils.text.smart_split.
func smartSplit(text string) []string {
	var out []string
	var cur strings.Builder
	var quote byte
	escaped := false

	for i := 0; i < len(text); i++ {
		ch := text[i]
		if escaped {
			cur.WriteByte(ch)
			escaped = false
			continue
		}
		if ch == '\\' && quote != 0 {
			cur.WriteByte(ch)
			escaped = true
			continue
		}
		if quote != 0 {
			cur.WriteByte(ch)
			if ch == quote {
				quote = 0
			}
			continue
		}
		if ch == '"' || ch == '\'' {
			quote = ch
			cur.WriteByte(ch)
			continue
		}
		if unicode.IsSpace(rune(ch)) {
			if cur.Len() > 0 {
				out = append(out, cur.String())
				cur.Reset()
			}
			continue
		}
		cur.WriteByte(ch)
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}

// unescapeStringLiteral strips outer matching quotes from s and unescapes
// \" / \' / \\ sequences, matching django.utils.text.unescape_string_literal.
func unescapeStringLiteral(s string) string {
	if len(s) < 2 {
		return s
	}
	q := s[0]
	if (q != '"' && q != '\'') || s[len(s)-1] != q {
		return s
	}
	inner := s[1 : len(s)-1]
	inner = strings.ReplaceAll(inner, `\`+string(q), string(q))
	inner = strings.ReplaceAll(inner, `\\`, `\`)
	return inner
}
