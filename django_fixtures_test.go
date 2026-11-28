package djangogo

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/apps"
	"github.com/oliverhaas/djangogo/auth"
	"github.com/oliverhaas/djangogo/conf"
	"github.com/oliverhaas/djangogo/contenttypes"
	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/manage"
	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/serializers"
)

// Site is django.contrib.sites.models.Site for the ported fixture tests.
type Site struct {
	ID     int64
	Domain string `orm:"max_length=100"`
	Name   string `orm:"max_length=50"`
}

// Meta orders Site rows by domain and names the table django_site.
func (Site) Meta() orm.Meta {
	return orm.Meta{Table: "django_site", Ordering: []string{"domain"}}
}

// Category is tests/fixtures/models.py::Category.
type Category struct {
	ID          int64
	Title       string `orm:"max_length=100"`
	Description string `orm:"type=text"`
}

// Meta orders Category rows by title.
func (Category) Meta() orm.Meta {
	return orm.Meta{Ordering: []string{"title"}}
}

// String returns the category title.
func (c Category) String() string { return c.Title }

// Article is tests/fixtures/models.py::Article.
type Article struct {
	ID       int64
	Headline string `orm:"max_length=100;default=Default headline"`
	PubDate  time.Time
}

// Meta orders Article rows by -pub_date, headline.
func (Article) Meta() orm.Meta {
	return orm.Meta{Ordering: []string{"-pub_date", "headline"}}
}

// String returns the article headline.
func (a Article) String() string { return a.Headline }

// Blog is tests/fixtures/models.py::Blog.
type Blog struct {
	orm.RelCache
	ID       int64
	Name     string           `orm:"max_length=100"`
	Featured orm.FK[Article]  `orm:"on_delete=cascade;related_name=fixtures_featured_set"`
	Articles orm.M2M[Article] `orm:"blank;related_name=fixtures_articles_set"`
}

// String returns the blog name.
func (b Blog) String() string { return b.Name }

// Tag is tests/fixtures/models.py::Tag.
type Tag struct {
	ID         int64
	Name       string                           `orm:"max_length=100"`
	TaggedType orm.FK[contenttypes.ContentType] `orm:"on_delete=cascade;related_name=fixtures_tag_set"`
	TaggedID   int64                            `orm:"type=positive_int;default=0"`
	Tagged     contenttypes.GenericForeignKey   `orm:"ct_field=tagged_type;fk_field=tagged_id"`
}

// repr formats Tag the way Django's repr(Tag) does:
// '<Tag: <Article: Headline> tagged "name">'.
func (tg Tag) repr(ctx context.Context, db *orm.DB) (string, error) {
	obj, err := contenttypes.GetObject(ctx, db, &tg, "tagged")
	if err != nil {
		return "", err
	}
	m, _ := db.Registry().ModelOf(obj)
	return fmt.Sprintf(`<Tag: <%s: %s> tagged "%s">`, m.Name(), orm.Label(m, obj), tg.Name), nil
}

// Person is tests/fixtures/models.py::Person.
type Person struct {
	ID   int64
	Name string `orm:"max_length=100;unique"`
}

// Meta orders Person rows by name.
func (Person) Meta() orm.Meta {
	return orm.Meta{Ordering: []string{"name"}}
}

// String returns the person name.
func (p Person) String() string { return p.Name }

// NaturalKey returns the single-element natural key [name] of Person.
func (p Person) NaturalKey() []any { return []any{p.Name} }

// GetByNaturalKey looks up a Person row by name.
func (*Person) GetByNaturalKey(ctx context.Context, db *orm.DB, key ...any) (any, error) {
	if len(key) != 1 {
		return nil, orm.ErrDoesNotExist
	}
	row, err := orm.Query[Person](db).Get(ctx, "name", key[0])
	if err != nil {
		return nil, err
	}
	return &row, nil
}

// Visa is tests/fixtures/models.py::Visa.
type Visa struct {
	orm.RelCache
	ID          int64
	Person      orm.FK[Person]           `orm:"on_delete=cascade"`
	Permissions orm.M2M[auth.Permission] `orm:"blank"`
}

// repr formats Visa the way Django's repr(Visa) does:
// "<Visa: PersonName perm1, perm2>".
func (v Visa) repr(ctx context.Context, db *orm.DB) (string, error) {
	person, err := orm.Query[Person](db).Get(ctx, "id", v.Person.PK())
	if err != nil {
		return "", err
	}
	perms, err := orm.Many[auth.Permission](db, &v, "permissions").OrderBy("id").All(ctx)
	if err != nil {
		return "", err
	}
	names := make([]string, len(perms))
	for i, p := range perms {
		names[i] = p.Name
	}
	return fmt.Sprintf("<Visa: %s %s>", person.Name, strings.Join(names, ", ")), nil
}

// Book is tests/fixtures/models.py::Book.
type Book struct {
	orm.RelCache
	ID      int64
	Name    string `orm:"max_length=100"`
	Authors orm.M2M[Person]
}

// Meta orders Book rows by name.
func (Book) Meta() orm.Meta {
	return orm.Meta{Ordering: []string{"name"}}
}

// String returns the book name.
func (b Book) String() string { return b.Name }

// PrimaryKeyUUIDModel is tests/fixtures/models.py::PrimaryKeyUUIDModel.
type PrimaryKeyUUIDModel struct {
	ID orm.UUID `orm:"pk;default=uuid4"`
}

// NaturalKeyThing is tests/fixtures/models.py::NaturalKeyThing.
type NaturalKeyThing struct {
	orm.RelCache
	ID          int64
	Key         string                   `orm:"max_length=100;unique"`
	OtherThing  orm.FK[NaturalKeyThing]  `orm:"null;on_delete=cascade"`
	OtherThings orm.M2M[NaturalKeyThing] `orm:"related_name=thing_m2m_set;symmetrical=false"`
}

// String returns the key of NaturalKeyThing.
func (n NaturalKeyThing) String() string { return n.Key }

// NaturalKey returns the single-element natural key [key] of NaturalKeyThing.
func (n NaturalKeyThing) NaturalKey() []any { return []any{n.Key} }

// GetByNaturalKey looks up a NaturalKeyThing row by key.
func (*NaturalKeyThing) GetByNaturalKey(ctx context.Context, db *orm.DB, key ...any) (any, error) {
	if len(key) != 1 {
		return nil, orm.ErrDoesNotExist
	}
	row, err := orm.Query[NaturalKeyThing](db).Get(ctx, "key", key[0])
	if err != nil {
		return nil, err
	}
	return &row, nil
}

// CircularA is tests/fixtures/models.py::CircularA.
type CircularA struct {
	ID  int64
	Key string            `orm:"max_length=3;unique"`
	Obj orm.FK[CircularB] `orm:"null;on_delete=set_null"`
}

// NaturalKey returns the single-element natural key [key] of CircularA.
func (c CircularA) NaturalKey() []any { return []any{c.Key} }

// GetByNaturalKey looks up a CircularA row by key.
func (*CircularA) GetByNaturalKey(ctx context.Context, db *orm.DB, key ...any) (any, error) {
	if len(key) != 1 {
		return nil, orm.ErrDoesNotExist
	}
	row, err := orm.Query[CircularA](db).Get(ctx, "key", key[0])
	if err != nil {
		return nil, err
	}
	return &row, nil
}

// CircularB is tests/fixtures/models.py::CircularB.
type CircularB struct {
	ID  int64
	Key string            `orm:"max_length=3;unique"`
	Obj orm.FK[CircularA] `orm:"null;on_delete=set_null"`
}

// NaturalKey returns the single-element natural key [key] of CircularB.
func (c CircularB) NaturalKey() []any { return []any{c.Key} }

// GetByNaturalKey looks up a CircularB row by key.
func (*CircularB) GetByNaturalKey(ctx context.Context, db *orm.DB, key ...any) (any, error) {
	if len(key) != 1 {
		return nil, orm.ErrDoesNotExist
	}
	row, err := orm.Query[CircularB](db).Get(ctx, "key", key[0])
	if err != nil {
		return nil, err
	}
	return &row, nil
}

// sitesTestApp is the sites app for the fixture tests.
type sitesTestApp struct{}

// Name returns "sites".
func (sitesTestApp) Name() string { return "sites" }

// Models returns the Site model.
func (sitesTestApp) Models() []any { return []any{&Site{}} }

// fixturesTestApp is the fixtures app of Django's tests/fixtures.
type fixturesTestApp struct {
	fsys fs.FS
}

// Name returns "fixtures".
func (fixturesTestApp) Name() string { return "fixtures" }

