package orm

import (
	"fmt"
	"slices"
	"strings"
	"unicode"
)

// LabelLower returns the lowercase registry key ("<app>.<name>", or lowercase
// bare name when registered without an app), matching Django's Options.label_lower.
func (m *Model) LabelLower() string {
	return strings.ToLower(m.Label())
}

// Ordering returns a copy of the model's default QuerySet ordering
// (Meta.Ordering), or nil when none is set.
func (m *Model) Ordering() []string {
	if len(m.ordering) == 0 {
		return nil
	}
	return slices.Clone(m.ordering)
}

// UniqueTogether returns a deep copy of the model's unique-together field sets
// (Meta.UniqueTogether), using canonical Go struct field names.
func (m *Model) UniqueTogether() [][]string {
	if len(m.uniqueTogether) == 0 {
		return nil
	}
	out := make([][]string, len(m.uniqueTogether))
	for i, set := range m.uniqueTogether {
		out[i] = slices.Clone(set)
	}
	return out
}

// Indexes returns a deep copy of the model's declared Meta.Indexes. Every
// returned Index carries a non-empty Name and canonical Go struct field names
// (retaining a leading "-" for descending order).
func (m *Model) Indexes() []Index {
	if len(m.indexes) == 0 {
		return nil
	}
	out := make([]Index, len(m.indexes))
	for i, idx := range m.indexes {
		out[i] = Index{Name: idx.Name, Fields: slices.Clone(idx.Fields)}
	}
	return out
}

// VerboseName returns the model's singular human-readable name (Django's
// Meta.verbose_name).
func (m *Model) VerboseName() string { return m.verboseName }

// VerboseNamePlural returns the model's plural human-readable name (Django's
// Meta.verbose_name_plural).
func (m *Model) VerboseNamePlural() string { return m.verboseNamePlural }

// camelCaseToSpaces splits CamelCase into lowercase space-separated words,
// matching Django's django.utils.text.camel_case_to_spaces.
func camelCaseToSpaces(s string) string {
	runes := []rune(s)
	var b strings.Builder
	for i, r := range runes {
		if unicode.IsUpper(r) {
			prevLower := i > 0 && unicode.IsLower(runes[i-1])
			nextNotUpperOrEnd := i+1 < len(runes) && !unicode.IsUpper(runes[i+1])
			if prevLower || nextNotUpperOrEnd {
				b.WriteByte(' ')
			}
			b.WriteRune(unicode.ToLower(r))
			continue
		}
		b.WriteRune(r)
	}
	return strings.TrimSpace(b.String())
}

// resolveMetaField resolves a field reference from Meta.UniqueTogether or
// Meta.Indexes by Go struct name, attribute name, column name, or "<attr>_id".
func resolveMetaField(name string, byName, byAttr, byColumn map[string]*Field) (*Field, bool) {
	if f, ok := byName[name]; ok {
		return f, true
	}
	if f, ok := byAttr[name]; ok {
		return f, true
	}
	if f, ok := byColumn[name]; ok {
		return f, true
	}
	if attr, ok := strings.CutSuffix(name, "_id"); ok {
		if f, found := byAttr[attr]; found && f.Rel != nil {
			return f, true
		}
	}
	return nil, false
}

// applyModelMeta validates and populates the Meta options on m during
// registration.
func applyModelMeta(m *Model, model any) error {
	var meta Meta
	if wm, ok := model.(withMeta); ok {
		meta = wm.Meta()
	}

	m.verboseName = meta.VerboseName
	if m.verboseName == "" {
		m.verboseName = camelCaseToSpaces(m.name)
	}
	m.verboseNamePlural = meta.VerboseNamePlural
	if m.verboseNamePlural == "" {
		m.verboseNamePlural = m.verboseName + "s"
	}

	if len(meta.Ordering) > 0 {
		m.ordering = make([]string, len(meta.Ordering))
		for i, term := range meta.Ordering {
			if term == "" {
				return fmt.Errorf("orm: model %s: Meta.Ordering contains an empty entry", m.name)
			}
			m.ordering[i] = term
		}
	}

	if len(meta.UniqueTogether) > 0 {
		m.uniqueTogether = make([][]string, len(meta.UniqueTogether))
		for i, set := range meta.UniqueTogether {
			if len(set) == 0 {
				return fmt.Errorf("orm: model %s: Meta.UniqueTogether contains an empty field set", m.name)
			}
			norm := make([]string, len(set))
			for j, ref := range set {
				f, ok := resolveMetaField(ref, m.byName, m.byAttr, m.byColumn)
				if !ok {
					return fmt.Errorf("orm: model %s: unique_together names unknown field %q", m.name, ref)
				}
				norm[j] = f.Name
			}
			m.uniqueTogether[i] = norm
		}
	}

	if len(meta.Indexes) > 0 {
		seenNames := make(map[string]bool, len(meta.Indexes))
		m.indexes = make([]Index, len(meta.Indexes))
		for i, idx := range meta.Indexes {
			if len(idx.Fields) == 0 {
				return fmt.Errorf("orm: model %s: index %q requires at least one field", m.name, idx.Name)
			}
			normFields := make([]string, len(idx.Fields))
			colsWithOrder := make([]string, len(idx.Fields))
			for j, raw := range idx.Fields {
				desc := strings.HasPrefix(raw, "-")
				bare := strings.TrimPrefix(raw, "-")
				f, ok := resolveMetaField(bare, m.byName, m.byAttr, m.byColumn)
				if !ok {
					return fmt.Errorf("orm: model %s: index names unknown field %q", m.name, raw)
				}
				if desc {
					normFields[j] = "-" + f.Name
					colsWithOrder[j] = "-" + f.Column
				} else {
					normFields[j] = f.Name
					colsWithOrder[j] = f.Column
				}
			}
			idxName := idx.Name
			if idxName == "" {
				idxName = IndexName(m.table, colsWithOrder)
			}
			if seenNames[idxName] {
				return fmt.Errorf("orm: model %s: duplicate index name %q", m.name, idxName)
			}
			seenNames[idxName] = true
			m.indexes[i] = Index{Name: idxName, Fields: normFields}
		}
	}

	if err := applyConstraints(m, meta.Constraints); err != nil {
		return err
	}

	if err := applyMetaFieldHooks(m, meta); err != nil {
		return err
	}

	return nil
}
