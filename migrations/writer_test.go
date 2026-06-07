package migrations

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/orm"
)

func testMigration() Migration {
	return Migration{
		App:  "myapp",
		Name: "0001_initial",
		Operations: []Operation{
			CreateModel{
				Name:  "Person",
				Table: "person",
				Fields: []FieldState{
					{Name: "ID", Column: "id", Kind: orm.KindAuto, PrimaryKey: true},
					{Name: "Name", Column: "name", Kind: orm.KindChar, MaxLength: 100},
					{Name: "Age", Column: "age", Kind: orm.KindInt},
				},
			},
			AddField{
				Model: "Person",
				Field: FieldState{Name: "Bio", Column: "bio", Kind: orm.KindText, Null: true},
			},
		},
	}
}

func TestRenderMigration_ParsesCleanly(t *testing.T) {
	t.Parallel()
	src, err := RenderMigration("mypkg", testMigration())
	if err != nil {
		t.Fatalf("RenderMigration error: %v", err)
	}

	fset := token.NewFileSet()
	_, parseErr := parser.ParseFile(fset, "", src, parser.AllErrors)
	if parseErr != nil {
		t.Fatalf("rendered source does not parse: %v\n\n%s", parseErr, src)
	}
}

func TestRenderMigration_ContainsExpectedTokens(t *testing.T) {
	t.Parallel()
	src, err := RenderMigration("mypkg", testMigration())
	if err != nil {
		t.Fatalf("RenderMigration error: %v", err)
	}

	checks := []string{
		"package mypkg",
		`"github.com/oliverhaas/djangogo/migrations"`,
		`"github.com/oliverhaas/djangogo/orm"`,
		"migrations.CreateModel",
		"orm.KindAuto",
		"orm.KindChar",
		"func init()",
		"migrations.Register",
	}
	for _, want := range checks {
		if !strings.Contains(src, want) {
			t.Errorf("rendered source missing %q\n\nsource:\n%s", want, src)
		}
	}
}

func TestWriteMigration_WritesFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	m := testMigration()
	path, err := WriteMigration(dir, "mypkg", m)
	if err != nil {
		t.Fatalf("WriteMigration error: %v", err)
	}

	wantPath := filepath.Join(dir, m.Name+".go")
	if path != wantPath {
		t.Errorf("returned path = %q, want %q", path, wantPath)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading written file: %v", err)
	}

	fset := token.NewFileSet()
	_, parseErr := parser.ParseFile(fset, "", content, parser.AllErrors)
	if parseErr != nil {
		t.Fatalf("written file does not parse: %v\n\n%s", parseErr, string(content))
	}
}

func TestRenderMigration_NoOrmImportWhenNoFields(t *testing.T) {
	t.Parallel()
	m := Migration{
		App:  "myapp",
		Name: "0002_delete",
		Operations: []Operation{
			DeleteModel{Name: "Person"},
		},
	}
	src, err := RenderMigration("mypkg", m)
	if err != nil {
		t.Fatalf("RenderMigration error: %v", err)
	}

	if strings.Contains(src, `"github.com/oliverhaas/djangogo/orm"`) {
		t.Errorf("orm import should be absent when no FieldState is present\n\nsource:\n%s", src)
	}

	fset := token.NewFileSet()
	_, parseErr := parser.ParseFile(fset, "", src, parser.AllErrors)
	if parseErr != nil {
		t.Fatalf("rendered source does not parse: %v\n\n%s", parseErr, src)
	}
}
