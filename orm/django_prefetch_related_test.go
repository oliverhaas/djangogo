package orm_test

import (
	"context"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
)

// Models ported from Django's tests/prefetch_related/models.py.
// Multi-table inheritance and GenericForeignKey/GenericRelation models are out of scope.

type prAuthor struct {
	orm.RelCache
	ID              int64
	Name            string            `orm:"max_length=50;unique"`
	FirstBook       orm.FK[prBook]    `orm:"on_delete=cascade;related_name=first_time_authors"`
	Books           orm.M2M[prBook]   `orm:"related_name=authors"`
	FavoriteAuthors orm.M2M[prAuthor] `orm:"through=prFavoriteAuthors;symmetrical=false;related_name=favors_me"`
}

func (prAuthor) Meta() orm.Meta   { return orm.Meta{Ordering: []string{"id"}} }
func (a prAuthor) String() string { return a.Name }

// prPlainAuthor omits orm.RelCache to verify forward FK prefetching works
// without RelCache by populating FK.SetObject directly.
type prPlainAuthor struct {
	ID        int64
	Name      string         `orm:"max_length=50"`
	FirstBook orm.FK[prBook] `orm:"on_delete=cascade;related_name=plain_authors"`
}

func (prPlainAuthor) Meta() orm.Meta { return orm.Meta{Ordering: []string{"id"}} }

type prFavoriteAuthors struct {
	ID          int64
	Author      orm.FK[prAuthor] `orm:"on_delete=cascade;related_name=i_like"`
	LikesAuthor orm.FK[prAuthor] `orm:"on_delete=cascade;related_name=likes_me"`
	IsActive    bool
}

func (prFavoriteAuthors) Meta() orm.Meta { return orm.Meta{Ordering: []string{"id"}} }

type prAuthorAddress struct {
	orm.RelCache
	ID      int64
	Author  orm.FK[prAuthor] `orm:"on_delete=cascade;related_name=addresses"`
	Address string           `orm:"type=text"`
}

func (prAuthorAddress) Meta() orm.Meta { return orm.Meta{Ordering: []string{"id"}} }

type prBook struct {
	orm.RelCache
	ID    int64
	Title string `orm:"max_length=255"`
}

func (prBook) Meta() orm.Meta   { return orm.Meta{Ordering: []string{"id"}} }
func (b prBook) String() string { return b.Title }

type prBio struct {
	orm.RelCache
	Author orm.O2O[prAuthor] `orm:"pk;on_delete=cascade;related_name=bio"`
	Books  orm.M2M[prBook]   `orm:"blank"`
}

type prBookProfile struct {
	orm.RelCache
	ID   int64
	Book orm.O2O[prBook] `orm:"on_delete=cascade;related_name=bookprofile"`
	Note string          `orm:"max_length=100"`
}

type prReader struct {
	orm.RelCache
	ID        int64
	Name      string          `orm:"max_length=50"`
	BooksRead orm.M2M[prBook] `orm:"related_name=read_by"`
}

func (prReader) Meta() orm.Meta   { return orm.Meta{Ordering: []string{"id"}} }
func (r prReader) String() string { return r.Name }

type prBookReview struct {
	orm.RelCache
	ID    int64
	Book  orm.FK[prBook] `orm:"null;on_delete=cascade"`
	Notes string         `orm:"type=text;blank"`
}

func (prBookReview) Meta() orm.Meta { return orm.Meta{Ordering: []string{"id"}} }

type prEmployee struct {
	orm.RelCache
	ID   int64
	Name string             `orm:"max_length=50"`
	Boss orm.FK[prEmployee] `orm:"null;on_delete=set_null;related_name=serfs"`
}

func (prEmployee) Meta() orm.Meta { return orm.Meta{Ordering: []string{"id"}} }

type prLessonEntry struct {
	orm.RelCache
	ID    int64
	Name1 string `orm:"max_length=200"`
	Name2 string `orm:"max_length=200"`
}

type prWordEntry struct {
	orm.RelCache
	ID          int64
	LessonEntry orm.FK[prLessonEntry] `orm:"on_delete=cascade;related_name=wordentry_set"`
	Name        string                `orm:"max_length=200"`
}

type prAuthor2 struct {
	orm.RelCache
	ID            int64
	Name          string          `orm:"max_length=50;unique"`
	FirstBook     orm.FK[prBook]  `orm:"on_delete=cascade;related_name=first_time_authors+"`
	FavoriteBooks orm.M2M[prBook] `orm:"related_name=+"`
}

func (prAuthor2) Meta() orm.Meta { return orm.Meta{Ordering: []string{"id"}} }

type prPet struct {
	orm.RelCache
	ID     orm.UUID          `orm:"pk"`
	Name   string            `orm:"max_length=20"`
	People orm.M2M[prPerson] `orm:"related_name=pets"`
}

func (prPet) Meta() orm.Meta { return orm.Meta{Ordering: []string{"name"}} }

// prModels returns the model list for the prefetch_related test app.
func prModels() []any {
	return []any{djangotest.App("prefetch_related",
		&prBook{}, &prAuthor{}, &prPlainAuthor{}, &prFavoriteAuthors{},
		&prAuthorAddress{}, &prBio{}, &prBookProfile{}, &prReader{},
		&prBookReview{}, &prEmployee{}, &prLessonEntry{}, &prWordEntry{},
		&prAuthor2{}, &prHouse{}, &prRoom{}, &prPerson{}, &prPet{},
	)}
}

// prFixture holds the shared TestDataMixin rows from tests/prefetch_related/tests.py.
type prFixture struct {
	book1, book2, book3, book4         *prBook
	author1, author2, author3, author4 *prAuthor
	reader1, reader2                   *prReader
}

