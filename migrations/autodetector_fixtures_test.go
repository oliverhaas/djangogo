package migrations_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/migrations"
	"github.com/oliverhaas/djangogo/orm"
)

// models is a list of model states, the argument of Django's
// make_project_state.
type models = []*migrations.ModelState

// model returns a model state with the table Django derives from the names.
func model(app, name string, fields ...migrations.FieldState) *migrations.ModelState {
	return &migrations.ModelState{App: app, Name: name, Table: app + "_" + strings.ToLower(name), Fields: fields}
}

// variant returns a copy of ms with change applied to it.
func variant(ms *migrations.ModelState, change func(*migrations.ModelState)) *migrations.ModelState {
	out := *ms
	out.Fields = slices.Clone(ms.Fields)
	change(&out)
	return &out
}

// withTable returns ms with another table, Django's db_table option.
func withTable(ms *migrations.ModelState, table string) *migrations.ModelState {
	return variant(ms, func(m *migrations.ModelState) { m.Table = table })
}

// withIndexes returns ms with model-level indexes.
func withIndexes(ms *migrations.ModelState, indexes ...migrations.Index) *migrations.ModelState {
	return variant(ms, func(m *migrations.ModelState) { m.Indexes = indexes })
}

// withUniqueTogether returns ms with unique-together sets.
func withUniqueTogether(ms *migrations.ModelState, sets ...[]string) *migrations.ModelState {
	return variant(ms, func(m *migrations.ModelState) { m.UniqueTogether = sets })
}

// charField returns a CharField.
func charField(name, column string, maxLength int) migrations.FieldState {
	return migrations.FieldState{Name: name, Column: column, Kind: orm.KindChar, MaxLength: maxLength}
}

// intField returns an IntegerField.
func intField(name, column string) migrations.FieldState {
	return migrations.FieldState{Name: name, Column: column, Kind: orm.KindInt}
}

// dateTimeField returns a DateTimeField.
func dateTimeField(name, column string) migrations.FieldState {
	return migrations.FieldState{Name: name, Column: column, Kind: orm.KindDateTime}
}

// nullable returns f with null=True.
func nullable(f migrations.FieldState) migrations.FieldState {
	f.Null = true
	return f
}

// withDefault returns f with a default.
func withDefault(f migrations.FieldState, value any) migrations.FieldState {
	f.Default = value
	return f
}

