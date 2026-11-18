package djangogo_test

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo"
	"github.com/oliverhaas/djangogo/conf"
	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
)

// ShellItem is the model the shell tests query.
type ShellItem struct {
	ID     int64
	Name   string `orm:"max_length=50"`
	Qty    int64
	Active bool
}

// shellApp returns an application over db and its models with buffers for
// the streams. The application itself has no database of its own, so Close
// leaves db to its test.
func shellApp(t *testing.T, db *orm.DB) (app *djangogo.Application, out, errOut *bytes.Buffer) {
	t.Helper()
	app, err := djangogo.New(conf.Settings{SecretKey: "k"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if db != nil {
		app.DB, app.Models = db, db.Registry()
	}
	out, errOut = &bytes.Buffer{}, &bytes.Buffer{}
	app.Out, app.Err = out, errOut
	return app, out, errOut
}

// insertItems inserts rows into the table of ShellItem with parameterized
// SQL, which works on both dialects.
func insertItems(t *testing.T, db *orm.DB, rows ...ShellItem) {
	t.Helper()
	m, ok := db.Registry().ModelOf(&ShellItem{})
	if !ok {
		t.Fatal("ShellItem is not registered")
	}
	d := db.Dialect()
	q := fmt.Sprintf("INSERT INTO %s (%s, %s, %s) VALUES (%s, %s, %s)", d.Quote(m.Table()),
		d.Quote("name"), d.Quote("qty"), d.Quote("active"), d.Placeholder(1), d.Placeholder(2), d.Placeholder(3))
	for _, r := range rows {
		if _, err := db.SQL().ExecContext(context.Background(), q, r.Name, r.Qty, r.Active); err != nil {
			t.Fatalf("insert %v: %v", r, err)
		}
	}
}

// runShell runs the statements of command with "shell -c" and fails the test
// when the command fails.
func runShell(t *testing.T, app *djangogo.Application, command string) {
	t.Helper()
	if err := app.Execute([]string{"shell", "-c", command}); err != nil {
		t.Fatalf("shell -c %q: %v", command, err)
	}
}

// TestShellQueries runs the QuerySet-style statements against both
// databases: count, all and filter by text, number, boolean and None.
func TestShellQueries(t *testing.T) {
	djangotest.EachDB(t, []any{djangotest.App("shop", &ShellItem{})}, func(t *testing.T, db *orm.DB) {
		insertItems(t, db,
			ShellItem{Name: "apple pie", Qty: 3, Active: true},
			ShellItem{Name: "bread", Qty: 7, Active: false},
		)
		app, out, errOut := shellApp(t, db)

		runShell(t, app, "ShellItem.count()")
		djangotest.AssertEqual(t, out.String(), "2\n")

		out.Reset()
		runShell(t, app, "shop.ShellItem.filter(name='apple pie')")
		djangotest.AssertEqual(t, out.String(), "id  name       qty  active\n1   apple pie  3    True\n")

		out.Reset()
		runShell(t, app, "shellitem.count()")
		djangotest.AssertEqual(t, out.String(), "2\n")

		out.Reset()
		runShell(t, app, "ShellItem.filter(qty=7, active=False)")
		djangotest.AssertEqual(t, errOut.String(), "")
		checkContains(t, out.String(), "bread")
		if strings.Contains(out.String(), "apple") {
			t.Errorf("filter(qty=7, active=False) shows the other row:\n%s", out)
		}

		out.Reset()
		runShell(t, app, "ShellItem.filter(name=None)")
		djangotest.AssertEqual(t, out.String(), "id  name  qty  active\n")
	})
}

// TestShellValuesAreParameters checks that a filter value never becomes SQL:
// a value written as an injection matches no row, and the table survives.
func TestShellValuesAreParameters(t *testing.T) {
	djangotest.EachDB(t, []any{djangotest.App("shop", &ShellItem{})}, func(t *testing.T, db *orm.DB) {
		insertItems(t, db, ShellItem{Name: "bread", Qty: 1})
		app, out, errOut := shellApp(t, db)

		runShell(t, app, `ShellItem.filter(name="x' OR '1'='1")`)
		djangotest.AssertEqual(t, out.String(), "id  name  qty  active\n")
		djangotest.AssertEqual(t, errOut.String(), "")

		out.Reset()
		runShell(t, app, `ShellItem.filter(name='bread\'; DROP TABLE shop_shellitem; --')`)
		runShell(t, app, "ShellItem.count()")
		djangotest.AssertEqual(t, strings.TrimSpace(out.String()), "id  name  qty  active\n1")
	})
}

// TestShellErrors prints an error for a bad statement and goes on with the
// next one, as an interactive interpreter does.
func TestShellErrors(t *testing.T) {
	db := djangotest.NewDB(t, djangotest.App("shop", &ShellItem{}))
	app, out, errOut := shellApp(t, db)

	runShell(t, app, strings.Join([]string{
		"Missing.count()",
		"ShellItem.filter(colour='red')",
		"ShellItem.filter(qty='many')",
		"ShellItem.count(1)",
		"ShellItem.filter(name='open)",
		"frobnicate",
		"ShellItem.count()",
	}, "\n"))

	djangotest.AssertEqual(t, errOut.String(), strings.Join([]string{
		`Error: no model named "Missing"; 'models' lists them`,
		`Error: cannot resolve keyword "colour" into field of shop.ShellItem`,
		`Error: field "qty" expected a number but got "many"`,
		`Error: count() takes no arguments`,
		`Error: unterminated string 'open`,
		`Error: unknown statement "frobnicate"; 'help' lists the statements`,
		"",
	}, "\n"))
	djangotest.AssertEqual(t, out.String(), "0\n")
}

// TestShellModelsAndFields lists the models and the fields of a model.
func TestShellModelsAndFields(t *testing.T) {
	db := djangotest.NewDB(t, djangotest.App("shop", &ShellItem{}))
	app, out, _ := shellApp(t, db)

	runShell(t, app, "models")
	djangotest.AssertEqual(t, out.String(), "shop.ShellItem  shop_shellitem\n")

	out.Reset()
	runShell(t, app, "fields ShellItem")
	djangotest.AssertEqual(t, out.String(), ""+
		"id      Auto  id      primary key\n"+
		"name    Char  name    \n"+
		"qty     Int   qty     \n"+
		"active  Bool  active  \n")
}

// TestShellTruncatesLongResults shows at most 20 rows with Django's
// truncation line, as the repr of a QuerySet does.
func TestShellTruncatesLongResults(t *testing.T) {
	db := djangotest.NewDB(t, djangotest.App("shop", &ShellItem{}))
	rows := make([]ShellItem, 0, 25)
	for i := range 25 {
		rows = append(rows, ShellItem{Name: fmt.Sprintf("item%02d", i), Qty: int64(i)})
	}
	insertItems(t, db, rows...)
	app, out, _ := shellApp(t, db)

	runShell(t, app, "ShellItem.all()")
	lines := strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n")
	djangotest.AssertEqual(t, len(lines), 1+20+1)
	djangotest.AssertEqual(t, lines[len(lines)-1], "...(remaining elements truncated)...")
}

// TestShellRawSQL runs raw SQL and shows the rows, or OK for a statement
// that returns none.
func TestShellRawSQL(t *testing.T) {
	db := djangotest.NewDB(t, djangotest.App("shop", &ShellItem{}))
	app, out, errOut := shellApp(t, db)

	runShell(t, app, "sql SELECT 1 AS one, NULL AS empty")
	djangotest.AssertEqual(t, errOut.String(), "")
	djangotest.AssertEqual(t, out.String(), "one  empty\n1    None\n")

	out.Reset()
	runShell(t, app, "sql DELETE FROM shop_shellitem")
	djangotest.AssertEqual(t, out.String(), "OK\n")

	runShell(t, app, "sql SELEC")
	checkContains(t, errOut.String(), "Error: ")
}

// TestShellInteractive reads statements from the input with a prompt until
// exit, and ends at the end of the input as well.
func TestShellInteractive(t *testing.T) {
	db := djangotest.NewDB(t, djangotest.App("shop", &ShellItem{}))
	app, out, _ := shellApp(t, db)

	app.In = strings.NewReader("ShellItem.count()\nexit\nShellItem.count()\n")
	if err := app.Execute([]string{"shell"}); err != nil {
		t.Fatalf("shell: %v", err)
	}
	djangotest.AssertEqual(t, out.String(), "Djan-Go-Go "+djangogo.Version+
		" query shell. Type 'help' for the statements, 'exit' to leave.\n>>> 0\n>>> ")

	out.Reset()
	app.In = strings.NewReader("help\n")
	if err := app.Execute([]string{"shell"}); err != nil {
		t.Fatalf("shell: %v", err)
	}
	checkContains(t, out.String(), "<Model>.filter(attr=value, ...)")
	if !strings.HasSuffix(out.String(), ">>> \n") {
		t.Errorf("the shell should end on the end of the input with a line end, got %q", out)
	}
}

// TestShellWithoutDatabase reports the missing database for a query and
// still lists the models.
func TestShellWithoutDatabase(t *testing.T) {
	app, out, errOut := shellApp(t, nil)
	runShell(t, app, "sql SELECT 1\nmodels")
	djangotest.AssertEqual(t, errOut.String(), "Error: no database is configured\n")
	djangotest.AssertEqual(t, out.String(), "")
}