// prSetUp populates the shared books, authors, and readers fixture.
func prSetUp(t *testing.T, db *orm.DB) prFixture {
	t.Helper()
	ctx := context.Background()
	b1 := create(t, db, &prBook{Title: "Poems"})
	b2 := create(t, db, &prBook{Title: "Jane Eyre"})
	b3 := create(t, db, &prBook{Title: "Wuthering Heights"})
	b4 := create(t, db, &prBook{Title: "Sense and Sensibility"})

	a1 := create(t, db, &prAuthor{Name: "Charlotte", FirstBook: fkTo[prBook](b1.ID)})
	a2 := create(t, db, &prAuthor{Name: "Anne", FirstBook: fkTo[prBook](b1.ID)})
	a3 := create(t, db, &prAuthor{Name: "Emily", FirstBook: fkTo[prBook](b1.ID)})
	a4 := create(t, db, &prAuthor{Name: "Jane", FirstBook: fkTo[prBook](b4.ID)})

	if err := orm.Many[prAuthor](db, b1, "authors").Add(ctx, a1, a2, a3); err != nil {
		t.Fatalf("b1.authors.Add: %v", err)
	}
	if err := orm.Many[prAuthor](db, b2, "authors").Add(ctx, a1); err != nil {
		t.Fatalf("b2.authors.Add: %v", err)
	}
	if err := orm.Many[prAuthor](db, b3, "authors").Add(ctx, a3); err != nil {
		t.Fatalf("b3.authors.Add: %v", err)
	}
	if err := orm.Many[prAuthor](db, b4, "authors").Add(ctx, a4); err != nil {
		t.Fatalf("b4.authors.Add: %v", err)
	}

	r1 := create(t, db, &prReader{Name: "Amy"})
	r2 := create(t, db, &prReader{Name: "Belinda"})

	if err := orm.Many[prBook](db, r1, "books_read").Add(ctx, b1, b4); err != nil {
		t.Fatalf("r1.books_read.Add: %v", err)
	}
	if err := orm.Many[prBook](db, r2, "books_read").Add(ctx, b2, b4); err != nil {
		t.Fatalf("r2.books_read.Add: %v", err)
	}

	return prFixture{
		book1: b1, book2: b2, book3: b3, book4: b4,
		author1: a1, author2: a2, author3: a3, author4: a4,
		reader1: r1, reader2: r2,
	}
}

// prAuthorNames extracts names from a slice of prAuthor.
func prAuthorNames(rows []prAuthor) []string {
	out := make([]string, len(rows))
	for i, a := range rows {
		out[i] = a.Name
	}
	return out
}

// prBookTitles extracts titles from a slice of prBook.
func prBookTitles(rows []prBook) []string {
	out := make([]string, len(rows))
	for i, b := range rows {
		out[i] = b.Title
	}
	return out
}

// prReaderNames extracts names from a slice of prReader.
func prReaderNames(rows []prReader) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.Name
	}
	return out
}

// Ported from Django: tests/prefetch_related/tests.py::PrefetchRelatedTests.test_m2m_forward
func TestPrefetchRelatedM2MForward(t *testing.T) {
	djangotest.EachDB(t, prModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		prSetUp(t, db)

		var lists [][]string
		djangotest.AssertNumQueries(t, db, 2, func() {
			books := mustAll(t, orm.Query[prBook](db).PrefetchRelated("authors"))
			for i := range books {
				authors, err := orm.Many[prAuthor](db, &books[i], "authors").All(ctx)
				if err != nil {
					t.Fatalf("authors.All: %v", err)
				}
				lists = append(lists, prAuthorNames(authors))
			}
		})

		allBooks := mustAll(t, orm.Query[prBook](db))
		normalLists := make([][]string, 0, len(allBooks))
		for _, b := range allBooks {
			authors, err := orm.Many[prAuthor](db, &b, "authors").All(ctx)
			if err != nil {
				t.Fatalf("normal authors.All: %v", err)
			}
			normalLists = append(normalLists, prAuthorNames(authors))
		}
		djangotest.AssertEqual(t, lists, normalLists)
	})
}

// Ported from Django: tests/prefetch_related/tests.py::PrefetchRelatedTests.test_m2m_reverse
func TestPrefetchRelatedM2MReverse(t *testing.T) {
	djangotest.EachDB(t, prModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		prSetUp(t, db)

		var lists [][]string
		djangotest.AssertNumQueries(t, db, 2, func() {
			authors := mustAll(t, orm.Query[prAuthor](db).PrefetchRelated("books"))
			for i := range authors {
				books, err := orm.Many[prBook](db, &authors[i], "books").All(ctx)
				if err != nil {
					t.Fatalf("books.All: %v", err)
				}
				lists = append(lists, prBookTitles(books))
			}
		})

		allAuthors := mustAll(t, orm.Query[prAuthor](db))
		normalLists := make([][]string, 0, len(allAuthors))
		for _, a := range allAuthors {
			books, err := orm.Many[prBook](db, &a, "books").All(ctx)
			if err != nil {
				t.Fatalf("normal books.All: %v", err)
			}
			normalLists = append(normalLists, prBookTitles(books))
		}
		djangotest.AssertEqual(t, lists, normalLists)
	})
}

// Ported from Django: tests/prefetch_related/tests.py::PrefetchRelatedTests.test_foreignkey_forward
func TestPrefetchRelatedForeignKeyForward(t *testing.T) {
	djangotest.EachDB(t, prModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		fx := prSetUp(t, db)

		var titles []string
		djangotest.AssertNumQueries(t, db, 2, func() {
			authors := mustAll(t, orm.Query[prAuthor](db).PrefetchRelated("first_book"))
			for i := range authors {
				b, err := authors[i].FirstBook.Fetch(ctx, db)
				if err != nil {
					t.Fatalf("FirstBook.Fetch: %v", err)
				}
				titles = append(titles, b.Title)
			}
		})

		allAuthors := mustAll(t, orm.Query[prAuthor](db))
		normalTitles := make([]string, 0, len(allAuthors))
		for _, a := range allAuthors {
			b, err := a.FirstBook.Fetch(ctx, db)
			if err != nil {
				t.Fatalf("normal FirstBook.Fetch: %v", err)
			}
			normalTitles = append(normalTitles, b.Title)
		}
		djangotest.AssertEqual(t, titles, normalTitles)

		// Models without orm.RelCache can still prefetch forward FKs into FK.SetObject.
		create(t, db, &prPlainAuthor{Name: "Plain1", FirstBook: fkTo[prBook](fx.book1.ID)})
		create(t, db, &prPlainAuthor{Name: "Plain2", FirstBook: fkTo[prBook](fx.book2.ID)})
		djangotest.AssertNumQueries(t, db, 2, func() {
			plain := mustAll(t, orm.Query[prPlainAuthor](db).PrefetchRelated("first_book"))
			djangotest.AssertEqual(t, len(plain), 2)
			b0, ok0 := plain[0].FirstBook.Object()
			b1, ok1 := plain[1].FirstBook.Object()
			djangotest.AssertEqual(t, ok0, true)
			djangotest.AssertEqual(t, ok1, true)
			djangotest.AssertEqual(t, b0.Title, "Poems")
			djangotest.AssertEqual(t, b1.Title, "Jane Eyre")
		})
	})
}

