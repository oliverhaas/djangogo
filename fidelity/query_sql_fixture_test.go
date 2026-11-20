package fidelity

import (
	"context"
	"errors"
	"sort"
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
)

// country mirrors Country in fidelity/oracle/query_sql.py (table fid_country).
type country struct {
	ID   int64  `orm:"pk"`
	Name string `orm:"max_length=100"`
	Code string `orm:"max_length=10"`
}

// publisher mirrors Publisher in fidelity/oracle/query_sql.py (table fid_publisher).
type publisher struct {
	ID      int64           `orm:"pk"`
	Name    string          `orm:"max_length=100"`
	Country orm.FK[country] `orm:"null;on_delete=set_null"`
}

// author mirrors Author in fidelity/oracle/query_sql.py (table fid_author).
type author struct {
	ID        int64  `orm:"pk"`
	Name      string `orm:"max_length=100"`
	Age       int64
	Active    bool
	Publisher orm.FK[publisher] `orm:"null;on_delete=set_null"`
}

// book mirrors Book in fidelity/oracle/query_sql.py (table fid_book).
type book struct {
	ID        int64  `orm:"pk"`
	Title     string `orm:"max_length=200"`
	Pages     int64
	Published bool
	CreatedAt time.Time
	Author    orm.FK[author] `orm:"on_delete=cascade"`
	Editor    orm.FK[author] `orm:"null;on_delete=set_null"`
}

// queryFixtureModels returns the model list registered for the query SQL harness.
func queryFixtureModels() []any {
	return []any{
		djangotest.App("fid", &country{}, &publisher{}, &author{}, &book{}),
	}
}

// fkID constructs an orm.FK[T] holding pk.
func fkID[T any](pk int64) orm.FK[T] {
	var f orm.FK[T]
	f.SetPK(pk)
	return f
}

// utcTime constructs a UTC time.Time for fixture and query timestamps.
func utcTime(year int, month time.Month, day, hour, minute, sec int) time.Time {
	return time.Date(year, month, day, hour, minute, sec, 0, time.UTC)
}

// seedQueryFixture populates db with the canonical rows matching query_sql.py.
func seedQueryFixture(t *testing.T, db *orm.DB) {
	t.Helper()
	ctx := context.Background()

	countries := []country{
		{ID: 1, Name: "United Kingdom", Code: "UK"},
		{ID: 2, Name: "United States", Code: "US"},
	}
	for i := range countries {
		if err := orm.Query[country](db).Create(ctx, &countries[i]); err != nil {
			t.Fatalf("seed country %d: %v", countries[i].ID, err)
		}
	}

	publishers := []publisher{
		{ID: 1, Name: "Analytic Press", Country: fkID[country](1)},
		{ID: 2, Name: "Compiler House", Country: fkID[country](2)},
		{ID: 3, Name: "Indie Press"},
	}
	for i := range publishers {
		if err := orm.Query[publisher](db).Create(ctx, &publishers[i]); err != nil {
			t.Fatalf("seed publisher %d: %v", publishers[i].ID, err)
		}
	}

	authors := []author{
		{ID: 1, Name: "Ada Lovelace", Age: 36, Active: true, Publisher: fkID[publisher](1)},
		{ID: 2, Name: "Grace Hopper", Age: 85, Active: true, Publisher: fkID[publisher](2)},
		{ID: 3, Name: "Alan Turing", Age: 41, Active: false, Publisher: fkID[publisher](3)},
		{ID: 4, Name: "Barbara Liskov", Age: 84, Active: true},
	}
	for i := range authors {
		if err := orm.Query[author](db).Create(ctx, &authors[i]); err != nil {
			t.Fatalf("seed author %d: %v", authors[i].ID, err)
		}
	}

	books := []book{
		{
			ID: 1, Title: "Notes on the Engine", Pages: 120, Published: true,
			CreatedAt: utcTime(2024, 1, 15, 10, 0, 0),
			Author:    fkID[author](1), Editor: fkID[author](2),
		},
		{
			ID: 2, Title: "Compilers 101", Pages: 340, Published: true,
			CreatedAt: utcTime(2024, 3, 20, 14, 30, 0),
			Author:    fkID[author](2),
		},
		{
			ID: 3, Title: "Computable Numbers", Pages: 210, Published: false,
			CreatedAt: utcTime(2024, 6, 1, 8, 15, 0),
			Author:    fkID[author](3), Editor: fkID[author](1),
		},
		{
			ID: 4, Title: "Engine Design", Pages: 280, Published: true,
			CreatedAt: utcTime(2024, 9, 10, 18, 45, 0),
			Author:    fkID[author](1),
		},
		{
			ID: 5, Title: "Abstraction Patterns", Pages: 180, Published: false,
			CreatedAt: utcTime(2024, 11, 5, 12, 0, 0),
			Author:    fkID[author](4), Editor: fkID[author](2),
		},
	}
	for i := range books {
		if err := orm.Query[book](db).Create(ctx, &books[i]); err != nil {
			t.Fatalf("seed book %d: %v", books[i].ID, err)
		}
	}
}

