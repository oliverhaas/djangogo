package migrations_test

import (
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/migrations"
	"github.com/oliverhaas/djangogo/orm"
)

// m2mField returns a FieldState describing an auto-through many-to-many field.
func m2mField(name, to string) migrations.FieldState {
	return migrations.FieldState{Name: name, RelKind: orm.RelM2M, RelTo: to}
}

// m2mThroughField returns a FieldState describing an explicit-through M2M field.
func m2mThroughField(name, to, through string) migrations.FieldState {
	return migrations.FieldState{Name: name, RelKind: orm.RelM2M, RelTo: to, RelThrough: through}
}

// Ported from Django: tests/migrations/test_operations.py::OperationTests.test_create_model_m2m
func TestCreateModelM2M(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		projectState := setUpTestModel(t, db, "test_crmomm", testModelOptions{})
		operation := migrations.CreateModel{
			Name: "Stable",
			Fields: []migrations.FieldState{
				idField,
				m2mField("Ponies", "test_crmomm.Pony"),
			},
		}
		newState := stateForwards(t, "test_crmomm", projectState, operation)
		djangotest.AssertTableNotExists(t, db, "test_crmomm_stable_ponies")
		databaseForwards(t, db, "test_crmomm", operation, projectState, newState)
		djangotest.AssertTableExists(t, db, "test_crmomm_stable")
		djangotest.AssertTableExists(t, db, "test_crmomm_stable_ponies")
		djangotest.AssertColumnNotExists(t, db, "test_crmomm_stable", "ponies")
		djangotest.AssertColumnExists(t, db, "test_crmomm_stable_ponies", "stable_id")
		djangotest.AssertColumnExists(t, db, "test_crmomm_stable_ponies", "pony_id")
		djangotest.AssertUniqueConstraintExists(t, db, "test_crmomm_stable_ponies", "stable_id", "pony_id")
		djangotest.AssertFKExists(t, db, "test_crmomm_stable_ponies", "stable_id", "test_crmomm_stable", "id")
		djangotest.AssertFKExists(t, db, "test_crmomm_stable_ponies", "pony_id", "test_crmomm_pony", "id")

		// Reversal drops both the through table and the model table.
		databaseBackwards(t, db, "test_crmomm", operation, newState, projectState)
		djangotest.AssertTableNotExists(t, db, "test_crmomm_stable")
		djangotest.AssertTableNotExists(t, db, "test_crmomm_stable_ponies")
	})
}

// Ported from Django: tests/migrations/test_operations.py::OperationTests.test_add_field_m2m
func TestAddFieldM2M(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		projectState := setUpTestModel(t, db, "test_adflmm", testModelOptions{secondModel: true})
		operation := migrations.AddField{
			Model: "Pony",
			Field: m2mField("Stables", "test_adflmm.Stable"),
		}
		newState := stateForwards(t, "test_adflmm", projectState, operation)
		pony := modelOf(t, newState, "test_adflmm", "Pony")
		djangotest.AssertEqual(t, len(pony.Fields), 6)

		djangotest.AssertTableNotExists(t, db, "test_adflmm_pony_stables")
		databaseForwards(t, db, "test_adflmm", operation, projectState, newState)
		djangotest.AssertTableExists(t, db, "test_adflmm_pony_stables")
		djangotest.AssertColumnNotExists(t, db, "test_adflmm_pony", "stables")
		djangotest.AssertColumnExists(t, db, "test_adflmm_pony_stables", "pony_id")
		djangotest.AssertColumnExists(t, db, "test_adflmm_pony_stables", "stable_id")

		databaseBackwards(t, db, "test_adflmm", operation, newState, projectState)
		djangotest.AssertTableNotExists(t, db, "test_adflmm_pony_stables")
	})
}

// Ported from Django: tests/migrations/test_operations.py::OperationTests.test_alter_field_m2m
func TestAlterFieldM2M(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		projectState := setUpTestModel(t, db, "test_alflmm", testModelOptions{secondModel: true})
		projectState = applyOps(t, db, "test_alflmm", projectState,
			migrations.AddField{Model: "Pony", Field: m2mField("Stables", "test_alflmm.Stable")},
		)
		f, _ := modelOf(t, projectState, "test_alflmm", "Pony").FieldByName("Stables")
		djangotest.AssertEqual(t, f.Blank, false)

		blankM2M := m2mField("Stables", "test_alflmm.Stable")
		blankM2M.Blank = true
		projectState = applyOps(t, db, "test_alflmm", projectState,
			migrations.AlterField{Model: "Pony", Field: blankM2M},
		)
		f, _ = modelOf(t, projectState, "test_alflmm", "Pony").FieldByName("Stables")
		djangotest.AssertEqual(t, f.Blank, true)
		djangotest.AssertTableExists(t, db, "test_alflmm_pony_stables")
	})
}

