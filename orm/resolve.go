package orm

import (
	"errors"
	"sort"
	"strings"
)

// lookupSep separates the parts of a key: relations, the field, and the lookup.
const lookupSep = "__"

// ErrFieldError is matched by every *FieldError, so a caller can test for an
// unresolvable key with errors.Is.
var ErrFieldError = errors.New("orm: field error")

// FieldError reports a key that does not name a field, relation, annotation,
// or lookup of a model. It is the counterpart of Django's FieldError and keeps
// Django's wording after the model name.
type FieldError struct {
	// Model is the name of the model the key was resolved against.
	Model string
	// Message describes the problem, for example
	// "cannot resolve keyword 'x' into field. Choices are: id, name".
	Message string
}

// Error returns the message prefixed with the package and model name.
func (e *FieldError) Error() string { return "orm: " + e.Model + ": " + e.Message }

// Is reports whether target is ErrFieldError.
func (e *FieldError) Is(target error) bool { return target == ErrFieldError }

// hop is one join of a relation traversal: the rows of the target's table
// whose toColumn equals fromColumn of the table the hop starts from.
type hop struct {
	fromColumn string
	target     *Model
	toColumn   string
	// nullable reports that the hop may match no row, so that a filter must
	// not silently drop the starting row (a nullable foreign key).
	nullable bool
	// many reports that the hop may match several rows. No relation sets it
	// yet; it is the flag a reverse foreign key or a many-to-many hop needs.
	many bool
}

// relationPath describes how a named relation of a model is traversed.
type relationPath struct {
	// hops are the joins from the model to target, in traversal order.
	hops []hop
	// target is the model the relation leads to.
	target *Model
	// local is the field of the starting model whose column already holds the
	// target's primary key, which is the case for a forward foreign key. A key
	// that ends on the relation, or on the target's primary key, then needs no
	// join. It is nil for a relation that has no such column.
	local *Field
}

// lookupName resolves one part of a key against model m. It returns the field
// the name addresses, when there is one, and the relation it traverses, when
// it names a relation. A field is addressed by its attribute name ("author"),
// its column ("author_id"), Django's attname for a relation ("<attr>_id"), or
// "pk" for the primary key.
//
// This is the extension point for new relation kinds: a reverse foreign key or
// a many-to-many accessor is a name that returns a relationPath with a nil
// field and a nil local.
func lookupName(m *Model, name string) (*Field, *relationPath, bool) {
	f, ok := lookupField(m, name)
	if !ok {
		return nil, nil, false
	}
	if f.Rel == nil || f.Rel.Kind != RelFK || f.Rel.Target == nil || f.Rel.Target.PrimaryKey() == nil {
		return f, nil, true
	}
	return f, &relationPath{
		hops: []hop{{
			fromColumn: f.Column,
			target:     f.Rel.Target,
			toColumn:   f.Rel.Target.PrimaryKey().Column,
			nullable:   f.Null,
		}},
		target: f.Rel.Target,
		local:  f,
	}, true
}

// lookupField finds the field of m addressed by name: "pk", an attribute
// name, a column name, or a relation's attname.
func lookupField(m *Model, name string) (*Field, bool) {
	if name == "pk" {
		return m.pk, m.pk != nil
	}
	if f, ok := m.byAttr[name]; ok {
		return f, true
	}
	if f, ok := m.byColumn[name]; ok {
		return f, true
	}
	if attr, ok := strings.CutSuffix(name, "_id"); ok {
		if f, found := m.byAttr[attr]; found && f.Rel != nil {
			return f, true
		}
	}
	return nil, false
}

// attname returns Django's attname of f: the attribute name, with "_id"
// appended for a relation. It is the key of a field in Values() rows.
func attname(f *Field) string {
	if f.Rel != nil {
		return f.Attr + "_id"
	}
	return f.Attr
}

// fieldChoices lists the keys that address a field of m, sorted, for the
// "Choices are" part of a FieldError. extra names (annotations) are included.
func fieldChoices(m *Model, extra []string) string {
	seen := make(map[string]bool)
	for _, f := range m.fields {
		seen[f.Attr] = true
		seen[attname(f)] = true
	}
	for _, name := range extra {
		seen[name] = true
	}
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

// fieldPath is a key resolved against a model: the joins to follow, the
// column the key ends on, and the parts that named no field.
type fieldPath struct {
	// hops are the joins from the starting model to the model owning field.
	hops []hop
	// field is the column the key addresses.
	field *Field
	// model owns field.
	model *Model
	// rest holds the trailing parts that are not fields: the lookup.
	rest []string
	// relation reports that the key ended on a relation rather than on a
	// plain field.
	relation bool
}

// resolvePath walks the parts of a key from model m, following relations for
// as long as the next part names a field of the model reached, the way
// Django's Query.names_to_path does. The first part must resolve; extra names
// are the annotation names listed among the choices when it does not.
//
// A key that ends on a forward foreign key, or on the primary key of its
// target, is answered from the local foreign-key column without a join.
func resolvePath(m *Model, parts []string, extra []string) (fieldPath, error) {
	p := fieldPath{model: m}
	var pending *relationPath
	for i, name := range parts {
		scope := p.model
		if pending != nil {
			scope = pending.target
		}
		f, rel, ok := lookupName(scope, name)
		if !ok {
			if i == 0 {
				return p, &FieldError{
					Model:   m.Name(),
					Message: "cannot resolve keyword '" + name + "' into field. Choices are: " + fieldChoices(m, extra),
				}
			}
			p.rest = parts[i:]
			break
		}
		if pending != nil {
			if pending.local != nil && rel == nil && f == pending.target.PrimaryKey() {
				// "author__id" is the local "author_id" column.
				p.field = pending.local
				p.rest = parts[i+1:]
				return p, nil
			}
			p.hops = append(p.hops, pending.hops...)
			p.model = pending.target
			pending = nil
		}
		if rel != nil {
			pending = rel
			continue
		}
		p.field = f
		p.rest = parts[i+1:]
		return p, nil
	}
	if pending == nil {
		return p, &FieldError{Model: m.Name(), Message: "cannot resolve an empty keyword into field"}
	}
	// The key ended on a relation.
	p.relation = true
	if pending.local != nil {
		p.field = pending.local
		return p, nil
	}
	p.hops = append(p.hops, pending.hops...)
	p.model = pending.target
	p.field = pending.target.PrimaryKey()
	return p, nil
}

// resolveFieldPath resolves a key that must name a field and nothing else, as
// in OrderBy, Values, and F. A trailing part that is not a field is an error,
// worded as Django words it.
func resolveFieldPath(m *Model, key string, extra []string) (fieldPath, error) {
	p, err := resolvePath(m, strings.Split(key, lookupSep), extra)
	if err != nil {
		return p, err
	}
	if len(p.rest) == 0 {
		return p, nil
	}
	if p.relation {
		target := p.model
		if rel := p.field.Rel; rel != nil && rel.Target != nil {
			target = rel.Target
		}
		return p, &FieldError{
			Model:   m.Name(),
			Message: "cannot resolve keyword '" + p.rest[0] + "' into field. Choices are: " + fieldChoices(target, nil),
		}
	}
	return p, &FieldError{
		Model:   m.Name(),
		Message: "cannot resolve keyword '" + p.rest[0] + "' into field. Join on '" + p.field.Attr + "' not permitted.",
	}
}
