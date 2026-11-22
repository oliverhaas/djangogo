package orm

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	"github.com/oliverhaas/djangogo/internal/naming"
)

// PrivateFieldType is implemented by the type of a model struct field that
// has no column of its own, Django's private field, such as
// contenttypes.GenericForeignKey. The model lists such a field in
// PrivateFields and leaves it out of Fields, so no query, form or migration
// sees it.
type PrivateFieldType interface {
	// PrivateField marks the type.
	PrivateField()
}

// GenericRelationType is implemented by the type of a generic relation
// field, contenttypes.GenericRelation[T]. A generic relation of a model leads
// to the rows of T whose content type field refers to the content type of
// the model and whose object id field holds its primary key. Filters follow
// it like a reverse foreign key, Delete deletes the related rows with the
// model's row, and GenericSet manages them.
type GenericRelationType interface {
	PrivateFieldType
	// GenericRelationTarget returns the struct type of the related model.
	GenericRelationTarget() reflect.Type
	// ContentTypeFilter returns the Filter arguments, attribute names and
	// values, that select the content type of m among the rows of the content
	// type model.
	ContentTypeFilter(m *Model) []any
	// ContentTypeID returns the primary key of the content type of m. It
	// creates the content type when it does not exist yet.
	ContentTypeID(ctx context.Context, db *DB, m *Model) (any, error)
}

// ObjectHolder is implemented by the type of a private field that can hold a
// model instance, such as contenttypes.GenericForeignKey. Save and BulkCreate
// refuse an instance whose private field holds an unsaved object, as Django
// does.
type ObjectHolder interface {
	PrivateFieldType
	// HeldObject returns the instance the field holds, or nil.
	HeldObject() any
}

// PrivateFieldAssigner is implemented by the type of a private field that
// GetOrCreate and UpdateOrCreate can assign from their defaults, such as
// contenttypes.GenericForeignKey.
type PrivateFieldAssigner interface {
	PrivateFieldType
	// AssignPrivateField sets the private field pf of instance, a pointer to
	// a struct of model m, to value.
	AssignPrivateField(ctx context.Context, db *DB, m *Model, pf *PrivateField, instance, value any) error
}

// PrivateFieldResolver is implemented by the type of a private field that
// checks its options once every model is registered. Registry.Resolve calls
// it and returns its error.
type PrivateFieldResolver interface {
	PrivateFieldType
	// ResolvePrivateField checks the private field pf of model m.
	ResolvePrivateField(m *Model, pf *PrivateField) error
}

// ObjectPrefetcher is implemented by the type of a private field that refers
// to one instance of any model, such as contenttypes.GenericForeignKey, so
// that PrefetchRelated loads the objects of many instances with one query per
// model. Each method takes instance, a pointer to a model struct that
// declares the field pf.
type ObjectPrefetcher interface {
	PrivateFieldType
	// PrefetchTarget returns the model and the primary key of the object the
	// field refers to, or a nil model when it refers to none.
	PrefetchTarget(ctx context.Context, db *DB, pf *PrivateField, instance any) (*Model, any, error)
	// CachedObject returns the object cached on the field and reports whether
	// one is cached. A cached object can be nil.
	CachedObject(pf *PrivateField, instance any) (any, bool)
	// CacheObject caches obj, a pointer to a model struct or nil, on the
	// field.
	CacheObject(pf *PrivateField, instance, obj any)
}

// PrivateField is a field of a model struct that has no column.
type PrivateField struct {
	// Name is the Go field name, e.g. "ContentObject".
	Name string
	// Attr is the snake_case name, e.g. "content_object".
	Attr string
	// Index is the position of the field in the struct.
	Index int
	// Tag is the field's orm tag, for the package that defines the type.
	Tag string
	// Type is the field's Go type.
	Type reflect.Type
}

// GenericRelation describes a generic relation field of a model.
type GenericRelation struct {
	// Name is the attribute name of the field on Model, e.g. "tags".
	Name string
	// QueryName is the related_query_name: the name that leads from a row of
	// Related back to Model in filters. It is empty when the tag sets none.
	QueryName string
	// Model is the model that declares the field.
	Model *Model
	// Related is the model of the related rows.
	Related *Model
	// ContentType is the foreign key of Related to the content type model,
	// the content_type_field option (default content_type).
	ContentType *Field
	// ObjectID is the field of Related that holds the primary key of a row
	// of Model, the object_id_field option (default object_id).
	ObjectID *Field

	impl         GenericRelationType
	targetType   reflect.Type
	ctFieldName  string
	objFieldName string
	// ctFilter holds the Filter arguments that select the content type.
	ctFilter []any
	// ctConds holds the columns and values of ctFilter.
	ctConds []ctCond
}

