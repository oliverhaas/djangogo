package djangogo_test

import (
	"cmp"
	"fmt"
	"go/format"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/migrations"
	"github.com/oliverhaas/djangogo/orm"
)

// generatedGoMod is the go.mod of the project the build test generates
// migrations into. The replace directive points at this repository.
const generatedGoMod = `module example.com/generated

go 1.26.0

require github.com/oliverhaas/djangogo v0.0.0-00010101000000-000000000000

replace github.com/oliverhaas/djangogo => %s
`

// generatedMain imports the generated migration packages, which register
// their migrations from init, and prints every migration it finds.
const generatedMain = `package main

import (
	"cmp"
	"fmt"
	"slices"

	"github.com/oliverhaas/djangogo/migrations"

%s)

func main() {
	all := migrations.DefaultRegistry.All()
	slices.SortFunc(all, func(a, b migrations.Migration) int {
		return cmp.Or(cmp.Compare(a.App, b.App), cmp.Compare(a.Name, b.Name))
	})
	for _, m := range all {
		fmt.Printf("%%#v\n", m)
	}
}
`

// handWritten is a migration with every operation the writer can render that
// makemigrations does not produce for the models of the build test.
func handWritten() migrations.Migration {
	return migrations.Migration{
		App: "people", Name: "0002_by_hand",
		Dependencies: []migrations.Key{key("people", "0001_initial"), key("books", "0001_initial")},
		RunBefore:    []migrations.Key{key("books", "0002_later")},
		Operations: []migrations.Operation{
			migrations.CreateModel{
				Name: "Agent", Table: "people_agents",
				Fields: []migrations.FieldState{
					idField,
					{Name: "Name", Column: "name", Kind: orm.KindChar, MaxLength: 80, Unique: true, Blank: true, Default: "n/a"},
					{Name: "Active", Column: "active", Kind: orm.KindBool, Default: true},
					{Name: "Rank", Column: "rank", Kind: orm.KindInt, Null: true, DBIndex: true, Default: int64(3)},
					{Name: "Notes", Column: "notes", Kind: orm.KindText, Default: "a \"quoted\" note\nwith a line break"},
					{Name: "Created", Column: "created", Kind: orm.KindDateTime, AutoNowAdd: true},
					{Name: "Updated", Column: "updated", Kind: orm.KindDateTime, AutoNow: true},
					{
						Name: "Writer", Column: "writer_id", Kind: orm.KindInt, Null: true,
						RelKind: orm.RelFK, RelTo: "people.Writer", RelOnDelete: orm.OnDeleteRestrict,
					},
				},
				Indexes:        []migrations.Index{{Name: "agent_rank_idx", Fields: []string{"Rank", "Name"}}},
				UniqueTogether: [][]string{{"Name", "Rank"}},
				// Conditions and expressions with nested pointers would
				// print their addresses, so the constraints use flat ones.
				Constraints: []orm.Constraint{
					orm.UniqueConstraint{
						Name: "agent_name_rank_uniq", Fields: []string{"Name", "Rank"},
						Condition: orm.Q("active", true), ViolationErrorCode: "duplicate",
					},
					orm.UniqueConstraint{Name: "agent_name_uniq", Expressions: []orm.Expr{orm.F("name")}, NullsNotDistinct: true},
				},
			},
			migrations.RenameModel{OldName: "Agent", NewName: "Broker"},
			migrations.AlterModelTable{Name: "Broker", Table: "people_broker"},
			migrations.AlterUniqueTogether{Name: "Broker", UniqueTogether: [][]string{{"Name", "Active"}, {"Rank", "Name"}}},
			migrations.AlterUniqueTogether{Name: "Broker"},
			migrations.AddIndex{Model: "Broker", Index: migrations.Index{Name: "broker_name_idx", Fields: []string{"Name"}}},
			migrations.RemoveIndex{Model: "Broker", Name: "broker_name_idx"},
			migrations.AddConstraint{Model: "Broker", Constraint: orm.CheckConstraint{
				Name: "broker_rank_gte_0", Condition: orm.Q("rank__gte", int64(0)).Or(orm.Q("rank__isnull", true)),
			}},
			migrations.AlterConstraint{Model: "Broker", Name: "broker_rank_gte_0", Constraint: orm.CheckConstraint{
				Name: "broker_rank_gte_0", Condition: orm.Q("rank__gte", int64(0)).Or(orm.Q("rank__isnull", true)),
				ViolationErrorMessage: "The rank must not be negative.",
			}},
			migrations.RemoveConstraint{Model: "Broker", Name: "broker_rank_gte_0"},
			migrations.AddField{
				Model:   "Broker",
				Field:   migrations.FieldState{Name: "Fee", Column: "fee", Kind: orm.KindInt},
				Default: int64(10),
			},
			migrations.AddField{
				Model:   "Broker",
				Field:   migrations.FieldState{Name: "Since", Column: "since", Kind: orm.KindDateTime},
				Default: migrations.Now,
			},
			migrations.AlterField{
				Model:   "Broker",
				Field:   migrations.FieldState{Name: "Rank", Column: "rank", Kind: orm.KindInt},
				Default: int64(0),
			},
			migrations.RenameField{Model: "Broker", OldName: "Fee", NewName: "Commission"},
			migrations.RenameField{Model: "Broker", OldName: "Notes", NewName: "Remarks", Column: "notes"},
			migrations.RemoveField{Model: "Broker", Field: "Since"},
			migrations.RunSQL{
				SQL:        []string{"UPDATE people_broker SET rank = 1\nWHERE rank IS NULL", "SELECT 1"},
				ReverseSQL: []string{"SELECT `one`"},
			},
			// Nothing to undo, which is not the same as irreversible.
			migrations.RunSQL{SQL: []string{"SELECT 2"}, ReverseSQL: []string{}},
			migrations.RunSQL{
				SQL: []string{"DROP TABLE people_broker"},
				StateOperations: []migrations.Operation{
					migrations.DeleteModel{Name: "Broker"},
				},
			},
		},
	}
}

