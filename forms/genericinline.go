package forms

import (
	"context"
	"fmt"
	"net/url"
	"reflect"
	"slices"
	"strings"

	"github.com/oliverhaas/djangogo/contenttypes"
	"github.com/oliverhaas/djangogo/orm"
)

// GenericInlineFormSet is a model formset of the rows of T that point at one
// object of any model through a content type field and an object id field,
// the fields of a contenttypes.GenericForeignKey. It is Django's
// generic_inlineformset_factory and BaseGenericInlineFormSet of
// django.contrib.contenttypes.forms. The initial forms edit the rows of the
// object, and the rows that the extra forms add point at it:
//
//	fs := forms.NewGenericInlineFormSet[TaggedItem](db, "", "", forms.WithFields("tag"))
//	bound := fs.WithInstance(animal).Bind(r.PostForm)
//	if bound.IsValid() {
//		tags, err := bound.Save(r.Context())
//	}
//
// The forms leave out the content type and object id fields; Save sets them
// on the rows it adds. The defaults are Django's: three extra forms,
// CanDelete, and the prefix "<app label>-<model>-<content type
// field>-<object id field>". The contenttypes app must be installed.
type GenericInlineFormSet[T any] struct {
	*ModelFormSet[T]
	// SaveAsNew makes every form add a row, for an object that is saved as
	// a new row ("save as new" in the admin), Django's save_as_new.
	SaveAsNew bool
}

// NewGenericInlineFormSet returns an unbound generic inline formset of T for
// no object, built with opts as NewModelForm builds one form. ctField names
// the foreign key of T to contenttypes.ContentType and fkField the object id
// field; "" selects Django's defaults "content_type" and "object_id". A
// field that is missing, or a content type field that is not a foreign key
// to ContentType, makes the formset fail (Err) with Django's message.
func NewGenericInlineFormSet[T any](db *orm.DB, ctField, fkField string, opts ...ModelFormOption) *GenericInlineFormSet[T] {
	if ctField == "" {
		ctField = "content_type"
	}
	if fkField == "" {
		fkField = "object_id"
	}
	var ct, fk *orm.Field
	var err error
	if model, ok := db.Registry().ModelOf(new(T)); ok {
		ct, fk, err = genericFields(model, ctField, fkField)
		if err == nil {
			opts = append(slices.Clip(opts), WithExclude(ct.Attr, fk.Attr))
		}
	}
	s := &GenericInlineFormSet[T]{ModelFormSet: NewModelFormSet[T](db, opts...)}
	s.Extra, s.CanDelete = 3, true
	if s.model == nil {
		return s
	}
	if err != nil {
		s.fail(err)
		return s
	}
	s.Prefix = strings.Join([]string{s.model.App(), strings.ToLower(s.model.Name()), ct.Attr, fk.Attr}, "-")
	s.inline = &inlineSpec{ctField: ct, idField: fk, saveAsNew: &s.SaveAsNew}
	return s
}

// genericFields returns the content type field and the object id field of a
// generic inline formset of model, as generic_inlineformset_factory checks
// them.
func genericFields(model *orm.Model, ctName, fkName string) (ct, fk *orm.Field, err error) {
	ct, ok := model.FieldByAttr(ctName)
	if !ok {
		return nil, nil, fmt.Errorf("forms: %s has no field named '%s'", model.Name(), ctName)
	}
	if ct.Rel == nil || (ct.Rel.Kind != orm.RelFK && ct.Rel.Kind != orm.RelO2O) ||
		ct.Rel.Target.GoType() != reflect.TypeFor[contenttypes.ContentType]() {
		return nil, nil, fmt.Errorf("forms: fk_name '%s.%s' is not a ForeignKey to ContentType", model.Label(), ct.Attr)
	}
	fk, ok = model.FieldByAttr(fkName)
	if !ok {
		return nil, nil, fmt.Errorf("forms: %s has no field named '%s'", model.Name(), fkName)
	}
	return ct, fk, nil
}

// WithInstance returns a copy of the formset for the rows of parent, a
// pointer to an instance of a registered model, Django's instance argument.
// Without it, or with nil, the formset has no initial forms.
func (s *GenericInlineFormSet[T]) WithInstance(parent any) *GenericInlineFormSet[T] {
	c := *s
	c.ModelFormSet = s.derive(s.clone())
	if c.inline == nil {
		return &c
	}
	c.inline.parent, c.inline.parentModel = nil, nil
	c.inline.saveAsNew = &c.SaveAsNew
	if rv := reflect.ValueOf(parent); parent == nil || (rv.Kind() == reflect.Pointer && rv.IsNil()) {
		return &c
	}
	model, ok := s.db.Registry().ModelOf(parent)
	if !ok {
		c.fail(fmt.Errorf("forms: %T is not a registered model", parent))
		return &c
	}
	c.inline.parent, c.inline.parentModel = parent, model
	return &c
}

// Bind returns a copy of the formset bound to data. The receiver stays
// unbound.
func (s *GenericInlineFormSet[T]) Bind(data url.Values) *GenericInlineFormSet[T] {
	c := *s
	c.ModelFormSet = s.ModelFormSet.Bind(data)
	if c.inline != nil {
		c.inline.saveAsNew = &c.SaveAsNew
	}
	return &c
}

// Instance returns the object of the rows, nil for none.
func (s *GenericInlineFormSet[T]) Instance() any {
	if s.inline == nil {
		return nil
	}
	return s.inline.parent
}

// contentType returns the content type of the parent of a generic inline
// formset, creating it when it does not exist yet, as Django's
// get_for_model does.
func (in *inlineSpec) contentType(ctx context.Context, db *orm.DB) (int64, error) {
	ct, err := contenttypes.GetForModel(ctx, db, in.parentModel)
	if err != nil {
		return 0, err
	}
	return ct.ID, nil
}

// objectID returns the primary key of the parent as the value of the object
// id field: as text for a text field, which holds the primary key of any
// model.
func (in *inlineSpec) objectID() any {
	if isStringField(in.idField) {
		key, _ := instancePK(in.parentModel, in.parent)
		return key
	}
	return in.parentPKValue()
}

// genericRows returns the condition that selects the rows of the parent of
// a generic inline formset.
func (in *inlineSpec) genericRows(ctx context.Context, db *orm.DB) (orm.Cond, error) {
	ctID, err := in.contentType(ctx, db)
	if err != nil {
		return orm.Cond{}, err
	}
	return orm.Q(in.ctField.Attr, ctID, in.idField.Attr, in.objectID()), nil
}

// setGenericParent points the instance of an extra form at the parent of a
// generic inline formset, Django's BaseGenericInlineFormSet.save_new.
func (s *ModelFormSet[T]) setGenericParent(ctx context.Context, mf *ModelForm[T]) error {
	in := s.inline
	if !in.parentHasPK() {
		return fmt.Errorf("forms: save the object of %s first", s.model.Label())
	}
	ctID, err := in.contentType(ctx, s.db)
	if err != nil {
		return err
	}
	return PopulateStruct(mf.model, map[string]any{in.ctField.Attr: ctID, in.idField.Attr: in.objectID()}, mf.instance)
}