// Ported from Django: tests/prefetch_related/tests.py::PrefetchRelatedTests.test_foreignkey_reverse
func TestPrefetchRelatedForeignKeyReverse(t *testing.T) {
	djangotest.EachDB(t, prModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		fx := prSetUp(t, db)

		var got [][]string
		djangotest.AssertNumQueries(t, db, 2, func() {
			books := mustAll(t, orm.Query[prBook](db).PrefetchRelated("first_time_authors"))
			for i := range books {
				authors, err := orm.RelatedSet[prAuthor](db, &books[i], "first_time_authors").All(ctx)
				if err != nil {
					t.Fatalf("first_time_authors.All: %v", err)
				}
				got = append(got, prAuthorNames(authors))
			}
		})
		djangotest.AssertEqual(t, got, [][]string{
			{"Charlotte", "Anne", "Emily"},
			{},
			{},
			{"Jane"},
		})

		authors, err := orm.Many[prAuthor](db, fx.book2, "authors").All(ctx)
		if err != nil {
			t.Fatalf("book2.authors.All: %v", err)
		}
		djangotest.AssertEqual(t, prAuthorNames(authors), []string{"Charlotte"})
	})
}

// Ported from Django: tests/prefetch_related/tests.py::PrefetchRelatedTests.test_onetoone_reverse_no_match
func TestPrefetchRelatedO2OReverseNoMatch(t *testing.T) {
	djangotest.EachDB(t, prModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		prSetUp(t, db)

		var book prBook
		djangotest.AssertNumQueries(t, db, 2, func() {
			books := mustAll(t, orm.Query[prBook](db).PrefetchRelated("bookprofile"))
			book = books[0]
		})
		djangotest.AssertNumQueries(t, db, 0, func() {
			_, err := orm.RelatedOne[prBookProfile](ctx, db, &book, "bookprofile")
			djangotest.AssertErrorIs(t, err, orm.ErrDoesNotExist)
		})
	})
}

// Ported from Django: tests/prefetch_related/tests.py::PrefetchRelatedTests.test_survives_clone
func TestPrefetchRelatedSurvivesClone(t *testing.T) {
	djangotest.EachDB(t, prModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		prSetUp(t, db)

		djangotest.AssertNumQueries(t, db, 2, func() {
			books := mustAll(t, orm.Query[prBook](db).PrefetchRelated("first_time_authors").Exclude("id", 1000))
			for i := range books {
				if _, err := orm.RelatedSet[prAuthor](db, &books[i], "first_time_authors").All(ctx); err != nil {
					t.Fatalf("first_time_authors.All: %v", err)
				}
			}
		})
	})
}

// Ported from Django: tests/prefetch_related/tests.py::PrefetchRelatedTests.test_count
func TestPrefetchRelatedCount(t *testing.T) {
	djangotest.EachDB(t, prModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		prSetUp(t, db)

		djangotest.AssertNumQueries(t, db, 2, func() {
			books := mustAll(t, orm.Query[prBook](db).PrefetchRelated("first_time_authors"))
			counts := make([]int64, len(books))
			for i := range books {
				n, err := orm.RelatedSet[prAuthor](db, &books[i], "first_time_authors").Count(ctx)
				if err != nil {
					t.Fatalf("Count: %v", err)
				}
				counts[i] = n
			}
			djangotest.AssertEqual(t, counts, []int64{3, 0, 0, 1})
		})

		djangotest.AssertNumQueries(t, db, 2, func() {
			books := mustAll(t, orm.Query[prBook](db).PrefetchRelated("authors"))
			counts := make([]int64, len(books))
			for i := range books {
				n, err := orm.Many[prAuthor](db, &books[i], "authors").Count(ctx)
				if err != nil {
					t.Fatalf("M2M Count: %v", err)
				}
				counts[i] = n
			}
			djangotest.AssertEqual(t, counts, []int64{3, 1, 1, 1})
		})
	})
}

// Ported from Django: tests/prefetch_related/tests.py::PrefetchRelatedTests.test_exists
func TestPrefetchRelatedExists(t *testing.T) {
	djangotest.EachDB(t, prModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		prSetUp(t, db)

		djangotest.AssertNumQueries(t, db, 2, func() {
			books := mustAll(t, orm.Query[prBook](db).PrefetchRelated("first_time_authors"))
			got := make([]bool, len(books))
			for i := range books {
				ok, err := orm.RelatedSet[prAuthor](db, &books[i], "first_time_authors").Exists(ctx)
				if err != nil {
					t.Fatalf("Exists: %v", err)
				}
				got[i] = ok
			}
			djangotest.AssertEqual(t, got, []bool{true, false, false, true})
		})

		djangotest.AssertNumQueries(t, db, 2, func() {
			books := mustAll(t, orm.Query[prBook](db).PrefetchRelated("authors"))
			for i := range books {
				ok, err := orm.Many[prAuthor](db, &books[i], "authors").Exists(ctx)
				if err != nil {
					t.Fatalf("M2M Exists: %v", err)
				}
				djangotest.AssertEqual(t, ok, true)
			}
		})
	})
}

// Ported from Django: tests/prefetch_related/tests.py::PrefetchRelatedTests.test_clear
func TestPrefetchRelatedClear(t *testing.T) {
	djangotest.EachDB(t, prModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		prSetUp(t, db)

		djangotest.AssertNumQueries(t, db, 5, func() {
			withPrefetch := orm.Query[prAuthor](db).PrefetchRelated("books")
			withoutPrefetch := withPrefetch.PrefetchRelated(nil)
			for _, a := range mustAll(t, withoutPrefetch) {
				if _, err := orm.Many[prBook](db, &a, "books").All(ctx); err != nil {
					t.Fatalf("books.All: %v", err)
				}
			}
		})

		// Calling PrefetchRelated() with no arguments also clears prefetches.
		djangotest.AssertNumQueries(t, db, 5, func() {
			withoutPrefetch := orm.Query[prAuthor](db).PrefetchRelated("books").PrefetchRelated()
			for _, a := range mustAll(t, withoutPrefetch) {
				if _, err := orm.Many[prBook](db, &a, "books").All(ctx); err != nil {
					t.Fatalf("books.All: %v", err)
				}
			}
		})
	})
}

// Ported from Django: tests/prefetch_related/tests.py::PrefetchRelatedTests.test_m2m_then_m2m
func TestPrefetchRelatedM2MThenM2M(t *testing.T) {
	djangotest.EachDB(t, prModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		prSetUp(t, db)

		djangotest.AssertNumQueries(t, db, 3, func() {
			authors := mustAll(t, orm.Query[prAuthor](db).PrefetchRelated("books__read_by"))
			lists := make([][][]string, 0, len(authors))
			for i := range authors {
				books, err := orm.Many[prBook](db, &authors[i], "books").All(ctx)
				if err != nil {
					t.Fatalf("books.All: %v", err)
				}
				byBook := make([][]string, 0, len(books))
				for j := range books {
					readers, err := orm.Many[prReader](db, &books[j], "read_by").All(ctx)
					if err != nil {
						t.Fatalf("read_by.All: %v", err)
					}
					byBook = append(byBook, prReaderNames(readers))
				}
				lists = append(lists, byBook)
			}
			djangotest.AssertEqual(t, lists, [][][]string{
				{{"Amy"}, {"Belinda"}},
				{{"Amy"}},
				{{"Amy"}, {}},
				{{"Amy", "Belinda"}},
			})
		})
	})
}