// The model states of Django's AutodetectorTests, with Go names for models
// and fields. Go has one date-time kind, so the three date fields of the
// author_dates_of_birth states are all date-time fields. Tests must not change
// the states: makeProjectState copies them.
var (
	nameField  = charField("Name", "name", 200)
	titleField = charField("Title", "title", 200)

	authorEmpty       = model("testapp", "Author", idField)
	authorName        = model("testapp", "Author", idField, nameField)
	authorNameNull    = model("testapp", "Author", idField, nullable(nameField))
	authorNameLonger  = model("testapp", "Author", idField, charField("Name", "name", 400))
	authorNameRenamed = model("testapp", "Author", idField, charField("Names", "names", 200))
	authorNameDefault = model("testapp", "Author", idField, withDefault(nameField, "Ada Lovelace"))

	authorDatesOfBirthAutoNow = model("testapp", "Author", idField,
		autoNow(dateTimeField("DateOfBirth", "date_of_birth")),
		autoNow(dateTimeField("DateTimeOfBirth", "date_time_of_birth")),
		autoNow(dateTimeField("TimeOfBirth", "time_of_birth")))
	authorDatesOfBirthAutoNowAdd = model("testapp", "Author", idField,
		autoNowAdd(dateTimeField("DateOfBirth", "date_of_birth")),
		autoNowAdd(dateTimeField("DateTimeOfBirth", "date_time_of_birth")),
		autoNowAdd(dateTimeField("TimeOfBirth", "time_of_birth")))

	authorWithBiographyNonBlank = model("testapp", "Author", idField,
		charField("Name", "name", 0),
		migrations.FieldState{Name: "Biography", Column: "biography", Kind: orm.KindText})
	authorWithBiographyBlank = model("testapp", "Author", idField,
		blank(charField("Name", "name", 0)),
		blank(migrations.FieldState{Name: "Biography", Column: "biography", Kind: orm.KindText}))

	authorWithBook          = model("testapp", "Author", idField, nameField, fkField("Book", "book_id", "otherapp.Book"))
	authorRenamedWithBook   = model("testapp", "Writer", idField, nameField, fkField("Book", "book_id", "otherapp.Book"))
	authorWithPublisherName = model("testapp", "Author", idField, nameField,
		charField("PublisherName", "publisher_name", 200))
	authorWithPublisher = model("testapp", "Author", idField, nameField,
		fkField("Publisher", "publisher_id", "testapp.Publisher"))

	authorWithDBTableOptions           = withTable(authorEmpty, "author_one")
	authorWithNewDBTableOptions        = withTable(authorEmpty, "author_two")
	authorRenamedWithDBTableOptions    = withTable(model("testapp", "NewAuthor", idField), "author_one")
	authorRenamedWithNewDBTableOptions = withTable(model("testapp", "NewAuthor", idField), "author_three")

	publisher           = model("testapp", "Publisher", idField, charField("Name", "name", 100))
	publisherWithAuthor = model("testapp", "Publisher", idField,
		fkField("Author", "author_id", "testapp.Author"), charField("Name", "name", 100))
	publisherWithAardvarkAuthor = model("testapp", "Publisher", idField,
		fkField("Author", "author_id", "testapp.Aardvark"), charField("Name", "name", 100))
	publisherWithBook = model("testapp", "Publisher", idField,
		fkField("Author", "author_id", "otherapp.Book"), charField("Name", "name", 100))

	otherPony       = model("otherapp", "Pony", idField)
	otherStable     = model("otherapp", "Stable", idField)
	thirdThing      = model("thirdapp", "Thing", idField)
	aardvarkTestapp = model("testapp", "Aardvark", idField)

	book = model("otherapp", "Book", idField, fkField("Author", "author_id", "testapp.Author"), titleField)
	// bookMigrationsFK points to a model of the "migrations" fixture app.
	bookMigrationsFK = model("otherapp", "Book", idField,
		fkField("Author", "author_id", "migrations.UnmigratedModel"), titleField)
	bookWithNoAuthorFK     = model("otherapp", "Book", idField, intField("Author", "author"), titleField)
	bookWithNoAuthor       = model("otherapp", "Book", idField, titleField)
	bookWithAuthorRenamed  = model("otherapp", "Book", idField, fkField("Author", "author_id", "testapp.Writer"), titleField)
	bookWithFieldAndAuthor = model("otherapp", "Book", idField, fkField("Writer", "writer_id", "testapp.Writer"), titleField)

	bookIndexes = withIndexes(book,
		migrations.Index{Name: "book_title_author_idx", Fields: []string{"Author", "Title"}})
	bookUnorderedIndexes = withIndexes(book,
		migrations.Index{Name: "book_author_title_idx", Fields: []string{"Title", "Author"}})
	bookUniqueTogether  = withUniqueTogether(book, []string{"Author", "Title"})
	bookUniqueTogether2 = withUniqueTogether(book, []string{"Title", "Author"})
	bookUniqueTogether3 = withUniqueTogether(
		model("otherapp", "Book", idField, intField("Newfield", "newfield"),
			fkField("Author", "author_id", "testapp.Author"), titleField),
		[]string{"Title", "Newfield"})
	bookUniqueTogether4 = withUniqueTogether(
		model("otherapp", "Book", idField, intField("Newfield2", "newfield2"),
			fkField("Author", "author_id", "testapp.Author"), titleField),
		[]string{"Title", "Newfield2"})

	attribution = model("otherapp", "Attribution", idField,
		fkField("Author", "author_id", "testapp.Author"), fkField("Book", "book_id", "otherapp.Book"))
	knight = model("eggs", "Knight", idField)
	rabbit = variant(
		model("eggs", "Rabbit", idField,
			fkField("Knight", "knight_id", "eggs.Knight"), fkField("Parent", "parent_id", "eggs.Rabbit")),
		func(m *migrations.ModelState) {
			m.UniqueTogether = [][]string{{"Parent", "Knight"}}
			m.Indexes = []migrations.Index{{Name: "rabbit_circular_fk_index", Fields: []string{"Parent", "Knight"}}}
		})
)

// autoNow returns f with auto_now=True.
func autoNow(f migrations.FieldState) migrations.FieldState {
	f.AutoNow = true
	return f
}

// autoNowAdd returns f with auto_now_add=True.
func autoNowAdd(f migrations.FieldState) migrations.FieldState {
	f.AutoNowAdd = true
	return f
}

// blank returns f with blank=True.
func blank(f migrations.FieldState) migrations.FieldState {
	f.Blank = true
	return f
}

// makeProjectState returns a project state with copies of the given models
// (Django's make_project_state).
func makeProjectState(states ...*migrations.ModelState) *migrations.ProjectState {
	ps := migrations.NewProjectState()
	for _, ms := range states {
		ps.AddModel(ms)
	}
	return ps.Clone()
}

