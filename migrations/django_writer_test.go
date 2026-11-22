package migrations_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/migrations"
	"github.com/oliverhaas/djangogo/orm"
)

// thisImportPath is the import path the runtime reports for the functions of
// this test package.
const thisImportPath = "github.com/oliverhaas/djangogo/migrations_test"

// ExportedForwards is a top-level function another package can refer to.
func ExportedForwards(context.Context, *orm.DB, *migrations.ProjectState) error { return nil }

// unexportedForwards is a top-level function only its own package can refer
// to.
func unexportedForwards(context.Context, *orm.DB, *migrations.ProjectState) error { return nil }

// assertContains fails the test unless src contains needle.
func assertContains(t *testing.T, src, needle string) {
	t.Helper()
	if !strings.Contains(src, needle) {
		t.Errorf("%q not found in:\n%s", needle, src)
	}
}

// assertNotContains fails the test when src contains needle.
func assertNotContains(t *testing.T, src, needle string) {
	t.Helper()
	if strings.Contains(src, needle) {
		t.Errorf("%q unexpectedly found in:\n%s", needle, src)
	}
}

// renderRunGo renders a migration with op as its only operation.
func renderRunGo(op migrations.RunGo, opts migrations.RenderOptions) (string, []string, error) {
	m := migrations.Migration{App: "migrations", Name: "0003_data", Operations: []migrations.Operation{op}}
	return migrations.RenderMigrationWith("migrations", m, opts)
}

// funcHolder has a method whose value is a GoFunc.
type funcHolder struct{}

// Forwards does nothing.
func (funcHolder) Forwards(context.Context, *orm.DB, *migrations.ProjectState) error { return nil }

// Ported from Django: tests/migrations/test_writer.py::WriterTests.test_serialize_functions
func TestSerializeFunctions(t *testing.T) {
	closure := func(context.Context, *orm.DB, *migrations.ProjectState) error { return nil }
	for name, fn := range map[string]migrations.GoFunc{
		"closure":      closure,
		"method value": funcHolder{}.Forwards,
	} {
		_, _, err := renderRunGo(migrations.RunGo{Forward: fn}, migrations.RenderOptions{})
		if !errors.Is(err, migrations.ErrNotSerializable) || !strings.Contains(err.Error(), "cannot serialize function") {
			t.Errorf("%s: err = %v, want a serialization error", name, err)
		}
	}
	// A package function of the migrations package.
	src, _, err := renderRunGo(migrations.RunGo{Forward: migrations.Noop, Reverse: migrations.Noop}, migrations.RenderOptions{})
	djangotest.AssertEqual(t, err, nil)
	assertContains(t, src, "migrations.RunGo{\n\t\t\t\tForward: migrations.Noop,\n\t\t\t\tReverse: migrations.Noop,\n\t\t\t}")
}

// No Django test exists: Python imports the module of any function, Go can
// only refer to the exported functions of another package.
func TestSerializeFunctionsOfOtherPackages(t *testing.T) {
	src, _, err := renderRunGo(migrations.RunGo{Forward: ExportedForwards}, migrations.RenderOptions{})
	djangotest.AssertEqual(t, err, nil)
	assertContains(t, src, "\tmigrations_test \""+thisImportPath+"\"\n")
	assertContains(t, src, "Forward: migrations_test.ExportedForwards,")

	_, _, err = renderRunGo(migrations.RunGo{Forward: unexportedForwards}, migrations.RenderOptions{})
	if !errors.Is(err, migrations.ErrNotSerializable) {
		t.Errorf("err = %v, want ErrNotSerializable", err)
	}
	// The package of the file refers to its own functions by name.
	src, _, err = renderRunGo(migrations.RunGo{Forward: unexportedForwards, Reverse: ExportedForwards},
		migrations.RenderOptions{ImportPath: thisImportPath})
	djangotest.AssertEqual(t, err, nil)
	assertContains(t, src, "Forward: unexportedForwards,")
	assertContains(t, src, "Reverse: ExportedForwards,")
	assertNotContains(t, src, thisImportPath)
}

// No Django test exists: Django's squashmigrations test checks the porting
// comment through the command output only.
func TestManualPortingComment(t *testing.T) {
	src, porting, err := renderRunGo(migrations.RunGo{Forward: portingForwards, Reverse: migrations.Noop},
		migrations.RenderOptions{ImportPath: thisImportPath})
	djangotest.AssertEqual(t, err, nil)
	djangotest.AssertSequenceEqual(t, porting, []string{thisImportPath + "/0002_manual_porting_test.go"})
	assertContains(t, src, ")\n\n"+
		"// Functions from the following migrations need manual copying.\n"+
		"// Move them and any dependencies into this file, then update the\n"+
		"// RunGo operations to refer to the local versions:\n"+
		"// "+thisImportPath+"/0002_manual_porting_test.go\n\n"+
		"func init() {")

	_, porting, err = renderRunGo(migrations.RunGo{Forward: ExportedForwards}, migrations.RenderOptions{})
	djangotest.AssertEqual(t, err, nil)
	djangotest.AssertEqual(t, len(porting), 0)
}

// Ported from Django: tests/migrations/test_writer.py::WriterTests.test_migration_file_header_comments
func TestMigrationFileHeaderComments(t *testing.T) {
	m := migrations.Migration{App: "migrations", Name: "0001_initial"}
	for _, header := range []string{"Generated by djangogo squashmigrations.", ""} {
		src, _, err := migrations.RenderMigrationWith("migrations", m, migrations.RenderOptions{Header: header})
		djangotest.AssertEqual(t, err, nil)
		djangotest.AssertEqual(t, strings.HasPrefix(src, "// "+header+"\n\npackage migrations\n"), header != "")
		if header == "" {
			djangotest.AssertEqual(t, strings.HasPrefix(src, "package migrations\n"), true)
		}
	}
	src, err := migrations.RenderMigration("migrations", m)
	djangotest.AssertEqual(t, err, nil)
	djangotest.AssertEqual(t, strings.HasPrefix(src, "// Generated by djangogo makemigrations.\n\npackage migrations\n"), true)
}

// No Django test exists: Django's writer tests check replaces and elidable
// through the squashmigrations command.
func TestWriteReplacesAndElidable(t *testing.T) {
	m := migrations.Migration{
		App: "migrations", Name: "0001_squashed_0002", Initial: true,
		Replaces: []migrations.Key{k("migrations", "0001_initial"), k("migrations", "0002_second")},
		Operations: []migrations.Operation{
			migrations.RunSQL{SQL: []string{"SELECT 1"}, Elidable: true},
			migrations.RunGo{Forward: migrations.Noop, Elidable: true},
		},
	}
	src, err := migrations.RenderMigration("migrations", m)
	djangotest.AssertEqual(t, err, nil)
	assertContains(t, src, "\t\tName: \"0001_squashed_0002\",\n"+
		"\t\tReplaces: []migrations.Key{\n"+
		"\t\t\t{App: \"migrations\", Name: \"0001_initial\"},\n"+
		"\t\t\t{App: \"migrations\", Name: \"0002_second\"},\n"+
		"\t\t},\n"+
		"\t\tInitial: true,\n")
	assertContains(t, src, "migrations.RunSQL{\n\t\t\t\tSQL:      []string{\"SELECT 1\"},\n\t\t\t\tElidable: true,\n\t\t\t}")
	assertContains(t, src, "migrations.RunGo{\n\t\t\t\tForward:  migrations.Noop,\n\t\t\t\tElidable: true,\n\t\t\t}")
}
