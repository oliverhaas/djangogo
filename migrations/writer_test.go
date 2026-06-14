package migrations

import (
	"go/format"
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

// fkMigration creates an Article model whose Author field is a foreign key, so
// the writer must round-trip the relation metadata.
func fkMigration() Migration {
	return Migration{
		App:  "myapp",
		Name: "0002_article",
		Operations: []Operation{
			CreateModel{
				Name:  "Article",
				Table: "article",
				Fields: []FieldState{
					{Name: "ID", Column: "id", Kind: orm.KindAuto, PrimaryKey: true},
					{Name: "Title", Column: "title", Kind: orm.KindChar, MaxLength: 200},
					{
						Name: "Author", Column: "author_id", Kind: orm.KindInt,
						RelKind: orm.RelFK, RelTargetTable: "author", RelTargetColumn: "id",
					},
				},
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

// TestRenderMigration_GofmtSClean verifies that the rendered output is already
// in gofmt -s canonical form (format.Source output equals the rendered source)
// and that FieldState slice elements are emitted without a redundant type name.
func TestRenderMigration_GofmtSClean(t *testing.T) {
	t.Parallel()
	src, err := RenderMigration("mypkg", testMigration())
	if err != nil {
		t.Fatalf("RenderMigration error: %v", err)
	}

	// format.Source applies gofmt (including -s simplification) -- the result
	// must be identical to what we rendered.
	formatted, err := format.Source([]byte(src))
	if err != nil {
		t.Fatalf("format.Source error: %v\n\nsource:\n%s", err, src)
	}
	if string(formatted) != src {
		t.Errorf("rendered source is not gofmt -s clean; diff (want formatted):\n%s\n\ngot:\n%s", string(formatted), src)
	}

	// Slice elements inside []migrations.FieldState{...} must NOT carry a
	// redundant type name. gofmt -s flags "T{...}" elements inside a []T literal
	// as simplifiable. We detect this by asserting that no line starts with
	// tab-indented "migrations.FieldState{" (which is the element-literal form);
	// the only allowed forms are the slice type "[]migrations.FieldState{" and
	// the struct-field form "Field: migrations.FieldState{".
	if strings.Contains(src, "\tmigrations.FieldState{") {
		t.Errorf(
			"rendered source contains a redundant FieldState element type "+
				"(tab-indented migrations.FieldState{ inside a slice literal); "+
				"gofmt -s requires bare {..} for slice elements\n\nsource:\n%s",
			src,
		)
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

// TestRenderMigration_RendersFK verifies that an FK FieldState round-trips its
// relation metadata: the rendered source names orm.RelFK and the target table
// and column, is gofmt -s clean, and parses.
func TestRenderMigration_RendersFK(t *testing.T) {
	t.Parallel()
	src, err := RenderMigration("mypkg", fkMigration())
	if err != nil {
		t.Fatalf("RenderMigration error: %v", err)
	}

	checks := []string{
		"RelKind: orm.RelFK",
		`RelTargetTable: "author"`,
		`RelTargetColumn: "id"`,
	}
	for _, want := range checks {
		if !strings.Contains(src, want) {
			t.Errorf("rendered FK source missing %q\n\nsource:\n%s", want, src)
		}
	}

	// gofmt -s clean: format.Source output must equal the rendered source.
	formatted, err := format.Source([]byte(src))
	if err != nil {
		t.Fatalf("format.Source error: %v\n\nsource:\n%s", err, src)
	}
	if string(formatted) != src {
		t.Errorf("rendered FK source is not gofmt -s clean; diff (want formatted):\n%s\n\ngot:\n%s", string(formatted), src)
	}

	fset := token.NewFileSet()
	if _, parseErr := parser.ParseFile(fset, "", src, parser.AllErrors); parseErr != nil {
		t.Fatalf("rendered FK source does not parse: %v\n\n%s", parseErr, src)
	}
}

// TestRenderMigration_RendersOnDelete confirms a FK's on_delete action survives
// the round-trip into generated migration source as a named orm constant.
func TestRenderMigration_RendersOnDelete(t *testing.T) {
	t.Parallel()
	m := fkMigration()
	cm := m.Operations[0].(CreateModel)
	cm.Fields[2].RelOnDelete = orm.OnDeleteCascade
	m.Operations[0] = cm

	src, err := RenderMigration("mypkg", m)
	if err != nil {
		t.Fatalf("RenderMigration error: %v", err)
	}
	if !strings.Contains(src, "RelOnDelete: orm.OnDeleteCascade") {
		t.Errorf("rendered source missing RelOnDelete constant\n\nsource:\n%s", src)
	}
	fset := token.NewFileSet()
	if _, perr := parser.ParseFile(fset, "", src, parser.AllErrors); perr != nil {
		t.Fatalf("rendered source does not parse: %v\n\n%s", perr, src)
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