// getChanges returns the migrations the autodetector generates for the change
// from before to after, with the placeholder names "auto_1", "auto_2" (Django's
// get_changes). A nil questioner answers no to every question.
func getChanges(t *testing.T, before, after models, q migrations.Questioner) map[string][]*migrations.Migration {
	t.Helper()
	a := migrations.NewAutodetector(makeProjectState(before...), makeProjectState(after...), q)
	changes, err := a.DetectChanges(nil)
	if err != nil {
		t.Fatalf("DetectChanges: %v", err)
	}
	return changes
}

// reprChanges prints the generated migrations for a failure message.
func reprChanges(changes map[string][]*migrations.Migration) string {
	apps := make([]string, 0, len(changes))
	for app := range changes {
		apps = append(apps, app)
	}
	slices.Sort(apps)
	var b strings.Builder
	for _, app := range apps {
		fmt.Fprintf(&b, "  %s:\n", app)
		for _, m := range changes[app] {
			fmt.Fprintf(&b, "    %s %v\n", m.Name, m.Dependencies)
			for _, op := range m.Operations {
				fmt.Fprintf(&b, "      %#v\n", op)
			}
		}
	}
	return b.String()
}

// assertNumberMigrations checks how many migrations app got.
func assertNumberMigrations(t *testing.T, changes map[string][]*migrations.Migration, app string, number int) {
	t.Helper()
	if len(changes[app]) != number {
		t.Fatalf("incorrect number of migrations (%d) for %s (expected %d)\n%s",
			len(changes[app]), app, number, reprChanges(changes))
	}
}

// migrationAt returns the migration at position of app.
func migrationAt(t *testing.T, changes map[string][]*migrations.Migration, app string, position int) *migrations.Migration {
	t.Helper()
	if len(changes[app]) <= position {
		t.Fatalf("no migration found for %s at position %d\n%s", app, position, reprChanges(changes))
	}
	return changes[app][position]
}

// assertMigrationDependencies compares the dependencies of a migration as a
// set, as Django does.
func assertMigrationDependencies(
	t *testing.T, changes map[string][]*migrations.Migration, app string, position int, deps ...migrations.Key,
) {
	t.Helper()
	m := migrationAt(t, changes, app, position)
	if deps == nil {
		deps = []migrations.Key{}
	}
	got := m.Dependencies
	if got == nil {
		got = []migrations.Key{}
	}
	djangotest.AssertSequenceEqual(t, got, deps, djangotest.Unordered())
}

// assertOperationTypes compares the Go type names of the operations of a
// migration, such as "CreateModel".
func assertOperationTypes(
	t *testing.T, changes map[string][]*migrations.Migration, app string, position int, types ...string,
) {
	t.Helper()
	m := migrationAt(t, changes, app, position)
	got := make([]string, 0, len(m.Operations))
	for _, op := range m.Operations {
		got = append(got, strings.TrimPrefix(fmt.Sprintf("%T", op), "migrations."))
	}
	if !slices.Equal(got, types) {
		t.Fatalf("operation type mismatch for %s.%s (expected %v):\n%s", app, m.Name, types, reprChanges(changes))
	}
}

// operationAt returns the operation at opPosition of a migration, which must
// have the type T.
func operationAt[T migrations.Operation](
	t *testing.T, changes map[string][]*migrations.Migration, app string, position, opPosition int,
) T {
	t.Helper()
	m := migrationAt(t, changes, app, position)
	if len(m.Operations) <= opPosition {
		t.Fatalf("no operation at index %d for %s.%s\n%s", opPosition, app, m.Name, reprChanges(changes))
	}
	op, ok := m.Operations[opPosition].(T)
	if !ok {
		t.Fatalf("operation %d of %s.%s is a %T\n%s", opPosition, app, m.Name, m.Operations[opPosition], reprChanges(changes))
	}
	return op
}

// countingQuestioner answers like the embedded FixedQuestioner and counts the
// questions about defaults. It stands in for the mock.patch of a questioner
// method in the Django tests.
type countingQuestioner struct {
	migrations.FixedQuestioner
	additions, alterations int
}

// AskNotNullAddition counts the question and answers with the fixed value.
func (q *countingQuestioner) AskNotNullAddition(ms *migrations.ModelState, f migrations.FieldState) (any, error) {
	q.additions++
	return q.FixedQuestioner.AskNotNullAddition(ms, f)
}

// AskNotNullAlteration counts the question and answers with the fixed value.
func (q *countingQuestioner) AskNotNullAlteration(ms *migrations.ModelState, f migrations.FieldState) (any, error) {
	q.alterations++
	return q.FixedQuestioner.AskNotNullAlteration(ms, f)
}
