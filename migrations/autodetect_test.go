package migrations

import (
	"testing"

	"github.com/oliverhaas/djangogo/orm"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// personModelState returns a ModelState for Person with ID, Name, Age fields.
func personModelState() *ModelState {
	return &ModelState{
		Name:  "Person",
		Table: "person",
		Fields: []FieldState{
			{Name: "ID", Column: "id", Kind: orm.KindAuto, PrimaryKey: true},
			{Name: "Name", Column: "name", Kind: orm.KindChar, MaxLength: 100},
			{Name: "Age", Column: "age", Kind: orm.KindInt},
		},
	}
}

// emptyState returns an empty ProjectState.
func emptyState() *ProjectState {
	return NewProjectState()
}

// stateWith builds a ProjectState from the given ModelStates (order matches argument order).
func stateWith(models ...*ModelState) *ProjectState {
	ps := NewProjectState()
	for _, ms := range models {
		ps.Models[ms.Name] = ms
		ps.Order = append(ps.Order, ms.Name)
	}
	return ps
}

// assertOps checks that the returned ops have exactly the expected describe strings.
func assertOps(t *testing.T, got []Operation, want []string) {
	t.Helper()
	if len(got) != len(want) {
		descs := make([]string, len(got))
		for i, op := range got {
			descs[i] = op.Describe()
		}
		t.Fatalf("op count: got %d %v, want %d %v", len(got), descs, len(want), want)
	}
	for i, op := range got {
		if op.Describe() != want[i] {
			t.Errorf("ops[%d]: got %q, want %q", i, op.Describe(), want[i])
		}
	}
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestDiff_NewModelOnly(t *testing.T) {
	t.Parallel()
	old := emptyState()
	nw := stateWith(personModelState())

	ops := Diff(old, nw)
	assertOps(t, ops, []string{"CreateModel Person"})

	cm, ok := ops[0].(CreateModel)
	if !ok {
		t.Fatalf("ops[0] is %T, want CreateModel", ops[0])
	}
	if cm.Name != "Person" {
		t.Errorf("CreateModel.Name = %q, want %q", cm.Name, "Person")
	}
	if cm.Table != "person" {
		t.Errorf("CreateModel.Table = %q, want %q", cm.Table, "person")
	}
	if len(cm.Fields) != 3 {
		t.Errorf("CreateModel.Fields len = %d, want 3", len(cm.Fields))
	}
}

func TestDiff_NoChange(t *testing.T) {
	t.Parallel()
	state := stateWith(personModelState())
	ops := Diff(state, state)
	if len(ops) != 0 {
		t.Errorf("expected empty ops, got %d ops", len(ops))
	}
}

func TestDiff_AddedField(t *testing.T) {
	t.Parallel()
	old := stateWith(personModelState())
	nw := stateWith(&ModelState{
		Name:  "Person",
		Table: "person",
		Fields: []FieldState{
			{Name: "ID", Column: "id", Kind: orm.KindAuto, PrimaryKey: true},
			{Name: "Name", Column: "name", Kind: orm.KindChar, MaxLength: 100},
			{Name: "Age", Column: "age", Kind: orm.KindInt},
			{Name: "Bio", Column: "bio", Kind: orm.KindText, Null: true},
		},
	})

	ops := Diff(old, nw)
	assertOps(t, ops, []string{"AddField Person.Bio"})

	af, ok := ops[0].(AddField)
	if !ok {
		t.Fatalf("ops[0] is %T, want AddField", ops[0])
	}
	if af.Model != "Person" {
		t.Errorf("AddField.Model = %q, want %q", af.Model, "Person")
	}
	if af.Field.Name != "Bio" {
		t.Errorf("AddField.Field.Name = %q, want %q", af.Field.Name, "Bio")
	}
}

func TestDiff_RemovedField(t *testing.T) {
	t.Parallel()
	old := stateWith(personModelState())
	nw := stateWith(&ModelState{
		Name:  "Person",
		Table: "person",
		Fields: []FieldState{
			{Name: "ID", Column: "id", Kind: orm.KindAuto, PrimaryKey: true},
			{Name: "Name", Column: "name", Kind: orm.KindChar, MaxLength: 100},
		},
	})

	ops := Diff(old, nw)
	assertOps(t, ops, []string{"RemoveField Person.Age"})

	rf, ok := ops[0].(RemoveField)
	if !ok {
		t.Fatalf("ops[0] is %T, want RemoveField", ops[0])
	}
	if rf.Model != "Person" {
		t.Errorf("RemoveField.Model = %q, want %q", rf.Model, "Person")
	}
	if rf.Field != "Age" {
		t.Errorf("RemoveField.Field = %q, want %q", rf.Field, "Age")
	}
}

func TestDiff_AlteredField(t *testing.T) {
	t.Parallel()
	old := stateWith(personModelState())
	nw := stateWith(&ModelState{
		Name:  "Person",
		Table: "person",
		Fields: []FieldState{
			{Name: "ID", Column: "id", Kind: orm.KindAuto, PrimaryKey: true},
			{Name: "Name", Column: "name", Kind: orm.KindChar, MaxLength: 200},
			{Name: "Age", Column: "age", Kind: orm.KindInt},
		},
	})

	ops := Diff(old, nw)
	assertOps(t, ops, []string{"AlterField Person.Name"})

	af, ok := ops[0].(AlterField)
	if !ok {
		t.Fatalf("ops[0] is %T, want AlterField", ops[0])
	}
	if af.Model != "Person" {
		t.Errorf("AlterField.Model = %q, want %q", af.Model, "Person")
	}
	if af.Field.Name != "Name" {
		t.Errorf("AlterField.Field.Name = %q, want %q", af.Field.Name, "Name")
	}
	if af.Field.MaxLength != 200 {
		t.Errorf("AlterField.Field.MaxLength = %d, want 200", af.Field.MaxLength)
	}
}

func TestDiff_DeletedModel(t *testing.T) {
	t.Parallel()
	old := stateWith(personModelState())
	nw := emptyState()

	ops := Diff(old, nw)
	assertOps(t, ops, []string{"DeleteModel Person"})

	dm, ok := ops[0].(DeleteModel)
	if !ok {
		t.Fatalf("ops[0] is %T, want DeleteModel", ops[0])
	}
	if dm.Name != "Person" {
		t.Errorf("DeleteModel.Name = %q, want %q", dm.Name, "Person")
	}
}

// TestDiff_CombinedOrdering verifies that: new model Tag appears first
// (CreateModel), then an altered field on Person (AlterField), then the
// deleted model Old (DeleteModel).
func TestDiff_CombinedOrdering(t *testing.T) {
	t.Parallel()

	oldPersonModel := personModelState()
	oldModel := &ModelState{
		Name:  "Old",
		Table: "old",
		Fields: []FieldState{
			{Name: "ID", Column: "id", Kind: orm.KindAuto, PrimaryKey: true},
		},
	}
	old := stateWith(oldPersonModel, oldModel)

	// new state: Tag added, Person.Name MaxLength changed, Old removed.
	tagModel := &ModelState{
		Name:  "Tag",
		Table: "tag",
		Fields: []FieldState{
			{Name: "ID", Column: "id", Kind: orm.KindAuto, PrimaryKey: true},
			{Name: "Label", Column: "label", Kind: orm.KindChar, MaxLength: 50},
		},
	}
	newPersonModel := &ModelState{
		Name:  "Person",
		Table: "person",
		Fields: []FieldState{
			{Name: "ID", Column: "id", Kind: orm.KindAuto, PrimaryKey: true},
			{Name: "Name", Column: "name", Kind: orm.KindChar, MaxLength: 200},
			{Name: "Age", Column: "age", Kind: orm.KindInt},
		},
	}
	// new.Order: Tag first (so CreateModel Tag comes before AlterField Person.Name).
	nw := stateWith(tagModel, newPersonModel)

	ops := Diff(old, nw)
	assertOps(t, ops, []string{
		"CreateModel Tag",
		"AlterField Person.Name",
		"DeleteModel Old",
	})
}

// TestDiff_FieldOpSubOrdering checks that for a single model that simultaneously
// gains a field, has a field altered, and loses a field, the sub-ordering is:
// AddField first, then AlterField, then RemoveField.
func TestDiff_FieldOpSubOrdering(t *testing.T) {
	t.Parallel()

	// Old: ID, Name (MaxLength 100), Age, Extra.
	old := stateWith(&ModelState{
		Name:  "Thing",
		Table: "thing",
		Fields: []FieldState{
			{Name: "ID", Column: "id", Kind: orm.KindAuto, PrimaryKey: true},
			{Name: "Name", Column: "name", Kind: orm.KindChar, MaxLength: 100},
			{Name: "Age", Column: "age", Kind: orm.KindInt},
			{Name: "Extra", Column: "extra", Kind: orm.KindInt},
		},
	})

	// New: ID, Name (MaxLength 200 -- altered), Age, NewField (added). Extra removed.
	nw := stateWith(&ModelState{
		Name:  "Thing",
		Table: "thing",
		Fields: []FieldState{
			{Name: "ID", Column: "id", Kind: orm.KindAuto, PrimaryKey: true},
			{Name: "Name", Column: "name", Kind: orm.KindChar, MaxLength: 200},
			{Name: "Age", Column: "age", Kind: orm.KindInt},
			{Name: "NewField", Column: "new_field", Kind: orm.KindText, Null: true},
		},
	})

	ops := Diff(old, nw)
	// Expect: AddField NewField, AlterField Name, RemoveField Extra.
	assertOps(t, ops, []string{
		"AddField Thing.NewField",
		"AlterField Thing.Name",
		"RemoveField Thing.Extra",
	})
}