// runTool runs a command of the Go toolchain in dir and returns its standard
// output. A failing command fails the test with everything it printed.
func runTool(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("go", args...)
	cmd.Dir = dir
	var stdout, stderr strings.Builder
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("go %s: %v\n%s%s", strings.Join(args, " "), err, stdout.String(), stderr.String())
	}
	return stdout.String()
}

// TestGeneratedMigrationsBuild writes migrations into a new Go module: the
// ones makemigrations generates for two apps and a hand-built one with the
// remaining operations. The files have to be formatted as gofmt formats them,
// pass go vet, compile, and register exactly the migrations they were written
// from. It is skipped under -short because it runs the Go toolchain.
func TestGeneratedMigrationsBuild(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping the build of generated migrations under -short")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skipf("go toolchain not on PATH: %v", err)
	}
	root, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}

	type Draft struct {
		ID    int64
		Title string        `orm:"max_length=200;index;default=untitled"`
		Body  string        `orm:"type=text;blank"`
		Novel orm.FK[Novel] `orm:"null;on_delete=set_null"`
	}
	c := newCommand(t, nil, nil,
		djangotest.App("books", &Novel{}, &Draft{}), djangotest.App("people", &Writer{}))
	dir := t.TempDir()
	c.app.MigrationsDir = filepath.Join(dir, "migrations")
	c.call("makemigrations")
	c.call("makemigrations", "books", "--empty", "--name", "later")

	extra := handWritten()
	if _, err := migrations.WriteMigration(filepath.Join(c.app.MigrationsDir, "people"), "people", extra); err != nil {
		t.Fatalf("write migration: %v", err)
	}
	want := append(c.app.Migrations.All(), extra)
	slices.SortFunc(want, func(a, b migrations.Migration) int {
		return cmp.Or(cmp.Compare(a.App, b.App), cmp.Compare(a.Name, b.Name))
	})
	djangotest.AssertEqual(t, len(want), 4)

	// Every file is formatted and says where it comes from.
	for _, m := range want {
		path := filepath.Join(c.app.MigrationsDir, m.App, migrations.FileName(m.Name))
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read migration: %v", err)
		}
		formatted, err := format.Source(src)
		if err != nil {
			t.Fatalf("%s is not valid Go: %v", path, err)
		}
		if string(formatted) != string(src) {
			t.Errorf("%s is not formatted as gofmt formats it:\n%s", path, src)
		}
		if !strings.HasPrefix(string(src), "// Generated by djangogo makemigrations.\n\npackage "+m.App+"\n") {
			t.Errorf("%s does not start with the header of a generated migration:\n%s", path, src)
		}
	}

	imports := "\t_ \"example.com/generated/migrations/books\"\n\t_ \"example.com/generated/migrations/people\"\n"
	files := map[string]string{
		"go.mod":  fmt.Sprintf(generatedGoMod, root),
		"main.go": fmt.Sprintf(generatedMain, imports),
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	runTool(t, dir, "mod", "tidy")
	runTool(t, dir, "vet", "./...")
	got := strings.Split(strings.TrimSuffix(runTool(t, dir, "run", "."), "\n"), "\n")
	djangotest.AssertEqual(t, len(got), len(want))
	for i := range min(len(got), len(want)) {
		// The file of an empty migration has an empty list of operations
		// for the developer to fill in.
		if want[i].Operations == nil {
			want[i].Operations = []migrations.Operation{}
		}
		djangotest.AssertEqual(t, got[i], fmt.Sprintf("%#v", want[i]))
	}
}