// ctCond is a column of the content type table and the value the content
// type of a generic relation's model has in it.
type ctCond struct {
	column string
	value  any
}

var privateFieldIface = reflect.TypeFor[PrivateFieldType]()

// parsePrivateField returns the private field that sf declares and, for a
// generic relation, its description. It reports false when the type of sf is
// not a PrivateFieldType.
func parsePrivateField(sf reflect.StructField, index int) (*PrivateField, *GenericRelation, bool, error) {
	if sf.PkgPath != "" || !sf.Type.Implements(privateFieldIface) {
		return nil, nil, false, nil
	}
	pf := &PrivateField{
		Name:  sf.Name,
		Attr:  naming.SnakeCase(sf.Name),
		Index: index,
		Tag:   sf.Tag.Get("orm"),
		Type:  sf.Type,
	}
	impl, ok := reflect.Zero(sf.Type).Interface().(GenericRelationType)
	if !ok {
		return pf, nil, true, nil
	}
	gr := &GenericRelation{
		Name:         pf.Attr,
		impl:         impl,
		targetType:   impl.GenericRelationTarget(),
		ctFieldName:  "content_type",
		objFieldName: "object_id",
	}
	for token := range strings.SplitSeq(pf.Tag, ";") {
		token = strings.TrimSpace(token)
		key, value, _ := strings.Cut(token, "=")
		switch key {
		case "":
		case "object_id_field":
			gr.objFieldName = value
		case "content_type_field":
			gr.ctFieldName = value
		case "related_query_name":
			gr.QueryName = value
		default:
			return nil, nil, false, fmt.Errorf("field %s: unknown generic relation option %q", sf.Name, token)
		}
	}
	return pf, gr, true, nil
}

// PrivateFields returns the fields of m that have no column, in declaration
// order, generic relations included.
func (m *Model) PrivateFields() []*PrivateField {
	return append([]*PrivateField(nil), m.privateFields...)
}

// PrivateField returns the field without a column whose attribute name is
// name.
func (m *Model) PrivateField(name string) (*PrivateField, bool) {
	for _, pf := range m.privateFields {
		if pf.Attr == name {
			return pf, true
		}
	}
	return nil, false
}

// GenericRelations returns the generic relations that m declares.
func (m *Model) GenericRelations() []*GenericRelation {
	return append([]*GenericRelation(nil), m.genericRels...)
}

// GenericRelation returns the generic relation of m named name.
func (m *Model) GenericRelation(name string) (*GenericRelation, bool) {
	for _, gr := range m.genericRels {
		if gr.Name == name {
			return gr, true
		}
	}
	return nil, false
}

// resolvePrivateFields links every generic relation to its related model and
// to the two fields of that model it uses, then lets each private field
// check its options.
func (r *Registry) resolvePrivateFields() error {
	for _, m := range r.models {
		m.byGenericQuery = nil
	}
	for _, m := range r.models {
		for _, gr := range m.genericRels {
			if err := r.resolveGenericRelation(m, gr); err != nil {
				return err
			}
		}
	}
	for _, m := range r.models {
		for _, pf := range m.privateFields {
			res, ok := reflect.Zero(pf.Type).Interface().(PrivateFieldResolver)
			if !ok {
				continue
			}
			if err := res.ResolvePrivateField(m, pf); err != nil {
				return fmt.Errorf("orm: model %s field %s: %w", m.name, pf.Name, err)
			}
		}
	}
	return nil
}

// resolveGenericRelation links the generic relation gr of m.
func (r *Registry) resolveGenericRelation(m *Model, gr *GenericRelation) error {
	related, ok := r.byType[gr.targetType]
	if !ok {
		return fmt.Errorf("orm: model %s field %s references unregistered model %s",
			m.name, gr.Name, gr.targetType)
	}
	gr.Model, gr.Related = m, related
	ct, ok := related.byAttr[gr.ctFieldName]
	if !ok || ct.Rel == nil || !isForwardToOne(ct.Rel.Kind) || ct.Rel.Target == nil {
		return fmt.Errorf("orm: model %s field %s: %s.%s is not a foreign key to the content types",
			m.name, gr.Name, related.name, gr.ctFieldName)
	}
	obj, ok := related.byAttr[gr.objFieldName]
	if !ok || obj.Rel != nil {
		return fmt.Errorf("orm: model %s field %s: %s has no object id field %q",
			m.name, gr.Name, related.name, gr.objFieldName)
	}
	gr.ContentType, gr.ObjectID = ct, obj
	gr.ctFilter = gr.impl.ContentTypeFilter(m)
	conds, err := ctConditions(ct.Rel.Target, gr.ctFilter)
	if err != nil {
		return fmt.Errorf("orm: model %s field %s: %w", m.name, gr.Name, err)
	}
	gr.ctConds = conds
	if gr.QueryName != "" {
		if related.byGenericQuery == nil {
			related.byGenericQuery = make(map[string]*GenericRelation)
		}
		related.byGenericQuery[gr.QueryName] = gr
	}
	return nil
}

