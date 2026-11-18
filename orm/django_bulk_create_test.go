package orm_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
)

// The models of Django's tests/bulk_create/models.py that the ported tests
// use. NullableFields keeps the nullable foreign keys only: a plain Go field
// cannot hold NULL. Restaurant is a plain model; in Django it inherits the
// abstract Place.

type bcCountry struct {
	ID           int64
	Name         string `orm:"max_length=255"`
	IsoTwoLetter string `orm:"max_length=2"`
	Description  string `orm:"type=text"`
}

type bcRestaurant struct {
	ID   int64
	Name string `orm:"max_length=100"`
}

type bcState struct {
	TwoLetterCode string `orm:"pk;max_length=2"`
}

type bcTwoFields struct {
	ID   int64
	F1   int64  `orm:"unique"`
	F2   int64  `orm:"unique"`
	Name string `orm:"max_length=15;null"`
}

type bcNoFields struct {
	ID int64
}

type bcBigAutoFieldModel struct {
	ID int64
}

type bcNullableFields struct {
	ID           int64
	AutoField    orm.FK[bcNoFields]          `orm:"null"`
	BigAutoField orm.FK[bcBigAutoFieldModel] `orm:"null"`
}

// bcModels returns the models of the bulk_create app.
func bcModels() []any {
	return []any{djangotest.App("bulk_create",
		&bcCountry{}, &bcRestaurant{}, &bcState{}, &bcTwoFields{}, &bcNoFields{},
		&bcBigAutoFieldModel{}, &bcNullableFields{})}
}

// bcData returns the countries of BulkCreateTests.setUp.
func bcData() []*bcCountry {
	return []*bcCountry{
		{Name: "United States of America", IsoTwoLetter: "US"},
		{Name: "The Netherlands", IsoTwoLetter: "NL"},
		{Name: "Germany", IsoTwoLetter: "DE"},
		{Name: "Czech Republic", IsoTwoLetter: "CZ"},
	}
}

// bcCreate runs BulkCreate and fails the test when it fails.
func bcCreate[T any](t *testing.T, db *orm.DB, objs []*T, opts ...orm.BulkOption) {
	t.Helper()
	if err := orm.Query[T](db).BulkCreate(context.Background(), objs, opts...); err != nil {
		t.Fatalf("BulkCreate: %v", err)
	}
}

// bcTwoFieldsRange returns one object per number in [low, high), with f1 set
// to the number and f2 to the number plus offset.
func bcTwoFieldsRange(low, high, offset int64) []*bcTwoFields {
	objs := make([]*bcTwoFields, 0, high-low)
	for i := low; i < high; i++ {
		objs = append(objs, &bcTwoFields{F1: i, F2: i + offset})
	}
	return objs
}

// bcMixed returns the objects of the large mixed batch: every other one
// carries a primary key.
func bcMixed() []*bcTwoFields {
	objs := bcTwoFieldsRange(100000, 101000, 1)
	for _, obj := range objs {
		if obj.F1%2 == 0 {
			obj.ID = obj.F1
		}
	}
	return objs
}

// bcMaxBatch returns the number of rows of ncols columns that fit into one
// statement of the database, or n when nothing limits it.
func bcMaxBatch(db *orm.DB, ncols, n int) int {
	features, ok := db.Dialect().(orm.FeatureDialect)
	if !ok || features.Features().MaxQueryParams == 0 {
		return n
	}
	return max(features.Features().MaxQueryParams/ncols, 1)
}

// Ported from Django: tests/bulk_create/tests.py::BulkCreateTests.test_simple
//
// Django returns the created objects; BulkCreate fills in the ones it is
// given.
func TestBulkCreateSimple(t *testing.T) {
	djangotest.EachDB(t, bcModels(), func(t *testing.T, db *orm.DB) {
		bcCreate(t, db, bcData())
		countries := orm.Query[bcCountry](db)
		djangotest.AssertQuerySetEqual(t, countries.OrderBy("-name"),
			[]string{"United States of America", "The Netherlands", "Germany", "Czech Republic"},
			func(c bcCountry) string { return c.Name })

		bcCreate(t, db, []*bcCountry{})
		djangotest.AssertEqual(t, mustCount(t, countries), 4)
	})
}