// queryCaseOutcome holds the captured SQL statement (if any) and the canonical
// semantic result returned by executing one Go ORM case.
type queryCaseOutcome struct {
	queries []djangotest.CapturedQuery
	result  any
}

// queryCase defines one canonical Go ORM query matching a Django case in
// fidelity/oracle/query_sql.py.
type queryCase struct {
	name string
	run  func(t *testing.T, db *orm.DB) queryCaseOutcome
}

// errRollbackFixture rolls back mutating cases (update, delete) so every case
// sees the initial fixture data.
var errRollbackFixture = errors.New("rollback fixture")

// runBookIDs runs a Book queryset, captures its SQL, and returns matching IDs.
func runBookIDs(t *testing.T, db *orm.DB, qs *orm.QuerySet[book], sortIDs bool) queryCaseOutcome {
	t.Helper()
	qc := djangotest.CaptureQueries(db)
	rows, err := qs.All(context.Background())
	qc.Stop()
	if err != nil {
		t.Fatalf("All: %v", err)
	}
	ids := make([]int64, len(rows))
	for i, b := range rows {
		ids[i] = b.ID
	}
	if sortIDs {
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	}
	return queryCaseOutcome{queries: qc.Queries(), result: ids}
}

// runCount captures SQL and returns the count from fn.
func runCount(t *testing.T, db *orm.DB, fn func(context.Context) (int64, error)) queryCaseOutcome {
	t.Helper()
	qc := djangotest.CaptureQueries(db)
	n, err := fn(context.Background())
	qc.Stop()
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	return queryCaseOutcome{queries: qc.Queries(), result: n}
}

// runExists captures SQL and returns the exists boolean from qs.
func runExists(t *testing.T, db *orm.DB, qs *orm.QuerySet[book]) queryCaseOutcome {
	t.Helper()
	qc := djangotest.CaptureQueries(db)
	ok, err := qs.Exists(context.Background())
	qc.Stop()
	if err != nil {
		t.Fatalf("Exists: %v", err)
	}
	return queryCaseOutcome{queries: qc.Queries(), result: ok}
}

// runValues captures SQL and returns the map slice from vs.
func runValues(t *testing.T, db *orm.DB, vs *orm.ValuesQuerySet) queryCaseOutcome {
	t.Helper()
	qc := djangotest.CaptureQueries(db)
	rows, err := vs.All(context.Background())
	qc.Stop()
	if err != nil {
		t.Fatalf("Values.All: %v", err)
	}
	return queryCaseOutcome{queries: qc.Queries(), result: rows}
}

