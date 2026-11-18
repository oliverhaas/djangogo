package checks_test

import (
	"testing"

	"github.com/oliverhaas/djangogo/checks"
	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
)

// messageObjects returns the object of every message, in order.
func messageObjects(msgs []checks.Message) []any {
	out := make([]any, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, m.Obj)
	}
	return out
}

// TestModelChecksHonorApps checks every model for a run without app labels
// and only the models of the named apps otherwise.
func TestModelChecksHonorApps(t *testing.T) {
	type First struct {
		Field int64 `orm:"pk;null"`
	}
	type Second struct {
		Field int64 `orm:"pk;null"`
	}
	reg := orm.NewRegistry()
	if _, err := reg.RegisterApp("one", &First{}); err != nil {
		t.Fatalf("RegisterApp: %v", err)
	}
	if _, err := reg.RegisterApp("two", &Second{}); err != nil {
		t.Fatalf("RegisterApp: %v", err)
	}

	djangotest.AssertEqual(t, messageObjects(modelMessages(reg)), []any{"one.First.field", "two.Second.field"})
	djangotest.AssertEqual(t, messageObjects(modelMessages(reg, "two")), []any{"two.Second.field"})
	assertMessages(t, modelMessages(reg, "three"))
}

// TestForeignKeyAfterMissingModel reports only the relation to the model that
// is not registered. Registry.Resolve stops at that field, so the relation
// after it has no resolved target either, although its model is registered.
func TestForeignKeyAfterMissingModel(t *testing.T) {
	type Missing struct {
		ID int64
	}
	type Present struct {
		ID int64
	}
	type Model struct {
		ID     int64
		First  orm.FK[Missing]
		Second orm.FK[Present]
	}
	reg := modelRegistry(t, invalidModelsApp, &Present{}, &Model{})
	if field(t, reg, &Model{}, "Second").Rel.Target != nil {
		t.Fatal("the relation after the missing model is resolved, so the test does not reach its case")
	}

	assertMessages(t, modelMessages(reg), checks.Error(
		"Field defines a relation with model 'Missing', which is either not installed, or is abstract.",
		checks.Obj("invalid_models_tests.Model.first"),
		checks.ID("fields.E300"),
	))
}

// TestTextFieldNeedsNoMaxLength accepts a text field, which has no max_length.
func TestTextFieldNeedsNoMaxLength(t *testing.T) {
	type Model struct {
		ID   int64
		Body string `orm:"type=text"`
	}
	reg := modelRegistry(t, invalidModelsApp, &Model{})

	assertMessages(t, modelMessages(reg))
}

// TestNameClashHidesColumnClash reports the model name and not the two fields
// with one column: as in Django, a clash of names hides the clashes of
// columns.
func TestNameClashHidesColumnClash(t *testing.T) {
	type Model_ struct { //nolint:revive // the name is the case under test
		ID  int64
		Foo int64
		Bar int64
	}
	reg := modelRegistry(t, invalidModelsApp, &Model_{})
	field(t, reg, &Model_{}, "Bar").Column = "foo"

	assertMessages(t, modelMessages(reg), checks.Error(
		"The model name 'Model_' cannot start or end with an underscore as it collides with the query lookup syntax.",
		checks.Obj(model(t, reg, &Model_{})),
		checks.ID("models.E023"),
	))
}