// Ported from Django: tests/bulk_create/tests.py::BulkCreateTests.test_efficiency
func TestBulkCreateEfficiency(t *testing.T) {
	djangotest.EachDB(t, bcModels(), func(t *testing.T, db *orm.DB) {
		djangotest.AssertNumQueries(t, db, 1, func() { bcCreate(t, db, bcData()) })
	})
}

// Ported from Django: tests/bulk_create/tests.py::BulkCreateTests.test_long_non_ascii_text
func TestBulkCreateLongNonASCIIText(t *testing.T) {
	djangotest.EachDB(t, bcModels(), func(t *testing.T, db *orm.DB) {
		bcCreate(t, db, []*bcCountry{{Description: strings.Repeat("Ж", 3000)}})
		djangotest.AssertEqual(t, mustCount(t, orm.Query[bcCountry](db)), 1)
	})
}

// Ported from Django: tests/bulk_create/tests.py::BulkCreateTests.test_long_and_short_text
func TestBulkCreateLongAndShortText(t *testing.T) {
	djangotest.EachDB(t, bcModels(), func(t *testing.T, db *orm.DB) {
		bcCreate(t, db, []*bcCountry{
			{Description: strings.Repeat("a", 4001), IsoTwoLetter: "A"},
			{Description: "a", IsoTwoLetter: "B"},
			{Description: strings.Repeat("Ж", 2001), IsoTwoLetter: "C"},
			{Description: "Ж", IsoTwoLetter: "D"},
		})
		djangotest.AssertEqual(t, mustCount(t, orm.Query[bcCountry](db)), 4)
	})
}

// bcStates returns one state per code.
func bcStates(codes ...string) []*bcState {
	objs := make([]*bcState, len(codes))
	for i, code := range codes {
		objs[i] = &bcState{TwoLetterCode: code}
	}
	return objs
}

// Ported from Django: tests/bulk_create/tests.py::BulkCreateTests.test_non_auto_increment_pk
func TestBulkCreateNonAutoIncrementPK(t *testing.T) {
	djangotest.EachDB(t, bcModels(), func(t *testing.T, db *orm.DB) {
		bcCreate(t, db, bcStates("IL", "NY", "CA", "ME"))
		djangotest.AssertQuerySetEqual(t, orm.Query[bcState](db).OrderBy("two_letter_code"),
			[]string{"CA", "IL", "ME", "NY"}, func(s bcState) string { return s.TwoLetterCode })
	})
}

// Ported from Django: tests/bulk_create/tests.py::BulkCreateTests.test_non_auto_increment_pk_efficiency
func TestBulkCreateNonAutoIncrementPKEfficiency(t *testing.T) {
	djangotest.EachDB(t, bcModels(), func(t *testing.T, db *orm.DB) {
		djangotest.AssertNumQueries(t, db, 1, func() { bcCreate(t, db, bcStates("IL", "NY", "CA", "ME")) })
		djangotest.AssertQuerySetEqual(t, orm.Query[bcState](db).OrderBy("two_letter_code"),
			[]string{"CA", "IL", "ME", "NY"}, func(s bcState) string { return s.TwoLetterCode })
	})
}

// Ported from Django: tests/bulk_create/tests.py::BulkCreateTests.test_batch_same_vals
func TestBulkCreateBatchSameVals(t *testing.T) {
	djangotest.EachDB(t, bcModels(), func(t *testing.T, db *orm.DB) {
		// Rows with the same values are not collapsed into one.
		bcCreate(t, db, []*bcRestaurant{{Name: "foo"}, {Name: "foo"}})
		djangotest.AssertEqual(t, mustCount(t, orm.Query[bcRestaurant](db)), 2)
	})
}