// runValuesList captures SQL and returns the row slices from vl.
func runValuesList(t *testing.T, db *orm.DB, vl *orm.ValuesListQuerySet) queryCaseOutcome {
	t.Helper()
	qc := djangotest.CaptureQueries(db)
	rows, err := vl.All(context.Background())
	qc.Stop()
	if err != nil {
		t.Fatalf("ValuesList.All: %v", err)
	}
	return queryCaseOutcome{queries: qc.Queries(), result: rows}
}

// runValuesListFlat captures SQL and returns the flat slice from vl.
func runValuesListFlat(t *testing.T, db *orm.DB, vl *orm.ValuesListQuerySet) queryCaseOutcome {
	t.Helper()
	qc := djangotest.CaptureQueries(db)
	rows, err := vl.Flat(context.Background())
	qc.Stop()
	if err != nil {
		t.Fatalf("ValuesList.Flat: %v", err)
	}
	return queryCaseOutcome{queries: qc.Queries(), result: rows}
}

// runAggregate captures SQL and returns the aggregate map from qs.
func runAggregate(t *testing.T, db *orm.DB, qs *orm.QuerySet[book], args ...any) queryCaseOutcome {
	t.Helper()
	qc := djangotest.CaptureQueries(db)
	res, err := qs.Aggregate(context.Background(), args...)
	qc.Stop()
	if err != nil {
		t.Fatalf("Aggregate: %v", err)
	}
	return queryCaseOutcome{queries: qc.Queries(), result: res}
}

// runUpdate executes an UPDATE inside a rolled-back transaction, captures only
// the UPDATE SQL, and returns the affected count plus a post-update snapshot.
func runUpdate(t *testing.T, db *orm.DB, qs *orm.QuerySet[book], pairs ...any) queryCaseOutcome {
	t.Helper()
	var out queryCaseOutcome
	err := db.Atomic(context.Background(), func(ctx context.Context) error {
		qc := djangotest.CaptureQueries(db)
		affected, err := qs.Update(ctx, pairs...)
		qc.Stop()
		if err != nil {
			return err
		}
		out.queries = qc.Queries()
		snapshot, err := orm.Query[book](db).OrderBy("id").Values("id", "pages", "published").All(ctx)
		if err != nil {
			return err
		}
		out.result = map[string]any{
			"affected": affected,
			"books":    snapshot,
		}
		return errRollbackFixture
	})
	if err != nil && !errors.Is(err, errRollbackFixture) {
		t.Fatalf("Update: %v", err)
	}
	return out
}

// runDelete executes a DELETE inside a rolled-back transaction, captures only
// the DELETE SQL, and returns the affected count plus remaining book IDs.
func runDelete(t *testing.T, db *orm.DB, qs *orm.QuerySet[book]) queryCaseOutcome {
	t.Helper()
	var out queryCaseOutcome
	err := db.Atomic(context.Background(), func(ctx context.Context) error {
		qc := djangotest.CaptureQueries(db)
		affected, err := qs.Delete(ctx)
		qc.Stop()
		if err != nil {
			return err
		}
		out.queries = qc.Queries()
		remaining, err := orm.Pluck[int64](ctx, orm.Query[book](db).OrderBy("id"), "id")
		if err != nil {
			return err
		}
		out.result = map[string]any{
			"affected":      affected,
			"remaining_ids": remaining,
		}
		return errRollbackFixture
	})
	if err != nil && !errors.Is(err, errRollbackFixture) {
		t.Fatalf("Delete: %v", err)
	}
	return out
}

// runSelectRelated captures SQL, runs qs, and extracts loaded related fields.
func runSelectRelated(t *testing.T, db *orm.DB, qs *orm.QuerySet[book], extract func([]book) []map[string]any) queryCaseOutcome {
	t.Helper()
	qc := djangotest.CaptureQueries(db)
	rows, err := qs.All(context.Background())
	qc.Stop()
	if err != nil {
		t.Fatalf("SelectRelated.All: %v", err)
	}
	return queryCaseOutcome{queries: qc.Queries(), result: extract(rows)}
}