// Ported from Django: tests/prefetch_related/tests.py::PrefetchRelatedTests.test_overriding_prefetch
func TestPrefetchRelatedOverridingPrefetch(t *testing.T) {
	djangotest.EachDB(t, prModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		prSetUp(t, db)
		want := [][][]string{
			{{"Amy"}, {"Belinda"}},
			{{"Amy"}},
			{{"Amy"}, {}},
			{{"Amy", "Belinda"}},
		}
		collect := func(qs *orm.QuerySet[prAuthor]) [][][]string {
			authors := mustAll(t, qs)
			lists := make([][][]string, 0, len(authors))
			for i := range authors {
				books, err := orm.Many[prBook](db, &authors[i], "books").All(ctx)
				if err != nil {
					t.Fatalf("books.All: %v", err)
				}
				byBook := make([][]string, 0, len(books))
				for j := range books {
					readers, err := orm.Many[prReader](db, &books[j], "read_by").All(ctx)
					if err != nil {
						t.Fatalf("read_by.All: %v", err)
					}
					byBook = append(byBook, prReaderNames(readers))
				}
				lists = append(lists, byBook)
			}
			return lists
		}

		djangotest.AssertNumQueries(t, db, 3, func() {
			got := collect(orm.Query[prAuthor](db).PrefetchRelated("books", "books__read_by"))
			djangotest.AssertEqual(t, got, want)
		})
		djangotest.AssertNumQueries(t, db, 3, func() {
			got := collect(orm.Query[prAuthor](db).PrefetchRelated("books__read_by", "books"))
			djangotest.AssertEqual(t, got, want)
		})
	})
}

// Ported from Django: tests/prefetch_related/tests.py::PrefetchRelatedTests.test_get
func TestPrefetchRelatedGet(t *testing.T) {
	djangotest.EachDB(t, prModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		prSetUp(t, db)

		djangotest.AssertNumQueries(t, db, 3, func() {
			author, err := orm.Query[prAuthor](db).PrefetchRelated("books__read_by").Get(ctx, "name", "Charlotte")
			if err != nil {
				t.Fatalf("Get: %v", err)
			}
			books, err := orm.Many[prBook](db, &author, "books").All(ctx)
			if err != nil {
				t.Fatalf("books.All: %v", err)
			}
			lists := make([][]string, 0, len(books))
			for i := range books {
				readers, err := orm.Many[prReader](db, &books[i], "read_by").All(ctx)
				if err != nil {
					t.Fatalf("read_by.All: %v", err)
				}
				lists = append(lists, prReaderNames(readers))
			}
			djangotest.AssertEqual(t, lists, [][]string{{"Amy"}, {"Belinda"}})
		})
	})
}

// Ported from Django: tests/prefetch_related/tests.py::PrefetchRelatedTests.test_foreign_key_then_m2m
func TestPrefetchRelatedForeignKeyThenM2M(t *testing.T) {
	djangotest.EachDB(t, prModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		prSetUp(t, db)

		djangotest.AssertNumQueries(t, db, 2, func() {
			authors := mustAll(t, orm.Query[prAuthor](db).SelectRelated("first_book").PrefetchRelated("first_book__read_by"))
			lists := make([][]string, 0, len(authors))
			for i := range authors {
				fb, err := authors[i].FirstBook.Fetch(ctx, db)
				if err != nil {
					t.Fatalf("FirstBook.Fetch: %v", err)
				}
				readers, err := orm.Many[prReader](db, fb, "read_by").All(ctx)
				if err != nil {
					t.Fatalf("read_by.All: %v", err)
				}
				lists = append(lists, prReaderNames(readers))
			}
			djangotest.AssertEqual(t, lists, [][]string{{"Amy"}, {"Amy"}, {"Amy"}, {"Amy", "Belinda"}})
		})
	})
}

// Ported from Django: tests/prefetch_related/tests.py::PrefetchRelatedTests.test_reverse_one_to_one_then_m2m
func TestPrefetchRelatedReverseO2OThenM2M(t *testing.T) {
	djangotest.EachDB(t, prModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		fx := prSetUp(t, db)

		qs := orm.Query[prAuthor](db).PrefetchRelated("bio__books").SelectRelated("bio")
		djangotest.AssertNumQueries(t, db, 1, func() {
			mustAll(t, qs)
		})

		bio := create(t, db, &prBio{Author: o2oTo[prAuthor](fx.author1.ID)})
		if err := orm.Many[prBook](db, bio, "books").Add(ctx, fx.book1); err != nil {
			t.Fatalf("bio.books.Add: %v", err)
		}
		djangotest.AssertNumQueries(t, db, 2, func() {
			authors := mustAll(t, qs)
			gotBio, err := orm.RelatedOne[prBio](ctx, db, &authors[0], "bio")
			if err != nil {
				t.Fatalf("RelatedOne bio: %v", err)
			}
			books, err := orm.Many[prBook](db, gotBio, "books").All(ctx)
			if err != nil {
				t.Fatalf("bio.books.All: %v", err)
			}
			djangotest.AssertEqual(t, prBookTitles(books), []string{"Poems"})
		})
	})
}

// Ported from Django: tests/prefetch_related/tests.py::PrefetchRelatedTests.test_attribute_error
func TestPrefetchRelatedAttributeError(t *testing.T) {
	djangotest.EachDB(t, prModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		prSetUp(t, db)

		_, err := orm.Query[prReader](db).PrefetchRelated("books_read__xyz").All(ctx)
		assertErrorContains(t, err, "Cannot find 'xyz' on prBook object, 'books_read__xyz' is an invalid parameter to prefetch_related()")
	})
}

// Ported from Django: tests/prefetch_related/tests.py::PrefetchRelatedTests.test_invalid_final_lookup
func TestPrefetchRelatedInvalidFinalLookup(t *testing.T) {
	djangotest.EachDB(t, prModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		prSetUp(t, db)

		_, err := orm.Query[prBook](db).PrefetchRelated("authors__name").All(ctx)
		assertErrorContains(t, err, "'authors__name' does not resolve to an item that supports prefetching")
		assertErrorContains(t, err, "prefetch_related")
		assertErrorContains(t, err, "name")
	})
}

// Ported from Django: tests/prefetch_related/tests.py::PrefetchRelatedTests.test_forward_m2m_to_attr_conflict
func TestPrefetchRelatedForwardM2MToAttrConflict(t *testing.T) {
	djangotest.EachDB(t, prModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		fx := prSetUp(t, db)

		_, err := orm.Query[prBook](db).PrefetchRelated(orm.Prefetch{
			Lookup:   "authors",
			QuerySet: orm.Query[prAuthor](db),
			ToAttr:   "authors",
		}).All(ctx)
		assertErrorContains(t, err, "to_attr=authors conflicts with a field on the prBook model")
		cnt, err := orm.Many[prAuthor](db, fx.book1, "authors").Count(ctx)
		if err != nil {
			t.Fatalf("Count: %v", err)
		}
		djangotest.AssertEqual(t, cnt, int64(3))
	})
}