// Ported from Django: tests/bulk_create/tests.py::BulkCreateTests.test_large_batch
func TestBulkCreateLargeBatch(t *testing.T) {
	djangotest.EachDB(t, bcModels(), func(t *testing.T, db *orm.DB) {
		bcCreate(t, db, bcTwoFieldsRange(0, 1001, 1))
		rows := orm.Query[bcTwoFields](db)
		djangotest.AssertEqual(t, mustCount(t, rows), 1001)
		djangotest.AssertEqual(t, mustCount(t, rows.Filter("f1__gte", 450, "f1__lte", 550)), 101)
		djangotest.AssertEqual(t, mustCount(t, rows.Filter("f2__gte", 901)), 101)
	})
}

// Ported from Django: tests/bulk_create/tests.py::BulkCreateTests.test_large_single_field_batch
func TestBulkCreateLargeSingleFieldBatch(t *testing.T) {
	djangotest.EachDB(t, bcModels(), func(t *testing.T, db *orm.DB) {
		objs := make([]*bcRestaurant, 501)
		for i := range objs {
			objs[i] = &bcRestaurant{}
		}
		bcCreate(t, db, objs)
		djangotest.AssertEqual(t, mustCount(t, orm.Query[bcRestaurant](db)), 501)
	})
}

// Ported from Django: tests/bulk_create/tests.py::BulkCreateTests.test_large_batch_efficiency
func TestBulkCreateLargeBatchEfficiency(t *testing.T) {
	djangotest.EachDB(t, bcModels(), func(t *testing.T, db *orm.DB) {
		capture := djangotest.CaptureQueries(db)
		bcCreate(t, db, bcTwoFieldsRange(0, 1001, 1))
		capture.Stop()
		if capture.Len() >= 10 {
			t.Errorf("%d queries, want fewer than 10", capture.Len())
		}
	})
}

// Ported from Django: tests/bulk_create/tests.py::BulkCreateTests.test_large_batch_mixed
func TestBulkCreateLargeBatchMixed(t *testing.T) {
	djangotest.EachDB(t, bcModels(), func(t *testing.T, db *orm.DB) {
		bcCreate(t, db, bcMixed())
		rows := orm.Query[bcTwoFields](db)
		djangotest.AssertEqual(t, mustCount(t, rows), 1000)
		// Nothing is known about the generated keys, but the given ones exist.
		given := make([]int64, 0, 500)
		for id := int64(100000); id < 101000; id += 2 {
			given = append(given, id)
		}
		djangotest.AssertEqual(t, mustCount(t, rows.Filter("id__in", given)), 500)
		djangotest.AssertEqual(t, mustCount(t, rows.Exclude("id__in", given)), 500)
	})
}

// Ported from Django: tests/bulk_create/tests.py::BulkCreateTests.test_large_batch_mixed_efficiency
func TestBulkCreateLargeBatchMixedEfficiency(t *testing.T) {
	djangotest.EachDB(t, bcModels(), func(t *testing.T, db *orm.DB) {
		capture := djangotest.CaptureQueries(db)
		bcCreate(t, db, bcMixed())
		capture.Stop()
		if capture.Len() >= 10 {
			t.Errorf("%d queries, want fewer than 10", capture.Len())
		}
	})
}

// Ported from Django: tests/bulk_create/tests.py::BulkCreateTests.test_explicit_batch_size
func TestBulkCreateExplicitBatchSize(t *testing.T) {
	djangotest.EachDB(t, bcModels(), func(t *testing.T, db *orm.DB) {
		objs := bcTwoFieldsRange(0, 4, 0)
		rows := orm.Query[bcTwoFields](db)
		for _, size := range []int{1, 2, 3, len(objs)} {
			bcCreate(t, db, objs, orm.BatchSize(size))
			djangotest.AssertEqual(t, mustCount(t, rows), int64(len(objs)))
			if _, err := rows.Delete(context.Background()); err != nil {
				t.Fatalf("Delete: %v", err)
			}
		}
	})
}