// Models returns all models of tests/fixtures/models.py.
func (fixturesTestApp) Models() []any {
	return []any{
		&Category{},
		&Article{},
		&Blog{},
		&Tag{},
		&Person{},
		&Visa{},
		&Book{},
		&PrimaryKeyUUIDModel{},
		&NaturalKeyThing{},
		&CircularA{},
		&CircularB{},
	}
}

// Fixtures returns the testdata/fixtures filesystem.
func (a fixturesTestApp) Fixtures() fs.FS { return a.fsys }

// fixturesTestDataDir holds the absolute path to testdata/fixtures.
var fixturesTestDataDir = func() string {
	dir, err := filepath.Abs(filepath.Join("testdata", "fixtures"))
	if err != nil {
		panic(err)
	}
	return dir
}()

// fixtureHarness holds a test Application and its output buffer.
type fixtureHarness struct {
	t   *testing.T
	app *Application
	db  *orm.DB
	out *bytes.Buffer
}

// eachFixtureApp runs fn against SQLite and PostgreSQL with the contenttypes,
// auth, sites, and fixtures apps installed and their tables, content types, and
// permissions created.
func eachFixtureApp(t *testing.T, fn func(t *testing.T, h *fixtureHarness)) {
	t.Helper()
	configs := []apps.Config{
		contenttypes.App{},
		auth.App{},
		sitesTestApp{},
		fixturesTestApp{fsys: os.DirFS(fixturesTestDataDir)},
	}
	models := []any{
		contenttypes.App{},
		auth.App{},
		sitesTestApp{},
		fixturesTestApp{},
	}
	djangotest.EachDB(t, models, func(t *testing.T, db *orm.DB) {
		t.Helper()
		ctx := context.Background()
		app, err := New(conf.Settings{SecretKey: "k"}, configs...)
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		out := &bytes.Buffer{}
		app.Out = out
		app.Err = out
		app.DB = db
		app.Models = db.Registry()
		if _, err := auth.SyncPermissions(ctx, db); err != nil {
			t.Fatalf("SyncPermissions: %v", err)
		}
		fn(t, &fixtureHarness{t: t, app: app, db: db, out: out})
	})
}

// loaddata runs the loaddata command with args and fails the test on error.
func (h *fixtureHarness) loaddata(args ...string) string {
	h.t.Helper()
	h.out.Reset()
	if err := h.app.Execute(append([]string{"loaddata"}, args...)); err != nil {
		h.t.Fatalf("loaddata %v: %v", args, err)
	}
	return h.out.String()
}

// loaddataErr runs the loaddata command with args and returns its error.
func (h *fixtureHarness) loaddataErr(args ...string) error {
	h.t.Helper()
	h.out.Reset()
	return h.app.Execute(append([]string{"loaddata"}, args...))
}

// dumpdataOpts configures a dumpdata assertion call.
type dumpdataOpts struct {
	format             string
	filename           string
	naturalForeignKeys bool
	naturalPrimaryKeys bool
	useBaseManager     bool
	excludeList        []string
	primaryKeys        string
}

// dumpdataAssert runs dumpdata with args and opts and asserts that the output
// matches wantOutput, porting DumpDataAssertMixin._dumpdata_assert.
func (h *fixtureHarness) dumpdataAssert(args []string, wantOutput string, opts dumpdataOpts) {
	h.t.Helper()
	if err := h.dumpdataRun(args, wantOutput, opts); err != nil {
		h.t.Fatalf("dumpdata %v: %v", args, err)
	}
}

// dumpdataRun executes dumpdata with args and opts, returning any command error
// or asserting the output against wantOutput.
func (h *fixtureHarness) dumpdataRun(args []string, wantOutput string, opts dumpdataOpts) error {
	h.t.Helper()
	format := opts.format
	if format == "" {
		format = "json"
	}
	cmdArgs := []string{"dumpdata", "--format", format}
	var filePath string
	if opts.filename != "" {
		filePath = filepath.Join(h.t.TempDir(), opts.filename)
		cmdArgs = append(cmdArgs, "-o", filePath)
	}
	if opts.naturalForeignKeys {
		cmdArgs = append(cmdArgs, "--natural-foreign")
	}
	if opts.naturalPrimaryKeys {
		cmdArgs = append(cmdArgs, "--natural-primary")
	}
	if opts.useBaseManager {
		cmdArgs = append(cmdArgs, "--all")
	}
	for _, ex := range opts.excludeList {
		cmdArgs = append(cmdArgs, "-e", ex)
	}
	if opts.primaryKeys != "" {
		cmdArgs = append(cmdArgs, "--pks", opts.primaryKeys)
	}
	cmdArgs = append(cmdArgs, args...)

	h.out.Reset()
	if err := h.app.Execute(cmdArgs); err != nil {
		return err
	}

	var commandOutput string
	if filePath != "" {
		commandOutput = readDumpedFile(h.t, filePath)
	} else {
		commandOutput = strings.TrimSpace(h.out.String())
	}
	if format == "json" {
		djangotest.AssertJSONEqual(h.t, commandOutput, wantOutput)
	} else {
		djangotest.AssertEqual(h.t, commandOutput, wantOutput)
	}
	return nil
}

// readDumpedFile reads the content of a file written by dumpdata, decompressing
// .gz files and resolving .bz2, .lzma, .xz, and .zip to their stripped file
// root as Django's _dumpdata_assert does.
func readDumpedFile(t *testing.T, filePath string) string {
	t.Helper()
	fileRoot, fileExt := splitExt(filePath)
	switch fileExt {
	case ".gz":
		f, err := os.Open(filePath)
		if err != nil {
			t.Fatalf("open %s: %v", filePath, err)
		}
		defer func() { _ = f.Close() }()
		gz, err := gzip.NewReader(f)
		if err != nil {
			t.Fatalf("gzip %s: %v", filePath, err)
		}
		defer func() { _ = gz.Close() }()
		b, err := io.ReadAll(gz)
		if err != nil {
			t.Fatalf("read gzip %s: %v", filePath, err)
		}
		return string(b)
	case ".bz2", ".lzma", ".xz", ".zip":
		b, err := os.ReadFile(fileRoot)
		if err != nil {
			t.Fatalf("read %s: %v", fileRoot, err)
		}
		return string(b)
	default:
		b, err := os.ReadFile(filePath)
		if err != nil {
			t.Fatalf("read %s: %v", filePath, err)
		}
		return string(b)
	}
}

// articleHeadlines returns the ordered headlines of all Article rows in db.
func (h *fixtureHarness) articleHeadlines() []string {
	h.t.Helper()
	rows, err := orm.Query[Article](h.db).All(context.Background())
	if err != nil {
		h.t.Fatalf("Query[Article]: %v", err)
	}
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.Headline
	}
	return out
}

// tagReprs returns the sorted repr strings of all Tag rows in db.
func (h *fixtureHarness) tagReprs() []string {
	h.t.Helper()
	ctx := context.Background()
	rows, err := orm.Query[Tag](h.db).All(ctx)
	if err != nil {
		h.t.Fatalf("Query[Tag]: %v", err)
	}
	out := make([]string, len(rows))
	for i, r := range rows {
		s, err := r.repr(ctx, h.db)
		if err != nil {
			h.t.Fatalf("Tag.repr: %v", err)
		}
		out[i] = s
	}
	slices.Sort(out)
	return out
}

// visaReprs returns the sorted repr strings of all Visa rows in db.
func (h *fixtureHarness) visaReprs() []string {
	h.t.Helper()
	ctx := context.Background()
	rows, err := orm.Query[Visa](h.db).All(ctx)
	if err != nil {
		h.t.Fatalf("Query[Visa]: %v", err)
	}
	out := make([]string, len(rows))
	for i, r := range rows {
		s, err := r.repr(ctx, h.db)
		if err != nil {
			h.t.Fatalf("Visa.repr: %v", err)
		}
		out[i] = s
	}
	slices.Sort(out)
	return out
}

// assertFixtureCommandError asserts that err is a *manage.CommandError whose
// message contains wantSubstr.
func assertFixtureCommandError(t *testing.T, err error, wantSubstr string) {
	t.Helper()
	var ce *manage.CommandError
	if !errors.As(err, &ce) {
		t.Fatalf("error = %v (%T), want *manage.CommandError containing %q", err, err, wantSubstr)
	}
	if !strings.Contains(ce.Error(), wantSubstr) {
		t.Errorf("command error = %q, want substring %q", ce.Error(), wantSubstr)
	}
}