// Ported from Django: tests/migrations/test_operations.py::OperationTests.test_repoint_field_m2m
func TestRepointFieldM2M(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		projectState := setUpTestModel(t, db, "test_rpflmm", testModelOptions{secondModel: true, thirdModel: true})
		projectState = applyOps(t, db, "test_rpflmm", projectState,
			migrations.AddField{Model: "Pony", Field: m2mField("Places", "test_rpflmm.Stable")},
		)
		djangotest.AssertColumnExists(t, db, "test_rpflmm_pony_places", "stable_id")

		applyOps(t, db, "test_rpflmm", projectState,
			migrations.AlterField{Model: "Pony", Field: m2mField("Places", "test_rpflmm.Van")},
		)
		djangotest.AssertColumnExists(t, db, "test_rpflmm_pony_places", "van_id")
		djangotest.AssertFKExists(t, db, "test_rpflmm_pony_places", "van_id", "test_rpflmm_van", "id")
	})
}

// Ported from Django: tests/migrations/test_operations.py::OperationTests.test_remove_field_m2m
func TestRemoveFieldM2M(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		projectState := setUpTestModel(t, db, "test_rmflmm", testModelOptions{secondModel: true})
		projectState = applyOps(t, db, "test_rmflmm", projectState,
			migrations.AddField{Model: "Pony", Field: m2mField("Stables", "test_rmflmm.Stable")},
		)
		djangotest.AssertTableExists(t, db, "test_rmflmm_pony_stables")

		withFieldState := projectState.Clone()
		op := migrations.RemoveField{Model: "Pony", Field: "Stables"}
		applyOps(t, db, "test_rmflmm", projectState, op)
		djangotest.AssertTableNotExists(t, db, "test_rmflmm_pony_stables")

		unapplyOps(t, db, "test_rmflmm", withFieldState, op)
		djangotest.AssertTableExists(t, db, "test_rmflmm_pony_stables")
	})
}

// Ported from Django: tests/migrations/test_operations.py::OperationTests.test_remove_field_m2m_with_through
func TestRemoveFieldM2MWithThrough(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		projectState := setUpTestModel(t, db, "test_rmflmmwt", testModelOptions{secondModel: true})
		djangotest.AssertTableNotExists(t, db, "test_rmflmmwt_ponystables")
		projectState = applyOps(t, db, "test_rmflmmwt", projectState,
			migrations.CreateModel{
				Name: "PonyStables",
				Fields: []migrations.FieldState{
					idField,
					fkField("Pony", "pony_id", "test_rmflmmwt.Pony"),
					fkField("Stable", "stable_id", "test_rmflmmwt.Stable"),
				},
			},
			migrations.AddField{
				Model: "Pony",
				Field: m2mThroughField("Stables", "test_rmflmmwt.Stable", "test_rmflmmwt.PonyStables"),
			},
		)
		djangotest.AssertTableExists(t, db, "test_rmflmmwt_ponystables")

		applyOps(t, db, "test_rmflmmwt", projectState,
			migrations.RemoveField{Model: "Pony", Field: "Stables"},
			migrations.DeleteModel{Name: "PonyStables"},
		)
		djangotest.AssertTableNotExists(t, db, "test_rmflmmwt_ponystables")
	})
}

// Ported from Django: tests/migrations/test_operations.py::OperationTests.test_alter_model_table_m2m
func TestAlterModelTableM2M(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		app := "test_talflmltlm2m"
		projectState := setUpTestModel(t, db, app, testModelOptions{secondModel: true, table: "pony_foo"})
		firstState := applyOps(t, db, app, projectState,
			migrations.AddField{Model: "Pony", Field: m2mField("Stables", app+".Stable")},
		)
		djangotest.AssertTableExists(t, db, "pony_foo_stables")
		djangotest.AssertTableNotExists(t, db, app+"_pony_stables")

		op := migrations.AlterModelTable{Name: "Pony", Table: ""}
		secondState := stateForwards(t, app, firstState, op)
		databaseForwards(t, db, app, op, firstState, secondState)
		djangotest.AssertTableExists(t, db, app+"_pony_stables")
		djangotest.AssertTableNotExists(t, db, "pony_foo_stables")

		databaseBackwards(t, db, app, op, secondState, firstState)
		djangotest.AssertTableExists(t, db, "pony_foo_stables")
		djangotest.AssertTableNotExists(t, db, app+"_pony_stables")
	})
}