// Ported from Django: tests/prefetch_related/tests.py::PrefetchRelatedTests.test_reverse_m2m_to_attr_conflict
func TestPrefetchRelatedReverseM2MToAttrConflict(t *testing.T) {
	djangotest.EachDB(t, prModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		fx := prSetUp(t, db)

		_, err := orm.Query[prAuthor](db).PrefetchRelated(orm.Prefetch{
			Lookup:   "books",
			QuerySet: orm.Query[prBook](db).Filter("title", "Poems"),
			ToAttr:   "books",
		}).All(ctx)
		assertErrorContains(t, err, "to_attr=books conflicts with a field on the prAuthor model")
		cnt, err := orm.Many[prBook](db, fx.author1, "books").Count(ctx)
		if err != nil {
			t.Fatalf("Count: %v", err)
		}
		djangotest.AssertEqual(t, cnt, int64(2))
	})
}

// Ported from Django: tests/prefetch_related/tests.py::PrefetchRelatedTests.test_m2m_join_reuse
func TestPrefetchRelatedM2MJoinReuse(t *testing.T) {
	djangotest.EachDB(t, prModels(), func(t *testing.T, db *orm.DB) {
		fx := prSetUp(t, db)
		create(t, db, &prFavoriteAuthors{Author: fkTo[prAuthor](fx.author1.ID), LikesAuthor: fkTo[prAuthor](fx.author3.ID), IsActive: true})
		create(t, db, &prFavoriteAuthors{Author: fkTo[prAuthor](fx.author1.ID), LikesAuthor: fkTo[prAuthor](fx.author4.ID), IsActive: false})
		create(t, db, &prFavoriteAuthors{Author: fkTo[prAuthor](fx.author2.ID), LikesAuthor: fkTo[prAuthor](fx.author3.ID), IsActive: true})
		create(t, db, &prFavoriteAuthors{Author: fkTo[prAuthor](fx.author2.ID), LikesAuthor: fkTo[prAuthor](fx.author4.ID), IsActive: true})

		var authors []prAuthor
		djangotest.AssertNumQueries(t, db, 2, func() {
			authors = mustAll(t, orm.Query[prAuthor](db).
				Filter("pk__in", []int64{fx.author1.ID, fx.author2.ID}).
				PrefetchRelated(orm.Prefetch{
					Lookup: "favorite_authors",
					QuerySet: orm.Query[prAuthor](db).
						Annotate("active_favorite", orm.F("likes_me__is_active")).
						Filter("active_favorite", true),
					ToAttr: "active_favorite_authors",
				}))
		})
		fav0, ok0 := orm.Prefetched[prAuthor](&authors[0], "active_favorite_authors")
		fav1, ok1 := orm.Prefetched[prAuthor](&authors[1], "active_favorite_authors")
		djangotest.AssertEqual(t, ok0, true)
		djangotest.AssertEqual(t, ok1, true)
		djangotest.AssertEqual(t, prAuthorNames(fav0), []string{"Emily"})
		djangotest.AssertEqual(t, prAuthorNames(fav1), []string{"Emily", "Jane"})
	})
}

// Ported from Django: tests/prefetch_related/tests.py::PrefetchRelatedTests.test_named_values_list
func TestPrefetchRelatedValuesList(t *testing.T) {
	djangotest.EachDB(t, prModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		prSetUp(t, db)

		qs := orm.Query[prAuthor](db).PrefetchRelated("books")
		names, err := qs.ValuesList("name").Flat(ctx)
		if err != nil {
			t.Fatalf("ValuesList.Flat: %v", err)
		}
		djangotest.AssertEqual(t, names, []any{"Charlotte", "Anne", "Emily", "Jane"})
	})
}

// Ported from Django: tests/prefetch_related/tests.py::RawQuerySetTests.test_basic
func TestPrefetchRelatedRawBasic(t *testing.T) {
	djangotest.EachDB(t, prModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		fx := prSetUp(t, db)

		var book1 prBook
		djangotest.AssertNumQueries(t, db, 2, func() {
			books, err := orm.Raw[prBook](db, "SELECT * FROM prefetch_related_prbook WHERE id = ?", fx.book1.ID).
				PrefetchRelated("authors").All(ctx)
			if err != nil {
				t.Fatalf("Raw.All: %v", err)
			}
			book1 = books[0]
		})
		djangotest.AssertNumQueries(t, db, 0, func() {
			authors, err := orm.Many[prAuthor](db, &book1, "authors").All(ctx)
			if err != nil {
				t.Fatalf("authors.All: %v", err)
			}
			djangotest.AssertEqual(t, prAuthorNames(authors), []string{"Charlotte", "Anne", "Emily"})
		})
	})
}

// Ported from Django: tests/prefetch_related/tests.py::RawQuerySetTests.test_clear
func TestPrefetchRelatedRawClear(t *testing.T) {
	djangotest.EachDB(t, prModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		prSetUp(t, db)

		djangotest.AssertNumQueries(t, db, 5, func() {
			withPrefetch := orm.Raw[prAuthor](db, "SELECT * FROM prefetch_related_prauthor").PrefetchRelated("books")
			withoutPrefetch := withPrefetch.PrefetchRelated(nil)
			authors, err := withoutPrefetch.All(ctx)
			if err != nil {
				t.Fatalf("Raw.All: %v", err)
			}
			for i := range authors {
				if _, err := orm.Many[prBook](db, &authors[i], "books").All(ctx); err != nil {
					t.Fatalf("books.All: %v", err)
				}
			}
		})
	})
}

// Ported from Django: tests/prefetch_related/tests.py::NullableTest.test_traverse_nullable
func TestPrefetchRelatedTraverseNullable(t *testing.T) {
	djangotest.EachDB(t, prModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		boss := create(t, db, &prEmployee{Name: "Peter"})
		create(t, db, &prEmployee{Name: "Joe", Boss: fkTo[prEmployee](boss.ID)})
		create(t, db, &prEmployee{Name: "Angela", Boss: fkTo[prEmployee](boss.ID)})

		var coSerfs [][]string
		djangotest.AssertNumQueries(t, db, 2, func() {
			emps := mustAll(t, orm.Query[prEmployee](db).SelectRelated("boss").PrefetchRelated("boss__serfs"))
			for i := range emps {
				b, loaded := emps[i].Boss.Object()
				if !loaded || b == nil {
					coSerfs = append(coSerfs, nil)
					continue
				}
				serfs, err := orm.RelatedSet[prEmployee](db, b, "serfs").All(ctx)
				if err != nil {
					t.Fatalf("serfs.All: %v", err)
				}
				names := make([]string, len(serfs))
				for j, s := range serfs {
					names[j] = s.Name
				}
				coSerfs = append(coSerfs, names)
			}
		})
		djangotest.AssertEqual(t, coSerfs, [][]string{nil, {"Joe", "Angela"}, {"Joe", "Angela"}})
	})
}