// Ported from Django: tests/fixtures/tests.py::TestCaseFixtureLoadingTests.test_class_fixtures
// Check that loading fixture1.json and fixture2.json installs the expected 3 articles.
func TestTestCaseFixtureLoadingClassFixtures(t *testing.T) {
	eachFixtureApp(t, func(t *testing.T, h *fixtureHarness) {
		h.loaddata("fixture1.json", "fixture2.json")
		djangotest.AssertSequenceEqual(t, h.articleHeadlines(), []string{
			"Django conquers world!",
			"Copyright is fine the way it is",
			"Poker has no place on ESPN",
		})
	})
}

// No Go counterpart: SubclassTestCaseFixtureLoadingTests.test_class_fixtures:
// Python TestCase class inheritance overriding the class attribute fixtures = [].

// Ported from Django: tests/fixtures/tests.py::FixtureLoadingTests.test_loading_and_dumping
// Check loading JSON fixtures with natural keys and dumping apps and models in JSON format.
func TestFixtureLoadingLoadingAndDumping(t *testing.T) {
	eachFixtureApp(t, func(t *testing.T, h *fixtureHarness) {
		ctx := context.Background()
		contenttypes.ClearCache(h.db)
		if _, err := orm.Query[Site](h.db).Delete(ctx); err != nil {
			t.Fatal(err)
		}

		h.loaddata("fixture1.json", "-v", "0")
		djangotest.AssertSequenceEqual(t, h.articleHeadlines(), []string{
			"Time to reform copyright",
			"Poker has no place on ESPN",
		})

		wantInitialDump := `[{"pk": 1, "model": "fixtures.category", "fields": ` +
			`{"description": "Latest news stories", "title": "News Stories"}}, ` +
			`{"pk": 2, "model": "fixtures.article", "fields": ` +
			`{"headline": "Poker has no place on ESPN", ` +
			`"pub_date": "2006-06-16T12:00:00Z"}}, ` +
			`{"pk": 3, "model": "fixtures.article", "fields": ` +
			`{"headline": "Time to reform copyright", ` +
			`"pub_date": "2006-06-16T13:00:00Z"}}]`

		h.dumpdataAssert([]string{"fixtures"}, wantInitialDump, dumpdataOpts{})

		h.dumpdataAssert(
			[]string{"fixtures.Category"},
			`[{"pk": 1, "model": "fixtures.category", "fields": `+
				`{"description": "Latest news stories", "title": "News Stories"}}]`,
			dumpdataOpts{},
		)

		wantArticlesDump := `[{"pk": 2, "model": "fixtures.article", "fields": ` +
			`{"headline": "Poker has no place on ESPN", ` +
			`"pub_date": "2006-06-16T12:00:00Z"}}, ` +
			`{"pk": 3, "model": "fixtures.article", "fields": ` +
			`{"headline": "Time to reform copyright", ` +
			`"pub_date": "2006-06-16T13:00:00Z"}}]`

		h.dumpdataAssert([]string{"fixtures.Article"}, wantArticlesDump, dumpdataOpts{})
		h.dumpdataAssert([]string{"fixtures.Category", "fixtures.Article"}, wantInitialDump, dumpdataOpts{})
		h.dumpdataAssert([]string{"fixtures.Article", "fixtures.Article"}, wantArticlesDump, dumpdataOpts{})
		h.dumpdataAssert([]string{"fixtures.Article", "fixtures"}, wantInitialDump, dumpdataOpts{})
		h.dumpdataAssert([]string{"fixtures", "fixtures.Article"}, wantInitialDump, dumpdataOpts{})
		h.dumpdataAssert([]string{"fixtures"}, wantInitialDump, dumpdataOpts{})

		h.dumpdataAssert(
			[]string{"fixtures.Category", "sites"},
			`[{"pk": 1, "model": "fixtures.category", "fields": `+
				`{"description": "Latest news stories", "title": "News Stories"}}, `+
				`{"pk": 1, "model": "sites.site", "fields": `+
				`{"domain": "example.com", "name": "example.com"}}]`,
			dumpdataOpts{},
		)

		h.loaddata("fixture2.json", "-v", "0")
		djangotest.AssertSequenceEqual(t, h.articleHeadlines(), []string{
			"Django conquers world!",
			"Copyright is fine the way it is",
			"Poker has no place on ESPN",
		})

		// FIXME-PORT: Go: xml serializer format, Django: loads fixture3.xml here.

		h.loaddata("fixture6.json", "-v", "0")
		djangotest.AssertSequenceEqual(t, h.tagReprs(), []string{
			`<Tag: <Article: Copyright is fine the way it is> tagged "copyright">`,
			`<Tag: <Article: Copyright is fine the way it is> tagged "law">`,
		})

		// FIXME-PORT: Go: xml serializer format, Django: loads fixture7.xml here.

		h.loaddata("fixture8.json", "-v", "0")
		djangotest.AssertSequenceEqual(t, h.visaReprs(), []string{
			"<Visa: Django Reinhardt Can add user, Can change user, Can delete user>",
			"<Visa: Prince >",
			"<Visa: Stephane Grappelli Can add user>",
		})

		// FIXME-PORT: Go: xml serializer format, Django: loads fixture9.xml and dumps XML here.

		h.dumpdataAssert(
			[]string{"fixtures.person"},
			`[{"fields": {"name": "Django Reinhardt"}, "model": "fixtures.person"}, `+
				`{"fields": {"name": "Stephane Grappelli"}, "model": "fixtures.person"}, `+
				`{"fields": {"name": "Prince"}, "model": "fixtures.person"}]`,
			dumpdataOpts{naturalPrimaryKeys: true},
		)

		h.dumpdataAssert(
			[]string{"fixtures"},
			`[{"pk": 1, "model": "fixtures.category", "fields": `+
				`{"description": "Latest news stories", "title": "News Stories"}}, `+
				`{"pk": 2, "model": "fixtures.article", "fields": `+
				`{"headline": "Poker has no place on ESPN", `+
				`"pub_date": "2006-06-16T12:00:00Z"}}, `+
				`{"pk": 3, "model": "fixtures.article", "fields": `+
				`{"headline": "Copyright is fine the way it is", `+
				`"pub_date": "2006-06-16T14:00:00Z"}}, `+
				`{"pk": 4, "model": "fixtures.article", "fields": `+
				`{"headline": "Django conquers world!", `+
				`"pub_date": "2006-06-16T15:00:00Z"}}, `+
				`{"pk": 1, "model": "fixtures.tag", "fields": `+
				`{"tagged_type": ["fixtures", "article"], "name": "copyright", `+
				`"tagged_id": 3}}, `+
				`{"pk": 2, "model": "fixtures.tag", "fields": `+
				`{"tagged_type": ["fixtures", "article"], "name": "law", `+
				`"tagged_id": 3}}, `+
				`{"pk": 1, "model": "fixtures.person", `+
				`"fields": {"name": "Django Reinhardt"}}, `+
				`{"pk": 2, "model": "fixtures.person", `+
				`"fields": {"name": "Stephane Grappelli"}}, `+
				`{"pk": 3, "model": "fixtures.person", `+
				`"fields": {"name": "Prince"}}, `+
				`{"pk": 1, "model": "fixtures.visa", `+
				`"fields": {"person": ["Django Reinhardt"], "permissions": `+
				`[["add_user", "auth", "user"], ["change_user", "auth", "user"], `+
				`["delete_user", "auth", "user"]]}}, `+
				`{"pk": 2, "model": "fixtures.visa", `+
				`"fields": {"person": ["Stephane Grappelli"], "permissions": `+
				`[["add_user", "auth", "user"]]}}, `+
				`{"pk": 3, "model": "fixtures.visa", `+
				`"fields": {"person": ["Prince"], "permissions": []}}]`,
			dumpdataOpts{naturalForeignKeys: true},
		)
	})
}