// Ported from Django: tests/migrations/test_operations.py::OperationTests.test_alter_model_table_m2m_field
func TestAlterModelTableM2MFieldToSelf(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		app := "test_talm2mfl"
		projectState := setUpTestModel(t, db, app, testModelOptions{secondModel: true})
		projectState = applyOps(t, db, app, projectState,
			migrations.AddField{Model: "Pony", Field: m2mField("Stables", app+".Stable")},
		)
		m2mTable := app + "_pony_stables"
		djangotest.AssertColumnExists(t, db, m2mTable, "pony_id")
		djangotest.AssertColumnExists(t, db, m2mTable, "stable_id")

		withFieldState := projectState.Clone()
		op := migrations.AlterField{Model: "Pony", Field: m2mField("Stables", app+".Pony")}
		applyOps(t, db, app, projectState, op)
		djangotest.AssertColumnExists(t, db, m2mTable, "from_pony_id")
		djangotest.AssertColumnExists(t, db, m2mTable, "to_pony_id")

		unapplyOps(t, db, app, withFieldState, op)
		djangotest.AssertColumnExists(t, db, m2mTable, "pony_id")
		djangotest.AssertColumnExists(t, db, m2mTable, "stable_id")
	})
}

// Ported from Django: tests/migrations/test_operations.py::OperationTests.test_rename_model_with_m2m
func TestRenameModelWithM2M(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		app := "test_rnmo_m2m"
		projectState := applyOps(t, db, app, migrations.NewProjectState(),
			migrations.CreateModel{Name: "Rider", Fields: []migrations.FieldState{idField}},
			migrations.CreateModel{Name: "Pony", Fields: []migrations.FieldState{
				idField, m2mField("Riders", app+".Rider"),
			}},
		)
		djangotest.AssertTableExists(t, db, app+"_pony_riders")

		applyOps(t, db, app, projectState,
			migrations.RenameModel{OldName: "Pony", NewName: "Pony2"},
		)
		djangotest.AssertTableExists(t, db, app+"_pony2_riders")
		djangotest.AssertTableNotExists(t, db, app+"_pony_riders")
		djangotest.AssertColumnExists(t, db, app+"_pony2_riders", "pony2_id")
		djangotest.AssertColumnExists(t, db, app+"_pony2_riders", "rider_id")
	})
}

// Ported from Django: tests/migrations/test_operations.py::OperationTests.test_rename_model_with_self_referential_m2m
func TestRenameModelWithSelfReferentialM2M(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		app := "test_rnmo_self_m2m"
		projectState := applyOps(t, db, app, migrations.NewProjectState(),
			migrations.CreateModel{
				Name: "ReflexivePony",
				Fields: []migrations.FieldState{
					idField,
					m2mField("Ponies", app+".ReflexivePony"),
				},
			},
		)
		djangotest.AssertTableExists(t, db, app+"_reflexivepony_ponies")
		djangotest.AssertColumnExists(t, db, app+"_reflexivepony_ponies", "from_reflexivepony_id")
		djangotest.AssertColumnExists(t, db, app+"_reflexivepony_ponies", "to_reflexivepony_id")

		applyOps(t, db, app, projectState,
			migrations.RenameModel{OldName: "ReflexivePony", NewName: "ReflexivePony2"},
		)
		djangotest.AssertTableExists(t, db, app+"_reflexivepony2_ponies")
		djangotest.AssertTableNotExists(t, db, app+"_reflexivepony_ponies")
		djangotest.AssertColumnExists(t, db, app+"_reflexivepony2_ponies", "from_reflexivepony2_id")
		djangotest.AssertColumnExists(t, db, app+"_reflexivepony2_ponies", "to_reflexivepony2_id")
	})
}

// Ported from Django: tests/migrations/test_operations.py::OperationTests.test_rename_m2m_target_model
func TestRenameM2MTargetModel(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		app := "test_rnmo_target_m2m"
		projectState := applyOps(t, db, app, migrations.NewProjectState(),
			migrations.CreateModel{Name: "Rider", Fields: []migrations.FieldState{idField}},
			migrations.CreateModel{Name: "Pony", Fields: []migrations.FieldState{
				idField, m2mField("Riders", app+".Rider"),
			}},
		)
		djangotest.AssertColumnExists(t, db, app+"_pony_riders", "rider_id")

		applyOps(t, db, app, projectState,
			migrations.RenameModel{OldName: "Rider", NewName: "Rider2"},
		)
		djangotest.AssertColumnExists(t, db, app+"_pony_riders", "rider2_id")
		djangotest.AssertFKExists(t, db, app+"_pony_riders", "rider2_id", app+"_rider2", "id")
	})
}
