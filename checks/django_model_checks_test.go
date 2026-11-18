package checks_test

import (
	"testing"

	"github.com/oliverhaas/djangogo/checks"
	"github.com/oliverhaas/djangogo/orm"
)

// The tests in this file are ports of Django's
// tests/check_framework/test_model_checks.py.
//
// Django builds two models with one db_table and lets the check report them.
// The model registry refuses the second model at registration, so the state
// these tests need does not exist today: each port skips itself when the
// registration fails and asserts Django's message when it does not.

// Model1 is the first model of DuplicateDBTableTests.
type Model1 struct {
	ID int64
}

// Meta names the table both models use.
func (*Model1) Meta() orm.Meta { return orm.Meta{Table: "test_table"} }

// Model2 is the second model of DuplicateDBTableTests.
type Model2 struct {
	ID int64
}

// Meta names the table both models use.
func (*Model2) Meta() orm.Meta { return orm.Meta{Table: "test_table"} }

// duplicateTableRegistry registers Model1 under app1 and Model2 under app2. It
// skips the test when the registry refuses the second model.
func duplicateTableRegistry(t *testing.T, app1, app2 string) *orm.Registry {
	t.Helper()
	reg := orm.NewRegistry()
	if _, err := reg.RegisterApp(app1, &Model1{}); err != nil {
		t.Fatalf("RegisterApp(%q, Model1): %v", app1, err)
	}
	if _, err := reg.RegisterApp(app2, &Model2{}); err != nil {
		t.Skipf("the model registry refuses a second model for a table, so the check has nothing to report: %v", err)
	}
	return reg
}

// TestCollisionInSameApp reports two models of one app that use one table.
//
// Ported from Django: tests/check_framework/test_model_checks.py::DuplicateDBTableTests.test_collision_in_same_app
func TestCollisionInSameApp(t *testing.T) {
	reg := duplicateTableRegistry(t, "check_framework", "check_framework")

	assertMessages(t, modelMessages(reg), checks.Error(
		"db_table 'test_table' is used by multiple models: check_framework.Model1, check_framework.Model2.",
		checks.Obj("test_table"),
		checks.ID("models.E028"),
	))
}

// TestCollisionAcrossApps reports two models of two apps that use one table.
//
// Ported from Django: tests/check_framework/test_model_checks.py::DuplicateDBTableTests.test_collision_across_apps
func TestCollisionAcrossApps(t *testing.T) {
	reg := duplicateTableRegistry(t, "basic", "check_framework")

	assertMessages(t, modelMessages(reg), checks.Error(
		"db_table 'test_table' is used by multiple models: basic.Model1, check_framework.Model2.",
		checks.Obj("test_table"),
		checks.ID("models.E028"),
	))
}