// Ported from Django: tests/fixtures/tests.py::FixtureLoadingTests.test_dumpdata_with_excludes
// Check that dumpdata --exclude filters out specified apps and models and rejects unknown ones.
func TestFixtureLoadingDumpdataWithExcludes(t *testing.T) {
	eachFixtureApp(t, func(t *testing.T, h *fixtureHarness) {
		if _, err := orm.Query[Site](h.db).Delete(context.Background()); err != nil {
			t.Fatal(err)
		}
		h.loaddata("fixture1.json", "-v", "0")

		h.dumpdataAssert(
			[]string{"sites", "fixtures"},
			`[{"pk": 1, "model": "sites.site", "fields": `+
				`{"domain": "example.com", "name": "example.com"}}]`,
			dumpdataOpts{excludeList: []string{"fixtures"}},
		)

		h.dumpdataAssert(
			[]string{"sites", "fixtures"},
			`[{"pk": 1, "model": "sites.site", `+
				`"fields": {"domain": "example.com", "name": "example.com"}}, `+
				`{"pk": 1, "model": "fixtures.category", "fields": `+
				`{"description": "Latest news stories", "title": "News Stories"}}]`,
			dumpdataOpts{excludeList: []string{"fixtures.Article", "fixtures.Book"}},
		)

		h.dumpdataAssert(
			[]string{"sites", "fixtures"},
			`[{"pk": 1, "model": "sites.site", `+
				`"fields": {"domain": "example.com", "name": "example.com"}}, `+
				`{"pk": 1, "model": "fixtures.category", `+
				`"fields": {"description": "Latest news stories", `+
				`"title": "News Stories"}}]`,
			dumpdataOpts{excludeList: []string{"fixtures.Article", "fixtures.Book"}},
		)

		h.dumpdataAssert(
			[]string{"sites", "fixtures"},
			`[{"pk": 1, "model": "fixtures.category", "fields": `+
				`{"description": "Latest news stories", "title": "News Stories"}}]`,
			dumpdataOpts{excludeList: []string{"fixtures.Article", "fixtures.Book", "sites"}},
		)

		err := h.dumpdataRun([]string{"fixtures", "sites"}, "", dumpdataOpts{excludeList: []string{"foo_app"}})
		assertFixtureCommandError(t, err, "No installed app with label 'foo_app'.")

		err = h.dumpdataRun([]string{"fixtures", "sites"}, "", dumpdataOpts{excludeList: []string{"fixtures.FooModel"}})
		assertFixtureCommandError(t, err, "Unknown model: fixtures.FooModel")
	})
}