// Ported from Django: tests/prefetch_related/tests.py::NullableTest.test_prefetch_nullable
func TestPrefetchRelatedPrefetchNullable(t *testing.T) {
	djangotest.EachDB(t, prModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		boss := create(t, db, &prEmployee{Name: "Peter"})
		create(t, db, &prEmployee{Name: "Joe", Boss: fkTo[prEmployee](boss.ID)})
		create(t, db, &prEmployee{Name: "Angela", Boss: fkTo[prEmployee](boss.ID)})

		var coSerfs [][]string
		djangotest.AssertNumQueries(t, db, 3, func() {
			emps := mustAll(t, orm.Query[prEmployee](db).PrefetchRelated("boss__serfs"))
			for i := range emps {
				if emps[i].Boss.PK() == 0 {
					coSerfs = append(coSerfs, nil)
					continue
				}
				b, err := emps[i].Boss.Fetch(ctx, db)
				if err != nil {
					t.Fatalf("Boss.Fetch: %v", err)
				}
				serfs, err := orm.RelatedSet[prEmployee](db, b, "serfs").All(ctx)
				if err != nil {
					t.Fatalf("serfs.All: %v", err)
				}
				names := make([]string, len(serfs))
				for j, s := range serfs {
					names[j] = s.Name
				}
				coSerfs = append(coSerfs, names)
			}
		})
		djangotest.AssertEqual(t, coSerfs, [][]string{nil, {"Joe", "Angela"}, {"Joe", "Angela"}})
	})
}

// Ported from Django: tests/prefetch_related/tests.py::Ticket19607Tests.test_bug
func TestPrefetchRelatedTicket19607(t *testing.T) {
	djangotest.EachDB(t, prModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		l1 := create(t, db, &prLessonEntry{Name1: "einfach", Name2: "simple"})
		l2 := create(t, db, &prLessonEntry{Name1: "schwierig", Name2: "difficult"})
		create(t, db, &prWordEntry{LessonEntry: fkTo[prLessonEntry](l1.ID), Name: "einfach"})
		create(t, db, &prWordEntry{LessonEntry: fkTo[prLessonEntry](l1.ID), Name: "simple"})
		create(t, db, &prWordEntry{LessonEntry: fkTo[prLessonEntry](l2.ID), Name: "schwierig"})
		create(t, db, &prWordEntry{LessonEntry: fkTo[prLessonEntry](l2.ID), Name: "difficult"})

		djangotest.AssertNumQueries(t, db, 3, func() {
			words := mustAll(t, orm.Query[prWordEntry](db).PrefetchRelated("lesson_entry", "lesson_entry__wordentry_set"))
			djangotest.AssertEqual(t, len(words), 4)
			le, err := words[0].LessonEntry.Fetch(ctx, db)
			if err != nil {
				t.Fatalf("LessonEntry.Fetch: %v", err)
			}
			peers, err := orm.RelatedSet[prWordEntry](db, le, "wordentry_set").All(ctx)
			if err != nil {
				t.Fatalf("wordentry_set.All: %v", err)
			}
			djangotest.AssertEqual(t, len(peers), 2)
		})
	})
}

// Ported from Django: tests/prefetch_related/tests.py::Ticket21410Tests.test_bug
func TestPrefetchRelatedTicket21410(t *testing.T) {
	djangotest.EachDB(t, prModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		b1 := create(t, db, &prBook{Title: "Poems"})
		b2 := create(t, db, &prBook{Title: "Jane Eyre"})
		b3 := create(t, db, &prBook{Title: "Wuthering Heights"})
		a1 := create(t, db, &prAuthor2{Name: "Charlotte", FirstBook: fkTo[prBook](b1.ID)})
		a2 := create(t, db, &prAuthor2{Name: "Anne", FirstBook: fkTo[prBook](b1.ID)})
		if err := orm.Many[prBook](db, a1, "favorite_books").Add(ctx, b1, b2, b3); err != nil {
			t.Fatalf("a1.favorite_books.Add: %v", err)
		}
		if err := orm.Many[prBook](db, a2, "favorite_books").Add(ctx, b1); err != nil {
			t.Fatalf("a2.favorite_books.Add: %v", err)
		}

		djangotest.AssertNumQueries(t, db, 3, func() {
			authors := mustAll(t, orm.Query[prAuthor2](db).PrefetchRelated("first_book", "favorite_books"))
			djangotest.AssertEqual(t, len(authors), 2)
			fb, err := authors[0].FirstBook.Fetch(ctx, db)
			if err != nil {
				t.Fatalf("FirstBook.Fetch: %v", err)
			}
			djangotest.AssertEqual(t, fb.Title, "Poems")
			favs, err := orm.Many[prBook](db, &authors[0], "favorite_books").All(ctx)
			if err != nil {
				t.Fatalf("favorite_books.All: %v", err)
			}
			djangotest.AssertEqual(t, len(favs), 3)
		})
	})
}