// Ported from Django: tests/bulk_create/tests.py::BulkCreateTests.test_empty_model
func TestBulkCreateEmptyModel(t *testing.T) {
	djangotest.EachDB(t, bcModels(), func(t *testing.T, db *orm.DB) {
		objs := []*bcNoFields{{}, {}}
		bcCreate(t, db, objs)
		djangotest.AssertEqual(t, mustCount(t, orm.Query[bcNoFields](db)), 2)
		if objs[0].ID == 0 || objs[1].ID == 0 || objs[0].ID == objs[1].ID {
			t.Errorf("primary keys = %d and %d, want two generated keys", objs[0].ID, objs[1].ID)
		}
	})
}

// Ported from Django: tests/bulk_create/tests.py::BulkCreateTests.test_explicit_batch_size_efficiency
func TestBulkCreateExplicitBatchSizeEfficiency(t *testing.T) {
	djangotest.EachDB(t, bcModels(), func(t *testing.T, db *orm.DB) {
		objs := bcTwoFieldsRange(0, 100, 0)
		djangotest.AssertNumQueries(t, db, 2, func() { bcCreate(t, db, objs, orm.BatchSize(50)) })
		if _, err := orm.Query[bcTwoFields](db).Delete(context.Background()); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		djangotest.AssertNumQueries(t, db, 1, func() { bcCreate(t, db, objs, orm.BatchSize(len(objs))) })
	})
}

// bcManyCountries returns a thousand countries.
func bcManyCountries() []*bcCountry {
	objs := make([]*bcCountry, 1000)
	for i := range objs {
		objs[i] = &bcCountry{Name: "Country " + string(rune('0'+i%10)), IsoTwoLetter: "X"}
	}
	return objs
}

// Ported from Django: tests/bulk_create/tests.py::BulkCreateTests.test_explicit_batch_size_respects_max_batch_size
func TestBulkCreateExplicitBatchSizeRespectsMaxBatchSize(t *testing.T) {
	djangotest.EachDB(t, bcModels(), func(t *testing.T, db *orm.DB) {
		objs := bcManyCountries()
		// A country has three columns besides its generated key.
		maxBatch := bcMaxBatch(db, 3, len(objs))
		want := (len(objs) + maxBatch - 1) / maxBatch
		djangotest.AssertNumQueries(t, db, want, func() { bcCreate(t, db, objs, orm.BatchSize(maxBatch+1)) })
	})
}

// Ported from Django: tests/bulk_create/tests.py::BulkCreateTests.test_max_batch_size
func TestBulkCreateMaxBatchSize(t *testing.T) {
	djangotest.EachDB(t, bcModels(), func(t *testing.T, db *orm.DB) {
		objs := bcManyCountries()
		maxBatch := bcMaxBatch(db, 3, len(objs))
		want := (len(objs) + maxBatch - 1) / maxBatch
		djangotest.AssertNumQueries(t, db, want, func() { bcCreate(t, db, objs) })
	})
}

// Ported from Django: tests/bulk_create/tests.py::BulkCreateTests.test_bulk_insert_nullable_fields
//
// Only a foreign key can hold NULL in a Go model, so the port has the two
// nullable foreign keys of the Django model.
func TestBulkCreateNullableFields(t *testing.T) {
	djangotest.EachDB(t, bcModels(), func(t *testing.T, db *orm.DB) {
		auto := create(t, db, &bcNoFields{})
		bigAuto := create(t, db, &bcBigAutoFieldModel{})
		// NULL can be mixed with other values in a nullable column: each object
		// leaves one of the fields empty.
		bcCreate(t, db, []*bcNullableFields{
			{BigAutoField: fkTo[bcBigAutoFieldModel](bigAuto.ID)},
			{AutoField: fkTo[bcNoFields](auto.ID)},
		})
		rows := orm.Query[bcNullableFields](db)
		djangotest.AssertEqual(t, mustCount(t, rows), 2)
		for _, key := range []string{"auto_field", "big_auto_field"} {
			djangotest.AssertEqual(t, mustCount(t, rows.Filter(key, nil)), 1)
		}
	})
}