// ctConditions maps the attribute names of a content type filter to the
// columns of the content type model ct.
func ctConditions(ct *Model, filter []any) ([]ctCond, error) {
	if len(filter)%2 != 0 {
		return nil, fmt.Errorf("the content type filter of %s has an odd number of arguments", ct.name)
	}
	conds := make([]ctCond, 0, len(filter)/2)
	for i := 0; i < len(filter); i += 2 {
		attr, _ := filter[i].(string)
		f, ok := ct.byAttr[attr]
		if !ok {
			return nil, fmt.Errorf("the content type model %s has no field %v", ct.name, filter[i])
		}
		conds = append(conds, ctCond{column: f.Column, value: filter[i+1]})
	}
	return conds, nil
}

// contentTypeIDs is the subquery that selects the content type of the model
// of a generic relation: SELECT id FROM the content type table WHERE each
// condition holds.
type contentTypeIDs struct {
	gr *GenericRelation
}

// render writes the SELECT.
func (n contentTypeIDs) render(w *sqlWriter) {
	ct := n.gr.ContentType.Rel.Target
	w.write("SELECT ")
	w.ident(ct.pk.Column)
	w.write(" FROM ")
	w.ident(ct.table)
	for i, c := range n.gr.ctConds {
		if i == 0 {
			w.write(" WHERE ")
		} else {
			w.write(" AND ")
		}
		w.ident(c.column)
		w.write(" = ")
		w.param(c.value)
	}
}

// vtype is the type of the content type's primary key.
func (n contentTypeIDs) vtype() valueType { return fieldValueType(n.gr.ContentType.Rel.Target.pk) }

// aggregate is false.
func (contentTypeIDs) aggregate() bool { return false }

// renderContentTypeRestriction writes the condition that the content type
// column of the related table of gr, joined as alias, refers to the content
// type of gr.Model.
func renderContentTypeRestriction(w *sqlWriter, alias string, gr *GenericRelation) {
	w.column(alias, gr.ContentType.Column)
	w.write(" IN (")
	contentTypeIDs{gr: gr}.render(w)
	w.write(")")
}

// contentTypePredicate returns the condition that the content type column of
// the related table joined as j refers to the content type of gr.Model.
func contentTypePredicate(j *join, gr *GenericRelation) *predicate {
	return &predicate{
		lookup: lookupIn,
		lhs:    colNode{join: j, field: gr.ContentType},
		rhs:    []sqlNode{contentTypeIDs{gr: gr}},
	}
}

// genericPath returns the relation path of the generic relation gr from
// gr.Model to gr.Related.
func genericPath(gr *GenericRelation) *relationPath {
	return &relationPath{
		hops: []hop{{
			fromColumn: gr.Model.pk.Column,
			target:     gr.Related,
			toColumn:   gr.ObjectID.Column,
			nullable:   true,
			many:       true,
			generic:    gr,
		}},
		target: gr.Related,
	}
}

// genericQueryPath returns the relation path of the related query name of gr,
// from gr.Related back to gr.Model.
func genericQueryPath(gr *GenericRelation) *relationPath {
	return &relationPath{
		hops: []hop{{
			fromColumn:      gr.ObjectID.Column,
			target:          gr.Model,
			toColumn:        gr.Model.pk.Column,
			nullable:        true,
			generic:         gr,
			genericOnParent: true,
		}},
		target: gr.Model,
	}
}

// genericFilter returns a clone of q, a query of gr.Related, restricted to
// the rows that belong to the rows of gr.Model whose primary keys are pks.
func (q *query) genericFilter(gr *GenericRelation, pks []any) *query {
	ct := newQuery(q.db, gr.ContentType.Rel.Target).filter(gr.ctFilter, false)
	return q.filter([]any{
		gr.ContentType.Attr + "__in", ct,
		gr.ObjectID.Attr + "__in", pks,
	}, false)
}

// pks returns the primary keys of the rows q selects.
func (q *query) pks(ctx context.Context) ([]any, error) {
	return (&ValuesListQuerySet{q: q.withValues([]string{"pk"})}).Flat(ctx)
}