// Ported from Django: tests/prefetch_related/tests.py::DirectPrefetchedObjectCacheReuseTests.test_prefetch_reverse_foreign_key
// Ported from Django: tests/prefetch_related/tests.py::DirectPrefetchedObjectCacheReuseTests.test_add_clears_prefetched_objects
// Ported from Django: tests/prefetch_related/tests.py::DirectPrefetchedObjectCacheReuseTests.test_remove_clears_prefetched_objects
func TestPrefetchRelatedObjectsAndCacheInvalidation(t *testing.T) {
	djangotest.EachDB(t, prModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		fx := prSetUp(t, db)
		rev1 := create(t, db, &prBookReview{Book: fkTo[prBook](fx.book1.ID), Notes: "great"})

		var b1 prBook
		djangotest.AssertNumQueries(t, db, 2, func() {
			var err error
			b1, err = orm.Query[prBook](db).PrefetchRelated("prbookreview_set").Get(ctx, "pk", fx.book1.ID)
			if err != nil {
				t.Fatalf("Get: %v", err)
			}
		})
		djangotest.AssertNumQueries(t, db, 0, func() {
			revs, err := orm.RelatedSet[prBookReview](db, &b1, "prbookreview_set").All(ctx)
			if err != nil {
				t.Fatalf("All: %v", err)
			}
			djangotest.AssertEqual(t, len(revs), 1)
			djangotest.AssertEqual(t, revs[0].ID, rev1.ID)
		})
		// Calling PrefetchRelatedObjects on an already-prefetched instance runs 0 queries.
		djangotest.AssertNumQueries(t, db, 0, func() {
			if err := orm.PrefetchRelatedObjects(ctx, db, []*prBook{&b1}, "prbookreview_set"); err != nil {
				t.Fatalf("PrefetchRelatedObjects: %v", err)
			}
		})

		// Filter bypasses the prefetch cache and queries the DB.
		djangotest.AssertNumQueries(t, db, 1, func() {
			filtered, err := orm.RelatedSet[prBookReview](db, &b1, "prbookreview_set").Filter("notes", "great").All(ctx)
			if err != nil {
				t.Fatalf("Filter.All: %v", err)
			}
			djangotest.AssertEqual(t, len(filtered), 1)
		})

		// Add clears the prefetch cache on RelatedManager.
		rev2 := create(t, db, &prBookReview{Notes: "second"})
		if err := orm.RelatedSet[prBookReview](db, &b1, "prbookreview_set").Add(ctx, rev2); err != nil {
			t.Fatalf("Add: %v", err)
		}
		djangotest.AssertNumQueries(t, db, 1, func() {
			revs, err := orm.RelatedSet[prBookReview](db, &b1, "prbookreview_set").All(ctx)
			if err != nil {
				t.Fatalf("All after Add: %v", err)
			}
			djangotest.AssertEqual(t, len(revs), 2)
		})

		// Remove clears the prefetch cache on RelatedManager.
		if err := orm.PrefetchRelatedObjects(ctx, db, []*prBook{&b1}, "prbookreview_set"); err != nil {
			t.Fatalf("PrefetchRelatedObjects: %v", err)
		}
		if err := orm.RelatedSet[prBookReview](db, &b1, "prbookreview_set").Remove(ctx, rev1); err != nil {
			t.Fatalf("Remove: %v", err)
		}
		djangotest.AssertNumQueries(t, db, 1, func() {
			revs, err := orm.RelatedSet[prBookReview](db, &b1, "prbookreview_set").All(ctx)
			if err != nil {
				t.Fatalf("All after Remove: %v", err)
			}
			djangotest.AssertEqual(t, len(revs), 1)
			djangotest.AssertEqual(t, revs[0].ID, rev2.ID)
		})

		// Clear and Set clear the prefetch cache on RelatedManager.
		if err := orm.PrefetchRelatedObjects(ctx, db, []*prBook{&b1}, "prbookreview_set"); err != nil {
			t.Fatalf("PrefetchRelatedObjects: %v", err)
		}
		if err := orm.RelatedSet[prBookReview](db, &b1, "prbookreview_set").Clear(ctx); err != nil {
			t.Fatalf("Clear: %v", err)
		}
		djangotest.AssertNumQueries(t, db, 1, func() {
			revs, err := orm.RelatedSet[prBookReview](db, &b1, "prbookreview_set").All(ctx)
			if err != nil {
				t.Fatalf("All after Clear: %v", err)
			}
			djangotest.AssertEqual(t, len(revs), 0)
		})

		if err := orm.PrefetchRelatedObjects(ctx, db, []*prBook{&b1}, "prbookreview_set"); err != nil {
			t.Fatalf("PrefetchRelatedObjects: %v", err)
		}
		if err := orm.RelatedSet[prBookReview](db, &b1, "prbookreview_set").Set(ctx, []*prBookReview{rev1}); err != nil {
			t.Fatalf("Set: %v", err)
		}
		djangotest.AssertNumQueries(t, db, 1, func() {
			revs, err := orm.RelatedSet[prBookReview](db, &b1, "prbookreview_set").All(ctx)
			if err != nil {
				t.Fatalf("All after Set: %v", err)
			}
			djangotest.AssertEqual(t, len(revs), 1)
		})

		// ManyManager Filter bypasses cache; Add, Remove, Clear, Set clear cache.
		book, err := orm.Query[prBook](db).PrefetchRelated("authors").Get(ctx, "pk", fx.book2.ID)
		if err != nil {
			t.Fatalf("Get book2: %v", err)
		}
		djangotest.AssertNumQueries(t, db, 0, func() {
			got, err := orm.Many[prAuthor](db, &book, "authors").All(ctx)
			if err != nil {
				t.Fatalf("M2M All: %v", err)
			}
			djangotest.AssertEqual(t, prAuthorNames(got), []string{"Charlotte"})
		})
		djangotest.AssertNumQueries(t, db, 1, func() {
			got, err := orm.Many[prAuthor](db, &book, "authors").Filter("name", "Charlotte").All(ctx)
			if err != nil {
				t.Fatalf("M2M Filter: %v", err)
			}
			djangotest.AssertEqual(t, len(got), 1)
		})
		if err := orm.Many[prAuthor](db, &book, "authors").Add(ctx, fx.author2); err != nil {
			t.Fatalf("M2M Add: %v", err)
		}
		djangotest.AssertNumQueries(t, db, 1, func() {
			got, err := orm.Many[prAuthor](db, &book, "authors").All(ctx)
			if err != nil {
				t.Fatalf("M2M All after Add: %v", err)
			}
			djangotest.AssertEqual(t, prAuthorNames(got), []string{"Charlotte", "Anne"})
		})
		if err := orm.PrefetchRelatedObjects(ctx, db, []*prBook{&book}, "authors"); err != nil {
			t.Fatalf("PrefetchRelatedObjects authors: %v", err)
		}
		if err := orm.Many[prAuthor](db, &book, "authors").Remove(ctx, fx.author2); err != nil {
			t.Fatalf("M2M Remove: %v", err)
		}
		djangotest.AssertNumQueries(t, db, 1, func() {
			got, err := orm.Many[prAuthor](db, &book, "authors").All(ctx)
			if err != nil {
				t.Fatalf("M2M All after Remove: %v", err)
			}
			djangotest.AssertEqual(t, prAuthorNames(got), []string{"Charlotte"})
		})
		if err := orm.PrefetchRelatedObjects(ctx, db, []*prBook{&book}, "authors"); err != nil {
			t.Fatalf("PrefetchRelatedObjects authors: %v", err)
		}
		if err := orm.Many[prAuthor](db, &book, "authors").Clear(ctx); err != nil {
			t.Fatalf("M2M Clear: %v", err)
		}
		djangotest.AssertNumQueries(t, db, 1, func() {
			got, err := orm.Many[prAuthor](db, &book, "authors").All(ctx)
			if err != nil {
				t.Fatalf("M2M All after Clear: %v", err)
			}
			djangotest.AssertEqual(t, len(got), 0)
		})
		if err := orm.PrefetchRelatedObjects(ctx, db, []*prBook{&book}, "authors"); err != nil {
			t.Fatalf("PrefetchRelatedObjects authors: %v", err)
		}
		if err := orm.Many[prAuthor](db, &book, "authors").Set(ctx, []*prAuthor{fx.author1}); err != nil {
			t.Fatalf("M2M Set: %v", err)
		}
		djangotest.AssertNumQueries(t, db, 1, func() {
			got, err := orm.Many[prAuthor](db, &book, "authors").All(ctx)
			if err != nil {
				t.Fatalf("M2M All after Set: %v", err)
			}
			djangotest.AssertEqual(t, prAuthorNames(got), []string{"Charlotte"})
		})
	})
}