// Ported from Django: tests/fixtures/tests.py::FixtureLoadingTests.test_load_fixture_with_special_characters
// Check that loaddata discovers and loads fixture files whose names contain glob metacharacters.
func TestFixtureLoadingLoadFixtureWithSpecialCharacters(t *testing.T) {
	eachFixtureApp(t, func(t *testing.T, h *fixtureHarness) {
		h.loaddata("fixture_with[special]chars", "-v", "0")
		art, err := orm.Query[Article](h.db).Get(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		djangotest.AssertEqual(t, art.Headline, "How To Deal With Special Characters")
	})
}

// Ported from Django: tests/fixtures/tests.py::FixtureLoadingTests.test_dumpdata_with_filtering_manager
// Check that dumpdata uses a model's default filtering manager unless --all is passed.
func TestFixtureLoadingDumpdataWithFilteringManager(t *testing.T) {
	t.Skip("FIXME-PORT: Go: multi-table inheritance and custom model managers, Django: Spy inherits from Person and uses SpyManager to filter cover_blown=False unless --all is passed")
}

// Ported from Django: tests/fixtures/tests.py::FixtureLoadingTests.test_dumpdata_with_pks
// Check that dumpdata --pks dumps only the specified primary keys and requires a single model argument.
func TestFixtureLoadingDumpdataWithPKs(t *testing.T) {
	eachFixtureApp(t, func(t *testing.T, h *fixtureHarness) {
		h.loaddata("fixture1.json", "-v", "0")
		h.loaddata("fixture2.json", "-v", "0")

		h.dumpdataAssert(
			[]string{"fixtures.Article"},
			`[{"pk": 2, "model": "fixtures.article", `+
				`"fields": {"headline": "Poker has no place on ESPN", `+
				`"pub_date": "2006-06-16T12:00:00Z"}}, `+
				`{"pk": 3, "model": "fixtures.article", "fields": `+
				`{"headline": "Copyright is fine the way it is", `+
				`"pub_date": "2006-06-16T14:00:00Z"}}]`,
			dumpdataOpts{primaryKeys: "2,3"},
		)

		h.dumpdataAssert(
			[]string{"fixtures.Article"},
			`[{"pk": 2, "model": "fixtures.article", `+
				`"fields": {"headline": "Poker has no place on ESPN", `+
				`"pub_date": "2006-06-16T12:00:00Z"}}]`,
			dumpdataOpts{primaryKeys: "2"},
		)

		err := h.dumpdataRun([]string{"fixtures"}, "", dumpdataOpts{primaryKeys: "2,3"})
		assertFixtureCommandError(t, err, "You can only use --pks option with one model")

		err = h.dumpdataRun(nil, "", dumpdataOpts{primaryKeys: "2,3"})
		assertFixtureCommandError(t, err, "You can only use --pks option with one model")

		err = h.dumpdataRun([]string{"fixtures.Article", "fixtures.category"}, "", dumpdataOpts{primaryKeys: "2,3"})
		assertFixtureCommandError(t, err, "You can only use --pks option with one model")
	})
}

// Ported from Django: tests/fixtures/tests.py::FixtureLoadingTests.test_dumpdata_with_uuid_pks
// Check that dumpdata --pks accepts comma-separated UUID primary keys.
func TestFixtureLoadingDumpdataWithUUIDPKs(t *testing.T) {
	eachFixtureApp(t, func(t *testing.T, h *fixtureHarness) {
		ctx := context.Background()
		var m1, m2 PrimaryKeyUUIDModel
		if err := orm.Query[PrimaryKeyUUIDModel](h.db).Create(ctx, &m1); err != nil {
			t.Fatal(err)
		}
		if err := orm.Query[PrimaryKeyUUIDModel](h.db).Create(ctx, &m2); err != nil {
			t.Fatal(err)
		}
		h.out.Reset()
		err := h.app.Execute([]string{
			"dumpdata",
			"fixtures.PrimaryKeyUUIDModel",
			"--pks",
			m1.ID.String() + ", " + m2.ID.String(),
		})
		if err != nil {
			t.Fatal(err)
		}
		result := h.out.String()
		if !strings.Contains(result, fmt.Sprintf(`"pk": "%s"`, m1.ID)) {
			t.Errorf("output does not contain pk %s:\n%s", m1.ID, result)
		}
		if !strings.Contains(result, fmt.Sprintf(`"pk": "%s"`, m2.ID)) {
			t.Errorf("output does not contain pk %s:\n%s", m2.ID, result)
		}
	})
}

// Ported from Django: tests/fixtures/tests.py::FixtureLoadingTests.test_dumpdata_with_file_output
// Check that dumpdata -o writes serialized data to a file.
func TestFixtureLoadingDumpdataWithFileOutput(t *testing.T) {
	eachFixtureApp(t, func(_ *testing.T, h *fixtureHarness) {
		h.loaddata("fixture1.json", "-v", "0")
		h.dumpdataAssert(
			[]string{"fixtures"},
			`[{"pk": 1, "model": "fixtures.category", "fields": `+
				`{"description": "Latest news stories", "title": "News Stories"}}, `+
				`{"pk": 2, "model": "fixtures.article", "fields": `+
				`{"headline": "Poker has no place on ESPN", `+
				`"pub_date": "2006-06-16T12:00:00Z"}}, `+
				`{"pk": 3, "model": "fixtures.article", "fields": `+
				`{"headline": "Time to reform copyright", `+
				`"pub_date": "2006-06-16T13:00:00Z"}}]`,
			dumpdataOpts{filename: "dumpdata.json"},
		)
	})
}

// Ported from Django: tests/fixtures/tests.py::FixtureLoadingTests.test_dumpdata_with_file_gzip_output
// Check that dumpdata -o with a .gz extension writes gzip-compressed output.
func TestFixtureLoadingDumpdataWithFileGzipOutput(t *testing.T) {
	eachFixtureApp(t, func(_ *testing.T, h *fixtureHarness) {
		h.loaddata("fixture1.json", "-v", "0")
		h.dumpdataAssert(
			[]string{"fixtures"},
			`[{"pk": 1, "model": "fixtures.category", "fields": `+
				`{"description": "Latest news stories", "title": "News Stories"}}, `+
				`{"pk": 2, "model": "fixtures.article", "fields": `+
				`{"headline": "Poker has no place on ESPN", `+
				`"pub_date": "2006-06-16T12:00:00Z"}}, `+
				`{"pk": 3, "model": "fixtures.article", "fields": `+
				`{"headline": "Time to reform copyright", `+
				`"pub_date": "2006-06-16T13:00:00Z"}}]`,
			dumpdataOpts{filename: "dumpdata.json.gz"},
		)
	})
}

// Ported from Django: tests/fixtures/tests.py::FixtureLoadingTests.test_dumpdata_with_file_bz2_output
// Check that dumpdata -o with a .bz2 extension writes bzip2-compressed output.
func TestFixtureLoadingDumpdataWithFileBz2Output(t *testing.T) {
	t.Skip("FIXME-PORT: Go: writing bz2 is not in the standard library (compress/bzip2 is read-only), Django: dumps bz2-compressed fixture file")
}

// Ported from Django: tests/fixtures/tests.py::FixtureLoadingTests.test_dumpdata_with_file_lzma_output
// Check that dumpdata -o with a .lzma extension writes lzma-compressed output.
func TestFixtureLoadingDumpdataWithFileLzmaOutput(t *testing.T) {
	t.Skip("FIXME-PORT: Go: lzma compression is not in the standard library, Django: dumps lzma-compressed fixture file")
}

// Ported from Django: tests/fixtures/tests.py::FixtureLoadingTests.test_dumpdata_with_file_xz_output
// Check that dumpdata -o with a .xz extension writes xz-compressed output.
func TestFixtureLoadingDumpdataWithFileXzOutput(t *testing.T) {
	t.Skip("FIXME-PORT: Go: xz compression is not in the standard library, Django: dumps xz-compressed fixture file")
}

// Ported from Django: tests/fixtures/tests.py::FixtureLoadingTests.test_dumpdata_with_file_zip_output
// Check that dumpdata -o with a .zip extension warns that .zip is unsupported and saves uncompressed output to dumpdata.json.
func TestFixtureLoadingDumpdataWithFileZipOutput(t *testing.T) {
	eachFixtureApp(t, func(t *testing.T, h *fixtureHarness) {
		h.loaddata("fixture1.json", "-v", "0")
		errBuf := &bytes.Buffer{}
		h.app.Err = errBuf
		h.dumpdataAssert(
			[]string{"fixtures"},
			`[{"pk": 1, "model": "fixtures.category", "fields": `+
				`{"description": "Latest news stories", "title": "News Stories"}}, `+
				`{"pk": 2, "model": "fixtures.article", "fields": `+
				`{"headline": "Poker has no place on ESPN", `+
				`"pub_date": "2006-06-16T12:00:00Z"}}, `+
				`{"pk": 3, "model": "fixtures.article", "fields": `+
				`{"headline": "Time to reform copyright", `+
				`"pub_date": "2006-06-16T13:00:00Z"}}]`,
			dumpdataOpts{filename: "dumpdata.json.zip"},
		)
		wantMsg := "Unsupported file extension (.zip). Fixtures saved in 'dumpdata.json'."
		if !strings.Contains(errBuf.String(), wantMsg) {
			t.Errorf("stderr = %q, want substring %q", errBuf.String(), wantMsg)
		}
	})
}

// ttyBuffer is a bytes.Buffer that reports itself as a terminal for progress
// bar tests.
type ttyBuffer struct {
	bytes.Buffer
}

// IsTTY returns true so dumpdata writes its progress bar to the buffer.
func (*ttyBuffer) IsTTY() bool { return true }

// Ported from Django: tests/fixtures/tests.py::FixtureLoadingTests.test_dumpdata_progressbar
// Check that dumpdata displays a progress bar when --output is set, stdout is a tty, and verbosity > 0.
func TestFixtureLoadingDumpdataProgressbar(t *testing.T) {
	eachFixtureApp(t, func(t *testing.T, h *fixtureHarness) {
		h.loaddata("fixture1.json", "-v", "0")
		file := filepath.Join(t.TempDir(), "dump.json")

		ttyOut := &ttyBuffer{}
		h.app.Out = ttyOut
		h.app.Err = ttyOut
		if err := h.app.Execute([]string{"dumpdata", "fixtures", "--format", "json", "-o", file}); err != nil {
			t.Fatal(err)
		}
		wantSuffix := "[" + strings.Repeat(".", serializers.ProgressWidth) + "]\n"
		if !strings.HasSuffix(ttyOut.String(), wantSuffix) {
			t.Errorf("progress output = %q, want suffix %q", ttyOut.String(), wantSuffix)
		}

		ttyOut2 := &ttyBuffer{}
		h.app.Out = ttyOut2
		h.app.Err = ttyOut2
		if err := h.app.Execute([]string{"dumpdata", "fixtures", "--format", "json", "-o", file, "-v", "0"}); err != nil {
			t.Fatal(err)
		}
		djangotest.AssertEqual(t, ttyOut2.String(), "")
	})
}

// Ported from Django: tests/fixtures/tests.py::FixtureLoadingTests.test_dumpdata_proxy_without_concrete
// Check that dumpdata warns and skips a proxy model when its concrete parent is not dumped.
func TestFixtureLoadingDumpdataProxyWithoutConcrete(t *testing.T) {
	t.Skip("FIXME-PORT: Go: proxy models and multi-table inheritance, Django: warns that fixtures.ProxySpy is a proxy model and skips it")
}

// Ported from Django: tests/fixtures/tests.py::FixtureLoadingTests.test_dumpdata_proxy_with_concrete
// Check that dumpdata emits no warning when a proxy model is dumped alongside its concrete parent.
func TestFixtureLoadingDumpdataProxyWithConcrete(t *testing.T) {
	t.Skip("FIXME-PORT: Go: proxy models and multi-table inheritance, Django: dumps Spy without warning when both ProxySpy and Spy are requested")
}

// Ported from Django: tests/fixtures/tests.py::FixtureLoadingTests.test_dumpdata_objects_with_prefetch_related
// Check that dumpdata serializes Visa objects with natural foreign keys in 5 queries.
func TestFixtureLoadingDumpdataObjectsWithPrefetchRelated(t *testing.T) {
	eachFixtureApp(t, func(t *testing.T, h *fixtureHarness) {
		h.loaddata("fixture6.json", "fixture8.json", "-v", "0")
		djangotest.AssertNumQueries(t, h.db, 5, func() {
			h.dumpdataAssert(
				[]string{"fixtures.visa"},
				`[{"fields": {"permissions": [["add_user", "auth", "user"]],`+
					`"person": ["Stephane Grappelli"]},`+
					`"model": "fixtures.visa", "pk": 2},`+
					`{"fields": {"permissions": [], "person": ["Prince"]},`+
					`"model": "fixtures.visa", "pk": 3}]`,
				dumpdataOpts{
					naturalForeignKeys: true,
					primaryKeys:        "2,3",
				},
			)
		})
	})
}

// Ported from Django: tests/fixtures/tests.py::FixtureLoadingTests.test_compress_format_loading
// Check that loaddata discovers a zip-compressed fixture when given its format extension.
func TestFixtureLoadingCompressFormatLoading(t *testing.T) {
	eachFixtureApp(t, func(t *testing.T, h *fixtureHarness) {
		h.loaddata("fixture4.json", "-v", "0")
		art, err := orm.Query[Article](h.db).Get(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		djangotest.AssertEqual(t, art.Headline, "Django pets kitten")
	})
}

// Ported from Django: tests/fixtures/tests.py::FixtureLoadingTests.test_compressed_specified_loading
// Check that loaddata loads a fixture when both format and .zip compression are specified.
func TestFixtureLoadingCompressedSpecifiedLoading(t *testing.T) {
	eachFixtureApp(t, func(t *testing.T, h *fixtureHarness) {
		h.loaddata("fixture5.json.zip", "-v", "0")
		art, err := orm.Query[Article](h.db).Get(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		djangotest.AssertEqual(t, art.Headline, "WoW subscribers now outnumber readers")
	})
}

// Ported from Django: tests/fixtures/tests.py::FixtureLoadingTests.test_compressed_loading
// Check that loaddata discovers the serialization format when only the .zip compression extension is specified.
func TestFixtureLoadingCompressedLoading(t *testing.T) {
	eachFixtureApp(t, func(t *testing.T, h *fixtureHarness) {
		h.loaddata("fixture5.zip", "-v", "0")
		art, err := orm.Query[Article](h.db).Get(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		djangotest.AssertEqual(t, art.Headline, "WoW subscribers now outnumber readers")
	})
}

// Ported from Django: tests/fixtures/tests.py::FixtureLoadingTests.test_compressed_loading_gzip
// Check that loaddata loads a gzip-compressed fixture.
func TestFixtureLoadingCompressedLoadingGzip(t *testing.T) {
	eachFixtureApp(t, func(t *testing.T, h *fixtureHarness) {
		h.loaddata("fixture5.json.gz", "-v", "0")
		art, err := orm.Query[Article](h.db).Get(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		djangotest.AssertEqual(t, art.Headline, "WoW subscribers now outnumber readers")
	})
}

// Ported from Django: tests/fixtures/tests.py::FixtureLoadingTests.test_compressed_loading_bz2
// Check that loaddata loads a bzip2-compressed fixture.
func TestFixtureLoadingCompressedLoadingBz2(t *testing.T) {
	eachFixtureApp(t, func(t *testing.T, h *fixtureHarness) {
		h.loaddata("fixture5.json.bz2", "-v", "0")
		art, err := orm.Query[Article](h.db).Get(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		djangotest.AssertEqual(t, art.Headline, "WoW subscribers now outnumber readers")
	})
}

// Ported from Django: tests/fixtures/tests.py::FixtureLoadingTests.test_compressed_loading_lzma
// Check that loaddata loads an lzma-compressed fixture.
func TestFixtureLoadingCompressedLoadingLzma(t *testing.T) {
	t.Skip("FIXME-PORT: Go: lzma compression is not in the standard library, Django: loads fixture5.json.lzma")
}

// Ported from Django: tests/fixtures/tests.py::FixtureLoadingTests.test_compressed_loading_xz
// Check that loaddata loads an xz-compressed fixture.
func TestFixtureLoadingCompressedLoadingXz(t *testing.T) {
	t.Skip("FIXME-PORT: Go: xz compression is not in the standard library, Django: loads fixture5.json.xz")
}

// Ported from Django: tests/fixtures/tests.py::FixtureLoadingTests.test_ambiguous_compressed_fixture
// Check that loaddata returns a CommandError when a fixture name matches multiple compressed files in one directory.
func TestFixtureLoadingAmbiguousCompressedFixture(t *testing.T) {
	eachFixtureApp(t, func(t *testing.T, h *fixtureHarness) {
		err := h.loaddataErr("fixture5", "-v", "0")
		assertFixtureCommandError(t, err, "Multiple fixtures named 'fixture5'")
	})
}

// Ported from Django: tests/fixtures/tests.py::FixtureLoadingTests.test_db_loading
// Check that loaddata discovers database-specific fixtures for the default database implicitly.
func TestFixtureLoadingDBLoading(t *testing.T) {
	eachFixtureApp(t, func(t *testing.T, h *fixtureHarness) {
		h.loaddata("db_fixture_1", "-v", "0")
		h.loaddata("db_fixture_2", "-v", "0")
		djangotest.AssertSequenceEqual(t, h.articleHeadlines(), []string{
			"Who needs more than one database?",
			"Who needs to use compressed data?",
		})
	})
}

// Ported from Django: tests/fixtures/tests.py::FixtureLoadingTests.test_loaddata_error_message
// Check that loading an invalid object reports an IntegrityError naming the model label and primary key.
func TestFixtureLoadingLoaddataErrorMessage(t *testing.T) {
	eachFixtureApp(t, func(t *testing.T, h *fixtureHarness) {
		err := h.loaddataErr("invalid.json", "-v", "0")
		djangotest.AssertErrorIs(t, err, orm.ErrIntegrity)
		if err == nil || !strings.Contains(err.Error(), "Could not load fixtures.Article(pk=1):") {
			t.Errorf("error = %v, want substring %q", err, "Could not load fixtures.Article(pk=1):")
		}
	})
}

// Ported from Django: tests/fixtures/tests.py::FixtureLoadingTests.test_loaddata_null_characters_on_postgresql
// Check that loading a text value containing a NUL byte on PostgreSQL reports the model label and primary key.
func TestFixtureLoadingLoaddataNullCharactersOnPostgreSQL(t *testing.T) {
	eachFixtureApp(t, func(t *testing.T, h *fixtureHarness) {
		if h.db.Dialect().Name() != "postgres" {
			t.Skip("SQLite allows NUL characters in text")
		}
		err := h.loaddataErr("null_character_in_field_value.json")
		if err == nil || !strings.Contains(err.Error(), "Could not load fixtures.Article(pk=2):") {
			t.Errorf("error = %v, want substring %q", err, "Could not load fixtures.Article(pk=2):")
		}
	})
}

// Ported from Django: tests/fixtures/tests.py::FixtureLoadingTests.test_loaddata_app_option
// Check that loaddata --app restricts app fixture search to the specified app label.
func TestFixtureLoadingLoaddataAppOption(t *testing.T) {
	eachFixtureApp(t, func(t *testing.T, h *fixtureHarness) {
		err := h.loaddataErr("db_fixture_1", "-v", "0", "--app", "someotherapp")
		assertFixtureCommandError(t, err, "No fixture named 'db_fixture_1' found.")
		djangotest.AssertSequenceEqual(t, h.articleHeadlines(), []string{})

		h.loaddata("db_fixture_1", "-v", "0", "--app", "fixtures")
		art, err := orm.Query[Article](h.db).Get(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		djangotest.AssertEqual(t, art.Headline, "Who needs more than one database?")
	})
}

// Ported from Django: tests/fixtures/tests.py::FixtureLoadingTests.test_loaddata_verbosity_three
// Check that loaddata at verbosity 3 prints per-object progress updates.
func TestFixtureLoadingLoaddataVerbosityThree(t *testing.T) {
	eachFixtureApp(t, func(t *testing.T, h *fixtureHarness) {
		out := h.loaddata("fixture1.json", "-v", "3")
		want := "\rProcessed 1 object(s).\rProcessed 2 object(s).\rProcessed 3 object(s).\rProcessed 4 object(s).\n"
		if !strings.Contains(out, want) {
			t.Errorf("output does not contain %q:\n%s", want, out)
		}
	})
}

// Ported from Django: tests/fixtures/tests.py::FixtureLoadingTests.test_loading_using
// Check that loaddata --database default loads database-qualified fixtures for the default database.
func TestFixtureLoadingLoadingUsing(t *testing.T) {
	eachFixtureApp(t, func(t *testing.T, h *fixtureHarness) {
		h.loaddata("db_fixture_1", "-v", "0", "--database", "default")
		h.loaddata("db_fixture_2", "-v", "0", "--database", "default")
		djangotest.AssertSequenceEqual(t, h.articleHeadlines(), []string{
			"Who needs more than one database?",
			"Who needs to use compressed data?",
		})
	})
}

// Ported from Django: tests/fixtures/tests.py::FixtureLoadingTests.test_unmatched_identifier_loading
// Check that loaddata ignores fixture files qualified for a different database alias.
func TestFixtureLoadingUnmatchedIdentifierLoading(t *testing.T) {
	eachFixtureApp(t, func(t *testing.T, h *fixtureHarness) {
		err := h.loaddataErr("db_fixture_3", "-v", "0")
		assertFixtureCommandError(t, err, "No fixture named 'db_fixture_3' found.")

		err = h.loaddataErr("db_fixture_3", "-v", "0", "--database", "default")
		assertFixtureCommandError(t, err, "No fixture named 'db_fixture_3' found.")
		djangotest.AssertSequenceEqual(t, h.articleHeadlines(), []string{})
	})
}

// Ported from Django: tests/fixtures/tests.py::FixtureLoadingTests.test_output_formats
// Check format discovery when loading fixture1 and fixture6 and dumping with natural foreign keys.
func TestFixtureLoadingOutputFormats(t *testing.T) {
	eachFixtureApp(t, func(t *testing.T, h *fixtureHarness) {
		h.loaddata("fixture1", "-v", "0")
		h.loaddata("fixture6", "-v", "0")
		djangotest.AssertSequenceEqual(t, h.tagReprs(), []string{
			`<Tag: <Article: Time to reform copyright> tagged "copyright">`,
			`<Tag: <Article: Time to reform copyright> tagged "law">`,
		})

		h.dumpdataAssert(
			[]string{"fixtures"},
			`[{"pk": 1, "model": "fixtures.category", "fields": `+
				`{"description": "Latest news stories", "title": "News Stories"}}, `+
				`{"pk": 2, "model": "fixtures.article", "fields": `+
				`{"headline": "Poker has no place on ESPN", `+
				`"pub_date": "2006-06-16T12:00:00Z"}}, `+
				`{"pk": 3, "model": "fixtures.article", "fields": `+
				`{"headline": "Time to reform copyright", `+
				`"pub_date": "2006-06-16T13:00:00Z"}}, `+
				`{"pk": 1, "model": "fixtures.tag", "fields": `+
				`{"tagged_type": ["fixtures", "article"], "name": "copyright", `+
				`"tagged_id": 3}}, `+
				`{"pk": 2, "model": "fixtures.tag", "fields": `+
				`{"tagged_type": ["fixtures", "article"], "name": "law", "tagged_id": 3}}, `+
				`{"pk": 1, "model": "fixtures.person", "fields": `+
				`{"name": "Django Reinhardt"}}, `+
				`{"pk": 2, "model": "fixtures.person", "fields": `+
				`{"name": "Stephane Grappelli"}}, `+
				`{"pk": 3, "model": "fixtures.person", "fields": {"name": "Prince"}}]`,
			dumpdataOpts{naturalForeignKeys: true},
		)

		// FIXME-PORT: Go: xml serializer format, Django: also dumps in XML format here.
	})
}

// Ported from Django: tests/fixtures/tests.py::FixtureLoadingTests.test_loading_with_exclude_app
// Check that loaddata --exclude skips all objects belonging to an excluded app.
func TestFixtureLoadingLoadingWithExcludeApp(t *testing.T) {
	eachFixtureApp(t, func(t *testing.T, h *fixtureHarness) {
		ctx := context.Background()
		if _, err := orm.Query[Site](h.db).Delete(ctx); err != nil {
			t.Fatal(err)
		}
		h.loaddata("fixture1", "-e", "fixtures", "-v", "0")
		hasArt, err := orm.Query[Article](h.db).Exists(ctx)
		if err != nil {
			t.Fatal(err)
		}
		djangotest.AssertEqual(t, hasArt, false)
		hasCat, err := orm.Query[Category](h.db).Exists(ctx)
		if err != nil {
			t.Fatal(err)
		}
		djangotest.AssertEqual(t, hasCat, false)
		site, err := orm.Query[Site](h.db).Get(ctx)
		if err != nil {
			t.Fatal(err)
		}
		djangotest.AssertEqual(t, site.Domain, "example.com")
	})
}

// Ported from Django: tests/fixtures/tests.py::FixtureLoadingTests.test_loading_with_exclude_model
// Check that loaddata --exclude skips all objects belonging to an excluded model.
func TestFixtureLoadingLoadingWithExcludeModel(t *testing.T) {
	eachFixtureApp(t, func(t *testing.T, h *fixtureHarness) {
		ctx := context.Background()
		if _, err := orm.Query[Site](h.db).Delete(ctx); err != nil {
			t.Fatal(err)
		}
		h.loaddata("fixture1", "-e", "fixtures.Article", "-v", "0")
		hasArt, err := orm.Query[Article](h.db).Exists(ctx)
		if err != nil {
			t.Fatal(err)
		}
		djangotest.AssertEqual(t, hasArt, false)
		cat, err := orm.Query[Category](h.db).Get(ctx)
		if err != nil {
			t.Fatal(err)
		}
		djangotest.AssertEqual(t, cat.Title, "News Stories")
		site, err := orm.Query[Site](h.db).Get(ctx)
		if err != nil {
			t.Fatal(err)
		}
		djangotest.AssertEqual(t, site.Domain, "example.com")
	})
}

// Ported from Django: tests/fixtures/tests.py::FixtureLoadingTests.test_exclude_option_errors
// Check that loaddata --exclude raises a CommandError for an unknown app or model.
func TestFixtureLoadingExcludeOptionErrors(t *testing.T) {
	eachFixtureApp(t, func(t *testing.T, h *fixtureHarness) {
		err := h.loaddataErr("fixture1", "-e", "foo_app", "-v", "0")
		assertFixtureCommandError(t, err, "No installed app with label 'foo_app'.")

		err = h.loaddataErr("fixture1", "-e", "fixtures.FooModel", "-v", "0")
		assertFixtureCommandError(t, err, "Unknown model: fixtures.FooModel")
	})
}

// Ported from Django: tests/fixtures/tests.py::FixtureLoadingTests.test_stdin_without_format
// Check that loaddata "-" without --format returns a CommandError.
func TestFixtureLoadingStdinWithoutFormat(t *testing.T) {
	eachFixtureApp(t, func(t *testing.T, h *fixtureHarness) {
		err := h.loaddataErr("-", "-v", "0")
		assertFixtureCommandError(t, err, "--format must be specified when reading from stdin.")
	})
}

// Ported from Django: tests/fixtures/tests.py::FixtureLoadingTests.test_loading_stdin
// Check that loaddata "-" reads fixture data from standard input with --format=json.
func TestFixtureLoadingLoadingStdin(t *testing.T) {
	eachFixtureApp(t, func(t *testing.T, h *fixtureHarness) {
		data, err := os.ReadFile(filepath.Join(fixturesTestDataDir, "fixture1.json"))
		if err != nil {
			t.Fatal(err)
		}
		h.app.In = bytes.NewReader(data)
		h.loaddata("--format=json", "-", "-v", "0")
		djangotest.AssertSequenceEqual(t, h.articleHeadlines(), []string{
			"Time to reform copyright",
			"Poker has no place on ESPN",
		})

		// FIXME-PORT: Go: xml serializer format, Django: also tests loading fixture3.xml from stdin with --format=xml.
	})
}

// Ported from Django: tests/fixtures/tests.py::NonexistentFixtureTests.test_loaddata_not_existent_fixture_file
// Check that loaddata returns a CommandError when a fixture cannot be found.
func TestNonexistentFixtureLoaddataNotExistentFixtureFile(t *testing.T) {
	eachFixtureApp(t, func(t *testing.T, h *fixtureHarness) {
		err := h.loaddataErr("this_fixture_doesnt_exist")
		assertFixtureCommandError(t, err, "No fixture named 'this_fixture_doesnt_exist' found.")
	})
}

// Ported from Django: tests/fixtures/tests.py::NonexistentFixtureTests.test_nonexistent_fixture_no_constraint_checking
// Check that loaddata performs no database queries when no fixtures match.
func TestNonexistentFixtureNoConstraintChecking(t *testing.T) {
	eachFixtureApp(t, func(t *testing.T, h *fixtureHarness) {
		djangotest.AssertNumQueries(t, h.db, 0, func() {
			err := h.loaddataErr("this_fixture_doesnt_exist", "-v", "0")
			assertFixtureCommandError(t, err, "No fixture named 'this_fixture_doesnt_exist' found.")
		})
	})
}

// Ported from Django: tests/fixtures/tests.py::FixtureTransactionTests.test_format_discovery
// Check that loaddata discovers uncompressed and compressed JSON fixtures by base name.
func TestFixtureTransactionFormatDiscovery(t *testing.T) {
	eachFixtureApp(t, func(t *testing.T, h *fixtureHarness) {
		h.loaddata("fixture1", "-v", "0")
		djangotest.AssertSequenceEqual(t, h.articleHeadlines(), []string{
			"Time to reform copyright",
			"Poker has no place on ESPN",
		})

		// FIXME-PORT: Go: xml serializer format, Django: loaddata "fixture2" is ambiguous between fixture2.json and fixture2.xml.

		h.dumpdataAssert(
			[]string{"fixtures"},
			`[{"pk": 1, "model": "fixtures.category", "fields": `+
				`{"description": "Latest news stories", "title": "News Stories"}}, `+
				`{"pk": 2, "model": "fixtures.article", "fields": `+
				`{"headline": "Poker has no place on ESPN", `+
				`"pub_date": "2006-06-16T12:00:00Z"}}, `+
				`{"pk": 3, "model": "fixtures.article", "fields": `+
				`{"headline": "Time to reform copyright", `+
				`"pub_date": "2006-06-16T13:00:00Z"}}]`,
			dumpdataOpts{},
		)

		h.loaddata("fixture4", "-v", "0")
		djangotest.AssertSequenceEqual(t, h.articleHeadlines(), []string{
			"Django pets kitten",
			"Time to reform copyright",
			"Poker has no place on ESPN",
		})
	})
}

// Ported from Django: tests/fixtures/tests.py::ForwardReferenceTests.test_forward_reference_fk
// Check loading and dumping objects with mutual forward-referenced integer foreign keys.
func TestForwardReferenceFK(t *testing.T) {
	eachFixtureApp(t, func(t *testing.T, h *fixtureHarness) {
		h.loaddata("forward_reference_fk.json", "-v", "0")
		rows, err := orm.Query[NaturalKeyThing](h.db).OrderBy("id").All(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		djangotest.AssertEqual(t, len(rows), 2)
		t1, t2 := rows[0], rows[1]
		djangotest.AssertEqual(t, t1.OtherThing.PK(), t2.ID)
		djangotest.AssertEqual(t, t2.OtherThing.PK(), t1.ID)

		h.dumpdataAssert(
			[]string{"fixtures"},
			`[{"model": "fixtures.naturalkeything", "pk": 1, `+
				`"fields": {"key": "t1", "other_thing": 2, "other_things": []}}, `+
				`{"model": "fixtures.naturalkeything", "pk": 2, `+
				`"fields": {"key": "t2", "other_thing": 1, "other_things": []}}]`,
			dumpdataOpts{},
		)
	})
}

// Ported from Django: tests/fixtures/tests.py::ForwardReferenceTests.test_forward_reference_fk_natural_key
// Check loading and dumping objects with mutual forward-referenced natural foreign keys.
func TestForwardReferenceFKNaturalKey(t *testing.T) {
	eachFixtureApp(t, func(t *testing.T, h *fixtureHarness) {
		h.loaddata("forward_reference_fk_natural_key.json", "-v", "0")
		rows, err := orm.Query[NaturalKeyThing](h.db).OrderBy("id").All(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		djangotest.AssertEqual(t, len(rows), 2)
		t1, t2 := rows[0], rows[1]
		djangotest.AssertEqual(t, t1.OtherThing.PK(), t2.ID)
		djangotest.AssertEqual(t, t2.OtherThing.PK(), t1.ID)

		h.dumpdataAssert(
			[]string{"fixtures"},
			`[{"model": "fixtures.naturalkeything", `+
				`"fields": {"key": "t1", "other_thing": ["t2"], "other_things": []}}, `+
				`{"model": "fixtures.naturalkeything", `+
				`"fields": {"key": "t2", "other_thing": ["t1"], "other_things": []}}]`,
			dumpdataOpts{
				naturalPrimaryKeys: true,
				naturalForeignKeys: true,
			},
		)
	})
}

// Ported from Django: tests/fixtures/tests.py::ForwardReferenceTests.test_forward_reference_m2m
// Check loading and dumping objects with forward-referenced integer many-to-many relations.
func TestForwardReferenceM2M(t *testing.T) {
	eachFixtureApp(t, func(t *testing.T, h *fixtureHarness) {
		ctx := context.Background()
		h.loaddata("forward_reference_m2m.json", "-v", "0")
		n, err := orm.Query[NaturalKeyThing](h.db).Count(ctx)
		if err != nil {
			t.Fatal(err)
		}
		djangotest.AssertEqual(t, n, int64(3))

		t1, err := orm.Query[NaturalKeyThing](h.db).Get(ctx, "key", "t1")
		if err != nil {
			t.Fatal(err)
		}
		related, err := orm.Many[NaturalKeyThing](h.db, &t1, "other_things").OrderBy("key").All(ctx)
		if err != nil {
			t.Fatal(err)
		}
		keys := make([]string, len(related))
		for i, r := range related {
			keys[i] = r.Key
		}
		djangotest.AssertSequenceEqual(t, keys, []string{"t2", "t3"})

		h.dumpdataAssert(
			[]string{"fixtures"},
			`[{"model": "fixtures.naturalkeything", "pk": 1, `+
				`"fields": {"key": "t1", "other_thing": null, "other_things": [2, 3]}}, `+
				`{"model": "fixtures.naturalkeything", "pk": 2, `+
				`"fields": {"key": "t2", "other_thing": null, "other_things": []}}, `+
				`{"model": "fixtures.naturalkeything", "pk": 3, `+
				`"fields": {"key": "t3", "other_thing": null, "other_things": []}}]`,
			dumpdataOpts{},
		)
	})
}

// Ported from Django: tests/fixtures/tests.py::ForwardReferenceTests.test_forward_reference_m2m_natural_key
// Check loading and dumping objects with forward-referenced natural many-to-many relations.
func TestForwardReferenceM2MNaturalKey(t *testing.T) {
	eachFixtureApp(t, func(t *testing.T, h *fixtureHarness) {
		ctx := context.Background()
		h.loaddata("forward_reference_m2m_natural_key.json", "-v", "0")
		n, err := orm.Query[NaturalKeyThing](h.db).Count(ctx)
		if err != nil {
			t.Fatal(err)
		}
		djangotest.AssertEqual(t, n, int64(3))

		t1, err := orm.Query[NaturalKeyThing](h.db).Get(ctx, "key", "t1")
		if err != nil {
			t.Fatal(err)
		}
		related, err := orm.Many[NaturalKeyThing](h.db, &t1, "other_things").OrderBy("key").All(ctx)
		if err != nil {
			t.Fatal(err)
		}
		keys := make([]string, len(related))
		for i, r := range related {
			keys[i] = r.Key
		}
		djangotest.AssertSequenceEqual(t, keys, []string{"t2", "t3"})

		h.dumpdataAssert(
			[]string{"fixtures"},
			`[{"model": "fixtures.naturalkeything", `+
				`"fields": {"key": "t1", "other_thing": null, `+
				`"other_things": [["t2"], ["t3"]]}}, `+
				`{"model": "fixtures.naturalkeything", `+
				`"fields": {"key": "t2", "other_thing": null, "other_things": []}}, `+
				`{"model": "fixtures.naturalkeything", `+
				`"fields": {"key": "t3", "other_thing": null, "other_things": []}}]`,
			dumpdataOpts{
				naturalPrimaryKeys: true,
				naturalForeignKeys: true,
			},
		)
	})
}

// Ported from Django: tests/fixtures/tests.py::CircularReferenceTests.test_circular_reference
// Check loading and dumping mutually referencing models with integer foreign keys.
func TestCircularReference(t *testing.T) {
	eachFixtureApp(t, func(t *testing.T, h *fixtureHarness) {
		ctx := context.Background()
		h.loaddata("circular_reference.json", "-v", "0")
		objA, err := orm.Query[CircularA](h.db).Get(ctx)
		if err != nil {
			t.Fatal(err)
		}
		objB, err := orm.Query[CircularB](h.db).Get(ctx)
		if err != nil {
			t.Fatal(err)
		}
		djangotest.AssertEqual(t, objA.Obj.PK(), objB.ID)
		djangotest.AssertEqual(t, objB.Obj.PK(), objA.ID)

		h.dumpdataAssert(
			[]string{"fixtures"},
			`[{"model": "fixtures.circulara", "pk": 1, `+
				`"fields": {"key": "x", "obj": 1}}, `+
				`{"model": "fixtures.circularb", "pk": 1, `+
				`"fields": {"key": "y", "obj": 1}}]`,
			dumpdataOpts{},
		)
	})
}

// Ported from Django: tests/fixtures/tests.py::CircularReferenceTests.test_circular_reference_natural_key
// Check loading and dumping mutually referencing models with natural keys.
func TestCircularReferenceNaturalKey(t *testing.T) {
	eachFixtureApp(t, func(t *testing.T, h *fixtureHarness) {
		ctx := context.Background()
		h.loaddata("circular_reference_natural_key.json", "-v", "0")
		objA, err := orm.Query[CircularA](h.db).Get(ctx)
		if err != nil {
			t.Fatal(err)
		}
		objB, err := orm.Query[CircularB](h.db).Get(ctx)
		if err != nil {
			t.Fatal(err)
		}
		djangotest.AssertEqual(t, objA.Obj.PK(), objB.ID)
		djangotest.AssertEqual(t, objB.Obj.PK(), objA.ID)

		h.dumpdataAssert(
			[]string{"fixtures"},
			`[{"model": "fixtures.circulara", `+
				`"fields": {"key": "x", "obj": ["y"]}}, `+
				`{"model": "fixtures.circularb", `+
				`"fields": {"key": "y", "obj": ["x"]}}]`,
			dumpdataOpts{
				naturalPrimaryKeys: true,
				naturalForeignKeys: true,
			},
		)
	})
}

// TestFlushCommand checks interactive cancellation, interactive confirmation,
// non-interactive flush, and post-migrate hooks of the flush command.
func TestFlushCommand(t *testing.T) {
	eachFixtureApp(t, func(t *testing.T, h *fixtureHarness) {
		ctx := context.Background()
		h.loaddata("fixture1.json", "-v", "0")

		hookRuns := 0
		h.app.OnPostMigrate(func(_ context.Context, _ *orm.DB) error {
			hookRuns++
			return nil
		})

		h.out.Reset()
		h.app.In = strings.NewReader("no\n")
		if err := h.app.Execute([]string{"flush"}); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(h.out.String(), "Flush cancelled.") {
			t.Errorf("output does not contain 'Flush cancelled.':\n%s", h.out.String())
		}
		n, err := orm.Query[Article](h.db).Count(ctx)
		if err != nil {
			t.Fatal(err)
		}
		djangotest.AssertEqual(t, n, int64(2))
		djangotest.AssertEqual(t, hookRuns, 0)

		h.out.Reset()
		h.app.In = strings.NewReader("yes\n")
		if err := h.app.Execute([]string{"flush"}); err != nil {
			t.Fatal(err)
		}
		n, err = orm.Query[Article](h.db).Count(ctx)
		if err != nil {
			t.Fatal(err)
		}
		djangotest.AssertEqual(t, n, int64(0))
		djangotest.AssertEqual(t, hookRuns, 1)

		h.loaddata("fixture1.json", "-v", "0")
		h.out.Reset()
		if err := h.app.Execute([]string{"flush", "--noinput"}); err != nil {
			t.Fatal(err)
		}
		n, err = orm.Query[Article](h.db).Count(ctx)
		if err != nil {
			t.Fatal(err)
		}
		djangotest.AssertEqual(t, n, int64(0))
		djangotest.AssertEqual(t, hookRuns, 2)
	})
}