// bcAssertSaved checks that the row with the primary key of want holds the
// values of want.
func bcAssertSaved(t *testing.T, db *orm.DB, want *bcCountry) {
	t.Helper()
	got, err := orm.Query[bcCountry](db).Get(context.Background(), "pk", want.ID)
	djangotest.AssertEqual(t, err, nil)
	djangotest.AssertEqual(t, got, *want)
}

// Ported from Django: tests/bulk_create/tests.py::BulkCreateTests.test_set_pk_and_insert_single_item
func TestBulkCreateSetPKAndInsertSingleItem(t *testing.T) {
	djangotest.EachDB(t, bcModels(), func(t *testing.T, db *orm.DB) {
		countries := bcData()[:1]
		djangotest.AssertNumQueries(t, db, 1, func() { bcCreate(t, db, countries) })
		bcAssertSaved(t, db, countries[0])
	})
}

// Ported from Django: tests/bulk_create/tests.py::BulkCreateTests.test_set_pk_and_query_efficiency
func TestBulkCreateSetPKAndQueryEfficiency(t *testing.T) {
	djangotest.EachDB(t, bcModels(), func(t *testing.T, db *orm.DB) {
		countries := bcData()
		djangotest.AssertNumQueries(t, db, 1, func() { bcCreate(t, db, countries) })
		for _, country := range countries {
			bcAssertSaved(t, db, country)
		}
	})
}

// Ported from Django: tests/bulk_create/tests.py::BulkCreateTests.test_ignore_conflicts_ignore
func TestBulkCreateIgnoreConflicts(t *testing.T) {
	djangotest.EachDB(t, bcModels(), func(t *testing.T, db *orm.DB) {
		bcCreate(t, db, bcTwoFieldsRange(1, 4, 0))
		rows := orm.Query[bcTwoFields](db)
		djangotest.AssertEqual(t, mustCount(t, rows), 3)

		// With IgnoreConflicts the conflicting rows are skipped, and no primary
		// key is written back.
		conflicting := bcTwoFieldsRange(2, 4, 0)
		bcCreate(t, db, conflicting[:1], orm.IgnoreConflicts())
		bcCreate(t, db, conflicting, orm.IgnoreConflicts())
		djangotest.AssertEqual(t, mustCount(t, rows), 3)
		djangotest.AssertEqual(t, conflicting[0].ID, int64(0))
		djangotest.AssertEqual(t, conflicting[1].ID, int64(0))

		// New objects are created and the conflicts are skipped.
		fresh := &bcTwoFields{F1: 4, F2: 4}
		bcCreate(t, db, append(conflicting, fresh), orm.IgnoreConflicts())
		djangotest.AssertEqual(t, mustCount(t, rows), 4)
		djangotest.AssertEqual(t, fresh.ID, int64(0))

		// Without the option a conflict is an error.
		err := rows.BulkCreate(context.Background(), conflicting)
		if !errors.Is(err, orm.ErrIntegrity) || !errors.Is(err, orm.ErrUniqueViolation) {
			t.Errorf("error = %v, want a unique violation", err)
		}
	})
}

// Ported from Django: tests/bulk_create/tests.py::BulkCreateTests.test_invalid_batch_size_exception
func TestBulkCreateInvalidBatchSize(t *testing.T) {
	djangotest.EachDB(t, bcModels(), func(t *testing.T, db *orm.DB) {
		err := orm.Query[bcCountry](db).BulkCreate(context.Background(), nil, orm.BatchSize(-1))
		assertErrorContains(t, err, "atch size must be a positive integer")
	})
}

// bcUnusedCountryID returns a primary key far enough above the used ones that
// the keys generated next do not reach it.
func bcUnusedCountryID(t *testing.T, db *orm.DB) int64 {
	t.Helper()
	last, err := orm.Query[bcCountry](db).Last(context.Background())
	if errors.Is(err, orm.ErrDoesNotExist) {
		return 110
	}
	djangotest.AssertEqual(t, err, nil)
	return last.ID + 100
}

