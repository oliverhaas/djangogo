package modelformsets_test

import (
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/forms"
	"github.com/oliverhaas/djangogo/orm"
)

// The tests of Django's tests/model_formsets/test_uuid.py whose parent has a
// UUID primary key or a UUID alternate key are not ported: a Go relation
// points at an integer primary key.

// Ported from Django: tests/model_formsets/test_uuid.py::InlineFormsetTests.test_inlineformset_factory_nulls_default_pks
func TestInlineformsetFactoryNullsDefaultPKs(t *testing.T) {
	t.Skip("FIXME-PORT: Go: relations point at integer primary keys only, " +
		"Django: an inline formset of UUIDPKChild under a UUIDPKParent")
}

// Ported from Django: tests/model_formsets/test_uuid.py::InlineFormsetTests.test_inlineformset_factory_ignores_default_pks_on_submit
func TestInlineformsetFactoryIgnoresDefaultPKsOnSubmit(t *testing.T) {
	t.Skip("FIXME-PORT: Go: relations point at integer primary keys only, " +
		"Django: an inline formset of UUIDPKChild under a UUIDPKParent")
}

// Ported from Django: tests/model_formsets/test_uuid.py::InlineFormsetTests.test_inlineformset_factory_nulls_default_pks_uuid_parent_auto_child
func TestInlineformsetFactoryNullsDefaultPKsUUIDParentAutoChild(t *testing.T) {
	t.Skip("FIXME-PORT: Go: relations point at integer primary keys only, " +
		"Django: an inline formset of AutoPKChildOfUUIDPKParent under a UUIDPKParent")
}

// Ported from Django: tests/model_formsets/test_uuid.py::InlineFormsetTests.test_inlineformset_factory_nulls_default_pks_auto_parent_uuid_child
//
// #24958 - Variant of test_inlineformset_factory_nulls_default_pks for
// the case of a parent object with an AutoField primary key and a child
// object with a UUID primary key.
func TestInlineformsetFactoryNullsDefaultPKsAutoParentUUIDChild(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		formSet := forms.NewInlineFormSet[AutoPKParent, UUIDPKChildOfAutoPKParent](db, "")
		parent, ok := formSet.Forms()[0].FieldDef("parent")
		if !ok {
			t.Fatal("the form has no parent field")
		}
		djangotest.AssertEqual(t, parent.Base().Initial, nil)
	})
}

// Ported from Django: tests/model_formsets/test_uuid.py::InlineFormsetTests.test_inlineformset_factory_nulls_default_pks_child_editable_pk
func TestInlineformsetFactoryNullsDefaultPKsChildEditablePK(t *testing.T) {
	t.Skip("FIXME-PORT: Go: relations point at integer primary keys only, " +
		"Django: an inline formset of ChildWithEditablePK under a UUIDPKParent")
}

// Ported from Django: tests/model_formsets/test_uuid.py::InlineFormsetTests.test_inlineformset_factory_nulls_default_pks_alternate_key_relation
func TestInlineformsetFactoryNullsDefaultPKsAlternateKeyRelation(t *testing.T) {
	t.Skip("FIXME-PORT: Go: relations have no to_field, " +
		"Django: ChildRelatedViaAK points at the uuid of ParentWithUUIDAlternateKey")
}

// Ported from Django: tests/model_formsets/test_uuid.py::InlineFormsetTests.test_inlineformset_factory_nulls_default_pks_alternate_key_relation_data
func TestInlineformsetFactoryNullsDefaultPKsAlternateKeyRelationData(t *testing.T) {
	t.Skip("FIXME-PORT: Go: relations have no to_field, " +
		"Django: ChildRelatedViaAK points at the uuid of ParentWithUUIDAlternateKey")
}