// TestPrefetchRelatedSQLShape checks that reverse FK, forward FK, and M2M
// prefetch queries issue a single batched IN query without extraneous joins.
func TestPrefetchRelatedSQLShape(t *testing.T) {
	djangotest.EachDB(t, prModels(), func(t *testing.T, db *orm.DB) {
		prSetUp(t, db)

		capRevFK := djangotest.CaptureQueries(db)
		mustAll(t, orm.Query[prBook](db).PrefetchRelated("first_time_authors"))
		capRevFK.Stop()
		djangotest.AssertEqual(t, len(capRevFK.Queries()), 2)
		revSQL := capRevFK.Queries()[1].SQL
		if !strings.Contains(revSQL, " IN (") || strings.Contains(revSQL, " JOIN ") {
			t.Errorf("reverse FK prefetch SQL = %q, want batched IN without JOIN", revSQL)
		}

		capFwdFK := djangotest.CaptureQueries(db)
		mustAll(t, orm.Query[prAuthor](db).PrefetchRelated("first_book"))
		capFwdFK.Stop()
		djangotest.AssertEqual(t, len(capFwdFK.Queries()), 2)
		fwdSQL := capFwdFK.Queries()[1].SQL
		if !strings.Contains(fwdSQL, " IN (") || strings.Contains(fwdSQL, " JOIN ") {
			t.Errorf("forward FK prefetch SQL = %q, want batched IN without JOIN", fwdSQL)
		}

		capM2M := djangotest.CaptureQueries(db)
		mustAll(t, orm.Query[prBook](db).PrefetchRelated("authors"))
		capM2M.Stop()
		djangotest.AssertEqual(t, len(capM2M.Queries()), 2)
		m2mSQL := capM2M.Queries()[1].SQL
		if !strings.Contains(m2mSQL, "INNER JOIN") || !strings.Contains(m2mSQL, " IN (") || !strings.Contains(m2mSQL, "_prefetch_related_val_") {
			t.Errorf("M2M prefetch SQL = %q, want INNER JOIN + _prefetch_related_val_ + batched IN", m2mSQL)
		}

		capRevO2O := djangotest.CaptureQueries(db)
		mustAll(t, orm.Query[prBook](db).PrefetchRelated("bookprofile"))
		capRevO2O.Stop()
		djangotest.AssertEqual(t, len(capRevO2O.Queries()), 2)
		o2oSQL := capRevO2O.Queries()[1].SQL
		if !strings.Contains(o2oSQL, " IN (") || strings.Contains(o2oSQL, " JOIN ") {
			t.Errorf("reverse O2O prefetch SQL = %q, want batched IN without JOIN", o2oSQL)
		}
	})
}

// TestPrefetchRelatedChainedPathsAndUUIDPK checks chained paths across
// reverse FK -> forward FK, forward FK -> reverse O2O, and UUID-PK M2M.
func TestPrefetchRelatedChainedPathsAndUUIDPK(t *testing.T) {
	djangotest.EachDB(t, prModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		fx := prSetUp(t, db)
		create(t, db, &prBio{Author: o2oTo[prAuthor](fx.author1.ID)})
		create(t, db, &prBookProfile{Book: o2oTo[prBook](fx.book1.ID), Note: "classic"})
		create(t, db, &prAuthorAddress{Author: fkTo[prAuthor](fx.author1.ID), Address: "Haworth"})

		// Chained forward FK -> reverse O2O ("first_book__bookprofile")
		djangotest.AssertNumQueries(t, db, 3, func() {
			authors := mustAll(t, orm.Query[prAuthor](db).PrefetchRelated("first_book__bookprofile"))
			fb, err := authors[0].FirstBook.Fetch(ctx, db)
			if err != nil {
				t.Fatalf("FirstBook.Fetch: %v", err)
			}
			prof, err := orm.RelatedOne[prBookProfile](ctx, db, fb, "bookprofile")
			if err != nil {
				t.Fatalf("RelatedOne bookprofile: %v", err)
			}
			djangotest.AssertEqual(t, prof.Note, "classic")
		})

		// Chained reverse FK -> forward FK ("first_time_authors__first_book")
		// and reverse FK -> reverse O2O ("first_time_authors__bio")
		djangotest.AssertNumQueries(t, db, 3, func() {
			books := mustAll(t, orm.Query[prBook](db).PrefetchRelated("first_time_authors__bio"))
			authors, err := orm.RelatedSet[prAuthor](db, &books[0], "first_time_authors").All(ctx)
			if err != nil {
				t.Fatalf("first_time_authors.All: %v", err)
			}
			bio, err := orm.RelatedOne[prBio](ctx, db, &authors[0], "bio")
			if err != nil {
				t.Fatalf("RelatedOne bio: %v", err)
			}
			djangotest.AssertEqual(t, bio.Author.PK(), fx.author1.ID)
			_, err = orm.RelatedOne[prBio](ctx, db, &authors[1], "bio")
			djangotest.AssertErrorIs(t, err, orm.ErrDoesNotExist)
		})

		// UUID primary key M2M prefetch
		p1 := create(t, db, &prPerson{Name: "Alice"})
		pet1 := create(t, db, &prPet{ID: orm.NewUUID(), Name: "Fido"})
		pet2 := create(t, db, &prPet{ID: orm.NewUUID(), Name: "Rex"})
		if err := orm.Many[prPerson](db, pet1, "people").Add(ctx, p1); err != nil {
			t.Fatalf("pet1.people.Add: %v", err)
		}
		if err := orm.Many[prPerson](db, pet2, "people").Add(ctx, p1); err != nil {
			t.Fatalf("pet2.people.Add: %v", err)
		}
		djangotest.AssertNumQueries(t, db, 2, func() {
			pets := mustAll(t, orm.Query[prPet](db).PrefetchRelated("people"))
			djangotest.AssertEqual(t, len(pets), 2)
			for i := range pets {
				people, err := orm.Many[prPerson](db, &pets[i], "people").All(ctx)
				if err != nil {
					t.Fatalf("pet.people.All: %v", err)
				}
				djangotest.AssertEqual(t, len(people), 1)
				djangotest.AssertEqual(t, people[0].Name, "Alice")
			}
		})
	})
}