// Ported from Django: tests/bulk_create/tests.py::BulkCreateTransactionTests.test_no_unnecessary_transaction
func TestBulkCreateNoUnnecessaryTransaction(t *testing.T) {
	djangotest.EachDB(t, bcModels(), func(t *testing.T, db *orm.DB) {
		unused := bcUnusedCountryID(t, db)
		djangotest.AssertNumQueries(t, db, 1, func() {
			bcCreate(t, db, []*bcCountry{{ID: unused, Name: "France", IsoTwoLetter: "FR"}})
		})
		djangotest.AssertNumQueries(t, db, 1, func() {
			bcCreate(t, db, []*bcCountry{{Name: "Canada", IsoTwoLetter: "CA"}})
		})
	})
}

// Ported from Django: tests/bulk_create/tests.py::BulkCreateTransactionTests.test_objs_with_and_without_pk
//
// Django counts four queries: the two INSERT statements and the BEGIN and
// COMMIT around them. The query hooks do not see the BEGIN and COMMIT of the
// outermost transaction, so two are counted here.
func TestBulkCreateObjsWithAndWithoutPK(t *testing.T) {
	djangotest.EachDB(t, bcModels(), func(t *testing.T, db *orm.DB) {
		unused := bcUnusedCountryID(t, db)
		djangotest.AssertNumQueries(t, db, 2, func() {
			bcCreate(t, db, []*bcCountry{
				{ID: unused, Name: "France", IsoTwoLetter: "FR"},
				{Name: "Canada", IsoTwoLetter: "CA"},
			})
		})
		djangotest.AssertEqual(t, mustCount(t, orm.Query[bcCountry](db)), 2)
	})
}

// Ported from Django: tests/bulk_create/tests.py::BulkCreateTransactionTests.test_multiple_batches
//
// As in the test above, the BEGIN and COMMIT that Django counts are not seen
// here.
func TestBulkCreateMultipleBatches(t *testing.T) {
	djangotest.EachDB(t, bcModels(), func(t *testing.T, db *orm.DB) {
		djangotest.AssertNumQueries(t, db, 2, func() {
			bcCreate(t, db, []*bcCountry{
				{Name: "France", IsoTwoLetter: "FR"},
				{Name: "Canada", IsoTwoLetter: "CA"},
			}, orm.BatchSize(1))
		})
	})
}

// TestBulkCreateRollsBackEveryBatch has no Django counterpart: when a later
// batch fails, the rows of the earlier ones are not kept.
func TestBulkCreateRollsBackEveryBatch(t *testing.T) {
	djangotest.EachDB(t, bcModels(), func(t *testing.T, db *orm.DB) {
		objs := []*bcTwoFields{{F1: 1, F2: 1}, {F1: 2, F2: 2}, {F1: 1, F2: 3}}
		err := orm.Query[bcTwoFields](db).BulkCreate(context.Background(), objs, orm.BatchSize(1))
		if !errors.Is(err, orm.ErrUniqueViolation) {
			t.Errorf("error = %v, want a unique violation", err)
		}
		djangotest.AssertEqual(t, mustCount(t, orm.Query[bcTwoFields](db)), 0)
	})
}

// TestBulkCreateAppliesDefaultsAndStamps has no Django counterpart in this
// test module: a bulk insert fills in the defaults and the auto_now fields
// like Create.
func TestBulkCreateAppliesDefaultsAndStamps(t *testing.T) {
	type stamped struct {
		ID      int64
		Status  string    `orm:"max_length=10;default=new"`
		Created time.Time `orm:"auto_now_add"`
	}
	djangotest.EachDB(t, []any{djangotest.App("bulk_create", &stamped{})}, func(t *testing.T, db *orm.DB) {
		objs := []*stamped{{}, {Status: "done"}}
		bcCreate(t, db, objs)
		djangotest.AssertEqual(t, objs[0].Status, "new")
		rows := mustAll(t, orm.Query[stamped](db).OrderBy("id"))
		djangotest.AssertEqual(t, len(rows), 2)
		djangotest.AssertEqual(t, rows[0].Status, "new")
		djangotest.AssertEqual(t, rows[1].Status, "done")
		djangotest.AssertEqual(t, rows[0].Created.IsZero(), false)
	})
}
